package backup

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// AWS's documented Signature Version 4 example (S3, "GET Object"): the signature must match.
func TestSign_AWSExample(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	c := s3Creds{access: "AKIAIOSFODNN7EXAMPLE", secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", region: "us-east-1"}
	sign(req, c, emptySHA256, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization:\n%s\nwant\n%s", got, want)
	}
}

func TestOpenBucket(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_ENDPOINT_URL", "")
	b, err := OpenBucket("s3://my-bucket/blog/prod")
	if err != nil {
		t.Fatal(err)
	}
	s := b.(*s3Bucket)
	if s.String() != "s3://my-bucket/blog/prod" || s.endpoint != "https://my-bucket.s3.eu-west-1.amazonaws.com" || s.objectPath("x/y") != "/blog/prod/x/y" {
		t.Errorf("%+v %s", s, s.objectPath("x/y"))
	}
	t.Setenv("AWS_ENDPOINT_URL", "https://acct.r2.cloudflarestorage.com/")
	b, _ = OpenBucket("s3://my-bucket")
	if s := b.(*s3Bucket); s.objectPath("k") != "/my-bucket/k" || s.endpoint != "https://acct.r2.cloudflarestorage.com" {
		t.Errorf("path style: %+v", s)
	}
	if b, err := OpenBucket("/srv/backups"); err != nil || b.String() != "file:///srv/backups" {
		t.Errorf("%v %v", b, err)
	}
	if _, err := OpenBucket("backups"); err == nil {
		t.Error("a relative directory is accepted")
	}
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	if _, err := OpenBucket("s3://b"); err == nil || !strings.Contains(err.Error(), "AWS_SECRET_ACCESS_KEY") {
		t.Errorf("err = %v", err)
	}
}

// appDB opens path as the app does with the shipper on: WAL, no automatic checkpoint.
func appDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	d, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS "Note" ("id" INTEGER PRIMARY KEY, "body" TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	return d
}

func insert(t *testing.T, d *sql.DB, from, to int) {
	t.Helper()
	for i := from; i < to; i++ {
		if _, err := d.Exec(`INSERT INTO "Note" ("id", "body") VALUES (?, ?)`, i, fmt.Sprintf("note %d %s", i, strings.Repeat("x", 200))); err != nil {
			t.Fatal(err)
		}
	}
}

func count(t *testing.T, path string) int {
	t.Helper()
	d, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM "Note"`).Scan(&n); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var ok string
	d.QueryRow(`PRAGMA integrity_check`).Scan(&ok)
	if ok != "ok" {
		t.Errorf("%s: integrity_check %s", path, ok)
	}
	return n
}

// clock is a shipper's time, advanced by the test.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newShipper(t *testing.T, path string, b Bucket, c *clock) *Shipper {
	t.Helper()
	s, err := NewShipper(path, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.now = c.now
	t.Cleanup(func() { s.Close() })
	return s
}

func snapshots(t *testing.T, b Bucket) []string {
	t.Helper()
	keys, err := b.List(context.Background(), "generations/")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, k := range keys {
		if strings.HasSuffix(k, "/snapshot.db") {
			out = append(out, k)
		}
	}
	return out
}

// The round trip through a directory bucket: a snapshot, then WAL segments across WAL restarts
// (each after the shipper's own checkpoint: one generation throughout), restored whole and at
// points in time between the uploads.
func TestShip_RoundTripAndPointInTime(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	d := appDB(t, path)
	b := dirBucket{filepath.Join(dir, "bucket")}
	c := &clock{time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)}
	s := newShipper(t, path, b, c)

	insert(t, d, 0, 10)
	if err := s.Sync(ctx); err != nil { // the snapshot and the first segment
		t.Fatal(err)
	}
	var times []time.Time
	want := []int{10}
	times = append(times, c.t)
	for round, n := 1, 10; round <= 5; round++ {
		c.t = c.t.Add(time.Minute)
		insert(t, d, n, n+20*round)
		n += 20 * round
		if err := s.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		times = append(times, c.t)
		want = append(want, n)
	}
	if snaps := snapshots(t, b); len(snaps) != 1 {
		t.Errorf("the WAL restarts after the shipper's checkpoints began new generations: %v", snaps)
	}
	if st, err := os.ReadFile(path + "-backup.json"); err != nil || !strings.Contains(string(st), `"last":"2026-10-17T10:05:00Z"`) || !strings.Contains(string(st), `"url":"file://`) {
		t.Errorf("status: %s %v", st, err)
	}
	r, err := RestoreFrom(ctx, b, time.Time{}, filepath.Join(dir, "latest.db"))
	if err != nil {
		if os.Getenv("DBG") != "" {
			exec.Command("cp", "-R", dir, os.Getenv("DBG")).Run()
		}
		t.Fatal(err)
	}
	if n := count(t, filepath.Join(dir, "latest.db")); n != want[len(want)-1] {
		t.Errorf("the latest restore has %d notes, want %d (%+v)", n, want[len(want)-1], r)
	}
	for i, at := range times {
		dst := filepath.Join(dir, fmt.Sprintf("at%d.db", i))
		if _, err := RestoreFrom(ctx, b, at.Add(30*time.Second), dst); err != nil {
			t.Fatal(err)
		}
		if n := count(t, dst); n != want[i] {
			t.Errorf("restored at %s: %d notes, want %d", at.Add(30*time.Second).Format(time.TimeOnly), n, want[i])
		}
	}
	if _, err := RestoreFrom(ctx, b, times[0].Add(-time.Hour), filepath.Join(dir, "before.db")); err == nil || !strings.Contains(err.Error(), "holds no snapshot at or before") {
		t.Errorf("before the first snapshot: %v", err)
	}
	if _, err := RestoreFrom(ctx, b, time.Time{}, filepath.Join(dir, "latest.db")); err == nil {
		t.Error("restored over an existing file")
	}
}

