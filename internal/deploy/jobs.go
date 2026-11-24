package deploy

import (
	"context"
	"strings"

	"alder/rt/db"
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

// JobStore is one database that holds jobs: the shared database (Key "") or the silo database of
// Key/Value. Open opens it from this machine, read-only when reader is true.
type JobStore struct {
	Key, Value string
	Open       func(ctx context.Context, reader bool) (*db.SQLite, error)
}

// JobStorer is a target whose jobs are read and written from this machine, over its databases,
// rather than by a task at the target. A target no build serves yet answers ErrNotDeployed.
type JobStorer interface {
	JobStores(ctx context.Context) ([]JobStore, error)
}

// JobStores are the shared database and the silo databases of every declared key, after Connect.
// It creates nothing (Lock does): no service or no shared database is ErrNotDeployed.
func (r *Render) JobStores(ctx context.Context) ([]JobStore, error) {
	if !r.have {
		return nil, ErrNotDeployed
	}
	if _, ok, err := r.DB.sharedDB(ctx); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrNotDeployed
	}
	api := r.DB.api()
	opener := func(name string) func(context.Context, bool) (*db.SQLite, error) {
		return func(ctx context.Context, reader bool) (*db.SQLite, error) { return api.Open(ctx, name, reader) }
	}
	stores := []JobStore{{Open: opener(r.DB.SharedName())}}
	for _, key := range r.DB.Silos {
		kids, err := api.Children(ctx, r.DB.App, key)
		if err != nil {
			return nil, err
		}
		for _, k := range kids {
			if v, ok := db.ChildValue(r.DB.App, key, k.Name); ok {
				stores = append(stores, JobStore{Key: key, Value: v, Open: opener(k.Name)})
			}
		}
	}
	return stores, nil
}

var (
	_ JobStorer  = (*Render)(nil)
	_ JobsRunner = (*Ssh)(nil)
	_ JobsRunner = (*Fly)(nil)
)
