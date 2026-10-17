package deploy

import (
	"context"
	"io"
	"time"
)

// Target is a place a build ships to and runs at, with its state there: the builds, the
// database, the environment file, the pointer to the current build (chapter 16). The engine
// (Deploy, Rollback) drives it step by step; Ssh implements it, and so does the Fly target.
// A Script target does not: it is handed the artifact and does the rest itself (Script).
//
// Every method that changes something at the target is called only after the gate passed and
// the plan was accepted; a --dry-run calls Connect, Requires, Schema, Current and Builds only.
type Target interface {
	// Describe is the target in one phrase: "Ssh deploy@box:/srv/blog".
	Describe() string
	// Connect reaches the target and reports the platform its binary must be built for. It
	// changes nothing there.
	Connect(ctx context.Context) (Platform, error)
	// Requires reports which of names the target's environment does not set (or sets empty).
	// It never reads a value back.
	Requires(ctx context.Context, names []string) (missing []string, err error)
	// Schema is the live database's schema: the snapshot (schema.json) of the last migration it
	// took and the migrations applied to it. Have is false when the target has no database yet.
	Schema(ctx context.Context) (Schema, error)
	// BuildSchema is build id's own: its migrations/schema.json and migration files (Have false
	// for an app without migrations).
	BuildSchema(ctx context.Context, id string) (Schema, error)
	// Current is the id of the build serving ("" before the first deploy).
	Current(ctx context.Context) (string, error)
	// Builds are the builds at the target, newest first.
	Builds(ctx context.Context) ([]Build, error)
	// Upload copies the artifact directory dir to the target as build id.
	Upload(ctx context.Context, dir, id string) error
	// Backup copies the live database into build id's directory (before any migration) and
	// returns where, relative to the target's root ("" when there is no database yet).
	Backup(ctx context.Context, id string) (string, error)
	// Migrate applies build id's pending migrations (the binary's --migrate-only), returning
	// its output; an error is the binary's refusal.
	Migrate(ctx context.Context, id string) (string, error)
	// Start starts build id beside the one serving and returns the port it listens on. A build
	// that exits instead is an error carrying its log.
	Start(ctx context.Context, id string) (port int, err error)
	// Health waits (at most timeout) until build id answers GET / on port with its build id in
	// the page's alder-build meta; the error carries the build's log.
	Health(ctx context.Context, id string, port int, timeout time.Duration) error
	// Switch sends the traffic to build id on port and makes it current, returning the port it
	// now serves on (another than port when the target restarts it on a public one).
	Switch(ctx context.Context, id string, port int) (int, error)
	// Stop stops build id after drain (in-flight requests finish; 0: now).
	Stop(ctx context.Context, id string, drain time.Duration) error
	// Record writes build id's deployed.json.
	Record(ctx context.Context, id string, rec []byte) error
	// Prune removes builds beyond the keep newest, never one in protect, and returns them.
	Prune(ctx context.Context, keep int, protect []string) ([]string, error)
	// Info is the database's size and its last backup.
	Info(ctx context.Context) (Info, error)
	// Logs writes the current build's log to w (the last lines; with follow, until ctx ends).
	Logs(ctx context.Context, follow bool, w io.Writer) error
	// Restore replaces the target's database with the local file db, stopping and restarting
	// the current build around it.
	Restore(ctx context.Context, db string) error
	// Close releases the connection.
	Close() error
}

// Schema is a database's schema or a build's, as a target reports it.
type Schema struct {
	Have       bool
	Snapshot   []byte   // schema.json
	Migrations []string // the migration names ("0002_add_mood"), in order
}

// DataLossAllower is a Target told whether this deploy may apply a migration that drops data
// (--allow-data-loss): its Migrate then runs the binary with ALDER_ALLOW_DATA_LOSS=1, which the
// runtime otherwise refuses such a migration without (rt/db.ErrDropsData). The engine calls it
// before Migrate.
type DataLossAllower interface{ AllowDataLoss(bool) }

// Locker is a Target that serializes the runs that change it (deploy, rollback, restore): Lock
// refuses (a *Refusal naming the holder) while another run holds it; unlock releases it.
type Locker interface {
	Lock(ctx context.Context) (unlock func(), err error)
}

// Preflighter is a Target that prepares what it needs to run a build (a service manager's
// unit, a proxy) before anything is uploaded, so a box that cannot run it refuses with
// nothing changed.
type Preflighter interface {
	Preflight(ctx context.Context) error
}

// Platform is the GOOS and GOARCH the target runs.
type Platform struct{ OS, Arch string }

func (p Platform) String() string { return p.OS + "/" + p.Arch }

// Build is one build at the target.
type Build struct {
	ID       string
	Record   *Record // nil when the build never finished deploying (its deployed.json is absent)
	Current  bool
	Running  bool
	Modified time.Time // when its directory was written
	BackupOf string    // the build its backup.db was taken under ("" for none): the data `rollback --restore-backup` to that build puts back
}

// Info is the target's database and backup state.
type Info struct {
	DBBytes    int64
	HaveDB     bool
	LastBackup time.Time // the newest of the shipper's last upload and the last pre-migration copy
	BackupFrom string    // "streamed to s3://…", "builds/<id>/backup.db"
}
