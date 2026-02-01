package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
)

func initGitRepoForTest(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", "README.md")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "init")
	return repo
}

func TestLoadRepoConfigFromAnyBranch_WorkingTree(t *testing.T) {
	repo := initGitRepoForTest(t)

	cfg := config.DefaultConfig("obinnaokechukwu", "main")
	cfg.Targets = append(cfg.Targets, config.Target{
		Label:       "t",
		Provider:    "custom",
		Account:     "johndoe",
		RepoName:    "r",
		RepoURL:     "/tmp/x.git",
		Replacement: "johndoe",
	})
	confPath := config.RepoConfigPath(repo)
	if err := config.SaveRepoConfigToFile(confPath, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadRepoConfigFromAnyBranch(context.Background(), repo)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.PrivateUsername != "obinnaokechukwu" || loaded.HeadBranch != "main" {
		t.Fatalf("unexpected loaded config: %#v", loaded)
	}
}

func TestLoadRepoConfigFromAnyBranch_FallbackToGitShow(t *testing.T) {
	repo := initGitRepoForTest(t)

	cfg := config.DefaultConfig("obinnaokechukwu", "main")
	cfg.Targets = append(cfg.Targets, config.Target{
		Label:       "t",
		Provider:    "custom",
		Account:     "johndoe",
		RepoName:    "r",
		RepoURL:     "/tmp/x.git",
		Replacement: "johndoe",
	})
	confPath := config.RepoConfigPath(repo)
	if err := config.SaveRepoConfigToFile(confPath, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	_, _ = gitx.Run(nil, repo, "add", ".git-copy/config.json")
	_, _ = gitx.Run(nil, repo, "commit", "-m", "add config")
	if err := os.Remove(confPath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	loaded, err := LoadRepoConfigFromAnyBranch(context.Background(), repo)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.PrivateUsername != "obinnaokechukwu" || len(loaded.Targets) != 1 {
		t.Fatalf("unexpected loaded config: %#v", loaded)
	}
}
