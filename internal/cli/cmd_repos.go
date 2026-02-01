package cli

import (
	"context"
	"fmt"

	"git-copy/internal/config"
	"git-copy/internal/daemon"
)

func cmdRepos() error {
	cfg, err := config.LoadDaemonConfig()
	if err != nil {
		return err
	}
	repos, err := daemon.DiscoverRepos(context.Background(), daemon.DiscoverOptions{Roots: cfg.Roots})
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("(none)")
		return nil
	}
	for _, r := range repos {
		fmt.Println(r)
	}
	return nil
}
