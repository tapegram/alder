package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"alder/internal/version"
)

// tuiUsage is `alder tui --help`: the arguments are alder-tui's own, passed on unchanged.
const tuiUsage = "usage: alder tui [alder-tui arguments]"

// runTuiTo is runTuiWith with the real environment and PATH.
func runTuiTo(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runTuiWith(args, stdin, stdout, stderr, os.Getenv, exec.LookPath)
}

// runTuiWith hands the terminal to the separate alder-tui program: the one ALDER_TUI names, else
// the one lookPath finds as "alder-tui". It runs it as a child process with args, the
// environment plus ALDER_BIN (this alder's own path, so the TUI starts the alder that started
// it), and the given standard streams, and returns its exit code. While it runs, alder ignores
// SIGINT: the TUI handles Ctrl-C itself, and the whole foreground process group gets the signal.
// With no alder-tui to run it prints how to get one and returns 1.
func runTuiWith(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string, lookPath func(string) (string, error)) int {
	bin := getenv("ALDER_TUI")
	if bin == "" {
		p, err := lookPath("alder-tui")
		if err != nil {
			fmt.Fprintf(stderr, "alder tui: alder-tui is not installed; brew upgrade alder, or download it from the release that matches alder %s\n", version.String())
			return 1
		}
		bin = p
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "alder tui: %v\n", err)
		return 1
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "ALDER_BIN="+self)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "alder tui: %v\n", err)
		return 1
	}
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(stderr, "alder tui: %v\n", err)
		return 1
	}
	return 0
}
