package git

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// parentSearchDepth is how far back in HEAD's history to look for the
	// branch it was forked from.
	parentSearchDepth = 2000

	// maxRangeCommits caps a branch's own commit list; maxRecentCommits is
	// what gets listed when no base diverges, for example on main.
	maxRangeCommits  = 200
	maxRecentCommits = 30
)

// CommitEntry is one commit on the current branch.
type CommitEntry struct {
	Hash      string
	Short     string
	Subject   string
	Time      time.Time
	Additions int
	Deletions int
	FileCount int
}

// BranchInfo is the Branch tab's overview and commit list.
type BranchInfo struct {
	Name string
	// BaseRef is the branch this one is stacked on, empty when none diverges.
	BaseRef        string
	Commits        []CommitEntry
	TotalAdditions int
	TotalDeletions int
	FilesTouched   int
}

// CommitFileDiff is one file's patch within a commit, loaded lazily when a
// commit is expanded.
type CommitFileDiff struct {
	Path      string
	Additions int
	Deletions int
	Binary    bool
	Diff      string
}

// baseCandidates are tried in order when no local branch is an ancestor of
// HEAD.
var baseCandidates = []string{"@{upstream}", "origin/main", "origin/master", "main", "master"}

// CollectBranch returns the commits unique to the current branch: everything
// since the merge-base with its base, which is the nearest branch it is
// stacked on, else the first resolvable of upstream, main or master. When no
// base diverges (sitting on main, say) it falls back to the last 30 commits.
func (r Repo) CollectBranch() (BranchInfo, error) {
	name := r.branchName()

	head, err := r.runStrict("rev-parse", "--verify", "HEAD")
	if err != nil {
		return BranchInfo{Name: name}, nil // no commits yet
	}
	head = strings.TrimSpace(head)

	baseRef, rng := r.baseRange(name, head)

	args := []string{"log", "--format=%x01%H%x00%h%x00%s%x00%at", "--numstat", "-M"}
	if rng != "" {
		args = append(args, "-n", strconv.Itoa(maxRangeCommits), rng)
	} else {
		args = append(args, "-n", strconv.Itoa(maxRecentCommits))
	}
	out, err := r.run(args...)
	if err != nil {
		return BranchInfo{Name: name}, err
	}

	info := parseLog(out)
	info.Name = name
	info.BaseRef = baseRef
	return info, nil
}

// branchName is the current branch, or "HEAD" when detached.
//
// One deviation from the original: on a repo with no commits, rev-parse fails
// and the original fell back to the literal "HEAD". A fresh repo is a state
// pook gets pointed at, so branch --show-current, which does answer on an
// unborn branch, is tried before giving up. It returns empty when detached,
// which is the case the "HEAD" default is actually for.
func (r Repo) branchName() string {
	if out, err := r.runStrict("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		if name := strings.TrimSpace(out); name != "" && name != "HEAD" {
			return name
		}
	}
	if out, err := r.runStrict("branch", "--show-current"); err == nil {
		if name := strings.TrimSpace(out); name != "" {
			return name
		}
	}
	return "HEAD"
}

// baseRange resolves the base branch and the commit range unique to HEAD.
func (r Repo) baseRange(name, head string) (baseRef, rng string) {
	if ref, mergeBase, ok := r.nearestParentBranch(name); ok {
		return ref, mergeBase + "..HEAD"
	}

	for _, cand := range baseCandidates {
		out, err := r.runStrict("merge-base", "HEAD", cand)
		if err != nil {
			continue // that ref does not exist here
		}
		if mb := strings.TrimSpace(out); mb != "" && mb != head {
			return cand, mb + "..HEAD"
		}
	}
	return "", ""
}

