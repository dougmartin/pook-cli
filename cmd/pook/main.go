package main

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pook:", err)
		os.Exit(1)
	}
}

func run() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	repo, err := git.Discover(cwd)
	if err != nil {
		if errors.Is(err, git.ErrNoRepo) {
			return fmt.Errorf("%s is not inside a git repository, nothing to watch", cwd)
		}
		return err
	}

	// Alternate buffer, and deliberately no mouse options: pook is
	// keyboard-only, so mouse reporting is never enabled.
	p := tea.NewProgram(ui.New(repo), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
