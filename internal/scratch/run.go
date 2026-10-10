package scratch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// WaitDelay bounds how long RunTimeout waits for the program's output to close after it exits
// or is killed: a process it started may still hold the pipes.
const WaitDelay = 5 * time.Second

// RunTimeout is Run with the time limit d instead of Timeout. The program runs in a process
// group of its own; at the limit the whole group is killed, so a process it started cannot keep
// RunTimeout waiting, and once the program has exited its output is read for at most WaitDelay
// longer (a process it left behind holding the output is then killed with the group). Either
// way the exit code is -1, with the reason appended to the standard error.
func RunTimeout(b Build, env []string, stdin string, d time.Duration) (string, string, int) {
	if b.Binary == "" {
		return "", "scratch: nothing to run\n", -1
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.Binary)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = WaitDelay
	ownGroup(cmd)
	err := cmd.Run()
	if cmd.Process != nil {
		killGroup(cmd) // whatever the program left running
	}
	if ctx.Err() != nil {
		fmt.Fprintf(&stderr, "scratch: %s timed out after %v\n", b.Binary, d)
		return stdout.String(), stderr.String(), -1
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
		return stdout.String(), stderr.String(), 0
	case errors.Is(err, exec.ErrWaitDelay):
		fmt.Fprintf(&stderr, "scratch: %s exited, but a process it started kept its output open; killed\n", b.Binary)
		return stdout.String(), stderr.String(), -1
	case errors.As(err, &ee) && ee.ExitCode() >= 0:
		return stdout.String(), stderr.String(), ee.ExitCode()
	default:
		fmt.Fprintf(&stderr, "scratch: %v\n", err)
		return stdout.String(), stderr.String(), -1
	}
}
