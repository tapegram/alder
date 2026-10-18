package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/ir"
	"alder/internal/migrate"
	"alder/internal/registryfake"
	"alder/internal/renderfake"
	"alder/internal/tursofake"
	"alder/rt/db"
)

// The Render target driven by the engine (Deploy, Rollback, Status) against three fakes: Render's
// API (internal/renderfake), a registry (internal/registryfake) and Turso (internal/tursofake).
// The app's executable is not run: Turso.Local is a hook that applies the build's migrations to
// the fake through the protocol (so Turso's points in time see them) and returns a script that
// prints what --migrate-only would.

// The secrets every scenario sets on this machine; each rig checks, when its test ends, that none
// of them (nor the ALDER_SECRET generated at the service) appears in anything printed, returned,
// encoded as --json or written to a builds row.
const (
	rKey         = "rnd_SECRETKEY_9f8e7d"
	rRegToken    = "ghp_SECRETREG_1a2b3c"
	rLocalSecret = "local-SECRET-session-0c0c"
	rSMTP        = "smtp://mailer:SECRETPASS@mail.example"
	rImage       = "ghcr.io/tavish/blog"
)

type renderRig struct {
	t         *testing.T
	render    *renderfake.Server
	reg       *registryfake.Server
	turso     *tursofake.Server
	dir, root string
	n         int
	instances int
	refuse    bool // the next migration run refuses (exits 1) without applying anything
	out       syncBuffer
	texts     []string // errors, --json, other outputs
	secrets   []string // extra values the test set (dashboard variables)
}

// syncBuffer is a bytes.Buffer safe for a writer and a reader in two goroutines.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newRenderRig(t *testing.T) *renderRig {
	t.Helper()
	g := &renderRig{t: t, instances: 1}
	g.render = renderfake.New(t)
	g.render.RequireKey(rKey)
	g.reg = registryfake.New(t, registryfake.Options{User: "tavish", Token: rRegToken, RequireAuth: true})
	g.turso = tursofake.New(t, "acme")
	for k, v := range map[string]string{
		"RENDER_API_KEY": rKey, "ALDER_RENDER_API": g.render.URL, "RENDER_OWNER_ID": "",
		"ALDER_REGISTRY_TOKEN": rRegToken, "ALDER_REGISTRY_USER": "", "ALDER_REGISTRY_BASE": g.reg.URL,
		db.EnvTursoToken: g.turso.APIToken, db.EnvTursoAPI: g.turso.URL,
		"ALDER_SECRET": rLocalSecret, "SMTP_URL": rSMTP, "ALDER_DEPLOY_HEALTH_TIMEOUT": "",
	} {
		t.Setenv(k, v)
	}
	old := renderPoll
	renderPoll = time.Millisecond
	t.Cleanup(func() { renderPoll = old })
	g.dir = renderApp(t)
	g.root = filepath.Join(g.dir, "build", "deploy")
	t.Cleanup(g.checkSecrets)
	return g
}

func (g *renderRig) decl() *ir.Deploy {
	return &ir.Deploy{Requires: []string{"SMTP_URL"}, Image: rImage}
}

// target is a Render target made as the command makes it (NewRender, from the environment).
func (g *renderRig) target() *Render {
	r := NewRender("blog", "production", ir.Target{Kind: "render", Region: "oregon", Service: "blog", Plan: "starter", Instances: g.instances,
		Database: &ir.Database{Kind: "turso", Org: "acme", App: "blog"}}, g.decl(), rImage)
	r.Out = &g.out
	r.Artifact = func(id string) string { return filepath.Join(g.root, id) }
	r.DB.Local = g.local(r)
	return r
}

// local applies build id's migrations that the shared database does not have (through the
// protocol) and returns a script printing "migrated <name>" for each, as --migrate-only does.
func (g *renderRig) local(r *Render) func(ctx context.Context, id string) (string, error) {
	return func(ctx context.Context, id string) (string, error) {
		if g.refuse {
			script := filepath.Join(g.t.TempDir(), "migrate")
			return script, os.WriteFile(script, []byte("#!/bin/sh\necho 'refused: a sampled silo fails the plan'\nexit 1\n"), 0o755)
		}
		files, _ := filepath.Glob(filepath.Join(g.root, id, "migrations", "*.sql"))
		st, err := r.DB.api().Open(ctx, r.DB.SharedName(), false)
		if err != nil {
			return "", err
		}
		defer st.Close()
		for _, s := range []string{`CREATE TABLE IF NOT EXISTS "_alder_meta" ("key" TEXT PRIMARY KEY, "value" TEXT)`, `CREATE TABLE IF NOT EXISTS "_alder_migrations" ("name" TEXT PRIMARY KEY)`} {
			if err := db.ExecRaw(ctx, st, s); err != nil {
				return "", err
			}
		}
		have, err := appliedNames(ctx, st)
		if err != nil {
			return "", err
		}
		var out strings.Builder
		for _, f := range files {
			n := strings.TrimSuffix(filepath.Base(f), ".sql")
			if slices.Contains(have, n) {
				continue
			}
			if err := db.ExecRaw(ctx, st, `INSERT INTO "_alder_migrations" ("name") VALUES (?)`, n); err != nil {
				return "", err
			}
			out.WriteString("migrated " + n + "\n")
		}
		script := filepath.Join(g.t.TempDir(), "migrate")
		return script, os.WriteFile(script, []byte("#!/bin/sh\ncat <<'EOF'\n"+out.String()+"EOF\n"), 0o755)
	}
}

// deploy runs the engine's Deploy of a new build (its executable differs from the last one's).
func (g *renderRig) deploy(r *Render, mod ...func(*Options)) (Result, error) {
	g.n++
	n := g.n
	o := Options{App: "blog", Env: "production", AppDir: g.dir, Built: filepath.Join(g.dir, "build"), ArtifactRoot: g.root, Decl: g.decl(),
		Runner: passRunner{}, Out: &g.out, User: "ada", Drain: -1, HealthTimeout: 10 * time.Second,
		Binary: func(p Platform, path string) error {
			return os.WriteFile(path, []byte(fmt.Sprintf("binary %d for %s", n, p)), 0o755)
		}}
	for _, m := range mod {
		m(&o)
	}
	res, err := Deploy(context.Background(), o, GateResult{Check: "ok", Build: "ok"}, r, nil)
	g.keep(res, err)
	return res, err
}

