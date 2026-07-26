package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/dougmartin/pook-cli/internal/prompts"
)

// promptsModel is a shell whose Prompts tab holds a small library in a
// throwaway directory.
func promptsModel(t *testing.T) Model {
	t.Helper()

	store := prompts.NewStore(filepath.Join(t.TempDir(), "prompts.json"))
	mustAddPrompt(t, store, "Review a file", "Review {{file}} for bugs, focusing on {{concern}}.")
	mustAddPrompt(t, store, "Explain", "Explain this code line by line.")
	mustAddPrompt(t, store, "Write a test", "Add a table test for {{file}}.")

	m := New(gitRepoForTest(), nil, nil, store).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})
	promptsTab(m).Load()
	return press(m, "4")
}

func promptsTab(m Model) *PromptsTab { return m.tabs[TabPrompts].(*PromptsTab) }

func mustAddPrompt(t *testing.T, s *prompts.Store, title, text string) {
	t.Helper()
	if _, err := s.Add(title, text); err != nil {
		t.Fatal(err)
	}
}

func promptTitles(t *PromptsTab) []string {
	out := make([]string, len(t.visible))
	for i, p := range t.visible {
		out[i] = p.Title
	}
	return out
}

func TestFramePromptsList(t *testing.T) {
	frame := promptsModel(t).View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFramePromptsTokenPrompt(t *testing.T) {
	frame := press(promptsModel(t), "enter").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFramePromptsEditor(t *testing.T) {
	frame := press(promptsModel(t), "e").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestPromptSearch(t *testing.T) {
	m := press(promptsModel(t), "/")
	if !promptsTab(m).CapturingInput() {
		t.Fatal("search did not take focus")
	}

	m = press(m, "t", "e", "s", "t")
	if got := promptTitles(promptsTab(m)); !equalStrings(got, []string{"Write a test"}) {
		t.Errorf("search found %v, want [Write a test]", got)
	}

	// Search covers the body too, not only the title.
	m = press(m, "esc")
	m = press(m, "/", "l", "i", "n", "e")
	if got := promptTitles(promptsTab(m)); !equalStrings(got, []string{"Explain"}) {
		t.Errorf("body search found %v, want [Explain]", got)
	}
}

// Copying a prompt with placeholders asks for each distinct one, in order of
// first appearance.
func TestCopyAsksForEveryPlaceholder(t *testing.T) {
	m := press(promptsModel(t), "enter")
	tab := promptsTab(m)

	if tab.mode != promptTokens {
		t.Fatal("a prompt with placeholders copied without asking")
	}
	if want := []string{"file", "concern"}; !equalStrings(tab.pending.tokens, want) {
		t.Fatalf("tokens = %v, want %v", tab.pending.tokens, want)
	}
	if !strings.Contains(m.View(), "file (1/2)") {
		t.Errorf("the first placeholder is not being asked for:\n%s", m.View())
	}

	m = press(m, "a", ".", "g", "o", "enter")
	tab = promptsTab(m)
	if tab.pending.index != 1 {
		t.Fatalf("index = %d, want the second placeholder", tab.pending.index)
	}
	if !strings.Contains(m.View(), "concern (2/2)") {
		t.Errorf("the second placeholder is not being asked for:\n%s", m.View())
	}

	// Answering the last one completes the copy.
	next, cmd := tab.Update(key("enter"))
	tab = next.(*PromptsTab)
	if tab.mode != promptList {
		t.Error("the copy did not finish")
	}
	if cmd == nil {
		t.Fatal("no copy command was produced")
	}
}

// Cancelling any placeholder aborts the copy and leaves the clipboard
// untouched.
func TestCancellingAPlaceholderAbortsTheCopy(t *testing.T) {
	m := press(promptsModel(t), "enter")
	m = press(m, "x", "enter") // answer the first

	tab := promptsTab(m)
	if tab.pending.index != 1 {
		t.Fatalf("index = %d, want to be on the second placeholder", tab.pending.index)
	}

	next, cmd := tab.Update(key("esc"))
	tab = next.(*PromptsTab)

	if cmd != nil {
		t.Error("cancelling still produced a copy command")
	}
	if tab.mode != promptList {
		t.Error("cancelling did not return to the list")
	}
	if len(tab.pending.tokens) != 0 {
		t.Error("the abandoned copy was not cleared")
	}
	if !strings.Contains(tab.message, "cancelled") {
		t.Errorf("message = %q, want it to say the copy was cancelled", tab.message)
	}
}

// A prompt with no placeholders copies straight away.
func TestCopyWithoutPlaceholders(t *testing.T) {
	m := press(promptsModel(t), "j") // Explain has none
	tab := promptsTab(m)

	next, cmd := tab.Update(key("enter"))
	tab = next.(*PromptsTab)

	if tab.mode != promptList {
		t.Error("a prompt with no placeholders asked for something")
	}
	if cmd == nil {
		t.Fatal("no copy command was produced")
	}
}

func TestPlaceholderValuesAreSubstituted(t *testing.T) {
	// The substitution itself is the prompts package's, but the tab has to
	// collect the values in the right order and hand them over.
	m := press(promptsModel(t), "enter")
	m = press(m, "a", ".", "g", "o", "enter")
	m = press(m, "b", "u", "g", "s", "enter")

	tab := promptsTab(m)
	if tab.mode != promptList {
		t.Fatal("the copy did not complete")
	}

	filled := prompts.FillTokens(
		"Review {{file}} for bugs, focusing on {{concern}}.",
		map[string]string{"file": "a.go", "concern": "bugs"},
	)
	if want := "Review a.go for bugs, focusing on bugs."; filled != want {
		t.Errorf("fill = %q, want %q", filled, want)
	}
}

func TestEditInPlace(t *testing.T) {
	m := press(promptsModel(t), "e")
	tab := promptsTab(m)

	if tab.mode != promptEdit {
		t.Fatal("e did not open the editor")
	}
	if got := tab.title.Value(); got != "Review a file" {
		t.Errorf("title = %q, want the current one", got)
	}

	// Tab moves to the body, so a multiline prompt can hold anything.
	m = press(m, "tab")
	if promptsTab(m).editingTitle {
		t.Error("tab did not move to the body")
	}

	m = press(m, "!", "ctrl+s")
	tab = promptsTab(m)
	if tab.mode != promptList {
		t.Fatal("ctrl-s did not save and close")
	}
	if !strings.HasSuffix(tab.store.Prompts[0].Text, "!") {
		t.Errorf("the edit was not saved: %q", tab.store.Prompts[0].Text)
	}
}

func TestEditCancelled(t *testing.T) {
	m := press(promptsModel(t), "e")
	before := promptsTab(m).store.Prompts[0].Title

	m = press(m, "x", "esc")
	tab := promptsTab(m)

	if tab.mode != promptList {
		t.Fatal("esc did not close the editor")
	}
	if tab.store.Prompts[0].Title != before {
		t.Errorf("title = %q, want it unchanged at %q", tab.store.Prompts[0].Title, before)
	}
}

func TestNewPrompt(t *testing.T) {
	m := press(promptsModel(t), "n")
	tab := promptsTab(m)

	if tab.mode != promptEdit || tab.editing != "" {
		t.Fatal("n did not open a blank editor")
	}
	if tab.title.Value() != "" {
		t.Errorf("title = %q, want empty", tab.title.Value())
	}

	m = press(m, "F", "r", "e", "s", "h", "ctrl+s")
	tab = promptsTab(m)

	if len(tab.store.Prompts) != 4 {
		t.Fatalf("library holds %d prompts, want 4", len(tab.store.Prompts))
	}
	if got := tab.store.Prompts[3].Title; got != "Fresh" {
		t.Errorf("new title = %q, want Fresh", got)
	}
}

func TestDeleteIsConfirmed(t *testing.T) {
	m := press(promptsModel(t), "d")
	if promptsTab(m).mode != promptConfirmDelete {
		t.Fatal("d did not ask for confirmation")
	}
	if !strings.Contains(m.View(), "delete Review a file?") {
		t.Errorf("the confirmation is not on screen:\n%s", m.View())
	}

	m = press(m, "n")
	if len(promptsTab(m).store.Prompts) != 3 {
		t.Error("n deleted the prompt anyway")
	}

	m = press(m, "d", "y")
	tab := promptsTab(m)
	if len(tab.store.Prompts) != 2 {
		t.Fatalf("library holds %d prompts, want 2", len(tab.store.Prompts))
	}
	if tab.store.Prompts[0].Title == "Review a file" {
		t.Error("the wrong prompt was deleted")
	}
}

func TestReorder(t *testing.T) {
	m := promptsModel(t)

	m = press(m, "J")
	if got := promptTitles(promptsTab(m)); !equalStrings(got, []string{"Explain", "Review a file", "Write a test"}) {
		t.Errorf("after J: %v", got)
	}

	m = press(m, "K")
	if got := promptTitles(promptsTab(m)); !equalStrings(got, []string{"Review a file", "Explain", "Write a test"}) {
		t.Errorf("after K: %v", got)
	}

	// The order is on disk, not only on screen.
	reopened := prompts.NewStore(promptsTab(m).store.Path())
	reopened.Load()
	if got := reopened.Prompts[0].Title; got != "Review a file" {
		t.Errorf("saved order starts with %q", got)
	}
}

// Import and export round-trip through a real file.
func TestImportAndExport(t *testing.T) {
	m := promptsModel(t)
	path := filepath.Join(t.TempDir(), "prompts.md")

	m = press(m, "x")
	if promptsTab(m).mode != promptExport {
		t.Fatal("x did not ask for a path")
	}
	m = typeString(m, path)
	m = press(m, "enter")

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nothing was exported: %v", err)
	}
	if !strings.Contains(promptsTab(m).message, "exported 3") {
		t.Errorf("message = %q", promptsTab(m).message)
	}

	// Importing it back appends every prompt again.
	m = press(m, "i")
	m = typeString(m, path)
	m = press(m, "enter")

	tab := promptsTab(m)
	if len(tab.store.Prompts) != 6 {
		t.Fatalf("library holds %d prompts, want 6 after re-importing 3", len(tab.store.Prompts))
	}
	if !strings.Contains(tab.message, "imported 3") {
		t.Errorf("message = %q", tab.message)
	}
}

func TestImportFromAMissingFileReports(t *testing.T) {
	m := press(promptsModel(t), "i")
	m = typeString(m, filepath.Join(t.TempDir(), "absent.md"))
	m = press(m, "enter")

	if got := promptsTab(m).message; !strings.Contains(got, "import failed") {
		t.Errorf("message = %q, want an import failure", got)
	}
}

// Another instance writing the library is picked up on the next refresh.
func TestLibraryFollowsTheFile(t *testing.T) {
	m := promptsModel(t)
	tab := promptsTab(m)

	other := prompts.NewStore(tab.store.Path())
	other.Load()
	mustAddPrompt(t, other, "From another window", "hello")

	m = apply(m, refreshedMsg{})
	if got := len(promptsTab(m).store.Prompts); got != 4 {
		t.Fatalf("library holds %d prompts, want 4 after another instance wrote it", got)
	}
	if got := promptTitles(promptsTab(m)); got[3] != "From another window" {
		t.Errorf("titles = %v", got)
	}
}

// While a field has focus, keys belong to it rather than the global keymap.
func TestPromptEditorOutranksGlobalKeys(t *testing.T) {
	m := press(promptsModel(t), "n")
	m = press(m, "a", "c", "q", "r")

	if m.overlay != nil || m.modal != nil {
		t.Fatal("typing in the editor opened a layer")
	}
	if got := promptsTab(m).title.Value(); got != "acqr" {
		t.Errorf("title = %q, want acqr", got)
	}
}

func TestEmptyLibrary(t *testing.T) {
	store := prompts.NewStore(filepath.Join(t.TempDir(), "prompts.json"))
	m := New(gitRepoForTest(), nil, nil, store).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})
	promptsTab(m).Load()
	m = press(m, "4")

	if !strings.Contains(m.View(), "press n to add one") {
		t.Errorf("an empty library does not say what to do:\n%s", m.View())
	}
}

