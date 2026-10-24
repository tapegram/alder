package generate

import (
	"fmt"
	"go/format"
	"strings"

	"alder/internal/gen"
	"alder/internal/syntax"
	"alder/internal/types"
)

// AuthOptions are the options of `alder new auth`.
type AuthOptions struct {
	Users string // --users T: the user table ("User" by default), created when it does not exist
	Role  string // --role R: the union of the user module the session's `role` has, "" for none
	To    string // --to P: the page a login goes to (default: the page at `/`; without one, the hole ?afterLogin)
}

// AuthData is the data of auth_login.tmpl and the adapter's templates.
type AuthData struct {
	Imports     []string
	Dir         string // the adapter's directory under adapters/: "auth"
	Package     string // its Go package
	Func        string // its constructor: "LoginPassword"
	Table       string // the user table's record type as written in Auth.Login: "User"
	TableName   string // its Go descriptor's name, quoted: `"User.User"`
	HashColumn  string // "passwordHash"
	HashColumnQ string // quoted
	IDType      string // "User.UserId"
	Role        bool
	RoleColumn  string
	RoleDefault string
	Home        string // the page value a login redirects to: "Home {}"
	Open, Close string
	Iterations  int
	Tests       []TestData
}

// hashColumn is the column a user table keeps its password hash in.
const hashColumn = "passwordHash"

