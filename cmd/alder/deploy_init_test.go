package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runDeployInit(args ...string) (int, string, string) {
	var out, errb strings.Builder
	code := runDeployInitTo(args, &out, &errb)
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
	for _, args := range [][]string{{}, {"heroku"}, {"render", "a", "b"}, {"render", "--nope"}} {
		if code, _, errs := runDeployInit(args...); code != 2 || !strings.Contains(errs, deployInitUsage) {
			t.Errorf("%v: exit %d %q", args, code, errs)
		}
	}
}

// `alder deploy init` is reached through `alder deploy`.
func TestDeployInit_Dispatch(t *testing.T) {
	var out, errb strings.Builder
	if code := runDeployTo([]string{"init", "heroku"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), deployInitUsage) {
		t.Errorf("exit %d %q", code, errb.String())
	}
}
