package patchsurvival

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeUnifiedDiffClassifiesPatchSurvival(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "survived.txt", "existing\nadded survived\n")
	writeFile(t, repo, "modified.txt", "first kept\n")
	writeFile(t, repo, "reverted.txt", "old line\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "fixture")

	diff := strings.Join([]string{
		"diff --git a/survived.txt b/survived.txt",
		"@@ -1 +1,2 @@",
		"+added survived",
		"diff --git a/modified.txt b/modified.txt",
		"@@ -1 +1,3 @@",
		"+first kept",
		"+second gone",
		"diff --git a/reverted.txt b/reverted.txt",
		"@@ -1 +1,2 @@",
		"+never persisted",
		"diff --git a/missing.txt b/missing.txt",
		"@@ -0,0 +1 @@",
		"+missing target",
	}, "\n")

	result, err := AnalyzeUnifiedDiff(context.Background(), []byte(diff), Options{RepoRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Survived != 1 || result.Summary.Modified != 1 || result.Summary.Reverted != 1 || result.Summary.Unknown != 1 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if result.Summary.AddedLineCount != 5 || result.Summary.SurvivedLineCount != 2 {
		t.Fatalf("unexpected line counts: %#v", result.Summary)
	}
	for _, file := range result.Files {
		if file.Path != "" {
			t.Fatalf("paths should be hidden by default: %#v", file)
		}
		if file.PathHash == "" {
			t.Fatalf("expected path hash: %#v", file)
		}
	}
}

func TestAnalyzeUnifiedDiffComputesPatchYieldWhenSpendIsAvailable(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "yield.txt", "base\none\ntwo\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "fixture")

	diff := strings.Join([]string{
		"diff --git a/yield.txt b/yield.txt",
		"@@ -1 +1,3 @@",
		"+one",
		"+two",
	}, "\n")

	result, err := AnalyzeUnifiedDiff(context.Background(), []byte(diff), Options{
		RepoRoot:   repo,
		TokenCount: 4000,
		CostUSD:    0.50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.PatchYield == nil {
		t.Fatalf("expected patch yield: %#v", result.Summary)
	}
	if result.Summary.PatchYield.SurvivedLinesPer1KTokens != 0.5 {
		t.Fatalf("unexpected token yield: %#v", result.Summary.PatchYield)
	}
	if result.Summary.PatchYield.SurvivedLinesPerDollar != 4 {
		t.Fatalf("unexpected dollar yield: %#v", result.Summary.PatchYield)
	}
}

func TestAnalyzeUnifiedDiffBatchAggregatesMultipleDiffs(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "one.txt", "base\nkept one\n")
	writeFile(t, repo, "two.txt", "base\nkept two\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "fixture")

	diffs := [][]byte{
		[]byte(strings.Join([]string{
			"diff --git a/one.txt b/one.txt",
			"@@ -1 +1,2 @@",
			"+kept one",
		}, "\n")),
		[]byte(strings.Join([]string{
			"diff --git a/two.txt b/two.txt",
			"@@ -1 +1,3 @@",
			"+kept two",
			"+dropped two",
		}, "\n")),
	}

	result, err := AnalyzeUnifiedDiffBatch(context.Background(), diffs, Options{
		RepoRoot:       repo,
		TokenCount:     2000,
		CostUSD:        0.25,
		MaxCachedFiles: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.DiffCount != 2 || result.Summary.FileCount != 2 || result.Summary.HunkCount != 2 {
		t.Fatalf("unexpected batch counters: %#v", result.Summary)
	}
	if result.Summary.Survived != 1 || result.Summary.Modified != 1 {
		t.Fatalf("unexpected batch buckets: %#v", result.Summary)
	}
	if result.Summary.AddedLineCount != 3 || result.Summary.SurvivedLineCount != 2 {
		t.Fatalf("unexpected batch line counts: %#v", result.Summary)
	}
	if result.Summary.PatchYield == nil || result.Summary.PatchYield.SurvivedLinesPer1KTokens != 1 || result.Summary.PatchYield.SurvivedLinesPerDollar != 8 {
		t.Fatalf("unexpected batch yield: %#v", result.Summary.PatchYield)
	}
	for _, file := range result.Files {
		if file.Path != "" {
			t.Fatalf("paths should remain hidden in batch output: %#v", file)
		}
	}
}

func TestAnalyzeDiffInputsGroupsBySafeContext(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "one.txt", "base\nkept one\n")
	writeFile(t, repo, "two.txt", "base\nkept two\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "fixture")

	projectCWD := filepath.Join(repo, "private", "workspace")
	inputs := []DiffInput{
		{
			Diff: []byte(strings.Join([]string{
				"diff --git a/one.txt b/one.txt",
				"@@ -1 +1,2 @@",
				"+kept one",
			}, "\n")),
			Context: Context{
				ProjectCWD:    projectCWD,
				SourceHarness: "codex",
				SessionID:     "session-one-private",
			},
		},
		{
			Diff: []byte(strings.Join([]string{
				"diff --git a/two.txt b/two.txt",
				"@@ -1 +1,2 @@",
				"+kept two",
			}, "\n")),
			Context: Context{
				ProjectCWD:    projectCWD,
				SourceHarness: "codex",
				SessionID:     "session-two-private",
			},
		},
	}

	result, err := AnalyzeDiffInputs(context.Background(), inputs, Options{RepoRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Summary.Groups) != 2 {
		t.Fatalf("expected one group per session hash, got %#v", result.Summary.Groups)
	}
	if result.Summary.Groups[0].ProjectCWDHash == "" || result.Summary.Groups[0].ProjectCWDHash != result.Summary.Groups[1].ProjectCWDHash {
		t.Fatalf("expected stable project cwd hash across groups: %#v", result.Summary.Groups)
	}
	if result.Summary.Groups[0].SessionHash == "" || result.Summary.Groups[0].SessionHash == result.Summary.Groups[1].SessionHash {
		t.Fatalf("expected distinct session hashes: %#v", result.Summary.Groups)
	}
	if result.Summary.Groups[0].SourceHarness != "codex" || result.Summary.Groups[1].SourceHarness != "codex" {
		t.Fatalf("expected source harness to be retained: %#v", result.Summary.Groups)
	}
	if result.Summary.Groups[0].DiffCount != 1 || result.Summary.Groups[0].SurvivedLineCount != 1 {
		t.Fatalf("unexpected first group counts: %#v", result.Summary.Groups[0])
	}
	if strings.Contains(result.Summary.Groups[0].ProjectCWDHash, "workspace") || strings.Contains(result.Summary.Groups[0].SessionHash, "session-one") {
		t.Fatalf("group hashes leaked raw context: %#v", result.Summary.Groups[0])
	}
}

func TestAnalyzeUnifiedDiffCanCompareAgainstSelectedRef(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "feature.txt", "base\nsurvived in head\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "head contains patch")
	writeFile(t, repo, "feature.txt", "base\n")

	diff := strings.Join([]string{
		"diff --git a/feature.txt b/feature.txt",
		"@@ -1 +1,2 @@",
		"+survived in head",
	}, "\n")

	worktree, err := AnalyzeUnifiedDiff(context.Background(), []byte(diff), Options{RepoRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	if worktree.Summary.Reverted != 1 {
		t.Fatalf("expected worktree comparison to see reverted patch, got %#v", worktree.Summary)
	}
	head, err := AnalyzeUnifiedDiff(context.Background(), []byte(diff), Options{RepoRoot: repo, Ref: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if head.Summary.Survived != 1 {
		t.Fatalf("expected selected ref comparison to see survived patch, got %#v", head.Summary)
	}
}

func TestAnalyzeUnifiedDiffCanExposeLocalPathsForDebug(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "debug.txt", "debug line\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "fixture")

	result, err := AnalyzeUnifiedDiff(context.Background(), []byte("diff --git a/debug.txt b/debug.txt\n@@ -0,0 +1 @@\n+debug line\n"), Options{RepoRoot: repo, ExposePaths: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "debug.txt" {
		t.Fatalf("expected explicit local path exposure, got %#v", result.Files)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	return repo
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
