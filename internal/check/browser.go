// The browser steps of module Test (`Test.login`, `get`, `open`, `fetch`, `submit`, `submitRaw`,
// `click`), typed as special forms, the selector literals of its queries (E0023), and the
// test-only Prelude types `Doc` and `Answer`.
package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
	"alder/rt/selector"
)

// BadSelector is E0023: a selector literal of a `Test` query that does not parse.
const BadSelector = diag.BadSelector

// testSteps are the browser steps and how many arguments each takes.
var testSteps = map[string]int{"login": 1, "get": 1, "open": 1, "fetch": 1, "submit": 2, "submitRaw": 2, "click": 1}

// IsTestStep reports whether `Test.name` is a browser step (a special form).
func IsTestStep(name string) bool { _, ok := testSteps[name]; return ok }

// stepExample is how each step is applied, for the message of a step used as a value.
var stepExample = map[string]string{
	"login": "Test.login { user: Just id }", "get": "Test.get (Todos.Index {})", "open": "Test.open (Todos.Index {})",
	"fetch":  "Test.fetch (Report.Csv { id: id })",
	"submit": "Test.submit NewTodo { title: \"Milk\" }", "submitRaw": "Test.submitRaw NewTodo [{ name: \"title\", value: \"\" }]",
	"click": "Test.click (Toggle { id: id })",
}

// selectorArg is, for each query, the index of its selector argument.
var selectorArg = map[string]int{"text": 0, "texts": 0, "has": 0, "count": 0, "attr": 1}

// answerCtors are the constructors of the Prelude's test type `Answer`.
var answerCtors = map[string]bool{"Shown": true, "Rerendered": true, "Redirected": true, "FormRejected": true, "PageMissing": true, "RequestFailed": true}

// testTypes are the Prelude types that exist only inside a `test`.
var testTypes = map[string]bool{"Doc": true, "Answer": true}

// testTypeUse reports E0003 for the test type name (Doc, Answer) written at span outside a
// `test`; each place once (a type may be converted more than once).
func (mc *modCtx) testTypeUse(name string, span diag.Span) {
	c := mc.c
	if mc.inTest || c.prog.Std[mc.m.Name] != nil || c.fakeUses[span] {
		return
	}
	if mc.value != "" || mc.fake != "" || mc.never != "" { // a value's signature or body: it may be a helper
		mc.testUse("`"+name+"`", span)
		return
	}
	c.fakeUses[span] = true
	c.errorf(diag.TypeMismatch, span, "`%s` is a test type: it exists only inside a `test`", name)
}

// stepResult is a step's result type: Task { <first>: t, session: Session, doc: Doc }.
func (mc *modCtx) stepResult(first string, t types.Type) types.Type {
	rec := &types.Record{Anonymous: true, Fields: []types.Field{
		{Name: first, Type: t}, {Name: "session", Type: mc.c.appSession()}, {Name: "doc", Type: mc.c.types["Prelude.Doc"]},
		{Name: "notices", Type: &types.App{Con: "List", Args: []types.Type{types.Text}}},
	}}
	return &types.App{Con: "Task", Args: []types.Type{rec}}
}

// fetchResult is Test.fetch's result type: Task { status: Int, contentType: Text, body: Bytes,
// headers: List { name: Text, value: Text } }, the raw answer.
func fetchResult() types.Type {
	header := &types.Record{Anonymous: true, Fields: []types.Field{{Name: "name", Type: types.Text}, {Name: "value", Type: types.Text}}}
	rec := &types.Record{Anonymous: true, Fields: []types.Field{
		{Name: "status", Type: types.Int}, {Name: "contentType", Type: types.Text},
		{Name: "body", Type: types.Bytes}, {Name: "headers", Type: &types.App{Con: "List", Args: []types.Type{header}}},
	}}
	return &types.App{Con: "Task", Args: []types.Type{rec}}
}

// appSession is App's session record, the empty anonymous record without an App module.
func (c *Checker) appSession() types.Type {
	if c.prog.Modules["App"] == nil {
		return &types.Record{Anonymous: true}
	}
	return c.session
}

// answerType is the Prelude's `Answer`.
func (c *Checker) answerType() types.Type {
	if t, ok := c.types["Prelude.Answer"]; ok {
		return t
	}
	return types.Err
}

