package generate

import (
	"fmt"
	"strings"
	"testing"
)

func runJob(t *testing.T, dir string, s Sets) (*Result, error) {
	t.Helper()
	return SpecByID("new job").Run(s, App{Dir: dir}, true)
}

func jobFile(t *testing.T, res *Result, path string) string {
	t.Helper()
	for _, f := range res.Files {
		if f.Path == path && f.After != nil {
			return *f.After
		}
	}
	t.Fatalf("no file %s in %+v", path, res.Files)
	return ""
}

// spec §10's block, as the generator writes it (the generated app also checks and passes its
// test: cmd/alder's generate tests)
const sendWelcome = `module Jobs.SendWelcome

import Table.User

job SendWelcome = { user: User.UserId }

SendWelcome.run : SendWelcome.Params -> Task Unit
SendWelcome.run p = {
  let _ <- Log.info "SendWelcome ran"
  Task.succeed ()
}

test "SendWelcome runs" = {
  let job = SendWelcome { user: User.UserId "u1" }
  let _ <- Job.enqueue job
  let runs <- Test.work
  let own = List.filter (\r -> r.job == job) runs
  Task.succeed (List.map (\r -> r.result) own == [Ok ()])
}
`

func TestNewJob_Params(t *testing.T) {
	res, err := runJob(t, fixture, Sets{"name": {"SendWelcome"}, "fields": {"user:User.UserId"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := jobFile(t, res, "app/Jobs/SendWelcome.alder"); got != sendWelcome {
		t.Fatalf("got:\n%s\nwant:\n%s", got, sendWelcome)
	}
}

func TestNewJob_Into(t *testing.T) {
	res, err := runJob(t, fixture, Sets{"name": {"SendWelcome"}, "fields": {"user:User.UserId"}, "into": {"Signup"}})
	if err != nil {
		t.Fatal(err)
	}
	got := jobFile(t, res, "app/Signup.alder")
	want := strings.Replace(sendWelcome, "module Jobs.SendWelcome", "module Signup", 1)
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestNewJob_Scheduled(t *testing.T) {
	for _, c := range []struct {
		flag, val, want string
	}{
		{"every", "1h", "job Sweep = {} every \"1h\"\n"},
		{"cron", "0 3 * * *", "job Sweep = {} cron \"0 3 * * *\"\n"},
	} {
		res, err := runJob(t, scaffoldApp(t, nil), Sets{"name": {"Sweep"}, c.flag: {c.val}})
		if err != nil {
			t.Fatalf("--%s: %v", c.flag, err)
		}
		got := jobFile(t, res, "app/Jobs/Sweep.alder")
		for _, w := range []string{c.want, "let _ <- Test.setClock (Time.fromMillis ", "let start <- Test.work", "let _ <- Test.advance ", "let runs <- Test.work", "let job = Sweep {}", "List.filter (\\r -> r.job == job) runs"} {
			if !strings.Contains(got, w) {
				t.Errorf("--%s: missing %q in:\n%s", c.flag, w, got)
			}
		}
	}
	res, err := runJob(t, scaffoldApp(t, nil), Sets{"name": {"Nightly"}, "cron": {"0 3 * * *"}})
	if err != nil {
		t.Fatal(err)
	}
	// 02:59, a minute before 03:00; advanced 61 s, a second past the tick
	got := jobFile(t, res, "app/Jobs/Nightly.alder")
	if !strings.Contains(got, "Test.setClock (Time.fromMillis 1772593140000)") || !strings.Contains(got, "Test.advance 61000") {
		t.Errorf("cron clock:\n%s", got)
	}
}

func TestNewJob_Refusals(t *testing.T) {
	dir := fixture
	for _, c := range []struct {
		s    Sets
		want string
	}{
		{Sets{"name": {"Sweep"}, "fields": {"n:Int"}, "every": {"1h"}}, "A scheduled job takes no parameters"},
		{Sets{"name": {"Sweep"}, "fields": {"n:Int"}, "cron": {"0 3 * * *"}}, "A scheduled job takes no parameters"},
		{Sets{"name": {"Sweep"}, "every": {"1h"}, "cron": {"0 3 * * *"}}, "A job has two schedules"},
		{Sets{"name": {"Sweep"}, "every": {"1x"}}, "cannot read `1x` as a duration"},
		{Sets{"name": {"Sweep"}, "every": {"500ms"}}, "is shorter than the minimum, `1s`"},
		{Sets{"name": {"Sweep"}, "cron": {"61 * * * *"}}, "`cron \"61 * * * *\"`"},
		{Sets{"name": {"sweep"}}, "is not a job name"},
		{Sets{"name": {"Sweep"}, "fields": {"n:Int", "n:Int"}}, "field `n` is given twice"},
	} {
		_, err := runJob(t, dir, c.s)
		text := fmt.Sprint(err)
		if r, ok := err.(*Refusal); ok { // the checker's errors: the diagnostics hold the wording
			for _, d := range r.Diags {
				text += "\n" + d.Message
			}
		}
		if err == nil || !strings.Contains(text, c.want) {
			t.Errorf("%v: err = %v, want %q", c.s, err, c.want)
		}
	}
	// a job already there is not written over
	scratch := scaffoldApp(t, nil)
	if _, err := SpecByID("new job").Run(Sets{"name": {"Sweep"}}, App{Dir: scratch}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := runJob(t, scratch, Sets{"name": {"Sweep"}, "into": {"Jobs.Sweep"}}); err == nil || !strings.Contains(err.Error(), "already declares `Sweep`") {
		t.Errorf("twice: %v", err)
	}
}
