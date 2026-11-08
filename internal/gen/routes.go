package gen

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// Routes: routes__alder.go, the web layer of an app with an App module (rt/web serves it).
//
// Per GET route R of module M:
//
//	decode__M__R__Params(params, q) R's parameters: path ones from params, the others from the
//	                                query q; defaults applied; an error is a 404
//	page__M__R(rq, params, keep)    `requires` (a 303 to App.login), the role (404), then load
//	Route__M__R()                   GET <path>: decode (404), page__M__R, R.view rendered with
//	                                rq.FormState (a File written raw); the router delivers it
//	                                as the request asks (a document, a morph, one element)
//
// and per write route R (POST, PUT, PATCH, DELETE):
//
//	decode__M__R__Params(params, q) R's parameters but its form, as a GET route's (404)
//	decode__M__<Form>(v)            the route's form from the request body
//	guard__M__R(rq, params)         `requires` (a 303 to App.login), the role (404): the session
//	                                R.update takes (a route that requires only)
//	Route__M__R()                   <METHOD> <path>: decode (404), guard__M__R, the form (one
//	                                that does not decode: web.InvalidResponse), the options check
//	                                against the page it was posted from (rq.CheckOptions), that
//	                                page before the update (rq.RenderShown: only when R.update
//	                                may answer refresh or invalid), R.update, web.AnswerResponse
//
// Routes lists every route, in module order, then declaration order. The book's request
// lifecycle (chapter 6) is the order of these steps. Under Datastar rt/web answers a 303 to a
// page of the app with that page, served in-process and morphed in with a history push, and one
// to another site with an SSE event navigating the browser (datastar.Navigate).
//
// A decoder reads each field with the rt/web helpers (web.Text, Int, Float, Date, Bool, Enum)
// and collects web.FieldErrors:
//   - a required field that is absent is web.Missing ("is required"); for Int, Float, Date and
//     unions a blank value (spaces only) counts as absent, for Text (and its brands) only a
//     missing key does: "" is decoded, and refinements such as Text.NonEmpty judge it;
//   - a field with a default takes it when absent;
//   - a Maybe is Nothing when absent or "" (blank for Int, Float, Date and unions; with a
//     default: the default when absent, Nothing when blank), else Just the decoded value;
//   - a Bool is a checkbox: absent is False, any value but "" and "false" True (web.Bool); in a URL
//     it is strict (web.BoolParam: True/False, true/false, 1/0; absent is its default when it has
//     one, else False; anything else is a 404);
//   - a List (URL parameters only) is every value of the key, each decoded as its element;
//   - a brand's check runs on the decoded value (inside a Maybe or List, on each present one):
//     Text.NonEmpty, Email, Url, Secret and Int.Positive, NonNegative by rt/web's helpers, an app
//     brand's `where` by its check__ function ("is not valid").

// routesSource is routes__alder.go.
func (g *gen) routesSource(p *ir.Program) string {
	var b strings.Builder
	var routes []string
	usesForms := false
	for _, r := range p.Routes {
		routes = append(routes, routeFunc(r.Module, r.Name)+"()")
		usesForms = usesForms || r.Form != nil
	}
	b.WriteString(comment("Routes are the app's endpoints for web.NewRouter: every route (GET and write routes), in module order, then declaration order."))
	if len(routes) == 0 {
		b.WriteString("var Routes = []web.Route{}\n\n")
	} else {
		b.WriteString("var Routes = []web.Route{\n" + strings.Join(routes, ",\n") + ",\n}\n\n")
	}
	forms := map[*ir.Form]bool{} // a form's decoder, once for every write route posting it
	for _, r := range p.Routes {
		if r.Method == "GET" {
			g.getRoute(&b, p, r)
		} else {
			g.writeRoute(&b, p, r, forms)
		}
	}
	if g.tests {
		g.encoders(&b, p) // browser.go
	}
	if len(p.Silos) > 0 {
		b.WriteString(`// route__fail answers a load or update that did not succeed: a 303 to the page for
// Query.redirect, 404 (App.notFound) for Query.notFound and Task.notFound, 503 (App.maintenance)
// for a silo that is not served, else 500 (App.error).
func route__fail(err error) web.Response {
	var rd *eff.ErrRedirect
	if errors.As(err, &rd) {
		return web.QueryRedirectResponse(rd.Location)
	}
	if errors.Is(err, eff.ErrNotFound) {
		return web.NotFoundResponse(html.None())
	}
	var m *db.MaintenanceError
	if errors.As(err, &m) {
		return web.MaintenanceResponse(err)
	}
	return web.ErrorResponse(html.None(), err)
}

`)
	} else {
		b.WriteString(`// route__fail answers a load or update that did not succeed: a 303 to the page for
// Query.redirect, 404 (App.notFound) for Query.notFound and Task.notFound, else 500 (App.error).
func route__fail(err error) web.Response {
	var rd *eff.ErrRedirect
	if errors.As(err, &rd) {
		return web.QueryRedirectResponse(rd.Location)
	}
	if errors.Is(err, eff.ErrNotFound) {
		return web.NotFoundResponse(html.None())
	}
	return web.ErrorResponse(html.None(), err)
}

`)
	}
	if usesForms {
		b.WriteString(`// route__values are the submitted fields of a form as its re-render shows them: the first value
// of each field, without the Text.Secret fields named in secrets.
func route__values(v url.Values, secrets ...string) map[string]string {
	out := map[string]string{}
	for k, vs := range v {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	for _, s := range secrets {
		delete(out, s)
	}
	return out
}

// route__errors are a form's decoding errors by field, for html.FormState.
func route__errors(errs []web.FieldError) map[string][]string {
	out := map[string][]string{}
	for _, e := range errs {
		out[e.Field] = append(out[e.Field], e.Message)
	}
	return out
}
`)
	}
	src := b.String()
	imports, err := importsOf(src)
	if err != nil {
		panic(fmt.Sprintf("gen: %s: %v\n%s", routesFile, err, src))
	}
	return "// Code generated by alder; DO NOT EDIT.\n\npackage app\n\n" + importBlock(imports) + src
}

