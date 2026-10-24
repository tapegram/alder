package lsp

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"alder/internal/diag"
	"alder/internal/markup"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

// The LSP CompletionItemKind values the server uses.
const (
	kindFunction    = 3
	kindConstructor = 4
	kindField       = 5
	kindVariable    = 6
	kindClass       = 7
	kindModule      = 9
	kindProperty    = 10
	kindKeyword     = 14
)

type completionItem struct {
	Label    string   `json:"label"`
	Kind     int      `json:"kind"`
	Detail   string   `json:"detail,omitempty"`
	SortText string   `json:"sortText,omitempty"`
	TextEdit TextEdit `json:"textEdit"`
}

// The contexts, matched against the cursor's line up to the cursor (completion's table, in
// order). A qualifier must not continue a longer name or a field chain.
var (
	reImport   = regexp.MustCompile(`^\s*import\s+([A-Za-z.]*)$`)
	reMember   = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])([A-Z][A-Za-z0-9]*(?:\.[A-Z][A-Za-z0-9]*)*)\.([a-zA-Z0-9_]*)$`)
	reField    = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])([a-z][A-Za-z0-9_]*)\.([a-zA-Z0-9_]*)$`)
	reCloseTag = regexp.MustCompile(`</([a-z0-9-]*)$`)
	reAttr     = regexp.MustCompile(`<([a-z][a-z0-9-]*)\s[^<>]*?\b([a-z-]*)$`)
	reTag      = regexp.MustCompile(`(?:^|[\s>({\[,])<([a-z0-9-]*)$`)
	reHole     = regexp.MustCompile(`\?([a-z][A-Za-z0-9_]*)?$`)
	reWord     = regexp.MustCompile(`[A-Za-z0-9_]*$`)
)

// completion is one completion request: the open text as it is now, read once, and the last
// good build with what it still says about the cursor (Ruling R11).
type completion struct {
	a      *analysis      // the last good build; nil before the first
	m      *syntax.Module // rel's module in a; nil when a has none
	lines  []string       // rel's current text
	line   int            // the cursor's line, 0-based
	col    int            // the cursor's byte offset in it
	prefix string         // the line up to the cursor
	pos    diag.Pos       // the cursor as the compiler counts
	fresh  bool           // lines above the cursor's are unchanged since a was built
	same   bool           // and the cursor's line too

	locals    []binder // scope's answer, once computed
	hasLocals bool
}

// completionReq answers textDocument/completion. The context comes from the current text before
// the cursor; types and scopes from the last good build, while the lines above the cursor are
// unchanged since it (Ruling R11).
func completionReq(s *server, params json.RawMessage) (any, error) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	items := []completionItem{}
	rel, ok := s.rel(p.TextDocument.URI)
	if ok {
		if cx := s.completionAt(rel, p.Position); cx != nil {
			items = append(items, cx.items()...)
		}
	}
	return map[string]any{"isIncomplete": false, "items": items}, nil
}

// completionAt is the completion at p in rel, nil when p is not in its text.
func (s *server) completionAt(rel string, p Position) *completion {
	lines := strings.Split(s.text(rel), "\n")
	if p.Line < 0 || p.Line >= len(lines) || p.Character < 0 {
		return nil
	}
	line := lines[p.Line]
	col := min(p.Character, len(line))
	for col > 0 && col < len(line) && !utf8.RuneStart(line[col]) {
		col--
	}
	cx := &completion{a: s.lastGood(), lines: lines, line: p.Line, col: col, prefix: line[:col],
		pos: diag.Pos{Line: p.Line + 1, Col: col + 1}}
	if cx.a != nil && cx.a.res != nil {
		for _, x := range cx.a.prog.Modules {
			if x.File == rel {
				cx.m = x
				break
			}
		}
	}
	if cx.m != nil {
		cx.fresh = freshLines(cx.a, rel, lines, cx.pos.Line, false)
		cx.same = cx.fresh && freshLines(cx.a, rel, lines, cx.pos.Line, true)
	}
	return cx
}

// items tries each context in the table's order; the first that applies answers.
func (cx *completion) items() []completionItem {
	for _, row := range []func() ([]completionItem, bool){
		cx.imports, cx.members, cx.fields, cx.closeTag, cx.attrs, cx.tags, cx.hole, cx.patterns,
	} {
		if items, ok := row(); ok {
			return items
		}
	}
	return cx.otherwise()
}

// edit replaces the n bytes before the cursor with text.
func (cx *completion) edit(n int, text string) TextEdit {
	return TextEdit{Range: Range{Start: Position{cx.line, cx.col - n}, End: Position{cx.line, cx.col}}, NewText: text}
}

