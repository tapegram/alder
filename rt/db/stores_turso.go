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
// that URL and every silo is a child database of its key's schema database:
//
//	<app>                  the shared database (ALDER_DB)
//	<app>-<key>-schema     the schema database of silo key: its tables, no rows
//	<app>-<key>-<value>    one silo (ChildName), created by the runtime on first use as a
//	                       child of the schema database, so it has the silo's tables at once
//
// A schema database carries schema changes only: `alder deploy` migrates it (MigrateSilos with
// SchemaOnly, from the deploying machine) and Turso passes each change on to its children. So
// the runtime never migrates a child itself; it reads the child's `_alder_schema` row and its
// tables:
//
//   - the row names this binary's schema: ok;
//   - the row names a migration this binary does not have, after its last: ahead (maintenance,
//     the restore named), as for a file;
//   - otherwise, when the child's tables are this binary's silo tables (the schema database's
//     change has reached it, or it is new), the row is written and the silo is ok;
//   - otherwise it is migrating: the change has not reached it yet (maintenance; checked again
//     after `recheck`).
//
// The schema database's own state (the hash and migration of the binary that last migrated it)
// is in the shared database's `_alder_meta`, under "silo_schema:<key>", so the schema database
// itself holds nothing but DDL. A pending migration whose statements on the silo's tables copy
// or change rows (a table rebuild, a backfill) cannot reach the children that way: it is
// refused, with a sentence, before anything is migrated.

// siloSchemaMeta is the _alder_meta key of silo key's schema database's state.
func siloSchemaMeta(key string) string { return "silo_schema:" + key }

// openChild opens (creating it on Turso when it does not exist) the child database of silo
// ss's value, and checks it (inspectChild).
func (s *Stores) openChild(ctx context.Context, d *siloDB, ss *siloSchema, value string, fail func(SiloState, string) *siloDB) *siloDB {
	st, err := s.child(ctx, ss.key, value, true, false)
	if err != nil {
		return fail(SiloFailed, err.Error())
	}
	d.store = st
	state, detail, err := s.checkChild(ctx, st, ss, true)
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
// child that does not exist is created under the key's schema database.
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
		if _, err := s.remote.Create(ctx, CreateOptions{Name: name, Schema: SchemaName(s.app, key)}); err != nil {
			return nil, fmt.Errorf("db: creating silo %s/%s's database %s: %w", key, value, name, err)
		}
		s.logf("info", fmt.Sprintf("silo %s/%s: created its database %s", key, value, name))
	}
	return s.remote.Open(ctx, name, reader)
}

// checkChild is the open-time check of a child database (see above); with apply, a child whose
// tables are this binary's gets its row.
func (s *Stores) checkChild(ctx context.Context, st *SQLite, ss *siloSchema, apply bool) (SiloState, string, error) {
	hash, mig, ok, err := schemaRow(ctx, st.db)
	if err != nil {
		return "", "", err
	}
	if ok && hash == s.hash {
		return SiloOK, "", nil
	}
	if ok && mig != "" && mig > s.last && !slices.ContainsFunc(s.cfg.Migrations, func(m Migration) bool { return m.Name == mig }) {
		return SiloAhead, fmt.Sprintf("a newer build migrated it (to %s, which this build does not have): deploy that build again, or restore the silo to a time before it (`alder restore <env> --silo %s:<id> --to <time>`)", mig, ss.key), nil
	}
	_, live, err := liveSchema(ctx, st.db)
	if err != nil {
		return "", "", err
	}
	if live != ss.hash {
		return SiloMigrating, fmt.Sprintf("its tables (%.12s) are not this build's (%.12s) yet: the schema database's change has not reached it; it is served once Turso has passed it on", live, ss.hash), nil
	}
	exists, err := tableExists(ctx, st.db, "_alder_schema")
	if err != nil {
		return "", "", err
	}
	if !exists {
		return SiloMigrating, "it has no _alder_schema table yet: the schema database's change has not reached it", nil
	}
	if !apply {
		return SiloOK, "", nil
	}
	// the child holds rows only: its table exists (the schema database made it), so the row is
	// written without DDL
	_, err = st.db.ExecContext(ctx, `INSERT INTO "_alder_schema" ("key", "hash", "migration", "at") VALUES ('schema', ?, ?, ?) ON CONFLICT ("key") DO UPDATE SET "hash" = excluded."hash", "migration" = excluded."migration", "at" = excluded."at"`,
		s.hash, s.last, EncodeTime(time.Now()))
	if err != nil {
		return "", "", err
	}
	return SiloOK, "took this build's schema from the schema database", nil
}

