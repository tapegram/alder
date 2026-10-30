package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The `alder fill --ai` tests run the command backend (ALDER_FILL_COMMAND) on a shell script:
// no test calls a model. They set the environment, so they are not parallel; ANTHROPIC_API_KEY
// is blanked, never read.

// fakeModel sets ALDER_FILL_COMMAND to a script that answers with answers in turn (the last one
// again after the end), one line each, and keeps each prompt it reads; it returns the directory
// holding the prompts (stdin.1, stdin.2, …).
func fakeModel(t *testing.T, answers ...string) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "answers"), []byte(strings.Join(answers, "\n")+"\n"), 0o644)
	script := `cd '` + dir + `' || exit 9
n=$(cat count 2>/dev/null || echo 0); n=$((n+1)); echo $n > count
cat > stdin.$n
a=$(sed -n "${n}p" answers); [ -z "$a" ] && a=$(tail -n 1 answers)
printf '%s\n' "$a"
`
	os.WriteFile(filepath.Join(dir, "model.sh"), []byte(script), 0o755)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ALDER_FILL_COMMAND", "sh '"+filepath.Join(dir, "model.sh")+"'")
	return dir
}

// clearApp is the scaffold with `alder new msg Home Clear`: holes ?clear (Home.update.Clear)
// and ?expected (the test "pressing Clear").
func clearApp(t *testing.T) string {
	t.Helper()
	dir := genApp(t, "scaffold")
	if code, out, errs := alderIn(t, dir, "", "new", "msg", "Home", "Clear"); code != 0 {
		t.Fatalf("new msg: exit %d\n%s%s", code, out, errs)
	}
	return dir
}

func homeOf(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "app", "Home.alder"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFill_AI(t *testing.T) {
	dir := clearApp(t)
	fakeModel(t, "Task.succeed Rerender")
	code, out, errs := alderIn(t, dir, "", "fill", "Home.update.Clear", "--ai")
	if code != 0 || !strings.Contains(out, "edited app/Home.alder\n") || !strings.Contains(out, "+  Clear p -> Task.succeed Rerender,\n") || errs != "" {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr: %s", code, out, errs)
	}
	if !strings.Contains(homeOf(t, dir), "  Clear p -> Task.succeed Rerender,\n") {
		t.Errorf("not written:\n%s", homeOf(t, dir))
	}
}

func TestFill_AIDryRun(t *testing.T) {
	dir := clearApp(t)
	before := homeOf(t, dir)
	fakeModel(t, "Task.succeed Rerender")
	code, out, errs := alderIn(t, dir, "", "fill", "Home.update.Clear", "--ai", "--dry-run")
	if code != 0 || !strings.Contains(out, "would edit app/Home.alder\n") || !strings.Contains(out, "dry run: nothing written") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr: %s", code, out, errs)
	}
	if homeOf(t, dir) != before {
		t.Error("a dry run wrote")
	}
}

func TestFill_AIRefusedWritesNothing(t *testing.T) {
	dir := clearApp(t)
	before := homeOf(t, dir)
	prompts := fakeModel(t, "42")
	code, out, errs := alderIn(t, dir, "", "fill", "Home.update.Clear", "--ai")
	if code != 1 || !strings.Contains(errs, "alder fill: attempt 2 of 3: E0003 ") || !strings.Contains(errs, "alder fill: attempt 3 of 3: E0003 ") ||
		!strings.HasSuffix(errs, "alder fill: the result does not check after 3 attempts: nothing was written\n") ||
		!strings.Contains(out, "error[E0003]") || !strings.HasSuffix(out, "the model's last expression:\n42\n") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr: %s", code, out, errs)
	}
	if homeOf(t, dir) != before {
		t.Error("a refused fill wrote")
	}
	b, _ := os.ReadFile(filepath.Join(prompts, "stdin.3"))
	if !strings.Contains(string(b), "Your answer:\n42\n") || !strings.Contains(string(b), "The checker refused it:") {
		t.Errorf("the third prompt:\n%s", b)
	}
}

