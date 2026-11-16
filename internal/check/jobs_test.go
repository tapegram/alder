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

// signupOK is the spec's opening Signup module (#168 §Intent), without `Job.enqueue` (Task 4
// adds the Job module), with a named parameter for `Nightly.run`, redirecting to a route of
// this app.
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
  let welcome = SendWelcome { user: id }
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
	}
	types := "type Shape = Circle Float | Square { side: Float } | Dot\n\ntype Tree = Leaf | Node (List Tree)\n\n"
	for _, field := range accepted {
		t.Run(field, func(t *testing.T) {
			if _, got := checkFiles(t, jobApp(t, jobModule(types+"job J = { "+field+" }\n\n"+runArm("J")))); len(got) != 0 {
				t.Fatalf("got %q", got)
			}
		})
	}
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
	wantJobDiag(t, got, diag.ScopedOutside, "job's `run`")
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
