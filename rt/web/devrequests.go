package web

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"alder/rt/applog"
	"alder/rt/eff"
)

// requestRing is how many request records the dev server keeps: the last 200.
const requestRing = 200

// devRequestsPath is where the ring is served under ALDER_DEV=1: the summaries at the path itself,
// one record at devRequestsPath + "/" + its request id.
const devRequestsPath = "/_dev/requests"

// devMapJSON is the dev map as JSON, for the dev server's tools.
const devMapJSON = "/_dev/map.json"

// reqRing is the last requestRing request records, oldest overwritten first, with a lookup by
// request id. It is filled only while the dev panel records requests (rt.panelOn()), so a
// production router's ring stays empty and costs nothing per request.
type reqRing struct {
	mu    sync.Mutex
	recs  [requestRing]*ReqRecord
	next  int            // the slot the next record goes in
	n     int            // records held, up to requestRing
	byRID map[string]int // a request id's slot; made on the first record
}

// add puts rec in the ring, in place of the oldest record once the ring is full. A request id
// seen again (a client may send its own X-Request-Id) looks up the newest record.
func (g *reqRing) add(rec *ReqRecord) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.byRID == nil {
		g.byRID = make(map[string]int, requestRing)
	}
	if old := g.recs[g.next]; old != nil && g.byRID[old.RID] == g.next {
		delete(g.byRID, old.RID)
	}
	g.recs[g.next] = rec
	g.byRID[rec.RID] = g.next
	g.next = (g.next + 1) % requestRing
	g.n = min(g.n+1, requestRing)
}

// get is the record of request rid, if it is still in the ring.
func (g *reqRing) get(rid string) (*ReqRecord, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	i, ok := g.byRID[rid]
	if !ok {
		return nil, false
	}
	return g.recs[i], true
}

// summaries is the ring's records as summaries, newest first.
func (g *reqRing) summaries() []ReqSummary {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]ReqSummary, 0, g.n)
	for k := 1; k <= g.n; k++ {
		out = append(out, g.recs[(g.next-k+requestRing)%requestRing].summary())
	}
	return out
}

// len is the number of records the ring holds.
func (g *reqRing) len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.n
}

// ReqRecord is one request as the dev server serves it at /_dev/requests/{rid}: its access
// record's fields, the steps and statements its Recorder kept (each statement with its N+1
// group's count, as the dev panel marks it), the lines it logged and the N+1 warnings the
// router logged for it.
type ReqRecord struct {
	RID      string    `json:"rid"`
	T        time.Time `json:"t"` // when the request arrived
	Method   string    `json:"method"`
	Path     string    `json:"path"` // masked, as the access line shows it
	Route    string    `json:"route"`
	Status   int       `json:"status"`
	Ms       float64   `json:"ms"`
	Bytes    int64     `json:"bytes"`
	Steps    []ReqStep `json:"steps"`
	SQL      []ReqSQL  `json:"sql"`
	Loads    int       `json:"loads"`
	Cuts     []int     `json:"cuts"`
	Logs     []ReqLog  `json:"logs"`
	Slow     bool      `json:"slow"`
	Warnings []string  `json:"warnings"`
}

// ReqStep is one effect step a request ran.
type ReqStep struct {
	Name string  `json:"name"`
	Ms   float64 `json:"ms"`
	Cap  string  `json:"cap"`
}

// ReqSQL is one statement a request ran, its arguments as the dev panel shows them (fmt.Sprint
// of each). Repeats is the count of the N+1 group the statement belongs to, left out when it
// belongs to none.
type ReqSQL struct {
	Stmt    string   `json:"stmt"`
	Args    []string `json:"args"`
	Ms      float64  `json:"ms"`
	Rows    int64    `json:"rows"`
	Group   int      `json:"group"`
	Repeats int      `json:"repeats,omitempty"`
}

