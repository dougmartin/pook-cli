package ui

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/clip"
	"github.com/dougmartin/pook-cli/internal/prompts"
)

// promptMode is what the tab is currently asking the user for. Only one of
// these is ever active, which is what keeps the key routing in this tab flat.
type promptMode int

const (
	promptList promptMode = iota
	promptSearch
	promptEdit
	promptTokens
	promptImport
	promptExport
	promptConfirmDelete
)

// PromptsTab is the reusable prompt library.
type PromptsTab struct {
	store *prompts.Store

	acc     accordion
	visible []prompts.Prompt

	mode  promptMode
	query string

	search textinput.Model
	title  textinput.Model
	body   textarea.Model
	path   textinput.Model
	value  textinput.Model

	// editing is the id being edited, empty when the edit is a new prompt.
	editing string
	// editingTitle is true while the title field has focus rather than the
	// body.
	editingTitle bool

	// pending is the copy waiting on placeholder values.
	pending pendingCopy

	message       string
	badge         Badge
	width, height int
}

// pendingCopy is a copy that cannot complete until every placeholder has a
// value.
type pendingCopy struct {
	text   string
	tokens []string
	index  int
	values map[string]string
}

func NewPromptsTab(store *prompts.Store) *PromptsTab {
	search := textinput.New()
	search.Prompt = "/"
	search.Placeholder = "search"

	title := textinput.New()
	title.Prompt = "title: "

	path := textinput.New()
	path.Prompt = "path: "

	value := textinput.New()

	body := textarea.New()
	body.Prompt = ""
	body.ShowLineNumbers = false

	return &PromptsTab{
		store:  store,
		acc:    newAccordion(),
		search: search,
		title:  title,
		body:   body,
		path:   path,
		value:  value,
	}
}

func (t *PromptsTab) Title() string { return "Prompts" }
func (t *PromptsTab) Badge() Badge  { return t.badge }

// CapturingInput is true whenever a field has focus, which is every mode but
// the plain list and the delete confirmation.
func (t *PromptsTab) CapturingInput() bool {
	switch t.mode {
	case promptList, promptConfirmDelete:
		return false
	default:
		return true
	}
}

func (t *PromptsTab) Bindings() []Binding {
	// The shared set rather than a hand-picked few: this tab drives the
	// accordion, so it answers to all of its keys, and listing them by hand
	// is how zR, zM, g and G went undocumented.
	return append(slices.Clone(accordionBindings),
		keySearchPrompts, keyCopyPrompt, keyEditPrompt, keyNewPrompt,
		keyDeletePrompt, keyMoveUp, keyMoveDown, keyImport, keyExport,
	)
}

var (
	// The shared keyFilter is labelled "filter"; here it searches bodies as
	// well as titles, which is worth saying.
	keySearchPrompts = Binding{Keys: []string{"/"}, Label: "/", Help: "search title or text"}

	keyCopyPrompt   = Binding{Keys: []string{"enter"}, Label: "enter", Help: "copy prompt"}
	keyEditPrompt   = Binding{Keys: []string{"e"}, Label: "e", Help: "edit"}
	keyNewPrompt    = Binding{Keys: []string{"n"}, Label: "n", Help: "new"}
	keyDeletePrompt = Binding{Keys: []string{"d"}, Label: "d", Help: "delete"}
	keyMoveDown     = Binding{Keys: []string{"J"}, Label: "J", Help: "move down"}
	keyMoveUp       = Binding{Keys: []string{"K"}, Label: "K", Help: "move up"}
	keyImport       = Binding{Keys: []string{"i"}, Label: "i", Help: "import markdown"}
	keyExport       = Binding{Keys: []string{"x"}, Label: "x", Help: "export markdown"}
	keySaveEdit     = Binding{Keys: []string{"ctrl+s"}, Label: "ctrl-s", Help: "save"}
)

func (t *PromptsTab) Focus() Tab {
	t.badge.Dot = false
	return t
}

