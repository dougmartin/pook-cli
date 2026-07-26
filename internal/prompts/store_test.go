package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// tmpStore is a library in a throwaway directory, with stable ids and clock so
// the assertions do not depend on either.
func tmpStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(filepath.Join(t.TempDir(), "prompts.json"))

	n := 0
	s.newID = func() string {
		n++
		return fmt.Sprintf("id-%d", n)
	}
	s.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	return s
}

func titles(prompts []Prompt) []string {
	out := make([]string, len(prompts))
	for i, p := range prompts {
		out[i] = p.Title
	}
	return out
}

func TestAddAppendsToTheEnd(t *testing.T) {
	s := tmpStore(t)
	mustAdd(t, s, "First", "one")
	mustAdd(t, s, "Second", "two")

	if want := []string{"First", "Second"}; !slices.Equal(titles(s.Prompts), want) {
		t.Errorf("titles = %v, want %v", titles(s.Prompts), want)
	}
}

func TestAddDefaultsAnEmptyTitle(t *testing.T) {
	s := tmpStore(t)
	mustAdd(t, s, "   ", "body")

	if s.Prompts[0].Title != "Untitled" {
		t.Errorf("title = %q, want Untitled", s.Prompts[0].Title)
	}
}

// The library is shared by every running instance, so each one has to follow
// the file rather than trust its own copy.
func TestReloadIfChangedPicksUpAnotherInstancesWrite(t *testing.T) {
	s := tmpStore(t)
	mustAdd(t, s, "Mine", "one")

	// Our own write is already in memory: nothing to report.
	if s.ReloadIfChanged() {
		t.Error("our own write was reported as a change")
	}

	other := NewStore(s.Path())
	other.Load()
	mustAdd(t, other, "Theirs", "two")

	if !s.ReloadIfChanged() {
		t.Fatal("another instance's write was not picked up")
	}
	if want := []string{"Mine", "Theirs"}; !slices.Equal(titles(s.Prompts), want) {
		t.Errorf("titles = %v, want %v", titles(s.Prompts), want)
	}
	if s.ReloadIfChanged() {
		t.Error("the reload did not settle")
	}
}

func TestReloadIfChangedKeepsTheListWhenTheFileIsMidWrite(t *testing.T) {
	s := tmpStore(t)
	mustAdd(t, s, "Mine", "one")

	if err := os.WriteFile(s.Path(), []byte(`[{"id": "trunc`), 0o644); err != nil {
		t.Fatal(err)
	}

	if s.ReloadIfChanged() {
		t.Error("a half-written file was reported as a change")
	}
	if want := []string{"Mine"}; !slices.Equal(titles(s.Prompts), want) {
		t.Errorf("titles = %v, want %v", titles(s.Prompts), want)
	}
}

func TestLoadOnAMissingFileIsAnEmptyLibrary(t *testing.T) {
	s := tmpStore(t)
	s.Load()

	if len(s.Prompts) != 0 {
		t.Errorf("prompts = %v, want none on first run", titles(s.Prompts))
	}
}

func TestUpdateAndRemove(t *testing.T) {
	s := tmpStore(t)
	first, _ := s.Add("First", "one")
	mustAdd(t, s, "Second", "two")

	if err := s.Update(first.ID, "Renamed", "changed"); err != nil {
		t.Fatal(err)
	}
	if s.Prompts[0].Title != "Renamed" || s.Prompts[0].Text != "changed" {
		t.Errorf("update did not apply: %+v", s.Prompts[0])
	}
	if err := s.Update("no-such-id", "x", "y"); err != nil {
		t.Errorf("updating an unknown id errored: %v", err)
	}

	if err := s.Remove(first.ID); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Second"}; !slices.Equal(titles(s.Prompts), want) {
		t.Errorf("titles = %v, want %v", titles(s.Prompts), want)
	}

	// The change is on disk, not just in memory.
	reopened := NewStore(s.Path())
	reopened.Load()
	if want := []string{"Second"}; !slices.Equal(titles(reopened.Prompts), want) {
		t.Errorf("reopened titles = %v, want %v", titles(reopened.Prompts), want)
	}
}

