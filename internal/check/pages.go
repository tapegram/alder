// Page-module and App-module rules (chapter 6): required signatures and App declarations.
package check

import (
	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// checkPage checks the required signatures of a page module. App rules run from Program once
// every module's bodies are checked (see checkApp).
func (mc *modCtx) checkPage() {
	c := mc.c
	if mc.isApp || mc.page == nil {
		return
	}
	html := func(arg types.Type) types.Type { return &types.App{Con: "Html", Args: []types.Type{arg}} }
	mc.requireSig("load", &types.Fun{Params: []types.Type{mc.page.Params, mc.session}, Result: &types.App{Con: "Query", Args: []types.Type{mc.modelType()}}},
		"`load` must have type `Params -> Session -> Query Model`")
	mc.requireSig("view", &types.Fun{Params: []types.Type{mc.modelType(), mc.session}, Result: html(mc.page.Msg)},
		"`view` must have type `Model -> Session -> Html Msg`")
	mc.requireSig("update", &types.Fun{Params: []types.Type{mc.page.Msg, mc.modelType(), mc.session}, Result: &types.App{Con: "Task", Args: []types.Type{c.navType()}}},
		"`update` must have type `Msg -> Model -> Session -> Task Nav`")
}

func (mc *modCtx) modelType() types.Type {
	if t, ok := mc.c.types[mc.c.key(mc.m.Name, "Model")]; ok {
		return t
	}
	return mc.c.u.Fresh()
}

func (c *Checker) navType() types.Type {
	if t, ok := c.types["Prelude.Nav"]; ok {
		return t
	}
	return c.u.Fresh()
}

// requireSig checks that value `name`'s declared signature unifies exactly with want.
func (mc *modCtx) requireSig(name string, want types.Type, msg string) {
	c := mc.c
	for _, d := range c.live(mc.m) {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok || vd.Name != name || vd.Sig == nil {
			continue
		}
		got, ok := c.values[c.key(mc.m.Name, name)]
		if !ok {
			continue
		}
		if err := c.u.Unify(got, want); err != nil {
			m := msg
			if name == "view" && mc.page != nil && viewIsNever(c.u.Apply(got)) {
				m += "; it is `Html Msg` even when the page has no messages"
			}
			c.errorf(diag.TypeMismatch, vd.Sig.GetSpan(), "%s", m)
		}
	}
}

func viewIsNever(t types.Type) bool {
	fn, ok := t.(*types.Fun)
	if !ok {
		return false
	}
	h, ok := fn.Result.(*types.App)
	return ok && h.Con == "Html" && len(h.Args) == 1 && isCon(h.Args[0], "Never")
}

func (mc *modCtx) checkApp() {
	c := mc.c
	has := map[string]*syntax.ValueDecl{}
	hasLogin, hasSession := false, false
	for _, d := range c.live(mc.m) {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			has[d.Name] = d
		case *syntax.LoginDecl:
			hasLogin = true
		case *syntax.SessionDecl:
			hasSession = true
		}
	}
	html := func(arg types.Type) types.Type { return &types.App{Con: "Html", Args: []types.Type{arg}} }
	if has["error"] == nil {
		c.errorf(diag.MissingDecl, mc.m.Span, "`App` has no `error : Error -> Html Never`")
	} else {
		mc.requireSig("error", &types.Fun{Params: []types.Type{types.Error}, Result: html(types.Never)}, "`error` must have type `Error -> Html Never`")
	}
	if has["notFound"] == nil {
		c.errorf(diag.MissingDecl, mc.m.Span, "`App` has no `notFound : Html Never`")
	} else {
		mc.requireSig("notFound", html(types.Never), "`notFound` must have type `Html Never`")
	}
	if len(c.appMsg.Constructors()) > 0 {
		if has["update"] == nil {
			c.errorf(diag.MissingDecl, mc.m.Span, "`App` declares messages but has no `update`")
		}
	}
	if has["update"] != nil {
		mc.requireSig("update", &types.Fun{Params: []types.Type{c.appMsg, c.session}, Result: &types.App{Con: "Task", Args: []types.Type{c.navType()}}}, "`update` must have type `Msg -> Session -> Task Nav`")
	}
	anyRequires := false
	for name, p := range c.pages {
		if c.prog.Failed[name] {
			continue
		}
		if p.Requires != "" {
			anyRequires = true
		}
	}
	if anyRequires && !hasLogin {
		c.errorf(diag.MissingDecl, mc.m.Span, "a page uses `requires`, but `App` has no `login`")
	}
	if anyRequires && !hasSession {
		c.errorf(diag.MissingDecl, mc.m.Span, "a page uses `requires`, but `App` has no `session`")
	}
	if c.usesSessionPut && !hasSession {
		c.errorf(diag.MissingDecl, mc.m.Span, "`Session.put` is used, but `App` has no `session`")
	}
}
