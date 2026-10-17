package db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// Silos on Turso (chapter 16, "Turso"): with ALDER_DB=libsql://<host>, StoresConfig.Dir is
// that URL and every silo is a database of its own at Turso:
//
//	<app>                  the shared database (ALDER_DB)
//	<app>-<key>-<name>     one silo (ChildName; <name> is its silo file's name without .db),
//	                       created by the runtime through the platform API on first use
//
// A silo database on Turso is checked and migrated exactly as a silo file is (check): created
// at this binary's schema when new, migrated on open when behind by migrations that drop no
// data, `pending-data-loss` (maintenance) when one would, `ahead` when a newer build migrated
// it. `alder migrate silos <env>` migrates them all at once (the children listed through the
// API). Turso's schema databases (one schema shared by many databases) are not used: Turso
// marks them deprecated for new organizations.

// openChild opens (creating it at Turso when it does not exist) the database of silo ss's
// value, and checks it as a file is checked.
func (s *Stores) openChild(ctx context.Context, d *siloDB, ss *siloSchema, value string, fail func(SiloState, string) *siloDB) *siloDB {
	st, err := s.child(ctx, ss.key, value, true, false)
	if err != nil {
		return fail(SiloFailed, err.Error())
	}
	d.store = st
	state, detail, err := s.check(ctx, st, ss, s.cfg.AllowDataLoss, true)
	if err != nil {
		return fail(SiloFailed, err.Error())
	}
	if state != SiloOK {
		return fail(state, detail)
	}
	if detail != "" {
		s.logf("info", fmt.Sprintf("silo %s/%s: %s", ss.key, value, detail))
	}
	st.register(ss.descs)
	r, err := s.child(ctx, ss.key, value, false, true)
	if err != nil {
		return fail(SiloFailed, err.Error())
	}
	r.register(ss.descs)
	d.reader, d.state = r, SiloOK
	return d
}

// child is the Store of silo key's value on Turso (reader: a read-only one). With create, a
// child that does not exist is created (a database of its own, ChildName); one another process
// created meanwhile ("already exists") is opened.
func (s *Stores) child(ctx context.Context, key, value string, create, reader bool) (*SQLite, error) {
	name := ChildName(s.app, key, value)
	if err := TursoName(name); err != nil {
		return nil, err
	}
	_, ok, err := s.remote.Database(ctx, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		if !create {
			return nil, fmt.Errorf("db: Turso has no database %s", name)
		}
		switch _, err := s.remote.Create(ctx, CreateOptions{Name: name}); {
		case IsTursoExists(err): // another process (an instance, a deploy) created it first
		case err != nil:
			return nil, fmt.Errorf("db: creating silo %s/%s's database %s: %w", key, value, name, err)
		default:
			s.logf("info", fmt.Sprintf("silo %s/%s: created its database %s", key, value, name))
		}
	}
	return s.remote.Open(ctx, name, reader)
}

func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) > n {
		return strings.Join(f[:n], " ") + " …"
	}
	return s
}

// ---- the binary's maintenance tasks over every silo: files or children

// siloEntry is one silo database: a file (path) or a child on Turso (name).
type siloEntry struct {
	key, value, path, name string
}

// entries are the silo databases of every key: the files under Dir, or the databases at
// Turso named as a key's children (ChildName: <app>-<key>-<name>).
func (s *Stores) entries(ctx context.Context) ([]siloEntry, error) {
	var out []siloEntry
	if s.remote == nil {
		files, err := SiloFiles(s.cfg.Dir, s.cfg.Silos)
		for _, f := range files {
			out = append(out, siloEntry{key: f.Key, value: f.Value, path: f.Path})
		}
		return out, err
	}
	for _, si := range s.cfg.Silos {
		kids, err := s.remote.Children(ctx, s.app, si.Key)
		if err != nil {
			return nil, err
		}
		for _, k := range kids {
			if v, ok := ChildValue(s.app, si.Key, k.Name); ok {
				out = append(out, siloEntry{key: si.Key, value: v, name: k.Name})
			}
		}
	}
	return out, nil
}

// openEntry opens silo database e (reader: read-only).
func (s *Stores) openEntry(ctx context.Context, e siloEntry, reader bool) (*SQLite, error) {
	if e.name != "" {
		return s.remote.Open(ctx, e.name, reader)
	}
	if reader {
		return OpenSQLiteReader(e.path)
	}
	return OpenSQLite(e.path)
}

// state is silo database st's state for --silo-status and --migrate-silos (apply: brought to
// this binary's schema where it can be).
func (s *Stores) state(ctx context.Context, st *SQLite, e siloEntry, allowDataLoss, apply bool) (SiloState, string, error) {
	return s.check(ctx, st, s.silos[e.key], allowDataLoss, apply)
}

