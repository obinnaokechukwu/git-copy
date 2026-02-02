package audit

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
)

type Options struct {
	// ForbiddenPaths are paths that must not exist anywhere in reachable history.
	// These are evaluated using `git rev-list --all -- <path>`.
	ForbiddenPaths []string

	// ForbiddenStrings are substrings that must not exist in reachable blobs.
	// If CaseInsensitive is true, matching is done with a lowercased search.
	ForbiddenStrings []string
	CaseInsensitive  bool

	// ReplaceHistoryWithCurrentFiles are files that should appear to have the same
	// content throughout history as they do at HEAD.
	ReplaceHistoryWithCurrentFiles []string

	// MaxBlobBytes skips blobs larger than this size when scanning.
	MaxBlobBytes int64
	// MaxHits limits the number of findings per category (paths/strings).
	MaxHits int
}

type Finding struct {
	Kind string // "path-history" | "string-hit" | "replace-history-mismatch"

	Path string
	Ref  string // commit sha for history-based findings, blob sha for string hits

	Detail string
}

type Report struct {
	RepoPath  string
	Findings  []Finding
	Succeeded bool
}

func DefaultOptions() Options {
	return Options{
		ForbiddenPaths: []string{
			".git-copy",
			".claude",
			"CLAUDE.md",
			".env",
			".envrc",
		},
		ForbiddenStrings: []string{},
		CaseInsensitive:  true,
		MaxBlobBytes:     5 * 1024 * 1024,
		MaxHits:          20,
	}
}

func AuditBareRepo(ctx context.Context, bareRepoPath string, opts Options) (Report, error) {
	if strings.TrimSpace(bareRepoPath) == "" {
		return Report{}, errors.New("bareRepoPath is required")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
	}

	opts = normalizeOptions(opts)

	var findings []Finding

	// 1) Forbidden paths in reachable history.
	for _, p := range opts.ForbiddenPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		res, err := gitx.Run(ctx, bareRepoPath, "rev-list", "--all", "--", p)
		if err != nil {
			// If path never existed, git rev-list returns empty output with exit 0;
			// any errors are unexpected and should fail the audit.
			return Report{}, err
		}
		lines := nonEmptyLines(res.Stdout)
		if len(lines) == 0 {
			continue
		}
		for i, sha := range lines {
			if i >= opts.MaxHits {
				break
			}
			findings = append(findings, Finding{
				Kind:   "path-history",
				Path:   p,
				Ref:    sha,
				Detail: "path exists in reachable history",
			})
		}
	}

	// 2) Replace-history consistency checks.
	for _, p := range opts.ReplaceHistoryWithCurrentFiles {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		head, err := showFileAt(ctx, bareRepoPath, "HEAD", p)
		if err != nil {
			// If file isn't present at HEAD, skip.
			continue
		}
		firstSha, err := firstCommitTouchingPath(ctx, bareRepoPath, p)
		if err != nil || firstSha == "" {
			continue
		}
		firstContent, err := showFileAt(ctx, bareRepoPath, firstSha, p)
		if err != nil {
			continue
		}
		if !bytes.Equal(head, firstContent) {
			findings = append(findings, Finding{
				Kind:   "replace-history-mismatch",
				Path:   p,
				Ref:    firstSha,
				Detail: "file content at first introduction does not match HEAD",
			})
		}
	}

	// 3) Forbidden strings in reachable blobs.
	if len(opts.ForbiddenStrings) > 0 {
		blobFindings, err := scanReachableBlobsForStrings(ctx, bareRepoPath, opts)
		if err != nil {
			return Report{}, err
		}
		findings = append(findings, blobFindings...)
	}

	return Report{
		RepoPath:  bareRepoPath,
		Findings:  findings,
		Succeeded: len(findings) == 0,
	}, nil
}

type CloneOptions struct {
	Dir string // if empty, a temp dir is created
}

func CloneMirrorToTemp(ctx context.Context, remoteURL string, opts CloneOptions) (string, func(), error) {
	if strings.TrimSpace(remoteURL) == "" {
		return "", nil, errors.New("remoteURL is required")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
	}
	dir := opts.Dir
	cleanup := func() {}
	if dir == "" {
		tmp, err := os.MkdirTemp("", "git-copy-audit-*")
		if err != nil {
			return "", nil, err
		}
		dir = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
	}
	dst := filepath.Join(dir, "repo.git")

	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", remoteURL, dst)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git clone --mirror failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}

	// Best-effort: fetch PR refs if GitHub exposes them.
	_ = exec.CommandContext(ctx, "git", "-C", dst, "fetch", "origin",
		"+refs/pull/*/head:refs/pull/*/head",
		"+refs/pull/*/merge:refs/pull/*/merge",
	).Run()

	return dst, cleanup, nil
}

