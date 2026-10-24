package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/deploy"
	"alder/internal/ir"
)

// The Ssh target behind Caddy (`proxy: Caddy`, the default), against the box of
// deploy_ssh_test.go and a fake Caddy (testdata/deploy/fakecaddy: the admin API calls the target
// makes, and a public listener that proxies to the last route's upstream), with an availability
// monitor that asks the public listener which build answers, every 100 ms. With Caddy the
// previous build keeps running for the drain time after a switch, which `proxy: None` never
// does: these are the tests of that window.

// fakeCaddyDir is the fake Caddy's source, found before a test changes the working directory.
var fakeCaddyDir, _ = filepath.Abs(filepath.Join("testdata", "deploy", "fakecaddy"))

// startFakeCaddy builds the fake Caddy and runs it at box until the test ends, and returns the
// URL this process reaches its public listener at and the address of its admin API at the box
// (the target's deployEnv.CaddyAdmin).
func startFakeCaddy(t *testing.T, box *sshBox) (string, string) {
	t.Helper()
	goos := runtime.GOOS
	if box.container != "" {
		goos = "linux"
	}
	bin := filepath.Join(t.TempDir(), "fakecaddy")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = fakeCaddyDir
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+runtime.GOARCH)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the fake Caddy: %v\n%s", err, out)
	}
	if box.container != "" {
		if out, err := exec.Command("docker", "cp", bin, box.container+":/usr/local/bin/fakecaddy").CombinedOutput(); err != nil {
			t.Fatalf("docker cp: %v %s", err, out)
		}
		if out, err := exec.Command("docker", "exec", "-d", box.container, "/usr/local/bin/fakecaddy", deploy.DefaultCaddyAdmin, "0.0.0.0:18443").CombinedOutput(); err != nil {
			t.Fatalf("docker exec: %v %s", err, out)
		}
		u := fmt.Sprintf("http://127.0.0.1:%d", box.caddyPort)
		waitHTTP(t, u)
		return u, deploy.DefaultCaddyAdmin
	}
	// the box is this machine (the loopback box, or the private sshd): a process of the test
	admin := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	public := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cmd := exec.Command(bin, admin, public)
	logFile, _ := os.Create(filepath.Join(t.TempDir(), "fakecaddy.log"))
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait(); logFile.Close() })
	u := "http://" + public
	waitHTTP(t, u)
	return u, admin
}

