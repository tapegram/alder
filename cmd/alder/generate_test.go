package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"alder/internal/buildkit"
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

// alderIn runs the alder command args (new, edit, fill, regen, remove, check or test) on the app in dir, with
// stdin as standard input.
func alderIn(t *testing.T, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	var code int
	withDir := append(slices.Clone(args[1:]), "--dir", dir)
	switch args[0] {
	case "new":
		code = runNewTo(withDir, &out, &errb)
	case "edit":
		code = runEditTo(withDir, &out, &errb)
	case "fill":
		code = runFillTo(withDir, strings.NewReader(stdin), &out, &errb)
	case "regen":
		code = runRegenTo(withDir, &out, &errb)
	case "remove":
		code = runRemoveTo(withDir, &out, &errb)
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
	wantFiles := map[string]string{} // none: a refusal, or a command that only removes
	if _, err := os.Stat(filepath.Join(dir, "files")); err == nil {
		wantFiles = appFiles(t, filepath.Join(dir, "files"))
	}
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
// check` passes; and with test, the holes are filled (fills: address and expression, in order)
// and every test passes.
type genCase struct {
	name   string
	base   string
	args   []string
	code   int
	breaks bool
	fills  [][2]string
	test   bool
}

var genCases = []genCase{
	{name: "page", base: "scaffold", args: []string{"new", "page", "About", "/about"}, test: true},
	{name: "page-list", base: "scaffold", args: []string{"new", "page", "Entries.Index", "/entries", "--list", "Entry"}, test: true},
	{name: "page-show", base: "scaffold", args: []string{"new", "page", "Entries.Show", "/entries", "--show", "Entry"}, test: true},
	{name: "page-from", base: "scaffold", args: []string{"new", "page", "Dash", "/dash", "--from", "Entry"}, test: true},
	{name: "table", base: "scaffold", args: []string{"new", "table", "Tag", "name:Text.NonEmpty@unique", "entry:Entry.EntryId@ref=Entry@cascade", "note:Maybe Text", "pinned:Bool=False"}, test: true},
	{name: "form-insert", base: "scaffold", args: []string{"new", "form", "Home", "Quick", "name:Text.NonEmpty", "message:Text.NonEmpty"}, test: true},
	{name: "form-hole", base: "scaffold", args: []string{"new", "form", "Home", "Contact", "email:Text.Email", "topic:Text"}, test: true,
		fills: [][2]string{{"Home.update.SubmitContact", "Task.succeed Rerender"}, {`Home."submitting Contact"`, "Rerendered"}}},
	{name: "msg", base: "scaffold", args: []string{"new", "msg", "Home", "Clear"}, test: true,
		fills: [][2]string{{"Home.update.Clear", "{\n  let _ <- Log.info \"cleared\"\n  Task.succeed Rerender\n}"}, {`Home."pressing Clear"`, "{ answer: r.answer, page: after.answer, entries: Test.count \"#entries li\" after.doc } == { answer: Rerendered, page: Shown, entries: 0 }"}}},
	{name: "capability", base: "scaffold", args: []string{"new", "capability", "Weather", "current", "{ city: Text } -> Query (Maybe Float)"}, test: true,
		fills: [][2]string{{"Weather.Weather.current", "Query.succeed (Just 21.5)"}}},
	{name: "edit-add", base: "scaffold", args: []string{"edit", "table", "Entry", "add-field", "note:Maybe Text"}, breaks: true},

	{name: "fixture-page-list-mine", base: "fixture", args: []string{"new", "page", "Posts.Index", "/posts", "--list", "Post", "--requires", "user"}, test: true},
	{name: "fixture-page-show-role", base: "fixture", args: []string{"new", "page", "Users.Show", "/users", "--show", "User", "--role", "Admin"}, test: true},
	{name: "fixture-page-from", base: "fixture", args: []string{"new", "page", "Dash", "/dash", "--from", "Post", "Tag", "--requires", "user"}, test: true},
	{name: "fixture-table", base: "fixture", args: []string{"new", "table", "Like", "post:Post.PostId@ref=Post@cascade", "owner:User.UserId@ref=User@owner"}, test: true},
	{name: "fixture-form-mine", base: "fixture", args: []string{"new", "form", "Home", "Quick", "title:Text.NonEmpty", "body:Text"}, test: true},
	{name: "fixture-form-ref", base: "fixture", args: []string{"new", "form", "Home", "Tagging", "name:Text.NonEmpty", "post:Post.PostId"}, test: true},
	{name: "fixture-msg-params", base: "fixture", args: []string{"new", "msg", "Home", "Feature", "id:Post.PostId"}},
	{name: "fixture-capability", base: "fixture", args: []string{"new", "capability", "Mail", "send", "{ to: Text.Email, subject: Text } -> Task Unit"}, test: true,
		fills: [][2]string{{"Mail.Mail.send", "Task.succeed ()"}}},
	{name: "fixture-edit-add", base: "fixture", args: []string{"edit", "table", "Post", "add-field", "summary:Maybe Text"}, breaks: true},
	{name: "fixture-edit-rename", base: "fixture", args: []string{"edit", "table", "Post", "rename-field", "body", "text"}, breaks: true},
	{name: "fixture-edit-drop", base: "fixture", args: []string{"edit", "table", "Tag", "drop-field", "created"}, test: true},
	{name: "fixture-admin", base: "fixture", args: []string{"new", "admin"}, test: true},

	// M13: the commands of the agent's loop
	{name: "auth", base: "scaffold", args: []string{"new", "auth"}, test: true},
	{name: "type-union", base: "scaffold", args: []string{"new", "type", "Entry", "Mood = Happy | Sad"}, test: true},
	{name: "type-brand", base: "scaffold", args: []string{"new", "type", "Entry", `Code = brand Text where Text.matches "^[a-z]+$"`}, test: true},
	{name: "type-record", base: "scaffold", args: []string{"new", "type", "Home", "Row = { entry: Entry, mood: Text }"}, test: true},
	{name: "type-module", base: "scaffold", args: []string{"new", "type", "User", "Role = Member | Admin"}, test: true},
	{name: "decl", base: "scaffold", args: []string{"new", "decl", "Home", "perPage", "Int"}, test: true,
		fills: [][2]string{{"Home.perPage", "20"}}},
	{name: "test", base: "scaffold", args: []string{"new", "test", "Home", "the guestbook starts empty"}, test: true,
		fills: [][2]string{{`Home."the guestbook starts empty"`, "{\n  let n <- Db.from Entry |> Db.count\n  Task.succeed (n == 0)\n}"}}},
	{name: "page-blank", base: "scaffold", args: []string{"new", "page", "Notes", "/notes", "--blank"}, test: true,
		fills: [][2]string{{"Notes.load", "Query.succeed { }"}, {"Notes.view", "<al-page><h1>Notes</h1></al-page>"}}},
	{name: "fixture-page-blank-show", base: "fixture", args: []string{"new", "page", "Posts.Edit", "/posts/edit", "--show", "Post", "--blank", "--requires", "user"}, test: true,
		fills: [][2]string{{"Posts.Edit.load", "Query.succeed { }"}, {"Posts.Edit.view", "<al-page><h1>Edit</h1></al-page>"}}},
	{name: "msg-in-for", base: "scaffold", args: []string{"new", "msg", "Home", "Hide", "id:Entry.EntryId", "--in-for", "e", "--when", "List.length model.entries > 0"}, test: true,
		fills: [][2]string{{"Home.update.Hide", "Task.succeed Rerender"}, {`Home."pressing Hide" --hole ?hideId`, "entry1"}, {`Home."pressing Hide"`, "{ answer: r.answer, found: Maybe.map (\\x -> x.id) after } == { answer: Rerendered, found: Just entry1 }"}}},
	{name: "edit-page-param", base: "scaffold", args: []string{"edit", "page", "Home", "add-param", `q:Text=""`}, test: true},

	// M13 final: the commands E11 lacked
	{name: "table-no-timestamps", base: "scaffold", args: []string{"new", "table", "Tag", "name:Text.NonEmpty", "created:Time@created", "--no-timestamps"}, test: true},
	{name: "page-param", base: "scaffold", args: []string{"new", "page", "Tags.Show", "/tags/{name}", "--param", "name:Text"}, test: true,
		fills: [][2]string{{"Tags.Show.load", "Query.succeed { }"}}},
	{name: "fixture-page-show-by", base: "fixture", args: []string{"new", "page", "Tags.Show", "/tags", "--show", "Tag", "--by", "name"}, test: true},
	{name: "form-msg", base: "scaffold", args: []string{"new", "form", "Home", "Quick", "name:Text.NonEmpty", "message:Text.NonEmpty", "--msg", "Sign2"}, test: true},
	// E11 fixes: the four commands
	{name: "app-msg", base: "scaffold", args: []string{"new", "msg", "App", "ToggleTheme", "dark:Bool"}, test: true,
		fills: [][2]string{{"App.update.ToggleTheme", "Task.succeed Rerender"}, {`App."pressing ToggleTheme" --hole ?toggleThemeDark`, "True"}, {`App."pressing ToggleTheme"`, "{ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: [] }"}}},
	{name: "fixture-app-msg-hole", base: "fixture", args: []string{"new", "msg", "App", "Ping"}},
	{name: "edit-decl-add-prop", base: "scaffold", args: []string{"edit", "decl", "Layout.Page", "add-prop", "dark:Bool=False"}, test: true},
	{name: "msg-id", base: "scaffold", args: []string{"new", "msg", "Home", "Clear", "--id", "clear"}, test: true,
		fills: [][2]string{{"Home.update.Clear", "Task.succeed Rerender"}, {`Home."pressing Clear"`, "{ answer: r.answer, button: Test.has \"#clear\" after.doc } == { answer: Rerendered, button: True }"}}},
	{name: "fixture-edit-session", base: "fixture", args: []string{"edit", "session", "add-field", `theme:Text="light"`}, test: true},
	{name: "remove-page", base: "scaffold", args: []string{"remove", "page", "Home"}},
	{name: "fixture-remove-table", base: "fixture", args: []string{"remove", "table", "Comment"}, test: true},
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
			if !c.test {
				return
			}
			for _, f := range c.fills {
				args := []string{"fill", f[0], "-"}
				if addr, hole, ok := strings.Cut(f[0], " --hole "); ok { // `Module."test" --hole ?x`
					args = []string{"fill", addr, "-", "--hole", hole}
				}
				if code, out, errs := alderIn(t, dir, f[1], args...); code != 0 {
					t.Fatalf("alder fill %s: exit %d\n%s%s", f[0], code, out, errs)
				}
			}
			code, out, errs = alderIn(t, dir, "", "test")
			if code != 0 || !strings.HasSuffix(out, " passed, 0 failed\n") {
				t.Fatalf("alder test after it: exit %d\n%s%s", code, out, errs)
			}
		})
	}
}

