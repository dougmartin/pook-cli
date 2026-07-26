package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/dougmartin/pook-cli/internal/claude"
	"github.com/dougmartin/pook-cli/internal/clip"
)

// sessionMsg delivers whatever the transcript reader found.
type sessionMsg struct {
	Tail     *claude.SessionTail
	Messages []claude.Message
	Title    string
	// StartIndex is how many messages have been dropped off the front, so a
	// position can be reported against the whole session.
	StartIndex int
	// Switched is set when this is a different session file than before.
	Switched bool
}

// copiedMsg reports the result of a copy to the clipboard.
type copiedMsg struct {
	What string
	Err  error
}

// SessionTab is a live view of the most recent Claude Code chat for this
// folder, one message at a time.
type SessionTab struct {
	root string

	tail     *claude.SessionTail
	messages []claude.Message
	title    string
	start    int

	// cursor indexes messages. following means it tracks the newest message
	// as the session grows; moving backwards anchors on a historical one and
	// stops following.
	cursor    int
	following bool

	// rendered caches glamour output, which is far too slow to run per frame.
	// A resize reflows everything, so the cache is dropped on a width change.
	rendered map[int]string

	polling bool
	message string
	badge   Badge

	now           func() time.Time
	width, height int
}

func NewSessionTab(root string) *SessionTab {
	return &SessionTab{
		root:      root,
		following: true,
		rendered:  map[int]string{},
		now:       time.Now,
	}
}

func (t *SessionTab) Title() string        { return "Session" }
func (t *SessionTab) Badge() Badge         { return t.badge }
func (t *SessionTab) CapturingInput() bool { return false }

func (t *SessionTab) Bindings() []Binding {
	return []Binding{keyPrevMsg, keyNextMsg, keyPrevUser, keyNextUser, keyJumpLive, keyCopyMsg}
}

func (t *SessionTab) Focus() Tab {
	t.badge.Dot = false
	return t
}

// Session bindings.
var (
	// The arrows are not bound here: left and right cycle tabs globally, so
	// message navigation is h and l only.
	keyPrevMsg  = Binding{Keys: []string{"h"}, Label: "h", Help: "previous message"}
	keyNextMsg  = Binding{Keys: []string{"l"}, Label: "l", Help: "next message"}
	keyPrevUser = Binding{Keys: []string{"H"}, Label: "H", Help: "previous user message"}
	keyNextUser = Binding{Keys: []string{"L"}, Label: "L", Help: "next user message"}
	keyJumpLive = Binding{Keys: []string{"$"}, Label: "$", Help: "jump to live"}
	keyCopyMsg  = Binding{Keys: []string{"y"}, Label: "y", Help: "copy raw message"}
)

func (t *SessionTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width != t.width {
			// A reflow invalidates every cached render.
			t.rendered = map[int]string{}
		}
		t.width, t.height = msg.Width, msg.Height

	case ActivityMsg:
		t.badge.Dot = true

	case RefreshMsg:
		// The transcript is watched, so a refresh is also the moment to read
		// whatever the agent has said since.
		return t, t.pollCmd()

	case sessionMsg:
		return t, t.applySession(msg)

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

// pollCmd reads whatever has been appended to the newest transcript, and
// switches session when a new one starts.
func (t *SessionTab) pollCmd() tea.Cmd {
	if t.polling {
		return nil
	}
	t.polling = true

	root, tail := t.root, t.tail
	return func() tea.Msg {
		file := claude.FindLatestSession(root)
		if file == "" {
			return sessionMsg{}
		}

		switched := tail == nil || tail.File != file
		if switched {
			tail = claude.NewSessionTail(file)
		}
		tail.ReadNew()

		return sessionMsg{
			Tail:       tail,
			Messages:   append([]claude.Message(nil), tail.Messages...),
			Title:      tail.Title,
			StartIndex: tail.StartIndex,
			Switched:   switched,
		}
	}
}

// applySession folds a poll into the view.
func (t *SessionTab) applySession(msg sessionMsg) tea.Cmd {
	t.polling = false
	if msg.Tail == nil {
		return nil
	}

	grew := len(msg.Messages) > len(t.messages)

	// How many messages fell off the front, computed before the new start
	// index replaces the old one.
	dropped := msg.StartIndex - t.start

	t.tail = msg.Tail
	t.title = msg.Title
	t.start = msg.StartIndex

	if msg.Switched {
		// A new session starts at its live edge, whatever the old position
		// was pointing at.
		t.messages = msg.Messages
		t.rendered = map[int]string{}
		t.following = true
		t.cursor = max(0, len(t.messages)-1)
		return nil
	}

	// Messages dropped off the front shift every index, the cursor's included.
	if dropped > 0 {
		t.cursor = max(0, t.cursor-dropped)
	}
	t.messages = msg.Messages

	if t.following {
		t.cursor = max(0, len(t.messages)-1)
	}
	if grew && !t.following {
		// New messages arrived while the user was reading history.
		t.badge.Dot = true
	}
	return nil
}

func (t *SessionTab) handleKey(k tea.KeyMsg) (Tab, tea.Cmd) {
	switch {
	case keyPrevMsg.Matches(k):
		t.moveTo(t.cursor - 1)
	case keyNextMsg.Matches(k):
		t.moveTo(t.cursor + 1)
	case keyPrevUser.Matches(k):
		if i, ok := t.findUser(-1); ok {
			t.moveTo(i)
		}
	case keyNextUser.Matches(k):
		if i, ok := t.findUser(1); ok {
			t.moveTo(i)
		}
	case keyJumpLive.Matches(k):
		t.cursor = max(0, len(t.messages)-1)
		t.following = true
	case keyCopyMsg.Matches(k):
		return t, t.copyCmd()
	}
	return t, nil
}

// moveTo positions the cursor, and decides whether the view is still live.
func (t *SessionTab) moveTo(i int) {
	if len(t.messages) == 0 {
		return
	}
	t.cursor = min(max(i, 0), len(t.messages)-1)

	// Only sitting on the newest message counts as following.
	t.following = t.cursor == len(t.messages)-1
}

// findUser is the next user message in a direction, if there is one.
func (t *SessionTab) findUser(dir int) (int, bool) {
	for i := t.cursor + dir; i >= 0 && i < len(t.messages); i += dir {
		if t.messages[i].Kind == claude.KindUser {
			return i, true
		}
	}
	return 0, false
}

// hasUser reports whether a jump in this direction would go anywhere, which is
// what greys out the hint.
func (t *SessionTab) hasUser(dir int) bool {
	_, ok := t.findUser(dir)
	return ok
}

func (t *SessionTab) current() (claude.Message, bool) {
	if t.cursor < 0 || t.cursor >= len(t.messages) {
		return claude.Message{}, false
	}
	return t.messages[t.cursor], true
}

// copyCmd puts the raw text of the current message on the clipboard.
func (t *SessionTab) copyCmd() tea.Cmd {
	m, ok := t.current()
	if !ok {
		return nil
	}

	text := m.Text
	if m.Kind == claude.KindTool {
		text = m.Tool + " " + m.Text
	}
	return func() tea.Msg {
		return copiedMsg{What: "message", Err: clip.Write(text)}
	}
}

// body renders the current message, from cache where possible.
func (t *SessionTab) body(width, height int) string {
	m, ok := t.current()
	if !ok {
		return styleDim.Render("no messages in this session yet")
	}

	switch m.Kind {
	case claude.KindTool:
		return styleTool.Render(m.Tool) + "\n" + styleDim.Render(m.Text)
	case claude.KindThinking:
		return styleDim.Render(wrapPlain(m.Text, width))
	}

	if cached, hit := t.rendered[t.cursor]; hit {
		return cached
	}
	out := renderMarkdown(m.Text, width)
	t.rendered[t.cursor] = out
	return out
}

// renderMarkdown runs glamour over message text, falling back to the raw text
// when it cannot.
func renderMarkdown(text string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("notty"),
		glamour.WithWordWrap(max(20, width)),
	)
	if err != nil {
		return wrapPlain(text, width)
	}

	out, err := r.Render(text)
	if err != nil {
		return wrapPlain(text, width)
	}
	return strings.Trim(out, "\n")
}

