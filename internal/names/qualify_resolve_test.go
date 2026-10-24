// internal/names/qualify_resolve_test.go
package names

import (
	"reflect"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// refs collects the Ref of every qualified-or-not reference in m, keyed by its written name
// (an element's as `<Name>`); a name written twice keeps its last Ref.
func refs(m *syntax.Module) map[string]syntax.Ref {
	out := map[string]syntax.Ref{}
	join := func(mod, name string) string {
		if mod == "" {
			return name
		}
		return mod + "." + name
	}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if v.IsNil() {
				return
			}
			switch n := v.Interface().(type) {
			case *syntax.Var:
				out[join(n.Module, n.Name)] = n.Ref
			case *syntax.CtorRef:
				out[join(n.Module, n.Name)] = n.Ref
			case *syntax.TypeName:
				out[join(n.Module, n.Name)] = n.Ref
			case *syntax.Element:
				if isUpper(n.Tag) {
					out["<"+join(n.Module, n.Tag)+">"] = n.Ref
				}
			}
			walk(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i))
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(m))
	return out
}

func resolveMods(t *testing.T, s map[string]*syntax.Module, srcs ...string) (*Program, *diag.Bag, map[string]*syntax.Module) {
	t.Helper()
	var mods []*syntax.Module
	byName := map[string]*syntax.Module{}
	for _, src := range srcs {
		name := strings.TrimPrefix(strings.SplitN(src, "\n", 2)[0], "module ")
		m := parse(t, "app/"+strings.ReplaceAll(name, ".", "/")+".alder", src)
		mods = append(mods, m)
		byName[name] = m
	}
	prog, bag := Build(mods, s)
	prog.Resolve(bag)
	return prog, bag, byName
}

func wantRef(t *testing.T, m *syntax.Module, written string, want syntax.Ref) {
	t.Helper()
	got, ok := refs(m)[written]
	if !ok {
		t.Errorf("%s: no reference written %q", m.Name, written)
	} else if got != want {
		t.Errorf("%s: %s resolved to %+v, want %+v", m.Name, written, got, want)
	}
}

const tableTodoSrc = "module Table.Todo\n\ntype Todo = { title: Text }\n\ntype Status = Open | Done\n\nfind : Int -> Todo\nfind n = ?find\n\nCard : { t: Todo } -> Html a\nCard props = <p>x</p>\n"

// With `import Table.Todo` the module's members, types and components are written through its
// qualifier `Todo`, and its eponymous type `Todo` by the qualifier alone; each Ref keeps the path.
func TestQual_ShortForm(t *testing.T) {
	_, bag, ms := resolveMods(t, std(t), tableTodoSrc,
		"module Use\n\nimport Table.Todo\n\nx : Todo -> Todo.Status\nx t = ?x\n\ny : Todo\ny = Todo.find 1\n\nz : Html a\nz = <Todo.Card t={y} />\n")
	if len(bag.Items) != 0 {
		t.Fatalf("unexpected: %s", codes(bag))
	}
	use := ms["Use"]
	wantRef(t, use, "Todo", syntax.Ref{Kind: syntax.RefType, Module: "Table.Todo", Name: "Todo"})
	wantRef(t, use, "Todo.Status", syntax.Ref{Kind: syntax.RefType, Module: "Table.Todo", Name: "Status"})
	wantRef(t, use, "Todo.find", syntax.Ref{Kind: syntax.RefValue, Module: "Table.Todo", Name: "find"})
	wantRef(t, use, "<Todo.Card>", syntax.Ref{Kind: syntax.RefComponent, Module: "Table.Todo", Name: "Card"})
}

// A reference spelling the imported module's full path is E0205 with a fix to the qualifier, and
// still resolves, so nothing else is reported.
func TestQual_FullPathIsE0205(t *testing.T) {
	_, bag, ms := resolveMods(t, std(t), tableTodoSrc,
		"module Use\n\nimport Table.Todo\n\ny : Todo\ny = Table.Todo.find 1\n")
	if len(bag.Items) != 1 || bag.Items[0].Code != "E0205" {
		t.Fatalf("want one E0205, got: %s", codes(bag))
	}
	d := bag.Items[0]
	if !strings.Contains(d.Message, "write `Todo.find`, not `Table.Todo.find`") {
		t.Errorf("message %q", d.Message)
	}
	if d.Fix == nil || d.Fix.Replace != "Todo.find" {
		t.Fatalf("fix %+v", d.Fix)
	}
	wantRef(t, ms["Use"], "Table.Todo.find", syntax.Ref{Kind: syntax.RefValue, Module: "Table.Todo", Name: "find"})
}