func (g *renderRig) keep(v any, err error) {
	b, _ := json.Marshal(v)
	g.texts = append(g.texts, string(b))
	if err != nil {
		g.texts = append(g.texts, err.Error(), fmt.Sprintf("%+v", err))
	}
}

func (g *renderRig) mustDeploy(r *Render, mod ...func(*Options)) string {
	g.t.Helper()
	res, err := g.deploy(r, mod...)
	if err != nil {
		g.t.Fatalf("deploy: %v\n%s", err, g.out.String())
	}
	return res.Record.Build
}

func allowLoss(o *Options) { o.AllowDataLoss = true }
func dryRun(o *Options)    { o.DryRun = true }

// requests are the fake Render's requests from index from on.
func (g *renderRig) requests(from int) []string { return g.render.Requests()[from:] }

func nonGET(reqs []string) []string {
	var out []string
	for _, r := range reqs {
		if !strings.HasPrefix(r, "GET ") && !strings.HasPrefix(r, "HEAD ") {
			out = append(out, r)
		}
	}
	return out
}

func count(reqs []string, prefix string) int {
	n := 0
	for _, r := range reqs {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func envOf(s *renderfake.Service) map[string]string {
	m := map[string]string{}
	for _, e := range s.Env {
		m[e.Key] = e.Value
	}
	return m
}

// connected is a new target after Connect (as a later command sees the environment).
func (g *renderRig) connected() *Render {
	g.t.Helper()
	r := g.target()
	if _, err := r.Connect(context.Background()); err != nil {
		g.t.Fatal(err)
	}
	return r
}

func (g *renderRig) current() string {
	g.t.Helper()
	cur, err := g.connected().Current(context.Background())
	if err != nil {
		g.t.Fatal(err)
	}
	return cur
}

// leaks are the indexes of the values that appear in text.
func leaks(text string, values []string) []int {
	var out []int
	for i, v := range values {
		if v != "" && strings.Contains(text, v) {
			out = append(out, i)
		}
	}
	return out
}

// secretValues are every secret value the test set or the service holds.
func (g *renderRig) secretValues() []string {
	vals := append([]string{rKey, rRegToken, rLocalSecret, rSMTP, g.turso.APIToken}, g.secrets...)
	if s := g.render.Service("blog"); s != nil {
		if v := envOf(s)["ALDER_SECRET"]; v != "" {
			vals = append(vals, v)
		}
	}
	return vals
}

// everything is all the rig's outputs and every builds row's text.
func (g *renderRig) everything() string {
	return g.out.String() + "\n" + strings.Join(g.texts, "\n") + g.rows()
}

// rows is the text of every builds row of every database at the fake Turso.
func (g *renderRig) rows() string {
	all := ""
	for _, n := range g.turso.Names() {
		rows, err := g.turso.DB(n).Query(`SELECT "id", "record", "schema", "migrations", "pitr" FROM "_alder_deploy_builds"`)
		if err != nil {
			continue
		}
		for rows.Next() {
			var a, b, c, d, e string
			rows.Scan(&a, &b, &c, &d, &e)
			all += "\n" + strings.Join([]string{a, b, c, d, e}, "\n")
		}
		rows.Close()
	}
	return all
}

func (g *renderRig) checkSecrets() {
	vals := g.secretValues()
	for _, i := range leaks(g.everything(), vals) {
		g.t.Errorf("secret value #%d (of %d set in the test or held by the service) appears in an output or a builds row", i, len(vals))
	}
}

// renderApp is an app whose one migration creates Post with a summary (schemas' s1).
func renderApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	os.MkdirAll(mig, 0o755)
	os.MkdirAll(filepath.Join(dir, "build", "static"), 0o755)
	os.WriteFile(filepath.Join(dir, "build", "static", "app.css"), []byte("body{}"), 0o644)
	s1, _ := schemas()
	m1, err := migrate.Plan(migrate.Schema{Format: db.Format}, s1, "0001_initial")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(mig, "0001_initial.sql"), []byte(migrate.Render(m1)), 0o644)
	if err := migrate.WriteSnapshot(filepath.Join(mig, "schema.json"), s1); err != nil {
		t.Fatal(err)
	}
	return dir
}

// addDrop adds the migration that drops Post.summary (s2).
func addDrop(t *testing.T, dir string) {
	t.Helper()
	s1, s2 := schemas()
	m2, err := migrate.Plan(s1, s2, "0002_drop_summary")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "migrations", "0002_drop_summary.sql"), []byte(migrate.Render(m2)), 0o644)
	if err := migrate.WriteSnapshot(filepath.Join(dir, "migrations", "schema.json"), s2); err != nil {
		t.Fatal(err)
	}
}

func copies(names []string) []string {
	var out []string
	for _, n := range names {
		if strings.HasPrefix(n, "blog-r") {
			out = append(out, n)
		}
	}
	return out
}

func migrationsIn(t *testing.T, f *tursofake.Server, name string) string {
	t.Helper()
	rows, err := f.DB(name).Query(`SELECT "name" FROM "_alder_migrations" ORDER BY "name"`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		out = append(out, n)
	}
	return strings.Join(out, ",")
}

