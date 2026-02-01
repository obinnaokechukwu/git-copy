package scrub

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
)

type ValidationError struct {
	Reason string
}

func (e ValidationError) Error() string { return e.Reason }

func ValidateScrubbedRepo(ctx context.Context, bareRepoPath string, privateUsername string, forbiddenPaths []string) error {
	if privateUsername == "" {
		return nil
	}

	// 1) Ensure forbidden paths do not exist in any tree
	refs, err := gitx.ListRefs(bareRepoPath)
	if err != nil {
		return err
	}
	for ref := range refs {
		if !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/tags/") {
			continue
		}
		res, err := gitx.Run(ctx, bareRepoPath, "ls-tree", "-r", "--name-only", "--full-tree", ref)
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(strings.NewReader(res.Stdout))
		for sc.Scan() {
			p := strings.TrimSpace(sc.Text())
			if p == "" {
				continue
			}
			if IsNonNegotiablePath(p) {
				return ValidationError{Reason: "found forbidden path in target repo: " + p}
			}
			for _, bad := range forbiddenPaths {
				if bad != "" && p == bad {
					return ValidationError{Reason: "found forbidden path in target repo: " + p}
				}
			}
		}
	}

	// 2) Search all object contents for private username
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch-all-objects", "--batch")
	cmd.Dir = bareRepoPath
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	needleLower := bytes.ToLower([]byte(privateUsername))
	privateUsernameLower := strings.ToLower(privateUsername)
	br := bufio.NewReader(stdout)
	for {
		h, err := br.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = cmd.Process.Kill()
			return err
		}
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		parts := strings.Split(h, " ")
		if len(parts) < 3 {
			continue
		}
		var size int
		_, _ = fmt.Sscanf(parts[2], "%d", &size)
		if size < 0 {
			size = 0
		}
		buf := make([]byte, size)
		if _, err := io.ReadFull(br, buf); err != nil {
			_ = cmd.Process.Kill()
			return err
		}
		// Consume trailing newline after object payload
		_, _ = br.ReadByte()

		// Case-insensitive check for private username
		if bytes.Contains(bytes.ToLower(buf), needleLower) || strings.Contains(strings.ToLower(h), privateUsernameLower) {
			_ = cmd.Process.Kill()
			return ValidationError{Reason: "private username still present in scrubbed git objects"}
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git cat-file validation failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