// Every full-path spelling (eponymous type, member type, value, element with its closing tag)
// gets a fix; applied together they give a module that resolves cleanly.
func TestQual_FullPathFixesApply(t *testing.T) {
	src := "module Use\n\nimport Table.Todo\n\nx : Table.Todo -> Table.Todo.Status\nx t = ?x\n\ny : Table.Todo\ny = Table.Todo.find 1\n\nz : Html a\nz = <Table.Todo.Card t={y}>hi</Table.Todo.Card>\n"
	_, bag, _ := resolveMods(t, std(t), tableTodoSrc, src)
	var fixes []diag.Fix
	for _, d := range bag.Items {
		if d.Code != "E0205" || d.Fix == nil {
			t.Fatalf("want only E0205s with fixes, got: %s", codes(bag))
		}
		fixes = append(fixes, *d.Fix)
	}
	if len(fixes) != 6 {
		t.Fatalf("want 6 E0205s, got: %s", codes(bag))
	}
	fixed, err := diag.Apply(src, fixes)
	if err != nil {
		t.Fatal(err)
	}
	want := "module Use\n\nimport Table.Todo\n\nx : Todo -> Todo.Status\nx t = ?x\n\ny : Todo\ny = Todo.find 1\n\nz : Html a\nz = <Todo.Card t={y}>hi</Todo.Card>\n"
	if fixed != want {
		t.Fatalf("fixed:\n%s", fixed)
	}
	_, bag, _ = resolveMods(t, std(t), tableTodoSrc, fixed)
	if len(bag.Items) != 0 {
		t.Fatalf("after the fixes: %s", codes(bag))
	}
}

const layoutSrc = "module Layout\n\nx : Int\nx = 1\n\nPage : { t: Int } -> Html a\nPage props = <p>x</p>\n"
const adminLayoutSrc = "module Admin.Layout\n\nx : Int\nx = 2\n\nPage : { t: Int } -> Html a\nPage props = <p>y</p>\n"

func onlyFix(t *testing.T, bag *diag.Bag, code string) *diag.Diagnostic {
	t.Helper()
	if len(bag.Items) != 1 || bag.Items[0].Code != code {
		t.Fatalf("want one %s, got: %s", code, codes(bag))
	}
	return &bag.Items[0]
}

// A missing, redundant or different `as` is E0204 with a fix to the canonical import line; the
// references are already keyed by the canonical qualifier, so nothing else is reported.
func TestQual_AliasRules(t *testing.T) {
	src := "module Use\n\nimport Layout\nimport Admin.Layout\n\na : Int\na = Layout.x\n\nb : Int\nb = AdminLayout.x\n"
	_, bag, ms := resolveMods(t, std(t), layoutSrc, adminLayoutSrc, src)
	d := onlyFix(t, bag, "E0204")
	if d.Fix == nil || d.Fix.Replace != "import Admin.Layout as AdminLayout" {
		t.Fatalf("missing alias: %s, fix %+v", d.Message, d.Fix)
	}
	if !strings.Contains(d.Message, "`as AdminLayout`") {
		t.Errorf("missing alias message %q", d.Message)
	}
	fixed, err := diag.Apply(src, []diag.Fix{*d.Fix})
	if err != nil || !strings.Contains(fixed, "\nimport Admin.Layout as AdminLayout\n\na : Int") {
		t.Fatalf("fixed (%v):\n%s", err, fixed)
	}
	wantRef(t, ms["Use"], "AdminLayout.x", syntax.Ref{Kind: syntax.RefValue, Module: "Admin.Layout", Name: "x"})
	wantRef(t, ms["Use"], "Layout.x", syntax.Ref{Kind: syntax.RefValue, Module: "Layout", Name: "x"})

	_, bag, _ = resolveMods(t, std(t), tableTodoSrc, "module Use\n\nimport Table.Todo as Todo\n\ny : Todo\ny = ?y\n")
	d = onlyFix(t, bag, "E0204")
	if d.Fix == nil || d.Fix.Replace != "import Table.Todo" {
		t.Fatalf("redundant alias: %s, fix %+v", d.Message, d.Fix)
	}

	_, bag, _ = resolveMods(t, std(t), layoutSrc, adminLayoutSrc, "module Use\n\nimport Admin.Layout as AL\nimport Layout\n\na : Int\na = Layout.x\n\nb : Int\nb = AdminLayout.x\n")
	d = onlyFix(t, bag, "E0204")
	if d.Fix == nil || d.Fix.Replace != "import Admin.Layout as AdminLayout" {
		t.Fatalf("different alias: %s, fix %+v", d.Message, d.Fix)
	}
}

