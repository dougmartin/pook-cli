package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dougmartin/pook-cli/internal/monitor"
)

// layer is a modal or overlay drawn over the pane. Update returns the layer to
// keep, or nil to dismiss it. The tab bar and status bar stay visible
// underneath, so the heartbeat is readable even with help open.
type layer interface {
	Update(tea.Msg) (layer, tea.Cmd)
	View(width, height int) string
}

// panel draws a box centered in the pane, clamped so a narrow terminal cannot
// push the border past the edge. The header arrives already styled, so a
// caller can mix a title with a hint without nesting styles.
func panel(header, body string, width, height int) string {
	inner := max(1, width-6)
	box := stylePanel.Width(inner).MaxHeight(max(1, height)).Render(header + "\n\n" + body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// helpSection renders a titled block of "key   description" rows.
//
// The key gutter is sized to the section's own longest key rather than a fixed
// width, so a long one like "shift-tab / left" cannot push its description out
// of line.
func helpSection(title string, bindings []Binding) string {
	gutter := minKeyGutter
	for _, bind := range bindings {
		gutter = max(gutter, lipgloss.Width(bind.Display()))
	}

	var b strings.Builder
	b.WriteString(styleTitle.Render(title))
	for _, bind := range bindings {
		fmt.Fprintf(&b, "\n%s  %s",
			styleKey.Render(pad(bind.Display(), gutter)),
			styleDim.Render(bind.Help))
	}
	return b.String()
}

// minKeyGutter keeps short sections from looking cramped.
const minKeyGutter = 11

// columnize packs sections into as many columns as the width allows,
// balancing their heights. Sections are never split across columns.
func columnize(sections []string, width int) []string {
	if len(sections) == 0 {
		return nil
	}

	colWidth := 0
	heights := make([]int, len(sections))
	for i, s := range sections {
		lines := strings.Split(s, "\n")
		heights[i] = len(lines)
		for _, l := range lines {
			colWidth = max(colWidth, lipgloss.Width(l))
		}
	}
	colWidth += columnGap

	cols := max(1, width/max(1, colWidth))
	cols = min(cols, len(sections))

	total := 0
	for _, h := range heights {
		total += h + 1 // the blank line between sections
	}
	target := (total + cols - 1) / cols

	var packed []string
	var cur []string
	used := 0
	for i, s := range sections {
		// Start a new column once this one is full, as long as there are
		// columns left to start.
		if len(cur) > 0 && used+heights[i]+1 > target && len(packed) < cols-1 {
			packed = append(packed, strings.Join(cur, "\n\n"))
			cur, used = nil, 0
		}
		if len(cur) > 0 {
			used++
		}
		cur = append(cur, s)
		used += heights[i]
	}
	if len(cur) > 0 {
		packed = append(packed, strings.Join(cur, "\n\n"))
	}

	for i := range packed {
		if i < len(packed)-1 {
			packed[i] = lipgloss.NewStyle().Width(colWidth).Render(packed[i])
		}
	}
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, packed...), "\n")
}

// columnGap is the space between help columns.
const columnGap = 3

// helpOverlay lists every binding: the global keymap, then each tab's own
// keys. It is built from the same Binding values that dispatch uses, so a key
// cannot be handled without appearing here.
//
// The list packs into columns to fit the width and scrolls when it still does
// not fit, because with every tab contributing a keymap it no longer does at
// 80x24.
type helpOverlay struct {
	sections []string
	offset   int
}

func newHelpOverlay(m Model) helpOverlay {
	sections := []string{helpSection("Global", globalBindings)}
	for _, t := range m.tabs {
		// Tabs with no keys of their own are left out rather than listed
		// as empty.
		if b := t.Bindings(); len(b) > 0 {
			sections = append(sections, helpSection(t.Title(), b))
		}
	}
	return helpOverlay{sections: sections}
}

func (h helpOverlay) Update(msg tea.Msg) (layer, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return h, nil
	}
	if keyClose.Matches(k) || keyHelp.Matches(k) || keyQuit.Matches(k) {
		return nil, nil
	}

	switch {
	case keyDown.Matches(k):
		h.offset++
	case keyUp.Matches(k):
		h.offset = max(0, h.offset-1)
	case keyPageDown.Matches(k):
		h.offset += helpPage
	case keyPageUp.Matches(k):
		h.offset = max(0, h.offset-helpPage)
	}
	return h, nil
}

// helpPage is how far the paging keys move in the overlay.
const helpPage = 10

func (h helpOverlay) View(width, height int) string {
	inner := max(1, width-panelChrome)
	rows := max(1, height-panelRows)

	lines := columnize(h.sections, inner)

	offset := min(h.offset, max(0, len(lines)-rows))
	end := min(offset+rows, len(lines))
	visible := lines[offset:end]

	hint := "esc closes"
	if len(lines) > rows {
		hint = fmt.Sprintf("esc closes · j/k scrolls · %d-%d of %d",
			offset+1, end, len(lines))
	}

	return panel(
		styleTitle.Render("Keys")+"  "+styleDim.Render(hint),
		strings.Join(visible, "\n"),
		width, height,
	)
}

// panelChrome is the width a panel spends on its border and padding, and
// panelRows the height it spends on its border, header and the blank under it.
const (
	panelChrome = 6
	panelRows   = 4
)

// tickerOverlay is the activity ticker: the rolling log of the last 50 file
// and commit events. Newest first, so the most recent activity is visible
// without scrolling.
type tickerOverlay struct {
	body string
}

func newTickerOverlay(events []monitor.Event, now time.Time) tickerOverlay {
	if len(events) == 0 {
		return tickerOverlay{body: styleDim.Render("nothing has happened yet")}
	}

	var b strings.Builder
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if b.Len() > 0 {
			b.WriteString("\n")
		}

		text := styleDim.Render(e.Text)
		if e.Watched {
			text = styleWatched.Render(e.Text)
		}
		fmt.Fprintf(&b, "%s  %s", styleKey.Render(pad(formatAgo(now.Sub(e.At))+" ago", 9)), text)
	}
	return tickerOverlay{body: b.String()}
}

func (t tickerOverlay) Update(msg tea.Msg) (layer, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if ok && (keyClose.Matches(k) || keyTicker.Matches(k) || keyQuit.Matches(k)) {
		return nil, nil
	}
	return t, nil
}

func (t tickerOverlay) View(width, height int) string {
	return panel(
		styleTitle.Render("Activity")+"  "+styleDim.Render("esc closes"),
		t.body,
		width, height,
	)
}
