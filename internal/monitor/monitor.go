// Package monitor turns repo state into the signals pook shows: the activity
// ticker, the heartbeat, the watched-path alert and the idle warning. It is
// the extension's ChangesPanel bookkeeping without the webview.
package monitor

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dougmartin/pook-cli/internal/config"
	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/oob"
	"github.com/dougmartin/pook-cli/internal/watch"
)

// maxEvents is how many ticker entries are kept.
const maxEvents = 50

// maxCommitEvents caps how many commits one refresh can report, so a rebase
// does not flood the ticker.
const maxCommitEvents = 10

// EventKind is what a ticker entry describes.
type EventKind string

const (
	EventCreated  EventKind = "created"
	EventModified EventKind = "modified"
	EventDeleted  EventKind = "deleted"
	EventCommit   EventKind = "commit"
)

// Event is one entry in the activity ticker.
type Event struct {
	At   time.Time
	Kind EventKind
	Text string
	// Path is set for file events, empty for commits.
	Path    string
	Watched bool
}

// Snapshot is everything one refresh produced.
type Snapshot struct {
	Files  []git.FileEntry
	Branch git.BranchInfo
	OOB    []oob.Group

	// Changed reports which views have new data, so the tab bar can raise an
	// activity dot on the ones the user is not looking at.
	FilesChanged  bool
	BranchChanged bool
	OOBChanged    bool

	// WatchedPaths are the changed files matching a configured glob.
	WatchedPaths []string

	// Err is set when the refresh failed. The previous snapshot stays on
	// screen rather than the view emptying out.
	Err error
}

// Activity is the most recent thing pook saw happen.
type Activity struct {
	At   time.Time
	Text string
}

// Monitor holds the state one refresh has to compare against the next.
//
// Refreshes run off the UI goroutine while the view reads the ticker, so every
// exported method takes the lock.
type Monitor struct {
	mu sync.Mutex

	repo    git.Repo
	cfg     config.Config
	matcher watch.Matcher

	prevDiffs map[string]string
	prevHead  string
	headSeen  bool
	prevOOB   string
	oobSeen   bool

	events       []Event
	lastActivity *Activity
	idleFired    bool

	// now and play are injectable so tests need neither a clock nor a
	// speaker.
	now  func() time.Time
	play func(command string)
}

// New builds a monitor for a repo.
func New(repo git.Repo, cfg config.Config) *Monitor {
	return &Monitor{
		repo:    repo,
		cfg:     cfg,
		matcher: watch.NewMatcher(cfg.WatchedGlobs),
		now:     time.Now,
		play:    playSound,
	}
}

// Config is the configuration in force.
func (m *Monitor) Config() config.Config { return m.cfg }

// Events is a copy of the ticker, oldest first.
func (m *Monitor) Events() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event(nil), m.events...)
}

// LastActivity is the heartbeat: the most recent change pook noticed.
func (m *Monitor) LastActivity() (Activity, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastActivity == nil {
		return Activity{}, false
	}
	return *m.lastActivity, true
}

// Refresh re-reads the repo and folds the result into the ticker.
func (m *Monitor) Refresh() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	files, err := m.repo.CollectChanges()
	if err != nil {
		return Snapshot{Err: err}
	}
	branch, err := m.repo.CollectBranch()
	if err != nil {
		return Snapshot{Err: err}
	}

	var watched []string
	for i := range files {
		files[i].Watched = m.matcher.Match(files[i].Path)
		if files[i].Watched {
			watched = append(watched, files[i].Path)
		}
	}

	groups := oob.Collect(filepath.Base(m.repo.Root), branch.Name)

	snap := Snapshot{
		Files:        files,
		Branch:       branch,
		OOB:          groups,
		WatchedPaths: watched,
	}
	snap.FilesChanged, snap.BranchChanged = m.computeEvents(files, branch)
	snap.OOBChanged = m.oobChanged(groups)
	return snap
}

