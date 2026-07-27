package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/monitor"
)

// sampleFiles is a working tree with one of everything: a modified file, a new
// one, a deleted one, a watched one and a binary.
func sampleFiles() []git.FileEntry {
	return []git.FileEntry{
		{
			Path: "internal/ui/app.go", Status: git.StatusChanged,
			Additions: 12, Deletions: 3,
			ModTime: testNow.Add(-30 * time.Second),
			Diff: "diff --git a/internal/ui/app.go b/internal/ui/app.go\n" +
				"index 1234567..89abcde 100644\n" +
				"--- a/internal/ui/app.go\n" +
				"+++ b/internal/ui/app.go\n" +
				"@@ -10,6 +10,7 @@ func (m Model) Update() {\n" +
				" \tswitch msg := msg.(type) {\n" +
				"-\tcase oldMsg:\n" +
				"+\tcase newMsg:\n" +
				"+\t\tm.handled = true\n" +
				" \t}\n",
		},
		{
			Path: "README.md", Status: git.StatusNew,
			Additions: 4,
			ModTime:   testNow.Add(-2 * time.Minute),
			Diff:      "diff --git a/README.md b/README.md\n@@ -0,0 +1 @@\n+hello\n",
		},
		{
			Path: "old/gone.go", Status: git.StatusDeleted,
			Deletions: 40,
			ModTime:   testNow.Add(-10 * time.Minute),
			Diff:      "diff --git a/old/gone.go b/old/gone.go\n@@ -1,2 +0,0 @@\n-package old\n",
		},
		{
			Path: "package.json", Status: git.StatusChanged,
			Additions: 2, Deletions: 1, Watched: true,
			ModTime: testNow.Add(-5 * time.Second),
			Diff:    "diff --git a/package.json b/package.json\n@@ -1 +1 @@\n-{}\n+{\"name\":\"x\"}\n",
		},
		{
			Path: "media/logo.png", Status: git.StatusNew, Binary: true,
			ModTime: testNow.Add(-time.Hour),
			Diff:    "diff --git a/media/logo.png b/media/logo.png\nBinary files /dev/null and b/media/logo.png differ\n",
		},
	}
}

// changesModel is a shell whose Changes tab holds the sample working tree.
func changesModel(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	return apply(m, refreshedMsg{Snap: monitor.Snapshot{Files: sampleFiles()}})
}

func changesTab(m Model) *ChangesTab { return m.tabs[TabChanges].(*ChangesTab) }

