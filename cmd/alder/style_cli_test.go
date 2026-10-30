package main

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// cliRun is one command as the style tests run it: its stdout, stderr and exit code.
type cliRun struct {
	code        int
	stdout, err string
}

// volatile are the parts of a run that differ between two runs of the same command: a test's
// milliseconds and the random ids of the rows it made.
var volatile = regexp.MustCompile(`"ms":[0-9.eE+-]+|[A-Za-z0-9_-]{20}`)

func runIn(f func(out, errb *bytes.Buffer) int) cliRun {
	var out, errb bytes.Buffer
	code := f(&out, &errb)
	return cliRun{code, volatile.ReplaceAllString(out.String(), "~"), errb.String()}
}

// jsonCommands are the --json modes that must never carry a style, each as a closure that runs
// the command (on a scratch app of its own where it writes) and returns its output.
func jsonCommands(t *testing.T) map[string]func() cliRun {
	t.Helper()
	todos := todosDir(t)
	bd := testBuildDir(t)
	bad := t.TempDir()
	writeFile(t, filepath.Join(bad, "app", "X.alder"), "module X\n\nx : Text\nx = 1\n")
	newApp := genApp(t, "scaffold")
	dep := deployScaffold(t, scriptDecl, nil)
	return map[string]func() cliRun{
		"check --json": func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int { return runCheckTo([]string{bad, "--json"}, o, e) })
		},
		"routes --json": func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int { return runRoutesTo([]string{todos, "--json"}, o, e) })
		},
		"help --json": func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int {
				if err := writeHelpJSON(o); err != nil {
					return 2
				}
				return 0
			})
		},
		"eval --json": func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int {
				return runEvalTo([]string{"--build-dir", bd, "--json", todos, "1 + 1"}, strings.NewReader(""), o, e)
			})
		},
		"new page --dry-run --json": func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int {
				return runNewTo([]string{"page", "About", "/about", "--dir", newApp, "--dry-run", "--json"}, o, e)
			})
		},
		"test --json": func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int { return runTestTo([]string{"--build-dir", bd, todos, "--json"}, o, e) })
		},
		"deploy local --dry-run --json": func() cliRun {
			c := newDeployCmds(t)
			code, out, errs := c.deploy("local", dep, "--dry-run", "--json")
			return cliRun{code, volatile.ReplaceAllString(out, "~"), errs}
		},
	}
}

// TestCLI_JSONIgnoresStyle pins that every --json mode is byte-identical with and without
// ALDER_COLOR=always, and carries no escape sequence: agents read these.
func TestCLI_JSONIgnoresStyle(t *testing.T) {
	needGoTool(t)
	cmds := jsonCommands(t)
	for name, run := range cmds {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ALDER_COLOR", "never")
			plain := run()
			t.Setenv("ALDER_COLOR", "always")
			styled := run()
			if plain != styled {
				pl, sl := strings.Split(plain.stdout+"\n--stderr--\n"+plain.err, "\n"), strings.Split(styled.stdout+"\n--stderr--\n"+styled.err, "\n")
				for i := 0; i < max(len(pl), len(sl)); i++ {
					var a, b string
					if i < len(pl) {
						a = pl[i]
					}
					if i < len(sl) {
						b = sl[i]
					}
					if a != b {
						t.Errorf("--json output changed with ALDER_COLOR=always (exit %d vs %d), first at line %d\nplain:  %s\nstyled: %s", plain.code, styled.code, i, a, b)
						break
					}
				}
			}
			if strings.Contains(styled.stdout, "\x1b") || strings.Contains(styled.err, "\x1b") {
				t.Errorf("escape sequence in --json output: %q %q", styled.stdout, styled.err)
			}
		})
	}
}
