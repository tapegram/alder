package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"alder/rt"
	"alder/rt/datastar"

	"alder/internal/buildkit"
	"alder/internal/watch"
)

// runRunTo is `alder run [--build-dir DIR] [dir]`: a development server for the app in dir. It builds the app
// (buildApp), runs its executable as a child process on a free port, and serves on ALDER_ADDR
// (default localhost:8080: the app behind it runs in development mode) a reverse proxy to it. While it runs, a save under app/ (*.alder),
// adapters/ (any file: the whole tree is copied) or migrations/ (`alder migrate`) rebuilds the app and reloads the browser; a change under static/ only
// copies static/ again into the build (with static/al/, the design system's assets) and reloads it; a check or build error is shown in the browser (an overlay) and on stderr while the last
// good build keeps serving. See devServer for the lifecycle. It returns when stop is closed,
// after stopping the child: 0, or 2 for a usage error or when it cannot listen.
func runRunTo(args []string, stdout, stderr io.Writer, stop <-chan struct{}) int {
	return runRunWith(args, stdout, stderr, stop, runOpts{addr: os.Getenv("ALDER_ADDR")})
}

// runOpts are what runRunWith takes besides the command line: the tests' way in, without
// globals or the environment.
type runOpts struct {
	addr        string           // the address to listen on; "" is localhost:8080
	onDevServer func(*devServer) // when set, called with the devServer before it starts
}

// runRunWith is runRunTo with its address and dev-server hook given in opts.
func runRunWith(args []string, stdout, stderr io.Writer, stop <-chan struct{}, opts runOpts) int {
	dir, haveDir, bd := ".", false, ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--build-dir" && i+1 < len(args):
			i++
			bd = args[i]
		case strings.HasPrefix(a, "--build-dir="):
			bd = strings.TrimPrefix(a, "--build-dir=")
		case strings.HasPrefix(a, "-") || haveDir:
			usageTo(stderr)
			return 2
		default:
			dir, haveDir = a, true
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(stderr, "alder run: %v\n", err)
		return 2
	}
	if !buildkit.IsDir(filepath.Join(dir, "app")) {
		fmt.Fprintf(stderr, "alder run: no app directory in %s\n", dir)
		return 2
	}
	ln, err := net.Listen("tcp", runAddr(opts.addr))
	if err != nil {
		fmt.Fprintf(stderr, "alder run: %v\n", err)
		return 2
	}
	d := newDevServer(dir, filepath.Base(abs), displayAddr(ln.Addr()), stdout, stderr)
	d.buildDir = bd
	if opts.onDevServer != nil {
		opts.onDevServer(d)
	}
	srv := &http.Server{Handler: d, ReadHeaderTimeout: 10 * time.Second}
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = srv.Serve(ln)
	}()

	var wg sync.WaitGroup
	changed := make(chan watch.Change)
	wg.Add(2)
	go func() {
		defer wg.Done()
		watch.Watch(dir, watch.DefaultInterval, d.done, changed)
	}()
	go func() {
		defer wg.Done()
		d.work(changed)
	}()

	<-stop
	close(d.done) // the watcher, the worker, the crash monitors and every /_dev/reload stream end
	grace, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := srv.Shutdown(grace); err != nil {
		_ = srv.Close()
	}
	cancel()
	<-served
	wg.Wait() // a rebuild in progress finishes first, so the child it may start is stopped below
	d.wg.Wait()
	d.mu.Lock()
	c := d.child
	d.child = nil
	d.mu.Unlock()
	if c != nil {
		c.stop()
	}
	return 0
}

// runAddr is the address `alder run` listens on: addr (ALDER_ADDR), by default localhost:8080.
// The default is the loopback interface only, because the app behind the dev server runs in
// development mode (error pages show the error, the public development cookie key).
func runAddr(addr string) string {
	if addr != "" {
		return addr
	}
	return "localhost:8080"
}

// displayAddr is a listener's address for a person to open: an unspecified host (":8080")
// is shown as localhost (rt.DisplayAddr, the rule the app's own public base follows).
func displayAddr(a net.Addr) string { return rt.DisplayAddr(a.String()) }

