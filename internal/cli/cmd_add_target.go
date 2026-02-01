package cli

import (
	"context"
	"fmt"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
	"github.com/obinnaokechukwu/git-copy/internal/repo"
	"github.com/obinnaokechukwu/git-copy/internal/sync"
)

func cmdAddTarget(repoFlag string) error {
	repoPath, err := resolveRepoPath(repoFlag)
	if err != nil {
		return err
	}
	if err := mustBeGitRepo(repoPath); err != nil {
		return err
	}
	cfg, err := repo.LoadRepoConfigFromAnyBranch(context.Background(), repoPath)
	if err != nil {
		return fmt.Errorf("repo is not initialized for git-copy: %w", err)
	}

	// Only check for clean worktree if we need to switch branches
	curBranch, _ := gitx.CurrentBranch(repoPath)
	if curBranch != "" && curBranch != cfg.HeadBranch {
		clean, err := gitx.HasCleanWorktree(repoPath)
		if err != nil {
			return err
		}
		if !clean {
			return fmt.Errorf("working tree is not clean; commit or stash changes before running git-copy add-target (branch switch required: %s -> %s)", curBranch, cfg.HeadBranch)
		}
	}

	target, err := interactiveTargetSetup(cfg, repoPath)
	if err != nil {
		return err
	}
	cfg.Targets = append(cfg.Targets, target)

	confPath := config.RepoConfigPath(repoPath)
	if err := config.SaveRepoConfigToFile(confPath, cfg); err != nil {
		return err
	}
	if err := ensureGitCopyGitignore(repoPath); err != nil {
		return err
	}
	if err := commitConfigOnHeadBranch(repoPath, cfg.HeadBranch, "Update git-copy configuration"); err != nil {
		return err
	}

	fmt.Println("Added target. Running initial sync...")
	_, err = sync.SyncRepo(context.Background(), repoPath, cfg, target.Label, sync.Options{Validate: true})
	if err != nil {
		return err
	}
	fmt.Printf("Initial sync complete for target %q.\n", target.Label)
	return nil
}
