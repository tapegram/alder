//go:build !unix

package backup

import "os"

// tryLock does nothing where flock does not exist: one shipper per database is not enforced.
func tryLock(*os.File) error { return nil }

func unlockFile(*os.File) {}