// TestGenerate_EditTableBreaks (Review Focus 2): a field without a default added to a table
// that is inserted in several places is written, with its migration, and every insert that
// now lacks it is printed with its file and line; --strict refuses the same edit and writes
// nothing.
func TestGenerate_EditTableBreaks(t *testing.T) {
	dir := genApp(t, "fixture")
	before := appFiles(t, dir)
	code, out, errs := alderIn(t, dir, "", "edit", "table", "Post", "add-field", "summary:Maybe Text", "--strict")
	if code != 1 || errs != "alder edit table: the result does not check (the errors above): nothing was written\n" {
		t.Errorf("--strict: exit %d, stderr %q", code, errs)
	}
	if !strings.Contains(out, "app/Home.alder:") || len(changedFiles(before, appFiles(t, dir))) != 0 {
		t.Errorf("--strict wrote files or printed no diagnostics:\n%s", out)
	}
	code, out, errs = alderIn(t, dir, "", "edit", "table", "Post", "add-field", "summary:Maybe Text")
	if code != 0 || errs != "" {
		t.Fatalf("exit %d, stderr %q", code, errs)
	}
	written := changedFiles(before, appFiles(t, dir))
	if got := slices.Sorted(maps.Keys(written)); !slices.Equal(got, []string{"app/Post.alder", "migrations/0002_add_summary.sql", "migrations/schema.json"}) {
		t.Errorf("written: %v", got)
	}
	// the inserts of Post: four in Home (its update's Db.insertMine and three in a test), two in
	// Post's tests, two in Comment's, one in Tag's
	for file, n := range map[string]int{"app/Home.alder": 4, "app/Post.alder": 2, "app/Comment.alder": 2, "app/Tag.alder": 1} {
		re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(file) + `:\d+:\d+: error\[E0003\]: record .* is missing field\(s\): summary$`)
		if got := len(re.FindAllString(out, -1)); got != n {
			t.Errorf("%d inserts printed in %s, want %d:\n%s", got, file, n, out)
		}
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

	// fill: an expression of the wrong type
	dir = genApp(t, "scaffold")
	if code, _, errs := alderIn(t, dir, "", "new", "msg", "Home", "Clear"); code != 0 {
		t.Fatalf("alder new msg: %s", errs)
	}
	before = appFiles(t, dir)
	code, out, errs = alderIn(t, dir, "42\n", "fill", `Home."pressing Clear"`, "-")
	if code != 1 || errs != "alder fill: the result does not check (the errors above): nothing was written\n" ||
		!strings.Contains(out, "expected Bool, got Int") || strings.Contains(out, "edited app/") || strings.Contains(out, "+++ b/") {
		t.Errorf("fill with a wrong type: exit %d\nstdout:\n%s\nstderr: %s", code, out, errs)
	}
	if len(changedFiles(before, appFiles(t, dir))) != 0 {
		t.Errorf("a refused fill wrote files")
	}
	code, out, _ = alderIn(t, dir, "r.answer == Rerendered\n", "fill", `Home."pressing Clear"`, "-")
	if code != 0 || !strings.Contains(out, "+  Task.succeed (r.answer == Rerendered)\n") {
		t.Errorf("fill: exit %d\n%s", code, out)
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
		{"scaffold", []string{"new", "page", "About", "/about/{id}"}, "", "alder new page: `/about/{id}` has a parameter: name its type with `--param <name>:<Type>` (`/posts/{slug} --param slug:Post.Slug`), or give the path without it (`--show T` adds `/{id}`)"},
		{"scaffold", []string{"new", "page", "X", "//evil.com"}, "", "alder new page: `//evil.com` is not a page path: it has an empty segment; a path is `/` followed by segments of letters, digits, `.`, `_`, `~`, `-` or `{param}`"},
		{"scaffold", []string{"new", "page", "X", "/a b"}, "", "alder new page: `/a b` is not a page path: it has a space; a path is `/` followed by segments of letters, digits, `.`, `_`, `~`, `-` or `{param}`"},
		{"scaffold", []string{"new", "page", "X", "/a?b=1"}, "", "alder new page: `/a?b=1` is not a page path: it has a query (`?`)"},
		{"scaffold", []string{"new", "page", "X", `/a\b`}, "", "alder new page: `/a\\b` is not a page path: it has the character `\\`"},
		{"scaffold", []string{"new", "page", "X", `/a"b`}, "", "alder new page: `/a\"b` is not a page path: it has the character `\"`"},
		{"scaffold", []string{"new", "page", "About", "/about", "--list", "Post"}, "", "alder new page: `Post` is not a table of this app (its tables: Entry)"},
		{"scaffold", []string{"new", "page", "About", "/about", "--list", "Entry", "--show", "Entry"}, "", "alder new page: choose one of --list, --show and --from: a page generated with --list shows one table's rows, --show one row, --from several tables"},
		{"scaffold", []string{"new", "page", "About", "/about", "--requires", "user"}, "", "alder new page: `--requires user` needs a session with a `user` field (`session { user: Maybe User.UserId }` in `App`): this app's session has none"},
		{"fixture", []string{"new", "page", "About", "/about", "--requires", "admin"}, "", "alder new page: `--requires admin`: a generated page can require `user`, the session's signed-in user"},
		{"fixture", []string{"new", "page", "About", "/about", "--role", "Editor"}, "", "alder new page: `--role Editor`: `Editor` is not a constructor of the session's `role` type `User.Role` (Reader, Admin)"},
		{"scaffold", []string{"new", "table", "Entry", "x:Int"}, "", "alder new table: module `Entry` exists (`app/Entry.alder`) and declares a table: a generator never overwrites a file; choose another table name"},
		{"scaffold", []string{"new", "table", "Tag", "id:Int"}, "", "alder new table: field `id`: the generator writes the id, `id: TagId @id`; leave it out"},
		{"scaffold", []string{"new", "table", "Tag", "name"}, "", "alder new table: `name` is not a field: write `name:Type`, such as `title:Text.NonEmpty`"},
		{"scaffold", []string{"new", "form", "Home", "Sign", "name:Text"}, "", "alder new form: `Home` already declares `Sign`: choose another name"},
		{"scaffold", []string{"new", "form", "Home", "Entry", "name:Text"}, "", "alder new form: `Entry` names a table of this app: a form named like it hides the table's row type in `Home`; choose another form name, such as `EntryForm`"},
		{"scaffold", []string{"new", "table", "Tag", "n:Int=1 -- evil"}, "", "alder new table: field `n`: the default `1 -- evil` holds `--`, which begins a comment in Alder source: write the value without it"},
		{"scaffold", []string{"new", "form", "Entry", "Sign", "name:Text"}, "", "alder new form: `Entry` is not a page (it has no `page` declaration): `alder new page Entry /path` makes one"},
		{"scaffold", []string{"new", "form", "Home", "Contact", "name:Text", "--into", "Post"}, "", "alder new form: `--into Post`: `Post` is not a table of this app (its tables: Entry)"},
		{"scaffold", []string{"new", "msg", "Home", "Submit"}, "", "alder new msg: `Home` already declares `Submit`: choose another message name"},
		{"scaffold", []string{"new", "msg", "Nowhere", "Go"}, "", "alder new msg: there is no page `Nowhere`: `alder new page Nowhere /path` makes one"},
		{"scaffold", []string{"new", "capability", "Weather", "now", "Query Float ->"}, "", "alder new capability: `Query Float ->` is not a type: "},
		{"scaffold", []string{"edit", "table", "Entry", "drop-field", "id"}, "", "alder edit table: `id` is the id of `Entry`: a table keeps its id"},
		{"scaffold", []string{"edit", "table", "Entry", "drop-field", "nope"}, "", "alder edit table: `Entry` has no field `nope` (its fields: id, name, message, created)"},
		{"scaffold", []string{"edit", "table", "Entry", "add-field", "name:Text"}, "", "alder edit table: `Entry` already has a field `name`"},
		{"scaffold", []string{"edit", "table", "Entry", "rename-field", "name", "message"}, "", "alder edit table: `Entry` already has a field `message`"},
		{"scaffold", []string{"edit", "table", "Nope", "drop-field", "x"}, "", "alder edit table: `Nope` is not a table of this app (its tables: Entry)"},
		{"fixture", []string{"edit", "table", "Post", "add-field", "views:Int"}, "", "alder edit table: the migration generator refuses the change: Post.views: a new non-null column needs a default (`= 0`) so existing rows can be filled"},
		{"scaffold", []string{"fill", "Home.update", "-"}, "Task.succeed Rerender", "alder fill: `Home.update` has no typed hole; to rewrite it whole, `alder fill --decl Home.update -` (`alder fill --list` lists the app's holes)"},
		{"scaffold", []string{"fill", "Nope.update", "-"}, "x", "alder fill: `Nope.update` names no module of this app: write `Module.decl`, such as `Home.update`"},
		{"scaffold", []string{"fill", "Home.update", "-"}, "\n", "alder fill: standard input is empty: write the expression that fills the hole"},
		{"scaffold", []string{"fill", "Home.update", "-"}, "Task.succeed Rerender,\n}\n\nevil : Int\nevil = 1\n", "alder fill: `alder fill` takes one expression; found more after it: `,` at line 1, column 22 of standard input"},
		{"scaffold", []string{"new", "table", "Tag", "n:Int=1, evil: Text"}, "", "alder new table: field `n`: the default `1, evil: Text` is not one expression"},
		{"scaffold", []string{"edit", "table", "Entry", "add-field", "n:Int=1, evil: Text"}, "", "alder edit table: field `n`: the default `1, evil: Text` is not one expression"},
		{"scaffold", []string{"new", "table", "Thing", "x:Nope.Type"}, "", "alder new table: the result does not check (the errors above): nothing was written"},
		{"scaffold", []string{"new", "page", "About", "/about", "--role", "Admin"}, "", "alder new page: `--role Admin` needs a session with a `user` field and a `role` field whose type is a union (`session { user: Maybe User.UserId, role: User.Role = Reader }` in `App`): this app's session has no `user` field"},
		// M13
		{"fixture", []string{"new", "auth"}, "", "alder new auth: the session already has a `user` field: the app has a login (`login = Auth.Login`)"},
		{"scaffold", []string{"new", "auth", "--role", "Role"}, "", "alder new auth: `--role Role`: module `User` has no union `Role` of constructors without payloads: `alder new type User \"Role = Member | Admin\"` makes one"},
		{"scaffold", []string{"new", "type", "Entry", "Entry = A | B"}, "", "alder new type: `Entry` already declares `Entry`: choose another name"},
		{"scaffold", []string{"new", "type", "Entry", "Mood ="}, "", "alder new type: `type Mood =` is not a type declaration: "},
		{"scaffold", []string{"new", "type", "Entry", "Slug = Text where Text.matches \"^[a-z]+$\""}, "", "alder new type: `type Slug = Text where Text.matches \"^[a-z]+$\"` is not a type declaration: a brand is written `Slug = brand Text where …` (chapter 4), or `Slug = brand Text` without a check"},
		{"scaffold", []string{"new", "type", "entry", "Mood = A"}, "", "alder new type: `entry` is not a module name"},
		{"scaffold", []string{"new", "decl", "Home", "load", "Int"}, "", "alder new decl: `Home` already declares `load`: choose another name, or `alder fill --decl Home.load` to replace its body"},
		{"scaffold", []string{"new", "decl", "Nope", "x", "Int"}, "", "alder new decl: there is no module `Nope`"},
		{"scaffold", []string{"new", "decl", "Home", "x", "Int ->"}, "", "alder new decl: `Int ->` is not a type: "},
		{"scaffold", []string{"new", "test", "Home", "signing adds an entry"}, "", "alder new test: `Home` already has a test \"signing adds an entry\": choose another name"},
		{"scaffold", []string{"new", "page", "Notes", "/notes", "--blank", "--list", "Entry"}, "", "alder new page: --blank takes no --list or --from"},
		{"scaffold", []string{"new", "msg", "Home", "Hide", "--in-for", "x"}, "", "alder new msg: `Home`'s `view` has no `{for x in …}`: name the loop variable of one of its loops"},
		{"scaffold", []string{"new", "msg", "Home", "Hide", "--when", "a b c,"}, "", "alder new msg: `--when a b c,` is not one expression"},
		{"scaffold", []string{"edit", "page", "Entry", "add-param", "q:Text"}, "", "alder edit page: `Entry` is not a page (it has no `page` declaration)"},
		{"scaffold", []string{"edit", "page", "Home", "add-param", "q:Text"}, "", "alder edit page: the result does not check (the errors above): nothing was written"},
		{"scaffold", []string{"fill", "--decl", "Home.update.Submit", "-"}, "Task.succeed Rerender", "alder fill: `Home.update.Submit` names an arm: --decl replaces a whole declaration, `Home.update`"},
		{"scaffold", []string{"fill", "--decl", "Home.nope", "-"}, "1", "alder fill: `Home` has no declaration `nope` with a body (a value, a type or a test)"},
		{"scaffold", []string{"fill", "--decl", "Home.load", "-"}, "Query.succeed { entries: [] },\n}\n", "alder fill: `alder fill --decl` takes one expression; found more after it"},
		{"scaffold", []string{"fill", "--decl", "Home.Model", "-"}, "{ entries: List Entry } | x", "alder fill: `alder fill --decl` takes one type for `type Model`"},
		// M13 final
		{"scaffold", []string{"new", "page", "Tags.Show", "/tags/{name}", "--param", "slug:Text"}, "", "alder new page: `/tags/{name}` has the parameter `{name}` but no `--param name:<Type>`"},
		{"scaffold", []string{"new", "page", "Tags.Show", "/tags/{name}", "--param", "name:Text", "--param", "x:Int"}, "", "alder new page: --param x: `/tags/{name}` has no `{x}` segment"},
		{"scaffold", []string{"new", "page", "Entries.Show", "/entries", "--show", "Entry", "--by", "name"}, "", "alder new page: `--by name`: `name` is not a `@unique` column of `Entry`"},
		{"scaffold", []string{"new", "page", "Entries.Show", "/entries", "--by", "name"}, "", "alder new page: --by names the column --show T reads its row by: give --show T too"},
		{"scaffold", []string{"new", "form", "Home", "Quick", "name:Text", "--msg", "Submit"}, "", "alder new form: `Home` already declares `Submit`: choose another name"},
		{"scaffold", []string{"new", "msg", "Home", "Clear", "--id", "1x"}, "", "alder new msg: `--id 1x` is not an id"},
		{"scaffold", []string{"new", "table", "Tag", "created:Time@created"}, "", "alder new table: field `created`: the generator writes `created: Time @created` and `updated: Time @updated`; leave them out, or give --no-timestamps"},
		{"scaffold", []string{"edit", "session", "add-field", "theme:Text"}, "", "alder edit session: session field `theme` needs a default"},
		{"fixture", []string{"edit", "session", "add-field", "role:Text=\"x\""}, "", "alder edit session: the session already has a field `role`"},
		// an error in the edited declaration itself refuses, whatever the write-despite-errors policy
		{"scaffold", []string{"edit", "session", "add-field", "bad:Theme=Nope"}, "", "alder edit session: the declaration the command edits does not check (the errors above): nothing was written"},
		{"scaffold", []string{"edit", "session", "add-field", "n:Int=\"x\""}, "", "alder edit session: the declaration the command edits does not check (the errors above): nothing was written"},
		{"scaffold", []string{"edit", "table", "Entry", "add-field", "bad:Theme=Nope"}, "", "alder edit table: the result does not check (the errors above): nothing was written"},
		{"scaffold", []string{"edit", "table", "Entry", "add-field", "n:Int=\"x\""}, "", "alder edit table: the result does not check (the errors above): nothing was written"},
		{"scaffold", []string{"remove", "table", "Entry"}, "", "alder remove table: table `Entry` is still used (the errors above): remove those uses first; nothing was removed"},
		{"scaffold", []string{"remove", "page", "Entry"}, "", "alder remove page: `Entry` is not a page"},
		{"scaffold", []string{"remove", "table", "Home"}, "", "alder remove table: `Home` is not a table of this app"},
		{"fixture", []string{"remove", "page", "Auth.Login"}, "", "alder remove page: `Auth.Login` is still used (the errors above)"},
		{"scaffold", []string{"fill", "--decl", "Home.Model", "-"}, "{ entries: List Entry, params: Params }", "alder fill: the result does not check (the errors above): nothing was written"}, // I6: a diagnostic, not a crash
		{"scaffold", []string{"fill", "--decl", "Home.load", "-"}, "\x0crom params session -> Query.succeed { entries: [] }", "alder fill: standard input has the control character U+000C at line 1, column 1: a shell's `echo`"},
		{"scaffold", []string{"fill", "Home.update.Submit", "-"}, "rom to -> rom", "alder fill: `alder fill` takes one expression; found more after it: `->` at line 1, column 8 of standard input (a lambda starts with `\\`: `\\a b -> …`)"},
		{"scaffold", []string{"fill", "Home.update.Submit", "-"}, "Task.succeed Rerender", "alder fill: `Home.update.Submit` has no typed hole: a fill replaces a hole, and this arm has none; to rewrite it, give `alder fill --decl Home.update -` the whole `match msg { … }`"},
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

	// two holes in one declaration, and a hole not there
	dir := genApp(t, "scaffold")
	alderIn(t, dir, "", "new", "msg", "Home", "Pin", "note:Text")
	if code, _, errs := alderIn(t, dir, "Rerendered", "fill", `Home."pressing Pin"`, "-"); code != 1 || errs != "alder fill: `Home.\"pressing Pin\"` has 2 holes (?pinNote, ?expected): name one with --hole\n" {
		t.Errorf("two holes: exit %d, %q", code, errs)
	}
	if code, _, errs := alderIn(t, dir, "x", "fill", `Home."pressing Pin"`, "-", "--hole", "?nope"); code != 1 || errs != "alder fill: `Home.\"pressing Pin\"` has no hole `?nope` (it has ?pinNote, ?expected)\n" {
		t.Errorf("no such hole: exit %d, %q", code, errs)
	}
	if code, out, _ := alderIn(t, dir, "r.answer == Rerendered", "fill", `Home."pressing Pin"`, "-", "--hole", "?expected"); code != 0 || !strings.Contains(out, "Task.succeed (r.answer == Rerendered)") {
		t.Errorf("--hole: exit %d\n%s", code, out)
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

// TestGenerate_JSON: --json is one object, {files, diff, diagnostics, holes, written}, with
// the refusal's sentence in "error".
func TestGenerate_JSON(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, _ := alderIn(t, dir, "", "new", "msg", "Home", "Clear", "--json")
	if code != 0 || strings.Count(out, "\n") != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	var got struct {
		Files []struct{ Path, Action string }
		Diff  string
		Diags []struct {
			Code, Severity, File, Message string
			Line, Col                     int
		} `json:"diagnostics"`
		Holes []struct {
			Module, Decl, Arm, Name, Type, File string
			Line, Col                           int
		}
		Written bool
		Error   string
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "app/Home.alder" || got.Files[0].Action != "edited" || !strings.HasPrefix(got.Diff, "--- a/app/Home.alder\n+++ b/app/Home.alder\n@@ ") || !got.Written || got.Error != "" {
		t.Errorf("result: %+v", got)
	}
	if len(got.Diags) != 2 || got.Diags[0].Code != "W0002" || got.Diags[0].Message != "hole ?clear has type Task Nav" {
		t.Errorf("diagnostics: %+v", got.Diags)
	}
	if len(got.Holes) != 2 || got.Holes[0].Module != "Home" || got.Holes[0].Decl != "update" || got.Holes[0].Arm != "Clear" || got.Holes[0].Name != "clear" || got.Holes[0].Type != "Task Nav" ||
		got.Holes[1].Decl != `"pressing Clear"` || got.Holes[1].Type != "Bool" {
		t.Errorf("holes: %+v", got.Holes)
	}
	code, out, errs := alderIn(t, dir, "", "new", "page", "Home", "/x", "--json")
	if code != 1 || !strings.Contains(out, `"error":"module `+"`Home`"+` exists`) || !strings.Contains(out, `"files":[]`) || !strings.Contains(out, `"written":false`) || errs == "" {
		t.Errorf("a refusal: exit %d\n%s%s", code, out, errs)
	}
	code, out, _ = alderIn(t, dir, "", "fill", "--list", "--json")
	if code != 0 || !strings.HasPrefix(out, `{"holes":[{"module":"Home","decl":"update","arm":"Clear","name":"clear","type":"Task Nav"`) {
		t.Errorf("fill --list --json: exit %d\n%s", code, out)
	}
	code, out, _ = alderIn(t, dir, "", "fill", "--list")
	if code != 0 || out != "Home update.Clear ?clear : Task Nav\nHome \"pressing Clear\" ?expected : Bool\n" {
		t.Errorf("fill --list: exit %d\n%s", code, out)
	}
}

// TestGenerate_DryRun: --dry-run says what it would write, created files in full, and writes
// nothing; the output is the same on every run.
func TestGenerate_DryRun(t *testing.T) {
	args := []string{"new", "table", "Tag", "name:Text.NonEmpty", "entry:Entry.EntryId@ref=Entry"}
	dir := genApp(t, "scaffold")
	before := appFiles(t, dir)
	code, dry, _ := alderIn(t, dir, "", append(slices.Clone(args), "--dry-run")...)
	if code != 0 || !strings.HasPrefix(dry, "would write app/Tag.alder\nwould write migrations/0002_create_tag.sql\n") ||
		!strings.Contains(dry, "would edit migrations/schema.json\n") || !strings.Contains(dry, "--- /dev/null\n+++ b/app/Tag.alder\n@@ -0,0 +1,") ||
		!strings.Contains(dry, "+table Tag {\n") || !strings.HasSuffix(dry, "dry run: nothing written\n") {
		t.Errorf("--dry-run: exit %d\n%s", code, dry)
	}
	if ch := changedFiles(before, appFiles(t, dir)); len(ch) != 0 {
		t.Errorf("--dry-run wrote %v", slices.Sorted(maps.Keys(ch)))
	}
	code, out, _ := alderIn(t, dir, "", args...)
	if code != 0 || !strings.HasPrefix(out, "wrote app/Tag.alder\nwrote migrations/0002_create_tag.sql\n") || strings.Contains(out, "/dev/null") {
		t.Errorf("the run after the dry run printed:\n%s", out)
	}
	other := genApp(t, "scaffold")
	if _, again, _ := alderIn(t, other, "", args...); again != out {
		t.Errorf("a second app printed:\n%s\nthe first:\n%s", again, out)
	}
	if a, b := appFiles(t, dir), appFiles(t, other); a["app/Tag.alder"] != b["app/Tag.alder"] || a["migrations/0002_create_tag.sql"] != b["migrations/0002_create_tag.sql"] {
		t.Errorf("two runs wrote different files")
	}
	// --json: the migration's steps as they are, unindented
	code, out, _ = alderIn(t, genApp(t, "scaffold"), "", append(slices.Clone(args), "--json")...)
	if code != 0 || !strings.Contains(out, `"migration":["create table Tag","add index on Tag.entry"]`) {
		t.Errorf("--json: exit %d\n%s", code, out)
	}
}

// TestGenerate_Args: --show takes a path with or without its `/{id}`; --from's tables may come
// before the path.
func TestGenerate_Args(t *testing.T) {
	dir := genApp(t, "scaffold")
	if code, _, errs := alderIn(t, dir, "", "new", "page", "Entries.Show", "/entries/{id}", "--show", "Entry"); code != 0 {
		t.Fatalf("--show with /{id}: exit %d: %s", code, errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "app", "Entries", "Show.alder")); !strings.Contains(string(b), `page "/entries/{id}" { id: Entry.EntryId }`) {
		t.Errorf("Entries.Show:\n%s", b)
	}
	if code, _, errs := alderIn(t, dir, "", "new", "page", "Dash", "--from", "Entry", "/dash"); code != 0 {
		t.Fatalf("--from before the path: exit %d: %s", code, errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "app", "Dash.alder")); !strings.Contains(string(b), `page "/dash" { }`) {
		t.Errorf("Dash:\n%s", b)
	}
}

// genSequence is the golden command sequence of TestGenerate_Mutate on the fixture: each
// generator, with the holes it leaves filled as an author would.
var genSequence = []struct {
	args  []string
	stdin string
}{
	{args: []string{"new", "page", "Posts.Index", "/posts", "--list", "Post", "--requires", "user"}},
	{args: []string{"new", "page", "Posts.Show", "/posts", "--show", "Post", "--requires", "user"}},
	{args: []string{"new", "page", "Dash", "/dash", "--from", "Post", "Tag", "--requires", "user"}},
	{args: []string{"new", "page", "Users.Index", "/users", "--list", "User", "--role", "Admin"}},
	{args: []string{"new", "table", "Like", "post:Post.PostId@ref=Post@cascade", "owner:User.UserId@ref=User@owner"}},
	{args: []string{"new", "form", "Home", "Quick", "title:Text.NonEmpty", "body:Text"}},
	{args: []string{"new", "form", "Home", "Tagging", "name:Text.NonEmpty", "post:Post.PostId"}},
	{args: []string{"new", "msg", "Posts.Show", "Feature", "id:Post.PostId"}},
	{args: []string{"fill", "Posts.Show.view", "-"}, stdin: "model.row.id"},
	{args: []string{"fill", "Posts.Show.update.Feature", "-"}, stdin: "{\n  let _ <- Db.updateMine Post model.row.id (\\t -> { t | featured: True })\n  Task.succeed Rerender\n}"},
	{args: []string{"fill", `Posts.Show."pressing Feature"`, "-"}, stdin: "{ answer: r.answer, featured: Maybe.map (\\x -> x.featured) after } == { answer: Rerendered, featured: Just True }"},
	{args: []string{"new", "capability", "Mail", "send", "{ to: Text.Email, subject: Text } -> Task Unit"}},
	{args: []string{"fill", "Mail.Mail.send", "-"}, stdin: "Task.succeed ()"},
}

var scoreRe = regexp.MustCompile(`(?m)^(\d+) mutants: (\d+) killed, (\d+) survived  \(score (\d\.\d\d)\)$`)

// TestGenerate_Mutate: the tests a generator writes kill the mutants of the code it writes.
// The fixture, after the golden sequence of every generator (its holes filled), scores at
// least 0.90 under `alder test --mutate`; the failure lists the survivors.
func TestGenerate_Mutate(t *testing.T) {
	needGoTool(t)
	if testing.Short() {
		t.Skip("runs every mutant of the fixture")
	}
	dir := genApp(t, "fixture")
	for _, step := range genSequence {
		if code, out, errs := alderIn(t, dir, step.stdin, step.args...); code != 0 {
			t.Fatalf("alder %s: exit %d\n%s%s", strings.Join(step.args, " "), code, out, errs)
		}
	}
	var out, errb bytes.Buffer
	code := runTestTo([]string{dir, "--mutate"}, &out, &errb)
	m := scoreRe.FindStringSubmatch(out.String())
	if code != 0 || m == nil {
		t.Fatalf("alder test --mutate: exit %d\n%s%s", code, out.String(), errb.String())
	}
	t.Logf("the fixture after the generators: %s", m[0])
	if m[4] < "0.90" {
		_, survivors, _ := strings.Cut(out.String(), "survived:\n")
		t.Errorf("score %s < 0.90; survivors:\n%s", m[4], survivors)
	}
}

// TestGenerate_FillImports: a fill that names a module of the app qualified (`Comment.body`)
// which the hole's module does not import gets the import, said on standard output and in
// --json's "imports"; a standard library module's name (`Text.trim`) needs none.
func TestGenerate_FillImports(t *testing.T) {
	dir := genApp(t, "fixture")
	if code, _, errs := alderIn(t, dir, "", "new", "msg", "Home", "Purge"); code != 0 {
		t.Fatalf("alder new msg: %s", errs)
	}
	fill := "{\n  let n <- Db.from Comment |> Db.where (Comment.body.contains (Text.trim \"x\")) |> Db.count\n  Task.succeed (if n > 0 then Rerender else Rerender)\n}"
	code, out, errs := alderIn(t, dir, fill, "fill", "Home.update.Purge", "-", "--dry-run", "--json")
	if code != 0 || !strings.Contains(out, `"imports":["Comment"]`) || !strings.Contains(out, `+import Comment\n`) {
		t.Errorf("fill --dry-run --json: exit %d\n%s%s", code, out, errs)
	}
	code, out, errs = alderIn(t, dir, fill, "fill", "Home.update.Purge", "-")
	if code != 0 || !strings.HasSuffix(out, "added import Comment to app/Home.alder\n") || !strings.Contains(out, "+import Comment\n") {
		t.Fatalf("fill: exit %d\n%s%s", code, out, errs)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "app", "Home.alder")); strings.Count(string(b), "import Comment\n") != 1 || strings.Contains(string(b), "import Text\n") {
		t.Errorf("Home's imports:\n%s", b)
	}
	if code, out, _ := alderIn(t, dir, "", "check"); code != 0 || out != "" {
		t.Errorf("check after the fill: exit %d\n%s", code, out)
	}
}

// TestGenerate_FillDecl: `alder fill --decl` replaces a whole body, a value's, a type's or a
// test's, and writes only what checks. Review Focus 5: an `update` that drops an arm the view's
// form posts is refused by the checker, and nothing is written.
func TestGenerate_FillDecl(t *testing.T) {
	dir := genApp(t, "scaffold")
	before := appFiles(t, dir)
	dropped := "match msg {\n}"
	code, out, errs := alderIn(t, dir, dropped, "fill", "--decl", "Home.update", "-")
	if code != 1 || errs != "alder fill: the result does not check (the errors above): nothing was written\n" || !strings.Contains(out, "error[") {
		t.Errorf("an update without the Submit arm: exit %d\nstdout:\n%s\nstderr: %s", code, out, errs)
	}
	if len(changedFiles(before, appFiles(t, dir))) != 0 {
		t.Errorf("a refused --decl wrote files")
	}
	// Model and load, one at a time: a Model that load does not build is refused
	if code, _, _ := alderIn(t, dir, "{ entries: List Entry, count: Int }", "fill", "--decl", "Home.Model", "-"); code != 1 {
		t.Errorf("a Model that load does not build: exit %d", code)
	}
	if code, out, errs := alderIn(t, dir, "{ entries: List Entry }", "fill", "--decl", "Home.Model", "-"); code != 0 || out != "" {
		t.Errorf("the same Model: exit %d\n%s%s", code, out, errs)
	}
	home, _ := os.ReadFile(filepath.Join(dir, "app", "Home.alder"))
	_, view, _ := strings.Cut(string(home), "view model session =\n")
	view, _, _ = strings.Cut(view, "\n\n test")
	view, _, _ = strings.Cut(view, "</Layout.Page>\n")
	var lines []string
	for _, l := range strings.Split(view+"</Layout.Page>", "\n") {
		lines = append(lines, strings.TrimPrefix(l, "  "))
	}
	body := strings.Replace(strings.Join(lines, "\n"), "<h1>Guestbook</h1>", "<h1>The guestbook</h1>", 1)
	code, out, errs = alderIn(t, dir, body, "fill", "--decl", "Home.view", "-")
	if code != 0 || !strings.Contains(out, "-    <h1>Guestbook</h1>\n+    <h1>The guestbook</h1>\n") || strings.Count(out, "\n+") != 2 {
		t.Fatalf("view: exit %d\n%s%s", code, out, errs)
	}
	test := "{\n  let page <- Test.get (Home {})\n  Task.succeed (Test.text \"h1\" page.doc == Just \"The guestbook\")\n}"
	if code, out, errs := alderIn(t, dir, test, "fill", "--decl", `Home."signing adds an entry"`, "-"); code != 0 || !strings.Contains(out, "+  Task.succeed (Test.text \"h1\" page.doc == Just \"The guestbook\")\n") ||
		!strings.HasSuffix(out, "rewriting a test: `alder test --mutate` is the check that it still tests something\n") {
		t.Fatalf("a test: exit %d\n%s%s", code, out, errs)
	}
	if code, out, errs := alderIn(t, dir, "", "test"); code != 0 || !strings.HasSuffix(out, "2 passed, 0 failed\n") {
		t.Errorf("alder test: exit %d\n%s%s", code, out, errs)
	}
	src, _ := os.ReadFile(filepath.Join(dir, "app", "Home.alder"))
	if !strings.Contains(string(src), "view model session =\n  <Layout.Page title=\"Guestbook\">\n    <h1>The guestbook</h1>\n\n    <al-card>\n") ||
		!strings.Contains(string(src), "  </Layout.Page>\n\ntest \"signing adds an entry\" = {\n  let page <- Test.get (Home {})\n") {
		t.Errorf("Home:\n%s", src)
	}
}

// TestGenerate_AuthAdapter: the adapter `alder new auth` writes builds with the app, passes its
// own Go tests (Review Focus 3: a wrong password, an unknown email, another iteration count, a
// malformed stored text), and the command is deterministic: on two apps it writes the same
// files.
func TestGenerate_AuthAdapter(t *testing.T) {
	needGoTool(t)
	dir := genApp(t, "scaffold")
	_, out, _ := alderIn(t, dir, "", "new", "auth")
	other := genApp(t, "scaffold")
	if _, again, _ := alderIn(t, other, "", "new", "auth"); again != out {
		t.Errorf("a second app printed:\n%s\nthe first:\n%s", again, out)
	}
	a, b := appFiles(t, dir), appFiles(t, other)
	for _, f := range []string{"app/App.alder", "app/Auth/Login.alder", "app/User.alder", "adapters/auth/login_password.go", "adapters/auth/login_password_test.go", "migrations/0002_create_user.sql"} {
		if a[f] == "" || a[f] != b[f] {
			t.Errorf("%s: missing or different on two runs", f)
		}
	}
	if !strings.Contains(a["adapters/auth/login_password.go"], "iterations = 600000") || !strings.Contains(a["adapters/auth/login_password.go"], "subtle.ConstantTimeCompare(got, p.want)") {
		t.Errorf("the adapter:\n%s", a["adapters/auth/login_password.go"])
	}
	t.Chdir(filepath.Dir(dir))
	if code, stdout, stderr := runBuild(t, filepath.Base(dir)); code != 0 {
		t.Fatalf("alder build: exit %d\n%s%s", code, stdout, stderr)
	}
	testOut, err := buildkit.GoCmd(filepath.Join("build", filepath.Base(dir)), "test", "-count=1", "-v", "./adapters/...")
	if err != nil || !strings.Contains(string(testOut), "ok  \tapp/adapters/auth") {
		t.Fatalf("go test ./adapters/...: %v\n%s", err, testOut)
	}
	for _, name := range []string{"TestHash_RoundTrip", "TestVerify", "TestVerify_UnknownEmailDerives", "TestVerify_Rehash", "TestVerify_RehashFails", "TestCheck_StoredIterations", "TestCheck_WholeKey", "TestCheck_Malformed"} {
		if !strings.Contains(string(testOut), "--- PASS: "+name) {
			t.Errorf("%s did not pass:\n%s", name, testOut)
		}
	}
}

// TestGenerate_TableInTypeModule: `alder new table User` after `alder new type User "Role =
// …"` adds the table to the module the type made, which then uses the type; `alder new auth
// --role` takes that table as it is.
func TestGenerate_TableInTypeModule(t *testing.T) {
	dir := genApp(t, "scaffold")
	for _, args := range [][]string{
		{"new", "type", "User", "Role = Member | Admin"},
		{"new", "table", "User", "email:Text.Email@unique", "name:Text.NonEmpty", "passwordHash:Text", "role:Role=Member"},
		{"new", "auth", "--role", "Role"},
	} {
		if code, out, errs := alderIn(t, dir, "", args...); code != 0 {
			t.Fatalf("alder %s: exit %d\n%s%s", strings.Join(args, " "), code, out, errs)
		}
	}
	src, _ := os.ReadFile(filepath.Join(dir, "app", "User.alder"))
	if !strings.HasPrefix(string(src), "module User\n\ntype Role = Member | Admin\n\ntype UserId = brand Text\n\ntable User {\n") {
		t.Errorf("User:\n%s", src)
	}
	if code, out, errs := alderIn(t, dir, "", "test"); code != 0 || !strings.HasSuffix(out, " passed, 0 failed\n") {
		t.Errorf("alder test: exit %d\n%s%s", code, out, errs)
	}
}

// TestGenerate_ViewHole (I7): a form or a button needs a view to go in: on a page whose `view`
// is still the hole ?view, `alder new form` and `alder new msg` refuse, saying to fill it first.
func TestGenerate_ViewHole(t *testing.T) {
	dir := genApp(t, "scaffold")
	if code, out, errs := alderIn(t, dir, "", "new", "page", "Notes", "/notes", "--blank"); code != 0 {
		t.Fatalf("new page: exit %d\n%s%s", code, out, errs)
	}
	for _, args := range [][]string{
		{"new", "form", "Notes", "Quick", "title:Text"},
		{"new", "msg", "Notes", "Clear"},
	} {
		code, _, errs := alderIn(t, dir, "", args...)
		if code != 1 || !strings.Contains(errs, "`Notes`'s `view` is a hole: fill `view` first (`alder fill Notes.view -`)") {
			t.Errorf("alder %s: exit %d, %s", strings.Join(args, " "), code, errs)
		}
	}
}

// TestHelp (m12): `alder <command> --help` prints the command's usage on standard output, a
// generator's for `alder new <generator> --help`.
func TestHelp(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"new", "page", "--help"}, newPageUsage},
		{[]string{"fill", "-h"}, fillUsage},
		{[]string{"edit", "--help"}, editUsage},
		{[]string{"remove", "--help"}, removeUsage},
		{[]string{"help", "test"}, testUsage},
		{[]string{"new", "--help"}, newUsage},
	} {
		var b bytes.Buffer
		if !helpTo(c.args, &b) || b.String() != c.want+"\n" {
			t.Errorf("%v: %q", c.args, b.String())
		}
	}
	if helpTo([]string{"new", "page", "X", "/x"}, io.Discard) {
		t.Error("no --help: helpTo answered")
	}
}

