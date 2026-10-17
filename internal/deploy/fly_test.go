package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/ir"
	"alder/internal/migrate"
	"alder/rt/db"
)

// fakeFly is Fly as the Fly target sees it: the Machines API (an httptest server), flyctl (a
// function), the app's public address (another server), and one machine whose volume is a
// local directory. Booting the machine runs the image's real start script (FlyRunScript) with
// the image's files, so the scripts the target sends and the one the image runs are the ones
// tested; the app's executable is a shell script that migrates (or refuses) and backs up.
type fakeFly struct {
	t        *testing.T
	mu       sync.Mutex
	data     string            // the volume
	images   map[string]string // image label (build id) -> its build context, as pushed
	exists   bool
	volumes  []flyVol
	machine  *flyMachine
	secrets  map[string]string
	ips      int
	broken   map[string]bool // builds that never answer with their id
	requests []string        // every API request: method, path and body
	flyctl   []string        // every flyctl command
	api, pub *httptest.Server
}

func newFakeFly(t *testing.T) *fakeFly {
	ff := &fakeFly{t: t, data: t.TempDir(), images: map[string]string{}, secrets: map[string]string{}, broken: map[string]bool{}}
	ff.api = httptest.NewServer(http.HandlerFunc(ff.serveAPI))
	ff.pub = httptest.NewServer(http.HandlerFunc(ff.servePublic))
	t.Cleanup(func() { ff.api.Close(); ff.pub.Close() })
	return ff
}

// target is a Fly target of app demo, environment staging, talking to the fake.
func (ff *fakeFly) target(env map[string]string) *Fly {
	f := NewFly("demo", "staging", ir.Target{Kind: "fly", Region: "ord", App: "demo-staging"}, &ir.Deploy{})
	f.API, f.PublicURL, f.Token, f.Data = ff.api.URL+"/v1", ff.pub.URL, "fly-token", ff.data
	f.Flyctl = ff.runFlyctl
	f.Getenv = func(n string) string { return env[n] }
	f.Poll, f.MigrateTimeout = 5*time.Millisecond, 5*time.Second
	return f
}

func (ff *fakeFly) servePublic(w http.ResponseWriter, r *http.Request) {
	ff.mu.Lock()
	defer ff.mu.Unlock()
	if ff.machine == nil || ff.machine.State != "started" || ff.broken[ff.machine.build()] {
		http.Error(w, "no machine answers", http.StatusBadGateway)
		return
	}
	fmt.Fprintf(w, `<html><head><meta name="alder-build" content="%s"></head></html>`, ff.machine.build())
}

