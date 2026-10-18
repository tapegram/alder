//go:build unix

package testgocache

import (
	"os"
	"syscall"
)

// flockWorks: the lock below is real, so a wipe never overlaps another test binary's use.
const flockWorks = true

// lockShared takes a shared flock on f, waiting while another process holds it exclusively (a
// wipe in progress). Taken while f holds the exclusive lock, it converts that lock, which flock
// does by releasing it first.
func lockShared(f *os.File) error { return flock(f, syscall.LOCK_SH) }

// tryLockExclusive takes an exclusive flock on f if no other process holds any lock on it: ok
// is false, with no error, when one does. Like every flock conversion, it first releases the
// shared lock f holds, so after a false f holds no lock at all.
func tryLockExclusive(f *os.File) (ok bool, err error) {
	switch err := flock(f, syscall.LOCK_EX|syscall.LOCK_NB); err {
	case nil:
		return true, nil
	case syscall.EWOULDBLOCK:
		return false, nil
	default:
		return false, err
	}
}

func unlock(f *os.File) { syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

func flock(f *os.File, how int) error {
	for {
		if err := syscall.Flock(int(f.Fd()), how); err != syscall.EINTR {
			return err
		}
	}
}