// waitHTTP waits until something answers at u (any status).
func waitHTTP(t *testing.T, u string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(u)
		if err == nil {
			resp.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("nothing answers at %s: %v", u, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

var buildMeta = regexp.MustCompile(`name="alder-build" content="([0-9a-f]+)"`)

// pageBuild is the build id the page at u names, or "DOWN".
func pageBuild(client *http.Client, u string) string {
	resp, err := client.Get(u + "/")
	if err != nil {
		return "DOWN"
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if m := buildMeta.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return "DOWN"
}

// monitor asks a public address which build answers, every 100 ms, until the test ends: the
// availability record of a deploy, a rollback or a restore. client is the HTTP client it asks
// with (nil: a plain one with a 2 s timeout); an https address with a certificate this process
// does not trust (Caddy's local CA for localhost) needs one that skips verification, or every
// sample is DOWN.
type monitor struct {
	mu      sync.Mutex
	samples []sample
}

type sample struct {
	at    time.Time
	build string
}

func startMonitor(t *testing.T, u string, client *http.Client) *monitor {
	m := &monitor{}
	stop := make(chan struct{})
	done := make(chan struct{})
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	go func() {
		defer close(done)
		for {
			b := pageBuild(client, u)
			m.mu.Lock()
			m.samples = append(m.samples, sample{time.Now(), b})
			m.mu.Unlock()
			select {
			case <-stop:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-done })
	return m
}

// since is the builds the monitor saw from t0 on, one per sample.
func (m *monitor) since(t0 time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, s := range m.samples {
		if !s.at.Before(t0) {
			out = append(out, s.build)
		}
	}
	return out
}

// dropped is how many samples from t0 on named neither build a nor build b (DOWN, mostly).
func (m *monitor) dropped(t0 time.Time, a, b string) int {
	n := 0
	for _, s := range m.since(t0) {
		if s != a && s != b {
			n++
		}
	}
	return n
}

// summary is the monitor's record from t0 on, runs collapsed: "a×12 DOWN×3 b×40".
func (m *monitor) summary(t0 time.Time) string {
	var parts []string
	prev, n := "", 0
	for _, b := range append(m.since(t0), "") {
		if b == prev {
			n++
			continue
		}
		if prev != "" {
			parts = append(parts, fmt.Sprintf("%s×%d", prev, n))
		}
		prev, n = b, 1
	}
	return strings.Join(parts, " ")
}

// sshScaffoldCaddy is sshScaffold with `proxy: Caddy` (the default) and the fake Caddy: it
// returns the app's directory, the URL of the fake Caddy's public listener and the deploy
// commands pointed at its admin API.
func sshScaffoldCaddy(t *testing.T, box *sshBox) (string, string, *deployCmds) {
	t.Helper()
	public, admin := startFakeCaddy(t, box)
	dir := deployScaffold(t, fmt.Sprintf(`deploy {
  production: Ssh %q { domain: "localhost", path: %q },
  requires: ["SMTP_URL"],
  keep: 4,
}`, box.host, box.root), nil)
	c := box.cmds(t)
	c.env.Drain, c.env.HealthTimeout, c.env.CaddyAdmin = -1, 20*time.Second, admin // -1: no drain
	box.sh(t, fmt.Sprintf("mkdir -p %s && printf 'ALDER_SECRET=%s\\nSMTP_URL=smtp://mail\\n' > %s/env", box.qr(), strings.Repeat("s", 32), box.qr()))
	return dir, public, c
}

// postVia posts an entry through the public address u.
func postVia(t *testing.T, u, name string) {
	t.Helper()
	req, _ := http.NewRequest("POST", u+"/_m/Home/Submit", strings.NewReader(url.Values{"name": {name}, "message": {"hello"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", u)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", name, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.Fatalf("POST %s: %s", name, resp.Status)
	}
}

// namesVia is the entries the page at u lists, newest first, comma-separated.
func namesVia(t *testing.T, u string) string {
	t.Helper()
	resp, err := http.Get(u + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var got []string
	for _, m := range regexp.MustCompile(`<li><b>([^<]*)</b>`).FindAllSubmatch(b, -1) {
		got = append(got, string(m[1]))
	}
	return strings.Join(got, ",")
}

// dbNames is the entries in <root>/db/app.db at the box (the file and its WAL, copied here),
// sorted: what is in the database file itself, whichever process wrote it.
func dbNames(t *testing.T, box *sshBox) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"app.db", "app.db-wal"} {
		b, err := exec.Command(box.bin, "-F", box.config, box.host, fmt.Sprintf("cat %s/db/%s 2>/dev/null || true", box.qr(), f)).Output()
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 0 {
			writeFile(t, filepath.Join(dir, f), string(b))
		}
	}
	conn, err := sql.Open("sqlite", filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	rows, err := conn.Query(`SELECT name FROM "Entry__Entry" ORDER BY name`)
	if err != nil {
		t.Fatalf("reading app.db: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	return strings.Join(names, ",")
}

// runningBuilds is the builds whose process runs at the box.
func runningBuilds(t *testing.T, s *deploy.Ssh) []string {
	t.Helper()
	builds, err := s.Builds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, b := range builds {
		if b.Running {
			ids = append(ids, b.ID)
		}
	}
	return ids
}

// C1: `rollback --restore-backup` inside the drain window. The previous build is still running
// (its delayed stop pending) when the rollback to it puts its pre-migration copy back: every
// build is stopped first and the build started fresh on the copy, so the site serves the
// restored database (a POST lands in app.db itself) and only that build runs. I5: without --yes
// and without a terminal it is refused. I3, I4, I2: `restore` routes the build again through
// Caddy, leaves the restored file 600, and a build that refuses the restored file brings the
// replaced database back, naming where the refused file is kept.
func TestDeploySsh_CaddyDrainRestore(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	box := startBox(t)
	dir, public, c := sshScaffoldCaddy(t, box)
	code, out, errs := c.deploy("production", dir)
	if code != 0 {
		t.Fatalf("deploy: %d\n%s%s", code, out, errs)
	}
	id1 := deployedID(t, out)
	if got := pageBuild(http.DefaultClient, public); got != id1 {
		t.Fatalf("through Caddy: %s, want %s\n%s", got, id1, out)
	}
	// the same build again, with Caddy's route pointing elsewhere: "already serves" checks the
	// route reaches the build, and sets it again
	box.sh(t, `curl -s -o /dev/null -X PATCH -H 'Content-Type: application/json' -d '{"dial":"127.0.0.1:1"}' http://`+c.env.CaddyAdmin+`/id/alder-demo-production`)
	if got := pageBuild(http.DefaultClient, public); got != "DOWN" {
		t.Fatalf("the broken route still reaches %s", got)
	}
	if code, out, errs = c.deploy("production", dir); code != 0 || !regexp.MustCompile(`build `+id1+` already serves production \(port \d+; health and route checked\): nothing to ship`).MatchString(out) {
		t.Fatalf("deploying the build that serves: %d\n%s%s", code, out, errs)
	}
	if got := pageBuild(http.DefaultClient, public); got != id1 {
		t.Fatalf("after the redeploy Caddy reaches %s, want %s", got, id1)
	}
	postVia(t, public, "Ada")
	migrateDemo(t, dir, "  message: Text.NonEmpty,\n", "  message: Text.NonEmpty,\n  mood: Maybe Text = Nothing,\n", "add_mood")
	c.env.Drain = 40 * time.Second // id1 keeps running, draining, past the rollback below
	if code, out, errs = c.deploy("production", dir); code != 0 {
		t.Fatalf("second deploy: %d\n%s%s", code, out, errs)
	}
	id2 := deployedID(t, out)
	if !strings.Contains(out, "  stop     ok  build "+id1+" stops after 40s") {
		t.Fatalf("no drain window:\n%s", out)
	}
	postVia(t, public, "Grace")
	s := newSshTarget(c.env, "demo", "production", ir.Target{Kind: "ssh", Host: box.host, Path: box.root, Domain: "localhost"}, &ir.Deploy{})
	ctx := context.Background()
	if _, err := s.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := strings.Join(runningBuilds(t, s), ","); !strings.Contains(got, id1) || !strings.Contains(got, id2) {
		t.Fatalf("both builds should run inside the drain window: %s", got)
	}

	// I5: not confirmed without a terminal
	var so, se strings.Builder
	if code := c.rollback([]string{"production", id1, "--dir", dir, "--restore-backup"}, &so, &se); code != 2 || !strings.Contains(se.String(), "not confirmed: nothing was changed") {
		t.Fatalf("rollback --restore-backup without --yes: %d\n%s%s", code, so.String(), se.String())
	}
	if got := dbNames(t, box); got != "Ada,Grace" {
		t.Fatalf("the refused rollback changed the database: %s", got)
	}

	// C1: inside the window, onto the draining build, with its copy
	mon := startMonitor(t, public, nil)
	so.Reset()
	se.Reset()
	if code := c.rollback([]string{"production", id1, "--dir", dir, "--restore-backup", "--yes"}, &so, &se); code != 0 {
		t.Fatalf("rollback %s --restore-backup: %d\n%s%s", id1, code, so.String(), se.String())
	}
	after := time.Now()
	if got := strings.Join(runningBuilds(t, s), ","); got != id1 {
		t.Errorf("after the rollback the builds running are %q, want only %s", got, id1)
	}
	if got := namesVia(t, public); got != "Ada" {
		t.Errorf("through Caddy after the rollback: %q, want Ada (the copy from before the migration)", got)
	}
	// where lsof is: the only process with app.db open is the build rolled back to
	if lsof := box.sh(t, fmt.Sprintf("command -v lsof >/dev/null && { lsof -t %s/db/app.db | sort -u | tr '\\n' ' '; echo; cat %s/builds/%s/pid; } || true", box.qr(), box.qr(), id1)); strings.TrimSpace(lsof) != "" {
		lines := strings.Split(strings.TrimSpace(lsof), "\n")
		if len(lines) != 2 || strings.TrimSpace(lines[0]) != strings.TrimSpace(lines[1]) {
			t.Errorf("processes with app.db open (lsof -t), then %s's pid:\n%s", id1, lsof)
		}
	}
	postVia(t, public, "Edsger")
	if got := dbNames(t, box); got != "Ada,Edsger" {
		t.Errorf("app.db holds %q, want Ada,Edsger: the build serving does not write the restored file", got)
	}
	// the monitor's samples since the rollback, 1.5 s of them (one each 100 ms) at least
	pollUntil(10*time.Second, func() bool { return len(mon.since(after)) >= 15 })
	for _, b := range mon.since(after) {
		if b != id1 {
			t.Errorf("after the rollback the site answered %s: %s", b, mon.summary(after))
			break
		}
	}

	// I3, I4: restore puts a file in place, routes the build through Caddy again, mode 600
	copyFile := filepath.Join(t.TempDir(), "copy.db")
	writeFile(t, copyFile, box.sh(t, fmt.Sprintf("cat %s/builds/%s/backup.db", box.qr(), id2))) // Ada only
	postVia(t, public, "Ken")
	kept, err := s.Restore(ctx, copyFile)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !strings.HasPrefix(kept, "db/app.db.before-restore-") {
		t.Errorf("kept %q", kept)
	}
	if got := namesVia(t, public); got != "Ada" {
		t.Errorf("through Caddy after the restore: %q, want Ada", got)
	}
	if got := pageBuild(http.DefaultClient, public); got != id1 {
		t.Errorf("through Caddy after the restore: build %s, want %s", got, id1)
	}
	if mode := box.sh(t, fmt.Sprintf("ls -l %s/db/app.db | cut -c1-10", box.qr())); mode != "-rw-------\n" {
		t.Errorf("the restored database's mode is %q", mode)
	}
	postVia(t, public, "Barbara")
	if got := dbNames(t, box); got != "Ada,Barbara" {
		t.Errorf("app.db after the restore holds %q", got)
	}

	// I2: a file the build refuses: undone, the site back on the replaced database
	bad := filepath.Join(t.TempDir(), "bad.db")
	writeFile(t, bad, strings.Repeat("not a database\n", 400))
	_, err = s.Restore(ctx, bad)
	if err == nil || !regexp.MustCompile(`the restored file is kept as db/app\.db\.refused-\d{8}T\d{6}Z, and the database it replaced \(db/app\.db\.before-restore-\d{8}T\d{6}Z\) is back in place as db/app\.db; build `+id1+` serves again`).MatchString(err.Error()) {
		t.Fatalf("restoring a bad file: %v", err)
	}
	if got := namesVia(t, public); got != "Barbara,Ada" {
		t.Errorf("through Caddy after the refused restore: %q, want Barbara,Ada", got)
	}
	if got := strings.Join(runningBuilds(t, s), ","); got != id1 {
		t.Errorf("after the refused restore the builds running are %q", got)
	}
}
