package devproc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"alder/tui/internal/feed"
)

// refused is the error a dial to a closed port returns.
var refused = &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}

// helloFor is a Connect that answers with a feed whose hello names dir.
func helloFor(dir string) func(context.Context, string) (<-chan feed.Event, error) {
	return func(ctx context.Context, addr string) (<-chan feed.Event, error) {
		ch := make(chan feed.Event, 1)
		ch <- feed.Event{Type: "hello", Seq: 1, Hello: &feed.Hello{Dir: dir}}
		return ch, nil
	}
}

// noStart is a Start that fails the test.
func noStart(t *testing.T) func(string, string, []string) (Proc, error) {
	return func(string, string, []string) (Proc, error) {
		t.Error("Start called")
		return nil, errors.New("no start")
	}
}

func TestOpen_AttachSameDir(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), Options{Dir: dir, Addr: "localhost:8080", Alder: "alder",
		Connect: helloFor(dir), Start: noStart(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Owned {
		t.Error("Owned = true, want false")
	}
	ev := <-s.Events
	if ev.Type != "hello" || ev.Hello.Dir != dir {
		t.Errorf("first event = %+v, want the hello", ev)
	}
	if s.Exited() != nil {
		t.Error("Exited() of an attached session is not nil")
	}
}

func TestOpen_OtherApp(t *testing.T) {
	_, err := Open(context.Background(), Options{Dir: t.TempDir(), Addr: "localhost:9000", Alder: "alder",
		Connect: helloFor("/elsewhere"), Start: noStart(t)})
	if err == nil || err.Error() != "localhost:9000 is the dev server of /elsewhere; pass --addr" {
		t.Errorf("err = %v", err)
	}
}

func TestOpen_NotAlder(t *testing.T) {
	_, err := Open(context.Background(), Options{Dir: t.TempDir(), Addr: "localhost:8080", Alder: "alder",
		Connect: func(context.Context, string) (<-chan feed.Event, error) { return nil, feed.ErrNotAlder },
		Start:   noStart(t)})
	if err == nil || err.Error() != "localhost:8080 is serving something that is not alder run; pass --addr" {
		t.Errorf("err = %v", err)
	}
}

// TestOpen_NotAlderReal is TestOpen_NotAlder against the real feed client: a server answering
// /_dev/events with a 404 page, and a listener that accepts connections but never answers.
func TestOpen_NotAlderReal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<h1>not found</h1>"))
	}))
	defer srv.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	for _, addr := range []string{srv.Listener.Addr().String(), ln.Addr().String()} {
		_, err := Open(context.Background(), Options{Dir: t.TempDir(), Addr: addr, Alder: "alder", Start: noStart(t)})
		if want := addr + " is serving something that is not alder run; pass --addr"; err == nil || err.Error() != want {
			t.Errorf("err = %v, want %s", err, want)
		}
	}
}

// TestOpen_CancelledDuringProbe cancels Open's ctx while the probe waits for the hello: Open
// returns the ctx's error, neither panicking on the missing hello nor blaming the address.
func TestOpen_CancelledDuringProbe(t *testing.T) {
	for _, closed := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		connect := func(context.Context, string) (<-chan feed.Event, error) {
			ch := make(chan feed.Event)
			if closed {
				close(ch)
			}
			cancel()
			return ch, nil
		}
		_, err := Open(ctx, Options{Dir: t.TempDir(), Addr: "localhost:8080", Alder: "alder", Connect: connect, Start: noStart(t)})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("closed=%v: err = %v, want context.Canceled", closed, err)
		}
	}
}

func TestOpen_NoRun(t *testing.T) {
	_, err := Open(context.Background(), Options{Dir: t.TempDir(), Addr: "localhost:8080", Alder: "alder", NoRun: true,
		Connect: func(context.Context, string) (<-chan feed.Event, error) { return nil, refused },
		Start:   noStart(t)})
	if err == nil || err.Error() != "no dev server on localhost:8080 (--no-run)" {
		t.Errorf("err = %v", err)
	}
	// The real feed client's error for a closed port is recognised as refused too.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	_, err = Open(context.Background(), Options{Dir: t.TempDir(), Addr: addr, Alder: "alder", NoRun: true, Start: noStart(t)})
	if want := "no dev server on " + addr + " (--no-run)"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %s", err, want)
	}
}

// fakeProc is a Proc whose stderr carries the lines given and whose Stop is counted.
type fakeProc struct {
	stderr chan string
	exited chan struct{}
	code   int
	mu     sync.Mutex
	stops  int
}

func (p *fakeProc) Stderr() <-chan string   { return p.stderr }
func (p *fakeProc) Exited() <-chan struct{} { return p.exited }
func (p *fakeProc) Code() int               { return p.code }
func (p *fakeProc) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stops++
	return nil
}

