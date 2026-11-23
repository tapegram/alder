package gen

import (
	"strings"
	"testing"
)

// TestJobCodec_RoundTrip, TestJobCodec_Drift, TestJobValue_Eq and TestApp_Open (#168) run in
// the emitted gen_jobs app (testdata/e2e_jobs_test.go.txt): every kind of parameter
// round-trips through JSON, a row that no longer decodes is an error naming the field, a job
// value's == and Show are those of its kind and parameters, and Open's App serves, gives a job
// its Env (JobEnv), starts and stops its workers and closes.
func TestJobCodec_RoundTrip(t *testing.T) {
	runE2E(t, "../../testdata/golden/gen_jobs", "testdata/e2e_jobs_test.go.txt", "TestJobCodec_RoundTrip", "TestJobCodec_Drift", "TestJobValue_Eq", "TestJobValue_Priority", "TestApp_Open")
}

// An app without jobs has an empty Jobs().
func TestJobs_NoJobs(t *testing.T) {
	files := emitSource(t, map[string]string{"M.alder": "module M\n\nx : Int\nx = 1\n"})
	if !strings.Contains(string(files[appFile]), "func Jobs() []jobs.Kind {\n\treturn []jobs.Kind{}\n}") {
		t.Errorf("app file has no empty Jobs():\n%s", files[appFile])
	}
}
