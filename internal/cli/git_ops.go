package cli

import (
	"fmt"
	"strings"

	gitx "git-copy/internal/git"
)

func commitConfigOnHeadBranch(repoPath, headBranch, message string) error {
	clean, err := gitx.HasCleanWorktree(repoPath)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("working tree is not clean; commit or stash changes before running this command")
	}

	cur, _ := gitx.CurrentBranch(repoPath)
	if cur != "" && cur != headBranch {
		if _, err := gitx.Run(nil, repoPath, "checkout", headBranch); err != nil {
			return err
		}
		defer func() {
			_, _ = gitx.Run(nil, repoPath, "checkout", cur)
		}()
	}

	if _, err := gitx.Run(nil, repoPath, "add", ".git-copy/config.json", ".git-copy/.gitignore"); err != nil {
		return err
	}
	if _, err := gitx.Run(nil, repoPath, "commit", "-m", message); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			return nil
		}
		return err
	}
	return nil
}
