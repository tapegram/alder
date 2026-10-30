package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hover(t *testing.T, c *client, rel string, p Position) string {
	t.Helper()
	var h *struct {
		Contents struct{ Kind, Value string } `json:"contents"`
	}
	if e := c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}, "position": p}, &h); e != nil {
		t.Fatal(e)
	}
	if h == nil {
		return ""
	}
	return h.Contents.Value
}

func openClean(t *testing.T, c *client, rel string) string {
	text := c.disk(rel)
	c.open(rel, text)
	c.diags(rel)
	return text
}

func TestHover_DeclWithDoc(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "lastPage then", 0))
	if !strings.Contains(got, "```alder\nlastPage : Int\n```") || !strings.Contains(got, "lastPage bounds `page`") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_Local(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "Text.isEmpty q", len("Text.isEmpty ")))
	if !strings.Contains(got, "q : Text") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_Hole(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), "Db.limit (perPage + 1)", "Db.limit ?n", 1)
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	if got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "?n", 1)); !strings.Contains(got, "?n : Int") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_Module(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "import Table.Post", len("import ")))
	if !strings.Contains(got, "app/Table/Post.alder") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_StaleBelowEdit(t *testing.T) {
	c := start(t, blogCopy(t))
	text := openClean(t, c, "app/Posts/Index.alder")
	p := c.at("app/Posts/Index.alder", "lastPage then", 0)
	broken := strings.Replace(text, "perPage = 10", "perPage = \"oops", 1) // a parse error above the hover point
	c.change("app/Posts/Index.alder", broken)
	c.diags("app/Posts/Index.alder")
	if got := hover(t, c, "app/Posts/Index.alder", p); got != "" {
		t.Fatalf("stale hover answered %q", got)
	}
	if got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "import Table.Post", len("import "))); got == "" {
		t.Fatal("hover above the edit did not answer")
	}
	if got := hover(t, c, "app/Posts/Index.alder", Position{Line: 99999, Character: 3}); got != "" {
		t.Fatalf("past the end: %q", got)
	}
}

func TestHover_MultiByte(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), "perPage = 10", "perPage = 10\n\nlabel : Text\nlabel = \"é\" ++ Text.trim \"x\"", 1)
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "trim \"x\"", 0))
	if !strings.Contains(got, "trim") || !strings.Contains(got, "Text -> Text") {
		t.Fatalf("hover after é: %q", got)
	}
}

func TestDefinition(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	var locs []Location
	c.call("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": c.uri("app/Posts/Index.alder")}, "position": c.at("app/Posts/Index.alder", "lastPage then", 0)}, &locs)
	if len(locs) != 1 || locs[0].Range.Start != c.at("app/Posts/Index.alder", "lastPage : Int", 0) {
		t.Fatalf("definition %+v", locs)
	}
	c.call("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": c.uri("app/Posts/Index.alder")}, "position": c.at("app/Posts/Index.alder", "import Table.User", len("import "))}, &locs)
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "app/Table/User.alder") {
		t.Fatalf("module definition %+v", locs)
	}
	// a local: `q` in `Text.isEmpty q` goes to `let q =`
	c.call("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": c.uri("app/Posts/Index.alder")}, "position": c.at("app/Posts/Index.alder", "Text.isEmpty q", len("Text.isEmpty "))}, &locs)
	if len(locs) != 1 || locs[0].Range.Start.Line != c.at("app/Posts/Index.alder", "let q =", 0).Line {
		t.Fatalf("local definition %+v", locs)
	}
}

func definitionAt(t *testing.T, c *client, rel string, p Position) []Location {
	t.Helper()
	var locs []Location
	if e := c.call("textDocument/definition", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}, "position": p}, &locs); e != nil {
		t.Fatal(e)
	}
	return locs
}

func TestHover_Ctor(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "Author\n", 0))
	if !strings.Contains(got, "Author : ") || !strings.Contains(got, "Role") {
		t.Fatalf("hover %q", got)
	}
	locs := definitionAt(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "Author\n", 0))
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "app/Table/User.alder") || locs[0].Range.Start.Line != 8 {
		t.Fatalf("ctor definition %+v", locs)
	}
}