// devServer is `alder run`'s HTTP handler and the state behind it.
//
// Lifecycle: runRunTo listens, then starts two goroutines, the watcher (watch) and the worker
// (work). The worker builds the app once at once and then once per change the watcher reports;
// it is the only caller of rebuild, so rebuilds never overlap. A rebuild runs `alder check`
// (errors → the strip and the dev panel's Errors tab, structured; the old child keeps serving),
// then buildApp (errors → the same, as text), then starts the new executable as a child on a free port (startChild; if it does not come up,
// its stderr → the overlay, the old child keeps serving), swaps the proxy over to it, stops the
// old child, clears the overlay and tells every browser to reload. The first child that comes
// up is announced on stdout (`dev server on <addr> → app on <child addr>`). A child that exits
// on its own is reported in the overlay and the proxy answers 502 until the next good build.
// When stop closes, done closes: the watcher, worker and /_dev/reload streams end, the HTTP
// server shuts down, and the current child is stopped (SIGTERM, then SIGKILL after 5 s).
//
// Generations: each child is started with its own build generation, ALDER_RELOAD=<nonce>-<n>
// (nonce: random per dev server, n: counts the children started), which the app writes into
// every page's subscriber (/_dev/reload?b=<gen>). A subscriber whose generation is not that of
// the last child that came up (gen) is sent the reload at once, so a page that missed the
// broadcast (its stream was closed, or the dev server restarted) still reloads when it
// reconnects.
//
// Requests: /_dev/reload is an event stream of Datastar patches (the overlay, its clearing, the
// reload); everything else is proxied to the current child unchanged (cookies, redirects and
// streamed SSE answers alike), or answered 502 with the overlay while there is none.
type devServer struct {
	dir, app string
	buildDir string    // --build-dir ("" is build)
	addr     string    // the dev server's address, as displayed
	stdout   io.Writer // the announcement
	stderr   io.Writer // every check, build and crash message, and the child's output (locked)
	proxy    *httputil.ReverseProxy
	done     chan struct{} // closed on stop
	wg       sync.WaitGroup
	nonce    string // this dev server's part of every generation
	started  int    // children started so far (the worker's)
	static   string // the build's static directory the child serves (the worker's); "" before the first build

	mu        sync.Mutex
	child     *child                   // the current child; nil before the first good build or after it exited
	clients   map[chan []byte]struct{} // /_dev/reload subscribers
	errs      errState                 // what the strip and the Errors tab show: the last check/build/crash failure; the zero value after a good build
	gen       string                   // the generation of the last child that came up (kept after it exits); "" before the first
	announced bool                     // the stdout line has been printed
}

// targetKey carries the child a request is proxied to, chosen once in ServeHTTP.
type targetKey struct{}

func newDevServer(dir, app, addr string, stdout, stderr io.Writer) *devServer {
	var nonce [4]byte
	_, _ = rand.Read(nonce[:]) // never fails (crypto/rand)
	d := &devServer{dir: dir, app: app, addr: addr, stdout: stdout, stderr: &lockedWriter{w: stderr},
		done: make(chan struct{}), clients: map[chan []byte]struct{}{}, nonce: hex.EncodeToString(nonce[:])}
	d.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(pr.In.Context().Value(targetKey{}).(*url.URL))
			pr.Out.Host = pr.In.Host // the app sees the address the browser uses
		},
		FlushInterval: -1, // stream: the app's Datastar answers are server-sent events
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			d.mu.Lock()
			gen := d.gen
			d.mu.Unlock()
			d.unavailable(w, errState{kind: "crash", text: fmt.Sprintf("alder run: the app is not answering: %v", err)}, gen)
		},
	}
	return d
}

func (d *devServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/_dev/reload" {
		d.serveReload(w, r)
		return
	}
	d.mu.Lock()
	c, st, gen := d.child, d.errs, d.gen
	d.mu.Unlock()
	if c == nil {
		if r.URL.Path == datastar.ScriptPath { // the 502 page needs it to subscribe
			datastar.Handler().ServeHTTP(w, r)
			return
		}
		if st.kind == "" {
			st = errState{kind: "building", text: "alder run: the app is building…"}
		}
		d.unavailable(w, st, gen)
		return
	}
	target := &url.URL{Scheme: "http", Host: c.addr}
	d.proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}