func (t *PromptsTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		reflow := msg.Width != t.width
		t.width, t.height = msg.Width, msg.Height
		t.body.SetWidth(max(10, msg.Width-2))
		t.body.SetHeight(max(3, msg.Height-6))
		if reflow {
			// Prompt bodies are wrapped to the pane, so a resize rebuilds
			// them.
			t.rebuild()
		}

	case ActivityMsg:
		t.badge.Dot = true

	case RefreshMsg:
		// The library is shared by every running instance, so each one has to
		// follow the file rather than trust its own copy. The file is small
		// and the watcher only fires on real changes.
		if t.store != nil && t.store.ReloadIfChanged() {
			t.rebuild()
		}

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

// Load reads the library for the first time.
func (t *PromptsTab) Load() {
	if t.store == nil {
		return
	}
	t.store.Load()
	t.rebuild()
}

func (t *PromptsTab) all() []prompts.Prompt {
	if t.store == nil {
		return nil
	}
	return t.store.Prompts
}

func (t *PromptsTab) rebuild() {
	query := strings.ToLower(t.query)

	t.visible = nil
	for _, p := range t.all() {
		if query != "" &&
			!strings.Contains(strings.ToLower(p.Title), query) &&
			!strings.Contains(strings.ToLower(p.Text), query) {
			continue
		}
		t.visible = append(t.visible, p)
	}

	rows := make([]accordionRow, 0, len(t.visible))
	for _, p := range t.visible {
		rows = append(rows, accordionRow{
			key:    p.ID,
			header: promptHeader(p, &t.acc),
			body:   promptBody(p, t.bodyWidth()),
		})
	}
	t.acc = t.acc.setRows(rows)
	t.badge.Count = len(t.all())
}

func promptHeader(p prompts.Prompt, acc *accordion) func(bool) string {
	tokens := prompts.ExtractTokens(p.Text)
	note := ""
	if len(tokens) > 0 {
		note = fmt.Sprintf("%d placeholders", len(tokens))
		if len(tokens) == 1 {
			note = "1 placeholder"
		}
	}

	return func(selected bool) string {
		open := "▸"
		if acc.isExpanded(p.ID) {
			open = "▾"
		}

		style := styleText
		if selected {
			style = styleSelected
		}
		return fmt.Sprintf("%s %s %s", styleDim.Render(open), style.Render(p.Title), styleDim.Render(note))
	}
}

// bodyWidth is what a wrapped body line has to fit in: the pane less the
// accordion's cursor column and this tab's indent.
func (t *PromptsTab) bodyWidth() int {
	return max(20, t.width-accordionPrefix-bodyIndent)
}

// promptBody is the prompt's text, wrapped. A prompt is prose, so a long line
// has to fold rather than run off the edge the way a diff line does.
func promptBody(p prompts.Prompt, width int) []string {
	wrapped := wrapPlain(strings.TrimRight(p.Text, "\n"), width)

	lines := strings.Split(wrapped, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, styleContext.Render(strings.Repeat(" ", bodyIndent)+l))
	}
	return out
}

func (t *PromptsTab) current() (prompts.Prompt, bool) {
	row, ok := t.acc.currentRow()
	if !ok {
		return prompts.Prompt{}, false
	}
	for _, p := range t.visible {
		if p.ID == row.key {
			return p, true
		}
	}
	return prompts.Prompt{}, false
}

func (t *PromptsTab) handleKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	switch t.mode {
	case promptSearch:
		return t.handleSearchKey(k)
	case promptEdit:
		return t.handleEditKey(k)
	case promptTokens:
		return t.handleTokenKey(k)
	case promptImport, promptExport:
		return t.handlePathKey(k)
	case promptConfirmDelete:
		return t.handleDeleteKey(k)
	}
	return t.handleListKey(k)
}

func (t *PromptsTab) handleListKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	// enter is copy here, not expand. The shared accordion binds it alongside
	// space, so it has to be claimed before the accordion sees it; space
	// still expands.
	if keyCopyPrompt.Matches(k) {
		return t, t.startCopy()
	}

	if acc, claimed := t.acc.handleAccordionKey(k, t.listHeight()); claimed {
		t.acc = acc
		return t, nil
	}

	switch {
	case keySearchPrompts.Matches(k):
		t.mode = promptSearch
		t.search.SetValue(t.query)
		t.search.CursorEnd()
		return t, t.search.Focus()

	case keyClearInput.Matches(k):
		if t.query != "" {
			t.query = ""
			t.rebuild()
		}

	case keyEditPrompt.Matches(k):
		if p, ok := t.current(); ok {
			t.beginEdit(p)
			return t, t.title.Focus()
		}

	case keyNewPrompt.Matches(k):
		t.beginEdit(prompts.Prompt{})
		return t, t.title.Focus()

	case keyDeletePrompt.Matches(k):
		if _, ok := t.current(); ok {
			t.mode = promptConfirmDelete
		}

	case keyMoveDown.Matches(k):
		return t, t.move(1)

	case keyMoveUp.Matches(k):
		return t, t.move(-1)

	case keyImport.Matches(k):
		t.mode = promptImport
		t.path.SetValue("")
		return t, t.path.Focus()

	case keyExport.Matches(k):
		t.mode = promptExport
		t.path.SetValue("")
		return t, t.path.Focus()
	}
	return t, nil
}

