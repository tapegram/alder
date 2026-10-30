package db

import (
	"testing"

	"alder/rt/eff"
)

// With Env.Stats set, each statement a step runs is counted by kind and timed (#92); with
// neither Stats nor a Recorder nothing is.
func TestStats_Counted(t *testing.T) {
	s := openItems(t)
	env := itemEnv(s, "ann")
	env.Stats = &eff.Stats{}
	ins := call("Db.insert Item", true, func(e *eff.Env) (string, error) {
		return InsertRow(e.Ctx, e.Store.(Store), TxOf(e), itemTable, []any{nil, "t", "ann", nil}, e.Now())
	})
	all := call("Db.all Item", false, func(e *eff.Env) ([]item, error) {
		return All(e.Ctx, e.Store.(Store), TxOf(e), From(itemTable))
	})
	if _, err := eff.Run(env, eff.KTask, ins); err != nil {
		t.Fatal(err)
	}
	if _, err := eff.Run(env, eff.KQuery, all); err != nil {
		t.Fatal(err)
	}
	st := *env.Stats
	if st.Exec < 1 || st.Query < 1 || st.ExecDur <= 0 || st.QueryDur <= 0 {
		t.Errorf("stats %+v", st)
	}
	if env.Rec != nil {
		t.Error("a Recorder appeared")
	}

	// neither set: untouched (nothing to count into, nothing timed)
	plain := itemEnv(s, "ann")
	if _, err := eff.Run(plain, eff.KQuery, all); err != nil {
		t.Fatal(err)
	}
	if plain.Stats != nil || plain.Rec != nil {
		t.Errorf("stats %v, rec %v", plain.Stats, plain.Rec)
	}
}
