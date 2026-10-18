package book

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/snippets"
	"alder/internal/testgocache"
)

// The verifier compiles every ```alder block of the book with the real `alder` binary, built
// once per run: a Module, Decls or Signatures block is `alder check`ed (type-checked) in a copy
// of examples/todos (a part=<example>/<Module> module, in a copy of examples/<example>); an Expr block is evaluated (`alder eval`) when its fence gives `eval=` or
// `error=`, and otherwise only parsed (`alder dump`).

var (
	tmpRoot   string // removed by TestMain
	buildOnce sync.Once
	alderBin  string
	buildErr  error
	// todosApp is examples/todos, absolute; exampleModules are its modules except App and the
	// generated admin's.
	todosApp, _    = filepath.Abs("../examples/todos")
	exampleModules []string
	// scaffoldApp is the app `alder new guestbook` wrote with the built binary (the example
	// ScaffoldExample), and newOutput what it printed.
	scaffoldApp string
	newOutput   string
	// bareApp is the app `alder new notes --bare` wrote (the state BareExample of chapter 15's
	// transcripts).
	bareApp string
)

// BareExample is the state name of chapter 15's transcripts on a bare app (`alder new notes
// --bare`): like ScaffoldExample, the built binary's output, not a directory under examples/.
const BareExample = "notes"

// ScaffoldExample is the example name under which chapter 2 quotes the scaffold `alder new`
// writes (```alder part=guestbook/Home, ```css file=guestbook/static/app.css): it is not a
// directory under examples/ but the output of the built binary, so the chapter and the
// generator cannot drift apart (TestBook_Scaffold).
const ScaffoldExample = "guestbook"

// exampleApp is the directory of the example app named ex (a directory under examples/),
// absolute.
func exampleApp(ex string) string {
	if ex == ScaffoldExample {
		return scaffoldApp
	}
	if ex == BareExample {
		return bareApp
	}
	if ex == DefaultExample {
		return todosApp
	}
	return filepath.Join(filepath.Dir(todosApp), ex)
}

func TestMain(m *testing.M) {
	// the go builds below go into a cache of their own, not the user's (internal/testgocache)
	release, err := testgocache.Setup()
	if err != nil {
		fmt.Fprintln(os.Stderr, "testgocache:", err)
		os.Exit(2)
	}
	dir, err := os.MkdirTemp("", "alder-book-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	tmpRoot = dir
	code := m.Run()
	os.RemoveAll(dir)
	release()
	os.Exit(code)
}

// alder is the path of the `alder` binary, built on first use.
func alder(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go tool is not on PATH")
	}
	buildOnce.Do(func() {
		alderBin = filepath.Join(tmpRoot, "alder")
		cmd := exec.Command("go", "build", "-o", alderBin, "./cmd/alder")
		cmd.Dir = ".."
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("building alder: %v\n%s", err, out)
			return
		}
		// the scaffold, as a user makes it: `alder new guestbook` in an empty directory
		work := filepath.Join(tmpRoot, "scaffold")
		if err := os.MkdirAll(work, 0o755); err != nil {
			buildErr = err
			return
		}
		cmd = exec.Command(alderBin, "new", ScaffoldExample)
		cmd.Dir = work
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("alder new %s: %v\n%s", ScaffoldExample, err, out)
			return
		}
		scaffoldApp, newOutput = filepath.Join(work, ScaffoldExample), string(out)
		cmd = exec.Command(alderBin, "new", BareExample, "--bare")
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("alder new %s --bare: %v\n%s", BareExample, err, out)
			return
		}
		bareApp = filepath.Join(work, BareExample)
		filepath.WalkDir(filepath.Join(todosApp, "app"), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".alder") {
				rel, _ := filepath.Rel(filepath.Join(todosApp, "app"), p)
				name := strings.ReplaceAll(strings.TrimSuffix(rel, ".alder"), string(filepath.Separator), ".")
				// the generated admin's modules (app/Admin/, chapter 15) are left out: their
				// messages (Create, Save, Remove) repeat from table to table, so one module that
				// imported them all would declare each twice, and no block uses them
				if name != "App" && name != "Admin" && !strings.HasPrefix(name, "Admin.") {
					exampleModules = append(exampleModules, name)
				}
			}
			return nil
		})
		sort.Strings(exampleModules)
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return alderBin
}

// report is what verifying one chapter found.
type report struct {
	failures  []string // "book/03-syntax.md:120: E0003 expected Int, got Text"
	fragments []string // "book/03-syntax.md:120"
	counts    map[string]int
}

// jdiag is a diagnostic as `alder check --json` and `alder eval --json` print it.
type jdiag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

// matches reports whether d is the diagnostic exp (an Error) asks for.
func (exp Expect) matches(d jdiag) bool {
	return d.Code == exp.Code && strings.Contains(d.Message, exp.Message)
}

// want describes what an Error expectation asks for, for a failure.
func (exp Expect) want() string {
	if exp.Message == "" {
		return exp.Code
	}
	return exp.Code + " " + strconv.Quote(exp.Message)
}

// verifier verifies the blocks of one chapter in its own temporary directory.
type verifier struct {
	t       *testing.T
	bin     string
	display string // the chapter's path as reported, e.g. book/03-syntax.md
	dir     string // this chapter's temporary directory
	evalDir string // the working directory of `alder eval`, shared by every chapter
	seq     int
	r       report
	// joined are the modules joined from part= blocks, with the book line of each source line.
	joined map[Snippet][]int
	// example is, for a module joined from part=<example>/<Module> blocks, its example app
	// (every other block is checked in DefaultExample).
	example map[Snippet]string
}

// exampleOf is the example app s is checked in.
func (v *verifier) exampleOf(s Snippet) string {
	if ex, ok := v.example[s]; ok {
		return ex
	}
	return DefaultExample
}

func (v *verifier) failf(line int, format string, a ...any) {
	v.r.failures = append(v.r.failures, fmt.Sprintf("%s:%d: %s", v.display, line, fmt.Sprintf(format, a...)))
}

