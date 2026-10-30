// Package grammar holds the tests that keep tree-sitter-alder in step with the Go parser. They
// shell out to the tree-sitter CLI, and fail without it unless ALDER_SKIP_TS=1: a drift test
// that skips silently stops guarding anything.
package grammar

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"alder/book"
	"alder/internal/scratch"
	"alder/internal/syntax"
)

// pinnedCLI is the tree-sitter version src/ is generated with (scripts/ts.sh, CI).
// The same version is pinned in scripts/ts.sh (want) and .github/install-tree-sitter.sh;
// change all three together.
const pinnedCLI = "0.27.1"

// minSources guards the drift test against a silently shrinking corpus.
const minSources = 1000

func tsCLI(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("tree-sitter")
	if err != nil {
		if os.Getenv("ALDER_SKIP_TS") == "1" {
			t.Skip("ALDER_SKIP_TS=1: the tree-sitter grammar tests are skipped")
		}
		t.Fatal("the tree-sitter CLI (>= 0.26.1) is required: brew install tree-sitter-cli, or set ALDER_SKIP_TS=1 to skip the grammar tests")
	}
	return p
}

// libDir holds the parser library built by parserLib; TestMain removes it.
var libDir string

func TestMain(m *testing.M) {
	code := m.Run()
	if libDir != "" {
		os.RemoveAll(libDir)
	}
	os.Exit(code)
}

var (
	libOnce sync.Once
	libPath string
	libErr  error
)

// parserLib compiles the grammar once per test run and returns the library's path. Pass it to
// `tree-sitter query` and `tree-sitter parse` with libArgs: their -p flag implies --rebuild,
// which recompiles the large parser on every call.
func parserLib(t *testing.T, cli string) string {
	t.Helper()
	libOnce.Do(func() {
		libDir, libErr = os.MkdirTemp("", "tree-sitter-alder-")
		if libErr != nil {
			return
		}
		dir, err := filepath.Abs(".")
		if err != nil {
			libErr = err
			return
		}
		so := filepath.Join(libDir, "alder.so")
		cmd := exec.Command(cli, "build", "-o", so)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			libErr = fmt.Errorf("tree-sitter build: %v\n%s", err, out)
			return
		}
		libPath = so
	})
	if libErr != nil {
		t.Fatal(libErr)
	}
	return libPath
}

// libArgs are the flags that make `tree-sitter query` and `tree-sitter parse` use parserLib.
func libArgs(t *testing.T, cli string) []string {
	t.Helper()
	return []string{"-l", parserLib(t, cli), "--lang-name", "alder"}
}

type decl struct{ Kind, Name string }

type source struct {
	Path    string // the file the CLI reads
	Label   string // where it came from, for messages
	Imports []decl
	Decls   []decl
}

func goDecls(m *syntax.Module) (imports, decls []decl) {
	for _, im := range m.Imports {
		imports = append(imports, decl{"import", im.Module})
	}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			decls = append(decls, decl{"value", d.Name})
		case *syntax.TypeDecl:
			decls = append(decls, decl{"type", d.Name})
		case *syntax.PageDecl:
			decls = append(decls, decl{"page", d.Path})
		case *syntax.MsgDecl:
			decls = append(decls, decl{"msg", d.Name})
		case *syntax.FormDecl:
			decls = append(decls, decl{"form", d.Name})
		case *syntax.TableDecl:
			decls = append(decls, decl{"table", d.Name})
		case *syntax.CapDecl:
			decls = append(decls, decl{"capability", d.Name})
		case *syntax.FakeDecl:
			decls = append(decls, decl{"fake", d.Name})
		case *syntax.SessionDecl:
			decls = append(decls, decl{"session", "session"})
		case *syntax.DeployDecl:
			decls = append(decls, decl{"deploy", "deploy"})
		case *syntax.ThemeDecl:
			decls = append(decls, decl{"theme", "theme"})
		case *syntax.SiloDecl:
			decls = append(decls, decl{"silo", d.Key})
		case *syntax.ChooseDecl:
			decls = append(decls, decl{"choose", d.Key})
		case *syntax.LoginDecl:
			decls = append(decls, decl{"login", d.Page})
		case *syntax.TestDecl:
			decls = append(decls, decl{"test", d.Name})
		default:
			decls = append(decls, decl{fmt.Sprintf("%T", d), "?"})
		}
	}
	return imports, decls
}

