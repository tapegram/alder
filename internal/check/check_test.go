package check

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

var update = flag.Bool("update", false, "rewrite golden files")

func runDir(t *testing.T, dir string) *diag.Bag {
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := names.Load(dir, std)
	for _, e := range ioErrs {
		t.Fatal(e)
	}
	if !prog.ParseFailed { // Load's bag also holds module-graph errors (cycles), which only gate their modules
		prog.Resolve(bag)
		prog.MarkFailed(bag)
		Program(prog, bag)
	}
	bag.Sort()
	return bag
}

func render(bag *diag.Bag) string {
	var sb strings.Builder
	for _, d := range bag.Items {
		if d.Severity != "error" {
			continue
		}
		fmt.Fprintf(&sb, "%s:%d:%d %s %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message)
		if d.Fix != nil {
			fmt.Fprintf(&sb, "  fix %d:%d-%d:%d %s\n", d.Fix.Line, d.Fix.Col, d.Fix.EndLine, d.Fix.EndCol, strings.ReplaceAll(d.Fix.Replace, "\n", "⏎"))
		}
	}
	return sb.String()
}

func TestCheckGolden(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/check_*")
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got := render(runDir(t, dir))
			golden := dir + ".diag"
			if *update {
				os.WriteFile(golden, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden %s", golden)
			}
			if got != string(want) {
				t.Fatalf("golden mismatch\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestCorpus(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/corpus/*")
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got := render(runDir(t, dir))
			if *update {
				if err := os.WriteFile(filepath.Join(dir, "expected.diag"), []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(filepath.Join(dir, "expected.diag"))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("corpus mismatch\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestExamplesCheckClean(t *testing.T) {
	for _, ex := range []string{"../../examples/todos"} {
		if got := render(runDir(t, ex)); got != "" {
			t.Fatalf("%s should check clean:\n%s", ex, got)
		}
	}
}

// TestFixesApply proves every corpus fix is well-formed: applying it to the source must not error
// and must change the file.
func TestFixesApply(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/corpus/*")
	golden, _ := filepath.Glob("../../testdata/golden/check_*")
	for _, g := range golden {
		if st, err := os.Stat(g); err == nil && st.IsDir() {
			dirs = append(dirs, g)
		}
	}
	// outputs that must contain a given text after applying the case's fix
	want := map[string]string{
		"E0102-maybe-field/E0102":   "(match user { Just x -> x.name, Nothing -> ?default }) }",
		"check_fix_multiline/E0003": "|> List.length", // a multi-line span's fix must keep the whole expression
		"E0101-get-by-slug/E0101":   "Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first\n",
	}
	for _, dir := range dirs {
		bag := runDir(t, dir)
		for _, d := range bag.Items {
			if d.Fix == nil {
				continue
			}
			src, err := os.ReadFile(filepath.Join(dir, d.Span.File))
			if err != nil {
				t.Fatal(err)
			}
			out, err := diag.Apply(string(src), []diag.Fix{*d.Fix})
			if err != nil {
				t.Errorf("%s %s: fix does not apply: %v", dir, d.Code, err)
			} else if out == string(src) {
				t.Errorf("%s %s: fix changes nothing", dir, d.Code)
			} else if _, pd := syntax.ParseFile(d.Span.File, []byte(out)); len(pd) > 0 && !strings.Contains(d.Fix.Replace, "?type") { // `?type` is a hole in type position, which the parser rejects by design
				t.Errorf("%s %s: fix must leave the file parseable: %v\n%s", dir, d.Code, pd[0].Message, out)
			} else if w, ok := want[filepath.Base(dir)+"/"+d.Code]; ok && !strings.Contains(out, w) {
				t.Errorf("%s %s: applied output lacks %q:\n%s", dir, d.Code, w, out)
			}
		}
	}
}
