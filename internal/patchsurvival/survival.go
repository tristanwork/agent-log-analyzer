package patchsurvival

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const defaultMaxCachedFiles = 1024

type Status string

const (
	StatusSurvived Status = "survived"
	StatusModified Status = "modified"
	StatusReverted Status = "reverted"
	StatusUnknown  Status = "unknown"
)

type Options struct {
	RepoRoot       string
	Ref            string
	ExposePaths    bool
	TokenCount     int
	CostUSD        float64
	MaxCachedFiles int
}

type Result struct {
	Summary Summary      `json:"summary"`
	Files   []FileResult `json:"files,omitempty"`
}

type Summary struct {
	DiffCount         int         `json:"diff_count,omitempty"`
	FileCount         int         `json:"file_count"`
	HunkCount         int         `json:"hunk_count"`
	AddedLineCount    int         `json:"added_line_count"`
	SurvivedLineCount int         `json:"survived_line_count"`
	Survived          int         `json:"survived"`
	Modified          int         `json:"modified"`
	Reverted          int         `json:"reverted"`
	Unknown           int         `json:"unknown"`
	PatchYield        *PatchYield `json:"patch_yield,omitempty"`
}

type PatchYield struct {
	TokenCount               int     `json:"token_count,omitempty"`
	CostUSD                  float64 `json:"cost_usd,omitempty"`
	SurvivedLinesPer1KTokens float64 `json:"survived_lines_per_1k_tokens,omitempty"`
	SurvivedLinesPerDollar   float64 `json:"survived_lines_per_dollar,omitempty"`
}

type FileResult struct {
	Path              string `json:"path,omitempty"`
	PathHash          string `json:"path_hash"`
	Status            Status `json:"status"`
	HunkCount         int    `json:"hunk_count"`
	AddedLineCount    int    `json:"added_line_count"`
	SurvivedLineCount int    `json:"survived_line_count"`
	Reason            string `json:"reason,omitempty"`
}

type diffFile struct {
	path       string
	hunkCount  int
	addedLines []string
}

type survivalAnalyzer struct {
	options    Options
	cacheLimit int
	cache      map[string]cachedContent
	cacheOrder []string
}

type cachedContent struct {
	data []byte
	err  error
}

func AnalyzeUnifiedDiff(ctx context.Context, diff []byte, options Options) (Result, error) {
	return AnalyzeUnifiedDiffBatch(ctx, [][]byte{diff}, options)
}

func AnalyzeUnifiedDiffBatch(ctx context.Context, diffs [][]byte, options Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(diffs) == 0 {
		return Result{}, errors.New("at least one diff is required")
	}
	analyzer, err := newSurvivalAnalyzer(options)
	if err != nil {
		return Result{}, err
	}
	result := Result{Files: make([]FileResult, 0, len(diffs))}
	result.Summary.DiffCount = len(diffs)
	for index, diff := range diffs {
		files, err := parseUnifiedDiff(diff)
		if err != nil {
			return Result{}, err
		}
		if len(files) == 0 {
			if len(diffs) == 1 {
				return Result{}, errors.New("diff contains no file hunks")
			}
			return Result{}, fmt.Errorf("diff %d contains no file hunks", index+1)
		}
		for _, file := range files {
			fileResult := analyzer.analyzeFile(ctx, file)
			result.Files = append(result.Files, fileResult)
			result.Summary.FileCount++
			result.Summary.HunkCount += fileResult.HunkCount
			result.Summary.AddedLineCount += fileResult.AddedLineCount
			result.Summary.SurvivedLineCount += fileResult.SurvivedLineCount
			switch fileResult.Status {
			case StatusSurvived:
				result.Summary.Survived++
			case StatusModified:
				result.Summary.Modified++
			case StatusReverted:
				result.Summary.Reverted++
			default:
				result.Summary.Unknown++
			}
		}
	}
	result.Summary.PatchYield = computePatchYield(result.Summary.SurvivedLineCount, analyzer.options)
	return result, nil
}

func newSurvivalAnalyzer(options Options) (*survivalAnalyzer, error) {
	options.RepoRoot = strings.TrimSpace(options.RepoRoot)
	if options.RepoRoot == "" {
		return nil, errors.New("repo root is required")
	}
	if options.MaxCachedFiles < 0 {
		return nil, errors.New("max cached files cannot be negative")
	}
	cacheLimit := options.MaxCachedFiles
	if cacheLimit == 0 {
		cacheLimit = defaultMaxCachedFiles
	}
	return &survivalAnalyzer{
		options:    options,
		cacheLimit: cacheLimit,
		cache:      make(map[string]cachedContent),
	}, nil
}

