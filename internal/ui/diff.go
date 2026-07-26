package ui

import (
	"regexp"
	"strconv"
	"strings"
)

// colorizeDiff renders a unified diff into styled lines.
//
// This runs once per refresh when rows are built, never per frame: a large
// diff is expensive to style and the result does not change between renders.
func colorizeDiff(diff string) []string {
	if diff == "" {
		return nil
	}

	raw := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		out = append(out, styleDiffLine(line))
	}
	return out
}

func styleDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "@@"):
		return styleHunk.Render(line)
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return styleDim.Render(line)
	case strings.HasPrefix(line, "diff --git"),
		strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "new file"),
		strings.HasPrefix(line, "deleted file"),
		strings.HasPrefix(line, "similarity index"),
		strings.HasPrefix(line, "rename "),
		strings.HasPrefix(line, "Binary files"):
		return styleDim.Render(line)
	case strings.HasPrefix(line, "+"):
		return styleAdded.Render(line)
	case strings.HasPrefix(line, "-"):
		return styleRemoved.Render(line)
	default:
		return styleContext.Render(line)
	}
}

// hunkRe captures the post-image start line of a hunk: @@ -a,b +c,d @@.
var hunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// diffLineNumber is the file line the cursor is pointing at, walking back to
// the enclosing hunk header and counting the lines that exist in the new file.
//
// It returns 0 when the position is not inside a hunk, which is what "open at
// the top of the file" means to the caller.
func diffLineNumber(diff []string, idx int) int {
	if idx < 0 || idx >= len(diff) {
		return 0
	}

	start := -1
	for i := idx; i >= 0; i-- {
		if m := hunkRe.FindStringSubmatch(diff[i]); m != nil {
			start = i
			break
		}
	}
	if start < 0 {
		return 0
	}

	m := hunkRe.FindStringSubmatch(diff[start])
	line, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}

	// Count forward from the hunk header. A removed line is not in the new
	// file, so it does not advance the counter.
	for i := start + 1; i <= idx; i++ {
		if strings.HasPrefix(diff[i], "-") {
			continue
		}
		if i < idx {
			line++
		}
	}
	return line
}
