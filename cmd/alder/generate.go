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
	g := genArgs{dir: ".", flags: map[string][]string{}}
	is := func(set []string, name string) bool {
		for _, s := range set {
			if s == name {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-" || !strings.HasPrefix(a, "-") {
			g.pos = append(g.pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=")
		switch {
		case name == "json" && !hasVal:
			g.json = true
		case name == "dry-run" && !hasVal:
			g.dry = true
		case is(bools, name) && !hasVal:
			g.flags[name] = append(g.flags[name], "")
		case name == "dir" || is(valued, name):
			if !hasVal {
				if i+1 >= len(args) {
					return g, fmt.Errorf("--%s needs a value", name)
				}
				i++
				val = args[i]
			}
			if name == "dir" {
				g.dir = val
			} else {
				g.flags[name] = append(g.flags[name], val)
			}
		case is(multi, name):
			if hasVal {
				g.flags[name] = append(g.flags[name], val)
			}
			// up to the next flag, or a path (`--from Post Tag /dash`)
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && !strings.HasPrefix(args[i+1], "/") {
				i++
				g.flags[name] = append(g.flags[name], args[i])
			}
			if len(g.flags[name]) == 0 {
				return g, fmt.Errorf("--%s needs at least one value", name)
			}
		default:
			return g, fmt.Errorf("unknown flag %s", a)
		}
	}
	return g, nil
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
	// Skipped and UpToDate are `alder regen admin`'s: the pages it did not recreate (deleted;
	// --all writes them), and whether it had nothing to do.
	Skipped  *[]string `json:"skipped,omitempty"`
	UpToDate *bool     `json:"upToDate,omitempty"`
	// Imports are `alder fill`'s: the modules it imported for the expression.
	Imports *[]string `json:"imports,omitempty"`
}

// runGenerator ends every generator: it applies edits (computed by gen, from the loaded app)
// with o, then reports. cmd names the command in messages ("alder new page").
func runGenerator(cmd string, g genArgs, o generate.Options, gen func(p *generate.Program) ([]generate.Edit, error), stdout, stderr io.Writer) (code int) {
	return runGeneratorWith(cmd, g, o, nil, gen, nil, stdout, stderr)
}

// runGeneratorWith is runGenerator with the app loaded by load (nil: generate.Load, or
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
	if g.json {
		out := genJSON{Files: []generate.FileChange{}, Diagnostics: []json.RawMessage{}, Holes: []generate.Hole{}}
		if res != nil {
			out.Files, out.Diff, out.Notes, out.Written = res.Files, res.Diff, res.Notes, res.Written
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
			out.Files, out.Diff = []generate.FileChange{}, "" // nothing was written
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

// migrateCopy is the generators' Options.Migrate: `alder migrate`'s generator (writeMigration)
// run on the copy of the app in dir, its tables read from their declarations alone
// (ir.LowerTables), so a table edit that breaks code elsewhere still gets its migration. A
// table declaration with errors is refused with them; a change no migration can make is
// refused with `alder migrate`'s reason. The notes are the migration's steps.
func migrateCopy(dir string) ([]string, error) { return migrateCopyNamed("")(dir) }

// migrateCopyNamed is migrateCopy with the migration named slug ("" derives it from its steps,
// as `alder migrate` does).
func migrateCopyNamed(slug string) func(dir string) ([]string, error) {
	return func(dir string) ([]string, error) { return migrateCopyAs(dir, slug) }
}

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
	if code := writeMigration(dir, migrate.FromProgram(p), slug, false, &out, &errb); code != 0 {
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

// newGenerators are `alder new`'s generators, by kind; their names are not app names.
var newGenerators = map[string]func(args []string, stdout, stderr io.Writer) int{
	"page":       runNewPageTo,
	"table":      runNewTableTo,
	"form":       runNewFormTo,
	"msg":        runNewMsgTo,
	"capability": runNewCapabilityTo,
	"admin":      runNewAdminTo,
	"auth":       runNewAuthTo,
	"type":       runNewTypeTo,
	"decl":       runNewDeclTo,
	"test":       runNewTestTo,
}

// genUsage prints a generator's usage line and returns 2.
func genUsage(stderr io.Writer, usage string, err error) int {
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

// parseFields parses field arguments; a bad one is refused (reported, false).
func parseFields(g genArgs, args []string, table bool, cmd string, stdout, stderr io.Writer) ([]generate.FieldSpec, bool) {
	var fields []generate.FieldSpec
	for _, a := range args {
		f, err := generate.ParseField(a, table)
		if err != nil {
			genReport(cmd, g, nil, generate.Refusef("%v", err), nil, stdout, stderr)
			return nil, false
		}
		fields = append(fields, f)
	}
	return fields, true
}

const newPageUsage = "usage: alder new page <Module> <path> [--list T | --show T | --from T …] [--blank] [--requires user] [--role R] [--dir app] [--json] [--dry-run]"

// runNewPageTo is `alder new page`.
func runNewPageTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, []string{"list", "show", "requires", "role"}, []string{"from"}, []string{"blank"})
	if err != nil || len(g.pos) != 2 {
		return genUsage(stderr, newPageUsage, err)
	}
	o := generate.PageOptions{Module: g.pos[0], Path: g.pos[1], List: g.flag("list"), Show: g.flag("show"), From: g.flags["from"], Requires: g.flag("requires"), Role: g.flag("role"), Blank: g.has("blank")}
	return runGenerator("alder new page", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) { return generate.NewPage(p, o) }, stdout, stderr)
}

const newTableUsage = "usage: alder new table <Name> <field>:<Type>[=default][@attr[=arg]…] … [--dir app] [--json] [--dry-run]"

// runNewTableTo is `alder new table`.
func runNewTableTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, nil, nil, nil)
	if err != nil || len(g.pos) < 1 {
		return genUsage(stderr, newTableUsage, err)
	}
	fields, ok := parseFields(g, g.pos[1:], true, "alder new table", stdout, stderr)
	if !ok {
		return 1
	}
	return runGenerator("alder new table", g, generate.Options{Migrate: migrateCopy}, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.NewTable(p, g.pos[0], fields)
	}, stdout, stderr)
}

