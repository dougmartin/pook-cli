package ui

import "github.com/charmbracelet/lipgloss"

// Colors are adaptive so the bars stay readable on light and dark terminals,
// and are picked from the 256-color cube rather than true color so they
// degrade predictably. Contrast is one of the things the spec flags for human
// review, since the golden tests run with color stripped.
var (
	colText     = lipgloss.AdaptiveColor{Light: "236", Dark: "252"}
	colDim      = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	colAccent   = lipgloss.AdaptiveColor{Light: "25", Dark: "39"}
	colOnAccent = lipgloss.AdaptiveColor{Light: "231", Dark: "16"}
	colAlert    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	colBadge    = lipgloss.AdaptiveColor{Light: "130", Dark: "214"}
	colBarBg    = lipgloss.AdaptiveColor{Light: "254", Dark: "236"}
)

var (
	styleBar       = lipgloss.NewStyle().Background(colBarBg).Foreground(colText)
	styleBarDim    = lipgloss.NewStyle().Background(colBarBg).Foreground(colDim)
	styleAppName   = lipgloss.NewStyle().Background(colBarBg).Foreground(colAccent).Bold(true)
	styleAlertBar  = lipgloss.NewStyle().Background(colAlert).Foreground(colOnAccent).Bold(true)
	styleBannerBar = lipgloss.NewStyle().Background(colBadge).Foreground(colOnAccent).Bold(true)

	styleTabActive   = lipgloss.NewStyle().Background(colAccent).Foreground(colOnAccent).Bold(true)
	styleTabInactive = lipgloss.NewStyle().Background(colBarBg).Foreground(colDim)

	styleBadgeActive   = lipgloss.NewStyle().Background(colAccent).Foreground(colOnAccent).Bold(true)
	styleBadgeInactive = lipgloss.NewStyle().Background(colBarBg).Foreground(colBadge).Bold(true)

	styleDim     = lipgloss.NewStyle().Foreground(colDim)
	styleWatched = lipgloss.NewStyle().Foreground(colBadge)
	styleTitle   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styleKey     = lipgloss.NewStyle().Foreground(colText).Bold(true)

	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).
			Padding(0, 1)
)
