package adapters

import (
	"strings"
	"testing"
	"time"

	"alder/rt/html"
)

// render is Markdown(src) as the page would send it.
func render(src string) string { return html.Render(Markdown(src)) }

func TestMarkdown_Constructs(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"empty", "", ""},
		{"blank lines only", "\n  \n\t\n", ""},
		{"paragraph", "Hello, world.", "<p>Hello, world.</p>"},
		{"paragraphs", "One.\n\nTwo.\n\n\n\nThree.", "<p>One.</p><p>Two.</p><p>Three.</p>"},
		{"line break", "Roses are red,\nviolets are blue.", "<p>Roses are red,<br />violets are blue.</p>"},
		{"crlf", "a\r\nb\r\n\r\nc", "<p>a<br />b</p><p>c</p>"},
		{"surrounding spaces", "   indented   \n  next", "<p>indented<br />next</p>"},
		{"emphasis", "a *b* c", "<p>a <em>b</em> c</p>"},
		{"strong", "a **b** c", "<p>a <strong>b</strong> c</p>"},
		{"emphasis in strong", "**a *b* c**", "<p><strong>a <em>b</em> c</strong></p>"},
		{"code", "run `go test` now", "<p>run <code>go test</code> now</p>"},
		{"code is literal", "`*not* [a](https://x.y) **b**`", "<p><code>*not* [a](https://x.y) **b**</code></p>"},
		{"link", "see [the docs](https://example.com/docs?a=1&b=2)", `<p>see <a href="https://example.com/docs?a=1&amp;b=2">the docs</a></p>`},
		{"http link", "[x](http://example.com)", `<p><a href="http://example.com">x</a></p>`},
		{"uppercase scheme", "[x](HTTPS://example.com)", `<p><a href="HTTPS://example.com">x</a></p>`},
		{"formatted link text", "[**big** news](https://example.com)", `<p><a href="https://example.com"><strong>big</strong> news</a></p>`},
		{"a link closes at the first ](", "[a [b](https://x.y)](https://e.com)", `<p><a href="https://x.y">a [b</a>](https://e.com)</p>`},
		{"a parenthesis ends the URL", "[Foo](https://e.com/wiki/Foo_(bar))", `<p><a href="https://e.com/wiki/Foo_(bar">Foo</a>)</p>`},
		{"an encoded parenthesis", "[Foo](https://e.com/wiki/Foo_%28bar%29)", `<p><a href="https://e.com/wiki/Foo_%28bar%29">Foo</a></p>`},
		{"link in emphasis", "*see [x](https://e.com)*", `<p><em>see <a href="https://e.com">x</a></em></p>`},
		{"arithmetic is not emphasis", "2 * 3 * 4", "<p>2 * 3 * 4</p>"},
		{"empty emphasis", "** and ****", "<p>** and ****</p>"},
		{"empty code", "a `` b", "<p>a `` b</p>"},
	} {
		if got := render(c.src); got != c.want {
			t.Errorf("%s: Markdown(%q)\n got %s\nwant %s", c.name, c.src, got, c.want)
		}
	}
}

