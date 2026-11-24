package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SiloDBs lists the files of the app's keys (not those under a key it does not declare), opens
// each as a reader or a writer, and tells the orphaned ones from cfg.Shared's key table.
func TestSiloDBs_Files(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	shared := filepath.Join(dir, "app.db")
	st, err := OpenSQLite(shared)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Ensure(ctx, []TableDesc{orgDesc}, false); err != nil {
		t.Fatal(err)
	}
	a, err := st.Insert(ctx, nil, "Org.Org", orgDesc.Columns, "id", "", "", []any{nil, "A"}, clock)
	if err != nil {
		t.Fatal(err)
	}
	b := "bbbb"
	for _, v := range []struct{ key, value string }{{"org", a}, {"org", b}, {"other", "cccc"}} {
		p, err := SiloPath(dir, v.key, v.value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := OpenSQLite(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	cfg := StoresConfig{Dir: dir, Shared: shared, Silos: []Silo{{Key: "org", Ref: "Org.Org"}}, Tables: []TableDesc{orgDesc}}
	dbs, err := SiloDBs(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(dbs) != 2 {
		t.Fatalf("%d silo databases, want 2: %+v", len(dbs), dbs)
	}
	got := map[string]SiloDB{}
	for _, d := range dbs {
		if d.Key != "org" {
			t.Fatalf("key %q", d.Key)
		}
		got[d.Value] = d
	}
	if got[a].Orphaned || !got[b].Orphaned {
		t.Fatalf("orphaned: %s %v, %s %v; want false, true", a, got[a].Orphaned, b, got[b].Orphaned)
	}
	r, err := got[a].Open(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.db.ExecContext(ctx, `INSERT INTO t (x) VALUES (1)`); err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("the reader's insert: %v, want readonly", err)
	}
	if state, detail, err := got[a].Check(ctx, r); err != nil || state != SiloFailed { // a table of no runtime's
		t.Fatalf("check: %s %q %v", state, detail, err)
	}
}

func TestSiloDBs_NoSilos(t *testing.T) {
	dbs, err := SiloDBs(context.Background(), StoresConfig{Shared: filepath.Join(t.TempDir(), "app.db")})
	if dbs != nil || err != nil {
		t.Fatalf("%v, %v; want nil, nil", dbs, err)
	}
}

func TestJobsSchemaOf(t *testing.T) {
	ctx := context.Background()
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if v, err := JobsSchemaOf(ctx, st.db); v != "" || err != nil {
		t.Fatalf("a fresh file: %q, %v; want empty", v, err)
	}
	if err := EnsureJobsTable(ctx, st.db); err != nil {
		t.Fatal(err)
	}
	if v, err := JobsSchemaOf(ctx, st.db); v != JobsSchema || err != nil {
		t.Fatalf("after EnsureJobsTable: %q, %v; want %q", v, err, JobsSchema)
	}
}
