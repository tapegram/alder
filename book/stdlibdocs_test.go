package book

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/doc"
	"alder/internal/syntax"
	"alder/stdlib"
)

// The stdlib doc tests hold every stdlib signature file to the doc format (the spec's "Tests
// that enforce it") and run every doc example in one batched `alder eval`.

// pendingDocs are the modules not yet documented; TestStdlibDocs_Complete skips them. Each
// documenting task removes its modules; the last deletes the list.
var pendingDocs = map[string]bool{
	"Date": true, "Db": true, "File": true,
	"Fuzz": true, "Html": true, "Http": true, "Json": true,
	"Log": true, "Notice": true, "Prelude": true, "Query": true,
	"Random": true, "Response": true, "Session": true, "Task": true,
	"Test": true, "Time": true, "Url": true,
}

// noExample are the values that cannot have an example, each with the reason. Keys are
// qualified names ("Test.click").
var noExample = map[string]string{}

// stdlibDecl is one documented thing of a stdlib file: the module itself or a declaration.
type stdlibDecl struct {
	File   string // "stdlib/List.alder"
	Line   int    // the line of the `module` line or the declaration
	Name   string // "List", "List.take"
	Doc    string // as syntax.DocOf returns it
	Module bool
	Type   bool
}

// docStart is the line of the doc's first line (the declaration's line when it has no doc).
func (d stdlibDecl) docStart() int {
	if d.Doc == "" {
		return d.Line
	}
	return d.Line - strings.Count(d.Doc, "\n") - 1
}

// stdlibDecls reads every *.alder file at the root of fsys, in name order, as `stdlib/<name>`.
func stdlibDecls(fsys fs.FS) ([]stdlibDecl, error) {
	names, err := fs.Glob(fsys, "*.alder")
	if err != nil {
		return nil, err
	}
	var out []stdlibDecl
	for _, n := range names {
		src, err := fs.ReadFile(fsys, n)
		if err != nil {
			return nil, err
		}
		file := "stdlib/" + n
		m, diags := syntax.ParseSignatures(file, src)
		if len(diags) > 0 {
			return nil, fmt.Errorf("%s: %v", file, diags)
		}
		lines := strings.Split(string(src), "\n")
		line := m.Span.Start.Line
		out = append(out, stdlibDecl{File: file, Line: line, Name: m.Name, Doc: syntax.DocOf(lines, line), Module: true})
		for _, d := range m.Decls {
			var name string
			isType := false
			switch d := d.(type) {
			case *syntax.TypeDecl:
				name, isType = d.Name, true
			case *syntax.ValueDecl:
				name = d.FullName()
			default:
				continue
			}
			line := d.GetSpan().Start.Line
			out = append(out, stdlibDecl{File: file, Line: line, Name: m.Name + "." + name, Doc: syntax.DocOf(lines, line), Type: isType})
		}
	}
	return out, nil
}

// stdlibDocProblems lists findings as "stdlib/List.alder:7: List.take: no example".
func stdlibDocProblems(fsys fs.FS, pending map[string]bool, noExample map[string]string, headingExists func(ref string) bool) []string {
	decls, err := stdlibDecls(fsys)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	named := map[string]bool{}
	module := ""
	for _, d := range decls {
		named[d.Name] = true
		if d.Module {
			module = d.Name
		}
		if pending[module] {
			continue
		}
		need := doc.Strict{}
		switch {
		case d.Module:
			need.Book = true
		case !d.Type:
			_, waived := noExample[d.Name]
			need.Example = !waived
		}
		for _, p := range doc.Problems(d.Doc, need) {
			out = append(out, fmt.Sprintf("%s:%d: %s: %s", d.File, d.Line, d.Name, p))
		}
		ref := doc.Parse(d.Doc).Book
		if _, _, _, ok := doc.ParseBook(ref); ok && !headingExists(ref) {
			out = append(out, fmt.Sprintf("%s:%d: %s: Book: %s: no such heading in the book", d.File, d.Line, d.Name, ref))
		}
	}
	var stale []string
	for name := range noExample {
		if !named[name] {
			stale = append(stale, fmt.Sprintf("noExample: %s: no such declaration", name))
		}
	}
	sort.Strings(stale)
	return append(out, stale...)
}

