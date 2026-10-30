package main

import (
	"os"
	"strings"
	"testing"
)

// Walking a project formats its scratch files too, outside app/ at any depth.
func TestFmtFormatsScratchFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Main.alder", fmtClean)
	root := fmtWrite(t, dir, "scratch.alder", "> 1   +1\n")
	nested := fmtWrite(t, dir, "notes/my_scratch.alder", "x=1\n> x\n")
	code, out, errOut := fmtRun("--check", dir)
	if code != 1 || errOut != "" || !strings.Contains(out, root+" would be reformatted") || !strings.Contains(out, nested+" would be reformatted") {
		t.Fatalf("--check: code %d out %q err %q", code, out, errOut)
	}
	if code, _, errOut := fmtRun(dir); code != 0 || errOut != "" {
		t.Fatalf("code %d err %q", code, errOut)
	}
	if got, _ := os.ReadFile(root); string(got) != "> 1 + 1\n" {
		t.Errorf("scratch.alder: %q", got)
	}
	if got, _ := os.ReadFile(nested); string(got) != "x = 1\n\n> x\n" {
		t.Errorf("notes/my_scratch.alder: %q", got)
	}
	if code, out, errOut := fmtRun("--check", dir); code != 0 || out != "" || errOut != "" {
		t.Fatalf("second --check: code %d out %q err %q", code, out, errOut)
	}
}

// A scratch file named on the command line is formatted (it used to be refused).
func TestFmtNamedScratchFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Main.alder", fmtClean)
	p := fmtWrite(t, dir, "probe.scratch.alder", "in Main\n> x+1\n")
	code, out, errOut := fmtRun(p)
	if code != 0 || errOut != "" || out != p+"\n" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	if got, _ := os.ReadFile(p); string(got) != "in Main\n\n> x + 1\n" {
		t.Fatalf("file %q", got)
	}
}

// A scratch file that does not parse is reported in its own lines, exit 1, and left alone.
func TestFmtScratchFileWithErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Main.alder", fmtClean)
	p := fmtWrite(t, dir, "scratch.alder", "x = 1\n>!Db.insert x\n")
	code, _, errOut := fmtRun(p)
	if code != 1 || !strings.Contains(errOut, "scratch.alder:2:1") || !strings.Contains(errOut, "not supported yet") {
		t.Fatalf("code %d err %q", code, errOut)
	}
	if got, _ := os.ReadFile(p); string(got) != "x = 1\n>!Db.insert x\n" {
		t.Fatalf("file changed: %q", got)
	}
}

// A scratch file outside any app (judged against the current directory) is still a scratch file.
func TestFmtScratchFileOutsideAnApp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := fmtWrite(t, dir, "scratch.alder", "> [1,2]\n")
	if code, _, errOut := fmtRun(p); code != 0 || errOut != "" {
		t.Fatalf("code %d err %q", code, errOut)
	}
	if got, _ := os.ReadFile(p); string(got) != "> [1, 2]\n" {
		t.Fatalf("file %q", got)
	}
}
