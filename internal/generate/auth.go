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
	Home        string // the route value a login redirects to: "Home {}"
	Open, Close string
	Iterations  int
	Tests       []TestData
}

// hashColumn is the column a user table keeps its password hash in.
const hashColumn = "passwordHash"

// NewAuth generates `alder new auth` (#160: routes): the login module app/Auth/Login.alder (the
// capability Password { verify, hash }, its fake, `route Login = GET "/login"` with its view, the
// form Credentials and `route SignIn = POST "/session" { } form Credentials`, whose
// update puts the user (and the role) in the session and redirects, or answers
// Response.invalid; two tests), the edits of App (the session's `user` and `role` fields,
// `login = Auth.Login.Login`, `route Logout = DELETE "/session"` and its update (the session is the resource: POST /session
// signs in, DELETE /session signs out), a test of it:
// editApp), the user table when it does not exist (with its migration, the caller's
// Options.Migrate), and the Go adapter adapters/auth/login_password.go (PBKDF2-SHA256) with its
// tests. The login goes to --to's route (loginTarget), else the GET route (or page) at `/`, else
// (an app with no route yet: `alder new <app> --bare`, whose routes come after the login they
// require) the typed hole ?afterLogin, in SignIn.update and in its test, filled once the route
// exists (EditAuth). It refuses an app whose session has a `user`, that has an Auth module or a
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
	if declared(app.Syntax, "Logout") || routeDecl(app.Syntax, "Logout") != nil {
		return nil, false, Refusef("`App` already declares `Logout`")
	}
	for _, w := range []struct{ name, method string }{{"SignIn", "POST"}, {"Logout", "DELETE"}} {
		if have := routeClash(p, w.method, "/session", ""); have != nil {
			return nil, false, Refusef("routes `%s.%s` and `%s` both answer %s /session; `alder new auth` makes the session a resource (POST /session signs in, DELETE /session signs out): move `%s` to another path first", have.Module, have.Name, w.name, w.method, have.Name)
		}
	}
	if !IsUpperIdent(o.Users) {
		return nil, false, Refusef("`--users %s`: a table is named by one capitalised identifier, such as `User`", o.Users)
	}

	// the page a login goes to: a GET route (or a route module written before routes)
	home, homeParams := "", false
	if o.To != "" {
		var ok bool
		if home, homeParams, ok = loginTarget(p, o.To, "Auth.Login"); !ok {
			return nil, false, Refusef("`--to %s` is not a page of this app: name a route module (`Home`) or a GET route (`Posts.Index.Index`)", o.To)
		}
	} else {
		for _, r := range p.Routes {
			if !r.Write() && r.Path == "/" && home == "" {
				home, homeParams = r.Ref("Auth.Login")+" {}", routeHasRequiredParams(r)
			}
		}
	}
	if home != "" && homeParams {
		return nil, false, Refusef("`%s` has parameters: a login goes to a page without, name one with `--to <Route>`", strings.TrimSuffix(home, " {}"))
	}

	var edits []Edit
	users := p.TableNamed(o.Users)
	created := users == nil
	userModName := tableModuleOf(p, o.Users) // Table.<Users> for a table made here (#20)
	userMod := p.Modules[userModName]
	if root := p.Modules[o.Users]; created && root != nil {
		return nil, false, Refusef("module `%s` exists (`%s`): the user table goes in `%s` (module `%s`), and `%s` would then name two modules; move `%s`'s declarations there, or name another table with --users", o.Users, root.File, FileOf(userModName), userModName, o.Users, root.File)
	}
	var role *types.Union
	var ctors []string
	if o.Role != "" {
		if userMod != nil {
			if t, ok := p.Res.Decls[userModName+"."+o.Role].(*types.Union); ok {
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
			return nil, false, Refusef("`--role %s`: module `%s` has no union `%s` of constructors without payloads: `alder new type %s \"%s = Member | Admin\"` makes one", o.Role, userModName, o.Role, userModName, o.Role)
		}
	}
	roleColumn := ""
	if created {
		if userMod != nil {
			for _, d := range userMod.Syntax.Decls {
				if _, ok := d.(*syntax.TableDecl); ok {
					return nil, false, Refusef("module `%s` declares another table: a table is declared in the module of its name", userModName)
				}
			}
			if declared(userMod.Syntax, o.Users+"Id") || declared(userMod.Syntax, o.Users) {
				return nil, false, Refusef("module `%s` declares `%s` or `%sId` already", userModName, o.Users, o.Users)
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
			edits = append(edits, Edit{Path: FileOf(userModName), Create: true, Body: "module " + userModName + "\n\n" + table + "\n"})
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
	if home != "" {
		d.Home = home
	}
	if role != nil {
		d.RoleDefault = ctors[0]
	}
	d.Open, d.Close = rootTags(p, "Log in", imports)
	tb := newTestBuilder(probe, module, imports)
	tb.let("let _ <- Test.fake Password")
	tb.let("let _ <- Test.get (Login {})")
	tb.let("let r <- Test.submit SignIn { email: Text.Email \"nobody@example.com\", password: Text.Secret \"secret\" }")
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
	tb.let("let _ <- Test.get (Login {})")
	tb.let("let r <- Test.submit SignIn { email: Text.Email \"me@example.com\", password: Text.Secret \"secret\" }")
	d.Tests = append(d.Tests, TestData{Name: quote("logging in through the form"), Lines: tb.lines, Result: strings.Replace(lhs+" } == "+rhs+" }", "Just me", "Just "+me, 1)})
	d.Imports = imports.list()
	loginBody, err := canonical(probe, FileOf(module), render("auth_login.tmpl", d)) // probe: it knows a user table created here (its name, recordName)
	if err != nil {
		return nil, false, err
	}
	edits = append(edits, Edit{Path: FileOf(module), Create: true, Body: loginBody})

	// App
	roleField, roleType := "", ""
	if role != nil {
		roleType = types.StringIn(role, "App")
		roleField = "role: " + roleType + " = " + ctors[0]
	}
	appEdit, err := editApp(probe, app, users, idType, roleField, roleColumn, ctors) // probe: it knows a user table created here
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
// its others, `login = Auth.Login.Login` and `route Logout = DELETE "/session" { }` after the
// session, the user module imported, and at the end `Logout.update` (it empties the session's
// user and role and goes to log in) and a test of it. Nothing else moves. Logout is a `DELETE`
// on /session (the session is the resource: POST /session signs in, DELETE /session signs out):
// a button `click={App.Logout}` sends it, a form with a hidden `_method` does without
// JavaScript, so a link (a GET) can never log anyone out.
func editApp(p *Program, app *Module, users *Table, idType, roleField, roleColumn string, ctors []string) (Edit, error) {
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
	after := "\nlogin = " + loginRoute + "\n\nroute Logout = DELETE \"/session\" { }" // what follows the session
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

	put := "{ s | user: Nothing }"
	if roleField != "" {
		put = "{ s | user: Nothing, role: " + ctors[0] + " }"
	}
	e.appendText("\n-- Logout empties the session's user and goes to log in: a `DELETE` of the session, sent by a button\n" +
		"-- (`click={App.Logout}`), a form without JavaScript.\n" +
		"Logout.update : Logout.Params -> Session -> Task Response\n" +
		"Logout.update params session = {\n  let _ <- Session.put (\\s -> " + put + ")\n  Task.succeed (Response.redirect (" + loginRoute + " {}))\n}")

	// the test of Logout: from the login page, which a visitor may open, so the redirect is
	// told from the page shown again (on a page that requires a user both end at the login)
	imports := newImports("App", p.Std)
	imports.add(users.Module)
	tb := newTestBuilder(p, "App", imports)
	over := map[string]string{}
	loginRole, outRole, want := "", "", "{ answer: Redirected ("+loginRoute+" {}), user: Nothing"
	if roleField != "" {
		r := ctors[len(ctors)-1]
		over[roleColumn] = r
		loginRole, outRole, want = ", role: "+r, ", role: r.session.role", want+", role: "+ctors[0]
	}
	me := tb.insertRow(users, "me", over, true)
	tb.let("let _ <- Test.login { user: Just %s%s }", me, loginRole)
	tb.let("let _ <- Test.get (%s {})", loginRoute)
	tb.let("let r <- Test.click Logout")
	e.appendText("\n" + snippet("test", TestData{Name: quote("Logout clears the user and goes to log in"),
		Comment: []string{"Logging out from the login page, which a visitor may open: the answer is the redirect,",
			"not the page shown again."},
		Lines:  tb.lines,
		Result: "{ answer: r.answer, user: r.session.user" + outRole + " } == " + want + " }"}))
	addImports(e, p, app.Syntax, imports.list())
	return Edit{Path: app.File, Body: e.result()}, nil
}

// loginRoute is the route `alder new auth` writes for App's `login`.
const loginRoute = "Auth.Login.Login"

// loginTarget is the route value a login goes to, written in module home, for name: a route
// module (its GET route, Plan call 14), a GET route `Module.Route`, or a route module written
// before routes; hasParams when it has a parameter without a default; ok false when name is
// none of these.
func loginTarget(p *Program, name, home string) (lit string, hasParams, ok bool) {
	r := p.RouteModule(name)
	if r == nil {
		for _, c := range p.Routes {
			if !c.Write() && c.Module+"."+c.Name == name {
				r = c
			}
		}
	}
	if r == nil {
		return "", false, false
	}
	return r.Ref(home) + " {}", routeHasRequiredParams(r), true
}

// routeHasRequiredParams reports whether route r has a parameter without a default.
func routeHasRequiredParams(r *Route) bool {
	for _, par := range r.Params {
		if !par.HasDefault {
			return true
		}
	}
	return false
}

// goesTo says where a login goes in a test's comment: the page, or, for the hole ?afterLogin
// (an app with no page yet), the page that hole will name.
func goesTo(home string) string {
	if strings.HasPrefix(home, "?") {
		return "the page " + home + " names (fill it, here and in SignIn.update, once the page exists)"
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
