package rtesting

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/web"
)

// slowRoute is GET /slow: a page whose load reads the rows, then each one again by id (n
// selects of one shape: the N+1).
func slowRoute(n int) web.Route {
	return web.Route{Method: "GET", Pattern: "/slow", Handle: func(rq *web.Request) web.Response {
		rows, err := loadIndex(rq)
		if err != nil {
			return fail(err)
		}
		_, err = eff.RunQuery(rq.Env, eff.Q(eff.Call("Db.get T", false, func(e *eff.Env) (int, error) {
			for i := range n {
				if _, err := db.GetRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, rows[i%len(rows)].ID); err != nil {
					return 0, err
				}
			}
			return 0, nil
		})))
		if err != nil {
			return fail(err)
		}
		return web.PageResponse(layout("Slow"), nil, nil, false)
	}}
}

// browseOpts is browse with o's Bench and Count and the fixture's routes plus extra.
func browseOpts(t *testing.T, o Options, extra []web.Route, steps ...func(b *Browser, e *eff.Env)) CaseResult {
	t.Helper()
	var task eff.Task[bool] = eff.Pure(true)
	for i := len(steps) - 1; i >= 0; i-- {
		f := steps[i]
		rest := task
		task = eff.Bind(eff.Call("step", true, func(e *eff.Env) (bool, error) {
			f(BrowserOf(e), e)
			return true, nil
		}), func(bool) eff.Task[bool] { return rest })
	}
	o.Tables, o.Routes, o.Web, o.Session = tables, append(fixtureRoutes(), extra...), fixtureWeb(), session{}
	r := benchCase(Case{Module: "M", Name: "browse", Task: func() eff.Task[bool] { return task }}, defaults(o))
	if !r.OK {
		t.Fatalf("the test failed: %s", r.Error)
	}
	return r
}

// A page whose load repeats a select five times gives the test one warning, and the test still
// passes; four is not a repeat. The JSON result carries the warning.
func TestBrowser_NPlusOneWarning(t *testing.T) {
	for _, n := range []int{4, 5} {
		r := browseOpts(t, Options{}, []web.Route{slowRoute(n)}, func(b *Browser, e *eff.Env) {
			insertDirect(t, e, "a")
			insertDirect(t, e, "b")
			if _, err := b.Open("/slow"); err != nil {
				t.Fatal(err)
			}
		})
		if n == 4 && len(r.Warnings) != 0 {
			t.Errorf("4 selects: %q", r.Warnings)
		}
		if n == 5 {
			if len(r.Warnings) != 1 || !strings.HasPrefix(r.Warnings[0], `GET /slow: N+1: SELECT "id", "title" FROM "M__T" WHERE "id" = ? ran 5 times (2 argument sets`) {
				t.Fatalf("warnings %q", r.Warnings)
			}
			var buf bytes.Buffer
			if err := WriteResult(&buf, r, true); err != nil {
				t.Fatal(err)
			}
			var j struct{ Warnings []string }
			if err := json.Unmarshal(buf.Bytes(), &j); err != nil || len(j.Warnings) != 1 {
				t.Errorf("json %s (%v)", buf.String(), err)
			}
			buf.Reset()
			WriteResult(&buf, r, false)
			if want := "ok   M \"browse\"\n  warn: GET /slow: N+1: "; !strings.HasPrefix(buf.String(), want) {
				t.Errorf("text %q", buf.String())
			}
		}
	}
}

