package git

import (
	"slices"
	"strings"
	"testing"
)

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []struct {
			path   string
			status Status
		}
	}{
		{
			name: "untracked is new",
			out:  "?? added.txt\x00",
			want: pathStatus("added.txt", StatusNew),
		},
		{
			name: "staged add is new",
			out:  "A  staged.txt\x00",
			want: pathStatus("staged.txt", StatusNew),
		},
		{
			name: "modified is changed",
			out:  " M mod.txt\x00",
			want: pathStatus("mod.txt", StatusChanged),
		},
		{
			name: "deleted in either column is deleted",
			out:  "D  staged-delete.txt\x00 D worktree-delete.txt\x00",
			want: append(pathStatus("staged-delete.txt", StatusDeleted),
				pathStatus("worktree-delete.txt", StatusDeleted)...),
		},
		{
			// A rename record is followed by the original path, which is not
			// an entry of its own.
			name: "rename consumes the original path",
			out:  "R  new-name.txt\x00old-name.txt\x00 M after.txt\x00",
			want: append(pathStatus("new-name.txt", StatusChanged),
				pathStatus("after.txt", StatusChanged)...),
		},
		{
			name: "copy consumes the source path",
			out:  "C  copy.txt\x00source.txt\x00",
			want: pathStatus("copy.txt", StatusChanged),
		},
		{
			name: "tokens too short to hold a path are skipped",
			out:  "?? \x00xy\x00?? real.txt\x00",
			want: pathStatus("real.txt", StatusNew),
		},
		{
			name: "empty output",
			out:  "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStatus(tt.out)

			if got.len() != len(tt.want) {
				t.Fatalf("parsed %d entries, want %d: %v", got.len(), len(tt.want), got.keys)
			}
			i := 0
			got.all(func(p string, s Status) {
				if p != tt.want[i].path || s != tt.want[i].status {
					t.Errorf("entry %d = %q %s, want %q %s", i, p, s, tt.want[i].path, tt.want[i].status)
				}
				i++
			})
		})
	}
}

// pathStatus builds a one-entry expectation.
func pathStatus(path string, status Status) []struct {
	path   string
	status Status
} {
	return []struct {
		path   string
		status Status
	}{{path, status}}
}

func TestParseNumstat(t *testing.T) {
	tests := []struct {
		name  string
		out   string
		want  map[string]counts
		order []string
	}{
		{
			name:  "text file counts",
			out:   "3\t4\tinternal/ui/app.go\x00",
			want:  map[string]counts{"internal/ui/app.go": {additions: 3, deletions: 4}},
			order: []string{"internal/ui/app.go"},
		},
		{
			name:  "binary reports dashes and counts as zero",
			out:   "-\t-\tmedia/logo.png\x00",
			want:  map[string]counts{"media/logo.png": {binary: true}},
			order: []string{"media/logo.png"},
		},
		{
			// A rename emits an empty path, then the old and new paths as
			// separate records.
			name:  "rename record uses the new path",
			out:   "1\t2\t\x00old/name.go\x00new/name.go\x00",
			want:  map[string]counts{"new/name.go": {additions: 1, deletions: 2}},
			order: []string{"new/name.go"},
		},
		{
			name: "order is preserved",
			out:  "1\t0\tb.txt\x005\t5\ta.txt\x00",
			want: map[string]counts{
				"b.txt": {additions: 1},
				"a.txt": {additions: 5, deletions: 5},
			},
			order: []string{"b.txt", "a.txt"},
		},
		{
			name: "junk is skipped",
			out:  "not a record\x001\t1\tok.txt\x00",
			want: map[string]counts{"ok.txt": {additions: 1, deletions: 1}},

			order: []string{"ok.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNumstat(tt.out)

			if !slices.Equal(got.keys, tt.order) {
				t.Fatalf("paths = %v, want %v", got.keys, tt.order)
			}
			for p, want := range tt.want {
				c, ok := got.get(p)
				if !ok {
					t.Fatalf("missing %q", p)
				}
				if c != want {
					t.Errorf("%q = %+v, want %+v", p, c, want)
				}
			}
		})
	}
}