// Module Sites.MnSos cannot name Data.MnSos by its own last segment: the qualifier is DataMnSos.
func TestQual_SelfCollision(t *testing.T) {
	prog, bag, ms := resolveMods(t, std(t), "module Data.MnSos\n\nx : Int\nx = 1\n",
		"module Sites.MnSos\n\nimport Data.MnSos as DataMnSos\n\ny : Int\ny = DataMnSos.x\n")
	if len(bag.Items) != 0 {
		t.Fatalf("unexpected: %s", codes(bag))
	}
	wantRef(t, ms["Sites.MnSos"], "DataMnSos.x", syntax.Ref{Kind: syntax.RefValue, Module: "Data.MnSos", Name: "x"})
	if q := prog.Qualifiers("Sites.MnSos"); !reflect.DeepEqual(q, map[string]string{"Data.MnSos": "DataMnSos"}) {
		t.Errorf("Qualifiers = %v", q)
	}
}

// A table declared in the importing module takes its name: `import Table.Todo` must be aliased
// TableTodo, and the eponymous type is then written TableTodo.
func TestQual_LocalTableTakesName(t *testing.T) {
	_, bag, ms := resolveMods(t, std(t), "module Table.Todo\n\ntype Todo = { n: Int }\n\nn : Int\nn = 1\n",
		"module Todos\n\nimport Table.Todo\n\ntable Todo {\n  title: Text,\n}\n\ny : Int\ny = TableTodo.n\n\nz : TableTodo -> Todo\nz t = ?z\n")
	d := onlyFix(t, bag, "E0204")
	if d.Fix == nil || d.Fix.Replace != "import Table.Todo as TableTodo" {
		t.Fatalf("%s, fix %+v", d.Message, d.Fix)
	}
	wantRef(t, ms["Todos"], "TableTodo.n", syntax.Ref{Kind: syntax.RefValue, Module: "Table.Todo", Name: "n"})
	wantRef(t, ms["Todos"], "TableTodo", syntax.Ref{Kind: syntax.RefType, Module: "Table.Todo", Name: "Todo"})
	wantRef(t, ms["Todos"], "Todo", syntax.Ref{Kind: syntax.RefType, Module: "Todos", Name: "Todo"})
}

// A.X, B.X and AX together: A.X and AX both end up as `AX`, so neither has a qualifier (E0206,
// no fix, no unused-import warning); B.X is BX.
func TestQual_Unqualifiable(t *testing.T) {
	_, bag, _ := resolveMods(t, std(t), "module A.X\n\nv : Int\nv = 1\n", "module B.X\n\nv : Int\nv = 1\n", "module AX\n\nv : Int\nv = 1\n",
		"module Use\n\nimport A.X\nimport B.X as BX\nimport AX\n\ny : Int\ny = BX.v\n")
	var got []string
	for _, d := range bag.Items {
		if d.Code != "E0206" || d.Fix != nil {
			t.Fatalf("want only E0206s without fixes, got: %s", codes(bag))
		}
		got = append(got, d.Message)
	}
	if len(got) != 2 || !strings.Contains(got[0], "`A.X`") || !strings.Contains(got[1], "`AX`") {
		t.Fatalf("got: %s", codes(bag))
	}
}

// Page values and App messages keep the full path, imported or not, and are never E0205; a
// member of an imported page module goes through its qualifier.
func TestQual_PagesStayFull(t *testing.T) {
	_, bag, ms := resolveMods(t, std(t),
		"module App\n\nmsg Logout\n",
		"module Todos.Index\n\npage \"/todos\" { }\n\nhelper : Int\nhelper = 1\n",
		"module Admin.User.Index\n\npage \"/admin/users\" { }\n",
		"module Use\n\nimport Todos.Index\n\na : Int\na = Index.helper\n\nv : Html App.Msg\nv = <div><a href={Todos.Index {}}>x</a><a href={Admin.User.Index {}}>y</a><button click={App.Logout}>z</button></div>\n")
	if len(bag.Items) != 0 {
		t.Fatalf("unexpected: %s", codes(bag))
	}
	use := ms["Use"]
	wantRef(t, use, "Index.helper", syntax.Ref{Kind: syntax.RefValue, Module: "Todos.Index", Name: "helper"})
	wantRef(t, use, "Todos.Index", syntax.Ref{Kind: syntax.RefPage, Module: "Todos.Index"})
	wantRef(t, use, "Admin.User.Index", syntax.Ref{Kind: syntax.RefPage, Module: "Admin.User.Index"})
	wantRef(t, use, "App.Logout", syntax.Ref{Kind: syntax.RefCtor, Module: "App", Name: "Logout"})
}

