package scrub

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// NonNegotiableDirs are directories that are always excluded and cannot be opted-in.
// Add new directories here to exclude them everywhere.
var NonNegotiableDirs = []string{".git-copy", ".claude"}

// IsNonNegotiablePath returns true if the path is inside a non-negotiable directory.
func IsNonNegotiablePath(p string) bool {
	p = normPath(p)
	for _, dir := range NonNegotiableDirs {
		if p == dir || strings.HasPrefix(p, dir+"/") {
			return true
		}
	}
	return false
}

// nonNegotiablePatterns returns glob patterns for all non-negotiable dirs.
func nonNegotiablePatterns() []string {
	patterns := make([]string, len(NonNegotiableDirs))
	for i, dir := range NonNegotiableDirs {
		patterns[i] = dir + "/**"
	}
	return patterns
}

// isNonNegotiablePattern returns true if the pattern is a non-negotiable glob pattern.
func isNonNegotiablePattern(p string) bool {
	for _, dir := range NonNegotiableDirs {
		if p == dir+"/**" {
			return true
		}
	}
	return false
}

type Rules struct {
	PrivateUsername   string
	Replacement       string
	ExtraReplacements map[string]string

	ExcludePatterns []string
	OptInPaths      []string

	PublicAuthorName  string
	PublicAuthorEmail string
}

type CompiledRules struct {
	private   string
	privateRe *regexp.Regexp // case-insensitive pattern for private username
	repl      string
	extra     [][2]string
	extraRe   []*regexp.Regexp // case-insensitive patterns for extra replacements

	exclude []string
	optIn   map[string]bool

	publicAuthorName  string
	publicAuthorEmail string
}

func Compile(r Rules) (CompiledRules, error) {
	priv := strings.TrimSpace(r.PrivateUsername)
	if priv == "" {
		return CompiledRules{}, errors.New("private username is required")
	}
	repl := strings.TrimSpace(r.Replacement)
	if repl == "" {
		return CompiledRules{}, errors.New("replacement is required")
	}
	if strings.Contains(strings.ToLower(repl), strings.ToLower(priv)) {
		return CompiledRules{}, fmt.Errorf("replacement string must not contain the private username")
	}

	// Compile case-insensitive regex for private username
	privateRe, err := regexp.Compile("(?i)" + regexp.QuoteMeta(priv))
	if err != nil {
		return CompiledRules{}, fmt.Errorf("invalid private username pattern: %w", err)
	}

	// Start with non-negotiable patterns
	nnPatterns := nonNegotiablePatterns()
	ex := make([]string, 0, len(r.ExcludePatterns)+len(nnPatterns))
	ex = append(ex, nnPatterns...)

	for _, p := range r.ExcludePatterns {
		p = normPath(p)
		if p == "" || IsNonNegotiablePath(p) {
			continue
		}
		ex = append(ex, p)
	}

	opt := map[string]bool{}
	for _, p := range r.OptInPaths {
		p = normPath(p)
		if p == "" || IsNonNegotiablePath(p) {
			continue
		}
		opt[p] = true
	}

	finalEx := make([]string, 0, len(ex))
	for _, p := range ex {
		if isNonNegotiablePattern(p) {
			finalEx = append(finalEx, p)
			continue
		}
		if opt[p] {
			continue
		}
		finalEx = append(finalEx, p)
	}

	extraPairs := make([][2]string, 0, len(r.ExtraReplacements))
	extraRe := make([]*regexp.Regexp, 0, len(r.ExtraReplacements))
	for k, v := range r.ExtraReplacements {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(k))
		if err != nil {
			return CompiledRules{}, fmt.Errorf("invalid extra replacement pattern %q: %w", k, err)
		}
		extraPairs = append(extraPairs, [2]string{k, v})
		extraRe = append(extraRe, re)
	}

	pubName := strings.TrimSpace(r.PublicAuthorName)
	if pubName == "" {
		pubName = repl
	}
	pubEmail := strings.TrimSpace(r.PublicAuthorEmail)
	if pubEmail == "" {
		pubEmail = repl + "@example.invalid"
	}

	return CompiledRules{
		private:           priv,
		privateRe:         privateRe,
		repl:              repl,
		extra:             extraPairs,
		extraRe:           extraRe,
		exclude:           finalEx,
		optIn:             opt,
		publicAuthorName:  pubName,
		publicAuthorEmail: pubEmail,
	}, nil
}

