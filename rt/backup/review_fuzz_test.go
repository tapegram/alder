package backup

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"
)

// Adopted from the Task 2 review: random interleavings of readers holding snapshots, writes,
// shipments and the handoff from the old build's shipper to the new build's (one bucket, as in
// production; the new one starts once the old one has exited). After every shipment the
// restore must be whole and pass its integrity check.
func TestReview_TwoShippersFuzz(t *testing.T) {
	ctx := context.Background()
	seeds := 300
	if testing.Short() {
		seeds = 40
	}
	bad := map[string]int{}
	for seed := 0; seed < seeds; seed++ {
		rng := rand.New(rand.NewSource(int64(seed)))
		dir := t.TempDir()
		path := filepath.Join(dir, "app.db")
		d := appDB(t, path)
		c := &clock{time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)}
		bucket := dirBucket{filepath.Join(dir, "bucket")}
		cur := newShipper(t, path, bucket, c)
		handed := false
		n := 0
		var reader *sql.Tx
		for step := 0; step < 30; step++ {
			c.t = c.t.Add(time.Second)
			switch k := rng.Intn(6); {
			case k == 0 && reader == nil:
				reader, _ = d.Begin()
				var x int
				reader.QueryRow(`SELECT count(*) FROM "Note"`).Scan(&x)
			case k == 1 && reader != nil:
				reader.Rollback()
				reader = nil
			case k == 2:
				m := 1 + rng.Intn(300)
				insert(t, d, n, n+m)
				n += m
			case k == 3 && !handed && step >= 3:
				if err := cur.Sync(ctx); err != nil { // the old build's last shipment
					t.Fatal(err)
				}
				cur.Close()
				cur = newShipper(t, path, bucket, c)
				handed = true
			default:
				if err := cur.Sync(ctx); err != nil {
					t.Fatal(err)
				}
				dst := filepath.Join(dir, fmt.Sprintf("r%d.db", step))
				if _, err := RestoreFrom(ctx, bucket, time.Time{}, dst); err != nil {
					bad["restore error: "+err.Error()]++
					continue
				}
				r, _ := sql.Open("sqlite", dst)
				var cnt int
				var ok string
				r.QueryRow(`PRAGMA integrity_check`).Scan(&ok)
				err := r.QueryRow(`SELECT count(*) FROM "Note"`).Scan(&cnt)
				r.Close()
				if ok != "ok" || err != nil {
					bad["corrupt"]++
				} else if cnt != n {
					bad[fmt.Sprintf("wrong count (seed %d)", seed)]++
				}
			}
		}
		if reader != nil {
			reader.Rollback()
		}
	}
	if len(bad) > 0 {
		t.Errorf("failures over %d runs: %v", seeds, bad)
	}
}