// An imported module's capability is written after its qualifier (`B.Clock.now`, and
// `Test.fake B.Clock`); with the full path it is E0205.
func TestQual_CapabilityAndFake(t *testing.T) {
	ab := "module A.B\n\ncapability Clock {\n  now : Query Int,\n}\n"
	_, bag, ms := resolveMods(t, testStd(t), ab,
		"module Use\n\nimport A.B\n\nx : Query Int\nx = B.Clock.now\n\ntest \"t\" = {\n  let _ <- Test.fake B.Clock\n  Task.succeed True\n}\n")
	if len(bag.Items) != 0 {
		t.Fatalf("unexpected: %s", codes(bag))
	}
	wantRef(t, ms["Use"], "B.Clock.now", syntax.Ref{Kind: syntax.RefCapability, Module: "A.B", Name: "Clock.now"})
	wantRef(t, ms["Use"], "B.Clock", syntax.Ref{Kind: syntax.RefCapability, Module: "A.B", Name: "A.B.Clock"})

	_, bag, ms = resolveMods(t, testStd(t), ab, "module Use\n\nimport A.B\n\nx : Query Int\nx = A.B.Clock.now\n")
	d := onlyFix(t, bag, "E0205")
	if d.Fix == nil || d.Fix.Replace != "B.Clock.now" {
		t.Fatalf("%s, fix %+v", d.Message, d.Fix)
	}
	wantRef(t, ms["Use"], "A.B.Clock.now", syntax.Ref{Kind: syntax.RefCapability, Module: "A.B", Name: "Clock.now"})
}

// The E0201 fix inserts the import in its canonical form: beside `import Layout`,
// `import Admin.Layout as AdminLayout`.
func TestQual_ImportFixIsCanonical(t *testing.T) {
	_, bag, _ := resolveMods(t, std(t), layoutSrc, adminLayoutSrc,
		"module Use\n\nimport Layout\n\nv : Html a\nv = <Layout.Page t={1}><Admin.Layout.Page t={2} /></Layout.Page>\n")
	d := onlyFix(t, bag, "E0201")
	if d.Fix == nil || d.Fix.Replace != "import Admin.Layout as AdminLayout\n" {
		t.Fatalf("%s, fix %+v", d.Message, d.Fix)
	}
}

// A member of an imported page module written with the page's full path is E0205 (only page
// values keep the full path), and a page's message through its qualifier is still another
// page's message.
func TestQual_PageMembers(t *testing.T) {
	idx := "module Todos.Index\n\npage \"/todos\" { }\n\nmsg Save\n\nhelper : Int\nhelper = 1\n"
	_, bag, _ := resolveMods(t, std(t), idx, "module Use\n\nimport Todos.Index\n\na : Int\na = Todos.Index.helper\n")
	if d := onlyFix(t, bag, "E0205"); d.Fix == nil || d.Fix.Replace != "Index.helper" {
		t.Fatalf("%s, fix %+v", d.Message, d.Fix)
	}
	_, bag, _ = resolveMods(t, std(t), idx, "module Use\n\nimport Todos.Index\n\nv : Html a\nv = <button click={Index.Save}>x</button>\n")
	if got := codes(bag); got != "E0002:`Save` belongs to page `Todos.Index`; a page can only send its own messages and `App` messages" {
		t.Fatalf("got %s", got)
	}
}

