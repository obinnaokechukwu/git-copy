package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GiteaProvider struct {
	BaseURL string // e.g. https://gitea.example.com
	Token   string
}

func (p GiteaProvider) Name() string { return "gitea" }

func (p GiteaProvider) apiBase() string {
	b := strings.TrimRight(p.BaseURL, "/")
	if b == "" {
		return ""
	}
	return b + "/api/v1"
}

func (p GiteaProvider) RepoExists(ctx context.Context, account, name string) (bool, error) {
	if p.Token == "" {
		return false, errors.New("gitea token is required")
	}
	if p.apiBase() == "" {
		return false, errors.New("gitea base_url is required")
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/repos/%s/%s", p.apiBase(), account, name), nil)
	req.Header.Set("Authorization", "token "+p.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false, nil
	}
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("gitea api error: %s", resp.Status)
	}
	return true, nil
}

// isOrganization checks if the account is an organization.
func (p GiteaProvider) isOrganization(ctx context.Context, account string) bool {
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/orgs/%s", p.apiBase(), account), nil)
	req.Header.Set("Authorization", "token "+p.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// getAuthenticatedUser returns the username of the authenticated user.
func (p GiteaProvider) getAuthenticatedUser(ctx context.Context) string {
	req, _ := http.NewRequestWithContext(ctx, "GET", p.apiBase()+"/user", nil)
	req.Header.Set("Authorization", "token "+p.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var u struct {
			Login string `json:"login"`
		}
		if json.NewDecoder(resp.Body).Decode(&u) == nil {
			return u.Login
		}
	}
	return ""
}

func (p GiteaProvider) CreatePrivateRepo(ctx context.Context, account, name, description string) (RepoURLs, error) {
	if p.Token == "" {
		return RepoURLs{}, errors.New("gitea token is required")
	}
	if p.apiBase() == "" {
		return RepoURLs{}, errors.New("gitea base_url is required")
	}
	body := map[string]any{
		"name":        name,
		"private":     true,
		"description": description,
	}
	b, _ := json.Marshal(body)

	// Determine endpoint: org repo or user repo
	var endpoint string
	authUser := p.getAuthenticatedUser(ctx)
	if account != "" && account != authUser && p.isOrganization(ctx, account) {
		// Create under organization
		endpoint = fmt.Sprintf("%s/orgs/%s/repos", p.apiBase(), account)
	} else {
		// Create under authenticated user
		endpoint = p.apiBase() + "/user/repos"
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(b))
	req.Header.Set("Authorization", "token "+p.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return RepoURLs{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return RepoURLs{}, fmt.Errorf("gitea create repo error: %s (%s)", resp.Status, string(bodyBytes))
	}
	var out struct {
		SSHURL   string `json:"ssh_url"`
		CloneURL string `json:"clone_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RepoURLs{}, err
	}
	return RepoURLs{SSH: out.SSHURL, HTTPS: out.CloneURL}, nil
}

// SetRepoTopics sets the topics for a Gitea repository.
func (p GiteaProvider) SetRepoTopics(ctx context.Context, account, name string, topics []string) error {
	if p.Token == "" {
		return errors.New("gitea token is required")
	}
	if p.apiBase() == "" {
		return errors.New("gitea base_url is required")
	}
	if len(topics) == 0 {
		return nil
	}

	body := map[string]any{
		"topics": topics,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "PUT", fmt.Sprintf("%s/repos/%s/%s/topics", p.apiBase(), account, name), bytes.NewReader(b))
	req.Header.Set("Authorization", "token "+p.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitea set topics error: %s (%s)", resp.Status, string(bodyBytes))
	}
	return nil
}
