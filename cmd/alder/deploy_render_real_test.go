//go:build render

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

// The Render target against a real Render workspace, a real Turso organization and a real
// registry. It is run by hand, with the person whose accounts they are, and never in CI: it
// creates a service that costs money while it exists, and it needs tokens that must not sit in
// a CI secret store for it.
//
//	ALDER_RENDER_REAL=1 RENDER_API_KEY=… TURSO_API_TOKEN=… ALDER_TURSO_ORG=<org> \
//	  ALDER_RENDER_IMAGE=ghcr.io/<owner>/alder-m16 ALDER_REGISTRY_TOKEN=<PAT, read+write:packages> \
//	  [ALDER_REGISTRY_USER=<owner>] [ALDER_RENDER_REGION=oregon] \
//	  go test -tags render ./cmd/alder -run TestRenderReal -v -timeout 60m
//
// ALDER_RENDER_IMAGE is a registry path the token can push to; make the package public after
// the first push if Render's registry credential cannot read it. The test deploys the scaffold
// to a service and a Turso database named alder-m16-<random>, edits the app and deploys again,
// reads the status, rolls back, and reads the logs, checking after each deploy that the service
// answers /_alder/health with the build it was just given. It deletes nothing: it prints the
// commands that remove what it made (the Render service, the Turso databases, the registry
// credential and the image package), whether it passed or not. The unit and end-to-end tests
// (internal/deploy/render_test.go, deploy_render_test.go) cover the same steps against fakes.
func TestRenderReal(t *testing.T) {
	if os.Getenv("ALDER_RENDER_REAL") != "1" {
		t.Skip("set ALDER_RENDER_REAL=1 (and the variables in this file's comment) to deploy to Render by hand")
	}
	for _, name := range []string{"RENDER_API_KEY", "TURSO_API_TOKEN", "ALDER_TURSO_ORG", "ALDER_RENDER_IMAGE", "ALDER_REGISTRY_TOKEN"} {
		if os.Getenv(name) == "" {
			t.Fatalf("ALDER_RENDER_REAL=1 needs %s", name)
		}
	}
	region := os.Getenv("ALDER_RENDER_REGION")
	if region == "" {
		region = "oregon"
	}
	var rb [3]byte
	rand.Read(rb[:])
	name := "alder-m16-" + hex.EncodeToString(rb[:])
	image, org := os.Getenv("ALDER_RENDER_IMAGE"), os.Getenv("ALDER_TURSO_ORG")
	t.Cleanup(func() {
		t.Logf("nothing was deleted. To clean up:\n"+
			"  Render service:  Render dashboard > %s > Settings > Delete Service (or DELETE https://api.render.com/v1/services/<id> with RENDER_API_KEY)\n"+
			"  Turso databases: turso db destroy %s --yes; and each silo, if any: turso db list | grep %s-\n"+
			"  registry:        the image %s:<build id> tags, and the Render registry credential alder-%s in Account Settings > Registry Credentials",
			name, name, name, image, strings.TrimPrefix(image, strings.SplitN(image, "/", 2)[0]+"/"))
	})

	dir := deployScaffold(t, fmt.Sprintf(`deploy {
  production: Render %q {
    service:  %q,
    database: Turso %q { app: %q },
  },
  image:    %q,
  requires: ["SMTP_URL"],
}`, region, name, org, name, image), nil)
	var key [16]byte
	rand.Read(key[:])
	t.Setenv("ALDER_SECRET", hex.EncodeToString(key[:]))
	t.Setenv("SMTP_URL", "smtp://localhost:25")
	t.Setenv(envHealthTimeout, "15m")

	deployed := regexp.MustCompile(`deployed ([0-9a-f]{16}) to production`)
	address := regexp.MustCompile(`(?m)^address  (\S+)`)
	var url string
	ship := func() string {
		t.Helper()
		code, out, errs := runDeploy(t, "production", dir)
		t.Log(out)
		m := deployed.FindStringSubmatch(out)
		if code != 0 || m == nil {
			t.Fatalf("deploy: exit %d\n%s", code, errs)
		}
		if a := address.FindStringSubmatch(out); a != nil {
			url = a[1]
		}
		return m[1]
	}
	serving := func() string {
		if url == "" {
			return ""
		}
		resp, err := http.Get(url + "/_alder/health")
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		m := regexp.MustCompile(`"build":"([0-9a-f]+)"`).FindSubmatch(b)
		if m == nil {
			return ""
		}
		return string(m[1])
	}
	waitServing := func(want, when string) {
		t.Helper()
		var got string
		for end := time.Now().Add(2 * time.Minute); time.Now().Before(end); time.Sleep(5 * time.Second) {
			if got = serving(); got == want {
				return
			}
		}
		t.Fatalf("%s %s serves build %q, want %s", when, url, got, want)
	}

	id1 := ship()
	waitServing(id1, "after the first deploy")
	appendTo(t, "demo/app/Home.alder", fmt.Sprintf("\nversion : Int\nversion = %d\n", time.Now().Unix()))
	id2 := ship()
	waitServing(id2, "after the second deploy")
	if id1 == id2 {
		t.Fatalf("the edit did not change the build: both are %s", id1)
	}

	var so, se bytes.Buffer
	if code := runStatusTo([]string{"production", dir}, &so, &se); code != 0 || !strings.Contains(so.String(), "current  "+id2) {
		t.Errorf("status: exit %d\n%s%s", code, so.String(), se.String())
	}
	t.Log(so.String())
	so.Reset()
	if code := runRollbackTo([]string{"production", "--dir", dir, "--yes"}, &so, &se); code != 0 || !strings.Contains(so.String(), "rolled back to "+id1) {
		t.Fatalf("rollback: exit %d\n%s%s", code, so.String(), se.String())
	}
	t.Log(so.String())
	waitServing(id1, "after the rollback")
	so.Reset()
	if code := runLogsTo([]string{"production", dir}, &so, &se); code != 0 {
		t.Errorf("logs: exit %d\n%s", code, se.String())
	}
	t.Logf("logs, first lines:\n%s", firstLines(so.String(), 10))
}

// firstLines is the first n lines of s.
func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
