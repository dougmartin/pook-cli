package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// accordionPrefix is the two columns every row spends on the cursor marker,
// and bodyIndent the indent a tab gives its expanded content.
const (
	accordionPrefix = 2
	bodyIndent      = 4
)

// accordionRow is one collapsible entry.
type accordionRow struct {
	// key identifies the row across refreshes, so expansion and the cursor
	// survive the list being rebuilt.
	key string
	// header renders the always-visible line. It takes the selected state
	// rather than returning pre-styled text, because styling a string that
	// already carries escape sequences does not compose.
	header func(selected bool) string
	// body is the pre-styled content revealed when the row is expanded.
	// Building it once per refresh keeps a huge diff off the render path.
	body []string
}

// accLine is one line of the flattened list.
type accLine struct {
	row int
	// body is false for a row's header line.
	body bool
	text string
}

// accordion is a list of collapsible rows flattened into a single virtual line
// list, sliced by cursor and offset.
//
// This is deliberately not a tree of nested components over a viewport:
// bubbletea re-renders the whole frame on every update, so handing a viewport
// a 10,000 line diff is a performance trap. Flattening means the cost of a
// frame is the height of the terminal, whatever the diff holds.
type accordion struct {
	rows     []accordionRow
	expanded map[string]bool

	// cursor and offset index the flattened lines.
	cursor int
	offset int

	lines []accLine

	// pending holds the first key of a two-key sequence, which is how zR and
	// zM arrive: a terminal sends them as separate key events.
	pending string
}

func newAccordion() accordion {
	return accordion{expanded: map[string]bool{}}
}

// setRows replaces the list, keeping the cursor on the same row where it still
// exists and dropping expansion state for rows that are gone.
func (a accordion) setRows(rows []accordionRow) accordion {
	var focusedKey string
	if row, ok := a.currentRow(); ok {
		focusedKey = row.key
	}
	// How far into the row's body the cursor was, so a refresh does not throw
	// away the reader's place in a long diff.
	depth := a.cursorDepth()

	live := make(map[string]bool, len(rows))
	for _, r := range rows {
		if a.expanded[r.key] {
			live[r.key] = true
		}
	}

	a.rows = rows
	a.expanded = live
	a.lines = flatten(rows, live)

	a.cursor = 0
	if focusedKey != "" {
		if i, ok := a.lineOfRow(focusedKey); ok {
			a.cursor = min(i+depth, a.lastLineOfRow(i))
		}
	}
	return a.clamp()
}

// flatten builds the virtual line list.
func flatten(rows []accordionRow, expanded map[string]bool) []accLine {
	var lines []accLine
	for i, r := range rows {
		lines = append(lines, accLine{row: i})
		if !expanded[r.key] {
			continue
		}
		for _, text := range r.body {
			lines = append(lines, accLine{row: i, body: true, text: text})
		}
	}
	return lines
}

// cursorDepth is how many lines below its header the cursor sits.
func (a accordion) cursorDepth() int {
	if a.cursor >= len(a.lines) {
		return 0
	}
	row := a.lines[a.cursor].row
	for i := a.cursor; i >= 0; i-- {
		if a.lines[i].row != row {
			break
		}
		if !a.lines[i].body {
			return a.cursor - i
		}
	}
	return 0
}

// lineOfRow is the header line index for a row key.
func (a accordion) lineOfRow(key string) (int, bool) {
	for i, l := range a.lines {
		if !l.body && a.rows[l.row].key == key {
			return i, true
		}
	}
	return 0, false
}

// lastLineOfRow is the final line index belonging to the row whose header is
// at headerIdx.
func (a accordion) lastLineOfRow(headerIdx int) int {
	row := a.lines[headerIdx].row
	last := headerIdx
	for i := headerIdx + 1; i < len(a.lines) && a.lines[i].row == row; i++ {
		last = i
	}
	return last
}

// currentRow is the row the cursor is inside.
func (a accordion) currentRow() (accordionRow, bool) {
	if a.cursor < 0 || a.cursor >= len(a.lines) {
		return accordionRow{}, false
	}
	return a.rows[a.lines[a.cursor].row], true
}

// currentLine is the line under the cursor.
func (a accordion) currentLine() (accLine, bool) {
	if a.cursor < 0 || a.cursor >= len(a.lines) {
		return accLine{}, false
	}
	return a.lines[a.cursor], true
}

// isExpanded reports whether a row is open.
func (a accordion) isExpanded(key string) bool { return a.expanded[key] }

// toggle opens or closes the row under the cursor, leaving the cursor on its
// header so a collapse cannot strand it inside vanished content.
func (a accordion) toggle() accordion {
	row, ok := a.currentRow()
	if !ok {
		return a
	}

	if a.expanded[row.key] {
		delete(a.expanded, row.key)
	} else {
		a.expanded[row.key] = true
	}

	a.lines = flatten(a.rows, a.expanded)
	if i, found := a.lineOfRow(row.key); found {
		a.cursor = i
	}
	return a.clamp()
}

// setExpandedAll opens or closes every row.
func (a accordion) setExpandedAll(open bool) accordion {
	row, hadRow := a.currentRow()

	a.expanded = map[string]bool{}
	if open {
		for _, r := range a.rows {
			if len(r.body) > 0 {
				a.expanded[r.key] = true
			}
		}
	}

	a.lines = flatten(a.rows, a.expanded)
	if hadRow {
		if i, found := a.lineOfRow(row.key); found {
			a.cursor = i
		}
	}
	return a.clamp()
}

