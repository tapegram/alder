//go:build unix

package runner

import (
	"context"
	"errors"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// gone waits up to a second for the process pid to be gone, and reports whether it went.
func gone(pid int) bool {
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
	}
	return false
}

func TestStream_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The shell's first line is the pid of a sleep it started, which holds stdout open and would
	// outlive a kill of the shell alone.
	s, err := Exec("/bin/sh").Stream(ctx, t.TempDir(), "-c", "sleep 30 & echo $!; wait")
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(<-s.Lines())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := s.Wait()
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Wait after cancel = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("Wait still blocks 2s after the stream was cancelled: sleep outlived its shell")
	}
	if !gone(pid) {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("sleep %d is still running after the stream was cancelled", pid)
	}
}

func TestStream_CancelKillsWhatIgnoresTerm(t *testing.T) {
	defer func(d time.Duration) { killDelay = d }(killDelay)
	killDelay = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The shell and its sleep ignore SIGTERM, as a stuck alder would: the group's SIGKILL ends both.
	s, err := Exec("/bin/sh").Stream(ctx, t.TempDir(), "-c", "trap '' TERM; sleep 30 & echo $!; wait")
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(<-s.Lines())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := s.Wait()
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Wait after cancel = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("Wait still blocks 2s after the stream was cancelled: SIGTERM was ignored and nothing followed")
	}
	if !gone(pid) {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("sleep %d is still running after the stream was cancelled", pid)
	}
}
