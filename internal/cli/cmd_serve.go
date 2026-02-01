package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"git-copy/internal/config"
	"git-copy/internal/daemon"
)

func cmdServe() error {
	cfg, err := config.LoadDaemonConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()

	srv := &daemon.Server{Config: cfg}
	return srv.Run(ctx)
}
