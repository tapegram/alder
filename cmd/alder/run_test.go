package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// devRun is an `alder run` started by startRun.
type devRun struct {
	dir    string        // the app's directory (a copy of examples/todos)
	base   string        // http://<dev server address>
	child  string        // the first child's address
	out    *syncBuffer   // runRunTo's stdout
	errs   *syncBuffer   // runRunTo's stderr
	done   chan struct{} // closed when runRunTo has returned
	code   int
	closer sync.Once
	stopCh chan struct{}
}

// stop closes runRunTo's stop channel and waits (15 s at most) for it to return.
func (r *devRun) stop(t *testing.T) {
	t.Helper()
	r.closer.Do(func() { close(r.stopCh) })
	select {
	case <-r.done:
	case <-time.After(15 * time.Second):
		t.Fatalf("alder run still running 15 s after stop; stderr:\n%s", r.errs.String())
	}
}

// syncBuffer is a strings.Builder safe for one writer and concurrent readers.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// startRun copies examples/todos into a temporary directory (the test's working directory),
// runs runRunTo on it with ALDER_ADDR=127.0.0.1:0, and waits (120 s at most) for its
// `dev server on <addr> → app on <child>` line. The dev server and its child are stopped when
// the test ends.
func startRun(t *testing.T) *devRun {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	src, err := filepath.Abs("../../examples/todos")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	copyDir(t, src, filepath.Join(tmp, "todos"))
	t.Chdir(tmp)
	t.Setenv("ALDER_ADDR", "127.0.0.1:0")
	r := &devRun{dir: filepath.Join(tmp, "todos"), out: &syncBuffer{}, errs: &syncBuffer{},
		done: make(chan struct{}), stopCh: make(chan struct{})}
	go func() {
		defer close(r.done)
		r.code = runRunTo([]string{"todos"}, r.out, r.errs, r.stopCh)
	}()
	t.Cleanup(func() { r.stop(t) })
	deadline := time.Now().Add(120 * time.Second)
	for {
		for _, line := range strings.Split(r.out.String(), "\n") {
			if rest, ok := strings.CutPrefix(line, "dev server on "); ok {
				dev, child, ok := strings.Cut(rest, " → app on ")
				if !ok {
					t.Fatalf("malformed line %q", line)
				}
				r.base, r.child = "http://"+dev, child
				return r
			}
		}
		select {
		case <-r.done:
			t.Fatalf("alder run exited %d before serving; stderr:\n%s", r.code, r.errs.String())
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatalf("no `dev server on` line within 120 s; stderr:\n%s", r.errs.String())
		}
	}
}

// get GETs url (without following redirects) and returns the body, failing unless 200.
func get(t *testing.T, url string) string {
	t.Helper()
	c := &http.Client{Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d:\n%s", url, res.StatusCode, b)
	}
	return string(b)
}

// sse subscribes to url and yields each event block (its lines, without the blank line that
// ends it). The subscription is closed when the test ends.
func sse(t *testing.T, url string) <-chan string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		cancel()
		res.Body.Close()
		t.Fatalf("GET %s = %d %q", url, res.StatusCode, res.Header.Get("Content-Type"))
	}
	events := make(chan string, 100)
	read := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		res.Body.Close()
		<-read
	})
	go func() {
		defer close(read)
		defer close(events)
		sc := bufio.NewScanner(res.Body)
		var block []string
		for sc.Scan() {
			if sc.Text() != "" {
				block = append(block, sc.Text())
				continue
			}
			if len(block) > 0 {
				select {
				case events <- strings.Join(block, "\n"):
				default: // the test stopped listening
				}
			}
			block = nil
		}
	}()
	// the ": ready" comment: the subscription is registered
	if first := waitFor(t, events, ": ready", 10*time.Second); len(first) != 1 {
		t.Fatalf("events before : ready: %q", first)
	}
	return events
}

// waitFor reads events until one contains want (failing after timeout) and returns every event
// read, the match last.
func waitFor(t *testing.T, events <-chan string, want string, timeout time.Duration) []string {
	t.Helper()
	var seen []string
	deadline := time.After(timeout)
	for {
		select {
		case e, ok := <-events:
			if !ok {
				t.Fatalf("the event stream ended before %q; events: %q", want, seen)
			}
			seen = append(seen, e)
			if strings.Contains(e, want) {
				return seen
			}
		case <-deadline:
			t.Fatalf("no event with %q within %v; events: %q", want, timeout, seen)
		}
	}
}

