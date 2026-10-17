package gen

import (
	"fmt"
	"go/format"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// Capabilities and the app file.
//
// A capability is implemented in Go, outside the language, by an adapter: capability C of
// module M is returned by func AdapterFunc(M, C) of package adapters/AdapterPkg(M) (names.go),
// and the generated package caps (CapsPackage) calls every such constructor. Its surface is
// exported, in the module's file:
//
//	type Cap__Auth__Login__Password interface {
//		Verify(ctx context.Context, args Args__Auth__Login__Password__Verify) (rt.Maybe[string], error)
//	}
//	type Args__Auth__Login__Password__Verify struct {
//		Email    string
//		Password string
//	}
//
// One method per entry, named by the entry, returning the entry's result and an error (a
// failure of the Task). Its parameters after the context: none for an entry that is itself a
// Query or Task; for one parameter, args of the record Args__M__C__Entry — the parameter's
// record when it is an anonymous one (a struct of the same fields, so an Alder record value is
// assignable to it), an alias of it when it is a named record, else struct{ Value T }; for
// several parameters (`limit : UserId -> Int -> Query Int`) one Go parameter each, a1, a2, ….
//
// app__alder.go holds Tables, Migrations and Capabilities and, for an app with an App module, Caps (one
// field per capability), Options and Serve, which wires everything into a web.NewRouter
// handler, and the session cookie codec.

// capArg is the Go type of the one parameter of a capability entry of type t (a function), and
// how its Args__ record is declared: "struct" for an anonymous record (its fields), "alias" for
// a named record, "wrap" for any other type (struct{ Value T }).
func capArg(t types.Type) (types.Type, string) {
	ft, ok := t.(*types.Fun)
	if !ok || len(ft.Params) != 1 {
		return nil, ""
	}
	p := ft.Params[0]
	if r, ok := p.(*types.Record); ok {
		if r.Name != "" && !r.Anonymous {
			return p, "alias"
		}
		return p, "struct"
	}
	return p, "wrap"
}

// capArgsWrapped reports whether entry of capability c takes its one argument wrapped in
// Args__…{Value: v}.
func capArgsWrapped(c *ir.Capability, entry string) bool {
	for _, e := range c.Entries {
		if e.Name == entry {
			_, kind := capArg(e.Type)
			return kind == "wrap"
		}
	}
	return false
}

// capDecl writes capability c's exported Go interface and its entries' Args__ records.
func capDecl(w *strings.Builder, c *ir.Capability) {
	name := capName(c.Module, c.Name)
	w.WriteString(comment(fmt.Sprintf("%s is capability %s of module %s, implemented by adapters/%s/ (func %s, wired by the generated caps package): one method per entry, called with the running request's context (db.TxFrom(ctx) is the running Task's transaction).", name, c.Name, c.Module, AdapterPkg(c.Module), AdapterFunc(c.Module, c.Name))))
	fmt.Fprintf(w, "type %s interface {\n", name)
	var args strings.Builder
	for _, e := range c.Entries {
		params := []string{"ctx context.Context"}
		res := e.Type
		if ft, ok := e.Type.(*types.Fun); ok {
			if p, kind := capArg(e.Type); p != nil {
				an := argsName(c.Module, c.Name, e.Name)
				params = append(params, "args "+an)
				fmt.Fprintf(&args, "// %s are the arguments of %s.%s.\n", an, c.Name, e.Name)
				switch kind {
				case "alias":
					fmt.Fprintf(&args, "type %s = %s\n\n", an, GoType(p))
				case "struct":
					var fs []string
					for _, f := range p.(*types.Record).Fields {
						fs = append(fs, Field(f.Name)+" "+GoType(f.Type))
					}
					if len(fs) == 0 {
						fmt.Fprintf(&args, "type %s struct{}\n\n", an)
					} else {
						fmt.Fprintf(&args, "type %s struct {\n%s\n}\n\n", an, strings.Join(fs, "\n"))
					}
				default:
					fmt.Fprintf(&args, "type %s struct {\nValue %s\n}\n\n", an, GoType(p))
				}
			} else {
				for i, p := range ft.Params {
					params = append(params, "a"+strconv.Itoa(i+1)+" "+GoType(p))
				}
			}
			res = ft.Result
		}
		fmt.Fprintf(w, "%s(%s) (%s, error)\n", Field(e.Name), strings.Join(params, ", "), GoType(effResult(res)))
	}
	w.WriteString("}\n\n")
	w.WriteString(args.String())
}

// fakeDecl writes, for a test build, the struct implementing fk's capability with fk's entries:
// each method runs its entry (fakeEntry) inside the effect that called the capability, through
// eff.RunInner on the Env that ctx carries (eff.EnvFrom), so the entry's steps are on the
// calling test's trace, its writes in its transaction (under Test.run's savepoint) and its
// failure the call's. The method takes its arguments as the capability's interface does
// (capDecl): one Args__ record (passed on as it is, or its Value when wrapped), several a1, a2, …,
// or none.
//
//	type fake__Payments__Stripe struct{}
//
//	func (fake__Payments__Stripe) Charge(ctx context.Context, args Args__Payments__Stripe__Charge) (rt.Result[string, string], error) {
//		env := eff.EnvFrom(ctx)
//		if env == nil {
//			var zero rt.Result[string, string]
//			return zero, errors.New("fake Stripe called outside a running effect")
//		}
//		return eff.RunInner(env, fakeentry__Payments__Stripe__Charge(args))
//	}
func fakeDecl(w *strings.Builder, fk *ir.Fake) {
	c := fk.Capability
	name := fakeName(c.Module, c.Name)
	w.WriteString(comment(fmt.Sprintf("%s is `fake %s` of module %s (test builds only): it implements %s by running the fake's entries inside the effect that calls them (eff.RunInner on the Env eff.EnvFrom finds in ctx), so their steps, writes and failures are the calling test's. Test.fake %s installs it.",
		name, c.Name, c.Module, capName(c.Module, c.Name), c.Name)))
	fmt.Fprintf(w, "type %s struct{}\n\n", name)
	for i, e := range c.Entries {
		params := []string{"ctx context.Context"}
		var args []string
		res := e.Type
		ft, isFun := e.Type.(*types.Fun)
		if isFun {
			if p, kind := capArg(e.Type); p != nil {
				params = append(params, "args "+argsName(c.Module, c.Name, e.Name))
				if kind == "wrap" {
					args = append(args, "args.Value")
				} else {
					args = append(args, "args")
				}
			} else {
				for j, p := range ft.Params {
					a := "a" + strconv.Itoa(j+1)
					params = append(params, a+" "+GoType(p))
					args = append(args, a)
				}
			}
			res = ft.Result
		}
		entry := fakeEntry(c.Module, c.Name, fk.Entries[i].Name)
		if isFun {
			entry += "(" + strings.Join(args, ", ") + ")"
		}
		if isQuery(res) {
			entry = "eff.T(" + entry + ")"
		}
		r := GoType(effResult(res))
		fmt.Fprintf(w, "func (%s) %s(%s) (%s, error) {\nenv := eff.EnvFrom(ctx)\nif env == nil {\nvar zero %s\nreturn zero, errors.New(%s)\n}\nreturn eff.RunInner(env, %s)\n}\n\n",
			name, Field(e.Name), strings.Join(params, ", "), r, r, strconv.Quote("fake "+c.Name+" called outside a running effect"), entry)
	}
}

// fakesTable writes, for a test build, FakeTables (the descriptors of every fake's tables, in
// the order of Program.Fakes, then declaration order: rtesting creates them after the app's,
// db.SQLite.CreateTables) and Fakes (each fake's constructor by its capability's Env.Caps key,
// for Test.fake).
func fakesTable(b *strings.Builder, p *ir.Program) {
	var ds, fs []string
	for _, fk := range p.Fakes {
		for _, tb := range fk.Tables {
			ds = append(ds, tableName(tb.Module, tb.Name)+".Desc()")
		}
		c := fk.Capability
		fs = append(fs, fmt.Sprintf("%s: func() any { return %s{} },\n", strconv.Quote(c.Module+"."+c.Name), fakeName(c.Module, c.Name)))
	}
	b.WriteString("// FakeTables are the test-only tables of the app's fakes, which alder test's main creates in each\n" +
		"// test's database after Tables (rtesting.Options.TestTables). Test builds only.\n" +
		"var FakeTables = []db.TableDesc{" + strings.Join(ds, ", ") + "}\n\n")
	b.WriteString("// Fakes are the app's fakes by the Env.Caps key of their capability: Test.fake installs one in the\n" +
		"// running test's Env.Caps. Test builds only.\n")
	if len(fs) == 0 {
		b.WriteString("var Fakes = map[string]func() any{}\n\n")
	} else {
		b.WriteString("var Fakes = map[string]func() any{\n" + strings.Join(fs, "") + "}\n\n")
	}
}

// appSource is app__alder.go: Tables, the descriptors of every table in module order then
// declaration order (db.Ensure's schema hash depends on it); Capabilities, the Env.Caps key of
// every capability; Caps and its Registry; in a test build only, Tests and TestSession
// (testsTable) and FakeTables and Fakes (fakesTable); and for an app with an App module Options,
// Serve and the session codec. Its header names the compiler's version (Options.Version) when
// given.
func (g *gen) appSource(p *ir.Program, migrations bool, version string) string {
	var ds, cs []string
	for _, tb := range p.Tables {
		ds = append(ds, tableName(tb.Module, tb.Name)+".Desc()")
	}
	for _, c := range p.Caps {
		cs = append(cs, strconv.Quote(c.Module+"."+c.Name))
	}
	var b strings.Builder
	b.WriteString("// Tables describes every table of the app, in module order then declaration order, for db.Ensure.\n" +
		"var " + tablesVar + " = []db.TableDesc{" + strings.Join(ds, ", ") + "}\n\n" +
		"// Capabilities are the keys of eff.Env.Caps the app's capability calls look up: Caps.Registry\n" +
		"// fills them.\n" +
		"var " + capsVar + " = []string{" + strings.Join(cs, ", ") + "}\n\n")
	if migrations {
		b.WriteString("// migrationsFS holds the app's migration files, migrations/*.sql, embedded when the app is built.\n" +
			"//\n" +
			"//go:embed migrations/*.sql\n" +
			"var migrationsFS embed.FS\n\n" +
			"// " + migrationsVar + " are the app's migrations in order (db.ReadMigrations): Serve applies those the\n" +
			"// database has not had (db.Migrate).\n" +
			"var " + migrationsVar + " = db.ReadMigrations(migrationsFS)\n\n")
	} else {
		b.WriteString("// " + migrationsVar + " are the app's migrations: none (the app has no migrations/ directory), so\n" +
			"// Serve creates the tables with db.Ensure.\n" +
			"var " + migrationsVar + " []db.Migration\n\n")
	}

	if len(p.Silos) > 0 {
		g.silosVar(&b, p)
	}

	// Caps
	b.WriteString("// Caps are the app's capability implementations, one field per capability, named by its\n" +
		"// module and name: the generated caps package's Caps function builds them from the adapters,\n" +
		"// and Serve refuses a field left nil.\n")
	if len(p.Caps) == 0 {
		b.WriteString("type Caps struct{}\n\n")
	} else {
		b.WriteString("type Caps struct {\n")
		for _, c := range p.Caps {
			fmt.Fprintf(&b, "%s %s\n", GoName(c.Module, c.Name), capName(c.Module, c.Name))
		}
		b.WriteString("}\n\n")
	}
	b.WriteString("// Registry is c as eff.Env.Caps: each implementation by its key in Capabilities (nil when the\n" +
		"// field is). It is exported for alder test's main; Serve uses it too.\n")
	if len(p.Caps) == 0 {
		b.WriteString("func (Caps) Registry() map[string]any {\nreturn map[string]any{}\n}\n\n")
	} else {
		b.WriteString("func (c Caps) Registry() map[string]any {\nreturn map[string]any{\n")
		for _, c := range p.Caps {
			fmt.Fprintf(&b, "%s: c.%s,\n", strconv.Quote(c.Module+"."+c.Name), GoName(c.Module, c.Name))
		}
		b.WriteString("}\n}\n\n")
	}
	if g.tests {
		g.testsTable(&b, p)
		fakesTable(&b, p)
		g.mutantsTable(&b, p) // a mutate build only
	}
	unionsInit(&b, p)
	if p.Session != nil {
		var sb strings.Builder
		g.serve(&sb, p)
		b.WriteString(g.serveSilos(p, sb.String()))
	} else if g.tests {
		b.WriteString("// Routes are the app's routes: none, as the app has no App module (alder test's browser serves them).\n" +
			"var Routes = []web.Route{}\n\n" +
			"// Web is the app's web.Options for alder test's browser: none, as the app has no App module.\n" +
			"func Web() web.Options {\nreturn web.Options{}\n}\n\n")
	}
	if g.tests {
		g.selectorVars(&b)
	}
	src := b.String()
	imports, err := importsOf(src)
	if err != nil {
		panic(fmt.Sprintf("gen: %s: %v\n%s", appFile, err, src))
	}
	if !slices.Contains(imports, "alder/rt") {
		// rt's init (the scratch programs' stdin tether, rt/tether.go) must run before this
		// package's variables are computed: the Go spec initializes a package's imports first
		imports = append(imports, rtBlank)
	}
	by := "alder"
	if version != "" {
		by += " " + version
	}
	return "// Code generated by " + by + "; DO NOT EDIT.\n\npackage app\n\n" + importBlock(imports) + src
}

// testsTable writes, for a test build, Tests, every `test` of the app as an rtesting.Case (module order, then
// declaration order), and TestSession, the session a test runs with: App's session with every
// field at its default, nil without an App module.
func (g *gen) testsTable(b *strings.Builder, p *ir.Program) {
	var cases []string
	for _, m := range p.Modules {
		for i, t := range m.Tests {
			kind := "Pure"
			if t.Effect {
				kind = "Task"
			}
			cases = append(cases, fmt.Sprintf("{Module: %s, Name: %s, %s: %s}", strconv.Quote(t.Module), strconv.Quote(t.Name), kind, testFunc(t.Module, i+1)))
		}
	}
	list := "{}"
	if len(cases) > 0 {
		list = "{\n" + strings.Join(cases, ",\n") + ",\n}"
	}
	b.WriteString("// Tests are the app's `test` declarations in module order then declaration order, for alder test (rtesting.Main).\n" +
		"var Tests = []rtesting.Case" + list + "\n\n")
	session := "nil"
	if p.Session != nil {
		session = "session__codec{}.Default()"
	}
	b.WriteString("// TestSession is the session a test runs with: App's session with every field at its default (nil without an App module).\n" +
		"func TestSession() any {\nreturn " + session + "\n}\n\n")
}

// unionsInit writes the init registering every union of the program (module order, then
// declaration order; page and App Msg unions included) with rt.RegisterUnion: its tag type and
// its constructor names in tag order, so rt.Show prints its values.
func unionsInit(b *strings.Builder, p *ir.Program) {
	var regs []string
	for _, m := range p.Modules {
		for _, d := range m.Types {
			if d.Union == nil {
				continue
			}
			names := make([]string, len(d.Union.Ctors))
			for i, c := range d.Union.Ctors {
				names[i] = strconv.Quote(c.Name)
			}
			regs = append(regs, fmt.Sprintf("rt.RegisterUnion(reflect.TypeOf(%s(0)), []string{%s})\n",
				tagType(d.Union.Module, d.Union.Name), strings.Join(names, ", ")))
		}
	}
	if len(regs) == 0 {
		return
	}
	b.WriteString("// init registers the constructor names of every union of the app, for rt.Show.\nfunc init() {\n")
	for _, r := range regs {
		b.WriteString(r)
	}
	b.WriteString("}\n\n")
}

// importBlock is the import declaration of a generated file.
func importBlock(imports []string) string {
	switch len(imports) {
	case 0:
		return ""
	case 1:
		return "import " + importSpec(imports[0]) + "\n\n"
	}
	var b strings.Builder
	b.WriteString("import (\n")
	for _, imp := range imports {
		b.WriteString(importSpec(imp) + "\n")
	}
	b.WriteString(")\n\n")
	return b.String()
}

// rtBlank is the blank import of alder/rt that app__alder.go has when it does not use rt: every
// app depends on rt, so rt's init runs before the app's package variables are computed.
const rtBlank = `_ "alder/rt" // initialized before this package (rt/tether.go)`

// importSpec is the import of path: quoted, after its name when that is not the path's last
// element (importNames); rtBlank as it is.
func importSpec(path string) string {
	if path == rtBlank {
		return path
	}
	if n := importNames[path]; n != "" {
		return n + " " + strconv.Quote(path)
	}
	return strconv.Quote(path)
}

// serve writes Options, Serve and the session codec of an app with an App module.
func (g *gen) serve(b *strings.Builder, p *ir.Program) {
	login := p.Login
	if login == "" { // no `login`: the checker accepts that only when no page `requires`
		login = "/"
	}
	notFound := "func() html.Node { return App__notFound }"
	if v := g.values["App.notFound"]; v != nil && g.thunks[v] {
		notFound = "App__notFound"
	}
	var adapters []string
	for _, c := range p.Caps {
		adapters = append(adapters, fmt.Sprintf("%q: %q,\n", c.Module+"."+c.Name, "adapters/"+AdapterPkg(c.Module)+"/: func "+AdapterFunc(c.Module, c.Name)))
	}
	b.WriteString("// capAdapters name the adapter constructor of each capability, for Serve's refusal of one that\n" +
		"// returned nil.\nvar capAdapters = map[string]string{\n" + strings.Join(adapters, "") + "}\n\n")
	b.WriteString(`// Options configure Serve. The generated cmd/main.go fills them from the environment.
type Options struct {
	DB        string                  // the SQLite database file; ":memory:" or "" for a database in memory
	Dev       bool                    // development mode: a changed schema recreates the tables; error pages show the error
	Secret    []byte                  // the session cookie key: at least 16 bytes (in dev mode, shorter means the public development key)
	StaticDir string                  // the directory served under /static/; "" for none
	Reload    string                  // under alder run, the build generation; non-empty: pages subscribe to the dev server's reloads and error overlay
	Log       func(level, msg string) // nil: no log
	Build     string                  // the build's id (ALDER_BUILD, set by alder deploy), answered by /_alder/health
}

// Serve is the app's HTTP handler. It checks that o.Secret is at least 16 bytes outside dev
// mode (web.CheckSecret) and that caps implements every capability (an error names the
// adapter of each field left nil: "adapters/mail/: func Mailer returned nil"), opens the
// database o.DB and brings it to the app's tables (db.MigrateLog with the app's Migrations,
// logging each one as it commits, an adoption and a dev recreate; db.Ensure for an app
// without), and routes Routes (web.NewRouter) with Web's options: the session codec,
// App.error, App.notFound and the page App.login names.
//
// A database on Turso (o.DB libsql://…, outside dev mode) is not migrated: several instances
// start at once, and alder deploy migrates it before the build starts. Serve compares it with
// this build instead (db.CheckShared, which writes nothing): behind (or without tables) is the
// error "the database is behind this build: run ` + "`alder deploy`" + `"; ahead (a newer build migrated
// it) serves the maintenance page on every route, and /_alder/health answers 503 "ahead:
// <detail>" (web.Options.SharedMaintenance); equal serves.
func Serve(caps Caps, o Options) (http.Handler, error) {
	if err := web.CheckSecret(o.Secret, o.Dev); err != nil {
		return nil, err
	}
	reg := caps.Registry()
	var missing []string
	for _, k := range Capabilities {
		if reg[k] == nil {
			missing = append(missing, "capability "+k+" is not wired: "+capAdapters[k]+" returned nil")
		}
	}
	if len(missing) > 0 {
		return nil, errors.New(strings.Join(missing, "; "))
	}
	path := o.DB
	if path == "" {
		path = ":memory:"
	}
	store, err := db.OpenSQLite(path)
	if err != nil {
		return nil, err
	}
	var shared error // the shared database is ahead of this build: maintenance
	if db.IsRemote(path) && !o.Dev {
		state, detail, err := db.CheckShared(context.Background(), store, Tables, Migrations)
		if err != nil {
			store.Close()
			return nil, err
		}
		switch state {
		case db.SharedBehind, db.SharedFresh:
			if o.Log != nil {
				o.Log("error", "the database: "+detail)
			}
			store.Close()
			return nil, errors.New("the database is behind this build: run ` + "`alder deploy`" + `")
		case db.SharedAhead:
			shared = errors.New("ahead: " + detail)
			if o.Log != nil {
				o.Log("warn", "the database is ahead of this build, every page answers the maintenance page: "+detail)
			}
		}
	} else if len(Migrations) > 0 {
		logf := func(msg string) {
			if o.Log != nil {
				o.Log("info", msg)
			}
		}
		if _, err := store.MigrateLog(context.Background(), Tables, Migrations, o.Dev, logf); err != nil {
			store.Close()
			return nil, err
		}
	} else if err := store.Ensure(context.Background(), Tables, o.Dev); err != nil {
		store.Close()
		return nil, err
	}
SILOSTORES	w := Web()
	w.Store, w.Caps, w.Log, w.Dev, w.Secret, w.StaticDir, w.Reload = store, reg, o.Log, o.Dev, o.Secret, o.StaticDir, o.Reload
	w.Build, w.SharedMaintenance = o.Build, shared
SILOWEB
	for _, m := range Migrations {
		w.Migrations = append(w.Migrations, m.Name)
	}
	return web.NewRouter(Routes, w), nil
}

// Web is the app's options for web.NewRouter that do not depend on how it runs: the session
// codec, App.error, App.notFound and the page App.login names. Serve adds the store, the
// capabilities, the log, dev mode, the secret, the static directory and the reload generation;
// alder test's browser routes the app with these options too.
func Web() web.Options {
	return web.Options{
		Session:     session__codec{},
		ErrorDoc:    App__error,
		NotFoundDoc: ` + notFound + `,
		Login:       ` + strconv.Quote(login) + `,
		DevMap:      ` + strconv.Quote(devMapJSON(p)) + `,
WEBEXTRA	}
}

` + g.sessionCodec(p.Session) + g.sessionVia(p))
}

// silosVar writes Silos, the app's `silo` declarations (keys sorted, tables as written).
func (g *gen) silosVar(b *strings.Builder, p *ir.Program) {
	var items []string
	for _, k := range slices.Sorted(maps.Keys(p.Silos)) {
		var ts []string
		ref := ""
		for _, tb := range p.Silos[k] {
			ts = append(ts, strconv.Quote(tb.Module+"."+tb.Name))
			if tb.Partition != nil && ref == "" {
				ref = tb.Refs[tb.Partition.Column]
			}
		}
		items = append(items, "{Key: "+strconv.Quote(k)+", Tables: []string{"+strings.Join(ts, ", ")+"}, Ref: "+strconv.Quote(ref)+"}")
	}
	b.WriteString("// Silos are the app's `silo` declarations: each key's tables live in one database per value of\n" +
		"// the session's key (rt/db: a file under silos/<key>/ next to the shared database, or a\n" +
		"// database of its own on Turso); Ref is the table the key's values are ids of.\n" +
		"var Silos = []db.Silo{" + strings.Join(items, ", ") + "}\n\n")
}

// serveSilos fills serve's placeholders: for an app with silos, Serve opens its stores
// (db.OpenStores, the silos next to o.DB) and Web names them; App.maintenance, when declared,
// and the membership check of a session with `via` (session__via) go into Web. Without any,
// the source is the M11 one.
func (g *gen) serveSilos(p *ir.Program, src string) string {
	stores, web, extra := "", "", ""
	if len(p.Silos) > 0 {
		stores = `	var stores *db.Stores
	if len(Silos) > 0 {
		dir := ""
		if path != ":memory:" {
			dir = db.SiloDir(path)
		}
		stores, err = db.OpenStores(store, path, db.StoresConfig{Dir: dir, Shared: path, Silos: Silos, Tables: Tables, Migrations: Migrations, Dev: o.Dev, Log: o.Log})
		if err != nil {
			store.Close()
			return nil, err
		}
	}
`
		web = "\tw.Stores = stores\n"
		extra += "\t\tSilos: Silos,\n"
	}
	if v := g.values["App.maintenance"]; v != nil {
		doc := "func() html.Node { return App__maintenance }"
		if g.thunks[v] {
			doc = "App__maintenance"
		}
		extra += "\t\tMaintenanceDoc: " + doc + ",\n"
	}
	if len(p.Via) > 0 {
		extra += "\t\tSessionCheck: session__via,\n"
	}
	src = strings.Replace(src, "SILOSTORES", stores, 1)
	src = strings.Replace(src, "SILOWEB\n", web, 1)
	return strings.Replace(src, "WEBEXTRA", extra, 1)
}

// sessionVia is, for a session with `via` fields, session__via: Session.put's (and
// Test.login's) check (web.Options.SessionCheck → eff.Env.CheckSession) that a `via` field set
// to Just v — changed, or with another user — names a row of its membership table for the
// session's user, read inside the running Task's transaction on the shared store (db.Member);
// else the step fails "Session.put org: no Membership for this user and org" (an eff.ErrFail
// marked NotMember: a message's route answers it with Reject or 404). Nothing is always
// allowed, and a session without a user has every `via` field cleared (logging out clears the
// org: it returns the session to store). In a mutate build each field's `membership dropped`
// mutant skips its check.
func (g *gen) sessionVia(p *ir.Program) string {
	if len(p.Via) == 0 {
		return ""
	}
	app := GoName(p.Session.Module, p.Session.Name)
	var b strings.Builder
	b.WriteString(`
// session__via is Session.put's membership check (web.Options.SessionCheck): a ` + "`via`" + ` field of
// the session set to Just v, for a user, needs a row (user, v) of its membership table.
func session__via(e *eff.Env, o, n any) (any, error) {
	a, b := o.(` + app + `), n.(` + app + `)
`)
	for _, k := range slices.Sorted(maps.Keys(p.Via)) { // M1: no user, no membership: logging out clears the key
		for _, fd := range p.Session.Fields {
			if fd.Name == k {
				fmt.Fprintf(&b, "\tif !b.%s.Ok {\n\t\tb.%s = %s{}\n\t}\n", Field("user"), Field(k), GoType(fd.Type))
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(p.Via)) {
		mt := p.Via[k]
		ucol, kcol := g.memberCols(p, mt, k)
		fk, fu := Field(k), Field("user")
		msg := strconv.Quote("Session.put " + k + ": no " + mt.Name + " for this user and " + k)
		check := fmt.Sprintf(`ok, err := db.Member(e.Ctx, e.Store.(db.Store), db.TxOf(e), %[4]s, %[5]s, b.%[2]s.Val, %[6]s, b.%[1]s.Val)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &eff.ErrFail{Msg: %[3]s, NotMember: true}
		}
`, fk, fu, msg, tableName(mt.Module, mt.Name), strconv.Quote(ucol), strconv.Quote(kcol))
		if id := g.membershipDropped(k); id != 0 {
			check = fmt.Sprintf("if Mutant != %d {\n%s}\n", id, check)
		}
		fmt.Fprintf(&b, "\tif b.%[1]s.Ok && (!a.%[1]s.Ok || a.%[1]s.Val != b.%[1]s.Val || a.%[2]s != b.%[2]s) {\n%[3]s\t}\n", fk, fu, check)
	}
	b.WriteString("\treturn b, nil\n}\n")
	return b.String()
}

// memberCols are membership table mt's column referring to the user's table and the one
// referring to the table of session field k's id (the checker made sure there is one of each).
func (g *gen) memberCols(p *ir.Program, mt *ir.Table, k string) (ucol, kcol string) {
	ut, kt := g.tableOfSessionKey(p, "user"), g.tableOfSessionKey(p, k)
	for _, fd := range mt.Record.Fields {
		switch ref := mt.Refs[fd.Name]; {
		case ref != "" && ref == ut:
			ucol = fd.Name
		case ref != "" && ref == kt:
			kcol = fd.Name
		}
	}
	if ucol == "" || kcol == "" {
		panic(fmt.Sprintf("gen: membership table %s has no @ref to the tables of session fields user and %s", mt.Name, k))
	}
	return ucol, kcol
}

// tableOfSessionKey is the table ("Module.Name", as ir.Table.Refs names it) whose @id brand is
// session field k's (the brand of its Just).
func (g *gen) tableOfSessionKey(p *ir.Program, k string) string {
	for _, fd := range p.Session.Fields {
		if fd.Name == k && fd.Brand != nil {
			return g.tableOfBrand(p, fd.Brand)
		}
	}
	return ""
}

// tableOfBrand is the table ("Module.Name") whose @id field has brand b, "" for none.
func (g *gen) tableOfBrand(p *ir.Program, b *ir.Brand) string {
	for _, tb := range p.Tables {
		for _, fd := range tb.Record.Fields {
			if fd.Name == tb.IDField && fd.Brand != nil && fd.Brand.Module == b.Module && fd.Brand.Name == b.Name {
				return tb.Module + "." + tb.Name
			}
		}
	}
	return ""
}

// sessionDefault is App's Session record with every field at its default (a field without one
// at its Go zero value: Nothing for a Maybe).
func (g *gen) sessionDefault(r *ir.RecordType) string {
	f := &fn{g: g}
	var fs []string
	for _, fd := range r.Fields {
		if fd.Default != nil {
			fs = append(fs, Field(fd.Name)+": "+f.code(fd.Default, newScope()))
		}
	}
	return GoName(r.Module, r.Name) + "{" + strings.Join(fs, ", ") + "}"
}

// Main is the source of the app's command, cmd/main.go of the generated module: it serves the
// app (Serve) with the capabilities the generated caps package wires (caps.Caps) when withCaps
// is set (the app has capabilities), else with none. name is the app's name, the default database file's base name.
// Configuration comes from the environment: ALDER_ADDR (default ":8080"), ALDER_DB (default
// "<name>.db", in the working directory), ALDER_SECRET, ALDER_DEV=1 and ALDER_STATIC; static
// files are served from ALDER_STATIC, by default the static directory next to the executable
// (alder build copies the app's static/ there). Before serving it prints "listening on
// <address>" with the bound address, so ALDER_ADDR=127.0.0.1:0 picks a free port that a caller
// can read. Its http.Server waits at most 10 s for a request's headers, and on SIGINT or SIGTERM
// it stops accepting connections and lets the requests in flight finish (5 s at most).
// ALDER_EXIT_ON_STDIN_EOF=1 (set by alder run, which holds the other end of the app's standard
// input) makes the same shutdown happen when standard input reaches EOF, so the app never
// outlives the dev server; the variable is cleared once read, so a process the app starts does
// not inherit it. Without it standard input is not read.
//
// withSilos (an app with `silo` declarations) adds maintenance tasks over the silo databases
// (rt/db: silos/<key>/<value>.db under ALDER_DB's directory, or with ALDER_DB=libsql://… the
// databases of their own on Turso): --migrate-silos [--allow-data-loss] opens and migrates each
// one (a line each; exit 1 when one is not served: `alder migrate silos` runs it), --silo-status prints one JSON line per silo ({"key","id","state","detail"}:
// ok, migrating, pending-data-loss, ahead, failed or orphaned) without changing any (`alder
// status` runs it), --verify-silos N tries the pending migrations on the N largest silos of each
// key and rolls them back (alder deploy runs it on Turso before migrating), and --prune-silos
// key:id… removes the databases of those silos that are orphaned (`alder silos prune`).
func Main(name string, withCaps, withSilos bool) []byte {
	imports := "\t\"app\"\n"
	caps := "app.Caps{}"
	if withCaps {
		imports += "\t\"app/caps\"\n"
		caps = "caps.Caps()"
	}
	src := `// Code generated by alder; DO NOT EDIT.

// Command ` + name + ` serves the app. ALDER_ADDR is the address to listen on (default :8080),
// ALDER_DB the SQLite database file (default ` + name + `.db), ALDER_SECRET the session cookie key
// (at least 16 bytes; required outside development mode, where without it a public development
// key is used) and ALDER_DEV=1 turns on development mode; ALDER_RELOAD (set by alder run to its
// build generation) makes pages subscribe to the dev server's reloads. Static files are served under /static/
// from ALDER_STATIC, by default the static directory next to the executable. SIGINT or SIGTERM
// shuts the server down gracefully: requests in flight get 5 s to finish. ALDER_EXIT_ON_STDIN_EOF=1
// (set by alder run) makes the app stop the same way when its standard input reaches EOF, so it
// never outlives the dev server. GET /_alder/health answers {"build":"<ALDER_BUILD>","db":"ok"}
// (200) when the database answers, else 503 "unreachable" (the error is logged). With
// ALDER_DB=libsql://… (a database on Turso) the app does not migrate on start: it exits 1 when
// the database is behind this build (alder deploy migrates it) and serves the maintenance page
// when a newer build migrated it.
//
// Two arguments run a maintenance task instead of serving (alder deploy runs them at the
// target, chapter 16): --migrate-only applies the pending migrations to ALDER_DB (each
// "migrated <name>" printed) and exits, 0 or 1 with the refusal, before the new build is
// started; --backup-to FILE writes a consistent copy of ALDER_DB to FILE (VACUUM INTO) while
// another process may be serving it. ALDER_BACKUP_URL (s3://bucket/prefix, or a local
// directory) streams the database there while it serves (rt/backup: a snapshot, then WAL
// segments every ALDER_BACKUP_INTERVAL, default 10s; once more at shutdown).
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"alder/rt/backup"
	"alder/rt/db"
	"alder/rt/web"
` + imports + `)

func main() {
	dev := os.Getenv("ALDER_DEV") == "1"
	dbPath := env("ALDER_DB", ` + strconv.Quote(name+".db") + `)
	if len(os.Args) > 1 {
		os.Exit(maintain(os.Args[1:], dbPath, dev))
	}
	static := os.Getenv("ALDER_STATIC")
	if static == "" {
		static = "static"
		if exe, err := os.Executable(); err == nil {
			static = filepath.Join(filepath.Dir(exe), "static")
		}
	}
	h, err := app.Serve(` + caps + `, app.Options{
		DB:        dbPath,
		Dev:       dev,
		Secret:    []byte(os.Getenv("ALDER_SECRET")),
		StaticDir: static,
		Reload:    os.Getenv("ALDER_RELOAD"),
		Build:     os.Getenv("ALDER_BUILD"),
		Log: func(level, msg string) {
			if level != "effect" || dev { // every effect step is logged in development mode only
				log.Printf("%s: %s", level, msg)
			}
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "alder:", err)
		os.Exit(1)
	}
	stopBackup := func() {}
	if loc := os.Getenv(backup.EnvURL); loc != "" {
		stopBackup, err = backup.Start(dbPath, loc, backup.IntervalFromEnv(), func(level, msg string) { log.Printf("%s: %s", level, msg) })
		if err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			os.Exit(1)
		}
	}
	ln, err := net.Listen("tcp", env("ALDER_ADDR", ":8080"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "alder:", err)
		os.Exit(1)
	}
	fmt.Println("listening on " + ln.Addr().String())
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if os.Getenv("ALDER_EXIT_ON_STDIN_EOF") == "1" {
		os.Unsetenv("ALDER_EXIT_ON_STDIN_EOF") // not inherited by a process the app starts
		// alder run holds the other end of stdin: when it dies, this read ends and the app stops
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			stop()
		}()
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		fmt.Fprintln(os.Stderr, "alder:", err)
		os.Exit(1)
	case <-ctx.Done():
	}
	stop()
DRAIN	grace, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(grace); err != nil {
		fmt.Fprintln(os.Stderr, "alder: shutting down:", err)
	}
	stopBackup() // ships what the last requests wrote
}

// maintain runs the maintenance task args name (see the package comment) on the database at
// path and returns the exit code.
func maintain(args []string, path string, dev bool) int {
	ctx := context.Background()
	switch {
	case len(args) == 1 && args[0] == "--migrate-only":
		if err := web.CheckSecret([]byte(os.Getenv("ALDER_SECRET")), dev); err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			return 1
		}
		store, err := db.OpenSQLite(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			return 1
		}
		defer store.Close()
		if len(app.Migrations) > 0 {
			_, err = store.MigrateLog(ctx, app.Tables, app.Migrations, dev, func(msg string) { fmt.Println(msg) })
		} else {
			err = store.Ensure(ctx, app.Tables, dev)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			return 1
		}
		return 0
	case len(args) == 2 && args[0] == "--backup-to":
		if err := backup.Snapshot(ctx, path, args[1]); err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			return 1
		}
		return 0
	}
SILOCASES	fmt.Fprintln(os.Stderr, "usage: "+filepath.Base(os.Args[0])+" [--migrate-only | --backup-to FILE"+SILOUSAGE+"]  (no argument: serve)")
	return 2
}

// env is the environment variable name, or def when it is unset or empty.
func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
`
	cases, usage := "", ""
	if withSilos {
		cases = siloCases
		usage = `+" | --migrate-silos [--allow-data-loss] | --silo-status | --verify-silos N | --prune-silos key:id…"`
		src = strings.Replace(src, `	"path/filepath"
`, `	"path/filepath"
	"strconv"
`, 1) + siloFlagsFunc
	}
	drain := ""
	if withSilos { // M3: the old process opens no silo while it drains, and re-checks before a write
		drain = "\tdb.Drain()\n"
	}
	src = strings.Replace(src, "DRAIN", drain, 1)
	src = strings.Replace(strings.Replace(src, "SILOCASES", cases, 1), "+SILOUSAGE", usage, 1)
	out, err := format.Source([]byte(src))
	if err != nil {
		panic(fmt.Sprintf("gen: cmd/main.go: %v\n%s", err, src))
	}
	return out
}

// TzdataDeploy is cmd/tzdata_deploy.go of a built app: under the alder_deploy build tag (the
// deploy's cross-build) it imports time/tzdata, so the binary carries the time zone database
// that a minimal image lacks. `alder build` and `alder run` leave it out (the tag is off), and
// the machine's own zoneinfo serves.
func TzdataDeploy() []byte {
	return []byte(`//go:build alder_deploy

// Code generated by alder; DO NOT EDIT.

// A deploy's build (alder deploy: go build -tags alder_deploy) embeds the time zone database:
// a minimal image has no /usr/share/zoneinfo.

package main

import _ "time/tzdata"
`)
}

// siloCases are the maintenance tasks of an app with silos (Main): the silo databases are
// those under ALDER_DB's directory.
const siloCases = `	silos := db.StoresConfig{Dir: db.SiloDir(path), Shared: path, Silos: app.Silos, Tables: app.Tables, Migrations: app.Migrations, Dev: dev}
	switch {
	case len(args) >= 1 && args[0] == "--migrate-silos" && siloFlags(args[1:], &silos):
		bad, err := db.MigrateSilos(ctx, silos, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			return 1
		}
		if bad > 0 {
			fmt.Fprintf(os.Stderr, "alder: %d silo(s) not migrated\n", bad)
			return 1
		}
		return 0
	case len(args) == 1 && args[0] == "--silo-status":
		if err := db.StatusSilos(ctx, silos, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			return 1
		}
		return 0
	case len(args) == 2 && args[0] == "--verify-silos":
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 {
			break
		}
		bad, err := db.VerifySilos(ctx, silos, n, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			return 1
		}
		if bad > 0 {
			fmt.Fprintf(os.Stderr, "alder: the migrations fail on %d sampled silo(s)\n", bad)
			return 1
		}
		return 0
	case len(args) >= 2 && args[0] == "--prune-silos":
		kept, err := db.PruneSilos(ctx, silos, args[1:], os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "alder:", err)
			return 1
		}
		if kept > 0 {
			fmt.Fprintf(os.Stderr, "alder: %d silo(s) kept\n", kept)
			return 1
		}
		return 0
	}
`

// siloFlags is the source of siloFlags in an app with silos' cmd/main.go: --migrate-silos's
// flags (--allow-data-loss), false for any other.
const siloFlagsFunc = `
// siloFlags reads --migrate-silos's flags into c: false for one it does not know.
func siloFlags(args []string, c *db.StoresConfig) bool {
	for _, a := range args {
		switch a {
		case "--allow-data-loss":
			c.AllowDataLoss = true
		default:
			return false
		}
	}
	return true
}
`

// sessionCodec is the session cookie's codec: App's session as a JSON object of its fields
// (by Go field name), a payload-less union stored by constructor name (a Maybe of one: the name
// or null), so reordering a union's constructors never changes what a cookie means. Decode
// starts from Default: a field the cookie lacks, or whose value no longer decodes (a
// constructor since removed), has its default.
func (g *gen) sessionCodec(r *ir.RecordType) string {
	sess := GoName(r.Module, r.Name)
	var enc, dec []string
	enums := false
	for _, fd := range r.Fields {
		name, key := Field(fd.Name), strconv.Quote(Field(fd.Name))
		t, maybe := fd.Type, false
		if a := maybeArg(t); a != nil {
			t, maybe = a, true
		}
		u := enumUnion(t)
		switch {
		case u == nil:
			enc = append(enc, key+": s."+name+",")
			dec = append(dec, fmt.Sprintf("if raw, ok := m[%s]; ok {\nvar v %s\nif json.Unmarshal(raw, &v) == nil {\ns.%s = v\n}\n}\n", key, GoType(fd.Type), name))
		case maybe:
			enums = true
			names, _ := enumLists(u)
			val := GoType(t) + "{Tag: " + tagType(u.Module, u.Name) + "(i)}"
			enc = append(enc, fmt.Sprintf("%s: session__name(s.%s.Ok, %s[s.%s.Val.Tag]),", key, name, names, name))
			dec = append(dec, fmt.Sprintf("if raw, ok := m[%s]; ok {\nif string(raw) == \"null\" {\ns.%s = rt.Nothing[%s]()\n} else if i := session__enum(raw, %s); i >= 0 {\ns.%s = rt.Just(%s)\n}\n}\n",
				key, name, GoType(t), names, name, val))
		default:
			enums = true
			names, _ := enumLists(u)
			enc = append(enc, fmt.Sprintf("%s: %s[s.%s.Tag],", key, names, name))
			dec = append(dec, fmt.Sprintf("if raw, ok := m[%s]; ok {\nif i := session__enum(raw, %s); i >= 0 {\ns.%s = %s{Tag: %s(i)}\n}\n}\n",
				key, names, name, GoType(t), tagType(u.Module, u.Name)))
		}
	}
	encode := "func (session__codec) Encode(any) ([]byte, error) { return []byte(\"{}\"), nil }\n"
	if len(r.Fields) > 0 {
		encode = "func (session__codec) Encode(v any) ([]byte, error) {\ns := v.(" + sess + ")\nreturn json.Marshal(map[string]any{\n" + strings.Join(enc, "\n") + "\n})\n}\n"
	}
	out := `// session__codec stores App's session in the signed cookie (web.SessionCodec): a JSON object
// of its fields, a payload-less union by constructor name (a Maybe of one: the name or null), so
// reordering a union's constructors does not change what a cookie means.
type session__codec struct{}

// Default is the session of a request without a valid cookie: every field at its default.
func (session__codec) Default() any {
	return ` + g.sessionDefault(r) + `
}

` + encode + `

// Decode starts from Default: a field the cookie lacks (one added since) or whose value does not
// decode (a constructor since removed) has its default.
func (c session__codec) Decode(b []byte) (any, error) {
	s := c.Default().(` + sess + `)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
` + strings.Join(dec, "") + `	return s, nil
}
`
	if enums {
		out += `
// session__enum is the index in names of the constructor name raw holds, -1 for none.
func session__enum(raw json.RawMessage, names []string) int {
	var n string
	if json.Unmarshal(raw, &n) != nil {
		return -1
	}
	for i, x := range names {
		if x == n {
			return i
		}
	}
	return -1
}

// session__name is a Maybe union field as stored: its constructor name, or null for Nothing.
func session__name(ok bool, name string) any {
	if !ok {
		return nil
	}
	return name
}
`
	}
	return out
}

// CapsFile is the generated caps package's file in the module (Emit's key).
const CapsFile = "caps/caps__alder.go"

// CapsPackage is the source of the generated package caps (CapsFile): its Caps returns the
// app's capabilities, each from its adapter's constructor, AdapterFunc(M, C) of package
// app/adapters/AdapterPkg(M) imported as adapterAlias (so an adapter package named like one
// the file imports, a module App's "app", is no clash); app.Caps{} when the app has none. The
// caller has checked that no two capabilities share a constructor (AdapterCollision). The
// capabilities in unwired (keys "M.C"; a test build's missing adapters, gen.Options.Unwired)
// are left nil, each with a comment, and a package none of the others needs is not imported.
func CapsPackage(p *ir.Program, unwired map[string]bool) []byte {
	var b strings.Builder
	b.WriteString(`// Code generated by alder; DO NOT EDIT.

// Package caps wires the app's capabilities to their adapters: capability C of module A.B is
// implemented by package app/adapters/a (the module's first segment, lower-cased), whose
// exported func BC returns it.
package caps

import (
	"app"
`)
	dirs := map[string]bool{}
	for _, c := range p.Caps {
		if !unwired[c.Module+"."+c.Name] {
			dirs[AdapterPkg(c.Module)] = true
		}
	}
	for _, d := range slices.Sorted(maps.Keys(dirs)) {
		fmt.Fprintf(&b, "%s %s\n", adapterAlias(d), strconv.Quote("app/adapters/"+d))
	}
	b.WriteString(")\n\n// Caps returns the app's capabilities, each from its adapter's constructor.\nfunc Caps() app.Caps {\n")
	if len(p.Caps) == 0 {
		b.WriteString("return app.Caps{}\n}\n")
	} else {
		b.WriteString("return app.Caps{\n")
		for _, c := range p.Caps {
			if unwired[c.Module+"."+c.Name] {
				fmt.Fprintf(&b, "// %s: no adapter (adapters/%s/ has no func %s): unwired in tests\n", GoName(c.Module, c.Name), AdapterPkg(c.Module), AdapterFunc(c.Module, c.Name))
				continue
			}
			fmt.Fprintf(&b, "%s: %s.%s(),\n", GoName(c.Module, c.Name), adapterAlias(AdapterPkg(c.Module)), AdapterFunc(c.Module, c.Name))
		}
		b.WriteString("}\n}\n")
	}
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		panic(fmt.Sprintf("gen: %s: %v\n%s", CapsFile, err, b.String()))
	}
	return src
}

// AdapterStub is the Go file `alder new capability` writes for capability c (internal/generate):
// package pkg (the package of adapters/AdapterPkg(c.Module)/) with the constructor the rule
// names, AdapterFunc(c.Module, c.Name), returning a value whose methods are the capability's
// entries, as capDecl declares them, each returning its result's zero value and the error "not
// implemented" until its author writes it (the step then fails with
// "<Capability>.<entry>: not implemented"). It is gofmt'ed.
func AdapterStub(c *ir.Capability, pkg string) ([]byte, error) {
	fn := AdapterFunc(c.Module, c.Name)
	impl := "adapter" + fn
	var b strings.Builder
	fmt.Fprintf(&b, "// Package %s holds the Go adapter of capability %s.%s, written by\n", pkg, c.Module, c.Name)
	b.WriteString("// `alder new capability`: each method returns an error until you write it (the Alder book,\n// chapter 7, \"Implementing a capability\").\n")
	fmt.Fprintf(&b, "package %s\n\nIMPORTS\n\n", pkg)
	fmt.Fprintf(&b, "// %s is the adapter of %s.%s.\nfunc %s() app.%s { return %s{} }\n\n", fn, c.Module, c.Name, fn, capName(c.Module, c.Name), impl)
	fmt.Fprintf(&b, "// %s implements %s.%s.\ntype %s struct{}\n", impl, c.Module, c.Name, impl)
	for _, e := range c.Entries {
		params := []string{"ctx context.Context"}
		res := e.Type
		if ft, ok := e.Type.(*types.Fun); ok {
			if p, _ := capArg(e.Type); p != nil {
				params = append(params, "args app."+argsName(c.Module, c.Name, e.Name))
			} else {
				for i, p := range ft.Params {
					params = append(params, "a"+strconv.Itoa(i+1)+" "+appQualified(GoType(p)))
				}
			}
			res = ft.Result
		}
		rt := appQualified(GoType(effResult(res)))
		fmt.Fprintf(&b, "\n// %s is %s.%s: not implemented yet.\nfunc (%s) %s(%s) (%s, error) {\n\tvar zero %s\n\treturn zero, errors.New(\"not implemented\")\n}\n",
			Field(e.Name), c.Name, e.Name, impl, Field(e.Name), strings.Join(params, ", "), rt, rt)
	}
	src := b.String()
	imports := []string{"context", "errors"}
	_, code, _ := strings.Cut(src, "IMPORTS")
	for _, m := range stubPkg.FindAllStringSubmatch(code, -1) {
		path := map[string]string{"rt": "alder/rt", "html": "alder/rt/html", "time": "time", "db": "alder/rt/db", "eff": "alder/rt/eff", "app": "app"}[m[1]]
		if !slices.Contains(imports, path) {
			imports = append(imports, path)
		}
	}
	slices.Sort(imports)
	var std, mod []string
	for _, im := range imports {
		if strings.Contains(im, "/") || im == "app" {
			mod = append(mod, strconv.Quote(im))
		} else {
			std = append(std, strconv.Quote(im))
		}
	}
	block := "import (\n" + strings.Join(std, "\n") + "\n\n" + strings.Join(mod, "\n") + "\n)"
	return format.Source([]byte(strings.Replace(src, "IMPORTS", block, 1)))
}

// appQualified qualifies the generated package's names in a Go type written inside it
// (GoType's), for code outside it: `Post__Status` → `app.Post__Status`.
func appQualified(t string) string {
	return appName.ReplaceAllString(t, "${1}app.$2")
}

var appName = regexp.MustCompile(`(^|[^.\w])([A-Z]\w*__\w+)`)

// stubPkg finds the packages a stub's code uses.
var stubPkg = regexp.MustCompile(`\b(rt|html|time|db|eff|app)\.[A-Z]`)