// orphans is a function telling whether silo key/value is orphaned (its key row is gone from
// the shared database), or nil when that cannot be told (no shared database named, no Ref).
func (s *Stores) orphans(ctx context.Context) (func(key, value string) bool, func(), error) {
	if s.cfg.Shared == "" || s.cfg.Shared == ":memory:" {
		return nil, func() {}, nil
	}
	if !IsRemote(s.cfg.Shared) {
		if _, err := os.Stat(s.cfg.Shared); err != nil {
			return nil, func() {}, nil
		}
	}
	shared, err := OpenSQLiteReader(s.cfg.Shared)
	if err != nil {
		return nil, func() {}, err
	}
	byName := map[string]TableDesc{}
	for _, d := range s.cfg.Tables {
		byName[d.Name] = d
	}
	refs := map[string]TableDesc{}
	for _, si := range s.cfg.Silos {
		if d, ok := byName[si.Ref]; ok {
			refs[si.Key] = d
		}
	}
	return func(key, value string) bool {
		d, ok := refs[key]
		if !ok {
			return false
		}
		var n int
		err := shared.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quote(SQLName(d.Name))+" WHERE "+quote(d.ID)+" = ?", value).Scan(&n)
		return err == nil && n == 0
	}, func() { shared.Close() }, nil
}

func orphanDetail(s *Stores, key string) string {
	ref := ""
	for _, si := range s.cfg.Silos {
		if si.Key == key {
			ref = shortName(si.Ref)
		}
	}
	return fmt.Sprintf("no %s row has this id any more (it was deleted): nothing reaches the silo; `alder silos prune <env>` removes its database", orNoneText(ref, "key"))
}

func orNoneText(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// stateLine is "<what>: ok", "<what>: <detail>" for an ok with a detail, or "<what>: <state>:
// <detail>".
func stateLine(what string, state SiloState, detail string) string {
	switch {
	case state == SiloOK && detail != "":
		return what + ": " + detail
	case detail != "":
		return what + ": " + string(state) + ": " + detail
	}
	return what + ": " + string(state)
}

// VerifySilos tries the pending migrations on the n largest silo databases of each key, each in
// a transaction it rolls back (it changes nothing), with the open-time check's full schema
// check: that the database's row names a migration of this build, that every statement runs,
// that no row is left pointing at a deleted one, and that the tables come out as this binary's.
// The largest are by storage size: the API's usage on Turso (when it gives none, the first n by
// name), the file's size otherwise. It writes a line per database and returns how many fail. `alder deploy` runs it on
// Turso before migrating anything (the binary's --verify-silos N).
func VerifySilos(ctx context.Context, cfg StoresConfig, n int, out io.Writer) (int, error) {
	s, err := newStores(nil, cfg)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		n = 5
	}
	es, err := s.entries(ctx)
	if err != nil {
		return 0, err
	}
	sizes := map[siloEntry]int64{}
	for _, e := range es {
		if e.name != "" {
			// a size the API does not give is 0: the first n by name are tried then
			sizes[e], _ = s.remote.StorageBytes(ctx, e.name)
		} else if fi, err := os.Stat(e.path); err == nil {
			sizes[e] = fi.Size()
		}
	}
	bad := 0
	for _, si := range s.cfg.Silos {
		ss := s.silos[si.Key]
		var mine []siloEntry
		for _, e := range es {
			if e.key == si.Key {
				mine = append(mine, e)
			}
		}
		slices.SortStableFunc(mine, func(a, b siloEntry) int {
			switch {
			case sizes[a] > sizes[b]:
				return -1
			case sizes[a] < sizes[b]:
				return 1
			}
			return strings.Compare(a.value, b.value)
		})
		if len(mine) > n {
			mine = mine[:n]
		}
		for _, e := range mine {
			what := e.key + "/" + e.value
			if e.name != "" {
				what += " (database " + e.name + ")"
			}
			st, err := s.openEntry(ctx, e, false)
			if err != nil {
				fmt.Fprintf(out, "%s: FAIL: %v\n", what, err)
				bad++
				continue
			}
			p, err := s.inspect(ctx, st.db, ss, true)
			if err != nil || p.state == SiloFailed || p.state == SiloAhead {
				st.Close()
				detail := p.detail
				if err != nil {
					detail = err.Error()
				}
				fmt.Fprintf(out, "%s: FAIL: %s: %s\n", what, p.state, detail)
				bad++
				continue
			}
			if p.fresh {
				st.Close()
				fmt.Fprintf(out, "%s: ok (new: made at this build's schema when it is opened)\n", what)
				continue
			}
			err = s.dryRun(ctx, st, ss, p.pending)
			st.Close()
			if err != nil {
				fmt.Fprintf(out, "%s: FAIL: %v\n", what, err)
				bad++
				continue
			}
			fmt.Fprintf(out, "%s: ok (%s)\n", what, sizeText(sizes[e]))
		}
	}
	return bad, nil
}

