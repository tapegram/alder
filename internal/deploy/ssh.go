package deploy

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"alder/internal/ir"
)

// EnvSSHConfig names an ssh_config file every ssh the Ssh target runs reads (`ssh -F`): an
// identity, a known-hosts file, a jump host. The tests point it at their own sshd.
const EnvSSHConfig = "ALDER_SSH_CONFIG"

// EnvSupervisor forces how the Ssh target runs a build: "process" (a background process with a
// pid file) even where systemd is present. Without it, systemd is used when it runs on the host.
const EnvSupervisor = "ALDER_DEPLOY_SUPERVISOR"

// EnvCaddyAdmin is the address of Caddy's admin API at the box (default DefaultCaddyAdmin):
// for a Caddy that listens elsewhere, and for the tests' fake Caddy.
const EnvCaddyAdmin = "ALDER_DEPLOY_CADDY_ADMIN"

// DefaultCaddyAdmin is where Caddy's admin API listens unless configured otherwise.
const DefaultCaddyAdmin = "127.0.0.1:2019"

// DefaultPublicPort is the port the app listens on with `proxy: None` when <path>/port does not
// say another.
const DefaultPublicPort = 8080

// Ssh is the target `Ssh "user@host" { domain, path, user?, port? }`: a Linux box (or any Unix
// with sh, tar and curl) reached with the system's ssh, its state under path:
//
//	<path>/builds/<id>/   app, static/, migrations/, alder-deploy.json, run, addr, log, pid,
//	                      backup.db (the copy before its migrations), deployed.json
//	<path>/current        the id of the build serving
//	<path>/port           the port it serves on (with `proxy: None`, the public port: default 8080)
//	<path>/env            NAME=value lines, written by the operator (mode 600); deploy only checks
//	                      that the `requires` names are set, never reads a value
//	<path>/db/app.db      the database (ALDER_DB), and db/schema.json, its schema snapshot
//
// A build runs under a systemd unit alder-<app>-<env>@<id>.service (a template: the old and the
// new build run side by side during the switch) when systemd runs on the host, else as a
// background process with a pid file; either way its output is builds/<id>/log. With `proxy:
// Caddy` (the default) the domain is routed to the build's port through Caddy's admin API
// (Caddy is installed with apt on Debian and Ubuntu when absent; automatic TLS); with `proxy:
// None` the app itself listens on <path>/port.
type Ssh struct {
	App, Env  string
	Dest      string // the ssh destination: user@host
	Port      string // the ssh port ("" for ssh's default)
	Root      string // <path>
	Domain    string
	Proxy     string // "Caddy" or "None"
	BackupURL string
	Warn      io.Writer // warnings (the environment file's mode, linger)
	// CaddyAdmin is the host:port of Caddy's admin API at the box (EnvCaddyAdmin, else
	// DefaultCaddyAdmin).
	CaddyAdmin string

	user string // the remote user, from Connect
	host string
	plat Platform
	sup  string // "system", "user" (systemd) or "process"
	conn bool

	allowDataLoss bool   // this run's --allow-data-loss (DataLossAllower)
	ctl           string // the shared connection's control socket ("" for none)
	unitDone      bool   // the unit was installed in this run
	caddyDone     bool   // Caddy answered in this run
}

// AllowDataLoss is DataLossAllower: Migrate passes ALDER_ALLOW_DATA_LOSS=1 to the binary.
func (s *Ssh) AllowDataLoss(allow bool) { s.allowDataLoss = allow }

// NewSsh is environment env's Ssh target t of app, with the declaration's proxy and backup.
func NewSsh(app, env string, t ir.Target, d *ir.Deploy) *Ssh {
	dest := t.Host
	if t.User != "" {
		if _, h, ok := strings.Cut(dest, "@"); ok {
			dest = h
		}
		dest = t.User + "@" + dest
	}
	s := &Ssh{App: app, Env: env, Dest: dest, Port: t.Port, Root: strings.TrimRight(t.Path, "/"), Domain: t.Domain, Proxy: d.Proxy, Warn: io.Discard, CaddyAdmin: DefaultCaddyAdmin}
	if a := os.Getenv(EnvCaddyAdmin); a != "" {
		s.CaddyAdmin = a
	}
	if s.Proxy == "" {
		s.Proxy = "Caddy"
	}
	if d.Backup != nil {
		s.BackupURL = d.Backup.URL
	}
	return s
}

// Describe is "Ssh deploy@box:/srv/blog".
func (s *Ssh) Describe() string { return "Ssh " + s.Dest + ":" + s.Root }

// Identity is the remote user and host, for the record.
func (s *Ssh) Identity() string { return s.user + "@" + s.host }

// Close does nothing: every command is its own ssh.
func (s *Ssh) Close() error {
	if s.ctl == "" {
		return nil
	}
	exec.Command("ssh", append(s.baseArgs(), "-o", "ControlPath="+s.ctl, "-O", "exit", "--", s.Dest)...).Run()
	os.RemoveAll(filepath.Dir(s.ctl))
	s.ctl = ""
	return nil
}

// master starts one ssh connection for the run (ControlMaster; ControlPersist bounds it if this
// process dies) that every later ssh shares. Without it (it did not start), each ssh connects
// on its own.
func (s *Ssh) master(ctx context.Context) {
	dir, err := os.MkdirTemp(shortTemp(), "alder-ssh-")
	if err != nil {
		return
	}
	ctl := filepath.Join(dir, "c")
	args := append(s.baseArgs(), "-o", "ControlMaster=yes", "-o", "ControlPersist=10m", "-o", "ControlPath="+ctl, "-N", "-f", "--", s.Dest)
	cmd := exec.CommandContext(ctx, "ssh", args...) // no pipes: the background master holds none of ours
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return
	}
	if _, err := os.Stat(ctl); err != nil { // not a real ssh (the tests' loopback): no socket
		os.RemoveAll(dir)
		return
	}
	s.ctl = ctl
}

// shortTemp is a directory for the control socket: /tmp when it exists (a socket's path has a
// short limit, and macOS's TMPDIR is long).
func shortTemp() string {
	if fi, err := os.Stat("/tmp"); err == nil && fi.IsDir() {
		return "/tmp"
	}
	return os.TempDir()
}

// unit is the systemd template unit's name, without ".service".
func (s *Ssh) unit() string { return "alder-" + unitName(s.App) + "-" + unitName(s.Env) }

var unitUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func unitName(s string) string { return unitUnsafe.ReplaceAllString(s, "-") }

// q quotes s for the remote POSIX shell.
func q(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// sshArgs are the arguments of every ssh before the remote command: the options, the run's
// shared connection when there is one, and `--` before the destination (a destination is never
// read as an option).
func (s *Ssh) sshArgs() []string {
	args := s.baseArgs()
	if s.ctl != "" {
		args = append(args, "-o", "ControlMaster=no", "-o", "ControlPath="+s.ctl)
	}
	return append(args, "--", s.Dest)
}

// baseArgs are the options every ssh of the target takes.
func (s *Ssh) baseArgs() []string {
	args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=20", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=4"}
	if cfg := os.Getenv(EnvSSHConfig); cfg != "" {
		args = append(args, "-F", cfg)
	}
	if s.Port != "" {
		args = append(args, "-p", s.Port)
	}
	return args
}

// RemoteError is a remote command that failed: ssh's exit 255 is the connection; any other code is the script's.
type RemoteError struct {
	Dest   string
	Code   int
	Stderr string
}