// typeString sends each character of s as its own key.
func typeString(m Model, s string) Model {
	for _, r := range s {
		m = apply(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

// A prompt is prose, so a long line has to fold rather than run off the edge
// the way a diff line does.
func TestLongPromptsWrap(t *testing.T) {
	long := "This is a deliberately long single-line prompt that runs well past " +
		"the width of any sensible terminal pane and therefore has to be folded " +
		"across several lines rather than truncated at the edge."

	store := prompts.NewStore(filepath.Join(t.TempDir(), "prompts.json"))
	mustAddPrompt(t, store, "Long one", long)

	m := New(gitRepoForTest(), nil, nil, store).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})
	promptsTab(m).Load()
	m = press(m, "4", " ")

	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)

	// Every word survives, spread over more than one line.
	body := stripStyles(frame)
	for _, word := range []string{"deliberately", "terminal", "truncated"} {
		if !strings.Contains(body, word) {
			t.Errorf("the wrapped prompt lost %q:\n%s", word, frame)
		}
	}

	wrapped := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, strings.Repeat(" ", accordionPrefix+bodyIndent)) &&
			strings.TrimSpace(line) != "" {
			wrapped++
		}
	}
	if wrapped < 3 {
		t.Errorf("the prompt folded onto %d lines, want it wrapped:\n%s", wrapped, frame)
	}
}

// A resize reflows the wrapped bodies rather than leaving them at the old
// width.
func TestPromptWrappingFollowsAResize(t *testing.T) {
	long := strings.Repeat("word ", 60)

	store := prompts.NewStore(filepath.Join(t.TempDir(), "prompts.json"))
	mustAddPrompt(t, store, "Long one", long)

	m := New(gitRepoForTest(), nil, nil, store).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	promptsTab(m).Load()
	m = press(m, "4", " ")

	wide := len(promptsTab(m).acc.rows[0].body)

	m = apply(m, tea.WindowSizeMsg{Width: 40, Height: 30})
	narrow := len(promptsTab(m).acc.rows[0].body)

	if narrow <= wide {
		t.Errorf("body is %d lines at width 40 and %d at width 100, want more when narrower", narrow, wide)
	}
	requireFrameSize(t, m.View(), 40, 30)
}
