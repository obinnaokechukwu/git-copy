package scrub

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
)

func initRepoForFilter(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := gitx.Run(nil, repo, "init", "-b", "main")
	if err != nil {
		_, err2 := gitx.Run(nil, repo, "init")
		if err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(nil, repo, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(nil, repo, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(nil, repo, "config", "user.email", "obinnaokechukwu@private.invalid")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello obinnaokechukwu\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "a.txt")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "add obinnaokechukwu")

	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=obinnaokechukwu\n"), 0o644); err != nil {
		t.Fatalf("write env: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", ".env")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "env obinnaokechukwu")
	return repo
}

func runExportFilterImport(t *testing.T, srcRepo, dstBare string, rules CompiledRules) {
	t.Helper()
	if _, err := gitx.Run(nil, "", "init", "--bare", dstBare); err != nil {
		t.Fatalf("init bare: %v", err)
	}

	exp := gitx.FastExportCmd(srcRepo, "--all", "--signed-tags=strip", "--tag-of-filtered-object=rewrite")
	expStdout, err := exp.StdoutPipe()
	if err != nil {
		t.Fatalf("stdoutpipe: %v", err)
	}
	var expStderr bytes.Buffer
	exp.Stderr = &expStderr

	imp := gitx.FastImportCmd(dstBare)
	impStdin, err := imp.StdinPipe()
	if err != nil {
		t.Fatalf("stdinpipe: %v", err)
	}
	var impStderr bytes.Buffer
	imp.Stderr = &impStderr

	if err := imp.Start(); err != nil {
		t.Fatalf("import start: %v (%s)", err, impStderr.String())
	}
	if err := exp.Start(); err != nil {
		_ = imp.Process.Kill()
		t.Fatalf("export start: %v (%s)", err, expStderr.String())
	}

	filter := NewExportFilter(rules)
	errCh := make(chan error, 1)
	go func() {
		defer impStdin.Close()
		errCh <- filter.Filter(expStdout, impStdin)
	}()

	ferr := <-errCh // Wait for filter to finish first

	if err := exp.Wait(); err != nil {
		_ = imp.Process.Kill()
		t.Fatalf("export wait: %v (%s)", err, expStderr.String())
	}
	if ferr != nil {
		_ = imp.Process.Kill()
		t.Fatalf("filter: %v", ferr)
	}
	if err := imp.Wait(); err != nil {
		t.Fatalf("import wait: %v (%s)", err, impStderr.String())
	}

	_, _ = gitx.Run(context.Background(), dstBare, "repack", "-adq")
}