// verify checks every ```alder block of the chapter at path, reported as display.
func verify(t *testing.T, path, display string) report {
	t.Helper()
	v := &verifier{t: t, bin: alder(t), display: display, r: report{counts: map[string]int{}}, joined: map[Snippet][]int{}, example: map[Snippet]string{}}
	v.evalDir = filepath.Join(tmpRoot, "eval")
	var err error
	if err = os.MkdirAll(v.evalDir, 0o755); err == nil {
		v.dir, err = os.MkdirTemp(tmpRoot, "chapter-")
	}
	if err != nil {
		t.Fatal(err)
	}
	snippets, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	var batch []Snippet
	parts := map[string][]Snippet{} // part= blocks by example/module
	var partOrder []string
	partExpect := map[string]Expect{}
	lastPart := ""
	moduleNames := map[string]int{}
	for _, s := range snippets {
		if Classify(s) == Module {
			moduleNames[ModuleName(s)]++
		}
	}
	for _, s := range snippets {
		exp, err := Expectation(s.Info)
		if err != nil {
			v.failf(s.Line, "%v", err)
			continue
		}
		kind := Classify(s)
		v.r.counts[kind.String()]++
		switch exp.Kind {
		case Part:
			v.r.counts["part"]++
			key := exp.Example + "/" + exp.Module
			if fi, err := os.Stat(filepath.Join(exampleApp(exp.Example), "app")); err != nil || !fi.IsDir() {
				v.failf(s.Line, "part=%s: there is no example app examples/%s", key, exp.Example)
				continue
			}
			if _, seen := parts[key]; !seen {
				partOrder = append(partOrder, key)
				partExpect[key] = exp
			} else if lastPart != key {
				v.failf(s.Line, "the part=%s blocks are not consecutive: a part= block of %s comes between", key, lastPart)
			}
			parts[key] = append(parts[key], s)
			lastPart = key
		case Written:
			v.r.counts["written"]++ // TestBook_Commands compares it with the file a command wrote
		case Fragment:
			v.r.counts["fragment"]++
			v.r.fragments = append(v.r.fragments, fmt.Sprintf("%s:%d", display, s.Line))
		case Eval:
			v.r.counts["eval"]++
			if kind != Expr {
				v.failf(s.Line, "eval= is for an expression; this block is %s", kind)
				continue
			}
			v.eval(s, exp)
		case Error:
			v.r.counts["error"]++
			if kind == Expr {
				v.eval(s, exp)
			} else {
				v.checkError(s, exp)
			}
		case Check:
			v.r.counts["checked"]++
			switch {
			case kind == Expr:
				v.r.counts["parsed"]++
				v.parseOnly(s)
			case kind == Module && moduleNames[ModuleName(s)] > 1, TableModule(s) != "":
				v.checkAlone(s)
			default:
				batch = append(batch, s)
			}
		}
	}
	batches := map[string][]Snippet{} // the joined modules of an example other than todos
	var others []string
	for _, key := range partOrder {
		exp := partExpect[key]
		m := exp.Module
		src, lines := JoinParts(parts[key])
		js := Snippet{File: path, Line: parts[key][0].Line, Info: "alder part=" + key, Source: src}
		if Classify(js) != Module || ModuleName(js) != m {
			v.failf(js.Line, "the first part=%s block must start with `module %s`", key, m)
			continue
		}
		v.joined[js] = lines
		v.example[js] = exp.Example
		if msg := sameAsExample(exp.Example, m, src); msg != "" {
			v.failf(js.Line, "%s", msg)
		}
		switch {
		case exp.Example == DefaultExample && moduleNames[m] > 1:
			v.checkAlone(js)
		case exp.Example == DefaultExample:
			batch = append(batch, js)
		default:
			if _, ok := batches[exp.Example]; !ok {
				others = append(others, exp.Example)
			}
			batches[exp.Example] = append(batches[exp.Example], js)
		}
	}
	v.checkBatch(DefaultExample, batch)
	for _, ex := range others {
		v.checkBatch(ex, batches[ex])
	}
	return v.r
}

// newApp copies examples/<ex>/app into a fresh directory and returns the app directory.
func (v *verifier) newApp(ex string) string {
	v.seq++
	dir := filepath.Join(v.dir, fmt.Sprintf("app%d", v.seq))
	src := filepath.Join(exampleApp(ex), "app")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		dst := filepath.Join(dir, "app", rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		v.t.Fatal(err)
	}
	return dir
}

var importRE = regexp.MustCompile(`(?m)^import ([A-Za-z0-9_.]+)`)

// exampleImports reports whether example module m imports module t, directly or through other
// example modules.
func exampleImports(m, t string) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(x string) bool {
		if seen[x] {
			return false
		}
		seen[x] = true
		src, err := os.ReadFile(filepath.Join(todosApp, "app", strings.ReplaceAll(x, ".", "/")+".alder"))
		if err != nil {
			return false
		}
		for _, im := range importRE.FindAllStringSubmatch(string(src), -1) {
			if im[1] == t || walk(im[1]) {
				return true
			}
		}
		return false
	}
	return walk(m)
}

// placed is a block written into an app: its file (relative to the app dir) and, for each line
// of the file, the book line it came from (0 for a line the verifier wrote). replaces is set
// when a one-table block took the place of the example's module of that name: the example's
// other modules may then miss what that module also declared, which is not the block's error.
type placed struct {
	s        Snippet
	file     string
	replaces bool
	lines    []int
}

// place writes s into the app at dir: a Module as its module's file; Decls as module Snippet<n>
// importing every example module (but App) it does not import itself; Signatures likewise, each
// `name : T` as `sigK : T` / `sigK = name`.
func (v *verifier) place(dir string, s Snippet, n int) placed {
	p := placed{s: s}
	var b strings.Builder
	add := func(text string, bookLine int) {
		b.WriteString(text + "\n")
		p.lines = append(p.lines, bookLine)
	}
	src := strings.Split(strings.TrimRight(s.Source, "\n"), "\n")
	kind := Classify(s)
	if kind == Module {
		p.file = filepath.Join("app", strings.ReplaceAll(ModuleName(s), ".", "/")+".alder")
		bookLines := v.joined[s] // nil unless s was joined from part= blocks
		for i, l := range src {
			if bookLines != nil {
				add(l, bookLines[i])
			} else {
				add(l, s.Line+1+i)
			}
		}
	} else {
		name := fmt.Sprintf("Snippet%d", n)
		own := map[string]bool{}
		for _, m := range importRE.FindAllStringSubmatch(s.Source, -1) {
			own[m[1]] = true
		}
		if t := TableModule(s); t != "" { // one table: its own module, replacing the example's
			name = t
			for _, m := range exampleModules {
				p.replaces = p.replaces || m == t
				if m == t || exampleImports(m, t) { // itself, or a module that would make a cycle
					own[m] = true
				}
			}
		}
		p.file = filepath.Join("app", strings.ReplaceAll(name, ".", "/")+".alder")
		add("module "+name, 0)
		for _, m := range exampleModules {
			if !own[m] {
				add("import "+m, 0)
			}
		}
		if kind == Signatures {
			sigs, _ := Sigs(s.Source)
			k := 0
			for _, sg := range sigs {
				if sg.Type == "" {
					continue
				}
				for _, nm := range sg.Names {
					k++
					add(fmt.Sprintf("sig%d : %s", k, sg.Type), s.Line+sg.Line)
					add(fmt.Sprintf("sig%d = %s", k, nm), s.Line+sg.Line)
				}
			}
		} else {
			for i, l := range src {
				add(l, s.Line+1+i)
			}
		}
	}
	full := filepath.Join(dir, p.file)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		v.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(b.String()), 0o644); err != nil {
		v.t.Fatal(err)
	}
	return p
}

