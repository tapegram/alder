package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// `alder new resource` (#194) on the fixture: Post is `@owner`-scoped by its author. Its files
// are goldens (genCases' "fixture-resource"); here: no unscoped write on an owned table's pages,
// the mutation gate, and the generator in `help --json`.

// resourceWrites are the unscoped calls a page of an owned table must not make outside its tests
// (or, for a count, without a `-- unscoped:` comment saying why).
var resourceWrites = []string{"Db.get ", "Db.update ", "Db.delete ", "Db.insert "}

// resourceCase is a resource of a fixture the tests below generate: the fixture, the table and
// `--read`, its module's directory, and the call its Edit and Delete pages load the row with;
// pre, the commands that make the table first.
type resourceCase struct {
	name, base, table, read, dir, load string
	pre                                [][]string
}

var resourceCases = []resourceCase{
	{"owner", "fixture", "Post", "owner", "Posts", "Db.getMine Post", nil},
	{"signed-in", "fixture", "Post", "signed-in", "Posts", "Db.getMine Post", nil},
	{"all", "fixture", "Post", "all", "Posts", "Db.getMine Post", nil},
	// `@partition org`: every read and write within the session's org
	{"partitioned", "fixture-silo", "Activity", "owner", "Activities", "Db.getScoped Activity", nil},
	// an owned table referring to a keyless lookup table (its select reads every row)
	{"lookup-ref", "fixture", "Article", "owner", "Articles", "Db.getMine Article", [][]string{
		{"new", "table", "Category", "name:Text.NonEmpty"},
		{"new", "table", "Article", "title:Text.NonEmpty", "owner:User.UserId@ref=User@owner", "category:Category.CategoryId@ref=Category"}}},
	// an owned table referring to a `@partition org` table (its routes require org too)
	{"partitioned-ref", "fixture-silo", "Note", "owner", "Notes", "Db.getMine Note", [][]string{
		{"new", "table", "Note", "text:Text.NonEmpty", "owner:User.UserId@ref=User@owner", "activity:Activity.ActivityId@ref=Activity"}}},
}

// resourceApp is a copy of c's fixture with c's table made (its pre commands) and its resource
// generated.
func resourceApp(t *testing.T, c resourceCase) string {
	t.Helper()
	dir := genApp(t, c.base)
	for _, pre := range c.pre {
		if code, out, errs := alderIn(t, dir, "", pre...); code != 0 {
			t.Fatalf("alder %s: exit %d\n%s%s", strings.Join(pre, " "), code, out, errs)
		}
	}
	if code, out, errs := alderIn(t, dir, "", "new", "resource", c.table, "--read", c.read); code != 0 {
		t.Fatalf("alder new resource %s --read %s: exit %d\n%s%s", c.table, c.read, code, out, errs)
	}
	return dir
}

