package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDiscoverFindsTheWorkingTreeRoot(t *testing.T) {
	root := initRepo(t)

	sub := filepath.Join(root, "internal", "ui")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	repo, err := Discover(sub)
	if err != nil {
		t.Fatal(err)
	}

	// macOS puts temp dirs behind a symlink, so compare resolved paths.
	want := resolve(t, root)
	if got := resolve(t, repo.Root); got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
	if repo.Name() != filepath.Base(want) {
		t.Fatalf("name = %q, want %q", repo.Name(), filepath.Base(want))
	}
}

func TestDiscoverOutsideARepo(t *testing.T) {
	// A temp dir with no repo above it: HOME and the parent chain are
	// irrelevant because git stops at the filesystem boundary here.
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	_, err := Discover(dir)
	if !errors.Is(err, ErrNoRepo) {
		t.Fatalf("err = %v, want ErrNoRepo", err)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "init")
	return dir
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func resolve(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
