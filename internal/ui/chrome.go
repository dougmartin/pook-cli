package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	tabBarHeight    = 1
	statusBarHeight = 1
)

// tabDivider separates the tabs. The active one carries its own background,
// so the divider only has to keep the inactive ones from running together.
const tabDivider = "|"

// pad extends s with spaces to a display width of n.
func pad(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

// fitBlock forces a rendered block to exactly width by height cells, so the
// status bar always lands on the last row no matter what a tab returns.
func fitBlock(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	out := make([]string, height)
	for i := range out {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(out, "\n")
}

// badgeText renders a tab's badge: a change count, or the activity dot.
func badgeText(b Badge) string {
	switch {
	case b.Count > 0:
		return fmt.Sprintf(" %d", b.Count)
	case b.Dot:
		return " •"
	default:
		return ""
	}
}

// tabBar is the top row: the app name, then every tab with its badge, the
// active one highlighted.
func (m Model) tabBar() string {
	var b strings.Builder
	b.WriteString(styleAppName.Render(" pook "))

	for i, t := range m.tabs {
		if i > 0 {
			b.WriteString(styleTabDivider.Render(tabDivider))
		}

		base, badge := styleTabInactive, styleBadgeInactive
		if i == m.active {
			base, badge = styleTabActive, styleBadgeActive
		}
		b.WriteString(base.Render(fmt.Sprintf(" %d %s", i+1, t.Title())))
		if txt := badgeText(t.Badge()); txt != "" {
			b.WriteString(badge.Render(txt))
		}
		b.WriteString(base.Render(" "))
	}

	return fillBar(b.String(), m.width)
}

// statusBar is the bottom row, visible on every tab: the watched-path alert,
// the heartbeat, and a key hint.
func (m Model) statusBar() string {
	var left string
	if m.alert != "" {
		left += styleAlertBar.Render(" ! " + m.alert + " ")
	}
	left += styleBar.Render(" " + m.heartbeatText() + " ")

	right := styleBarDim.Render(" ? help · q quit ")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		// Too narrow for both: the heartbeat is the more useful of the two.
		return fillBar(ansi.Truncate(left, m.width, ""), m.width)
	}
	return left + styleBar.Render(strings.Repeat(" ", gap)) + right
}

// bannerRow is the idle warning, across the full width just above the status
// bar so it cannot be missed.
func (m Model) bannerRow() string {
	return fillWith(styleBannerBar, " ! "+m.banner+"  (esc dismisses) ", m.width)
}

// heartbeatText is the "is the agent still working" line.
func (m Model) heartbeatText() string {
	if !m.hasAct {
		return "watching for changes"
	}
	return fmt.Sprintf("last change %s ago · %s",
		formatAgo(m.now().Sub(m.activity.At)), m.activity.Text)
}

// formatAgo renders an elapsed duration compactly: seconds while the number is
// small, then minutes, then hours.
func formatAgo(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// fillBar pads a bar row out to the full width with the bar background, or
// truncates it if the terminal is narrower than its content.
func fillBar(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		return ansi.Truncate(s, width, "")
	}
	return s + styleBar.Render(strings.Repeat(" ", width-w))
}

// fillWith renders text in one style across the full width.
func fillWith(style lipgloss.Style, text string, width int) string {
	rendered := style.Render(text)
	w := lipgloss.Width(rendered)
	if w > width {
		return ansi.Truncate(rendered, width, "")
	}
	return rendered + style.Render(strings.Repeat(" ", width-w))
}
