package runner

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"testing"
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
	code, stderr := s.Wait()
	if code != 0 || string(stderr) != "oops\n" {
		t.Fatalf("got %d %q", code, stderr)
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
	if code, _ := s.Wait(); code != 2 {
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
	if code, _ := s.Wait(); code != 1 {
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
