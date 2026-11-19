// Job declarations (#168): `job NAME = { params } [every "…"] [cron "…"] [retries N]
// [timeout "…"] [silo k]` and the arm `NAME.run : NAME.Params -> Task Unit` (E0043). Params that
// cannot be stored are E0049; the options' rules are E0050 (a cron expression is read by rt/jobs/cron).
package check

import (
	"slices"
	"strings"
	"time"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
	"alder/rt/jobs/cron"
)

// Defaults and limits of a job's options (spec §1).
const (
	jobRetries     = 4
	jobTimeout     = 5 * time.Minute
	jobTimeoutMax  = time.Hour
	jobEveryMin    = time.Second
	jobDurationEnd = "write a whole number and a unit, `ms`, `s`, `m`, `h` or `d` (\"30s\", \"1h\")"
)

// JobInfo is a job's generated `Params` and its options, with the defaults filled.
type JobInfo struct {
	Module, Name string
	Decl         *syntax.JobDecl
	Params       *types.Record // `Name.Params` (Module "Signup", Name "SendWelcome.Params")
	Retries      int
	Timeout      time.Duration
	Every        time.Duration // 0 without `every`
	Cron         string        // "" without `cron`; read in rt/jobs (Task 11)
	Silo         string        // the `silo k` key when it is valid, else ""
}

// job is job name of module, nil when there is none.
func (c *Checker) job(module, name string) *JobInfo {
	for _, j := range c.jobs[module] {
		if j.Name == name {
			return j
		}
	}
	return nil
}

// defineJobs defines the module's jobs: each one's `Name.Params`, with its fields checked for
// storage (E0049), and its options (E0050, and the `silo k` rule of the admin form).
func (mc *modCtx) defineJobs() {
	c := mc.c
	for _, d := range c.live(mc.m) {
		jd, ok := d.(*syntax.JobDecl)
		if !ok || c.prog.Job(mc.m.Name, jd.Name) != jd {
			continue
		}
		info := &JobInfo{Module: mc.m.Name, Name: jd.Name, Decl: jd, Retries: jobRetries, Timeout: jobTimeout,
			Params: &types.Record{Module: mc.m.Name, Name: jd.Name + ".Params"}}
		if jd.Params != nil {
			for _, f := range jd.Params.Fields {
				ft := mc.typeOf(f.Type, nil)
				if bad := jobUnstorable(ft, map[string]bool{}); bad != nil {
					what := "`" + mc.ts(ft) + "`"
					if bad != ft {
						what += ", which holds `" + mc.ts(bad) + "`"
					}
					c.errorf(diag.JobParam, f.Span, "A job parameter that cannot be stored: `%s` of job `%s` is %s. A job's parameters are stored as JSON: use table-column types, records, lists, `Maybe` and the app's own types without functions", f.Name, jd.Name, what)
				}
				info.Params.Fields = append(info.Params.Fields, types.Field{Name: f.Name, Type: ft})
			}
		}
		mc.jobOptions(jd, info)
		c.types[c.key(mc.m.Name, info.Params.Name)] = info.Params
		c.jobs[mc.m.Name] = append(c.jobs[mc.m.Name], info)
	}
}

// jobUnstorable is the part of t a job parameter cannot store (t itself, or a type inside it),
// nil when all of it can be: a table column's type, a record, `List` or `Maybe` of storable
// types, or a union whose constructors' arguments are storable. seen stops on recursive types.
func jobUnstorable(t types.Type, seen map[string]bool) types.Type {
	if isScalar(t) {
		return nil
	}
	switch t := t.(type) {
	case *types.App:
		if t.Con == "List" && len(t.Args) == 1 {
			return jobUnstorable(t.Args[0], seen)
		}
	case *types.Record:
		k := t.Module + "." + t.Name
		if !t.Anonymous {
			if seen[k] {
				return nil
			}
			seen[k] = true
		}
		for _, f := range t.Fields {
			if bad := jobUnstorable(f.Type, seen); bad != nil {
				return bad
			}
		}
		return nil
	case *types.Union:
		if inner, ok := maybeOf(t); ok {
			return jobUnstorable(inner, seen)
		}
		k := types.String(t) // with its arguments: `Result Text Int` and `Result (Int -> Int) Int` differ
		if seen[k] {
			return nil
		}
		seen[k] = true
		for _, ct := range t.Constructors() {
			arg, _ := types.CtorArg(t, ct.Name) // at t's arguments (`Result Text Int`)
			if arg == nil {
				continue
			}
			if bad := jobUnstorable(arg, seen); bad != nil {
				if bad == arg {
					return t // the app's type is what the field names
				}
				return bad
			}
		}
		return nil
	}
	return t
}

