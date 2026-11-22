package lsp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alder/internal/names"
	"alder/internal/syntax"
	"alder/stdlib"
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
	// a documented module: the line, then its doc
	c, rel := stdUseCopy(t)
	got := hover(t, c, rel, c.at(rel, "List.take 2", 0))
	if !strings.HasPrefix(got, "`List` · stdlib/List.alder") || !strings.Contains(got, "Functions for working with lists: building, slicing, searching, transforming and sorting them.") {
		t.Fatalf("List qualifier hover %q", got)
	}
}

// stdUseCopy is the blog with Posts.Index calling List.take and List.drop, opened.
func stdUseCopy(t *testing.T) (*client, string) {
	t.Helper()
	c := start(t, blogCopy(t))
	rel := "app/Posts/Index.alder"
	text := strings.Replace(c.disk(rel), "perPage = 10", "perPage = 10\n\nfirstTwo : List Int\nfirstTwo = List.take 2 [1, 2, 3]\n\nrest : List Int\nrest = List.drop 1 [1, 2, 3]", 1)
	c.open(rel, text)
	c.diags(rel)
	return c, rel
}

func TestHover_StdDoc(t *testing.T) {
	c, rel := stdUseCopy(t)
	got := hover(t, c, rel, c.at(rel, "take 2", 0))
	sig := strings.Index(got, "```alder\ntake : Int -> List a -> List a\n```")
	summary := strings.Index(got, "The first n elements of a list.")
	if sig < 0 || summary < sig || !strings.Contains(got, "```alder\nList.take 2 [1, 2, 3] == [1, 2]") || !strings.Contains(got, `*Book: chapter 9, "List"*`) {
		t.Fatalf("hover %q", got)
	}
}

// TestHover_StdModuleBook: List.drop's doc has no Book line, so it shows its module's.
func TestHover_StdModuleBook(t *testing.T) {
	c, rel := stdUseCopy(t)
	got := hover(t, c, rel, c.at(rel, "drop 1", 0))
	if !strings.Contains(got, "List.drop 1 [1, 2, 3] == [2, 3]") || !strings.Contains(got, `*Book: chapter 9, "List"*`) {
		t.Fatalf("hover %q", got)
	}
}

// TestHover_AppDocNoBook: an app declaration without a Book line shows none.
func TestHover_AppDocNoBook(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "lastPage then", 0))
	if !strings.Contains(got, "lastPage bounds `page`") || strings.Contains(got, "Book:") {
		t.Fatalf("hover %q", got)
	}
}

// TestHover_HelperLineAboveDoc: a documented test helper's note comes before its doc.
func TestHover_HelperLineAboveDoc(t *testing.T) {
	c := start(t, helpersCopy(t))
	openClean(t, c, "app/Helpers.alder")
	got := hover(t, c, "app/Helpers.alder", c.at("app/Helpers.alder", "fetchText cfg = {", 0))
	note, doc := strings.Index(got, "test helper (uses "), strings.Index(got, "A test helper: it uses")
	if note < 0 || doc < note {
		t.Fatalf("hover %q", got)
	}
}

// TestHover_AppExamples: an indented line in an app's doc comment shows as a fenced alder block.
func TestHover_AppExamples(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Index.alder"
	text := strings.Replace(c.disk(rel), "perPage = 10", "perPage = 10\n\n-- Twice a number.\n--\n--     double 2 == 4\ndouble : Int -> Int\ndouble n = n * 2", 1)
	c.open(rel, text)
	c.diags(rel)
	got := hover(t, c, rel, c.at(rel, "double n =", 0))
	if !strings.Contains(got, "Twice a number.\n\n```alder\ndouble 2 == 4\n```") {
		t.Fatalf("hover %q", got)
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
		{"Index.load params session", len("Index.load "), "params : Index.Params"},
		{"Index.load params session", len("Index."), "Index.load : Index.Params -> App.Session -> Query Model"},
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

// A type inside a route's header does not hover as the route.
func TestHover_RouteHeaderType(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "q: Text", len("q: ")))
	if strings.Contains(got, "GET") {
		t.Errorf("Text in the header hovered as the route:\n%s", got)
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

// A route's hover shows the query key of each parameter declared `as "Key"` (#55).
func TestHover_RouteRefWire(t *testing.T) {
	c := start(t, wireBlog(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "Index { page: 1, q: p.form.q }", 0))
	if want := `{ page: Int = 1, q: Text = "" as "Q" }`; !strings.Contains(got, want) {
		t.Errorf("route hover lacks %q:\n%s", want, got)
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

func TestHover_CustomElement(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Write.alder")
	got := hover(t, c, "app/Write.alder", c.at("app/Write.alder", "<al-button type", 1))
	i, j, k := strings.Index(got, "`<al-button>` — "), strings.Index(got, "\n\n**Attributes**\n- `variant`: `"), strings.Index(got, "\n\n**Slots**: default")
	if i != 0 || j < 0 || k < j {
		t.Fatalf("hover %q", got)
	}
	if !strings.Contains(got, "= `\"primary\"` — ") {
		t.Fatalf("variant line missing its default: %q", got)
	}
}

func TestHover_CustomAttr(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Write.alder")
	got := hover(t, c, "app/Write.alder", c.at("app/Write.alder", `variant="outline"`, 1))
	if !strings.HasPrefix(got, "`variant`: `\"primary\" | ") || !strings.Contains(got, " = `\"primary\"` — ") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_DivAndComponentUnchanged(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "<div class", 1))
	if !strings.Contains(got, "`<div>` — ") || strings.Contains(got, "**Attributes**") {
		t.Fatalf("div hover %q", got)
	}
	if got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", `rel="prev"`, 0)); !strings.HasPrefix(got, "`rel` — ") {
		t.Fatalf("attr hover %q", got)
	}
	got = hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "<Layout.Page title", len("<Layout.")))
	if !strings.HasPrefix(got, "```alder\nPage : { title: Text") || strings.Contains(got, "**Attributes**") {
		t.Fatalf("component hover %q", got)
	}
}