func (c CompiledRules) Private() string     { return c.private }
func (c CompiledRules) Replacement() string { return c.repl }

func (c CompiledRules) ShouldExclude(p string) bool {
	p = normPath(p)
	if IsNonNegotiablePath(p) {
		return true
	}
	for _, pat := range c.exclude {
		pat = normPath(pat)
		if pat == "" {
			continue
		}
		if matchGlob(pat, p) {
			return true
		}
	}
	return false
}

func (c CompiledRules) RewriteString(s string) string {
	out := c.privateRe.ReplaceAllStringFunc(s, func(match string) string {
		return applyCasePattern(match, c.repl)
	})
	for i, re := range c.extraRe {
		repl := c.extra[i][1]
		out = re.ReplaceAllStringFunc(out, func(match string) string {
			return applyCasePattern(match, repl)
		})
	}
	return out
}

// RewriteBytes performs case-preserving replacement on byte slices.
func (c CompiledRules) RewriteBytes(b []byte) []byte {
	out := c.privateRe.ReplaceAllFunc(b, func(match []byte) []byte {
		return []byte(applyCasePattern(string(match), c.repl))
	})
	for i, re := range c.extraRe {
		repl := c.extra[i][1]
		out = re.ReplaceAllFunc(out, func(match []byte) []byte {
			return []byte(applyCasePattern(string(match), repl))
		})
	}
	return out
}

// applyCasePattern applies the case pattern of match to replacement.
// - All lowercase match → all lowercase replacement
// - All uppercase match → all uppercase replacement
// - Title case match (first upper, rest lower) → title case replacement
// - Otherwise, use replacement as-is
func applyCasePattern(match, replacement string) string {
	if len(match) == 0 {
		return replacement
	}

	allLower := true
	allUpper := true
	for _, r := range match {
		if r >= 'A' && r <= 'Z' {
			allLower = false
		}
		if r >= 'a' && r <= 'z' {
			allUpper = false
		}
	}

	if allLower {
		return strings.ToLower(replacement)
	}
	if allUpper {
		return strings.ToUpper(replacement)
	}

	// Check for title case (first letter upper, rest lower)
	runes := []rune(match)
	if len(runes) > 0 && runes[0] >= 'A' && runes[0] <= 'Z' {
		restLower := true
		for _, r := range runes[1:] {
			if r >= 'A' && r <= 'Z' {
				restLower = false
				break
			}
		}
		if restLower {
			// Title case: capitalize first letter, lowercase rest
			replRunes := []rune(strings.ToLower(replacement))
			if len(replRunes) > 0 && replRunes[0] >= 'a' && replRunes[0] <= 'z' {
				replRunes[0] = replRunes[0] - 'a' + 'A'
			}
			return string(replRunes)
		}
	}

	// Mixed case or unknown pattern: use replacement as-is
	return replacement
}

func (c CompiledRules) PublicAuthorName() string  { return c.publicAuthorName }
func (c CompiledRules) PublicAuthorEmail() string { return c.publicAuthorEmail }

func normPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "./")
	p = strings.ReplaceAll(p, "\\", "/")
	// clean but preserve trailing /** patterns
	return p
}

// matchGlob supports patterns with `**` as a full path segment (like `.git-copy/**` or `**/*.go`).
// Other segments use path.Match semantics.
func matchGlob(pattern, target string) bool {
	pattern = normPath(pattern)
	target = normPath(target)
	ps := strings.Split(pattern, "/")
	ts := strings.Split(target, "/")
	return matchSegs(ps, ts)
}

func matchSegs(patSegs, pathSegs []string) bool {
	if len(patSegs) == 0 {
		return len(pathSegs) == 0
	}
	if len(patSegs) == 1 && patSegs[0] == "" {
		return len(pathSegs) == 1 && pathSegs[0] == ""
	}
	if patSegs[0] == "**" {
		// match any number of segments
		for k := 0; k <= len(pathSegs); k++ {
			if matchSegs(patSegs[1:], pathSegs[k:]) {
				return true
			}
		}
		return false
	}
	if len(pathSegs) == 0 {
		return false
	}
	ok, err := path.Match(patSegs[0], pathSegs[0])
	if err != nil || !ok {
		return false
	}
	return matchSegs(patSegs[1:], pathSegs[1:])
}
