package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
func helpSection(title string, bindings []Binding) string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(title))
	for _, bind := range bindings {
		fmt.Fprintf(&b, "\n%s  %s",
			styleKey.Render(pad(bind.Display(), 11)),
			styleDim.Render(bind.Help))
	}
	return b.String()
}

// columnize packs sections into as many columns as it takes to fit rows,
// which is what keeps the whole keymap visible once every tab contributes
// bindings. Sections are never split across columns.
func columnize(sections []string, rows int) string {
	rows = max(1, rows)

	var cols []string
	var cur []string
	used := 0

	for _, s := range sections {
		n := strings.Count(s, "\n") + 1
		if len(cur) > 0 && used+1+n > rows {
			cols = append(cols, strings.Join(cur, "\n\n"))
			cur, used = nil, 0
		}
		if len(cur) > 0 {
			used++ // the blank line between sections
		}
		cur = append(cur, s)
		used += n
	}
	if len(cur) > 0 {
		cols = append(cols, strings.Join(cur, "\n\n"))
	}

	for i, c := range cols {
		if i < len(cols)-1 {
			cols[i] = lipgloss.NewStyle().PaddingRight(3).Render(c)
		} else {
			cols[i] = c
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

// helpOverlay lists every binding: the global keymap, then each tab's own
// keys. It is built from the same Binding values that dispatch uses, so a key
// cannot be handled without appearing here.
type helpOverlay struct {
	sections []string
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
	if ok && (keyClose.Matches(k) || keyHelp.Matches(k) || keyQuit.Matches(k)) {
		return nil, nil
	}
	return h, nil
}

func (h helpOverlay) View(width, height int) string {
	// The panel costs two border rows, the header and the blank under it.
	return panel(
		styleTitle.Render("Keys")+"  "+styleDim.Render("esc closes"),
		columnize(h.sections, height-4),
		width, height,
	)
}

// tickerOverlay is the activity ticker. Phase 3 fills it with the rolling log
// of the last 50 file events; for now it reports that nothing is watched yet.
type tickerOverlay struct{}

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
		styleDim.Render("no events yet, watchers arrive in phase 3"),
		width, height,
	)
}

// clipboardModal is the phase 7 clipboard editor. It exists now as the one
// thing routed ahead of overlays, which is what keeps the update loop's
// precedence honest.
type clipboardModal struct{}

func (c clipboardModal) Update(msg tea.Msg) (layer, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if ok && keyClose.Matches(k) {
		return nil, nil
	}
	return c, nil
}

func (c clipboardModal) View(width, height int) string {
	return panel(
		styleTitle.Render("Clipboard")+"  "+styleDim.Render("esc closes"),
		styleDim.Render("editable clipboard arrives in phase 7"),
		width, height,
	)
}
