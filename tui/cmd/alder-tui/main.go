// Command alder-tui is the terminal UI for the Alder dev server. It drives the alder binary and
// reads its JSON feeds; it never parses alder's human-readable output.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"alder/tui/internal/alderbin"
	"alder/tui/internal/devproc"
	"alder/tui/internal/runner"
	"alder/tui/internal/ui"
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
	addr := fs.String("addr", "", "the address of the dev server to attach to, or to start alder run on (default $ALDER_ADDR, else localhost:8080)")
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
	vctx, vcancel := context.WithTimeout(context.Background(), 10*time.Second)
	have, err := alderbin.Version(vctx, func(ctx context.Context, args ...string) ([]byte, error) {
		res, err := runner.Exec(bin).Run(ctx, dir, args...)
		if err != nil {
			return nil, err
		}
		if res.Code != 0 {
			return nil, fmt.Errorf("alder %s exited %d: %s", strings.Join(args, " "), res.Code, strings.TrimSpace(string(res.Stderr)))
		}
		return res.Stdout, nil
	})
	vcancel()
	if err != nil {
		fmt.Fprintln(stderr, "alder-tui:", err)
		return 1
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintln(stderr, "alder-tui:", err)
		return 1
	}
	// The program starts at once and opens the dev server off its update loop, so the terminal is
	// not blank while an owned dev server builds the app. The feed lives as long as ctx, so ctx
	// lasts until the program has exited; then stop waits for Open to return, if it was called,
	// and closes its session. stop is deferred too, so that a panic cannot leave alder run running.
	ctx, cancel := context.WithCancel(context.Background())
	var (
		sess     *devproc.Session
		called   atomic.Bool
		opened   = make(chan struct{})
		stopOnce sync.Once
	)
	open := func(line func(string)) (ui.Feed, error) {
		called.Store(true)
		defer close(opened)
		s, err := devproc.Open(ctx, devproc.Options{Dir: abs, Addr: *addr, Alder: bin, NoRun: *noRun, Lines: line})
		if err != nil {
			return ui.Feed{}, err
		}
		sess = s
		return ui.Feed{Events: s.Events, Owned: s.Owned}, nil
	}
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			if called.Load() {
				<-opened
			}
			if sess == nil {
				return
			}
			if err := sess.Close(); err != nil {
				fmt.Fprintln(stderr, "alder-tui: stopping alder run:", err)
			}
		})
	}
	defer stop()
	m := ui.New(ui.Options{
		App: filepath.Base(abs), Dir: abs, Addr: *addr, Version: version, AlderVersion: have,
		Open: open, Runner: runner.Exec(bin),
		HTTP: &http.Client{Timeout: 5 * time.Second}, Now: time.Now,
		OpenURL: openURL, Copy: copyText,
	})
	final, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(stdout)).Run()
	stop()
	if serr := ui.StartErr(final); serr != nil {
		fmt.Fprintln(stderr, "alder-tui:", serr)
		return 1
	}
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		fmt.Fprintln(stderr, "alder-tui:", err)
		return 1
	}
	return 0
}

// openURL opens url in the default browser.
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}

// copyText puts text on the clipboard with the platform's tool: pbcopy on macOS, clip on
// Windows, and wl-copy, xclip or xsel elsewhere, the first one found.
func copyText(text string) error {
	var tools [][]string
	switch runtime.GOOS {
	case "darwin":
		tools = [][]string{{"pbcopy"}}
	case "windows":
		tools = [][]string{{"clip"}}
	default:
		tools = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, t := range tools {
		if _, err := exec.LookPath(t[0]); err != nil {
			continue
		}
		cmd := exec.Command(t[0], t[1:]...)
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	return errors.New("no clipboard tool found")
}
