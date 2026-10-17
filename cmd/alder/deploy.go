package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"alder/internal/buildkit"
	"alder/internal/deploy"
	"alder/internal/diag"
	"alder/internal/ir"
	"alder/internal/version"
	"alder/rt/backup"
)

// The usage lines of the deploy commands (chapter 16).
const (
	deployUsage   = "usage: alder deploy <env> [dir] [--host name] [--allow-data-loss] [--unsafe-skip-tests] [--dry-run] [--json]"
	statusUsage   = "usage: alder status <env> [dir] [--host name]"
	rollbackUsage = "usage: alder rollback <env> [<build id>] [--dir dir] [--host name] [--allow-schema-mismatch | --restore-backup]"
	logsUsage     = "usage: alder logs <env> [dir] [--host name] [-f]"
	restoreUsage  = "usage: alder restore <env> [--from url] [--to time] [--out file.db] [--dir dir]"
)

// Two knobs for tests and impatient operators, read from the environment: how long the previous
// build drains after the switch (default 30s; 0 stops it at once) and how long a new build has to
// pass its health check (default 60s).
const (
	envDrain         = "ALDER_DEPLOY_DRAIN"
	envHealthTimeout = "ALDER_DEPLOY_HEALTH_TIMEOUT"
)

// runDeployTo is `alder deploy <env> [dir] [--allow-data-loss] [--unsafe-skip-tests] [--dry-run]
// [--json]` (chapter 16): it reads App's `deploy` declaration in dir (default "."), runs the
// gate (`alder check`: errors and holes refuse; `alder build`; `alder test`; `alder test
// --mutate` against the floor; each declared check), writes the artifact
// build/<app>/deploy/<build id>/ (the executable for the target's platform, static/,
// migrations/, alder-deploy.json), and ships it to the environment's target (internal/deploy:
// Deploy). Progress goes to stdout, or with --json to stderr while stdout gets the record (or
// {"error": …}). --dry-run stops after the plan and changes nothing at the target.
//
// Exit 0 when deployed (or the dry run passed); 1 for a refusal (the gate, a missing `requires`
// name, a migration that drops data without --allow-data-loss, a build that fails its health
// check), a target that cannot be reached, a script that exits non-zero; 2 for usage errors, a
// missing declaration or environment, and internal errors.
func runDeployTo(args []string, stdout, stderr io.Writer) (code int) {
	jsonOut := false
	fail := func(code int, msg string) int {
		fmt.Fprintf(stderr, "alder deploy: %s\n", msg)
		if jsonOut {
			writeJSON(stdout, map[string]string{"error": msg})
		}
		return code
	}
	defer func() {
		if r := recover(); r != nil {
			code = fail(2, fmt.Sprintf("internal error: %v", r))
		}
	}()
	var pos []string
	o := deploy.Options{}
	host := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--host" && i+1 < len(args):
			i++
			host = args[i]
		case strings.HasPrefix(a, "--host="):
			host = strings.TrimPrefix(a, "--host=")
		case a == "--allow-data-loss":
			o.AllowDataLoss = true
		case a == "--unsafe-skip-tests":
			o.SkipTests = true
		case a == "--dry-run":
			o.DryRun = true
		case a == "--json":
			jsonOut = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, deployUsage)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 1 || len(pos) > 2 {
		fmt.Fprintln(stderr, deployUsage)
		return 2
	}
	env, dir := pos[0], "."
	if len(pos) == 2 {
		dir = pos[1]
	}
	progress := stdout
	if jsonOut {
		progress = stderr
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := &deployRunner{dir: dir}
	errs, _, _, _ := r.Check()
	if r.loadErr != "" {
		return fail(2, r.loadErr)
	}
	if errs > 0 || r.decl == nil {
		// the declaration may be what failed: its E0030 is among the diagnostics
		if errs == 0 {
			return fail(2, "App has no `deploy` declaration: declare where the app ships (chapter 16), such as\n  deploy { production: Ssh \"deploy@box.example.com\" { domain: \"example.com\", path: \"/srv/app\" } }")
		}
	}
	var tgt ir.Target
	if r.decl != nil {
		var ok bool
		if tgt, ok = r.decl.Envs[env]; !ok {
			return fail(2, fmt.Sprintf("App's `deploy` has no environment `%s` (it declares %s)", env, strings.Join(envNames(r.decl), ", ")))
		}
	}
	if host != "" && r.decl != nil {
		var err error
		if tgt, err = withHost(env, tgt, host); err != nil {
			return fail(2, err.Error())
		}
	}
	if r.decl != nil {
		fmt.Fprintf(progress, "alder deploy %s → %s\n", env, describeTarget(tgt))
		if host != "" {
			fmt.Fprintf(progress, "host %s (--host)\n", host)
		}
	} else {
		fmt.Fprintf(progress, "alder deploy %s\n", env)
	}
	fmt.Fprintln(progress, "gate")
	g, err := deploy.GateCheck(r, progress)
	if err != nil {
		return deployFailed(err, fail)
	}
	t, script, err := targetFor(r.app, targetEnv(env, host), tgt, r.decl, dir, progress, stderr)
	if err != nil {
		return fail(2, err.Error())
	}
	if script != nil && host != "" {
		// the script reads the host the run is for from ALDER_HOST
		prev, had := os.LookupEnv("ALDER_HOST")
		os.Setenv("ALDER_HOST", host)
		defer func() {
			if had {
				os.Setenv("ALDER_HOST", prev)
			} else {
				os.Unsetenv("ALDER_HOST")
			}
		}()
	}
	if t != nil {
		defer t.Close()
	}
	o.App, o.Env, o.AppDir, o.Decl, o.Runner = r.app, env, dir, r.decl, r
	o.Built = filepath.Join("build", r.app)
	o.ArtifactRoot = filepath.Join(o.Built, "deploy")
	o.Binary = func(p deploy.Platform, path string) error { return buildFor(o.Built, r.app, p, path) }
	o.Version, o.Commit = version.String(), gitCommit(dir)
	o.User = os.Getenv("USER")
	o.Out = progress
	o.Drain = durationEnv(envDrain, 0)
	if v := os.Getenv(envDrain); v != "" && o.Drain == 0 {
		o.Drain = -1 // explicitly 0: stop at once
	}
	o.HealthTimeout = durationEnv(envHealthTimeout, 0)
	res, err := deploy.Deploy(ctx, o, g, t, script)
	if err != nil {
		return deployFailed(err, fail)
	}
	if jsonOut {
		writeJSONIndented(stdout, res.Record)
	}
	return 0
}

