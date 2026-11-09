//go:build browser

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBrowser_Patterns drives the four route-trigger patterns of the corpus app
// routes-patterns-ok (#160) in a real browser (testdata/browser/patterns.cjs): search as you
// type, infinite scroll, a slow widget and a polling badge. The app runs under `alder run`
// behind a reverse proxy that records every request (`loads`: the count per path, and each GET's
// path and query) and can hold the next /results answer 800 ms, so the script can show that a
// slow older answer never lands after a newer one. The script asks the proxy for what it saw
// (GET /_patterns/loads, POST /_patterns/reset, POST /_patterns/delay) and prints
// {"errors": [...]}, one entry per failed check.
func TestBrowser_Patterns(t *testing.T) {
	t.Parallel()
	node, env := playwrightEnv(t)
	script := browserScript(t, "patterns.cjs")
	r := startRunApp(t, "../testdata/corpus/routes-patterns-ok")
	target, err := url.Parse(r.base)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	loads := map[string]int{}
	var gets []string
	delayResults := false
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_patterns/loads", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"loads": loads, "gets": gets})
	})
	mux.HandleFunc("POST /_patterns/reset", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		loads, gets, delayResults = map[string]int{}, nil, false
	})
	mux.HandleFunc("POST /_patterns/delay", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		delayResults = true
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		loads[req.URL.Path]++
		if req.Method == http.MethodGet {
			gets = append(gets, req.URL.RequestURI())
		}
		hold := delayResults && req.URL.Path == "/results"
		if hold {
			delayResults = false
		}
		mu.Unlock()
		if hold {
			select {
			case <-time.After(800 * time.Millisecond):
			case <-req.Context().Done(): // the browser aborted it: nothing to answer
				return
			}
		}
		proxy.ServeHTTP(w, req)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cmd := exec.Command(node, script, srv.URL)
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	start := time.Now()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("patterns.cjs: %v\n%s\n%s\n%s", err, out, stderr.String(), r.errs.String())
	}
	var res struct{ Errors []string }
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("patterns.cjs printed %q (%v)\n%s", out, err, stderr.String())
	}
	for _, e := range res.Errors {
		t.Error(e)
	}
	t.Logf("patterns.cjs ran in %v", time.Since(start).Round(time.Millisecond))
}

// TestBrowser_ThemeSwitch presses the theme switch of book chapter 6 (the corpus app
// routes-theme-ok, #189) in a real browser (testdata/browser/themeswitch.cjs): the `dark` class
// on <html> turns on and off in place, with no reload, and an attribute set in the browser stays.
func TestBrowser_ThemeSwitch(t *testing.T) {
	t.Parallel()
	playwrightEnv(t) // skip before the build when there is no browser
	script := browserScript(t, "themeswitch.cjs")
	runBrowserScript(t, script, startRunApp(t, "../testdata/corpus/routes-theme-ok"))
}

// TestBrowser_ClientHtmlClassKept: the scaffold's head script sets the class dark on <html> in
// the browser (the blog's Layout, with a dark system preference); a boosted link and a Back keep
// it, since the attrs event removes only what the server set (#189;
// testdata/browser/htmlclient.cjs).
func TestBrowser_ClientHtmlClassKept(t *testing.T) {
	t.Parallel()
	playwrightEnv(t) // skip before the build when there is no browser
	script := browserScript(t, "htmlclient.cjs")
	runBrowserScript(t, script, startRunApp(t, "blog"))
}
