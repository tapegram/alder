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
