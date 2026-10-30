// internal/syntax/parser_test.go
package syntax

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestParseGolden(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/golden/parse_*.alder")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			m, diags := ParseFile(filepath.Base(f), src)
			var sb strings.Builder
			sb.WriteString(Dump(m))
			sb.WriteString("\n")
			sp := m.Span // the dump has no spans; the module's is checked here
			fmt.Fprintf(&sb, "@ module %d:%d-%d:%d\n", sp.Start.Line, sp.Start.Col, sp.End.Line, sp.End.Col)
			for _, d := range diags {
				fmt.Fprintf(&sb, "! %s %d:%d %s\n", d.Code, d.Span.Start.Line, d.Span.Start.Col, d.Message)
			}
			got := sb.String()
			goldenPath := strings.TrimSuffix(f, ".alder") + ".dump"
			if *update {
				os.WriteFile(goldenPath, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden %s (run with -update)", goldenPath)
			}
			if got != string(want) {
				t.Fatalf("golden mismatch for %s\n got: %s\nwant: %s", f, got, want)
			}
		})
	}
}

func TestParse_UnclosedElement(t *testing.T) {
	src := "module U\nv : Html a\nv = <div><p>x</div>"
	_, diags := ParseFile("u.alder", []byte(src))
	var msgs []string
	for _, d := range diags {
		if d.Code != "E0001" {
			t.Errorf("unexpected code %s: %s", d.Code, d.Message)
		}
		msgs = append(msgs, d.Message)
	}
	want := []string{"in <p>: closing tag </div> does not match <p>", "in <div>: unclosed element <div>"}
	if strings.Join(msgs, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q want %q", msgs, want)
	}
}

func TestParse_KeywordsAsIdentifiers(t *testing.T) {
	src := "module K\n\nupdate : Msg -> Model -> Session -> Task Nav\nupdate msg model session = f msg { form: 1, page: 2 }\n"
	m, diags := ParseFile("k.alder", []byte(src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	want := "(app (var f) (var msg) (record (form (int 1)) (page (int 2))))"
	if got := Dump(m); !strings.Contains(got, want) {
		t.Fatalf("dump %s does not contain %s", got, want)
	}
}

func TestParseSignatures_KeywordNamedValue(t *testing.T) {
	m, ds := ParseSignatures("Db.alder", []byte("module Db\n\nwhere : Condition t -> Select t -> Select t\nfrom : Table t -> Select t\n"))
	if len(ds) > 0 {
		t.Fatal(ds[0].Message)
	}
	if len(m.Decls) != 2 {
		t.Fatalf("decls = %d", len(m.Decls))
	}
	if v, ok := m.Decls[0].(*ValueDecl); !ok || v.Name != "where" || v.Sig == nil {
		t.Fatalf("got %#v", m.Decls[0])
	}
}

// Generated Go names join parts with "__": no identifier may contain it, and no module name
// segment may end with '_' (internal/gen/names.go).
func TestParse_ReservedUnderscores(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"module M\n\nmax__len : Int\nmax__len = 1\n", "3:1 identifiers may not contain two underscores in a row: `max__len`"},
		{"module M\n\ntype T__U = A\n", "3:6 identifiers may not contain two underscores in a row: `T__U`"},
		{"module M\n\nf : Int -> Int\nf __ = 1\n", "4:3 identifiers may not contain two underscores in a row: `__`"},
		{"module A__B\n", "1:8 identifiers may not contain two underscores in a row: `A__B`"},
		{"module A_\n", "1:8 module name segments may not end with an underscore: `A_`"},
		{"module A.B_\n", "1:10 module name segments may not end with an underscore: `B_`"},
		{"module M\n\nimport X_\n", "3:8 module name segments may not end with an underscore: `X_`"},
	} {
		_, diags := ParseFile("t.alder", []byte(tc.src))
		var got []string
		for _, d := range diags {
			got = append(got, fmt.Sprintf("%d:%d %s", d.Span.Start.Line, d.Span.Start.Col, d.Message))
		}
		if !slicesContains(got, tc.want) {
			t.Errorf("%q: diagnostics %q lack %q", tc.src, got, tc.want)
		}
	}
	// HTML attribute names are not identifiers of the language: `__` is allowed there
	_, diags0 := ParseFile("t.alder", []byte("module M\n\nv : Html a\nv = <div data-x__y=\"1\" aria-a__b=\"2\">x</div>\n"))
	if len(diags0) != 0 {
		t.Errorf("attribute names with __ rejected: %v", diags0)
	}
	// single underscores, also leading and trailing, are fine
	_, diags := ParseFile("t.alder", []byte("module A_b.C\n\n_x_ : Int\n_x_ = 1\n\ntype T_ = A_ | B_c\n"))
	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics: %v", diags)
	}
}

func slicesContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// dumpHTML parses `v = <src>` and dumps the body.
func dumpHTML(t *testing.T, src string) string {
	t.Helper()
	m, diags := ParseString("V.alder", "module V\nv : Html a\nv = "+src+"\n")
	if len(diags) != 0 {
		t.Fatalf("%q: unexpected diagnostics: %v", src, diags)
	}
	return Dump(m.Decls[len(m.Decls)-1].(*ValueDecl).Body)
}

// The JSX whitespace rule: a text run containing a newline is trimmed where it touches a newline
// and dropped if empty; a run with no newline is kept as-is.
func TestParse_InlineWhitespaceJSXRule(t *testing.T) {
	cases := []struct{ src, want string }{
		{`<p><b>a</b> <i>b</i></p>`, `(el p (el b (text "a")) (text " ") (el i (text "b")))`},
		{"<p>\n  <b>a</b>\n  <i>b</i>\n</p>", `(el p (el b (text "a")) (el i (text "b")))`},
		{`<p>{k} {v}</p>`, `(el p (expr (var k)) (text " ") (expr (var v)))`},
		{"<p>\n  hello {name}\n</p>", `(el p (text "hello ") (expr (var name)))`},
		{"<p><b>a</b>\n<i>b</i></p>", `(el p (el b (text "a")) (el i (text "b")))`},
		{"<p>\n  one\n  two  \n  three {x}</p>", `(el p (text "one two three ") (expr (var x)))`},
		{"<p>\n  a\n  -- a comment\n  b\n</p>", `(el p (text "a b"))`},
		{`<p>-- not a comment</p>`, `(el p (text "-- not a comment"))`},
		{"<p><b>a</b> -- text\n</p>", `(el p (el b (text "a")) (text " -- text"))`},
	}
	for _, c := range cases {
		got := dumpHTML(t, c.src)
		if got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestTypeDecl_EqualOnNextLine(t *testing.T) {
	src := "module M\n\ntype Shape\n  = Circle Float\n  | Rect { w: Float, h: Float }\n\ntype Tree a\n    = Leaf\n    | Node a\n\ntype B\n  = brand Text\n\ntype R\n  = { x: Int }\n"
	m, diags := ParseFile("M.alder", []byte(src))
	if len(diags) > 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	got := []string{}
	for _, d := range m.Decls {
		td := d.(*TypeDecl)
		got = append(got, fmt.Sprintf("%s:%d/%v/%v", td.Name, len(td.Union), td.Brand != nil, td.Record != nil))
	}
	if strings.Join(got, " ") != "Shape:2/false/false Tree:2/false/false B:0/true/false R:0/false/true" {
		t.Errorf("decls %v", got)
	}
	// an `=` at column 1 is not a continuation: the declaration is incomplete
	_, diags = ParseFile("M.alder", []byte("module M\n\ntype T\n= A\n"))
	if len(diags) == 0 || diags[0].Code != "E0001" {
		t.Errorf("column-1 `=`: %v", diags)
	}
}

// A delete rule is an attribute like any other; its misuse (without `@ref`, before it, or a
// second rule) is E0011 from the checker (internal/check TestDeleteRules), not a parse error.
func TestParse_DeleteRules(t *testing.T) {
	m, diags := ParseFile("c.alder", []byte("module C\n\ntable C {\n  id: CId @id,\n  post: Post.PostId @ref Post @cascade,\n  author: Maybe User.UserId @ref User @setNull,\n  tag: Tag.TagId @ref Tag @restrict,\n}\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	td := m.Decls[0].(*TableDecl)
	var got []string
	for _, f := range td.Fields {
		for _, a := range f.Attrs {
			got = append(got, f.Name+":"+a.Name+a.Arg)
		}
	}
	if want := "id:id post:refPost post:cascade author:refUser author:setNull tag:refTag tag:restrict"; strings.Join(got, " ") != want {
		t.Fatalf("attrs %q, want %q", strings.Join(got, " "), want)
	}
	for _, field := range []string{"post: Post.PostId @cascade", "post: Post.PostId @cascade @ref Post", "post: Post.PostId @ref Post @cascade @setNull"} {
		if _, diags := ParseFile("c.alder", []byte("module C\n\ntable C {\n  id: CId @id,\n  "+field+",\n}\n")); len(diags) != 0 {
			t.Errorf("%s: unexpected parse diagnostics %+v", field, diags)
		}
	}
}

// `@renamed "old"` takes a plain string: on a field after its type (and default), on a table
// after its name.
func TestParse_Renamed(t *testing.T) {
	m, diags := ParseFile("post.alder", []byte("module Post\n\ntable Post @renamed \"Article\" {\n  id: PostId @id,\n  title: Text = \"x\" @renamed \"heading\",\n  n: Int @unique @renamed \"count\",\n}\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	td := m.Decls[0].(*TableDecl)
	if len(td.Attrs) != 1 || td.Attrs[0].Name != "renamed" || td.Attrs[0].Arg != "Article" {
		t.Fatalf("table attrs %+v", td.Attrs)
	}
	var got []string
	for _, f := range td.Fields {
		for _, a := range f.Attrs {
			got = append(got, f.Name+":"+a.Name+"="+a.Arg)
		}
	}
	if want := "id:id= title:renamed=heading n:unique= n:renamed=count"; strings.Join(got, " ") != want {
		t.Fatalf("attrs %q, want %q", strings.Join(got, " "), want)
	}
	for _, c := range []struct{ src, want string }{
		{"  title: Text @renamed heading,", "expected the old name as a string"},
		{"  title: Text @renamed \"a{b}\",", "`@renamed` takes a plain string"},
	} {
		_, diags := ParseFile("post.alder", []byte("module Post\n\ntable Post {\n  id: PostId @id,\n"+c.src+"\n}\n"))
		if len(diags) == 0 || !strings.Contains(diags[0].Message, c.want) {
			t.Errorf("%s: got %+v, want %q", c.src, diags, c.want)
		}
	}
	// a form field: the attribute is refused, its string skipped
	_, diags = ParseFile("f.alder", []byte("module F\n\nform F = {\n  title: Text @renamed \"heading\",\n}\n"))
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "only allowed on table fields") {
		t.Errorf("form: %+v", diags)
	}
}

func TestParse_FakeErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// a fake with no body
		{"module M\n\nfake Stripe\n", "3:12 expected `{` after `fake Stripe`: a fake holds its tables and one definition per entry of the capability, got end of line"},
		// a non-table declaration inside
		{"module M\n\nfake Stripe {\n  type Card = { n: Int }\n  charge args = Task.succeed 1\n}\n", "4:3 in `fake Stripe`: a fake holds only `table` declarations and entry definitions; `type` declarations belong at the top level of the module"},
		// a signature inside
		{"module M\n\nfake Stripe {\n  charge : Int -> Task Int\n  charge args = Task.succeed 1\n}\n", "4:3 in `fake Stripe`: a fake's entry takes its type from the capability: write `charge args = …` without a signature"},
		// unterminated
		{"module M\n\nfake Stripe {\n  now = Query.succeed 1\n", "5:1 in `fake Stripe`: unterminated `fake Stripe`: expected `}`"},
		// a top-level value named fake: its signature, its definition, a function
		{"module M\n\nfake : Int\nfake = 1\n", "3:1 `fake` starts a fake declaration at column 1: rename the value"},
		{"module M\n\nfake : Int\nfake = 1\n", "4:1 `fake` starts a fake declaration at column 1: rename the value"},
		{"module M\n\nfake x =\n  x\n\ny = 1\n", "3:1 `fake` starts a fake declaration at column 1: rename the value"},
	} {
		_, diags := ParseFile("t.alder", []byte(tc.src))
		var got []string
		for _, d := range diags {
			got = append(got, fmt.Sprintf("%d:%d %s", d.Span.Start.Line, d.Span.Start.Col, d.Message))
		}
		if !slicesContains(got, tc.want) {
			t.Errorf("%q: diagnostics %q lack %q", tc.src, got, tc.want)
		}
		if strings.Contains(tc.want, "rename the value") {
			for _, g := range got { // the value's lines say nothing else
				if !strings.Contains(g, "rename the value") {
					t.Errorf("%q: also %q", tc.src, g)
				}
			}
		}
	}
}

func TestParse_FakeEntriesAndTables(t *testing.T) {
	src := "module M\n\nfake Stripe {\n  now = Query.succeed 1\n  table Charge {\n    id: ChargeId @id,\n  }\n  charge args = Task.succeed args.cents\n}\n"
	m, diags := ParseFile("t.alder", []byte(src))
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %+v", diags)
	}
	fd, ok := m.Decls[0].(*FakeDecl)
	if !ok || fd.Name != "Stripe" || len(fd.Tables) != 1 || len(fd.Entries) != 2 {
		t.Fatalf("got %#v", m.Decls[0])
	}
	if fd.Entries[0].Name != "now" || len(fd.Entries[0].Params) != 0 || fd.Entries[1].Name != "charge" || len(fd.Entries[1].Params) != 1 || fd.Entries[0].Sig != nil {
		t.Fatalf("entries: %+v %+v", fd.Entries[0], fd.Entries[1])
	}
	// `fake` is a keyword only in column 1: a local or field named fake is fine
	if _, diags := ParseFile("t.alder", []byte("module M\n\nf : Int -> Int\nf fake = fake + 1\n")); len(diags) != 0 {
		t.Fatalf("fake as a name: %+v", diags)
	}
}

// dumpBody parses `module V` plus src and dumps the last declaration's body.
func dumpBody(t *testing.T, src string) string {
	t.Helper()
	m, diags := ParseString("V.alder", "module V\n"+src)
	if len(diags) != 0 {
		t.Fatalf("%q: unexpected diagnostics: %v", src, diags)
	}
	return Dump(m.Decls[len(m.Decls)-1].(*ValueDecl).Body)
}

// Every binary operator continues the expression when it begins the next line (book chapter 3).
func TestParse_ContinuationEveryOperator(t *testing.T) {
	for _, op := range []string{"|>", "??", "||", "&&", "==", "!=", "<", "<=", ">", ">=", "+", "-", "++", "*", "/", "%"} {
		oneLine := dumpBody(t, "v = f 1 "+op+" 2\n")
		split := dumpBody(t, "v =\n  f 1\n    "+op+" 2\n")
		if split != oneLine {
			t.Errorf("%s on the next line:\n got %s\nwant %s", op, split, oneLine)
		}
	}
}

// A comparison begun on the next line still does not chain.
func TestParse_ContinuedComparisonDoesNotChain(t *testing.T) {
	_, diags := ParseString("V.alder", "module V\nv =\n  a\n    == b\n    == c\n")
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "comparisons do not chain") {
		t.Errorf("diags %v", diags)
	}
}

// `<` continues only when whitespace follows it; `<` followed by a letter or `/` starts an
// element, so a block's last line `<p>…</p>` is the block's result, not `x < p`.
func TestParse_LtContinuationVersusElement(t *testing.T) {
	got := dumpBody(t, "v = {\n  let x = f 1\n  <p>{x}</p>\n}\n")
	if !strings.Contains(got, "(el p") || strings.Contains(got, "<") {
		t.Errorf("element line read as a comparison: %s", got)
	}
	if got := dumpBody(t, "v =\n  a\n    < b\n"); got != dumpBody(t, "v = a < b\n") {
		t.Errorf("`< b` did not continue: %s", got)
	}
	// `<` with no space before a name is an element, so it cannot continue: the old E0001.
	_, diags := ParseString("V.alder", "module V\nv =\n  a\n    <b\n")
	if len(diags) == 0 || diags[0].Code != "E0001" {
		t.Errorf("`<b` on the next line: %v", diags)
	}
}

// A line that begins with something that is not an operator is still not a continuation.
func TestParse_NonOperatorLineDoesNotContinue(t *testing.T) {
	_, diags := ParseString("V.alder", "module V\nv =\n  f 1\n    = 2\n")
	if len(diags) == 0 || diags[0].Code != "E0001" || !strings.Contains(diags[0].Message, "expected a declaration, got `=`") {
		t.Errorf("diags %v", diags)
	}
}

// TestParse_RulesSurface (M11): `@owner` in any position among a field's attributes, and the
// `role` clause of a page's `requires`, alone, after `user`, and with several constructors.
func TestParse_RulesSurface(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"table Todo {\n  owner: User.UserId @owner @ref User @cascade,\n}", "(field owner User.UserId @owner @ref User @cascade)"},
		{"table Todo {\n  owner: User.UserId @ref User @owner,\n}", "(field owner User.UserId @ref User @owner)"},
		{`page "/a" { } requires user`, "(page \"/a\" {} requires user)"},
		{`page "/a" { } requires user, role Author`, "(page \"/a\" {} requires user role Author)"},
		{`page "/a" { } requires user, role Author | Admin`, "(page \"/a\" {} requires user role Author | Admin)"},
		{`page "/a" { } requires role Author`, "(page \"/a\" {} role Author)"},
		{"page \"/a\" { } requires role\ntype Model = { }", "(page \"/a\" {} requires role)"},
	} {
		m, diags := ParseFile("m.alder", []byte("module M\n\n"+c.src+"\n"))
		if len(diags) > 0 {
			t.Errorf("%q: %v", c.src, diags)
			continue
		}
		if got := Dump(m); !strings.Contains(got, c.want) {
			t.Errorf("%q: dump %s lacks %s", c.src, got, c.want)
		}
	}
	m, _ := ParseFile("m.alder", []byte("module M\n\npage \"/a\" { } requires user, role Author | Admin\n"))
	pd := m.Decls[0].(*PageDecl)
	if strings.Join(pd.Requires, ",") != "user" || strings.Join(pd.Roles, ",") != "Author,Admin" {
		t.Fatalf("page: requires %q roles %v", pd.Requires, pd.Roles)
	}
	if r, s := pd.RequiresSpan, pd.RolesSpan; r.Start.Col != 15 || r.End.Col != 28 || s.Start.Col != 30 || s.End.Col != 49 {
		t.Fatalf("spans: requires %+v roles %+v", r, s)
	}
	for _, c := range []struct{ src, want string }{
		{"requires user, Author", "expected `role R` after `requires user,`"},
		{"requires user role Admin", "expected `,` between `requires user` and `role`: write `requires user, role R`"},
		{"requires role Admin, user", "`user` goes first: write `requires user, role R` with unqualified constructors"},
		{"requires user, role User.Admin", "write `requires user, role R` with unqualified constructors"},
		{"requires user, role Author | User.Admin", "write `requires user, role R` with unqualified constructors"},
	} {
		_, diags := ParseFile("m.alder", []byte("module M\n\npage \"/a\" { } "+c.src+"\n\ntype Model = { }\n"))
		if len(diags) != 1 || !strings.Contains(diags[0].Message, c.want) {
			t.Errorf("%s: diags %v, want one with %q", c.src, diags, c.want)
		}
	}
}

