package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The generated main's public base (#9): ALDER_PUBLIC_URL, validated before anything is served,
// or else the listener's address as alder run shows it. The app is testdata/corpus/url-absolute-ok,
// whose page shows Url.absolute of itself at its defaults (<p id="home">).
func TestPublicURL_Binary(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	if testing.Short() {
		t.Skip("builds and runs an app")
	}
	dir := tempAppAbs(t, "../../testdata/corpus/url-absolute-ok", "uabs")
	if code, out, errs := runBuild(t, dir); code != 0 {
		t.Fatalf("build: %d\n%s%s", code, out, errs)
	}
	bin := filepath.Join(testBuildDir(t), "uabs", "uabs")
	base := []string{"ALDER_DB=:memory:", "ALDER_DEV=", "ALDER_SECRET=" + strings.Repeat("s", 32)}

	t.Run("bad value exits before serving", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var out []byte
		var err error
		retryETXTBSY(func() error {
			cmd := exec.CommandContext(ctx, bin)
			cmd.Env = append(append(os.Environ(), base...), "ALDER_ADDR=127.0.0.1:0", "ALDER_PUBLIC_URL=localhost:8080")
			out, err = cmd.CombinedOutput()
			if errors.Is(err, syscall.ETXTBSY) {
				return err
			}
			return nil
		})
		if ctx.Err() != nil {
			t.Fatalf("still running after 60 s:\n%s", out)
		}
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 1 {
			t.Fatalf("exit %v, want 1:\n%s", err, out)
		}
		want := `alder: ALDER_PUBLIC_URL must be http(s)://host[:port], such as https://blog.example.com (got "localhost:8080")` + "\n"
		if string(out) != want {
			t.Errorf("output %q, want %q", out, want)
		}
	})

	// home is Url.absolute in the view; constant a top-level value, computed while the program
	// initialises: it sees ALDER_PUBLIC_URL and a fixed ALDER_ADDR, but a port-0 address only as
	// port 0 (the port is picked at Listen, after it)
	for _, c := range []struct {
		name, addr, public string
		home, constant     func(port string) string
	}{
		{"unset, loopback", "127.0.0.1:0", "", func(p string) string { return "http://127.0.0.1:" + p + "/" }, func(string) string { return "http://127.0.0.1:0/" }},
		{"unset, unspecified host", ":0", "", func(p string) string { return "http://localhost:" + p + "/" }, func(string) string { return "http://localhost:0/" }},
		{"unset, fixed port", "127.0.0.1:" + strconv.Itoa(freePort(t)), "", func(p string) string { return "http://127.0.0.1:" + p + "/" }, func(p string) string { return "http://127.0.0.1:" + p + "/" }},
		{"set", "127.0.0.1:0", "https://ex.com/", func(string) string { return "https://ex.com/" }, func(string) string { return "https://ex.com/" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			listen := serveBinary(t, bin, append(base, "ALDER_ADDR="+c.addr, "ALDER_PUBLIC_URL="+c.public)...)
			_, port, err := net.SplitHostPort(listen)
			if err != nil {
				t.Fatal(err)
			}
			res, err := http.Get("http://127.0.0.1:" + port + "/")
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			for _, want := range []string{`<p id="home">` + c.home(port) + `</p>`, `<p id="const">` + c.constant(port) + `</p>`} {
				if !strings.Contains(string(body), want) {
					_, b, _ := strings.Cut(string(body), "<body>")
					t.Errorf("listening on %s: the page lacks %q: <body>%s", listen, want, b)
				}
			}
		})
	}
}

// alder run's child listens on 127.0.0.1:0 behind the dev server, so it is told the dev
// server's address as its public base; a value in the shell (a tunnel) passes through untouched.
func TestChildEnv_PublicURL(t *testing.T) {
	t.Parallel()
	last := func(env []string) (string, int) {
		v, n := "", 0
		for _, kv := range env {
			if s, ok := strings.CutPrefix(kv, "ALDER_PUBLIC_URL="); ok {
				v, n = s, n+1
			}
		}
		return v, n
	}
	for _, c := range []struct {
		environ []string
		devAddr string
		want    string
	}{
		{[]string{"HOME=/h"}, "localhost:8080", "http://localhost:8080"},
		{[]string{"HOME=/h"}, "127.0.0.1:18765", "http://127.0.0.1:18765"},
		{[]string{"ALDER_PUBLIC_URL=  "}, "localhost:8080", "http://localhost:8080"},
		{[]string{"ALDER_PUBLIC_URL=https://x.ngrok.app"}, "localhost:8080", "https://x.ngrok.app"},
	} {
		env := childEnv(c.environ, "dev.db", "static", "g-1", c.devAddr)
		if v, _ := last(env); v != c.want {
			t.Errorf("%v on %s: ALDER_PUBLIC_URL %q, want %q (%v)", c.environ, c.devAddr, v, c.want, env)
		}
		for _, want := range []string{"ALDER_ADDR=127.0.0.1:0", "ALDER_DEV=1", "ALDER_RELOAD=g-1", "ALDER_DB=dev.db", "ALDER_STATIC=static", "ALDER_EXIT_ON_STDIN_EOF=1"} {
			if !slices.Contains(env, want) {
				t.Errorf("the child's environment lacks %s: %v", want, env)
			}
		}
	}
	if _, n := last(childEnv([]string{"ALDER_PUBLIC_URL=https://x.ngrok.app"}, "a", "b", "c", "localhost:8080")); n != 1 {
		t.Errorf("the shell's ALDER_PUBLIC_URL is not passed through untouched: %d values", n)
	}
}

// Inside alder test Url.absolute starts with http://localhost:8080 whatever the environment
// holds: url-absolute-ok's tests expect it, and pass with ALDER_PUBLIC_URL set to another base.
func TestPublicURL_TestRunnerIgnoresEnv(t *testing.T) {
	needGoTool(t)
	t.Setenv("ALDER_PUBLIC_URL", "https://elsewhere.example")
	dir := tempAppAbs(t, "../../testdata/corpus/url-absolute-ok", "uabs")
	code, stdout, stderr := runTest(t, dir)
	if code != 0 || !strings.Contains(stdout, "3 passed, 0 failed\n") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

// serveBinary starts bin with env added to the environment and returns the address of its
// `listening on` line (60 s at most). The process is killed when t ends.
func serveBinary(t *testing.T, bin string, env ...string) string {
	t.Helper()
	var cmd *exec.Cmd
	var stdout io.ReadCloser
	var stderr strings.Builder
	if err := retryETXTBSY(func() error {
		var err error
		cmd = exec.Command(bin)
		cmd.Env = append(os.Environ(), env...)
		cmd.Stderr = &stderr
		if stdout, err = cmd.StdoutPipe(); err != nil {
			return err
		}
		return cmd.Start()
	}); err != nil {
		t.Fatal(err)
	}
	drained := make(chan struct{})
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-drained
		_ = cmd.Wait()
	})
	addr := make(chan string, 1)
	go func() {
		defer close(drained)
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
			t.Fatalf("exited without `listening on`; stderr:\n%s", stderr.String())
		}
		return a
	case <-time.After(60 * time.Second):
		t.Fatalf("no `listening on` within 60 s; stderr:\n%s", stderr.String())
	}
	return ""
}
