package cli

import (
	"context"
	"fmt"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	"github.com/obinnaokechukwu/git-copy/internal/repo"
)

func cmdRemoveTarget(repoFlag, label string) error {
	repoPath, err := resolveRepoPath(repoFlag)
	if err != nil {
		return err
	}
	cfg, err := repo.LoadRepoConfigFromAnyBranch(context.Background(), repoPath)
	if err != nil {
		return err
	}
	out := make([]config.Target, 0, len(cfg.Targets))
	found := false
	for _, t := range cfg.Targets {
		if t.Label == label {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return fmt.Errorf("target not found: %s", label)
	}
	cfg.Targets = out

	confPath := config.RepoConfigPath(repoPath)
	if err := config.SaveRepoConfigToFile(confPath, cfg); err != nil {
		return err
	}
	if err := commitConfigOnHeadBranch(repoPath, cfg.HeadBranch, "Update git-copy configuration"); err != nil {
		return err
	}

	fmt.Printf("Removed target %q.\n", label)
	return nil
}
