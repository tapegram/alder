package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alder/internal/generate"
)

// The admin (`alder new admin`, `alder regen admin`; book chapter 15) on the fixture, whose
// tables have a reference of each delete rule, a union, a Bool, an `@owner`, and whose session
// has a role with an Admin constructor. Its files are goldens (genCases' "fixture-admin"); here:
// the refusals, the relocation flags, --dry-run and --json, determinism, and the mutation gate.

func init() {
	// the version in the admin's headers: `alder version`'s in a real binary, fixed here so the
	// goldens do not follow the build
	adminVersion = func() string { return "v0.0.0-test" }
}

// TestAdmin_Refusals: no role, an admin that exists, no admin to regenerate, broken markers,
// flags that are not a module or a path: one sentence each, exit 1, nothing written.
func TestAdmin_Refusals(t *testing.T) {
	withAdmin := genApp(t, "fixture")
	if code, _, errs := alderIn(t, withAdmin, "", "new", "admin"); code != 0 {
		t.Fatalf("alder new admin: exit %d: %s", code, errs)
	}

	for _, c := range []struct {
		dir  string
		args []string
		want string
	}{
		{genApp(t, "scaffold"), []string{"new", "admin"}, "alder new admin: " + generate.AdminNeeds},
		{withAdmin, []string{"new", "admin"}, "alder new admin: `app/Admin/` exists: `alder regen admin` updates the admin in it; to generate another, choose another `--module`"},
		{genApp(t, "fixture"), []string{"new", "admin", "--module", "admin"}, "alder new admin: `--module admin` is not a module name: write capitalised names joined by dots, such as `Admin`"},
		{genApp(t, "fixture"), []string{"new", "admin", "--path", "/admin/{id}"}, "alder new admin: `--path /admin/{id}` is not a path without parameters, such as `/admin`"},
	} {
		before := appFiles(t, c.dir)
		code, out, errs := alderIn(t, c.dir, "", c.args...)
		if code != 1 || errs != c.want+"\n" {
			t.Errorf("alder %s: exit %d\nstderr: %s\nwant:   %s\nstdout: %s", strings.Join(c.args, " "), code, errs, c.want, out)
		}
		if ch := changedFiles(before, appFiles(t, c.dir)); len(ch) != 0 {
			t.Errorf("alder %s wrote %v", strings.Join(c.args, " "), slices.Sorted(maps.Keys(ch)))
		}
	}
}

// TestAdmin_Relocated: --module and --path put the admin elsewhere; it checks and its tests
// pass there.
func TestAdmin_Relocated(t *testing.T) {
	needGoTool(t)
	dir := genApp(t, "fixture")
	code, out, errs := alderIn(t, dir, "", "new", "admin", "--module", "Back.Office", "--path", "/back/")
	if code != 0 || !strings.Contains(out, "wrote app/Back/Office/Post/Index.alder\n") || !strings.Contains(out, "wrote app/Back/Office/Layout.alder\n") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "app", "Back", "Office", "Post", "Index.alder"))
	for _, want := range []string{"module Back.Office.Post.Index\n", `page "/back/post" {`, "<Back.Office.Layout.Page title=\"Posts\">", "<al-button href={Back.Office.Post.New {}}>New post</al-button>"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("Back.Office.Post.Index lacks %q:\n%s", want, b)
		}
	}
	if code, out, errs := alderIn(t, dir, "", "test"); code != 0 || !strings.HasSuffix(out, " passed, 0 failed\n") {
		t.Fatalf("alder test: exit %d\n%s%s", code, out, errs)
	}
}

// TestAdmin_DryRunJSON: --dry-run writes nothing and says what it would; --json is one object;
// two apps get the same files.
func TestAdmin_DryRunJSON(t *testing.T) {
	dir := genApp(t, "fixture")
	before := appFiles(t, dir)
	code, out, _ := alderIn(t, dir, "", "new", "admin", "--dry-run")
	if code != 0 || !strings.HasPrefix(out, "would write app/Admin/Comment/Delete.alder\n") || !strings.Contains(out, "--- /dev/null\n+++ b/app/Admin/Index.alder\n") || !strings.HasSuffix(out, "dry run: nothing written\n") {
		t.Errorf("--dry-run: exit %d\n%.2000s", code, out)
	}
	if ch := changedFiles(before, appFiles(t, dir)); len(ch) != 0 {
		t.Errorf("--dry-run wrote %v", slices.Sorted(maps.Keys(ch)))
	}
	code, out, _ = alderIn(t, dir, "", "new", "admin", "--json")
	var got struct {
		Files   []struct{ Path, Action string }
		Written bool
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 || strings.Count(out, "\n") != 1 || len(got.Files) != 22 || !got.Written || got.Files[0].Path != "app/Admin/Comment/Delete.alder" || got.Files[0].Action != "created" {
		t.Errorf("--json: exit %d, %v\n%.500s", code, err, out)
	}
	other := genApp(t, "fixture")
	alderIn(t, other, "", "new", "admin")
	a, b := appFiles(t, dir), appFiles(t, other)
	for p := range a {
		if strings.HasPrefix(p, "app/Admin/") && a[p] != b[p] {
			t.Errorf("two runs wrote different %s", p)
		}
	}
}

// TestAdmin_Mutate: the admin's tests kill its mutants. The fixture with `alder new admin`
// scores at least 0.95 under `alder test --mutate`; a failure lists the survivors.
func TestAdmin_Mutate(t *testing.T) {
	needGoTool(t)
	if testing.Short() {
		t.Skip("runs every mutant of the fixture's admin")
	}
	dir := genApp(t, "fixture")
	if code, out, errs := alderIn(t, dir, "", "new", "admin"); code != 0 {
		t.Fatalf("alder new admin: exit %d\n%s%s", code, out, errs)
	}
	var out, errb bytes.Buffer
	code := runTestTo([]string{dir, "--mutate"}, &out, &errb)
	m := scoreRe.FindStringSubmatch(out.String())
	if code != 0 || m == nil {
		t.Fatalf("alder test --mutate: exit %d\n%s%s", code, out.String(), errb.String())
	}
	t.Logf("the fixture with its admin: %s", m[0])
	if _, survivors, ok := strings.Cut(out.String(), "survived:\n"); ok {
		t.Logf("survivors:\n%s", survivors)
	}
	if m[4] < "0.95" {
		_, survivors, _ := strings.Cut(out.String(), "survived:\n")
		t.Errorf("score %s < 0.95; survivors:\n%s", m[4], survivors)
	}
}
