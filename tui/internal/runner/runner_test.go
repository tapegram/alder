package runner

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
}

func TestExec_RunCode(t *testing.T) {
	skipWindows(t)
	res, err := Exec("/bin/sh").Run(context.Background(), t.TempDir(), "-c", "echo hi; echo err >&2; exit 3")
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Stdout) != "hi\n" || string(res.Stderr) != "err\n" || res.Code != 3 {
		t.Fatalf("got %q %q %d", res.Stdout, res.Stderr, res.Code)
	}
}

func TestExec_RunMissingBinary(t *testing.T) {
	if _, err := Exec("/nonexistent/alder").Run(context.Background(), t.TempDir(), "x"); err == nil {
		t.Fatal("want an error when the binary cannot start")
	}
}

func TestExec_StreamLines(t *testing.T) {
	skipWindows(t)
	s, err := Exec("/bin/sh").Stream(context.Background(), t.TempDir(), "-c", `printf 'a\nb\nc\n'; echo oops >&2; exit 0`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for l := range s.Lines() {
		got = append(got, l)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	code, stderr, err := s.Wait()
	if code != 0 || stderr != "oops\n" || err != nil {
		t.Fatalf("got %d %q %v", code, stderr, err)
	}
}

func TestExec_StreamHugeLine(t *testing.T) {
	skipWindows(t)
	s, err := Exec("/bin/sh").Stream(context.Background(), t.TempDir(), "-c", `head -c 2000000 /dev/zero | tr '\0' x; echo; echo tail; exit 2`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for l := range s.Lines() {
		got = append(got, l)
	}
	if len(got) != 2 || len(got[0]) != 2000000 || got[1] != "tail" {
		t.Fatalf("got %d lines", len(got))
	}
	if code, _, _ := s.Wait(); code != 2 {
		t.Fatalf("code %d", code)
	}
}

func TestFake(t *testing.T) {
	f := &Fake{}
	f.On("test --json", "{\"a\":1}\n{\"b\":2}", 1)
	s, err := f.Stream(context.Background(), "d", "test", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for l := range s.Lines() {
		got = append(got, l)
	}
	if want := []string{`{"a":1}`, `{"b":2}`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	if code, _, _ := s.Wait(); code != 1 {
		t.Fatalf("code %d", code)
	}
	res, err := f.Run(context.Background(), "d", "test", "--json")
	if err != nil || res.Code != 1 || !strings.HasPrefix(string(res.Stdout), `{"a"`) {
		t.Fatalf("got %+v %v", res, err)
	}
	if _, err := f.Run(context.Background(), "d", "other"); err == nil {
		t.Fatal("want an error for an unscripted call")
	}
	if want := []string{"test --json", "test --json", "other"}; !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("calls %q", f.Calls())
	}
}

// settled waits up to a second for the number of goroutines to fall back to n, and reports
// whether it did.
func settled(n int) bool {
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if runtime.NumGoroutine() <= n {
			return true
		}
	}
	return false
}

func TestStream_MissingBinary(t *testing.T) {
	before := runtime.NumGoroutine()
	s, err := Exec("/nonexistent/alder").Stream(context.Background(), t.TempDir(), "test", "--json")
	if err == nil || s != nil {
		t.Fatalf("got %v %v, want no stream and an error when the binary cannot start", s, err)
	}
	if !settled(before) {
		t.Errorf("%d goroutines after a failed start, %d before", runtime.NumGoroutine(), before)
	}
}

func TestRun_Cancel(t *testing.T) {
	skipWindows(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Exec("/bin/sh").Run(ctx, t.TempDir(), "-c", "sleep 30 & echo $!; wait")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run after its deadline = %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Run took %v after its deadline", d)
	}
}

func TestWait_Twice(t *testing.T) {
	skipWindows(t)
	s, err := Exec("/bin/sh").Stream(context.Background(), t.TempDir(), "-c", "echo out; echo err >&2; exit 4")
	if err != nil {
		t.Fatal(err)
	}
	c1, e1, err1 := s.Wait()
	c2, e2, err2 := s.Wait()
	if c1 != 4 || e1 != "err\n" || err1 != nil {
		t.Fatalf("first Wait = %d %q %v", c1, e1, err1)
	}
	if c1 != c2 || e1 != e2 || err1 != err2 {
		t.Errorf("second Wait = %d %q %v, first %d %q %v", c2, e2, err2, c1, e1, err1)
	}
}

func TestFake_CRLF(t *testing.T) {
	f := &Fake{}
	f.On("test --json", "a\r\nb\r\n", 0)
	s, err := f.Stream(context.Background(), "d", "test", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for l := range s.Lines() {
		got = append(got, l)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFake_Stderr(t *testing.T) {
	f := &Fake{}
	f.OnStderr("test --json", "alder: no app here\n", 1)
	s, err := f.Stream(context.Background(), "d", "test", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for range s.Lines() {
	}
	if code, stderr, err := s.Wait(); code != 1 || stderr != "alder: no app here\n" || err != nil {
		t.Errorf("Wait = %d %q %v", code, stderr, err)
	}
	if res, err := f.Run(context.Background(), "d", "test", "--json"); err != nil || string(res.Stderr) != "alder: no app here\n" {
		t.Errorf("Run = %+v %v", res, err)
	}
}

func TestFake_Cancel(t *testing.T) {
	f := &Fake{}
	f.On("test --json", "a", 0)
	ctx, cancel := context.WithCancel(context.Background())
	s, err := f.Stream(ctx, "d", "test", "--json")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, _, err := s.Wait(); !errors.Is(err, context.Canceled) {
		t.Errorf("Wait after cancel = %v, want context.Canceled", err)
	}
}
