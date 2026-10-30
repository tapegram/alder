package main

import (
	"fmt"
	"io/fs"
	"syscall"

	"alder/internal/format"
	"alder/internal/generate"
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

func TestFmt_WritesThroughSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := fmtWrite(t, dir, "real/Main.alder", fmtMessy)
	link := filepath.Join(dir, "app", "Link.alder")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	if code, _, errOut := fmtRun(link); code != 0 {
		t.Fatalf("code %d err %q", code, errOut)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced: %v %v", fi, err)
	}
	if got, _ := os.ReadFile(real); string(got) != fmtClean {
		t.Fatalf("target %q", got)
	}
}

func TestFmt_UnsafeSkipsWrite(t *testing.T) {
	t.Parallel()
	var e bytes.Buffer
	code := fmtResult("app/Main.alder", nil, []byte("x"), nil, fmt.Errorf("x: %w", format.ErrUnsafe), &e, func([]byte) int {
		t.Error("write callback ran")
		return 0
	})
	if code != 2 || !strings.Contains(e.String(), "a known limitation") || strings.Contains(e.String(), "file an issue") || strings.Contains(e.String(), "internal error") {
		t.Fatalf("code %d err %q", code, e.String())
	}
}

// TestFmt_ReadOnlyDirWritesInPlace (#28): where no temporary file can be made the file is written
// in place, keeping its mode.
func TestFmt_ReadOnlyDirWritesInPlace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	dir := t.TempDir()
	p := fmtWrite(t, dir, "app/Main.alder", fmtMessy)
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	ro := filepath.Dir(p)
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(ro, 0o755) })
	if code, _, errOut := fmtRun(p); code != 0 {
		t.Fatalf("code %d err %q", code, errOut)
	}
	if got, _ := os.ReadFile(p); string(got) != fmtClean {
		t.Fatalf("file %q", got)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}

// A full disk (no temporary file) must not fall back to truncating the file.
func TestFmt_TempFileErrorOtherThanPermissionWritesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := fmtWrite(t, dir, "app/Main.alder", fmtMessy)
	cache := &fmtCache{noted: map[string]bool{}, progs: map[string]*fmtProject{}}
	cache.hooks.createTemp = func(string, string) (*os.File, error) { return nil, syscall.ENOSPC }
	cache.hooks.writeInPlace = func(string, []byte, os.FileMode) error { t.Error("wrote in place"); return nil }
	var o, e bytes.Buffer
	if code := fmtFile(p, false, cache, &o, &e); code != 2 || o.Len() != 0 {
		t.Fatalf("code %d out %q err %q", code, o.String(), e.String())
	}
	if got, _ := os.ReadFile(p); string(got) != fmtMessy {
		t.Errorf("file %q", got)
	}
}

// A failed in-place write (after the file was truncated) is put back.
func TestFmt_InPlaceFailureRestoresOriginal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := fmtWrite(t, dir, "app/Main.alder", fmtMessy)
	cache := &fmtCache{noted: map[string]bool{}, progs: map[string]*fmtProject{}}
	cache.hooks.createTemp = func(string, string) (*os.File, error) { return nil, fs.ErrPermission }
	calls := 0
	cache.hooks.writeInPlace = func(path string, data []byte, mode os.FileMode) error {
		calls++
		if calls == 1 {
			os.WriteFile(path, data[:3], mode) // partial write, then failure
			return syscall.EIO
		}
		return os.WriteFile(path, data, mode)
	}
	var o, e bytes.Buffer
	if code := fmtFile(p, false, cache, &o, &e); code != 2 || !strings.Contains(e.String(), "original restored") {
		t.Fatalf("code %d err %q", code, e.String())
	}
	if got, _ := os.ReadFile(p); string(got) != fmtMessy {
		t.Errorf("file %q", got)
	}
}

