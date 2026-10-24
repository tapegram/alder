package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"alder/internal/diag"
	"alder/internal/format"
	"alder/internal/names"
	"alder/internal/scratch"
	"alder/internal/syntax"
	"alder/internal/types"
)

const fmtUsage = "usage: alder fmt [--check | --stdin] [path...]"

// runFmtTo is `alder fmt`: the canonical formatter. Exit 0 when nothing is wrong, 1 when --check
// would reformat or a file does not parse, 2 for usage, IO and internal errors.
func runFmtTo(args []string, stdin io.Reader, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder fmt: internal error: %v\n", r)
			code = 2
		}
	}()
	checkOnly, useStdin := false, false
	var paths []string
	for _, a := range args {
		switch {
		case a == "--check":
			checkOnly = true
		case a == "--stdin":
			useStdin = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "alder fmt: unknown flag %s\n%s\n", a, fmtUsage)
			return 2
		default:
			paths = append(paths, a)
		}
	}
	if useStdin && (checkOnly || len(paths) > 0) {
		fmt.Fprintf(stderr, "alder fmt: --stdin takes no paths and cannot be combined with --check\n%s\n", fmtUsage)
		return 2
	}
	if useStdin {
		return fmtStdin(stdin, stdout, stderr)
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	var files []string
	for _, p := range paths {
		found, err := fmtFiles(p)
		if errors.Is(err, errScratchFile) {
			fmt.Fprintf(stderr, "alder fmt: %s is a scratch file; scratch files are not formatted yet\n", p)
			return 2
		}
		if err != nil {
			fmt.Fprintf(stderr, "alder fmt: %v\n", err)
			return 2
		}
		files = append(files, found...)
	}
	code = 0
	raise := func(c int) {
		if c > code {
			code = c
		}
	}
	cache := &fmtCache{noted: map[string]bool{}, progs: map[string]*fmtProject{}}
	for _, f := range files {
		raise(fmtFile(f, checkOnly, cache, stdout, stderr))
	}
	return code
}

func fmtStdin(stdin io.Reader, stdout, stderr io.Writer) int {
	src, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "alder fmt: %v\n", err)
		return 2
	}
	out, ds, err := format.Source("<stdin>", src)
	return fmtResult("<stdin>", src, out, ds, err, stderr, func(out []byte) int {
		if _, err := stdout.Write(out); err != nil {
			fmt.Fprintf(stderr, "alder fmt: %v\n", err)
			return 2
		}
		return 0
	})
}

// fmtCache is what one `alder fmt` run keeps between files: the standard library, the loaded and
// resolved program of each app root (dropped once a file in the root is written, so the next file
// is migrated against what is on disk), and the roots whose skipped migration was reported.
type fmtCache struct {
	std   map[string]*syntax.Module
	noted map[string]bool
	progs map[string]*fmtProject
}

// fmtProject is a loaded app root; bag is nil when the project could not be loaded or has files
// that do not parse (broken lists them, relative to the root).
type fmtProject struct {
	bag    *diag.Bag
	broken []string
}

// fmtFile formats one file, reporting on stdout and problems on stderr; it returns the file's
// exit code. When the file's project has a file that does not parse, migration is skipped and
// stderr says so once per project.
func fmtFile(path string, checkOnly bool, cache *fmtCache, stdout, stderr io.Writer) int {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(stderr, "alder fmt: %v\n", err)
		return 2
	}
	target := path // a symbolic link is written through, not replaced
	if r, err := filepath.EvalSymlinks(path); err == nil {
		target = r
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "alder fmt: %v\n", err)
		return 2
	}
	// the formatter's safety check compares the migrated source with its output, so the
	// migration is never judged by it; the file on disk is compared with the final output
	migrated, root, broken := fmtMigrate(path, src, cache, stderr)
	if len(broken) > 0 && !cache.noted[root] {
		cache.noted[root] = true
		verb, them := "does", "it"
		if len(broken) > 1 {
			verb, them = "do", "them"
		}
		fmt.Fprintf(stderr, "alder fmt: %s %s not parse, so imports were not migrated; fix %s and run again\n", strings.Join(broken, ", "), verb, them)
	}
	out, ds, err := format.Source(path, migrated)
	return fmtResult(path, src, out, ds, err, stderr, func(out []byte) int {
		if string(out) == string(src) {
			return 0
		}
		if checkOnly {
			fmt.Fprintf(stdout, "%s would be reformatted\n", path)
			return 1
		}
		if err := writeFileAtomic(target, out, info.Mode().Perm()); err != nil {
			fmt.Fprintf(stderr, "alder fmt: %v\n", err)
			return 2
		}
		delete(cache.progs, root) // the project on disk has changed
		if tr := findAppRoot(filepath.Dir(target)); tr != root {
			delete(cache.progs, tr) // a symbolic link written through into another project
		}
		fmt.Fprintln(stdout, path)
		return 0
	})
}