func (e *RemoteError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if e.Code == 255 {
		return fmt.Sprintf("cannot reach %s over SSH: %s (ssh uses your keys and ~/.ssh/config; %s may name another config file)", e.Dest, orNone(lastLines(msg, 5)), EnvSSHConfig)
	}
	return fmt.Sprintf("a command at %s failed (exit %d): %s", e.Dest, e.Code, orNone(lastLines(msg, 20)))
}

// run runs the shell script script at the target (`sh -s`, the script on standard input, after
// the prelude: R is the root, helper functions) and returns its standard output.
func (s *Ssh) run(ctx context.Context, script string) (string, error) {
	var out, errb bytes.Buffer
	if err := s.stream(ctx, script, &out, &errb); err != nil {
		var re *RemoteError
		if errors.As(err, &re) && re.Stderr == "" {
			re.Stderr = errb.String()
		}
		return out.String(), err
	}
	return out.String(), nil
}

// stream is run with the script's output going to stdout and stderr as it comes.
func (s *Ssh) stream(ctx context.Context, script string, stdout, stderr io.Writer) error {
	var errb bytes.Buffer
	cmd := exec.CommandContext(ctx, "ssh", append(s.sshArgs(), "sh -s")...)
	cmd.Stdin = strings.NewReader(s.prelude() + script)
	cmd.Stdout = stdout
	cmd.Stderr = io.MultiWriter(stderr, &errb)
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return &RemoteError{Dest: s.Dest, Code: ee.ExitCode(), Stderr: errb.String()}
		}
		return fmt.Errorf("running ssh: %w", err)
	}
	return nil
}

// prelude sets, for every script, umask 077 (what the deploy writes is the deploy user's
// alone) and defines R (the root), U (the unit), SUP (the supervisor), alive ID (whether build
// ID's process runs), stopnow ID and cancel_later ID.
func (s *Ssh) prelude() string {
	return `umask 077
R=` + q(s.Root) + `
U=` + q(s.unit()) + `
SUP=` + q(s.sup) + `
SCTL="systemctl"; [ "$SUP" = user ] && SCTL="systemctl --user"
alive() {
  if [ "$SUP" = system ] || [ "$SUP" = user ]; then $SCTL is-active --quiet "$U@$1.service"; return; fi
  p=$(cat "$R/builds/$1/pid" 2>/dev/null) || return 1
  [ -n "$p" ] && kill -0 "$p" 2>/dev/null || return 1
  if [ -r "/proc/$p/cmdline" ]; then tr '\0' ' ' < "/proc/$p/cmdline" | grep -q "builds/$1/"; fi
}
` + s.stopNow() + `
# cancel_later ID: a pending stop-later of build ID (a drain in progress) no longer fires
cancel_later() {
  if [ -f "$R/builds/$1/stop-later.pid" ]; then kill "$(cat "$R/builds/$1/stop-later.pid")" 2>/dev/null; rm -f "$R/builds/$1/stop-later.pid"; fi
}
`
}

// stopNow is the shell function stopnow ID: SIGTERM (the app lets its requests finish, 5 s at
// most), then SIGKILL after 40 s; with systemd, the unit is stopped and disabled.
func (s *Ssh) stopNow() string {
	if s.sup != "process" && s.sup != "" {
		return `stopnow() { $SCTL disable "$U@$1.service" >/dev/null 2>&1; $SCTL stop "$U@$1.service" >/dev/null 2>&1; return 0; }`
	}
	return `stopnow() {
  sp=$(cat "$R/builds/$1/pid" 2>/dev/null) || return 0
  alive "$1" || { rm -f "$R/builds/$1/pid"; return 0; }
  kill -TERM "$sp" 2>/dev/null
  si=0; while alive "$1" && [ $si -lt 200 ]; do sleep 0.2; si=$((si + 1)); done
  alive "$1" && kill -KILL "$sp" 2>/dev/null
  rm -f "$R/builds/$1/pid"; return 0
}`
}

// Connect checks the connection and reads the user, the platform and the supervisor.
func (s *Ssh) Connect(ctx context.Context) (Platform, error) {
	if s.conn {
		return s.plat, nil
	}
	if s.ctl == "" {
		s.master(ctx)
	}
	out, err := s.run(ctx, `echo "user=$(id -un)"
echo "host=$(hostname 2>/dev/null || uname -n)"
echo "os=$(uname -s)"
echo "arch=$(uname -m)"
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  if [ "$(id -u)" = 0 ]; then echo "systemd=system"
  elif systemctl --user show-environment >/dev/null 2>&1; then echo "systemd=user"; fi
fi
for c in curl tar; do command -v $c >/dev/null 2>&1 || echo "lack=$c"; done
`)
	if err != nil {
		return Platform{}, err
	}
	kv := keyValues(out)
	s.user, s.host = kv["user"], kv["host"]
	if l := kv["lack"]; l != "" {
		return Platform{}, fmt.Errorf("%s has no `%s` (the deploy needs sh, tar and curl there): install it and deploy again", s.Dest, l)
	}
	goos := map[string]string{"Linux": "linux", "Darwin": "darwin", "FreeBSD": "freebsd"}[kv["os"]]
	goarch := map[string]string{"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[kv["arch"]]
	if goos == "" || goarch == "" {
		return Platform{}, fmt.Errorf("%s runs %s on %s, which alder cannot build for yet (Linux or macOS on x86_64 or arm64)", s.Dest, kv["os"], kv["arch"])
	}
	s.plat = Platform{goos, goarch}
	s.sup = "process"
	if v := kv["systemd"]; v != "" && os.Getenv(EnvSupervisor) != "process" {
		s.sup = v
	}
	s.conn = true
	return s.plat, nil
}

// keyValues are the key=value lines of out (the last of each key wins).
func keyValues(out string) map[string]string {
	kv := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			kv[k] = v
		}
	}
	return kv
}

// Requires checks <path>/env for each name: a line NAME=… with a non-empty value. No value is
// read back: grep's exit status is all that crosses the connection.
func (s *Ssh) Requires(ctx context.Context, names []string) ([]string, error) {
	var b strings.Builder
	b.WriteString(`[ -f "$R/env" ] && echo "mode=$(ls -l "$R/env" | cut -c1-10)"` + "\n")
	b.WriteString(`grep -Eq '^(export[[:space:]]+)?ALDER_DEV=' "$R/env" 2>/dev/null && echo devmode=yes` + "\n")
	for _, n := range names {
		fmt.Fprintf(&b, "grep -Eq %s \"$R/env\" 2>/dev/null || echo missing=%s\n", q(`^(export[[:space:]]+)?`+n+`=([^[:space:]"']|"[^"]|'[^'])`), q(n))
	}
	out, err := s.run(ctx, b.String())
	if err != nil {
		return nil, err
	}
	if keyValues(out)["devmode"] == "yes" {
		return nil, refuse("", "the environment file %s:%s/env sets ALDER_DEV: development mode (the public cookie key, errors shown to visitors) never runs at a target; remove the line and deploy again", s.Dest, s.Root)
	}
	var missing []string
	for _, l := range strings.Split(out, "\n") {
		if n, ok := strings.CutPrefix(l, "missing="); ok {
			missing = append(missing, n)
		}
		if m, ok := strings.CutPrefix(l, "mode="); ok && len(m) == 10 && (m[4] != '-' || m[7] != '-') {
			fmt.Fprintf(s.Warn, "warning: %s:%s/env is readable by others (%s): `chmod 600` it\n", s.Dest, s.Root, m)
		}
	}
	return missing, nil
}

