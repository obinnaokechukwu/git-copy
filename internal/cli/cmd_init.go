package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git-copy/internal/config"
	gitx "git-copy/internal/git"
	"git-copy/internal/repo"
	"git-copy/internal/sync"
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

	privateUser, err := promptString("Private username to scrub (exact string)", "", true)
	if err != nil {
		return err
	}
	headBranch, err = promptString("Head branch (authoritative config branch)", headBranch, true)
	if err != nil {
		return err
	}
	privateUser = strings.TrimSpace(privateUser)
	headBranch = strings.TrimSpace(headBranch)

	cfg := config.DefaultConfig(privateUser, headBranch)

	target, err := interactiveTargetSetup(cfg)
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
