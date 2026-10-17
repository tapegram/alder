//go:build fly

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The Fly target against the real Fly, run by hand (it is never run by CI: it needs an account
// and costs a little):
//
//	FLY_API_TOKEN=… ALDER_FLY_APP=alder-test-<you> [ALDER_FLY_REGION=ord] [FLY_ORG=personal] \
//	  go test -tags fly ./cmd/alder -run DeployFly -v -timeout 30m
//
// flyctl must be on PATH. The app is created when it does not exist (in FLY_ORG), with a 1 GB
// volume and one shared-cpu machine; the test leaves them: `fly apps destroy <app>` removes
// everything. The unit tests of internal/deploy (fly_test.go) cover the same steps against a
// fake Machines API.
func TestDeployFly_Real(t *testing.T) {
	app := os.Getenv("ALDER_FLY_APP")
	if os.Getenv("FLY_API_TOKEN") == "" || app == "" {
		t.Skip("set FLY_API_TOKEN and ALDER_FLY_APP to deploy to Fly")
	}
	region := os.Getenv("ALDER_FLY_REGION")
	if region == "" {
		region = "ord"
	}
	dir := deployScaffold(t, fmt.Sprintf(`deploy {
  staging: Fly %q { app: %q },
  requires: ["SMTP_URL"],
}`, region, app), nil)
	var key [16]byte
	rand.Read(key[:])
	t.Setenv("ALDER_SECRET", hex.EncodeToString(key[:]))
	t.Setenv("SMTP_URL", "smtp://localhost:25")
	t.Setenv(envHealthTimeout, "180s")
	deployed := regexp.MustCompile(`deployed ([0-9a-f]{16}) to staging`)
	ship := func() string {
		t.Helper()
		code, out, errs := runDeploy(t, "staging", dir)
		m := deployed.FindStringSubmatch(out)
		if code != 0 || m == nil {
			t.Fatalf("deploy: exit %d\n%s%s", code, out, errs)
		}
		t.Log(out)
		return m[1]
	}
	serving := func() string {
		resp, err := http.Get("https://" + app + ".fly.dev/")
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		m := regexp.MustCompile(`name="alder-build" content="([0-9a-f]+)"`).FindSubmatch(b)
		if m == nil {
			return ""
		}
		return string(m[1])
	}

	id1 := ship()
	if got := serving(); got != id1 {
		t.Fatalf("after the first deploy %s.fly.dev serves %q, want %s", app, got, id1)
	}
	appendTo(t, "demo/app/Home.alder", fmt.Sprintf("\nversion : Int\nversion = %d\n", time.Now().Unix()))
	id2 := ship()
	if got := serving(); got != id2 {
		t.Fatalf("after the second deploy %s.fly.dev serves %q, want %s", app, got, id2)
	}
	var so, se bytes.Buffer
	if code := runStatusTo([]string{"staging", dir}, &so, &se); code != 0 || !strings.Contains(so.String(), "current  "+id2) {
		t.Errorf("status: exit %d\n%s%s", code, so.String(), se.String())
	}
	so.Reset()
	if code := runRollbackTo([]string{"staging", "--dir", dir}, &so, &se); code != 0 || !strings.Contains(so.String(), "rolled back to "+id1) {
		t.Fatalf("rollback: exit %d\n%s%s", code, so.String(), se.String())
	}
	if got := serving(); got != id1 {
		t.Errorf("after the rollback %s.fly.dev serves %q, want %s", app, got, id1)
	}
	so.Reset()
	if code := runLogsTo([]string{"staging", dir}, &so, &se); code != 0 {
		t.Errorf("logs: exit %d\n%s", code, se.String())
	}
}