// bookLine is the book line of line l of p's file, 0 for a line the verifier wrote.
func (p placed) bookLine(l int) int {
	if l < 1 || l > len(p.lines) {
		return 0
	}
	return p.lines[l-1]
}

// check runs `alder check --json` on the app at dir and returns its diagnostics, or a failure
// message when it did not run as a check (exit 2).
func (v *verifier) check(dir string) ([]jdiag, string) {
	cmd := exec.Command(v.bin, "check", dir, "--json")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
			return nil, fmt.Sprintf("alder check: %v: %s", err, strings.TrimSpace(errb.String()))
		}
	}
	var ds []jdiag
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var d jdiag
		if err := json.Unmarshal([]byte(l), &d); err != nil {
			return nil, fmt.Sprintf("alder check: bad JSON %q", l)
		}
		ds = append(ds, d)
	}
	return ds, ""
}

// failing are the diagnostics that make a block fail: errors, and warnings except an unused
// import the verifier inserted.
func failing(ds []jdiag, ps []placed) []jdiag {
	var out []jdiag
	for _, d := range ds {
		if d.Severity != "error" && d.Code == "W0001" {
			inserted := false
			for _, p := range ps {
				if d.File == filepath.ToSlash(p.file) && p.bookLine(d.Line) == 0 {
					inserted = true
				}
			}
			if inserted {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}

// where is the book line of a diagnostic of the app holding p, and the diagnostic as reported.
func (p placed) where(d jdiag) (int, string) {
	msg := d.Code + " " + d.Message
	if d.File != filepath.ToSlash(p.file) {
		return p.s.Line, fmt.Sprintf("%s (in %s:%d)", msg, d.File, d.Line)
	}
	if l := p.bookLine(d.Line); l > 0 {
		return l, msg
	}
	return p.s.Line, msg
}

// checkBatch checks the blocks in one app; when that is not clean, each alone, so each failure
// is pinned on its block.
func (v *verifier) checkBatch(ex string, batch []Snippet) {
	if len(batch) == 0 {
		return
	}
	dir := v.newApp(ex)
	var ps []placed
	for i, s := range batch {
		ps = append(ps, v.place(dir, s, i+1))
	}
	ds, bad := v.check(dir)
	if bad == "" && len(failing(ds, ps)) == 0 {
		return
	}
	for _, s := range batch {
		v.checkAlone(s)
	}
}

// checkAlone checks one Module or Decls block in its own app; it must be clean.
func (v *verifier) checkAlone(s Snippet) {
	dir := v.newApp(v.exampleOf(s))
	p := v.place(dir, s, 1)
	ds, bad := v.check(dir)
	if bad != "" {
		v.failf(s.Line, "%s", bad)
		return
	}
	// A signature table's lines are separate claims: report each; any other block, its first.
	f := failing(ds, []placed{p})
	if p.replaces { // only the block's own module is its claim
		var own []jdiag
		for _, d := range f {
			if d.File == filepath.ToSlash(p.file) {
				own = append(own, d)
			}
		}
		f = own
	}
	if Classify(s) != Signatures && len(f) > 1 {
		f = f[:1]
	}
	for _, d := range f {
		line, msg := p.where(d)
		v.failf(line, "%s", msg)
	}
}

// checkError checks one Module or Decls block, which must report exp's diagnostic in its own
// file (not in an example module it breaks).
func (v *verifier) checkError(s Snippet, exp Expect) {
	dir := v.newApp(DefaultExample)
	p := v.place(dir, s, 1)
	ds, bad := v.check(dir)
	if bad != "" {
		v.failf(s.Line, "%s", bad)
		return
	}
	var got []string
	for _, d := range ds {
		if d.File == filepath.ToSlash(p.file) && exp.matches(d) {
			return
		}
		if d.Severity == "error" || d.Code == exp.Code {
			got = append(got, d.Code+" "+d.Message+" ("+d.File+")")
		}
	}
	v.failf(s.Line, "want %s, got [%s]", exp.want(), strings.Join(got, "; "))
}

var textDiagRE = regexp.MustCompile(`^[^\s:]+:(\d+):\d+: \w+\[(\w+)\]: (.*)$`)

// parseOnly parses an Expr block: each expression in it (where `alder eval` would start an
// item, snippets.Starts) becomes `itN = …` in module Snippet, line for line, and `alder dump`
// must find no error.
func (v *verifier) parseOnly(s Snippet) {
	lines := strings.Split(strings.TrimRight(s.Source, "\n"), "\n")
	starts := snippets.Starts(lines) // eval's item rule
	var b strings.Builder
	b.WriteString("module Snippet\n")
	n := 0
	for i, l := range lines {
		if starts[i] {
			n++
			fmt.Fprintf(&b, "it%d = ", n)
		}
		b.WriteString(l + "\n")
	}
	v.seq++
	file := filepath.Join(v.dir, fmt.Sprintf("expr%d.alder", v.seq))
	if err := os.WriteFile(file, []byte(b.String()), 0o644); err != nil {
		v.t.Fatal(err)
	}
	cmd := exec.Command(v.bin, "dump", file)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	err := cmd.Run()
	if err == nil {
		return
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		first, _, _ := strings.Cut(errb.String(), "\n")
		if m := textDiagRE.FindStringSubmatch(first); m != nil {
			l, _ := strconv.Atoi(m[1])
			v.failf(s.Line+max(l-1, 1), "%s %s", m[2], m[3])
			return
		}
		v.failf(s.Line, "%s", first)
		return
	}
	v.failf(s.Line, "alder dump: %v: %s", err, strings.TrimSpace(errb.String()))
}

// eval runs an Expr block through `alder eval --json -`: for Eval its value must be exp.Value
// exactly; for Error a diagnostic must have exp.Code.
func (v *verifier) eval(s Snippet, exp Expect) {
	cmd := exec.Command(v.bin, "eval", todosApp, "--json", "-")
	cmd.Dir = v.evalDir
	cmd.Stdin = strings.NewReader(s.Source) // as written: eval continues an item on a closer
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Run() // the exit code is in the JSON's diagnostics and error
	var res struct {
		Value       string  `json:"value"`
		Type        string  `json:"type"`
		Diagnostics []jdiag `json:"diagnostics"`
		Error       string  `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		v.failf(s.Line, "alder eval: bad JSON %q: %s", out.String(), strings.TrimSpace(errb.String()))
		return
	}
	at := func(d jdiag) int {
		if d.File == "input" {
			return s.Line + d.Line
		}
		return s.Line
	}
	if exp.Kind == Error {
		var got []string
		for _, d := range res.Diagnostics {
			if d.File == "input" && exp.matches(d) {
				return
			}
			if d.Severity == "error" || d.Code == exp.Code {
				got = append(got, d.Code+" "+d.Message+" ("+d.File+")")
			}
		}
		v.failf(s.Line, "want %s, got [%s] %s", exp.want(), strings.Join(got, "; "), res.Error)
		return
	}
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			v.failf(at(d), "%s %s", d.Code, d.Message)
			return
		}
	}
	if res.Error != "" {
		v.failf(s.Line, "%s", res.Error)
		return
	}
	if res.Value != exp.Value {
		v.failf(s.Line, "eval= wants %s, got %s : %s", exp.Value, res.Value, res.Type)
	}
}

func TestBook_Classify(t *testing.T) {
	cases := []struct {
		src  string
		want Kind
	}{
		{"module Todo\n\nimport User\n", Module},
		{"-- a comment\n\nmodule Todos.Index\n", Module},
		{"greeting : Text\ngreeting = \"hello\"\n", Decls},
		{"add a b = a + b\n", Decls},
		{"Item : { todo: Todo } -> Html Msg\n", Decls},
		{"type Filter = All | Active\n", Decls},
		{"table Todo {\n  id: TodoId @id,\n}\n", Decls},
		{"msg Toggle { id: TodoId }\n", Decls},
		{"import Todo\n", Decls},
		{"login = Auth.Login\n", Decls},
		{"test \"flip alternates\" =\n  flip (flip Light) == Light\n", Decls},
		{"-- only a comment\n", Decls},
		{"-- Value with a signature\ngreeting : Text\n", Decls},
		{"1 + 2\n", Expr},
		{"add 1 2\nList.map (add 1) [1, 2, 3]\n", Expr},
		{"todos\n  |> List.length\n", Expr},
		{"x == y\n", Expr},
		{"\\x -> x + 1\n", Expr},
		{"<ul>{items}</ul>\n", Expr},
		{"let xs = [1, 2]\nList.length xs\n", Expr},
		{"Toggle p -> {\n  Task.succeed Rerender\n},\n", Expr},
		{"  module Indented\n", Expr},
		{"List.length : List a -> Int\nList.isEmpty  : List a -> Bool   -- comment\n", Signatures},
		{"Db          -- chapter 8\nLog.info    : Text -> Task Unit\n", Signatures},
		{"not : Bool -> Bool\nBool.toText : Bool -> Text\n", Signatures},
		{"Date.year, Date.month, Date.day : Date -> Int\n", Signatures},
		{"first : List a -> Maybe a\n", Decls},
		{"Text.NonEmpty     -- length > 0\nInt.Positive      -- > 0\n", Expr},
	}
	for _, c := range cases {
		if got := Classify(Snippet{Source: c.src}); got != c.want {
			t.Errorf("Classify(%q) = %s, want %s", c.src, got, c.want)
		}
	}
}

func TestBook_Expectations(t *testing.T) {
	cases := []struct {
		info string
		want Expect
		bad  bool
	}{
		{info: "alder", want: Expect{Kind: Check}},
		{info: "alder fragment", want: Expect{Kind: Fragment}},
		{info: "alder eval=3", want: Expect{Kind: Eval, Value: "3"}},
		{info: "alder eval=[2, 3, 4]", want: Expect{Kind: Eval, Value: "[2, 3, 4]"}},
		{info: `alder eval={ a: 1, b: "x = y" }`, want: Expect{Kind: Eval, Value: `{ a: 1, b: "x = y" }`}},
		{info: "alder error=E0304", want: Expect{Kind: Error, Code: "E0304"}},
		{info: "alder error=W0003", want: Expect{Kind: Error, Code: "W0003"}},
		{info: `alder error=E0003 "expected Int, got Text"`, want: Expect{Kind: Error, Code: "E0003", Message: "expected Int, got Text"}},
		{info: `alder error=E0003 "a \"quoted\" part"`, want: Expect{Kind: Error, Code: "E0003", Message: `a "quoted" part`}},
		{info: "alder error=E0003 expected", bad: true},
		{info: `alder error=E0003 ""`, bad: true},
		{info: "alder part=Todos.Index", want: Expect{Kind: Part, Module: "Todos.Index", Example: "todos"}},
		{info: "alder part=blog/Posts.Show", want: Expect{Kind: Part, Module: "Posts.Show", Example: "blog"}},
		{info: "alder part=todos/Todo", want: Expect{Kind: Part, Module: "Todo", Example: "todos"}},
		{info: "alder part=todos", bad: true},
		{info: "alder part=Blog/Post", bad: true},
		{info: "alder part=blog/", bad: true},
		{info: "alder part=blog/post", bad: true},
		{info: "alder part=a/b/Post", bad: true},
		{info: "alder written=guestbook/app/About.alder", want: Expect{Kind: Written, Example: "guestbook", Path: "app/About.alder"}},
		{info: "alder written=guestbook", bad: true},
		{info: "alder written=Guestbook/app/About.alder", bad: true},
		{info: "alder error=oops", bad: true},
		{info: "alder eval=", bad: true},
		{info: "alder frag", bad: true},
		{info: "alderish", bad: true},
		{info: "go", bad: true},
	}
	for _, c := range cases {
		got, err := Expectation(c.info)
		if c.bad {
			if err == nil {
				t.Errorf("Expectation(%q) = %+v, want an error", c.info, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("Expectation(%q) = %+v, %v; want %+v", c.info, got, err, c.want)
		}
	}
}

func TestBook_Extract(t *testing.T) {
	ss, err := Extract("testdata/fixture.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 14 {
		t.Fatalf("got %d snippets, want 14 (the go block is not one)", len(ss))
	}
	if ss[0].Line != 8 || !strings.HasPrefix(ss[0].Source, "module Theme\n") || ss[0].Info != "alder" {
		t.Errorf("first snippet: %+v", ss[0])
	}
	if ss[3].Info != "alder eval=[2, 3, 4]" || ss[3].Source != "List.map (\\x -> x + 1) [1, 2, 3]\n" {
		t.Errorf("fourth snippet: %+v", ss[3])
	}
	wantKinds := []Kind{Module, Decls, Expr, Expr, Expr, Decls, Signatures, Decls, Module, Decls, Signatures, Decls, Decls, Decls}
	for i, s := range ss {
		if k := Classify(s); k != wantKinds[i] {
			t.Errorf("snippet %d (line %d): %s, want %s", i, s.Line, k, wantKinds[i])
		}
	}
	// one table: checked as its module; two tables cannot be one module; a module block is itself
	for i, s := range ss {
		if want := map[int]string{13: "Note"}[i]; TableModule(s) != want {
			t.Errorf("TableModule(snippet %d) = %q, want %q", i, TableModule(s), want)
		}
	}
	two := Snippet{Source: "table A {\n  id: AId @id,\n}\n\ntable B {\n  id: BId @id,\n}\n"}
	if got := TableModule(two); got != "" {
		t.Errorf("TableModule of two tables = %q, want \"\"", got)
	}
}

// TestBook_FixtureFails proves the verifier fails: the fixture's wrong eval=, wrong signature,
// wrong declaration and wrong error= message are its only failures, each on its line, and
// every other kind of block (the part= pieces joined) passes.
func TestBook_FixtureFails(t *testing.T) {
	t.Parallel()
	r := verify(t, "testdata/fixture.md", "book/testdata/fixture.md")
	want := []string{
		"book/testdata/fixture.md:50: eval= wants 4, got 3 : Int",
		"book/testdata/fixture.md:101: E0003 expected Int -> Int, got Int -> Text",
		"book/testdata/fixture.md:108: E0003 expected Text, got Int",
		`book/testdata/fixture.md:113: want E0003 "expected Bool", got [E0003 expected Int, got Text (app/Snippet1.alder)]`,
	}
	got := append([]string(nil), r.failures...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("failures:\n%s\nwant exactly:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(r.fragments) != 1 || r.counts["eval"] != 2 || r.counts["error"] != 2 || r.counts["checked"] != 7 || r.counts["part"] != 2 {
		t.Errorf("fragments %v, counts %v", r.fragments, r.counts)
	}
}

// TestBook verifies every ```alder block of every chapter.
func TestBook(t *testing.T) {
	chapters, err := filepath.Glob("*.md")
	if err != nil || len(chapters) == 0 {
		t.Fatalf("no chapters: %v", err)
	}
	start := time.Now()
	var mu sync.Mutex
	total := report{counts: map[string]int{}}
	t.Cleanup(func() {
		sort.Slice(total.fragments, func(i, j int) bool { // by chapter, then line
			fi, li, _ := strings.Cut(total.fragments[i], ":")
			fj, lj, _ := strings.Cut(total.fragments[j], ":")
			ni, _ := strconv.Atoi(li)
			nj, _ := strconv.Atoi(lj)
			return fi < fj || fi == fj && ni < nj
		})
		t.Logf("%d blocks: checked %d (%d of them expressions, parsed only), part= %d, eval= %d, error= %d, written= %d (TestBook_Commands), fragment %d (module %d, decls %d, signatures %d, expr %d) in %v",
			total.counts["module"]+total.counts["decls"]+total.counts["signatures"]+total.counts["expr"],
			total.counts["checked"], total.counts["parsed"], total.counts["part"], total.counts["eval"], total.counts["error"], total.counts["written"], total.counts["fragment"],
			total.counts["module"], total.counts["decls"], total.counts["signatures"], total.counts["expr"],
			time.Since(start).Round(time.Millisecond))
		for _, f := range total.fragments {
			t.Logf("fragment: %s", f)
		}
	})
	for _, ch := range chapters {
		t.Run(ch, func(t *testing.T) {
			t.Parallel()
			r := verify(t, ch, "book/"+ch)
			for _, f := range r.failures {
				t.Error(f)
			}
			mu.Lock()
			defer mu.Unlock()
			for k, n := range r.counts {
				total.counts[k] += n
			}
			total.fragments = append(total.fragments, r.fragments...)
		})
	}
}

// TestBook_Scaffold (RF4): chapter 2 quotes every file `alder new guestbook` writes, each as
// it is (scaffoldDrift), chapters 1 and 11 quote what `alder new guestbook` prints, and the scaffold
// checks clean and passes its tests.
func TestBook_Scaffold(t *testing.T) {
	bin := alder(t)
	for _, msg := range scaffoldDrift("02-tour.md", "book/02-tour.md") {
		t.Error(msg)
	}
	for _, ch := range []string{"01-why.md", "11-tooling.md"} {
		b, err := os.ReadFile(ch)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "$ alder new guestbook\n"+newOutput+"```") {
			t.Errorf("book/%s does not quote what `alder new guestbook` prints, which is:\n%s", ch, newOutput)
		}
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"check"}, ""},
		{[]string{"test"}, "ok   Home \"signing adds an entry\"\nok   Home \"entries are listed newest first\"\n2 passed, 0 failed\n"},
	} {
		cmd := exec.Command(bin, c.args...)
		cmd.Dir = scaffoldApp
		out, err := cmd.CombinedOutput()
		if err != nil || string(out) != c.want {
			t.Errorf("alder %s in the scaffold: %v\n%s", strings.Join(c.args, " "), err, out)
		}
	}
}

// scaffoldDrift compares chapter (reported as display) with the scaffold at scaffoldApp: every
// file of it but go.mod (written only inside another Go module) must be quoted, a module by
// its part=guestbook/<Module> blocks (joined, the same line for line but blank lines) and any
// other file by one file=guestbook/<path> block (the same byte for byte); a quoted file the
// scaffold does not have fails too. It returns a message per difference.
func scaffoldDrift(chapter, display string) []string {
	var msgs []string
	files := map[string]bool{}
	err := filepath.WalkDir(scaffoldApp, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(scaffoldApp, p)
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir() && rel == "build":
			return filepath.SkipDir
		case !d.IsDir() && rel != "go.mod":
			files[rel] = false
		}
		return nil
	})
	if err != nil {
		return []string{err.Error()}
	}
	snips, err := Extract(chapter)
	if err != nil {
		return []string{err.Error()}
	}
	parts := map[string][]Snippet{}
	var order []string
	for _, s := range snips {
		exp, err := Expectation(s.Info)
		if err != nil || exp.Kind != Part || exp.Example != ScaffoldExample {
			continue
		}
		if _, ok := parts[exp.Module]; !ok {
			order = append(order, exp.Module)
		}
		parts[exp.Module] = append(parts[exp.Module], s)
	}
	for _, m := range order {
		rel := "app/" + strings.ReplaceAll(m, ".", "/") + ".alder"
		src, _ := JoinParts(parts[m])
		if msg := sameAsExample(ScaffoldExample, m, src); msg != "" {
			msgs = append(msgs, fmt.Sprintf("%s:%d: %s", display, parts[m][0].Line, msg))
		}
		files[rel] = true // quoted (rightly or not: a difference is reported once)
	}
	blocks, err := ExtractFiles(chapter)
	if err != nil {
		return append(msgs, err.Error())
	}
	for _, b := range blocks {
		if b.Example != ScaffoldExample {
			continue
		}
		quoted, ok := files[b.Path]
		switch {
		case !ok:
			msgs = append(msgs, fmt.Sprintf("%s:%d: file=%s/%s: alder new writes no such file", display, b.Line, b.Example, b.Path))
			continue
		case quoted:
			msgs = append(msgs, fmt.Sprintf("%s:%d: file=%s/%s is quoted twice", display, b.Line, b.Example, b.Path))
			continue
		}
		want, err := os.ReadFile(filepath.Join(scaffoldApp, filepath.FromSlash(b.Path)))
		if err != nil {
			return append(msgs, err.Error())
		}
		if b.Source != string(want) {
			msgs = append(msgs, fmt.Sprintf("%s:%d: file=%s/%s differs from what alder new writes (internal/scaffold/files/%s, or the migration generator):\n%s", display, b.Line, b.Example, b.Path, b.Path, firstDiff(b.Source, string(want))))
		}
		files[b.Path] = true
	}
	var missing []string
	for f, quoted := range files {
		if !quoted {
			missing = append(missing, f)
		}
	}
	sort.Strings(missing)
	for _, f := range missing {
		how := "a ```<lang> file=" + ScaffoldExample + "/" + f + " block"
		if strings.HasSuffix(f, ".alder") {
			how = "```alder part=" + ScaffoldExample + "/" + strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(f, "app/"), ".alder"), "/", ".") + " blocks"
		}
		msgs = append(msgs, fmt.Sprintf("%s: alder new writes %s, which the chapter does not quote as it is: add %s", display, f, how))
	}
	return msgs
}

