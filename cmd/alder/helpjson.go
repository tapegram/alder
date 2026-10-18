package main

import (
	"encoding/json"
	"io"

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

var hostFlagSpec = flagSpec{Name: "--host", Value: "name", Summary: "the host of a multi-host environment"}

// commandTree is `alder help --json`: what the plugin completes and offers in pickers.
var commandTree = []cmdSpec{
	{Name: "check", Summary: "type-check the app and print its diagnostics", Usage: commandUsages["check"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--json", Summary: "one JSON object per diagnostic, one per line"}}},
	{Name: "build", Summary: "compile the app to a single binary", Usage: commandUsages["build"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--emit", Summary: "write the generated Go source and stop"}}},
	{Name: "run", Summary: "run the app with live reload", Usage: commandUsages["run"], Args: []string{"[dir]"}},
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
			{Name: "--module", Value: "Name", Summary: "evaluate inside this module"},
		}},
	{Name: "test", Summary: "run the app's tests", Usage: commandUsages["test"],
		Args: []string{"[dir]"},
		Flags: []flagSpec{
			{Name: "-run", Value: "re", Summary: "only the tests whose name matches"},
			{Name: "-timeout", Value: "d", Summary: "fail a test that runs longer"},
			{Name: "--json", Summary: "one JSON object per result"},
			{Name: "--mutate", Summary: "mutation testing: list the mutants no test kills"},
			{Name: "--min", Value: "R", Summary: "with --mutate, fail below this mutation score"},
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
	{Name: "repl", Summary: "an interactive session in the app", Usage: commandUsages["repl"],
		Args:  []string{"[dir]"},
		Flags: []flagSpec{{Name: "--module", Value: "Name", Summary: "start inside this module"}}},
	{Name: "new", Summary: "create an app, or generate code in one", Usage: commandUsages["new"],
		Args:  []string{"<name>"},
		Flags: []flagSpec{{Name: "--bare", Summary: "the app without its starter pages"}},
		Subcommands: []cmdSpec{
			{Name: "page", Summary: "a page module", Usage: newGeneratorUsages["page"],
				Args: []string{"<Module>", "<path>"},
				Flags: withGenFlags(
					flagSpec{Name: "--list", Value: "T", Summary: "a page listing the rows of table T"},
					flagSpec{Name: "--show", Value: "T", Summary: "a page showing one row of table T"},
					flagSpec{Name: "--by", Value: "column", Summary: "with --show, the unique column in the path"},
					flagSpec{Name: "--from", Value: "T…", Summary: "a card of every row of each table T; takes several tables up to the next flag (--from A B)"},
					flagSpec{Name: "--blank", Summary: "an empty page with typed holes"},
					flagSpec{Name: "--param", Value: "name:Type", Multi: true, Summary: "a page parameter (repeatable)"},
					flagSpec{Name: "--requires", Value: "user", Summary: "require a signed-in user"},
					flagSpec{Name: "--role", Value: "R", Summary: "require role R"},
				)},
			{Name: "table", Summary: "a table and its migration", Usage: newGeneratorUsages["table"],
				Args: []string{"<Name>", "<field>:<Type>…"},
				Flags: withGenFlags(
					flagSpec{Name: "--no-timestamps", Summary: "without created and updated columns"},
					flagSpec{Name: "--silo", Summary: "put the table in its key's silo in App"},
				)},
			{Name: "form", Summary: "a form with its msg, handler and view", Usage: newGeneratorUsages["form"],
				Args: []string{"<Page module>", "<Name>", "<field>:<Type>…"},
				Flags: withGenFlags(
					flagSpec{Name: "--into", Value: "T", Summary: "insert the submitted form into table T"},
					flagSpec{Name: "--msg", Value: "Msg", Summary: "the msg's name"},
				)},
			{Name: "msg", Summary: "a msg with its arm, button and test", Usage: newGeneratorUsages["msg"],
				Args: []string{"<Page module>", "<Name>", "<field>:<Type>…"},
				Flags: withGenFlags(
					flagSpec{Name: "--in-for", Value: "binder", Summary: "the button goes inside this for loop"},
					flagSpec{Name: "--when", Value: "Bool expression", Summary: "show the button only when this holds"},
					flagSpec{Name: "--id", Value: "id", Summary: "the button's id"},
				)},
			{Name: "capability", Summary: "a capability entry with its fake and adapter", Usage: newGeneratorUsages["capability"],
				Args: []string{"<Module>", "<entry>", "<Type>"}, Flags: withGenFlags()},
			{Name: "admin", Summary: "the admin pages for every table", Usage: newGeneratorUsages["admin"],
				Flags: withGenFlags(
					flagSpec{Name: "--module", Value: "Admin", Summary: "the admin's module prefix"},
					flagSpec{Name: "--path", Value: "/admin", Summary: "the admin's path prefix"},
				)},
			{Name: "auth", Summary: "sign-in, sign-out and the user table", Usage: newGeneratorUsages["auth"],
				Flags: withGenFlags(
					flagSpec{Name: "--users", Value: "User", Summary: "the user table"},
					flagSpec{Name: "--role", Value: "Role", Summary: "the role type"},
					flagSpec{Name: "--to", Value: "Page", Summary: "the page after sign-in"},
				)},
			{Name: "type", Summary: "a type declaration", Usage: newGeneratorUsages["type"],
				Args: []string{"<Module>", "<declaration>"}, Flags: withGenFlags()},
			{Name: "decl", Summary: "a value declaration with a typed hole for its body", Usage: newGeneratorUsages["decl"],
				Args: []string{"<Module>", "<name>", "<Type>"}, Flags: withGenFlags()},
			{Name: "test", Summary: "a test skeleton", Usage: newGeneratorUsages["test"],
				Args: []string{"<Module>", "<test name>"}, Flags: withGenFlags()},
			{Name: "silo", Summary: "put tables in a database per value of a session key", Usage: newGeneratorUsages["silo"],
				Args:  []string{"<key>", "<Table>…"},
				Flags: withGenFlags(flagSpec{Name: "--empty", Summary: "a silo with no tables yet"})},
		}},
	{Name: "edit", Summary: "change a table, page, declaration, auth or the session", Usage: commandUsages["edit"],
		Flags: withGenFlags(),
		Subcommands: []cmdSpec{
			{Name: "table", Summary: "add, rename or drop a field (and write the migration)",
				Args:  []string{"<T>", "add-field|rename-field|drop-field", "…"},
				Flags: []flagSpec{{Name: "--strict", Summary: "refuse a change that breaks code"}}},
			{Name: "page", Summary: "add a page parameter", Args: []string{"<Page module>", "add-param", "<name>:<Type>[=default]"}},
			{Name: "decl", Summary: "add a component prop", Args: []string{"<Module>.<Name>", "add-prop", "<name>:<Type>=<default>"}},
			{Name: "auth", Summary: "change where sign-in leads",
				Flags: []flagSpec{{Name: "--to", Value: "Page", Summary: "the page after sign-in"}}},
			{Name: "session", Summary: "add a session field",
				Args: []string{"add-field", "<name>:<Type>=<default> | <key>:<T>.<T>Id via <Membership>"},
				Flags: []flagSpec{
					{Name: "--choose", Value: "Page", Summary: "the page that chooses the key"},
					{Name: "--strict", Summary: "refuse a change that breaks code"},
				}},
		}},
	{Name: "remove", Summary: "remove a page or a table", Usage: commandUsages["remove"],
		Flags: withGenFlags(),
		Subcommands: []cmdSpec{
			{Name: "page", Summary: "remove a page module", Args: []string{"<Page module>"}},
			{Name: "table", Summary: "remove a table (and write the migration)", Args: []string{"<T>"}},
		}},
	{Name: "fill", Summary: "fill a typed hole with an expression from stdin", Usage: commandUsages["fill"],
		Args: []string{"<Module>.<decl>[.<arm>]", "-"},
		Flags: withGenFlags(
			flagSpec{Name: "--hole", Value: "?name", Summary: "the hole to fill when the declaration has several"},
			flagSpec{Name: "--decl", Summary: "replace the whole declaration body"},
			flagSpec{Name: "--list", Summary: "list the app's holes"},
		)},
	{Name: "regen", Summary: "regenerate generated code", Usage: commandUsages["regen"],
		Flags: withGenFlags(
			flagSpec{Name: "--module", Value: "Admin", Summary: "the admin's module prefix"},
			flagSpec{Name: "--path", Value: "/admin", Summary: "the admin's path prefix"},
			flagSpec{Name: "--all", Summary: "recreate the admin pages the author deleted"},
		),
		Subcommands: []cmdSpec{{Name: "admin", Summary: "the admin pages"}}},
	{Name: "deploy", Summary: "deploy to an environment", Usage: commandUsages["deploy"],
		Args: []string{"<env>", "[dir]"},
		Flags: []flagSpec{hostFlagSpec,
			{Name: "--allow-data-loss", Summary: "apply a migration that drops data"},
			{Name: "--unsafe-skip-tests", Summary: "deploy without running the tests"},
			{Name: "--dry-run", Summary: "print the plan; change nothing"},
			{Name: "--json", Summary: "print the result as JSON"},
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
