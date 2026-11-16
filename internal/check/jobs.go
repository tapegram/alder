// Job declarations (#168): `job NAME = { params } [every "…"] [cron "…"] [retries N]
// [timeout "…"] [silo k]` and the arm `NAME.run : NAME.Params -> Task Unit` (E0043). Params that
// cannot be stored are E0049; the options' rules are E0050 (cron expressions are read in rt/jobs).
package check

import (
	"strings"
	"time"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
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
		k := t.Module + "." + t.Name
		if seen[k] {
			return nil
		}
		seen[k] = true
		for _, ct := range t.Constructors() {
			if ct.Arg == nil {
				continue
			}
			if bad := jobUnstorable(ct.Arg, seen); bad != nil {
				if bad == ct.Arg {
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
	if jd.Every != nil && jd.Every.Text != "" {
		if d, ok := jobDuration(jd.Every.Text); !ok {
			c.errorf(diag.JobSchedule, jd.Every.Span, "`every \"%s\"`: cannot read `%s` as a duration; %s", jd.Every.Text, jd.Every.Text, jobDurationEnd)
		} else if d < jobEveryMin {
			c.errorf(diag.JobSchedule, jd.Every.Span, "`every \"%s\"` is shorter than the minimum, `1s`", jd.Every.Text)
		} else {
			info.Every = d
		}
	}
	if jd.Cron != nil {
		info.Cron = jd.Cron.Text
	}
	if jd.Timeout != nil && jd.Timeout.Text != "" {
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
			at = diag.Span{File: jd.Span.File, Start: o.Pos, End: o.Pos}
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

// jobArm reports whether mc is checking a job's `run`.
func (mc *modCtx) jobArm() bool {
	name, member, ok := strings.Cut(mc.value, ".")
	return ok && member == "run" && mc.c.job(mc.m.Name, name) != nil
}
