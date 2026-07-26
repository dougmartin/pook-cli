package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// changed returns the entry for a path, or fails.
func changed(t *testing.T, files []FileEntry, path string) FileEntry {
	t.Helper()
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no entry for %q in %v", path, paths(files))
	return FileEntry{}
}

func paths(files []FileEntry) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func collectChanges(t *testing.T, r Repo) []FileEntry {
	t.Helper()
	files, err := r.CollectChanges()
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectChangesCoversEveryKindOfChange(t *testing.T) {
	r := makeRepo(t)

	write(t, r.Root, "a.txt", "main one\nextra\n") // modified, tracked
	write(t, r.Root, "untracked.txt", "brand new\n")
	if err := os.Remove(filepath.Join(r.Root, "b.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, r.Root, "staged.txt", "staged new\n")
	run(t, r.Root, "add", "staged.txt")

	files := collectChanges(t, r)

	mod := changed(t, files, "a.txt")
	if mod.Status != StatusChanged || mod.Additions != 1 || mod.Deletions != 0 {
		t.Errorf("a.txt = %s +%d -%d, want changed +1 -0", mod.Status, mod.Additions, mod.Deletions)
	}
	if !strings.Contains(mod.Diff, "+extra") {
		t.Errorf("a.txt diff is missing the added line:\n%s", mod.Diff)
	}
	if mod.ModTime.IsZero() {
		t.Error("a.txt has no mtime")
	}

	if got := changed(t, files, "b.txt"); got.Status != StatusDeleted {
		t.Errorf("b.txt = %s, want deleted", got.Status)
	}
	if got := changed(t, files, "staged.txt"); got.Status != StatusNew {
		t.Errorf("staged.txt = %s, want new", got.Status)
	}

	// An untracked file is in no diff against HEAD, so its diff and counts
	// come from the --no-index path.
	untracked := changed(t, files, "untracked.txt")
	if untracked.Status != StatusNew || untracked.Additions != 1 {
		t.Errorf("untracked.txt = %s +%d, want new +1", untracked.Status, untracked.Additions)
	}
	if !strings.Contains(untracked.Diff, "+brand new") {
		t.Errorf("untracked.txt has no diff:\n%s", untracked.Diff)
	}
}

// A deleted file has no mtime, and that must not be an error.
func TestCollectChangesDeletedFileHasNoModTime(t *testing.T) {
	r := makeRepo(t)
	if err := os.Remove(filepath.Join(r.Root, "a.txt")); err != nil {
		t.Fatal(err)
	}

	if got := changed(t, collectChanges(t, r), "a.txt"); !got.ModTime.IsZero() {
		t.Errorf("mtime = %v, want zero for a deleted file", got.ModTime)
	}
}

func TestCollectChangesDetectsBinary(t *testing.T) {
	r := makeRepo(t)
	if err := os.WriteFile(filepath.Join(r.Root, "blob.bin"),
		[]byte{0x00, 0x01, 0x02, 0xff, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}

	got := changed(t, collectChanges(t, r), "blob.bin")
	if !got.Binary {
		t.Errorf("blob.bin was not detected as binary: %+v", got)
	}
	if got.Additions != 0 {
		t.Errorf("binary additions = %d, want 0", got.Additions)
	}
}

// A repo with no commits has no HEAD, so everything is diffed against the
// empty tree instead.
func TestCollectChangesOnEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "first.txt", "hello\n")
	run(t, dir, "add", "-A")

	files := collectChanges(t, Repo{Root: dir})
	got := changed(t, files, "first.txt")
	if got.Status != StatusNew {
		t.Errorf("first.txt = %s, want new", got.Status)
	}
	if got.Additions != 1 {
		t.Errorf("first.txt +%d, want +1", got.Additions)
	}
}

func TestCollectChangesInSubdirectories(t *testing.T) {
	r := makeRepo(t)
	write(t, r.Root, "internal/ui/app.go", "package ui\n")

	got := changed(t, collectChanges(t, r), "internal/ui/app.go")
	if got.Status != StatusNew {
		t.Errorf("nested file = %s, want new", got.Status)
	}
}

func TestDiscardFile(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, r Repo) (path string, status Status)
		wantExists bool
		wantBody   string
	}{
		{
			name: "a modified file reverts to HEAD",
			setup: func(t *testing.T, r Repo) (string, Status) {
				write(t, r.Root, "a.txt", "clobbered\n")
				return "a.txt", StatusChanged
			},
			wantExists: true,
			wantBody:   "main one\n",
		},
		{
			name: "an untracked file is deleted",
			setup: func(t *testing.T, r Repo) (string, Status) {
				write(t, r.Root, "junk.txt", "junk\n")
				return "junk.txt", StatusNew
			},
			wantExists: false,
		},
		{
			name: "a staged new file is removed from the index and disk",
			setup: func(t *testing.T, r Repo) (string, Status) {
				write(t, r.Root, "staged.txt", "staged\n")
				run(t, r.Root, "add", "staged.txt")
				return "staged.txt", StatusNew
			},
			wantExists: false,
		},
		{
			name: "a deleted file is restored",
			setup: func(t *testing.T, r Repo) (string, Status) {
				if err := os.Remove(filepath.Join(r.Root, "a.txt")); err != nil {
					t.Fatal(err)
				}
				return "a.txt", StatusDeleted
			},
			wantExists: true,
			wantBody:   "main one\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := makeRepo(t)
			path, status := tt.setup(t, r)

			if err := r.DiscardFile(path, status); err != nil {
				t.Fatal(err)
			}

			body, err := os.ReadFile(filepath.Join(r.Root, path))
			switch {
			case tt.wantExists && err != nil:
				t.Fatalf("%s should still exist: %v", path, err)
			case !tt.wantExists && err == nil:
				t.Fatalf("%s should have been deleted", path)
			case tt.wantExists && string(body) != tt.wantBody:
				t.Errorf("%s = %q, want %q", path, body, tt.wantBody)
			}

			// Whatever the case, the file is no longer a pending change.
			for _, f := range collectChanges(t, r) {
				if f.Path == path {
					t.Errorf("%s is still listed as %s after discard", path, f.Status)
				}
			}
		})
	}
}

// Discarding must not silently succeed when git fails.
func TestDiscardFileReportsFailure(t *testing.T) {
	r := makeRepo(t)
	if err := r.DiscardFile("does/not/exist.txt", StatusChanged); err == nil {
		t.Error("discarding an unknown path succeeded, want an error")
	}
}
