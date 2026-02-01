package cli

import (
	"context"
	"fmt"

	"github.com/obinnaokechukwu/git-copy/internal/repo"
	"github.com/obinnaokechukwu/git-copy/internal/sync"
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
			fmt.Printf("%s: synced %s → %s\n", r.TargetLabel, r.SourceCommit, r.TargetURL)
		} else {
			fmt.Printf("%s: up to date (%s)\n", r.TargetLabel, r.SourceCommit)
		}
	}
	return nil
}
