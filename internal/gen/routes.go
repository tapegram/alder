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
// Per page module M:
//
//	decode__M__Params(params, q)    the page's parameters: path ones from params, the others
//	                                from the query q; defaults applied; an error is a 404
//	decode__M__<Msg>Params(q)       a message's parameters from its URL's query (404)
//	decode__M__<Form>(v)            a form from the request body; errors re-render the page
//	params__M(p)                    the page's parameters as text, carried by every message
//	                                URL the page renders (rt.MsgRef.URL: "_p.<name>" keys)
//	page__M(rq, params, keep)       `requires` (a 303 to App.login storing keep), then load (a
//	                                failure's response also stores keep)
//	update__M(rq, params, msg, st)  load, update, and the Nav: Rerender → under Datastar load,
//	                                view, patched; else a 303 to the page (rt.PageAt: PRG);
//	                                Redirect p → 303; Reject t → load, view with t on the form
//	                                state st (patched under Datastar)
//	Route__M__page()                GET <path>: decode (404), page__M, view
//	Route__M__<Msg>()               POST /_m/M/<Msg>: decode the page's parameters (the query's
//	                                "_p." keys: rt.PageParams; 404 without them) and the message's
//	                                (404), the form (re-render with errors), update__M
//
// and for App's messages update__App (Rerender and Reject redirect back to the Referer) and
// Route__App__<Msg>. Routes lists every route: pages, then messages. The book's request
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
	var routes, msgRoutes []string
	usesForms := false
	for _, pg := range p.Pages {
		routes = append(routes, routeFunc(pg.Module, "")+"()")
		if pg.Update != nil {
			for _, m := range pg.Msgs {
				msgRoutes = append(msgRoutes, routeFunc(pg.Module, m.Name)+"()")
				usesForms = usesForms || m.Form != nil
			}
		}
	}
	if p.AppUpdate != nil {
		for _, m := range p.AppMsgs {
			msgRoutes = append(msgRoutes, routeFunc("App", m.Name)+"()")
		}
	}
	b.WriteString(comment("Routes are the app's endpoints for web.NewRouter: every page (GET), then every message of the pages and of App (POST), in module order."))
	all := append(routes, msgRoutes...)
	if len(all) == 0 {
		b.WriteString("var Routes = []web.Route{}\n\n")
	} else {
		b.WriteString("var Routes = []web.Route{\n" + strings.Join(all, ",\n") + ",\n}\n\n")
	}
	for _, pg := range p.Pages {
		g.pageRoutes(&b, p, pg)
	}
	if p.AppUpdate != nil && len(p.AppMsgs) > 0 {
		g.appRoutes(&b, p)
	}
	if g.tests {
		g.encoders(&b, p) // browser.go
	}
	if len(p.Silos) > 0 {
		b.WriteString(`// route__fail answers a load or update that did not succeed: 404 (App.notFound) for
// Query.notFound and Task.notFound, 503 (App.maintenance) for a silo that is not served, else
// 500 (App.error).
func route__fail(err error) web.Response {
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
		b.WriteString(`// route__fail answers a load or update that did not succeed: 404 (App.notFound) for
// Query.notFound and Task.notFound, else 500 (App.error).
func route__fail(err error) web.Response {
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

// pageRoutes writes page pg's decoders, page__/update__ functions and routes.
func (g *gen) pageRoutes(b *strings.Builder, p *ir.Program, pg *ir.Page) {
	m := pg.Module
	g.paramsDecoder(b, pg)
	hasUpdate := pg.Update != nil && len(pg.Msgs) > 0
	forms := map[*ir.Form]bool{}
	if hasUpdate {
		for _, msg := range pg.Msgs {
			g.msgDecoders(b, m, msg, forms)
		}
	}
	g.pageFunc(b, p, pg)
	checks := ""
	if hasUpdate && g.optionChecks(b, pg) {
		checks = fmt.Sprintf(`	if st != nil {
		if errs := %s(st.Form, st.Values, model, session); errs != nil {
			st.Errors, st.PageParams = route__errors(errs), %s(params)
			return web.PageResponse(%s(model, session), st, nil, true)
		}
	}
`, optionsFunc(m), paramsFunc(m), GoName(m, "view"))
	}
	keep := func(k string) string { return ", " + k }
	g.paramsKVs(b, pg)
	enter := ""
	if hasUpdate {
		enter = g.enter(b, p, pg)
	}
	if hasUpdate {
		b.WriteString(comment(fmt.Sprintf("%s runs page %s's update for msg and answers its Nav (book 06): load, update (a Task, whose session is stored when it succeeds), then for Redirect p a 303 to p; for Reject t on a form (st, the state of the form that sent the message; nil for a message without one) load and view again with t on the form (patched into the page under Datastar); for Rerender (and a Reject without a form) the same under Datastar, else a 303 to the page (Post/Redirect/Get: a refresh does not post again). A failed Task changes nothing: 500 (App.error), or 404 for Query.notFound or Task.notFound.", updateFunc(m), m)))
		fmt.Fprintf(b, `func %[1]s(rq *web.Request, params %[2]s, msg %[3]s, st *html.FormState) web.Response {
	model, session, stop := %[4]s(rq, params%[5]s)
	if stop != nil {
		return *stop
	}
%[11]s%[12]s	nav, err := eff.Run(rq.Env, eff.KTask, %[6]s(msg, model, session))
%[13]s	nav, rq.Notice = nav.Unnotice()
	switch {
	case nav.Tag == rt.NavRedirect:
		return web.RedirectResponse(rt.PageURL(nav.Page), rq.Env.Session)
	case nav.Tag == rt.NavReject && st != nil:
		st.FormError = nav.Text
	case !rq.Datastar:
		return web.RedirectResponse(rt.PageAt(%[10]s, %[9]s(params)), rq.Env.Session)
	default:
		st = &html.FormState{}
	}
	st.PageParams = %[9]s(params)
	model, session, stop = %[4]s(rq, params%[7]s)
	if stop != nil {
		return *stop
	}
	return web.PageResponse(%[8]s(model, session), st, rq.Env.Session, true)
}

`, updateFunc(m), GoName(m, "Params"), GoName(m, "Msg"), pageFunc(m), keep("nil"), GoName(m, "update"), keep("rq.Env.Session"), GoName(m, "view"), paramsFunc(m), strconv.Quote(pg.Path), checks, enter, updateFailure(p))
	}

	// the GET route
	steps := "decode the parameters (404 when they do not), load (404 on Query.notFound, 500 on a failure), view (the model also in rq.Model, which a test's browser reads)."
	if len(pg.Requires) > 0 {
		steps = "decode the parameters (404 when they do not), `requires` (303 to App.login), load (404 on Query.notFound, 500 on a failure), view (the model also in rq.Model, which a test's browser reads)."
	}
	if pg.Roles != nil {
		steps = "decode the parameters (404 when they do not), `requires` (303 to App.login), the role (404), load (404 on Query.notFound, 500 on a failure), view (the model also in rq.Model, which a test's browser reads)."
	}
	b.WriteString(comment(fmt.Sprintf("%s serves page %s, GET %s (book 06): %s", routeFunc(m, ""), m, pg.Path, steps)))
	fmt.Fprintf(b, `func %s() web.Route {
	return web.Route{Method: "GET", Pattern: %s, Handle: func(rq *web.Request) web.Response {
		params, errs := %s(rq.Params, rq.Query)
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
		model, session, stop := %s(rq, params%s)
		if stop != nil {
			return *stop
		}
		rq.Model = model
		return web.PageResponse(%s(model, session), &html.FormState{PageParams: %s(params)}, rq.Session, false)
	}}
}

`, routeFunc(m, ""), strconv.Quote(pg.Path), decodeName(m, "Params"), pageFunc(m), keep("nil"), GoName(m, "view"), paramsFunc(m))

	if !hasUpdate {
		return
	}
	for _, msg := range pg.Msgs {
		text := fmt.Sprintf("%s serves message %s of page %s, POST %s (book 06): decode the page's parameters (the message URL's _p. keys, which the rendered page put there)", routeFunc(m, msg.Name), msg.Name, m, msgPath(m, msg.Name))
		if msgHasParams(msg) {
			text += " and the message's (from the query)"
		}
		text += ": 404 when they do not decode;"
		if msg.Form != nil {
			text += fmt.Sprintf(" decode form %s from the body: when it does not, re-render the page with the submitted values (no Text.Secret field) and the errors, without calling update;", msg.Form.Name)
		}
		text += " then " + updateFunc(m) + "."
		b.WriteString(comment(text))
		fmt.Fprintf(b, `func %s() web.Route {
	return web.Route{Method: "POST", Pattern: %s, Handle: func(rq *web.Request) web.Response {
		params, errs := %s(rt.PageParams(rq.Query))
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
`, routeFunc(m, msg.Name), strconv.Quote(msgPath(m, msg.Name)), decodeName(m, "Params"))
		g.msgBody(b, m, msg, func(form, st string) string {
			return fmt.Sprintf(`model, session, stop := %s(rq, params%s)
			if stop != nil {
				return *stop
			}
			return web.PageResponse(%s(model, session), %s, nil, true)
`, pageFunc(m), keep("nil"), GoName(m, "view"), st)
		}, func(value, st string) string {
			return fmt.Sprintf("return %s(rq, params, %s, %s)\n", updateFunc(m), value, st)
		})
		b.WriteString("}}\n}\n\n")
	}
}

// msgPath is the URL message msg of module m is posted to.
func msgPath(m, msg string) string { return "/_m/" + m + "/" + msg }

// msgHasParams reports whether msg has parameters besides its form.
func msgHasParams(msg *ir.Msg) bool {
	if msg.Params == nil {
		return false
	}
	for _, f := range msg.Params.Fields {
		if f.Name != "form" {
			return true
		}
	}
	return false
}

// optionInput is a `<Select options={…}>` of a page's view: the form ("M.F") and field it binds,
// its options' expression, the name of the Go function computing them again, and whether a blank
// value is allowed (a Maybe field, a field with a default).
type optionInput struct {
	form, field string
	input       *ir.Input
	fn          string
	blank       bool
}

// optionInputs are the selects with `options={…}` in page pg's view, in the order they come. A
// field drawn by two selects (in two branches of an `if`) has both: a value either offers is
// accepted.
func optionInputs(pg *ir.Page) []optionInput {
	var out []optionInput
	if pg.View == nil {
		return out
	}
	n := map[string]int{}
	var visit func(e ir.Expr)
	visit = func(e ir.Expr) {
		if e == nil {
			return
		}
		if x, ok := e.(*ir.Input); ok && x.Kind == ir.InputSelect && x.Options != nil && x.Form != nil {
			key := x.Form.Module + "." + x.Form.Name
			blank := false
			for _, fd := range x.Form.Record.Fields {
				if fd.Name == x.Field {
					u, isMaybe := fd.Type.(*types.Union)
					blank = fd.Default != nil || (isMaybe && u.Module == "Prelude" && u.Name == "Maybe")
				}
			}
			fn := "choices__" + GoName(pg.Module, x.Form.Name) + "__" + Field(x.Field)
			if n[key+"."+x.Field]++; n[key+"."+x.Field] > 1 {
				fn += strconv.Itoa(n[key+"."+x.Field])
			}
			out = append(out, optionInput{key, x.Field, x, fn, blank})
		}
		for _, k := range ir.Children(e) {
			if k != nil {
				visit(k)
			}
		}
	}
	visit(pg.View.Body)
	return out
}

// optionsFunc checks the posted values of page M's selects with `options={…}` ("options__M").
func optionsFunc(module string) string { return "options__" + strings.ReplaceAll(module, ".", "__") }

// optionChecks writes, for page pg whose view has selects with `options={…}`, a function per
// select computing its options again from the view's parameters (as the view does), and
// options__M: the field errors of the values posted on form st.Form that its selects did not
// offer. update__M calls it once the page is loaded, before update. It reports whether it wrote
// them.
func (g *gen) optionChecks(b *strings.Builder, pg *ir.Page) bool {
	ft, ok := pg.View.Type.(*types.Fun)
	ins := optionInputs(pg)
	if !ok || len(ins) == 0 {
		return false
	}
	m := pg.Module
	for _, oi := range ins {
		v := &ir.Value{Module: m, Name: "choices", Params: pg.View.Params, Body: oi.input.Options,
			Type: &types.Fun{Params: ft.Params, Result: oi.input.Options.Type()}}
		b.WriteString(comment(fmt.Sprintf("%s computes again the options page %s's view offers for field %s of form %s, to check a posted value (chapter 5).", oi.fn, m, oi.field, oi.form)))
		g.valueAs(b, v, oi.fn)
	}
	model, session := GoName(m, "Model"), GoName(pg.Session.Module, pg.Session.Name)
	b.WriteString(comment(fmt.Sprintf("%s are the field errors of values posted on form (a page %s form) that its `options={…}` selects did not offer: \"must be one of the options\" (chapter 5). The options are computed again from the model, loaded for the message as for the page.", optionsFunc(m), m)))
	fmt.Fprintf(b, "func %s(form string, values map[string]string, model %s, session %s) []web.FieldError {\nvar errs []web.FieldError\nswitch form {\n", optionsFunc(m), model, session)
	var forms []string
	byForm := map[string][]optionInput{}
	for _, oi := range ins {
		if byForm[oi.form] == nil {
			forms = append(forms, oi.form)
		}
		byForm[oi.form] = append(byForm[oi.form], oi)
	}
	for _, form := range forms {
		fmt.Fprintf(b, "case %s:\n", strconv.Quote(form))
		var fields []string
		byField := map[string][]optionInput{}
		for _, oi := range byForm[form] {
			if byField[oi.field] == nil {
				fields = append(fields, oi.field)
			}
			byField[oi.field] = append(byField[oi.field], oi)
		}
		for _, field := range fields {
			var lists []string
			for _, oi := range byField[field] {
				elem := GoType(oi.input.Options.Type().(*types.App).Args[0])
				lists = append(lists, fmt.Sprintf("html.Choices(%s(model, session), func(_o %s) html.Choice { return html.Choice{Value: _o.%s, Label: _o.%s} })", oi.fn, elem, Field("value"), Field("label")))
			}
			choices := lists[0]
			if len(lists) > 1 {
				for _, l := range lists[1:] {
					choices = "append(" + choices + ", " + l + "...)"
				}
			}
			fmt.Fprintf(b, "if !html.Offered(values[%s], %s, %t) {\nerrs = append(errs, web.FieldError{Field: %s, Message: web.MsgOption})\n}\n",
				strconv.Quote(field), choices, byField[field][0].blank, strconv.Quote(field))
		}
	}
	b.WriteString("}\nreturn errs\n}\n\n")
	return true
}

// msgBody writes the part of message msg's handler (of module m) after the page's parameters:
// decode its parameters (404) and its form (formErr: the statements answering form errors, with
// the FormState expression), then run (the statements answering the message value and the
// FormState of a Reject).
func (g *gen) msgBody(b *strings.Builder, m string, msg *ir.Msg, formErr func(form, st string) string, run func(value, st string) string) {
	switch {
	case msgHasParams(msg):
		fmt.Fprintf(b, `p, errs := %s(rq.Query)
if errs != nil {
	return web.NotFoundResponse(html.None())
}
`, decodeName(m, msg.Params.Name))
	case msg.Params != nil:
		fmt.Fprintf(b, "p := %s{}\n", GoName(m, msg.Params.Name))
	}
	value := ctorName(m, "Msg", msg.Name)
	if msg.Params != nil {
		value += "(p)"
	}
	st := "nil"
	if msg.Form != nil {
		f := msg.Form
		formName := strconv.Quote(f.Module + "." + f.Name)
		key := "rt.MsgKey(" + strconv.Quote(msgPath(m, msg.Name)) + ", rq.Query)"
		args := []string{"rq.Form"}
		for _, fd := range f.Record.Fields {
			if isSecret(fd.Brand) {
				args = append(args, strconv.Quote(fd.Name))
			}
		}
		if m == "App" { // App renders no page: a form error goes back to the page, and no state is kept
			fmt.Fprintf(b, "form, errs := %s(rq.Form)\nif errs != nil {\n%s}\np.Form = form\n",
				decodeName(f.Module, f.Name), formErr(formName, "nil"))
		} else {
			fmt.Fprintf(b, "values := route__values(%s)\nform, errs := %s(rq.Form)\nif errs != nil {\n%s}\np.Form = form\n",
				strings.Join(args, ", "), decodeName(f.Module, f.Name),
				formErr(formName, "&html.FormState{Form: "+formName+", Msg: "+key+", Values: values, Errors: route__errors(errs), PageParams: "+paramsFunc(m)+"(params)}"))
			st = "&html.FormState{Form: " + formName + ", Msg: " + key + ", Values: values}"
		}
	}
	b.WriteString(run(value, st))
}

// isSecret reports whether b is the stdlib's Text.Secret.
func isSecret(b *ir.Brand) bool { return b != nil && b.Module == "Text" && b.Name == "Secret" }

// paramsKVs writes params__M: page pg's parameters as text, the PageParams every message URL
// on its rendering carries (left out at a default, as in a link).
func (g *gen) paramsKVs(b *strings.Builder, pg *ir.Page) {
	m := pg.Module
	f := &fn{g: g}
	s := newScope()
	p := s.declare("p")
	pv := &ir.Var{Typed: ir.Typed{Ty: &types.Record{Module: m, Name: "Params"}}, Kind: ir.VarLocal, Name: "p"}
	var items []kvItem
	for _, fd := range pg.Params.Fields {
		items = append(items, kvItem{name: fd.Name, t: fd.Type, v: &ir.Field{Typed: ir.Typed{Ty: fd.Type}, Expr: pv, Name: fd.Name}, def: fd.Default, omitBlank: true})
	}
	kvs := f.kvList(items, s)
	if len(items) == 0 {
		p = "_"
	}
	b.WriteString(comment(fmt.Sprintf("%s are page %s's parameters as text for its message URLs (html.FormState.PageParams): each message route decodes them back from the URL's _p. keys.", paramsFunc(m), m)))
	fmt.Fprintf(b, "func %s(%s %s) []rt.KV {\nreturn %s\n}\n\n", paramsFunc(m), p, GoName(m, "Params"), kvs)
}

// pageFunc writes page__M: `requires`, then load.
func (g *gen) pageFunc(b *strings.Builder, p *ir.Program, pg *ir.Page) {
	m := pg.Module
	model, session := GoName(m, "Model"), GoName(pg.Session.Module, pg.Session.Name)
	app := GoName(p.Session.Module, p.Session.Name)
	if len(pg.Requires) == 0 {
		b.WriteString(comment(fmt.Sprintf("%s runs page %s for rq up to its model: load (with params and the session of rq.Env). It returns the model and the session view and update take, or the response that ends the request, which stores keep (the session a successful update left, or nil).", pageFunc(m), m)))
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

`, pageFunc(m), GoName(m, "Params"), model, session, app, GoName(m, "load"))
		return
	}
	var fs []string
	for _, f := range p.Session.Fields {
		v := "app." + Field(f.Name)
		if slices.Contains(pg.Requires, f.Name) {
			v += ".Val"
		}
		if f.Name == pg.Silo { // the admin form: the key is the page's parameter
			v = "params." + Field(f.Name)
		}
		fs = append(fs, Field(f.Name)+": "+v)
	}
	roles := ""
	if pg.Roles != nil {
		roles = fmt.Sprintf(", then `role %s` (404, App.notFound, as for a missing row, when the session's role is not one of them: a cookie check, read when the user logged in)", strings.Join(pg.Roles, " | "))
	}
	b.WriteString(comment(fmt.Sprintf("%s runs page %s for rq up to its model: `requires %s` (a 303 to App.login, storing keep, when the session's %s is Nothing)%s, then load (with params and the narrowed session). It returns the model and the session view and update take, or the response that ends the request, which stores keep (the session a successful update left, or nil).", pageFunc(m), m, strings.Join(pg.Requires, ", "), strings.Join(pg.Requires, " or "), roles)))
	fmt.Fprintf(b, `func %[1]s(rq *web.Request, params %[2]s, keep any) (%[3]s, %[4]s, *web.Response) {
	var model %[3]s
	app := rq.Env.Session.(%[5]s)
`, pageFunc(m), GoName(m, "Params"), model, session, app)
	g.requiresPlaceholder(b, pg, "app") // a mutate build's requires dropped (mutation.go)
	for _, req := range pg.Requires {   // M15: a key other than user goes to its `choose` page (none: 404)
		switch ch := p.Choose[req]; {
		case req == "user":
			fmt.Fprintf(b, `	if !app.%[1]s.Ok {
		r := web.RedirectResponse(rq.Login, keep)
		return model, %[2]s{}, &r
	}
`, Field(req), session)
		case ch != nil:
			fmt.Fprintf(b, `	if !app.%[1]s.Ok {
		r := web.RedirectResponse(%[3]s, keep)
		return model, %[2]s{}, &r
	}
`, Field(req), session, strconv.Quote(ch.Path))
		default:
			fmt.Fprintf(b, `	if !app.%[1]s.Ok {
		r := web.NotFoundResponse(html.None())
		r.Session = keep
		return model, %[2]s{}, &r
	}
`, Field(req), session)
		}
	}
	if pg.Roles != nil {
		fmt.Fprintf(b, `	if %s {
		r := web.NotFoundResponse(html.None())
		r.Session = keep
		return model, %s{}, &r
	}
`, g.roleRefused(p, pg, "app"), session)
	}
	g.narrow(b, p, pg)
	fmt.Fprintf(b, `	session := %[4]s{%[7]s}
	model, err := eff.RunQuery(rq.Env, %[8]s(params, session))
	if err != nil {
		r := route__fail(err)
		r.Session = keep
		return model, session, &r
	}
	return model, session, nil
}

`, pageFunc(m), GoName(m, "Params"), model, session, app, Field(firstRequired(pg)), strings.Join(fs, ", "), GoName(m, "load"))
}

// updateFailure is the update route's answer to a failed Task: route__fail, and for an app with
// `via` fields a Session.put the membership check refused (eff.ErrFail.NotMember: a crafted
// switch to an org the user is no member of) is Reject "You are not a member of that
// workspace." on the form that sent the message, else 404 — not a 500.
func updateFailure(p *ir.Program) string {
	if len(p.Via) == 0 {
		return "\tif err != nil {\n\t\treturn route__fail(err)\n\t}\n"
	}
	return `	if err != nil {
		var nm *eff.ErrFail
		switch {
		case !errors.As(err, &nm) || !nm.NotMember:
			return route__fail(err)
		case st == nil:
			return web.NotFoundResponse(html.None())
		}
		nav = rt.Reject("You are not a member of that workspace.")
	}
`
}

// enter writes, for an app with silos, the function naming the store of each arm of page pg's
// update (store__M: its silo key, "" for the shared store; the checker's Arm.Store: a case for
// each arm whose store is not the `_` arm's, the default) and
// returns the statement binding the Task to it (db.Enter) before update__M runs it: a silo not
// served answers the maintenance page. "" without silos.
func (g *gen) enter(b *strings.Builder, p *ir.Program, pg *ir.Page) string {
	if len(p.Silos) == 0 {
		return ""
	}
	m := pg.Module
	def := "" // the store of the `_` arm (the others), shared when there is none
	for _, a := range pg.Arms {
		if a.Msg == "" {
			def = a.Store.Silo
		}
	}
	var cases []string
	for _, a := range pg.Arms {
		// every arm whose store is not the default's, the shared one included: an explicit
		// shared arm beside a `_` silo arm must not inherit the silo
		if a.Msg != "" && a.Store.Silo != def {
			cases = append(cases, fmt.Sprintf("case %s:\nreturn %s\n", tagConst(m, "Msg", a.Msg), strconv.Quote(a.Store.Silo)))
		}
	}
	fn := "store__" + strings.ReplaceAll(m, ".", "__")
	b.WriteString(comment(fmt.Sprintf("%s is the store the Task of page %s's update for msg writes to (the checker's one-store rule): the silo key, \"\" for the shared store.", fn, m)))
	param := "msg"
	if len(cases) == 0 {
		param = "_"
	}
	fmt.Fprintf(b, "func %s(%s %s) string {\n", fn, param, GoName(m, "Msg"))
	if len(cases) > 0 {
		b.WriteString("switch msg.Tag {\n" + strings.Join(cases, "") + "}\n")
	}
	fmt.Fprintf(b, "return %s\n}\n\n", strconv.Quote(def))
	return fmt.Sprintf("\tif err := db.Enter(rq.Env, %s(msg)); err != nil {\n\t\treturn route__fail(err)\n\t}\n", fn)
}

// narrow writes, for an app with partitions other than the user's, page pg's db.Narrow of each
// such key it requires (the session's value) and of its admin form's `silo k` (the parameter's):
// the scoped operations and the silo routing read Env.Keys before the session.
func (g *gen) narrow(b *strings.Builder, p *ir.Program, pg *ir.Page) {
	for _, req := range pg.Requires {
		if req != "user" && g.partitionKey(req) {
			fmt.Fprintf(b, "\tdb.Narrow(rq.Env, %s, app.%s.Val)\n", strconv.Quote(req), Field(req))
		}
	}
	if pg.Silo != "" {
		fmt.Fprintf(b, "\tdb.Narrow(rq.Env, %s, params.%s)\n", strconv.Quote(pg.Silo), Field(pg.Silo))
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

// roleRefused is the Go condition under which page pg's `requires user, role R | …` refuses
// the App session app: its role is none of the page's roles. In a mutate build each `role
// dropped` mutant of the page lets every role through under its id (mutation.go).
func (g *gen) roleRefused(p *ir.Program, pg *ir.Page, app string) string {
	var u *types.Union
	for _, f := range p.Session.Fields {
		if f.Name == "role" {
			u, _ = f.Type.(*types.Union)
		}
	}
	if u == nil {
		panic("gen: page " + pg.Module + " has roles but App's session has no union field `role`")
	}
	var is []string
	for _, r := range pg.Roles {
		is = append(is, app+".Role.Tag == "+tagConst(u.Module, u.Name, r))
	}
	cond := "!(" + strings.Join(is, " || ") + ")"
	if len(is) == 1 {
		cond = app + ".Role.Tag != " + tagConst(u.Module, u.Name, pg.Roles[0])
	}
	return g.roleDropped(pg) + cond
}

// appRoutes writes the decoders and routes of App's messages, and update__App.
func (g *gen) appRoutes(b *strings.Builder, p *ir.Program) {
	forms := map[*ir.Form]bool{}
	for _, msg := range p.AppMsgs {
		g.msgDecoders(b, "App", msg, forms)
	}
	app := GoName(p.Session.Module, p.Session.Name)
	b.WriteString(comment(fmt.Sprintf("%s runs App's update for msg and answers its Nav: Redirect p is a 303 to p; Rerender and Reject t are a 303 back to the page that sent the message (the Referer, else /), as App has no page of its own to render (a Reject's text is not shown). The session the Task leaves is stored; a failed Task changes nothing: 500 (App.error), or 404 for Query.notFound or Task.notFound.", updateFunc("App"))))
	fmt.Fprintf(b, `func %s(rq *web.Request, msg %s) web.Response {
	nav, err := eff.Run(rq.Env, eff.KTask, %s(msg, rq.Env.Session.(%s)))
	if err != nil {
		return route__fail(err)
	}
	nav, rq.Notice = nav.Unnotice()
	if nav.Tag == rt.NavRedirect {
		return web.RedirectResponse(rt.PageURL(nav.Page), rq.Env.Session)
	}
	return web.RedirectResponse(rt.BackURL(rq.HTTP.Referer()), rq.Env.Session)
}

`, updateFunc("App"), GoName("App", "Msg"), GoName("App", "update"), app)
	for _, msg := range p.AppMsgs {
		var steps []string
		if msgHasParams(msg) {
			steps = append(steps, "decode its parameters from the query (404 when they do not decode)")
		}
		if msg.Form != nil {
			steps = append(steps, fmt.Sprintf("decode form %s from the body (when it does not, a 303 back to the page that sent the message, without calling update)", msg.Form.Name))
		}
		steps = append(steps, updateFunc("App"))
		if len(steps) > 1 {
			steps[len(steps)-1] = "then " + steps[len(steps)-1]
		}
		b.WriteString(comment(fmt.Sprintf("%s serves App's message %s, POST %s: %s.", routeFunc("App", msg.Name), msg.Name, msgPath("App", msg.Name), strings.Join(steps, "; "))))
		fmt.Fprintf(b, "func %s() web.Route {\nreturn web.Route{Method: \"POST\", Pattern: %s, Handle: func(rq *web.Request) web.Response {\n",
			routeFunc("App", msg.Name), strconv.Quote(msgPath("App", msg.Name)))
		g.msgBody(b, "App", msg, func(string, string) string {
			return "return web.RedirectResponse(rt.BackURL(rq.HTTP.Referer()), nil)\n"
		}, func(value, _ string) string {
			return fmt.Sprintf("return %s(rq, %s)\n", updateFunc("App"), value)
		})
		b.WriteString("}}\n}\n\n")
	}
}

// ---- decoders

// paramsDecoder writes decode__M__Params for page pg.
func (g *gen) paramsDecoder(b *strings.Builder, pg *ir.Page) {
	m := pg.Module
	inPath := map[string]bool{}
	for _, seg := range strings.Split(pg.Path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			inPath[seg[1:len(seg)-1]] = true
		}
	}
	d := &decoder{g: g, url: true}
	var body strings.Builder
	pathUsed, queryUsed := false, false
	for _, f := range pg.Params.Fields {
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
	b.WriteString(comment(fmt.Sprintf("%s decodes page %s's parameters: the path's from params, the others from q. A parameter that does not decode is an error (the route answers 404).", decodeName(m, "Params"), m)))
	fmt.Fprintf(b, "func %s(%s map[string]string, %s url.Values) (r %s, errs []web.FieldError) {\n%s%sreturn r, errs\n}\n\n",
		decodeName(m, "Params"), params, q, GoName(m, "Params"), pre, body.String())
}

// msgDecoders writes the decoders of message msg of module m: its parameters (when it has any
// besides the form) and its form (unless forms has it already).
func (g *gen) msgDecoders(b *strings.Builder, m string, msg *ir.Msg, forms map[*ir.Form]bool) {
	if msgHasParams(msg) {
		d := &decoder{g: g, url: true}
		var body strings.Builder
		for _, f := range msg.Params.Fields {
			if f.Name != "form" {
				body.WriteString(d.field(f, "q"))
			}
		}
		b.WriteString(comment(fmt.Sprintf("%s decodes the parameters of message %s from its URL's query q; the form is decoded apart. A parameter that does not decode is an error (the route answers 404).", decodeName(m, msg.Params.Name), msg.Name)))
		fmt.Fprintf(b, "func %s(q url.Values) (r %s, errs []web.FieldError) {\n%sreturn r, errs\n}\n\n",
			decodeName(m, msg.Params.Name), GoName(m, msg.Params.Name), body.String())
	}
	if f := msg.Form; f != nil && !forms[f] {
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

// field is the code decoding record field f from the url.Values named src into r.
func (d *decoder) field(f ir.FieldT, src string) string {
	def := ""
	if f.Default != nil {
		def = (&fn{g: d.g}).code(f.Default, newScope())
	}
	return d.item(f.Type, f.Brand, src, strconv.Quote(f.Name), "r."+Field(f.Name), def)
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

// firstRequired is the first session field page pg requires, "" for none.
func firstRequired(pg *ir.Page) string {
	if len(pg.Requires) == 0 {
		return ""
	}
	return pg.Requires[0]
}