// comment is text as a Go comment, wrapped at 100 columns.
func comment(text string) string {
	var b strings.Builder
	line := "//"
	for _, w := range strings.Fields(text) {
		if len(line)+1+len(w) > 100 && line != "//" {
			b.WriteString(line + "\n")
			line = "//"
		}
		line += " " + w
	}
	b.WriteString(line + "\n")
	return b.String()
}

// getRoute writes GET route r's parameters decoder, its page__ function (`requires`, load) and
// its route, Route__M__R (#160). Its view renders with the request's FormState: nil for a
// request of its own, the posted form's state when a write renders the page the browser is on
// again (web.Request.RenderShown). How the view is delivered (a document, a body morph, one
// element patch) is the router's, by the request (rt/web deliver.go).
func (g *gen) getRoute(b *strings.Builder, p *ir.Program, r *ir.Route) {
	m, key := r.Module, routeKey(r)
	decode := decodeName(m, r.Name+".Params")
	g.paramsDecoderOf(b, decode, "route "+key, GoName(m, r.Name+".Params"), r.Path, r.Params)
	g.pageFunc(b, p, routeGuard(r))
	steps := "decode the parameters (404 when they do not), load (404 on Query.notFound, 303 on Query.redirect, 500 on a failure), view (the model also in rq.Model, which a test's browser reads)."
	if len(r.Requires) > 0 {
		steps = "decode the parameters (404 when they do not), `requires` (303 to App.login), load (404 on Query.notFound, 303 on Query.redirect, 500 on a failure), view (the model also in rq.Model, which a test's browser reads)."
	}
	if r.Roles != nil {
		steps = "decode the parameters (404 when they do not), `requires` (303 to App.login), the role (404), load (404 on Query.notFound, 303 on Query.redirect, 500 on a failure), view (the model also in rq.Model, which a test's browser reads)."
	}
	view := GoName(m, r.Name+".view")
	fileField, respond := "", fmt.Sprintf("web.PageResponse(%s(model, session), rq.FormState, rq.Session, false)", view)
	if r.File {
		steps = strings.Replace(steps, "view (the model", "view (its File written raw; the model", 1)
		fileField, respond = "File: true, ", fmt.Sprintf("web.FileResponse(%s(model, session), rq.Session)", view)
	}
	b.WriteString(comment(fmt.Sprintf("%s serves route %s, GET %s: %s", routeFunc(m, r.Name), key, r.Path, steps)))
	fmt.Fprintf(b, `func %s() web.Route {
	return web.Route{Method: "GET", Pattern: %s, %sHandle: func(rq *web.Request) web.Response {
		params, errs := %s(rq.Params, rq.Query)
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
		model, session, stop := %s(rq, params, nil)
		if stop != nil {
			return *stop
		}
		rq.Model = model
		return %s
	}}
}

`, routeFunc(m, r.Name), strconv.Quote(r.Path), fileField, decode, pageFunc(key), respond)
}

// guardFunc is write route r's guard__ function ("guard__Todos__Toggle").
func guardFunc(r *ir.Route) string { return "guard__" + GoName(r.Module, r.Name) }

// urlParams is route r's parameters as its URL carries them: without the `form` field of a
// route with `form F`, which the request's body supplies.
func urlParams(r *ir.Route) *ir.RecordType {
	if r.Form == nil {
		return r.Params
	}
	rec := *r.Params
	rec.Fields = nil
	for _, f := range r.Params.Fields {
		if f.Name != "form" {
			rec.Fields = append(rec.Fields, f)
		}
	}
	return &rec
}

