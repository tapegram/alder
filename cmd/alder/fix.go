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
	"alder/internal/names"
	"alder/internal/routefix"
	"alder/internal/style"
)

const fixUsage = "usage: alder fix --routes [--name N] [--dry-run] [--dir app] [file…]"

// runFixTo is `alder fix --routes`: it rewrites an app written with `page` and `msg` into routes
// (#160; internal/routefix). With no files it rewrites the whole app under --dir (the current
// directory by default); with files, only those (the rest of the app is still read, so references
// resolve). It prints a line per file changed, per typed hole left and per write route whose page's
// `load` (which may have checked access) no longer runs first, and exits 1 while holes remain,
// on the run that wrote such a route, when it left a page `alder new admin` wrote on pages, or
// when the rewritten app has errors (which it prints); 0 when none of these, 2 for usage errors,
// a refusal (the app does not check, a route name collides, or a message's field shares a path
// parameter's name) and IO errors. --dry-run prints each change as a diff and writes nothing.
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
		printErrors(prog, bag, stderr)
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

	// With files named, only their page modules become routes: a reference from one of them to
	// a page that stays a page is left as written (routefix.Options.Only).
	opts := routefix.Options{Name: name}
	if len(want) > 0 {
		opts.Only = want
	}
	out, holes, err := routefix.Program(files, opts)
	if err != nil {
		var nc *routefix.ErrNameCollision
		var pc *routefix.ErrParamClash
		if errors.As(err, &nc) {
			fmt.Fprintf(stderr, "alder fix: %s: %v\n", nc.Module, err)
		} else if errors.As(err, &pc) {
			fmt.Fprintf(stderr, "alder fix: nothing written: %v\n", err)
		} else {
			fmt.Fprintf(stderr, "alder fix: %v\n", err)
		}
		return 2
	}
	changes := map[string][]byte{} // the files this run rewrites, by key
	for f, b := range out {
		if string(b) != string(files[f]) && (len(want) == 0 || want[f]) {
			changes[f] = b
		}
	}
	paths := slices.Sorted(maps.Keys(changes))

	// Files named are rewritten only when the app still checks with just them rewritten (holes
	// aside): the rest of the app may name their pages, which are routes now (`Todos.Index {}`
	// is `Todos.Index.Index {}`), and only a whole-app run rewrites those references.
	if len(want) > 0 && !dry && len(changes) > 0 {
		prog, _, bag, ioErrs := buildkit.LoadCheckedOverlay(dir, changes)
		if reportLoadErrs("alder fix", ioErrs, stderr) {
			return 2
		}
		if n := printErrors(prog, bag, stderr); n > 0 {
			fmt.Fprintf(stderr, "alder fix: rewriting only the files named leaves the app with %s: nothing written; name the files that refer to their pages too, or run alder fix --routes on the whole app\n",
				plural(n, "error", "errors"))
			return 2
		}
	}

	// A page `alder new admin` wrote is left on pages (routefix.AdminPage): only its `load`
	// gates its writes, which a route would not run, and its route lines would fall outside its
	// `@admin` regions, which `alder regen admin` then refuses. The admin's generator writes
	// routes now, so regenerating it is the rewrite; until then the run exits 1.
	var admin []string
	for _, f := range slices.Sorted(maps.Keys(files)) {
		if routefix.AdminPage(files[f]) && (len(want) == 0 || want[f]) {
			admin = append(admin, f)
		}
	}
	if len(admin) > 0 {
		fmt.Fprintf(stderr, "alder fix: %s (%s…) written by alder new admin left on pages: a route would not run the load that gates their writes, and their @admin regions would not match the route lines; regenerate the admin instead (move app/Admin aside, run alder new admin, which writes routes, then carry over your own changes)\n",
			plural(len(admin), "page", "pages"), admin[0])
	}

	for _, f := range paths {
		if dry {
			fmt.Fprintf(stdout, "would rewrite %s\n", f)
			fmt.Fprint(stdout, generate.UnifiedDiff(f, string(files[f]), string(changes[f])))
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(f)), changes[f], 0o644); err != nil {
			fmt.Fprintf(stderr, "alder fix: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "rewrote %s\n", f)
	}
	if len(changes) == 0 && len(admin) == 0 {
		fmt.Fprintln(stdout, "nothing to rewrite: no page or msg left")
	}
	left, access := 0, 0
	seen := map[string]bool{}
	for _, h := range holes {
		if _, ok := changes[h.File]; !ok {
			continue
		}
		if h.Access { // the comment above a write route's update: its page's load no longer runs first
			access++
			fmt.Fprintf(stdout, "%s:%d: access: %s\n", h.File, h.Line, h.Why)
			continue
		}
		left++
		seen[fmt.Sprintf("%s:%d", h.File, h.Line)] = true
		fmt.Fprintf(stdout, "%s:%d: hole: %s\n", h.File, h.Line, h.Why)
	}

	// The app as it is now: the holes an earlier run left and nobody filled (so a second run, as
	// in CI, still fails), and the errors the rewrite cannot settle: a login page that `requires
	// user` (which a page allowed and a route does not), or a lambda over a hole whose type is
	// unknown until it is filled.
	errs := 0
	if !dry {
		prog, _, bag, ioErrs := checkedApp(dir)
		if reportLoadErrs("alder fix", ioErrs, stderr) {
			return 2
		}
		for _, d := range bag.Items {
			at := fmt.Sprintf("%s:%d", d.Span.File, d.Span.Start.Line)
			if d.Code != "W0002" || seen[at] || len(want) > 0 && !want[d.Span.File] {
				continue
			}
			seen[at] = true
			left++
			fmt.Fprintf(stdout, "%s: hole: %s\n", at, d.Message)
		}
		errs = printErrors(prog, bag, stderr)
	}
	if errs > 0 {
		fmt.Fprintf(stderr, "alder fix: alder check finds %s after the rewrite: fix them by hand (filling the holes may clear some)\n",
			plural(errs, "error", "errors"))
	}
	if left > 0 {
		fmt.Fprintf(stderr, "alder fix: %s left: fill them (render for a one-row change, refresh otherwise), then run alder check\n",
			plural(left, "typed hole", "typed holes"))
	}
	if access > 0 {
		fmt.Fprintf(stderr, "alder fix: %s no longer run their page's load first: repeat in each update any access check its load made (a 404 for another user's row, a role re-read), then delete the comment above it\n",
			plural(access, "write route", "write routes"))
	}
	if left > 0 || errs > 0 || access > 0 || len(admin) > 0 {
		return 1
	}
	return 0
}

// printErrors prints the errors of bag (not its warnings) and returns how many there were.
func printErrors(prog *names.Program, bag *diag.Bag, w io.Writer) int {
	sources := buildkit.Sources(prog)
	n := 0
	for _, d := range bag.Items {
		if d.Severity == "error" {
			n++
			fmt.Fprint(w, diag.RenderTextStyled(d, sources[d.Span.File], style.For(w)))
		}
	}
	return n
}

// plural is "1 hole" or "3 holes".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func cmpErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
