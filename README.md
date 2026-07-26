# pook

A terminal UI for watching what an AI coding agent is doing to your repo while it works.

`pook` shows uncommitted changes, the branch's commits, the live Claude Code session, and the
repo's out-of-band files as top/htop-style tabs, with heartbeat, activity, and watched-path
cues in a persistent status bar so you can tell at a glance whether the agent is still working,
done, or stuck.

It is a Go port of the [babysit-agent](../babysit-agent) VS Code extension, rebuilt as a single
static binary with no editor dependency, so it also works over ssh.

**Status: phase 0 of 7.** The shell is up: tab bar, status bar, help overlay, and the key
routing everything else hangs off. Every pane is a placeholder until its phase lands. See
[specs/pook-tui-port.md](specs/pook-tui-port.md) for the full design, keymap, phase plan, and
the exact behaviors the port has to reproduce.

## Build

```bash
go build ./cmd/pook   # produces ./pook
go test ./...         # golden frames and key routing
go test ./... -update # regenerate the golden frames after a deliberate layout change
```

## Prerequisites

- Go, pinned in `.tool-versions` for asdf
- `tmux`, used as the second verification harness for real terminal output. Not yet installed.
