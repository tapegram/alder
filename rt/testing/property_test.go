package rtesting

import (
	"testing"

	"alder/rt/eff"
	"alder/rt/fuzz"
)

// Until the property loop exists (Task 6), a property case fails with a message saying so.
func TestRun_PropertyNotRunYet(t *testing.T) {
	p := ForAll(fuzz.IntF(), func(n int64) bool { return n == n })
	rs := one(Case{Module: "M", Name: "p", Prop: p})
	if rs[0].OK || rs[0].Error != "property tests are not run yet" {
		t.Fatalf("got %+v", rs[0])
	}
}

func TestForAll(t *testing.T) {
	p := ForAll(fuzz.IntF(), func(n int64) bool { return n >= 0 })
	vs := p.Draw(fuzz.Replay(make([]uint64, 8)))
	if len(vs) != 1 || vs[0].(int64) != 0 || !p.Pure(vs) || p.Task != nil {
		t.Fatalf("ForAll: %v", vs)
	}
	q := ForAllTask(fuzz.BoolF(), func(b bool) eff.Task[bool] { return eff.Pure(!b) })
	if vs := q.Draw(fuzz.Replay([]uint64{1})); vs[0] != true || q.Pure != nil || q.Task == nil {
		t.Fatalf("ForAllTask: %v", vs)
	}
}
