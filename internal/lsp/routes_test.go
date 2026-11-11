package lsp

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestRoutes_List(t *testing.T) {
	root := blogCopy(t)
	c := start(t, root)
	openClean(t, c, "app/Posts/Show.alder")
	var out []routeItem
	if e := c.call("alder/routes", map[string]any{}, &out); e != nil {
		t.Fatalf("rpc error %+v", e)
	}
	var get, post bool
	for _, r := range out {
		if r.Method == "GET" && r.Path == "/posts/{slug}" && r.Module == "Posts.Show" && r.Name == "Show" &&
			r.URI == c.uri("app/Posts/Show.alder") && r.Range.Start == (Position{Line: 8, Character: 0}) {
			get = true
		}
		if r.Method == "POST" && r.Path == "/posts/{slug}/comments" && r.Module == "Posts.Show" && r.Name == "AddComment" &&
			strings.HasSuffix(r.URI, "app/Posts/Show.alder") && r.Range.Start == (Position{Line: 11, Character: 0}) {
			post = true
		}
	}
	if !get || !post {
		t.Fatalf("GET Show %v, POST AddComment %v in %+v", get, post, out)
	}
}

// routesCopy copies the routes-ok corpus app to a temporary root and opens Todos.
func routesCopy(t *testing.T) (*client, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../testdata/corpus/routes-ok")); err != nil {
		t.Fatal(err)
	}
	c := start(t, root)
	const rel = "app/Todos.alder"
	c.open(rel, c.disk(rel))
	if ds := c.diags(rel); len(ds) != 0 {
		t.Fatalf("diagnostics %+v", ds)
	}
	return c, rel
}

// A route's hover is its method, path, parameters, form and guard, at its declaration and at a
// constructor that sends it.
func TestHover_Route(t *testing.T) {
	c, rel := routesCopy(t)
	const want = "PATCH /todos/{id}/toggle  { id: Todo.TodoId }  requires user"
	for _, p := range []Position{
		c.at(rel, "route Toggle", len("route ")),
		c.at(rel, "Toggle { id: t.id }", 0),
		c.at(rel, "Toggle.update :", 0),
	} {
		if got := hover(t, c, rel, p); !strings.Contains(got, want) {
			t.Errorf("hover at %+v: %q, want it to contain %q", p, got, want)
		}
	}
	if got := hover(t, c, rel, c.at(rel, "route Create", len("route "))); !strings.Contains(got, `POST /todos  { }  form NewTodo  requires user`) {
		t.Errorf("form route hover %q", got)
	}
}

// A member's hover names its own route's session even when the route shares a narrowed record
// with an earlier one (Row and Index both `requires user`).
func TestHover_MemberOwnSession(t *testing.T) {
	c, rel := routesCopy(t)
	for _, p := range []Position{c.at(rel, "Row.view :", len("Row.")), c.at(rel, "Row.view t session}", len("Row."))} {
		got := hover(t, c, rel, p)
		if !strings.Contains(got, "Row.view : Todo -> Row.Session -> Html") || strings.Contains(got, "Index.Session") {
			t.Errorf("hover at %+v: %q", p, got)
		}
	}
}

// Definition from a route's member goes to the route line (on the route's name) or to the member
// (on the member's name); from a route constructor in markup, to the route line.
func TestDefinition_FromMember(t *testing.T) {
	c, rel := routesCopy(t)
	routeLine := c.at(rel, "route Row ", 0).Line
	for _, k := range []struct {
		name string
		p    Position
		line int
	}{
		{"route part of a use", c.at(rel, "Row.view t session}", 0), routeLine},
		{"member part of a use", c.at(rel, "Row.view t session}", len("Row.")), c.at(rel, "Row.view :", 0).Line},
		{"route part of a declaration", c.at(rel, "Row.view :", 0), routeLine},
		{"route part of a type", c.at(rel, "Row.Params ->", 0), routeLine},
	} {
		locs := definitionAt(t, c, rel, k.p)
		if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, rel) || locs[0].Range.Start.Line != k.line {
			t.Errorf("%s: %+v, want line %d", k.name, locs, k.line)
		}
	}
	// the member's span is the whole `Row.view`, not a prefix of it
	locs := definitionAt(t, c, rel, c.at(rel, "Row.view t session}", len("Row.")))
	if len(locs) == 1 && locs[0].Range.End.Character-locs[0].Range.Start.Character != len("Row.view") {
		t.Errorf("member range %+v", locs[0].Range)
	}
}

