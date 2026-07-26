// Package ui is the pook shell: the tab bar, the status bar, and the update
// loop that routes every key to exactly one place.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/monitor"
	"github.com/dougmartin/pook-cli/internal/oob"
	"github.com/dougmartin/pook-cli/internal/prompts"
	"github.com/dougmartin/pook-cli/internal/watch"
)

// ActivityMsg raises the activity dot on one tab.
type ActivityMsg struct {
	Tab int
}

// RefreshMsg hands every tab the latest snapshot.
type RefreshMsg struct {
	Snap monitor.Snapshot
}

// tickMsg drives the status bar clock, so "last change Ns ago" counts up on
// its own without any file activity, and the idle check gets a heartbeat.
type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model is the root. Exactly one pane is visible at a time, which is what
// keeps the layout math to a single subtraction and the update loop flat.
type Model struct {
	repo    git.Repo
	mon     *monitor.Monitor
	watcher *watch.Watcher

	width, height int
	ready         bool

	tabs   []Tab
	active int

	// modal takes keys ahead of overlay, and overlay ahead of everything
	// else. Both are nil when the active tab has the keyboard.
	modal   layer
	overlay layer

	// The last refresh, and the derived chrome state.
	snap     monitor.Snapshot
	events   []monitor.Event
	activity monitor.Activity
	hasAct   bool
	alert    string
	banner   string

	// One refresh runs at a time; activity during it queues exactly one more.
	refreshing    bool
	refreshQueued bool

	// now is injectable so golden tests render a fixed heartbeat.
	now func() time.Time
}

// New builds the shell for a repo. mon and w may be nil, which is what the
// view tests use: the shell then renders whatever it is sent and watches
// nothing.
func New(repo git.Repo, mon *monitor.Monitor, w *watch.Watcher, store *prompts.Store) Model {
	return Model{
		repo:    repo,
		mon:     mon,
		watcher: w,
		now:     time.Now,
		tabs:    tabsFor(repo, store),
	}
}

// tabsFor builds the tab set. The oob tab is only included when there is an
// oob home to read, so a machine that does not use oob never sees the tab or
// its keys in help.
func tabsFor(repo git.Repo, store *prompts.Store) []Tab {
	tabs := []Tab{
		NewChangesTab(repo),
		NewBranchTab(repo),
		NewGraphTab(repo),
		NewSessionTab(repo.Root),
		NewPromptsTab(store),
	}
	if oob.Available() {
		tabs = append(tabs, NewOOBTab())
	}
	return tabs
}

// hasOOB reports whether the oob tab is part of this run.
func (m Model) hasOOB() bool { return TabOOB < len(m.tabs) }

// WithClock replaces the clock used for the heartbeat.
func (m Model) WithClock(now func() time.Time) Model {
	m.now = now
	return m
}

func (m Model) Init() tea.Cmd {
	// The prompt library is read once at startup; the watcher keeps it
	// current after that.
	if tab, ok := m.tabs[TabPrompts].(*PromptsTab); ok {
		tab.Load()
	}
	return tea.Batch(tick(), refreshCmd(m.mon), waitForWatch(m.watcher))
}

// paneHeight is the layout calculation: the terminal minus the chrome.
func (m Model) paneHeight() int {
	return max(0, m.height-tabBarHeight-statusBarHeight-m.bannerHeight())
}

