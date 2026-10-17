package backup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// EnvURL turns the shipper on: the generated main starts Ship when it is set, and the
// database's connections leave checkpoints to the shipper (rt/db: wal_autocheckpoint(0)).
const EnvURL = "ALDER_BACKUP_URL"

// EnvInterval is how often the shipper uploads (a duration; default DefaultInterval).
const EnvInterval = "ALDER_BACKUP_INTERVAL"

// DefaultInterval is how often the shipper uploads without EnvInterval.
const DefaultInterval = 10 * time.Second

// The bucket's layout. A generation is a snapshot of the database and the WAL segments after
// it; every start of the app begins one (and so does a gap the shipper cannot bridge):
//
//	generations/<gen>/snapshot.db             the database file when the generation began
//	generations/<gen>/wal/<seq>-<ms>.wal      a WAL header and the frames after the previous
//	                                          segment of the same WAL (seq 00000000 is the WAL
//	                                          as it stood with the snapshot), shipped at <ms>
//
// <gen> is the generation's start, 20261017T101500.000Z, and 4 random hex digits.
const genTime = "20060102T150405.000Z"

// Status is what the shipper writes next to the database (<db>-backup.json) after each upload:
// `alder status` reads it.
type Status struct {
	Last       time.Time `json:"last"`
	URL        string    `json:"url"`
	Generation string    `json:"generation"`
}

// Shipper streams a database to a Bucket: Litestream's design, a small subset. Every interval
// it takes the write lock (BEGIN IMMEDIATE: writers wait, readers do not), copies the WAL frames
// committed since the last upload, runs a PASSIVE checkpoint, releases the lock and uploads the
// frames as a segment. Only the shipper checkpoints (the app's connections run with
// wal_autocheckpoint(0) while it is on), and only right after copying, so a WAL that restarts
// has had every frame shipped; a restart it did not cause, or a failed upload, begins a new
// generation with a snapshot.
type Shipper struct {
	path   string
	bucket Bucket
	log    func(level, msg string)
	now    func() time.Time

	db       *sql.DB
	gen      string
	seq      int
	salt     [2]uint32 // the WAL whose frames are being shipped
	haveWAL  bool
	offset   int64 // bytes of that WAL shipped
	restarts bool  // the last checkpoint backfilled everything shipped: the next WAL continues the generation
	fresh    bool  // a new generation is due

	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

// NewShipper is the shipper of the database at path to bucket (log may be nil).
func NewShipper(path string, bucket Bucket, log func(level, msg string)) (*Shipper, error) {
	if log == nil {
		log = func(string, string) {}
	}
	d, err := open(path)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(2) // one holds the write lock while the other checkpoints
	return &Shipper{path: path, bucket: bucket, log: log, now: time.Now, db: d, fresh: true}, nil
}

// Start opens the location loc and ships the database at path every interval until the
// returned stop is called (which ships once more and closes). A database that does not exist
// yet is waited for.
func Start(path, loc string, interval time.Duration, log func(level, msg string)) (stop func(), err error) {
	b, err := OpenBucket(loc)
	if err != nil {
		return nil, err
	}
	s, err := NewShipper(path, b, log)
	if err != nil {
		return nil, err
	}
	if interval <= 0 {
		interval = DefaultInterval
	}
	s.stop, s.done = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(s.done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			s.tick()
			select {
			case <-s.stop:
				s.tick()
				s.db.Close()
				return
			case <-t.C:
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(s.stop); <-s.done })
	}, nil
}

func (s *Shipper) tick() {
	if _, err := os.Stat(s.path); err != nil {
		return // no database yet
	}
	if err := s.Sync(context.Background()); err != nil {
		s.log("error", "backup: "+err.Error())
	}
}

// walHeader is the 32-byte header of a WAL file.
type walHeader struct {
	pageSize uint32
	salt     [2]uint32
}

func readWALHeader(b []byte) (walHeader, bool) {
	if len(b) < 32 {
		return walHeader{}, false
	}
	magic := binary.BigEndian.Uint32(b[0:4])
	if magic != 0x377f0682 && magic != 0x377f0683 {
		return walHeader{}, false
	}
	h := walHeader{pageSize: binary.BigEndian.Uint32(b[8:12]), salt: [2]uint32{binary.BigEndian.Uint32(b[16:20]), binary.BigEndian.Uint32(b[20:24])}}
	if h.pageSize == 1 {
		h.pageSize = 65536
	}
	return h, h.pageSize >= 512
}