// bookHeadingExists: "chapter 9" is 09-*.md, "appendix B" is B-*.md; the heading is a
// heading line's exact text after its #s. Lines in fenced code blocks are not headings.
func bookHeadingExists(ref string) bool {
	kind, number, heading, ok := doc.ParseBook(ref)
	if !ok {
		return false
	}
	if kind == "chapter" && len(number) == 1 {
		number = "0" + number
	}
	pattern := number + "-*.md"
	files, _ := filepath.Glob(pattern)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		fence := ""
		for _, l := range strings.Split(string(data), "\n") {
			if fence != "" {
				if strings.HasPrefix(l, fence) && strings.Trim(l, "`") == "" {
					fence = ""
				}
				continue
			}
			if strings.HasPrefix(l, "```") {
				fence = l[:len(l)-len(strings.TrimLeft(l, "`"))]
				continue
			}
			hashes := strings.TrimLeft(l, "#")
			if len(hashes) < len(l) && strings.HasPrefix(hashes, " ") && strings.TrimSpace(hashes) == heading {
				return true
			}
		}
	}
	return false
}

type docExample struct{ Where, Source string } // Where: "stdlib/List.alder:14"

// stdlibExamples are the examples of every doc of fsys (modules and declarations, pending ones
// too), in file order, each at its first line.
func stdlibExamples(fsys fs.FS) []docExample {
	decls, err := stdlibDecls(fsys)
	if err != nil {
		return []docExample{{Where: "stdlib", Source: err.Error()}}
	}
	var out []docExample
	for _, d := range decls {
		start := d.docStart()
		for _, e := range doc.Parse(d.Doc).Examples {
			out = append(out, docExample{Where: fmt.Sprintf("%s:%d", d.File, start+e.Line), Source: e.Source})
		}
	}
	return out
}

// equality is a checked example's two sides as written: the source of `left == right`.
type equality struct{ left, right string }

// checkedEquality is ex's sides when its root is `==`.
func checkedEquality(src string) (equality, bool) {
	e, diags, more, _ := syntax.ParseExpr("example", []byte(src))
	b, ok := e.(*syntax.BinOp)
	if !ok || b.Op != "==" || len(diags) > 0 || more != "" {
		return equality{}, false
	}
	return equality{spanText(src, b.L.GetSpan()), spanText(src, b.R.GetSpan())}, true
}

// spanText is the text of src that sp covers (1-based lines, 1-based byte columns, End
// exclusive).
func spanText(src string, sp diag.Span) string {
	lines := strings.Split(src, "\n")
	offset := func(p diag.Pos) int {
		n := 0
		for _, l := range lines[:p.Line-1] {
			n += len(l) + 1
		}
		return n + p.Col - 1
	}
	return src[offset(sp.Start):offset(sp.End)]
}

// evalBatch is one `alder eval` input of lets, one per source, then a last expression.
type evalBatch struct {
	text  strings.Builder
	lines int
	spans [][2]int // the input lines (1-based, inclusive) of each let
}

// let adds `let name =` with src's lines below it, indented four spaces.
func (b *evalBatch) let(name, src string) {
	first := b.lines + 1
	fmt.Fprintf(&b.text, "let %s =\n", name)
	b.lines++
	for _, l := range strings.Split(src, "\n") {
		b.text.WriteString("    " + l + "\n")
		b.lines++
	}
	b.spans = append(b.spans, [2]int{first, b.lines})
}

// owner is the index of the let holding input line l, or -1.
func (b *evalBatch) owner(l int) int {
	for i, s := range b.spans {
		if s[0] <= l && l <= s[1] {
			return i
		}
	}
	return -1
}

type evalResult struct {
	Value       string  `json:"value"`
	Type        string  `json:"type"`
	Diagnostics []jdiag `json:"diagnostics"`
	Error       string  `json:"error"`
}

// run evaluates the batch with last as its last line, against the example app
// (exampleApp(DefaultExample)), as the book's eval= blocks are.
func (b *evalBatch) run(t *testing.T, last string) (evalResult, error) {
	t.Helper()
	dir := filepath.Join(tmpRoot, "eval")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(alder(t), "eval", exampleApp(DefaultExample), "--json", "-")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(b.text.String() + last + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Run() // the exit code is in the JSON's diagnostics and error
	var res evalResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		return res, fmt.Errorf("alder eval: bad JSON %q: %s", out.String(), strings.TrimSpace(errb.String()))
	}
	return res, nil
}

