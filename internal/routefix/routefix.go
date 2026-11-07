// Package routefix rewrites a module written with `page` and `msg` into routes (#160; spec
// docs/superpowers/specs/2026-11-03-routes-design.md, "Migration"; plan calls 13 and 14):
//
//   - `page "/x" { … } requires u` → `route <Name> = GET "/x" { … } requires u`, where Name is the
//     module's last segment (Options.Name overrides it);
//   - `load` and `view` → `Name.load` and `Name.view`, `Params` → `Name.Params`; `type Model` stays;
//   - each `msg M …` → `route M = POST "/x/_/M" { <the page's path parameters>, <payload> } [form F]
//     [requires …]`, and `M.update` holds the arm of `update` that answered M;
//   - `Rerender` → `Response.refresh`, `Redirect p` → `Response.redirect p`, `Reject t` →
//     `Response.invalid t`, `Notice.with t n` → `n |> Response.toast t` (and `success`, `warning`,
//     `error`); `Nav` → `Response`; `Html Msg` and `Html a` → `Html`, `Msg` → `Route`;
//   - the module's own page value `Todos.Index {…}` → `Index {…}` (inside a module its own names
//     are written unqualified); another page module of Options.Pages `Todos.Show {…}` →
//     `Todos.Show.Show {…}` (plan call 14);
//   - an arm that read the page's model gets a `?model` hole for each read and the comment
//     `-- the page's model is gone: render, refresh or query (#160)`; a message whose route gained
//     the page's path parameters gets a hole for each of them where the module names it.
//
// Tests are left as they are written (Task 7.2 rewrites them). The edits splice the source
// along the syntax tree's spans, so comments and layout outside the rewritten nodes are kept;
// `format.Source` then lays the result out as `alder fmt` does (route lines grouped).
package routefix

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"alder/internal/diag"
	"alder/internal/format"
	"alder/internal/syntax"
)

// Options steer the rewrite.
type Options struct {
	// Name is the GET route's name (`alder fix --routes --name N`); "" takes the module's last
	// segment.
	Name string
	// Pages are the page modules of the app, by path (`Todos.Show`), whose routes take their
	// module's last segment: a reference to one becomes `Todos.Show.Show` (plan call 14). Nil
	// leaves references to other modules as they are.
	Pages map[string]bool
}

// Hole is a typed hole the rewrite left, on line Line of the output (1-based), and why.
type Hole struct {
	Line int
	Why  string
}

// ErrNameCollision is the rewrite's refusal when the route's name is taken in the module (a
// message, a type, a constructor, a form, a table, a component or an import's qualifier):
// `alder fix --routes` then asks for `--name`.
type ErrNameCollision struct{ Name string }

func (e *ErrNameCollision) Error() string {
	return fmt.Sprintf("the route would be named `%s`, which the module already uses: choose another name with --name", e.Name)
}

// modelGone is the comment above an `update` whose arm read the page's model, and the end of
// its holes' reason.
const modelGone = "the page's model is gone: render, refresh or query (#160)"

