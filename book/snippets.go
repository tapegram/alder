// Package book reads the fenced Alder blocks of the book's chapters: where they are, what kind
// of source each is, and what its fence says to expect of it. The verifier (book_test.go) checks
// every block against the real compiler with them; the REPL and `alder test` reuse them.
package book

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
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
	// Error: compiling the block reports the diagnostic Code, in the block's own source, its
	// message containing Message when one is given (```alder error=<CODE> "<message part>").
	Error
	// Fragment: the block is a deliberate excerpt and is not checked (```alder fragment).
	Fragment
	// Part: the block is one piece of module Module of an example app (```alder part=<Module>
	// for examples/todos, ```alder part=<example>/<Module> for examples/<example>). The part=
	// blocks of one module in a chapter, consecutive among the chapter's part= blocks, are
	// joined in order (JoinParts) and checked as that module in a copy of that example,
	// replacing its file; the first piece starts with its `module` line. A walk through one
	// module, piece by piece, is checked whole this way.
	Part
	// Written: the block is a file as a command of the chapter left it (```alder
	// written=<state>/<path>, after a ```text command=<state> block): TestBook_Commands compares
	// it, byte for byte, with the file in that state's app; the verifier does not check it again,
	// since the command that wrote it checked the app.
	Written
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
	case Written:
		return "written"
	}
	return fmt.Sprintf("ExpectKind(%d)", int(k))
}

// Expect is a fence's expectation of its block.
type Expect struct {
	Kind  ExpectKind
	Value string // Eval: the expected value, verbatim (the rest of the info string after `eval=`)
	Code  string // Error: the expected diagnostic code
	// Message is, for Error, a part of the expected diagnostic's message ("" for any message)
	Message string
	Module  string // Part: the module the block is a piece of
	Example string // Part: the example app it belongs to, a directory under examples/ ("todos" by default); Written: the state
	Path    string // Written: the file's path in the state's app, slash-separated ("app/About.alder")
}

// DefaultExample is the example app a plain ```alder part=<Module> block belongs to, and the
// one every other checked block is placed in.
const DefaultExample = "todos"

var (
	codeRE       = regexp.MustCompile(`^[EW][0-9]{4}$`)
	moduleNameRE = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*(\.[A-Z][A-Za-z0-9_]*)*$`)
	exampleRE    = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// Expectation parses a fence's info string: `alder`, `alder eval=<value>` (the value is the rest
// of the line, verbatim), `alder error=<CODE>` optionally followed by a quoted part of the
// message (`alder error=E0003 "expected Int"`, Go string syntax), `alder fragment`,
// `alder part=<Module>` or `alder part=<example>/<Module>` (`alder part=blog/Posts.Show`: the
// example is a lowercase directory name under examples/). Anything else is an error.
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
		c, msg, _ := strings.Cut(strings.TrimPrefix(rest, "error="), " ")
		if !codeRE.MatchString(c) {
			return Expect{}, fmt.Errorf("error= wants a diagnostic code such as E0304, got %q", c)
		}
		e := Expect{Kind: Error, Code: c}
		if msg = strings.TrimSpace(msg); msg != "" {
			m, err := strconv.Unquote(msg)
			if err != nil || !strings.HasPrefix(msg, `"`) || m == "" {
				return Expect{}, fmt.Errorf("error=%s wants a quoted part of the message after it, got %s", c, msg)
			}
			e.Message = m
		}
		return e, nil
	case strings.HasPrefix(rest, "written="):
		st, p, ok := strings.Cut(strings.TrimPrefix(rest, "written="), "/")
		if !ok || !exampleRE.MatchString(st) || p == "" || strings.Contains(p, " ") {
			return Expect{}, fmt.Errorf("written= wants <state>/<path>, such as guestbook/app/About.alder, got %q", strings.TrimPrefix(rest, "written="))
		}
		return Expect{Kind: Written, Example: st, Path: p}, nil
	case strings.HasPrefix(rest, "part="):
		m := strings.TrimPrefix(rest, "part=")
		ex := DefaultExample
		if e, mod, ok := strings.Cut(m, "/"); ok {
			if !exampleRE.MatchString(e) {
				return Expect{}, fmt.Errorf("part= wants an example such as blog before the /, got %q", e)
			}
			ex, m = e, mod
		}
		if !moduleNameRE.MatchString(m) {
			return Expect{}, fmt.Errorf("part= wants a module name such as Todos.Index or blog/Posts.Show, got %q", m)
		}
		return Expect{Kind: Part, Module: m, Example: ex}, nil
	}
	return Expect{}, fmt.Errorf("unknown fence expectation %q (want eval=, error=, part=, written= or fragment)", rest)
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
	declKeywordRE = regexp.MustCompile(`^(type|table|form|page|msg|capability|fake|session|login|test|import)( |$)`)
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

var tableRE = regexp.MustCompile(`(?m)^table ([A-Z][A-Za-z0-9_]*)`)

