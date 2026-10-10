package rt

import (
	"fmt"
	"io"
	"os"
)

// EnvStdinEOF is the variable (scratch.EnvStdinEOF) that tethers a scratch program to its
// parent: with it set to 1, the program exits (2) when its standard input reaches end of file.
// `alder eval` and `alder repl` run their programs with it and a stdin pipe whose write end only
// they hold (scratch.RunTethered), so a program, in a process group of its own and out of reach
// of the terminal's signals, still exits when its parent dies in any way, SIGKILL included.
const EnvStdinEOF = "ALDER_SCRATCH_STDIN_EOF"

// init starts the tether here, in rt, not in the scratch main: the generated package computes
// its plain values (an evaluation's `it`, a REPL `let`) as package variables while it
// initializes, before any init of the main package runs, so a tether there would start only
// after a never-ending evaluation had finished. rt is initialized before the app's variables
// because the app imports it: the Go spec initializes a package's imports before the package
// itself, and gen gives app__alder.go a blank import of alder/rt when nothing else in it uses
// rt. The variable is cleared once read, so a process the program starts does not inherit it
// (its stdin is not the tether). When the parent is gone the program says so on stderr and
// exits 2.
func init() {
	if os.Getenv(EnvStdinEOF) == "1" {
		os.Unsetenv(EnvStdinEOF)
		go func() {
			io.Copy(io.Discard, os.Stdin)
			fmt.Fprintln(os.Stderr, "alder: parent gone")
			os.Exit(2)
		}()
	}
}
