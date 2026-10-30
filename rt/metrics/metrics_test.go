package metrics

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"alder/rt/eff"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestFromEnv(t *testing.T) {
	for _, v := range []string{"", "  "} {
		if c, err := fromEnv(env(map[string]string{"ALDER_METRICS": v})); c != nil || err != nil {
			t.Errorf("%q: %v, %v", v, c, err)
		}
	}
	if c, err := fromEnv(env(nil)); c != nil || err != nil {
		t.Errorf("unset: %v, %v", c, err)
	}
	if c, err := fromEnv(env(map[string]string{"ALDER_METRICS": " open "})); err != nil || c == nil || !c.auth.open {
		t.Errorf("open: %+v, %v", c, err)
	}
	short := "abcdefghijklmno" // 15 bytes
	c, err := fromEnv(env(map[string]string{"ALDER_METRICS": short}))
	if c != nil || err == nil || err.Error() != `ALDER_METRICS: want "open" or a token of at least 16 characters` || strings.Contains(err.Error(), short) {
		t.Errorf("15 bytes: %v, %v", c, err)
	}
	c, err = fromEnv(env(map[string]string{"ALDER_METRICS": short + "p"}))
	if err != nil || c == nil || c.auth.open {
		t.Errorf("16 bytes: %+v, %v", c, err)
	}
}

const tok = "s3cret-token-0123456789"

func TestAuthorized(t *testing.T) {
	open := New("")
	r := httptest.NewRequest("GET", "/_alder/metrics", nil)
	if !open.Authorized(r) {
		t.Error("open refused a scrape")
	}
	c := New(tok)
	for _, h := range []struct {
		auth string
		want bool
	}{
		{"Bearer " + tok, true},
		{"bearer " + tok, true},
		{"", false},
		{"Bearer ", false},
		{"Bearer " + tok + "x", false},
		{"Bearer " + tok[:len(tok)-1], false},
		{"Basic " + tok, false},
		{tok, false},
	} {
		r := httptest.NewRequest("GET", "/_alder/metrics", nil)
		if h.auth != "" {
			r.Header.Set("Authorization", h.auth)
		}
		if got := c.Authorized(r); got != h.want {
			t.Errorf("%q: %v", h.auth, got)
		}
	}
}

// The token is in nothing the package shows: not in the Collector printed or marshalled, not in
// the exposition, not in an error.
func TestToken_NeverShown(t *testing.T) {
	c := New(tok)
	c.Request("/", "GET", 200, time.Millisecond)
	var b strings.Builder
	if err := c.Write(&b, "b-1"); err != nil {
		t.Fatal(err)
	}
	j, _ := json.Marshal(c)
	for what, s := range map[string]string{
		"%v": fmt.Sprintf("%v", c), "%+v": fmt.Sprintf("%+v", c), "%#v": fmt.Sprintf("%#v", c),
		"json": string(j), "exposition": b.String(),
	} {
		if strings.Contains(s, tok) || strings.Contains(s, tok[:16]) {
			t.Errorf("%s shows the token: %s", what, s)
		}
	}
	_, err := fromEnv(env(map[string]string{"ALDER_METRICS": tok[:15]}))
	if err == nil || strings.Contains(err.Error(), tok[:15]) {
		t.Errorf("error %v", err)
	}
}

// expoHead is the exposition up to the Go runtime's metrics, which vary run to run.
func expoHead(t *testing.T, c *Collector) string {
	t.Helper()
	var b strings.Builder
	if err := c.Write(&b, "b-1"); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	i := strings.Index(s, "# HELP alder_go_goroutines")
	if i < 0 {
		t.Fatalf("no runtime section:\n%s", s)
	}
	return s[:i]
}

