package main

import (
	"bytes"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var updateGenerate = flag.Bool("update", false, "rewrite the generators' golden files (testdata/generate/want)")

// The generators (book chapter 15) are tested on scratch copies of two apps: the scaffold that
// `alder new` writes, and testdata/generate/fixture, whose tables have refs with each delete
// rule, unions, a Bool, an `@owner`, and a session with a user and a role. Each command's
// output and every file it writes are compared with testdata/generate/want/<case>/ (stdout, and
// files/ mirroring the app; `go test ./cmd/alder -run TestGenerate -update` rewrites them), and
// after it the app checks and, its typed holes filled with `alder fill`, `alder test` passes.

// genApp makes a scratch copy of base ("scaffold" or "fixture") and returns its directory.
func genApp(t *testing.T, base string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "app")
	switch base {
	case "scaffold":
		var out, errb bytes.Buffer
		if code := runNewTo([]string{dir}, &out, &errb); code != 0 {
			t.Fatalf("alder new: exit %d: %s", code, errb.String())
		}
	case "fixture":
		copyDir(t, "../../testdata/generate/fixture", dir)
	default:
		t.Fatalf("no base app %q", base)
	}
	return dir
}

// alderIn runs the alder command args (new, edit, fill, check or test) on the app in dir, with
// stdin as standard input.
func alderIn(t *testing.T, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	var code int
	withDir := append(slices.Clone(args[1:]), "--dir", dir)
	switch args[0] {
	case "new":
		code = runNewTo(withDir, &out, &errb)
	case "check":
		code = runCheckTo([]string{dir}, &out, &errb)
	case "test":
		code = runTestTo([]string{dir}, &out, &errb)
	default:
		t.Fatalf("no command %s", args[0])
	}
	return code, out.String(), errb.String()
}

