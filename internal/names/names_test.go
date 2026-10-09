// internal/names/names_test.go
package names

import (
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/syntax"
)

func parse(t *testing.T, file, src string) *syntax.Module {
	m, ds := syntax.ParseFile(file, []byte(src))
	if len(ds) > 0 {
		t.Fatalf("parse %s: %+v", file, ds)
	}
	return m
}

func codes(b *diag.Bag) string {
	var s []string
	for _, d := range b.Items {
		s = append(s, d.Code+":"+d.Message)
	}
	return strings.Join(s, "\n")
}

var prelude = map[string]*syntax.Module{} // filled by Task 7; tests here use syntax.ParseSignatures directly

func std(t *testing.T) map[string]*syntax.Module {
	m, _ := syntax.ParseSignatures("Prelude.alder", []byte("module Prelude\ntype Int\ntype Text\ntype Bool = True | False\ntype List a\ntype Maybe a = Just a | Nothing\ntype Html a\ntype Query a\ntype Task a\ntype Nav = Rerender | Redirect Text\nnot : Bool -> Bool\n"))
	l, _ := syntax.ParseSignatures("List.alder", []byte("module List\nmap : (a -> b) -> List a -> List b\n"))
	q, _ := syntax.ParseSignatures("Query.alder", []byte("module Query\nsucceed : a -> Query a\n"))
	tk, _ := syntax.ParseSignatures("Task.alder", []byte("module Task\nsucceed : a -> Task a\n"))
	return map[string]*syntax.Module{"Prelude": m, "List": l, "Query": q, "Task": tk}
}

func TestResolve_Basics(t *testing.T) {
	todo := parse(t, "app/Todo.alder", "module Todo\n\ntype TodoId = brand Text\n\ntype Filter = All | Active\n\ntable Todo {\n  id: TodoId @id,\n  done: Bool = False,\n}\n")
	idx := parse(t, "app/Todos/Index.alder", "module Todos.Index\n\nimport Todo\n\npage \"/\" { filter: Todo.Filter = All }\n\ntype Model = { todos: List Todo }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { todos: [] }\n\nview : Model -> Session -> Html Msg\nview model session = <ul>{for t in model.todos -> <li>{Todo.filterLabel All}</li>}</ul>\n\nf : Todo.Filter -> Bool\nf x = match x { All -> True, Active -> not False }\n")
	prog, bag := Build([]*syntax.Module{todo, idx}, std(t))
	prog.Resolve(bag)
	for _, d := range bag.Items {
		if d.Severity == "error" && d.Code != "E0002" {
			t.Fatalf("unexpected: %s", codes(bag))
		}
	}
	// Todo.filterLabel is unknown: must be reported
	if !strings.Contains(codes(bag), "E0002") {
		t.Fatalf("expected E0002 for Todo.filterLabel, got: %s", codes(bag))
	}
}

