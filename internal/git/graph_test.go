package git

import (
	"strings"
	"testing"
)

// graphRepo builds a repo whose history actually has a shape: main, a feature
// branch merged back into it, and a stacked branch on top.
func graphRepo(t *testing.T) Repo {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "init", "-q", "-b", "main")
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "Test")

	commit(t, dir, "a.txt", "main one")
	run(t, dir, "checkout", "-q", "-b", "feature")
	commit(t, dir, "b.txt", "feature one")
	run(t, dir, "checkout", "-q", "main")
	commit(t, dir, "c.txt", "main two")
	run(t, dir, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	run(t, dir, "checkout", "-q", "-b", "stacked")
	commit(t, dir, "d.txt", "stacked one")

	return Repo{Root: dir}
}

func TestGraphSeparatesCommitsFromContinuationRows(t *testing.T) {
	r := graphRepo(t)

	rows, err := r.Graph(GraphSubject, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("no rows")
	}

	var commits, continuations int
	for _, row := range rows {
		if row.Hash == "" {
			continuations++
			continue
		}
		commits++
		if len(row.Hash) != 40 {
			t.Errorf("hash = %q, want a full sha", row.Hash)
		}
	}

	// Five commits, and a merge means git has to draw at least one row that
	// is graph only.
	if commits != 5 {
		t.Errorf("commits = %d, want 5", commits)
	}
	if continuations == 0 {
		t.Error("a merge produced no continuation rows, so the split is not working")
	}
}

// The sentinel must never reach the screen.
func TestGraphStripsTheSentinel(t *testing.T) {
	rows, err := graphRepo(t).Graph(GraphSubject, 50, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range rows {
		if strings.Contains(row.Text, graphSentinel) {
			t.Fatalf("a sentinel survived into the display text: %q", row.Text)
		}
		if row.Hash != "" && strings.Contains(row.Text, row.Hash) {
			t.Errorf("the full hash leaked into the text: %q", row.Text)
		}
	}
}

func TestGraphSubjectAndCompact(t *testing.T) {
	r := graphRepo(t)

	subject, err := r.Graph(GraphSubject, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := r.Graph(GraphCompact, 50, false)
	if err != nil {
		t.Fatal(err)
	}

	if !containsText(subject, "stacked one") {
		t.Errorf("subject mode is missing the commit subject:\n%s", textOf(subject))
	}
	if containsText(compact, "stacked one") {
		t.Errorf("compact mode still carries subjects:\n%s", textOf(compact))
	}

	// Both label the refs, which is what makes a stack readable.
	if !containsText(compact, "stacked") {
		t.Errorf("compact mode lost the ref decorations:\n%s", textOf(compact))
	}
	// And compact really is shorter.
	if len(textOf(compact)) >= len(textOf(subject)) {
		t.Error("compact mode is not shorter than subject mode")
	}
}

// The graph column is git's, passed through untouched.
func TestGraphKeepsTheLaneCharacters(t *testing.T) {
	rows, err := graphRepo(t).Graph(GraphSubject, 50, false)
	if err != nil {
		t.Fatal(err)
	}

	text := textOf(rows)
	if !strings.Contains(text, "*") {
		t.Errorf("no commit markers in the graph:\n%s", text)
	}
	if !strings.Contains(text, "|") {
		t.Errorf("no lane characters in the graph:\n%s", text)
	}
}

func TestGraphHonorsTheLimit(t *testing.T) {
	rows, err := graphRepo(t).Graph(GraphSubject, 2, false)
	if err != nil {
		t.Fatal(err)
	}

	var commits int
	for _, row := range rows {
		if row.Hash != "" {
			commits++
		}
	}
	if commits != 2 {
		t.Errorf("commits = %d, want the limit of 2", commits)
	}
}

// A repo with no commits has no graph, and must not be an error the UI has to
// special case beyond showing nothing.
func TestGraphOnAnEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")

	rows, err := Repo{Root: dir}.Graph(GraphSubject, 50, false)
	if err == nil && len(rows) != 0 {
		t.Errorf("rows = %d, want none on an empty repo", len(rows))
	}
}

func TestSplitGraphLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantHash string
		wantText string
	}{
		{
			name:     "a commit row",
			line:     "* \x00abc123\x00abc123 subject",
			wantHash: "abc123",
			wantText: "* abc123 subject",
		},
		{
			name:     "a continuation row has no sentinel",
			line:     "|\\  ",
			wantHash: "",
			wantText: "|\\  ",
		},
		{
			name:     "a nested lane prefix is preserved exactly",
			line:     "| * | \x00deadbeef\x00deadbee more",
			wantHash: "deadbeef",
			wantText: "| * | deadbee more",
		},
		{
			name:     "an unterminated sentinel is left alone",
			line:     "* \x00abc123 no closer",
			wantHash: "",
			wantText: "* \x00abc123 no closer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, text := splitGraphLine(tt.line)
			if hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", hash, tt.wantHash)
			}
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func containsText(rows []GraphRow, want string) bool {
	return strings.Contains(textOf(rows), want)
}

func textOf(rows []GraphRow) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.Text)
		b.WriteString("\n")
	}
	return b.String()
}
