package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(runCheckTo(os.Args[2:], os.Stdout, os.Stderr))
	case "build":
		os.Exit(runBuildTo(os.Args[2:], os.Stdout, os.Stderr))
	case "dump":
		os.Exit(runDumpTo(os.Args[2:], os.Stdout, os.Stderr))
	case "explain":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		text, ok := diag.Explain[os.Args[2]]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown diagnostic code %s\n", os.Args[2])
			os.Exit(2)
		}
		fmt.Println(text)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() { usageTo(os.Stderr) }

func usageTo(w io.Writer) {
	fmt.Fprintln(w, "usage: alder check [dir] [--json] | alder build [--emit] [dir] | alder dump [--tokens|--ast|--ir] <file.alder | dir> | alder explain <code>")
}

func runCheckTo(args []string, stdout, stderr io.Writer) (code int) {
	// A checker bug must degrade to an internal error, never a stack trace.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder check: internal error: %v\n", r)
			code = 2
		}
	}()
	dir, jsonOut, haveDir := ".", false, false
	for _, a := range args {
		switch {
		case a == "--json":
			jsonOut = true
		case strings.HasPrefix(a, "-") || haveDir:
			usageTo(stderr)
			return 2
		default:
			dir, haveDir = a, true
		}
	}
	count := 0
	werr := filepath.WalkDir(filepath.Join(dir, "app"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".alder") {
			count++
		}
		return nil
	})
	if werr != nil && !errors.Is(werr, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "alder check: %v\n", werr)
		return 2
	}
	if count == 0 {
		fmt.Fprintf(stderr, "alder check: no .alder files under %s/app\n", dir)
		return 2
	}
	std, err := types.LoadStd()
	if err != nil {
		fmt.Fprintf(stderr, "alder: internal error loading stdlib: %v\n", err)
		return 2
	}
	// A parse error stops the pipeline; after that each module is checked unless its own name resolution failed.
	prog, bag, ioErrs := names.Load(dir, std)
	if len(ioErrs) > 0 {
		for _, e := range ioErrs {
			fmt.Fprintf(stderr, "alder check: %v\n", e)
		}
		return 2
	}
	if !prog.ParseFailed { // Load's bag also holds module-graph errors (cycles), which only gate their modules
		prog.Resolve(bag)
		prog.MarkFailed(bag)
		check.Program(prog, bag)
	}
	bag.Sort()
	sources := map[string][]string{}
	for _, m := range prog.Modules {
		sources[m.File] = m.Lines
	}
	for _, d := range bag.Items {
		if jsonOut {
			fmt.Fprintln(stdout, diag.RenderJSON(d))
			continue
		}
		w := stdout
		if d.Severity == "warning" {
			w = stderr
		}
		fmt.Fprint(w, diag.RenderText(d, sources[d.Span.File]))
	}
	if bag.HasErrors() {
		return 1
	}
	return 0
}
