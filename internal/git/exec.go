package git

import (
	"errors"
	"os/exec"
	"sync"
)

// run invokes git in the repo root and returns stdout.
//
// Exit status 1 is not treated as failure: `git diff` uses it to mean
// "differences exist", which the --no-index call in CollectChanges relies on.
// This matches the original's execFile callback, which only rejected on codes
// other than 1.
func (r Repo) run(args ...string) (string, error) {
	out, err := r.command(args).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return string(out), nil
		}
		return string(out), err
	}
	return string(out), nil
}

// runStrict is run without the exit-1 tolerance, for commands where a failure
// has to surface. Discarding a file is the only place that matters: the
// original swallowed a failed checkout, which is not a behavior worth porting.
func (r Repo) runStrict(args ...string) (string, error) {
	out, err := r.command(args).Output()
	return string(out), err
}

func (r Repo) command(args []string) *exec.Cmd {
	return exec.Command("git", append([]string{"-C", r.Root}, args...)...)
}

// runAll invokes several git commands concurrently and returns their outputs
// in order. The original used Promise.all for the same reason: on a large repo
// the status and diff calls each take real time.
func (r Repo) runAll(cmds ...[]string) ([]string, error) {
	outs := make([]string, len(cmds))
	errs := make([]error, len(cmds))

	var wg sync.WaitGroup
	for i, args := range cmds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outs[i], errs[i] = r.run(args...)
		}()
	}
	wg.Wait()

	return outs, errors.Join(errs...)
}