// nearestParentBranch is the branch this one is stacked on: the nearest other
// branch tip in HEAD's own history.
//
// Git records no parent branch, so this is inferred, but it is what makes a
// branch stacked on another show only its own commits instead of repeating the
// stack's. A branch just cut from another sits at HEAD and is still the right
// base: the new branch then correctly reports zero commits of its own.
//
// Walking rev-list once, newest first, finds the nearest tip without a
// merge-base call per branch, which matters in repos with hundreds of them.
//
// Remote-tracking branches are scanned alongside local ones, which the
// original did not do. Without them a stale local branch that was merged long
// ago still sits in HEAD's history and gets mistaken for the parent, so a
// branch cut from origin/master reports every commit back to whenever that
// old branch was merged. The remote tip is usually the nearer and truer
// answer. Local refs sort first, so where both point at the same commit the
// shorter local name is the one shown.
func (r Repo) nearestParentBranch(current string) (ref, mergeBase string, ok bool) {
	out, err := r.runAll(
		[]string{"for-each-ref", "--format=%(objectname) %(refname)", "refs/heads", "refs/remotes"},
		[]string{"rev-list", "-n", strconv.Itoa(parentSearchDepth), "HEAD"},
	)
	if err != nil {
		return "", "", false
	}

	tips := map[string]string{} // commit -> branch at that commit
	for _, line := range strings.Split(out[0], "\n") {
		sha, full, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found {
			continue
		}
		name, usable := branchRefName(full, current)
		if !usable {
			continue
		}
		if _, dup := tips[sha]; dup {
			continue
		}
		tips[sha] = name
	}
	if len(tips) == 0 {
		return "", "", false
	}

	// rev-list only walks ancestors, so the first tip it hits is the nearest.
	for _, sha := range strings.Split(out[1], "\n") {
		sha = strings.TrimSpace(sha)
		if name, hit := tips[sha]; hit {
			return name, sha, true
		}
	}
	return "", "", false
}

// branchRefName reduces a full ref to the name worth showing, rejecting the
// ones that cannot be a parent of the current branch.
func branchRefName(full, current string) (string, bool) {
	switch {
	case strings.HasPrefix(full, "refs/heads/"):
		name := strings.TrimPrefix(full, "refs/heads/")
		return name, name != "" && name != current

	case strings.HasPrefix(full, "refs/remotes/"):
		name := strings.TrimPrefix(full, "refs/remotes/")
		if name == "" {
			return "", false
		}
		// A remote's HEAD is a symbolic duplicate of its default branch under
		// a name nobody wants to read.
		if strings.HasSuffix(name, "/HEAD") {
			return "", false
		}
		// This branch pushed is this same branch, not something it sits on.
		// Using it would reduce the view to whatever is unpushed.
		if _, branch, found := strings.Cut(name, "/"); found && branch == current {
			return "", false
		}
		return name, true
	}
	return "", false
}

// logNumstatRe matches a --numstat row inside a log block.
var logNumstatRe = regexp.MustCompile(`^(-|\d+)\t(-|\d+)\t(.+)$`)

// parseLog reads the \x01-delimited log blocks into commits and totals.
func parseLog(out string) BranchInfo {
	var info BranchInfo
	paths := map[string]bool{}

	for _, block := range strings.Split(out, "\x01") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		header, body, _ := strings.Cut(block, "\n")

		fields := strings.Split(header, "\x00")
		hash, short, subject, at := field(fields, 0), field(fields, 1), field(fields, 2), field(fields, 3)
		if hash == "" {
			continue
		}
		if short == "" {
			short = hash[:min(7, len(hash))]
		}

		c := CommitEntry{
			Hash:    hash,
			Short:   short,
			Subject: subject,
			Time:    unixSeconds(at),
		}
		for _, line := range strings.Split(body, "\n") {
			m := logNumstatRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			c.Additions += atoiDash(m[1])
			c.Deletions += atoiDash(m[2])
			c.FileCount++
			paths[m[3]] = true
		}

		info.Commits = append(info.Commits, c)
		info.TotalAdditions += c.Additions
		info.TotalDeletions += c.Deletions
	}

	info.FilesTouched = len(paths)
	return info
}

// field is fields[i] when it exists, empty otherwise, so a short header parses
// the way the original's destructuring did.
func field(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return ""
}

// unixSeconds parses a %at timestamp; an unparseable one becomes the epoch,
// matching the original's `|| 0`.
func unixSeconds(s string) time.Time {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		n = 0
	}
	return time.Unix(n, 0)
}

// CommitDiff is the per-file patch set for one commit. Commits are immutable,
// so the Branch tab caches this permanently once loaded.
func (r Repo) CommitDiff(hash string) ([]CommitFileDiff, error) {
	out, err := r.runAll(
		[]string{"show", "--numstat", "-z", "--format=", "-M", hash},
		[]string{"show", "--patch", "--format=", "-M", hash},
	)
	if err != nil {
		return nil, err
	}

	counted := parseNumstat(out[0])
	diffs := parseDiffs(out[1])

	files := make([]CommitFileDiff, 0, counted.len())
	counted.all(func(p string, c counts) {
		diff, _ := diffs.get(p)
		files = append(files, CommitFileDiff{
			Path:      p,
			Additions: c.additions,
			Deletions: c.deletions,
			Binary:    c.binary,
			Diff:      truncateDiff(diff),
		})
	})
	return files, nil
}
