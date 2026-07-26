package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/dougmartin/pook-cli/internal/git"
)

// graphMsg delivers a rendered graph.
type graphMsg struct {
	Rows []git.GraphRow
	Err  error
}

// GraphTab shows recent history as git's own commit graph, walking down
// through the merge-base into the base branch so a stack of branches reads as
// a whole.
//
// The Branch tab deliberately shows only the commits unique to this branch;
// this one is the opposite view, for orienting yourself in a stack.
type GraphTab struct {
	repo git.Repo

	acc    accordion
	rows   []git.GraphRow
	detail git.GraphDetail

	branch  string
	loading bool
	err     error
	message string

	badge         Badge
	width, height int
}

func NewGraphTab(repo git.Repo) *GraphTab {
	return &GraphTab{repo: repo, acc: newAccordion()}
}

func (t *GraphTab) Title() string        { return "Graph" }
func (t *GraphTab) Badge() Badge         { return t.badge }
func (t *GraphTab) CapturingInput() bool { return false }

func (t *GraphTab) Bindings() []Binding {
	return []Binding{keyDown, keyUp, keyTop, keyBottom, keyPageDown, keyPageUp,
		keyGraphDetail, keyCopyHash}
}

var (
	keyGraphDetail = Binding{Keys: []string{"s"}, Label: "s", Help: "subject / compact"}
	keyCopyHash    = Binding{Keys: []string{"y"}, Label: "y", Help: "copy commit hash"}
)

func (t *GraphTab) Focus() Tab {
	t.badge.Dot = false
	return t
}

func (t *GraphTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = msg.Width, msg.Height

	case ActivityMsg:
		t.badge.Dot = true

	case RefreshMsg:
		t.branch = msg.Snap.Branch.Name
		return t, t.loadCmd()

	case graphMsg:
		t.loading = false
		t.err = msg.Err
		t.rows = msg.Rows
		t.rebuild()

	case copiedMsg:
		if msg.Err != nil {
			t.message = "copy failed: " + msg.Err.Error()
		} else {
			t.message = "copied " + msg.What
		}

	case tea.KeyMsg:
		return t.handleKey(msg)
	}
	return t, nil
}

// loadCmd re-renders the graph off the UI goroutine.
func (t *GraphTab) loadCmd() tea.Cmd {
	if t.loading {
		return nil
	}
	t.loading = true

	repo, detail := t.repo, t.detail
	// Color comes from git, so it has to be asked for only when pook is
	// rendering in color at all.
	color := lipgloss.ColorProfile() != termenv.Ascii

	return func() tea.Msg {
		rows, err := repo.Graph(detail, git.MaxGraphCommits, color)
		return graphMsg{Rows: rows, Err: err}
	}
}

func (t *GraphTab) rebuild() {
	rows := make([]accordionRow, 0, len(t.rows))
	for i, r := range t.rows {
		// A commit keys on its hash so the cursor stays on the same commit
		// across refreshes; a continuation row has only its position.
		key := r.Hash
		if key == "" {
			key = fmt.Sprintf("row-%d", i)
		}

		text := r.Text
		rows = append(rows, accordionRow{
			key:    key,
			header: func(bool) string { return text },
		})
	}
	t.acc = t.acc.setRows(rows)
}

func (t *GraphTab) handleKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	if acc, claimed := t.acc.handleAccordionKey(k, t.listHeight()); claimed {
		t.acc = acc
		return t, nil
	}

	switch {
	case keyGraphDetail.Matches(k):
		if t.detail == git.GraphSubject {
			t.detail = git.GraphCompact
		} else {
			t.detail = git.GraphSubject
		}
		return t, t.loadCmd()

	case keyCopyHash.Matches(k):
		return t, t.copyHashCmd()
	}
	return t, nil
}

// currentHash is the commit under the cursor, empty when the cursor is on one
// of the graph's continuation rows.
func (t *GraphTab) currentHash() string {
	row, ok := t.acc.currentRow()
	if !ok {
		return ""
	}
	for _, r := range t.rows {
		if r.Hash != "" && r.Hash == row.key {
			return r.Hash
		}
	}
	return ""
}

func (t *GraphTab) copyHashCmd() tea.Cmd {
	hash := t.currentHash()
	if hash == "" {
		return nil
	}
	return copyTextCmd(hash, "commit "+shortHash(hash))
}

func shortHash(hash string) string {
	if len(hash) > 9 {
		return hash[:9]
	}
	return hash
}

func (t *GraphTab) listHeight() int {
	h := t.height - 1 // summary
	if t.message != "" {
		h--
	}
	return max(0, h)
}

// commitCount is how many rows are commits rather than graph continuations.
func (t *GraphTab) commitCount() int {
	var n int
	for _, r := range t.rows {
		if r.Hash != "" {
			n++
		}
	}
	return n
}

func (t *GraphTab) summary() string {
	if t.err != nil {
		return styleDim.Render("no history yet")
	}

	detail := "subject"
	if t.detail == git.GraphCompact {
		detail = "compact"
	}

	parts := []string{}
	if t.branch != "" {
		parts = append(parts, t.branch)
	}
	parts = append(parts,
		fmt.Sprintf("%d commits", t.commitCount()),
		detail,
	)
	if t.commitCount() >= git.MaxGraphCommits {
		// Say so rather than letting a capped view look like the whole story.
		parts = append(parts, fmt.Sprintf("capped at %d", git.MaxGraphCommits))
	}
	return styleDim.Render(strings.Join(parts, " · "))
}

func (t *GraphTab) View(width, height int) string {
	t.width, t.height = width, height

	listHeight := t.listHeight()
	t.acc = t.acc.scrollInto(listHeight)

	empty := "no commits yet"
	if t.loading {
		empty = "loading…"
	}

	rows := []string{t.summary(), t.acc.view(width, listHeight, empty)}
	if t.message != "" {
		rows = append(rows, styleDim.Render(t.message))
	}
	return strings.Join(rows, "\n")
}
