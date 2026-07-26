package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/dougmartin/pook-cli/internal/monitor"
)

// The golden frames below are the regression net for layout math, tab bar
// state and status bar composition. Regenerate with: go test ./... -update

func TestFrameDefault(t *testing.T) {
	frame := newTestModel(t).View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameTabSelected(t *testing.T) {
	frame := press(newTestModel(t), "3").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameBadges(t *testing.T) {
	m := newTestModel(t)
	// A count on Changes, and activity on two tabs the user is not viewing.
	m.tabs[TabChanges] = PlaceholderTab{title: "Changes", note: "x", badge: Badge{Count: 12}}
	m = apply(m, ActivityMsg{Tab: TabSession}, ActivityMsg{Tab: TabOOB})

	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameStatusBar(t *testing.T) {
	m := apply(newTestModel(t), refreshedMsg{
		Snap:        monitor.Snapshot{WatchedPaths: []string{"package.json", ".env"}},
		Activity:    monitor.Activity{At: testNow.Add(-42 * time.Second), Text: "internal/ui/app.go"},
		HasActivity: true,
	})

	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameHelpOverlay(t *testing.T) {
	frame := press(newTestModel(t), "?").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameTickerOverlay(t *testing.T) {
	frame := press(newTestModel(t), "a").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameClipboardModal(t *testing.T) {
	frame := press(newTestModel(t), "c").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

// A terminal too narrow for the status bar's two segments must still produce a
// well-formed frame.
func TestFrameNarrow(t *testing.T) {
	m := New(gitRepoForTest(), nil, nil, nil).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: 32, Height: 12}, refreshedMsg{
		Activity:    monitor.Activity{At: testNow.Add(-5 * time.Second), Text: "internal/ui/app.go"},
		HasActivity: true,
	})

	frame := m.View()
	requireFrameSize(t, frame, 32, 12)
	golden.RequireEqual(t, []byte(frame))
}

// A terminal so short that no pane fits still renders both bars.
func TestFrameTiny(t *testing.T) {
	m := New(gitRepoForTest(), nil, nil, nil).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: 20, Height: 2})

	frame := m.View()
	requireFrameSize(t, frame, 20, 2)
	golden.RequireEqual(t, []byte(frame))
}

// Before the first WindowSizeMsg there is no size to lay out against, so the
// model renders nothing rather than guessing.
func TestFrameBeforeSize(t *testing.T) {
	if got := New(gitRepoForTest(), nil, nil, nil).View(); got != "" {
		t.Fatalf("view before window size = %q, want empty", got)
	}
}
