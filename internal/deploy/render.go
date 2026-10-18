package deploy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"alder/internal/ir"
	"alder/rt/db"
)

// Render is an environment on Render (chapter 16, "The Render target"): a stateless web service
// running the build's image, its data in a shared Turso database (Turso, as an Ssh environment's
// `database:`). Render does the roll: it starts the new instances beside the old ones, checks
// /_alder/health, switches and stops the old ones, so the engine's Switch and Stop change
// nothing there. What a deploy needs to remember lives in the shared database, the one durable
// thing: the builds (_alder_deploy_builds: each build's record, schema.json, migration names and
// the point in time taken before it migrated), the build serving (_alder_deploy_current) and the
// deploy lock (_alder_lock).
//
// The step mapping: Connect finds the workspace and the service by name (and the shared
// database's name in its ALDER_TURSO_DB); Lock creates the shared database when absent, takes
// its lock and creates the deploy tables (R7); Upload builds and pushes the image; Backup
// records the point in time; Migrate runs the build's migrations from this machine
// (Turso.Migrate); Start creates the service (R2), or merges its environment (R3) and scales it
// and creates a deploy of <image>:<id>, and waits until Render reports it live (nothing changes
// at Render before Start); Health
// waits until `instances` consecutive answers of /_alder/health carry the build; Switch records
// it current.
//
// The environment it reads on this machine (never in App): RENDER_API_KEY and ALDER_RENDER_API
// (RenderAPI), ALDER_REGISTRY_TOKEN and ALDER_REGISTRY_USER (the push and Render's registry
// credential), TURSO_API_TOKEN (Turso), RENDER_OWNER_ID (the workspace, when the key reaches
// several) and the `requires` names. How long Render may take to roll is RollTimeout (the command
// reads ALDER_DEPLOY_HEALTH_TIMEOUT once for every target). No value of the service's environment is ever printed.
type Render struct {
	Env, AppName, Region, Service, Plan, Image string
	Instances                                  int
	DB                                         *Turso
	API                                        *RenderAPI
	Reg                                        *Registry
	// Artifact is the local directory of build id's artifact (build/<app>/deploy/<id>); a build
	// whose directory is absent (a fresh checkout) is read from its builds row instead.
	Artifact func(id string) string
	// Getenv reads this machine's environment (the `requires` values, RENDER_OWNER_ID).
	Getenv func(string) string
	// RollTimeout is how long a deploy may take to go live before it is cancelled (0: 10
	// minutes). The command sets it from ALDER_DEPLOY_HEALTH_TIMEOUT, as it does the health
	// timeout of the other targets.
	RollTimeout time.Duration
	// Required are the declaration's `requires` names, whose values set here go to the service.
	Required []string
	// Out receives the transcript lines of the steps ("image pushed …", "Render deploy … live",
	// "health: …").
	Out io.Writer
	// HTTP is the client of the health check.
	HTTP *http.Client

	imageHost string // the image's registry host as declared (Render's registry kind)
	imageErr  error
	owner     string
	svc       RenderService
	have      bool        // the service exists
	env       []RenderEnv // the service's environment as read: values never printed
	allowLoss bool
	dirs      map[string]string // artifact directories uploaded in this run, by build id

	st      *db.SQLite // the shared database, opened for stName
	stName  string
	unlocks []func() // locks taken in this run beyond Turso's own (a copy made the app's)
}

// RenderPort is what Start and Switch report: Render serves the service on https.
const RenderPort = 443

// RenderAddr is the address the app listens on inside Render's container (the image exposes it).
const RenderAddr = ":10000"

// renderPoll is how often Start asks Render about a deploy and Health asks again after an answer
// without the build; tests shorten it.
var renderPoll = 3 * time.Second

// renderHealthPath is the runtime's health route (rt/web.HealthPath).
const renderHealthPath = "/_alder/health"

// NewRender is environment env's Render target t of app, with the declaration d and the
// registry path image (resolved by the command, R5). It reads RENDER_API_KEY and ALDER_RENDER_API
// (NewRenderAPI), ALDER_REGISTRY_TOKEN, ALDER_REGISTRY_USER (default: the image's first path part)
// and ALDER_REGISTRY_BASE: the registry's API base, an override for the tests only (R-A).
func NewRender(app, env string, t ir.Target, d *ir.Deploy, image string) *Render {
	r := &Render{Env: env, AppName: app, Region: t.Region, Service: t.Service, Plan: t.Plan, Image: image, Instances: t.Instances,
		API: NewRenderAPI(), Getenv: os.Getenv, Out: io.Discard, HTTP: &http.Client{Timeout: 10 * time.Second}, dirs: map[string]string{}}
	if r.Instances < 1 {
		r.Instances = 1
	}
	if d != nil {
		r.Required = slices.Clone(d.Requires)
	}
	if t.Database != nil {
		r.DB = NewTurso(t.Database)
	}
	host, repo, err := ParseImage(image)
	r.imageErr = err
	r.imageHost, _, _ = strings.Cut(image, "/")
	user := os.Getenv("ALDER_REGISTRY_USER")
	if user == "" {
		user, _, _ = strings.Cut(strings.TrimPrefix(image, r.imageHost+"/"), "/")
	}
	r.Reg = &Registry{Host: host, Repo: repo, User: user, Token: os.Getenv("ALDER_REGISTRY_TOKEN"), Base: os.Getenv("ALDER_REGISTRY_BASE"),
		Client: &http.Client{Timeout: 10 * time.Minute}}
	return r
}

// Describe is "Render blog (oregon)".
func (r *Render) Describe() string { return "Render " + r.Service + " (" + r.Region + ")" }

// Identity is the record's By: "render:<service>".
func (r *Render) Identity() string { return "render:" + r.Service }