// TestResource_NoUnscopedWrites (Review Focus 1): every line of a generated module of an owned
// or partitioned table that names an unscoped get, update, delete or insert, or reads the table
// itself with Db.from, is a test's line (in the region `tests`) or follows a `-- unscoped:`
// comment; under `--read signed-in` and `--read all` the one such get or write is Show's read
// (Index's is a `Db.from`), and the write pages still load with getMine (getScoped).
func TestResource_NoUnscopedWrites(t *testing.T) {
	t.Parallel()
	for _, c := range resourceCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := resourceApp(t, c)
			files, err := filepath.Glob(filepath.Join(dir, "app", c.dir, "*.alder"))
			if err != nil || len(files) != 5 {
				t.Fatalf("app/%s/: %v, %v", c.dir, files, err)
			}
			for _, f := range files {
				b, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				from := regexp.MustCompile(`\bDb\.from ` + c.table + `\b`)
				inTests, prev, stmt, excused := false, "", "", 0 // stmt: the line before the statement l is in
				for i, l := range strings.Split(string(b), "\n") {
					s := strings.TrimSpace(l)
					if strings.HasPrefix(s, "let ") || stmt == "" {
						stmt = prev
					}
					switch {
					case s == "-- @resource begin tests":
						inTests = true
					case inTests && s == "-- @resource end":
						inTests = false
					}
					// under --read owner, the table's own rows are read scoped but for a commented count
					if c.read == "owner" && from.MatchString(l) && !inTests && !strings.HasPrefix(stmt, "-- unscoped:") {
						t.Errorf("%s:%d: `Db.from %s` outside the tests with no `-- unscoped:` comment: %s", filepath.Base(f), i+1, c.table, s)
					}
					for _, w := range resourceWrites {
						if !strings.Contains(l, w) || inTests {
							continue
						}
						if !strings.HasPrefix(prev, "-- unscoped:") {
							t.Errorf("%s:%d: %q outside the tests with no `-- unscoped:` comment: %s", filepath.Base(f), i+1, strings.TrimSpace(w), s)
						}
						excused++
					}
					if s != "" {
						prev = s
					}
				}
				// the only unscoped get, update, delete or insert a page makes is Show's read
				// under --read signed-in|all
				want := 0
				if filepath.Base(f) == "Show.alder" && c.read != "owner" {
					want = 1
				}
				if excused != want {
					t.Errorf("%s: %d unscoped gets and writes outside the tests, want %d", filepath.Base(f), excused, want)
				}
				if base := filepath.Base(f); (base == "Edit.alder" || base == "Delete.alder") && !strings.Contains(string(b), c.load) {
					t.Errorf("%s does not load with %s", base, c.load)
				}
			}
		})
	}
}

// TestResource_Mutate: the generated tests kill the mutants of the resource's code, as the
// admin's do: Post's under each `--read`, Activity's (partitioned) and Tag's (keyless). The score
// is of the resource's modules' mutants (fixture-silo's own code has survivors of its own).
func TestResource_Mutate(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	mutationGate(t)
	if testing.Short() {
		t.Skip("runs every mutant of the fixture's resource")
	}
	cases := append(slices.Clone(resourceCases), resourceCase{"keyless", "fixture", "Tag", "signed-in", "Tags", "", nil})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := resourceApp(t, c)
			var out, errb bytes.Buffer
			code := runTestTo(withBuildDir(t, []string{dir, "--mutate", "--json"}), &out, &errb)
			if code != 0 {
				t.Fatalf("alder test --mutate --json: exit %d\n%s%s", code, out.String(), errb.String())
			}
			var killed, total int
			var survivors []string
			for _, l := range strings.Split(out.String(), "\n") {
				var ms struct {
					Mutants []struct {
						Module, Decl, Kind, Before, After string
						Line                              int
						Killed                            bool
					}
				}
				if !strings.HasPrefix(l, `{"mutants":`) || json.Unmarshal([]byte(l), &ms) != nil {
					continue
				}
				for _, m := range ms.Mutants {
					if !strings.HasPrefix(m.Module, c.dir+".") {
						continue
					}
					total++
					if m.Killed {
						killed++
					} else {
						survivors = append(survivors, fmt.Sprintf("%s %s:%d %s: %s → %s", m.Module, m.Decl, m.Line, m.Kind, m.Before, m.After))
					}
				}
			}
			if total == 0 {
				t.Fatalf("no mutant of app/%s/:\n%s", c.dir, out.String())
			}
			score := float64(killed) / float64(total)
			t.Logf("the %s resource (--read %s): %d mutants, %d killed (score %.2f)\n%s", c.table, c.read, total, killed, score, strings.Join(survivors, "\n"))
			if score < 0.95 {
				t.Errorf("score %.2f < 0.95; survivors:\n%s", score, strings.Join(survivors, "\n"))
			}
		})
	}
}

// TestResource_HelpJSON: `alder help --json` lists `new resource` with its usage line.
func TestResource_HelpJSON(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := writeHelpJSON(&out); err != nil {
		t.Fatal(err)
	}
	want := `"name":"resource"`
	if !strings.Contains(out.String(), want) {
		t.Fatalf("help --json has no %s", want)
	}
	usage := "usage: alder new resource <Table> [<field>:<Type>…] [--read owner|signed-in|all] [--requires user|<key>[,…]] [--module <M>] [--path /p] [--dir app] [--json] [--dry-run]"
	if !strings.Contains(out.String(), `"usage":"`+usage+`"`) {
		t.Errorf("help --json does not give `new resource` the usage %q", usage)
	}
}

