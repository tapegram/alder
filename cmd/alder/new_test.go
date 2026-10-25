package main

import (
	"bytes"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"alder/internal/scaffold"
)

func runNew(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runNewTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

// outsideModule is a fresh directory that no go.mod is above (the test skips when the
// temporary directory is itself inside a Go module).
func outsideModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if p, err := scaffold.ParentModule(filepath.Join(dir, "x")); err != nil || p != "" {
		t.Skipf("the temporary directory is inside the Go module of %s", p)
	}
	return dir
}

// tree is every file under dir, slash-separated and relative to it, sorted; build/ is left out.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() && rel == "build" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

var scaffoldFiles = []string{
	".gitignore",
	"README.md",
	"adapters/README.md",
	"app/App.alder",
	"app/Home.alder",
	"app/Layout.alder",
	"app/Table/Entry.alder",
	"migrations/0001_initial.sql",
	"migrations/schema.json",
	"scratch.alder",
	"static/app.css",
}

// TestNew_Writes: outside any Go module `alder new` writes exactly the scaffold, no go.mod, the
// name in the README, and the migrations `alder migrate` writes for those tables (a second run
// of the generator finds nothing to do); it prints the files and the five commands.
func TestNew_Writes(t *testing.T) {
	t.Parallel()
	base := outsideModule(t)
	dir := filepath.Join(base, "guestbook")
	code, out, errs := runNew(t, dir)
	if code != 0 || errs != "" {
		t.Fatalf("alder new: exit %d, stderr %q", code, errs)
	}
	if got := tree(t, dir); !slices.Equal(got, scaffoldFiles) {
		t.Errorf("files:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(scaffoldFiles, "\n"))
	}
	want := "created " + dir + "/\n"
	for _, f := range scaffoldFiles {
		want += "  " + f + "\n"
	}
	want += "\nnext:\n  cd " + dir + "\n" +
		"  alder check            # type-check the app\n" +
		"  alder test             # run its tests\n" +
		"  alder test --mutate    # measure the tests\n" +
		"  alder build            # build the server, build/guestbook/guestbook\n" +
		"  alder run              # serve on http://localhost:8080, rebuilding on every save\n"
	if out != want {
		t.Errorf("printed:\n%s\nwant:\n%s", out, want)
	}
	readme := readFile(t, filepath.Join(dir, "README.md"))
	if !strings.HasPrefix(readme, "# guestbook\n") || !strings.Contains(readme, "`alder new guestbook`") || strings.Contains(readme, "{{") {
		t.Errorf("README.md:\n%s", readme)
	}
	for _, c := range []string{"alder check", "alder test", "alder test --mutate", "alder build", "alder run"} {
		if !strings.Contains(readme, "    "+c+" ") {
			t.Errorf("README.md lacks the command %q", c)
		}
	}
	if sql := readFile(t, filepath.Join(dir, "migrations", "0001_initial.sql")); !strings.Contains(sql, `CREATE TABLE "Table__Entry__Entry"`) {
		t.Errorf("0001_initial.sql:\n%s", sql)
	}
	if code, out, _ := runMigrate(t, dir); code != 0 || !strings.HasPrefix(out, "no changes since 0001_initial") {
		t.Errorf("alder migrate after alder new: exit %d, %q", code, out)
	}
	if code, out, errs := run(t, dir); code != 0 || out != "" || errs != "" {
		t.Errorf("alder check: exit %d, %q %q", code, out, errs)
	}

	// another name is substituted where the name appears; an empty directory is accepted
	empty := filepath.Join(base, "my-app2")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := runNew(t, empty); code != 0 || !strings.Contains(out, "build/my-app2/my-app2") {
		t.Fatalf("alder new into an empty directory: exit %d, %q %q", code, out, errs)
	}
	if readme := readFile(t, filepath.Join(empty, "README.md")); !strings.HasPrefix(readme, "# my-app2\n") || strings.Contains(readme, "guestbook/") {
		t.Errorf("README.md of my-app2:\n%s", readme)
	}
}

// TestNew_Refusals: a name that is not letters, digits and `-` starting with a letter, an
// existing non-empty directory and a file are refused (exit 1) and nothing is written; usage
// errors exit 2.
func TestNew_Refusals(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	for _, name := range []string{"9lives", "my_app", "my app", "café"} {
		code, out, errs := runNew(t, filepath.Join(base, name))
		want := "alder new: `" + name + "` is not a valid app name: use letters, digits and `-`, starting with a letter"
		if code != 1 || out != "" || !strings.HasPrefix(errs, want) {
			t.Errorf("alder new %q: exit %d, %q %q", name, code, out, errs)
		}
		if _, err := os.Stat(filepath.Join(base, name)); err == nil {
			t.Errorf("alder new %q wrote %s", name, name)
		}
	}
	if code, _, errs := runNew(t, "."); code != 1 || !strings.HasPrefix(errs, "alder new: `.` is not a valid app name") {
		t.Errorf("alder new .: exit %d, %q", code, errs)
	}
	full := filepath.Join(base, "taken")
	if err := os.MkdirAll(filepath.Join(full, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runNew(t, full); code != 1 || errs != "alder new: `"+full+"` exists and is not empty: choose another name, or empty it first\n" {
		t.Errorf("a non-empty directory: exit %d, %q", code, errs)
	}
	if got := tree(t, full); len(got) != 0 {
		t.Errorf("wrote into the non-empty directory: %v", got)
	}
	file := filepath.Join(base, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := runNew(t, file); code != 1 || errs != "alder new: `"+file+"` is a file: choose another name\n" {
		t.Errorf("a file: exit %d, %q", code, errs)
	}
	if code, _, errs := runNew(t, ""); code != 2 || errs != "alder new: the app needs a name, such as `alder new guestbook`\n" {
		t.Errorf("alder new \"\": exit %d, %q", code, errs)
	}
	for _, args := range [][]string{nil, {"a", "b"}, {"--force", "a"}, {"-x"}} {
		if code, _, errs := runNew(t, args...); code != 2 || !strings.HasPrefix(errs, "usage: alder new") {
			t.Errorf("alder new %v: exit %d, %q", args, code, errs)
		}
	}
}

// TestNew_InsideModule (RF3): under a directory with a go.mod the app gets a go.mod of its own,
// so the module around it does not compile its adapters/: `go vet ./...` there passes with an
// adapter that imports "app" (which resolves only in build/<name>). Without the app's go.mod
// the same vet fails, which is why it is written.
func TestNew_InsideModule(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	outer := t.TempDir()
	if err := os.WriteFile(filepath.Join(outer, "go.mod"), []byte("module outer\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "outer.go"), []byte("package outer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(outer, "apps", "guestbook")
	code, out, errs := runNew(t, dir)
	if code != 0 {
		t.Fatalf("alder new: exit %d, %q", code, errs)
	}
	if !strings.Contains(out, "  go.mod\n") || !strings.Contains(out, "go.mod keeps the app out of the Go module of "+filepath.Join(outer, "go.mod")+"\n") {
		t.Errorf("printed:\n%s", out)
	}
	gomod := readFile(t, filepath.Join(dir, "go.mod"))
	if !strings.Contains(gomod, "\nmodule guestbook\n\ngo 1.27\n") || !strings.HasPrefix(gomod, "// This go.mod only keeps the app out of the Go module around it") {
		t.Errorf("go.mod:\n%s", gomod)
	}
	if got := tree(t, dir); !slices.Equal(got, append(slices.Clone(scaffoldFiles[:7]), append([]string{"go.mod"}, scaffoldFiles[7:]...)...)) {
		t.Errorf("files: %v", got)
	}
	adapter := filepath.Join(dir, "adapters", "x", "x.go")
	if err := os.MkdirAll(filepath.Dir(adapter), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adapter, []byte("package x\n\nimport _ \"app\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vet := func() ([]byte, error) {
		cmd := exec.Command("go", "vet", "./...")
		cmd.Dir, cmd.Env = outer, append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
		return cmd.CombinedOutput()
	}
	if out, err := vet(); err != nil {
		t.Errorf("go vet ./... in the outer module: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if out, err := vet(); err == nil {
		t.Errorf("go vet ./... passed without the app's go.mod; the test proves nothing:\n%s", out)
	}
}

// TestNew_OutOfTree: the scaffold made by the built binary, with PATH holding only it and Go,
// checks, tests (two tests), kills every mutant (score 1.00), builds and serves `/` (with the
// design system: /static/al/al.css linked and served with the design-system cache header and an ETag, the button a native one), in both placements:
// outside any Go module and inside one (RF3).
func TestNew_OutOfTree(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	if testing.Short() {
		t.Skip("builds the scaffold twice")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	vars := []string{"GOROOT", "GOMODCACHE", "GOPATH", "GOCACHE", "GOPROXY"}
	goOut, err := exec.Command("go", append([]string{"env"}, vars...)...).Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	goenv := map[string]string{}
	for i, v := range strings.Split(strings.TrimSpace(string(goOut)), "\n") {
		goenv[vars[i]] = v
	}
	tmp := t.TempDir()
	if real, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = real
	}
	if rel, err := filepath.Rel(repo, tmp); err == nil && !strings.HasPrefix(rel, "..") {
		t.Fatalf("the temporary directory %s is inside the checkout %s", tmp, repo)
	}
	bin := filepath.Join(tmp, "bin")
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "alder"), ".")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build of alder: %v\n%s", err, out)
	}
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + bin + string(os.PathListSeparator) + filepath.Join(goenv["GOROOT"], "bin"),
		"HOME=" + home,
		"TMPDIR=" + os.TempDir(),
		"GOMODCACHE=" + goenv["GOMODCACHE"],
		"GOPATH=" + goenv["GOPATH"],
		"GOCACHE=" + goenv["GOCACHE"],
		"GOPROXY=" + goenv["GOPROXY"],
		"GOTOOLCHAIN=local",
	}
	scoreRE := regexp.MustCompile(`(?m)^(\d+) mutants: (\d+) killed, (\d+) survived  \(score (\d\.\d\d)\)$`)

	for _, placement := range []string{"outside", "inside"} {
		t.Run(placement, func(t *testing.T) {
			work := filepath.Join(tmp, placement)
			if err := os.MkdirAll(work, 0o755); err != nil {
				t.Fatal(err)
			}
			if placement == "inside" {
				if err := os.WriteFile(filepath.Join(work, "go.mod"), []byte("module outer\n\ngo 1.27\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			app := filepath.Join(work, "guestbook")
			alder := func(dir string, args ...string) string {
				t.Helper()
				cmd := exec.Command(filepath.Join(bin, "alder"), args...)
				cmd.Dir, cmd.Env = dir, append(env, "PWD="+dir)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("alder %s: %v\n%s", strings.Join(args, " "), err, out)
				}
				return string(out)
			}
			out := alder(work, "new", "guestbook")
			if hasMod := strings.Contains(out, "  go.mod\n"); hasMod != (placement == "inside") {
				t.Errorf("alder new %s a module printed:\n%s", placement, out)
			}
			if out := alder(app, "check"); out != "" {
				t.Errorf("alder check printed %q", out)
			}
			if out := alder(app, "test"); out != "ok   Home \"signing adds an entry\"\nok   Home \"entries are listed newest first\"\n2 passed, 0 failed\n" {
				t.Errorf("alder test printed:\n%s", out)
			}
			out = alder(app, "test", "--mutate")
			m := scoreRE.FindStringSubmatch(out)
			if m == nil {
				t.Fatalf("alder test --mutate printed no score:\n%s", out)
			}
			t.Logf("the scaffold's mutation score: %s", m[0])
			if m[3] != "0" || m[4] != "1.00" { // a first app's tests kill every mutant of it
				t.Errorf("the scaffold's mutants are not all killed:\n%s", out)
			}
			if out := alder(app, "build"); out != "wrote build/guestbook/guestbook\n" {
				t.Errorf("alder build printed %q", out)
			}
			r := startAppNamed(t, app, "guestbook", "ALDER_DEV=1", "ALDER_SECRET=")
			c := &http.Client{Timeout: 10 * time.Second}
			res, err := c.Get("http://" + r.addr + "/")
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "<h1>Guestbook</h1>") || !strings.Contains(string(body), `<ul id="entries">`) {
				t.Fatalf("GET / = %d:\n%s", res.StatusCode, body)
			}
			if i, j := strings.Index(string(body), scaffold.ThemeScript), strings.Index(string(body), `<link rel="stylesheet" href="/static/al/al.css" />`); i < 0 || j < 0 || i > j {
				t.Errorf("the theme script at %d, the al.css link at %d:\n%s", i, j, body)
			}
			// the design system: linked, used, and its button a real submit button
			for _, want := range []string{`<link rel="stylesheet" href="/static/al/al.css" />`, `<al-page width="narrow">`, "<al-card>",
				`<al-input label="Your name"><label>Your name<input name="name"`, `<al-button type="submit"><button type="submit">Sign</button></al-button>`} {
				if !strings.Contains(string(body), want) {
					t.Errorf("GET / lacks %s:\n%s", want, body)
				}
			}
			if res, err = c.Get("http://" + r.addr + "/static/al/al.css"); err != nil {
				t.Fatal(err)
			}
			css, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "public, max-age=300, stale-while-revalidate=86400" || res.Header.Get("ETag") == "" || !strings.Contains(string(css), "al-button") {
				t.Errorf("GET /static/al/al.css = %d %q %q (%d bytes)", res.StatusCode, res.Header.Get("Cache-Control"), res.Header.Get("ETag"), len(css))
			}
			if code := r.stop(t, os.Interrupt); code != 0 {
				t.Fatalf("after SIGINT: exit %d; stderr:\n%s", code, r.stderr.String())
			}
		})
	}
}

// TestNew_Bare: `alder new <name> --bare` writes the scaffold without the guestbook (App,
// Layout, the stylesheet, adapters/, .gitignore: no page, no table, no migration), and the
// sequence a task like T5 takes from it — the tables, the login, then its page at `/`, which
// requires the user — reaches `page "/"` without a second page there (E0016), and checks and
// passes its tests once the holes are filled.
func TestNew_Bare(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := filepath.Join(outsideModule(t), "notes")
	if code, out, errs := runNew(t, dir, "--bare"); code != 0 || !strings.Contains(out, "created "+dir+"/\n") {
		t.Fatalf("alder new --bare: exit %d\n%s%s", code, out, errs)
	}
	got := slices.Sorted(maps.Keys(appFiles(t, dir)))
	if want := []string{".gitignore", "adapters/README.md", "app/App.alder", "app/Layout.alder", "scratch.alder", "static/app.css"}; !slices.Equal(got, want) {
		t.Errorf("files %v, want %v", got, want)
	}
	if code, _, errs := runNew(t, dir, "--bare"); code != 1 || !strings.Contains(errs, "exists and is not empty") {
		t.Errorf("again: exit %d, %s", code, errs)
	}
	for _, st := range []struct {
		stdin string
		args  []string
	}{
		{"", []string{"new", "table", "User", "email:Text.Email@unique", "name:Text.NonEmpty", "passwordHash:Text", "created:Time@created", "--no-timestamps"}},
		{"", []string{"new", "table", "Note", "title:Text.NonEmpty", "body:Text", "owner:User.UserId@ref=User", "created:Time@created", "--no-timestamps"}},
		{"", []string{"new", "auth"}},
		{"", []string{"new", "page", "Notes.Index", "/", "--blank", "--requires", "user"}},
		{"Notes.Index {}", []string{"fill", "Auth.Login.update.Submit", "-"}},
		{"Notes.Index {}", []string{"fill", `Auth.Login."logging in through the form"`, "-"}},
		{"Query.succeed { }", []string{"fill", "Notes.Index.load", "-"}},
		{`<Layout.Page title="Notes"><h1>Notes</h1></Layout.Page>`, []string{"fill", "Notes.Index.view", "-"}},
	} {
		if code, out, errs := alderIn(t, dir, st.stdin, st.args...); code != 0 {
			t.Fatalf("alder %s: exit %d\n%s%s", strings.Join(st.args, " "), code, out, errs)
		}
	}
	if code, out, errs := alderIn(t, dir, "", "check"); code != 0 || out != "" {
		t.Fatalf("alder check: exit %d\n%s%s", code, out, errs)
	}
	if code, out, errs := alderIn(t, dir, "", "test"); code != 0 || !strings.HasSuffix(out, " passed, 0 failed\n") {
		t.Fatalf("alder test: exit %d\n%s%s", code, out, errs)
	}
	app, _ := os.ReadFile(filepath.Join(dir, "app", "App.alder"))
	if !strings.Contains(string(app), "session { user: Maybe User.UserId }\n\nlogin = Auth.Login\n\nmsg Logout\n") {
		t.Errorf("App:\n%s", app)
	}
}

// TestNewScaffoldsScratch: both scaffolds write scratch.alder at the project root, and
// `alder scratch` on a fresh app prints the starter watch's value.
func TestNewScaffoldsScratch(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	has := func(fs []scaffold.File) bool {
		for _, f := range fs {
			if f.Path == "scratch.alder" {
				return true
			}
		}
		return false
	}
	if !has(scaffold.Files("x", false)) || !has(scaffold.BareFiles("x", false)) {
		t.Fatal("scratch.alder is missing from Files or BareFiles")
	}
	dir := filepath.Join(outsideModule(t), "sc")
	if code, out, errs := runNew(t, dir); code != 0 {
		t.Fatalf("alder new: exit %d\n%s%s", code, out, errs)
	}
	if !slices.Contains(tree(t, dir), "scratch.alder") {
		t.Errorf("alder new wrote no scratch.alder: %v", tree(t, dir))
	}
	var so, se bytes.Buffer
	code := runScratchTo(withBuildDir(t, []string{filepath.Join(dir, "scratch.alder")}), &so, &se)
	out, errs := so.String(), se.String()
	if code != 0 || !strings.Contains(out, "⇒ 2 : Int") {
		t.Errorf("alder scratch: exit %d\n%s%s", code, out, errs)
	}
}

// TestNewAppScaffoldTable (#20): the scaffold's table is app/Table/Entry.alder, there is no
// app/Entry.alder, and the new app checks.
func TestNewAppScaffoldTable(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(outsideModule(t), "demo")
	if code, _, errs := runNew(t, dir); code != 0 {
		t.Fatalf("alder new: exit %d, %q", code, errs)
	}
	if !strings.HasPrefix(readFile(t, filepath.Join(dir, "app", "Table", "Entry.alder")), "module Table.Entry\n") {
		t.Errorf("app/Table/Entry.alder is not module Table.Entry")
	}
	if _, err := os.Stat(filepath.Join(dir, "app", "Entry.alder")); err == nil {
		t.Errorf("app/Entry.alder exists")
	}
	if code, out, errs := run(t, dir); code != 0 || out != "" || errs != "" {
		t.Errorf("alder check: exit %d, %q %q", code, out, errs)
	}
}
