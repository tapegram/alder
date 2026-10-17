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
	"errors"
	"fmt"
	"io"
	"os"
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
		if l, ok := t.(Locker); ok && !o.DryRun {
			unlock, err := l.Lock(ctx)
			if err != nil {
				return res, err
			}
			defer unlock()
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
		s.AllowDataLoss = o.AllowDataLoss
		fmt.Fprintf(w, "script %s %s %s\n", s.Path, relative(dir), o.Env)
		if err := s.Run(ctx, dir, o.Env, append(slices.Clone(o.Decl.Requires), SecretName)); err != nil {
			return res, err
		}
		res.Deployed = true
		fmt.Fprintf(w, "deployed %s to %s\n", id, o.Env)
		return res, nil
	}
	// the current build again: nothing to ship when it runs; started again when it does not
	restart := false
	if id == current {
		for _, b := range builds {
			if b.ID == id && !b.Running {
				restart = true
			}
		}
		if !restart {
			fmt.Fprintf(w, "build %s already serves %s: nothing to ship\n", id, o.Env)
			return res, nil
		}
		fmt.Fprintf(w, "build %s is current but not running: starting it again\n", id)
	}

	if p, ok := t.(Preflighter); ok {
		if err := p.Preflight(ctx); err != nil {
			return res, err
		}
	}
	fmt.Fprintln(w, "ship")
	if !restart {
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
		if a, ok := t.(DataLossAllower); ok {
			a.AllowDataLoss(o.AllowDataLoss)
		}
		out, err := t.Migrate(ctx, id)
		if err != nil {
			line(w, "migrate", "FAIL", "")
			return res, refuse(lastLines(out, 20), "build %s's migrations did not apply: nothing was switched; %s", id, serving(current))
		}
		rec.Applied = appliedFrom(out)
		line(w, "migrate", "ok", appliedNote(rec.Applied))
	}
	failed := func(err error) error {
		if len(rec.Applied) > 0 && current != "" && rec.Backup != "" && len(plan.DropsData()) > 0 {
			if br, ok := t.(BackupRestorer); ok {
				return restoreAfterDrop(ctx, t, br, o, id, current, rec.Applied, err)
			}
		}
		_, restorer := t.(BackupRestorer)
		return startRefusalMigrated(id, current, o.Env, rec.Applied, restorer, err)
	}
	port, err := t.Start(ctx, id)
	if err != nil {
		line(w, "start", "FAIL", "")
		return res, failed(err)
	}
	line(w, "start", "ok", fmt.Sprintf("port %d", port))
	if err := t.Health(ctx, id, port, o.HealthTimeout); err != nil {
		line(w, "health", "FAIL", "")
		t.Stop(ctx, id, 0)
		return res, failed(err)
	}
	line(w, "health", "ok", "build "+id+" answers GET /")
	if port, err = t.Switch(ctx, id, port); err != nil {
		line(w, "switch", "FAIL", "")
		t.Stop(ctx, id, 0)
		return res, fmt.Errorf("switching to build %s: %w", id, err)
	}
	line(w, "switch", "ok", fmt.Sprintf("current → %s (port %d)", id, port))
	if restart {
		res.Deployed = true
		fmt.Fprintf(w, "started %s again on %s\n", id, o.Env)
		return res, nil
	}
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
		stopPrevious(ctx, t, current, o.Drain, w)
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

// stopPrevious stops the build that served before a switch, after drain, and says so.
func stopPrevious(ctx context.Context, t Target, id string, drain time.Duration, w io.Writer) {
	switch err := t.Stop(ctx, id, drain); {
	case errors.Is(err, ErrNotRunning):
		line(w, "stop", "ok", "build "+id+" already stopped")
	case err != nil:
		fmt.Fprintf(w, "warning: stopping build %s: %v\n", id, err)
	case drain > 0:
		line(w, "stop", "ok", fmt.Sprintf("build %s stops after %s (unless it is current again by then)", id, drain))
	default:
		line(w, "stop", "ok", "build "+id)
	}
}