// deployFailed reports a deploy's error: a refusal's text as it is, anything else (the target
// could not be reached, a remote command failed) as an error; both exit 1.
func deployFailed(err error, fail func(int, string) int) int {
	return fail(1, err.Error())
}

// writeJSONIndented writes v as indented JSON (the deploy record, as alder-deploy.json holds it).
func writeJSONIndented(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func envNames(d *ir.Deploy) []string {
	var names []string
	for n := range d.Envs {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// describeTarget is the target as the declaration writes it.
func describeTarget(t ir.Target) string {
	switch t.Kind {
	case "ssh":
		return fmt.Sprintf("Ssh %s:%s", t.Host, t.Path)
	case "fly":
		return fmt.Sprintf("Fly %s (app %s)", t.Region, t.App)
	case "script":
		return "Script " + t.ScriptPath
	}
	return t.Kind
}

// targetFor is environment env's target: a deploy.Target for Ssh and Fly, or the Script.
func targetFor(app, env string, t ir.Target, d *ir.Deploy, dir string, stdout, stderr io.Writer) (deploy.Target, *deploy.Script, error) {
	switch t.Kind {
	case "script":
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, nil, err
		}
		return nil, &deploy.Script{Path: t.ScriptPath, AppDir: abs, Stdout: stdout, Stderr: stderr}, nil
	case "ssh":
		s := deploy.NewSsh(app, env, t, d)
		s.Warn = stdout
		return s, nil, nil
	case "fly":
		f := deploy.NewFly(app, env, t, d)
		f.Warn = stdout
		return f, nil, nil
	}
	return nil, nil, fmt.Errorf("the %s target is not available in this alder yet", t.Kind)
}

// withHost is environment env's target t serving host instead of its declared domain (`alder
// deploy preview --host 42.preview.example.com`, which `alder ci init`'s workflow runs for a
// pull request). A host of a wildcard domain must match it. An Ssh target keeps each host's
// state apart: under <path>/<host>, as environment <env>-<host> (its own systemd unit and Caddy
// route), so one pull request's deploy never replaces another's. A Script target gets the host
// in ALDER_HOST. A Fly environment's host is its app's (<app>.fly.dev): --host is refused.
func withHost(env string, t ir.Target, host string) (ir.Target, error) {
	if host == "" || strings.ContainsAny(host, "/ *:") || strings.HasPrefix(host, ".") {
		return t, fmt.Errorf("bad --host %q: give a host name such as 42.preview.example.com", host)
	}
	switch t.Kind {
	case "fly":
		return t, fmt.Errorf("--host does not apply to environment %s: a Fly environment is served at its app's address (%s.fly.dev)", env, t.App)
	case "ssh":
		if suffix, ok := strings.CutPrefix(t.Domain, "*"); ok {
			label, rest, _ := strings.Cut(host, ".")
			if label == "" || "."+rest != suffix {
				return t, fmt.Errorf("--host %s does not match environment %s's domain %s", host, env, t.Domain)
			}
		}
		t.Domain = host
		t.Path = strings.TrimRight(t.Path, "/") + "/" + host
	}
	return t, nil
}

// hostFlag reads `--host name` or `--host=name` at args[*i], advancing *i past its value.
func hostFlag(args []string, i *int) (string, bool) {
	a := args[*i]
	if a == "--host" && *i+1 < len(args) {
		*i++
		return args[*i], true
	}
	return strings.CutPrefix(a, "--host=")
}

// targetEnv is the environment's name at the target: with --host, <env>-<host> (withHost).
func targetEnv(env, host string) string {
	if host == "" {
		return env
	}
	return env + "-" + host
}

// deployRunner runs the gate's steps with the code of `alder check`, `alder build` and `alder
// test`, capturing what each prints.
type deployRunner struct {
	dir     string
	app     string
	decl    *ir.Deploy
	loadErr string
	checked bool
	errs    int
	warns   int
	holes   int
	out     string
}

func (r *deployRunner) Check() (int, int, int, string) {
	if r.checked {
		return r.errs, r.warns, r.holes, r.out
	}
	r.checked = true
	abs, err := filepath.Abs(r.dir)
	if err != nil {
		r.loadErr = err.Error()
		return 0, 0, 0, ""
	}
	r.app = filepath.Base(abs)
	if !dirHasApp(r.dir) {
		r.loadErr = fmt.Sprintf("no .alder files under %s/app", r.dir)
		return 0, 0, 0, ""
	}
	prog, res, bag, ioErrs := checkedApp(r.dir)
	var buf bytes.Buffer
	if reportLoadErrs("alder check", ioErrs, &buf) {
		r.loadErr = strings.TrimSpace(buf.String())
		return 0, 0, 0, ""
	}
	sources := buildkit.Sources(prog)
	for _, d := range bag.Items {
		buf.WriteString(diag.RenderText(d, sources[d.Span.File]))
		switch {
		case d.Severity == "error":
			r.errs++
		case d.Code != "W0002":
			r.warns++
		}
	}
	r.holes = countHoles(bag.Items)
	r.out = buf.String()
	if res != nil {
		if p, err := ir.LowerTables(prog, res); err == nil {
			r.decl = p.Deploy
		}
	}
	return r.errs, r.warns, r.holes, r.out
}

func (r *deployRunner) Build() (string, bool) {
	var buf bytes.Buffer
	_, code := buildTo(r.dir, false, "", &buf, &buf)
	return buf.String(), code == 0
}

func (r *deployRunner) Test() (deploy.TestResult, string, bool) {
	var out, errb bytes.Buffer
	code := runTestTo([]string{r.dir}, &out, &errb)
	tr := deploy.TestResult{}
	for _, l := range strings.Split(out.String(), "\n") {
		var p, f int
		if n, _ := fmt.Sscanf(l, "%d passed, %d failed", &p, &f); n == 2 {
			tr = deploy.TestResult{Passed: p, Failed: f}
		}
	}
	return tr, out.String() + errb.String(), code == 0
}

func (r *deployRunner) Mutate() (float64, int, int, string, bool) {
	var out, errb bytes.Buffer
	code := runTestTo([]string{r.dir, "--mutate", "--json"}, &out, &errb)
	var score float64
	mutants, survived, found := 0, 0, false
	sc := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		var s struct {
			Score    *float64 `json:"score"`
			Killed   int      `json:"killed"`
			Survived int      `json:"survived"`
		}
		if json.Unmarshal(sc.Bytes(), &s) == nil && s.Score != nil {
			score, mutants, survived, found = *s.Score, s.Killed+s.Survived, s.Survived, true
		}
	}
	return score, mutants, survived, errb.String(), code == 0 && found
}

// buildFor builds the app's executable from the generated module built (build/<app>, which
// `alder build` just wrote) for platform p at path: a static binary (CGO off: the SQLite driver
// is pure Go), cross-compiled when the target is another OS or architecture.
func buildFor(built, app string, p deploy.Platform, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "alder-go-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	cmd := exec.Command("go", "build", "-mod=mod", "-trimpath", "-o", abs, "./cmd")
	cmd.Dir = built
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOTMPDIR="+tmp, "CGO_ENABLED=0", "GOOS="+p.OS, "GOARCH="+p.Arch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build of %s for %s failed: %v\n%s", app, p, err, out)
	}
	return nil
}

