package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"alder/internal/deploy"
)

// The default production path end to end: systemd units and a real Caddy, which the Ssh target
// installs itself (apt, its --resume drop-in) on the first deploy. It needs a Debian or Ubuntu
// machine with systemd as PID 1 that ssh reaches as root at the host alder-systemd-box of
// ALDER_SSH_CONFIG, and that this process reaches at https://localhost (the box is this
// machine): CI's deploy-systemd job (.github/workflows/ci.yml) is one, a GitHub runner's VM.
// Anywhere else it skips:
//
//	ALDER_DEPLOY_SYSTEMD=1 ALDER_SSH_CONFIG=… go test ./cmd/alder -run '^TestDeploySystemd_'
//
// Two deploys with the availability monitor on Caddy's public address (at most 2 of its samples
// missed at the switch), Caddy restarted (the routes survive: --resume), a build's unit restarted (systemd
// brings it back), and a rollback with --restore-backup (every unit stopped, the build started
// fresh, the route moved).
func TestDeploySystemd_Caddy(t *testing.T) {
	if os.Getenv("ALDER_DEPLOY_SYSTEMD") != "1" {
		t.Skip("ALDER_DEPLOY_SYSTEMD=1 runs this on a systemd box (CI's deploy-systemd job)")
	}
	cfg := os.Getenv(deploy.EnvSSHConfig)
	if cfg == "" {
		t.Fatal("ALDER_DEPLOY_SYSTEMD=1 needs ALDER_SSH_CONFIG naming the host alder-systemd-box")
	}
	box := &sshBox{config: cfg, host: "alder-systemd-box", root: "/srv/alder-systemd"}
	box.sh(t, "rm -rf "+box.root+" && mkdir -p "+box.root+fmt.Sprintf(" && printf 'ALDER_SECRET=%s\\nSMTP_URL=smtp://mail\\n' > %s/env && chmod 600 %s/env", strings.Repeat("s", 32), box.root, box.root))
	dir := deployScaffold(t, fmt.Sprintf(`deploy {
  production: Ssh %q { domain: "localhost", path: %q },
  requires: ["SMTP_URL"],
}`, box.host, box.root), nil)
	t.Setenv(envDrain, "3s")
	t.Setenv(envHealthTimeout, "60s")
	t.Setenv(deploy.EnvSupervisor, "")
	t.Cleanup(func() {
		exec.Command("ssh", "-F", cfg, box.host, "for u in $(systemctl list-units --plain --no-legend 'alder-demo-production@*' | cut -d' ' -f1); do systemctl disable --now \"$u\"; done").Run()
	})
	const public = "https://localhost"
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	serving := func() string { return pageBuild(client, public) }

	code, out, errs := runDeploy(t, "production", dir)
	if code != 0 {
		t.Fatalf("first deploy: %d\n%s%s", code, out, errs)
	}
	id1 := deployedID(t, out)
	if got := box.sh(t, "systemctl is-active alder-demo-production@"+id1+".service; systemctl is-enabled alder-demo-production@"+id1+".service"); got != "active\nenabled\n" {
		t.Errorf("the unit of %s: %q", id1, got)
	}
	if got := box.sh(t, "test -f /etc/systemd/system/caddy.service.d/alder.conf && echo drop-in"); got != "drop-in\n" {
		t.Errorf("Caddy's --resume drop-in: %q", got)
	}
	deadline := time.Now().Add(60 * time.Second) // Caddy's certificate for localhost, from its own CA
	for serving() != id1 {
		if time.Now().After(deadline) {
			t.Fatalf("Caddy does not serve %s at %s: %s", id1, public, box.sh(t, "journalctl -u caddy --no-pager -n 40 || true"))
		}
		time.Sleep(time.Second)
	}

	// a second deploy: no request misses at the switch
	mon := startMonitor(t, public, client) // client: Caddy's certificate for localhost is from its own CA
	t0 := time.Now()
	appendTo(t, "demo/app/Home.alder", "\nversion : Int\nversion = 2\n")
	if code, out, errs = runDeploy(t, "production", dir); code != 0 {
		t.Fatalf("second deploy: %d\n%s%s", code, out, errs)
	}
	id2 := deployedID(t, out)
	time.Sleep(4 * time.Second) // past the drain: the previous unit stops
	if n := mon.dropped(t0, id1, id2); n > 2 {
		t.Errorf("the switch dropped %d requests (at most 2 allowed): %s", n, mon.summary(t0))
	} else {
		t.Logf("the second deploy, as the monitor saw it: %s", mon.summary(t0))
	}
	if b := mon.since(t0); len(b) == 0 || b[len(b)-1] != id2 {
		t.Errorf("after the second deploy the monitor does not see %s: %s", id2, mon.summary(t0))
	}
	if got := box.sh(t, "systemctl is-active alder-demo-production@"+id1+".service || true"); got != "inactive\n" {
		t.Errorf("after the drain the previous unit is %q", got)
	}

	// Caddy restarted: the route comes back with it (--resume)
	box.sh(t, "systemctl restart caddy")
	deadline = time.Now().Add(30 * time.Second)
	for serving() != id2 {
		if time.Now().After(deadline) {
			t.Fatalf("after `systemctl restart caddy` the route is gone: %s", box.sh(t, "journalctl -u caddy --no-pager -n 40 || true"))
		}
		time.Sleep(500 * time.Millisecond)
	}
	// the build's process killed: systemd restarts it
	box.sh(t, "systemctl kill -s KILL alder-demo-production@"+id2+".service")
	deadline = time.Now().Add(30 * time.Second)
	for serving() != id2 {
		if time.Now().After(deadline) {
			t.Fatalf("systemd did not restart the killed build")
		}
		time.Sleep(500 * time.Millisecond)
	}

	// rollback with the copy taken when id1 was replaced: every unit stopped, id1 fresh, routed
	var so, se strings.Builder
	if code := runRollbackTo([]string{"production", id1, "--dir", dir, "--restore-backup", "--yes"}, &so, &se); code != 0 {
		t.Fatalf("rollback --restore-backup: %d\n%s%s", code, so.String(), se.String())
	}
	if got := serving(); got != id1 {
		t.Errorf("after the rollback Caddy serves %s, want %s", got, id1)
	}
	if got := box.sh(t, "systemctl list-units --plain --no-legend --state=active 'alder-demo-production@*' | cut -d' ' -f1"); got != "alder-demo-production@"+id1+".service\n" {
		t.Errorf("active units after the rollback: %q", got)
	}
}
