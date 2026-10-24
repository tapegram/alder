package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/buildkit"
	"alder/internal/syntax"
	"alder/internal/types"
)

type item struct {
	Label, Detail, SortText string
	Kind                    int
	TextEdit                struct {
		Range   Range
		NewText string
	}
}

func complete(t *testing.T, c *client, rel string, p Position) []item {
	t.Helper()
	var out struct{ Items []item }
	if e := c.call("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}, "position": p}, &out); e != nil {
		t.Fatal(e)
	}
	return out.Items
}

func labels(items []item) map[string]item {
	m := map[string]item{}
	for _, i := range items {
		m[i.Label] = i
	}
	return m
}

// typing inserts typed as a line of its own after needle (which ends a line) and returns the
// position at the end of the typed text.
func typing(t *testing.T, c *client, rel, needle, typed string) Position {
	t.Helper()
	text := c.texts[rel]
	i := strings.Index(text, needle)
	if i < 0 {
		t.Fatalf("no %q", needle)
	}
	at := i + len(needle)
	text = text[:at] + typed + "\n" + text[at:]
	c.change(rel, text)
	end := at + len(typed)
	return Position{Line: strings.Count(text[:end], "\n"), Character: end - (strings.LastIndex(text[:end], "\n") + 1)}
}

const afterQ = "let q = Text.slice 0 100 (Text.trim params.q)\n"

func TestComplete_ModuleMembers(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := typing(t, c, "app/Posts/Index.alder", afterQ, "  let z = Text.")
	got := labels(complete(t, c, "app/Posts/Index.alder", p))
	it, ok := got["trim"]
	if !ok {
		t.Fatalf("Text. members: %v", got)
	}
	if it.Kind != 3 || it.Detail != "trim : Text -> Text" || it.TextEdit.Range.Start != p || it.TextEdit.NewText != "trim" {
		t.Fatalf("trim: %+v", it)
	}
}

func TestComplete_RecordFields(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := typing(t, c, "app/Posts/Index.alder", afterQ, "  let z = params.")
	got := labels(complete(t, c, "app/Posts/Index.alder", p))
	if _, ok := got["page"]; !ok {
		t.Fatalf("params. fields: %v", got)
	}
	if got["page"].Detail != "page : Int" || got["page"].Kind != 5 {
		t.Fatalf("page: %+v", got["page"])
	}
}

func TestComplete_Imports(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := typing(t, c, "app/Posts/Index.alder", "import Layout\n", "import ")
	got := labels(complete(t, c, "app/Posts/Index.alder", p))
	for _, want := range []string{"Comment", "Markdown", "Maybe"} {
		if it, ok := got[want]; !ok || it.Kind != 9 {
			t.Errorf("import: no %s in %v", want, got)
		}
	}
	// Posts.Index already imports these, and the module itself is not offered
	for _, not := range []string{"Layout", "Post", "User", "Posts.Index"} {
		if _, ok := got[not]; ok {
			t.Errorf("import: %s offered", not)
		}
	}
}

func TestComplete_HoleRanksFittingNames(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), "Db.limit (perPage + 1)", "Db.limit ?n", 1)
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	u := c.srv.lastGood().res.U
	before := len(u.Subst)
	items := complete(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "?n", 2))
	if len(u.Subst) != before {
		t.Fatal("completion changed the build's unifier")
	}
	got := labels(items)
	if _, ok := got["q"]; !ok {
		t.Fatalf("no q in %v", got)
	}
	if !strings.HasPrefix(got["perPage"].SortText, "0") || !strings.HasPrefix(got["q"].SortText, "1") {
		t.Fatalf("perPage (Int) not ranked before q (Text): %q %q", got["perPage"].SortText, got["q"].SortText)
	}
	hole := c.at("app/Posts/Index.alder", "?n", 0)
	if it := got["perPage"]; it.TextEdit.Range != (Range{Start: hole, End: Position{hole.Line, hole.Character + 2}}) || it.TextEdit.NewText != "perPage" {
		t.Fatalf("the edit must replace the whole hole, `?` included: %+v", it.TextEdit)
	}
	// with the cursor between `?` and its name, the edit still covers the whole hole
	mid := complete(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "?n", 1))
	if r := labels(mid)["perPage"].TextEdit.Range; r.Start != hole || r.End.Character != hole.Character+2 {
		t.Fatalf("mid-hole range %+v", r)
	}
}

