package scratch

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"alder/internal/diag"
	"alder/internal/snippets"
	"alder/internal/syntax"
)

// IsScratchFile reports whether rel (slash-separated, relative to the project root) is a
// scratch file: its base name ends in "scratch.alder" and it is not under the app's app/ directory.
func IsScratchFile(rel string) bool {
	return strings.HasSuffix(path.Base(rel), "scratch.alder") && !strings.HasPrefix(rel, "app/")
}

// Watch is one `> expr` item of a scratch file.
type Watch struct {
	Line, End int    // 1-based: the `>` line and the watch's last line (continuations included)
	Name      string // "it1", "it2", … in file order
	Expr      string // as written, without the leading `> `; lines joined by "\n"
}

// File is a parsed scratch file.
type File struct {
	Name    string   // the path as given, for diagnostics
	Lines   []string // the file's lines
	In      string   // the `in` header's module; "" without one
	Source  string   // module source: same line count as Lines; header line blanked; each watch's first line `itK = expr`
	Watches []Watch
	Map     SourceMap // InLine identity; Shift on each watch's first line = 2 - len("itK = ")
}

// appModuleRE is an app module's name, dotted for a nested one (Todos.Index), as for --in.
var appModuleRE = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*(_[A-Za-z0-9]+)*(\.[A-Z][A-Za-z0-9]*(_[A-Za-z0-9]+)*)*$`)

// reservedRE is a name the file's watches take.
var reservedRE = regexp.MustCompile(`^it[0-9]+$`)

// ParseFile splits a scratch file into its module source and watches. A watch starts at a
// column-1 item start (snippets.Starts) that is `>` or begins `> `, and runs until the next
// start. The `in Module` header is recognised on the first start only. The diagnostics are
// positioned in the file, named name; the source is still returned, so a caller may go on.
func ParseFile(name, text string) (File, []diag.Diagnostic) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	f := File{Name: name, Lines: lines}
	var diags []diag.Diagnostic
	errAt := func(line, col, width int, msg string) {
		diags = append(diags, diag.Diagnostic{Severity: "error", Message: msg, Span: diag.Span{
			File: name, Start: diag.Pos{Line: line, Col: col}, End: diag.Pos{Line: line, Col: col + width},
		}})
	}

	starts := snippets.Starts(lines)
	src := append([]string(nil), lines...)
	f.Map = SourceMap{Lines: lines, InLine: make([]int, len(lines)), Shift: make([]int, len(lines))}
	for i := range lines {
		f.Map.InLine[i] = i + 1
	}

	first := true    // the next start is the file's first
	cur := -1        // index in Watches of the watch the line continues
	dropped := false // the item before is one with a diagnostic, blanked with its continuation lines
	for i, l := range lines {
		if !starts[i] {
			if dropped {
				src[i] = ""
			}
			if cur >= 0 && strings.TrimSpace(l) != "" && !strings.HasPrefix(strings.TrimSpace(l), "--") {
				f.Watches[cur].End = i + 1
			}
			continue
		}
		wasFirst := first
		first, cur, dropped = false, -1, false
		switch {
		case l == "in" || strings.HasPrefix(l, "in ") || strings.HasPrefix(l, "in\t"):
			src[i], dropped = "", true
			mod := strings.TrimSpace(l[2:])
			switch {
			case !wasFirst:
				errAt(i+1, 1, 2, "`in Module` must be the file's first line")
			case !appModuleRE.MatchString(mod):
				errAt(i+1, 2+1+len(l[2:])-len(strings.TrimLeft(l[2:], " \t")), len(mod), "`in` wants an app module name such as Todo or Todos.Index")
			default:
				f.In = mod
			}
		case l == ">" || strings.HasPrefix(l, "> ") || strings.HasPrefix(l, ">\t"):
			expr := strings.TrimSpace(l[1:])
			if expr == "" {
				src[i], dropped = "", true
				errAt(i+1, 1, 1, "a watch needs an expression: `> expr`")
				break
			}
			w := Watch{Line: i + 1, End: i + 1, Name: fmt.Sprintf("it%d", len(f.Watches)+1), Expr: strings.TrimLeft(l[1:], " \t")}
			prefix := w.Name + " = "
			src[i] = prefix + w.Expr
			f.Map.Shift[i] = 1 + len(l[1:]) - len(w.Expr) - len(prefix)
			f.Watches = append(f.Watches, w)
			cur = len(f.Watches) - 1
		case strings.HasPrefix(l, ">!"):
			src[i], dropped = "", true
			errAt(i+1, 1, 2, "`>!` (run a Task) is not supported yet: scratch evaluates pure values only")
		}
	}
	for k := range f.Watches {
		w := &f.Watches[k]
		if w.End > w.Line {
			w.Expr += "\n" + strings.Join(lines[w.Line:w.End], "\n")
		}
	}
	f.Source = strings.Join(src, "\n")

	// A name the watches take may not be used: the generated `itK =` is the only one allowed.
	toks, _ := syntax.Tokens(name, []byte(f.Source))
	gen := map[int]string{} // line -> the generated name it starts with
	for _, w := range f.Watches {
		gen[w.Line] = w.Name
	}
	for _, t := range toks {
		if t.Kind != syntax.LIdent || !reservedRE.MatchString(t.Text) {
			continue
		}
		col := t.Col
		if g, ok := gen[t.Line]; ok {
			if t.Col == 1 && t.Text == g {
				continue
			}
			col += f.Map.Shift[t.Line-1]
		}
		errAt(t.Line, max(1, col), len(t.Text), fmt.Sprintf("`%s` is reserved for the file's watches; rename it", t.Text))
	}
	return f, diags
}
