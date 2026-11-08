package gen

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// The browser steps of a test (test builds only; testNode refuses them in a production build).
// Each is an eff.Call named as the trace shows it, whose function drives the test's browser
// (rtesting.BrowserOf(_e)); the effects of the requests it makes follow it on the trace. R is
// the step's result record ({ model | answer, session, doc, notices, toasts, url, status }), S App's Session (struct{}
// without an App module):
//
//	TestLogin      "Test.login": BrowserOf(_e).Login(session), and the test's own session becomes
//	               session (_e.Session), so the scoped Db operations a test runs act for that user
//	TestGet        "Test.get M.R" / "Test.open M.R": _r, _err := _b.GetPage(rt.RouteURL(route),
//	               "<requires>") (Open for open); for get, the model the route's GET rendered
//	               (rq.Model, through web.Options.Served):
//	               _m, _err := rtesting.ModelOf[M](_u, _r);
//	               R{Model: _m, Session: rtesting.SessionOf[S](_r.Session), Doc: rtesting.DocOf(_r.Doc)}
//	TestGet        "Test.fetch M.R" (Fetch): _r, _err := _b.Fetch(rt.RouteURL(route)); _r is the
//	               result as is (rtesting.Fetched is the record's Go type)
//	TestSubmit     "Test.submit F": _b.SubmitRoute(route, "F", encode__M__F(record))
//	TestSubmitRaw  "Test.submitRaw F": rtesting.RawFields("F", <F's fields>, fields), then
//	               SubmitRoute
//	TestClick      "Test.click M.R": _b.ClickRoute(route, rtesting.MsgName("M.R", params))
//	TestRefresh    "Test.refresh": _r, _err := rtesting.BrowserOf(_e).Refresh() (#160); R{Answer:
//	               _r.Answer, …}
//
// Each names the route value, an rt.Route literal (routeRef) with its parameters bound once
// (routeTarget); get's model is the route's Model, its first `requires` named in GetPage. A
// route's form has an encoder, encode__M__F.
//
// A query (Test.text, texts, has, count, attr) whose selector is a literal calls the Sel form of
// its rtesting function with the selector parsed once, a package variable selector__N
// (app__alder.go); any other selector is parsed when the query runs.

// member is Test.member u k (M15): a row of the membership table of the session's `via` field
// (E0032 allows one), with its user column u and its key column k, every other
// column its default, or NULL (the checker refuses a membership table with another required column).
//
//	rtesting.Member(func(_e *eff.Env, _u, _k string) error {
//		_, err := db.InsertRow(…, table__Membership__Membership, []any{nil, _u, _k}, _e.Now())
//		return err
//	}, u, k)
func (f *fn) member(x *ir.Call, s *scope) string {
	g := f.g
	var mt *ir.Table
	var key string
	for _, k := range slices.Sorted(maps.Keys(g.prog.Via)) { // one (E0032 refuses a second)
		if mt == nil {
			mt, key = g.prog.Via[k], k
		}
	}
	if mt == nil {
		panic("gen: Test.member: the session has no `via` field")
	}
	ucol, kcol := g.memberCols(g.prog, mt, key)
	var vals []string
	for _, fd := range mt.Record.Fields {
		switch fd.Name {
		case ucol:
			vals = append(vals, "_u")
		case kcol:
			vals = append(vals, "_k")
		default: // the checker made sure it is a Maybe, has a default, or is filled by the store
			if fd.Default != nil {
				vals = append(vals, encode(fd.Type, f.typedExpr(fd.Default, s)))
			} else {
				vals = append(vals, "nil")
			}
		}
	}
	return "rtesting.Member(func(_e *eff.Env, _u, _k string) error {\n_, err := db.InsertRow(" + dbArgs("_e") + ", " + tableName(mt.Module, mt.Name) +
		", []any{" + strings.Join(vals, ", ") + "}, _e.Now())\nreturn err\n}, " + f.code(x.Args[0], s) + ", " + f.code(x.Args[1], s) + ")"
}

