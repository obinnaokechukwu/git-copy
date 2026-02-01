package provider

import (
	"context"
	"fmt"
)

type RepoURLs struct {
	SSH   string
	HTTPS string
}

type Provider interface {
	Name() string
	RepoExists(ctx context.Context, account, name string) (bool, error)
	CreatePrivateRepo(ctx context.Context, account, name, description string) (RepoURLs, error)
}

func ErrUnsupportedProvider(p string) error {
	return fmt.Errorf("unsupported provider: %s", p)
}
