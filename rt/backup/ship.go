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

// Shipper streams a database to a Bucket: Litestream's design, a small subset. One shipper per
// database (an exclusive lock on <db>-backup.lock: the new build's waits for the old build's to
// exit). Every interval it takes the write lock (BEGIN IMMEDIATE: writers wait, readers do not)
// only to copy the WAL frames committed since the last tick and run a PASSIVE checkpoint, then
// releases it and uploads the frames as a segment. Only the shipper checkpoints (the app's
// connections run with wal_autocheckpoint(0) while it is on), and only right after copying, so
// a WAL that restarts has had every frame copied; a restart it did not cause begins a new
// generation, whose snapshot is the database file copied outside the write lock (the file
// does not change until the shipper's next checkpoint). Uploads that fail are queued on disk
// and retried in order with exponential backoff; only WAL segments past the queue's cap begin a
// new generation (the generation's snapshot, and the WAL that stands with it, never count
// against the cap: a database larger than the cap is shipped like any other).
//
// The snapshot is a raw copy of the database file, taken inside a read transaction: correct
// because only the shipper checkpoints. A process that checkpoints the file meanwhile (an
// operator's sqlite3 with its default automatic checkpoint, a build started without
// ALDER_BACKUP_URL) can tear that copy; the restore's integrity check then refuses it, and the
// next generation is whole again.
//
// Close drops what is still queued: a WAL segment that failed to upload before the build
// stopped is not in the bucket. Its frames are in the database file by then (the shipper
// checkpointed them), and the next build's shipper begins a new generation whose snapshot
// holds them, so the bucket loses no data, only the points in time of that gap.
type Shipper struct {
	path   string
	bucket Bucket
	log    func(level, msg string)
	now    func() time.Time

	db       *sql.DB
	lock     *os.File
	gen      string
	seq      int
	salt     [2]uint32 // the WAL whose frames are being shipped
	haveWAL  bool
	offset   int64 // bytes of that WAL copied
	restarts bool  // the last checkpoint backfilled everything copied: the next WAL continues the generation
	fresh    bool  // a new generation is due

	queue    []pending // copied, not uploaded yet, in order
	queued   int64
	walBytes int64 // the part of queued that is WAL segments: what the cap counts
	failures int
	nextTry  time.Time
	maxQueue int64
	interval time.Duration

	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

// pending is a file waiting in the upload queue.
type pending struct {
	key, path string
	at        time.Time
	size      int64
	base      bool // the generation's snapshot, or its first segment: not counted by the cap
}

// ErrShipperRunning is NewShipper's error while another shipper (another process, or another
// Shipper in this one) holds the database's backup lock.
var ErrShipperRunning = errors.New("another backup shipper runs for this database")

// DefaultMaxQueue caps the WAL segments' bytes waiting for upload while the bucket fails; past it
// the queue is dropped and a new generation begins when uploads work again. A generation's
// snapshot is not counted: it is the database's size, whatever that is.
const DefaultMaxQueue = 256 << 20

// maxBackoff bounds the wait between failed upload attempts.
const maxBackoff = 5 * time.Minute

// NewShipper is the shipper of the database at path to bucket (log may be nil). It takes the
// database's backup lock, or returns ErrShipperRunning.
func NewShipper(path string, bucket Bucket, log func(level, msg string)) (*Shipper, error) {
	if log == nil {
		log = func(string, string) {}
	}
	lf, err := os.OpenFile(path+"-backup.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := tryLock(lf); err != nil {
		lf.Close()
		return nil, ErrShipperRunning
	}
	d, err := open(path)
	if err != nil {
		unlockFile(lf)
		lf.Close()
		return nil, err
	}
	d.SetMaxOpenConns(2) // one holds the write lock while the other checkpoints
	return &Shipper{path: path, bucket: bucket, log: log, now: time.Now, db: d, lock: lf, fresh: true, maxQueue: DefaultMaxQueue, interval: DefaultInterval}, nil
}

// Start opens the location loc and ships the database at path every interval until the
// returned stop is called (which ships once more and closes). A database that does not exist
// yet is waited for, and so is the shipper of another process (the build being replaced).
func Start(path, loc string, interval time.Duration, log func(level, msg string)) (stop func(), err error) {
	b, err := OpenBucket(loc)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = func(string, string) {}
	}
	if interval <= 0 {
		interval = DefaultInterval
	}
	stopc, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var s *Shipper
		for waited := false; s == nil; {
			var err error
			s, err = NewShipper(path, b, log)
			if errors.Is(err, ErrShipperRunning) {
				if !waited {
					log("info", "backup: waiting for the shipper of the build being replaced to exit")
					waited = true
				}
			} else if err != nil {
				log("error", "backup: "+err.Error())
			}
			if s == nil {
				select {
				case <-stopc:
					return
				case <-time.After(time.Second):
				}
			}
		}
		s.interval = interval
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			s.tick()
			select {
			case <-stopc:
				s.nextTry = time.Time{} // the last chance: try the queue now
				s.tick()
				s.Close()
				return
			case <-t.C:
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(stopc); <-done })
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

