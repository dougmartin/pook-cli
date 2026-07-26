package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// area builds an editor over some text with a given width.
func area(text string, width int) textArea {
	return newTextArea().setValue(text).setSize(width, 10)
}

// typeInto sends each key name through the editor.
func typeInto(a textArea, names ...string) textArea {
	for _, n := range names {
		a, _ = a.handleKey(key(n))
	}
	return a
}

func TestTextAreaInsertAndDelete(t *testing.T) {
	a := area("", 20)

	a = typeInto(a, "a", "b", "c")
	if got := a.value(); got != "abc" {
		t.Fatalf("value = %q, want abc", got)
	}
	if a.cursor != 3 {
		t.Errorf("cursor = %d, want 3", a.cursor)
	}

	a = a.moveLeft()
	a = typeInto(a, "X")
	if got := a.value(); got != "abXc" {
		t.Errorf("value = %q, want abXc", got)
	}

	a = a.backspace()
	if got := a.value(); got != "abc" {
		t.Errorf("value = %q, want abc after backspace", got)
	}

	a = a.deleteForward()
	if got := a.value(); got != "ab" {
		t.Errorf("value = %q, want ab after delete", got)
	}
}

func TestTextAreaEdgeDeletes(t *testing.T) {
	a := area("ab", 20).moveTo(0)

	if got := a.backspace().value(); got != "ab" {
		t.Errorf("backspace at the start changed the text to %q", got)
	}

	a = a.moveTo(2)
	if got := a.deleteForward().value(); got != "ab" {
		t.Errorf("delete at the end changed the text to %q", got)
	}
}

// Typing over a selection replaces it. This is the whole point of the
// selection model.
func TestTypingReplacesTheSelection(t *testing.T) {
	a := area("Work on [STORY] next", 40)

	a, ok := a.selectFirstMarker()
	if !ok {
		t.Fatal("no marker was found")
	}
	if got := a.selection(); got != "[STORY]" {
		t.Fatalf("selection = %q, want [STORY]", got)
	}

	a = typeInto(a, "Q", "I", "-", "1", "4", "0")
	if want := "Work on QI-140 next"; a.value() != want {
		t.Errorf("value = %q, want %q", a.value(), want)
	}
	if a.hasSelection() {
		t.Error("the selection survived being typed over")
	}
	if a.cursor != len("Work on QI-140") {
		t.Errorf("cursor = %d, want it after the replacement", a.cursor)
	}
}

func TestBackspaceDeletesTheWholeSelection(t *testing.T) {
	a := area("Open [FILE PATH] now", 40)
	a, _ = a.selectFirstMarker()

	a = a.backspace()
	if want := "Open  now"; a.value() != want {
		t.Errorf("value = %q, want %q", a.value(), want)
	}
	if a.hasSelection() {
		t.Error("the selection survived the delete")
	}
}

// Tab cycles markers in order and wraps at the end.
func TestSelectNextMarkerWraps(t *testing.T) {
	a := area("[ONE] middle [TWO] end [THREE]", 60)

	a, ok := a.selectNextMarker()
	if !ok || a.selection() != "[ONE]" {
		t.Fatalf("first = %q", a.selection())
	}

	a, _ = a.selectNextMarker()
	if got := a.selection(); got != "[TWO]" {
		t.Fatalf("second = %q, want [TWO]", got)
	}

	a, _ = a.selectNextMarker()
	if got := a.selection(); got != "[THREE]" {
		t.Fatalf("third = %q, want [THREE]", got)
	}

	// Past the last one it wraps to the first.
	a, _ = a.selectNextMarker()
	if got := a.selection(); got != "[ONE]" {
		t.Errorf("wrapped to %q, want [ONE]", got)
	}
}

func TestSelectNextMarkerWithNone(t *testing.T) {
	a := area("no markers here at all", 40)

	if _, ok := a.selectNextMarker(); ok {
		t.Error("a marker was found in text with none")
	}
	if _, ok := a.selectFirstMarker(); ok {
		t.Error("selectFirstMarker found one in text with none")
	}
}

// A markdown link is not a marker, so cycling must skip it.
func TestMarkersSkipMarkdownLinks(t *testing.T) {
	a := area("see [API](https://x.dev) then [REAL]", 60)

	a, ok := a.selectFirstMarker()
	if !ok {
		t.Fatal("no marker found")
	}
	if got := a.selection(); got != "[REAL]" {
		t.Errorf("selection = %q, want [REAL]", got)
	}
}

// Multibyte text must not be split, since positions are rune indices.
func TestMarkerPositionsSurviveMultibyteText(t *testing.T) {
	a := area("héllo wörld [NAME] ✓", 40)

	a, ok := a.selectFirstMarker()
	if !ok {
		t.Fatal("no marker found")
	}
	if got := a.selection(); got != "[NAME]" {
		t.Errorf("selection = %q, want [NAME]", got)
	}

	a = typeInto(a, "x")
	if want := "héllo wörld x ✓"; a.value() != want {
		t.Errorf("value = %q, want %q", a.value(), want)
	}
}

