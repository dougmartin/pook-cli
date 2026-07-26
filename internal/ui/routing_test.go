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
	m := newTestModel(t)

	m = press(m, "shift+tab")
	if m.active != TabPrompts {
		t.Fatalf("shift-tab from first tab = %d, want %d", m.active, TabPrompts)
	}

	m = press(m, "tab")
	if m.active != TabChanges {
		t.Fatalf("tab from last tab = %d, want %d", m.active, TabChanges)
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
	m := press(newTestModel(t), "c")
	if m.modal == nil {
		t.Fatal("c did not open the clipboard modal")
	}

	m = press(m, "3")
	if m.active != TabChanges {
		t.Fatalf("tab switched to %d while a modal was open", m.active)
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
	m := press(newTestModel(t), "c")
	_, cmd := applyCmd(m, key("ctrl+c"))
	if !isQuit(cmd) {
		t.Fatal("ctrl+c did not quit with a modal open")
	}
}

// A modal opened over an overlay is still what receives keys.
func TestModalOutranksOverlay(t *testing.T) {
	m := press(newTestModel(t), "?")
	m.modal = clipboardModal{}

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

func TestRefreshReachesEveryTab(t *testing.T) {
	counts := make([]int, 5)
	m := newTestModel(t)
	for i := range m.tabs {
		m.tabs[i] = refreshProbe{count: &counts[i]}
	}

	m = press(m, "r")
	for i, n := range counts {
		if n != 1 {
			t.Fatalf("tab %d saw %d refreshes, want 1", i, n)
		}
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

func (p sizeProbe) Title() string       { return "Probe" }
func (p sizeProbe) Badge() Badge        { return Badge{} }
func (p sizeProbe) Bindings() []Binding { return nil }
func (p sizeProbe) Focus() Tab          { return p }
func (p sizeProbe) Update(msg tea.Msg) (Tab, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		*p.seen = size
	}
	return p, nil
}
func (p sizeProbe) View(width, height int) string { return "" }

type refreshProbe struct{ count *int }

func (p refreshProbe) Title() string       { return "Probe" }
func (p refreshProbe) Badge() Badge        { return Badge{} }
func (p refreshProbe) Bindings() []Binding { return nil }
func (p refreshProbe) Focus() Tab          { return p }
func (p refreshProbe) Update(msg tea.Msg) (Tab, tea.Cmd) {
	if _, ok := msg.(RefreshMsg); ok {
		*p.count++
	}
	return p, nil
}
func (p refreshProbe) View(width, height int) string { return "" }

type keyProbe struct{ seen *[]string }

func (p keyProbe) Title() string       { return "Probe" }
func (p keyProbe) Badge() Badge        { return Badge{} }
func (p keyProbe) Bindings() []Binding { return nil }
func (p keyProbe) Focus() Tab          { return p }
func (p keyProbe) Update(msg tea.Msg) (Tab, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		*p.seen = append(*p.seen, k.String())
	}
	return p, nil
}
func (p keyProbe) View(width, height int) string { return "" }
