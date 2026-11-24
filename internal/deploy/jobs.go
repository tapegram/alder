package deploy

import (
	"context"
	"strings"
)

// JobsRunner runs the app's --jobs maintenance task at an environment: args are the task's
// (list, retry, ...), out is its output (stdout, then stderr) and code its exit status. A target
// no build serves yet answers ErrNotDeployed.
type JobsRunner interface {
	Jobs(ctx context.Context, args ...string) (out string, code int, err error)
}

// Jobs runs the current build's --jobs at the box.
func (s *Ssh) Jobs(ctx context.Context, args ...string) (string, int, error) {
	return s.current(ctx, append([]string{"--jobs"}, args...)...)
}

// Jobs runs the app's --jobs in the environment's machine (started first when stopped).
func (f *Fly) Jobs(ctx context.Context, args ...string) (string, int, error) {
	ok, err := f.running(ctx)
	if err != nil {
		return "", -1, err
	}
	if !ok {
		return "", -1, ErrNotDeployed
	}
	var a strings.Builder
	for _, x := range args {
		a.WriteString(" " + q(x))
	}
	r, err := f.execCode(ctx, `ALDER_DB="$R/db/app.db" ALDER_STATIC="${ALDER_APP_DIR:-/app}/static" exec "${ALDER_APP_DIR:-/app}/app" --jobs`+a.String())
	if err != nil {
		return "", -1, err
	}
	return r.Stdout + r.Stderr, r.ExitCode, nil
}

var (
	_ JobsRunner = (*Ssh)(nil)
	_ JobsRunner = (*Fly)(nil)
)
