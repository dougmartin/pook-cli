package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The spec calls out the exact transcript behaviors that must be reproduced,
// and src/claude.ts had no tests of its own, so these are written from both.

func TestTranscriptDirSanitizesTheRoot(t *testing.T) {
	t.Setenv("HOME", "/home/tester")

	got := TranscriptDir("/home/doug/projects/pook-cli")
	want := "/home/tester/.claude/projects/-home-doug-projects-pook-cli"
	if got != want {
		t.Errorf("TranscriptDir = %q, want %q", got, want)
	}
}

func TestTranscriptDirReplacesEveryNonAlphanumeric(t *testing.T) {
	t.Setenv("HOME", "/home/tester")

	got := filepath.Base(TranscriptDir("/a/b_c.d-e"))
	if want := "-a-b-c-d-e"; got != want {
		t.Errorf("sanitized = %q, want %q", got, want)
	}
}

func TestFindLatestSessionPicksTheNewestTranscript(t *testing.T) {
	root, dir := fakeTranscriptDir(t)

	writeSession(t, dir, "old.jsonl", "", time.Now().Add(-time.Hour))
	writeSession(t, dir, "new.jsonl", "", time.Now())
	// Not a transcript, even though it is newer.
	writeSession(t, dir, "notes.txt", "", time.Now().Add(time.Hour))

	if got := FindLatestSession(root); filepath.Base(got) != "new.jsonl" {
		t.Errorf("FindLatestSession = %q, want new.jsonl", got)
	}
}

func TestFindLatestSessionWithNoTranscripts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if got := FindLatestSession("/no/such/project"); got != "" {
		t.Errorf("FindLatestSession = %q, want empty", got)
	}
}

func TestSessionTailReadsOnlyWhatWasAppended(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.jsonl")
	writeLines(t, file, userLine("first"))

	tail := NewSessionTail(file)
	if !tail.ReadNew() {
		t.Fatal("the first read reported nothing")
	}
	if len(tail.Messages) != 1 || tail.Messages[0].Text != "first" {
		t.Fatalf("messages = %+v, want one saying first", tail.Messages)
	}
	if tail.ID != "session" {
		t.Errorf("id = %q, want session", tail.ID)
	}

	// Nothing appended: nothing to report, and no duplicate.
	if tail.ReadNew() {
		t.Error("an unchanged file reported new messages")
	}
	if len(tail.Messages) != 1 {
		t.Errorf("messages = %d, want 1", len(tail.Messages))
	}

	appendLines(t, file, userLine("second"))
	if !tail.ReadNew() {
		t.Fatal("an appended line was not picked up")
	}
	if len(tail.Messages) != 2 || tail.Messages[1].Text != "second" {
		t.Fatalf("messages = %+v, want first then second", tail.Messages)
	}
}

// A line still being written must not be parsed until the rest of it lands.
func TestSessionTailHoldsBackAPartialLine(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.jsonl")
	writeLines(t, file, userLine("complete"))

	tail := NewSessionTail(file)
	tail.ReadNew()

	// Append half a record, with no newline to close it.
	half := userLine("partial")
	appendRaw(t, file, half[:len(half)/2])

	tail.ReadNew()
	if len(tail.Messages) != 1 {
		t.Fatalf("a half written line was parsed: %+v", tail.Messages)
	}

	appendRaw(t, file, half[len(half)/2:]+"\n")
	if !tail.ReadNew() {
		t.Fatal("the completed line was not picked up")
	}
	if len(tail.Messages) != 2 || tail.Messages[1].Text != "partial" {
		t.Fatalf("messages = %+v, want the completed line", tail.Messages)
	}
}

// A shorter file is a different file, so the tail starts over rather than
// reading from a stale offset.
func TestSessionTailRestartsWhenTheFileShrinks(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.jsonl")
	writeLines(t, file, userLine("one"), userLine("two"), userLine("three"))

	tail := NewSessionTail(file)
	tail.ReadNew()
	if len(tail.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(tail.Messages))
	}

	writeLines(t, file, userLine("replaced"))
	if !tail.ReadNew() {
		t.Fatal("the replaced file was not re-read")
	}
	if len(tail.Messages) != 1 || tail.Messages[0].Text != "replaced" {
		t.Fatalf("messages = %+v, want just the replacement", tail.Messages)
	}
	if tail.StartIndex != 0 {
		t.Errorf("startIndex = %d, want 0 after a restart", tail.StartIndex)
	}
}

// Past the cap, messages drop off the front and StartIndex accounts for them,
// so a position can still be reported against the whole session.
func TestSessionTailCapsMessagesAndTracksTheStartIndex(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.jsonl")

	lines := make([]string, maxMessages+50)
	for i := range lines {
		lines[i] = userLine(fmt.Sprintf("message %d", i))
	}
	writeLines(t, file, lines...)

	tail := NewSessionTail(file)
	tail.ReadNew()

	if len(tail.Messages) != maxMessages {
		t.Fatalf("messages = %d, want %d", len(tail.Messages), maxMessages)
	}
	if tail.StartIndex != 50 {
		t.Errorf("startIndex = %d, want 50", tail.StartIndex)
	}
	if want := "message 50"; tail.Messages[0].Text != want {
		t.Errorf("first kept message = %q, want %q", tail.Messages[0].Text, want)
	}
}

func TestSessionTailReadsTheTitle(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.jsonl")
	writeLines(t, file, `{"type":"ai-title","aiTitle":"Porting the git backend"}`, userLine("hi"))

	tail := NewSessionTail(file)
	tail.ReadNew()

	if tail.Title != "Porting the git backend" {
		t.Errorf("title = %q", tail.Title)
	}
	if len(tail.Messages) != 1 {
		t.Errorf("the title line became a message: %+v", tail.Messages)
	}
}