// writeRoute writes write route r's parameters decoder, its guard (guard__M__R, for a route that
// requires), its form's decoder (forms: once per form) and its route, Route__M__R (#160). The
// handler's steps, in order:
//   - decode the URL's parameters (404);
//   - `requires`, role (guard__M__R: 303 to App.login, 404);
//   - decode the form (plan call 10): one that does not decode is web.InvalidResponse, the page
//     the browser is on rendered again with the posted values and the field errors (diffed
//     against it rendered before, under Datastar), update not called;
//   - the options check (plan call 11), when the form has selects with `options={…}` in the
//     views of some GET routes (OptionRoutes): the page it was posted from is rendered
//     (RenderShown) and checked (rq.CheckOptions); a value it did not offer is answered as a form
//     that does not decode, a post from another page is a 400;
//   - the page before (rq.RenderShown), only when the update may answer refresh or invalid
//     (NeedsShown) and under Datastar (the plain answers render it afresh); the options check's
//     render serves when there is one;
//   - for an app with silos, the Task bound to the route's store (db.Enter);
//   - R.update (a Task Response), answered by web.AnswerResponse with the session it left; an
//     `invalid` keeps the form's posted values (a FormState keyed by the route's rt.RouteKey). A
//     failed Task changes nothing (route__fail).
func (g *gen) writeRoute(b *strings.Builder, p *ir.Program, r *ir.Route, forms map[*ir.Form]bool) {
	m, rkey := r.Module, routeKey(r)
	decode := decodeName(m, r.Name+".Params")
	g.paramsDecoderOf(b, decode, "route "+rkey, GoName(m, r.Name+".Params"), r.Path, urlParams(r))
	session := GoName(r.Session.Module, r.Session.Name)
	app := GoName(p.Session.Module, p.Session.Name)
	if len(r.Requires) > 0 {
		gd := guarded{key: rkey, what: "route " + rkey, params: GoName(m, r.Name+".Params"), session: r.Session,
			requires: r.Requires, roles: r.Roles, silo: r.Silo}
		roles := ""
		if r.Roles != nil {
			roles = fmt.Sprintf(", then `role %s` (404 when the session's role is not one of them)", strings.Join(r.Roles, " | "))
		}
		b.WriteString(comment(fmt.Sprintf("%s runs the guard of route %s for rq: `requires %s` (a 303 to App.login when the session's %s is Nothing)%s. It returns the session the route's update takes, or the response that ends the request.", guardFunc(r), rkey, strings.Join(r.Requires, ", "), strings.Join(r.Requires, " or "), roles)))
		fmt.Fprintf(b, "func %s(rq *web.Request, params %s) (%s, *web.Response) {\n\tapp := rq.Env.Session.(%s)\n", guardFunc(r), gd.params, session, app)
		g.requiresChecks(b, p, gd, "", "nil")
		fmt.Fprintf(b, "\treturn %s{%s}, nil\n}\n\n", session, strings.Join(sessionFields(p, gd), ", "))
	}
	if r.Form != nil {
		g.formDecoder(b, r.Form, forms)
	}

	steps := []string{"decode the parameters (404 when they do not)"}
	if len(r.Requires) > 0 {
		steps = append(steps, "`requires` (303 to App.login)")
	}
	if r.Roles != nil {
		steps = append(steps, "the role (404)")
	}
	if r.Form != nil {
		steps = append(steps, fmt.Sprintf("decode form %s from the body (when it does not: 422, the page the browser is on rendered again with the submitted values, no Text.Secret field, and the errors, without calling update)", r.Form.Name))
	}
	if len(r.OptionRoutes) > 0 {
		steps = append(steps, "check its options against the page it was posted from (a value that page did not offer: as a form that does not decode; posted from another page: 400)")
	}
	if r.NeedsShown {
		steps = append(steps, "render the page the browser is on before the update (refresh and invalid are diffed against it)")
	}
	steps = append(steps, "run the update and answer its Response (a failed Task: 500, 404 for Query.notFound or Task.notFound, 303 for Query.redirect)")
	b.WriteString(comment(fmt.Sprintf("%s serves route %s, %s %s: %s.", routeFunc(m, r.Name), rkey, r.Method, r.Path, strings.Join(steps, "; "))))

	fmt.Fprintf(b, `func %s() web.Route {
	return web.Route{Method: %s, Pattern: %s, Handle: func(rq *web.Request) web.Response {
		params, errs := %s(rq.Params, rq.Query)
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
`, routeFunc(m, r.Name), strconv.Quote(r.Method), strconv.Quote(r.Path), decode)
	if len(r.Requires) > 0 {
		fmt.Fprintf(b, `		session, stop := %s(rq, params)
		if stop != nil {
			return *stop
		}
`, guardFunc(r))
	} else {
		fmt.Fprintf(b, "\t\tsession := rq.Env.Session.(%s)\n", app)
	}
	before := "nil"
	if r.Form != nil {
		f := r.Form
		formName := strconv.Quote(f.Module + "." + f.Name)
		args := []string{"rq.Form"}
		for _, fd := range f.Record.Fields {
			if isSecret(fd.Brand) {
				args = append(args, strconv.Quote(fd.Name))
			}
		}
		fmt.Fprintf(b, `		key := rt.RouteKey(%s)
		values := route__values(%s)
		form, errs := %s(rq.Form)
		if errs != nil {
			resp := web.InvalidResponse(&html.FormState{Form: %s, Msg: key, Values: values, Errors: route__errors(errs)}, nil)
			if rq.Datastar {
				resp.Shown, _ = rq.RenderShown(nil)
			}
			return resp
		}
		params.Form = form
`, g.selfRoute(r), strings.Join(args, ", "), decodeName(f.Module, f.Name), formName)
		if len(r.OptionRoutes) > 0 {
			var pats []string
			for _, o := range r.OptionRoutes {
				pats = append(pats, strconv.Quote(o.Path))
			}
			fmt.Fprintf(b, `		shown, _ := rq.RenderShown(nil)
		if errs, bad := rq.CheckOptions(shown, []string{%s}, %s, key, values); bad != nil {
			return *bad
		} else if errs != nil {
			resp := web.InvalidResponse(&html.FormState{Form: %s, Msg: key, Values: values, Errors: route__errors(errs)}, nil)
			resp.Shown = shown
			return resp
		}
`, strings.Join(pats, ", "), formName, formName)
			if r.NeedsShown {
				before = "shown"
			}
		}
	}
	if r.NeedsShown && before == "nil" {
		b.WriteString(`		var before *web.Shown
		if rq.Datastar {
			before, _ = rq.RenderShown(nil)
		}
`)
		before = "before"
	}
	if len(p.Silos) > 0 {
		fmt.Fprintf(b, "\t\tif err := db.Enter(rq.Env, %s); err != nil {\n\t\t\treturn route__fail(err)\n\t\t}\n", strconv.Quote(r.Store.Silo))
	}
	fmt.Fprintf(b, "\t\tresp, err := eff.Run(rq.Env, eff.KTask, %s(params, session))\n", GoName(m, r.Name+".update"))
	b.WriteString(routeUpdateFailure(p, r.Form != nil))
	if r.Form != nil {
		fmt.Fprintf(b, `		ans := web.AnswerResponse(resp, %s, rq.Env.Session)
		if resp.Kind == rt.RespInvalid {
			ans.State = &html.FormState{Form: %s, Msg: key, Values: values}
		}
		return ans
	}}
}

`, before, strconv.Quote(r.Form.Module+"."+r.Form.Name))
		return
	}
	fmt.Fprintf(b, "\t\treturn web.AnswerResponse(resp, %s, rq.Env.Session)\n\t}}\n}\n\n", before)
}

