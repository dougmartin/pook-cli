// Package claude reads Claude Code's session transcripts for a workspace.
// It is a port of src/claude.ts.
package claude

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// maxMessages is how many messages a tail keeps; older ones are dropped
	// from the front.
	maxMessages = 500

	// maxTextChars caps a rendered message, and maxThinkingChars the dimmer
	// thinking blocks.
	maxTextChars     = 4000
	maxThinkingChars = 1500
	maxSummaryChars  = 160
)

// Kind is what produced a message.
type Kind string

const (
	KindUser      Kind = "user"
	KindAssistant Kind = "assistant"
	KindThinking  Kind = "thinking"
	KindTool      Kind = "tool"
)

// Message is one renderable entry in the session view.
type Message struct {
	Time time.Time
	Kind Kind
	Text string
	// Tool is the tool name, set only when Kind is KindTool.
	Tool string
}

// nonAlphanumeric is every character Claude Code replaces with a dash when it
// names a project's transcript folder.
var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

// TranscriptDir is ~/.claude/projects/<sanitized-root> for a workspace.
func TranscriptDir(root string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects", nonAlphanumeric.ReplaceAllString(root, "-"))
}

// FindLatestSession is the most recently modified transcript for a workspace,
// or empty when there is none.
func FindLatestSession(root string) string {
	dir := TranscriptDir(root)
	if dir == "" {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var best string
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = filepath.Join(dir, e.Name()), info.ModTime()
		}
	}
	return best
}

// truncate caps a string for display.
//
// The original counted UTF-16 units; this counts runes, so a cut can never
// land inside a character and produce invalid UTF-8.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + " …"
}

var (
	systemReminderRe = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
	ideBlockRe       = regexp.MustCompile(`(?s)<ide_[a-z_]+>.*?</ide_[a-z_]+>`)
)

// cleanUserText strips the blocks Claude Code embeds in user message text,
// which are context for the model rather than anything the user typed.
func cleanUserText(text string) string {
	text = systemReminderRe.ReplaceAllString(text, "")
	text = ideBlockRe.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}
