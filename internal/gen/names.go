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
//	tableName   table__M__Table             a table's descriptor (*db.Table)
//	insertName  insert__M__Table            a table's insert record type (Db.insert's row)
//	insertVals  insertvals__M__Table        the column values of an insert record
//	enumName    name__M__Union              the constructor name of a payload-less union value
//	                                        stored in a table column
//	checkName   check__M__Brand             a brand's `where` check (func(v Base) bool)
//	decodeName  decode__M__Record           the decoder of a page's Params, a message's
//	                                        <Msg>Params or a form (routes__alder.go)
//	pageFunc    page__M                     page M's `requires` check and load (routes)
//	paramsFunc  params__M                   page M's parameters as text for message URLs
//	updateFunc  update__M                   page M's (or App's) update and its Nav (routes)
//	routeFunc   Route__M__page              page M's GET route (routes)
//	            Route__M__Msg               message Msg of page M or of App (M = App)
//	capName     Cap__M__Capability          a capability's Go interface (exported: the adapters
//	                                        package implements it)
//	argsName    Args__M__Capability__Entry  the arguments record of a capability entry
//	route__fail, route__values, route__errors, session__codec
//	                                        fixed helpers of routes__alder.go and app__alder.go
//	Tables, Capabilities, Caps, Options, Serve, Routes
//	                                        the app's table descriptors, capability keys,
//	                                        capability implementations, Serve and its options,
//	                                        and its routes (app__alder.go, routes__alder.go)
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
//  3. payloadName, tagType, tagConst, widenName, tableName, insertName, insertVals, enumName,
//     checkName, decodeName, pageFunc, paramsFunc, updateFunc and the route__/session__ helpers start with a
//     lower-case word and "__". No GoName or ctorName does (they start with an upper-case module
//     segment); no local does (a local contains "__" only at its end, or followed by its "_N"
//     renaming suffix); the words differ, so the kinds cannot collide with each other; within a
//     kind the rest is decoded as in 1 and 2 (decodeName's record is a page's Params, a
//     <Msg>Params or a form of M: type names of one module, distinct). Tables, Capabilities,
//     Caps, Options, Serve and Routes contain no "__" and start upper-case, which no GoName (it
//     has "__"), no local (lower-case or '_') and none of the others is. app__alder.go and
//     routes__alder.go are no module's file, even ignoring case (on a case-insensitive file
//     system such as macOS's, app_alder.go would be App's file): they would be files of a
//     module segment "app_" or "routes_", which never ends a segment.
//  4. Record fields and union fields live in their struct's own namespace: Field is injective
//     (only the first letter changes, and Alder fields start lower-case or with '_'), and union
//     fields are constructor names (upper-case) plus Tag and Tag__.
//  5. routeFunc, capName and argsName are exported (the adapters package and cmd/main.go use
//     them), so they start with an upper-case word: Route, Cap or Args, then "__". They are
//     GoNames of a module whose first segment is Route, Cap or Args, so no module may be named
//     so (E0015, names.ReservedModules); then no GoName or ctorName starts with those
//     words, and within a kind the rest decodes as in 1: Route__M__page (a page is a module, so
//     M names it and "page" is lower-case, no message), Route__M__Msg (M a page module or App,
//     Msg upper-case), Cap__M__C, Args__M__C__Entry. In the last, Entry never starts with '_'
//     (the entry capitalized, or "x" + an entry starting with '_') and contains no "__", while C
//     may end with '_': so Entry is the text after the last "__", and the rest decodes as in 1
//     (C is its name).
//
// The per-page generated types (Params, Msg, Session, <Msg>Params) are GoNames of names the
// checker reserves in page modules and App (E0012), so no user declaration can take them. Two
// modules whose names differ only in case (their files would be one on a case-insensitive file
// system) are rejected by the names graph (E0014).

// GoName is the package-level Go name of top-level Alder name in module: the module's segments,
// then the name, joined by "__" ("Todos.Index", "view" → "Todos__Index__view").
func GoName(module, name string) string {
	return strings.ReplaceAll(module, ".", "__") + "__" + name
}

// fileName is the Go file a module is emitted to: its segments joined by "__", then "_alder.go"
// ("Todos.Index" → "Todos__Index_alder.go"), injective as the suffix is fixed. The suffix keeps
// every module an ordinary file of the package whatever its name: Go treats a file ending in
// "_test.go" as a test, and one whose name up to its first '.' ends in "_<GOOS>", "_<GOARCH>" or
// "_<GOOS>_<GOARCH>" (optionally followed by "_test") as built only there, so a module named
// Foo_test or Bar_linux needs a last "_" element that is neither; "alder" is not. (A double
// extension, "Bar_linux.alder.go", would not do: Go cuts the name at the first '.'.)
func fileName(module string) string { return strings.ReplaceAll(module, ".", "__") + "_alder.go" }

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
		rt html std time math eff db context fmt url web http json errors strings reflect`) {
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

// tableName is table M.T's descriptor ("table__Todo__Todo").
func tableName(module, table string) string { return "table__" + GoName(module, table) }

// insertName is table M.T's insert record type ("insert__Todo__Todo").
func insertName(module, table string) string { return "insert__" + GoName(module, table) }

// insertVals is the function giving an insert record's column values ("insertvals__Todo__Todo").
func insertVals(module, table string) string { return "insertvals__" + GoName(module, table) }

// capName is capability M.C's interface ("Cap__Auth__Login__Password").
func capName(module, capability string) string { return "Cap__" + GoName(module, capability) }

// argsName is the arguments record of entry e of capability M.C
// ("Args__Auth__Login__Password__Verify"): the entry capitalized, or prefixed with "x" when it
// starts with '_' ("_raw" → "x_raw"), so that it never starts with '_' (naming rule 5).
func argsName(module, capability, entry string) string {
	e := Field(entry)
	if strings.HasPrefix(entry, "_") {
		e = "x" + entry
	}
	return "Args__" + GoName(module, capability) + "__" + e
}

// checkName is brand M.B's `where` check ("check__Todo__Slug").
func checkName(module, brand string) string { return "check__" + GoName(module, brand) }

// decodeName is the decoder of record R of module M: a page's Params, a message's <Msg>Params
// or a form ("decode__Todos__Index__NewTodo").
func decodeName(module, record string) string { return "decode__" + GoName(module, record) }

// pageFunc runs page M's requires check and load ("page__Todos__Index").
func pageFunc(module string) string { return "page__" + strings.ReplaceAll(module, ".", "__") }

// paramsFunc renders page M's parameters for its message URLs ("params__Todos__Index").
func paramsFunc(module string) string { return "params__" + strings.ReplaceAll(module, ".", "__") }

// updateFunc runs page M's update, or App's, and answers its Nav ("update__Todos__Index").
func updateFunc(module string) string { return "update__" + strings.ReplaceAll(module, ".", "__") }

// routeFunc is the route of page M (msg "") or of its message msg ("Route__Todos__Index__page",
// "Route__Todos__Index__Toggle", "Route__App__Logout").
func routeFunc(module, msg string) string {
	if msg == "" {
		msg = "page"
	}
	return "Route__" + GoName(module, msg)
}

// enumName is the function naming a value of payload-less union M.U ("name__Todo__Filter").
func enumName(module, union string) string { return "name__" + GoName(module, union) }

// appFile holds the app-wide declarations: tablesVar (table descriptors), capsVar (capability
// keys), and with an App module Caps, Options and Serve. routesFile holds the routes.
const (
	appFile    = "app__alder.go"
	routesFile = "routes__alder.go"
	tablesVar  = "Tables"
	capsVar    = "Capabilities"
)
