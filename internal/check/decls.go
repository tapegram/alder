// Declaration pass: types, tables, forms, pages, msgs, capabilities, value signatures.
package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// declareTypeNames creates placeholders so types can refer to each other in any order.
func (c *Checker) declareTypeNames(m *syntax.Module) {
	seen := map[string]diag.Span{}
	var cur syntax.Decl
	dup := func(name string, sp diag.Span) bool {
		if prev, ok := seen[name]; ok {
			c.errorf(diag.Duplicate, sp, "`%s` is already declared at line %d", name, prev.Start.Line)
			c.losers[cur] = true // every later pass skips it (see live)
			return true
		}
		seen[name] = sp
		return false
	}
	for _, d := range m.Decls {
		cur = d
		switch d := d.(type) {
		case *syntax.TypeDecl:
			if dup(d.Name, d.Span) {
				continue
			}
			k := c.key(m.Name, d.Name)
			if _, isStdCon := c.types[k]; isStdCon && m.Name == "Prelude" {
				continue // Int, Text… already canonical
			}
			switch {
			case d.Brand != nil:
				c.types[k] = &types.Brand{Module: m.Name, Name: d.Name, Base: baseType(d.Brand.Base), HasCheck: d.Brand.Check != nil}
			case d.Record != nil:
				c.types[k] = &types.Record{Module: m.Name, Name: d.Name}
			case d.Union != nil:
				c.types[k] = &types.Union{Module: m.Name, Name: d.Name, Params: d.Params}
			default: // opaque (stdlib): represented as App with Param args, or Con when no params
				if len(d.Params) == 0 {
					c.types[k] = &types.Con{Name: d.Name}
				} else {
					var args []types.Type
					for _, p := range d.Params {
						args = append(args, &types.Param{Name: p})
					}
					c.types[k] = &types.App{Con: d.Name, Args: args}
				}
			}
		case *syntax.TableDecl:
			if dup(d.Name, d.Span) {
				continue
			}
			c.types[c.key(m.Name, d.Name)] = &types.Record{Module: m.Name, Name: d.Name}
		case *syntax.FormDecl:
			if dup(d.Name, d.Span) {
				continue
			}
			c.types[c.key(m.Name, d.Name)] = &types.Record{Module: m.Name, Name: d.Name}
		case *syntax.ValueDecl:
			dup(d.Name, d.Span)
		case *syntax.MsgDecl:
			dup(d.Name, d.Span)
		}
	}
}

// live returns m's declarations without the duplicates declareTypeNames reported:
// the first declaration of a name wins and the others are never defined or checked.
func (c *Checker) live(m *syntax.Module) []syntax.Decl {
	out := make([]syntax.Decl, 0, len(m.Decls))
	for _, d := range m.Decls {
		if !c.losers[d] {
			out = append(out, d)
		}
	}
	return out
}

func baseType(name string) types.Type {
	switch name {
	case "Int":
		return types.Int
	case "Float":
		return types.Float
	case "Bool":
		return types.Bool
	default:
		return types.Text
	}
}

