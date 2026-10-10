package scratch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// WaitDelay bounds how long RunTimeout waits for the program's output to close after it exits
// or is killed: a process it started may still hold the pipes.
const WaitDelay = 5 * time.Second

// RunTimeout is Run with the time limit d instead of Timeout: RunContext with a context that
// times out after d.
func RunTimeout(b Build, env []string, stdin string, d time.Duration) (string, string, int) {
	ctx, cancel := context.WithTimeoutCause(context.Background(), d, fmt.Errorf("timed out after %v", d))
	defer cancel()
	return RunContext(ctx, b, env, stdin)
}

// EnvStdinEOF is the variable RunTethered sets to 1: a main that honours it exits when its
// standard input reaches end of file.
const EnvStdinEOF = "ALDER_SCRATCH_STDIN_EOF"

// RunTethered is RunContext (no stdin) for a program whose main honours EnvStdinEOF, which it
// sets: the program's standard input is a pipe whose write end only this process holds,
// closed when the run ends, so the program, out of reach of signals in its own process group,
// still exits when this process dies in any way (SIGKILL included). Its contract is
// otherwise RunContext's.
func RunTethered(ctx context.Context, b Build, env []string) (string, string, int) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return "", fmt.Sprintf("scratch: %v\n", err), -1
	}
	defer pr.Close()
	defer pw.Close()
	var stdout, stderr bytes.Buffer
	exit := RunStream(ctx, b, append(append([]string(nil), env...), EnvStdinEOF+"=1"), pr, &stdout, &stderr)
	return stdout.String(), stderr.String(), exit
}

// RunContext is Run until ctx is done (no time limit of its own): RunStream with the output
// collected. The program runs in a process group of its own, so a terminal's Ctrl-C reaches
// only the caller; when ctx is done (the caller's own interrupt, or a deadline) the whole group
// is killed, so a process it started cannot keep RunContext waiting either. Once the program
// has exited its output is read for at most WaitDelay longer (a process it left behind holding
// the output is then killed with the group). Killed or left behind, the exit code is -1, with
// the reason appended to the standard error: "<binary> timed out…" for a deadline (its cause
// when it has one, as RunTimeout's does), "<binary> was interrupted" for a cancellation.
func RunContext(ctx context.Context, b Build, env []string, stdin string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	exit := RunStream(ctx, b, env, strings.NewReader(stdin), &stdout, &stderr)
	return stdout.String(), stderr.String(), exit
}

// RunStream is RunContext writing the program's standard output and error to stdout and
// stderr as it produces them (each from a goroutine of its own when it is not an *os.File, so
// the two writers must not be the same unsynchronized value), and reading its standard input
// from stdin (nil: none). An *os.File stdin, such as a pipe's read end, is given to the program
// itself: it sees end of file when every write end is closed — when the caller closes its end,
// or dies. The exit code and the reason lines appended to stderr are RunContext's.
func RunStream(ctx context.Context, b Build, env []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if b.Binary == "" {
		io.WriteString(stderr, "scratch: nothing to run\n")
		return -1
	}
	cmd := exec.CommandContext(ctx, b.Binary)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = stdin
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = WaitDelay
	ownGroup(cmd)
	err := cmd.Run()
	if cmd.Process != nil {
		killGroup(cmd) // whatever the program left running
	}
	if ctx.Err() != nil {
		fmt.Fprintf(stderr, "scratch: %s %s\n", b.Binary, stopReason(ctx))
		return -1
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.Is(err, exec.ErrWaitDelay):
		fmt.Fprintf(stderr, "scratch: %s exited, but a process it started kept its output open; killed\n", b.Binary)
		return -1
	case errors.As(err, &ee) && ee.ExitCode() >= 0:
		return ee.ExitCode()
	default:
		fmt.Fprintf(stderr, "scratch: %v\n", err)
		return -1
	}
}

// stopReason says why the done ctx stopped a program.
func stopReason(ctx context.Context) string {
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(cause, context.Canceled):
		return "was interrupted"
	default:
		return cause.Error()
	}
}