func TestOpen_Owns(t *testing.T) {
	dir := t.TempDir()
	p := &fakeProc{stderr: make(chan string, 1), exited: make(chan struct{})}
	p.stderr <- "alder run: building"
	var starts int
	var gotEnv []string
	var gotDir string
	var calls int
	var mu sync.Mutex
	var early []string
	lines := func(l string) {
		mu.Lock()
		defer mu.Unlock()
		early = append(early, l)
	}
	connect := func(ctx context.Context, addr string) (<-chan feed.Event, error) {
		calls++
		if calls <= 3 { // the probe, then two retries
			return nil, refused
		}
		// The line the dev server printed has been handed on already, before the feed answered.
		mu.Lock()
		if !slices.Equal(early, []string{"alder run: building"}) {
			t.Errorf("lines before the feed answered = %q", early)
		}
		mu.Unlock()
		return helloFor(dir)(ctx, addr)
	}
	start := func(alder, d string, env []string) (Proc, error) {
		starts++
		gotDir, gotEnv = d, env
		return p, nil
	}
	s, err := Open(context.Background(), Options{Dir: dir, Addr: "localhost:8080", Alder: "alder",
		Connect: connect, Start: start, Lines: lines})
	if err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Errorf("Start called %d times, want 1", starts)
	}
	if gotDir != dir {
		t.Errorf("Start dir = %q, want %q", gotDir, dir)
	}
	for _, want := range []string{"ALDER_EXIT_ON_STDIN_EOF=1", "ALDER_ADDR=localhost:8080"} {
		if !slices.Contains(gotEnv, want) {
			t.Errorf("Start env = %q, want %s in it", gotEnv, want)
		}
	}
	if !s.Owned {
		t.Error("Owned = false, want true")
	}
	mu.Lock()
	if !slices.Equal(early, []string{"alder run: building"}) {
		t.Errorf("lines = %q", early)
	}
	mu.Unlock()
	if ev := <-s.Events; ev.Type != "hello" {
		t.Errorf("first event = %+v, want the hello", ev)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if p.stops != 1 {
		t.Errorf("Stop called %d times, want 1", p.stops)
	}
	// Closing is no exit of the dev server's own: Exited closes without a value.
	select {
	case e, ok := <-s.Exited():
		if ok {
			t.Errorf("Exited delivered %+v after Close", e)
		}
	case <-time.After(time.Second):
		t.Error("Exited still open 1 s after Close")
	}
}

// The child runs in the app's directory, so that alder run's build lands in <dir>/build.
func TestStart_WorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	script := filepath.Join(t.TempDir(), "alder")
	if err := os.WriteFile(script, []byte("#!/bin/sh\npwd -P >&2\ncat >/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := Start(script, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	if l := <-p.Stderr(); l != dir {
		t.Errorf("working directory = %q, want %q", l, dir)
	}
}

func TestProc_DiesWithParentPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	// The script stands in for alder run with ALDER_EXIT_ON_STDIN_EOF=1: it exits once its
	// standard input reaches EOF.
	script := filepath.Join(t.TempDir(), "alder")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"started $1 $ALDER_EXIT_ON_STDIN_EOF\" >&2\ncat >/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := Start(script, t.TempDir(), []string{"ALDER_EXIT_ON_STDIN_EOF=1"})
	if err != nil {
		t.Fatal(err)
	}
	if l := <-p.Stderr(); l != "started run 1" {
		t.Errorf("stderr = %q, want %q", l, "started run 1")
	}
	select {
	case <-p.Exited():
		t.Fatal("exited while its stdin was open")
	case <-time.After(100 * time.Millisecond):
	}
	p.(*proc).stdin.Close() // what the TUI's death does
	select {
	case <-p.Exited():
	case <-time.After(time.Second):
		p.Stop()
		t.Fatal("still running 1 s after its stdin closed")
	}
}

