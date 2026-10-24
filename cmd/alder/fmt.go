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
	for _, f := range files {
		raise(fmtFile(f, checkOnly, stdout, stderr))
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

// fmtFile formats one file, reporting on stdout and problems on stderr; it returns the file's exit code.
func fmtFile(path string, checkOnly bool, stdout, stderr io.Writer) int {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(stderr, "alder fmt: %v\n", err)
		return 2
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "alder fmt: %v\n", err)
		return 2
	}
	out, ds, err := format.Source(path, src)
	return fmtResult(path, src, out, ds, err, stderr, func(out []byte) int {
		if string(out) == string(src) {
			return 0
		}
		if checkOnly {
			fmt.Fprintf(stdout, "%s would be reformatted\n", path)
			return 1
		}
		if err := writeFileAtomic(path, out, info.Mode().Perm()); err != nil {
			fmt.Fprintf(stderr, "alder fmt: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, path)
		return 0
	})
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

// fmtFiles is the .alder files path names: itself, or a directory's, skipping build/ and
// dot-directories.
func fmtFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
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
		if strings.HasSuffix(p, ".alder") {
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