// startRefusal is the refusal of a build that did not start or did not pass its health check:
// the target's error carries the build's log.
func startRefusal(id, current string, err error) error {
	return startRefusalMigrated(id, current, "", nil, false, err)
}

// startRefusalMigrated is startRefusal after this deploy applied migrations: the serving build
// keeps running, but would refuse to start again on the migrated database; the refusal names
// the copy taken before them and, on a target that can put it back (restorer:
// BackupRestorer), the command that does.
func startRefusalMigrated(id, current, env string, applied []string, restorer bool, err error) error {
	if len(applied) > 0 && current != "" {
		back := fmt.Sprintf("To go back: `alder rollback %s %s --restore-backup` (puts builds/%s/backup.db, taken before the migration, in place)", env, current, id)
		if !restorer {
			back = fmt.Sprintf("The copy taken before the migration is builds/%s/backup.db: put it in place with `alder restore %s` from a file you download, or deploy a build that starts on the migrated schema", id, env)
		}
		return refuse(err.Error(), "build %s did not pass its health check: it was stopped and builds/%s/ is kept for inspection; nothing was switched; build %s still serves, but the database was migrated (%s) and that build would refuse to start again on it. %s",
			id, id, current, strings.Join(applied, ", "), back)
	}
	return refuse(err.Error(), "build %s did not pass its health check: it was stopped and builds/%s/ is kept for inspection; nothing was switched; %s", id, id, serving(current))
}

// restoreAfterDrop is the answer to a new build that failed after its migrations dropped data
// (the plan's DROPS DATA, applied with --allow-data-loss): the build serving still runs, but
// on a database whose tables it no longer matches, so its writes are failing now. The copy
// taken before the migrations (builds/<id>/backup.db) is put back at once, without asking,
// and the previous build is started on it again; the migrated database is kept beside it.
// The result is always a refusal: it says what was done, or, when putting the copy back
// failed too, the command to run.
func restoreAfterDrop(ctx context.Context, t Target, br BackupRestorer, o Options, id, current string, applied []string, cause error) error {
	w := o.Out
	manual := fmt.Sprintf("build %s did not pass its health check after its migrations dropped data (%s), and putting the copy taken before them back failed", id, strings.Join(applied, ", "))
	fmt.Fprintf(w, "the migrations dropped data and build %s failed: putting builds/%s/backup.db (taken before them) back, and build %s on it\n", id, id, current)
	r, err := br.RestoreBackup(ctx, current, id)
	if err != nil {
		line(w, "restore", "FAIL", "")
		return refuse(cause.Error(), "%s (%v): build %s may be failing its writes on the migrated database now; run `alder rollback %s %s --restore-backup --yes` immediately", manual, err, current, o.Env, current)
	}
	line(w, "restore", "ok", r.From+" (the database it replaced is kept as "+r.Kept+")")
	port, err := t.Start(ctx, current)
	if err == nil {
		line(w, "start", "ok", fmt.Sprintf("port %d", port))
		if err = t.Health(ctx, current, port, o.HealthTimeout); err == nil {
			line(w, "health", "ok", "build "+current+" answers GET /")
			if port, err = t.Switch(ctx, current, port); err == nil {
				line(w, "switch", "ok", fmt.Sprintf("current → %s (port %d)", current, port))
			}
		}
	}
	if err != nil {
		line(w, "start", "FAIL", "")
		t.Stop(ctx, current, 0)
		uerr := r.Undo(ctx)
		if uerr == nil {
			if p, serr := t.Start(ctx, current); serr == nil {
				t.Switch(ctx, current, p)
			}
		}
		return refuse(cause.Error()+"\n"+err.Error(), "%s: build %s did not start on the copy (%v), so the migrated database is back in place; build %s may be failing its writes: restore a backup (`alder restore %s`) or deploy a build that fits the migrated schema", manual, current, err, current, o.Env)
	}
	return refuse(cause.Error(), "build %s did not pass its health check after its migrations dropped data (%s): the database copied before them (%s) was put back automatically and build %s serves again on it; the migrated database is kept as %s (a write made between the migration and now is only there); builds/%s/ is kept for inspection",
		id, strings.Join(applied, ", "), r.From, current, r.Kept, id)
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
	// RestoreBackup puts back the database as it was when the build was replaced (the
	// pre-migration copy the next deploy took: BackupRestorer), losing every write since.
	RestoreBackup bool
	// Confirm asks, before RestoreBackup replaces anything, whether to go ahead (nil: yes). A
	// false answer is ErrNotConfirmed.
	Confirm       func(question string) bool
	Env           string // the environment, for the commands the refusals name
	Drain         time.Duration
	HealthTimeout time.Duration
	Out           io.Writer
}

