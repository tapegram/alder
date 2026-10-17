package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"alder/rt/eff"
)

// The M15 silo fixture: Org is shared, Project lives in `silo org` (org @ref Org @partition org).

type proj struct{ ID, Name, Org string }

type orgSession struct{ org string } // "" is Nothing

func projTable(cols []Column) *Table[proj] {
	return &Table[proj]{
		Name:    "Project.Project",
		Columns: cols,
		ID:      "id",
		Encode:  func(r proj) []any { return []any{r.ID, r.Name, r.Org} },
		Decode: func(v []any) (r proj, err error) {
			r.ID, _ = v[0].(string)
			for i, c := range cols {
				if c.Name == "name" {
					r.Name, _ = v[i].(string)
				}
				if c.Name == "org" {
					r.Org, _ = v[i].(string)
				}
			}
			return r, nil
		},
		ReadID: func(r proj) string { return r.ID },
		SetID:  func(r proj, id string) proj { r.ID = id; return r },
		Owner:  "org",
		Key:    "org",
		Silo:   "org",
		User: func(s any) (string, bool) {
			a, ok := s.(orgSession)
			return a.org, ok && a.org != ""
		},
	}
}

var (
	orgDesc   = TableDesc{Name: "Org.Org", ID: "id", Columns: []Column{{Name: "id", Type: CText}, {Name: "name", Type: CText}}}
	projCols1 = []Column{{Name: "id", Type: CText}, {Name: "name", Type: CText}, {Name: "org", Type: CText, Ref: "Org.Org", OnDelete: Cascade}}
	// v2 adds a column; v3 instead drops `name` (DROPS DATA)
	projCols2 = append(append([]Column(nil), projCols1...), Column{Name: "note", Type: CText, Nullable: true})
	projCols3 = []Column{{Name: "id", Type: CText}, {Name: "org", Type: CText, Ref: "Org.Org", OnDelete: Cascade}}
	projT     = projTable(projCols1)
	orgT      = &Table[proj]{Name: "Org.Org", Columns: orgDesc.Columns, ID: "id",
		Encode: func(r proj) []any { return []any{r.ID, r.Name} },
		Decode: func(v []any) (r proj, err error) { r.ID, _ = v[0].(string); r.Name, _ = v[1].(string); return r, nil },
		ReadID: func(r proj) string { return r.ID }, SetID: func(r proj, id string) proj { r.ID = id; return r }}
)

func projDesc(cols []Column) TableDesc {
	return TableDesc{Name: "Project.Project", ID: "id", Columns: cols}
}

// migrationsUpTo are the app's migrations: 0001 creates both tables (v1); 0002 is v2's (add
// `note`) or v3's (drop `name`, DROPS DATA).
func siloMigrations(t *testing.T, v int) []Migration {
	t.Helper()
	by := map[string]TableDesc{"Org.Org": orgDesc, "Project.Project": projDesc(projCols1)}
	a, _ := createStmt(orgDesc, by)
	b, _ := createStmt(projDesc(projCols1), by)
	ms := []Migration{{Name: "0001_initial", SQL: a + ";\n" + b + ";\n" + `CREATE INDEX IF NOT EXISTS "Project__Project__org" ON "Project__Project"("org");`}}
	hashOf := func(cols []Column) string {
		sc, err := schemaOf([]TableDesc{orgDesc, projDesc(cols)})
		if err != nil {
			t.Fatal(err)
		}
		return short(sc.hash)
	}
	_, empty := hashSchema(nil)
	ms[0].From, ms[0].To = short(empty), hashOf(projCols1)
	defer func() {
		if len(ms) == 2 {
			ms[1].From, ms[1].To = ms[0].To, hashOf(map[int][]Column{2: projCols2, 3: projCols3}[v])
		}
	}()
	switch v {
	case 2:
		ms = append(ms, Migration{Name: "0002_note", SQL: `ALTER TABLE "Project__Project" ADD COLUMN "note" TEXT;`})
	case 3:
		ms = append(ms, Migration{Name: "0003_drop_name", SQL: `ALTER TABLE "Project__Project" DROP COLUMN "name";`, DropsData: []string{"column Project.name"}})
	}
	return ms
}

func siloConfig(t *testing.T, dir string, v int) StoresConfig {
	cols := map[int][]Column{1: projCols1, 2: projCols2, 3: projCols3}[v]
	return StoresConfig{Dir: dir, Silos: []Silo{{Key: "org", Tables: []string{"Project.Project"}}},
		Tables: []TableDesc{orgDesc, projDesc(cols)}, Migrations: siloMigrations(t, v)}
}

