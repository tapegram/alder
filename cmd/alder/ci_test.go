package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"alder/internal/ir"
)

// The workflows `alder ci init` writes, by declaration: golden files under testdata/ci (go test
// -run Ci -update rewrites them), linted with actionlint when it is on PATH.
func TestCi_Golden(t *testing.T) {
	ssh := func(domain, path string) ir.Target {
		return ir.Target{Kind: "ssh", Host: "deploy@box.example.com", Domain: domain, Path: path}
	}
	for _, c := range []struct {
		name    string
		decl    *ir.Deploy
		version string
	}{
		// the declaration of chapter 16 (and the spec): production and preview on one box,
		// staging on Fly (not deployed by the workflow), a floor
		{"full", &ir.Deploy{Envs: map[string]ir.Target{
			"production": ssh("blog.example.com", "/srv/blog"),
			"staging":    {Kind: "fly", Region: "ord", App: "blog-staging"},
			"preview":    ssh("*.preview.example.com", "/srv/blog-preview"),
		}, Floor: 0.95, FloorSet: true, Checks: []string{"./scripts/lint-content"}, Requires: []string{"SMTP_URL", "STRIPE_KEY"}}, "v0.1.0"},
		// Fly for both: the values are repository secrets, set as Fly secrets at deploy
		{"fly", &ir.Deploy{Envs: map[string]ir.Target{
			"production": {Kind: "fly", Region: "ord", App: "blog"},
			"preview":    {Kind: "fly", Region: "ord", App: "blog-preview"},
		}, Requires: []string{"SMTP_URL"}}, "v0.2.1"},
		// the examples' declaration: a Script environment named local, so nothing is deployed;
		// written by a development build, which installs the latest release
		{"script", &ir.Deploy{Envs: map[string]ir.Target{"local": {Kind: "script", ScriptPath: "./scripts/deploy-local.sh"}}}, "devel+71c46e8abcde"},
		// Render for both: no SSH step, the registry token, the Turso and Render secrets
		{"render-only", &ir.Deploy{Envs: map[string]ir.Target{
			"production": {Kind: "render", Region: "oregon", Service: "blog", Plan: "starter", Instances: 2, Database: &ir.Database{Kind: "turso", Org: "acme", App: "blog"}},
			"preview":    {Kind: "render", Region: "oregon", Service: "blog-preview", Database: &ir.Database{Kind: "turso", Org: "acme", App: "blog-preview"}},
		}, Image: "ghcr.io/tavish/blog", Requires: []string{"SMTP_URL"}}, "v0.3.0"},
		// production at Render, the previews on the box: the SSH step only where it is needed
		{"mixed", &ir.Deploy{Envs: map[string]ir.Target{
			"production": {Kind: "render", Region: "oregon", Service: "blog", Database: &ir.Database{Kind: "turso", Org: "acme", App: "blog"}},
			"preview":    ssh("*.preview.example.com", "/srv/blog-preview"),
		}, Image: "ghcr.io/tavish/blog", Floor: 0.9, FloorSet: true, Requires: []string{"SMTP_URL"}}, "v0.3.0"},
		{"none", nil, "v0.1.0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			wf := ciWorkflow(c.decl, c.version, "1.27")
			golden := filepath.Join("testdata", "ci", c.name+".yml")
			if *updateGenerate {
				os.MkdirAll(filepath.Dir(golden), 0o755)
				if err := os.WriteFile(golden, []byte(wf.YAML), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (go test -run Ci -update writes it)", err)
			}
			if wf.YAML != string(want) {
				t.Errorf("%s differs (go test -run Ci -update rewrites it):\n%s", golden, firstDiffLines(string(want), wf.YAML))
			}
			if lint, err := exec.LookPath("actionlint"); err == nil {
				if out, err := exec.Command(lint, golden).CombinedOutput(); err != nil {
					t.Errorf("actionlint %s: %v\n%s", golden, err, out)
				}
			}
		})
	}
}

