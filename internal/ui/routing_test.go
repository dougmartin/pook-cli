package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDigitSelectsTab(t *testing.T) {
	m := newTestModel(t)
	for i, digit := range []string{"1", "2", "3", "4", "5"} {
		m = press(m, digit)
		if m.active != i {
			t.Fatalf("after %q active = %d, want %d", digit, m.active, i)
		}
	}
}

func TestDigitBeyondTabsIsIgnored(t *testing.T) {
	m := press(newTestModel(t), "3", "9")
	if m.active != TabSession {
		t.Fatalf("active = %d, want %d", m.active, TabSession)
	}
}

func TestTabCyclingWraps(t *testing.T) {
	// The arrows cycle tabs alongside tab and shift-tab.
	for _, keys := range []struct{ prev, next string }{
		{prev: "shift+tab", next: "tab"},
		{prev: "left", next: "right"},
	} {
		t.Run(keys.next, func(t *testing.T) {
			m := newTestModel(t)

			m = press(m, keys.prev)
			if m.active != TabOOB {
				t.Fatalf("%s from the first tab = %d, want %d", keys.prev, m.active, TabOOB)
			}

			m = press(m, keys.next)
			if m.active != TabChanges {
				t.Fatalf("%s from the last tab = %d, want %d", keys.next, m.active, TabChanges)
			}

			m = press(m, keys.next)
			if m.active != TabBranch {
				t.Fatalf("%s = %d, want %d", keys.next, m.active, TabBranch)
			}
		})
	}
}

// The arrows belong to the shell, so the Session tab navigates with h and l.
func TestArrowsCycleTabsFromTheSessionTab(t *testing.T) {
	m := press(newTestModel(t), "3")

	m = press(m, "right")
	if m.active != TabPrompts {
		t.Errorf("right from Session = %d, want %d", m.active, TabPrompts)
	}

	m = press(m, "left")
	if m.active != TabSession {
		t.Errorf("left = %d, want %d", m.active, TabSession)
	}
}

