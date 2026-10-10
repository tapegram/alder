package rt

import (
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
// after a never-ending evaluation had finished. rt is initialized before the app's variables:
// the Go spec initializes a package's imports before the package itself, and an app that has a
// union imports alder/rt (rt.RegisterUnion); an app that does not is still preceded by rt,
// which the main imports (rt.Show), because the spec (Go 1.21 and later) initializes the
// packages that are ready in import-path order, and "alder/rt" sorts before "app".
func init() {
	if os.Getenv(EnvStdinEOF) == "1" {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			os.Exit(2)
		}()
	}
}
