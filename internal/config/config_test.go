package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// pook is meant to run with no setup at all.
func TestLoadWithNoFileReturnsTheDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("a missing config errored: %v", err)
	}
	if !slices.Equal(cfg.WatchedGlobs, Default().WatchedGlobs) {
		t.Errorf("globs = %v, want the defaults", cfg.WatchedGlobs)
	}
	if cfg.IdleNotifyMinutes != 0 || cfg.SoundCommand != "" || cfg.SoundOnWatched {
		t.Errorf("defaults are not off: %+v", cfg)
	}
}

func TestLoadReadsEveryKey(t *testing.T) {
	path := writeConfig(t, `
watched_globs = ["*.sql", "infra/**"]
idle_notify_minutes = 15
sound_command = "paplay /usr/share/sounds/bell.oga"
sound_on_watched = true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"*.sql", "infra/**"}; !slices.Equal(cfg.WatchedGlobs, want) {
		t.Errorf("globs = %v, want %v", cfg.WatchedGlobs, want)
	}
	if cfg.IdleNotifyMinutes != 15 {
		t.Errorf("idle = %d, want 15", cfg.IdleNotifyMinutes)
	}
	if want := "paplay /usr/share/sounds/bell.oga"; cfg.SoundCommand != want {
		t.Errorf("sound = %q, want %q", cfg.SoundCommand, want)
	}
	if !cfg.SoundOnWatched {
		t.Error("sound_on_watched did not take")
	}
}

// A config that sets one key keeps the defaults for the rest.
func TestLoadLeavesAbsentKeysAtTheirDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, "idle_notify_minutes = 3\n"))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.IdleNotifyMinutes != 3 {
		t.Errorf("idle = %d, want 3", cfg.IdleNotifyMinutes)
	}
	if !slices.Equal(cfg.WatchedGlobs, Default().WatchedGlobs) {
		t.Errorf("globs = %v, want the defaults", cfg.WatchedGlobs)
	}
}

// An explicitly empty list means "flag nothing", not "use the defaults".
func TestLoadHonorsAnEmptyGlobList(t *testing.T) {
	cfg, err := Load(writeConfig(t, "watched_globs = []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WatchedGlobs) != 0 {
		t.Errorf("globs = %v, want none", cfg.WatchedGlobs)
	}
}

func TestLoadReportsABrokenFile(t *testing.T) {
	cfg, err := Load(writeConfig(t, "watched_globs = [unclosed\n"))
	if err == nil {
		t.Fatal("a malformed config did not error")
	}
	// And the caller still gets something usable to run with.
	if !slices.Equal(cfg.WatchedGlobs, Default().WatchedGlobs) {
		t.Errorf("globs = %v, want the defaults after a parse failure", cfg.WatchedGlobs)
	}
}

func TestDefaultPathIsUnderTheConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/home/tester/.config")

	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/home/tester/.config/pook/config.toml"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}
