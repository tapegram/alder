package generate

import "strings"

// The generators as Specs: the one description `alder help --json`, the CLI's argument parsing
// and the language server all read. Value and Summary strings are the CLI's help text.

// runEdits is the CLI's generator pipeline without the report: load the app, run the
// generator, apply its edits to a checked copy. load nil means Load (LoadWithErrors for
// o.WriteDespiteErrors).
func runEdits(app App, dryRun bool, o Options, load func(string) (*Program, error), gen func(*Program) ([]Edit, error)) (*Result, error) {
	if load == nil {
		load = Load
		if o.WriteDespiteErrors {
			load = LoadWithErrors
		}
	}
	p, err := load(app.Dir)
	if err != nil {
		return nil, err
	}
	edits, err := gen(p)
	if err != nil {
		return nil, err
	}
	o.DryRun = dryRun
	return Apply(app.Dir, edits, o)
}

// migrate is app's migration writer for slug, or a refusal when the caller gave none.
func (a App) migrate(slug string) func(string) ([]string, error) {
	return func(dir string) ([]string, error) {
		if a.Migrate == nil {
			return nil, Refusef("this generator writes a migration, and no migration writer was given")
		}
		return a.Migrate(dir, slug)
	}
}

// fieldList parses field words; a bad one is a refusal.
func fieldList(words []string, table bool) ([]FieldSpec, error) {
	var out []FieldSpec
	for _, w := range words {
		f, err := ParseField(w, table)
		if err != nil {
			return nil, Refusef("%v", err)
		}
		out = append(out, f)
	}
	return out, nil
}

var specs []*Spec

// Specs is every generator, in `help --json` order.
func Specs() []*Spec { return specs }

