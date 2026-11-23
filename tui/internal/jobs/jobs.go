// Package jobs reads the jobs of the app the TUI is attached to from the dev server's Jobs page,
// GET /_dev/jobs.json, and acts on them with POST /_dev/jobs/{id}/retry and /run (rt/web's
// devjobs.go).
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxBody is the most of a response that is read.
const maxBody = 16 << 20

// Row is a job as the listing carries it: its id, kind, the silo it is in ("org/42"; empty for
// the shared store), its state (queued, running, dead or done), the attempts made, its error (all
// of it), whether it is its kind's queued schedule tick, and when it is due, in milliseconds
// since the epoch.
type Row struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Silo     string `json:"silo"`
	State    string `json:"state"`
	Attempts int    `json:"attempts"`
	Error    string `json:"error"`
	Tick     bool   `json:"tick"`
	RunAt    int64  `json:"runAt"`
}

// ErrorLine is the first line of the row's error.
func (r Row) ErrorLine() string {
	l, _, _ := strings.Cut(strings.TrimSpace(r.Error), "\n")
	return strings.TrimSpace(l)
}

// Schedule is a scheduled kind: its rule (`every 1h`) and its queued tick's due time in
// milliseconds since the epoch and id; Next is nil, and ID empty, when no tick is queued.
type Schedule struct {
	Kind string `json:"kind"`
	Rule string `json:"rule"`
	Next *int64 `json:"next"`
	ID   string `json:"id"`
}

// View is the listing: whether workers run in the app's process, the schedules, the queued rows
// by due time, the running, the dead (the last finished first), the last fifty done, and the
// stores that could not be read, each with its error.
type View struct {
	Workers   bool       `json:"workers"`
	Schedules []Schedule `json:"schedules"`
	Queued    []Row      `json:"queued"`
	Running   []Row      `json:"running"`
	Dead      []Row      `json:"dead"`
	Done      []Row      `json:"done"`
	Missed    []string   `json:"missed"`
}

// Parse reads the body of /_dev/jobs.json.
func Parse(b []byte) (View, error) {
	var v View
	if err := json.Unmarshal(b, &v); err != nil {
		return View{}, fmt.Errorf("reading the jobs: %w", err)
	}
	return v, nil
}

// ErrNoJobs is Fetch's error when the dev server has no Jobs page: the app declares no jobs, or
// its alder is older than the page.
var ErrNoJobs = errors.New("the app has no jobs page (it declares no jobs)")

// Fetch gets the listing from the app at addr, GET http://<addr>/_dev/jobs.json.
func Fetch(ctx context.Context, client *http.Client, addr string) (View, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/_dev/jobs.json", nil)
	if err != nil {
		return View{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return View{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return View{}, ErrNoJobs
	}
	if resp.StatusCode != http.StatusOK {
		return View{}, fmt.Errorf("GET /_dev/jobs.json: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return View{}, err
	}
	return Parse(b)
}

// Action is what a POST asks of a job: Retry queues a dead job again, Run runs a queued job now.
type Action string

const (
	Retry Action = "retry"
	Run   Action = "run"
)

// Act posts action for job id to the dev server at addr. The request is what a non-browser
// client sends: it carries no Origin and no Sec-Fetch-Site, which the dev server's cross-origin
// protection accepts (it refuses a POST a browser says came from another site). The dev server
// answers 303 to the job's page, which is success and is not followed; any other answer is an
// error carrying the dev server's text.
func Act(ctx context.Context, client *http.Client, addr string, a Action, id string) error {
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	u := "http://" + addr + "/_dev/jobs/" + url.PathEscape(id) + "/" + string(a)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusSeeOther {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if msg := strings.TrimSpace(string(b)); msg != "" {
		l, _, _ := strings.Cut(msg, "\n")
		return fmt.Errorf("%s", l)
	}
	return fmt.Errorf("%s", resp.Status)
}
