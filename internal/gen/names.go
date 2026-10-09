package gen

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// GoName is the package-level Go name of top-level Alder name in module: the module path with
// '.' replaced by '_', then "__", then the name ("Todos.Index", "view" → "Todos_Index__view").
// The "__" keeps every generated name clear of Go keywords and of the runtime's package names.
func GoName(module, name string) string {
	return strings.ReplaceAll(module, ".", "_") + "__" + name
}

// fileName is the Go file a module is emitted to.
func fileName(module string) string { return strings.ReplaceAll(module, ".", "_") + ".go" }

// reserved are the identifiers a local Alder name must not become: Go keywords, Go's predeclared
// identifiers (the emitted code calls panic, len, int64, …) and the packages generated code
// imports.
var reserved = map[string]bool{}

func init() {
	for _, s := range strings.Fields(`
		break case chan const continue default defer else fallthrough for func go goto if import
		interface map package range return select struct switch type var
		any append bool byte cap clear close comparable complex complex64 complex128 copy delete
		error false float32 float64 imag int int8 int16 int32 int64 iota len make max min new nil
		panic print println real recover rune string true uint uint8 uint16 uint32 uint64 uintptr
		rt html std time math`) {
		reserved[s] = true
	}
}

// local is the Go name of a local Alder name: the name itself, with a trailing '_' when it is
// reserved ("type" → "type_", "rt" → "rt_").
func local(name string) string {
	if reserved[name] {
		return name + "_"
	}
	return name
}

// Field is the Go name of a record field: the Alder name with its first letter upper-cased
// ("title" → "Title"), so that it is exported and never a Go keyword.
func Field(name string) string {
	r, n := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[n:]
}

// typeParam is the Go name of an Alder type parameter ("a" → "A").
func typeParam(name string) string { return Field(name) }

// Union naming: for union M.Shape with constructor Circle,
//
//	tagType   M__ShapeTag          the tag's type
//	tagConst  M__Shape_Tag_Circle  the constructor's tag
//	payload   M__Shape_Circle      the payload struct (anonymous record payloads only)
//	ctorFunc  M__Shape_Circle_     the constructor function (payload ctors)
//	ctorVar   M__Shape_Circle      the constructor value (payload-less ctors of non-generic unions)
//	ctorField Circle               the union struct's field holding the payload ("Tag_" for Tag)
func tagType(module, union string) string { return GoName(module, union) + "Tag" }
func tagConst(module, union, ctor string) string {
	return GoName(module, union) + "_Tag_" + ctor
}
func payloadName(module, union, ctor string) string { return GoName(module, union) + "_" + ctor }
func ctorFunc(module, union, ctor string) string    { return payloadName(module, union, ctor) + "_" }
func ctorVar(module, union, ctor string) string     { return payloadName(module, union, ctor) }
func ctorField(ctor string) string {
	if ctor == "Tag" {
		return "Tag_"
	}
	return ctor
}

// stdName is the Go name of stdlib value Module.name: std.<Module><Name> ("List", "map" →
// "std.ListMap"). Task 6 replaces this mechanical rule with the stdlib call table.
func stdName(module, name string) string { return "std." + module + Field(name) }
