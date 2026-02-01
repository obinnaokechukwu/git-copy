package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	"github.com/obinnaokechukwu/git-copy/internal/notify"
	"github.com/obinnaokechukwu/git-copy/internal/repo"
	syncer "github.com/obinnaokechukwu/git-copy/internal/sync"
)

type Server struct {
	Config config.DaemonConfig
}

func (s *Server) Run(ctx context.Context) error {
	if s.Config.PollInterval <= 0 {
		s.Config.PollInterval = 30 * time.Second
	}
	if s.Config.MaxConcurrent <= 0 {
		s.Config.MaxConcurrent = 2
	}

	log.Println("========================================")
	log.Println("git-copy daemon starting")
	log.Printf("  Poll interval: %s", s.Config.PollInterval)
	log.Printf("  Cache dir: %s", s.Config.CacheDir)
	log.Printf("  Watch roots: %v", s.Config.Roots)
	if len(s.Config.Roots) == 0 {
		log.Println("  WARNING: No roots configured! Run 'git-copy init' in a repo to register it.")
	}
	log.Println("========================================")

	// Do initial discovery
	repos, _ := DiscoverRepos(ctx, DiscoverOptions{Roots: s.Config.Roots})
	if len(repos) > 0 {
		log.Printf("Found %d git-copy repo(s):", len(repos))
		for _, r := range repos {
			log.Printf("  - %s", r)
		}
	}

	ticker := time.NewTicker(s.Config.PollInterval)
	defer ticker.Stop()

	sem := make(chan struct{}, s.Config.MaxConcurrent)
	for {
		select {
		case <-ctx.Done():
			log.Println("git-copy daemon shutting down")
			return nil
		case <-ticker.C:
			// Reload config to pick up newly registered repos
			if newCfg, err := config.LoadDaemonConfig(); err == nil {
				s.Config = newCfg
			}
			repos, err := DiscoverRepos(ctx, DiscoverOptions{Roots: s.Config.Roots})
			if err != nil {
				log.Printf("discover error: %v", err)
				continue
			}
			var wg sync.WaitGroup
			for _, rp := range repos {
				rp := rp
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					cfg, err := repo.LoadRepoConfigFromAnyBranch(ctx, rp)
					if err != nil {
						log.Printf("config load error [%s]: %v", rp, err)
						if s.Config.NotifyOnError {
							notify.Error("git-copy: config error", fmt.Sprintf("%s: %v", rp, err))
						}
						return
					}
					results, err := syncer.SyncRepo(ctx, rp, cfg, "", syncer.Options{CacheDir: s.Config.CacheDir, Validate: true})
					if err != nil {
						log.Printf("sync error [%s]: %v", rp, err)
						if s.Config.NotifyOnError {
							notify.Error("git-copy: sync error", fmt.Sprintf("%s: %v", rp, err))
						}
						return
					}
					for _, r := range results {
						if r.Error != nil {
							log.Printf("[%s] %s: ERROR %v", rp, r.TargetLabel, r.Error)
						} else if r.DidWork {
							log.Printf("[%s] %s: synced %s → %s", rp, r.TargetLabel, r.SourceCommit, r.TargetURL)
						}
					}
				}()
			}
			wg.Wait()
		}
	}
}
