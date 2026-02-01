package cli

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
)

// getOriginRepoName extracts the repo name from the origin remote URL.
// Supports same formats as getOriginUsername.
func getOriginRepoName(repoPath string) string {
	res, err := gitx.Run(nil, repoPath, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(res.Stdout)
	if url == "" {
		return ""
	}

	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Get last path segment
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		// Handle SSH format git@host:user/repo
		if strings.Contains(name, ":") {
			subparts := strings.Split(name, ":")
			if len(subparts) > 1 {
				name = subparts[len(subparts)-1]
			}
		}
		return name
	}
	return ""
}

// getOriginUsername extracts the username/org from the origin remote URL.
// Supports:
//   - git@github.com:username/repo.git
//   - https://github.com/username/repo.git
//   - ssh://git@github.com/username/repo.git
func getOriginUsername(repoPath string) string {
	res, err := gitx.Run(nil, repoPath, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(res.Stdout)
	if url == "" {
		return ""
	}

	// SSH format: git@host:username/repo.git
	if strings.Contains(url, "@") && strings.Contains(url, ":") && !strings.Contains(url, "://") {
		re := regexp.MustCompile(`@[^:]+:([^/]+)/`)
		if m := re.FindStringSubmatch(url); len(m) > 1 {
			return m[1]
		}
	}

	// HTTPS/SSH URL format: https://host/username/repo.git or ssh://git@host/username/repo.git
	re := regexp.MustCompile(`://[^/]+/([^/]+)/`)
	if m := re.FindStringSubmatch(url); len(m) > 1 {
		return m[1]
	}

	return ""
}

// getRepoDescription gets the repo description using gh cli.
func getRepoDescription(repoPath string) string {
	cmd := exec.Command("gh", "repo", "view", "--json", "description", "-q", ".description")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func commitConfigOnHeadBranch(repoPath, headBranch, message string) error {
	cur, _ := gitx.CurrentBranch(repoPath)
	needsBranchSwitch := cur != "" && cur != headBranch

	// Only require clean worktree if we need to switch branches
	if needsBranchSwitch {
		clean, err := gitx.HasCleanWorktree(repoPath)
		if err != nil {
			return err
		}
		if !clean {
			return fmt.Errorf("working tree is not clean; commit or stash changes before running this command (branch switch required: %s -> %s)", cur, headBranch)
		}
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
