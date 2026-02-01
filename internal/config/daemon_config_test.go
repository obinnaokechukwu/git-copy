package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonConfig_SaveLoad_IsolatedByXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	cfg := DefaultDaemonConfig()
	cfg.Roots = []string{"/a", "/b"}
	cfg.PollInterval = 123 * time.Millisecond
	cfg.CacheDir = filepath.Join(tmp, "cache")
	cfg.MaxConcurrent = 7
	cfg.NotifyOnError = false

	if err := SaveDaemonConfig(cfg); err != nil {
		t.Fatalf("SaveDaemonConfig: %v", err)
	}

	path, err := DaemonConfigPath()
	if err != nil {
		t.Fatalf("DaemonConfigPath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected daemon config file to exist: %v", err)
	}

	cfg2, err := LoadDaemonConfig()
	if err != nil {
		t.Fatalf("LoadDaemonConfig: %v", err)
	}

	if cfg2.PollInterval != cfg.PollInterval {
		t.Fatalf("poll mismatch: %v != %v", cfg2.PollInterval, cfg.PollInterval)
	}
	if cfg2.MaxConcurrent != 7 {
		t.Fatalf("max concurrent mismatch: %d", cfg2.MaxConcurrent)
	}
	if cfg2.NotifyOnError != false {
		t.Fatalf("notify mismatch: %v", cfg2.NotifyOnError)
	}
}