func normalizeOptions(opts Options) Options {
	if opts.MaxBlobBytes <= 0 {
		opts.MaxBlobBytes = DefaultOptions().MaxBlobBytes
	}
	if opts.MaxHits <= 0 {
		opts.MaxHits = DefaultOptions().MaxHits
	}
	// Normalize and de-dupe forbidden strings.
	seen := map[string]bool{}
	out := make([]string, 0, len(opts.ForbiddenStrings))
	for _, s := range opts.ForbiddenStrings {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := s
		if opts.CaseInsensitive {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	opts.ForbiddenStrings = out

	// Normalize file lists.
	opts.ForbiddenPaths = normalizeStringList(opts.ForbiddenPaths)
	opts.ReplaceHistoryWithCurrentFiles = normalizeStringList(opts.ReplaceHistoryWithCurrentFiles)
	return opts
}

func normalizeStringList(xs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

func nonEmptyLines(s string) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

func firstCommitTouchingPath(ctx context.Context, repoPath, p string) (string, error) {
	res, err := gitx.Run(ctx, repoPath, "rev-list", "--reverse", "--all", "--", p)
	if err != nil {
		return "", err
	}
	lines := nonEmptyLines(res.Stdout)
	if len(lines) == 0 {
		return "", nil
	}
	return lines[0], nil
}

func showFileAt(ctx context.Context, repoPath, ref, p string) ([]byte, error) {
	// git show prints to stdout; internal/git returns strings. Use exec directly to avoid encoding issues.
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "show", ref+":"+p)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git show %s:%s failed: %w\n%s", ref, p, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func scanReachableBlobsForStrings(ctx context.Context, repoPath string, opts Options) ([]Finding, error) {
	// Build sha->path map from `git rev-list --objects --all`.
	rev, err := gitx.Run(ctx, repoPath, "rev-list", "--objects", "--all")
	if err != nil {
		return nil, err
	}
	objToPath := map[string]string{}
	var objList strings.Builder
	sc := bufio.NewScanner(strings.NewReader(rev.Stdout))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		sha := parts[0]
		if sha == "" {
			continue
		}
		objList.WriteString(sha)
		objList.WriteString("\n")
		if len(parts) != 2 {
			continue
		}
		if _, ok := objToPath[sha]; ok {
			continue
		}
		objToPath[sha] = parts[1]
	}

	// Filter to blobs using batch-check.
	blobShas, err := listReachableBlobs(ctx, repoPath, objList.String(), opts.MaxBlobBytes)
	if err != nil {
		return nil, err
	}

	needles := make([][]byte, 0, len(opts.ForbiddenStrings))
	for _, s := range opts.ForbiddenStrings {
		if opts.CaseInsensitive {
			s = strings.ToLower(s)
		}
		needles = append(needles, []byte(s))
	}

	// Stream blob contents via `git cat-file --batch` and search.
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git cat-file --batch start failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}

	go func() {
		defer stdin.Close()
		for _, sha := range blobShas {
			_, _ = io.WriteString(stdin, sha+"\n")
		}
	}()

	r := bufio.NewReader(stdout)
	findings := []Finding{}
	hitCount := 0

	for range blobShas {
		// header: "<sha> <type> <size>\n" or "<sha> missing\n"
		h, err := r.ReadString('\n')
		if err != nil {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("git cat-file --batch read header failed: %w", err)
		}
		h = strings.TrimSpace(h)
		if strings.HasSuffix(h, " missing") {
			continue
		}
		parts := strings.Split(h, " ")
		if len(parts) < 3 {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("unexpected cat-file header: %q", h)
		}
		sha := parts[0]
		typ := parts[1]
		sizeStr := parts[2]
		if typ != "blob" {
			// Consume payload + newline anyway.
			sz := parseInt64(sizeStr)
			if _, err := io.CopyN(io.Discard, r, sz+1); err != nil {
				_ = cmd.Process.Kill()
				return nil, err
			}
			continue
		}
		sz := parseInt64(sizeStr)
		if sz < 0 {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("invalid blob size in header: %q", h)
		}

		payload := make([]byte, sz)
		if _, err := io.ReadFull(r, payload); err != nil {
			_ = cmd.Process.Kill()
			return nil, err
		}
		// trailing newline after payload
		if _, err := r.ReadByte(); err != nil {
			_ = cmd.Process.Kill()
			return nil, err
		}

		content := payload
		if opts.CaseInsensitive {
			content = bytes.ToLower(content)
		}
		for i, needle := range needles {
			if len(needle) == 0 {
				continue
			}
			if bytes.Contains(content, needle) {
				path := objToPath[sha]
				findings = append(findings, Finding{
					Kind:   "string-hit",
					Path:   path,
					Ref:    sha,
					Detail: fmt.Sprintf("contains forbidden string %q", opts.ForbiddenStrings[i]),
				})
				hitCount++
				break
			}
		}
		if hitCount >= opts.MaxHits {
			break
		}
	}

	_ = cmd.Wait()
	return findings, nil
}

func listReachableBlobs(ctx context.Context, repoPath, revListObjectsAllStdout string, maxBlobBytes int64) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "cat-file", "--batch-check=%(objectname) %(objecttype) %(objectsize)")
	cmd.Stdin = strings.NewReader(revListObjectsAllStdout)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git cat-file --batch-check failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	out := []string{}
	sc := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 3 {
			continue
		}
		if parts[1] != "blob" {
			continue
		}
		sz := parseInt64(parts[2])
		if sz < 0 {
			continue
		}
		if maxBlobBytes > 0 && sz > maxBlobBytes {
			continue
		}
		out = append(out, parts[0])
	}
	return out, nil
}

func parseInt64(s string) int64 {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return -1
		}
		n = n*10 + int64(ch-'0')
		if n < 0 {
			return -1
		}
	}
	return n
}