// TestFmt_FileChangedBeforeRename (#28): a file changed after it was read is not replaced.
func TestFmt_FileChangedBeforeRename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := fmtWrite(t, dir, "app/Main.alder", fmtMessy)
	const edited = "module Main\n\ny : Int\ny   =   2\n"
	cache := &fmtCache{noted: map[string]bool{}, progs: map[string]*fmtProject{}}
	cache.hooks.before = func() {
		if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
			t.Error(err)
		}
	}
	var o, e bytes.Buffer
	if code := fmtFile(p, false, cache, &o, &e); code != 2 || !strings.Contains(e.String(), "changed while it was being formatted") || o.Len() != 0 {
		t.Fatalf("code %d out %q err %q", code, o.String(), e.String())
	}
	if got, _ := os.ReadFile(p); string(got) != edited {
		t.Errorf("file overwritten: %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".alder-fmt-*")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

// TestExamples_AreFormatted: the example apps and the scaffold `alder new` writes are as `alder
// fmt` prints them, so the code an agent reads and copies is canonical.
func TestExamples_AreFormatted(t *testing.T) {
	if code, out, errOut := fmtRun("--check", "../../examples", "../../internal/scaffold/files"); code != 0 {
		t.Fatalf("alder fmt --check examples internal/scaffold/files: exit %d (run `alder fmt` on them)\n%s%s", code, out, errOut)
	}
}

// TestGenerated_IsFormatted: every Alder file the generators wrote in TestGenerate's cases
// (testdata/generate/want/*/files) is as `alder fmt` prints it.
func TestGenerated_IsFormatted(t *testing.T) {
	n := 0
	err := filepath.WalkDir("../../testdata/generate/want", func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".alder") {
			return err
		}
		n++
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out, diags, err := format.Source(p, b)
		switch {
		case len(diags) > 0:
			t.Errorf("%s does not parse: %s", p, diags[0].Message)
		case err != nil:
			t.Errorf("%v", err)
		case string(out) != string(b):
			t.Errorf("%s is not formatted:\n%s", p, generate.UnifiedDiff(p, string(b), string(out)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no generated Alder file found")
	}
}

func TestFmtSkipsScratchFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Main.alder", fmtClean)
	fmtWrite(t, dir, "scratch.alder", "> 1 + 1\n")
	fmtWrite(t, dir, "notes/my_scratch.alder", "> 1   +1\n")
	if code, out, errOut := fmtRun("--check", dir); code != 0 || out != "" || errOut != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

func TestFmtRefusesScratchFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Main.alder", fmtClean)
	p := fmtWrite(t, dir, "scratch.alder", "> 1 + 1\n")
	code, _, errOut := fmtRun(p)
	if code != 2 || !strings.Contains(errOut, "is a scratch file; scratch files are not formatted yet") {
		t.Fatalf("code %d err %q", code, errOut)
	}
}

func TestFmtWalkBelowAppKeepsScratchNamedModules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := fmtWrite(t, dir, "app/Posts/x.scratch.alder", fmtMessy)
	for _, target := range []string{filepath.Join(dir, "app"), filepath.Join(dir, "app", "Posts")} {
		if code, out, _ := fmtRun("--check", target); code != 1 || !strings.Contains(out, "x.scratch.alder would be reformatted") {
			t.Fatalf("%s: code %d out %q", target, code, out)
		}
	}
	if code, _, _ := fmtRun(filepath.Join(dir, "app")); code != 0 {
		t.Fatal(code)
	}
	if got, _ := os.ReadFile(p); string(got) != fmtClean {
		t.Fatalf("not formatted: %q", got)
	}
}

// A directory argument is judged against its own app root: the directory itself when it holds
// app/*.alder, and a scratch file there is skipped even with the cwd elsewhere.
func TestFmtAppRootStartsAtTheDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Main.alder", fmtClean)
	fmtWrite(t, dir, "sub/app/Main.alder", fmtClean) // a nested app
	fmtWrite(t, dir, "sub/x.scratch.alder", "> 1\n")
	sub := filepath.Join(dir, "sub")
	if got := fmtAppRoot(sub); got != sub {
		t.Errorf("fmtAppRoot(dir) = %q, want %q", got, sub)
	}
	if got := fmtAppRoot(filepath.Join(sub, "x.scratch.alder")); got != sub {
		t.Errorf("fmtAppRoot(file) = %q, want %q", got, sub)
	}
	if got := fmtAppRoot(filepath.Join(dir, "app")); got != dir {
		t.Errorf("fmtAppRoot(app) = %q, want %q", got, dir)
	}
	// an empty app/ directory is not an app, as for `alder scratch`
	empty := t.TempDir()
	os.MkdirAll(filepath.Join(empty, "app"), 0o755)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := fmtAppRoot(empty); got != cwd { // no app above it either: the current directory
		t.Errorf("fmtAppRoot(empty app/) = %q, want the cwd %q", got, cwd)
	}
}

const (
	fmtTodoSrc  = "module Table.Todo\n\ntype Todo = { title: Text }\n\nfind : Int -> Todo\nfind n = ?find\n"
	fmtUseOld   = "module Todos\n\nimport Table.Todo\n\na : Table.Todo\na = Table.Todo.find 1\n"
	fmtUseFixed = "module Todos\n\nimport Table.Todo\n\na : Todo\na = Todo.find 1\n"
)

func TestFmt_MigratesProjectFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Table/Todo.alder", fmtTodoSrc)
	p := fmtWrite(t, dir, "app/Todos.alder", fmtUseOld)
	if code, out, _ := fmtRun("--check", p); code != 1 || out != p+" would be reformatted\n" {
		t.Fatalf("--check: code %d out %q", code, out)
	}
	if code, out, errOut := fmtRun(p); code != 0 || out != p+"\n" || errOut != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	if got, _ := os.ReadFile(p); string(got) != fmtUseFixed {
		t.Fatalf("file %q", got)
	}
	if code, out, _ := fmtRun("--check", p); code != 0 || out != "" {
		t.Fatalf("second --check: code %d out %q", code, out)
	}
}

