package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	gitx "github.com/obinnaokechukwu/git-copy/internal/git"
	"github.com/obinnaokechukwu/git-copy/internal/scrub"
	"github.com/obinnaokechukwu/git-copy/internal/state"
)

type Options struct {
	CacheDir string
	Validate bool
}

type Result struct {
	TargetLabel string
	DidWork     bool
	Error       error
}

func SyncRepo(ctx context.Context, repoPath string, cfg config.RepoConfig, onlyTarget string, opts Options) ([]Result, error) {
	if opts.CacheDir == "" {
		opts.CacheDir = defaultCacheDir()
	}
	if opts.Validate == false {
		opts.Validate = true
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// update private repo first (best-effort)
	clean, _ := gitx.HasCleanWorktree(repoPath)
	if clean {
		_ = gitx.PullRebaseAutostash(repoPath)
	} else {
		_ = gitx.FetchAll(repoPath)
	}

	privateRefs, err := gitx.ListRefs(repoPath)
	if err != nil {
		return nil, err
	}
	privateRefsHash := gitx.HashRefs(privateRefs)

	st, _ := state.Load(repoPath)
	if st.Targets == nil {
		st.Targets = map[string]*state.TargetState{}
	}

	repoKey := repoCacheKey(repoPath)
	results := []Result{}

	for _, t := range cfg.Targets {
		if onlyTarget != "" && t.Label != onlyTarget {
			continue
		}
		ts := st.Targets[t.Label]
		if ts == nil {
			ts = &state.TargetState{}
			st.Targets[t.Label] = ts
		}
		// Skip if private refs unchanged and last sync succeeded
		if ts.LastPrivateRefs == privateRefsHash && ts.LastError == "" {
			results = append(results, Result{TargetLabel: t.Label, DidWork: false, Error: nil})
			continue
		}

		res := Result{TargetLabel: t.Label, DidWork: true}

		err := syncTarget(ctx, repoPath, repoKey, cfg, t, opts)
		if err != nil {
			res.Error = err
			ts.LastError = err.Error()
		} else {
			ts.LastError = ""
			ts.LastSyncAt = time.Now()
			ts.LastPrivateRefs = privateRefsHash
		}
		results = append(results, res)
		_ = state.Save(repoPath, st)
	}

	return results, nil
}

func syncTarget(ctx context.Context, repoPath, repoKey string, cfg config.RepoConfig, t config.Target, opts Options) error {
	// Build rules
	repl := t.Replacement
	if repl == "" {
		repl = t.Account
	}
	exclude := append([]string{}, cfg.Defaults.Exclude...)
	exclude = append(exclude, t.Exclude...)
	optIn := append([]string{}, cfg.Defaults.OptIn...)
	optIn = append(optIn, t.OptIn...)

	rules, err := scrub.Compile(scrub.Rules{
		PrivateUsername:   cfg.PrivateUsername,
		Replacement:       repl,
		ExtraReplacements: cfg.Defaults.ExtraReplacementPairs,
		ExcludePatterns:   exclude,
		OptInPaths:        optIn,
		PublicAuthorName:  t.PublicAuthorName,
		PublicAuthorEmail: t.PublicAuthorEmail,
	})
	if err != nil {
		return err
	}

	cacheDir := filepath.Join(opts.CacheDir, repoKey)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	finalBare := filepath.Join(cacheDir, t.Label+".git")
	tmpBare := filepath.Join(cacheDir, t.Label+".tmp.git")

	_ = os.RemoveAll(tmpBare)
	if err := gitx.InitEmptyBare(tmpBare); err != nil {
		return err
	}

	// Run fast-export -> filter -> fast-import
	if err := exportFilterImport(ctx, repoPath, tmpBare, rules); err != nil {
		_ = os.RemoveAll(tmpBare)
		return err
	}

	// Validate invariants before pushing
	if opts.Validate {
		// exact-path checks; patterns are already excluded
		forbidden := []string{}
		if !contains(optIn, ".env") {
			forbidden = append(forbidden, ".env")
		}
		if !contains(optIn, "CLAUDE.md") {
			forbidden = append(forbidden, "CLAUDE.md")
		}
		if err := scrub.ValidateScrubbedRepo(ctx, tmpBare, cfg.PrivateUsername, forbidden); err != nil {
			_ = os.RemoveAll(tmpBare)
			return err
		}
	}

	// Atomically replace cache
	_ = os.RemoveAll(finalBare)
	if err := os.Rename(tmpBare, finalBare); err != nil {
		return fmt.Errorf("failed to move scrubbed repo into place: %w", err)
	}

	// Push mirror
	if err := gitx.PushMirror(ctx, finalBare, t.RepoURL); err != nil {
		return err
	}
	return nil
}

func exportFilterImport(ctx context.Context, srcRepo, dstBare string, rules scrub.CompiledRules) error {
	// Fast-export
	exp := gitx.FastExportCmd(srcRepo, "--all", "--signed-tags=strip", "--tag-of-filtered-object=rewrite")
	expStdout, err := exp.StdoutPipe()
	if err != nil {
		return err
	}
	var expStderr bytes.Buffer
	exp.Stderr = &expStderr

	// Fast-import
	imp := gitx.FastImportCmd(dstBare)
	impStdin, err := imp.StdinPipe()
	if err != nil {
		return err
	}
	var impStderr bytes.Buffer
	imp.Stderr = &impStderr

	if err := imp.Start(); err != nil {
		return fmt.Errorf("fast-import start failed: %w (%s)", err, strings.TrimSpace(impStderr.String()))
	}
	if err := exp.Start(); err != nil {
		_ = imp.Process.Kill()
		return fmt.Errorf("fast-export start failed: %w (%s)", err, strings.TrimSpace(expStderr.String()))
	}

	filter := scrub.NewExportFilter(rules)
	filterErrCh := make(chan error, 1)
	go func() {
		defer impStdin.Close()
		filterErrCh <- filter.Filter(expStdout, impStdin)
	}()

	if err := exp.Wait(); err != nil {
		_ = imp.Process.Kill()
		return fmt.Errorf("fast-export failed: %w (%s)", err, strings.TrimSpace(expStderr.String()))
	}
	if ferr := <-filterErrCh; ferr != nil {
		_ = imp.Process.Kill()
		return fmt.Errorf("export filter failed: %w", ferr)
	}
	if err := imp.Wait(); err != nil {
		return fmt.Errorf("fast-import failed: %w (%s)", err, strings.TrimSpace(impStderr.String()))
	}
	_, _ = gitx.Run(ctx, dstBare, "repack", "-adq")
	return nil
}

func repoCacheKey(repoPath string) string {
	sum := sha256.Sum256([]byte(repoPath))
	return hex.EncodeToString(sum[:8])
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func defaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "git-copy")
}