// propagable refuses the pending migrations ms when a statement of one on silo ss's tables
// copies or changes rows: a schema database passes its children its schema changes only.
func (ss *siloSchema) propagable(ms []Migration) error {
	for _, m := range ms {
		for _, st := range ss.statements(m) {
			verb := strings.ToUpper(strings.Fields(st)[0])
			if verb == "INSERT" || verb == "UPDATE" || verb == "DELETE" {
				return fmt.Errorf("migration %s copies or changes rows of a table in silo %s (%s): on Turso the silo's schema database passes its children schema changes only, so their rows would not follow; make the change one that adds a nullable column, or adds or drops a table or an index (or keep that table out of the silo)", m.Name, ss.key, firstWords(st, 6))
			}
		}
	}
	return nil
}

func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) > n {
		return strings.Join(f[:n], " ") + " …"
	}
	return s
}

// schemaState is silo key's schema database's state, as the shared database records it (ok
// false: never migrated).
func schemaState(ctx context.Context, shared querier, key string) (hash, mig string, ok bool, err error) {
	if exists, err := tableExists(ctx, shared, "_alder_meta"); err != nil || !exists {
		return "", "", false, err
	}
	v, err := metaValue(ctx, shared, siloSchemaMeta(key))
	if err != nil || v == "" {
		return "", "", false, err
	}
	hash, mig, _ = strings.Cut(v, " ")
	return hash, mig, true, nil
}