// appFiles is every file of the app in dir but build/, by slash-separated path.
func appFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() && rel == "build" {
			return filepath.SkipDir
		}
		if !d.IsDir() && !strings.HasSuffix(rel, ".db") {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			files[rel] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// changedFiles are the files of after that are new or different in before.
func changedFiles(before, after map[string]string) map[string]string {
	out := map[string]string{}
	for p, s := range after {
		if b, ok := before[p]; !ok || b != s {
			out[p] = s
		}
	}
	return out
}

// golden compares the command's stdout and the files it wrote with testdata/generate/want/name
// (or, with -update, rewrites them).
func golden(t *testing.T, name, stdout string, files map[string]string) {
	t.Helper()
	dir := filepath.Join("../../testdata/generate/want", name)
	if *updateGenerate {
		os.RemoveAll(dir)
		write := func(p, s string) {
			p = filepath.Join(dir, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write("stdout", stdout)
		for p, s := range files {
			write("files/"+p, s)
		}
		return
	}
	want, err := os.ReadFile(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatalf("%v (run with -update to write the golden files)", err)
	}
	if stdout != string(want) {
		t.Errorf("stdout:\n%s\nwant (%s/stdout):\n%s", stdout, dir, want)
	}
	wantFiles := appFiles(t, filepath.Join(dir, "files"))
	if got, w := slices.Sorted(maps.Keys(files)), slices.Sorted(maps.Keys(wantFiles)); !slices.Equal(got, w) {
		t.Errorf("files written: %v, want %v", got, w)
	}
	for p, s := range files {
		if w, ok := wantFiles[p]; ok && s != w {
			t.Errorf("%s:\n%s\nwant (%s/files/%s):\n%s", p, s, dir, p, w)
		}
	}
}

// genCase is one generator run: on a copy of base, the command args; its exit code; then,
// unless the command breaks code on purpose (breaks: `alder edit table`'s to-do list), `alder
// check` passes; and when tests is not 0, the holes are filled (fills: address and expression,
// in order) and `alder test` passes with that many tests.
type genCase struct {
	name   string
	base   string
	args   []string
	code   int
	breaks bool
	fills  [][2]string
	tests  int
}

var genCases = []genCase{
	{name: "page", base: "scaffold", args: []string{"new", "page", "About", "/about"}, tests: 3},
	{name: "page-list", base: "scaffold", args: []string{"new", "page", "Entries.Index", "/entries", "--list", "Entry"}, tests: 3},
	{name: "page-show", base: "scaffold", args: []string{"new", "page", "Entries.Show", "/entries", "--show", "Entry"}, tests: 4},
	{name: "page-from", base: "scaffold", args: []string{"new", "page", "Dash", "/dash", "--from", "Entry"}, tests: 3},
	{name: "table", base: "scaffold", args: []string{"new", "table", "Tag", "name:Text.NonEmpty@unique", "entry:Entry.EntryId@ref=Entry@cascade", "note:Maybe Text", "pinned:Bool=False"}, tests: 2},
	{name: "form-insert", base: "scaffold", args: []string{"new", "form", "Home", "Quick", "name:Text.NonEmpty", "message:Text.NonEmpty"}, tests: 4},
	{name: "form-hole", base: "scaffold", args: []string{"new", "form", "Home", "Contact", "email:Text.Email", "topic:Text"}},
	{name: "msg", base: "scaffold", args: []string{"new", "msg", "Home", "Clear"}},
	{name: "capability", base: "scaffold", args: []string{"new", "capability", "Weather", "current", "{ city: Text } -> Query (Maybe Float)"}},

	{name: "fixture-page-list-mine", base: "fixture", args: []string{"new", "page", "Posts.Index", "/posts", "--list", "Post", "--requires", "user"}, tests: 6},
	{name: "fixture-page-show-role", base: "fixture", args: []string{"new", "page", "Users.Show", "/users", "--show", "User", "--role", "Admin"}, tests: 7},
	{name: "fixture-page-from", base: "fixture", args: []string{"new", "page", "Dash", "/dash", "--from", "Post", "Tag", "--requires", "user"}, tests: 6},
	{name: "fixture-table", base: "fixture", args: []string{"new", "table", "Like", "post:Post.PostId@ref=Post@cascade", "owner:User.UserId@ref=User@owner"}, tests: 5},
	{name: "fixture-form-mine", base: "fixture", args: []string{"new", "form", "Home", "Quick", "title:Text.NonEmpty", "body:Text"}, tests: 7},
	{name: "fixture-form-ref", base: "fixture", args: []string{"new", "form", "Home", "Tagging", "name:Text.NonEmpty", "post:Post.PostId"}, tests: 7},
	{name: "fixture-msg-params", base: "fixture", args: []string{"new", "msg", "Home", "Feature", "id:Post.PostId"}},
	{name: "fixture-capability", base: "fixture", args: []string{"new", "capability", "Mail", "send", "{ to: Text.Email, subject: Text } -> Task Unit"}},
}

// TestGenerate runs every case of genCases.
func TestGenerate(t *testing.T) {
	needGoTool(t)
	for _, c := range genCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := genApp(t, c.base)
			before := appFiles(t, dir)
			code, out, errs := alderIn(t, dir, "", c.args...)
			if code != c.code {
				t.Fatalf("alder %s: exit %d, want %d\nstdout:\n%s\nstderr:\n%s", strings.Join(c.args, " "), code, c.code, out, errs)
			}
			golden(t, c.name, out, changedFiles(before, appFiles(t, dir)))
			if c.breaks {
				return
			}
			if code, out, errs := alderIn(t, dir, "", "check"); code != 0 || out != "" {
				t.Fatalf("alder check after it: exit %d\n%s%s", code, out, errs)
			}
			if c.tests == 0 {
				return
			}
			for _, f := range c.fills {
				if code, out, errs := alderIn(t, dir, f[1], "fill", f[0], "-"); code != 0 {
					t.Fatalf("alder fill %s: exit %d\n%s%s", f[0], code, out, errs)
				}
			}
			code, out, errs = alderIn(t, dir, "", "test")
			if code != 0 || !strings.HasSuffix(out, fmt.Sprintf("%d passed, 0 failed\n", c.tests)) {
				t.Fatalf("alder test after it: exit %d\n%s%s", code, out, errs)
			}
		})
	}
}

