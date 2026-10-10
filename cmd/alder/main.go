package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"alder/internal/buildkit"
	"alder/internal/diag"
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
	case "run":
		stop := make(chan struct{})
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sig
			signal.Stop(sig) // a second Ctrl-C kills alder run outright
			close(stop)
		}()
		os.Exit(runRunTo(os.Args[2:], os.Stdout, os.Stderr, stop))
	case "dump":
		os.Exit(runDumpTo(os.Args[2:], os.Stdout, os.Stderr))
	case "eval":
		os.Exit(runEvalTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "test":
		os.Exit(runTestTo(os.Args[2:], os.Stdout, os.Stderr))
	case "repl":
		// Ctrl-C does not stop the REPL: each one is a value on interrupt (it stops a running
		// evaluation, whose program runs out of the terminal's reach in a process group of its
		// own); SIGTERM still ends the REPL
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		interrupt := make(chan struct{}, 1)
		go func() {
			for range sig {
				select {
				case interrupt <- struct{}{}:
				default: // one pending is enough
				}
			}
		}()
		os.Exit(runReplTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr, interrupt))
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
	fmt.Fprintln(w, "usage: alder check [dir] [--json] | alder build [--emit] [dir] | alder run [dir] | alder dump [--tokens|--ast|--ir] <file.alder | dir> | alder eval [dir] [--run] [--json] [--module Name] <expr | -> | alder test [dir] [-run re] [-timeout d] [--json] | alder repl [dir] [--module Name] | alder explain <code>")
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
	// A parse error stops the pipeline; after that each module is checked unless its own name resolution failed.
	prog, _, bag, ioErrs := buildkit.LoadChecked(dir, nil)
	if reportLoadErrs("alder check", ioErrs, stderr) {
		return 2
	}
	sources := buildkit.Sources(prog)
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