// migrateSchemaDB creates silo ss's schema database when it does not exist and brings it to
// this binary's schema: the state recorded in the shared database, the pending migrations'
// statements on the silo's tables (schema changes only: propagable), DROPS DATA refused
// without allowDataLoss. It returns the state and a detail, as check does.
func (s *Stores) migrateSchemaDB(ctx context.Context, shared *SQLite, ss *siloSchema, allowDataLoss bool) (SiloState, string, error) {
	name := SchemaName(s.app, ss.key)
	if err := TursoName(name); err != nil {
		return "", "", err
	}
	_, found, err := s.remote.Database(ctx, name)
	if err != nil {
		return "", "", err
	}
	if !found {
		if _, err := s.remote.Create(ctx, CreateOptions{Name: name, IsSchema: true}); err != nil {
			return "", "", fmt.Errorf("db: creating the schema database %s: %w", name, err)
		}
	}
	st, err := s.remote.Open(ctx, name, false)
	if err != nil {
		return "", "", err
	}
	defer st.Close()
	hash, mig, ok, err := schemaState(ctx, shared.db, ss.key)
	if err != nil {
		return "", "", err
	}
	if ok && hash == s.hash {
		return SiloOK, "", nil
	}
	ms := s.cfg.Migrations
	var pending []Migration
	fresh := !ok
	if ok {
		at := slices.IndexFunc(ms, func(m Migration) bool { return m.Name == mig })
		switch {
		case mig != "" && at < 0 && mig > s.last:
			return SiloAhead, fmt.Sprintf("a newer build migrated the schema database (to %s, which this build does not have): deploy that build again", mig), nil
		case len(ms) == 0 && s.cfg.Dev:
			fresh = true
		case len(ms) == 0 || at < 0 || at == len(ms)-1:
			return SiloFailed, fmt.Sprintf("the schema database is at %q with schema %.12s, which this build's migrations do not lead from: run `alder migrate`", mig, hash), nil
		default:
			pending = ms[at+1:]
		}
	}
	if fresh {
		existing, err := appTables(ctx, st.db)
		if err != nil {
			return "", "", err
		}
		if len(existing) > 0 && !s.cfg.Dev {
			if _, live, err := liveSchema(ctx, st.db); err == nil && live == ss.hash {
				return SiloOK, "", s.recordSchemaDB(ctx, shared, ss.key)
			}
			return SiloFailed, fmt.Sprintf("the schema database %s has tables but no state in the shared database: it was not made by this app's deploy", name), nil
		}
	}
	var drops []string
	for _, m := range pending {
		for _, line := range m.DropsData {
			if ss.mentions(line) {
				drops = append(drops, m.Name+": "+line)
			}
		}
	}
	if len(drops) > 0 && !allowDataLoss && !s.cfg.Dev && os.Getenv(EnvAllowDataLoss) != "1" {
		return SiloDataLoss, "a pending migration DROPS DATA of every silo (" + strings.Join(drops, "; ") + "): deploy with --allow-data-loss", nil
	}
	if err := ss.propagable(pending); err != nil {
		return SiloFailed, err.Error(), nil
	}
	conn, err := st.db.Conn(ctx)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	restore, err := foreignKeysOff(ctx, conn)
	if err != nil {
		return "", "", err
	}
	defer restore()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var stmts []string
	if fresh {
		existing, err := appTables(ctx, tx)
		if err != nil {
			return "", "", err
		}
		for _, n := range existing {
			stmts = append(stmts, "DROP TABLE "+quote(n))
		}
		stmts = append(append(append(stmts, ss.stmts...), ss.indexes...), createSchemaTable)
	} else {
		for _, m := range pending {
			stmts = append(stmts, ss.statements(m)...)
		}
	}
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return SiloFailed, fmt.Sprintf("the schema database %s: %s: %v", name, firstWords(q, 6), err), nil
		}
	}
	if _, live, err := liveSchema(ctx, tx); err != nil {
		return "", "", err
	} else if live != ss.hash {
		return SiloFailed, fmt.Sprintf("after the migrations the schema database's tables (%.12s) are not the silo's (%.12s)", live, ss.hash), nil
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	if err := s.recordSchemaDB(ctx, shared, ss.key); err != nil {
		return "", "", err
	}
	if fresh {
		return SiloOK, "created", nil
	}
	var names []string
	for _, m := range pending {
		names = append(names, m.Name)
	}
	return SiloOK, "migrated " + strings.Join(names, ", "), nil
}

// recordSchemaDB records in the shared database that silo key's schema database is at this
// binary's schema.
func (s *Stores) recordSchemaDB(ctx context.Context, shared *SQLite, key string) error {
	if _, err := shared.db.ExecContext(ctx, createMetaTable); err != nil {
		return err
	}
	_, err := shared.db.ExecContext(ctx, `INSERT INTO "_alder_meta" ("key", "value") VALUES (?, ?) ON CONFLICT ("key") DO UPDATE SET "value" = excluded."value"`, siloSchemaMeta(key), s.hash+" "+s.last)
	return err
}

// ---- the binary's maintenance tasks over every silo: files or children

// siloEntry is one silo database: a file (path) or a child on Turso (name).
type siloEntry struct {
	key, value, path, name string
}

// entries are the silo databases of every key: the files under Dir, or Turso's children of
// each key's schema database.
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
		kids, err := s.remote.Children(ctx, SchemaName(s.app, si.Key))
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
	if e.name != "" {
		return s.checkChild(ctx, st, s.silos[e.key], apply)
	}
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