// ReqLog is one line a request logged through its Env.Log.
type ReqLog struct {
	T     time.Time `json:"t"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
}

// ReqSummary is a request as /_dev/requests lists it: its access fields with the number of its
// steps, statements and N+1 groups.
type ReqSummary struct {
	RID     string    `json:"rid"`
	T       time.Time `json:"t"`
	Method  string    `json:"method"`
	Path    string    `json:"path"`
	Route   string    `json:"route"`
	Status  int       `json:"status"`
	Ms      float64   `json:"ms"`
	Bytes   int64     `json:"bytes"`
	Steps   int       `json:"steps"`
	Stmts   int       `json:"stmts"`
	Repeats int       `json:"repeats"`
	Slow    bool      `json:"slow"`
}

// tenthsMs is d in milliseconds rounded to tenths, as the access line shows it, so the two
// records of one request agree.
func tenthsMs(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*10) / 10
}

// summary is rec as the list shows it. Its Repeats is the number of rec's N+1 groups
// (repeatCount): the router warns once per group, so it is the number of rec's warnings.
func (rec *ReqRecord) summary() ReqSummary {
	return ReqSummary{
		RID: rec.RID, T: rec.T, Method: rec.Method, Path: rec.Path, Route: rec.Route, Status: rec.Status,
		Ms: rec.Ms, Bytes: rec.Bytes, Steps: len(rec.Steps), Stmts: len(rec.SQL), Repeats: len(rec.Warnings), Slow: rec.Slow,
	}
}

// devFeed reports whether path is one of the dev server's machine endpoints (the request ring,
// the map as JSON): their requests are never recorded in the ring, and their 2xx access lines
// are quiet.
func devFeed(path string) bool {
	return path == devRequestsPath || strings.HasPrefix(path, devRequestsPath+"/") || path == devMapJSON
}

// recordRequest files the request tw tracked, which took dur, in the ring: only one the dev
// panel recorded (a Recorder, rt.panelOn()) whose access line is not quiet, and never a request
// for a dev endpoint.
func (rt *router) recordRequest(tw *tracker, dur time.Duration) {
	if tw.rq == nil || tw.rq.Env == nil || tw.rq.Env.Rec == nil || !rt.panelOn() {
		return
	}
	r := tw.r
	if quiet(r.URL.Path, tw.status) || devFeed(r.URL.Path) {
		return
	}
	rec := tw.rq.Env.Rec
	acc := applog.Access{Method: tw.loggedMethod(), URL: r.URL, Masker: rt.masker}
	out := &ReqRecord{
		RID: tw.id, T: rt.now().Add(-dur).UTC(), Method: tw.loggedMethod(), Path: acc.MaskedPath(), Route: routeLabel(tw.route, r.URL.Path),
		Status: tw.status, Ms: tenthsMs(dur), Bytes: tw.bytes, Loads: rec.Loads, Cuts: append([]int{}, rec.Cuts...),
		Slow:  rt.opts.SlowAfter > 0 && dur >= rt.opts.SlowAfter,
		Steps: make([]ReqStep, 0, len(rec.Steps)), SQL: make([]ReqSQL, 0, len(rec.SQL)),
		Logs: make([]ReqLog, 0, len(tw.logs)), Warnings: []string{},
	}
	for _, s := range rec.Steps {
		out.Steps = append(out.Steps, ReqStep{Name: s.Name, Ms: s.Ms, Cap: s.Cap})
	}
	reps := tw.segRepeats()
	marks := repeatMarks(reps)
	for i, q := range rec.SQL {
		args := make([]string, len(q.Args))
		for j, a := range q.Args {
			args[j] = fmt.Sprint(a)
		}
		e := ReqSQL{Stmt: q.Stmt, Args: args, Ms: q.Ms, Rows: q.Rows, Group: q.Group}
		if m, ok := marks[segStmt{segOf(tw.hops, i), eff.Shape(q.Stmt)}]; ok {
			e.Repeats = m.Count
		}
		out.SQL = append(out.SQL, e)
	}
	for _, l := range tw.logs {
		out.Logs = append(out.Logs, ReqLog{T: l.t, Level: l.level, Msg: l.msg})
	}
	for _, rs := range reps {
		for _, rp := range rs {
			out.Warnings = append(out.Warnings, rp.String())
		}
	}
	rt.reqs.add(out)
}

// devRequests answers GET /_dev/requests (the ring's summaries, newest first) and
// GET /_dev/requests/{rid} (one record, or 404 with a JSON error once it has left the ring or
// the app restarted). Dev panel only (rt.panelOn()).
func (rt *router) devRequests(w http.ResponseWriter, r *http.Request) {
	var v any
	status := http.StatusOK
	if rid, ok := strings.CutPrefix(r.URL.Path, devRequestsPath+"/"); ok {
		if rec, found := rt.reqs.get(rid); found {
			v = rec
		} else {
			status = http.StatusNotFound
			v = map[string]string{"error": fmt.Sprintf("no request %s in the last %d", rid, requestRing)}
		}
	} else {
		v = rt.reqs.summaries()
	}
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// loggedMethod is the method the request's log lines carry: the one it was routed by (a POST's
// _method), else the one it was sent with.
func (tw *tracker) loggedMethod() string {
	if tw.method != "" {
		return tw.method
	}
	return tw.r.Method
}
