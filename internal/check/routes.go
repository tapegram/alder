// Route declarations (#160): `route NAME = METHOD "path" { params } [form F] [requires …]` and
// the members `NAME.load`, `NAME.view`, `NAME.update` (E0043). Markup and test steps on routes
// are html.go's and browser.go's.
package check

import (
	"reflect"
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// RouteInfo is a route's generated types and what the checker learned of its members.
type RouteInfo struct {
	Module, Name, Method, Path string
	Decl                       *syntax.RouteDecl
	// Params is `Row.Params` (Module "Todos", Name "Row.Params"); a `form F` route has the
	// field `form: F`.
	Params *types.Record
	// Session is the route's session narrowed by its `requires` (plan call 4), nil for a route
	// without `requires`, whose members see App's session. Routes of one module that narrow the
	// same fields share one record, named after the first of them, so their members can call
	// each other with the session they have.
	Session *types.Record
	// Requires, Roles and Silo are the route's guard: the `requires`
	// fields, the roles of a well-formed `role` clause, the key of a valid `silo k`.
	Requires []string
	Roles    []string
	Silo     string
	Model    types.Type // GET: what `load` returns (nil when it is not known)
	File     bool       // GET: `view` returns File
	// NeedsShown is set for a write route whose `update` may answer with the page it was sent
	// from rendered again (plan call 12; Task 2.7).
	NeedsShown bool
	// OptionRoutes are, for a write route with a form, the GET routes whose view renders a
	// `<Select options>` of the form, written in the view itself or in a view it reaches through
	// the values it uses (`Row.view` called in `Index.view`), in module order, then declaration
	// order (plan call 11): the views the runtime renders again to check a submitted option.
	OptionRoutes []*RouteInfo
	// narrowed are the session fields the route's session narrows: Requires and Silo.
	narrowed []string
	// optionViews are the GET routes whose own view writes a `<Select options>` of this write
	// route's form (recorded where inShownView allows one), from which OptionRoutes are found.
	optionViews []*RouteInfo
}

// isWrite reports whether method is a write route's (anything but GET).
func isWrite(method string) bool { return method != "GET" }

// route is route name of module, nil when the module has none by that name (or its routes are
// not defined: a module that also declares a `page`).
func (c *Checker) route(module, name string) *RouteInfo {
	for _, r := range c.routes[module] {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// routeTarget is the route constructor heading e (`Row`, `Row { id }`, `Todos.Row { id }`) and
// its route; nil, nil when e is not written as a route, or its route is not defined.
func (mc *modCtx) routeTarget(e syntax.Expr) (*syntax.CtorRef, *RouteInfo) {
	if app, ok := e.(*syntax.App); ok && len(app.Args) == 1 {
		e = app.Fn
	}
	ct, ok := e.(*syntax.CtorRef)
	if !ok || ct.Ref.Kind != syntax.RefRoute {
		return nil, nil
	}
	if r := mc.c.route(ct.Ref.Module, ct.Ref.Name); r != nil {
		return ct, r
	}
	return nil, nil
}

// routeModel is GET route r's model, what its `load`'s signature says it gives, nil when that
// is not known (no `load`, or a signature E0043 refuses). It is read from the signature, not
// from Model, so a test checked before the route's module has its bodies checked sees it.
func (c *Checker) routeModel(r *RouteInfo) types.Type {
	if r.Model != nil {
		return r.Model
	}
	fn, ok := c.values[c.key(r.Module, r.Name+".load")].(*types.Fun)
	if !ok || len(fn.Params) != 2 {
		return nil
	}
	if q, ok := fn.Result.(*types.App); ok && q.Con == "Query" && len(q.Args) == 1 && !types.IsErr(q.Args[0]) {
		return q.Args[0]
	}
	return nil
}

// routeFile reports whether GET route r is a file route (its `view` returns File), read from
// its signature as routeModel is.
func (c *Checker) routeFile(r *RouteInfo) bool {
	if r.File {
		return true
	}
	view := c.routeMembers(c.prog.Modules[r.Module], r.Name)["view"]
	return view != nil && sigReturnsFile(view)
}

// inShownView reports whether the markup being checked is in the view the runtime renders
// again from the model to check a submitted `<Select options>` value (plan call 11): a GET
// route's `R.view`.
func (mc *modCtx) inShownView() bool {
	return mc.route != nil && !isWrite(mc.route.Method) && mc.value == mc.route.Name+".view"
}

// recordOptions notes that the view being checked, a GET route's, writes a `<Select options>`
// of the form posting to write route w (nil when the form posts to no route).
func (mc *modCtx) recordOptions(w *RouteInfo) {
	if w != nil && mc.route != nil && !slices.Contains(w.optionViews, mc.route) {
		w.optionViews = append(w.optionViews, mc.route)
	}
}

// checkOptionRoutes sets each write route's OptionRoutes once every body is checked: the GET
// routes whose view is, or reaches through the values it refers to, a view that recorded a
// `<Select options>` of the route's form.
func (c *Checker) checkOptionRoutes() {
	var writes, gets []*RouteInfo
	for _, name := range c.prog.Order {
		for _, r := range c.routes[name] {
			if len(r.optionViews) > 0 {
				writes = append(writes, r)
			}
			if !isWrite(r.Method) {
				gets = append(gets, r)
			}
		}
	}
	if len(writes) == 0 {
		return
	}
	refs := map[string]map[string]bool{} // "Module.value" → the values its body refers to
	for _, name := range c.prog.Order {
		for _, d := range c.live(c.prog.Modules[name]) {
			if vd, ok := d.(*syntax.ValueDecl); ok && vd.Body != nil {
				refs[c.key(name, vd.FullName())] = c.bodyFacts(vd.Body).refs
			}
		}
	}
	viewKey := func(r *RouteInfo) string { return c.key(r.Module, r.Name+".view") }
	for _, g := range gets {
		reached := map[string]bool{}
		stack := []string{viewKey(g)}
		for len(stack) > 0 {
			k := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if reached[k] {
				continue
			}
			reached[k] = true
			for r := range refs[k] {
				stack = append(stack, r)
			}
		}
		for _, w := range writes {
			if slices.ContainsFunc(w.optionViews, func(v *RouteInfo) bool { return reached[viewKey(v)] }) {
				w.OptionRoutes = append(w.OptionRoutes, g)
			}
		}
	}
}

// defineRoutes defines the routes of a route module: each route's Params and Session (as the
// types `R.Params` and `R.Session`), and checks its declaration. A module that also declares a
// `page` is E0043, and its routes are left undefined.
func (mc *modCtx) defineRoutes() {
	c := mc.c
	var decls []*syntax.RouteDecl
	for _, d := range c.live(mc.m) {
		if rd, ok := d.(*syntax.RouteDecl); ok && c.prog.Route(mc.m.Name, rd.Name) == rd {
			decls = append(decls, rd)
		}
	}
	sessions := map[string]*types.Record{} // narrowed fields → the module's record for them
	for _, d := range decls {
		info := &RouteInfo{Module: mc.m.Name, Name: d.Name, Method: d.Method, Path: d.Path, Decl: d,
			Params: &types.Record{Module: mc.m.Name, Name: d.Name + ".Params"}}
		c.paramRecs[c.key(mc.m.Name, info.Params.Name)] = true
		declared := mc.defineParams(d.Params, info.Params, "route")
		if d.Form != "" {
			if !isWrite(d.Method) {
				c.errorf(diag.RouteDecl, d.FormSpan, "route `%s` is a GET route, which has no form to post: `form %s` is for a write route (POST, PUT, PATCH or DELETE)", d.Name, d.Form)
			} else if rec, ok := c.forms[c.key(mc.m.Name, d.Form)]; ok {
				info.Params.Fields = append(info.Params.Fields, types.Field{Name: "form", Type: rec})
			} else {
				c.errorf(diag.UnknownName, d.FormSpan, "unknown form `%s`", d.Form)
			}
		}
		segs := pathParams(d.Path)
		for _, s := range segs {
			if !declared[s] {
				c.errorf(diag.RouteDecl, d.Span, "path segment `{%s}` of route `%s` has no matching parameter: add `%s` to its `{ … }`", s, d.Name, s)
			}
		}
		mc.checkWireNames(d.Params, segs, "route")
		if len(d.Requires) > 0 || d.Silo != "" {
			info.Requires = d.Requires
			mc.checkRequires(&d.Guard, d.Span)
			info.narrowed, info.Silo = mc.checkRequiresM15(&d.Guard, info.Params, "route")
			set := slices.Clone(info.narrowed)
			slices.Sort(set)
			k := strings.Join(set, ",")
			if sessions[k] == nil {
				sessions[k] = narrow(c.session, info.narrowed, mc.m.Name, d.Name+".Session")
			}
			info.Session = sessions[k]
		}
		if len(d.Roles) > 0 {
			info.Roles = mc.checkRoles(&d.Guard)
		}
		c.types[c.key(mc.m.Name, info.Params.Name)] = info.Params
		c.types[c.key(mc.m.Name, d.Name+".Session")] = info.sessionOr(c.session)
		c.routes[mc.m.Name] = append(c.routes[mc.m.Name], info)
	}
}

// sessionOr is the session the route's members see: its narrowed session, else app's.
func (r *RouteInfo) sessionOr(app *types.Record) *types.Record {
	if r.Session != nil {
		return r.Session
	}
	return app
}

// enterMember makes d's route the context of what is checked next, when d is a route member
// (`Row.view`): its `Session` is the route's (plan call 4), and its scoped operations are
// narrowed by the route's `requires`. It returns the function that restores the context.
func (mc *modCtx) enterMember(d *syntax.ValueDecl) func() {
	if d.Route == "" {
		return func() {}
	}
	saved, savedSession := mc.route, mc.session
	mc.route = mc.c.route(mc.m.Name, d.Route)
	if mc.route != nil {
		mc.session = mc.route.sessionOr(mc.c.session)
	}
	return func() { mc.route, mc.session = saved, savedSession }
}

// routeMembers maps each member name of route r to its live declaration in module m.
func (c *Checker) routeMembers(m *syntax.Module, r string) map[string]*syntax.ValueDecl {
	out := map[string]*syntax.ValueDecl{}
	for _, d := range c.live(m) {
		if vd, ok := d.(*syntax.ValueDecl); ok && vd.Route == r {
			out[vd.Name] = vd
		}
	}
	return out
}

// checkRoutes checks the members every route of the module has (E0043): a GET route's `load`
// and `view`, tied by their model, and a write route's `update`, each with the type the route
// fixes. It records each GET route's Model and File.
func (mc *modCtx) checkRoutes() {
	c := mc.c
	for _, r := range c.routes[mc.m.Name] {
		restore := mc.enterMember(&syntax.ValueDecl{Route: r.Name})
		members := c.routeMembers(mc.m, r.Name)
		if isWrite(r.Method) {
			mc.checkWriteRoute(r, members)
		} else {
			mc.checkGetRoute(r, members)
		}
		restore()
	}
}

func (mc *modCtx) checkWriteRoute(r *RouteInfo, members map[string]*syntax.ValueDecl) {
	c := mc.c
	sig := r.Name + ".Params -> Session -> Task Response"
	for _, name := range []string{"load", "view"} {
		if d := members[name]; d != nil {
			c.errorf(diag.RouteDecl, d.Span, "route `%s` is a %s route, which has no `%s`: a write route has only `%s.update : %s`; `load` and `view` belong to a GET route", r.Name, r.Method, name, r.Name, sig)
		}
	}
	d := members["update"]
	if d == nil {
		c.errorf(diag.RouteDecl, r.Decl.Span, "route `%s` is a %s route: it needs `%s.update : %s`", r.Name, r.Method, r.Name, sig)
		return
	}
	want := &types.Fun{Params: []types.Type{r.Params, mc.session}, Result: &types.App{Con: "Task", Args: []types.Type{types.Response}}}
	if mc.routeSig(d, want, sig) {
		r.NeedsShown = mc.needsShown(d)
	}
}

// needsShown reports whether write route update d may answer with the page the request came
// from rendered again (plan call 12): its body mentions `Response.refresh` or `Response.invalid`,
// or calls an app function whose type's result contains `Response`. A function is judged by its
// type, not its name, so a helper that wraps or builds a Response counts, and one that does not
// (`Text.trim`, a predicate) does not.
func (mc *modCtx) needsShown(d *syntax.ValueDecl) bool {
	if d.Body == nil {
		return false
	}
	c := mc.c
	return mentions(reflect.ValueOf(d.Body), func(n any) bool {
		v, ok := n.(*syntax.Var)
		if !ok {
			return false
		}
		switch v.Ref.Kind {
		case syntax.RefStd:
			return v.Ref.Module == "Response" && (v.Ref.Name == "refresh" || v.Ref.Name == "invalid")
		case syntax.RefValue:
			t, ok := c.values[c.key(v.Ref.Module, v.Ref.Name)]
			return ok && resultHasResponse(c.u.Apply(t))
		}
		return false
	})
}

// resultHasResponse reports whether t, or the result a function type finally gives, is
// `Response` or holds it: as an argument (`Task Response`, `Maybe Response`), a record's field
// (`{ response: Response, … }`) or a constructor's payload (`Answer Response | Stay`), at any
// depth. A recursive type is visited once.
func resultHasResponse(t types.Type) bool {
	return holdsResponse(t, map[types.Type]bool{})
}

func holdsResponse(t types.Type, seen map[types.Type]bool) bool {
	holds := func(t types.Type) bool { return holdsResponse(t, seen) }
	switch t := t.(type) {
	case *types.Con:
		return t == types.Response
	case *types.App:
		return slices.ContainsFunc(t.Args, holds)
	case *types.Fun:
		return holds(t.Result)
	case *types.Record:
		if seen[t] {
			return false
		}
		seen[t] = true
		for _, f := range t.Fields {
			if holds(f.Type) {
				return true
			}
		}
	case *types.Union:
		if seen[t] {
			return false
		}
		seen[t] = true
		if slices.ContainsFunc(t.Args, holds) {
			return true
		}
		for _, c := range t.Ctors {
			if c.Arg != nil && holds(c.Arg) {
				return true
			}
		}
	}
	return false
}

func (mc *modCtx) checkGetRoute(r *RouteInfo, members map[string]*syntax.ValueDecl) {
	c := mc.c
	if d := members["update"]; d != nil {
		c.errorf(diag.RouteDecl, d.Span, "route `%s` is a GET route, which has no `update`: a GET route has `%s.load` and `%s.view`; `update` belongs to a write route (POST, PUT, PATCH or DELETE)", r.Name, r.Name, r.Name)
	}
	load, view := members["load"], members["view"]
	model := c.u.Fresh()
	loadOK := false
	if load != nil && load.Sig != nil {
		want := &types.Fun{Params: []types.Type{r.Params, mc.session}, Result: &types.App{Con: "Query", Args: []types.Type{model}}}
		loadOK = mc.routeSig(load, want, r.Name+".Params -> Session -> Query "+mc.modelText(view, nil))
		if loadOK {
			r.Model = c.u.Apply(model)
		}
	}
	if view != nil && view.Sig != nil {
		r.File = sigReturnsFile(view)
		result := types.Type(&types.App{Con: "Html", Args: []types.Type{types.Route}})
		res := "Html"
		if r.File {
			result, res = types.File, "File"
		}
		arg := c.u.Fresh()
		want := &types.Fun{Params: []types.Type{arg, mc.session}, Result: result}
		if mc.routeSig(view, want, mc.modelText(nil, r.Model)+" -> Session -> "+res) {
			if err := c.u.Unify(arg, model); err != nil {
				d := c.errorf(diag.RouteDecl, view.Sig.GetSpan(), "`%s.view` takes `%s`, but `%s.load` gives `%s`", r.Name, mc.ts(c.u.Apply(arg)), r.Name, mc.ts(c.u.Apply(model)))
				fixSig(d, view.Sig.GetSpan(), mc.ts(c.u.Apply(model))+" -> Session -> "+res)
			} else if !loadOK {
				r.Model = c.u.Apply(arg)
			}
		}
	}
	if load == nil {
		c.errorf(diag.RouteDecl, r.Decl.Span, "route `%s` is a GET route: it needs `%s.load : %s.Params -> Session -> Query %s`", r.Name, r.Name, r.Name, mc.modelText(nil, r.Model))
	}
	if view == nil {
		c.errorf(diag.RouteDecl, r.Decl.Span, "route `%s` is a GET route: it needs `%s.view : %s -> Session -> Html`", r.Name, r.Name, mc.modelText(nil, r.Model))
	}
}

// modelText is a route's model type as a signature writes it: model when known, else the first
// parameter of view's declared signature, else `m`.
func (mc *modCtx) modelText(view *syntax.ValueDecl, model types.Type) string {
	if model != nil && !types.IsErr(model) {
		return mc.ts(model)
	}
	if view != nil && view.Sig != nil {
		if fn, ok := mc.c.values[mc.c.key(mc.m.Name, view.FullName())].(*types.Fun); ok && len(fn.Params) > 0 && !types.IsErr(fn.Params[0]) {
			return mc.ts(fn.Params[0])
		}
	}
	return "m"
}

// routeSig checks that member d's declared signature unifies with want, whose Alder text is sig
// (E0043, with the fix that writes sig), and reports whether it does. A member without a
// signature is E0007's.
func (mc *modCtx) routeSig(d *syntax.ValueDecl, want types.Type, sig string) bool {
	c := mc.c
	got, ok := c.values[c.key(mc.m.Name, d.FullName())]
	if !ok || d.Sig == nil {
		return false
	}
	if err := c.u.Unify(got, want); err != nil {
		e := c.errorf(diag.RouteDecl, d.Sig.GetSpan(), "`%s` must have type `%s`", d.FullName(), sig)
		fixSig(e, d.Sig.GetSpan(), sig)
		return false
	}
	return true
}

// fixSig sets d's fix to replace the signature at span with sig.
func fixSig(d *diag.Diagnostic, span diag.Span, sig string) {
	d.Fix = diag.ReplaceSpan(span, sig)
	d.Fix.Title, d.Fix.Preferred = "Change signature to `"+sig+"`", true
}

// sigReturnsFile reports whether d's declared signature returns File.
func sigReturnsFile(d *syntax.ValueDecl) bool {
	if d.Sig == nil {
		return false
	}
	t := d.Sig
	for {
		ft, ok := t.(*syntax.FunType)
		if !ok {
			break
		}
		t = ft.Result
	}
	tn, ok := t.(*syntax.TypeName)
	// the written module: the stdlib's Prelude is written by its name (a stdlib module's
	// qualifier is its name), and an unqualified `File` is the Prelude's unless declared here
	return ok && tn.Name == "File" && (tn.Module == "" || tn.Module == "Prelude") && len(tn.Args) == 0
}

// routeRef is a reference from a route member to a value of its own module, checked once the
// module's bodies are (checkRouteRefs): the value may use scoped operations its route does not
// narrow.
type routeRef struct {
	x     *syntax.Var
	route *RouteInfo
}

// checkRouteRefs reports E0026 for each pending routeRef whose value uses a scoped operation
// whose key the member's route does not narrow. It runs after propagateScoped, so a helper's
// operations are known whatever the order of the module's declarations.
func (c *Checker) checkRouteRefs() {
	refs := c.routeRefs
	c.routeRefs = nil
	for _, rr := range refs {
		k := c.key(rr.x.Ref.Module, rr.x.Ref.Name)
		uses := c.scopedUsing[k]
		keys := make([]string, 0, len(uses))
		for key := range uses {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if !slices.Contains(rr.route.narrowed, key) {
				c.reportScoped(rr.x.Span, "`"+rr.x.Ref.Name+"` uses `"+uses[key]+"`, which needs a route that `requires "+key+"`: `"+rr.route.Name+"` does not")
				break
			}
		}
	}
}

// bareRoute reports E0301 when e is a route with parameters (other than its form) written bare
// where a request goes (`click={Toggle}`, `href={Row}`, `Test.click Toggle`): the route is a
// function of its parameters there, not a Route. The fix writes the record with a `?hole` for
// each parameter, in parentheses when paren is set (a Test step's argument). It reports whether
// it did; e is then recorded as a Route.
func (mc *modCtx) bareRoute(e syntax.Expr, paren bool) bool {
	ct, ok := e.(*syntax.CtorRef)
	if !ok || ct.Ref.Kind != syntax.RefRoute {
		return false
	}
	r := mc.c.route(ct.Ref.Module, ct.Ref.Name)
	if r == nil || r.Params == nil || messageLike(r.Params) {
		return false
	}
	var parts []string
	for _, f := range r.Params.Fields {
		if f.Name != "form" {
			parts = append(parts, f.Name+": ?"+f.Name)
		}
	}
	full := mc.source(ct.Span) + " { " + strings.Join(parts, ", ") + " }"
	d := mc.c.errorf("E0301", ct.Span, "route `%s` needs its parameters: `%s`", ct.Name, full)
	if paren {
		full = "(" + full + ")"
	}
	d.Fix = diag.ReplaceSpan(ct.Span, full)
	d.Fix.Preferred = true
	mc.c.record(ct, types.Route)
	return true
}
