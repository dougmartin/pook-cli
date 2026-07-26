package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/git"
)

// commitDiffMsg delivers a lazily loaded commit's per-file diffs.
type commitDiffMsg struct {
	Hash  string
	Files []git.CommitFileDiff
	Err   error
}

// markMsg records when the user marked the working tree reviewed. The Branch
// tab uses it to flag commits made since.
type markMsg struct{ At time.Time }

// BranchTab lists the commits unique to this branch.
type BranchTab struct {
	repo git.Repo

	acc  accordion
	info git.BranchInfo

	// diffs caches per-commit file diffs forever. Commits are immutable, so a
	// commit loaded once never needs loading again.
	diffs   map[string][]git.CommitFileDiff
	loading map[string]bool

	markedAt time.Time
	badge    Badge

	width, height int
}

func NewBranchTab(repo git.Repo) *BranchTab {
	return &BranchTab{
		repo:    repo,
		acc:     newAccordion(),
		diffs:   map[string][]git.CommitFileDiff{},
		loading: map[string]bool{},
	}
}

func (t *BranchTab) Title() string        { return "Branch" }
func (t *BranchTab) Badge() Badge         { return t.badge }
func (t *BranchTab) CapturingInput() bool { return false }
func (t *BranchTab) Bindings() []Binding  { return slices.Clone(accordionBindings) }

func (t *BranchTab) Focus() Tab {
	t.badge.Dot = false
	return t
}

func (t *BranchTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = msg.Width, msg.Height

	case ActivityMsg:
		t.badge.Dot = true

	case RefreshMsg:
		t.setInfo(msg.Snap.Branch)

	case markMsg:
		t.markedAt = msg.At
		t.rebuild()

	case commitDiffMsg:
		delete(t.loading, msg.Hash)
		if msg.Err == nil {
			t.diffs[msg.Hash] = msg.Files
		} else {
			// A commit that failed to load caches as empty rather than
			// retrying forever on every frame.
			t.diffs[msg.Hash] = nil
		}
		t.rebuild()

	case tea.KeyMsg:
		return t.handleKey(msg)
	}
	return t, nil
}

func (t *BranchTab) setInfo(info git.BranchInfo) {
	t.info = info
	t.rebuild()
}

func (t *BranchTab) rebuild() {
	rows := make([]accordionRow, 0, len(t.info.Commits))
	for _, c := range t.info.Commits {
		rows = append(rows, accordionRow{
			key:    c.Hash,
			header: t.headerFor(c),
			body:   t.bodyFor(c.Hash),
		})
	}
	t.acc = t.acc.setRows(rows)
}

// headerFor renders one commit row.
func (t *BranchTab) headerFor(c git.CommitEntry) func(bool) string {
	fresh := " "
	if !t.markedAt.IsZero() && c.Time.After(t.markedAt) {
		fresh = "*"
	}

	stats := fmt.Sprintf("+%d -%d in %d", c.Additions, c.Deletions, c.FileCount)

	return func(selected bool) string {
		open := "▸"
		if t.acc.isExpanded(c.Hash) {
			open = "▾"
		}

		subject := styleText
		if selected {
			subject = styleSelected
		}

		return fmt.Sprintf("%s%s %s %s %s",
			styleStale.Render(fresh),
			styleDim.Render(open),
			styleHunk.Render(c.Short),
			subject.Render(c.Subject),
			styleDim.Render(stats),
		)
	}
}

// bodyFor renders a commit's cached diffs, or a placeholder while they load.
func (t *BranchTab) bodyFor(hash string) []string {
	files, ok := t.diffs[hash]
	if !ok {
		return []string{styleDim.Render("  loading…")}
	}
	if len(files) == 0 {
		return []string{styleDim.Render("  no file changes")}
	}

	var out []string
	for _, f := range files {
		counts := fmt.Sprintf("+%d -%d", f.Additions, f.Deletions)
		if f.Binary {
			counts = "binary"
		}
		out = append(out, fmt.Sprintf("  %s %s",
			styleText.Render(f.Path), styleDim.Render(counts)))
		out = append(out, colorizeDiff(f.Diff)...)
	}
	return out
}

func (t *BranchTab) handleKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	acc, claimed := t.acc.handleAccordionKey(k, t.listHeight())
	if !claimed {
		return t, nil
	}
	t.acc = acc

	// Expanding a commit is what triggers its load, so a branch with 200
	// commits costs nothing until one is opened.
	return t, t.loadVisible()
}

// loadVisible issues loads for every expanded commit that has none cached.
func (t *BranchTab) loadVisible() tea.Cmd {
	var cmds []tea.Cmd
	for _, c := range t.info.Commits {
		if !t.acc.isExpanded(c.Hash) {
			continue
		}
		if _, cached := t.diffs[c.Hash]; cached || t.loading[c.Hash] {
			continue
		}
		t.loading[c.Hash] = true
		cmds = append(cmds, commitDiffCmd(t.repo, c.Hash))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func commitDiffCmd(repo git.Repo, hash string) tea.Cmd {
	return func() tea.Msg {
		files, err := repo.CommitDiff(hash)
		return commitDiffMsg{Hash: hash, Files: files, Err: err}
	}
}

func (t *BranchTab) listHeight() int { return max(0, t.height-1) }

// overview is the top line: the branch, what it is based on, and its totals.
func (t *BranchTab) overview() string {
	if t.info.Name == "" {
		return styleDim.Render("no branch")
	}

	parts := []string{styleSelected.Render(t.info.Name)}
	if t.info.BaseRef != "" {
		parts = append(parts, fmt.Sprintf("%d ahead of %s", len(t.info.Commits), t.info.BaseRef))
	} else {
		parts = append(parts, fmt.Sprintf("%d recent commits", len(t.info.Commits)))
	}
	parts = append(parts,
		fmt.Sprintf("%d files", t.info.FilesTouched),
		fmt.Sprintf("+%d -%d", t.info.TotalAdditions, t.info.TotalDeletions),
	)

	return styleDim.Render(strings.Join(parts, " · "))
}

func (t *BranchTab) View(width, height int) string {
	t.width, t.height = width, height

	listHeight := t.listHeight()
	t.acc = t.acc.scrollInto(listHeight)

	return t.overview() + "\n" + t.acc.view(width, listHeight, "no commits on this branch yet")
}
