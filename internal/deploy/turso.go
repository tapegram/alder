package deploy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"alder/internal/ir"
	"alder/rt/db"
)

// Turso is an Ssh environment's `database: Turso "org" { group?, app, sample? }` (chapter 16,
// "Turso"): the app's data on Turso instead of a file on the box. The binary still runs on the
// Ssh host; what changes is where its database is and how a deploy changes it:
//
//   - the shared database is named after `app` (or, after a point-in-time restore, the copy
//     that replaced it: <path>/db/turso.env on the box names the current one, and the build's
//     run script reads ALDER_DB=libsql://<its host> and ALDER_TURSO_DB from it);
//   - each silo key has a schema database, <app>-<key>-schema, and each silo a child of it,
//     <app>-<key>-<value> (rt/db), created by the running app on first use;
//   - the migrations run from the deploying machine, over HTTP, with the build's executable for
//     this machine (Local): first --verify-silos <sample> (the plan tried on the largest
//     children and rolled back: a failure refuses the deploy, naming the child, before anything
//     is migrated), then --migrate-only (the shared database, the DROPS DATA rule as for a
//     file), then --migrate-silos --schema-only (each schema database; Turso passes the change
//     on to the children);
//   - the backup is Turso's point-in-time recovery: before migrating, the deploy records the
//     time in builds/<id>/backup.pitr, and `rollback --restore-backup` creates a copy of the
//     shared database as it was then and makes it the app's (the replaced one is kept);
//   - the deploy lock is the Ssh target's, and a row of the shared database's _alder_lock;
//   - there is no S3 stream: `backup` does not apply to this environment.
//
// Its tokens: TURSO_API_TOKEN in the deploying machine's environment (the platform API, the
// migrations) and in the box's environment file (the running app creates children and mints
// their tokens with it).
type Turso struct {
	Org, Group, App string
	Sample          int
	// Silos are the app's silo keys (its `silo` declarations).
	Silos []string
	// Local is the path of build id's executable for this machine, which runs the migrations
	// here (cmd/alder builds it next to the artifact when the target's platform is another).
	Local func(ctx context.Context, id string) (string, error)
	// API is the platform API client (made from the environment: TURSO_API_TOKEN,
	// ALDER_TURSO_API); tests set it.
	API *db.Turso
	Now func() time.Time

	shared string // the shared database's name (db/turso.env, else App)
	lock   *db.SQLite
	holder string
}

// DefaultSample is how many of the largest silo databases a Turso deploy tries the migrations
// on first.
const DefaultSample = 5

// NewTurso is the Turso database of environment env (d from the declaration).
func NewTurso(d *ir.Database) *Turso {
	t := &Turso{Org: d.Org, Group: d.Group, App: d.App, Sample: d.Sample, Now: time.Now, shared: d.App}
	if t.Group == "" {
		t.Group = "default"
	}
	if t.Sample <= 0 {
		t.Sample = DefaultSample
	}
	return t
}

// api is the platform client, made from this machine's environment on first use.
func (t *Turso) api() *db.Turso {
	if t.API == nil {
		c := db.TursoConfigFromEnv()
		c.Org, c.Group, c.App = t.Org, t.Group, t.App
		t.API = db.NewTurso(c)
	}
	return t.API
}

// Describe is "Turso acme/blog".
func (t *Turso) Describe() string { return "Turso " + t.Org + "/" + t.App }

// SharedName is the shared database's current name.
func (t *Turso) SharedName() string { return t.shared }

// setShared takes the shared database's name from the box's turso.env ("" keeps App).
func (t *Turso) setShared(name string) {
	if name != "" {
		t.shared = name
	}
}

// tokenSet refuses a deploy from a machine without TURSO_API_TOKEN.
func (t *Turso) tokenSet() error {
	if os.Getenv(db.EnvTursoToken) == "" && (t.API == nil || t.API.Config().Token == "") {
		return refuse("", "this machine's environment does not set %s: a deploy to %s runs the migrations from here, over Turso's API (create a token with `turso auth api-tokens mint alder` and export it)", db.EnvTursoToken, t.Describe())
	}
	return nil
}

// sharedDB is the shared database at Turso (found false before the first deploy).
func (t *Turso) sharedDB(ctx context.Context) (db.TursoDB, bool, error) {
	return t.api().Database(ctx, t.shared)
}

// HaveTables reports whether the shared database exists and holds the app's tables (a deploy
// migrated it).
func (t *Turso) HaveTables(ctx context.Context) (bool, error) {
	if err := t.tokenSet(); err != nil {
		return false, err
	}
	_, ok, err := t.sharedDB(ctx)
	if err != nil || !ok {
		return false, err
	}
	st, err := t.api().Open(ctx, t.shared, true)
	if err != nil {
		return false, err
	}
	defer st.Close()
	return db.HasAppTables(ctx, st)
}

