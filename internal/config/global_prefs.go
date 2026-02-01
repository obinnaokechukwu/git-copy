package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// GlobalPrefs stores user preferences that span across repositories.
type GlobalPrefs struct {
	// AccountEmails maps account/username to their preferred public email.
	AccountEmails map[string]string `json:"account_emails,omitempty"`
}

// GlobalPrefsPath returns the path to the global preferences file.
func GlobalPrefsPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "git-copy", "prefs.json")
}

// LoadGlobalPrefs loads preferences from the global config file.
// Returns empty prefs if file doesn't exist.
func LoadGlobalPrefs() GlobalPrefs {
	prefs := GlobalPrefs{
		AccountEmails: make(map[string]string),
	}
	data, err := os.ReadFile(GlobalPrefsPath())
	if err != nil {
		return prefs
	}
	_ = json.Unmarshal(data, &prefs)
	if prefs.AccountEmails == nil {
		prefs.AccountEmails = make(map[string]string)
	}
	return prefs
}

// Save writes the preferences to the global config file.
func (p GlobalPrefs) Save() error {
	path := GlobalPrefsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetAccountEmail returns the saved email for an account, or empty string if not found.
func (p GlobalPrefs) GetAccountEmail(account string) string {
	return p.AccountEmails[account]
}

// SetAccountEmail saves the email for an account.
func (p *GlobalPrefs) SetAccountEmail(account, email string) {
	if p.AccountEmails == nil {
		p.AccountEmails = make(map[string]string)
	}
	p.AccountEmails[account] = email
}