func (ff *fakeFly) serveAPI(w http.ResponseWriter, r *http.Request) {
	ff.mu.Lock()
	defer ff.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	ff.requests = append(ff.requests, r.Method+" "+r.URL.Path+" "+string(body))
	if r.Header.Get("Authorization") != "Bearer fly-token" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	reply := func(v any) { json.NewEncoder(w).Encode(v) }
	p := strings.TrimPrefix(r.URL.Path, "/v1")
	const app = "/apps/demo-staging"
	switch {
	case r.Method == "POST" && p == "/apps":
		ff.exists = true
		w.WriteHeader(http.StatusCreated)
	case !ff.exists && strings.HasPrefix(p, app):
		http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
	case r.Method == "GET" && p == app:
		reply(map[string]string{"name": "demo-staging"})
	case r.Method == "GET" && p == app+"/volumes":
		reply(ff.volumes)
	case r.Method == "POST" && p == app+"/volumes":
		var v flyVol
		json.Unmarshal(body, &v)
		v.ID = fmt.Sprintf("vol_%d", len(ff.volumes)+1)
		ff.volumes = append(ff.volumes, v)
		reply(v)
	case r.Method == "GET" && p == app+"/machines":
		var ms []flyMachine
		if ff.machine != nil {
			ms = append(ms, *ff.machine)
		}
		reply(ms)
	case r.Method == "POST" && p == app+"/machines":
		var m flyMachine
		json.Unmarshal(body, &m)
		m.ID = "m1"
		ff.machine = &m
		ff.boot()
		reply(ff.machine)
	case ff.machine == nil:
		http.Error(w, "no such machine", http.StatusNotFound)
	case r.Method == "GET" && p == app+"/machines/m1":
		reply(ff.machine)
	case r.Method == "POST" && p == app+"/machines/m1":
		var m flyMachine
		json.Unmarshal(body, &m)
		ff.machine.Config = m.Config
		ff.boot()
		reply(ff.machine)
	case r.Method == "POST" && (p == app+"/machines/m1/start" || p == app+"/machines/m1/restart"):
		ff.boot()
	case r.Method == "POST" && p == app+"/machines/m1/stop":
		ff.machine.State = "stopped"
	case r.Method == "POST" && p == app+"/machines/m1/exec":
		if ff.machine.State != "started" {
			http.Error(w, "machine not started", http.StatusPreconditionFailed)
			return
		}
		var req struct{ Command []string }
		json.Unmarshal(body, &req)
		cmd := exec.Command(req.Command[0], req.Command[1:]...)
		cmd.Env = append(os.Environ(), "ALDER_APP_DIR="+ff.images[ff.machine.build()], "ALDER_DB="+ff.data+"/db/app.db")
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		code := 0
		if err := cmd.Run(); err != nil {
			code = 1
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
		reply(execResult{ExitCode: code, Stdout: out.String(), Stderr: errb.String()})
	default:
		http.Error(w, "the fake does not know "+r.Method+" "+p, http.StatusNotFound)
	}
}

// boot runs the machine's image: its start script, with the volume and the image's files. The
// fake executable exits at once instead of serving; the machine is "started" when the script
// got as far as serving (its migrations applied), else "stopped".
func (ff *fakeFly) boot() {
	id := ff.machine.build()
	ctxDir, ok := ff.images[id]
	if !ok || ff.machine.Config.Image != "registry.fly.io/demo-staging:"+id {
		ff.machine.State = "failed"
		return
	}
	cmd := exec.Command("sh", filepath.Join(ctxDir, "run"))
	cmd.Env = append(os.Environ(), "ALDER_DATA="+ff.data, "ALDER_APP_DIR="+ctxDir, "ALDER_BUILD="+id, "ALDER_DB="+ff.data+"/db/app.db")
	if out, err := cmd.CombinedOutput(); err != nil {
		ff.t.Errorf("the start script failed: %v\n%s", err, out)
	}
	st, _ := os.ReadFile(filepath.Join(ff.data, "builds", id, "migrate.status"))
	ff.machine.State = map[bool]string{true: "started", false: "stopped"}[strings.TrimSpace(string(st)) == "ok"]
}

func (ff *fakeFly) runFlyctl(_ context.Context, stdin io.Reader, args ...string) (string, error) {
	ff.mu.Lock()
	defer ff.mu.Unlock()
	ff.flyctl = append(ff.flyctl, strings.Join(args, " "))
	switch {
	case len(args) >= 2 && args[0] == "secrets" && args[1] == "list":
		var list []map[string]string
		for n := range ff.secrets {
			list = append(list, map[string]string{"Name": n, "Digest": "x"})
		}
		b, _ := json.Marshal(list)
		return string(b), nil
	case len(args) >= 2 && args[0] == "secrets" && args[1] == "import":
		b, _ := io.ReadAll(stdin)
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			n, v, _ := strings.Cut(l, "=")
			ff.secrets[n] = v
		}
		return "", nil
	case len(args) >= 2 && args[0] == "ips" && args[1] == "list":
		if ff.ips == 0 {
			return "[]", nil
		}
		return `[{"Address":"1.2.3.4"}]`, nil
	case len(args) >= 2 && args[0] == "ips":
		ff.ips++
		return "", nil
	case args[0] == "deploy":
		label := args[len(args)-1]
		dst := filepath.Join(ff.t.TempDir(), "image")
		if err := copyTree(args[1], dst); err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(dst, "Dockerfile")); err != nil {
			return "", errors.New("no Dockerfile in the build context")
		}
		os.Chmod(filepath.Join(dst, "app"), 0o755)
		ff.images[label] = dst
		return "", nil
	case args[0] == "logs":
		return "alder: listening on :8080\n", nil
	}
	return "", fmt.Errorf("the fake flyctl does not know %v", args)
}

// flyApp is an app directory with one migration (Post, with a summary).
func flyApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	os.MkdirAll(mig, 0o755)
	os.MkdirAll(filepath.Join(dir, "build", "static"), 0o755)
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

// fakeApp is the executable of a build: variant makes each build's id its own; refuse makes
// its --migrate-only refuse.
func fakeApp(variant string, refuse bool) string {
	mig := `[ -f "$ALDER_DB" ] || { echo "migrated 0001_initial"; echo db > "$ALDER_DB"; }`
	if refuse {
		mig = `echo "alder: migration 0002 refused: the table Post has rows"; exit 1`
	}
	return `#!/bin/sh
# ` + variant + `
case "$1" in
--migrate-only) ` + mig + ` ;;
--backup-to) cp "$ALDER_DB" "$2" ;;
esac
exit 0
`
}