// A `?` typed on a new line (no hole in the build yet) is replaced too.
func TestComplete_TypedHoleReplacesQuestionMark(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := typing(t, c, "app/Posts/Index.alder", afterQ, "  let z = ?pa")
	it, ok := labels(complete(t, c, "app/Posts/Index.alder", p))["params"]
	if !ok || it.TextEdit.Range.Start.Character != p.Character-3 || it.TextEdit.Range.End != p {
		t.Fatalf("params: %+v at %+v", it, p)
	}
}

func TestComplete_Markup(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	text := c.texts["app/Posts/Index.alder"]
	// a fresh line inside view's body: the line after the first `<div`
	i := strings.Index(text, "<div")
	j := strings.Index(text[i:], "\n") + i + 1
	c.change("app/Posts/Index.alder", text[:j]+"    <se\n"+text[j:])
	p := c.at("app/Posts/Index.alder", "    <se", len("    <se"))
	if _, ok := labels(complete(t, c, "app/Posts/Index.alder", p))["section"]; !ok {
		t.Fatal("no <section>")
	}
	c.change("app/Posts/Index.alder", text[:j]+"    <input dis\n"+text[j:])
	p = c.at("app/Posts/Index.alder", "<input dis", len("<input dis"))
	it, ok := labels(complete(t, c, "app/Posts/Index.alder", p))["disabled"]
	if !ok || it.TextEdit.NewText != "disabled" {
		t.Fatalf("disabled: %+v", it)
	}
	c.change("app/Posts/Index.alder", text[:j]+"    <a hr\n"+text[j:])
	p = c.at("app/Posts/Index.alder", "<a hr", len("<a hr"))
	if it := labels(complete(t, c, "app/Posts/Index.alder", p))["href"]; it.TextEdit.NewText != `href=""` {
		t.Fatalf("href: %+v", it)
	}
}

func TestComplete_CloseTag(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := typing(t, c, "app/Posts/Index.alder", "<li>\n", "            <span class=\"x\"><b>x</b><br/></")
	got := complete(t, c, "app/Posts/Index.alder", p)
	if len(got) != 1 || got[0].Label != "span" || got[0].Kind != 10 {
		t.Fatalf("</: %+v", got)
	}
}

func TestComplete_PatternCtors(t *testing.T) {
	c := start(t, blogCopy(t))
	rel, p, want := firstUnionArm(t, c)
	got := labels(complete(t, c, rel, p))
	if len(got) != len(want) {
		t.Errorf("pattern: %v, want only %v", got, want)
	}
	for _, ctor := range want {
		if it, ok := got[ctor]; !ok || it.Kind != 4 {
			t.Errorf("pattern: no %s in %v", ctor, got)
		}
	}
}

func TestComplete_Otherwise(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := typing(t, c, "app/Posts/Index.alder", afterQ, "  let z = pa")
	got := labels(complete(t, c, "app/Posts/Index.alder", p))
	for _, want := range []string{"params", "session", "q", "page", "perPage", "view", "Post", "Text", "match"} {
		if _, ok := got[want]; !ok {
			t.Errorf("no %s in %v", want, got)
		}
	}
	if _, ok := got["matching"]; ok {
		t.Error("matching is bound after the cursor's line")
	}
	if it := got["params"]; it.TextEdit.Range.Start.Character != p.Character-2 {
		t.Fatalf("params: %+v", it)
	}
	if got["match"].Kind != 14 || got["Text"].Kind != 9 {
		t.Fatalf("kinds: %+v %+v", got["match"], got["Text"])
	}
}

