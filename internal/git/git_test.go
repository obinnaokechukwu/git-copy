package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashRefs_Deterministic(t *testing.T) {
	refs1 := map[string]string{
		"refs/heads/main": "aaaa",
		"refs/tags/v1":    "bbbb",
	}
	refs2 := map[string]string{
		"refs/tags/v1":    "bbbb",
		"refs/heads/main": "aaaa",
	}
	if HashRefs(refs1) != HashRefs(refs2) {
		t.Fatalf("expected deterministic hash independent of map order")
	}
}

func TestRepoTopLevelAndIsGitRepo(t *testing.T) {
	tmp := t.TempDir()
	ok, err := IsGitRepo(tmp)
	if err != nil {
		t.Fatalf("IsGitRepo: %v", err)
	}
	if ok {
		t.Fatalf("expected non-repo")
	}

	repo := filepath.Join(tmp, "r")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err = Run(nil, repo, "init", "-b", "main")
	if err != nil {
		_, err2 := Run(nil, repo, "init")
		if err2 != nil {
			t.Fatalf("git init: %v (%v)", err, err2)
		}
		_, _ = Run(nil, repo, "checkout", "-b", "main")
	}

	top, err := RepoTopLevel(repo)
	if err != nil {
		t.Fatalf("RepoTopLevel: %v", err)
	}
	if top != repo {
		t.Fatalf("expected %q, got %q", repo, top)
	}

	ok, err = IsGitRepo(repo)
	if err != nil {
		t.Fatalf("IsGitRepo repo: %v", err)
	}
	if !ok {
		t.Fatalf("expected repo")
	}
}
