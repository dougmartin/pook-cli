# pook: Go TUI Port of babysit-agent

**Status**: **Open**

## Overview

`pook` is a terminal UI for watching what an AI coding agent is doing to your repo while it
works. It is a port of the `babysit-agent` VS Code extension (`~/projects/babysit-agent`) from
a TypeScript webview to a single Go binary.

The motivation is independence: no VS Code, no webview, one static binary that runs over ssh.
The extension's four stacked accordion sections become **top/htop-style tabs with one pane
visible at a time**, which is the correct shape for a terminal and which removes the most
expensive and most bug-prone part of the port (multi-pane layout, resizable dividers, and
cross-pane focus routing).

The ambient "is the agent still working" signals do not live in the panes. They live in the
persistent header and status bar, so showing one pane at a time costs nothing that matters.

Binary name: `pook`. Module path: `github.com/dougmartin/pook-cli`.

## Requirements

### Shell

- Full-screen alternate-buffer TUI. Launches with no arguments and operates on the git repo
  containing the current working directory. Exits with a clear message if there is no repo.
- **Tab bar** across the top listing the five tabs, active tab highlighted, each tab carrying a
  badge: a change count on Changes, and an activity dot on Branch / Session / oob when new data
  arrives while that tab is not focused. Badges clear when the tab is viewed.
- **Status bar** across the bottom, always visible regardless of tab: the watched-file alert,
  the heartbeat (`last change Ns ago · <path>`), and a key hint.
- Keyboard only. No mouse support at all, including no mouse-drag, no click targets, and no
  scroll-wheel handling.
- A `?` help overlay listing every binding, since the keymap is the only way in.

### Tab 1: Changes

Every file differing from `HEAD` (staged, unstaged, and untracked).

- Rows show relative path, a status badge (new / changed / deleted), and a `+added -deleted`
  summary. Expanding a row reveals a colored unified inline diff.
- **Filters**: All (default), New, Changed, Deleted, plus a path text filter.
- **Sort**: Latest change (default), Oldest change, A-Z, Most changes.
- A marker on files whose diff changed since the user last expanded them.
- **Discard** a file (revert to `HEAD`, delete if untracked) behind a confirmation prompt.
- **Open in editor**: launch `$EDITOR` on the file, at the cursor's diff line where known.
- **Mark reviewed**: snapshot current state; a "since mark" filter then shows only files changed
  after the mark.

### Tab 2: Branch

- Overview line: branch name, commits ahead of base, files touched, total `+/-`.
- Commit list, latest first. Expanding a commit lazily loads its per-file diffs and caches them
  permanently, since commits are immutable.
- Commits newer than the review mark are flagged.

### Tab 3: Session

Live view of the most recent Claude Code chat for this folder.

- One message at a time with a position indicator and a scrubber over the message range.
- When positioned at the newest message, the view follows new messages live and shows a `live`
  badge. Moving backwards anchors on a historical message and stops following.
- Single-step previous/next, plus jump to previous/next **user** message (disabled when there is
  none in that direction).
- Assistant and user text renders as markdown. Tool calls render as tool name plus a one-line
  input summary. Thinking blocks render dimmed.
- Copy the raw message text to the system clipboard.
- Switches automatically when a new session starts.

### Tab 4: oob files