// bannerHeight is one row while the idle warning is up, zero otherwise.
func (m Model) bannerHeight() int {
	if m.banner == "" {
		return 0
	}
	return 1
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		// Tabs are told the pane size, never the terminal size, so no tab
		// has to know that the chrome exists.
		mm, cmd := m.broadcast(tea.WindowSizeMsg{Width: m.width, Height: m.paneHeight()})
		return mm, cmd

	case tickMsg:
		return m, tea.Batch(tick(), idleCmd(m.mon))

	case watchMsg:
		// Keep listening, and fold this into a refresh.
		mm, cmd := m.scheduleRefresh()
		return mm, tea.Batch(cmd, waitForWatch(m.watcher))

	case refreshedMsg:
		mm, cmd := m.applyRefresh(msg)
		return mm, cmd

	case idleMsg:
		m.banner = msg.Text
		return m, nil

	case needsRefreshMsg:
		mm, cmd := m.scheduleRefresh()
		return mm, cmd

	case openClipboardMsg:
		m.modal = newClipboardModal(msg)
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

// scheduleRefresh starts a refresh, or notes that one more is wanted when the
// running one finishes. Without the queue an agent writing continuously would
// start a refresh per batch.
func (m Model) scheduleRefresh() (Model, tea.Cmd) {
	if m.refreshing {
		m.refreshQueued = true
		return m, nil
	}
	if m.mon == nil {
		return m, nil
	}
	m.refreshing = true
	return m, refreshCmd(m.mon)
}

// applyRefresh folds a completed refresh into the view.
func (m Model) applyRefresh(msg refreshedMsg) (Model, tea.Cmd) {
	m.refreshing = false

	// A failed refresh keeps the previous snapshot on screen: git failing
	// mid-write should not blank the pane.
	if msg.Snap.Err == nil {
		m.snap = msg.Snap
	}
	m.events = msg.Events
	m.activity, m.hasAct = msg.Activity, msg.HasActivity
	m.alert = watchedAlert(msg.Snap.WatchedPaths)

	// Fresh activity retires the idle banner.
	if msg.Snap.FilesChanged || msg.Snap.BranchChanged || msg.Snap.OOBChanged {
		m.banner = ""
	}

	cmds := []tea.Cmd{}

	// Newly watched namespace directories, and a transcript folder that only
	// appeared after startup, are picked up here.
	if m.watcher != nil {
		for _, g := range msg.Snap.OOB {
			m.watcher.AddDir(g.Dir)
		}
	}

	mm, cmd := m.broadcast(RefreshMsg{Snap: m.snap})
	m = mm
	cmds = append(cmds, cmd)

	// Tabs the user is not looking at get a dot.
	dots := map[int]bool{
		TabChanges: msg.Snap.FilesChanged,
		TabBranch:  msg.Snap.BranchChanged,
		TabGraph:   msg.Snap.BranchChanged,
	}
	if m.hasOOB() {
		dots[TabOOB] = msg.Snap.OOBChanged
	}
	for tab, changed := range dots {
		if !changed || tab == m.active {
			continue
		}
		next, c := m.routeTo(tab, ActivityMsg{Tab: tab})
		m = next
		cmds = append(cmds, c)
	}

	if m.refreshQueued {
		m.refreshQueued = false
		next, c := m.scheduleRefresh()
		m = next
		cmds = append(cmds, c)
	}

	return m, tea.Batch(cmds...)
}

// watchedAlert is the status bar warning for changed files matching a watched
// glob.
func watchedAlert(paths []string) string {
	switch len(paths) {
	case 0:
		return ""
	case 1:
		return "watched: " + paths[0]
	default:
		return fmt.Sprintf("watched: %s +%d more", paths[0], len(paths)-1)
	}
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

	// A tab with a focused text field takes keys ahead of the global keymap,
	// so a filter can contain any character.
	if m.tabs[m.active].CapturingInput() {
		mm, cmd := m.routeTo(m.active, k)
		return mm, cmd
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
		m.overlay = newTickerOverlay(m.events, m.now())
		return true, m, nil

	case keyClipboard.Matches(k):
		return true, m, readClipboardCmd()

	case keyRefresh.Matches(k):
		mm, cmd := m.scheduleRefresh()
		return true, mm, cmd

	case keyClose.Matches(k):
		// With no layer open, esc dismisses the idle banner.
		if m.banner != "" {
			m.banner = ""
			return true, m, nil
		}
		return false, m, nil

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

	// A terminal short enough to leave no pane still gets its chrome, and one
	// shorter than the chrome itself gets as many rows as fit.
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
	if m.banner != "" {
		rows = append(rows, m.bannerRow())
	}
	rows = append(rows, m.statusBar())

	if len(rows) > m.height {
		rows = rows[:m.height]
	}
	return strings.Join(rows, "\n")
}
