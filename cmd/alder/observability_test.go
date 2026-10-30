package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The generated main's log (#56): ALDER_LOG and ALDER_LOG_FORMAT, validated before anything is
// served; one access line per request routed, none for the health check. The app is
// testdata/corpus/url-absolute-ok (a page at /).
func TestGen_MainLog(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	if testing.Short() {
		t.Skip("builds and runs an app")
	}
	dir := tempAppAbs(t, "../../testdata/corpus/url-absolute-ok", "olog")
	// a page slower than ALDER_SLOW_MS=1 for the slow line (#92)
	slowPage := "module Slow\n\npage \"/slow\" { }\n\ntype Model = { n: Int }\n\nload : Params -> Session -> Query Model\n" +
		"load params session = {\n  let _ <- Query.delay 20\n  Query.succeed { n: 1 }\n}\n\n" +
		"view : Model -> Session -> Html Msg\nview model session =\n  <html><body><p>slow</p></body></html>\n"
	if err := os.WriteFile(filepath.Join(dir, "app", "Slow.alder"), []byte(slowPage), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := runBuild(t, dir); code != 0 {
		t.Fatalf("build: %d\n%s%s", code, out, errs)
	}
	bin := filepath.Join(testBuildDir(t), "olog", "olog")
	base := []string{"ALDER_DB=:memory:", "ALDER_DEV=", "ALDER_SECRET=" + strings.Repeat("s", 32), "ALDER_ADDR=127.0.0.1:0", "ALDER_PUBLIC_URL=", "ALDER_LOG=", "ALDER_LOG_FORMAT=", "ALDER_METRICS=", "ALDER_SLOW_MS="}

	t.Run("BadALDER_LOG", func(t *testing.T) {
		t.Parallel()
		for _, c := range []struct{ env, want string }{
			{"ALDER_LOG=verbose", "alder: ALDER_LOG=verbose: want debug, info, warn or error\n"},
			{"ALDER_LOG_FORMAT=yaml", "alder: ALDER_LOG_FORMAT=yaml: want text or json\n"},
			// #92: the token is never echoed
			{"ALDER_METRICS=short-secret", "alder: ALDER_METRICS: want \"open\" or a token of at least 16 characters\n"},
			{"ALDER_SLOW_MS=250ms", "alder: ALDER_SLOW_MS=250ms: want a whole number of milliseconds (0: off)\n"},
		} {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			var out []byte
			var err error
			retryETXTBSY(func() error {
				cmd := exec.CommandContext(ctx, bin)
				cmd.Env = append(append(os.Environ(), base...), c.env)
				out, err = cmd.CombinedOutput()
				if errors.Is(err, syscall.ETXTBSY) {
					return err
				}
				return nil
			})
			cancel()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 1 || string(out) != c.want {
				t.Errorf("%s: exit %v, output %q; want 1, %q", c.env, err, out, c.want)
			}
		}
	})

	t.Run("AccessLine", func(t *testing.T) {
		t.Parallel()
		app := startLogApp(t, bin, base...)
		app.get(t, "/?key=s3cret&seed=7")
		app.get(t, "/_alder/health")
		errs := app.stop(t)
		var access []string
		for _, l := range strings.Split(errs, "\n") {
			if strings.Contains(l, " GET ") {
				access = append(access, l)
			}
		}
		if len(access) != 1 || !strings.Contains(access[0], " info: GET /?key=••••&seed=7 200 ") || !strings.Contains(access[0], " rid=") {
			t.Errorf("access lines %q; stderr:\n%s", access, errs)
		}
		if strings.Contains(errs, "s3cret") || strings.Contains(errs, "/_alder/health") || strings.Contains(errs, "effect ") {
			t.Errorf("stderr:\n%s", errs)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		app := startLogApp(t, bin, append(base, "ALDER_LOG_FORMAT=json")...)
		app.get(t, "/")
		errs := app.stop(t)
		var found bool
		for _, l := range strings.Split(strings.TrimSpace(errs), "\n") {
			var m map[string]any
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				t.Errorf("not JSON: %q", l)
				continue
			}
			if m["method"] == "GET" && m["path"] == "/" {
				found = true
				if m["status"] != float64(200) || m["level"] != "INFO" || m["rid"] == "" {
					t.Errorf("line %v", m)
				}
			}
		}
		if !found {
			t.Errorf("no access line:\n%s", errs)
		}
	})

	t.Run("WarnSilencesAccess", func(t *testing.T) {
		t.Parallel()
		app := startLogApp(t, bin, append(base, "ALDER_LOG=warn")...)
		app.get(t, "/")
		app.get(t, "/nowhere")
		if errs := app.stop(t); strings.Contains(errs, "GET /") {
			t.Errorf("access lines under warn:\n%s", errs)
		}
	})

	t.Run("MetricsOffByDefault", func(t *testing.T) {
		t.Parallel()
		app := startLogApp(t, bin, base...)
		if code, _ := app.fetch(t, "/_alder/metrics", ""); code != 404 {
			t.Errorf("status %d", code)
		}
		app.stop(t)
	})

	t.Run("MetricsOpen", func(t *testing.T) {
		t.Parallel()
		app := startLogApp(t, bin, append(base, "ALDER_METRICS=open", "ALDER_BUILD=b-92")...)
		app.get(t, "/")
		code, body := app.fetch(t, "/_alder/metrics", "")
		if code != 200 || !strings.Contains(body, `alder_build_info{build="b-92"} 1`) ||
			!strings.Contains(body, `alder_http_requests_total{route="/",method="GET",status="2xx"} 1`) {
			t.Errorf("status %d\n%s", code, body)
		}
		if errs := app.stop(t); strings.Contains(errs, "/_alder/metrics") {
			t.Errorf("a scrape logged:\n%s", errs)
		}
	})

	t.Run("MetricsToken", func(t *testing.T) {
		t.Parallel()
		const tok = "tok-0123456789abcdef"
		app := startLogApp(t, bin, append(base, "ALDER_METRICS="+tok)...)
		if code, _ := app.fetch(t, "/_alder/metrics", ""); code != 401 {
			t.Errorf("without: %d", code)
		}
		if code, _ := app.fetch(t, "/_alder/metrics", "Bearer wrong-0123456789abcdef"); code != 401 {
			t.Errorf("wrong: %d", code)
		}
		if code, body := app.fetch(t, "/_alder/metrics", "Bearer "+tok); code != 200 || !strings.Contains(body, "alder_http_requests_in_flight") {
			t.Errorf("with: %d\n%s", code, body)
		}
		if errs := app.stop(t); strings.Contains(errs, tok) || !strings.Contains(errs, "GET /_alder/metrics 401 ") {
			t.Errorf("stderr:\n%s", errs)
		}
	})

	t.Run("SlowLine", func(t *testing.T) {
		t.Parallel()
		app := startLogApp(t, bin, append(base, "ALDER_SLOW_MS=1", "ALDER_LOG=warn")...)
		app.get(t, "/slow")
		errs := app.stop(t)
		var slow []string
		for _, l := range strings.Split(errs, "\n") {
			if strings.Contains(l, " warn: GET /slow ") {
				slow = append(slow, l)
			}
		}
		if len(slow) != 1 || !strings.Contains(slow[0], " slow route=/slow steps=") || !strings.Contains(slow[0], " db=") {
			t.Errorf("slow lines %q; stderr:\n%s", slow, errs)
		}
	})

	t.Run("DebugShowsEffects", func(t *testing.T) {
		t.Parallel()
		app := startLogApp(t, bin, append(base, "ALDER_LOG=debug")...)
		app.get(t, "/")
		if errs := app.stop(t); !strings.Contains(errs, " info: GET / 200 ") || !strings.Contains(errs, " steps=") {
			t.Errorf("debug:\n%s", errs)
		}
	})
}

// logApp is a served app's process: its address and its standard error.
type logApp struct {
	cmd    *exec.Cmd
	addr   string
	mu     sync.Mutex
	stderr bytes.Buffer
}

func (a *logApp) Write(b []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stderr.Write(b)
}

// startLogApp starts bin with env added and waits for its `listening on` line (60 s at most).
func startLogApp(t *testing.T, bin string, env ...string) *logApp {
	t.Helper()
	a := &logApp{}
	var stdout io.ReadCloser
	if err := retryETXTBSY(func() error {
		var err error
		a.cmd = exec.Command(bin)
		a.cmd.Env = append(os.Environ(), env...)
		a.cmd.Stderr = a
		if stdout, err = a.cmd.StdoutPipe(); err != nil {
			return err
		}
		return a.cmd.Start()
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.cmd.Process.Kill(); _ = a.cmd.Wait() })
	addr := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if s, ok := strings.CutPrefix(sc.Text(), "listening on "); ok {
				addr <- s
				break
			}
		}
		close(addr)
		_, _ = io.Copy(io.Discard, stdout)
	}()
	select {
	case s, ok := <-addr:
		if !ok {
			t.Fatalf("exited without `listening on`:\n%s", a.text())
		}
		a.addr = s
	case <-time.After(60 * time.Second):
		t.Fatalf("no `listening on` within 60 s:\n%s", a.text())
	}
	return a
}

func (a *logApp) text() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stderr.String()
}

func (a *logApp) get(t *testing.T, path string) {
	t.Helper()
	res, err := http.Get("http://" + a.addr + path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
}

// fetch GETs path with an Authorization header when auth is set: the status and the body.
func (a *logApp) fetch(t *testing.T, path, auth string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("GET", "http://"+a.addr+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// stop shuts the app down (SIGTERM: the requests in flight finish, their lines written) and
// returns its standard error.
func (a *logApp) stop(t *testing.T) string {
	t.Helper()
	_ = a.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = a.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("still running 30 s after SIGTERM:\n%s", a.text())
	}
	return a.text()
}
