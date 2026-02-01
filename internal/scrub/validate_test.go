package scrub

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitx "git-copy/internal/git"
)

func initRepo(t *testing.T, content string) string {
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
	_, _ = gitx.Run(nil, repo, "config", "user.name", "Private")
	_, _ = gitx.Run(nil, repo, "config", "user.email", "private@example.com")

	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "file.txt")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "msg")
	return repo
}

func TestValidateScrubbedRepo_FailsWhenUsernamePresent(t *testing.T) {
	repo := initRepo(t, "hello obinnaokechukwu")
	bare := filepath.Join(t.TempDir(), "bare.git")
	_, err := gitx.Run(nil, "", "clone", "--bare", repo, bare)
	if err != nil {
		t.Fatalf("clone --bare: %v", err)
	}

	err = ValidateScrubbedRepo(context.Background(), bare, "obinnaokechukwu", nil)
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateScrubbedRepo_PassesWhenClean(t *testing.T) {
	repo := initRepo(t, "hello world")
	bare := filepath.Join(t.TempDir(), "bare.git")
	_, err := gitx.Run(nil, "", "clone", "--bare", repo, bare)
	if err != nil {
		t.Fatalf("clone --bare: %v", err)
	}

	if err := ValidateScrubbedRepo(context.Background(), bare, "obinnaokechukwu", []string{".git-copy"}); err != nil {
		t.Fatalf("expected pass, got: %v", err)
	}
}
