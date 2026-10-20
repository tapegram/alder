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
// the step's result record ({ model | answer, session, doc }), S App's Session (struct{}
// without an App module):
//
//	TestLogin      "Test.login": BrowserOf(_e).Login(session), and the test's own session becomes
//	               session (_e.Session), so the scoped Db operations a test runs act for that user
//	TestGet        "Test.get M" / "Test.open M": _p := params; _r, _err := _b.GetPage(rt.PageURL(
//	               page(_p)), "<requires>") (Open for open); for get, the model the page's GET
//	               rendered (the route's rq.Model, through web.Options.Served):
//	               _m, _err := rtesting.ModelOf[M__Model](_u, _r);
//	               R{Model: _m, Session: rtesting.SessionOf[S](_r.Session), Doc: rtesting.DocOf(_r.Doc)}
//	TestGet        "Test.fetch M" (Fetch): _r, _err := _b.Fetch(rt.PageURL(page(_p))); _r is the
//	               result as is (rtesting.Fetched is the record's Go type)
//	TestSubmit     "Test.submit F": _b.SubmitForm("/_m/M/Msg", "F", encode__M__F(record)); when
//	               the test wrote the message with its parameters, _b.SubmitMsg(<its URL>, "F",
//	               <its label>, encode__M__F(record)) (matched as a click is)
//	TestSubmitRaw  "Test.submitRaw F": rtesting.RawFields("F", <F's fields>, fields), then
//	               SubmitForm or SubmitMsg
//	TestClick      "Test.click M.Msg": _b.ClickMsg(rt.MsgRef{…}.URL(nil), rtesting.MsgName("M.Msg",
//	               params)) — the message's URL with its own parameters only: the browser posts
//	               the URL the held page renders for it, which carries the page's parameters
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
		return "return " + r + "{" + first + ": " + value + ", Session: rtesting.SessionOf[" + f.sessionGo(t) + "](" + res + ".Session), Doc: rtesting.DocOf(" + res + ".Doc), Notices: " + res + ".Notices}, nil\n"
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
		return f.callStmts(name+x.Page.Module, true, t, s, func(env string, c *scope) string {
			var w strings.Builder
			pKey, p := f.tempVar(c, "_p")
			params := GoName(x.Page.Module, "Params") + "{}"
			if x.Params != nil {
				params = f.code(x.Params, c)
			}
			if len(x.Page.Params.Fields) > 0 { // the URL's parameters
				fmt.Fprintf(&w, "var %s %s = %s\n", p, GoName(x.Page.Module, "Params"), params)
			}
			pt := &types.Record{Module: x.Page.Module, Name: "Params"}
			for _, fd := range x.Page.Params.Fields {
				pt.Fields = append(pt.Fields, types.Field{Name: fd.Name, Type: fd.Type})
			}
			ref := &ir.PageRef{Typed: ir.Typed{Ty: types.Page}, Page: x.Page,
				Params: &ir.Var{Typed: ir.Typed{Ty: pt}, Kind: ir.VarLocal, Name: pKey}}
			if len(x.Page.Params.Fields) == 0 {
				ref.Params = nil
			}
			u, b, res, err := f.temp(c, "_u"), f.temp(c, "_b"), f.temp(c, "_r"), f.temp(c, "_err")
			fmt.Fprintf(&w, "%s := rt.PageURL(%s)\n%s := rtesting.BrowserOf(%s)\n", u, f.pageRef(ref, c), b, env)
			if x.Fetch { // the record's Go type is rtesting.Fetched
				fmt.Fprintf(&w, "%s, %s := %s.Fetch(%s)\n%sreturn %s, nil\n", res, err, b, u, fail(c, err), res)
				return w.String()
			}
			if x.Open {
				fmt.Fprintf(&w, "%s, %s := %s.Open(%s)\n%s", res, err, b, u, fail(c, err))
				w.WriteString(result(c, "Answer", res+".Answer", res))
				return w.String()
			}
			fmt.Fprintf(&w, "%s, %s := %s.GetPage(%s, %s)\n%s", res, err, b, u, strconv.Quote(firstRequired(x.Page)), fail(c, err))
			m := f.temp(c, "_m")
			fmt.Fprintf(&w, "%s, %s := rtesting.ModelOf[%s](%s, %s)\n%s", m, err, GoName(x.Page.Module, "Model"), u, res, fail(c, err))
			w.WriteString(result(c, "Model", m, res))
			return w.String()
		})
	case *ir.TestSubmit:
		return f.callStmts("Test.submit "+x.Form.Name, true, t, s, func(env string, c *scope) string {
			var w strings.Builder
			fields := encodeName(x.Form.Module, x.Form.Name) + "(" + f.code(x.Record, c) + ")"
			f.submit(&w, c, env, x.Form, x.Msg, x.Page, x.Params, fields, fail, result)
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
			f.submit(&w, c, env, x.Form, x.Msg, x.Page, x.Params, fs, fail, result)
			return w.String()
		})
	case *ir.TestClick:
		owner := stepOwner(x.Page)
		return f.callStmts("Test.click "+owner+"."+x.Msg.Name, true, t, s, func(env string, c *scope) string {
			var w strings.Builder
			u, label := f.msgTarget(&w, c, owner, x.Msg, x.Params)
			res, err := f.temp(c, "_r"), f.temp(c, "_err")
			fmt.Fprintf(&w, "%s, %s := rtesting.BrowserOf(%s).ClickMsg(%s, %s)\n%s", res, err, env, u, label, fail(c, err))
			w.WriteString(result(c, "Answer", res+".Answer", res))
			return w.String()
		})
	}
	panic(fmt.Sprintf("gen: %T is not a browser step", x))
}

