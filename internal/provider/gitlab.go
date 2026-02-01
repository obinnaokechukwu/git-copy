package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type GitLabProvider struct {
	BaseURL string // e.g. https://gitlab.com
	Token   string // personal access token
}

func (p GitLabProvider) Name() string { return "gitlab" }

func (p GitLabProvider) apiBase() string {
	b := strings.TrimRight(p.BaseURL, "/")
	if b == "" {
		b = "https://gitlab.com"
	}
	return b + "/api/v4"
}

func (p GitLabProvider) RepoExists(ctx context.Context, account, name string) (bool, error) {
	if p.Token == "" {
		return false, errors.New("gitlab token is required")
	}
	// GET /projects/:id where id is URL-encoded path "namespace%2Frepo"
	id := url.PathEscape(account + "/" + name)
	req, _ := http.NewRequestWithContext(ctx, "GET", p.apiBase()+"/projects/"+id, nil)
	req.Header.Set("PRIVATE-TOKEN", p.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false, nil
	}
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("gitlab api error: %s", resp.Status)
	}
	return true, nil
}

func (p GitLabProvider) CreatePrivateRepo(ctx context.Context, account, name, description string) (RepoURLs, error) {
	if p.Token == "" {
		return RepoURLs{}, errors.New("gitlab token is required")
	}
	// Minimal: create project in user's namespace (token owner). account is informational for now.
	body := map[string]any{
		"name":                   name,
		"visibility":             "private",
		"description":            description,
		"initialize_with_readme": false,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", p.apiBase()+"/projects", bytes.NewReader(b))
	req.Header.Set("PRIVATE-TOKEN", p.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return RepoURLs{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return RepoURLs{}, fmt.Errorf("gitlab create project error: %s", resp.Status)
	}
	var out struct {
		SSHURLToRepo  string `json:"ssh_url_to_repo"`
		HTTPURLToRepo string `json:"http_url_to_repo"`
		PathWithNS    string `json:"path_with_namespace"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RepoURLs{}, err
	}
	return RepoURLs{SSH: out.SSHURLToRepo, HTTPS: out.HTTPURLToRepo}, nil
}
