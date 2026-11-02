package semtok

import (
	"testing"

	"alder/internal/syntax"
)

const exprSrc = `module M

view : Model -> Html Msg
view model =
  <Layout.Page title={model.post.title}>
    <h1 class="t">{model.post.title}</h1>
    {for c in model.items ->
      <li>{c.name}</li>}
    <Errors form />
    <form submit={Save}>x</form>
  </Layout.Page>

calc : Int -> Int
calc x = {
  let y = Post.bySlug x
  let z <- Db.from Comment |> Db.where (Comment.post.eq x)
  match y {
    Just { title: t } -> -2,
    [a, ..rest] -> 0,
    True -> 1,
    _ -> calc (x - 1),
  }
}

lam = \a b -> a.f b
rec = { r | name: "n", n: () }
neg = -3
str = "a {x.y} b"
ctor = Posts.Show { slug: "s" }

update msg m = match msg {
  _ -> session.user
}
`

func TestExprs(t *testing.T) {
	if _, ds := syntax.ParseFile("t.alder", []byte(exprSrc)); len(ds) > 0 {
		t.Fatalf("the fixture does not parse: %v", ds)
	}
	toks := Module("t.alder", []byte(exprSrc))
	checkInvariants(t, "exprSrc", exprSrc, toks)
	s := exprSrc
	for _, c := range []struct {
		needle string
		off    int
		kind   Kind
		mods   int
	}{
		{"<Layout.Page title", 1, TNamespace, 0},
		{"<Layout.Page title", 8, TTag, 0},
		{"<Layout.Page title", 13, TAttribute, 0},
		{"<Layout.Page title", 18, TOperator, 0},
		{"{model.post.title}>", 1, TVariable, 0},
		{"{model.post.title}>", 7, TProperty, 0},
		{"{model.post.title}>", 12, TProperty, 0},
		{`<h1 class="t">`, 1, TTag, 0},
		{`<h1 class="t">`, 4, TAttribute, 0},
		{`<h1 class="t">`, 10, TString, 0},
		{"</h1>", 2, TTag, 0},
		{"{for c in", 1, TKeyword, 0},
		{"{for c in", 5, TParameter, Declaration},
		{"{for c in", 7, TKeyword, 0},
		{"<Errors form />", 1, TTag, 0},
		{"<Errors form />", 8, TAttribute, 0},
		{"<form submit", 1, TTag, 0},
		{"<form submit", 6, TAttribute, 0},
		{"{Save}", 1, TEnumMember, 0},
		{"</Layout.Page>", 2, TNamespace, 0},
		{"</Layout.Page>", 9, TTag, 0},
		{"let y = Post.bySlug x", 4, TVariable, Declaration},
		{"let y = Post.bySlug x", 8, TNamespace, 0},
		{"let y = Post.bySlug x", 13, TFunction, 0},
		{"let y = Post.bySlug x", 20, TVariable, 0},
		{"Db.from Comment", 0, TNamespace, 0},
		{"Db.from Comment", 3, TFunction, 0},
		{"Db.from Comment", 8, TEnumMember, 0},
		{"|> Db.where", 0, TOperator, 0},
		{"|> Db.where", 6, TFunction, 0},
		{"Comment.post.eq x", 0, TNamespace, 0},
		{"Comment.post.eq x", 8, TFunction, 0},
		{"Comment.post.eq x", 13, TMethod, 0},
		{"Just { title: t }", 0, TEnumMember, 0},
		{"Just { title: t }", 7, TProperty, 0},
		{"Just { title: t }", 14, TVariable, 0},
		{"[a, ..rest]", 1, TVariable, 0},
		{"[a, ..rest]", 4, TOperator, 0},
		{"[a, ..rest]", 6, TVariable, 0},
		{"True -> 1", 0, TBoolean, 0},
		{"_ -> calc", 5, TFunction, 0},
		{`\a b -> a.f b`, 0, TKeyword, 0},
		{`\a b -> a.f b`, 1, TParameter, Declaration},
		{`\a b -> a.f b`, 3, TParameter, Declaration},
		{`\a b -> a.f b`, 10, TMethod, 0},
		{`\a b -> a.f b`, 12, TVariable, 0},
		{"{ r | name", 2, TVariable, 0},
		{"{ r | name", 4, TOperator, 0},
		{"{ r | name", 6, TProperty, 0},
		{"n: ()", 0, TProperty, 0},
		{`"a {x.y} b"`, 4, TVariable, 0},
		{`"a {x.y} b"`, 6, TProperty, 0},
		{"Posts.Show { slug", 0, TNamespace, 0},
		{"Posts.Show { slug", 6, TEnumMember, 0},
		{"Posts.Show { slug", 13, TProperty, 0},
		{"match msg {", 6, TVariable, 0},
		{"session.user", 0, TVariable, 0},
		{"session.user", 8, TProperty, 0},
	} {
		want(t, s, toks, c.needle, c.off, c.kind, c.mods)
	}
	for _, c := range []struct {
		needle string
		off    int
	}{
		{"<Layout.Page title", 0}, {"{model.post.title}>", 18}, {`<h1 class="t">`, 0}, {`<h1 class="t">`, 13},
		{"</h1>", 0}, {"</h1>", 1}, {"<Errors form />", 13}, {"_ -> calc", 0}, {"-> -2", 4},
	} {
		wantNone(t, s, toks, c.needle, c.off)
	}
	wantLen(t, s, toks, "-> -2", 3, TNumber, 2)
	wantLen(t, s, toks, "neg = -3", 6, TNumber, 2)
	wantLen(t, s, toks, "n: ()", 3, TEnumMember, 2)
}

// TestBroken: text that does not parse still gets tokens, in order and inside their lines, and
// the declarations around the broken one keep their colours (Review Focus 1).
func TestBroken(t *testing.T) {
	for _, src := range []string{
		"module M\n\nok = 1\n\nbad = \"abc\n\nalso = 2\n",
		"module M\n\nv = <div class=\"x\">\n",
		"module",
		"",
		"module M\n\nf x = match x {\n",
		"module M\n\nf = \"a {b\"\n",
		"module M\n\nf = \"a {\"b {c}\"} d\"\n",
		"module M\n\nf = <p>{\n",
	} {
		checkInvariants(t, src, src, Module("t.alder", []byte(src)))
	}
	src := "module M\n\nok = 1\n\nbad = \"abc\n\nalso = 2\n"
	toks := Module("t.alder", []byte(src))
	want(t, src, toks, "ok = 1", 0, TFunction, Declaration)
	want(t, src, toks, "also = 2", 0, TFunction, Declaration)
}
