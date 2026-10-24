package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/generate"
	"alder/internal/ir"
	"alder/internal/migrate"
)

// The generators: `alder new page|table|form|msg|capability`, `alder edit table` and `alder
// fill` (book chapter 15). Each reads the app in --dir (default "."), computes its edits
// (internal/generate), applies them to a copy of the app, checks the copy as `alder check`
// does, and writes the files only when the copy checks (generate.Apply). It prints the files
// written (`wrote app/X.alder`, `edited app/Y.alder`), the unified diff of the edited ones and
// the copy's diagnostics, its typed holes among them; --json prints the same as one JSON object;
// --dry-run writes nothing. A refusal is one sentence on standard error, exit 1, nothing
// written; usage errors exit 2.

// genArgs are a generator's command line: --dir, --json and --dry-run, which every generator
// takes, the positional arguments, and the other flags with their values.
type genArgs struct {
	dir   string
	json  bool
	dry   bool
	pos   []string
	flags map[string][]string
}

// parseGenArgs splits args. valued names the flags that take a value (`--list T` or
// `--list=T`); multi, those that take every argument up to the next flag (`--from A B`);
// boolean flags besides --json and --dry-run are in bools. An unknown flag is an error.
func parseGenArgs(args []string, valued, multi, bools []string) (genArgs, error) {
	pos, flags, cli, err := generate.ScanArgs(args, valued, multi, bools)
	return genArgs{dir: cli.Dir, json: cli.JSON, dry: cli.DryRun, pos: pos, flags: flags}, err
}

// flag is the last value of flag name, or "".
func (g genArgs) flag(name string) string {
	if v := g.flags[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

// has reports whether boolean flag name was given.
func (g genArgs) has(name string) bool { return len(g.flags[name]) > 0 }

// genJSON is the --json output of a generator: the spec's {files, diff, diagnostics, holes},
// the migration's steps, whether the files were written, and the refusal's sentence.
type genJSON struct {
	Files       []generate.FileChange `json:"files"`
	Diff        string                `json:"diff"`
	Diagnostics []json.RawMessage     `json:"diagnostics"`
	Holes       []generate.Hole       `json:"holes"`
	Notes       []string              `json:"migration,omitempty"`
	Written     bool                  `json:"written"`
	Error       string                `json:"error,omitempty"`
	// Next is the command to run next (`alder new table`'s `next:` line), "" for none.
	Next string `json:"next,omitempty"`
	// Skipped and UpToDate are `alder regen admin`'s: the pages it did not recreate (deleted;
	// --all writes them), and whether it had nothing to do.
	Skipped  *[]string `json:"skipped,omitempty"`
	UpToDate *bool     `json:"upToDate,omitempty"`
	// Imports are `alder fill`'s: the modules it imported for the expression.
	Imports *[]string `json:"imports,omitempty"`
}

// runGeneratorWith ends every generator: it applies edits (computed by gen, from the loaded app)
// with o, then reports. cmd names the command in messages ("alder new page"). The app is loaded by load (nil: generate.Load, or
// LoadWithErrors for o.WriteDespiteErrors), and --json's object completed by extra (nil for
// none) before it is written.
func runGeneratorWith(cmd string, g genArgs, o generate.Options, load func(dir string) (*generate.Program, error), gen func(p *generate.Program) ([]generate.Edit, error), extra func(*genJSON), stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "%s: internal error: %v\n", cmd, r)
			code = 2
		}
	}()
	o.DryRun = g.dry
	if load == nil {
		load = generate.Load
		if o.WriteDespiteErrors {
			load = generate.LoadWithErrors
		}
	}
	p, err := load(g.dir)
	if err != nil {
		return genReport(cmd, g, nil, err, extra, stdout, stderr)
	}
	edits, err := gen(p)
	if err != nil {
		return genReport(cmd, g, nil, err, extra, stdout, stderr)
	}
	res, err := generate.Apply(g.dir, edits, o)
	return genReport(cmd, g, res, err, extra, stdout, stderr)
}

