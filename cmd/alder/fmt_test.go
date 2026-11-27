package main

import (
	"fmt"
	"io/fs"
	"syscall"

	"alder/internal/buildkit"
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
	if code := fmtFile(p, false, false, cache, &o, &e); code != 2 || o.Len() != 0 {
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
	if code := fmtFile(p, false, false, cache, &o, &e); code != 2 || !strings.Contains(e.String(), "original restored") {
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
	if code := fmtFile(p, false, false, cache, &o, &e); code != 2 || !strings.Contains(e.String(), "changed while it was being formatted") || o.Len() != 0 {
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
// app/*.alder, and a scratch file there is judged against it even with the cwd elsewhere.
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

// #308: the handler-rename fixture of the corpus: a PATCH route's handler still named `update`,
// referenced from its own module and from another.
func fmtHandlerProject(t *testing.T) (dir string, files map[string]string) {
	t.Helper()
	dir = t.TempDir()
	src := "../../testdata/corpus/W0013-handler-update"
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "expected.diag")); err != nil {
		t.Fatal(err)
	}
	return dir, map[string]string{"todos": filepath.Join(dir, "app", "Todos.alder"), "checks": filepath.Join(dir, "app", "Checks.alder")}
}

// fmtCheckClean fails unless the app at dir checks with no diagnostic, warnings included.
func fmtCheckClean(t *testing.T, dir string) {
	t.Helper()
	_, _, bag, ioErrs := buildkit.LoadChecked(dir, nil)
	if len(ioErrs) > 0 {
		t.Fatal(ioErrs)
	}
	for _, d := range bag.Items {
		t.Errorf("after fmt: %s %s:%d:%d %s", d.Code, d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Message)
	}
}

// `alder fmt` over the project renames the handler's signature, its definition and every
// reference to it, in its module and in another (W0013); a second run changes nothing.
func TestMigrate_HandlerRename(t *testing.T) {
	t.Parallel()
	dir, f := fmtHandlerProject(t)
	if code, out, errOut := fmtRun(dir); code != 0 || errOut != "" || !strings.Contains(out, "Todos.alder") || !strings.Contains(out, "Checks.alder") {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	todos, _ := os.ReadFile(f["todos"])
	checks, _ := os.ReadFile(f["checks"])
	for _, want := range []string{"Toggle.patch : Toggle.Params -> Session -> Task Response\nToggle.patch params session", "Test.run (Toggle.patch {"} {
		if !strings.Contains(string(todos), want) {
			t.Errorf("Todos.alder lacks %q:\n%s", want, todos)
		}
	}
	if !strings.Contains(string(checks), "Test.run (Todos.Toggle.patch {") {
		t.Errorf("Checks.alder not renamed:\n%s", checks)
	}
	if strings.Contains(string(todos)+string(checks), "update") {
		t.Errorf("`update` remains:\n%s\n%s", todos, checks)
	}
	if code, out, errOut := fmtRun(dir); code != 0 || out != "" || errOut != "" {
		t.Fatalf("second run: code %d out %q err %q", code, out, errOut)
	}
	fmtCheckClean(t, dir)
}

// `alder fmt` on the route module alone leaves the handler as `update`: other files still name
// it, and renaming only the declaration would break them. The app still checks, with W0013 only.
func TestMigrate_HandlerRenameSingleFileWaits(t *testing.T) {
	t.Parallel()
	dir, f := fmtHandlerProject(t)
	before, _ := os.ReadFile(f["todos"])
	if code, _, errOut := fmtRun(f["todos"]); code != 0 || errOut != "" {
		t.Fatalf("code %d err %q", code, errOut)
	}
	if got, _ := os.ReadFile(f["todos"]); string(got) != string(before) {
		t.Fatalf("Todos.alder changed:\n%s", got)
	}
	_, _, bag, ioErrs := buildkit.LoadChecked(dir, nil)
	if len(ioErrs) > 0 {
		t.Fatal(ioErrs)
	}
	for _, d := range bag.Items {
		if d.Code != "W0013" {
			t.Errorf("after fmt of one file: %s %s:%d:%d %s", d.Code, d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Message)
		}
	}
	// both files named on the command line: the rename is whole
	if code, _, errOut := fmtRun(f["todos"], f["checks"]); code != 0 || errOut != "" {
		t.Fatalf("both: code %d err %q", code, errOut)
	}
	fmtCheckClean(t, dir)
}

// #310: `alder fmt` adds the `requires` clause to a route module's helpers that do not state
// their need (W0014, the corpus fixture), and the app then checks clean; a second run changes
// nothing.
func TestMigrate_RequiresClause(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../testdata/corpus/W0014-route-helper")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "expected.diag")); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := fmtRun(dir); code != 0 || errOut != "" || !strings.Contains(out, "Todos.alder") {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	todos, _ := os.ReadFile(filepath.Join(dir, "app", "Todos.alder"))
	for _, want := range []string{"recent : Select Todo requires user\n", "count : Query Int requires user\n", "both : Query Int requires user, org\n"} {
		if !strings.Contains(string(todos), want) {
			t.Errorf("Todos.alder lacks %q:\n%s", want, todos)
		}
	}
	if code, out, errOut := fmtRun(dir); code != 0 || out != "" || errOut != "" {
		t.Fatalf("second run: code %d out %q err %q", code, out, errOut)
	}
	fmtCheckClean(t, dir)
}

// #327: while a module has errors (here Todos, a missing import), `alder fmt` applies no rename
// that reaches other files: Checks keeps `Todos.Toggle.update`, which still checks, instead of
// naming a `patch` that Todos does not declare yet. It says so, naming the module. Once the
// import is fixed one run renames everything and a second changes nothing.
func TestMigrate_HandlerRenameHeldWhileModuleFails(t *testing.T) {
	t.Parallel()
	dir, f := fmtHandlerProject(t)
	good, _ := os.ReadFile(f["todos"])
	bad := strings.Replace(string(good), "= Task.succeed Response.refresh\n", "= Task.succeed (Missing.thing Response.refresh)\n", 1)
	if bad == string(good) {
		t.Fatal("fixture changed: no handler body to break")
	}
	fmtWrite(t, dir, "app/Todos.alder", bad)
	checks, _ := os.ReadFile(f["checks"])
	code, out, errOut := fmtRun(dir)
	want := "alder fmt: `Todos` has errors, so its migrations were not checked and the handler renames and `requires` clauses that reach other files (W0013, W0014) were held back; fix it and run again\n"
	if code != 0 || out != "" || errOut != want {
		t.Fatalf("code %d out %q err %q, want err %q", code, out, errOut, want)
	}
	if got, _ := os.ReadFile(f["checks"]); string(got) != string(checks) {
		t.Fatalf("Checks.alder changed while Todos has errors:\n%s", got)
	}
	if got, _ := os.ReadFile(f["todos"]); string(got) != bad {
		t.Fatalf("Todos.alder changed:\n%s", got)
	}
	fmtWrite(t, dir, "app/Todos.alder", string(good))
	if code, out, errOut := fmtRun(dir); code != 0 || errOut != "" || !strings.Contains(out, "Todos.alder") || !strings.Contains(out, "Checks.alder") {
		t.Fatalf("after the fix: code %d out %q err %q", code, out, errOut)
	}
	if code, out, errOut := fmtRun(dir); code != 0 || out != "" || errOut != "" {
		t.Fatalf("second run: code %d out %q err %q", code, out, errOut)
	}
	fmtCheckClean(t, dir)
}

// #327: a rename that stays in one file is still applied while an unrelated module (one that
// does not import the route's module) has errors; the run says that module's migrations wait.
func TestMigrate_SameFileRenameAppliesWhileOtherModuleFails(t *testing.T) {
	t.Parallel()
	dir, f := fmtHandlerProject(t)
	if err := os.Remove(f["checks"]); err != nil {
		t.Fatal(err)
	}
	fmtWrite(t, dir, "app/Broken.alder", "module Broken\n\nx : Int\nx = nope\n")
	code, out, errOut := fmtRun(dir)
	want := "alder fmt: `Broken` has errors, so its migrations were not checked and the handler renames and `requires` clauses that reach other files (W0013, W0014) were held back; fix it and run again\n"
	if code != 0 || out != f["todos"]+"\n" || errOut != want {
		t.Fatalf("code %d out %q err %q, want err %q", code, out, errOut, want)
	}
	if got, _ := os.ReadFile(f["todos"]); strings.Contains(string(got), "update") {
		t.Fatalf("Todos.alder not renamed:\n%s", got)
	}
}

// #327: a helper's `requires` clause (W0014) waits while a module it imports has errors: what
// the helper needs through that module is not known yet.
func TestMigrate_RequiresClauseHeldWhileImportFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../testdata/corpus/W0014-route-helper")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "expected.diag")); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "app", "Todos.alder")
	good, _ := os.ReadFile(p)
	src := strings.Replace(string(good), "import Table.Todo\n", "import Table.Todo\nimport Broken\n", 1)
	fmtWrite(t, dir, "app/Todos.alder", src)
	fmtWrite(t, dir, "app/Broken.alder", "module Broken\n\nx : Int\nx = nope\n")
	code, _, errOut := fmtRun(dir)
	if code != 0 || !strings.Contains(errOut, "`Broken` has errors") {
		t.Fatalf("code %d err %q", code, errOut)
	}
	if got, _ := os.ReadFile(p); strings.Contains(string(got), "requires user\n") {
		t.Fatalf("clause added while an import has errors:\n%s", got)
	}
}

// #327: when the route's module has only errors `alder fmt` fixes itself (an import qualifier),
// the first run fixes them and holds the rename that reaches Checks, saying to run again; the
// second run renames everything.
func TestMigrate_HandlerRenameHeldUntilImportsMigrated(t *testing.T) {
	t.Parallel()
	dir, f := fmtHandlerProject(t)
	fmtWrite(t, dir, "app/Table/Todo.alder", "module Table.Todo\n\nfind : Int -> Int\nfind n = n\n")
	good, _ := os.ReadFile(f["todos"])
	src := strings.Replace(string(good), "module Todos\n", "module Todos\n\nimport Table.Todo\n\nfirst : Int\nfirst = Table.Todo.find 1\n", 1)
	fmtWrite(t, dir, "app/Todos.alder", src)
	checks, _ := os.ReadFile(f["checks"])
	code, out, errOut := fmtRun(dir)
	want := "alder fmt: `Todos` has errors that `alder fmt` fixes, so the handler renames and `requires` clauses that reach other files (W0013, W0014) were held back; run `alder fmt` again\n"
	if code != 0 || out != f["todos"]+"\n" || errOut != want {
		t.Fatalf("code %d out %q err %q, want err %q", code, out, errOut, want)
	}
	if got, _ := os.ReadFile(f["checks"]); string(got) != string(checks) {
		t.Fatalf("Checks.alder changed on the first run:\n%s", got)
	}
	if code, out, errOut := fmtRun(dir); code != 0 || errOut != "" || !strings.Contains(out, "Todos.alder") || !strings.Contains(out, "Checks.alder") {
		t.Fatalf("second run: code %d out %q err %q", code, out, errOut)
	}
	if code, out, errOut := fmtRun(dir); code != 0 || out != "" || errOut != "" {
		t.Fatalf("third run: code %d out %q err %q", code, out, errOut)
	}
	fmtCheckClean(t, dir)
}
