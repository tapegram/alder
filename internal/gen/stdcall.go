package gen

import (
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
}

// Effectful lists the stdlib modules and functions that have no StdCalls entry: Query.*, Task.*
// and Db.*, which Lower turns into effect and database nodes (effects.go emits them), and
// Form.values, bound to a request (M3c). A key is a module name (all of its functions) or
// "Module.name".
var Effectful = map[string]bool{
	"Query": true, "Task": true, "Db": true, "Form.values": true,
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