// typeOf converts an AST type. params are the type variables in scope (signature or type decl).
func (mc *modCtx) typeOf(t syntax.Type, params map[string]bool) types.Type {
	c := mc.c
	switch t := t.(type) {
	case *syntax.TypeName:
		var args []types.Type
		for _, a := range t.Args {
			args = append(args, mc.typeOf(a, params))
		}
		switch t.Ref.Kind {
		case syntax.RefPage:
			return types.Page
		case syntax.RefType:
			if t.Ref.Module == mc.m.Name || t.Ref.Module == "App" || c.prog.IsPageModule(t.Ref.Module) {
				switch t.Name {
				case "Params":
					if mc.page != nil && t.Ref.Module == mc.m.Name {
						return mc.page.Params
					}
				case "Msg":
					if t.Ref.Module == "App" {
						return c.appMsg
					}
					if mc.page != nil {
						return mc.page.Msg
					}
					if mc.isApp {
						return c.appMsg
					}
					c.errorf("E0601", t.Span, "`Msg` is only defined in page modules; a shared component is typed `Html a`").Fix = &diag.Fix{Line: t.Span.Start.Line, Col: t.Span.Start.Col, Replace: "a"}
					return c.u.Fresh()
				case "Session":
					return mc.session
				}
			}
			decl, ok := c.types[c.key(t.Ref.Module, t.Name)]
			if !ok {
				return c.u.Fresh()
			}
			return applyArgs(decl, args)
		}
		return c.u.Fresh()
	case *syntax.TypeVar:
		if params != nil && params[t.Name] {
			return &types.Param{Name: t.Name}
		}
		// an unknown lowercase type is a type variable in a signature (implicitly quantified)
		if params != nil {
			params[t.Name] = true
			return &types.Param{Name: t.Name}
		}
		return c.u.Fresh()
	case *syntax.RecordType:
		r := &types.Record{Anonymous: true}
		for _, f := range t.Fields {
			r.Fields = append(r.Fields, types.Field{Name: f.Name, Type: mc.typeOf(f.Type, params), HasDefault: f.Default != nil})
		}
		return r
	case *syntax.FunType:
		fn := &types.Fun{}
		for _, p := range t.Params {
			fn.Params = append(fn.Params, mc.typeOf(p, params))
		}
		fn.Result = mc.typeOf(t.Result, params)
		return fn
	}
	return c.u.Fresh()
}

// applyArgs instantiates a declared type with arguments: unions get Args; opaque Apps substitute their Params.
func applyArgs(decl types.Type, args []types.Type) types.Type {
	switch d := decl.(type) {
	case *types.Union:
		if len(args) == 0 {
			return d
		}
		return d.WithArgs(args)
	case *types.App:
		if len(args) == 0 {
			return d
		}
		return &types.App{Con: d.Con, Args: args}
	}
	return decl
}