// visiblePaths is the file list as the tab would show it.
func visiblePaths(t *ChangesTab) []string {
	files := t.visibleFiles()
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func TestFrameChangesList(t *testing.T) {
	frame := changesModel(t).View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameChangesExpanded(t *testing.T) {
	// The default sort is newest first, so package.json leads.
	frame := press(changesModel(t), " ").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameChangesExpandAll(t *testing.T) {
	frame := press(changesModel(t), "z", "R").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameChangesDiscardConfirmation(t *testing.T) {
	frame := press(changesModel(t), "d").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameChangesFiltering(t *testing.T) {
	m := press(changesModel(t), "/")
	m = press(m, "u", "i")
	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameChangesEmpty(t *testing.T) {
	m := apply(newTestModel(t), refreshedMsg{})
	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestStatusFilterCycles(t *testing.T) {
	m := changesModel(t)

	want := [][]string{
		{"package.json", "internal/ui/app.go", "README.md", "old/gone.go", "media/logo.png"}, // all
		{"README.md", "media/logo.png"},        // new
		{"package.json", "internal/ui/app.go"}, // changed
		{"old/gone.go"},                        // deleted
		{"package.json", "internal/ui/app.go", "README.md", "old/gone.go", "media/logo.png"}, // back to all
	}

	for i, expect := range want {
		got := visiblePaths(changesTab(m))
		if !equalStrings(got, expect) {
			t.Fatalf("filter step %d: %v, want %v", i, got, expect)
		}
		m = press(m, "f")
	}
}

func TestSortCycles(t *testing.T) {
	m := changesModel(t)

	want := [][]string{
		// latest first
		{"package.json", "internal/ui/app.go", "README.md", "old/gone.go", "media/logo.png"},
		// oldest first
		{"media/logo.png", "old/gone.go", "README.md", "internal/ui/app.go", "package.json"},
		// a to z
		{"README.md", "internal/ui/app.go", "media/logo.png", "old/gone.go", "package.json"},
		// most changes: gone.go removed 40 lines, app.go moved 15
		{"old/gone.go", "internal/ui/app.go", "README.md", "package.json", "media/logo.png"},
	}

	for i, expect := range want {
		got := visiblePaths(changesTab(m))
		if !equalStrings(got, expect) {
			t.Fatalf("sort step %d: %v, want %v", i, got, expect)
		}
		m = press(m, "s")
	}
}

func TestPathFilter(t *testing.T) {
	m := press(changesModel(t), "/")
	if !changesTab(m).CapturingInput() {
		t.Fatal("the filter did not take focus")
	}

	m = press(m, "u", "i")
	if got := visiblePaths(changesTab(m)); !equalStrings(got, []string{"internal/ui/app.go"}) {
		t.Errorf("filtered to %v, want [internal/ui/app.go]", got)
	}

	// enter keeps the filter and gives the keyboard back.
	m = press(m, "enter")
	if changesTab(m).CapturingInput() {
		t.Error("enter did not release the input")
	}
	if got := visiblePaths(changesTab(m)); len(got) != 1 {
		t.Errorf("the filter did not survive enter: %v", got)
	}

	// esc clears it.
	m = press(m, "esc")
	if got := visiblePaths(changesTab(m)); len(got) != 5 {
		t.Errorf("esc did not clear the filter: %v", got)
	}
}

// While the filter has focus, keys belong to it rather than the global keymap.
func TestFilterInputOutranksGlobalKeys(t *testing.T) {
	m := press(changesModel(t), "/")
	m = press(m, "a", "c", "r", "q")

	if m.overlay != nil || m.modal != nil {
		t.Fatal("typing in the filter opened a layer")
	}
	if got := changesTab(m).pathQuery; got != "acrq" {
		t.Errorf("filter text = %q, want acrq", got)
	}
}

func TestFilterIsCaseInsensitive(t *testing.T) {
	m := press(changesModel(t), "/")
	m = press(m, "R", "E", "A", "D")

	if got := visiblePaths(changesTab(m)); !equalStrings(got, []string{"README.md"}) {
		t.Errorf("filtered to %v, want [README.md]", got)
	}
}

func TestExpandAndCollapse(t *testing.T) {
	m := changesModel(t)
	tab := changesTab(m)

	if tab.acc.isExpanded("package.json") {
		t.Fatal("rows start expanded")
	}

	m = press(m, " ")
	if !changesTab(m).acc.isExpanded("package.json") {
		t.Fatal("space did not expand the row under the cursor")
	}

	m = press(m, " ")
	if changesTab(m).acc.isExpanded("package.json") {
		t.Fatal("space did not collapse it again")
	}

	m = press(m, "z", "R")
	for _, f := range sampleFiles() {
		if !changesTab(m).acc.isExpanded(f.Path) {
			t.Errorf("zR did not expand %s", f.Path)
		}
	}

	m = press(m, "z", "M")
	for _, f := range sampleFiles() {
		if changesTab(m).acc.isExpanded(f.Path) {
			t.Errorf("zM did not collapse %s", f.Path)
		}
	}
}

// Expanding a row is what marks it seen, so the stale marker only appears for
// a file that moved on afterwards.
func TestStaleMarker(t *testing.T) {
	m := press(changesModel(t), " ") // expand package.json
	tab := changesTab(m)

	files := sampleFiles()
	if tab.staleSinceExpanded(files[3]) {
		t.Fatal("a file is stale immediately after being expanded")
	}

	moved := files[3]
	moved.Diff += "+more\n"
	if !tab.staleSinceExpanded(moved) {
		t.Error("a file whose diff changed since it was expanded is not marked")
	}

	// A file never expanded is not marked either way.
	if tab.staleSinceExpanded(files[0]) {
		t.Error("a file that was never expanded is marked stale")
	}
}

func TestMarkReviewedAndSinceMark(t *testing.T) {
	m := press(changesModel(t), "m") // mark everything reviewed
	m = press(m, "M")                // and show only what changed since

	if got := visiblePaths(changesTab(m)); len(got) != 0 {
		t.Fatalf("since-mark shows %v right after marking, want nothing", got)
	}

	// One file moves on, and a new one appears.
	files := sampleFiles()
	files[0].Diff += "+another line\n"
	files = append(files, git.FileEntry{
		Path: "fresh.go", Status: git.StatusNew, Additions: 1,
		ModTime: testNow, Diff: "diff --git a/fresh.go b/fresh.go\n@@ -0,0 +1 @@\n+new\n",
	})
	m = apply(m, refreshedMsg{Snap: monitor.Snapshot{Files: files}})

	want := []string{"fresh.go", "internal/ui/app.go"}
	if got := visiblePaths(changesTab(m)); !equalStrings(got, want) {
		t.Errorf("since-mark shows %v, want %v", got, want)
	}

	// Toggling it off brings everything back.
	m = press(m, "M")
	if got := visiblePaths(changesTab(m)); len(got) != 6 {
		t.Errorf("since-mark off shows %d files, want 6", len(got))
	}
}

// Discarding asks first, and only acts on a yes.
func TestDiscardIsConfirmed(t *testing.T) {
	m := press(changesModel(t), "d")
	if changesTab(m).confirm != "package.json" {
		t.Fatalf("confirm = %q, want package.json", changesTab(m).confirm)
	}
	if !strings.Contains(m.View(), "discard package.json?") {
		t.Error("the confirmation is not on screen")
	}

	// While it is up, other keys do nothing.
	m = press(m, "j", "f", "s")
	if changesTab(m).confirm != "package.json" {
		t.Error("another key dismissed the confirmation")
	}

	m = press(m, "n")
	if changesTab(m).confirm != "" {
		t.Error("n did not cancel")
	}
}

func TestDiscardCancelledByEsc(t *testing.T) {
	m := press(changesModel(t), "d", "esc")
	if changesTab(m).confirm != "" {
		t.Error("esc did not cancel the confirmation")
	}
}

// A confirmed discard runs against the repo and asks for a refresh.
func TestDiscardRunsAndRefreshes(t *testing.T) {
	repo := makeDirtyRepo(t)
	tab := NewChangesTab(repo)

	files, err := repo.CollectChanges()
	if err != nil {
		t.Fatal(err)
	}
	tab.setFiles(files)

	next, _ := tab.Update(key("d"))
	tab = next.(*ChangesTab)
	if tab.confirm == "" {
		t.Fatal("d did not ask for confirmation")
	}
	target := tab.confirm

	next, cmd := tab.Update(key("y"))
	tab = next.(*ChangesTab)
	if cmd == nil {
		t.Fatal("y produced no discard command")
	}

	msg, ok := cmd().(discardedMsg)
	if !ok {
		t.Fatalf("discard produced %T", cmd())
	}
	if msg.Err != nil {
		t.Fatalf("discard failed: %v", msg.Err)
	}

	// And the tab asks the shell to re-read the repo.
	next, cmd = tab.Update(msg)
	tab = next.(*ChangesTab)
	if cmd == nil {
		t.Fatal("a completed discard did not request a refresh")
	}
	if _, ok := cmd().(needsRefreshMsg); !ok {
		t.Errorf("discard follow-up = %T, want needsRefreshMsg", cmd())
	}
	if !strings.Contains(tab.message, target) {
		t.Errorf("message = %q, want it to name %s", tab.message, target)
	}
}

// The cursor and the reader's place in a long diff survive a refresh.
func TestRefreshKeepsCursorAndExpansion(t *testing.T) {
	m := press(changesModel(t), "j", " ", "j", "j")
	before := changesTab(m).acc.cursor

	m = apply(m, refreshedMsg{Snap: monitor.Snapshot{Files: sampleFiles()}})

	tab := changesTab(m)
	if !tab.acc.isExpanded("internal/ui/app.go") {
		t.Error("expansion did not survive the refresh")
	}
	if tab.acc.cursor != before {
		t.Errorf("cursor = %d, want %d", tab.acc.cursor, before)
	}
}

func TestBadgeCountsFiles(t *testing.T) {
	m := changesModel(t)
	if got := changesTab(m).Badge().Count; got != 5 {
		t.Errorf("badge = %d, want 5", got)
	}
}

func TestSummaryLine(t *testing.T) {
	m := changesModel(t)
	view := m.View()

	for _, want := range []string{"5/5 files", "+18 -44", "filter all", "sort latest"} {
		if !strings.Contains(view, want) {
			t.Errorf("the summary is missing %q:\n%s", want, strings.SplitN(view, "\n", 3)[1])
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// makeDirtyRepo is a repo with one committed file that has been edited.
func makeDirtyRepo(t *testing.T) git.Repo {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := execCommand(t, dir, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	writeTestFile(t, dir, "tracked.txt", "one\n")
	run("add", "-A")
	run("commit", "-q", "-m", "first")
	writeTestFile(t, dir, "tracked.txt", "one\ntwo\n")

	return git.Repo{Root: dir}
}

var _ tea.Model = Model{}

// The Changes path filter behaves the same way: arrows move the list, letters
// keep typing.
func TestArrowsMoveTheListWhileFiltering(t *testing.T) {
	m := press(changesModel(t), "/")
	m = press(m, "o") // matches several paths

	tab := changesTab(m)
	if len(tab.visibleFiles()) < 2 {
		t.Fatalf("filter left %d files, need at least two to move between", len(tab.visibleFiles()))
	}
	first := tab.acc.cursor

	m = press(m, "down")
	tab = changesTab(m)
	if tab.acc.cursor == first {
		t.Error("down did not move the filtered list")
	}
	if tab.pathQuery != "o" {
		t.Errorf("query = %q, want it unchanged by the arrow", tab.pathQuery)
	}
	if !tab.CapturingInput() {
		t.Error("moving the cursor dropped focus from the filter")
	}
}