// Hostile input: markup is text, unsafe links are their literal source, unclosed delimiters
// are literal.
func TestMarkdown_Hostile(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"script", "<script>alert(1)</script>", "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>"},
		{"html in emphasis", "*<img src=x onerror=alert(1)>*", "<p><em>&lt;img src=x onerror=alert(1)&gt;</em></p>"},
		{"html in code", "`</code><script>`", "<p><code>&lt;/code&gt;&lt;script&gt;</code></p>"},
		{"entity", "&lt;b&gt; &amp;", "<p>&amp;lt;b&amp;gt; &amp;amp;</p>"},
		{"javascript link", "[x](javascript:alert(1))", "<p>[x](javascript:alert(1))</p>"},
		{"javascript link, mixed case", "[x](JaVaScRiPt:alert(1))", "<p>[x](JaVaScRiPt:alert(1))</p>"},
		{"javascript link, leading space", "[x]( javascript:alert(1))", "<p>[x]( javascript:alert(1))</p>"},
		{"data link", "[x](data:text/html,<script>alert(1)</script>)", "<p>[x](data:text/html,&lt;script&gt;alert(1)&lt;/script&gt;)</p>"},
		{"vbscript link", "[x](vbscript:msgbox)", "<p>[x](vbscript:msgbox)</p>"},
		{"mailto link", "[x](mailto:a@b.c)", "<p>[x](mailto:a@b.c)</p>"},
		{"protocol-relative link", "[x](//evil.example)", "<p>[x](//evil.example)</p>"},
		{"relative link", "[x](/admin)", "<p>[x](/admin)</p>"},
		{"no host", "[x](https://)", "<p>[x](https://)</p>"},
		{"control character", "[x](https://e.com/\x01)", "<p>[x](https://e.com/\x01)</p>"},
		{"no ]( in a url", "[a](https://e.com/](https://f.com)", "<p>[a](https://e.com/](https://f.com)</p>"},
		{"too long a url", "[x](https://e.com/" + strings.Repeat("a", maxURL) + ")", "<p>[x](https://e.com/" + strings.Repeat("a", maxURL) + ")</p>"},
		{"tab in url", "[x](https://e.com/\tx)", "<p>[x](https://e.com/\tx)</p>"},
		{"attribute breakout", `[x](https://e.com/" onclick="alert(1))`, `<p><a href="https://e.com/&#34; onclick=&#34;alert(1">x</a>)</p>`},
		{"unclosed emphasis", "an *unclosed emphasis", "<p>an *unclosed emphasis</p>"},
		{"unclosed strong", "an **unclosed strong", "<p>an **unclosed strong</p>"},
		{"unclosed code", "an `unclosed code", "<p>an `unclosed code</p>"},
		{"unclosed link text", "[never closed", "<p>[never closed</p>"},
		{"unclosed link url", "[x](https://e.com", "<p>[x](https://e.com</p>"},
		{"emphasis across paragraphs", "*a\n\nb*", "<p>*a</p><p>b*</p>"},
	} {
		got := render(c.src)
		if got != c.want {
			t.Errorf("%s: Markdown(%q)\n got %s\nwant %s", c.name, c.src, got, c.want)
		}
		if strings.Contains(got, "<script") || strings.Contains(got, `href="javascript`) || strings.Contains(got, `href="data`) {
			t.Errorf("%s: unsafe output %s", c.name, got)
		}
	}
}

// A 50 KB body renders in under 50 ms, whatever it holds: every closing delimiter is searched
// for once (finder), so even 50 000 unclosed delimiters stay linear.
func TestMarkdown_Large(t *testing.T) {
	const size = 50 * 1024
	words := strings.Repeat("Some *emphasis*, **strong**, `code` and a [link](https://example.com/x).\n", 1000)
	for name, src := range map[string]string{
		"prose":           words[:size],
		"stars":           strings.Repeat("*", size),
		"double stars":    strings.Repeat("** ", size/3),
		"brackets":        strings.Repeat("[", size),
		"unclosed links":  strings.Repeat("[x](", size/4),
		"unsafe links":    strings.Repeat("[x](javascript:y)", size/17),
		"backticks":       strings.Repeat("`", size),
		"one long line":   strings.Repeat("a", size),
		"blank lines":     strings.Repeat("\n", size),
		"mixed unclosed":  strings.Repeat("*[`", size/3),
		"nested brackets": strings.Repeat("[a](", size/8) + strings.Repeat(")", size/8),
		"long http urls":  strings.Repeat("[a](http://e.com/", size/17) + ")",
		// every "[" before one long link that is refused: checked once, not once per "["
		"no host":     strings.Repeat("[", 48000) + "[x](https:///" + strings.Repeat("a", 2000) + ")",
		"bad escape":  strings.Repeat("[", 48000) + "[x](https://e.com/%zz" + strings.Repeat("a", 2000) + ")",
		"a good link": strings.Repeat("[", 48000) + "[x](https://e.com/" + strings.Repeat("a", 2000) + ")",
		"nested urls": strings.Repeat("[](https://e.com/", size/17) + ")",
		"url windows": strings.Repeat("[](https://e.com/"+strings.Repeat("a", 100)+strings.Repeat("[](https://e.com/", 100)+")", size/1800),
	} {
		start := time.Now()
		out := render(src)
		if d := time.Since(start); d > 50*time.Millisecond {
			t.Errorf("%s: %d bytes rendered in %v (want < 50ms)", name, len(src), d)
		}
		if strings.Contains(out, "<script") {
			t.Errorf("%s: unsafe output", name)
		}
	}
}