// inferTestStep types the browser step `Test.<v>` applied in x: only inside a `test`, applied
// to all its arguments directly (not through `|>`); see each step below.
func (mc *modCtx) inferTestStep(x *syntax.App, v *syntax.Var, env *env, eff effect, pipe *pipeArg) types.Type {
	c := mc.c
	name := v.Ref.Name
	if !mc.testUse("`Test."+name+"`", v.Span) {
		mc.inferArgs(x.Args, env, eff)
		return types.Err
	}
	if pipe != nil {
		c.errorf(diag.TypeMismatch, pipe.span, "`Test.%s` takes its arguments directly, not through `|>`: `%s`", name, stepExample[name])
		mc.inferArgs(x.Args, env, eff)
		return types.Err
	}
	want := testSteps[name]
	if len(x.Args) != want {
		plural := "s"
		if want == 1 {
			plural = ""
		}
		c.errorf(diag.Arity, x.Span, "`Test.%s` takes %d argument%s, got %d: `%s`", name, want, plural, len(x.Args), stepExample[name])
		mc.inferArgs(x.Args, env, eff)
		return types.Err
	}
	if name != "login" && c.prog.Modules["App"] == nil {
		c.errorf(diag.TypeMismatch, v.Span, "`Test.%s` needs an `App` module: the browser requests the app's routes, which an app without `App` does not serve", name)
		mc.inferArgs(x.Args, env, eff)
		return types.Err
	}
	var res types.Type
	switch name {
	case "login":
		res = mc.testLogin(x.Args[0], env, eff)
	case "get", "open", "fetch":
		res = mc.testGet(name, x.Args[0], env, eff)
	case "submit", "submitRaw":
		res = mc.testSubmit(name, x.Args[0], x.Args[1], env, eff)
	case "click":
		res = mc.testClick(x.Args[0], env, eff)
	}
	if !types.IsErr(res) {
		c.record(v, &types.Fun{Params: mc.stepParams(name, x.Args), Result: res})
	}
	return res
}

// stepParams are the types the arguments of a step were checked at, for the record of the
// step's own type.
func (mc *modCtx) stepParams(name string, args []syntax.Expr) []types.Type {
	var ps []types.Type
	for i, a := range args {
		if (name == "submit" || name == "submitRaw") && i == 0 {
			ps = append(ps, types.Unit) // a form name: not a value
			continue
		}
		if t, ok := mc.c.res.Types[a]; ok {
			ps = append(ps, t)
		} else {
			ps = append(ps, types.Err)
		}
	}
	return ps
}

func (mc *modCtx) inferArgs(args []syntax.Expr, env *env, eff effect) {
	for _, a := range args {
		if ct, ok := a.(*syntax.CtorRef); ok && ct.Ref.Kind == syntax.RefType {
			continue // a form name
		}
		mc.infer(a, env, eff)
	}
}

// testLogin types `Test.login s`: s is App's session record; a Task Unit. s is the cookie's
// value, so a literal may leave out a field with a default (cookieLit).
func (mc *modCtx) testLogin(arg syntax.Expr, env *env, eff effect) types.Type {
	c := mc.c
	want := c.appSession()
	from := len(c.bag.Items)
	if lit, ok := arg.(*syntax.RecordLit); ok {
		mc.cookieLit = lit
		defer func() { mc.cookieLit = nil }()
	}
	mc.check(arg, want, env, eff)
	if len(c.bag.Items) > from {
		var fs []string
		if r, ok := want.(*types.Record); ok {
			for _, f := range r.Fields {
				fs = append(fs, f.Name+": "+mc.ts(f.Type))
			}
		}
		shape := "{ }"
		if len(fs) > 0 {
			shape = "{ " + strings.Join(fs, ", ") + " }"
		}
		for i := from; i < len(c.bag.Items); i++ {
			it := &c.bag.Items[i]
			if it.Severity == "error" {
				msg, _, _ := strings.Cut(it.Message, " (a default applies only to") // a literal of App's Session names every field
				it.Message = "`Test.login` wants the app's session `" + shape + "`: " + msg
			}
		}
	}
	return &types.App{Con: "Task", Args: []types.Type{types.Unit}}
}

// pageOf is the page module a page value written in place names: `Todos.Index {…}` or (a page
// without parameters) `Todos.Show`; "" for another expression, and ok false when the head is
// a name the resolver could not resolve (reported).
func pageOf(e syntax.Expr) (page string, ok bool) {
	head := e
	if app, isApp := e.(*syntax.App); isApp && len(app.Args) == 1 {
		head = app.Fn
	}
	ct, isCt := head.(*syntax.CtorRef)
	if !isCt {
		return "", true
	}
	if ct.Ref.Kind == syntax.RefNone {
		return "", false
	}
	if ct.Ref.Kind == syntax.RefPage {
		return ct.Ref.Module, true
	}
	return "", true
}