func TestDefinition_FromClick(t *testing.T) {
	c, rel := routesCopy(t)
	locs := definitionAt(t, c, rel, c.at(rel, "Toggle { id: t.id }", 0))
	line := c.at(rel, "route Toggle", 0).Line
	if len(locs) != 1 || locs[0].Range.Start.Line != line || locs[0].Range.Start.Character != len("route ") {
		t.Fatalf("definition %+v, want the name on line %d", locs, line)
	}
	// and every use of the route is a reference
	var refs []Location
	p := c.at(rel, "route Toggle", len("route "))
	if e := c.call("textDocument/references", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}, "position": p,
		"context": map[string]any{"includeDeclaration": true}}, &refs); e != nil {
		t.Fatal(e)
	}
	// the declaration, the constructor, and the route part of each member: the signature's name,
	// the `Toggle.Params` in it, and the definition line
	var got []string
	for _, r := range refs {
		if r.Range.End.Line != r.Range.Start.Line || r.Range.End.Character-r.Range.Start.Character != len("Toggle") {
			t.Errorf("reference %+v does not span the route part only", r.Range)
		}
		got = append(got, fmt.Sprintf("%d:%d", r.Range.Start.Line, r.Range.Start.Character))
	}
	sort.Strings(got)
	var want []string
	for _, w := range []Position{
		c.at(rel, "route Toggle", len("route ")), c.at(rel, "Toggle { id: t.id }", 0),
		c.at(rel, "Toggle.update :", 0), c.at(rel, "Toggle.Params ->", 0), c.at(rel, "Toggle.update params", 0),
	} {
		want = append(want, fmt.Sprintf("%d:%d", w.Line, w.Character))
	}
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("references %v, want %v", got, want)
	}
}

// `R.` offers a GET route's load, view, Params and Session, and a write route's update, Params and Session.
func TestComplete_RouteMembers(t *testing.T) {
	c, rel := routesCopy(t)
	const after = "  let _ <- Db.updateMine Todo params.id (\\t -> { t | done: not t.done })\n"
	p := typing(t, c, rel, after, "  let z = Row.")
	got := labels(complete(t, c, rel, p))
	for _, name := range []string{"load", "view", "Params", "Session"} {
		if _, ok := got[name]; !ok {
			t.Errorf("Row. lacks %s: %v", name, got)
		}
	}
	if it := got["view"]; it.Detail != "view : Todo -> Session -> Html" {
		t.Errorf("view detail %q", it.Detail)
	}
	p = typing(t, c, rel, after, "  let z = Toggle.")
	got = labels(complete(t, c, rel, p))
	if len(got) != 3 {
		t.Errorf("Toggle. offers %v, want update, Params and Session only", got)
	}
	for _, name := range []string{"update", "Params", "Session"} {
		if _, ok := got[name]; !ok {
			t.Errorf("Toggle. lacks %s: %v", name, got)
		}
	}
}

// Where markup expects a route, the module's route constructors complete, each with its route
// line as the detail.
func TestComplete_RouteInAttr(t *testing.T) {
	c, rel := routesCopy(t)
	p := typing(t, c, rel, "  <Layout.Page title=\"Todos\" theme={session.theme}>\n", "    <a href={To")
	got := labels(complete(t, c, rel, p))
	it, ok := got["Toggle"]
	if !ok {
		t.Fatalf("no route constructor in %v", got)
	}
	if !strings.Contains(it.Detail, `PATCH "/todos/{id}/toggle"`) || it.Kind != kindConstructor {
		t.Errorf("Toggle: %+v", it)
	}
	if _, ok := got["Index"]; !ok {
		t.Errorf("no Index in %v", got)
	}
}

