package git

import "strings"

// IgnoredDirs lists the directories git ignores, relative to the root and
// slash-separated with no trailing slash.
//
// The watcher uses this to avoid descending into node_modules and friends: a
// tree that git will never report on is not worth an inotify watch.
func (r Repo) IgnoredDirs() []string {
	// --directory collapses a wholly ignored directory to a single entry
	// ending in a slash, so this stays cheap even when the tree is enormous.
	out, err := r.run("ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil
	}

	var dirs []string
	for _, entry := range strings.Split(out, "\x00") {
		if strings.HasSuffix(entry, "/") {
			dirs = append(dirs, strings.TrimSuffix(entry, "/"))
		}
	}
	return dirs
}