func TestExportFilter_SkipsExcludedOnlyCommitAndRewritesIdentity(t *testing.T) {
	repo := initRepoForFilter(t)
	bare := filepath.Join(t.TempDir(), "out.git")

	rules, err := Compile(Rules{
		PrivateUsername:   "obinnaokechukwu",
		Replacement:       "johndoe",
		ExcludePatterns:   []string{".env"},
		PublicAuthorName:  "John Doe",
		PublicAuthorEmail: "john@public.invalid",
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	runExportFilterImport(t, repo, bare, rules)

	out, err := gitx.Run(nil, bare, "log", "-1", "--format=%s", "refs/heads/main")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	subj := strings.TrimSpace(out.Stdout)
	if strings.Contains(subj, "env") {
		t.Fatalf("expected env commit skipped, got subject: %q", subj)
	}
	if strings.Contains(subj, "obinnaokechukwu") {
		t.Fatalf("expected scrubbed subject, got: %q", subj)
	}

	out, err = gitx.Run(nil, bare, "log", "-1", "--format=%an <%ae>", "refs/heads/main")
	if err != nil {
		t.Fatalf("log author: %v", err)
	}
	id := strings.TrimSpace(out.Stdout)
	if id != "John Doe <john@public.invalid>" {
		t.Fatalf("unexpected author identity: %q", id)
	}

	show, err := gitx.Run(nil, bare, "show", "refs/heads/main:a.txt")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if strings.Contains(show.Stdout, "obinnaokechukwu") || !strings.Contains(show.Stdout, "johndoe") {
		t.Fatalf("content not scrubbed: %q", show.Stdout)
	}

	ls, err := gitx.Run(nil, bare, "ls-tree", "-r", "--name-only", "refs/heads/main")
	if err != nil {
		t.Fatalf("ls-tree: %v", err)
	}
	if strings.Contains(ls.Stdout, ".env") {
		t.Fatalf("expected .env excluded; tree:\n%s", ls.Stdout)
	}
}

func TestFilterOps_RejectsExplicitRenameFromExcludedToIncluded(t *testing.T) {
	rules, err := Compile(Rules{
		PrivateUsername: "obinnaokechukwu",
		Replacement:     "johndoe",
		ExcludePatterns: []string{".env"},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	f := NewExportFilter(rules)

	ops := []string{
		"R .env public.txt\n",
	}
	_, _, err = f.filterOps(ops)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "unsafe rename") {
		t.Fatalf("expected unsafe rename error, got: %v", err)
	}
}

// initRepoWithLicenseHistory creates a repo with LICENSE that changes over time.
// Returns the repo path.
func initRepoWithLicenseHistory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := gitx.Run(nil, repo, "init", "-b", "main")
	if err != nil {
		_, err2 := gitx.Run(nil, repo, "init")
		if err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(nil, repo, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(nil, repo, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(nil, repo, "config", "user.email", "obinnaokechukwu@private.invalid")

	// Commit 1: Initial LICENSE (old copyright)
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("Copyright 2020 obinnaokechukwu\nOld license text\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Initial LICENSE")

	// Commit 2: Add code file
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n// by obinnaokechukwu\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "main.go")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Add main.go")

	// Commit 3: Update LICENSE (changed copyright year)
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("Copyright 2021 obinnaokechukwu\nUpdated license text\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Update LICENSE year")

	// Commit 4: Final LICENSE (current state)
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("Copyright 2024 obinnaokechukwu\nFinal license text\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Final LICENSE update")

	return repo
}

func TestReplaceHistoryWithCurrent_BasicReplacement(t *testing.T) {
	repo := initRepoWithLicenseHistory(t)
	bare := filepath.Join(t.TempDir(), "out.git")

	// Read current LICENSE content
	licenseContent := []byte("Copyright 2024 obinnaokechukwu\nFinal license text\n")

	rules, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"LICENSE"},
		ReplaceHistoryContent:     map[string][]byte{"LICENSE": licenseContent},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	runExportFilterImport(t, repo, bare, rules)

	// Get all commits using --all flag to find all branches
	out, err := gitx.Run(nil, bare, "log", "--oneline", "--all")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	commits := strings.Split(strings.TrimSpace(out.Stdout), "\n")

	// Should have fewer commits - the LICENSE-only update commits should be dropped
	// Original: Initial LICENSE, Add main.go, Update LICENSE year, Final LICENSE update
	// After filter: Initial LICENSE (with final content), Add main.go
	// The "Update LICENSE year" and "Final LICENSE update" commits should be dropped
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d:\n%s", len(commits), out.Stdout)
	}

	// Check that LICENSE has the final content in the first commit
	// (The first commit in the filtered history should have the final LICENSE)
	firstCommit, err := gitx.Run(nil, bare, "rev-list", "--reverse", "refs/heads/main")
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}
	firstSHA := strings.TrimSpace(strings.Split(firstCommit.Stdout, "\n")[0])

	show, err := gitx.Run(nil, bare, "show", firstSHA+":LICENSE")
	if err != nil {
		t.Fatalf("show LICENSE: %v", err)
	}

	// The content should be scrubbed (obinnaokechukwu -> johndoe) but should be the final version
	if !strings.Contains(show.Stdout, "2024") {
		t.Fatalf("expected LICENSE to have 2024 (final version), got: %q", show.Stdout)
	}
	if !strings.Contains(show.Stdout, "johndoe") {
		t.Fatalf("expected LICENSE to be scrubbed, got: %q", show.Stdout)
	}
	if strings.Contains(show.Stdout, "obinnaokechukwu") {
		t.Fatalf("expected obinnaokechukwu to be scrubbed out of LICENSE, got: %q", show.Stdout)
	}
}

func TestReplaceHistoryWithCurrent_IntermediateChangesSkipped(t *testing.T) {
	repo := initRepoWithLicenseHistory(t)
	bare := filepath.Join(t.TempDir(), "out.git")

	licenseContent := []byte("Copyright 2024 obinnaokechukwu\nFinal license text\n")

	rules, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"LICENSE"},
		ReplaceHistoryContent:     map[string][]byte{"LICENSE": licenseContent},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	runExportFilterImport(t, repo, bare, rules)

	// Check all commits in the history - LICENSE should be identical in all of them
	out, err := gitx.Run(nil, bare, "rev-list", "refs/heads/main")
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}
	shas := strings.Split(strings.TrimSpace(out.Stdout), "\n")

	var lastContent string
	for _, sha := range shas {
		show, err := gitx.Run(nil, bare, "show", sha+":LICENSE")
		if err != nil {
			continue // File might not exist in this commit
		}
		if lastContent == "" {
			lastContent = show.Stdout
		} else if show.Stdout != lastContent {
			t.Fatalf("LICENSE content changed between commits!\nFirst: %q\nCurrent (%s): %q", lastContent, sha, show.Stdout)
		}
	}
}