func (t *PromptsTab) handleSearchKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	switch k.Type {
	case tea.KeyEnter:
		t.mode = promptList
		t.search.Blur()
		return t, nil
	case tea.KeyEsc:
		t.mode = promptList
		t.search.Blur()
		t.query = ""
		t.rebuild()
		return t, nil
	}

	// The arrows move the results while the query keeps focus, so you can
	// type and then pick without leaving the search.
	if acc, claimed := t.acc.handleNavKey(k, t.listHeight()); claimed {
		t.acc = acc
		return t, nil
	}

	var cmd tea.Cmd
	t.search, cmd = t.search.Update(k)
	t.query = t.search.Value()
	t.rebuild()
	return t, cmd
}

// beginEdit opens the editor over a prompt, or over a blank one for a new
// entry.
func (t *PromptsTab) beginEdit(p prompts.Prompt) {
	t.mode = promptEdit
	t.editing = p.ID
	t.editingTitle = true

	t.title.SetValue(p.Title)
	t.title.CursorEnd()
	t.body.SetValue(p.Text)
	t.body.Blur()
}

func (t *PromptsTab) handleEditKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	switch {
	case keySaveEdit.Matches(k):
		return t, t.saveEdit()

	case k.Type == tea.KeyEsc:
		t.mode = promptList
		t.title.Blur()
		t.body.Blur()
		t.message = "edit cancelled"
		return t, nil

	case k.Type == tea.KeyTab:
		// Tab moves between the two fields, so a multiline body can still
		// contain anything.
		t.editingTitle = !t.editingTitle
		if t.editingTitle {
			t.body.Blur()
			return t, t.title.Focus()
		}
		t.title.Blur()
		return t, t.body.Focus()
	}

	var cmd tea.Cmd
	if t.editingTitle {
		t.title, cmd = t.title.Update(k)
	} else {
		t.body, cmd = t.body.Update(k)
	}
	return t, cmd
}

func (t *PromptsTab) saveEdit() tea.Cmd {
	if t.store == nil {
		return nil
	}

	title, text := t.title.Value(), t.body.Value()

	var err error
	if t.editing == "" {
		_, err = t.store.Add(title, text)
	} else {
		err = t.store.Update(t.editing, title, text)
	}

	t.mode = promptList
	t.title.Blur()
	t.body.Blur()

	if err != nil {
		t.message = "save failed: " + err.Error()
	} else {
		t.message = "saved"
	}
	t.rebuild()
	return nil
}

func (t *PromptsTab) handleDeleteKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	switch {
	case keyConfirmYes.Matches(k):
		p, ok := t.current()
		t.mode = promptList
		if !ok || t.store == nil {
			return t, nil
		}
		if err := t.store.Remove(p.ID); err != nil {
			t.message = "delete failed: " + err.Error()
		} else {
			t.message = "deleted " + p.Title
		}
		t.rebuild()

	case keyConfirmNo.Matches(k):
		t.mode = promptList
	}
	return t, nil
}

func (t *PromptsTab) move(delta int) tea.Cmd {
	p, ok := t.current()
	if !ok || t.store == nil {
		return nil
	}
	if err := t.store.Move(p.ID, delta); err != nil {
		t.message = "reorder failed: " + err.Error()
	}
	t.rebuild()
	return nil
}

// startCopy copies a prompt, first collecting a value for every distinct
// placeholder it carries.
func (t *PromptsTab) startCopy() tea.Cmd {
	p, ok := t.current()
	if !ok {
		return nil
	}

	tokens := prompts.ExtractTokens(p.Text)
	if len(tokens) == 0 {
		return copyTextCmd(p.Text, p.Title)
	}

	t.mode = promptTokens
	t.pending = pendingCopy{text: p.Text, tokens: tokens, values: map[string]string{}}
	t.value.SetValue("")
	t.value.Prompt = t.tokenPrompt()
	return t.value.Focus()
}

func (t *PromptsTab) tokenPrompt() string {
	return fmt.Sprintf("%s (%d/%d): ",
		t.pending.tokens[t.pending.index], t.pending.index+1, len(t.pending.tokens))
}

func (t *PromptsTab) handleTokenKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		// Cancelling any input aborts the whole copy, leaving the clipboard
		// untouched.
		t.mode = promptList
		t.value.Blur()
		t.pending = pendingCopy{}
		t.message = "copy cancelled"
		return t, nil

	case tea.KeyEnter:
		name := t.pending.tokens[t.pending.index]
		t.pending.values[name] = t.value.Value()
		t.pending.index++

		if t.pending.index < len(t.pending.tokens) {
			t.value.SetValue("")
			t.value.Prompt = t.tokenPrompt()
			return t, nil
		}

		text := prompts.FillTokens(t.pending.text, t.pending.values)
		t.mode = promptList
		t.value.Blur()
		t.pending = pendingCopy{}
		return t, copyTextCmd(text, "prompt")
	}

	var cmd tea.Cmd
	t.value, cmd = t.value.Update(k)
	return t, cmd
}

