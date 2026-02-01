package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubProvider_APIRepoExistsAndCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acct/repo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token TOKEN" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	})
	mux.HandleFunc("/repos/acct/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	mux.HandleFunc("/user/repos", func(w http.ResponseWriter, r *http.Request) {
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
			"ssh_url":   "ssh://example/newrepo.git",
			"clone_url": "https://example/newrepo.git",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := GitHubProvider{UseGHCLI: false, Token: "TOKEN", BaseURL: srv.URL}

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