// AllowDataLoss is DataLossAllower: Migrate runs with ALDER_ALLOW_DATA_LOSS=1.
func (r *Render) AllowDataLoss(allow bool) { r.allowLoss = allow }

// StreamsBackup is StreamChecker: Turso's point-in-time recovery is the backup.
func (r *Render) StreamsBackup(context.Context) (bool, error) { return true, nil }

// Address is the service's URL (R6): Render's serviceDetails.url once Connect found the service,
// before that the one it is expected to get, https://<service>.onrender.com (AddressExpected).
func (r *Render) Address() string {
	if r.have && r.svc.URL != "" {
		return strings.TrimRight(r.svc.URL, "/")
	}
	return "https://" + r.Service + ".onrender.com"
}

// AddressExpected reports whether Address is the expected URL rather than Render's (the service
// does not exist yet, or Connect has not run): a name Render finds taken gets a suffix.
func (r *Render) AddressExpected() bool { return !r.have || r.svc.URL == "" }

// Close releases the shared database's connection.
func (r *Render) Close() error {
	if r.st != nil {
		r.st.Close()
		r.st, r.stName = nil, ""
	}
	return nil
}

// ref is build id's image reference.
func (r *Render) ref(id string) string { return r.Image + ":" + id }

// Connect finds the workspace (the first the key reaches, unless RENDER_OWNER_ID names one) and
// the service by name, and reads the shared database's name from the service's ALDER_TURSO_DB.
// It changes nothing. The platform is Render's only one, linux/amd64.
func (r *Render) Connect(ctx context.Context) (Platform, error) {
	switch {
	case r.imageErr != nil:
		return Platform{}, refuse("", "%v", r.imageErr)
	case r.DB == nil:
		return Platform{}, refuse("", "a Render service keeps nothing between deploys: give it `database: Turso \"org\" { app }`")
	case r.API.Key == "":
		return Platform{}, refuse("", "this machine's environment does not set RENDER_API_KEY: a deploy to %s uses Render's API (create a key in Render's account settings and export it)", r.Describe())
	}
	if err := r.DB.tokenSet(); err != nil {
		return Platform{}, err
	}
	if r.owner == "" {
		if id := r.Getenv("RENDER_OWNER_ID"); id != "" {
			r.owner = id
		} else {
			owners, err := r.API.Owners(ctx)
			if err != nil {
				return Platform{}, err
			}
			if len(owners) == 0 {
				return Platform{}, refuse("", "the Render API key reaches no workspace")
			}
			r.owner = owners[0].ID
		}
	}
	svc, ok, err := r.API.ServiceByName(ctx, r.Service)
	if err != nil {
		return Platform{}, err
	}
	r.svc, r.have, r.env = svc, ok, nil
	if ok {
		if r.env, err = r.API.EnvVars(ctx, svc.ID); err != nil {
			return Platform{}, err
		}
		r.DB.SetShared(r.envValue(db.EnvTursoDB))
	}
	return Platform{OS: "linux", Arch: "amd64"}, nil
}

// envValue is the service's variable name ("" when unset).
func (r *Render) envValue(name string) string {
	for _, e := range r.env {
		if e.Key == name {
			return e.Value
		}
	}
	return ""
}

// Requires reports the names set neither on this machine nor at the service (R3). ALDER_SECRET
// is never missing: the service's is kept, or one is generated.
func (r *Render) Requires(ctx context.Context, names []string) ([]string, error) {
	var missing []string
	for _, n := range names {
		if n == SecretName || r.Getenv(n) != "" || r.envValue(n) != "" {
			continue
		}
		missing = append(missing, n)
	}
	return missing, nil
}

// Lock creates the shared database when it does not exist yet (Turso.Preflight), takes its
// deploy lock and creates the deploy tables under it (R7, R4): two first deploys cannot both
// proceed.
func (r *Render) Lock(ctx context.Context) (func(), error) {
	if _, err := r.DB.Preflight(ctx); err != nil {
		return nil, err
	}
	unlockDB, err := r.DB.Lock(ctx, "local "+r.Getenv("USER")+" at "+r.Identity())
	if err != nil {
		return nil, err
	}
	st, err := r.store(ctx)
	if err == nil && st == nil {
		err = fmt.Errorf("Turso has no database %s", r.DB.SharedName())
	}
	if err == nil {
		err = db.EnsureDeployTables(ctx, st)
	}
	if err != nil {
		unlockDB()
		return nil, err
	}
	return func() {
		for _, u := range r.unlocks {
			u()
		}
		r.unlocks = nil
		unlockDB()
	}, nil
}

// Preflight is Turso's: the token, the group, the names, the shared database created when
// absent. (Render's key answered Connect.)
func (r *Render) Preflight(ctx context.Context) error {
	_, err := r.DB.Preflight(ctx)
	return err
}

// store is the shared database, opened once per name; nil when it does not exist yet. Opening it
// mints a token for it; nothing is written.
func (r *Render) store(ctx context.Context) (*db.SQLite, error) {
	name := r.DB.SharedName()
	if r.st != nil && r.stName == name {
		return r.st, nil
	}
	_, ok, err := r.DB.sharedDB(ctx)
	if err != nil || !ok {
		return nil, err
	}
	st, err := r.DB.api().Open(ctx, name, false)
	if err != nil {
		return nil, err
	}
	r.Close()
	r.st, r.stName = st, name
	return st, nil
}