// A page's forms and messages are global addresses, like page values: a test writes them with
// the page's full path (`Test.submit Auth.Login.Credentials`, `Test.click Todos.Index.Toggle`),
// and the short spelling through the import's qualifier is E0002 with a fix to the full path.
func TestQual_PageFormsAndMsgsFullPath(t *testing.T) {
	login := "module Auth.Login\n\npage \"/login\" { }\n\nform Credentials = { email: Text }\n\nmsg Toggle\n"
	full := "module Use\n\nimport Auth.Login\n\nx : Int\nx = Login.n\n\ntest \"t\" = {\n  let _ <- Test.submit Auth.Login.Credentials { email: \"a\" }\n  let _ <- Test.click Auth.Login.Toggle\n  Task.succeed True\n}\n"
	loginN := login + "\nn : Int\nn = 1\n"
	_, bag, ms := resolveMods(t, testStd(t), loginN, full)
	if len(bag.Items) != 0 {
		t.Fatalf("full path: %s", codes(bag))
	}
	wantRef(t, ms["Use"], "Auth.Login.Credentials", syntax.Ref{Kind: syntax.RefType, Module: "Auth.Login", Name: "Credentials"})
	wantRef(t, ms["Use"], "Auth.Login.Toggle", syntax.Ref{Kind: syntax.RefCtor, Module: "Auth.Login", Name: "Toggle"})

	for _, tc := range []struct{ step, written, want string }{
		{"Test.submit", "Login.Credentials", "Auth.Login.Credentials"},
		{"Test.click", "Login.Toggle", "Auth.Login.Toggle"},
		{"Test.submit", "Login.Toggle", "Auth.Login.Toggle"},
	} {
		src := "module Use\n\nimport Auth.Login\n\nx : Int\nx = Login.n\n\ntest \"t\" = {\n  let _ <- " + tc.step + " " + tc.written + "\n  Task.succeed True\n}\n"
		_, bag, _ := resolveMods(t, testStd(t), loginN, src)
		d := onlyFix(t, bag, diag.UnknownName)
		if !strings.Contains(d.Message, "write `"+tc.want+"`") || strings.Contains(d.Message, "can only send") {
			t.Errorf("%s %s: message %q", tc.step, tc.written, d.Message)
		}
		if d.Fix == nil || d.Fix.Replace != tc.want {
			t.Errorf("%s %s: fix %+v", tc.step, tc.written, d.Fix)
		}
	}
}

// The module's own last segment blocks a qualifier only while the path can grow: `import Layout`
// inside `Admin.Layout` keeps `Layout`.
func TestQual_OwnSegmentFullLength(t *testing.T) {
	_, bag, ms := resolveMods(t, std(t), "module Layout\n\npageTitle : Int\npageTitle = 1\n",
		"module Admin.Layout\n\nimport Layout\n\nx : Int\nx = Layout.pageTitle\n")
	if len(bag.Items) != 0 {
		t.Fatalf("unexpected: %s", codes(bag))
	}
	wantRef(t, ms["Admin.Layout"], "Layout.pageTitle", syntax.Ref{Kind: syntax.RefValue, Module: "Layout", Name: "pageTitle"})
}

// fullPath only fires for the longest real module prefix: beside `import Admin.Users`, a name of
// the unimported module `Admin.Users.Util` (or page `Admin.Users.Edit`) is not E0205.
func TestQual_LongestModulePrefix(t *testing.T) {
	users := "module Admin.Users\n\nn : Int\nn = 1\n"
	util := "module Admin.Users.Util\n\nhelper : Int\nhelper = 1\n"
	_, bag, _ := resolveMods(t, std(t), users, util,
		"module Use\n\nimport Admin.Users\n\na : Int\na = Users.n\n\nx : Int\nx = Admin.Users.Util.helper\n")
	d := onlyFix(t, bag, "E0201")
	if !strings.Contains(d.Message, "module `Admin.Users.Util` is not imported") || d.Fix == nil || d.Fix.Replace != "import Admin.Users.Util\n" {
		t.Fatalf("%s, fix %+v", d.Message, d.Fix)
	}

	edit := "module Admin.Users.Edit\n\npage \"/admin/users/edit\" { }\n\ntype Model = { n: Int }\n\nview : Int\nview = 1\n"
	_, bag, ms := resolveMods(t, std(t), users, edit,
		"module Use\n\nimport Admin.Users\n\na : Int\na = Users.n\n\nm : Admin.Users.Edit.Model -> Int\nm x = 1\n\nv : Html a\nv = <a href={Admin.Users.Edit {}}>e</a>\n")
	if len(bag.Items) != 0 {
		t.Fatalf("page type and value: %s", codes(bag))
	}
	wantRef(t, ms["Use"], "Admin.Users.Edit.Model", syntax.Ref{Kind: syntax.RefPage, Module: "Admin.Users.Edit.Model"}) // as before #23
	wantRef(t, ms["Use"], "Admin.Users.Edit", syntax.Ref{Kind: syntax.RefPage, Module: "Admin.Users.Edit"})

	_, bag, _ = resolveMods(t, std(t), users, edit,
		"module Use\n\nimport Admin.Users\n\na : Int\na = Users.n\n\nx : Int\nx = Admin.Users.Edit.view\n")
	d = onlyFix(t, bag, "E0201")
	if !strings.Contains(d.Message, "module `Admin.Users.Edit` is not imported") || d.Fix == nil || d.Fix.Replace != "import Admin.Users.Edit\n" {
		t.Fatalf("page value member: %s, fix %+v", d.Message, d.Fix)
	}
}

