// Package clip reads and writes the system clipboard.
//
// Two mechanisms, because pook is meant to run over ssh. A local helper
// (xclip, wl-copy, pbcopy) is used when there is a display to talk to, and is
// the only way to read the clipboard back. Otherwise text is written with the
// OSC 52 escape sequence, which the terminal emulator carries to the machine
// the user is actually sitting at. OSC 52 is write-only, and some terminals
// disable it, so it is the fallback rather than the default.
package clip

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// ErrNoReader is returned when nothing on this machine can read the clipboard.
var ErrNoReader = errors.New("no clipboard reader available (install xclip, xsel or wl-clipboard)")

// tool is one clipboard helper.
type tool struct {
	name      string
	writeArgs []string
	readArgs  []string
	// readName is the separate binary used for reading, when the tool has
	// one (wl-copy pairs with wl-paste).
	readName string
}

// tools are tried in order. The X11 and Wayland entries are only offered when
// their display is actually present, since the binaries can be installed on a
// machine with no session attached.
func tools() []tool {
	var out []tool

	if runtime.GOOS == "darwin" {
		return []tool{{name: "pbcopy", readName: "pbpaste"}}
	}

	if os.Getenv("WAYLAND_DISPLAY") != "" {
		out = append(out, tool{
			name:     "wl-copy",
			readName: "wl-paste",
			readArgs: []string{"--no-newline"},
		})
	}
	if os.Getenv("DISPLAY") != "" {
		out = append(out,
			tool{
				name:      "xclip",
				writeArgs: []string{"-selection", "clipboard"},
				readArgs:  []string{"-selection", "clipboard", "-o"},
			},
			tool{
				name:      "xsel",
				writeArgs: []string{"--clipboard", "--input"},
				readArgs:  []string{"--clipboard", "--output"},
			},
		)
	}
	return out
}

// writer is the first available helper that can write.
func writer() (tool, bool) {
	for _, t := range tools() {
		if _, err := exec.LookPath(t.name); err == nil {
			return t, true
		}
	}
	return tool{}, false
}

// reader is the first available helper that can read.
func reader() (tool, bool) {
	for _, t := range tools() {
		name := t.readName
		if name == "" {
			name = t.name
		}
		if _, err := exec.LookPath(name); err == nil {
			return t, true
		}
	}
	return tool{}, false
}

// osc52Sequence is the escape sequence that sets the terminal's clipboard.
func osc52Sequence(text string) string {
	return ansi.SetClipboard(ansi.SystemClipboard, text)
}

// Write puts text on the clipboard.
func Write(text string) error {
	if t, ok := writer(); ok {
		cmd := exec.Command(t.name, t.writeArgs...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
		// A helper that exists but failed still leaves OSC 52 worth trying.
	}
	return writeOSC52(text)
}

// CanRead reports whether the clipboard can be read back here. It is false
// over ssh with no local helper, where OSC 52 can only write.
func CanRead() bool {
	_, ok := reader()
	return ok
}

// Read returns the clipboard contents.
func Read() (string, error) {
	t, ok := reader()
	if !ok {
		return "", ErrNoReader
	}

	name := t.readName
	if name == "" {
		name = t.name
	}

	var buf bytes.Buffer
	cmd := exec.Command(name, t.readArgs...)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// writeOSC52 asks the terminal emulator to set its own clipboard, which is
// what puts text on the right machine when pook is running over ssh.
//
// It is written to the controlling terminal rather than stdout so a redirected
// stdout cannot swallow it. The sequence does not move the cursor, so it
// cannot disturb the frame bubbletea has drawn.
func writeOSC52(text string) error {
	seq := osc52Sequence(text)

	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		defer tty.Close()
		_, err = tty.WriteString(seq)
		return err
	}

	_, err := os.Stdout.WriteString(seq)
	return err
}
