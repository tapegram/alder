package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	d      *devServer    // runRunTo's dev server
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
// `dev server on <addr> → app on <child>` line and for the watcher's first snapshot (an edit
// before it would be the baseline, not a change). The dev server and its child are stopped when
// the test ends.
func startRun(t *testing.T) *devRun {
	t.Helper()
	return startRunApp(t, "todos")
}

// startRunApp is startRun of examples/<app>.
func startRunApp(t *testing.T, app string) *devRun {
	t.Helper()
	return startRunAppWith(t, app, nil)
}

// startRunAppWith is startRunApp with files (path in the app → content) added to the copy
// before it is served.
func startRunAppWith(t *testing.T, app string, files map[string]string) *devRun {
	t.Helper()
	return startRunAppOpts(t, app, files, runOpts{})
}

// startRunAppOpts is startRunAppWith with opts given to runRunWith (its addr, onDevServer and
// watchReady are set here).
func startRunAppOpts(t *testing.T, app string, files map[string]string, opts runOpts) *devRun {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	src, err := filepath.Abs("../../examples/" + app)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	app = filepath.Base(app)
	copyDir(t, src, filepath.Join(tmp, app))
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(tmp, app, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := &devRun{dir: filepath.Join(tmp, app), out: &syncBuffer{}, errs: &syncBuffer{},
		done: make(chan struct{}), stopCh: make(chan struct{})}
	devs, ready := make(chan *devServer, 1), make(chan struct{})
	opts.addr, opts.onDevServer, opts.watchReady = "127.0.0.1:0", func(d *devServer) { devs <- d }, func() { close(ready) }
	go func() {
		defer close(r.done)
		r.code = runRunWith([]string{"--build-dir", filepath.Join(tmp, "build"), filepath.Join(tmp, app)}, r.out, r.errs, r.stopCh, opts)
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
				r.base, r.child, r.d = "http://"+dev, child, <-devs
				await(t, ready, "the watcher's first snapshot")
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
	return getStatus(t, url, http.StatusOK)
}

// getStatus GETs url (without following redirects) and returns the body, failing unless the
// status is want.
func getStatus(t *testing.T, url string, want int) string {
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
	if res.StatusCode != want {
		t.Fatalf("GET %s = %d, want %d:\n%s", url, res.StatusCode, want, b)
	}
	return string(b)
}

// subscriberRE is the reload subscriber the router injects; it captures the build generation.
var subscriberRE = regexp.MustCompile(`<div data-init="@get\('/_dev/reload\?b=([0-9a-f]{8}-[0-9]+)', \{openWhenHidden: true\}\)"></div>`)

// generation is the build generation of the page body, failing when it has no subscriber.
func generation(t *testing.T, body string) string {
	t.Helper()
	m := subscriberRE.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no reload subscriber in:\n%s", body)
	}
	return m[1]
}

// subscribe subscribes to /_dev/reload as a page of the build now serving /login does.
func (r *devRun) subscribe(t *testing.T) <-chan string {
	t.Helper()
	return sse(t, r.base+"/_dev/reload?b="+generation(t, get(t, r.base+"/login")))
}

// quiet fails when an event arrives within d.
func quiet(t *testing.T, events <-chan string, d time.Duration) {
	t.Helper()
	select {
	case e := <-events:
		t.Fatalf("an unexpected event: %q", e)
	case <-time.After(d):
	}
}

// sse subscribes to url and yields each event block (its lines, without the blank line that
// ends it), but the ": ping" keepalives, which come by wall time and are not the dev server's
// answer to anything: quiet and the other "no further event" checks must not see them. The
// subscription is closed when the test ends.
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
			if len(block) == 1 && block[0] == ": ping" {
				block = nil // the keepalive: a stream open past 15 s (a loaded machine's rebuild) has one (#133)
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

// The child logs JSON at debug; the dev server shows its lines on stderr as the text an info
// child writes, and feeds each record, debug fields included, as a log event.
func TestRun_LogsRenderedAndFed(t *testing.T) {
	r := startRun(t)
	lines := eventStream(t, r.base+"/_dev/events")
	get(t, r.base+"/login")
	deadline := time.Now().Add(10 * time.Second)
	for {
		e := nextEvent(t, lines)
		if rec, ok := e["record"].(map[string]any); e["type"] == "log" && ok && rec["path"] == "/login" {
			if _, ok := rec["steps"]; !ok {
				t.Errorf("the /login record has no steps (the child is not at debug): %v", rec)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no log event with the /login record in 10 s")
		}
	}
	access := regexp.MustCompile(`(?m)^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} info: GET /login 200 `)
	for !access.MatchString(r.errs.String()) {
		if time.Now().After(deadline) {
			t.Fatalf("stderr lacks the /login access line:\n%s", r.errs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Contains(r.errs.String(), `{"time"`) {
		t.Errorf("stderr shows JSON:\n%s", r.errs.String())
	}
}

// The dev server proxies the app, whose pages carry the overlay target (kept by Datastar's
// morph) and the reload subscriber (naming the build generation, open while the tab is hidden);
// /_dev/reload is an event stream.
func TestRun_ServesAndInjectsSubscriber(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	body := get(t, r.base+"/login")
	for _, want := range []string{`<body><div id="_alder_errors" data-ignore-morph></div><div data-init=`, "/_alder/datastar-v1.0.0.js", `type="password"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /login lacks %q:\n%s", want, body)
		}
	}
	if gen := generation(t, body); gen != r.d.nonce+"-1" {
		t.Fatalf("the first child's generation is %q, want %q", gen, r.d.nonce+"-1")
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
	// (a page view stores no cookie; a Session.put, here App.Logout's, does)
	if res, err = doDelete(c, r.base+"/session"); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Set-Cookie"), "alder_session=") {
		t.Fatalf("DELETE /session Set-Cookie %q", res.Header.Get("Set-Cookie"))
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
		!strings.HasPrefix(string(b), "event: datastar-patch-elements\ndata: selector body\ndata: mode outer\n") || !strings.Contains(string(b), `<div id="_alder_errors" data-ignore-morph></div>`) {
		t.Fatalf("Datastar GET /nope = %d %q:\n%s", res.StatusCode, res.Header.Get("Content-Type"), b)
	}
	quiet(t, r.subscribe(t), time.Second) // the current generation: no reload
}

// dsDo sends req as the Datastar client does (Datastar-Request: true) without following
// redirects, and returns the answer's status, headers and body.
func dsDo(t *testing.T, req *http.Request) (*http.Response, string) {
	t.Helper()
	req.Header.Set("Datastar-Request", "true")
	c := &http.Client{Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(b)
}

// Boosted navigation through the dev server (M13): a page's <head> begins with the color-scheme
// meta, the build's meta and the Datastar script, and ends with the boost; a
// Datastar GET of a page (a boosted link, with Datastar's ?datastar= signals) is answered with
// its body patch (overlay target and subscriber kept), its title and a history push; one whose
// `requires` redirects pushes the login page; a message that redirects is answered with the
// target's morph under Datastar, and with the 303 without it.
func TestRun_BoostedNavigation(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	if page := get(t, r.base+"/login"); !strings.Contains(page, `<head><meta name="color-scheme" content="light dark" /><meta name="alder-build" content="`) ||
		!regexp.MustCompile(`<meta name="alder-build" content="[0-9a-f]{16}" /><style>[^<]*</style><script type="module" src="/_alder/datastar-v1.0.0.js"></script>`).MatchString(page) ||
		!regexp.MustCompile(`<meta name="alder-boost" content="links" data-init="[^"]*"[^>]*/></head>`).MatchString(page) ||
		!strings.Contains(page, `data-on:click__window="if (evt.defaultPrevented`) {
		t.Fatalf("GET /login lacks the boost:\n%s", page)
	}
	push := func(u string) string {
		return `<script data-effect="el.remove()">(function(){var u="` + u + `";`
	}
	for _, c := range []struct{ path, push string }{{"/login?datastar=%7B%7D", "/login"}, {"/", "/login"}} {
		req, _ := http.NewRequest("GET", r.base+c.path, nil)
		res, body := dsDo(t, req)
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" ||
			!strings.HasPrefix(body, "event: datastar-patch-elements\ndata: selector body\ndata: mode outer\ndata: elements <body><div id=\"_alder_errors\" data-ignore-morph></div>") ||
			!strings.Contains(body, `type="password"`) ||
			!strings.Contains(body, "event: datastar-patch-elements\ndata: selector head > title\ndata: mode outer\ndata: elements <title>") ||
			!strings.Contains(body, push(c.push)) || strings.Contains(body, "location.assign") {
			t.Fatalf("boosted GET %s = %d %q:\n%s", c.path, res.StatusCode, res.Header.Get("Content-Type"), body)
		}
	}
	req, _ := http.NewRequest("DELETE", r.base+"/session", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, body := dsDo(t, req)
	if res.StatusCode != 200 || res.Header.Get("Location") != "" || !strings.HasPrefix(res.Header.Get("Set-Cookie"), "alder_session=") ||
		!strings.Contains(body, `type="password"`) || !strings.Contains(body, push("/login")) {
		t.Fatalf("Datastar DELETE /session = %d %q:\n%s", res.StatusCode, res.Header.Get("Location"), body)
	}
	c := &http.Client{Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if res, err := doDelete(c, r.base+"/session"); err != nil {
		t.Fatal(err)
	} else if res.Body.Close(); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("DELETE /session = %d %q, want 303 /login", res.StatusCode, res.Header.Get("Location"))
	}
}

// Saving a source file rebuilds the app, pushes a reload, and the new build serves, under a
// new generation. A page of the old build that subscribes only now (its stream was closed while
// the reload was sent) is reloaded at once, as is one of an earlier dev server.
func TestRun_EditReloads(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	old := generation(t, get(t, r.base+"/login"))
	events := sse(t, r.base+"/_dev/reload?b="+old)
	edit(t, r.dir, "app/Layout.alder", `<main>`, `<main class="edited">`)
	waitFor(t, events, wantReload, 60*time.Second)
	body := get(t, r.base+"/login")
	if !strings.Contains(body, `<main class="edited">`) {
		t.Fatalf("the new build is not serving:\n%s", body)
	}
	if gen := generation(t, body); gen == old {
		t.Fatalf("the new build has the old generation %q", gen)
	}
	for _, stale := range []string{old, "0badf00d-1", ""} {
		seen := waitFor(t, sse(t, r.base+"/_dev/reload?b="+stale), "location.reload()", 10*time.Second)
		if len(seen) != 1 || seen[0] != wantReload {
			t.Fatalf("a subscriber of generation %q got %q, want the reload at once", stale, seen)
		}
	}
	quiet(t, r.subscribe(t), time.Second)
}

// The child serves the build's static directory: the app's static/ and the design system's
// assets (/static/al/al.css with an ETag, revalidated on every load under `alder run` so an edit
// to App's theme shows (#38); the other files fresh for five minutes then
// stale-while-revalidate). Saving a static file copies it again and reloads, without a rebuild
// (the generation is unchanged).
func TestRun_ServesStaticAndDesignSystem(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	gen := generation(t, get(t, r.base+"/login"))
	res, err := http.Get(r.base + "/static/al/al.css")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-cache" || res.Header.Get("ETag") == "" || !strings.Contains(string(b), "alder-ui v0.6.0") {
		t.Fatalf("GET /static/al/al.css = %d %q %.60q", res.StatusCode, res.Header.Get("Cache-Control"), b)
	}
	fres, err := http.Get(r.base + "/static/al/fonts/GeistMono-Variable.woff2")
	if err != nil {
		t.Fatal(err)
	}
	fres.Body.Close()
	if fres.StatusCode != 200 || fres.Header.Get("Content-Type") != "font/woff2" || fres.Header.Get("Cache-Control") != "public, max-age=300, stale-while-revalidate=86400" {
		t.Fatalf("GET /static/al/fonts/GeistMono-Variable.woff2 = %d %q %q", fres.StatusCode, fres.Header.Get("Content-Type"), fres.Header.Get("Cache-Control"))
	}
	get(t, r.base+"/static/al/custom-elements.json")
	events := r.subscribe(t)
	edit(t, r.dir, "static/app.css", "ul#todos {", "ul#todos.edited {")
	waitFor(t, events, wantReload, 30*time.Second)
	if css := get(t, r.base+"/static/app.css"); !strings.Contains(css, "ul#todos.edited {") {
		t.Fatalf("the edited static file is not served:\n%.200s", css)
	}
	if g := generation(t, get(t, r.base+"/login")); g != gen {
		t.Fatalf("a static change rebuilt the app: generation %q, was %q", g, gen)
	}
	get(t, r.base+"/static/al/al.css")
}

// A type error is overlaid (escaped) while the old build keeps serving; fixing it clears the
// overlay, then reloads.
func TestRun_ErrorThenFixReloads(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	events := r.subscribe(t)
	edit(t, r.dir, "app/Table/Todo.alder", "|> List.length\n", "|> List.length |> Text.length\n")
	seen := waitFor(t, events, "selector #_alder_errors", 60*time.Second)
	overlay := seen[len(seen)-1]
	// the strip, then each diagnostic structured: file:line:col, the code, the message, the excerpt and its caret
	for _, want := range []string{"data: mode inner", `<div class="_alder-strip" role="alert">`, "Build failed: 1 error. The page is the last good build.",
		`data-count="1"`, `<code class="_alder-at">app/Table/Todo.alder:36:3</code>`, `<b class="_alder-code">E0003</b>`, "expected Text, got Int",
		"<pre>  todos\n", "   ^</pre>"} {
		if !strings.Contains(overlay, want) {
			t.Fatalf("overlay lacks %q:\n%s", want, overlay)
		}
	}
	if !strings.Contains(r.errs.String(), "E0003") {
		t.Fatalf("stderr lacks the diagnostic:\n%s", r.errs.String())
	}
	get(t, r.base+"/login") // the old child still serves
	// a page opened now gets the overlay at once
	if later := waitFor(t, r.subscribe(t), "E0003", 10*time.Second); len(later) != 1 {
		t.Fatalf("a new subscriber's events: %q", later)
	}

	edit(t, r.dir, "app/Table/Todo.alder", "|> List.length |> Text.length\n", "|> List.length\n")
	seen = waitFor(t, events, wantReload, 60*time.Second)
	if len(seen) < 2 || seen[len(seen)-2] != "event: datastar-patch-elements\ndata: selector #_alder_errors\ndata: mode inner\ndata: elements " {
		t.Fatalf("no overlay clear right before the reload: %q", seen)
	}
}

// Two saves with no poll between them make one rebuild and one reload. The test drives the
// watcher's polls (runOpts.watchTicks), so a stall between the saves cannot put a poll there
// (#137): each poll is a send, which returns once the poll before it is done.
func TestRun_CoalescesSaves(t *testing.T) {
	t.Parallel()
	ticks, poll := pollDriver(t)
	r := startRunAppOpts(t, "todos", nil, runOpts{watchTicks: ticks})
	events := r.subscribe(t)
	edit(t, r.dir, "app/Layout.alder", `<main>`, `<main class="edited">`)
	edit(t, r.dir, "app/Layout.alder", `</title>`, `!</title>`)
	poll() // sees the saves
	poll() // finds nothing new and reports them: one change
	waitFor(t, events, "location.reload()", 60*time.Second)
	assertOneRebuild(t, r, events, poll)
}

// Saves on consecutive polls are one change too: the watcher reports only on a poll that finds
// nothing new, not on the first one that finds a change.
func TestRun_CoalescesSavesAcrossPolls(t *testing.T) {
	t.Parallel()
	ticks, poll := pollDriver(t)
	r := startRunAppOpts(t, "todos", nil, runOpts{watchTicks: ticks})
	events := r.subscribe(t)
	// noRebuildYet gives the poll just sent time to run (a stall only lets a broken watcher
	// pass, never fails a working one) and fails if it reported the change.
	noRebuildYet := func(after string) {
		t.Helper()
		time.Sleep(time.Second)
		if strings.Contains(r.errs.String(), "rebuilding") {
			t.Fatalf("a rebuild after %s, before a quiet poll; stderr:\n%s", after, r.errs.String())
		}
	}
	edit(t, r.dir, "app/Layout.alder", `<main>`, `<main class="edited">`)
	poll() // sees the first save
	noRebuildYet("the first save's poll")
	edit(t, r.dir, "app/Layout.alder", `</title>`, `!</title>`)
	poll() // sees the second
	noRebuildYet("the second save's poll")
	poll() // finds nothing new and reports both: one change
	waitFor(t, events, "location.reload()", 60*time.Second)
	assertOneRebuild(t, r, events, poll)
}

// pollDriver is a tick channel for runOpts.watchTicks and poll, which sends one tick (60 s at
// most): it returns once the watcher has taken the tick, and so finished every earlier poll.
func pollDriver(t *testing.T) (chan time.Time, func()) {
	ticks := make(chan time.Time)
	return ticks, func() {
		t.Helper()
		select {
		case ticks <- time.Now():
		case <-time.After(60 * time.Second):
			t.Fatal("the watcher took no poll within 60 s")
		}
	}
}

// assertOneRebuild polls once more (nothing is left pending), then fails on any further event
// within 2 s or on other than one rebuild.
func assertOneRebuild(t *testing.T, r *devRun, events <-chan string, poll func()) {
	t.Helper()
	poll()
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
func TestRun_BuildFailureKeepsOld(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	events := r.subscribe(t)
	if err := os.Remove(filepath.Join(r.dir, "adapters", "auth", "password.go")); err != nil {
		t.Fatal(err)
	}
	seen := waitFor(t, events, "selector #_alder_errors", 60*time.Second)
	// a build failure has no diagnostics: its text is one block, counted as one error
	for _, want := range []string{"is missing adapters for 1 capability:", "Build failed: 1 error. The page is the last good build.", `data-count="1"`} {
		if overlay := seen[len(seen)-1]; !strings.Contains(overlay, want) {
			t.Fatalf("overlay lacks %q:\n%s", want, overlay)
		}
	}
	if body := get(t, r.base+"/login"); !strings.Contains(body, `type="password"`) {
		t.Fatalf("the old child does not serve /login:\n%s", body)
	}
}

// A new child that does not start (here: its database path is a directory) is overlaid with
// its stderr, and the old child keeps serving.
func TestRun_StartupCrashKeepsOld(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	events := r.subscribe(t)
	gen := generation(t, get(t, r.base+"/login"))
	db := filepath.Join(filepath.Dir(r.dir), "build", "todos", "dev.db") // startRunAppWith: --build-dir <tmp>/build
	for _, f := range []string{db, db + "-wal", db + "-shm", db + "-journal"} {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(db, 0o755); err != nil {
		t.Fatal(err)
	}
	edit(t, r.dir, "app/Layout.alder", `<main>`, `<main class="edited">`)
	seen := waitFor(t, events, "selector #_alder_errors", 120*time.Second)
	overlay := seen[len(seen)-1]
	for _, want := range []string{"alder run: the app did not start (exit status 1):", "Build failed: 1 error.", "alder: "} {
		if !strings.Contains(overlay, want) {
			t.Fatalf("overlay lacks %q:\n%s", want, overlay)
		}
	}
	body := get(t, r.base+"/login") // the old child still serves, as the same generation
	if strings.Contains(body, `<main class="edited">`) || generation(t, body) != gen {
		t.Fatalf("the old child is not the one serving /login:\n%s", body)
	}
	quiet(t, events, time.Second) // no reload
}

// A child that dies while it serves (SIGKILL) is overlaid, the dev server answers 502 with the
// overlay and a subscriber until the next good build, and saving a source file recovers.
func TestRun_RuntimeCrashThenRecovers(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	events := r.subscribe(t)
	gen := generation(t, get(t, r.base+"/login"))
	r.d.mu.Lock()
	c := r.d.child
	r.d.mu.Unlock()
	if err := c.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	seen := waitFor(t, events, "selector #_alder_errors", 30*time.Second)
	if overlay := seen[len(seen)-1]; !strings.Contains(overlay, "alder run: the app exited (signal: killed); save a source file to rebuild it") {
		t.Fatalf("overlay lacks the crash message:\n%s", overlay)
	}
	body := getStatus(t, r.base+"/login", http.StatusBadGateway)
	// no app at all: the same list is the whole page, styled (data-page), without the strip's text
	for _, want := range []string{`<div id="_alder_errors" data-ignore-morph data-page>`, `<div class="_alder-errs" data-count="1"`, "alder run: the app exited (signal: killed)", "<h1", "/_alder/datastar-v1.0.0.js"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the 502 page lacks %q:\n%s", want, body)
		}
	}
	if g := generation(t, body); g != gen { // the 502 page waits for a generation after the crashed one
		t.Fatalf("the 502 page's generation is %q, want %q", g, gen)
	}
	edit(t, r.dir, "app/Layout.alder", `<main>`, `<main class="edited">`)
	waitFor(t, events, wantReload, 120*time.Second)
	if body := get(t, r.base+"/login"); !strings.Contains(body, `<main class="edited">`) {
		t.Fatalf("the new build is not serving:\n%s", body)
	}
}

func TestRunAddr(t *testing.T) {
	t.Parallel()
	if got := runAddr(""); got != "localhost:8080" {
		t.Errorf("default ALDER_ADDR = %q, want localhost:8080", got)
	}
	if got := runAddr("127.0.0.1:0"); got != "127.0.0.1:0" {
		t.Errorf("ALDER_ADDR = %q", got)
	}
}

func TestLastLines(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		n    int
		want string
	}{
		{"", 2, ""},
		{"a\n", 2, "a"},
		{"a\nb\nc\n", 2, "b\nc"},
		{"a\nb\nc", 3, "a\nb\nc"},
		{"a\nb\nc", 1, "c"},
	} {
		if got := lastLines(tc.in, tc.n); got != tc.want {
			t.Errorf("lastLines(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
	var b strings.Builder
	for i := range 100 {
		b.WriteString(strings.Repeat("x", i%7) + "\n")
	}
	if got := strings.Count(lastLines(b.String(), overlayLines), "\n"); got != overlayLines-1 {
		t.Errorf("%d lines kept, want %d", got+1, overlayLines)
	}
}

// Stopping the dev server stops the child.
func TestRun_StopKillsChild(t *testing.T) {
	t.Parallel()
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

// The blog's layout begins its <head> with <meta charset>: the router's insertions keep it in
// the first 1024 bytes, where a browser looks for it (the boost goes before </head>).
func TestRun_CharsetEarly(t *testing.T) {
	t.Parallel()
	r := startRunApp(t, "blog")
	page := get(t, r.base+"/")
	if i := strings.Index(page, `<meta charset="utf-8" />`); i < 0 || i+len(`<meta charset="utf-8" />`) > 1024 {
		t.Fatalf("<meta charset> at byte %d:\n%.1200s", i, page)
	}
}

// lifecycleEvents reads lines (an eventStream) until it has n build or app events, skipping the
// others, and returns them; it fails when they do not come within timeout.
func lifecycleEvents(t *testing.T, lines <-chan string, n int, timeout time.Duration) []map[string]any {
	t.Helper()
	var got []map[string]any
	deadline := time.After(timeout)
	for len(got) < n {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("the stream ended after %v", got)
			}
			data, ok := strings.CutPrefix(l, "data: ")
			if !ok {
				continue // a ping
			}
			var e map[string]any
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				t.Fatalf("data %q: %v", data, err)
			}
			if e["type"] == "build" || e["type"] == "app" {
				got = append(got, e)
			}
		case <-deadline:
			t.Fatalf("%d of %d build/app events within %v: %v", len(got), n, timeout, got)
		}
	}
	return got
}

// isEvent reports whether e is of type typ with the field state equal to state.
func isEvent(e map[string]any, typ, state string) bool {
	return e["type"] == typ && e["state"] == state
}

// A failed check and its fix show on the feed as build events (the failure with its diagnostics),
// then the new child as an app event; hello's build state is that of the last build.
func TestRun_FeedLifecycle(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	lines := eventStream(t, r.base+"/_dev/events")
	hello := nextEvent(t, lines)
	if hello["type"] != "hello" || hello["build"] != "ok" || hello["child"] != r.child {
		t.Fatalf("hello = %v, want build ok and child %s", hello, r.child)
	}
	first := lifecycleEvents(t, lines, 3, 10*time.Second) // the first build, replayed
	if !isEvent(first[0], "build", "building") || !isEvent(first[1], "build", "ok") || !isEvent(first[2], "app", "started") {
		t.Fatalf("the first build's events: %v", first)
	}
	if first[1]["child"] != r.child || first[2]["child"] != r.child || first[0]["gen"] != first[1]["gen"] {
		t.Fatalf("the first build's events: %v", first)
	}
	if _, ok := first[1]["ms"].(float64); !ok {
		t.Fatalf("build ok has no ms: %v", first[1])
	}

	edit(t, r.dir, "app/App.alder", "notFound : Html", "notFound : Int")
	got := lifecycleEvents(t, lines, 2, 60*time.Second)
	if !isEvent(got[0], "build", "building") || !isEvent(got[1], "build", "failed") {
		t.Fatalf("after the error: %v", got)
	}
	ds, _ := got[1]["diagnostics"].([]any)
	if len(ds) == 0 {
		t.Fatalf("build failed has no diagnostics: %v", got[1])
	}
	if d, _ := ds[0].(map[string]any); d["file"] != "app/App.alder" || d["line"] == nil || d["line"].(float64) < 1 {
		t.Fatalf("the diagnostic lacks its file and line: %v", ds[0])
	}
	if b := nextHello(t, r); b != "failed" {
		t.Fatalf("hello.build after the error = %q, want failed", b)
	}

	edit(t, r.dir, "app/App.alder", "notFound : Int", "notFound : Html")
	got = lifecycleEvents(t, lines, 3, 120*time.Second)
	if !isEvent(got[0], "build", "building") || !isEvent(got[1], "build", "ok") || !isEvent(got[2], "app", "started") {
		t.Fatalf("after the fix: %v", got)
	}
	if c, _ := got[1]["child"].(string); c == "" || c == r.child || got[2]["child"] != c {
		t.Fatalf("after the fix, no new child address: %v", got)
	}
}

// liveEvents is an eventStream of r's feed from now on: the backlog and the hello are skipped.
func liveEvents(t *testing.T, r *devRun) <-chan string {
	t.Helper()
	seq := nextEvent(t, eventStream(t, r.base+"/_dev/events"))["seq"].(float64)
	lines := eventStream(t, fmt.Sprintf("%s/_dev/events?after=%d", r.base, int64(seq)))
	nextEvent(t, lines) // hello
	return lines
}

// nextHello is the build state the hello of a new /_dev/events stream reports.
func nextHello(t *testing.T, r *devRun) any {
	t.Helper()
	return nextEvent(t, eventStream(t, r.base+"/_dev/events"))["build"]
}

// The dev server's own lines (`alder run: …`) are run events too.
func TestRun_FeedRunLines(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	lines := eventStream(t, r.base+"/_dev/events")
	nextEvent(t, lines) // hello
	for {
		e := nextEvent(t, lines) // the backlog first: it holds the first build's lines
		if text, _ := e["text"].(string); e["type"] == "run" && strings.HasPrefix(text, "alder run: building ") {
			return
		}
	}
}

// POST /_dev/restart starts a fresh child from the last good build; only a same-origin POST (or
// one without Origin, as curl sends) is taken.
func TestRun_Restart(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	lines := liveEvents(t, r)
	post := func(origin string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, r.base+"/_dev/restart", nil)
		if err != nil {
			t.Fatal(err)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := post("http://evil.example"); code != http.StatusForbidden {
		t.Fatalf("POST with a foreign Origin = %d, want 403", code)
	}
	getStatus(t, r.base+"/_dev/restart", http.StatusMethodNotAllowed)
	if code := post(r.base); code != http.StatusAccepted {
		t.Fatalf("POST /_dev/restart = %d, want 202", code)
	}
	for {
		e := lifecycleEvents(t, lines, 1, 60*time.Second)[0]
		if isEvent(e, "app", "started") {
			if c, _ := e["child"].(string); c == "" || c == r.child {
				t.Fatalf("app started without a new child: %v", e)
			}
			break
		}
		if e["type"] == "build" {
			t.Fatalf("a restart with a good build rebuilt: %v", e)
		}
	}
	get(t, r.base+"/login")
}

// A restart after a failed check rebuilds from source: it does not run the last good build and
// report it ok while the source is broken, so the errors and the failed state stay.
func TestRun_RestartAfterFailedCheck(t *testing.T) {
	t.Parallel()
	r := startRun(t)
	lines := liveEvents(t, r)
	edit(t, r.dir, "app/App.alder", "notFound : Html", "notFound : Int")
	if got := lifecycleEvents(t, lines, 2, 60*time.Second); !isEvent(got[1], "build", "failed") {
		t.Fatalf("after the error: %v", got)
	}
	res, err := http.Post(r.base+"/_dev/restart", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /_dev/restart = %d, want 202", res.StatusCode)
	}
	got := lifecycleEvents(t, lines, 2, 60*time.Second)
	if !isEvent(got[0], "build", "building") || !isEvent(got[1], "build", "failed") {
		t.Fatalf("a restart of broken source: %v, want build building then failed", got)
	}
	if ds, _ := got[1]["diagnostics"].([]any); len(ds) == 0 {
		t.Fatalf("the restart's build failed has no diagnostics: %v", got[1])
	}
	if b := nextHello(t, r); b != "failed" {
		t.Fatalf("hello.build after the restart = %q, want failed", b)
	}
	r.d.mu.Lock()
	kind := r.d.errs.kind
	r.d.mu.Unlock()
	if kind != "check" {
		t.Fatalf("the overlay after the restart is %q, want the check's errors", kind)
	}
}

// doDelete sends DELETE to u with c, as a browser's logout does.
func doDelete(c *http.Client, u string) (*http.Response, error) {
	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}