// TestHover_Qualifier: on the `Text` of `Text.isEmpty` hover shows the stdlib module, and on a
// stdlib name definition answers nothing (Ruling R12).
func TestHover_Qualifier(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := c.at("app/Posts/Index.alder", "Text.isEmpty q", 0)
	if got := hover(t, c, "app/Posts/Index.alder", p); !strings.Contains(got, "`Text` · stdlib/Text.alder") {
		t.Fatalf("hover %q", got)
	}
	p.Character += len("Text.")
	if locs := definitionAt(t, c, "app/Posts/Index.alder", p); len(locs) != 0 {
		t.Fatalf("stdlib definition %+v", locs)
	}
}

func TestDefinition_Field(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	locs := definitionAt(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "post.slug", len("post.")))
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "app/Table/Post.alder") || locs[0].Range != (Range{Position{12, 2}, Position{12, 6}}) {
		t.Fatalf("field definition %+v", locs)
	}
}

// TestHover_BindingSites: a let, a `for` variable, a parameter and a value's definition line
// are hovered (and gone to) on their own names.
func TestHover_BindingSites(t *testing.T) {
	c := start(t, blogCopy(t))
	f := "app/Posts/Index.alder"
	openClean(t, c, f)
	for _, tc := range []struct {
		needle string
		off    int
		want   string
	}{
		{"let found <-", len("let "), "found : List Post"},
		{"post in model", 0, "post : Post"},
		{"load params session", len("load "), "params : Params"},
		{"load params session", 0, "load : Params -> App.Session -> Query Model"},
	} {
		p := c.at(f, tc.needle, tc.off)
		if got := hover(t, c, f, p); !strings.Contains(got, tc.want) {
			t.Errorf("hover at %q+%d: %q, want %q", tc.needle, tc.off, got, tc.want)
		}
		if locs := definitionAt(t, c, f, p); len(locs) != 1 {
			t.Errorf("definition at %q+%d: %+v", tc.needle, tc.off, locs)
		}
	}
}

// TestDefinition_StaleTarget: when the file a definition lands in has unsaved lines inserted
// above the declaration since the last good build (here a broken buffer), definition answers
// nothing rather than the old place; a module still goes to its start.
func TestDefinition_StaleTarget(t *testing.T) {
	c := start(t, blogCopy(t))
	f := "app/Posts/Index.alder"
	openClean(t, c, f)
	for _, other := range []string{"app/Table/User.alder", "app/Table/Post.alder"} {
		text := c.disk(other)
		i := strings.Index(text, "\n") + 1
		c.open(other, text[:i]+"\n\nbroken = \"oops\n"+text[i:])
		c.diags(other)
	}
	for _, tc := range []struct {
		needle string
		off    int
	}{{"Author\n", 0}, {"post.slug", len("post.")}} {
		if locs := definitionAt(t, c, f, c.at(f, tc.needle, tc.off)); len(locs) != 0 {
			t.Errorf("definition at %q into a stale file: %+v", tc.needle, locs)
		}
	}
	if locs := definitionAt(t, c, f, c.at(f, "import Table.User", len("import "))); len(locs) != 1 || locs[0].Range != (Range{}) {
		t.Errorf("module definition %+v", locs)
	}
}

func TestHover_Tag(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "<div class", 1))
	if !strings.Contains(got, "`<div>`") || !strings.Contains(got, "generic block container") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_Attr(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", `rel="prev"`, 0))
	if !strings.Contains(got, "`rel` — how the linked resource relates") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_PageHooks(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	cases := []struct{ needle, sig, role string }{
		{"load params session", "load : Params -> App.Session -> Query Model", "Page hook. Runs on every request"},
		{"view model session", "view : Model -> App.Session -> Html Msg", "Page hook. Renders"},
		{"update msg model session", "update : Msg -> Model -> App.Session -> Task Nav", "`Rerender` runs `load`"},
	}
	for _, tc := range cases {
		got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", tc.needle, 0))
		if !strings.Contains(got, "```alder\n"+tc.sig+"\n```") || !strings.Contains(got, tc.role) {
			t.Errorf("hover on %q:\n%s", tc.needle, got)
		}
	}
	// a value that is not a hook says nothing about hooks
	if got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "lastPage then", 0)); strings.Contains(got, "Page hook") {
		t.Errorf("lastPage is not a hook:\n%s", got)
	}
}