// firstDiff names the first line where got and want differ.
func firstDiff(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < max(len(g), len(w)); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return fmt.Sprintf("  line %d: the book has %q, the file %q", i+1, gl, wl)
		}
	}
	return "  (the same lines)"
}

// TestBook_ScaffoldDrift proves scaffoldDrift fails: a chapter that quotes every scaffold file
// passes; one changed line of a file= block, one changed line of a module and one file left out
// are each one failure.
func TestBook_ScaffoldDrift(t *testing.T) {
	alder(t)
	var full strings.Builder
	err := filepath.WalkDir(scaffoldApp, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && d.Name() == "build" {
				return filepath.SkipDir
			}
			return err
		}
		rel, _ := filepath.Rel(scaffoldApp, p)
		rel = filepath.ToSlash(rel)
		if rel == "go.mod" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if m, ok := strings.CutSuffix(strings.TrimPrefix(rel, "app/"), ".alder"); ok {
			fmt.Fprintf(&full, "```alder part=guestbook/%s\n%s```\n\n", strings.ReplaceAll(m, "/", "."), b)
		} else {
			fmt.Fprintf(&full, "```text file=guestbook/%s\n%s```\n\n", rel, b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	chapter := func(md string) []string {
		p := filepath.Join(t.TempDir(), "ch.md")
		if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		return scaffoldDrift(p, "ch.md")
	}
	if msgs := chapter(full.String()); len(msgs) != 0 {
		t.Fatalf("every file quoted: %v", msgs)
	}
	md := full.String()
	for _, c := range []struct{ name, old, new, want string }{
		{"a file= line", "gap: 1rem;", "gap: 2rem;", "file=guestbook/static/app.css differs"},
		{"a module line", "<h1>Guestbook</h1>", "<h1>Visitors</h1>", "the part=guestbook/Home blocks joined differ"},
		{"a file left out", "```text file=guestbook/adapters/README.md", "```text", "alder new writes adapters/README.md, which the chapter does not quote"},
	} {
		if !strings.Contains(md, c.old) {
			t.Fatalf("%s: the chapter lacks %q", c.name, c.old)
		}
		msgs := chapter(strings.Replace(md, c.old, c.new, 1))
		if len(msgs) != 1 || !strings.Contains(msgs[0], c.want) {
			t.Errorf("%s: %v", c.name, msgs)
		}
	}
}

// sameAsExample compares the module m joined from part= blocks (src) with example's
// module of that name, ignoring blank lines: a module the book shows piece by piece must be
// the example's, line for line, so the two cannot drift apart. It returns "" when they agree
// or the to-do example has no module m (the tour's pieces may be a module of their own), else a
// message naming the first line that differs. Another example must have the module: a part=
// block names it to quote it, so a misspelt name fails.
func sameAsExample(example, m, src string) string {
	file := filepath.Join(exampleApp(example), "app", filepath.FromSlash(strings.ReplaceAll(m, ".", "/"))+".alder")
	b, err := os.ReadFile(file)
	if err != nil {
		if example == DefaultExample {
			return ""
		}
		return fmt.Sprintf("the part=%s/%s blocks name a module the example does not have: there is no %s", example, m, file)
	}
	nonBlank := func(s string) []string {
		var out []string
		for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
			if strings.TrimSpace(l) != "" {
				out = append(out, l)
			}
		}
		return out
	}
	book, ex := nonBlank(src), nonBlank(string(b))
	for i := 0; i < max(len(book), len(ex)); i++ {
		var bl, el string
		if i < len(book) {
			bl = book[i]
		}
		if i < len(ex) {
			el = ex[i]
		}
		if bl != el {
			label := m
			if example != DefaultExample {
				label = example + "/" + m
			}
			return fmt.Sprintf("the part=%s blocks joined differ from %s at its non-blank line %d: the book has %q, the file %q (update the book's pieces or the example so they match)", label, file, i+1, bl, el)
		}
	}
	return ""
}

func TestSameAsExample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(todosApp, "app", "Todo.alder"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if msg := sameAsExample("todos", "Todo", "\n\n"+strings.ReplaceAll(src, "\n", "\n\n")); msg != "" {
		t.Errorf("blank lines aside the same, got %q", msg)
	}
	if msg := sameAsExample("todos", "Todo", strings.Replace(src, "countOpen", "countAll", 1)); !strings.Contains(msg, "differ from") {
		t.Errorf("a changed line: got %q", msg)
	}
	if msg := sameAsExample("todos", "Todo", src+"\nextra = 1\n"); !strings.Contains(msg, `"extra = 1"`) {
		t.Errorf("an extra line: got %q", msg)
	}
	if msg := sameAsExample("todos", "Extra", "module Extra\n"); msg != "" {
		t.Errorf("a module the example lacks: got %q", msg)
	}
}

func TestSameAsExample_OtherExample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(exampleApp("blog"), "app", "Comment.alder"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if msg := sameAsExample("blog", "Comment", src); msg != "" {
		t.Errorf("the blog's own Comment: got %q", msg)
	}
	if msg := sameAsExample("blog", "Comment", strings.Replace(src, "@cascade", "@restrict", 1)); !strings.Contains(msg, "part=blog/Comment") || !strings.Contains(msg, filepath.Join("blog", "app", "Comment.alder")) {
		t.Errorf("a changed line: got %q", msg)
	}
	// todos has no Comment: the same source is not compared with anything there.
	if msg := sameAsExample("todos", "Comment", "module Comment\n"); msg != "" {
		t.Errorf("todos has no Comment: got %q", msg)
	}
	// a module of another example must exist there: a typo'd name fails
	if msg := sameAsExample("blog", "Commnet", "module Commnet\n"); !strings.Contains(msg, "part=blog/Commnet") || !strings.Contains(msg, filepath.Join("blog", "app", "Commnet.alder")) {
		t.Errorf("a module the blog lacks: got %q", msg)
	}
}

// TestBook_PartOfAnotherExample: a part=blog/<Module> walk is joined, compared with
// examples/blog's file and checked in a copy of examples/blog (where Post exists); a drifted
// piece fails on the walk's first line, even when it still checks; an example that does not
// exist fails.
func TestBook_PartOfAnotherExample(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join(exampleApp("blog"), "app", "Comment.alder"))
	if err != nil {
		t.Fatal(err)
	}
	head, rest, ok := strings.Cut(string(b), "type CommentId")
	if !ok {
		t.Fatal("examples/blog/app/Comment.alder has no `type CommentId`")
	}
	rest = "type CommentId" + rest
	chapter := func(name, rest string) string {
		md := "# A walk\n\n```alder part=blog/Comment\n" + head + "```\n\nProse.\n\n```alder part=blog/Comment\n" + rest + "```\n"
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	r := verify(t, chapter("same.md", rest), "same.md")
	if len(r.failures) != 0 || r.counts["part"] != 2 {
		t.Errorf("the blog's own Comment in two pieces: failures %v, counts %v", r.failures, r.counts)
	}
	r = verify(t, chapter("drift.md", strings.Replace(rest, "@cascade", "@restrict", 1)), "drift.md")
	if len(r.failures) != 1 || !strings.HasPrefix(r.failures[0], "drift.md:3: the part=blog/Comment blocks joined differ") || !strings.Contains(r.failures[0], `"  post: Post.PostId @ref Post @restrict,"`) {
		t.Errorf("a drifted piece: failures %v", r.failures)
	}
	p := filepath.Join(t.TempDir(), "nope.md")
	if err := os.WriteFile(p, []byte("```alder part=nope/Comment\nmodule Comment\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r = verify(t, p, "nope.md")
	if len(r.failures) != 1 || !strings.Contains(r.failures[0], "there is no example app examples/nope") {
		t.Errorf("an example that does not exist: failures %v", r.failures)
	}
}

// TestBook_Commands runs the transcripts of the book (chapter 15's): every ```… command=<state>
// block runs its `alder` command, with the block's standard input, in that state's app (a copy
// of the scaffold, or of an example, made at the chapter's first block of that state and changed
// by each command after it, in order), and what the command prints must be the block's text
// (MatchOutput); every written=<state>/<path> block must be that file of the app as the commands
// before it left it, byte for byte. So the chapter cannot show a command, an output or a written
// file the generators do not produce.
func TestBook_Commands(t *testing.T) {
	bin := alder(t)
	chapters, err := filepath.Glob("*.md")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ch := range chapters {
		msgs, k := runCommands(t, bin, ch, "book/"+ch)
		for _, m := range msgs {
			t.Error(m)
		}
		n += k
	}
	if n == 0 {
		t.Error("no chapter has a command= block")
	}
	t.Logf("%d command= and written= blocks", n)
}

// runCommands runs the command= and written= blocks of chapter (reported as display) and returns
// a message per difference, and the number of blocks.
func runCommands(t *testing.T, bin, chapter, display string) ([]string, int) {
	t.Helper()
	blocks, err := ExtractCommands(chapter)
	if err != nil {
		return []string{err.Error()}, 0
	}
	var msgs []string
	states := map[string]string{} // state -> its app's directory
	for _, b := range blocks {
		dir, ok := states[b.State]
		if !ok {
			src := exampleApp(b.State)
			if fi, err := os.Stat(filepath.Join(src, "app")); err != nil || !fi.IsDir() {
				msgs = append(msgs, fmt.Sprintf("%s:%d: %s is not an app: a state is guestbook (the scaffold), notes (a bare app) or an example under examples/", display, b.Line, b.State))
				continue
			}
			dir = filepath.Join(t.TempDir(), b.State)
			if err := copyApp(src, dir); err != nil {
				t.Fatal(err)
			}
			states[b.State] = dir
		}
		if b.Path != "" {
			got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(b.Path)))
			switch {
			case err != nil:
				msgs = append(msgs, fmt.Sprintf("%s:%d: written=%s/%s: the commands before it wrote no such file", display, b.Line, b.State, b.Path))
			case string(got) != b.Source:
				msgs = append(msgs, fmt.Sprintf("%s:%d: written=%s/%s differs from the file the commands before it left:\n%s", display, b.Line, b.State, b.Path, firstDiff(b.Source, string(got))))
			}
			continue
		}
		args, stdin, want, err := parseCommand(b.Source)
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("%s:%d: %v", display, b.Line, err))
			continue
		}
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(stdin)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			if _, exit := err.(*exec.ExitError); !exit {
				t.Fatal(err)
			}
		}
		if got := stdout.String() + stderr.String(); !MatchOutput(want, got) {
			msgs = append(msgs, fmt.Sprintf("%s:%d: `alder %s` printed what the block does not show:\n%s\nit printed:\n%s", display, b.Line, strings.Join(args, " "), firstDiff(want, got), got))
		}
	}
	return msgs, len(blocks)
}

