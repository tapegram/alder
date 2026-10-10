//go:build unix

package scratch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockFile takes an exclusive flock on path (created, with its directory, when absent) and
// returns its release. When another holds it, lockFile calls waiting (unless nil) once and
// waits, trying again every 100 ms, until it gets the lock or ctx is done (ctx's error). The
// kernel drops the lock when the process dies, so a crashed holder never leaves it held.
func lockFile(ctx context.Context, path string, waiting func()) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	try := func() error {
		for {
			if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != syscall.EINTR {
				return err
			}
		}
	}
	err = try()
	if err == syscall.EWOULDBLOCK {
		if waiting != nil {
			waiting()
		}
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for err == syscall.EWOULDBLOCK {
			select {
			case <-ctx.Done():
				f.Close()
				return nil, ctx.Err()
			case <-tick.C:
			}
			err = try()
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("locking %s: %v", path, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
