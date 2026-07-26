package ui

import (
	"bytes"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/dougmartin/pook-cli/internal/monitor"
)

// The frame tests drive Update and View directly, which is deterministic but
// bypasses the runtime. This one runs the real bubbletea event loop over the
// real model, so a mistake in Init, in the tick command or in quitting shows
// up as a hang or a missing frame rather than a passing unit test.
func TestProgramDrivesTabsAndQuits(t *testing.T) {
	tm := teatest.NewTestModel(t,
		New(gitRepoForTest(), nil, nil).WithClock(fixedClock),
		teatest.WithInitialTermSize(testWidth, testHeight),
	)

	// The first frame carries the pane and both bars.
	waitForOutput(t, tm, "no changes against HEAD", "watching for changes", "1 Changes")

	tm.Send(key("3"))
	waitForOutput(t, tm, "live Claude Code session")

	tm.Send(key("?"))
	waitForOutput(t, tm, "activity ticker")

	tm.Send(key("esc"))
	tm.Send(key("q"))

	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	final, ok := tm.FinalModel(t).(Model)
	if !ok {
		t.Fatalf("final model is %T, want ui.Model", tm.FinalModel(t))
	}
	if final.active != TabSession {
		t.Fatalf("final active tab = %d, want %d", final.active, TabSession)
	}
	if final.overlay != nil {
		t.Fatal("overlay outlived esc")
	}
}

// The watcher's messages reach the chrome through the runtime, not just
// through a direct Update call.
func TestProgramRendersHeartbeat(t *testing.T) {
	tm := teatest.NewTestModel(t,
		New(gitRepoForTest(), nil, nil).WithClock(fixedClock),
		teatest.WithInitialTermSize(testWidth, testHeight),
	)

	tm.Send(refreshedMsg{
		Snap:        monitor.Snapshot{WatchedPaths: []string{"package.json"}},
		Activity:    monitor.Activity{At: testNow.Add(-9 * time.Second), Text: "internal/ui/app.go"},
		HasActivity: true,
	})
	waitForOutput(t, tm, "last change 9s ago", "watched: package.json")

	tm.Send(key("q"))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// waitForOutput blocks until every want has appeared in the output written
// since the previous call. Bubbletea only rewrites the lines that changed, so
// each call must look for something the newest frames actually redraw.
func waitForOutput(t *testing.T, tm *teatest.TestModel, wants ...string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(),
		func(b []byte) bool {
			for _, want := range wants {
				if !bytes.Contains(b, []byte(want)) {
					return false
				}
			}
			return true
		},
		teatest.WithCheckInterval(10*time.Millisecond),
		teatest.WithDuration(3*time.Second),
	)
}

var _ tea.Model = Model{}