// TestRouteFile_Answers: the editor works on a route module (#160) without failing a request:
// diagnostics are clean, and hover, definition, references, completion, semantic tokens and
// code actions answer on a route constructor, a route member and a route's type. Their full
// support for routes is PR 5's.
func TestRouteFile_Answers(t *testing.T) {
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
	doc := map[string]any{"uri": c.uri(rel)}
	for _, p := range []Position{
		c.at(rel, "Toggle { id: t.id }", 0),           // a route constructor
		c.at(rel, "Row.view t session}", len("Row.")), // a route member
		c.at(rel, "Row.Params ->", len("Row.")),       // a route's type
		c.at(rel, "route Row", len("route ")),         // a route's declaration
		c.at(rel, "Index.load params", len("Index.")), // a member's definition
	} {
		hover(t, c, rel, p)
		definitionAt(t, c, rel, p)
		var out any
		for method, params := range map[string]map[string]any{
			"textDocument/references": {"textDocument": doc, "position": p, "context": map[string]any{"includeDeclaration": true}},
			"textDocument/completion": {"textDocument": doc, "position": p},
			"textDocument/codeAction": {"textDocument": doc, "range": map[string]any{"start": p, "end": p}, "context": map[string]any{"diagnostics": []any{}}},
		} {
			if e := c.call(method, params, &out); e != nil {
				t.Fatalf("%s at %+v: %v", method, p, e)
			}
		}
	}
	var out any
	if e := c.call("textDocument/semanticTokens/full", map[string]any{"textDocument": doc}, &out); e != nil {
		t.Fatalf("semantic tokens: %v", e)
	}
}

// TestDeclDoc_StdTypeAndCtorBook: a stdlib type or constructor with no Book line of its own shows
// its module's, as a function does. The made-up module keeps the case small; the Prelude test
// below runs it on the real file.
func TestDeclDoc_StdTypeAndCtorBook(t *testing.T) {
	src := "-- Optional values.\n--\n-- Book: chapter 9, \"Maybe\"\nmodule Opt\n\n-- A value that may be missing.\ntype Opt a = Some a | None\n"
	mod, diags := syntax.ParseFile("stdlib/Opt.alder", []byte(src))
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	a := &analysis{prog: &names.Program{Std: map[string]*syntax.Module{"Opt": mod}}}
	for _, ref := range []syntax.Ref{{Kind: syntax.RefType, Module: "Opt", Name: "Opt"}, {Kind: syntax.RefCtor, Module: "Opt", Name: "Some"}} {
		if got := declDoc(a, ref); !strings.Contains(got, `*Book: chapter 9, "Maybe"*`) {
			t.Errorf("%s: %q", ref.Name, got)
		}
	}
}

