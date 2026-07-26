package watch

import (
	"testing"

	"github.com/dougmartin/pook-cli/internal/config"
)

func TestMatcher(t *testing.T) {
	tests := []struct {
		name  string
		globs []string
		match []string
		miss  []string
	}{
		{
			name:  "a bare glob matches at any depth by base name",
			globs: []string{"*.lock"},
			match: []string{"Cargo.lock", "deep/nested/yarn.lock"},
			miss:  []string{"lockfile", "a.lock.bak"},
		},
		{
			name:  "** spans directories",
			globs: []string{"**/package.json"},
			match: []string{"package.json", "web/package.json", "a/b/c/package.json"},
			miss:  []string{"package.json.bak", "packagejson"},
		},
		{
			name:  "a trailing ** matches everything under a prefix",
			globs: []string{".github/**"},
			match: []string{".github/workflows/ci.yml", ".github/CODEOWNERS"},
			miss:  []string{"github/x", "docs/.github/x"},
		},
		{
			name:  "** in the middle matches any depth on both sides",
			globs: []string{"**/migrations/**"},
			match: []string{"migrations/001.sql", "db/migrations/001.sql", "a/b/migrations/c/d.sql"},
			miss:  []string{"migrations", "db/migration/001.sql"},
		},
		{
			name:  "a single star stops at a slash",
			globs: []string{"src/*.go"},
			match: []string{"src/main.go"},
			miss:  []string{"src/ui/main.go", "main.go"},
		},
		{
			name:  "a question mark matches one character",
			globs: []string{"v?.txt"},
			match: []string{"v1.txt"},
			miss:  []string{"v10.txt", "v.txt"},
		},
		{
			name:  "a prefix glob matches dotfile variants",
			globs: []string{".env*"},
			match: []string{".env", ".env.local", "app/.env.production"},
			miss:  []string{"env", "denv"},
		},
		{
			name:  "dots are literal, not wildcards",
			globs: []string{"a.txt"},
			match: []string{"a.txt"},
			miss:  []string{"axtxt"},
		},
		{
			name:  "nothing configured matches nothing",
			globs: nil,
			miss:  []string{"anything", "package.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMatcher(tt.globs)

			for _, p := range tt.match {
				if !m.Match(p) {
					t.Errorf("%q should match %v", p, tt.globs)
				}
			}
			for _, p := range tt.miss {
				if m.Match(p) {
					t.Errorf("%q should not match %v", p, tt.globs)
				}
			}
		})
	}
}

// The shipped defaults are what most people will actually run with.
func TestMatcherWithTheDefaultGlobs(t *testing.T) {
	m := NewMatcher(config.Default().WatchedGlobs)

	watched := []string{
		".env", ".env.local",
		"package.json", "web/package.json",
		"package-lock.json", "yarn.lock", "Cargo.lock",
		".github/workflows/ci.yml",
		"db/migrations/0001_init.sql",
	}
	for _, p := range watched {
		if !m.Match(p) {
			t.Errorf("%q should be watched by default", p)
		}
	}

	ordinary := []string{
		"internal/ui/app.go", "README.md", "cmd/pook/main.go", "src/env.ts",
	}
	for _, p := range ordinary {
		if m.Match(p) {
			t.Errorf("%q should not be watched by default", p)
		}
	}
}

func TestMatcherEmpty(t *testing.T) {
	if !NewMatcher(nil).Empty() {
		t.Error("a matcher with no globs is not empty")
	}
	if NewMatcher([]string{"*.go"}).Empty() {
		t.Error("a matcher with a glob reports empty")
	}
}

// A glob that cannot compile must not take the process down.
func TestMatcherSurvivesABadGlob(t *testing.T) {
	m := NewMatcher([]string{"[", "*.go"})

	if m.Match("anything") {
		t.Error("the broken glob matched")
	}
	if !m.Match("main.go") {
		t.Error("a valid glob alongside a broken one stopped working")
	}
}
