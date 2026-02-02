package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/obinnaokechukwu/git-copy/internal/audit"
	"github.com/obinnaokechukwu/git-copy/internal/config"
	"github.com/obinnaokechukwu/git-copy/internal/repo"
	"github.com/obinnaokechukwu/git-copy/internal/sync"
)

type syncCmdOptions struct {
	AuditAfterSync bool
	AuditRemote    bool
}

func cmdSync(repoFlag, target string, opts syncCmdOptions) error {
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

	targetByLabel := make(map[string]config.Target, len(cfg.Targets))
	for _, t := range cfg.Targets {
		targetByLabel[t.Label] = t
	}

	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("%s: ERROR: %v\n", r.TargetLabel, r.Error)
			continue
		} else if r.DidWork {
			fmt.Printf("%s: synced %s → %s\n", r.TargetLabel, r.SourceCommit, r.TargetURL)
		} else {
			fmt.Printf("%s: up to date (%s)\n", r.TargetLabel, r.SourceCommit)
		}

		if !opts.AuditAfterSync {
			continue
		}
		t, ok := targetByLabel[r.TargetLabel]
		if !ok {
			return fmt.Errorf("internal error: missing target config for %q", r.TargetLabel)
		}

		aopts := audit.DefaultOptions()
		aopts.ForbiddenStrings = append(aopts.ForbiddenStrings, cfg.PrivateUsername)
		aopts.ReplaceHistoryWithCurrentFiles = append([]string{}, cfg.Defaults.ReplaceHistoryWithCurrent...)
		aopts.ReplaceHistoryWithCurrentFiles = append(aopts.ReplaceHistoryWithCurrentFiles, t.ReplaceHistoryWithCurrent...)

		repoKey := repoCacheKey(repoPath)
		localBare := filepath.Join(defaultCacheDir(), repoKey, t.Label+".git")

		fmt.Printf("%s: audit (local)\n", r.TargetLabel)
		rep, err := audit.AuditBareRepo(context.Background(), localBare, aopts)
		if err != nil {
			return err
		}
		printAuditReport(rep)
		if !rep.Succeeded {
			return errors.New("audit failed (local)")
		}

		if opts.AuditRemote {
			fmt.Printf("%s: audit (remote)\n", r.TargetLabel)
			clonePath, cleanup, err := audit.CloneMirrorToTemp(context.Background(), t.RepoURL, audit.CloneOptions{})
			if err != nil {
				return err
			}
			var remoteErr error
			func() {
				defer cleanup()
				rrep, rerr := audit.AuditBareRepo(context.Background(), clonePath, aopts)
				if rerr != nil {
					remoteErr = rerr
					return
				}
				printAuditReport(rrep)
				if !rrep.Succeeded {
					remoteErr = errors.New("audit failed (remote)")
				}
			}()
			if remoteErr != nil {
				return remoteErr
			}
		}
	}
	return nil
}
