// Declaration pass: types, tables, forms, pages, msgs, capabilities, value signatures.
package check

import (
	"fmt"
	"strconv"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// declareTypeNames creates placeholders so types can refer to each other in any order.
func (c *Checker) declareTypeNames(m *syntax.Module) {
	seen, tests := map[string]diag.Span{}, map[string]diag.Span{}
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
				c.types[k] = &types.Record{Module: m.Name, Name: d.Name, Params: d.Params}
			case d.Union != nil:
				c.types[k] = &types.Union{Module: m.Name, Name: d.Name, Params: d.Params}
			default: // opaque (stdlib): represented as App with Param args, or Con when no params
				if len(d.Params) == 0 {
					c.types[k] = &types.Con{Name: d.Name, Module: m.Name}
					if m.Name == "Prelude" {
						c.types[k] = &types.Con{Name: d.Name}
					}
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
		case *syntax.FakeDecl:
			c.declareFake(m, d, dup, &cur)
		case *syntax.ValueDecl:
			dup(d.Name, d.Span)
		case *syntax.MsgDecl:
			dup(d.Name, d.Span)
		case *syntax.TestDecl: // test names are a namespace of their own
			if prev, ok := tests[d.Name]; ok {
				c.errorf(diag.Duplicate, d.Span, "test %s is already declared at line %d", strconv.Quote(d.Name), prev.Start.Line)
				c.losers[d] = true
				continue
			}
			tests[d.Name] = d.Span
		}
	}
}

// live returns m's declarations without the duplicates declareTypeNames reported:
// the first declaration of a name wins and the others are never defined or checked. The
// tables of a `fake` come right after it, so every table pass sees them.
func (c *Checker) live(m *syntax.Module) []syntax.Decl {
	out := make([]syntax.Decl, 0, len(m.Decls))
	for _, d := range m.Decls {
		if c.losers[d] {
			continue
		}
		out = append(out, d)
		if fd, ok := d.(*syntax.FakeDecl); ok { // a fake's tables follow it, as tables of the module
			for _, t := range fd.Tables {
				if !c.losers[t] {
					out = append(out, t)
				}
			}
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
					if c.prog.IsPageModule(t.Ref.Module) {
						// a type declaration (resolved before the page's parameters are known),
						// or another module's page: the record is the page's own, for its load
						c.errorf(diag.TypeMismatch, t.Span, "`Params` is a page's parameter type and cannot be stored in another type; copy the fields you need (`type Model = { q: Text }`, filled with `params.q` in `load`)")
						return types.Err
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
					c.errorf("E0601", t.Span, "`Msg` is only defined in page modules; a shared component is typed `Html a`").Fix = diag.ReplaceSpan(t.Span, "a")
					return types.Err
				case "Session":
					if c.prog.Modules["App"] == nil && c.prog.Std[mc.m.Name] == nil {
						c.errorf(diag.MissingDecl, t.Span, "`Session` is the app's session, declared in `App.alder` (`session { … }`); this app has no `App` module")
						return types.Err
					}
					return mc.session
				}
			}
			// the declared name: `TableTodo`, the qualifier of `import Table.Todo as
			// TableTodo`, names that module's type `Todo`
			name := t.Ref.Name
			if name == "" {
				name = t.Name
			}
			mc.fakeTableUse(c.key(t.Ref.Module, name), name, t.Span)
			if t.Ref.Module == "Prelude" && testTypes[name] {
				mc.testTypeUse(name, t.Span)
			}
			decl, ok := c.types[c.key(t.Ref.Module, name)]
			if !ok {
				return types.Err
			}
			if n := typeArity(decl); len(args) != n {
				plural := "s"
				if n == 1 {
					plural = ""
				}
				c.errorf(diag.Arity, t.Span, "`%s` takes %d type argument%s, got %d", t.Name, n, plural, len(args))
				return types.Err
			}
			if app, ok := decl.(*types.App); ok && app.Con == "Dict" && len(args) == 2 && containsFun(args[0]) {
				c.errorf(diag.TypeMismatch, t.Span, "%s", dictKeyMessage(args[0], mc.ts(args[0])))
				return types.Err
			}
			out := applyArgs(decl, args)
			if r, ok := out.(*types.Record); ok && r.Def != nil && !c.typesDefined {
				c.early = append(c.early, r) // its declaration may not have its fields yet
			}
			return out
		}
		return types.Err // unresolved: the resolver reported it
	case *syntax.TypeVar:
		if params != nil && params[t.Name] {
			return &types.Param{Name: t.Name}
		}
		// an unknown lowercase type is a type variable in a signature (implicitly quantified)
		if params != nil {
			params[t.Name] = true
			return &types.Param{Name: t.Name}
		}
		// a type variable in a field (table, form, session, parameters): nothing has reported it,
		// so it stays a variable and the field's representability check rejects it
		return c.u.Fresh()
	case *syntax.RecordType:
		r := &types.Record{Anonymous: true}
		for _, f := range t.Fields {
			ft := mc.typeOf(f.Type, params)
			if f.Default != nil {
				mc.anonDefault(f, ft)
			}
			r.Fields = append(r.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil, Default: f.Default})
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
	return types.Err
}

