package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/ir"
	"alder/internal/tursofake"
)

func TestNewSsh(t *testing.T) {
	d := &ir.Deploy{Proxy: "None", Backup: &ir.Backup{Kind: "s3", URL: "s3://bucket/blog"}}
	for _, c := range []struct {
		t    ir.Target
		dest string
	}{
		{ir.Target{Host: "deploy@box.example.com", Path: "/srv/blog/"}, "deploy@box.example.com"},
		{ir.Target{Host: "box.example.com", User: "ops", Path: "/srv/blog"}, "ops@box.example.com"},
		{ir.Target{Host: "deploy@box.example.com", User: "ops", Path: "/srv/blog"}, "ops@box.example.com"},
	} {
		s := NewSsh("blog", "production", c.t, d)
		if s.Dest != c.dest || s.Root != "/srv/blog" || s.Proxy != "None" || s.BackupURL != "s3://bucket/blog" {
			t.Errorf("%+v → %+v", c.t, s)
		}
	}
	if s := NewSsh("my blog", "pr.12", ir.Target{Host: "h", Path: "/p"}, &ir.Deploy{}); s.unit() != "alder-my-blog-pr-12" || s.Proxy != "Caddy" {
		t.Errorf("unit %q proxy %q", s.unit(), s.Proxy)
	}
}

// The target's ssh settings are its fields, not this process's environment: SSHConfig is every
// ssh's -F, SSH the executable, CaddyAdmin DefaultCaddyAdmin unless set.
func TestSsh_SettingsAreFields(t *testing.T) {
	t.Parallel()
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: "/srv/blog"}, &ir.Deploy{})
	if s.CaddyAdmin != DefaultCaddyAdmin || s.ssh() != "ssh" || strings.Contains(strings.Join(s.baseArgs(), " "), "-F") {
		t.Errorf("defaults: admin %q, ssh %q, args %v", s.CaddyAdmin, s.ssh(), s.baseArgs())
	}
	s.SSH, s.SSHConfig = "/opt/ssh", "/etc/alder/ssh_config"
	if args := strings.Join(s.baseArgs(), " "); s.ssh() != "/opt/ssh" || !strings.Contains(args, "-F /etc/alder/ssh_config") {
		t.Errorf("set: ssh %q, args %s", s.ssh(), args)
	}
}

// q quotes for the remote shell: sh reads back exactly what was quoted.
func TestQuote(t *testing.T) {
	for _, s := range []string{"plain", "it's", "a b\tc", `$HOME "x" \n`, "'"} {
		out, err := exec.Command("sh", "-c", "printf %s "+q(s)).Output()
		if err != nil || string(out) != s {
			t.Errorf("q(%q): sh printed %q (%v)", s, out, err)
		}
	}
}

// The run script sources the operator's env file at the target and carries no value of it;
// the backup location is the declaration's.
func TestRunScript(t *testing.T) {
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: "/srv/blog"}, &ir.Deploy{Backup: &ir.Backup{URL: "s3://bucket/blog"}})
	r := s.runScript("0123456789abcdef")
	for _, want := range []string{"set -a; . '/srv/blog/env'; set +a", "export ALDER_DB='/srv/blog/db/app.db'", "export ALDER_STATIC='/srv/blog/builds/0123456789abcdef/static'",
		"export ALDER_BACKUP_URL='s3://bucket/blog'", `exec '/srv/blog/builds/0123456789abcdef/app' "$@"`} {
		if !strings.Contains(r, want) {
			t.Errorf("the run script lacks %q:\n%s", want, r)
		}
	}
}

