// App-module rules (chapter 6): App's required declarations and their signatures, run once every
// module's bodies are checked; and the literal arguments of File's builders.
package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
	rtpkg "alder/rt"
)

// requireSig checks that value `name`'s declared signature unifies exactly with want, whose
// Alder text is sig. A mismatch carries the preferred fix "Change signature to `sig`", which
// replaces the declared signature (issue 45).
func (mc *modCtx) requireSig(name string, want types.Type, sig, msg string) {
	c := mc.c
	for _, d := range c.live(mc.m) {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok || vd.Name != name || vd.Route != "" || vd.Sig == nil {
			continue
		}
		got, ok := c.values[c.key(mc.m.Name, name)]
		if !ok {
			continue
		}
		if err := c.u.Unify(got, want); err != nil {
			d := c.errorf(diag.TypeMismatch, vd.Sig.GetSpan(), "%s", msg)
			d.Fix = diag.ReplaceSpan(vd.Sig.GetSpan(), sig)
			d.Fix.Title, d.Fix.Preferred = "Change signature to `"+sig+"`", true
		}
	}
}

func (mc *modCtx) checkApp() {
	c := mc.c
	has := map[string]*syntax.ValueDecl{}
	hasLogin, hasSession := false, false
	for _, d := range c.live(mc.m) {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if d.Route == "" {
				has[d.Name] = d
			}
		case *syntax.LoginDecl:
			hasLogin = true
			mc.checkLoginRoute(d)
		case *syntax.SessionDecl:
			hasSession = true
		}
	}
	html := &types.App{Con: "Html", Args: []types.Type{types.Route}} // `Html`, as typeOf reads it
	if has["error"] == nil {
		c.errorf(diag.MissingDecl, mc.m.Span, "`App` has no `error : Error -> Html`")
	} else {
		mc.requireSig("error", &types.Fun{Params: []types.Type{types.Error}, Result: html}, "Error -> Html", "`error` must have type `Error -> Html`")
	}
	if has["notFound"] == nil {
		c.errorf(diag.MissingDecl, mc.m.Span, "`App` has no `notFound : Html`")
	} else {
		mc.requireSig("notFound", html, "Html", "`notFound` must have type `Html`")
	}
	if has["maintenance"] != nil { // optional (M15): what a silo whose database cannot be served answers
		mc.requireSig("maintenance", html, "Html", "`maintenance` must have type `Html`")
	}
	anyRequires := ""
	for name, rs := range c.routes {
		for _, r := range rs {
			if anyRequires == "" && !c.prog.Failed[name] && len(r.Requires) > 0 {
				anyRequires = "a route"
			}
		}
	}
	if anyRequires != "" && !hasLogin {
		c.errorf(diag.MissingDecl, mc.m.Span, "%s uses `requires`, but `App` has no `login`", anyRequires)
	}
	if anyRequires != "" && !hasSession {
		c.errorf(diag.MissingDecl, mc.m.Span, "%s uses `requires`, but `App` has no `session`", anyRequires)
	}
	if c.usesSessionPut && !hasSession {
		c.errorf(diag.MissingDecl, mc.m.Span, "`Session.put` is used, but `App` has no `session`")
	}
}

// checkLoginRoute: `login = Module.Route` (#160) names a GET route that a visitor can open: no
// `requires` (a 303 to it would loop) and no parameter without a default (the runtime sends a
// visitor to its path as written).
func (mc *modCtx) checkLoginRoute(d *syntax.LoginDecl) {
	i := strings.LastIndexByte(d.Page, '.')
	if i < 0 {
		return
	}
	r := mc.c.route(d.Page[:i], d.Page[i+1:])
	switch {
	case r == nil: // names reported it
	case isWrite(r.Method):
		mc.c.errorf(diag.InvalidDecl, d.Span, "`login = %s`: `%s` is a %s route; a visitor is sent to log in with a GET", d.Page, d.Page, r.Method)
	case len(r.Requires) > 0:
		mc.c.errorf(diag.InvalidDecl, d.Span, "`login = %s`: `%s` has `requires %s`; a visitor sent to log in could not open it", d.Page, d.Page, strings.Join(r.Requires, ", "))
	case r.Params != nil:
		for _, f := range r.Params.Fields {
			if !f.HasDefault {
				mc.c.errorf(diag.InvalidDecl, d.Span, "`login = %s`: its parameter `%s` has no default; a visitor is sent to log in without one", d.Page, f.Name)
				return
			}
		}
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
				mc.c.errorf(diag.FileHeader, lit.Span, "`%s` cannot be set from a file route; the headers a file may set are `Content-Disposition`, `Cache-Control`, `ETag`, `Last-Modified`, `Retry-After`, `Content-Language` and `X-…` names", text.String())
			}
		}
	}
}