func TestCollector_Exposition(t *testing.T) {
	c := New("")
	c.Request("/todos/{id}", "GET", 200, 30*time.Millisecond)
	c.Request("/", "POST", 303, 2*time.Millisecond)
	c.Request("/", "GET", 200, 20*time.Second)
	c.InFlight(1)
	c.Steps([]string{"Db.all Todo", "Time.now", "Db.all Todo"})
	c.Statements(&eff.Stats{Exec: 2, Query: 3, ExecDur: 500 * time.Millisecond, QueryDur: 250 * time.Millisecond})
	want := `# HELP alder_http_requests_total Requests answered, by route pattern, method and status class.
# TYPE alder_http_requests_total counter
alder_http_requests_total{route="/",method="GET",status="2xx"} 1
alder_http_requests_total{route="/",method="POST",status="3xx"} 1
alder_http_requests_total{route="/todos/{id}",method="GET",status="2xx"} 1
# HELP alder_http_request_duration_seconds Request latency, by route pattern and method.
# TYPE alder_http_request_duration_seconds histogram
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="0.005"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="0.01"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="0.025"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="0.05"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="0.1"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="0.25"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="0.5"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="1"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="2.5"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="5"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="10"} 0
alder_http_request_duration_seconds_bucket{route="/",method="GET",le="+Inf"} 1
alder_http_request_duration_seconds_sum{route="/",method="GET"} 20
alder_http_request_duration_seconds_count{route="/",method="GET"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="0.005"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="0.01"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="0.025"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="0.05"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="0.1"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="0.25"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="0.5"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="1"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="2.5"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="5"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="10"} 1
alder_http_request_duration_seconds_bucket{route="/",method="POST",le="+Inf"} 1
alder_http_request_duration_seconds_sum{route="/",method="POST"} 0.002
alder_http_request_duration_seconds_count{route="/",method="POST"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="0.005"} 0
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="0.01"} 0
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="0.025"} 0
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="0.05"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="0.1"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="0.25"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="0.5"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="1"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="2.5"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="5"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="10"} 1
alder_http_request_duration_seconds_bucket{route="/todos/{id}",method="GET",le="+Inf"} 1
alder_http_request_duration_seconds_sum{route="/todos/{id}",method="GET"} 0.03
alder_http_request_duration_seconds_count{route="/todos/{id}",method="GET"} 1
# HELP alder_http_requests_in_flight Requests being answered now.
# TYPE alder_http_requests_in_flight gauge
alder_http_requests_in_flight 1
# HELP alder_db_statements_total Database statements run by requests, by kind (exec or query).
# TYPE alder_db_statements_total counter
alder_db_statements_total{kind="exec"} 2
alder_db_statements_total{kind="query"} 3
# HELP alder_db_statement_seconds_total Time spent in database statements run by requests, by kind.
# TYPE alder_db_statement_seconds_total counter
alder_db_statement_seconds_total{kind="exec"} 0.5
alder_db_statement_seconds_total{kind="query"} 0.25
# HELP alder_effect_steps_total Effect steps run by requests, by Call name.
# TYPE alder_effect_steps_total counter
alder_effect_steps_total{name="Db.all Todo"} 2
alder_effect_steps_total{name="Time.now"} 1
`
	if got := expoHead(t, c); got != want {
		t.Errorf("exposition:\n%s\nwant:\n%s", got, want)
	}
}

// line is the value of the series written exactly as series, or -1.
func line(t *testing.T, c *Collector, series string) float64 {
	t.Helper()
	var b strings.Builder
	c.Write(&b, "b-1")
	for _, l := range strings.Split(b.String(), "\n") {
		if v, ok := strings.CutPrefix(l, series+" "); ok {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				t.Fatalf("%s: %q", series, v)
			}
			return f
		}
	}
	return -1
}

func TestCollector_StatusClass(t *testing.T) {
	c := New("")
	for _, s := range []int{0, 101, 204, 303, 404, 500} {
		c.Request("/", "GET", s, time.Millisecond)
	}
	for class, n := range map[string]float64{"1xx": 1, "2xx": 2, "3xx": 1, "4xx": 1, "5xx": 1} {
		if got := line(t, c, `alder_http_requests_total{route="/",method="GET",status="`+class+`"}`); got != n {
			t.Errorf("%s: %v, want %v", class, got, n)
		}
	}
}

func TestCollector_MethodOther(t *testing.T) {
	c := New("")
	c.Request("/", "PUT", 404, time.Millisecond)
	c.Request("/", "DELETE", 404, time.Millisecond)
	c.Request("/", "HEAD", 200, time.Millisecond)
	if got := line(t, c, `alder_http_requests_total{route="/",method="other",status="4xx"}`); got != 2 {
		t.Errorf("other: %v", got)
	}
	if got := line(t, c, `alder_http_requests_total{route="/",method="HEAD",status="2xx"}`); got != 1 {
		t.Errorf("HEAD: %v", got)
	}
}

func TestCollector_HistogramBuckets(t *testing.T) {
	c := New("")
	c.Request("/", "GET", 200, 30*time.Millisecond)
	for le, n := range map[string]float64{"0.025": 0, "0.05": 1, "0.1": 1, "10": 1, "+Inf": 1} {
		if got := line(t, c, `alder_http_request_duration_seconds_bucket{route="/",method="GET",le="`+le+`"}`); got != n {
			t.Errorf("le %s: %v, want %v", le, got, n)
		}
	}
}

