package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/deploy"
	"alder/internal/version"
)

func runDeployInit(args ...string) (int, string, string) {
	return runDeployInitSrc(version.SourceOf, args...)
}

func runDeployInitSrc(source func() version.Source, args ...string) (int, string, string) {
	var out, errb strings.Builder
	code := runDeployInitWith(args, &out, &errb, source)
	return code, out.String(), errb.String()
}

const staticHome = `module Home

import Layout

page "/" { }

load : Params -> Session -> Query Text
load params session = Query.succeed "hi"

view : Text -> Session -> Html Never
view model session = <Layout.Page title="Hi"><h1>{model}</h1></Layout.Page>
`

// noTables is the scaffold with its one table (and the page that uses it) replaced by a static page.
func noTables(t *testing.T) string {
	t.Helper()
	dir := deployScaffold(t, "", map[string]string{"app/Home.alder": staticHome})
	if err := os.Remove(filepath.Join(dir, "app", "Entry.alder")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// An app without tables: the three Render files, named after the directory, no disk; the
// output says what was written and the next step.
func TestDeployInit_Render(t *testing.T) {
	// serial: inTemp (t.Chdir) via deployScaffold/noTables; deploy scaffolds use relative paths (Task 3)
	dir := noTables(t)
	code, out, errs := runDeployInit("render", dir)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	for _, f := range []string{"Dockerfile", ".dockerignore", "render.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s not written", f)
		}
		if !strings.Contains(out, "wrote "+f) {
			t.Errorf("output lacks %q:\n%s", "wrote "+f, out)
		}
	}
	if !strings.Contains(out, "connect the repo in Render") {
		t.Errorf("no next step:\n%s", out)
	}
	y := readFile(t, filepath.Join(dir, "render.yaml"))
	if !strings.Contains(y, "name: demo") || strings.Contains(y, "disk:") {
		t.Errorf("render.yaml:\n%s", y)
	}
	if d := readFile(t, filepath.Join(dir, "Dockerfile")); !strings.Contains(d, "ARG ALDER_REF=") || !strings.Contains(d, `ENTRYPOINT ["/app/demo"]`) {
		t.Errorf("Dockerfile:\n%s", d)
	}
}

// An app with a table (the scaffold's) gets the volume.
func TestDeployInit_Fly_Volume(t *testing.T) {
	// serial: inTemp (t.Chdir) via deployScaffold/noTables; deploy scaffolds use relative paths (Task 3)
	dir := deployScaffold(t, "", nil)
	code, out, errs := runDeployInit("fly", dir)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	toml := readFile(t, filepath.Join(dir, "fly.toml"))
	if !strings.Contains(toml, "[mounts]") || !strings.Contains(toml, `app = "demo"`) {
		t.Errorf("fly.toml:\n%s", toml)
	}
	if _, err := os.Stat(filepath.Join(dir, "render.yaml")); err == nil {
		t.Error("render.yaml written for fly")
	}
	if !strings.Contains(out, "fly deploy") {
		t.Errorf("no next step:\n%s", out)
	}
}

// The overwrite rule: an existing file is never replaced without --force, nothing is written,
// the skipped files are listed and the exit is 1; --force replaces.
func TestDeployInit_Overwrite(t *testing.T) {
	// serial: inTemp (t.Chdir) via deployScaffold/noTables; deploy scaffolds use relative paths (Task 3)
	dir := deployScaffold(t, "", nil)
	writeFile(t, filepath.Join(dir, "Dockerfile"), "mine\n")
	code, out, errs := runDeployInit("render", dir)
	if code != 1 || !strings.Contains(errs, "Dockerfile") || !strings.Contains(errs, "--force") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	if readFile(t, filepath.Join(dir, "Dockerfile")) != "mine\n" {
		t.Error("Dockerfile overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "render.yaml")); err == nil {
		t.Error("a file was written although another was skipped")
	}
	if code, out, errs := runDeployInit("render", dir, "--force"); code != 0 {
		t.Fatalf("--force: exit %d\n%s%s", code, out, errs)
	}
	if readFile(t, filepath.Join(dir, "Dockerfile")) == "mine\n" {
		t.Error("--force did not replace")
	}
}

func TestDeployInit_DryRun(t *testing.T) {
	// serial: inTemp (t.Chdir) via deployScaffold/noTables; deploy scaffolds use relative paths (Task 3)
	dir := deployScaffold(t, "", nil)
	code, out, errs := runDeployInit("fly", dir, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	for _, want := range []string{"Dockerfile", "fly.toml", "FROM golang"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run lacks %q:\n%s", want, out)
		}
	}
	for _, f := range []string{"Dockerfile", "fly.toml", ".dockerignore"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("dry run wrote %s", f)
		}
	}
}

func TestDeployInit_Usage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{}, {"heroku"}, {"render", "a", "b"}, {"render", "--nope"}} {
		if code, _, errs := runDeployInit(args...); code != 2 || !strings.Contains(errs, deployInitUsage) {
			t.Errorf("%v: exit %d %q", args, code, errs)
		}
	}
}

// A silo-only app (no shared tables) still keeps SQLite files under ALDER_DB's directory.
func TestDeployInit_NeedsDisk(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		tables, silos int
		want          bool
	}{{0, 0, false}, {1, 0, true}, {0, 2, true}} {
		if got := initNeedsDisk(c.tables, c.silos); got != c.want {
			t.Errorf("initNeedsDisk(%d, %d) = %v", c.tables, c.silos, got)
		}
	}
}

// The pin: a release tag is written as is; a clean commit is written with a note to push it; a
// modified checkout has no ref, so the placeholder is written and the output warns.
func TestDeployInit_Pin(t *testing.T) {
	// serial: inTemp (t.Chdir) via deployScaffold/noTables; deploy scaffolds use relative paths (Task 3)
	for _, c := range []struct {
		src        version.Source
		inDocker   string
		inOut      string
		notInOut   string
		wantDocker bool
	}{
		{version.Source{Ref: "v0.3.0", Release: true}, "ARG ALDER_REF=v0.3.0", "pinned to v0.3.0", "is pushed", true},
		{version.Source{Ref: "abc123"}, "ARG ALDER_REF=abc123", "make sure abc123 is pushed to the alder remote", "", true},
		{version.Source{Dirty: true}, "ARG ALDER_REF=" + deploy.InitRefPlaceholder, "uncommitted changes", "is pushed", true},
	} {
		src := c.src
		dir := noTables(t)
		code, out, errs := runDeployInitSrc(func() version.Source { return src }, "render", dir)
		if code != 0 {
			t.Fatalf("%+v: exit %d\n%s%s", c.src, code, out, errs)
		}
		if d := readFile(t, filepath.Join(dir, "Dockerfile")); !strings.Contains(d, c.inDocker) {
			t.Errorf("%+v: Dockerfile lacks %q", c.src, c.inDocker)
		}
		if !strings.Contains(out, c.inOut) || (c.notInOut != "" && strings.Contains(out, c.notInOut)) {
			t.Errorf("%+v: output:\n%s", c.src, out)
		}
	}
}

// `alder deploy init` is reached through `alder deploy`.
func TestDeployInit_Dispatch(t *testing.T) {
	t.Parallel()
	var out, errb strings.Builder
	if code := runDeployTo([]string{"init", "heroku"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), deployInitUsage) {
		t.Errorf("exit %d %q", code, errb.String())
	}
}