// submit writes the request of Test.submit or submitRaw posting fields (Go url.Values): to the
// form of message msg the held page renders — matched by the message and its parameters when
// the test wrote them (params, SubmitMsg), else by the message alone (SubmitForm) — and the
// return of the step's result.
func (f *fn) submit(w *strings.Builder, c *scope, env string, form *ir.Form, msg *ir.Msg, page *ir.Page, params ir.Expr, fields string,
	fail func(*scope, string) string, result func(*scope, string, string, string) string) {
	owner := stepOwner(page)
	res, err := f.temp(c, "_r"), f.temp(c, "_err")
	if params == nil {
		fmt.Fprintf(w, "%s, %s := rtesting.BrowserOf(%s).SubmitForm(%s, %s, %s)\n%s", res, err, env, strconv.Quote(msgPath(owner, msg.Name)), strconv.Quote(form.Name), fields, fail(c, err))
	} else {
		u, label := f.msgTarget(w, c, owner, msg, params)
		fmt.Fprintf(w, "%s, %s := rtesting.BrowserOf(%s).SubmitMsg(%s, %s, %s, %s)\n%s", res, err, env, u, strconv.Quote(form.Name), label, fields, fail(c, err))
	}
	w.WriteString(result(c, "Answer", res+".Answer", res))
}

// msgTarget writes the statements binding message msg's parameters params (its payload record;
// nil for a message without parameters) and returns the Go expressions of its URL with its
// own parameters only (the browser posts the URL the held page renders for it, which carries
// the page's parameters too) and of its label, the message as Alder writes it (MsgName).
func (f *fn) msgTarget(w *strings.Builder, c *scope, owner string, msg *ir.Msg, params ir.Expr) (string, string) {
	name := strconv.Quote(owner + "." + msg.Name)
	if params == nil {
		return "rt.MsgURL(" + strconv.Quote(owner) + ", " + strconv.Quote(msg.Name) + ")", "rtesting.MsgName(" + name + ", nil)"
	}
	key, m := f.tempVar(c, "_m")
	fmt.Fprintf(w, "%s := %s\n", m, f.code(params, c))
	mv := &ir.Var{Typed: ir.Typed{Ty: params.Type()}, Kind: ir.VarLocal, Name: key}
	var items []kvItem
	var shown []string
	for _, fd := range msg.Params.Fields {
		if fd.Name != "form" {
			items = append(items, kvItem{name: fd.Name, t: fd.Type, v: &ir.Field{Typed: ir.Typed{Ty: fd.Type}, Expr: mv, Name: fd.Name}})
			shown = append(shown, Field(fd.Name)+": "+m+"."+Field(fd.Name))
		}
	}
	label := m
	if msg.Form != nil { // its payload's form is not the test's to show: the parameters alone
		label = "struct{ " + paramFields(msg) + " }{" + strings.Join(shown, ", ") + "}"
	}
	u := "rt.MsgRef{Module: " + strconv.Quote(owner) + ", Msg: " + strconv.Quote(msg.Name) + ", Params: " + f.kvList(items, c) + "}.URL(nil)"
	return u, "rtesting.MsgName(" + name + ", " + label + ")"
}

// paramFields are the Go fields of message msg's parameters, without its form.
func paramFields(msg *ir.Msg) string {
	var fs []string
	for _, fd := range msg.Params.Fields {
		if fd.Name != "form" {
			fs = append(fs, Field(fd.Name)+" "+GoType(fd.Type))
		}
	}
	return strings.Join(fs, "; ")
}

// stepOwner is the module of a step's message: its page's, or App.
func stepOwner(p *ir.Page) string {
	if p == nil {
		return "App"
	}
	return p.Module
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

// encoders writes, for a test build, encode__M__F for every form a message of a page or of App
// posts: the form's record as the browser's inputs submit it, the inverse of decode__M__F (the
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
	add := func(ms []*ir.Msg) {
		for _, m := range ms {
			if m.Form != nil && !seen[m.Form] {
				seen[m.Form] = true
				forms = append(forms, m.Form)
			}
		}
	}
	for _, pg := range p.Pages {
		add(pg.Msgs)
	}
	add(p.AppMsgs)
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
