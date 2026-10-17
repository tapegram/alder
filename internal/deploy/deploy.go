// Package deploy is `alder deploy` (chapter 16): the gate, the artifact, the migration plan
// against the target's live schema, and the ship to a Target (Ssh, Fly) or a Script, with the
// record each build leaves at its target. State lives at the target; the unit is the build id
// (the SHA-256 prefix of the executable, which every page names in its alder-build meta).
//
// The order is fixed and every refusal happens before anything at the target changes: the
// gate (check, build, test, the mutation score against the floor, the declared checks), the
// artifact, the `requires` names verified at the target, the plan (a data-dropping migration
// needs --allow-data-loss), and only then upload, backup, migrate, start, health check by build
// id, switch, stop the previous build, prune. A build that fails its health check is stopped
// and kept for inspection; nothing was switched.
package deploy

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"alder/internal/ir"
)

// DefaultKeep is how many builds a target keeps when the declaration says nothing.
const DefaultKeep = 5

// DefaultDrain is how long the previous build keeps serving its in-flight requests after the
// switch before it is sent SIGTERM.
const DefaultDrain = 30 * time.Second

// DefaultHealthTimeout bounds the health check of a new build.
const DefaultHealthTimeout = 60 * time.Second

// SecretName is the variable the runtime itself needs outside development mode (the session
// key): every Target checks it with the declaration's `requires`.
const SecretName = "ALDER_SECRET"

// Options configure Deploy.
type Options struct {
	App, Env     string
	AppDir       string // the app's root (the declared checks and a Script run there)
	Built        string // build/<app>: what `alder build` wrote (static/, migrations/)
	ArtifactRoot string // build/<app>/deploy
	Decl         *ir.Deploy
	Runner       Runner
	// Binary builds the app's executable for p at path.
	Binary          func(p Platform, path string) error
	Version, Commit string
	AllowDataLoss   bool
	SkipTests       bool
	DryRun          bool
	Drain           time.Duration
	HealthTimeout   time.Duration
	User            string // the local user, for the record
	Out             io.Writer
	Now             func() time.Time
}

// Result is what a deploy produced.
type Result struct {
	Record   Record
	Dir      string // the artifact directory
	Deployed bool   // false for a dry run
}

// identity is a Target that knows who it connects as (the record's By).
type identity interface{ Identity() string }