// typeArity is the number of type arguments declared type decl takes.
func typeArity(decl types.Type) int {
	switch d := decl.(type) {
	case *types.Union:
		return len(d.Params)
	case *types.Record:
		return len(d.Params)
	case *types.App:
		return len(d.Args)
	}
	return 0
}

// applyArgs instantiates a declared type with arguments: unions and generic records get Args (a
// record also its fields at those arguments); opaque Apps substitute their Params.
func applyArgs(decl types.Type, args []types.Type) types.Type {
	switch d := decl.(type) {
	case *types.Union:
		if len(args) == 0 {
			return d
		}
		return d.WithArgs(args)
	case *types.Record:
		if len(args) == 0 || len(d.Params) == 0 {
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
					if f.Default != nil { // a literal of a declared record names every field
						c.bag.Warn(diag.PlainDefault, f.Span, "field `%s`: a default on a plain record type never applies; remove it", f.Name)
					}
					mc.checkRegular(tt, f, ft)
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

// checkRegular reports E0011 when field f of generic record decl refers to decl at other
// arguments than its own parameters (`type T a = { more: List (T (List a)) }`): such a record
// has no finite instance, in the checker or in Go.
func (mc *modCtx) checkRegular(decl *types.Record, f *syntax.Field, ft types.Type) {
	if len(decl.Params) == 0 {
		return
	}
	var bad types.Type
	var walk func(t types.Type)
	walk = func(t types.Type) {
		switch t := t.(type) {
		case *types.Record:
			if t.Anonymous {
				for _, fl := range t.Fields {
					walk(fl.Type)
				}
				return
			}
			if (t == decl || t.Def == decl) && bad == nil {
				for i, a := range t.Args {
					if p, ok := a.(*types.Param); !ok || i >= len(decl.Params) || p.Name != decl.Params[i] {
						bad = t
					}
				}
			}
			for _, a := range t.Args {
				walk(a)
			}
		case *types.App:
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Union:
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Fun:
			for _, p := range t.Params {
				walk(p)
			}
			walk(t.Result)
		}
	}
	walk(ft)
	if bad != nil {
		mc.c.errorf(diag.InvalidDecl, f.Span, "field `%s` refers to `%s` as `%s`; a record may refer to itself only with its own parameters (`%s %s`)",
			f.Name, decl.Name, mc.ts(bad), decl.Name, strings.Join(decl.Params, " "))
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
	_, fake := c.fakeTables[c.key(mc.m.Name, d.Name)]
	if d.Name != lastSegment(mc.m.Name) && !fake { // reported once here; uses elsewhere point back at it (names); a fake's tables belong to the fake
		c.errorf(diag.TablePlacement, d.Span, "a table is declared in a module named after it: `table %s` belongs in `app/Table/%s.alder`", d.Name, d.Name)
	}
	rec := c.types[c.key(mc.m.Name, d.Name)].(*types.Record)
	info := &types.TableInfo{Module: mc.m.Name, Name: d.Name, Record: rec, Columns: map[string]types.Type{}, Insert: &types.Record{Module: mc.m.Name, Name: "Insert " + d.Name}}
	idCount := 0
	c.checkTableAttrs(d)
	for _, f := range d.Fields {
		ft := mc.typeOf(f.Type, nil)
		mc.deferDefault(f, ft)
		c.checkDeleteRules(f)
		if f.Default != nil && !LiteralDefault(f.Default) {
			c.errorf(diag.TableDefault, f.Default.GetSpan(), "a table field's default must be a literal (migrations write it into the database): got an expression")
		}
		rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
		info.Columns[f.Name] = ft
		generated := false
		for _, a := range f.Attrs {
			switch a.Name {
			case "id":
				idCount++
				generated = true
				info.IDField, info.IDType = f.Name, ft
				if b, ok := ft.(*types.Brand); !types.IsErr(ft) && (!ok || !isCon(b.Base, "Text")) {
					c.errorf(diag.InvalidDecl, f.Span, "`@id` field must be a `brand Text`, got `%s`", mc.ts(ft))
				}
			case "created", "updated":
				generated = true
				if !isCon(ft, "Time") && !types.IsErr(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "`@%s` field must have type `Time`", a.Name)
				}
			case "ref":
				mc.checkRef(f, a, ft, fake)
			case "renamed":
				// checkTableAttrs reports a malformed or self-naming `@renamed`
			case "owner", "partition":
				// checkOwners validates it once App's session is known (E0024, E0032)
			case "unique", "cascade", "restrict":
				// checkDeleteRules reports a delete rule without `@ref`, before it, or a second one
			case "setNull":
				// without `@ref` the rule itself is the error (checkDeleteRules): no second one here
				if _, ok := maybeOf(ft); !ok && !types.IsErr(ft) && hasAttr(f, "ref") {
					c.errorf(diag.TypeMismatch, a.Span, "`@setNull` needs a `Maybe` reference, got `%s`; write `%s: Maybe %s`", mc.ts(ft), f.Name, mc.ts(ft))
				}
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

// LiteralDefault reports whether e is a table field default a migration can write into the
// database (E0022): `True`/`False`, an integer, a float, a string without `{…}`, a constructor
// without arguments (`Reader`, `Nothing`), or `Just` of one of those. The checker's types make sure
// a constructor here is payload-less: a table field's type is a scalar or `Maybe` of one.
func LiteralDefault(e syntax.Expr) bool {
	switch e := e.(type) {
	case *syntax.IntLit, *syntax.FloatLit, *syntax.BoolLit, *syntax.CtorRef:
		return true
	case *syntax.StrLit:
		for _, p := range e.Parts {
			if p.Expr != nil {
				return false
			}
		}
		return true
	case *syntax.App:
		if fn, ok := e.Fn.(*syntax.CtorRef); ok && fn.Name == "Just" && len(e.Args) == 1 {
			if _, nested := e.Args[0].(*syntax.App); !nested {
				return LiteralDefault(e.Args[0])
			}
		}
	}
	return false
}

// checkTableAttrs reports (E0011) an attribute on a table other than `@renamed`, and every
// misuse of `@renamed "old"` on the table or its fields: an old name that is not an identifier
// of the right kind, one naming the table or field itself, a second `@renamed`, two fields
// renamed from one name, and an old name that is still a field of the table.
func (c *Checker) checkTableAttrs(d *syntax.TableDecl) {
	seenTable := false
	for _, a := range d.Attrs {
		switch {
		case a.Name != "renamed":
			c.errorf(diag.InvalidDecl, a.Span, "a table takes only `@renamed \"OldName\"` after its name, not `@%s`", a.Name)
		case seenTable:
			c.errorf(diag.InvalidDecl, a.Span, "a table takes one `@renamed`")
		case !isIdent(a.Arg, true):
			c.errorf(diag.InvalidDecl, a.Span, "`@renamed %q`: the old name must be a table name such as `Article`", a.Arg)
		case a.Arg == d.Name:
			c.errorf(diag.InvalidDecl, a.Span, "`@renamed %q` names the table itself: write the table's old name, or remove the attribute", a.Arg)
		}
		seenTable = seenTable || a.Name == "renamed"
	}
	current := map[string]bool{}
	for _, f := range d.Fields {
		current[f.Name] = true
	}
	from := map[string]string{} // old name → the field renamed from it
	for _, f := range d.Fields {
		seen := false
		for _, a := range f.Attrs {
			if a.Name != "renamed" {
				continue
			}
			switch {
			case seen:
				c.errorf(diag.InvalidDecl, a.Span, "a field takes one `@renamed`")
			case !isIdent(a.Arg, false):
				c.errorf(diag.InvalidDecl, a.Span, "`@renamed %q`: the old name must be a field name such as `heading`", a.Arg)
			case a.Arg == f.Name:
				c.errorf(diag.InvalidDecl, a.Span, "`@renamed %q` names the field itself: write the field's old name, or remove the attribute", a.Arg)
			case current[a.Arg]:
				c.errorf(diag.InvalidDecl, a.Span, "`@renamed %q`: the table still has a field `%s`; a renamed field's old name must be gone", a.Arg, a.Arg)
			case from[a.Arg] != "":
				c.errorf(diag.InvalidDecl, a.Span, "fields `%s` and `%s` are both `@renamed %q`", from[a.Arg], f.Name, a.Arg)
			default:
				from[a.Arg] = f.Name
			}
			seen = true
		}
	}
}

// isIdent reports whether s is an Alder name: a letter (upper case when upper) followed by
// letters, digits and single underscores.
func isIdent(s string, upper bool) bool {
	if s == "" || strings.Contains(s, "__") {
		return false
	}
	for i, r := range s {
		switch {
		case i == 0 && upper && r >= 'A' && r <= 'Z', i == 0 && !upper && r >= 'a' && r <= 'z':
		case i > 0 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_'):
		default:
			return false
		}
	}
	return true
}

// deleteRules are the attributes that say what deleting a referenced row does to a `@ref`
// column's row; `@restrict` is the default.
var deleteRules = map[string]bool{"cascade": true, "restrict": true, "setNull": true}

// checkDeleteRules reports (E0011) a delete rule (`@cascade`, `@restrict`, `@setNull`) on a
// field without `@ref`, one written before `@ref Table`, and a second delete rule on one field.
func (c *Checker) checkDeleteRules(f *syntax.Field) {
	ref := -1
	for i, a := range f.Attrs {
		if a.Name == "ref" && ref < 0 {
			ref = i
		}
	}
	first := ""
	for i, a := range f.Attrs {
		if !deleteRules[a.Name] {
			continue
		}
		switch {
		case ref < 0:
			c.errorf(diag.InvalidDecl, a.Span, "`@%s` belongs on a `@ref` column, after `@ref Table`", a.Name)
		case i < ref:
			c.errorf(diag.InvalidDecl, a.Span, "`@%s` goes after `@ref %s`: write `@ref %s @%s`", a.Name, f.Attrs[ref].Arg, f.Attrs[ref].Arg, a.Name)
		case first != "":
			c.errorf(diag.InvalidDecl, a.Span, "a `@ref` column takes one delete rule; it has `@%s` and `@%s`", first, a.Name)
		}
		if first == "" {
			first = a.Name
		}
	}
}

// checkRef validates `@ref T` after all tables have placeholders; the referenced table's id type is read from its decl.
// fromFake says the field's table is a fake's: only such a table may refer to a fake's table
// (E0021 otherwise).
func (mc *modCtx) checkRef(f *syntax.Field, a *syntax.Attr, ft types.Type, fromFake bool) {
	c := mc.c
	// find the table decl named a.Arg in this module or an import
	var idType types.Type
	for _, m := range c.allModules() {
		if m.Name != mc.m.Name && !imports(mc.m, m.Name) {
			continue
		}
		for _, d := range c.live(m) {
			if td, ok := d.(*syntax.TableDecl); ok && td.Name == a.Arg {
				if owner, isFake := c.fakeTables[c.key(m.Name, td.Name)]; isFake && !fromFake {
					c.errorf(FakeTable, a.Span, "`%s` is a fake's table: only tests and `fake %s` may use it", td.Name, owner)
					return
				}
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
		c.errorf("E0502", f.Span, "`@ref %s` field `%s` must have type `%s` or `Maybe %s`, got `%s`", a.Arg, f.Name, mc.ts(idType), mc.ts(idType), mc.ts(ft))
	}
}

// imports reports whether m imports module name. Import lines keep the full path (only
// references use the qualifier), so it compares paths.
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
		if types.IsErr(t) { // unresolved: already reported, so every representability check accepts it
			return true
		}
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

// checkFinite reports E0011 when record decl contains itself by value (`type R = { r: R }`),
// directly or through other records, anonymous records and generic record instances: no value
// of it can ever be built, and its Go struct would be infinite. Maybe, Result, List, Dict, other
// unions and functions hold their contents indirectly, so a field through one of them is fine.
func (mc *modCtx) checkFinite(decl *types.Record, d *syntax.TypeDecl) {
	var reaches func(t types.Type, seen map[*types.Record]bool) bool
	reaches = func(t types.Type, seen map[*types.Record]bool) bool {
		r, ok := t.(*types.Record)
		if !ok {
			return false
		}
		if r == decl || r.Def == decl {
			return true
		}
		if seen[r] {
			return false
		}
		seen[r] = true
		for _, f := range r.Fields {
			if reaches(f.Type, seen) {
				return true
			}
		}
		return false
	}
	for i, f := range decl.Fields {
		if i < len(d.Record.Fields) && reaches(f.Type, map[*types.Record]bool{}) {
			mc.c.errorf(diag.InvalidDecl, d.Record.Fields[i].Span, "`%s` contains itself through field `%s`; wrap the field in `Maybe` or a union", decl.Name, f.Name)
			return
		}
	}
}

// checkRepresentable runs after every module's types are defined, so payload unions
// declared later in a file (or in another module) are seen with their constructors.
func (mc *modCtx) checkRepresentable() {
	c := mc.c
	for _, d := range c.live(mc.m) {
		switch d := d.(type) {
		case *syntax.TypeDecl:
			if rec, ok := c.types[c.key(mc.m.Name, d.Name)].(*types.Record); ok && d.Record != nil {
				mc.checkFinite(rec, d)
			}
		case *syntax.TableDecl:
			for _, f := range d.Fields {
				if ft := mc.typeOf(f.Type, nil); !storable(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "table field `%s` has type `%s`, which cannot be stored; model nesting with a second table and `@ref`", f.Name, mc.ts(ft))
				}
			}
		case *syntax.FormDecl:
			for _, f := range d.Fields {
				if ft := mc.typeOf(f.Type, nil); !formRepresentable(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "form field `%s` has type `%s`, which cannot come from a form", f.Name, mc.ts(ft))
				}
			}
		}
	}
}

// defineApp reads session/login/msg from App.
func (mc *modCtx) defineApp() {
	c := mc.c
	c.appMsg = &types.Union{Module: "App", Name: "Msg"}
	var deploys []*syntax.DeployDecl
	var themes []*syntax.ThemeDecl
	for _, d := range c.live(mc.m) {
		switch d := d.(type) {
		case *syntax.DeployDecl:
			deploys = append(deploys, d)
		case *syntax.ThemeDecl:
			themes = append(themes, d)
		case *syntax.SessionDecl:
			rec := c.session // the record types declared in pass 2 already point at
			rec.Fields = nil
			for _, f := range d.Fields {
				ft := mc.typeOf(f.Type, nil)
				if !formRepresentable(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "session field `%s` has type `%s`, which cannot be stored in a session", f.Name, mc.ts(ft))
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
	mc.checkDeploy(deploys)
	mc.checkTheme(themes)
	mc.reserveGenerated("the App module", map[string]string{"Msg": "App's messages", "Session": "the session"})
}

// addMsg appends a constructor for a msg declaration to union u; payload is a record of params (+ form).
func (mc *modCtx) addMsg(u *types.Union, d *syntax.MsgDecl, owner string) {
	c := mc.c
	payload := &types.Record{Module: owner, Name: d.Name + "Params"}
	c.paramRecs[c.key(owner, payload.Name)] = true
	if d.Params != nil {
		for _, f := range d.Params.Fields {
			ft := mc.typeOf(f.Type, nil)
			if !urlRepresentable(ft) {
				c.errorf(diag.InvalidDecl, f.Span, "message parameter `%s` has type `%s`, which cannot appear in a URL", f.Name, mc.ts(ft))
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
	c.paramRecs[c.key(mc.m.Name, "Params")] = true
	info.File = viewReturnsFile(c.live(mc.m))
	c.pages[mc.m.Name] = info
	mc.page = info
	segs := pathParams(pd.Path)
	declared := map[string]bool{}
	for _, f := range pd.Params.Fields {
		ft := mc.typeOf(f.Type, nil)
		declared[f.Name] = true
		if !urlRepresentable(ft) {
			c.errorf(diag.InvalidDecl, f.Span, "page parameter `%s` has type `%s`, which cannot appear in a URL", f.Name, mc.ts(ft))
		}
		mc.deferDefault(f, ft)
		info.Params.Fields = append(info.Params.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
	}
	for _, s := range segs {
		if !declared[s] {
			c.errorf(diag.InvalidDecl, pd.Span, "path segment `{%s}` has no matching parameter", s)
		}
	}
	mc.checkWireNames(pd, segs)
	if len(pd.Requires) > 0 || pd.Silo != "" {
		info.Requires = pd.Requires
		for i, req := range pd.Requires {
			found := false
			for _, f := range c.session.Fields {
				if f.Name == req {
					found = true
					if _, ok := maybeOf(f.Type); !ok {
						c.errorf(diag.InvalidDecl, pd.Span, "`requires %s`: session field `%s` must be a `Maybe`", req, req)
					}
				}
			}
			if !found {
				sp := pd.RequiresSpan
				if i > 0 && i < len(pd.FieldSpans) {
					sp = pd.FieldSpans[i]
				}
				if sp.File == "" {
					sp = pd.Span
				}
				c.errorf(diag.InvalidDecl, sp, "`requires %s`: the session has no field `%s`", req, req)
			}
		}
		info.Session = narrow(c.session, mc.checkRequiresM15(pd, info), mc.m.Name)
		mc.session = info.Session
	}
	if len(pd.Roles) > 0 {
		mc.checkRoles(pd, info)
	}
	for _, d := range c.live(mc.m) {
		if md, ok := d.(*syntax.MsgDecl); ok {
			info.Msgs[md.Name] = md
			mc.addMsg(info.Msg, md, mc.m.Name)
		}
	}
	mc.reserveGenerated("a page module", map[string]string{
		"Params": "the page's parameters", "Msg": "the page's messages", "Session": "the page's session"})
}

// viewReturnsFile reports whether a page module's declared `view` returns File. It reads the
// signature's syntax, so every module knows it before bodies are checked; `File` is a
// reserved Prelude name (E0012), so the name decides.
func viewReturnsFile(decls []syntax.Decl) bool {
	for _, d := range decls {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok || vd.Name != "view" || vd.Sig == nil {
			continue
		}
		t := vd.Sig
		for {
			ft, ok := t.(*syntax.FunType)
			if !ok {
				break
			}
			t = ft.Result
		}
		tn, ok := t.(*syntax.TypeName)
		// the written module: the stdlib's Prelude is written by its name (a stdlib module's
		// qualifier is its name), and an unqualified `File` is the Prelude's unless declared here
		return ok && tn.Name == "File" && (tn.Module == "" || tn.Module == "Prelude") && len(tn.Args) == 0
	}
	return false
}

// reserveGenerated reports E0012 for a type, table, form or component that takes the name of a
// type the compiler generates in this module (a page module or App): Params, Msg, Session, or
// `<Msg>Params` for a declared `msg`. The generated type and the user's declaration would get
// the same Go name (internal/gen/names.go). fixed names the fixed reserved names and what each
// is generated for.
func (mc *modCtx) reserveGenerated(where string, fixed map[string]string) {
	c := mc.c
	reserved := map[string]string{}
	for name, what := range fixed {
		reserved[name] = what
	}
	for _, d := range c.live(mc.m) {
		if md, ok := d.(*syntax.MsgDecl); ok {
			reserved[md.Name+"Params"] = "`msg " + md.Name + "`"
		}
	}
	for _, d := range c.live(mc.m) {
		var name string
		switch d := d.(type) {
		case *syntax.TypeDecl:
			name = d.Name
		case *syntax.TableDecl:
			name = d.Name
		case *syntax.FormDecl:
			name = d.Name
		case *syntax.ValueDecl: // a component: a capitalized value
			name = d.Name
		}
		if what, ok := reserved[name]; ok && name != "" {
			c.errorf(ReservedName, d.GetSpan(), "`%s` is reserved in %s (generated for %s); rename it", name, where, what)
		}
	}
}

// preludeReserved are the Prelude types a program writes in its own signatures. A declaration
// of one of these names (`form Query`) would hide it in its module, so every use of the name
// there, such as `load`'s `Query Model`, would read as the declaration. The Prelude's other
// types (`Order`, `Page`, `Form`, `Table`, `Id`, `Insert`, `Select`, `Condition`, `Column`) are
// Db's and the runtime's plumbing, which apps do not write, and stay free for domain names
// (`table Order`).
var preludeReserved = map[string]bool{
	"Query": true, "Task": true, "Html": true, "Maybe": true, "Result": true, "Nav": true,
	"List": true, "Dict": true, "Int": true, "Float": true, "Text": true, "Bool": true,
	"Unit": true, "Date": true, "Time": true, "Never": true, "Error": true, "Doc": true, "Answer": true, "File": true, "Bytes": true,
}

// reservePrelude reports E0012 for a type, table, form or union of an app module that takes a
// name of preludeReserved: its module's other errors would be a cascade of the name. It
// returns the files of the modules it reported, whose other errors quietErrors drops; those
// modules' bodies are not checked.
func (c *Checker) reservePrelude() map[string]bool {
	prelude := preludeReserved
	quiet := map[string]bool{}
	for _, name := range c.prog.Order {
		m := c.prog.Modules[name]
		for _, d := range c.live(m) {
			var decl, kind string
			switch d := d.(type) {
			case *syntax.TypeDecl:
				decl, kind = d.Name, "type"
			case *syntax.TableDecl:
				decl, kind = d.Name, "table"
			case *syntax.FormDecl:
				decl, kind = d.Name, "form"
			}
			for _, ctor := range ctorsOf(d) {
				if answerCtors[ctor.name] {
					c.errorf(ReservedName, ctor.span, "`%s` is a constructor of the Prelude's test type `Answer`; name the %s differently (for example `%s%s`)", ctor.name, ctor.kind, ctor.name, ctor.suffix)
				}
			}
			if prelude[decl] {
				c.errorf(ReservedName, d.GetSpan(), "`%s` is a Prelude type; name the %s differently (for example `%s%s`)", decl, kind, decl, strings.ToUpper(kind[:1])+kind[1:])
				quiet[m.File] = true
				if c.prog.Failed == nil {
					c.prog.Failed = map[string]bool{}
				}
				c.prog.Failed[name] = true
			}
		}
	}
	return quiet
}

// namedCtor is a constructor a declaration makes: a union's, or a `msg` (a constructor of its
// module's Msg).
type namedCtor struct {
	name, kind, suffix string // suffix: a word to suggest a different name with
	span               diag.Span
}

// ctorsOf lists the constructors declaration d makes.
func ctorsOf(d syntax.Decl) []namedCtor {
	switch d := d.(type) {
	case *syntax.TypeDecl:
		var out []namedCtor
		for _, ct := range d.Union {
			out = append(out, namedCtor{ct.Name, "constructor", d.Name, ct.Span})
		}
		return out
	case *syntax.MsgDecl:
		return []namedCtor{{d.Name, "message", "Msg", d.Span}}
	}
	return nil
}

// quietErrors drops the errors reported from index from on in the files of quiet, except E0012:
// in a module that declares a Prelude type's name they are a cascade of it.
func quietErrors(bag *diag.Bag, from int, quiet map[string]bool) {
	if len(quiet) == 0 {
		return
	}
	keep := bag.Items[:from]
	for _, d := range bag.Items[from:] {
		if d.Severity != "error" || d.Code == ReservedName || !quiet[d.Span.File] {
			keep = append(keep, d)
		}
	}
	bag.Items = keep
}

// ReservedName is E0012: a declaration takes the name of a type the compiler generates.
const ReservedName = "E0012"

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
	c.defaultsChecked = true
}

// anonDefault checks the default of a field of an anonymous record type written in source (a
// signature's `{ label: Text, kind: Text = "a" }`), once however often the type is converted:
// with the other defaults when they have not been checked yet, else at once (a type written in
// a body, such as a `let` annotation).
func (mc *modCtx) anonDefault(f *syntax.Field, ft types.Type) {
	c := mc.c
	if c.anonDefaults[f] || c.prog.Std[mc.m.Name] != nil {
		return
	}
	c.anonDefaults[f] = true
	c.res.DefaultModules[f.Default] = mc.m.Name
	if !c.defaultsChecked {
		c.defaults = append(c.defaults, pendingDefault{mc.m, f, ft})
		return
	}
	mc.check(f.Default, ft, newEnv(), effect{})
}

func lineOf(m *syntax.Module, line int) string {
	if line >= 1 && line <= len(m.Lines) {
		return m.Lines[line-1]
	}
	return ""
}

// checkPagePaths reports E0016 for a page whose path answers the same requests as an earlier
// page's (in module order): the same segments, with any parameter names, ignoring a trailing
// slash, as rt/web matches them.
func (c *Checker) checkPagePaths() {
	seen := map[string]string{}
	for _, name := range c.prog.Order {
		pd := c.prog.Pages[name]
		if pd == nil || !c.prog.IsPageModule(name) {
			continue
		}
		if why := PagePathProblem(pd.Path); why != "" {
			c.errorf(diag.PagePath, pd.Span, "page path `%s` %s; %s", pd.Path, why, PagePathRule)
			continue
		}
		shape := pathShape(pd.Path)
		if first, ok := seen[shape]; ok {
			c.errorf(diag.DuplicatePage, pd.Span, "pages `%s` and `%s` both answer `%s`; give one of them another path", first, name, pd.Path)
			continue
		}
		seen[shape] = name
	}
}

// PagePathRule is what a page path is (E0029), as its message and `alder new page` say it.
const PagePathRule = "a path is `/` followed by segments of letters, digits, `.`, `_`, `~`, `-` or `{param}`"

// PagePathProblem is what keeps path from being a path the router can serve, as words after
// "page path `…`" ("has an empty segment"), or "" when nothing does: a path is a single
// leading `/` and segments of `[A-Za-z0-9._~-]+` or a whole `{param}`, with no empty segment
// (`//evil.com`), no `.` or `..` (a browser resolves them away), and no query, fragment, space,
// quote or backslash. A trailing `/` is allowed: the router serves `/x/` as `/x` (E0016).
func PagePathProblem(path string) string {
	if !strings.HasPrefix(path, "/") {
		return "does not begin with `/`"
	}
	if path == "/" {
		return ""
	}
	segs := strings.Split(strings.TrimSuffix(path[1:], "/"), "/")
	for _, s := range segs {
		for _, r := range s {
			switch {
			case r == ' ':
				return "has a space"
			case r == '?':
				return "has a query (`?`): a page's query parameters are its `{ … }` fields without a `{param}` segment"
			case r == '#':
				return "has a fragment (`#`)"
			case r < 0x21 || r > 0x7e:
				return fmt.Sprintf("has the character %q", r)
			}
		}
	}
	for _, s := range segs {
		switch {
		case s == "":
			return "has an empty segment"
		case s == "." || s == "..":
			return "has the segment `" + s + "`, which a browser resolves away"
		case strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && len(s) > 2 && plainSegment(s[1:len(s)-1], false):
		case plainSegment(s, true):
		case strings.ContainsAny(s, "{}"):
			return "has the segment `" + s + "`, which is neither a name nor a whole `{param}`"
		default:
			for _, r := range s {
				if !segmentRune(r) {
					return "has the character `" + string(r) + "`"
				}
			}
		}
	}
	return ""
}

// checkWireNames checks the wire names of page pd's parameters (`e: Text = "" as "ElectionId"`,
// #55): only a query parameter (not a `{segment}` of path segments segs) takes one, it is a
// plain query key outside the message URLs' `_p.` keys, and no two query parameters read one
// key (a parameter without a wire name reads its name).
func (mc *modCtx) checkWireNames(pd *syntax.PageDecl, segs []string) {
	c := mc.c
	inPath := map[string]bool{}
	for _, s := range segs {
		inPath[s] = true
	}
	keys := map[string]string{} // query key → the parameter reading it
	for _, f := range pd.Params.Fields {
		if f.Wire != "" || f.WireSpan.File != "" {
			sp := f.WireSpan
			switch {
			case inPath[f.Name]:
				c.errorf(diag.InvalidDecl, sp, "page parameter `%s` is the path segment `{%s}`, not a query parameter: only a query parameter takes a wire name (`as %q`)", f.Name, f.Name, f.Wire)
				continue
			case f.Wire == "":
				c.errorf(diag.InvalidDecl, sp, "the wire name of page parameter `%s` is empty: write the query key the URL carries (`as \"Id\"`), or leave the `as` out", f.Name)
				continue
			case !plainQueryKey(f.Wire):
				c.errorf(diag.InvalidDecl, sp, "the wire name `%s` of page parameter `%s` is not a plain query key: use letters, digits, `-`, `_`, `.` and `~`", f.Wire, f.Name)
				continue
			case strings.HasPrefix(f.Wire, "_p."):
				c.errorf(diag.InvalidDecl, sp, "the wire name `%s` of page parameter `%s` starts with `_p.`, which Alder keeps for message URLs (they carry the page's parameters as `_p.<key>`)", f.Wire, f.Name)
				continue
			case inPath[f.Wire]:
				// message URLs carry path parameters as `_p.<name>` too: the two would share a key
				c.errorf(diag.InvalidDecl, sp, "the wire name `%s` of page parameter `%s` is the path segment `{%s}`: give `%s` another wire name", f.Wire, f.Name, f.Wire, f.Name)
				continue
			}
		}
		if inPath[f.Name] {
			continue
		}
		key, sp := f.Name, f.Span
		if f.Wire != "" {
			key, sp = f.Wire, f.WireSpan
		}
		if prev, ok := keys[key]; ok {
			c.errorf(diag.InvalidDecl, sp, "page parameters `%s` and `%s` both read the query key `%s`: give one of them another wire name (`as \"…\"`)", prev, f.Name, key)
			continue
		}
		keys[key] = f.Name
	}
}

// plainQueryKey reports whether k is a query key a URL carries as written: URL-unreserved
// characters (letters, digits, `-`, `_`, `.`, `~`), at least one.
func plainQueryKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.~", r)) {
			return false
		}
	}
	return true
}

// plainSegment reports whether s is a path segment of letters, digits, `_` and (literal, a
// segment rather than a parameter's name) `.`, `~`, `-`.
func plainSegment(s string, literal bool) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !segmentRune(r) || !literal && (r == '.' || r == '~' || r == '-') {
			return false
		}
	}
	return true
}

func segmentRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '~' || r == '-'
}

// pathShape is path as the router matches it: every `{param}` segment as `{}`, no trailing
// slash.
func pathShape(path string) string {
	segs := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for i, s := range segs {
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			segs[i] = "{}"
		}
	}
	return strings.Join(segs, "/")
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
			if app, isApp := res.(*types.App); !types.IsErr(res) && (!isApp || (app.Con != "Query" && app.Con != "Task")) {
				c.errorf(diag.InvalidDecl, e.Span, "capability entry `%s` must return a `Query` or `Task`", e.Name)
			}
			entries[e.Name] = et
		}
		c.caps[c.key(mc.m.Name, cd.Name)] = entries
	}
}

// declareValues records every value signature and checks page/App shape (E0010). A value with no
// signature records nothing: checkBodies infers its type for E0007.
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
			if c.prog.Failed[mc.m.Name] && c.prog.Std[mc.m.Name] == nil && !mc.infersTopLevel() {
				mc.missingSig(vd, "?type") // bodies of this module are not checked, so nothing is inferred
			}
			continue
		}
		params := map[string]bool{}
		c.values[c.key(mc.m.Name, vd.Name)] = mc.typeOf(vd.Sig, params)
	}
	if !mc.isApp && c.prog.Std[mc.m.Name] == nil {
		for _, d := range mc.m.Decls {
			c.m15Placement(d)
		}
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
		if len(mc.page.Msgs) > 0 && !has["update"] && !mc.page.File { // a file page's msg is E0034
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
			case *syntax.DeployDecl:
				c.errorf(diag.DeployDecl, d.Span, "`deploy` is only allowed in `App`")
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

// hasAttr reports whether field f carries attribute name.
func hasAttr(f *syntax.Field, name string) bool {
	for _, a := range f.Attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}
