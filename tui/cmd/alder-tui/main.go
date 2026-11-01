// Command alder-tui is the terminal UI for the Alder dev server. It drives the alder binary and
// reads its JSON feeds; it never parses alder's human-readable output.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"alder/tui/internal/alderbin"
)

// version is the release this binary was built as. GoReleaser sets it with -ldflags, since a
// nested module's build information does not carry the root module's tag.
var version = "(devel)"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("alder-tui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "localhost:8080", "the address of the dev server to attach to, or to start `alder run` on")
	noRun := fs.Bool("no-run", false, "attach to a dev server that is already running; never start one")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	bin, err := alderbin.Find(os.Getenv, exec.LookPath)
	if err != nil {
		fmt.Fprintln(stderr, "alder-tui:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	have, err := alderbin.Version(ctx, func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, bin, args...).Output()
	})
	if err != nil {
		fmt.Fprintln(stderr, "alder-tui:", err)
		return 1
	}
	if m := alderbin.Mismatch(version, have); m != "" {
		fmt.Fprintln(stderr, "alder-tui:", m)
	}
	_, _, _ = addr, noRun, dir // the screens arrive in the tasks that follow
	return 0
}
