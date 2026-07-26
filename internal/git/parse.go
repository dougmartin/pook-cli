package git

import (
	"regexp"
	"strconv"
	"strings"
)

// The parsers below are ports of src/git.ts. The original returns JS Maps,
// whose iteration order is insertion order, and callers depend on that order
// for display. ordered preserves it.

// ordered is a map that remembers the order keys were first set in.
type ordered[V any] struct {
	keys []string
	vals map[string]V
}

func newOrdered[V any]() *ordered[V] {
	return &ordered[V]{vals: map[string]V{}}
}

// set stores a value, leaving an existing key in its original position.
func (o *ordered[V]) set(k string, v V) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *ordered[V]) get(k string) (V, bool) {
	v, ok := o.vals[k]
	return v, ok
}

func (o *ordered[V]) len() int { return len(o.keys) }

// all iterates in insertion order.
func (o *ordered[V]) all(f func(k string, v V)) {
	for _, k := range o.keys {
		f(k, o.vals[k])
	}
}

// counts is one file's line delta from a --numstat record.
type counts struct {
	additions int
	deletions int
	binary    bool
}

// parseStatus reads `git status --porcelain=v1 -z` into path -> status.
func parseStatus(out string) *ordered[Status] {
	entries := newOrdered[Status]()

	var tokens []string
	for _, t := range strings.Split(out, "\x00") {
		if t != "" {
			tokens = append(tokens, t)
		}
	}

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if len(t) < 4 {
			continue
		}
		xy := t[:2]
		p := t[3:]
		if xy[0] == 'R' || xy[0] == 'C' {
			i++ // the next token is the original path
		}

		var status Status
		switch {
		case xy == "??":
			status = StatusNew
		case strings.Contains(xy, "D"):
			status = StatusDeleted
		case xy[0] == 'A':
			status = StatusNew
		default:
			status = StatusChanged
		}
		entries.set(p, status)
	}
	return entries
}

var numstatRe = regexp.MustCompile(`(?s)^(-|\d+)\t(-|\d+)\t(.*)$`)

// parseNumstat reads `git diff --numstat -z` into path -> counts. A binary
// file reports "-" for both numbers.
func parseNumstat(out string) *ordered[counts] {
	result := newOrdered[counts]()
	tokens := strings.Split(out, "\x00")

	for i := 0; i < len(tokens); i++ {
		m := numstatRe.FindStringSubmatch(tokens[i])
		if m == nil {
			continue
		}
		p := m[3]
		if p == "" {
			// Rename record: "counts\t\t" NUL oldpath NUL newpath.
			if i+2 >= len(tokens) {
				continue
			}
			p = tokens[i+2]
			i += 2
		}
		result.set(p, counts{
			additions: atoiDash(m[1]),
			deletions: atoiDash(m[2]),
			binary:    m[1] == "-",
		})
	}
	return result
}

// atoiDash parses a numstat field, where "-" means binary and counts as zero.
func atoiDash(s string) int {
	if s == "-" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// binaryDiffRe spots the line git emits instead of a patch for binary content.
var binaryDiffRe = regexp.MustCompile(`(?m)^Binary files `)

var (
	diffPlusRe   = regexp.MustCompile(`(?m)^\+\+\+ b/(.*)$`)
	diffMinusRe  = regexp.MustCompile(`(?m)^--- a/(.*)$`)
	diffHeaderRe = regexp.MustCompile(`(?m)^diff --git a/.* b/(.*)$`)
)

const diffMarker = "diff --git "

// parseDiffs splits a multi-file patch into path -> that file's hunk.
func parseDiffs(out string) *ordered[string] {
	diffs := newOrdered[string]()
	for _, chunk := range splitDiffChunks(out) {
		if p := diffPath(chunk); p != "" {
			diffs.set(p, chunk)
		}
	}
	return diffs
}

// splitDiffChunks cuts a patch at each "diff --git " line. The original used a
// lookahead split, which RE2 cannot express, so the split is done by hand.
func splitDiffChunks(out string) []string {
	var chunks []string
	start := -1

	for i := 0; i <= len(out); {
		atLineStart := i == 0 || out[i-1] == '\n'
		if atLineStart && strings.HasPrefix(out[i:], diffMarker) {
			if start >= 0 {
				chunks = append(chunks, out[start:i])
			}
			start = i
		}
		nl := strings.IndexByte(out[i:], '\n')
		if nl == -1 {
			break
		}
		i += nl + 1
	}

	if start >= 0 {
		chunks = append(chunks, out[start:])
	}
	return chunks
}

// diffPath is the file a chunk describes, preferring the post-image name so a
// rename is filed under its new path.
func diffPath(chunk string) string {
	for _, re := range []*regexp.Regexp{diffPlusRe, diffMinusRe, diffHeaderRe} {
		if m := re.FindStringSubmatch(chunk); m != nil {
			return m[1]
		}
	}
	return ""
}

// truncateDiff caps a diff at the same limit the original used, with the same
// marker, so an enormous file cannot blow up the renderer.
func truncateDiff(diff string) string {
	if len(diff) <= maxDiffChars {
		return diff
	}
	return diff[:maxDiffChars] + "\n… diff truncated …"
}