// browserStep is step x (a Test* browser node of effect type x.Type()).
func (f *fn) browserStep(x ir.Expr, s *scope) string {
	t := x.Type()
	r := GoType(effResult(t))
	result := func(c *scope, first, value, res string) string {
		return "return " + r + "{" + first + ": " + value + ", Session: rtesting.SessionOf[" + f.sessionGo(t) + "](" + res + ".Session), Doc: rtesting.DocOf(" + res + ".Doc), Notices: " + res + ".Notices, Toasts: " + res + ".Toasts, Url: " + res + ".URL, Status: " + res + ".Status}, nil\n"
	}
	fail := func(c *scope, err string) string {
		z := f.temp(c, "_z")
		return "if " + err + " != nil {\nvar " + z + " " + r + "\nreturn " + z + ", " + err + "\n}\n"
	}
	switch x := x.(type) {
	case *ir.TestLogin:
		return f.callStmts("Test.login", true, t, s, func(env string, c *scope) string {
			err := f.temp(c, "_err")
			check := ""
			if len(f.g.prog.Via) > 0 { // M15: a `via` field must name a membership (session__via)
				sess, ck := f.temp(c, "_s"), f.temp(c, "_ck")
				check = sess + " := " + f.code(x.Session, c) + "\n" +
					ck + ", " + err + " := rtesting.CheckSession(" + env + ", " + sess + ")\nif " + err + " != nil {\nreturn struct{}{}, " + err + "\n}\n" +
					env + ".Session = " + ck + "\n"
			} else {
				check = env + ".Session = " + f.code(x.Session, c) + "\n"
			}
			// the session is the test's own first (a failed step restores it: eff.Run)
			return check +
				"if " + err + " := rtesting.BrowserOf(" + env + ").Login(" + env + ".Session); " + err + " != nil {\nreturn struct{}{}, " + err + "\n}\nreturn struct{}{}, nil\n"
		})
	case *ir.TestGet:
		name := "Test.get "
		if x.Open {
			name = "Test.open "
		}
		if x.Fetch {
			name = "Test.fetch "
		}
		return f.routeGet(name, x, t, s, fail, result)
	case *ir.TestSubmit:
		return f.callStmts("Test.submit "+x.Form.Name, true, t, s, func(env string, c *scope) string {
			var w strings.Builder
			fields := encodeName(x.Form.Module, x.Form.Name) + "(" + f.code(x.Record, c) + ")"
			f.submitRoute(&w, c, env, x.Form, x.Route, x.Params, fields, fail, result)
			return w.String()
		})
	case *ir.TestSubmitRaw:
		return f.callStmts("Test.submitRaw "+x.Form.Name, true, t, s, func(env string, c *scope) string {
			var w strings.Builder
			var names []string
			for _, fd := range x.Form.Record.Fields {
				names = append(names, strconv.Quote(fd.Name))
			}
			fs, err := f.temp(c, "_f"), f.temp(c, "_err")
			fmt.Fprintf(&w, "%s, %s := rtesting.RawFields(%s, []string{%s}, %s)\n%s", fs, err, strconv.Quote(x.Form.Name), strings.Join(names, ", "), f.code(x.Fields, c), fail(c, err))
			f.submitRoute(&w, c, env, x.Form, x.Route, x.Params, fs, fail, result)
			return w.String()
		})
	case *ir.TestClick:
		return f.callStmts("Test.click "+x.Route.Module+"."+x.Route.Name, true, t, s, func(env string, c *scope) string {
			var w strings.Builder
			r, label := f.routeTarget(&w, c, x.Route, x.Params)
			res, err := f.temp(c, "_r"), f.temp(c, "_err")
			fmt.Fprintf(&w, "%s, %s := rtesting.BrowserOf(%s).ClickRoute(%s, %s)\n%s", res, err, env, r, label, fail(c, err))
			w.WriteString(result(c, "Answer", res+".Answer", res))
			return w.String()
		})
	case *ir.TestRefresh:
		return f.callStmts("Test.refresh", true, t, s, func(env string, c *scope) string {
			res, err := f.temp(c, "_r"), f.temp(c, "_err")
			return res + ", " + err + " := rtesting.BrowserOf(" + env + ").Refresh()\n" + fail(c, err) + result(c, "Answer", res+".Answer", res)
		})
	}
	panic(fmt.Sprintf("gen: %T is not a browser step", x))
}

// routeGet is Test.get, open or fetch (name, "Test.get ") of a GET route: the URL the route
// value builds (rt.RouteURL), its first `requires` named in a redirect to the login page, and
// get's model the route's Model.
func (f *fn) routeGet(name string, x *ir.TestGet, t types.Type, s *scope, fail func(*scope, string) string, result func(*scope, string, string, string) string) string {
	r := x.Route
	return f.callStmts(name+r.Module+"."+r.Name, true, t, s, func(env string, c *scope) string {
		var w strings.Builder
		route, _ := f.routeTarget(&w, c, r, x.Params)
		u, b, res, err := f.temp(c, "_u"), f.temp(c, "_b"), f.temp(c, "_r"), f.temp(c, "_err")
		fmt.Fprintf(&w, "%s := rt.RouteURL(%s)\n%s := rtesting.BrowserOf(%s)\n", u, route, b, env)
		switch {
		case x.Fetch: // the record's Go type is rtesting.Fetched
			fmt.Fprintf(&w, "%s, %s := %s.Fetch(%s)\n%sreturn %s, nil\n", res, err, b, u, fail(c, err), res)
		case x.Open:
			fmt.Fprintf(&w, "%s, %s := %s.Open(%s)\n%s", res, err, b, u, fail(c, err))
			w.WriteString(result(c, "Answer", res+".Answer", res))
		default:
			requires := ""
			if len(r.Requires) > 0 {
				requires = r.Requires[0]
			}
			fmt.Fprintf(&w, "%s, %s := %s.GetPage(%s, %s)\n%s", res, err, b, u, strconv.Quote(requires), fail(c, err))
			m := f.temp(c, "_m")
			fmt.Fprintf(&w, "%s, %s := rtesting.ModelOf[%s](%s, %s)\n%s", m, err, GoType(r.Model), u, res, fail(c, err))
			w.WriteString(result(c, "Model", m, res))
		}
		return w.String()
	})
}

