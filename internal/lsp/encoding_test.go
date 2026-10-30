package lsp

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestNegotiate(t *testing.T) {
	for _, c := range []struct {
		offered []string
		want    posEncoding
	}{
		{nil, utf16Enc},
		{[]string{"utf-16"}, utf16Enc},
		{[]string{"utf-32"}, utf16Enc},
		{[]string{"utf-8", "utf-16"}, utf8Enc},
		{[]string{"utf-16", "utf-8"}, utf8Enc},
	} {
		if got := negotiate(c.offered); got != c.want {
			t.Errorf("negotiate(%v) = %v, want %v", c.offered, got, c.want)
		}
	}
}

func TestEncoding_Convert(t *testing.T) {
	line := "aé\U0001F600b" // a, é (2 bytes, 1 unit), 😀 (4 bytes, 2 units), b
	for _, c := range []struct{ byteCol, utf16Col int }{{0, 0}, {1, 1}, {3, 2}, {7, 4}, {8, 5}} {
		if got := utf16Enc.toWire(line, c.byteCol); got != c.utf16Col {
			t.Errorf("toWire(%d) = %d, want %d", c.byteCol, got, c.utf16Col)
		}
		if got := utf16Enc.fromWire(line, c.utf16Col); got != c.byteCol {
			t.Errorf("fromWire(%d) = %d, want %d", c.utf16Col, got, c.byteCol)
		}
		if utf8Enc.toWire(line, c.byteCol) != c.byteCol || utf8Enc.fromWire(line, c.byteCol) != c.byteCol {
			t.Errorf("utf-8 is not the identity at %d", c.byteCol)
		}
	}
	if got := utf16Enc.fromWire(line, 3); got != 3 { // inside the surrogate pair: the character's start
		t.Errorf("fromWire inside a pair = %d, want 3", got)
	}
	if got := utf16Enc.fromWire(line, 7); got != 10 { // past the end keeps its excess
		t.Errorf("fromWire past the end = %d, want 10", got)
	}
}

// wireCol is the character offset of byte column col of line in enc, counted independently of
// the server.
func wireCol(enc posEncoding, line string, col int) int {
	if enc == utf8Enc {
		return col
	}
	return len(utf16.Encode([]rune(line[:col])))
}

var encodings = []struct {
	name    string
	offered []string
	enc     posEncoding
}{
	{"utf-8", []string{"utf-8", "utf-16"}, utf8Enc},
	{"utf-16", []string{"utf-16"}, utf16Enc},
	{"none offered", nil, utf16Enc},
}

const (
	encRel  = "app/Posts/Index.alder"
	encLine = `  let e = if Text.isEmpty "é😀" then lastPage else `
)

// atCol is the position of needle's first occurrence (plus off bytes) in line number ln of the
// open text of rel, in enc.
func atCol(c *client, enc posEncoding, rel string, ln int, needle string, off int) Position {
	line := strings.Split(c.texts[rel], "\n")[ln]
	return Position{Line: ln, Character: wireCol(enc, line, strings.Index(line, needle)+off)}
}

// TestEncoding_Requests runs the requests that take or give a column on a line holding é and an
// astral character before the places asked about, under each encoding.
func TestEncoding_Requests(t *testing.T) {
	for _, e := range encodings {
		t.Run(e.name, func(t *testing.T) {
			c := startOffering(t, blogCopy(t), Options{}, e.offered)
			if got := c.initResult["capabilities"].(map[string]any)["positionEncoding"]; got != e.enc.String() {
				t.Fatalf("positionEncoding %v, want %v", got, e.enc)
			}
			text := c.disk(encRel)
			c.open(encRel, text)
			c.diags(encRel)
			i := strings.Index(text, afterQ) + len(afterQ)
			good := text[:i] + encLine + "0\n" + text[i:]
			c.change(encRel, good)
			if ds := c.diags(encRel); len(ds) != 0 {
				t.Fatalf("diagnostics %+v", ds)
			}
			ln := strings.Count(text[:i], "\n")
			use := atCol(c, e.enc, encRel, ln, "lastPage", 2)
			if e.enc == utf16Enc && use.Character == strings.Index(encLine, "lastPage")+2 {
				t.Fatal("the test line does not tell the encodings apart")
			}

			if got := hover(t, c, encRel, use); !strings.Contains(got, "lastPage : Int") {
				t.Errorf("hover %q", got)
			}
			locs := definitionAt(t, c, encRel, use)
			if len(locs) != 1 || locs[0].Range.Start.Line != strings.Count(text[:strings.Index(text, "\nlastPage :")+1], "\n") {
				t.Fatalf("definition %+v", locs)
			}
			refs := refsAt(t, c, encRel, use, false)
			var onLine *Location
			for k := range refs {
				if refs[k].Range.Start.Line == ln {
					onLine = &refs[k]
				}
			}
			start := atCol(c, e.enc, encRel, ln, "lastPage", 0)
			if onLine == nil || onLine.Range.Start != start || onLine.Range.End.Character != start.Character+len("lastPage") {
				t.Errorf("references %+v, want the use at %+v", refs, start)
			}

			// completion: the edit replaces the word before the cursor, in the client's unit
			typed := `  let é = "é😀" == Text.tr`
			p := typing(t, c, encRel, encLine+"0\n", typed)
			p.Character = wireCol(e.enc, typed, len(typed))
			it := labels(complete(t, c, encRel, p))["trim"]
			if want := (Range{Start: Position{p.Line, p.Character - 2}, End: p}); it.TextEdit.Range != want {
				t.Errorf("completion range %+v, want %+v", it.TextEdit.Range, want)
			}

			// a diagnostic after the characters
			c.change(encRel, good[:i]+encLine+"nope\n"+good[i+len(encLine)+2:])
			var found bool
			for _, d := range c.diags(encRel) {
				if d.Range.Start.Line != ln {
					continue
				}
				found = true
				want := atCol(c, e.enc, encRel, ln, "nope", 0)
				if d.Range.Start != want || d.Range.End.Character != want.Character+len("nope") {
					t.Errorf("diagnostic %+v, want it at %+v", d.Range, want)
				}
			}
			if !found {
				t.Error("no diagnostic on the test line")
			}
		})
	}
}

// TestEncoding_Format: a format edit that ends at the text's end (no final newline) names the
// end of a last line holding é and an astral character.
func TestEncoding_Format(t *testing.T) {
	for _, e := range encodings {
		t.Run(e.name, func(t *testing.T) {
			c := startOffering(t, blogCopy(t), Options{}, e.offered)
			last := `-- é😀 `
			src := strings.TrimRight(c.disk(encRel), "\n") + "\n" + last
			c.open(encRel, src)
			c.diags(encRel)
			var edits []TextEdit
			if err := c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": c.uri(encRel)}, "options": map[string]any{}}, &edits); err != nil {
				t.Fatal(err)
			}
			n := strings.Count(src, "\n")
			for _, ed := range edits {
				if ed.Range.End.Line == n && ed.Range.End.Character != 0 {
					if want := wireCol(e.enc, last, len(last)); ed.Range.End.Character != want {
						t.Errorf("end %+v, want character %d", ed.Range.End, want)
					}
					return
				}
			}
			t.Logf("no edit ends on the last line (%+v); nothing to check", edits)
		})
	}
}
