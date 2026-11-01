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
	mu     sync.Mutex
	stops  int
}

func (p *fakeProc) Stderr() <-chan string   { return p.stderr }
func (p *fakeProc) Exited() <-chan struct{} { return p.exited }
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
	connect := func(ctx context.Context, addr string) (<-chan feed.Event, error) {
		calls++
		if calls <= 3 { // the probe, then two retries
			return nil, refused
		}
		return helloFor(dir)(ctx, addr)
	}
	start := func(alder, d string, env []string) (Proc, error) {
		starts++
		gotDir, gotEnv = d, env
		return p, nil
	}
	s, err := Open(context.Background(), Options{Dir: dir, Addr: "localhost:8080", Alder: "alder",
		Connect: connect, Start: start})
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
	var early []string
	for l := range s.Early {
		early = append(early, l)
	}
	if !slices.Equal(early, []string{"alder run: building"}) {
		t.Errorf("Early = %q", early)
	}
	if ev := <-s.Events; ev.Type != "hello" {
		t.Errorf("first event = %+v, want the hello", ev)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if p.stops != 1 {
		t.Errorf("Stop called %d times, want 1", p.stops)
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
	p, err := Start(script, "/app", []string{"ALDER_EXIT_ON_STDIN_EOF=1"})
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
