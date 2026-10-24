package format

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestFmtGolden(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/golden/fmt_*.alder")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			got := format(t, filepath.Base(f), src)
			goldenPath := strings.TrimSuffix(f, ".alder") + ".golden"
			if *update {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				want, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatalf("missing golden %s (run with -update)", goldenPath)
				}
				if string(got) != string(want) {
					t.Fatalf("golden mismatch for %s\n--- got:\n%s\n--- want:\n%s", f, got, want)
				}
			}
			if again := format(t, filepath.Base(f), got); string(again) != string(got) {
				t.Fatalf("not idempotent for %s\n--- first:\n%s\n--- second:\n%s", f, got, again)
			}
		})
	}
}

func format(t *testing.T, file string, src []byte) []byte {
	t.Helper()
	out, diags, err := Source(file, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) > 0 {
		t.Fatalf("parse errors: %v", diags)
	}
	return out
}

func TestSource_ParseError(t *testing.T) {
	out, diags, err := Source("m.alder", []byte("module M\n\nf ="))
	if out != nil || err != nil || len(diags) == 0 {
		t.Fatalf("got out=%q diags=%v err=%v, want nil output and diagnostics", out, diags, err)
	}
}

func TestSource_UnsafeRefused(t *testing.T) {
	testMutate = func(b []byte) []byte { // drop the last character of the value: 12 becomes 1
		i := strings.Index(string(b), "12")
		return append(append([]byte{}, b[:i+1]...), b[i+2:]...)
	}
	defer func() { testMutate = nil }()
	out, diags, err := Source("m.alder", []byte("module M\n\nx : Int\nx = 12\n"))
	if !errors.Is(err, ErrUnsafe) || out != nil || diags != nil {
		t.Fatalf("got out=%q diags=%v err=%v, want ErrUnsafe", out, diags, err)
	}
	if !strings.Contains(err.Error(), "m.alder") {
		t.Errorf("error %q does not name the file", err)
	}
}

func TestSource_OutputThatDoesNotParseIsUnsafe(t *testing.T) {
	testMutate = func(b []byte) []byte { return append(b, "x ="...) }
	defer func() { testMutate = nil }()
	if _, _, err := Source("m.alder", []byte("module M\n")); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}

// Comments are not printed yet (attachment comes later): a file whose comments the output would
// drop is refused rather than rewritten without them.
func TestSource_DroppedCommentIsUnsafe(t *testing.T) {
	_, _, err := Source("m.alder", []byte("module M\n\n-- the answer\nx : Int\nx = 42\n"))
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}

func TestSource_SpanTextIsByteBased(t *testing.T) {
	src := "module M\n\ns : Text\ns = \"héllo wörld\"\n"
	out, _, err := Source("m.alder", []byte(src))
	if err != nil || string(out) != src {
		t.Fatalf("got %q, %v", out, err)
	}
}
