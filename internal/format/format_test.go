package format

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/syntax"
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

func TestSource_DroppedImportIsUnsafe(t *testing.T) {
	testMutate = func(b []byte) []byte { return []byte(strings.Replace(string(b), "import B\n", "", 1)) }
	defer func() { testMutate = nil }()
	_, _, err := Source("m.alder", []byte("module M\n\nimport A\nimport B\n\nx : Int\nx = 1\n"))
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}

func TestSource_OutputThatDoesNotParseIsUnsafe(t *testing.T) {
	testMutate = func(b []byte) []byte { return append(b, "x ="...) }
	defer func() { testMutate = nil }()
	if _, _, err := Source("m.alder", []byte("module M\n")); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}

// The theme declaration: the spec's example is already formatted; a record of one or two fields
// stays on one line, three go vertical; a comment inside a scheme is kept.
func TestSource_Theme(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"module App\n\ntheme {\n  radius: \"0.5rem\",\n  fontSans: [\"Inter\", \"system-ui\", \"sans-serif\"],\n  light: {\n    primary: \"oklch(0.55 0.22 285)\",\n    primaryForeground: \"#ffffff\",\n    ring: \"var(--primary)\",\n  },\n  dark: { primary: \"oklch(0.72 0.16 285)\", primaryForeground: \"oklch(0.145 0 0)\" },\n}\n", ""},
		{"module App\n\ntheme {\n  radius: \"0.5rem\"\n}\n", "module App\n\ntheme { radius: \"0.5rem\" }\n"},
		{"module App\n\ntheme { radius: \"1rem\", light: { ring: \"#000\" }, dark: { } }\n", "module App\n\ntheme {\n  radius: \"1rem\",\n  light: { ring: \"#000\" },\n  dark: { },\n}\n"},
		{"module App\n\ntheme {\n  light: {\n    -- the brand\n    primary: \"#4f46e5\",\n  },\n}\n", ""},
		{"module App\n\ntheme { }\n", ""},
	} {
		want := c.want
		if want == "" {
			want = c.src
		}
		out, _, err := Source("App.alder", []byte(c.src))
		if err != nil || string(out) != want {
			t.Errorf("Source(%q) = %q, %v; want %q", c.src, out, err, want)
		}
	}
}

func TestSource_KeepsComments(t *testing.T) {
	src := "module M\n\n-- the answer\nx : Int\nx = 42 -- forty-two\n"
	out, _, err := Source("m.alder", []byte(src))
	if err != nil || string(out) != src {
		t.Fatalf("got %q, %v; want the input unchanged", out, err)
	}
}

// Output that drops a comment, or changes the comments' order, is refused.
func TestSource_DroppedCommentIsUnsafe(t *testing.T) {
	testMutate = func(b []byte) []byte { return []byte(strings.Replace(string(b), "-- the answer\n", "", 1)) }
	defer func() { testMutate = nil }()
	_, _, err := Source("m.alder", []byte("module M\n\n-- the answer\nx : Int\nx = 42\n"))
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}

func TestSource_ReorderedCommentsAreUnsafe(t *testing.T) {
	testMutate = func(b []byte) []byte {
		s := strings.Replace(string(b), "-- one", "-- tmp", 1)
		s = strings.Replace(s, "-- two", "-- one", 1)
		return []byte(strings.Replace(s, "-- tmp", "-- two", 1))
	}
	defer func() { testMutate = nil }()
	_, _, err := Source("m.alder", []byte("module M\n\n-- one\nx : Int\nx = 1\n\n-- two\ny : Int\ny = 2\n"))
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}

// A comment line inside markup text is a comment too: dropping it is refused.
func TestSource_DroppedMarkupCommentIsUnsafe(t *testing.T) {
	testMutate = func(b []byte) []byte { return []byte(strings.Replace(string(b), "-- note\n", "", 1)) }
	defer func() { testMutate = nil }()
	_, _, err := Source("m.alder", []byte("module M\n\nv : Html Never\nv =\n  <div>\n    -- note\n    <b>x</b>\n  </div>\n"))
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}

// Comments move with the imports they belong to, which are sorted: in the import block their
// order is free.
func TestSource_ImportCommentsMoveWithImports(t *testing.T) {
	out, _, err := Source("m.alder", []byte("module M\n\n-- for b\nimport B\nimport A -- for a\n"))
	if want := "module M\n\nimport A -- for a\n-- for b\nimport B\n"; err != nil || string(out) != want {
		t.Fatalf("got %q, %v; want %q", out, err, want)
	}
}

func TestSource_SpanTextIsByteBased(t *testing.T) {
	src := "module M\n\ns : Text\ns = \"héllo wörld\"\n"
	out, _, err := Source("m.alder", []byte(src))
	if err != nil || string(out) != src {
		t.Fatalf("got %q, %v", out, err)
	}
}

// A comment cannot move out of the import block, where the comments' order is free.
func TestSource_CommentLeavingImportBlockIsUnsafe(t *testing.T) {
	testMutate = func(b []byte) []byte {
		s := strings.Replace(string(b), "-- zz for a\n", "", 1)
		return []byte(strings.Replace(s, "import B\n", "import B\n-- zz for a\n", 1))
	}
	defer func() { testMutate = nil }()
	_, _, err := Source("m.alder", []byte("module M\n\n-- zz for a\nimport A\n-- b\nimport B\n\nx : Int\nx = 1\n"))
	if !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}

// Known limit: a code comment that has to lead its item, after a comment line of markup text in
// that item, cannot keep the order (the text's comment is printed with the text): refused.
func TestSource_MarkupCommentThenHoistIsUnsafe(t *testing.T) {
	src := "module M\n\nv : Bool\nv =\n  <p>\n    text\n    -- in the text\n  </p> == -- between operands\n    w\n"
	if _, diags := syntax.ParseFile("m.alder", []byte(src)); len(diags) > 0 {
		t.Fatal(diags)
	}
	if _, _, err := Source("m.alder", []byte(src)); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("err = %v, want ErrUnsafe", err)
	}
}
