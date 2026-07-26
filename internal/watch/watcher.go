package watch

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Debounce is how long activity must settle before a batch is emitted. An
// agent editing a file produces a burst of events; the whole burst should cost
// one refresh.
const Debounce = 300 * time.Millisecond

// maxWatchedDirs caps how many directories are registered. inotify has a
// per-user watch limit, and a repo with a huge unignored tree should degrade
// to watching less rather than failing to start. Skipped counts the overflow.
const maxWatchedDirs = 8192

// Op is what happened to a path.
type Op string

const (
	OpCreate Op = "created"
	OpWrite  Op = "modified"
	OpRemove Op = "deleted"
)

// Event is one coalesced filesystem change, with an absolute path.
type Event struct {
	Path string
	Op   Op
	At   time.Time
}

// Watcher reports debounced batches of filesystem activity.
type Watcher struct {
	fsw      *fsnotify.Watcher
	batches  chan []Event
	debounce time.Duration
	now      func() time.Time

	mu      sync.Mutex
	dirs    map[string]bool
	skipped int
	trees   []tree

	closeOnce sync.Once
	done      chan struct{}
}

// tree is a root being watched recursively, kept so directories created later
// can be added under it.
type tree struct {
	root string
	skip func(dir string) bool
}

// New starts a watcher. Close it when finished.
func New(debounce time.Duration) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		fsw:      fsw,
		batches:  make(chan []Event, 1),
		debounce: debounce,
		now:      time.Now,
		dirs:     map[string]bool{},
		done:     make(chan struct{}),
	}
	go w.run()
	return w, nil
}

// Batches yields coalesced activity, one batch per quiet period.
func (w *Watcher) Batches() <-chan []Event { return w.batches }

// Skipped is how many directories went unwatched at the limit.
func (w *Watcher) Skipped() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.skipped
}

// AddDir watches one directory, without descending into it. A directory that
// does not exist is not an error: the transcript folder and the oob namespaces
// are all created on first use.
func (w *Watcher) AddDir(dir string) {
	if dir == "" {
		return
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	w.add(dir)
}

// AddTree watches a directory and everything under it, skipping any directory
// skip returns true for. Directories created later are picked up as they
// appear.
func (w *Watcher) AddTree(root string, skip func(dir string) bool) {
	if skip == nil {
		skip = func(string) bool { return false }
	}

	w.mu.Lock()
	w.trees = append(w.trees, tree{root: root, skip: skip})
	w.mu.Unlock()

	w.walk(root, skip)
}

func (w *Watcher) walk(root string, skip func(dir string) bool) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: watch what we can
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && skip(path) {
			return filepath.SkipDir
		}
		w.add(path)
		return nil
	})
}

func (w *Watcher) add(dir string) {
	w.mu.Lock()
	if w.dirs[dir] {
		w.mu.Unlock()
		return
	}
	if len(w.dirs) >= maxWatchedDirs {
		w.skipped++
		w.mu.Unlock()
		return
	}
	w.dirs[dir] = true
	w.mu.Unlock()

	if err := w.fsw.Add(dir); err != nil {
		w.mu.Lock()
		delete(w.dirs, dir)
		w.mu.Unlock()
	}
}

// Close stops watching.
func (w *Watcher) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.done)
		err = w.fsw.Close()
	})
	return err
}

// run coalesces raw events into batches, emitting one once activity has been
// quiet for the debounce interval.
func (w *Watcher) run() {
	var pending []Event
	var timer *time.Timer
	var quiet <-chan time.Time

	// arm restarts the quiet period. A fresh timer each time keeps a value
	// left in an already fired timer's channel unreachable.
	arm := func() {
		if timer != nil {
			timer.Stop()
		}
		timer = time.NewTimer(w.debounce)
		quiet = timer.C
	}

	for {
		select {
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return

		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if e, keep := w.translate(ev); keep {
				pending = append(pending, e)
			}
			arm()

		case <-quiet:
			quiet = nil
			if len(pending) == 0 {
				continue
			}
			select {
			case w.batches <- pending:
				pending = nil
			default:
				// Nobody has read the last batch yet. Hold what we have and
				// try again after another quiet period, rather than dropping
				// activity or blocking the watcher.
				arm()
			}

		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// Watch errors are routine here: directories come and go while an
			// agent works. The next refresh re-reads the truth from git.
		}
	}
}

// translate converts an fsnotify event, and registers directories that appear
// under a watched tree so new subtrees are covered.
func (w *Watcher) translate(ev fsnotify.Event) (Event, bool) {
	e := Event{Path: ev.Name, At: w.now()}

	switch {
	case ev.Has(fsnotify.Create):
		e.Op = OpCreate
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			w.addNewTreeDir(ev.Name)
		}
	case ev.Has(fsnotify.Write):
		e.Op = OpWrite
	case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
		e.Op = OpRemove
	default:
		// Chmod and friends are not activity worth reporting.
		return Event{}, false
	}
	return e, true
}

// addNewTreeDir starts watching a directory created under one of the
// recursive roots.
func (w *Watcher) addNewTreeDir(dir string) {
	w.mu.Lock()
	trees := append([]tree(nil), w.trees...)
	w.mu.Unlock()

	for _, t := range trees {
		rel, err := filepath.Rel(t.root, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if t.skip(dir) {
			return
		}
		w.walk(dir, t.skip)
		return
	}
}