// maxBatchRuns caps the batch runs of runExamples: the checker reports resolve errors (E0002)
// without type errors (E0003), so a batch can need a run for each, then one to evaluate.
const maxBatchRuns = 4

// runExamples checks every example in one batch and returns the failures. An example that does
// not parse is reported at its own line and left out (in a batch, the parser would blame the
// next example). Every other example is a `let` (so it is type-checked); the checked
// equalities are the list the input evaluates. A diagnostic is reported against the example
// holding its line, and those examples are dropped for another run, until a run drops none, so
// a false example is still reported beside a misspelt name and an ill-typed one. The left
// operands of the false ones are then evaluated together, to show what they gave: at most
// maxBatchRuns+1 runs, whatever the number of examples.
func runExamples(t *testing.T, exs []docExample) []string {
	t.Helper()
	var fails []string
	live := make([]bool, len(exs))
	for i, ex := range exs {
		live[i] = true
		if p := parseProblem(ex.Source); p != "" {
			fails = append(fails, fmt.Sprintf("%s: %s: %s", ex.Where, firstLine(ex.Source), p))
			live[i] = false
		}
	}
	var results []bool // the value of each checked example, in order
	var checked []int  // indexes into exs of the checked examples
	for attempt := 1; ; attempt++ {
		var b evalBatch
		var idx []int // let k of b is exs[idx[k]]
		var names []string
		checked = nil
		for i, ex := range exs {
			if !live[i] {
				continue
			}
			idx = append(idx, i)
			name := fmt.Sprintf("ex%d", i+1)
			b.let(name, ex.Source)
			if _, ok := checkedEquality(ex.Source); ok {
				names = append(names, name)
				checked = append(checked, i)
			}
		}
		if len(idx) == 0 {
			return fails
		}
		last := "0"
		if len(names) > 0 {
			last = "[" + strings.Join(names, ", ") + "]"
		}
		res, err := b.run(t, last)
		if err != nil {
			return append(fails, err.Error())
		}
		if hasErrors(res) {
			dropped := false
			for _, d := range res.Diagnostics {
				if d.Severity != "error" {
					continue
				}
				k := -1
				if d.File == "input" {
					k = b.owner(d.Line)
				}
				if k < 0 {
					fails = append(fails, fmt.Sprintf("examples: %s %s (%s:%d)", d.Code, d.Message, d.File, d.Line))
					continue
				}
				ex := exs[idx[k]]
				fails = append(fails, fmt.Sprintf("%s: %s: %s %s", ex.Where, firstLine(ex.Source), d.Code, d.Message))
				dropped = dropped || live[idx[k]]
				live[idx[k]] = false
			}
			if dropped && attempt < maxBatchRuns {
				continue // again without them
			}
			return fails
		}
		if res.Error != "" {
			return append(fails, "examples: "+res.Error)
		}
		if len(checked) == 0 {
			return fails
		}
		if results, err = parseBools(res.Value, len(checked)); err != nil {
			return append(fails, err.Error())
		}
		break
	}
	var b evalBatch
	var falseOnes []int
	var fields []string
	for k, ok := range results {
		if ok {
			continue
		}
		i := checked[k]
		eq, _ := checkedEquality(exs[i].Source)
		name := fmt.Sprintf("got%d", i+1)
		b.let(name, eq.left)
		falseOnes = append(falseOnes, i)
		fields = append(fields, name+": "+name)
	}
	if len(falseOnes) == 0 {
		return fails
	}
	res, err := b.run(t, "{ "+strings.Join(fields, ", ")+" }")
	got := map[int]string{}
	if err == nil && !hasErrors(res) && res.Error == "" {
		got = recordFields(res.Value, falseOnes)
	}
	for _, i := range falseOnes {
		eq, _ := checkedEquality(exs[i].Source)
		v, ok := got[i]
		if !ok {
			v = "(could not evaluate the left side)"
		}
		fails = append(fails, fmt.Sprintf("%s: %s: want %s, got %s", exs[i].Where, firstLine(exs[i].Source), eq.right, v))
	}
	return fails
}

// parseProblem is why src does not parse as one expression, or "".
func parseProblem(src string) string {
	_, diags, more, _ := syntax.ParseExpr("example", []byte(src))
	if len(diags) > 0 {
		return diags[0].Code + " " + diags[0].Message
	}
	if more != "" {
		return "unexpected " + more + " after the example"
	}
	return ""
}

