package rtesting

import (
	"alder/rt/eff"
	"alder/rt/fuzz"
)

// Arg is one argument of a property test: its name in the lambda (or of forAll's function),
// its type as the program writes it, and whether a type variable in it defaulted to Int (spec
// §4.4), which the report then shows.
type Arg struct {
	Name, Type string
	Defaulted  bool
}

// Property is a property test's body as the generated code builds it: Draw draws one value per
// argument from the choices, Lit prints each as an Alder literal, and exactly one of Pure and
// Task runs the body on them (Task for a block body or Fuzz.forAllTask). Case, when set, is the
// paste-ready plain test for the literals of a failing case.
type Property struct {
	Args []Arg
	Draw func(*fuzz.Choices) []any
	Lit  func([]any) []string
	Pure func([]any) bool
	Task func([]any) eff.Task[bool]
	Case func(lits []string) string
}

// ForAll is `Fuzz.forAll g f`: one argument drawn by g. The generated code fills in Args and
// Lit.
func ForAll[A any](g fuzz.Fuzzer[A], f func(A) bool) *Property {
	return &Property{
		Draw: func(c *fuzz.Choices) []any { return []any{g(c)} },
		Pure: func(vs []any) bool { return f(vs[0].(A)) },
	}
}

// ForAllTask is `Fuzz.forAllTask g f`, whose body is a Task.
func ForAllTask[A any](g fuzz.Fuzzer[A], f func(A) eff.Task[bool]) *Property {
	return &Property{
		Draw: func(c *fuzz.Choices) []any { return []any{g(c)} },
		Task: func(vs []any) eff.Task[bool] { return f(vs[0].(A)) },
	}
}