// Schema is db/schema.json and db/migrations (the snapshot and the migration names of the last
// build that migrated the database), or the current build's own when the database predates
// them; Have false without a database.
func (s *Ssh) Schema(ctx context.Context) (Schema, error) {
	out, err := s.run(ctx, `[ -f "$R/db/app.db" ] || { echo none; exit 0; }
c=$(cat "$R/current" 2>/dev/null)
echo db
if [ -f "$R/db/migrations" ]; then cat "$R/db/migrations"; else ls "$R/builds/$c/migrations" 2>/dev/null | sed -n 's/\.sql$//p'; fi
echo @@schema
if [ -f "$R/db/schema.json" ]; then cat "$R/db/schema.json"
elif [ -f "$R/builds/$c/migrations/schema.json" ]; then cat "$R/builds/$c/migrations/schema.json"
else echo '{"tables": []}'; fi
`)
	if err != nil {
		return Schema{}, err
	}
	return parseSchema(out, "db"), nil
}

// parseSchema reads the output of Schema's and BuildSchema's scripts: the marker line, the
// migration names one per line, @@schema, the snapshot.
func parseSchema(out, marker string) Schema {
	first, rest, _ := strings.Cut(out, "\n")
	if first != marker {
		return Schema{}
	}
	names, snap, _ := strings.Cut(rest, "@@schema\n")
	sc := Schema{Have: true, Snapshot: []byte(snap)}
	for _, n := range strings.Fields(names) {
		sc.Migrations = append(sc.Migrations, n)
	}
	return sc
}

// BuildSchema is builds/<id>/migrations/: schema.json and the files' names.
func (s *Ssh) BuildSchema(ctx context.Context, id string) (Schema, error) {
	out, err := s.run(ctx, `d="$R/builds/"`+q(id)+`/migrations; [ -f "$d/schema.json" ] || { echo none; exit 0; }
echo yes
ls "$d" | sed -n 's/\.sql$//p'
echo @@schema
cat "$d/schema.json"
`)
	if err != nil {
		return Schema{}, err
	}
	return parseSchema(out, "yes"), nil
}

// Current is <path>/current.
func (s *Ssh) Current(ctx context.Context) (string, error) {
	out, err := s.run(ctx, `cat "$R/current" 2>/dev/null; exit 0`)
	return strings.TrimSpace(out), err
}

var idRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Builds lists builds/, newest first, with each one's deployed.json and whether it runs.
func (s *Ssh) Builds(ctx context.Context) ([]Build, error) {
	out, err := s.run(ctx, `cd "$R/builds" 2>/dev/null || exit 0
cur=$(cat "$R/current" 2>/dev/null)
for d in */; do
  d=${d%/}; [ -d "$d" ] || continue
  run=no; alive "$d" && run=yes
  echo "@@build $d $(date -r "$d" +%s 2>/dev/null || echo 0) $run $(cat "$d/backup.of" 2>/dev/null)"
  cat "$d/deployed.json" 2>/dev/null
done
exit 0
`)
	if err != nil {
		return nil, err
	}
	cur, _ := s.Current(ctx)
	var builds []Build
	for _, chunk := range strings.Split(out, "@@build ")[1:] {
		head, body, _ := strings.Cut(chunk, "\n")
		f := strings.Fields(head)
		if len(f) < 3 || !idRe.MatchString(f[0]) {
			continue
		}
		secs, _ := strconv.ParseInt(f[1], 10, 64)
		b := Build{ID: f[0], Modified: time.Unix(secs, 0), Running: f[2] == "yes", Current: f[0] == cur}
		if len(f) > 3 && idRe.MatchString(f[3]) {
			b.BackupOf = f[3]
		}
		if strings.TrimSpace(body) != "" {
			var r Record
			if json.Unmarshal([]byte(body), &r) == nil {
				b.Record = &r
			}
		}
		builds = append(builds, b)
	}
	sort.SliceStable(builds, func(i, j int) bool { return newer(builds[i], builds[j]) })
	return builds, nil
}

// newer orders builds by their deploy time, then their directory's.
func newer(a, b Build) bool {
	ta, tb := a.Modified, b.Modified
	if a.Record != nil && a.Record.When != nil {
		ta = *a.Record.When
	}
	if b.Record != nil && b.Record.When != nil {
		tb = *b.Record.When
	}
	if !ta.Equal(tb) {
		return ta.After(tb)
	}
	return a.ID < b.ID
}

// Upload sends the artifact dir as builds/<id>/ (through builds/<id>.part, so a build directory
// is never half written), with the build's run script, and creates db/.
func (s *Ssh) Upload(ctx context.Context, dir, id string) error {
	var tarball bytes.Buffer
	if err := tarDir(&tarball, dir); err != nil {
		return err
	}
	part := s.Root + "/builds/" + id + ".part"
	cmd := exec.CommandContext(ctx, "ssh", append(s.sshArgs(), "umask 077 && rm -rf "+q(part)+" && mkdir -p "+q(part)+" && tar -xf - -C "+q(part))...)
	cmd.Stdin = &tarball
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		code := -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return &RemoteError{Dest: s.Dest, Code: code, Stderr: errb.String()}
	}
	_, err := s.run(ctx, `set -e
id=`+q(id)+`
mkdir -p "$R/db"; chmod 700 "$R/db"; chmod 750 "$R/builds"
if [ -d "$R/builds/$id" ]; then
  if alive "$id"; then echo "build $id is running" >&2; exit 1; fi
  # an earlier upload of the same build: its record and pre-migration copy stay
  for f in deployed.json backup.db backup.of log; do [ -f "$R/builds/$id/$f" ] && mv "$R/builds/$id/$f" "$R/builds/$id.part/$f"; done
  rm -rf "$R/builds/$id"
fi
mv "$R/builds/$id.part" "$R/builds/$id"
cat > "$R/builds/$id/run" <<'ALDER_RUN_EOF'
`+s.runScript(id)+`ALDER_RUN_EOF
chmod 700 "$R/builds/$id/run" "$R/builds/$id/app"
echo 127.0.0.1:0 > "$R/builds/$id/addr"
`)
	return err
}

// runScript is builds/<id>/run: it reads the operator's environment file, points the app at the
// database, its static files, its address and the backup location, and execs the binary with
// its arguments. It holds no value of the environment file.
func (s *Ssh) runScript(id string) string {
	b := s.Root + "/builds/" + id
	backup := ""
	if s.BackupURL != "" {
		backup = "export ALDER_BACKUP_URL=" + q(s.BackupURL) + "\n"
	}
	return `#!/bin/sh
# Written by alder deploy: runs build ` + id + ` of ` + s.App + ` (environment ` + s.Env + `). The
# variables come from ` + s.Root + `/env, which the operator writes; nothing here is a secret.
set -e
if [ -f ` + q(s.Root+"/env") + ` ]; then set -a; . ` + q(s.Root+"/env") + `; set +a; fi
unset ALDER_DEV  # development mode never runs at a target
export ALDER_DB=` + q(s.Root+"/db/app.db") + `
export ALDER_STATIC=` + q(b+"/static") + `
export ALDER_ADDR="$(cat ` + q(b+"/addr") + ` 2>/dev/null || echo 127.0.0.1:0)"
` + backup + `exec ` + q(b+"/app") + ` "$@"
`
}

