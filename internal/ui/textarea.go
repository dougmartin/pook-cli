package ui

import (
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// textArea is a multiline editor with a selection range.
//
// bubbles/textarea has no selection model, and the clipboard modal needs one:
// opening with a [MARKER] selected so that typing replaces it is the whole
// point of the feature. Rather than fork the component, this is a small
// editor that does only what is needed, over a flat rune slice.
//
// Positions are rune indices throughout. Bytes never appear outside the
// conversion helpers, so a multibyte character cannot be split.
type textArea struct {
	runes  []rune
	cursor int

	// selStart and selEnd bound the selection, end exclusive. Equal means no
	// selection.
	selStart int
	selEnd   int

	width, height int
	// offset is the first visible visual row.
	offset int
}

func newTextArea() textArea { return textArea{} }

func (a textArea) value() string { return string(a.runes) }

func (a textArea) setValue(s string) textArea {
	a.runes = []rune(s)
	a.cursor = min(a.cursor, len(a.runes))
	a.selStart, a.selEnd = 0, 0
	return a
}

func (a textArea) setSize(width, height int) textArea {
	a.width = max(1, width)
	a.height = max(1, height)
	return a
}

// hasSelection reports whether anything is selected.
func (a textArea) hasSelection() bool { return a.selEnd > a.selStart }

// selection is the selected text.
func (a textArea) selection() string {
	if !a.hasSelection() {
		return ""
	}
	return string(a.runes[a.selStart:a.selEnd])
}

// selectRange selects [start, end) and puts the cursor at its end, so typing
// replaces the range and the caret lands where the replacement finishes.
func (a textArea) selectRange(start, end int) textArea {
	start = min(max(start, 0), len(a.runes))
	end = min(max(end, start), len(a.runes))

	a.selStart, a.selEnd = start, end
	a.cursor = end
	return a
}

func (a textArea) clearSelection() textArea {
	a.selStart, a.selEnd = 0, 0
	return a
}

// deleteSelection removes the selected range and leaves the cursor where it
// was.
func (a textArea) deleteSelection() textArea {
	if !a.hasSelection() {
		return a
	}

	next := make([]rune, 0, len(a.runes)-(a.selEnd-a.selStart))
	next = append(next, a.runes[:a.selStart]...)
	next = append(next, a.runes[a.selEnd:]...)

	a.cursor = a.selStart
	a.runes = next
	return a.clearSelection()
}

// insert types text at the cursor, replacing the selection when there is one.
func (a textArea) insert(s string) textArea {
	a = a.deleteSelection()

	in := []rune(s)
	next := make([]rune, 0, len(a.runes)+len(in))
	next = append(next, a.runes[:a.cursor]...)
	next = append(next, in...)
	next = append(next, a.runes[a.cursor:]...)

	a.runes = next
	a.cursor += len(in)
	return a
}

// backspace deletes the selection, or the rune before the cursor.
func (a textArea) backspace() textArea {
	if a.hasSelection() {
		return a.deleteSelection()
	}
	if a.cursor == 0 {
		return a
	}

	a.runes = append(a.runes[:a.cursor-1], a.runes[a.cursor:]...)
	a.cursor--
	return a
}

// deleteForward deletes the selection, or the rune under the cursor.
func (a textArea) deleteForward() textArea {
	if a.hasSelection() {
		return a.deleteSelection()
	}
	if a.cursor >= len(a.runes) {
		return a
	}

	a.runes = append(a.runes[:a.cursor], a.runes[a.cursor+1:]...)
	return a
}

// rowRange is one visual row's rune span, end exclusive.
type rowRange struct{ start, end int }

// layout wraps the text to the current width, splitting on newlines first.
// Wrapping is hard rather than word-aware: this edits clipboard text, where
// preserving exactly what the user typed matters more than pretty reflow.
func (a textArea) layout() []rowRange {
	var rows []rowRange

	start := 0
	for i := 0; i <= len(a.runes); i++ {
		atEnd := i == len(a.runes)
		newline := !atEnd && a.runes[i] == '\n'
		full := i-start >= a.width

		switch {
		case newline:
			rows = append(rows, rowRange{start, i})
			start = i + 1
		case full:
			rows = append(rows, rowRange{start, i})
			start = i
		case atEnd:
			rows = append(rows, rowRange{start, i})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, rowRange{0, 0})
	}
	return rows
}

// rowOf is the visual row holding a position, and the column within it.
func (a textArea) rowOf(pos int) (row, col int) {
	rows := a.layout()
	for i, r := range rows {
		if pos >= r.start && pos <= r.end {
			// A position at a wrap point belongs to the row that starts
			// there, unless that is the last row.
			if pos == r.end && i+1 < len(rows) && rows[i+1].start == pos {
				continue
			}
			return i, pos - r.start
		}
	}
	return len(rows) - 1, 0
}

// Cursor movement. Every move clears the selection: with no shift-selection
// to maintain, a moved caret means the user is done with the marker.

func (a textArea) moveTo(pos int) textArea {
	a.cursor = min(max(pos, 0), len(a.runes))
	return a.clearSelection()
}

func (a textArea) moveLeft() textArea  { return a.moveTo(a.cursor - 1) }
func (a textArea) moveRight() textArea { return a.moveTo(a.cursor + 1) }

func (a textArea) moveVertical(delta int) textArea {
	rows := a.layout()
	row, col := a.rowOf(a.cursor)

	target := min(max(row+delta, 0), len(rows)-1)
	if target == row {
		// Nowhere to go, but the caret still moved as far as the user is
		// concerned, so the selection is released either way.
		return a.moveTo(a.cursor)
	}

	r := rows[target]
	return a.moveTo(r.start + min(col, r.end-r.start))
}

func (a textArea) moveHome() textArea {
	rows := a.layout()
	row, _ := a.rowOf(a.cursor)
	return a.moveTo(rows[row].start)
}

func (a textArea) moveEnd() textArea {
	rows := a.layout()
	row, _ := a.rowOf(a.cursor)
	return a.moveTo(rows[row].end)
}

// selectNextMarker selects the first inline [MARKER] after the cursor,
// wrapping to the first one. It reports whether there was any marker at all.
func (a textArea) selectNextMarker() (textArea, bool) {
	text := a.value()

	ranges := markerRuneRanges(text)
	if len(ranges) == 0 {
		return a, false
	}

	for _, r := range ranges {
		if r.start >= a.cursor {
			return a.selectRange(r.start, r.end), true
		}
	}
	// Past the last marker, so wrap to the first.
	return a.selectRange(ranges[0].start, ranges[0].end), true
}

// selectFirstMarker selects the first marker in the text.
func (a textArea) selectFirstMarker() (textArea, bool) {
	ranges := markerRuneRanges(a.value())
	if len(ranges) == 0 {
		return a, false
	}
	return a.selectRange(ranges[0].start, ranges[0].end), true
}

// handleKey applies an editing key, reporting whether it was claimed.
func (a textArea) handleKey(k tea.KeyMsg) (textArea, bool) {
	switch k.Type {
	case tea.KeyRunes:
		return a.insert(string(k.Runes)), true
	case tea.KeySpace:
		return a.insert(" "), true
	case tea.KeyEnter:
		return a.insert("\n"), true
	case tea.KeyBackspace:
		return a.backspace(), true
	case tea.KeyDelete:
		return a.deleteForward(), true
	case tea.KeyLeft:
		return a.moveLeft(), true
	case tea.KeyRight:
		return a.moveRight(), true
	case tea.KeyUp:
		return a.moveVertical(-1), true
	case tea.KeyDown:
		return a.moveVertical(1), true
	case tea.KeyHome, tea.KeyCtrlA:
		return a.moveHome(), true
	case tea.KeyEnd, tea.KeyCtrlE:
		return a.moveEnd(), true
	}
	return a, false
}

// scrollInto keeps the cursor's row visible.
func (a textArea) scrollInto() textArea {
	rows := a.layout()
	row, _ := a.rowOf(a.cursor)

	if row < a.offset {
		a.offset = row
	}
	if row >= a.offset+a.height {
		a.offset = row - a.height + 1
	}

	a.offset = min(max(a.offset, 0), max(0, len(rows)-a.height))
	return a
}

// view renders exactly height rows, with the selection highlighted and the
// cursor drawn as a reversed cell.
func (a textArea) view() string {
	rows := a.layout()

	out := make([]string, 0, a.height)
	for i := a.offset; i < len(rows) && len(out) < a.height; i++ {
		out = append(out, a.renderRow(rows[i]))
	}
	for len(out) < a.height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

func (a textArea) renderRow(r rowRange) string {
	var b strings.Builder

	for pos := r.start; pos < r.end; pos++ {
		ch := string(a.runes[pos])
		switch {
		case pos == a.cursor && !a.hasSelection():
			b.WriteString(styleCaret.Render(ch))
		case a.hasSelection() && pos >= a.selStart && pos < a.selEnd:
			b.WriteString(styleSelection.Render(ch))
		default:
			b.WriteString(styleContext.Render(ch))
		}
	}

	// A cursor at the very end of a row has no character to sit on, so it
	// gets a drawn block.
	if a.cursor == r.end && !a.hasSelection() {
		b.WriteString(styleCaret.Render(" "))
	}
	return b.String()
}

// markerRuneRanges is MarkerIndexes converted from byte offsets to rune
// indices, which is what this editor counts in.
func markerRuneRanges(text string) []rowRange {
	var out []rowRange
	for _, loc := range markerIndexes(text) {
		out = append(out, rowRange{
			start: utf8.RuneCountInString(text[:loc[0]]),
			end:   utf8.RuneCountInString(text[:loc[1]]),
		})
	}
	return out
}
