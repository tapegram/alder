package main

import (
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"alder/internal/diag"
	"alder/internal/generate"
	"alder/internal/syntax"
	"alder/internal/theme"
)

// The theme configurator's routes under `alder run` (#38 phase 2, spec "Routes"): the page at
// /_dev/theme, its state, the CSS importer and the write. They are the dev server's own, never
// proxied to the app, and answered while no child is up (the page needs no app state).
//
// The write changes App.alder from a browser, so every route passes the gate (themeAllowed)
// first, and a POST body is JSON (application/json) of at most themeBodyMax bytes.

// themeFile is the file the theme is read from and written to, relative to the app dir.
const themeFile = "app/App.alder"

// themeBodyMax is the limit on a POST body (and so on the importer's input): over it is 413.
const themeBodyMax = 256 << 10

// themeState is GET /_dev/theme/state.
type themeState struct {
	Version  string                   `json:"version"`
	File     string                   `json:"file"`            // "app/App.alder"
	Theme    *theme.Theme             `json:"theme"`           // nil when App does not parse
	Error    string                   `json:"error,omitempty"` // why Theme is nil
	Defaults map[string]theme.Default `json:"defaults"`        // keyed by field name; a theme-wide token's in .Light (and .Dark)
	Tokens   []themeToken             `json:"tokens"`          // theme.Tokens, in order
	Building bool                     `json:"building"`        // a build, or a write's rebuild, has not finished (devServer.building)
}

// themeToken is one row of theme.Tokens: Kind is Kind.String(), Scope "scheme" (a colour, set
// under `light` and `dark`) or "root" (theme-wide).
type themeToken struct {
	Field string `json:"field"`
	CSS   string `json:"css"`
	Kind  string `json:"kind"`
	Scope string `json:"scope"`
}