// Preflight checks, before anything is uploaded, that this machine has the token, that the group
// exists, that every database name fits Turso's rules, and creates the shared database when it
// does not exist yet. It returns the shared database's host.
func (t *Turso) Preflight(ctx context.Context) (string, error) {
	if err := t.tokenSet(); err != nil {
		return "", err
	}
	names := []string{t.shared, t.App + "-r0000000000"}
	for _, k := range t.Silos {
		names = append(names, db.SchemaName(t.App, k), db.ChildName(t.App, k, strings.Repeat("x", 20)))
	}
	for _, n := range names {
		if err := db.TursoName(n); err != nil {
			return "", refuse("", "%v", strings.TrimPrefix(err.Error(), "db: "))
		}
	}
	ok, err := t.api().GroupExists(ctx, t.Group)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", refuse("", "Turso organization %s has no group %s: create it (`turso group create %s`) or name another in the declaration's `group`", t.Org, t.Group, t.Group)
	}
	d, found, err := t.sharedDB(ctx)
	if err != nil {
		return "", err
	}
	if !found {
		if d, err = t.api().Create(ctx, db.CreateOptions{Name: t.shared, Group: t.Group}); err != nil {
			return "", fmt.Errorf("creating the database %s at Turso: %w", t.shared, err)
		}
		if d.Hostname == "" {
			if d, _, err = t.sharedDB(ctx); err != nil {
				return "", err
			}
		}
	}
	if d.Hostname == "" {
		return "", fmt.Errorf("Turso does not give the database %s a host name", t.shared)
	}
	return d.Hostname, nil
}

// TursoEnv is the text of <path>/db/turso.env for the shared database name at host: what the
// run script reads (no secret: the token comes from the operator's environment file).
func TursoEnv(name, host string) string {
	return "ALDER_DB=libsql://" + host + "\nALDER_TURSO_DB=" + name + "\n"
}

// Lock is the deploy lock's Turso half: a row of the shared database (when it exists already;
// before the first deploy the Ssh target's lock is the only one). unlock releases it.
func (t *Turso) Lock(ctx context.Context, who string) (func(), error) {
	if err := t.tokenSet(); err != nil {
		return nil, err
	}
	_, ok, err := t.sharedDB(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return func() {}, nil
	}
	st, err := t.api().Open(ctx, t.shared, false)
	if err != nil {
		return nil, err
	}
	var b [4]byte
	rand.Read(b[:])
	holder := who + " (" + hex.EncodeToString(b[:]) + ")"
	held, err := db.TakeLock(ctx, st, holder, 2*time.Hour, t.Now())
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("taking the deploy lock in the database %s: %w", t.shared, err)
	}
	if held != "" {
		st.Close()
		return nil, refuse("", "another deploy, rollback or restore holds the database %s at Turso (its _alder_lock row: %s): wait for it to finish (a lock left by a killed run expires at that time)", t.shared, held)
	}
	t.lock, t.holder = st, holder
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db.ReleaseLock(ctx, st, holder)
		st.Close()
		t.lock = nil
	}, nil
}

// env is the environment the local executable runs the migrations with.
func (t *Turso) env(host string, allowDataLoss bool) []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "ALDER_DB", "ALDER_DEV", "ALDER_SECRET", "ALDER_ALLOW_DATA_LOSS", "ALDER_BACKUP_URL", db.EnvDBToken, db.EnvTursoOrg, db.EnvTursoGroup, db.EnvTursoApp, db.EnvTursoDB, db.EnvTursoAPI:
			continue
		}
		out = append(out, kv)
	}
	var b [24]byte
	rand.Read(b[:])
	c := t.api().Config()
	out = append(out, "ALDER_DB=libsql://"+host, db.EnvTursoOrg+"="+t.Org, db.EnvTursoGroup+"="+t.Group, db.EnvTursoApp+"="+t.App, db.EnvTursoDB+"="+t.shared,
		db.EnvTursoAPI+"="+c.API, db.EnvTursoToken+"="+c.Token,
		// --migrate-only checks that a session key is set; this run serves nothing, so any will do
		"ALDER_SECRET="+hex.EncodeToString(b[:]))
	if allowDataLoss {
		out = append(out, "ALDER_ALLOW_DATA_LOSS=1")
	}
	return out
}

// Migrate runs build id's migrations from this machine (see Turso): it returns what the
// executable printed; a sampled silo that fails the plan is a refusal naming it.
func (t *Turso) Migrate(ctx context.Context, id string, allowDataLoss bool) (string, error) {
	bin, err := t.Local(ctx, id)
	if err != nil {
		return "", err
	}
	d, ok, err := t.sharedDB(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("Turso has no database %s (the preflight creates it)", t.shared)
	}
	env := t.env(d.Hostname, allowDataLoss)
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		return out.String(), err
	}
	var all strings.Builder
	if len(t.Silos) > 0 {
		out, err := run("--verify-silos", strconv.Itoa(t.Sample))
		all.WriteString(prefixLines("verify ", out))
		if err != nil {
			var failed []string
			for _, l := range strings.Split(out, "\n") {
				if strings.Contains(l, ": FAIL: ") {
					failed = append(failed, l)
				}
			}
			return all.String(), refuse(strings.Join(failed, "\n"), "the migrations fail on %d of the largest silo databases they were tried on (each in a transaction, rolled back): nothing was migrated or switched, and the shared database is as it was", len(failed))
		}
	}
	out, err := run("--migrate-only")
	all.WriteString(out)
	if err != nil {
		return all.String(), errors.New("--migrate-only refused")
	}
	if len(t.Silos) > 0 {
		args := []string{"--migrate-silos", "--schema-only"}
		if allowDataLoss {
			args = append(args, "--allow-data-loss")
		}
		out, err := run(args...)
		all.WriteString(prefixLines("silos: ", strings.ReplaceAll(out, ": migrated ", ": applied ")))
		if err != nil {
			return all.String(), errors.New("--migrate-silos refused")
		}
	}
	return all.String(), nil
}