// The routes list carries a route's method (the plugins read it, falling back to kind).
func TestRoutes_ListRoutes(t *testing.T) {
	c, _ := routesCopy(t)
	var out []routeItem
	if e := c.call("alder/routes", map[string]any{}, &out); e != nil {
		t.Fatalf("rpc error %+v", e)
	}
	found := map[string]routeItem{}
	for _, r := range out {
		if r.Module == "Todos" {
			found[r.Name] = r
		}
	}
	tg, ix := found["Toggle"], found["Index"]
	if tg.Method != "PATCH" || tg.Path != "/todos/{id}/toggle" || tg.Range.Start.Line != c.at("app/Todos.alder", "route Toggle", 0).Line ||
		!strings.HasSuffix(tg.URI, "app/Todos.alder") {
		t.Errorf("Toggle %+v", tg)
	}
	if ix.Method != "GET" || ix.Path != "/" {
		t.Errorf("Index %+v", ix)
	}
}

// insertEdit is the text of the "Insert ..." action offered on the line of needle.
func insertEdit(t *testing.T, c *client, rel, needle string) TextEdit {
	t.Helper()
	acts := actions(t, c, rel, c.at(rel, needle, 0).Line)
	var got *action
	for i, a := range acts {
		if strings.HasPrefix(a.Title, "Insert ") {
			got = &acts[i]
		}
	}
	if got == nil || got.Kind != "quickfix" || got.Edit == nil || len(got.Edit.DocumentChanges) != 1 || len(got.Edit.DocumentChanges[0].Edits) != 1 {
		t.Fatalf("%s: actions %+v", needle, acts)
	}
	return got.Edit.DocumentChanges[0].Edits[0]
}

// insertInto is text with the single insertion e made (ASCII text: character is a byte offset).
func insertInto(text string, e TextEdit) string {
	lines := strings.SplitAfter(text, "\n")
	off := 0
	for _, l := range lines[:e.Range.Start.Line] {
		off += len(l)
	}
	off += e.Range.Start.Character
	return text[:off] + e.NewText + text[off:]
}

// On a route line the code actions offer the signatures of its missing members, typed from the
// route and each followed by its definition with a hole: the GET pair (the model from a declared
// member), or the write signature. With no member yet they go right after the route's line.
func TestAction_InsertSignatures(t *testing.T) {
	c, rel := routesCopy(t)
	text := c.disk(rel)
	text += "\nroute Stats = GET \"/stats\" { } requires user\nroute Archive = PATCH \"/todos/{id}/archive\" { id: Todo.TodoId } requires user\n" +
		"route Count = GET \"/count\" { } requires user\n\nCount.load : Count.Params -> Session -> Query (List Todo)\nCount.load params session = Db.mine Todo |> Db.all\n"
	c.change(rel, text)
	c.diags(rel)
	want := map[string]string{
		"route Stats": "\n\nStats.load : Stats.Params -> Session -> Query m\nStats.load params session = ?\n\n" +
			"Stats.view : m -> Session -> Html\nStats.view model session = ?\n",
		"route Archive": "\n\nArchive.update : Archive.Params -> Session -> Task Response\nArchive.update params session = ?\n",
		"route Count":   "\n\nCount.view : List Todo -> Session -> Html\nCount.view model session = ?",
	}
	for needle, sigs := range want {
		if nt := insertEdit(t, c, rel, needle).NewText; nt != sigs {
			t.Errorf("%s: inserts %q, want %q", needle, nt, sigs)
		}
	}
	// the edit goes right after the route's line when it has no member yet, after the last member otherwise
	if e := insertEdit(t, c, rel, "route Stats"); e.Range.Start != (Position{Line: c.at(rel, "route Stats", 0).Line, Character: len("route Stats = GET \"/stats\" { } requires user")}) {
		t.Errorf("Stats edit at %+v", e.Range.Start)
	}
	// a route with all its members is offered nothing
	for _, a := range actions(t, c, rel, c.at(rel, "route Toggle", 0).Line) {
		if strings.HasPrefix(a.Title, "Insert ") {
			t.Errorf("complete route offered %q", a.Title)
		}
	}
}

