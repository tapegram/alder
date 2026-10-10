package gen

import (
	"fmt"
	"go/format"
	"slices"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// Capabilities and the app file.
//
// A capability is implemented in Go, outside the language, by the app's adapters package
// (adapters.Caps returns them). Its surface is exported, in the module's file:
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
// app__alder.go holds Tables and Capabilities and, for an app with an App module, Caps (one
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
	w.WriteString(comment(fmt.Sprintf("%s is capability %s of module %s, implemented by the app's adapters package (adapters.Caps): one method per entry, called with the running request's context (db.TxFrom(ctx) is the running Task's transaction).", name, c.Name, c.Module)))
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

// appSource is app__alder.go: Tables, the descriptors of every table in module order then
// declaration order (db.Ensure's schema hash depends on it); Capabilities, the Env.Caps key of
// every capability; Caps and its Registry; Tests and TestSession (testsTable); and for an app
// with an App module Options, Serve and the session codec.
func (g *gen) appSource(p *ir.Program) string {
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

	// Caps
	b.WriteString("// Caps are the app's capability implementations, one field per capability, named by its\n" +
		"// module and name: the adapters package's Caps function returns them, and Serve refuses a\n" +
		"// field left nil.\n")
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
	g.testsTable(&b, p)
	unionsInit(&b, p)
	if p.Session != nil {
		g.serve(&b, p)
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
	return "// Code generated by alder; DO NOT EDIT.\n\npackage app\n\n" + importBlock(imports) + src
}

// testsTable writes Tests, every `test` of the app as an rtesting.Case (module order, then
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
	b.WriteString(`// Options configure Serve. The generated cmd/main.go fills them from the environment.
type Options struct {
	DB        string                  // the SQLite database file; ":memory:" or "" for a database in memory
	Dev       bool                    // development mode: a changed schema recreates the tables; error pages show the error
	Secret    []byte                  // the session cookie key: at least 16 bytes (in dev mode, shorter means the public development key)
	StaticDir string                  // the directory served under /static/; "" for none
	Reload    string                  // under alder run, the build generation; non-empty: pages subscribe to the dev server's reloads and error overlay
	Log       func(level, msg string) // nil: no log
}

// Serve is the app's HTTP handler. It checks that o.Secret is at least 16 bytes outside dev
// mode (web.CheckSecret) and that caps implements every capability (an error names each field
// left nil), opens the database o.DB and creates its tables (db.Ensure), and routes Routes
// (web.NewRouter) with the session codec, App.error, App.notFound and the page App.login names.
func Serve(caps Caps, o Options) (http.Handler, error) {
	if err := web.CheckSecret(o.Secret, o.Dev); err != nil {
		return nil, err
	}
	reg := caps.Registry()
	var missing []string
	for _, k := range Capabilities {
		if reg[k] == nil {
			missing = append(missing, "capability "+k+" is not wired: set Caps."+strings.ReplaceAll(k, ".", "__"))
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
	if err := store.Ensure(context.Background(), Tables, o.Dev); err != nil {
		store.Close()
		return nil, err
	}
	return web.NewRouter(Routes, web.Options{
		Store:       store,
		Caps:        reg,
		Log:         o.Log,
		Dev:         o.Dev,
		Secret:      o.Secret,
		Session:     session__codec{},
		StaticDir:   o.StaticDir,
		Reload:      o.Reload,
		ErrorDoc:    App__error,
		NotFoundDoc: ` + notFound + `,
		Login:       ` + strconv.Quote(login) + `,
	}), nil
}

` + g.sessionCodec(p.Session))
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
// app (Serve) with the capabilities of the adapters package (adapters.Caps) when withAdapters
// is set, else with none. name is the app's name, the default database file's base name.
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
func Main(name string, withAdapters bool) []byte {
	imports := "\t\"app\"\n"
	caps := "app.Caps{}"
	if withAdapters {
		imports += "\t\"app/adapters\"\n"
		caps = "adapters.Caps()"
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
// never outlives the dev server.
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

` + imports + `)

func main() {
	dev := os.Getenv("ALDER_DEV") == "1"
	static := os.Getenv("ALDER_STATIC")
	if static == "" {
		static = "static"
		if exe, err := os.Executable(); err == nil {
			static = filepath.Join(filepath.Dir(exe), "static")
		}
	}
	h, err := app.Serve(` + caps + `, app.Options{
		DB:        env("ALDER_DB", ` + strconv.Quote(name+".db") + `),
		Dev:       dev,
		Secret:    []byte(os.Getenv("ALDER_SECRET")),
		StaticDir: static,
		Reload:    os.Getenv("ALDER_RELOAD"),
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
	grace, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(grace); err != nil {
		fmt.Fprintln(os.Stderr, "alder: shutting down:", err)
	}
}

// env is the environment variable name, or def when it is unset or empty.
func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
`
	out, err := format.Source([]byte(src))
	if err != nil {
		panic(fmt.Sprintf("gen: cmd/main.go: %v\n%s", err, src))
	}
	return out
}

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