// Sync copies what was committed since the last call (under the write lock: the WAL frames and
// a checkpoint), then, for a new generation, the database file (outside the lock), queues them
// and uploads the queue unless a failed upload's backoff is running.
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
		// everything copied (salt-1 is the old one plus one: exactly one restart)
		if !s.restarts || h.salt[0] != s.salt[0]+1 {
			s.fresh = true
		}
		// frames of the old WAL past what was copied, still in the file beyond the new WAL's:
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
	if haveWAL && !s.haveWAL || s.fresh {
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
	s.haveWAL, s.salt, s.offset = haveWAL, h.salt, end
	s.restarts = haveWAL && busy == 0 && logFrames == done && int64(32+logFrames*int(24+h.pageSize)) == end

	now := s.now().UTC()
	backingOff := now.Before(s.nextTry)
	switch {
	case s.fresh && backingOff:
		// the frames are in the database file now: the generation's snapshot, when uploads
		// work again, has them
	case s.fresh:
		if err := s.begin(ctx, now, segment); err != nil {
			return err
		}
	case segment != nil:
		key := fmt.Sprintf("generations/%s/wal/%08d-%013d.wal", s.gen, s.seq, now.UnixMilli())
		if err := s.enqueue(key, now, segment, "", false); err != nil {
			return err
		}
		s.seq++
	}
	if s.walBytes > s.maxQueue {
		s.log("error", fmt.Sprintf("backup: %d bytes of WAL segments wait for %s; dropping them: a new generation begins when uploads work again", s.walBytes, s.bucket))
		s.dropQueue()
		s.fresh = true
		return nil
	}
	if backingOff {
		return nil
	}
	return s.flush(ctx, now)
}

// begin starts a new generation: the database file, copied now (outside the write lock, inside a
// read transaction: only the shipper's next checkpoint could change it), and segment as its
// first WAL segment, queued.
func (s *Shipper) begin(ctx context.Context, now time.Time, segment []byte) error {
	s.dropQueue() // a generation that did not finish uploading: its uploaded part still restores
	reader, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()
	if _, err := reader.ExecContext(ctx, "BEGIN"); err == nil {
		var n int
		reader.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master").Scan(&n)
		defer reader.ExecContext(context.Background(), "ROLLBACK")
	}
	var r [2]byte
	rand.Read(r[:])
	gen := now.Format(genTime) + "-" + hex.EncodeToString(r[:])
	if err := s.enqueue("generations/"+gen+"/snapshot.db", now, nil, s.path, true); err != nil {
		return err
	}
	s.gen, s.seq, s.fresh = gen, 0, false
	if segment != nil { // the WAL as it stands with the snapshot: part of the generation's base
		key := fmt.Sprintf("generations/%s/wal/%08d-%013d.wal", s.gen, s.seq, now.UnixMilli())
		if err := s.enqueue(key, now, segment, "", true); err != nil {
			return err
		}
		s.seq++
	}
	s.log("info", "backup: generation "+gen+" at "+s.bucket.String())
	return nil
}

// enqueue writes body (or a copy of the file at from) to a queue file uploaded as key; base is a
// generation's snapshot or its first segment, which the queue's cap does not count.
func (s *Shipper) enqueue(key string, at time.Time, body []byte, from string, base bool) error {
	f, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".backup-queue-*")
	if err != nil {
		return err
	}
	if from != "" {
		var src *os.File
		if src, err = os.Open(from); err == nil {
			_, err = f.ReadFrom(src)
			src.Close()
		}
	} else {
		_, err = f.Write(body)
	}
	fi, serr := f.Stat()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = serr
	}
	if err != nil {
		os.Remove(f.Name())
		return err
	}
	s.queue = append(s.queue, pending{key: key, path: f.Name(), at: at, size: fi.Size(), base: base})
	s.queued += fi.Size()
	if !base {
		s.walBytes += fi.Size()
	}
	return nil
}

// flush uploads the queue in order; a failure leaves the rest queued and backs off
// (interval × 2^failures, at most maxBackoff).
func (s *Shipper) flush(ctx context.Context, now time.Time) error {
	for len(s.queue) > 0 {
		p := s.queue[0]
		if err := s.bucket.Put(ctx, p.key, p.path); err != nil {
			s.failures++
			wait := s.interval << min(s.failures, 10)
			if wait <= 0 || wait > maxBackoff {
				wait = maxBackoff
			}
			s.nextTry = now.Add(wait)
			return fmt.Errorf("%w (%d bytes queued; next try in %s)", err, s.queued, wait)
		}
		os.Remove(p.path)
		s.queued -= p.size
		if !p.base {
			s.walBytes -= p.size
		}
		s.queue = s.queue[1:]
	}
	s.failures, s.nextTry = 0, time.Time{}
	return s.writeStatus(now)
}

func (s *Shipper) dropQueue() {
	for _, p := range s.queue {
		os.Remove(p.path)
	}
	s.queue, s.queued, s.walBytes = nil, 0, 0
}

func (s *Shipper) writeStatus(now time.Time) error {
	b, _ := json.Marshal(Status{Last: now, URL: s.bucket.String(), Generation: s.gen})
	tmp := s.path + "-backup.json.tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path+"-backup.json")
}

// Close closes the shipper's connections, drops what is still queued (see Shipper: the next
// generation's snapshot holds those frames) and releases the database's backup lock.
func (s *Shipper) Close() error {
	s.dropQueue()
	err := s.db.Close()
	if s.lock != nil {
		unlockFile(s.lock)
		s.lock.Close()
		s.lock = nil
	}
	return err
}

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
