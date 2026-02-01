package cli

import (
	"context"
	"fmt"

	"git-copy/internal/repo"
	"git-copy/internal/state"
)

func cmdStatus(repoFlag string) error {
	repoPath, err := resolveRepoPath(repoFlag)
	if err != nil {
		return err
	}
	cfg, err := repo.LoadRepoConfigFromAnyBranch(context.Background(), repoPath)
	if err != nil {
		return err
	}
	st, err := state.Load(repoPath)
	if err != nil {
		return err
	}
	fmt.Printf("Repo: %s\n", repoPath)
	for _, t := range cfg.Targets {
		ts := st.Targets[t.Label]
		if ts == nil {
			fmt.Printf("- %s: never synced\n", t.Label)
			continue
		}
		if ts.LastError != "" {
			fmt.Printf("- %s: ERROR (%s)\n", t.Label, ts.LastError)
		} else {
			fmt.Printf("- %s: ok (last sync %s)\n", t.Label, ts.LastSyncAt.Format("2006-01-02 15:04:05"))
		}
	}
	return nil
}