// gitCommit is the app's commit ("<sha>" or "<sha>+dirty"), or "" outside a repository.
func gitCommit(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	sha := strings.TrimSpace(string(out))
	if st, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--", ".").Output(); err == nil && len(bytes.TrimSpace(st)) > 0 {
		sha += "+dirty"
	}
	return sha
}

func durationEnv(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// envTarget loads dir's `deploy` declaration and environment env's target for status,
// rollback, logs and restore: a Script target has no state to read.
func envTarget(cmd, env, dir, host string) (deploy.Target, *ir.Deploy, error) {
	r := &deployRunner{dir: dir}
	r.Check()
	if r.loadErr != "" {
		return nil, nil, errors.New(r.loadErr)
	}
	if r.decl == nil {
		return nil, nil, errors.New("App has no `deploy` declaration (or it does not check: run `alder check`)")
	}
	t, ok := r.decl.Envs[env]
	if !ok {
		return nil, nil, fmt.Errorf("App's `deploy` has no environment `%s` (it declares %s)", env, strings.Join(envNames(r.decl), ", "))
	}
	if t.Kind == "script" {
		return nil, nil, deploy.ScriptUnavailable(cmd, env)
	}
	if host != "" {
		var err error
		if t, err = withHost(env, t, host); err != nil {
			return nil, nil, err
		}
	}
	tg, _, err := targetFor(r.app, targetEnv(env, host), t, r.decl, dir, io.Discard, io.Discard)
	return tg, r.decl, err
}

// runStatusTo is `alder status <env> [dir]`: the environment's current build, its last five
// builds, the database's size and the last backup (deploy.Status). Exit 0, 1 when the target
// cannot be reached, 2 for usage errors and a Script environment.
func runStatusTo(args []string, stdout, stderr io.Writer) int {
	var pos []string
	host := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if h, ok := hostFlag(args, &i); ok {
			host = h
			continue
		}
		if strings.HasPrefix(a, "-") {
			fmt.Fprintln(stderr, statusUsage)
			return 2
		}
		pos = append(pos, a)
	}
	if len(pos) < 1 || len(pos) > 2 {
		fmt.Fprintln(stderr, statusUsage)
		return 2
	}
	dir := "."
	if len(pos) == 2 {
		dir = pos[1]
	}
	t, _, err := envTarget("status", pos[0], dir, host)
	if err != nil {
		fmt.Fprintf(stderr, "alder status: %v\n", err)
		return 2
	}
	defer t.Close()
	if err := deploy.Status(context.Background(), t, stdout); err != nil {
		fmt.Fprintf(stderr, "alder status: %v\n", err)
		return 1
	}
	return 0
}

