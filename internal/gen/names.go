package gen

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Naming scheme. Every package-level Go name the emitter writes is one of:
//
//	GoName      M__…__name                  a top-level Alder declaration of module M.… (value,
//	                                        component, record, union, table, form, and a page's
//	                                        or App's generated Params, Msg, Session, <Msg>Params)
//	ctorName    M__Union__Ctor__            a constructor: its function, or the value of one
//	                                        without a payload
//	payloadName payload__M__Union__Ctor     the struct of a constructor's anonymous record payload
//	tagType     tag__M__Union               a union's tag type
//	tagConst    is__M__Union__Ctor          a constructor's tag constant
//	widenName   widen__Page__Session        a `requires` page's session widened to App's
//
// where "M__…" is the module path with every '.' replaced by "__" ("Todos.Index", "view" →
// "Todos__Index__view"). A local is its Alder name, with "__" appended when that is reserved
// ("type" → "type__"), renamed "x_2", "x_3", … only to avoid capture (see scope), or a
// temporary "_a1", "_m2", …. Record fields are capitalized ("title" → "Title"); the union struct
// field holding constructor C's payload is C ("Tag__" for a constructor named Tag, which would
// otherwise be the tag field).
//
// Why no two Alder declarations get the same Go name (TestGoNameInjective checks it on a
// collision corpus; Emit also rejects a duplicate as defense in depth). Alder identifiers never
// contain "__" (the lexer rejects it, E0001); module segments start with an upper-case letter
// and never end with '_' (the parser rejects it, E0001); constructor, union and type names start
// with an upper-case letter.
//
//  1. GoName is injective. Read M__…__name from the left: a module segment contains no "__" and
//     does not end with '_', so the first "__" ends it; the piece after the last such split is
//     the name (it may start with '_', but contains no "__"). So the string determines the
//     segments and the name.
//  2. ctorName is GoName(M, Union) + "__" + Ctor + "__": it ends in "__", which no GoName does
//     (an identifier never ends in "__"). Stripping that "__", Ctor is the text after the last
//     "__" (Ctor has no "__" and does not start with '_'), and the rest is a GoName (1).
//  3. payloadName, tagType, tagConst and widenName start with a lower-case word and "__". No
//     GoName or ctorName does (they start with an upper-case module segment); no local does (a
//     local contains "__" only at its end, or followed by its "_N" renaming suffix); the four
//     words differ, so the kinds cannot collide with each other; within a kind the rest is
//     decoded as in 1 and 2.
//  4. Record fields and union fields live in their struct's own namespace: Field is injective
//     (only the first letter changes, and Alder fields start lower-case or with '_'), and union
//     fields are constructor names (upper-case) plus Tag and Tag__.
//
// The per-page generated types (Params, Msg, Session, <Msg>Params) are GoNames of names the
// checker reserves in page modules and App (E0012), so no user declaration can take them.

// GoName is the package-level Go name of top-level Alder name in module: the module's segments,
// then the name, joined by "__" ("Todos.Index", "view" → "Todos__Index__view").
func GoName(module, name string) string {
	return strings.ReplaceAll(module, ".", "__") + "__" + name
}

// fileName is the Go file a module is emitted to: its segments joined by "__" ("Todos.Index" →
// "Todos__Index.go"), as injective as GoName.
func fileName(module string) string { return strings.ReplaceAll(module, ".", "__") + ".go" }

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

// local is the Go name of a local Alder name: the name itself, with "__" appended when it is
// reserved ("type" → "type__", "rt" → "rt__").
func local(name string) string {
	if reserved[name] {
		return name + "__"
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

// tagType is the Go type of union M.Union's tag ("tag__M__Shape").
func tagType(module, union string) string { return "tag__" + GoName(module, union) }

// tagConst is the tag constant of constructor ctor ("is__M__Shape__Circle").
func tagConst(module, union, ctor string) string {
	return "is__" + GoName(module, union) + "__" + ctor
}

// payloadName is the struct of constructor ctor's anonymous record payload
// ("payload__M__Shape__Rect").
func payloadName(module, union, ctor string) string {
	return "payload__" + GoName(module, union) + "__" + ctor
}

// ctorName is constructor ctor: the function for one with a payload, the value for one without
// ("M__Shape__Rect__").
func ctorName(module, union, ctor string) string {
	return GoName(module, union) + "__" + ctor + "__"
}

// widenName is the function that widens page module page's narrowed Session to App's
// ("widen__Home__Session").
func widenName(page string) string { return "widen__" + GoName(page, "Session") }

// ctorField is the union struct's field holding constructor ctor's payload.
func ctorField(ctor string) string {
	if ctor == "Tag" {
		return "Tag__"
	}
	return ctor
}