// The run script defaults ALDER_PUBLIC_URL to https://<domain> (the --host host, which NewSsh
// gets as the domain) after the operator's env file, so a value there wins; a wildcard domain
// without a host writes no default.
func TestRunScript_PublicURL(t *testing.T) {
	const line = `: "${ALDER_PUBLIC_URL:=https://blog.example.com}"; export ALDER_PUBLIC_URL`
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: "/srv/blog", Domain: "blog.example.com"}, &ir.Deploy{})
	r := s.runScript("0123456789abcdef")
	if i, j := strings.Index(r, line), strings.Index(r, "/srv/blog/env"); i < 0 || j < 0 || i < j {
		t.Errorf("the run script lacks %q after the env file:\n%s", line, r)
	}
	host := NewSsh("blog", "preview-42.preview.example.com", ir.Target{Host: "box", Path: "/srv/blog/42.preview.example.com", Domain: "42.preview.example.com"}, &ir.Deploy{})
	if r := host.runScript("0123456789abcdef"); !strings.Contains(r, `: "${ALDER_PUBLIC_URL:=https://42.preview.example.com}"; export ALDER_PUBLIC_URL`) {
		t.Errorf("--host: no default with the host:\n%s", r)
	}
	for _, domain := range []string{"*.preview.example.com", ""} {
		w := NewSsh("blog", "preview", ir.Target{Host: "box", Path: "/srv/blog", Domain: domain}, &ir.Deploy{})
		if r := w.runScript("0123456789abcdef"); strings.Contains(r, "ALDER_PUBLIC_URL") {
			t.Errorf("domain %q: a default:\n%s", domain, r)
		}
	}

	// the wrapper itself, run by sh with a fake app that prints the variable
	root := t.TempDir()
	b := root + "/builds/0123456789abcdef"
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b+"/app", []byte("#!/bin/sh\nprintf %s \"$ALDER_PUBLIC_URL\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.Root = root
	run := func() string {
		t.Helper()
		cmd := exec.Command("sh", "-c", s.runScript("0123456789abcdef"))
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return string(out)
	}
	if got := run(); got != "https://blog.example.com" {
		t.Errorf("without an env file: %q", got)
	}
	if err := os.WriteFile(root+"/env", []byte("ALDER_PUBLIC_URL=https://www.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := run(); got != "https://www.example.com" {
		t.Errorf("the env file's value does not win: %q", got)
	}
}

func TestCaddyRoute(t *testing.T) {
	var r map[string]any
	if err := json.Unmarshal([]byte(CaddyRoute("alder-blog-production", "blog.example.com", 41233)), &r); err != nil {
		t.Fatal(err)
	}
	if r["@id"] != "alder-blog-production" || r["terminal"] != true {
		t.Errorf("%v", r)
	}
	b, _ := json.Marshal(r["match"])
	h, _ := json.Marshal(r["handle"])
	if string(b) != `[{"host":["blog.example.com"]}]` || string(h) != `[{"handler":"reverse_proxy","upstreams":[{"dial":"127.0.0.1:41233"}]}]` {
		t.Errorf("match %s handle %s", b, h)
	}
	sc := caddyRoute(DefaultCaddyAdmin, "alder-blog-production", "blog.example.com", 41233)
	for _, want := range []string{`put PATCH "$ROUTE" "/id/$ID"`, `/config/apps/http/servers/alder/routes`, `"listen":[":443"]`, "/load"} {
		if !strings.Contains(sc, want) {
			t.Errorf("the route script lacks %q", want)
		}
	}
	if _, err := exec.Command("sh", "-n", "-c", sc).CombinedOutput(); err != nil {
		t.Errorf("the route script does not parse: %v", err)
	}
}

// Every script the target sends parses as POSIX sh (sh -n), with each supervisor.
func TestScriptsParse(t *testing.T) {
	for _, sup := range []string{"process", "system", "user"} {
		s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: "/srv/blog"}, &ir.Deploy{})
		s.sup = sup
		for name, sc := range map[string]string{
			"prelude": s.prelude(), "stop": s.prelude() + "stopnow x\ncancel_later x\n", "ensure": caddyEnsure(DefaultCaddyAdmin), "run": s.runScript("0123456789abcdef"),
		} {
			if out, err := exec.Command("sh", "-n", "-c", sc).CombinedOutput(); err != nil {
				t.Errorf("%s/%s: %v %s", sup, name, err, out)
			}
		}
	}
}

func TestParseSchema(t *testing.T) {
	sc := parseSchema("db\n0001_initial\n0002_add\n@@schema\n{\"tables\": []}\n", "db")
	if !sc.Have || strings.Join(sc.Migrations, ",") != "0001_initial,0002_add" || string(sc.Snapshot) != "{\"tables\": []}\n" {
		t.Errorf("%+v", sc)
	}
	if sc := parseSchema("none\n", "db"); sc.Have {
		t.Errorf("%+v", sc)
	}
}

// Prune removes failed builds first, never the newest deployed non-current build, and keeps a
// build whose backup.db a kept build needs.
func TestPrunePlan(t *testing.T) {
	ok := &Record{}
	b := func(id string, rec *Record, cur bool, of string) Build {
		return Build{ID: id, Record: rec, Current: cur, BackupOf: of}
	}
	builds := []Build{ // newest first
		b("f1", nil, false, ""), b("c", ok, true, "p"), b("p", ok, false, ""), b("f2", nil, false, ""), b("o1", ok, false, ""), b("o2", ok, false, ""),
	}
	if got := strings.Join(PrunePlan(builds, 3, nil), ","); got != "f2,f1,o2" {
		t.Errorf("keep 3: %s", got)
	}
	if got := strings.Join(PrunePlan(builds, 1, nil), ","); got != "f2,f1,o2,o1" {
		t.Errorf("keep 1 (c is current, p the last deployed one): %s", got)
	}
	// c's backup.db is the way back to p: c is current anyway; a pruned build whose backup names a kept one stays
	builds = []Build{b("n", ok, true, "m"), b("m", ok, false, "k"), b("k", ok, false, "")}
	if got := strings.Join(PrunePlan(builds, 1, nil), ","); got != "k" {
		t.Errorf("backup chain: %s", got)
	}
	// x holds the only copy taken under the kept y: it stays; an older copy under y does not
	builds = []Build{b("c", ok, true, "y"), b("y", ok, false, "z"), b("x", ok, false, "w"), b("old", ok, false, "y"), b("w", ok, false, "")}
	if got := strings.Join(PrunePlan(builds, 2, nil), ","); got != "w,old,x" {
		t.Errorf("copies: %s", got)
	}
}