// NewAuth generates `alder new auth`: the login page app/Auth/Login.alder (the capability
// Password { verify, hash }, its fake, `page "/login"`, the form Credentials, the Submit arm that
// puts the user (and the role) in the session or rejects the form, the view, two tests), the
// edits of App (the session's `user` and `role` fields, `login = Auth.Login`, `msg Logout` and
// its arm, a test of it), the user table when it does not exist (with its migration, the
// caller's Options.Migrate), and the Go adapter adapters/auth/login_password.go (PBKDF2-SHA256)
// with its tests. The login goes to --to's page, else the page at `/`, else (an app with no
// page yet: `alder new <app> --bare`, whose pages come after the login they require) the typed
// hole ?afterLogin, in Submit's arm and in its test, filled once the page exists. It refuses an app whose session has a `user`, that has an Auth module or a
// `login`, a user table without `email` and `passwordHash`, and a role that is not a union of
// the user module (`alder new type` makes one) or that the table has no column of.
func NewAuth(p *Program, o AuthOptions) ([]Edit, bool, error) {
	if o.Users == "" {
		o.Users = "User"
	}
	app := p.Modules["App"]
	if app == nil || p.Session == nil {
		return nil, false, Refusef("there is no module `App` with a `session`: `alder new <app>` writes one")
	}
	if p.Session.User != nil || hasField(p.Session, "user") {
		return nil, false, Refusef("the session already has a `user` field: the app has a login (`login = %s`)", orNone(p.Login))
	}
	for _, name := range p.Order {
		if name == "Auth" || strings.HasPrefix(name, "Auth.") {
			return nil, false, Refusef("module `%s` exists: `alder new auth` writes `Auth.Login`; the app has one, or remove `%s` first", name, p.Modules[name].File)
		}
	}
	if p.Login != "" {
		return nil, false, Refusef("`App` already has `login = %s`", p.Login)
	}
	if hasField(p.Session, "role") && o.Role != "" {
		return nil, false, Refusef("the session already has a `role` field: leave out --role, or remove it first")
	}
	if declared(app.Syntax, "Logout") {
		return nil, false, Refusef("`App` already declares `Logout`")
	}
	if !IsUpperIdent(o.Users) {
		return nil, false, Refusef("`--users %s`: a table is named by one capitalised identifier, such as `User`", o.Users)
	}

	// the page a login goes to
	var home *Page
	if o.To != "" {
		if home = p.PageNamed(o.To); home == nil {
			return nil, false, Refusef("`--to %s` is not a page of this app", o.To)
		}
	} else {
		for _, pg := range p.Pages {
			if pg.Path == "/" {
				home = pg
			}
		}
	}
	if home != nil && hasRequiredParams(home) {
		return nil, false, Refusef("`%s` has parameters: a login goes to a page without, name one with `--to <Page>`", home.Module)
	}

	var edits []Edit
	users := p.TableNamed(o.Users)
	created := users == nil
	userMod := p.Modules[o.Users]
	var role *types.Union
	var ctors []string
	if o.Role != "" {
		if userMod != nil {
			if t, ok := p.Res.Decls[o.Users+"."+o.Role].(*types.Union); ok {
				role = t
			}
		}
		if role != nil {
			for _, c := range role.Constructors() {
				if c.Arg != nil {
					role = nil
					break
				}
				ctors = append(ctors, c.Name)
			}
		}
		if role == nil {
			return nil, false, Refusef("`--role %s`: module `%s` has no union `%s` of constructors without payloads: `alder new type %s \"%s = Member | Admin\"` makes one", o.Role, o.Users, o.Role, o.Users, o.Role)
		}
	}
	roleColumn := ""
	if created {
		if userMod != nil {
			for _, d := range userMod.Syntax.Decls {
				if _, ok := d.(*syntax.TableDecl); ok {
					return nil, false, Refusef("module `%s` declares another table: a table is declared in the module of its name", o.Users)
				}
			}
			if declared(userMod.Syntax, o.Users+"Id") || declared(userMod.Syntax, o.Users) {
				return nil, false, Refusef("module `%s` declares `%s` or `%sId` already", o.Users, o.Users, o.Users)
			}
		}
		fields := []string{"id: " + o.Users + "Id @id", "email: Text.Email @unique", "name: Text.NonEmpty", hashColumn + ": Text.Secret"}
		if role != nil {
			roleColumn = "role"
			fields = append(fields, "role: "+o.Role+" = "+ctors[0])
		}
		fields = append(fields, "created: Time @created")
		table := "type " + o.Users + "Id = brand Text\n\ntable " + o.Users + " {\n  " + strings.Join(fields, ",\n  ") + ",\n}"
		if userMod == nil {
			edits = append(edits, Edit{Path: FileOf(o.Users), Create: true, Body: "module " + o.Users + "\n\n" + table + "\n"})
		} else {
			e := newSrcEdit(userMod.Source)
			e.appendText("\n" + table)
			edits = append(edits, Edit{Path: userMod.File, Body: e.result()})
		}
	} else {
		email, hash := users.Column("email"), users.Column(hashColumn)
		if email == nil || email.Maybe || email.Brand == nil || email.Brand.Module+"."+email.Brand.Name != "Text.Email" || hash == nil || hash.Maybe || hash.Kind != KindText {
			return nil, false, Refusef("`%s` needs the columns `email: Text.Email` and `%s: Text.Secret` (or Text) for a login: `alder edit table %s add-field …`", o.Users, hashColumn, o.Users)
		}
		if role != nil {
			for _, c := range users.Columns {
				if !c.Maybe && types.String(c.Type) == types.String(role) {
					roleColumn = c.Name
				}
			}
			if roleColumn == "" {
				return nil, false, Refusef("`%s` has no column of type `%s`: `alder edit table %s add-field role:%s=%s` adds one", o.Users, o.Role, o.Users, o.Role, ctors[0])
			}
		}
	}

	// the program with the user table, for its columns' types
	probe := p
	if created {
		var err error
		if probe, err = Probe(p.Dir, edits); err != nil {
			return nil, false, err
		}
		if r := probe.errorsIn(edits[0].Path); r != nil {
			return nil, false, r
		}
		if users = probe.TableNamed(o.Users); users == nil {
			return nil, false, Refusef("internal error: the table %s was not checked", o.Users)
		}
	}
	idType := types.StringIn(users.IDType, "App")

	// Auth.Login
	const module = "Auth.Login"
	imports := newImports(module, p.Std)
	imports.add(users.Module)
	d := AuthData{Dir: gen.AdapterPkg(module), Func: gen.AdapterFunc(module, "Password"), Table: users.Name,
		TableName: fmt.Sprintf("%q", users.Module+"."+users.Name), HashColumn: hashColumn, HashColumnQ: fmt.Sprintf("%q", hashColumn),
		IDType: types.StringIn(users.IDType, module), Role: role != nil, RoleColumn: roleColumn, Home: "?afterLogin", Iterations: 600000}
	if home != nil {
		d.Home = pageLit(home.Module, nil)
	}
	if role != nil {
		d.RoleDefault = ctors[0]
	}
	d.Open, d.Close = rootTags(p, "Log in", imports)
	tb := newTestBuilder(probe, module, imports)
	tb.let("let _ <- Test.fake Password")
	tb.let("let _ <- Test.get (Auth.Login {})")
	tb.let("let r <- Test.submit Credentials { email: Text.Email \"nobody@example.com\", password: Text.Secret \"secret\" }")
	role1 := ""
	if role != nil {
		role1 = " and their role"
	}
	d.Tests = append(d.Tests, TestData{Name: quote("an unknown email is rejected"), Lines: tb.lines,
		Comment: []string{"Logging in through the form, with the fake: an unknown email is rejected with the form's",
			"message, and a known one puts the user" + role1 + " in the session and goes to " + goesTo(d.Home) + "."},
		Result: "{ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: \"\", message: \"Email or password is incorrect.\" }], user: Nothing }"})
	tb = newTestBuilder(probe, module, imports)
	tb.let("let _ <- Test.fake Password")
	over := map[string]string{"email": "Text.Email \"me@example.com\""}
	lhs, rhs := "{ answer: r.answer, user: r.session.user", "{ answer: Redirected ("+d.Home+"), user: Just me"
	if role != nil { // a role other than the default, so a login that drops it fails
		r := ctors[len(ctors)-1]
		over[roleColumn] = r
		lhs, rhs = lhs+", role: r.session.role", rhs+", role: "+r
	}
	me := tb.insertRow(users, "me", over, true)
	tb.let("let _ <- Test.get (Auth.Login {})")
	tb.let("let r <- Test.submit Credentials { email: Text.Email \"me@example.com\", password: Text.Secret \"secret\" }")
	d.Tests = append(d.Tests, TestData{Name: quote("logging in through the form"), Lines: tb.lines, Result: strings.Replace(lhs+" } == "+rhs+" }", "Just me", "Just "+me, 1)})
	// logging out, from this page: App's Logout empties the session's user (and role)
	tb = newTestBuilder(probe, module, imports)
	over = map[string]string{}
	loginRole, outRole := "", ""
	if role != nil {
		over[roleColumn] = ctors[len(ctors)-1]
		loginRole, outRole = ", role: "+ctors[len(ctors)-1], ", role: r.session.role"
	}
	me = tb.insertRow(users, "me", over, true)
	tb.let("let _ <- Test.login { user: Just %s%s }", me, loginRole)
	tb.let("let _ <- Test.get (Auth.Login {})")
	tb.let("let r <- Test.click App.Logout")
	want := "{ answer: Redirected (Auth.Login {}), user: Nothing"
	if role != nil {
		want += ", role: " + ctors[0]
	}
	d.Tests = append(d.Tests, TestData{Name: quote("a signed-in user logs out here"), Lines: tb.lines,
		Result: "{ answer: r.answer, user: r.session.user" + outRole + " } == " + want + " }"})
	d.Imports = imports.list()
	edits = append(edits, Edit{Path: FileOf(module), Create: true, Body: canonical(p, FileOf(module), render("auth_login.tmpl", d))})

	// App
	roleField, roleType := "", ""
	if role != nil {
		roleType = types.StringIn(role, "App")
		roleField = "role: " + roleType + " = " + ctors[0]
	}
	appEdit, err := editApp(p, app, users, idType, roleField, ctors)
	if err != nil {
		return nil, false, err
	}
	edits = append(edits, appEdit)

	// the adapter and its tests
	pkg, err := adapterPackage(p.Dir, d.Dir)
	if err != nil {
		return nil, false, err
	}
	d.Package = pkg
	for _, f := range []struct{ tmpl, file string }{{"auth_adapter", "login_password.go"}, {"auth_adapter_test", "login_password_test.go"}} {
		src, err := format.Source([]byte(snippet(f.tmpl, d) + "\n"))
		if err != nil {
			return nil, false, fmt.Errorf("internal error: the adapter %s does not format: %v", f.file, err)
		}
		edits = append(edits, Edit{Path: "adapters/" + d.Dir + "/" + f.file, Create: true, Body: string(src)})
	}
	return edits, created, nil
}