// defineTypes fills in placeholder bodies for this module's unions, records, tables, forms.
func (mc *modCtx) defineTypes() {
	c := mc.c
	for _, d := range c.live(mc.m) {
		switch d := d.(type) {
		case *syntax.TypeDecl:
			params := map[string]bool{}
			for _, p := range d.Params {
				params[p] = true
			}
			k := c.key(mc.m.Name, d.Name)
			if aliased := mc.checkAlias(d, params); aliased != nil {
				c.types[k] = aliased // recovery: read the alias as its target so uses do not cascade
				continue
			}
			switch tt := c.types[k].(type) {
			case *types.Union:
				for _, ct := range d.Union {
					var arg types.Type
					if ct.Arg != nil {
						arg = mc.typeOf(ct.Arg, params)
					}
					tt.Ctors = append(tt.Ctors, types.Ctor{Name: ct.Name, Arg: arg})
					c.ctors[c.key(mc.m.Name, ct.Name)] = &ctorInfo{Union: tt, Arg: arg}
				}
			case *types.Record:
				for _, f := range d.Record.Fields {
					ft := mc.typeOf(f.Type, params)
					mc.deferDefault(f, ft)
					tt.Fields = append(tt.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
				}
			}
		case *syntax.TableDecl:
			mc.defineTable(d)
		case *syntax.FormDecl:
			rec := c.types[c.key(mc.m.Name, d.Name)].(*types.Record)
			for _, f := range d.Fields {
				ft := mc.typeOf(f.Type, nil)
				mc.deferDefault(f, ft)
				rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
			}
			c.forms[c.key(mc.m.Name, d.Name)] = rec
		}
	}
	if mc.m.Name == "Prelude" {
		// True/False are literals; Bool stays a Con. Register ctors Just/Nothing/Ok/Err/Rerender/Redirect/Reject only.
		delete(c.ctors, "Prelude.True")
		delete(c.ctors, "Prelude.False")
	}
}

// checkAlias reports E0602 for `type Model = Unit`: a one-constructor union whose constructor
// is named after a Prelude type is an alias in disguise, which Alder does not have. It returns
// the aliased type (for error recovery), or nil when d is not an alias.
func (mc *modCtx) checkAlias(d *syntax.TypeDecl, params map[string]bool) types.Type {
	c := mc.c
	if len(d.Union) != 1 || c.prog.Std[mc.m.Name] != nil || d.Union[0].Name == d.Name {
		return nil
	}
	target, isType := c.types[c.key("Prelude", d.Union[0].Name)]
	if !isType {
		return nil
	}
	rhs := mc.source(d.Union[0].Span)
	if rhs == "" {
		rhs = d.Union[0].Name
	}
	c.errorf("E0602", d.Span, "`type %s = %s` is an alias; a new name is always a new type; write `type %s = { }` or a union with its own constructors", d.Name, rhs, d.Name)
	if arg := d.Union[0].Arg; arg != nil {
		return applyArgs(target, []types.Type{mc.typeOf(arg, params)})
	}
	return target
}

func (mc *modCtx) defineTable(d *syntax.TableDecl) {
	c := mc.c
	rec := c.types[c.key(mc.m.Name, d.Name)].(*types.Record)
	info := &types.TableInfo{Module: mc.m.Name, Name: d.Name, Record: rec, Columns: map[string]types.Type{}, Insert: &types.Record{Module: mc.m.Name, Name: "Insert " + d.Name}}
	idCount := 0
	for _, f := range d.Fields {
		ft := mc.typeOf(f.Type, nil)
		mc.deferDefault(f, ft)
		rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
		info.Columns[f.Name] = ft
		generated := false
		for _, a := range f.Attrs {
			switch a.Name {
			case "id":
				idCount++
				generated = true
				info.IDField, info.IDType = f.Name, ft
				if b, ok := ft.(*types.Brand); !ok || !isCon(b.Base, "Text") {
					c.errorf(diag.InvalidDecl, f.Span, "`@id` field must be a `brand Text`, got `%s`", types.String(ft))
				}
			case "created", "updated":
				generated = true
				if !isCon(ft, "Time") {
					c.errorf(diag.InvalidDecl, f.Span, "`@%s` field must have type `Time`", a.Name)
				}
			case "ref":
				mc.checkRef(f, a, ft)
			case "unique":
			default:
				c.errorf(diag.InvalidDecl, a.Span, "unknown attribute `@%s`", a.Name)
			}
		}
		if !generated {
			info.Insert.Fields = append(info.Insert.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
		}
	}
	if idCount != 1 {
		c.errorf(diag.InvalidDecl, d.Span, "table `%s` must have exactly one `@id` field", d.Name)
	}
	c.tables[c.key(mc.m.Name, d.Name)] = info
	c.u.RegisterTable(info)
}

// checkRef validates `@ref T` after all tables have placeholders; the referenced table's id type is read from its decl.
func (mc *modCtx) checkRef(f *syntax.Field, a *syntax.Attr, ft types.Type) {
	c := mc.c
	// find the table decl named a.Arg in this module or an import
	var idType types.Type
	for _, m := range c.allModules() {
		if m.Name != mc.m.Name && !imports(mc.m, m.Name) {
			continue
		}
		for _, d := range c.live(m) {
			if td, ok := d.(*syntax.TableDecl); ok && td.Name == a.Arg {
				for _, tf := range td.Fields {
					for _, ta := range tf.Attrs {
						if ta.Name == "id" {
							idType = c.ctx(m).typeOf(tf.Type, nil)
						}
					}
				}
			}
		}
	}
	if idType == nil {
		c.errorf(diag.UnknownName, a.Span, "`@ref %s`: no table named `%s` is in scope", a.Arg, a.Arg)
		return
	}
	ok := c.u.Unify(ft, idType) == nil
	if !ok {
		if un, isU := ft.(*types.Union); isU && un.Name == "Maybe" && len(un.Args) == 1 {
			ok = c.u.Unify(un.Args[0], idType) == nil
		}
	}
	if !ok {
		c.errorf("E0502", f.Span, "`@ref %s` field `%s` must have type `%s` or `Maybe %s`, got `%s`", a.Arg, f.Name, types.String(idType), types.String(idType), types.String(ft))
	}
}

func imports(m *syntax.Module, name string) bool {
	for _, im := range m.Imports {
		if im.Module == name {
			return true
		}
	}
	return false
}

func isScalar(t types.Type) bool {
	switch t := t.(type) {
	case *types.Con:
		switch t.Name {
		case "Int", "Float", "Bool", "Text", "Date", "Time":
			return true
		}
	case *types.Brand:
		return true
	case *types.Union:
		if t.Name == "Maybe" {
			return false
		}
		for _, ct := range t.Constructors() {
			if ct.Arg != nil {
				return false
			}
		}
		return true
	}
	return false
}

func maybeOf(t types.Type) (types.Type, bool) {
	if un, ok := t.(*types.Union); ok && un.Name == "Maybe" && len(un.Args) == 1 {
		return un.Args[0], true
	}
	return nil, false
}

func storable(t types.Type) bool {
	if inner, ok := maybeOf(t); ok {
		return isScalar(inner)
	}
	return isScalar(t)
}

func formRepresentable(t types.Type) bool {
	if isCon(t, "Time") {
		return false
	}
	return storable(t)
}

func urlRepresentable(t types.Type) bool {
	if inner, ok := maybeOf(t); ok {
		return urlRepresentable(inner)
	}
	if app, ok := t.(*types.App); ok && app.Con == "List" && len(app.Args) == 1 {
		return urlRepresentable(app.Args[0])
	}
	if isCon(t, "Float") || isCon(t, "Date") || isCon(t, "Time") {
		return false
	}
	return isScalar(t)
}

// checkRepresentable runs after every module's types are defined, so payload unions
// declared later in a file (or in another module) are seen with their constructors.
func (mc *modCtx) checkRepresentable() {
	c := mc.c
	for _, d := range c.live(mc.m) {
		switch d := d.(type) {
		case *syntax.TableDecl:
			for _, f := range d.Fields {
				if ft := mc.typeOf(f.Type, nil); !storable(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "table field `%s` has type `%s`, which cannot be stored; model nesting with a second table and `@ref`", f.Name, types.String(ft))
				}
			}
		case *syntax.FormDecl:
			for _, f := range d.Fields {
				if ft := mc.typeOf(f.Type, nil); !formRepresentable(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "form field `%s` has type `%s`, which cannot come from a form", f.Name, types.String(ft))
				}
			}
		}
	}
}