func flyOptions(t *testing.T, dir string, out io.Writer, variant string, refuse bool) Options {
	o := options(t, dir, out)
	o.Env, o.App = "staging", "demo"
	o.Decl = &ir.Deploy{Requires: []string{"SMTP_URL"}}
	o.HealthTimeout = 300 * time.Millisecond
	o.Binary = func(p Platform, path string) error {
		if p.String() != "linux/amd64" {
			t.Errorf("built for %s", p)
		}
		return os.WriteFile(path, []byte(fakeApp(variant, refuse)), 0o755)
	}
	return o
}

const flySecretValue = "smtp://mailer:hunter2-very-secret@mail.example.com"

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The Fly target, deploy by deploy: the first creates the app, its addresses, the volume and the
// machine from the pushed image and sets the secrets from this machine's environment (no value
// anywhere else); a build that fails its health check, and one whose migrations refuse, are
// replaced by the previous image; a rollback puts an older image back; status reads the state
// on the volume; a missing `requires` value refuses before anything is pushed; a restore
// replaces the database at the next start.
func TestFly_DeployLifecycle(t *testing.T) {
	ff := newFakeFly(t)
	dir := flyApp(t)
	env := map[string]string{"SMTP_URL": flySecretValue, "ALDER_SECRET": strings.Repeat("k", 32)}
	ctx := context.Background()
	deployOnce := func(variant string, refuse bool) (Result, string, error) {
		var out bytes.Buffer
		res, err := Deploy(ctx, flyOptions(t, dir, &out, variant, refuse), GateResult{Check: "ok"}, ff.target(env), nil)
		return res, out.String(), err
	}

	// the first deploy
	res, out, err := deployOnce("one", false)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	id1 := res.Record.Build
	for _, want := range []string{"  requires ok  SMTP_URL, ALDER_SECRET\n", "first deploy", "  migrate  ok  applied 0001_initial\n",
		"  start    ok  port 8080\n", "  health   ok  build " + id1 + " answers GET /\n", "deployed " + id1 + " to staging\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("first deploy: no %q in\n%s", want, out)
		}
	}
	m := ff.machine
	if m == nil || m.Config.Image != "registry.fly.io/demo-staging:"+id1 || len(m.Config.Mounts) != 1 || m.Config.Mounts[0].Path != ff.data ||
		m.Config.Mounts[0].Volume != "vol_1" || m.Config.Env["ALDER_DB"] != ff.data+"/db/app.db" || m.Config.Metadata["alder"] != "demo/staging" ||
		m.Config.Services[0].InternalPort != FlyPort {
		t.Errorf("machine: %+v", m)
	}
	if ff.secrets["SMTP_URL"] != flySecretValue || ff.secrets["ALDER_SECRET"] != env["ALDER_SECRET"] || ff.ips != 2 || len(ff.volumes) != 1 {
		t.Errorf("secrets %v, ips %d, volumes %v", ff.secrets, ff.ips, ff.volumes)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(ff.data, "current"))); got != id1 {
		t.Errorf("current = %q", got)
	}
	if !strings.Contains(readFile(t, filepath.Join(ff.data, "builds", id1, "deployed.json")), `"by": "fly:demo-staging (local ada)"`) {
		t.Errorf("deployed.json:\n%s", readFile(t, filepath.Join(ff.data, "builds", id1, "deployed.json")))
	}
	ctxDir := ff.images[id1]
	if !strings.Contains(readFile(t, filepath.Join(ctxDir, "Dockerfile")), "ENV ALDER_BUILD="+id1) || !strings.Contains(readFile(t, filepath.Join(ctxDir, "fly.toml")), `app = "demo-staging"`) {
		t.Errorf("the build context: %v", ctxDir)
	}
	pushed := false
	for _, c := range ff.flyctl {
		pushed = pushed || strings.HasPrefix(c, "deploy ") && strings.HasSuffix(c, "--build-only --push --remote-only --image-label "+id1)
	}
	if !pushed {
		t.Errorf("flyctl: %q", ff.flyctl)
	}

	// a second build, and a third: each replaces the image, after a backup
	res, out, err = deployOnce("two", false)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	id2 := res.Record.Build
	res, out, err = deployOnce("three", false)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	id3 := res.Record.Build
	if !strings.Contains(out, "  backup   ok  builds/"+id3+"/backup.db\n") || !strings.Contains(out, "  migrate  ok  nothing to apply\n") {
		t.Errorf("third deploy:\n%s", out)
	}

	// a build that never answers with its id: the previous image is put back
	var o4 bytes.Buffer
	opts := flyOptions(t, dir, &o4, "four", false)
	tg := ff.target(env)
	// break build four before its health check: its id is known once the artifact is made
	opts.Binary = func(p Platform, path string) error {
		b := []byte(fakeApp("four", false))
		ff.mu.Lock()
		ff.broken[fileIDBytes(b)] = true
		ff.mu.Unlock()
		return os.WriteFile(path, b, 0o755)
	}
	res, err = Deploy(ctx, opts, GateResult{Check: "ok"}, tg, nil)
	id4 := res.Record.Build
	if err == nil || !strings.Contains(err.Error(), "build "+id4+" did not pass its health check") || !strings.Contains(err.Error(), "build "+id3+" still serves") ||
		!strings.Contains(err.Error(), "alder: listening on :8080") {
		t.Fatalf("a broken build: %v\n%s", err, o4.String())
	}
	if ff.machine.build() != id3 || ff.machine.State != "started" || strings.TrimSpace(readFile(t, filepath.Join(ff.data, "current"))) != id3 {
		t.Errorf("after a failed health check the machine runs %s (%s)", ff.machine.build(), ff.machine.State)
	}

	// migrations that refuse: the previous image is put back, the refusal shown
	res, out, err = deployOnce("five", true)
	if err == nil || !strings.Contains(err.Error(), "migrations did not apply") || !strings.Contains(err.Error(), "migration 0002 refused") {
		t.Fatalf("a refusing migration: %v\n%s", err, out)
	}
	if ff.machine.build() != id3 || ff.machine.State != "started" {
		t.Errorf("after a refused migration the machine runs %s (%s)", ff.machine.build(), ff.machine.State)
	}

	// status
	var st bytes.Buffer
	if err := Status(ctx, ff.target(env), &st); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Fly ord (app demo-staging)\n", "current  " + id3 + "\n", "  * " + id3 + "  ", "running", "never switched to", "database 3 bytes\n", "backup   "} {
		if !strings.Contains(st.String(), want) {
			t.Errorf("status: no %q in\n%s", want, st.String())
		}
	}

	// rollback: the previous image
	var rb bytes.Buffer
	to, err := Rollback(ctx, ff.target(env), RollbackOptions{Out: &rb, HealthTimeout: time.Second, Drain: -1})
	if err != nil || to != id2 {
		t.Fatalf("rollback to %q: %v\n%s", to, err, rb.String())
	}
	if ff.machine.Config.Image != "registry.fly.io/demo-staging:"+id2 || strings.TrimSpace(readFile(t, filepath.Join(ff.data, "current"))) != id2 {
		t.Errorf("after the rollback the machine runs %s", ff.machine.Config.Image)
	}

	// a `requires` name neither here nor at Fly
	pushes := len(ff.images)
	var o6 bytes.Buffer
	opts = flyOptions(t, dir, &o6, "six", false)
	opts.Decl.Requires = []string{"SMTP_URL", "STRIPE_KEY"}
	_, err = Deploy(ctx, opts, GateResult{Check: "ok"}, ff.target(env), nil)
	if err == nil || !strings.Contains(err.Error(), "has no secret STRIPE_KEY") || len(ff.images) != pushes {
		t.Errorf("a missing name: %v (images %d → %d)", err, pushes, len(ff.images))
	}
	// …set at Fly already: not missing, even with no value here
	ff.secrets["STRIPE_KEY"] = "sk_live_x"
	if missing, err := ff.target(env).connected(t).Requires(ctx, []string{"STRIPE_KEY"}); err != nil || len(missing) != 0 {
		t.Errorf("a name Fly has: %v %v", missing, err)
	}

	// restore: the start script puts the file in place, the old one beside it
	local := filepath.Join(t.TempDir(), "restored.db")
	os.WriteFile(local, bytes.Repeat([]byte("r"), restoreChunk*2+10), 0o600)
	if _, err := ff.target(env).Restore(ctx, local); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(ff.data, "db", "app.db")); len(got) != restoreChunk*2+10 {
		t.Errorf("restored database: %d bytes", len(got))
	}
	olds, _ := filepath.Glob(filepath.Join(ff.data, "db", "app.db.before-restore-*"))
	if len(olds) != 1 {
		t.Errorf("the previous database kept: %v", olds)
	}

	// no value of the environment in any API request, any build context, or the output
	for _, r := range ff.requests {
		if strings.Contains(r, "hunter2") {
			t.Errorf("a secret value in an API request: %s", r)
		}
	}
	for _, d := range ff.images {
		filepath.WalkDir(d, func(p string, _ os.DirEntry, _ error) error {
			if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte("hunter2")) {
				t.Errorf("a secret value in the image: %s", p)
			}
			return nil
		})
	}
}