// TestResource_DryRunJSON: --dry-run writes nothing and lists the five files in full; --json is
// the report's one object, with no command to run next (the resource is complete).
func TestResource_DryRunJSON(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "fixture")
	before := appFiles(t, dir)
	code, out, errs := alderIn(t, dir, "", "new", "resource", "Post", "--dry-run")
	if code != 0 || errs != "" || !strings.HasSuffix(out, "dry run: nothing written\n") {
		t.Fatalf("--dry-run: exit %d\n%s%s", code, out, errs)
	}
	for _, f := range []string{"Index", "Show", "New", "Edit", "Delete"} {
		p := "app/Posts/" + f + ".alder"
		if !strings.Contains(out, "would write "+p+"\n") || !strings.Contains(out, "--- /dev/null\n+++ b/"+p+"\n") {
			t.Errorf("--dry-run does not list %s in full:\n%s", p, out)
		}
	}
	if ch := changedFiles(before, appFiles(t, dir)); len(ch) != 0 {
		t.Errorf("--dry-run wrote %v", slices.Sorted(maps.Keys(ch)))
	}
	code, out, errs = alderIn(t, dir, "", "new", "resource", "Post", "--json")
	if code != 0 {
		t.Fatalf("--json: exit %d\n%s%s", code, out, errs)
	}
	var rep map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("--json: %v\n%s", err, out)
	}
	// the report is the generators' shared one: migration, error and next are left out when empty
	// (a resource makes no migration and has no command to run next)
	for _, k := range []string{"files", "diff", "diagnostics", "holes", "written"} {
		if _, ok := rep[k]; !ok {
			t.Errorf("--json has no %q: %s", k, out)
		}
	}
	for k := range rep {
		if !slices.Contains([]string{"files", "diff", "diagnostics", "holes", "migration", "written", "error", "next"}, k) {
			t.Errorf("--json has the unexpected key %q: %s", k, out)
		}
	}
	for _, k := range []string{"migration", "error", "next"} {
		if v, ok := rep[k]; ok && string(v) != `""` && string(v) != "[]" && string(v) != "null" {
			t.Errorf("%s = %s, want empty: %s", k, v, out)
		}
	}
}