// The first deploy: the registry credential, the image pushed, the service created (R2) with
// the health path, instances, region, plan and the whole environment, Render live, health 1/1,
// the build current and one builds row.
func TestRender_FirstDeploy(t *testing.T) {
	g := newRenderRig(t)
	r := g.target()
	id := g.mustDeploy(r)
	out := g.out.String()

	creds := g.render.Credentials()
	if len(creds) != 1 || creds[0].Name != "alder-tavish/blog" || creds[0].Registry != "GITHUB" || creds[0].Username != "tavish" || creds[0].Token != rRegToken {
		t.Errorf("credentials: %d %+v", len(creds), func() []string {
			var s []string
			for _, c := range creds {
				s = append(s, c.Name+" "+c.Registry+" "+c.Username)
			}
			return s
		}())
	}
	if g.reg.Manifest("tavish/blog", id) == nil {
		t.Errorf("no manifest %s pushed: %v", id, g.reg.Requests())
	}
	s := g.render.Service("blog")
	if s == nil {
		t.Fatalf("no service: %v", g.render.Requests())
	}
	if s.HealthCheckPath != "/_alder/health" || s.Instances != 1 || s.Region != "oregon" || s.Plan != "starter" || s.MaxShutdownDelay != 30 ||
		s.Image != rImage+":"+id || s.CredentialID != creds[0].ID {
		t.Errorf("service: path %q instances %d region %q plan %q delay %d image %q credential %q", s.HealthCheckPath, s.Instances, s.Region, s.Plan, s.MaxShutdownDelay, s.Image, s.CredentialID)
	}
	host, err := r.DB.Host(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	env := envOf(s)
	want := map[string]string{"ALDER_DB": "libsql://" + host, "ALDER_TURSO_ORG": "acme", "ALDER_TURSO_GROUP": "default", "ALDER_TURSO_APP": "blog",
		"ALDER_TURSO_DB": "blog", "ALDER_TURSO_API": g.turso.URL, "TURSO_API_TOKEN": g.turso.APIToken, "ALDER_ADDR": ":10000", "SMTP_URL": rSMTP}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("env %s: got %d bytes, want %d", k, len(env[k]), len(v))
		}
	}
	if !regexp.MustCompile(`^[0-9a-f]{48}$`).MatchString(env["ALDER_SECRET"]) || env["ALDER_SECRET"] == rLocalSecret {
		t.Errorf("ALDER_SECRET is not a new 48-hex key")
	}
	reqs := g.render.Requests()
	if count(reqs, "POST /services") != 1 || count(reqs, "PUT ") != 0 || count(reqs, "POST /registrycredentials") != 1 {
		t.Errorf("render requests: %v", nonGET(reqs))
	}
	for _, l := range []string{"image pushed " + rImage + ":" + id + " (", " live (1 instance)\n", "health: 1/1 instances serve " + id + "\n"} {
		if !strings.Contains(out, l) {
			t.Errorf("transcript lacks %q:\n%s", l, out)
		}
	}
	if !regexp.MustCompile(`Render deploy dep-\S+ live`).MatchString(out) {
		t.Errorf("no Render deploy line:\n%s", out)
	}
	r2 := g.connected()
	ctx := context.Background()
	if cur, _ := r2.Current(ctx); cur != id {
		t.Errorf("current %q, want %s", cur, id)
	}
	builds, err := r2.Builds(ctx)
	if err != nil || len(builds) != 1 || builds[0].ID != id || !builds[0].Current || !builds[0].Running || builds[0].Record == nil || builds[0].Record.Port != 443 {
		t.Errorf("builds: %+v %v", builds, err)
	}
	if r2.Address() != g.render.URL+"/svc/blog" || r2.AddressExpected() {
		t.Errorf("address %q", r2.Address())
	}
}

// The second deploy: no credential and no service made; the image pushed with every blob
// HEADed first and one manifest PUT; ALDER_SECRET kept; one POST deploys of <image>:<id2>.
func TestRender_SecondDeploy(t *testing.T) {
	g := newRenderRig(t)
	g.mustDeploy(g.target())
	secret := envOf(g.render.Service("blog"))["ALDER_SECRET"]
	nr, ng := len(g.render.Requests()), len(g.reg.Requests())
	id2 := g.mustDeploy(g.target())
	reqs := g.requests(nr)
	if count(reqs, "POST /registrycredentials") != 0 || count(reqs, "POST /services ") != 0 || slices.Contains(reqs, "POST /services") {
		t.Errorf("made again: %v", nonGET(reqs))
	}
	if n := count(reqs, "POST /services/"); n != 1 {
		t.Errorf("%d POSTs: %v", n, nonGET(reqs))
	}
	var body string
	for _, c := range g.render.Calls() {
		if c.Method == "POST" && strings.HasSuffix(c.Path, "/deploys") {
			body = c.Body
		}
	}
	if !strings.Contains(body, `"imageUrl":"`+rImage+":"+id2+`"`) {
		t.Errorf("deploy body %s", body)
	}
	if got := envOf(g.render.Service("blog"))["ALDER_SECRET"]; got != secret {
		t.Error("ALDER_SECRET changed")
	}
	if count(reqs, "PUT ") != 0 {
		t.Errorf("the environment did not change, yet: %v", nonGET(reqs))
	}
	regs := g.reg.Requests()[ng:]
	manifests, headed := 0, map[string]bool{}
	for _, q := range regs {
		switch {
		case strings.HasPrefix(q, "PUT /v2/tavish/blog/manifests/"):
			manifests++
			if q != "PUT /v2/tavish/blog/manifests/"+id2 {
				t.Errorf("manifest %s", q)
			}
		case strings.HasPrefix(q, "HEAD /v2/tavish/blog/blobs/"):
			headed[strings.TrimPrefix(q, "HEAD /v2/tavish/blog/blobs/")] = true
		case strings.HasPrefix(q, "PUT /v2/tavish/blog/blobs/uploads/"):
			if len(headed) == 0 {
				t.Errorf("a blob uploaded before any HEAD: %v", regs)
			}
		}
	}
	if manifests != 1 || len(headed) != 2 {
		t.Errorf("registry: %d manifests, %d blobs headed: %v", manifests, len(headed), regs)
	}
	if g.current() != id2 {
		t.Errorf("current is not %s", id2)
	}
}