// openApp opens the shared store at dir/app.db, migrated to version v, and its Stores.
func openApp(t *testing.T, dir string, v int) *Stores {
	t.Helper()
	cfg := siloConfig(t, dir, v)
	path := filepath.Join(dir, "app.db")
	shared, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.Close() })
	if _, err := shared.Migrate(context.Background(), cfg.Tables, cfg.Migrations, false); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStores(shared, path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func siloEnv(s *Stores, org string) *eff.Env {
	return &eff.Env{Ctx: context.Background(), Store: s.Shared(), Stores: s, Session: orgSession{org}, Now: func() time.Time { return clock }}
}

func TestSiloPath_FilenameSafety(t *testing.T) {
	p, err := SiloPath("/data", "org", "Ab3_-x")
	if err != nil || p != filepath.Join("/data", "silos", "org", SiloFileName("Ab3_-x")) {
		t.Fatalf("got %q, %v", p, err)
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b", "../etc", "x..y", "-rf " + strings.Repeat("x", 3), strings.Repeat("x", 65), "é"} {
		if _, err := SiloPath("/data", "org", bad); err == nil {
			t.Errorf("value %q: no refusal", bad)
		}
	}
	if _, err := SiloPath("/data", "../x", "a"); err == nil {
		t.Error("key ../x: no refusal")
	}
}

// Two orgs: each one's rows are in its own file, and a scoped read sees only its own.
func TestStores_SiloPerValue(t *testing.T) {
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	ctx := context.Background()
	for _, org := range []string{"A", "B"} {
		if _, err := InsertRow(ctx, s.Shared(), nil, orgT, []any{nil, org}, clock); err != nil {
			t.Fatal(err)
		}
	}
	insert := func(org, name string) {
		env := siloEnv(s, org)
		Narrow(env, "org", org)
		if err := Enter(env, "org"); err != nil {
			t.Fatal(err)
		}
		task := eff.Call("Db.insertScoped Project", true, func(e *eff.Env) (string, error) {
			return InsertScopedRow(e.Ctx, e.Store.(Store), TxOf(e), projT, []any{nil, name, nil}, clock)
		})
		if _, err := eff.Run(env, eff.KTask, task); err != nil {
			t.Fatal(err)
		}
	}
	insert("A", "pa")
	insert("B", "pb")
	for org, want := range map[string]string{"A": "pa", "B": "pb"} {
		env := siloEnv(s, org)
		rows, err := eff.RunQuery(env, eff.Q(eff.Call("Db.all", false, func(e *eff.Env) ([]proj, error) {
			return All(e.Ctx, e.Store.(Store), TxOf(e), Scoped(projT))
		})))
		if err != nil || len(rows) != 1 || rows[0].Name != want {
			t.Fatalf("org %s: %v %v", org, rows, err)
		}
	}
	// the files: B's lacks A's row; the shared store has none
	for org, other := range map[string]string{"A": "pb", "B": "pa"} {
		f, err := OpenSQLite(filepath.Join(dir, "silos", "org", SiloFileName(org)))
		if err != nil {
			t.Fatal(err)
		}
		if n := rawCount(t, f, `SELECT COUNT(*) FROM "Project__Project" WHERE "name" = '`+other+`'`); n != 0 {
			t.Errorf("silo %s holds %s", org, other)
		}
		if n := rawCount(t, f, `SELECT COUNT(*) FROM "Project__Project"`); n != 1 {
			t.Errorf("silo %s holds %d rows", org, n)
		}
		f.Close()
	}
	if n := rawCount(t, s.Shared(), `SELECT COUNT(*) FROM "Project__Project"`); n != 0 {
		t.Errorf("the shared store holds %d projects", n)
	}
	// the _alder_schema row records the app's hash and last migration
	f, _ := OpenSQLite(filepath.Join(dir, "silos", "org", SiloFileName("A")))
	defer f.Close()
	h, m, ok, err := schemaRow(ctx, f.db)
	if !ok || err != nil || h != s.hash || m != "0001_initial" {
		t.Fatalf("schema row %q %q %v %v", h, m, ok, err)
	}
	// the silo's Project has no REFERENCES into the shared Org
	var stmt string
	if err := f.db.QueryRow(`SELECT "sql" FROM sqlite_schema WHERE "name" = 'Project__Project'`).Scan(&stmt); err != nil || strings.Contains(stmt, "REFERENCES") {
		t.Fatalf("silo table %q %v", stmt, err)
	}
}

// A Task on silo A reads the shared store (Org) through a read-only connection outside its
// transaction; a write through that connection fails, and so does a write to silo B.
func TestStores_CrossStoreReadOnly(t *testing.T) {
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	ctx := context.Background()
	orgID, _ := InsertRow(ctx, s.Shared(), nil, orgT, []any{nil, "Acme"}, clock)
	env := siloEnv(s, "A")
	Narrow(env, "org", "A")
	if err := Enter(env, "org"); err != nil {
		t.Fatal(err)
	}
	var name string
	task := eff.Call("t", true, func(e *eff.Env) (struct{}, error) {
		o, err := GetRow(e.Ctx, e.Store.(Store), TxOf(e), orgT, orgID)
		if err != nil || !o.Ok {
			return struct{}{}, errors.Join(errors.New("no org"), err)
		}
		name = o.Val.Name
		_, err = InsertScopedRow(e.Ctx, e.Store.(Store), TxOf(e), projT, []any{nil, "p", nil}, clock)
		return struct{}{}, err
	})
	if _, err := eff.Run(env, eff.KTask, task); err != nil || name != "Acme" {
		t.Fatalf("%q %v", name, err)
	}
	// a write of the shared table from the silo's Task: refused
	env = siloEnv(s, "A")
	Narrow(env, "org", "A")
	Enter(env, "org")
	_, err := eff.Run(env, eff.KTask, eff.Call("w", true, func(e *eff.Env) (string, error) {
		return InsertRow(e.Ctx, e.Store.(Store), TxOf(e), orgT, []any{nil, "x"}, clock)
	}))
	if !errors.Is(err, eff.ErrInternal) {
		t.Fatalf("shared write from a silo Task: %v", err)
	}
	// the reader itself refuses a write, at the Go level and at SQLite's
	ro := readOnly{s.reader, errReaderWrite}
	if _, err := ro.Insert(ctx, nil, "Org.Org", orgDesc.Columns, "id", "", "", []any{nil, "x"}, clock); !errors.Is(err, eff.ErrInternal) {
		t.Fatalf("Insert through the reader: %v", err)
	}
	if _, err := s.reader.db.Exec(`INSERT INTO "Org__Org" ("id", "name") VALUES ('z', 'z')`); err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("raw write through the reader: %v", err)
	}
	// a row of another org from silo A's Task
	env = siloEnv(s, "A")
	Narrow(env, "org", "A")
	Enter(env, "org")
	_, err = eff.Run(env, eff.KTask, eff.Call("w", true, func(e *eff.Env) (string, error) {
		return InsertRow(e.Ctx, e.Store.(Store), TxOf(e), projT, []any{nil, "x", "B"}, clock)
	}))
	var f *eff.ErrFail
	if !errors.As(err, &f) || f.Msg != "Db.insert Project: the row's org is not this request's" {
		t.Fatalf("a B row from A's Task: %v", err)
	}
}