// SpecByID is the Spec with the given ID ("new page"), or nil.
func SpecByID(id string) *Spec {
	for _, s := range specs {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func init() {
	specs = append(specs, newSpecs()...)
	specs = append(specs, editSpecs()...)
	specs = append(specs, removeSpecs()...)
	specs = append(specs, regenSpecs()...)
}

func newSpecs() []*Spec {
	return []*Spec{
		{
			ID: "new page", Summary: "a page module",
			Args: []Field{
				{Name: "module", Kind: ArgModule, Required: true, Value: "<Module>"},
				{Name: "path", Kind: ArgRoute, Required: true, Value: "<path>"},
			},
			Flags: []Field{
				{Name: "list", Kind: ArgTable, Value: "T", Summary: "a page listing the rows of table T"},
				{Name: "show", Kind: ArgTable, Value: "T", Summary: "a page showing one row of table T"},
				{Name: "by", Kind: ArgText, Value: "column", Summary: "with --show, the unique column in the path"},
				{Name: "from", Kind: ArgTable, Greedy: true, Value: "T…", Summary: "a card of every row of each table T; takes several tables up to the next flag (--from A B)"},
				{Name: "blank", Kind: ArgBool, Summary: "an empty page with typed holes"},
				{Name: "param", Kind: ArgText, Variadic: true, Value: "name:Type", Summary: "a page parameter (repeatable)"},
				{Name: "requires", Kind: ArgText, Value: "user", Summary: "require a signed-in user"},
				{Name: "role", Kind: ArgText, Value: "R", Summary: "require role R"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				o := PageOptions{Module: s.One("module"), Path: s.One("path"), List: s.One("list"), Show: s.One("show"), From: s.All("from"),
					Requires: s.One("requires"), Role: s.One("role"), Blank: s.Has("blank"), Params: s.All("param"), By: s.One("by")}
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return NewPage(p, o) })
			},
		},
		{
			ID: "new table", Summary: "a table and its migration",
			Args: []Field{
				{Name: "name", Kind: ArgModule, Required: true, Value: "<Name>"},
				{Name: "fields", Kind: ArgFieldList, Variadic: true, Table: true, Value: "<field>:<Type>…"}, // not Required: `new table Tag` is accepted
			},
			Flags: []Field{
				{Name: "no-timestamps", Kind: ArgBool, Summary: "without created and updated columns"},
				{Name: "silo", Kind: ArgBool, Summary: "put the table in its key's silo in App"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				fields, err := fieldList(s.All("fields"), true)
				if err != nil {
					return nil, err
				}
				return runEdits(app, dry, Options{Migrate: app.migrate("")}, nil, func(p *Program) ([]Edit, error) {
					return NewTableWith(p, s.One("name"), fields, TableOptions{NoTimestamps: s.Has("no-timestamps"), Silo: s.Has("silo")})
				})
			},
		},
		{
			ID: "new form", Summary: "a form with its msg, handler and view",
			Args: []Field{
				{Name: "page", Kind: ArgPage, Required: true, Value: "<Page module>"},
				{Name: "name", Kind: ArgText, Required: true, Value: "<Name>"},
				{Name: "fields", Kind: ArgFieldList, Variadic: true, Value: "<field>:<Type>…"},
			},
			Flags: []Field{
				{Name: "into", Kind: ArgTable, Value: "T", Summary: "insert the submitted form into table T"},
				{Name: "msg", Kind: ArgText, Value: "Msg", Summary: "the msg's name"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				fields, err := fieldList(s.All("fields"), false)
				if err != nil {
					return nil, err
				}
				o := FormOptions{Page: s.One("page"), Name: s.One("name"), Fields: fields, Into: s.One("into"), Msg: s.One("msg")}
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return NewForm(p, o) })
			},
		},
		{
			ID: "new msg", Summary: "a msg with its arm, button and test",
			Args: []Field{
				{Name: "page", Kind: ArgPage, Required: true, Value: "<Page module>"},
				{Name: "name", Kind: ArgText, Required: true, Value: "<Name>"},
				{Name: "fields", Kind: ArgFieldList, Variadic: true, Value: "<field>:<Type>…"},
			},
			Flags: []Field{
				{Name: "in-for", Kind: ArgText, Value: "binder", Summary: "the button goes inside this for loop"},
				{Name: "when", Kind: ArgText, Value: "Bool expression", Summary: "show the button only when this holds"},
				{Name: "id", Kind: ArgText, Value: "id", Summary: "the button's id"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				fields, err := fieldList(s.All("fields"), false)
				if err != nil {
					return nil, err
				}
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) {
					return NewMsg(p, s.One("page"), s.One("name"), fields, MsgOptions{InFor: s.One("in-for"), When: s.One("when"), ID: s.One("id")})
				})
			},
		},
		{
			ID: "new capability", Summary: "a capability entry with its fake and adapter",
			Args: []Field{
				{Name: "module", Kind: ArgModule, Required: true, Value: "<Module>"},
				{Name: "entry", Kind: ArgText, Required: true, Value: "<entry>"},
				{Name: "type", Kind: ArgType, Required: true, Value: "<Type>"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) {
					return NewCapability(p, s.One("module"), s.One("entry"), s.One("type"))
				})
			},
		},
		{
			ID: "new admin", Summary: "the admin pages for every table",
			Flags: []Field{
				{Name: "module", Kind: ArgModule, Value: "Admin", Summary: "the admin's module prefix"},
				{Name: "path", Kind: ArgRoute, Value: "/admin", Summary: "the admin's path prefix"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				o := AdminOptions{Module: s.One("module"), Path: s.One("path")}
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return NewAdmin(p, o) })
			},
		},
		{
			ID: "new auth", Summary: "sign-in, sign-out and the user table",
			Flags: []Field{
				{Name: "users", Kind: ArgTable, Value: "User", Summary: "the user table"},
				{Name: "role", Kind: ArgText, Value: "Role", Summary: "the role type"},
				{Name: "to", Kind: ArgPage, Value: "Page", Summary: "the page after sign-in"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				created := false
				o := Options{Migrate: func(dir string) ([]string, error) {
					if !created {
						return nil, nil
					}
					return app.migrate("")(dir)
				}}
				return runEdits(app, dry, o, nil, func(p *Program) ([]Edit, error) {
					edits, c, err := NewAuth(p, AuthOptions{Users: s.One("users"), Role: s.One("role"), To: s.One("to")})
					created = c
					return edits, err
				})
			},
		},
		{
			ID: "new type", Summary: "a type declaration",
			Args: []Field{
				{Name: "module", Kind: ArgModule, Required: true, Value: "<Module>"},
				{Name: "decl", Kind: ArgType, Required: true, Value: "<declaration>"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return NewType(p, s.One("module"), s.One("decl")) })
			},
		},
		{
			ID: "new decl", Summary: "a value declaration with a typed hole for its body",
			Args: []Field{
				{Name: "module", Kind: ArgModule, Required: true, Value: "<Module>"},
				{Name: "name", Kind: ArgText, Required: true, Value: "<name>"},
				{Name: "type", Kind: ArgType, Required: true, Value: "<Type>"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) {
					return NewDecl(p, s.One("module"), s.One("name"), s.One("type"))
				})
			},
		},
		{
			ID: "new test", Summary: "a test skeleton",
			Args: []Field{
				{Name: "module", Kind: ArgModule, Required: true, Value: "<Module>"},
				{Name: "name", Kind: ArgText, Required: true, Value: "<test name>"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return NewTest(p, s.One("module"), s.One("name")) })
			},
		},
		{
			ID: "new silo", Summary: "put tables in a database per value of a session key",
			Args: []Field{
				{Name: "key", Kind: ArgText, Required: true, Value: "<key>"},
				{Name: "tables", Kind: ArgTable, Variadic: true, Value: "<Table>…"},
			},
			Flags: []Field{{Name: "empty", Kind: ArgBool, Summary: "a silo with no tables yet"}},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				var notes []string
				o := Options{Migrate: func(string) ([]string, error) { return notes, nil }}
				return runEdits(app, dry, o, nil, func(p *Program) ([]Edit, error) {
					edits, n, err := NewSilo(p, s.One("key"), s.All("tables"), s.Has("empty"))
					notes = n
					return edits, err
				})
			},
		},
	}
}

