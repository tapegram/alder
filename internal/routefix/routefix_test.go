package routefix

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/format"
	"alder/internal/syntax"
)

// TestModule runs every case under testdata: in.alder is rewritten with the options the case's
// files give (`name`: Options.Name; `pages`: Options.Pages, one module a line) and compared with
// out.alder, and the holes with `holes` ("line: why" a line, none when the file is missing). A
// case with an `error` file expects ErrNameCollision naming what the file holds.
func TestModule(t *testing.T) {
	dirs, _ := filepath.Glob("testdata/*")
	if len(dirs) == 0 {
		t.Fatal("no cases")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			src := read(t, filepath.Join(dir, "in.alder"))
			opts := Options{Name: strings.TrimSpace(read(t, filepath.Join(dir, "name")))}
			for _, p := range strings.Fields(read(t, filepath.Join(dir, "pages"))) {
				if opts.Pages == nil {
					opts.Pages = map[string]bool{}
				}
				opts.Pages[p] = true
			}
			out, holes, err := Module([]byte(src), "in.alder", opts)
			if want := strings.TrimSpace(read(t, filepath.Join(dir, "error"))); want != "" {
				var nc *ErrNameCollision
				if !errors.As(err, &nc) || nc.Name != want {
					t.Fatalf("want ErrNameCollision{%s}, got %v", want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := read(t, filepath.Join(dir, "out.alder")); string(out) != want {
				t.Errorf("out differs:\n%s", lineDiff(want, string(out)))
			}
			var got []string
			for _, h := range holes {
				got = append(got, fmt.Sprintf("%d: %s", h.Line, h.Why))
			}
			want := strings.TrimSpace(read(t, filepath.Join(dir, "holes")))
			if g := strings.Join(got, "\n"); g != want {
				t.Errorf("holes:\n%s\nwant:\n%s", g, want)
			}
		})
	}
}

// TestModule_Idempotent: a rewritten module has no page left, so a second run changes nothing,
// and the output is what `alder fmt` writes.
func TestModule_Idempotent(t *testing.T) {
	for _, c := range []string{"messages", "model-in-arm", "comments", "notice"} {
		out := read(t, filepath.Join("testdata", c, "out.alder"))
		again, holes, err := Module([]byte(out), "out.alder", Options{})
		if err != nil || string(again) != out || len(holes) != 0 {
			t.Errorf("%s: a second run changed it (err %v, %d holes)", c, err, len(holes))
		}
		f, _, err := format.Source("out.alder", []byte(out))
		if err != nil || string(f) != out {
			t.Errorf("%s: out.alder is not formatted (%v)", c, err)
		}
	}
}

// read is the file's content, "" when it does not exist.
func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// lineDiff lists the lines where want and got first differ, with a little context.
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			return fmt.Sprintf("line %d\nwant: %q\ngot:  %q\n--- got from there:\n%s", i+1, a, b, strings.Join(g[max(i-3, 0):min(i+12, len(g))], "\n"))
		}
	}
	return "(equal lines, different bytes)"
}

// TestModule_RepoPages rewrites every page module of the repo's examples and corpus that parses:
// none is refused (but for a name collision) and each output formats to itself.
func TestModule_RepoPages(t *testing.T) {
	var files []string
	for _, g := range []string{"../../examples/*/app/*.alder", "../../examples/*/app/*/*.alder", "../../examples/*/app/*/*/*.alder",
		"../../testdata/corpus/*-ok/app/*.alder", "../../testdata/corpus/*-ok/app/*/*.alder"} {
		fs, _ := filepath.Glob(g)
		files = append(files, fs...)
	}
	n := 0
	for _, f := range files {
		src := read(t, f)
		if !strings.Contains(src, "\npage ") {
			continue
		}
		if _, diags := syntax.ParseFile(f, []byte(src)); len(diags) > 0 {
			continue
		}
		n++
		out, _, err := Module([]byte(src), f, Options{})
		var nc *ErrNameCollision
		if errors.As(err, &nc) {
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if again, _, err := format.Source(f, out); err != nil || string(again) != string(out) {
			t.Errorf("%s: the output does not format to itself (%v)", f, err)
		}
	}
	if n == 0 {
		t.Fatal("no page modules found")
	}
}
