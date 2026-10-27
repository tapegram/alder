package main

import (
	"encoding/json"
	"io"
	"strings"

	"alder/internal/generate"
	"alder/internal/version"
)

// flagSpec is one flag in `alder help --json`.
type flagSpec struct {
	Name    string `json:"name"`            // as typed: "--json", "-run"
	Value   string `json:"value,omitempty"` // the value's placeholder ("T", "re"); empty for a switch
	Multi   bool   `json:"multi,omitempty"` // may be given more than once
	Summary string `json:"summary"`
}

// cmdSpec is a command or subcommand in `alder help --json`. Usage is the very string
// `alder <command> --help` prints (commandUsages, newGeneratorUsages); a subcommand with no
// Usage is covered by its parent's. TestHelpJSON_MatchesUsage pins Flags to those strings.
type cmdSpec struct {
	Name        string     `json:"name"`
	Summary     string     `json:"summary"`
	Usage       string     `json:"usage,omitempty"`
	Args        []string   `json:"args"`
	Flags       []flagSpec `json:"flags"`
	Subcommands []cmdSpec  `json:"subcommands,omitempty"`
}

// genFlags are the flags every generator takes (parseGenArgs).
var genFlags = []flagSpec{
	{Name: "--dir", Value: "app", Summary: "the app root, the directory holding app/ (default: the current directory)"},
	{Name: "--json", Summary: "print the result as one JSON object"},
	{Name: "--dry-run", Summary: "check the change on a copy and print the diff; write nothing"},
}

func withGenFlags(fs ...flagSpec) []flagSpec { return append(fs, genFlags...) }

// specFlags are a Spec's flags as help --json shows them. A repeatable flag (--param) is Multi;
// a greedy one (--from) takes several words but is given once, so it is not.
func specFlags(s *generate.Spec) []flagSpec {
	var out []flagSpec
	for _, f := range s.Flags {
		v := f.Value
		if f.Kind == generate.ArgBool {
			v = ""
		}
		out = append(out, flagSpec{Name: "--" + f.Name, Value: v, Multi: f.Variadic, Summary: f.Summary})
	}
	return out
}

func specArgs(s *generate.Spec) []string {
	var out []string
	for _, a := range s.Args {
		out = append(out, a.Value)
	}
	return out
}

// groupCmd is the subcommand nodes of group, from the Specs, in their order. `new` nodes carry
// their usage line and the generator flags. `edit` and `remove` nodes carry their Args and
// Flags but no Usage and no generator flags: the group has one usage text, and withGenFlags
// sits on the group node.
func groupCmd(group string) []cmdSpec {
	var out []cmdSpec
	for _, s := range generate.Specs() {
		g, sub, _ := strings.Cut(s.ID, " ")
		if g != group {
			continue
		}
		c := cmdSpec{Name: sub, Summary: s.Summary, Args: specArgs(s), Flags: specFlags(s)}
		if group == "new" {
			c.Usage = newGeneratorUsages[sub]
			c.Flags = withGenFlags(c.Flags...)
		}
		out = append(out, c)
	}
	return out
}

var hostFlagSpec = flagSpec{Name: "--host", Value: "name", Summary: "the host of a multi-host environment"}

// buildDirFlag is --build-dir, shared by the commands that write under build/.
var buildDirFlag = flagSpec{Name: "--build-dir", Value: "DIR", Summary: "put the build output under DIR instead of build/"}

