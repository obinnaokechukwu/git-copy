package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"git-copy/internal/config"
	"git-copy/internal/notify"
	"git-copy/internal/repo"
	syncer "git-copy/internal/sync"
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
	log.Printf("git-copy daemon started (poll=%s roots=%v)", s.Config.PollInterval, s.Config.Roots)

	ticker := time.NewTicker(s.Config.PollInterval)
	defer ticker.Stop()

	sem := make(chan struct{}, s.Config.MaxConcurrent)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
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
					_, err = syncer.SyncRepo(ctx, rp, cfg, "", syncer.Options{CacheDir: s.Config.CacheDir, Validate: true})
					if err != nil {
						log.Printf("sync error [%s]: %v", rp, err)
						if s.Config.NotifyOnError {
							notify.Error("git-copy: sync error", fmt.Sprintf("%s: %v", rp, err))
						}
					}
				}()
			}
			wg.Wait()
		}
	}
}
