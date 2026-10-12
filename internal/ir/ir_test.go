package ir

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

var update = flag.Bool("update", false, "rewrite golden files")

// lowerDir loads, resolves and checks dir, requires it to be free of errors, and lowers it.
func lowerDir(t *testing.T, dir string) *Program {
	t.Helper()
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := names.Load(dir, std)
	for _, e := range ioErrs {
		t.Fatal(e)
	}
	if prog.ParseFailed {
		t.Fatalf("%s does not parse:\n%s", dir, errors(bag))
	}
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	res := check.Program(prog, bag)
	if bag.HasErrors() {
		t.Fatalf("%s does not check:\n%s", dir, errors(bag))
	}
	return Lower(prog, res)
}

func errors(bag *diag.Bag) string {
	var sb strings.Builder
	for _, d := range bag.Items {
		if d.Severity == "error" {
			fmt.Fprintf(&sb, "%s:%d:%d %s %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message)
		}
	}
	return sb.String()
}

// compareGolden checks got against the golden file, or rewrites it under -update.
func compareGolden(t *testing.T, golden, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update)", golden)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}

func TestIRGolden(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/ir_*")
	n := 0
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		n++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			compareGolden(t, dir+".ir", Dump(lowerDir(t, dir)))
		})
	}
	if n == 0 {
		t.Fatal("no IR golden directories")
	}
}

// TestIR_Todos lowers the real example app: it must not panic (a missing recorded type is a
// checker bug), and its dump is reviewed by hand in testdata/golden/ir_todos.ir.
func TestIR_Todos(t *testing.T) {
	p := lowerDir(t, "../../examples/todos")
	got := Dump(p)
	compareGolden(t, "../../testdata/golden/ir_todos.ir", got)
	for _, want := range []string{"(value Todos.Index.view ", "(component Todos.Index.Item ", "(component Layout.Page ",
		"(value Todos.Index.load ", "(value Todos.Index.update ", "(value App.update ", "(app-update App.update)"} {
		if !strings.Contains(got, want) {
			t.Errorf("todos dump lacks %q", want)
		}
	}
	if strings.Contains(got, "(skipped-effect") {
		t.Errorf("todos dump still skips an effect")
	}
	for _, pg := range p.Pages {
		if pg.View == nil || pg.Load == nil || pg.Update == nil {
			t.Errorf("page %s: view %v, load %v, update %v", pg.Module, pg.View != nil, pg.Load != nil, pg.Update != nil)
		}
		if pg.Load != nil && (!pg.Load.IsEffect || pg.Load.Effect != EffQuery) {
			t.Errorf("page %s: load is not a Query effect", pg.Module)
		}
		if pg.Update != nil && (!pg.Update.IsEffect || pg.Update.Effect != EffTask) {
			t.Errorf("page %s: update is not a Task effect", pg.Module)
		}
	}
	if p.AppUpdate == nil || !p.AppUpdate.IsEffect || p.AppUpdate.Effect != EffTask {
		t.Errorf("App.update: %+v; want a Task effect", p.AppUpdate)
	}
}

// TestIR_NoSkipped: nothing is skipped any more; every value of every module of every IR
// golden, the gen corpus and the todos app is lowered (Lower panics on a shape it cannot lower).
func TestIR_NoSkipped(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/ir_*")
	vet, _ := filepath.Glob("../../testdata/golden/gen_vet/*")
	dirs = append(append(dirs, vet...), "../../examples/todos")
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			p := lowerDir(t, dir)
			if d := Dump(p); strings.Contains(d, "skipped") {
				t.Errorf("dump mentions a skipped value")
			}
			for _, m := range p.Modules {
				for _, v := range m.Values {
					if v.Body == nil {
						t.Errorf("%s.%s has no body", v.Module, v.Name)
					}
				}
			}
		})
	}
}

// TestIR_NoBrands walks every node, type and map of the lowered todos app and of every IR golden
// (brands appear in tables, messages, page parameters, the session and values) and requires that
// none survives.
func TestIR_NoBrands(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/ir_*")
	dirs = append(dirs, "../../examples/todos")
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) { noBrands(t, lowerDir(t, dir)) })
	}
	if !reflect.TypeFor[*types.Brand]().Implements(reflect.TypeFor[types.Type]()) {
		t.Fatal("types.Brand is no longer a types.Type; update this test")
	}
}