// Module rewrites one module's source (file names it in errors). A module with no `page` is
// returned as it is, with no holes.
func Module(src []byte, file string, opts Options) ([]byte, []Hole, error) {
	m, diags := syntax.ParseFile(file, src)
	if len(diags) > 0 {
		return nil, nil, fmt.Errorf("%s does not parse: %s", file, diags[0].Message)
	}
	r := &rewriter{src: src, m: m, opts: opts, extra: map[string][]*syntax.Field{}}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.PageDecl:
			r.page = d
		case *syntax.MsgDecl:
			r.msgs = append(r.msgs, d)
		}
	}
	if r.page == nil {
		return src, nil, nil
	}
	r.route = opts.Name
	if r.route == "" {
		r.route = m.Name[strings.LastIndex(m.Name, ".")+1:]
	}
	r.local = taken(m, false)
	if taken(m, true)[r.route] {
		return nil, nil, &ErrNameCollision{Name: r.route}
	}
	r.lineOff = []int{0}
	for i, c := range src {
		if c == '\n' {
			r.lineOff = append(r.lineOff, i+1)
		}
	}
	inPath := map[string]bool{}
	for _, seg := range regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`).FindAllStringSubmatch(r.page.Path, -1) {
		inPath[seg[1]] = true
	}
	for _, f := range r.page.Params.Fields {
		if inPath[f.Name] {
			r.pathParams = append(r.pathParams, f)
		}
	}
	for _, md := range r.msgs {
		for _, f := range r.pathParams {
			if !hasField(md.Params, f.Name) {
				r.extra[md.Name] = append(r.extra[md.Name], f)
			}
		}
	}
	pre := r.splice(0, len(src), declNodes(m.Decls))
	if r.err != nil {
		return nil, nil, r.err
	}
	return r.finish(file, pre)
}

// rewriter holds one module's rewrite.
type rewriter struct {
	src        []byte
	lineOff    []int // the byte offset of each line's start
	m          *syntax.Module
	opts       Options
	page       *syntax.PageDecl
	route      string
	msgs       []*syntax.MsgDecl
	local      map[string]bool // the names the module declares
	pathParams []*syntax.Field
	// extra are, per message, the page's path parameters its route gains that its payload does
	// not have: a reference to the message must now pass them.
	extra map[string][]*syntax.Field
	// modelVar is the name of `update`'s model parameter while an arm is written, else "".
	modelVar string
	// msg is the message whose `update` is being written, else ""; matches are `update`'s
	// matches on its message, and params the name each message's parameters take.
	msg     string
	matches map[*syntax.Match]*msgMatch
	params  map[string]string
	whys    []string // each hole's reason, by the number in its temporary name
	err     error
}

func (r *rewriter) off(p diag.Pos) int {
	if p.Line < 1 || p.Line > len(r.lineOff) {
		return len(r.src)
	}
	return min(r.lineOff[p.Line-1]+p.Col-1, len(r.src))
}

func (r *rewriter) raw(sp diag.Span) string { return string(r.src[r.off(sp.Start):r.off(sp.End)]) }

// insertion is text to put at a position of the source, spliced like a node of no width.
type insertion struct {
	at   diag.Pos
	text string
}

func (i *insertion) GetSpan() diag.Span { return diag.Span{Start: i.at, End: i.at} }

// splice is the source from start to end with each of kids (in source order, inside the range)
// replaced by its rewritten text.
func (r *rewriter) splice(start, end int, kids []syntax.Spanned) string {
	slices.SortStableFunc(kids, func(a, b syntax.Spanned) int { return r.off(a.GetSpan().Start) - r.off(b.GetSpan().Start) })
	var b strings.Builder
	at := start
	for _, k := range kids {
		ks, ke := r.off(k.GetSpan().Start), r.off(k.GetSpan().End)
		if in, ok := k.(*insertion); ok {
			if ks < at || ks > end {
				continue
			}
			b.Write(r.src[at:ks])
			b.WriteString(in.text)
			at = ks
			continue
		}
		if ks < at || ke > end || ks >= ke {
			continue
		}
		b.Write(r.src[at:ks])
		b.WriteString(r.text(k))
		at = ke
	}
	b.Write(r.src[at:end])
	return b.String()
}

// text is node n rewritten.
func (r *rewriter) text(n syntax.Spanned) string {
	if s, ok := r.replace(n); ok {
		return s
	}
	sp := n.GetSpan()
	return r.splice(r.off(sp.Start), r.off(sp.End), syntax.Children(n))
}

// hole is a hole named name with its reason, under a temporary name finish takes off again.
func (r *rewriter) hole(name, why string) string {
	r.whys = append(r.whys, why)
	return "?" + name + "__rf" + strconv.Itoa(len(r.whys)-1)
}

// replace is the rewrite of n when n itself changes (its children are rewritten through text).
func (r *rewriter) replace(n syntax.Spanned) (string, bool) {
	switch n := n.(type) {
	case *syntax.TestDecl:
		return r.raw(n.Span), true // Task 7.2 rewrites tests
	case *syntax.PageDecl:
		return "route " + r.route + " = GET" + r.raw(n.Span)[len("page"):], true
	case *syntax.MsgDecl:
		return r.msgRoute(n), true
	case *syntax.ValueDecl:
		if n.Route != "" {
			return "", false
		}
		switch n.Name {
		case "update":
			return r.updates(n), true
		case "load", "view":
			return r.qualify(n), true
		}
	case *syntax.TypeName:
		if n.Module != "" {
			return "", false
		}
		switch {
		case n.Name == "Params" && len(n.Args) == 0:
			return r.route + ".Params", true
		case n.Name == "Nav" && len(n.Args) == 0:
			return "Response", true
		case n.Name == "Msg" && len(n.Args) == 0:
			return "Route", true
		case n.Name == "Html" && len(n.Args) == 1:
			switch a := n.Args[0].(type) {
			case *syntax.TypeVar:
				return "Html", true
			case *syntax.TypeName:
				if a.Module == "" && a.Name == "Msg" && len(a.Args) == 0 {
					return "Html", true
				}
			}
		}
	case *syntax.CtorRef:
		return r.ctor(n)
	case *syntax.Var:
		if n.Module == "Notice" {
			if b, ok := noticeBuilder(n.Name); ok {
				return b, true
			}
		}
		if n.Module == "" && r.modelVar != "" && n.Name == r.modelVar {
			return r.hole("model", "`"+r.raw(n.Span)+"`: "+modelGone), true
		}
		if n.Module == "" && (n.Name == "load" || n.Name == "view") {
			return r.route + "." + n.Name, true
		}
	case *syntax.Access:
		if r.modelVar != "" && readsModel(n, r.modelVar) {
			return r.hole("model", "`"+r.raw(n.Span)+"`: "+modelGone), true
		}
	case *syntax.App:
		return r.app(n)
	case *syntax.Match:
		if mm := r.matches[n]; mm != nil && r.msg != "" {
			return r.armBody(mm), true
		}
	}
	return "", false
}

// readsModel reports whether a is a field of the variable model (`model.todo.id`).
func readsModel(a *syntax.Access, model string) bool {
	var e syntax.Expr = a
	for {
		switch x := e.(type) {
		case *syntax.Access:
			e = x.Expr
		case *syntax.Var:
			return x.Module == "" && x.Name == model
		default:
			return false
		}
	}
}

// noticeBuilder is the Response builder that replaces `Notice.name`.
func noticeBuilder(name string) (string, bool) {
	switch name {
	case "with":
		return "Response.toast", true
	case "success", "warning", "error":
		return "Response." + name, true
	}
	return "", false
}

// ctor rewrites a constructor: the answers of `update`, a page value, a message that must now
// pass the page's path parameters.
func (r *rewriter) ctor(n *syntax.CtorRef) (string, bool) {
	if n.Module == "" {
		switch n.Name {
		case "Rerender":
			return "Response.refresh", true
		case "Redirect":
			return "Response.redirect", true
		case "Reject":
			return "Response.invalid", true
		}
		if fs := r.extra[n.Name]; len(fs) > 0 {
			return n.Name + " " + r.withPathParams(n.Name, "{ }", fs), true
		}
		if n.Name == r.m.Name && n.Name != r.route && !r.isMsg(n.Name) {
			return r.route, true
		}
		if r.opts.Pages[n.Name] && n.Name != r.m.Name && !r.local[n.Name] { // a page module of one segment
			return n.Name + "." + n.Name, true
		}
		return "", false
	}
	path := n.Module + "." + n.Name
	if path == r.m.Name {
		return r.route, true
	}
	if r.opts.Pages[path] {
		return path + "." + n.Name, true
	}
	return "", false
}

func (r *rewriter) isMsg(name string) bool {
	return slices.ContainsFunc(r.msgs, func(md *syntax.MsgDecl) bool { return md.Name == name })
}

// withPathParams is the record literal rec (as rewritten) with a hole for each of fs added.
func (r *rewriter) withPathParams(msg, rec string, fs []*syntax.Field) string {
	body := strings.TrimSpace(rec)
	body = strings.TrimSpace(strings.TrimSuffix(body, "}"))
	body = strings.TrimSuffix(body, ",")
	empty := strings.TrimSpace(strings.TrimPrefix(body, "{")) == ""
	var add []string
	for _, f := range fs {
		add = append(add, f.Name+": "+r.hole(f.Name, fmt.Sprintf("`%s` takes the page's path parameter `%s`: pass it from the model (#160)", msg, f.Name)))
	}
	if empty {
		return "{ " + strings.Join(add, ", ") + " }"
	}
	return body + ", " + strings.Join(add, ", ") + " }"
}

// app rewrites an application: `Notice.with t n` (and the variants) → `n |> Response.toast t`;
// a message applied to its payload, when its route gained the page's path parameters.
func (r *rewriter) app(n *syntax.App) (string, bool) {
	switch fn := n.Fn.(type) {
	case *syntax.Var:
		b, ok := noticeBuilder(fn.Name)
		if fn.Module != "Notice" || !ok || len(n.Args) != 2 {
			return "", false
		}
		return paren(n.Args[1], r.text(n.Args[1])) + " |> " + b + " " + paren(n.Args[0], r.text(n.Args[0])), true
	case *syntax.CtorRef:
		fs := r.extra[fn.Name]
		if fn.Module != "" || len(fs) == 0 || len(n.Args) != 1 {
			return "", false
		}
		rec, ok := n.Args[0].(*syntax.RecordLit)
		if !ok { // a payload passed whole: left for the checker to point at
			return r.raw(n.Span), true
		}
		return fn.Name + " " + r.withPathParams(fn.Name, r.text(rec), fs), true
	}
	return "", false
}

// paren is text (e's rewrite) in parentheses unless e is atomic; `alder fmt` drops the ones the
// precedence does not need.
func paren(e syntax.Expr, text string) string {
	switch e.(type) {
	case *syntax.StrLit, *syntax.Var, *syntax.CtorRef, *syntax.IntLit, *syntax.FloatLit, *syntax.BoolLit,
		*syntax.RecordLit, *syntax.ListLit, *syntax.HoleExpr, *syntax.Access, *syntax.UnitLit:
		return text
	}
	return "(" + text + ")"
}

// msgRoute is the route line of message md.
func (r *rewriter) msgRoute(md *syntax.MsgDecl) string {
	path := strings.TrimSuffix(r.page.Path, "/") + "/_/" + md.Name
	var fields []string
	for _, f := range r.pathParams {
		if !hasField(md.Params, f.Name) {
			fields = append(fields, r.text(f))
		}
	}
	if md.Params != nil {
		for _, f := range md.Params.Fields {
			fields = append(fields, r.text(f))
		}
	}
	line := fmt.Sprintf("route %s = POST %q ", md.Name, path)
	if len(fields) == 0 {
		line += "{ }"
	} else {
		line += "{ " + strings.Join(fields, ", ") + " }"
	}
	if md.Form != "" {
		line += " form " + md.Form
	}
	if g := r.guard(); g != "" {
		line += " " + g
	}
	return line
}

// guard is the page's `requires …` clause as written, "" for none.
func (r *rewriter) guard() string {
	return strings.TrimSpace(string(r.src[r.off(r.page.Params.Span.End):r.off(r.page.Span.End)]))
}

func hasField(rt *syntax.RecordType, name string) bool {
	return rt != nil && slices.ContainsFunc(rt.Fields, func(f *syntax.Field) bool { return f.Name == name })
}

// qualify is the declaration `load` or `view` as `Route.load` or `Route.view`.
func (r *rewriter) qualify(v *syntax.ValueDecl) string {
	kids := syntax.Children(v)
	if v.Sig != nil { // the definition's line names it again: `load params session = …`
		from := r.off(v.Sig.GetSpan().End)
		to := r.off(v.Span.End)
		if len(v.Params) > 0 {
			to = r.off(v.Params[0].Span.Start)
		}
		// the first line of the gap that starts with the name (a comment between may mention it)
		gap := string(r.src[from:to])
		for i := 0; i < len(gap); {
			rest := gap[i:]
			if strings.HasPrefix(rest, v.Name) && (i == 0 || gap[i-1] == '\n') {
				kids = append(kids, &insertion{at: r.posOf(from + i), text: r.route + "."})
				break
			}
			nl := strings.IndexByte(rest, '\n')
			if nl < 0 {
				break
			}
			i += nl + 1
		}
	}
	s := r.splice(r.off(v.Span.Start), r.off(v.Span.End), kids)
	return r.route + "." + s
}

// posOf is the position of byte offset o.
func (r *rewriter) posOf(o int) diag.Pos {
	l := sort.Search(len(r.lineOff), func(i int) bool { return r.lineOff[i] > o })
	return diag.Pos{Line: l, Col: o - r.lineOff[l-1] + 1}
}

// msgMatch is a `match msg { … }` in `update`: per message the arm that answers it (or the `_`
// arm), and the comments of the match outside its arms' bodies, by the arm they lead.
type msgMatch struct {
	arms  map[string]*syntax.Arm
	wild  *syntax.Arm
	order []string // the messages, in the order of the arms (a `_` arm's in declaration order)
	first string   // the first message the `_` arm answers: its comments lead that one
	lead  map[*syntax.Arm][]string
	trail map[*syntax.Arm]string // a comment on the line an arm ends
	last  []string               // the comments after the last arm
}

// updates is the module's `update` written as one `M.update` per message. Each is `update`'s
// body with every `match msg { … }` replaced by the arm that answers M, so an `update` that
// checks something before matching (a generated bulk action) keeps it in every message's route.
// The comments between the arms lead the `M.update` of the arm after them.
func (r *rewriter) updates(v *syntax.ValueDecl) string {
	if len(v.Params) != 3 {
		r.err = fmt.Errorf("`update` takes %d parameters, not a message, the model and the session: rewrite it by hand", len(v.Params))
		return r.raw(v.Span)
	}
	msgVar, model, session := v.Params[0].Name, v.Params[1].Name, v.Params[2].Name
	r.matches = map[*syntax.Match]*msgMatch{}
	var order []string
	var stray *syntax.Var
	var walk func(n syntax.Spanned)
	walk = func(n syntax.Spanned) {
		if mt, ok := n.(*syntax.Match); ok {
			if s, ok := mt.Subject.(*syntax.Var); ok && s.Module == "" && s.Name == msgVar {
				mm := r.msgMatch(mt)
				if mm == nil {
					return
				}
				r.matches[mt] = mm
				if order == nil {
					order = mm.order
				}
				for _, a := range mt.Arms {
					walk(a.Body)
				}
				return
			}
		}
		if x, ok := n.(*syntax.Var); ok && x.Module == "" && x.Name == msgVar && stray == nil {
			stray = x
		}
		for _, c := range syntax.Children(n) {
			walk(c)
		}
	}
	walk(v.Body)
	if r.err != nil {
		return r.raw(v.Span)
	}
	if stray != nil {
		r.err = fmt.Errorf("line %d: `update` uses `%s` other than in `match %s { … }`: rewrite it by hand", stray.Span.Start.Line, msgVar, msgVar)
		return r.raw(v.Span)
	}
	for _, md := range r.msgs {
		if !slices.Contains(order, md.Name) {
			order = append(order, md.Name)
		}
	}
	// the name each message's parameters take: what its first arm binds them to
	r.params = map[string]string{}
	for _, mm := range r.matches {
		for name, a := range mm.arms {
			if c, ok := a.Pat.(*syntax.PCtor); ok {
				if pv, ok := c.Arg.(*syntax.PVar); ok && r.params[name] == "" {
					r.params[name] = pv.Name
				}
			}
		}
	}
	// the comments of `update` outside every match on the message (in its signature, say) are
	// in every message's body already; the ones a match leaves out go to the arms
	var decls []string
	var head, last []string
	for _, c := range r.m.Comments { // between the signature and the body: they lead the first
		if c.Offset >= r.off(v.Span.Start) && c.Offset < r.off(v.Body.GetSpan().Start) {
			head = append(head, c.Text)
		}
	}
	for _, mm := range r.matchesInOrder() {
		last = append(last, mm.last...)
	}
	for i, name := range order {
		param := r.params[name]
		if param == "" {
			param = "params"
		}
		var lead []string
		if i == 0 {
			lead = head
		}
		trail := ""
		for _, mm := range r.matchesInOrder() {
			a := mm.arms[name]
			if a == nil && name == mm.first {
				a = mm.wild
			}
			if a != nil {
				lead = append(lead, mm.lead[a]...)
				mm.lead[a] = nil
				if mm.trail[a] != "" {
					trail, mm.trail[a] = mm.trail[a], ""
				}
			}
		}
		before := len(r.whys)
		r.msg, r.modelVar = name, model
		body := r.text(v.Body)
		r.msg, r.modelVar = "", ""
		var b strings.Builder
		for _, c := range lead {
			b.WriteString(c + "\n")
		}
		for _, w := range r.whys[before:] {
			if strings.HasSuffix(w, modelGone) {
				b.WriteString("-- " + modelGone + "\n")
				break
			}
		}
		fmt.Fprintf(&b, "%s.update : %s.Params -> Session -> Task Response\n", name, name)
		fmt.Fprintf(&b, "%s.update %s %s = %s", name, param, session, body)
		if trail != "" {
			b.WriteString(" " + trail)
		}
		decls = append(decls, b.String())
	}
	text := strings.Join(decls, "\n\n")
	if len(last) > 0 {
		text += "\n\n" + strings.Join(last, "\n")
	}
	return text
}

// matchesInOrder are the matches on the message, in source order.
func (r *rewriter) matchesInOrder() []*msgMatch {
	var ms []*syntax.Match
	for m := range r.matches {
		ms = append(ms, m)
	}
	slices.SortFunc(ms, func(a, b *syntax.Match) int { return r.off(a.Span.Start) - r.off(b.Span.Start) })
	out := make([]*msgMatch, len(ms))
	for i, m := range ms {
		out[i] = r.matches[m]
	}
	return out
}

// msgMatch reads a `match msg { … }`; nil (with r.err set) when an arm is not a message's name
// or `_`, names something else, or a message twice.
func (r *rewriter) msgMatch(mt *syntax.Match) *msgMatch {
	mm := &msgMatch{arms: map[string]*syntax.Arm{}, lead: map[*syntax.Arm][]string{}, trail: map[*syntax.Arm]string{}}
	for _, a := range mt.Arms {
		switch p := a.Pat.(type) {
		case *syntax.PCtor:
			if !r.isMsg(p.Name) {
				r.err = fmt.Errorf("line %d: `update` has an arm for `%s`, which is not a message of this page", a.Span.Start.Line, p.Name)
				return nil
			}
			if mm.arms[p.Name] != nil {
				r.err = fmt.Errorf("line %d: `update` has more than one arm for `%s`: merge them by hand first", a.Span.Start.Line, p.Name)
				return nil
			}
			mm.arms[p.Name] = a
			mm.order = append(mm.order, p.Name)
		case *syntax.PWild:
			mm.wild = a
			for _, md := range r.msgs {
				if mm.arms[md.Name] == nil {
					if mm.first == "" {
						mm.first = md.Name
					}
					mm.order = append(mm.order, md.Name)
				}
			}
		default:
			r.err = fmt.Errorf("line %d: an arm of `update` matches `%s`: only a message's name or `_` can be rewritten", a.Span.Start.Line, r.raw(a.Pat.GetSpan()))
			return nil
		}
	}
	// the comments in the match and outside its arms' bodies
	start, end := r.off(mt.Span.Start), r.off(mt.Span.End)
	k := 0
	var prev *syntax.Arm
	for _, c := range r.m.Comments {
		if c.Offset < start || c.Offset >= end {
			continue
		}
		for k < len(mt.Arms) && c.Offset >= r.off(mt.Arms[k].Body.GetSpan().End) {
			prev = mt.Arms[k]
			k++
		}
		if k < len(mt.Arms) && c.Offset >= r.off(mt.Arms[k].Body.GetSpan().Start) {
			continue // inside an arm's body: it goes with the body
		}
		switch {
		case prev != nil && c.Line == prev.Span.End.Line:
			mm.trail[prev] = c.Text
		case k < len(mt.Arms):
			mm.lead[mt.Arms[k]] = append(mm.lead[mt.Arms[k]], c.Text)
		default:
			mm.last = append(mm.last, c.Text)
		}
	}
	return mm
}

// armBody is the rewrite of a match on the message, for the message being written: the body of
// the arm that answers it, its payload bound to the parameters' name.
func (r *rewriter) armBody(mm *msgMatch) string {
	a := mm.arms[r.msg]
	if a == nil {
		a = mm.wild
	}
	if a == nil {
		return r.hole("update", fmt.Sprintf("`update` has no arm for `%s` (#160)", r.msg))
	}
	body := r.text(a.Body)
	param := r.params[r.msg]
	if param == "" {
		param = "params"
	}
	c, ok := a.Pat.(*syntax.PCtor)
	if !ok {
		return body
	}
	switch p := c.Arg.(type) {
	case nil, *syntax.PWild:
		return body
	case *syntax.PVar:
		if p.Name == param {
			return body
		}
		return "{\nlet " + p.Name + " = " + param + "\n" + body + "\n}"
	default:
		return "match " + param + " {\n" + r.raw(p.GetSpan()) + " -> " + body + ",\n}"
	}
}

// declNodes is ds as nodes.
func declNodes(ds []syntax.Decl) []syntax.Spanned {
	out := make([]syntax.Spanned, len(ds))
	for i, d := range ds {
		out[i] = d
	}
	return out
}

// taken are the names module m declares (with imports, and the qualifiers it imports under),
// that a route may not take.
func taken(m *syntax.Module, imports bool) map[string]bool {
	t := map[string]bool{}
	for _, im := range m.Imports {
		if !imports {
			break
		}
		q := im.Alias
		if q == "" {
			q = im.Module[strings.LastIndex(im.Module, ".")+1:]
		}
		t[q] = true
	}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.MsgDecl:
			t[d.Name] = true
		case *syntax.TypeDecl:
			t[d.Name] = true
			for _, c := range d.Union {
				t[c.Name] = true
			}
		case *syntax.FormDecl:
			t[d.Name] = true
		case *syntax.TableDecl:
			t[d.Name] = true
		case *syntax.CapDecl:
			t[d.Name] = true
		case *syntax.ValueDecl:
			if d.Route == "" {
				t[d.Name] = true
			}
		}
	}
	return t
}

var tempHole = regexp.MustCompile(`(\?[A-Za-z0-9_]*?)__rf[0-9]+`)

// finish formats the rewritten source pre and finds where its holes landed: the holes of pre
// and of the output are the same, in the same order (the formatter moves route lines, which
// hold none).
func (r *rewriter) finish(file, pre string) ([]byte, []Hole, error) {
	pm, diags := syntax.ParseFile(file, []byte(pre))
	if len(diags) > 0 {
		return nil, nil, fmt.Errorf("internal error: %s rewritten does not parse (line %d): %s", file, diags[0].Span.Start.Line, diags[0].Message)
	}
	var mine []int // per hole of pre, its reason's index, -1 for one the module had
	for _, h := range holes(pm) {
		i := -1
		if k := strings.LastIndex(h.Name, "__rf"); k >= 0 {
			if n, err := strconv.Atoi(h.Name[k+4:]); err == nil {
				i = n
			}
		}
		mine = append(mine, i)
	}
	out, diags, err := format.Source(file, []byte(tempHole.ReplaceAllString(pre, "$1")))
	if err != nil {
		return nil, nil, err
	}
	if len(diags) > 0 {
		return nil, nil, fmt.Errorf("internal error: %s rewritten does not format: %s", file, diags[0].Message)
	}
	om, _ := syntax.ParseFile(file, out)
	got := holes(om)
	if len(got) != len(mine) {
		return nil, nil, fmt.Errorf("internal error: %s: %d holes before formatting, %d after", file, len(mine), len(got))
	}
	var hs []Hole
	for k, h := range got {
		if mine[k] >= 0 {
			hs = append(hs, Hole{Line: h.Span.Start.Line, Why: r.whys[mine[k]]})
		}
	}
	return out, hs, nil
}

// holes are m's holes in source order.
func holes(m *syntax.Module) []*syntax.HoleExpr {
	var out []*syntax.HoleExpr
	var walk func(n syntax.Spanned)
	walk = func(n syntax.Spanned) {
		if h, ok := n.(*syntax.HoleExpr); ok {
			out = append(out, h)
		}
		for _, c := range syntax.Children(n) {
			walk(c)
		}
	}
	for _, d := range m.Decls {
		walk(d)
	}
	return out
}
