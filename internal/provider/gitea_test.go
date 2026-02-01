package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGiteaProvider_RepoExistsAndCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/acct/repo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token TOKEN" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/v1/repos/acct/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	mux.HandleFunc("/api/v1/user/repos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if r.Header.Get("Authorization") != "token TOKEN" {
			w.WriteHeader(401)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "newrepo" || body["private"] != true {
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ssh_url":   "ssh://gitea/newrepo.git",
			"clone_url": "https://gitea/newrepo.git",
		})
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"login": "acct"})
	})
	mux.HandleFunc("/api/v1/orgs/acct", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404) // acct is a user, not an org
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GiteaProvider{BaseURL: srv.URL, Token: "TOKEN"}

	ok, err := p.RepoExists(context.Background(), "acct", "repo")
	if err != nil || !ok {
		t.Fatalf("RepoExists expected true, got ok=%v err=%v", ok, err)
	}
	ok, err = p.RepoExists(context.Background(), "acct", "missing")
	if err != nil || ok {
		t.Fatalf("RepoExists expected false, got ok=%v err=%v", ok, err)
	}

	urls, err := p.CreatePrivateRepo(context.Background(), "acct", "newrepo", "desc")
	if err != nil {
		t.Fatalf("CreatePrivateRepo: %v", err)
	}
	if !strings.Contains(urls.SSH, "ssh://") || !strings.Contains(urls.HTTPS, "https://") {
		t.Fatalf("unexpected urls: %#v", urls)
	}
}

func TestGiteaProvider_CreateOrgRepo(t *testing.T) {
	mux := http.NewServeMux()

	var usedOrgEndpoint bool
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"login": "myuser"})
	})
	mux.HandleFunc("/api/v1/orgs/myorg", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "myorg"})
	})
	mux.HandleFunc("/api/v1/orgs/myorg/repos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		usedOrgEndpoint = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ssh_url":   "ssh://gitea/myorg/orgrepo.git",
			"clone_url": "https://gitea/myorg/orgrepo.git",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GiteaProvider{BaseURL: srv.URL, Token: "TOKEN"}

	urls, err := p.CreatePrivateRepo(context.Background(), "myorg", "orgrepo", "org repo desc")
	if err != nil {
		t.Fatalf("CreatePrivateRepo for org failed: %v", err)
	}
	if !usedOrgEndpoint {
		t.Fatal("expected to use /orgs/myorg/repos endpoint for org repo creation")
	}
	if !strings.Contains(urls.SSH, "myorg/orgrepo") {
		t.Fatalf("expected URL to contain myorg/orgrepo, got %s", urls.SSH)
	}
}

func TestGiteaProvider_SetRepoTopics(t *testing.T) {
	mux := http.NewServeMux()

	var receivedTopics []string
	mux.HandleFunc("/api/v1/repos/owner/repo/topics", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token TOKEN" {
			w.WriteHeader(401)
			return
		}
		if r.Method == "PUT" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if topics, ok := body["topics"].([]any); ok {
				for _, t := range topics {
					receivedTopics = append(receivedTopics, t.(string))
				}
			}
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(405)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GiteaProvider{BaseURL: srv.URL, Token: "TOKEN"}

	err := p.SetRepoTopics(context.Background(), "owner", "repo", []string{"golang", "cli", "sync"})
	if err != nil {
		t.Fatalf("SetRepoTopics failed: %v", err)
	}
	if len(receivedTopics) != 3 || receivedTopics[0] != "golang" {
		t.Fatalf("expected topics [golang cli sync], got %v", receivedTopics)
	}
}

func TestGiteaProvider_IsOrganization(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/orgs/realorg", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	})
	mux.HandleFunc("/api/v1/orgs/notanorg", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GiteaProvider{BaseURL: srv.URL, Token: "TOKEN"}

	if !p.isOrganization(context.Background(), "realorg") {
		t.Fatal("expected realorg to be detected as organization")
	}
	if p.isOrganization(context.Background(), "notanorg") {
		t.Fatal("expected notanorg to NOT be detected as organization")
	}
}

func TestGiteaProvider_GetAuthenticatedUser(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token TOKEN" {
			w.WriteHeader(401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"login": "testuser"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GiteaProvider{BaseURL: srv.URL, Token: "TOKEN"}

	user := p.getAuthenticatedUser(context.Background())
	if user != "testuser" {
		t.Fatalf("expected user 'testuser', got '%s'", user)
	}
}
