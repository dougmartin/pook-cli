// Package prompts is the reusable prompt library: placeholder handling,
// markdown import and export, and the shared prompts.json store. It is a port
// of src/prompts.ts.
package prompts

import (
	"fmt"
	"regexp"
	"strings"
)

// Prompt is one entry in the library. The JSON shape matches the extension's,
// so a library written by either tool imports into the other.
type Prompt struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	// CreatedAt and UpdatedAt are Unix milliseconds.
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// tokenRe matches a {{name}} placeholder. The name is trimmed by the callers.
var tokenRe = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

// ExtractTokens lists the distinct {{name}} placeholders in a prompt, in order
// of first appearance. Names are trimmed, so {{ a }} and {{a}} are one token.
func ExtractTokens(text string) []string {
	var out []string
	for _, m := range tokenRe.FindAllStringSubmatch(text, -1) {
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		if !contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// FillTokens replaces every {{name}} with values[name]. An unknown token is
// left exactly as it was, so a half-filled prompt still shows what is missing.
func FillTokens(text string, values map[string]string) string {
	return tokenRe.ReplaceAllStringFunc(text, func(whole string) string {
		m := tokenRe.FindStringSubmatch(whole)
		if v, ok := values[strings.TrimSpace(m[1])]; ok {
			return v
		}
		return whole
	})
}

// markerRe matches an inline all-caps bracketed placeholder such as [STORY] or
// [FILE PATH].
//
// The original excluded markdown links with a negative lookahead, `](`, which
// RE2 cannot express. The check moves into MarkerIndexes instead: a match
// followed by "(" is a link, not a marker.
var markerRe = regexp.MustCompile(`\[[A-Z][A-Z0-9 _-]*\]`)

// MarkerIndexes returns the [start, end) byte ranges of every inline marker,
// in order. It is what the clipboard modal selects over.
func MarkerIndexes(text string) [][]int {
	var out [][]int
	for _, loc := range markerRe.FindAllStringIndex(text, -1) {
		// A marker immediately followed by "(" is a markdown link label.
		if loc[1] < len(text) && text[loc[1]] == '(' {
			continue
		}
		out = append(out, loc)
	}
	return out
}

// HasInlineMarker reports whether the text still has a [MARKER] to fill in.
func HasInlineMarker(text string) bool {
	return len(MarkerIndexes(text)) > 0
}

// FormatMarkdown serializes prompts in the format ParseMarkdown reads back: a
// "## title" heading before every prompt, so a prompt's paragraphs can only be
// attributed to its own heading.
//
// A prompt whose text has blank lines still re-imports as several prompts,
// because the importer splits paragraphs. That caveat is inherited from the
// original and is covered by a test.
func FormatMarkdown(prompts []Prompt) string {
	var b strings.Builder
	for _, p := range prompts {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", p.Title, strings.TrimSpace(p.Text))
	}
	return b.String()
}

var headingRe = regexp.MustCompile(`^#{1,3}\s+(.*)$`)

// ParseMarkdown is the bulk importer: # and ## headings name the prompts, and
// each blank-line-separated paragraph under a heading becomes its own prompt.
// A heading with several paragraphs gets " (2)", " (3)" suffixes.
func ParseMarkdown(md string) []Prompt {
	var out []Prompt
	var buf []string
	title := ""
	count := 0

	flush := func() {
		text := strings.TrimSpace(strings.Join(buf, "\n"))
		buf = nil
		if text == "" {
			return
		}
		count++

		name := title
		if name == "" {
			name = "Untitled"
		}
		if count > 1 {
			name = fmt.Sprintf("%s (%d)", title, count)
		}
		out = append(out, Prompt{Title: name, Text: text})
	}

	for _, line := range strings.Split(md, "\n") {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			flush()
			title = strings.TrimSpace(m[1])
			count = 0
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		buf = append(buf, line)
	}
	flush()

	return out
}
