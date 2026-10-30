package gen

import (
	"fmt"
	"slices"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// GoType is the Go type of an Alder type:
//
//	Int Float Bool Text Unit         int64 float64 bool string struct{}
//	Date Time Page Error Never       rt.Date time.Time rt.Page error struct{}
//	File, Bytes                      rt.File, []byte
//	HttpError, Nav                   rt.HttpError, rt.Nav
//	Doc, Answer (test builds only)   rtesting.Doc, rtesting.Answer
//	Fuzz.Fuzzer a, Fuzz.Property     fuzz.Fuzzer[A], *rtesting.Property
//	Random.Generator a, Random.Seed  std.Generator[A], std.Seed
//	Text.Builder                     rt.TextBuilder
//	List a, Dict k v, Html a         []A, rt.Dict[K, V], html.Node (whatever a is)
//	Maybe a, Result e a              rt.Maybe[A], rt.Result[E, A]
//	Query a, Task a                  eff.Query[A], eff.Task[A]
//	Table t, Select t                *db.Table[T], db.Select[T]
//	Condition t, Order t             db.Cond, db.Order
//	Column t a, Id t                 string (a column is named by its field; an id is Text)
//	Insert t                         insert__M__T, the table's insert record (an anonymous
//	                                 struct of its InsertFields; Lower erases most uses to it)
//	nominal record or union M.T      M__T, with type arguments when generic (M__T[A])
//	anonymous record                 struct{ F1 T1; F2 T2 } (fields PascalCase, in order)
//	a -> b -> c                      func(A, B) C
//	type parameter a                 A
//
// A brand is its base type (the IR has none left); an unsolved inference variable (an
// unconstrained type) is `any`. The poison type never reaches gen: ir.Lower refuses it.
func GoType(t types.Type) string {
	switch t := t.(type) {
	case *types.Con:
		switch t.Name {
		case "Int":
			return "int64"
		case "Float":
			return "float64"
		case "Bool":
			return "bool"
		case "Text":
			return "string"
		case "Unit", "Never":
			return "struct{}"
		case "Date":
			return "rt.Date"
		case "Time":
			return "time.Time"
		case "Page":
			return "rt.Page"
		case "Error":
			return "error"
		case "Bytes":
			return "[]byte"
		case "File":
			return "rt.File"
		case "HttpError":
			return "rt.HttpError"
		case "Doc":
			return "rtesting.Doc"
		case "Property":
			if t.Module == "Fuzz" {
				return "*rtesting.Property"
			}
		case "Seed":
			if t.Module == "Random" {
				return "std.Seed"
			}
		case "Builder":
			if t.Module == "Text" {
				return "rt.TextBuilder"
			}
		}
		return "any"
	case *types.Param:
		return typeParam(t.Name)
	case *types.Brand:
		return GoType(t.Base)
	case *types.App:
		switch t.Con {
		case "List":
			return "[]" + GoType(t.Args[0])
		case "Dict":
			return "rt.Dict[" + GoType(t.Args[0]) + ", " + GoType(t.Args[1]) + "]"
		case "Html":
			return "html.Node"
		case "Query", "Task":
			return "eff." + t.Con + "[" + GoType(t.Args[0]) + "]"
		case "Table":
			return "*db.Table[" + GoType(t.Args[0]) + "]"
		case "Select":
			return "db.Select[" + GoType(t.Args[0]) + "]"
		case "Condition":
			return "db.Cond"
		case "Order":
			return "db.Order"
		case "Column", "Id":
			return "string"
		case "Fuzzer":
			return "fuzz.Fuzzer[" + GoType(t.Args[0]) + "]"
		case "Generator":
			return "std.Generator[" + GoType(t.Args[0]) + "]"
		case "Insert":
			if r, ok := t.Args[0].(*types.Record); ok && r.Name != "" && !r.Anonymous {
				return insertName(r.Module, r.Name)
			}
		}
		return "any"
	case *types.Record:
		if t.Name != "" && !t.Anonymous {
			return GoName(t.Module, t.Name) + typeArgs(t.Args)
		}
		if len(t.Fields) == 0 {
			return "struct{}"
		}
		var fs []string
		for _, f := range t.Fields {
			fs = append(fs, Field(f.Name)+" "+GoType(f.Type))
		}
		return "struct{ " + strings.Join(fs, "; ") + " }"
	case *types.Union:
		if t.Module == "Prelude" {
			switch t.Name {
			case "Maybe":
				return "rt.Maybe[" + GoType(t.Args[0]) + "]"
			case "Result":
				return "rt.Result[" + GoType(t.Args[0]) + ", " + GoType(t.Args[1]) + "]"
			case "Nav":
				return "rt.Nav"
			case "Answer":
				return "rtesting.Answer"
			}
		}
		return GoName(t.Module, t.Name) + typeArgs(t.Args)
	case *types.Fun:
		var ps []string
		for _, p := range t.Params {
			ps = append(ps, GoType(p))
		}
		return "func(" + strings.Join(ps, ", ") + ") " + GoType(t.Result)
	}
	return "any"
}

// typeArgs renders Go type arguments: "" for none, else "[A, B]".
func typeArgs(args []types.Type) string {
	if len(args) == 0 {
		return ""
	}
	var as []string
	for _, a := range args {
		as = append(as, GoType(a))
	}
	return "[" + strings.Join(as, ", ") + "]"
}

// typeParams lists the type parameters that occur in ts, in order of first occurrence. A
// parameter that occurs only inside Html is left out: Html a is html.Node whatever a is, and Go
// could never infer it. Every Go type parameter is `any`: no Alder type needs a Go constraint
// (rt.Dict keys any type by its canonical encoding, rt.Key).
func typeParams(ts ...types.Type) []string {
	var params []string
	seen := map[string]bool{}
	var walk func(t types.Type)
	walk = func(t types.Type) {
		switch t := t.(type) {
		case *types.Param:
			if !seen[t.Name] {
				seen[t.Name] = true
				params = append(params, t.Name)
			}
		case *types.Brand:
			walk(t.Base)
		case *types.App:
			if t.Con == "Html" {
				return
			}
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Record:
			if t.Name != "" && !t.Anonymous { // its parameters are in its arguments; it may be recursive
				for _, a := range t.Args {
					walk(a)
				}
				return
			}
			for _, f := range t.Fields {
				walk(f.Type)
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
	for _, t := range ts {
		walk(t)
	}
	return params
}

// typeParamDecl renders a Go type parameter list ("[A any, K any]"), "" for none.
func typeParamDecl(params []string) string {
	if len(params) == 0 {
		return ""
	}
	var ps []string
	for _, p := range params {
		ps = append(ps, typeParam(p)+" any")
	}
	return "[" + strings.Join(ps, ", ") + "]"
}

// typeParamUse renders the type parameters as arguments ("[A, K]"), "" for none.
func typeParamUse(params []string) string {
	if len(params) == 0 {
		return ""
	}
	var ps []string
	for _, p := range params {
		ps = append(ps, typeParam(p))
	}
	return "[" + strings.Join(ps, ", ") + "]"
}

// wrapped reports whether a constructor payload is a wrapped non-record payload ({ value: T }):
// its constructor function then takes the T itself.
func wrapped(p *ir.RecordType) bool {
	return p != nil && p.Name == "" && len(p.Fields) == 1 && p.Fields[0].Name == "value"
}

// unionParams are the Go type parameters of a union declaration.
func unionParams(u *ir.UnionType) (decl, use string) {
	return typeParamDecl(u.Params), typeParamUse(u.Params)
}

// typeDecl writes the Go declarations of one nominal type; boxed are the boxed record fields.
func typeDecl(w *strings.Builder, d *ir.TypeDecl, boxed boxedSet) {
	if d.Record != nil && len(d.Record.Fields) == 0 {
		fmt.Fprintf(w, "type %s%s struct{}\n\n", GoName(d.Module, d.Name), typeParamDecl(d.Record.Params))
		return
	}
	if d.Record != nil {
		fmt.Fprintf(w, "type %s%s struct {\n", GoName(d.Module, d.Name), typeParamDecl(d.Record.Params))
		fields(w, d.Record.Fields, boxed[d.Record.Module+"."+d.Record.Name])
		w.WriteString("}\n\n")
		return
	}
	u := d.Union
	name := GoName(u.Module, u.Name)
	decl, use := unionParams(u)
	tt := tagType(u.Module, u.Name)
	fmt.Fprintf(w, "type %s%s struct {\nTag %s\n", name, decl, tt)
	for _, c := range u.Ctors {
		if c.Payload != nil {
			fmt.Fprintf(w, "%s *%s\n", ctorField(c.Name), payloadType(u, c, use))
		}
	}
	w.WriteString("}\n\n")
	fmt.Fprintf(w, "type %s uint8\n\n", tt)
	if len(u.Ctors) > 0 {
		w.WriteString("const (\n")
		for i, c := range u.Ctors {
			fmt.Fprintf(w, "%s %s = %d\n", tagConst(u.Module, u.Name, c.Name), tt, i)
		}
		w.WriteString(")\n\n")
	}
	for _, c := range u.Ctors {
		if c.Payload != nil && c.Payload.Name == "" {
			fmt.Fprintf(w, "type %s%s struct {\n", payloadName(u.Module, u.Name, c.Name), decl)
			fields(w, c.Payload.Fields, nil)
			w.WriteString("}\n\n")
		}
	}
	for _, c := range u.Ctors {
		tc := tagConst(u.Module, u.Name, c.Name)
		switch {
		case c.Payload == nil && decl == "":
			fmt.Fprintf(w, "var %s = %s{Tag: %s}\n\n", ctorName(u.Module, u.Name, c.Name), name, tc)
		case c.Payload == nil: // a generic union's payload-less value is built where it is used
		case wrapped(c.Payload):
			fmt.Fprintf(w, "func %s%s(v %s) %s%s {\nreturn %s%s{Tag: %s, %s: &%s{Value: v}}\n}\n\n",
				ctorName(u.Module, u.Name, c.Name), decl, GoType(c.Payload.Fields[0].Type), name, use,
				name, use, tc, ctorField(c.Name), payloadType(u, c, use))
		default:
			fmt.Fprintf(w, "func %s%s(v %s) %s%s {\nreturn %s%s{Tag: %s, %s: &v}\n}\n\n",
				ctorName(u.Module, u.Name, c.Name), decl, payloadType(u, c, use), name, use,
				name, use, tc, ctorField(c.Name))
		}
	}
}

// payloadType is the Go type of constructor c's payload inside union u, with u's type arguments
// args: the generated payload struct for an anonymous record, else the named record at the
// arguments the union declares it with (`Boxed (Box a)` → M__Box[A]).
func payloadType(u *ir.UnionType, c ir.CtorT, args string) string {
	if c.Payload.Name == "" {
		return payloadName(u.Module, u.Name, c.Name) + args
	}
	if c.PayloadType != nil {
		return GoType(c.PayloadType)
	}
	return GoName(c.Payload.Module, c.Payload.Name)
}

func fields(w *strings.Builder, fs []ir.FieldT, boxed map[string]bool) {
	for _, f := range fs {
		star := ""
		if boxed[f.Name] {
			star = "*"
		}
		fmt.Fprintf(w, "%s %s%s\n", Field(f.Name), star, GoType(f.Type))
	}
}

// Boxed fields. A Go struct may not contain itself by value, but an Alder record may reach
// itself through a field held by value: `type Node = { v: Int, next: Maybe Node }` would be
// `struct { Next rt.Maybe[M__Node] }`, and rt.Maybe holds its value directly ("invalid
// recursive type"). Such a field is boxed: its Go type is a pointer (`Next *rt.Maybe[M__Node]`),
// reads dereference it (`(*n.Next)`), and record literals, updates and component props box the
// new value (`new(v)`). The pointer is never nil and its target never changes, so an update
// shares it, and rt.Eq and rt.Key follow it.
//
// The rule: field f of nominal record R is boxed when f's type is not a nominal record without
// type arguments and f's type reaches R by value. Reaching by value follows Maybe and Result
// arguments, anonymous records' fields, and nominal records: their declared fields and their
// type arguments (conservatively: a parameter may be held by value). List, Dict, functions and
// other unions (whose payloads are behind pointers) stop it. So mutual recursion through Maybe
// boxes the Maybe field of each record on the cycle; a record held directly (`b: B`) is never
// boxed, as a cycle of such fields alone is an infinite type and any other cycle has a boxed
// field on it.
type boxedSet map[string]map[string]bool // "Module.Name" → field names

func (b boxedSet) has(module, name, field string) bool { return b[module+"."+name][field] }

// of reports whether field of the record type t is boxed (t a nominal record).
func (b boxedSet) of(t types.Type, field string) bool {
	if r, ok := unbrand(t).(*types.Record); ok && r.Name != "" && !r.Anonymous {
		return b.has(r.Module, r.Name, field)
	}
	return false
}

// boxedFields computes the boxed fields of every nominal record of p.
func boxedFields(p *ir.Program) boxedSet {
	decls := map[string]*ir.RecordType{}
	for _, m := range p.Modules {
		for _, d := range m.Types {
			if d.Record != nil {
				decls[d.Record.Module+"."+d.Record.Name] = d.Record
			}
		}
	}
	if p.Session != nil {
		decls[p.Session.Module+"."+p.Session.Name] = p.Session
	}
	// direct[R]: the nominal records R's fields hold by value, one step
	direct := map[string][]string{}
	for k, d := range decls {
		for _, f := range d.Fields {
			direct[k] = append(direct[k], byValue(f.Type)...)
		}
	}
	reaches := func(from []string, target string) bool {
		seen := map[string]bool{}
		stack := slices.Clone(from)
		for len(stack) > 0 {
			k := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if k == target {
				return true
			}
			if !seen[k] {
				seen[k] = true
				stack = append(stack, direct[k]...)
			}
		}
		return false
	}
	out := boxedSet{}
	for k, d := range decls {
		for _, f := range d.Fields {
			if r, ok := unbrand(f.Type).(*types.Record); ok && r.Name != "" && !r.Anonymous && len(r.Args) == 0 {
				continue
			}
			if reaches(byValue(f.Type), k) {
				if out[k] == nil {
					out[k] = map[string]bool{}
				}
				out[k][f.Name] = true
			}
		}
	}
	return out
}

func unbrand(t types.Type) types.Type {
	for {
		b, ok := t.(*types.Brand)
		if !ok {
			return t
		}
		t = b.Base
	}
}

// byValue lists the nominal records ("Module.Name") a value of type t holds by value without
// going through another nominal record.
func byValue(t types.Type) []string {
	var out []string
	var walk func(t types.Type)
	walk = func(t types.Type) {
		switch t := unbrand(t).(type) {
		case *types.Record:
			if t.Name != "" && !t.Anonymous {
				out = append(out, t.Module+"."+t.Name)
				for _, a := range t.Args {
					walk(a)
				}
				return
			}
			for _, f := range t.Fields {
				walk(f.Type)
			}
		case *types.Union:
			if t.Module == "Prelude" && (t.Name == "Maybe" || t.Name == "Result") {
				for _, a := range t.Args {
					walk(a)
				}
			}
		}
	}
	walk(t)
	return out
}
