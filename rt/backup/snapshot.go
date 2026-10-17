// Package backup keeps copies of an app's SQLite database (chapter 16): Snapshot, the
// consistent copy `alder deploy` takes into a build's directory before migrating (the binary's
// --backup-to), and the streaming shipper ALDER_BACKUP_URL turns on (Ship: a snapshot and then
// WAL segments to an S3-compatible bucket or a local directory) with Restore, which rebuilds the
// database as it was at a point in time.
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// open opens the database at path for the backup's own use (its own connections, never the
// app's pool), with the app's busy timeout and no automatic checkpoint (the shipper checkpoints
// only after copying the frames).
func open(path string) (*sql.DB, error) {
	if strings.Contains(path, "?") {
		return nil, fmt.Errorf("backup: invalid SQLite path %q", path)
	}
	d, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	return d, nil
}

// Snapshot writes a consistent copy of the database at path to dst (VACUUM INTO: a read
// transaction, so the app keeps serving and writing while it runs), replacing dst. A database
// that does not exist yet is an error wrapping os.ErrNotExist.
func Snapshot(ctx context.Context, path, dst string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	d, err := open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	os.Remove(dst)
	if _, err := d.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		return fmt.Errorf("backup: copying %s to %s: %w", path, dst, err)
	}
	return nil
}