// fmtMigrate is src with the import-qualifier fixes (E0204, E0205) of a file inside an app
// project applied; src itself for a file outside one, or when the project cannot be loaded and
// resolved. When project files do not parse, it also returns the app root and those files
// (relative to the root) so the caller can say why nothing was migrated; the formatter reports
// the syntax errors of the file it formats. An error applying the fixes goes to stderr and the
// file is formatted unmigrated.
func fmtMigrate(path string, src []byte, cache *fmtCache, stderr io.Writer) (out []byte, root string, broken []string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return src, "", nil
	}
	root = findAppRoot(filepath.Dir(abs))
	if root == "" {
		return src, "", nil
	}
	pr := cache.progs[root]
	if pr == nil {
		pr = loadFmtProject(root, cache)
		cache.progs[root] = pr
	}
	if pr.bag == nil {
		return src, root, pr.broken
	}
	out, _, err = format.Migrate(pr.bag, fmtRel(root, abs), src)
	if err != nil {
		fmt.Fprintf(stderr, "alder fmt: %s: imports were not migrated: %v\n", path, err)
		return src, root, nil
	}
	return out, root, nil
}

func loadFmtProject(root string, cache *fmtCache) *fmtProject {
	if cache.std == nil {
		std, err := types.LoadStd()
		if err != nil {
			return &fmtProject{}
		}
		cache.std = std
	}
	prog, bag, ioErrs := names.LoadOverlay(root, cache.std, nil)
	if len(ioErrs) > 0 {
		return &fmtProject{}
	}
	if prog.ParseFailed {
		return &fmtProject{broken: prog.Unparsed}
	}
	prog.Resolve(bag)
	return &fmtProject{bag: bag}
}

// fmtResult turns a format.Source result into an exit code: diagnostics (1), an unsafe rewrite
// or other error (2), or ok(out).
func fmtResult(name string, src, out []byte, ds []diag.Diagnostic, err error, stderr io.Writer, ok func([]byte) int) int {
	switch {
	case errors.Is(err, format.ErrUnsafe):
		fmt.Fprintf(stderr, "alder fmt: internal error: %s: formatting changed the program; please file an issue\n", name)
		return 2
	case err != nil:
		fmt.Fprintf(stderr, "alder fmt: %s: %v\n", name, err)
		return 2
	case out == nil:
		for _, d := range ds {
			fmt.Fprint(stderr, diag.RenderText(d, strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")))
		}
		return 1
	}
	return ok(out)
}

var errScratchFile = errors.New("scratch file")

// fmtRel is p relative to root, slash-separated; p itself when it cannot be made relative.
func fmtRel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(p)
}

// fmtAppRoot is the root an explicitly named file or directory is judged against: the nearest
// directory at or above it (itself, for a directory) holding an app (app/*.alder, as `alder
// scratch` finds it), else the current directory.
func fmtAppRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "."
	}
	start := filepath.Dir(abs)
	if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
		start = abs
	}
	if root := findAppRoot(start); root != "" {
		return root
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// fmtFiles is the .alder files path names: itself, or a directory's, skipping build/ and
// dot-directories.
func fmtFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		abs, _ := filepath.Abs(path)
		if scratch.IsScratchFile(fmtRel(fmtAppRoot(abs), abs)) {
			return nil, errScratchFile
		}
		return []string{path}, nil
	}
	root := fmtAppRoot(path) // scratch files are judged relative to the project root, not the walked dir
	var files []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != path && (d.Name() == "build" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".alder") && !scratch.IsScratchFile(fmtRel(root, absPath(p))) {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

// writeFileAtomic replaces path with data through a temporary file in the same directory,
// keeping the file's mode.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".alder-fmt-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(name, mode)); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