// A bare type that is the eponymous type of an aliased import is fixed to the qualifier alone
// (`AdminLayout`), not `AdminLayout.Layout`.
func TestQual_BareEponymousTypeSuggestsQualifier(t *testing.T) {
	_, bag, _ := resolveMods(t, std(t), layoutSrc, "module Admin.Layout\n\ntype Layout = { n: Int }\n",
		"module Use\n\nimport Layout\nimport Admin.Layout as AdminLayout\n\nz : Layout -> Int\nz l = 1\n")
	var d *diag.Diagnostic
	for i := range bag.Items {
		if bag.Items[i].Code == "E0202" {
			d = &bag.Items[i]
		}
	}
	if d == nil || d.Fix == nil || d.Fix.Replace != "AdminLayout" || !strings.Contains(d.Message, "write `AdminLayout`") {
		t.Fatalf("got %s", codes(bag))
	}
}

// The short spelling of an import with no qualifier (`X.v` for A.X) adds nothing to its E0206.
func TestQual_UnqualifiableShortSpelling(t *testing.T) {
	_, bag, _ := resolveMods(t, std(t), "module A.X\n\nv : Int\nv = 1\n", "module B.X\n\nv : Int\nv = 1\n", "module AX\n\nv : Int\nv = 1\n",
		"module Use\n\nimport A.X\nimport B.X as BX\nimport AX\n\ny : Int\ny = X.v\n\nz : Int\nz = A.X.v\n\nw : Int\nw = BX.v\n")
	for _, d := range bag.Items {
		if d.Code != "E0206" {
			t.Fatalf("want only E0206s, got: %s", codes(bag))
		}
	}
}

// E0206 names what to rename: a declaration or a standard-library name that took the whole
// join, or else the module; E0204 names the other import as an import.
func TestQual_NoQualifierMessages(t *testing.T) {
	msg := func(code string, srcs ...string) string {
		t.Helper()
		_, bag, _ := resolveMods(t, std(t), srcs...)
		for _, d := range bag.Items {
			if d.Code == code {
				return d.Message
			}
		}
		t.Fatalf("no %s in %s", code, codes(bag))
		return ""
	}
	if m := msg("E0206", "module A.X\n\nv : Int\nv = 1\n", "module Use\n\nimport A.X\n\ntype X = Int\ntype AX = Int\n"); !strings.Contains(m, "declares `AX`; rename that declaration") {
		t.Errorf("declaration: %s", m)
	}
	if m := msg("E0206", "module Todo\n\nv : Int\nv = 1\n", "module Use\n\nimport Todo\n\ntype Todo = Int\n"); !strings.Contains(m, "declares `Todo`; rename that declaration") {
		t.Errorf("single segment: %s", m)
	}
	if m := msg("E0206", "module A.X\n\nv : Int\nv = 1\n", "module AX\n\nv : Int\nv = 1\n", "module B.X\n\nv : Int\nv = 1\n", "module Use\n\nimport A.X\nimport B.X\nimport AX\n"); !strings.Contains(m, "another import's; rename the module") {
		t.Errorf("collision: %s", m)
	}
	if m := msg("E0204", layoutSrc, adminLayoutSrc, "module Use\n\nimport Layout\nimport Admin.Layout\n"); !strings.Contains(m, "import `Layout` also ends in `Layout`") {
		t.Errorf("whyAlias: %s", m)
	}
}

// A real, unimported module `X` is still "not imported" when another import (A.X) has no
// qualifier: the short-spelling leniency does not cover it.
func TestQual_UnqualifiableDoesNotHideRealModule(t *testing.T) {
	_, bag, _ := resolveMods(t, std(t), "module A.X\n\nv : Int\nv = 1\n", "module B.X\n\nv : Int\nv = 1\n", "module AX\n\nv : Int\nv = 1\n", "module X\n\nfoo : Int\nfoo = 1\n",
		"module Use\n\nimport A.X\nimport B.X as BX\nimport AX\n\ny : Int\ny = X.foo\n\nw : Int\nw = BX.v\n")
	found := false
	for _, d := range bag.Items {
		found = found || d.Code == "E0201"
	}
	if !found {
		t.Fatalf("no E0201: %s", codes(bag))
	}
}
