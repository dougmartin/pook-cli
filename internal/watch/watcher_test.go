package watch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testDebounce is short enough to keep the suite quick and long enough that a
// burst still coalesces.
const testDebounce = 20 * time.Millisecond

func newTestWatcher(t *testing.T) *Watcher {
	t.Helper()
	w, err := New(testDebounce)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

// waitForBatch blocks for the next batch, failing rather than hanging.
func waitForBatch(t *testing.T, w *Watcher) []Event {
	t.Helper()
	select {
	case batch := <-w.Batches():
		return batch
	case <-time.After(2 * time.Second):
		t.Fatal("no batch arrived")
		return nil
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherReportsAWrite(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	writeFile(t, file, "one")

	w := newTestWatcher(t)
	w.AddDir(dir)

	writeFile(t, file, "two")

	batch := waitForBatch(t, w)
	if len(batch) == 0 {
		t.Fatal("empty batch")
	}
	if batch[0].Path != file {
		t.Errorf("path = %q, want %q", batch[0].Path, file)
	}
	if batch[0].At.IsZero() {
		t.Error("event has no timestamp")
	}
}

// A burst of activity is one refresh, not one per event: that is the whole
// point of the debounce.
func TestWatcherCoalescesABurst(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(t)
	w.AddDir(dir)

	for i := range 10 {
		writeFile(t, filepath.Join(dir, "f"+string(rune('a'+i))+".txt"), "x")
	}

	batch := waitForBatch(t, w)
	if len(batch) < 2 {
		t.Fatalf("burst produced %d events in the first batch, want them coalesced", len(batch))
	}

	// And nothing more is waiting.
	select {
	case extra := <-w.Batches():
		t.Errorf("a second batch arrived with %d events", len(extra))
	case <-time.After(10 * testDebounce):
	}
}

func TestWatcherClassifiesOps(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(t)
	w.AddDir(dir)

	file := filepath.Join(dir, "a.txt")
	writeFile(t, file, "one")
	if got := opsFor(waitForBatch(t, w), file); !hasOp(got, OpCreate) {
		t.Errorf("creating a file gave %v, want a create", got)
	}

	writeFile(t, file, "two")
	if got := opsFor(waitForBatch(t, w), file); !hasOp(got, OpWrite) {
		t.Errorf("writing a file gave %v, want a write", got)
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if got := opsFor(waitForBatch(t, w), file); !hasOp(got, OpRemove) {
		t.Errorf("removing a file gave %v, want a remove", got)
	}
}

// A tree is watched all the way down, including directories that appear after
// the watch is set up, which is what an agent scaffolding a package does.
func TestWatcherFollowsNewSubdirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "existing"), 0o755); err != nil {
		t.Fatal(err)
	}

	w := newTestWatcher(t)
	w.AddTree(root, nil)

	writeFile(t, filepath.Join(root, "existing", "a.txt"), "x")
	if batch := waitForBatch(t, w); len(batch) == 0 {
		t.Fatal("no event from an existing subdirectory")
	}

	fresh := filepath.Join(root, "fresh", "deeper")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatal(err)
	}
	waitForBatch(t, w) // the directory creation itself

	writeFile(t, filepath.Join(fresh, "b.txt"), "x")
	batch := waitForBatch(t, w)
	if !containsPath(batch, filepath.Join(fresh, "b.txt")) {
		t.Errorf("a file in a newly created directory was not reported: %v", pathsOf(batch))
	}
}

func TestWatcherSkipsWhatItIsTold(t *testing.T) {
	root := t.TempDir()
	skipped := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(skipped, 0o755); err != nil {
		t.Fatal(err)
	}

	w := newTestWatcher(t)
	w.AddTree(root, func(dir string) bool {
		return filepath.Base(dir) == "node_modules"
	})

	writeFile(t, filepath.Join(skipped, "junk.js"), "x")
	select {
	case batch := <-w.Batches():
		t.Fatalf("a skipped directory produced events: %v", pathsOf(batch))
	case <-time.After(10 * testDebounce):
	}

	// The rest of the tree still works.
	writeFile(t, filepath.Join(root, "real.txt"), "x")
	if batch := waitForBatch(t, w); !containsPath(batch, filepath.Join(root, "real.txt")) {
		t.Errorf("the unskipped tree stopped reporting: %v", pathsOf(batch))
	}
}

// The transcript folder and the oob namespaces may not exist yet.
func TestWatcherIgnoresMissingDirectories(t *testing.T) {
	w := newTestWatcher(t)
	w.AddDir(filepath.Join(t.TempDir(), "not-created-yet"))
	w.AddDir("")

	if w.Skipped() != 0 {
		t.Errorf("skipped = %d, want 0", w.Skipped())
	}
}

func TestWatcherCloseIsIdempotent(t *testing.T) {
	w, err := New(testDebounce)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("closing twice errored: %v", err)
	}
}

// Helpers.

func opsFor(batch []Event, path string) []Op {
	var out []Op
	for _, e := range batch {
		if e.Path == path {
			out = append(out, e.Op)
		}
	}
	return out
}

func hasOp(ops []Op, want Op) bool {
	for _, o := range ops {
		if o == want {
			return true
		}
	}
	return false
}

func containsPath(batch []Event, path string) bool {
	for _, e := range batch {
		if e.Path == path {
			return true
		}
	}
	return false
}

func pathsOf(batch []Event) string {
	var out []string
	for _, e := range batch {
		out = append(out, string(e.Op)+" "+e.Path)
	}
	return strings.Join(out, ", ")
}