// A step's Measure: Bytes is the answers' bodies; a Datastar submit whose redirect the router
// followed is 2 requests, its load counted once (the login page's POST has none); a click that
// re-renders runs the page's load; SQL is the statements the requests recorded.
func TestBrowser_Measure(t *testing.T) {
	var fetched int
	r := browseOpts(t, Options{Bench: true}, nil, func(b *Browser, e *eff.Env) {
		id := insertDirect(t, e, "a")
		f, err := b.Fetch("/login")
		if err != nil {
			t.Fatal(err)
		}
		fetched = len(f.Body)
		if _, err := b.Get("/login"); err != nil {
			t.Fatal(err)
		}
		if _, err := b.Submit("/_m/Login/Submit", url.Values{"email": {"me@example.com"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := b.Click("/_m/Index/Delete?id=" + id); err != nil {
			t.Fatal(err)
		}
	})
	if len(r.Bench) != 4 {
		t.Fatalf("%d measures: %+v", len(r.Bench), r.Bench)
	}
	fetch, get, submit, click := r.Bench[0], r.Bench[1], r.Bench[2], r.Bench[3]
	if fetch.Step != "GET /login" || fetch.Bytes != fetched || fetch.Requests != 1 || fetch.Loads != 0 || fetch.SQL != 0 {
		t.Errorf("fetch %+v (body %d)", fetch, fetched)
	}
	if get.Step != "GET /login" || get.Requests != 1 || get.Allocs == 0 || get.Ms <= 0 {
		t.Errorf("get %+v", get)
	}
	if submit.Step != "POST /_m/Login/Submit" || submit.Requests != 2 || submit.Loads != 1 || submit.SQL != 1 || submit.Steps != 1 {
		t.Errorf("submit %+v", submit)
	}
	if click.Step != "POST /_m/Index/Delete" || click.Requests != 1 || click.Loads != 1 || click.SQL != 3 || len(click.Repeats) != 0 {
		t.Errorf("click %+v", click)
	}
}

// Without Bench nothing is measured (the warnings are still made).
func TestBrowser_NoBenchNoMeasure(t *testing.T) {
	r := browseOpts(t, Options{}, nil, func(b *Browser, e *eff.Env) {
		if _, err := b.Get("/login"); err != nil {
			t.Fatal(err)
		}
	})
	if r.Bench != nil {
		t.Errorf("measured: %+v", r.Bench)
	}
}

// --count runs the test again: the report has each step's median time.
func TestBench_CountMedian(t *testing.T) {
	r := browseOpts(t, Options{Bench: true, Count: 3}, nil, func(b *Browser, e *eff.Env) {
		if _, err := b.Get("/login"); err != nil {
			t.Fatal(err)
		}
	})
	if len(r.Bench) != 1 {
		t.Fatalf("%+v", r.Bench)
	}
	got := mergeRuns([][]Measure{{{Step: "a", Ms: 3, Alloc: 30, SQL: 1}}, {{Step: "a", Ms: 1, Alloc: 10, SQL: 1}}, {{Step: "a", Ms: 2, Alloc: 20, SQL: 1}}})
	if len(got) != 1 || got[0].Ms != 2 || got[0].Alloc != 20 || got[0].SQL != 1 {
		t.Errorf("%+v", got)
	}
}

// Under --bench pure tests are skipped; the task test's result has its bench rows, after its ok
// line, in text and JSON.
func TestMain_Bench(t *testing.T) {
	t.Setenv("ALDER_TEST_RUN", "")
	t.Setenv("ALDER_TEST_JSON", "")
	t.Setenv(EnvBench, "")
	t.Setenv(EnvCount, "")
	cases := []Case{
		{Module: "M", Name: "pure", Pure: func() bool { return true }},
		{Module: "M", Name: "page", Task: func() eff.Task[bool] {
			return eff.Call("step", true, func(e *eff.Env) (bool, error) {
				_, err := BrowserOf(e).Get("/login")
				return err == nil, err
			})
		}},
	}
	run := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		code := Main(cases, tables, nil, nil, nil, session{}, fixtureRoutes(), fixtureWeb(), args, &out, &errb)
		return out.String(), errb.String(), code
	}
	out, errs, code := run("--bench", "--count", "2")
	if code != 0 || strings.Contains(out, "pure") || !strings.Contains(out, "1 passed, 0 failed") {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	lines := strings.Split(out, "\n")
	if lines[0] != `ok   M "page"` || !strings.HasPrefix(strings.TrimSpace(lines[1]), "step") || !strings.HasPrefix(strings.TrimSpace(lines[2]), "GET /login") {
		t.Errorf("text:\n%s", out)
	}
	out, _, code = run("--bench", "--json")
	var j struct {
		Bench []struct {
			Step     string
			Requests int
			Repeats  []any
		}
	}
	if code != 0 || json.Unmarshal([]byte(strings.SplitN(out, "\n", 2)[0]), &j) != nil || len(j.Bench) != 1 || j.Bench[0].Step != "GET /login" || j.Bench[0].Requests != 1 || j.Bench[0].Repeats == nil {
		t.Errorf("json %d: %s", code, out)
	}
	if _, errs, code := run("--count", "2"); code != 2 || !strings.Contains(errs, "--count") {
		t.Errorf("--count without --bench: %d %s", code, errs)
	}
	out, _, _ = run()
	if !strings.Contains(out, `ok   M "pure"`) || strings.Contains(out, "step") {
		t.Errorf("without --bench: %s", out)
	}
}
