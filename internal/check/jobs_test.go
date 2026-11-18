package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
)

// jobApp is silo-routes-ok (an app with `silo org`) with a User table that has an email, an
// Email capability, and the module Signup.alder set to signup.
func jobApp(t *testing.T, signup string) map[string]string {
	t.Helper()
	files := map[string]string{}
	dir := "../../testdata/corpus/silo-routes-ok/app"
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(b)
	}
	files["App.alder"] = strings.Replace(files["App.alder"], "role: User.Role = Reader", "role: User.Role = Member", 1)
	files["User.alder"] = "module User\n\ntype UserId = brand Text\n\ntype Role = Member | Admin\n\ntable User {\n  id: UserId @id,\n  email: Text.Email,\n  name: Text.NonEmpty,\n  passwordHash: Text,\n  role: Role,\n}\n"
	files["Email.alder"] = "module Email\n\ncapability Email {\n  send : { to: Text.Email, subject: Text, body: Html } -> Task Unit,\n}\n"
	files["Signup.alder"] = signup
	return files
}

// signupOK is the spec's opening Signup module (#168 §Intent), with a named parameter for
// `Nightly.run` and `Task.succeed ()`, redirecting to a route of this app.
const signupOK = `module Signup

import User
import Email

job SendWelcome = { user: User.UserId }

SendWelcome.run : SendWelcome.Params -> Task Unit
SendWelcome.run p = {
  let user <- Db.get User p.user
  match user {
    Just u -> Email.send { to: u.email, subject: "Welcome", body: <p>Hello {u.name}</p> },
    Nothing -> Task.succeed (),
  }
}

form NewUser = { email: Text.Email, name: Text.NonEmpty }

route Create = POST "/signup" { } form NewUser

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert User { email: p.form.email, name: p.form.name, passwordHash: "", role: Member }
  let _ <- Job.enqueue (SendWelcome { user: id })
  Task.succeed (Response.redirect (Projects.Index {}))
}

job Nightly = {} cron "0 3 * * *"

Nightly.run : Nightly.Params -> Task Unit
Nightly.run p = Log.info "cleanup ran"
`

// jobModule is a module Signup with the given declarations after its imports.
func jobModule(decls string) string {
	return "module Signup\n\nimport User\nimport Org\n\n" + decls + "\n"
}

// runArm is the arm of job name that does nothing.
func runArm(name string) string {
	return name + ".run : " + name + ".Params -> Task Unit\n" + name + ".run p = Task.succeed ()\n"
}

// wantJobDiag asserts that got holds exactly one diagnostic, of code, whose text contains each of subs.
func wantJobDiag(t *testing.T, got []string, code string, subs ...string) {
	t.Helper()
	if len(got) != 1 || !strings.HasPrefix(got[0], code+" ") {
		t.Fatalf("got %q, want one %s", got, code)
	}
	for _, s := range subs {
		if !strings.Contains(got[0], s) {
			t.Errorf("%q does not contain %q", got[0], s)
		}
	}
}

func TestJob_OK(t *testing.T) {
	res, got := checkFiles(t, jobApp(t, signupOK))
	if len(got) != 0 {
		t.Fatalf("Signup should check clean:\n%s", strings.Join(got, "\n"))
	}
	js := res.Jobs["Signup"]
	if len(js) != 2 || js[0].Name != "SendWelcome" || js[1].Name != "Nightly" {
		t.Fatalf("jobs = %+v", js)
	}
	if js[0].Params.Name != "SendWelcome.Params" || len(js[0].Params.Fields) != 1 || js[1].Cron != "0 3 * * *" {
		t.Errorf("SendWelcome.Params = %+v, Nightly.Cron = %q", js[0].Params, js[1].Cron)
	}
}