// genReport prints a generator's result (res, which may be nil) and its refusal or error, and
// returns the exit code: 0, 1 for a refusal, 2 for anything else. extra, when not nil, completes
// --json's object.
func genReport(cmd string, g genArgs, res *generate.Result, err error, extra func(*genJSON), stdout, stderr io.Writer) int {
	var ref *generate.Refusal
	refused := errors.As(err, &ref)
	if err != nil && !refused {
		fmt.Fprintf(stderr, "%s: %v\n", cmd, err)
		if g.json {
			out := genJSON{Files: []generate.FileChange{}, Diagnostics: []json.RawMessage{}, Holes: []generate.Hole{}, Error: err.Error()}
			if extra != nil {
				extra(&out)
			}
			writeJSON(stdout, out)
		}
		return 2
	}
	if res == nil && refused && ref.Result != nil {
		res = ref.Result
	}
	if res != nil && !refused { // a refusal writes nothing, formatted or not
		for _, w := range res.Warnings {
			fmt.Fprintf(stderr, "%s: warning: %s\n", cmd, w)
		}
	}
	if g.json {
		out := genJSON{Files: []generate.FileChange{}, Diagnostics: []json.RawMessage{}, Holes: []generate.Hole{}}
		if res != nil {
			out.Files, out.Diff, out.Notes, out.Written, out.Next = res.Files, res.Diff, res.Notes, res.Written, res.Next
			if out.Files == nil {
				out.Files = []generate.FileChange{}
			}
			for _, d := range res.Diagnostics {
				out.Diagnostics = append(out.Diagnostics, json.RawMessage(diag.RenderJSON(d)))
			}
			if res.Holes != nil {
				out.Holes = res.Holes
			}
		} else if refused {
			for _, d := range ref.Diags {
				out.Diagnostics = append(out.Diagnostics, json.RawMessage(diag.RenderJSON(d)))
			}
		}
		if refused {
			out.Error = strings.Replace(ref.Msg, " (the errors above)", "", 1) // they are in "diagnostics"
			out.Written = false
			out.Files, out.Diff, out.Next = []generate.FileChange{}, "", "" // nothing was written
		}
		for i, n := range out.Notes {
			out.Notes[i] = strings.TrimSpace(n)
		}
		if extra != nil {
			extra(&out)
		}
		writeJSON(stdout, out)
	} else {
		if res != nil {
			io.WriteString(stdout, res.RenderText(refused))
		} else if refused {
			for _, d := range ref.Diags {
				fmt.Fprint(stdout, diag.RenderText(d, ref.Sources[d.Span.File]))
			}
		}
		if res != nil && !refused && g.dry {
			fmt.Fprintln(stdout, "dry run: nothing written")
		}
	}
	if refused {
		fmt.Fprintf(stderr, "%s: %s\n", cmd, ref.Msg)
		return 1
	}
	return 0
}

// writeJSON writes v as one line of JSON, HTML characters unescaped (the diff and the
// diagnostics quote `<al-card>`, `->` and `&&`).
func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

// migrateCopyAs is the generators' migration writer: `alder migrate`'s generator (writeMigration)
// run on the copy of the app in dir, its tables read from their declarations alone
// (ir.LowerTables), so a table edit that breaks code elsewhere still gets its migration. A
// table declaration with errors is refused with them; a change no migration can make is
// refused with `alder migrate`'s reason. The notes are the migration's steps.
// The migration is named slug ("" derives it from its steps, as `alder migrate` does).
func migrateCopyAs(dir, slug string) ([]string, error) {
	prog, res, bag, ioErrs := buildkit.LoadChecked(dir, nil)
	if len(ioErrs) > 0 {
		return nil, ioErrs[0]
	}
	if res == nil {
		return nil, nil // a parse error: the check after this reports it
	}
	if errs := generate.TableErrors(prog, bag.Items); len(errs) > 0 {
		return nil, &generate.Refusal{Msg: "the result does not check (the errors above): nothing was written", Diags: errs, Sources: buildkit.Sources(prog)}
	}
	p, err := ir.LowerTables(prog, res)
	if err != nil { // a table names something that does not check (an import that is missing, say)
		var errs []diag.Diagnostic
		for _, d := range bag.Items {
			if d.Severity == "error" {
				errs = append(errs, d)
			}
		}
		return nil, &generate.Refusal{Msg: "the result does not check (the errors above): nothing was written", Diags: errs, Sources: buildkit.Sources(prog)}
	}
	var out, errb bytes.Buffer
	if code := writeMigration(dir, migrate.FromProgram(p).WithSilos(res.Silos), slug, false, &out, &errb); code != 0 {
		msg := strings.TrimSpace(out.String() + errb.String())
		var lines []string
		for _, l := range strings.Split(msg, "\n") {
			lines = append(lines, strings.TrimPrefix(l, "alder migrate: "))
		}
		return nil, generate.Refusef("the migration generator refuses the change: %s", strings.Join(lines, "; "))
	}
	var notes []string
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(l, "  ") || strings.HasPrefix(l, "!! ") {
			notes = append(notes, l)
		}
	}
	return notes, nil
}

