package gen

import (
	"strings"
	"sync"

	"alder/internal/syntax"
	"alder/internal/types"
)

// StdCalls maps every stdlib function emitted as a call, named "Module.name" as in its stdlib
// file, to the Go function of alder/rt/std that implements it: the pure ones, and the effectful
// ones that stay calls in the IR (Session.put, Log.*, Random.*, Time.now, Date.today, Http.*),
// whose Go functions build effect values. A stdlib constant (Html.none, Time.epoch, Dict.empty,
// Random.id, Time.now) is a Go function of no parameters, called at each use. Emitting a stdlib
// reference that is not here is an internal error of gen (TestStdCallsCoverStdlib keeps the
// table complete).
var StdCalls = map[string]string{
	// Bool
	"Bool.toText": "std.BoolToText",
	// Date
	"Date.fromText": "std.DateFromText",
	"Date.toText":   "std.DateToText",
	"Date.format":   "std.DateFormat",
	"Date.addDays":  "std.DateAddDays",
	"Date.diffDays": "std.DateDiffDays",
	"Date.year":     "std.DateYear",
	"Date.month":    "std.DateMonth",
	"Date.day":      "std.DateDay",
	// Dict
	"Dict.empty":    "std.DictEmpty",
	"Dict.fromList": "std.DictFromList",
	"Dict.toList":   "std.DictToList",
	"Dict.get":      "std.DictGet",
	"Dict.insert":   "std.DictInsert",
	"Dict.remove":   "std.DictRemove",
	"Dict.keys":     "std.DictKeys",
	"Dict.values":   "std.DictValues",
	"Dict.size":     "std.DictSize",
	// Float
	"Float.round":  "std.FloatRound",
	"Float.floor":  "std.FloatFloor",
	"Float.ceil":   "std.FloatCeil",
	"Float.min":    "std.FloatMin",
	"Float.max":    "std.FloatMax",
	"Float.abs":    "std.FloatAbs",
	"Float.format": "std.FloatFormat",
	// Html
	"Html.unsafe": "std.HtmlUnsafe",
	"Html.none":   "std.HtmlNone",
	"Html.text":   "std.HtmlText",
	// Int
	"Int.toFloat": "std.IntToFloat",
	"Int.min":     "std.IntMin",
	"Int.max":     "std.IntMax",
	"Int.abs":     "std.IntAbs",
	"Int.clamp":   "std.IntClamp",
	// List
	"List.length":     "std.ListLength",
	"List.isEmpty":    "std.ListIsEmpty",
	"List.head":       "std.ListHead",
	"List.last":       "std.ListLast",
	"List.take":       "std.ListTake",
	"List.drop":       "std.ListDrop",
	"List.reverse":    "std.ListReverse",
	"List.map":        "std.ListMap",
	"List.filter":     "std.ListFilter",
	"List.filterMap":  "std.ListFilterMap",
	"List.find":       "std.ListFind",
	"List.any":        "std.ListAny",
	"List.all":        "std.ListAll",
	"List.member":     "std.ListMember",
	"List.foldl":      "std.ListFoldl",
	"List.sum":        "std.ListSum",
	"List.sumFloat":   "std.ListSumFloat",
	"List.sortBy":     "std.ListSortBy",
	"List.sortByText": "std.ListSortByText",
	"List.indexed":    "std.ListIndexed",
	"List.range":      "std.ListRange",
	"List.concat":     "std.ListConcat",
	"List.append":     "std.ListAppend",
	"List.unique":     "std.ListUnique",
	"List.groupBy":    "std.ListGroupBy",
	// Maybe
	"Maybe.map":         "std.MaybeMap",
	"Maybe.andThen":     "std.MaybeAndThen",
	"Maybe.withDefault": "std.MaybeWithDefault",
	"Maybe.isJust":      "std.MaybeIsJust",
	"Maybe.toList":      "std.MaybeToList",
	// Prelude
	"Prelude.not": "std.PreludeNot",
	// Result
	"Result.map":         "std.ResultMap",
	"Result.mapErr":      "std.ResultMapErr",
	"Result.andThen":     "std.ResultAndThen",
	"Result.withDefault": "std.ResultWithDefault",
	"Result.toMaybe":     "std.ResultToMaybe",
	// Text
	"Text.length":     "std.TextLength",
	"Text.isEmpty":    "std.TextIsEmpty",
	"Text.trim":       "std.TextTrim",
	"Text.toUpper":    "std.TextToUpper",
	"Text.toLower":    "std.TextToLower",
	"Text.contains":   "std.TextContains",
	"Text.startsWith": "std.TextStartsWith",
	"Text.endsWith":   "std.TextEndsWith",
	"Text.replace":    "std.TextReplace",
	"Text.split":      "std.TextSplit",
	"Text.join":       "std.TextJoin",
	"Text.slice":      "std.TextSlice",
	"Text.matches":    "std.TextMatches",
	"Text.fromInt":    "std.TextFromInt",
	"Text.fromFloat":  "std.TextFromFloat",
	"Text.toInt":      "std.TextToInt",
	"Text.toFloat":    "std.TextToFloat",
	// Time
	"Time.epoch":  "std.TimeEpoch",
	"Time.toDate": "std.TimeToDate",
	"Time.format": "std.TimeFormat",
	"Time.ago":    "std.TimeAgo",
	// Notice
	"Notice.with": "std.NavNoticed",
	// Url
	"Url.external": "std.UrlExternal",
	// effects
	"Session.put": "std.SessionPut",
	"Log.info":    "std.LogInfo",
	"Log.warn":    "std.LogWarn",
	"Random.int":  "std.RandomInt",
	"Random.id":   "std.RandomId",
	"Time.now":    "std.TimeNow",
	"Date.today":  "std.DateToday",
	"Http.get":    "std.HttpGet",
	"Http.post":   "std.HttpPost",
	// Test's queries (test builds only: the checker allows Test.* only in a `test`)
	"Test.text":  "rtesting.DocText",
	"Test.texts": "rtesting.DocTexts",
	"Test.has":   "rtesting.DocHas",
	"Test.count": "rtesting.DocCount",
	"Test.attr":  "rtesting.DocAttr",
	"Test.doc":   "rtesting.ViewDoc",
}