// TestDeclDoc_PreludeBook: in the real Prelude, `Unit` and `Bool` have no Book line of their own, so
// hovering the type or the constructor `True` shows the module's; `Maybe` and `Just` show the
// declaration's own.
func TestDeclDoc_PreludeBook(t *testing.T) {
	data, err := stdlib.FS.ReadFile("Prelude.alder")
	if err != nil {
		t.Fatal(err)
	}
	mod, diags := syntax.ParseSignatures("stdlib/Prelude.alder", data)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	a := &analysis{prog: &names.Program{Std: map[string]*syntax.Module{"Prelude": mod}}}
	for _, c := range []struct {
		ref  syntax.Ref
		book string
	}{
		{syntax.Ref{Kind: syntax.RefType, Module: "Prelude", Name: "Unit"}, `*Book: chapter 4, "Built-in types"*`},
		{syntax.Ref{Kind: syntax.RefType, Module: "Prelude", Name: "Bool"}, `*Book: chapter 4, "Built-in types"*`},
		{syntax.Ref{Kind: syntax.RefCtor, Module: "Prelude", Name: "True"}, `*Book: chapter 4, "Built-in types"*`},
		{syntax.Ref{Kind: syntax.RefType, Module: "Prelude", Name: "Maybe"}, `*Book: chapter 9, "Maybe"*`},
		{syntax.Ref{Kind: syntax.RefCtor, Module: "Prelude", Name: "Just"}, `*Book: chapter 9, "Maybe"*`},
	} {
		if got := declDoc(a, c.ref); !strings.Contains(got, c.book) {
			t.Errorf("%s: want %s in %q", c.ref.Name, c.book, got)
		}
	}
}

func TestHover_Operator(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "|> Db.where (Post.published", 1))
	if !strings.Contains(got, "```alder\n(|>) : a -> (a -> b) -> b\n```") || !strings.Contains(got, `*Book: chapter 3, "Operators"*`) {
		t.Fatalf("hover %q", got)
	}
	if got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "* perPage)", 0)); !strings.Contains(got, "(*) : number -> number -> number") {
		t.Fatalf("hover on * %q", got)
	}
}

func TestHover_OperatorInInterpolation(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), `then "" else`, `then "{Text.fromInt (perPage + 1)}" else`, 1)
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	if got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "+ 1)}", 0)); !strings.Contains(got, "(+) : number -> number -> number") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_Keyword(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Write.alder")
	if got := hover(t, c, "app/Write.alder", c.at("app/Write.alder", "match taken", 2)); !strings.Contains(got, `*Book: chapter 3, "Match"*`) {
		t.Fatalf("hover on match %q", got)
	}
	if got := hover(t, c, "app/Write.alder", c.at("app/Write.alder", "<- Post.bySlug", 0)); !strings.Contains(got, `*Book: chapter 7, "Building effects"*`) {
		t.Fatalf("hover on <- %q", got)
	}
	if got := hover(t, c, "app/Write.alder", c.at("app/Write.alder", "route Create", 0)); !strings.Contains(got, `*Book: chapter 6, "`+"`route`"+`"*`) {
		t.Fatalf("hover on route %q", got)
	}
}

func TestHover_KeywordAsAttribute(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), "<h1>Blog</h1>", `<h1>Blog</h1><label for="q">Q</label>`, 1)
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	if got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", `for="q"`, 0)); strings.Contains(got, "Book: chapter 5") {
		t.Fatalf("the attribute `for` hovered as the keyword: %q", got)
	}
}

func TestHover_HoleDoc(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), "Db.limit (perPage + 1)", "Db.limit ?n", 1)
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "?n", 1))
	if !strings.HasPrefix(got, "```alder\n?n : Int\n```\n\n") || !strings.Contains(got, "alder fill") {
		t.Fatalf("hover %q", got)
	}
}

func TestHover_OperatorInString(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), `then "" else`, `then "a |> b" else`, 1)
	text = strings.Replace(text, "perPage : Int", "-- perPage |> pages\nperPage : Int", 1)
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	for _, at := range []Position{
		c.at("app/Posts/Index.alder", "|> b\"", 0),   // in a text literal
		c.at("app/Posts/Index.alder", "|> pages", 0), // in a comment
		c.at("app/Posts/Index.alder", "<h1>Blog", 0), // `<` opening an element
		c.at("app/Posts/Index.alder", "</h1>", 1),    // `/` closing one
	} {
		if got := hover(t, c, "app/Posts/Index.alder", at); strings.Contains(got, "Book: chapter 3, \"Operators\"") {
			t.Errorf("operator hover at %v: %q", at, got)
		}
	}
}

