package generate

import (
	"fmt"
	"strings"
	"time"

	"alder/internal/check"
	"alder/internal/types"
	"alder/rt/jobs"
)

// JobData is the data of job.tmpl.
type JobData struct {
	Module   string
	Imports  []string
	Name     string
	Params   string // the record type as written: "{ user: User.UserId }", or "{}"
	Schedule string // ` every "1h"` or ` cron "0 3 * * *"` (with its leading space), or ""
}

// JobOptions are the options of `alder new job`.
type JobOptions struct {
	Name   string
	Fields []FieldSpec
	Into   string // --into: the module the job goes in (default Jobs.<Name>)
	Every  string // --every: `every "<Every>"`
	Cron   string // --cron: `cron "<Cron>"`
}

// jobTestBase is the instant a scheduled job's test starts from, 2026-03-04T00:00:00Z (the day
// examples/todos' Nightly test uses).
var jobTestBase = time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)

// NewJob generates `alder new job <Name> [<field>:<Type> …] [--every D | --cron E] [--into M]`
// (#168, book chapter 17): the declaration of job Name with its `Name.run` (which logs and
// succeeds), and a test, in module Jobs.<Name> (created) or in --into's module (appended to
// when it exists). A job with fields is tested by enqueueing it with sample parameters and
// working; a scheduled one (no fields) by setting the test's clock a little before its first
// tick, working once (the worker's start: it queues the tick and runs nothing), advancing past
// the tick and working again. `--every` with fields and both schedules are refused with E0050's
// wording; a schedule the checker cannot read is refused with the checker's own message.
func NewJob(p *Program, o JobOptions) ([]Edit, error) {
	if !IsUpperIdent(o.Name) {
		return nil, Refusef("`%s` is not a job name: write a capitalised name, such as `SendWelcome`", o.Name)
	}
	module := o.Into
	if module == "" {
		module = "Jobs." + o.Name
	}
	if !IsModuleName(module) {
		return nil, Refusef("`%s` is not a module name: write capitalised names joined by dots, such as `Signup`", module)
	}
	scheduled := o.Every != "" || o.Cron != ""
	switch {
	case o.Every != "" && o.Cron != "":
		return nil, Refusef("A job has two schedules: `--every` and `--cron` are both given for job `%s`; keep one", o.Name)
	case scheduled && len(o.Fields) > 0:
		return nil, Refusef("A scheduled job takes no parameters: job `%s` runs on its schedule, with nothing to fill them; leave out the fields, or leave out `--every`/`--cron`", o.Name)
	}
	seen := map[string]bool{}
	d := JobData{Module: module, Name: o.Name, Params: "{}"}
	imports := newImports(module, p.Std)
	var fields []string
	for _, f := range o.Fields {
		if seen[f.Name] {
			return nil, Refusef("field `%s` is given twice", f.Name)
		}
		seen[f.Name] = true
		if err := bareType(p, module, f.Type); err != nil {
			return nil, err
		}
		typeImports(imports, f.Type, p)
		fields = append(fields, f.Name+": "+f.Type)
	}
	if len(fields) > 0 {
		d.Params = "{ " + strings.Join(fields, ", ") + " }"
	}
	switch {
	case o.Every != "":
		d.Schedule = " every " + quote(o.Every)
	case o.Cron != "":
		d.Schedule = " cron " + quote(o.Cron)
	}

	m := p.Modules[module]
	if m != nil && declared(m.Syntax, o.Name) {
		return nil, Refusef("`%s` already declares `%s`: a job needs a name of its own", module, o.Name)
	}
	// build is the module's edit with the job and, when test is not nil, its test.
	build := func(imps []string, test *TestData) (Edit, error) {
		if m != nil {
			e := newSrcEdit(m.Source)
			e.appendText("\n" + snippet("job", d))
			if test != nil {
				if err := addTests(e, m, []TestData{*test}); err != nil {
					return Edit{}, err
				}
			}
			addImports(e, p, m.Syntax, imps) // last: it requalifies what was appended (`Table.User.UserId` to `User.UserId`)
			return Edit{Path: m.File, Body: e.result()}, nil
		}
		dd := d
		dd.Imports = imps
		body := snippet("job_module", dd)
		if test != nil {
			body += "\n\n" + snippet("test", *test)
		}
		body, err := canonical(p, FileOf(module), body)
		if err != nil {
			return Edit{}, err
		}
		return Edit{Path: FileOf(module), Create: true, Body: body}, nil
	}

	// the probe: the job declared, to read its parameters' types and its schedule as checked
	probeEdit, err := build(imports.list(), nil)
	if err != nil {
		return nil, err
	}
	probe, err := Probe(p.Dir, []Edit{probeEdit})
	if err != nil {
		return nil, err
	}
	if r := probe.errorsIn(probeEdit.Path); r != nil {
		return nil, r
	}
	var info *check.JobInfo
	for _, j := range probe.Res.Jobs[module] {
		if j.Name == o.Name {
			info = j
		}
	}
	if info == nil {
		return nil, Refusef("internal error: the job %s.%s was not checked", module, o.Name)
	}

	test := TestData{Name: quote(o.Name + " runs")}
	if scheduled {
		test.Name = quote(o.Name + " runs on its schedule")
		if err := scheduledJobTest(&test, info, module+"."+o.Name); err != nil {
			return nil, err
		}
	} else {
		b := newTestBuilder(p, module, imports)
		var args []string
		for _, f := range info.Params.Fields {
			args = append(args, f.Name+": "+b.jobSample(f.Type, f.Name))
		}
		arg := "{}"
		if len(args) > 0 {
			arg = "{ " + strings.Join(args, ", ") + " }"
		}
		test.Lines = append(b.lines, "let job = "+o.Name+" "+arg, "let _ <- Job.enqueue job", "let runs <- Test.work")
		// only this job's runs: another job in the app (a scheduled one, whose tick can come due
		// in the same call as the clock moves) shows in `runs` too
		test.Lines = append(test.Lines, "let own = List.filter (\\r -> r.job == job) runs")
		test.Result = "List.map (\\r -> r.result) own == [Ok ()]"
	}
	edit, err := build(imports.list(), &test)
	if err != nil {
		return nil, err
	}
	return []Edit{edit}, nil
}

