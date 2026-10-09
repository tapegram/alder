package main

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuild_BinarySmoke builds examples/todos (a copy, in a temporary directory) into its
// executable, starts it on a free port with an in-memory database in dev mode, waits for the
// `listening on <addr>` line, and checks the login page is served.
func TestBuild_BinarySmoke(t *testing.T) {
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
	if code, stdout, stderr := runBuild(t, "todos"); code != 0 {
		t.Fatalf("alder build: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(tmp, "build", "todos", "todos"))
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "ALDER_ADDR=127.0.0.1:0", "ALDER_DB=:memory:", "ALDER_DEV=1", "ALDER_SECRET=")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	addr := make(chan string, 1)
	go func() {
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
	var a string
	select {
	case got, ok := <-addr:
		if !ok {
			_ = cmd.Wait()
			t.Fatalf("the app exited without printing `listening on`; stderr:\n%s", stderr.String())
		}
		a = got
	case <-ctx.Done():
		t.Fatalf("no `listening on` line within 60 s; stderr:\n%s", stderr.String())
	}
	if !strings.HasPrefix(a, "127.0.0.1:") || strings.HasSuffix(a, ":0") {
		t.Fatalf("listening on %q, want 127.0.0.1:<port>", a)
	}

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
	for _, f := range []string{`<form method="post" action="/_m/Auth.Login/Submit"`, `type="password"`, `src="/_alder/datastar.js"`} {
		if !strings.Contains(body, f) {
			t.Fatalf("GET /login lacks %q:\n%s", f, body)
		}
	}
	if !strings.HasPrefix(res.Header.Get("Set-Cookie"), "alder_session=") {
		t.Fatalf("GET /login Set-Cookie %q", res.Header.Get("Set-Cookie"))
	}
	// the copied stylesheet is served from beside the executable
	if res, body := get("/static/app.css"); res.StatusCode != http.StatusOK || body == "" {
		t.Fatalf("GET /static/app.css = %d (%d bytes)", res.StatusCode, len(body))
	}
	if res, _ := get("/"); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("GET / = %d %q, want 303 /login", res.StatusCode, res.Header.Get("Location"))
	}
}