// move shifts the cursor by delta lines.
func (a accordion) move(delta int) accordion {
	a.cursor += delta
	return a.clamp()
}

func (a accordion) top() accordion {
	a.cursor = 0
	return a.clamp()
}

func (a accordion) bottom() accordion {
	a.cursor = len(a.lines) - 1
	return a.clamp()
}

func (a accordion) clamp() accordion {
	a.cursor = min(max(a.cursor, 0), max(len(a.lines)-1, 0))
	return a
}

// scrollInto adjusts the offset so the cursor is visible in a window of the
// given height.
func (a accordion) scrollInto(height int) accordion {
	if height <= 0 {
		a.offset = 0
		return a
	}

	if a.cursor < a.offset {
		a.offset = a.cursor
	}
	if a.cursor >= a.offset+height {
		a.offset = a.cursor - height + 1
	}

	maxOffset := max(0, len(a.lines)-height)
	a.offset = min(max(a.offset, 0), maxOffset)
	return a
}

// view renders exactly height lines.
func (a accordion) view(width, height int, empty string) string {
	if len(a.lines) == 0 {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, styleDim.Render(empty))
	}

	out := make([]string, 0, height)
	for i := a.offset; i < len(a.lines) && len(out) < height; i++ {
		l := a.lines[i]
		selected := i == a.cursor

		var text string
		if l.body {
			text = l.text
		} else {
			text = a.rows[l.row].header(selected)
		}

		prefix := "  "
		if selected {
			prefix = styleCursor.Render("> ")
		}
		out = append(out, ansi.Truncate(prefix+text, width, ""))
	}

	for len(out) < height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// accordionKeys are the bindings every accordion tab shares.
var (
	keyDown     = Binding{Keys: []string{"j", "down"}, Label: "j / down", Help: "move down"}
	keyUp       = Binding{Keys: []string{"k", "up"}, Label: "k / up", Help: "move up"}
	keyTop      = Binding{Keys: []string{"g", "home"}, Label: "g", Help: "top"}
	keyBottom   = Binding{Keys: []string{"G", "end"}, Label: "G", Help: "bottom"}
	keyPageDown = Binding{Keys: []string{"ctrl+d", "pgdown"}, Label: "ctrl-d", Help: "page down"}
	keyPageUp   = Binding{Keys: []string{"ctrl+u", "pgup"}, Label: "ctrl-u", Help: "page up"}
	keyToggle   = Binding{Keys: []string{" ", "enter"}, Label: "space", Help: "expand / collapse"}

	// zR and zM are two-key sequences, matched through the accordion's
	// pending state rather than by Matches. The Keys here are for display.
	keyExpandAll = Binding{Keys: []string{"zR"}, Label: "zR", Help: "expand all"}
	keyCollapse  = Binding{Keys: []string{"zM"}, Label: "zM", Help: "collapse all"}

	keyFilter     = Binding{Keys: []string{"/"}, Label: "/", Help: "filter"}
	keyClearInput = Binding{Keys: []string{"esc"}, Label: "esc", Help: "clear filter"}
	keyOpen       = Binding{Keys: []string{"o"}, Label: "o", Help: "open in $EDITOR"}
)

var accordionBindings = []Binding{
	keyDown, keyUp, keyTop, keyBottom, keyPageDown, keyPageUp,
	keyToggle, keyExpandAll, keyCollapse,
}

// handleNavKey applies only the movement keys that cannot be typed as text,
// so a focused search box can still drive the list underneath it. The letter
// bindings are deliberately absent: while typing, j has to mean j.
func (a accordion) handleNavKey(k tea.KeyMsg, pageSize int) (accordion, bool) {
	switch k.Type {
	case tea.KeyUp:
		return a.move(-1), true
	case tea.KeyDown:
		return a.move(1), true
	case tea.KeyPgUp:
		return a.move(-max(1, pageSize-1)), true
	case tea.KeyPgDown:
		return a.move(max(1, pageSize-1)), true
	}
	return a, false
}

// handleAccordionKey applies the shared navigation keys, reporting whether it
// claimed the key. pageSize is the visible height.
func (a accordion) handleAccordionKey(k tea.KeyMsg, pageSize int) (accordion, bool) {
	// Finish a two-key sequence first, so the z in zR is never taken for a
	// binding of its own.
	if a.pending == "z" {
		a.pending = ""
		switch k.String() {
		case "R":
			return a.setExpandedAll(true), true
		case "M":
			return a.setExpandedAll(false), true
		}
		return a, true // an unknown pair is swallowed, not acted on
	}
	if k.String() == "z" {
		a.pending = "z"
		return a, true
	}

	switch {
	case keyDown.Matches(k):
		return a.move(1), true
	case keyUp.Matches(k):
		return a.move(-1), true
	case keyPageDown.Matches(k):
		return a.move(max(1, pageSize-1)), true
	case keyPageUp.Matches(k):
		return a.move(-max(1, pageSize-1)), true
	case keyTop.Matches(k):
		return a.top(), true
	case keyBottom.Matches(k):
		return a.bottom(), true
	case keyToggle.Matches(k):
		return a.toggle(), true
	}
	return a, false
}
