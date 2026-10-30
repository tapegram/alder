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
	if code, out, errs := runBuild(t, dir); code != 0 {
		t.Fatalf("build: %d\n%s%s", code, out, errs)
	}
	bin := filepath.Join(testBuildDir(t), "olog", "olog")
	base := []string{"ALDER_DB=:memory:", "ALDER_DEV=", "ALDER_SECRET=" + strings.Repeat("s", 32), "ALDER_ADDR=127.0.0.1:0", "ALDER_PUBLIC_URL=", "ALDER_LOG=", "ALDER_LOG_FORMAT="}

	t.Run("BadALDER_LOG", func(t *testing.T) {
		t.Parallel()
		for _, c := range []struct{ env, want string }{
			{"ALDER_LOG=verbose", "alder: ALDER_LOG=verbose: want debug, info, warn or error\n"},
			{"ALDER_LOG_FORMAT=yaml", "alder: ALDER_LOG_FORMAT=yaml: want text or json\n"},
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
