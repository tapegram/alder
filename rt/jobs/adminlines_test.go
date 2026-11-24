package jobs_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"alder/rt/jobs"
)

func TestAdminLine_RoundTrip(t *testing.T) {
	full := jobs.Row{
		ID: "j1", Kind: "Signup.Nightly", Params: "secret", State: "dead", Worker: "w1",
		Key: ptr("k"), RunAt: 1, Created: 2, Started: ptr(int64(3)), Finished: ptr(int64(4)),
		LeaseUntil: ptr(int64(5)), ScheduledFor: ptr(int64(6)), Attempts: 3, Priority: 7,
		Error: ptr("boom"), Schedule: true,
	}
	bare := jobs.Row{ID: "j2", Kind: "Signup.Nightly", State: "queued", RunAt: 1, Created: 2}
	line := jobs.AdminLine{
		Silo:   "org/1",
		Counts: map[jobs.State]int{jobs.Dead: 1, jobs.Queued: 2},
		Ticks:  []jobs.Row{full, bare},
		Dead:   []jobs.Row{bare, full},
		Row:    &full,
	}
	b, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, bad := range []string{`"params"`, `"In"`, `secret`} {
		if strings.Contains(text, bad) {
			t.Errorf("encoded line has %s: %s", bad, text)
		}
	}
	lines, msgs := jobs.DecodeAdmin(text + "\n")
	if len(lines) != 1 || len(msgs) != 0 {
		t.Fatalf("lines %v, messages %v", lines, msgs)
	}
	want := line
	zero := func(r jobs.Row) jobs.Row { r.Params = ""; r.In = jobs.Row{}.In; return r }
	want.Ticks = []jobs.Row{zero(full), zero(bare)}
	want.Dead = []jobs.Row{zero(bare), zero(full)}
	w := zero(full)
	want.Row = &w
	if !reflect.DeepEqual(lines[0], want) {
		t.Errorf("decoded %+v, want %+v", lines[0], want)
	}
}

func TestDecodeAdmin_Messages(t *testing.T) {
	in := "alder: warn x\n{\"level\":\"warn\",\"msg\":\"y\"}\n{\"silo\":\"\",\"counts\":{\"dead\":1}}\n{not json\n"
	lines, msgs := jobs.DecodeAdmin(in)
	if len(lines) != 1 || lines[0].Counts[jobs.Dead] != 1 {
		t.Errorf("lines %+v", lines)
	}
	want := []string{"alder: warn x", `{"level":"warn","msg":"y"}`, "{not json"}
	if !reflect.DeepEqual(msgs, want) {
		t.Errorf("messages %q, want %q", msgs, want)
	}
}

func TestDecodeAdmin_UnknownField(t *testing.T) {
	lines, msgs := jobs.DecodeAdmin(`{"silo":"org/1","counts":{},"later":7}` + "\n")
	if len(lines) != 1 || len(msgs) != 0 || lines[0].Silo != "org/1" {
		t.Errorf("lines %+v, messages %q", lines, msgs)
	}
}

func TestResolveKind(t *testing.T) {
	kinds := []jobs.Kind{
		{Name: "Signup.Nightly"}, {Name: "Reports.Nightly"},
		{Name: "Signup.SendWelcome", Params: []string{"user"}},
	}
	k, err := jobs.ResolveKind(kinds, "Signup.Nightly")
	if err != nil || k.Name != "Signup.Nightly" {
		t.Errorf("Signup.Nightly: %v, %v", k.Name, err)
	}
	for _, c := range []struct {
		kinds []jobs.Kind
		name  string
		want  string
	}{
		{kinds, "Nightly", "Nightly names several jobs: Signup.Nightly, Reports.Nightly; name one with its module"},
		{kinds, "Nope", "the app has no job Nope; its jobs: Signup.Nightly, Reports.Nightly, Signup.SendWelcome"},
		{kinds, "SendWelcome", "Signup.SendWelcome takes parameters (user): only a job without parameters is run from here; queue it from the app with Job.enqueue"},
		{nil, "Nope", "the app has no job Nope: it declares no job"},
	} {
		_, err := jobs.ResolveKind(c.kinds, c.name)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}

func TestHint_Upsert(t *testing.T) {
	st := open(t)
	q := raw(t, st)
	t1 := time.UnixMilli(1000)
	t2 := time.UnixMilli(2000)
	for _, at := range []time.Time{t1, t2} {
		if err := jobs.Hint(context.Background(), q, "org", "A", at); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := q.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM "_alder_jobs_silos"`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows %d, %v", n, err)
	}
	if got, ok := hint(t, st, "A"); !ok || got != t2.UnixMilli() {
		t.Errorf("touched %d (%v), want %d", got, ok, t2.UnixMilli())
	}
}

func TestOrphanedError(t *testing.T) {
	var err error = &jobs.OrphanedError{Silo: "org/1"}
	if got := err.Error(); got != "silo org/1 is orphaned: nothing runs there (alder silos prune)" {
		t.Error(got)
	}
}