// A checkpoint the shipper did not make (a process without ALDER_BACKUP_URL) loses frames it had
// not shipped: the shipper notices the old WAL's frames past what it shipped and begins a new
// generation, so the restore is still whole.
func TestShip_ForeignCheckpoint(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	d := appDB(t, path)
	b := dirBucket{filepath.Join(dir, "bucket")}
	c := &clock{time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)}
	s := newShipper(t, path, b, c)
	insert(t, d, 0, 5)
	// a reader holding a snapshot in the WAL keeps the writes after the shipper's checkpoint
	// from restarting the WAL: they append after what was shipped
	reader, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var n int
	reader.QueryRow(`SELECT count(*) FROM "Note"`).Scan(&n)
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	insert(t, d, 5, 60) // not shipped
	reader.Rollback()
	if _, err := d.Exec(`PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
		t.Fatal(err)
	}
	insert(t, d, 60, 61) // restarts the WAL (salt-1 + 1) over the frames of 5..59
	c.t = c.t.Add(time.Minute)
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if snaps := snapshots(t, b); len(snaps) != 2 {
		t.Errorf("snapshots %v: want a new generation", snaps)
	}
	if _, err := RestoreFrom(ctx, b, time.Time{}, filepath.Join(dir, "r.db")); err != nil {
		t.Fatal(err)
	}
	if n := count(t, filepath.Join(dir, "r.db")); n != 61 {
		t.Errorf("restored %d notes, want 61", n)
	}
}

// Start ships at its interval and once more when stopped.
func TestStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	d := appDB(t, path)
	insert(t, d, 0, 3)
	stop, err := Start(path, "file://"+filepath.Join(dir, "bucket"), 20*time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	insert(t, d, 3, 7)
	stop()
	if _, err := Restore(context.Background(), filepath.Join(dir, "bucket"), time.Time{}, filepath.Join(dir, "r.db")); err != nil {
		t.Fatal(err)
	}
	if n := count(t, filepath.Join(dir, "r.db")); n != 7 {
		t.Errorf("restored %d notes, want 7", n)
	}
}

// Snapshot is a consistent copy taken while the database is open elsewhere.
func TestSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	d := appDB(t, path)
	insert(t, d, 0, 4)
	if err := Snapshot(context.Background(), path, filepath.Join(dir, "copy.db")); err != nil {
		t.Fatal(err)
	}
	if n := count(t, filepath.Join(dir, "copy.db")); n != 4 {
		t.Errorf("the copy has %d notes", n)
	}
	if err := Snapshot(context.Background(), filepath.Join(dir, "none.db"), filepath.Join(dir, "x.db")); !os.IsNotExist(err) {
		t.Errorf("a missing database: %v", err)
	}
}
