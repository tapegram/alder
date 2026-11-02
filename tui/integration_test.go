// The live test: the TUI's dev server, its feed and the dev endpoints against a real `alder run`
// of the fixture app in testdata/shelf. It needs alder (ALDER_BIN, or alder on PATH) and Go, and
// is skipped without alder and under -short.
package tui_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"alder/tui/internal/data"
	"alder/tui/internal/devproc"
	"alder/tui/internal/feed"
	"alder/tui/internal/routes"
	"alder/tui/internal/runner"
	"alder/tui/internal/testrun"
)

// buildWait bounds the wait for the first build: a cold Go cache compiles the app's SQLite driver.
const buildWait = 4 * time.Minute

// TestLive_HelloAndFirstRequest starts `alder run` on a copy of the shelf app through devproc,
// waits for its first good build, opens /books/1 (whose load looks up each book's loan on its own)
// and checks what the feed and the dev endpoints report about that request: an access record
// that feed.Decode reads, with its N+1 counted, and the record's steps at /_dev/requests/<rid>.
// Close then stops `alder run`, and the address is free again within 10 s.
func TestLive_HelloAndFirstRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("the live test runs alder run; skipped under -short")
	}
	alder := os.Getenv("ALDER_BIN")
	if alder == "" {
		p, err := exec.LookPath("alder")
		if err != nil {
			t.Skip("alder is not on PATH and ALDER_BIN is not set")
		}
		alder = p
	}
	dir := filepath.Join(t.TempDir(), "shelf")
	copyApp(t, "testdata/shelf", dir)
	addr := freeAddr(t)

	ctx, cancel := context.WithTimeout(context.Background(), buildWait+time.Minute)
	defer cancel()
	s, err := devproc.Open(ctx, devproc.Options{Dir: dir, Addr: addr, Alder: alder})
	if err != nil {
		t.Fatalf("devproc.Open: %v", err)
	}
	closed := false
	defer func() {
		if !closed {
			s.Close()
		}
	}()
	if !s.Owned {
		t.Fatal("Open attached to a dev server it should have started")
	}

	hello := next(t, s.Events, time.Second, func(ev feed.Event) bool { return true })
	if hello.Type != "hello" || hello.Hello.App != "shelf" || filepath.Clean(hello.Hello.Dir) != mustAbs(t, dir) {
		t.Fatalf("first event = %+v, want the hello of shelf in %s", hello, dir)
	}
	if hello.Hello.Build != "ok" {
		next(t, s.Events, buildWait, func(ev feed.Event) bool {
			if ev.Type == "build" && ev.Build.State == "failed" {
				t.Fatalf("the build failed: %s %+v", ev.Build.Text, ev.Build.Diagnostics)
			}
			return ev.Type == "build" && ev.Build.State == "ok"
		})
	}

	status, body := get(t, "http://"+addr+"/books/1")
	if status != http.StatusOK || !bytes.Contains(body, []byte("Middlemarch")) {
		t.Fatalf("GET /books/1 = %d:\n%s", status, body)
	}

	acc := next(t, s.Events, 10*time.Second, func(ev feed.Event) bool {
		return ev.Type == "log" && ev.Log.Record != nil && ev.Log.Record.HasAccess && ev.Log.Record.Path == "/books/1"
	})
	rec := acc.Log.Record
	if rec.Status != http.StatusOK || rec.Rid == "" {
		t.Errorf("access record = %+v, want status 200 and a rid", rec)
	}
	if rec.Repeats < 1 {
		t.Errorf("access record repeats = %d, want at least 1 (the per-book loan lookups)", rec.Repeats)
	}
	if raw := rawEvent(t, addr, acc.Seq); raw.Log == nil || raw.Log.Record == nil || *raw.Log.Record != *rec {
		t.Errorf("feed.Decode of the event's data line = %+v, want the record %+v", raw.Log, rec)
	}

	status, body = get(t, "http://"+addr+"/_dev/requests/"+rec.Rid)
	var detail struct {
		Steps []struct {
			Name string `json:"name"`
		} `json:"steps"`
	}
	if status != http.StatusOK {
		t.Fatalf("GET /_dev/requests/%s = %d:\n%s", rec.Rid, status, body)
	}
	if err := json.Unmarshal(body, &detail); err != nil || len(detail.Steps) == 0 {
		t.Errorf("GET /_dev/requests/%s: want steps, got %s (%v)", rec.Rid, body, err)
	}

	closed = true
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		l, err := net.Listen("tcp", addr)
		if err == nil {
			l.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s is still taken 10 s after Close: %v", addr, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestLive_Screens checks what the Tests, Routes and Data screens read against a real alder and
// a real `alder run` of a copy of the shelf app: `alder test --json` through runner.Exec gives at
// least one passing result, `alder routes --json` lists /books/{id}, and the SQL name that
// /_dev/map.json gives the Book table is one that sqlite_master of build/shelf/dev.db lists.
func TestLive_Screens(t *testing.T) {
	if testing.Short() {
		t.Skip("the live test runs alder run; skipped under -short")
	}
	alder := os.Getenv("ALDER_BIN")
	if alder == "" {
		p, err := exec.LookPath("alder")
		if err != nil {
			t.Skip("alder is not on PATH and ALDER_BIN is not set")
		}
		alder = p
	}
	dir := filepath.Join(t.TempDir(), "shelf")
	copyApp(t, "testdata/shelf", dir)
	addr := freeAddr(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*buildWait+time.Minute)
	defer cancel()
	s, err := devproc.Open(ctx, devproc.Options{Dir: dir, Addr: addr, Alder: alder})
	if err != nil {
		t.Fatalf("devproc.Open: %v", err)
	}
	defer s.Close()
	next(t, s.Events, buildWait, func(ev feed.Event) bool {
		if ev.Type == "build" && ev.Build.State == "failed" {
			t.Fatalf("the build failed: %s %+v", ev.Build.Text, ev.Build.Diagnostics)
		}
		return ev.Type == "hello" && ev.Hello.Build == "ok" || ev.Type == "build" && ev.Build.State == "ok"
	})

	// The Data screen: the map's SQL name for Book is a table of the database file.
	m, err := data.FetchMap(ctx, http.DefaultClient, addr)
	if err != nil {
		t.Fatalf("data.FetchMap: %v", err)
	}
	var book *data.Table
	for i, tb := range m.Tables {
		if strings.HasSuffix(tb.Name, ".Book") {
			book = &m.Tables[i]
		}
	}
	if book == nil || book.SQL == "" {
		t.Fatalf("the map has no Book table with an SQL name: %+v", m.Tables)
	}
	dbs := data.Databases(filepath.Join(dir, "build", "shelf"), m)
	db, err := data.Open(dbs[0].Path)
	if err != nil {
		t.Fatalf("data.Open(%s): %v", dbs[0].Path, err)
	}
	defer db.Close()
	_, rows, err := db.Query(ctx, "SELECT name FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		t.Fatalf("listing the tables of %s: %v", dbs[0].Path, err)
	}
	listed := false
	for _, row := range rows {
		listed = listed || len(row) == 1 && row[0] == book.SQL
	}
	if !listed {
		t.Errorf("sqlite_master lists %v, not the map's SQL name %q for %s", rows, book.SQL, book.Name)
	}
	if n, err := db.Count(ctx, *book); err != nil || n < 1 {
		t.Errorf("Count(%s) = %d, %v; want the seeded books", book.SQL, n, err)
	}

	// The Tests screen: the suite, as the screen starts it.
	r := runner.Exec(alder)
	res, err := r.Run(ctx, dir, testrun.Args(nil, false)...)
	if err != nil {
		t.Fatalf("alder test --json: %v", err)
	}
	var run testrun.Run
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		testrun.Parse(line, &run)
	}
	passed := 0
	for _, tr := range run.Results {
		if tr.OK {
			passed++
		}
	}
	if passed < 1 {
		t.Errorf("alder test --json gave %d passing results (exit %d, error %q):\n%s%s", passed, res.Code, run.Error, res.Stdout, res.Stderr)
	}

	// The Routes screen.
	res, err = r.Run(ctx, dir, "routes", "--json")
	if err != nil || res.Code != 0 {
		t.Fatalf("alder routes --json: %v (exit %d):\n%s", err, res.Code, res.Stderr)
	}
	rs, err := routes.Parse(res.Stdout)
	if err != nil {
		t.Fatalf("routes.Parse: %v\n%s", err, res.Stdout)
	}
	found := false
	for _, rt := range rs {
		found = found || rt.Kind == "page" && rt.Path == "/books/{id}"
	}
	if !found {
		t.Errorf("alder routes --json lacks the page /books/{id}: %+v", rs)
	}
}