func TestStores_Cache(t *testing.T) {
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	ctx := context.Background()
	d1, err := s.silo(ctx, "org", "A")
	if err != nil {
		t.Fatal(err)
	}
	d2, _ := s.silo(ctx, "org", "A")
	if d1 != d2 {
		t.Fatal("not cached")
	}
	if _, err := s.silo(ctx, "org", "../x"); err == nil {
		t.Fatal("unsafe value opened")
	}
}

// makeSilo creates silo org/value with app version v (one row), then closes everything.
func makeSilo(t *testing.T, dir string, v int, value string) {
	t.Helper()
	cfg := siloConfig(t, dir, v)
	st, err := newStores(nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := SiloPath(dir, "org", value)
	os.MkdirAll(filepath.Dir(p), 0o755)
	f, err := OpenSQLite(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if state, _, err := st.check(context.Background(), f, st.silos["org"], false, true); err != nil || state != SiloOK {
		t.Fatalf("%s %v", state, err)
	}
	f.register(st.silos["org"].descs)
	if _, err := f.Insert(context.Background(), nil, "Project.Project", st.silos["org"].descs[0].Columns, "id", "", "", projVals(v), clock); err != nil {
		t.Fatal(err)
	}
}

func projVals(v int) []any {
	if v == 3 {
		return []any{nil, value3}
	}
	if v == 2 {
		return []any{nil, "p", value3, nil}
	}
	return []any{nil, "p", value3}
}

const value3 = "X"

// The three open-time outcomes: behind (migrated on open), behind by a migration that drops data
// (maintenance; the other org served), ahead (maintenance); and --migrate-silos/--silo-status.
func TestStores_OpenTimeOutcomes(t *testing.T) {
	dir := t.TempDir()
	makeSilo(t, dir, 1, "behind")
	makeSilo(t, dir, 3, "ahead")
	// v2 serves: "behind" migrates on open, "ahead" (made by v3, whose 0002 v2 lacks) does not
	s := openApp(t, dir, 2)
	ctx := context.Background()
	if _, err := s.silo(ctx, "org", "behind"); err != nil {
		t.Fatalf("behind: %v", err)
	}
	var stmt string
	f, _ := OpenSQLite(filepath.Join(dir, "silos", "org", SiloFileName("behind")))
	f.db.QueryRow(`SELECT "sql" FROM sqlite_schema WHERE "name" = 'Project__Project'`).Scan(&stmt)
	if !strings.Contains(stmt, `"note" TEXT`) {
		t.Fatalf("not migrated: %s", stmt)
	}
	if _, m, _, _ := schemaRow(ctx, f.db); m != "0002_note" {
		t.Fatalf("row %q", m)
	}
	f.Close()
	_, err := s.silo(ctx, "org", "ahead")
	var me *MaintenanceError
	if !errors.As(err, &me) || me.State != SiloAhead || !strings.Contains(me.Detail, "alder restore <env> --silo org:<id>") {
		t.Fatalf("ahead: %v", err)
	}

	// data loss: a v1 silo opened by v3 (0002 drops Project.name)
	dir2 := t.TempDir()
	makeSilo(t, dir2, 1, "old")
	s3 := openApp(t, dir2, 3)
	_, err = s3.silo(ctx, "org", "old")
	if !errors.As(err, &me) || me.State != SiloDataLoss || !strings.Contains(me.Detail, "0003_drop_name: column Project.name") {
		t.Fatalf("data loss: %v", err)
	}
	if _, err := s3.silo(ctx, "org", "fresh"); err != nil { // another org is served
		t.Fatalf("fresh: %v", err)
	}
	// --silo-status lists them without changing anything
	var out bytes.Buffer
	if err := StatusSilos(ctx, siloConfig(t, dir2, 3), &out); err != nil {
		t.Fatal(err)
	}
	var lines []SiloStatus
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var st SiloStatus
		if err := json.Unmarshal([]byte(l), &st); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		lines = append(lines, st)
	}
	if len(lines) != 2 || lines[0].ID != "fresh" || lines[0].State != SiloOK || lines[1].ID != "old" || lines[1].State != SiloDataLoss {
		t.Fatalf("status: %s", out.String())
	}
	// --migrate-silos refuses without --allow-data-loss, applies with it
	out.Reset()
	if bad, err := MigrateSilos(ctx, siloConfig(t, dir2, 3), &out); err != nil || bad != 1 || !strings.Contains(out.String(), "org/old: pending-data-loss: ") {
		t.Fatalf("%d %v %s", bad, err, out.String())
	}
	cfg := siloConfig(t, dir2, 3)
	cfg.AllowDataLoss = true
	out.Reset()
	if bad, err := MigrateSilos(ctx, cfg, &out); err != nil || bad != 0 || !strings.Contains(out.String(), "org/old: migrated 0003_drop_name") {
		t.Fatalf("%d %v %s", bad, err, out.String())
	}
	// the cached maintenance answer is checked again after `recheck`
	s3.mu.Lock()
	s3.open["org/old"].checked = time.Now().Add(-2 * time.Minute)
	s3.mu.Unlock()
	if _, err := s3.silo(ctx, "org", "old"); err != nil {
		t.Fatalf("after migrate silos: %v", err)
	}
	// the statuses of a directory mixing the three: behind (migrating), data loss, ahead
	dir3 := t.TempDir()
	makeSilo(t, dir3, 1, "b1")
	makeSilo(t, dir3, 3, "c3")
	out.Reset()
	cfg2 := siloConfig(t, dir3, 2)
	if err := StatusSilos(ctx, cfg2, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `{"key":"org","id":"b1","state":"migrating"}`) || !strings.Contains(out.String(), `"id":"c3","state":"ahead"`) {
		t.Fatalf("status: %s", out.String())
	}
}

// A test's stores: one in-memory database per silo value touched, each with the test's
// transaction, and savepoints that span them (a failed step undoes its silo writes too).
func TestStores_TestMode(t *testing.T) {
	ctx := context.Background()
	shared, _ := OpenSQLite(":memory:")
	defer shared.Close()
	cfg := siloConfig(t, "", 1)
	if err := shared.Ensure(ctx, cfg.Tables, true); err != nil {
		t.Fatal(err)
	}
	s, err := NewTestStores(shared, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	env := &eff.Env{Ctx: ctx, Store: TestStore{shared, s}, Stores: s, Session: orgSession{}, Now: func() time.Time { return clock }}
	_, err = eff.Run(env, eff.KTask, eff.Call("test", true, func(e *eff.Env) (struct{}, error) {
		st, tx := e.Store.(Store), TxOf(e)
		// rows name their own silo
		if _, err := InsertRow(e.Ctx, st, tx, projT, []any{nil, "a", "A"}, clock); err != nil {
			return struct{}{}, err
		}
		sp := e.Store.(Savepointer)
		if err := sp.Savepoint(e.Ctx, tx, "s1"); err != nil {
			return struct{}{}, err
		}
		InsertRow(e.Ctx, st, tx, projT, []any{nil, "a2", "A"}, clock)
		InsertRow(e.Ctx, st, tx, projT, []any{nil, "b", "B"}, clock) // B's transaction begins inside s1
		if err := sp.RollbackTo(e.Ctx, tx, "s1"); err != nil {
			return struct{}{}, err
		}
		if err := sp.Release(e.Ctx, tx, "s1"); err != nil {
			return struct{}{}, err
		}
		e.Session = orgSession{"A"}
		rows, err := All(e.Ctx, st, tx, Scoped(projT))
		if err != nil || len(rows) != 1 || rows[0].Name != "a" {
			return struct{}{}, errors.Join(errors.New("A"), err)
		}
		e.Session = orgSession{"B"}
		n, err := CountRows(e.Ctx, st, tx, From(projT))
		if err != nil || n != 0 {
			return struct{}{}, errors.Join(errors.New("B"), err)
		}
		e.Session = orgSession{}
		_, err = All(e.Ctx, st, tx, From(projT))
		var f *eff.ErrFail
		if !errors.As(err, &f) || f.Msg != "Db.all Project: `Project` lives in `silo org` and the session has no org" {
			return struct{}{}, errors.Join(errors.New("no org"), err)
		}
		return struct{}{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if s.TestSilo("org", "A") == nil || s.TestSilo("org", "C") != nil {
		t.Fatal("TestSilo")
	}
}

// Member is `via`'s check, inside the Task's transaction: a membership deleted earlier in the
// same Task is gone.
func TestMember_InsideTheTransaction(t *testing.T) {
	ctx := context.Background()
	mem := &Table[proj]{Name: "Membership.Membership", ID: "id",
		Columns: []Column{{Name: "id", Type: CText}, {Name: "user", Type: CText}, {Name: "org", Type: CText}},
		Decode:  func(v []any) (proj, error) { return proj{}, nil }}
	s, _ := OpenSQLite(":memory:")
	defer s.Close()
	if err := s.Ensure(ctx, []TableDesc{mem.Desc()}, true); err != nil {
		t.Fatal(err)
	}
	id, _ := InsertRow(ctx, s, nil, mem, []any{nil, "u", "A"}, clock)
	env := &eff.Env{Ctx: ctx, Store: s}
	_, err := eff.Run(env, eff.KTask, eff.Call("t", true, func(e *eff.Env) (struct{}, error) {
		ok, err := Member(e.Ctx, s, TxOf(e), mem, "user", "u", "org", "A")
		if err != nil || !ok {
			return struct{}{}, errors.Join(errors.New("before"), err)
		}
		if err := DeleteRow(e.Ctx, s, TxOf(e), mem, id); err != nil {
			return struct{}{}, err
		}
		if ok, err := Member(e.Ctx, s, TxOf(e), mem, "user", "u", "org", "A"); err != nil || ok {
			return struct{}{}, errors.Join(errors.New("after"), err)
		}
		return struct{}{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
}

// I1: opening (and migrating) one silo holds up no other: B's open takes 2 s, A's — new or open
// — is served meanwhile; a second request for B waits for the first.
func TestStores_OpenIsolation(t *testing.T) {
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	ctx := context.Background()
	if _, err := s.silo(ctx, "org", "A"); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	openHook = func(key, value string) {
		if value == "B" {
			close(started)
			time.Sleep(2 * time.Second)
		}
	}
	defer func() { openHook = nil }()
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.silo(ctx, "org", "B"); done <- err }()
	}
	<-started
	t0 := time.Now()
	if _, err := s.silo(ctx, "org", "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.silo(ctx, "org", "C"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(t0); d > 500*time.Millisecond {
		t.Fatalf("A and C waited %s behind B's open", d)
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(t0); d < time.Second {
		t.Fatalf("B's second request did not wait for its open (%s)", d)
	}
}

// M2: a silo unused for IdleClose is closed and opened again on next use.
func TestStores_IdleClose(t *testing.T) {
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	s.cfg.IdleClose = time.Millisecond
	ctx := context.Background()
	d1, _ := s.silo(ctx, "org", "A")
	time.Sleep(5 * time.Millisecond)
	s.mu.Lock()
	s.swept = time.Time{}
	s.mu.Unlock()
	d2, err := s.silo(ctx, "org", "A")
	if err != nil || d1 == d2 {
		t.Fatalf("not reopened: %v", err)
	}
}

// M3: a draining process opens no silo, and a write to an open one whose schema row another
// build changed answers the maintenance page.
func TestStores_DrainRecheck(t *testing.T) {
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	ctx := context.Background()
	if _, err := s.silo(ctx, "org", "A"); err != nil {
		t.Fatal(err)
	}
	Drain()
	defer s.draining.Store(false)
	var me *MaintenanceError
	if _, err := s.silo(ctx, "org", "new"); !errors.As(err, &me) || !strings.Contains(me.Detail, "shutting down") {
		t.Fatalf("a new silo while draining: %v", err)
	}
	write := func() error {
		env := siloEnv(s, "A")
		Narrow(env, "org", "A")
		if err := Enter(env, "org"); err != nil {
			return err
		}
		_, err := eff.Run(env, eff.KTask, eff.Call("w", true, func(e *eff.Env) (string, error) {
			return InsertScopedRow(e.Ctx, e.Store.(Store), TxOf(e), projT, []any{nil, "p", nil}, clock)
		}))
		return err
	}
	if err := write(); err != nil {
		t.Fatalf("write while draining, schema unchanged: %v", err)
	}
	sqlFile(t, filepath.Join(dir, "silos", "org", SiloFileName("A")), `UPDATE "_alder_schema" SET "hash" = 'newer'`)
	if err := write(); !errors.As(err, &me) || me.State != SiloAhead {
		t.Fatalf("write after another build migrated it: %v", err)
	}
	if _, err := s.silo(ctx, "org", "A"); !errors.As(err, &me) {
		t.Fatalf("the silo still served: %v", err)
	}
}

// sqlFile runs stmt on the SQLite file path.
func sqlFile(t *testing.T, path, stmt string) {
	t.Helper()
	f, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.db.Exec(stmt); err != nil {
		t.Fatal(err)
	}
}

// I2: ids are case-sensitive, file systems may not be: "a" and "A" are two files; a name is the
// id in lowercase base32hex, read back by SiloValue.
func TestSiloFileName(t *testing.T) {
	a, A := SiloFileName("a"), SiloFileName("A")
	if strings.EqualFold(a, A) || a != strings.ToLower(a) {
		t.Fatalf("%q %q", a, A)
	}
	for _, id := range []string{"a", "A", "-rf", "app", "Zz_9-" + strings.Repeat("q", 59)} {
		if v, ok := SiloValue(SiloFileName(id)); !ok || v != id {
			t.Fatalf("%q: %q %v", id, v, ok)
		}
	}
	for _, name := range []string{"A.db", "c4.DB", "x.db-wal", "c4", "zz.db"} {
		if v, ok := SiloValue(name); ok {
			t.Fatalf("%q read as %q", name, v)
		}
	}
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	ctx := context.Background()
	for _, org := range []string{"a", "A"} {
		if _, err := s.silo(ctx, "org", org); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := SiloFiles(dir, s.cfg.Silos)
	if len(files) != 2 || files[0].Value != "A" || files[1].Value != "a" {
		t.Fatalf("%+v", files)
	}
}

// I2: a value that is not an id, or whose key row (its Org) does not exist, is not-found and
// makes no file — the admin form's parameter included; an existing one opens.
func TestStores_KeyRowExists(t *testing.T) {
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	s.cfg.Silos[0].Ref = "Org.Org"
	ctx := context.Background()
	id, _ := InsertRow(ctx, s.Shared(), nil, orgT, []any{nil, "Acme"}, clock)
	for _, v := range []string{"-rf", "app", "nope", strings.Repeat("x", 65), "a/b"} {
		if _, err := s.silo(ctx, "org", v); !errors.Is(err, eff.ErrNotFound) {
			t.Fatalf("%q: %v", v, err)
		}
	}
	if _, err := s.silo(ctx, "org", id); err != nil {
		t.Fatal(err)
	}
	files, _ := SiloFiles(dir, s.cfg.Silos)
	if len(files) != 1 || files[0].Value != id {
		t.Fatalf("%+v", files)
	}
	// through a request: the admin form's parameter
	env := siloEnv(s, "")
	Narrow(env, "org", "nope")
	if err := Enter(env, "org"); !errors.Is(err, eff.ErrNotFound) {
		t.Fatalf("Enter: %v", err)
	}
}

// I4: Db.update cannot move a siloed row to another key value.
func TestUpdateRow_KeyColumnRefused(t *testing.T) {
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	env := siloEnv(s, "A")
	Narrow(env, "org", "A")
	if err := Enter(env, "org"); err != nil {
		t.Fatal(err)
	}
	_, err := eff.Run(env, eff.KTask, eff.Call("t", true, func(e *eff.Env) (struct{}, error) {
		id, err := InsertScopedRow(e.Ctx, e.Store.(Store), TxOf(e), projT, []any{nil, "p", nil}, clock)
		if err != nil {
			return struct{}{}, err
		}
		if err := UpdateRow(e.Ctx, e.Store.(Store), TxOf(e), projT, id, func(p proj) proj { p.Name = "q"; return p }, clock); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, UpdateRow(e.Ctx, e.Store.(Store), TxOf(e), projT, id, func(p proj) proj { p.Org = "B"; return p }, clock)
	}))
	var f *eff.ErrFail
	if !errors.As(err, &f) || f.Msg != "Db.update Project: a row cannot move to another org (delete it and insert it there)" {
		t.Fatalf("%v", err)
	}
}

// I3: a test fails where production does: from the test's body, a row of another org than the
// session's; from a request's Task (Enter), a row of another org, or a write of the other store.
func TestStores_TestModeWriteChecks(t *testing.T) {
	ctx := context.Background()
	shared, _ := OpenSQLite(":memory:")
	defer shared.Close()
	cfg := siloConfig(t, "", 1)
	if err := shared.Ensure(ctx, cfg.Tables, true); err != nil {
		t.Fatal(err)
	}
	s, _ := NewTestStores(shared, cfg)
	defer s.Close()
	env := &eff.Env{Ctx: ctx, Store: TestStore{shared, s}, Stores: s, Session: orgSession{}, Now: func() time.Time { return clock }}
	ins := func(e *eff.Env, t2 *Table[proj], vals []any) error {
		_, err := InsertRow(e.Ctx, e.Store.(Store), TxOf(e), t2, vals, clock)
		return err
	}
	_, err := eff.Run(env, eff.KTask, eff.Call("test", true, func(e *eff.Env) (struct{}, error) {
		if err := ins(e, projT, []any{nil, "b", "B"}); err != nil { // no org in the session: setup
			return struct{}{}, err
		}
		e.Session = orgSession{"A"}
		var f *eff.ErrFail
		if err := ins(e, projT, []any{nil, "b2", "B"}); !errors.As(err, &f) || f.Msg != "Db.insert Project: the row's org is not this request's" {
			return struct{}{}, errors.Join(errors.New("body"), err)
		}
		// a request's Task on A, and one on the shared store, inside the test's transaction
		req := func(key string, f func(*eff.Env) error) error {
			r := &eff.Env{Ctx: context.Background(), Store: e.Store, Stores: s, Session: orgSession{"A"}, Within: &eff.Within{Tx: e.Tx}}
			Narrow(r, "org", "A")
			if err := Enter(r, key); err != nil {
				return err
			}
			_, err := eff.Run(r, eff.KTask, eff.Call("req", true, func(r *eff.Env) (struct{}, error) { return struct{}{}, f(r) }))
			return err
		}
		if err := req("org", func(r *eff.Env) error { return ins(r, projT, []any{nil, "b3", "B"}) }); !errors.As(err, &f) {
			return struct{}{}, errors.Join(errors.New("bound"), err)
		}
		if err := req("org", func(r *eff.Env) error { return ins(r, orgT, []any{nil, "o"}) }); !errors.Is(err, eff.ErrInternal) {
			return struct{}{}, errors.Join(errors.New("shared from silo"), err)
		}
		if err := req("", func(r *eff.Env) error { return ins(r, projT, []any{nil, "a", "A"}) }); !errors.Is(err, eff.ErrInternal) {
			return struct{}{}, errors.Join(errors.New("silo from shared"), err)
		}
		return struct{}{}, req("org", func(r *eff.Env) error { return ins(r, projT, []any{nil, "a", "A"}) })
	}))
	if err != nil {
		t.Fatal(err)
	}
}

// C1: Test.run's Task (eff.Env.Pending, set by rtesting.Inner) is bound by its first write, so a
// write of a second store fails as a request's does; one bound to an arm's silo beforehand
// (rtesting.InnerOn) refuses the shared store from its first write.
func TestStores_TestRunBindsOnFirstWrite(t *testing.T) {
	ctx := context.Background()
	shared, _ := OpenSQLite(":memory:")
	defer shared.Close()
	cfg := siloConfig(t, "", 1)
	if err := shared.Ensure(ctx, cfg.Tables, true); err != nil {
		t.Fatal(err)
	}
	s, _ := NewTestStores(shared, cfg)
	defer s.Close()
	ins := func(e *eff.Env, t2 *Table[proj], vals []any) error {
		_, err := InsertRow(e.Ctx, e.Store.(Store), TxOf(e), t2, vals, clock)
		return err
	}
	run := func(key string, steps ...func(*eff.Env) error) error {
		env := &eff.Env{Ctx: ctx, Store: TestStore{shared, s}, Stores: s, Session: orgSession{"A"}, Now: func() time.Time { return clock }, Pending: true, SiloKey: key}
		_, err := eff.Run(env, eff.KTask, eff.Call("Test.run", true, func(e *eff.Env) (struct{}, error) {
			for _, f := range steps {
				if err := f(e); err != nil {
					return struct{}{}, err
				}
			}
			return struct{}{}, nil
		}))
		return err
	}
	org := func(e *eff.Env) error { return ins(e, orgT, []any{nil, "o"}) }
	project := func(e *eff.Env) error { return ins(e, projT, []any{nil, "p", "A"}) }
	if err := run("", org, project); !errors.Is(err, eff.ErrInternal) || !strings.Contains(err.Error(), "writes silo org in a Task on the shared store") {
		t.Fatalf("shared then silo: %v", err)
	}
	if err := run("", project, org); !errors.Is(err, eff.ErrInternal) || !strings.Contains(err.Error(), "writes the shared store in a Task on silo org") {
		t.Fatalf("silo then shared: %v", err)
	}
	if err := run("org", org); !errors.Is(err, eff.ErrInternal) {
		t.Fatalf("shared on an arm of silo org: %v", err)
	}
	var f *eff.ErrFail
	if err := run("", func(e *eff.Env) error { return ins(e, projT, []any{nil, "b", "B"}) }); !errors.As(err, &f) {
		t.Fatalf("another org's row: %v", err)
	}
	for _, steps := range [][]func(*eff.Env) error{{org, org}, {project, project}} {
		if err := run("", steps...); err != nil {
			t.Fatal(err)
		}
	}
	if err := run("org", project); err != nil {
		t.Fatal(err)
	}
}

// I1: a build that moves a table into a silo (`alder new silo org Note --empty`: the app's
// tables unchanged, the move a migration of CREATE TABLE IF NOT EXISTS whose DROPS DATA line is
// the shared database's) finds a silo made before the move behind — not ok without the table —
// and not pending data loss; opening it creates the table there.
func TestStores_MoveIntoSiloIsBehind(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	noteDesc := TableDesc{Name: "Note.Note", ID: "id", Columns: []Column{{Name: "id", Type: CText}, {Name: "org", Type: CText, Ref: "Org.Org", OnDelete: Cascade}}}
	tables := []TableDesc{orgDesc, projDesc(projCols1), noteDesc}
	by := map[string]TableDesc{}
	var stmts []string
	for _, d := range tables {
		by[d.Name] = d
	}
	for _, d := range tables {
		st, _ := createStmt(d, by)
		stmts = append(stmts, st+";")
	}
	sc, _ := schemaOf(tables)
	_, empty := hashSchema(nil)
	idx := `CREATE INDEX IF NOT EXISTS "Note__Note__org" ON "Note__Note"("org");`
	ms := []Migration{{Name: "0001_initial", From: short(empty), To: short(sc.hash), SQL: strings.Join(stmts, "\n") + "\n" + idx}}
	cfg := func(v int) StoresConfig {
		c := StoresConfig{Dir: dir, Tables: tables, Silos: []Silo{{Key: "org", Tables: []string{"Project.Project"}}}, Migrations: ms}
		if v == 2 {
			noteStmt, _ := createStmt(noteDesc, by)
			c.Silos[0].Tables = append(c.Silos[0].Tables, "Note.Note")
			c.Migrations = append(slices.Clone(ms), Migration{Name: "0002_silo_org", From: short(sc.hash), To: short(sc.hash),
				SQL:       strings.Replace(noteStmt, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1) + ";\n" + idx,
				DropsData: []string{"shared rows of Note: it moves into silo org, and the rows the shared database holds of it stay there, unread"}})
		}
		return c
	}
	path := filepath.Join(dir, "app.db")
	shared, _ := OpenSQLite(path)
	defer shared.Close()
	if _, err := shared.Migrate(ctx, tables, cfg(1).Migrations, false); err != nil {
		t.Fatal(err)
	}
	s1, _ := OpenStores(shared, path, cfg(1))
	if _, err := s1.silo(ctx, "org", "A"); err != nil {
		t.Fatal(err)
	}
	s1.Close()
	if s2, _ := newStores(nil, cfg(2)); s2.hash == s1.hash {
		t.Fatal("the silo hash does not change with the silos' tables")
	}
	// the shared database: the move is DROPS DATA (refused without the flag), its statements no-ops
	if _, err := shared.Migrate(ctx, tables, cfg(2).Migrations, false); !errors.Is(err, ErrDropsData) {
		t.Fatalf("shared migrate without --allow-data-loss: %v", err)
	}
	os.Setenv(EnvAllowDataLoss, "1")
	_, err := shared.Migrate(ctx, tables, cfg(2).Migrations, false)
	os.Unsetenv(EnvAllowDataLoss)
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := OpenStores(shared, path, cfg(2))
	defer s2.Close()
	p, _ := SiloPath(dir, "org", "A")
	f, _ := OpenSQLite(p)
	if st, detail, err := s2.check(ctx, f, s2.silos["org"], false, false); err != nil || st != SiloMigrating {
		t.Fatalf("before opening: %s %q %v", st, detail, err)
	}
	f.Close()
	d, err := s2.silo(ctx, "org", "A")
	if err != nil || d.state != SiloOK {
		t.Fatalf("open: %v", err)
	}
	if ok, err := tableExists(ctx, d.store.db, "Note__Note"); err != nil || !ok {
		t.Fatalf("Note in A's database: %v %v", ok, err)
	}
	if _, m, _, _ := schemaRow(ctx, d.store.db); m != "0002_silo_org" {
		t.Fatalf("row %q", m)
	}
	if _, err := d.store.db.ExecContext(ctx, `INSERT INTO "Note__Note" ("id", "org") VALUES ('n1', 'A')`); err != nil {
		t.Fatalf("a Note in A's database: %v", err)
	}
}

// A silo whose row has the app's hash but names an older migration (a later one changed none
// of its tables) is ok, and its row is brought to the build's last migration.
func TestStores_HashMatchRenamesRow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openApp(t, dir, 1)
	if _, err := s.silo(ctx, "org", "A"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	cfg := siloConfig(t, dir, 1)
	cfg.Migrations = append(cfg.Migrations, Migration{Name: "0002_names", From: cfg.Migrations[0].To, To: cfg.Migrations[0].To, SQL: `UPDATE "Org__Org" SET "name" = "name";`})
	s2, err := newStores(nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := SiloPath(dir, "org", "A")
	f, _ := OpenSQLite(p)
	defer f.Close()
	if st, _, err := s2.check(ctx, f, s2.silos["org"], false, true); err != nil || st != SiloOK {
		t.Fatalf("%s %v", st, err)
	}
	if _, m, _, _ := schemaRow(ctx, f.db); m != "0002_names" {
		t.Fatalf("row %q", m)
	}
}

// M4: a silo made before the app had migrations (its row names none) whose schema is where a
// migration leads is behind from there, not failed.
func TestStores_AdoptsMigrationless(t *testing.T) {
	dir := t.TempDir()
	cfg := siloConfig(t, dir, 1)
	cfg.Migrations = nil
	pre, _ := newStores(nil, cfg)
	p, _ := SiloPath(dir, "org", "old")
	os.MkdirAll(filepath.Dir(p), 0o755)
	f, _ := OpenSQLite(p)
	if st, _, err := pre.check(context.Background(), f, pre.silos["org"], false, true); err != nil || st != SiloOK {
		t.Fatalf("%s %v", st, err)
	}
	f.Close()
	s := openApp(t, dir, 2)
	if _, err := s.silo(context.Background(), "org", "old"); err != nil {
		t.Fatal(err)
	}
	f, _ = OpenSQLite(p)
	defer f.Close()
	if _, m, _, _ := schemaRow(context.Background(), f.db); m != "0002_note" {
		t.Fatalf("row %q", m)
	}
}