// themeUnplaced and themeSkipped are theme.Unplaced and theme.Skipped as the page reads them.
type themeUnplaced struct {
	Block  string `json:"block"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type themeSkipped struct {
	N     int      `json:"n"`
	Kinds []string `json:"kinds"`
}

// themeWritten is POST /_dev/theme/write's answer: Written and File, or the refusal (Error, its
// sentence, and the diagnostics, as the Errors tab lists them), or Unchanged.
type themeWritten struct {
	Written     bool       `json:"written"`
	Unchanged   bool       `json:"unchanged,omitempty"` // the file already holds this theme: nothing to write
	File        string     `json:"file,omitempty"`
	Error       string     `json:"error,omitempty"`
	Diagnostics []diagItem `json:"diagnostics,omitempty"`
}

func (d *devServer) serveTheme(w http.ResponseWriter, r *http.Request) {
	if !d.themeAllowed(r) {
		http.Error(w, "alder run: the theme page answers only its own origin on this dev server's address", http.StatusForbidden)
		return
	}
	route := map[string]string{
		"/_dev/theme":        http.MethodGet,
		"/_dev/theme/state":  http.MethodGet,
		"/_dev/theme/import": http.MethodPost,
		"/_dev/theme/write":  http.MethodPost,
	}
	method, ok := route[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/_dev/theme":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		d.mu.Lock()
		gen := d.gen
		d.mu.Unlock()
		_, _ = w.Write([]byte(strings.Replace(themePage, themeGenMark, url.QueryEscape(gen), 1)))
	case "/_dev/theme/state":
		writeThemeJSON(w, http.StatusOK, d.themeState())
	case "/_dev/theme/import":
		var body struct {
			CSS string `json:"css"`
		}
		if !readThemeBody(w, r, &body) {
			return
		}
		th, unplaced, skipped, err := theme.Import(body.CSS)
		if err != nil {
			writeThemeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		us := []themeUnplaced{}
		for _, u := range unplaced {
			us = append(us, themeUnplaced{u.Block, u.Name, u.Value, u.Reason})
		}
		kinds := skipped.Kinds
		if kinds == nil {
			kinds = []string{}
		}
		writeThemeJSON(w, http.StatusOK, map[string]any{"theme": themeMaps(th), "unplaced": us, "skipped": themeSkipped{skipped.N, kinds}})
	case "/_dev/theme/write":
		var body struct {
			Theme theme.Theme `json:"theme"`
		}
		if !readThemeBody(w, r, &body) {
			return
		}
		writeThemeJSON(w, http.StatusOK, d.writeTheme(body.Theme))
	}
}

// themeAllowed is the gate every /_dev/theme route passes (spec "Protecting the write"): the
// request is not a cross-site one (Sec-Fetch-Site absent, `same-origin` or `none`, the address
// typed in), its Origin, when present, is the dev server's own, and its Host names the address
// the dev server listens on: localhost, 127.0.0.1 or [::1] (or the host of d.addr, when it
// names one) with d.addr's port. A page on another site cannot reach the handler, and neither
// can one that rebinds its own name to 127.0.0.1 (its requests carry its name in Host).
func (d *devServer) themeAllowed(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return false
	}
	if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
		return false
	}
	wantHost, wantPort, err := net.SplitHostPort(d.addr)
	if err != nil {
		return false
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, port = r.Host, "80"
	}
	if port != wantPort {
		return false
	}
	host = strings.ToLower(host)
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return host == strings.ToLower(wantHost)
}

// readThemeBody decodes r's JSON body into v, or answers 415 (not application/json), 413 (over
// themeBodyMax) or 400 (not JSON) and returns false.
func readThemeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		http.Error(w, "alder run: the body must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, themeBodyMax)).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "alder run: the body is over 256 KiB", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "alder run: the body is not JSON: "+err.Error(), http.StatusBadRequest)
		}
		return false
	}
	return true
}

func writeThemeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// themeState reads App.alder's declaration from the file itself (syntax.ParseFile, not a
// Program load), so an error elsewhere in the app does not hide the theme.
func (d *devServer) themeState() themeState {
	st := themeState{Version: theme.Version(), File: themeFile, Defaults: map[string]theme.Default{}}
	defaults := theme.Defaults()
	for _, t := range theme.Tokens {
		scope := "root"
		if t.Kind == theme.Colour {
			scope = "scheme"
		}
		st.Tokens = append(st.Tokens, themeToken{t.Field, t.CSS, t.Kind.String(), scope})
		if v, ok := defaults[t.CSS]; ok {
			st.Defaults[t.Field] = v
		}
	}
	d.mu.Lock()
	st.Building = d.building
	d.mu.Unlock()

	src, err := os.ReadFile(filepath.Join(d.dir, filepath.FromSlash(themeFile)))
	if err != nil {
		st.Error = "`" + themeFile + "` cannot be read: " + err.Error()
		return st
	}
	m, ds := syntax.ParseFile(themeFile, src)
	if e := firstError(ds); e != nil || m == nil {
		st.Error = "`" + themeFile + "` does not parse"
		if e != nil {
			st.Error = diagItemOf(*e).where() + ": " + e.Message
		}
		return st
	}
	var decl *syntax.ThemeDecl
	for _, dc := range m.Decls {
		if td, ok := dc.(*syntax.ThemeDecl); ok {
			decl = td
		}
	}
	th := themeMaps(generate.ThemeOf(decl))
	st.Theme = &th
	return st
}

// writeTheme is the write: generate.WriteTheme on the app dir (App.alder read now, so an edit
// made in an editor since the page loaded is kept), its refusal as the page shows it. Writes
// are one at a time (two tabs), and a write that changes the file sets building until the
// rebuild the watcher starts for it ends.
func (d *devServer) writeTheme(th theme.Theme) themeWritten {
	d.themeMu.Lock()
	defer d.themeMu.Unlock()
	res, err := generate.WriteTheme(generate.App{Dir: d.dir, Migrate: migrateFor}, th, false)
	if err == nil && res.Written {
		if len(res.Files) == 0 {
			return themeWritten{Unchanged: true}
		}
		d.mu.Lock()
		d.writes++
		d.building = true
		d.mu.Unlock()
		return themeWritten{Written: true, File: themeFile}
	}
	out := themeWritten{}
	var ds []diag.Diagnostic
	var invalid *generate.ThemeInvalid
	var refusal *generate.Refusal
	switch {
	case errors.As(err, &invalid):
		out.Error = "the theme does not check: nothing was written"
		ds = invalid.Diags
	case errors.As(err, &refusal):
		out.Error = refusal.Msg
		ds = refusal.Diags
		if len(ds) == 0 && refusal.Result != nil {
			ds = refusal.Result.Diagnostics
		}
		if refusal.AppErrors {
			out.Error = "the theme cannot be written while the app has errors the checker cannot get past (the errors below): fix them, then write again"
		}
	case err != nil:
		out.Error = err.Error()
	default:
		out.Error = "nothing was written"
		ds = res.Diagnostics
	}
	for _, dg := range ds {
		if dg.Severity == "error" {
			it := diagItemOf(dg)
			it.Excerpt = excerpt(d.dir, it)
			out.Diagnostics = append(out.Diagnostics, it)
		}
	}
	return out
}

// firstError is the first error among ds, nil for none.
func firstError(ds []diag.Diagnostic) *diag.Diagnostic {
	for i := range ds {
		if ds[i].Severity == "error" {
			return &ds[i]
		}
	}
	return nil
}

// themeMaps is th with every nil map made empty, so the JSON has {} and never null.
func themeMaps(th theme.Theme) theme.Theme {
	for _, m := range []*map[string]string{&th.Root, &th.Light, &th.Dark} {
		if *m == nil {
			*m = map[string]string{}
		}
	}
	return th
}
