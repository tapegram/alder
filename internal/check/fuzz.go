package check

import (
	"alder/internal/syntax"
	"alder/internal/types"
)

// fuzzProperty is `Fuzz.Property`, the type of a property test's body.
var fuzzProperty = &types.Con{Name: "Property", Module: "Fuzz"}

// fuzzForAllHead returns the `Fuzz.forAll` or `Fuzz.forAllTask` reference at the head of e: e is an
// application of it, or a `|>` pipeline whose last stage is one. nil when e is neither.
func fuzzForAllHead(e syntax.Expr) *syntax.Var {
	switch x := e.(type) {
	case *syntax.Var:
		if isFuzzProperty(x) {
			return x
		}
	case *syntax.App:
		return fuzzForAllHead(x.Fn)
	case *syntax.BinOp:
		if x.Op == "|>" {
			return fuzzForAllHead(x.R)
		}
	}
	return nil
}

func isFuzzProperty(v *syntax.Var) bool {
	return v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Fuzz" && (v.Ref.Name == "forAll" || v.Ref.Name == "forAllTask")
}

// isFuzzForAll reports whether e is an App (or the last `|>` stage) whose head resolves to
// `Fuzz.forAll` or `Fuzz.forAllTask`.
func isFuzzForAll(e syntax.Expr) bool { return fuzzForAllHead(e) != nil }