// firstDiffLines is the first line where want and got differ.
func firstDiffLines(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			return "line " + strconv.Itoa(i+1) + ":\n  want " + a + "\n  got  " + b
		}
	}
	return ""
}

// RF5: the workflow deploys preview on a pull request with the pull request's number in the
// host and comments the address (editing its last comment on a later push), from the
// repository only (a fork's pull request has no secrets); production on a push to main; the
// secrets it needs are named in its header, and no value is in it.
func TestCi_PreviewWorkflow(t *testing.T) {
	wf := ciWorkflow(&ir.Deploy{Envs: map[string]ir.Target{
		"production": {Kind: "ssh", Host: "deploy@box.example.com", Domain: "blog.example.com", Path: "/srv/blog"},
		"preview":    {Kind: "ssh", Host: "deploy@box.example.com", Domain: "*.preview.example.com", Path: "/srv/blog-preview"},
	}, Requires: []string{"SMTP_URL"}, Floor: 0.9, FloorSet: true}, "v0.1.0", "1.27")
	for _, want := range []string{
		"  pull_request:\n",
		"    if: github.event_name == 'pull_request' && github.event.pull_request.head.repo.full_name == github.repository\n",
		"      HOST: ${{ github.event.pull_request.number }}.preview.example.com\n",
		"        run: alder deploy preview --host \"$HOST\"\n",
		"gh pr comment \"$PR\" --edit-last --body \"$body\" || gh pr comment \"$PR\" --body \"$body\"\n",
		"    if: github.event_name == 'push' && github.ref == 'refs/heads/main'\n",
		"        run: alder deploy production\n",
		"      - run: alder test --mutate --min 0.9\n",
		"#   SSH_PRIVATE_KEY  a private key deploy@box.example.com accepts (production, preview)\n",
		"# A preview stays up after its pull request is closed",
	} {
		if !strings.Contains(wf.YAML, want) {
			t.Errorf("the workflow lacks %q:\n%s", want, wf.YAML)
		}
	}
	if strings.Contains(wf.YAML, "secrets.SMTP_URL") {
		t.Errorf("an Ssh environment's requires value is read at the box, not from a repository secret:\n%s", wf.YAML)
	}
}

// `alder ci init` on an app: the file written, its summary and secrets printed; a second run
// refuses to replace it, --force does; a usage error.
func TestCi_Init(t *testing.T) {
	dir := deployScaffold(t, `deploy {
  production: Fly "ord" { app: "demo-prod" },
  preview: Script "./scripts/record.sh",
  requires: ["SMTP_URL"],
  floor: 0.8,
}`, nil)
	t.Setenv("PATH", t.TempDir()+string(os.PathListSeparator)+os.Getenv("PATH")) // (actionlint, if any, still found)
	var out, errb strings.Builder
	code := runCiTo([]string{"init", dir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errb.String())
	}
	for _, want := range []string{
		"wrote .github/workflows/alder.yml\n",
		"  pull request  alder check, alder test --mutate --min 0.8, alder deploy preview --host pr-<number>\n",
		"  push to main  alder check, alder test --mutate --min 0.8, alder deploy production\n",
		"repository secrets it needs: FLY_API_TOKEN, SMTP_URL, ALDER_SECRET (the file's header says what each is)\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", "alder.yml"))
	if err != nil || !strings.Contains(string(b), "SMTP_URL: ${{ secrets.SMTP_URL }}") || !strings.Contains(string(b), "superfly/flyctl-actions/setup-flyctl") {
		t.Errorf("the file: %v\n%s", err, b)
	}
	out.Reset()
	errb.Reset()
	if code := runCiTo([]string{"init", dir}, &out, &errb); code != 2 || !strings.Contains(errb.String(), ".github/workflows/alder.yml exists: remove it, or pass --force") {
		t.Errorf("second run: exit %d %s", code, errb.String())
	}
	if code := runCiTo([]string{"init", dir, "--force"}, &out, &errb); code != 0 {
		t.Errorf("--force: exit %d %s", code, errb.String())
	}
	errb.Reset()
	if code := runCiTo([]string{"start"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), ciUsage) {
		t.Errorf("usage: exit %d %s", code, errb.String())
	}
}