// copyApp copies the app in src to dst, leaving out its build directory, which a local `alder
// test` of the fixture may have left behind.
func copyApp(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "build" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatalf("copying %s: %v", src, err)
	}
}

// freeAddr is a loopback address nothing listens on: the kernel picks the port.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// next is the first event of ch that ok accepts, failing the test when ch closes or wait passes
// first.
func next(t *testing.T, ch <-chan feed.Event, wait time.Duration, ok func(feed.Event) bool) feed.Event {
	t.Helper()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case ev, open := <-ch:
			if !open {
				t.Fatal("the feed closed")
			}
			if ok(ev) {
				return ev
			}
		case <-timer.C:
			t.Fatalf("no matching event within %s", wait)
		}
	}
}

// get is the status and body of a GET of url.
func get(t *testing.T, url string) (int, []byte) {
	t.Helper()
	c := http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp.StatusCode, body
}

// rawEvent reads the feed itself (GET /_dev/events?after=seq-1) and is feed.Decode of the data
// line of the event numbered seq: the decoder the NDJSON fixtures go through, on the real wire.
func rawEvent(t *testing.T, addr string, seq int64) feed.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/_dev/events?after="+strconv.FormatInt(seq-1, 10), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /_dev/events: %v", err)
	}
	defer resp.Body.Close()
	r := bufio.NewReader(resp.Body)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			t.Fatalf("reading /_dev/events for seq %d: %v", seq, err)
		}
		data, ok := bytes.CutPrefix(bytes.TrimRight(line, "\r\n"), []byte("data: "))
		if !ok {
			continue
		}
		ev, err := feed.Decode(data)
		if err != nil {
			t.Fatalf("feed.Decode(%s): %v", data, err)
		}
		if ev.Type != "hello" && ev.Seq == seq {
			return ev
		}
	}
}