- Lists [out-of-band files](https://github.com/dougmartin/oob) for this repo, read from
  `$OOB_HOME` (default `~/oob`): branch namespace first, then repo, then global, each under a
  namespace header. Groups are always shown even when empty.
- Reuses the Changes accordion viewer, but content is plain (no diff coloring).
- Open the real file in `$EDITOR`.
- oob directories are watched, so files written via the oob MCP server appear live.

### Tab 5: Prompts

Reusable prompt library, stored as `prompts.json` in the user config dir and shared across all
running instances (the file is watched, so an edit in one instance appears in the others).

- List with search filter. Copy a prompt to the clipboard, edit title and text in place, delete
  behind confirmation, append a new prompt, and reorder with move-up / move-down keys.
- **Placeholders**: a prompt may contain `{{name}}` tokens. Copying such a prompt asks for a
  value per distinct token, in order of first appearance, repeated tokens asked once. Cancelling
  any input aborts the copy and leaves the clipboard untouched.
- **Import** from markdown: `#`/`##` headings become titles, each blank-line-separated paragraph
  becomes its own prompt (multi-paragraph headings get `(2)`, `(3)` suffixes).
- **Export** to markdown in the same format. Import and export round-trip.
- Import and export take a typed path, since there is no native file dialog.

### Clipboard modal

- Overlay showing the current clipboard in an editable text area. Save writes it back; Esc
  closes.
- Copying a prompt containing an inline all-caps `[MARKER]` opens the modal automatically with
  the first marker **selected**, so typing replaces it. Tab selects the next marker, wrapping.
- This requires a text area with a selection model, which `bubbles/textarea` does not provide.
  See Technical Notes.

### Watching and alerting

- Watch the working tree, `.git`, the Claude transcript dir, the oob namespace dirs, and the
  prompt library file. Coalesce events on a 300ms debounce.
- **Activity ticker**: rolling log of the last 50 events (created / modified / deleted files,
  commits) with timestamps, reachable from the status bar.
- **Watched paths**: files matching configured globs get a marker and raise a status-bar alert.
- **Idle notification**: after activity has been observed, N minutes of silence raises an in-app
  banner ("no activity for Nm, agent done or stuck?"). Fires once per quiet stretch and re-arms
  when work resumes.
- **Sound**: a configurable shell command played on the idle notification, and optionally when a
  watched path is touched. Failures are logged, never surfaced.

### Configuration

VS Code settings become a config file (`$XDG_CONFIG_HOME/pook/config.toml`), with the same
defaults as the extension:

| Key | Default |
|---|---|
| `watched_globs` | `.env*`, `**/package.json`, `**/package-lock.json`, `**/*.lock`, `**/yarn.lock`, `.github/**`, `**/migrations/**` |
| `idle_notify_minutes` | `0` (disabled) |
| `sound_command` | `""` (disabled) |
| `sound_on_watched` | `false` |

### Keymap

Global:

```
1-5            select tab            tab / shift-tab   cycle tabs
a              activity ticker       c                 clipboard modal
?              help                  q                 quit
r              force refresh
```

Within a list tab (Changes, Branch, oob, Prompts):

```
j / k          move cursor           g / G             top / bottom
space / enter  expand / collapse     zR / zM           expand / collapse all
/              path or text filter   esc               clear filter
o              open in $EDITOR
```

Changes only: `f` cycle status filter, `s` cycle sort, `d` discard, `m` mark reviewed,
`M` toggle since-mark.

Session only: `h` / `l` previous / next message, `H` / `L` previous / next user message,
`$` jump to live, `y` copy raw message.

Prompts only: `enter` copy, `e` edit, `n` new, `d` delete, `J` / `K` reorder, `i` import,
`x` export.

Arrow keys are bound alongside `hjkl` throughout.

## Technical Notes

### Stack

- `charmbracelet/bubbletea` (Elm architecture), `lipgloss` (styling), `bubbles` (viewport,
  textinput, textarea).
- `charmbracelet/glamour` for markdown rendering in the Session tab.
- `fsnotify/fsnotify` for file watching.
- `atotto/clipboard` or equivalent for system clipboard access.
- Shell out to the `git` binary via `os/exec`. **Do not use go-git.** The existing TypeScript
  parses `git` output directly, so shelling out keeps the port line-for-line verifiable against
  the original.

### Architecture

Single-pane-at-a-time makes the update loop trivial, and this should be preserved deliberately:

```
Update(msg):
    if modal != nil        -> route to modal
    else if overlay != nil -> route to overlay (help, ticker)
    else if global key     -> handle
    else                   -> route to tabs[active]
```

Each tab is its own model with its own cursor and scroll offset. Tabs never reflow against each
other. Pane height is `terminalHeight - tabBar - statusBar`, one subtraction, which avoids the
Lipgloss height-math bugs that dominate multi-pane layouts.

Accordions are **not** nested components. Flatten expanded content into a single virtual line
list and slice the visible window by cursor and offset. Bubbletea re-renders the entire frame
each update, so handing a viewport a 10,000-line diff is a performance trap.

### Data layer parity

The four backend modules port near-mechanically. Behaviors that must be reproduced exactly:

- **Base branch resolution** (`src/git.ts`, `collectBranch`): the base is the nearest *other*
  local branch tip in `HEAD`'s own history, so a branch stacked on another shows only its own
  commits. When no branch is an ancestor of `HEAD`, fall back in order to `@{upstream}`,
  `origin/main`, `origin/master`, `main`, `master`, and to the last 30 commits when nothing
  diverges (for example when sitting on `main`). Commit range is capped at 200.
- **Empty repo**: `collectChanges` diffs against the empty-tree hash when `HEAD` does not
  resolve.
- **Transcript discovery** (`src/claude.ts`): the folder is `~/.claude/projects/<root>` where
  `<root>` is the absolute repo path with every non-alphanumeric character replaced by `-`.
  Pick the newest `*.jsonl` by mtime. `SessionTail` reads incrementally from a byte offset and
  keeps at most 500 messages, advancing `startIndex` as it drops old ones. Message text is
  truncated at 4000 chars.
- **oob layout** (`src/oob.ts`): `global/`, `repos/<repo>/`, `branches/<repo>/<branch...>`,
  where a branch containing slashes nests as directories. Dotfiles are skipped. Files are binary
  if a NUL byte appears in the first 8000 bytes; content is truncated at 200,000 chars.
- **Prompt parsing** (`src/prompts.ts`): token regex `\{\{([^{}]*)\}\}`; inline marker regex
  `\[[A-Z][A-Z0-9 _-]*\](?!\()`, where the negative lookahead exists so markdown links are not
  treated as markers. Go's RE2 has **no lookahead**, so this needs rewriting as a match plus an
  explicit check of the following byte.
- Debounce is 300ms, ticker keeps 50 events.

Port the three existing test files (`test/git.test.js`, `test/oob.test.js`,
`test/prompts.test.js`) to Go table tests first. They are the specification for the parsing.

### Verification strategy

This matters more than usual, because the author cannot see a terminal.

1. **`charmbracelet/x/exp/teatest`** drives the model in-process, sends synthetic key messages,
   and captures rendered frames as strings for golden-file comparison. Because the app is
   keyboard-only, *every* interaction is reachable this way. Golden files are the primary
   regression net for layout math, cursor movement, scroll offsets, and key routing.
2. **tmux** provides the second loop: run the real binary detached, `send-keys`, then
   `capture-pane -p` to read actual terminal output. This catches what teatest cannot, namely
   real terminal rendering and resize. **`tmux` is not currently installed and needs to be.**
3. Human review remains necessary for: color and contrast choices, whether the tab activity dots
   are noticeable enough, and true-color versus 256-color degradation.

Also required before starting: a Go version selected via asdf. `go` is on PATH as an asdf shim
but no version is set.

### Phase plan

| # | Scope | Confidence |
|---|---|---|
| 0 | `go.mod`, teatest harness, tab shell, status bar | high |
| 1 | git backend + ported tests | high |
| 2 | claude / oob / prompts backends + tests | high |
| 3 | watchers, debounce, ticker, idle, watched globs | high |
| 4 | Changes tab | high |
| 5 | Branch + oob tabs, Session tab | high |
| 6 | Prompts tab | high |
| 7 | Clipboard modal + selection editor | **low** |

Phases 0-5 deliver a complete read-only agent monitor, which is the core value. Phases 6-7 are
separable and can be deferred until the tool has seen real use.

Phase 7 is the one genuinely risky piece. `bubbles/textarea` has no selection model, so the
`[MARKER]` select-and-replace behavior requires either patching the component or writing a
multiline editor with a selection range. Expect subtle cursor bugs that golden tests will not
catch unless the failing cases are written deliberately.

## Out of Scope

- Mouse support of any kind.
- Multi-pane layout, resizable dividers, and collapsible sections. Superseded by tabs.
- Drag-to-reorder. Superseded by move-up / move-down keys.
- Native file dialogs for prompt import and export. Paths are typed.
- OS-level desktop notifications. The idle warning is an in-app banner plus the configured sound
  command.
- Multi-repo or multi-workspace views. One repo per process.
- Any write access to the repo beyond the existing discard-file action.
