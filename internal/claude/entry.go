package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// entry is one line of a .jsonl transcript. Only the fields the session view
// needs are decoded; the format carries a great deal more.
type entry struct {
	Type        string `json:"type"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	AITitle     string `json:"aiTitle"`
	Message     struct {
		// Content is a string on some user entries and an array of blocks
		// everywhere else.
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// block is one content block within a message.
type block struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
}

// extractMessages turns one transcript entry into the messages it renders as.
// A sidechain entry belongs to a subagent, not this conversation.
func extractMessages(e entry) []Message {
	if e.IsSidechain {
		return nil
	}
	at := parseTime(e.Timestamp)

	switch e.Type {
	case "user":
		return userMessages(e.Message.Content, at)
	case "assistant":
		return assistantMessages(e.Message.Content, at)
	default:
		return nil
	}
}

func userMessages(content json.RawMessage, at time.Time) []Message {
	blocks := contentBlocks(content)

	var out []Message
	for _, b := range blocks {
		if b.Type != "text" {
			continue
		}
		text := cleanUserText(b.Text)
		// Skip pure command and meta payloads, which are markup rather than
		// anything the user typed.
		if text == "" || strings.HasPrefix(text, "<") {
			continue
		}
		out = append(out, Message{Time: at, Kind: KindUser, Text: truncate(text, maxTextChars)})
	}
	return out
}

func assistantMessages(content json.RawMessage, at time.Time) []Message {
	var blocks []block
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}

	var out []Message
	for _, b := range blocks {
		switch {
		case b.Type == "text" && strings.TrimSpace(b.Text) != "":
			out = append(out, Message{
				Time: at, Kind: KindAssistant,
				Text: truncate(strings.TrimSpace(b.Text), maxTextChars),
			})
		case b.Type == "thinking" && strings.TrimSpace(b.Thinking) != "":
			out = append(out, Message{
				Time: at, Kind: KindThinking,
				Text: truncate(strings.TrimSpace(b.Thinking), maxThinkingChars),
			})
		case b.Type == "tool_use":
			name := b.Name
			if name == "" {
				name = "tool"
			}
			out = append(out, Message{
				Time: at, Kind: KindTool,
				Tool: name,
				Text: summarizeToolInput(b.Input),
			})
		}
	}
	return out
}

// contentBlocks reads a message body, which is a bare string on some user
// entries and an array of blocks everywhere else.
func contentBlocks(content json.RawMessage) []block {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return []block{{Type: "text", Text: text}}
	}
	var blocks []block
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	return blocks
}

// summaryKeys are the tool input fields worth showing, most descriptive first.
var summaryKeys = []string{
	"description", "command", "file_path", "path", "pattern",
	"query", "prompt", "skill", "url", "question",
}

// summarizeToolInput reduces a tool call's input to the one line the session
// view shows next to the tool name.
func summarizeToolInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(input, &obj) != nil {
		return ""
	}

	for _, key := range summaryKeys {
		if s, ok := stringField(obj[key]); ok && strings.TrimSpace(s) != "" {
			return truncate(s, maxSummaryChars)
		}
	}

	// An AskUserQuestion-shaped input: the first question stands in for it.
	if raw, ok := obj["questions"]; ok {
		var questions []struct {
			Question string `json:"question"`
		}
		if json.Unmarshal(raw, &questions) == nil && len(questions) > 0 && questions[0].Question != "" {
			return truncate(questions[0].Question, maxSummaryChars)
		}
	}

	// Otherwise name the first field holding a list, which at least says how
	// much work the call is doing. Key order is read from the raw JSON: the
	// original relied on JS object insertion order, which a Go map loses.
	for _, key := range objectKeys(input) {
		var list []json.RawMessage
		if json.Unmarshal(obj[key], &list) == nil && len(list) > 0 {
			return fmt.Sprintf("%s × %d", key, len(list))
		}
	}
	return ""
}

// stringField reports whether raw is a JSON string, and its value.
func stringField(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// objectKeys lists a JSON object's keys in the order they appear in the text.
func objectKeys(raw json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))

	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil
	}

	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return keys
		}
		key, ok := tok.(string)
		if !ok {
			return keys
		}
		keys = append(keys, key)

		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return keys
		}
	}
	return keys
}

// parseTime reads an entry timestamp, returning the zero time when it is
// missing or unparseable.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
