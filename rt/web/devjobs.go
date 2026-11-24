package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"alder/rt/applog"
	"alder/rt/jobs"
)

// devJobsRoot is the dev server's Jobs page (spec §9): the rows of the app's jobs tables. A
// row's page is devJobsRoot + "/" + its id, its actions that + "/retry", "/run" and "/cancel" (POST).
const devJobsRoot = "/_dev/jobs"

// devJobsJSON is the Jobs page's rows as JSON, for the dev server's tools (alder-tui).
const devJobsJSON = "/_dev/jobs.json"

// devJobsPath reports whether path is the Jobs page, its JSON, or under it.
func devJobsPath(path string) bool {
	return path == devJobsRoot || path == devJobsJSON || strings.HasPrefix(path, devJobsRoot+"/")
}

// JobRow is a row of the jobs table as /_dev/jobs.json carries it: its columns, times in
// milliseconds since the epoch as the table keeps them, and the silo it is in ("org/42"; empty
// for the shared store).
type JobRow struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Silo         string  `json:"silo,omitempty"`
	Priority     int     `json:"priority"`
	State        string  `json:"state"`
	Params       string  `json:"params"`
	Key          *string `json:"key,omitempty"`
	RunAt        int64   `json:"runAt"`
	Created      int64   `json:"created"`
	Started      *int64  `json:"started,omitempty"`
	Finished     *int64  `json:"finished,omitempty"`
	Attempts     int     `json:"attempts"`
	Error        *string `json:"error,omitempty"`
	LeaseUntil   *int64  `json:"leaseUntil,omitempty"`
	Worker       string  `json:"worker,omitempty"`
	Tick         bool    `json:"tick,omitempty"` // its kind's queued schedule tick
	ScheduledFor *int64  `json:"scheduledFor,omitempty"`
}

// JobSchedule is a scheduled kind and its queued tick: Next is the tick's due time, nil when none
// is queued (the workers' housekeeping inserts it).
type JobSchedule struct {
	Kind string `json:"kind"`
	Rule string `json:"rule"` // "every 1h", `cron "0 3 * * *"`
	Next *int64 `json:"next"`
	ID   string `json:"id,omitempty"` // the tick's id
}

// JobsView is what /_dev/jobs shows and /_dev/jobs.json encodes (jobs.Panel.List): the queued
// rows by due time, the running, the dead (the last finished first), the last fifty done and the last fifty cancelled, each
// scheduled kind with its next tick, and whether workers run in this process.
type JobsView struct {
	Workers   bool          `json:"workers"`
	Schedules []JobSchedule `json:"schedules"`
	Queued    []JobRow      `json:"queued"`
	Running   []JobRow      `json:"running"`
	Dead      []JobRow      `json:"dead"`
	Done      []JobRow      `json:"done"`
	Cancelled []JobRow      `json:"cancelled"`
	Missed    []string      `json:"missed"` // stores not read, each with its error
}

func jobRow(r jobs.Row) JobRow {
	out := JobRow{ID: r.ID, Kind: r.Kind, State: r.State, Params: maskParams(r.Params), Priority: r.Priority, Key: r.Key, RunAt: r.RunAt, Created: r.Created,
		Started: r.Started, Finished: r.Finished, Attempts: r.Attempts, Error: r.Error, LeaseUntil: r.LeaseUntil,
		Worker: r.Worker, Tick: r.Schedule && r.State == string(jobs.Queued), ScheduledFor: r.ScheduledFor}
	if r.In.Key != "" {
		out.Silo = r.In.Key + "/" + r.In.Value
	}
	return out
}

// maskParams is a job's params JSON with each top-level member whose name says it holds a secret
// (applog.Sensitive: a password, a token, a key…) shown as applog.Mask, as the request panel
// shows such a form field. Params that are not a JSON object are returned as they are.
func maskParams(params string) string {
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(params), &m) != nil || m == nil {
		return params
	}
	mask, _ := json.Marshal(applog.Mask)
	masked := false
	for k := range m {
		if applog.Sensitive(k) {
			m[k], masked = mask, true
		}
	}
	if !masked {
		return params
	}
	b, err := json.Marshal(m)
	if err != nil {
		return params
	}
	return string(b)
}

func jobRows(rs []jobs.Row) []JobRow {
	out := make([]JobRow, 0, len(rs))
	for _, r := range rs {
		out = append(out, jobRow(r))
	}
	return out
}

