package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/generate"
	"alder/internal/routefix"
	"alder/internal/style"
)

const fixUsage = "usage: alder fix --routes [--name N] [--dry-run] [--dir app] [file…]"

// runFixTo is `alder fix --routes`: it rewrites an app written with `page` and `msg` into routes
// (#160; internal/routefix). With no files it rewrites the whole app under --dir (the current
// directory by default); with files, only those (the rest of the app is still read, so references
// resolve). It prints a line per file changed and per typed hole left, and exits 1 while holes
// remain, 0 when none do, 2 for usage errors, a refusal (the app does not check, or a route name
// collides) and IO errors. --dry-run prints each change as a diff and writes nothing.
func runFixTo(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder fix: internal error: %v\n", r)
			code = 2
		}
	}()
	dir, name, routes, dry := ".", "", false, false
	var only []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--routes":
			routes = true
		case a == "--dry-run":
			dry = true
		case (a == "--name" || a == "--dir") && i+1 < len(args):
			i++
			if a == "--name" {
				name = args[i]
			} else {
				dir = args[i]
			}
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, fixUsage)
			return 2
		default:
			only = append(only, a)
		}
	}
	if !routes {
		fmt.Fprintln(stderr, fixUsage)
		return 2
	}

	// The rewrite reads a program that checks: on a broken one its holes and refusals would mix
	// with the errors the app already has.
	prog, _, bag, ioErrs := checkedApp(dir)
	if reportLoadErrs("alder fix", ioErrs, stderr) {
		return 2
	}
	if len(prog.Modules) == 0 && len(prog.Refused) == 0 {
		fmt.Fprintf(stderr, "alder fix: no .alder files under %s\n", filepath.Join(dir, "app"))
		return 2
	}
	if bag.HasErrors() {
		sources := buildkit.Sources(prog)
		for _, d := range bag.Items {
			if d.Severity == "error" {
				fmt.Fprint(stderr, diag.RenderTextStyled(d, sources[d.Span.File], style.For(stderr)))
			}
		}
		fmt.Fprintln(stderr, "alder fix: `alder check` fails; fix its errors first, then run alder fix --routes again")
		return 2
	}

	files := map[string][]byte{}
	app := filepath.Join(dir, "app")
	err := filepath.WalkDir(app, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".alder") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "alder fix: %v\n", err)
		return 2
	}

	// The files named, as keys of files.
	want := map[string]bool{}
	for _, f := range only {
		abs, err1 := filepath.Abs(f)
		root, err2 := filepath.Abs(dir)
		if err1 != nil || err2 != nil {
			fmt.Fprintf(stderr, "alder fix: %s: %v\n", f, cmpErr(err1, err2))
			return 2
		}
		rel, err := filepath.Rel(root, abs)
		key := filepath.ToSlash(rel)
		if err != nil || files[key] == nil {
			fmt.Fprintf(stderr, "alder fix: %s is not an .alder file of the app under %s\n", f, app)
			return 2
		}
		want[key] = true
	}

	out, holes, err := routefix.Program(files, routefix.Options{Name: name})
	if err != nil {
		var nc *routefix.ErrNameCollision
		if errors.As(err, &nc) {
			fmt.Fprintf(stderr, "alder fix: %s: %v\n", nc.Module, err)
		} else {
			fmt.Fprintf(stderr, "alder fix: %v\n", err)
		}
		return 2
	}

	changed := 0
	for _, f := range slices.Sorted(maps.Keys(out)) {
		if string(out[f]) == string(files[f]) || len(want) > 0 && !want[f] {
			continue
		}
		changed++
		if dry {
			fmt.Fprintf(stdout, "would rewrite %s\n", f)
			fmt.Fprint(stdout, generate.UnifiedDiff(f, string(files[f]), string(out[f])))
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(f)), out[f], 0o644); err != nil {
			fmt.Fprintf(stderr, "alder fix: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "rewrote %s\n", f)
	}
	left := 0
	for _, h := range holes {
		if len(want) > 0 && !want[h.File] {
			continue
		}
		left++
		fmt.Fprintf(stdout, "%s:%d: hole: %s\n", h.File, h.Line, h.Why)
	}
	if changed == 0 {
		fmt.Fprintln(stdout, "nothing to rewrite: no page or msg left")
	}
	if left > 0 {
		fmt.Fprintf(stderr, "alder fix: %d typed %s left: fill %s (render for a one-row change, refresh otherwise), then run alder check\n",
			left, map[bool]string{true: "hole", false: "holes"}[left == 1], map[bool]string{true: "it", false: "them"}[left == 1])
		return 1
	}
	return 0
}

func cmpErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
