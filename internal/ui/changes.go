package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/git"
)

// statusFilter narrows the list to one kind of change.
type statusFilter int

const (
	filterAll statusFilter = iota
	filterNew
	filterChanged
	filterDeleted
)

func (f statusFilter) String() string {
	return [...]string{"all", "new", "changed", "deleted"}[f]
}

func (f statusFilter) matches(s git.Status) bool {
	switch f {
	case filterNew:
		return s == git.StatusNew
	case filterChanged:
		return s == git.StatusChanged
	case filterDeleted:
		return s == git.StatusDeleted
	default:
		return true
	}
}

// sortMode orders the file list.
type sortMode int

const (
	sortLatest sortMode = iota
	sortOldest
	sortAlpha
	sortMostChanges
)

func (s sortMode) String() string {
	return [...]string{"latest", "oldest", "a-z", "most"}[s]
}

// discardedMsg reports the result of a discard.
type discardedMsg struct {
	Path string
	Err  error
}

// ChangesTab lists every file differing from HEAD.
type ChangesTab struct {
	repo git.Repo

	acc   accordion
	files []git.FileEntry
	// raw holds each file's uncolored diff lines, which is what the editor
	// jump needs to count line numbers against.
	raw map[string][]string

	status statusFilter
	sort   sortMode

	input     textinput.Model
	filtering bool
	pathQuery string

	// marked is the diff of every file when the user last marked the list
	// reviewed; sinceMark filters to what has changed since.
	marked    map[string]string
	sinceMark bool

	// seen is the diff of a file when it was last expanded, so a file whose
	// content moved on since then can be flagged.
	seen map[string]string

	// confirm holds the path awaiting a discard confirmation.
	confirm string
	// message is a transient line under the list.
	message string

	badge         Badge
	width, height int

	// now is injectable so the review mark is deterministic in tests.
	now func() time.Time
}

// NewChangesTab builds the tab for a repo.
func NewChangesTab(repo git.Repo) *ChangesTab {
	in := textinput.New()
	in.Prompt = "/"
	in.Placeholder = "path"

	return &ChangesTab{
		repo:   repo,
		now:    time.Now,
		acc:    newAccordion(),
		raw:    map[string][]string{},
		marked: map[string]string{},
		seen:   map[string]string{},
		input:  in,
	}
}

func (t *ChangesTab) Title() string { return "Changes" }
func (t *ChangesTab) Badge() Badge  { return t.badge }

func (t *ChangesTab) Bindings() []Binding {
	return append(slices.Clone(accordionBindings),
		keyFilter, keyClearInput, keyOpen,
		keyCycleFilter, keyCycleSort, keyDiscard, keyMark, keySinceMark)
}

// CapturingInput is true while the path filter has focus.
func (t *ChangesTab) CapturingInput() bool { return t.filtering }

func (t *ChangesTab) Focus() Tab {
	t.badge.Dot = false
	return t
}

func (t *ChangesTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = msg.Width, msg.Height

	case ActivityMsg:
		t.badge.Dot = true

	case RefreshMsg:
		t.setFiles(msg.Snap.Files)

	case editorFailedMsg:
		t.message = "editor: " + msg.Err.Error()

	case discardedMsg:
		if msg.Err != nil {
			t.message = "discard failed: " + msg.Err.Error()
			return t, nil
		}
		t.message = "discarded " + msg.Path
		return t, func() tea.Msg { return needsRefreshMsg{} }

	case tea.KeyMsg:
		return t.handleKey(msg)
	}
	return t, nil
}

// setFiles folds a new snapshot into the list, preserving the cursor,
// expansion and scroll position.
func (t *ChangesTab) setFiles(files []git.FileEntry) {
	t.files = files
	t.badge.Count = len(files)

	t.raw = make(map[string][]string, len(files))
	for _, f := range files {
		t.raw[f.Path] = strings.Split(strings.TrimRight(f.Diff, "\n"), "\n")
	}
	t.rebuild()
}

// rebuild applies the filters and sort, then hands the accordion new rows.
func (t *ChangesTab) rebuild() {
	visible := t.visibleFiles()

	rows := make([]accordionRow, 0, len(visible))
	for _, f := range visible {
		rows = append(rows, accordionRow{
			key:    f.Path,
			header: t.headerFor(f),
			body:   colorizeDiff(f.Diff),
		})
	}
	t.acc = t.acc.setRows(rows)
}

