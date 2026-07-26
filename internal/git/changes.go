package git

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// emptyTree is git's hash of the empty tree, used to diff a repo that has
	// no commits yet.
	emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

	maxDiffChars = 200_000
)

// Status is how a file differs from HEAD.
type Status string

const (
	StatusNew     Status = "new"
	StatusChanged Status = "changed"
	StatusDeleted Status = "deleted"
)

// FileEntry is one file differing from HEAD, staged, unstaged or untracked.
type FileEntry struct {
	Path      string
	Status    Status
	Additions int
	Deletions int
	Binary    bool
	ModTime   time.Time
	Diff      string
	// Watched is set by the watcher when the path matches a configured glob.
	Watched bool
}

// CollectChanges returns every file differing from HEAD.
func (r Repo) CollectChanges() ([]FileEntry, error) {
	// A repo with no commits has no HEAD to diff against.
	base := "HEAD"
	if _, err := r.runStrict("rev-parse", "--verify", "HEAD"); err != nil {
		base = emptyTree
	}

	out, err := r.runAll(
		[]string{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		[]string{"diff", "--numstat", "-z", "-M", base},
		[]string{"diff", "-M", base},
	)
	if err != nil {
		return nil, err
	}

	entries := parseStatus(out[0])
	counted := parseNumstat(out[1])
	diffs := parseDiffs(out[2])

	files := make([]FileEntry, 0, entries.len())
	entries.all(func(p string, status Status) {
		diff, _ := diffs.get(p)
		c, ok := counted.get(p)

		if !ok && status == StatusNew {
			// An untracked file is in no diff against HEAD, so it is diffed
			// against /dev/null to produce one.
			diff, c = r.untrackedDiff(p)
		}

		files = append(files, FileEntry{
			Path:      p,
			Status:    status,
			Additions: c.additions,
			Deletions: c.deletions,
			Binary:    c.binary,
			ModTime:   r.modTime(p),
			Diff:      truncateDiff(diff),
		})
	})
	return files, nil
}

// untrackedDiff builds the diff and counts for a file git does not track yet.
func (r Repo) untrackedDiff(p string) (string, counts) {
	diff, err := r.run("diff", "--no-index", "--", os.DevNull, p)
	if err != nil {
		return "", counts{}
	}

	binary := binaryDiffRe.MatchString(diff)
	c := counts{binary: binary}
	if !binary {
		for _, line := range strings.Split(diff, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				c.additions++
			}
		}
	}
	return diff, c
}

// modTime is the file's mtime, or the zero time for a deleted file.
func (r Repo) modTime(p string) time.Time {
	info, err := os.Stat(filepath.Join(r.Root, p))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// DiscardFile reverts a file to its HEAD state, or deletes it if it is new.
func (r Repo) DiscardFile(p string, status Status) error {
	if status != StatusNew {
		_, err := r.runStrict("checkout", "HEAD", "--", p)
		return err
	}

	// This clears a staged-new file from the index and disk. A plain untracked
	// file is left alone by --ignore-unmatch, so it is removed directly after.
	if _, err := r.runStrict("rm", "-f", "--ignore-unmatch", "--", p); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(r.Root, p)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
