package claude

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SessionTail is an incremental reader for one transcript. ReadNew consumes
// only the bytes appended since the last call, which is what makes following a
// live session cheap no matter how long it runs.
type SessionTail struct {
	// File is the transcript path, and ID its session id.
	File string
	ID   string

	// Title is the session's AI-generated title, once the transcript names one.
	Title string

	// Messages is the tail of the conversation, capped at 500.
	Messages []Message

	// StartIndex is how many messages have been dropped off the front, so a
	// position in Messages can still be reported against the whole session.
	StartIndex int

	offset  int64
	partial string
}

// NewSessionTail opens a transcript without reading it yet.
func NewSessionTail(file string) *SessionTail {
	return &SessionTail{
		File: file,
		ID:   strings.TrimSuffix(filepath.Base(file), ".jsonl"),
	}
}

// ReadNew consumes whatever has been appended since the last call and reports
// whether anything was added.
func (s *SessionTail) ReadNew() bool {
	info, err := os.Stat(s.File)
	if err != nil {
		return false
	}
	size := info.Size()

	// A shorter file is a different file: truncated, or replaced by a new
	// session reusing the name.
	if size < s.offset {
		s.offset = 0
		s.partial = ""
		s.Messages = nil
		s.StartIndex = 0
	}
	if size == s.offset {
		return false
	}

	chunk, err := s.readFrom(s.offset, size)
	if err != nil {
		return false
	}
	s.offset = size

	// The final line may be half written, so it is held back until the rest
	// of it arrives.
	lines := strings.Split(s.partial+chunk, "\n")
	s.partial = lines[len(lines)-1]
	lines = lines[:len(lines)-1]

	added := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e entry
		if json.Unmarshal([]byte(line), &e) != nil {
			continue // a malformed line is skipped, not fatal
		}
		if e.Type == "ai-title" && e.AITitle != "" {
			s.Title = e.AITitle
			added = true
			continue
		}
		for _, m := range extractMessages(e) {
			s.Messages = append(s.Messages, m)
			added = true
		}
	}

	if over := len(s.Messages) - maxMessages; over > 0 {
		s.StartIndex += over
		s.Messages = append([]Message(nil), s.Messages[over:]...)
	}
	return added
}

// readFrom returns the bytes in [start, size).
func (s *SessionTail) readFrom(start, size int64) (string, error) {
	f, err := os.Open(s.File)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	buf := make([]byte, size-start)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}
	return string(buf[:n]), nil
}
