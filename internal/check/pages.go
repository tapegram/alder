// Page-module and App-module rules (chapter 6): required signatures and App declarations.
package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
	rtpkg "alder/rt"
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
		hookSig("load"), hookMessage("load"))
	viewMsg := hookMessage("view") + ", or `Model -> Session -> File` for a file page"
	if mc.page.File {
		mc.requireSig("view", &types.Fun{Params: []types.Type{mc.modelType(), mc.session}, Result: types.File}, "Model -> Session -> File", viewMsg)
		mc.checkFilePage()
		return
	}
	mc.requireSig("view", &types.Fun{Params: []types.Type{mc.modelType(), mc.session}, Result: html(mc.page.Msg)}, hookSig("view"), viewMsg)
	mc.requireSig("update", &types.Fun{Params: []types.Type{mc.page.Msg, mc.modelType(), mc.session}, Result: &types.App{Con: "Task", Args: []types.Type{c.navType()}}},
		hookSig("update"), hookMessage("update"))
}

// checkFilePage reports E0034 for each `msg`, `form` and `update` of a file page: its view
// returns File, so there is no document to post from (chapter 6, "File pages").
func (mc *modCtx) checkFilePage() {
	for _, d := range mc.c.live(mc.m) {
		var kind string
		switch d := d.(type) {
		case *syntax.MsgDecl:
			kind = "message"
		case *syntax.FormDecl:
			kind = "form"
		case *syntax.ValueDecl:
			if d.Name != "update" {
				continue
			}
			kind = "update"
		default:
			continue
		}
		mc.c.errorf(diag.FilePageMsg, d.GetSpan(), "a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the %s to an HTML page", kind)
	}
}

// checkFileArg checks the literal first argument of File.withStatus (200..599) and
// File.withHeader (an allowed name). The first argument is args[0] in both the direct and the
// piped form; a non-literal is checked when the page answers.
func (mc *modCtx) checkFileArg(name string, args []syntax.Expr) {
	if len(args) == 0 {
		return
	}
	switch name {
	case "withStatus":
		if lit, ok := args[0].(*syntax.IntLit); ok && (lit.Value < 200 || lit.Value > 599) {
			mc.c.errorf(diag.TypeMismatch, lit.Span, "`File.withStatus` takes a status from 200 to 599, got %d", lit.Value)
		}
	case "withHeader":
		rec, ok := args[0].(*syntax.RecordLit)
		if !ok {
			return
		}
		for _, f := range rec.Fields {
			if f.Name != "name" {
				continue
			}
			lit, ok := f.Value.(*syntax.StrLit)
			if !ok {
				return
			}
			var text strings.Builder
			for _, p := range lit.Parts {
				if p.Expr != nil {
					return
				}
				text.WriteString(p.Text)
			}
			if _, ok := rtpkg.FileHeaderAllowed(text.String()); !ok {
				mc.c.errorf(diag.FileHeader, lit.Span, "`%s` cannot be set from a page; the headers a file may set are `Content-Disposition`, `Cache-Control`, `ETag`, `Last-Modified`, `Retry-After`, `Content-Language` and `X-…` names", text.String())
			}
		}
	}
}

func (mc *modCtx) modelType() types.Type {
	if t, ok := mc.c.types[mc.c.key(mc.m.Name, "Model")]; ok {
		return t
	}
	return types.Err // a page without `Model` is reported as E0010
}

func (c *Checker) navType() types.Type {
	if t, ok := c.types["Prelude.Nav"]; ok {
		return t
	}
	return c.u.Fresh()
}

// requireSig checks that value `name`'s declared signature unifies exactly with want, whose
// Alder text is sig. A mismatch carries the preferred fix "Change signature to `sig`", which
// replaces the declared signature (issue 45).
func (mc *modCtx) requireSig(name string, want types.Type, sig, msg string) {
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
			d := c.errorf(diag.TypeMismatch, vd.Sig.GetSpan(), "%s", m)
			d.Fix = diag.ReplaceSpan(vd.Sig.GetSpan(), sig)
			d.Fix.Title, d.Fix.Preferred = "Change signature to `"+sig+"`", true
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
		mc.requireSig("error", &types.Fun{Params: []types.Type{types.Error}, Result: html(types.Never)}, "Error -> Html Never", "`error` must have type `Error -> Html Never`")
	}
	if has["notFound"] == nil {
		c.errorf(diag.MissingDecl, mc.m.Span, "`App` has no `notFound : Html Never`")
	} else {
		mc.requireSig("notFound", html(types.Never), "Html Never", "`notFound` must have type `Html Never`")
	}
	if has["maintenance"] != nil { // optional (M15): what a silo whose database cannot be served answers
		mc.requireSig("maintenance", html(types.Never), "Html Never", "`maintenance` must have type `Html Never`")
	}
	if len(c.appMsg.Constructors()) > 0 {
		if has["update"] == nil {
			c.errorf(diag.MissingDecl, mc.m.Span, "`App` declares messages but has no `update`")
		}
	}
	if has["update"] != nil {
		mc.requireSig("update", &types.Fun{Params: []types.Type{c.appMsg, c.session}, Result: &types.App{Con: "Task", Args: []types.Type{c.navType()}}}, "Msg -> Session -> Task Nav", "`update` must have type `Msg -> Session -> Task Nav`")
	}
	anyRequires := false
	for name, p := range c.pages {
		if c.prog.Failed[name] {
			continue
		}
		if len(p.Requires) > 0 {
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
