// Package watch turns filesystem activity into the signals the status bar and
// ticker are built from.
package watch

import (
	"path"
	"regexp"
	"strings"
)

// globToRegex compiles a glob into an anchored regexp, matching the original's
// hand written translation: ** spans directories, * and ? do not.
func globToRegex(glob string) *regexp.Regexp {
	var re strings.Builder
	re.WriteString("^")

	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				re.WriteString(".*")
				i++
				// A trailing slash after ** is optional, so **/foo also
				// matches a plain foo at the root.
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
				}
			} else {
				re.WriteString("[^/]*")
			}
		case '?':
			re.WriteString("[^/]")
		default:
			re.WriteString(regexp.QuoteMeta(string(c)))
		}
	}

	re.WriteString("$")
	compiled, err := regexp.Compile(re.String())
	if err != nil {
		// A glob that will not compile matches nothing, rather than taking
		// the process down over a config typo.
		return regexp.MustCompile(`$^`)
	}
	return compiled
}

// Matcher reports whether a repo-relative path is one of the watched globs.
type Matcher struct {
	patterns []pattern
}

type pattern struct {
	re *regexp.Regexp
	// baseOnly is set for a glob with no slash in it, which is also tried
	// against the file's base name, so "*.lock" matches at any depth.
	baseOnly bool
}

// NewMatcher compiles the configured globs.
func NewMatcher(globs []string) Matcher {
	m := Matcher{}
	for _, g := range globs {
		m.patterns = append(m.patterns, pattern{
			re:       globToRegex(g),
			baseOnly: !strings.Contains(g, "/"),
		})
	}
	return m
}

// Match reports whether a slash-separated repo-relative path is watched.
func (m Matcher) Match(p string) bool {
	base := path.Base(p)
	for _, pat := range m.patterns {
		if pat.re.MatchString(p) || (pat.baseOnly && pat.re.MatchString(base)) {
			return true
		}
	}
	return false
}

// Empty reports whether nothing is being watched.
func (m Matcher) Empty() bool { return len(m.patterns) == 0 }