func noBrands(t *testing.T, p *Program) {
	seen := map[uintptr]bool{}
	var walk func(v reflect.Value, path string)
	walk = func(v reflect.Value, path string) {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() || seen[v.Pointer()] {
				return
			}
			seen[v.Pointer()] = true
			if _, isBrand := v.Interface().(*types.Brand); isBrand {
				t.Errorf("brand left in the IR at %s", path)
				return
			}
			walk(v.Elem(), path)
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem(), path)
			}
		case reflect.Struct:
			for i := range v.NumField() {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i), path+"."+v.Type().Field(i).Name)
				}
			}
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				walk(v.MapIndex(k), fmt.Sprintf("%s[%v]", path, k))
			}
		}
	}
	walk(reflect.ValueOf(p), "Program")
}

// TestIR_CtorPayloadShared: a record literal given to a constructor, directly or through `|>`,
// is a Record of the constructor's own payload RecordType, and a message payload omits `form`.
func TestIR_CtorPayloadShared(t *testing.T) {
	p := lowerDir(t, "../../testdata/golden/ir_patterns")
	var boxed *Value
	for _, v := range p.Modules[0].Values {
		if v.Name == "boxed" || v.Name == "nodeOf" {
			c := v.Body.(*Ctor)
			rec, ok := c.Payload.(*Record)
			if !ok || rec.T != c.U.ctorT(c.Name).Payload {
				t.Errorf("%s: payload is %T with RecordType %p, want the ctor's %p", v.Name, c.Payload, rec.T, c.U.ctorT(c.Name).Payload)
			}
			if v.Name == "boxed" {
				boxed = v
			}
		}
	}
	if boxed == nil {
		t.Fatal("ir_patterns has no `boxed`")
	}
	pg := lowerDir(t, "../../testdata/golden/ir_page")
	var save *Record
	for _, m := range pg.Modules {
		for _, v := range m.Values {
			if v.Module == "Edit" && v.Name == "view" {
				form := v.Body.(*Elem).Children[2].(*FormEl)
				save = form.Attrs[0].Value.(*Ctor).Payload.(*Record)
			}
		}
	}
	if save == nil || len(save.Fields) != 1 || save.Fields[0].Name != "n" || save.T != pg.Pages[0].Msgs[0].Params {
		t.Fatalf("Save payload: %+v; want only field n, of the message's Params record", save)
	}
}

// Lower refuses the poison type with an internal error naming the place (I3): it must only see
// programs that check without errors. The checker never records poison in a clean program, so
// the test plants it.
func TestLower_RefusesPoison(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "M.alder"), []byte("module M\n\nx : List Int\nx = [1]\n"), 0o644)
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, _ := names.Load(dir, std)
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	res := check.Program(prog, bag)
	if bag.HasErrors() {
		t.Fatal(errors(bag))
	}
	for e, ty := range res.Types {
		if _, ok := ty.(*types.App); ok {
			res.Types[e] = &types.App{Con: "List", Args: []types.Type{types.Err}}
		}
	}
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(fmt.Sprint(r), "poison type reached Lower at app/M.alder:4:5") {
			t.Fatalf("Lower did not refuse the poison type: %v", r)
		}
	}()
	Lower(prog, res)
}

// A table used as a value (passed to a function taking `Table Order`) lowers to a TableRef, and a
// table's attributes are kept: @created, @updated, @unique and @ref (to a table of another
// module).
func TestIR_TableRefAndAttrs(t *testing.T) {
	p := lowerDir(t, "../../testdata/golden/gen_vet/m3b-effects")
	d := Dump(p)
	for _, want := range []string{
		"(call (global Shop.countIn) (table Order.Order))",
		"(created created)", "(updated updated)", "(unique code)", "(ref buyer Customer.Customer)",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("dump lacks %q:\n%s", want, d)
		}
	}
	var tb *Table
	for _, x := range p.Tables {
		if x.Name == "Order" {
			tb = x
		}
	}
	if tb == nil || tb.Created != "created" || tb.Updated != "updated" || len(tb.Unique) != 1 || tb.Refs["buyer"] != "Customer.Customer" {
		t.Errorf("table Order: %+v", tb)
	}
}