// A line above the cursor that changed since the last good build hides the locals.
func TestComplete_StaleAboveHidesLocals(t *testing.T) {
	c := start(t, blogCopy(t))
	text := openClean(t, c, "app/Posts/Index.alder")
	c.change("app/Posts/Index.alder", strings.Replace(text, "perPage = 10", "perPage = \"oops", 1))
	c.diags("app/Posts/Index.alder")
	p := typing(t, c, "app/Posts/Index.alder", afterQ, "  let z = pa")
	got := labels(complete(t, c, "app/Posts/Index.alder", p))
	if _, ok := got["params"]; ok {
		t.Fatal("a stale scope answered")
	}
	if _, ok := got["let"]; !ok {
		t.Fatalf("keywords still complete: %v", got)
	}
}

// A burst of changes, then completion, answers from the last text.
func TestComplete_AfterBurst(t *testing.T) {
	c := start(t, blogCopy(t))
	text := openClean(t, c, "app/Posts/Index.alder")
	i := strings.Index(text, afterQ) + len(afterQ)
	for _, typed := range []string{"  let z = p", "  let z = pa", "  let z = params.", "  let z = Te", "  let z = Text."} {
		c.change("app/Posts/Index.alder", text[:i]+typed+"\n"+text[i:])
	}
	p := c.at("app/Posts/Index.alder", "  let z = Text.", len("  let z = Text."))
	got := labels(complete(t, c, "app/Posts/Index.alder", p))
	if _, ok := got["trim"]; !ok {
		t.Fatalf("after the burst: %v", got)
	}
}

func TestComplete_MultiByte(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := typing(t, c, "app/Posts/Index.alder", afterQ, "  let é = Text.tr")
	items := complete(t, c, "app/Posts/Index.alder", p)
	it := labels(items)["trim"]
	if it.TextEdit.Range.Start.Character != p.Character-2 || it.TextEdit.Range.End != p {
		t.Fatalf("range %+v at %+v", it.TextEdit.Range, p)
	}
}

// firstUnionArm opens the first module (in the program's order) whose code has a match on a
// union with two or more constructors, and returns its rel, the position of the first arm's
// pattern and the constructors.
func firstUnionArm(t *testing.T, c *client) (string, Position, []string) {
	t.Helper()
	prog, res, _, ioErrs := buildkit.LoadChecked(c.root, nil)
	if len(ioErrs) > 0 || res == nil {
		t.Fatalf("load: %v", ioErrs)
	}
	var found *syntax.Match
	var walk func(n syntax.Spanned)
	walk = func(n syntax.Spanned) {
		if found != nil {
			return
		}
		if x, ok := n.(*syntax.Match); ok && len(x.Arms) > 0 {
			if u, ok := res.U.Apply(res.Types[x.Subject]).(*types.Union); ok && len(u.Constructors()) >= 2 {
				found = x
				return
			}
		}
		for _, ch := range syntax.Children(n) {
			walk(ch)
		}
	}
	for _, name := range prog.Order {
		m := prog.Modules[name]
		walk(m)
		if found == nil {
			continue
		}
		u := res.U.Apply(res.Types[found.Subject]).(*types.Union)
		var ctors []string
		for _, k := range u.Constructors() {
			ctors = append(ctors, k.Name)
		}
		c.open(m.File, c.disk(m.File))
		c.diags(m.File)
		sp := found.Arms[0].Pat.GetSpan()
		return m.File, Position{Line: sp.Start.Line - 1, Character: sp.Start.Col - 1}, ctors
	}
	t.Fatal("no match on a union in the blog")
	return "", Position{}, nil
}

