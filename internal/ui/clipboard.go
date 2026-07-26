package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/clip"
	"github.com/dougmartin/pook-cli/internal/prompts"
)

// openClipboardMsg opens the clipboard modal over some text.
type openClipboardMsg struct {
	Text string
	// SelectMarker asks for the first inline [MARKER] to be selected, which
	// is what a copied prompt wants: typing then replaces it.
	SelectMarker bool
	// Err is set when the clipboard could not be read.
	Err error
}

// markerIndexes is the prompts package's marker scanner, named here so the
// editor does not have to import it directly.
func markerIndexes(text string) [][]int { return prompts.MarkerIndexes(text) }

// clipboardModal is an editable view of the clipboard.
//
// It is the one thing routed ahead of overlays, and the only place in pook
// with a selection model.
type clipboardModal struct {
	area    textArea
	message string
	// markers is how many inline markers the text had when it opened, which
	// decides whether the Tab hint is worth showing.
	markers int
}

// readClipboardCmd opens the modal over whatever is on the clipboard.
func readClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		if !clip.CanRead() {
			return openClipboardMsg{Err: clip.ErrNoReader}
		}
		text, err := clip.Read()
		return openClipboardMsg{Text: text, Err: err}
	}
}

func newClipboardModal(msg openClipboardMsg) clipboardModal {
	m := clipboardModal{area: newTextArea().setValue(msg.Text)}
	m.markers = len(markerIndexes(msg.Text))

	switch {
	case msg.Err != nil:
		// Over ssh with no local helper the clipboard can be written but not
		// read, so the modal opens empty and says why.
		m.message = "cannot read the clipboard: " + msg.Err.Error()
	case msg.SelectMarker:
		if area, ok := m.area.selectFirstMarker(); ok {
			m.area = area
			m.message = "typing replaces the selected marker"
		}
	}
	return m
}

var (
	keySaveClipboard = Binding{Keys: []string{"ctrl+s"}, Label: "ctrl-s", Help: "save to clipboard"}
	keyNextMarker    = Binding{Keys: []string{"tab"}, Label: "tab", Help: "select next marker"}
)

func (c clipboardModal) Update(msg tea.Msg) (layer, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}

	switch {
	case keyClose.Matches(k):
		return nil, nil

	case keySaveClipboard.Matches(k):
		text := c.area.value()
		return nil, func() tea.Msg {
			return copiedMsg{What: "clipboard", Err: clip.Write(text)}
		}

	case keyNextMarker.Matches(k):
		area, found := c.area.selectNextMarker()
		c.area = area
		if !found {
			c.message = "no markers left"
		}
		return c, nil
	}

	if area, claimed := c.area.handleKey(k); claimed {
		c.area = area
		c.message = ""
		return c, nil
	}
	return c, nil
}

func (c clipboardModal) View(width, height int) string {
	inner := max(1, width-panelChrome)
	rows := max(1, height-panelRows-1) // one row for the hint line

	c.area = c.area.setSize(inner, rows).scrollInto()

	hint := "ctrl-s saves · esc closes"
	if c.markers > 0 {
		hint = fmt.Sprintf("%s · tab cycles %d markers", hint, c.markers)
	}

	body := c.area.view()
	if c.message != "" {
		body += "\n" + styleDim.Render(c.message)
	}

	return panel(
		styleTitle.Render("Clipboard")+"  "+styleDim.Render(hint),
		body,
		width, height,
	)
}
