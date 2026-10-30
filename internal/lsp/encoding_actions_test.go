package lsp

import (
	"strings"
	"testing"
)

// TestEncoding_QuickFix: a diagnostic's fix, and the quick-fix action carrying it, name their
// range in the client's unit when the line has é and an astral character before it.
func TestEncoding_QuickFix(t *testing.T) {
	for _, e := range encodings {
		t.Run(e.name, func(t *testing.T) {
			c := startOffering(t, blogCopy(t), Options{}, e.offered)
			text := c.disk(encRel)
			i := strings.Index(text, afterQ) + len(afterQ)
			line := "  let e = Text.isEmpty \"é😀\" == Posts.Index.lastPage"
			c.open(encRel, text[:i]+line+"\n"+text[i:])
			ln := strings.Count(text[:i], "\n")
			from := atCol(c, e.enc, encRel, ln, "Posts.Index.lastPage", 0)
			want := Range{Start: from, End: Position{ln, from.Character + len("Posts.Index.lastPage")}}
			var d *Diagnostic
			for _, x := range c.diags(encRel) {
				if x.Data != nil && x.Data.Fix != nil {
					d = &x
				}
			}
			if d == nil || d.Range != want || d.Data.Fix.Range != want {
				t.Fatalf("diagnostic %+v, want range %+v", d, want)
			}
			acts := actions(t, c, encRel, ln)
			if len(acts) == 0 || acts[0].Edit == nil {
				t.Fatalf("actions %+v", acts)
			}
			if got := acts[0].Edit.DocumentChanges[0].Edits[0]; got.Range != want || got.NewText != "lastPage" {
				t.Errorf("edit %+v, want range %+v", got, want)
			}
			if got := acts[0].Diagnostics[0].Range; got != want {
				t.Errorf("action diagnostic range %+v, want %+v", got, want)
			}
		})
	}
}
