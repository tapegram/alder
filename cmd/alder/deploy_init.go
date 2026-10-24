package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"alder/internal/deploy"
	"alder/internal/version"
)

// initNeedsDisk: SQLite files to keep are the shared tables' and each silo's, which live
// under ALDER_DB's directory.
func initNeedsDisk(tables, silos int) bool { return tables > 0 || silos > 0 }

const deployInitUsage = "usage: alder deploy init render|fly [dir] [--force] [--dry-run]"

// runDeployInitTo is `alder deploy init render|fly [dir] [--force] [--dry-run]` (chapter 16): it
// writes the files a platform that builds the image from the repository needs (internal/deploy:
// InitFiles): a Dockerfile that builds alder at the compiler's own version and then the app, a
// .dockerignore and render.yaml or fly.toml. The app is named after dir. A disk or volume is
// declared when the app has tables (a SQLite file to keep). It never replaces a file: when any
// exists nothing is written, the existing ones are listed and the exit is 1, unless --force.
// --dry-run prints the files and writes nothing. Exit 0; 1 for files in the way; 2 for usage
// errors and an app that does not load.
func runDeployInitTo(args []string, stdout, stderr io.Writer) int {
	return runDeployInitWith(args, stdout, stderr, version.SourceOf)
}

// runDeployInitWith is runDeployInitTo with the alder source to pin given by source.
func runDeployInitWith(args []string, stdout, stderr io.Writer, source func() version.Source) int {
	var pos []string
	force, dry := false, false
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case a == "--dry-run":
			dry = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, deployInitUsage)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 1 || len(pos) > 2 || (pos[0] != "render" && pos[0] != "fly") {
		fmt.Fprintln(stderr, deployInitUsage)
		return 2
	}
	target, dir := pos[0], "."
	if len(pos) == 2 {
		dir = pos[1]
	}
	r := &deployRunner{dir: dir}
	r.Check()
	if r.loadErr != "" {
		fmt.Fprintf(stderr, "alder deploy init: %s\n", r.loadErr)
		return 2
	}
	src := source()
	spec := deploy.InitSpec{Target: target, App: r.app, Ref: src.Ref, Disk: initNeedsDisk(r.tables, len(r.silos))}
	files, err := deploy.InitFiles(spec)
	if err != nil {
		fmt.Fprintf(stderr, "alder deploy init: %v\n", err)
		return 2
	}
	var exist []string
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f.Name)); err == nil {
			exist = append(exist, f.Name)
		} else if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(stderr, "alder deploy init: %v\n", err)
			return 2
		}
	}
	blocked := len(exist) > 0 && !force
	if blocked {
		fmt.Fprintf(stderr, "alder deploy init: not writing: %s already exist(s); remove them, or pass --force to replace them\n", strings.Join(exist, ", "))
	}
	if dry {
		for _, f := range files {
			fmt.Fprintf(stdout, "--- %s\n%s", f.Name, f.Body)
			if !strings.HasSuffix(f.Body, "\n") {
				fmt.Fprintln(stdout)
			}
		}
		fmt.Fprintln(stdout, "--- dry run: nothing was written")
		if blocked {
			return 1
		}
		return 0
	}
	if blocked {
		return 1
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.Name), []byte(f.Body), 0o644); err != nil {
			fmt.Fprintf(stderr, "alder deploy init: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "wrote %s\n", f.Name)
	}
	switch {
	case src.Dirty:
		fmt.Fprintf(stdout, "this alder was built from a checkout with uncommitted changes, which no ref builds: set ALDER_REF in the Dockerfile (%s) to a release tag or a full commit\n", deploy.InitRefPlaceholder)
	case spec.Ref == "":
		fmt.Fprintf(stdout, "this alder records no version: set ALDER_REF in the Dockerfile (%s) to a release tag or a full commit\n", deploy.InitRefPlaceholder)
	default:
		fmt.Fprintf(stdout, "alder is pinned to %s (ALDER_REF in the Dockerfile)\n", spec.Ref)
		if !src.Release {
			fmt.Fprintf(stdout, "note: make sure %s is pushed to the alder remote, or the image build cannot fetch it\n", spec.Ref)
		}
	}
	if spec.Disk {
		fmt.Fprintf(stdout, "the app has tables or silos: SQLite lives at %s on a persistent %s\n", deploy.InitDB, map[string]string{"render": "disk", "fly": "volume"}[target])
	} else {
		fmt.Fprintln(stdout, "the app has no tables: no disk or volume declared")
	}
	switch target {
	case "render":
		fmt.Fprintln(stdout, "next: commit these files, push, and connect the repo in Render (New > Blueprint)")
	case "fly":
		fmt.Fprintf(stdout, "next: commit these files, then `fly launch --no-deploy --copy-config`, `fly secrets set ALDER_SECRET=$(openssl rand -hex 32)`%s and `fly deploy`\n",
			map[bool]string{true: ", `fly volumes create data --size 1`"}[spec.Disk])
	}
	return 0
}