// commandTree is `alder help --json`: what the plugin completes and offers in pickers.
var commandTree = []cmdSpec{
	{Name: "check", Summary: "type-check the app and print its diagnostics", Usage: commandUsages["check"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--json", Summary: "one JSON object per diagnostic, one per line"}}},
	{Name: "fmt", Summary: "format .alder files canonically", Usage: commandUsages["fmt"],
		Args: []string{"[path...]"},
		Flags: []flagSpec{{Name: "--check", Summary: "write nothing; list files that would be reformatted (exit 1)"},
			{Name: "--stdin", Summary: "format stdin to stdout"}}},
	{Name: "build", Summary: "compile the app to a single binary", Usage: commandUsages["build"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--emit", Summary: "write the generated Go source and stop"}, buildDirFlag}},
	{Name: "run", Summary: "run the app with live reload", Usage: commandUsages["run"], Args: []string{"[dir]"},
		Flags: []flagSpec{buildDirFlag}},
	{Name: "routes", Summary: "list the app's routes: pages (GET) and messages (POST)", Usage: commandUsages["routes"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--json", Summary: "a JSON array of routes with file, line and col"}}},
	{Name: "dump", Summary: "print a file's tokens or syntax tree, or an app's IR", Usage: commandUsages["dump"],
		Args: []string{"<file.alder | dir>"},
		Flags: []flagSpec{
			{Name: "--tokens", Summary: "the tokens"},
			{Name: "--ast", Summary: "the syntax tree"},
			{Name: "--ir", Summary: "the typed IR"},
		}},
	{Name: "eval", Summary: "evaluate an expression in the app", Usage: commandUsages["eval"],
		Args: []string{"[dir]", "<expr | ->"},
		Flags: []flagSpec{
			{Name: "--run", Summary: "run the effect the expression describes"},
			{Name: "--json", Summary: "print the result as JSON"},
			{Name: "--in", Value: "Name", Summary: "evaluate inside this module: its names unqualified"},
			buildDirFlag,
			{Name: "--module", Value: "Name", Summary: "name eval's own scratch module (default Scratch)"},
		}},
	{Name: "scratch", Summary: "run a scratch file's watches against the app", Usage: commandUsages["scratch"],
		Args: []string{"[file]"},
		Flags: []flagSpec{
			{Name: "--watch", Summary: "run again whenever the file or the app changes"},
			{Name: "--json", Summary: "print the result as JSON"},
			buildDirFlag,
			{Name: "--module", Value: "Name", Summary: "name the scratch module (default Scratch)"},
		}},
	{Name: "test", Summary: "run the app's tests", Usage: commandUsages["test"],
		Args: []string{"[dir]"},
		Flags: []flagSpec{
			{Name: "-run", Value: "re", Summary: "only the tests whose name matches"},
			{Name: "-timeout", Value: "d", Summary: "fail a test that runs longer"},
			{Name: "--json", Summary: "one JSON object per result"},
			{Name: "--fuzz", Value: "N", Summary: "runs per property test (default 100)"},
			{Name: "--seed", Value: "S", Summary: "the seed of the property tests' random cases; saved failures are replayed first regardless"},
			{Name: "--mutate", Summary: "mutation testing: list the mutants no test kills"},
			{Name: "--min", Value: "R", Summary: "with --mutate, fail below this mutation score"},
			buildDirFlag,
		}},
	{Name: "migrate", Summary: "write the migration for a schema change", Usage: commandUsages["migrate"],
		Args: []string{"[dir]"},
		Flags: []flagSpec{
			{Name: "--name", Value: "slug", Summary: "the migration's name"},
			{Name: "--dry-run", Summary: "print the migration; write nothing"},
		},
		Subcommands: []cmdSpec{
			{Name: "silos", Summary: "migrate every silo of an environment", Usage: migrateSilosUsage,
				Args: []string{"<env>", "[dir]"},
				Flags: []flagSpec{hostFlagSpec,
					{Name: "--allow-data-loss", Summary: "apply a migration that drops data"}}},
		}},
	{Name: "repl", Summary: "an interactive session in the app (deprecated: use alder scratch)", Usage: commandUsages["repl"],
		Args: []string{"[dir]"},
		Flags: []flagSpec{
			{Name: "--in", Value: "Name", Summary: "start inside this module: its names unqualified"},
			{Name: "--module", Value: "Name", Summary: "name the REPL's own scratch module (default Scratch)"},
			buildDirFlag,
		}},
	{Name: "new", Summary: "create an app, or generate code in one", Usage: commandUsages["new"],
		Args:        []string{"<name>"},
		Flags:       []flagSpec{{Name: "--bare", Summary: "the app without its starter pages"}},
		Subcommands: groupCmd("new")},
	{Name: "edit", Summary: "change a table, page, declaration, auth or the session", Usage: commandUsages["edit"],
		Flags:       withGenFlags(),
		Subcommands: groupCmd("edit")},
	{Name: "remove", Summary: "remove a page or a table", Usage: commandUsages["remove"],
		Flags:       withGenFlags(),
		Subcommands: groupCmd("remove")},
	{Name: "fill", Summary: "fill a typed hole with an expression from stdin", Usage: commandUsages["fill"],
		Args: []string{"<Module>.<decl>[.<arm>]", "-"},
		Flags: withGenFlags(
			flagSpec{Name: "--hole", Value: "?name", Summary: "the hole to fill when the declaration has several"},
			flagSpec{Name: "--decl", Summary: "replace the whole declaration body"},
			flagSpec{Name: "--list", Summary: "list the app's holes"},
		)},
	{Name: "regen", Summary: "regenerate generated code", Usage: commandUsages["regen"],
		Flags:       withGenFlags(specFlags(generate.SpecByID("regen admin"))...),
		Subcommands: []cmdSpec{{Name: "admin", Summary: generate.SpecByID("regen admin").Summary}}},
	{Name: "deploy", Summary: "deploy to an environment", Usage: commandUsages["deploy"],
		Args: []string{"<env>", "[dir]"},
		Flags: []flagSpec{hostFlagSpec,
			{Name: "--allow-data-loss", Summary: "apply a migration that drops data"},
			{Name: "--unsafe-skip-tests", Summary: "deploy without running the tests"},
			{Name: "--dry-run", Summary: "print the plan; change nothing"},
			{Name: "--json", Summary: "print the result as JSON"},
			buildDirFlag,
		},
		Subcommands: []cmdSpec{
			{Name: "init", Summary: "write the Dockerfile and Render or Fly config", Args: []string{"render|fly", "[dir]"},
				Flags: []flagSpec{
					{Name: "--force", Summary: "overwrite existing files"},
					{Name: "--dry-run", Summary: "print the files; write nothing"},
				}},
		}},
	{Name: "status", Summary: "show what an environment runs", Usage: commandUsages["status"],
		Args: []string{"<env>", "[dir]"}, Flags: []flagSpec{hostFlagSpec}},
	{Name: "rollback", Summary: "go back to an earlier build", Usage: commandUsages["rollback"],
		Args: []string{"<env>", "[<build id>]"},
		Flags: []flagSpec{
			{Name: "--dir", Value: "dir", Summary: "the app root"},
			hostFlagSpec,
			{Name: "--allow-schema-mismatch", Summary: "roll back across a schema change"},
			{Name: "--restore-backup", Summary: "also put back the database copy taken when that build was replaced"},
			{Name: "--yes", Summary: "do not ask for confirmation"},
		}},
	{Name: "logs", Summary: "print an environment's logs", Usage: commandUsages["logs"],
		Args:  []string{"<env>", "[dir]"},
		Flags: []flagSpec{hostFlagSpec, {Name: "-f", Summary: "follow"}}},
	{Name: "restore", Summary: "restore a database from a backup", Usage: commandUsages["restore"],
		Args: []string{"<env>"},
		Flags: []flagSpec{
			{Name: "--silo", Value: "key:id", Summary: "one silo's database"},
			{Name: "--from", Value: "url", Summary: "the backup to restore from"},
			{Name: "--to", Value: "time", Summary: "the point in time"},
			{Name: "--out", Value: "file.db", Summary: "write to a file instead of the live database"},
			{Name: "--dir", Value: "dir", Summary: "the app root"},
			{Name: "--yes", Summary: "do not ask for confirmation"},
		}},
	{Name: "silos", Summary: "manage an environment's silos", Usage: commandUsages["silos"],
		Subcommands: []cmdSpec{
			{Name: "prune", Summary: "delete the databases of silos that no longer exist",
				Args:  []string{"<env>", "[dir]"},
				Flags: []flagSpec{hostFlagSpec, {Name: "--yes", Summary: "do not ask for confirmation"}}},
		}},
	{Name: "ci", Summary: "set up continuous integration", Usage: commandUsages["ci"],
		Subcommands: []cmdSpec{
			{Name: "init", Summary: "write the CI workflow", Args: []string{"[dir]"},
				Flags: []flagSpec{
					{Name: "--github", Summary: "a GitHub Actions workflow"},
					{Name: "--force", Summary: "overwrite an existing workflow"},
				}},
		}},
	{Name: "version", Summary: "print alder's version and the Go it was built with", Usage: commandUsages["version"]},
	{Name: "lsp", Summary: "language server for editors, over stdio", Usage: commandUsages["lsp"]},
	{Name: "explain", Summary: "explain a diagnostic code", Usage: commandUsages["explain"], Args: []string{"<code>"}},
	{Name: "help", Summary: "print usage", Usage: commandUsages["help"],
		Args:  []string{"[<command> [<generator>]]"},
		Flags: []flagSpec{{Name: "--json", Summary: "the command tree as JSON"}}},
}

// filled returns c with nil Args, Flags and Subcommands' slices replaced by empty ones, so
// the JSON always has arrays.
func filled(cs []cmdSpec) []cmdSpec {
	out := make([]cmdSpec, len(cs))
	for i, c := range cs {
		if c.Args == nil {
			c.Args = []string{}
		}
		if c.Flags == nil {
			c.Flags = []flagSpec{}
		}
		c.Subcommands = filled(c.Subcommands)
		if len(c.Subcommands) == 0 {
			c.Subcommands = nil
		}
		out[i] = c
	}
	return out
}

// writeHelpJSON prints `alder help --json`.
func writeHelpJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(struct {
		Version  string    `json:"version"`
		Commands []cmdSpec `json:"commands"`
	}{version.String(), filled(commandTree)})
}
