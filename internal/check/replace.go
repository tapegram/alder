package check

import (
	"alder/internal/diag"
	"alder/internal/syntax"
)

// The compile-time side of `Response.replace`, `Response.remove` and the deprecated
// `Response.render` (#309).

// responseCall checks the Response builders #309 adds or deprecates, applied to args with the
// piped value (pipe, nil when none) last: E0051 for the roots of a `Response.replace` list
// literal, E0045 for a `Response.remove` written after a whole-page answer, W0016 for a
// `Response.render`. renderWarned records the `render` references warned here, so the reference
// itself (inferVar) does not warn again without the route's name.
func (mc *modCtx) responseCall(v *syntax.Var, args []syntax.Expr, pipe *pipeArg) {
	if v.Ref.Kind != syntax.RefStd || v.Ref.Module != "Response" {
		return
	}
	all := args
	if pipe != nil {
		all = append(append([]syntax.Expr(nil), args...), pipe.expr)
	}
	switch v.Ref.Name {
	case "replace":
		if len(all) == 1 {
			mc.replaceRoots(all[0])
		}
	case "remove":
		if len(all) == 2 {
			mc.removePlace(v, all[1])
		}
	case "render":
		if len(all) == 1 {
			if ct, r := mc.routeTarget(all[0]); r != nil {
				name := written(ct.Module, ct.Name)
				mc.c.bag.Warn(diag.RenderDeprecated, v.Span, "`Response.render %s` is going away: load what `%s.view` shows and answer `Response.replace [ %s.view … ]`, a handler sends the views it built", name, name, name)
				if mc.c.renderWarned == nil {
					mc.c.renderWarned = map[*syntax.Var]bool{}
				}
				mc.c.renderWarned[v] = true
			}
		}
	}
}

// renderRef is W0016 at a `Response.render` reference responseCall did not see applied to a
// route (passed as a function, or applied to a variable).
func (mc *modCtx) renderRef(v *syntax.Var) {
	if v.Ref.Kind != syntax.RefStd || v.Ref.Module != "Response" || v.Ref.Name != "render" || mc.c.renderWarned[v] {
		return
	}
	mc.c.bag.Warn(diag.RenderDeprecated, v.Span, "`Response.render` is going away: load what the route's view shows and answer `Response.replace [ … ]`, a handler sends the views it built")
}

// removePlace is E0045 for `Response.remove` on a response written in place as a whole-page
// answer (through other modifiers): there is no element left to remove. One reached through a
// variable is left to the run time.
func (mc *modCtx) removePlace(v *syntax.Var, resp syntax.Expr) {
	why := map[string]string{
		"refresh":  "a refresh sends the page as it now is, which already lacks the element",
		"redirect": "a redirect loads another page, which has no element to remove",
		"invalid":  "an invalid answer shows the form again, with the page as it is",
	}
	if base := baseAnswer(resp); why[base] != "" {
		mc.c.errorf(diag.RouteTarget, v.Span, "`Response.remove` after `Response.%s`: %s", base, why[base])
	}
}

// baseAnswer is the name of the Response builder e is written as, looking through the
// modifiers (`toast`, `success`, `warning`, `error`, `remove`), applied or piped; "" when e is
// not one written in place.
func baseAnswer(e syntax.Expr) string {
	modifier := func(fn syntax.Expr) bool {
		v, ok := fn.(*syntax.Var)
		if !ok || v.Ref.Kind != syntax.RefStd || v.Ref.Module != "Response" {
			return false
		}
		switch v.Ref.Name {
		case "toast", "success", "warning", "error", "remove":
			return true
		}
		return false
	}
	switch x := e.(type) {
	case *syntax.Var:
		if x.Ref.Kind == syntax.RefStd && x.Ref.Module == "Response" {
			return x.Ref.Name
		}
	case *syntax.App:
		if modifier(x.Fn) && len(x.Args) == 2 {
			return baseAnswer(x.Args[1])
		}
		if v, ok := x.Fn.(*syntax.Var); ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Response" && !modifier(x.Fn) {
			return v.Ref.Name
		}
	case *syntax.BinOp:
		if app, ok := x.R.(*syntax.App); x.Op == "|>" && ok && modifier(app.Fn) && len(app.Args) == 1 {
			return baseAnswer(x.L)
		}
	}
	return ""
}

// replaceRoot is one root of a replace's list the checker can see: the element, and the
// top-level value whose body it ends (as written, "" for an element written in place).
type replaceRoot struct {
	el  *syntax.Element
	via string
}

// replaceRoots is E0051 for the roots of a `Response.replace` list literal the checker can see
// (an element in place, a call of a top-level function whose body ends in an element through
// `if`, `case` and a block's last expression, an `Html.fragment` of those): one without an `id`,
// and two with the same literal `id`. A root it cannot see (a variable, a `for`, a list built
// elsewhere) is left to the run time; a computed `id` counts as present.
func (mc *modCtx) replaceRoots(arg syntax.Expr) {
	lit, ok := arg.(*syntax.ListLit)
	if !ok {
		return
	}
	seen := map[string]bool{}
	for _, item := range lit.Elems {
		mc.replaceItem(item, seen)
	}
}

