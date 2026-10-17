//go:build unix

package backup

import (
	"os"
	"syscall"
)

// tryLock takes an exclusive lock on f without waiting (flock: per open file, so a second open
// of the same file in this process conflicts too).
func tryLock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) }

func unlockFile(f *os.File) { syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
