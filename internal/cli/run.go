package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
		fs := flag.NewFlagSet("sync", flag.ExitOnError)
		repo := fs.String("repo", "", "path to repo (default: current directory)")
		target := fs.String("target", "", "sync only this target label")
		_ = fs.Parse(args[1:])
		return cmdSync(*repo, *target)
	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		repo := fs.String("repo", "", "path to repo (default: current directory)")
		_ = fs.Parse(args[1:])
		return cmdStatus(*repo)
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
  %s sync [--repo PATH] [--target LABEL]
  %s status [--repo PATH]

Daemon:
  %s roots add <path>
  %s roots remove <path>
  %s roots list
  %s repos
  %s serve

`, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe, exe)
}
