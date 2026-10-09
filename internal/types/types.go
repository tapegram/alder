// internal/types/types.go
package types

import (
	"strings"
)

type Type interface{ isType() }

type Con struct{ Name string }
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
}
type Record struct {
	Module, Name string
	Fields       []Field
	Anonymous    bool
}
type Ctor struct {
	Name string
	Arg  Type
}
type Union struct {
	Module, Name string
	Args         []Type
	Ctors        []Ctor
	Def          *Union // declaration this instance was copied from (nil on the declaration itself)
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
	Int   = &Con{"Int"}
	Float = &Con{"Float"}
	Bool  = &Con{"Bool"}
	Text  = &Con{"Text"}
	Unit  = &Con{"Unit"}
	Date  = &Con{"Date"}
	Time  = &Con{"Time"}
	Never = &Con{"Never"}
	Error = &Con{"Error"}
	Page  = &Con{"Page"}
)

func String(t Type) string { return str(t, false) }

func str(t Type, paren bool) string {
	switch t := t.(type) {
	case *Con:
		return t.Name
	case *Var:
		if t.Name != "" {
			return t.Name
		}
		return "?" + itoa(t.ID)
	case *Param:
		return t.Name
	case *Brand:
		return t.Module + "." + t.Name
	case *Record:
		if !t.Anonymous && t.Name != "" {
			if t.Module != "" && t.Module != t.Name {
				return t.Module + "." + t.Name
			}
			return t.Name
		}
		var parts []string
		for _, f := range t.Fields {
			parts = append(parts, f.Name+": "+str(f.Type, false))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case *Union:
		s := t.Name
		if t.Module != "" && t.Module != "Prelude" && t.Module != t.Name {
			s = t.Module + "." + t.Name
		}
		if len(t.Args) == 0 {
			return s
		}
		for _, a := range t.Args {
			s += " " + str(a, true)
		}
		if paren {
			return "(" + s + ")"
		}
		return s
	case *App:
		s := t.Con
		for _, a := range t.Args {
			s += " " + str(a, true)
		}
		if paren && len(t.Args) > 0 {
			return "(" + s + ")"
		}
		return s
	case *Fun:
		var parts []string
		for _, p := range t.Params {
			_, isFun := p.(*Fun)
			parts = append(parts, str(p, isFun))
		}
		s := strings.Join(parts, " -> ") + " -> " + str(t.Result, false)
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
