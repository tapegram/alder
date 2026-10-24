// internal/types/types.go
package types

import (
	"fmt"
	"strings"

	"alder/internal/syntax"
)

type Type interface{ isType() }

// Con is a builtin type without arguments. Module is "" for Prelude's, and the stdlib module
// that declares it otherwise (Http's HttpError), which qualifies its printed name.
type Con struct{ Name, Module string }
type Var struct {
	ID   int
	Name string
}
type Brand struct {
	Module, Name string
	Base         Type
	HasCheck     bool
}
type Field struct {
	Name       string
	Type       Type
	HasDefault bool
	// Default is the default's expression (`kind: Text = "a"`), nil when the field has none or
	// the record was not written in source (a generated record). The checker types it against Type.
	Default syntax.Expr
}

// Record is a record type: anonymous (structural) when Anonymous, else nominal Module.Name. A
// nominal record may have type parameters (`type Box a = { item: a }`): the declaration has
// Params and fields mentioning them; an instance (`Box Int`) has Args, Def pointing at the
// declaration, and Fields with the parameters substituted (see WithArgs).
type Record struct {
	Module, Name string
	Fields       []Field
	Anonymous    bool
	Params       []string // declared parameter names
	Args         []Type   // an instance's arguments, same length as Params
	Def          *Record  // declaration this instance was made from (nil on the declaration)
}
type Ctor struct {
	Name string
	Arg  Type
}
type Union struct {
	Module, Name string
	Args         []Type
	Ctors        []Ctor
	Def          *Union   // declaration this instance was copied from (nil on the declaration itself)
	Params       []string // declared parameter names, same length as Args
}
type App struct {
	Con  string
	Args []Type
}
type Fun struct {
	Params []Type
	Result Type
}
type Param struct{ Name string }

func (*Con) isType()    {}
func (*Var) isType()    {}
func (*Brand) isType()  {}
func (*Record) isType() {}
func (*Union) isType()  {}
func (*App) isType()    {}
func (*Fun) isType()    {}
func (*Param) isType()  {}

var (
	Int   = &Con{Name: "Int"}
	Float = &Con{Name: "Float"}
	Bool  = &Con{Name: "Bool"}
	Text  = &Con{Name: "Text"}
	Unit  = &Con{Name: "Unit"}
	Date  = &Con{Name: "Date"}
	Time  = &Con{Name: "Time"}
	Never = &Con{Name: "Never"}
	Error = &Con{Name: "Error"}
	Page  = &Con{Name: "Page"}
	File  = &Con{Name: "File"}
	Bytes = &Con{Name: "Bytes"}
)

// Err is the poison type: the type of a reference that could not be resolved. An error has
// already been reported for it, so it unifies with everything and never reaches a message.
var Err = &Con{Name: "<error>"}

// IsErr reports whether t is the poison type.
func IsErr(t Type) bool { c, ok := t.(*Con); return ok && c == Err }

func String(t Type) string { return str(t, false, printer{}) }

// StringIn prints t as written inside module mod: mod's own types are not qualified.
func StringIn(t Type, mod string) string { return str(t, false, printer{home: mod}) }

// StringQ prints t as written inside module mod, each other module through the qualifier q
// gives it (the importing module's canonical qualifiers, #23: `Post` for Table.Post); a module
// q gives "" for (one mod does not import) is written by its whole path. A record or union
// whose qualifier is its own name (the table `Post` of Table.Post) is written bare, as a
// program writes it.
func StringQ(t Type, mod string, q func(module string) string) string {
	return str(t, false, printer{home: mod, q: q})
}

// printer is how str writes a module: not at all for home, else through q when it gives a
// qualifier, else by its path.
type printer struct {
	home string
	q    func(module string) string
}

// qual is the qualifier module is written with ("" for home or no module).
func (p printer) qual(module string) string {
	if module == "" || module == p.home {
		return ""
	}
	if p.q != nil {
		if q := p.q(module); q != "" {
			return q
		}
	}
	return module
}

// qualify writes name with its module's qualifier, unless the module is home.
func (p printer) qualify(module, name string) string {
	if q := p.qual(module); q != "" {
		return q + "." + name
	}
	return name
}

// eponymous reports whether a record or union named name of module is written bare: its
// module is named after it, or (StringQ) is written with its name.
func (p printer) eponymous(module, name string) bool {
	return module == name || p.q != nil && p.qual(module) == name
}

