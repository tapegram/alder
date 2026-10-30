package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/style"
)

// The colour-only outputs: with ALDER_COLOR=always the text is the plain run's with escape
// sequences added and nothing else, and some part of it is styled.
func TestColourOnly_StripEqualsPlain(t *testing.T) {
	needGoTool(t)
	todos := todosDir(t)
	bd := testBuildDir(t)
	unformatted := t.TempDir()
	writeFile(t, filepath.Join(unformatted, "app", "X.alder"), "module X\n\nx : Int\nx   =   1\n")
	empty := t.TempDir()
	writeFile(t, filepath.Join(empty, "app", "keep.txt"), "")
	cases := []struct {
		name string
		run  func() cliRun
		want string // a substring that must differ between the runs (something is styled)
	}{
		{"routes", func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int { return runRoutesTo([]string{todos}, o, e) })
		}, "KIND"},
		{"eval", func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int {
				return runEvalTo([]string{"--build-dir", bd, todos, "1 + 1"}, strings.NewReader(""), o, e)
			})
		}, " : Int"},
		{"fmt --check", func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int {
				return runFmtTo([]string{"--check", unformatted}, strings.NewReader(""), o, e)
			})
		}, "would be reformatted"},
		{"help", func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int { usageTo(o); return 0 })
		}, "check"},
		{"build error line", func() cliRun {
			return runIn(func(o, e *bytes.Buffer) int { return runBuildTo([]string{empty}, o, e) })
		}, "alder build: no .alder files"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("ALDER_COLOR", "never")
			plain := c.run()
			t.Setenv("ALDER_COLOR", "always")
			styled := c.run()
			if strings.Contains(plain.stdout+plain.err, "\x1b") {
				t.Fatalf("plain run has escapes: %+v", plain)
			}
			if !strings.Contains(styled.stdout+styled.err, "\x1b[") {
				t.Errorf("nothing styled: %+v", styled)
			}
			if styled.code != plain.code || style.Strip(styled.stdout) != plain.stdout || style.Strip(styled.err) != plain.err {
				t.Errorf("Strip(styled) differs from plain:\nstyled %+v\nplain  %+v", styled, plain)
			}
			if !strings.Contains(plain.stdout+plain.err, c.want) {
				t.Errorf("plain output lacks %q: %+v", c.want, plain)
			}
		})
	}
}