// ErrNotConfirmed is Rollback's answer when RollbackOptions.Confirm said no: nothing changed.
var ErrNotConfirmed = errors.New("not confirmed: nothing was changed (pass --yes to confirm without a terminal)")

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
	if l, ok := t.(Locker); ok {
		unlock, err := l.Lock(ctx)
		if err != nil {
			return "", err
		}
		defer unlock()
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
	if to == current && !o.RestoreBackup {
		return "", refuse("", "build %s is already current", to)
	}
	fmt.Fprintf(w, "rollback %s → %s\n", current, to)
	if o.RestoreBackup {
		return rollbackWithBackup(ctx, t, o, builds, current, *target)
	}
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
			restore := ""
			if _, ok := t.(BackupRestorer); ok {
				if src := CopyFor(to, builds); src != "" {
					restore = fmt.Sprintf("; or `alder rollback %s %s --restore-backup` puts back the copy of the data taken when that build was replaced (builds/%s/backup.db), losing every write since", o.Env, to, src)
				}
			}
			return "", refuse("", "%s: that build would refuse to start on it, and rolling back never changes the data. Deploy a build with the schema you want, or pass --allow-schema-mismatch to try anyway%s", why, restore)
		}
		line(w, "schema", "MISMATCH", "--allow-schema-mismatch: "+why)
	} else {
		line(w, "schema", "ok", "the live schema is the build's")
	}
	if p, ok := t.(Preflighter); ok {
		if err := p.Preflight(ctx); err != nil {
			return "", err
		}
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
	if port, err = t.Switch(ctx, to, port); err != nil {
		t.Stop(ctx, to, 0)
		return "", fmt.Errorf("switching to build %s: %w", to, err)
	}
	line(w, "switch", "ok", fmt.Sprintf("current → %s (port %d)", to, port))
	recordRollback(ctx, t, *target, current, port, "", w)
	if current != "" {
		stopPrevious(ctx, t, current, o.Drain, w)
	}
	fmt.Fprintf(w, "rolled back to %s\n", to)
	return to, nil
}

// recordRollback rewrites the deployed.json of build b, now current again: when, by whom, the
// build it replaced, and the copy of the data that was put back (if any).
func recordRollback(ctx context.Context, t Target, b Build, previous string, port int, restored string, w io.Writer) {
	rec := *b.Record
	now := time.Now().UTC().Truncate(time.Second)
	rec.When, rec.Previous, rec.Port, rec.RolledBack, rec.RestoredFrom = &now, previous, port, true, restored
	rec.By = "local " + os.Getenv("USER")
	if who, ok := t.(identity); ok {
		rec.By = who.Identity() + " (" + rec.By + ")"
	}
	if err := t.Record(ctx, b.ID, rec.JSON()); err != nil {
		fmt.Fprintf(w, "warning: recording the rollback: %v\n", err)
	}
}

// BackupRestorer is a Target that can put back the pre-migration copy taken under a build
// (`alder rollback --restore-backup`, and a failed deploy whose migrations dropped data):
// RestoreBackup stops EVERY running build (a build still draining after a switch included,
// its delayed stop cancelled: no process may keep the replaced file open and go on serving
// it), keeps the current database beside the copy's place, and puts builds/<src>/backup.db in
// place with build to's schema records. The engine starts a build on it afterwards.
type BackupRestorer interface {
	RestoreBackup(ctx context.Context, to, src string) (Restored, error)
}

