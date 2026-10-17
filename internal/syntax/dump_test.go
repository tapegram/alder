// internal/syntax/dump_test.go
package syntax

import "testing"

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
