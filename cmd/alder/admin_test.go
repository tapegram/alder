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
// the refusals, the relocation flags, --dry-run and --json, determinism, the mutation gate, and
// `regen` keeping the author's text (Review Focus 2 and 5).

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
	broken := genApp(t, "fixture")
	alderIn(t, broken, "", "new", "admin")
	p := filepath.Join(broken, "app", "Admin", "Post", "Index.alder")
	b, _ := os.ReadFile(p)
	src := strings.Replace(string(b), "              -- @admin end\n            </tr>}", "            </tr>}", 1)
	if src == string(b) {
		t.Fatal("the cells region's end marker was not found")
	}
	os.WriteFile(p, []byte(src), 0o644)

	for _, c := range []struct {
		dir  string
		args []string
		want string
	}{
		{genApp(t, "scaffold"), []string{"new", "admin"}, "alder new admin: " + generate.AdminNeeds},
		{withAdmin, []string{"new", "admin"}, "alder new admin: `app/Admin/` exists: `alder regen admin` updates the admin in it; to generate another, choose another `--module`"},
		{genApp(t, "fixture"), []string{"regen", "admin"}, "alder regen admin: there is no admin in `app/Admin/`: `alder new admin` generates one (name another with --module)"},
		{broken, []string{"regen", "admin"}, "alder regen admin: `app/Admin/Post/Index.alder`: region `cells` has no end marker (region `tests` begins at line 120, inside it)"},
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
	if code, out, _ := alderIn(t, dir, "", "regen", "admin", "--module", "Back.Office", "--path", "/back"); code != 0 || out != "nothing to do: the admin is up to date\n" {
		t.Errorf("regen of the relocated admin: exit %d\n%s", code, out)
	}
}

