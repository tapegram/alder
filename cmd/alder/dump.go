package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/ir"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

// runDumpTo is `alder dump [--tokens|--ast|--ir] <file.alder | dir>`. The default (--ast) prints
// syntax.Dump of each file's AST, one module per line; --tokens prints the tokens the parser
// consumed, one per line as `L:C KIND "text"` (EOF and empty text runs omitted); for a dir both
// cover every .alder file under dir/app. --ir needs an app dir: it checks the app and prints
// ir.Dump of the lowered program.
// The dump goes to stdout and diagnostics to stderr. Exit 0 when clean, 1 when there are errors
// (the AST and tokens are still printed; the IR is not), 2 for usage, I/O and internal errors.
func runDumpTo(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder dump: internal error: %v\n", r)
			code = 2
		}
	}()
	mode, path := "", ""
	for _, a := range args {
		switch {
		case (a == "--tokens" || a == "--ast" || a == "--ir") && mode == "":
			mode = a
		case strings.HasPrefix(a, "-") || path != "":
			usageTo(stderr)
			return 2
		default:
			path = a
		}
	}
	if path == "" {
		usageTo(stderr)
		return 2
	}
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(stderr, "alder dump: %v\n", err)
		return 2
	}
	if mode == "--ir" {
		if !info.IsDir() {
			fmt.Fprintf(stderr, "alder dump: --ir needs an app directory, not a file: %s\n", path)
			return 2
		}
		return dumpIR(path, stdout, stderr)
	}
	files := []string{path}
	if info.IsDir() {
		files = nil
		werr := filepath.WalkDir(filepath.Join(path, "app"), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".alder") {
				files = append(files, p)
			}
			return nil
		})
		if werr != nil {
			fmt.Fprintf(stderr, "alder dump: %v\n", werr)
			return 2
		}
		if len(files) == 0 {
			fmt.Fprintf(stderr, "alder dump: no .alder files under %s/app\n", path)
			return 2
		}
		sort.Strings(files)
	}
	failed := false
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(stderr, "alder dump: %v\n", err)
			return 2
		}
		var diags []diag.Diagnostic
		if mode == "--tokens" {
			var toks []syntax.Token
			toks, diags = syntax.Tokens(f, src)
			if len(files) > 1 {
				fmt.Fprintf(stdout, "-- %s\n", f)
			}
			for _, t := range toks {
				if t.Kind != syntax.EOF && (t.Kind != syntax.Text || t.Text != "") {
					fmt.Fprintln(stdout, tokenLine(t))
				}
			}
		} else {
			var m *syntax.Module
			m, diags = syntax.ParseFile(f, src)
			fmt.Fprintln(stdout, syntax.Dump(m))
		}
		lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
		for _, d := range diags {
			fmt.Fprint(stderr, diag.RenderText(d, lines))
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

// tokenLine renders a token as `L:C KIND "text"`, with the text quoted as a Go string so that
// newlines and the whitespace of HTML text runs are visible.
func tokenLine(t syntax.Token) string {
	return fmt.Sprintf("%d:%d %s %q", t.Line, t.Col, t.Kind, t.Text)
}

// dumpIR checks the app in dir like `alder check` and, when it has no errors and no holes,
// prints the IR of the lowered program.
func dumpIR(dir string, stdout, stderr io.Writer) int {
	std, err := types.LoadStd()
	if err != nil {
		fmt.Fprintf(stderr, "alder: internal error loading stdlib: %v\n", err)
		return 2
	}
	prog, bag, ioErrs := names.Load(dir, std)
	if len(ioErrs) > 0 {
		for _, e := range ioErrs {
			fmt.Fprintf(stderr, "alder dump: %v\n", e)
		}
		return 2
	}
	if len(prog.Modules) == 0 {
		fmt.Fprintf(stderr, "alder dump: no .alder files under %s/app\n", dir)
		return 2
	}
	var res *check.Result
	if !prog.ParseFailed {
		prog.Resolve(bag)
		prog.MarkFailed(bag)
		res = check.Program(prog, bag)
	}
	bag.Sort()
	sources := map[string][]string{}
	for _, m := range prog.Modules {
		sources[m.File] = m.Lines
	}
	for _, d := range bag.Items {
		fmt.Fprint(stderr, diag.RenderText(d, sources[d.Span.File]))
	}
	if bag.HasErrors() || res == nil {
		return 1
	}
	if res.Holes > 0 {
		fmt.Fprintf(stderr, "alder dump: the program has %d typed hole(s); fill them before dumping the IR\n", res.Holes)
		return 1
	}
	fmt.Fprint(stdout, ir.Dump(ir.Lower(prog, res)))
	return 0
}
