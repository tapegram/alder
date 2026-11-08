package grammar

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"alder/internal/semtok"
	"alder/internal/syntax"
)

// minParity guards the parity test against a shrinking corpus, as minSources guards the drift test.
const minParity = 1400

// tsTypes maps each capture of queries/neovim/highlights.scm to the legend type the server gives
// the same bytes (internal/semtok), "" for a capture the server leaves uncoloured. A capture
// missing here fails the test: a new query capture needs a decision.
var tsTypes = map[string]string{
	"variable": "variable", "constructor": "enumMember", "comment": "comment",
	"string": "string", "string.special.url": "string", "number": "number", "number.float": "number",
	"boolean": "boolean", "constant.builtin": "enumMember", "variable.builtin": "hole", "character.special": "",
	"keyword": "keyword", "keyword.import": "keyword", "keyword.type": "keyword",
	"keyword.conditional": "keyword", "keyword.repeat": "keyword", "keyword.function": "keyword",
	"operator": "operator", "punctuation.bracket": "", "punctuation.delimiter": "", "punctuation.special": "",
	"function": "function", "function.call": "function", "function.method.call": "method",
	"variable.parameter": "parameter", "type": "type", "type.definition": "type", "module": "namespace",
	"variable.member": "property", "property": "property", "attribute": "decorator",
	"tag": "tag", "tag.attribute": "attribute", "tag.delimiter": "",
}

// hlCapture is one capture `tree-sitter query` prints: the pattern it belongs to, its name and
// its span (0-based rows, byte columns).
type hlCapture struct {
	pattern        int
	name           string
	sr, sc, er, ec int
}

var (
	patternRE   = regexp.MustCompile(`^\s+pattern: (\d+)$`)
	hlCaptureRE = regexp.MustCompile(`^\s+capture: (?:\d+ - )?([a-z.]+), start: \((\d+), (\d+)\), end: \((\d+), (\d+)\)`)
)

// queryHighlights runs highlights.scm over the files and returns each file's captures.
func queryHighlights(t *testing.T, cli, paths string) map[string][]hlCapture {
	t.Helper()
	res := map[string][]hlCapture{}
	pattern := -1
	scm := filepath.Join(grammarDir(t), "queries", "neovim", "highlights.scm")
	runQuery(t, cli, scm, paths, func(file, line string) {
		if m := patternRE.FindStringSubmatch(line); m != nil {
			pattern, _ = strconv.Atoi(m[1])
			return
		}
		m := hlCaptureRE.FindStringSubmatch(line)
		if m == nil {
			return
		}
		n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
		res[file] = append(res[file], hlCapture{pattern, m[1], n(2), n(3), n(4), n(5)})
	})
	return res
}

// parityDiff is a run of bytes on one line that the server and nvim colour differently.
type parityDiff struct {
	label        string
	line, col    int // 1-based
	text         string
	server, nvim string // legend types, "none" for no colour
}

func (d parityDiff) String() string {
	return fmt.Sprintf("%s:%d:%d `%s`: server %s, nvim %s", d.label, d.line, d.col, d.text, d.server, d.nvim)
}

// parityExceptions are differences the test accepts, each with its reason: a construct the
// server and nvim cannot agree on without a grammar change. Every entry is listed in the PR that
// adds it.
var parityExceptions = []struct {
	server, nvim string
	text         *regexp.Regexp
	why          string
}{}