// unavailable answers 502 with a page that is the list of errors st, styled (the same list the
// dev panel's Errors tab shows), and subscribes to the reloads as of generation gen (the
// subscriber the router injects, rt/web reloadTags), so the browser reloads by itself once a new
// app is up.
func (d *devServer) unavailable(w http.ResponseWriter, st errState, gen string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusBadGateway)
	title := map[string]string{"check": "Build failed", "build": "Build failed", "crash": "The app exited"}[st.kind]
	if title == "" {
		title = "alder run"
	}
	fmt.Fprintf(w, `<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>alder run</title><style>%s</style><script type="module" src="%s"></script></head><body><main><h1>%s</h1><div id="_alder_errors" data-ignore-morph data-page>%s</div></main><div data-init="@get('/_dev/reload?b=%s', {openWhenHidden: true})"></div></body></html>
`, pageCSS, datastar.ScriptPath, html.EscapeString(title), errorsHTML(st), url.QueryEscape(gen))
}

// pageCSS styles the 502 page: tokens with fallbacks, as al.css is not served while no app runs.
const pageCSS = `:root{color-scheme:light dark;--background:#fff;--foreground:#0b1220;--muted:#f1f5f9;--border:#e2e8f0;--destructive:#b91c1c}` +
	`@media(prefers-color-scheme:dark){:root{--background:#0b1220;--foreground:#f8fafc;--muted:#1e293b;--border:#334155;--destructive:#f87171}}` +
	`body{margin:0;background:var(--background);color:var(--foreground);font:14px/1.5 system-ui,sans-serif}` +
	`main{max-inline-size:60rem;margin:0 auto;padding:2rem 1rem}h1{margin:0 0 1rem;font-size:1.1rem;font-weight:600}`

// The events on /_dev/reload.
var (
	clearEvent  = datastar.PatchEvent("#_alder_errors", "inner", "")
	reloadEvent = datastar.PatchEvent("body", "append", `<script data-effect="el.remove()">location.reload()</script>`)
)

func overlayEvent(st errState) []byte {
	return datastar.PatchEvent("#_alder_errors", "inner", errorsHTML(st))
}

// serveReload is /_dev/reload?b=<gen>: an event stream that starts with the comment ": ready",
// then the reload when gen is not the current generation (the page is of an older build, or of
// an earlier dev server), else the overlay when there is an error now; then it sends every event
// broadcast while it is open, and a ": ping" comment every 15 s. It ends when the client goes
// away or the dev server stops.
func (d *devServer) serveReload(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)
	ch := make(chan []byte, 16)
	d.mu.Lock()
	d.clients[ch] = struct{}{}
	st, stale := d.errs, r.URL.Query().Get("b") != d.gen
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.clients, ch)
		d.mu.Unlock()
	}()
	send := func(b []byte) bool {
		if _, err := w.Write(b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send([]byte(": ready\n\n")) {
		return
	}
	switch {
	case stale:
		if !send(reloadEvent) {
			return
		}
	case st.kind != "":
		if !send(overlayEvent(st)) {
			return
		}
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-d.done:
			return
		case b := <-ch:
			if !send(b) {
				return
			}
		case <-ping.C:
			if !send([]byte(": ping\n\n")) {
				return
			}
		}
	}
}

// broadcast sends events to every subscriber; the caller holds mu. A subscriber too slow to
// take them misses them.
func (d *devServer) broadcast(events ...[]byte) {
	b := bytes.Join(events, nil)
	for ch := range d.clients {
		select {
		case ch <- b:
		default:
		}
	}
}

// fail reports the text msg of a failed build or start: on stderr, and as the strip and the
// Errors tab of every open page and of pages opened later.
func (d *devServer) fail(msg string) {
	d.report(msg, errState{kind: "build", text: msg})
}

// report prints text on stderr and shows st: the strip and the Errors tab of every open page
// and of pages opened later.
func (d *devServer) report(text string, st errState) {
	text = strings.TrimRight(text, "\n") + "\n"
	fmt.Fprint(d.stderr, text)
	st.text = text
	d.mu.Lock()
	defer d.mu.Unlock()
	d.errs = st
	d.broadcast(overlayEvent(st))
}

