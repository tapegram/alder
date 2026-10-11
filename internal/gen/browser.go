package gen

import (
	"fmt"
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
//	TestLogin      "Test.login": BrowserOf(_e).Login(session)
//	TestGet        "Test.get M" / "Test.open M": _p := params; _r, _err := _b.GetPage(rt.PageURL(
//	               page(_p)), "<requires>") (Open for open); for get, the page's load is run
//	               again for its typed model: _m, _, _stop := page__M(_b.LoadRequest(), _p, nil);
//	               R{Model: _m, Session: rtesting.SessionOf[S](_r.Session), Doc: rtesting.DocOf(_r.Doc)}
//	TestSubmit     "Test.submit F": _b.SubmitForm("/_m/M/Msg", "F", encode__M__F(record))
//	TestSubmitRaw  "Test.submitRaw F": rtesting.RawFields("F", <F's fields>, fields), then
//	               SubmitForm
//	TestClick      "Test.click M.Msg": _b.ClickMsg(rt.MsgRef{…}.URL(nil), rtesting.MsgName("M.Msg",
//	               params)) — the message's URL with its own parameters only: the browser posts
//	               the URL the held page renders for it, which carries the page's parameters
//
// A query (Test.text, texts, has, count, attr) whose selector is a literal calls the Sel form of
// its rtesting function with the selector parsed once, a package variable selector__N
// (app__alder.go); any other selector is parsed when the query runs.

// browserStep is step x (a Test* browser node of effect type x.Type()).
func (f *fn) browserStep(x ir.Expr, s *scope) string {
	t := x.Type()
	r := GoType(effResult(t))
	result := func(c *scope, first, value, res string) string {
		return "return " + r + "{" + first + ": " + value + ", Session: rtesting.SessionOf[" + f.sessionGo(t) + "](" + res + ".Session), Doc: rtesting.DocOf(" + res + ".Doc)}, nil\n"
	}
	fail := func(c *scope, err string) string {
		z := f.temp(c, "_z")
		return "if " + err + " != nil {\nvar " + z + " " + r + "\nreturn " + z + ", " + err + "\n}\n"
	}
	switch x := x.(type) {
	case *ir.TestLogin:
		return f.callStmts("Test.login", true, t, s, func(env string, c *scope) string {
			err := f.temp(c, "_err")
			return "if " + err + " := rtesting.BrowserOf(" + env + ").Login(" + f.code(x.Session, c) + "); " + err + " != nil {\nreturn struct{}{}, " + err + "\n}\nreturn struct{}{}, nil\n"
		})
	case *ir.TestGet:
		name := "Test.get "
		if x.Open {
			name = "Test.open "
		}
		return f.callStmts(name+x.Page.Module, true, t, s, func(env string, c *scope) string {
			var w strings.Builder
			pKey, p := f.tempVar(c, "_p")
			params := GoName(x.Page.Module, "Params") + "{}"
			if x.Params != nil {
				params = f.code(x.Params, c)
			}
			if len(x.Page.Params.Fields) > 0 || !x.Open { // the URL's parameters, and get's load's
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
			if x.Open {
				fmt.Fprintf(&w, "%s, %s := %s.Open(%s)\n%s", res, err, b, u, fail(c, err))
				w.WriteString(result(c, "Answer", res+".Answer", res))
				return w.String()
			}
			fmt.Fprintf(&w, "%s, %s := %s.GetPage(%s, %s)\n%s", res, err, b, u, strconv.Quote(x.Page.Requires), fail(c, err))
			rq, m, stop := f.temp(c, "_rq"), f.temp(c, "_m"), f.temp(c, "_stop")
			fmt.Fprintf(&w, "%s, %s := %s.LoadRequest()\n%s", rq, err, b, fail(c, err))
			fmt.Fprintf(&w, "%s, _, %s := %s(%s, %s, nil)\nif %s != nil {\nreturn %s{}, rtesting.LoadStopped(%s, %s.Status)\n}\n", m, stop, pageFunc(x.Page.Module), rq, p, stop, r, u, stop)
			w.WriteString(result(c, "Model", m, res))
			return w.String()
		})
	case *ir.TestSubmit:
		return f.callStmts("Test.submit "+x.Form.Name, true, t, s, func(env string, c *scope) string {
			res, err := f.temp(c, "_r"), f.temp(c, "_err")
			return res + ", " + err + " := rtesting.BrowserOf(" + env + ").SubmitForm(" + strconv.Quote(msgPath(stepOwner(x.Page), x.Msg.Name)) + ", " +
				strconv.Quote(x.Form.Name) + ", " + encodeName(x.Form.Module, x.Form.Name) + "(" + f.code(x.Record, c) + "))\n" +
				fail(c, err) + result(c, "Answer", res+".Answer", res)
		})
	case *ir.TestSubmitRaw:
		return f.callStmts("Test.submitRaw "+x.Form.Name, true, t, s, func(env string, c *scope) string {
			var names []string
			for _, fd := range x.Form.Record.Fields {
				names = append(names, strconv.Quote(fd.Name))
			}
			fs, res, err := f.temp(c, "_f"), f.temp(c, "_r"), f.temp(c, "_err")
			return fs + ", " + err + " := rtesting.RawFields(" + strconv.Quote(x.Form.Name) + ", []string{" + strings.Join(names, ", ") + "}, " + f.code(x.Fields, c) + ")\n" +
				fail(c, err) +
				res + ", " + err + " := rtesting.BrowserOf(" + env + ").SubmitForm(" + strconv.Quote(msgPath(stepOwner(x.Page), x.Msg.Name)) + ", " + strconv.Quote(x.Form.Name) + ", " + fs + ")\n" +
				fail(c, err) + result(c, "Answer", res+".Answer", res)
		})
	case *ir.TestClick:
		owner := stepOwner(x.Page)
		return f.callStmts("Test.click "+owner+"."+x.Msg.Name, true, t, s, func(env string, c *scope) string {
			var w strings.Builder
			ref := "rt.MsgRef{Module: " + strconv.Quote(owner) + ", Msg: " + strconv.Quote(x.Msg.Name) + "}"
			label := "nil"
			if x.Params != nil {
				key, m := f.tempVar(c, "_m")
				fmt.Fprintf(&w, "%s := %s\n", m, f.code(x.Params, c))
				mv := &ir.Var{Typed: ir.Typed{Ty: x.Params.Type()}, Kind: ir.VarLocal, Name: key}
				var items []kvItem
				for _, fd := range x.Msg.Params.Fields {
					if fd.Name != "form" {
						items = append(items, kvItem{name: fd.Name, t: fd.Type, v: &ir.Field{Typed: ir.Typed{Ty: fd.Type}, Expr: mv, Name: fd.Name}})
					}
				}
				ref = "rt.MsgRef{Module: " + strconv.Quote(owner) + ", Msg: " + strconv.Quote(x.Msg.Name) + ", Params: " + f.kvList(items, c) + "}"
				label = m
			}
			res, err := f.temp(c, "_r"), f.temp(c, "_err")
			fmt.Fprintf(&w, "%s, %s := rtesting.BrowserOf(%s).ClickMsg(%s.URL(nil), rtesting.MsgName(%s, %s))\n%s", res, err, env, ref, strconv.Quote(owner+"."+x.Msg.Name), label, fail(c, err))
			w.WriteString(result(c, "Answer", res+".Answer", res))
			return w.String()
		})
	}
	panic(fmt.Sprintf("gen: %T is not a browser step", x))
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
// name, Maybe absent for Nothing.
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
		fmt.Fprintf(b, "func %s(%s %s) url.Values {\nv := url.Values{}\n", encodeName(fm.Module, fm.Name), r, GoName(fm.Module, fm.Name))
		for _, fd := range fm.Record.Fields {
			b.WriteString(encodeField(fd.Type, "r."+Field(fd.Name), strconv.Quote(fd.Name)))
		}
		b.WriteString("return v\n}\n\n")
	}
}

// encodeField is the statement setting v[name] (a Go string literal) to value (of type t) as an
// input submits it.
func encodeField(t types.Type, value, name string) string {
	if a := maybeArg(t); a != nil {
		return "if " + value + ".Ok {\n" + encodeField(a, value+".Val", name) + "}\n"
	}
	set := func(text string) string { return "v.Set(" + name + ", " + text + ")\n" }
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
