package scrub

import "testing"

func TestRules_ExcludeAndOptIn(t *testing.T) {
	r, err := Compile(Rules{
		PrivateUsername: "obinnaokechukwu",
		Replacement:     "johndoe",
		ExcludePatterns: []string{".env", "secrets/**", "docs/*.md"},
		OptInPaths:      []string{".env"},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !r.ShouldExclude(".git-copy/config.json") {
		t.Fatalf("expected .git-copy to be excluded")
	}
	if !r.ShouldExclude(".claude/session.json") {
		t.Fatalf("expected .claude to be excluded")
	}
	if r.ShouldExclude(".env") {
		t.Fatalf("expected .env to be opt-in included")
	}
	if !r.ShouldExclude("secrets/a/b/c.txt") {
		t.Fatalf("expected secrets/** to match")
	}
	if !r.ShouldExclude("docs/readme.md") {
		t.Fatalf("expected docs/*.md to match")
	}
	if r.ShouldExclude("docs/readme.txt") {
		t.Fatalf("did not expect docs/*.md to match .txt")
	}
}

func TestRules_RewriteString(t *testing.T) {
	r, err := Compile(Rules{
		PrivateUsername: "obinnaokechukwu",
		Replacement:     "johndoe",
		ExtraReplacements: map[string]string{
			"old": "new",
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	out := r.RewriteString("github.com/obinnaokechukwu/old")
	if out != "github.com/johndoe/new" {
		t.Fatalf("rewrite mismatch: %q", out)
	}
}

func TestRules_RejectReplacementContainingPrivate(t *testing.T) {
	_, err := Compile(Rules{
		PrivateUsername: "abc",
		Replacement:     "xxabcxx",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}
