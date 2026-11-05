// Route declarations (#160): `route NAME = METHOD "path" { params } [form F] [requires …]` and
// the members `NAME.load`, `NAME.view`, `NAME.update` (E0043). Markup and test steps on routes
// are html.go's and browser.go's.
package check

import (
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
	// Requires, Roles and Silo are the route's guard as PageInfo has a page's: the `requires`
	// fields, the roles of a well-formed `role` clause, the key of a valid `silo k`.
	Requires []string
	Roles    []string
	Silo     string
	Model    types.Type // GET: what `load` returns (nil when it is not known)
	File     bool       // GET: `view` returns File
	// NeedsShown is set for a write route whose `update` may answer with the page it was sent
	// from rendered again (plan call 12; Task 2.7).
	NeedsShown bool
	// narrowed are the session fields the route's session narrows: Requires and Silo.
	narrowed []string
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
	if c.prog.IsPageModule(mc.m.Name) {
		for _, d := range decls {
			c.errorf(diag.RouteDecl, d.NameSpan, "module `%s` declares a `page`; a module holds a page and its messages, or routes, not both: move route `%s` to a module of its own", mc.m.Name, d.Name)
		}
		return
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
	mc.routeSig(d, want, sig)
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

// sigReturnsFile reports whether d's declared signature returns File (as viewReturnsFile reads
// a page's `view`).
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