// submitRoute writes the request of Test.submit or submitRaw posting fields (rtesting.Fields)
// to the held page's form for write route r with its parameters params (without `form`; nil
// for a route without parameters), and the return of the step's result.
func (f *fn) submitRoute(w *strings.Builder, c *scope, env string, form *ir.Form, r *ir.Route, params ir.Expr, fields string,
	fail func(*scope, string) string, result func(*scope, string, string, string) string) {
	route, _ := f.routeTarget(w, c, r, params)
	res, err := f.temp(c, "_r"), f.temp(c, "_err")
	fmt.Fprintf(w, "%s, %s := rtesting.BrowserOf(%s).SubmitRoute(%s, %s, %s)\n%s", res, err, env, route, strconv.Quote(form.Name), fields, fail(c, err))
	w.WriteString(result(c, "Answer", res+".Answer", res))
}

// routeTarget writes the statement binding route r's parameters params (nil for a route
// without parameters) and returns the Go expressions of the route value (an rt.Route literal,
// routeRef) and of its label, the route as Alder writes it with its module (MsgName):
// `Todos.Toggle { id: "a" }`, `Todos.Export` for a route whose only parameter is its form or
// that has none.
func (f *fn) routeTarget(w *strings.Builder, c *scope, r *ir.Route, params ir.Expr) (string, string) {
	name := strconv.Quote(r.Module + "." + r.Name)
	own := false
	if r.Params != nil {
		for _, fd := range r.Params.Fields {
			own = own || fd.Name != "form"
		}
	}
	if params == nil || !own {
		return f.routeRef(&ir.RouteRef{Typed: ir.Typed{Ty: types.Route}, Route: r}, c), "rtesting.MsgName(" + name + ", nil)"
	}
	key, m := f.tempVar(c, "_m")
	fmt.Fprintf(w, "%s := %s\n", m, f.code(params, c))
	mv := &ir.Var{Typed: ir.Typed{Ty: params.Type()}, Kind: ir.VarLocal, Name: key}
	return f.routeRef(&ir.RouteRef{Typed: ir.Typed{Ty: types.Route}, Route: r, Params: mv}, c), "rtesting.MsgName(" + name + ", " + m + ")"
}

// sessionGo is the Go type of the `session` field of step type t's result record.
func (f *fn) sessionGo(t types.Type) string {
	if r, ok := effResult(t).(*types.Record); ok {
		for _, fd := range r.Fields {
			if fd.Name == "session" {
				return GoType(fd.Type)
			}
		}
	}
	panic("gen: a browser step's result has no session")
}

// selectorArg is, for each Test query, the index of its selector argument.
var selectorArg = map[string]int{"text": 0, "texts": 0, "has": 0, "count": 0, "attr": 1}

// querySel is the call of query callee with args when its selector argument is a text literal:
// the Sel form of its rtesting function, the selector a package variable parsed once
// (g.selector); ok is false otherwise.
func (f *fn) querySel(callee ir.Expr, args []ir.Expr, s *scope) (string, bool) {
	v, ok := callee.(*ir.Var)
	if !ok || v.Kind != ir.VarStd || v.Module != "Test" {
		return "", false
	}
	i, ok := selectorArg[v.Name]
	if !ok || len(args) != i+2 {
		return "", false
	}
	lit, ok := args[i].(*ir.Lit)
	if !ok || lit.Kind != ir.LitText {
		return "", false
	}
	f.g.testNode(callee)
	var as []string
	for j, a := range args {
		if j == i {
			as = append(as, f.g.selector(lit.Text))
		} else {
			as = append(as, f.code(a, s))
		}
	}
	return stdGoName(v.Module, v.Name) + "Sel(" + strings.Join(as, ", ") + ")", true
}

// selector is the package variable holding selector literal text, parsed once
// (rtesting.Sel), declared in app__alder.go: selector__1, selector__2, … in order of first use.
func (g *gen) selector(text string) string {
	if name, ok := g.selectors[text]; ok {
		return name
	}
	name := "selector__" + strconv.Itoa(len(g.selectorOrder)+1)
	g.selectors[text] = name
	g.selectorOrder = append(g.selectorOrder, text)
	return name
}

