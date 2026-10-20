package generate

import (
	"errors"
	"fmt"
	"strings"
)

// ArgKind is what a generator argument or flag value names; it drives help --json and, later,
// completion. (Distinct from Kind, the column kind of a table field.)
type ArgKind int

const (
	ArgText      ArgKind = iota
	ArgModule            // a module name, new or existing
	ArgPage              // an existing page module
	ArgTable             // an existing table
	ArgDecl              // Module.name of an existing declaration
	ArgFieldList         // name:Type[=default][@attr…] words
	ArgType              // a type expression
	ArgRoute             // a URL path starting with /
	ArgPath
	ArgBool
	ArgChoice // one of Field.Choices
)

var argKindNames = [...]string{"text", "module", "page", "table", "decl", "fieldList", "type", "route", "path", "bool", "choice"}

func (k ArgKind) String() string {
	if k < 0 || int(k) >= len(argKindNames) {
		return "text"
	}
	return argKindNames[k]
}

// Field is one argument or flag of a generator.
type Field struct {
	Name     string // the Sets key; a flag's name without dashes
	Kind     ArgKind
	Required bool
	Variadic bool     // an arg: takes the rest (Required: at least one); a flag: may repeat
	Greedy   bool     // a flag: takes every following word up to the next starting with "-" or "/" (--from)
	Table    bool     // ArgFieldList: a table's fields (attributes allowed)
	Choices  []string // ArgChoice
	// Default is the editor's starting value when neither the cursor (here) nor the user set
	// the field: "true" for a bool flag the form should start checked. Only Form applies it;
	// the CLI's default is always unset, so agents get the minimal result.
	Default string
	// DefaultUnless names fields that cancel Default: Form applies Default only when none of
	// them has a value (a bool counts as set unless "false" or ""), so an editor does not start
	// a combination the generator refuses.
	DefaultUnless []string
	Value         string // help --json: an arg's display ("<Module>") or a flag's placeholder ("T")
	Summary       string // help --json: a flag's summary
}

// Sets are the parsed values of a generator's arguments and flags, by Field.Name.
type Sets map[string][]string

// One is the last value of name, or "".
func (s Sets) One(name string) string {
	if v := s[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

// All is every value of name.
func (s Sets) All(name string) []string { return s[name] }

// Has reports whether name was given.
func (s Sets) Has(name string) bool { return len(s[name]) > 0 }

// App is what a generator runs against.
type App struct {
	Dir     string
	Migrate func(dir, slug string) ([]string, error) // slug "" = migrateCopy's default name
}

// Spec describes one generator command: its ID ("new page"), its arguments and flags, and
// how to run it.
type Spec struct {
	ID, Summary string
	Args, Flags []Field
	Here        func(HereCtx) *Match // nil: never "here"
	Run         func(sets Sets, app App, dryRun bool) (*Result, error)
}

// CLIFlags are the flags every generator takes.
type CLIFlags struct {
	Dir          string // default "."
	JSON, DryRun bool
}

// ErrArity is returned when a command has too few or too many positional arguments.
var ErrArity = errors.New("wrong number of arguments")

// ScanArgs splits args. valued names the flags that take a value (`--list T` or `--list=T`);
// multi, those that take every argument up to the next flag or path (`--from A B`); boolean
// flags besides --json and --dry-run are in bools. An unknown flag is an error.
func ScanArgs(args []string, valued, multi, bools []string) (pos []string, flags map[string][]string, cli CLIFlags, err error) {
	cli.Dir = "."
	flags = map[string][]string{}
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
			pos = append(pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=")
		switch {
		case name == "json" && !hasVal:
			cli.JSON = true
		case name == "dry-run" && !hasVal:
			cli.DryRun = true
		case is(bools, name) && !hasVal:
			flags[name] = append(flags[name], "")
		case name == "dir" || is(valued, name):
			if !hasVal {
				if i+1 >= len(args) {
					return pos, flags, cli, fmt.Errorf("--%s needs a value", name)
				}
				i++
				val = args[i]
			}
			if name == "dir" {
				cli.Dir = val
			} else {
				flags[name] = append(flags[name], val)
			}
		case is(multi, name):
			if hasVal {
				flags[name] = append(flags[name], val)
			}
			// up to the next flag, or a path (`--from Post Tag /dash`)
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && !strings.HasPrefix(args[i+1], "/") {
				i++
				flags[name] = append(flags[name], args[i])
			}
			if len(flags[name]) == 0 {
				return pos, flags, cli, fmt.Errorf("--%s needs at least one value", name)
			}
		default:
			return pos, flags, cli, fmt.Errorf("unknown flag %s", a)
		}
	}
	return pos, flags, cli, nil
}

// ParseArgs parses a generator's command line (the words after its ID) by s.
func ParseArgs(s *Spec, argv []string) (Sets, CLIFlags, error) {
	var valued, multi, bools []string
	for _, f := range s.Flags {
		switch {
		case f.Kind == ArgBool:
			bools = append(bools, f.Name)
		case f.Greedy:
			multi = append(multi, f.Name)
		default:
			valued = append(valued, f.Name)
		}
	}
	pos, flags, cli, err := ScanArgs(argv, valued, multi, bools)
	if err != nil {
		return nil, cli, err
	}
	sets := Sets{}
	for k, v := range flags {
		sets[k] = v
	}
	for i, a := range s.Args {
		if a.Variadic {
			if len(pos) > i {
				sets[a.Name] = pos[i:]
			} else if a.Required {
				return nil, cli, ErrArity
			}
			pos = nil
			break
		}
		if i >= len(pos) {
			if a.Required {
				return nil, cli, ErrArity
			}
			continue
		}
		sets[a.Name] = []string{pos[i]}
	}
	if len(s.Args) == 0 || !s.Args[len(s.Args)-1].Variadic {
		if len(pos) > len(s.Args) {
			return nil, cli, ErrArity
		}
	}
	return sets, cli, nil
}

// Select picks the subcommand of a group (`edit`, `remove`, `regen`): the first word of argv
// that, after group, is a Spec's ID. It returns the Spec and argv without that word.
func Select(group string, argv []string, specs []*Spec) (*Spec, []string) {
	for i, a := range argv {
		if strings.HasPrefix(a, "-") {
			continue
		}
		for _, s := range specs {
			if s.ID == group+" "+a {
				rest := append(append([]string{}, argv[:i]...), argv[i+1:]...)
				return s, rest
			}
		}
	}
	return nil, argv
}
