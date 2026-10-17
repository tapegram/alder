package backup

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The silos beside the database are shipped one by one, each under silos/<key>/<value>/, and
// one silo is restored from its own stream; the shared database's restore does not see them.
func TestShip_SilosPerFile(t *testing.T) {
	dir, bucket := t.TempDir(), t.TempDir()
	shared := filepath.Join(dir, "app.db")
	a := filepath.Join(dir, "silos", "org", "A.db")
	b := filepath.Join(dir, "silos", "org", "B.db")
	os.MkdirAll(filepath.Dir(a), 0o755)
	ds, da, db := appDB(t, shared), appDB(t, a), appDB(t, b)
	insert(t, ds, 0, 3)
	insert(t, da, 0, 5)
	insert(t, db, 100, 101)
	stop, err := Start(shared, bucket, 50*time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	insert(t, da, 5, 8)
	time.Sleep(200 * time.Millisecond)
	stop()
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "a.db")
	if _, err := RestoreSilo(ctx, bucket, "org", "A", time.Time{}, out); err != nil {
		t.Fatal(err)
	}
	if n := count(t, out); n != 8 {
		t.Fatalf("silo A restored with %d rows, want 8", n)
	}
	outB := filepath.Join(t.TempDir(), "b.db")
	if _, err := RestoreSilo(ctx, bucket, "org", "B", time.Time{}, outB); err != nil || count(t, outB) != 1 {
		t.Fatalf("silo B: %v", err)
	}
	outS := filepath.Join(t.TempDir(), "s.db")
	if _, err := Restore(ctx, bucket, time.Time{}, outS); err != nil || count(t, outS) != 3 {
		t.Fatalf("the shared database: %v", err)
	}
	if _, err := os.Stat(a + "-backup.json"); err != nil {
		t.Fatalf("silo A's status file: %v", err)
	}
}

// Past MaxSilos files, no silo is shipped and the log says so once, pointing at Turso.
func TestShip_SilosBound(t *testing.T) {
	dir, bucket := t.TempDir(), t.TempDir()
	shared := filepath.Join(dir, "app.db")
	appDB(t, shared)
	os.MkdirAll(filepath.Join(dir, "silos", "org"), 0o755)
	for i := 0; i <= MaxSilos; i++ {
		os.WriteFile(filepath.Join(dir, "silos", "org", "s"+itoa(i)+".db"), nil, 0o600)
	}
	var mu sync.Mutex
	var logs []string
	stop, err := Start(shared, bucket, 50*time.Millisecond, func(level, msg string) {
		mu.Lock()
		logs = append(logs, level+": "+msg)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	stop()
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for _, l := range logs {
		if strings.Contains(l, "more than the 200 the shipper streams: none of the silos is shipped") && strings.Contains(l, "Turso") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the bound was logged %d times:\n%s", n, strings.Join(logs, "\n"))
	}
	if keys, _ := (dirBucket{bucket}).List(context.Background(), "silos/"); len(keys) != 0 {
		t.Fatalf("silos shipped: %v", keys)
	}
}

// A database on Turso is not shipped: Start does nothing and says why.
func TestShip_TursoNotShipped(t *testing.T) {
	var said string
	stop, err := Start("libsql://blog-acme.turso.io", "/nowhere", time.Second, func(_, msg string) { said = msg })
	if err != nil || !strings.Contains(said, "point-in-time recovery") {
		t.Fatalf("%v %q", err, said)
	}
	stop()
}

func itoa(i int) string { return strconv.Itoa(i) }
