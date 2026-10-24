package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/signal"
	"strings"
	"syscall"

	"alder/internal/deploy"
	"alder/rt/db"
)

// The commands over a deployed app's silos (M15, chapter 16): `alder migrate silos <env>` and
// `alder silos prune <env>`; `alder status` prints their table, and `alder restore --silo`
// restores one (deploy.go).

const (
	migrateSilosUsage = "usage: alder migrate silos <env> [dir] [--host name] [--allow-data-loss]"
	silosUsage        = "usage: alder silos prune <env> [dir] [--host name] [--yes]"
)

// siloTarget is environment env's Ssh target, with its deploy lock taken (release it with the
// returned function), for a silo command.
func siloTarget(ctx context.Context, cmd, env, dir, host string, e deployEnv) (*deploy.Ssh, func(), error) {
	t, _, err := envTarget(cmd, env, dir, host, e)
	if err != nil {
		return nil, nil, err
	}
	s, ok := t.(*deploy.Ssh)
	if !ok {
		t.Close()
		return nil, nil, fmt.Errorf("`alder %s` is for an Ssh environment: %s is %s", cmd, env, t.Describe())
	}
	if len(s.Silos) == 0 {
		t.Close()
		return nil, nil, errors.New("the app declares no `silo`: it has no silos")
	}
	if _, err := s.Connect(ctx); err != nil {
		t.Close()
		return nil, nil, err
	}
	unlock, err := s.Lock(ctx)
	if err != nil {
		t.Close()
		return nil, nil, err
	}
	return s, func() { unlock(); t.Close() }, nil
}

// siloArgs reads `<env> [dir] [--host name]` and the flags named in flags (each reported in
// the returned set), or ok false.
func siloArgs(args []string, flags ...string) (env, dir, host string, set map[string]bool, ok bool) {
	set, dir = map[string]bool{}, "."
	var pos []string
	for i := 0; i < len(args); i++ {
		if h, isHost := hostFlag(args, &i); isHost {
			host = h
			continue
		}
		a := args[i]
		switch {
		case strings.HasPrefix(a, "-"):
			known := false
			for _, f := range flags {
				if a == f {
					set[f], known = true, true
				}
			}
			if !known {
				return "", "", "", nil, false
			}
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 1 || len(pos) > 2 {
		return "", "", "", nil, false
	}
	if len(pos) == 2 {
		dir = pos[1]
	}
	return pos[0], dir, host, set, true
}

// runMigrateSilosTo is `alder migrate silos <env> [dir] [--allow-data-loss]`: the current
// build's --migrate-silos at the target, under the deploy lock: every silo database (the files,
// or on Turso the databases the API lists) is migrated now rather than on its next open; one
// behind by a migration that drops data needs --allow-data-loss. Exit 0
// when every silo is served afterwards, 1 when one is not or the target fails, 2 for usage.
func runMigrateSilosTo(args []string, stdout, stderr io.Writer) int {
	return runMigrateSilosWith(args, stdout, stderr, osDeployEnv())
}

// runMigrateSilosWith is runMigrateSilosTo with the environment e.
func runMigrateSilosWith(args []string, stdout, stderr io.Writer, e deployEnv) int {
	env, dir, host, set, ok := siloArgs(args, "--allow-data-loss")
	if !ok {
		fmt.Fprintln(stderr, migrateSilosUsage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	s, release, err := siloTarget(ctx, "migrate silos", env, dir, host, e)
	if err != nil {
		fmt.Fprintf(stderr, "alder migrate silos: %v\n", err)
		return 2
	}
	defer release()
	out, err := s.MigrateSilos(ctx, set["--allow-data-loss"])
	fmt.Fprint(stdout, out)
	if err != nil {
		if errors.Is(err, deploy.ErrNotDeployed) {
			fmt.Fprintf(stderr, "alder migrate silos: %s is %v\n", env, err)
			return 1
		}
		fmt.Fprintf(stderr, "alder migrate silos: not every silo of %s is served (%v): the lines above say why\n", env, err)
		return 1
	}
	return 0
}

// runSilosTo is `alder silos prune <env> [dir] [--yes]`: it lists the orphaned silos (those
// whose key row — their Org — is gone from the shared database; `alder status` lists them),
// asks, and removes their databases (the current build's --prune-silos, which removes each only
// when it is still orphaned). Exit 0, 1 when the target fails or a silo was kept, 2 for usage
// and a removal not confirmed.
func runSilosTo(args []string, stdout, stderr io.Writer) int {
	return runSilosWith(args, stdout, stderr, osDeployEnv())
}

// runSilosWith is runSilosTo with the environment e.
func runSilosWith(args []string, stdout, stderr io.Writer, e deployEnv) int {
	if len(args) == 0 || args[0] != "prune" {
		fmt.Fprintln(stderr, silosUsage)
		return 2
	}
	env, dir, host, set, ok := siloArgs(args[1:], "--yes")
	if !ok {
		fmt.Fprintln(stderr, silosUsage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	s, release, err := siloTarget(ctx, "silos prune", env, dir, host, e)
	if err != nil {
		fmt.Fprintf(stderr, "alder silos prune: %v\n", err)
		return 2
	}
	defer release()
	lines, err := s.SiloStatus(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "alder silos prune: %v\n", err)
		return 1
	}
	var names []string
	for _, l := range lines {
		if l.State == db.SiloOrphaned {
			names = append(names, l.Key+":"+l.ID)
		}
	}
	if len(names) == 0 {
		fmt.Fprintf(stdout, "no orphaned silo at %s (%d silos)\n", env, len(lines))
		return 0
	}
	fmt.Fprintf(stdout, "orphaned silos at %s (their key row is gone from the shared database):\n", env)
	for _, n := range names {
		fmt.Fprintln(stdout, "  "+strings.Replace(n, ":", "/", 1))
	}
	if !set["--yes"] && !confirm(fmt.Sprintf("remove the databases of these %d silos (their data is gone for good, except in a backup)? type yes: ", len(names)), stdout) {
		fmt.Fprintln(stderr, "alder silos prune: not confirmed: nothing was removed (pass --yes to confirm without a terminal)")
		return 2
	}
	out, err := s.PruneSilos(ctx, names)
	fmt.Fprint(stdout, out)
	if err != nil {
		fmt.Fprintf(stderr, "alder silos prune: not every silo was removed (%v)\n", err)
		return 1
	}
	return 0
}
