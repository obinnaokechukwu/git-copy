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
