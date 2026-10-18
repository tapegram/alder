// internal/syntax/walk_test.go
package syntax

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
)

const walkSrc = `module M

-- double doubles.
-- Twice.
double : Int -> Int
double n = n * 2

greet : Text -> Text
greet name = "hé {name}"

view : Int -> Html
view n = <div class="x">{Html.text (greet "a")}</div>
`

func parse(t *testing.T, src string) *Module {
	t.Helper()
	m, ds := ParseFile("app/M.alder", []byte(src))
	if len(ds) > 0 {
		t.Fatalf("parse: %v", ds)
	}
	return m
}

// pos finds needle on its line (1-based line, 1-based byte column of the needle plus off).
func pos(t *testing.T, src, needle string, off int) diag.Pos {
	t.Helper()
	for i, l := range strings.Split(src, "\n") {
		if c := strings.Index(l, needle); c >= 0 {
			return diag.Pos{Line: i + 1, Col: c + 1 + off}
		}
	}
	t.Fatalf("no %q", needle)
	return diag.Pos{}
}

func TestNodeAt_InnermostVar(t *testing.T) {
	m := parse(t, walkSrc)
	path := NodeAt(m, pos(t, walkSrc, "n * 2", 0))
	v, ok := path[len(path)-1].(*Var)
	if !ok || v.Name != "n" {
		t.Fatalf("innermost = %T %+v", path[len(path)-1], path[len(path)-1])
	}
	if _, ok := path[1].(*ValueDecl); !ok {
		t.Fatalf("path[1] = %T, want *ValueDecl", path[1])
	}
}

func TestNodeAt_AfterMultiByte(t *testing.T) {
	m := parse(t, walkSrc)
	// `name` inside "hé {name}": é is two bytes, so the byte column is one more than the rune column
	path := NodeAt(m, pos(t, walkSrc, "name}", 0))
	if v, ok := path[len(path)-1].(*Var); !ok || v.Name != "name" {
		t.Fatalf("innermost = %T", path[len(path)-1])
	}
}

func TestNodeAt_HTML(t *testing.T) {
	m := parse(t, walkSrc)
	path := NodeAt(m, pos(t, walkSrc, `class=`, 0))
	if a, ok := path[len(path)-1].(*HtmlAttr); !ok || a.Name != "class" {
		t.Fatalf("innermost = %T", path[len(path)-1])
	}
	var el *Element
	for _, n := range path {
		if e, ok := n.(*Element); ok {
			el = e
		}
	}
	if el == nil || el.Tag != "div" {
		t.Fatalf("no enclosing div in %v", path)
	}
}

func TestNodeAt_OutsideEverything(t *testing.T) {
	m := parse(t, walkSrc)
	if p := NodeAt(m, diag.Pos{Line: 999, Col: 1}); len(p) != 1 {
		t.Fatalf("past the end: %v", p)
	}
}

func TestDocOf(t *testing.T) {
	m := parse(t, walkSrc)
	d := m.Decls[0].(*ValueDecl)
	if got := DocOf(m.Lines, d.Span.Start.Line); got != "double doubles.\nTwice." {
		t.Fatalf("DocOf = %q", got)
	}
	if got := DocOf(m.Lines, m.Decls[1].GetSpan().Start.Line); got != "" {
		t.Fatalf("no comment: %q", got)
	}
}

// TestChildren_SpansNest walks every example module and checks that each child with a non-empty
// span lies within its parent's span (parents with an empty span are exempt).
func TestChildren_SpansNest(t *testing.T) {
	var files []string
	err := filepath.WalkDir("../../examples", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".alder") && strings.Contains(p, "/app/") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no example files")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		m, ds := ParseFile(f, src)
		if len(ds) > 0 {
			t.Fatalf("%s: parse: %v", f, ds)
		}
		var visit func(n Spanned)
		visit = func(n Spanned) {
			ps := n.GetSpan()
			for _, k := range Children(n) {
				ks := k.GetSpan()
				if ks.Start != ks.End && ps.Start != ps.End &&
					(less(ks.Start, ps.Start) || less(ps.End, ks.End)) {
					t.Errorf("%s: %T %v not within parent %T %v", f, k, ks, n, ps)
				}
				visit(k)
			}
		}
		visit(m)
	}
}
