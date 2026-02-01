package scrub

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

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
	private string
	repl    string
	extra   [][2]string

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
	if strings.Contains(repl, priv) {
		return CompiledRules{}, fmt.Errorf("replacement string must not contain the private username")
	}

	ex := make([]string, 0, len(r.ExcludePatterns)+1)
	ex = append(ex, ".git-copy/**") // non-negotiable

	for _, p := range r.ExcludePatterns {
		p = normPath(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, ".git-copy/") || p == ".git-copy" {
			continue
		}
		ex = append(ex, p)
	}

	opt := map[string]bool{}
	for _, p := range r.OptInPaths {
		p = normPath(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, ".git-copy/") || p == ".git-copy" {
			continue
		}
		opt[p] = true
	}

	finalEx := make([]string, 0, len(ex))
	for _, p := range ex {
		if p == ".git-copy/**" {
			finalEx = append(finalEx, p)
			continue
		}
		if opt[p] {
			continue
		}
		finalEx = append(finalEx, p)
	}

	extraPairs := make([][2]string, 0, len(r.ExtraReplacements))
	for k, v := range r.ExtraReplacements {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		extraPairs = append(extraPairs, [2]string{k, v})
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
		private:          priv,
		repl:             repl,
		extra:            extraPairs,
		exclude:          finalEx,
		optIn:            opt,
		publicAuthorName: pubName,
		publicAuthorEmail: pubEmail,
	}, nil
}

func (c CompiledRules) Private() string     { return c.private }
func (c CompiledRules) Replacement() string { return c.repl }

func (c CompiledRules) ShouldExclude(p string) bool {
	p = normPath(p)
	if p == ".git-copy" || strings.HasPrefix(p, ".git-copy/") {
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
	out := strings.ReplaceAll(s, c.private, c.repl)
	for _, kv := range c.extra {
		out = strings.ReplaceAll(out, kv[0], kv[1])
	}
	return out
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
