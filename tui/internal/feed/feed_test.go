package feed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixture is the lines of the feed fixture name under tui/testdata/feeds.
func fixture(t *testing.T, name string) [][]byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "feeds", name))
	if err != nil {
		t.Fatal(err)
	}
	var lines [][]byte
	for _, l := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
		lines = append(lines, l)
	}
	return lines
}

// decodeAll is lines decoded, failing the test on an error.
func decodeAll(t *testing.T, lines [][]byte) []Event {
	t.Helper()
	var evs []Event
	for i, l := range lines {
		ev, err := Decode(l)
		if err != nil {
			t.Fatalf("line %d: %v\n%s", i+1, err, l)
		}
		evs = append(evs, ev)
	}
	return evs
}

func TestDecode_Fixtures(t *testing.T) {
	for _, name := range []string{"basic.ndjson", "failed.ndjson", "exited.ndjson"} {
		decodeAll(t, fixture(t, name))
	}

	basic := decodeAll(t, fixture(t, "basic.ndjson"))
	h := basic[0]
	if h.Type != "hello" || h.Hello == nil {
		t.Fatalf("first event = %+v, want a hello", h)
	}
	if h.Hello.App != "shelf" || h.Hello.Build != "ok" || h.Hello.Boot != "5f0c1a2e" || h.Hello.First != 1 ||
		h.Hello.Child == nil || *h.Hello.Child != "127.0.0.1:52507" || h.Hello.Since.IsZero() || h.Seq != 13 {
		t.Errorf("hello = %+v (child %v)", h.Hello, h.Hello.Child)
	}
	if b := basic[1]; b.Type != "build" || b.Build == nil || b.Build.State != "ok" || b.Build.Child != "127.0.0.1:52507" || b.Build.Ms != 1409 {
		t.Errorf("build = %+v", b.Build)
	}
	if a := basic[2]; a.Type != "app" || a.App == nil || a.App.State != "started" || a.App.Child != "127.0.0.1:52507" {
		t.Errorf("app = %+v", a.App)
	}
	if r := basic[3]; r.Type != "run" || r.Run == nil || !strings.HasPrefix(r.Run.Text, "dev server on") {
		t.Errorf("run = %+v", r.Run)
	}

	var access []*Record
	var rid []*Record
	for _, ev := range basic {
		if ev.Type != "log" || ev.Log == nil {
			continue
		}
		if r := ev.Log.Record; r != nil {
			if r.HasAccess {
				access = append(access, r)
			}
			if r.Rid == "5f3a2c1f" && !r.HasAccess {
				rid = append(rid, r)
			}
		}
	}
	if len(access) != 6 {
		t.Fatalf("access records = %d, want 6", len(access))
	}
	if n := len(rid); n != 3 {
		t.Errorf("log lines of rid 5f3a2c1f = %d, want 3", n)
	}
	book := access[1]
	if book.Method != "GET" || book.Path != "/books/42" || book.Status != 200 || book.Repeats != 1 || book.DBStmts != 7 ||
		book.Steps != 5 || book.Bytes != 6377 || book.Ms != 12.8 || book.DBMs != 1.8 || book.Level != "INFO" || book.Time.IsZero() {
		t.Errorf("GET /books/42 = %+v", book)
	}
	if lend := access[2]; lend.Method != "POST" || lend.Status != 303 || lend.DBStmts != 2 {
		t.Errorf("POST lend = %+v", lend)
	}
	slow := access[4]
	if !slow.Slow || slow.Status != 500 || slow.Ms != 812.4 || slow.Route != "/search" || slow.Path != "/search?q=••••" || slow.Level != "WARN" {
		t.Errorf("slow = %+v", slow)
	}
	if css := access[5]; css.Level != "DEBUG" || css.Path != "/static/app.css" {
		t.Errorf("static = %+v", css)
	}
	if last := basic[len(basic)-1]; last.Log == nil || last.Log.Record != nil || !strings.HasPrefix(last.Log.Raw, "panic:") {
		t.Errorf("last = %+v", last.Log)
	}

	failed := decodeAll(t, fixture(t, "failed.ndjson"))
	if h := failed[0].Hello; h == nil || h.Build != "failed" || h.Child != nil {
		t.Errorf("failed hello = %+v", h)
	}
	b := failed[1].Build
	if b == nil || b.State != "failed" || len(b.Diagnostics) != 2 {
		t.Fatalf("failed build = %+v", b)
	}
	want := Diag{Code: "E0002", Severity: "error", File: "app/Books.al", Line: 14, Col: 12, EndLine: 14, EndCol: 20, Message: "unknown name `findBook`"}
	if b.Diagnostics[0] != want {
		t.Errorf("diagnostic = %+v, want %+v", b.Diagnostics[0], want)
	}

	exited := decodeAll(t, fixture(t, "exited.ndjson"))
	a := exited[0].App
	if a == nil || a.State != "exited" || a.Exit == nil || a.Exit.Code != 2 || a.Exit.Signal != "" || len(a.Tail) != 3 {
		t.Errorf("exited = %+v", a)
	}
}