// tables are the names of st's tables among the ones the target reads.
func tables(ctx context.Context, st *db.SQLite) (map[string]bool, error) {
	rows, err := st.Query(ctx, nil, "sqlite_schema", []db.Column{{Name: "name"}},
		db.SQL{Where: `"type" = 'table' AND "name" IN ('_alder_meta', '_alder_migrations', '_alder_deploy_builds', '_alder_deploy_current')`})
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, row := range rows {
		out[text(row[0])] = true
	}
	return out, nil
}

func text(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// appliedNames are the migrations _alder_migrations records, sorted.
func appliedNames(ctx context.Context, st *db.SQLite) ([]string, error) {
	rows, err := st.Query(ctx, nil, "_alder_migrations", []db.Column{{Name: "name"}}, db.SQL{Order: `"name"`})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, row := range rows {
		out = append(out, text(row[0]))
	}
	return out, nil
}

// rows are the builds rows, newest first, with the tables read (nil before the first deploy).
func (r *Render) rows(ctx context.Context) ([]db.DeployRow, map[string]bool, *db.SQLite, error) {
	st, err := r.store(ctx)
	if err != nil || st == nil {
		return nil, nil, nil, err
	}
	have, err := tables(ctx, st)
	if err != nil {
		return nil, nil, nil, err
	}
	if !have["_alder_deploy_builds"] {
		return nil, have, st, nil
	}
	rows, err := db.DeployRows(ctx, st)
	return rows, have, st, err
}

func rowOf(rows []db.DeployRow, id string) (db.DeployRow, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return db.DeployRow{}, false
}

func rowMigrations(row db.DeployRow) []string {
	var names []string
	json.Unmarshal([]byte(row.Migrations), &names)
	return names
}

// Current is the build _alder_deploy_current names ("" before the first deploy).
func (r *Render) Current(ctx context.Context) (string, error) {
	st, err := r.store(ctx)
	if err != nil || st == nil {
		return "", err
	}
	have, err := tables(ctx, st)
	if err != nil || !have["_alder_deploy_current"] {
		return "", err
	}
	return db.CurrentBuild(ctx, st)
}

// pitr is a builds row's point-in-time mark: "<time> <database> [<the build serving then>]".
type pitr struct {
	at       time.Time
	db, from string
}

func parsePitr(s string) (pitr, bool) {
	f := strings.Fields(s)
	if len(f) < 2 {
		return pitr{}, false
	}
	at, err := time.Parse(time.RFC3339, f[0])
	if err != nil {
		return pitr{}, false
	}
	p := pitr{at: at, db: f[1]}
	if len(f) > 2 {
		p.from = f[2]
	}
	return p, true
}

// live is the build tag of the service's live deploy ("" when none).
func (r *Render) live(ctx context.Context) (RenderDeploy, string, error) {
	if !r.have {
		return RenderDeploy{}, "", nil
	}
	deps, err := r.API.Deploys(ctx, r.svc.ID, 20)
	if err != nil {
		return RenderDeploy{}, "", err
	}
	for _, d := range deps {
		if d.Status == "live" {
			return d, tagOf(d.ImageURL), nil
		}
	}
	return RenderDeploy{}, "", nil
}

// tagOf is the tag of an image reference ("" without one).
func tagOf(ref string) string {
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[i+1:]
	}
	return ""
}

// Builds are the builds rows, newest first: Running is the build Render's live deploy runs,
// BackupOf the build serving when the row's point in time was taken.
func (r *Render) Builds(ctx context.Context) ([]Build, error) {
	rows, _, _, err := r.rows(ctx)
	if err != nil {
		return nil, err
	}
	current, err := r.Current(ctx)
	if err != nil {
		return nil, err
	}
	_, running, err := r.live(ctx)
	if err != nil {
		return nil, err
	}
	var out []Build
	for _, row := range rows {
		b := Build{ID: row.ID, Current: row.ID == current, Running: running != "" && row.ID == running, Modified: row.Created}
		if row.Record != "" {
			var rec Record
			if err := json.Unmarshal([]byte(row.Record), &rec); err == nil {
				b.Record = &rec
			}
		}
		if p, ok := parsePitr(row.Pitr); ok {
			b.BackupOf = p.from
			if b.BackupOf == "" && b.Record != nil {
				b.BackupOf = b.Record.Previous
			}
		}
		out = append(out, b)
	}
	return out, nil
}

// Schema is the shared database's: Have false before it holds the app's tables; its migrations
// are the ones _alder_migrations records, its snapshot the schema.json of a build with exactly
// those migrations (the builds rows), else the current build's. A point-in-time copy carries its
// own _alder_migrations, so the schema follows the data.
func (r *Render) Schema(ctx context.Context) (Schema, error) {
	rows, have, st, err := r.rows(ctx)
	if err != nil || st == nil || !have["_alder_meta"] {
		return Schema{}, err
	}
	var applied []string
	if have["_alder_migrations"] {
		if applied, err = appliedNames(ctx, st); err != nil {
			return Schema{}, err
		}
	}
	sc := Schema{Have: true, Migrations: applied}
	for _, row := range rows {
		if row.Schema != "" && slices.Equal(rowMigrations(row), applied) {
			sc.Snapshot = []byte(row.Schema)
			return sc, nil
		}
	}
	if cur, err := r.Current(ctx); err == nil && cur != "" {
		if row, ok := rowOf(rows, cur); ok && row.Schema != "" {
			sc.Snapshot = []byte(row.Schema)
			return sc, nil
		}
	}
	sc.Snapshot = []byte(`{"tables": []}`)
	return sc, nil
}

// localArtifact is build id's artifact directory on this machine, "" when there is none.
func (r *Render) localArtifact(id string) string {
	if d := r.dirs[id]; d != "" {
		return d
	}
	if r.Artifact == nil {
		return ""
	}
	d := r.Artifact(id)
	if st, err := os.Stat(filepath.Join(d, BinaryName)); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	return d
}