// selectorVars writes the selector literals' variables (selectors).
func (g *gen) selectorVars(b *strings.Builder) {
	if len(g.selectorOrder) == 0 {
		return
	}
	b.WriteString("// The selector literals of the tests' queries, parsed once (the checker has parsed them too).\nvar (\n")
	for _, text := range g.selectorOrder {
		fmt.Fprintf(b, "%s = rtesting.Sel(%s)\n", g.selectors[text], strconv.Quote(text))
	}
	b.WriteString(")\n\n")
}

// encoders writes, for a test build, encode__M__F for every form a write route posts: the
// form's record as the browser's inputs submit it, the inverse of decode__M__F (the
// rules at the top of routes.go): Text as given, Int and Float as digits, Bool as a checkbox
// ("True" when checked, absent when not), Date as YYYY-MM-DD, a union by its constructor's
// name, Maybe blank for Nothing (a cleared input; absent would give a field with a default its
// default). Its Absent are the fields whose value is what the decoder makes of the field not
// being sent — a Maybe without a default that is Nothing, a field (not a Bool) equal to its
// default — which the browser leaves out when the page's form has no input for them
// (rtesting.Fields.For); a Bool's False is not sent at all. Any other field the form has no
// input for fails the step: a user could not send it.
func (g *gen) encoders(b *strings.Builder, p *ir.Program) {
	seen := map[*ir.Form]bool{}
	var forms []*ir.Form
	for _, r := range p.Routes { // a write route's `form F`
		if r.Form != nil && !seen[r.Form] {
			seen[r.Form] = true
			forms = append(forms, r.Form)
		}
	}
	for _, fm := range forms {
		b.WriteString(comment(fmt.Sprintf("%s is form %s as the browser's inputs submit it, for Test.submit (test builds): the inverse of %s.", encodeName(fm.Module, fm.Name), fm.Name, decodeName(fm.Module, fm.Name))))
		r := "r"
		if len(fm.Record.Fields) == 0 {
			r = "_"
		}
		fmt.Fprintf(b, "func %s(%s %s) rtesting.Fields {\nv := url.Values{}\nvar absent []string\n", encodeName(fm.Module, fm.Name), r, GoName(fm.Module, fm.Name))
		for _, fd := range fm.Record.Fields {
			value, name := "r."+Field(fd.Name), strconv.Quote(fd.Name)
			b.WriteString(encodeField(fd.Type, value, name))
			if cond := g.absentWhen(fd, value); cond != "" {
				b.WriteString("if " + cond + " {\nabsent = append(absent, " + name + ")\n}\n")
			}
		}
		b.WriteString("return rtesting.Fields{Values: v, Absent: absent}\n}\n\n")
	}
}

// absentWhen is the Go condition under which form field fd, of value value, decodes to the same
// value when it is not sent: a Maybe without a default when Nothing, any other field but a Bool
// when equal to its default; "" when never (a required field, a Bool: False is never sent).
func (g *gen) absentWhen(fd ir.FieldT, value string) string {
	if fd.Default != nil {
		if maybeArg(fd.Type) == nil && scalarKind(fd.Type) == "bool" {
			return "" // a form's Bool is False when not sent, whatever its default
		}
		return "rt.Eq(" + value + ", " + (&fn{g: g}).code(fd.Default, newScope()) + ")"
	}
	if maybeArg(fd.Type) != nil {
		return "!" + value + ".Ok"
	}
	return ""
}

// encodeField is the statement setting v[name] (a Go string literal) to value (of type t) as an
// input submits it.
func encodeField(t types.Type, value, name string) string {
	set := func(text string) string { return "v.Set(" + name + ", " + text + ")\n" }
	if a := maybeArg(t); a != nil { // Nothing is a cleared input: blank (absent would be a default)
		return "if " + value + ".Ok {\n" + encodeField(a, value+".Val", name) + "} else {\n" + set(`""`) + "}\n"
	}
	switch scalarKind(t) {
	case "text":
		return set(value)
	case "int":
		return set("std.TextFromInt(" + value + ")")
	case "float":
		return set("rtesting.FormatFloat(" + value + ")")
	case "date":
		return set("std.DateToText(" + value + ")")
	case "bool":
		return "if " + value + " {\n" + set(`"True"`) + "}\n"
	case "enum":
		names, _ := enumLists(t.(*types.Union))
		return set(names + "[" + value + ".Tag]")
	}
	panic(fmt.Sprintf("gen: a form field of type %s cannot be encoded", types.String(t)))
}

// encodeName is the encoder of form F of module M in a test build: encode__M__F.
func encodeName(module, form string) string { return "encode__" + GoName(module, form) }
