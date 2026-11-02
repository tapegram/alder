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
	"sync"
)

// Runner runs alder in a directory. Run waits for the process; Stream hands back its stdout lines
// as they arrive. Ending ctx stops the process: on Unix it and every process it started get
// SIGTERM, then SIGKILL if any is left 2s later; elsewhere it is killed. Run then returns
// ctx.Err(), as does the Stream's Wait.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) (Result, error)
	Stream(ctx context.Context, dir string, args ...string) (Stream, error)
}

// Result is what a finished process left behind. Code is the exit status.
type Result struct {
	Stdout, Stderr []byte
	Code           int
}

// Stream is a running process. Lines yields stdout lines without their newline or a trailing \r,
// with no limit on line length, and closes at EOF. Wait drains what Lines has left, reaps the
// process and returns its exit status and everything it wrote to stderr. err is ctx.Err() when
// the stream's ctx ended before the process did, with code -1; any other error that is not an
// exit status is returned the same way. Wait may be called more than once, and from more than one
// goroutine: every call returns what the first did.
type Stream interface {
	Lines() <-chan string
	Wait() (code int, stderr string, err error)
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

// command is the alder command for args in dir, in a process group of its own so that ctx's end
// stops whatever it started too. The group's reaped must be called once the command is.
func (r execRunner) command(ctx context.Context, dir string, args []string) (*exec.Cmd, *group) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Dir = dir
	return cmd, ownGroup(cmd)
}

// waited is the exit status and error of a process whose Wait (or Run) returned werr: ctx.Err()
// when ctx ended and the process did not finish on its own.
func waited(ctx context.Context, werr error) (int, error) {
	if werr != nil && ctx.Err() != nil {
		return -1, ctx.Err()
	}
	return exitCode(werr)
}

func (r execRunner) Run(ctx context.Context, dir string, args ...string) (Result, error) {
	var out, errb bytes.Buffer
	cmd, g := r.command(ctx, dir, args)
	cmd.Stdout, cmd.Stderr = &out, &errb
	werr := cmd.Run()
	g.reaped()
	code, err := waited(ctx, werr)
	if err != nil {
		return Result{}, err
	}
	return Result{Stdout: out.Bytes(), Stderr: errb.Bytes(), Code: code}, nil
}

type execStream struct {
	ctx   context.Context
	cmd   *exec.Cmd
	group *group
	lines chan string
	errb  *bytes.Buffer

	once   sync.Once
	code   int
	stderr string
	err    error
}

func (r execRunner) Stream(ctx context.Context, dir string, args ...string) (Stream, error) {
	cmd, g := r.command(ctx, dir, args)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &execStream{ctx: ctx, cmd: cmd, group: g, lines: make(chan string), errb: &errb}
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
// pipe, then reaps it, once.
func (s *execStream) Wait() (int, string, error) {
	s.once.Do(func() {
		for range s.lines {
		}
		werr := s.cmd.Wait()
		s.group.reaped()
		s.code, s.err = waited(s.ctx, werr)
		s.stderr = s.errb.String()
	})
	return s.code, s.stderr, s.err
}
