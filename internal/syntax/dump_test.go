// internal/syntax/dump_test.go
package syntax

import (
	"strings"
	"testing"
)

func TestDump(t *testing.T) {
	e := &App{Fn: &Var{Name: "add"}, Args: []Expr{&IntLit{Value: 1}, &Var{Module: "M", Name: "x"}}}
	got := Dump(e)
	want := "(app (var add) (int 1) (var M.x))"
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestParseDeploy(t *testing.T) {
	src := "module App\n\ndeploy {\n  production: Ssh \"u@h\" { domain: \"d\", path: \"/p\", port: 22 },\n  local: Script \"./s.sh\",\n  floor: 0.9,\n  requires: [\"A\", \"B\"],\n  backup: S3 \"s3://b\",\n  proxy: None,\n}\n\ndeploy = 1\n"
	m, ds := ParseFile("App.alder", []byte(src))
	if len(ds) != 0 {
		t.Fatalf("diagnostics: %v", ds)
	}
	got := Dump(m.Decls[0])
	want := `(deploy (production Ssh "u@h" {(domain "d") (path "/p") (port 22)}) (local Script "./s.sh") (floor 0.9) (requires ["A" "B"]) (backup S3 "s3://b") (proxy None))`
	if got != want {
		t.Fatalf("got %s", got)
	}
	if len(m.Decls) != 2 { // `deploy = 1` stays a value
		t.Fatalf("decls: %d", len(m.Decls))
	}
}

func TestParse_ThemeDecl(t *testing.T) {
	src := "module App\n\ntheme {\n  radius: \"0.5rem\",\n  fontSans: [\"Inter\", \"sans-serif\"],\n  light: {\n    primary: \"oklch(0.55 0.22 285)\",\n  },\n  dark: { },\n}\n"
	m, ds := ParseFile("App.alder", []byte(src))
	if len(ds) != 0 {
		t.Fatalf("diagnostics: %v", ds)
	}
	got := Dump(m.Decls[0])
	want := `(theme (radius "0.5rem") (fontSans ["Inter" "sans-serif"]) (light {(primary "oklch(0.55 0.22 285)")}) (dark {}))`
	if got != want {
		t.Fatalf("got %s", got)
	}
	d := m.Decls[0].(*ThemeDecl)
	if v := d.Fields[3].Value; v.Kind != "record" || v.Rec == nil || v.Ctor != "" {
		t.Errorf("an empty record value: %+v", v)
	}
}

// `theme` is contextual: only `theme {` at the start of a declaration is the declaration.
func TestParse_ThemeStaysAName(t *testing.T) {
	src := "module M\n\ntheme : Int\ntheme = 1\n\nf : { theme: Int } -> Int\nf r = r.theme\n\ng : Int -> Int\ng theme = theme\n"
	m, ds := ParseFile("M.alder", []byte(src))
	if len(ds) != 0 {
		t.Fatalf("diagnostics: %v", ds)
	}
	for _, d := range m.Decls {
		if _, ok := d.(*ThemeDecl); ok {
			t.Fatalf("a ThemeDecl in %s", Dump(m))
		}
	}
}

func TestDump_IgnoresComments(t *testing.T) {
	with, _ := ParseFile("m.alder", []byte(commentsSrc))
	m, _ := ParseFile("m.alder", []byte(commentsSrc))
	m.Comments = nil
	if Dump(with) != Dump(m) {
		t.Errorf("Dump differs with comments:\n%s\nvs\n%s", Dump(with), Dump(m))
	}
	if strings.Contains(Dump(with), "head") {
		t.Errorf("Dump prints comments:\n%s", Dump(with))
	}
}
