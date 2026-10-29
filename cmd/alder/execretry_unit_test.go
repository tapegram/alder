package main

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestRetryETXTBSY(t *testing.T) {
	t.Parallel()
	busy := &exec.Error{Name: "x", Err: fmt.Errorf("fork/exec x: %w", syscall.ETXTBSY)}
	calls := 0
	err := retryETXTBSYSleep(func() error {
		calls++
		if calls <= 2 {
			return busy
		}
		return nil
	}, func(time.Duration) {})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d, want nil after 3", err, calls)
	}

	calls = 0
	err = retryETXTBSYSleep(func() error { calls++; return busy }, func(time.Duration) {})
	if !errors.Is(err, syscall.ETXTBSY) || calls != 20 {
		t.Fatalf("err=%v calls=%d, want ETXTBSY after 20", err, calls)
	}

	other := errors.New("boom")
	calls = 0
	if err = retryETXTBSYSleep(func() error { calls++; return other }, func(time.Duration) {}); err != other || calls != 1 {
		t.Fatalf("err=%v calls=%d, want the other error at once", err, calls)
	}
}
