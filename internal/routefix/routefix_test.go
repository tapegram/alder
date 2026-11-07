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
	ins, _ := filepath.Glob("testdata/*/in.alder")
	if len(ins) == 0 {
		t.Fatal("no cases")
	}
	for _, in := range ins {
		dir := filepath.Dir(in)
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

// TestProgram runs every case under testdata/program: the files under in/ (keyed by their path
// there) go through Program, and each output is compared with the file of the same path under
// out/; the holes with `holes` ("file:line: why" a line). A case with an `error` file expects
// ErrNameCollision naming what the file holds; `name` is Options.Name.
func TestProgram(t *testing.T) {
	dirs, _ := filepath.Glob("testdata/program/*")
	if len(dirs) == 0 {
		t.Fatal("no cases")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			in := tree(t, filepath.Join(dir, "in"))
			out, holes, err := Program(in, Options{Name: strings.TrimSpace(read(t, filepath.Join(dir, "name")))})
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
			want := tree(t, filepath.Join(dir, "out"))
			for f, w := range want {
				if string(out[f]) != string(w) {
					t.Errorf("%s differs:\n%s", f, lineDiff(string(w), string(out[f])))
				}
			}
			for f := range out {
				if want[f] == nil {
					t.Errorf("%s: an output with no out/ file", f)
				}
			}
			var got []string
			for _, h := range holes {
				got = append(got, fmt.Sprintf("%s:%d: %s", h.File, h.Line, h.Why))
			}
			if g, w := strings.Join(got, "\n"), strings.TrimSpace(read(t, filepath.Join(dir, "holes"))); g != w {
				t.Errorf("holes:\n%s\nwant:\n%s", g, w)
			}
			// a second run changes nothing
			again, more, err := Program(out, Options{})
			if err != nil {
				t.Fatalf("second run: %v", err)
			}
			for f := range out {
				if string(again[f]) != string(out[f]) {
					t.Errorf("%s: a second run changed it:\n%s", f, lineDiff(string(out[f]), string(again[f])))
				}
			}
			if len(more) != 0 {
				t.Errorf("a second run left %d holes", len(more))
			}
		})
	}
}

// tree is every .alder file under root, by its path relative to root.
func tree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".alder" {
			return err
		}
		b, err := os.ReadFile(path)
		rel, _ := filepath.Rel(root, path)
		files[filepath.ToSlash(rel)] = b
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
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

// TestProgram_RepoApps rewrites every app of the repo's examples and of the corpus's `-ok` cases
// with a page or a message: none is refused (but for a name collision), each output formats to
// itself, and a second run changes nothing.
func TestProgram_RepoApps(t *testing.T) {
	var apps []string
	for _, g := range []string{"../../examples/*/app", "../../testdata/corpus/*-ok/app"} {
		ds, _ := filepath.Glob(g)
		apps = append(apps, ds...)
	}
	n := 0
	for _, app := range apps {
		files := tree(t, app)
		old := false
		for _, src := range files {
			if strings.Contains("\n"+string(src), "\npage ") || strings.Contains("\n"+string(src), "\nmsg ") {
				old = true
			}
		}
		if !old {
			continue
		}
		n++
		out, _, err := Program(files, Options{})
		var nc *ErrNameCollision
		if errors.As(err, &nc) {
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", app, err)
			continue
		}
		for f, b := range out {
			if string(b) == string(files[f]) {
				continue
			}
			if again, _, err := format.Source(f, b); err != nil || string(again) != string(b) {
				t.Errorf("%s/%s: the output does not format to itself (%v)", app, f, err)
			}
		}
		again, _, err := Program(out, Options{})
		if err != nil {
			t.Errorf("%s: second run: %v", app, err)
			continue
		}
		for f := range out {
			if string(again[f]) != string(out[f]) {
				t.Errorf("%s/%s: a second run changed it", app, f)
			}
		}
	}
	if n == 0 {
		t.Fatal("no apps found")
	}
}
