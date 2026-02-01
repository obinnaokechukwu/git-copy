package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
	"github.com/obinnaokechukwu/git-copy/internal/repo"
	"github.com/obinnaokechukwu/git-copy/internal/sync"
)

func cmdInit(repoFlag string) error {
	repoPath, err := resolveRepoPath(repoFlag)
	if err != nil {
		return err
	}
	if err := mustBeGitRepo(repoPath); err != nil {
		return err
	}

	// Refuse if already initialized
	if _, err := os.Stat(filepath.Join(repoPath, ".git-copy", "config.json")); err == nil {
		return fmt.Errorf("git-copy already initialized in this repo (found .git-copy/config.json); use add-target instead")
	}
	if _, err := repo.LoadRepoConfigFromAnyBranch(context.Background(), repoPath); err == nil {
		return fmt.Errorf("git-copy config exists on main/master; checkout head branch or use add-target")
	}

	curBranch, _ := gitx.CurrentBranch(repoPath)
	headBranch := curBranch
	if headBranch == "" {
		headBranch = "main"
	}

	// Try to infer private username from origin remote URL
	defaultPrivateUser := getOriginUsername(repoPath)

	privateUser, err := promptString("Private username to scrub (exact string)", defaultPrivateUser, true)
	if err != nil {
		return err
	}
	headBranch, err = promptString("Head branch (authoritative config branch)", headBranch, true)
	if err != nil {
		return err
	}
	privateUser = strings.TrimSpace(privateUser)
	headBranch = strings.TrimSpace(headBranch)

	// Only check for clean worktree if we need to switch branches
	if curBranch != "" && curBranch != headBranch {
		clean, err := gitx.HasCleanWorktree(repoPath)
		if err != nil {
			return err
		}
		if !clean {
			return fmt.Errorf("working tree is not clean; commit or stash changes before running git-copy init (branch switch required: %s -> %s)", curBranch, headBranch)
		}
	}

	cfg := config.DefaultConfig(privateUser, headBranch)

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
	if err := commitConfigOnHeadBranch(repoPath, headBranch, "Add git-copy configuration"); err != nil {
		return err
	}

	fmt.Println("Initialized git-copy configuration.")
	fmt.Println("Running initial sync...")
	_, err = sync.SyncRepo(context.Background(), repoPath, cfg, target.Label, sync.Options{Validate: true})
	if err != nil {
		return err
	}
	fmt.Printf("Initial sync complete for target %q.\n", target.Label)
	fmt.Println("Note: Target repos are created as private. You choose if/when to make them public.")
	return nil
}