// copyApp copies the app at src to dst, leaving out build/ and the SQLite files; an executable
// file stays executable.
func copyApp(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			if rel == "build" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.Contains(d.Name(), ".db") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info, err := d.Info(); err == nil && info.Mode()&0o111 != 0 {
			mode = 0o755 // a script (scripts/deploy-local.sh) stays executable
		}
		return os.WriteFile(filepath.Join(dst, rel), b, mode)
	})
}

// parseCommand splits a command block's source into the arguments after `$ alder`, its
// standard input (a here-document: the first line ends in `<<'EOF'`, and the lines up to `EOF`
// are the input) and the output it shows.
func parseCommand(src string) (args []string, stdin, want string, err error) {
	lines := strings.SplitAfter(src, "\n")
	first := strings.TrimSuffix(lines[0], "\n")
	rest, ok := strings.CutPrefix(first, "$ alder ")
	if !ok {
		return nil, "", "", fmt.Errorf("a command= block starts with `$ alder …`, not %q", first)
	}
	args, err = shellWords(rest)
	if err != nil {
		return nil, "", "", err
	}
	lines = lines[1:]
	if n := len(args); n > 0 && strings.HasPrefix(args[n-1], "<<") {
		delim := strings.Trim(strings.TrimPrefix(args[n-1], "<<"), "'\"")
		args = args[:n-1]
		var in strings.Builder
		for {
			if len(lines) == 0 {
				return nil, "", "", fmt.Errorf("the here-document has no %s line", delim)
			}
			l := lines[0]
			lines = lines[1:]
			if strings.TrimSuffix(l, "\n") == delim {
				break
			}
			in.WriteString(l)
		}
		stdin = in.String()
	}
	return args, stdin, strings.Join(lines, ""), nil
}

