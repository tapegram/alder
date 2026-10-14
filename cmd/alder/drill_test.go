package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The E11 drill (book 15, "An app from the command line"): testdata/drill/t10-teams/script.txt
// builds the E9 teams app (T10) from `alder new teams` with `alder new`, `alder edit` and
// `alder fill` only, no file edited by hand. TestDrill replays it; the app's own tests pass,
// the E9 hidden tests for T10 pass once added to the app as the E9 harness adds them
// (experiments/e9-feedback/verify.sh: app/Hidden.alder, `alder test -run '^Hidden '`), and its
// tests score at least 0.90 under `alder test --mutate`.

const drillScript = "../../testdata/drill/t10-teams/script.txt"

// drillStep is one command of a drill script: its arguments after `alder` and its standard
// input (a here-document: the lines after a `<<'EOF'` up to `EOF`).
type drillStep struct {
	line  int
	args  []string
	stdin string
}

// parseDrill reads a drill script: one command a line, `#` comments and blank lines skipped.
func parseDrill(src string) ([]drillStep, error) {
	lines := strings.Split(src, "\n")
	var steps []drillStep
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "#") {
			continue
		}
		words, err := drillWords(l)
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", i+1, err)
		}
		if len(words) < 2 || words[0] != "alder" {
			return nil, fmt.Errorf("line %d: a step is an `alder` command: %q", i+1, l)
		}
		st := drillStep{line: i + 1, args: words[1:]}
		if n := len(st.args); strings.HasPrefix(st.args[n-1], "<<") {
			delim := strings.Trim(strings.TrimPrefix(st.args[n-1], "<<"), "'\"")
			st.args = st.args[:n-1]
			var in strings.Builder
			for i++; ; i++ {
				if i >= len(lines) {
					return nil, fmt.Errorf("line %d: the here-document has no %s line", st.line, delim)
				}
				if lines[i] == delim {
					break
				}
				in.WriteString(lines[i] + "\n")
			}
			st.stdin = in.String()
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// drillWords splits a command line as a shell does: words separated by spaces, '…' and "…"
// taken as they are.
func drillWords(s string) ([]string, error) {
	var words []string
	var w strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ':
			if in {
				words = append(words, w.String())
				w.Reset()
				in = false
			}
		case '\'', '"':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return nil, fmt.Errorf("an unterminated %c", c)
			}
			w.WriteString(s[i+1 : i+1+j])
			i += j + 1
			in = true
		default:
			w.WriteByte(c)
			in = true
		}
	}
	if in {
		words = append(words, w.String())
	}
	return words, nil
}

// TestDrill replays the drill on a new app: every step succeeds; then the app checks, its own
// tests pass, the hidden tests pass, and its tests score at least 0.90.
func TestDrill(t *testing.T) {
	needGoTool(t)
	if testing.Short() {
		t.Skip("builds an app and runs its mutants")
	}
	src, err := os.ReadFile(drillScript)
	if err != nil {
		t.Fatal(err)
	}
	steps, err := parseDrill(string(src))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "teams")
	if code, _, errs := runNew(t, dir); code != 0 {
		t.Fatalf("alder new teams: %s", errs)
	}
	before := appFiles(t, dir)
	for _, st := range steps {
		code, out, errs := alderIn(t, dir, st.stdin, st.args...)
		if code != 0 {
			t.Fatalf("%s:%d: alder %s: exit %d\n%s%s", drillScript, st.line, strings.Join(st.args, " "), code, out, errs)
		}
		if st.args[0] == "test" && !strings.HasSuffix(out, " passed, 0 failed\n") {
			t.Fatalf("%s:%d: alder test:\n%s", drillScript, st.line, out)
		}
	}
	if code, out, errs := alderIn(t, dir, "", "check"); code != 0 || strings.Contains(out, "warning") {
		t.Fatalf("alder check after the drill: exit %d\n%s%s", code, out, errs)
	}
	t.Logf("the drill: %d commands; %d files written", len(steps), len(changedFiles(before, appFiles(t, dir))))

	// the score, on the app's own tests (before the hidden ones are added)
	var out, errb bytes.Buffer
	code := runTestTo([]string{dir, "--mutate"}, &out, &errb)
	m := scoreRe.FindStringSubmatch(out.String())
	if code != 0 || m == nil {
		t.Fatalf("alder test --mutate: exit %d\n%s%s", code, out.String(), errb.String())
	}
	t.Logf("the drill's app: %s", m[0])
	if m[4] < "0.90" {
		_, survivors, _ := strings.Cut(out.String(), "survived:\n")
		t.Errorf("score %s < 0.90; survivors:\n%s", m[4], survivors)
	}

	// the hidden tests, as the E9 harness runs them
	hidden, err := os.ReadFile("../../experiments/e9-feedback/hidden/t10-teams/Hidden.alder")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "Hidden.alder"), hidden, 0o644); err != nil {
		t.Fatal(err)
	}
	total := strings.Count(string(hidden), "\ntest \"")
	out.Reset()
	errb.Reset()
	code = runTestTo([]string{dir, "-run", "^Hidden "}, &out, &errb)
	if want := fmt.Sprintf("%d passed, 0 failed\n", total); code != 0 || total == 0 || !strings.HasSuffix(out.String(), want) {
		t.Fatalf("the hidden tests: exit %d, want %q\n%s%s", code, want, out.String(), errb.String())
	}
	t.Logf("the hidden tests: %d of %d pass", total, total)
}

// TestDrill_Chapter: chapter 15's "An app from the command line" shows the drill's script, as
// it is: its `text` blocks, one after another, are the script's commands (the script without
// its first comment paragraph).
func TestDrill_Chapter(t *testing.T) {
	src, err := os.ReadFile(drillScript)
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(src), "\n\n")
	if !ok {
		t.Fatal("the script has no header paragraph")
	}
	ch, err := os.ReadFile("../../book/15-generators.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(ch), "\n## An app from the command line\n")
	if !ok {
		t.Fatal("chapter 15 has no section \"An app from the command line\"")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var blocks []string
	for _, part := range strings.Split(section, "\n```text\n")[1:] {
		block, _, _ := strings.Cut(part, "\n```\n")
		if strings.HasPrefix(block, "alder ") || strings.HasPrefix(block, "# ") {
			blocks = append(blocks, block+"\n")
		}
	}
	if got := strings.Join(blocks, "\n"); got != body {
		t.Errorf("the chapter's blocks differ from %s:\n%s", drillScript, firstDifference(got, body))
	}
}

// firstDifference shows the first line where got and want differ.
func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < max(len(g), len(w)); i++ {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a != b {
			return fmt.Sprintf("line %d:\n  chapter: %q\n  script:  %q", i+1, a, b)
		}
	}
	return "(equal)"
}
