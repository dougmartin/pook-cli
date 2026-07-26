package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tab indices, also the digits that select them.
//
// oob is last so that every other index is fixed whether or not it is shown.
// TabOOB is only a valid index when the oob tab exists; routeTo bounds-checks,
// and the shell asks hasOOB before addressing it.
const (
	TabChanges = iota
	TabBranch
	TabSession
	TabPrompts
	TabOOB
)

// Badge is the marker a tab carries in the tab bar. Count is a persistent
// number (the Changes tab's change count). Dot is the transient activity
// indicator raised when data arrives for a tab that is not focused; it is
// cleared by Focus, which the shell calls on the visible tab every update.
type Badge struct {
	Count int
	Dot   bool
}

// Tab is one pane. Tabs never reflow against each other: each owns its cursor
// and scroll offset, and is handed the pane size, so the layout math is a
// single subtraction in the shell.
type Tab interface {
	// Title is the tab bar label.
	Title() string
	// Badge is the marker drawn next to the title.
	Badge() Badge
	// Bindings are the tab's own keys, listed in the help overlay.
	Bindings() []Binding
	// CapturingInput reports that the tab has a focused text field. While it
	// does, keys reach the tab ahead of the global keymap, or typing "a" in a
	// filter would open the activity ticker.
	CapturingInput() bool
	// Focus is called whenever this tab is the visible one. It clears the
	// activity dot, since the user is now looking at the data.
	Focus() Tab
	// Update handles a message routed to this tab.
	Update(tea.Msg) (Tab, tea.Cmd)
	// View renders exactly width by height cells of content.
	View(width, height int) string
}

// PlaceholderTab stands in for a pane that a later phase implements. It
// carries a real badge so the tab bar, activity dots and focus-clearing are
// exercisable before any backend exists.
type PlaceholderTab struct {
	title string
	note  string
	badge Badge

	// counts makes the badge track the number of changed files. Phase 4's
	// Changes tab takes this over.
	counts bool
}

// NewPlaceholderTab returns a tab that renders note in the middle of its pane.
func NewPlaceholderTab(title, note string) PlaceholderTab {
	return PlaceholderTab{title: title, note: note}
}

func (t PlaceholderTab) Title() string        { return t.title }
func (t PlaceholderTab) Badge() Badge         { return t.badge }
func (t PlaceholderTab) Bindings() []Binding  { return nil }
func (t PlaceholderTab) CapturingInput() bool { return false }

func (t PlaceholderTab) Focus() Tab {
	t.badge.Dot = false
	return t
}

// countingFiles makes this tab badge the number of changed files.
func (t PlaceholderTab) countingFiles() PlaceholderTab {
	t.counts = true
	return t
}

func (t PlaceholderTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case ActivityMsg:
		t.badge.Dot = true
	case RefreshMsg:
		if t.counts {
			t.badge.Count = len(msg.Snap.Files)
		}
	}
	return t, nil
}

func (t PlaceholderTab) View(width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		styleDim.Render(t.note))
}