// A project file that does not parse turns migration off for the run; fmt says so once on
// stderr, naming the file, and the exit code stays the formatter's.
func TestFmt_BrokenSiblingNotesSkippedMigration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Table/Todo.alder", fmtTodoSrc)
	fmtWrite(t, dir, "app/Broken.alder", "module Broken\n\nx = (\n")
	p := fmtWrite(t, dir, "app/Todos.alder", fmtUseOld)
	q := fmtWrite(t, dir, "app/Other.alder", strings.Replace(fmtUseOld, "module Todos", "module Other", 1))
	code, out, errOut := fmtRun(p, q)
	if code != 0 || out != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	want := "alder fmt: app/Broken.alder does not parse, so imports were not migrated; fix it and run again\n"
	if errOut != want {
		t.Fatalf("stderr %q, want %q", errOut, want)
	}
	if got, _ := os.ReadFile(p); string(got) != fmtUseOld {
		t.Fatalf("file %q", got)
	}
}

func TestFmt_StdinNoMigration(t *testing.T) {
	t.Parallel()
	var o, e bytes.Buffer
	if code := runFmtTo([]string{"--stdin"}, strings.NewReader(fmtUseOld), &o, &e); code != 0 || o.String() != fmtUseOld {
		t.Fatalf("code %d out %q err %q", code, o.String(), e.String())
	}
}

// Several files of one project in one run are each migrated, though the run loads the project
// once and drops it only after a write.
func TestFmt_MigratesSeveralFilesOfOneProject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fmtWrite(t, dir, "app/Table/Todo.alder", fmtTodoSrc)
	p := fmtWrite(t, dir, "app/Todos.alder", fmtUseOld)
	q := fmtWrite(t, dir, "app/Other.alder", strings.Replace(fmtUseOld, "module Todos", "module Other", 1))
	if code, out, errOut := fmtRun(p, q); code != 0 || out != p+"\n"+q+"\n" || errOut != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	for _, f := range []string{p, q} {
		if got, _ := os.ReadFile(f); !strings.HasSuffix(string(got), "a : Todo\na = Todo.find 1\n") {
			t.Fatalf("%s: %q", f, got)
		}
	}
}