// TableModule is, for a Decls snippet that declares exactly one table, that table's name: a
// table is declared in a module named after it, in app/Table/ (E0018), so the verifier checks the block
// as that module. "" for any other snippet; a block with two tables cannot be one module and is
// marked `fragment`.
func TableModule(s Snippet) string {
	if Classify(s) != Decls {
		return ""
	}
	m := tableRE.FindAllStringSubmatch(s.Source, -1)
	if len(m) != 1 {
		return ""
	}
	return m[0][1]
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

// A FileBlock is a fenced block that quotes a whole file of an example app that is not an Alder
// module: its info string has a word `file=<example>/<path>` (```css file=guestbook/static/app.css).
// Its source must be the file, byte for byte.
type FileBlock struct {
	Line    int    // the line of the opening fence
	Example string // the example app, e.g. "guestbook"
	Path    string // the file's path in the app, slash-separated, e.g. "static/app.css"
	Source  string // the lines between the fences, each ending in "\n"
}

// ExtractFiles returns the file= blocks of the Markdown file at path, in order. A file= word
// without an example and a path is an error.
func ExtractFiles(path string) ([]FileBlock, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []FileBlock
	var cur *FileBlock
	var src strings.Builder
	inFence := false
	for i, l := range strings.Split(string(b), "\n") {
		fence := strings.HasPrefix(l, "```")
		switch {
		case cur != nil && fence:
			cur.Source = src.String()
			out = append(out, *cur)
			cur, inFence = nil, false
		case cur != nil:
			src.WriteString(l + "\n")
		case inFence && fence:
			inFence = false
		case fence:
			inFence = true
			for _, w := range strings.Fields(strings.TrimPrefix(l, "```")) {
				f, ok := strings.CutPrefix(w, "file=")
				if !ok {
					continue
				}
				ex, p, ok := strings.Cut(f, "/")
				if !ok || !exampleRE.MatchString(ex) || p == "" {
					return nil, fmt.Errorf("%s:%d: file= wants <example>/<path>, such as guestbook/static/app.css, got %q", path, i+1, f)
				}
				cur = &FileBlock{Line: i + 1, Example: ex, Path: p}
				src.Reset()
			}
		}
	}
	if cur != nil {
		return out, fmt.Errorf("%s:%d: unterminated file= block", path, cur.Line)
	}
	return out, nil
}

// A CommandBlock is a fenced block of chapter 15's kind (any language): a command and what it
// prints, its info string having a word `command=<state>` (```text command=guestbook), or a file
// as the commands before it left it, `written=<state>/<path>` (```alder
// written=guestbook/app/About.alder). A state is an app the chapter's commands change in order:
// `guestbook`, a copy of what `alder new guestbook` writes, or the name of an example app, a copy
// of examples/<name>. A command block's first line is `$ alder …`; when it ends in `<<'EOF'`,
// the lines up to `EOF` are its standard input; the rest is what it prints, standard output then
// standard error, where a line `…` stands for any number of lines.
type CommandBlock struct {
	Line   int    // the line of the opening fence
	State  string // the app the block runs in, or quotes
	Path   string // written=: the file's path in the app; "" for a command
	Source string // the lines between the fences, each ending in "\n"
}

// ExtractCommands returns the command= and written= blocks of the Markdown file at path, in
// order. A malformed command= or written= word is an error.
func ExtractCommands(path string) ([]CommandBlock, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []CommandBlock
	var cur *CommandBlock
	var src strings.Builder
	inFence := false
	for i, l := range strings.Split(string(b), "\n") {
		fence := strings.HasPrefix(l, "```")
		switch {
		case cur != nil && fence:
			cur.Source = src.String()
			out = append(out, *cur)
			cur, inFence = nil, false
		case cur != nil:
			src.WriteString(l + "\n")
		case inFence && fence:
			inFence = false
		case fence:
			inFence = true
			for _, w := range strings.Fields(strings.TrimPrefix(l, "```")) {
				if st, ok := strings.CutPrefix(w, "command="); ok {
					if !exampleRE.MatchString(st) {
						return nil, fmt.Errorf("%s:%d: command= wants a state, such as guestbook or blog, got %q", path, i+1, st)
					}
					cur = &CommandBlock{Line: i + 1, State: st}
				} else if f, ok := strings.CutPrefix(w, "written="); ok {
					st, p, ok := strings.Cut(f, "/")
					if !ok || !exampleRE.MatchString(st) || p == "" {
						return nil, fmt.Errorf("%s:%d: written= wants <state>/<path>, such as guestbook/app/About.alder, got %q", path, i+1, f)
					}
					cur = &CommandBlock{Line: i + 1, State: st, Path: p}
				}
			}
			if cur != nil {
				src.Reset()
			}
		}
	}
	if cur != nil {
		return out, fmt.Errorf("%s:%d: unterminated command= or written= block", path, cur.Line)
	}
	return out, nil
}

// MatchOutput reports whether got is what a command block says it prints, want: the same lines,
// where a line of want that is `…` alone matches any number of lines of got (none included), and
// a `…` inside a line matches any text within that line (a build id, a platform: chapter 16).
func MatchOutput(want, got string) bool {
	return matchLines(strings.Split(want, "\n"), strings.Split(got, "\n"))
}

func matchLines(want, got []string) bool {
	if len(want) == 0 {
		return len(got) == 0
	}
	if want[0] == "…" {
		for k := 0; k <= len(got); k++ {
			if matchLines(want[1:], got[k:]) {
				return true
			}
		}
		return false
	}
	return len(got) > 0 && matchLine(want[0], got[0]) && matchLines(want[1:], got[1:])
}

// matchLine reports whether the line got is want, each `…` in want standing for any text.
func matchLine(want, got string) bool {
	parts := strings.Split(want, "…")
	if len(parts) == 1 {
		return want == got
	}
	if !strings.HasPrefix(got, parts[0]) {
		return false
	}
	got = got[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(got, p)
		if i < 0 {
			return false
		}
		got = got[i+len(p):]
	}
	return len(got) >= len(last) && strings.HasSuffix(got, last)
}
