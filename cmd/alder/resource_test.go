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
// `--read`, its module's directory, and the call its Edit and Delete pages load the row with.
type resourceCase struct {
	name, base, table, read, dir, load string
}

var resourceCases = []resourceCase{
	{"owner", "fixture", "Post", "owner", "Posts", "Db.getMine Post"},
	{"signed-in", "fixture", "Post", "signed-in", "Posts", "Db.getMine Post"},
	{"all", "fixture", "Post", "all", "Posts", "Db.getMine Post"},
	// `@partition org`: every read and write within the session's org
	{"partitioned", "fixture-silo", "Activity", "owner", "Activities", "Db.getScoped Activity"},
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
			dir := genApp(t, c.base)
			if code, out, errs := alderIn(t, dir, "", "new", "resource", c.table, "--read", c.read); code != 0 {
				t.Fatalf("alder new resource %s --read %s: exit %d\n%s%s", c.table, c.read, code, out, errs)
			}
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
	cases := append(slices.Clone(resourceCases), resourceCase{"keyless", "fixture", "Tag", "signed-in", "Tags", ""})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := genApp(t, c.base)
			if code, out, errs := alderIn(t, dir, "", "new", "resource", c.table, "--read", c.read); code != 0 {
				t.Fatalf("alder new resource %s --read %s: exit %d\n%s%s", c.table, c.read, code, out, errs)
			}
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