// editTableSlug names the migration after the edit, whatever its steps (a rebuild is still
// add_<field>); "" lets the migration writer derive it.
func editTableSlug(s Sets) string {
	args := s.All("args")
	switch op := s.One("op"); {
	case op == "add-field" && len(args) == 1:
		name, _, _ := strings.Cut(args[0], ":")
		return "add_" + Snake(name)
	case op == "rename-field" && len(args) == 2:
		return "rename_" + Snake(args[0])
	case op == "drop-field" && len(args) == 1:
		return "drop_" + Snake(args[0])
	}
	return ""
}

func editSpecs() []*Spec {
	return []*Spec{
		{
			ID: "edit table", Summary: "add, rename or drop a field (and write the migration)",
			Args: []Field{
				{Name: "table", Kind: ArgTable, Required: true, Value: "<T>"},
				{Name: "op", Kind: ArgChoice, Required: true, Choices: []string{"add-field", "rename-field", "drop-field"}, Value: "add-field|rename-field|drop-field"},
				{Name: "args", Kind: ArgText, Variadic: true, Value: "…"}, // not Required: EditTable refuses a missing argument itself
			},
			Flags: []Field{{Name: "strict", Kind: ArgBool, Summary: "refuse a change that breaks code"}},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				o := Options{Migrate: app.migrate(editTableSlug(s)), WriteDespiteErrors: !s.Has("strict")}
				return runEdits(app, dry, o, nil, func(p *Program) ([]Edit, error) { return EditTable(p, s.One("table"), s.One("op"), s.All("args")) })
			},
		},
		{
			ID: "edit page", Summary: "add a page parameter",
			Args: []Field{
				{Name: "page", Kind: ArgPage, Required: true, Value: "<Page module>"},
				{Name: "op", Kind: ArgChoice, Required: true, Choices: []string{"add-param"}, Value: "add-param"},
				{Name: "field", Kind: ArgFieldList, Required: true, Value: "<name>:<Type>[=default]"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return EditPage(p, s.One("page"), s.One("op"), s.All("field")) })
			},
		},
		{
			ID: "edit decl", Summary: "add a component prop",
			Args: []Field{
				{Name: "address", Kind: ArgDecl, Required: true, Value: "<Module>.<Name>"},
				{Name: "op", Kind: ArgChoice, Required: true, Choices: []string{"add-prop"}, Value: "add-prop"},
				{Name: "field", Kind: ArgFieldList, Required: true, Value: "<name>:<Type>=<default>"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return EditDecl(p, s.One("address"), s.One("op"), s.All("field")) })
			},
		},
		{
			ID: "edit auth", Summary: "change where sign-in leads",
			Flags: []Field{{Name: "to", Kind: ArgPage, Value: "Page", Summary: "the page after sign-in"}},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return EditAuth(p, s.One("to")) })
			},
		},
		{
			ID: "edit session", Summary: "add a session field",
			Args: []Field{
				{Name: "op", Kind: ArgChoice, Required: true, Choices: []string{"add-field"}, Value: "add-field"},
				{Name: "field", Kind: ArgText, Required: true, Variadic: true, Value: "<name>:<Type>=<default> | <key>:<T>.<T>Id via <Membership>"},
			},
			Flags: []Field{
				{Name: "choose", Kind: ArgPage, Value: "Page", Summary: "the page that chooses the key"},
				{Name: "strict", Kind: ArgBool, Summary: "refuse a change that breaks code"},
			},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				// the session literals the new field breaks (a test's `{ user: me }`) are the to-do
				// list, as a table edit's are; --strict refuses instead
				return runEdits(app, dry, Options{WriteDespiteErrors: !s.Has("strict")}, nil, func(p *Program) ([]Edit, error) {
					return EditSessionChoose(p, s.One("op"), s.All("field"), s.One("choose"))
				})
			},
		},
	}
}