// visibleFiles is the file list after filtering and sorting.
func (t *ChangesTab) visibleFiles() []git.FileEntry {
	query := strings.ToLower(t.pathQuery)

	out := make([]git.FileEntry, 0, len(t.files))
	for _, f := range t.files {
		if !t.status.matches(f.Status) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(f.Path), query) {
			continue
		}
		if t.sinceMark && !t.changedSinceMark(f) {
			continue
		}
		out = append(out, f)
	}

	slices.SortStableFunc(out, func(a, b git.FileEntry) int {
		switch t.sort {
		case sortOldest:
			return a.ModTime.Compare(b.ModTime)
		case sortAlpha:
			return strings.Compare(a.Path, b.Path)
		case sortMostChanges:
			return (b.Additions + b.Deletions) - (a.Additions + a.Deletions)
		default:
			return b.ModTime.Compare(a.ModTime)
		}
	})
	return out
}

// changedSinceMark reports whether a file differs from what it was when the
// list was last marked reviewed. A file that did not exist then counts.
func (t *ChangesTab) changedSinceMark(f git.FileEntry) bool {
	prev, ok := t.marked[f.Path]
	return !ok || prev != f.Diff
}

// staleSinceExpanded reports whether a file's diff moved on since the user
// last had it open.
func (t *ChangesTab) staleSinceExpanded(f git.FileEntry) bool {
	prev, ok := t.seen[f.Path]
	return ok && prev != f.Diff
}

// headerFor renders one file's row.
func (t *ChangesTab) headerFor(f git.FileEntry) func(bool) string {
	marker := " "
	if t.staleSinceExpanded(f) {
		marker = "*"
	}

	badge := statusBadge(f.Status)
	counts := fmt.Sprintf("+%d -%d", f.Additions, f.Deletions)
	if f.Binary {
		counts = "binary"
	}

	path := f.Path
	if f.Watched {
		path += " !"
	}

	return func(selected bool) string {
		open := "▸"
		if t.acc.isExpanded(f.Path) {
			open = "▾"
		}

		pathStyle := styleText
		switch {
		case selected:
			pathStyle = styleSelected
		case f.Watched:
			pathStyle = styleWatched
		}

		return fmt.Sprintf("%s%s %s %s %s",
			styleStale.Render(marker),
			styleDim.Render(open),
			badge,
			pathStyle.Render(path),
			styleDim.Render(counts),
		)
	}
}

func statusBadge(s git.Status) string {
	switch s {
	case git.StatusNew:
		return styleStatusNew.Render("new")
	case git.StatusDeleted:
		return styleStatusDeleted.Render("del")
	default:
		return styleStatusChanged.Render("chg")
	}
}

// Tab-specific bindings.
var (
	keyCycleFilter = Binding{Keys: []string{"f"}, Label: "f", Help: "cycle status filter"}
	keyCycleSort   = Binding{Keys: []string{"s"}, Label: "s", Help: "cycle sort"}
	keyDiscard     = Binding{Keys: []string{"d"}, Label: "d", Help: "discard file"}
	keyMark        = Binding{Keys: []string{"m"}, Label: "m", Help: "mark reviewed"}
	keySinceMark   = Binding{Keys: []string{"M"}, Label: "M", Help: "toggle since-mark"}
	keyConfirmYes  = Binding{Keys: []string{"y", "enter"}, Label: "y", Help: "confirm"}
	keyConfirmNo   = Binding{Keys: []string{"n", "esc"}, Label: "n", Help: "cancel"}
)

func (t *ChangesTab) handleKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	// A pending confirmation owns the keyboard: nothing else should act
	// while the question is on screen.
	if t.confirm != "" {
		switch {
		case keyConfirmYes.Matches(k):
			path, status := t.confirm, t.confirmStatus()
			t.confirm = ""
			return t, discardCmd(t.repo, path, status)
		case keyConfirmNo.Matches(k):
			t.confirm = ""
		}
		return t, nil
	}

	if t.filtering {
		return t.handleFilterKey(k)
	}

	if acc, claimed := t.acc.handleAccordionKey(k, t.listHeight()); claimed {
		t.acc = acc
		// Opening a row is what "seen" means, so the stale marker clears.
		if row, ok := t.acc.currentRow(); ok && t.acc.isExpanded(row.key) {
			t.seen[row.key] = t.diffOf(row.key)
			t.rebuild()
		}
		return t, nil
	}

	switch {
	case keyFilter.Matches(k):
		t.filtering = true
		t.input.SetValue(t.pathQuery)
		t.input.CursorEnd()
		return t, t.input.Focus()

	case keyClearInput.Matches(k):
		if t.pathQuery == "" {
			return t, nil
		}
		t.pathQuery = ""
		t.rebuild()

	case keyCycleFilter.Matches(k):
		t.status = (t.status + 1) % 4
		t.rebuild()

	case keyCycleSort.Matches(k):
		t.sort = (t.sort + 1) % 4
		t.rebuild()

	case keyMark.Matches(k):
		at := t.markReviewed()
		// The Branch tab flags commits made after the mark, so it has to
		// hear about it too.
		return t, func() tea.Msg { return markMsg{At: at} }

	case keySinceMark.Matches(k):
		t.sinceMark = !t.sinceMark
		t.rebuild()

	case keyDiscard.Matches(k):
		if row, ok := t.acc.currentRow(); ok {
			t.confirm = row.key
		}

	case keyOpen.Matches(k):
		return t, t.openCmd()
	}
	return t, nil
}

