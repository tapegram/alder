package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/theme"
)

// themeAddr is the address the theme tests' dev server is said to listen on.
const themeAddr = "localhost:8123"

// themeServer is a dev server (no child: a build is "in progress") on a copy of the blog.
func themeServer(t *testing.T) (*devServer, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "blog")
	copyDir(t, blogDir(t), dir)
	return newDevServer(dir, "blog", themeAddr, io.Discard, io.Discard), dir
}

// themeReq is a request as the theme page sends it: same origin, JSON body for a POST.
func themeReq(method, path, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Host = themeAddr
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Origin", "http://"+themeAddr)
	return r
}

// serve answers r and decodes the JSON answer into out (when not nil).
func serve(t *testing.T, d *devServer, r *http.Request, out any) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: %d, not JSON (%v): %s", r.Method, r.URL.Path, w.Code, err, w.Body)
		}
	}
	return w
}

type writeAnswer struct {
	Written     bool       `json:"written"`
	File        string     `json:"file"`
	Error       string     `json:"error"`
	Diagnostics []diagItem `json:"diagnostics"`
}

func writeBody(t *testing.T, th theme.Theme) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"theme": th})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestThemeState_Blog(t *testing.T) {
	d, _ := themeServer(t)
	var st themeState
	if w := serve(t, d, themeReq("GET", "/_dev/theme/state", ""), &st); w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if st.Theme == nil {
		t.Fatalf("no theme: %+v", st)
	}
	if got := st.Theme.Light["primary"]; got != "oklch(0.51 0.23 277)" {
		t.Errorf("light.primary %q", got)
	}
	if got := st.Theme.Root["radius"]; got != "0.5rem" {
		t.Errorf("root.radius %q", got)
	}
	if len(st.Tokens) != len(theme.Tokens) {
		t.Errorf("%d tokens, want %d", len(st.Tokens), len(theme.Tokens))
	}
	if st.Tokens[0] != (themeToken{Field: "radius", CSS: "--radius", Kind: "length", Scope: "root"}) {
		t.Errorf("first token %+v", st.Tokens[0])
	}
	if st.Defaults["primary"].Light == "" || st.Defaults["radius"].Light == "" {
		t.Errorf("defaults by field: primary %+v, radius %+v", st.Defaults["primary"], st.Defaults["radius"])
	}
	if !st.Building || st.File != "app/App.alder" || st.Version != theme.Version() || st.Error != "" {
		t.Errorf("building %v, file %q, version %q, error %q", st.Building, st.File, st.Version, st.Error)
	}
	// the JSON keys are lowercase, and an empty map is {}, never null
	w := serve(t, d, themeReq("GET", "/_dev/theme/state", ""), nil)
	for _, want := range []string{`"field":"radius"`, `"scope":"scheme"`, `"defaults":{`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("state lacks %s: %s", want, w.Body)
		}
	}
	if err := os.WriteFile(filepath.Join(d.dir, "app", "App.alder"), []byte("module App\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w = serve(t, d, themeReq("GET", "/_dev/theme/state", ""), nil)
	if !strings.Contains(w.Body.String(), `"theme":{"root":{},"light":{},"dark":{}}`) {
		t.Errorf("an app without a theme: %s", w.Body)
	}
}

// breakApp makes App.alder not parse and returns its text.
func breakApp(t *testing.T, dir string) string {
	t.Helper()
	src := strings.Replace(readAppAlder(t, dir), "notFound = <Layout.Page", "notFound = = (<Layout.Page", 1)
	if err := os.WriteFile(filepath.Join(dir, "app", "App.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

// Review Focus 3: an App.alder that does not parse gives `theme: null` and the parse error.
func TestThemeState_ParseError(t *testing.T) {
	d, dir := themeServer(t)
	breakApp(t, dir)
	w := serve(t, d, themeReq("GET", "/_dev/theme/state", ""), nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"theme":null`) {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var st themeState
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.Error, "app/App.alder") || len(st.Tokens) != len(theme.Tokens) {
		t.Errorf("error %q, %d tokens", st.Error, len(st.Tokens))
	}
}

// Review Focus 3: a write to an App.alder that does not parse is refused with that
// diagnostic, and the file is unchanged.
func TestThemeWrite_ParseErrorRefused(t *testing.T) {
	d, dir := themeServer(t)
	before := breakApp(t, dir)
	var ans writeAnswer
	serve(t, d, themeReq("POST", "/_dev/theme/write", writeBody(t, theme.Theme{Root: map[string]string{"radius": "1rem"}})), &ans)
	if ans.Written || len(ans.Diagnostics) == 0 || !strings.Contains(ans.Error, "errors the checker cannot get past") {
		t.Fatalf("answer %+v", ans)
	}
	if dg := ans.Diagnostics[0]; dg.Code != "E0001" || dg.File != "app/App.alder" || dg.Line == 0 {
		t.Errorf("diagnostic %+v", dg)
	}
	if readAppAlder(t, dir) != before {
		t.Errorf("App.alder changed")
	}
}

func TestThemeWrite_Valid(t *testing.T) {
	d, dir := themeServer(t)
	th := theme.Theme{Root: map[string]string{"radius": "1rem"}, Light: map[string]string{"primary": "oklch(0.6 0.2 30)"}}
	var ans writeAnswer
	if w := serve(t, d, themeReq("POST", "/_dev/theme/write", writeBody(t, th)), &ans); w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if !ans.Written || ans.File != "app/App.alder" {
		t.Fatalf("answer %+v", ans)
	}
	src := readAppAlder(t, dir)
	if !strings.Contains(src, `primary: "oklch(0.6 0.2 30)"`) || !strings.Contains(src, `radius: "1rem"`) || strings.Contains(src, "0.51 0.23 277") {
		t.Errorf("App.alder:\n%s", src)
	}
}

func TestThemeWrite_Invalid(t *testing.T) {
	d, dir := themeServer(t)
	before := readAppAlder(t, dir)
	var ans writeAnswer
	serve(t, d, themeReq("POST", "/_dev/theme/write", writeBody(t, theme.Theme{Light: map[string]string{"primary": "not a colour"}})), &ans)
	if ans.Written || len(ans.Diagnostics) == 0 || ans.Diagnostics[0].Code != "E0042" || ans.Diagnostics[0].File != "app/App.alder" {
		t.Fatalf("answer %+v", ans)
	}
	if readAppAlder(t, dir) != before {
		t.Errorf("App.alder changed")
	}
}

// Review Focus 2: the page loaded the theme, then the person edited another declaration in
// their editor; the write keeps that edit (App.alder is read at write time).
func TestThemeWrite_KeepsConcurrentEdit(t *testing.T) {
	d, dir := themeServer(t)
	var st themeState
	serve(t, d, themeReq("GET", "/_dev/theme/state", ""), &st)
	edited := strings.Replace(readAppAlder(t, dir), "<h1>Something went wrong</h1>", "<h1>Something broke</h1>", 1)
	if err := os.WriteFile(filepath.Join(dir, "app", "App.alder"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	th := *st.Theme
	th.Root["radius"] = "0.75rem"
	var ans writeAnswer
	serve(t, d, themeReq("POST", "/_dev/theme/write", writeBody(t, th)), &ans)
	if !ans.Written {
		t.Fatalf("answer %+v", ans)
	}
	src := readAppAlder(t, dir)
	if !strings.Contains(src, "<h1>Something broke</h1>") || !strings.Contains(src, `radius: "0.75rem"`) {
		t.Errorf("App.alder:\n%s", src)
	}
}

// Review Focus 5: clearing every field and writing deletes the declaration.
func TestThemeWrite_EmptyDeletes(t *testing.T) {
	d, dir := themeServer(t)
	var ans writeAnswer
	serve(t, d, themeReq("POST", "/_dev/theme/write", `{"theme":{"root":{},"light":{},"dark":{}}}`), &ans)
	if !ans.Written {
		t.Fatalf("answer %+v", ans)
	}
	if src := readAppAlder(t, dir); strings.Contains(src, "theme {") {
		t.Errorf("the declaration is still there:\n%s", src)
	}
}

func TestThemeImport_Route(t *testing.T) {
	d, dir := themeServer(t)
	before := readAppAlder(t, dir)
	css, err := os.ReadFile(themeFixtures + "shadcn-v4.css")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"css": string(css) + "\n.sidebar { --primary: red }\n"})
	var ans struct {
		Theme    *theme.Theme `json:"theme"`
		Unplaced []struct {
			Block, Name, Value, Reason string
		} `json:"unplaced"`
		Skipped *struct {
			N     int      `json:"n"`
			Kinds []string `json:"kinds"`
		} `json:"skipped"`
	}
	w := serve(t, d, themeReq("POST", "/_dev/theme/import", string(body)), &ans)
	if w.Code != 200 || ans.Theme == nil || ans.Theme.Light["primary"] == "" || ans.Skipped == nil {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"block":".sidebar"`) || !strings.Contains(w.Body.String(), `"reason":`) {
		t.Errorf("unplaced: %s", w.Body)
	}
	if readAppAlder(t, dir) != before {
		t.Errorf("App.alder changed")
	}
	// no theme in the input: 422 with the importer's message
	w = serve(t, d, themeReq("POST", "/_dev/theme/import", `{"css":"body { color: red }"}`), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "no theme in the input") {
		t.Errorf("garbage: %d %s", w.Code, w.Body)
	}
}

// The gate: a cross-site request, a foreign origin, a rebound host, a body that is not JSON or
// too big are refused; the page's own request passes.
func TestThemeGate(t *testing.T) {
	d, _ := themeServer(t)
	valid := `{"theme":{"root":{"radius":"0.5rem"},"light":{"primary":"oklch(0.51 0.23 277)","primaryForeground":"oklch(0.98 0 0)"},"dark":{"primary":"oklch(0.68 0.16 277)","primaryForeground":"oklch(0.15 0 0)"}}}`
	big := `{"theme":{"root":{"radius":"` + strings.Repeat("x", 300<<10) + `"}}}`
	cases := []struct {
		name string
		edit func(r *http.Request)
		body string
		want int
	}{
		{"cross-site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, valid, 403},
		{"same-site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") }, valid, 403},
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }, valid, 403},
		{"null origin", func(r *http.Request) { r.Header.Set("Origin", "null") }, valid, 403},
		{"rebound host", func(r *http.Request) {
			r.Host = "evil.example:8123"
			r.Header.Set("Origin", "http://evil.example:8123")
		}, valid, 403},
		{"other port", func(r *http.Request) {
			r.Host = "localhost:9999"
			r.Header.Set("Origin", "http://localhost:9999")
		}, valid, 403},
		{"text/plain", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, valid, 415},
		{"300 KiB", func(r *http.Request) {}, big, 413},
		{"no fetch metadata, 127.0.0.1", func(r *http.Request) {
			r.Header.Del("Sec-Fetch-Site")
			r.Header.Del("Origin")
			r.Host = "127.0.0.1:8123"
		}, valid, 200},
		{"same-origin JSON", func(r *http.Request) {}, valid, 200},
	}
	for _, c := range cases {
		r := themeReq("POST", "/_dev/theme/write", c.body)
		c.edit(r)
		w := serve(t, d, r, nil)
		if w.Code != c.want {
			t.Errorf("%s: status %d, want %d: %s", c.name, w.Code, c.want, w.Body)
		}
	}
	// the state and the page are gated the same way
	r := themeReq("GET", "/_dev/theme/state", "")
	r.Host = "evil.example:8123"
	if w := serve(t, d, r, nil); w.Code != 403 {
		t.Errorf("rebound state: %d", w.Code)
	}
	if w := serve(t, d, themeReq("GET", "/_dev/theme", ""), nil); w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Errorf("page: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w := serve(t, d, themeReq("GET", "/_dev/theme/write", ""), nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET write: %d", w.Code)
	}
}