// jobSample is a sample parameter of type t: a table's id as a literal of its brand (`User.UserId
// "u1"`: the job does nothing with it, so no row is inserted), else the tests' sample value.
func (b *testBuilder) jobSample(t types.Type, field string) string {
	if br, ok := t.(*types.Brand); ok {
		for _, tbl := range b.p.Tables {
			if types.String(br) == types.String(tbl.IDType) {
				v := "1"
				if k, _, _ := kindOf(br); k == KindText {
					v = quote(strings.ToLower(tbl.Name[:1]) + "1")
				}
				return b.brandCtor(br) + " " + v
			}
		}
	}
	return b.sample(t, field, 1)
}

// scheduledJobTest fills test for a scheduled job: the clock is set a minute before the job's
// first tick after jobTestBase, the first `Test.work` queues the tick (and runs nothing), the
// clock moves a second past the tick, and the second `Test.work` runs the job, once. Both checks
// look at this job's runs only (`r.job == job`): another scheduled job's tick may come due in the
// same call. The ticks are the worker's own: jobs.Next (rt/jobs/schedule.go).
func scheduledJobTest(test *TestData, info *check.JobInfo, qname string) error {
	k := jobs.Kind{Name: qname, Every: info.Every, Cron: info.Cron}
	tick, err := jobs.Next(k, jobTestBase)
	if err != nil {
		return Refusef("%v", err)
	}
	start := tick.Add(-time.Minute)
	if next, err := jobs.Next(k, start); err != nil || !next.Equal(tick) { // an `every` off the day's grid
		start = jobTestBase
		if tick, err = jobs.Next(k, start); err != nil {
			return Refusef("%v", err)
		}
	}
	name := qname[strings.LastIndex(qname, ".")+1:]
	at := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }
	test.Comment = []string{
		fmt.Sprintf("The clock starts at %s; the job's first tick is at %s.", at(start), at(tick)),
		"The first Test.work plays the worker's start: it queues the tick and runs nothing.",
		"A second past the tick, the second Test.work runs the job, once.",
		"Other jobs in the app run in the same calls, so only this job's runs are checked.",
	}
	test.Lines = []string{
		fmt.Sprintf("let _ <- Test.setClock (Time.fromMillis %d)", start.UnixMilli()),
		"let start <- Test.work",
		fmt.Sprintf("let _ <- Test.advance %d", tick.Sub(start).Milliseconds()+1000),
		"let runs <- Test.work",
		fmt.Sprintf("let job = %s {}", name),
		"let ownStart = List.filter (\\r -> r.job == job) start",
		"let own = List.filter (\\r -> r.job == job) runs",
	}
	test.Result = "List.isEmpty ownStart && List.map (\\r -> r.result) own == [Ok ()]"
	return nil
}