// Review Focus 1: a variable set in Render's dashboard (SMTP_URL, not on this machine) is kept by
// the deploy's one env-var PUT, Requires does not report it missing, and no value is printed.
func TestRender_EnvMergeKeepsDashboardVars(t *testing.T) {
	g := newRenderRig(t)
	t.Setenv("SMTP_URL", "")
	dash, other, secret := "smtp://dash:DASHPASS@mail.example", "dashboard-OTHER-value", strings.Repeat("ab", 24)
	g.secrets = append(g.secrets, dash, other)
	g.render.AddService("blog", rImage+":0000000000000000", 1,
		renderfake.Env{Key: "SMTP_URL", Value: dash}, renderfake.Env{Key: "OTHER_VAR", Value: other}, renderfake.Env{Key: "ALDER_SECRET", Value: secret})
	nr := len(g.render.Requests())
	id := g.mustDeploy(g.target())
	if !strings.Contains(g.out.String(), "  requires ok  SMTP_URL, ALDER_SECRET\n") {
		t.Errorf("requires:\n%s", g.out.String())
	}
	env := envOf(g.render.Service("blog"))
	if env["SMTP_URL"] != dash || env["OTHER_VAR"] != other || env["ALDER_SECRET"] != secret || env["ALDER_TURSO_DB"] != "blog" || env["ALDER_ADDR"] != ":10000" {
		t.Errorf("the merge lost a variable: %v", slices.Sorted(func(yield func(string) bool) {
			for k := range env {
				if !yield(k) {
					return
				}
			}
		}))
	}
	reqs := g.requests(nr)
	if count(reqs, "PUT /services/") != 1 || count(reqs, "POST /services ") != 0 {
		t.Errorf("requests: %v", nonGET(reqs))
	}
	if g.current() != id {
		t.Error("not current")
	}
}

// A `requires` name unset on this machine and at the service is the refusal, before anything
// changes at Render.
func TestRender_RequiresMissing(t *testing.T) {
	g := newRenderRig(t)
	t.Setenv("SMTP_URL", "")
	g.render.AddService("blog", rImage+":0000000000000000", 1)
	_, err := g.deploy(g.target())
	if err == nil || !strings.Contains(err.Error(), "does not set SMTP_URL") {
		t.Fatalf("err = %v", err)
	}
	if n := nonGET(g.render.Requests()); len(n) != 0 {
		t.Errorf("changed at Render: %v", n)
	}
}

// instances: 2, the second instance late to switch: the health check waits until both serve the
// build (2/2); the change 1 → 2 is one POST scale.
func TestRender_TwoInstances(t *testing.T) {
	g := newRenderRig(t)
	g.mustDeploy(g.target())
	g.instances = 2
	nr := len(g.render.Requests())
	g.render.DelaySwitch("blog", 1)
	released := make(chan int, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Millisecond):
			}
			if h := g.render.HealthHits("blog"); h >= 8 {
				g.render.ReleaseSwitch("blog")
				released <- h
				return
			}
		}
	}()
	id := g.mustDeploy(g.target())
	select {
	case <-released:
	default:
		t.Fatalf("the health check passed before the late instance switched:\n%s", g.out.String())
	}
	if !strings.Contains(g.out.String(), "health: 2/2 instances serve "+id+"\n") || !strings.Contains(g.out.String(), " live (2 instances)\n") {
		t.Errorf("transcript:\n%s", g.out.String())
	}
	if n := count(g.requests(nr), "POST /services/"+g.render.Service("blog").ID+"/scale"); n != 1 {
		t.Errorf("%d scale requests: %v", n, nonGET(g.requests(nr)))
	}
	if s := g.render.Service("blog"); s.Instances != 2 {
		t.Errorf("instances %d", s.Instances)
	}
}

// A data-dropping plan without --allow-data-loss is refused before any POST or PUT at Render.
func TestRender_MigrationRefusedBeforeRender(t *testing.T) {
	g := newRenderRig(t)
	g.mustDeploy(g.target())
	addDrop(t, g.dir)
	nr := len(g.render.Requests())
	_, err := g.deploy(g.target())
	var r *Refusal
	if !errors.As(err, &r) || !strings.Contains(err.Error(), "the migration drops data") || !strings.Contains(err.Error(), "DROPS DATA: 0002_drop_summary") {
		t.Fatalf("err = %v\n%s", err, g.out.String())
	}
	if n := nonGET(g.requests(nr)); len(n) != 0 {
		t.Errorf("changed at Render: %v", n)
	}
}

var renderLogLines = []renderfake.LogLine{
	{Time: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC), Instance: "srv-a1", Message: "listening on :10000"},
	{Time: time.Date(2026, 10, 17, 12, 0, 1, 0, time.UTC), Instance: "srv-a1", Message: "boom: the mail server is unreachable"},
}

// Render answers update_failed: the error carries Render's log lines, the previous build stays
// current, and no row of the new build is current.
func TestRender_UpdateFailed(t *testing.T) {
	g := newRenderRig(t)
	id1 := g.mustDeploy(g.target())
	g.render.Logs(renderLogLines...)
	g.render.Script("update_in_progress", "update_failed")
	res, err := g.deploy(g.target())
	if err == nil {
		t.Fatal("deployed")
	}
	msg := err.Error()
	if !strings.Contains(msg, "update_failed") || !strings.Contains(msg, "[srv-a1] boom: the mail server is unreachable") || !strings.Contains(msg, "build "+id1+" still serves") {
		t.Errorf("error: %s", msg)
	}
	if g.current() != id1 {
		t.Error("current changed")
	}
	builds, _ := g.connected().Builds(context.Background())
	for _, b := range builds {
		if b.ID == res.Record.Build && (b.Current || b.Record != nil) {
			t.Errorf("the failed build is recorded: %+v", b)
		}
	}
}

// The roll is bounded by RollTimeout (the command's ALDER_DEPLOY_HEALTH_TIMEOUT, the one source
// for every target): a deploy that never goes live is cancelled and the error says so.
func TestRender_RollTimeout(t *testing.T) {
	g := newRenderRig(t)
	id1 := g.mustDeploy(g.target())
	g.render.Script("update_in_progress")
	r := g.target()
	r.RollTimeout = 100 * time.Millisecond
	_, err := g.deploy(r)
	if err == nil || !strings.Contains(err.Error(), "still update_in_progress after 100ms") || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error: %v", err)
	}
	if g.current() != id1 {
		t.Error("current changed")
	}
}

