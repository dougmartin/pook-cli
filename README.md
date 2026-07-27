# pook

A terminal UI for watching what an AI coding agent is doing to your repo while it works.

`pook` shows uncommitted changes, the branch's commits, a commit graph back through main, the
live Claude Code session, and your prompt library as top/htop-style tabs, with heartbeat,
activity, and watched-path cues in a persistent status bar so you can tell at a glance whether
the agent is still working, done, or stuck. A sixth tab lists the repo's
[out-of-band files](https://github.com/dougmartin/oob), and appears only when an oob home
exists.

It is a Go port of the [babysit-agent](https://github.com/dougmartin/babysit-agent) VS Code
extension, rebuilt as a single static binary with no editor dependency, so it also works over
ssh.

**Status:** feature complete against [the spec](specs/pook-tui-port.md), which carries the full
design, the keymap, and the exact behaviors the port reproduces.

## Install

```bash
git clone https://github.com/dougmartin/pook-cli
cd pook-cli
./install.sh                     # build and install to ~/.local/bin
./install.sh --test              # run the suite first
PREFIX=/usr/local/bin ./install.sh
```

It warns if the target is not on your PATH, or if an older `pook` earlier on PATH would win.

Or with the Go toolchain directly:

```bash
go install github.com/dougmartin/pook-cli/cmd/pook@latest
```

## Usage

Run `pook` anywhere inside a git repo. It takes no arguments and watches the repo containing the
working directory.

Press `?` for the full keymap. The essentials: `1`-`6` select a tab, left/right or tab/shift-tab
cycle them, `a` opens the activity ticker, `c` opens the clipboard editor, `r` forces a refresh,
`q` quits.

It is keyboard only. There is no mouse support of any kind, by design.

## Configuration

Optional, at `$XDG_CONFIG_HOME/pook/config.toml`. Every key has a default, so no file is needed.

| Key | Default | Meaning |
|---|---|---|
| `watched_globs` | `.env*`, `**/package.json`, `**/package-lock.json`, `**/*.lock`, `**/yarn.lock`, `.github/**`, `**/migrations/**` | Paths that raise a status bar alert when touched |
| `idle_notify_minutes` | `0` (off) | Raise a banner after this much silence |
| `sound_command` | `""` (off) | Shell command played on the idle warning |
| `sound_on_watched` | `false` | Also play it when a watched path is touched |

The prompt library lives beside it at `prompts.json`, and is shared by every running instance.

## Requirements

- Go 1.25 or later to build. The repo pins a version in `.tool-versions` for asdf.
- `git` on your PATH. pook shells out to it rather than reimplementing it.
- Optional, for reading the clipboard back: `xclip`, `xsel` or `wl-clipboard`. Without one, pook
  still writes the clipboard using OSC 52, which is what works over ssh, but cannot read it.

## Development

```bash
go build ./cmd/pook   # produces ./pook
go test ./...         # golden frames, key routing and the ported backend tests
go test ./... -update # regenerate the golden frames after a deliberate layout change
```

The golden frames render with color stripped, so a failing diff stays readable and the palette
can change without churning them. `tmux` is used as a second harness for checking real terminal
output, including resize, which the in-process tests cannot cover.
