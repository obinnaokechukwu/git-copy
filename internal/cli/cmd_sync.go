package cli

import (
	"context"
	"fmt"

	"git-copy/internal/repo"
	"git-copy/internal/sync"
)

func cmdSync(repoFlag, target string) error {
	repoPath, err := resolveRepoPath(repoFlag)
	if err != nil {
		return err
	}
	cfg, err := repo.LoadRepoConfigFromAnyBranch(context.Background(), repoPath)
	if err != nil {
		return err
	}
	results, err := sync.SyncRepo(context.Background(), repoPath, cfg, target, sync.Options{Validate: true})
	if err != nil {
		return err
	}
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("%s: ERROR: %v\n", r.TargetLabel, r.Error)
		} else if r.DidWork {
			fmt.Printf("%s: synced\n", r.TargetLabel)
		} else {
			fmt.Printf("%s: up to date\n", r.TargetLabel)
		}
	}
	return nil
}