// jobOptions checks jd's options into info: one schedule, none with parameters, `every` at
// least 1s and `timeout` at most 1h (E0050), and `silo k` (E0033).
func (mc *modCtx) jobOptions(jd *syntax.JobDecl, info *JobInfo) {
	c := mc.c
	if jd.Every != nil && jd.Cron != nil {
		c.errorf(diag.JobSchedule, jd.Span, "A job has two schedules: job `%s` has both `every` and `cron`; keep one", jd.Name)
	} else if (jd.Every != nil || jd.Cron != nil) && len(info.Params.Fields) > 0 {
		c.errorf(diag.JobSchedule, jd.Params.Span, "A scheduled job takes no parameters: job `%s` runs on its schedule, with nothing to fill them; write `job %s = {}`", jd.Name, jd.Name)
	}
	if jd.Every != nil {
		if d, ok := jobDuration(jd.Every.Text); !ok {
			c.errorf(diag.JobSchedule, jd.Every.Span, "`every \"%s\"`: cannot read `%s` as a duration; %s", jd.Every.Text, jd.Every.Text, jobDurationEnd)
		} else if d < jobEveryMin {
			c.errorf(diag.JobSchedule, jd.Every.Span, "`every \"%s\"` is shorter than the minimum, `1s`", jd.Every.Text)
		} else {
			info.Every = d
		}
	}
	if jd.Cron != nil {
		if _, err := cron.Parse(jd.Cron.Text); err != nil {
			c.errorf(diag.JobSchedule, jd.Cron.Span, "`cron \"%s\"`: %v; write five fields, `minute hour day-of-month month day-of-week`, as in `0 3 * * *`", jd.Cron.Text, err)
		} else {
			info.Cron = jd.Cron.Text
		}
	}
	if jd.Timeout != nil {
		if d, ok := jobDuration(jd.Timeout.Text); !ok {
			c.errorf(diag.JobSchedule, jd.Timeout.Span, "`timeout \"%s\"`: cannot read `%s` as a duration; %s", jd.Timeout.Text, jd.Timeout.Text, jobDurationEnd)
		} else if d > jobTimeoutMax {
			c.errorf(diag.JobSchedule, jd.Timeout.Span, "`timeout \"%s\"` is longer than the maximum, `1h`: one attempt may take at most an hour", jd.Timeout.Text)
		} else {
			info.Timeout = d
		}
	}
	if jd.Retries != nil { // the parser takes a whole number only: `retries -1` is E0001
		info.Retries = int(jd.Retries.Value)
	}
	if jd.Silo != "" {
		info.Silo = mc.jobSilo(jd, info.Params)
	}
}

// jobSilo checks `silo k` of job jd with the admin form's wording: the session has a `Maybe`
// field k, App lists tables in `silo k`, and the job has a parameter k of the key's type. It
// returns k when it is valid.
func (mc *modCtx) jobSilo(jd *syntax.JobDecl, params *types.Record) string {
	c := mc.c
	k, at := jd.Silo, jd.Span
	for _, o := range jd.Options {
		if o.Name == "silo" {
			at = diag.Span{File: jd.Span.File, Start: o.Pos, End: diag.Pos{Line: o.Pos.Line, Col: o.Pos.Col + len("silo")}}
		}
	}
	kt, has := c.sessionKey(k)
	_, isMaybe := c.fieldMaybe(k)
	var param *types.Field
	for i := range params.Fields {
		if params.Fields[i].Name == k {
			param = &params.Fields[i]
		}
	}
	switch {
	case !has:
		c.errorf(diag.SiloDecl, at, "`silo %s`: the session has no field `%s`", k, k)
	case !isMaybe:
		c.errorf(diag.SiloDecl, at, "`silo %s`: session field `%s` must be a `Maybe`", k, k)
	case len(c.silos[k]) == 0:
		c.errorf(diag.SiloDecl, at, "`silo %s`: the app has no `silo %s { … }` in App, so there is no silo for the job to run in; remove `silo %s`", k, k, k)
	case param == nil:
		c.errorf(diag.SiloDecl, at, "`silo %s` takes `%s` from the job's parameter: add `%s: %s` to the job's parameters", k, k, k, mc.ts(kt))
	case types.IsErr(param.Type):
	case c.u.Unify(param.Type, kt) != nil:
		c.errorf(diag.SiloDecl, at, "the job's parameter `%s` is `%s`; `silo %s` needs `%s`, the type of the session's `%s`", k, mc.ts(param.Type), k, mc.ts(kt), k)
	default:
		return k
	}
	return ""
}

