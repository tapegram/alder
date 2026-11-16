package ir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIR_Jobs lowers silo-routes-ok with a module of jobs: each becomes an ir.Job with its Params,
// its run arm and its options (defaults filled), in the dump; a job constructor is a JobRef.
func TestIR_Jobs(t *testing.T) {
	dir := t.TempDir()
	src := "../../testdata/corpus/silo-routes-ok"
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	jobs := "module Work\n\nimport Org\n\n" +
		"job Digest = { org: Org.OrgId, n: Int } silo org retries 0 timeout \"30s\"\n\n" +
		"Digest.run : Digest.Params -> Task Unit\nDigest.run p = Task.succeed ()\n\n" +
		"job Hourly = {} every \"1h\"\n\n" +
		"Hourly.run : Hourly.Params -> Task Unit\nHourly.run p = Task.succeed ()\n\n" +
		"next : Org.OrgId -> Job\nnext org = Digest { org: org, n: 1 }\n"
	if err := os.WriteFile(filepath.Join(dir, "app", "Work.alder"), []byte(jobs), 0o644); err != nil {
		t.Fatal(err)
	}
	p := lowerDir(t, dir)
	if len(p.Jobs) != 2 {
		t.Fatalf("jobs = %+v", p.Jobs)
	}
	d, h := p.Jobs[0], p.Jobs[1]
	if d.Module != "Work" || d.Name != "Digest" || d.Params.Name != "Digest.Params" || d.Run == nil || d.Run.Name != "Digest.run" ||
		d.Retries != 0 || d.Timeout != 30*time.Second || d.Every != 0 || d.SiloParam != "org" {
		t.Errorf("Digest = %+v", d)
	}
	if h.Retries != 4 || h.Timeout != 5*time.Minute || h.Every != time.Hour || h.Cron != "" || h.SiloParam != "" || h.Run == nil {
		t.Errorf("Hourly = %+v", h)
	}
	dump := spaces(Dump(p))
	for _, want := range []string{
		"(job Work.Digest (params Digest.Params) (run Work.Digest.run) (retries 0) (timeout 30s) (silo org))",
		"(job Work.Hourly (params Hourly.Params) (run Work.Hourly.run) (retries 4) (timeout 5m0s) (every 1h0m0s))",
		"(job-ref Work.Digest",
	} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump lacks %q", want)
		}
	}
}
