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
// dev server, Open probes every retryEvery until it answers, for at most Options.StartTimeout
// (defaultStartTimeout when zero). Stop waits stopGrace after SIGTERM before it kills. Once a dev
// server has exited, the lines it printed last are read for at most drainFor.
const (
	probeTimeout        = 2 * time.Second
	retryEvery          = 200 * time.Millisecond
	defaultStartTimeout = 30 * time.Second
	stopGrace           = 5 * time.Second
	drainFor            = 100 * time.Millisecond
)

// probeAfter is the probe's clock for the hello's deadline; tests replace it.
var probeAfter = time.After

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
// feed answers; it is never called after Open has returned. StartTimeout is how long a dev
// server Open started has to answer; zero is 30 s. Connect and Start default to feed.Connect and
// Start; tests replace them.
type Options struct {
	Dir, Addr, Alder string
	NoRun            bool
	Lines            func(string)
	StartTimeout     time.Duration
	Connect          func(ctx context.Context, addr string) (<-chan feed.Event, error)
	Start            func(alder, dir string, env []string) (Proc, error)
}

// Proc is a dev server the TUI started: its stderr lines, Stop, a channel closed once it has
// exited, and then its exit code.
type Proc interface {
	Stderr() <-chan string
	Stop() error
	Exited() <-chan struct{}
	Code() int
}

// Session is the dev server's feed (its first event the hello) and whether the TUI owns the dev
// server.
type Session struct {
	Events <-chan feed.Event
	Owned  bool

	cancel context.CancelFunc
	proc   Proc
	exited chan Exit
	once   sync.Once
	err    error
}

// Exit is how an owned dev server ended after its feed had answered: its exit code (128 plus the
// signal's number when a signal killed it) and the last stderr lines it printed after its feed
// answered, joined by newlines.
type Exit struct {
	Code       int
	StderrTail string
}

// Open attaches to the dev server on o.Addr when its hello names o.Dir (made absolute), and fails
// when the address serves another app's dev server or something that is not a dev server at all.
// When nothing listens on the address it starts `alder run o.Dir` there (unless o.NoRun) and
// probes it every 200 ms until its feed answers; it fails after o.StartTimeout, or once the dev
// server has exited, with the dev server's last stderr lines. The directories compare with their
// symlinks resolved. The feed lives until Close or until ctx ends.
func Open(ctx context.Context, o Options) (*Session, error) {
	if o.Connect == nil {
		o.Connect = feed.Connect
	}
	if o.Start == nil {
		o.Start = Start
	}
	if o.StartTimeout <= 0 {
		o.StartTimeout = defaultStartTimeout
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return nil, err
	}
	s, err := probe(ctx, o.Connect, o.Addr)
	switch {
	case err == nil:
		if d := s.hello.Hello.Dir; resolved(d) != resolved(dir) {
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

// resolved is path with its symlinks resolved, or only cleaned when that fails (it does not
// exist, say).
func resolved(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return filepath.Clean(path)
}

// own starts the dev server and waits for its feed, collecting its stderr meanwhile and handing
// each line to o.Lines. However it fails, the dev server's stderr is read to its end, so the
// goroutine reading it never stays blocked.
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
	deadline := time.NewTimer(o.StartTimeout)
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
			drain(stderr, add)
			discard(p)
			return nil, fail(fmt.Sprintf("alder run exited (code %d) before its feed answered on %s", p.Code(), o.Addr))
		case <-deadline.C:
			p.Stop()
			discard(p)
			return nil, fail(fmt.Sprintf("alder run did not answer on %s within %s", o.Addr, o.StartTimeout))
		case <-ctx.Done():
			p.Stop()
			discard(p)
			return nil, ctx.Err()
		case <-tick.C:
			s, err := probe(ctx, o.Connect, o.Addr)
			if err != nil {
				continue
			}
			return s.session(p), nil
		}
	}
}