// loopbackSSH stands in for ssh: it drops ssh's options and the destination and runs the
// remote command here with sh (cmd/alder's loopback box does the same).
const loopbackSSH = `#!/bin/sh
while [ $# -gt 0 ]; do
  case "$1" in
    -o|-F|-p|-i|-l|-O) shift 2;;
    -*) shift;;
    *) break;;
  esac
done
shift
exec sh -c "$*"
`

// testClock is a fake clock: Sleep advances it without waiting.
type testClock struct {
	mu    sync.Mutex
	now   time.Time
	slept []time.Duration
}

func (c *testClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *testClock) Sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slept = append(c.slept, d)
	c.now = c.now.Add(d)
}

// Backup's point in time on Turso is the next whole second by the injected clock, waited for
// with the injected Sleep (no real wait).
func TestSsh_BackupPITRUsesInjectedClock(t *testing.T) {
	t.Parallel()
	ssh := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(ssh, []byte(loopbackSSH), 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "builds", "b1"), 0o700); err != nil {
		t.Fatal(err)
	}
	f := tursofake.New(t, "acme")
	if err := f.Create("blog", "default"); err != nil {
		t.Fatal(err)
	}
	f.Exec("blog", `CREATE TABLE "_alder_meta" ("k" TEXT)`)
	clock := &testClock{now: time.Date(2026, 10, 17, 12, 0, 0, 300*int(time.Millisecond), time.UTC)}
	f.SetNow(clock.Now)
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: root}, &ir.Deploy{Proxy: "None"})
	s.SSH = ssh
	s.DB = fakeTurso(t, f)
	s.DB.Now, s.DB.Sleep = clock.Now, clock.Sleep
	start := time.Now()
	got, err := s.Backup(context.Background(), "b1")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed >= 100*time.Millisecond {
		t.Errorf("Backup waited %s in real time", elapsed)
	}
	clock.mu.Lock()
	slept := clock.slept
	clock.mu.Unlock()
	if len(slept) != 1 || slept[0] < 700*time.Millisecond {
		t.Errorf("the injected Sleep was called with %v, want one call of at least 700ms", slept)
	}
	if !strings.Contains(got, "2026-10-17T12:00:01Z") {
		t.Errorf("Backup: %q", got)
	}
	b, err := os.ReadFile(filepath.Join(root, "builds", "b1", "backup.pitr"))
	if err != nil || !strings.HasPrefix(string(b), "2026-10-17T12:00:01Z blog") {
		t.Errorf("backup.pitr: %q %v", b, err)
	}
}

// Poll reaches the scripts as ALDER_POLL (the loops' interval); unset, each loop keeps its own.
// Deploy's and Rollback's clock reaches the Turso database (Clocked).
func TestSsh_PollAndClock(t *testing.T) {
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: "/srv/blog"}, &ir.Deploy{})
	if strings.Contains(s.prelude(), "ALDER_POLL=") {
		t.Errorf("Poll 0 exports ALDER_POLL:\n%s", s.prelude())
	}
	s.Poll = 10 * time.Millisecond
	out, err := exec.Command("sh", "-c", s.prelude()+`printf %s "$ALDER_POLL"; sh -c 'printf " %s" "$ALDER_POLL"'`).Output()
	if err != nil || string(out) != "0.01 0.01" {
		t.Errorf("ALDER_POLL at the target: %q %v", out, err)
	}
	s.DB = NewTurso(&ir.Database{Kind: "turso", Org: "acme", App: "blog"})
	clock := &testClock{now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	setClock(s, clock.Now, clock.Sleep)
	s.DB.sleep(time.Hour)
	if got := s.DB.Now(); !got.Equal(time.Date(2026, 10, 17, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("the database's clock after Sleep(1h): %s", got)
	}
	setClock(s, nil, nil) // nothing set: the database keeps its clock
	if s.DB.Now == nil || s.DB.Sleep == nil {
		t.Error("an unset clock cleared the database's")
	}
}

// Logs -n N tails the current build's log by N lines (the loopback ssh runs the script here).
func TestSsh_LogsN(t *testing.T) {
	t.Parallel()
	ssh := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(ssh, []byte(loopbackSSH), 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "builds", "b1"), 0o700); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i := 1; i <= 150; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	os.WriteFile(filepath.Join(root, "builds", "b1", "log"), []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	os.WriteFile(filepath.Join(root, "current"), []byte("b1\n"), 0o600)
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: root}, &ir.Deploy{Proxy: "None"})
	s.SSH = ssh
	for _, n := range []int{3, 100, 150} {
		var w bytes.Buffer
		if err := s.Logs(context.Background(), false, n, &w); err != nil {
			t.Fatal(err)
		}
		if want := strings.Join(lines[150-n:], "\n") + "\n"; w.String() != want {
			t.Errorf("-n %d:\n%s", n, w.String())
		}
	}
}