func TestCollector_LabelEscape(t *testing.T) {
	c := New("")
	c.Request("/a\\b\"c\nd", "GET", 200, time.Millisecond)
	if got := line(t, c, `alder_http_requests_total{route="/a\\b\"c\nd",method="GET",status="2xx"}`); got != 1 {
		t.Errorf("escaped: %v", got)
	}
	var b strings.Builder
	c.Write(&b, "x\"y")
	if !strings.Contains(b.String(), `alder_build_info{build="x\"y"} 1`) {
		t.Errorf("build:\n%s", b.String())
	}
}

// A step is counted by name; a delay's milliseconds (the app may compute them from a request)
// are not part of the name, and past MaxSteps distinct names the rest go to "(other)".
func TestCollector_Steps(t *testing.T) {
	c := New("")
	c.Steps([]string{"Db.all Todo", "Task.sleep 20ms", "Task.sleep 31ms", "Query.delay 5ms", "Log.info"})
	c.Steps(nil)
	for name, n := range map[string]float64{"Db.all Todo": 1, "Task.sleep": 2, "Query.delay": 1, "Log.info": 1} {
		if got := line(t, c, `alder_effect_steps_total{name="`+name+`"}`); got != n {
			t.Errorf("%s: %v, want %v", name, got, n)
		}
	}
	var many []string
	for i := range MaxSteps + 10 {
		many = append(many, fmt.Sprintf("X.y%d", i))
	}
	c.Steps(many)
	if len(c.steps) != MaxSteps+1 || c.steps["(other)"] == 0 {
		t.Errorf("%d names, (other) %d", len(c.steps), c.steps["(other)"])
	}
}

func TestCollector_Statements(t *testing.T) {
	c := New("")
	c.Statements(nil)
	c.Statements(&eff.Stats{})
	c.Statements(&eff.Stats{Exec: 1, Query: 2, ExecDur: time.Second, QueryDur: 2 * time.Second})
	c.Statements(&eff.Stats{Query: 1, QueryDur: time.Second})
	for s, n := range map[string]float64{
		`alder_db_statements_total{kind="exec"}`: 1, `alder_db_statements_total{kind="query"}`: 3,
		`alder_db_statement_seconds_total{kind="exec"}`: 1, `alder_db_statement_seconds_total{kind="query"}`: 3,
	} {
		if got := line(t, c, s); got != n {
			t.Errorf("%s: %v, want %v", s, got, n)
		}
	}
}

func TestCollector_Runtime(t *testing.T) {
	c := New("")
	if g := line(t, c, "alder_go_goroutines"); g < 1 {
		t.Errorf("goroutines %v", g)
	}
	if h := line(t, c, "alder_go_heap_bytes"); h <= 0 {
		t.Errorf("heap %v", h)
	}
	if g := line(t, c, "alder_go_gc_cycles_total"); g < 0 {
		t.Errorf("gc %v", g)
	}
	st := line(t, c, "alder_process_start_time_seconds")
	if d := float64(time.Now().Unix()) - st; d < -1 || d > 60 {
		t.Errorf("start %v, now %v", st, time.Now().Unix())
	}
	if b := line(t, c, `alder_build_info{build="b-1"}`); b != 1 {
		t.Errorf("build_info %v", b)
	}
	var b strings.Builder
	c.Write(&b, "b-1")
	for _, h := range []string{"# TYPE alder_go_goroutines gauge", "# TYPE alder_go_heap_bytes gauge", "# TYPE alder_go_gc_cycles_total counter",
		"# TYPE alder_process_start_time_seconds gauge", "# TYPE alder_build_info gauge"} {
		if !strings.Contains(b.String(), h+"\n") {
			t.Errorf("missing %q", h)
		}
	}
}

// Counting a request on a known route allocates nothing: the series' key is a struct.
func TestRequest_NoAlloc(t *testing.T) {
	c := New("")
	c.Request("/todos/{id}", "GET", 200, time.Millisecond)
	if n := testing.AllocsPerRun(200, func() {
		c.InFlight(1)
		c.Request("/todos/{id}", "GET", 200, time.Millisecond)
		c.Steps([]string{"Db.get Todo"})
		c.InFlight(-1)
	}); n != 0 {
		t.Errorf("%v allocations per request", n)
	}
}

func BenchmarkRequest(b *testing.B) {
	c := New("")
	b.ReportAllocs()
	for b.Loop() {
		c.Request("/todos/{id}", "GET", 200, time.Millisecond)
	}
}