// migrateRemote is MigrateSilos on Turso: each key's schema database (created when absent),
// then, unless cfg.SchemaOnly, each child (its row written when the change has reached it).
func (s *Stores) migrateRemote(ctx context.Context, out io.Writer) (int, error) {
	shared, err := OpenSQLite(s.cfg.Shared)
	if err != nil {
		return 0, err
	}
	defer shared.Close()
	bad := 0
	for _, si := range s.cfg.Silos {
		ss := s.silos[si.Key]
		state, detail, err := s.migrateSchemaDB(ctx, shared, ss, s.cfg.AllowDataLoss)
		if err != nil {
			state, detail = SiloFailed, err.Error()
		}
		fmt.Fprintln(out, stateLine(si.Key+" (schema database "+SchemaName(s.app, si.Key)+")", state, detail))
		if state != SiloOK {
			bad++
		}
	}
	if s.cfg.SchemaOnly || bad > 0 {
		return bad, nil
	}
	es, err := s.entries(ctx)
	if err != nil {
		return bad, err
	}
	for _, e := range es {
		state, detail := SiloFailed, ""
		st, err := s.openEntry(ctx, e, false)
		if err == nil {
			state, detail, err = s.state(ctx, st, e, s.cfg.AllowDataLoss, true)
			st.Close()
		}
		if err != nil {
			state, detail = SiloFailed, err.Error()
		}
		if state != SiloOK {
			bad++
		}
		fmt.Fprintln(out, stateLine(e.key+"/"+e.value, state, detail))
	}
	return bad, nil
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
// check: that the database is where the plan starts, that every statement runs, that no row is
// left pointing at a deleted one, and that the tables come out as this binary's. On Turso the
// plan starts at the schema database's state (the shared database's record) and the databases
// are its children by storage size (the API's usage); for files, each file's own row, by file
// size. It writes a line per database and returns how many fail. `alder deploy` runs it on
// Turso before migrating anything (the binary's --verify-silos N).
func VerifySilos(ctx context.Context, cfg StoresConfig, n int, out io.Writer) (int, error) {
	s, err := newStores(nil, cfg)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		n = 5
	}
	var shared *SQLite
	if s.remote != nil {
		if shared, err = OpenSQLiteReader(cfg.Shared); err != nil {
			return 0, err
		}
		defer shared.Close()
	}
	es, err := s.entries(ctx)
	if err != nil {
		return 0, err
	}
	sizes := map[siloEntry]int64{}
	for _, e := range es {
		if e.name != "" {
			if sizes[e], err = s.remote.StorageBytes(ctx, e.name); err != nil {
				return 0, err
			}
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
		var pending []Migration
		startHash := "" // the tables a child must have before the plan (Turso: the schema database's)
		if s.remote != nil {
			_, mig, ok, err := schemaState(ctx, shared.db, si.Key)
			if err != nil {
				return bad, err
			}
			if !ok {
				fmt.Fprintf(out, "%s: no schema database yet: nothing to verify\n", si.Key)
				continue
			}
			at := slices.IndexFunc(cfg.Migrations, func(m Migration) bool { return m.Name == mig })
			if at >= 0 {
				pending = cfg.Migrations[at+1:]
			}
			if err := ss.propagable(pending); err != nil {
				fmt.Fprintf(out, "%s: FAIL: %v\n", si.Key, err)
				bad++
				continue
			}
			sdb, err := s.remote.Open(ctx, SchemaName(s.app, si.Key), true)
			if err != nil {
				return bad, err
			}
			_, startHash, err = liveSchema(ctx, sdb.db)
			sdb.Close()
			if err != nil {
				return bad, err
			}
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
			ms := pending
			if s.remote == nil {
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
				ms = p.pending
			}
			err = s.dryRun(ctx, st, ss, ms, startHash)
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
// back, after checking that st's tables are at startHash ("": not checked), and checks the
// result as the open-time check does.
func (s *Stores) dryRun(ctx context.Context, st *SQLite, ss *siloSchema, ms []Migration, startHash string) error {
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
	if startHash != "" {
		_, live, err := liveSchema(ctx, tx)
		if err != nil {
			return err
		}
		if live != startHash {
			return fmt.Errorf("its tables (%.12s) are not the schema database's (%.12s): it has not taken an earlier change yet, or was changed by hand", live, startHash)
		}
	}
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