// A delete rule other than the default @restrict is kept per @ref field, and `.startsWith` is a
// Cond of kind startsWith.
func TestIR_DeleteRulesAndStartsWith(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "Post.alder"), []byte(`module Post

type PostId = brand Text

table Post {
  id: PostId @id,
  title: Text,
}

titled : Text -> Query (List Post)
titled p = Db.from Post |> Db.where (Post.title.startsWith p) |> Db.all
`), 0o644)
	os.WriteFile(filepath.Join(dir, "app", "Comment.alder"), []byte(`module Comment

import Post

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  reply: Maybe Post.PostId @ref Post @setNull,
  quote: Post.PostId @ref Post @restrict,
}
`), 0o644)
	p := lowerDir(t, dir)
	d := Dump(p)
	for _, want := range []string{"(ref post Post.Post cascade)", "(ref reply Post.Post setNull)", "(ref quote Post.Post)", "startsWith"} {
		if !strings.Contains(d, want) {
			t.Errorf("dump lacks %q:\n%s", want, d)
		}
	}
	for _, tb := range p.Tables {
		if tb.Name == "Comment" && (len(tb.OnDelete) != 2 || tb.OnDelete["post"] != "cascade" || tb.OnDelete["reply"] != "setNull") {
			t.Errorf("Comment.OnDelete = %v", tb.OnDelete)
		}
	}
}

// A module's `test` declarations lower to Module.Tests in declaration order: a pure test's
// top-level `==` and an effect test's final `Task.succeed (… == …)` become TestEq, and
// Test.run / Test.trace their own nodes.
func TestIR_Tests(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	src := `module M

countOpen : List Bool -> Int
countOpen xs = List.length (List.filter (\x -> not x) xs)

test "a" = countOpen [] == 0

test "b" = {
  let r <- Test.run (Task.succeed 1)
  let tr <- Test.trace
  let n = List.length tr
  Task.succeed (r.value == Ok n)
}

test "c" = not (1 == 2)
`
	os.WriteFile(filepath.Join(dir, "app", "M.alder"), []byte(src), 0o644)
	p := lowerDir(t, dir)
	m := p.Modules[len(p.Modules)-1]
	if len(m.Tests) != 3 || m.Tests[0].Name != "a" || m.Tests[0].Effect || !m.Tests[1].Effect || m.Tests[1].Module != "M" {
		t.Fatalf("tests: %+v", m.Tests)
	}
	if _, ok := m.Tests[2].Body.(*TestEq); ok {
		t.Errorf("only a top-level == is a TestEq")
	}
	d := Dump(p)
	for _, want := range []string{
		`(test "a" pure (test-eq (call (global M.countOpen) (list)) 0))`,
		`(test "b" task`, `(test-run (pure task 1))`, `(test-trace)`, "(pure task\n          (test-eq\n            (field value (var r))",
		`(test "c" pure (call (std Prelude.not) (== : Int 1 2)))`,
	} {
		if !strings.Contains(d, want) {
			t.Errorf("dump lacks %q:\n%s", want, d)
		}
	}
}

// A table's `@renamed` old names and its fields' literal defaults, as SQL, reach the IR.
func TestIR_RenamedAndDefaultLit(t *testing.T) {
	p := lowerDir(t, "../../testdata/corpus/renamed-attrs-ok")
	var tb *Table
	for _, x := range p.Tables {
		if x.Name == "Post" {
			tb = x
		}
	}
	if tb == nil || tb.RenamedFrom != "Article" {
		t.Fatalf("table: %+v", tb)
	}
	var got []string
	for _, f := range tb.Record.Fields {
		s := f.Name
		if f.DefaultLit != nil {
			s += "=" + *f.DefaultLit
		}
		if f.RenamedFrom != "" {
			s += "<" + f.RenamedFrom
		}
		got = append(got, s)
	}
	want := "id title<heading count=0<total ratio=-0.5 shown=1 role='Reader' note=NULL level='Admin' motto='it''s {done}'"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
}