// script is an executable shell script with body, standing in for alder.
func script(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	path := filepath.Join(t.TempDir(), "alder")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// refusing is a Connect for an address nothing listens on.
func refusing(context.Context, string) (<-chan feed.Event, error) { return nil, refused }

func TestOpen_StartTimeout(t *testing.T) {
	p := &fakeProc{stderr: make(chan string, 2), exited: make(chan struct{})}
	p.stderr <- "alder run: building"
	p.stderr <- "alder run: stuck"
	start := func(string, string, []string) (Proc, error) { return p, nil }
	began := time.Now()
	_, err := Open(context.Background(), Options{Dir: t.TempDir(), Addr: "localhost:8080", Alder: "alder",
		Connect: refusing, Start: start, StartTimeout: 300 * time.Millisecond})
	if took := time.Since(began); took > 5*time.Second {
		t.Errorf("Open took %s with a 300ms start timeout", took)
	}
	want := "alder run did not answer on localhost:8080 within 300ms:\nalder run: building\nalder run: stuck"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if p.stops != 1 {
		t.Errorf("Stop called %d times, want 1", p.stops)
	}
}

func TestOpen_ChildExitsEarly(t *testing.T) {
	alder := script(t, "echo 'error: shelf.alder:3: no such type' >&2\nexit 3")
	_, err := Open(context.Background(), Options{Dir: t.TempDir(), Addr: "localhost:8080", Alder: alder,
		Connect: refusing})
	want := "alder run exited (code 3) before its feed answered on localhost:8080:\nerror: shelf.alder:3: no such type"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// TestOpen_NoGoroutineLeak fails Open on its start timeout against a dev server that, once told
// to stop, prints more lines than the stderr channel holds: the goroutine reading them must not
// stay blocked once Open has returned.
func TestOpen_NoGoroutineLeak(t *testing.T) {
	alder := script(t, `trap 'i=0; while [ $i -lt 300 ]; do echo "stopping $i" >&2; i=$((i+1)); done; exit 0' TERM
echo started >&2
while :; do sleep 0.05; done`)
	base := runtime.NumGoroutine()
	_, err := Open(context.Background(), Options{Dir: t.TempDir(), Addr: "localhost:8080", Alder: alder,
		Connect: refusing, StartTimeout: 300 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("err = %v, want the start timeout", err)
	}
	n := runtime.NumGoroutine()
	for deadline := time.Now().Add(3 * time.Second); n > base && time.Now().Before(deadline); n = runtime.NumGoroutine() {
		time.Sleep(20 * time.Millisecond)
	}
	if n > base {
		buf := make([]byte, 1<<16)
		t.Errorf("%d goroutines after the failed Open, %d before:\n%s", n, base, buf[:runtime.Stack(buf, true)])
	}
}

// TestDirCheck_Symlink attaches to a dev server whose hello names the app's directory by another
// path through a symlink: t.TempDir is under /var on macOS, which is /private/var.
func TestDirCheck_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ dir, hello string }{{dir, real}, {real, dir}, {link, real}, {real, link}, {dir, dir}} {
		s, err := Open(context.Background(), Options{Dir: c.dir, Addr: "localhost:8080", Alder: "alder",
			Connect: helloFor(c.hello), Start: noStart(t)})
		if err != nil {
			t.Errorf("dir %s, hello %s: %v", c.dir, c.hello, err)
			continue
		}
		s.Close()
	}
}

// TestProbe_HelloBeatsTimer has the probe's deadline pass in the same tick as the hello arrives:
// the hello counts, every time.
func TestProbe_HelloBeatsTimer(t *testing.T) {
	old := probeAfter
	t.Cleanup(func() { probeAfter = old })
	probeAfter = func(time.Duration) <-chan time.Time {
		c := make(chan time.Time, 1)
		c <- time.Now()
		return c
	}
	dir := t.TempDir()
	for i := range 100 {
		s, err := Open(context.Background(), Options{Dir: dir, Addr: "localhost:8080", Alder: "alder",
			Connect: helloFor(dir), Start: noStart(t)})
		if err != nil {
			t.Fatalf("try %d: %v", i, err)
		}
		s.Close()
	}
}

func TestSession_Exited(t *testing.T) {
	for _, c := range []struct {
		name, body string
		kill       bool
		code       int
		tail       string
	}{
		{"killed", "echo started >&2\nwhile :; do sleep 0.05; done", true, 128 + int(syscall.SIGKILL), ""},
		{"crashed", "echo started >&2\nsleep 0.3\necho 'panic: boom' >&2\necho 'goroutine 1' >&2\nexit 4", false, 4,
			"panic: boom\ngoroutine 1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			alder := script(t, c.body)
			dir := t.TempDir()
			var p Proc
			start := func(alder, dir string, env []string) (Proc, error) {
				var err error
				p, err = Start(alder, dir, env)
				return p, err
			}
			var calls int
			connect := func(ctx context.Context, addr string) (<-chan feed.Event, error) {
				if calls++; calls == 1 {
					return nil, refused
				}
				return helloFor(dir)(ctx, addr)
			}
			s, err := Open(context.Background(), Options{Dir: dir, Addr: "localhost:8080", Alder: alder,
				Connect: connect, Start: start})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if c.kill {
				p.(*proc).cmd.Process.Kill()
			}
			select {
			case e, ok := <-s.Exited():
				if !ok {
					t.Fatal("Exited closed without an Exit")
				}
				if e.Code != c.code || e.StderrTail != c.tail {
					t.Errorf("Exit = %+v, want code %d and tail %q", e, c.code, c.tail)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("no Exit 5 s after the dev server died")
			}
		})
	}
}