func TestSessionTailSkipsMalformedLines(t *testing.T) {
	file := filepath.Join(t.TempDir(), "session.jsonl")
	writeLines(t, file, "not json at all", "", userLine("survivor"))

	tail := NewSessionTail(file)
	if !tail.ReadNew() {
		t.Fatal("nothing was read")
	}
	if len(tail.Messages) != 1 || tail.Messages[0].Text != "survivor" {
		t.Fatalf("messages = %+v, want just the valid line", tail.Messages)
	}
}

func TestSessionTailOnAMissingFile(t *testing.T) {
	tail := NewSessionTail(filepath.Join(t.TempDir(), "gone.jsonl"))
	if tail.ReadNew() {
		t.Error("a missing file reported messages")
	}
}

func TestExtractMessages(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []Message
	}{
		{
			name: "a user string body",
			line: `{"type":"user","message":{"content":"hello"}}`,
			want: []Message{{Kind: KindUser, Text: "hello"}},
		},
		{
			name: "system reminders are stripped from user text",
			line: `{"type":"user","message":{"content":"real text<system-reminder>ignore\nme</system-reminder>"}}`,
			want: []Message{{Kind: KindUser, Text: "real text"}},
		},
		{
			name: "ide blocks are stripped too",
			line: `{"type":"user","message":{"content":"<ide_selection>x</ide_selection>kept"}}`,
			want: []Message{{Kind: KindUser, Text: "kept"}},
		},
		{
			name: "a user message that is only markup is dropped",
			line: `{"type":"user","message":{"content":"<command-name>/loop</command-name>"}}`,
			want: nil,
		},
		{
			name: "assistant text and thinking are separate messages",
			line: `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"  pondering  "},{"type":"text","text":"  answer  "}]}}`,
			want: []Message{
				{Kind: KindThinking, Text: "pondering"},
				{Kind: KindAssistant, Text: "answer"},
			},
		},
		{
			name: "a tool call is named and summarized",
			line: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}}]}}`,
			want: []Message{{Kind: KindTool, Tool: "Bash", Text: "go test ./..."}},
		},
		{
			name: "a sidechain entry belongs to a subagent, not this conversation",
			line: `{"type":"user","isSidechain":true,"message":{"content":"subagent chatter"}}`,
			want: nil,
		},
		{
			name: "empty assistant text is not a message",
			line: `{"type":"assistant","message":{"content":[{"type":"text","text":"   "}]}}`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLine(t, tt.line)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d messages, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i].Kind != tt.want[i].Kind || got[i].Text != tt.want[i].Text || got[i].Tool != tt.want[i].Tool {
					t.Errorf("message %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSummarizeToolInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "the most descriptive field wins",
			input: `{"command":"ls","description":"List files"}`,
			want:  "List files",
		},
		{
			name:  "falls through to a later key",
			input: `{"url":"https://example.com"}`,
			want:  "https://example.com",
		},
		{
			name:  "a questions array is summarized by its first question",
			input: `{"questions":[{"question":"Which one?"},{"question":"And this?"}]}`,
			want:  "Which one?",
		},
		{
			name:  "otherwise the first list field is counted",
			input: `{"edits":[1,2,3]}`,
			want:  "edits × 3",
		},
		{
			name:  "an empty object summarizes to nothing",
			input: `{}`,
			want:  "",
		},
		{
			name:  "blank strings are skipped",
			input: `{"command":"   ","path":"/tmp/x"}`,
			want:  "/tmp/x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeToolInput([]byte(tt.input)); got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("a", maxTextChars+10)

	got := truncate(long, maxTextChars)
	if !strings.HasSuffix(got, " …") {
		t.Error("truncated text has no marker")
	}
	if n := len([]rune(got)); n != maxTextChars+2 {
		t.Errorf("truncated to %d runes, want %d", n, maxTextChars+2)
	}

	// Truncation counts runes, so it can never split a character.
	multibyte := strings.Repeat("é", 10)
	if got := truncate(multibyte, 5); got != "ééééé …" {
		t.Errorf("multibyte truncation = %q", got)
	}
}

func TestMessageTimeIsParsed(t *testing.T) {
	got := parseLine(t, `{"type":"user","timestamp":"2026-07-26T13:45:00Z","message":{"content":"hi"}}`)

	if len(got) != 1 {
		t.Fatalf("messages = %+v", got)
	}
	if want := time.Date(2026, 7, 26, 13, 45, 0, 0, time.UTC); !got[0].Time.Equal(want) {
		t.Errorf("time = %v, want %v", got[0].Time, want)
	}
}

// Helpers.

// parseLine runs one transcript line through a tail, which is the only way
// entries are read in production.
func parseLine(t *testing.T, line string) []Message {
	t.Helper()
	file := filepath.Join(t.TempDir(), "session.jsonl")
	writeLines(t, file, line)

	tail := NewSessionTail(file)
	tail.ReadNew()
	return tail.Messages
}

func userLine(text string) string {
	return fmt.Sprintf(`{"type":"user","message":{"content":%q}}`, text)
}

func writeLines(t *testing.T, file string, lines ...string) {
	t.Helper()
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendLines(t *testing.T, file string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		appendRaw(t, file, l+"\n")
	}
}

func appendRaw(t *testing.T, file, s string) {
	t.Helper()
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

// fakeTranscriptDir points HOME at a temp dir and returns a workspace root
// along with its transcript directory.
func fakeTranscriptDir(t *testing.T) (root, dir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	root = "/home/doug/projects/pook-cli"
	dir = TranscriptDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func writeSession(t *testing.T, dir, name, body string, mod time.Time) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(full, mod, mod); err != nil {
		t.Fatal(err)
	}
}