// Restored is what RestoreBackup put in place.
type Restored struct {
	From  string    // builds/<src>/backup.db
	Taken time.Time // when the copy was taken
	Kept  string    // where the database it replaced is kept: db/app.db.before-rollback-<time>
	// Undo puts the kept database back (every build stopped first).
	Undo func(context.Context) error
}

// CopyFor is the build holding the newest copy of the database taken while build to served
// (the first of builds, newest first, whose BackupOf is to), or "".
func CopyFor(to string, builds []Build) string {
	for _, b := range builds {
		if b.BackupOf == to {
			return b.ID
		}
	}
	return ""
}

// rollbackWithBackup is Rollback with RestoreBackup: the database goes back to the copy taken
// when build to was replaced, then to is started, checked and switched to; when it fails, the
// kept database and the current build come back.
func rollbackWithBackup(ctx context.Context, t Target, o RollbackOptions, builds []Build, current string, to Build) (string, error) {
	w := o.Out
	br, ok := t.(BackupRestorer)
	if !ok {
		return "", refuse("", "--restore-backup is not available for %s", t.Describe())
	}
	if p, ok := t.(Preflighter); ok {
		if err := p.Preflight(ctx); err != nil {
			return "", err
		}
	}
	src := CopyFor(to.ID, builds)
	if src == "" {
		return "", refuse("", "no build at %s holds a copy of the database taken while build %s served (builds/<id>/backup.db with backup.of %s): --restore-backup cannot put one back", t.Describe(), to.ID, to.ID)
	}
	if o.Confirm != nil && !o.Confirm(fmt.Sprintf("put back builds/%s/backup.db, the database as it was when build %s was replaced, and lose every write since (the current database is kept beside it)? type yes: ", src, to.ID)) {
		return "", ErrNotConfirmed
	}
	r, err := br.RestoreBackup(ctx, to.ID, src)
	if err != nil {
		return "", err
	}
	from := r.From
	fmt.Fprintf(w, "warning: --restore-backup: the database is now %s (taken %s); every write since is gone from it (the database it replaces is kept as %s)\n",
		from, r.Taken.UTC().Format(time.RFC3339), r.Kept)
	line(w, "restore", "ok", from)
	back := func(cause error) error {
		t.Stop(ctx, to.ID, 0)
		if uerr := r.Undo(ctx); uerr != nil {
			return fmt.Errorf("%v; and putting the previous database back failed: %v", cause, uerr)
		}
		if current != "" {
			if p, err := t.Start(ctx, current); err == nil {
				if _, err := t.Switch(ctx, current, p); err == nil {
					return refuse(cause.Error(), "build %s did not pass its health check on the restored copy: the previous database is back and build %s serves again", to.ID, current)
				}
			}
			return refuse(cause.Error(), "build %s did not pass its health check on the restored copy: the previous database is back, but build %s did not start again: run `alder deploy`", to.ID, current)
		}
		return refuse(cause.Error(), "build %s did not pass its health check on the restored copy: the previous database is back", to.ID)
	}
	port, err := t.Start(ctx, to.ID)
	if err == nil {
		line(w, "start", "ok", fmt.Sprintf("port %d", port))
		err = t.Health(ctx, to.ID, port, o.HealthTimeout)
	}
	if err != nil {
		line(w, "health", "FAIL", "")
		return "", back(err)
	}
	line(w, "health", "ok", "build "+to.ID+" answers GET /")
	if port, err = t.Switch(ctx, to.ID, port); err != nil {
		return "", back(fmt.Errorf("switching to build %s: %w", to.ID, err))
	}
	line(w, "switch", "ok", fmt.Sprintf("current → %s (port %d)", to.ID, port))
	recordRollback(ctx, t, to, current, port, from, w)
	fmt.Fprintf(w, "rolled back to %s with the data of %s\n", to.ID, r.Taken.UTC().Format(time.RFC3339))
	return to.ID, nil
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