// TestGenerate_E11Commands: `new form --msg` on a message declared without a form, and `edit
// auth --to` (their goldens), and the refusals of the four commands.
func TestGenerate_E11Commands(t *testing.T) {
	needGoTool(t)
	t.Run("form-on-msg", func(t *testing.T) {
		dir := genApp(t, "fixture")
		if code, _, errs := alderIn(t, dir, "", "new", "msg", "Home", "Feature", "id:Post.PostId"); code != 0 {
			t.Fatalf("new msg: %s", errs)
		}
		before := appFiles(t, dir)
		code, out, errs := alderIn(t, dir, "", "new", "form", "Home", "Reason", "why:Text.NonEmpty", "--msg", "Feature")
		if code != 0 {
			t.Fatalf("new form --msg: exit %d\n%s%s", code, out, errs)
		}
		golden(t, "form-on-msg", out, changedFiles(before, appFiles(t, dir)))
		if code, _, errs := alderIn(t, dir, "", "new", "form", "Home", "Other", "why:Text", "--msg", "Feature"); code != 1 || !strings.Contains(errs, "`Home` already declares `Feature`") {
			t.Errorf("a second form on the message: exit %d: %s", code, errs)
		}
	})
	t.Run("edit-auth", func(t *testing.T) {
		dir := genApp(t, "fixture")
		if code, _, errs := alderIn(t, dir, "", "edit", "auth", "--to", "Home"); code != 1 || !strings.Contains(errs, "no hole ?afterLogin") {
			t.Errorf("without the hole: exit %d: %s", code, errs)
		}
		bare := filepath.Join(t.TempDir(), "app")
		var o, e bytes.Buffer
		if code := runNewTo([]string{bare, "--bare"}, &o, &e); code != 0 {
			t.Fatalf("new --bare: %s", e.String())
		}
		for _, args := range [][]string{{"new", "auth"}, {"new", "page", "Notes", "/notes", "--blank", "--requires", "user"}} {
			if code, _, errs := alderIn(t, bare, "", args...); code != 0 {
				t.Fatalf("%v: %s", args, errs)
			}
		}
		before := appFiles(t, bare)
		code, out, errs := alderIn(t, bare, "", "edit", "auth", "--to", "Notes")
		if code != 0 {
			t.Fatalf("edit auth: exit %d\n%s%s", code, out, errs)
		}
		golden(t, "edit-auth", out, changedFiles(before, appFiles(t, bare)))
		if code, _, errs := alderIn(t, bare, "", "edit", "auth", "--to", "Notes"); code != 1 || !strings.Contains(errs, "no hole ?afterLogin") {
			t.Errorf("a second time: exit %d: %s", code, errs)
		}
	})
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"edit", "decl", "Layout.Page", "add-prop", "dark:Bool"}, "needs a default"},
		{[]string{"edit", "decl", "Layout.Page", "add-prop", "title:Text=\"x\""}, "already has a property `title`"},
		{[]string{"new", "msg", "App", "Logout"}, "`App` already declares `Logout`"},
	} {
		dir := genApp(t, "fixture")
		code, _, errs := alderIn(t, dir, "", c.args...)
		if code != 1 || !strings.Contains(errs, c.want) {
			t.Errorf("%v: exit %d: %s (want %q)", c.args, code, errs, c.want)
		}
	}
}