func (cx *completion) item(label string, kind int, detail string, n int) completionItem {
	return completionItem{Label: label, Kind: kind, Detail: detail, TextEdit: cx.edit(n, label)}
}

// imports: after `import `, the modules of the program and the standard library that this module
// does not already import (and not the module itself).
func (cx *completion) imports() ([]completionItem, bool) {
	g := reImport.FindStringSubmatch(cx.prefix)
	if g == nil {
		return nil, false
	}
	var out []completionItem
	if cx.m != nil {
		imported := map[string]bool{cx.m.Name: true}
		for _, im := range cx.m.Imports {
			imported[im.Module] = true
		}
		for _, name := range cx.moduleNames() {
			if !imported[name] {
				it := cx.item(name, kindModule, "", len(g[1]))
				it.TextEdit.NewText = cx.importText(name)
				out = append(out, it)
			}
		}
	}
	return out, true
}

// importText is what accepting module path in an import completion inserts: the path, and
// `as Q` when the path's canonical qualifier among this module's imports is not its last
// segment (design rule 4). A standard-library module, or one with no usable qualifier, is
// inserted as it is.
func (cx *completion) importText(path string) string {
	isStd := func(n string) bool { return cx.a.prog.Std[n] != nil }
	if isStd(path) {
		return path
	}
	paths := []string{path}
	for _, im := range cx.m.Imports {
		if !isStd(im.Module) {
			paths = append(paths, im.Module)
		}
	}
	quals, _ := names.CanonicalQualifiers(cx.m.Name, paths, names.Taken(cx.m, isStd))
	if q := quals[path]; q != "" && names.NeedsAlias(path, q) {
		return path + " as " + q
	}
	return path
}

// moduleNames is every app and standard-library module, sorted.
func (cx *completion) moduleNames() []string {
	var names []string
	for name := range cx.a.prog.Modules {
		names = append(names, name)
	}
	for name := range cx.a.prog.Std {
		if cx.a.prog.Modules[name] == nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// members: after `M.`, where M is a module, its declarations.
func (cx *completion) members() ([]completionItem, bool) {
	g := reMember.FindStringSubmatch(cx.prefix)
	if g == nil || cx.m == nil {
		return nil, false
	}
	mod := moduleNamed(cx.a, cx.m, g[1])
	if mod == nil {
		return nil, false
	}
	return cx.declItems(mod, len(g[2])), true
}

// declItems is mod's declarations: values, types (tables, forms, records, unions, brands) and
// constructors (a union's, a message), each replacing the n bytes before the cursor.
func (cx *completion) declItems(mod *syntax.Module, n int) []completionItem {
	var out []completionItem
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			out = append(out, cx.valueItem(d.Name, cx.a.res.Values[mod.Name+"."+d.Name], n))
		case *syntax.TypeDecl:
			out = append(out, cx.item(d.Name, kindClass, "", n))
			for _, c := range d.Union {
				out = append(out, cx.ctorItem(mod.Name, c.Name, n))
			}
		case *syntax.TableDecl:
			out = append(out, cx.item(d.Name, kindClass, "", n))
		case *syntax.FormDecl:
			out = append(out, cx.item(d.Name, kindClass, "", n))
		case *syntax.MsgDecl:
			out = append(out, cx.ctorItem(mod.Name, d.Name, n))
		}
	}
	return out
}

// valueItem is a value or a local of type t (nil: unknown): a function when t is one.
func (cx *completion) valueItem(name string, t types.Type, n int) completionItem {
	if t == nil {
		return cx.item(name, kindVariable, "", n)
	}
	t = cx.a.res.U.Apply(t)
	kind := kindVariable
	if _, ok := t.(*types.Fun); ok {
		kind = kindFunction
	}
	return cx.item(name, kind, name+" : "+types.StringQ(t, cx.m.Name, cx.a.prog.Qualify(cx.m.Name)), n)
}

// ctorItem is constructor name of module mod, with its signature when the checker knows it.
func (cx *completion) ctorItem(mod, name string, n int) completionItem {
	if t := cx.ctorType(mod + "." + name); t != nil {
		return cx.item(name, kindConstructor, name+" : "+types.StringQ(t, cx.m.Name, cx.a.prog.Qualify(cx.m.Name)), n)
	}
	return cx.item(name, kindConstructor, "", n)
}

// ctorType is the type of constructor key ("Module.Ctor"): its union, or a function from its
// payload to it; nil when the checker does not know it.
func (cx *completion) ctorType(key string) types.Type {
	ci := cx.a.res.Ctors[key]
	if ci == nil || ci.Union == nil {
		return nil
	}
	if ci.Arg != nil {
		return &types.Fun{Params: []types.Type{ci.Arg}, Result: ci.Union}
	}
	return ci.Union
}