// A dev server listening on a named host (ALDER_ADDR=myhost.test:8123) accepts that host too.
func TestThemeGate_NamedHost(t *testing.T) {
	d := newDevServer(t.TempDir(), "x", "myhost.test:8123", io.Discard, io.Discard)
	for host, want := range map[string]bool{"myhost.test:8123": true, "localhost:8123": true, "[::1]:8123": true, "other.test:8123": false} {
		r := httptest.NewRequest("GET", "/_dev/theme/state", nil)
		r.Host = host
		if got := d.themeAllowed(r); got != want {
			t.Errorf("%s: allowed %v, want %v", host, got, want)
		}
	}
}

// building: true until the first build ends; a write sets it, and only a build that began after
// the write (the watcher's rebuild for it) clears it.
func TestThemeState_BuildingAfterWrite(t *testing.T) {
	d, _ := themeServer(t)
	building := func() bool {
		t.Helper()
		var st themeState
		serve(t, d, themeReq("GET", "/_dev/theme/state", ""), &st)
		return st.Building
	}
	if !building() {
		t.Fatal("not building before the first build")
	}
	d.endBuild(d.beginBuild())
	if building() {
		t.Fatal("building after the first build")
	}
	seen := d.beginBuild() // a rebuild already running when the write lands
	var ans writeAnswer
	serve(t, d, themeReq("POST", "/_dev/theme/write", `{"theme":{"root":{"radius":"1rem"}}}`), &ans)
	if !ans.Written || !building() {
		t.Fatalf("after the write: building %v, answer %+v", building(), ans)
	}
	d.endBuild(seen)
	if !building() {
		t.Fatal("the rebuild that began before the write cleared building")
	}
	d.endBuild(d.beginBuild()) // the watcher's rebuild for the write
	if building() {
		t.Fatal("building after the write's rebuild")
	}
}

