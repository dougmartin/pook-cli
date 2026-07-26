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
	frame := press(newTestModel(t), "?").View()
	requireFrameSize(t, frame, testWidth, testHeight)

	for _, b := range globalBindings {
		if !strings.Contains(frame, b.Display()) {
			t.Errorf("help is missing the key %q\n%s", b.Display(), frame)
		}
		if !strings.Contains(frame, b.Help) {
			t.Errorf("help is missing the description %q\n%s", b.Help, frame)
		}
	}
	if !strings.Contains(frame, "esc closes") {
		t.Errorf("help does not say how to close it\n%s", frame)
	}
}

// Once tabs carry their own keys the list spills into columns.
func TestHelpColumnizesTabBindings(t *testing.T) {
	m := New(gitRepoForTest()).WithClock(fixedClock)
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
func (b bindingTab) Focus() Tab                    { return b }
func (b bindingTab) Update(tea.Msg) (Tab, tea.Cmd) { return b, nil }
func (b bindingTab) View(width, height int) string { return "" }