// testGet types `Test.get p`, `Test.open p` and `Test.fetch p`: p is a page value written in
// place (its head a page); get's result holds the page's `Model`, which must be a record (a
// union cannot be named in the result: E0003), open's the Answer, fetch's the raw answer
// (fetchResult). Only fetch may name a file page.
func (mc *modCtx) testGet(name string, arg syntax.Expr, env *env, eff effect) types.Type {
	c := mc.c
	page, ok := pageOf(arg)
	if !ok {
		return types.Err
	}
	if page == "" {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.%s` wants a page value such as `Todos.Index {}`, got `%s`", name, mc.source(arg.GetSpan()))
		mc.infer(arg, env, eff)
		return types.Err
	}
	mc.check(arg, types.Page, env, eff)
	if c.pages[page] == nil {
		return types.Err
	}
	if c.pages[page].File && name != "fetch" {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`%s` is a file page: use `Test.fetch`", page)
		return types.Err
	}
	if name == "fetch" {
		return fetchResult()
	}
	if name == "open" {
		return mc.stepResult("answer", c.answerType())
	}
	model, ok := c.types[c.key(page, "Model")]
	if !ok {
		return types.Err // a page without Model: E0010
	}
	if _, isUnion := model.(*types.Union); isUnion {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.get` needs a record `Model`: `%s`'s is a union — use `Test.open`", page)
		return types.Err
	}
	return mc.stepResult("model", model)
}

// stepForm is the form a `Test.submit` names and the message posting it.
type stepForm struct {
	module string // the form's module (a page, or App)
	form   *types.Record
	msg    string
}