// TestParse_PartitionsSurface (M15): `@partition k`, a session field's `via T`, App's `silo k
// { … }` and `choose k = Page`, and a page's `requires user, org` and the admin form `requires
// user, role Admin, silo org`; `silo` and `choose` stay ordinary names elsewhere.
func TestParse_PartitionsSurface(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"table Project {\n  org: Org.OrgId @ref Org @partition org,\n}", "(field org Org.OrgId @ref Org @partition org)"},
		{"session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership }", "(field org (Maybe Org.OrgId) via Membership)"},
		{"session { org: Maybe Org.OrgId via Membership = Nothing }", "(field org (Maybe Org.OrgId) via Membership = (ctor Nothing))"},
		{"silo org { Project, Task }", "(silo org Project Task)"},
		{"silo org {\n  Project,\n  Task,\n}", "(silo org Project Task)"},
		{"choose org = Orgs.Index", "(choose org Orgs.Index)"},
		{`page "/a" { } requires user, org`, "(page \"/a\" {} requires user, org)"},
		{`page "/a" { } requires user, org, role Admin`, "(page \"/a\" {} requires user, org role Admin)"},
		{`page "/a" { org: Org.OrgId } requires user, role Admin, silo org`, "requires user role Admin silo org)"},
		{"page \"/a\" { } requires user, silo\ntype Model = { }", "(page \"/a\" {} requires user, silo)"},
		{"silo : Int\nsilo = 1", "(val silo : Int"},
		{"choose : Int -> Int\nchoose x = x", "(val choose : (fn Int -> Int) x"},
	} {
		m, diags := ParseFile("m.alder", []byte("module M\n\n"+c.src+"\n"))
		if len(diags) > 0 {
			t.Errorf("%q: %v", c.src, diags)
			continue
		}
		if got := Dump(m); !strings.Contains(got, c.want) {
			t.Errorf("%q: dump %s lacks %s", c.src, got, c.want)
		}
	}
	m, _ := ParseFile("m.alder", []byte("module M\n\npage \"/a\" { org: Org.OrgId } requires user, team, role Admin, silo org\n"))
	pd := m.Decls[0].(*PageDecl)
	if strings.Join(pd.Requires, ",") != "user,team" || pd.Silo != "org" || len(pd.FieldSpans) != 2 || pd.FieldSpans[1].Start.Col != 45 || pd.SiloSpan.Start.Col != 63 {
		t.Fatalf("page: %+v", pd)
	}
	for _, c := range []struct{ src, want string }{
		{"requires user, role Admin, org", "only `silo k` may follow the `role` clause"},
		{"requires user, 3", "expected `role R` after `requires user,` (or another session field, or `silo k`)"},
		{"requires user, org role Admin", "expected `,` between `requires user, org` and `role`"},
	} {
		_, diags := ParseFile("m.alder", []byte("module M\n\npage \"/a\" { } "+c.src+"\n\ntype Model = { }\n"))
		if len(diags) != 1 || !strings.Contains(diags[0].Message, c.want) {
			t.Errorf("%s: diags %v, want one with %q", c.src, diags, c.want)
		}
	}
}