func TestDecode_Static(t *testing.T) {
	ev, err := Decode([]byte(`{"seq":4,"t":"2026-11-01T14:02:10Z","type":"static"}`))
	if err != nil || ev.Type != "static" || ev.Seq != 4 || ev.T.IsZero() {
		t.Errorf("Decode(static) = %+v, %v", ev, err)
	}
	if _, err := Decode([]byte(`not json`)); err == nil {
		t.Error("Decode(not json) did not fail")
	}
	if _, err := Decode([]byte(`{"seq":1}`)); err == nil {
		t.Error("Decode(no type) did not fail")
	}
}

// feedServer is an alder dev server's /_dev/events for the tests: each request is answered by
// the next of its scripts, which writes data lines and returns to end the stream (or blocks
// until the client goes away). It records each request's ?after.
type feedServer struct {
	mu      sync.Mutex
	scripts []func(w http.ResponseWriter, r *http.Request)
	afters  []string
}

func (s *feedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.afters = append(s.afters, r.URL.Query().Get("after"))
	var script func(w http.ResponseWriter, r *http.Request)
	if len(s.scripts) > 0 {
		script, s.scripts = s.scripts[0], s.scripts[1:]
	}
	s.mu.Unlock()
	if script == nil {
		http.Error(w, "no more scripts", http.StatusServiceUnavailable)
		return
	}
	script(w, r)
}

func (s *feedServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.afters...)
}

// serve is a script answering as a dev server whose hello and ring are lines (a hello first,
// then the ring's events): the hello, then the events with a seq above the request's ?after, or
// all of them when after is above the hello's seq, as run_events.go does (lines without a hello
// are sent whole). A ping goes between events. It ends the stream when hold is false, or keeps
// it open until the client goes away.
func serve(lines [][]byte, hold bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		send := lines
		if a := r.URL.Query().Get("after"); a != "" && len(lines) > 0 {
			after, err := strconv.ParseInt(a, 10, 64)
			hello, herr := Decode(lines[0])
			if err == nil && herr == nil && hello.Type == "hello" && after <= hello.Seq {
				send = [][]byte{lines[0]}
				for _, l := range lines[1:] {
					if ev, err := Decode(l); err == nil && ev.Seq > after {
						send = append(send, l)
					}
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range send {
			fmt.Fprintf(w, "data: %s\n\n: ping\n\n", l)
		}
		w.(http.Flusher).Flush()
		if hold {
			<-r.Context().Done()
		}
	}
}

func startFeed(t *testing.T, scripts ...func(w http.ResponseWriter, r *http.Request)) (*feedServer, string) {
	t.Helper()
	s := &feedServer{scripts: scripts}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return s, strings.TrimPrefix(srv.URL, "http://")
}

// take is the next n events of ch, failing the test when they take too long.
func take(t *testing.T, ch <-chan Event, n int) []Event {
	t.Helper()
	var evs []Event
	for len(evs) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed after %d events, want %d", len(evs), n)
			}
			evs = append(evs, ev)
		case <-time.After(5 * time.Second):
			t.Fatalf("got %d events, want %d", len(evs), n)
		}
	}
	return evs
}

// shape is evs as "type:seq" words, the synthetic disconnected one as "disconnected".
func shape(evs []Event) string {
	var w []string
	for _, ev := range evs {
		if ev.Type == "disconnected" {
			w = append(w, ev.Type)
			continue
		}
		w = append(w, fmt.Sprintf("%s:%d", ev.Type, ev.Seq))
	}
	return strings.Join(w, " ")
}

func TestConnect_Replay(t *testing.T) {
	lines := fixture(t, "basic.ndjson")
	_, addr := startFeed(t, serve(lines, true))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	got := take(t, ch, len(lines))
	if want := decodeAll(t, lines); !reflect.DeepEqual(got, want) {
		t.Errorf("events:\n%s\nwant:\n%s", shape(got), shape(want))
	}
	cancel()
	select {
	case _, ok := <-ch:
		for ok {
			_, ok = <-ch
		}
	case <-time.After(5 * time.Second):
		t.Fatal("channel not closed after the context ended")
	}
}

