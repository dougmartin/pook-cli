package git

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// These five cases are ported from test/git.test.js, which is the
// specification for base-ref selection.

// makeRepo builds main (2 commits), feature on main (1), stacked on feature
// (1), matching the fixture in the original test file.
func makeRepo(t *testing.T) Repo {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "init", "-q", "-b", "main")
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "Test")

	commit(t, dir, "a.txt", "main one")
	commit(t, dir, "b.txt", "main two")
	run(t, dir, "checkout", "-q", "-b", "feature")
	commit(t, dir, "c.txt", "feature one")
	run(t, dir, "checkout", "-q", "-b", "stacked")
	commit(t, dir, "d.txt", "stacked one")

	return Repo{Root: dir}
}

func commit(t *testing.T, dir, file, subject string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(subject+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", subject)
}

// subjects is the commit list reduced to what the assertions care about.
func subjects(commits []CommitEntry) []string {
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i] = c.Subject
	}
	return out
}

func collectBranch(t *testing.T, r Repo) BranchInfo {
	t.Helper()
	info, err := r.CollectBranch()
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestOnMainNoBaseDivergesSoRecentCommitsAreListed(t *testing.T) {
	r := makeRepo(t)
	run(t, r.Root, "checkout", "-q", "main")

	info := collectBranch(t, r)
	if info.Name != "main" {
		t.Errorf("name = %q, want main", info.Name)
	}
	if info.BaseRef != "" {
		t.Errorf("baseRef = %q, want none", info.BaseRef)
	}
	if want := []string{"main two", "main one"}; !slices.Equal(subjects(info.Commits), want) {
		t.Errorf("commits = %v, want %v", subjects(info.Commits), want)
	}
}

func TestBranchOffMainListsOnlyItsOwnCommits(t *testing.T) {
	r := makeRepo(t)
	run(t, r.Root, "checkout", "-q", "feature")

	info := collectBranch(t, r)
	if info.Name != "feature" {
		t.Errorf("name = %q, want feature", info.Name)
	}
	if info.BaseRef != "main" {
		t.Errorf("baseRef = %q, want main", info.BaseRef)
	}
	if want := []string{"feature one"}; !slices.Equal(subjects(info.Commits), want) {
		t.Errorf("commits = %v, want %v", subjects(info.Commits), want)
	}
}

func TestStackedBranchBasesOnNearestParentNotMain(t *testing.T) {
	r := makeRepo(t)
	run(t, r.Root, "checkout", "-q", "stacked")

	info := collectBranch(t, r)
	// Not main: the stack underneath is not repeated.
	if info.BaseRef != "feature" {
		t.Errorf("baseRef = %q, want feature", info.BaseRef)
	}
	if want := []string{"stacked one"}; !slices.Equal(subjects(info.Commits), want) {
		t.Errorf("commits = %v, want %v", subjects(info.Commits), want)
	}
}

func TestFreshBranchBasesOnWhatItWasCutFrom(t *testing.T) {
	r := makeRepo(t)
	run(t, r.Root, "checkout", "-q", "stacked")
	run(t, r.Root, "checkout", "-q", "-b", "stacked-2") // same tip, nothing committed

	info := collectBranch(t, r)
	if info.Name != "stacked-2" {
		t.Errorf("name = %q, want stacked-2", info.Name)
	}
	// The branch it was cut from, even though that tip is HEAD.
	if info.BaseRef != "stacked" {
		t.Errorf("baseRef = %q, want stacked", info.BaseRef)
	}
	if len(info.Commits) != 0 {
		t.Errorf("commits = %v, want none on a fresh branch", subjects(info.Commits))
	}
}

func TestSiblingBranchesAreIgnored(t *testing.T) {
	r := makeRepo(t)
	run(t, r.Root, "checkout", "-q", "main")
	run(t, r.Root, "checkout", "-q", "-b", "sibling")
	run(t, r.Root, "commit", "-q", "--allow-empty", "-m", "sibling one")
	run(t, r.Root, "checkout", "-q", "feature")

	info := collectBranch(t, r)
	// sibling is not an ancestor of feature, so only ancestors can be a base.
	if info.BaseRef != "main" {
		t.Errorf("baseRef = %q, want main", info.BaseRef)
	}
	if want := []string{"feature one"}; !slices.Equal(subjects(info.Commits), want) {
		t.Errorf("commits = %v, want %v", subjects(info.Commits), want)
	}
}

// Beyond the ported cases: the totals and per-commit stats the overview line
// is built from.

func TestBranchTotals(t *testing.T) {
	r := makeRepo(t)
	run(t, r.Root, "checkout", "-q", "feature")

	info := collectBranch(t, r)
	if len(info.Commits) != 1 {
		t.Fatalf("commits = %v, want one", subjects(info.Commits))
	}
	c := info.Commits[0]
	if c.Additions != 1 || c.Deletions != 0 || c.FileCount != 1 {
		t.Errorf("commit stats = +%d -%d over %d files, want +1 -0 over 1", c.Additions, c.Deletions, c.FileCount)
	}
	if c.Short == "" || len(c.Hash) != 40 {
		t.Errorf("hash = %q, short = %q", c.Hash, c.Short)
	}
	if c.Time.IsZero() {
		t.Error("commit time is zero")
	}
	if info.TotalAdditions != 1 || info.TotalDeletions != 0 || info.FilesTouched != 1 {
		t.Errorf("totals = +%d -%d over %d files, want +1 -0 over 1",
			info.TotalAdditions, info.TotalDeletions, info.FilesTouched)
	}
}

// A repo with no commits has no HEAD, and must not error.
func TestBranchOnEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")

	info := collectBranch(t, Repo{Root: dir})
	if info.Name != "main" {
		t.Errorf("name = %q, want main", info.Name)
	}
	if len(info.Commits) != 0 {
		t.Errorf("commits = %v, want none", subjects(info.Commits))
	}
}

func TestBranchNameWhenDetached(t *testing.T) {
	r := makeRepo(t)
	run(t, r.Root, "checkout", "-q", "--detach", "HEAD")

	if info := collectBranch(t, r); info.Name != "HEAD" {
		t.Errorf("name = %q, want HEAD when detached", info.Name)
	}
}

func TestCommitDiffIsPerFile(t *testing.T) {
	r := makeRepo(t)
	run(t, r.Root, "checkout", "-q", "main")

	info := collectBranch(t, r)
	files, err := r.CommitDiff(info.Commits[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	f := files[0]
	if f.Path != "b.txt" || f.Additions != 1 || f.Deletions != 0 || f.Binary {
		t.Errorf("file = %+v, want b.txt +1 -0 text", f)
	}
	if !strings.Contains(f.Diff, "+main two") {
		t.Errorf("diff does not contain the added line:\n%s", f.Diff)
	}
}
