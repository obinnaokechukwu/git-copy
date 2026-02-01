package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGitLabProvider_RepoExistsAndCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "TOKEN" {
			w.WriteHeader(401)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/v4/projects/")
		decoded, _ := url.PathUnescape(id)
		if decoded == "acct/repo" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	})
	mux.HandleFunc("/api/v4/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if r.Header.Get("PRIVATE-TOKEN") != "TOKEN" {
			w.WriteHeader(401)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "newrepo" || body["visibility"] != "private" {
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ssh_url_to_repo":  "ssh://gitlab/newrepo.git",
			"http_url_to_repo": "https://gitlab/newrepo.git",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GitLabProvider{BaseURL: srv.URL, Token: "TOKEN"}

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

func TestGitLabProvider_NamespaceLookup(t *testing.T) {
	mux := http.NewServeMux()

	// Mock group endpoint
	mux.HandleFunc("/api/v4/groups/mygroup", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "TOKEN" {
			w.WriteHeader(401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42})
	})

	// Mock users endpoint for user lookup
	mux.HandleFunc("/api/v4/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "TOKEN" {
			w.WriteHeader(401)
			return
		}
		username := r.URL.Query().Get("username")
		if username == "myuser" {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 99}})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})

	// Mock group not found
	mux.HandleFunc("/api/v4/groups/myuser", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GitLabProvider{BaseURL: srv.URL, Token: "TOKEN"}

	// Test group lookup
	nsID, err := p.getNamespaceID(context.Background(), "mygroup")
	if err != nil || nsID != 42 {
		t.Fatalf("expected namespace_id=42 for group, got %d, err=%v", nsID, err)
	}

	// Test user lookup (falls back after group 404)
	nsID, err = p.getNamespaceID(context.Background(), "myuser")
	if err != nil || nsID != 99 {
		t.Fatalf("expected namespace_id=99 for user, got %d, err=%v", nsID, err)
	}
}

func TestGitLabProvider_SetRepoTopics(t *testing.T) {
	mux := http.NewServeMux()

	var receivedTopics []string
	mux.HandleFunc("/api/v4/projects/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "TOKEN" {
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
		w.WriteHeader(200)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GitLabProvider{BaseURL: srv.URL, Token: "TOKEN"}

	err := p.SetRepoTopics(context.Background(), "acct", "repo", []string{"go", "cli", "tool"})
	if err != nil {
		t.Fatalf("SetRepoTopics failed: %v", err)
	}
	if len(receivedTopics) != 3 || receivedTopics[0] != "go" {
		t.Fatalf("expected topics [go cli tool], got %v", receivedTopics)
	}
}

func TestGitLabProvider_CreateRepoWithNamespace(t *testing.T) {
	mux := http.NewServeMux()

	var receivedNamespaceID float64
	mux.HandleFunc("/api/v4/groups/myorg", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 123})
	})
	mux.HandleFunc("/api/v4/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if ns, ok := body["namespace_id"].(float64); ok {
			receivedNamespaceID = ns
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":               456,
			"ssh_url_to_repo":  "ssh://gitlab/myorg/testrepo.git",
			"http_url_to_repo": "https://gitlab/myorg/testrepo.git",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GitLabProvider{BaseURL: srv.URL, Token: "TOKEN"}

	urls, err := p.CreatePrivateRepo(context.Background(), "myorg", "testrepo", "test desc")
	if err != nil {
		t.Fatalf("CreatePrivateRepo failed: %v", err)
	}
	if receivedNamespaceID != 123 {
		t.Fatalf("expected namespace_id=123, got %v", receivedNamespaceID)
	}
	if !strings.Contains(urls.SSH, "myorg/testrepo") {
		t.Fatalf("expected URL to contain myorg/testrepo, got %s", urls.SSH)
	}
}