// jobsView reads the panel's listing.
func (rt *router) jobsView(r *http.Request) (JobsView, error) {
	p := rt.opts.DevJobs
	l, err := p.List(r.Context())
	if err != nil {
		return JobsView{}, err
	}
	v := JobsView{Workers: p.Workers(), Schedules: []JobSchedule{}, Queued: jobRows(l.Queued), Running: jobRows(l.Running),
		Dead: jobRows(l.Dead), Done: jobRows(l.Done), Cancelled: jobRows(l.Cancelled), Missed: append([]string{}, l.Missed...)}
	for _, k := range p.Kinds {
		rule := k.ScheduleText()
		if rule == "" {
			continue
		}
		s := JobSchedule{Kind: k.Name, Rule: rule}
		for _, q := range v.Queued {
			if q.Tick && q.Kind == k.Name && q.Silo == "" { // ticks are only ever in the shared store
				at := q.RunAt
				s.Next, s.ID = &at, q.ID
				break
			}
		}
		v.Schedules = append(v.Schedules, s)
	}
	return v, nil
}

// devJobs serves the Jobs page and its JSON (GET), a row's page (GET /_dev/jobs/{id}) and its
// actions (POST /_dev/jobs/{id}/retry, /run, /cancel: 303 to the row's page). Dev server only (rt.devOn())
// and only for an app with jobs (Options.DevJobs).
func (rt *router) devJobs(w http.ResponseWriter, r *http.Request) {
	get := r.Method == http.MethodGet || r.Method == http.MethodHead
	rest, under := strings.CutPrefix(r.URL.Path, devJobsRoot+"/")
	switch {
	case get && r.URL.Path == devJobsJSON:
		v, err := rt.jobsView(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b, err := json.Marshal(v)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(append(b, '\n'))
	case get && r.URL.Path == devJobsRoot:
		v, err := rt.jobsView(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeDevPage(w, devJobsPage, v)
	case get && under && rest != "" && !strings.Contains(rest, "/"):
		rt.devJob(w, r, rest)
	case r.Method == http.MethodPost && under:
		rt.devJobAction(w, r, rest)
	default:
		http.NotFound(w, r)
	}
}

// devJob serves job id's page: its row, params and error; Retry (dead or cancelled), Run now (queued) or Cancel (queued, not a tick, or running); and
// the last attempt this process ran of it, with its trace, marked when it held the write lock
// above applog.LockMark (spec §6a).
func (rt *router) devJob(w http.ResponseWriter, r *http.Request, id string) {
	p := rt.opts.DevJobs
	row, err := p.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if row == nil {
		http.Error(w, "no job "+id+" in the stores this process sees (never enqueued there, or pruned)", http.StatusNotFound)
		return
	}
	v := devJobView{Row: jobRow(*row), Workers: p.Workers(), DeadTick: (row.State == string(jobs.Dead) || row.State == string(jobs.Cancelled)) && row.ScheduledFor != nil, Ring: jobs.AttemptRing}
	for _, k := range p.Kinds {
		if k.Name == row.Kind {
			v.Known, v.Max = true, k.Retries+1
		}
	}
	if a, ok := p.Last(id); ok {
		la := &devAttempt{Line: a.Line().Text(), Marked: a.LockHeld > applog.LockMark, Start: a.Start.UnixMilli(),
			Steps: make([]ReqStep, 0, len(a.Steps)), SQL: make([]ReqSQL, 0, len(a.SQL))}
		for _, s := range a.Steps {
			la.Steps = append(la.Steps, ReqStep{Name: s.Name, Ms: s.Ms, Cap: s.Cap})
		}
		for _, q := range a.SQL {
			args := make([]string, len(q.Args))
			for j, x := range q.Args {
				args[j] = fmt.Sprint(x)
			}
			la.SQL = append(la.SQL, ReqSQL{Stmt: q.Stmt, Args: args, Ms: q.Ms, Rows: q.Rows, Group: q.Group})
		}
		v.Last = la
	}
	writeDevPage(w, devJobPage, v)
}

// devJobAction runs POST /_dev/jobs/{id}/retry, /run or /cancel (jobs.Panel.Retry, RunNow, Cancel) and answers
// 303 to the row's page; no such row is 404, a row in another state, a tick to cancel or a retry whose key is held 409. A cross-origin POST
// (another site's form) is refused with 403.
func (rt *router) devJobAction(w http.ResponseWriter, r *http.Request, rest string) {
	if err := http.NewCrossOriginProtection().Check(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	id, action, _ := strings.Cut(rest, "/")
	p := rt.opts.DevJobs
	var err error
	switch action {
	case "retry":
		err = p.Retry(r.Context(), id)
	case "run":
		err = p.RunNow(r.Context(), id)
	case "cancel":
		err = p.Cancel(r.Context(), id)
	default:
		http.NotFound(w, r)
		return
	}
	var (
		se *jobs.StateError
		ke *jobs.KeyHeldError
		te *jobs.TickError
	)
	switch {
	case errors.Is(err, jobs.ErrNoJob):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.As(err, &se), errors.As(err, &ke), errors.As(err, &te):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		http.Redirect(w, r, devJobsRoot+"/"+id, http.StatusSeeOther)
	}
}

// writeDevPage writes page t with data v, or a 500 with the template's error.
func writeDevPage(w http.ResponseWriter, t *template.Template, v any) {
	var b bytes.Buffer
	if err := t.Execute(&b, v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b.Bytes())
}

// devJobView is a row's page: the row, its kind's attempts at most (Max, when this build knows
// the kind) and the last attempt this process ran of it.
type devJobView struct {
	Row      JobRow
	Known    bool
	Max      int
	Workers  bool
	DeadTick bool // a dead schedule tick: its retry runs once and inserts no next tick
	Last     *devAttempt
	Ring     int // jobs.AttemptRing
}

// devAttempt is the last attempt of a job: its log line (which carries the write-lock mark),
// whether it is marked, when it started, and its trace.
type devAttempt struct {
	Line   string
	Marked bool
	Start  int64
	Steps  []ReqStep
	SQL    []ReqSQL
}

// devJobsFuncs format the pages' times: milliseconds since the epoch as UTC.
var devJobsFuncs = template.FuncMap{
	"ms": func(v any) string {
		switch t := v.(type) {
		case int64:
			return time.UnixMilli(t).UTC().Format("2006-01-02 15:04:05 UTC")
		case *int64:
			if t != nil {
				return time.UnixMilli(*t).UTC().Format("2006-01-02 15:04:05 UTC")
			}
		}
		return "—"
	},
}

const devJobsStyle = `<style>body{font:14px/1.4 system-ui,sans-serif;margin:1rem}table{border-collapse:collapse}th,td{text-align:start;padding:.15rem .6rem .15rem 0;vertical-align:top}pre{white-space:pre-wrap}[data-lock]{color:#b45309;font-weight:600}</style>`

// devJobsPage is /_dev/jobs.
var devJobsPage = template.Must(template.New("jobs").Funcs(devJobsFuncs).Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Alder dev jobs</title>` + devJobsStyle + `</head><body>
<h1>Jobs</h1>
<p><a href="/_dev">dev map</a> · <a href="/_dev/jobs.json">as JSON</a></p>
{{if not .Workers}}<p>No job worker runs in this process (ALDER_JOBS=0, or the workers stopped): a queued job waits for another process's.</p>{{end}}
{{range .Missed}}<p>Not read: {{.}}</p>{{end}}
<h2>Schedules</h2>
<ul>{{range .Schedules}}<li><code>{{.Kind}}</code> {{.Rule}}: next {{if .Next}}<a href="/_dev/jobs/{{.ID}}">{{ms .Next}}</a>{{else}}none queued yet{{end}}</li>{{else}}<li>none</li>{{end}}</ul>
<h2>Queued ({{len .Queued}})</h2>
<table><tr><th>id</th><th>kind</th><th>due</th><th>attempts</th><th>last error</th></tr>
{{range .Queued}}<tr><td><a href="/_dev/jobs/{{.ID}}">{{.ID}}</a>{{if .Tick}} <small>tick</small>{{end}}</td><td><code>{{.Kind}}</code>{{if .Priority}} <small>priority {{.Priority}}</small>{{end}}{{if .Silo}} <small>silo {{.Silo}}</small>{{end}}</td><td>{{ms .RunAt}}</td><td>{{.Attempts}}</td><td>{{with .Error}}{{.}}{{end}}</td></tr>
{{end}}</table>
<h2>Running ({{len .Running}})</h2>
<table><tr><th>id</th><th>kind</th><th>started</th><th>attempt</th><th>worker</th></tr>
{{range .Running}}<tr><td><a href="/_dev/jobs/{{.ID}}">{{.ID}}</a></td><td><code>{{.Kind}}</code>{{if .Silo}} <small>silo {{.Silo}}</small>{{end}}</td><td>{{ms .Started}}</td><td>{{.Attempts}}</td><td>{{.Worker}}</td></tr>
{{end}}</table>
<h2>Dead ({{len .Dead}})</h2>
<table><tr><th>id</th><th>kind</th><th>finished</th><th>attempts</th><th>error</th></tr>
{{range .Dead}}<tr><td><a href="/_dev/jobs/{{.ID}}">{{.ID}}</a></td><td><code>{{.Kind}}</code>{{if .Silo}} <small>silo {{.Silo}}</small>{{end}}</td><td>{{ms .Finished}}</td><td>{{.Attempts}}</td><td>{{with .Error}}{{.}}{{end}}</td></tr>
{{end}}</table>
<h2>Cancelled (last 50)</h2>
<table><tr><th>id</th><th>kind</th><th>finished</th><th>attempts</th></tr>
{{range .Cancelled}}<tr><td><a href="/_dev/jobs/{{.ID}}">{{.ID}}</a></td><td><code>{{.Kind}}</code>{{if .Silo}} <small>silo {{.Silo}}</small>{{end}}</td><td>{{ms .Finished}}</td><td>{{.Attempts}}</td></tr>
{{end}}</table>
<h2>Done (last 50)</h2>
<table><tr><th>id</th><th>kind</th><th>finished</th><th>attempts</th></tr>
{{range .Done}}<tr><td><a href="/_dev/jobs/{{.ID}}">{{.ID}}</a></td><td><code>{{.Kind}}</code>{{if .Silo}} <small>silo {{.Silo}}</small>{{end}}</td><td>{{ms .Finished}}</td><td>{{.Attempts}}</td></tr>
{{end}}</table>
</body></html>`))

// devJobPage is /_dev/jobs/{id}.
var devJobPage = template.Must(template.New("job").Funcs(devJobsFuncs).Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Alder dev job {{.Row.ID}}</title>` + devJobsStyle + `</head><body>
<h1>Job {{.Row.ID}}</h1>
<p><a href="/_dev/jobs">all jobs</a></p>
<ul>
<li>kind <code>{{.Row.Kind}}</code>{{if not .Known}} (not a kind this build knows: no worker of it claims the row){{end}}</li>
{{if .Row.Silo}}<li>silo {{.Row.Silo}}</li>{{end}}
<li>state {{.Row.State}}</li>
<li>attempts {{.Row.Attempts}}{{if .Known}} of {{.Max}}{{end}}</li>
{{if eq .Row.State "queued"}}<li>due {{ms .Row.RunAt}}</li>{{end}}
<li>created {{ms .Row.Created}}</li>
{{if .Row.Started}}<li>started {{ms .Row.Started}}</li>{{end}}
{{if .Row.Finished}}<li>finished {{ms .Row.Finished}}</li>{{end}}
{{if .Row.LeaseUntil}}<li>leased until {{ms .Row.LeaseUntil}}</li>{{end}}
{{with .Row.Worker}}<li>worker {{.}}</li>{{end}}
{{with .Row.Key}}<li>key <code>{{.}}</code></li>{{end}}
{{if .Row.ScheduledFor}}<li>schedule tick for {{ms .Row.ScheduledFor}}</li>{{end}}
</ul>
<h2>Params</h2>
<pre>{{.Row.Params}}</pre>
{{with .Row.Error}}<h2>Error</h2>
<pre>{{.}}</pre>{{end}}
{{if or (eq .Row.State "dead") (eq .Row.State "cancelled")}}<form method="post" action="/_dev/jobs/{{.Row.ID}}/retry"><button>Retry</button> queues it again, due now, with attempts 0.{{if .DeadTick}} A retried tick runs once, now, and inserts no next tick: its schedule's next tick is a row of its own.{{end}}</form>{{end}}
{{if eq .Row.State "queued"}}<form method="post" action="/_dev/jobs/{{.Row.ID}}/run"><button>Run now</button> makes it due now.{{if .Row.Tick}} This tick runs in place of its slot: the next tick is the first after the later of {{ms .Row.ScheduledFor}} and the time it is claimed.{{end}}{{if not .Workers}} No job worker runs in this process to claim it.{{end}}</form>{{end}}
{{if or (eq .Row.State "running") (and (eq .Row.State "queued") (not .Row.Tick))}}<form method="post" action="/_dev/jobs/{{.Row.ID}}/cancel"><button>Cancel</button> a queued job never runs; a running job is stopped at its next step in this process, within a second in another, and its writes are undone.</form>{{end}}
<h2>Last attempt</h2>
{{with .Last}}<p{{if .Marked}} data-lock{{end}}><code>{{.Line}}</code></p>
<p>started {{ms .Start}}</p>
<h3>Trace</h3>
<ol>{{range .Steps}}<li><code>{{.Name}}</code>{{if .Cap}} <small>{{.Cap}}</small>{{end}} {{printf "%.1f" .Ms}} ms</li>{{else}}<li>no step</li>{{end}}</ol>
<h3>SQL</h3>
<ol>{{range .SQL}}<li><code>{{.Stmt}}</code>{{if .Args}} <small>{{range $i, $a := .Args}}{{if $i}}, {{end}}{{$a}}{{end}}</small>{{end}} {{printf "%.1f" .Ms}} ms{{if ge .Rows 0}}, {{.Rows}} rows{{end}}</li>{{else}}<li>no statement</li>{{end}}</ol>
{{else}}<p>No attempt of it among the last {{.Ring}} jobs this process ran (the record is kept in memory, not across a restart).</p>{{end}}
</body></html>`))
