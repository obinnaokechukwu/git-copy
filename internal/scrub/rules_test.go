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

func TestRules_RewriteStringCasePreserving(t *testing.T) {
	r, err := Compile(Rules{
		PrivateUsername: "obinnaokechukwu",
		Replacement:     "johndoe",
		ExtraReplacements: map[string]string{
			"SecretKey": "PublicKey",
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cases := []struct {
		input, expected string
	}{
		// Lowercase → lowercase
		{"github.com/obinnaokechukwu/repo", "github.com/johndoe/repo"},
		// Uppercase → uppercase
		{"github.com/OBINNAOKECHUKWU/repo", "github.com/JOHNDOE/repo"},
		// Title case → title case
		{"github.com/Obinnaokechukwu/repo", "github.com/Johndoe/repo"},
		// Mixed case → replacement as-is
		{"Hello obinnaokechukwu!", "Hello johndoe!"},
		// Extra replacements also case-preserving
		{"Use SecretKey here", "Use PublicKey here"},
		{"Use SECRETKEY here", "Use PUBLICKEY here"},
		{"Use secretkey here", "Use publickey here"},
		{"Use Secretkey here", "Use Publickey here"},
	}

	for _, tc := range cases {
		out := r.RewriteString(tc.input)
		if out != tc.expected {
			t.Errorf("RewriteString(%q) = %q, want %q", tc.input, out, tc.expected)
		}
	}

	// Test RewriteBytes too
	for _, tc := range cases {
		out := r.RewriteBytes([]byte(tc.input))
		if string(out) != tc.expected {
			t.Errorf("RewriteBytes(%q) = %q, want %q", tc.input, string(out), tc.expected)
		}
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

func TestRules_ReplaceHistoryWithCurrent(t *testing.T) {
	// Test that ReplaceHistoryWithCurrent files are properly tracked
	r, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"LICENSE", "NOTICE", "docs/copyright.txt"},
		ReplaceHistoryContent: map[string][]byte{
			"LICENSE":           []byte("MIT License - Copyright obinnaokechukwu\n"),
			"NOTICE":            []byte("Notice file content\n"),
			"docs/copyright.txt": []byte("Copyright obinnaokechukwu 2024\n"),
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// Check that files are marked for replacement
	if !r.ShouldReplaceHistory("LICENSE") {
		t.Errorf("expected LICENSE to be marked for replacement")
	}
	if !r.ShouldReplaceHistory("NOTICE") {
		t.Errorf("expected NOTICE to be marked for replacement")
	}
	if !r.ShouldReplaceHistory("docs/copyright.txt") {
		t.Errorf("expected docs/copyright.txt to be marked for replacement")
	}
	if r.ShouldReplaceHistory("README.md") {
		t.Errorf("README.md should not be marked for replacement")
	}

	// Check that content is scrubbed
	licenseContent := r.GetReplaceHistoryContent("LICENSE")
	if licenseContent == nil {
		t.Fatalf("expected LICENSE content to exist")
	}
	if string(licenseContent) != "MIT License - Copyright johndoe\n" {
		t.Errorf("expected LICENSE content to be scrubbed, got: %q", string(licenseContent))
	}

	copyrightContent := r.GetReplaceHistoryContent("docs/copyright.txt")
	if copyrightContent == nil {
		t.Fatalf("expected docs/copyright.txt content to exist")
	}
	if string(copyrightContent) != "Copyright johndoe 2024\n" {
		t.Errorf("expected copyright content to be scrubbed, got: %q", string(copyrightContent))
	}

	// Check that GetReplaceHistoryFiles returns all files
	files := r.GetReplaceHistoryFiles()
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}
}

func TestRules_ReplaceHistoryWithCurrentEmptyContent(t *testing.T) {
	// Test handling of files not in HEAD (no content provided)
	r, err := Compile(Rules{
		PrivateUsername:           "obinnaokechukwu",
		Replacement:               "johndoe",
		ReplaceHistoryWithCurrent: []string{"LICENSE", "OLD_FILE"},
		ReplaceHistoryContent: map[string][]byte{
			"LICENSE": []byte("MIT License\n"),
			// OLD_FILE intentionally not in ReplaceHistoryContent
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// Both files should be marked for replacement
	if !r.ShouldReplaceHistory("LICENSE") {
		t.Errorf("expected LICENSE to be marked for replacement")
	}
	if !r.ShouldReplaceHistory("OLD_FILE") {
		t.Errorf("expected OLD_FILE to be marked for replacement")
	}

	// LICENSE should have content
	if r.GetReplaceHistoryContent("LICENSE") == nil {
		t.Errorf("expected LICENSE content to exist")
	}

	// OLD_FILE should have no content
	if r.GetReplaceHistoryContent("OLD_FILE") != nil {
		t.Errorf("expected OLD_FILE content to be nil")
	}
}