// Review Focus 3: a deploy that fails at Render after a data-dropping migration: the engine's
// restoreAfterDrop makes exactly one point-in-time copy and one env-var PUT naming it; the
// previous build serves again on the copy; the error carries Render's log lines; and the next
// run is not locked out.
func TestRender_UpdateFailedAfterDropRestores(t *testing.T) {
	g := newRenderRig(t)
	ctx := context.Background()
	id1 := g.mustDeploy(g.target())
	addDrop(t, g.dir)
	g.render.Logs(renderLogLines...)
	g.render.Script("update_in_progress", "update_failed")
	nr := len(g.render.Requests())
	_, err := g.deploy(g.target(), allowLoss)
	if err == nil {
		t.Fatal("deployed")
	}
	msg := err.Error()
	if !strings.Contains(msg, "was put back automatically and build "+id1+" serves again") || !strings.Contains(msg, "[srv-a1] boom: the mail server is unreachable") {
		t.Fatalf("error: %s\n%s", msg, g.out.String())
	}
	cps := copies(g.turso.Names())
	if len(cps) != 1 {
		t.Fatalf("copies %v", g.turso.Names())
	}
	if got := migrationsIn(t, g.turso, cps[0]); got != "0001_initial" {
		t.Errorf("the copy holds %s: not the database before the migration", got)
	}
	if got := migrationsIn(t, g.turso, "blog"); got != "0001_initial,0002_drop_summary" {
		t.Errorf("the replaced database holds %s", got)
	}
	r := g.connected()
	cp, _, err := r.DB.api().Database(ctx, cps[0])
	if err != nil {
		t.Fatal(err)
	}
	reqs := g.requests(nr)
	if n := count(reqs, "PUT /services/"); n != 1 {
		t.Errorf("%d env-var PUTs: %v", n, nonGET(reqs))
	}
	for _, c := range g.render.Calls() {
		if c.Method == "PUT" && !strings.Contains(c.Body, `"libsql://`+cp.Hostname+`"`) {
			t.Errorf("the PUT does not name the copy's host")
		}
	}
	if env := envOf(g.render.Service("blog")); env["ALDER_TURSO_DB"] != cps[0] || env["ALDER_DB"] != "libsql://"+cp.Hostname {
		t.Errorf("service database %q", env["ALDER_TURSO_DB"])
	}
	if r.DB.SharedName() != cps[0] {
		t.Errorf("a new run reads the database %s", r.DB.SharedName())
	}
	if cur, _ := r.Current(ctx); cur != id1 {
		t.Errorf("current %q", cur)
	}
	unlock, err := r.Lock(ctx)
	if err != nil {
		t.Fatalf("the next run is locked out: %v", err)
	}
	unlock()
}

// Rollback: the build before the current one is deployed again by its tag, health checked and
// made current; its record says it was rolled back to.
func TestRender_Rollback(t *testing.T) {
	g := newRenderRig(t)
	id1 := g.mustDeploy(g.target())
	g.mustDeploy(g.target())
	nr := len(g.render.Requests())
	to, err := Rollback(context.Background(), g.target(), RollbackOptions{Env: "production", Out: &g.out, Drain: -1, HealthTimeout: 10 * time.Second})
	g.keep(to, err)
	if err != nil || to != id1 {
		t.Fatalf("%q %v\n%s", to, err, g.out.String())
	}
	assertRolledTo(t, g, nr, id1)
}

