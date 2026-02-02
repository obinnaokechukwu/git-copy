package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/obinnaokechukwu/git-copy/internal/config"
)

func Run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "help", "-h", "--help":
		printUsage()
		return nil
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		repo := fs.String("repo", "", "path to repo (default: current directory)")
		_ = fs.Parse(args[1:])
		return cmdInit(*repo)
	case "add-target":
		fs := flag.NewFlagSet("add-target", flag.ExitOnError)
		repo := fs.String("repo", "", "path to repo (default: current directory)")
		_ = fs.Parse(args[1:])
		return cmdAddTarget(*repo)
	case "remove-target":
		fs := flag.NewFlagSet("remove-target", flag.ExitOnError)
		repo := fs.String("repo", "", "path to repo (default: current directory)")
		_ = fs.Parse(args[1:])
		rest := fs.Args()
		if len(rest) != 1 {
			return errors.New("usage: git-copy remove-target <label> [--repo PATH]")
		}
		return cmdRemoveTarget(*repo, rest[0])
	case "list-targets":
		fs := flag.NewFlagSet("list-targets", flag.ExitOnError)
		repo := fs.String("repo", "", "path to repo (default: current directory)")
		_ = fs.Parse(args[1:])
		return cmdListTargets(*repo)
	case "sync":
		s, err := parseSyncArgs(args[1:])
		if err != nil {
			return err
		}
		return cmdSync(s.repo, s.target, syncCmdOptions{
			AuditAfterSync: s.audit,
			AuditRemote:    s.auditRemote,
		})
	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		repo := fs.String("repo", "", "path to repo (default: current directory)")
		_ = fs.Parse(args[1:])
		return cmdStatus(*repo)
	case "audit":
		a, err := parseAuditArgs(args[1:])
		if err != nil {
			return err
		}
		return cmdAudit(a.repo, a.target, a.remote, []string(a.strings))
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		_ = fs.Parse(args[1:])
		return cmdServe()
	case "roots":
		if len(args) < 2 {
			return errors.New("usage: git-copy roots <add|remove|list> ...")
		}
		return cmdRoots(args[1:])
	case "repos":
		return cmdRepos()
	case "install":
		fs := flag.NewFlagSet("install", flag.ExitOnError)
		uninstall := fs.Bool("uninstall", false, "uninstall the daemon service")
		_ = fs.Parse(args[1:])
		return cmdInstall(*uninstall)
	case "uninstall":
		return cmdInstall(true)
	case "show-defaults":
		return cmdShowDefaults()
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printUsage() {
	exe := filepath.Base(os.Args[0])
	fmt.Printf(`%s — scrubbed one-way replication from private git repos to public targets

Usage:
  %s init [--repo PATH]
  %s add-target [--repo PATH]
  %s remove-target <label> [--repo PATH]
  %s list-targets [--repo PATH]
  %s sync [--repo PATH] [--target LABEL] [--audit] [--audit-remote]
  %s status [--repo PATH]
  %s audit [--repo PATH] --target LABEL [--remote] [--string S ...]

Daemon:
  %s roots add <path>
  %s roots remove <path>
  %s roots list
  %s repos
  %s serve
  %s install [--uninstall]
  %s uninstall

Info:
  %s show-defaults

`, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe)
}

func cmdShowDefaults() error {
	fmt.Println("Default exclusions (add to opt_in in config.json to override):")
	fmt.Println("")
	fmt.Println("Environment files:")
	fmt.Printf("  %s\n", strings.Join(config.DefaultExcludedEnvFiles, ", "))
	fmt.Println("")
	fmt.Println("Secrets and credentials:")
	fmt.Printf("  %s\n", strings.Join(config.DefaultExcludedSecrets, ", "))
	fmt.Println("")
	fmt.Println("Always excluded:")
	fmt.Println("  .git-copy/**, CLAUDE.md")
	fmt.Println("")
	fmt.Println("To include a pattern, add it to defaults.opt_in in .git-copy/config.json:")
	fmt.Println(`  "opt_in": [".envrc", ".env.development"]`)
	return nil
}
