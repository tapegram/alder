// Package devproc gives the TUI its dev server: it attaches to an `alder run` already serving the
// app's directory on the address, or starts one and owns it, stopping it on Close. An owned dev
// server never outlives the TUI, even one killed with SIGKILL: it runs with
// ALDER_EXIT_ON_STDIN_EOF=1 and its standard input is a pipe whose other end only the TUI holds.
package devproc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"alder/tui/internal/feed"
)

// A probe of the feed that has not answered with its hello within probeTimeout is abandoned, so
// a listener that accepts connections but never answers cannot hang Open. After starting its own
// dev server, Open probes every retryEvery until it answers, for at most startTimeout. Stop waits
// stopGrace after SIGTERM before it kills.
const (
	probeTimeout = 2 * time.Second
	retryEvery   = 200 * time.Millisecond
	startTimeout = 30 * time.Second
	stopGrace    = 5 * time.Second
)

// Open keeps the last earlyMax stderr lines of an owned dev server that has not answered yet, and
// its error names the last tailLines of them.
const (
	earlyMax  = 200
	tailLines = 10
)

// errNoAnswer is a probe's error when the address accepted the connection but sent no hello in
// time.
var errNoAnswer = errors.New("no answer")

// Options says which dev server Open wants: the app's directory, the dev server's address, the
// alder binary to start one with, and whether starting one is forbidden (--no-run). Lines, when
// set, is called with each stderr line of a dev server Open started, as it arrives, until its
// feed answers; it is never called after Open has returned. Connect and Start default to
// feed.Connect and Start; tests replace them.
type Options struct {
	Dir, Addr, Alder string
	NoRun            bool
	Lines            func(string)
	Connect          func(ctx context.Context, addr string) (<-chan feed.Event, error)
	Start            func(alder, dir string, env []string) (Proc, error)
}

// Proc is a dev server the TUI started: its stderr lines, Stop, and a channel closed once it has
// exited.
type Proc interface {
	Stderr() <-chan string
	Stop() error
	Exited() <-chan struct{}
}

// Session is the dev server's feed (its first event the hello) and whether the TUI owns the dev
// server.
type Session struct {
	Events <-chan feed.Event
	Owned  bool

	cancel context.CancelFunc
	proc   Proc
	once   sync.Once
	err    error
}

// Open attaches to the dev server on o.Addr when its hello names o.Dir (made absolute), and fails
// when the address serves another app's dev server or something that is not a dev server at all.
// When nothing listens on the address it starts `alder run o.Dir` there (unless o.NoRun) and
// probes it every 200 ms until its feed answers; it fails after 30 s, or once the dev server has
// exited, with the dev server's last stderr lines. The feed lives until Close or until ctx ends.
func Open(ctx context.Context, o Options) (*Session, error) {
	if o.Connect == nil {
		o.Connect = feed.Connect
	}
	if o.Start == nil {
		o.Start = Start
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return nil, err
	}
	s, err := probe(ctx, o.Connect, o.Addr)
	switch {
	case err == nil:
		if d := s.hello.Hello.Dir; filepath.Clean(d) != dir {
			s.cancel()
			return nil, fmt.Errorf("%s is the dev server of %s; pass --addr", o.Addr, d)
		}
		return s.session(nil), nil
	case errors.Is(err, feed.ErrNotAlder), errors.Is(err, errNoAnswer):
		return nil, fmt.Errorf("%s is serving something that is not alder run; pass --addr", o.Addr)
	case !errors.Is(err, syscall.ECONNREFUSED):
		return nil, fmt.Errorf("%s: %w", o.Addr, err)
	case o.NoRun:
		return nil, fmt.Errorf("no dev server on %s (--no-run)", o.Addr)
	}
	return own(ctx, o, dir)
}

