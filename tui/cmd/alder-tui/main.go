// Command alder-tui is the terminal UI for the Alder dev server. It drives the alder binary and
// reads its JSON feeds; it never parses alder's human-readable output.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"alder/tui/internal/alderbin"
	"alder/tui/internal/runner"
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
	addr := fs.String("addr", "", "the address of the dev server to attach to, or to start `alder run` on (default $ALDER_ADDR, else localhost:8080)")
	noRun := fs.Bool("no-run", false, "attach to a dev server that is already running; never start one")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if *addr == "" {
		*addr = os.Getenv("ALDER_ADDR")
	}
	if *addr == "" {
		*addr = "localhost:8080"
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
		res, err := runner.Exec(bin).Run(ctx, dir, args...)
		if err != nil {
			return nil, err
		}
		if res.Code != 0 {
			return nil, fmt.Errorf("alder %s exited %d: %s", strings.Join(args, " "), res.Code, strings.TrimSpace(string(res.Stderr)))
		}
		return res.Stdout, nil
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