// edit replaces old with new in the file at dir/rel (old must occur).
func edit(t *testing.T, dir, rel, old, new string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s lacks %q", rel, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

const wantReload = "event: datastar-patch-elements\ndata: selector body\ndata: mode append\ndata: elements <script data-effect=\"el.remove()\">location.reload()</script>"

// The dev server proxies the app, whose pages carry the overlay target and the reload
// subscriber; /_dev/reload is an event stream.
func TestRun_ServesAndInjectsSubscriber(t *testing.T) {
	r := startRun(t)
	body := get(t, r.base+"/login")
	for _, want := range []string{`id="_alder_errors"`, `data-init="@get('/_dev/reload')"`, "/_alder/datastar.js", `type="password"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /login lacks %q:\n%s", want, body)
		}
	}
	// redirects and cookies pass through unchanged
	c := &http.Client{Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Get(r.base + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("GET / = %d %q, want 303 /login", res.StatusCode, res.Header.Get("Location"))
	}
	if res, err = c.Get(r.base + "/login"); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Set-Cookie"), "alder_session=") {
		t.Fatalf("GET /login Set-Cookie %q", res.Header.Get("Set-Cookie"))
	}
	// the app's Datastar answers (server-sent events) stream through, with the overlay target
	req, _ := http.NewRequest("GET", r.base+"/nope", nil)
	req.Header.Set("Datastar-Request", "true")
	if res, err = c.Do(req); err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" ||
		!strings.HasPrefix(string(b), "event: datastar-patch-elements\ndata: selector body\ndata: mode outer\n") || !strings.Contains(string(b), `<div id="_alder_errors"></div>`) {
		t.Fatalf("Datastar GET /nope = %d %q:\n%s", res.StatusCode, res.Header.Get("Content-Type"), b)
	}
	sse(t, r.base+"/_dev/reload")
}

// Saving a source file rebuilds the app, pushes a reload, and the new build serves.
func TestRun_EditReloads(t *testing.T) {
	r := startRun(t)
	events := sse(t, r.base+"/_dev/reload")
	edit(t, r.dir, "app/Layout.alder", `"Dark mode"`, `"Dark mode!"`)
	waitFor(t, events, wantReload, 60*time.Second)
	if body := get(t, r.base+"/login"); !strings.Contains(body, "Dark mode!") {
		t.Fatalf("the new build is not serving:\n%s", body)
	}
}

// A type error is overlaid (escaped) while the old build keeps serving; fixing it clears the
// overlay, then reloads.
func TestRun_ErrorThenFixReloads(t *testing.T) {
	r := startRun(t)
	events := sse(t, r.base+"/_dev/reload")
	edit(t, r.dir, "app/Todo.alder", "|> List.length\n", "|> List.length |> Text.length\n")
	seen := waitFor(t, events, "selector #_alder_errors", 60*time.Second)
	overlay := seen[len(seen)-1]
	for _, want := range []string{"data: mode inner", `data: elements <pre class="_alder-errors">`, "E0003", "Todo.alder", "(\\t -&gt; not t.done)"} {
		if !strings.Contains(overlay, want) {
			t.Fatalf("overlay lacks %q:\n%s", want, overlay)
		}
	}
	if !strings.Contains(r.errs.String(), "E0003") {
		t.Fatalf("stderr lacks the diagnostic:\n%s", r.errs.String())
	}
	get(t, r.base+"/login") // the old child still serves
	// a page opened now gets the overlay at once
	if later := waitFor(t, sse(t, r.base+"/_dev/reload"), "E0003", 10*time.Second); len(later) != 1 {
		t.Fatalf("a new subscriber's events: %q", later)
	}

	edit(t, r.dir, "app/Todo.alder", "|> List.length |> Text.length\n", "|> List.length\n")
	seen = waitFor(t, events, wantReload, 60*time.Second)
	if len(seen) < 2 || seen[len(seen)-2] != "event: datastar-patch-elements\ndata: selector #_alder_errors\ndata: mode inner\ndata: elements " {
		t.Fatalf("no overlay clear right before the reload: %q", seen)
	}
}

// Two saves in quick succession make one rebuild and one reload.
func TestRun_CoalescesSaves(t *testing.T) {
	r := startRun(t)
	events := sse(t, r.base+"/_dev/reload")
	edit(t, r.dir, "app/Layout.alder", `"Dark mode"`, `"Dark mode!"`)
	time.Sleep(50 * time.Millisecond)
	edit(t, r.dir, "app/Layout.alder", `"Light mode"`, `"Light mode!"`)
	waitFor(t, events, "location.reload()", 60*time.Second)
	select {
	case e := <-events:
		t.Fatalf("a second event after the reload: %q", e)
	case <-time.After(2 * time.Second):
	}
	if n := strings.Count(r.errs.String(), "rebuilding"); n != 1 {
		t.Fatalf("%d rebuilds, want 1; stderr:\n%s", n, r.errs.String())
	}
}

// A build failure after the check (here: the adapters are gone) is overlaid and the old child
// keeps serving.
func TestRun_ChildCrashKeepsOld(t *testing.T) {
	r := startRun(t)
	events := sse(t, r.base+"/_dev/reload")
	if err := os.Remove(filepath.Join(r.dir, "adapters", "password.go")); err != nil {
		t.Fatal(err)
	}
	seen := waitFor(t, events, "selector #_alder_errors", 60*time.Second)
	if overlay := seen[len(seen)-1]; !strings.Contains(overlay, "needs an adapters package") {
		t.Fatalf("overlay lacks the missing-adapters message:\n%s", overlay)
	}
	if body := get(t, r.base+"/login"); !strings.Contains(body, `type="password"`) {
		t.Fatalf("the old child does not serve /login:\n%s", body)
	}
}

// Stopping the dev server stops the child.
func TestRun_StopKillsChild(t *testing.T) {
	r := startRun(t)
	get(t, "http://"+r.child+"/login")
	r.stop(t)
	if r.code != 0 {
		t.Fatalf("alder run exited %d; stderr:\n%s", r.code, r.errs.String())
	}
	deadline := time.Now().Add(6 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", r.child, time.Second)
		if err != nil {
			break
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatalf("the child at %s still accepts connections 6 s after stop", r.child)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := net.DialTimeout("tcp", strings.TrimPrefix(r.base, "http://"), time.Second); err == nil {
		t.Fatal("the dev server still accepts connections after stop")
	}
}