// defineApp reads session/login/msg from App.
func (mc *modCtx) defineApp() {
	c := mc.c
	c.appMsg = &types.Union{Module: "App", Name: "Msg"}
	for _, d := range c.live(mc.m) {
		switch d := d.(type) {
		case *syntax.SessionDecl:
			rec := &types.Record{Module: "App", Name: "Session"}
			for _, f := range d.Fields {
				ft := mc.typeOf(f.Type, nil)
				if !formRepresentable(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "session field `%s` has type `%s`, which cannot be stored in a session", f.Name, types.String(ft))
				}
				mc.deferDefault(f, ft)
				rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
			}
			c.session = rec
		case *syntax.MsgDecl:
			mc.addMsg(c.appMsg, d, "App")
		}
	}
	mc.session = c.session
}

// addMsg appends a constructor for a msg declaration to union u; payload is a record of params (+ form).
func (mc *modCtx) addMsg(u *types.Union, d *syntax.MsgDecl, owner string) {
	c := mc.c
	payload := &types.Record{Module: owner, Name: d.Name + "Params"}
	if d.Params != nil {
		for _, f := range d.Params.Fields {
			ft := mc.typeOf(f.Type, nil)
			if !urlRepresentable(ft) {
				c.errorf(diag.InvalidDecl, f.Span, "message parameter `%s` has type `%s`, which cannot appear in a URL", f.Name, types.String(ft))
			}
			mc.deferDefault(f, ft)
			payload.Fields = append(payload.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
		}
	}
	if d.Form != "" {
		if rec, ok := c.forms[c.key(mc.m.Name, d.Form)]; ok {
			payload.Fields = append(payload.Fields, types.Field{Name: "form", Type: rec})
		} else {
			c.errorf(diag.UnknownName, d.Span, "unknown form `%s`", d.Form)
		}
	}
	u.Ctors = append(u.Ctors, types.Ctor{Name: d.Name, Arg: payload})
	c.ctors[c.key(owner, d.Name)] = &ctorInfo{Union: u, Arg: payload}
}

// definePage builds Params and Msg for a page module and validates the page declaration.
func (mc *modCtx) definePage() {
	c := mc.c
	pd := c.prog.Pages[mc.m.Name]
	info := &pageInfo{Decl: pd, Module: mc.m.Name, Msgs: map[string]*syntax.MsgDecl{}, Params: &types.Record{Module: mc.m.Name, Name: "Params"}, Msg: &types.Union{Module: mc.m.Name, Name: "Msg"}}
	c.pages[mc.m.Name] = info
	mc.page = info
	segs := pathParams(pd.Path)
	declared := map[string]bool{}
	for _, f := range pd.Params.Fields {
		ft := mc.typeOf(f.Type, nil)
		declared[f.Name] = true
		if !urlRepresentable(ft) {
			c.errorf(diag.InvalidDecl, f.Span, "page parameter `%s` has type `%s`, which cannot appear in a URL", f.Name, types.String(ft))
		}
		mc.deferDefault(f, ft)
		info.Params.Fields = append(info.Params.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
	}
	for _, s := range segs {
		if !declared[s] {
			c.errorf(diag.InvalidDecl, pd.Span, "path segment `{%s}` has no matching parameter", s)
		}
	}
	if pd.Requires != "" {
		info.Requires = pd.Requires
		found := false
		for _, f := range c.session.Fields {
			if f.Name == pd.Requires {
				found = true
				if _, ok := maybeOf(f.Type); !ok {
					c.errorf(diag.InvalidDecl, pd.Span, "`requires %s`: session field `%s` must be a `Maybe`", pd.Requires, pd.Requires)
				}
			}
		}
		if !found {
			sp := pd.RequiresSpan
			if sp.File == "" {
				sp = pd.Span
			}
			c.errorf(diag.InvalidDecl, sp, "`requires %s`: the session has no field `%s`", pd.Requires, pd.Requires)
		}
		mc.session = narrow(c.session, pd.Requires)
	}
	for _, d := range c.live(mc.m) {
		if md, ok := d.(*syntax.MsgDecl); ok {
			info.Msgs[md.Name] = md
			mc.addMsg(info.Msg, md, mc.m.Name)
		}
	}
}

// pendingDefault is a field default (`x: Int = 0`), checked against its field's type once
// every constructor and value signature is known.
type pendingDefault struct {
	m *syntax.Module
	f *syntax.Field
	t types.Type
}

func (mc *modCtx) deferDefault(f *syntax.Field, ft types.Type) {
	if f.Default != nil && mc.c.prog.Std[mc.m.Name] == nil {
		mc.c.defaults = append(mc.c.defaults, pendingDefault{mc.m, f, ft})
	}
}

func (c *Checker) checkDefaults() {
	for _, d := range c.defaults {
		mc := c.ctx(d.m)
		mc.check(d.f.Default, d.t, newEnv(), effect{})
		mc.reportHoles()
	}
}

func lineOf(m *syntax.Module, line int) string {
	if line >= 1 && line <= len(m.Lines) {
		return m.Lines[line-1]
	}
	return ""
}

// pathParams extracts {name} segments.
func pathParams(path string) []string {
	var out []string
	for i := 0; i < len(path); i++ {
		if path[i] == '{' {
			j := strings.IndexByte(path[i:], '}')
			if j > 0 {
				out = append(out, path[i+1:i+j])
				i += j
			}
		}
	}
	return out
}

func (mc *modCtx) defineCapabilities() {
	c := mc.c
	for _, d := range c.live(mc.m) {
		cd, ok := d.(*syntax.CapDecl)
		if !ok {
			continue
		}
		entries := map[string]types.Type{}
		for _, e := range cd.Entries {
			params := map[string]bool{}
			et := mc.typeOf(e.Type, params)
			res := et
			if fn, isFn := et.(*types.Fun); isFn {
				res = fn.Result
			}
			if app, isApp := res.(*types.App); !isApp || (app.Con != "Query" && app.Con != "Task") {
				c.errorf(diag.InvalidDecl, e.Span, "capability entry `%s` must return a `Query` or `Task`", e.Name)
			}
			entries[e.Name] = et
		}
		c.caps[c.key(mc.m.Name, cd.Name)] = entries
	}
}

// declareValues records every value signature (E0007 when missing) and checks page/App shape (E0010).
func (mc *modCtx) declareValues() {
	c := mc.c
	has := map[string]bool{}
	for _, d := range c.live(mc.m) {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok {
			continue
		}
		has[vd.Name] = true
		if vd.Sig == nil {
			if mc.m.Name == "Prelude" || c.prog.Std[mc.m.Name] != nil {
				continue
			}
			ty := "?type"
			switch vd.Body.(type) {
			case *syntax.IntLit:
				ty = "Int"
			case *syntax.StrLit:
				ty = "Text"
			case *syntax.BoolLit:
				ty = "Bool"
			case *syntax.FloatLit:
				ty = "Float"
			}
			c.errorf(diag.MissingSig, vd.Span, "`%s` has no type signature; write `%s : %s` on the line before it", vd.Name, vd.Name, ty).Fix =
				&diag.Fix{Line: vd.Span.Start.Line, Col: 1, Replace: vd.Name + " : " + ty + "\n"}
			continue
		}
		params := map[string]bool{}
		c.values[c.key(mc.m.Name, vd.Name)] = mc.typeOf(vd.Sig, params)
	}
	if mc.page != nil {
		for _, need := range []string{"load", "view"} {
			if !has[need] {
				c.errorf(diag.MissingDecl, mc.m.Span, "page module `%s` has no `%s`", mc.m.Name, need)
			}
		}
		if _, ok := c.types[c.key(mc.m.Name, "Model")]; !ok {
			c.errorf(diag.MissingDecl, mc.m.Span, "page module `%s` has no `Model`", mc.m.Name)
		}
		if len(mc.page.Msgs) > 0 && !has["update"] {
			sp := mc.m.Span
			sp.Start = diag.Pos{Line: lastDeclLine(mc.m), Col: 1}
			c.errorf(diag.MissingDecl, sp, "page module `%s` declares messages but has no `update`", mc.m.Name)
		}
	} else if !mc.isApp && c.prog.Std[mc.m.Name] == nil {
		for _, bad := range []string{"load", "view", "update"} {
			if has[bad] {
				c.errorf(diag.MissingDecl, mc.m.Span, "`%s` belongs in a page module; `%s` has no `page` declaration", bad, mc.m.Name)
			}
		}
		for _, d := range mc.m.Decls {
			switch d := d.(type) {
			case *syntax.MsgDecl:
				c.errorf(diag.InvalidDecl, d.Span, "`msg` is only allowed in a page module or `App`")
			case *syntax.SessionDecl:
				c.errorf(diag.InvalidDecl, d.Span, "`session` is only allowed in `App`")
			case *syntax.LoginDecl:
				c.errorf(diag.InvalidDecl, d.Span, "`login` is only allowed in `App`")
			}
		}
	}
}

func lastDeclLine(m *syntax.Module) int {
	line := 1
	for _, d := range m.Decls {
		if l := d.GetSpan().Start.Line; l > line {
			line = l
		}
	}
	return line
}
