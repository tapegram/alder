package ir

// Children lists the direct subexpressions of e in IR order (nil entries possible; a Comp's
// absent Props is left out). Consumers walking the IR (gen, mutate) share it.
func Children(e Expr) []Expr {
	switch x := e.(type) {
	case *Call:
		return append([]Expr{x.Fn}, x.Args...)
	case *Partial:
		return append([]Expr{x.Fn}, x.Given...)
	case *Lambda:
		return []Expr{x.Body}
	case *Record:
		return fieldValues(x.Fields)
	case *Update:
		return append([]Expr{x.Base}, fieldValues(x.Fields)...)
	case *Field:
		return []Expr{x.Expr}
	case *Ctor:
		return []Expr{x.Payload}
	case *List:
		return x.Elems
	case *PageRef:
		return []Expr{x.Params}
	case *Match:
		return matchChildren(x)
	case *HMatch:
		return matchChildren((*Match)(x))
	case *If:
		return []Expr{x.Cond, x.Then, x.Else}
	case *Let:
		return []Expr{x.Value, x.Body}
	case *Op:
		return []Expr{x.L, x.R}
	case *Str:
		return x.Parts
	case *Widen:
		return []Expr{x.Expr}
	case *Elem:
		return append(attrValues(x.Attrs), x.Children...)
	case *Comp:
		out := x.Children
		if x.Props != nil {
			out = append([]Expr{x.Props}, out...)
		}
		return out
	case *HExpr:
		return []Expr{x.Expr}
	case *HFor:
		return []Expr{x.List, x.Body}
	case *HIf:
		return []Expr{x.Cond, x.Then, x.Else}
	case *Input:
		return append([]Expr{x.LabelExpr, x.Value, x.Options}, attrValues(x.WrapAttrs)...)
	case *FormEl:
		return append(attrValues(x.Attrs), x.Children...)
	case *EffPure:
		return []Expr{x.Value}
	case *EffBind:
		return []Expr{x.Effect, x.Body}
	case *EffMap:
		return []Expr{x.Fn, x.Effect}
	case *EffAll:
		return []Expr{x.Effects}
	case *EffFail:
		return []Expr{x.Message}
	case *DbGet:
		return []Expr{x.ID}
	case *DbSelect:
		out := append([]Expr{x.Base}, x.Where...)
		return append(append(out, x.Order...), x.Limit, x.Offset)
	case *DbRun:
		return []Expr{x.Select}
	case *DbInsert:
		return []Expr{x.Row}
	case *DbUpdate:
		return []Expr{x.ID, x.Fn}
	case *DbDelete:
		return []Expr{x.ID}
	case *DbGetScoped:
		return []Expr{x.ID}
	case *DbUpdateScoped:
		return []Expr{x.ID, x.Fn}
	case *DbDeleteScoped:
		return []Expr{x.ID}
	case *DbInsertScoped:
		return []Expr{x.Row}
	case *Cond:
		return append([]Expr{x.Value}, x.Subs...)
	case *CapCall:
		return x.Args
	case *TestRun:
		return []Expr{x.Task}
	case *TestEq:
		return []Expr{x.Left, x.Right}
	case *TestProp:
		if x.ForAllGen != nil {
			return []Expr{x.ForAllGen, x.Fn}
		}
		return []Expr{x.Fn}
	case *TestLogin:
		return []Expr{x.Session}
	case *TestGet:
		return []Expr{x.Params}
	case *TestSubmit:
		return []Expr{x.Params, x.Record}
	case *TestSubmitRaw:
		return []Expr{x.Params, x.Fields}
	case *TestClick:
		return []Expr{x.Params}
	}
	return nil // Lit, Var, Hole, HText, EffNotFound, ColumnRef, TableRef, Ord, TestTrace, TestFake, DbScoped, FuzzDerive
}

func matchChildren(x *Match) []Expr {
	out := []Expr{x.Subject}
	for _, c := range x.Cases {
		out = append(out, c.Body)
	}
	return append(out, x.Default)
}

func fieldValues(fs []FieldInit) []Expr {
	var out []Expr
	for _, f := range fs {
		out = append(out, f.Value)
	}
	return out
}

func attrValues(as []HAttr) []Expr {
	var out []Expr
	for _, a := range as {
		out = append(out, a.Value)
	}
	return out
}