// TestAdmin_DryRunJSON: --dry-run writes nothing and says what it would; --json is one object;
// two apps get the same files; a regen right after has nothing to do.
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
	code, out, _ = alderIn(t, dir, "", "regen", "admin", "--json")
	if code != 0 || !strings.Contains(out, `"files":[]`) {
		t.Errorf("regen --json after new: exit %d\n%s", code, out)
	}
	before = appFiles(t, dir)
	if code, out, _ := alderIn(t, dir, "", "regen", "admin"); code != 0 || out != "nothing to do: the admin is up to date\n" {
		t.Errorf("regen after new: exit %d\n%s", code, out)
	}
	if ch := changedFiles(before, appFiles(t, dir)); len(ch) != 0 {
		t.Errorf("a regen with nothing to do wrote %v", slices.Sorted(maps.Keys(ch)))
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

// outside is src with the lines of its owned regions left out (the markers kept): the author's
// text.
func outside(t *testing.T, src string) string {
	t.Helper()
	if err := generate.Regions.Check(src); err != nil {
		t.Fatal(err)
	}
	var out []string
	in := false
	for _, l := range strings.Split(src, "\n") {
		s := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(s, generate.RegionBegin):
			in = true
			out = append(out, l)
		case s == generate.RegionEnd:
			in = false
			out = append(out, l)
		case !in:
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// TestAdmin_Regen (Review Focus 2 and 5): the author customises the generated admin outside
// its regions (a heading, a column of their own, a test), then adds a field to Post and a
// table; `regen` writes the field into Post's list, show page and forms and the table's pages
// into the admin, and leaves every customisation byte for byte. The app checks and its tests,
// the author's among them, pass; a second regen has nothing to do. A page the author deleted
// stays deleted, unless --all.
func TestAdmin_Regen(t *testing.T) {
	needGoTool(t)
	dir := genApp(t, "fixture")
	if code, out, errs := alderIn(t, dir, "", "new", "admin"); code != 0 {
		t.Fatalf("alder new admin: exit %d\n%s%s", code, out, errs)
	}
	file := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write := func(rel, s string) {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	replace := func(rel, old, new string) {
		s := file(rel)
		if !strings.Contains(s, old) {
			t.Fatalf("%s has no %q", rel, old)
		}
		write(rel, strings.Replace(s, old, new, 1))
	}

	// the author's customisations, outside the regions
	index := "app/Admin/Post/Index.alder"
	replace(index, "    <h1>Posts</h1>\n", "    <h1>Every post</h1>\n")
	replace(index, "            -- @admin end\n          </tr>\n", "            -- @admin end\n            <th>Words</th>\n          </tr>\n")
	replace(index, "              -- @admin end\n            </tr>}", "              -- @admin end\n              <td class=\"words\">{Text.fromInt (List.length (Text.split \" \" row.body))}</td>\n            </tr>}")
	write(index, file(index)+`
test "the list counts each post's words" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me" }
  let _ <- Test.login { user: Just me, role: Admin }
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", body: "one two three", author: me }
  let page <- Test.get (Admin.Post.Index {})
  Task.succeed (Test.texts "td.words" page.doc == ["3"])
}
`)
	// a page the author does without: Tag's Delete page, and the button that went to it
	replace("app/Admin/Tag/Show.alder", "        <al-button variant=\"destructive\" href={Admin.Tag.Delete { id: model.row.id }}>Delete</al-button>\n", "")
	os.Remove(filepath.Join(dir, "app", "Admin", "Tag", "Delete.alder"))
	if code, out, errs := alderIn(t, dir, "", "check"); code != 0 || strings.Contains(out, "error") {
		t.Fatalf("the customised admin does not check: exit %d\n%s%s", code, out, errs)
	}
	before := appFiles(t, dir)

	// the schema changes: a field, and a table
	if code, out, errs := alderIn(t, dir, "", "edit", "table", "Post", "add-field", "views:Int=0"); code != 0 {
		t.Fatalf("alder edit table: exit %d\n%s%s", code, out, errs)
	}
	if code, out, errs := alderIn(t, dir, "", "new", "table", "Like", "post:Post.PostId@ref=Post@cascade", "owner:User.UserId@ref=User@owner"); code != 0 {
		t.Fatalf("alder new table: exit %d\n%s%s", code, out, errs)
	}
	mid := appFiles(t, dir)
	code, out, errs := alderIn(t, dir, "", "regen", "admin")
	if code != 0 {
		t.Fatalf("alder regen admin: exit %d\n%s%s", code, out, errs)
	}
	if !strings.Contains(out, "not recreated app/Admin/Tag/Delete.alder (deleted; --all writes it again)\n") || !strings.Contains(out, "wrote app/Admin/Like/Index.alder\n") || !strings.Contains(out, "edited app/Admin/Post/Index.alder\n") {
		t.Errorf("regen printed:\n%s", out)
	}
	after := appFiles(t, dir)
	if _, ok := after["app/Admin/Tag/Delete.alder"]; ok {
		t.Errorf("regen recreated the deleted page")
	}

	// every file the admin had keeps its text outside the regions
	for p, s := range before {
		if strings.HasPrefix(p, "app/Admin/") {
			if outside(t, after[p]) != outside(t, s) {
				t.Errorf("regen changed %s outside its regions:\n%s", p, generate.UnifiedDiff(p, outside(t, s), outside(t, after[p])))
			}
		}
	}
	// the new field, in the list, the show page and both forms; the new table, in the nav and the dashboard
	for p, wants := range map[string][]string{
		index:                         {"<h1>Every post</h1>", "<th>Words</th>", `{ name: "views", asc: Post.views.asc, desc: Post.views.desc },`, ">Views</a></th>", "<td>{Text.fromInt row.views}</td>", "test \"the list counts each post's words\""},
		"app/Admin/Post/Show.alder":   {"<dt>Views</dt>"},
		"app/Admin/Post/New.alder":    {"  views: Int,", `<al-input field="views" label="Views" value={0} />`, "views: p.form.views"},
		"app/Admin/Post/Edit.alder":   {"  views: Int,", `<al-input field="views" label="Views" value={model.row.views} />`, "views: p.form.views"},
		"app/Admin/Post/Delete.alder": {"likes: Int,", `<dd id="likes">`},
		"app/Admin/Layout.alder":      {"<a href={Admin.Like.Index {}}>Likes</a>"},
		"app/Admin/Index.alder":       {`{ key: "likes", title: "Likes"`},
	} {
		for _, w := range wants {
			if !strings.Contains(after[p], w) {
				t.Errorf("%s lacks %q after regen:\n%s", p, w, after[p])
			}
		}
	}
	// regen wrote only the admin's files
	for p := range changedFiles(mid, after) {
		if !strings.HasPrefix(p, "app/Admin/") {
			t.Errorf("regen wrote %s", p)
		}
	}
	if code, out, errs := alderIn(t, dir, "", "test"); code != 0 || !strings.HasSuffix(out, " passed, 0 failed\n") || !strings.Contains(out, "ok   Admin.Post.Index \"the list counts each post's words\"\n") {
		t.Fatalf("alder test after regen: exit %d\n%s%s", code, out, errs)
	}
	// idempotent: nothing to do, nothing written
	if code, out, _ := alderIn(t, dir, "", "regen", "admin"); code != 0 || out != "not recreated app/Admin/Tag/Delete.alder (deleted; --all writes it again)\nnothing to do: the admin is up to date\n" {
		t.Errorf("a second regen: exit %d\n%s", code, out)
	}
	if ch := changedFiles(after, appFiles(t, dir)); len(ch) != 0 {
		t.Errorf("a second regen wrote %v", slices.Sorted(maps.Keys(ch)))
	}
	// --dry-run, then --all writes the deleted page again
	if code, out, _ := alderIn(t, dir, "", "regen", "admin", "--all", "--dry-run"); code != 0 || !strings.HasPrefix(out, "would write app/Admin/Tag/Delete.alder\n") || !strings.HasSuffix(out, "dry run: nothing written\n") {
		t.Errorf("regen --all --dry-run: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "Admin", "Tag", "Delete.alder")); err == nil {
		t.Errorf("--dry-run wrote the page")
	}
	if code, out, errs := alderIn(t, dir, "", "regen", "admin", "--all"); code != 0 || out != "wrote app/Admin/Tag/Delete.alder\n" {
		t.Errorf("regen --all: exit %d\n%s%s", code, out, errs)
	}
}
