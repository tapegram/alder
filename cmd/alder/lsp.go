package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"alder/internal/lsp"
)

// runLspTo is `alder lsp`: the language server, over stdin and stdout, for editors.
// ALDER_LSP_LOG names a file that receives every message in and out.
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
	if err := lsp.Serve(stdin, stdout, o); err != nil {
		fmt.Fprintf(stderr, "alder lsp: %v\n", err)
		return 2
	}
	return 0
}
