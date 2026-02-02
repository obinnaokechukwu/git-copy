package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
)

func TestAuditBareRepo_FindsForbiddenPathHistory(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := gitx.Run(ctx, src, "init", "-b", "main"); err != nil {
		if _, err2 := gitx.Run(ctx, src, "init"); err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(ctx, src, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(ctx, src, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(ctx, src, "config", "user.email", "obinnaokechukwu@private.invalid")

	if err := os.WriteFile(filepath.Join(src, ".envrc"), []byte("export SECRET=1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(ctx, src, "add", ".envrc")
	_, _ = gitx.Run(ctx, src, "commit", "-m", "add envrc")

	// Remove it later; it should still be flagged in history.
	_, _ = gitx.Run(ctx, src, "rm", ".envrc")
	_, _ = gitx.Run(ctx, src, "commit", "-m", "remove envrc")

	bare := filepath.Join(tmp, "bare.git")
	if _, err := gitx.Run(ctx, "", "clone", "--bare", src, bare); err != nil {
		t.Fatalf("clone --bare: %v", err)
	}

	opts := DefaultOptions()
	opts.ForbiddenPaths = []string{".envrc"}
	opts.ForbiddenStrings = nil
	rep, err := AuditBareRepo(ctx, bare, opts)
	if err != nil {
		t.Fatalf("AuditBareRepo: %v", err)
	}
	if rep.Succeeded {
		t.Fatalf("expected audit to fail")
	}
	found := false
	for _, f := range rep.Findings {
		if f.Kind == "path-history" && f.Path == ".envrc" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a path-history finding for .envrc, got: %#v", rep.Findings)
	}
}

func TestAuditBareRepo_FindsForbiddenString(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := gitx.Run(ctx, src, "init", "-b", "main"); err != nil {
		if _, err2 := gitx.Run(ctx, src, "init"); err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(ctx, src, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(ctx, src, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(ctx, src, "config", "user.email", "obinnaokechukwu@private.invalid")

	if err := os.WriteFile(filepath.Join(src, "secret.txt"), []byte("hello Obinnaokechukwu\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(ctx, src, "add", "secret.txt")
	_, _ = gitx.Run(ctx, src, "commit", "-m", "add secret")

	bare := filepath.Join(tmp, "bare.git")
	if _, err := gitx.Run(ctx, "", "clone", "--bare", src, bare); err != nil {
		t.Fatalf("clone --bare: %v", err)
	}

	opts := DefaultOptions()
	opts.ForbiddenPaths = nil
	opts.ForbiddenStrings = []string{"obinnaokechukwu"}
	opts.CaseInsensitive = true
	opts.MaxBlobBytes = 1024 * 1024
	opts.MaxHits = 5
	rep, err := AuditBareRepo(ctx, bare, opts)
	if err != nil {
		t.Fatalf("AuditBareRepo: %v", err)
	}
	if rep.Succeeded {
		t.Fatalf("expected audit to fail")
	}
	found := false
	for _, f := range rep.Findings {
		if f.Kind == "string-hit" && f.Path == "secret.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a string-hit finding for secret.txt, got: %#v", rep.Findings)
	}
}

func TestAuditBareRepo_ReplaceHistoryMismatch(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := gitx.Run(ctx, src, "init", "-b", "main"); err != nil {
		if _, err2 := gitx.Run(ctx, src, "init"); err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(ctx, src, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(ctx, src, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(ctx, src, "config", "user.email", "obinnaokechukwu@private.invalid")

	if err := os.WriteFile(filepath.Join(src, "LICENSE"), []byte("MIT\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(ctx, src, "add", "LICENSE")
	_, _ = gitx.Run(ctx, src, "commit", "-m", "add license")

	if err := os.WriteFile(filepath.Join(src, "LICENSE"), []byte("Apache\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(ctx, src, "add", "LICENSE")
	_, _ = gitx.Run(ctx, src, "commit", "-m", "update license")

	bare := filepath.Join(tmp, "bare.git")
	if _, err := gitx.Run(ctx, "", "clone", "--bare", src, bare); err != nil {
		t.Fatalf("clone --bare: %v", err)
	}

	opts := DefaultOptions()
	opts.ForbiddenPaths = nil
	opts.ForbiddenStrings = nil
	opts.ReplaceHistoryWithCurrentFiles = []string{"LICENSE"}
	rep, err := AuditBareRepo(ctx, bare, opts)
	if err != nil {
		t.Fatalf("AuditBareRepo: %v", err)
	}
	if rep.Succeeded {
		t.Fatalf("expected audit to fail")
	}
	found := false
	for _, f := range rep.Findings {
		if f.Kind == "replace-history-mismatch" && f.Path == "LICENSE" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected replace-history-mismatch for LICENSE, got: %#v", rep.Findings)
	}
}

func TestAuditBareRepo_ReplaceHistoryOK(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := gitx.Run(ctx, src, "init", "-b", "main"); err != nil {
		if _, err2 := gitx.Run(ctx, src, "init"); err2 != nil {
			t.Fatalf("git init: %v", err2)
		}
		_, _ = gitx.Run(ctx, src, "checkout", "-b", "main")
	}
	_, _ = gitx.Run(ctx, src, "config", "user.name", "obinnaokechukwu")
	_, _ = gitx.Run(ctx, src, "config", "user.email", "obinnaokechukwu@private.invalid")

	if err := os.WriteFile(filepath.Join(src, "LICENSE"), []byte("Apache\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(ctx, src, "add", "LICENSE")
	_, _ = gitx.Run(ctx, src, "commit", "-m", "add license")

	bare := filepath.Join(tmp, "bare.git")
	if _, err := gitx.Run(ctx, "", "clone", "--bare", src, bare); err != nil {
		t.Fatalf("clone --bare: %v", err)
	}

	opts := DefaultOptions()
	opts.ForbiddenPaths = nil
	opts.ForbiddenStrings = nil
	opts.ReplaceHistoryWithCurrentFiles = []string{"LICENSE"}
	rep, err := AuditBareRepo(ctx, bare, opts)
	if err != nil {
		t.Fatalf("AuditBareRepo: %v", err)
	}
	if !rep.Succeeded {
		t.Fatalf("expected audit to succeed, got findings: %#v", rep.Findings)
	}
}

