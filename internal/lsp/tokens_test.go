package lsp

import (
	"strings"
	"testing"

	"alder/internal/semtok"
)

type semTok struct {
	Line, Char, Len int
	Type            string
	Mods            []string
}

// semanticTokens asks for rel's tokens (method: …/full or …/range) and decodes them.
func semanticTokens(t *testing.T, c *client, method, rel string, rng *Range) []semTok {
	t.Helper()
	params := map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}}
	if rng != nil {
		params["range"] = rng
	}
	var out struct {
		Data []int `json:"data"`
	}
	if e := c.call(method, params, &out); e != nil {
		t.Fatalf("%s: %v", method, e)
	}
	return decodeTokens(t, out.Data)
}

// decodeTokens turns the five-integer stream back into absolute tokens with names.
func decodeTokens(t *testing.T, data []int) []semTok {
	t.Helper()
	if len(data)%5 != 0 {
		t.Fatalf("%d integers: not a multiple of 5", len(data))
	}
	var out []semTok
	line, char := 0, 0
	for i := 0; i < len(data); i += 5 {
		if data[i] > 0 {
			line, char = line+data[i], data[i+1]
		} else {
			char += data[i+1]
		}
		k := semTok{Line: line, Char: char, Len: data[i+2], Type: semtok.Types[data[i+3]]}
		for b, name := range semtok.Modifiers {
			if data[i+4]&(1<<b) != 0 {
				k.Mods = append(k.Mods, name)
			}
		}
		out = append(out, k)
	}
	return out
}

// wantTok checks the token starting at needle+off in rel's open text (utf-8: characters are bytes).
func wantTok(t *testing.T, c *client, toks []semTok, rel, needle string, off int, typ string, mods ...string) {
	t.Helper()
	p := c.at(rel, needle, off)
	for _, k := range toks {
		if k.Line == p.Line && k.Char == p.Character {
			if k.Type != typ || strings.Join(k.Mods, ",") != strings.Join(mods, ",") {
				t.Errorf("%q+%d: %s %v, want %s %v", needle, off, k.Type, k.Mods, typ, mods)
			}
			return
		}
	}
	t.Errorf("%q+%d: no token at %d:%d", needle, off, p.Line, p.Character)
}

func wantNoTok(t *testing.T, c *client, toks []semTok, rel, needle string, off int) {
	t.Helper()
	p := c.at(rel, needle, off)
	for _, k := range toks {
		if k.Line == p.Line && k.Char == p.Character {
			t.Errorf("%q+%d: token %s, want none", needle, off, k.Type)
		}
	}
}

const showRel = "app/Posts/Show.alder"

func TestSemanticTokens_Show(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(showRel, c.disk(showRel))
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", showRel, nil)
	for _, w := range []struct {
		needle string
		off    int
		typ    string
		mods   []string
	}{
		{"module Posts.Show", 0, "keyword", nil},
		{"module Posts.Show", 7, "namespace", nil},
		{"module Posts.Show", 13, "namespace", nil},
		{"import Layout", 7, "namespace", nil},
		{"route Show ", 0, "keyword", nil},
		{`"/posts/{slug}"`, 0, "string", nil},
		{"-- load finds", 0, "comment", []string{"documentation"}},
		{"Show.load : Show.Params", 0, "namespace", nil},
		{"Show.load : Show.Params", 5, "function", []string{"declaration"}},
		{"Show.load params session", 5, "function", []string{"declaration"}},
		{"Show.load params session", 10, "parameter", []string{"declaration"}},
		{"Show.load params session", 17, "parameter", []string{"declaration"}},
		{"Post.bySlug", 0, "namespace", nil},
		{"Post.bySlug", 5, "function", nil},
		{"let found", 4, "variable", []string{"declaration"}},
		{"Just post ->", 0, "enumMember", nil},
		{"Just post ->", 5, "variable", nil},
		{"|> Db.where", 0, "operator", nil},
		{"type Model", 5, "type", []string{"declaration"}},
		{"<Layout.Page title", 1, "namespace", nil},
		{"<Layout.Page title", 8, "tag", nil},
		{"<Layout.Page title", 13, "attribute", nil},
		{"{model.post.title}>", 1, "variable", nil},
		{"{model.post.title}>", 7, "property", nil},
		{"{model.post.title}>", 12, "property", nil},
		{"</Layout.Page>", 9, "tag", nil},
		{`<al-alert heading="Draft"`, 1, "tag", nil},
		{`<al-alert heading="Draft"`, 10, "attribute", nil},
		{`<al-alert heading="Draft"`, 18, "string", nil},
		{"<h1>", 1, "tag", nil},
		{"</h1>", 2, "tag", nil},
		{"{for c in", 1, "keyword", nil},
		{"{for c in", 5, "parameter", []string{"declaration"}},
	} {
		wantTok(t, c, toks, showRel, w.needle, w.off, w.typ, w.mods...)
	}
	wantNoTok(t, c, toks, showRel, "<Layout.Page title", 0)
	for _, k := range toks { // the route path is one string: `{slug}` is not an interpolation there
		if p := c.at(showRel, `"/posts/{slug}"`, 0); k.Line == p.Line && k.Char == p.Character && k.Len != len(`"/posts/{slug}"`) {
			t.Errorf("route path length %d", k.Len)
		}
	}
}

