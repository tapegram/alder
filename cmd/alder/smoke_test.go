package main

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBuild_BinarySmoke builds examples/todos (a copy, in a temporary directory) into its
// executable; checks that production without ALDER_SECRET refuses to start; starts it on a free
// port with an in-memory database in dev mode, waits for the `listening on <addr>` line, checks
// the login page is served and that SIGINT shuts it down cleanly; then starts it in production
// with ALDER_STATIC and checks that directory is served and SIGTERM stops it.
func TestBuild_BinarySmoke(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	src, err := filepath.Abs("../../examples/todos")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	copyDir(t, src, filepath.Join(tmp, "todos"))
	if code, stdout, stderr := runBuild(t, "--build-dir", filepath.Join(tmp, "build"), filepath.Join(tmp, "todos")); code != 0 {
		t.Fatalf("alder build: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// production without ALDER_SECRET refuses to start (C2): exit 1, the reason on stderr
	prod := exec.Command(filepath.Join(tmp, "build", "todos", "todos"))
	prod.Dir = tmp
	prod.Env = append(os.Environ(), "ALDER_ADDR=127.0.0.1:0", "ALDER_DB=:memory:", "ALDER_DEV=", "ALDER_SECRET=short")
	out, err := prod.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 || string(out) != "alder: ALDER_SECRET must be set to at least 16 bytes outside dev mode (ALDER_DEV=1)\n" {
		t.Fatalf("production start without a secret: %v\n%s", err, out)
	}

	app := startApp(t, tmp, "ALDER_DEV=1", "ALDER_SECRET=")
	a := app.addr
	c := &http.Client{Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path string) (*http.Response, string) {
		t.Helper()
		res, err := c.Get("http://" + a + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res, string(b)
	}
	res, body := get("/login")
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("GET /login = %d %q:\n%s", res.StatusCode, res.Header.Get("Content-Type"), body)
	}
	for _, f := range []string{`<form method="post" action="/_m/Auth.Login/Submit"`, `type="password"`, `src="/_alder/datastar-v1.0.0.js"`} {
		if !strings.Contains(body, f) {
			t.Fatalf("GET /login lacks %q:\n%s", f, body)
		}
	}
	// developer mode: the panel is in the page, with History and the curl button, and /_dev is the
	// generator's map of the app
	for _, f := range []string{`<al-devpanel id="_alder_panel"`, `_alder_tab_history`, `data-copy="curl -X GET`} {
		if !strings.Contains(body, f) {
			t.Fatalf("GET /login in dev mode lacks %q", f)
		}
	}
	if res, dev := get("/_dev"); res.StatusCode != http.StatusOK || !strings.Contains(dev, "Auth.Login") || !strings.Contains(dev, "Todo") || !strings.Contains(dev, "Migrations applied") {
		t.Fatalf("GET /_dev in dev mode = %d:\n%s", res.StatusCode, dev)
	}
	// a visitor's page view leaves the session as it was: no cookie (M11 final I1)
	if sc := res.Header.Get("Set-Cookie"); sc != "" {
		t.Fatalf("GET /login Set-Cookie %q, want none", sc)
	}
	// a Session.put (App.Logout's) stores it
	out2, err := c.Post("http://"+a+"/_m/App/Logout", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	out2.Body.Close()
	if !strings.HasPrefix(out2.Header.Get("Set-Cookie"), "alder_session=") {
		t.Fatalf("POST /_m/App/Logout Set-Cookie %q", out2.Header.Get("Set-Cookie"))
	}
	// the copied stylesheet is served from beside the executable
	if res, body := get("/static/app.css"); res.StatusCode != http.StatusOK || body == "" {
		t.Fatalf("GET /static/app.css = %d (%d bytes)", res.StatusCode, len(body))
	}
	if res, _ := get("/"); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("GET / = %d %q, want 303 /login", res.StatusCode, res.Header.Get("Location"))
	}

	// SIGINT: a graceful shutdown, exit 0
	if code := app.stop(t, os.Interrupt); code != 0 {
		t.Fatalf("after SIGINT: exit %d; stderr:\n%s", code, app.stderr.String())
	}

	// ALDER_STATIC replaces the static directory beside the executable; production with a secret
	static := t.TempDir()
	os.WriteFile(filepath.Join(static, "only-here.txt"), []byte("here"), 0o644)
	app = startApp(t, tmp, "ALDER_STATIC="+static, "ALDER_DEV=", "ALDER_SECRET=0123456789abcdef")
	a = app.addr
	if res, body := get("/static/only-here.txt"); res.StatusCode != http.StatusOK || body != "here" {
		t.Fatalf("GET /static/only-here.txt = %d %q", res.StatusCode, body)
	}
	if res, _ := get("/static/app.css"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /static/app.css with ALDER_STATIC = %d, want 404", res.StatusCode)
	}
	// a production build has no developer mode: /_dev is a 404 and no page carries the panel,
	// its script or its History
	if res, _ := get("/_dev"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /_dev in production = %d, want 404", res.StatusCode)
	}
	for _, path := range []string{"/login", "/nowhere"} {
		_, page := get(path)
		for _, f := range []string{"al-devpanel", "_alder_panel", "_alder_hist", "_alder_tab_"} {
			if strings.Contains(page, f) {
				t.Fatalf("GET %s in production has %q:\n%s", path, f, page)
			}
		}
	}
	if code := app.stop(t, syscall.SIGTERM); code != 0 {
		t.Fatalf("after SIGTERM: exit %d; stderr:\n%s", code, app.stderr.String())
	}
}

// running is an app process started by startApp.
type running struct {
	cmd     *exec.Cmd
	addr    string
	stderr  *strings.Builder
	drained chan struct{} // closed when its stdout has been read to the end
}

// startApp starts build/todos/todos in dir with an in-memory database on a free port and env
// added to the environment, and waits (60 s at most) for its `listening on <addr>` line. The
// process is killed when the test ends, unless stop was called.
func startApp(t *testing.T, dir string, env ...string) *running {
	t.Helper()
	return startAppNamed(t, dir, "todos", env...)
}

// startAppNamed is startApp for the app name's executable, build/<name>/<name>.
func startAppNamed(t *testing.T, dir, name string, env ...string) *running {
	t.Helper()
	cmd := exec.Command(filepath.Join(dir, "build", name, name))
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "ALDER_ADDR=127.0.0.1:0", "ALDER_DB=:memory:"), env...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	r := &running{cmd: cmd, stderr: &strings.Builder{}, drained: make(chan struct{})}
	cmd.Stderr = r.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			<-r.drained // every read from the pipe is done before Wait
			_ = cmd.Wait()
		}
	})
	addr := make(chan string, 1)
	go func() {
		defer close(r.drained)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if a, ok := strings.CutPrefix(sc.Text(), "listening on "); ok {
				addr <- a
				break
			}
		}
		close(addr)
		_, _ = io.Copy(io.Discard, stdout)
	}()
	select {
	case a, ok := <-addr:
		if !ok {
			<-r.drained
			_ = cmd.Wait()
			t.Fatalf("the app exited without printing `listening on`; stderr:\n%s", r.stderr.String())
		}
		if !strings.HasPrefix(a, "127.0.0.1:") || strings.HasSuffix(a, ":0") {
			t.Fatalf("listening on %q, want 127.0.0.1:<port>", a)
		}
		r.addr = a
	case <-time.After(60 * time.Second):
		t.Fatalf("no `listening on` line within 60 s; stderr:\n%s", r.stderr.String())
	}
	return r
}

// stop sends sig and returns the exit code (failing the test after 10 s).
func (r *running) stop(t *testing.T, sig os.Signal) int {
	t.Helper()
	if err := r.cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.drained:
	case <-time.After(10 * time.Second):
		t.Fatalf("still running 10 s after %v", sig)
	}
	_ = r.cmd.Wait()
	return r.cmd.ProcessState.ExitCode()
}
