// Package scaffold holds the app `alder new` writes: the guestbook of the Alder book's chapter 2
// (an empty session, an error and a not-found page, a layout, one table, one page with a form
// and a test through the browser, a stylesheet, an adapters/ directory explained by its README
// and a README of the commands). The files are embedded from files/; the name of the app is
// substituted for {{name}}. The migrations are not here: `alder new` writes them by running the
// migration generator on these files, so they are what `alder migrate` would write.
package scaffold

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed files
var files embed.FS

// goModTemplate is the go.mod written only when the app is inside another Go module (see
// ParentModule); it is not part of Files. (A file named go.mod cannot be embedded: it would make
// files/ a module of its own.)
const goModTemplate = "files/go.mod.tmpl"

// File is one file of the scaffold: its path relative to the app's directory (slash-separated)
// and its contents, the app's name substituted.
type File struct {
	Path string
	Data []byte
}

// Files are the scaffold's files for an app named name, in path order, with the go.mod among
// them when withGoMod is set.
func Files(name string, withGoMod bool) []File {
	var out []File
	err := fs.WalkDir(files, "files", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if p == goModTemplate {
			if withGoMod {
				out = append(out, File{Path: "go.mod", Data: fill(p, name)})
			}
			return nil
		}
		out = append(out, File{Path: strings.TrimPrefix(p, "files/"), Data: fill(p, name)})
		return nil
	})
	if err != nil { // the embedded tree is fixed at compile time: it cannot fail to read
		panic(err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// fill is the embedded file p with name in place of {{name}}.
func fill(p, name string) []byte {
	b, err := files.ReadFile(p)
	if err != nil {
		panic(err)
	}
	return []byte(strings.ReplaceAll(string(b), "{{name}}", name))
}

var nameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

// ValidName reports whether name can name an app: letters, digits and `-`, starting with a
// letter. It names the directory, the built binary (build/<name>/<name>) and, inside another
// Go module, the app's own Go module, so it must be all three.
func ValidName(name string) bool { return nameRE.MatchString(name) }

// ErrNotEmpty is CheckTarget's refusal of a directory that already has something in it.
var ErrNotEmpty = errors.New("exists and is not empty")

// CheckTarget reports whether dir can receive a new app: it does not exist, or it is an empty
// directory. A directory with anything in it is ErrNotEmpty (nothing of a user's is ever
// overwritten); a file there is ErrNotEmpty too.
func CheckTarget(dir string) error {
	fi, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return ErrNotEmpty
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return ErrNotEmpty
	}
	return nil
}

// ParentModule is the go.mod of the Go module a new app at dir would be inside, found by
// walking up from dir's parent, or "" when there is none. An app inside another module needs a
// go.mod of its own: without one, `go vet ./...` and `go test ./...` in that module would
// compile the app's adapters/, whose import of "app" resolves only in build/<name>.
func ParentModule(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := filepath.Dir(abs); ; d = filepath.Dir(d) {
		p := filepath.Join(d, "go.mod")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
		if filepath.Dir(d) == d {
			return "", nil
		}
	}
}