// wrapPlain is a last-resort word wrap.
func wrapPlain(text string, width int) string {
	width = max(20, width)

	var out []string
	for _, line := range strings.Split(text, "\n") {
		for len([]rune(line)) > width {
			runes := []rune(line)
			cut := width
			for i := width; i > width/2; i-- {
				if runes[i] == ' ' {
					cut = i
					break
				}
			}
			out = append(out, string(runes[:cut]))
			line = strings.TrimPrefix(string(runes[cut:]), " ")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// header names the position in the session and whether the view is live.
func (t *SessionTab) header() string {
	if len(t.messages) == 0 {
		return styleDim.Render("no session found for this folder")
	}

	m, _ := t.current()
	parts := []string{
		fmt.Sprintf("%d/%d", t.start+t.cursor+1, t.start+len(t.messages)),
		string(m.Kind),
	}
	if !m.Time.IsZero() {
		parts = append(parts, formatAgo(t.now().Sub(m.Time))+" ago")
	}
	if t.title != "" {
		parts = append(parts, t.title)
	}

	line := styleDim.Render(strings.Join(parts, " · "))
	if t.following {
		line += " " + styleLive.Render(" live ")
	}
	return line
}

// scrubber is a bar showing where the cursor sits in the session.
func (t *SessionTab) scrubber(width int) string {
	if len(t.messages) <= 1 || width < 8 {
		return ""
	}

	inner := max(1, width-2)
	pos := t.cursor * (inner - 1) / max(1, len(t.messages)-1)

	var b strings.Builder
	b.WriteString(styleDim.Render("["))
	for i := range inner {
		switch {
		case i == pos:
			b.WriteString(styleSelected.Render("●"))
		case i < pos:
			b.WriteString(styleHunk.Render("─"))
		default:
			b.WriteString(styleDim.Render("─"))
		}
	}
	b.WriteString(styleDim.Render("]"))
	return b.String()
}

// footer shows the jumps that are available from here.
func (t *SessionTab) footer() string {
	if t.message != "" {
		return styleDim.Render(t.message)
	}

	prev, next := styleDim, styleDim
	if t.hasUser(-1) {
		prev = styleKey
	}
	if t.hasUser(1) {
		next = styleKey
	}
	return prev.Render("H prev user") + styleDim.Render("  ") + next.Render("L next user")
}

func (t *SessionTab) View(width, height int) string {
	t.width, t.height = width, height

	head := t.header()
	scrub := t.scrubber(width)
	foot := t.footer()

	used := 1 + 1 // header and footer
	if scrub != "" {
		used++
	}
	bodyHeight := max(0, height-used)

	rows := []string{head}
	if scrub != "" {
		rows = append(rows, scrub)
	}
	rows = append(rows, fitBlock(t.body(width, bodyHeight), width, bodyHeight), foot)
	return strings.Join(rows, "\n")
}