// runRollbackTo is `alder rollback <env> [<build id>] [--dir dir] [--allow-schema-mismatch]`:
// makes an earlier build current (deploy.Rollback; by default the build that served before).
// Exit 0, 1 for a refusal or an unreachable target, 2 for usage errors and a Script environment.
func runRollbackTo(args []string, stdout, stderr io.Writer) int {
	var pos []string
	dir, o, host := ".", deploy.RollbackOptions{Out: stdout}, ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if h, ok := hostFlag(args, &i); ok {
			host = h
			continue
		}
		switch {
		case a == "--allow-schema-mismatch":
			o.AllowSchemaMismatch = true
		case a == "--restore-backup":
			o.RestoreBackup = true
		case a == "--dir" && i+1 < len(args):
			i++
			dir = args[i]
		case strings.HasPrefix(a, "--dir="):
			dir = strings.TrimPrefix(a, "--dir=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, rollbackUsage)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 1 || len(pos) > 2 {
		fmt.Fprintln(stderr, rollbackUsage)
		return 2
	}
	if len(pos) == 2 {
		o.To = pos[1]
	}
	t, _, err := envTarget("rollback", pos[0], dir, host)
	if err != nil {
		fmt.Fprintf(stderr, "alder rollback: %v\n", err)
		return 2
	}
	defer t.Close()
	o.Drain = durationEnv(envDrain, 0)
	if v := os.Getenv(envDrain); v != "" && o.Drain == 0 {
		o.Drain = -1
	}
	o.HealthTimeout = durationEnv(envHealthTimeout, 0)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if _, err := deploy.Rollback(ctx, t, o); err != nil {
		fmt.Fprintf(stderr, "alder rollback: %v\n", err)
		return 1
	}
	return 0
}

// runLogsTo is `alder logs <env> [dir] [-f]`: the current build's log, the last lines, or with
// -f followed until interrupted.
func runLogsTo(args []string, stdout, stderr io.Writer) int {
	var pos []string
	follow, host := false, ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if h, ok := hostFlag(args, &i); ok {
			host = h
			continue
		}
		switch {
		case a == "-f" || a == "--follow":
			follow = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, logsUsage)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 1 || len(pos) > 2 {
		fmt.Fprintln(stderr, logsUsage)
		return 2
	}
	dir := "."
	if len(pos) == 2 {
		dir = pos[1]
	}
	t, _, err := envTarget("logs", pos[0], dir, host)
	if err != nil {
		fmt.Fprintf(stderr, "alder logs: %v\n", err)
		return 2
	}
	defer t.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := t.Logs(ctx, follow, stdout); err != nil && ctx.Err() == nil {
		fmt.Fprintf(stderr, "alder logs: %v\n", err)
		return 1
	}
	return 0
}

// runRestoreTo is `alder restore <env> [--from url] [--to time] [--out file.db] [--dir dir]`:
// it rebuilds the database environment env's backup location held at a time (rt/backup.Restore:
// the newest snapshot at or before it, then the WAL segments up to it) and puts it in place at
// the target (Target.Restore: the current build stopped, the old database kept beside it as
// app.db.before-restore-<time>, the build started again and health checked), or with --out
// writes it to a local file instead. --from defaults to the declaration's `backup`; --to (UTC:
// 2026-10-17T10:00:00Z, 2026-10-17T10:00 or 2026-10-17 10:00:00) to the latest. The bucket's
// credentials come from this machine's environment (AWS_ACCESS_KEY_ID, …). Exit 0, 1 when the
// backup or the target fails, 2 for usage errors.
func runRestoreTo(args []string, stdout, stderr io.Writer) int {
	var pos []string
	dir, from, toText, out := ".", "", "", ""
	flag := func(i *int, name string) (string, bool) {
		a := args[*i]
		if a == name && *i+1 < len(args) {
			*i++
			return args[*i], true
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v, true
		}
		return "", false
	}
	for i := 0; i < len(args); i++ {
		if v, ok := flag(&i, "--dir"); ok {
			dir = v
		} else if v, ok := flag(&i, "--from"); ok {
			from = v
		} else if v, ok := flag(&i, "--to"); ok {
			toText = v
		} else if v, ok := flag(&i, "--out"); ok {
			out = v
		} else if strings.HasPrefix(args[i], "-") {
			fmt.Fprintln(stderr, restoreUsage)
			return 2
		} else {
			pos = append(pos, args[i])
		}
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, restoreUsage)
		return 2
	}
	fail := func(code int, format string, a ...any) int {
		fmt.Fprintf(stderr, "alder restore: "+format+"\n", a...)
		return code
	}
	var to time.Time
	if toText != "" {
		var err error
		if to, err = parseRestoreTime(toText); err != nil {
			return fail(2, "%v", err)
		}
	}
	r := &deployRunner{dir: dir}
	r.Check()
	if r.loadErr != "" {
		return fail(2, "%s", r.loadErr)
	}
	if r.decl == nil {
		return fail(2, "App has no `deploy` declaration (or it does not check: run `alder check`)")
	}
	tgt, ok := r.decl.Envs[pos[0]]
	if !ok {
		return fail(2, "App's `deploy` has no environment `%s` (it declares %s)", pos[0], strings.Join(envNames(r.decl), ", "))
	}
	if from == "" {
		if r.decl.Backup == nil {
			return fail(2, "the declaration has no `backup` location: pass --from s3://bucket/prefix (or a directory)")
		}
		from = r.decl.Backup.URL
	}
	if out == "" && tgt.Kind == "script" {
		return fail(2, "%v; write the database to a file with --out", deploy.ScriptUnavailable("restore", pos[0]))
	}
	if out != "" {
		if _, err := os.Stat(out); err == nil {
			return fail(2, "%s exists: restore into a new file", out)
		}
	}
	tmpDir, err := os.MkdirTemp("", "alder-restore-")
	if err != nil {
		return fail(2, "%v", err)
	}
	defer os.RemoveAll(tmpDir)
	file := filepath.Join(tmpDir, "app.db")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	res, err := backup.Restore(ctx, from, to, file)
	if err != nil {
		return fail(1, "%v", err)
	}
	fmt.Fprintf(stdout, "restored generation %s: its snapshot and %d WAL segments, as of %s\n", res.Generation, res.Segments, res.At.UTC().Format(time.RFC3339))
	if out != "" {
		b, err := os.ReadFile(file)
		if err == nil {
			err = os.WriteFile(out, b, 0o600)
		}
		if err != nil {
			return fail(1, "%v", err)
		}
		fmt.Fprintf(stdout, "wrote %s\n", out)
		return 0
	}
	t, _, err := targetFor(r.app, pos[0], tgt, r.decl, dir, stdout, stderr)
	if err != nil {
		return fail(2, "%v", err)
	}
	defer t.Close()
	if _, err := t.Connect(ctx); err != nil {
		return fail(1, "%v", err)
	}
	if l, ok := t.(deploy.Locker); ok {
		unlock, err := l.Lock(ctx)
		if err != nil {
			return fail(1, "%v", err)
		}
		defer unlock()
	}
	if err := t.Restore(ctx, file); err != nil {
		return fail(1, "%v", err)
	}
	fmt.Fprintf(stdout, "replaced the database at %s (the previous one is kept beside it as app.db.before-restore-<time>)\n", t.Describe())
	return 0
}

// parseRestoreTime reads --to: RFC 3339, or a UTC time written 2026-10-17T10:00[:05] or
// 2026-10-17 10:00[:05].
func parseRestoreTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad --to %q: write a UTC time such as 2026-10-17T10:00:00Z", s)
}
