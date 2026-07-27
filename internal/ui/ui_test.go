package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/dougmartin/pook-cli/internal/git"
)

// Golden frames are compared as plain text: color is forced off so a failing
// diff is readable, and so the goldens do not churn when a palette changes.
// Color and contrast are on the spec's human-review list for that reason.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)

	// The oob tab only exists when there is an oob home, so the suite points
	// at one it created. Otherwise whether the tab is present would depend on
	// the machine the tests run on. The path is fixed rather than random
	// because the oob tab renders it, and a golden frame cannot hold a name
	// that changes every run.
	home := filepath.Join(os.TempDir(), "pook-oob-test-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		panic(err)
	}
	os.Setenv("OOB_HOME", home)

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// withoutOOB points the oob home at a path that does not exist, so a model
// built inside the test has no oob tab.
func withoutOOB(t *testing.T) {
	t.Helper()
	t.Setenv("OOB_HOME", filepath.Join(t.TempDir(), "absent"))
}

const (
	testWidth  = 80
	testHeight = 24
)

var testNow = time.Date(2026, 7, 26, 13, 45, 0, 0, time.UTC)

func fixedClock() time.Time { return testNow }

// gitRepoForTest is a repo value with no side effects: phase 0 only reads its
// path, and never runs git.
func gitRepoForTest() git.Repo {
	return git.Repo{Root: "/home/doug/projects/pook-cli"}
}

// newTestModel is a shell sized to a standard terminal with a stopped clock.
func newTestModel(t *testing.T) Model {
	t.Helper()
	m := New(gitRepoForTest(), nil, nil, nil).WithClock(fixedClock)
	return apply(m, tea.WindowSizeMsg{Width: testWidth, Height: testHeight})
}

// apply runs messages through the model in order, the way the program would.
func apply(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// applyCmd is apply for a single message, keeping the returned command so a
// test can assert on what the model asked the runtime to do.
func applyCmd(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

var namedKeys = map[string]tea.KeyType{
	"ctrl+d":    tea.KeyCtrlD,
	"ctrl+u":    tea.KeyCtrlU,
	"ctrl+s":    tea.KeyCtrlS,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"home":      tea.KeyHome,
	"end":       tea.KeyEnd,
	"backspace": tea.KeyBackspace,
	"pgup":      tea.KeyPgUp,
	"pgdown":    tea.KeyPgDown,
	"tab":       tea.KeyTab,
	"shift+tab": tea.KeyShiftTab,
	"esc":       tea.KeyEsc,
	"enter":     tea.KeyEnter,
	"space":     tea.KeySpace,
	"ctrl+c":    tea.KeyCtrlC,
}

// key builds the KeyMsg a terminal would produce for a key name.
func key(s string) tea.KeyMsg {
	if t, ok := namedKeys[s]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// press sends a sequence of keys.
func press(m Model, names ...string) Model {
	for _, n := range names {
		m = apply(m, key(n))
	}
	return m
}

// isQuit reports whether a command is tea.Quit, by running it.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// requireFrameSize asserts the layout invariant every frame must hold: exactly
// the terminal's height in rows, and no row wider than the terminal. This is
// what keeps the status bar pinned to the last line.
func requireFrameSize(t *testing.T, frame string, width, height int) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if len(lines) != height {
		t.Fatalf("frame has %d lines, want %d", len(lines), height)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line %d is %d cells wide, want at most %d: %q", i, w, width, line)
		}
	}
}

// execCommand builds a git invocation against a directory.
func execCommand(t *testing.T, dir string, args ...string) *exec.Cmd {
	t.Helper()
	return exec.Command("git", append([]string{"-C", dir}, args...)...)
}

// writeTestFile writes a file under dir, creating parents.
func writeTestFile(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stripStyles removes ANSI sequences so a test can assert on text alone.
func stripStyles(s string) string { return ansi.Strip(s) }