func TestFill_AIJSON(t *testing.T) {
	dir := clearApp(t)
	fakeModel(t, "Task.succeed Rerender")
	code, out, errs := alderIn(t, dir, "", "fill", "Home.update.Clear", "--ai", "--json")
	var got struct {
		Written    bool     `json:"written"`
		Expression string   `json:"expression"`
		Attempts   int      `json:"attempts"`
		Model      string   `json:"model"`
		Imports    []string `json:"imports"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr: %s", code, out, errs)
	}
	if !got.Written || got.Expression != "Task.succeed Rerender" || got.Attempts != 1 || got.Model != "command" || got.Imports == nil {
		t.Errorf("json %+v", got)
	}
	// refused: the error, the last expression, nothing written
	fakeModel(t, "42")
	code, out, _ = alderIn(t, dir, "", "fill", `Home."pressing Clear"`, "--ai", "--json")
	var bad map[string]any
	if code != 1 || json.Unmarshal([]byte(out), &bad) != nil || bad["written"] != false || bad["expression"] != "42" || bad["attempts"] != float64(3) ||
		bad["error"] != "the result does not check after 3 attempts: nothing was written" {
		t.Errorf("refused: exit %d\n%s", code, out)
	}
}

func TestFill_Prompt(t *testing.T) {
	dir := clearApp(t)
	prompts := fakeModel(t, "Task.succeed Rerender")
	code, out, errs := alderIn(t, dir, "", "fill", `Home."pressing Clear"`, "--prompt")
	if code != 0 || errs != "" || !strings.HasPrefix(out, "You fill one typed hole") ||
		!strings.Contains(out, "Fill the typed hole ?expected in \"pressing Clear\" of module Home (app/Home.alder).\nIt must have type: Bool\n") ||
		!strings.Contains(out, "-- Fill ?expected with what pressing Clear must do") || !strings.HasSuffix(out, "Reply with the expression only.\n") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr: %s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(prompts, "count")); err == nil {
		t.Error("--prompt ran the command")
	}
	// with ALDER_FILL_REFERENCE, the reference comes first, as the API is sent it
	ref := filepath.Join(t.TempDir(), "reference.md")
	os.WriteFile(ref, []byte("THE REFERENCE\n"), 0o644)
	t.Setenv("ALDER_FILL_REFERENCE", ref)
	if code, out, errs = alderIn(t, dir, "", "fill", `Home."pressing Clear"`, "--prompt"); code != 0 || !strings.HasPrefix(out, "THE REFERENCE\n\nYou fill one typed hole") {
		t.Errorf("with a reference: exit %d\n%.200s\n%s", code, out, errs)
	}
}

func TestFill_AIUsageErrors(t *testing.T) {
	dir := clearApp(t)
	fakeModel(t, "Task.succeed Rerender")
	for _, c := range []struct {
		args []string
		msg  string
	}{
		{[]string{"fill", "Home.update.Clear", "-", "--ai"}, "alder fill: --ai reads no standard input: drop the `-`"},
		{[]string{"fill", "Home.update.Clear", "-", "--prompt"}, "alder fill: --prompt reads no standard input: drop the `-`"},
		{[]string{"fill", "--decl", "Home.update", "--ai"}, "alder fill: --ai fills a hole: it takes no --list or --decl"},
		{[]string{"fill", "--list", "--ai"}, "alder fill: --ai fills a hole: it takes no --list or --decl"},
		{[]string{"fill", "Home.update.Clear", "--ai", "--prompt"}, "alder fill: --ai and --prompt are two ways to run: give one"},
		{[]string{"fill", "--ai"}, "usage: alder fill"},
	} {
		code, _, errs := alderIn(t, dir, "", c.args...)
		if code != 2 || !strings.HasPrefix(errs, c.msg) {
			t.Errorf("%v: exit %d, stderr %q", c.args, code, errs)
		}
	}
	// no backend: exit 2, the variables named
	t.Setenv("ALDER_FILL_COMMAND", "")
	code, _, errs := alderIn(t, dir, "", "fill", "Home.update.Clear", "--ai")
	if code != 2 || errs != "alder fill: set ANTHROPIC_API_KEY, or ALDER_FILL_COMMAND, in the environment\n" {
		t.Errorf("no backend: exit %d, %q", code, errs)
	}
}
