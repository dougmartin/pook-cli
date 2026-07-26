package git

import (
	"strconv"
	"strings"
)

// GraphDetail is how much text each commit row carries.
type GraphDetail int

const (
	// GraphSubject shows the hash, refs and subject line.
	GraphSubject GraphDetail = iota
	// GraphCompact shows only the hash and refs, so the lane structure is
	// not pushed rightwards by sixty characters of subject.
	GraphCompact
)

// MaxGraphCommits caps how far back the graph walks.
const MaxGraphCommits = 200

// GraphRow is one line of the rendered graph.
type GraphRow struct {
	// Hash is the commit this row describes, empty on the pure-graph
	// continuation rows git draws between commits.
	Hash string
	// Text is the line exactly as git drew it, lane colors included.
	Text string
}

// graphSentinel brackets the full hash that Graph smuggles into git's own
// output, so a row can be matched to its commit without parsing the graph.
const graphSentinel = "\x00"

// Graph renders recent history with git's own commit graph.
//
// The graph column is passed through rather than parsed. git draws the lanes
// and colors them, which is both far better than a hand rolled renderer and
// immune to the fact that --graph output is not a stable machine format. The
// only thing read out of it is the hash, and that arrives through a NUL
// delimited field this format puts there deliberately.
//
// It walks down through the merge-base into the base branch rather than
// stopping at it, which is what makes a stack of branches legible: you see
// your own commits and what the branch underneath has been doing.
func (r Repo) Graph(detail GraphDetail, limit int, color bool) ([]GraphRow, error) {
	format := "%x00%H%x00%C(auto)%h%d %s"
	if detail == GraphCompact {
		format = "%x00%H%x00%C(auto)%h%d"
	}

	// Color is forced because output is a pipe, not a terminal. It is turned
	// off when pook itself is rendering without color, which is what keeps
	// the golden frames readable.
	colorFlag := "--color=never"
	if color {
		colorFlag = "--color=always"
	}

	out, err := r.runStrict("log",
		"--graph",
		colorFlag,
		// Topological order keeps each branch's commits contiguous instead of
		// interleaving them by date, which is the whole point for a stack.
		"--topo-order",
		"--decorate=short",
		"-n", strconv.Itoa(limit),
		"--format="+format,
	)
	if err != nil {
		return nil, err // no commits yet, or not a repo
	}
	return parseGraph(out), nil
}

func parseGraph(out string) []GraphRow {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}

	lines := strings.Split(out, "\n")
	rows := make([]GraphRow, 0, len(lines))
	for _, line := range lines {
		hash, text := splitGraphLine(line)
		rows = append(rows, GraphRow{Hash: hash, Text: text})
	}
	return rows
}

// splitGraphLine pulls the sentinel-delimited hash out of a line and returns
// the line as it should be displayed.
func splitGraphLine(line string) (hash, text string) {
	start := strings.Index(line, graphSentinel)
	if start < 0 {
		return "", line // a continuation row: all graph, no commit
	}

	rest := line[start+len(graphSentinel):]
	end := strings.Index(rest, graphSentinel)
	if end < 0 {
		return "", line
	}
	return rest[:end], line[:start] + rest[end+len(graphSentinel):]
}