// RF5, the command's side: `alder deploy preview --host <pr>.<domain>` serves the pull
// request's host; a Script dry run names it, a Script run gets it in ALDER_HOST; a host that
// does not match the wildcard domain, and --host for a Fly environment, are refused.
func TestDeploy_Host(t *testing.T) {
	dir := deployScaffold(t, `deploy {
  preview: Script "./scripts/host.sh",
  requires: ["SMTP_URL"],
}`, map[string]string{"scripts/host.sh": "#!/bin/sh\necho \"ALDER_HOST=$ALDER_HOST\" > \"$1.host\"\n"})
	os.Chmod(filepath.Join(dir, "scripts", "host.sh"), 0o755)
	code, out, errs := runDeploy(t, "preview", dir, "--host", "42.preview.example.com", "--dry-run")
	if code != 0 || !strings.Contains(out, "alder deploy preview → Script ./scripts/host.sh\nhost 42.preview.example.com (--host)\ngate\n") || !strings.Contains(out, "dry run: nothing was shipped") {
		t.Fatalf("dry run: exit %d\n%s%s", code, out, errs)
	}
	code, out, errs = runDeploy(t, "preview", dir, "--host=43.preview.example.com", "--unsafe-skip-tests")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	hosts, _ := filepath.Glob("build/demo/deploy/*.host")
	if len(hosts) != 1 {
		t.Fatalf("the script ran %d times", len(hosts))
	}
	if b, _ := os.ReadFile(hosts[0]); string(b) != "ALDER_HOST=43.preview.example.com\n" {
		t.Errorf("the script got %q", b)
	}
	if v, ok := os.LookupEnv("ALDER_HOST"); ok {
		t.Errorf("ALDER_HOST left set: %q", v)
	}

	ssh := ir.Target{Kind: "ssh", Host: "deploy@box", Domain: "*.preview.example.com", Path: "/srv/blog-preview/"}
	got, err := withHost("preview", ssh, "42.preview.example.com")
	if err != nil || got.Domain != "42.preview.example.com" || got.Path != "/srv/blog-preview/42.preview.example.com" || targetEnv("preview", "42.preview.example.com") != "preview-42.preview.example.com" {
		t.Errorf("withHost: %+v %v", got, err)
	}
	for _, c := range []struct {
		t    ir.Target
		host string
		want string
	}{
		{ssh, "42.other.example.com", "--host 42.other.example.com does not match environment preview's domain *.preview.example.com"},
		{ssh, "a.b.preview.example.com", "does not match"},
		{ssh, "../x", "bad --host"},
		{ir.Target{Kind: "ssh", Host: "deploy@box", Domain: "preview.example.com", Path: "/srv/p"}, "x$(id).preview.example.com", "bad --host"},
		{ir.Target{Kind: "ssh", Host: "deploy@box", Domain: "preview.example.com", Path: "/srv/p"}, "-o.example.com", "bad --host"},
		{ir.Target{Kind: "ssh", Host: "deploy@box", Domain: "preview.example.com", Path: "/srv/p"}, "a_b.example.com", "bad --host"},
		{ir.Target{Kind: "fly", Region: "ord", App: "blog-preview"}, "42.preview.example.com", "a Fly environment is served at its app's address (blog-preview.fly.dev)"},
	} {
		if _, err := withHost("preview", c.t, c.host); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("withHost(%q): %v, want %q", c.host, err, c.want)
		}
	}
}