func prefixLines(p, s string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l != "" {
			b.WriteString(p + l + "\n")
		}
	}
	return b.String()
}

// CopyAt creates a copy of database name as it was at time at (Turso's point-in-time
// recovery), named <app>-r<unix seconds now>, and returns it.
func (t *Turso) CopyAt(ctx context.Context, name string, at time.Time) (db.TursoDB, error) {
	copyName := fmt.Sprintf("%s-r%d", t.App, t.Now().Unix())
	d, err := t.api().Create(ctx, db.CreateOptions{Name: copyName, Group: t.Group, Seed: &db.TursoSeed{Name: name, Timestamp: at}})
	if err != nil {
		return db.TursoDB{}, fmt.Errorf("copying the database %s as it was at %s (Turso's point-in-time recovery): %w", name, at.UTC().Format(time.RFC3339), err)
	}
	if d.Hostname == "" {
		if d, _, err = t.api().Database(ctx, copyName); err != nil {
			return db.TursoDB{}, err
		}
	}
	return d, nil
}

// RestoreChild puts silo key/value's database back as it was at time at: Turso cannot restore
// a database in place, so a copy at that time is made (seeded), the database as it is now is
// copied too (kept, returned), and the silo's database is created again under its schema
// database from the first copy, which is then deleted.
func (t *Turso) RestoreChild(ctx context.Context, key, value string, at time.Time, w io.Writer) (kept string, err error) {
	name := db.ChildName(t.App, key, value)
	if _, ok, err := t.api().Database(ctx, name); err != nil {
		return "", err
	} else if !ok {
		return "", refuse("", "Turso has no database %s for silo %s:%s (`alder status` lists the silos)", name, key, value)
	}
	stamp := t.Now().Unix()
	restored := fmt.Sprintf("%s-r%d", t.App, stamp)
	keptName := fmt.Sprintf("%s-k%d", t.App, stamp)
	if _, err := t.api().Create(ctx, db.CreateOptions{Name: restored, Group: t.Group, Seed: &db.TursoSeed{Name: name, Timestamp: at}}); err != nil {
		return "", fmt.Errorf("copying %s as it was at %s (Turso's point-in-time recovery): %w", name, at.UTC().Format(time.RFC3339), err)
	}
	fmt.Fprintf(w, "  copy     ok  %s as it was at %s → %s\n", name, at.UTC().Format(time.RFC3339), restored)
	if _, err := t.api().Create(ctx, db.CreateOptions{Name: keptName, Group: t.Group, Seed: &db.TursoSeed{Name: name}}); err != nil {
		t.api().Delete(ctx, restored)
		return "", fmt.Errorf("keeping %s as it is now: %w", name, err)
	}
	fmt.Fprintf(w, "  keep     ok  %s as it is now → %s\n", name, keptName)
	if err := t.api().Delete(ctx, name); err != nil {
		t.api().Delete(ctx, restored)
		return keptName, err
	}
	if _, err := t.api().Create(ctx, db.CreateOptions{Name: name, Group: t.Group, Schema: db.SchemaName(t.App, key), Seed: &db.TursoSeed{Name: restored}}); err != nil {
		// put the database as it was a moment ago back
		if _, uerr := t.api().Create(ctx, db.CreateOptions{Name: name, Group: t.Group, Schema: db.SchemaName(t.App, key), Seed: &db.TursoSeed{Name: keptName}}); uerr != nil {
			return keptName, fmt.Errorf("creating %s from the copy failed (%v), and so did putting it back from %s (%v): create it by hand from %s", name, err, keptName, uerr, keptName)
		}
		return keptName, fmt.Errorf("creating %s from the copy failed: %v; it was put back as it was", name, err)
	}
	fmt.Fprintf(w, "  restore  ok  %s from %s\n", name, restored)
	t.api().Delete(ctx, restored)
	return keptName, nil
}

// StorageBytes is the shared database's size.
func (t *Turso) StorageBytes(ctx context.Context) (int64, bool, error) {
	_, ok, err := t.sharedDB(ctx)
	if err != nil || !ok {
		return 0, false, err
	}
	n, err := t.api().StorageBytes(ctx, t.shared)
	return n, true, err
}
