package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type GitHubProvider struct {
	// Auth method:
	// - if UseGHCLI and gh is available + authenticated, uses gh.
	// - else uses Token (PAT).
	UseGHCLI bool
	Token    string
	BaseURL  string // default https://api.github.com
}

func (p GitHubProvider) Name() string { return "github" }

func (p GitHubProvider) RepoExists(ctx context.Context, account, name string) (bool, error) {
	if p.UseGHCLI && ghAvailable() {
		return ghRepoExists(ctx, account, name)
	}
	return p.apiRepoExists(ctx, account, name)
}

func (p GitHubProvider) CreatePrivateRepo(ctx context.Context, account, name, description string) (RepoURLs, error) {
	if p.UseGHCLI && ghAvailable() {
		return ghCreatePrivateRepo(ctx, account, name, description)
	}
	return p.apiCreatePrivateRepo(ctx, account, name, description)
}

func ghAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// ghTokenForAccount retrieves the gh auth token for a specific account.
// This enables multi-account support where the user has authenticated
// multiple GitHub accounts with gh auth login.
func ghTokenForAccount(account string) string {
	cmd := exec.Command("gh", "auth", "token", "--user", account)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ghCommandForAccount creates an exec.Cmd with the correct GH_TOKEN
// environment variable set for the target account.
func ghCommandForAccount(ctx context.Context, account string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if token := ghTokenForAccount(account); token != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+token)
	}
	return cmd
}

func ghRepoExists(ctx context.Context, account, name string) (bool, error) {
	cmd := ghCommandForAccount(ctx, account, "repo", "view", fmt.Sprintf("%s/%s", account, name))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s := strings.ToLower(stderr.String())
		if strings.Contains(s, "could not resolve") || strings.Contains(s, "not found") || strings.Contains(s, "404") {
			return false, nil
		}
		// could be auth issue
		return false, fmt.Errorf("gh repo view failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return true, nil
}

func ghCreatePrivateRepo(ctx context.Context, account, name, description string) (RepoURLs, error) {
	full := fmt.Sprintf("%s/%s", account, name)
	args := []string{"repo", "create", full, "--private"}
	if strings.TrimSpace(description) != "" {
		args = append(args, "--description", description)
	}
	cmd := ghCommandForAccount(ctx, account, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return RepoURLs{}, fmt.Errorf("gh repo create failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	// Derive URLs (gh doesn't give structured output without json flag; keep simple)
	return RepoURLs{
		SSH:   fmt.Sprintf("git@github.com:%s/%s.git", account, name),
		HTTPS: fmt.Sprintf("https://github.com/%s/%s.git", account, name),
	}, nil
}

func (p GitHubProvider) apiRepoExists(ctx context.Context, account, name string) (bool, error) {
	if p.Token == "" {
		return false, errors.New("github token is required when gh is not available/authenticated")
	}
	base := p.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/repos/%s/%s", strings.TrimRight(base, "/"), account, name), nil)
	req.Header.Set("Authorization", "token "+p.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false, nil
	}
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("github api error: %s", resp.Status)
	}
	return true, nil
}

func (p GitHubProvider) apiCreatePrivateRepo(ctx context.Context, account, name, description string) (RepoURLs, error) {
	if p.Token == "" {
		return RepoURLs{}, errors.New("github token is required when gh is not available/authenticated")
	}
	base := p.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	// Try create under /user/repos (assumes token user matches account).
	body := map[string]any{
		"name":        name,
		"private":     true,
		"description": description,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/user/repos", bytes.NewReader(b))
	req.Header.Set("Authorization", "token "+p.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return RepoURLs{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return RepoURLs{}, fmt.Errorf("github api create repo error: %s", resp.Status)
	}
	var out struct {
		SSHURL   string `json:"ssh_url"`
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RepoURLs{}, err
	}
	ssh := out.SSHURL
	https := out.CloneURL
	if ssh == "" || https == "" {
		// Derive if missing
		ssh = fmt.Sprintf("git@github.com:%s/%s.git", account, name)
		https = fmt.Sprintf("https://github.com/%s/%s.git", account, name)
	}
	return RepoURLs{SSH: ssh, HTTPS: https}, nil
}

// Helper for optionally reading token from env and trying to locate gh
func GitHubTokenFromEnv(env string) string {
	if env == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(env))
}

func DefaultGitHubCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "git-copy", "github")
}

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), 30*time.Second)
	}
	return context.WithTimeout(ctx, 30*time.Second)
}