// shellWords splits s as a shell does a simple command: words separated by spaces, '…' taken
// as it is, "…" with \" and \\ escapes, and a backslash outside quotes escaping the next byte.
func shellWords(s string) ([]string, error) {
	var words []string
	var w strings.Builder
	in := false // inside a word
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ':
			if in {
				words = append(words, w.String())
				w.Reset()
				in = false
			}
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("an unterminated ' in %q", s)
			}
			w.WriteString(s[i+1 : i+1+j])
			i += j + 1
			in = true
		case c == '"':
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
					i++
				}
				w.WriteByte(s[i])
			}
			if i == len(s) {
				return nil, fmt.Errorf("an unterminated \" in %q", s)
			}
			in = true
		case c == '\\' && i+1 < len(s):
			i++
			w.WriteByte(s[i])
			in = true
		default:
			w.WriteByte(c)
			in = true
		}
	}
	if in {
		words = append(words, w.String())
	}
	return words, nil
}

// TestBook_CommandParts: the pieces of a command block, and the `…` of MatchOutput.
func TestBook_CommandParts(t *testing.T) {
	args, stdin, want, err := parseCommand("$ alder fill 'Home.\"a test\"' - --hole \"?x y\" <<'EOF'\n{ a: 1 }\nEOF\nedited app/Home.alder\n")
	if err != nil || !slices.Equal(args, []string{"fill", `Home."a test"`, "-", "--hole", "?x y"}) || stdin != "{ a: 1 }\n" || want != "edited app/Home.alder\n" {
		t.Errorf("parseCommand: %q %q %q %v", args, stdin, want, err)
	}
	for _, bad := range []string{"alder check\n", "$ go test\n", "$ alder fill x - <<'EOF'\n1\n", "$ alder new page 'About\n"} {
		if _, _, _, err := parseCommand(bad); err == nil {
			t.Errorf("parseCommand(%q): no error", bad)
		}
	}
	for _, c := range []struct {
		want, got string
		ok        bool
	}{
		{"a\nb\n", "a\nb\n", true},
		{"a\nb\n", "a\nc\n", false},
		{"a\n…\nd\n", "a\nb\nc\nd\n", true},
		{"a\n…\nd\n", "a\nd\n", true},
		{"a\n…\nd\n", "a\nb\nc\n", false},
		{"…\nz\n", "x\ny\nz\n", true},
		{"", "", true},
		{"", "x\n", false},
		{"artifact build/todos/deploy/… (…)\n", "artifact build/todos/deploy/d2bd2ae9c535b108 (darwin/arm64)\n", true},
		{"deployed … to local\n", "deployed d2bd to local\n", true},
		{"deployed … to local\n", "deployed d2bd to production\n", false},
		{"a…b…c\n", "abc\n", true},
		{"a…bc…c\n", "abc\n", false},
	} {
		if MatchOutput(c.want, c.got) != c.ok {
			t.Errorf("MatchOutput(%q, %q) = %v", c.want, c.got, !c.ok)
		}
	}
}