// TestGenerate_DoesNotCheck (Review Focus 1): a command whose result does not check writes
// nothing, prints the checker's diagnostics and exits 1; after the mistake is corrected the
// same command works.
func TestGenerate_DoesNotCheck(t *testing.T) {
	dir := genApp(t, "fixture")
	before := appFiles(t, dir)
	code, out, errs := alderIn(t, dir, "", "new", "form", "Home", "Bad", "title:Int", "--into", "Post")
	if code != 1 || errs != "alder new form: the result does not check (the errors above): nothing was written\n" ||
		!strings.Contains(out, "error[E0003]: expected Text.NonEmpty, got Int") {
		t.Errorf("a form that does not fit its table: exit %d\nstdout:\n%s\nstderr: %s", code, out, errs)
	}
	if len(changedFiles(before, appFiles(t, dir))) != 0 {
		t.Errorf("a refused form wrote files")
	}
	if code, _, errs := alderIn(t, dir, "", "new", "form", "Home", "Bad", "title:Text.NonEmpty", "body:Text", "--into", "Post"); code != 0 {
		t.Errorf("the corrected form: exit %d: %s", code, errs)
	}
}

// TestGenerate_Refusals: each refusal is one sentence on standard error, exit 1, nothing
// written.
func TestGenerate_Refusals(t *testing.T) {
	for _, c := range []struct {
		base  string
		args  []string
		stdin string
		want  string
	}{
		{"scaffold", []string{"new", "page", "Home", "/home"}, "", "alder new page: module `Home` exists (`app/Home.alder`): a generator never overwrites a file; choose another module name"},
		{"scaffold", []string{"new", "page", "about", "/about"}, "", "alder new page: `about` is not a module name: write capitalised names joined by dots, such as `Posts.Index`"},
		{"scaffold", []string{"new", "page", "About", "about"}, "", "alder new page: `about` is not a path: a page's path starts with `/`, such as `/posts`"},
		{"scaffold", []string{"new", "page", "About", "/about/{id}"}, "", "alder new page: `/about/{id}` has a parameter: give the path without it (`--show T` adds `/{id}`), or write the page's `page` line yourself"},
		{"scaffold", []string{"new", "page", "About", "/about", "--list", "Post"}, "", "alder new page: `Post` is not a table of this app (its tables: Entry)"},
		{"scaffold", []string{"new", "page", "About", "/about", "--list", "Entry", "--show", "Entry"}, "", "alder new page: choose one of --list, --show and --from: a page generated with --list shows one table's rows, --show one row, --from several tables"},
		{"scaffold", []string{"new", "page", "About", "/about", "--requires", "user"}, "", "alder new page: `--requires user` needs a session with a `user` field (`session { user: Maybe User.UserId }` in `App`): this app's session has none"},
		{"fixture", []string{"new", "page", "About", "/about", "--requires", "admin"}, "", "alder new page: `--requires admin`: a generated page can require `user`, the session's signed-in user"},
		{"fixture", []string{"new", "page", "About", "/about", "--role", "Editor"}, "", "alder new page: `--role Editor`: `Editor` is not a constructor of the session's `role` type `User.Role` (Reader, Admin)"},
		{"scaffold", []string{"new", "table", "Entry", "x:Int"}, "", "alder new table: module `Entry` exists (`app/Entry.alder`): a generator never overwrites a file; choose another table name"},
		{"scaffold", []string{"new", "table", "Tag", "id:Int"}, "", "alder new table: field `id`: the generator writes the id, `id: TagId @id`; leave it out"},
		{"scaffold", []string{"new", "table", "Tag", "name"}, "", "alder new table: `name` is not a field: write `name:Type`, such as `title:Text.NonEmpty`"},
		{"scaffold", []string{"new", "form", "Home", "Sign", "name:Text"}, "", "alder new form: `Home` already declares `Sign`: choose another form name"},
		{"scaffold", []string{"new", "form", "Entry", "Sign", "name:Text"}, "", "alder new form: `Entry` is not a page (it has no `page` declaration): `alder new page Entry /path` makes one"},
		{"scaffold", []string{"new", "form", "Home", "Contact", "name:Text", "--into", "Post"}, "", "alder new form: `--into Post`: `Post` is not a table of this app (its tables: Entry)"},
		{"scaffold", []string{"new", "msg", "Home", "Submit"}, "", "alder new msg: `Home` already declares `Submit`: choose another message name"},
		{"scaffold", []string{"new", "msg", "Nowhere", "Go"}, "", "alder new msg: there is no page `Nowhere`: `alder new page Nowhere /path` makes one"},
		{"scaffold", []string{"new", "capability", "Weather", "now", "Query Float ->"}, "", "alder new capability: `Query Float ->` is not a type: "},
	} {
		dir := genApp(t, c.base)
		before := appFiles(t, dir)
		code, out, errs := alderIn(t, dir, c.stdin, c.args...)
		if code != 1 || !strings.HasPrefix(errs, c.want) || strings.Count(errs, "\n") != 1 {
			t.Errorf("alder %s: exit %d\nstderr: %s\nwant:   %s\nstdout: %s", strings.Join(c.args, " "), code, errs, c.want, out)
		}
		if ch := changedFiles(before, appFiles(t, dir)); len(ch) != 0 {
			t.Errorf("alder %s wrote %v", strings.Join(c.args, " "), slices.Sorted(maps.Keys(ch)))
		}
	}

	// the generators' names are not app names
	base := t.TempDir()
	if code, _, errs := runNew(t, filepath.Join(base, "page")); code != 1 || errs != "alder new: `page` names a generator (`alder new page …`), so it cannot name an app: choose another name\n" {
		t.Errorf("an app named page: exit %d, %q", code, errs)
	}
	if code, _, errs := runNew(t, "page"); code != 2 || !strings.HasPrefix(errs, "usage: alder new page <Module> <path>") {
		t.Errorf("alder new page alone: exit %d, %q", code, errs)
	}
}