// fieldsMidLine opens rel (after fix rewrites its text on disk), cuts the first occurrence of
// cut down to keep, and completes at the end of keep: binders introduced earlier on the
// cursor's line stay in scope while it is edited.
func fieldsMidLine(t *testing.T, rel string, fix func(string) string, cut, keep string) map[string]item {
	t.Helper()
	root := blogCopy(t)
	if fix != nil {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(fix(string(b))), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := start(t, root)
	text := openClean(t, c, rel)
	if !strings.Contains(text, cut) {
		t.Fatalf("no %q in %s", cut, rel)
	}
	c.change(rel, strings.Replace(text, cut, keep, 1))
	return labels(complete(t, c, rel, c.at(rel, keep, len(keep))))
}

// The user's case: `e.` inside a one-line `for e in …` offers e's fields.
func TestComplete_ForBinderOnTheSameLine(t *testing.T) {
	oneLine := func(s string) string {
		i := strings.Index(s, "{for post in model.posts ->")
		j := strings.Index(s[i:], "</tr>}") + i + len("</tr>}")
		return s[:i] + "{for post in model.posts -> <tr><td>{post.title}</td><td>{post.published}</td></tr>}" + s[j:]
	}
	got := fieldsMidLine(t, "app/Write.alder", oneLine, "<td>{post.title}</td><td>", "<td>{post.")
	if _, ok := got["title"]; !ok {
		t.Fatalf("post. fields: %v", got)
	}
}

// A lambda parameter bound earlier on the cursor's line is in scope.
func TestComplete_LambdaParamOnTheSameLine(t *testing.T) {
	got := fieldsMidLine(t, "app/Write.alder", nil, "(\\p -> p.title)", "(\\p -> p.")
	if _, ok := got["title"]; !ok {
		t.Fatalf("p. fields: %v", got)
	}
}

func TestComplete_MemberViaQualifier(t *testing.T) {
	c, rel := qualCopy(t, "Text.trim")
	openClean(t, c, rel)
	p := typing(t, c, rel, "useTodo = Text.trim 1\n", "  let z = Todo.")
	got := labels(complete(t, c, rel, p))
	if it, ok := got["find"]; !ok || it.TextEdit.NewText != "find" {
		t.Fatalf("Todo. members: %v", got)
	}
}

// TestComplete_ImportInsertsAlias: Admin.Layout beside Layout needs `as AdminLayout`; Post does not.
func TestComplete_ImportInsertsAlias(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Index.alder"
	openClean(t, c, rel)
	p := typing(t, c, rel, "import Layout\n", "import ")
	got := labels(complete(t, c, rel, p))
	if it, ok := got["Admin.Layout"]; !ok || it.TextEdit.NewText != "Admin.Layout as AdminLayout" {
		t.Fatalf("Admin.Layout: %+v", it)
	}
	if it := got["Comment"]; it.TextEdit.NewText != "Comment" {
		t.Fatalf("Comment: %+v", it)
	}
}

// TestComplete_OtherwiseOffersQualifiers: an imported module is offered by its qualifier, the
// spelling the compiler accepts (`AdminLayout`, not `Admin.Layout`, which is E0205 with a
// member); an imported page module is offered by its qualifier, for its members, and by its full
// path, for its page value.
func TestComplete_OtherwiseOffersQualifiers(t *testing.T) {
	root := blogCopy(t)
	rel := "app/Posts/Index.alder"
	p := filepath.Join(root, rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(b), "import Layout\n", "import Admin.Layout as AdminLayout\nimport Layout\nimport Posts.Show\n", 1)
	text += "\nuseAdmin : User.UserId -> Query Bool\nuseAdmin = AdminLayout.isAdmin\n\nuseShow : Html a\nuseShow = <a href={Posts.Show { id: 1 }}>x</a>\n"
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	c := start(t, root)
	openClean(t, c, rel)
	at := typing(t, c, rel, afterQ, "  let z = A")
	got := labels(complete(t, c, rel, at))
	if it, ok := got["AdminLayout"]; !ok || it.TextEdit.NewText != "AdminLayout" || it.Kind != 9 {
		t.Errorf("no AdminLayout: %+v", got["AdminLayout"])
	}
	if _, ok := got["Admin.Layout"]; ok {
		t.Error("Admin.Layout offered by its full path")
	}
	for _, want := range []string{"Layout", "Show", "Posts.Show"} {
		if _, ok := got[want]; !ok {
			t.Errorf("no %s", want)
		}
	}
}
