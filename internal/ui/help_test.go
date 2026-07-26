package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The help overlay is the only way to discover the keymap, so nothing in it
// may be quietly clipped. This fails the day the keymap outgrows the overlay,
// which is the point at which it needs scrolling rather than more columns.
func TestHelpShowsEveryBinding(t *testing.T) {
	m := newTestModel(t)

	// With every tab contributing a keymap the list no longer fits at 80x24,
	// so what has to hold is that nothing is unreachable: scrolling to the
	// bottom must expose every binding.
	frame := scrollThroughHelp(t, m)

	// Every binding the shell dispatches, and every binding each tab
	// contributes, has to be readable at the standard terminal size.
	sections := [][]Binding{globalBindings}
	for _, tab := range m.tabs {
		if b := tab.Bindings(); len(b) > 0 {
			sections = append(sections, b)
		}
	}

	for _, bindings := range sections {
		for _, b := range bindings {
			if !strings.Contains(frame, b.Display()) {
				t.Errorf("help is missing the key %q\n%s", b.Display(), frame)
			}
			if !strings.Contains(frame, b.Help) {
				t.Errorf("help is missing the description %q\n%s", b.Help, frame)
			}
		}
	}
	if !strings.Contains(frame, "esc closes") {
		t.Errorf("help does not say how to close it\n%s", frame)
	}
}

// Once tabs carry their own keys the list spills into columns.
func TestHelpColumnizesTabBindings(t *testing.T) {
	m := New(gitRepoForTest(), nil, nil).WithClock(fixedClock)
	for i, t := range m.tabs {
		m.tabs[i] = bindingTab{title: t.Title(), bindings: fakeBindings(t.Title(), 8)}
	}
	m = apply(m, tea.WindowSizeMsg{Width: 120, Height: 30})

	frame := press(m, "?").View()
	requireFrameSize(t, frame, 120, 30)

	for _, tab := range m.tabs {
		for _, b := range tab.Bindings() {
			if !strings.Contains(frame, b.Help) {
				t.Fatalf("help is missing %q from the %s section\n%s", b.Help, tab.Title(), frame)
			}
		}
	}
}

// Tabs with no keys of their own are left out rather than listed as empty.
func TestHelpOmitsEmptyTabSections(t *testing.T) {
	frame := press(newTestModel(t), "?").View()
	for _, tab := range newTestModel(t).tabs {
		if strings.Contains(frame, tab.Title()+"\n") {
			t.Errorf("help lists an empty section for %s\n%s", tab.Title(), frame)
		}
	}
}

func fakeBindings(prefix string, n int) []Binding {
	out := make([]Binding, n)
	for i := range out {
		out[i] = Binding{
			Keys: []string{fmt.Sprintf("%c", 'a'+i)},
			Help: fmt.Sprintf("%s action %d", strings.ToLower(prefix), i),
		}
	}
	return out
}

// bindingTab is a tab that exists only to contribute keys to the help overlay.
type bindingTab struct {
	title    string
	bindings []Binding
}

func (b bindingTab) Title() string                 { return b.title }
func (b bindingTab) Badge() Badge                  { return Badge{} }
func (b bindingTab) Bindings() []Binding           { return b.bindings }
func (b bindingTab) CapturingInput() bool          { return false }
func (b bindingTab) Focus() Tab                    { return b }
func (b bindingTab) Update(tea.Msg) (Tab, tea.Cmd) { return b, nil }
func (b bindingTab) View(width, height int) string { return "" }

// scrollThroughHelp opens the overlay and pages to the bottom, returning every
// frame it passed through joined together.
func scrollThroughHelp(t *testing.T, m Model) string {
	t.Helper()
	m = press(m, "?")

	var seen []string
	last := ""
	for range 40 {
		frame := m.View()
		requireFrameSize(t, frame, testWidth, testHeight)
		if frame == last {
			break // the overlay has stopped moving
		}
		seen = append(seen, frame)
		last = frame
		m = press(m, "ctrl+d")
	}
	return strings.Join(seen, "\n")
}
