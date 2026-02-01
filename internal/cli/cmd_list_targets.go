package cli

import (
	"context"
	"fmt"

	"github.com/obinnaokechukwu/git-copy/internal/repo"
)

func cmdListTargets(repoFlag string) error {
	repoPath, err := resolveRepoPath(repoFlag)
	if err != nil {
		return err
	}
	cfg, err := repo.LoadRepoConfigFromAnyBranch(context.Background(), repoPath)
	if err != nil {
		return err
	}
	fmt.Printf("Private username: %s\nHead branch: %s\n\nTargets:\n", cfg.PrivateUsername, cfg.HeadBranch)
	for _, t := range cfg.Targets {
		fmt.Printf("- %s (%s) %s/%s -> %s\n", t.Label, t.Provider, t.Account, t.RepoName, t.RepoURL)
	}
	return nil
}
