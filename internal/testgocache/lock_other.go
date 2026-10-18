//go:build !unix

package testgocache

import "os"

// Without flock the lock is a no-op: a wipe can then overlap another test binary's go command.
// wipe renames the cache away before deleting it, which fails while that command has files
// in it open (Windows), and so leaves the cache for a later run to wipe.

const flockWorks = false

func lockShared(*os.File) error { return nil }

func tryLockExclusive(*os.File) (bool, error) { return true, nil }

func unlock(*os.File) {}
