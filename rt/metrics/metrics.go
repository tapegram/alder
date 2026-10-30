// Package metrics is a served app's counters and timings, written as Prometheus text
// (`text/plain; version=0.0.4`) at /_alder/metrics when ALDER_METRICS turns it on (FromEnv):
// requests by route pattern, method and status class, their latency, the requests in flight,
// database statements, effect steps, Go runtime basics and the build id.
//
// Every label value comes from a finite set the program fixes: the route is the matched
// pattern or one of a few fixed names (the router's), the method one of four, the status one of
// five classes, the effect a Call name of the generated code. No series is made from a path,
// query, header, parameter or session value.
package metrics

import (
	"fmt"
	"io"
	"runtime/metrics"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"alder/rt/eff"
)

// buckets are the latency histogram's upper bounds, in seconds.
var buckets = [...]float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// methods are the method label's values: GET, POST, HEAD and every other method as "other".
var methods = [...]string{"GET", "POST", "HEAD", "other"}

// classes are the status label's values.
var classes = [...]string{"1xx", "2xx", "3xx", "4xx", "5xx"}

// MaxSteps bounds the effect-name series: a name past this many distinct ones is counted as
// "(other)". The names are the generated code's Call names, so an app never reaches it; it is
// a guard, not a limit an app is expected to meet.
const MaxSteps = 1000

type reqKey struct {
	route  string
	method uint8
	class  uint8
}

type histKey struct {
	route  string
	method uint8
}

type histogram struct {
	counts [len(buckets)]uint64 // not cumulative: the count in each bucket's own range
	inf    uint64               // above the last bucket
	sum    float64              // seconds
}

// Collector holds the counts. It is safe for concurrent use: one mutex guards everything.
type Collector struct {
	auth  authorizer
	start time.Time

	mu       sync.Mutex
	inFlight int64
	reqs     map[reqKey]uint64
	hist     map[histKey]*histogram
	steps    map[string]uint64
	stmts    [2]uint64        // exec, query
	stmtDur  [2]time.Duration // exec, query
}

// New is a Collector answering a scrape that carries token (Authorization: Bearer <token>), or
// every scrape when token is "" (open). The process start time is now.
func New(token string) *Collector {
	return &Collector{
		auth:  newAuthorizer(token),
		start: time.Now(),
		reqs:  map[reqKey]uint64{},
		hist:  map[histKey]*histogram{},
		steps: map[string]uint64{},
	}
}

// InFlight adds delta to the requests in flight.
func (c *Collector) InFlight(delta int64) {
	c.mu.Lock()
	c.inFlight += delta
	c.mu.Unlock()
}

// Request counts one request answered: route (the router's label: a route pattern or a fixed
// name), method (anything but GET, POST and HEAD is "other"), status (its class; 0, nothing
// written, is 2xx) and how long it took.
func (c *Collector) Request(route, method string, status int, d time.Duration) {
	m := methodIndex(method)
	k := reqKey{route: route, method: m, class: classIndex(status)}
	sec := d.Seconds()
	c.mu.Lock()
	c.reqs[k]++
	h := c.hist[histKey{route: route, method: m}]
	if h == nil {
		h = &histogram{}
		c.hist[histKey{route: route, method: m}] = h
	}
	if i, ok := bucketOf(sec); ok {
		h.counts[i]++
	} else {
		h.inf++
	}
	h.sum += sec
	c.mu.Unlock()
}

// Steps counts one effect step per name (a request's Env.Trace). A delay's milliseconds
// ("Task.sleep 250ms", which the app may compute) are not part of the name counted.
func (c *Collector) Steps(names []string) {
	if len(names) == 0 {
		return
	}
	c.mu.Lock()
	for _, n := range names {
		n = stepName(n)
		if _, ok := c.steps[n]; !ok && len(c.steps) >= MaxSteps {
			n = "(other)"
		}
		c.steps[n]++
	}
	c.mu.Unlock()
}

// Statements adds a request's database statements, counted and timed by rt/db.
func (c *Collector) Statements(s *eff.Stats) {
	if s == nil || s.Exec == 0 && s.Query == 0 {
		return
	}
	c.mu.Lock()
	c.stmts[0] += uint64(s.Exec)
	c.stmts[1] += uint64(s.Query)
	c.stmtDur[0] += s.ExecDur
	c.stmtDur[1] += s.QueryDur
	c.mu.Unlock()
}

func methodIndex(m string) uint8 {
	switch m {
	case "GET":
		return 0
	case "POST":
		return 1
	case "HEAD":
		return 2
	}
	return 3
}

func classIndex(status int) uint8 {
	if status < 100 || status > 599 {
		return 1 // 0: nothing written, a 200
	}
	return uint8(status/100 - 1)
}

func bucketOf(sec float64) (int, bool) {
	for i, b := range buckets {
		if sec <= b {
			return i, true
		}
	}
	return 0, false
}

// stepName is a trace entry as the effect label counts it: a delay's " <n>ms" dropped.
func stepName(n string) string {
	if i := strings.LastIndexByte(n, ' '); i > 0 && strings.HasSuffix(n, "ms") {
		digits := n[i+1 : len(n)-2]
		if digits != "" && strings.Trim(digits, "0123456789") == "" {
			return n[:i]
		}
	}
	return n
}

