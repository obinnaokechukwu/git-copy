package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
)

type DiscoverOptions struct {
	Roots []string
}

// DiscoverRepos walks configured roots and returns unique repo toplevel paths that appear to be git repos
// and have git-copy config either in the working tree or on main/master.
func DiscoverRepos(ctx context.Context, opts DiscoverOptions) ([]string, error) {
	seen := map[string]bool{}
	var repos []string

	for _, root := range opts.Roots {
		root = expandHome(root)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() && (name == ".git" || name == ".hg" || name == ".svn") {
				// If .git dir found, parent is repo root
				if name == ".git" {
					repoRoot := filepath.Dir(p)
					if seen[repoRoot] {
						return filepath.SkipDir
					}
					ok, _ := gitx.IsGitRepo(repoRoot)
					if ok && hasGitCopyConfig(ctx, repoRoot) {
						seen[repoRoot] = true
						repos = append(repos, repoRoot)
					}
					return filepath.SkipDir
				}
				return filepath.SkipDir
			}
			// Skip extremely deep vendor dirs
			if d.IsDir() {
				if name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return repos, nil
}

func hasGitCopyConfig(ctx context.Context, repoRoot string) bool {
	// working tree
	if _, err := os.Stat(filepath.Join(repoRoot, ".git-copy", "config.json")); err == nil {
		return true
	}
	// try main/master without needing the config itself
	for _, b := range []string{"main", "master"} {
		_, err := gitx.Run(ctx, repoRoot, "show", b+":.git-copy/config.json")
		if err == nil {
			return true
		}
	}
	return false
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~/"))
	}
	return p
}
