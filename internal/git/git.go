package git

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type CmdResult struct {
	Stdout string
	Stderr string
}

func Run(ctx context.Context, dir string, args ...string) (CmdResult, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := CmdResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		return res, fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(res.Stderr))
	}
	return res, nil
}

func IsGitRepo(path string) (bool, error) {
	_, err := Run(nil, path, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return false, nil
		}
		return false, nil
	}
	return true, nil
}

func RepoTopLevel(path string) (string, error) {
	res, err := Run(nil, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

func CurrentBranch(repoPath string) (string, error) {
	res, err := Run(nil, repoPath, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", nil // detached
	}
	return strings.TrimSpace(res.Stdout), nil
}

func HasCleanWorktree(repoPath string) (bool, error) {
	res, err := Run(nil, repoPath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(res.Stdout) == "", nil
}

func FetchAll(repoPath string) error {
	_, err := Run(nil, repoPath, "fetch", "--all", "--prune")
	return err
}

func PullRebaseAutostash(repoPath string) error {
	_, err := Run(nil, repoPath, "pull", "--rebase", "--autostash")
	return err
}

func FastExportCmd(repoPath string, args ...string) *exec.Cmd {
	a := append([]string{"fast-export"}, args...)
	cmd := exec.Command("git", a...)
	cmd.Dir = repoPath
	return cmd
}

func FastImportCmd(bareRepoPath string) *exec.Cmd {
	cmd := exec.Command("git", "fast-import", "--force", "--quiet")
	cmd.Dir = bareRepoPath
	return cmd
}

func InitEmptyBare(path string) error {
	_, err := Run(nil, "", "init", "--bare", path)
	return err
}

func PushMirror(ctx context.Context, bareRepoPath, remoteURL string) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", "push", "--mirror", "--force", remoteURL)
	cmd.Dir = bareRepoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push --mirror failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func ListRefs(repoPath string) (map[string]string, error) {
	res, err := Run(nil, repoPath, "show-ref")
	if err != nil {
		// empty repo => show-ref exits nonzero; treat as empty
		if strings.Contains(strings.ToLower(err.Error()), "show-ref") {
			return map[string]string{}, nil
		}
		return nil, err
	}
	m := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(res.Stdout))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		m[parts[1]] = parts[0]
	}
	return m, nil
}

func HashRefs(refs map[string]string) string {
	keys := make([]string, 0, len(refs))
	for k := range refs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(refs[k])
		b.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", sum[:])
}