func TestReorderPutsUnlistedIdsAtTheEndInOrder(t *testing.T) {
	s := tmpStore(t)
	a, _ := s.Add("A", "1")
	b, _ := s.Add("B", "2")
	c, _ := s.Add("C", "3")

	if err := s.Reorder([]string{c.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"C", "A", "B"}; !slices.Equal(titles(s.Prompts), want) {
		t.Errorf("titles = %v, want %v", titles(s.Prompts), want)
	}
	_ = b
}

// Move is what the reorder keys drive, in place of drag and drop.
func TestMove(t *testing.T) {
	tests := []struct {
		name  string
		which int
		delta int
		want  []string
	}{
		{name: "down one", which: 0, delta: 1, want: []string{"B", "A", "C"}},
		{name: "up one", which: 2, delta: -1, want: []string{"A", "C", "B"}},
		{name: "up from the top is clamped", which: 0, delta: -1, want: []string{"A", "B", "C"}},
		{name: "down from the bottom is clamped", which: 2, delta: 1, want: []string{"A", "B", "C"}},
		{name: "a big jump is clamped to the end", which: 0, delta: 9, want: []string{"B", "C", "A"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tmpStore(t)
			mustAdd(t, s, "A", "1")
			mustAdd(t, s, "B", "2")
			mustAdd(t, s, "C", "3")

			if err := s.Move(s.Prompts[tt.which].ID, tt.delta); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(titles(s.Prompts), tt.want) {
				t.Errorf("titles = %v, want %v", titles(s.Prompts), tt.want)
			}
		})
	}
}

func TestMoveUnknownIDIsANoOp(t *testing.T) {
	s := tmpStore(t)
	mustAdd(t, s, "A", "1")

	if err := s.Move("no-such-id", 1); err != nil {
		t.Fatal(err)
	}
	if want := []string{"A"}; !slices.Equal(titles(s.Prompts), want) {
		t.Errorf("titles = %v, want %v", titles(s.Prompts), want)
	}
}

func TestImportAndExportRoundTripThroughDisk(t *testing.T) {
	s := tmpStore(t)
	mustAdd(t, s, "Review", "Review {{file}} for bugs.")
	mustAdd(t, s, "Explain", "Explain this code line by line.")

	path := filepath.Join(t.TempDir(), "export.md")
	if err := s.ExportMarkdown(path); err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	fresh := tmpStore(t)
	n, err := fresh.ImportMarkdown(string(md))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("imported %d prompts, want 2", n)
	}
	if !slices.Equal(titles(fresh.Prompts), titles(s.Prompts)) {
		t.Errorf("titles = %v, want %v", titles(fresh.Prompts), titles(s.Prompts))
	}
	if fresh.Prompts[0].Text != s.Prompts[0].Text {
		t.Errorf("text = %q, want %q", fresh.Prompts[0].Text, s.Prompts[0].Text)
	}
}

func TestImportAppendsToAnExistingLibrary(t *testing.T) {
	s := tmpStore(t)
	mustAdd(t, s, "Existing", "keep me")

	if _, err := s.ImportMarkdown("## Added\n\nnew body\n"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Existing", "Added"}; !slices.Equal(titles(s.Prompts), want) {
		t.Errorf("titles = %v, want %v", titles(s.Prompts), want)
	}
	// Imported prompts get distinct timestamps so they stay orderable.
	if s.Prompts[1].CreatedAt == 0 {
		t.Error("imported prompt has no createdAt")
	}
}

// A saved library must never be observable half written, since other instances
// watch the file.
func TestSaveIsAtomic(t *testing.T) {
	s := tmpStore(t)
	mustAdd(t, s, "A", "1")

	entries, err := os.ReadDir(filepath.Dir(s.Path()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "prompts.json" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory holds %v, want just prompts.json", names)
	}
}

func mustAdd(t *testing.T, s *Store, title, text string) {
	t.Helper()
	if _, err := s.Add(title, text); err != nil {
		t.Fatal(err)
	}
}