// dryRun applies ms's statements on silo ss's tables to st inside a transaction that it rolls
// back, and checks the result as the open-time check does.
func (s *Stores) dryRun(ctx context.Context, st *SQLite, ss *siloSchema, ms []Migration) error {
	conn, err := st.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	restore, err := foreignKeysOff(ctx, conn)
	if err != nil {
		return err
	}
	defer restore()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, m := range ms {
		for _, q := range ss.statements(m) {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("migration %s fails on it: %s: %v", m.Name, firstWords(q, 6), err)
			}
		}
	}
	if child, err := foreignKeyViolation(ctx, tx); err != nil {
		return err
	} else if child != "" {
		return fmt.Errorf("after the migrations rows in %s refer to rows they delete", child)
	}
	if _, live, err := liveSchema(ctx, tx); err != nil {
		return err
	} else if live != ss.hash {
		return fmt.Errorf("after the migrations its tables (%.12s) are not this build's (%.12s)", live, ss.hash)
	}
	return nil
}

func sizeText(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f kB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// PruneSilos removes the databases of the silos named ("org:<id>"), each only when it is still
// orphaned (its key row gone from the shared database: --silo-status lists it so): a file with
// its -wal, -shm and backup records, or a child database on Turso. It writes a line each and
// returns how many it kept. `alder silos prune <env>` runs it (the binary's --prune-silos) after
// asking.
func PruneSilos(ctx context.Context, cfg StoresConfig, names []string, out io.Writer) (int, error) {
	s, err := newStores(nil, cfg)
	if err != nil {
		return 0, err
	}
	orphaned, done, err := s.orphans(ctx)
	if err != nil {
		return 0, err
	}
	defer done()
	if orphaned == nil {
		return 0, errors.New("db: --prune-silos needs the shared database (ALDER_DB) and the silos' key tables")
	}
	kept := 0
	for _, n := range names {
		key, value, ok := strings.Cut(n, ":")
		if !ok || s.silos[key] == nil || safeName(value) != nil {
			fmt.Fprintf(out, "%s: not a silo (write <key>:<id>)\n", n)
			kept++
			continue
		}
		what := key + "/" + value
		if !orphaned(key, value) {
			fmt.Fprintf(out, "%s: kept: its key row exists (it is not orphaned)\n", what)
			kept++
			continue
		}
		if s.remote != nil {
			if err := s.remote.Delete(ctx, ChildName(s.app, key, value)); err != nil && !IsTursoNotFound(err) {
				fmt.Fprintf(out, "%s: kept: %v\n", what, err)
				kept++
				continue
			}
		} else {
			p, err := SiloPath(s.cfg.Dir, key, value)
			if err == nil {
				err = os.Remove(p)
				for _, x := range []string{"-wal", "-shm", "-backup.json", "-backup.lock"} {
					os.Remove(p + x)
				}
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(out, "%s: kept: %v\n", what, err)
				kept++
				continue
			}
		}
		fmt.Fprintf(out, "%s: removed\n", what)
	}
	return kept, nil
}

// ---- the deploy lock of a database on Turso

const createLockTable = `CREATE TABLE IF NOT EXISTS "_alder_lock" ("key" TEXT NOT NULL PRIMARY KEY, "holder" TEXT NOT NULL, "expires" TEXT NOT NULL)`

// TakeLock takes the deploy lock of the database st (a row of _alder_lock: there is no file to
// flock on Turso) for holder until now+ttl. It returns "" when taken, else the holder that has
// it (and until when). A lock past its expiry is taken over.
func TakeLock(ctx context.Context, st *SQLite, holder string, ttl time.Duration, now time.Time) (string, error) {
	if _, err := st.db.ExecContext(ctx, createLockTable); err != nil {
		return "", err
	}
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var who, until string
	err = tx.QueryRowContext(ctx, `SELECT "holder", "expires" FROM "_alder_lock" WHERE "key" = 'deploy'`).Scan(&who, &until)
	if err == nil && who != holder {
		if t, perr := time.Parse(time.RFC3339, until); perr == nil && t.After(now) {
			return who + " until " + until, nil
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO "_alder_lock" ("key", "holder", "expires") VALUES ('deploy', ?, ?) ON CONFLICT ("key") DO UPDATE SET "holder" = excluded."holder", "expires" = excluded."expires"`,
		holder, now.Add(ttl).UTC().Format(time.RFC3339)); err != nil {
		return "", err
	}
	return "", tx.Commit()
}

// ReleaseLock gives up holder's deploy lock of st.
func ReleaseLock(ctx context.Context, st *SQLite, holder string) error {
	_, err := st.db.ExecContext(ctx, `DELETE FROM "_alder_lock" WHERE "key" = 'deploy' AND "holder" = ?`, holder)
	return err
}

// HasAppTables reports whether st holds the app's tables already (a database a deploy migrated:
// _alder_meta exists).
func HasAppTables(ctx context.Context, st *SQLite) (bool, error) {
	return tableExists(ctx, st.db, "_alder_meta")
}
