package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/dougmartin/pook-cli/internal/claude"
	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/monitor"
	"github.com/dougmartin/pook-cli/internal/oob"
)

// Branch tab.

func sampleBranch() git.BranchInfo {
	return git.BranchInfo{
		Name:    "feature/port",
		BaseRef: "main",
		Commits: []git.CommitEntry{
			{
				Hash: "aaaa111", Short: "aaaa111", Subject: "phase 4 Changes tab",
				Time: testNow.Add(-10 * time.Minute), Additions: 210, Deletions: 12, FileCount: 4,
			},
			{
				Hash: "bbbb222", Short: "bbbb222", Subject: "phase 3 watchers",
				Time: testNow.Add(-2 * time.Hour), Additions: 480, Deletions: 30, FileCount: 9,
			},
		},
		TotalAdditions: 690,
		TotalDeletions: 42,
		FilesTouched:   13,
	}
}

func branchTab(m Model) *BranchTab { return m.tabs[TabBranch].(*BranchTab) }

func branchModel(t *testing.T) Model {
	t.Helper()
	m := apply(newTestModel(t), refreshedMsg{Snap: monitor.Snapshot{Branch: sampleBranch()}})
	return press(m, "2")
}

func TestFrameBranchList(t *testing.T) {
	frame := branchModel(t).View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestBranchOverview(t *testing.T) {
	view := branchModel(t).View()

	for _, want := range []string{"feature/port", "2 ahead of main", "13 files", "+690 -42"} {
		if !strings.Contains(view, want) {
			t.Errorf("the overview is missing %q:\n%s", want, view)
		}
	}
}

// A branch with 200 commits must not load 200 diffs, so a commit loads only
// when it is opened, and then only once.
func TestCommitDiffsLoadLazilyAndCacheForever(t *testing.T) {
	m := branchModel(t)

	if len(branchTab(m).diffs) != 0 {
		t.Fatal("commit diffs were loaded before anything was expanded")
	}

	next, cmd := branchTab(m).Update(key(" "))
	tab := next.(*BranchTab)
	if cmd == nil {
		t.Fatal("expanding a commit did not request its diffs")
	}
	if !tab.loading["aaaa111"] {
		t.Error("the commit was not marked loading")
	}

	// Expanding again while it loads must not queue a second read.
	if again := tab.loadVisible(); again != nil {
		t.Error("a second load was issued while the first was in flight")
	}

	loaded := commitDiffMsg{Hash: "aaaa111", Files: []git.CommitFileDiff{
		{Path: "internal/ui/changes.go", Additions: 200, Deletions: 10,
			Diff: "diff --git a/x b/x\n@@ -1 +1 @@\n-old\n+new\n"},
	}}
	next, _ = tab.Update(loaded)
	tab = next.(*BranchTab)

	if len(tab.diffs["aaaa111"]) != 1 {
		t.Fatalf("diffs = %v, want one file", tab.diffs["aaaa111"])
	}
	if tab.loading["aaaa111"] {
		t.Error("the commit is still marked loading after arriving")
	}

	// Collapsing and reopening uses the cache: commits are immutable.
	tab.Update(key(" "))
	next, _ = tab.Update(key(" "))
	tab = next.(*BranchTab)
	if cmd := tab.loadVisible(); cmd != nil {
		t.Error("reopening a cached commit loaded it again")
	}
}

// A commit that fails to load caches as empty rather than retrying forever.
func TestCommitDiffFailureIsNotRetried(t *testing.T) {
	m := branchModel(t)
	tab := branchTab(m)

	tab.Update(key(" "))
	next, _ := tab.Update(commitDiffMsg{Hash: "aaaa111", Err: errNoEditor})
	tab = next.(*BranchTab)

	if cmd := tab.loadVisible(); cmd != nil {
		t.Error("a failed load was retried")
	}
}

// Commits made after the review mark are flagged, and the mark is set in the
// Changes tab.
func TestCommitsAfterTheMarkAreFlagged(t *testing.T) {
	m := branchModel(t)

	m = apply(m, markMsg{At: testNow.Add(-time.Hour)})
	view := m.View()

	lines := strings.Split(view, "\n")
	var recent, old string
	for _, l := range lines {
		if strings.Contains(l, "phase 4 Changes tab") {
			recent = l
		}
		if strings.Contains(l, "phase 3 watchers") {
			old = l
		}
	}

	if !strings.Contains(recent, "*") {
		t.Errorf("a commit after the mark is not flagged: %q", recent)
	}
	if strings.Contains(old, "*") {
		t.Errorf("a commit before the mark is flagged: %q", old)
	}
}

// Pressing m in the Changes tab is what sets the mark.
func TestMarkReviewedReachesTheBranchTab(t *testing.T) {
	m := changesModel(t)
	m = apply(m, refreshedMsg{Snap: monitor.Snapshot{Files: sampleFiles(), Branch: sampleBranch()}})

	next, cmd := changesTab(m).Update(key("m"))
	if cmd == nil {
		t.Fatal("marking reviewed produced no message for the other tabs")
	}
	_ = next

	msg, ok := cmd().(markMsg)
	if !ok {
		t.Fatalf("mark produced %T, want markMsg", cmd())
	}
	if msg.At.IsZero() {
		t.Error("the mark has no timestamp")
	}
}

// oob tab.

func sampleOOB() []oob.Group {
	return []oob.Group{
		{
			Namespace: oob.NamespaceBranch, Label: "branch: feature/port",
			Dir: "/home/doug/oob/branches/pook-cli/feature/port",
			Files: []oob.File{
				{Path: "notes.md", Namespace: oob.NamespaceBranch, Content: "port notes\nsecond line\n",
					FSPath: "/home/doug/oob/branches/pook-cli/feature/port/notes.md"},
			},
		},
		{
			Namespace: oob.NamespaceRepo, Label: "repo: pook-cli",
			Dir: "/home/doug/oob/repos/pook-cli",
			Files: []oob.File{
				{Path: "todo.md", Namespace: oob.NamespaceRepo, Content: "one thing\n",
					FSPath: "/home/doug/oob/repos/pook-cli/todo.md"},
				{Path: "sub/deep.txt", Namespace: oob.NamespaceRepo, Binary: true,
					FSPath: "/home/doug/oob/repos/pook-cli/sub/deep.txt"},
			},
		},
		{Namespace: oob.NamespaceGlobal, Label: "global", Dir: "/home/doug/oob/global"},
	}
}

func oobTab(m Model) *OOBTab { return m.tabs[TabOOB].(*OOBTab) }

func oobModel(t *testing.T) Model {
	t.Helper()
	m := apply(newTestModel(t), refreshedMsg{Snap: monitor.Snapshot{OOB: sampleOOB()}})
	return press(m, "6")
}

func TestFrameOOBList(t *testing.T) {
	frame := oobModel(t).View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

// Every namespace gets a header, even one with no files in it.
func TestOOBShowsEmptyNamespaces(t *testing.T) {
	view := oobModel(t).View()

	for _, want := range []string{"branch: feature/port", "repo: pook-cli", "global"} {
		if !strings.Contains(view, want) {
			t.Errorf("namespace %q is missing:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "0 files") {
		t.Errorf("the empty global namespace does not say it is empty:\n%s", view)
	}
}

// oob content is plain: a note that happens to start with a plus is not a
// diff line.
func TestOOBContentIsNotColorizedAsADiff(t *testing.T) {
	groups := sampleOOB()
	groups[0].Files[0].Content = "+not an addition\n-not a removal\n"

	m := apply(newTestModel(t), refreshedMsg{Snap: monitor.Snapshot{OOB: groups}})
	body := oobBody(groups[0].Files[0], 60)

	if len(body) != 2 {
		t.Fatalf("body = %v", body)
	}
	for _, line := range body {
		if line != styleContext.Render(strings.TrimRight(line, "")) && strings.Contains(line, "\x1b[3") {
			t.Errorf("oob content carries diff coloring: %q", line)
		}
	}
	_ = m
}

func TestOOBOpenOnANamespaceSaysSo(t *testing.T) {
	m := oobModel(t)
	// The cursor starts on the first namespace header.
	next, cmd := oobTab(m).Update(key("o"))
	tab := next.(*OOBTab)

	if cmd != nil {
		t.Error("opening a namespace launched an editor")
	}
	if !strings.Contains(tab.message, "namespace") {
		t.Errorf("message = %q, want it to explain", tab.message)
	}
}

func TestOOBFilesAreOpenable(t *testing.T) {
	m := oobModel(t)
	tab := oobTab(m)

	// Move onto the first real file.
	next, _ := tab.Update(key("j"))
	tab = next.(*OOBTab)

	row, ok := tab.acc.currentRow()
	if !ok {
		t.Fatal("no row under the cursor")
	}
	if _, isFile := tab.files[row.key]; !isFile {
		t.Fatalf("row %q is not a file", row.key)
	}
}

// Session tab.

func sampleMessages() []claude.Message {
	return []claude.Message{
		{Time: testNow.Add(-9 * time.Minute), Kind: claude.KindUser, Text: "port phase 5 please"},
		{Time: testNow.Add(-8 * time.Minute), Kind: claude.KindThinking, Text: "considering the tabs"},
		{Time: testNow.Add(-7 * time.Minute), Kind: claude.KindAssistant, Text: "Starting with the **Branch** tab."},
		{Time: testNow.Add(-6 * time.Minute), Kind: claude.KindTool, Tool: "Write", Text: "internal/ui/branch.go"},
		{Time: testNow.Add(-time.Minute), Kind: claude.KindUser, Text: "looks good"},
		{Time: testNow, Kind: claude.KindAssistant, Text: "Done."},
	}
}

func sessionTab(m Model) *SessionTab { return m.tabs[TabSession].(*SessionTab) }

func sessionModel(t *testing.T) Model {
	t.Helper()
	m := press(newTestModel(t), "4")

	tab := sessionTab(m)
	tab.now = fixedClock
	next, _ := tab.Update(sessionMsg{
		Tail:     &claude.SessionTail{File: "/tmp/session.jsonl"},
		Messages: sampleMessages(),
		Title:    "Porting pook",
	})
	m.tabs[TabSession] = next
	return m
}

func TestFrameSessionLive(t *testing.T) {
	frame := sessionModel(t).View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameSessionHistorical(t *testing.T) {
	frame := press(sessionModel(t), "h", "h").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

// The newest message is live; moving back anchors on history and stops
// following.
func TestSessionFollowsUntilYouMoveBack(t *testing.T) {
	m := sessionModel(t)
	tab := sessionTab(m)

	if !tab.following {
		t.Fatal("a fresh session is not following")
	}
	if tab.cursor != 5 {
		t.Fatalf("cursor = %d, want the newest message", tab.cursor)
	}
	if !strings.Contains(m.View(), "live") {
		t.Error("the live badge is missing")
	}

	m = press(m, "h")
	if sessionTab(m).following {
		t.Error("moving back kept following")
	}
	if strings.Contains(m.View(), "live") {
		t.Error("the live badge survived moving back")
	}

	// A new message arrives while the user is reading history: the position
	// holds.
	tab = sessionTab(m)
	before := tab.cursor
	next, _ := tab.Update(sessionMsg{
		Tail:     &claude.SessionTail{File: "/tmp/session.jsonl"},
		Messages: append(sampleMessages(), claude.Message{Kind: claude.KindAssistant, Text: "and more"}),
	})
	tab = next.(*SessionTab)
	if tab.cursor != before {
		t.Errorf("cursor moved to %d while anchored, want %d", tab.cursor, before)
	}
	if !tab.badge.Dot {
		t.Error("a message arriving while reading history raised no dot")
	}

	// $ returns to the live edge.
	next, _ = tab.Update(key("$"))
	tab = next.(*SessionTab)
	if !tab.following || tab.cursor != 6 {
		t.Errorf("$ did not jump to live: following=%v cursor=%d", tab.following, tab.cursor)
	}
}

func TestSessionUserJumps(t *testing.T) {
	m := sessionModel(t)
	tab := sessionTab(m)

	// From the newest message, H reaches the user message before it.
	next, _ := tab.Update(key("H"))
	tab = next.(*SessionTab)
	if tab.cursor != 4 {
		t.Fatalf("H moved to %d, want 4", tab.cursor)
	}

	next, _ = tab.Update(key("H"))
	tab = next.(*SessionTab)
	if tab.cursor != 0 {
		t.Fatalf("H moved to %d, want 0", tab.cursor)
	}

	// There is no earlier user message, so H does nothing.
	next, _ = tab.Update(key("H"))
	tab = next.(*SessionTab)
	if tab.cursor != 0 {
		t.Errorf("H moved past the first user message to %d", tab.cursor)
	}
	if tab.hasUser(-1) {
		t.Error("hasUser reports a previous user message from the first one")
	}

	next, _ = tab.Update(key("L"))
	tab = next.(*SessionTab)
	if tab.cursor != 4 {
		t.Errorf("L moved to %d, want 4", tab.cursor)
	}
}

// A new session file replaces the view and starts at its live edge.
func TestSessionSwitchesToANewSession(t *testing.T) {
	m := press(sessionModel(t), "h", "h") // anchored in history
	tab := sessionTab(m)

	next, _ := tab.Update(sessionMsg{
		Tail:     &claude.SessionTail{File: "/tmp/other.jsonl"},
		Messages: sampleMessages()[:2],
		Switched: true,
	})
	tab = next.(*SessionTab)

	if !tab.following {
		t.Error("a new session did not start live")
	}
	if tab.cursor != 1 {
		t.Errorf("cursor = %d, want the newest message of the new session", tab.cursor)
	}
}

// Messages dropping off the front shift every index, the cursor's included.
func TestSessionCursorSurvivesTheMessageCap(t *testing.T) {
	m := sessionModel(t)
	tab := sessionTab(m)

	next, _ := tab.Update(key("h")) // anchor at index 4
	tab = next.(*SessionTab)
	if tab.cursor != 4 {
		t.Fatalf("cursor = %d, want 4", tab.cursor)
	}

	// Two messages fell off the front.
	next, _ = tab.Update(sessionMsg{
		Tail:       &claude.SessionTail{File: "/tmp/session.jsonl"},
		Messages:   sampleMessages()[2:],
		StartIndex: 2,
	})
	tab = next.(*SessionTab)

	if tab.cursor != 2 {
		t.Errorf("cursor = %d, want 2 after two messages were dropped", tab.cursor)
	}
}

func TestSessionRendersEachKind(t *testing.T) {
	m := sessionModel(t)

	// The newest is an assistant message, rendered as markdown.
	if got := m.View(); !strings.Contains(got, "Done.") {
		t.Errorf("assistant text is missing:\n%s", got)
	}

	// A tool call shows its name and the one-line summary.
	m = press(m, "h", "h")
	view := m.View()
	if !strings.Contains(view, "Write") || !strings.Contains(view, "internal/ui/branch.go") {
		t.Errorf("the tool call is not summarized:\n%s", view)
	}
}

func TestSessionCopyPutsTheRawTextOnTheClipboard(t *testing.T) {
	tab := sessionTab(sessionModel(t))

	_, cmd := tab.Update(key("y"))
	if cmd == nil {
		t.Fatal("y produced no copy command")
	}
	// Running it would touch the real clipboard, so only the shape is
	// checked here; clip has its own tests.
	if tab.cursor != 5 {
		t.Errorf("copy moved the cursor to %d", tab.cursor)
	}
}

func TestSessionWithNoMessages(t *testing.T) {
	m := press(newTestModel(t), "4")
	view := m.View()

	if !strings.Contains(view, "no session found") {
		t.Errorf("an empty session does not say so:\n%s", view)
	}
	requireFrameSize(t, view, testWidth, testHeight)
}

// oob is a separate tool. On a machine that does not use it there is no tab,
// no key in help, and no digit that reaches it.
func TestOOBTabIsAbsentWithoutAnOOBHome(t *testing.T) {
	withoutOOB(t)

	m := New(gitRepoForTest(), nil, nil, nil).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})

	if got := len(m.tabs); got != 5 {
		t.Fatalf("tabs = %d, want 5 without an oob home", got)
	}
	for _, tab := range m.tabs {
		if tab.Title() == "oob" {
			t.Fatal("the oob tab is present with no oob home")
		}
	}

	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)
	if strings.Contains(frame, "oob") {
		t.Errorf("the tab bar still mentions oob:\n%s", frame)
	}

	// And it contributes no section to help. Its bindings are all shared with
	// other tabs, so the section heading is what has to be gone.
	help := scrollThroughHelp(t, m, testWidth, testHeight)
	if strings.Contains(help, "oob") {
		t.Errorf("help still has an oob section:\n%s", help)
	}
}

// The digit that would select it does nothing, rather than landing somewhere
// unexpected.
func TestTheOOBDigitDoesNothingWithoutTheTab(t *testing.T) {
	withoutOOB(t)

	m := New(gitRepoForTest(), nil, nil, nil).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})

	m = press(m, "4", "6")
	if m.active != TabSession {
		t.Errorf("active = %d, want it to stay on Session", m.active)
	}
}

// Cycling wraps over the four tabs that are there.
func TestCyclingWrapsWithoutTheOOBTab(t *testing.T) {
	withoutOOB(t)

	m := New(gitRepoForTest(), nil, nil, nil).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})

	m = press(m, "left")
	if m.active != TabPrompts {
		t.Fatalf("left from the first tab = %d, want Prompts as the last", m.active)
	}
	m = press(m, "right")
	if m.active != TabChanges {
		t.Errorf("right from the last tab = %d, want Changes", m.active)
	}
}