func TestSQLLiteral(t *testing.T) {
	for _, c := range []struct {
		e    syntax.Expr
		want string
	}{
		{&syntax.FloatLit{Value: 2}, "2.0"},
		{&syntax.FloatLit{Value: 1e21}, "1e+21"},
		{&syntax.IntLit{Value: -7}, "-7"},
		{&syntax.BoolLit{Value: false}, "0"},
		{&syntax.StrLit{Parts: []syntax.StrPart{{Text: "a\nb'"}}}, "'a' || char(10) || 'b'''"},
		{&syntax.StrLit{Parts: []syntax.StrPart{{Text: ""}}}, "''"},
		{&syntax.StrLit{Parts: []syntax.StrPart{{Text: "a\tb\x1b"}}}, "'a' || char(9) || 'b' || char(27) || ''"},
		{&syntax.App{Fn: &syntax.CtorRef{Name: "Just"}, Args: []syntax.Expr{&syntax.IntLit{Value: 3}}}, "3"},
	} {
		if got, ok := SQLLiteral(c.e); !ok || got != c.want {
			t.Errorf("SQLLiteral(%T) = %q, %v; want %q", c.e, got, ok, c.want)
		}
	}
	if _, ok := SQLLiteral(&syntax.Var{Name: "x"}); ok {
		t.Error("a variable is not a literal")
	}
}

// A fake lowers to Program.Fakes: its tables stay out of Program.Tables and out of its
// module's Types, its entries follow the capability's entry order (whatever the source order),
// and `Test.fake` is a TestFake pointing at it.
func TestIR_Fakes(t *testing.T) {
	p := lowerDir(t, "../../testdata/golden/ir_fakes")
	for _, tb := range p.Tables {
		if tb.Name == "Charge" {
			t.Fatalf("a fake's table is in Program.Tables")
		}
	}
	for _, m := range p.Modules {
		for _, td := range m.Types {
			if td.Name == "Charge" {
				t.Fatalf("a fake's table record is in %s's Types", m.Name)
			}
		}
	}
	var f *Fake
	var mods []string
	for _, x := range p.Fakes {
		mods = append(mods, x.Module)
		if x.Module == "Payments" {
			f = x
		}
	}
	if len(p.Fakes) != 2 || f == nil { // module order (dependencies first)
		t.Fatalf("fakes of modules %v", mods)
	}
	var cap *Capability
	for _, c := range p.Caps {
		if c.Name == "Stripe" {
			cap = c
		}
	}
	if f.Capability == nil || f.Capability != cap {
		t.Fatalf("capability: %+v", f.Capability)
	}
	if len(f.Tables) != 1 || f.Tables[0].Name != "Charge" || f.Tables[0].Refs["user"] != "User.User" || f.Tables[0].Record == nil {
		t.Fatalf("tables: %+v", f.Tables)
	}
	var got []string
	for i, e := range f.Entries {
		got = append(got, e.Name)
		if e.Name != f.Capability.Entries[i].Name || !reflect.DeepEqual(e.Type, f.Capability.Entries[i].Type) || !e.IsEffect || e.Body == nil {
			t.Errorf("entry %d: %+v", i, e)
		}
	}
	if strings.Join(got, " ") != "charge refund balance" {
		t.Fatalf("entries %v", got)
	}
	var fakes []*TestFake
	for _, m := range p.Modules {
		for _, ts := range m.Tests {
			walkTestFakes(ts.Body, &fakes)
		}
	}
	if len(fakes) != 5 || fakes[0].Fake != f {
		t.Fatalf("test fakes: %d", len(fakes))
	}
	if tf := fakes[0]; types.String(tf.Type()) != "Task Unit" {
		t.Fatalf("Test.fake type %s", types.String(tf.Type()))
	}
}

// walkTestFakes collects the TestFake nodes of a test body's effect chain (and of Test.run's task).
func walkTestFakes(e Expr, out *[]*TestFake) {
	switch x := e.(type) {
	case *TestFake:
		*out = append(*out, x)
	case *EffBind:
		walkTestFakes(x.Effect, out)
		walkTestFakes(x.Body, out)
	case *Let:
		walkTestFakes(x.Value, out)
		walkTestFakes(x.Body, out)
	case *TestRun:
		walkTestFakes(x.Task, out)
	case *Match:
		for _, c := range x.Cases {
			walkTestFakes(c.Body, out)
		}
	}
}

