#!/usr/bin/env bash
#
# Build pook and install it onto your PATH.
#
#   ./install.sh                    # install to ~/.local/bin
#   PREFIX=/usr/local/bin ./install.sh
#   ./install.sh --test             # run the test suite first
#
set -euo pipefail

PREFIX="${PREFIX:-$HOME/.local/bin}"
BIN="pook"

# Work from the repo root, so the script can be run from anywhere.
cd "$(dirname "${BASH_SOURCE[0]}")"

run_tests=false
for arg in "$@"; do
	case "$arg" in
	--test | -t) run_tests=true ;;
	--help | -h)
		sed -n '3,8p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "install.sh: unknown option $arg" >&2
		exit 1
		;;
	esac
done

if ! command -v go >/dev/null 2>&1; then
	echo "install.sh: go is not on PATH (asdf shim missing, or no version set)" >&2
	exit 1
fi

if $run_tests; then
	echo "running tests"
	go test ./...
fi

echo "building $BIN"
go build -o "$BIN" ./cmd/pook

mkdir -p "$PREFIX"
install -m 0755 "$BIN" "$PREFIX/$BIN"
echo "installed $PREFIX/$BIN"

# A binary somewhere off PATH is an install that looks like it worked and did
# not, so say so plainly rather than leaving it to be discovered later.
case ":$PATH:" in
*":$PREFIX:"*) ;;
*)
	echo
	echo "warning: $PREFIX is not on your PATH."
	echo "  add it with: export PATH=\"$PREFIX:\$PATH\""
	;;
esac

# Report what will actually be found, which is not necessarily what was just
# installed if an older copy sits earlier on PATH.
hash -r 2>/dev/null || true
found="$(command -v "$BIN" || true)"
if [ -n "$found" ] && [ "$found" != "$PREFIX/$BIN" ]; then
	echo
	echo "warning: '$BIN' on your PATH resolves to $found, not the copy just installed."
fi
