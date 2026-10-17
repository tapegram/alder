package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/deploy"
)

// The tests of `alder deploy` that need no SSH: the gate's refusals, the artifact, --dry-run
// and the Script target end to end on the scaffold. The Ssh target's tests are in
// deploy_ssh_test.go (a local sshd: Docker under -tags deploy, or ALDER_DEPLOY_LOCAL=1).

func runDeploy(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runDeployTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

// recordScript is a Script target that records its arguments and ALDER_REQUIRES next to the
// artifact (in <artifact>.recorded), and whether a planted variable reached it.
const recordScript = `#!/bin/sh
set -eu
{
  echo "args: $*"
  echo "ALDER_REQUIRES=${ALDER_REQUIRES-unset}"
  echo "cwd: $(pwd)"
} > "$1.recorded"
`

// deployScaffold makes a fresh `alder new demo` app in a new temporary working directory,
// appends decl (a `deploy` declaration) to its App.alder and extra files (path → content), adds
// scripts/record.sh (recordScript), and returns "demo".
func deployScaffold(t *testing.T, decl string, extra map[string]string) string {
	t.Helper()
	needGoTool(t)
	inTemp(t)
	if code, _, errs := runNew(t, "demo"); code != 0 {
		t.Fatalf("alder new: %d %s", code, errs)
	}
	appendTo(t, "demo/app/App.alder", "\n"+decl+"\n")
	for p, c := range extra {
		writeFile(t, filepath.Join("demo", p), c)
	}
	writeFile(t, "demo/scripts/record.sh", recordScript)
	if err := os.Chmod("demo/scripts/record.sh", 0o755); err != nil {
		t.Fatal(err)
	}
	return "demo"
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(b)+s)
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// artifacts are the artifact directories under build/demo/deploy.
func artifacts(t *testing.T) []string {
	t.Helper()
	ds, _ := filepath.Glob("build/demo/deploy/*")
	var out []string
	for _, d := range ds {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

const scriptDecl = `deploy {
  local: Script "./scripts/record.sh",
  requires: ["SMTP_URL", "STRIPE_KEY"],
}`

// The Script target end to end on the scaffold: the gate's lines, the artifact (the executable
// whose hash is the build id, static/, migrations/, alder-deploy.json with the gate's results,
// the plan and the requires names), and the script run from the app's root with the artifact
// directory and the environment's name, the names in ALDER_REQUIRES. A value set in the
// environment appears nowhere: not in the output, not in any file of the artifact.
func TestDeploy_ScriptScaffold(t *testing.T) {
	dir := deployScaffold(t, scriptDecl, nil)
	const planted = "planted-secret-9f8e7d6c5b4a"
	t.Setenv("SMTP_URL", "smtp://user:"+planted+"@mail.example.com")
	t.Setenv("STRIPE_KEY", planted)
	code, out, errs := runDeploy(t, "local", dir)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	for _, want := range []string{"alder deploy local → Script ./scripts/record.sh\n", "gate\n", "  check    ok\n", "  build    ok\n",
		"  test     ok  2 passed\n", "  mutate   ok  score 1.00 ≥ floor 0.00 (none yet; 3 mutants, 0 survived)\n",
		"plan\n", "  0001_initial\n", "    create table Entry\n", "script ./scripts/record.sh build/demo/deploy/"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	arts := artifacts(t)
	if len(arts) != 1 {
		t.Fatalf("artifacts: %v", arts)
	}
	art := arts[0]
	id := filepath.Base(art)
	if !strings.Contains(out, "deployed "+id+" to local\n") {
		t.Errorf("output lacks the deployed line for %s:\n%s", id, out)
	}
	exe, err := os.ReadFile(filepath.Join(art, "app"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(exe)
	if got := hex.EncodeToString(sum[:])[:16]; got != id {
		t.Errorf("the artifact is named %s but its executable's build id is %s", id, got)
	}
	for _, f := range []string{"static/app.css", "migrations/0001_initial.sql", "migrations/schema.json", "alder-deploy.json"} {
		if _, err := os.Stat(filepath.Join(art, f)); err != nil {
			t.Errorf("the artifact lacks %s: %v", f, err)
		}
	}
	var rec deploy.Record
	b, _ := os.ReadFile(filepath.Join(art, "alder-deploy.json"))
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Build != id || rec.App != "demo" || rec.Env != "local" || rec.Target != "Script ./scripts/record.sh" ||
		rec.Gate.Check != "ok" || rec.Gate.Build != "ok" || rec.Gate.Tests == nil || rec.Gate.Tests.Passed != 2 ||
		rec.Gate.Mutate == nil || rec.Gate.Mutate.Score != 1 || strings.Join(rec.Requires, ",") != "SMTP_URL,STRIPE_KEY" ||
		len(rec.Plan.Migrations) != 1 || rec.Plan.Migrations[0].Name != "0001_initial" || rec.Version == "" {
		t.Errorf("alder-deploy.json:\n%s", b)
	}
	recorded, err := os.ReadFile(art + ".recorded")
	if err != nil {
		t.Fatalf("the script did not run: %v", err)
	}
	abs, _ := filepath.Abs(art)
	appRoot, _ := filepath.Abs(dir)
	abs, appRoot = evalSymlinks(abs), evalSymlinks(appRoot)
	want := "args: " + abs + " local\nALDER_REQUIRES=SMTP_URL,STRIPE_KEY\ncwd: " + appRoot + "\n"
	if got := strings.ReplaceAll(string(recorded), "/private/var/", "/var/"); got != strings.ReplaceAll(want, "/private/var/", "/var/") {
		t.Errorf("the script recorded\n%s\nwant\n%s", recorded, want)
	}
	if strings.Contains(out+errs, planted) {
		t.Errorf("the planted value is in the output")
	}
	filepath.WalkDir(art, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte(planted)) {
				t.Errorf("the planted value is in %s", p)
			}
		}
		return nil
	})

	// --json: the record on stdout, the progress on stderr
	code, out, errs = runDeploy(t, "local", dir, "--json")
	if code != 0 {
		t.Fatalf("--json: exit %d\n%s", code, errs)
	}
	var again deploy.Record
	if err := json.Unmarshal([]byte(out), &again); err != nil || again.Build != id {
		t.Errorf("--json printed %q (%v)", out, err)
	}
	if !strings.Contains(errs, "deployed "+id) {
		t.Errorf("--json: the progress is not on stderr:\n%s", errs)
	}
}

func evalSymlinks(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// --dry-run runs the gate, writes the artifact and prints the plan, and ships nothing: the
// script does not run.
func TestDeploy_DryRun(t *testing.T) {
	dir := deployScaffold(t, scriptDecl, nil)
	code, out, errs := runDeploy(t, "local", dir, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	if !strings.Contains(out, "plan\n  the target's schema is not known") || !strings.Contains(out, "dry run: nothing was shipped\n") || strings.Contains(out, "deployed ") {
		t.Errorf("output:\n%s", out)
	}
	arts := artifacts(t)
	if len(arts) != 1 {
		t.Fatalf("artifacts: %v", arts)
	}
	if _, err := os.Stat(arts[0] + ".recorded"); err == nil {
		t.Error("the script ran on a dry run")
	}
	if _, err := os.Stat(filepath.Join(arts[0], "alder-deploy.json")); err != nil {
		t.Error("a dry run's artifact lacks alder-deploy.json")
	}
}

// The gate's refusals: a mutation score below the declared floor (naming the survivors' count),
// a declared check that exits non-zero (with its last lines), a typed hole, a check error. Each
// exits 1, ships nothing and writes no artifact.
func TestDeploy_GateRefusals(t *testing.T) {
	weak := `module Calc

clamp : Int -> Int
clamp n = if n > 10 then 10 else n

test "clamp keeps a small number" = clamp 3 == 3
`
	for _, c := range []struct {
		name, decl string
		extra      map[string]string
		want       []string
	}{
		{"floor", `deploy { local: Script "./scripts/record.sh", floor: 0.95 }`, map[string]string{"app/Calc.alder": weak},
			[]string{"  mutate   FAIL  score 0.", "< floor 0.95 (declared)", "alder deploy: refused: the mutation score 0.", "is below the floor 0.95 (declared): ", "survived; `alder test --mutate` lists them"}},
		{"check", `deploy { local: Script "./scripts/record.sh", checks: ["./scripts/lint.sh"] }`, map[string]string{"scripts/lint.sh": "#!/bin/sh\necho checking content\necho 'post 3 has no title' >&2\nexit 3\n"},
			[]string{"  check    FAIL  ./scripts/lint.sh", "alder deploy: refused: the check `./scripts/lint.sh` exited 3\nchecking content\npost 3 has no title\n"}},
		{"hole", `deploy { local: Script "./scripts/record.sh" }`, map[string]string{"app/Calc.alder": "module Calc\n\nanswer : Int\nanswer = ?todo\n"},
			[]string{"  check    FAIL  1 typed hole", "alder deploy: refused: the program has 1 typed hole left: fill them before deploying", "W0002"}},
		{"error", `deploy { local: Script "./scripts/record.sh" }`, map[string]string{"app/Calc.alder": "module Calc\n\nanswer : Int\nanswer = \"no\"\n"},
			[]string{"  check    FAIL  1 error", "alder deploy: refused: `alder check` found 1 error: fix them before deploying"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := deployScaffold(t, c.decl, c.extra)
			if c.name == "check" {
				os.Chmod("demo/scripts/lint.sh", 0o755)
			}
			code, out, errs := runDeploy(t, "local", dir)
			if code != 1 {
				t.Fatalf("exit %d, want 1\n%s\n%s", code, out, errs)
			}
			for _, w := range c.want {
				if !strings.Contains(out+errs, w) {
					t.Errorf("output lacks %q:\n%s\n%s", w, out, errs)
				}
			}
			if arts := artifacts(t); len(arts) != 0 {
				t.Errorf("a refused gate wrote artifacts %v", arts)
			}
		})
	}
}

// --unsafe-skip-tests skips the tests, the score and the checks, and the record says so.
func TestDeploy_UnsafeSkipTests(t *testing.T) {
	dir := deployScaffold(t, `deploy { local: Script "./scripts/record.sh", floor: 1, checks: ["exit 1"] }`,
		map[string]string{"app/Calc.alder": "module Calc\n\ntest \"wrong\" = 1 == 2\n"})
	code, out, errs := runDeploy(t, "local", dir, "--unsafe-skip-tests", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	var rec deploy.Record
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatal(err)
	}
	if !rec.UnsafeSkipTests || !rec.Gate.Skipped || rec.Gate.Tests != nil || rec.Gate.Mutate != nil {
		t.Errorf("record: %s", out)
	}
	if !strings.Contains(errs, "  test     SKIPPED  --unsafe-skip-tests") {
		t.Errorf("progress:\n%s", errs)
	}
}

// The usage errors and the refusals that come before the gate: exit 2.
func TestDeploy_Usage(t *testing.T) {
	for _, args := range [][]string{{}, {"a", "b", "c"}, {"local", "--bogus"}} {
		if code, _, errs := runDeploy(t, args...); code != 2 || !strings.Contains(errs, "usage: alder deploy") {
			t.Errorf("%v: exit %d %s", args, code, errs)
		}
	}
	dir := deployScaffold(t, scriptDecl, nil)
	if code, _, errs := runDeploy(t, "production", dir); code != 2 || !strings.Contains(errs, "App's `deploy` has no environment `production` (it declares local)") {
		t.Errorf("unknown env: exit %d %s", code, errs)
	}
	for _, f := range []func([]string, *bytes.Buffer, *bytes.Buffer) int{
		func(a []string, o, e *bytes.Buffer) int { return runStatusTo(a, o, e) },
		func(a []string, o, e *bytes.Buffer) int { return runLogsTo(a, o, e) },
		func(a []string, o, e *bytes.Buffer) int { return runRollbackTo(append(a[:1], "--dir", a[1]), o, e) },
	} {
		var o, e bytes.Buffer
		if code := f([]string{"local", dir}, &o, &e); code != 2 || !strings.Contains(e.String(), "is not available for a Script target") {
			t.Errorf("exit %d %s", code, e.String())
		}
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"local", "--dir", dir}, "the declaration has no `backup` location: pass --from"},
		{[]string{"local", "--dir", dir, "--from", "/tmp/b"}, "is not available for a Script target: environment local is deployed by its script, which keeps its own state; write the database to a file with --out"},
		{[]string{"local", "--dir", dir, "--from", "/tmp/b", "--to", "yesterday"}, `bad --to "yesterday": write a UTC time such as 2026-10-17T10:00:00Z`},
		{[]string{"local", "x", "--dir", dir}, "usage: alder restore"},
	} {
		var o, e bytes.Buffer
		if code := runRestoreTo(c.args, &o, &e); code != 2 || !strings.Contains(e.String(), c.want) {
			t.Errorf("restore %v: exit %d %s", c.args, code, e.String())
		}
	}
	inTemp(t)
	if code, _, errs := runNew(t, "bare"); code != 0 {
		t.Fatal(errs)
	}
	if code, _, errs := runDeploy(t, "local", "bare"); code != 2 || !strings.Contains(errs, "App has no `deploy` declaration") {
		t.Errorf("no declaration: exit %d %s", code, errs)
	}
}