func (t *ChangesTab) handleFilterKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	switch k.Type {
	case tea.KeyEnter:
		t.filtering = false
		t.input.Blur()
		return t, nil
	case tea.KeyEsc:
		t.filtering = false
		t.input.Blur()
		t.pathQuery = ""
		t.rebuild()
		return t, nil
	}

	var cmd tea.Cmd
	t.input, cmd = t.input.Update(k)
	t.pathQuery = t.input.Value()
	t.rebuild()
	return t, cmd
}

// markReviewed snapshots the current diffs, so the since-mark filter can show
// what an agent touched after this point.
func (t *ChangesTab) markReviewed() time.Time {
	t.marked = make(map[string]string, len(t.files))
	for _, f := range t.files {
		t.marked[f.Path] = f.Diff
	}
	t.message = fmt.Sprintf("marked %d files reviewed", len(t.files))
	return t.now()
}

func (t *ChangesTab) diffOf(path string) string {
	for _, f := range t.files {
		if f.Path == path {
			return f.Diff
		}
	}
	return ""
}

func (t *ChangesTab) confirmStatus() git.Status {
	for _, f := range t.files {
		if f.Path == t.confirm {
			return f.Status
		}
	}
	return git.StatusChanged
}

// openCmd launches $EDITOR on the file under the cursor, at the diff line the
// cursor is pointing at when it is inside a hunk.
func (t *ChangesTab) openCmd() tea.Cmd {
	row, ok := t.acc.currentRow()
	if !ok {
		return nil
	}

	line := 0
	if depth := t.acc.cursorDepth(); depth > 0 {
		line = diffLineNumber(t.raw[row.key], depth-1)
	}
	return openInEditor(t.repo.Root, row.key, line)
}

// discardCmd reverts one file.
func discardCmd(repo git.Repo, path string, status git.Status) tea.Cmd {
	return func() tea.Msg {
		return discardedMsg{Path: path, Err: repo.DiscardFile(path, status)}
	}
}

// listHeight is how many rows the accordion gets, once the summary line and
// any footer are taken out.
func (t *ChangesTab) listHeight() int {
	h := t.height - 1 // summary
	if t.footer() != "" {
		h--
	}
	return max(0, h)
}

// summary is the top line: what is listed, and how it is filtered.
func (t *ChangesTab) summary() string {
	var added, deleted int
	for _, f := range t.files {
		added += f.Additions
		deleted += f.Deletions
	}

	shown := len(t.visibleFiles())
	parts := []string{
		fmt.Sprintf("%d/%d files", shown, len(t.files)),
		fmt.Sprintf("+%d -%d", added, deleted),
		"filter " + t.status.String(),
		"sort " + t.sort.String(),
	}
	if t.pathQuery != "" {
		parts = append(parts, "/"+t.pathQuery)
	}
	if t.sinceMark {
		parts = append(parts, "since mark")
	}
	return styleDim.Render(strings.Join(parts, " · "))
}

// footer is the confirmation question, the filter input, or a transient
// message, in that order of precedence.
func (t *ChangesTab) footer() string {
	switch {
	case t.confirm != "":
		return styleAlertText.Render("discard "+t.confirm+"? ") + styleDim.Render("y / n")
	case t.filtering:
		return t.input.View()
	case t.message != "":
		return styleDim.Render(t.message)
	}
	return ""
}

func (t *ChangesTab) View(width, height int) string {
	t.width, t.height = width, height

	listHeight := t.listHeight()
	t.acc = t.acc.scrollInto(listHeight)

	rows := []string{t.summary(), t.acc.view(width, listHeight, t.emptyText())}
	if f := t.footer(); f != "" {
		rows = append(rows, f)
	}
	return strings.Join(rows, "\n")
}

func (t *ChangesTab) emptyText() string {
	switch {
	case len(t.files) == 0:
		return "no changes against HEAD"
	case t.sinceMark:
		return "nothing has changed since the mark"
	default:
		return "nothing matches this filter"
	}
}
