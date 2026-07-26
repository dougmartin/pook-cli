package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmartin/pook-cli/internal/claude"
	"github.com/dougmartin/pook-cli/internal/config"
	"github.com/dougmartin/pook-cli/internal/git"
	"github.com/dougmartin/pook-cli/internal/monitor"
	"github.com/dougmartin/pook-cli/internal/oob"
	"github.com/dougmartin/pook-cli/internal/prompts"
	"github.com/dougmartin/pook-cli/internal/ui"
	"github.com/dougmartin/pook-cli/internal/watch"
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

	cfg := loadConfig()
	mon := monitor.New(repo, cfg)
	store := loadPromptStore()

	watcher, err := watch.New(watch.Debounce)
	if err != nil {
		return err
	}
	defer watcher.Close()
	addWatches(watcher, repo, mon)

	// Alternate buffer, and deliberately no mouse options: pook is
	// keyboard-only, so mouse reporting is never enabled.
	p := tea.NewProgram(ui.New(repo, mon, watcher, store), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// loadConfig reads config.toml, falling back to the defaults. A broken config
// is reported and then ignored: refusing to start a monitor over a stray
// character in a settings file would be the wrong trade.
func loadConfig() config.Config {
	path, err := config.DefaultPath()
	if err != nil {
		return config.Default()
	}

	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pook: ignoring %s: %v\n", path, err)
	}
	return cfg
}

// loadPromptStore opens the shared prompt library.
func loadPromptStore() *prompts.Store {
	path, err := prompts.DefaultPath()
	if err != nil {
		return nil
	}
	return prompts.NewStore(path)
}

// addWatches registers everything pook follows: the working tree, .git, the
// Claude transcript folder, the oob namespaces and the prompt library.
//
// Directories that do not exist yet are skipped here and picked up on a later
// refresh, which is how a transcript folder created after startup is caught.
func addWatches(w *watch.Watcher, repo git.Repo, mon *monitor.Monitor) {
	w.AddTree(repo.Root, mon.IgnoredDirSkip())

	// .git itself, not its contents: HEAD and the index moving is what
	// matters, and the object store churns constantly.
	w.AddDir(filepath.Join(repo.Root, ".git"))

	w.AddDir(claude.TranscriptDir(repo.Root))

	for _, g := range oob.NamespaceDirs(oob.Home(), repo.Name(), "") {
		w.AddDir(g.Dir)
	}

	// The library file is replaced by a rename on every write, so the
	// directory is what has to be watched.
	if path, err := prompts.DefaultPath(); err == nil {
		w.AddDir(filepath.Dir(path))
	}
}