func computePatchYield(survivedLines int, options Options) *PatchYield {
	if options.TokenCount <= 0 && options.CostUSD <= 0 {
		return nil
	}
	yield := &PatchYield{}
	if options.TokenCount > 0 {
		yield.TokenCount = options.TokenCount
		yield.SurvivedLinesPer1KTokens = round2(float64(survivedLines) * 1000 / float64(options.TokenCount))
	}
	if options.CostUSD > 0 {
		yield.CostUSD = round2(options.CostUSD)
		yield.SurvivedLinesPerDollar = round2(float64(survivedLines) / options.CostUSD)
	}
	return yield
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func parseUnifiedDiff(diff []byte) ([]diffFile, error) {
	scanner := bufio.NewScanner(bytes.NewReader(diff))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var files []diffFile
	var current *diffFile
	flush := func() {
		if current != nil && current.path != "" && current.hunkCount > 0 {
			files = append(files, *current)
		}
		current = nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			current = &diffFile{path: pathFromDiffGit(line)}
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			if current == nil {
				current = &diffFile{}
			}
			if path := pathFromPlusPlusPlus(line); path != "" {
				current.path = path
			}
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if current == nil {
				current = &diffFile{}
			}
			current.hunkCount++
			continue
		}
		if current == nil || strings.HasPrefix(line, "+++") || !strings.HasPrefix(line, "+") {
			continue
		}
		normalized := normalizeLine(line[1:])
		if normalized != "" {
			current.addedLines = append(current.addedLines, normalized)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	for index := range files {
		path, err := safeRepoPath(files[index].path)
		if err != nil {
			return nil, err
		}
		files[index].path = path
	}
	return files, nil
}

func (analyzer *survivalAnalyzer) analyzeFile(ctx context.Context, file diffFile) FileResult {
	result := FileResult{
		PathHash:       hashPath(file.path),
		Status:         StatusUnknown,
		HunkCount:      file.hunkCount,
		AddedLineCount: len(file.addedLines),
	}
	if analyzer.options.ExposePaths {
		result.Path = file.path
	}
	if len(file.addedLines) == 0 {
		result.Reason = "no_added_lines"
		return result
	}
	content, err := analyzer.targetFileContent(ctx, file.path)
	if err != nil {
		result.Reason = "target_unavailable"
		return result
	}
	lines := normalizedLineSet(content)
	for _, added := range file.addedLines {
		if lines[added] {
			result.SurvivedLineCount++
		}
	}
	switch {
	case result.SurvivedLineCount == result.AddedLineCount:
		result.Status = StatusSurvived
	case result.SurvivedLineCount == 0:
		result.Status = StatusReverted
	default:
		result.Status = StatusModified
	}
	return result
}

func (analyzer *survivalAnalyzer) targetFileContent(ctx context.Context, path string) ([]byte, error) {
	key := analyzer.options.Ref + "\x00" + path
	if cached, ok := analyzer.cache[key]; ok {
		return cached.data, cached.err
	}
	data, err := targetFileContent(ctx, analyzer.options, path)
	analyzer.storeCachedContent(key, data, err)
	return data, err
}

func (analyzer *survivalAnalyzer) storeCachedContent(key string, data []byte, err error) {
	if analyzer.cacheLimit <= 0 {
		return
	}
	if _, ok := analyzer.cache[key]; ok {
		analyzer.cache[key] = cachedContent{data: data, err: err}
		return
	}
	for len(analyzer.cacheOrder) >= analyzer.cacheLimit {
		oldest := analyzer.cacheOrder[0]
		analyzer.cacheOrder = analyzer.cacheOrder[1:]
		delete(analyzer.cache, oldest)
	}
	analyzer.cache[key] = cachedContent{data: data, err: err}
	analyzer.cacheOrder = append(analyzer.cacheOrder, key)
}

func targetFileContent(ctx context.Context, options Options, path string) ([]byte, error) {
	if options.Ref == "" {
		return os.ReadFile(filepath.Join(options.RepoRoot, filepath.FromSlash(path)))
	}
	cmd := exec.CommandContext(ctx, "git", "-C", options.RepoRoot, "show", fmt.Sprintf("%s:%s", options.Ref, path))
	return cmd.Output()
}

func pathFromDiffGit(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return ""
	}
	return stripDiffPrefix(fields[3])
}

func pathFromPlusPlusPlus(line string) string {
	path := strings.TrimSpace(strings.TrimPrefix(line, "+++"))
	if path == "/dev/null" {
		return ""
	}
	if idx := strings.IndexAny(path, "\t "); idx >= 0 {
		path = path[:idx]
	}
	return stripDiffPrefix(path)
}

func stripDiffPrefix(path string) string {
	path = strings.Trim(path, `"`)
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}

func safeRepoPath(path string) (string, error) {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, "/"))
	if path == "" || strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("unsafe diff path: %q", path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean != path || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("unsafe diff path: %q", path)
	}
	return clean, nil
}

func normalizedLineSet(content []byte) map[string]bool {
	lines := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if line := normalizeLine(scanner.Text()); line != "" {
			lines[line] = true
		}
	}
	return lines
}

func normalizeLine(line string) string {
	return strings.TrimSpace(line)
}

func hashPath(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}
