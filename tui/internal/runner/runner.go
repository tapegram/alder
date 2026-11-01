// Package runner is the one seam every alder subprocess call goes through, so that tests can
// replace the binary with a Fake.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Runner runs alder in a directory. Run waits for the process; Stream hands back its stdout lines
// as they arrive.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) (Result, error)
	Stream(ctx context.Context, dir string, args ...string) (Stream, error)
}

// Result is what a finished process left behind. Code is the exit status.
type Result struct {
	Stdout, Stderr []byte
	Code           int
}

// Stream is a running process. Lines yields stdout lines without their newline, with no limit on
// line length, and closes at EOF. Wait must be called after Lines is drained; it returns the exit
// status and everything the process wrote to stderr.
type Stream interface {
	Lines() <-chan string
	Wait() (code int, stderr []byte)
}

// Exec returns a Runner that runs the alder binary at the given path with the inherited
// environment.
func Exec(alder string) Runner { return execRunner{bin: alder} }

type execRunner struct{ bin string }

// exitCode turns the error from Wait into an exit status. An error that is not an exit status
// (the process could not start) is returned as err.
func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

func (r execRunner) Run(ctx context.Context, dir string, args ...string) (Result, error) {
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &out, &errb
	code, err := exitCode(cmd.Run())
	if err != nil {
		return Result{}, err
	}
	return Result{Stdout: out.Bytes(), Stderr: errb.Bytes(), Code: code}, nil
}

type execStream struct {
	cmd   *exec.Cmd
	lines chan string
	errb  *bytes.Buffer
}

func (r execRunner) Stream(ctx context.Context, dir string, args ...string) (Stream, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Dir = dir
	var errb bytes.Buffer
	cmd.Stderr = &errb
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &execStream{cmd: cmd, lines: make(chan string), errb: &errb}
	go func() {
		defer close(s.lines)
		br := bufio.NewReader(pipe)
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				s.lines <- strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			}
			if err != nil {
				return
			}
		}
	}()
	return s, nil
}

func (s *execStream) Lines() <-chan string { return s.lines }

// Wait drains any lines the caller left unread, so that the process is never blocked on a full
// pipe, then reaps it.
func (s *execStream) Wait() (int, []byte) {
	for range s.lines {
	}
	code, err := exitCode(s.cmd.Wait())
	if err != nil {
		code = -1
	}
	return code, s.errb.Bytes()
}