// selfRoute is the rt.Route literal of write route r for the request being served, built from
// its decoded parameters (params) as a link to it is: its rt.RouteKey is the FormState.Msg of the
// <form> that posted it (html.FormToRoute), so the form's state shows on that very form.
func (g *gen) selfRoute(r *ir.Route) string {
	f := &fn{g: g}
	s := newScope()
	s.declareAs("params", "params")
	ty := &types.Record{Module: r.Module, Name: r.Name + ".Params"}
	v := &ir.Var{Typed: ir.Typed{Ty: ty}, Kind: ir.VarLocal, Name: "params"}
	rec := &ir.Record{Typed: ir.Typed{Ty: ty}}
	url := urlParams(r)
	for _, fd := range url.Fields {
		rec.Fields = append(rec.Fields, ir.FieldInit{Name: fd.Name, Value: &ir.Field{Typed: ir.Typed{Ty: fd.Type}, Expr: v, Name: fd.Name}})
	}
	var params ir.Expr = rec
	if len(rec.Fields) == 0 {
		params = nil
	}
	return f.routeLit(routeKey(r), r.Method, r.Path, url, params, false, s)
}

// routeUpdateFailure is a write route's answer to a failed update Task: route__fail, and for an
// app with `via` fields a Session.put the membership check refused (eff.ErrFail.NotMember: a
// crafted switch to an org the user is no member of) is `invalid` "You are not a member of that
// workspace." for a route with a form (hasForm), else 404 — not a 500.
func routeUpdateFailure(p *ir.Program, hasForm bool) string {
	if len(p.Via) == 0 {
		return "\t\tif err != nil {\n\t\t\treturn route__fail(err)\n\t\t}\n"
	}
	member := "\t\t\treturn web.NotFoundResponse(html.None())\n"
	if hasForm {
		member = "\t\t\tresp = rt.Invalid(\"You are not a member of that workspace.\")\n"
	}
	return "\t\tif err != nil {\n\t\t\tvar nm *eff.ErrFail\n\t\t\tif !errors.As(err, &nm) || !nm.NotMember {\n\t\t\t\treturn route__fail(err)\n\t\t\t}\n" + member + "\t\t}\n"
}

