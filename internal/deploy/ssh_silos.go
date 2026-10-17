package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"alder/rt/db"
)

// The Ssh target's silos (M15): `alder status`'s silo table, `alder migrate silos`, `alder
// silos prune` and `alder restore --silo` run the current build's maintenance tasks at the box
// (its run script gives them the environment: the file database's directory, or the database on
// Turso and the token). With a file database the silos are <path>/db/silos/<key>/<value>.db;
// with `database: Turso` they are child databases (rt/db).

// ErrNotDeployed is the answer of a silo task at a target no build serves yet.
var ErrNotDeployed = errors.New("not deployed yet: no build serves there")

// current runs the current build's executable with args at the box and returns its output
// (stdout and stderr) and exit code.
func (s *Ssh) current(ctx context.Context, args ...string) (string, int, error) {
	var a strings.Builder
	for _, x := range args {
		a.WriteString(" " + q(x))
	}
	out, err := s.run(ctx, `c=$(cat "$R/current" 2>/dev/null) || { echo "@@none"; exit 0; }
"$R/builds/$c/run"`+a.String()+` 2>&1
echo "@@exit $?"
`)
	if err != nil {
		return out, -1, err
	}
	if strings.HasPrefix(out, "@@none") {
		return "", -1, ErrNotDeployed
	}
	i := strings.LastIndex(out, "@@exit ")
	if i < 0 {
		return out, -1, fmt.Errorf("the current build at %s printed no exit status", s.Dest)
	}
	code := 0
	fmt.Sscan(out[i+len("@@exit "):], &code)
	return out[:i], code, nil
}

// SiloStatus is the current build's --silo-status: every silo's state (nil for an app whose
// build has no silos).
func (s *Ssh) SiloStatus(ctx context.Context) ([]db.SiloStatus, error) {
	if len(s.Silos) == 0 {
		return nil, nil
	}
	out, code, err := s.current(ctx, "--silo-status")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("the current build's --silo-status failed (exit %d): %s", code, lastLines(out, 10))
	}
	var lines []db.SiloStatus
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "{") {
			continue
		}
		var st db.SiloStatus
		if err := json.Unmarshal([]byte(l), &st); err != nil {
			return nil, fmt.Errorf("--silo-status printed %q: %v", l, err)
		}
		lines = append(lines, st)
	}
	return lines, nil
}

// MigrateSilos is `alder migrate silos <env>`: the current build's --migrate-silos [--allow-
// data-loss] at the box. Its output is returned; an error when a silo is still not served.
func (s *Ssh) MigrateSilos(ctx context.Context, allowDataLoss bool) (string, error) {
	args := []string{"--migrate-silos"}
	if allowDataLoss {
		args = append(args, "--allow-data-loss")
	}
	out, code, err := s.current(ctx, args...)
	if err == nil && code != 0 {
		err = fmt.Errorf("exit %d", code)
	}
	return out, err
}

// PruneSilos is `alder silos prune <env>` after its confirmation: the current build's
// --prune-silos with the silos named ("org:<id>"), each removed only when still orphaned.
func (s *Ssh) PruneSilos(ctx context.Context, names []string) (string, error) {
	out, code, err := s.current(ctx, append([]string{"--prune-silos"}, names...)...)
	if err == nil && code != 0 {
		err = fmt.Errorf("exit %d", code)
	}
	return out, err
}

// serveAgain starts build cur (stopped for a restore), health checks it and routes to it.
func (s *Ssh) serveAgain(ctx context.Context, cur string) error {
	port, err := s.Start(ctx, cur)
	if err == nil {
		err = s.Health(ctx, cur, port, DefaultHealthTimeout)
	}
	if err == nil {
		_, err = s.Switch(ctx, cur, port)
	}
	return err
}

// swapTurso replaces db/turso.env with the shared database name at host, keeping the current
// one as db/turso.env.before-<why>-<time>; it returns that time.
func (s *Ssh) swapTurso(ctx context.Context, name, host, why string) (string, error) {
	out, err := s.run(ctx, `set -e
cd "$R/db"
ts=$(date -u +%Y%m%dT%H%M%SZ)
cp turso.env "turso.env.before-`+why+`-$ts"
printf '%s' `+q(TursoEnv(name, host))+` > turso.env.new && mv turso.env.new turso.env
echo "ts=$ts"
`)
	if err != nil {
		return "", err
	}
	return keyValues(out)["ts"], nil
}