const newAdminUsage = "usage: alder new admin [--module Admin] [--path /admin] [--dir app] [--json] [--dry-run]"

// runNewAdminTo is `alder new admin`: the admin of every table (generate.NewAdmin).
func runNewAdminTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, []string{"module", "path"}, nil, nil)
	if err != nil || len(g.pos) != 0 {
		return genUsage(stderr, newAdminUsage, err)
	}
	o := generate.AdminOptions{Module: g.flag("module"), Path: g.flag("path")}
	return runGenerator("alder new admin", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) { return generate.NewAdmin(p, o) }, stdout, stderr)
}

const newFormUsage = "usage: alder new form <Page module> <Name> <field>:<Type>[=default] … [--into T] [--dir app] [--json] [--dry-run]"

// runNewFormTo is `alder new form`.
func runNewFormTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, []string{"into"}, nil, nil)
	if err != nil || len(g.pos) < 2 {
		return genUsage(stderr, newFormUsage, err)
	}
	fields, ok := parseFields(g, g.pos[2:], false, "alder new form", stdout, stderr)
	if !ok {
		return 1
	}
	o := generate.FormOptions{Page: g.pos[0], Name: g.pos[1], Fields: fields, Into: g.flag("into")}
	return runGenerator("alder new form", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) { return generate.NewForm(p, o) }, stdout, stderr)
}

const newMsgUsage = "usage: alder new msg <Page module> <Name> [<field>:<Type> …] [--in-for <binder>] [--when <Bool expression>] [--dir app] [--json] [--dry-run]"

// runNewMsgTo is `alder new msg`.
func runNewMsgTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, []string{"in-for", "when"}, nil, nil)
	if err != nil || len(g.pos) < 2 {
		return genUsage(stderr, newMsgUsage, err)
	}
	fields, ok := parseFields(g, g.pos[2:], false, "alder new msg", stdout, stderr)
	if !ok {
		return 1
	}
	return runGenerator("alder new msg", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.NewMsg(p, g.pos[0], g.pos[1], fields, generate.MsgOptions{InFor: g.flag("in-for"), When: g.flag("when")})
	}, stdout, stderr)
}

const newCapabilityUsage = "usage: alder new capability <Module> <entry> \"<Type>\" [--dir app] [--json] [--dry-run]"

// runNewCapabilityTo is `alder new capability`.
func runNewCapabilityTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, nil, nil, nil)
	if err != nil || len(g.pos) != 3 {
		return genUsage(stderr, newCapabilityUsage, err)
	}
	return runGenerator("alder new capability", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.NewCapability(p, g.pos[0], g.pos[1], g.pos[2])
	}, stdout, stderr)
}

const newTypeUsage = "usage: alder new type <Module> \"<Name> = A | B …\" | \"<Name> = brand <Base> [where …]\" | \"<Name> = { … }\" [--dir app] [--json] [--dry-run]"

// runNewTypeTo is `alder new type`.
func runNewTypeTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, nil, nil, nil)
	if err != nil || len(g.pos) != 2 {
		return genUsage(stderr, newTypeUsage, err)
	}
	return runGenerator("alder new type", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.NewType(p, g.pos[0], g.pos[1])
	}, stdout, stderr)
}

const newDeclUsage = "usage: alder new decl <Module> <name> \"<Type>\" [--dir app] [--json] [--dry-run]"

// runNewDeclTo is `alder new decl`.
func runNewDeclTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, nil, nil, nil)
	if err != nil || len(g.pos) != 3 {
		return genUsage(stderr, newDeclUsage, err)
	}
	return runGenerator("alder new decl", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.NewDecl(p, g.pos[0], g.pos[1], g.pos[2])
	}, stdout, stderr)
}

const newTestUsage = "usage: alder new test <Module> \"<test name>\" [--dir app] [--json] [--dry-run]"

// runNewTestTo is `alder new test`.
func runNewTestTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, nil, nil, nil)
	if err != nil || len(g.pos) != 2 {
		return genUsage(stderr, newTestUsage, err)
	}
	return runGenerator("alder new test", g, generate.Options{}, func(p *generate.Program) ([]generate.Edit, error) {
		return generate.NewTest(p, g.pos[0], g.pos[1])
	}, stdout, stderr)
}

const newAuthUsage = "usage: alder new auth [--users User] [--role Role] [--to Page] [--dir app] [--json] [--dry-run]"

// runNewAuthTo is `alder new auth`: a login with a real password adapter (generate.NewAuth);
// the user table's migration when it creates the table.
func runNewAuthTo(args []string, stdout, stderr io.Writer) int {
	g, err := parseGenArgs(args, []string{"users", "role", "to"}, nil, nil)
	if err != nil || len(g.pos) != 0 {
		return genUsage(stderr, newAuthUsage, err)
	}
	created := false
	o := generate.Options{Migrate: func(dir string) ([]string, error) {
		if !created {
			return nil, nil
		}
		return migrateCopy(dir)
	}}
	return runGenerator("alder new auth", g, o, func(p *generate.Program) ([]generate.Edit, error) {
		edits, c, err := generate.NewAuth(p, generate.AuthOptions{Users: g.flag("users"), Role: g.flag("role"), To: g.flag("to")})
		created = c
		return edits, err
	}, stdout, stderr)
}