// editApp is App with a login (RF1, chapter 15): the session's `user` (and role) fields after
// its others, `login = Auth.Login` after the session, `msg Logout` and its arm (in App's
// `update`, or in one created after `login`), the user module imported, and a test of Logout at
// the end. Nothing else moves.
func editApp(p *Program, app *Module, users *Table, idType, roleField string, ctors []string) (Edit, error) {
	e := newSrcEdit(app.Source)
	newFields := []string{"user: Maybe " + idType}
	if roleField != "" {
		newFields = append(newFields, roleField)
	}
	var sd *syntax.SessionDecl
	for _, d := range app.Syntax.Decls {
		if s, ok := d.(*syntax.SessionDecl); ok {
			sd = s
		}
	}
	login := "login = Auth.Login"
	put := "{ s | user: Nothing }"
	if roleField != "" {
		put = "{ s | user: Nothing, role: " + ctors[0] + " }"
	}
	arm := "Logout p -> {\n  let _ <- Session.put (\\s -> " + put + ")\n  Task.succeed (Redirect (Auth.Login {}))\n},"
	line, ind, hasUpdate, ok := updateArmSpot(e, app.Syntax)
	if !ok {
		return Edit{}, Refusef("`App`'s `update` is not `match msg { … }` written over several lines: add the Logout arm yourself")
	}
	var after string // what follows the session
	if hasUpdate {
		after = "\n" + login
		e.insertBefore(declStart(e, valueDecl(app.Syntax, "update")), "msg Logout\n")
		e.insertBefore(line, indent(arm, ind))
	} else {
		after = "\n" + login + "\n\nmsg Logout\n\nupdate : Msg -> Session -> Task Nav\nupdate msg session = match msg {\n" + indent(arm, "  ") + "\n}"
	}
	switch {
	case sd == nil:
		at := len(e.lines) + 1
		if len(app.Syntax.Decls) > 0 {
			at = declStart(e, app.Syntax.Decls[0])
		}
		e.insertBefore(at, "session { "+strings.Join(newFields, ", ")+" }"+after+"\n")
	case sd.Span.Start.Line == sd.Span.End.Line:
		l := e.line(sd.Span.Start.Line)
		open, close := strings.Index(l, "{"), sd.Span.End.Col-2
		if open < 0 || close >= len(l) || close < open || l[close] != '}' {
			return Edit{}, Refusef("`App`'s session is not `session { … }` where the parser says: add the fields yourself")
		}
		inner := strings.TrimSuffix(strings.TrimSpace(l[open+1:close]), ",")
		fields := newFields
		if inner != "" {
			fields = append([]string{inner}, newFields...)
		}
		e.replaceLine(sd.Span.Start.Line, l[:open]+"{ "+strings.Join(fields, ", ")+" }"+l[close+1:])
		e.insertBefore(sd.Span.End.Line+1, after)
	default: // over several lines: the fields before its closing line
		end := sd.Span.End.Line
		if strings.TrimSpace(e.line(end)) != "}" {
			return Edit{}, Refusef("`App`'s session does not end with `}` on a line of its own: add the fields yourself")
		}
		fieldInd := "  "
		if len(sd.Fields) > 0 {
			lastLine := sd.Fields[len(sd.Fields)-1].Span.End.Line
			fieldInd = indentOf(e.line(sd.Fields[len(sd.Fields)-1].Span.Start.Line))
			for n := lastLine; n < end; n++ { // the last field's comma
				if t := strings.TrimRight(e.line(n), " "); n == lastLine && !strings.HasSuffix(t, ",") {
					e.replaceLine(n, t+",")
				}
			}
		}
		for _, f := range newFields {
			e.insertBefore(end, fieldInd+f+",")
		}
		e.insertBefore(end+1, after)
	}

	// the test of Logout: it calls update itself, as every Log out button is on a page that
	// requires a user (book 10, "The logout redirect")
	sess := []string{"user: Just (" + idType + " \"me\")"}
	if roleField != "" {
		sess = append(sess, "role: "+ctors[len(ctors)-1])
	}
	imports := newImports("App", p.Std)
	imports.add(users.Module)
	tb := newTestBuilder(p, "App", imports)
	for i, f := range p.Session.Fields { // a record literal names every field: a default's value, else a sample
		if f.Name == "user" || f.Name == "role" {
			continue
		}
		v := ""
		if sd != nil && i < len(sd.Fields) && sd.Fields[i].Default != nil {
			v = sliceSpan(app.Syntax.Lines, sd.Fields[i].Default.GetSpan())
		}
		if v == "" {
			v = tb.sample(f.Type, f.Name, 1)
		}
		sess = append(sess, f.Name+": "+v)
	}
	e.appendText("\n" + snippet("test", TestData{Name: quote("Logout clears the user and goes to log in"),
		Comment: []string{"Every Log out button is on a page that requires a user, so a browser test cannot tell this",
			"Redirect from a Rerender (book 10, \"The logout redirect\"): the test calls update itself."},
		Lines:  []string{"let r <- Test.run (update Logout { " + strings.Join(sess, ", ") + " })"},
		Result: "{ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: [\"Session.put\"] }"}))
	addImports(e, p, app.Syntax, imports.list())
	return Edit{Path: app.File, Body: e.result()}, nil
}

// goesTo says where a login goes in a test's comment: the page, or, for the hole ?afterLogin
// (an app with no page yet), the page that hole will name.
func goesTo(home string) string {
	if strings.HasPrefix(home, "?") {
		return "the page " + home + " names (fill it, here and in Submit's arm, once the page exists)"
	}
	return home
}

func hasField(s *Session, name string) bool {
	for _, f := range s.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