// unswapTurso puts db/turso.env.before-<why>-<ts> back.
func (s *Ssh) unswapTurso(ctx context.Context, why, ts string) error {
	_, err := s.run(ctx, `set -e
cd "$R/db"
mv "turso.env.before-`+why+`-"`+q(ts)+` turso.env
`)
	return err
}

// restorePITR is RestoreBackup on Turso: the shared database as it was at the time builds/<src>/
// backup.pitr records (before src's migrations) is copied (Turso's point-in-time recovery)
// and becomes the app's database (db/turso.env), with build to's schema records; the
// database it replaces is kept at Turso. Undo puts the replaced one back.
func (s *Ssh) restorePITR(ctx context.Context, to, src string) (Restored, error) {
	if err := s.tursoInit(ctx); err != nil {
		return Restored{}, err
	}
	out, err := s.run(ctx, `cat "$R/builds/"`+q(src)+`/backup.pitr 2>/dev/null; exit 0`)
	if err != nil {
		return Restored{}, err
	}
	f := strings.Fields(out)
	if len(f) < 2 {
		return Restored{}, refuse("", "builds/%s/backup.pitr is missing: no point in time was recorded before that build's migrations", src)
	}
	at, err := time.Parse(time.RFC3339, f[0])
	if err != nil {
		return Restored{}, fmt.Errorf("builds/%s/backup.pitr: %v", src, err)
	}
	old := s.DB.SharedName()
	cp, err := s.DB.CopyAt(ctx, f[1], at)
	if err != nil {
		return Restored{}, err
	}
	if err := s.stopAll(ctx); err != nil {
		return Restored{}, err
	}
	ts, err := s.swapTurso(ctx, cp.Name, cp.Hostname, "rollback")
	if err != nil {
		return Restored{}, err
	}
	if _, err := s.run(ctx, `set -e
to=`+q(to)+`; ts=`+q(ts)+`
cd "$R/db"
for f in schema.json migrations; do if [ -f "$f" ]; then cp "$f" "$f.before-rollback-$ts"; fi; done
if [ -f "$R/builds/$to/migrations/schema.json" ]; then
  cp "$R/builds/$to/migrations/schema.json" schema.json
  ls "$R/builds/$to/migrations" | sed -n 's/\.sql$//p' > migrations
else rm -f schema.json migrations; fi
`); err != nil {
		return Restored{}, err
	}
	s.DB.setShared(cp.Name)
	undo := func(ctx context.Context) error {
		if err := s.stopAll(ctx); err != nil {
			return err
		}
		if err := s.unswapTurso(ctx, "rollback", ts); err != nil {
			return err
		}
		s.DB.setShared(old)
		_, err := s.run(ctx, `ts=`+q(ts)+`
cd "$R/db"
for f in schema.json migrations; do if [ -f "$f.before-rollback-$ts" ]; then mv "$f.before-rollback-$ts" "$f"; fi; done
`)
		return err
	}
	return Restored{From: "Turso point in time " + at.UTC().Format(time.RFC3339) + " of " + f[1] + " (now the database " + cp.Name + ")", Taken: at,
		Kept: "the database " + old + " at Turso (db/turso.env.before-rollback-" + ts + " names it)", Undo: undo}, nil
}