// work is the worker: it builds the app, then rebuilds it (or, for a change to static files
// only, reloads the browsers) for each change, until done closes. Changes reported while a
// rebuild runs are merged into one.
func (d *devServer) work(changed <-chan watch.Change) {
	fmt.Fprintf(d.stderr, "alder run: building %s\n", d.app)
	d.rebuild()
	d.mu.Lock()
	up := d.child != nil
	d.mu.Unlock()
	if !up {
		fmt.Fprintf(d.stderr, "alder run: dev server on %s; the app starts once it builds\n", d.addr)
	}
	for {
		var k watch.Change
		select {
		case <-d.done:
			return
		case k = <-changed:
		}
	merge:
		for {
			select {
			case k2 := <-changed:
				k |= k2
			default:
				break merge
			}
		}
		if k&watch.SourceChange != 0 {
			fmt.Fprintf(d.stderr, "alder run: rebuilding %s\n", d.app)
			d.rebuild()
		} else {
			if d.static != "" { // the child serves the build's copy: refresh it first
				if err := buildkit.CopyStatic(filepath.Join(d.dir, "static"), d.static); err != nil {
					d.fail("alder run: " + err.Error())
					continue
				}
			}
			d.mu.Lock()
			d.broadcast(reloadEvent)
			d.mu.Unlock()
		}
	}
}

// rebuild checks and builds the app and, when both succeed, replaces the child with one running
// the new executable and reloads the browsers; otherwise it reports the failure (fail) and the
// old child keeps serving. Only the worker calls it.
func (d *devServer) rebuild() {
	var out bytes.Buffer
	if code := runCheckTo([]string{d.dir}, &out, &out); code != 0 {
		// the checker's structured output (what `alder check --json` prints), run again for the
		// failure only; a failure without diagnostics (a load error) stays text
		var js bytes.Buffer
		runCheckTo([]string{"--json", d.dir}, &js, io.Discard)
		if ds := parseDiags(d.dir, js.String()); len(ds) > 0 {
			d.report(out.String(), errState{kind: "check", diags: ds})
		} else {
			d.fail(out.String())
		}
		return
	}
	d.stderr.Write(out.Bytes()) // warnings
	out.Reset()
	binary, code := buildApp(d.dir, filepath.Join(buildRoot(d.buildDir), d.app), &out)
	if code != 0 {
		d.fail(out.String())
		return
	}
	dir := filepath.Dir(binary)
	static := filepath.Join(dir, "static") // the build's copy: the app's static/ and static/al/
	d.static = static
	d.started++
	gen := fmt.Sprintf("%s-%d", d.nonce, d.started)
	c, err := startChild(binary, filepath.Join(dir, "dev.db"), static, gen, d.addr, d.stderr)
	if err != nil {
		d.fail("alder run: " + err.Error())
		return
	}

	d.mu.Lock()
	old := d.child
	d.child = c
	d.gen = gen
	announce := !d.announced
	d.announced = true
	d.mu.Unlock()
	d.wg.Add(1)
	go d.monitor(c)
	if announce {
		fmt.Fprintf(d.stdout, "dev server on %s → app on %s\n", d.addr, c.addr)
	} else {
		fmt.Fprintf(d.stderr, "alder run: rebuilt; app on %s\n", c.addr)
	}
	if old != nil {
		old.stop()
	}
	d.mu.Lock()
	d.errs = errState{}
	d.broadcast(clearEvent, reloadEvent) // the clear first
	d.mu.Unlock()
}

// monitor reports c in the overlay when it exits without being stopped, and stops proxying to
// it.
func (d *devServer) monitor(c *child) {
	defer d.wg.Done()
	select {
	case <-d.done:
		return
	case <-c.exited:
	}
	if c.stopping.Load() {
		return
	}
	d.mu.Lock()
	if d.child == c {
		d.child = nil
	}
	d.mu.Unlock()
	msg := fmt.Sprintf("alder run: the app exited (%v); save a source file to rebuild it\n%s", c.cmd.ProcessState, lastLines(c.stderr.String(), overlayLines))
	d.report(msg, errState{kind: "crash"})
}

// overlayLines is how many of a child's last stderr lines a crash report shows.
const overlayLines = 40

// lastLines is the last n lines of s (a final newline ignored).
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// child is a running app executable. Its standard input is the read end of a pipe whose write
// end, stdinW, only this process holds (ALDER_EXIT_ON_STDIN_EOF=1 is set): stop closes it, and
// when the dev server dies without stopping it (SIGKILL), the kernel closes it, the app reads
// EOF and shuts down by itself, so it never outlives the dev server.
type child struct {
	cmd      *exec.Cmd
	stdinW   *os.File      // the write end of the child's stdin pipe; closed by stop
	addr     string        // where it listens
	stderr   tailBuffer    // the end of its stderr, for error reports
	exited   chan struct{} // closed once it has exited (and been waited for)
	stopping atomic.Bool   // stop was called: its exit is expected
}

