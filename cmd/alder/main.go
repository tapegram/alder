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
	"sync"
	"syscall"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/style"
	"alder/internal/version"
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
	case "fmt":
		os.Exit(runFmtTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "build":
		os.Exit(runBuildTo(os.Args[2:], os.Stdout, os.Stderr))
	case "run":
		stop := make(chan struct{})
		var once sync.Once
		end := func() { once.Do(func() { close(stop) }) }
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sig
			signal.Stop(sig) // a second Ctrl-C kills alder run outright
			end()
		}()
		// ALDER_EXIT_ON_STDIN_EOF=1 (set by a program that owns alder run, such as alder-tui, and
		// holds the other end of its standard input) stops it as Ctrl-C does once standard input
		// reaches EOF, so it never outlives its owner, even one killed with SIGKILL. The variable
		// is cleared once read, so the app does not inherit it from here (childEnv sets it for the
		// app's own stdin pipe).
		if os.Getenv("ALDER_EXIT_ON_STDIN_EOF") == "1" {
			os.Unsetenv("ALDER_EXIT_ON_STDIN_EOF")
			go func() {
				_, _ = io.Copy(io.Discard, os.Stdin)
				end()
			}()
		}
		os.Exit(runRunTo(os.Args[2:], os.Stdout, os.Stderr, stop))
	case "tui":
		os.Exit(runTuiTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "routes":
		os.Exit(runRoutesTo(os.Args[2:], os.Stdout, os.Stderr))
	case "dump":
		os.Exit(runDumpTo(os.Args[2:], os.Stdout, os.Stderr))
	case "eval":
		os.Exit(runEvalTo(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "scratch":
		os.Exit(runScratchTo(os.Args[2:], os.Stdout, os.Stderr))
	case "test":
		os.Exit(runTestTo(os.Args[2:], os.Stdout, os.Stderr))
	case "bench":
		os.Exit(runBenchTo(os.Args[2:], os.Stdout, os.Stderr))
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
	case "theme":
		os.Exit(runThemeTo(os.Args[2:], os.Stdout, os.Stderr))
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
		os.Exit(explainTo(os.Args[2], os.Stdout, os.Stderr))
	default:
		usage()
		os.Exit(2)
	}
}

// explainTo is `alder explain <code>`: the explanation as one line, or on a terminal wrapped to
// the terminal's width (at most 100 columns).
func explainTo(code string, stdout, stderr io.Writer) int {
	text, ok := diag.Explain[code]
	if !ok {
		fmt.Fprintf(stderr, "unknown diagnostic code %s\n", code)
		return 2
	}
	if st := style.For(stdout); st.On() {
		text = st.Wrap(text, min(st.Width(), 100))
	}
	fmt.Fprintln(stdout, text)
	return 0
}

// errTag is the `alder <command>:` prefix of an error line on w, bold red on a terminal. It is
// used by the commands that have no --json mode; the ones that have style nothing in JSON mode.
func errTag(w io.Writer, tag string) string { return style.For(w).BoldRed(tag) }

// internalError reports a bug in alder itself on w: a panic a command recovered, or an error a
// pass returns for a program it cannot handle. The user's program is not at fault, and saying so
// is the point — an internal error surfaces in the dev panel beside ordinary diagnostics, where a
// bare pass name ("gen: cannot emit <nil>") reads as "your code is broken and you cannot see how".
//
// detail is printed as it is, for the report. ALDER_DEBUG=1 adds the stack where one was kept.
func internalError(w io.Writer, tag string, detail any) {
	fmt.Fprintf(w, "%s internal error: %v\n", errTag(w, tag), detail)
	fmt.Fprintf(w, "  This is a bug in alder, not in your program.\n")
	fmt.Fprintf(w, "  alder %s (go %s)\n", version.String(), version.Go())
	fmt.Fprintf(w, "  Please report it: %s\n", issueURL)
	if os.Getenv("ALDER_DEBUG") == "" {
		fmt.Fprintf(w, "  Re-run with ALDER_DEBUG=1 for a stack trace.\n")
	}
}

// issueURL is where an internal error asks to be reported.
const issueURL = "https://github.com/tapegram/alder/issues/new"

func usage() { usageTo(os.Stderr) }

// usageTo prints the overall usage: one row per command of commandTree (the tree that
// `alder help --json` serves), names bold on a terminal. A command's own usage is
// `alder <command> --help`.
func usageTo(w io.Writer) {
	st := style.For(w)
	fmt.Fprint(w, usageText(st))
}

// usageText is the overall usage as text; with style.Plain it is the book's Commands block.
func usageText(st style.Style) string {
	rows := make([][]string, len(commandTree))
	for i, c := range commandTree {
		rows[i] = []string{c.Name, c.Summary}
	}
	var sb strings.Builder
	sb.WriteString("usage: alder <command> [arguments]\n\n")
	sb.WriteString(style.ColumnsStyled("  ", rows, func(col int, cell string) string {
		if col == 0 {
			return st.Bold(cell)
		}
		return cell
	}))
	sb.WriteString("\nalder <command> --help prints its usage; alder help --json prints the command tree\n")
	return sb.String()
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
		fmt.Fprint(w, diag.RenderTextStyled(d, sources[d.Span.File], style.For(w)))
	}
	if holes := countHoles(bag.Items); holes > 0 && !jsonOut { // "holes left" is not "build failed"
		fmt.Fprintf(stderr, "alder check: %d typed %s left\n", holes, map[bool]string{true: "hole", false: "holes"}[holes == 1])
	}
	if st := style.For(stderr); st.On() && !jsonOut && len(bag.Items) > 0 {
		fmt.Fprintln(stderr, countLine(bag.Items, st))
	}
	if bag.HasErrors() {
		return 1
	}
	return 0
}

// countLine is `2 errors, 1 warning` for ds, bold red when any is an error, else yellow: the
// last line `alder check` prints on a terminal.
func countLine(ds []diag.Diagnostic, st style.Style) string {
	errs, warns := 0, 0
	for _, d := range ds {
		if d.Severity == "error" {
			errs++
		} else {
			warns++
		}
	}
	plural := func(n int, word string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s", word)
		}
		return fmt.Sprintf("%d %ss", n, word)
	}
	var parts []string
	if errs > 0 {
		parts = append(parts, plural(errs, "error"))
	}
	if warns > 0 {
		parts = append(parts, plural(warns, "warning"))
	}
	line := strings.Join(parts, ", ")
	if errs > 0 {
		return st.BoldRed(line)
	}
	return st.Yellow(line)
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
