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
)

// `alder new resource` (#194) on the fixture: Post is `@owner`-scoped by its author. Its files
// are goldens (genCases' "fixture-resource"); here: no unscoped write on an owned table's pages,
// the mutation gate, and the generator in `help --json`.

// resourceWrites are the unscoped calls a page of an owned table must not make outside its tests
// (or, for a count, without a `-- unscoped:` comment saying why).
var resourceWrites = []string{"Db.get ", "Db.update ", "Db.delete ", "Db.insert "}

// TestResource_NoUnscopedWrites (Review Focus 1): every line of a generated module of an owned
// table that names an unscoped get, update, delete or insert is a test's line (in the region
// `tests`) or follows a `-- unscoped:` comment; under `--read signed-in` and `--read all` the
// one such line is Show's read (Index's is a `Db.from`), and the write pages still load with
// getMine.
func TestResource_NoUnscopedWrites(t *testing.T) {
	t.Parallel()
	for _, read := range []string{"owner", "signed-in", "all"} {
		t.Run(read, func(t *testing.T) {
			t.Parallel()
			dir := genApp(t, "fixture")
			if code, out, errs := alderIn(t, dir, "", "new", "resource", "Post", "--read", read); code != 0 {
				t.Fatalf("alder new resource Post --read %s: exit %d\n%s%s", read, code, out, errs)
			}
			files, err := filepath.Glob(filepath.Join(dir, "app", "Posts", "*.alder"))
			if err != nil || len(files) != 5 {
				t.Fatalf("app/Posts/: %v, %v", files, err)
			}
			for _, f := range files {
				b, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				inTests, prev, excused := false, "", 0
				for i, l := range strings.Split(string(b), "\n") {
					s := strings.TrimSpace(l)
					switch {
					case s == "-- @resource begin tests":
						inTests = true
					case inTests && s == "-- @resource end":
						inTests = false
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
					prev = s
				}
				// the only unscoped get, update, delete or insert a page makes is Show's read
				// under --read signed-in|all
				want := 0
				if filepath.Base(f) == "Show.alder" && read != "owner" {
					want = 1
				}
				if excused != want {
					t.Errorf("%s: %d unscoped gets and writes outside the tests, want %d", filepath.Base(f), excused, want)
				}
				if base := filepath.Base(f); (base == "Edit.alder" || base == "Delete.alder") && !strings.Contains(string(b), "Db.getMine Post") {
					t.Errorf("%s does not load with getMine", base)
				}
			}
		})
	}
}

// TestResource_Mutate: the generated tests kill the mutants of the resource's code (and of the
// fixture's own), as the admin's do, under each `--read`.
func TestResource_Mutate(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	mutationGate(t)
	if testing.Short() {
		t.Skip("runs every mutant of the fixture's resource")
	}
	for _, read := range []string{"owner", "signed-in", "all"} {
		t.Run(read, func(t *testing.T) {
			t.Parallel()
			dir := genApp(t, "fixture")
			if code, out, errs := alderIn(t, dir, "", "new", "resource", "Post", "--read", read); code != 0 {
				t.Fatalf("alder new resource Post --read %s: exit %d\n%s%s", read, code, out, errs)
			}
			var out, errb bytes.Buffer
			code := runTestTo(withBuildDir(t, []string{dir, "--mutate"}), &out, &errb)
			m := scoreRe.FindStringSubmatch(out.String())
			if code != 0 || m == nil {
				t.Fatalf("alder test --mutate: exit %d\n%s%s", code, out.String(), errb.String())
			}
			t.Logf("the fixture with its Post resource, --read %s: %s", read, m[0])
			_, survivors, _ := strings.Cut(out.String(), "survived:\n")
			if survivors != "" {
				t.Logf("survivors:\n%s", survivors)
			}
			if m[4] < "0.95" {
				t.Errorf("score %s < 0.95; survivors:\n%s", m[4], survivors)
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
