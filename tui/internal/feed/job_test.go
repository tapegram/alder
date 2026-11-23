package feed

import "testing"

// A job attempt's record (rt/applog/job.go) is recognised by rid, job, outcome and attempt; it is
// no access record, and a record missing one of the four is neither.
func TestDecode_JobRecord(t *testing.T) {
	line := func(rec string) []byte { return []byte(`{"type":"log","seq":3,"record":` + rec + `}`) }
	ev, err := Decode(line(`{"time":"2026-11-01T14:02:20Z","level":"WARN","msg":"job Signup.SendWelcome a1b2c3d4 failed attempt 2/5","rid":"a1b2c3d4","job":"Signup.SendWelcome","outcome":"failed","ms":120.5,"attempt":2}`))
	if err != nil {
		t.Fatal(err)
	}
	r := ev.Log.Record
	if !r.HasJob || r.HasAccess || r.Rid != "a1b2c3d4" || r.Job != "Signup.SendWelcome" || r.Outcome != "failed" || r.Attempt != 2 || r.Ms != 120.5 || r.Level != "WARN" {
		t.Errorf("job record = %+v", r)
	}
	for _, rec := range []string{
		`{"level":"INFO","msg":"x","rid":"a1","job":"K","outcome":"done"}`,
		`{"level":"INFO","msg":"x","job":"K","outcome":"done","attempt":1}`,
		`{"level":"INFO","msg":"x","rid":"a1","method":"GET","path":"/","status":200}`,
	} {
		ev, err := Decode(line(rec))
		if err != nil || ev.Log.Record == nil || ev.Log.Record.HasJob {
			t.Errorf("%s: HasJob is true or the record did not decode (%v)", rec, err)
		}
	}
}
