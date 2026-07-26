package clip

import (
	"encoding/base64"
	"runtime"
	"strings"
	"testing"
)

// With no display there is nothing local to talk to, which is the state pook
// is in over ssh.
func TestNoToolsWithoutADisplay(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS always has pbcopy")
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")

	if got := tools(); len(got) != 0 {
		t.Errorf("tools = %v, want none without a display", got)
	}
	if CanRead() {
		t.Error("CanRead is true with no display")
	}
}

func TestX11ToolsAreOfferedWithADisplay(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS uses pbcopy")
	}
	t.Setenv("DISPLAY", ":0")
	t.Setenv("WAYLAND_DISPLAY", "")

	var names []string
	for _, tl := range tools() {
		names = append(names, tl.name)
	}
	if len(names) != 2 || names[0] != "xclip" || names[1] != "xsel" {
		t.Errorf("tools = %v, want [xclip xsel]", names)
	}
}

// Wayland is preferred when both are advertised, since an X11 helper under
// Wayland talks to XWayland's clipboard rather than the session's.
func TestWaylandIsPreferred(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS uses pbcopy")
	}
	t.Setenv("DISPLAY", ":0")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	got := tools()
	if len(got) == 0 || got[0].name != "wl-copy" {
		t.Errorf("first tool = %v, want wl-copy", got)
	}
}

// The OSC 52 fallback is what carries text to the machine the user is sitting
// at when pook runs over ssh.
func TestOSC52CarriesTheTextBase64Encoded(t *testing.T) {
	seq := osc52Sequence("hello over ssh")

	if !strings.HasPrefix(seq, "\x1b]52;c;") {
		t.Errorf("sequence = %q, want an OSC 52 system-clipboard prefix", seq)
	}
	want := base64.StdEncoding.EncodeToString([]byte("hello over ssh"))
	if !strings.Contains(seq, want) {
		t.Errorf("sequence = %q, want it to carry %q", seq, want)
	}
}

func TestOSC52HandlesEmptyText(t *testing.T) {
	if got := osc52Sequence(""); !strings.HasPrefix(got, "\x1b]52;c;") {
		t.Errorf("sequence = %q", got)
	}
}
