package prompts

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Store is the prompt library persisted as prompts.json. Every pook instance
// reads the same file, so the store follows the file rather than trusting its
// own copy.
type Store struct {
	file string
	// lastRead is the exact bytes of our last read or write, so a reload can
	// tell someone else's write from our own.
	lastRead string

	Prompts []Prompt

	// now and newID are injectable so tests get stable values.
	now   func() time.Time
	newID func() string
}

// DefaultPath is prompts.json in the user config dir, shared by every
// instance.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pook", "prompts.json"), nil
}

// NewStore opens the library at a path, without reading it yet.
func NewStore(file string) *Store {
	return &Store{file: file, now: time.Now, newID: newUUID}
}

// Path is the file backing the store.
func (s *Store) Path() string { return s.file }

// Load reads the library. A missing or unreadable file is an empty library,
// which is what the first run looks like.
func (s *Store) Load() {
	raw, err := os.ReadFile(s.file)
	if err != nil {
		s.Prompts = nil
		return
	}
	s.lastRead = string(raw)

	var parsed []Prompt
	if json.Unmarshal(raw, &parsed) != nil {
		s.Prompts = nil
		return
	}
	s.Prompts = parsed
}

// ReloadIfChanged re-reads the file when another instance wrote it, and
// reports whether the in-memory list changed.
func (s *Store) ReloadIfChanged() bool {
	raw, err := os.ReadFile(s.file)
	if err != nil {
		return false
	}
	if string(raw) == s.lastRead {
		return false // our own write, or a no-op touch
	}
	s.lastRead = string(raw)

	var parsed []Prompt
	if json.Unmarshal(raw, &parsed) != nil {
		// Mid-write or corrupt: keep what we have and let the next event
		// re-read it.
		return false
	}
	s.Prompts = parsed
	return true
}

// Add appends a prompt to the end of the library.
func (s *Store) Add(title, text string) (Prompt, error) {
	now := s.now().UnixMilli()
	p := Prompt{
		ID:        s.newID(),
		Title:     defaultTitle(title),
		Text:      text,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.Prompts = append(s.Prompts, p)
	return p, s.save()
}

// Update rewrites a prompt's title and text. An unknown id is a no-op.
func (s *Store) Update(id, title, text string) error {
	for i := range s.Prompts {
		if s.Prompts[i].ID != id {
			continue
		}
		s.Prompts[i].Title = defaultTitle(title)
		s.Prompts[i].Text = text
		s.Prompts[i].UpdatedAt = s.now().UnixMilli()
		return s.save()
	}
	return nil
}

// Remove deletes a prompt by id.
func (s *Store) Remove(id string) error {
	kept := make([]Prompt, 0, len(s.Prompts))
	for _, p := range s.Prompts {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	s.Prompts = kept
	return s.save()
}

// Reorder applies a full ordering by id. Ids not listed keep their relative
// order at the end.
func (s *Store) Reorder(ids []string) error {
	byID := make(map[string]Prompt, len(s.Prompts))
	for _, p := range s.Prompts {
		byID[p.ID] = p
	}

	next := make([]Prompt, 0, len(s.Prompts))
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			next = append(next, p)
			delete(byID, id)
		}
	}
	// Whatever was not named keeps its original relative order.
	for _, p := range s.Prompts {
		if _, unplaced := byID[p.ID]; unplaced {
			next = append(next, p)
		}
	}

	s.Prompts = next
	return s.save()
}

// Move shifts one prompt by delta positions, clamped to the ends. This is what
// the reorder keys drive, in place of the extension's drag and drop.
func (s *Store) Move(id string, delta int) error {
	from := -1
	for i, p := range s.Prompts {
		if p.ID == id {
			from = i
			break
		}
	}
	if from < 0 {
		return nil
	}

	to := min(max(from+delta, 0), len(s.Prompts)-1)
	if to == from {
		return nil
	}

	p := s.Prompts[from]
	next := slices.Delete(slices.Clone(s.Prompts), from, from+1)
	s.Prompts = slices.Insert(next, to, p)
	return s.save()
}

// ImportMarkdown appends every prompt found in markdown text and reports how
// many were added.
func (s *Store) ImportMarkdown(md string) (int, error) {
	now := s.now().UnixMilli()

	parsed := ParseMarkdown(md)
	for i, p := range parsed {
		// Distinct timestamps keep imported prompts orderable by age.
		s.Prompts = append(s.Prompts, Prompt{
			ID:        s.newID(),
			Title:     p.Title,
			Text:      p.Text,
			CreatedAt: now + int64(i),
			UpdatedAt: now + int64(i),
		})
	}
	return len(parsed), s.save()
}

// ExportMarkdown writes the library to a markdown file.
func (s *Store) ExportMarkdown(path string) error {
	return os.WriteFile(path, []byte(FormatMarkdown(s.Prompts)), 0o644)
}

// save writes the library out.
//
// Unlike the original this writes a temporary file and renames it, so another
// instance watching the file can never read a half-written library. The
// mid-write guard in ReloadIfChanged stays as a backstop for libraries written
// by the extension.
func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.Prompts, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.file), ".prompts-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), s.file); err != nil {
		return err
	}

	s.lastRead = string(raw)
	return nil
}

func defaultTitle(title string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	return "Untitled"
}

// newUUID is a random v4 identifier, matching the shape the extension wrote.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
