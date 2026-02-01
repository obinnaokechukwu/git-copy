package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type DaemonConfig struct {
	Roots         []string      `json:"roots"`
	PollInterval  time.Duration `json:"poll_interval"`
	CacheDir      string        `json:"cache_dir"`
	MaxConcurrent int           `json:"max_concurrent"`
	NotifyOnError bool          `json:"notify_on_error"`
}

func DefaultDaemonConfig() DaemonConfig {
	home, _ := os.UserHomeDir()
	cache := filepath.Join(home, ".cache", "git-copy")
	// Default roots: scan home directory for git-copy repos
	return DaemonConfig{
		Roots:         []string{home},
		PollInterval:  30 * time.Second,
		CacheDir:      cache,
		MaxConcurrent: 2,
		NotifyOnError: true,
	}
}

func DaemonConfigPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "git-copy", "daemon.json"), nil
}

func LoadDaemonConfig() (DaemonConfig, error) {
	path, err := DaemonConfigPath()
	if err != nil {
		return DaemonConfig{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultDaemonConfig(), nil
		}
		return DaemonConfig{}, err
	}
	var c DaemonConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return DaemonConfig{}, err
	}
	d := DefaultDaemonConfig()
	if c.PollInterval == 0 {
		c.PollInterval = d.PollInterval
	}
	if c.CacheDir == "" {
		c.CacheDir = d.CacheDir
	}
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = d.MaxConcurrent
	}
	return c, nil
}

func SaveDaemonConfig(c DaemonConfig) error {
	path, err := DaemonConfigPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// RegisterRepoRoot adds a repo path to the daemon's watch roots if not already present.
// This is called during git-copy init to enable auto-sync for the repo.
func RegisterRepoRoot(repoPath string) error {
	cfg, err := LoadDaemonConfig()
	if err != nil {
		return err
	}
	// Normalize path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	// Check if already registered
	for _, r := range cfg.Roots {
		if expandHome(r) == absPath {
			return nil // already registered
		}
	}
	cfg.Roots = append(cfg.Roots, absPath)
	return SaveDaemonConfig(cfg)
}

func expandHome(p string) string {
	if len(p) > 1 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