// TestGenerate_E11Messages: the refusals that name the fix (E11 §7): a lambda over bound
// parameters, a hole-less test or Model, a test's block in a Bool hole, a bare app type, and `alder test`
// with holes left; an unquoted test address is accepted.
func TestGenerate_E11Messages(t *testing.T) {
	needGoTool(t)
	dir := genApp(t, "fixture")
	run := func(stdin string, args ...string) (int, string) {
		code, out, errs := alderIn(t, dir, stdin, args...)
		return code, out + errs
	}
	if code, o := run("", "new", "page", "Notes", "/notes", "--blank"); code != 0 {
		t.Fatalf("new page: %s", o)
	}
	for _, args := range [][]string{{"fill", "Notes.load", "-"}, {"fill", "--decl", "Notes.load", "-"}} {
		code, o := run("\\params session -> Query.succeed { }", args...)
		if code != 1 || !strings.Contains(o, "the parameters `params session` are bound already: send the body without `\\params session ->`") {
			t.Errorf("%v: exit %d: %s", args, code, o)
		}
	}
	if code, o := run("Query.succeed { }", "fill", "Notes.load", "-"); code != 0 {
		t.Fatalf("fill load: %s", o)
	}
	if code, o := run("", "new", "test", "Notes", "a b c"); code != 0 {
		t.Fatalf("new test: %s", o)
	}
	if code, o := run("Task.succeed True", "fill", "Notes.a b c", "-"); code != 0 {
		t.Errorf("an unquoted test address: exit %d: %s", code, o)
	}
	if code, o := run("True", "fill", `Notes."the page is shown"`, "-"); code != 1 || !strings.Contains(o, `alder fill --decl 'Notes."the page is shown"' -`) {
		t.Errorf("a test without a hole: exit %d: %s", code, o)
	}
	if code, o := run("", "new", "msg", "Home", "Clear"); code != 0 {
		t.Fatalf("new msg: %s", o)
	}
	if code, o := run("{\n  Task.succeed True\n}", "fill", `Home."pressing Clear"`, "-"); code != 1 || !strings.Contains(o, "to replace the whole test, use `alder fill --decl") {
		t.Errorf("a block in ?expected: exit %d: %s", code, o)
	}
	if code, o := run("", "new", "msg", "Home", "Pick", "role:Role"); code != 1 || !strings.Contains(o, "write `User.Role`") {
		t.Errorf("a bare app type: exit %d: %s", code, o)
	}
	if code, o := run("", "test"); code != 1 || !strings.Contains(o, "fill the hole): ") || !strings.Contains(o, "?expected : Bool") {
		t.Errorf("alder test with holes: exit %d: %s", code, o)
	}
}