// jobDuration reads a job option's duration ("500ms", "30s", "5m", "1h", "1d") with the grammar
// of the polling attribute (syntax.EveryDuration).
func jobDuration(lit string) (time.Duration, bool) {
	s, ok := syntax.EveryDuration(lit)
	if !ok {
		return 0, false
	}
	d, err := time.ParseDuration(s)
	return d, err == nil
}

// checkJobs checks that every job of the module has its arm, `Name.run : Name.Params -> Task
// Unit` (E0043, worded for a job).
func (mc *modCtx) checkJobs() {
	c := mc.c
	for _, j := range c.jobs[mc.m.Name] {
		sig := j.Name + ".Params -> Task Unit"
		var run *syntax.ValueDecl
		for _, d := range c.live(mc.m) {
			if vd, ok := d.(*syntax.ValueDecl); ok && vd.Route == j.Name {
				if vd.Name == "run" {
					run = vd
				} else {
					c.errorf(diag.RouteDecl, vd.Span, "job `%s` has no `%s`: a job has only `%s.run : %s`", j.Name, vd.Name, j.Name, sig)
				}
			}
		}
		if run == nil {
			c.errorf(diag.RouteDecl, j.Decl.Span, "job `%s` needs its `run`: `%s.run : %s`", j.Name, j.Name, sig)
			continue
		}
		want := &types.Fun{Params: []types.Type{j.Params}, Result: &types.App{Con: "Task", Args: []types.Type{types.Unit}}}
		mc.routeSig(run, want, sig)
	}
}

// armJob is the job whose `run` mc is checking, nil outside one.
func (mc *modCtx) armJob() *JobInfo {
	name, member, ok := strings.Cut(mc.value, ".")
	if !ok || member != "run" || mc.inTest || mc.fake != "" {
		return nil
	}
	return mc.c.job(mc.m.Name, name)
}

// jobRefusal is E0026's message for subject used in job j's `run` where it needs the session's
// key: a job has no session, so only its own `silo k` is narrowed.
func jobRefusal(j *JobInfo, subject, key string) string {
	switch {
	case key == "user":
		return subject + " cannot be used in a job's `run`: a job has no session; give the job the user as a parameter and use the unscoped operations"
	case j.Silo != "":
		return subject + " needs `silo " + key + "`: job `" + j.Name + "` runs in `silo " + j.Silo + "`"
	}
	return subject + " needs a job with `silo " + key + "`: a job without `silo " + key + "` runs on the shared store"
}

// checkJobSessions refuses `Session.put` reached from a job's `run` through the top-level values
// it refers to (a job has no session; written in `run` itself it is refused where it is checked).
func (c *Checker) checkJobSessions() {
	puts := map[string]bool{} // values whose body names Session.put
	refs := map[string]map[string]bool{}
	for _, name := range c.prog.Order {
		for _, d := range c.live(c.prog.Modules[name]) {
			vd, ok := d.(*syntax.ValueDecl)
			if !ok || vd.Body == nil {
				continue
			}
			k := c.key(name, vd.FullName())
			refs[k] = c.bodyFacts(vd.Body).refs
			walkExpr(vd.Body, func(x syntax.Expr) {
				if v, ok := x.(*syntax.Var); ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Session" && v.Ref.Name == "put" {
					puts[k] = true
				}
			})
		}
	}
	for changed := true; changed; {
		changed = false
		for k, rs := range refs {
			for r := range rs {
				if puts[r] && !puts[k] {
					puts[k], changed = true, true
				}
			}
		}
	}
	for _, name := range c.prog.Order {
		if c.prog.Failed[name] {
			continue
		}
		for _, j := range c.jobs[name] {
			run := c.routeMembers(c.prog.Modules[name], j.Name)["run"]
			if run == nil || run.Body == nil {
				continue
			}
			var via []string
			for r := range refs[c.key(name, run.FullName())] {
				if puts[r] {
					via = append(via, r)
				}
			}
			if len(via) > 0 {
				slices.Sort(via)
				c.errorf(diag.TypeMismatch, run.Span, "`%s` reaches `Session.put` through `%s`: a job has no session to put", run.FullName(), via[0])
			}
		}
	}
}
