package generate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/syntax"
)

// Edit is one change a generator makes to the app: a file it creates (Create, with its Body), a
// file it rewrites (Body, the whole new text), or the owned regions of a file it regenerates
// (Replace: each region's new body, by name; ReplaceRegions applies them), or a file it removes
// (Delete: `alder remove`). Path is relative to the app and slash-separated
// ("app/Posts/Index.alder").
type Edit struct {
	Path    string
	Create  bool
	Body    string
	Replace []Region
	Delete  bool
	// Guard, when set, are the lines (first, last) of the declaration the edit changes (the
	// session, a table): an error located there refuses the edit even under
	// Options.WriteDespiteErrors, whose to-do list is for breakage in other code.
	Guard *[2]int
}

// Region is the new body of one owned region of a file (Task 2's `-- @admin begin <name>` …
// `-- @admin end`), the lines between the markers.
type Region struct {
	Name string
	Body string
}

// Options says how Apply runs.
type Options struct {
	// DryRun computes and checks everything and writes nothing.
	DryRun bool
	// WriteDespiteErrors writes the files even when the copy has errors, which are then the
	// caller's to-do list: `alder edit table` uses it (without --strict), since a table change
	// breaks the code that inserts or reads the field, and the errors say where.
	WriteDespiteErrors bool
	// Migrate, when set, runs on the copy after the edits and before the check: the migration
	// generator (`alder migrate`'s), which writes migrations/ in the copy. Its notes are the
	// migration's steps, as `alder migrate` prints them; its error refuses the command.
	Migrate func(dir string) (notes []string, err error)
}

// Result is what Apply did, or would do.
type Result struct {
	Files []FileChange // the files written (or to write), sorted by path
	// Diff is the unified diff of every edited file under app/ (created files are listed, not
	// diffed).
	Diff string
	// Diagnostics are the copy's: every error, and the warnings in the files the command
	// changed (its typed holes among them, W0002).
	Diagnostics []diag.Diagnostic
	Holes       []Hole   // the typed holes in the files the command changed
	Notes       []string // the migration's steps, when Migrate wrote one
	Sources     map[string][]string
	Written     bool // the files were written into the app
}

// FileChange is one file Apply writes: Action is "created", "edited" or "removed".
type FileChange struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

// HasErrors reports whether the copy had errors.
func (r *Result) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