// TestParse_HoleSpan: a hole's span covers its `?` and its whole name (the token's text is the
// name alone), so W0002's caret marks `?count` and a tool can replace exactly the hole.
func TestParse_HoleSpan(t *testing.T) {
	m, diags := ParseFile("M.alder", []byte("module M\n\nx : Int\nx = 1 + ?count\n"))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	h := m.Decls[0].(*ValueDecl).Body.(*BinOp).R.(*HoleExpr)
	if sp := h.Span; sp.Start.Line != 4 || sp.Start.Col != 9 || sp.End.Line != 4 || sp.End.Col != 15 {
		t.Errorf("span of ?count: %d:%d-%d:%d, want 4:9-4:15", sp.Start.Line, sp.Start.Col, sp.End.Line, sp.End.Col)
	}
}

// A negative literal's span starts at its `-` (the formatter prints literals from their source).
func TestParse_NegativeLiteralSpan(t *testing.T) {
	m, diags := ParseFile("M.alder", []byte("module M\n\nx : Float\nx = - 1.5\n"))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	l := m.Decls[0].(*ValueDecl).Body.(*FloatLit)
	if sp := l.Span; sp.Start.Line != 4 || sp.Start.Col != 5 || sp.End.Line != 4 || sp.End.Col != 10 {
		t.Errorf("span of - 1.5: %d:%d-%d:%d, want 4:5-4:10", sp.Start.Line, sp.Start.Col, sp.End.Line, sp.End.Col)
	}
}