func TestConnect_Resumes(t *testing.T) {
	lines := fixture(t, "basic.ndjson")
	hello, events := lines[0], lines[1:]
	s, addr := startFeed(t,
		serve(append([][]byte{hello}, events[:3]...), false),
		serve(lines, true),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	got := shape(take(t, ch, 1+3+1+1+len(events)-3))
	want := "hello:13 build:1 app:2 run:3 disconnected hello:13 log:4 log:5 log:6 log:7 log:8 log:9 log:10 log:11 log:12 log:13"
	if got != want {
		t.Errorf("events:\n%s\nwant:\n%s", got, want)
	}
	if r := s.requests(); !reflect.DeepEqual(r, []string{"", "3"}) {
		t.Errorf("after of each request = %q, want [\"\" \"3\"]", r)
	}
}

func TestConnect_NewBoot(t *testing.T) {
	lines := fixture(t, "basic.ndjson")
	hello, events := lines[0], lines[1:]
	newHello := bytes.Replace(bytes.Replace(hello, []byte(`"boot":"5f0c1a2e"`), []byte(`"boot":"77aa0e19"`), 1),
		[]byte(`"seq":13`), []byte(`"seq":2`), 1)
	s, addr := startFeed(t,
		serve(append([][]byte{hello}, events[:3]...), false),
		serve([][]byte{newHello, events[0], events[1]}, true),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	evs := take(t, ch, 1+3+1+3)
	if got, want := shape(evs), "hello:13 build:1 app:2 run:3 disconnected hello:2 build:1 app:2"; got != want {
		t.Errorf("events:\n%s\nwant:\n%s", got, want)
	}
	if h := evs[5].Hello; h == nil || h.Boot != "77aa0e19" {
		t.Errorf("second hello = %+v, want boot 77aa0e19", h)
	}
	if r := s.requests(); !reflect.DeepEqual(r, []string{"", "3"}) {
		t.Errorf("after of each request = %q", r)
	}
}

// A new dev server that has already emitted as many events as the client saw on the old one
// replays only the events above the old seq to ?after; the client opens its feed again without
// after, so none of the new server's first events are lost.
func TestConnect_NewBootAhead(t *testing.T) {
	lines := fixture(t, "basic.ndjson")
	hello, events := lines[0], lines[1:]
	newHello := bytes.Replace(bytes.Replace(hello, []byte(`"boot":"5f0c1a2e"`), []byte(`"boot":"77aa0e19"`), 1),
		[]byte(`"seq":13`), []byte(`"seq":5`), 1)
	newRing := append([][]byte{newHello}, events[:5]...)
	s, addr := startFeed(t,
		serve(append([][]byte{hello}, events[:3]...), false),
		serve(newRing, false),
		serve(newRing, true),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	evs := take(t, ch, 1+3+1+1+5)
	if got, want := shape(evs), "hello:13 build:1 app:2 run:3 disconnected hello:5 build:1 app:2 run:3 log:4 log:5"; got != want {
		t.Errorf("events:\n%s\nwant:\n%s", got, want)
	}
	if h := evs[5].Hello; h == nil || h.Boot != "77aa0e19" {
		t.Errorf("second hello = %+v, want boot 77aa0e19", h)
	}
	select {
	case ev := <-ch:
		t.Errorf("extra event %s", shape([]Event{ev}))
	case <-time.After(100 * time.Millisecond):
	}
	if r := s.requests(); !reflect.DeepEqual(r, []string{"", "3", ""}) {
		t.Errorf("after of each request = %q, want [\"\" \"3\" \"\"]", r)
	}
}

func TestConnect_NotAlder(t *testing.T) {
	_, addr := startFeed(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "<!doctype html><title>Not found</title>")
	})
	if _, err := Connect(context.Background(), addr); !errors.Is(err, ErrNotAlder) {
		t.Errorf("Connect(404 HTML) = %v, want ErrNotAlder", err)
	}

	// An event stream whose first event is not a hello is not a dev server either.
	_, addr = startFeed(t, serve(fixture(t, "basic.ndjson")[1:], true))
	if _, err := Connect(context.Background(), addr); !errors.Is(err, ErrNotAlder) {
		t.Errorf("Connect(no hello) = %v, want ErrNotAlder", err)
	}

	// Nothing listening is another error: the caller starts its own dev server.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	free := l.Addr().String()
	l.Close()
	if _, err := Connect(context.Background(), free); err == nil || errors.Is(err, ErrNotAlder) {
		t.Errorf("Connect(nothing listening) = %v, want a connection error", err)
	}
}

// A log record whose typed decode fails (a key of the wrong type) is kept as raw text, the
// record's JSON, rather than dropped.
func TestDecode_BadAttrKeepsRaw(t *testing.T) {
	rec := `{"time":"2026-11-01T14:02:11Z","level":"INFO","msg":"hi","repeats":"x"}`
	ev, err := Decode([]byte(`{"type":"log","seq":4,"t":"2026-11-01T14:02:11Z","record":` + rec + `}`))
	if err != nil {
		t.Fatalf("Decode = %v, want the log kept", err)
	}
	if ev.Type != "log" || ev.Seq != 4 || ev.Log == nil || ev.Log.Record != nil || ev.Log.Raw != rec {
		t.Errorf("Decode = %+v (log %+v), want a log with Raw %s and no record", ev, ev.Log, rec)
	}
}

// helloOnly is a script answering with a hello and then nothing at all, no pings either, until
// the client goes away: a half-open connection, as the client sees it.
func helloOnly(hello []byte) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", hello)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}
}

