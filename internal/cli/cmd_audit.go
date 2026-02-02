package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/obinnaokechukwu/git-copy/internal/audit"
	"github.com/obinnaokechukwu/git-copy/internal/config"
	"github.com/obinnaokechukwu/git-copy/internal/repo"
)

type multiStringFlag []string

func (m *multiStringFlag) String() string { return strings.Join(*m, ",") }
func (m *multiStringFlag) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	*m = append(*m, v)
	return nil
}

type auditArgs struct {
	repo   string
	target string
	remote bool
	// repeated
	strings multiStringFlag
}

func parseAuditArgs(args []string) (auditArgs, error) {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	var a auditArgs
	fs.StringVar(&a.repo, "repo", "", "path to repo (default: current directory)")
	fs.StringVar(&a.target, "target", "", "audit only this target label")
	fs.BoolVar(&a.remote, "remote", false, "also audit the remote mirror by cloning it")
	fs.Var(&a.strings, "string", "forbidden substring to search for (repeatable)")
	if err := fs.Parse(args); err != nil {
		return auditArgs{}, err
	}
	return a, nil
}

func cmdAudit(repoFlag, target string, remote bool, extraStrings []string) error {
	repoPath, err := resolveRepoPath(repoFlag)
	if err != nil {
		return err
	}
	cfg, err := repo.LoadRepoConfigFromAnyBranch(context.Background(), repoPath)
	if err != nil {
		return err
	}

	t, err := selectTarget(cfg, target)
	if err != nil {
		return err
	}

	opts := audit.DefaultOptions()
	opts.ForbiddenStrings = append(opts.ForbiddenStrings, cfg.PrivateUsername)
	opts.ForbiddenStrings = append(opts.ForbiddenStrings, extraStrings...)

	opts.ReplaceHistoryWithCurrentFiles = append([]string{}, cfg.Defaults.ReplaceHistoryWithCurrent...)
	opts.ReplaceHistoryWithCurrentFiles = append(opts.ReplaceHistoryWithCurrentFiles, t.ReplaceHistoryWithCurrent...)

	fmt.Printf("Audit target %q\n", t.Label)

	// Local scrubbed bare repo location.
	repoKey := repoCacheKey(repoPath)
	localBare := filepath.Join(defaultCacheDir(), repoKey, t.Label+".git")

	if _, err := os.Stat(localBare); err == nil {
		fmt.Printf("- Local scrubbed repo: %s\n", localBare)
		rep, err := audit.AuditBareRepo(context.Background(), localBare, opts)
		if err != nil {
			return err
		}
		printAuditReport(rep)
		if !rep.Succeeded {
			return errors.New("audit failed (local)")
		}
	} else {
		fmt.Printf("- Local scrubbed repo: (missing) %s\n", localBare)
		fmt.Println("  Tip: run `git-copy sync` first to generate the local scrubbed cache.")
	}

	if remote {
		fmt.Printf("- Remote repo: %s\n", t.RepoURL)
		clonePath, cleanup, err := audit.CloneMirrorToTemp(context.Background(), t.RepoURL, audit.CloneOptions{})
		if err != nil {
			return err
		}
		defer cleanup()
		rep, err := audit.AuditBareRepo(context.Background(), clonePath, opts)
		if err != nil {
			return err
		}
		printAuditReport(rep)
		if !rep.Succeeded {
			return errors.New("audit failed (remote)")
		}
	}

	fmt.Println("Audit: OK")
	return nil
}

func selectTarget(cfg config.RepoConfig, label string) (config.Target, error) {
	if label == "" {
		if len(cfg.Targets) == 1 {
			return cfg.Targets[0], nil
		}
		return config.Target{}, errors.New("usage: git-copy audit [--repo PATH] --target LABEL [--remote] [--string S ...]")
	}
	for _, t := range cfg.Targets {
		if t.Label == label {
			return t, nil
		}
	}
	return config.Target{}, fmt.Errorf("unknown target: %s", label)
}

func repoCacheKey(repoPath string) string {
	sum := sha256.Sum256([]byte(repoPath))
	return fmt.Sprintf("%x", sum[:8])
}

func defaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "git-copy")
}

func printAuditReport(rep audit.Report) {
	if rep.Succeeded {
		fmt.Println("  OK (no findings)")
		return
	}
	for _, f := range rep.Findings {
		path := f.Path
		if strings.TrimSpace(path) == "" {
			path = "(unknown)"
		}
		ref := f.Ref
		if strings.TrimSpace(ref) == "" {
			ref = "(unknown)"
		}
		fmt.Printf("  FAIL %-22s %s %s (%s)\n", f.Kind, ref, path, f.Detail)
	}
}

