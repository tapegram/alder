// The browser steps of module Test (`Test.login`, `get`, `open`, `fetch`, `submit`, `submitRaw`,
// `click`) and `Test.refresh`, typed as special forms, the selector literals of its queries
// (E0023), and the test-only Prelude types `Doc` and `Answer`.
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
	"login": "Test.login { user: Just id }", "get": "Test.get (Todos.Index.Index {})", "open": "Test.open (Todos.Index.Index {})",
	"fetch":  "Test.fetch (Report.Csv.Csv { id: id })",
	"submit": "Test.submit Create { title: \"Milk\" }", "submitRaw": "Test.submitRaw Create [{ name: \"title\", value: \"\" }]",
	"click": "Test.click (Toggle { id: id })",
}

// selectorArg is, for each query, the index of its selector argument.
var selectorArg = map[string]int{"text": 0, "texts": 0, "has": 0, "count": 0, "attr": 1}

// answerCtors are the constructors of the Prelude's test type `Answer`.
var answerCtors = map[string]bool{"Shown": true, "Rendered": true, "Refreshed": true, "Redirected": true, "FormRejected": true, "PageMissing": true, "RequestFailed": true}

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

// stepResult is a step's result type: Task { <first>: t, session: Session, doc: Doc,
// notices: List Text, toasts: List { kind: Text, text: Text }, url: Text, status: Int }.
func (mc *modCtx) stepResult(first string, t types.Type) types.Type {
	toast := &types.Record{Anonymous: true, Fields: []types.Field{{Name: "kind", Type: types.Text}, {Name: "text", Type: types.Text}}}
	rec := &types.Record{Anonymous: true, Fields: []types.Field{
		{Name: first, Type: t}, {Name: "session", Type: mc.c.appSession()}, {Name: "doc", Type: mc.c.types["Prelude.Doc"]},
		{Name: "notices", Type: &types.App{Con: "List", Args: []types.Type{types.Text}}},
		{Name: "toasts", Type: &types.App{Con: "List", Args: []types.Type{toast}}},
		{Name: "url", Type: types.Text}, {Name: "status", Type: types.Int},
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
// step's own type. The first argument of a submit is Unit when it names a form (not a value);
// a route or a message, written with its parameters, is the value it was checked as (#160 R4).
func (mc *modCtx) stepParams(name string, args []syntax.Expr) []types.Type {
	var ps []types.Type
	for i, a := range args {
		if (name == "submit" || name == "submitRaw") && i == 0 {
			t, ok := mc.c.res.Types[a]
			if !ok {
				t = types.Unit // a form name: not a value
			}
			ps = append(ps, t)
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

// unresolvedHead reports whether e's head (e, or the function of an application) is a name the
// resolver could not resolve (reported), or an old page's value (E0046 in its module).
func unresolvedHead(e syntax.Expr) bool {
	if app, isApp := e.(*syntax.App); isApp {
		e = app.Fn
	}
	ct, isCt := e.(*syntax.CtorRef)
	return isCt && (ct.Ref.Kind == syntax.RefNone || ct.Ref.Kind == syntax.RefPage)
}

// testGet types `Test.get r`, `Test.open r` and `Test.fetch r`: r is a GET route written in
// place (testGetRoute).
func (mc *modCtx) testGet(name string, arg syntax.Expr, env *env, eff effect) types.Type {
	c := mc.c
	if mc.bareRoute(arg, true) {
		return types.Err
	}
	if ct, r := mc.routeTarget(arg); r != nil {
		return mc.testGetRoute(name, arg, ct, r, env, eff)
	}
	if unresolvedHead(arg) {
		return types.Err
	}
	c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.%s` wants a route written in place, such as `Todos.Index.Index {}`, got `%s`", name, mc.source(arg.GetSpan()))
	mc.infer(arg, env, eff)
	return types.Err
}

// testGetRoute is testGet of route r, written in place as arg (ct its head): a GET route, as a
// link requests it (a write route is E0045); get's model is what its `load` gives, which must
// not be a union; only fetch may name a file route.
func (mc *modCtx) testGetRoute(name string, arg syntax.Expr, ct *syntax.CtorRef, r *RouteInfo, env *env, eff effect) types.Type {
	c := mc.c
	if isWrite(r.Method) {
		use := "Test.click"
		if r.Decl.Form != "" {
			use = "Test.submit"
		}
		c.errorf(diag.RouteTarget, arg.GetSpan(), "`Test.%s` requests a GET route; `%s` is %s: use `%s`", name, ct.Name, r.Method, use)
		mc.infer(arg, env, eff)
		return types.Err
	}
	mc.check(arg, types.Route, env, eff)
	if c.routeFile(r) && name != "fetch" {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`%s` is a file route: use `Test.fetch`", ct.Name)
		return types.Err
	}
	switch name {
	case "fetch":
		return fetchResult()
	case "open":
		return mc.stepResult("answer", c.answerType())
	}
	model := c.routeModel(r)
	if model == nil {
		return types.Err // a route without `load`: E0043
	}
	if _, isUnion := model.(*types.Union); isUnion {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.get` needs a model that is not a union: `%s.load` gives `%s` — use `Test.open`", ct.Name, mc.ts(model))
		return types.Err
	}
	return mc.stepResult("model", model)
}

// routeStep is how a test sends route r, for messages: `Name`, or `(Name { a: …, b: … })` with
// its parameters (its form left out).
func routeStep(r *RouteInfo) string {
	var fs []string
	for _, f := range r.Params.Fields {
		if f.Name != "form" {
			fs = append(fs, f.Name+": …")
		}
	}
	if len(fs) == 0 {
		return r.Name
	}
	return "(" + r.Name + " { " + strings.Join(fs, ", ") + " })"
}

// stepForm is the form a `Test.submit` posts and the write route posting it.
type stepForm struct {
	module string // the route's module
	form   *types.Record
	route  string
}

// stepFormOf resolves the target of `Test.submit`/`submitRaw`: a write route with `form F`,
// written in place with its parameters (`Move { id: id }`). ok is false when an error was
// reported (or the resolver reported one).
func (mc *modCtx) stepFormOf(step string, arg syntax.Expr, env *env, eff effect) (stepForm, bool) {
	c := mc.c
	if unresolvedHead(arg) {
		mc.inferArgs([]syntax.Expr{arg}, env, eff)
		return stepForm{}, false
	}
	if ct, r := mc.routeTarget(arg); r != nil { // a write route with `form F`: F's record
		if r.Decl.Form != "" && mc.bareRoute(arg, true) {
			return stepForm{}, false
		}
		if r.Decl.Form == "" {
			if isWrite(r.Method) {
				c.errorf(diag.RouteTarget, arg.GetSpan(), "`Test.%s` posts a route's form; `%s` has no `form`: use `Test.click %s`", step, ct.Name, routeStep(r))
			} else {
				c.errorf(diag.RouteTarget, arg.GetSpan(), "`Test.%s` posts a route's form; `%s` is GET: use `Test.get`", step, ct.Name)
			}
			mc.infer(arg, env, eff)
			return stepForm{}, false
		}
		mc.check(arg, types.Route, env, eff)
		for _, f := range r.Params.Fields {
			if fr, ok := f.Type.(*types.Record); ok && f.Name == "form" {
				return stepForm{module: r.Module, form: fr, route: r.Name}, true
			}
		}
		return stepForm{}, false // its form is not a record: reported at the route
	}
	ct, isCt := arg.(*syntax.CtorRef)
	if !isCt || ct.Ref.Kind != syntax.RefType || c.forms[c.key(ct.Ref.Module, ct.Ref.Name)] == nil {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.%s` wants a write route with `form`, such as `Create`, got `%s`", step, mc.source(arg.GetSpan()))
		if !isCt || ct.Ref.Kind != syntax.RefType {
			mc.infer(arg, env, eff)
		}
		return stepForm{}, false
	}
	c.record(ct, types.Unit) // a form's name is not a value (stepParams)
	mod := ct.Ref.Module
	for _, r := range c.routes[mod] { // a route's form is posted by naming the route
		if r.Decl.Form == ct.Ref.Name {
			c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.%s` names the route that posts a form: write `Test.%s %s …`", step, step, routeStep(r))
			return stepForm{}, false
		}
	}
	c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.%s` wants a write route with `form`, got form `%s` of `%s`, which no route posts", step, ct.Name, mod)
	return stepForm{}, false
}

// testSubmit types `Test.submit r f` (r a write route with `form F`, f F's record: a request
// value, so its defaults apply) and `Test.submitRaw r fields` (fields a List { name: Text,
// value: Text }).
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

// testClick types `Test.click r`: r is a route written in place, a GET route fetched or a write
// route without a form (a form's route is posted with Test.submit).
func (mc *modCtx) testClick(arg syntax.Expr, env *env, eff effect) types.Type {
	c := mc.c
	if unresolvedHead(arg) {
		mc.infer(arg, env, eff)
		return types.Err
	}
	if mc.bareRoute(arg, true) {
		return types.Err
	}
	if ct, r := mc.routeTarget(arg); r != nil {
		if r.Decl.Form != "" {
			c.errorf(diag.RouteTarget, arg.GetSpan(), "`Test.click` sends a route without a form; `%s` posts form `%s`: use `Test.submit %s { … }`", ct.Name, r.Decl.Form, routeStep(r))
			mc.infer(arg, env, eff)
			return types.Err
		}
		mc.check(arg, types.Route, env, eff)
		return mc.stepResult("answer", c.answerType())
	}
	t := c.u.Apply(mc.infer(arg, env, eff))
	got := "`" + mc.source(arg.GetSpan()) + "`"
	if !types.IsErr(t) {
		got += " of type `" + mc.ts(t) + "`"
	}
	c.errorf(diag.TypeMismatch, arg.GetSpan(), "`Test.click` wants a route, got %s", got)
	return types.Err
}

// testRefresh types `Test.refresh` at x: a step's result, as `Test.open`'s.
func (mc *modCtx) testRefresh(x *syntax.Var) types.Type {
	c := mc.c
	if c.prog.Modules["App"] == nil {
		c.errorf(diag.TypeMismatch, x.Span, "`Test.refresh` needs an `App` module: the browser requests the app's routes, which an app without `App` does not serve")
		return types.Err
	}
	return mc.stepResult("answer", c.answerType())
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