// TestGenerate_DryRun: --dry-run prints what the command prints and writes nothing; the
// output is the same on every run.
func TestGenerate_DryRun(t *testing.T) {
	args := []string{"new", "table", "Tag", "name:Text.NonEmpty", "entry:Entry.EntryId@ref=Entry"}
	dir := genApp(t, "scaffold")
	before := appFiles(t, dir)
	code, dry, _ := alderIn(t, dir, "", append(slices.Clone(args), "--dry-run")...)
	if code != 0 || !strings.HasSuffix(dry, "dry run: nothing written\n") {
		t.Errorf("--dry-run: exit %d\n%s", code, dry)
	}
	if ch := changedFiles(before, appFiles(t, dir)); len(ch) != 0 {
		t.Errorf("--dry-run wrote %v", slices.Sorted(maps.Keys(ch)))
	}
	code, out, _ := alderIn(t, dir, "", args...)
	if code != 0 || out+"dry run: nothing written\n" != dry {
		t.Errorf("the run after the dry run printed:\n%s\nthe dry run:\n%s", out, dry)
	}
	other := genApp(t, "scaffold")
	if _, again, _ := alderIn(t, other, "", args...); again != out {
		t.Errorf("a second app printed:\n%s\nthe first:\n%s", again, out)
	}
	if a, b := appFiles(t, dir), appFiles(t, other); a["app/Tag.alder"] != b["app/Tag.alder"] || a["migrations/0002_create_tag.sql"] != b["migrations/0002_create_tag.sql"] {
		t.Errorf("two runs wrote different files")
	}
}
