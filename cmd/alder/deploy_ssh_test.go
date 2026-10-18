package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"alder/internal/deploy"
	"alder/internal/ir"

	_ "modernc.org/sqlite"
)

// The Ssh target's tests run against a box on this machine, one of two harnesses:
//
//   - go test -tags deploy ./cmd/alder: a Docker container built from testdata/deploy/Dockerfile
//     (Debian, sshd, curl, a user `deploy`), started per test and removed after. CI's deploy job
//     runs this. Without Docker the tests skip, saying so.
//   - ALDER_DEPLOY_LOCAL=1 go test ./cmd/alder -run DeploySsh: a private sshd (the system's
//     /usr/sbin/sshd, run as the current user on a free port of 127.0.0.1 with a host key and a
//     client key made for the test, so nothing in ~/.ssh is used or changed) logging in to
//     this machine as the current user, the target's <path> under a temporary directory.
//
// The untagged default suite has neither: it runs the same tests against a loopback box (an
// `ssh` on PATH that runs the remote command here with sh: loopbackBox), so everything but the
// SSH connection is exercised everywhere. The engine's order (RF1–RF4) is also covered by
// internal/deploy's tests with a fake target, and the Script target end to end by
// deploy_test.go.

// dockerBox is set by deploy_docker_test.go under -tags deploy.
var dockerBox bool

// qr is the box's root quoted for its shell (the loopback box's has a space and a quote in it).
func (b *sshBox) qr() string { return fmt.Sprintf("%q", b.root) }

// sshBox is a running box: ssh reaches it as host with the ssh_config config; root is a
// directory there for the target's <path>; port is the public port its `proxy: None` app
// listens on. A container box also names its container and the port of this machine its
// port 18443 (the fake Caddy's public listener: deploy_caddy_test.go) is published on.
type sshBox struct {
	config, host, root string
	port               int
	container          string
	caddyPort          int
}

// sh runs the shell command cmd at the box and returns its output.
func (b *sshBox) sh(t *testing.T, cmd string) string {
	t.Helper()
	out, err := exec.Command("ssh", "-F", b.config, b.host, cmd).CombinedOutput()
	if err != nil {
		t.Fatalf("ssh %s: %v\n%s", cmd, err, out)
	}
	return string(out)
}

// write writes content to the file rel under the box's root (through ssh's standard input).
func (b *sshBox) write(t *testing.T, rel, content string) {
	t.Helper()
	cmd := exec.Command("ssh", "-F", b.config, b.host, fmt.Sprintf("umask 077; cat > %s/%s", b.qr(), rel))
	cmd.Stdin = strings.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("writing %s: %v %s", rel, err, out)
	}
}

// post posts an entry to the scaffold's form at the box's public port (curl at the box).
func (b *sshBox) post(t *testing.T, name string) {
	t.Helper()
	b.sh(t, fmt.Sprintf("curl -s -o /dev/null -X POST -H 'Origin: http://127.0.0.1:%d' -d 'name=%s&message=hello' http://127.0.0.1:%d/_m/Home/Submit", b.port, name, b.port))
}

// names is the entries the page at the box's public port lists, newest first.
func (b *sshBox) names(t *testing.T) string {
	t.Helper()
	page := b.sh(t, fmt.Sprintf("curl -sL http://127.0.0.1:%d/", b.port))
	var got []string
	for _, m := range regexp.MustCompile(`<li><b>([^<]*)</b>`).FindAllStringSubmatch(page, -1) {
		got = append(got, m[1])
	}
	return strings.Join(got, ",")
}

