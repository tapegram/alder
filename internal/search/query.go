package search

import (
	"fmt"
	"sort"
	"strings"

	"alder/internal/check"
	"alder/internal/syntax"
	"alder/internal/types"
)

// queryType reads src as a type query: its type, on a fresh clone of the index's base unifier that
// holds the query's wildcards. note is set, and t is nil, when a name is unknown or ambiguous or
// takes a different number of type arguments; bad is true when src does not parse as a type.
func (ix *Index) queryType(src string) (t types.Type, u *types.Unifier, note string, bad bool) {
	st, diags := syntax.ParseType(src, true)
	if len(diags) > 0 || st == nil {
		return nil, nil, "not a complete type", true
	}
	qc := &queryConv{ix: ix, u: ix.base.Clone()}
	t = qc.convert(st)
	if qc.note != "" {
		return nil, nil, qc.note, false
	}
	return t, qc.u, "", false
}

// queryConv converts a parsed query type; note is the first problem found.
type queryConv struct {
	ix   *Index
	u    *types.Unifier
	note string
}

func (qc *queryConv) convert(st syntax.Type) types.Type {
	if qc.note != "" {
		return nil
	}
	switch st := st.(type) {
	case *syntax.TypeVar:
		if st.Name == "_" {
			return qc.u.Fresh() // every `_` is its own variable
		}
		return &types.Param{Name: st.Name} // rigid
	case *syntax.FunType:
		fn := &types.Fun{}
		for _, p := range st.Params {
			fn.Params = append(fn.Params, qc.convert(p))
		}
		fn.Result = qc.convert(st.Result)
		return fn
	case *syntax.RecordType:
		r := &types.Record{Anonymous: true}
		for _, f := range st.Fields {
			r.Fields = append(r.Fields, types.Field{Name: f.Name, Type: qc.convert(f.Type)})
		}
		return r
	case *syntax.TypeName:
		shown := st.Name
		if st.Module != "" {
			shown = st.Module + "." + st.Name
		}
		decl, note := qc.ix.resolveType(st.Module, st.Name, shown)
		if note != "" {
			qc.note = note
			return nil
		}
		var args []types.Type
		for _, a := range st.Args {
			args = append(args, qc.convert(a))
		}
		if qc.note != "" {
			return nil
		}
		t, ok := check.ApplyTypeArgs(decl, args)
		if !ok {
			n := check.TypeArity(decl)
			plural := "s"
			if n == 1 {
				plural = ""
			}
			qc.note = fmt.Sprintf("`%s` takes %d type argument%s", shown, n, plural)
			return nil
		}
		return t
	}
	qc.note = "not a complete type"
	return nil
}

// resolveType finds the declared type a query names: the Prelude's for an unqualified Prelude
// name, else the only declaration with that name (in a module matching qualifier, when given).
// shown is the name as the query spells it, for the note.
func (ix *Index) resolveType(qualifier, name, shown string) (types.Type, string) {
	if qualifier == "" {
		if d, ok := ix.decls["Prelude."+name]; ok {
			return d, ""
		}
	}
	var keys []string
	for k := range ix.decls {
		dot := strings.LastIndex(k, ".")
		if dot < 0 || k[dot+1:] != name {
			continue
		}
		mod := k[:dot]
		if qualifier == "" || mod == qualifier || strings.HasSuffix(mod, "."+qualifier) {
			keys = append(keys, k)
		}
	}
	switch len(keys) {
	case 0:
		return nil, "no type named `" + shown + "`"
	case 1:
		return ix.decls[keys[0]], ""
	}
	sort.Strings(keys)
	return nil, "`" + shown + "` is ambiguous: " + strings.Join(keys, ", ")
}