func excepted(d parityDiff) bool {
	for _, e := range parityExceptions {
		if e.server == d.server && e.nvim == d.nvim && e.text.MatchString(d.text) {
			return true
		}
	}
	return false
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// parityDiffs compares, byte by byte, the server's tokens with the captures under nvim's rule
// (the highest pattern covering a byte wins). Whitespace is skipped, and so are the bytes
// inside a string's interpolation braces, which tree-sitter does not parse (README). The mask
// comes from syntax.Interpolations on every string token, so it also skips `{…}` inside strings
// that are not interpolated (route paths, test names): a few bytes both sides colour as string.
func parityDiffs(label string, src []byte, toks []semtok.Token, caps []hlCapture) ([]parityDiff, error) {
	lines := strings.Split(string(src), "\n")
	goT := make([][]string, len(lines))
	tsT := make([][]string, len(lines))
	tsP := make([][]int, len(lines))
	mask := make([][]bool, len(lines))
	for i, l := range lines {
		goT[i], tsT[i], tsP[i], mask[i] = make([]string, len(l)), make([]string, len(l)), make([]int, len(l)), make([]bool, len(l))
		for b := range tsP[i] {
			tsP[i][b] = -1
		}
	}
	for _, k := range toks {
		for b := k.Col - 1; k.Line-1 < len(lines) && b < k.Col-1+k.Len && b < len(lines[k.Line-1]); b++ {
			goT[k.Line-1][b] = semtok.Types[k.Kind]
		}
	}
	for _, c := range caps {
		if c.name == "spell" { // a spell-checking region, not a colour
			continue
		}
		typ, ok := tsTypes[c.name]
		if !ok {
			return nil, fmt.Errorf("capture @%s has no entry in tsTypes", c.name)
		}
		for r := c.sr; r <= c.er && r < len(lines); r++ {
			from, to := 0, len(lines[r])
			if r == c.sr {
				from = c.sc
			}
			if r == c.er {
				to = min(c.ec, to)
			}
			for b := from; b < to; b++ {
				if c.pattern > tsP[r][b] {
					tsP[r][b], tsT[r][b] = c.pattern, typ
				}
			}
		}
	}
	_, all, _ := syntax.ParseTokens(label, src)
	for _, tk := range all {
		for _, in := range syntax.Interpolations(label, tk) {
			for b := tk.Col - 1 + in.From; b < tk.Col-1+in.To && b < len(mask[tk.Line-1]); b++ {
				mask[tk.Line-1][b] = true
			}
		}
	}
	var out []parityDiff
	skip := func(r, b int) bool { c := lines[r][b]; return c == ' ' || c == '\t' || c == '\r' || mask[r][b] }
	for r, l := range lines {
		for b := 0; b < len(l); {
			if skip(r, b) || goT[r][b] == tsT[r][b] {
				b++
				continue
			}
			e := b
			for e < len(l) && !skip(r, e) && goT[r][e] == goT[r][b] && tsT[r][e] == tsT[r][b] {
				e++
			}
			out = append(out, parityDiff{label, r + 1, b + 1, l[b:e], orNone(goT[r][b]), orNone(tsT[r][b])})
			b = e
		}
	}
	return out, nil
}

// fileColumns moves the server's tokens on the first line of a file that starts with a UTF-8
// BOM past it. The lexer drops the BOM, as nvim and VS Code drop it from the buffer they send the
// server, so the server's columns count from after it; the tree-sitter CLI's count from the
// file's first byte.
func fileColumns(src []byte, toks []semtok.Token) []semtok.Token {
	const bom = "\xEF\xBB\xBF"
	if !bytes.HasPrefix(src, []byte(bom)) {
		return toks
	}
	for i := range toks {
		if toks[i].Line == 1 {
			toks[i].Col += len(bom)
		}
	}
	return toks
}

// TestParityDiffs checks the comparison itself, without the CLI: the later pattern wins, spell
// is ignored, whitespace and interpolations are skipped.
func TestParityDiffs(t *testing.T) {
	src := []byte("module M\n\nx = f \"a {y} b\"\n")
	toks := semtok.Module("t.alder", src)
	caps := []hlCapture{
		{11, "keyword.import", 0, 0, 0, 6}, {35, "module", 0, 7, 0, 8}, {1, "constructor", 0, 7, 0, 8},
		{21, "function", 2, 0, 2, 1}, {0, "variable", 2, 0, 2, 1}, {18, "operator", 2, 2, 2, 3},
		{38, "function.call", 2, 4, 2, 5}, {0, "variable", 2, 4, 2, 5}, {3, "string", 2, 6, 2, 15},
		{2, "spell", 2, 6, 2, 15},
	}
	ds, err := parityDiffs("t.alder", src, toks, caps)
	if err != nil || len(ds) != 0 {
		t.Fatalf("diffs %v, err %v", ds, err)
	}
	caps[0].name = "operator" // `module` now differs
	if ds, _ := parityDiffs("t.alder", src, toks, caps); len(ds) != 1 || ds[0].text != "module" || ds[0].nvim != "operator" {
		t.Fatalf("diffs %v", ds)
	}
	if _, err := parityDiffs("t.alder", src, toks, []hlCapture{{1, "made.up", 0, 0, 0, 1}}); err == nil {
		t.Fatal("an unknown capture must fail")
	}
}

// TestHighlightParity: for every module the drift test reads (stdlib, which parses in signature
// mode, and scratch files aside), the server's semantic tokens colour each byte as nvim colours
// it with queries/neovim/highlights.scm. This is the guard that the two editors look the same.
func TestHighlightParity(t *testing.T) {
	cli := tsCLI(t)
	dir := t.TempDir()
	var srcs []source
	for _, s := range sources(t, dir) { // scratch files are not here: they fail ParseFile in sources
		if !strings.HasPrefix(filepath.ToSlash(s.Label), "../stdlib") {
			srcs = append(srcs, s)
		}
	}
	if len(srcs) < minParity {
		t.Fatalf("only %d sources; expected at least %d", len(srcs), minParity)
	}
	caps := queryHighlights(t, cli, writePaths(t, dir, srcs))
	var bad []string
	files := 0
	for _, s := range srcs {
		src, err := os.ReadFile(s.Path)
		if err != nil {
			t.Fatal(err)
		}
		ds, err := parityDiffs(s.Label, src, fileColumns(src, semtok.Module(s.Path, src)), caps[s.Path])
		if err != nil {
			t.Fatal(err)
		}
		n := len(bad)
		for _, d := range ds {
			if !excepted(d) {
				bad = append(bad, d.String())
			}
		}
		if len(bad) > n {
			files++
		}
	}
	if len(bad) > 0 {
		shown := bad
		if len(shown) > 20 {
			shown = shown[:20]
		}
		t.Fatalf("%d differences in %d of %d sources:\n%s", len(bad), files, len(srcs), strings.Join(shown, "\n"))
	}
	t.Logf("%d sources agree", len(srcs))
}