// committedEnd is the end of the last commit frame of wal's frames from offset on that belong
// to h (their salts are h's), or offset when there is none.
func committedEnd(wal []byte, h walHeader, offset int64) int64 {
	frame := int64(24 + h.pageSize)
	end := offset
	for at := offset; at+frame <= int64(len(wal)); at += frame {
		fh := wal[at : at+24]
		if binary.BigEndian.Uint32(fh[8:12]) != h.salt[0] || binary.BigEndian.Uint32(fh[12:16]) != h.salt[1] {
			break
		}
		if binary.BigEndian.Uint32(fh[4:8]) != 0 { // the database's size after a commit
			end = at + frame
		}
	}
	return end
}

// Sync ships what was committed since the last call: under the write lock it copies the new WAL
// frames (and, for a new generation, the database file), checkpoints, releases the lock, and
// uploads.
func (s *Shipper) Sync(ctx context.Context) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := lock.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("taking the write lock: %w", err)
	}
	locked := true
	unlock := func() {
		if locked {
			lock.ExecContext(context.Background(), "ROLLBACK")
			locked = false
		}
	}
	defer unlock()

	wal, err := os.ReadFile(s.path + "-wal")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	h, haveWAL := readWALHeader(wal)
	if haveWAL && s.haveWAL && h.salt != s.salt {
		// the WAL restarted: a continuation only right after a checkpoint of ours backfilled
		// everything shipped (salt-1 is the old one plus one: exactly one restart)
		if !s.restarts || h.salt[0] != s.salt[0]+1 {
			s.fresh = true
		}
		// frames of the old WAL past what was shipped, still in the file beyond the new WAL's:
		// someone else checkpointed them (a process without ALDER_BACKUP_URL)
		frame := int64(24 + h.pageSize)
		if newEnd := committedEnd(wal, h, 32); !s.fresh && newEnd <= s.offset && s.offset+frame <= int64(len(wal)) {
			fh := wal[s.offset : s.offset+24]
			if binary.BigEndian.Uint32(fh[8:12]) == s.salt[0] && binary.BigEndian.Uint32(fh[12:16]) == s.salt[1] {
				s.fresh = true
			}
		}
		s.offset = 32
	}
	if haveWAL && !s.haveWAL {
		s.offset = 32
	}
	var snapshot string
	if s.fresh {
		f, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".snapshot-*")
		if err != nil {
			return err
		}
		snapshot = f.Name()
		defer os.Remove(snapshot)
		db, err := os.Open(s.path)
		if err != nil {
			f.Close()
			return err
		}
		_, err = f.ReadFrom(db)
		db.Close()
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		s.offset = 32
	}
	var segment []byte
	end := s.offset
	if haveWAL {
		end = committedEnd(wal, h, s.offset)
		if end > s.offset {
			segment = append(append([]byte{}, wal[:32]...), wal[s.offset:end]...)
		}
	}
	// checkpoint what was just copied (the lock keeps writers out, so that is the whole WAL)
	var busy, logFrames, done int
	if haveWAL {
		if err := s.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &logFrames, &done); err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
	}
	unlock()

	now := s.now().UTC()
	if s.fresh {
		var r [2]byte
		rand.Read(r[:])
		gen := now.Format(genTime) + "-" + hex.EncodeToString(r[:])
		if err := s.bucket.Put(ctx, "generations/"+gen+"/snapshot.db", snapshot); err != nil {
			return err // still fresh: the next Sync tries again
		}
		s.gen, s.seq, s.fresh = gen, 0, false
		s.log("info", "backup: generation "+gen+" (snapshot) at "+s.bucket.String())
	}
	if segment != nil {
		if err := s.put(ctx, now, segment); err != nil {
			s.fresh = true // the frames were checkpointed: only a new snapshot bridges the gap
			return err
		}
	}
	s.haveWAL, s.salt, s.offset = haveWAL, h.salt, end
	s.restarts = haveWAL && busy == 0 && logFrames == done && int64(32+logFrames*int(24+h.pageSize)) == end
	return s.writeStatus(now)
}

// put uploads segment as the generation's next.
func (s *Shipper) put(ctx context.Context, now time.Time, segment []byte) error {
	f, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".segment-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(segment)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	key := fmt.Sprintf("generations/%s/wal/%08d-%013d.wal", s.gen, s.seq, now.UnixMilli())
	if err := s.bucket.Put(ctx, key, f.Name()); err != nil {
		return err
	}
	s.seq++
	return nil
}

func (s *Shipper) writeStatus(now time.Time) error {
	b, _ := json.Marshal(Status{Last: now, URL: s.bucket.String(), Generation: s.gen})
	tmp := s.path + "-backup.json.tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path+"-backup.json")
}

// Close closes the shipper's connections.
func (s *Shipper) Close() error { return s.db.Close() }

// IntervalFromEnv is EnvInterval's duration, or DefaultInterval.
func IntervalFromEnv() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(EnvInterval)); err == nil && d > 0 {
		return d
	}
	if n, err := strconv.Atoi(os.Getenv(EnvInterval)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return DefaultInterval
}