// RestorePointInTime is `alder restore <env> --to <time>` on Turso: a copy of the shared
// database as it was then becomes the app's database (db/turso.env), every build is stopped
// first and the current one started again on it, health checked and routed; a build that
// refuses it brings the replaced database back. It returns where the replaced one is kept.
func (s *Ssh) RestorePointInTime(ctx context.Context, at time.Time, w io.Writer) (string, error) {
	if s.DB == nil {
		return "", errors.New("not a Turso database")
	}
	if err := s.tursoInit(ctx); err != nil {
		return "", err
	}
	old := s.DB.SharedName()
	cp, err := s.DB.CopyAt(ctx, old, at)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "copied %s as it was at %s → %s\n", old, at.UTC().Format(time.RFC3339), cp.Name)
	cur, err := s.Current(ctx)
	if err != nil {
		return "", err
	}
	if err := s.stopAll(ctx); err != nil {
		return "", err
	}
	ts, err := s.swapTurso(ctx, cp.Name, cp.Hostname, "restore")
	if err != nil {
		return "", err
	}
	s.DB.setShared(cp.Name)
	kept := "the database " + old + " at Turso (db/turso.env.before-restore-" + ts + " names it)"
	if cur == "" {
		return kept, nil
	}
	if err := s.serveAgain(ctx, cur); err != nil {
		s.stopAll(ctx)
		if uerr := s.unswapTurso(ctx, "restore", ts); uerr != nil {
			return kept, refuse(err.Error(), "build %s did not pass its health check on the copy, and putting the database %s back failed (%v): restore db/turso.env.before-restore-%s as db/turso.env by hand and run `alder deploy`", cur, old, uerr, ts)
		}
		s.DB.setShared(old)
		if serr := s.serveAgain(ctx, cur); serr != nil {
			return kept, refuse(err.Error(), "build %s did not pass its health check on the copy %s: the database %s is the app's again, but build %s did not start on it (%v): run `alder deploy`", cur, cp.Name, old, cur, serr)
		}
		return kept, refuse(err.Error(), "build %s did not pass its health check on the copy %s (kept at Turso): the database %s is the app's again and build %s serves", cur, cp.Name, old, cur)
	}
	return kept, nil
}

// RestoreSilo is `alder restore <env> --silo key:value --to <time>`: one silo's database put
// back as it was at that time — on Turso a copy at that time made its database again
// (Turso.RestoreChild), for a file the local file (rebuilt from the silo's stream) uploaded in
// place, the file it replaces kept beside it as <value>.db.before-restore-<time> — then every
// build is stopped and the current one started again, so no process keeps what it read of the
// silo. It returns where the replaced database is kept.
func (s *Ssh) RestoreSilo(ctx context.Context, key, value, file string, at time.Time, w io.Writer) (string, error) {
	if _, err := db.SiloPath("/", key, value); err != nil {
		return "", refuse("", "%s:%s is not a silo (write --silo <key>:<id>)", key, value)
	}
	cur, err := s.Current(ctx)
	if err != nil {
		return "", err
	}
	var kept string
	if s.DB != nil {
		if err := s.tursoInit(ctx); err != nil {
			return "", err
		}
		if kept, err = s.DB.RestoreChild(ctx, key, value, at, w); err != nil {
			return kept, err
		}
		kept = "the database " + kept + " at Turso"
		if err := s.stopAll(ctx); err != nil {
			return kept, err
		}
	} else {
		rel := "db/silos/" + key + "/" + db.SiloFileName(value)
		fh, err := os.Open(file)
		if err != nil {
			return "", err
		}
		defer fh.Close()
		dst := s.Root + "/" + rel + ".restore"
		cmd := exec.CommandContext(ctx, "ssh", append(s.sshArgs(), "umask 077 && mkdir -p "+q(s.Root+"/db/silos/"+key)+" && cat > "+q(dst+".part")+" && mv "+q(dst+".part")+" "+q(dst))...)
		cmd.Stdin = fh
		var errb bytes.Buffer
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			return "", &RemoteError{Dest: s.Dest, Code: -1, Stderr: errb.String()}
		}
		if err := s.stopAll(ctx); err != nil {
			return "", err
		}
		out, err := s.run(ctx, `set -e
f="$R/"`+q(rel)+`
ts=$(date -u +%Y%m%dT%H%M%SZ)
for x in "" -wal -shm; do if [ -f "$f$x" ]; then mv "$f$x" "$f.before-restore-$ts$x"; fi; done
mv "$f.restore" "$f"
echo "ts=$ts"
`)
		if err != nil {
			return "", err
		}
		kept = rel + ".before-restore-" + keyValues(out)["ts"]
	}
	if cur == "" {
		return kept, nil
	}
	if err := s.serveAgain(ctx, cur); err != nil {
		return kept, refuse(err.Error(), "the silo %s:%s was restored, but build %s did not start again: run `alder deploy` (the replaced database is %s)", key, value, cur, kept)
	}
	return kept, nil
}
