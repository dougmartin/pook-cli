package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/monitor"
)

// sampleEvents is a ticker with one of each kind, including a watched path.
func sampleEvents() []monitor.Event {
	return []monitor.Event{
		{At: testNow.Add(-5 * time.Minute), Kind: monitor.EventCommit, Text: `commit a1b2c3d "port the git backend"`},
		{At: testNow.Add(-90 * time.Second), Kind: monitor.EventCreated, Text: "created internal/ui/app.go (+210 -0)", Path: "internal/ui/app.go"},
		{At: testNow.Add(-30 * time.Second), Kind: monitor.EventModified, Text: "modified package.json (+2 -1)", Path: "package.json", Watched: true},
		{At: testNow.Add(-3 * time.Second), Kind: monitor.EventDeleted, Text: "deleted old.go (+0 -40)", Path: "old.go"},
	}
}

func TestFrameTickerOverlayWithEvents(t *testing.T) {
	m := apply(newTestModel(t), refreshedMsg{Events: sampleEvents()})

	frame := press(m, "a").View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

func TestFrameIdleBanner(t *testing.T) {
	m := apply(newTestModel(t), idleMsg{Text: "no activity for 5m, agent done or stuck?"})

	frame := m.View()
	requireFrameSize(t, frame, testWidth, testHeight)
	golden.RequireEqual(t, []byte(frame))
}

// The banner costs a row, and the pane gives it up rather than the status bar
// sliding off the bottom.
func TestBannerTakesItsRowFromThePane(t *testing.T) {
	m := newTestModel(t)
	before := m.paneHeight()

	m = apply(m, idleMsg{Text: "quiet"})
	if got := m.paneHeight(); got != before-1 {
		t.Errorf("pane height = %d, want %d", got, before-1)
	}
	requireFrameSize(t, m.View(), testWidth, testHeight)
}

func TestBannerClearsWhenWorkResumes(t *testing.T) {
	m := apply(newTestModel(t), idleMsg{Text: "quiet"})
	if m.banner == "" {
		t.Fatal("the banner did not appear")
	}

	// A refresh that saw nothing new leaves it up.
	m = apply(m, refreshedMsg{})
	if m.banner == "" {
		t.Fatal("an empty refresh cleared the banner")
	}

	m = apply(m, refreshedMsg{Snap: monitor.Snapshot{FilesChanged: true}})
	if m.banner != "" {
		t.Errorf("banner = %q, want it cleared once work resumed", m.banner)
	}
}

func TestBannerIsDismissedByEsc(t *testing.T) {
	m := apply(newTestModel(t), idleMsg{Text: "quiet"})

	m = press(m, "esc")
	if m.banner != "" {
		t.Errorf("banner = %q, want esc to dismiss it", m.banner)
	}
}

// esc only claims the banner when there is one; otherwise it belongs to the
// active tab, which uses it to clear a filter.
func TestEscReachesTheTabWhenThereIsNoBanner(t *testing.T) {
	var seen []string
	m := newTestModel(t)
	m.tabs[TabChanges] = keyProbe{seen: &seen}

	m = press(m, "esc")
	if len(seen) != 1 || seen[0] != "esc" {
		t.Errorf("tab saw %v, want [esc]", seen)
	}
}

func TestWatchedAlert(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{name: "nothing watched", paths: nil, want: ""},
		{name: "one path", paths: []string{"package.json"}, want: "watched: package.json"},
		{
			name:  "several are summarized",
			paths: []string{".env", "package.json", "yarn.lock"},
			want:  "watched: .env +2 more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := watchedAlert(tt.paths); got != tt.want {
				t.Errorf("watchedAlert(%v) = %q, want %q", tt.paths, got, tt.want)
			}
		})
	}
}

// A refresh raises dots on the tabs whose data changed, except the one being
// looked at.
func TestSnapshotRaisesTabDots(t *testing.T) {
	m := apply(newTestModel(t), refreshedMsg{
		Snap: monitor.Snapshot{FilesChanged: true, BranchChanged: true, OOBChanged: true},
	})

	if m.tabs[TabChanges].Badge().Dot {
		t.Error("the visible Changes tab raised a dot")
	}
	for _, tab := range []int{TabBranch, TabOOB} {
		if !m.tabs[tab].Badge().Dot {
			t.Errorf("tab %d did not raise a dot", tab)
		}
	}
	// Session activity is the transcript's business, not the repo's.
	if m.tabs[TabSession].Badge().Dot {
		t.Error("the Session tab raised a dot from a repo refresh")
	}
}

// The Changes badge counts the files in the snapshot.
func TestChangesBadgeCountsFiles(t *testing.T) {
	m := apply(newTestModel(t), refreshedMsg{
		Snap: monitor.Snapshot{Files: make([]git.FileEntry, 3)},
	})

	if got := m.tabs[TabChanges].Badge().Count; got != 3 {
		t.Errorf("badge count = %d, want 3", got)
	}
	if !strings.Contains(m.View(), "Changes 3") {
		t.Errorf("the tab bar does not show the count:\n%s", m.View())
	}
}

func TestTickerOverlayWithNoEvents(t *testing.T) {
	frame := press(newTestModel(t), "a").View()

	if !strings.Contains(frame, "nothing has happened yet") {
		t.Errorf("an empty ticker does not say so:\n%s", frame)
	}
}

func TestFormatAgo(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{d: -time.Second, want: "0s"},
		{d: 0, want: "0s"},
		{d: 45 * time.Second, want: "45s"},
		{d: 59*time.Second + 999*time.Millisecond, want: "59s"},
		{d: time.Minute, want: "1m"},
		{d: 90 * time.Minute, want: "1h"},
		{d: 26 * time.Hour, want: "26h"},
	}

	for _, tt := range tests {
		if got := formatAgo(tt.d); got != tt.want {
			t.Errorf("formatAgo(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// The tabs are divided, so two inactive ones cannot run together.
func TestTabBarDividers(t *testing.T) {
	bar := stripStyles(strings.SplitN(newTestModel(t).View(), "\n", 2)[0])

	want := "1 Changes | 2 Branch | 3 Session | 4 Prompts | 5 oob"
	if !strings.Contains(bar, want) {
		t.Errorf("tab bar = %q, want it to contain %q", bar, want)
	}

	// Not before the first tab, and not trailing after the last.
	if strings.Contains(bar, "pook  |") {
		t.Errorf("a divider sits before the first tab: %q", bar)
	}
	if strings.Contains(strings.TrimRight(bar, " "), "oob |") {
		t.Errorf("a divider trails the last tab: %q", bar)
	}
}

// With a tab hidden the dividers still fall between what is left.
func TestTabBarDividersWithoutTheOOBTab(t *testing.T) {
	withoutOOB(t)

	m := New(gitRepoForTest(), nil, nil, nil).WithClock(fixedClock)
	m = apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})

	bar := stripStyles(strings.SplitN(m.View(), "\n", 2)[0])
	want := "1 Changes | 2 Branch | 3 Session | 4 Prompts"
	if !strings.Contains(bar, want) {
		t.Errorf("tab bar = %q, want it to contain %q", bar, want)
	}
	if strings.Contains(strings.TrimRight(bar, " "), "Prompts |") {
		t.Errorf("a divider trails the last tab: %q", bar)
	}
}
