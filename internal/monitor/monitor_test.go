package monitor

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/dougmartin/pook-cli/internal/config"
	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/watch"
)

var testNow = time.Date(2026, 7, 26, 13, 45, 0, 0, time.UTC)

// testMonitor is a monitor over a throwaway repo, with a clock that only moves
// when a test moves it and a speaker that only records.
type testMonitor struct {
	*Monitor
	root   string
	clock  time.Time
	played []string
}

func newTestMonitor(t *testing.T, cfg config.Config) *testMonitor {
	t.Helper()
	root := initRepo(t)

	tm := &testMonitor{root: root, clock: testNow}
	tm.Monitor = New(git.Repo{Root: root}, cfg)
	tm.Monitor.now = func() time.Time { return tm.clock }
	tm.Monitor.play = func(cmd string) { tm.played = append(tm.played, cmd) }
	return tm
}

func (tm *testMonitor) advance(d time.Duration) { tm.clock = tm.clock.Add(d) }

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "README.md", "hello\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "first")

	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func eventTexts(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Text
	}
	return out
}

// The first refresh is a baseline: everything already in the working tree is
// not news, or opening pook on a dirty repo would flood the ticker.
func TestFirstRefreshIsABaseline(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	writeFile(t, tm.root, "already-there.txt", "x")

	snap := tm.Refresh()
	if len(snap.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(snap.Files))
	}
	if len(tm.Events()) != 0 {
		t.Errorf("baseline produced ticker events: %v", eventTexts(tm.Events()))
	}
	if snap.FilesChanged {
		t.Error("baseline reported a change")
	}
	if _, ok := tm.LastActivity(); ok {
		t.Error("baseline set the heartbeat")
	}
}

func TestNewFileBecomesACreatedEvent(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	tm.Refresh()

	writeFile(t, tm.root, "fresh.txt", "hello\n")
	snap := tm.Refresh()

	if !snap.FilesChanged {
		t.Error("a new file did not report a change")
	}
	events := tm.Events()
	if len(events) != 1 {
		t.Fatalf("events = %v, want one", eventTexts(events))
	}
	if events[0].Kind != EventCreated {
		t.Errorf("kind = %s, want created", events[0].Kind)
	}
	if want := "created fresh.txt (+1 -0)"; events[0].Text != want {
		t.Errorf("text = %q, want %q", events[0].Text, want)
	}

	// And the heartbeat now points at that file.
	act, ok := tm.LastActivity()
	if !ok || act.Text != "fresh.txt" || !act.At.Equal(testNow) {
		t.Errorf("activity = %+v (ok=%v), want fresh.txt at %v", act, ok, testNow)
	}
}

func TestEditingATrackedFileBecomesAModifiedEvent(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	writeFile(t, tm.root, "README.md", "hello\nworld\n")
	tm.Refresh() // baseline with the edit already present

	writeFile(t, tm.root, "README.md", "hello\nworld\nmore\n")
	tm.Refresh()

	events := tm.Events()
	if len(events) != 1 || events[0].Kind != EventModified {
		t.Fatalf("events = %v, want one modified", eventTexts(events))
	}
}

// An unchanged file must not produce an event on every refresh, or the ticker
// fills with noise while the agent thinks.
func TestUnchangedFilesProduceNothing(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	writeFile(t, tm.root, "steady.txt", "x")
	tm.Refresh()

	for range 3 {
		tm.Refresh()
	}
	if len(tm.Events()) != 0 {
		t.Errorf("idle refreshes produced %v", eventTexts(tm.Events()))
	}
}

func TestCommitsBecomeEvents(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	runGit(t, tm.root, "checkout", "-q", "-b", "feature")
	tm.Refresh()

	writeFile(t, tm.root, "a.txt", "a\n")
	runGit(t, tm.root, "add", "-A")
	runGit(t, tm.root, "commit", "-q", "-m", "add a")

	snap := tm.Refresh()
	if !snap.BranchChanged {
		t.Error("a commit did not report a branch change")
	}

	var commits []Event
	for _, e := range tm.Events() {
		if e.Kind == EventCommit {
			commits = append(commits, e)
		}
	}
	if len(commits) != 1 {
		t.Fatalf("commit events = %v, want one", eventTexts(commits))
	}
	if want := `commit ` + snap.Branch.Commits[0].Short + ` "add a"`; commits[0].Text != want {
		t.Errorf("text = %q, want %q", commits[0].Text, want)
	}
}

func TestTickerKeepsOnlyTheMostRecentEvents(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	tm.Refresh()

	for i := range maxEvents + 10 {
		writeFile(t, tm.root, "f.txt", string(rune('a'+i%26))+"\n")
		tm.Refresh()
	}

	if len(tm.Events()) != maxEvents {
		t.Errorf("events = %d, want %d", len(tm.Events()), maxEvents)
	}
}

func TestWatchedPathsAreFlagged(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	writeFile(t, tm.root, "package.json", "{}\n")
	writeFile(t, tm.root, "ordinary.go", "package main\n")

	snap := tm.Refresh()
	if !slices.Equal(snap.WatchedPaths, []string{"package.json"}) {
		t.Errorf("watched = %v, want [package.json]", snap.WatchedPaths)
	}
	for _, f := range snap.Files {
		want := f.Path == "package.json"
		if f.Watched != want {
			t.Errorf("%s watched = %v, want %v", f.Path, f.Watched, want)
		}
	}
}