// BuildSchema is build id's migrations/: from its local artifact when this machine has it, else
// from its builds row (R4: a rollback from a fresh checkout).
func (r *Render) BuildSchema(ctx context.Context, id string) (Schema, error) {
	if dir := r.localArtifact(id); dir != "" {
		mig := filepath.Join(dir, "migrations")
		snap, err := os.ReadFile(filepath.Join(mig, "schema.json"))
		if errors.Is(err, os.ErrNotExist) {
			return Schema{}, nil
		} else if err != nil {
			return Schema{}, err
		}
		files, _ := filepath.Glob(filepath.Join(mig, "*.sql"))
		sc := Schema{Have: true, Snapshot: snap}
		for _, f := range files {
			sc.Migrations = append(sc.Migrations, strings.TrimSuffix(filepath.Base(f), ".sql"))
		}
		slices.Sort(sc.Migrations)
		return sc, nil
	}
	rows, _, _, err := r.rows(ctx)
	if err != nil {
		return Schema{}, err
	}
	row, ok := rowOf(rows, id)
	if !ok || row.Schema == "" {
		return Schema{}, nil
	}
	return Schema{Have: true, Snapshot: []byte(row.Schema), Migrations: rowMigrations(row)}, nil
}

// putRow updates build id's row with change (made when absent, Created now).
func (r *Render) putRow(ctx context.Context, id string, change func(*db.DeployRow)) error {
	rows, have, st, err := r.rows(ctx)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("Turso has no database %s", r.DB.SharedName())
	}
	if !have["_alder_deploy_builds"] {
		if err := db.EnsureDeployTables(ctx, st); err != nil {
			return err
		}
	}
	row, ok := rowOf(rows, id)
	if !ok {
		row = db.DeployRow{ID: id, Created: time.Now().UTC()}
	}
	change(&row)
	return db.PutDeployRow(ctx, st, row)
}

// withSchema puts build id's schema.json, its hash and its migrations' names in row.
func (r *Render) withSchema(ctx context.Context, id string, row *db.DeployRow) error {
	sc, err := r.BuildSchema(ctx, id)
	if err != nil || !sc.Have {
		return err
	}
	names, _ := json.Marshal(append([]string{}, sc.Migrations...))
	row.Schema, row.SchemaHash, row.Migrations = string(sc.Snapshot), SchemaHash(sc.Snapshot), string(names)
	return nil
}

// Upload builds build id's image from its artifact dir and pushes it as <image>:<id>. Nothing
// changes at Render: Start sets the environment and scale (prepare) or creates the service (R2).
func (r *Render) Upload(ctx context.Context, dir, id string) error {
	img, err := BuildImage(dir, id)
	if err != nil {
		return err
	}
	if err := r.Reg.Push(ctx, img, id); err != nil {
		return fmt.Errorf("pushing %s: %w", r.ref(id), err)
	}
	fmt.Fprintf(r.Out, "image pushed %s (%s)\n", r.ref(id), size(int64(len(img.Layer))))
	r.dirs[id] = dir
	return nil
}

// prepare brings an existing service in line before build id's deploy: its registry credential
// updated with this run's token (credential), its environment merged
// (R3: one PUT, only when something changed; a deploy Render started for it that is not ours is
// cancelled) and its instance count when `instances` changed. Start calls it, after the
// migrations, so a refusal before Start changes nothing at Render.
func (r *Render) prepare(ctx context.Context, id string) error {
	if _, err := r.credential(ctx); err != nil {
		return err
	}
	env, changed, err := r.mergedEnv(ctx)
	if err != nil {
		return err
	}
	if changed {
		if err := r.API.PutEnvVars(ctx, r.svc.ID, env); err != nil {
			return fmt.Errorf("setting the environment of the service %s: %w", r.Service, err)
		}
		r.env = env
		fmt.Fprintf(r.Out, "environment of %s set (%d variables)\n", r.Service, len(env))
		if err := r.cancelForeign(ctx, id); err != nil {
			return err
		}
	}
	if r.svc.NumInstances != r.Instances {
		if err := r.API.Scale(ctx, r.svc.ID, r.Instances); err != nil {
			return fmt.Errorf("scaling the service %s to %d instances: %w", r.Service, r.Instances, err)
		}
		fmt.Fprintf(r.Out, "service %s scaled %d → %d instances\n", r.Service, r.svc.NumInstances, r.Instances)
		r.svc.NumInstances = r.Instances
	}
	return nil
}

// cancelForeign cancels a deploy in progress whose image is not build id's (one Render may start
// for an environment change), so the roll Start triggers is the one that counts.
func (r *Render) cancelForeign(ctx context.Context, id string) error {
	deps, err := r.API.Deploys(ctx, r.svc.ID, 10)
	if err != nil {
		return err
	}
	for _, d := range deps {
		if d.Status == "live" || d.Failed() || d.ImageURL == r.ref(id) {
			continue
		}
		if err := r.API.CancelDeploy(ctx, r.svc.ID, d.ID); err != nil {
			return fmt.Errorf("cancelling Render's deploy %s (%s): %w", d.ID, d.Status, err)
		}
		fmt.Fprintf(r.Out, "Render deploy %s (%s) cancelled: not this build's\n", d.ID, d.Status)
	}
	return nil
}