// childStartTimeout is how long startChild waits for the `listening on` line.
const childStartTimeout = 10 * time.Second

// childEnv is the environment of an app startChild runs: environ (alder run's) with the
// variables startChild names, and ALDER_PUBLIC_URL=http://<devAddr> (the dev server's displayed
// address: the child listens on a free port behind it), unless environ sets ALDER_PUBLIC_URL (a
// tunnel's address), which is passed through untouched.
func childEnv(environ []string, dbPath, staticDir, gen, devAddr string) []string {
	env := append(slices.Clip(environ), "ALDER_ADDR=127.0.0.1:0", "ALDER_DEV=1", "ALDER_RELOAD="+gen,
		"ALDER_DB="+dbPath, "ALDER_STATIC="+staticDir, "ALDER_EXIT_ON_STDIN_EOF=1")
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "ALDER_PUBLIC_URL="); ok && strings.TrimSpace(v) != "" {
			return env
		}
	}
	return append(env, "ALDER_PUBLIC_URL=http://"+devAddr)
}

// startChild runs the app executable binary in development mode with reloads on for the build
// generation gen (ALDER_DEV=1, ALDER_RELOAD=<gen>) on a free port of 127.0.0.1, the database
// dbPath and the static files of staticDir (ALDER_SECRET and the rest of the environment are
// passed through; ALDER_PUBLIC_URL=http://<devAddr> unless the environment sets it: childEnv), its
// output copied to logs; its standard input is a pipe the child keeps
// open (ALDER_EXIT_ON_STDIN_EOF=1: the app exits when the dev server dies). It waits (10 s at most) for its
// `listening on <addr>` line; when the child exits first or does not print it in time (it is
// then killed), the error holds the last overlayLines lines of its stderr.
func startChild(binary, dbPath, staticDir, gen, devAddr string, logs io.Writer) (*child, error) {
	cmd := exec.Command(binary)
	cmd.Env = childEnv(os.Environ(), dbPath, staticDir, gen, devAddr)
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	c := &child{cmd: cmd, stdinW: stdinW, exited: make(chan struct{})}
	cmd.Stdin = stdinR
	cmd.Stderr = io.MultiWriter(&c.stderr, logs)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdinR.Close()
		stdinW.Close()
		return nil, err
	}
	err = cmd.Start()
	stdinR.Close() // the child holds its copy
	if err != nil {
		stdinW.Close()
		return nil, err
	}
	addr := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if a, ok := strings.CutPrefix(sc.Text(), "listening on "); ok {
				addr <- a
				break
			}
			fmt.Fprintln(logs, sc.Text())
		}
		_, _ = io.Copy(logs, stdout) // every read is done before Wait
		_ = cmd.Wait()
		stdinW.Close() // the child is gone; a second Close (stop's) is harmless
		close(c.exited)
	}()
	select {
	case c.addr = <-addr:
		return c, nil
	case <-c.exited:
		return nil, fmt.Errorf("the app did not start (%v):\n%s", cmd.ProcessState, lastLines(c.stderr.String(), overlayLines))
	case <-time.After(childStartTimeout):
		_ = cmd.Process.Kill()
		<-c.exited
		return nil, fmt.Errorf("the app did not print `listening on` within %v:\n%s", childStartTimeout, lastLines(c.stderr.String(), overlayLines))
	}
}

// stop shuts c down: SIGTERM (a graceful shutdown), then SIGKILL after 5 s; it returns once c
// has exited. It closes the child's stdin pipe after the SIGTERM (the EOF only repeats it).
func (c *child) stop() {
	c.stopping.Store(true)
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	_ = c.stdinW.Close()
	select {
	case <-c.exited:
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
		<-c.exited
	}
}

// lockedWriter serialises the writes of several goroutines to w.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// tailBuffer keeps the last tailMax bytes written to it; it is safe for concurrent use.
type tailBuffer struct {
	mu sync.Mutex
	b  []byte
}

const tailMax = 16 << 10

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > tailMax {
		t.b = append([]byte(nil), t.b[len(t.b)-tailMax:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}
