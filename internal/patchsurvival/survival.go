package patchsurvival

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Status string

const (
	StatusSurvived Status = "survived"
	StatusModified Status = "modified"
	StatusReverted Status = "reverted"
	StatusUnknown  Status = "unknown"
)

type Options struct {
	RepoRoot    string
	Ref         string
	ExposePaths bool
}

type Result struct {
	Summary Summary      `json:"summary"`
	Files   []FileResult `json:"files,omitempty"`
}

type Summary struct {
	FileCount int `json:"file_count"`
	HunkCount int `json:"hunk_count"`
	Survived  int `json:"survived"`
	Modified  int `json:"modified"`
	Reverted  int `json:"reverted"`
	Unknown   int `json:"unknown"`
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

func AnalyzeUnifiedDiff(ctx context.Context, diff []byte, options Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	options.RepoRoot = strings.TrimSpace(options.RepoRoot)
	if options.RepoRoot == "" {
		return Result{}, errors.New("repo root is required")
	}
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{}, errors.New("diff contains no file hunks")
	}
	result := Result{Files: make([]FileResult, 0, len(files))}
	for _, file := range files {
		fileResult := analyzeFile(ctx, file, options)
		result.Files = append(result.Files, fileResult)
		result.Summary.FileCount++
		result.Summary.HunkCount += fileResult.HunkCount
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
	return result, nil
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

func analyzeFile(ctx context.Context, file diffFile, options Options) FileResult {
	result := FileResult{
		PathHash:       hashPath(file.path),
		Status:         StatusUnknown,
		HunkCount:      file.hunkCount,
		AddedLineCount: len(file.addedLines),
	}
	if options.ExposePaths {
		result.Path = file.path
	}
	if len(file.addedLines) == 0 {
		result.Reason = "no_added_lines"
		return result
	}
	content, err := targetFileContent(ctx, options, file.path)
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
