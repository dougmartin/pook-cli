# pook

A terminal UI for watching what an AI coding agent is doing to your repo while it works.

`pook` shows uncommitted changes, the branch's commits, the live Claude Code session, and the
repo's out-of-band files as top/htop-style tabs, with heartbeat, activity, and watched-path
cues in a persistent status bar so you can tell at a glance whether the agent is still working,
done, or stuck.

It is a Go port of the [babysit-agent](../babysit-agent) VS Code extension, rebuilt as a single
static binary with no editor dependency, so it also works over ssh.

**Status: all seven phases implemented.** See
[specs/pook-tui-port.md](specs/pook-tui-port.md) for the full design, the keymap, and the exact
behaviors the port reproduces.

## Usage

Run `pook` anywhere inside a git repo. It takes no arguments and watches the repo containing the
working directory.

Press `?` for the full keymap. The essentials: `1`-`5` select a tab, `a` opens the activity
ticker, `c` opens the clipboard editor, `r` forces a refresh, `q` quits.

## Configuration

Optional, at `$XDG_CONFIG_HOME/pook/config.toml`. Every key has a default, so no file is needed.

| Key | Default | Meaning |
|---|---|---|
| `watched_globs` | `.env*`, `**/package.json`, `**/package-lock.json`, `**/*.lock`, `**/yarn.lock`, `.github/**`, `**/migrations/**` | Paths that raise a status bar alert when touched |
| `idle_notify_minutes` | `0` (off) | Raise a banner after this much silence |
| `sound_command` | `""` (off) | Shell command played on the idle warning |
| `sound_on_watched` | `false` | Also play it when a watched path is touched |

The prompt library lives beside it at `prompts.json`, and is shared by every running instance.

## Build

```bash
go build ./cmd/pook   # produces ./pook
go test ./...         # golden frames, key routing and the ported backend tests
go test ./... -update # regenerate the golden frames after a deliberate layout change
```

## Prerequisites

- Go, pinned in `.tool-versions` for asdf
- `tmux`, used as the second verification harness for real terminal output
- For clipboard reading: `xclip`, `xsel` or `wl-clipboard`. Without one, pook still writes the
  clipboard over OSC 52, which is what works over ssh, but cannot read it back.