// computeEvents diffs this refresh against the last one and appends whatever
// is new to the ticker.
//
// Events come from comparing state rather than from raw filesystem
// notifications: that is what makes "modified" mean the file's diff actually
// changed, not merely that something touched it.
func (m *Monitor) computeEvents(files []git.FileEntry, branch git.BranchInfo) (filesChanged, branchChanged bool) {
	now := m.now()
	var added []Event

	// The first refresh establishes the baseline; everything already present
	// is not news.
	if m.prevDiffs != nil {
		for _, f := range files {
			prev, seen := m.prevDiffs[f.Path]
			if seen && prev == f.Diff {
				continue
			}

			kind := EventModified
			if !seen {
				switch f.Status {
				case git.StatusNew:
					kind = EventCreated
				case git.StatusDeleted:
					kind = EventDeleted
				}
			}
			added = append(added, Event{
				At:      now,
				Kind:    kind,
				Text:    fmt.Sprintf("%s %s (+%d -%d)", kind, f.Path, f.Additions, f.Deletions),
				Path:    f.Path,
				Watched: f.Watched,
			})
		}
	}
	filesChanged = len(added) > 0

	next := make(map[string]string, len(files))
	for _, f := range files {
		next[f.Path] = f.Diff
	}
	m.prevDiffs = next

	var head string
	if len(branch.Commits) > 0 {
		head = branch.Commits[0].Hash
	}

	// headSeen rather than a non-empty prevHead, so the first commit on a
	// branch that had none is reported. The original compared against an
	// unset previous head and silently absorbed that commit into the
	// baseline, which loses the one commit event most worth seeing.
	if m.headSeen && head != "" && head != m.prevHead {
		var fresh []Event
		for _, c := range branch.Commits {
			if c.Hash == m.prevHead {
				break
			}
			fresh = append(fresh, Event{
				At:   now,
				Kind: EventCommit,
				Text: fmt.Sprintf("commit %s %q", c.Short, c.Subject),
			})
			if len(fresh) >= maxCommitEvents {
				break
			}
		}
		// The log is newest first, but the ticker reads oldest first.
		slicesReverse(fresh)
		added = append(added, fresh...)
		branchChanged = len(fresh) > 0
	}
	if head != "" {
		m.prevHead = head
	}
	m.headSeen = true

	if len(added) == 0 {
		return filesChanged, branchChanged
	}

	m.events = append(m.events, added...)
	if over := len(m.events) - maxEvents; over > 0 {
		m.events = append([]Event(nil), m.events[over:]...)
	}

	last := added[len(added)-1]
	text := last.Path
	if text == "" {
		text = last.Text
	}
	m.lastActivity = &Activity{At: last.At, Text: text}

	// Work resumed, so the idle warning re-arms.
	m.idleFired = false

	if m.cfg.SoundOnWatched && anyWatched(added) {
		m.play(m.cfg.SoundCommand)
	}
	return filesChanged, branchChanged
}

// oobChanged reports whether the oob files differ from the last refresh.
func (m *Monitor) oobChanged(groups []oob.Group) bool {
	var b strings.Builder
	for _, g := range groups {
		for _, f := range g.Files {
			fmt.Fprintf(&b, "%s\x00%s\x00%d\x00", g.Namespace, f.Path, f.ModTime)
		}
	}
	sig := b.String()

	// The first refresh is the baseline, not a change.
	if !m.oobSeen {
		m.oobSeen = true
		m.prevOOB = sig
		return false
	}

	changed := sig != m.prevOOB
	m.prevOOB = sig
	return changed
}

// CheckIdle reports whether the agent has just gone quiet for long enough to
// warrant the banner. It fires once per quiet stretch, and re-arms when work
// resumes.
func (m *Monitor) CheckIdle() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	minutes := m.cfg.IdleNotifyMinutes
	if minutes <= 0 || m.idleFired || m.lastActivity == nil {
		return "", false
	}
	if m.now().Sub(m.lastActivity.At) < time.Duration(minutes)*time.Minute {
		return "", false
	}

	m.idleFired = true
	m.play(m.cfg.SoundCommand)
	return fmt.Sprintf("no activity for %dm, agent done or stuck?", minutes), true
}

// IgnoredDirSkip is the predicate the watcher uses to stay out of directories
// git ignores.
func (m *Monitor) IgnoredDirSkip() func(dir string) bool {
	ignored := map[string]bool{}
	for _, d := range m.repo.IgnoredDirs() {
		ignored[filepath.Join(m.repo.Root, filepath.FromSlash(d))] = true
	}

	return func(dir string) bool {
		// .git is watched directly, not walked: its internals churn on every
		// git command and none of it is worth an event.
		if filepath.Base(dir) == ".git" {
			return true
		}
		return ignored[dir]
	}
}

func anyWatched(events []Event) bool {
	for _, e := range events {
		if e.Watched {
			return true
		}
	}
	return false
}

func slicesReverse(events []Event) {
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
}

// playSound runs the configured command through the shell and forgets about
// it. Failures are never surfaced: a missing audio player must not interrupt
// someone watching an agent work.
func playSound(command string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}

	cmd := exec.Command("/bin/sh", "-c", command)
	if err := cmd.Start(); err != nil {
		return
	}
	go cmd.Wait() // reap it without caring how it went
}
