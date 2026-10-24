package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"alder/internal/deploy"
	"alder/internal/ir"
	"alder/internal/registryfake"
	"alder/internal/renderfake"
	"alder/internal/tursofake"
	"alder/rt/db"
)

// The Render target through the commands, against the three fakes (internal/renderfake,
// internal/registryfake, internal/tursofake). Its engine-level cases are in internal/deploy.

const (
	rdKey      = "rnd_KEY_must_not_appear"
	rdRegToken = "ghp_REGISTRY_must_not_appear"
	rdSecret   = "0123456789abcdef0123456789abcdef"
	rdSMTP     = "smtp://user:smtp-pass-must-not-appear@mail.example:25"
	rdDecl     = `deploy {
  production: Render "oregon" { service: "blog", plan: "starter", database: Turso "acme" { app: "blog" } },
  image: "ghcr.io/tavish/blog",
  requires: ["SMTP_URL"],
}`
)

type renderFakes struct {
	render *renderfake.Server
	reg    *registryfake.Server
	turso  *tursofake.Server
	clock  *fakeClock // the deploys' points in time and the Turso fake's commits
}

// renderApp makes the scaffold app with decl, and the deploy commands pointed at fresh fakes.
func renderApp(t *testing.T, decl string) (string, *renderFakes, *deployCmds) {
	t.Helper()
	dir := deployScaffold(t, decl, nil)
	f := &renderFakes{render: renderfake.New(t), reg: registryfake.New(t, registryfake.Options{User: "tavish", Token: rdRegToken, RequireAuth: true}), turso: tursofake.New(t, "acme")}
	f.render.RequireKey(rdKey)
	c := newDeployCmds(t)
	f.clock = c.useFakeClock()
	f.turso.SetNow(f.clock.Now)
	for k, v := range map[string]string{
		"RENDER_API_KEY": rdKey, "ALDER_RENDER_API": f.render.URL, "RENDER_OWNER_ID": "",
		"ALDER_REGISTRY_TOKEN": rdRegToken, "ALDER_REGISTRY_USER": "", "ALDER_REGISTRY_BASE": f.reg.URL,
		db.EnvTursoToken: f.turso.APIToken, db.EnvTursoAPI: f.turso.URL,
		"ALDER_SECRET": rdSecret, "SMTP_URL": rdSMTP,
	} {
		c.set(k, v)
	}
	c.env.Turso = db.TursoConfig{API: f.turso.URL, Token: f.turso.APIToken}
	c.env.HealthTimeout, c.env.Drain = 30*time.Second, -1 // -1: no drain
	return dir, f, c
}

// noSecrets fails when a secret of the environment is in any of texts.
func (f *renderFakes) noSecrets(t *testing.T, texts ...string) {
	t.Helper()
	for _, s := range []string{rdKey, rdRegToken, f.turso.APIToken, rdSecret, "smtp-pass-must-not-appear"} {
		for _, text := range texts {
			if s != "" && strings.Contains(text, s) {
				t.Errorf("a secret (%q…) is in the output:\n%s", s[:min(len(s), 8)], text)
			}
		}
	}
}

// `alder deploy production --host x` is refused: a Render service has the one address Render
// gave it.
func TestDeployRender_HostRefused(t *testing.T) {
	t.Parallel()
	dir, f, c := renderApp(t, rdDecl)
	code, out, errs := c.deploy("production", dir, "--host", "x.example.com")
	if code != 2 || !strings.Contains(errs, "the address is the service's: https://blog.onrender.com") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	if n := len(f.render.Requests()); n != 0 {
		t.Errorf("a refused --host reached Render: %v", f.render.Requests())
	}
	for _, cmd := range []func([]string, io.Writer, io.Writer) int{c.status, c.logs} {
		if code, out := runCmd(t, cmd, "production", dir, "--host", "x.example.com"); code != 2 || !strings.Contains(out, "the address is the service's") {
			t.Errorf("exit %d\n%s", code, out)
		}
	}
}