func (t *PromptsTab) handlePathKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		t.mode = promptList
		t.path.Blur()
		return t, nil

	case tea.KeyEnter:
		path := strings.TrimSpace(t.path.Value())
		mode := t.mode
		t.mode = promptList
		t.path.Blur()

		if path == "" || t.store == nil {
			return t, nil
		}
		if mode == promptImport {
			t.importFrom(path)
		} else {
			t.exportTo(path)
		}
		return t, nil
	}

	var cmd tea.Cmd
	t.path, cmd = t.path.Update(k)
	return t, cmd
}

func (t *PromptsTab) importFrom(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.message = "import failed: " + err.Error()
		return
	}

	n, err := t.store.ImportMarkdown(string(raw))
	if err != nil {
		t.message = "import failed: " + err.Error()
		return
	}
	t.message = fmt.Sprintf("imported %d prompts", n)
	t.rebuild()
}

func (t *PromptsTab) exportTo(path string) {
	if err := t.store.ExportMarkdown(path); err != nil {
		t.message = "export failed: " + err.Error()
		return
	}
	t.message = fmt.Sprintf("exported %d prompts to %s", len(t.all()), path)
}

// copyTextCmd puts text on the clipboard, and opens the clipboard modal when
// the text still carries an inline [MARKER] to fill in.
func copyTextCmd(text, what string) tea.Cmd {
	copy := func() tea.Msg {
		return copiedMsg{What: what, Err: clip.Write(text)}
	}
	if !prompts.HasInlineMarker(text) {
		return copy
	}

	open := func() tea.Msg {
		return openClipboardMsg{Text: text, SelectMarker: true}
	}
	return tea.Batch(copy, open)
}

func (t *PromptsTab) listHeight() int {
	h := t.height - 1
	if t.footer() != "" {
		h--
	}
	return max(0, h)
}

func (t *PromptsTab) summary() string {
	parts := []string{fmt.Sprintf("%d/%d prompts", len(t.visible), len(t.all()))}
	if t.query != "" {
		parts = append(parts, "/"+t.query)
	}
	return styleDim.Render(strings.Join(parts, " · "))
}

func (t *PromptsTab) footer() string {
	switch t.mode {
	case promptSearch:
		return t.search.View()
	case promptTokens:
		return t.value.View()
	case promptImport:
		return styleTitle.Render("import from ") + t.path.View()
	case promptExport:
		return styleTitle.Render("export to ") + t.path.View()
	case promptConfirmDelete:
		p, _ := t.current()
		return styleAlertText.Render("delete "+p.Title+"? ") + styleDim.Render("y / n")
	}
	if t.message != "" {
		return styleDim.Render(t.message)
	}
	// Nothing to report, so the row earns its keep by naming the keys. The
	// search in particular is invisible otherwise.
	return promptHint()
}

// promptHint names the actions worth knowing, shortest first so a narrow
// pane keeps the most useful ones.
func promptHint() string {
	return styleDim.Render("/ search · enter copy · e edit · n new · zR/zM expand all")
}

func (t *PromptsTab) View(width, height int) string {
	t.width, t.height = width, height

	if t.mode == promptEdit {
		return t.editView(width, height)
	}

	listHeight := t.listHeight()
	t.acc = t.acc.scrollInto(listHeight)

	rows := []string{t.summary(), t.acc.view(width, listHeight, t.emptyText())}
	if f := t.footer(); f != "" {
		rows = append(rows, f)
	}
	return strings.Join(rows, "\n")
}

// editView is the in-place editor: a title field over a body field.
func (t *PromptsTab) editView(width, height int) string {
	heading := "editing"
	if t.editing == "" {
		heading = "new prompt"
	}

	t.body.SetWidth(max(10, width-2))
	t.body.SetHeight(max(1, height-5))

	focus := styleDim.Render("tab switches field · ctrl-s saves · esc cancels")

	return strings.Join([]string{
		styleTitle.Render(heading),
		t.title.View(),
		t.body.View(),
		focus,
	}, "\n")
}

func (t *PromptsTab) emptyText() string {
	if len(t.all()) == 0 {
		return "no prompts yet, press n to add one"
	}
	return "nothing matches this search"
}
