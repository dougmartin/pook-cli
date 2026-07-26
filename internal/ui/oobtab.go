package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/oob"
)

// OOBTab lists the out-of-band files for this repo, grouped by namespace.
//
// It reuses the Changes accordion, with plain content: an oob file is a note,
// not a diff, so nothing here is colored as one.
type OOBTab struct {
	acc    accordion
	groups []oob.Group

	// files maps a row key back to the file it came from, for opening in an
	// editor.
	files map[string]oob.File

	badge         Badge
	message       string
	width, height int
}

func NewOOBTab() *OOBTab {
	return &OOBTab{acc: newAccordion(), files: map[string]oob.File{}}
}

func (t *OOBTab) Title() string        { return "oob" }
func (t *OOBTab) Badge() Badge         { return t.badge }
func (t *OOBTab) CapturingInput() bool { return false }

func (t *OOBTab) Bindings() []Binding {
	return append(slices.Clone(accordionBindings), keyOpen)
}

func (t *OOBTab) Focus() Tab {
	t.badge.Dot = false
	return t
}

func (t *OOBTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = msg.Width, msg.Height

	case ActivityMsg:
		t.badge.Dot = true

	case RefreshMsg:
		t.setGroups(msg.Snap.OOB)

	case editorFailedMsg:
		t.message = "editor: " + msg.Err.Error()

	case tea.KeyMsg:
		return t.handleKey(msg)
	}
	return t, nil
}

func (t *OOBTab) setGroups(groups []oob.Group) {
	t.groups = groups
	t.files = map[string]oob.File{}

	var rows []accordionRow
	for _, g := range groups {
		// Every namespace gets a header, even an empty one, so the layout
		// says what exists rather than what happens to have files today.
		rows = append(rows, groupHeaderRow(g))

		for _, f := range g.Files {
			key := string(g.Namespace) + "/" + f.Path
			t.files[key] = f
			rows = append(rows, accordionRow{
				key:    key,
				header: oobHeader(f),
				body:   oobBody(f),
			})
		}
	}

	t.acc = t.acc.setRows(rows)
}

// groupHeaderRow is a namespace heading: selectable, but with nothing to open.
func groupHeaderRow(g oob.Group) accordionRow {
	count := fmt.Sprintf("%d files", len(g.Files))
	if len(g.Files) == 1 {
		count = "1 file"
	}

	return accordionRow{
		key: "namespace:" + string(g.Namespace),
		header: func(selected bool) string {
			style := styleTitle
			if selected {
				style = styleSelected
			}
			return fmt.Sprintf(" %s %s", style.Render(g.Label), styleDim.Render(count))
		},
	}
}

func oobHeader(f oob.File) func(bool) string {
	size := fmt.Sprintf("%d chars", len([]rune(f.Content)))
	switch {
	case f.Binary:
		size = "binary"
	case f.Truncated:
		size = fmt.Sprintf("%d chars, truncated", len([]rune(f.Content)))
	}

	return func(selected bool) string {
		style := styleText
		if selected {
			style = styleSelected
		}
		return fmt.Sprintf("  %s %s", style.Render(f.Path), styleDim.Render(size))
	}
}

// oobBody is the file's content, plain.
func oobBody(f oob.File) []string {
	if f.Binary {
		return []string{styleDim.Render("    binary file")}
	}
	if strings.TrimSpace(f.Content) == "" {
		return []string{styleDim.Render("    empty")}
	}

	lines := strings.Split(strings.TrimRight(f.Content, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, styleContext.Render("    "+l))
	}
	return out
}

func (t *OOBTab) handleKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	if acc, claimed := t.acc.handleAccordionKey(k, t.listHeight()); claimed {
		t.acc = acc
		return t, nil
	}

	if keyOpen.Matches(k) {
		row, ok := t.acc.currentRow()
		if !ok {
			return t, nil
		}
		f, isFile := t.files[row.key]
		if !isFile {
			t.message = "that is a namespace, not a file"
			return t, nil
		}
		// oob files live outside the repo, so the absolute path is what the
		// editor is given.
		return t, openInEditor("", f.FSPath, 0)
	}
	return t, nil
}

func (t *OOBTab) listHeight() int {
	h := t.height - 1
	if t.message != "" {
		h--
	}
	return max(0, h)
}

func (t *OOBTab) summary() string {
	var files int
	for _, g := range t.groups {
		files += len(g.Files)
	}
	return styleDim.Render(fmt.Sprintf("%d files in %s", files, oob.Home()))
}

func (t *OOBTab) View(width, height int) string {
	t.width, t.height = width, height

	listHeight := t.listHeight()
	t.acc = t.acc.scrollInto(listHeight)

	rows := []string{t.summary(), t.acc.view(width, listHeight, "no oob files for this repo")}
	if t.message != "" {
		rows = append(rows, styleDim.Render(t.message))
	}
	return strings.Join(rows, "\n")
}
