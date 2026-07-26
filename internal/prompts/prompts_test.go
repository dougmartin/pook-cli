package prompts

import (
	"slices"
	"testing"
)

// Ported from test/prompts.test.js.

func titlesAndTexts(prompts []Prompt) [][2]string {
	out := make([][2]string, len(prompts))
	for i, p := range prompts {
		out[i] = [2]string{p.Title, p.Text}
	}
	return out
}

func TestExtractTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "none when the text has no placeholders",
			text: "plain prompt, no tokens {not one} either",
			want: nil,
		},
		{
			name: "distinct names in order of first appearance",
			text: "{{a}} {{b}} {{a}}",
			want: []string{"a", "b"},
		},
		{
			name: "names are trimmed and empty tokens ignored",
			text: "{{ file path }} then {{file path}} and {{}}",
			want: []string{"file path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractTokens(tt.text); !slices.Equal(got, tt.want) {
				t.Errorf("ExtractTokens(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestFillTokens(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		values map[string]string
		want   string
	}{
		{
			name:   "substitutes every occurrence, including whitespace variants",
			text:   "{{a}}/{{ a }}, {{b}}",
			values: map[string]string{"a": "1", "b": "2"},
			want:   "1/1, 2",
		},
		{
			name:   "leaves unknown tokens untouched",
			text:   "{{a}} {{c}}",
			values: map[string]string{"a": "x"},
			want:   "x {{c}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FillTokens(tt.text, tt.values); got != tt.want {
				t.Errorf("FillTokens(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestHasInlineMarker(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"Work on story [STORY] next.", true},
		{"Open [FILE PATH] and [PR-2].", true},
		{"no markers [here] at all", false},
		// A "](" is a markdown link, not a marker. This is the case the
		// original expressed with a lookahead.
		{"see [API](https://example.com/docs)", false},
		{"- [ ] a checkbox", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := HasInlineMarker(tt.text); got != tt.want {
				t.Errorf("HasInlineMarker(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// The clipboard modal selects over these ranges, so their order and bounds
// matter as much as the count.
func TestMarkerIndexes(t *testing.T) {
	text := "Open [FILE PATH] then [PR-2], but not [API](url) or [lower]."

	got := MarkerIndexes(text)
	if len(got) != 2 {
		t.Fatalf("found %d markers, want 2: %v", len(got), got)
	}
	if s := text[got[0][0]:got[0][1]]; s != "[FILE PATH]" {
		t.Errorf("first marker = %q, want [FILE PATH]", s)
	}
	if s := text[got[1][0]:got[1][1]]; s != "[PR-2]" {
		t.Errorf("second marker = %q, want [PR-2]", s)
	}
}

func TestFormatMarkdownWritesAHeadingBeforeEveryPrompt(t *testing.T) {
	md := FormatMarkdown([]Prompt{{Title: "A", Text: "one"}, {Title: "B", Text: "two"}})

	if want := "## A\n\none\n\n## B\n\ntwo\n\n"; md != want {
		t.Errorf("markdown = %q, want %q", md, want)
	}
}

func TestMarkdownRoundTripsSingleParagraphPrompts(t *testing.T) {
	original := []Prompt{
		{Title: "Review", Text: "Review {{file}} for bugs."},
		{Title: "Explain", Text: "Explain this code line by line."},
	}

	got := ParseMarkdown(FormatMarkdown(original))
	if !slices.Equal(titlesAndTexts(got), titlesAndTexts(original)) {
		t.Errorf("round trip = %v, want %v", titlesAndTexts(got), titlesAndTexts(original))
	}
}

// A documented caveat inherited from the original: the importer splits
// paragraphs, so a multi-paragraph prompt comes back as several.
func TestMultiParagraphTextSplitsOnReimport(t *testing.T) {
	md := FormatMarkdown([]Prompt{{Title: "Two paras", Text: "first\n\nsecond"}})

	want := [][2]string{{"Two paras", "first"}, {"Two paras (2)", "second"}}
	if got := titlesAndTexts(ParseMarkdown(md)); !slices.Equal(got, want) {
		t.Errorf("reimport = %v, want %v", got, want)
	}
}

func TestParseMarkdown(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want [][2]string
	}{
		{
			name: "text before any heading is Untitled",
			md:   "loose paragraph\n",
			want: [][2]string{{"Untitled", "loose paragraph"}},
		},
		{
			name: "h1 and h3 headings both name prompts",
			md:   "# One\n\nfirst\n\n### Three\n\nthird\n",
			want: [][2]string{{"One", "first"}, {"Three", "third"}},
		},
		{
			name: "a heading with no body contributes nothing",
			md:   "## Empty\n\n## Real\n\nbody\n",
			want: [][2]string{{"Real", "body"}},
		},
		{
			name: "multi-line paragraphs stay together",
			md:   "## Wrapped\n\nline one\nline two\n",
			want: [][2]string{{"Wrapped", "line one\nline two"}},
		},
		{
			name: "empty input",
			md:   "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := titlesAndTexts(ParseMarkdown(tt.md)); !slices.Equal(got, tt.want) {
				t.Errorf("ParseMarkdown = %v, want %v", got, tt.want)
			}
		})
	}
}