// TestResource_Regen (Review Focus 3): the author's edits survive a regen; a region whose
// markers were deleted is not re-added; a deleted file is skipped unless --all; a file without
// the header is refused.
func TestResource_Regen(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "fixture")
	if code, out, errs := alderIn(t, dir, "", "new", "resource", "Post"); code != 0 {
		t.Fatalf("new resource: exit %d\n%s%s", code, out, errs)
	}
	if code, out, errs := alderIn(t, dir, "", "regen", "resource"); code != 0 || out != "nothing to do: the resource is up to date\n" {
		t.Fatalf("regen right after: exit %d\n%s%s", code, out, errs)
	}
	read := func(f string) string {
		b, err := os.ReadFile(filepath.Join(dir, "app", "Posts", f))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write := func(f, s string) {
		if err := os.WriteFile(filepath.Join(dir, "app", "Posts", f), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	show := read("Show.alder")
	write("Show.alder", show+"\n-- the author's own line\n")
	idx := read("Index.alder")
	i := strings.Index(idx, "-- @resource begin search")
	j := strings.Index(idx[i:], "-- @resource end")
	if i < 0 || j < 0 {
		t.Fatalf("Index.alder has no search region:\n%s", idx)
	}
	// the author takes the region over: its text stays (the list's code uses it), its markers go
	write("Index.alder", idx[:i]+strings.TrimPrefix(idx[i:i+j], strings.SplitN(idx[i:], "\n", 2)[0]+"\n")+idx[i+j+len("-- @resource end\n"):])
	if code, out, errs := alderIn(t, dir, "", "edit", "table", "Post", "add-field", `summary:Text=""`); code != 0 {
		t.Fatalf("edit table: exit %d\n%s%s", code, out, errs)
	}
	if code, out, errs := alderIn(t, dir, "", "regen", "resource", "Post"); code != 0 {
		t.Fatalf("regen: exit %d\n%s%s", code, out, errs)
	}
	for _, f := range []string{"New.alder", "Edit.alder", "Show.alder"} {
		if !strings.Contains(read(f), "summary") {
			t.Errorf("%s did not gain summary", f)
		}
	}
	if !strings.Contains(read("Show.alder"), "-- the author's own line\n") {
		t.Errorf("Show.alder lost the author's line")
	}
	if strings.Contains(read("Index.alder"), "@resource begin search") {
		t.Errorf("regen re-added the deleted search region")
	}
	if err := os.Remove(filepath.Join(dir, "app", "Posts", "Delete.alder")); err != nil {
		t.Fatal(err)
	}
	code, out, errs := alderIn(t, dir, "", "regen", "resource")
	if code != 0 || !strings.Contains(out, "not recreated app/Posts/Delete.alder (deleted; --all writes it again)\n") {
		t.Fatalf("regen with Delete.alder gone: exit %d\n%s%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "Posts", "Delete.alder")); err == nil {
		t.Errorf("regen recreated Delete.alder without --all")
	}
	if code, out, errs := alderIn(t, dir, "", "regen", "resource", "--all"); code != 0 {
		t.Fatalf("regen --all: exit %d\n%s%s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "Posts", "Delete.alder")); err != nil {
		t.Errorf("regen --all did not recreate Delete.alder")
	}
	// no header: refused, with the options to pass
	for _, f := range []string{"Index.alder", "Show.alder", "New.alder", "Edit.alder", "Delete.alder"} {
		src := read(f)
		write(f, src[strings.Index(src, "\n")+1:])
	}
	code, out, errs = alderIn(t, dir, "", "regen", "resource", "Post")
	want := "alder regen resource: `app/Posts/Index.alder` has no `alder new resource` header; pass the table and its options: `alder regen resource Post --module Posts --path /posts`\n"
	if code != 1 || errs != want {
		t.Errorf("headerless regen: exit %d\n%s%s\nwant %q", code, out, errs, want)
	}
}

// TestResource_RegenKeylessGainsOwner: a keyless table's resource switches to the scoped family
// when the table gains an `@owner` column (its header recorded no --requires).
func TestResource_RegenKeylessGainsOwner(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "fixture")
	if code, out, errs := alderIn(t, dir, "", "new", "resource", "Tag"); code != 0 {
		t.Fatalf("new resource: exit %d\n%s%s", code, out, errs)
	}
	// the author adds the owner by hand (a migration cannot fill a required reference), and
	// migrates over the keyless pages' errors by setting them aside: the app has no data
	tags := filepath.Join(dir, "app", "Tags")
	kept := appFiles(t, tags)
	if err := os.RemoveAll(tags); err != nil {
		t.Fatal(err)
	}
	tag := filepath.Join(dir, "app", "Table", "Tag.alder")
	b, err := os.ReadFile(tag)
	if err != nil {
		t.Fatal(err)
	}
	owned := strings.Replace(string(b), "  created: Time @created,\n", "  owner: User.UserId @ref User @owner,\n  created: Time @created,\n", 1)
	if owned == string(b) {
		t.Fatal("Tag.alder has no created column to put the owner before")
	}
	owned = strings.Replace(owned, `Db.insert Tag { name: Text.NonEmpty "news", post: post }`, `Db.insert Tag { name: Text.NonEmpty "news", post: post, owner: me }`, 1)
	if err := os.WriteFile(tag, []byte(owned), 0o644); err != nil {
		t.Fatal(err)
	}
	// the fixture holds no data: its migrations start over, with the owner in the first
	if err := os.RemoveAll(filepath.Join(dir, "migrations")); err != nil {
		t.Fatal(err)
	}
	var mout, merr bytes.Buffer
	if code := runMigrateTo([]string{dir}, &mout, &merr); code != 0 {
		t.Fatalf("alder migrate: exit %d\n%s%s", code, mout.String(), merr.String())
	}
	for name, src := range kept {
		if err := os.MkdirAll(tags, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tags, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if code, out, errs := alderIn(t, dir, "", "regen", "resource", "Tag"); code != 0 {
		t.Fatalf("regen: exit %d\n%s%s", code, out, errs)
	}
	b, err = os.ReadFile(filepath.Join(dir, "app", "Tags", "Edit.alder"))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); !strings.Contains(s, "requires user") || !strings.Contains(s, "Db.getMine Tag") {
		t.Errorf("Tags/Edit.alder is not in the scoped family:\n%s", s)
	}
	// reads too: the keyless header recorded no --read, so Index and Show are the owner's now
	for f, want := range map[string]string{"Index.alder": "Db.mine Tag", "Show.alder": "Db.getMine Tag"} {
		b, err := os.ReadFile(filepath.Join(dir, "app", "Tags", f))
		if err != nil {
			t.Fatal(err)
		}
		if s := string(b); !strings.Contains(s, want) || !strings.Contains(s, "requires user") || strings.Contains(s, "-- unscoped: `--read") {
			t.Errorf("Tags/%s does not read by its owner (%s):\n%s", f, want, s)
		}
	}
}

// TestResource_RegenSpacedRequires (Review Focus 4): a `--requires` given with a space is
// recorded as its keys joined by ",", which regen reads back: the resource is found, by name
// and by a bare `regen resource`, and is up to date.
func TestResource_RegenSpacedRequires(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "fixture-silo")
	if code, out, errs := alderIn(t, dir, "", "new", "resource", "Activity", "--requires", "user, org"); code != 0 {
		t.Fatalf("new resource: exit %d\n%s%s", code, out, errs)
	}
	b, err := os.ReadFile(filepath.Join(dir, "app", "Activities", "Index.alder"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "-- generated by `alder new resource Activity --module Activities --path /activities --requires user,org`;"; !strings.HasPrefix(string(b), want) {
		t.Errorf("header:\n%s\nwant prefix %q", strings.SplitN(string(b), "\n", 2)[0], want)
	}
	if !strings.Contains(string(b), "requires user, org\n") {
		t.Errorf("the Index route does not require `user, org`:\n%s", b)
	}
	for _, args := range [][]string{{"regen", "resource"}, {"regen", "resource", "Activity"}} {
		if code, out, errs := alderIn(t, dir, "", args...); code != 0 || out != "nothing to do: the resource is up to date\n" {
			t.Errorf("alder %s: exit %d\n%s%s", strings.Join(args, " "), code, out, errs)
		}
	}
}

// TestResource_RegenWarns: a regen prints the warnings `new resource` printed (a keyless table's).
func TestResource_RegenWarns(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "fixture")
	if code, out, errs := alderIn(t, dir, "", "new", "resource", "Tag"); code != 0 {
		t.Fatalf("new resource: exit %d\n%s%s", code, out, errs)
	}
	code, out, errs := alderIn(t, dir, "", "regen", "resource")
	if code != 0 || out != "nothing to do: the resource is up to date\n" || !strings.HasPrefix(errs, "alder regen resource: warning: `Tag` has no `@owner`") {
		t.Errorf("regen of a keyless resource: exit %d\n%s%s", code, out, errs)
	}
}

// TestRegenHelp: `regen resource --help` and `help regen resource` print the resource's usage,
// `regen admin --help` the admin's, and bare `regen` both.
func TestRegenHelp(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"regen", "resource", "--help"}, newRegenResourceUsage},
		{[]string{"help", "regen", "resource"}, newRegenResourceUsage},
		{[]string{"regen", "admin", "--help"}, regenAdminUsage},
		{[]string{"help", "regen"}, regenUsage},
	} {
		var out bytes.Buffer
		if !helpTo(c.args, &out) || out.String() != c.want+"\n" {
			t.Errorf("%v printed %q, want %q", c.args, out.String(), c.want)
		}
	}
}