// A refresh reporting oob activity must not address a tab that is not there.
func TestOOBActivityIsIgnoredWithoutTheTab(t *testing.T) {
	withoutOOB(t)

	m := New(gitRepoForTest(), nil, nil, nil).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})

	m = apply(m, refreshedMsg{Snap: monitor.Snapshot{OOBChanged: true, BranchChanged: true}})

	if !m.tabs[TabBranch].Badge().Dot {
		t.Error("the Branch dot was lost")
	}
	requireFrameSize(t, m.View(), testWidth, testHeight)
}

// oob sits last, so every other tab keeps its position either way.
func TestTabOrder(t *testing.T) {
	m := newTestModel(t)

	want := []string{"Changes", "Branch", "Graph", "Session", "Prompts", "oob"}
	for i, title := range want {
		if i >= len(m.tabs) {
			t.Fatalf("only %d tabs, want %d", len(m.tabs), len(want))
		}
		if got := m.tabs[i].Title(); got != title {
			t.Errorf("tab %d = %q, want %q", i, got, title)
		}
	}
}

// The oob badge counts files across every namespace, the way the Prompts
// badge counts the whole library.
func TestOOBBadgeCountsFilesAcrossNamespaces(t *testing.T) {
	m := oobModel(t)

	// The fixture holds one branch file and two repo files, global is empty.
	if got := oobTab(m).Badge().Count; got != 3 {
		t.Errorf("badge = %d, want 3", got)
	}
	if !strings.Contains(stripStyles(m.View()), "oob 3") {
		t.Errorf("the tab bar does not show the count:\n%s", m.View())
	}

	// It follows the data rather than being set once.
	m = apply(m, refreshedMsg{Snap: monitor.Snapshot{OOB: []oob.Group{
		{Namespace: oob.NamespaceGlobal, Label: "global"},
	}}})
	if got := oobTab(m).Badge().Count; got != 0 {
		t.Errorf("badge = %d, want 0 once the files are gone", got)
	}
}