// mergedEnv is the service's environment with alder's variables and the `requires` values set on
// this machine laid over it (R3): a variable set only at the service (in the dashboard) is
// kept, and ALDER_SECRET is the service's own, or a new one. changed reports whether it differs
// from the service's. No value is ever printed.
func (r *Render) mergedEnv(ctx context.Context) ([]RenderEnv, bool, error) {
	host, err := r.DB.Host(ctx)
	if err != nil {
		return nil, false, err
	}
	c := r.DB.api().Config()
	secret := r.envValue(SecretName)
	if secret == "" {
		var b [24]byte
		rand.Read(b[:])
		secret = hex.EncodeToString(b[:])
	}
	set := map[string]string{
		"ALDER_DB": "libsql://" + host, db.EnvTursoOrg: r.DB.Org, db.EnvTursoGroup: r.DB.Group, db.EnvTursoApp: r.DB.App, db.EnvTursoDB: r.DB.SharedName(),
		db.EnvTursoAPI: c.API, db.EnvTursoToken: c.Token, "ALDER_ADDR": RenderAddr, SecretName: secret,
	}
	for _, n := range r.Required {
		if v := r.Getenv(n); v != "" && n != SecretName {
			set[n] = v
		}
	}
	return overlay(r.env, set)
}

// overlay is env with set's values (new names appended in order); changed when it differs.
func overlay(env []RenderEnv, set map[string]string) ([]RenderEnv, bool, error) {
	out := slices.Clone(env)
	changed := false
	done := map[string]bool{}
	for i, e := range out {
		if v, ok := set[e.Key]; ok {
			done[e.Key] = true
			if e.Value != v {
				out[i].Value, changed = v, true
			}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		if !done[k] {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		out = append(out, RenderEnv{Key: k, Value: set[k]})
		changed = true
	}
	return out, changed, nil
}

// Backup records, in build id's row, the point in time before its migrations (the next whole
// second, waited for: every write before it is in the copy, every change of the migrations
// after it), the shared database's name and the build serving. Nothing when the database has no
// tables yet.
func (r *Render) Backup(ctx context.Context, id string) (string, error) {
	_, have, st, err := r.rows(ctx)
	if err != nil || st == nil || !have["_alder_meta"] {
		return "", err
	}
	current, err := r.Current(ctx)
	if err != nil {
		return "", err
	}
	next := r.DB.Now().UTC().Truncate(time.Second).Add(time.Second)
	if d := time.Until(next); d > 0 && d <= time.Second {
		time.Sleep(d + 50*time.Millisecond)
	}
	at := next.Format(time.RFC3339)
	shared := r.DB.SharedName()
	if err := r.putRow(ctx, id, func(row *db.DeployRow) { row.Pitr = strings.TrimSpace(at + " " + shared + " " + current) }); err != nil {
		return "", err
	}
	return "Turso point in time " + at + " of " + shared, nil
}

// Migrate runs build id's migrations from this machine (Turso.Migrate) and records its schema in
// its row, which Schema finds by the migrations the database then records.
func (r *Render) Migrate(ctx context.Context, id string) (string, error) {
	out, err := r.DB.Migrate(ctx, id, r.allowLoss)
	if err != nil {
		return out, err
	}
	var serr error
	if err := r.putRow(ctx, id, func(row *db.DeployRow) { serr = r.withSchema(ctx, id, row) }); err != nil {
		return out, err
	}
	return out, serr
}

// credential is the id of Render's credential for the registry, holding ALDER_REGISTRY_TOKEN:
// made on the first deploy of the image, and on every later deploy found by name and updated
// with the token of this run, so a new or rotated token reaches Render, which pulls the image
// with it on every restart; "" without a token (a public image).
func (r *Render) credential(ctx context.Context) (string, error) {
	if r.Reg.Token == "" {
		return "", nil
	}
	name := "alder-" + strings.TrimPrefix(r.Image, r.imageHost+"/")
	kind, known := ir.RegistryKind(r.imageHost)
	if !known {
		return "", refuse("", "Render has no registry kind for %s (it takes ghcr.io, docker.io, registry.gitlab.com, *.pkg.dev and *.amazonaws.com)", r.imageHost)
	}
	id, ok, err := r.API.Credential(ctx, name)
	if err != nil {
		return "", err
	}
	if ok {
		if err := r.API.UpdateCredential(ctx, id, name, kind, r.Reg.User, r.Reg.Token); err != nil {
			return "", fmt.Errorf("updating Render's registry credential %s: %w", name, err)
		}
		fmt.Fprintf(r.Out, "registry credential %s updated\n", name)
		return id, nil
	}
	if id, err = r.API.CreateCredential(ctx, r.owner, name, kind, r.Reg.User, r.Reg.Token); err != nil {
		return "", fmt.Errorf("creating Render's registry credential %s: %w", name, err)
	}
	fmt.Fprintf(r.Out, "registry credential %s created (%s)\n", name, kind)
	return id, nil
}

// Start rolls build id at Render: the service's creation when it does not exist yet (R2: its own
// deploy is of <image>:<id>, after the migrations), else the environment and scale brought in
// line (prepare) and a deploy of <image>:<id>; then it waits
// until Render reports the deploy live. A deploy that fails is an error carrying its last 50 log
// lines; Render keeps the previous build serving.
func (r *Render) Start(ctx context.Context, id string) (int, error) {
	var dep string
	if !r.have {
		cred, err := r.credential(ctx)
		if err != nil {
			return 0, err
		}
		env, _, err := r.mergedEnv(ctx)
		if err != nil {
			return 0, err
		}
		svc, d, err := r.API.CreateService(ctx, RenderCreate{OwnerID: r.owner, Name: r.Service, Region: r.Region, Plan: r.Plan, ImagePath: r.ref(id),
			CredentialID: cred, Instances: r.Instances, Env: env})
		if err != nil {
			return 0, fmt.Errorf("creating the Render service %s: %w", r.Service, err)
		}
		if d == "" {
			return 0, fmt.Errorf("Render created the service %s but named no deploy of it: check it in the dashboard and deploy again", r.Service)
		}
		r.svc, r.have, r.env, dep = svc, true, env, d
		fmt.Fprintf(r.Out, "Render service %s created (%s)\n", r.Service, r.Address())
	} else {
		if err := r.prepare(ctx, id); err != nil {
			return 0, err
		}
		d, err := r.API.CreateDeploy(ctx, r.svc.ID, r.ref(id))
		if err != nil {
			return 0, fmt.Errorf("deploying %s at Render: %w", r.ref(id), err)
		}
		if d.ID == "" {
			return 0, fmt.Errorf("Render named no deploy of %s", r.ref(id))
		}
		dep = d.ID
	}
	return RenderPort, r.await(ctx, dep)
}

// rollTimeout is how long Render may take to roll a deploy: RollTimeout, which the command sets
// from ALDER_DEPLOY_HEALTH_TIMEOUT (the one place that variable is read, for every target), 10
// minutes by default.
func (r *Render) rollTimeout() time.Duration {
	if r.RollTimeout > 0 {
		return r.RollTimeout
	}
	return 10 * time.Minute
}

func instances(n int) string {
	if n == 1 {
		return "1 instance"
	}
	return fmt.Sprintf("%d instances", n)
}

// await polls deploy dep until it is live, fails, or the roll timeout passes (it is then
// cancelled).
func (r *Render) await(ctx context.Context, dep string) error {
	deadline := time.Now().Add(r.rollTimeout())
	for {
		d, err := r.API.Deploy(ctx, r.svc.ID, dep)
		if err != nil {
			return err
		}
		switch {
		case d.Status == "live":
			fmt.Fprintf(r.Out, "Render deploy %s live (%s)\n", dep, instances(r.Instances))
			return nil
		case d.Failed():
			return fmt.Errorf("Render deploy %s %s%s", dep, d.Status, r.logTail(ctx))
		case time.Now().After(deadline):
			cancelled := "it was cancelled"
			if err := r.API.CancelDeploy(ctx, r.svc.ID, dep); err != nil {
				cancelled = "cancelling it failed (" + err.Error() + "): cancel it in Render's dashboard"
			}
			return fmt.Errorf("Render deploy %s is still %s after %s (ALDER_DEPLOY_HEALTH_TIMEOUT): %s%s", dep, d.Status, r.rollTimeout(), cancelled, r.logTail(ctx))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(renderPoll):
		}
	}
}

// logTail is the service's last 50 log lines, for an error.
func (r *Render) logTail(ctx context.Context) string {
	logs, err := r.API.Tail(ctx, r.owner, r.svc.ID, 50)
	if err != nil {
		return "; its log could not be read: " + err.Error()
	}
	if len(logs) == 0 {
		return "; its log is empty"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "; its log (the last %d lines):", len(logs))
	for _, l := range logs {
		b.WriteString("\n" + logLine(l))
	}
	return b.String()
}

func logLine(l RenderLog) string { return "[" + l.Instance + "] " + l.Message }

// Health waits until `instances` consecutive answers of GET <address>/_alder/health carry build
// id, so every instance has rolled; the error carries the last answer.
func (r *Render) Health(ctx context.Context, id string, port int, timeout time.Duration) error {
	url := r.Address() + renderHealthPath
	deadline := time.Now().Add(timeout)
	ok, last := 0, ""
	for {
		build, answer := r.healthOnce(ctx, url)
		if build == id {
			ok++
			if ok >= r.Instances {
				fmt.Fprintf(r.Out, "health: %d/%d instances serve %s\n", ok, r.Instances, id)
				return nil
			}
			continue
		}
		ok, last = 0, answer
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not answer with build %s on %s within %s; the last answer: %s", url, id, instances(r.Instances), timeout, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(renderPoll):
		}
	}
}

// healthOnce is one health answer's build ("" for none) and a description of it.
func (r *Render) healthOnce(ctx context.Context, url string) (string, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err.Error()
	}
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return "", err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var h struct{ Build, DB string }
	answer := fmt.Sprintf("%d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &h) != nil {
		return "", answer
	}
	return h.Build, answer
}

