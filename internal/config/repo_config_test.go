package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoConfig_SaveLoadValidate(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")

	cfg := DefaultConfig("obinnaokechukwu", "main")
	cfg.Targets = append(cfg.Targets, Target{
		Label:       "public",
		Provider:    "custom",
		Account:     "johndoe",
		RepoName:    "repo",
		RepoURL:     "/tmp/does-not-matter.git",
		Replacement: "johndoe",
	})

	if err := SaveRepoConfigToFile(p, cfg); err != nil {
		t.Fatalf("SaveRepoConfigToFile: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}

	cfg2, err := LoadRepoConfigFromFile(p)
	if err != nil {
		t.Fatalf("LoadRepoConfigFromFile: %v", err)
	}

	if cfg2.PrivateUsername != "obinnaokechukwu" {
		t.Fatalf("private username mismatch: %q", cfg2.PrivateUsername)
	}
	if cfg2.HeadBranch != "main" {
		t.Fatalf("head branch mismatch: %q", cfg2.HeadBranch)
	}
	if len(cfg2.Targets) != 1 || cfg2.Targets[0].Label != "public" {
		t.Fatalf("targets mismatch: %#v", cfg2.Targets)
	}
}

func TestRepoConfig_ValidateRejectsMissingFields(t *testing.T) {
	cfg := RepoConfig{Version: RepoConfigVersion}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for missing private_username")
	}
	cfg.PrivateUsername = "x"
	cfg.Targets = []Target{{Label: "t"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for missing target repo_url/account/repo_name")
	}
}

func TestDefaultConfig_ContainsExpectedExclusions(t *testing.T) {
	cfg := DefaultConfig("test", "main")

	// Build expected set from exported variables
	expected := map[string]bool{
		".git-copy/**": true,
		"CLAUDE.md":    true,
	}
	for _, p := range DefaultExcludedEnvFiles {
		expected[p] = true
	}
	for _, p := range DefaultExcludedSecrets {
		expected[p] = true
	}

	// Verify all expected patterns are present
	actual := make(map[string]bool)
	for _, p := range cfg.Defaults.Exclude {
		actual[p] = true
	}

	for p := range expected {
		if !actual[p] {
			t.Errorf("missing expected exclusion pattern: %q", p)
		}
	}

	// Verify key patterns exist
	keyPatterns := []string{".env", ".envrc", ".env.local", ".secrets", ".npmrc"}
	for _, p := range keyPatterns {
		if !actual[p] {
			t.Errorf("missing key exclusion pattern: %q", p)
		}
	}
}

func TestDefaultExcludedEnvFiles_ContainsExpectedPatterns(t *testing.T) {
	if len(DefaultExcludedEnvFiles) == 0 {
		t.Fatal("DefaultExcludedEnvFiles should not be empty")
	}

	required := []string{".env", ".envrc", ".env.local", ".direnv/"}
	actual := make(map[string]bool)
	for _, p := range DefaultExcludedEnvFiles {
		actual[p] = true
	}

	for _, r := range required {
		if !actual[r] {
			t.Errorf("DefaultExcludedEnvFiles missing required pattern: %q", r)
		}
	}
}

func TestDefaultExcludedSecrets_ContainsExpectedPatterns(t *testing.T) {
	if len(DefaultExcludedSecrets) == 0 {
		t.Fatal("DefaultExcludedSecrets should not be empty")
	}

	required := []string{".secrets", ".npmrc", ".netrc"}
	actual := make(map[string]bool)
	for _, p := range DefaultExcludedSecrets {
		actual[p] = true
	}

	for _, r := range required {
		if !actual[r] {
			t.Errorf("DefaultExcludedSecrets missing required pattern: %q", r)
		}
	}
}

func TestDefaultConfig_OptInIsEmpty(t *testing.T) {
	cfg := DefaultConfig("test", "main")

	if len(cfg.Defaults.OptIn) != 0 {
		t.Errorf("expected opt_in to be empty by default, got: %v", cfg.Defaults.OptIn)
	}
}

func TestDefaultConfig_ExcludeCountMatchesVariables(t *testing.T) {
	cfg := DefaultConfig("test", "main")

	// 2 (always excluded) + env files + secrets
	expectedCount := 2 + len(DefaultExcludedEnvFiles) + len(DefaultExcludedSecrets)
	actualCount := len(cfg.Defaults.Exclude)

	if actualCount != expectedCount {
		t.Errorf("exclude count mismatch: expected %d, got %d", expectedCount, actualCount)
	}
}