// Deploy runs everything after the gate's check step (g) for environment o.Env: to target t,
// or, when t is nil, through script s.
func Deploy(ctx context.Context, o Options, g GateResult, t Target, s *Script) (Result, error) {
	w := o.Out
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Drain == 0 {
		o.Drain = DefaultDrain
	} else if o.Drain < 0 {
		o.Drain = 0
	}
	if o.HealthTimeout <= 0 {
		o.HealthTimeout = DefaultHealthTimeout
	}
	res := Result{}
	plat := Platform{runtime.GOOS, runtime.GOARCH}
	current := ""
	var builds []Build
	if t != nil {
		var err error
		if plat, err = t.Connect(ctx); err != nil {
			return res, err
		}
		if current, err = t.Current(ctx); err != nil {
			return res, err
		}
		if builds, err = t.Builds(ctx); err != nil {
			return res, err
		}
	}
	floor := Floor{From: "none yet"}
	switch {
	case o.Decl.FloorSet:
		floor = Floor{Value: o.Decl.Floor, From: "declared"}
	default:
		for _, b := range builds {
			if b.ID == current && b.Record != nil && b.Record.Gate.Mutate != nil {
				floor = Floor{Value: b.Record.Gate.Mutate.Score, From: "last deploy"}
			}
		}
	}
	g, err := GateRest(ctx, g, o.Runner, o.AppDir, o.Decl.Checks, floor, o.SkipTests, w)
	if err != nil {
		return res, err
	}

	dir, id, err := MakeArtifact(o.ArtifactRoot, o.Built, filepath.Join(o.AppDir, "migrations"), func(path string) error { return o.Binary(plat, path) })
	if err != nil {
		return res, fmt.Errorf("the artifact: %w", err)
	}
	res.Dir = dir
	fmt.Fprintf(w, "artifact %s (%s)\n", relative(dir), plat)
	desc := ""
	if t != nil {
		desc = t.Describe()
	} else {
		desc = s.Describe()
	}
	rec := Record{
		Build: id, App: o.App, Env: o.Env, Target: desc, Version: o.Version, Commit: o.Commit,
		Platform: plat.String(), Gate: g, Requires: append([]string{}, o.Decl.Requires...),
		AllowDataLoss: o.AllowDataLoss, UnsafeSkipTests: o.SkipTests,
	}
	if o.Decl.Backup != nil {
		rec.BackupURL = o.Decl.Backup.URL
	}

	var plan Plan
	if t != nil {
		fmt.Fprintln(w, "target")
		names := append(slices.Clone(o.Decl.Requires), SecretName)
		missing, err := t.Requires(ctx, names)
		if err != nil {
			return res, err
		}
		if len(missing) > 0 {
			line(w, "requires", "FAIL", "missing "+strings.Join(missing, ", "))
			return res, refuse("", "the environment at %s does not set %s: add %s to its environment file (NAME=value, one per line) and deploy again; nothing was uploaded",
				desc, strings.Join(missing, ", "), map[bool]string{true: "it", false: "them"}[len(missing) == 1])
		}
		line(w, "requires", "ok", strings.Join(names, ", "))
		live, err := t.Schema(ctx)
		if err != nil {
			return res, err
		}
		if plan, err = MakePlan(filepath.Join(dir, "migrations"), live.Snapshot, live.Have, true); err != nil {
			return res, refuse("", "%v", err)
		}
	} else if plan, err = MakePlan(filepath.Join(dir, "migrations"), nil, false, false); err != nil {
		return res, refuse("", "%v", err)
	}
	rec.Plan = plan
	fmt.Fprintln(w, "plan")
	for _, l := range plan.Lines() {
		fmt.Fprintln(w, "  "+l)
	}
	if drops := plan.DropsData(); len(drops) > 0 && !o.AllowDataLoss {
		var b strings.Builder
		for _, d := range drops {
			b.WriteString("  DROPS DATA: " + d + "\n")
		}
		res.Record = rec
		return res, refuse(b.String()+"run again with --allow-data-loss to apply it: the database is copied into the build's directory first (backup.db)",
			"the migration drops data")
	}
	if err := WriteDescription(dir, rec); err != nil {
		return res, err
	}
	res.Record = rec
	if o.Decl.Backup == nil && t != nil {
		fmt.Fprintln(w, "warning: no `backup` in the declaration: only the pre-migration copies (builds/<id>/backup.db) exist")
	}
	if o.Decl.Backup != nil && strings.HasPrefix(o.Decl.Backup.URL, "s3://") {
		for _, n := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
			if !slices.Contains(o.Decl.Requires, n) {
				fmt.Fprintf(w, "warning: `backup` streams to %s with the bucket's credentials: name %s in `requires` so the deploy checks the target sets it\n", o.Decl.Backup.URL, n)
			}
		}
	}
	if o.DryRun {
		fmt.Fprintln(w, "dry run: nothing was shipped")
		return res, nil
	}
	if t == nil {
		fmt.Fprintf(w, "script %s %s %s\n", s.Path, relative(dir), o.Env)
		if err := s.Run(ctx, dir, o.Env, o.Decl.Requires); err != nil {
			return res, err
		}
		res.Deployed = true
		fmt.Fprintf(w, "deployed %s to %s\n", id, o.Env)
		return res, nil
	}
	if id == current {
		fmt.Fprintf(w, "build %s already serves %s: nothing to ship\n", id, o.Env)
		return res, nil
	}

	fmt.Fprintln(w, "ship")
	if err := t.Upload(ctx, dir, id); err != nil {
		return res, err
	}
	line(w, "upload", "ok", "builds/"+id+"/")
	backup, err := t.Backup(ctx, id)
	if err != nil {
		return res, fmt.Errorf("the backup before migrating: %w", err)
	}
	if backup != "" {
		line(w, "backup", "ok", backup)
	} else {
		line(w, "backup", "ok", "no database yet")
	}
	rec.Backup = backup
	out, err := t.Migrate(ctx, id)
	if err != nil {
		line(w, "migrate", "FAIL", "")
		return res, refuse(lastLines(out, 20), "build %s's migrations did not apply: nothing was switched; %s", id, serving(current))
	}
	rec.Applied = appliedFrom(out)
	line(w, "migrate", "ok", appliedNote(rec.Applied))
	port, err := t.Start(ctx, id)
	if err != nil {
		line(w, "start", "FAIL", "")
		return res, startRefusal(id, current, err)
	}
	line(w, "start", "ok", fmt.Sprintf("port %d", port))
	if err := t.Health(ctx, id, port, o.HealthTimeout); err != nil {
		line(w, "health", "FAIL", "")
		t.Stop(ctx, id, 0)
		return res, startRefusal(id, current, err)
	}
	line(w, "health", "ok", "build "+id+" answers GET /")
	if err := t.Switch(ctx, id, port); err != nil {
		line(w, "switch", "FAIL", "")
		t.Stop(ctx, id, 0)
		return res, fmt.Errorf("switching to build %s: %w", id, err)
	}
	line(w, "switch", "ok", fmt.Sprintf("current → %s (port %d)", id, port))
	now := o.Now().UTC().Truncate(time.Second)
	rec.When, rec.Previous, rec.Port = &now, current, port
	rec.By = "local " + o.User
	if who, ok := t.(identity); ok {
		rec.By = who.Identity() + " (local " + o.User + ")"
	}
	if err := t.Record(ctx, id, rec.JSON()); err != nil {
		return res, err
	}
	res.Record, res.Deployed = rec, true
	if current != "" {
		if err := t.Stop(ctx, current, o.Drain); err != nil {
			fmt.Fprintf(w, "warning: stopping build %s: %v\n", current, err)
		} else if o.Drain > 0 {
			line(w, "stop", "ok", fmt.Sprintf("build %s stops after %s", current, o.Drain))
		} else {
			line(w, "stop", "ok", "build "+current)
		}
	}
	keep := o.Decl.Keep
	if keep <= 0 {
		keep = DefaultKeep
	}
	removed, err := t.Prune(ctx, keep, []string{id, current})
	if err != nil {
		fmt.Fprintf(w, "warning: pruning: %v\n", err)
	} else if len(removed) > 0 {
		line(w, "prune", "ok", fmt.Sprintf("removed %s (keep %d)", strings.Join(removed, ", "), keep))
	}
	fmt.Fprintf(w, "deployed %s to %s\n", id, o.Env)
	return res, nil
}