const tokensRel = "app/Tokens.alder"

const tokensSrc = `module Tokens

answer : Int
answer = -1

flag : Bool
flag = True

pick : Maybe Int -> Int
pick m = match m {
  -- inside a body
  Just n -> n,
  Nothing -> ?h,
}

broken : Int
broken = )

after : Unit
after = {
  let u = ()
  u
}

greet : Text -> Text
greet x = "héllo 😀 {x}" ++ x
`

func TestSemanticTokens_Fixture(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(tokensRel, tokensSrc)
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", tokensRel, nil)
	wantTok(t, c, toks, tokensRel, "= -1", 2, "number")
	wantTok(t, c, toks, tokensRel, "= True", 2, "boolean")
	wantTok(t, c, toks, tokensRel, "-- inside a body", 0, "comment")
	wantTok(t, c, toks, tokensRel, "?h", 0, "hole")
	wantTok(t, c, toks, tokensRel, "= ()", 2, "enumMember")
	wantTok(t, c, toks, tokensRel, "pick m = match", 0, "function", "declaration")
	wantTok(t, c, toks, tokensRel, "broken = )", 0, "function", "declaration")
	wantTok(t, c, toks, tokensRel, "after = {", 0, "function", "declaration") // after the broken one
	wantTok(t, c, toks, tokensRel, "let u", 4, "variable", "declaration")
	wantTok(t, c, toks, tokensRel, "{x}", 1, "variable")
	for _, k := range toks {
		p, q, r := c.at(tokensRel, "= -1", 2), c.at(tokensRel, "?h", 0), c.at(tokensRel, "= ()", 2)
		switch {
		case k.Line == p.Line && k.Char == p.Character && k.Len != 2,
			k.Line == q.Line && k.Char == q.Character && k.Len != 2,
			k.Line == r.Line && k.Char == r.Character && k.Len != 2:
			t.Errorf("token %+v: length %d, want 2", k, k.Len)
		}
	}
}

// TestSemanticTokens_UTF16: with a client that offers only utf-16, columns and lengths after
// and over é (one unit, two bytes) and 😀 (two units, four bytes) are in UTF-16 (Review Focus 3).
func TestSemanticTokens_UTF16(t *testing.T) {
	c := startOffering(t, blogCopy(t), Options{}, []string{"utf-16"})
	c.open(tokensRel, tokensSrc)
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", tokensRel, nil)
	line := strings.Split(tokensSrc, "\n")
	ln := 0
	for !strings.HasPrefix(line[ln], "greet x =") {
		ln++
	}
	byteX := strings.LastIndex(line[ln], "x")
	var str, last *semTok
	for i := range toks {
		if toks[i].Line == ln && toks[i].Type == "string" && str == nil {
			str = &toks[i]
		}
		if toks[i].Line == ln {
			last = &toks[i]
		}
	}
	if str == nil || last == nil {
		t.Fatalf("no tokens on line %d: %+v", ln, toks)
	}
	// `"héllo 😀 {` is 14 bytes, 11 UTF-16 units; the final x is 3 units left of its byte column
	if str.Len != 11 || last.Char != byteX-3 || last.Type != "variable" {
		t.Fatalf("string %+v, last %+v (x at byte %d)", *str, *last, byteX)
	}
}

func TestSemanticTokens_Range(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(showRel, c.disk(showRel))
	full := semanticTokens(t, c, "textDocument/semanticTokens/full", showRel, nil)
	got := semanticTokens(t, c, "textDocument/semanticTokens/range", showRel, &Range{Start: Position{18, 0}, End: Position{23, 0}})
	var want []semTok
	for _, k := range full {
		if k.Line >= 18 && k.Line <= 23 {
			want = append(want, k)
		}
	}
	if len(got) == 0 || len(got) != len(want) {
		t.Fatalf("range gave %d tokens, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Line != want[i].Line || got[i].Char != want[i].Char || got[i].Len != want[i].Len || got[i].Type != want[i].Type {
			t.Fatalf("token %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSemanticTokens_Scratch(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "scratch.alder"
	c.open(rel, "in Posts.Show\n\n> 1 + 1\n>    model\n")
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", rel, nil)
	wantTok(t, c, toks, rel, "in Posts.Show", 0, "keyword")
	wantTok(t, c, toks, rel, "in Posts.Show", 3, "namespace")
	wantTok(t, c, toks, rel, "> 1 + 1", 0, "operator")
	wantTok(t, c, toks, rel, ">    model", 5, "variable")
}

// TestSemanticTokens_Empty: an unknown URI, a file outside the root, a non-.alder file and an
// empty document answer `{"data": []}`, never null (Review Focus 5).
func TestSemanticTokens_Empty(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open("app/Empty.alder", "")
	for _, uri := range []string{"file:///nowhere/x.alder", "untitled:Untitled-1", c.uri("README.md"), c.uri("app/Empty.alder")} {
		var out map[string]any
		if e := c.call("textDocument/semanticTokens/full", map[string]any{"textDocument": map[string]any{"uri": uri}}, &out); e != nil {
			t.Fatalf("%s: %v", uri, e)
		}
		if d, ok := out["data"].([]any); !ok || len(d) != 0 {
			t.Fatalf("%s: data %#v, want []", uri, out["data"])
		}
	}
}