// `--json` carries the address of the environment: Ssh https://<domain>, Fly
// https://<app>.fly.dev, Render the service's URL; a Script has none.
func TestDeployRender_JSONAddress(t *testing.T) {
	t.Parallel()
	ssh := ir.Target{Kind: "ssh", Host: "deploy@box", Domain: "blog.example.com", Path: "/srv/blog"}
	fly := ir.Target{Kind: "fly", Region: "ord", App: "blog"}
	render := deploy.NewRender("blog", "production", ir.Target{Kind: "render", Region: "oregon", Service: "blog", Database: &ir.Database{Kind: "turso", Org: "acme", App: "blog"}}, nil, "ghcr.io/tavish/blog")
	for _, c := range []struct {
		name string
		t    ir.Target
		tg   deploy.Target
		want string
	}{
		{"ssh", ssh, deploy.NewSsh("blog", "production", ssh, &ir.Deploy{}), "https://blog.example.com"},
		{"ssh preview host", ir.Target{Kind: "ssh", Host: "deploy@box", Domain: "7.preview.example.com", Path: "/srv/p/7.preview.example.com"}, nil, "https://7.preview.example.com"},
		{"ssh wildcard without a host", ir.Target{Kind: "ssh", Host: "deploy@box", Domain: "*.preview.example.com", Path: "/srv/p"}, nil, ""},
		{"fly", fly, deploy.NewFly("blog", "production", fly, &ir.Deploy{}), "https://blog.fly.dev"},
		{"render, expected", ir.Target{Kind: "render", Service: "blog"}, render, "https://blog.onrender.com"},
		{"script", ir.Target{Kind: "script", ScriptPath: "./x.sh"}, nil, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := deployAddress(c.t, c.tg); got != c.want {
				t.Errorf("deployAddress = %q, want %q", got, c.want)
			}
			var b bytes.Buffer
			writeDeployJSON(&b, deploy.Record{Build: "abc", Env: "production"}, deployAddress(c.t, c.tg))
			var got map[string]any
			if err := json.Unmarshal(b.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			addr, has := got["address"]
			if (c.want != "") != has || (has && addr != c.want) || got["build"] != "abc" {
				t.Errorf("--json = %s; want address %q beside the record", b.String(), c.want)
			}
		})
	}
}

var depLive = regexp.MustCompile(`Render deploy (dep-\w+) live \(1 instance\)`)

// `alder deploy production` on the scaffold app against the three fakes: the first deploy creates
// the service, the second rolls it, `status`, `rollback`, `logs` and `restore --to` read and move
// it; no secret is in any output.
func TestDeployRender_EndToEnd(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	dir, f, c := renderApp(t, rdDecl)
	var all []string
	code, out, errs := c.deploy("production", dir, "--unsafe-skip-tests", "--json")
	all = append(all, out, errs)
	if code != 0 {
		t.Fatalf("first deploy: exit %d\n%s%s", code, out, errs)
	}
	var rec struct{ Build, Address string }
	if err := json.Unmarshal([]byte(out), &rec); err != nil || rec.Build == "" {
		t.Fatalf("--json: %v\n%s", err, out)
	}
	id1, errsFirst := rec.Build, errs
	svc := f.render.Service("blog")
	if svc == nil || rec.Address != svc.URL {
		t.Fatalf("--json address %q, the service's URL %v", rec.Address, svc)
	}
	for _, want := range []string{"alder deploy production → Render oregon (service blog)", "image pushed ghcr.io/tavish/blog:" + id1, "Render service blog created", "health: 1/1 instances serve " + id1} {
		if !strings.Contains(errs, want) {
			t.Errorf("first deploy lacks %q:\n%s", want, errs)
		}
	}
	if !strings.Contains(errs, "upload   ok  image ghcr.io/tavish/blog:"+id1+"\n") || strings.Contains(errs, "builds/") {
		t.Errorf("the transcript names a box directory for an image:\n%s", errs)
	}
	if !depLive.MatchString(errs) {
		t.Errorf("no `Render deploy dep-… live (1 instance)`:\n%s", errs)
	}

	// a second build rolls the existing service
	appendTo(t, filepath.Join(dir, "app", "Home.alder"), fmt.Sprintf("\nversion : Int\nversion = %d\n", time.Now().UnixNano()))
	code, out, errs = c.deploy("production", dir, "--unsafe-skip-tests")
	all = append(all, out, errs)
	id2 := deployedID(t, out)
	if code != 0 || id2 == id1 || !strings.Contains(out, "health: 1/1 instances serve "+id2) || !depLive.MatchString(out) {
		t.Fatalf("second deploy: exit %d\n%s%s", code, out, errs)
	}
	t.Logf("first deploy:\n%s", errsFirst)
	t.Logf("second deploy:\n%s", out)

	status := func() string {
		t.Helper()
		code, so := runCmd(t, c.status, "production", dir)
		all = append(all, so)
		if code != 0 {
			t.Fatalf("status: exit %d\n%s", code, so)
		}
		return so
	}
	st := status()
	for _, want := range []string{"current  " + id2, "service  " + svc.URL, "1 instance", "database "} {
		if !strings.Contains(st, want) {
			t.Errorf("status lacks %q:\n%s", want, st)
		}
	}
	t.Logf("status:\n%s", st)

	code, ro := runCmd(t, c.rollback, "production", "--dir", dir, "--yes")
	all = append(all, ro)
	if code != 0 || !strings.Contains(ro, "rolled back to "+id1) {
		t.Fatalf("rollback: exit %d\n%s", code, ro)
	}
	t.Logf("rollback:\n%s", ro)
	if st := status(); !strings.Contains(st, "current  "+id1) {
		t.Errorf("status after the rollback:\n%s", st)
	}

	f.render.Logs(renderfake.LogLine{Time: time.Now().Add(-time.Minute), Instance: "i-1", Message: "listening on :10000"}, renderfake.LogLine{Time: time.Now(), Instance: "i-1", Message: "GET / 200"})
	code, lo := runCmd(t, c.logs, "production", dir)
	all = append(all, lo)
	if code != 0 || !strings.Contains(lo, "[i-1] listening on :10000") || !strings.Contains(lo, "[i-1] GET / 200") {
		t.Fatalf("logs: exit %d\n%s", code, lo)
	}
	if !contains(f.render.Requests(), "GET /logs") {
		t.Errorf("logs did not use GET /logs: %v", f.render.Requests())
	}

	// restore --to: the shared database as it was is the app's again (a copy), the service rolled
	// onto it
	f.clock.Sleep(1100 * time.Millisecond)
	at := f.clock.Now().UTC().Truncate(time.Second)
	f.clock.Sleep(1100 * time.Millisecond)
	if code, o := runCmd(t, c.restore, "production", "--dir", dir, "--to", at.Format(time.RFC3339)); code != 2 || !strings.Contains(o, "not confirmed") {
		t.Errorf("restore without --yes: exit %d\n%s", code, o)
	}
	code, re := runCmd(t, c.restore, "production", "--dir", dir, "--to", at.Format(time.RFC3339), "--yes")
	all = append(all, re)
	if code != 0 || !strings.Contains(re, "back as it was at "+at.Format(time.RFC3339)) {
		t.Fatalf("restore: exit %d\n%s", code, re)
	}
	t.Logf("restore:\n%s", re)
	if names := strings.Join(f.turso.Names(), " "); !strings.Contains(names, "blog-r") || !strings.Contains(names, "blog ") && !strings.HasSuffix(names, "blog") {
		t.Errorf("databases after the restore: %s", names)
	}
	if code, o := runCmd(t, c.restore, "production", "--dir", dir, "--from", "s3://x/y", "--to", at.Format(time.RFC3339)); code != 2 || !strings.Contains(o, "is on Turso") {
		t.Errorf("restore --from: exit %d\n%s", code, o)
	}
	f.noSecrets(t, all...)
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s || strings.HasPrefix(x, s+"?") {
			return true
		}
	}
	return false
}

