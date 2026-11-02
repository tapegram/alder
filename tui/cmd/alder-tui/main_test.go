//go:build unix

package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// helperEnv makes the test binary run as alder-tui, with the arguments after "--", so that a
// test can signal a real TUI process without a pty.
const helperEnv = "ALDER_TUI_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		args := os.Args
		for i, a := range args {
			if a == "--" {
				args = args[i+1:]
				break
			}
		}
		extraOptions = append(extraOptions, tea.WithInput(os.Stdin))
		os.Exit(run(args, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// fakeAlder is an alder that reports a version, serves nothing on `run` until its stdin closes,
// and on `test` starts a long sleep, writes the sleep's pid to $PIDFILE and waits for it.
const fakeAlder = `#!/bin/sh
case "$1" in
help) echo '{"version":"(devel)"}' ;;
run) exec cat >/dev/null ;;
test) sleep 300 & echo $! > "$PIDFILE"; wait ;;
esac
`

// eventually polls cond every 20 ms for up to d, and reports whether it held.
func eventually(d time.Duration, cond func() bool) bool {
	for deadline := time.Now().Add(d); ; time.Sleep(20 * time.Millisecond) {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

// freeAddr is a localhost address nothing listens on.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// TestRun_SignalStopsTests runs the TUI with a fake alder, opens the Tests tab so that a run is
// in flight, then sends the TUI each signal that ends it from outside: the TUI exits, and the
// run's grandchild (a stand-in for the compiled tests) is gone with it.
func TestRun_SignalStopsTests(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := t.TempDir()
			alder := filepath.Join(dir, "alder")
			if err := os.WriteFile(alder, []byte(fakeAlder), 0o755); err != nil {
				t.Fatal(err)
			}
			app := filepath.Join(dir, "app")
			if err := os.Mkdir(app, 0o755); err != nil {
				t.Fatal(err)
			}
			pidFile := filepath.Join(dir, "pid")
			out, err := os.Create(filepath.Join(dir, "out"))
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			stdin, keys, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer keys.Close()

			cmd := exec.Command(os.Args[0], "-test.run=^$", "--", "-addr", freeAddr(t), app)
			cmd.Env = append(os.Environ(), helperEnv+"=1", "ALDER_BIN="+alder, "PIDFILE="+pidFile)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, out, out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			stdin.Close()
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			defer func() {
				cmd.Process.Kill()
				<-exited
			}()

			if _, err := keys.Write([]byte("4")); err != nil {
				t.Fatal(err)
			}
			var pid int
			if !eventually(10*time.Second, func() bool {
				b, err := os.ReadFile(pidFile)
				if err != nil {
					return false
				}
				pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
				return err == nil
			}) {
				b, _ := os.ReadFile(out.Name())
				t.Fatalf("the Tests tab started no run within 10s; the TUI wrote:\n%s", b)
			}
			defer syscall.Kill(pid, syscall.SIGKILL)

			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case <-exited:
				exited <- nil // for the deferred wait
			case <-time.After(15 * time.Second):
				t.Fatalf("the TUI is still running 15s after %v", sig)
			}
			if !eventually(2*time.Second, func() bool { return syscall.Kill(pid, 0) != nil }) {
				t.Errorf("the run's sleep %d outlived the TUI after %v", pid, sig)
			}
		})
	}
}
