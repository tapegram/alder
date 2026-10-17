package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Adopted from the Task 2 review: two shippers on one database (the old and the new build
// during the drain). Only one runs: the second waits for the database's backup lock, and the
// handoff loses nothing.
func TestReview_TwoShippers(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	d := appDB(t, path)
	bucket := dirBucket{filepath.Join(dir, "bucket")}
	c := &clock{time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)}
	a := newShipper(t, path, bucket, c)
	insert(t, d, 0, 5)
	reader, _ := d.Begin()
	var n int
	reader.QueryRow(`SELECT count(*) FROM "Note"`).Scan(&n)
	if err := a.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	insert(t, d, 5, 60) // appended after A's offset (the reader keeps the WAL from restarting)
	reader.Rollback()
	if _, err := NewShipper(path, bucket, nil); !errors.Is(err, ErrShipperRunning) {
		t.Fatalf("a second shipper started while the first runs: %v", err)
	}
	c.t = c.t.Add(time.Second)
	if err := a.Sync(ctx); err != nil { // A's last shipment, then it exits
		t.Fatal(err)
	}
	a.Close()
	b := newShipper(t, path, bucket, c)
	insert(t, d, 60, 400)
	c.t = c.t.Add(time.Minute)
	if err := b.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreFrom(ctx, bucket, time.Time{}, filepath.Join(dir, "r.db")); err != nil {
		t.Fatal(err)
	}
	if got := count(t, filepath.Join(dir, "r.db")); got != 400 {
		t.Errorf("restored %d notes, live has 400", got)
	}
	// a point between A's last shipment and B's: A's generation, which shipped later than B began
	if _, err := RestoreFrom(ctx, bucket, c.t.Add(-30*time.Second), filepath.Join(dir, "mid.db")); err != nil {
		t.Fatal(err)
	}
	if got := count(t, filepath.Join(dir, "mid.db")); got != 60 {
		t.Errorf("restored %d notes at A's last shipment, want 60", got)
	}
}

type failBucket struct {
	Bucket
	fail *bool
	puts *int
}

func (f failBucket) Put(ctx context.Context, key, path string) error {
	*f.puts++
	if *f.fail {
		return errors.New("s3 down")
	}
	return f.Bucket.Put(ctx, key, path)
}

// Adopted from the Task 2 review: while every upload fails the WAL stays small (the shipper
// still checkpoints, queueing the frames), no new snapshot is taken per failed tick, attempts
// back off, and once the bucket answers the queue drains and the restore is whole.
func TestReview_WALWhileUploadsFail(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	d := appDB(t, path)
	c := &clock{time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)}
	down, puts := true, 0
	inner := dirBucket{filepath.Join(dir, "bucket")}
	s := newShipper(t, path, failBucket{inner, &down, &puts}, c)
	for round := 0; round < 20; round++ {
		c.t = c.t.Add(10 * time.Second)
		insert(t, d, round*200, round*200+200)
		s.Sync(ctx)
		if st, _ := os.Stat(path + "-wal"); st != nil && st.Size() > 4<<20 {
			t.Fatalf("round %d: the WAL grew to %d bytes while uploads fail", round, st.Size())
		}
	}
	if puts >= 20 {
		t.Errorf("%d upload attempts in 20 ticks: no backoff", puts)
	}
	snaps := 0
	for _, p := range s.queue {
		if filepath.Base(p.key) == "snapshot.db" {
			snaps++
		}
	}
	if snaps != 1 {
		t.Errorf("%d snapshots queued, want the generation's one", snaps)
	}
	down = false
	c.t = c.t.Add(10 * time.Minute)
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(s.queue) != 0 {
		t.Errorf("%d files still queued", len(s.queue))
	}
	if _, err := RestoreFrom(ctx, inner, time.Time{}, filepath.Join(dir, "r.db")); err != nil {
		t.Fatal(err)
	}
	if got := count(t, filepath.Join(dir, "r.db")); got != 4000 {
		t.Errorf("restored %d notes, want 4000", got)
	}
}

// A new generation of a large database holds the write lock only to copy the WAL: a writer
// that waits on it is not held for the database file's copy.
func TestShip_FreshGenerationLockIsShort(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	d := appDB(t, path)
	insert(t, d, 0, 60000) // ~15 MB
	if _, err := d.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	slow := slowBucket{dirBucket{filepath.Join(dir, "bucket")}}
	s, err := NewShipper(path, slow, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	done := make(chan struct{})
	go func() { s.Sync(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond) // the copy and the slow upload are under way
	t0 := time.Now()
	insert(t, d, 60000, 60001)
	if waited := time.Since(t0); waited > 300*time.Millisecond {
		t.Errorf("a write waited %s during a new generation", waited)
	}
	<-done
}

type slowBucket struct{ Bucket }

func (b slowBucket) Put(ctx context.Context, key, path string) error {
	time.Sleep(time.Second)
	return b.Bucket.Put(ctx, key, path)
}