// ci init says what a preview domain that is not a wildcard does.
func TestCi_NonWildcardPreview(t *testing.T) {
	d := &ir.Deploy{Envs: map[string]ir.Target{"preview": {Kind: "ssh", Host: "deploy@box.example.com", Domain: "preview.example.com", Path: "/srv/p"}}}
	wf := ciWorkflow(d, "v0.1.0", "1.27")
	if !strings.Contains(strings.Join(wf.Notes, "\n"), "the preview domain preview.example.com is not a wildcard: each pull request is served at <number>.preview.example.com") {
		t.Errorf("notes: %q", wf.Notes)
	}
	if !strings.Contains(wf.YAML, "# The preview domain preview.example.com is not a wildcard") {
		t.Errorf("the file does not say so:\n%s", wf.YAML)
	}
}

// The Render jobs have no SSH step; they push an image (`packages: write`, the registry token
// falling back to the run's) and read the preview's address from `alder deploy preview --json`.
func TestCiInit_RenderOnly(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "ci", "render-only.yml"))
	if err != nil {
		t.Fatal(err)
	}
	y := string(b)
	for _, want := range []string{
		"#   RENDER_API_KEY        ", "#   TURSO_API_TOKEN       ", "#   ALDER_REGISTRY_TOKEN  ", "#   SMTP_URL              ", "#   ALDER_SECRET          ",
		"personal access token set once as\n# ALDER_REGISTRY_TOKEN is simplest; the run token alone works only for a public image.",
		"The first deploy creates the Render service",
		"      packages: write\n",
		"          ALDER_REGISTRY_TOKEN: ${{ secrets.ALDER_REGISTRY_TOKEN || github.token }}\n",
		"          alder deploy preview --json > \"$RUNNER_TEMP/deploy.json\"\n",
		"          ADDRESS: ${{ steps.deploy.outputs.address }}\n",
		"        run: alder deploy production\n",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("render-only.yml lacks %q", want)
		}
	}
	for _, not := range []string{"SSH", "--host", "HOST"} {
		if strings.Contains(y, not) {
			t.Errorf("render-only.yml has %q: a Render job has no SSH step and no --host", not)
		}
	}
}

// Production at Render and previews on a box: the SSH step only in the job that deploys to the box.
func TestCiInit_Mixed(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "ci", "mixed.yml"))
	if err != nil {
		t.Fatal(err)
	}
	y := string(b)
	prod := y[strings.Index(y, "\n  production:"):]
	prev := y[strings.Index(y, "\n  preview:"):strings.Index(y, "\n  production:")]
	if strings.Contains(prod, "SSH key") || !strings.Contains(prev, "SSH key") || strings.Contains(prev, "packages: write") || !strings.Contains(prod, "packages: write") {
		t.Errorf("mixed.yml:\n%s", y)
	}
	if !strings.Contains(prev, "alder deploy preview --host \"$HOST\"") || strings.Contains(prod, "--host") {
		t.Errorf("--host is for the box's preview only:\n%s", y)
	}
}

// `alder ci init` pushes an image: a Render environment with no `image` and no GitHub origin is
// refused (R5); with an origin it writes the file.
func TestCiInit_RenderImage(t *testing.T) {
	dir := deployScaffold(t, `deploy {
  production: Render "oregon" { service: "blog", database: Turso "acme" { app: "blog" } },
}`, nil)
	var out, errb strings.Builder
	if code := runCiTo([]string{"init", dir}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "E0030 name the registry path") {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errb.String())
	}
	writeFile(t, filepath.Join(dir, ".git", "config"), "[remote \"origin\"]\n\turl = https://github.com/Tavish/Blog\n")
	out.Reset()
	errb.Reset()
	if code := runCiTo([]string{"init", dir}, &out, &errb); code != 0 || !strings.Contains(out.String(), "repository secrets it needs: RENDER_API_KEY, TURSO_API_TOKEN, ALDER_REGISTRY_TOKEN, ") {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errb.String())
	}
}
