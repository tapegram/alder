package main

import (
	"errors"
	"syscall"
	"time"
)

// retryETXTBSY runs start until it stops failing with ETXTBSY ("text file busy"), at most 20
// times with a growing short pause. On Linux, a process that forks while another goroutine holds
// a write descriptor to a just-written binary inherits that descriptor until its own exec, and
// exec'ing the binary meanwhile fails with ETXTBSY. The tests run in parallel and write binaries
// (copying a build into build/<app>/<app>), so any exec of one can hit it; Go's own cmd/go tests
// retry the same way. start must build a fresh exec.Cmd on each call, since a Cmd cannot be
// started again after a failed Start.
func retryETXTBSY(start func() error) error {
	return retryETXTBSYSleep(start, time.Sleep)
}

func retryETXTBSYSleep(start func() error, sleep func(time.Duration)) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = start(); !errors.Is(err, syscall.ETXTBSY) {
			return err
		}
		sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
}
