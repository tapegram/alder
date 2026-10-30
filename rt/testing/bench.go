package rtesting

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"alder/rt/eff"
)

// Measure is what one browser step (Test.get, Test.open, Test.click, Test.submit,
// Test.submitRaw, Test.fetch) cost, from its first request to its return: `alder bench`'s row.
// Step is its first request, `GET /` or `POST /_m/Rows.Index/Toggle`; Ms the wall time (with
// --count, the median over the runs); Bytes the answers' bodies (a document, or a Datastar
// answer's SSE text); Alloc and Allocs the bytes and objects allocated meanwhile
// (runtime.MemStats TotalAlloc and Mallocs); Requests the requests the router served (a redirect
// it followed is a second; so is each 303 the browser followed); Loads the pages' `load` runs
// (eff.Recorder.Loads: a Rerender runs it before and after update); Steps the effect Calls
// (the requests' Env.Trace); SQL the statements; Repeats what eff.Repeats found in them.
type Measure struct {
	Step     string
	Ms       float64
	Bytes    int
	Alloc    uint64
	Allocs   uint64
	Requests int
	Loads    int
	Steps    int
	SQL      int
	Repeats  []eff.Repeat
}

// stepAcc accumulates one step's requests while it runs (Browser.track).
type stepAcc struct {
	m     Measure
	start time.Time
	mem   runtime.MemStats
}

// track brackets a browser step: it returns the function that ends it, which turns the requests
// the step made into its warnings (Browser.warnings: one per repeat eff.Repeats finds in one
// request's statements, "<METHOD> <path>: " + Repeat.String()) and, when the browser measures
// (bench), a Measure. A step made inside another (Get is Open) is part of it.
func (b *Browser) track() func() {
	if b.step != nil {
		return func() {}
	}
	s := &stepAcc{}
	b.step = s
	if b.bench {
		runtime.ReadMemStats(&s.mem)
		s.start = time.Now()
	}
	return func() {
		b.step = nil
		if !b.bench || s.m.Step == "" {
			return
		}
		ms := eff.Ms(time.Since(s.start))
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		s.m.Ms = ms
		s.m.Alloc, s.m.Allocs = after.TotalAlloc-s.mem.TotalAlloc, after.Mallocs-s.mem.Mallocs
		b.measures = append(b.measures, s.m)
	}
}

// account adds the request just served (its answer's body bytes, and the Envs it ran) to the
// step running, and warns of its repeats.
func (b *Browser) account(head string, bodyLen int) {
	s := b.step
	if s != nil {
		if s.m.Step == "" {
			s.m.Step = strings.TrimSuffix(head, ": ")
		}
		s.m.Requests += max(1, len(b.reqEnvs))
		s.m.Bytes += bodyLen
	}
	for _, e := range b.reqEnvs {
		if e.Rec == nil {
			continue
		}
		sql := appSQL(e.Rec.SQL)
		reps := eff.Repeats(sql, eff.RepeatThreshold)
		h := b.heads[e]
		if h == "" {
			h = strings.TrimSuffix(head, ": ")
		}
		for _, r := range reps {
			b.warnings = append(b.warnings, h+": "+r.String())
		}
		if s != nil {
			s.m.Loads += e.Rec.Loads
			s.m.Steps += len(e.Trace)
			s.m.SQL += len(sql)
			s.m.Repeats = append(s.m.Repeats, reps...)
		}
	}
}

// appSQL is sql without the savepoints a test's request runs under (eff.Within): the browser's,
// not the app's.
func appSQL(sql []eff.SQLRec) []eff.SQLRec {
	out := make([]eff.SQLRec, 0, len(sql))
	for _, q := range sql {
		if strings.HasPrefix(q.Stmt, "SAVEPOINT ") || strings.HasPrefix(q.Stmt, "RELEASE ") || strings.HasPrefix(q.Stmt, "ROLLBACK TO ") {
			continue
		}
		out = append(out, q)
	}
	return out
}

