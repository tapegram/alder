// internal/syntax/parser_test.go
package syntax

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestParseGolden(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/golden/parse_*.alder")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			m, diags := ParseFile(filepath.Base(f), src)
			var sb strings.Builder
			sb.WriteString(Dump(m))
			sb.WriteString("\n")
			for _, d := range diags {
				fmt.Fprintf(&sb, "! %s %d:%d %s\n", d.Code, d.Span.Start.Line, d.Span.Start.Col, d.Message)
			}
			got := sb.String()
			goldenPath := strings.TrimSuffix(f, ".alder") + ".dump"
			if *update {
				os.WriteFile(goldenPath, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden %s (run with -update)", goldenPath)
			}
			if got != string(want) {
				t.Fatalf("golden mismatch for %s\n got: %s\nwant: %s", f, got, want)
			}
		})
	}
}

func TestParse_UnclosedElement(t *testing.T) {
	src := "module U\nv : Html a\nv = <div><p>x</div>"
	_, diags := ParseFile("u.alder", []byte(src))
	var msgs []string
	for _, d := range diags {
		if d.Code != "E0001" {
			t.Errorf("unexpected code %s: %s", d.Code, d.Message)
		}
		msgs = append(msgs, d.Message)
	}
	want := []string{"closing tag </div> does not match <p>", "unclosed element <div>"}
	if strings.Join(msgs, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q want %q", msgs, want)
	}
}