// A write of the theme the file already holds writes nothing and says so, without an error,
// and leaves building alone.
func TestThemeWrite_Unchanged(t *testing.T) {
	d, dir := themeServer(t)
	d.endBuild(d.beginBuild())
	var st themeState
	serve(t, d, themeReq("GET", "/_dev/theme/state", ""), &st)
	before := readAppAlder(t, dir)
	w := serve(t, d, themeReq("POST", "/_dev/theme/write", writeBody(t, *st.Theme)), nil)
	if got := strings.TrimSpace(w.Body.String()); got != `{"written":false,"unchanged":true}` {
		t.Errorf("answer %s", got)
	}
	if readAppAlder(t, dir) != before {
		t.Errorf("App.alder changed")
	}
	serve(t, d, themeReq("GET", "/_dev/theme/state", ""), &st)
	if st.Building {
		t.Errorf("building after a write that changed nothing")
	}
}

// Only /_dev/theme and the paths under it are the configurator's: /_dev/themes goes to the app
// (here: no child, so the 502 page).
func TestThemeRoutePrefix(t *testing.T) {
	d, _ := themeServer(t)
	if w := serve(t, d, themeReq("GET", "/_dev/themes", ""), nil); w.Code != http.StatusBadGateway {
		t.Errorf("/_dev/themes: %d, want the proxy's 502", w.Code)
	}
}

func TestThemeState_FirstDeclaration(t *testing.T) {
	d, dir := themeServer(t)
	p := filepath.Join(dir, "app", "App.alder")
	src, _ := os.ReadFile(p)
	extra := "\ntheme {\n  light: { primary: \"oklch(0.1 0.1 10)\" },\n}\n"
	if err := os.WriteFile(p, append(src, extra...), 0o644); err != nil {
		t.Fatal(err)
	}
	st := d.themeState()
	if st.Theme == nil || st.Theme.Light["primary"] == "oklch(0.1 0.1 10)" {
		t.Errorf("theme %+v", st.Theme)
	}
}