// isSecret reports whether b is the stdlib's Text.Secret.
func isSecret(b *ir.Brand) bool { return b != nil && b.Module == "Text" && b.Name == "Secret" }

// guarded is what page__M__R (pageFunc) runs for a GET route, and what a write route's
// guard__M__R runs. key names the function ("Todos.Row" for route Row of module Todos: a module
// Todos.Row and that route cannot both exist) and keys a mutate build's requires dropped and
// role dropped mutants (mutation.go); what names it in comments.
type guarded struct {
	key, what       string
	params, model   string // Go types
	session         *ir.RecordType
	load            string // the Go function
	requires, roles []string
	silo            string
}

// routeGuard is GET route r as guarded.
func routeGuard(r *ir.Route) guarded {
	m := r.Module
	return guarded{key: routeKey(r), what: "route " + routeKey(r), params: GoName(m, r.Name+".Params"), model: GoType(r.Model),
		session: r.Session, load: GoName(m, r.Name+".load"), requires: r.Requires, roles: r.Roles, silo: r.Silo}
}

// routeKey is route r's qualified name, "Todos.Row".
func routeKey(r *ir.Route) string { return r.Module + "." + r.Name }

// pageFunc writes page__X for gd (a page or a GET route): `requires`, then load.
func (g *gen) pageFunc(b *strings.Builder, p *ir.Program, gd guarded) {
	model, session := gd.model, GoName(gd.session.Module, gd.session.Name)
	app := GoName(p.Session.Module, p.Session.Name)
	fnName := pageFunc(gd.key)
	if len(gd.requires) == 0 {
		b.WriteString(comment(fmt.Sprintf("%s runs %s for rq up to its model: load (with params and the session of rq.Env). It returns the model and the session view and update take, or the response that ends the request, which stores keep (the session a successful update left, or nil).", fnName, gd.what)))
		fmt.Fprintf(b, `func %s(rq *web.Request, params %s, keep any) (%s, %s, *web.Response) {
	session := rq.Env.Session.(%s)
	model, err := eff.RunQuery(rq.Env, %s(params, session))
	if err != nil {
		r := route__fail(err)
		r.Session = keep
		return model, session, &r
	}
	return model, session, nil
}

`, fnName, gd.params, model, session, app, gd.load)
		return
	}
	roles := ""
	if gd.roles != nil {
		roles = fmt.Sprintf(", then `role %s` (404, App.notFound, as for a missing row, when the session's role is not one of them: a cookie check, read when the user logged in)", strings.Join(gd.roles, " | "))
	}
	b.WriteString(comment(fmt.Sprintf("%s runs %s for rq up to its model: `requires %s` (a 303 to App.login, storing keep, when the session's %s is Nothing)%s, then load (with params and the narrowed session). It returns the model and the session view and update take, or the response that ends the request, which stores keep (the session a successful update left, or nil).", fnName, gd.what, strings.Join(gd.requires, ", "), strings.Join(gd.requires, " or "), roles)))
	fmt.Fprintf(b, `func %[1]s(rq *web.Request, params %[2]s, keep any) (%[3]s, %[4]s, *web.Response) {
	var model %[3]s
	app := rq.Env.Session.(%[5]s)
`, fnName, gd.params, model, session, app)
	g.requiresChecks(b, p, gd, "model, ", "keep")
	fmt.Fprintf(b, `	session := %[1]s{%[2]s}
	model, err := eff.RunQuery(rq.Env, %[3]s(params, session))
	if err != nil {
		r := route__fail(err)
		r.Session = keep
		return model, session, &r
	}
	return model, session, nil
}

`, session, strings.Join(sessionFields(p, gd), ", "), gd.load)
}

// sessionFields are the fields of gd's session record built from App's session app: a field gd
// requires unwrapped from its Maybe, the `silo k` key from the parameters.
func sessionFields(p *ir.Program, gd guarded) []string {
	var fs []string
	for _, f := range p.Session.Fields {
		v := "app." + Field(f.Name)
		if slices.Contains(gd.requires, f.Name) {
			v += ".Val"
		}
		if f.Name == gd.silo { // the admin form: the key is the page's parameter
			v = "params." + Field(f.Name)
		}
		fs = append(fs, Field(f.Name)+": "+v)
	}
	return fs
}