// jobsCopy is examples/todos with a job that has parameters and options, app/Jobs/SendWelcome.alder.
func jobsCopy(t *testing.T) string {
	t.Helper()
	root := copyExample(t, "todos")
	src := "module Jobs.SendWelcome\n\njob SendWelcome = { email: Text } retries 2 timeout \"30s\"\n\nSendWelcome.run : SendWelcome.Params -> Task Unit\nSendWelcome.run p = Log.info p.email\n\ncron : Int\ncron = 1\n\nwelcome : Task Unit\nwelcome = {\n  let _ <- Job.enqueue (SendWelcome { email: \"a@b.c\" })\n  Task.succeed ()\n}\n"
	if err := os.WriteFile(filepath.Join(root, "app", "Jobs", "SendWelcome.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// The `job` keyword and a job's option words hover with their langdoc entries; an option word
// used as a name elsewhere is a name.
func TestHover_JobKeywords(t *testing.T) {
	c := start(t, jobsCopy(t))
	const nightly, welcome = "app/Jobs/Nightly.alder", "app/Jobs/SendWelcome.alder"
	openClean(t, c, nightly)
	openClean(t, c, welcome)
	if got := hover(t, c, nightly, c.at(nightly, "job Nightly", 1)); !strings.Contains(got, "Declare a background job") {
		t.Fatalf("hover on job %q", got)
	}
	if got := hover(t, c, nightly, c.at(nightly, `cron "0 3`, 2)); !strings.Contains(got, "five fields") {
		t.Fatalf("hover on cron %q", got)
	}
	for _, w := range []string{"retries", "timeout"} {
		if got := hover(t, c, welcome, c.at(welcome, w+" ", 0)); !strings.Contains(got, "*Book: ") || strings.Contains(got, "```alder") {
			t.Fatalf("hover on %s %q", w, got)
		}
	}
	if got := hover(t, c, welcome, c.at(welcome, "cron : Int", 0)); !strings.Contains(got, "cron : Int") || strings.Contains(got, "five fields") {
		t.Fatalf("hover on the value cron %q", got)
	}
}

// A job's name and its constructor's uses go to the job and find each other, as a route's do.
func TestDefinition_Job(t *testing.T) {
	c := start(t, jobsCopy(t))
	const rel = "app/Jobs/SendWelcome.alder"
	openClean(t, c, rel)
	decl := c.at(rel, "SendWelcome = {", 0)
	for _, p := range []Position{c.at(rel, "SendWelcome { email", 0), c.at(rel, "SendWelcome.run p", 0)} {
		if locs := definitionAt(t, c, rel, p); len(locs) != 1 || locs[0].Range.Start != decl {
			t.Fatalf("definition at %+v: %+v, want %+v", p, locs, decl)
		}
	}
	var lines []int
	for _, l := range refsAt(t, c, rel, decl, true) {
		lines = append(lines, l.Range.Start.Line)
	}
	sig := c.at(rel, "SendWelcome.run :", 0).Line // `SendWelcome.run` and `SendWelcome.Params`
	want := []int{decl.Line, c.at(rel, "SendWelcome { email", 0).Line, sig, sig, c.at(rel, "SendWelcome.run p", 0).Line}
	slices.Sort(lines)
	slices.Sort(want)
	if !slices.Equal(lines, want) {
		t.Fatalf("references on lines %v, want %v", lines, want)
	}
	if got := hover(t, c, rel, c.at(rel, "SendWelcome { email", 0)); !strings.Contains(got, "SendWelcome.Params -> Job") {
		t.Fatalf("hover on the constructor %q", got)
	}
}

// In an app with silos, `every` and `silo` hover with their entries in a job's line, and `silo`
// in a route's `requires …, silo k` or in App's `silo k { … }` is not a job option.
func TestHover_JobOptionsOnlyInAJob(t *testing.T) {
	root := copyExample(t, "workspace")
	src := "module Jobs.Digest\n\nimport Table.Org\n\njob Digest = { org: Org.OrgId } silo org\n\nDigest.run : Digest.Params -> Task Unit\nDigest.run p = Log.info \"digest\"\n\njob Tick = {} every \"1h\"\n\nTick.run : Tick.Params -> Task Unit\nTick.run p = Log.info \"tick\"\n"
	if err := os.MkdirAll(filepath.Join(root, "app", "Jobs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "Jobs", "Digest.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c := start(t, root)
	const job, app, route = "app/Jobs/Digest.alder", "app/App.alder", "app/Admin/Projects/New.alder"
	openClean(t, c, job)
	openClean(t, c, app)
	openClean(t, c, route)
	if got := hover(t, c, job, c.at(job, "silo org", 0)); !strings.Contains(got, "run the job in the silo") {
		t.Fatalf("hover on silo in a job %q", got)
	}
	if got := hover(t, c, job, c.at(job, `every "1h"`, 0)); !strings.Contains(got, "at a fixed interval") {
		t.Fatalf("hover on every %q", got)
	}
	for _, at := range []struct{ rel, needle string }{{app, "silo org {"}, {route, "silo org"}} {
		if got := hover(t, c, at.rel, c.at(at.rel, at.needle, 0)); strings.Contains(got, "job") {
			t.Fatalf("%s: hover on %q shows the job option: %q", at.rel, at.needle, got)
		}
	}
}