// An element's span ends after its final `>`, of a closing tag or of a self-closing one.
func TestParse_ElementSpanEndsAfterGt(t *testing.T) {
	m, diags := ParseFile("M.alder", []byte("module M\n\nv : Html a\nv = <p>x</p>\n\nw : Html a\nw = <br />\n"))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	for i, want := range []int{13, 11} {
		sp := m.Decls[i].(*ValueDecl).Body.(*Element).Span
		if sp.End.Col != want {
			t.Errorf("element %d ends at column %d, want %d", i, sp.End.Col, want)
		}
	}
}

// A raw-text body ends at its closing tag in any case, as in HTML: `</Script>` and `</STYLE >`
// close their elements, and the body before them is kept byte for byte.
func TestParse_RawTextCloseAnyCase(t *testing.T) {
	src := "module K\n\nv : Html a\nv = <div><script>if (a < b) { go() }</Script><style>p { x: y }</STYLE ><p>after</p></div>\n"
	m, diags := ParseFile("k.alder", []byte(src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	want := `(el script (rawtext "if (a < b) { go() }")) (el style (rawtext "p { x: y }")) (el p (text "after"))`
	if got := Dump(m); !strings.Contains(got, want) {
		t.Fatalf("dump %s does not contain %s", got, want)
	}
}

func TestParse_StringEscapes(t *testing.T) {
	src := "module U\nv : Text\nv = \"a\\r\\nb\\tc\\\\d\\\"e\\{f}\"\n"
	m, diags := ParseFile("u.alder", []byte(src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if got, want := Dump(m), `"a\r\nb\tc\\d\"e{f}"`; !strings.Contains(got, want) {
		t.Errorf("dump lacks %s:\n%s", want, got)
	}
}

// The formatter's layouts parse to the same tree as the one-line spelling.
func TestParse_LayoutEquivalence(t *testing.T) {
	cases := []struct{ name, broken, flat string }{
		{"attrs",
			"module M\nv : Html a\nv =\n  <div>\n    <a\n        href=\"x\"\n        class=\"y\"\n      >hi</a>\n    <a href=\"x\"\n        class=\"y\">hi</a>\n    <input\n        name=\"x\"\n      />\n  </div>\n",
			"module M\nv : Html a\nv =\n  <div>\n    <a href=\"x\" class=\"y\">hi</a>\n    <a href=\"x\" class=\"y\">hi</a>\n    <input name=\"x\" />\n  </div>\n"},
		{"sig",
			"module M\nf : Int\n  -> Text\n  -> Bool\nf a b = True\ng :\n  { a: Int, b: Int }\n  -> Int\ng r = 1\n",
			"module M\nf : Int -> Text -> Bool\nf a b = True\ng : { a: Int, b: Int } -> Int\ng r = 1\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bm, bd := ParseFile("b.alder", []byte(c.broken))
			fm, fd := ParseFile("f.alder", []byte(c.flat))
			if len(bd) != 0 || len(fd) != 0 {
				t.Fatalf("diagnostics: broken %v flat %v", bd, fd)
			}
			if Dump(bm) != Dump(fm) {
				t.Fatalf("dumps differ\nbroken: %s\nflat: %s", Dump(bm), Dump(fm))
			}
		})
	}
}

func TestIsScratchName(t *testing.T) {
	for file, want := range map[string]bool{
		"scratch.alder":           true,
		"Todos.scratch.alder":     true,
		"app/Sub/X.scratch.alder": true,
		"Scratch.alder":           false,
		"scratch.alder.bak":       false,
		"scratch/Todo.alder":      false,
	} {
		if got := IsScratchName(file); got != want {
			t.Errorf("IsScratchName(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestParse_ImportAs(t *testing.T) {
	m, diags := ParseFile("m.alder", []byte("module M\n\nimport Admin.Layout as AdminLayout\n"))
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	im := m.Imports[0]
	if im.Module != "Admin.Layout" || im.Alias != "AdminLayout" {
		t.Fatalf("import = %q as %q", im.Module, im.Alias)
	}
	// Spans are Start-inclusive, End-exclusive (like spanTok): AdminLayout is cols 24-34.
	if s := im.AliasSpan; s.Start.Line != 3 || s.Start.Col != 24 || s.End.Line != 3 || s.End.Col != 35 {
		t.Errorf("AliasSpan = %+v", s)
	}
	if !strings.Contains(Dump(m), "(import Admin.Layout as AdminLayout)") {
		t.Errorf("dump: %s", Dump(m))
	}
	for src, want := range map[string]string{
		"module M\n\nimport A.B as\n": "expected alias after `as`",
		"module M\n\nimport A as b\n": "alias must be capitalized",
	} {
		m, diags := ParseFile("m.alder", []byte(src))
		if len(diags) == 0 || !strings.Contains(diags[0].Message, want) {
			t.Errorf("%q: diags = %v, want %q", src, diags, want)
		}
		// the import survives its malformed alias, without one
		if m == nil || len(m.Imports) != 1 || m.Imports[0].Alias != "" {
			t.Errorf("%q: imports = %+v, want one with no alias", src, m)
		}
	}
	m, diags = ParseFile("m.alder", []byte("module M\n\nas : Int\nas = 1\n"))
	if len(diags) != 0 || len(m.Decls) != 1 {
		t.Errorf("value named as: diags=%v decls=%d", diags, len(m.Decls))
	}
}

// TestParse_PageParamWire (#55): a page's query parameter may name its query key with `as
// "Key"`, after its type or its default; `as` stays an ordinary name in expressions and types
// elsewhere, and a wire name outside a page's parameters is a parse error saying so.
func TestParse_PageParamWire(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`page "/r/{cfg}" { cfg: Text, electionId: Text = "" as "ElectionId" }`, `electionId: Text = (str "") as "ElectionId"}`},
		{`page "/r" { electionId: Maybe Text as "ElectionId" }`, `{electionId: (Maybe Text) as "ElectionId"}`},
		{"page \"/r\" {\n  a: Int = f 1 as \"A\",\n  b: List Int as \"B\",\n}", `{a: Int = (app (var f) (int 1)) as "A", b: (List Int) as "B"}`},
		{`page "/r" { n: Int = 0 }`, `{n: Int = (int 0)}`},
		{"x : Int\nx = f as 1", "(app (var f) (var as) (int 1))"},
		// #63: `as` may start the line after the default or the type (the list is brace-delimited)
		{"page \"/r\" {\n  a: Text = \"\"\n    as \"A\",\n  b: Int\n    as \"B\",\n}", `{a: Text = (str "") as "A", b: Int as "B"}`},
		// #63: inside parens, brackets or braces in a default, `as` is an ordinary name
		{`page "/r" { a: Int = f (g as) as "A" }`, `{a: Int = (app (var f) (app (var g) (var as))) as "A"}`},
		{`page "/r" { a: List Int = f [as] as "A" }`, `{a: (List Int) = (app (var f) (list (var as))) as "A"}`},
	} {
		m, diags := ParseFile("m.alder", []byte("module M\n\n"+c.src+"\n"))
		if len(diags) > 0 {
			t.Errorf("%q: %v", c.src, diags)
			continue
		}
		if got := Dump(m); !strings.Contains(got, c.want) {
			t.Errorf("%q: dump %s lacks %s", c.src, got, c.want)
		}
	}
	m, _ := ParseFile("m.alder", []byte("module M\n\npage \"/r\" { e: Text = \"\" as \"E\" }\n"))
	f := m.Decls[0].(*PageDecl).Params.Fields[0]
	if f.Wire != "E" || f.WireSpan.Start.Col != 26 || f.WireSpan.End.Col != 32 || f.Span.End.Col != 32 {
		t.Fatalf("field: wire %q span %+v field span %+v", f.Wire, f.WireSpan, f.Span)
	}
	for _, c := range []struct{ src, want string }{
		{"msg Pick { id: Int as \"Id\" }", "a wire name (`as \"Id\"`) is only allowed on a page's query parameters"},
		{"form F = { title: Text as \"Title\" }", "a wire name (`as \"Title\"`) is only allowed on a page's query parameters"},
		{"type R = { a: Int = 0 as \"A\" }", "a wire name (`as \"A\"`) is only allowed on a page's query parameters"},
		{"page \"/r\" { e: Text as Key }", "expected the query key after `as`, as a string (`as \"Key\"`)"},
		{"page \"/r\" { e: Text as \"{x}\" }", "a wire name is a plain string, without `{…}`"},
	} {
		_, diags := ParseFile("m.alder", []byte("module M\n\n"+c.src+"\n"))
		if len(diags) != 1 || !strings.Contains(diags[0].Message, c.want) {
			t.Errorf("%s: diags %v, want one with %q", c.src, diags, c.want)
		}
	}
}

// A bad `(` in a lambda's parameters reports that one error, not a second about the missing `->`.
func TestParse_LambdaBadParenOneError(t *testing.T) {
	for _, src := range []string{
		"module M\n\nf = \\(1 : Int) -> 2\n",
		"module M\n\nf = \\( -> 2\n",
		"module M\n\nf = \\(x : Int) (] -> x\n",
	} {
		_, diags := ParseFile("t.alder", []byte(src))
		if len(diags) != 1 || !strings.Contains(diags[0].Message, "expected a parameter name after `(`") {
			t.Errorf("%q: diagnostics %+v, want just the parameter-name error", src, diags)
		}
	}
}

// A bad lambda parameter inside delimiters leaves them to the enclosing parser, and the next
// declaration's own errors stay its own: exactly one error in `f` (the parameter) and one in
// `g`.
func TestParse_LambdaBadParenInsideDelimiters(t *testing.T) {
	for name, src := range map[string]string{
		"parens, blank line": "module M\n\nf = List.map (\\(1 x) -> x) [1]\n\ng : Int\ng = (1 +\n",
		"block":              "module M\n\nf = {\n  let h = List.map (\\(1 x) -> x) [1]\n  h\n}\n\ng : Int\ng = (1 +\n",
		"no blank line":      "module M\n\nf = List.map (\\(1 x) -> x) [1]\ng : Int\ng = (1 +\n",
	} {
		_, diags := ParseFile("t.alder", []byte(src))
		if len(diags) != 2 ||
			!strings.Contains(diags[0].Message, "in the definition of `f`: expected a parameter name after `(`") ||
			!strings.Contains(diags[1].Message, "in the definition of `g`:") {
			t.Errorf("%s: diagnostics %+v", name, diags)
		}
	}
}
