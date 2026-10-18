// Package testgocache gives the go command that alder's tests run a build cache of its own.
//
// Alder compiles apps to Go, and many tests build and run the generated programs (go build,
// go run, go vet, go test, directly or through the alder binary). Those programs differ on
// every run, and the go command keeps a cache entry until it has gone unused for five days,
// with no size limit, so through the user's GOCACHE a few days of test runs filled hundreds
// of gigabytes. A package whose tests run the go command calls Main (or Setup) from its
// TestMain. GOCACHE is then, for the test binary and every process it starts:
//
//   - $ALDER_TEST_GOCACHE when set (an absolute path; CI sets it to the go command's own cache,
//     which its runners restore and then discard), else
//   - os.TempDir()/alder-test-gocache, one directory every test run shares, so a warm run
//     does not compile the standard library and the dependencies again.
//
// A GOCACHE the user has set is not kept: the point is that the tests never write into the
// cache the user builds with, whatever it is (GOCACHE=off cannot build at all).
//
// The cache is bounded: before the tests run, when it holds more than the cap (10 GiB, or
// $ALDER_TEST_GOCACHE_MAX_GB gigabytes, a positive number), it is wiped. `go test ./...` runs
// several test binaries at once, so each holds a shared flock on the lock file beside the
// cache (<cache>.lock) for as long as it runs, and a wipe takes the lock exclusively, without
// waiting: when another binary is using the cache, the wipe is left to a later run. A binary
// that starts during a wipe waits for it. Without flock (not unix) the wipe is best effort;
// see lock_other.go.
//
// To clear the cache by hand, remove the directory, on unix
// rm -rf "${TMPDIR:-/tmp}/alder-test-gocache" (not "$(go env GOCACHE)", which in a shell is
// the user's cache).
package testgocache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

const (
	// DirEnv names the cache directory, replacing the default under os.TempDir.
	DirEnv = "ALDER_TEST_GOCACHE"
	// MaxEnv is the cap in gigabytes (GiB), replacing DefaultMaxBytes.
	MaxEnv = "ALDER_TEST_GOCACHE_MAX_GB"
	// DefaultMaxBytes is the size above which the cache is wiped before a test binary runs.
	DefaultMaxBytes = 10 << 30
)

// Main is a TestMain: Setup, the tests, the release, os.Exit with their code. A failed Setup
// exits 2 before any test runs.
func Main(m *testing.M) {
	release, err := Setup()
	if err != nil {
		fmt.Fprintln(os.Stderr, "testgocache:", err)
		os.Exit(2)
	}
	code := m.Run()
	release()
	os.Exit(code)
}

// Setup points GOCACHE at the tests' cache (see the package comment), wiping the cache first
// if it is over the cap, and returns the release of its lock, to call when the tests are done.
// It is called before m.Run: m.Run starts the log of the files a test reads, by which go test
// decides whether a cached result still holds, and the cache's files must not be in it.
func Setup() (release func(), err error) {
	dir := os.Getenv(DirEnv)
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "alder-test-gocache")
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("%s=%s: the go command's cache must be an absolute path", DirEnv, dir)
	}
	limit := int64(DefaultMaxBytes)
	if s := os.Getenv(MaxEnv); s != "" {
		gb, err := strconv.ParseFloat(s, 64)
		if err != nil || !(gb > 0) {
			return nil, fmt.Errorf("%s=%s: want a positive number of gigabytes", MaxEnv, s)
		}
		limit = max(1, int64(gb*(1<<30)))
	}
	release, err = prepare(dir, limit)
	if err != nil {
		return nil, err
	}
	if err := os.Setenv("GOCACHE", dir); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// prepare takes the shared lock on dir's lock file, wipes dir if it holds more than limit bytes
// and no other process holds the lock, and makes sure dir exists. The lock is held until the
// returned release.
func prepare(dir string, limit int64) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, err
	}
	lockPath := dir + ".lock"
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (func(), error) {
		unlock(f)
		f.Close()
		return nil, fmt.Errorf("the tests' go cache %s: %w", dir, err)
	}
	if err := lockShared(f); err != nil {
		return fail(err)
	}
	if over, err := larger(dir, limit); err != nil {
		return fail(err)
	} else if over {
		ok, err := tryLockExclusive(f)
		if err != nil {
			return fail(err)
		}
		if ok {
			// measured again: another binary may have wiped it between the shared lock and this one
			if over, err := larger(dir, limit); err != nil {
				return fail(err)
			} else if over {
				if err := wipe(dir); err != nil {
					return fail(err)
				}
			}
		}
		if err := lockShared(f); err != nil {
			return fail(err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(err)
	}
	return func() {
		unlock(f)
		f.Close()
	}, nil
}

// larger reports whether the regular files under dir hold more than limit bytes in all, stopping
// as soon as they do. A missing dir holds nothing.
func larger(dir string, limit int64) (bool, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // a file the go command trimmed while we walked, or no cache yet
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		total += info.Size()
		if total > limit {
			return fs.SkipAll
		}
		return nil
	})
	return total > limit, err
}

// wipe removes dir: renamed first to dir.wipe-<pid>, so the go command never sees it half
// deleted, then removed, with any copy an interrupted earlier wipe left behind.
func wipe(dir string) error {
	if err := os.Rename(dir, fmt.Sprintf("%s.wipe-%d", dir, os.Getpid())); err != nil && !errors.Is(err, fs.ErrNotExist) {
		if !flockWorks {
			return nil // in use (lock_other.go): a later run wipes it
		}
		return err
	}
	old, err := filepath.Glob(dir + ".wipe-*")
	if err != nil {
		return err
	}
	for _, p := range old {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}
