// Package oob lists out-of-band files: the cross-repo and cross-branch scratch
// files kept outside any git working tree. It is a port of src/oob.ts, and
// mirrors the layout the oob MCP server writes.
package oob

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// maxContentChars caps a file's content so one huge note cannot stall the
// renderer.
const maxContentChars = 200_000

// binarySniffBytes is how much of a file is examined for a NUL byte.
const binarySniffBytes = 8000

// Namespace is one of the three scopes an oob file can live in.
type Namespace string

const (
	NamespaceBranch Namespace = "branch"
	NamespaceRepo   Namespace = "repo"
	NamespaceGlobal Namespace = "global"
)

// File is one oob file.
type File struct {
	// Path is namespace-relative and slash-separated, e.g. "notes/todo.md".
	Path      string
	Namespace Namespace
	// FSPath is the absolute path on disk, for opening in an editor.
	FSPath    string
	ModTime   int64
	Content   string
	Binary    bool
	Truncated bool
}

// Group is one namespace and the files in it.
type Group struct {
	Namespace Namespace
	// Label is the header shown above the group.
	Label string
	Dir   string
	Files []File
}

// Home is the oob root, honoring OOB_HOME exactly as the server does.
func Home() string {
	if env := strings.TrimSpace(os.Getenv("OOB_HOME")); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "oob"
	}
	return filepath.Join(home, "oob")
}

// Available reports whether an oob home exists on this machine.
//
// pook hides the oob tab entirely when it does not: oob is a separate tool,
// and a permanently empty tab is worse than no tab.
func Available() bool {
	info, err := os.Stat(Home())
	return err == nil && info.IsDir()
}

// NamespaceDirs resolves the three namespace directories for a repo and
// branch: global, repos/<repo>, branches/<repo>/<branch...>, where a branch
// containing slashes nests as directories. The branch group is omitted when no
// branch is known.
func NamespaceDirs(home, repo, branch string) []Group {
	var groups []Group
	if branch != "" {
		parts := append([]string{home, "branches", repo}, strings.Split(branch, "/")...)
		groups = append(groups, Group{
			Namespace: NamespaceBranch,
			Label:     "branch: " + branch,
			Dir:       filepath.Join(parts...),
		})
	}
	return append(groups,
		Group{
			Namespace: NamespaceRepo,
			Label:     "repo: " + repo,
			Dir:       filepath.Join(home, "repos", repo),
		},
		Group{
			Namespace: NamespaceGlobal,
			Label:     "global",
			Dir:       filepath.Join(home, "global"),
		},
	)
}

// Collect lists the oob files for a repo: branch first, then repo, then
// global. Groups are always returned, in that order and even when empty, so
// the tab can show every namespace.
func Collect(repo, branch string) []Group {
	groups := NamespaceDirs(Home(), repo, branch)
	for i, g := range groups {
		for _, rel := range listFiles(g.Dir) {
			groups[i].Files = append(groups[i].Files, readFile(filepath.Join(g.Dir, rel), g.Namespace, rel))
		}
	}
	return groups
}

// listFiles walks dir recursively and returns slash-separated relative paths,
// sorted A to Z. A missing directory is empty, not an error: a namespace with
// nothing in it is a normal state.
func listFiles(dir string) []string {
	var out []string

	var walk func(abs, rel string)
	walk = func(abs, rel string) {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return
		}
		for _, e := range entries {
			// Dotfiles and dot directories are skipped, which keeps a stray
			// .git out of the listing.
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			child := e.Name()
			if rel != "" {
				child = rel + "/" + e.Name()
			}
			switch {
			case e.IsDir():
				walk(filepath.Join(abs, e.Name()), child)
			case e.Type().IsRegular():
				out = append(out, child)
			}
		}
	}
	walk(dir, "")

	slices.Sort(out)
	return out
}

// readFile loads one oob file. An unreadable file still appears in the
// listing, just with no content.
func readFile(fsPath string, ns Namespace, rel string) File {
	f := File{Path: rel, Namespace: ns, FSPath: fsPath}

	info, err := os.Stat(fsPath)
	if err != nil {
		return f
	}
	f.ModTime = info.ModTime().UnixMilli()

	buf, err := os.ReadFile(fsPath)
	if err != nil {
		return f
	}

	// A NUL byte early in the file means it is not text worth rendering.
	sniff := buf
	if len(sniff) > binarySniffBytes {
		sniff = sniff[:binarySniffBytes]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		f.Binary = true
		return f
	}

	content := []rune(string(buf))
	if len(content) > maxContentChars {
		content = content[:maxContentChars]
		f.Truncated = true
	}
	f.Content = string(content)
	return f
}