// replaceItem checks one item of the list (or of a fragment in it), seen holding the literal
// ids of the roots before it.
func (mc *modCtx) replaceItem(item syntax.Expr, seen map[string]bool) {
	if app, ok := item.(*syntax.App); ok && isStd(app.Fn, "Html", "fragment") && len(app.Args) == 1 {
		if lit, ok := app.Args[0].(*syntax.ListLit); ok {
			for _, e := range lit.Elems {
				mc.replaceItem(e, seen)
			}
		}
		return
	}
	var rs rootSet
	mc.visibleRoots(item, "", map[string]bool{}, &rs)
	for _, r := range rs.roots {
		if _, has := rootID(r.el); !has {
			subject := "the root"
			if r.via != "" {
				subject = "the root of `" + r.via + "`"
			}
			mc.c.errorf(diag.ReplaceRoot, item.GetSpan(), "`Response.replace`: %s is a `<%s>` without an `id`. A replace swaps the element with the same `id`: give the root one.", subject, r.el.Tag)
			return
		}
	}
	// the item's id, when every root it may have is visible and carries the same literal one
	if rs.hidden || len(rs.roots) == 0 {
		return
	}
	id, _ := rootID(rs.roots[0].el)
	for _, r := range rs.roots[1:] {
		if other, _ := rootID(r.el); other != id {
			return
		}
	}
	if id == "" {
		return
	}
	if seen[id] {
		mc.c.errorf(diag.ReplaceRoot, item.GetSpan(), "`Response.replace` has two roots with the id `%s`", id)
		return
	}
	seen[id] = true
}

// rootSet is the elements an item of a replace's list may end in: roots the checker can see,
// and hidden when some branch ends in something it cannot (a variable, a `for`, a parameter).
type rootSet struct {
	roots  []replaceRoot
	hidden bool
}

// visibleRoots adds to rs the elements e may end in: e itself, the branches of an `if` or
// `case`, a block's last expression, the body of the top-level value e calls (via names it, as
// written at the outermost call). visiting guards recursion.
func (mc *modCtx) visibleRoots(e syntax.Expr, via string, visiting map[string]bool, rs *rootSet) {
	switch x := e.(type) {
	case *syntax.Element:
		if x.Ref.Kind == syntax.RefComponent || isUpperName(x.Tag) || x.Tag == "html" || x.Tag == "head" || x.Tag == "body" { // a component, or a page's frame, is the run time's to judge
			rs.hidden = true
			return
		}
		rs.roots = append(rs.roots, replaceRoot{el: x, via: via})
		return
	case *syntax.If:
		mc.visibleRoots(x.Then, via, visiting, rs)
		mc.visibleRoots(x.Else, via, visiting, rs)
		return
	case *syntax.Match:
		for _, arm := range x.Arms {
			mc.visibleRoots(arm.Body, via, visiting, rs)
		}
		return
	case *syntax.Block:
		mc.visibleRoots(x.Result, via, visiting, rs)
		return
	case *syntax.App:
		if v, ok := x.Fn.(*syntax.Var); ok && mc.calledRoots(v, via, visiting, rs) {
			return
		}
	case *syntax.Var:
		if mc.calledRoots(x, via, visiting, rs) {
			return
		}
	}
	rs.hidden = true
}

// calledRoots adds the roots of the body of the top-level value v names, reporting whether v
// names one with a body to look into.
func (mc *modCtx) calledRoots(v *syntax.Var, via string, visiting map[string]bool, rs *rootSet) bool {
	if v.Ref.Kind != syntax.RefValue {
		return false
	}
	key := mc.c.key(v.Ref.Module, v.Ref.Name)
	vd, ok := mc.c.valueDecls[key]
	if !ok || vd.d.Body == nil || visiting[key] {
		return false
	}
	visiting[key] = true
	defer delete(visiting, key)
	if via == "" {
		via = written(v.Module, v.Name)
	}
	mc.visibleRoots(vd.d.Body, via, visiting, rs)
	return true
}

// rootID is el's `id`: its text when written as a literal, and whether it has one at all (a
// computed value counts; an empty literal does not).
func rootID(el *syntax.Element) (string, bool) {
	for _, a := range el.Attrs {
		if a.Name != "id" || a.Value == nil {
			continue
		}
		s, ok := a.Value.(*syntax.StrLit)
		if !ok {
			return "", true
		}
		text := ""
		for _, p := range s.Parts {
			if p.Expr != nil {
				return "", true
			}
			text += p.Text
		}
		return text, text != ""
	}
	return "", false
}

// isStd reports whether e is the stdlib value module.name.
func isStd(e syntax.Expr, module, name string) bool {
	v, ok := e.(*syntax.Var)
	return ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == module && v.Ref.Name == name
}
