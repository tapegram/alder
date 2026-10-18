package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/scaffold"
)

// newUsage is the usage line of `alder new`.
const newUsage = "usage: alder new <name> [--bare]  (or a path ending in the name: alder new apps/<name>), or alder new page|table|form|msg|capability|admin|auth|type|decl|test|silo … (a generator)"

// runNewTo is `alder new <dir>`: it writes the scaffold (internal/scaffold, the book's
// guestbook) into dir, whose last element is the app's name, then runs the migration generator
// on it (runMigrateTo, so migrations/ is exactly what `alder migrate` writes), and prints the
// files and the five commands to try. With --bare the app is the scaffold without the
// guestbook (scaffold.BareFiles: App, Layout, the stylesheet, adapters/; no page, no table, no
// migration), for an app built with the generators from nothing. The name must be letters, digits and `-`, starting with a
// letter; dir must not exist or be empty. When dir is inside another Go module (a go.mod above
// it), the app gets a go.mod of its own that keeps it out of that module, as the examples have.
// Exit 0 on success; 1 for a refused name or directory; 2 for usage errors and files that
// cannot be written (whatever was written is then removed).
func runNewTo(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder new: internal error: %v\n", r)
			code = 2
		}
	}()
	if len(args) > 0 {
		if s := newGenerator(args[0]); s != nil {
			return runSpec(s, args[1:], specUsage[s.ID], stdout, stderr)
		}
	}
	bare := false
	var rest []string
	for _, a := range args {
		if a == "--bare" || a == "-bare" {
			bare = true
			continue
		}
		rest = append(rest, a)
	}
	args = rest
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, newUsage)
		return 2
	}
	if args[0] == "" {
		fmt.Fprintln(stderr, "alder new: the app needs a name, such as `alder new guestbook`")
		return 2
	}
	dir := filepath.Clean(args[0])
	name := filepath.Base(dir)
	if newGenerator(name) != nil {
		fmt.Fprintf(stderr, "alder new: `%s` names a generator (`alder new %s …`), so it cannot name an app: choose another name\n", name, name)
		return 1
	}
	if !scaffold.ValidName(name) {
		fmt.Fprintf(stderr, "alder new: `%s` is not a valid app name: use letters, digits and `-`, starting with a letter (it names the directory, the built binary and the app's Go module)\n", name)
		return 1
	}
	if err := scaffold.CheckTarget(dir); errors.Is(err, scaffold.ErrNotEmpty) {
		fmt.Fprintf(stderr, "alder new: `%s` exists and is not empty: choose another name, or empty it first\n", dir)
		return 1
	} else if errors.Is(err, scaffold.ErrIsFile) {
		fmt.Fprintf(stderr, "alder new: `%s` is a file: choose another name\n", dir)
		return 1
	} else if err != nil {
		fmt.Fprintf(stderr, "alder new: %v\n", err)
		return 2
	}
	parent, err := scaffold.ParentModule(dir)
	if err != nil {
		fmt.Fprintf(stderr, "alder new: %v\n", err)
		return 2
	}
	_, existed := os.Stat(dir)
	fail := func(err error) int {
		if existed == nil { // it was an empty directory: empty it again
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		} else {
			os.RemoveAll(dir)
		}
		fmt.Fprintf(stderr, "alder new: %v\n", err)
		return 2
	}
	var written []string
	files := scaffold.Files(name, parent != "")
	if bare {
		files = scaffold.BareFiles(name, parent != "")
	}
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(p, f.Data, 0o644); err != nil {
			return fail(err)
		}
		written = append(written, f.Path)
	}
	var mout bytes.Buffer
	if c := runMigrateTo([]string{dir}, &mout, &mout); c != 0 {
		return fail(fmt.Errorf("the migration generator failed on the scaffold:\n%s", mout.String()))
	}
	if !bare { // a bare app has no table: nothing to migrate yet
		written = append(written, "migrations/0001_initial.sql", "migrations/schema.json")
	}

	fmt.Fprintf(stdout, "created %s/\n", dir)
	slices.Sort(written)
	for _, p := range written {
		fmt.Fprintf(stdout, "  %s\n", p)
	}
	if parent != "" {
		fmt.Fprintf(stdout, "go.mod keeps the app out of the Go module of %s\n", parent)
	}
	if bare {
		fmt.Fprintf(stdout, "\nnext:\n  cd %s\n", dir)
		fmt.Fprintln(stdout, "  alder new table …      # the app's tables (chapter 15)")
		fmt.Fprintln(stdout, "  alder new page …       # its pages")
		fmt.Fprintln(stdout, "  alder new auth         # a login")
		fmt.Fprintln(stdout, "  alder check            # type-check the app")
		return 0
	}
	fmt.Fprintf(stdout, "\nnext:\n  cd %s\n", dir)
	fmt.Fprintln(stdout, "  alder check            # type-check the app")
	fmt.Fprintln(stdout, "  alder test             # run its tests")
	fmt.Fprintln(stdout, "  alder test --mutate    # measure the tests")
	fmt.Fprintf(stdout, "  alder build            # build the server, build/%s/%s\n", name, name)
	fmt.Fprintln(stdout, "  alder run              # serve on http://localhost:8080, rebuilding on every save")
	return 0
}
