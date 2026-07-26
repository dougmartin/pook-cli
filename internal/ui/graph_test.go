package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/monitor"
)

// sampleGraph is what git would hand back for a small stack with a merge in
// it: commit rows carrying hashes, and pure-graph continuation rows between
// them.
func sampleGraph() []git.GraphRow {
	return []git.GraphRow{
		{Hash: strings.Repeat("a", 40), Text: "* aaaaaaaaa (HEAD -> stacked) stacked one"},
		{Hash: strings.Repeat("b", 40), Text: "* bbbbbbbbb (feature) merge feature"},
		{Text: "|\\  "},
		{Hash: strings.Repeat("c", 40), Text: "| * ccccccccc feature one"},
		{Hash: strings.Repeat("d", 40), Text: "* | ddddddddd (main) main two"},
		{Text: "|/  "},
		{Hash: strings.Repeat("e", 40), Text: "* eeeeeeeee main one"},
	}
}

func graphTab(m Model) *GraphTab { return m.tabs[TabGraph].(*GraphTab) }

func graphModel(t *testing.T) Model {
	t.Helper()
	m := apply(newTestModel(t), refreshedMsg{
		Snap: monitor.Snapshot{Branch: git.BranchInfo{Name: "stacked"}},
	})
	m = press(m, "3")

	next, _ := graphTab(m).Update(graphMsg{Rows: sampleGraph()})
	m.tabs[TabGraph] = next
	return m
}

func TestFrameGraph(t *testing.T) {
	frame := graphModel(t).View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

// The graph is passed through exactly as git drew it, lanes and all.
func TestGraphRendersGitsOwnLanes(t *testing.T) {
	view := stripStyles(graphModel(t).View())

	for _, want := range []string{
		"* aaaaaaaaa (HEAD -> stacked) stacked one",
		"|\\",
		"| * ccccccccc feature one",
		"|/",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the graph is missing %q:\n%s", want, view)
		}
	}
}

func TestGraphSummary(t *testing.T) {
	view := stripStyles(graphModel(t).View())

	// Five of the seven rows are commits; the other two are graph only.
	for _, want := range []string{"stacked", "5 commits", "subject"} {
		if !strings.Contains(view, want) {
			t.Errorf("the summary is missing %q:\n%s", want, view)
		}
	}
}

// s toggles the detail level and asks git for the graph again.
func TestGraphDetailToggle(t *testing.T) {
	m := graphModel(t)
	tab := graphTab(m)

	if tab.detail != git.GraphSubject {
		t.Fatal("the graph did not open in subject mode")
	}

	next, cmd := tab.Update(key("s"))
	tab = next.(*GraphTab)
	if tab.detail != git.GraphCompact {
		t.Error("s did not switch to compact")
	}
	if cmd == nil {
		t.Error("switching detail did not re-render the graph")
	}

	// While that load is in flight a second one is not queued.
	if again := tab.loadCmd(); again != nil {
		t.Error("a second load was issued while the first was running")
	}

	next, _ = tab.Update(graphMsg{Rows: sampleGraph()})
	tab = next.(*GraphTab)
	next, _ = tab.Update(key("s"))
	tab = next.(*GraphTab)
	if tab.detail != git.GraphSubject {
		t.Error("s did not toggle back to subject")
	}
}

// The cursor moves over every row, but only a commit row has a hash to act on.
func TestGraphCopiesOnlyFromCommitRows(t *testing.T) {
	m := graphModel(t)
	tab := graphTab(m)

	if got := tab.currentHash(); got != strings.Repeat("a", 40) {
		t.Fatalf("hash under the cursor = %q, want the first commit", got)
	}

	next, cmd := tab.Update(key("y"))
	tab = next.(*GraphTab)
	if cmd == nil {
		t.Fatal("y produced no copy command on a commit row")
	}

	// Move onto the continuation row between the merge and its parent.
	for range 2 {
		next, _ = tab.Update(key("j"))
		tab = next.(*GraphTab)
	}
	if got := tab.currentHash(); got != "" {
		t.Errorf("a continuation row reported hash %q", got)
	}
	if _, cmd := tab.Update(key("y")); cmd != nil {
		t.Error("y produced a copy command on a row with no commit")
	}
}

// A refresh keeps the cursor on the same commit rather than the same row
// number, since new commits arrive at the top.
func TestGraphCursorSticksToItsCommit(t *testing.T) {
	m := graphModel(t)
	tab := graphTab(m)

	next, _ := tab.Update(key("j"))
	tab = next.(*GraphTab)
	want := tab.currentHash()

	// A new commit lands on top, pushing everything down a row.
	grown := append([]git.GraphRow{
		{Hash: strings.Repeat("f", 40), Text: "* fffffffff (HEAD -> stacked) newest"},
	}, sampleGraph()...)

	next, _ = tab.Update(graphMsg{Rows: grown})
	tab = next.(*GraphTab)

	if got := tab.currentHash(); got != want {
		t.Errorf("cursor moved to %q, want it to stay on %q", got, want)
	}
}

func TestGraphWithNoHistory(t *testing.T) {
	m := press(newTestModel(t), "3")

	view := m.View()
	if !strings.Contains(view, "no commits yet") {
		t.Errorf("an empty graph does not say so:\n%s", view)
	}
	requireFrameSize(t, view, testWidth, testHeight)
}

// The Graph tab sits beside Branch, since they answer related questions.
func TestGraphTabPosition(t *testing.T) {
	m := newTestModel(t)

	if got := m.tabs[TabGraph].Title(); got != "Graph" {
		t.Errorf("tab %d = %q, want Graph", TabGraph, got)
	}
	if TabGraph != TabBranch+1 {
		t.Errorf("Graph is at %d, want it next to Branch at %d", TabGraph, TabBranch)
	}
}

// New commits raise a dot on Graph as well as Branch, since both show them.
func TestGraphGetsADotForNewCommits(t *testing.T) {
	m := apply(newTestModel(t), refreshedMsg{
		Snap: monitor.Snapshot{BranchChanged: true},
	})

	if !m.tabs[TabGraph].Badge().Dot {
		t.Error("a new commit raised no dot on the Graph tab")
	}
}

// A copy says so, the way every other tab does.
func TestGraphConfirmsACopy(t *testing.T) {
	m := graphModel(t)
	tab := graphTab(m)

	next, _ := tab.Update(copiedMsg{What: "commit aaaaaaaaa"})
	tab = next.(*GraphTab)

	if !strings.Contains(tab.message, "copied commit aaaaaaaaa") {
		t.Errorf("message = %q", tab.message)
	}

	m.tabs[TabGraph] = tab
	view := m.View()
	if !strings.Contains(stripStyles(view), "copied commit aaaaaaaaa") {
		t.Errorf("the confirmation is not on screen:\n%s", view)
	}
	requireFrameSize(t, view, testWidth, testHeight)
}

func TestGraphReportsAFailedCopy(t *testing.T) {
	tab := graphTab(graphModel(t))

	next, _ := tab.Update(copiedMsg{Err: errNoEditor})
	if got := next.(*GraphTab).message; !strings.Contains(got, "copy failed") {
		t.Errorf("message = %q", got)
	}
}