// tarDir writes dir as a tar stream (paths relative to dir; modes kept).
func tarDir(w io.Writer, dir string) error {
	tw := tar.NewWriter(w)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == "." {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Uname, h.Gname, h.Uid, h.Gid = "", "", 0, 0
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if d.Type().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

// Backup copies the database into builds/<id>/backup.db with the new build's binary (VACUUM
// INTO, beside the serving build), before any migration.
func (s *Ssh) Backup(ctx context.Context, id string) (string, error) {
	out, err := s.run(ctx, `id=`+q(id)+`
[ -f "$R/db/app.db" ] || exit 0
"$R/builds/$id/run" --backup-to "$R/builds/$id/backup.db" >&2 || exit 1
chmod 600 "$R/builds/$id/backup.db"
cat "$R/current" > "$R/builds/$id/backup.of" 2>/dev/null
echo "backup=builds/$id/backup.db"
`)
	if err != nil {
		return "", err
	}
	return keyValues(out)["backup"], nil
}

// Migrate runs the new build's --migrate-only (with ALDER_ALLOW_DATA_LOSS=1 only when this run
// has --allow-data-loss: the binary refuses a migration that drops data otherwise), and on
// success records its snapshot as db/schema.json and its migrations as db/migrations.
func (s *Ssh) Migrate(ctx context.Context, id string) (string, error) {
	allow := ""
	if s.allowDataLoss {
		allow = "ALDER_ALLOW_DATA_LOSS=1 "
	}
	out, err := s.run(ctx, `id=`+q(id)+`
`+allow+`"$R/builds/$id/run" --migrate-only 2>&1 || exit 3
if [ -f "$R/builds/$id/migrations/schema.json" ]; then
  cp "$R/builds/$id/migrations/schema.json" "$R/db/schema.json.new" && mv "$R/db/schema.json.new" "$R/db/schema.json"
  ls "$R/builds/$id/migrations" | sed -n 's/\.sql$//p' > "$R/db/migrations.new" && mv "$R/db/migrations.new" "$R/db/migrations"
fi
`)
	var re *RemoteError
	if errors.As(err, &re) && re.Code == 3 {
		return out, errors.New("--migrate-only refused")
	}
	return out, err
}

// Start starts build id on a free port of 127.0.0.1.
func (s *Ssh) Start(ctx context.Context, id string) (int, error) {
	return s.start(ctx, id, "127.0.0.1:0")
}

// start starts build id listening on addr and waits (15 s at most) for its "listening on" line;
// a build that exits first is an error with its log.
func (s *Ssh) start(ctx context.Context, id, addr string) (int, error) {
	// detached: no terminal, output to the log, SIGHUP ignored (kept across exec; not nohup,
	// which macOS refuses outside a console session)
	launch := `(trap '' HUP; exec "$B/run") >> "$B/log" 2>&1 < /dev/null &
echo $! > "$B/pid"
`
	if s.sup != "process" {
		if !s.unitDone {
			if err := s.installUnit(ctx); err != nil {
				return 0, err
			}
		}
		launch = `$SCTL reset-failed "$U@$id.service" >/dev/null 2>&1
$SCTL start "$U@$id.service" || { echo exited; tail -n 40 "$B/log"; exit 0; }
`
	}
	out, err := s.run(ctx, `id=`+q(id)+`
B="$R/builds/$id"
cancel_later "$id"
if alive "$id"; then
  # still running (a rollback inside the drain window): keep it when it listens where wanted
  a=$(cat "$B/addr" 2>/dev/null); p=${a##*:}
  case "$p" in ''|0|*[!0-9]*) ;; *)
    if [ `+q(addr)+` = 127.0.0.1:0 ] || [ `+q(addr)+` = "$a" ]; then echo "port=$p"; exit 0; fi;;
  esac
  stopnow "$id"
fi
echo `+q(addr)+` > "$B/addr"
touch "$B/log"
off=$(wc -c < "$B/log" | tr -d ' ')
echo "--- alder deploy: start $(date -u +%Y-%m-%dT%H:%M:%SZ) on `+addr+`" >> "$B/log"
`+launch+`i=0
while [ $i -lt 75 ]; do
  port=$(tail -c +$((off + 1)) "$B/log" | sed -n 's/^listening on .*:\([0-9][0-9]*\)$/\1/p' | tail -n 1)
  if [ -n "$port" ]; then
    case "`+addr+`" in 127.0.0.1:0) echo "127.0.0.1:$port" > "$B/addr";; esac
    echo "port=$port"; exit 0
  fi
  if ! alive "$id"; then sleep 0.3; echo exited; tail -c +$((off + 1)) "$B/log" | tail -n 40; exit 0; fi
  sleep 0.2; i=$((i + 1))
done
echo timeout; tail -c +$((off + 1)) "$B/log" | tail -n 40
`)
	if err != nil {
		return 0, err
	}
	first, rest, _ := strings.Cut(out, "\n")
	if p, ok := strings.CutPrefix(first, "port="); ok {
		return strconv.Atoi(strings.TrimSpace(p))
	}
	if first == "exited" {
		return 0, fmt.Errorf("build %s exited at start; its log (builds/%s/log):\n%s", id, id, strings.TrimRight(rest, "\n"))
	}
	s.stop(ctx, id)
	return 0, fmt.Errorf("build %s did not start listening within 15s; its log (builds/%s/log):\n%s", id, id, strings.TrimRight(rest, "\n"))
}

// installUnit writes the systemd template unit when it differs, and reloads systemd.
func (s *Ssh) installUnit(ctx context.Context) error {
	dir, wanted := "/etc/systemd/system", "multi-user.target"
	if s.sup == "user" {
		dir, wanted = `"$HOME/.config/systemd/user"`, "default.target"
	} else {
		dir = q(dir)
	}
	sandbox := ""
	if s.sup == "system" {
		sandbox = "User=" + s.user + "\nProtectSystem=full\nPrivateTmp=yes\nNoNewPrivileges=yes\n"
	}
	// systemd's own quoting: ExecStart's path in double quotes (backslash and quote escaped); a
	// literal % doubled everywhere (the specifier %i is the build)
	raw := strings.ReplaceAll(s.Root, "%", "%%")
	quoted := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(raw) + `/builds/%i/run"`
	unit := `[Unit]
Description=alder ` + s.App + ` ` + s.Env + ` (build %i)
After=network-online.target

[Service]
Type=simple
WorkingDirectory=` + raw + `/builds/%i
ExecStart=` + quoted + `
StandardOutput=append:` + raw + `/builds/%i/log
StandardError=append:` + raw + `/builds/%i/log
UnsetEnvironment=ALDER_DEV
Restart=on-failure
RestartSec=2
TimeoutStopSec=40
` + sandbox + `
[Install]
WantedBy=` + wanted + `
`
	out, err := s.run(ctx, `D=`+dir+`
mkdir -p "$D"
f="$D/$U@.service"
cat > "$f.new" <<'ALDER_UNIT_EOF'
`+unit+`ALDER_UNIT_EOF
if cmp -s "$f.new" "$f"; then rm "$f.new"; else mv "$f.new" "$f"; $SCTL daemon-reload; fi
if [ "$SUP" = user ] && [ "$(loginctl show-user "$(id -un)" -p Linger 2>/dev/null)" != "Linger=yes" ]; then
  loginctl enable-linger "$(id -un)" 2>/dev/null || echo nolinger
fi
`)
	if err == nil {
		s.unitDone = true
	}
	if strings.Contains(out, "nolinger") {
		fmt.Fprintf(s.Warn, "warning: user %s has no lingering systemd session: the app stops when the user logs out and does not start at boot; run `sudo loginctl enable-linger %s`\n", s.user, s.user)
	}
	return err
}

// Health waits until GET http://127.0.0.1:<port>/ (following redirects) is a page whose
// alder-build meta names id.
func (s *Ssh) Health(ctx context.Context, id string, port int, timeout time.Duration) error {
	secs := int(timeout / time.Second)
	if secs < 1 {
		secs = 1
	}
	out, err := s.run(ctx, `id=`+q(id)+`
end=$(( $(date +%s) + `+strconv.Itoa(secs)+` ))
while :; do
  if curl -sL --max-time 3 "http://127.0.0.1:`+strconv.Itoa(port)+`/" 2>/dev/null | grep -qF "name=\"alder-build\" content=\"$id\""; then echo ok; exit 0; fi
  if ! alive "$id"; then echo exited; break; fi
  if [ "$(date +%s)" -ge "$end" ]; then echo timeout; break; fi
  sleep 1
done
tail -n 40 "$R/builds/$id/log"
`)
	if err != nil {
		return err
	}
	first, rest, _ := strings.Cut(out, "\n")
	switch first {
	case "ok":
		return nil
	case "exited":
		return fmt.Errorf("build %s exited before answering; its log (builds/%s/log):\n%s", id, id, strings.TrimRight(rest, "\n"))
	}
	return fmt.Errorf("build %s did not answer GET / with its build id within %ds; its log (builds/%s/log):\n%s", id, secs, id, strings.TrimRight(rest, "\n"))
}

// Switch routes the traffic to build id on port, makes it current and returns the port it
// serves on: with Caddy, port (the route's upstream changes); with `proxy: None`, the public
// port, after the previous build is stopped and id restarted on it (a moment without a
// listener; when that fails the previous build is started again).
func (s *Ssh) Switch(ctx context.Context, id string, port int) (int, error) {
	prev, err := s.Current(ctx)
	if err != nil {
		return 0, err
	}
	if s.Proxy == "None" {
		out, err := s.run(ctx, `p=$(cat "$R/port" 2>/dev/null | tr -d ' \n'); [ -n "$p" ] || { p=`+strconv.Itoa(DefaultPublicPort)+`; echo "$p" > "$R/port"; }; echo "$p"`)
		if err != nil {
			return 0, err
		}
		public := strings.TrimSpace(out)
		if prev != "" && prev != id {
			if err := s.stop(ctx, prev); err != nil {
				return 0, err
			}
		}
		if err := s.stop(ctx, id); err != nil {
			return 0, err
		}
		p, err := s.start(ctx, id, ":"+public)
		if err == nil {
			err = s.Health(ctx, id, p, 30*time.Second)
		}
		if err != nil {
			s.stop(ctx, id)
			if prev != "" && prev != id {
				if _, perr := s.start(ctx, prev, ":"+public); perr != nil {
					return 0, fmt.Errorf("%v; and build %s did not start again: %v", err, prev, perr)
				}
				return 0, fmt.Errorf("%v; build %s was started again on port %s", err, prev, public)
			}
			return 0, err
		}
		port = p
	} else if err := s.caddy(ctx, port); err != nil {
		return 0, err
	}
	enable := ""
	if s.sup != "process" {
		enable = `$SCTL enable "$U@$id.service" >/dev/null 2>&1 || true` + "\n"
	}
	_, err = s.run(ctx, `id=`+q(id)+`
echo `+strconv.Itoa(port)+` > "$R/port.new" && mv "$R/port.new" "$R/port"
echo "$id" > "$R/current.new" && mv "$R/current.new" "$R/current"
`+enable)
	return port, err
}

// caddy makes Caddy route the domain to 127.0.0.1:port: it installs Caddy on Debian or Ubuntu
// when absent (refusing other systems with a sentence), then sets the route through the admin
// API (localhost:2019), the route's @id naming the app and environment.
func (s *Ssh) caddy(ctx context.Context, port int) error {
	if err := s.ensureCaddy(ctx); err != nil {
		return err
	}
	_, err := s.run(ctx, caddyRoute(s.admin(), s.unit(), s.Domain, port))
	return err
}

// ensureCaddy installs Caddy when absent (Debian and Ubuntu; other systems refused with a
// sentence) and waits for its admin API.
func (s *Ssh) ensureCaddy(ctx context.Context) error {
	if s.caddyDone {
		return nil
	}
	out, err := s.run(ctx, caddyEnsure(s.admin()))
	if err != nil {
		return err
	}
	kv := keyValues(out)
	switch {
	case kv["unsupported"] != "":
		return refuse("", "Caddy is not installed on %s, and alder installs it on Debian and Ubuntu only (this is %s): install Caddy (https://caddyserver.com/docs/install) and deploy again, or write `proxy: None` and put your own proxy in front", s.Dest, kv["unsupported"])
	case strings.Contains(out, "aptfailed"):
		return refuse("", "installing Caddy with apt on %s failed: install it by hand (https://caddyserver.com/docs/install; apt needs root or passwordless sudo), or write `proxy: None`", s.Dest)
	case strings.Contains(out, "noadmin"):
		return refuse("", "Caddy on %s does not answer on its admin API (%s): start it (`systemctl start caddy`) and deploy again", s.Dest, s.admin())
	}
	s.caddyDone = true
	return nil
}

// Preflight is Preflighter: before anything is uploaded, the systemd unit is installed (and
// lingering checked) and Caddy installed and answering, so a box that cannot run the build
// refuses with nothing changed in builds/.
func (s *Ssh) Preflight(ctx context.Context) error {
	if s.sup != "process" && !s.unitDone {
		if err := s.installUnit(ctx); err != nil {
			return err
		}
	}
	if s.Proxy != "None" {
		return s.ensureCaddy(ctx)
	}
	return nil
}

// admin is the admin API's host:port.
func (s *Ssh) admin() string {
	if s.CaddyAdmin == "" {
		return DefaultCaddyAdmin
	}
	return s.CaddyAdmin
}

// caddyEnsure is the script that installs Caddy when absent (Debian and Ubuntu) and waits for
// its admin API at admin. A Caddy whose admin API already answers is used as it is.
//
// Three things about a Caddy alder did not install (chapter 16, "Caddy"): the routes alder
// sets through the admin API live in Caddy's running configuration, so a Caddy restarted from
// its Caddyfile without --resume (the drop-in below is written only when alder installs
// Caddy) forgets them until the next deploy; a server of its own on :443 conflicts with the
// server "alder" at the first switch; and the admin API answers every local user of the box.
func caddyEnsure(admin string) string {
	return `A=` + q("http://"+admin) + `
if curl -s -o /dev/null "$A/config/"; then echo ready=yes; exit 0; fi
SUDO=""; [ "$(id -u)" = 0 ] || SUDO="sudo -n"
if ! command -v caddy >/dev/null 2>&1; then
  ID=""; ID_LIKE=""; [ -r /etc/os-release ] && . /etc/os-release
  case " $ID $ID_LIKE " in
    *" debian "*|*" ubuntu "*) ;;
    *) echo "unsupported=${ID:-an unknown system}"; exit 0;;
  esac
  ( umask 022; $SUDO env DEBIAN_FRONTEND=noninteractive apt-get update -q && $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -q caddy ) >&2 || { echo aptfailed; exit 0; }
  if [ -d /run/systemd/system ]; then
    # keep the routes alder deploy sets through the admin API when Caddy restarts
    $SUDO mkdir -p /etc/systemd/system/caddy.service.d
    printf '[Service]\nExecStart=\nExecStart=/usr/bin/caddy run --environ --resume --config /etc/caddy/Caddyfile\n' | ( umask 022; $SUDO tee /etc/systemd/system/caddy.service.d/alder.conf >/dev/null )
    $SUDO systemctl daemon-reload && $SUDO systemctl restart caddy
  fi
  echo installed=yes
fi
i=0
until curl -s -o /dev/null "$A/config/"; do
  i=$((i + 1)); [ $i -gt 20 ] && { echo noadmin; exit 0; }; sleep 0.5
done
echo ready=yes
`
}

// caddyRoute is the script that sets the route id (host domain → 127.0.0.1:port) in Caddy's
// configuration: replaced when it exists, else added to the server "alder" (listening on :443,
// so Caddy gets the domain's certificate), creating what is missing above it.
func caddyRoute(admin, id, domain string, port int) string {
	route := CaddyRoute(id, domain, port)
	server := `{"listen":[":443"],"routes":[` + route + `]}`
	return `A=` + q("http://"+admin) + `
ID=` + q(id) + `
ROUTE=` + q(route) + `
SERVER=` + q(server) + `
put() { curl -sf -X "$1" -H 'Content-Type: application/json' -d "$2" "$A$3" >/dev/null; }
if curl -sf "$A/id/$ID" >/dev/null 2>&1; then put PATCH "$ROUTE" "/id/$ID"; exit; fi
if [ "$(curl -s "$A/config/apps/http/servers/alder")" != null ]; then put POST "$ROUTE" /config/apps/http/servers/alder/routes; exit; fi
if [ "$(curl -s "$A/config/apps/http")" != null ]; then put PUT "$SERVER" /config/apps/http/servers/alder; exit; fi
if [ "$(curl -s "$A/config/apps")" != null ]; then put PUT "{\"servers\":{\"alder\":$SERVER}}" /config/apps/http; exit; fi
put POST "{\"apps\":{\"http\":{\"servers\":{\"alder\":$SERVER}}}}" /load
`
}

// CaddyRoute is the JSON of the route @id id: requests for domain go to 127.0.0.1:port.
func CaddyRoute(id, domain string, port int) string {
	r := map[string]any{
		"@id":      id,
		"match":    []any{map[string]any{"host": []string{domain}}},
		"handle":   []any{map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": "127.0.0.1:" + strconv.Itoa(port)}}}},
		"terminal": true,
	}
	b, _ := json.Marshal(r)
	return string(b)
}

// Lock is Locker: one deploy, rollback or restore at a time per environment. A long-lived ssh
// session holds the lock (flock on <path>/deploy.lock where flock exists, else the directory
// <path>/deploy.lock.d with who/when/pid, taken over when that pid no longer runs) until the
// returned unlock closes it; a session that dies releases it. A held lock is a refusal naming
// its holder.
func (s *Ssh) Lock(ctx context.Context) (func(), error) {
	who := "local " + os.Getenv("USER")
	script := `umask 077
R=` + q(s.Root) + `
L="$R/deploy.lock"
mkdir -p "$R" || exit 1
info="held by $(id -un)@$(hostname 2>/dev/null || uname -n) (` + strings.ReplaceAll(who, "'", "") + `) since $(date -u +%Y-%m-%dT%H:%M:%SZ), pid $$"
if command -v flock >/dev/null 2>&1; then
  exec 9>>"$L"
  flock -n 9 || { echo "busy $(cat "$L.info" 2>/dev/null)"; exit 0; }
  echo "$info" > "$L.info"
  echo locked
  cat > /dev/null
  rm -f "$L.info"
  exit 0
fi
D="$L.d"
if ! mkdir "$D" 2>/dev/null; then
  op=$(sed -n 's/.*, pid \([0-9][0-9]*\)$/\1/p' "$D/info" 2>/dev/null)
  if [ -n "$op" ] && kill -0 "$op" 2>/dev/null; then echo "busy $(cat "$D/info")"; exit 0; fi
  rm -rf "$D"
  mkdir "$D" 2>/dev/null || { echo "busy (another run is taking the lock)"; exit 0; }
fi
echo "$info" > "$D/info"
echo locked
cat > /dev/null
rm -rf "$D"
`
	cmd := exec.Command("ssh", append(s.sshArgs(), "sh -c "+q(script))...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("running ssh: %w", err)
	}
	line := make(chan string, 1)
	go func() {
		b := make([]byte, 0, 256)
		one := make([]byte, 1)
		for {
			n, err := outPipe.Read(one)
			if n == 1 && one[0] != '\n' {
				b = append(b, one[0])
				continue
			}
			if n == 1 || err != nil {
				line <- string(b)
				io.Copy(io.Discard, outPipe)
				return
			}
		}
	}()
	var first string
	select {
	case first = <-line:
	case <-ctx.Done():
		cmd.Process.Kill()
		cmd.Wait()
		return nil, ctx.Err()
	case <-time.After(60 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("taking the deploy lock at %s timed out", s.Dest)
	}
	if first != "locked" {
		in.Close()
		werr := cmd.Wait()
		if holder, ok := strings.CutPrefix(first, "busy "); ok || strings.HasPrefix(first, "busy") {
			return nil, refuse("", "another deploy, rollback or restore of %s is running (the lock at %s/deploy.lock is %s): wait for it to finish", s.Env, s.Root, strings.TrimSpace(holder))
		}
		code := -1
		if ee, ok := werr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return nil, &RemoteError{Dest: s.Dest, Code: code, Stderr: errb.String()}
	}
	return func() {
		in.Close()
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	}, nil
}

// RestoreBackup is BackupRestorer: builds/<src>/backup.db, the copy of the database taken
// while build to served (before src's migrations), goes in place. Every running build is
// stopped first (stopAll: a build draining after a switch would otherwise keep the replaced
// file open and go on serving it); db/app.db (with -wal, -shm), db/schema.json and
// db/migrations are kept as *.before-rollback-<time>; the copy goes in place (mode 600) with
// to's schema.json and migration names. Undo stops every build and puts the kept files back.
func (s *Ssh) RestoreBackup(ctx context.Context, to, src string) (Restored, error) {
	if err := s.stopAll(ctx); err != nil {
		return Restored{}, err
	}
	out, err := s.run(ctx, `set -e
to=`+q(to)+`; src=`+q(src)+`
[ -f "$R/builds/$src/backup.db" ] || { echo "builds/$src/backup.db is missing" >&2; exit 1; }
cd "$R/db"
ts=$(date -u +%Y%m%dT%H%M%SZ)
for x in "" -wal -shm; do if [ -f "app.db$x" ]; then mv "app.db$x" "app.db.before-rollback-$ts$x"; fi; done
for f in schema.json migrations; do if [ -f "$f" ]; then cp "$f" "$f.before-rollback-$ts"; fi; done
cp "$R/builds/$src/backup.db" app.db.new && chmod 600 app.db.new && mv app.db.new app.db
if [ -f "$R/builds/$to/migrations/schema.json" ]; then
  cp "$R/builds/$to/migrations/schema.json" schema.json
  ls "$R/builds/$to/migrations" | sed -n 's/\.sql$//p' > migrations
else rm -f schema.json migrations; fi
echo "ts=$ts"
echo "taken=$(date -r "$R/builds/$src/backup.db" +%s 2>/dev/null || echo 0)"
`)
	if err != nil {
		return Restored{}, err
	}
	kv := keyValues(out)
	ts := kv["ts"]
	secs, _ := strconv.ParseInt(kv["taken"], 10, 64)
	undo := func(ctx context.Context) error {
		if err := s.stopAll(ctx); err != nil {
			return err
		}
		_, err := s.run(ctx, `set -e
ts=`+q(ts)+`
cd "$R/db"
rm -f app.db app.db-wal app.db-shm
for x in "" -wal -shm; do if [ -f "app.db.before-rollback-$ts$x" ]; then mv "app.db.before-rollback-$ts$x" "app.db$x"; fi; done
for f in schema.json migrations; do if [ -f "$f.before-rollback-$ts" ]; then mv "$f.before-rollback-$ts" "$f"; fi; done
`)
		return err
	}
	return Restored{From: "builds/" + src + "/backup.db", Taken: time.Unix(secs, 0), Kept: "db/app.db.before-rollback-" + ts, Undo: undo}, nil
}

// stopAll stops every build that runs, now, and cancels every delayed stop: before the
// database file is replaced, no process may hold the old one open.
func (s *Ssh) stopAll(ctx context.Context) error {
	_, err := s.run(ctx, `for d in "$R"/builds/*/; do
  [ -d "$d" ] || continue
  id=$(basename "$d")
  cancel_later "$id"
  if alive "$id"; then stopnow "$id"; fi
done
exit 0
`)
	return err
}

// ErrNotRunning is Stop's answer for a build that was not running (nothing was scheduled).
var ErrNotRunning = errors.New("not running")

// Stop stops build id: after drain in the background (builds/<id>/stop-later, its pid in
// stop-later.pid; the command does not wait), or now. The delayed stop does nothing when the
// build is current again by then (a rollback inside the drain window), and Start cancels it.
// A build that is not running is ErrNotRunning (with `proxy: None`, Switch stopped it already).
func (s *Ssh) Stop(ctx context.Context, id string, drain time.Duration) error {
	if drain <= 0 {
		return s.stop(ctx, id)
	}
	later := "#!/bin/sh\n" + s.prelude() + "id=" + q(id) + "\nsleep " + strconv.Itoa(int(drain/time.Second)) + `
[ "$(cat "$R/current" 2>/dev/null)" = "$id" ] && { rm -f "$R/builds/$id/stop-later.pid"; exit 0; }
stopnow "$id"
rm -f "$R/builds/$id/stop-later.pid"
`
	out, err := s.run(ctx, `id=`+q(id)+`
B="$R/builds/$id"
alive "$id" || { echo notrunning; exit 0; }
cancel_later "$id"
cat > "$B/stop-later" <<'ALDER_STOP_EOF'
`+later+`ALDER_STOP_EOF
chmod 700 "$B/stop-later"
(trap '' HUP; exec "$B/stop-later") >/dev/null 2>&1 < /dev/null &
echo $! > "$B/stop-later.pid"
`)
	if err == nil && strings.TrimSpace(out) == "notrunning" {
		return ErrNotRunning
	}
	return err
}

// stop stops build id now (and cancels a pending stop-later of it).
func (s *Ssh) stop(ctx context.Context, id string) error {
	_, err := s.run(ctx, `id=`+q(id)+`
cancel_later "$id"
stopnow "$id"
`)
	return err
}

// Record writes builds/<id>/deployed.json.
func (s *Ssh) Record(ctx context.Context, id string, rec []byte) error {
	_, err := s.run(ctx, `cat > "$R/builds/"`+q(id)+`/deployed.json <<'ALDER_RECORD_EOF'
`+string(rec)+`ALDER_RECORD_EOF
`)
	return err
}

// Prune removes builds beyond keep: failed ones (never switched to) first, oldest first, then
// deployed ones, oldest first. It never removes one in protect, one running, the current one,
// the newest deployed one that is not current (the rollback target), or one holding the newest
// pre-migration copy (backup.db, backup.of naming the build it was taken under) of a kept build,
// which `rollback --restore-backup` to that build needs.
func (s *Ssh) Prune(ctx context.Context, keep int, protect []string) ([]string, error) {
	builds, err := s.Builds(ctx)
	if err != nil {
		return nil, err
	}
	return s.removeBuilds(ctx, PrunePlan(builds, keep, protect))
}

// PrunePlan is the ids Prune removes from builds (newest first).
func PrunePlan(builds []Build, keep int, protect []string) []string {
	excess := len(builds) - keep
	if excess <= 0 {
		return nil
	}
	fixed := map[string]bool{}
	for _, id := range protect {
		fixed[id] = true
	}
	lastGood := false
	for _, b := range builds {
		if b.Running || b.Current {
			fixed[b.ID] = true
		}
		if !lastGood && b.Record != nil && !b.Current {
			fixed[b.ID], lastGood = true, true
		}
	}
	var order []Build // the candidates: failed oldest first, then deployed oldest first
	for _, failed := range []bool{true, false} {
		for i := len(builds) - 1; i >= 0; i-- {
			if b := builds[i]; !fixed[b.ID] && (b.Record == nil) == failed {
				order = append(order, b)
			}
		}
	}
	gone := map[string]bool{}
	for _, b := range order {
		if len(gone) == excess {
			break
		}
		gone[b.ID] = true
	}
	// the newest copy taken under a kept build (backup.db, backup.of naming it) is the way back
	// to that build's data: its directory stays
	for _, k := range builds {
		if gone[k.ID] {
			continue
		}
		for _, b := range builds { // newest first: the first that names k holds k's newest copy
			if b.BackupOf == k.ID {
				delete(gone, b.ID)
				break
			}
		}
	}
	var ids []string
	for _, b := range order {
		if gone[b.ID] {
			ids = append(ids, b.ID)
		}
	}
	return ids
}

func (s *Ssh) removeBuilds(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var sc strings.Builder
	for _, id := range ids {
		sc.WriteString(`cancel_later ` + q(id) + `; rm -rf "$R/builds/"` + q(id) + "\n")
	}
	_, err := s.run(ctx, sc.String())
	return ids, err
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// streamEnv prints streamenv=yes when the environment file sets ALDER_BACKUP_URL (only grep's
// answer crosses the connection, never the value).
const streamEnv = `grep -Eq '^(export[[:space:]]+)?ALDER_BACKUP_URL=[^[:space:]]' "$R/env" 2>/dev/null && echo streamenv=yes`

// StreamsBackup is StreamChecker: whether the environment file sets ALDER_BACKUP_URL.
func (s *Ssh) StreamsBackup(ctx context.Context) (bool, error) {
	out, err := s.run(ctx, streamEnv+"\nexit 0\n")
	return strings.Contains(out, "streamenv=yes"), err
}

// Info reads the database's size, the shipper's last upload (db/app.db-backup.json, which the
// runtime writes) and the newest pre-migration copy.
func (s *Ssh) Info(ctx context.Context) (Info, error) {
	out, err := s.run(ctx, `if [ -f "$R/db/app.db" ]; then
  n=$(wc -c < "$R/db/app.db" | tr -d ' '); w=0; [ -f "$R/db/app.db-wal" ] && w=$(wc -c < "$R/db/app.db-wal" | tr -d ' ')
  echo "db=$((n + w))"
fi
[ -f "$R/db/app.db-backup.json" ] && echo "shipped=$(tr -d '\n' < "$R/db/app.db-backup.json")"
for f in "$R"/builds/*/backup.db; do [ -f "$f" ] && echo "copy=$(date -r "$f" +%s) builds/$(basename "$(dirname "$f")")/backup.db"; done
`+streamEnv+`
exit 0
`)
	if err != nil {
		return Info{}, err
	}
	in := Info{StreamTo: s.BackupURL}
	if s.BackupURL == "" && strings.Contains(out, "streamenv=yes") {
		in.StreamTo = "the environment file's ALDER_BACKUP_URL"
	}
	for _, l := range strings.Split(out, "\n") {
		k, v, _ := strings.Cut(l, "=")
		switch k {
		case "db":
			in.HaveDB = true
			in.DBBytes, _ = strconv.ParseInt(v, 10, 64)
		case "shipped":
			var st struct {
				Last time.Time `json:"last"`
				URL  string    `json:"url"`
			}
			if json.Unmarshal([]byte(v), &st) == nil {
				in.Shipped = st.Last
				if st.Last.After(in.LastBackup) {
					in.LastBackup, in.BackupFrom = st.Last, "streamed to "+st.URL
				}
			}
		case "copy":
			secs, path, _ := strings.Cut(v, " ")
			n, _ := strconv.ParseInt(secs, 10, 64)
			if t := time.Unix(n, 0); t.After(in.LastBackup) {
				in.LastBackup, in.BackupFrom = t, path
			}
		}
	}
	return in, nil
}

// Logs writes the current build's last 100 log lines; with follow it goes on writing what is
// appended, and when another build becomes current (a deploy, a rollback) that build's log.
func (s *Ssh) Logs(ctx context.Context, follow bool, w io.Writer) error {
	if _, err := s.Connect(ctx); err != nil {
		return err
	}
	script := `c=$(cat "$R/current" 2>/dev/null) || { echo "not deployed yet" >&2; exit 1; }
exec tail -n 100 "$R/builds/$c/log"
`
	if follow { // across switches: when current changes, the new build's log from its start
		script = `c=$(cat "$R/current" 2>/dev/null) || { echo "not deployed yet" >&2; exit 1; }
tail -n 100 "$R/builds/$c/log"
off=$(wc -c < "$R/builds/$c/log" | tr -d ' ')
while kill -0 "$PPID" 2>/dev/null; do  # the session's sshd gone: stop
  n=$(cat "$R/current" 2>/dev/null)
  if [ -n "$n" ] && [ "$n" != "$c" ]; then c=$n; off=0; echo "--- alder logs: build $c is current"; fi
  size=$(wc -c < "$R/builds/$c/log" 2>/dev/null | tr -d ' ')
  if [ -n "$size" ] && [ "$size" -gt "$off" ]; then tail -c +$((off + 1)) "$R/builds/$c/log" | head -c $((size - off)); off=$size; fi
  sleep 1
done
`
	}
	// its own connection, not the run's shared one: when it closes, the session's sshd exits
	// and the loop above, which watches it, stops
	ctl := s.ctl
	s.ctl = ""
	defer func() { s.ctl = ctl }()
	return s.stream(ctx, script, w, io.Discard)
}

// Restore uploads the local database file db as db/app.db (mode 600): every running build is
// stopped (stopAll), the old database files are kept beside it (app.db.before-restore-<time>,
// with its -wal and -shm, and the schema records), and the current build is started again on
// the restored file, health checked, and routed (Switch: Caddy's upstream, or with `proxy:
// None` the public port). A build that refuses the restored file is stopped, the file is kept
// as app.db.refused-<time>, and the replaced database and the build come back.
func (s *Ssh) Restore(ctx context.Context, db string) (string, error) {
	if _, err := s.Connect(ctx); err != nil {
		return "", err
	}
	f, err := os.Open(db)
	if err != nil {
		return "", err
	}
	defer f.Close()
	dst := s.Root + "/db/app.db.restore"
	cmd := exec.CommandContext(ctx, "ssh", append(s.sshArgs(), "umask 077 && mkdir -p "+q(s.Root+"/db")+" && cat > "+q(dst+".part")+" && chmod 600 "+q(dst+".part")+" && mv "+q(dst+".part")+" "+q(dst))...)
	cmd.Stdin = f
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", &RemoteError{Dest: s.Dest, Code: -1, Stderr: errb.String()}
	}
	cur, err := s.Current(ctx)
	if err != nil {
		return "", err
	}
	if err := s.stopAll(ctx); err != nil {
		return "", err
	}
	out, err := s.run(ctx, `set -e
cd "$R/db"
ts=$(date -u +%Y%m%dT%H%M%SZ)
for x in "" -wal -shm; do if [ -f "app.db$x" ]; then mv "app.db$x" "app.db.before-restore-$ts$x"; fi; done
for f in schema.json migrations; do if [ -f "$f" ]; then mv "$f" "$f.before-restore-$ts"; fi; done
chmod 600 app.db.restore
mv app.db.restore app.db
echo "ts=$ts"
`)
	if err != nil {
		return "", err
	}
	ts := keyValues(out)["ts"]
	kept := "db/app.db.before-restore-" + ts
	if cur == "" {
		return kept, nil
	}
	serve := func() error {
		port, err := s.Start(ctx, cur)
		if err == nil {
			err = s.Health(ctx, cur, port, DefaultHealthTimeout)
		}
		if err == nil {
			_, err = s.Switch(ctx, cur, port)
		}
		return err
	}
	if err := serve(); err != nil {
		s.stopAll(ctx)
		_, uerr := s.run(ctx, `set -e
ts=`+q(ts)+`
cd "$R/db"
for x in "" -wal -shm; do if [ -f "app.db$x" ]; then mv "app.db$x" "app.db.refused-$ts$x"; fi; done
for x in "" -wal -shm; do if [ -f "app.db.before-restore-$ts$x" ]; then mv "app.db.before-restore-$ts$x" "app.db$x"; fi; done
for f in schema.json migrations; do if [ -f "$f.before-restore-$ts" ]; then mv "$f.before-restore-$ts" "$f"; fi; done
`)
		if uerr != nil {
			return kept, refuse(err.Error(), "build %s did not pass its health check on the restored database, and putting the replaced one back failed (%v): the replaced database is %s; put it back as db/app.db by hand and run `alder deploy`", cur, uerr, kept)
		}
		if serr := serve(); serr != nil {
			return kept, refuse(err.Error(), "build %s did not pass its health check on the restored database: the restored file is kept as db/app.db.refused-%s and the database it replaced (%s) is back in place as db/app.db, but build %s did not start again on it (%v): run `alder deploy`", cur, ts, kept, cur, serr)
		}
		return kept, refuse(err.Error(), "build %s did not pass its health check on the restored database: the restored file is kept as db/app.db.refused-%s, and the database it replaced (%s) is back in place as db/app.db; build %s serves again", cur, ts, kept, cur)
	}
	return kept, nil
}