// Switch records build id current: Render switched the traffic already.
func (r *Render) Switch(ctx context.Context, id string, port int) (int, error) {
	st, err := r.store(ctx)
	if err == nil && st == nil {
		err = fmt.Errorf("Turso has no database %s", r.DB.SharedName())
	}
	if err != nil {
		return 0, err
	}
	if err := db.EnsureDeployTables(ctx, st); err != nil {
		return 0, err
	}
	return RenderPort, db.SetCurrent(ctx, st, id)
}

// Stop does nothing: Render stops the old instances itself after the roll.
func (r *Render) Stop(context.Context, string, time.Duration) error { return nil }

// Record writes build id's record into its row, with its schema.json, hash and migrations.
func (r *Render) Record(ctx context.Context, id string, rec []byte) error {
	var serr error
	err := r.putRow(ctx, id, func(row *db.DeployRow) {
		row.Record = string(rec)
		serr = r.withSchema(ctx, id, row)
	})
	if err != nil {
		return err
	}
	return serr
}

// Prune removes the rows beyond the keep newest, never one in protect. The images stay in the
// registry.
func (r *Render) Prune(ctx context.Context, keep int, protect []string) ([]string, error) {
	rows, _, st, err := r.rows(ctx)
	if err != nil || st == nil {
		return nil, err
	}
	var ids []string
	for i, row := range rows {
		if i >= keep && !slices.Contains(protect, row.ID) {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, db.DeleteDeployRows(ctx, st, ids)
}

// Info is Turso's storage size and point-in-time recovery, and the service's URL, instances
// and live deploy (Info.Lines).
func (r *Render) Info(ctx context.Context) (Info, error) {
	n, have, err := r.DB.StorageBytes(ctx)
	if err != nil {
		return Info{}, err
	}
	in := Info{HaveDB: have, DBBytes: n}
	if have {
		in.LastBackup, in.BackupFrom = time.Now(), "continuous: Turso point-in-time recovery of the database "+r.DB.SharedName()+" ("+r.DB.Describe()+"; `alder restore "+r.Env+" --to <time>`)"
	}
	if !r.have {
		in.Lines = append(in.Lines, "service  "+r.Address()+" (expected: not created yet)")
		return in, nil
	}
	line := "service  " + r.Address() + ", " + instances(r.svc.NumInstances)
	d, _, err := r.live(ctx)
	if err != nil {
		return Info{}, err
	}
	if d.ID != "" {
		since := d.FinishedAt
		if since.IsZero() {
			since = d.CreatedAt
		}
		line += ", live deploy " + d.ID + " (" + tagOf(d.ImageURL) + ", " + time.Since(since).Truncate(time.Second).String() + " ago)"
	} else {
		line += ", no live deploy"
	}
	in.Lines = append(in.Lines, line)
	return in, nil
}

// Logs writes the service's last 100 lines ("[instance] message"); with follow, the lines of
// Render's log stream as they come, until ctx ends.
func (r *Render) Logs(ctx context.Context, follow bool, w io.Writer) error {
	if !r.have {
		// `alder logs` has not connected (status and the deploys do): look for the service
		if _, err := r.Connect(ctx); err != nil {
			return err
		}
	}
	if !r.have {
		return refuse("", "the service %s does not exist at Render yet: deploy first", r.Service)
	}
	if !follow {
		logs, err := r.API.Tail(ctx, r.owner, r.svc.ID, 100)
		if err != nil {
			return err
		}
		for _, l := range logs {
			fmt.Fprintln(w, logLine(l))
		}
		return nil
	}
	c, err := DialWS(ctx, r.API.LogsURL(r.owner, r.svc.ID), r.API.AuthHeader())
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("Render's log stream: %s", r.API.redact(err.Error()))
	}
	defer c.Close()
	for {
		msg, err := c.Read()
		if ctx.Err() != nil || errors.Is(err, io.EOF) {
			return nil // ctx ended (the read then fails on the closed connection), or Render closed
		}
		if err != nil {
			return fmt.Errorf("Render's log stream: %w", err)
		}
		var f struct {
			Message string
			Labels  []struct{ Name, Value string }
		}
		if json.Unmarshal([]byte(msg), &f) != nil || f.Message == "" {
			fmt.Fprintln(w, msg)
			continue
		}
		l := RenderLog{Message: f.Message}
		for _, lb := range f.Labels {
			if lb.Name == "instance" {
				l.Instance = lb.Value
			}
		}
		fmt.Fprintln(w, logLine(l))
	}
}

// Restore refuses a file: the data is at Turso, restored to a point in time.
func (r *Render) Restore(ctx context.Context, file string) (string, error) {
	return "", refuse("", "the database of %s is at Turso: it is restored to a point in time (`alder restore <env> --to <time>`), not from a file", r.Describe())
}

// RestoreBackup is BackupRestorer: the shared database as it was at the point in time build src's
// row records (before src's migrations) is copied (Turso's point-in-time recovery) and made the
// service's (one env-var PUT of ALDER_DB and ALDER_TURSO_DB); the builds rows and this run's lock
// move to the copy. It starts nothing: the engine's Start of build to rolls the service onto it.
// Undo puts the replaced database's variables back.
func (r *Render) RestoreBackup(ctx context.Context, to, src string) (Restored, error) {
	if !r.have {
		return Restored{}, refuse("", "the service %s does not exist at Render", r.Service)
	}
	rows, _, st, err := r.rows(ctx)
	if err != nil {
		return Restored{}, err
	}
	row, _ := rowOf(rows, src)
	p, ok := parsePitr(row.Pitr)
	if st == nil || !ok {
		return Restored{}, refuse("", "build %s's row records no point in time: none was taken before its migrations", src)
	}
	old := r.DB.SharedName()
	cp, err := r.DB.CopyAt(ctx, p.db, p.at)
	if err != nil {
		return Restored{}, err
	}
	if err := r.adopt(ctx, st, rows, cp.Name); err != nil {
		return Restored{}, fmt.Errorf("preparing the copy %s: %w", cp.Name, err)
	}
	prev := r.env
	env, _, _ := overlay(prev, map[string]string{"ALDER_DB": "libsql://" + cp.Hostname, db.EnvTursoDB: cp.Name})
	if err := r.API.PutEnvVars(ctx, r.svc.ID, env); err != nil {
		return Restored{}, fmt.Errorf("pointing the service %s at the copy %s: %w", r.Service, cp.Name, err)
	}
	r.env = env
	r.DB.SetShared(cp.Name)
	undo := func(ctx context.Context) error {
		if err := r.API.PutEnvVars(ctx, r.svc.ID, prev); err != nil {
			return err
		}
		r.env = prev
		r.DB.SetShared(old)
		return nil
	}
	return Restored{From: "Turso point in time " + p.at.UTC().Format(time.RFC3339) + " of " + p.db, Taken: p.at, Kept: old, Undo: undo}, nil
}

// adopt prepares the copy name to be the app's database: the builds rows and the current
// pointer of the database it replaces (from) are written into it (the copy holds them as they
// were at its point in time), and this run's deploy lock is taken in it, any lock row it was
// copied with dropped (the copy is new: no run holds it).
func (r *Render) adopt(ctx context.Context, from *db.SQLite, rows []db.DeployRow, name string) error {
	cp, err := r.DB.api().Open(ctx, name, false)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			cp.Close()
		}
	}()
	if err := db.EnsureDeployTables(ctx, cp); err != nil {
		return err
	}
	for _, row := range rows {
		if err := db.PutDeployRow(ctx, cp, row); err != nil {
			return err
		}
	}
	cur, err := db.CurrentBuild(ctx, from)
	if err != nil {
		return err
	}
	if cur != "" {
		if err := db.SetCurrent(ctx, cp, cur); err != nil {
			return err
		}
	}
	if holder := r.DB.holder; holder != "" && r.DB.lock != nil {
		db.ExecRaw(ctx, cp, `DELETE FROM "_alder_lock"`) // absent before any lock: nothing to drop
		held, err := db.TakeLock(ctx, cp, holder, 2*time.Hour, r.DB.Now())
		if err != nil {
			return err
		}
		if held != "" {
			return fmt.Errorf("its deploy lock is held by %s", held)
		}
		r.unlocks = append(r.unlocks, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			db.ReleaseLock(ctx, cp, holder)
			cp.Close()
		})
		keep = true
	}
	return nil
}