// fields: after `x.`, where x is in scope with a record type, its fields.
func (cx *completion) fields() ([]completionItem, bool) {
	g := reField.FindStringSubmatch(cx.prefix)
	if g == nil || !cx.fresh {
		return nil, false
	}
	var t types.Type
	if b := innermost(cx.scope(), g[1]); b != nil {
		t = cx.a.res.Binds[b.Key]
	} else {
		t = cx.a.res.Values[cx.m.Name+"."+g[1]]
	}
	if t == nil {
		return nil, false
	}
	r, ok := cx.a.res.U.Apply(t).(*types.Record)
	if !ok {
		return nil, false
	}
	var out []completionItem
	for _, f := range r.Fields {
		out = append(out, cx.item(f.Name, kindField, f.Name+" : "+types.StringQ(cx.a.res.U.Apply(f.Type), cx.m.Name, cx.a.prog.Qualify(cx.m.Name)), len(g[2])))
	}
	return out, true
}

// scope is the locals in scope at the cursor. On a line unchanged since the build, the scope at
// the cursor; on the line being typed, the scope at the end of what it keeps of its built text,
// else at the end of the line above, which holds the names bound before it (Ruling R11). Empty when the build does not hold above the cursor.
func (cx *completion) scope() []binder {
	if !cx.hasLocals {
		cx.locals, cx.hasLocals = cx.scopeNow(), true
	}
	return cx.locals
}

func (cx *completion) scopeNow() []binder {
	if !cx.fresh {
		return nil
	}
	if cx.same {
		return scopeAt(syntax.NodeAt(cx.m, cx.pos), cx.pos)
	}
	at, ok := cx.keptEnd()
	if !ok {
		if at, ok = cx.lineAbove(); !ok {
			return nil
		}
	}
	return scopeAt(syntax.NodeAt(cx.m, at), at)
}

// keptEnd is the end, in the build, of the longest common prefix of the cursor's line as built
// and as it is now, cut at the cursor; false when the line is new or that prefix is empty. Binders introduced
// earlier on the line being typed (a `for x in`, a lambda parameter) are in scope there.
func (cx *completion) keptEnd() (diag.Pos, bool) {
	l := cx.pos.Line
	if l > len(cx.m.Lines) {
		return diag.Pos{}, false
	}
	built, now := cx.m.Lines[l-1], cx.lines[cx.line]
	n := 0
	for n < cx.col && n < len(built) && n < len(now) && built[n] == now[n] {
		n++
	}
	if n == 0 {
		return diag.Pos{}, false
	}
	return diag.Pos{Line: l, Col: n + 1}, true
}

// lineAbove is the end of the line above the cursor's in the build; false on the first line.
func (cx *completion) lineAbove() (diag.Pos, bool) {
	l := cx.pos.Line - 1
	if l < 1 || l > len(cx.m.Lines) {
		return diag.Pos{}, false
	}
	return diag.Pos{Line: l, Col: len(cx.m.Lines[l-1]) + 1}, true
}

// closeTag: after `</`, the innermost tag still open before the cursor.
func (cx *completion) closeTag() ([]completionItem, bool) {
	g := reCloseTag.FindStringSubmatch(cx.prefix)
	if g == nil {
		return nil, false
	}
	start := cx.declStart()
	text := strings.Join(cx.lines[start:cx.line], "\n")
	if start < cx.line {
		text += "\n"
	}
	stack := openTags(text + cx.prefix[:len(cx.prefix)-len(g[0])])
	if len(stack) == 0 {
		return nil, true
	}
	return []completionItem{cx.item(stack[len(stack)-1], kindProperty, "", len(g[1]))}, true
}

// declStart is the 0-based line the declaration around the cursor starts on: the build's
// declaration holding the end of the line above when the build holds there, else the nearest
// line at or above the cursor's that starts in column 1 (where a declaration starts).
func (cx *completion) declStart() int {
	if cx.fresh {
		if above, ok := cx.lineAbove(); ok {
			if d := declAt(cx.m, above); d != nil {
				return d.GetSpan().Start.Line - 1
			}
		}
	}
	for l := cx.line; l >= 0; l-- {
		if s := cx.lines[l]; s != "" && s[0] != ' ' && s[0] != '\t' {
			return l
		}
	}
	return 0
}

// voidTags are the HTML elements that take no children and need no `/>`.
var voidTags = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}