// own starts the dev server and waits for its feed, collecting its stderr meanwhile and handing
// each line to o.Lines.
func own(ctx context.Context, o Options, dir string) (*Session, error) {
	p, err := o.Start(o.Alder, dir, []string{"ALDER_ADDR=" + o.Addr, "ALDER_EXIT_ON_STDIN_EOF=1"})
	if err != nil {
		return nil, fmt.Errorf("starting alder run: %w", err)
	}
	var lines []string
	add := func(l string) {
		if len(lines) == earlyMax {
			lines = lines[1:]
		}
		lines = append(lines, l)
		if o.Lines != nil {
			o.Lines(l)
		}
	}
	fail := func(msg string) error {
		tail := lines[max(0, len(lines)-tailLines):]
		if len(tail) == 0 {
			return errors.New(msg)
		}
		return fmt.Errorf("%s:\n%s", msg, strings.Join(tail, "\n"))
	}
	deadline := time.NewTimer(startTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(retryEvery)
	defer tick.Stop()
	stderr := p.Stderr()
	for {
		select {
		case l, ok := <-stderr:
			if !ok {
				stderr = nil
				continue
			}
			add(l)
		case <-p.Exited():
			// The lines it printed last may still be in the pipe.
			for drain := time.After(100 * time.Millisecond); stderr != nil; {
				select {
				case l, ok := <-stderr:
					if !ok {
						stderr = nil
						break
					}
					add(l)
				case <-drain:
					stderr = nil
				}
			}
			return nil, fail("alder run exited before its feed answered on " + o.Addr)
		case <-deadline.C:
			p.Stop()
			return nil, fail(fmt.Sprintf("alder run did not answer on %s within %s", o.Addr, startTimeout))
		case <-ctx.Done():
			p.Stop()
			return nil, ctx.Err()
		case <-tick.C:
			s, err := probe(ctx, o.Connect, o.Addr)
			if err != nil {
				continue
			}
			// The dev server's later stderr is read and dropped, so it never blocks on a full
			// pipe; its feed carries what it prints from here on.
			go func() {
				for range p.Stderr() {
				}
			}()
			return s.session(p), nil
		}
	}
}

// probed is a feed that answered: its hello, the rest of its events, and the cancel that ends it.
type probed struct {
	hello  feed.Event
	events <-chan feed.Event
	ctx    context.Context
	cancel context.CancelFunc
}

// probe connects to the feed on addr and reads its hello, giving up after probeTimeout. The feed
// of a probe that succeeds lives until its cancel (or ctx's end); a failed probe's is ended.
func probe(ctx context.Context, connect func(context.Context, string) (<-chan feed.Event, error), addr string) (*probed, error) {
	fctx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(probeTimeout, cancel)
	ch, err := connect(fctx, addr)
	var hello feed.Event
	if err == nil {
		select {
		case ev, ok := <-ch:
			if !ok || ev.Type != "hello" || ev.Hello == nil {
				err = feed.ErrNotAlder
			}
			hello = ev
		case <-fctx.Done():
			err = fctx.Err()
		}
	}
	timer.Stop()
	switch {
	case ctx.Err() != nil: // the caller gave up, whatever the probe saw
		cancel()
		return nil, ctx.Err()
	case fctx.Err() != nil: // the timer fired: no hello in time, or one whose feed it has just ended
		return nil, errNoAnswer
	case err != nil:
		cancel()
		return nil, err
	}
	return &probed{hello: hello, events: ch, ctx: fctx, cancel: cancel}, nil
}

// session is the Session for the probed feed, owning p when it is not nil. Its Events carry the
// hello again, then the feed's events.
func (f *probed) session(p Proc) *Session {
	out := make(chan feed.Event, 64)
	go func() {
		defer close(out)
		select {
		case out <- f.hello:
		case <-f.ctx.Done():
			return
		}
		for ev := range f.events {
			select {
			case out <- ev:
			case <-f.ctx.Done():
				return
			}
		}
	}()
	return &Session{Events: out, Owned: p != nil, cancel: f.cancel, proc: p}
}

// Close ends the feed and, when the TUI owns the dev server, stops it. Only the first call does
// anything; later ones return its error.
func (s *Session) Close() error {
	s.once.Do(func() {
		s.cancel()
		if s.proc != nil {
			s.err = s.proc.Stop()
		}
	})
	return s.err
}

// proc is the Proc Start returns: the child, the write end of its stdin, which nothing writes to
// but which keeps the child alive while the TUI does, its stderr lines and its exit.
type proc struct {
	cmd    *exec.Cmd
	stdin  *os.File
	stderr chan string
	exited chan struct{}
}

// Start runs `alder run dir` in dir (which must be absolute: alder resolves its build directory
// against the working directory, so the build lands in dir/build, where the Data and Tests
// screens look) with env added to the TUI's environment, its stdin a pipe whose
// write end only this process holds (so the child sees EOF when the TUI dies, however it dies),
// its stdout discarded and its stderr read line by line, each line whole however long.
func Start(alder, dir string, env []string) (Proc, error) {
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		return nil, err
	}
	cmd := exec.Command(alder, "run", dir)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin, cmd.Stderr = inR, errW
	err = cmd.Start()
	inR.Close()
	errW.Close()
	if err != nil {
		inW.Close()
		errR.Close()
		return nil, err
	}
	p := &proc{cmd: cmd, stdin: inW, stderr: make(chan string, 64), exited: make(chan struct{})}
	go func() {
		defer close(p.stderr)
		defer errR.Close()
		r := bufio.NewReader(errR)
		for {
			l, err := r.ReadString('\n')
			if l = strings.TrimRight(l, "\r\n"); l != "" || err == nil {
				p.stderr <- l
			}
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
					p.stderr <- "alder-tui: reading alder run's stderr: " + err.Error()
				}
				return
			}
		}
	}()
	go func() {
		_ = cmd.Wait()
		inW.Close()
		close(p.exited)
	}()
	return p, nil
}

func (p *proc) Stderr() <-chan string   { return p.stderr }
func (p *proc) Exited() <-chan struct{} { return p.exited }

// Stop sends SIGTERM and waits for the child to exit, killing it after 5 s; on Windows, which has
// no SIGTERM, it kills it at once. It returns once the child has exited.
func (p *proc) Stop() error {
	select {
	case <-p.exited:
		return nil
	default:
	}
	if runtime.GOOS != "windows" {
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		t := time.NewTimer(stopGrace)
		defer t.Stop()
		select {
		case <-p.exited:
			return nil
		case <-t.C:
		}
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	<-p.exited
	return nil
}