func TestResolve_QualificationErrors(t *testing.T) {
	todo := parse(t, "app/Todo.alder", "module Todo\n\ntype Priority = Low | High\n")
	idx := parse(t, "app/Todos/Index.alder", "module Todos.Index\n\nimport Todo\n\npage \"/\" { }\n\ntype Model = { p: Priority }\n\nx : Todo.Priority\nx = Todo.Low\n\ny : User.UserId\ny = ?y\n")
	prog, bag := Build([]*syntax.Module{todo, idx}, std(t))
	prog.Resolve(bag)
	got := codes(bag)
	for _, want := range []string{"E0202:type `Priority` belongs to module `Todo`; write `Todo.Priority`", "E0203:constructors are not qualified; write `Low`", "E0201:module `User` is not imported"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	var fixes []string
	for _, d := range bag.Items {
		if d.Fix != nil {
			fixes = append(fixes, d.Fix.Replace)
		}
	}
	if strings.Join(fixes, "|") != "Todo.Priority|Low|import User\n" {
		t.Fatalf("fixes: %v", fixes)
	}
}

func TestResolve_ImportCycle(t *testing.T) {
	a := parse(t, "app/A.alder", "module A\n\nimport B\n\nx : Int\nx = 1\n")
	b := parse(t, "app/B.alder", "module B\n\nimport A\n\ny : Int\ny = A.x\n")
	c := parse(t, "app/C.alder", "module C\n\nz : Int\nz = 1\n")
	prog, bag := Build([]*syntax.Module{a, b, c}, std(t))
	prog.Resolve(bag)
	n := 0
	for _, d := range bag.Items {
		if d.Code == "E0006" {
			n++
			if !strings.Contains(d.Message, "A -> B -> A") && !strings.Contains(d.Message, "B -> A -> B") {
				t.Fatalf("cycle message: %s", d.Message)
			}
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly one E0006, got %d: %s", n, codes(bag))
	}
	if _, ok := prog.Modules["C"]; !ok || len(prog.Order) == 0 {
		t.Fatal("other modules must still be processed")
	}
}

func TestResolve_GlobalsAndModulePath(t *testing.T) {
	app := parse(t, "app/App.alder", "module App\n\nsession { user: Maybe Text }\n\nlogin = Auth.Login\n\nmsg Logout\n\nupdate : Msg -> Session -> Task Nav\nupdate msg session = Task.succeed Rerender\n")
	login := parse(t, "app/Auth/Login.alder", "module Auth.Login\n\npage \"/login\" { }\n\ntype Model = { }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { }\n\nview : Model -> Session -> Html Msg\nview model session = <a href={Auth.Login {}}><button click={App.Logout}>x</button></a>\n")
	wrong := parse(t, "app/Misc/Wrong.alder", "module Wrong\n\nx : Int\nx = 1\n")
	prog, bag := Build([]*syntax.Module{app, login, wrong}, std(t))
	prog.Resolve(bag)
	got := codes(bag)
	if !strings.Contains(got, "E0005") {
		t.Fatalf("expected E0005 for module/path mismatch: %s", got)
	}
	for _, d := range bag.Items {
		if d.Code != "E0005" && d.Severity == "error" {
			t.Fatalf("unexpected error: %s", codes(bag))
		}
	}
	if prog.AppMsgs["Logout"] == nil || prog.Pages["Auth.Login"] == nil {
		t.Fatal("globals not collected")
	}
}

func TestResolve_UnknownImport(t *testing.T) {
	m := parse(t, "app/X.alder", "module X\n\nimport Nope\n\nx : Int\nx = match Just 1 { Just n -> n, Nothing -> 0 }\n")
	prog, bag := Build([]*syntax.Module{m}, std(t))
	prog.Resolve(bag)
	n := 0
	for _, d := range bag.Items {
		if d.Code == "E0002" && strings.Contains(d.Message, "Nope") {
			n++
		}
	}
	if n != 1 || len(bag.Items) != 1 {
		t.Fatalf("expected exactly one E0002 for Nope, got: %s", codes(bag))
	}
}

func TestResolve_DeterministicOrder(t *testing.T) {
	for i := 0; i < 20; i++ {
		a := parse(t, "app/A.alder", "module A\n\nx : Int\nx = 1\n")
		b := parse(t, "app/B.alder", "module B\n\ny : Int\ny = 1\n")
		c := parse(t, "app/C.alder", "module C\n\nimport B\nimport A\n\nz : Int\nz = 1\n")
		prog, bag := Build([]*syntax.Module{a, b, c}, std(t))
		prog.Resolve(bag)
		if got := codes(bag); got != "W0001:import `B` is unused\nW0001:import `A` is unused" {
			t.Fatalf("run %d: %s", i, got)
		}
	}
}

func TestResolve_ModuleNameRuleBeatsPrelude(t *testing.T) {
	o := parse(t, "app/Order.alder", "module Order\n\ntype OrderId = brand Text\n\ntable Order {\n  id: OrderId @id,\n}\n")
	p := parse(t, "app/P.alder", "module P\n\nimport Order\n\nx : List Order\nx = []\n")
	prog, bag := Build([]*syntax.Module{o, p}, std(t))
	prog.Resolve(bag)
	if len(bag.Items) != 0 {
		t.Fatalf("unexpected: %s", codes(bag))
	}
	var vd *syntax.ValueDecl
	for _, d := range p.Decls {
		if v, ok := d.(*syntax.ValueDecl); ok {
			vd = v
		}
	}
	tn := vd.Sig.(*syntax.TypeName).Args[0].(*syntax.TypeName)
	want := syntax.Ref{Kind: syntax.RefType, Module: "Order", Name: "Order"}
	if tn.Ref != want {
		t.Fatalf("ref = %+v", tn.Ref)
	}
}

// `App` is global and never imported: no App-qualified name is reported as E0201.
func TestResolve_AppQualifierNeverE0201(t *testing.T) {
	app := parse(t, "app/App.alder", "module App\n\nmsg Logout\n")
	lay := parse(t, "app/Layout.alder", "module Layout\n\nPage : { children: List (Html App.Msg) } -> Html App.Msg\nPage props = <main>{props.children}</main>\n\nx : App.Session\nx = ?x\n\ny : App.Theme\ny = ?y\n\nz : Int\nz = App.login\n\nw : Int\nw = App.update\n")
	prog, bag := Build([]*syntax.Module{app, lay}, std(t))
	prog.Resolve(bag)
	got := codes(bag)
	if strings.Contains(got, "E0201") {
		t.Fatalf("App must never be reported as not imported:\n%s", got)
	}
	for _, want := range []string{
		"E0002:`Session` is not qualified; write `Session`",
		"E0002:`App` has no type `Theme` other modules can use; only `App.Msg` is global",
		"E0002:`App` has no `login`; `login` is a declaration, not a value — link to the page with `href={Auth.Login {}}`",
		"E0002:`App` has no `update`; only app-level messages such as `App.Logout` can be used from other modules",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != 4 {
		t.Errorf("want exactly 4 diagnostics (App.Msg resolves), got %d:\n%s", n, got)
	}
}

func TestMarkFailed(t *testing.T) {
	a := parse(t, "app/A.alder", "module A\n\nx : Int\nx = nope\n")
	b := parse(t, "app/B.alder", "module B\n\ny : Int\ny = 1\n")
	prog, bag := Build([]*syntax.Module{a, b}, std(t))
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	if !prog.Failed["A"] || prog.Failed["B"] {
		t.Fatalf("Failed = %v", prog.Failed)
	}
}

// Module names that differ only in case are E0014: their generated files (and source files)
// would be one file on a case-insensitive file system.
func TestBuild_ModuleCaseCollision(t *testing.T) {
	a := parse(t, "app/Ab.alder", "module Ab\n\nx : Int\nx = 1\n")
	b := parse(t, "app/Shop/AB.alder", "module Shop.AB\n\ny : Int\ny = 1\n")
	c := parse(t, "app/AB.alder", "module AB\n\nz : Int\nz = 1\n")
	_, bag := Build([]*syntax.Module{a, b, c}, std(t))
	var got []string
	for _, d := range bag.Items {
		got = append(got, d.Code+" "+d.Message)
	}
	want := "E0014 module names `Ab` and `AB` differ only in case (app/Ab.alder, app/AB.alder); rename one of them"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %q, want [%q]", got, want)
	}
}

// A module named Route, Cap or Args, or under one of them, is E0015: the generated exported
// names Route__…, Cap__… and Args__… would be its GoNames.
func TestBuild_ReservedModuleNames(t *testing.T) {
	var mods []*syntax.Module
	for _, n := range []string{"Route", "Cap.Mail", "Args", "Routes", "Capx.Y", "My.Cap"} {
		mods = append(mods, parse(t, "app/"+strings.ReplaceAll(n, ".", "/")+".alder", "module "+n+"\n\nx : Int\nx = 1\n"))
	}
	_, bag := Build(mods, std(t))
	var got []string
	for _, d := range bag.Items {
		got = append(got, d.Code+" "+d.Message)
	}
	want := []string{
		"E0015 module name `Route` is reserved: the compiled program's names `Route__…` are taken; rename the module",
		"E0015 module name `Cap.Mail` is reserved: the compiled program's names `Cap__…` are taken; rename the module",
		"E0015 module name `Args` is reserved: the compiled program's names `Args__…` are taken; rename the module",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// The import fix lands right after the module line also when comments come before it, and
// applying it gives a module that resolves.
func TestResolve_ImportFixAfterLeadingComment(t *testing.T) {
	user := parse(t, "app/User.alder", "module User\n\ntype UserId = brand Text\n")
	src := "-- The account page.\n-- Second comment line.\nmodule Account\n\ny : User.UserId\ny = ?y\n"
	acct := parse(t, "app/Account.alder", src)
	if acct.Span.Start.Line != 3 || acct.Span.Start.Col != 1 {
		t.Fatalf("module span starts at %d:%d, want 3:1", acct.Span.Start.Line, acct.Span.Start.Col)
	}
	prog, bag := Build([]*syntax.Module{user, acct}, std(t))
	prog.Resolve(bag)
	var fix *diag.Fix
	for _, d := range bag.Items {
		if d.Code == "E0201" {
			fix = d.Fix
		}
	}
	if fix == nil {
		t.Fatalf("no E0201 fix: %s", codes(bag))
	}
	if fix.Line != 4 || fix.Col != 1 {
		t.Fatalf("import fix at %d:%d, want 4:1 (after the module line)", fix.Line, fix.Col)
	}
	fixed, err := diag.Apply(src, []diag.Fix{*fix})
	if err != nil {
		t.Fatal(err)
	}
	if want := "-- The account page.\n-- Second comment line.\nmodule Account\nimport User\n\ny : User.UserId\ny = ?y\n"; fixed != want {
		t.Fatalf("fixed source:\n%s", fixed)
	}
	prog, bag = Build([]*syntax.Module{user, parse(t, "app/Account.alder", fixed)}, std(t))
	prog.Resolve(bag)
	for _, d := range bag.Items {
		if d.Code == "E0201" {
			t.Fatalf("still not imported after the fix: %s", codes(bag))
		}
	}
}
