package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	gitx "git-copy/internal/git"
)

func resolveRepoPath(repoFlag string) (string, error) {
	var p string
	if repoFlag != "" {
		p = repoFlag
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		p = wd
	}
	top, err := gitx.RepoTopLevel(p)
	if err != nil {
		return "", err
	}
	return top, nil
}

func mustBeGitRepo(repoPath string) error {
	ok, err := gitx.IsGitRepo(repoPath)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("not a git repository: " + repoPath)
	}
	return nil
}

func normalizeLabel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func ensureGitCopyGitignore(repoPath string) error {
	p := filepath.Join(repoPath, ".git-copy", ".gitignore")
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	content := "# git-copy runtime state (never commit)\nstate.json\n\n# cache directories\ncache/\ntmp/\n"
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o600)
}