// UploadLabel is what the transcript's `upload` line names: the image, not a box directory.
func (r *Render) UploadLabel(id string) string { return "image " + r.ref(id) }

// roll starts build id again at Render (a deploy of its image) and waits until every instance
// serves it: after a restore, so no instance keeps the database it had open.
func (r *Render) roll(ctx context.Context, id string) error {
	port, err := r.Start(ctx, id)
	if err != nil {
		return err
	}
	return r.Health(ctx, id, port, r.rollTimeout())
}

// RestorePointInTime is `alder restore <env> --to <time>`: a copy of the shared database as it
// was then (Turso's point-in-time recovery) becomes the service's database (one env-var PUT of
// ALDER_DB and ALDER_TURSO_DB; the builds rows and this run's lock move to the copy), and the
// current build is rolled onto it, health checked. A build that refuses the copy brings the
// replaced database back. It returns where the replaced one is kept.
func (r *Render) RestorePointInTime(ctx context.Context, at time.Time, w io.Writer) (string, error) {
	if !r.have {
		return "", refuse("", "the service %s does not exist at Render yet: deploy first", r.Service)
	}
	rows, _, st, err := r.rows(ctx)
	if err != nil {
		return "", err
	}
	if st == nil {
		return "", refuse("", "Turso has no database %s: there is nothing to restore", r.DB.SharedName())
	}
	old := r.DB.SharedName()
	cp, err := r.DB.CopyAt(ctx, old, at)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "copied %s as it was at %s → %s\n", old, at.UTC().Format(time.RFC3339), cp.Name)
	if err := r.adopt(ctx, st, rows, cp.Name); err != nil {
		return "", fmt.Errorf("preparing the copy %s: %w", cp.Name, err)
	}
	prev := r.env
	env, _, _ := overlay(prev, map[string]string{"ALDER_DB": "libsql://" + cp.Hostname, db.EnvTursoDB: cp.Name})
	if err := r.API.PutEnvVars(ctx, r.svc.ID, env); err != nil {
		return "", fmt.Errorf("pointing the service %s at the copy %s: %w", r.Service, cp.Name, err)
	}
	r.env = env
	r.DB.SetShared(cp.Name)
	kept := "the database " + old + " at Turso"
	cur, err := r.Current(ctx)
	if err != nil {
		return kept, err
	}
	if cur == "" {
		return kept, nil
	}
	if err := r.roll(ctx, cur); err != nil {
		if uerr := r.API.PutEnvVars(ctx, r.svc.ID, prev); uerr != nil {
			return kept, refuse(err.Error(), "build %s did not serve on the copy %s, and putting the database %s back failed (%v): set ALDER_DB and ALDER_TURSO_DB of the service %s back to %s by hand and run `alder deploy`", cur, cp.Name, old, uerr, r.Service, old)
		}
		r.env = prev
		r.DB.SetShared(old)
		if rerr := r.roll(ctx, cur); rerr != nil {
			return kept, refuse(err.Error(), "build %s did not serve on the copy %s: the database %s is the service's again, but build %s did not roll onto it (%v): run `alder deploy`", cur, cp.Name, old, cur, rerr)
		}
		return kept, refuse(err.Error(), "build %s did not serve on the copy %s (kept at Turso): the database %s is the service's again and build %s serves", cur, cp.Name, old, cur)
	}
	return kept, nil
}

// RestoreSilo is `alder restore <env> --silo key:value --to <time>`: the silo's database put
// back as it was (Turso.RestoreChild), then the current build rolled again so no instance keeps
// what it read of the old one. file is unused (a silo is at Turso). It returns where the replaced
// database is kept.
func (r *Render) RestoreSilo(ctx context.Context, key, value, file string, at time.Time, w io.Writer) (string, error) {
	if _, err := db.SiloPath("/", key, value); err != nil {
		return "", refuse("", "%s:%s is not a silo (write --silo <key>:<id>)", key, value)
	}
	if !r.have {
		return "", refuse("", "the service %s does not exist at Render yet: deploy first", r.Service)
	}
	cur, err := r.Current(ctx)
	if err != nil {
		return "", err
	}
	kept, err := r.DB.RestoreChild(ctx, key, value, at, w)
	if err != nil {
		return kept, err
	}
	kept = "the database " + kept + " at Turso"
	if cur == "" {
		return kept, nil
	}
	if err := r.roll(ctx, cur); err != nil {
		return kept, refuse(err.Error(), "the silo %s:%s was restored, but build %s did not roll again: run `alder deploy` (the replaced database is %s)", key, value, cur, kept)
	}
	return kept, nil
}
