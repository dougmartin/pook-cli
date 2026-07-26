// Package git wraps the git binary. The TypeScript original parses git's
// plumbing output directly, so pook shells out too: it keeps the port
// verifiable line-for-line against src/git.ts.
package git

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoRepo is returned when dir is not inside a git working tree.
var ErrNoRepo = errors.New("not inside a git repository")

// Repo identifies the working tree pook is watching. One repo per process.
type Repo struct {
	// Root is the absolute path of the working tree root.
	Root string
}

// Name is the last path element of the root, used wherever a repo needs a
// short label (the oob repo namespace, window titles).
func (r Repo) Name() string { return filepath.Base(r.Root) }

// Discover finds the working tree containing dir.
func Discover(dir string) (Repo, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return Repo{}, ErrNoRepo
		}
		return Repo{}, err
	}

	root := strings.TrimSpace(string(out))
	if root == "" {
		return Repo{}, ErrNoRepo
	}
	return Repo{Root: root}, nil
}