// sources is every .alder file under examples/, stdlib/ and testdata/, and every book snippet
// that is a whole module, that the Go parser parses without a diagnostic. stdlib/ is read the
// way the compiler reads it, with ParseSignatures. Snippets are written into dir.
func sources(t *testing.T, dir string) []source {
	t.Helper()
	var out []source
	for _, root := range []string{"../examples", "../stdlib", "../testdata"} {
		sig := root == "../stdlib"
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".alder") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			parse := syntax.ParseFile
			if sig {
				parse = syntax.ParseSignatures
			}
			m, diags := parse(path, src)
			if len(diags) > 0 || m == nil {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			im, ds := goDecls(m)
			out = append(out, source{Path: abs, Label: path, Imports: im, Decls: ds})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	chapters, _ := filepath.Glob("../book/*.md")
	for _, ch := range chapters {
		snips, err := book.Extract(ch)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range snips {
			if f := strings.Fields(s.Info); len(f) == 0 || f[0] != "alder" || book.Classify(s) != book.Module {
				continue
			}
			label := fmt.Sprintf("%s:%d", ch, s.Line)
			m, diags := syntax.ParseFile(label, []byte(s.Source))
			if len(diags) > 0 || m == nil {
				continue
			}
			path := filepath.Join(dir, fmt.Sprintf("%s-%d.alder", strings.TrimSuffix(filepath.Base(ch), ".md"), s.Line))
			if err := os.WriteFile(path, []byte(s.Source), 0o644); err != nil {
				t.Fatal(err)
			}
			im, ds := goDecls(m)
			out = append(out, source{Path: path, Label: label, Imports: im, Decls: ds})
		}
	}
	return out
}

func writePaths(t *testing.T, dir string, srcs []source) string {
	t.Helper()
	var b strings.Builder
	for _, s := range srcs {
		b.WriteString(s.Path + "\n")
	}
	p := filepath.Join(dir, "paths.txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func grammarDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// captureRE matches one line of `tree-sitter query` output:
//
//	capture: 0 - value, start: (3, 0), end: (3, 6), text: `update`
//
// A capture that spans lines is printed without its index or text; drift.scm captures only a
// watch that way (as `watchnode`), for its end row:
//
//	capture: watchnode, start: (2, 0), end: (3, 14)
var captureRE = regexp.MustCompile("^\\s+capture: (?:\\d+ - )?([a-z]+), start: \\((\\d+), (\\d+)\\), end: \\((\\d+), \\d+\\)(?:, text: `(.*)`)?$")

type capture struct {
	row, col int
	endRow   int // set on a watch capture: the last row of its watch
	d        decl
}

// queryDecls runs test/drift.scm over the files and returns each file's captures in source order.
func queryDecls(t *testing.T, cli, paths string) map[string][]decl {
	t.Helper()
	res := map[string][]decl{}
	for f, cs := range queryCaptures(t, cli, paths) {
		for _, c := range cs {
			res[f] = append(res[f], c.d)
		}
	}
	return res
}

// queryCaptures runs test/drift.scm over the files and returns each file's captures, with
// their positions, in source order.
func queryCaptures(t *testing.T, cli, paths string) map[string][]capture {
	t.Helper()
	args := append([]string{"query"}, libArgs(t, cli)...)
	args = append(args, filepath.Join(grammarDir(t), "test", "drift.scm"), "--paths", paths)
	cmd := exec.Command(cli, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("tree-sitter query: %v\n%s", err, stderr.String())
	}
	caps := map[string][]capture{}
	type at struct {
		file string
		row  int
	}
	watchEnds := map[at]int{} // a watch's file and first row → its last row
	file := ""
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line != "" && !strings.HasPrefix(line, " ") {
			file = line
			continue
		}
		m := captureRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		row, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		end, _ := strconv.Atoi(m[4])
		name := m[5]
		switch m[1] {
		case "watchnode":
			watchEnds[at{file, row}] = end
			continue
		case "page", "test":
			name = strings.TrimSuffix(strings.TrimPrefix(name, `"`), `"`)
		}
		caps[file] = append(caps[file], capture{row: row, col: col, d: decl{m[1], name}})
	}
	for file, cs := range caps {
		for i := range cs {
			if cs[i].d.Kind == "watch" {
				cs[i].endRow = watchEnds[at{file, cs[i].row}]
			}
		}
		sort.SliceStable(cs, func(i, j int) bool {
			if cs[i].row != cs[j].row {
				return cs[i].row < cs[j].row
			}
			return cs[i].col < cs[j].col
		})
	}
	return caps
}

func split(ds []decl) (imports, rest []decl) {
	for _, d := range ds {
		if d.Kind == "import" {
			imports = append(imports, d)
		} else {
			rest = append(rest, d)
		}
	}
	return imports, rest
}

func equal(a, b []decl) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDrift: every file the Go parser accepts parses with no ERROR or MISSING node, and its
// top-level declarations (kind and name, in order) and imports are the Go parser's.
func TestDrift(t *testing.T) {
	cli := tsCLI(t)
	dir := t.TempDir()
	srcs := sources(t, dir)
	if len(srcs) < minSources {
		t.Fatalf("only %d sources parse clean with the Go parser; expected at least %d", len(srcs), minSources)
	}
	paths := writePaths(t, dir, srcs)

	args := append([]string{"parse", "-q"}, libArgs(t, cli)...)
	cmd := exec.Command(cli, append(args, "--paths", paths)...)
	out, err := cmd.Output()
	if err != nil || len(bytes.TrimSpace(out)) > 0 {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 20 {
			lines = append(lines[:20], fmt.Sprintf("… and %d more", len(lines)-20))
		}
		t.Fatalf("%d sources; tree-sitter reports errors (%v):\n%s", len(srcs), err, strings.Join(lines, "\n"))
	}

	got := queryDecls(t, cli, paths)
	var bad []string
	for _, s := range srcs {
		gi, gd := split(got[s.Path])
		if !equal(gi, s.Imports) || !equal(gd, s.Decls) {
			bad = append(bad, fmt.Sprintf("%s\n  go:          %v %v\n  tree-sitter: %v %v", s.Label, s.Imports, s.Decls, gi, gd))
		}
	}
	if len(bad) > 0 {
		n := len(bad)
		if n > 20 {
			bad = bad[:20]
		}
		t.Fatalf("%d of %d sources disagree on top-level declarations:\n%s", n, len(srcs), strings.Join(bad, "\n"))
	}
	t.Logf("%d sources agree", len(srcs))
}

// TestDriftScratch: every *scratch.alder in the repo that scratch.ParseFile accepts with no
// diagnostic parses with no ERROR or MISSING node, has one watch per File.Watches starting and
// ending on the same lines, and has the Go parser's top-level declarations and imports (read from
// File.Source as a module, less the watches' generated `itK =` values).
func TestDriftScratch(t *testing.T) {
	cli := tsCLI(t)
	type scratchSource struct {
		source
		watchLines [][2]int // each watch's first and last line, 1-based
	}
	var srcs []scratchSource
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n != ".." && (strings.HasPrefix(n, ".") || n == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "scratch.alder") {
			return nil
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, diags := scratch.ParseFile(path, string(text))
		if len(diags) > 0 {
			return nil
		}
		m, diags := syntax.ParseFile(path, []byte("module Scratch\n"+f.Source))
		if len(diags) > 0 || m == nil {
			t.Fatalf("%s: scratch.ParseFile accepts it but its module source does not parse: %v", path, diags)
		}
		watchNames := map[string]bool{}
		var lines [][2]int
		for _, w := range f.Watches {
			watchNames[w.Name] = true
			lines = append(lines, [2]int{w.Line, w.End})
		}
		im, ds := goDecls(m)
		var kept []decl
		for _, d := range ds {
			if !(d.Kind == "value" && watchNames[d.Name]) {
				kept = append(kept, d)
			}
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		srcs = append(srcs, scratchSource{source{Path: abs, Label: path, Imports: im, Decls: kept}, lines})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"internal/scaffold/files/scratch.alder", "tree-sitter-alder/test/drift/watches.scratch.alder"} {
		found := false
		for _, s := range srcs {
			found = found || filepath.ToSlash(s.Label) == "../"+want
		}
		if !found {
			t.Errorf("%s is not among the scratch files tested: it is missing or does not parse clean", want)
		}
	}
	dir := t.TempDir()
	plain := make([]source, len(srcs))
	for i, s := range srcs {
		plain[i] = s.source
	}
	paths := writePaths(t, dir, plain)

	args := append([]string{"parse", "-q"}, libArgs(t, cli)...)
	out, err := exec.Command(cli, append(args, "--paths", paths)...).Output()
	if err != nil || len(bytes.TrimSpace(out)) > 0 {
		t.Fatalf("tree-sitter reports errors in scratch files (%v):\n%s", err, out)
	}

	got := queryCaptures(t, cli, paths)
	for _, s := range srcs {
		var ds []decl
		var lines [][2]int
		for _, c := range got[s.Path] {
			if c.d.Kind == "watch" {
				lines = append(lines, [2]int{c.row + 1, c.endRow + 1})
			} else {
				ds = append(ds, c.d)
			}
		}
		gi, gd := split(ds)
		if !equal(gi, s.Imports) || !equal(gd, s.Decls) {
			t.Errorf("%s: top-level declarations disagree\n  go:          %v %v\n  tree-sitter: %v %v", s.Label, s.Imports, s.Decls, gi, gd)
		}
		if fmt.Sprint(lines) != fmt.Sprint(s.watchLines) {
			t.Errorf("%s: watch lines [first last] disagree\n  scratch:     %v\n  tree-sitter: %v", s.Label, s.watchLines, lines)
		}
	}
	t.Logf("%d scratch files agree", len(srcs))
}

// TestRecovery: code being typed (an unterminated string, an unclosed element, a dangling
// let) leaves the next column-1 declaration parsed as a declaration, whole: the capture must
// start on the declaration's first line, so a value_decl that lost its signature (and starts
// at its binding) does not count.
func TestRecovery(t *testing.T) {
	cli := tsCLI(t)
	dir := t.TempDir()
	after := capture{d: decl{"value", "after"}}
	at := func(c capture, row int) capture { c.row = row; return c }
	cases := map[string]struct {
		src  string
		want capture // the declaration's capture and the 0-based row of its first line
	}{
		"string.alder":  {"module M\n\na = \"unterminated\n\nafter : Int\nafter = 1\n", at(after, 4)},
		"element.alder": {"module M\n\nview m =\n  <div>\n    <p>hello\n\nafter : Int\nafter = 1\n", at(after, 6)},
		"let.alder":     {"module M\n\nb = {\n  let x =\n}\n\nafter : Int\nafter = 1\n", at(after, 6)},
		// An element left open before a column-1 line ends there (scanner.c scan_html_text).
		"element-binding.alder":  {"module M\n\nview m =\n  <div>\n    <p>hello\n\nafter = 1\n", at(after, 6)},
		"element-adjacent.alder": {"module M\n\nview m =\n  <div>\n    <p>hello\nafter : Int\nafter = 1\n", at(after, 5)},
		"element-type.alder":     {"module M\n\nview m =\n  <div>\n    <p>hello\n\ntype T = A | B\n", at(capture{d: decl{"type", "T"}}, 6)},
	}
	var paths []string
	for name, c := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(c.src), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	list := filepath.Join(dir, "paths.txt")
	if err := os.WriteFile(list, []byte(strings.Join(paths, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := queryCaptures(t, cli, list)
	for _, p := range paths {
		want := cases[filepath.Base(p)].want
		found := false
		for _, c := range got[p] {
			if c.row == want.row && c.d == want.d {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the declaration after the broken one (%v on line %d) is lost or incomplete; captures: %v",
				filepath.Base(p), want.d, want.row+1, got[p])
		}
	}
}

// TestIncrementalFirstWord edits the first word, which decides the root's branch: `module`
// makes a module file and `modul` a scratch file, where `> 2` is a watch rather than the right
// side of `x = 1 > 2`. Each reparse after an edit (tree-sitter parse --edits reuses the old
// tree, as an editor does) must give the tree a fresh parse of the edited text gives.
func TestIncrementalFirstWord(t *testing.T) {
	cli := tsCLI(t)
	dir := t.TempDir()
	const module = "module M\n\nx = 1\n> 2\n"
	steps := []struct {
		edits []string // "offset deleted inserted", applied in order to module
		text  string   // the text after them
	}{
		{[]string{"5 1 "}, "modul M\n\nx = 1\n> 2\n"},
		{[]string{"5 1 ", "5 0 e"}, module},
	}
	parse := func(path string, edits ...string) string {
		args := append(append([]string{"parse"}, libArgs(t, cli)...), path)
		if len(edits) > 0 {
			args = append(append(args, "--edits"), edits...)
		}
		out, _ := exec.Command(cli, args...).Output()     // exit 1 when the tree has an ERROR
		tree, _, _ := strings.Cut(string(out), "\n"+path) // drop the summary line --edits or an error adds
		if !strings.HasPrefix(tree, "(source_file") {
			t.Fatalf("tree-sitter %v: no tree in\n%s", args, out)
		}
		return strings.TrimSpace(tree)
	}
	src := filepath.Join(dir, "first-word.alder")
	if err := os.WriteFile(src, []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	for i, st := range steps {
		fresh := filepath.Join(dir, fmt.Sprintf("fresh-%d.alder", i))
		if err := os.WriteFile(fresh, []byte(st.text), 0o644); err != nil {
			t.Fatal(err)
		}
		want := parse(fresh)
		if got := parse(src, st.edits...); got != want {
			t.Errorf("after edits %q: the incremental tree differs from a fresh parse of %q\n  incremental:\n%s\n  fresh:\n%s", st.edits, st.text, got, want)
		}
	}
	if !strings.Contains(parse(src, steps[0].edits...), "(watch") {
		t.Errorf("`modul M` does not make a scratch file with a watch")
	}
}

// TestCorpus runs the tree-sitter corpus and highlight tests (tree-sitter test).
func TestCorpus(t *testing.T) {
	cli := tsCLI(t)
	cmd := exec.Command(cli, "test")
	cmd.Dir = grammarDir(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tree-sitter test: %v\n%s", err, out)
	}
}

// TestGenerated regenerates the parser in a copy and compares it with the committed src/, so
// a grammar change without its regenerated parser fails.
func TestGenerated(t *testing.T) {
	cli := tsCLI(t)
	v, err := exec.Command(cli, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(v), pinnedCLI) {
		t.Skipf("tree-sitter %s generates differently from the pinned %s; CI checks regeneration", strings.TrimSpace(string(v)), pinnedCLI)
	}
	dir := t.TempDir()
	for _, f := range []string{"grammar.js", "tree-sitter.json", "src/scanner.c"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(cli, "generate")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TREE_SITTER_JS_RUNTIME=native")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tree-sitter generate: %v\n%s", err, out)
	}
	for _, f := range []string{"src/parser.c", "src/grammar.json", "src/node-types.json"} {
		want, _ := os.ReadFile(f)
		got, _ := os.ReadFile(filepath.Join(dir, f))
		if !bytes.Equal(want, got) {
			t.Errorf("%s is stale: run scripts/ts.sh generate and commit tree-sitter-alder/src", f)
		}
	}
}

// TestTSScript_RefusesOtherVersion: scripts/ts.sh generate refuses a CLI other than the pinned one.
func TestTSScript_RefusesOtherVersion(t *testing.T) {
	bin := t.TempDir()
	shim := "#!/bin/sh\necho 'tree-sitter 0.99.0'\n"
	if err := os.WriteFile(filepath.Join(bin, "tree-sitter"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "../scripts/ts.sh", "generate")
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("ts.sh generate ran with tree-sitter 0.99.0:\n%s", out)
	}
	if !strings.Contains(string(out), "must be generated with "+pinnedCLI) {
		t.Fatalf("ts.sh generate refusal does not name the pinned version:\n%s", out)
	}
}

// TestQueriesCompile runs every query file over a real module, so a query naming a node or
// field the grammar does not have fails here rather than in the editor. Neovim-only predicates
// and directives are not evaluated by the CLI, which is fine: the plugin tests load the
// queries in Neovim (alder.nvim tests/filetype_spec.lua).
func TestQueriesCompile(t *testing.T) {
	cli := tsCLI(t)
	queries, _ := filepath.Glob("queries/neovim/*.scm")
	want := map[string]bool{"highlights": true, "locals": true, "indents": true, "folds": true, "injections": true, "textobjects": true}
	for _, q := range queries {
		delete(want, strings.TrimSuffix(filepath.Base(q), ".scm"))
	}
	if len(want) > 0 {
		t.Fatalf("missing query files in queries/neovim: %v", want)
	}
	queries = append(queries, "queries/highlights.scm")
	sample, err := filepath.Abs("../examples/blog/app/App.alder")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range queries {
		args := append([]string{"query", "-q"}, libArgs(t, cli)...)
		cmd := exec.Command(cli, append(args, q, sample)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", q, err, out)
		}
	}
}
