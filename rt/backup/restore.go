package backup

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Restored is what Restore rebuilt.
type Restored struct {
	Generation string
	Segments   int       // the WAL segments applied
	At         time.Time // the time of the last thing applied (the snapshot's, or the last segment's)
}

// Restore rebuilds at dst (which must not exist) the database the bucket at loc held at time
// to (the zero time: the latest): the newest generation that began at or before to, its
// snapshot, then its WAL segments shipped at or before to, in order, each WAL's frames
// checkpointed into the file by SQLite itself.
func Restore(ctx context.Context, loc string, to time.Time, dst string) (Restored, error) {
	b, err := OpenBucket(loc)
	if err != nil {
		return Restored{}, err
	}
	return RestoreFrom(ctx, b, to, dst)
}

// RestoreFrom is Restore from an open bucket.
func RestoreFrom(ctx context.Context, b Bucket, to time.Time, dst string) (Restored, error) {
	if _, err := os.Stat(dst); err == nil {
		return Restored{}, fmt.Errorf("backup: %s exists: restore into a new file", dst)
	}
	keys, err := b.List(ctx, "generations/")
	if err != nil {
		return Restored{}, err
	}
	gens := map[string]time.Time{}
	for _, k := range keys {
		parts := strings.Split(k, "/")
		if len(parts) == 3 && parts[2] == "snapshot.db" {
			t, err := genStart(parts[1])
			if err == nil {
				gens[parts[1]] = t
			}
		}
	}
	// the segments of each generation, in order
	type seg struct {
		key string
		seq int
		at  time.Time
	}
	segsOf := map[string][]seg{}
	for _, k := range keys {
		parts := strings.Split(k, "/")
		if len(parts) != 4 || parts[2] != "wal" || !strings.HasSuffix(parts[3], ".wal") {
			continue
		}
		seqText, msText, _ := strings.Cut(strings.TrimSuffix(parts[3], ".wal"), "-")
		seq, err1 := strconv.Atoi(seqText)
		ms, err2 := strconv.ParseInt(msText, 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		segsOf[parts[1]] = append(segsOf[parts[1]], seg{k, seq, time.UnixMilli(ms)})
	}
	// each generation's last point at or before to: its snapshot, then its contiguous segments;
	// the generation whose point is latest wins (not the newest snapshot: a generation begun
	// earlier may have shipped later than one begun after it)
	var best string
	var bestAt time.Time
	var segs []seg
	for g, start := range gens {
		if !to.IsZero() && start.After(to) {
			continue
		}
		ss := segsOf[g]
		sort.Slice(ss, func(i, j int) bool { return ss[i].seq < ss[j].seq })
		at, n := start, 0
		for i, sg := range ss {
			if sg.seq != i || !to.IsZero() && sg.at.After(to) {
				break
			}
			at, n = sg.at, i+1
		}
		if best == "" || at.After(bestAt) || at.Equal(bestAt) && g > best {
			best, bestAt, segs = g, at, ss[:n]
		}
	}
	if best == "" {
		when := "yet"
		if !to.IsZero() {
			when = "at or before " + to.UTC().Format(time.RFC3339)
		}
		return Restored{}, fmt.Errorf("backup: %s holds no snapshot %s", b, when)
	}
	r := Restored{Generation: best, At: gens[best]}
	if err := b.Get(ctx, "generations/"+best+"/snapshot.db", dst); err != nil {
		return r, err
	}
	// apply the segments in order, one WAL (a run of segments with the same salts) at a time
	var wal []byte
	var salt [2]uint32
	flush := func() error {
		if wal == nil {
			return nil
		}
		err := applyWAL(ctx, dst, wal)
		wal = nil
		return err
	}
	tmp := dst + ".segment"
	defer os.Remove(tmp)
	for _, s := range segs {
		if err := b.Get(ctx, s.key, tmp); err != nil {
			return r, err
		}
		body, err := os.ReadFile(tmp)
		if err != nil {
			return r, err
		}
		h, ok := readWALHeader(body)
		if !ok {
			return r, fmt.Errorf("backup: %s is not a WAL segment", s.key)
		}
		if wal != nil && h.salt != salt {
			if err := flush(); err != nil {
				return r, err
			}
		}
		if wal == nil {
			wal, salt = append([]byte{}, body[:32]...), h.salt
		}
		wal = append(wal, body[32:]...)
		r.Segments++
		r.At = s.at
	}
	if err := flush(); err != nil {
		return r, err
	}
	if err := integrity(ctx, dst); err != nil {
		os.Remove(dst)
		return r, err
	}
	return r, nil
}

// integrity runs PRAGMA integrity_check on the restored file: anything but "ok" is refused.
func integrity(ctx context.Context, path string) error {
	d, err := open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	rows, err := d.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("backup: checking the restored database: %w", err)
	}
	defer rows.Close()
	var problems []string
	for rows.Next() {
		var line string
		rows.Scan(&line)
		if line != "ok" {
			problems = append(problems, line)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("backup: the restored database fails its integrity check (%s): nothing was written", strings.Join(problems[:min(len(problems), 3)], "; "))
	}
	return rows.Err()
}

// genStart is the time a generation's name begins with.
func genStart(gen string) (time.Time, error) {
	if len(gen) < len(genTime) {
		return time.Time{}, errors.New("short")
	}
	return time.Parse(genTime, gen[:len(genTime)])
}

// applyWAL makes wal (a header and frames) the database dst's WAL file and has SQLite
// checkpoint it into the database (TRUNCATE), leaving no WAL behind.
func applyWAL(ctx context.Context, dst string, wal []byte) error {
	os.Remove(dst + "-shm")
	if err := os.WriteFile(dst+"-wal", wal, 0o644); err != nil {
		return err
	}
	d, err := open(dst)
	if err != nil {
		return err
	}
	defer d.Close()
	var n, busy, logFrames, done, pages int
	// a read first: the connection opens the database, and with it the WAL (recovering it)
	if err := d.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master").Scan(&n); err != nil {
		return fmt.Errorf("backup: reading %s with its WAL: %w", filepath.Base(dst), err)
	}
	if err := d.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &done); err != nil {
		return fmt.Errorf("backup: applying the WAL to %s: %w", filepath.Base(dst), err)
	}
	if err := d.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return err
	}
	// the last commit frame records the database's size after it: the file now has that size
	h, _ := readWALHeader(wal)
	frame := int(24 + h.pageSize)
	want := 0
	for at := 32; at+frame <= len(wal); at += frame {
		if size := int(binary.BigEndian.Uint32(wal[at+4 : at+8])); size != 0 {
			want = size
		}
	}
	if busy != 0 || pages != want {
		return fmt.Errorf("backup: the WAL was not applied to %s (the database has %d pages, its last commit %d)", filepath.Base(dst), pages, want)
	}
	return nil
}