// openTags is the stack of tags open at the end of text: `<tag …>` pushes, `</tag>` pops back
// to the tag, and `<tag … />` (or a void tag) pushes nothing. Inside an open tag, `>` within an
// attribute's `{…}` or quotes does not end it.
func openTags(text string) []string {
	var stack []string
	for i := 0; i < len(text); i++ {
		if text[i] != '<' {
			continue
		}
		if strings.HasPrefix(text[i:], "</") {
			name := tagNameAt(text, i+2)
			for k := len(stack) - 1; k >= 0; k-- {
				if stack[k] == name {
					stack = stack[:k]
					break
				}
			}
			continue
		}
		name := tagNameAt(text, i+1)
		if name == "" {
			continue
		}
		j, self := openTagEnd(text, i+1+len(name))
		if !self && !voidTags[name] {
			stack = append(stack, name)
		}
		i = j
	}
	return stack
}

// tagNameAt is the tag or component name starting at text[i] (`div`, `al-input`,
// `Layout.Page`), "" when none starts there.
func tagNameAt(text string, i int) string {
	j := i
	for j < len(text) && (isIdentByte(text[j]) || j > i && (text[j] == '-' || text[j] == '.')) {
		j++
	}
	if j == i || '0' <= text[i] && text[i] <= '9' || text[i] == '_' {
		return ""
	}
	return text[i:j]
}

// openTagEnd scans an open tag from text[i] to its `>`, and returns its index (len(text) when
// it does not end) and whether the tag closes itself (`/>`).
func openTagEnd(text string, i int) (int, bool) {
	depth := 0
	for ; i < len(text); i++ {
		switch c := text[i]; {
		case c == '"':
			if k := strings.IndexByte(text[i+1:], '"'); k >= 0 {
				i += k + 1
			} else {
				return len(text), false
			}
		case c == '{':
			depth++
		case c == '}':
			depth--
		case c == '>' && depth <= 0:
			return i, i > 0 && text[i-1] == '/'
		}
	}
	return len(text), false
}

// attrs: inside `<tag `, the tag's attributes and the global ones: `name=""`, or `name` for a
// boolean one.
func (cx *completion) attrs() ([]completionItem, bool) {
	g := reAttr.FindStringSubmatch(cx.prefix)
	if g == nil {
		return nil, false
	}
	var out []completionItem
	for _, at := range markup.HTML.AttrsOf(g[1]) {
		it := cx.item(at.Name, kindProperty, at.Description, len(g[2]))
		if !at.Boolean {
			it.TextEdit.NewText = at.Name + `=""`
		}
		out = append(out, it)
	}
	return out, true
}

// tags: after `<`, the built-in tags.
func (cx *completion) tags() ([]completionItem, bool) {
	g := reTag.FindStringSubmatch(cx.prefix)
	if g == nil {
		return nil, false
	}
	var out []completionItem
	for _, t := range markup.HTML.Tags {
		out = append(out, cx.item(t.Name, kindProperty, t.Description, len(g[1])))
	}
	return out, true
}

// hole: after `?name`, or with the cursor in a hole of the build, the names in scope, those
// whose type fits the hole's first (sortText 0…), then the others (1…). The edit replaces the
// whole hole, `?` included, so picking a name fills it.
func (cx *completion) hole() ([]completionItem, bool) {
	g := reHole.FindStringSubmatch(cx.prefix)
	h := cx.holeAt()
	if g == nil && h == nil {
		return nil, false
	}
	n := len(reWord.FindString(cx.prefix))
	if g != nil {
		n = len(g[0]) // the `?` too: picking a name fills the hole
	}
	if !cx.fresh {
		return nil, true
	}
	var want types.Type
	if h != nil {
		want = cx.a.res.Types[h]
	}
	var out []completionItem
	for _, it := range cx.names(n) {
		it.SortText = "1" + it.Label
		if want != nil && cx.fits(it.Label, want) {
			it.SortText = "0" + it.Label
		}
		if h != nil && h.Span.Start.Line == cx.pos.Line && h.Span.End.Line == cx.pos.Line {
			it.TextEdit.Range = Range{Start: Position{cx.line, h.Span.Start.Col - 1}, End: Position{cx.line, max(h.Span.End.Col-1, cx.col)}}
		}
		out = append(out, it)
	}
	return out, true
}

// holeAt is the build's hole the cursor is in or just after, when the cursor's line is
// unchanged since the build; nil otherwise.
func (cx *completion) holeAt() *syntax.HoleExpr {
	if !cx.same {
		return nil
	}
	var found *syntax.HoleExpr
	var walk func(n syntax.Spanned)
	walk = func(n syntax.Spanned) {
		for _, c := range syntax.Children(n) {
			if found != nil {
				return
			}
			if !within(c.GetSpan(), cx.pos) {
				continue
			}
			if h, ok := c.(*syntax.HoleExpr); ok {
				found = h
				return
			}
			walk(c)
		}
	}
	walk(cx.m)
	return found
}

