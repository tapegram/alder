package langdoc

import (
	"strings"
	"testing"

	"alder/internal/doc"
	"alder/internal/syntax"
)

// operatorSample is every operator token of the language, as the brief lists them, and `<-`.
const operatorSample = "== != < <= > >= + - * / % ++ && || ?? |> <-"

// oldKeywords are the reserved words with no entry, each with the reason.
var oldKeywords = map[string]string{
	"page": "the old form of a route: a `page` declaration is E0046 (internal/check/removed.go)",
	"msg":  "the old form of a write route: a `msg` declaration is E0046 (internal/check/removed.go)",
}

// notDocumented are the token kinds that have no entry: names, literals, brackets and
// punctuation, the hole (its entry is `?`, looked up by the hover of a hole) and the keyword
// kind (each word is checked on its own).
var notDocumented = map[string]bool{
	"EOF": true, "NL": true, "LIdent": true, "UIdent": true, "Int": true, "Float": true,
	"String": true, "Text": true, "LParen": true, "RParen": true, "LBrace": true, "RBrace": true,
	"LBracket": true, "RBracket": true, "Comma": true, "Colon": true, "Dot": true, "DotDot": true,
	"Pipe": true, "Arrow": true, "Equal": true, "Backslash": true, "Underscore": true, "At": true,
	"Hole": true, "Keyword": true, "Invalid": true,
}

func TestCovers(t *testing.T) {
	toks, diags := syntax.Tokens("sample", []byte(operatorSample))
	_ = diags // the sample is no module; only its tokens matter
	sampled := map[syntax.Kind]bool{}
	for _, tok := range toks {
		if tok.Kind == syntax.EOF || tok.Kind == syntax.NL {
			continue
		}
		sampled[tok.Kind] = true
		if _, ok := Lookup(tok.Text); !ok {
			t.Errorf("operator %s has no entry", tok.Text)
		}
	}
	if len(sampled) != len(strings.Fields(operatorSample)) {
		t.Errorf("the sample lexed as %d kinds, want %d", len(sampled), len(strings.Fields(operatorSample)))
	}
	keywords := map[string]bool{}
	for _, w := range syntax.Keywords() {
		keywords[w] = true
		_, has := Lookup(w)
		_, old := oldKeywords[w]
		switch {
		case !has && !old:
			t.Errorf("keyword %s has no entry and is not in oldKeywords", w)
		case has && old:
			t.Errorf("keyword %s has an entry and is in oldKeywords", w)
		}
	}
	for w := range oldKeywords {
		if !keywords[w] {
			t.Errorf("oldKeywords: %s is not a keyword", w)
		}
	}
	for k := syntax.Kind(0); k <= syntax.Invalid; k++ {
		if !sampled[k] && !notDocumented[k.String()] {
			t.Errorf("token kind %s is neither in the sample nor in notDocumented", k)
		}
	}
	if _, ok := Lookup("?"); !ok {
		t.Error("the typed hole `?` has no entry")
	}
}

func TestAllOrderAndLookup(t *testing.T) {
	all := All()
	seen := map[string]bool{}
	for _, e := range all {
		if seen[e.Token] {
			t.Errorf("%s: two entries", e.Token)
		}
		seen[e.Token] = true
		if got, ok := Lookup(e.Token); !ok || got != e {
			t.Errorf("Lookup(%q) is not All's entry", e.Token)
		}
	}
	// operators first, then `<-` and `?`, then keywords
	phase := func(e Entry) int {
		switch {
		case e.Token == "<-" || e.Token == "?":
			return 1
		case e.Signature == "":
			return 2
		}
		return 0
	}
	for i := 1; i < len(all); i++ {
		if phase(all[i]) < phase(all[i-1]) {
			t.Errorf("%s comes after %s", all[i].Token, all[i-1].Token)
		}
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup(nope) found an entry")
	}
}

func TestWellFormed(t *testing.T) {
	for _, e := range All() {
		need := doc.Strict{Book: true}
		if e.Signature != "" {
			need.Example = true
		}
		for _, p := range doc.Problems(e.Doc, need) {
			t.Errorf("%s: %s", e.Token, p)
		}
	}
}

func TestMarkdown(t *testing.T) {
	e, _ := Lookup("|>")
	got := e.Markdown()
	if !strings.HasPrefix(got, "```alder\n(|>) : a -> (a -> b) -> b\n```\n\n") || !strings.HasSuffix(got, `*Book: chapter 3, "Operators"*`) {
		t.Fatalf("Markdown %q", got)
	}
	k, _ := Lookup("match")
	if got := k.Markdown(); strings.HasPrefix(got, "```") || !strings.HasSuffix(got, `*Book: chapter 3, "Match"*`) {
		t.Fatalf("keyword Markdown %q", got)
	}
}
