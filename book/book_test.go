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
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/snippets"
)

// The verifier compiles every ```alder block of the book with the real `alder` binary, built
// once per run: a Module, Decls or Signatures block is `alder check`ed (type-checked) in a copy
// of examples/todos; an Expr block is evaluated (`alder eval`) when its fence gives `eval=` or
// `error=`, and otherwise only parsed (`alder dump`).

var (
	tmpRoot   string // removed by TestMain
	buildOnce sync.Once
	alderBin  string
	buildErr  error
	// todosApp is examples/todos, absolute; exampleModules are its modules except App.
	todosApp, _    = filepath.Abs("../examples/todos")
	exampleModules []string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "alder-book-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	tmpRoot = dir
	code := m.Run()
	os.RemoveAll(dir)
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
		filepath.WalkDir(filepath.Join(todosApp, "app"), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".alder") {
				rel, _ := filepath.Rel(filepath.Join(todosApp, "app"), p)
				name := strings.ReplaceAll(strings.TrimSuffix(rel, ".alder"), string(filepath.Separator), ".")
				if name != "App" {
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
}

func (v *verifier) failf(line int, format string, a ...any) {
	v.r.failures = append(v.r.failures, fmt.Sprintf("%s:%d: %s", v.display, line, fmt.Sprintf(format, a...)))
}

// verify checks every ```alder block of the chapter at path, reported as display.
func verify(t *testing.T, path, display string) report {
	t.Helper()
	v := &verifier{t: t, bin: alder(t), display: display, r: report{counts: map[string]int{}}, joined: map[Snippet][]int{}}
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
	parts := map[string][]Snippet{} // part= blocks by module
	var partOrder []string
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
			if _, seen := parts[exp.Module]; !seen {
				partOrder = append(partOrder, exp.Module)
			} else if lastPart != exp.Module {
				v.failf(s.Line, "the part=%s blocks are not consecutive: a part= block of %s comes between", exp.Module, lastPart)
			}
			parts[exp.Module] = append(parts[exp.Module], s)
			lastPart = exp.Module
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
			case kind == Module && moduleNames[ModuleName(s)] > 1:
				v.checkAlone(s)
			default:
				batch = append(batch, s)
			}
		}
	}
	for _, m := range partOrder {
		src, lines := JoinParts(parts[m])
		js := Snippet{File: path, Line: parts[m][0].Line, Info: "alder part=" + m, Source: src}
		if Classify(js) != Module || ModuleName(js) != m {
			v.failf(js.Line, "the first part=%s block must start with `module %s`", m, m)
			continue
		}
		v.joined[js] = lines
		if moduleNames[m] > 1 {
			v.checkAlone(js)
		} else {
			batch = append(batch, js)
		}
	}
	v.checkBatch(batch)
	return v.r
}

// newApp copies examples/todos/app into a fresh directory and returns the app directory.
func (v *verifier) newApp() string {
	v.seq++
	dir := filepath.Join(v.dir, fmt.Sprintf("app%d", v.seq))
	src := filepath.Join(todosApp, "app")
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

// placed is a block written into an app: its file (relative to the app dir) and, for each line
// of the file, the book line it came from (0 for a line the verifier wrote).
type placed struct {
	s     Snippet
	file  string
	lines []int
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
		p.file = filepath.Join("app", name+".alder")
		own := map[string]bool{}
		for _, m := range importRE.FindAllStringSubmatch(s.Source, -1) {
			own[m[1]] = true
		}
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
func (v *verifier) checkBatch(batch []Snippet) {
	if len(batch) == 0 {
		return
	}
	dir := v.newApp()
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
	dir := v.newApp()
	p := v.place(dir, s, 1)
	ds, bad := v.check(dir)
	if bad != "" {
		v.failf(s.Line, "%s", bad)
		return
	}
	// A signature table's lines are separate claims: report each; any other block, its first.
	f := failing(ds, []placed{p})
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
	dir := v.newApp()
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
		{info: "alder part=Todos.Index", want: Expect{Kind: Part, Module: "Todos.Index"}},
		{info: "alder part=todos", bad: true},
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
	if len(ss) != 13 {
		t.Fatalf("got %d snippets, want 13 (the go block is not one)", len(ss))
	}
	if ss[0].Line != 8 || !strings.HasPrefix(ss[0].Source, "module Theme\n") || ss[0].Info != "alder" {
		t.Errorf("first snippet: %+v", ss[0])
	}
	if ss[3].Info != "alder eval=[2, 3, 4]" || ss[3].Source != "List.map (\\x -> x + 1) [1, 2, 3]\n" {
		t.Errorf("fourth snippet: %+v", ss[3])
	}
	wantKinds := []Kind{Module, Decls, Expr, Expr, Expr, Decls, Signatures, Decls, Module, Decls, Signatures, Decls, Decls}
	for i, s := range ss {
		if k := Classify(s); k != wantKinds[i] {
			t.Errorf("snippet %d (line %d): %s, want %s", i, s.Line, k, wantKinds[i])
		}
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
	if len(r.fragments) != 1 || r.counts["eval"] != 2 || r.counts["error"] != 2 || r.counts["checked"] != 6 || r.counts["part"] != 2 {
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
		t.Logf("%d blocks: checked %d (%d of them expressions, parsed only), part= %d, eval= %d, error= %d, fragment %d (module %d, decls %d, signatures %d, expr %d) in %v",
			total.counts["module"]+total.counts["decls"]+total.counts["signatures"]+total.counts["expr"],
			total.counts["checked"], total.counts["parsed"], total.counts["part"], total.counts["eval"], total.counts["error"], total.counts["fragment"],
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