// Apply is the copy-check-write pipeline every generator ends in. It copies the app in appDir
// (all but build/, .git and SQLite databases) to a temporary directory, applies edits there (a created file that
// already exists in the app is refused), runs o.Migrate on the copy, checks the copy exactly as
// `alder check` checks an app, and, unless the copy has errors (or o.WriteDespiteErrors), writes
// every file the copy changed into the app. With o.DryRun nothing is written. A refusal is a
// *Refusal; when the copy did not check it carries the Result, whose Diagnostics say why.
func Apply(appDir string, edits []Edit, o Options) (*Result, error) {
	tmp, err := os.MkdirTemp("", "alder-generate-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	copyDir := filepath.Join(tmp, "app")
	copied := map[string][]byte{} // the app's files as they were copied, to write over only those
	if err := copyApp(appDir, copyDir, copied); err != nil {
		return nil, err
	}
	for _, e := range edits {
		p := filepath.Join(copyDir, filepath.FromSlash(e.Path))
		_, statErr := os.Stat(filepath.Join(appDir, filepath.FromSlash(e.Path)))
		switch {
		case e.Delete:
			if statErr != nil {
				return nil, Refusef("`%s` does not exist", e.Path)
			}
			if err := os.Remove(p); err != nil {
				return nil, err
			}
		case e.Create:
			if statErr == nil {
				return nil, Refusef("`%s` exists: a generator never overwrites a file; choose another name, or remove the file first", e.Path)
			}
			if err := writeFile(p, e.Body); err != nil {
				return nil, err
			}
		case e.Replace != nil:
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, Refusef("`%s` does not exist: nothing to regenerate", e.Path)
			}
			src, err := ReplaceRegions(string(b), e.Replace)
			if err != nil {
				return nil, Refusef("`%s`: %v", e.Path, err)
			}
			if err := writeFile(p, src); err != nil {
				return nil, err
			}
		default:
			if statErr != nil {
				return nil, Refusef("`%s` does not exist", e.Path)
			}
			if err := writeFile(p, e.Body); err != nil {
				return nil, err
			}
		}
	}
	res := &Result{}
	if o.Migrate != nil {
		notes, err := o.Migrate(copyDir)
		if err != nil {
			var r *Refusal
			if errors.As(err, &r) {
				return nil, r
			}
			return nil, err
		}
		res.Notes = notes
	}
	prog, checked, bag, ioErrs := buildkit.LoadChecked(copyDir, nil)
	if len(ioErrs) > 0 {
		return nil, ioErrs[0]
	}
	buildkit.AddSchemaDrift(copyDir, prog, checked, bag) // `alder check`'s E0019, as checkedApp adds it
	res.Sources = buildkit.Sources(prog)

	// the files the copy changed
	changed := map[string]bool{}
	err = filepath.WalkDir(copyDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(copyDir, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		now, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		before, berr := os.ReadFile(filepath.Join(appDir, filepath.FromSlash(rel)))
		switch {
		case berr != nil:
			res.Files = append(res.Files, FileChange{rel, "created"})
			changed[rel] = true
			if o.DryRun { // what would be written, in full
				res.Diff += strings.Replace(UnifiedDiff(rel, "", string(now)), "--- a/"+rel+"\n", "--- /dev/null\n", 1)
			}
		case !bytes.Equal(before, now):
			res.Files = append(res.Files, FileChange{rel, "edited"})
			changed[rel] = true
			if strings.HasPrefix(rel, "app/") {
				res.Diff += UnifiedDiff(rel, string(before), string(now))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, e := range edits {
		if e.Delete {
			res.Files = append(res.Files, FileChange{e.Path, "removed"})
			changed[e.Path] = true
		}
	}
	slices.SortFunc(res.Files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	for _, d := range bag.Items {
		if d.Severity == "error" || changed[d.Span.File] {
			res.Diagnostics = append(res.Diagnostics, d)
		}
	}
	for _, h := range HoleIndex(prog, bag.Items) {
		if changed[h.File] {
			res.Holes = append(res.Holes, h)
		}
	}
	for _, e := range edits {
		if e.Guard == nil {
			continue
		}
		var own []diag.Diagnostic
		for _, d := range res.Diagnostics {
			if d.Severity == "error" && d.Span.File == e.Path && d.Span.Start.Line >= e.Guard[0] && d.Span.Start.Line <= e.Guard[1] {
				own = append(own, d)
			}
		}
		if len(own) > 0 {
			res.Diagnostics = own
			return res, &Refusal{Msg: "the declaration the command edits does not check (the errors above): nothing was written", Result: res}
		}
	}
	if res.HasErrors() && !o.WriteDespiteErrors {
		return res, &Refusal{Msg: "the result does not check (the errors above): nothing was written", Result: res}
	}
	if o.DryRun {
		return res, nil
	}
	if beforeWrite != nil {
		beforeWrite()
	}
	// the app's files are still the ones copied (another command, or an editor, may have written
	// one since): else nothing is written, rather than the copy's text over a change it never saw
	for _, f := range res.Files {
		now, err := os.ReadFile(filepath.Join(appDir, filepath.FromSlash(f.Path)))
		was, ok := copied[f.Path]
		if ok != (err == nil) || ok && !bytes.Equal(now, was) {
			return res, &Refusal{Msg: "`" + f.Path + "` changed since the command read the app: nothing was written; run it again"}
		}
	}
	for _, f := range res.Files {
		if f.Action == "removed" {
			if err := os.Remove(filepath.Join(appDir, filepath.FromSlash(f.Path))); err != nil {
				return nil, err
			}
			removeEmptyDirs(appDir, filepath.Dir(filepath.FromSlash(f.Path)))
			continue
		}
		b, err := os.ReadFile(filepath.Join(copyDir, filepath.FromSlash(f.Path)))
		if err != nil {
			return nil, err
		}
		if err := writeFile(filepath.Join(appDir, filepath.FromSlash(f.Path)), string(b)); err != nil {
			return nil, err
		}
	}
	res.Written = true
	return res, nil
}

// removeEmptyDirs removes dir (relative to appDir) and its parents up to app/'s level while
// they are empty: the directory a removed page module leaves (app/Posts/ after Posts.Index).
func removeEmptyDirs(appDir, dir string) {
	for dir != "." && dir != "app" && dir != "" {
		if os.Remove(filepath.Join(appDir, dir)) != nil { // not empty
			return
		}
		dir = filepath.Dir(dir)
	}
}

// beforeWrite, when set (by a test), runs after the copy checked and before Apply writes.
var beforeWrite func()

// skipDir reports whether the directory rel (relative to the app) is left out of a copy: the
// build output and version control.
func skipDir(rel string) bool { return rel == "build" || rel == ".git" }

// skipFile reports whether a file is left out of a copy: a SQLite database (`alder run`'s dev
// database, its journal), which no command reads and which may be large or in use.
func skipFile(name string) bool {
	for _, ext := range []string{".db", ".db-wal", ".db-shm", ".db-journal"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// copyApp copies the app at src into dst, all but build/, .git and databases (symbolic links
// are left out), and, when copied is not nil, keeps each file's bytes there by its
// slash-separated path.
func copyApp(src, dst string, copied map[string][]byte) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			if skipDir(filepath.ToSlash(rel)) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() || skipFile(d.Name()) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if copied != nil {
			copied[filepath.ToSlash(rel)] = b
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
}

// writeFile writes text to path, creating its directory.
func writeFile(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// RenderText renders the result as the commands print it: one line per file (`wrote app/X.alder`,
// `edited app/Y.alder`; under --dry-run `would write`, `would edit`), the migration's steps under
// its file, the diff of the edited files (under --dry-run also the created ones, in full), then
// the diagnostics (as `alder check` renders them). A refused result (refused) is its
// diagnostics alone: nothing was written, so no file is named.
func (r *Result) RenderText(refused bool) string {
	var b strings.Builder
	if refused {
		for _, d := range r.Diagnostics {
			b.WriteString(diag.RenderText(d, r.Sources[d.Span.File]))
		}
		return b.String()
	}
	for _, f := range r.Files {
		verb := map[bool]string{true: "wrote", false: "would write"}[r.Written]
		switch f.Action {
		case "edited":
			verb = map[bool]string{true: "edited", false: "would edit"}[r.Written]
		case "removed":
			verb = map[bool]string{true: "removed", false: "would remove"}[r.Written]
		}
		fmt.Fprintf(&b, "%s %s\n", verb, f.Path)
		if strings.HasPrefix(f.Path, "migrations/") && strings.HasSuffix(f.Path, ".sql") {
			for _, n := range r.Notes {
				fmt.Fprintf(&b, "%s\n", n)
			}
		}
	}
	b.WriteString(r.Diff)
	for _, d := range r.Diagnostics {
		b.WriteString(diag.RenderText(d, r.Sources[d.Span.File]))
	}
	return b.String()
}

// Probe applies edits to a copy of the app and checks the copy, writing nothing: a generator
// that needs the checked types of what it is about to write (a form's fields, a capability's
// entries) reads them from the probe. The Program is the copy's; its diagnostics are all of the
// copy's, errors included.
func Probe(appDir string, edits []Edit) (*Program, error) {
	tmp, err := os.MkdirTemp("", "alder-probe-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	copyDir := filepath.Join(tmp, "app")
	if err := copyApp(appDir, copyDir, nil); err != nil {
		return nil, err
	}
	for _, e := range edits {
		if e.Delete {
			if err := os.Remove(filepath.Join(copyDir, filepath.FromSlash(e.Path))); err != nil {
				return nil, err
			}
			continue
		}
		if err := writeFile(filepath.Join(copyDir, filepath.FromSlash(e.Path)), e.Body); err != nil {
			return nil, err
		}
	}
	prog, res, bag, ioErrs := buildkit.LoadChecked(copyDir, nil)
	if len(ioErrs) > 0 {
		return nil, ioErrs[0]
	}
	if res == nil {
		return nil, &Refusal{Msg: "the result does not parse (the errors above): nothing was written", Diags: bag.Items, Sources: buildkit.Sources(prog)}
	}
	return newProgram(appDir, prog, res, bag.Items), nil
}

// errorsIn are the errors among p's diagnostics in file, as a Refusal (nil when there are none).
func (p *Program) errorsIn(file string) *Refusal {
	var errs []diag.Diagnostic
	for _, d := range p.Diags {
		if d.Severity == "error" && d.Span.File == file {
			errs = append(errs, d)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return &Refusal{Msg: "the result does not check (the errors above): nothing was written", Diags: errs, Sources: buildkit.Sources(p.Prog)}
}

// declLines are the first and last lines of the first declaration of src (a file of the app,
// path) that match picks, or nil when the file does not parse or has none: an Edit's Guard.
func declLines(path, src string, match func(syntax.Decl) bool) *[2]int {
	m, _ := syntax.ParseFile(path, []byte(src))
	if m == nil {
		return nil
	}
	for _, d := range m.Decls {
		if match(d) {
			sp := d.GetSpan()
			return &[2]int{sp.Start.Line, sp.End.Line}
		}
	}
	return nil
}