// A focused text field still outranks them, so a filter can be edited with the
// arrow keys.
func TestArrowsReachAFocusedInput(t *testing.T) {
	m := press(changesModel(t), "/")
	m = press(m, "a", "b")

	m = press(m, "left")
	if m.active != TabChanges {
		t.Fatal("left switched tabs while the filter had focus")
	}

	// The caret moved, so typing lands before the last character.
	m = press(m, "X")
	if got := changesTab(m).pathQuery; got != "aXb" {
		t.Errorf("filter = %q, want aXb", got)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, name := range []string{"q", "ctrl+c"} {
		_, cmd := applyCmd(newTestModel(t), key(name))
		if !isQuit(cmd) {
			t.Fatalf("%q did not quit", name)
		}
	}
}

// The update loop's precedence: a modal takes keys ahead of the global keymap,
// so tab switching is inert while it is open.
func TestModalSwallowsGlobalKeys(t *testing.T) {
	// c reads the clipboard first, so the modal is opened by the result.
	m := apply(newTestModel(t), openClipboardMsg{Text: "text"})
	if m.modal == nil {
		t.Fatal("the clipboard modal did not open")
	}

	m = press(m, "3")
	if m.active != TabChanges {
		t.Fatalf("tab switched to %d while a modal was open", m.active)
	}
	if got := m.modal.(clipboardModal).area.value(); got != "3text" {
		t.Errorf("the modal did not receive the key: %q", got)
	}

	m = press(m, "esc")
	if m.modal != nil {
		t.Fatal("esc did not close the modal")
	}
	m = press(m, "3")
	if m.active != TabSession {
		t.Fatalf("tab switching still inert after the modal closed, active = %d", m.active)
	}
}

func TestOverlaySwallowsGlobalKeys(t *testing.T) {
	m := press(newTestModel(t), "?")
	if m.overlay == nil {
		t.Fatal("? did not open the help overlay")
	}

	m = press(m, "2")
	if m.active != TabChanges {
		t.Fatalf("tab switched to %d while an overlay was open", m.active)
	}

	// ? toggles help back off, the same as esc.
	m = press(m, "?")
	if m.overlay != nil {
		t.Fatal("? did not close the help overlay")
	}
}

// ctrl+c is checked ahead of the modal, so no layer can trap the user.
func TestForceQuitEscapesAModal(t *testing.T) {
	m := apply(newTestModel(t), openClipboardMsg{Text: "text"})
	_, cmd := applyCmd(m, key("ctrl+c"))
	if !isQuit(cmd) {
		t.Fatal("ctrl+c did not quit with a modal open")
	}
}

// A modal opened over an overlay is still what receives keys.
func TestModalOutranksOverlay(t *testing.T) {
	m := press(newTestModel(t), "?")
	m.modal = newClipboardModal(openClipboardMsg{Text: "text"})

	if !strings.Contains(m.View(), "Clipboard") {
		t.Fatal("overlay rendered over an open modal")
	}

	m = press(m, "esc")
	if m.modal != nil {
		t.Fatal("esc closed the wrong layer")
	}
	if m.overlay == nil {
		t.Fatal("esc closed the overlay underneath the modal")
	}
}

// The activity dot marks tabs the user is not looking at, and clears on view.
func TestActivityDotClearsOnFocus(t *testing.T) {
	m := apply(newTestModel(t), ActivityMsg{Tab: TabBranch})
	if !m.tabs[TabBranch].Badge().Dot {
		t.Fatal("activity on an unfocused tab did not raise its dot")
	}

	m = press(m, "2")
	if m.tabs[TabBranch].Badge().Dot {
		t.Fatal("dot survived viewing the tab")
	}
}

func TestActivityOnTheVisibleTabNeverShows(t *testing.T) {
	m := apply(newTestModel(t), ActivityMsg{Tab: TabChanges})
	if m.tabs[TabChanges].Badge().Dot {
		t.Fatal("the visible tab raised a dot for data the user can already see")
	}
}

// Tabs are handed the pane size, never the terminal size.
func TestTabsReceivePaneSize(t *testing.T) {
	var got tea.WindowSizeMsg
	m := newTestModel(t)
	m.tabs[TabChanges] = sizeProbe{seen: &got}
	m = apply(m, tea.WindowSizeMsg{Width: 100, Height: 40})

	if want := (tea.WindowSizeMsg{Width: 100, Height: 40 - tabBarHeight - statusBarHeight}); got != want {
		t.Fatalf("tab saw %+v, want %+v", got, want)
	}
}

// A completed refresh hands every tab the new snapshot.
func TestRefreshReachesEveryTab(t *testing.T) {
	counts := make([]int, 5)
	m := newTestModel(t)
	for i := range m.tabs {
		m.tabs[i] = refreshProbe{count: &counts[i]}
	}

	m = apply(m, refreshedMsg{})
	for i, n := range counts {
		if n != 1 {
			t.Fatalf("tab %d saw %d refreshes, want 1", i, n)
		}
	}
}

// Activity while a refresh is running queues exactly one more, rather than
// starting a refresh per batch while an agent writes continuously.
func TestRefreshesDoNotPileUp(t *testing.T) {
	m := newTestModel(t)
	m.refreshing = true

	for range 5 {
		m, _ = m.scheduleRefresh()
	}
	if !m.refreshQueued {
		t.Fatal("activity during a refresh did not queue another")
	}

	m, _ = m.applyRefresh(refreshedMsg{})
	if m.refreshQueued {
		t.Error("the queued refresh was not consumed")
	}
	if m.refreshing {
		t.Error("a refresh is still marked in flight with no monitor to run it")
	}
}

// Keys the shell does not claim fall through to the active tab, and keys it
// does claim never reach one.
func TestUnclaimedKeysReachTheActiveTab(t *testing.T) {
	var seen []string
	m := press(newTestModel(t), "2")
	m.tabs[TabBranch] = keyProbe{seen: &seen}

	// j and k are the tab's; ? opens help, and while help is open even the
	// tab's own keys belong to the overlay.
	m = press(m, "j", "?", "k")
	m = press(m, "esc", "k")

	if want := []string{"j", "k"}; !slices.Equal(seen, want) {
		t.Fatalf("tab saw %v, want %v", seen, want)
	}
}

// Probes: minimal Tab implementations that record what the shell routes.

type sizeProbe struct{ seen *tea.WindowSizeMsg }

func (p sizeProbe) Title() string        { return "Probe" }
func (p sizeProbe) Badge() Badge         { return Badge{} }
func (p sizeProbe) Bindings() []Binding  { return nil }
func (p sizeProbe) CapturingInput() bool { return false }
func (p sizeProbe) Focus() Tab           { return p }
func (p sizeProbe) Update(msg tea.Msg) (Tab, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		*p.seen = size
	}
	return p, nil
}
func (p sizeProbe) View(width, height int) string { return "" }

type refreshProbe struct{ count *int }

func (p refreshProbe) Title() string        { return "Probe" }
func (p refreshProbe) Badge() Badge         { return Badge{} }
func (p refreshProbe) Bindings() []Binding  { return nil }
func (p refreshProbe) CapturingInput() bool { return false }
func (p refreshProbe) Focus() Tab           { return p }
func (p refreshProbe) Update(msg tea.Msg) (Tab, tea.Cmd) {
	if _, ok := msg.(RefreshMsg); ok {
		*p.count++
	}
	return p, nil
}
func (p refreshProbe) View(width, height int) string { return "" }

type keyProbe struct{ seen *[]string }

func (p keyProbe) Title() string        { return "Probe" }
func (p keyProbe) Badge() Badge         { return Badge{} }
func (p keyProbe) Bindings() []Binding  { return nil }
func (p keyProbe) CapturingInput() bool { return false }
func (p keyProbe) Focus() Tab           { return p }
func (p keyProbe) Update(msg tea.Msg) (Tab, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		*p.seen = append(*p.seen, k.String())
	}
	return p, nil
}
func (p keyProbe) View(width, height int) string { return "" }
