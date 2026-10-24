package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fmtMessy = "module Main\n\nx:Int\nx   =   1\n"
	fmtClean = "module Main\n\nx : Int\nx = 1\n"
)

func fmtWrite(t *testing.T, dir, rel, text string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func fmtRun(args ...string) (code int, out, errOut string) {
	var o, e bytes.Buffer
	code = runFmtTo(args, strings.NewReader(""), &o, &e)
	return code, o.String(), e.String()
}

func TestFmt_RewritesAndReports(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := fmtWrite(t, dir, "app/Main.alder", fmtMessy)
	code, out, errOut := fmtRun(dir)
	if code != 0 || errOut != "" || out != p+"\n" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	if got, _ := os.ReadFile(p); string(got) != fmtClean {
		t.Fatalf("file %q", got)
	}
	if code, out, errOut = fmtRun(dir); code != 0 || out != "" || errOut != "" {
		t.Fatalf("second run: code %d out %q err %q", code, out, errOut)
	}
}

func TestFmt_Check(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := fmtWrite(t, dir, "app/Main.alder", fmtMessy)
	code, out, _ := fmtRun("--check", dir)
	if code != 1 || out != p+" would be reformatted\n" {
		t.Fatalf("code %d out %q", code, out)
	}
	if got, _ := os.ReadFile(p); string(got) != fmtMessy {
		t.Fatalf("--check wrote the file: %q", got)
	}
	fmtWrite(t, dir, "app/Main.alder", fmtClean)
	if code, out, _ = fmtRun("--check", dir); code != 0 || out != "" {
		t.Fatalf("clean: code %d out %q", code, out)
	}
}

func TestFmt_Stdin(t *testing.T) {
	t.Parallel()
	var o, e bytes.Buffer
	if code := runFmtTo([]string{"--stdin"}, strings.NewReader(fmtMessy), &o, &e); code != 0 || o.String() != fmtClean || e.Len() != 0 {
		t.Fatalf("code %d out %q err %q", code, o.String(), e.String())
	}
	o.Reset()
	if code := runFmtTo([]string{"--stdin"}, strings.NewReader("module M\n\nf ="), &o, &e); code != 1 || o.Len() != 0 || e.Len() == 0 {
		t.Fatalf("parse error: code %d out %q err %q", code, o.String(), e.String())
	}
}

func TestFmt_ParseErrorSkips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	broken := fmtWrite(t, dir, "app/Broken.alder", "module Broken\n\nf =")
	good := fmtWrite(t, dir, "app/Main.alder", fmtMessy)
	code, out, errOut := fmtRun(dir)
	if code != 1 || out != good+"\n" || !strings.Contains(errOut, "Broken.alder") {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	if got, _ := os.ReadFile(broken); string(got) != "module Broken\n\nf =" {
		t.Fatalf("broken file touched: %q", got)
	}
	if got, _ := os.ReadFile(good); string(got) != fmtClean {
		t.Fatalf("good file not rewritten: %q", got)
	}
}

func TestFmt_SkipsBuildAndDotDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "build/Gen.alder", fmtMessy)
	fmtWrite(t, dir, ".git/Hook.alder", fmtMessy)
	fmtWrite(t, dir, "app/Main.alder", fmtClean)
	if code, out, errOut := fmtRun("--check", dir); code != 0 || out != "" || errOut != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

func TestFmt_BadFlag(t *testing.T) {
	t.Parallel()
	code, _, errOut := fmtRun("--nope")
	if code != 2 || !strings.HasPrefix(errOut, "alder fmt: ") {
		t.Fatalf("code %d err %q", code, errOut)
	}
}