// A file that ends in a blank line gets no second one, and with the inserted holes it builds with
// only the W0002 hole warnings (so fill-with-AI can take over).
func TestAction_InsertSignaturesBuilds(t *testing.T) {
	for name, tail := range map[string]string{
		"route last, file ends with a line break": "\nroute Count = GET \"/count\" { } requires user\n",
		"route last, file ends blank":             "\nroute Count = GET \"/count\" { } requires user\n\n",
		"route last, no final line break":         "\nroute Count = GET \"/count\" { } requires user",
		"write route":                             "\nroute Archive = PATCH \"/todos/{id}/archive\" { id: Todo.TodoId } requires user\n",
	} {
		t.Run(name, func(t *testing.T) {
			c, rel := routesCopy(t)
			text := strings.TrimRight(c.disk(rel), "\n") + "\n" + tail
			c.change(rel, text)
			c.diags(rel)
			needle := "route Count"
			if strings.Contains(name, "write") {
				needle = "route Archive"
			}
			got := insertInto(text, insertEdit(t, c, rel, needle))
			// the file's own trailing blank line stays; the edit adds none
			if strings.Count(got, "\n\n\n") > 0 || strings.HasSuffix(got, "\n\n") != strings.HasSuffix(text, "\n\n") {
				t.Errorf("a doubled blank line in %q", got[len(got)-120:])
			}
			c.change(rel, got)
			ds := c.diags(rel)
			holes := 0
			for _, d := range ds {
				if d.Code != "W0002" {
					t.Errorf("diagnostic %s %q after the insert", d.Code, d.Message)
				} else {
					holes++
				}
			}
			if holes == 0 {
				t.Errorf("no hole warnings in %+v", ds)
			}
		})
	}
}

// The route line's tokens: the `route` keyword, the name, the method, the path, the form and
// `requires` clause.
func TestSemtok_RouteLine(t *testing.T) {
	c, rel := routesCopy(t)
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", rel, nil)
	wantTok(t, c, toks, rel, "route Create", 0, "keyword")
	wantTok(t, c, toks, rel, "Create = POST", 0, "enumMember")
	wantTok(t, c, toks, rel, "POST", 0, "enumMember")
	wantTok(t, c, toks, rel, `"/todos"`, 0, "string")
	wantTok(t, c, toks, rel, "form NewTodo", 0, "keyword")
	wantTok(t, c, toks, rel, "form NewTodo", len("form "), "enumMember") // as the form of a `msg` is
	wantTok(t, c, toks, rel, "requires user", 0, "keyword")
	wantTok(t, c, toks, rel, "requires user", len("requires "), "property")
	wantTok(t, c, toks, rel, "id: Todo.TodoId }  ", 0, "property")
	wantTok(t, c, toks, rel, "Toggle.update :", 0, "namespace")
	wantTok(t, c, toks, rel, "Toggle.update :", len("Toggle."), "function", "declaration")
}

// A module that imports a route module is offered its full name (`Posts.Index`) as well as the
// short qualifier: a route value keeps its full path (rule 6 in complete.go). This pins the
// behaviour kept on purpose (#205).
func TestComplete_RouteModuleFullPath(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "app/Write/Edit.alder"
	openClean(t, c, rel)
	p := typing(t, c, rel, "Edit.load params session = {\n", "  let z = Pos")
	got := labels(complete(t, c, rel, p))
	it, ok := got["Posts.Index"]
	if !ok {
		t.Fatalf("no Posts.Index in %v", got)
	}
	if it.Kind != kindModule {
		t.Errorf("Posts.Index kind %d, want a module", it.Kind)
	}
}