// requiresChecks writes gd's `requires` checks on App's session app (each key: a 303 to App.login
// for user, to its `choose` target for another, else 404), its role check (404) and its
// db.Narrow calls, after a mutate build's `requires dropped` placeholders (mutation.go). ret is
// what each stopping return gives before the session ("model, " in page__X), keep the session
// the stopping response stores ("nil": none).
func (g *gen) requiresChecks(b *strings.Builder, p *ir.Program, gd guarded, ret, keep string) {
	session := GoName(gd.session.Module, gd.session.Name)
	stored := "\t\tr.Session = " + keep + "\n" // a 404 stores keep (a redirect takes it as an argument)
	if keep == "nil" {
		stored = ""
	}
	// a mutate build's requires dropped (mutation.go)
	g.requiresPlaceholder(b, gd, "app")
	for _, req := range gd.requires { // M15: a key other than user goes to its `choose` page (none: 404)
		switch ch := p.Choose[req]; {
		case req == "user":
			fmt.Fprintf(b, `	if !app.%[1]s.Ok {
		r := web.RedirectResponse(rq.Login, %[4]s)
		return %[3]s%[2]s{}, &r
	}
`, Field(req), session, ret, keep)
		case ch != nil:
			fmt.Fprintf(b, `	if !app.%[1]s.Ok {
		r := web.RedirectResponse(%[3]s, %[5]s)
		return %[4]s%[2]s{}, &r
	}
`, Field(req), session, strconv.Quote(ch.Path), ret, keep)
		default:
			fmt.Fprintf(b, `	if !app.%[1]s.Ok {
		r := web.NotFoundResponse(html.None())
%[4]s		return %[3]s%[2]s{}, &r
	}
`, Field(req), session, ret, stored)
		}
	}
	if gd.roles != nil {
		fmt.Fprintf(b, `	if %s {
		r := web.NotFoundResponse(html.None())
%s		return %s%s{}, &r
	}
`, g.roleRefused(p, gd, "app"), stored, ret, session)
	}
	g.narrow(b, gd)
}

// narrow writes, for an app with partitions other than the user's, gd's db.Narrow of each such
// key it requires (the session's value) and of its admin form's `silo k` (the parameter's): the
// scoped operations and the silo routing read Env.Keys before the session.
func (g *gen) narrow(b *strings.Builder, gd guarded) {
	for _, req := range gd.requires {
		if req != "user" && g.partitionKey(req) {
			fmt.Fprintf(b, "\tdb.Narrow(rq.Env, %s, app.%s.Val)\n", strconv.Quote(req), Field(req))
		}
	}
	if gd.silo != "" {
		fmt.Fprintf(b, "\tdb.Narrow(rq.Env, %s, params.%s)\n", strconv.Quote(gd.silo), Field(gd.silo))
	}
}

// partitionKey reports whether session field k partitions a table or keys a silo.
func (g *gen) partitionKey(k string) bool {
	if _, ok := g.prog.Silos[k]; ok {
		return true
	}
	for _, tb := range g.prog.Tables {
		if tb.Partition != nil && tb.Partition.Key == k {
			return true
		}
	}
	return false
}

// roleRefused is the Go condition under which gd's (a page's or a route's) `requires user, role
// R | …` refuses the App session app: its role is none of gd's roles. In a mutate build each
// `role dropped` mutant of gd lets every role through under its id (mutation.go).
func (g *gen) roleRefused(p *ir.Program, gd guarded, app string) string {
	var u *types.Union
	for _, f := range p.Session.Fields {
		if f.Name == "role" {
			u, _ = f.Type.(*types.Union)
		}
	}
	if u == nil {
		panic("gen: " + gd.what + " has roles but App's session has no union field `role`")
	}
	var is []string
	for _, r := range gd.roles {
		is = append(is, app+".Role.Tag == "+tagConst(u.Module, u.Name, r))
	}
	cond := "!(" + strings.Join(is, " || ") + ")"
	if len(is) == 1 {
		cond = app + ".Role.Tag != " + tagConst(u.Module, u.Name, gd.roles[0])
	}
	return g.roleDropped(gd) + cond
}

// ---- decoders