func TestHover_PageHooks_NotInAPage(t *testing.T) {
	c := start(t, blogCopy(t))
	// Post is a table module, not a page: a value called `view` there is an ordinary value
	text := c.disk("app/Table/Post.alder") + "\nview : Int\nview = 1\n"
	c.open("app/Table/Post.alder", text)
	c.diags("app/Table/Post.alder")
	got := hover(t, c, "app/Table/Post.alder", c.at("app/Table/Post.alder", "view = 1", 0))
	if strings.Contains(got, "Page hook") {
		t.Errorf("view outside a page:\n%s", got)
	}
}

func TestHover_PageHeader(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", `page "/"`, 0))
	for _, want := range []string{"```alder\npage \"/\"", "params : { page: Int, q: Text }", "hooks"} {
		if !strings.Contains(got, want) {
			t.Errorf("page header hover lacks %q:\n%s", want, got)
		}
	}
	// a type inside the header still hovers as the type
	got = hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "q: Text", len("q: ")))
	if strings.Contains(got, "hooks") {
		t.Errorf("Text in the header hovered as the page:\n%s", got)
	}
}

func TestHover_HookNameOutsideAPage(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Write.alder")
	// `update` in `Db.update` names the stdlib function in a page module: a plain signature, no hook role
	got := hover(t, c, "app/Write.alder", c.at("app/Write.alder", "update User author", 0))
	if !strings.Contains(got, "```alder\nupdate : Table t -> Id t -> (t -> t) -> Task Unit\n```") || strings.Contains(got, "Page hook") {
		t.Errorf("Db.update hovered as a hook or not at all:\n%s", got)
	}
}

// qualCopy is the blog plus a module Table.Todo, which Posts.Index imports (written by its
// qualifier `Todo`) and calls through `Todo.find`.
func qualCopy(t *testing.T, use string) (*client, string) {
	t.Helper()
	root := blogCopy(t)
	if err := os.MkdirAll(filepath.Join(root, "app/Table"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app/Table/Todo.alder"), []byte("module Table.Todo\n\nfind : Int -> Int\nfind n = n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := "app/Posts/Index.alder"
	p := filepath.Join(root, rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(b), "import Layout\n", "import Layout\nimport Table.Todo\n", 1)
	text += "\nuseTodo : Int\nuseTodo = " + use + " 1\n"
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return start(t, root), rel
}

func TestHover_QualifiedShort(t *testing.T) {
	c, rel := qualCopy(t, "Todo.find")
	openClean(t, c, rel)
	p := c.at(rel, "Todo.find 1", len("Todo."))
	if got := hover(t, c, rel, p); !strings.Contains(got, "Table.Todo.find : Int -> Int") {
		t.Fatalf("hover %q", got)
	}
	// on the qualifier: the module, by its full path
	p.Character = c.at(rel, "Todo.find 1", 0).Character
	if got := hover(t, c, rel, p); !strings.Contains(got, "`Table.Todo` · app/Table/Todo.alder") {
		t.Fatalf("qualifier hover %q", got)
	}
}

func TestDefinition_Qualifier(t *testing.T) {
	c, rel := qualCopy(t, "Todo.find")
	openClean(t, c, rel)
	locs := definitionAt(t, c, rel, c.at(rel, "Todo.find 1", 1))
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "app/Table/Todo.alder") || locs[0].Range.Start.Line != 0 {
		t.Fatalf("qualifier definition %+v", locs)
	}
	locs = definitionAt(t, c, rel, c.at(rel, "Todo.find 1", len("Todo.")))
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "app/Table/Todo.alder") || locs[0].Range.Start.Line != 2 {
		t.Fatalf("member definition %+v", locs)
	}
}