// serving is the build id in the page the box's public port serves ("" when nothing answers).
func (b *sshBox) serving(t *testing.T) string {
	t.Helper()
	out, _ := exec.Command("ssh", "-F", b.config, b.host, fmt.Sprintf("curl -sL --max-time 5 http://127.0.0.1:%d/", b.port)).Output()
	m := regexp.MustCompile(`name="alder-build" content="([0-9a-f]+)"`).FindSubmatch(out)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// startBox starts the box for the test (see above), or skips.
func startBox(t *testing.T) *sshBox {
	t.Helper()
	switch {
	case os.Getenv("ALDER_DEPLOY_LOCAL") == "1":
		return localBox(t)
	case dockerBox:
		return containerBox(t)
	}
	return loopbackBox(t)
}

// loopbackSSH stands in for ssh in the default suite: it drops ssh's options and the
// destination and runs the remote command here with sh, so the Ssh target's scripts run as
// they would at a box, without sshd.
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

// loopbackBox is the default suite's box: PATH starts with loopbackSSH as ssh, and the
// target's <path> is a temporary directory. Everything but the SSH connection itself is
// exercised: the remote scripts, the processes, the health check by build id.
func loopbackBox(t *testing.T) *sshBox {
	t.Helper()
	for _, tool := range []string{"sh", "curl", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("the loopback box needs %s", tool)
		}
	}
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "ssh"), loopbackSSH)
	os.Chmod(filepath.Join(bin, "ssh"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := filepath.Join(bin, "ssh_config")
	writeFile(t, cfg, "# the loopback box reads no configuration\n")
	root := filepath.Join(evalSymlinks(t.TempDir()), "box q'x") // every path the target writes is quoted
	killBuilds(t, root)
	return &sshBox{config: cfg, host: "alder-test-box", root: root, port: freePort(t)}
}

// killBuilds stops, when the test ends, the builds started under root: they run as this
// user's processes (a container's die with it).
func killBuilds(t *testing.T, root string) {
	t.Cleanup(func() {
		pids, _ := filepath.Glob(filepath.Join(root, "builds", "*", "pid"))
		for _, f := range pids {
			if b, err := os.ReadFile(f); err == nil {
				var pid int
				if _, err := fmt.Sscan(string(b), &pid); err == nil && pid > 1 {
					if p, err := os.FindProcess(pid); err == nil {
						p.Kill()
					}
				}
			}
		}
	})
}

// clientKey makes an ed25519 key pair in dir and returns the private key's path.
func clientKey(t *testing.T, dir, name string) string {
	t.Helper()
	key := filepath.Join(dir, name)
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Skipf("ssh-keygen: %v %s", err, out)
	}
	return key
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// writeSSHConfig writes the ssh_config of a box at 127.0.0.1:port as user with key.
func writeSSHConfig(t *testing.T, dir string, port int, login, key string) string {
	t.Helper()
	cfg := filepath.Join(dir, "ssh_config")
	writeFile(t, cfg, fmt.Sprintf(`Host alder-test-box
  HostName 127.0.0.1
  Port %d
  User %s
  IdentityFile %s
  IdentitiesOnly yes
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  LogLevel ERROR
`, port, login, key))
	return cfg
}

// waitSSH waits for the box to accept a login.
func waitSSH(t *testing.T, cfg string, logs func() string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, err := exec.Command("ssh", "-F", cfg, "-o", "ConnectTimeout=3", "alder-test-box", "echo ok").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "ok" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the box's sshd does not accept the test's key: %v %s\n%s", err, out, logs())
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func localBox(t *testing.T) *sshBox {
	t.Helper()
	sshd := "/usr/sbin/sshd"
	if _, err := os.Stat(sshd); err != nil {
		t.Skip("ALDER_DEPLOY_LOCAL=1 needs /usr/sbin/sshd")
	}
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	host := clientKey(t, dir, "host_key")
	key := clientKey(t, dir, "client_key")
	pub, _ := os.ReadFile(key + ".pub")
	writeFile(t, filepath.Join(dir, "authorized_keys"), string(pub))
	port := freePort(t)
	writeFile(t, filepath.Join(dir, "sshd_config"), fmt.Sprintf(`Port %d
ListenAddress 127.0.0.1
HostKey %s
AuthorizedKeysFile %s
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
StrictModes no
PidFile %s
`, port, host, filepath.Join(dir, "authorized_keys"), filepath.Join(dir, "sshd.pid")))
	var logs bytes.Buffer
	cmd := exec.Command(sshd, "-D", "-e", "-f", filepath.Join(dir, "sshd_config"))
	cmd.Stdout, cmd.Stderr = &logs, &logs
	cmd.WaitDelay = 3 * time.Second // a session child may hold the output a moment longer
	if err := cmd.Start(); err != nil {
		t.Skipf("starting sshd: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	cfg := writeSSHConfig(t, dir, port, u.Username, key)
	waitSSH(t, cfg, logs.String)
	root := filepath.Join(evalSymlinks(t.TempDir()), "box")
	killBuilds(t, root)
	return &sshBox{config: cfg, host: "alder-test-box", root: root, port: freePort(t)}
}

func containerBox(t *testing.T) *sshBox {
	t.Helper()
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		if os.Getenv("ALDER_DEPLOY_REQUIRE_DOCKER") == "1" { // CI's deploy job: Docker must be there
			t.Fatalf("-tags deploy with ALDER_DEPLOY_REQUIRE_DOCKER=1: Docker does not answer (%v): %s", err, lastLine(string(out)))
		}
		t.Skipf("-tags deploy: Docker does not answer (%v), so the Ssh target's tests skip: %s", err, lastLine(string(out)))
	}
	const image = "alder-deploy-test-box"
	if out, err := exec.Command("docker", "build", "-q", "-t", image, "testdata/deploy").CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v\n%s", err, out)
	}
	out, err := exec.Command("docker", "run", "-d", "--rm", "-p", "127.0.0.1::22", "-p", "127.0.0.1::18443", image).Output()
	if err != nil {
		t.Fatalf("docker run: %v", err)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", id).Run() })
	out, err = exec.Command("docker", "port", id, "22").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	hostPort := func(out []byte) int {
		var port int
		if f := strings.Fields(string(out)); len(f) == 0 {
			t.Fatalf("docker port printed %q", out)
		} else if _, err := fmt.Sscanf(f[0][strings.LastIndex(f[0], ":")+1:], "%d", &port); err != nil {
			t.Fatalf("docker port printed %q", out)
		}
		return port
	}
	port := hostPort(out)
	out, err = exec.Command("docker", "port", id, "18443").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	caddyPort := hostPort(out)
	dir := t.TempDir()
	key := clientKey(t, dir, "client_key")
	pub, _ := os.ReadFile(key + ".pub")
	in := exec.Command("docker", "exec", "-i", id, "sh", "-c", "mkdir -p /home/deploy/.ssh && cat > /home/deploy/.ssh/authorized_keys && chmod 700 /home/deploy/.ssh && chmod 600 /home/deploy/.ssh/authorized_keys && chown -R deploy /home/deploy/.ssh")
	in.Stdin = bytes.NewReader(pub)
	if out, err := in.CombinedOutput(); err != nil {
		t.Fatalf("docker exec: %v %s", err, out)
	}
	cfg := writeSSHConfig(t, dir, port, "deploy", key)
	waitSSH(t, cfg, func() string { out, _ := exec.Command("docker", "logs", id).CombinedOutput(); return string(out) })
	return &sshBox{config: cfg, host: "alder-test-box", root: "/srv/alder-test", port: 18080, container: id, caddyPort: caddyPort}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// sshScaffold is deployScaffold with an Ssh environment `production` pointing at box (`proxy:
// None`, requires SMTP_URL, keep 3), and the knobs that keep the test short: no drain, a 20 s
// health check, the process supervisor.
func sshScaffold(t *testing.T, box *sshBox) string {
	t.Helper()
	dir := deployScaffold(t, fmt.Sprintf(`deploy {
  production: Ssh %q { domain: "localhost", path: %q },
  requires: ["SMTP_URL"],
  proxy: None,
  keep: 3,
}`, box.host, box.root), nil)
	t.Setenv(deploy.EnvSSHConfig, box.config)
	t.Setenv(envDrain, "0")
	t.Setenv(envHealthTimeout, "20s")
	t.Setenv(deploy.EnvSupervisor, "process")
	box.sh(t, fmt.Sprintf("mkdir -p %s && echo %d > %s/port", box.qr(), box.port, box.qr()))
	return dir
}

// migrateDemo edits demo's Entry table with edit (old → new) and writes its migration.
func migrateDemo(t *testing.T, old, new, name string) {
	t.Helper()
	b, err := os.ReadFile("demo/app/Entry.alder")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("Entry.alder lacks %q", old)
	}
	writeFile(t, "demo/app/Entry.alder", strings.Replace(string(b), old, new, 1))
	var out, errb bytes.Buffer
	if code := runMigrateTo([]string{"demo", "--name", name}, &out, &errb); code != 0 {
		t.Fatalf("alder migrate: %d %s %s", code, out.String(), errb.String())
	}
}

// buildIDs are the ids the deploy output names in "deployed <id>".
func deployedID(t *testing.T, out string) string {
	t.Helper()
	m := regexp.MustCompile(`deployed ([0-9a-f]{16}) to production`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no deployed line:\n%s", out)
	}
	return m[1]
}

// The Ssh target's life: RF4 (a missing `requires` name refused before any upload), a first
// deploy, a view change and a rollback to the first build, a deploy with a migration (status
// lists the builds), RF3 (a rollback to a build older than
// the live schema refused, naming the table), RF1 (a migration that drops data refused, by
// --dry-run too, and with --allow-data-loss the database copied before the migration), a
// rollback that passes, RF2 (a build that exits at start: nothing switched, the old build
// serving, the log in the refusal, its directory kept), logs, and no planted value anywhere
// under builds/ or in any output.
func TestDeploySsh_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	box := startBox(t)
	dir := sshScaffold(t, box)
	var rnd [8]byte
	rand.Read(rnd[:])
	planted := "planted-" + hex.EncodeToString(rnd[:])
	var all strings.Builder // every output, searched for the planted value at the end
	deployRun := func(args ...string) (int, string) {
		t.Helper()
		code, out, errs := runDeploy(t, append(args[:1:1], append([]string{dir}, args[1:]...)...)...)
		all.WriteString(out + errs)
		return code, out + errs
	}

	// RF4: SMTP_URL is not in the environment file: refused, nothing uploaded
	box.sh(t, fmt.Sprintf("printf 'ALDER_SECRET=%s\\n' > %s/env && chmod 600 %s/env", strings.Repeat("s", 32), box.qr(), box.qr()))
	code, out := deployRun("production")
	if code != 1 || !strings.Contains(out, "alder deploy: refused: the environment at Ssh alder-test-box:"+box.root+" does not set SMTP_URL: add it to its environment file") {
		t.Fatalf("RF4: exit %d\n%s", code, out)
	}
	if ls := box.sh(t, "ls "+box.qr()); strings.Contains(ls, "builds") {
		t.Errorf("RF4: a build was uploaded before the refusal: %s", ls)
	}
	box.sh(t, fmt.Sprintf("printf 'SMTP_URL=smtp://u:%s@mail\\n' >> %s/env", planted, box.qr()))

	// the first deploy
	code, out = deployRun("production")
	if code != 0 {
		t.Fatalf("first deploy: exit %d\n%s", code, out)
	}
	id1 := deployedID(t, out)
	for _, want := range []string{"  requires ok  SMTP_URL, ALDER_SECRET\n", "first deploy: the database is created by every migration", "  upload   ok  builds/" + id1 + "/\n",
		"  backup   ok  no database yet\n", "  migrate  ok  applied 0001_initial\n", "  health   ok  build " + id1 + " answers GET /\n", fmt.Sprintf("  switch   ok  current → %s (port %d)\n", id1, box.port)} {
		if !strings.Contains(out, want) {
			t.Errorf("first deploy: output lacks %q:\n%s", want, out)
		}
	}
	if got := box.serving(t); got != id1 {
		t.Fatalf("after the first deploy the box serves %q, want %s", got, id1)
	}

	// a deploy without a migration, and a rollback to the first build (the same schema)
	appendTo(t, "demo/app/Home.alder", "\nversion : Int\nversion = 2\n")
	code, out = deployRun("production")
	if code != 0 {
		t.Fatalf("deploy of a view change: exit %d\n%s", code, out)
	}
	id1b := deployedID(t, out)
	if !strings.Contains(out, "no pending migration") || box.serving(t) != id1b {
		t.Errorf("deploy of a view change:\n%s", out)
	}
	var so, se bytes.Buffer
	if code := runRollbackTo([]string{"production", "--dir", dir}, &so, &se); code != 0 || !strings.Contains(so.String(), "  schema   ok  the live schema is the build's\n") || !strings.Contains(so.String(), "rolled back to "+id1) {
		t.Fatalf("rollback to %s: exit %d\n%s\n%s", id1, code, so.String(), se.String())
	}
	if got := box.serving(t); got != id1 {
		t.Errorf("after the rollback the box serves %q, want %s", got, id1)
	}

	// a second deploy with a migration
	migrateDemo(t, "  message: Text.NonEmpty,\n", "  message: Text.NonEmpty,\n  mood: Maybe Text = Nothing,\n", "add_mood")
	code, out = deployRun("production")
	if code != 0 {
		t.Fatalf("second deploy: exit %d\n%s", code, out)
	}
	id2 := deployedID(t, out)
	if !strings.Contains(out, "  0002_add_mood\n    rebuild table Entry") || !strings.Contains(out, "  migrate  ok  applied 0002_add_mood\n") || !strings.Contains(out, "  backup   ok  builds/"+id2+"/backup.db\n") {
		t.Errorf("second deploy:\n%s", out)
	}
	if got := box.serving(t); got != id2 {
		t.Fatalf("after the second deploy the box serves %q, want %s", got, id2)
	}
	so.Reset()
	se.Reset()
	if code := runStatusTo([]string{"production", dir}, &so, &se); code != 0 {
		t.Fatalf("status: %d %s", code, se.String())
	}
	all.WriteString(so.String() + se.String())
	st := so.String()
	if !strings.Contains(st, "current  "+id2+"\n") || !strings.Contains(st, "  * "+id2+"  ") || !strings.Contains(st, "    "+id1+"  ") ||
		!strings.Contains(st, "migrated 0002_add_mood") || !strings.Contains(st, "database ") || !strings.Contains(st, "backup   ") {
		t.Errorf("status:\n%s", st)
	}

	// RF3: rolling back to id1 (no mood) under the live schema (mood) is refused
	so.Reset()
	se.Reset()
	if code := runRollbackTo([]string{"production", "--dir", dir}, &so, &se); code != 1 || !strings.Contains(se.String(), "(changed tables: Entry.Entry): that build would refuse to start on it") {
		t.Errorf("RF3: exit %d\n%s\n%s", code, so.String(), se.String())
	}
	if got := box.serving(t); got != id2 {
		t.Errorf("RF3: after the refused rollback the box serves %q, want %s", got, id2)
	}

	// RF1: dropping mood drops data: refused by --dry-run and by deploy, nothing uploaded
	migrateDemo(t, "  mood: Maybe Text = Nothing,\n", "", "drop_mood")
	before := box.sh(t, "ls "+box.qr()+"/builds")
	for _, args := range [][]string{{"production", "--dry-run"}, {"production"}} {
		code, out = deployRun(args...)
		if code != 1 || !strings.Contains(out, "alder deploy: refused: the migration drops data\n  DROPS DATA: 0003_drop_mood: column Entry.mood\nrun again with --allow-data-loss") ||
			!strings.Contains(out, "    DROPS DATA: column Entry.mood\n") {
			t.Errorf("RF1 %v: exit %d\n%s", args, code, out)
		}
	}
	if after := box.sh(t, "ls "+box.qr()+"/builds"); after != before {
		t.Errorf("RF1: the refused deploys changed builds/: %q → %q", before, after)
	}
	code, out = deployRun("production", "--allow-data-loss")
	if code != 0 {
		t.Fatalf("RF1 with --allow-data-loss: exit %d\n%s", code, out)
	}
	id3 := deployedID(t, out)
	// the copy was taken before the migration: it still has the column
	backupFile := filepath.Join(t.TempDir(), "backup.db")
	writeFile(t, backupFile, box.sh(t, "cat "+box.qr()+"/builds/"+id3+"/backup.db"))
	conn, err := sql.Open("sqlite", backupFile)
	if err != nil {
		t.Fatal(err)
	}
	var ddl string
	conn.QueryRow(`SELECT group_concat(sql, ';') FROM sqlite_master WHERE type = 'table'`).Scan(&ddl)
	conn.Close()
	if !strings.Contains(ddl, `"mood"`) {
		t.Errorf("RF1: builds/%s/backup.db was not taken before the migration (its tables: %s)", id3, ddl)
	}
	var rec deploy.Record
	if err := json.Unmarshal([]byte(box.sh(t, "cat "+box.qr()+"/builds/"+id3+"/deployed.json")), &rec); err != nil || !rec.AllowDataLoss || rec.Backup != "builds/"+id3+"/backup.db" ||
		strings.Join(rec.Applied, ",") != "0003_drop_mood" || rec.Previous != id2 {
		t.Errorf("RF1: deployed.json: %+v %v", rec, err)
	}

	// keep 3: id1b, the oldest by its deploy (id1's record was rewritten by the rollback), was pruned
	if !strings.Contains(out, "  prune    ok  removed "+id1b+" (keep 3)\n") || strings.Contains(box.sh(t, "ls "+box.qr()+"/builds"), id1b) {
		t.Errorf("prune:\n%s", out)
	}
	// RF3 again: id1's schema is the live one again, but the database has had 0002 and 0003
	so.Reset()
	se.Reset()
	if code := runRollbackTo([]string{"production", id1, "--dir", dir}, &so, &se); code != 1 || !strings.Contains(se.String(), "the live database has migrations build "+id1+" does not have (0002_add_mood, 0003_drop_mood)") {
		t.Errorf("rollback to %s: exit %d\n%s\n%s", id1, code, so.String(), se.String())
	}

	// RF2: a build whose adapter exits at start
	writeFile(t, "demo/app/Clock.alder", "module Clock\n\ncapability Clock {\n  now : Task Int,\n}\n")
	writeFile(t, "demo/adapters/clock/clock.go", `package clock

import (
	"context"
	"fmt"
	"os"

	"app"
)

func Clock() app.Cap__Clock__Clock {
	if os.Getenv("ALDER_DB") != "" {
		fmt.Fprintln(os.Stderr, "boom: the clock adapter cannot reach its time server")
		os.Exit(1)
	}
	return clock{}
}

type clock struct{}

func (clock) Now(context.Context) (int64, error) { return 0, nil }
`)
	code, out = deployRun("production", "--unsafe-skip-tests")
	if code != 1 || !strings.Contains(out, "did not pass its health check: it was stopped and builds/") || !strings.Contains(out, "nothing was switched; build "+id3+" still serves") ||
		!strings.Contains(out, "boom: the clock adapter cannot reach its time server") {
		t.Errorf("RF2: exit %d\n%s", code, out)
	}
	if got := box.serving(t); got != id3 {
		t.Errorf("RF2: the box serves %q, want %s", got, id3)
	}
	if cur := strings.TrimSpace(box.sh(t, "cat "+box.qr()+"/current")); cur != id3 {
		t.Errorf("RF2: current is %s", cur)
	}
	if m := regexp.MustCompile(`builds/([0-9a-f]{16})/ is kept`).FindStringSubmatch(out); m == nil || !strings.Contains(box.sh(t, "ls "+box.qr()+"/builds"), m[1]) {
		t.Errorf("RF2: the failed build's directory is not kept")
	}

	// status lists the failed build (it has no deployed.json)
	so.Reset()
	se.Reset()
	if code := runStatusTo([]string{"production", dir}, &so, &se); code != 0 || !strings.Contains(so.String(), "never switched to (failed or interrupted; kept for inspection)") || !strings.Contains(so.String(), "current  "+id3+"\n") {
		t.Errorf("status after RF2: %d\n%s%s", code, so.String(), se.String())
	}

	// logs
	so.Reset()
	se.Reset()
	if code := runLogsTo([]string{"production", dir}, &so, &se); code != 0 || !strings.Contains(so.String(), "listening on") {
		t.Errorf("logs: %d\n%s\n%s", code, so.String(), se.String())
	}
	all.WriteString(so.String())

	// no planted value under builds/ nor in any output
	if hits := box.sh(t, "grep -rl "+planted+" "+box.qr()+"/builds || true"); strings.TrimSpace(hits) != "" {
		t.Errorf("the planted value is in %s", hits)
	}
	if strings.Contains(all.String(), planted) {
		t.Error("the planted value is in the commands' output")
	}
}

// The backup shipper at the box and `alder restore`: the operator's environment file names a
// directory as ALDER_BACKUP_URL (the declaration's `backup` takes S3 only); entries posted to
// the app are streamed there; `alder restore --out` rebuilds them locally from a copy of that
// directory; `alder restore` from that copy puts the database back at the box (entries posted
// since are gone, the previous file kept beside it) and the build serves it again.
func TestDeploySsh_BackupRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	box := startBox(t)
	dir := sshScaffold(t, box)
	bucket := box.root + "/bucket"
	box.write(t, "env", fmt.Sprintf("ALDER_SECRET=%s\nSMTP_URL=smtp://mail\nALDER_BACKUP_URL=%q\nALDER_BACKUP_INTERVAL=1s\n", strings.Repeat("s", 32), bucket))
	code, out, errs := runDeploy(t, "production", dir)
	if code != 0 {
		t.Fatalf("deploy: exit %d\n%s\n%s", code, out, errs)
	}
	id := deployedID(t, out)
	if strings.Contains(out, "warning: no `backup` in the declaration") {
		t.Errorf("the environment file streams the backup, but the deploy warns there is none:\n%s", out)
	}
	post := func(name string) {
		t.Helper()
		box.sh(t, fmt.Sprintf("curl -s -o /dev/null -X POST -H 'Origin: http://127.0.0.1:%d' -d 'name=%s&message=hello' http://127.0.0.1:%d/_m/Home/Submit", box.port, name, box.port))
	}
	entries := func() int {
		t.Helper()
		page := box.sh(t, fmt.Sprintf("curl -sL http://127.0.0.1:%d/", box.port))
		return strings.Count(page, "<li><b>")
	}
	for _, n := range []string{"Ada", "Grace", "Edsger"} {
		post(n)
	}
	if n := entries(); n != 3 {
		t.Fatalf("the app lists %d entries", n)
	}
	// wait for a shipment after the posts (the shipper's status file names its last upload)
	// (more than an interval after the posts: a sync that took the lock before them also ends after them)
	posted := time.Now().Add(1100 * time.Millisecond)
	deadline := time.Now().Add(15 * time.Second)
	for {
		var st struct {
			Last time.Time `json:"last"`
		}
		json.Unmarshal([]byte(box.sh(t, "cat "+box.qr()+"/db/app.db-backup.json 2>/dev/null || true")), &st)
		if st.Last.After(posted) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("nothing shipped: %s\n%s", box.sh(t, "find "+fmt.Sprintf("%q", bucket)+" -type f || true"), box.sh(t, "cat "+box.qr()+"/builds/"+id+"/log"))
		}
		time.Sleep(300 * time.Millisecond)
	}
	var so, se bytes.Buffer
	if code := runStatusTo([]string{"production", dir}, &so, &se); code != 0 || !strings.Contains(so.String(), "(streamed to file://"+bucket+")") {
		t.Errorf("status: %d\n%s%s", code, so.String(), se.String())
	}
	// a copy of the bucket here
	local := t.TempDir()
	tarball, err := exec.Command("ssh", "-F", box.config, box.host, "tar -C "+box.qr()+" -cf - bucket").Output()
	if err != nil {
		t.Fatal(err)
	}
	untar := exec.Command("tar", "-xf", "-", "-C", local)
	untar.Stdin = bytes.NewReader(tarball)
	if out, err := untar.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v %s", err, out)
	}
	copyDir := filepath.Join(local, "bucket")
	restored := filepath.Join(t.TempDir(), "restored.db")
	so.Reset()
	se.Reset()
	if code := runRestoreTo([]string{"production", "--dir", dir, "--from", copyDir, "--out", restored}, &so, &se); code != 0 || !strings.Contains(so.String(), "wrote "+restored) {
		t.Fatalf("restore --out: %d\n%s%s", code, so.String(), se.String())
	}
	conn, err := sql.Open("sqlite", restored)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM "Entry__Entry"`).Scan(&n); err != nil || n != 3 {
		t.Errorf("the restored file has %d entries (%v)", n, err)
	}
	conn.Close()

	// without --yes and without a terminal: refused, nothing replaced
	so.Reset()
	se.Reset()
	if code := runRestoreTo([]string{"production", "--dir", dir, "--from", copyDir}, &so, &se); code != 2 || !strings.Contains(se.String(), "not confirmed: nothing was replaced") {
		t.Errorf("restore without --yes: %d %s", code, se.String())
	}
	// two more entries, then the copy (three) put back at the box
	post("Barbara")
	post("Ken")
	if n := entries(); n != 5 {
		t.Fatalf("the app lists %d entries", n)
	}
	so.Reset()
	se.Reset()
	if code := runRestoreTo([]string{"production", "--dir", dir, "--from", copyDir, "--yes"}, &so, &se); code != 0 || !strings.Contains(so.String(), "replaced the database at Ssh alder-test-box:"+box.root) {
		t.Fatalf("restore: %d\n%s%s", code, so.String(), se.String())
	}
	if n := entries(); n != 3 {
		t.Errorf("after the restore the app lists %d entries, want 3", n)
	}
	if got := box.serving(t); got != id {
		t.Errorf("after the restore the box serves %q, want %s", got, id)
	}
	if kept := box.sh(t, "ls "+box.qr()+"/db"); !strings.Contains(kept, "app.db.before-restore-") {
		t.Errorf("the previous database is not kept: %s", kept)
	}
}

// C1: a rollback inside the drain window. The previous build's delayed stop does nothing once
// that build is current again, and starting it again (what the rollback does) reuses the
// running process and cancels the delayed stop.
func TestDeploySsh_DrainRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	box := startBox(t)
	dir := sshScaffold(t, box)
	box.sh(t, fmt.Sprintf("printf 'ALDER_SECRET=%s\\nSMTP_URL=smtp://mail\\n' > %s/env", strings.Repeat("s", 32), box.qr()))
	code, out, errs := runDeploy(t, "production", dir)
	if code != 0 {
		t.Fatalf("deploy: %d\n%s%s", code, out, errs)
	}
	id1 := deployedID(t, out)
	appendTo(t, "demo/app/Home.alder", "\nversion : Int\nversion = 2\n")
	if code, out, errs = runDeploy(t, "production", dir); code != 0 {
		t.Fatalf("second deploy: %d\n%s%s", code, out, errs)
	}
	id2 := deployedID(t, out)
	ctx := context.Background()
	s := deploy.NewSsh("demo", "production", ir.Target{Kind: "ssh", Host: box.host, Path: box.root, Domain: "localhost"}, &ir.Deploy{Proxy: "None"})
	if _, err := s.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close() // the shared connection
	port, err := s.Start(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	// started again inside the window: the same process, the delayed stop cancelled
	if err := s.Stop(ctx, id1, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if again, err := s.Start(ctx, id1); err != nil || again != port {
		t.Fatalf("start inside the drain window: port %d (was %d) %v", again, port, err)
	}
	time.Sleep(3 * time.Second)
	if err := s.Health(ctx, id1, port, 3*time.Second); err != nil {
		t.Errorf("the delayed stop fired after the build was started again: %v", err)
	}
	// current again when the delay ends: the delayed stop does nothing
	if err := s.Stop(ctx, id1, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	box.sh(t, fmt.Sprintf("echo %s > %s/current", id1, box.qr()))
	time.Sleep(3 * time.Second)
	if err := s.Health(ctx, id1, port, 3*time.Second); err != nil {
		t.Errorf("the delayed stop fired although the build is current again: %v", err)
	}
	box.sh(t, fmt.Sprintf("echo %s > %s/current", id2, box.qr()))

	// logs -f follows the build that becomes current
	var logs syncBuffer
	lctx, lcancel := context.WithCancel(ctx)
	ldone := make(chan struct{})
	go func() { s.Logs(lctx, true, &logs); close(ldone) }()
	time.Sleep(1500 * time.Millisecond)
	box.sh(t, fmt.Sprintf("echo %s > %s/current", id1, box.qr()))
	time.Sleep(2500 * time.Millisecond)
	lcancel()
	<-ldone
	box.sh(t, fmt.Sprintf("echo %s > %s/current", id2, box.qr()))
	if !strings.Contains(logs.String(), "--- alder logs: build "+id1+" is current\n") {
		t.Errorf("logs -f did not follow the switch:\n%s", logs.String())
	}

	// I3: a second run refuses while the lock is held, and goes ahead once it is released
	unlock, err := s.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	code, out, errs = runDeploy(t, "production", dir)
	if code != 1 || !strings.Contains(errs, "alder deploy: refused: another deploy, rollback or restore of production is running (the lock at "+box.root+"/deploy.lock is held by ") {
		t.Errorf("deploy under the lock: %d\n%s%s", code, out, errs)
	}
	var so, se bytes.Buffer
	if code := runRollbackTo([]string{"production", "--dir", dir}, &so, &se); code != 1 || !strings.Contains(se.String(), "another deploy, rollback or restore") {
		t.Errorf("rollback under the lock: %d %s", code, se.String())
	}
	unlock()
	again, err := s.Lock(ctx)
	if err != nil {
		t.Fatalf("the lock was not released: %v", err)
	}
	again()
	// a lock left by a killed run (the directory form, where flock is missing): taken over, and said
	if strings.TrimSpace(box.sh(t, "command -v flock || true")) == "" {
		box.sh(t, fmt.Sprintf("mkdir %s/deploy.lock.d && echo 'held by x@y (local z) since 2026-10-17T00:00:00Z, pid 999999' > %s/deploy.lock.d/info", box.qr(), box.qr()))
		var warn bytes.Buffer
		s.Warn = &warn
		u, err := s.Lock(ctx)
		if err != nil {
			t.Fatalf("a stale lock: %v", err)
		}
		u()
		s.Warn = io.Discard
		if !strings.Contains(warn.String(), "warning: took over a stale deploy lock at "+box.root+"/deploy.lock.d (held by x@y (local z) since 2026-10-17T00:00:00Z, pid 999999, whose process no longer runs") {
			t.Errorf("the takeover was not reported: %q", warn.String())
		}
	}
	// I8: an environment file that turns on development mode is refused
	box.sh(t, fmt.Sprintf("echo ALDER_DEV=1 >> %s/env", box.qr()))
	if code, out, errs = runDeploy(t, "production", dir); code != 1 || !strings.Contains(errs, "sets ALDER_DEV: development mode") {
		t.Errorf("ALDER_DEV: %d\n%s%s", code, out, errs)
	}
	box.sh(t, fmt.Sprintf("sed -i.bak '/ALDER_DEV/d' %s/env", box.qr()))
	// I5: what the deploy writes is the deploy user's
	if modes := box.sh(t, fmt.Sprintf("cd %s && ls -ld builds | cut -c1-10; ls -l builds/%s/run builds/%s/backup.db | cut -c1-10", box.qr(), id2, id2)); modes != "drwxr-x---\n-rw-------\n-rwx------\n" {
		t.Errorf("modes:\n%s", modes)
	}
	if err := s.Stop(ctx, id1, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(ctx, id1, 2*time.Second); !errors.Is(err, deploy.ErrNotRunning) {
		t.Errorf("stopping a stopped build: %v", err)
	}
}

// I1, I2: a deploy whose migration applied but whose build fails its health check names the
// pre-migration copy and the command that puts it back; `rollback --restore-backup` puts it back
// (the serving build restarts on it), and across an earlier migration it brings back the first
// build with its data from before that migration.
func TestDeploySsh_RestoreBackup(t *testing.T) {
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	box := startBox(t)
	dir := sshScaffold(t, box)
	box.sh(t, fmt.Sprintf("printf 'ALDER_SECRET=%s\\nSMTP_URL=smtp://mail\\n' > %s/env", strings.Repeat("s", 32), box.qr()))
	post := func(name string) {
		t.Helper()
		box.sh(t, fmt.Sprintf("curl -s -o /dev/null -X POST -H 'Origin: http://127.0.0.1:%d' -d 'name=%s&message=hello' http://127.0.0.1:%d/_m/Home/Submit", box.port, name, box.port))
	}
	names := func() string {
		t.Helper()
		page := box.sh(t, fmt.Sprintf("curl -sL http://127.0.0.1:%d/", box.port))
		var got []string
		for _, m := range regexp.MustCompile(`<li><b>([^<]*)</b>`).FindAllStringSubmatch(page, -1) {
			got = append(got, m[1])
		}
		return strings.Join(got, ",")
	}
	code, out, errs := runDeploy(t, "production", dir)
	if code != 0 {
		t.Fatalf("deploy: %d\n%s%s", code, out, errs)
	}
	id1 := deployedID(t, out)
	post("Ada")
	migrateDemo(t, "  message: Text.NonEmpty,\n", "  message: Text.NonEmpty,\n  mood: Maybe Text = Nothing,\n", "add_mood")
	if code, out, errs = runDeploy(t, "production", dir); code != 0 {
		t.Fatalf("second deploy: %d\n%s%s", code, out, errs)
	}
	id2 := deployedID(t, out)
	post("Grace")
	if got := names(); got != "Grace,Ada" {
		t.Fatalf("entries %q", got)
	}
	// a migration, then a build that exits at start
	migrateDemo(t, "  mood: Maybe Text = Nothing,\n", "  mood: Maybe Text = Nothing,\n  tag: Maybe Text = Nothing,\n", "add_tag")
	writeFile(t, "demo/app/Clock.alder", "module Clock\n\ncapability Clock {\n  now : Task Int,\n}\n")
	writeFile(t, "demo/adapters/clock/clock.go", "package clock\n\nimport (\n\t\"context\"\n\t\"os\"\n\n\t\"app\"\n)\n\nfunc Clock() app.Cap__Clock__Clock {\n\tif os.Getenv(\"ALDER_DB\") != \"\" {\n\t\tos.Exit(1)\n\t}\n\treturn clock{}\n}\n\ntype clock struct{}\n\nfunc (clock) Now(context.Context) (int64, error) { return 0, nil }\n")
	code, out, errs = runDeploy(t, "production", dir, "--unsafe-skip-tests")
	m := regexp.MustCompile(`builds/([0-9a-f]{16})/ is kept`).FindStringSubmatch(errs)
	if code != 1 || m == nil || !strings.Contains(errs, "build "+id2+" still serves, but the database was migrated (0003_add_tag) and that build would refuse to start again on it. To go back: `alder rollback production "+id2+" --restore-backup` (puts builds/"+m[1]+"/backup.db, taken before the migration, in place)") {
		t.Fatalf("I2: %d\n%s%s", code, out, errs)
	}
	var so, se bytes.Buffer
	if code := runRollbackTo([]string{"production", id2, "--dir", dir, "--restore-backup", "--yes"}, &so, &se); code != 0 || !strings.Contains(so.String(), "warning: --restore-backup: the database is now builds/"+m[1]+"/backup.db") {
		t.Fatalf("rollback %s --restore-backup: %d\n%s%s", id2, code, so.String(), se.String())
	}
	if got, serving := names(), box.serving(t); got != "Grace,Ada" || serving != id2 {
		t.Errorf("after restoring %s's copy: entries %q, serving %s", id2, got, serving)
	}
	// across the migration to the first build: its data from before the migration
	so.Reset()
	se.Reset()
	if code := runRollbackTo([]string{"production", id1, "--dir", dir, "--restore-backup", "--yes"}, &so, &se); code != 0 {
		t.Fatalf("rollback %s --restore-backup: %d\n%s%s", id1, code, so.String(), se.String())
	}
	if got, serving := names(), box.serving(t); got != "Ada" || serving != id1 {
		t.Errorf("after rolling back to %s: entries %q, serving %s", id1, got, serving)
	}
	if kept := box.sh(t, fmt.Sprintf("ls %s/db", box.qr())); strings.Count(kept, "app.db.before-rollback-") < 2 {
		t.Errorf("the replaced databases are not kept: %s", kept)
	}
	var rec deploy.Record
	if err := json.Unmarshal([]byte(box.sh(t, fmt.Sprintf("cat %s/builds/%s/deployed.json", box.qr(), id1))), &rec); err != nil || !rec.RolledBack || rec.Previous != id2 || !strings.HasSuffix(rec.RestoredFrom, "/backup.db") {
		t.Errorf("deployed.json after the rollback: %+v %v", rec, err)
	}
}

// I8: a deploy whose migration drops data (--allow-data-loss) and whose build then fails its
// health check: the previous build is already running on a database its tables no longer
// match, so the deploy puts this run's pre-migration copy back at once and starts the previous
// build on it. The site serves the old build, with the dropped column back, and takes writes.
func TestDeploySsh_DropsDataAutoRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	box := startBox(t)
	dir := sshScaffold(t, box)
	box.sh(t, fmt.Sprintf("printf 'ALDER_SECRET=%s\\nSMTP_URL=smtp://mail\\n' > %s/env", strings.Repeat("s", 32), box.qr()))
	code, out, errs := runDeploy(t, "production", dir)
	if code != 0 {
		t.Fatalf("deploy: %d\n%s%s", code, out, errs)
	}
	box.post(t, "Ada")
	migrateDemo(t, "  message: Text.NonEmpty,\n", "  message: Text.NonEmpty,\n  mood: Maybe Text = Nothing,\n", "add_mood")
	if code, out, errs = runDeploy(t, "production", dir); code != 0 {
		t.Fatalf("second deploy: %d\n%s%s", code, out, errs)
	}
	id2 := deployedID(t, out)
	box.post(t, "Grace")
	// drop the column, and a build that exits at start
	migrateDemo(t, "  mood: Maybe Text = Nothing,\n", "", "drop_mood")
	writeFile(t, "demo/app/Clock.alder", "module Clock\n\ncapability Clock {\n  now : Task Int,\n}\n")
	writeFile(t, "demo/adapters/clock/clock.go", "package clock\n\nimport (\n\t\"context\"\n\t\"os\"\n\n\t\"app\"\n)\n\nfunc Clock() app.Cap__Clock__Clock {\n\tif os.Getenv(\"ALDER_DB\") != \"\" {\n\t\tos.Exit(1)\n\t}\n\treturn clock{}\n}\n\ntype clock struct{}\n\nfunc (clock) Now(context.Context) (int64, error) { return 0, nil }\n")
	code, out, errs = runDeploy(t, "production", dir, "--allow-data-loss", "--unsafe-skip-tests")
	m := regexp.MustCompile(`builds/([0-9a-f]{16})/ is kept for inspection`).FindStringSubmatch(errs)
	if code != 1 || m == nil || !strings.Contains(errs, "did not pass its health check after its migrations dropped data (0003_drop_mood): the database copied before them (builds/"+m[1]+"/backup.db) was put back automatically and build "+id2+" serves again on it; the migrated database is kept as db/app.db.before-rollback-") {
		t.Fatalf("I8: %d\n%s%s", code, out, errs)
	}
	if !strings.Contains(out, "  restore  ok  builds/"+m[1]+"/backup.db (the database it replaced is kept as db/app.db.before-rollback-") || !strings.Contains(out, "  switch   ok  current → "+id2) {
		t.Errorf("I8 output:\n%s", out)
	}
	if got := box.serving(t); got != id2 {
		t.Errorf("after the automatic restore the box serves %q, want %s", got, id2)
	}
	box.post(t, "Ken")
	if got := box.names(t); got != "Ken,Grace,Ada" {
		t.Errorf("after the automatic restore: %q", got)
	}
	if got := dbNames(t, box); got != "Ada,Grace,Ken" {
		t.Errorf("app.db holds %q", got)
	}
	ddl := box.sh(t, fmt.Sprintf("cat %s/db/schema.json", box.qr()))
	if !strings.Contains(ddl, `"mood"`) {
		t.Errorf("db/schema.json is not build %s's again:\n%s", id2, ddl)
	}
}