// paramsDecoderOf writes name, the decoder of the parameters rec (Go type typ) of what (a page or
// a route) at path.
func (g *gen) paramsDecoderOf(b *strings.Builder, name, what, typ, path string, rec *ir.RecordType) {
	inPath := map[string]bool{}
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			inPath[seg[1:len(seg)-1]] = true
		}
	}
	d := &decoder{g: g, url: true}
	var body strings.Builder
	pathUsed, queryUsed := false, false
	for _, f := range rec.Fields {
		src := "q"
		if inPath[f.Name] {
			src, pathUsed = "pv", true
		} else {
			queryUsed = true
		}
		body.WriteString(d.field(f, src))
	}
	params, q := "_", "_"
	var pre string
	if pathUsed {
		params = "params"
		pre = "pv := url.Values{}\nfor k, v := range params {\npv[k] = []string{v}\n}\n"
	}
	if queryUsed {
		q = "q"
	}
	b.WriteString(comment(fmt.Sprintf("%s decodes %s's parameters: the path's from params, the others from q. A parameter that does not decode is an error (the route answers 404).", name, what)))
	fmt.Fprintf(b, "func %s(%s map[string]string, %s url.Values) (r %s, errs []web.FieldError) {\n%s%sreturn r, errs\n}\n\n",
		name, params, q, typ, pre, body.String())
}

// formDecoder writes the decoder of form f, unless forms has it already.
func (g *gen) formDecoder(b *strings.Builder, f *ir.Form, forms map[*ir.Form]bool) {
	if !forms[f] {
		forms[f] = true
		d := &decoder{g: g}
		var body strings.Builder
		for _, fd := range f.Record.Fields {
			body.WriteString(d.field(fd, "v"))
		}
		v := "v"
		if len(f.Record.Fields) == 0 {
			v = "_"
		}
		b.WriteString(comment(fmt.Sprintf("%s decodes form %s from a request body v. Its errors are shown on the re-rendered form.", decodeName(f.Module, f.Name), f.Name)))
		fmt.Fprintf(b, "func %s(%s url.Values) (r %s, errs []web.FieldError) {\n%sreturn r, errs\n}\n\n",
			decodeName(f.Module, f.Name), v, GoName(f.Module, f.Name), body.String())
	}
}

// decoder emits the statements decoding record fields (see the rules at the top of the file).
// url is set for URL parameters, unset for a form.
type decoder struct {
	g     *gen
	url   bool
	depth int // nesting of List loops, for their variables
}

// clause is one `if`/`else if` of a decoding chain: an if header (init; condition) and its body.
type clause struct{ cond, body string }

// chain is clauses as an if/else-if chain, then final as its else (no else when final is "");
// final alone when there are no clauses.
func chain(cs []clause, final string) string {
	if len(cs) == 0 {
		return final
	}
	var b strings.Builder
	for i, c := range cs {
		if i > 0 {
			b.WriteString(" else ")
		}
		b.WriteString("if " + c.cond + " {\n" + c.body + "}")
	}
	if final != "" {
		b.WriteString(" else {\n" + final + "}")
	}
	b.WriteString("\n")
	return b.String()
}

// field is the code decoding record field f from the url.Values named src into r: src[f.Key()],
// the wire name of a page parameter declared `as "Key"`.
func (d *decoder) field(f ir.FieldT, src string) string {
	def := ""
	if f.Default != nil {
		def = (&fn{g: d.g}).code(f.Default, newScope())
	}
	return d.item(f.Type, f.Brand, src, strconv.Quote(f.Key()), "r."+Field(f.Name), def)
}

// item is the code decoding a value of type t at src[name] (name a Go string literal) into
// target; def is the Go code of its default, "" for none. b is the brand of the scalar.
func (d *decoder) item(t types.Type, b *ir.Brand, src, name, target, def string) string {
	missing := "errs = append(errs, web.Missing(" + name + "))\n"
	if def != "" {
		missing = target + " = " + def + "\n"
	}
	absent := "_, ok := " + src + "[" + name + "]; !ok"
	if a := maybeArg(t); a != nil {
		var cs []clause
		if def != "" {
			cs = append(cs, clause{absent, missing})
		}
		var inner string
		if listArg(a) != nil || maybeArg(a) != nil {
			x := d.temp("just")
			inner = "var " + x + " " + GoType(a) + "\n" + d.item(a, b, src, name, x, "") + target + " = rt.Just(" + x + ")\n"
		} else {
			pcs, final := d.parse(a, b, src, name, "s", func(v string) string { return target + " = rt.Just(" + v + ")\n" })
			inner = chain(pcs, final)
		}
		present := "s != \"\""
		if listArg(a) == nil && maybeArg(a) == nil && scalarKind(a) != "text" {
			present = "strings.TrimSpace(s) != \"\""
		}
		cs = append(cs, clause{"s, _ := web.Text(" + src + ", " + name + ", false); " + present, inner})
		return chain(cs, "")
	}
	if elem := listArg(t); elem != nil {
		x, one := d.temp("item"), d.temp("one")
		d.depth++
		loop := target + " = " + GoType(t) + "{}\nfor _, s := range " + src + "[" + name + "] {\n" +
			one + " := url.Values{" + name + ": {s}}\nvar " + x + " " + GoType(elem) + "\n" +
			d.item(elem, b, one, name, x, "") + target + " = append(" + target + ", " + x + ")\n}\n"
		d.depth--
		if def != "" {
			return chain([]clause{{absent, missing}}, loop)
		}
		return loop
	}
	assign := func(v string) string { return target + " = " + v + "\n" }
	switch scalarKind(t) {
	case "bool":
		var cs []clause
		if d.url && def != "" {
			cs = append(cs, clause{absent, missing})
		}
		pcs, final := d.parse(t, b, src, name, "", assign)
		return chain(append(cs, pcs...), final)
	case "text":
		pcs, final := d.parse(t, b, src, name, "s", assign)
		return chain(append([]clause{{"s, ok := web.Text(" + src + ", " + name + ", false); !ok", missing}}, pcs...), final)
	}
	pcs, final := d.parse(t, b, src, name, "", assign)
	return chain(append([]clause{{"s, _ := web.Text(" + src + ", " + name + ", false); strings.TrimSpace(s) == \"\"", missing}}, pcs...), final)
}