// mergeRuns is the bench of n runs of one test: each step's Ms, Alloc and Allocs the median of
// the runs', the rest the last run's (a test takes the same steps on every run).
func mergeRuns(runs [][]Measure) []Measure {
	if len(runs) == 0 {
		return nil
	}
	out := slices.Clone(runs[len(runs)-1])
	for i := range out {
		var ms []float64
		var alloc, allocs []uint64
		for _, r := range runs {
			if i < len(r) {
				ms, alloc, allocs = append(ms, r[i].Ms), append(alloc, r[i].Alloc), append(allocs, r[i].Allocs)
			}
		}
		out[i].Ms, out[i].Alloc, out[i].Allocs = median(ms), median(alloc), median(allocs)
	}
	return out
}

// median is the middle of xs (the lower middle of an even count).
func median[T float64 | uint64](xs []T) T {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[(len(s)-1)/2]
}

// jsonMeasure is a Measure in --json's "bench".
type jsonMeasure struct {
	Step     string       `json:"step"`
	Ms       float64      `json:"ms"`
	Bytes    int          `json:"bytes"`
	Alloc    uint64       `json:"alloc"`
	Allocs   uint64       `json:"allocs"`
	Requests int          `json:"requests"`
	Loads    int          `json:"loads"`
	Steps    int          `json:"steps"`
	SQL      int          `json:"sql"`
	Repeats  []jsonRepeat `json:"repeats"`
}

type jsonRepeat struct {
	Stmt  string  `json:"stmt"`
	Count int     `json:"count"`
	Args  int     `json:"args"`
	Ms    float64 `json:"ms"`
}

func benchJSON(ms []Measure) []jsonMeasure {
	var out []jsonMeasure
	for _, m := range ms {
		j := jsonMeasure{Step: m.Step, Ms: m.Ms, Bytes: m.Bytes, Alloc: m.Alloc, Allocs: m.Allocs, Requests: m.Requests,
			Loads: m.Loads, Steps: m.Steps, SQL: m.SQL, Repeats: []jsonRepeat{}}
		for _, r := range m.Repeats {
			j.Repeats = append(j.Repeats, jsonRepeat{Stmt: r.Stmt, Count: r.Count, Args: r.Args, Ms: r.Ms})
		}
		out = append(out, j)
	}
	return out
}

// benchRowFormat is a row of the text table under a test's result line (writeBench).
const benchRowFormat = "  %-34s %9s %9s %17s %4s %5s %5s %5s  %s\n"

func writeBench(b *strings.Builder, ms []Measure) {
	if len(ms) == 0 {
		return
	}
	fmt.Fprintf(b, benchRowFormat, "step", "ms", "bytes", "alloc", "req", "loads", "steps", "sql", "n+1")
	for _, m := range ms {
		var reps []string
		for _, r := range m.Repeats {
			reps = append(reps, fmt.Sprintf("%dx (%d args)", r.Count, r.Args))
		}
		fmt.Fprintf(b, benchRowFormat, m.Step, strconv.FormatFloat(m.Ms, 'f', 2, 64), size(uint64(m.Bytes)),
			size(m.Alloc)+" / "+count(m.Allocs), strconv.Itoa(m.Requests), strconv.Itoa(m.Loads), strconv.Itoa(m.Steps),
			strconv.Itoa(m.SQL), strings.Join(reps, ", "))
	}
}

// size is n bytes as "812 B", "12.3 kB" or "4.56 MB".
func size(n uint64) string {
	switch {
	case n < 1000:
		return strconv.FormatUint(n, 10) + " B"
	case n < 1000*1000:
		return strconv.FormatFloat(float64(n)/1000, 'f', 1, 64) + " kB"
	}
	return strconv.FormatFloat(float64(n)/1e6, 'f', 2, 64) + " MB"
}

// count is n as "812", "12.3k" or "4.56M".
func count(n uint64) string {
	switch {
	case n < 1000:
		return strconv.FormatUint(n, 10)
	case n < 1000*1000:
		return strconv.FormatFloat(float64(n)/1000, 'f', 1, 64) + "k"
	}
	return strconv.FormatFloat(float64(n)/1e6, 'f', 2, 64) + "M"
}
