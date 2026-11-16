// Package stdref writes the standard library reference, appendix D of the book, from the doc
// comments of stdlib/*.alder.
package stdref

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"alder/internal/doc"
	"alder/internal/syntax"
	"alder/stdlib"
)

// Markdown is the whole appendix: a title, then each module (alphabetically) with its doc and
// every declaration in file order. Examples are fenced `alder fragment`, so the book's test does
// not run each as its own `alder eval` (the stdlib doc tests in book/stdlibdocs_test.go run them).
func Markdown() (string, error) {
	names, err := fs.Glob(stdlib.FS, "*.alder")
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# Appendix D: Standard library reference\n\n")
	b.WriteString("This file is generated from the doc comments in `stdlib/`. To regenerate it, run `go test ./book -run TestAppendixD_Current -update`.\n")
	for _, n := range names {
		src, err := fs.ReadFile(stdlib.FS, n)
		if err != nil {
			return "", err
		}
		m, diags := syntax.ParseSignatures("stdlib/"+n, src)
		if len(diags) > 0 {
			return "", fmt.Errorf("stdlib/%s: %v", n, diags)
		}
		lines := strings.Split(string(src), "\n")
		fmt.Fprintf(&b, "\n## %s\n", m.Name)
		if body := doc.Parse(syntax.DocOf(lines, m.Span.Start.Line)).Body("alder fragment"); body != "" {
			b.WriteString("\n" + body + "\n")
		}
		for _, d := range m.Decls {
			var name string
			switch d := d.(type) {
			case *syntax.TypeDecl:
				name = d.Name
			case *syntax.ValueDecl:
				name = d.FullName()
			default:
				continue
			}
			sp := d.GetSpan()
			full := m.Name + "." + name
			if m.Name == "Prelude" {
				full = name
			}
			fmt.Fprintf(&b, "\n### `%s`\n\n", full)
			b.WriteString(doc.Fence("alder fragment", strings.Join(lines[sp.Start.Line-1:sp.End.Line], "\n")) + "\n")
			if body := doc.Parse(syntax.DocOf(lines, sp.Start.Line)).Body("alder fragment"); body != "" {
				b.WriteString("\n" + body + "\n")
			}
		}
	}
	return b.String(), nil
}