// drain hands add the lines a dev server that has exited printed last, which may still be in the
// pipe, reading them for at most drainFor: a child of the dev server may hold the pipe open.
func drain(stderr <-chan string, add func(string)) {
	for until := time.After(drainFor); stderr != nil; {
		select {
		case l, ok := <-stderr:
			if !ok {
				return
			}
			add(l)
		case <-until:
			return
		}
	}
}

// discard reads p's stderr to its end in the background and drops it, so the goroutine that
// reads the pipe never blocks on a full channel once nothing else reads it.
func discard(p Proc) {
	go func() {
		for range p.Stderr() {
		}
	}()
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
//
// While connect runs, the deadline cancels the feed, which is how a connect that never answers
// returns. Once connect has returned the deadline only ends the wait for the hello, and a hello
// that is there when it passes still counts.
func probe(ctx context.Context, connect func(context.Context, string) (<-chan feed.Event, error), addr string) (*probed, error) {
	began := time.Now()
	fctx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(probeTimeout, cancel)
	ch, err := connect(fctx, addr)
	if !timer.Stop() && ctx.Err() == nil { // the deadline passed while connecting, and ended the feed
		return nil, errNoAnswer
	}
	var hello feed.Event
	if err == nil {
		got := func(ev feed.Event, ok bool) {
			if !ok || ev.Type != "hello" || ev.Hello == nil {
				err = feed.ErrNotAlder
			}
			hello = ev
		}
		select {
		case ev, ok := <-ch:
			got(ev, ok)
		case <-probeAfter(probeTimeout - time.Since(began)):
			select {
			case ev, ok := <-ch:
				got(ev, ok)
			default:
				err = errNoAnswer
			}
		case <-ctx.Done():
		}
	}
	switch {
	case ctx.Err() != nil: // the caller gave up, whatever the probe saw
		cancel()
		return nil, ctx.Err()
	case err != nil:
		cancel()
		return nil, err
	}
	return &probed{hello: hello, events: ch, ctx: fctx, cancel: cancel}, nil
}

// session is the Session for the probed feed, owning p when it is not nil. Its Events carry the
// hello again, then the feed's events. An owned dev server is watched for its exit.
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
	s := &Session{Events: out, Owned: p != nil, cancel: f.cancel, proc: p}
	if p != nil {
		s.exited = make(chan Exit, 1)
		go watch(f.ctx, p, s.exited)
	}
	return s
}

// watch keeps the last tailLines stderr lines of the owned dev server p, and sends its Exit on
// out once it exits, then closes out. When ctx ends first (Close, or the TUI's end), p's exit is
// the TUI's doing, so out is closed with no Exit.
func watch(ctx context.Context, p Proc, out chan<- Exit) {
	defer close(out)
	var tail []string
	add := func(l string) {
		if len(tail) == tailLines {
			tail = tail[1:]
		}
		tail = append(tail, l)
	}
	defer discard(p)
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
			drain(stderr, add)
			if ctx.Err() == nil {
				out <- Exit{Code: p.Code(), StderrTail: strings.Join(tail, "\n")}
			}
			return
		case <-ctx.Done():
			return
		}
	}
}

// Exited carries the Exit of an owned dev server that exits on its own after its feed answered,
// then is closed; it is closed with no Exit once the session is closed first. It is nil when
// the TUI does not own the dev server.
func (s *Session) Exited() <-chan Exit { return s.exited }

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
	code   int
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
		p.code = exitCode(cmd.ProcessState)
		close(p.exited)
	}()
	return p, nil
}

func (p *proc) Stderr() <-chan string   { return p.stderr }
func (p *proc) Exited() <-chan struct{} { return p.exited }

// Code is the child's exit code once Exited is closed.
func (p *proc) Code() int { return p.code }

// exitCode is the exit code of a child that has exited, as a shell gives it: 128 plus the
// signal's number when a signal killed it.
func exitCode(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

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
