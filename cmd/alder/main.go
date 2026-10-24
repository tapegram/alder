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
	if wantsHelpJSON(os.Args[1:]) {
		if err := writeHelpJSON(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	if helpTo(os.Args[1:], os.Stdout) {
		os.Exit(0)
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
	case "routes":
		os.Exit(runRoutesTo(os.Args[2:], os.Stdout, os.Stderr))
	case "dump":
		os.Exit(runDumpTo(os.Args[2:], os.Stdout, os.Stderr))
	case "eval":
		os.Exit(runEvalTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "test":
		os.Exit(runTestTo(os.Args[2:], os.Stdout, os.Stderr))
	case "migrate":
		os.Exit(runMigrateTo(os.Args[2:], os.Stdout, os.Stderr))
	case "repl":
		// Ctrl-C does not stop the REPL: each one is a value on interrupt (it stops a running
		// evaluation, whose program runs out of the terminal's reach in a process group of its
		// own). SIGTERM and SIGHUP (the terminal closed) close terminate: the REPL stops a
		// running evaluation and exits 1
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		interrupt, terminate := make(chan struct{}, 1), make(chan struct{})
		go func() {
			for s := range sig {
				if s != os.Interrupt {
					close(terminate)
					signal.Reset(syscall.SIGTERM, syscall.SIGHUP) // a second one ends alder outright
					return
				}
				select {
				case interrupt <- struct{}{}:
				default: // one pending is enough
				}
			}
		}()
		os.Exit(replMain(os.Args[2:], os.Stdin, os.Stdout, os.Stderr, interrupt, terminate))
	case "new":
		os.Exit(runNewTo(os.Args[2:], os.Stdout, os.Stderr))
	case "edit":
		os.Exit(runEditTo(os.Args[2:], os.Stdout, os.Stderr))
	case "fill":
		os.Exit(runFillTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "remove":
		os.Exit(runRemoveTo(os.Args[2:], os.Stdout, os.Stderr))
	case "regen":
		os.Exit(runRegenTo(os.Args[2:], os.Stdout, os.Stderr))
	case "deploy":
		os.Exit(runDeployTo(os.Args[2:], os.Stdout, os.Stderr))
	case "status":
		os.Exit(runStatusTo(os.Args[2:], os.Stdout, os.Stderr))
	case "rollback":
		os.Exit(runRollbackTo(os.Args[2:], os.Stdout, os.Stderr))
	case "logs":
		os.Exit(runLogsTo(os.Args[2:], os.Stdout, os.Stderr))
	case "restore":
		os.Exit(runRestoreTo(os.Args[2:], os.Stdout, os.Stderr))
	case "silos":
		os.Exit(runSilosTo(os.Args[2:], os.Stdout, os.Stderr))
	case "ci":
		os.Exit(runCiTo(os.Args[2:], os.Stdout, os.Stderr))
	case "version":
		os.Exit(runVersionTo(os.Args[2:], os.Stdout, os.Stderr))
	case "lsp":
		os.Exit(runLspTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
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
	fmt.Fprintln(w, "usage: alder new <name> | alder new page|table|form|msg|capability|admin … | alder edit table|page|session … | alder remove page|table … | alder fill <Module>.<decl> - | alder regen admin | alder check [dir] [--json] | alder build [--emit] [--build-dir DIR] [dir] | alder run [--build-dir DIR] [dir] | alder routes [dir] [--json] | alder dump [--tokens|--ast|--ir] <file.alder | dir> | alder eval [dir] [--run] [--json] [--build-dir DIR] [--in Name | --module Name] <expr | -> | alder test [dir] [-run re] [-timeout d] [--json] [--build-dir DIR] [--mutate [--min R]] | alder repl [dir] [--build-dir DIR] [--in Name | --module Name] | alder migrate [dir] [--name slug] [--dry-run] | alder migrate silos <env> [--allow-data-loss] | alder deploy <env> [dir] [--build-dir DIR] [--host name] [--allow-data-loss] [--unsafe-skip-tests] [--dry-run] [--json] | alder deploy init render|fly [dir] [--force] [--dry-run] | alder status|logs <env> | alder rollback <env> [<id>] | alder restore <env> [--silo key:id] [--from url] [--to time] [--out file] | alder silos prune <env> [--yes] | alder ci init [dir] | alder lsp | alder explain <code> | alder version")
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
	prog, _, bag, ioErrs := checkedApp(dir)
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
	if holes := countHoles(bag.Items); holes > 0 && !jsonOut { // "holes left" is not "build failed"
		fmt.Fprintf(stderr, "alder check: %d typed %s left\n", holes, map[bool]string{true: "hole", false: "holes"}[holes == 1])
	}
	if bag.HasErrors() {
		return 1
	}
	return 0
}

// countHoles is the number of typed holes (W0002) among ds.
func countHoles(ds []diag.Diagnostic) int {
	n := 0
	for _, d := range ds {
		if d.Code == "W0002" {
			n++
		}
	}
	return n
}
