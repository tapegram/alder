package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runDump(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runDumpTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestDump_AST(t *testing.T) {
	code, out, errs := runDump("../../testdata/golden/gen_basic/app/M.alder")
	if code != 0 || !strings.Contains(out, "(val area") || errs != "" {
		t.Fatalf("code %d, out %s, err %s", code, out, errs)
	}
	code2, out2, _ := runDump("--ast", "../../testdata/golden/gen_basic/app/M.alder")
	if code2 != 0 || out2 != out {
		t.Fatalf("--ast differs from the default: %d %s", code2, out2)
	}
}

func TestDump_IR(t *testing.T) {
	code, out, errs := runDump("--ir", "../../testdata/golden/gen_basic")
	want, err := os.ReadFile("../../testdata/golden/ir_basic.ir")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || out != string(want) {
		t.Fatalf("code %d\nerr %s\nout %s", code, errs, out)
	}
}

func TestDump_Tokens(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "V.alder")
	if err := os.WriteFile(f, []byte("module V\nv = <p>hi {x}</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runDump("--tokens", f)
	if code != 0 || errs != "" {
		t.Fatalf("code %d err %s", code, errs)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if lines[0] != `1:1 Keyword "module"` || lines[2] != `1:9 NL "\n"` || lines[8] != `2:8 Text "hi "` || lines[len(lines)-1] != `2:18 NL "\n"` {
		t.Fatalf("tokens:\n%s", out)
	}
}

func TestDump_ParseErrorExits1(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "B.alder")
	if err := os.WriteFile(f, []byte("module B\nx = (1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runDump(f)
	if code != 1 || !strings.Contains(errs, "E0001") || !strings.Contains(out, "(module B") {
		t.Fatalf("code %d out %s err %s", code, out, errs)
	}
}

func TestDump_IRCheckErrorExits1(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "X.alder"), []byte("module X\n\nx : Int\nx = \"s\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runDump("--ir", dir)
	if code != 1 || out != "" || !strings.Contains(errs, "E0003") {
		t.Fatalf("code %d out %s err %s", code, out, errs)
	}
}

func TestDump_Usage(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--ir", "../../testdata/golden/gen_basic/app/M.alder"},
		{"--tokens", "--ir", "../../testdata/golden/gen_basic"},
		{"--bogus", "x.alder"},
		{"a.alder", "b.alder"},
		{"../../testdata/golden/no_such_file.alder"},
	} {
		code, out, errs := runDump(args...)
		if code != 2 || out != "" || errs == "" {
			t.Errorf("%v: code %d out %q err %q", args, code, out, errs)
		}
	}
	_, _, errs := runDump("--ir", "../../testdata/golden/gen_basic/app/M.alder")
	if !strings.Contains(errs, "--ir needs an app directory") {
		t.Errorf("message: %s", errs)
	}
}

func TestDump_DirAST(t *testing.T) {
	code, out, errs := runDump("../../testdata/golden/gen_basic")
	if code != 0 || !strings.HasPrefix(out, "(module M") || errs != "" {
		t.Fatalf("code %d out %s err %s", code, out, errs)
	}
}
