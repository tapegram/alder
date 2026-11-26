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

func TestFormat_Routes(t *testing.T) {
	in, err := os.ReadFile("testdata/routes/in.alder")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/routes/out.alder")
	if err != nil {
		t.Fatal(err)
	}
	got := format(t, "routes.alder", in)
	if string(got) != string(want) {
		t.Fatalf("golden mismatch\n--- got:\n%s\n--- want:\n%s", got, want)
	}
	if again := format(t, "routes.alder", got); string(again) != string(got) {
		t.Fatalf("not idempotent\n--- first:\n%s\n--- second:\n%s", got, again)
	}
}

func TestFormat_RoutesLongLineNotWrapped(t *testing.T) {
	src := "module M\n\nroute Index = GET \"/a/very/long/path/that/goes/on/and/on/{id}\" { id: Todo.TodoId, other: Text = \"default value\", more: Int = 12 } form SomeFormName requires user, role Admin | Owner\n"
	got := string(format(t, "m.alder", []byte(src)))
	if strings.Count(got, "\n") != 3 {
		t.Fatalf("route line wrapped:\n%s", got)
	}
}

func TestFormat_RoutesMoveCommentsWithThem(t *testing.T) {
	src := "module M\n\nroute A = GET \"/a\" { }\n\n-- about T\ntype T = { x: Int }\n\n-- about B\nroute B = GET \"/b\" { }\n"
	want := "module M\n\nroute A = GET \"/a\" { }\n-- about B\nroute B = GET \"/b\" { }\n\n-- about T\ntype T = { x: Int }\n"
	if got := string(format(t, "m.alder", []byte(src))); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormat_Job(t *testing.T) {
	block := "module M\n\n" +
		"job SendWelcome = { user: User.UserId }\n" +
		"job Digest      = { org: Org.OrgId } silo org\n" +
		"job Hourly      = {} every \"1h\"\n" +
		"job Nightly     = {} cron \"0 3 * * *\"\n" +
		"job Charge      = { invoice: Invoice.InvoiceId } retries 0 timeout \"30s\"\n" +
		"job BuildExport = { user: User.UserId } priority 10\n"
	if got := string(format(t, "m.alder", []byte(block))); got != block {
		t.Fatalf("not a fixed point\n--- got:\n%s\n--- want:\n%s", got, block)
	}
	got := string(format(t, "m.alder", []byte("module M\n\njob   A={}  retries 2\n")))
	if want := "module M\n\njob A = {} retries 2\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	got = string(format(t, "m.alder", []byte("module M\n\njob A={}  priority   -5\n")))
	if want := "module M\n\njob A = {} priority -5\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// options keep their source order
	got = string(format(t, "m.alder", []byte("module M\n\njob A = {x:Int}   timeout \"5s\"   retries 1\n")))
	if want := "module M\n\njob A = { x: Int } timeout \"5s\" retries 1\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// An SVG view (#306) is a fixed point: `viewBox` keeps its case and each shape stays `/>`-closed.
func TestFormat_SVG(t *testing.T) {
	src := "module Chart\n\nchart : Html\nchart =\n  <svg viewBox=\"0 0 10 10\" role=\"img\">\n    <title>t</title>\n" +
		"    <polyline points=\"0,0 5,5\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" />\n" +
		"    <line x1=\"0\" y1=\"0\" x2=\"10\" y2=\"0\" />\n  </svg>\n"
	if got := string(format(t, "chart.alder", []byte(src))); got != src {
		t.Fatalf("not a fixed point\n--- got:\n%s\n--- want:\n%s", got, src)
	}
}

// TestFormat_ValueRequires (#310): a value's `requires` clause stays on the signature line, and
// goes on its own indented line when the signature does not fit with it; either way a trailing
// comment keeps its place and the output is stable.
func TestFormat_ValueRequires(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"line", "module M\n\ncount : Query Int   requires  user,org\ncount = Query.succeed 1\n",
			"module M\n\ncount : Query Int requires user, org\ncount = Query.succeed 1\n"},
		{"next line joins", "module M\n\ncount : Query Int\n    requires org\ncount = Query.succeed 1\n",
			"module M\n\ncount : Query Int requires org\ncount = Query.succeed 1\n"},
		{"own line", "module M\n\nloadEverything : Dict Text (List Int) -> Dict Text (List Int) -> Dict Text (List Int) requires user, org\nloadEverything a b = a\n",
			"module M\n\nloadEverything : Dict Text (List Int) -> Dict Text (List Int) -> Dict Text (List Int)\n  requires user, org\nloadEverything a b = a\n"},
		{"signature breaks", "module M\n\nloadEverything : Dict Text (List Int) -> Dict Text (List Int) -> Dict Text (List Int) -> Dict Text (List Int) requires user\nloadEverything a b c = a\n",
			"module M\n\nloadEverything : Dict Text (List Int)\n  -> Dict Text (List Int)\n  -> Dict Text (List Int)\n  -> Dict Text (List Int)\n  requires user\nloadEverything a b c = a\n"},
		{"comment", "module M\n\ncount : Query Int requires org -- per org\ncount = Query.succeed 1\n",
			"module M\n\ncount : Query Int requires org -- per org\ncount = Query.succeed 1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := format(t, "m.alder", []byte(c.in))
			if string(got) != c.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, c.want)
			}
			if again := format(t, "m.alder", got); string(again) != string(got) {
				t.Fatalf("not idempotent:\n%s", again)
			}
		})
	}
}
