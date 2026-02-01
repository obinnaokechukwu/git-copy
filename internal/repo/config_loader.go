package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"git-copy/internal/config"
	gitx "git-copy/internal/git"
)

// LoadRepoConfigFromAnyBranch loads .git-copy/config.json from:
// 1) working tree, if present
// 2) head branch candidates main/master (via git show)
func LoadRepoConfigFromAnyBranch(ctx context.Context, repoPath string) (config.RepoConfig, error) {
	p := filepath.Join(repoPath, ".git-copy", "config.json")
	if b, err := os.ReadFile(p); err == nil {
		var c config.RepoConfig
		if err := json.Unmarshal(b, &c); err != nil {
			return config.RepoConfig{}, err
		}
		if err := c.Validate(); err != nil {
			return config.RepoConfig{}, err
		}
		return c, nil
	}

	for _, b := range []string{"main", "master"} {
		res, err := gitx.Run(ctx, repoPath, "show", b+":.git-copy/config.json")
		if err != nil {
			continue
		}
		var c config.RepoConfig
		if err := json.Unmarshal([]byte(res.Stdout), &c); err != nil {
			return config.RepoConfig{}, err
		}
		if err := c.Validate(); err != nil {
			return config.RepoConfig{}, err
		}
		return c, nil
	}

	return config.RepoConfig{}, fmt.Errorf("git-copy config not found in working tree or main/master")
}