// Effectful lists the stdlib modules and functions that have no StdCalls entry: Query.*, Task.*,
// Db.* and Test's run, trace, fake and browser steps, which Lower turns into effect, database
// and test nodes (effects.go and browser.go emit them). A key is a module name (all of its
// functions) or "Module.name".
var Effectful = map[string]bool{
	"Query": true, "Task": true, "Db": true,
	"Test.run": true, "Test.trace": true, "Test.fake": true, "Test.login": true, "Test.get": true,
	"Test.open": true, "Test.submit": true, "Test.submitRaw": true, "Test.click": true,
	"Test.member": true, // a call gen writes itself (member, browser.go): its table is the `via` field's
}

func isEffectful(module, name string) bool {
	return Effectful[module] || Effectful[module+"."+name]
}

// stdGoName is the Go function implementing stdlib function module.name.
func stdGoName(module, name string) string {
	g, ok := StdCalls[module+"."+name]
	if !ok {
		panic("gen: stdlib function " + module + "." + name + " has no Go implementation (StdCalls)")
	}
	return g
}

// stdVarParams records, per stdlib function, which parameters are a bare type variable
// (`withDefault : a -> Maybe a -> a`): Go infers such a parameter's type argument from the
// argument, so an untyped integer constant passed there must be typed int64.
var stdVarParams = sync.OnceValue(func() map[string][]bool {
	out := map[string][]bool{}
	std, err := types.LoadStd()
	if err != nil {
		panic("gen: loading stdlib signatures: " + err.Error())
	}
	for _, m := range std {
		for _, d := range m.Decls {
			v, ok := d.(*syntax.ValueDecl)
			if !ok {
				continue
			}
			ft, ok := v.Sig.(*syntax.FunType)
			if !ok {
				continue
			}
			var vs []bool
			for _, p := range ft.Params {
				_, isVar := p.(*syntax.TypeVar)
				vs = append(vs, isVar)
			}
			out[m.Name+"."+v.Name] = vs
		}
	}
	return out
})

// stdVarParam reports whether parameter i of stdlib function module.name is a bare type variable.
func stdVarParam(module, name string, i int) bool {
	vs := stdVarParams()[module+"."+name]
	return i < len(vs) && vs[i]
}

