package backup

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The silos of an app with `silo` declarations (M15) are files beside the shared database:
// <dir>/silos/<key>/<value>.db. Each is shipped like the shared database, by a Shipper of its
// own, under its own prefix of the location:
//
//	silos/<key>/<name>/generations/<gen>/…     the layout above, per silo (<name>: the
//	                                           silo file's name without .db, rt/db's
//	                                           SiloFileName of the id)
//
// so `alder restore <env> --silo <key>:<id> --to <time>` rebuilds one silo (RestoreSilo).
// The shippers are bounded: past MaxSilos files (one shipper, a lock file and two connections
// each), none of the silos is shipped and the log says why, once, pointing at Turso; their
// WALs are still checkpointed (their connections leave it to the shipper), so they do not grow.

// MaxSilos is how many silo files the shipper streams.
const MaxSilos = 200

// prefixBucket is a Bucket under a prefix of another.
type prefixBucket struct {
	b      Bucket
	prefix string
}

func (p prefixBucket) Put(ctx context.Context, key, path string) error {
	return p.b.Put(ctx, p.prefix+key, path)
}

func (p prefixBucket) Get(ctx context.Context, key, path string) error {
	return p.b.Get(ctx, p.prefix+key, path)
}

func (p prefixBucket) List(ctx context.Context, prefix string) ([]string, error) {
	keys, err := p.b.List(ctx, p.prefix+prefix)
	for i, k := range keys {
		keys[i] = strings.TrimPrefix(k, p.prefix)
	}
	return keys, err
}

func (p prefixBucket) String() string {
	return strings.TrimSuffix(p.b.String(), "/") + "/" + strings.TrimSuffix(p.prefix, "/")
}

// SiloPrefix is where the silo file <dir>/silos/<key>/<name>.db is shipped under the location.
func SiloPrefix(key, name string) string { return "silos/" + key + "/" + name + "/" }

// RestoreSilo is Restore of one silo's stream: the silo file silos/<key>/<name>.db as the
// location loc held it at time to (the zero time: the latest), rebuilt at dst.
func RestoreSilo(ctx context.Context, loc, key, name string, to time.Time, dst string) (Restored, error) {
	b, err := OpenBucket(loc)
	if err != nil {
		return Restored{}, err
	}
	return RestoreFrom(ctx, prefixBucket{b, SiloPrefix(key, name)}, to, dst)
}

// silos ships the silo files under dir: every interval it starts a shipper for a file it has
// none for (a file whose shipper another process still holds is tried on the next tick) and
// ticks each one; until stop is closed, when it ticks each once more and closes them.
func shipSilos(dir string, b Bucket, interval time.Duration, log func(level, msg string), stop <-chan struct{}) {
	ships := map[string]*Shipper{}
	warned := false
	defer func() {
		for _, s := range ships {
			s.nextTry = time.Time{}
			s.tick()
			s.Close()
		}
	}()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		files, _ := filepath.Glob(filepath.Join(dir, "silos", "*", "*.db"))
		slices.Sort(files)
		if len(files) > MaxSilos {
			if !warned {
				log("error", fmt.Sprintf("backup: %d silo files under %s, more than the %d the shipper streams: none of the silos is shipped (the shared database is); with this many silos put the data on Turso (`database: Turso`, chapter 16)", len(files), filepath.Join(dir, "silos"), MaxSilos))
				warned = true
			}
			for p, s := range ships {
				s.Close()
				delete(ships, p)
			}
			for _, f := range files {
				checkpoint(f)
			}
		} else {
			warned = false
			for _, f := range files {
				s := ships[f]
				if s == nil {
					key := filepath.Base(filepath.Dir(f))
					name := strings.TrimSuffix(filepath.Base(f), ".db")
					var err error
					s, err = NewShipper(f, prefixBucket{b, SiloPrefix(key, name)}, log)
					if errors.Is(err, ErrShipperRunning) {
						continue
					}
					if err != nil {
						log("error", "backup: silo file "+key+"/"+name+".db: "+err.Error())
						continue
					}
					s.interval = interval
					ships[f] = s
				}
				s.tick()
			}
			for p, s := range ships { // a silo pruned meanwhile
				if !slices.Contains(files, p) {
					s.Close()
					delete(ships, p)
				}
			}
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

// checkpoint folds the WAL of the database at path into it (PASSIVE: it waits for no one).
func checkpoint(path string) {
	d, err := open(path)
	if err != nil {
		return
	}
	defer d.Close()
	d.Exec("PRAGMA wal_checkpoint(PASSIVE)")
}