const twoFilePatch = `diff --git a/a.txt b/a.txt
index 1234567..89abcde 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
diff --git a/b.txt b/b.txt
index 1234567..89abcde 100644
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-one
+two
`

func TestParseDiffs(t *testing.T) {
	diffs := parseDiffs(twoFilePatch)

	if !slices.Equal(diffs.keys, []string{"a.txt", "b.txt"}) {
		t.Fatalf("paths = %v, want [a.txt b.txt]", diffs.keys)
	}

	a, _ := diffs.get("a.txt")
	if !strings.HasPrefix(a, "diff --git a/a.txt") || !strings.Contains(a, "+new") {
		t.Errorf("a.txt chunk is wrong:\n%s", a)
	}
	if strings.Contains(a, "b.txt") {
		t.Errorf("a.txt chunk leaked into the next file:\n%s", a)
	}
}

func TestParseDiffsNames(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  string
	}{
		{
			name:  "deleted file has no post-image, so the pre-image names it",
			patch: "diff --git a/gone.txt b/gone.txt\n--- a/gone.txt\n+++ /dev/null\n",
			want:  "gone.txt",
		},
		{
			name:  "binary file has neither, so the header names it",
			patch: "diff --git a/logo.png b/logo.png\nBinary files a/logo.png and b/logo.png differ\n",
			want:  "logo.png",
		},
		{
			name:  "path with spaces",
			patch: "diff --git a/some file.txt b/some file.txt\n--- a/some file.txt\n+++ b/some file.txt\n",
			want:  "some file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffs := parseDiffs(tt.patch)
			if !slices.Equal(diffs.keys, []string{tt.want}) {
				t.Fatalf("paths = %v, want [%s]", diffs.keys, tt.want)
			}
		})
	}
}

// The original split on a lookahead, which RE2 cannot express, so the hand
// written split has to agree with it, including on content that merely looks
// like a header.
func TestSplitDiffChunks(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  int
	}{
		{name: "empty", patch: "", want: 0},
		{name: "no header", patch: "just text\n", want: 0},
		{name: "one file", patch: "diff --git a/a b/a\n+x\n", want: 1},
		{name: "two files", patch: twoFilePatch, want: 2},
		{
			// An added line whose content starts with the marker is indented
			// by the leading +, so it is not a header.
			name:  "marker inside a hunk is not a split point",
			patch: "diff --git a/a b/a\n@@ -0,0 +1 @@\n+diff --git a/fake b/fake\n",
			want:  1,
		},
		{
			name:  "no trailing newline",
			patch: "diff --git a/a b/a\n+x",
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(splitDiffChunks(tt.patch)); got != tt.want {
				t.Errorf("chunks = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTruncateDiff(t *testing.T) {
	short := strings.Repeat("x", maxDiffChars)
	if got := truncateDiff(short); got != short {
		t.Error("a diff at the limit was truncated")
	}

	long := strings.Repeat("x", maxDiffChars+1)
	got := truncateDiff(long)
	if !strings.HasSuffix(got, "\n… diff truncated …") {
		t.Errorf("truncated diff does not end with the marker: %q", got[len(got)-30:])
	}
	if len(got) != maxDiffChars+len("\n… diff truncated …") {
		t.Errorf("truncated length = %d", len(got))
	}
}

func TestOrderedKeepsFirstPosition(t *testing.T) {
	o := newOrdered[int]()
	o.set("b", 1)
	o.set("a", 2)
	o.set("b", 3) // reassigning must not move b to the end

	if !slices.Equal(o.keys, []string{"b", "a"}) {
		t.Fatalf("keys = %v, want [b a]", o.keys)
	}
	if v, _ := o.get("b"); v != 3 {
		t.Errorf("b = %d, want 3", v)
	}
}