// stdSigs is every stdlib function's declared signature, by "Module.name".
var stdSigs = sync.OnceValue(func() map[string]syntax.Type {
	out := map[string]syntax.Type{}
	std, err := types.LoadStd()
	if err != nil {
		panic("gen: loading stdlib signatures: " + err.Error())
	}
	for _, m := range std {
		for _, d := range m.Decls {
			if v, ok := d.(*syntax.ValueDecl); ok && v.Sig != nil {
				out[m.Name+"."+v.Name] = v.Sig
			}
		}
	}
	return out
})

// stdTypeParamOrder overrides, for the stdlib functions whose Go type parameters are not in the
// order their type variables first occur in the Alder signature, the Go order (as Alder type
// variable names). "Session" stands for the app's Session type (Session.put's Go S).
// TestStdTypeParamOrder checks every generic function of rt/std against stdTypeParams.
var stdTypeParamOrder = map[string][]string{
	"Result.map":         {"e", "a", "b"},
	"Result.andThen":     {"e", "a", "b"},
	"Result.withDefault": {"e", "a"},
	"Session.put":        {"Session"},
}

// stdTypeParams lists the type parameters of the Go function implementing stdlib function
// module.name, in Go order, as the Alder type variables they stand for: by default the
// signature's type variables in order of first occurrence, leaving out one that occurs only
// inside Html (Html a is html.Node whatever a is).
func stdTypeParams(module, name string) []string {
	key := module + "." + name
	if ps, ok := stdTypeParamOrder[key]; ok {
		return ps
	}
	var ps []string
	seen := map[string]bool{}
	var walk func(t syntax.Type)
	walk = func(t syntax.Type) {
		switch t := t.(type) {
		case *syntax.TypeVar:
			if !seen[t.Name] {
				seen[t.Name] = true
				ps = append(ps, t.Name)
			}
		case *syntax.TypeName:
			if t.Name == "Html" && (t.Module == "" || t.Module == "Html") {
				return
			}
			for _, a := range t.Args {
				walk(a)
			}
		case *syntax.RecordType:
			for _, f := range t.Fields {
				walk(f.Type)
			}
		case *syntax.FunType:
			for _, p := range t.Params {
				walk(p)
			}
			walk(t.Result)
		}
	}
	walk(stdSigs()[key])
	return ps
}

// stdInstance is the explicit type argument list ("[int64, string]") of a reference to generic
// stdlib function module.name at type t (its solved type at this use), for a use that does not
// call it (`let r = List.reverse`): Go cannot infer type arguments there. A type variable left
// unsolved is `any`. A function that is not generic gets "".
func stdInstance(module, name string, t types.Type) string {
	ps := stdTypeParams(module, name)
	if len(ps) == 0 {
		return ""
	}
	m := map[string]types.Type{}
	bindSig(stdSigs()[module+"."+name], t, m)
	args := make([]string, len(ps))
	for i, p := range ps {
		if a := m[p]; a != nil {
			args[i] = GoType(a)
		} else {
			args[i] = "any"
		}
	}
	return "[" + strings.Join(args, ", ") + "]"
}

// bindSig matches the declared stdlib signature sig against its instance inst, recording what
// each type variable (and `Session`) stands for.
func bindSig(sig syntax.Type, inst types.Type, m map[string]types.Type) {
	switch d := sig.(type) {
	case *syntax.TypeVar:
		if m[d.Name] == nil {
			m[d.Name] = inst
		}
	case *syntax.TypeName:
		if d.Name == "Session" && d.Module == "" && len(d.Args) == 0 {
			if m["Session"] == nil {
				m["Session"] = inst
			}
			return
		}
		var args []types.Type
		switch i := inst.(type) {
		case *types.App:
			args = i.Args
		case *types.Union:
			args = i.Args
		case *types.Record:
			args = i.Args
		}
		if len(args) == len(d.Args) {
			for k := range d.Args {
				bindSig(d.Args[k], args[k], m)
			}
		}
	case *syntax.RecordType:
		if i, ok := inst.(*types.Record); ok {
			for _, f := range d.Fields {
				for _, g := range i.Fields {
					if g.Name == f.Name {
						bindSig(f.Type, g.Type, m)
					}
				}
			}
		}
	case *syntax.FunType:
		if i, ok := inst.(*types.Fun); ok && len(i.Params) == len(d.Params) {
			for k := range d.Params {
				bindSig(d.Params[k], i.Params[k], m)
			}
			bindSig(d.Result, i.Result, m)
		}
	}
}