func TestClient_IdleDeadline(t *testing.T) {
	_, addr := startFeed(t, helloOnly(fixture(t, "basic.ndjson")[0]))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	ch, err := ConnectWith(ctx, addr, Options{Idle: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if ev := take(t, ch, 1)[0]; ev.Type != "hello" {
		t.Fatalf("first event = %s, want hello", shape([]Event{ev}))
	}
	select {
	case ev := <-ch:
		if ev.Type != "disconnected" {
			t.Errorf("after a silent stream got %s, want disconnected", shape([]Event{ev}))
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("disconnected after %v, want within 1s", d)
		}
	case <-time.After(time.Second):
		t.Fatal("a silent stream still connected after 1s with Idle 200ms")
	}
}

// Pings alone push the idle deadline back: a stream that carries only pings stays connected.
func TestClient_PingsKeepAlive(t *testing.T) {
	hello := fixture(t, "basic.ndjson")[0]
	_, addr := startFeed(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", hello)
		w.(http.Flusher).Flush()
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
				fmt.Fprint(w, ": ping\n\n")
				w.(http.Flusher).Flush()
			}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := ConnectWith(ctx, addr, Options{Idle: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	take(t, ch, 1)
	select {
	case ev := <-ch:
		t.Errorf("a pinged stream got %s", shape([]Event{ev}))
	case <-time.After(700 * time.Millisecond):
	}
}

// A reconnect to the same dev server whose ring no longer holds the events after the last one
// seen (its hello's first is above last+1) gets a gap event, after the hello, with the count of
// the events lost.
func TestClient_Gap(t *testing.T) {
	lines := fixture(t, "basic.ndjson")
	hello, events := lines[0], lines[1:]
	// The client saw seqs 1 to 3; meanwhile the ring moved on and keeps 8 to 13 (last+5).
	later := bytes.Replace(hello, []byte(`"first":1`), []byte(`"first":8`), 1)
	_, addr := startFeed(t,
		serve(append([][]byte{hello}, events[:3]...), false),
		serve(append([][]byte{later}, events[7:]...), true),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	evs := take(t, ch, 1+3+1+1+1+6)
	if got, want := shape(evs), "hello:13 build:1 app:2 run:3 disconnected hello:13 gap:0 log:8 log:9 log:10 log:11 log:12 log:13"; got != want {
		t.Errorf("events:\n%s\nwant:\n%s", got, want)
	}
	if g := evs[6].Gap; g == nil || g.Missed != 4 {
		t.Errorf("gap = %+v, want Missed 4", g)
	}
}

// Neither a reconnect whose ring still holds the next event, nor the first connection to a dev
// server whose ring has already moved on, makes a gap.
func TestClient_NoGap(t *testing.T) {
	lines := fixture(t, "basic.ndjson")
	hello, events := lines[0], lines[1:]
	moved := bytes.Replace(hello, []byte(`"first":1`), []byte(`"first":4`), 1)
	_, addr := startFeed(t,
		serve(append([][]byte{moved}, events[3:5]...), false),
		serve(append([][]byte{moved}, events[3:]...), true),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	evs := take(t, ch, 1+2+1+1+8)
	if got, want := shape(evs), "hello:13 log:4 log:5 disconnected hello:13 log:6 log:7 log:8 log:9 log:10 log:11 log:12 log:13"; got != want {
		t.Errorf("events:\n%s\nwant:\n%s", got, want)
	}
}
