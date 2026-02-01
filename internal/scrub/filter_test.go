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

	if err := exp.Wait(); err != nil {
		_ = imp.Process.Kill()
		t.Fatalf("export wait: %v (%s)", err, expStderr.String())
	}
	if ferr := <-errCh; ferr != nil {
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