func str(t Type, paren bool, home printer) string {
	switch t := t.(type) {
	case *Con:
		if t == Err {
			return "_"
		}
		return home.qualify(t.Module, t.Name)
	case *Var:
		if t.Name != "" {
			return t.Name
		}
		return "_" // an unsolved inference variable; its internal id never reaches a message
	case *Param:
		return t.Name
	case *Brand:
		return home.qualify(t.Module, t.Name)
	case *Record:
		if !t.Anonymous && t.Name != "" {
			s := t.Name
			if !home.eponymous(t.Module, t.Name) {
				s = home.qualify(t.Module, t.Name)
			}
			if len(t.Args) == 0 {
				return s
			}
			for _, a := range t.Args {
				s += " " + str(a, true, home)
			}
			if paren {
				return "(" + s + ")"
			}
			return s
		}
		var parts []string
		for _, f := range t.Fields {
			parts = append(parts, f.Name+": "+str(f.Type, false, home))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case *Union:
		s := t.Name
		if t.Module != "Prelude" && !home.eponymous(t.Module, t.Name) {
			s = home.qualify(t.Module, t.Name)
		}
		if len(t.Args) == 0 {
			return s
		}
		for _, a := range t.Args {
			s += " " + str(a, true, home)
		}
		if paren {
			return "(" + s + ")"
		}
		return s
	case *App:
		s := t.Con
		for _, a := range t.Args {
			s += " " + str(a, true, home)
		}
		if paren && len(t.Args) > 0 {
			return "(" + s + ")"
		}
		return s
	case *Fun:
		var parts []string
		for _, p := range t.Params {
			_, isFun := p.(*Fun)
			parts = append(parts, str(p, isFun, home))
		}
		s := strings.Join(parts, " -> ") + " -> " + str(t.Result, false, home)
		if paren {
			return "(" + s + ")"
		}
		return s
	}
	return "?"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// Constructors returns the declaration's constructors, even when this is an instance
// copied before the declaration's body was defined.
func (u *Union) Constructors() []Ctor {
	if u.Def != nil {
		return u.Def.Ctors
	}
	return u.Ctors
}

// WithArgs returns a copy of u carrying args and a back-pointer to the declaration.
func (u *Union) WithArgs(args []Type) *Union {
	def := u.Def
	if def == nil {
		def = u
	}
	return &Union{Module: u.Module, Name: u.Name, Args: args, Ctors: u.Ctors, Params: u.Params, Def: def}
}

// WithArgs returns the instance of a generic record's declaration at args: a copy carrying args,
// a back-pointer to the declaration, and the declaration's fields with its parameters replaced
// by args. A record that refers to itself (`type T a = { kids: List (T a) }`) gets a cyclic
// instance: inside the substitution, the same declaration at the same arguments is this
// instance. The fields are those the declaration has now: the checker refreshes instances made
// before every declaration was complete (see Refresh).
func (r *Record) WithArgs(args []Type) *Record {
	return instantiateRecord(r, args, map[string]*Record{})
}

// Refresh recomputes an instance's fields from its (now complete) declaration.
func (r *Record) Refresh() {
	if r.Def != nil {
		r.Fields = instantiateRecord(r.Def, r.Args, map[string]*Record{}).Fields
	}
}

func instantiateRecord(r *Record, args []Type, inProgress map[string]*Record) *Record {
	def := r.Def
	if def == nil {
		def = r
	}
	key := fmt.Sprintf("%p", def) + typeKey(&App{Con: "", Args: args})
	if inst, ok := inProgress[key]; ok {
		return inst
	}
	inst := &Record{Module: def.Module, Name: def.Name, Params: def.Params, Args: args, Def: def}
	inProgress[key] = inst
	if len(inProgress) > maxRecordNesting {
		// A record that refers to itself at growing arguments (`type T a = { more: List (T
		// (List a)) }`) has no finite instance; the checker rejects it (E0011). Stop here.
		return inst
	}
	idx := map[string]Type{}
	for i, n := range def.Params {
		if i < len(args) {
			idx[n] = args[i]
		}
	}
	for _, f := range def.Fields {
		inst.Fields = append(inst.Fields, Field{Name: f.Name, Type: subst(f.Type, idx, inProgress), HasDefault: f.HasDefault, Default: f.Default})
	}
	return inst
}

// maxRecordNesting bounds the generic record instances one instantiation builds.
const maxRecordNesting = 64

// typeKey is a string that identifies t up to equality, telling inference variables apart.
func typeKey(t Type) string {
	switch t := t.(type) {
	case *Var:
		return "?" + itoa(t.ID)
	case *Con:
		return t.Name
	case *Param:
		return "'" + t.Name
	case *Brand:
		return "#" + t.Module + "." + t.Name
	case *App:
		s := "(" + t.Con
		for _, a := range t.Args {
			s += " " + typeKey(a)
		}
		return s + ")"
	case *Union:
		s := "(" + t.Module + "." + t.Name
		for _, a := range t.Args {
			s += " " + typeKey(a)
		}
		return s + ")"
	case *Record:
		if t.Anonymous {
			s := "{"
			for _, f := range t.Fields {
				s += f.Name + ":" + typeKey(f.Type) + ","
			}
			return s + "}"
		}
		s := "(" + t.Module + "." + t.Name
		for _, a := range t.Args {
			s += " " + typeKey(a)
		}
		return s + ")"
	case *Fun:
		s := "(->"
		for _, p := range t.Params {
			s += " " + typeKey(p)
		}
		return s + " " + typeKey(t.Result) + ")"
	}
	return "?"
}