// TestPositions: the node kinds a mutant targets carry the line and column of their source
// (todos example), and the insert row's omitted default is the table field's own Default node.
func TestPositions(t *testing.T) {
	p := lowerDir(t, "../../examples/todos")
	var index *Module
	for _, m := range p.Modules {
		if m.Name == "Todos.Index" {
			index = m
		}
	}
	value := func(m *Module, name string) *Value {
		for _, v := range m.Values {
			if v.Name == name {
				return v
			}
		}
		t.Fatalf("no value %s.%s", m.Name, name)
		return nil
	}
	got := map[string][]Pos{}
	var visit func(e Expr)
	visit = func(e Expr) {
		switch x := e.(type) {
		case nil:
		case *EffBind:
			got["bind"] = append(got["bind"], x.Pos)
			visit(x.Effect)
			visit(x.Body)
		case *Let:
			visit(x.Value)
			visit(x.Body)
		case *Match:
			got["match"] = append(got["match"], x.Pos)
			visit(x.Subject)
			for _, c := range x.Cases {
				visit(c.Body)
			}
			visit(x.Default)
		case *If:
			got["if"] = append(got["if"], x.Pos)
			visit(x.Cond)
			visit(x.Then)
			visit(x.Else)
		case *Op:
			got["op"] = append(got["op"], x.Pos)
		case *DbGet:
		case *DbSelect:
			got["select"] = append(got["select"], x.Pos)
			for _, w := range x.Where {
				got["cond"] = append(got["cond"], w.(*Cond).Pos)
			}
			for _, o := range x.Order {
				got["ord"] = append(got["ord"], o.(*Ord).Pos)
			}
			got["where-step"] = append(got["where-step"], x.WherePos...)
			got["order-step"] = append(got["order-step"], x.OrderPos...)
		case *DbInsert:
			got["insert"] = append(got["insert"], x.Pos)
		case *DbUpdate:
			got["update"] = append(got["update"], x.Pos)
			visit(x.Fn.(*Lambda).Body)
		case *DbDelete:
			got["delete"] = append(got["delete"], x.Pos)
		case *Update:
			got["record-update"] = append(got["record-update"], x.Pos)
		case *EffPure:
			if c, ok := x.Value.(*Ctor); ok {
				got["ctor "+c.Name] = append(got["ctor "+c.Name], c.Pos)
			}
		}
	}
	visit(value(index, "owned").Body)
	visit(value(index, "load").Body)
	visit(value(index, "update").Body)
	want := map[string][]Pos{
		"bind":          {{56, 3}, {16, 3}, {36, 5}, {40, 5}, {41, 5}, {45, 5}, {46, 5}},
		"match":         {{57, 3}, {34, 28}},
		"if":            {{58, 18}},
		"op":            {{58, 21}},
		"select":        {{17, 5}},
		"cond":          {{18, 20}},
		"ord":           {{19, 21}},
		"where-step":    {{18, 10}},
		"order-step":    {{19, 10}},
		"insert":        {{36, 14}},
		"update":        {{41, 14}},
		"record-update": {{41, 44}},
		"delete":        {{46, 14}},
		"ctor Redirect": {{37, 19}},
		"ctor Rerender": {{42, 18}, {47, 18}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("positions:\n got %v\nwant %v", got, want)
	}
	for _, pg := range p.Pages {
		if pg.Module == "Todos.Index" && pg.RequiresPos != (Pos{7, 40}) {
			t.Fatalf("RequiresPos = %v", pg.RequiresPos)
		}
	}
	// the default of Todo.done: a Lit at its declaration, shared by the insert row that omits it
	var done FieldT
	for _, f := range p.Tables[1].Record.Fields {
		if f.Name == "done" {
			done = f
		}
	}
	if lit, ok := done.Default.(*Lit); !ok || lit.Pos != (Pos{12, 16}) || done.Pos != (Pos{12, 3}) {
		t.Fatalf("Todo.done: default %#v, field pos %v", done.Default, done.Pos)
	}
	ins := value(index, "update").Body.(*Match).Cases[0].Body.(*Let).Body.(*EffBind).Effect.(*DbInsert)
	for _, f := range ins.Row.(*Record).Fields {
		if f.Name == "done" && f.Value != done.Default {
			t.Fatalf("the insert row's done is %#v, not the table's default node", f.Value)
		}
	}
}