// genUsage prints a generator's usage line and returns 2.
func genUsage(stderr io.Writer, usage string, err error) int {
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

const newPageUsage = "usage: alder new page [<Module>] <path> [--list T | --show T [--by <unique column>] | --from T …] [--blank] [--file [--type T]] [--example] [--param <name>:<Type> …] [--requires user] [--role R] [--dir app] [--json] [--dry-run]"

const newTableUsage = "usage: alder new table <Name> <field>:<Type>[=default][@attr[=arg]…] … [--no-timestamps] [--silo] [--dir app] [--json] [--dry-run]"

const newAdminUsage = "usage: alder new admin [--module Admin] [--path /admin] [--dir app] [--json] [--dry-run]"

const newFormUsage = "usage: alder new form <Page module> <Name> <field>:<Type>[=default] … [--into T] [--msg <Msg>] [--dir app] [--json] [--dry-run]"

const newMsgUsage = "usage: alder new msg <Page module> <Name> [<field>:<Type> …] [--in-for <binder>] [--when <Bool expression>] [--id <id>] [--dir app] [--json] [--dry-run]"

const newCapabilityUsage = "usage: alder new capability <Module> <entry> \"<Type>\" [--dir app] [--json] [--dry-run]"

const newTypeUsage = "usage: alder new type <Module> \"<Name> = A | B …\" | \"<Name> = brand <Base> [where …]\" | \"<Name> = { … }\" [--dir app] [--json] [--dry-run]"

const newDeclUsage = "usage: alder new decl <Module> <name> \"<Type>\" [--dir app] [--json] [--dry-run]"

const newTestUsage = "usage: alder new test <Module> \"<test name>\" [--dir app] [--json] [--dry-run]"

const newAuthUsage = "usage: alder new auth [--users User] [--role Role] [--to Page] [--dir app] [--json] [--dry-run]"

const newSiloUsage = "usage: alder new silo <key> <Table> … [--empty] [--dir app] [--json] [--dry-run]"

// specUsage is each generator's usage line (the text `--help` prints and an argument error shows).
var specUsage = map[string]string{
	"new page": newPageUsage, "new table": newTableUsage, "new form": newFormUsage, "new msg": newMsgUsage,
	"new capability": newCapabilityUsage, "new admin": newAdminUsage, "new auth": newAuthUsage,
	"new type": newTypeUsage, "new decl": newDeclUsage, "new test": newTestUsage, "new silo": newSiloUsage,
}

// migrateFor is the migration writer the Specs are given (generate.App.Migrate); slug "" is
// the name derived from the steps.
func migrateFor(dir, slug string) ([]string, error) { return migrateCopyAs(dir, slug) }

// runSpec is a generator command: argv parsed by its Spec, run, and reported as
// runGeneratorWith reports.
func runSpec(s *generate.Spec, argv []string, usage string, stdout, stderr io.Writer) int {
	return runSpecWith(s, argv, usage, nil, stdout, stderr)
}

// runSpecWith is runSpec that also hands seen (when not nil) the Result and the parsed common
// flags once the Spec has run.
func runSpecWith(s *generate.Spec, argv []string, usage string, seen func(*generate.Result, generate.CLIFlags), stdout, stderr io.Writer) (code int) {
	sets, cli, err := generate.ParseArgs(s, argv)
	if errors.Is(err, generate.ErrArity) {
		return genUsage(stderr, usage, nil)
	}
	if err != nil {
		return genUsage(stderr, usage, err)
	}
	g := genArgs{dir: cli.Dir, json: cli.JSON, dry: cli.DryRun}
	cmd := "alder " + s.ID
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "%s: internal error: %v\n", cmd, r)
			code = 2
		}
	}()
	res, err := s.Run(sets, generate.App{Dir: g.dir, Migrate: migrateFor}, g.dry)
	if seen != nil {
		seen(res, cli)
	}
	return genReport(cmd, g, res, err, specExtra(s, res), stdout, stderr)
}

// specExtra adds a Spec's own JSON fields (regen admin's skipped and upToDate); nil for others.
func specExtra(s *generate.Spec, res *generate.Result) func(*genJSON) {
	if s.ID != "regen admin" {
		return nil
	}
	skipped, upToDate := []string{}, false
	if res != nil {
		upToDate = res.UpToDate
		if res.Skipped != nil {
			skipped = res.Skipped
		}
	}
	return func(out *genJSON) { out.Skipped, out.UpToDate = &skipped, &upToDate }
}

// newGenerator is the Spec of `alder new <kind>`, or nil when kind is an app name.
func newGenerator(kind string) *generate.Spec { return generate.SpecByID("new " + kind) }