func TestSoundOnWatchedOnlyFiresForWatchedPaths(t *testing.T) {
	cfg := config.Default()
	cfg.SoundOnWatched = true
	cfg.SoundCommand = "beep"

	tm := newTestMonitor(t, cfg)
	tm.Refresh()

	writeFile(t, tm.root, "ordinary.go", "package main\n")
	tm.Refresh()
	if len(tm.played) != 0 {
		t.Errorf("an ordinary file played a sound: %v", tm.played)
	}

	writeFile(t, tm.root, "package.json", "{}\n")
	tm.Refresh()
	if !slices.Equal(tm.played, []string{"beep"}) {
		t.Errorf("played = %v, want [beep]", tm.played)
	}
}

func TestSoundStaysSilentWhenNotConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.SoundOnWatched = false
	cfg.SoundCommand = "beep"

	tm := newTestMonitor(t, cfg)
	tm.Refresh()
	writeFile(t, tm.root, "package.json", "{}\n")
	tm.Refresh()

	if len(tm.played) != 0 {
		t.Errorf("played = %v, want nothing", tm.played)
	}
}

func TestIdleNotification(t *testing.T) {
	cfg := config.Default()
	cfg.IdleNotifyMinutes = 5
	cfg.SoundCommand = "beep"

	tm := newTestMonitor(t, cfg)
	tm.Refresh()

	// Nothing has happened yet, so there is no silence to report.
	if _, fired := tm.CheckIdle(); fired {
		t.Fatal("idle fired before any activity was seen")
	}

	writeFile(t, tm.root, "a.txt", "x")
	tm.Refresh()

	tm.advance(4 * time.Minute)
	if _, fired := tm.CheckIdle(); fired {
		t.Fatal("idle fired early")
	}

	tm.advance(time.Minute)
	text, fired := tm.CheckIdle()
	if !fired {
		t.Fatal("idle did not fire after the configured silence")
	}
	if want := "no activity for 5m, agent done or stuck?"; text != want {
		t.Errorf("banner = %q, want %q", text, want)
	}
	if !slices.Equal(tm.played, []string{"beep"}) {
		t.Errorf("played = %v, want [beep]", tm.played)
	}

	// It fires once per quiet stretch, not on every check.
	tm.advance(time.Hour)
	if _, fired := tm.CheckIdle(); fired {
		t.Error("idle fired twice for one quiet stretch")
	}

	// Work resuming re-arms it.
	writeFile(t, tm.root, "b.txt", "x")
	tm.Refresh()
	tm.advance(5 * time.Minute)
	if _, fired := tm.CheckIdle(); !fired {
		t.Error("idle did not re-arm after work resumed")
	}
}

func TestIdleDisabledByDefault(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	tm.Refresh()
	writeFile(t, tm.root, "a.txt", "x")
	tm.Refresh()

	tm.advance(24 * time.Hour)
	if _, fired := tm.CheckIdle(); fired {
		t.Error("idle fired with the notification disabled")
	}
}

func TestOOBChangesAreReported(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OOB_HOME", home)

	tm := newTestMonitor(t, config.Default())
	snap := tm.Refresh()
	if snap.OOBChanged {
		t.Error("the baseline reported an oob change")
	}
	if len(snap.OOB) != 3 {
		t.Fatalf("oob groups = %d, want 3", len(snap.OOB))
	}

	writeFile(t, home, filepath.Join("global", "note.md"), "hello\n")
	if snap := tm.Refresh(); !snap.OOBChanged {
		t.Error("a new oob file was not reported")
	}
	if snap := tm.Refresh(); snap.OOBChanged {
		t.Error("an unchanged oob tree reported a change")
	}
}

// The watcher must not descend into directories git ignores.
func TestIgnoredDirSkip(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	writeFile(t, tm.root, ".gitignore", "node_modules/\n")
	writeFile(t, tm.root, filepath.Join("node_modules", "pkg", "index.js"), "x")
	writeFile(t, tm.root, filepath.Join("src", "main.go"), "package main\n")

	skip := tm.IgnoredDirSkip()

	if !skip(filepath.Join(tm.root, "node_modules")) {
		t.Error("an ignored directory was not skipped")
	}
	if !skip(filepath.Join(tm.root, ".git")) {
		t.Error(".git was not skipped")
	}
	if skip(filepath.Join(tm.root, "src")) {
		t.Error("a tracked directory was skipped")
	}
}

// The skip predicate and the matcher have to agree with what the watcher
// package expects.
func TestSkipPredicateWorksWithTheWatcher(t *testing.T) {
	tm := newTestMonitor(t, config.Default())
	writeFile(t, tm.root, ".gitignore", "ignored/\n")
	writeFile(t, tm.root, filepath.Join("ignored", "junk.txt"), "x")

	w, err := watch.New(20 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	w.AddTree(tm.root, tm.IgnoredDirSkip())

	writeFile(t, tm.root, filepath.Join("ignored", "more-junk.txt"), "x")
	select {
	case batch := <-w.Batches():
		t.Fatalf("an ignored directory produced %d events", len(batch))
	case <-time.After(200 * time.Millisecond):
	}
}
