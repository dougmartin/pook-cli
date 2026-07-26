package ui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

// Binding is one entry in the keymap. It is the single source of truth for
// both dispatch and the help overlay, so a binding can never be handled
// without also being documented.
type Binding struct {
	// Keys are the key strings bubbletea reports, in the order they should
	// be tried.
	Keys []string
	// Label is how the binding appears in help, when the raw key list would
	// read badly (for example "1-5" for the five digit keys).
	Label string
	// Help is the one-line description shown in the help overlay.
	Help string
}

// Matches reports whether k triggers this binding.
func (b Binding) Matches(k tea.KeyMsg) bool {
	s := k.String()
	for _, key := range b.Keys {
		if key == s {
			return true
		}
	}
	return false
}

// Display is the label to print in help.
func (b Binding) Display() string {
	if b.Label != "" {
		return b.Label
	}
	out := ""
	for i, k := range b.Keys {
		if i > 0 {
			out += " / "
		}
		out += k
	}
	return out
}

// Global bindings, available from every tab.
var (
	keySelectTab = Binding{Keys: []string{"1", "2", "3", "4", "5", "6"}, Label: "1-6", Help: "select tab"}
	keyNextTab   = Binding{Keys: []string{"tab", "right"}, Label: "tab / right", Help: "next tab"}
	keyPrevTab   = Binding{Keys: []string{"shift+tab", "left"}, Label: "shift-tab / left", Help: "previous tab"}
	keyTicker    = Binding{Keys: []string{"a"}, Help: "activity ticker"}
	keyClipboard = Binding{Keys: []string{"c"}, Help: "clipboard modal"}
	keyRefresh   = Binding{Keys: []string{"r"}, Help: "force refresh"}
	keyHelp      = Binding{Keys: []string{"?"}, Help: "help"}
	keyQuit      = Binding{Keys: []string{"q"}, Help: "quit"}

	// keyForceQuit is handled ahead of any modal or overlay, so no layer can
	// ever trap the user. It is not listed in help; ctrl+c is universal.
	keyForceQuit = Binding{Keys: []string{"ctrl+c"}}

	// keyClose dismisses whichever modal or overlay is open.
	keyClose = Binding{Keys: []string{"esc"}, Help: "close"}
)

// globalBindings is the help-overlay ordering for the global keymap.
var globalBindings = []Binding{
	keySelectTab, keyNextTab, keyPrevTab,
	keyTicker, keyClipboard, keyRefresh,
	keyHelp, keyQuit,
}

// tabDigit maps a "1".."9" key to a zero-based tab index.
func tabDigit(k tea.KeyMsg) (int, bool) {
	if !keySelectTab.Matches(k) {
		return 0, false
	}
	n, err := strconv.Atoi(k.String())
	if err != nil {
		return 0, false
	}
	return n - 1, true
}