// Moving the caret clears the selection: the user is done with the marker.
func TestMovementClearsTheSelection(t *testing.T) {
	for _, move := range []struct {
		name string
		key  string
	}{
		{"left", "left"},
		{"right", "right"},
		{"up", "up"},
		{"down", "down"},
		{"home", "home"},
		{"end", "end"},
	} {
		t.Run(move.name, func(t *testing.T) {
			a := area("first [X] line\nsecond line", 40)
			a, _ = a.selectFirstMarker()
			if !a.hasSelection() {
				t.Fatal("nothing was selected")
			}

			a = typeInto(a, move.key)
			if a.hasSelection() {
				t.Error("the selection survived a caret move")
			}
		})
	}
}

func TestTextAreaNewlines(t *testing.T) {
	a := area("", 20)
	a = typeInto(a, "a", "enter", "b")

	if got := a.value(); got != "a\nb" {
		t.Errorf("value = %q, want a\\nb", got)
	}

	rows := a.layout()
	if len(rows) != 2 {
		t.Errorf("layout has %d rows, want 2", len(rows))
	}
}

// Vertical movement follows the wrapped layout, not the source lines.
func TestVerticalMovementAcrossWrappedRows(t *testing.T) {
	// Width 5 wraps this into rows of five.
	a := area("abcdefghij", 5)

	rows := a.layout()
	if len(rows) != 2 {
		t.Fatalf("layout has %d rows, want 2", len(rows))
	}

	a = a.moveTo(2) // row 0, column 2
	a = a.moveVertical(1)
	if a.cursor != 7 {
		t.Errorf("cursor = %d, want 7 (row 1, column 2)", a.cursor)
	}

	a = a.moveVertical(-1)
	if a.cursor != 2 {
		t.Errorf("cursor = %d, want back to 2", a.cursor)
	}

	// Past the ends it clamps rather than wrapping.
	a = a.moveVertical(-5)
	if a.cursor != 2 {
		t.Errorf("cursor = %d, want it to stay put at the top", a.cursor)
	}
}

// A short row cannot hold a long row's column, so the caret clamps to its end.
func TestVerticalMovementClampsColumn(t *testing.T) {
	a := area("long first line\nshort", 40)

	a = a.moveTo(12) // well past the end of "short"
	a = a.moveVertical(1)

	if want := len("long first line\n") + len("short"); a.cursor != want {
		t.Errorf("cursor = %d, want %d (end of the short line)", a.cursor, want)
	}
}

func TestHomeAndEnd(t *testing.T) {
	a := area("first line\nsecond line", 40).moveTo(14)

	a = a.moveHome()
	if want := len("first line\n"); a.cursor != want {
		t.Errorf("home = %d, want %d", a.cursor, want)
	}

	a = a.moveEnd()
	if want := len("first line\nsecond line"); a.cursor != want {
		t.Errorf("end = %d, want %d", a.cursor, want)
	}
}

func TestTextAreaViewShowsSelectionAndCaret(t *testing.T) {
	a := area("[X] tail", 20).setSize(20, 3)

	a, _ = a.selectFirstMarker()
	view := a.view()

	if lines := strings.Split(view, "\n"); len(lines) != 3 {
		t.Errorf("view has %d lines, want 3", len(lines))
	}
	if !strings.Contains(view, "[X]") {
		t.Errorf("the selected marker is not rendered:\n%s", view)
	}

	// With no selection the caret is drawn instead.
	a = a.clearSelection().moveTo(0)
	if got := a.view(); !strings.Contains(got, "[") {
		t.Errorf("the caret row is missing:\n%s", got)
	}
}

// The caret sitting past the last character still gets a cell to occupy.
func TestCaretAtTheEndIsDrawn(t *testing.T) {
	a := area("ab", 20).setSize(20, 1).moveTo(2)

	view := a.view()
	if len([]rune(strings.TrimRight(view, " "))) < 2 {
		t.Errorf("view = %q", view)
	}
}

func TestTextAreaScrolls(t *testing.T) {
	a := area(strings.Repeat("line\n", 20), 20).setSize(20, 5)

	a = a.moveTo(len([]rune(a.value()))).scrollInto()
	if a.offset == 0 {
		t.Error("the view did not scroll to the caret")
	}

	a = a.moveTo(0).scrollInto()
	if a.offset != 0 {
		t.Errorf("offset = %d, want 0 back at the top", a.offset)
	}
}

func TestTextAreaIgnoresUnknownKeys(t *testing.T) {
	a := area("text", 20)

	if _, claimed := a.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS}); claimed {
		t.Error("the editor claimed ctrl-s, which belongs to the modal")
	}
}
