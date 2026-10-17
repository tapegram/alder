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

// todosWithoutAdmin copies examples/todos without its generated admin (app/Admin/) and build/
// into a temporary directory and returns it.
func todosWithoutAdmin(t *testing.T) string {
	dir := t.TempDir()
	src := "../../examples/todos"
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			if rel == "build" || rel == filepath.Join("app", "Admin") {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestIR_Todos lowers the real example app: it must not panic (a missing recorded type is a
// checker bug), and its dump is reviewed by hand in testdata/golden/ir_todos.ir. The admin
// `alder new admin` generated in it (app/Admin/, book 15) is left out: it is the generator's
// output, tested by its own goldens, and its pages have no update of their own.
func TestIR_Todos(t *testing.T) {
	p := lowerDir(t, todosWithoutAdmin(t))
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
// (the todos example's scoped operations and plain writes, the blog's conditions), and the insert
// row's omitted default is the table field's own Default node.
func TestPositions(t *testing.T) {
	p := lowerDir(t, "../../examples/todos")
	module := func(p *Program, name string) *Module {
		for _, m := range p.Modules {
			if m.Name == name {
				return m
			}
		}
		t.Fatalf("no module %s", name)
		return nil
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
	index, show := module(p, "Todos.Index"), module(p, "Todos.Show")
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
			visit(x.L)
			visit(x.R)
		case *DbGet:
		case *DbSelect:
			got["select"] = append(got["select"], x.Pos)
			visit(x.Base)
			for _, w := range x.Where {
				if c, ok := w.(*Cond); ok { // not a variable holding one
					got["cond"] = append(got["cond"], c.Pos)
				}
			}
			for _, o := range x.Order {
				got["ord"] = append(got["ord"], o.(*Ord).Pos)
			}
			got["where-step"] = append(got["where-step"], x.WherePos...)
			got["order-step"] = append(got["order-step"], x.OrderPos...)
		case *DbScoped:
			got["mine"] = append(got["mine"], x.Pos)
		case *DbGetScoped:
			got["get-mine"] = append(got["get-mine"], x.Pos)
		case *DbInsertScoped:
			got["insert-mine"] = append(got["insert-mine"], x.Pos)
		case *DbUpdateScoped:
			got["update-mine"] = append(got["update-mine"], x.Pos)
			visit(x.Fn.(*Lambda).Body)
		case *DbDeleteScoped:
			got["delete-mine"] = append(got["delete-mine"], x.Pos)
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
	visit(value(index, "load").Body)
	visit(value(index, "update").Body)
	visit(value(show, "load").Body)
	visit(value(show, "update").Body)
	want := map[string][]Pos{
		"bind":          {{16, 3}, {35, 5}, {42, 5}, {46, 5}, {13, 3}, {32, 5}, {36, 5}},
		"match":         {{33, 28}, {30, 28}},
		"select":        {{18, 10}}, // the step that extends the Base
		"mine":          {{17, 5}},
		"where-step":    nil,
		"ord":           {{18, 21}},
		"order-step":    {{18, 10}},
		"insert-mine":   {{35, 14}},
		"update-mine":   {{42, 14}, {32, 14}},
		"record-update": {{42, 45}, {32, 54}},
		"delete-mine":   {{46, 14}, {36, 14}},
		"get-mine":      {{13, 15}},
		"ctor Redirect": {{36, 19}, {33, 19}, {37, 19}},
		"ctor Rerender": {{43, 18}, {47, 18}},
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
	if lit, ok := done.Default.(*Lit); !ok || lit.Pos != (Pos{14, 16}) || done.Pos != (Pos{14, 3}) {
		t.Fatalf("Todo.done: default %#v, field pos %v", done.Default, done.Pos)
	}
	ins := value(index, "update").Body.(*Match).Cases[0].Body.(*Let).Body.(*EffBind).Effect.(*DbInsertScoped)
	for _, f := range ins.Row.(*Record).Fields {
		if f.Name == "done" && f.Value != done.Default {
			t.Fatalf("the insert row's done is %#v, not the table's default node", f.Value)
		}
	}

	// the blog's front page: its `where` steps and conditions, its `if`s and operators
	blog := lowerDir(t, "../../examples/blog")
	got = map[string][]Pos{}
	visit(value(module(blog, "Posts.Index"), "load").Body)
	want = map[string][]Pos{
		"bind":       {{34, 3}},
		"if":         {{30, 14}, {30, 45}, {33, 5}},
		"op":         {{30, 17}, {30, 48}, {42, 18}},
		"select":     {{35, 5}},
		"cond":       {{36, 20}}, // the second `Db.where` takes a variable, `matching`
		"ord":        {{38, 21}},
		"where-step": {{36, 10}, {37, 10}},
		"order-step": {{38, 10}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("blog positions:\n got %v\nwant %v", got, want)
	}
}

// TestIR_Rules (M11): the corpus app rules-ok lowers its `@owner` column to Table.Owner, its
// role pages to Page.Roles, and the five scoped operations to their nodes: Db.mine a Select
// value (a DbSelect's Base, or a DbRun's Select through a helper), Db.insertMine's row without
// the owner column. Its dump is reviewed by hand in testdata/golden/ir_rules.ir.
func TestIR_Rules(t *testing.T) {
	p := lowerDir(t, "../../testdata/corpus/rules-ok")
	compareGolden(t, "../../testdata/golden/ir_rules.ir", Dump(p))
	for _, tb := range p.Tables {
		want := map[string]string{"Todo": "owner", "User": ""}[tb.Name]
		if tb.Owner() != want {
			t.Errorf("table %s: Owner %q, want %q", tb.Name, tb.Owner(), want)
		}
	}
	roles := map[string]string{}
	for _, pg := range p.Pages {
		roles[pg.Module] = strings.Join(pg.Roles, "|")
		if len(pg.Roles) > 0 && (strings.Join(pg.Requires, ",") != "user" || pg.RolesPos.Line == 0) {
			t.Errorf("page %s: requires %q, roles at %+v", pg.Module, pg.Requires, pg.RolesPos)
		}
	}
	if roles["Admin.Index"] != "Author|Admin" || roles["Admin.Users"] != "Admin" || roles["Todos.Index"] != "" {
		t.Errorf("roles: %v", roles)
	}
	seen := map[string]int{}
	var walk func(e Expr)
	walk = func(e Expr) {
		switch x := e.(type) {
		case *DbScoped:
			seen["mine"]++
		case *DbGetScoped:
			seen["getMine"]++
		case *DbUpdateScoped:
			seen["updateMine"]++
		case *DbDeleteScoped:
			seen["deleteMine"]++
		case *DbInsertScoped:
			seen["insertMine"]++
			rec := x.Row.(*Record)
			var names []string
			for _, f := range rec.Fields {
				names = append(names, f.Name)
			}
			if strings.Join(names, " ") != "title done" {
				t.Errorf("insertMine row: %v", names)
			}
		}
		for _, k := range Children(e) {
			if k != nil {
				walk(k)
			}
		}
	}
	for _, m := range p.Modules {
		for _, v := range m.Values {
			walk(v.Body)
		}
	}
	if fmt.Sprint(seen) != "map[deleteMine:1 getMine:2 insertMine:2 mine:2 updateMine:2]" {
		t.Errorf("scoped nodes: %v", seen)
	}
}

// TestIR_Deploy (M14): the corpus app deploy-ok lowers its `deploy` declaration to Program.Deploy:
// one Target per environment, the gate's fields beside them. Its dump is in testdata/golden/ir_deploy.ir.
func TestIR_Deploy(t *testing.T) {
	p := lowerDir(t, "../../testdata/corpus/deploy-ok")
	compareGolden(t, "../../testdata/golden/ir_deploy.ir", Dump(p))
	d := p.Deploy
	if d == nil {
		t.Fatal("no Deploy")
	}
	want := map[string]Target{
		"production": {Kind: "ssh", Host: "deploy@box.example.com", Domain: "blog.example.com", Path: "/srv/blog", User: "deploy", Port: "2222"},
		"staging":    {Kind: "fly", Region: "ord", App: "blog-staging"},
		"preview":    {Kind: "ssh", Host: "deploy@box.example.com", Domain: "*.preview.example.com", Path: "/srv/blog-preview"},
		"long":       {Kind: "fly", Region: "ord", App: "a-very-long-hyphenated-application-name-for-staging"},
		"local":      {Kind: "script", ScriptPath: "./scripts/deploy-local.sh"},
	}
	if len(d.Envs) != len(want) {
		t.Errorf("envs: %v", d.Envs)
	}
	for n, w := range want {
		if d.Envs[n] != w {
			t.Errorf("env %s: %+v, want %+v", n, d.Envs[n], w)
		}
	}
	if !d.FloorSet || d.Floor != 0.95 || d.Keep != 5 || d.Proxy != "None" || d.Backup == nil || d.Backup.URL != "s3://blog-backups/production" ||
		strings.Join(d.Requires, ",") != "SMTP_URL,STRIPE_KEY" || len(d.Checks) != 3 {
		t.Errorf("deploy: %+v", d)
	}
}
