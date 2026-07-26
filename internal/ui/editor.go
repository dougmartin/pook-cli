package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// editorFailedMsg reports that the editor could not be launched or exited
// badly. It is shown in the tab's footer rather than taking pook down.
type editorFailedMsg struct{ Err error }

// openInEditor suspends the TUI, runs $EDITOR on a file, and restores the
// screen afterwards.
//
// tea.ExecProcess is what makes this safe: bubbletea releases the terminal
// while the editor owns it, then takes it back.
func openInEditor(root, rel string, line int) tea.Cmd {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("VISUAL"))
	}
	if editor == "" {
		return func() tea.Msg {
			return editorFailedMsg{Err: errNoEditor}
		}
	}

	path := filepath.Join(root, rel)
	name, args := editorCommand(editor, path, line)

	return tea.ExecProcess(exec.Command(name, args...), func(err error) tea.Msg {
		if err != nil {
			return editorFailedMsg{Err: err}
		}
		// The file may have been edited, so the view is stale.
		return needsRefreshMsg{}
	})
}

// errNoEditor is returned when neither EDITOR nor VISUAL is set.
var errNoEditor = errNoEditorType{}

type errNoEditorType struct{}

func (errNoEditorType) Error() string { return "$EDITOR is not set" }

// editorCommand builds the argv for an editor, jumping to a line where the
// editor has a syntax for it.
//
// $EDITOR may carry its own flags ("code -w"), so it is split before the
// editor is identified.
func editorCommand(editor, path string, line int) (string, []string) {
	fields := strings.Fields(editor)
	name := fields[0]
	args := append([]string(nil), fields[1:]...)

	if line <= 0 {
		return name, append(args, path)
	}

	switch filepath.Base(name) {
	case "vi", "vim", "nvim", "gvim", "nano", "pico", "emacs", "emacsclient", "kak", "helix", "hx":
		// +N filename is the shared convention among terminal editors.
		return name, append(args, "+"+strconv.Itoa(line), path)
	case "code", "code-insiders", "codium", "cursor", "windsurf":
		return name, append(args, "--goto", path+":"+strconv.Itoa(line))
	case "subl", "sublime_text":
		return name, append(args, path+":"+strconv.Itoa(line))
	case "idea", "goland", "webstorm", "pycharm":
		return name, append(args, "--line", strconv.Itoa(line), path)
	default:
		// An unknown editor gets the file and nothing it might not parse.
		return name, append(args, path)
	}
}
