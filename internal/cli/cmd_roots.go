package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"git-copy/internal/config"
)

func cmdRoots(args []string) error {
	switch args[0] {
	case "add":
		if len(args) != 2 {
			return errors.New("usage: git-copy roots add <path>")
		}
		p, _ := filepath.Abs(args[1])
		cfg, err := config.LoadDaemonConfig()
		if err != nil {
			return err
		}
		for _, r := range cfg.Roots {
			if r == p {
				fmt.Println("Already present.")
				return nil
			}
		}
		cfg.Roots = append(cfg.Roots, p)
		if err := config.SaveDaemonConfig(cfg); err != nil {
			return err
		}
		fmt.Println("Added:", p)
		return nil
	case "remove":
		if len(args) != 2 {
			return errors.New("usage: git-copy roots remove <path>")
		}
		p, _ := filepath.Abs(args[1])
		cfg, err := config.LoadDaemonConfig()
		if err != nil {
			return err
		}
		out := []string{}
		for _, r := range cfg.Roots {
			if r != p {
				out = append(out, r)
			}
		}
		cfg.Roots = out
		if err := config.SaveDaemonConfig(cfg); err != nil {
			return err
		}
		fmt.Println("Removed:", p)
		return nil
	case "list":
		cfg, err := config.LoadDaemonConfig()
		if err != nil {
			return err
		}
		if len(cfg.Roots) == 0 {
			fmt.Println("(none)")
			return nil
		}
		for _, r := range cfg.Roots {
			fmt.Println(r)
		}
		return nil
	default:
		return errors.New("usage: git-copy roots <add|remove|list> ...")
	}
}