func assertRolledTo(t *testing.T, g *renderRig, nr int, id string) {
	t.Helper()
	reqs := g.requests(nr)
	if n := count(reqs, "POST /services/"); n != 1 {
		t.Errorf("POSTs: %v", nonGET(reqs))
	}
	found := false
	for _, c := range g.render.Calls() {
		if c.Method == "POST" && strings.HasSuffix(c.Path, "/deploys") && strings.Contains(c.Body, `"imageUrl":"`+rImage+":"+id+`"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("no deploy of %s:%s", rImage, id)
	}
	if g.current() != id {
		t.Errorf("current is not %s", id)
	}
	builds, _ := g.connected().Builds(context.Background())
	for _, b := range builds {
		if b.ID == id && (b.Record == nil || !b.Record.RolledBack) {
			t.Errorf("record of %s: %+v", id, b.Record)
		}
	}
}

// Review Focus 4: a rollback from a machine without the old build's artifact (a fresh CI
// checkout) reads the build's schema from its builds row: the schema rule refuses a build older
// than the live database, and a build with the live schema is deployed again by its tag.
func TestRender_RollbackWithoutLocalArtifact(t *testing.T) {
	g := newRenderRig(t)
	ctx := context.Background()
	id1 := g.mustDeploy(g.target())
	id2 := g.mustDeploy(g.target())
	addDrop(t, g.dir)
	g.mustDeploy(g.target(), allowLoss)
	if err := os.RemoveAll(g.root); err != nil {
		t.Fatal(err)
	}
	r := g.connected()
	sc, err := r.BuildSchema(ctx, id1)
	s1, _ := schemas()
	want, _ := json.Marshal(s1)
	if err != nil || !sc.Have || strings.Join(sc.Migrations, ",") != "0001_initial" || SchemaHash(sc.Snapshot) != SchemaHash(want) {
		t.Fatalf("build schema from the row: %+v %v", sc, err)
	}
	nr := len(g.render.Requests())
	_, err = Rollback(ctx, g.target(), RollbackOptions{To: id1, Env: "production", Out: &g.out, Drain: -1})
	g.keep(nil, err)
	if err == nil || !strings.Contains(err.Error(), "the live database's schema") || !strings.Contains(err.Error(), "is not build "+id1+"'s") {
		t.Fatalf("err = %v", err)
	}
	if n := nonGET(g.requests(nr)); len(n) != 0 {
		t.Errorf("the refused rollback changed Render: %v", n)
	}
	// a build with the live schema: deploy one more without the drop's artifact, then roll back to it
	_, err = Rollback(ctx, g.target(), RollbackOptions{To: id2, Env: "production", Out: &g.out, Drain: -1, AllowSchemaMismatch: true, HealthTimeout: 10 * time.Second})
	g.keep(nil, err)
	if err != nil {
		t.Fatalf("rollback: %v\n%s", err, g.out.String())
	}
	assertRolledTo(t, g, nr, id2)
}

// rollback --restore-backup: exactly one point-in-time copy (at the time recorded before the
// later build migrated) and exactly one env-var PUT naming the copy's host.
func TestRender_RollbackRestoreBackup(t *testing.T) {
	g := newRenderRig(t)
	ctx := context.Background()
	id1 := g.mustDeploy(g.target())
	addDrop(t, g.dir)
	id2 := g.mustDeploy(g.target(), allowLoss)
	nr := len(g.render.Requests())
	to, err := Rollback(ctx, g.target(), RollbackOptions{To: id1, RestoreBackup: true, Env: "production", Out: &g.out, Drain: -1, HealthTimeout: 10 * time.Second})
	g.keep(to, err)
	if err != nil || to != id1 {
		t.Fatalf("%q %v\n%s", to, err, g.out.String())
	}
	cps := copies(g.turso.Names())
	if len(cps) != 1 {
		t.Fatalf("copies: %v", g.turso.Names())
	}
	if got := migrationsIn(t, g.turso, cps[0]); got != "0001_initial" {
		t.Errorf("the copy holds %s", got)
	}
	r := g.connected()
	cp, _, _ := r.DB.api().Database(ctx, cps[0])
	reqs := g.requests(nr)
	if n := count(reqs, "PUT /services/"); n != 1 {
		t.Errorf("%d env-var PUTs: %v", n, nonGET(reqs))
	}
	for _, c := range g.render.Calls() {
		if c.Method == "PUT" && !strings.Contains(c.Body, `"libsql://`+cp.Hostname+`"`) {
			t.Error("the PUT does not name the copy's host")
		}
	}
	if cur, _ := r.Current(ctx); cur != id1 {
		t.Errorf("current %q", cur)
	}
	// the copy holds the builds rows of the database it replaced (as they were when it was
	// replaced, not at the point in time): id2's record and mark stay
	builds, _ := r.Builds(ctx)
	seen := 0
	for _, b := range builds {
		if b.ID == id1 && (b.Record == nil || b.Record.RestoredFrom == "") {
			t.Errorf("record: %+v", b.Record)
		}
		if b.ID == id2 && (b.Record == nil || b.BackupOf != id1) {
			t.Errorf("build %s lost its row in the copy: %+v", id2, b)
		}
		seen++
	}
	if seen != 2 {
		t.Errorf("%d builds", seen)
	}
	unlock, err := r.Lock(ctx)
	if err != nil {
		t.Fatalf("locked out after the restore: %v", err)
	}
	unlock()
}

// RestorePointInTime: the copy becomes the service's database and the current build rolls onto
// it; when that roll fails the replaced database is put back (one more PUT) and the lock is not
// left behind.
func TestRender_RestorePointInTime(t *testing.T) {
	g := newRenderRig(t)
	ctx := context.Background()
	id1 := g.mustDeploy(g.target())
	time.Sleep(1100 * time.Millisecond)
	at := time.Now().UTC().Truncate(time.Second)
	time.Sleep(1100 * time.Millisecond)

	r := g.connected()
	unlock, err := r.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nr := len(g.render.Requests())
	kept, err := r.RestorePointInTime(ctx, at, &g.out)
	g.keep(kept, err)
	unlock()
	if err != nil || !strings.Contains(kept, "the database blog at Turso") {
		t.Fatalf("%q %v", kept, err)
	}
	cps := copies(g.turso.Names())
	if len(cps) != 1 {
		t.Fatalf("copies: %v", g.turso.Names())
	}
	reqs := g.requests(nr)
	if n := count(reqs, "PUT /services/"); n != 1 {
		t.Errorf("%d env-var PUTs: %v", n, nonGET(reqs))
	}
	if count(reqs, "POST /services/") < 1 {
		t.Errorf("the current build was not rolled onto the copy: %v", nonGET(reqs))
	}
	if cur := g.current(); cur != id1 {
		t.Errorf("current %q in the copy", cur)
	}

	// a roll that fails brings the replaced database back
	g2 := g.connected()
	unlock, err = g2.Lock(ctx)
	if err != nil {
		t.Fatalf("locked out after the restore: %v", err)
	}
	defer unlock()
	g.render.Script("update_in_progress", "update_failed")
	time.Sleep(1100 * time.Millisecond) // the copy's name carries the second
	_, err = g2.RestorePointInTime(ctx, time.Now().UTC(), &g.out)
	g.keep("", err)
	if err == nil || !strings.Contains(err.Error(), "is the service's again") {
		t.Fatalf("error: %v", err)
	}
	if got := g2.DB.SharedName(); got != cps[0] {
		t.Errorf("shared database %q, want %q (the one before the failed restore)", got, cps[0])
	}
}

// The lock held by another run (a row of the shared database's _alder_lock): refused naming
// the holder, with nothing at Render but Connect's reads and nothing pushed.
func TestRender_LockHeld(t *testing.T) {
	g := newRenderRig(t)
	if err := g.turso.Create("blog", "default"); err != nil {
		t.Fatal(err)
	}
	g.turso.Exec("blog", `CREATE TABLE "_alder_lock" ("key" TEXT NOT NULL PRIMARY KEY, "holder" TEXT NOT NULL, "expires" TEXT NOT NULL)`)
	g.turso.Exec("blog", `INSERT INTO "_alder_lock" VALUES ('deploy', 'ci@runner', ?)`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	_, err := g.deploy(g.target())
	var r *Refusal
	if !errors.As(err, &r) || !strings.Contains(err.Error(), "ci@runner") {
		t.Fatalf("err = %v", err)
	}
	for _, q := range g.render.Requests() {
		if !strings.HasPrefix(q, "GET /owners") && !strings.HasPrefix(q, "GET /services?") {
			t.Errorf("beyond Connect: %s", q)
		}
	}
	if n := len(g.reg.Requests()); n != 0 {
		t.Errorf("registry: %v", g.reg.Requests())
	}
}

// --dry-run makes no POST, PUT, PATCH or DELETE at Render, no request at the registry, and
// changes nothing at Turso: before the first deploy, no request at Turso but GETs; after it, the
// only other one is the token minted to read the shared database (it changes no data).
func TestRender_DryRun(t *testing.T) {
	g := newRenderRig(t)
	if _, err := g.deploy(g.target(), dryRun); err != nil {
		t.Fatalf("%v\n%s", err, g.out.String())
	}
	if n := nonGET(g.render.Requests()); len(n) != 0 {
		t.Errorf("render: %v", n)
	}
	if n := nonGET(g.reg.Requests()); len(n) != 0 {
		t.Errorf("registry: %v", n)
	}
	if n := nonGET(g.turso.Calls()); len(n) != 0 || len(g.turso.Names()) != 0 {
		t.Errorf("turso: %v %v", n, g.turso.Names())
	}
	if !strings.Contains(g.out.String(), "dry run: nothing was shipped") || !strings.Contains(g.out.String(), "first deploy") {
		t.Errorf("transcript:\n%s", g.out.String())
	}

	id1 := g.mustDeploy(g.target())
	addDrop(t, g.dir)
	nr, ng, nt := len(g.render.Requests()), len(g.reg.Requests()), len(g.turso.Calls())
	names := g.turso.Names()
	rowsBefore := g.rows()
	if _, err := g.deploy(g.target(), dryRun, allowLoss); err != nil {
		t.Fatalf("%v\n%s", err, g.out.String())
	}
	if n := nonGET(g.requests(nr)); len(n) != 0 {
		t.Errorf("render: %v", n)
	}
	if n := nonGET(g.reg.Requests()[ng:]); len(n) != 0 {
		t.Errorf("registry: %v", n)
	}
	for _, c := range nonGET(g.turso.Calls()[nt:]) {
		if c != "POST /databases/blog/auth/tokens" {
			t.Errorf("turso: %s", c)
		}
	}
	if !slices.Equal(names, g.turso.Names()) || migrationsIn(t, g.turso, "blog") != "0001_initial" {
		t.Errorf("turso changed: %v", g.turso.Names())
	}
	if !strings.Contains(g.out.String(), "DROPS DATA: column Post.summary") {
		t.Errorf("no plan:\n%s", g.out.String())
	}
	if g.rows() != rowsBefore || rowsBefore == "" {
		t.Error("the builds rows changed")
	}
	if g.current() != id1 {
		t.Error("current changed")
	}
}

// Every output of every scenario is free of the secrets (each rig checks its own when its test
// ends); this one runs them in sequence in one environment, Status and Logs included, and checks
// that the check itself sees a leak.
func TestRender_NoSecretsInOutput(t *testing.T) {
	g := newRenderRig(t)
	ctx := context.Background()
	g.render.Logs(renderLogLines...)
	id1 := g.mustDeploy(g.target())
	g.mustDeploy(g.target())
	g.render.Script("update_in_progress", "update_failed")
	g.deploy(g.target())
	addDrop(t, g.dir)
	g.deploy(g.target(), dryRun, allowLoss)
	g.mustDeploy(g.target(), allowLoss)
	to, err := Rollback(ctx, g.target(), RollbackOptions{To: id1, RestoreBackup: true, Env: "production", Out: &g.out, Drain: -1})
	g.keep(to, err)
	if err != nil {
		t.Fatal(err)
	}
	var st bytes.Buffer
	if err := Status(ctx, g.connected(), &st); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	if err := g.connected().Logs(ctx, false, &logs); err != nil {
		t.Fatal(err)
	}
	g.texts = append(g.texts, st.String(), logs.String())
	_, err = g.connected().Restore(ctx, "/tmp/app.db")
	g.keep(nil, err)
	t.Setenv("SMTP_URL", "")
	g.render.Fail429Once()
	_, err = g.deploy(g.target())
	g.keep(nil, err)

	vals := g.secretValues()
	if len(vals) != 6 {
		t.Fatalf("%d secret values (the service's ALDER_SECRET missing?)", len(vals))
	}
	if l := leaks(g.everything(), vals); len(l) != 0 {
		t.Errorf("leaked: %v", l)
	}
	if l := leaks(g.everything()+"x"+vals[5]+"x", vals); !slices.Equal(l, []int{5}) {
		t.Errorf("the check misses a leak: %v", l)
	}
}

// Status: the current build, the builds, Turso's size, the point-in-time backup, and the
// service's URL, instance count and live deploy (Info.Lines).
func TestRender_Status(t *testing.T) {
	g := newRenderRig(t)
	id := g.mustDeploy(g.target())
	var w bytes.Buffer
	if err := Status(context.Background(), g.connected(), &w); err != nil {
		t.Fatal(err)
	}
	out := w.String()
	g.texts = append(g.texts, out)
	for _, l := range []string{"Render blog (oregon)\n", "current  " + id + "\n", "database ", "Turso point-in-time", "service  " + g.render.URL + "/svc/blog, 1 instance, live deploy dep-"} {
		if !strings.Contains(out, l) {
			t.Errorf("status lacks %q:\n%s", l, out)
		}
	}
}

// Logs: the last lines, "[instance] message".
func TestRender_Logs(t *testing.T) {
	g := newRenderRig(t)
	g.mustDeploy(g.target())
	g.render.Logs(renderLogLines...)
	var w bytes.Buffer
	if err := g.connected().Logs(context.Background(), false, &w); err != nil {
		t.Fatal(err)
	}
	g.texts = append(g.texts, w.String())
	if w.String() != "[srv-a1] listening on :10000\n[srv-a1] boom: the mail server is unreachable\n" {
		t.Errorf("logs:\n%s", w.String())
	}
	var q string
	for _, c := range g.render.Calls() {
		if c.Path == "/logs" {
			q = c.Query.Get("limit")
		}
	}
	if q != "100" {
		t.Errorf("limit %q", q)
	}
}

// Logs -f: the websocket's frames are printed as they come, until ctx ends (a clean end).
func TestRender_LogsFollow(t *testing.T) {
	g := newRenderRig(t)
	g.mustDeploy(g.target())
	g.render.Logs(renderLogLines...)
	r := g.connected()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var w syncBuffer
	done := make(chan error, 1)
	go func() { done <- r.Logs(ctx, true, &w) }()
	want := "[srv-a1] listening on :10000\n[srv-a1] boom: the mail server is unreachable\n"
	deadline := time.Now().Add(5 * time.Second)
	for w.String() != want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("returned before ctx ended: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("follow ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not end with ctx")
	}
	if w.String() != want {
		t.Errorf("frames:\n%s", w.String())
	}
	g.texts = append(g.texts, w.String())
}

// A file restore is refused: the data is at Turso, restored to a point in time.
func TestRender_RestoreFileRefused(t *testing.T) {
	g := newRenderRig(t)
	_, err := g.target().Restore(context.Background(), "/tmp/app.db")
	var r *Refusal
	if !errors.As(err, &r) || !strings.Contains(err.Error(), "restored to a point in time") {
		t.Fatalf("err = %v", err)
	}
}

// NewRender reads the machine's environment: the API, the registry's user, token and base (R-A),
// and the address before the service exists (R6).
func TestNewRender(t *testing.T) {
	t.Setenv("RENDER_API_KEY", "k")
	t.Setenv("ALDER_RENDER_API", "http://render.test/v1/")
	t.Setenv("ALDER_REGISTRY_TOKEN", "tok")
	t.Setenv("ALDER_REGISTRY_USER", "")
	t.Setenv("ALDER_REGISTRY_BASE", "http://registry.test")
	r := NewRender("blog", "preview", ir.Target{Kind: "render", Region: "ohio", Service: "blog-preview", Database: &ir.Database{Org: "acme", App: "blog-preview"}},
		&ir.Deploy{Requires: []string{"SMTP_URL"}}, rImage)
	if r.API.Base != "http://render.test/v1" || r.API.Key != "k" || r.Reg.Host != "ghcr.io" || r.Reg.Repo != "tavish/blog" || r.Reg.User != "tavish" ||
		r.Reg.Token != "tok" || r.Reg.Base != "http://registry.test" || r.Instances != 1 || r.DB == nil || r.DB.SharedName() != "blog-preview" ||
		!slices.Equal(r.Required, []string{"SMTP_URL"}) || r.Identity() != "render:blog-preview" {
		t.Errorf("%+v", r)
	}
	if r.Address() != "https://blog-preview.onrender.com" || !r.AddressExpected() {
		t.Errorf("address %q", r.Address())
	}
	t.Setenv("ALDER_REGISTRY_USER", "someone")
	if r := NewRender("blog", "preview", ir.Target{Database: &ir.Database{}}, &ir.Deploy{}, rImage); r.Reg.User != "someone" {
		t.Errorf("user %q", r.Reg.User)
	}
}

// A deploy in progress that is not this build's (one Render may start for the environment
// change) is cancelled after the env-var PUT, so the roll Start triggers is the one that counts.
func TestRender_CancelsForeignDeploy(t *testing.T) {
	g := newRenderRig(t)
	ctx := context.Background()
	s := g.render.AddService("blog", rImage+":0000000000000000", 1)
	g.render.Script("update_in_progress")
	api := NewRenderAPI()
	foreign, err := api.CreateDeploy(ctx, s.ID, rImage+":1111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	id := g.mustDeploy(g.target())
	reqs := g.render.Requests()
	put, cancel := slices.Index(reqs, "PUT /services/"+s.ID+"/env-vars"), slices.Index(reqs, "POST /services/"+s.ID+"/deploys/"+foreign.ID+"/cancel")
	if put < 0 || cancel < put {
		t.Errorf("no cancel of %s after the PUT: %v", foreign.ID, nonGET(reqs))
	}
	if !strings.Contains(g.out.String(), "Render deploy "+foreign.ID+" (created) cancelled: not this build's\n") || g.current() != id {
		t.Errorf("transcript:\n%s", g.out.String())
	}
}

// A deploy refused at Migrate (after the image is pushed) changes nothing at Render, though the
// service's environment (a new `requires` value) and its instance count would change: both are
// set by Start, after the migrations.
func TestRender_MigrateRefusalChangesNothingAtRender(t *testing.T) {
	g := newRenderRig(t)
	g.mustDeploy(g.target())
	t.Setenv("SMTP_URL", "smtp://mailer:SECRETNEWPASS@mail.example")
	g.secrets = append(g.secrets, "smtp://mailer:SECRETNEWPASS@mail.example")
	g.instances, g.refuse = 2, true
	nr := len(g.render.Requests())
	res, err := g.deploy(g.target())
	if err == nil || !strings.Contains(err.Error(), "migrations did not apply") {
		t.Fatalf("err = %v\n%s", err, g.out.String())
	}
	if g.reg.Manifest("tavish/blog", res.Record.Build) == nil {
		t.Errorf("the image was not pushed: the refusal came before Upload")
	}
	if n := nonGET(g.requests(nr)); len(n) != 0 {
		t.Errorf("changed at Render before Start: %v", n)
	}
	if s := g.render.Service("blog"); envOf(s)["SMTP_URL"] != rSMTP || s.Instances != 1 {
		t.Errorf("the service changed: %d instances", s.Instances)
	}
}

// Render reports the deploy live but one instance never serves the build: the health check times
// out, nothing is recorded current, and the error carries the last answer's body.
func TestRender_HealthNeverServes(t *testing.T) {
	g := newRenderRig(t)
	g.instances = 2
	id1 := g.mustDeploy(g.target())
	g.render.DelaySwitch("blog", 1)
	res, err := g.deploy(g.target(), func(o *Options) { o.HealthTimeout = 200 * time.Millisecond })
	if err == nil {
		t.Fatalf("deployed:\n%s", g.out.String())
	}
	if !strings.Contains(g.out.String(), " live (2 instances)\n") {
		t.Errorf("Render did not report live:\n%s", g.out.String())
	}
	if !strings.Contains(err.Error(), `200 {"build":"`+id1+`","db":"ok"}`) || !strings.Contains(err.Error(), "did not answer with build "+res.Record.Build) {
		t.Errorf("error: %v", err)
	}
	if g.current() != id1 {
		t.Error("current changed")
	}
	builds, _ := g.connected().Builds(context.Background())
	for _, b := range builds {
		if b.ID == res.Record.Build && (b.Current || b.Record != nil) {
			t.Errorf("recorded: %+v", b)
		}
	}
}
