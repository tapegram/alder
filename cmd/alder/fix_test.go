package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func runFix(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runFixTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

// todosOnPages copies testdata/fix/todos (examples/todos as it was before #160 moved it to
// routes) into a temporary directory and returns the copy's root.
func todosOnPages(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "todos")
	copyDir(t, "../../testdata/fix/todos", dir)
	return dir
}

// TestFix_Routes: `alder fix --routes` rewrites the todos app on pages to routes, prints a line
// per file changed and per hole, exits 1 for the holes, and `alder check` afterwards lists exactly
// those holes (W0002) and nothing else.
func TestFix_Routes(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	code, out, errs := runFix("--routes", "--dir", dir)
	if code != 1 {
		t.Fatalf("code %d (want 1: holes remain)\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
	for _, f := range []string{"app/App.alder", "app/Todos/Index.alder", "app/Todos/Show.alder", "app/Auth/Login.alder"} {
		if !strings.Contains(out, "rewrote "+f+"\n") {
			t.Errorf("no line for %s:\n%s", f, out)
		}
	}
	holeLine := regexp.MustCompile(`(?m)^(app/[^:]+):(\d+): hole: (.+)$`)
	var holes []string
	for _, m := range holeLine.FindAllStringSubmatch(out, -1) {
		holes = append(holes, m[1]+":"+m[2])
	}
	if len(holes) == 0 {
		t.Fatalf("no holes reported:\n%s", out)
	}
	if !strings.Contains(errs, "typed holes left") {
		t.Errorf("stderr does not count the holes:\n%s", errs)
	}
	for _, f := range []string{"app/Todos/Index.alder", "app/Todos/Show.alder", "app/App.alder"} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if regexp.MustCompile(`(?m)^(page|msg) `).Match(b) {
			t.Errorf("%s still has a page or msg:\n%s", f, b)
		}
	}

	var cout, cerr bytes.Buffer
	runCheckTo([]string{dir, "--json"}, &cout, &cerr)
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(cout.String()), "\n") {
		if line == "" {
			continue
		}
		var d struct {
			Code, File, Message string
			Line                int
		}
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("%v: %s", err, line)
		}
		if d.Code != "W0002" {
			t.Errorf("check lists more than the holes: %s", line)
			continue
		}
		got = append(got, d.File+":"+strconv.Itoa(d.Line))
	}
	slices.Sort(got)
	slices.Sort(holes)
	if !slices.Equal(got, holes) {
		t.Errorf("check's holes %v, fix reported %v\nfix:\n%s", got, holes, out)
	}

	// a second run has nothing left to rewrite
	code, out, errs = runFix("--routes", "--dir", dir)
	if code != 0 || strings.Contains(out, "rewrote") {
		t.Errorf("second run: code %d\n%s%s", code, out, errs)
	}
}

// TestFix_RoutesDryRun prints the changes as a diff and writes nothing.
func TestFix_RoutesDryRun(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	before, _ := os.ReadFile(filepath.Join(dir, "app/Todos/Index.alder"))
	code, out, errs := runFix("--routes", "--dry-run", "--dir", dir)
	if code != 1 {
		t.Fatalf("code %d\n%s%s", code, out, errs)
	}
	for _, w := range []string{"would rewrite app/Todos/Index.alder", "+route Index", "-page "} {
		if !strings.Contains(out, w) {
			t.Errorf("dry run lacks %q:\n%s", w, out)
		}
	}
	after, _ := os.ReadFile(filepath.Join(dir, "app/Todos/Index.alder"))
	if !bytes.Equal(before, after) {
		t.Error("--dry-run wrote the file")
	}
}

// TestFix_RoutesFiles rewrites only the files named.
func TestFix_RoutesFiles(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	app, _ := os.ReadFile(filepath.Join(dir, "app/App.alder"))
	code, out, errs := runFix("--routes", "--dir", dir, filepath.Join(dir, "app/Todos/Index.alder"))
	if code != 1 || !strings.Contains(out, "rewrote app/Todos/Index.alder") || strings.Contains(out, "rewrote app/App.alder") {
		t.Fatalf("code %d\n%s%s", code, out, errs)
	}
	if again, _ := os.ReadFile(filepath.Join(dir, "app/App.alder")); !bytes.Equal(app, again) {
		t.Error("a file not named was written")
	}
}

// TestFix_RefusesBrokenApp: the rewrite runs only on an app `alder check` accepts.
func TestFix_RefusesBrokenApp(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	f := filepath.Join(dir, "app/Todos/Index.alder")
	src, _ := os.ReadFile(f)
	broken := append(src, []byte("\nbroken : Int\nbroken = \"text\"\n")...)
	if err := os.WriteFile(f, broken, 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runFix("--routes", "--dir", dir)
	if code != 2 || !strings.Contains(errs, "alder check") || !strings.Contains(errs, "E0003") {
		t.Fatalf("code %d\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
	if again, _ := os.ReadFile(f); !bytes.Equal(broken, again) {
		t.Error("the broken app was rewritten")
	}
}

// TestFix_Usage: --routes is required.
func TestFix_Usage(t *testing.T) {
	t.Parallel()
	if code, _, errs := runFix(); code != 2 || !strings.Contains(errs, "usage: alder fix --routes") {
		t.Errorf("no args: code %d %s", code, errs)
	}
	if code, _, errs := runFix("--bogus"); code != 2 || !strings.Contains(errs, "usage:") {
		t.Errorf("--bogus: code %d %s", code, errs)
	}
}

// TestFix_RoutesOldAdmin: a page that `alder new admin` wrote is rewritten like any other, but its
// `@admin` regions no longer line up with the route lines, so the command says to regenerate the
// admin instead.
func TestFix_RoutesOldAdmin(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	old := "-- generated by `alder new admin`; regions between\n" +
		"-- `-- @admin begin <name>` / `-- @admin end` are rewritten by `alder regen admin`\n" +
		`module Admin.Old

page "/admin/old" { } requires user, role Admin

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

-- @admin begin go
msg Go
-- @admin end

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Go _ -> Task.succeed Rerender,
}

view : Model -> Session -> Html Msg
view model session = <button click={Go}>Go</button>
`
	if err := os.WriteFile(filepath.Join(dir, "app/Admin/Old.alder"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runFix("--routes", "--dir", dir)
	if code != 1 || !strings.Contains(out, "rewrote app/Admin/Old.alder") {
		t.Fatalf("code %d\n%s%s", code, out, errs)
	}
	if !strings.Contains(errs, "app/Admin/Old.alder: written by alder new admin") || !strings.Contains(errs, "alder new admin") {
		t.Errorf("no advice to regenerate the admin:\n%s", errs)
	}
}
