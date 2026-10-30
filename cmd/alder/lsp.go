package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"alder/internal/fill"
	"alder/internal/lsp"
)

// runLspTo is `alder lsp`: the language server, over stdin and stdout, for editors.
// ALDER_LSP_LOG names a file that receives every message in and out, and each request body and
// answer of the alder.fill command's model (never a header). The fill backend comes from the
// environment the editor started the server in (fill.Env).
func runLspTo(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, commandUsages["lsp"])
		return 2
	}
	o := lsp.Options{Log: stderr, Debounce: 150 * time.Millisecond, Poll: 300 * time.Millisecond, Migrate: migrateCopyAs}
	if p := os.Getenv("ALDER_LSP_LOG"); p != "" {
		if f, err := os.Create(p); err == nil {
			defer f.Close()
			o.Wire = f
		}
	}
	o.Fill = func() (fill.Backend, string) {
		b, model := fill.Env()
		if a, ok := b.(*fill.Anthropic); ok && o.Wire != nil {
			a.Log = o.Wire
		}
		return b, model
	}
	if err := lsp.Serve(stdin, stdout, o); err != nil {
		fmt.Fprintf(stderr, "alder lsp: %v\n", err)
		return 2
	}
	return 0
}