// within reports whether pos is in sp or at its end.
func within(sp diag.Span, pos diag.Pos) bool {
	return !posLess(pos, sp.Start) && !posLess(sp.End, pos)
}

// fits reports whether the name (a local in scope, else a value of the module) has a type that
// fits want. It unifies on a clone of the build's unifier, which the build keeps unchanged.
func (cx *completion) fits(name string, want types.Type) bool {
	var t types.Type
	if b := innermost(cx.scope(), name); b != nil {
		t = cx.a.res.Binds[b.Key]
	} else if v, ok := cx.a.res.Values[cx.m.Name+"."+name]; ok {
		t = v
	} else {
		t = cx.ctorType(cx.m.Name + "." + name)
	}
	if t == nil {
		return false
	}
	u := cx.a.res.U.Clone()
	return u.Check(u.Instantiate(t), want) == nil
}

// names is the locals in scope (innermost first) and the module's declarations, each name once.
func (cx *completion) names(n int) []completionItem {
	seen := map[string]bool{}
	var out []completionItem
	add := func(it completionItem) {
		if !seen[it.Label] {
			seen[it.Label] = true
			out = append(out, it)
		}
	}
	bs := cx.scope()
	for i := len(bs) - 1; i >= 0; i-- {
		add(cx.valueItem(bs[i].Name, cx.a.res.Binds[bs[i].Key], n))
	}
	for _, it := range cx.declItems(cx.m, n) {
		add(it)
	}
	return out
}

// patterns: with the cursor on an arm's pattern in the build, the constructors of the union the
// match is on.
func (cx *completion) patterns() ([]completionItem, bool) {
	if !cx.same {
		return nil, false
	}
	path := syntax.NodeAt(cx.m, cx.pos)
	for i := len(path) - 1; i > 0; i-- {
		arm, ok := path[i].(*syntax.Arm)
		if !ok {
			continue
		}
		if arm.Pat == nil || !within(arm.Pat.GetSpan(), cx.pos) {
			return nil, false
		}
		var subject syntax.Expr
		switch mt := path[i-1].(type) {
		case *syntax.Match:
			subject = mt.Subject
		case *syntax.HtmlMatch:
			subject = mt.Subject
		}
		t, ok := cx.a.res.Types[subject]
		if subject == nil || !ok || t == nil {
			return nil, false
		}
		un, ok := cx.a.res.U.Apply(t).(*types.Union)
		if !ok {
			return nil, false
		}
		n := len(reWord.FindString(cx.prefix))
		var out []completionItem
		for _, c := range un.Constructors() {
			out = append(out, cx.ctorItem(un.Module, c.Name, n))
		}
		return out, true
	}
	return nil, false
}

// otherwise: the locals in scope, the module's declarations, the imported modules (by their
// qualifiers, and an imported page also by its full path for its page value), the standard
// library's modules and the keywords. Nothing right after a `.`, where none of them
// can follow.
func (cx *completion) otherwise() []completionItem {
	word := reWord.FindString(cx.prefix)
	n := len(word)
	if strings.HasSuffix(cx.prefix[:len(cx.prefix)-n], ".") {
		return nil
	}
	var out []completionItem
	seen := map[string]bool{}
	add := func(it completionItem) {
		if !seen[it.Label] {
			seen[it.Label] = true
			out = append(out, it)
		}
	}
	if cx.m != nil {
		for _, it := range cx.names(n) {
			add(it)
		}
		quals := cx.a.prog.Qualifiers(cx.m.Name)
		for _, im := range cx.m.Imports {
			if q := quals[im.Module]; q != "" {
				add(cx.item(q, kindModule, "", n)) // the spelling the compiler accepts (rule 5)
			} else {
				add(cx.item(im.Module, kindModule, "", n)) // no usable qualifier (E0206)
			}
			if cx.a.prog.IsPageModule(im.Module) { // a page value keeps its full path (rule 6)
				add(cx.item(im.Module, kindModule, "", n))
			}
		}
		var std []string
		for name := range cx.a.prog.Std {
			std = append(std, name)
		}
		sort.Strings(std)
		for _, name := range std {
			add(cx.item(name, kindModule, "", n))
		}
	}
	for _, k := range syntax.Keywords() {
		add(cx.item(k, kindKeyword, "", n))
	}
	return out
}
