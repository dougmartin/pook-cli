package oob

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Ported from test/oob.test.js.

// makeHome lays out global, repo and branch files under a fresh OOB_HOME and
// points the package at it.
func makeHome(t *testing.T, repo, branch string) string {
	t.Helper()
	home := t.TempDir()

	write(t, home, filepath.Join("global", "g.md"), "global note\n")
	write(t, home, filepath.Join("repos", repo, "r.md"), "repo note\n")
	write(t, home, filepath.Join("repos", repo, "sub", "nested.md"), "nested repo note\n")
	write(t, home, filepath.Join("branches", repo, branch, "b.md"), "branch note\n")

	t.Setenv("OOB_HOME", home)
	return home
}

func write(t *testing.T, home, rel, body string) {
	t.Helper()
	abs := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func namespaces(groups []Group) []Namespace {
	out := make([]Namespace, len(groups))
	for i, g := range groups {
		out[i] = g.Namespace
	}
	return out
}

func filePaths(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func TestHomeHonorsOOBHome(t *testing.T) {
	t.Setenv("OOB_HOME", "/some/where/oob")
	if got := Home(); got != "/some/where/oob" {
		t.Errorf("Home() = %q, want /some/where/oob", got)
	}
}

func TestHomeFallsBackToTheDefault(t *testing.T) {
	t.Setenv("OOB_HOME", "")
	t.Setenv("HOME", "/home/tester")

	if got := Home(); got != "/home/tester/oob" {
		t.Errorf("Home() = %q, want /home/tester/oob", got)
	}
}

func TestNamespaceDirsNestsASlashedBranch(t *testing.T) {
	dirs := NamespaceDirs("/home/x/oob", "myrepo", "feature/foo")

	if want := []Namespace{NamespaceBranch, NamespaceRepo, NamespaceGlobal}; !slices.Equal(namespaces(dirs), want) {
		t.Fatalf("namespaces = %v, want %v", namespaces(dirs), want)
	}
	// A branch with a slash nests as directories, the way the server writes it.
	if want := filepath.Join("/home/x/oob", "branches", "myrepo", "feature", "foo"); dirs[0].Dir != want {
		t.Errorf("branch dir = %q, want %q", dirs[0].Dir, want)
	}
	if want := filepath.Join("/home/x/oob", "repos", "myrepo"); dirs[1].Dir != want {
		t.Errorf("repo dir = %q, want %q", dirs[1].Dir, want)
	}
	if want := filepath.Join("/home/x/oob", "global"); dirs[2].Dir != want {
		t.Errorf("global dir = %q, want %q", dirs[2].Dir, want)
	}
}

func TestCollectReturnsEveryNamespaceWithItsFiles(t *testing.T) {
	makeHome(t, "myrepo", "main")

	groups := Collect("myrepo", "main")
	if want := []Namespace{NamespaceBranch, NamespaceRepo, NamespaceGlobal}; !slices.Equal(namespaces(groups), want) {
		t.Fatalf("namespaces = %v, want %v", namespaces(groups), want)
	}

	if want := []string{"b.md"}; !slices.Equal(filePaths(groups[0].Files), want) {
		t.Errorf("branch files = %v, want %v", filePaths(groups[0].Files), want)
	}
	// Recursive listing, sorted A to Z.
	if want := []string{"r.md", "sub/nested.md"}; !slices.Equal(filePaths(groups[1].Files), want) {
		t.Errorf("repo files = %v, want %v", filePaths(groups[1].Files), want)
	}
	if want := []string{"g.md"}; !slices.Equal(filePaths(groups[2].Files), want) {
		t.Errorf("global files = %v, want %v", filePaths(groups[2].Files), want)
	}

	if got := groups[1].Files[0].Content; got != "repo note\n" {
		t.Errorf("content = %q, want %q", got, "repo note\n")
	}
	if want := filepath.Join("main", "b.md"); !hasSuffix(groups[0].Files[0].FSPath, want) {
		t.Errorf("fsPath = %q, want it to end with %q", groups[0].Files[0].FSPath, want)
	}
	if groups[0].Files[0].ModTime == 0 {
		t.Error("branch file has no mtime")
	}
}

func TestCollectOmitsTheBranchGroupWhenNoBranchIsKnown(t *testing.T) {
	makeHome(t, "myrepo", "main")

	groups := Collect("myrepo", "")
	if want := []Namespace{NamespaceRepo, NamespaceGlobal}; !slices.Equal(namespaces(groups), want) {
		t.Errorf("namespaces = %v, want %v", namespaces(groups), want)
	}
}

// A repo with no oob files yields empty groups rather than errors, so the tab
// can still show every namespace header.
func TestCollectYieldsEmptyGroupsForAnUnknownRepo(t *testing.T) {
	makeHome(t, "myrepo", "main")

	groups := Collect("other-repo", "other-branch")
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	if len(groups[0].Files) != 0 || len(groups[1].Files) != 0 {
		t.Errorf("branch/repo groups are not empty: %v %v",
			filePaths(groups[0].Files), filePaths(groups[1].Files))
	}
	// global is shared, so it still has its file.
	if want := []string{"g.md"}; !slices.Equal(filePaths(groups[2].Files), want) {
		t.Errorf("global files = %v, want %v", filePaths(groups[2].Files), want)
	}
}

func TestCollectFlagsBinaryFilesAndSkipsTheirContent(t *testing.T) {
	home := makeHome(t, "myrepo", "main")
	if err := os.WriteFile(filepath.Join(home, "global", "logo.bin"),
		[]byte{0x89, 0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}

	groups := Collect("myrepo", "main")
	for _, f := range groups[2].Files {
		if f.Path != "logo.bin" {
			continue
		}
		if !f.Binary {
			t.Error("logo.bin was not flagged as binary")
		}
		if f.Content != "" {
			t.Errorf("binary content = %q, want empty", f.Content)
		}
		return
	}
	t.Fatalf("logo.bin missing from %v", filePaths(groups[2].Files))
}

// Beyond the ported cases.

func TestCollectSkipsDotfiles(t *testing.T) {
	home := makeHome(t, "myrepo", "main")
	write(t, home, filepath.Join("global", ".hidden"), "secret\n")
	write(t, home, filepath.Join("global", ".git", "config"), "[core]\n")

	groups := Collect("myrepo", "main")
	if want := []string{"g.md"}; !slices.Equal(filePaths(groups[2].Files), want) {
		t.Errorf("global files = %v, want %v", filePaths(groups[2].Files), want)
	}
}

func TestCollectTruncatesHugeFiles(t *testing.T) {
	home := makeHome(t, "myrepo", "main")
	big := make([]byte, maxContentChars+100)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(home, "global", "big.txt"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, f := range Collect("myrepo", "main")[2].Files {
		if f.Path != "big.txt" {
			continue
		}
		if !f.Truncated {
			t.Error("big.txt was not flagged as truncated")
		}
		if len([]rune(f.Content)) != maxContentChars {
			t.Errorf("content is %d chars, want %d", len([]rune(f.Content)), maxContentChars)
		}
		return
	}
	t.Fatal("big.txt missing")
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
