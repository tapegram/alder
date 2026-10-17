package deploy

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"alder/internal/ir"
)

func TestNewSsh(t *testing.T) {
	d := &ir.Deploy{Proxy: "None", Backup: &ir.Backup{Kind: "s3", URL: "s3://bucket/blog"}}
	for _, c := range []struct {
		t    ir.Target
		dest string
	}{
		{ir.Target{Host: "deploy@box.example.com", Path: "/srv/blog/"}, "deploy@box.example.com"},
		{ir.Target{Host: "box.example.com", User: "ops", Path: "/srv/blog"}, "ops@box.example.com"},
		{ir.Target{Host: "deploy@box.example.com", User: "ops", Path: "/srv/blog"}, "ops@box.example.com"},
	} {
		s := NewSsh("blog", "production", c.t, d)
		if s.Dest != c.dest || s.Root != "/srv/blog" || s.Proxy != "None" || s.BackupURL != "s3://bucket/blog" {
			t.Errorf("%+v → %+v", c.t, s)
		}
	}
	if s := NewSsh("my blog", "pr.12", ir.Target{Host: "h", Path: "/p"}, &ir.Deploy{}); s.unit() != "alder-my-blog-pr-12" || s.Proxy != "Caddy" {
		t.Errorf("unit %q proxy %q", s.unit(), s.Proxy)
	}
}

// q quotes for the remote shell: sh reads back exactly what was quoted.
func TestQuote(t *testing.T) {
	for _, s := range []string{"plain", "it's", "a b\tc", `$HOME "x" \n`, "'"} {
		out, err := exec.Command("sh", "-c", "printf %s "+q(s)).Output()
		if err != nil || string(out) != s {
			t.Errorf("q(%q): sh printed %q (%v)", s, out, err)
		}
	}
}

// The run script sources the operator's env file at the target and carries no value of it;
// the backup location is the declaration's.
func TestRunScript(t *testing.T) {
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: "/srv/blog"}, &ir.Deploy{Backup: &ir.Backup{URL: "s3://bucket/blog"}})
	r := s.runScript("0123456789abcdef")
	for _, want := range []string{"set -a; . '/srv/blog/env'; set +a", "export ALDER_DB='/srv/blog/db/app.db'", "export ALDER_STATIC='/srv/blog/builds/0123456789abcdef/static'",
		"export ALDER_BACKUP_URL='s3://bucket/blog'", `exec '/srv/blog/builds/0123456789abcdef/app' "$@"`} {
		if !strings.Contains(r, want) {
			t.Errorf("the run script lacks %q:\n%s", want, r)
		}
	}
}

func TestCaddyRoute(t *testing.T) {
	var r map[string]any
	if err := json.Unmarshal([]byte(CaddyRoute("alder-blog-production", "blog.example.com", 41233)), &r); err != nil {
		t.Fatal(err)
	}
	if r["@id"] != "alder-blog-production" || r["terminal"] != true {
		t.Errorf("%v", r)
	}
	b, _ := json.Marshal(r["match"])
	h, _ := json.Marshal(r["handle"])
	if string(b) != `[{"host":["blog.example.com"]}]` || string(h) != `[{"handler":"reverse_proxy","upstreams":[{"dial":"127.0.0.1:41233"}]}]` {
		t.Errorf("match %s handle %s", b, h)
	}
	sc := caddyRoute("alder-blog-production", "blog.example.com", 41233)
	for _, want := range []string{`put PATCH "$ROUTE" "/id/$ID"`, `/config/apps/http/servers/alder/routes`, `"listen":[":443"]`, "/load"} {
		if !strings.Contains(sc, want) {
			t.Errorf("the route script lacks %q", want)
		}
	}
	if _, err := exec.Command("sh", "-n", "-c", sc).CombinedOutput(); err != nil {
		t.Errorf("the route script does not parse: %v", err)
	}
}

// Every script the target sends parses as POSIX sh (sh -n), with each supervisor.
func TestScriptsParse(t *testing.T) {
	for _, sup := range []string{"process", "system", "user"} {
		s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: "/srv/blog"}, &ir.Deploy{})
		s.sup = sup
		for name, sc := range map[string]string{
			"prelude": s.prelude(), "stop": s.prelude() + "stopnow x\ncancel_later x\n", "ensure": caddyEnsure, "run": s.runScript("0123456789abcdef"),
		} {
			if out, err := exec.Command("sh", "-n", "-c", sc).CombinedOutput(); err != nil {
				t.Errorf("%s/%s: %v %s", sup, name, err, out)
			}
		}
	}
}

func TestParseSchema(t *testing.T) {
	sc := parseSchema("db\n0001_initial\n0002_add\n@@schema\n{\"tables\": []}\n", "db")
	if !sc.Have || strings.Join(sc.Migrations, ",") != "0001_initial,0002_add" || string(sc.Snapshot) != "{\"tables\": []}\n" {
		t.Errorf("%+v", sc)
	}
	if sc := parseSchema("none\n", "db"); sc.Have {
		t.Errorf("%+v", sc)
	}
}

// Prune removes failed builds first, never the newest deployed non-current build, and keeps a
// build whose backup.db a kept build needs.
func TestPrunePlan(t *testing.T) {
	ok := &Record{}
	b := func(id string, rec *Record, cur bool, of string) Build {
		return Build{ID: id, Record: rec, Current: cur, BackupOf: of}
	}
	builds := []Build{ // newest first
		b("f1", nil, false, ""), b("c", ok, true, "p"), b("p", ok, false, ""), b("f2", nil, false, ""), b("o1", ok, false, ""), b("o2", ok, false, ""),
	}
	if got := strings.Join(PrunePlan(builds, 3, nil), ","); got != "f2,f1,o2" {
		t.Errorf("keep 3: %s", got)
	}
	if got := strings.Join(PrunePlan(builds, 1, nil), ","); got != "f2,f1,o2,o1" {
		t.Errorf("keep 1 (c is current, p the last deployed one): %s", got)
	}
	// c's backup.db is the way back to p: c is current anyway; a pruned build whose backup names a kept one stays
	builds = []Build{b("n", ok, true, "m"), b("m", ok, false, "k"), b("k", ok, false, "")}
	if got := strings.Join(PrunePlan(builds, 1, nil), ","); got != "k" {
		t.Errorf("backup chain: %s", got)
	}
}
