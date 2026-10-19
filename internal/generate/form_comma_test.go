package generate

import (
	"testing"

	"alder/internal/syntax"
)

func TestLastArmLine(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"empty match", "match msg {\n}", 0},
		{"comment only", "match msg {\n  -- note\n}", 0},
		{"one arm", "match msg {\n  Go -> model,\n}", 2},
		{"arm then comment", "match msg {\n  Go -> model\n  -- note\n}", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "update msg model = " + tc.body + "\n"
			m, _ := syntax.ParseFile("x.alder", []byte(src))
			e := newSrcEdit(src)
			close, _, _, ok := updateArmSpot(e, m)
			if !ok {
				t.Fatal("updateArmSpot not ok")
			}
			if got := lastArmLine(e, m, close); got != tc.want {
				t.Fatalf("lastArmLine = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestWithTrailingComma(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		changed  bool
	}{
		{"  Go -> model", "  Go -> model,", true},
		{"  Go -> model,", "  Go -> model,", false},
		{"  Go -> model  ", "  Go -> model,", true},
		{"  Go -> model -- last", "  Go -> model, -- last", true},
		{"  Go -> model, -- last", "  Go -> model, -- last", false},
		{`  Go -> say "a -- b"`, `  Go -> say "a -- b",`, true},
		{`  Go -> say "a -- b" -- c`, `  Go -> say "a -- b", -- c`, true},
	} {
		got, changed := withTrailingComma(tc.in)
		if got != tc.want || changed != tc.changed {
			t.Errorf("withTrailingComma(%q) = %q, %v; want %q, %v", tc.in, got, changed, tc.want, tc.changed)
		}
	}
}