// startRefusal is the refusal of a build that did not start or did not pass its health check:
// the target's error carries the build's log.
func startRefusal(id, current string, err error) error {
	return refuse(err.Error(), "build %s did not pass its health check: it was stopped and builds/%s/ is kept for inspection; nothing was switched; %s", id, id, serving(current))
}

func serving(current string) string {
	if current == "" {
		return "no build serves yet"
	}
	return "build " + current + " still serves"
}

// appliedFrom are the migrations the binary's --migrate-only output says it applied
// ("migrated <name>" lines, as db.MigrateLog logs them).
func appliedFrom(out string) []string {
	var names []string
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, "migrated "); i >= 0 {
			names = append(names, strings.TrimSpace(l[i+len("migrated "):]))
		}
	}
	return names
}

func appliedNote(names []string) string {
	if len(names) == 0 {
		return "nothing to apply"
	}
	return "applied " + strings.Join(names, ", ")
}

// relative is path relative to the working directory when it is under it.
func relative(path string) string {
	if wd, err := filepath.Abs("."); err == nil {
		if r, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return path
}

// RollbackOptions configure Rollback.
type RollbackOptions struct {
	To                  string // "" for the build that served before the current one
	AllowSchemaMismatch bool
	Drain               time.Duration
	HealthTimeout       time.Duration
	Out                 io.Writer
}

// Rollback makes an earlier build current again: it starts it beside the current one, health
// checks it by build id, switches, and stops the current one. The database is not touched:
// when its schema is not the build's own snapshot (a later deploy migrated it), the build
// would refuse to start on it, so Rollback refuses first, naming the tables, unless
// AllowSchemaMismatch.
func Rollback(ctx context.Context, t Target, o RollbackOptions) (string, error) {
	w := o.Out
	if o.Drain == 0 {
		o.Drain = DefaultDrain
	} else if o.Drain < 0 {
		o.Drain = 0
	}
	if o.HealthTimeout <= 0 {
		o.HealthTimeout = DefaultHealthTimeout
	}
	if _, err := t.Connect(ctx); err != nil {
		return "", err
	}
	current, err := t.Current(ctx)
	if err != nil {
		return "", err
	}
	builds, err := t.Builds(ctx)
	if err != nil {
		return "", err
	}
	to := o.To
	if to == "" {
		for _, b := range builds { // the current build's Previous, else the newest other deployed one
			if b.ID == current && b.Record != nil && b.Record.Previous != "" {
				to = b.Record.Previous
			}
		}
		found := false
		for _, b := range builds {
			found = found || b.ID == to && b.Record != nil
		}
		if !found {
			to = ""
			for _, b := range builds {
				if b.ID != current && b.Record != nil {
					to = b.ID
					break
				}
			}
		}
		if to == "" {
			return "", refuse("", "there is no earlier deployed build at %s to roll back to", t.Describe())
		}
	}
	var target *Build
	for i, b := range builds {
		if b.ID == to || len(to) >= 4 && strings.HasPrefix(b.ID, to) {
			if target != nil {
				return "", refuse("", "build %s is ambiguous: give more of its id", to)
			}
			target = &builds[i]
		}
	}
	if target == nil {
		return "", refuse("", "there is no build %s at %s (`alder status` lists them)", to, t.Describe())
	}
	to = target.ID
	if target.Record == nil {
		return "", refuse("", "build %s never finished deploying (it has no deployed.json): deploy it instead", to)
	}
	if to == current {
		return "", refuse("", "build %s is already current", to)
	}
	fmt.Fprintf(w, "rollback %s → %s\n", current, to)
	live, err := t.Schema(ctx)
	if err != nil {
		return "", err
	}
	own, err := t.BuildSchema(ctx, to)
	if err != nil {
		return "", err
	}
	var lacks []string // migrations the database has had that the build does not have
	for _, m := range live.Migrations {
		if !slices.Contains(own.Migrations, m) {
			lacks = append(lacks, m)
		}
	}
	if live.Have && (!own.Have || SchemaHash(live.Snapshot) != SchemaHash(own.Snapshot) || len(lacks) > 0) {
		why := ""
		if tables := SchemaTables(own.Snapshot, live.Snapshot); SchemaHash(live.Snapshot) != SchemaHash(own.Snapshot) {
			why = fmt.Sprintf("the live database's schema %s is not build %s's %s (changed tables: %s)", SchemaHash(live.Snapshot), to, orNone(SchemaHash(own.Snapshot)), strings.Join(tables, ", "))
		} else {
			why = fmt.Sprintf("the live database has migrations build %s does not have (%s)", to, strings.Join(lacks, ", "))
		}
		if !o.AllowSchemaMismatch {
			line(w, "schema", "FAIL", "the live database is newer than the build")
			return "", refuse("", "%s: that build would refuse to start on it, and rolling back never changes the data. Deploy a build with the schema you want, or pass --allow-schema-mismatch to try anyway", why)
		}
		line(w, "schema", "MISMATCH", "--allow-schema-mismatch: "+why)
	} else {
		line(w, "schema", "ok", "the live schema is the build's")
	}
	port, err := t.Start(ctx, to)
	if err != nil {
		line(w, "start", "FAIL", "")
		return "", startRefusal(to, current, err)
	}
	line(w, "start", "ok", fmt.Sprintf("port %d", port))
	if err := t.Health(ctx, to, port, o.HealthTimeout); err != nil {
		line(w, "health", "FAIL", "")
		t.Stop(ctx, to, 0)
		return "", startRefusal(to, current, err)
	}
	line(w, "health", "ok", "build "+to+" answers GET /")
	if err := t.Switch(ctx, to, port); err != nil {
		t.Stop(ctx, to, 0)
		return "", fmt.Errorf("switching to build %s: %w", to, err)
	}
	line(w, "switch", "ok", fmt.Sprintf("current → %s (port %d)", to, port))
	if current != "" {
		if err := t.Stop(ctx, current, o.Drain); err != nil {
			fmt.Fprintf(w, "warning: stopping build %s: %v\n", current, err)
		}
	}
	fmt.Fprintf(w, "rolled back to %s\n", to)
	return to, nil
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// Status writes the target's current build, its last five builds, the database's size and
// the last backup.
func Status(ctx context.Context, t Target, w io.Writer) error {
	if _, err := t.Connect(ctx); err != nil {
		return err
	}
	current, err := t.Current(ctx)
	if err != nil {
		return err
	}
	builds, err := t.Builds(ctx)
	if err != nil {
		return err
	}
	info, err := t.Info(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, t.Describe())
	if current == "" {
		fmt.Fprintln(w, "current  none (not deployed yet)")
	} else {
		fmt.Fprintln(w, "current  "+current)
	}
	fmt.Fprintln(w, "builds")
	for i, b := range builds {
		if i == 5 {
			fmt.Fprintf(w, "  … %d more\n", len(builds)-5)
			break
		}
		mark := " "
		if b.ID == current {
			mark = "*"
		}
		fmt.Fprintf(w, "  %s %s  %s\n", mark, b.ID, buildNote(b))
	}
	if info.HaveDB {
		fmt.Fprintf(w, "database %s\n", size(info.DBBytes))
	} else {
		fmt.Fprintln(w, "database none yet")
	}
	if info.LastBackup.IsZero() {
		fmt.Fprintln(w, "backup   none yet")
	} else {
		fmt.Fprintf(w, "backup   %s (%s)\n", info.LastBackup.UTC().Format(time.RFC3339), info.BackupFrom)
	}
	return nil
}

func buildNote(b Build) string {
	r := b.Record
	if r == nil {
		note := "never switched to (failed or interrupted; kept for inspection)"
		if b.Running {
			note += ", running"
		}
		return note
	}
	parts := []string{}
	if r.When != nil {
		parts = append(parts, r.When.UTC().Format(time.RFC3339))
	}
	if r.By != "" {
		parts = append(parts, "by "+r.By)
	}
	if r.Gate.Mutate != nil {
		parts = append(parts, fmt.Sprintf("score %.2f", r.Gate.Mutate.Score))
	}
	if r.UnsafeSkipTests {
		parts = append(parts, "--unsafe-skip-tests")
	}
	if r.AllowDataLoss {
		parts = append(parts, "--allow-data-loss")
	}
	if len(r.Applied) > 0 {
		parts = append(parts, "migrated "+strings.Join(r.Applied, ", "))
	}
	if b.Running {
		parts = append(parts, "running")
	}
	return strings.Join(parts, "  ")
}

func size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f kB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
