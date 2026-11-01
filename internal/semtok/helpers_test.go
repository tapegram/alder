package semtok

import (
	"strings"
	"testing"
)

// where is the 1-based line and byte column of needle's first occurrence in src, plus off bytes.
func where(t *testing.T, src, needle string, off int) (line, col int) {
	t.Helper()
	i := strings.Index(src, needle)
	if i < 0 {
		t.Fatalf("%q is not in the source", needle)
	}
	i += off
	return strings.Count(src[:i], "\n") + 1, i - strings.LastIndex(src[:i], "\n")
}

// tokAt is the token starting at needle+off, and whether there is one.
func tokAt(t *testing.T, src string, toks []Token, needle string, off int) (Token, bool) {
	t.Helper()
	line, col := where(t, src, needle, off)
	for _, k := range toks {
		if k.Line == line && k.Col == col {
			return k, true
		}
	}
	return Token{}, false
}

// want checks the kind and modifiers of the token starting at needle+off.
func want(t *testing.T, src string, toks []Token, needle string, off int, kind Kind, mods int) {
	t.Helper()
	k, ok := tokAt(t, src, toks, needle, off)
	if !ok {
		t.Errorf("%q+%d: no token", needle, off)
		return
	}
	if k.Kind != kind || k.Mods != mods {
		t.Errorf("%q+%d: %s/%d, want %s/%d", needle, off, Types[k.Kind], k.Mods, Types[kind], mods)
	}
}

// wantLen is want plus the token's length in bytes.
func wantLen(t *testing.T, src string, toks []Token, needle string, off int, kind Kind, n int) {
	t.Helper()
	want(t, src, toks, needle, off, kind, 0)
	if k, ok := tokAt(t, src, toks, needle, off); ok && k.Len != n {
		t.Errorf("%q+%d: length %d, want %d", needle, off, k.Len, n)
	}
}

// wantNone checks that no token starts at needle+off.
func wantNone(t *testing.T, src string, toks []Token, needle string, off int) {
	t.Helper()
	if k, ok := tokAt(t, src, toks, needle, off); ok {
		t.Errorf("%q+%d: token %s, want none", needle, off, Types[k.Kind])
	}
}

// checkInvariants: sorted, non-overlapping, one line each and inside it, kinds and modifiers in the legend.
func checkInvariants(t *testing.T, label, src string, toks []Token) {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for i, k := range toks {
		switch {
		case k.Len <= 0, k.Line < 1, k.Line > len(lines), k.Col < 1, k.Col-1+k.Len > len(lines[k.Line-1]):
			t.Errorf("%s: token %d %+v is not inside its line", label, i, k)
		case k.Kind < 0 || int(k.Kind) >= len(Types) || k.Mods < 0 || k.Mods >= 1<<len(Modifiers):
			t.Errorf("%s: token %d %+v is outside the legend", label, i, k)
		case i > 0 && (toks[i-1].Line > k.Line || toks[i-1].Line == k.Line && toks[i-1].Col+toks[i-1].Len > k.Col):
			t.Errorf("%s: token %d %+v overlaps or precedes %+v", label, i, k, toks[i-1])
		}
	}
}
