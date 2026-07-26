// Package config loads pook's config.toml. The keys and defaults are the
// extension's VS Code settings, carried over unchanged.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the whole of pook's configuration.
type Config struct {
	// WatchedGlobs are paths worth flagging when an agent touches them.
	WatchedGlobs []string `toml:"watched_globs"`

	// IdleNotifyMinutes raises the "agent done or stuck?" banner after this
	// much silence. Zero disables it.
	IdleNotifyMinutes int `toml:"idle_notify_minutes"`

	// SoundCommand is run through the shell on the idle notification. Empty
	// disables it.
	SoundCommand string `toml:"sound_command"`

	// SoundOnWatched also plays the sound when a watched path is touched.
	SoundOnWatched bool `toml:"sound_on_watched"`
}

// Default is the configuration pook runs with when there is no config file.
func Default() Config {
	return Config{
		WatchedGlobs: []string{
			".env*",
			"**/package.json",
			"**/package-lock.json",
			"**/*.lock",
			"**/yarn.lock",
			".github/**",
			"**/migrations/**",
		},
		IdleNotifyMinutes: 0,
		SoundCommand:      "",
		SoundOnWatched:    false,
	}
}

// DefaultPath is $XDG_CONFIG_HOME/pook/config.toml.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pook", "config.toml"), nil
}

// Load reads a config file over the defaults. A missing file is not an error:
// pook is meant to run with no setup at all.
//
// Keys absent from the file keep their default, so a config naming only a
// sound command still gets the standard watched globs.
func Load(path string) (Config, error) {
	cfg := Default()

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	// Decoding over the defaults leaves absent keys alone, so a file naming
	// only a sound command keeps the standard globs, and an explicitly empty
	// watched_globs list still means "flag nothing".
	if _, err := toml.Decode(string(raw), &cfg); err != nil {
		return Default(), err
	}
	return cfg, nil
}
