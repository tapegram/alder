// Package book reads the fenced Alder blocks of the book's chapters: where they are, what kind
// of source each is, and what its fence says to expect of it. The verifier (book_test.go) checks
// every block against the real compiler with them; the REPL and `alder test` reuse them.
package book

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// A Snippet is one fenced block whose info string starts with the word `alder`.
type Snippet struct {
	File   string // the chapter's path, as given to Extract
	Line   int    // the line of the opening fence; the source's first line is Line+1
	Info   string // the fence's info string, e.g. "alder eval=3"
	Source string // the lines between the fences, each ending in "\n"
}

// Kind is what a snippet's source is.
type Kind int

const (
	// Module is a whole module: its first line of code is `module …`.
	Module Kind = iota
	// Decls is declarations without a module header: its first line of code is a declaration
	// (a declaration keyword, `import`, `name : T` or `name args = …` at column 1). A block of
	// comments alone is an empty Decls.
	Decls
	// Expr is anything else: one expression, or several, each starting at column 1.
	Expr
	// Signatures is a reference table of existing names and their types, one `name : T` (or
	// `a, b : T`) per line, at least one name qualified (`List.map : …`); a line may also list
	// names alone (`Float.min, Float.max`). Each typed name is checked to have that type.
	Signatures
)

func (k Kind) String() string {
	switch k {
	case Module:
		return "module"
	case Decls:
		return "decls"
	case Expr:
		return "expr"
	case Signatures:
		return "signatures"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// ExpectKind is what a fence asks of its block.
type ExpectKind int

const (
	// Check: the block compiles cleanly (```alder).
	Check ExpectKind = iota
	// Eval: the block, given to `alder eval`, prints Value (```alder eval=<value>).
	Eval
	// Error: compiling the block reports the diagnostic Code (```alder error=<CODE>).
	Error
	// Fragment: the block is a deliberate excerpt and is not checked (```alder fragment).
	Fragment
	// Part: the block is one piece of module Module (```alder part=<Module>). The part= blocks
	// of one module in a chapter, consecutive among the chapter's part= blocks, are joined in
	// order (JoinParts) and checked as that module, replacing the example's; the first piece
	// starts with its `module` line. A walk through one module, piece by piece, is checked
	// whole this way.
	Part
)

func (k ExpectKind) String() string {
	switch k {
	case Check:
		return "check"
	case Eval:
		return "eval"
	case Error:
		return "error"
	case Fragment:
		return "fragment"
	case Part:
		return "part"
	}
	return fmt.Sprintf("ExpectKind(%d)", int(k))
}

// Expect is a fence's expectation of its block.
type Expect struct {
	Kind   ExpectKind
	Value  string // Eval: the expected value, verbatim (the rest of the info string after `eval=`)
	Code   string // Error: the expected diagnostic code
	Module string // Part: the module the block is a piece of
}

var (
	codeRE       = regexp.MustCompile(`^[EW][0-9]{4}$`)
	moduleNameRE = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*(\.[A-Z][A-Za-z0-9_]*)*$`)
)

// Expectation parses a fence's info string: `alder`, `alder eval=<value>` (the value is the rest
// of the line, verbatim), `alder error=<CODE>`, `alder fragment` or `alder part=<Module>`.
// Anything else is an error.
func Expectation(info string) (Expect, error) {
	rest, ok := strings.CutPrefix(info, "alder")
	if !ok || (rest != "" && rest[0] != ' ') {
		return Expect{}, fmt.Errorf("not an alder fence: %q", info)
	}
	rest = strings.TrimLeft(rest, " ")
	switch {
	case rest == "":
		return Expect{Kind: Check}, nil
	case rest == "fragment":
		return Expect{Kind: Fragment}, nil
	case strings.HasPrefix(rest, "eval="):
		v := strings.TrimPrefix(rest, "eval=")
		if v == "" {
			return Expect{}, fmt.Errorf("empty eval= value in %q", info)
		}
		return Expect{Kind: Eval, Value: v}, nil
	case strings.HasPrefix(rest, "error="):
		c := strings.TrimPrefix(rest, "error=")
		if !codeRE.MatchString(c) {
			return Expect{}, fmt.Errorf("error= wants a diagnostic code such as E0304, got %q", c)
		}
		return Expect{Kind: Error, Code: c}, nil
	case strings.HasPrefix(rest, "part="):
		m := strings.TrimPrefix(rest, "part=")
		if !moduleNameRE.MatchString(m) {
			return Expect{}, fmt.Errorf("part= wants a module name such as Todos.Index, got %q", m)
		}
		return Expect{Kind: Part, Module: m}, nil
	}
	return Expect{}, fmt.Errorf("unknown fence expectation %q (want eval=, error=, part= or fragment)", rest)
}

// Extract returns the ```alder blocks of the Markdown file at path, in order.
func Extract(path string) ([]Snippet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Snippet
	var cur *Snippet
	var src strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	inOther := false // inside a fence that is not alder
	for sc.Scan() {
		line++
		l := sc.Text()
		fence := strings.HasPrefix(l, "```")
		switch {
		case cur != nil && fence:
			cur.Source = src.String()
			out = append(out, *cur)
			cur = nil
		case cur != nil:
			src.WriteString(l)
			src.WriteString("\n")
		case inOther && fence:
			inOther = false
		case inOther:
		case fence:
			info := strings.TrimSpace(strings.TrimPrefix(l, "```"))
			if info == "alder" || strings.HasPrefix(info, "alder ") {
				cur = &Snippet{File: path, Line: line, Info: info}
				src.Reset()
			} else {
				inOther = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if cur != nil {
		return out, fmt.Errorf("%s:%d: unterminated ```alder block", path, cur.Line)
	}
	return out, nil
}

var (
	declKeywordRE = regexp.MustCompile(`^(type|table|form|page|msg|capability|session|login|test|import)( |$)`)
	// signatureRE is `name : T` at column 1.
	signatureRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]* :( |$)`)
	// definitionRE is `name args = …` at column 1.
	definitionRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*( [a-z_][A-Za-z0-9_]*)* =( |$)`)
)

// firstCode is the first line of src that is neither blank nor a comment ("" when none).
func firstCode(src string) string {
	for _, l := range strings.Split(src, "\n") {
		t := strings.TrimSpace(l)
		if t != "" && !strings.HasPrefix(t, "--") {
			return l
		}
	}
	return ""
}

// Classify says what kind of source a snippet is, from its first line of code.
func Classify(s Snippet) Kind {
	l := firstCode(s.Source)
	switch {
	case l == "":
		return Decls
	case strings.HasPrefix(l, "module "):
		return Module
	case isSignatures(s.Source):
		return Signatures
	case strings.HasPrefix(l, "let "): // `let` lines before an expression, as `alder eval` takes them
		return Expr
	case declKeywordRE.MatchString(l), signatureRE.MatchString(l), definitionRE.MatchString(l):
		return Decls
	}
	return Expr
}

// ModuleName is the name in a Module snippet's `module` line ("" for other kinds).
func ModuleName(s Snippet) string {
	l := firstCode(s.Source)
	name, ok := strings.CutPrefix(l, "module ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}

// ExprStarts reports, for each line of an Expr snippet's source, whether an expression starts
// there: a column-1 line that is not blank, a comment, or a closing `}`, `)`, `]` or `</…>`. A
// block of several expressions (`add 1 2` then `List.map (add 1) [1, 2, 3]`) has several starts.
func ExprStarts(src string) []bool {
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	starts := make([]bool, len(lines))
	for i, l := range lines {
		t := strings.TrimSpace(l)
		starts[i] = t != "" && !strings.HasPrefix(t, "--") && l[0] != ' ' && l[0] != '\t' &&
			!strings.HasPrefix(t, "}") && !strings.HasPrefix(t, ")") && !strings.HasPrefix(t, "]") &&
			!strings.HasPrefix(t, "</")
	}
	return starts
}

// A Sig is one typed line of a Signatures snippet: `List.map : (a -> b) -> List a -> List b`.
type Sig struct {
	Line  int      // 1-based, in the snippet's source
	Names []string // the names given the type: one, or several (`Date.year, Date.month : …`)
	Type  string   // "" for a line of names alone
}

var (
	nameRE = `(?:[A-Za-z_][A-Za-z0-9_]*\.)*[A-Za-z_][A-Za-z0-9_]*`
	sigRE  = regexp.MustCompile(`^(` + nameRE + `(?:, *` + nameRE + `)*)(?: +: +(.+?))? *$`)
)

// Sigs parses a Signatures snippet's lines (blank and comment lines skipped, trailing comments
// dropped); ok is false when a line of code is not a signature or a list of names.
func Sigs(src string) (sigs []Sig, ok bool) {
	for i, l := range strings.Split(strings.TrimRight(src, "\n"), "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		if l[0] == ' ' || l[0] == '\t' {
			return nil, false
		}
		if c := strings.Index(l, "--"); c >= 0 {
			l = l[:c]
		}
		m := sigRE.FindStringSubmatch(strings.TrimRight(l, " \t"))
		if m == nil {
			return nil, false
		}
		var names []string
		for _, n := range strings.Split(m[1], ",") {
			names = append(names, strings.TrimSpace(n))
		}
		sigs = append(sigs, Sig{Line: i + 1, Names: names, Type: m[2]})
	}
	return sigs, true
}

// isSignatures: every line of code is a signature or a list of names, at least one has a type,
// and at least one name is qualified.
func isSignatures(src string) bool {
	sigs, ok := Sigs(src)
	if !ok {
		return false
	}
	typed, qualified := false, false
	for _, s := range sigs {
		typed = typed || s.Type != ""
		for _, n := range s.Names {
			qualified = qualified || strings.Contains(n, ".")
		}
	}
	return typed && qualified
}

// JoinParts joins the pieces of one module, in order, with a blank line between them, and
// returns the source and, for each of its lines, the book line it came from (0 for a blank line
// put between two pieces).
func JoinParts(parts []Snippet) (src string, lines []int) {
	var b strings.Builder
	for k, p := range parts {
		if k > 0 {
			b.WriteString("\n")
			lines = append(lines, 0)
		}
		for i, l := range strings.Split(strings.TrimRight(p.Source, "\n"), "\n") {
			b.WriteString(l + "\n")
			lines = append(lines, p.Line+1+i)
		}
	}
	return b.String(), lines
}