// ContentType is the exposition's media type.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

// Write writes the exposition: each metric's HELP and TYPE, the metrics in a fixed order, the
// series sorted by their labels. build is the build_info label (the build id).
func (c *Collector) Write(w io.Writer, build string) error {
	var b strings.Builder
	c.mu.Lock()
	c.write(&b, build)
	c.mu.Unlock()
	_, err := io.WriteString(w, b.String())
	return err
}

func (c *Collector) write(b *strings.Builder, build string) {
	head := func(name, typ, help string) {
		fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
	}

	head("alder_http_requests_total", "counter", "Requests answered, by route pattern, method and status class.")
	keys := make([]reqKey, 0, len(c.reqs))
	for k := range c.reqs {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b reqKey) int {
		if c := strings.Compare(a.route, b.route); c != 0 {
			return c
		}
		if c := strings.Compare(methods[a.method], methods[b.method]); c != 0 {
			return c
		}
		return int(a.class) - int(b.class)
	})
	for _, k := range keys {
		fmt.Fprintf(b, "alder_http_requests_total{route=\"%s\",method=\"%s\",status=\"%s\"} %d\n",
			escape(k.route), methods[k.method], classes[k.class], c.reqs[k])
	}

	head("alder_http_request_duration_seconds", "histogram", "Request latency, by route pattern and method.")
	hkeys := make([]histKey, 0, len(c.hist))
	for k := range c.hist {
		hkeys = append(hkeys, k)
	}
	slices.SortFunc(hkeys, func(a, b histKey) int {
		if c := strings.Compare(a.route, b.route); c != 0 {
			return c
		}
		return strings.Compare(methods[a.method], methods[b.method])
	})
	for _, k := range hkeys {
		h := c.hist[k]
		labels := "route=\"" + escape(k.route) + "\",method=\"" + methods[k.method] + "\""
		var cum uint64
		for i, le := range buckets {
			cum += h.counts[i]
			fmt.Fprintf(b, "alder_http_request_duration_seconds_bucket{%s,le=\"%s\"} %d\n", labels, num(le), cum)
		}
		cum += h.inf
		fmt.Fprintf(b, "alder_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, cum)
		fmt.Fprintf(b, "alder_http_request_duration_seconds_sum{%s} %s\n", labels, num(h.sum))
		fmt.Fprintf(b, "alder_http_request_duration_seconds_count{%s} %d\n", labels, cum)
	}

	head("alder_http_requests_in_flight", "gauge", "Requests being answered now.")
	fmt.Fprintf(b, "alder_http_requests_in_flight %d\n", c.inFlight)

	head("alder_db_statements_total", "counter", "Database statements run by requests, by kind (exec or query).")
	fmt.Fprintf(b, "alder_db_statements_total{kind=\"exec\"} %d\nalder_db_statements_total{kind=\"query\"} %d\n", c.stmts[0], c.stmts[1])
	head("alder_db_statement_seconds_total", "counter", "Time spent in database statements run by requests, by kind.")
	fmt.Fprintf(b, "alder_db_statement_seconds_total{kind=\"exec\"} %s\nalder_db_statement_seconds_total{kind=\"query\"} %s\n",
		num(c.stmtDur[0].Seconds()), num(c.stmtDur[1].Seconds()))

	head("alder_effect_steps_total", "counter", "Effect steps run by requests, by Call name.")
	names := make([]string, 0, len(c.steps))
	for n := range c.steps {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		fmt.Fprintf(b, "alder_effect_steps_total{name=\"%s\"} %d\n", escape(n), c.steps[n])
	}

	rt := []metrics.Sample{{Name: "/sched/goroutines:goroutines"}, {Name: "/memory/classes/heap/objects:bytes"}, {Name: "/gc/cycles/total:gc-cycles"}}
	metrics.Read(rt)
	head("alder_go_goroutines", "gauge", "Goroutines that currently exist.")
	fmt.Fprintf(b, "alder_go_goroutines %d\n", sampleUint(rt[0]))
	head("alder_go_heap_bytes", "gauge", "Bytes of live and not yet swept heap objects.")
	fmt.Fprintf(b, "alder_go_heap_bytes %d\n", sampleUint(rt[1]))
	head("alder_go_gc_cycles_total", "counter", "Completed garbage collection cycles.")
	fmt.Fprintf(b, "alder_go_gc_cycles_total %d\n", sampleUint(rt[2]))

	head("alder_process_start_time_seconds", "gauge", "When the process started, in seconds since the Unix epoch.")
	fmt.Fprintf(b, "alder_process_start_time_seconds %s\n", num(float64(c.start.UnixNano())/1e9))

	head("alder_build_info", "gauge", "The build being served (ALDER_BUILD); always 1.")
	fmt.Fprintf(b, "alder_build_info{build=\"%s\"} 1\n", escape(build))
}

func sampleUint(s metrics.Sample) uint64 {
	if s.Value.Kind() == metrics.KindUint64 {
		return s.Value.Uint64()
	}
	return 0
}

// num is a sample value or a bucket bound as the text format writes it.
func num(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// escape is a label value escaped per the text format: \ " and a line break.
func escape(s string) string {
	if !strings.ContainsAny(s, "\\\"\n") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(s)
}