// --allow-data-loss reaches the machine's --migrate-only as ALDER_ALLOW_DATA_LOSS=1, and only
// on that deploy's configuration.
func TestFly_AllowDataLoss(t *testing.T) {
	f := NewFly("demo", "staging", ir.Target{Kind: "fly", Region: "ord", App: "demo-staging"}, &ir.Deploy{})
	if _, ok := f.config("0123456789abcdef", "vol_1").Env["ALDER_ALLOW_DATA_LOSS"]; ok {
		t.Error("ALDER_ALLOW_DATA_LOSS without the flag")
	}
	f.AllowDataLoss(true)
	if f.config("0123456789abcdef", "vol_1").Env["ALDER_ALLOW_DATA_LOSS"] != "1" {
		t.Error("no ALDER_ALLOW_DATA_LOSS with the flag")
	}
}

// connected is f after Connect.
func (f *Fly) connected(t *testing.T) *Fly {
	t.Helper()
	if _, err := f.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}

func fileIDBytes(b []byte) string {
	p := filepath.Join(os.TempDir(), fmt.Sprintf("alder-fly-id-%d", time.Now().UnixNano()))
	os.WriteFile(p, b, 0o644)
	defer os.Remove(p)
	id, _ := fileID(p)
	return id
}

// Connect refuses without the token and without flyctl, naming each; a dry run changes
// nothing at Fly.
func TestFly_Refusals(t *testing.T) {
	ff := newFakeFly(t)
	f := ff.target(nil)
	f.Token = ""
	if _, err := f.Connect(context.Background()); err == nil || !strings.Contains(err.Error(), "FLY_API_TOKEN") {
		t.Errorf("no token: %v", err)
	}
	f = ff.target(nil)
	f.Flyctl = nil
	t.Setenv("PATH", t.TempDir())
	if _, err := f.Connect(context.Background()); err == nil || !strings.Contains(err.Error(), "with `flyctl`, which is not on PATH: install it (https://fly.io/docs/flyctl/install/)") {
		t.Errorf("no flyctl: %v", err)
	}

	var out bytes.Buffer
	o := flyOptions(t, flyApp(t), &out, "dry", false)
	o.DryRun = true
	env := map[string]string{"SMTP_URL": flySecretValue, "ALDER_SECRET": strings.Repeat("k", 32)}
	if _, err := Deploy(context.Background(), o, GateResult{Check: "ok"}, ff.target(env), nil); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	for _, r := range ff.requests {
		if !strings.HasPrefix(r, "GET ") {
			t.Errorf("a dry run sent %s", r)
		}
	}
	if len(ff.images) != 0 || ff.exists || !strings.Contains(out.String(), "dry run: nothing was shipped") {
		t.Errorf("a dry run pushed or created: %v %v\n%s", ff.images, ff.exists, out.String())
	}
}

// The image's start script and the scripts the target runs parse; the Dockerfile copies the
// artifact and names the build; a token from `fly tokens create` is sent as it is.
func TestFly_Scripts(t *testing.T) {
	for name, s := range map[string]string{"run": FlyRunScript("demo", "staging")} {
		cmd := exec.Command("sh", "-n")
		cmd.Stdin = strings.NewReader(s)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", name, err, out)
		}
	}
	d := FlyDockerfile("demo", "0123456789abcdef")
	for _, want := range []string{"FROM alpine:", "COPY app /app/app\n", "COPY static /app/static\n", "COPY migrations /app/migrations\n", "ENV ALDER_BUILD=0123456789abcdef\n", `CMD ["/app/run"]`} {
		if !strings.Contains(d, want) {
			t.Errorf("Dockerfile: no %q", want)
		}
	}
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		http.NotFound(w, r)
	}))
	defer srv.Close()
	f := &Fly{API: srv.URL, Token: "FlyV1 fm2_abc", FlyApp: "x"}
	f.call(context.Background(), "GET", "/apps/x", nil, nil)
	if auth != "FlyV1 fm2_abc" {
		t.Errorf("Authorization: %q", auth)
	}
}
