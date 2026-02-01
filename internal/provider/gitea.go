package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	// Minimal: create under authenticated user (ignores account)
	req, _ := http.NewRequestWithContext(ctx, "POST", p.apiBase()+"/user/repos", bytes.NewReader(b))
	req.Header.Set("Authorization", "token "+p.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return RepoURLs{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return RepoURLs{}, fmt.Errorf("gitea create repo error: %s", resp.Status)
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