// stepFormOf resolves the target of `Test.submit`/`submitRaw`: a form of a page module or of
// App that exactly one of its messages posts, or a message that posts a form, written with its
// parameters (`Save { id: id }`). ok is false when an error was reported (or the resolver
// reported one).
func (mc *modCtx) stepFormOf(step string, arg syntax.Expr, env *env, eff effect) (stepForm, bool) {
	c := mc.c
	head := arg
	if app, isApp := arg.(*syntax.App); isApp {
		head = app.Fn
	}
	ct, isCt := head.(*syntax.CtorRef)
	if isCt && ct.Ref.Kind == syntax.RefNone {
		mc.inferArgs([]syntax.Expr{arg}, env, eff)
		return stepForm{}, false
	}
	if isCt && ct.Ref.Kind == syntax.RefCtor {
		if info := c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)]; info != nil && c.isMessageUnion(info.Union) {
			form := mc.msgForm(ct.Ref.Module, ct.Ref.Name)
			if form == "" {
				c.errorf(diag.TypeMismatch, arg.GetSpan(), "`%s` posts no form: use `Test.click`", ct.Ref.Name)
				mc.infer(arg, env, eff)
				return stepForm{}, false
			}
			mc.check(arg, info.Union, env, eff)
			return stepForm{module: ct.Ref.Module, form: c.forms[c.key(ct.Ref.Module, form)], msg: ct.Ref.Name}, true
		}
	}
	if arg != head || !isCt || ct.Ref.Kind != syntax.RefType || c.forms[c.key(ct.Ref.Module, ct.Ref.Name)] == nil {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.%s` wants a form declared by a page, got `%s`", step, mc.source(arg.GetSpan()))
		if !isCt || ct.Ref.Kind != syntax.RefType {
			mc.infer(arg, env, eff)
		}
		return stepForm{}, false
	}
	c.record(ct, types.Unit) // a form's name is not a value (stepParams)
	mod := ct.Ref.Module
	if c.pages[mod] == nil && mod != "App" {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.%s` wants a form declared by a page: `%s` is a form of `%s`, which is not a page", step, ct.Name, mod)
		return stepForm{}, false
	}
	var posts []*syntax.MsgDecl
	if m := c.prog.Modules[mod]; m != nil {
		for _, d := range c.live(m) {
			if md, ok := d.(*syntax.MsgDecl); ok && md.Form == ct.Ref.Name {
				posts = append(posts, md)
			}
		}
	}
	switch len(posts) {
	case 0:
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.%s` wants a form a message posts: no `msg … form %s` in `%s`", step, ct.Ref.Name, mod)
		return stepForm{}, false
	case 1:
		return stepForm{module: mod, form: c.forms[c.key(mod, ct.Ref.Name)], msg: posts[0].Name}, true
	}
	var names []string
	for _, md := range posts {
		names = append(names, "`"+md.Name+"`")
	}
	list := strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	first := posts[0]
	write := first.Name
	if first.Params != nil && len(first.Params.Fields) > 0 {
		var fs []string
		for _, f := range first.Params.Fields {
			fs = append(fs, f.Name+": …")
		}
		write = "(" + first.Name + " { " + strings.Join(fs, ", ") + " })"
	}
	value := "{ … }"
	if step == "submitRaw" {
		value = "[…]"
	}
	d := c.errorf(diag.TypeMismatch, arg.GetSpan(), "`%s` is posted by %s: write `Test.%s %s %s`", ct.Name, list, step, write, value)
	if write == first.Name { // a message without parameters: the fix names it
		d.Fix = diag.ReplaceSpan(arg.GetSpan(), first.Name)
	}
	return stepForm{}, false
}

// testSubmit types `Test.submit F r` (r is F's record: a request value, so its defaults apply)
// and `Test.submitRaw F fields` (fields a List { name: Text, value: Text }); F may be the
// message posting the form, with its parameters.
func (mc *modCtx) testSubmit(name string, formArg, value syntax.Expr, env *env, eff effect) types.Type {
	c := mc.c
	f, ok := mc.stepFormOf(name, formArg, env, eff)
	var want types.Type = types.Err
	if ok {
		want = f.form
	}
	if name == "submitRaw" {
		want = &types.App{Con: "List", Args: []types.Type{&types.Record{Anonymous: true, Fields: []types.Field{{Name: "name", Type: types.Text}, {Name: "value", Type: types.Text}}}}}
	}
	if types.IsErr(want) {
		mc.infer(value, env, eff)
	} else {
		mc.check(value, want, env, eff)
	}
	if !ok {
		return types.Err
	}
	return mc.stepResult("answer", c.answerType())
}

// testClick types `Test.click m`: m is a message of a page or of App that carries no form (a
// form's message is posted with Test.submit).
func (mc *modCtx) testClick(arg syntax.Expr, env *env, eff effect) types.Type {
	c := mc.c
	head := arg
	if app, isApp := arg.(*syntax.App); isApp {
		head = app.Fn
	}
	ct, isCt := head.(*syntax.CtorRef)
	if isCt && ct.Ref.Kind == syntax.RefNone {
		mc.infer(arg, env, eff)
		return types.Err
	}
	var info *ctorInfo
	if isCt && ct.Ref.Kind == syntax.RefCtor {
		info = c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)]
	}
	if info == nil || !c.isMessageUnion(info.Union) {
		t := c.u.Apply(mc.infer(arg, env, eff))
		got := "`" + mc.source(arg.GetSpan()) + "`"
		if !types.IsErr(t) {
			got += " of type `" + mc.ts(t) + "`"
		}
		if page, _ := pageOf(arg); page != "" { // a link, not a button
			got += ": follow a link with `Test.get` or `Test.open`"
		}
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.click` wants a message of a page or of `App`, got %s", got)
		return types.Err
	}
	if form := mc.msgForm(ct.Ref.Module, ct.Ref.Name); form != "" {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`%s` carries form `%s`: use `Test.submit`", ct.Ref.Name, form)
		mc.infer(arg, env, eff)
		return types.Err
	}
	mc.check(arg, info.Union, env, eff)
	return mc.stepResult("answer", c.answerType())
}

// msgForm is the form message msg of module mod (a page or App) posts, "" for none.
func (mc *modCtx) msgForm(mod, msg string) string {
	c := mc.c
	if p := c.pages[mod]; p != nil {
		if md := p.Msgs[msg]; md != nil {
			return md.Form
		}
	}
	if mod == "App" {
		if m := c.prog.Modules["App"]; m != nil {
			for _, d := range c.live(m) {
				if md, ok := d.(*syntax.MsgDecl); ok && md.Name == msg {
					return md.Form
				}
			}
		}
	}
	return ""
}

// checkSelectorArg reports E0023 when the selector argument of the query `Test.<name>` applied
// to args is a literal that does not parse.
func (mc *modCtx) checkSelectorArg(name string, args []syntax.Expr) {
	i, ok := selectorArg[name]
	if !ok || i >= len(args) {
		return
	}
	lit, ok := args[i].(*syntax.StrLit)
	if !ok {
		return
	}
	var text strings.Builder
	for _, p := range lit.Parts {
		if p.Expr != nil {
			return // interpolated: parsed when the test runs
		}
		text.WriteString(p.Text)
	}
	if _, err := selector.Parse(text.String()); err != nil { // the text names the token and the subset
		mc.c.errorf(BadSelector, lit.Span, "%s", err.Error())
	}
}
