package jobs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// listing is /_dev/jobs.json as rt/web's devjobs.go writes it (JobsView).
const listing = `{"workers":true,"schedules":[{"kind":"Reports.Nightly","rule":"every 1h","next":1793541600000,"id":"t1"},` +
	`{"kind":"Cleanup.Sweep","rule":"cron \"0 3 * * *\"","next":null}],` +
	`"queued":[{"id":"q1","kind":"Signup.SendWelcome","state":"queued","params":"{}","runAt":1793541000000,"created":1793540000000,"attempts":0},` +
	`{"id":"t1","kind":"Reports.Nightly","state":"queued","params":"{}","runAt":1793541600000,"created":1793540000000,"attempts":0,"tick":true}],` +
	`"running":[{"id":"r1","kind":"Signup.SendWelcome","state":"running","params":"{}","runAt":1,"created":1,"attempts":1,"worker":"w1"}],` +
	`"dead":[{"id":"d1","kind":"Signup.SendWelcome","silo":"org/42","state":"dead","params":"{}","runAt":1,"created":1,"attempts":5,"error":"smtp: timeout\nsecond line"}],` +
	`"done":[{"id":"x1","kind":"Reports.Nightly","state":"done","params":"{}","runAt":1,"created":1,"attempts":1}],` +
	`"missed":["silo org/7: unreadable"]}`

func TestParse(t *testing.T) {
	v, err := Parse([]byte(listing))
	if err != nil {
		t.Fatal(err)
	}
	if !v.Workers || len(v.Queued) != 2 || len(v.Running) != 1 || len(v.Dead) != 1 || len(v.Done) != 1 || len(v.Schedules) != 2 {
		t.Fatalf("view = %+v", v)
	}
	if s := v.Schedules[0]; s.Kind != "Reports.Nightly" || s.Rule != "every 1h" || s.Next == nil || *s.Next != 1793541600000 || s.ID != "t1" {
		t.Errorf("schedule 0 = %+v", s)
	}
	if s := v.Schedules[1]; s.Next != nil || s.ID != "" {
		t.Errorf("schedule 1 = %+v, want no next tick", s)
	}
	d := v.Dead[0]
	if d.ID != "d1" || d.Kind != "Signup.SendWelcome" || d.Silo != "org/42" || d.Attempts != 5 || d.Error != "smtp: timeout\nsecond line" {
		t.Errorf("dead = %+v", d)
	}
	if got := d.ErrorLine(); got != "smtp: timeout" {
		t.Errorf("ErrorLine = %q", got)
	}
	if !v.Queued[1].Tick || v.Queued[0].Tick {
		t.Errorf("ticks = %v %v", v.Queued[0].Tick, v.Queued[1].Tick)
	}
	if len(v.Missed) != 1 {
		t.Errorf("missed = %q", v.Missed)
	}
	if _, err := Parse([]byte("<html>")); err == nil {
		t.Error("parsed HTML")
	}
}

// devServer is a dev server's jobs endpoints: the listing, and the two POSTs behind the same
// cross-origin check rt/web's devJobAction makes. It records each POST's path and its headers.
func devServer(t *testing.T, post func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *[]string) {
	t.Helper()
	var posts []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_dev/jobs.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(listing))
	})
	mux.HandleFunc("POST /_dev/jobs/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewCrossOriginProtection().Check(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		posts = append(posts, r.URL.Path)
		if post != nil {
			post(w, r)
			return
		}
		http.Redirect(w, r, "/_dev/jobs/"+r.PathValue("rest"), http.StatusSeeOther)
	})
	mux.HandleFunc("GET /_dev/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the client followed the 303 to %s", r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &posts
}

func addr(srv *httptest.Server) string { return strings.TrimPrefix(srv.URL, "http://") }

func TestFetch(t *testing.T) {
	srv, _ := devServer(t, nil)
	v, err := Fetch(context.Background(), srv.Client(), addr(srv))
	if err != nil || len(v.Dead) != 1 {
		t.Fatalf("Fetch = %+v, %v", v, err)
	}
}

func TestFetch_NoJobs(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err := Fetch(context.Background(), srv.Client(), addr(srv))
	if !errors.Is(err, ErrNoJobs) {
		t.Errorf("err = %v, want ErrNoJobs", err)
	}
}

// The POSTs are what a non-browser client sends (no Origin, no Sec-Fetch-Site), which the dev
// server's cross-origin protection lets through; the 303 is success and is not followed.
func TestAct(t *testing.T) {
	srv, posts := devServer(t, nil)
	for _, a := range []Action{Retry, Run} {
		if err := Act(context.Background(), srv.Client(), addr(srv), a, "d1"); err != nil {
			t.Errorf("%s: %v", a, err)
		}
	}
	if want := []string{"/_dev/jobs/d1/retry", "/_dev/jobs/d1/run"}; strings.Join(*posts, " ") != strings.Join(want, " ") {
		t.Errorf("posts = %q, want %q", *posts, want)
	}
}

// A refusal's text (the dev server's own: a 409 for a job in another state, a 404, a 403) is the
// error.
func TestAct_Refused(t *testing.T) {
	for _, code := range []int{http.StatusConflict, http.StatusNotFound, http.StatusForbidden} {
		srv, _ := devServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "job d1 is running, not dead", code)
		})
		err := Act(context.Background(), srv.Client(), addr(srv), Retry, "d1")
		if err == nil || !strings.Contains(err.Error(), "job d1 is running, not dead") {
			t.Errorf("%d: err = %v", code, err)
		}
	}
}