func TestReplaceHistoryWithCurrent_MixedCommits(t *testing.T) {
	// Test that commits modifying both replaced files AND other files keep the other changes
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := gitx.Run(nil, repo, "init", "-b", "main")
	if err != nil {
		_, err2 := gitx.Run(nil, repo, "init")
		if err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(nil, repo, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(nil, repo, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(nil, repo, "config", "user.email", "obinnaokechukwu@private.invalid")

	// Commit 1: Initial files
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("Old license\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# README v1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", ".")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Initial commit")

	// Commit 2: Update both LICENSE and README in same commit
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("New license\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# README v2\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", ".")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Update both files")

	bare := filepath.Join(t.TempDir(), "out.git")
	licenseContent := []byte("New license\n")

	rules, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"LICENSE"},
		ReplaceHistoryContent:     map[string][]byte{"LICENSE": licenseContent},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	runExportFilterImport(t, repo, bare, rules)

	// Should have 2 commits still (the mixed commit should be kept because it has README changes)
	out, err := gitx.Run(nil, bare, "log", "--oneline", "--all")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	commits := strings.Split(strings.TrimSpace(out.Stdout), "\n")
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d:\n%s", len(commits), out.Stdout)
	}

	// Check that README.md was updated correctly
	show, err := gitx.Run(nil, bare, "show", "refs/heads/main:README.md")
	if err != nil {
		t.Fatalf("show README: %v", err)
	}
	if !strings.Contains(show.Stdout, "v2") {
		t.Fatalf("expected README v2, got: %q", show.Stdout)
	}
}

func TestReplaceHistoryWithCurrent_DeleteOperationsSkipped(t *testing.T) {
	// Test that D (delete) operations for replaced files are skipped
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := gitx.Run(nil, repo, "init", "-b", "main")
	if err != nil {
		_, err2 := gitx.Run(nil, repo, "init")
		if err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(nil, repo, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(nil, repo, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(nil, repo, "config", "user.email", "obinnaokechukwu@private.invalid")

	// Commit 1: Add LICENSE
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("License v1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Add LICENSE")

	// Commit 2: Add code file
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "main.go")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Add main.go")

	// Commit 3: Delete LICENSE
	_, _ = gitx.Run(nil, repo, "rm", "LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Delete LICENSE")

	// Commit 4: Re-add LICENSE
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("License v2 (final)\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Re-add LICENSE")

	bare := filepath.Join(t.TempDir(), "out.git")
	licenseContent := []byte("License v2 (final)\n")

	rules, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"LICENSE"},
		ReplaceHistoryContent:     map[string][]byte{"LICENSE": licenseContent},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	runExportFilterImport(t, repo, bare, rules)

	// LICENSE should exist in all commits (delete was skipped)
	out, err := gitx.Run(nil, bare, "rev-list", "refs/heads/main")
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}
	shas := strings.Split(strings.TrimSpace(out.Stdout), "\n")

	for _, sha := range shas {
		_, err := gitx.Run(nil, bare, "show", sha+":LICENSE")
		if err != nil {
			t.Fatalf("LICENSE should exist in commit %s, but got error: %v", sha, err)
		}
	}
}

func TestReplaceHistoryWithCurrent_MultipleFiles(t *testing.T) {
	// Test multiple files in replace_history_with_current
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := gitx.Run(nil, repo, "init", "-b", "main")
	if err != nil {
		_, err2 := gitx.Run(nil, repo, "init")
		if err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(nil, repo, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(nil, repo, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(nil, repo, "config", "user.email", "obinnaokechukwu@private.invalid")

	// Commit 1: Add files
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("License v1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "NOTICE"), []byte("Notice v1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", ".")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Initial files")

	// Commit 2: Update LICENSE
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("License v2\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Update LICENSE")

	// Commit 3: Update NOTICE
	if err := os.WriteFile(filepath.Join(repo, "NOTICE"), []byte("Notice v2\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "NOTICE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Update NOTICE")

	bare := filepath.Join(t.TempDir(), "out.git")

	rules, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"LICENSE", "NOTICE"},
		ReplaceHistoryContent: map[string][]byte{
			"LICENSE": []byte("License v2\n"),
			"NOTICE":  []byte("Notice v2\n"),
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	runExportFilterImport(t, repo, bare, rules)

	// Should only have 1 commit (all intermediate changes dropped)
	out, err := gitx.Run(nil, bare, "log", "--oneline", "--all")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	commits := strings.Split(strings.TrimSpace(out.Stdout), "\n")
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d:\n%s", len(commits), out.Stdout)
	}

	// Check both files have final content
	show, err := gitx.Run(nil, bare, "show", "refs/heads/main:LICENSE")
	if err != nil {
		t.Fatalf("show LICENSE: %v", err)
	}
	if !strings.Contains(show.Stdout, "v2") {
		t.Fatalf("expected LICENSE v2, got: %q", show.Stdout)
	}

	show, err = gitx.Run(nil, bare, "show", "refs/heads/main:NOTICE")
	if err != nil {
		t.Fatalf("show NOTICE: %v", err)
	}
	if !strings.Contains(show.Stdout, "v2") {
		t.Fatalf("expected NOTICE v2, got: %q", show.Stdout)
	}
}

func TestReplaceHistoryWithCurrent_FileNotInHistory(t *testing.T) {
	// Test handling files that don't exist in history but do exist in HEAD
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := gitx.Run(nil, repo, "init", "-b", "main")
	if err != nil {
		_, err2 := gitx.Run(nil, repo, "init")
		if err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(nil, repo, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(nil, repo, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(nil, repo, "config", "user.email", "obinnaokechukwu@private.invalid")

	// Commit 1: Add code file (no LICENSE yet)
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "main.go")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Add main.go")

	// Commit 2: Add LICENSE
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), []byte("My License\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Add LICENSE")

	bare := filepath.Join(t.TempDir(), "out.git")

	rules, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"LICENSE"},
		ReplaceHistoryContent:     map[string][]byte{"LICENSE": []byte("My License\n")},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	runExportFilterImport(t, repo, bare, rules)

	// Should have 2 commits (first without LICENSE, second adding it with final content)
	out, err := gitx.Run(nil, bare, "log", "--oneline", "--all")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	commits := strings.Split(strings.TrimSpace(out.Stdout), "\n")
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d:\n%s", len(commits), out.Stdout)
	}

	// First commit should not have LICENSE
	firstCommit, _ := gitx.Run(nil, bare, "rev-list", "--reverse", "HEAD")
	firstSHA := strings.TrimSpace(strings.Split(firstCommit.Stdout, "\n")[0])

	_, err = gitx.Run(nil, bare, "show", firstSHA+":LICENSE")
	if err == nil {
		t.Fatal("expected LICENSE to not exist in first commit")
	}

	// Second commit should have LICENSE
	_, err = gitx.Run(nil, bare, "show", "refs/heads/main:LICENSE")
	if err != nil {
		t.Fatalf("expected LICENSE in HEAD: %v", err)
	}
}

func TestReplaceHistoryWithCurrent_FileNotInHEAD(t *testing.T) {
	// Test handling files that are specified but don't exist in HEAD
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := gitx.Run(nil, repo, "init", "-b", "main")
	if err != nil {
		_, err2 := gitx.Run(nil, repo, "init")
		if err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(nil, repo, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(nil, repo, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(nil, repo, "config", "user.email", "obinnaokechukwu@private.invalid")

	// Commit 1: Add files
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "OLD_LICENSE"), []byte("Old license\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", ".")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Initial")

	// Commit 2: Delete OLD_LICENSE
	_, _ = gitx.Run(nil, repo, "rm", "OLD_LICENSE")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "Remove OLD_LICENSE")

	bare := filepath.Join(t.TempDir(), "out.git")

	// OLD_LICENSE doesn't exist in HEAD, so ReplaceHistoryContent is empty for it
	rules, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"OLD_LICENSE"},
		ReplaceHistoryContent:     map[string][]byte{}, // Empty - file doesn't exist in HEAD
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	runExportFilterImport(t, repo, bare, rules)

	// Check OLD_LICENSE doesn't appear anywhere in HEAD
	ls, err := gitx.Run(nil, bare, "ls-tree", "-r", "--name-only", "refs/heads/main")
	if err != nil {
		t.Fatalf("ls-tree: %v", err)
	}
	if strings.Contains(ls.Stdout, "OLD_LICENSE") {
		t.Fatalf("expected OLD_LICENSE to be excluded from HEAD, got:\n%s", ls.Stdout)
	}
}
