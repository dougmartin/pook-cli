// Package ui is the pook shell: the tab bar, the status bar, and the update
// loop that routes every key to exactly one place.
package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/git"
)

// Messages the shell understands. Phase 3's watcher is what emits the first
// three; they are defined here because the shell owns the chrome they drive.
type (
	// HeartbeatMsg updates the status bar's "last change Ns ago" line.
	HeartbeatMsg struct {
		Path string
		At   time.Time
	}
	// AlertMsg sets the watched-path alert. An empty Text clears it.
	AlertMsg struct {
		Text string
	}
	// ActivityMsg raises the activity dot on one tab.
	ActivityMsg struct {
		Tab int
	}
	// RefreshMsg asks every tab to reload from disk.
	RefreshMsg struct{}
)

// tickMsg drives the status bar clock, so "last change Ns ago" counts up on
// its own without any file activity.
type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// heartbeat is the last observed change to anything pook watches.
type heartbeat struct {
	seen bool
	at   time.Time
	path string
}

// Model is the root. Exactly one pane is visible at a time, which is what
// keeps the layout math to a single subtraction and the update loop flat.
type Model struct {
	repo git.Repo

	width, height int
	ready         bool

	tabs   []Tab
	active int

	// modal takes keys ahead of overlay, and overlay ahead of everything
	// else. Both are nil when the active tab has the keyboard.
	modal   layer
	overlay layer

	hb    heartbeat
	alert string

	// now is injectable so golden tests render a fixed heartbeat.
	now func() time.Time
}

// New builds the shell for a repo. The panes are placeholders until their
// phases land.
func New(repo git.Repo) Model {
	return Model{
		repo: repo,
		now:  time.Now,
		tabs: []Tab{
			NewPlaceholderTab("Changes", "uncommitted changes, phase 4"),
			NewPlaceholderTab("Branch", "commits on this branch, phase 5"),
			NewPlaceholderTab("Session", "live Claude Code session, phase 5"),
			NewPlaceholderTab("oob", "out-of-band files, phase 5"),
			NewPlaceholderTab("Prompts", "prompt library, phase 6"),
		},
	}
}

// WithClock replaces the clock used for the heartbeat.
func (m Model) WithClock(now func() time.Time) Model {
	m.now = now
	return m
}

func (m Model) Init() tea.Cmd { return tick() }

// paneHeight is the whole layout calculation: the terminal minus the two bars.
func (m Model) paneHeight() int {
	return max(0, m.height-tabBarHeight-statusBarHeight)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		// Tabs are told the pane size, never the terminal size, so no tab
		// has to know that the bars exist.
		mm, cmd := m.broadcast(tea.WindowSizeMsg{Width: m.width, Height: m.paneHeight()})
		return mm, cmd

	case tickMsg:
		return m, tick()

	case HeartbeatMsg:
		m.hb = heartbeat{seen: true, at: msg.At, path: msg.Path}
		return m, nil

	case AlertMsg:
		m.alert = msg.Text
		return m, nil

	case ActivityMsg:
		mm, cmd := m.routeTo(msg.Tab, msg)
		return mm, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	mm, cmd := m.broadcast(msg)
	return mm, cmd
}

// handleKey is the routing rule the whole design rests on: modal, then
// overlay, then global keys, then the active tab.
func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ctrl+c is checked first so no layer can ever trap the user.
	if keyForceQuit.Matches(k) {
		return m, tea.Quit
	}

	if m.modal != nil {
		l, cmd := m.modal.Update(k)
		m.modal = l
		return m, cmd
	}

	if m.overlay != nil {
		l, cmd := m.overlay.Update(k)
		m.overlay = l
		return m, cmd
	}

	if handled, mm, cmd := m.handleGlobalKey(k); handled {
		return mm, cmd
	}

	mm, cmd := m.routeTo(m.active, k)
	return mm, cmd
}

func (m Model) handleGlobalKey(k tea.KeyMsg) (bool, Model, tea.Cmd) {
	switch {
	case keyQuit.Matches(k):
		return true, m, tea.Quit

	case keyHelp.Matches(k):
		m.overlay = newHelpOverlay(m)
		return true, m, nil

	case keyTicker.Matches(k):
		m.overlay = tickerOverlay{}
		return true, m, nil

	case keyClipboard.Matches(k):
		m.modal = clipboardModal{}
		return true, m, nil

	case keyRefresh.Matches(k):
		mm, cmd := m.broadcast(RefreshMsg{})
		return true, mm, cmd

	case keyNextTab.Matches(k):
		return true, m.selectTab(m.active + 1), nil

	case keyPrevTab.Matches(k):
		return true, m.selectTab(m.active - 1), nil
	}

	if i, ok := tabDigit(k); ok && i < len(m.tabs) {
		return true, m.selectTab(i), nil
	}
	return false, m, nil
}

// selectTab moves the focus, wrapping at both ends.
func (m Model) selectTab(i int) Model {
	n := len(m.tabs)
	m.active = ((i % n) + n) % n
	return m.focusActive()
}

// focusActive clears the visible tab's activity dot: the user is looking at
// it, so there is nothing new to point at.
func (m Model) focusActive() Model {
	m.tabs[m.active] = m.tabs[m.active].Focus()
	return m
}

// routeTo sends a message to one tab.
func (m Model) routeTo(i int, msg tea.Msg) (Model, tea.Cmd) {
	if i < 0 || i >= len(m.tabs) {
		return m, nil
	}
	t, cmd := m.tabs[i].Update(msg)
	m.tabs[i] = t
	return m.focusActive(), cmd
}

// broadcast sends a message to every tab.
func (m Model) broadcast(msg tea.Msg) (Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, len(m.tabs))
	for i, t := range m.tabs {
		next, cmd := t.Update(msg)
		m.tabs[i] = next
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m.focusActive(), tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready || m.height <= 0 || m.width <= 0 {
		return ""
	}

	rows := []string{m.tabBar()}

	// A terminal short enough to leave no pane still gets both bars, and one
	// shorter than the bars themselves gets as many as fit.
	if h := m.paneHeight(); h > 0 {
		var body string
		switch {
		case m.modal != nil:
			body = m.modal.View(m.width, h)
		case m.overlay != nil:
			body = m.overlay.View(m.width, h)
		default:
			body = m.tabs[m.active].View(m.width, h)
		}
		rows = append(rows, fitBlock(body, m.width, h))
	}
	rows = append(rows, m.statusBar())

	if len(rows) > m.height {
		rows = rows[:m.height]
	}
	return strings.Join(rows, "\n")
}