// temp is a variable name for a List loop or a nested Maybe at the current depth.
func (d *decoder) temp(base string) string {
	if d.depth == 0 {
		return base
	}
	return base + strconv.Itoa(d.depth+1)
}

// parse is the chain decoding a present scalar of type t at src[name] (for Text, the value is
// already in text): clauses reporting a malformed value or a failed brand check, and the final
// statement storing the value with assign.
func (d *decoder) parse(t types.Type, b *ir.Brand, src, name, text string, assign func(string) string) ([]clause, string) {
	fieldErr := "errs = append(errs, web.FieldError{Field: " + name + ", Message: msg})\n"
	var cs []clause
	var v string
	switch scalarKind(t) {
	case "text":
		v = text
	case "bool":
		if d.url { // an app-made URL: strict (web.BoolParam)
			cs = append(cs, clause{"x, msg := web.BoolParam(" + src + ", " + name + "); msg != \"\"", fieldErr})
			v = "x"
		} else {
			v = "web.Bool(" + src + ", " + name + ")"
		}
	case "enum":
		u := t.(*types.Union)
		names, _ := enumLists(u)
		cs = append(cs, clause{"i, msg := web.Enum(" + src + ", " + name + ", " + names + "); msg != \"\"", fieldErr})
		v = GoType(t) + "{Tag: " + tagType(u.Module, u.Name) + "(i)}"
	default:
		fn := map[string]string{"int": "web.Int", "float": "web.Float", "date": "web.Date"}[scalarKind(t)]
		cs = append(cs, clause{"x, msg := " + fn + "(" + src + ", " + name + "); msg != \"\"", fieldErr})
		v = "x"
	}
	if check, msg := brandCheck(b, v); check != "" {
		cs = append(cs, clause{"e := web.Check(" + check + ", " + name + ", " + msg + "); e != nil", "errs = append(errs, *e)\n"})
	}
	return cs, assign(v)
}

// brandCheck is the Go condition brand b's check is on value v and its message, "" for a brand
// without one.
func brandCheck(b *ir.Brand, v string) (string, string) {
	if b == nil {
		return "", ""
	}
	switch b.Module + "." + b.Name {
	case "Text.NonEmpty":
		return "web.NonEmpty(" + v + ")", "web.MsgNonEmpty"
	case "Text.Email":
		return "web.Email(" + v + ")", "web.MsgEmail"
	case "Text.Url":
		return "web.URL(" + v + ")", "web.MsgURL"
	case "Text.Secret":
		return "web.Secret(" + v + ")", "web.MsgSecret"
	case "Int.Positive":
		return v + " > 0", strconv.Quote("must be positive")
	case "Int.NonNegative":
		return v + " >= 0", strconv.Quote("must not be negative")
	}
	if b.Check == nil {
		return "", ""
	}
	return checkName(b.Module, b.Name) + "(" + v + ")", strconv.Quote("is not valid")
}

// scalarKind is how a scalar of type t decodes: text, int, float, date, bool or enum.
func scalarKind(t types.Type) string {
	if enumUnion(t) != nil {
		return "enum"
	}
	switch GoType(t) {
	case "string":
		return "text"
	case "int64":
		return "int"
	case "float64":
		return "float"
	case "rt.Date":
		return "date"
	case "bool":
		return "bool"
	}
	panic(fmt.Sprintf("gen: type %s cannot be decoded from a form or URL", types.String(t)))
}

// listArg is the a of `List a`, or nil.
func listArg(t types.Type) types.Type {
	if app, ok := t.(*types.App); ok && app.Con == "List" && len(app.Args) == 1 {
		return app.Args[0]
	}
	return nil
}