// --dry-run reads and plans: no change at Render, no push, and the address is marked expected.
func TestDeployRender_DryRun(t *testing.T) {
	t.Parallel()
	dir, f, c := renderApp(t, rdDecl)
	code, out, errs := c.deploy("production", dir, "--unsafe-skip-tests", "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	for _, r := range f.render.Requests() {
		if !strings.HasPrefix(r, "GET ") {
			t.Errorf("a dry run made %s at Render", r)
		}
	}
	for _, r := range f.reg.Requests() {
		if strings.HasPrefix(r, "PUT ") || strings.HasPrefix(r, "POST /v2/") || strings.HasPrefix(r, "PATCH ") {
			t.Errorf("a dry run made %s at the registry", r)
		}
	}
	if !strings.Contains(out, "https://blog.onrender.com (expected)") {
		t.Errorf("the dry run does not say where the service will be:\n%s", out)
	}
	f.noSecrets(t, out, errs)
}

// A Render environment with no `image` and no GitHub origin is refused with E0030's sentence;
// with one, the image is ghcr.io/<owner>/<repo>.
func TestDeployRender_DefaultImage(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	decl := strings.Replace(rdDecl, "  image: \"ghcr.io/tavish/blog\",\n", "", 1)
	dir, f, c := renderApp(t, decl)
	code, out, errs := c.deploy("production", dir, "--unsafe-skip-tests")
	if code != 2 || !strings.Contains(errs, "E0030 name the registry path") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	if code, o := runCmd(t, c.status, "production", dir); code != 2 || !strings.Contains(o, "E0030 name the registry path") {
		t.Errorf("status: exit %d\n%s", code, o)
	}
	writeFile(t, dir+"/.git/config", "[remote \"origin\"]\n\turl = git@github.com:Tavish/Blog.git\n")
	code, out, errs = c.deploy("production", dir, "--unsafe-skip-tests")
	id := regexp.MustCompile(`deployed ([0-9a-f]{16}) to production`).FindStringSubmatch(out)
	if code != 0 || id == nil || !strings.Contains(out, "image pushed ghcr.io/tavish/blog:"+id[1]) {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	if f.reg.Manifest("tavish/blog", id[1]) == nil {
		t.Errorf("no manifest at tavish/blog:%s; registry saw %v", id[1], f.reg.Requests())
	}
}
