package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tab indices, also the digits that select them.
const (
	TabChanges = iota
	TabBranch
	TabSession
	TabOOB
	TabPrompts
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
}

// NewPlaceholderTab returns a tab that renders note in the middle of its pane.
func NewPlaceholderTab(title, note string) PlaceholderTab {
	return PlaceholderTab{title: title, note: note}
}

func (t PlaceholderTab) Title() string       { return t.title }
func (t PlaceholderTab) Badge() Badge        { return t.badge }
func (t PlaceholderTab) Bindings() []Binding { return nil }

func (t PlaceholderTab) Focus() Tab {
	t.badge.Dot = false
	return t
}

func (t PlaceholderTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	if _, ok := msg.(ActivityMsg); ok {
		t.badge.Dot = true
	}
	return t, nil
}

func (t PlaceholderTab) View(width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		styleDim.Render(t.note))
}