func hasErrors(res evalResult) bool {
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// parseBools reads the rendered `List Bool` of n elements.
func parseBools(v string, n int) ([]bool, error) {
	inner, ok := strings.CutPrefix(v, "[")
	if inner, ok = strings.CutSuffix(inner, "]"); !ok {
		return nil, fmt.Errorf("examples: want a list of %d Bools, got %s", n, v)
	}
	var out []bool
	for _, s := range strings.Split(inner, ", ") {
		switch s {
		case "True":
			out = append(out, true)
		case "False":
			out = append(out, false)
		default:
			return nil, fmt.Errorf("examples: want a list of %d Bools, got %s", n, v)
		}
	}
	if len(out) != n {
		return nil, fmt.Errorf("examples: want a list of %d Bools, got %s", n, v)
	}
	return out, nil
}

// recordFields splits the rendered record `{ got1: v1, got4: v4 }` into its values by
// example index; its fields are in the order written.
func recordFields(v string, idx []int) map[int]string {
	inner, ok := strings.CutPrefix(v, "{ ")
	if inner, ok = strings.CutSuffix(inner, " }"); !ok {
		return nil
	}
	out := map[int]string{}
	for k, i := range idx {
		label := fmt.Sprintf("got%d: ", i+1)
		rest, ok := strings.CutPrefix(inner, label)
		if !ok {
			return nil
		}
		end := len(rest)
		if k+1 < len(idx) {
			end = strings.Index(rest, fmt.Sprintf(", got%d: ", idx[k+1]+1))
			if end < 0 {
				return nil
			}
		}
		out[i] = rest[:end]
		inner = strings.TrimPrefix(rest[end:], ", ")
	}
	return out
}

func TestStdlibDocs_FixtureFails(t *testing.T) {
	fsys := os.DirFS("testdata/stdlibdocs")
	got := stdlibDocProblems(fsys, nil, nil, bookHeadingExists)
	want := []string{
		"stdlib/Bad.alder:1: Bad: no doc comment",
		"stdlib/Bad.alder:3: Bad.undocumented: no doc comment",
		"stdlib/Bad.alder:5: Bad.fragment: the summary is not one line ending in a full stop",
		"stdlib/Bad.alder:5: Bad.fragment: no example",
		"stdlib/Bad.alder:7: Bad.noExample: no example",
		`stdlib/Bad.alder:22: Bad.badBook: Book: chapter 9, "No such heading": no such heading in the book`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	// Review Focus 3: both wrong examples in one run, each at its own first line.
	fails := strings.Join(runExamples(t, stdlibExamples(fsys)), "\n")
	// A parse error is reported at its own example (I-1); a misspelt name (E0002) does not hide
	// the ill-typed example (E0003) or the false ones (I-2).
	for _, w := range []string{
		"stdlib/Bad.alder:10: 1 + 1 == 3: want 3, got 2",
		"stdlib/Bad.alder:14: 1 +: E0003 ",
		"stdlib/Bad.alder:25: (1 + 1 == 2: ",
		"stdlib/Bad.alder:33: List.nope [] == 1: E0002 ",
		"stdlib/Bad.alder:37: 2 + 2 == 5: want 5, got 4",
	} {
		if !strings.Contains(fails, w) {
			t.Errorf("missing %q in:\n%s", w, fails)
		}
	}
	for _, w := range []string{"Bad.alder:19", "Bad.alder:29", "examples: "} {
		if strings.Contains(fails, w) {
			t.Errorf("unexpected %q in:\n%s", w, fails)
		}
	}
}

func TestStdlibDocs_Complete(t *testing.T) {
	for _, p := range stdlibDocProblems(stdlib.FS, pendingDocs, noExample, bookHeadingExists) {
		t.Error(p)
	}
}

func TestStdlibDocs_Examples(t *testing.T) {
	for _, f := range runExamples(t, stdlibExamples(stdlib.FS)) {
		t.Error(f)
	}
}

func TestBookHeadingExists(t *testing.T) {
	if !bookHeadingExists(`chapter 9, "List"`) || !bookHeadingExists(`chapter 6, "Notices and in-flight state"`) || bookHeadingExists(`chapter 9, "Nope"`) {
		t.Fatal("heading lookup")
	}
}