// qualTypeCopy is qualCopy with a type and a brand in Table.Todo, used through the qualifier.
func qualTypeCopy(t *testing.T, use string) (*client, string) {
	t.Helper()
	root := blogCopy(t)
	if err := os.MkdirAll(filepath.Join(root, "app/Table"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app/Table/Todo.alder"), []byte("module Table.Todo\n\ntype Status = Open | Done\n\ntype Tag = brand Text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := "app/Posts/Index.alder"
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(b), "import Layout\n", "import Layout\nimport Table.Todo\n", 1) + use
	if err := os.WriteFile(filepath.Join(root, rel), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return start(t, root), rel
}

// A type or constructor reached through a qualifier hovers with its module's full path, as a
// value does.
func TestHover_QualifiedTypeAndCtor(t *testing.T) {
	c, rel := qualTypeCopy(t, "\nuseStatus : Todo.Status\nuseStatus = Open\n\nuseTag : Todo.Tag\nuseTag = Todo.Tag \"x\"\n")
	openClean(t, c, rel)
	for _, w := range []struct {
		at       Position
		contains string
	}{
		{c.at(rel, "useStatus : Todo.Status", len("useStatus : Todo.")), "type Table.Todo.Status"},
		{c.at(rel, "useStatus = Open", len("useStatus = ")), "Open : Todo.Status"},
		{c.at(rel, "Todo.Tag \"x\"", len("Todo.")), "Todo.Tag : Text -> Todo.Tag"},
	} {
		if got := hover(t, c, rel, w.at); !strings.Contains(got, w.contains) {
			t.Errorf("hover at %+v = %q, want %q", w.at, got, w.contains)
		}
	}
}

// wireBlog is the blog with Posts.Index's `q` read from the query key "Q" (#55).
func wireBlog(t *testing.T) string {
	t.Helper()
	root := blogCopy(t)
	path := filepath.Join(root, "app/Posts/Index.alder")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Replace(string(src), `q: Text = "" }`, `q: Text = "" as "Q" }`, 1)
	if out == string(src) {
		t.Fatal("Posts.Index's header changed")
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A page header's hover names the query key of each parameter declared `as "Key"` (#55).
func TestHover_PageHeaderWire(t *testing.T) {
	c := start(t, wireBlog(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", `page "/"`, 0))
	for _, want := range []string{"params : { page: Int, q: Text }", "-- q is read from the query key \"Q\""} {
		if !strings.Contains(got, want) {
			t.Errorf("page header hover lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "page is read") {
		t.Errorf("a parameter without a wire name is listed:\n%s", got)
	}
}

// A type written in a lambda parameter's annotation is a use like any other: hover, definition
// and references work on it.
func TestHover_LambdaAnnotationType(t *testing.T) {
	c := start(t, blogCopy(t))
	f := "app/Posts/Index.alder"
	text := c.disk(f) + "\nsame : List Post -> List Post\nsame = List.map (\\(p : Post) -> p)\n"
	c.open(f, text)
	if ds := c.diags(f); len(ds) != 0 {
		t.Fatalf("diagnostics %+v", ds)
	}
	p := c.at(f, "(p : Post)", len("(p : "))
	for _, at := range []Position{c.at(f, "(p : Post)", 1), c.at(f, "-> p)", len("-> "))} { // the parameter, and its use
		if got := hover(t, c, f, at); !strings.Contains(got, "p : Post") {
			t.Errorf("hover on the annotated parameter: %q", got)
		}
	}
	locs := definitionAt(t, c, f, p)
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "app/Table/Post.alder") {
		t.Errorf("definition on the annotation: %+v", locs)
	}
	found := false
	for _, l := range refsAt(t, c, f, c.at(f, "List Post", len("List ")), false) {
		found = found || l.Range.Start == p
	}
	if !found {
		t.Errorf("the annotation's Post is not among the references")
	}
}