// TestBook_CommandsFail proves runCommands fails: a command with its output and the file it
// wrote passes, and a wrong output line, a wrong line of a written file and a state that is no
// app are a failure each.
func TestBook_CommandsFail(t *testing.T) {
	bin := alder(t)
	about := "module About\n\nimport Layout\n\npage \"/about\" { }\n\ntype Model = { }\n\n" +
		"load : Params -> Session -> Query Model\nload params session = Query.succeed { }\n\n" +
		"view : Model -> Session -> Html Msg\nview model session =\n  <Layout.Page title=\"About\">\n    <h1>About</h1>\n  </Layout.Page>\n\n" +
		"test \"the page is shown\" = {\n  let r <- Test.open (About {})\n" +
		"  Task.succeed ({ answer: r.answer, heading: Test.text \"h1\" r.doc } == { answer: Shown, heading: Just \"About\" })\n}\n"
	newPage := "```text command=guestbook\n$ alder new page About /about\nwrote app/About.alder\n```\n\n"
	ch := filepath.Join(t.TempDir(), "15-x.md")
	for _, c := range []struct {
		name, chapter string
		fails         int
	}{
		{"right", newPage + "```alder written=guestbook/app/About.alder\n" + about + "```\n\n```text command=guestbook\n$ alder check\n```\n", 0},
		{"wrong output", "```text command=guestbook\n$ alder new page About /about\nwrote app/Home.alder\n```\n", 1},
		{"wrong file", newPage + "```alder written=guestbook/app/About.alder\n" + strings.Replace(about, "<h1>About</h1>", "<h1>Us</h1>", 1) + "```\n", 1},
		{"no such state", "```text command=nowhere\n$ alder check\n```\n", 1},
	} {
		if err := os.WriteFile(ch, []byte(c.chapter), 0o644); err != nil {
			t.Fatal(err)
		}
		if msgs, _ := runCommands(t, bin, ch, c.name); len(msgs) != c.fails {
			t.Errorf("%s: %d failures, want %d: %q", c.name, len(msgs), c.fails, msgs)
		}
	}
}