func removeSpecs() []*Spec {
	return []*Spec{
		{
			ID: "remove page", Summary: "remove a page module",
			Args: []Field{{Name: "page", Kind: ArgPage, Required: true, Value: "<Page module>"}},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				return runEdits(app, dry, Options{}, nil, func(p *Program) ([]Edit, error) { return RemovePage(p, s.One("page")) })
			},
		},
		{
			ID: "remove table", Summary: "remove a table (and write the migration)",
			Args: []Field{{Name: "table", Kind: ArgTable, Required: true, Value: "<T>"}},
			Run: func(s Sets, app App, dry bool) (*Result, error) {
				o := Options{Migrate: app.migrate("drop_" + Snake(s.One("table")))}
				return runEdits(app, dry, o, nil, func(p *Program) ([]Edit, error) { return RemoveTable(p, s.One("table")) })
			},
		},
	}
}

func regenSpecs() []*Spec {
	return []*Spec{{
		ID: "regen admin", Summary: "the admin pages",
		Flags: []Field{
			{Name: "module", Kind: ArgModule, Value: "Admin", Summary: "the admin's module prefix"},
			{Name: "path", Kind: ArgRoute, Value: "/admin", Summary: "the admin's path prefix"},
			{Name: "all", Kind: ArgBool, Summary: "recreate the admin pages the author deleted"},
		},
		Run: func(s Sets, app App, dry bool) (*Result, error) {
			o := AdminOptions{Module: s.One("module"), Path: s.One("path")}
			skipped, nothing := []string{}, false
			load := func(dir string) (*Program, error) { return LoadExcept(dir, AdminDir(o.Module)) }
			res, err := runEdits(app, dry, Options{}, load, func(p *Program) ([]Edit, error) {
				edits, sk, err := RegenAdmin(p, o, s.Has("all"))
				if sk != nil {
					skipped = sk
				}
				if err == nil && len(edits) == 0 {
					nothing = true
				}
				return edits, err
			})
			if res != nil {
				res.Skipped, res.UpToDate = skipped, nothing
			}
			return res, err
		},
	}}
}
