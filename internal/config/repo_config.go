package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const RepoConfigVersion = 1

type RepoConfig struct {
	Version         int            `json:"version"`
	PrivateUsername string         `json:"private_username"`
	HeadBranch      string         `json:"head_branch"`
	Defaults        TargetDefaults `json:"defaults"`
	Targets         []Target       `json:"targets"`
}

type TargetDefaults struct {
	Exclude               []string          `json:"exclude"`
	OptIn                 []string          `json:"opt_in"`
	ExtraReplacementPairs map[string]string `json:"extra_replacements,omitempty"`
}

type Target struct {
	Label              string   `json:"label"`
	Provider           string   `json:"provider"`
	Account            string   `json:"account"`
	RepoName           string   `json:"repo_name"`
	RepoURL            string   `json:"repo_url"`
	Replacement        string   `json:"replacement,omitempty"`
	PublicAuthorName   string   `json:"public_author_name,omitempty"`
	PublicAuthorEmail  string   `json:"public_author_email,omitempty"`
	Exclude            []string `json:"exclude,omitempty"`
	OptIn              []string `json:"opt_in,omitempty"`
	Auth               AuthRef  `json:"auth,omitempty"`
	InitialHistoryMode string   `json:"initial_history_mode,omitempty"` // "full" or "future"
	InitialSyncAt      string   `json:"initial_sync_at,omitempty"`
}

type AuthRef struct {
	Method   string `json:"method,omitempty"`    // "gh", "token_env", "none"
	TokenEnv string `json:"token_env,omitempty"` // env var holding token (recommended)
	BaseURL  string `json:"base_url,omitempty"`  // provider API base URL, if needed
}

func DefaultConfig(privateUsername, headBranch string) RepoConfig {
	return RepoConfig{
		Version:         RepoConfigVersion,
		PrivateUsername: privateUsername,
		HeadBranch:      headBranch,
		Defaults: TargetDefaults{
			Exclude: []string{
				".git-copy/**",
				"CLAUDE.md",
				".env",
			},
			OptIn:                 []string{},
			ExtraReplacementPairs: map[string]string{},
		},
		Targets: []Target{},
	}
}

func (c *RepoConfig) Validate() error {
	if c.Version != RepoConfigVersion && c.Version != 0 {
		return fmt.Errorf("unsupported config version: %d", c.Version)
	}
	if strings.TrimSpace(c.PrivateUsername) == "" {
		return errors.New("private_username is required")
	}
	if strings.TrimSpace(c.HeadBranch) == "" {
		c.HeadBranch = "main"
	}
	seen := map[string]bool{}
	for i := range c.Targets {
		t := &c.Targets[i]
		t.Label = strings.TrimSpace(t.Label)
		if t.Label == "" {
			return fmt.Errorf("target[%d].label is required", i)
		}
		key := strings.ToLower(t.Label)
		if seen[key] {
			return fmt.Errorf("duplicate target label: %s", t.Label)
		}
		seen[key] = true
		if strings.TrimSpace(t.RepoURL) == "" {
			return fmt.Errorf("target[%s].repo_url is required", t.Label)
		}
		if strings.TrimSpace(t.Account) == "" {
			return fmt.Errorf("target[%s].account is required", t.Label)
		}
		if strings.TrimSpace(t.RepoName) == "" {
			return fmt.Errorf("target[%s].repo_name is required", t.Label)
		}
		if t.InitialHistoryMode == "" {
			t.InitialHistoryMode = "full"
		}
	}
	return nil
}

func RepoConfigPath(repoPath string) string {
	return filepath.Join(repoPath, ".git-copy", "config.json")
}

func LoadRepoConfigFromFile(path string) (RepoConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return RepoConfig{}, err
	}
	var c RepoConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return RepoConfig{}, err
	}
	if err := c.Validate(); err != nil {
		return RepoConfig{}, err
	}
	return c, nil
}

func SaveRepoConfigToFile(path string, c RepoConfig) error {
	if err := c.Validate(); err != nil {
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