// A job's arm, `Name.Params` and constructor resolve like a route's (no E0044), within its
// module and from another module that imports it.
func TestJob_Names(t *testing.T) {
	files := jobApp(t, jobModule("job Ping = { n: Int }\n\n"+runArm("Ping")+"\nping : Ping.Params -> Job\nping p = Ping p\n"))
	files["Other.alder"] = "module Other\n\nimport Signup\n\nmore : Job\nmore = Signup.Ping { n: 1 }\n\nsame : Bool\nsame = Signup.Ping { n: 1 } == Signup.ping { n: 1 }\n"
	if _, got := checkFiles(t, files); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestJob_MissingRun(t *testing.T) {
	_, got := checkFiles(t, jobApp(t, jobModule("job SendWelcome = { user: User.UserId }")))
	wantJobDiag(t, got, diag.RouteDecl, "job `SendWelcome`", "run", "SendWelcome.Params -> Task Unit")
}

func TestJob_RunWrongType(t *testing.T) {
	_, got := checkFiles(t, jobApp(t, jobModule("job Ping = {}\n\nPing.run : Ping.Params -> Task Int\nPing.run p = Task.succeed 1")))
	wantJobDiag(t, got, diag.RouteDecl, "Ping.run", "Ping.Params -> Task Unit")
}

func TestJob_ParamNotStorable(t *testing.T) {
	refused := map[string]string{
		"f: Int -> Int":                "Int -> Int",
		"f: Task Int":                  "Task Int",
		"f: Query Int":                 "Query Int",
		"f: Html":                      "Html",
		"f: Job":                       "Job",
		"f: Route":                     "Route",
		"f: List (Maybe (Int -> Int))": "List (Maybe (Int -> Int))",
	}
	for field, ty := range refused {
		t.Run(field, func(t *testing.T) {
			_, got := checkFiles(t, jobApp(t, jobModule("job J = { "+field+" }\n\n"+runArm("J"))))
			wantJobDiag(t, got, diag.JobParam, "`f`", "`"+ty+"`")
		})
	}
	accepted := []string{
		"f: List (Maybe User.UserId)",
		"f: { a: Int, b: Maybe Text }",
		"f: User.UserId",
		"f: Shape",
		"f: Tree",
		"f: Result Text Int",
		"f: Maybe (Result Text (List Shape))",
	}
	types := "type Shape = Circle Float | Square { side: Float } | Dot\n\ntype Tree = Leaf | Node (List Tree)\n\n"
	for _, field := range accepted {
		t.Run(field, func(t *testing.T) {
			if _, got := checkFiles(t, jobApp(t, jobModule(types+"job J = { "+field+" }\n\n"+runArm("J")))); len(got) != 0 {
				t.Fatalf("got %q", got)
			}
		})
	}
	t.Run("Result of a function", func(t *testing.T) { // at its arguments, not its parameters
		_, got := checkFiles(t, jobApp(t, jobModule("job J = { f: Result Text Int, g: Result (Int -> Int) Int }\n\n"+runArm("J"))))
		wantJobDiag(t, got, diag.JobParam, "`g`")
	})
	t.Run("app type with a function", func(t *testing.T) {
		_, got := checkFiles(t, jobApp(t, jobModule("type Box = Box (Int -> Int)\n\njob J = { f: Box }\n\n"+runArm("J"))))
		wantJobDiag(t, got, diag.JobParam, "`f`", "`Box`")
	})
}

func TestJob_Schedules(t *testing.T) {
	cases := []struct{ decl, sub string }{
		{`job J = {} every "1h" cron "0 3 * * *"`, "A job has two schedules"},
		{`job J = { n: Int } every "1h"`, "A scheduled job takes no parameters"},
		{`job J = { n: Int } cron "0 3 * * *"`, "A scheduled job takes no parameters"},
		{`job J = {} every "500ms"`, "1s"},
		{`job J = {} every "3x"`, "`3x`"},
	}
	for _, tc := range cases {
		t.Run(tc.decl, func(t *testing.T) {
			_, got := checkFiles(t, jobApp(t, jobModule(tc.decl+"\n\n"+runArm("J"))))
			wantJobDiag(t, got, diag.JobSchedule, tc.sub)
		})
	}
	if _, got := checkFiles(t, jobApp(t, jobModule("job J = {} every \"1s\" retries 0 timeout \"1h\"\n\n"+runArm("J")))); len(got) != 0 {
		t.Fatalf("every 1s, timeout 1h: got %q", got)
	}
}

func TestJob_Options(t *testing.T) {
	cases := []struct{ decl, code, sub string }{
		{`job J = {} timeout "2h"`, diag.JobSchedule, "1h"},
		{`job J = {} timeout "soon"`, diag.JobSchedule, "`soon`"},
		{`job J = {} retries -1`, diag.ParseError, "retries"},
		{`job J = { n: Int } silo org`, diag.SiloDecl, "`silo org` takes `org` from the job's parameter: add `org: Org.OrgId` to the job's parameters"},
		{`job J = { org: Text } silo org`, diag.SiloDecl, "the job's parameter `org` is `Text`; `silo org` needs `Org.OrgId`"},
		{`job J = { user: User.UserId } silo user`, diag.SiloDecl, "`silo user`: the app has no `silo user"},
		{`job J = { team: Int } silo team`, diag.SiloDecl, "`silo team`: the session has no field `team`"},
	}
	for _, tc := range cases {
		t.Run(tc.decl, func(t *testing.T) {
			_, got := checkFiles(t, jobApp(t, jobModule(tc.decl+"\n\n"+runArm("J"))))
			if len(got) == 0 || !strings.HasPrefix(got[0], tc.code+" ") || !strings.Contains(got[0], tc.sub) {
				t.Fatalf("got %q, want %s containing %q", got, tc.code, tc.sub)
			}
		})
	}
	res, got := checkFiles(t, jobApp(t, jobModule("job J = { org: Org.OrgId } silo org retries 2 timeout \"30s\"\n\n"+runArm("J"))))
	if len(got) != 0 {
		t.Fatalf("silo org: got %q", got)
	}
	if j := res.Jobs["Signup"][0]; j.Silo != "org" || j.Retries != 2 || j.Timeout.String() != "30s" {
		t.Errorf("job = %+v", j)
	}
	res, _ = checkFiles(t, jobApp(t, jobModule("job J = {} every \"1h\"\n\n"+runArm("J"))))
	if j := res.Jobs["Signup"][0]; j.Retries != 4 || j.Timeout.String() != "5m0s" || j.Every.String() != "1h0m0s" {
		t.Errorf("defaults: job = %+v", j)
	}
}

func TestJob_ScopedDbInRun(t *testing.T) {
	src := "module Signup\n\nimport Project\n\njob J = {}\n\nJ.run : J.Params -> Task Unit\nJ.run p = {\n  let _ <- Db.insertScoped Project { name: \"x\" }\n  Task.succeed ()\n}\n"
	_, got := checkFiles(t, jobApp(t, src))
	wantJobDiag(t, got, diag.ScopedOutside, "a job without `silo org` runs on the shared store")
	if e := diag.Explain[diag.ScopedOutside]; !strings.Contains(e, "A job's `run` has no session either") {
		t.Errorf("explain E0026 = %q", e)
	}
}

func TestJob_NoSession(t *testing.T) {
	_, got := checkFiles(t, jobApp(t, jobModule("job J = {}\n\nJ.run : J.Params -> Task Unit\nJ.run p = {\n  let _ <- Session.put session\n  Task.succeed ()\n}")))
	if len(got) == 0 || !strings.HasPrefix(got[0], diag.UnknownName+" ") || !strings.Contains(got[0], "session") {
		t.Fatalf("got %q, want an unknown name `session`", got)
	}
}

// A `silo org` job's `run` may use its silo's tables, scoped or not (review fix 1).
func TestJob_SiloRunUsesItsSilo(t *testing.T) {
	src := "module Signup\n\nimport Org\nimport Project\n\njob J = { org: Org.OrgId } silo org\n\n" +
		"J.run : J.Params -> Task Unit\nJ.run p = {\n  let _ <- Db.insertScoped Project { name: \"x\" }\n  let ps <- Db.from Project |> Db.all\n  Task.succeed ()\n}\n"
	if _, got := checkFiles(t, jobApp(t, src)); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
	// without `silo org`, an unscoped write of a silo's table is refused too
	src = "module Signup\n\nimport Org\nimport Project\n\njob J = {}\n\nJ.run : J.Params -> Task Unit\nJ.run p = {\n  let _ <- Db.insert Project { name: \"x\", org: Org.OrgId \"o\" }\n  Task.succeed ()\n}\n"
	_, got := checkFiles(t, jobApp(t, src))
	if len(got) == 0 || !strings.HasPrefix(got[0], diag.ScopedOutside+" ") || !strings.Contains(got[0], "runs on the shared store") {
		t.Fatalf("got %q, want E0026", got)
	}
}

// A job's `run` that reaches a helper of its module using a scoped operation is E0026 (review
// fix 2), in a module with a route (where the helper itself is allowed).
func TestJob_HelperScopedInRun(t *testing.T) {
	src := "module Signup\n\nimport Project\n\nroute Home = GET \"/home\" { } requires user, org\n\n" +
		"Home.load : Home.Params -> Session -> Query Int\nHome.load p s = Query.succeed 0\n\n" +
		"Home.view : Int -> Session -> Html\nHome.view n s = <p>hi</p>\n\n" +
		"add : Task Unit\nadd = {\n  let _ <- Db.insertScoped Project { name: \"x\" }\n  Task.succeed ()\n}\n\n" +
		"job J = {}\n\nJ.run : J.Params -> Task Unit\nJ.run p = add\n"
	_, got := checkFiles(t, jobApp(t, src))
	wantJobDiag(t, got, diag.ScopedOutside, "`add` uses", "runs on the shared store")
}

// A job's `run` that writes two stores is E0031 (review fix 3).
func TestJob_RunWritesTwoStores(t *testing.T) {
	src := "module Signup\n\nimport Org\nimport Project\n\njob J = { org: Org.OrgId } silo org\n\n" +
		"J.run : J.Params -> Task Unit\nJ.run p = {\n  let _ <- Db.insertScoped Project { name: \"x\" }\n" +
		"  let _ <- Db.insert Org { name: \"o\" }\n  Task.succeed ()\n}\n"
	_, got := checkFiles(t, jobApp(t, src))
	wantJobDiag(t, got, diag.WriteStores, "`J.run` writes", "`Project` (silo `org`)", "`Org` (shared)")
}

// `Session.put` in a job's `run`, written there or reached through a helper, is refused (review
// fix 4).
func TestJob_SessionPutInRun(t *testing.T) {
	_, got := checkFiles(t, jobApp(t, jobModule("job J = {}\n\nJ.run : J.Params -> Task Unit\nJ.run p = Session.put (\\s -> { s | user: Nothing })")))
	wantJobDiag(t, got, diag.TypeMismatch, "`Session.put` cannot be used in a job's `run`")
	_, got = checkFiles(t, jobApp(t, jobModule("signOut : Task Unit\nsignOut = Session.put (\\s -> { s | user: Nothing })\n\n"+
		"job J = {}\n\nJ.run : J.Params -> Task Unit\nJ.run p = signOut")))
	wantJobDiag(t, got, diag.TypeMismatch, "`J.run` reaches `Session.put` through `Signup.signOut`")
}

// A job's name is checked like a route's (E0044, review fix 5).
func TestJob_NameClashes(t *testing.T) {
	_, got := checkFiles(t, jobApp(t, jobModule("job J = {}\n\njob J = { n: Int }\n\n"+runArm("J"))))
	if len(got) == 0 || !strings.HasPrefix(got[0], diag.RouteName+" ") || !strings.Contains(got[0], "job `J` is already declared at line") {
		t.Fatalf("duplicate: got %q", got)
	}
	_, got = checkFiles(t, jobApp(t, jobModule("type Kind = Circle | Square\n\njob Circle = {}\n\n"+runArm("Circle"))))
	if len(got) == 0 || !strings.HasPrefix(got[0], diag.RouteName+" ") || !strings.Contains(got[0], "job `Circle` is named like the constructor `Circle`") {
		t.Fatalf("constructor: got %q", got)
	}
}

// Another module's job constructor needs no import, like a route's (controller ruling 6).
func TestJob_CtorWithoutImport(t *testing.T) {
	files := jobApp(t, jobModule("job Ping = { n: Int }\n\n"+runArm("Ping")))
	files["Other.alder"] = "module Other\n\nmore : Job\nmore = Signup.Ping { n: 1 }\n"
	if _, got := checkFiles(t, files); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

// Job.enqueue is not a write for E0031 (spec §2): a `requires org` route that only enqueues, and
// one that writes the org's project and enqueues, both check.
func TestJob_EnqueueNotAWrite(t *testing.T) {
	files := jobApp(t, jobModule("job J = { n: Int }\n\n"+runArm("J")))
	files["Projects.alder"] = strings.Replace(files["Projects.alder"], "  let _ <- Db.insertScoped Project { name: params.form.name }\n",
		"  let _ <- Db.insertScoped Project { name: params.form.name }\n  let _ <- Job.enqueue (Signup.J { n: 1 })\n", 1)
	files["Projects.alder"] += "\nroute Ping = POST \"/projects/ping\" { } requires user, org\n\n" +
		"Ping.update : Ping.Params -> Session -> Task Response\nPing.update params session = {\n" +
		"  let _ <- Job.enqueueOnce \"ping\" (Signup.J { n: 2 })\n  Task.succeed (Response.redirect Index)\n}\n"
	if !strings.Contains(files["Projects.alder"], "Job.enqueue (") {
		t.Fatal("fixture: Add.update not found")
	}
	if _, got := checkFiles(t, files); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

// Job.State's constructors resolve unqualified as the last resort: an app's own `Done` (or an
// imported one) wins, and elsewhere `Queued` is Job's.
func TestJob_StateConstructors(t *testing.T) {
	use := "state : Job.State\nstate = Queued\n\nisDone : Job.Status -> Bool\nisDone s = match s.state {\n  Done -> True,\n  _ -> False,\n}\n"
	if _, got := checkFiles(t, jobApp(t, jobModule(use))); len(got) != 0 {
		t.Fatalf("Job's constructors: got %q", got)
	}
	own := "type Step = Todo | Done\n\nlast : Step\nlast = Done\n"
	if _, got := checkFiles(t, jobApp(t, jobModule(own))); len(got) != 0 {
		t.Fatalf("own Done: got %q", got)
	}
	// imported: Other's Done, not Job's
	files := jobApp(t, jobModule("import Other\n\nlast : Other.Step\nlast = Done\n"))
	files["Signup.alder"] = strings.Replace(files["Signup.alder"], "import Org\n\nimport Other\n", "import Org\nimport Other\n", 1)
	files["Other.alder"] = "module Other\n\ntype Step = Todo | Done\n"
	if _, got := checkFiles(t, files); len(got) != 0 {
		t.Fatalf("imported Done: got %q", got)
	}
	// another module's Done, not imported here, does not hide Job's
	files = jobApp(t, jobModule(use))
	files["Other.alder"] = "module Other\n\ntype Step = Todo | Done\n"
	if _, got := checkFiles(t, files); len(got) != 0 {
		t.Fatalf("unimported Done elsewhere: got %q", got)
	}
	// own Done is not Job's: using it as a Job.State is a type error
	_, got := checkFiles(t, jobApp(t, jobModule(own+"\nstate : Job.State\nstate = Done\n")))
	if len(got) != 1 || !strings.HasPrefix(got[0], diag.TypeMismatch+" ") {
		t.Fatalf("own Done as Job.State: got %q", got)
	}
}
