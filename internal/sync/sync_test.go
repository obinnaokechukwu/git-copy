package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
)

func TestSyncRepo_RebuildsWhenConfigChangesEvenIfRefsUnchanged(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	// Source repo with a LICENSE that changes over time.
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
	_, _ = gitx.Run(ctx, src, "commit", "-m", "Add LICENSE (MIT)")

	if err := os.WriteFile(filepath.Join(src, "LICENSE"), []byte("Apache-2.0\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = gitx.Run(ctx, src, "add", "LICENSE")
	_, _ = gitx.Run(ctx, src, "commit", "-m", "Update LICENSE (Apache)")

	// Destination bare repo acts like a "remote".
	dst := filepath.Join(tmp, "dst.git")
	if _, err := gitx.Run(ctx, tmp, "init", "--bare", dst); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}

	baseCfg := config.RepoConfig{
		Version:         config.RepoConfigVersion,
		PrivateUsername: "obinnaokechukwu",
		HeadBranch:      "main",
		Defaults: config.TargetDefaults{
			Exclude:               []string{".git-copy/**", ".claude/**"},
			OptIn:                 []string{},
			ExtraReplacementPairs: map[string]string{},
		},
		Targets: []config.Target{
			{
				Label:              "t",
				Provider:           "none",
				Account:            "public",
				RepoName:           "dst",
				RepoURL:            dst,
				InitialHistoryMode: "full",
			},
		},
	}

	// First sync (no replace_history_with_current): should preserve both commits.
	if _, err := SyncRepo(ctx, src, baseCfg, "", Options{CacheDir: filepath.Join(tmp, "cache")}); err != nil {
		t.Fatalf("SyncRepo: %v", err)
	}
	log1, err := gitx.Run(ctx, dst, "log", "--oneline", "--all")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if got := len(nonEmptyLines(log1.Stdout)); got != 2 {
		t.Fatalf("expected 2 commits before config change, got %d:\n%s", got, log1.Stdout)
	}

	// Change config: enable replace_history_with_current for LICENSE.
	updatedCfg := baseCfg
	updatedCfg.Defaults.ReplaceHistoryWithCurrent = []string{"LICENSE"}

	// No new commits in src, but config changed => must rebuild and push.
	if _, err := SyncRepo(ctx, src, updatedCfg, "", Options{CacheDir: filepath.Join(tmp, "cache")}); err != nil {
		t.Fatalf("SyncRepo (after config change): %v", err)
	}

	log2, err := gitx.Run(ctx, dst, "log", "--oneline", "--all")
	if err != nil {
		t.Fatalf("log2: %v", err)
	}
	if got := len(nonEmptyLines(log2.Stdout)); got != 1 {
		t.Fatalf("expected 1 commit after replace_history_with_current, got %d:\n%s", got, log2.Stdout)
	}

	// LICENSE should be the current (Apache) content at the remaining commit.
	show, err := gitx.Run(ctx, dst, "show", "refs/heads/main:LICENSE")
	if err != nil {
		t.Fatalf("show LICENSE: %v", err)
	}
	if strings.TrimSpace(show.Stdout) != "Apache-2.0" {
		t.Fatalf("expected LICENSE to be replaced with current content, got %q", show.Stdout)
	}
}

func nonEmptyLines(s string) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

