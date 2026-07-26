package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/monitor"
	"github.com/dougmartin/pook-cli/internal/watch"
)

// refreshedMsg carries everything one refresh produced. The view keeps its own
// copy rather than reading back through the monitor, so rendering never
// depends on what a background refresh is doing.
type refreshedMsg struct {
	Snap   monitor.Snapshot
	Events []monitor.Event

	Activity    monitor.Activity
	HasActivity bool
}

// needsRefreshMsg asks the shell for a refresh after something changed the
// repo from inside pook: a discarded file, or a file edited in $EDITOR.
type needsRefreshMsg struct{}

// idleMsg raises the "agent went quiet" banner.
type idleMsg struct{ Text string }

// watchMsg reports that the filesystem moved. The batch itself is not used to
// build events: the refresh it triggers compares repo state, which is what
// makes an event mean the content actually changed.
type watchMsg struct{ Count int }

// refreshCmd re-reads the repo off the UI goroutine.
func refreshCmd(mon *monitor.Monitor) tea.Cmd {
	if mon == nil {
		return nil
	}
	return func() tea.Msg {
		snap := mon.Refresh()
		activity, ok := mon.LastActivity()
		return refreshedMsg{
			Snap:        snap,
			Events:      mon.Events(),
			Activity:    activity,
			HasActivity: ok,
		}
	}
}

// idleCmd checks whether the agent has gone quiet for long enough to warrant
// the banner.
func idleCmd(mon *monitor.Monitor) tea.Cmd {
	if mon == nil {
		return nil
	}
	return func() tea.Msg {
		text, fired := mon.CheckIdle()
		if !fired {
			return nil
		}
		return idleMsg{Text: text}
	}
}

// waitForWatch blocks on the watcher until a batch of activity settles.
func waitForWatch(w *watch.Watcher) tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg {
		batch, ok := <-w.Batches()
		if !ok {
			return nil
		}
		return watchMsg{Count: len(batch)}
	}
}
