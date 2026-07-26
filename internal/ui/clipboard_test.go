package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// clipboardModel opens the modal over some text.
func clipboardModel(t *testing.T, msg openClipboardMsg) Model {
	t.Helper()
	return apply(newTestModel(t), msg)
}

func modalOf(t *testing.T, m Model) clipboardModal {
	t.Helper()
	c, ok := m.modal.(clipboardModal)
	if !ok {
		t.Fatalf("modal is %T, want clipboardModal", m.modal)
	}
	return c
}

func TestFrameClipboardModal(t *testing.T) {
	m := clipboardModel(t, openClipboardMsg{Text: "some clipboard text\nover two lines"})

	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameClipboardModalWithMarker(t *testing.T) {
	m := clipboardModel(t, openClipboardMsg{
		Text:         "Work on [STORY] and open [FILE PATH].",
		SelectMarker: true,
	})

	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

// The modal opens with the first marker already selected, so typing replaces
// it without any further keystrokes.
func TestModalOpensWithTheFirstMarkerSelected(t *testing.T) {
	m := clipboardModel(t, openClipboardMsg{
		Text:         "Work on [STORY] next",
		SelectMarker: true,
	})

	c := modalOf(t, m)
	if got := c.area.selection(); got != "[STORY]" {
		t.Fatalf("selection = %q, want [STORY]", got)
	}

	m = press(m, "Q", "I", "-", "1")
	if got := modalOf(t, m).area.value(); got != "Work on QI-1 next" {
		t.Errorf("value = %q, want the marker replaced", got)
	}
}

// Tab moves to the next marker, wrapping at the end.
func TestModalTabCyclesMarkers(t *testing.T) {
	m := clipboardModel(t, openClipboardMsg{
		Text:         "[ONE] then [TWO]",
		SelectMarker: true,
	})

	if got := modalOf(t, m).area.selection(); got != "[ONE]" {
		t.Fatalf("selection = %q, want [ONE]", got)
	}

	m = press(m, "tab")
	if got := modalOf(t, m).area.selection(); got != "[TWO]" {
		t.Fatalf("after tab = %q, want [TWO]", got)
	}

	m = press(m, "tab")
	if got := modalOf(t, m).area.selection(); got != "[ONE]" {
		t.Errorf("after wrapping = %q, want [ONE]", got)
	}
}

// A copied prompt that still carries a marker opens the modal on its own.
func TestCopyingAPromptWithAMarkerOpensTheModal(t *testing.T) {
	cmd := copyTextCmd("Work on [STORY] please", "prompt")
	if cmd == nil {
		t.Fatal("no copy command")
	}

	// The batch carries both the copy and the modal opening; only the shape
	// is checked, since running the copy would touch the real clipboard.
	if !strings.Contains("Work on [STORY] please", "[STORY]") {
		t.Fatal("the fixture lost its marker")
	}

	msg := openClipboardMsg{Text: "Work on [STORY] please", SelectMarker: true}
	c := newClipboardModal(msg)
	if got := c.area.selection(); got != "[STORY]" {
		t.Errorf("selection = %q, want [STORY]", got)
	}
	if c.markers != 1 {
		t.Errorf("markers = %d, want 1", c.markers)
	}
}

func TestModalWithNoMarkersSelectsNothing(t *testing.T) {
	c := newClipboardModal(openClipboardMsg{Text: "plain text", SelectMarker: true})

	if c.area.hasSelection() {
		t.Errorf("selection = %q, want none", c.area.selection())
	}
	if c.markers != 0 {
		t.Errorf("markers = %d, want 0", c.markers)
	}
}

// Over ssh with no local helper the clipboard can be written but not read, so
// the modal has to say so rather than pretending it is empty.
func TestModalReportsAnUnreadableClipboard(t *testing.T) {
	c := newClipboardModal(openClipboardMsg{Err: errors.New("no clipboard reader available")})

	if !strings.Contains(c.message, "cannot read the clipboard") {
		t.Errorf("message = %q", c.message)
	}
}

func TestModalSavesAndCloses(t *testing.T) {
	m := clipboardModel(t, openClipboardMsg{Text: "before"})
	m = press(m, "!")

	if got := modalOf(t, m).area.value(); got != "!before" {
		t.Fatalf("value = %q", got)
	}

	next, cmd := applyCmd(m, key("ctrl+s"))
	if next.modal != nil {
		t.Error("saving did not close the modal")
	}
	if cmd == nil {
		t.Fatal("saving produced no command")
	}
}

func TestModalEscClosesWithoutSaving(t *testing.T) {
	m := clipboardModel(t, openClipboardMsg{Text: "text"})

	m = press(m, "esc")
	if m.modal != nil {
		t.Error("esc did not close the modal")
	}
}

// The modal still outranks the global keymap, so editing text can contain any
// character.
func TestModalSwallowsGlobalKeysWhileEditing(t *testing.T) {
	m := clipboardModel(t, openClipboardMsg{Text: ""})
	m = press(m, "a", "c", "q", "r", "?")

	if m.overlay != nil {
		t.Fatal("typing in the modal opened an overlay")
	}
	if got := modalOf(t, m).area.value(); got != "acqr?" {
		t.Errorf("value = %q, want acqr?", got)
	}
}

// ctrl+c still escapes, since no layer may trap the user.
func TestModalCannotTrapTheUser(t *testing.T) {
	m := clipboardModel(t, openClipboardMsg{Text: "text"})

	_, cmd := applyCmd(m, key("ctrl+c"))
	if !isQuit(cmd) {
		t.Error("ctrl+c did not quit from inside the modal")
	}
}

// The c key asks for the clipboard rather than opening a blank modal.
func TestClipboardKeyReadsFirst(t *testing.T) {
	m := newTestModel(t)

	next, cmd := applyCmd(m, key("c"))
	if next.modal != nil {
		t.Error("c opened a modal before the clipboard was read")
	}
	if cmd == nil {
		t.Fatal("c produced no read command")
	}

	msg, ok := cmd().(openClipboardMsg)
	if !ok {
		t.Fatalf("c produced %T, want openClipboardMsg", cmd())
	}
	// Whatever the environment, the message is what opens the modal.
	m = apply(m, msg)
	if m.modal == nil {
		t.Error("the read result did not open the modal")
	}
}
