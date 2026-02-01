package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// getNamespaceID looks up the namespace ID for the given account (user or group).
func (p GitLabProvider) getNamespaceID(ctx context.Context, account string) (int, error) {
	// Try as group first
	req, _ := http.NewRequestWithContext(ctx, "GET", p.apiBase()+"/groups/"+url.PathEscape(account), nil)
	req.Header.Set("PRIVATE-TOKEN", p.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var g struct {
			ID int `json:"id"`
		}
		if json.NewDecoder(resp.Body).Decode(&g) == nil && g.ID > 0 {
			return g.ID, nil
		}
	}

	// Try as user
	req2, _ := http.NewRequestWithContext(ctx, "GET", p.apiBase()+"/users?username="+url.QueryEscape(account), nil)
	req2.Header.Set("PRIVATE-TOKEN", p.Token)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return 0, err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == 200 {
		var users []struct {
			ID int `json:"id"`
		}
		if json.NewDecoder(resp2.Body).Decode(&users) == nil && len(users) > 0 {
			return users[0].ID, nil
		}
	}
	return 0, fmt.Errorf("could not find namespace for %q", account)
}

func (p GitLabProvider) CreatePrivateRepo(ctx context.Context, account, name, description string) (RepoURLs, error) {
	if p.Token == "" {
		return RepoURLs{}, errors.New("gitlab token is required")
	}

	body := map[string]any{
		"name":                   name,
		"path":                   name,
		"visibility":             "private",
		"description":            description,
		"initialize_with_readme": false,
	}

	// Try to get namespace ID for the account to create repo under that namespace
	if nsID, err := p.getNamespaceID(ctx, account); err == nil && nsID > 0 {
		body["namespace_id"] = nsID
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
		bodyBytes, _ := io.ReadAll(resp.Body)
		return RepoURLs{}, fmt.Errorf("gitlab create project error: %s (%s)", resp.Status, string(bodyBytes))
	}
	var out struct {
		ID            int    `json:"id"`
		SSHURLToRepo  string `json:"ssh_url_to_repo"`
		HTTPURLToRepo string `json:"http_url_to_repo"`
		PathWithNS    string `json:"path_with_namespace"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RepoURLs{}, err
	}
	return RepoURLs{SSH: out.SSHURLToRepo, HTTPS: out.HTTPURLToRepo}, nil
}

// SetRepoTopics sets the topics for a GitLab project.
func (p GitLabProvider) SetRepoTopics(ctx context.Context, account, name string, topics []string) error {
	if p.Token == "" {
		return errors.New("gitlab token is required")
	}
	if len(topics) == 0 {
		return nil
	}

	id := url.PathEscape(account + "/" + name)
	body := map[string]any{
		"topics": topics,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "PUT", p.apiBase()+"/projects/"+id, bytes.NewReader(b))
	req.Header.Set("PRIVATE-TOKEN", p.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitlab set topics error: %s (%s)", resp.Status, string(bodyBytes))
	}
	return nil
}
