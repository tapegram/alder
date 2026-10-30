package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rtpkg "alder/rt"
	"alder/rt/al"
	"alder/rt/datastar"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/html"
)

// Options configures NewRouter. The generated main fills it from the App module and the
// environment.
type Options struct {
	Store       eff.Store               // db store (db.Store); nil only for apps without tables
	Caps        map[string]any          // capability implementations, copied into every Env
	Now         func() time.Time        // nil: time.Now
	Log         func(level, msg string) // nil: no log
	Dev         bool                    // dev mode: error text is appended to error pages
	Secret      []byte                  // cookie key, at least 16 bytes; in dev mode a shorter one → DevKey + a warning
	Session     SessionCodec            // generated JSON codec of the App session type
	StaticDir   string                  // served under /static/; "" → none
	ErrorDoc    func(err error) html.Node
	NotFoundDoc func() html.Node
	Login       string   // App.login page path, copied to Request.Login
	DevMap      string   // the generator's JSON map of the app (pages, tables, session), shown by /_dev under ALDER_DEV=1 only
	Migrations  []string // the migrations the database has (names), shown by /_dev
	Reload      string   // under `alder run`, the build generation (ALDER_RELOAD); non-empty: every page carries the dev server's overlay and reload subscriber (withReload)
	// EnvFor, when set, makes each request's Env instead of the router: it is called with the
	// request's context and its decoded session, and the router then sets the Env's Session to
	// that session. An Alder test's browser (rt/testing) supplies one sharing the test's Store,
	// capabilities, clock and log, running Within the test's transaction. nil: a fresh Env from
	// Store, Caps, Now and Log, as in production.
	EnvFor func(ctx context.Context, session any) *eff.Env
	// Served, when set, is called with each request once its handler has answered, before the
	// answer is written: an Alder test's browser reads the model a page's GET rendered
	// (Request.Model) from it. nil in production.
	Served func(rq *Request)
	// Silos are the app's `silo` declarations (generated Web: an Alder test makes its stores
	// from them, db.NewTestStores) and Stores its stores (Serve: db.OpenStores), put in every
	// request's Env (eff.Env.Stores); nil for an app without silos.
	Silos  []db.Silo
	Stores *db.Stores
	// SessionCheck is every request's eff.Env.CheckSession: an app whose session has a `via`
	// field checks the membership of a Session.put (generated Web); nil otherwise.
	SessionCheck func(e *eff.Env, old, new any) (any, error)
	// MaintenanceDoc is the document of a request on a silo that is not served (App.maintenance;
	// nil: a default page), answered with status 503 (MaintenanceResponse).
	MaintenanceDoc func() html.Node
	// Build is the build's id (ALDER_BUILD, set by the deploy), answered by /_alder/health.
	Build string
	// SharedMaintenance, when set, is why the shared database cannot be served by this build
	// (Serve: on Turso, a newer build migrated it, "ahead: <detail>"): every route answers the
	// maintenance page (MaintenanceDoc) with 503, and /_alder/health 503 with its text.
	SharedMaintenance error
}

// maxBody is the largest POST body the router reads.
const maxBody = 1 << 20

// staticPrefix is where Options.StaticDir is served.
const staticPrefix = "/static/"

type router struct {
	routes []route
	opts   Options
	key    []byte
	log    func(level, msg string)
	now    func() time.Time
	static http.Handler // nil without a StaticDir
	script http.Handler

	bigCookie sync.Once // the oversized-cookie warning is logged once per router

	// healthDown is whether the last health check failed its SELECT 1: the error is logged
	// when it starts failing and the recovery when it answers again, not on every check
	healthDown atomic.Bool
}

// sessionMaxAge is the session cookie's lifetime in seconds: 30 days from the time the session
// was issued (signed into it, sessionEnvelope) — a login or any other change of the session.
// The lifetime is absolute, not sliding: the router stores the cookie only when a response
// changes the session, so visiting does not renew it, and a role written into the session at
// login applies until logout or 30 days after the login, whatever the user does in between.
const sessionMaxAge = 30 * 24 * 60 * 60

// tracker records whether anything has been written to the client, so a panic after the
// response has started is only logged (writing an error page then would corrupt it).
type tracker struct {
	http.ResponseWriter
	started bool

	// dev panel: the request, what it ran, how it was answered
	r       *http.Request
	rq      *Request
	start   time.Time
	answer  string
	detail  string
	patches []string
	notice  string // a Noticed answer's toast text, patched in after the body
	// hops are the redirects follow served in this same answer, after the request the client
	// sent (whose rq and recorder the panel keeps): each one's URL, where its steps and
	// statements start in the recorder, and how it was answered
	hops []hopMark
	// alert is a Noticed answer's text for a request without Datastar (no toast without the
	// script): rendered as an <al-alert> at the top of the page (document)
	alert string
}

type hopMark struct {
	url, answer string
	steps, sql  int
}

// setAnswer records how the request was answered for the panel: the first request's answer is
// set by it alone; a redirect follow serves records its own on its hop.
func (t *tracker) setAnswer(a string) {
	if n := len(t.hops); n > 0 {
		t.hops[n-1].answer = a
		return
	}
	t.answer = a
}

func (t *tracker) WriteHeader(code int) {
	t.started = true
	t.ResponseWriter.WriteHeader(code)
}

func (t *tracker) Write(b []byte) (int, error) {
	t.started = true
	return t.ResponseWriter.Write(b)
}

// Flush lets datastar.PatchBody flush through the tracker.
func (t *tracker) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type route struct {
	method string
	segs   []segment
	file   bool // Route.File
	handle func(*Request) Response
}

// segment is a literal path segment, or a parameter when param is set.
type segment struct {
	lit   string
	param string
}

// NewRouter returns the app's http.Handler. For each request it: recovers panics (→ 500 via
// ErrorDoc); serves the Datastar script at datastar.ScriptPath (versioned, immutable),
// Options.StaticDir under /static/, and the design system at /static/al/ (DesignSystemCache, ETag); matches method and pattern, the most specific pattern first (a literal segment
// before a parameter, left to right; equally specific ones in declaration order), a trailing
// slash tolerated (anything unmatched, including a GET to a POST-only path, is a 404 rendered
// with NotFoundDoc); reads a POST body
// (at most 1 MiB, else 413; url-encoded forms are parsed, multipart is refused with 415);
// decodes the session cookie; builds the Request and its Env; calls the route's Handle; and
// writes the Response. Patterns are compiled here; a malformed one panics, as does a Secret
// shorter than 16 bytes outside dev mode (CheckSecret: the generated Serve reports it first).
//
// Under Datastar (header Datastar-Request: true) a 404 or 500 is answered like a re-rendered
// page: status 200 and an SSE patch of the NotFound/Error document's <body>, because the
// Datastar client ignores other answers. A GET under Datastar is a boosted navigation (a link
// of the page, datastar.BoostTag, or a back/forward): the page's <body> patched in (morphed),
// its <title>, and a history step to the URL (push, or replace for a back/forward). A Redirect
// to a page of this app is followed by the router itself (follow): the answer is the target's
// morph and push; one to another site is a 200 SSE event that navigates the browser there
// (datastar.Navigate), because the client's fetch would follow a 303 itself and leave the
// address bar and history at the old URL.
//
// The router's only mutable state is a once-only warning and the health check's last outcome
// (an atomic): it is safe for concurrent requests.
func NewRouter(routes []Route, opts Options) http.Handler {
	rt := &router{opts: opts, key: opts.Secret, log: opts.Log, now: opts.Now, script: datastar.Handler()}
	if rt.log == nil {
		rt.log = func(string, string) {}
	}
	if rt.now == nil {
		rt.now = time.Now
	}
	if err := CheckSecret(opts.Secret, opts.Dev); err != nil {
		panic("web: " + err.Error())
	}
	if len(rt.key) < minSecret {
		rt.key = DevKey()
		rt.log("warn", devWarning)
	}
	if opts.StaticDir != "" {
		rt.static = http.StripPrefix(strings.TrimSuffix(staticPrefix, "/"), http.FileServer(filesOnly{http.Dir(opts.StaticDir)}))
	}
	for _, r := range routes {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			panic(fmt.Sprintf("web: route %q: method %q is not GET or POST", r.Pattern, r.Method))
		}
		rt.routes = append(rt.routes, route{method: r.Method, segs: compile(r.Pattern), file: r.File, handle: r.Handle})
	}
	// the most specific pattern first: /x/new before /x/{id}, whatever the declaration order
	slices.SortStableFunc(rt.routes, func(a, b route) int { return strings.Compare(a.shape(), b.shape()) })
	return rt
}

// shape orders routes by specificity: one byte per segment, a literal ('a') before a parameter
// ('b'), left to right. Two patterns that can match the same path have the same length, so the
// first segment where they differ in kind decides; equal shapes keep the declaration order.
func (r *route) shape() string {
	b := make([]byte, len(r.segs))
	for i, s := range r.segs {
		b[i] = 'a'
		if s.param != "" {
			b[i] = 'b'
		}
	}
	return string(b)
}

func compile(pattern string) []segment {
	if !strings.HasPrefix(pattern, "/") {
		panic(fmt.Sprintf("web: pattern %q does not start with /", pattern))
	}
	p := strings.TrimSuffix(pattern[1:], "/")
	if p == "" {
		return nil
	}
	var segs []segment
	for _, s := range strings.Split(p, "/") {
		switch {
		case s == "":
			panic(fmt.Sprintf("web: pattern %q has an empty segment", pattern))
		case strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && len(s) > 2:
			segs = append(segs, segment{param: s[1 : len(s)-1]})
		default:
			segs = append(segs, segment{lit: s})
		}
	}
	return segs
}

// match reports whether the path segments fit the pattern, and its params.
func (r *route) match(segs []string) (map[string]string, bool) {
	if len(segs) != len(r.segs) {
		return nil, false
	}
	params := map[string]string{}
	for i, s := range r.segs {
		switch {
		case s.param != "":
			if segs[i] == "" {
				return nil, false
			}
			params[s.param] = segs[i]
		case s.lit != segs[i]:
			return nil, false
		}
	}
	return params, true
}

// pathSegments splits the request path into unescaped segments, ignoring one trailing slash.
// ok is false for a path that cannot be unescaped.
func pathSegments(r *http.Request) ([]string, bool) {
	return splitPath(r.URL.EscapedPath())
}

// splitPath splits an escaped path into unescaped segments, ignoring one trailing slash. ok is
// false for a path that cannot be unescaped.
func splitPath(escaped string) ([]string, bool) {
	p := strings.TrimSuffix(strings.TrimPrefix(escaped, "/"), "/")
	if p == "" {
		return nil, true
	}
	raw := strings.Split(p, "/")
	segs := make([]string, len(raw))
	for i, s := range raw {
		u, err := url.PathUnescape(s)
		if err != nil {
			return nil, false
		}
		segs[i] = u
	}
	return segs, true
}

func (rt *router) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w := &tracker{ResponseWriter: rw, r: r, start: time.Now()}
	st := hop{origin: r}
	defer func() {
		if v := recover(); v != nil {
			if v == http.ErrAbortHandler {
				panic(v)
			}
			err := fmt.Errorf("panic: %v", v)
			// The stack goes to the log only, never to the page.
			if w.started {
				rt.log("error", fmt.Sprintf("%s %s: %v (after the response started)\n%s", r.Method, r.URL.Path, err, debug.Stack()))
				return
			}
			rt.fail(w, r, st, html.None(), err, debug.Stack())
		}
	}()

	get := r.Method == http.MethodGet || r.Method == http.MethodHead
	if get && r.URL.Path == HealthPath {
		rt.health(rw, r)
		return
	}
	if get && datastar.IsDatastar(r) && (strings.HasPrefix(r.URL.Path, staticPrefix) || strings.HasPrefix(r.URL.Path, alderPrefix)) {
		// a file, not a page: a boost of an older runtime (or a hand-made Datastar GET) asked
		// for it to morph in; the browser loads it itself instead
		rt.reload(w, r)
		return
	}
	if get && datastar.IsDatastar(r) {
		if b := r.Header.Get(datastar.BuildHeader); b != "" && b != BuildID() {
			// a tab of another build: its <head> (stylesheet, scripts, boost) is that build's,
			// and a morph keeps it; a real load of the URL brings this build's
			rt.reload(w, r)
			return
		}
	}
	if get && r.URL.Path == "/_dev" && rt.devOn() {
		rt.devMap(w, r)
		return
	}
	if get && r.URL.Path == datastar.ScriptPath {
		rt.script.ServeHTTP(w, r)
		return
	}
	if get && rt.static != nil && strings.HasPrefix(r.URL.Path, staticPrefix) {
		rt.serveStatic(w, r)
		return
	}
	if rt.opts.SharedMaintenance != nil {
		rt.maintenance(w, r, st, rt.opts.SharedMaintenance)
		return
	}
	if get && datastar.IsDatastar(r) {
		// a boosted link or a back/forward: answered with a morph and a history step
		dropSignals(r.URL)
		st.gets, st.url, st.mode = 1, r.URL.RequestURI(), datastar.Push
		if r.Header.Get(datastar.HistoryHeader) == "replace" {
			st.mode = datastar.Replace
		}
	}
	rt.serve(w, r, st)
	rt.warnRepeats(w)
}

// alderPrefix is the runtime's own files' path (the Datastar script, datastar.ScriptPath).
const alderPrefix = "/_alder/"

// HealthPath is the health check every app answers (GET or HEAD), whatever its routes.
const HealthPath = alderPrefix + "health"

// healthTimeout bounds the health check's SELECT 1: a database that does not answer in time is
// down (503), not a check left hanging.
const healthTimeout = 5 * time.Second

// health answers HealthPath: 200 {"build":"<Options.Build>","db":"ok"} when the store answers
// SELECT 1 (within healthTimeout), else 503 with "db": "unreachable" — a fixed category: the
// route is public, and the driver's error names the database's host and the upstream's answer;
// the error itself goes to the log at warn, once when the check starts failing (and a warn
// line once it answers again), not on every check. When Options.SharedMaintenance is set, 503
// with its text as "db" ("ahead: <detail>", the schema check's own sentence: migration names,
// no host). A platform's health check (Render's) sends it: a build that cannot reach its
// database, or that a newer build's migrations left behind, is never switched to. It is answered
// before anything else the router does: no session is read or set, no CSRF check, no request
// log line, no dev panel; never cached.
func (rt *router) health(w http.ResponseWriter, r *http.Request) {
	status, state := http.StatusOK, "ok"
	if err := rt.opts.SharedMaintenance; err != nil {
		status, state = http.StatusServiceUnavailable, err.Error()
	} else if p, ok := rt.opts.Store.(interface{ Ping(context.Context) error }); ok {
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()
		if err := p.Ping(ctx); err != nil {
			status, state = http.StatusServiceUnavailable, "unreachable"
			if !rt.healthDown.Swap(true) {
				rt.log("warn", "health: the database does not answer: "+err.Error())
			}
		} else if rt.healthDown.Swap(false) {
			rt.log("warn", "health: the database answers again")
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Build string `json:"build"`
		DB    string `json:"db"`
	}{rt.opts.Build, state})
}

// reload answers a Datastar GET with a real navigation to its URL (datastar.Assign): the
// browser loads it as a document (a file under /static/ or /_alder/; a page asked for by a tab
// of another build). Asked from a back/forward, the URL is the one the tab shows, which a
// browser loads in place of the entry rather than as a new one.
func (rt *router) reload(w http.ResponseWriter, r *http.Request) {
	dropSignals(r.URL)
	if err := datastar.Navigate(w, r.URL.RequestURI(), datastar.Assign); err != nil {
		rt.log("error", fmt.Sprintf("%s %s: writing the navigation: %v", r.Method, r.URL.Path, err))
	}
}

// hop is how a request is being served: origin is the request the client sent (the log line
// of a redirect loop names it); for a Datastar GET — a boosted link, a back/forward, or the
// target of a redirect the router follows (follow) — url is the URL the answer records in the
// history with mode (datastar.Push or Replace), and gets counts the GETs of the chain so far,
// this one included. url is "" for any other request (a POST, a plain GET).
type hop struct {
	origin *http.Request
	url    string
	mode   datastar.History
	gets   int
	// session, when has is set, is the session the request runs with instead of the cookie's:
	// the one the redirect being followed stored (in this same answer's Set-Cookie), with its
	// encoding.
	has     bool
	session any
	enc     []byte
}

// serve routes r (not the Datastar script, not static/) and writes the handler's answer.
func (rt *router) serve(w http.ResponseWriter, r *http.Request, st hop) {
	method := r.Method
	if method == http.MethodHead {
		method = http.MethodGet
	}
	var hit *route
	var params map[string]string
	if segs, ok := pathSegments(r); ok {
		hit, params = rt.find(method, segs)
	}
	if hit == nil {
		rt.notFound(w, r, st, html.None())
		return
	}
	if why := badQuery(r.URL.RawQuery); why != "" {
		rt.badRequest(w, r, st, why)
		return
	}
	if hit.file && datastar.IsDatastar(r) {
		// a file is a document of its own: a real navigation, its load not run. Not rt.reload:
		// ServeHTTP has already dropped the boost's signals parameter from r.URL.
		if err := datastar.Navigate(w, r.URL.RequestURI(), datastar.Assign); err != nil {
			rt.log("error", fmt.Sprintf("%s %s: writing the navigation: %v", r.Method, r.URL.Path, err))
		}
		return
	}

	rq := &Request{
		Ctx:      r.Context(),
		HTTP:     r,
		Params:   params,
		Query:    r.URL.Query(),
		Datastar: datastar.IsDatastar(r),
		Dev:      rt.opts.Dev,
		Login:    rt.opts.Login,
	}
	if method == http.MethodPost {
		form, status := readForm(w, r)
		if status != 0 {
			http.Error(w, http.StatusText(status), status)
			return
		}
		rq.Form = form
	}
	if st.has {
		rq.Session, rq.sessionEnc = st.session, st.enc
	} else {
		rq.Session, rq.sessionEnc = rt.decodeSession(r)
	}
	if rt.opts.EnvFor != nil {
		rq.Env = rt.opts.EnvFor(r.Context(), rq.Session)
		rq.Env.Session = rq.Session
	} else {
		rq.Env = &eff.Env{
			Ctx:     r.Context(),
			Store:   rt.opts.Store,
			Caps:    rt.opts.Caps,
			Now:     rt.now,
			Log:     rt.log,
			Session: rq.Session,
		}
		if rt.opts.Stores != nil {
			rq.Env.Stores = rt.opts.Stores
		}
		rq.Env.CheckSession = rt.opts.SessionCheck
	}
	if tw, ok := w.(*tracker); ok {
		if !rq.Datastar && r.Method == http.MethodGet && !hit.file { // an HTML page GET only: not a HEAD, which shows nothing, nor a file, which shows no notice
			tw.alert = rt.takeNotice(w, r)
		}
		if rt.panelOn() {
			if tw.rq == nil {
				rq.Env.Rec = &eff.Recorder{}
				tw.rq = rq
			} else { // a redirect follow serves: its steps go on in the first request's trace
				rec := tw.rq.Env.Rec
				rq.Env.Rec = rec
				tw.hops = append(tw.hops, hopMark{url: r.URL.RequestURI(), steps: len(rec.Steps), sql: len(rec.SQL)})
			}
		}
	}
	resp := hit.handle(rq)
	if rt.opts.Served != nil {
		rt.opts.Served(rq)
	}
	rt.write(w, r, st, rq, resp)
}

// find is the route serve picks for method and the path's segments (the routes are sorted most
// specific first), and its params; nil when none matches.
func (rt *router) find(method string, segs []string) (*route, map[string]string) {
	for i := range rt.routes {
		if rt.routes[i].method != method {
			continue
		}
		if p, ok := rt.routes[i].match(segs); ok {
			return &rt.routes[i], p
		}
	}
	return nil, nil
}

// readForm reads a POST body: url-encoded forms are parsed, any other body except multipart
// gives an empty form (a Datastar post without a form sends its signals as JSON). status is
// non-zero on failure: 413 too large, 415 multipart, 400 malformed.
func readForm(w http.ResponseWriter, r *http.Request) (url.Values, int) {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch {
	case ct == "application/x-www-form-urlencoded":
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		if err := r.ParseForm(); err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				return nil, http.StatusRequestEntityTooLarge
			}
			return nil, http.StatusBadRequest
		}
		return r.PostForm, 0
	case strings.HasPrefix(ct, "multipart/"):
		return nil, http.StatusUnsupportedMediaType
	default:
		return url.Values{}, 0
	}
}

// decodeSession is the session in r's cookie, or the codec's Default when it is missing, not
// signed with the key, expired, without an issued time, or not decodable; and the codec's
// encoding of it, which setSession compares a response's session with. When the request
// carries several alder_session cookies, only the first is considered.
func (rt *router) decodeSession(r *http.Request) (any, []byte) {
	codec := rt.opts.Session
	if codec == nil {
		return nil, nil
	}
	s := codec.Default()
	if c, err := r.Cookie(CookieName); err == nil {
		if cs, ok := SessionFromCookie(rt.key, codec, c.Value, rt.now()); ok {
			s = cs
		}
	}
	enc, err := codec.Encode(s)
	if err != nil {
		return s, nil // stored again by setSession, which reports the error
	}
	return s, enc
}

// SessionFromCookie is the session a cookie value made by SessionCookie carries, decoded with
// codec, and whether it is valid at now: signed with key, issued less than sessionMaxAge before
// now, and with a session the codec decodes.
func SessionFromCookie(key []byte, codec SessionCodec, value string, now time.Time) (any, bool) {
	payload, ok := Verify(key, value)
	if !ok {
		return nil, false
	}
	var env sessionEnvelope
	if json.Unmarshal(payload, &env) != nil || env.Iss == nil || now.Unix() >= *env.Iss+sessionMaxAge || env.Session == nil {
		return nil, false
	}
	s, err := codec.Decode(env.Session)
	if err != nil {
		return nil, false
	}
	return s, true
}

// SessionCookie is the value of the session cookie storing s at now: the codec's encoding of s
// and now as its issued time (it expires sessionMaxAge later), signed with key. An Alder test's Test.login writes it
// into its browser's cookie jar the way the router would.
func SessionCookie(key []byte, codec SessionCodec, s any, now time.Time) (string, error) {
	enc, err := codec.Encode(s)
	if err != nil {
		return "", fmt.Errorf("encoding the session: %w", err)
	}
	iss := now.Unix()
	payload, err := json.Marshal(sessionEnvelope{Iss: &iss, Session: enc})
	if err != nil {
		return "", fmt.Errorf("encoding the session: %w", err)
	}
	return Sign(key, payload), nil
}

// sessionEnvelope is the signed cookie payload: the codec's encoding of the session and the time
// it was issued (unix seconds: when a response last changed the session). The router refuses a
// cookie sessionMaxAge or more after it was issued, or without an issued time, whatever the
// browser does with the cookie's Max-Age: a copied cookie stops working 30 days after the
// session last changed, however often it is sent.
type sessionEnvelope struct {
	Iss     *int64          `json:"iss"`
	Session json.RawMessage `json:"session"`
}

// setSession adds the Set-Cookie header storing s, issued now and valid for sessionMaxAge, when s
// differs from the session the request carried (rq.sessionEnc) or a Session.put ran
// (rq.Env.SessionPut: a login as the user already signed in is issued anew too); an unchanged
// session is not stored, so the cookie keeps its issued time and its expiry (no sliding
// renewal). It returns an error when s cannot be encoded (a bug in the generated codec).
func (rt *router) setSession(w http.ResponseWriter, r *http.Request, rq *Request, s any) error {
	if s == nil || rt.opts.Session == nil {
		return nil
	}
	put := rq.Env != nil && rq.Env.SessionPut
	if enc, err := rt.opts.Session.Encode(s); err == nil && !put && rq.sessionEnc != nil && bytes.Equal(enc, rq.sessionEnc) {
		return nil
	}
	now := rt.now()
	value, err := SessionCookie(rt.key, rt.opts.Session, s, now)
	if err != nil {
		return err
	}
	// Secure is set for TLS requests and behind a proxy that says https. X-Forwarded-Proto
	// is client-supplied, but it can only add the flag (making the cookie stricter), never
	// remove it.
	c := &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   sessionMaxAge,
		Expires:  now.Add(sessionMaxAge * time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	}
	if len(c.Value) > 4000 {
		n := len(c.Value)
		rt.bigCookie.Do(func() {
			rt.log("warn", fmt.Sprintf("the session cookie is %d bytes; browsers may drop cookies over 4096", n))
		})
	}
	http.SetCookie(w, c)
	return nil
}

func (rt *router) write(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	if resp.Raw != nil {
		rt.writeFile(w, r, st, rq, resp)
		return
	}
	if tw, ok := w.(*tracker); ok {
		if rq.Notice != "" && rq.Datastar {
			tw.notice = rq.Notice
		} else if rq.Notice != "" {
			tw.alert = rq.Notice // a Rerender's page shows it; a Redirect's stores it (below)
		}
		switch {
		case resp.Status == http.StatusSeeOther:
			tw.setAnswer("redirect " + resp.Location)
		case resp.Status == http.StatusOK && !resp.Patch: // a document, or a boosted GET's morph of one
			tw.setAnswer("page")
		case resp.Status == http.StatusOK:
			tw.setAnswer("rerender")
		case resp.Status == http.StatusNotFound:
			tw.setAnswer("not found")
		default:
			tw.setAnswer("error")
		}
	}
	switch resp.Status {
	case http.StatusOK, http.StatusSeeOther:
		if err := rt.setSession(w, r, rq, resp.Session); err != nil {
			rt.fail(w, r, st, html.None(), err, nil)
			return
		}
		if resp.Status == http.StatusSeeOther {
			if !rtpkg.SafeURL(resp.Location) {
				// defense in depth: Url.external already turns such a URL into "#"
				rt.fail(w, r, st, html.None(), fmt.Errorf("%w: refusing to navigate to %s", eff.ErrInternal, rtpkg.URLScheme(resp.Location)), nil)
				return
			}
			if rq.Datastar && rt.followable(resp.Location) {
				// a page of this app: serve it here and morph it in (follow)
				rt.follow(w, r, st, rq, resp)
				return
			}
			if rq.Datastar {
				// another site: the client's fetch would follow a 303 itself and patch the new
				// page into the old URL; navigate the browser instead
				if err := datastar.Navigate(w, resp.Location, datastar.Assign); err != nil {
					rt.log("error", fmt.Sprintf("%s %s: writing the navigation: %v", r.Method, r.URL.Path, err))
				}
				return
			}
			if tw, ok := w.(*tracker); ok && tw.alert != "" {
				rt.storeNotice(w, r, tw.alert) // the next page shows it (takeNotice)
			}
			w.Header().Set("Location", resp.Location)
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		page := html.RenderWith(resp.Node, resp.State)
		if st.url != "" {
			rt.morph(w, r, st, http.StatusOK, page)
			return
		}
		if resp.Patch && rq.Datastar {
			if err := datastar.PatchBody(w, rt.shellBody(w, rt.dev(bodyOf(page)))); err != nil {
				rt.log("error", fmt.Sprintf("%s %s: writing the patch: %v", r.Method, r.URL.Path, err))
			}
			if tw, ok := w.(*tracker); ok {
				tw.patches = append(tw.patches, "body")
			}
			rt.notify(w)
			rt.patchPanel(w, http.StatusOK)
			return
		}
		rt.document(w, http.StatusOK, page)
	case http.StatusServiceUnavailable:
		rt.maintenance(w, r, st, resp.Err)
	case http.StatusNotFound, http.StatusInternalServerError:
		// a non-nil session here is one a successful Task left before a later step failed
		// (the re-load after update): it is stored all the same
		if err := rt.setSession(w, r, rq, resp.Session); err != nil {
			rt.log("error", fmt.Sprintf("%s %s: %v", r.Method, r.URL.Path, err))
		}
		if resp.Status == http.StatusNotFound {
			rt.notFound(w, r, st, resp.Node)
		} else {
			rt.fail(w, r, st, resp.Node, resp.Err, nil)
		}
	default:
		rt.fail(w, r, st, html.None(), fmt.Errorf("handler returned a response with status %d", resp.Status), nil)
	}
}

// headerNewlines turns CR and LF in a file's header value into spaces: a value cannot end its
// header line and start another (net/http does the same when it writes; this does not rely on it).
var headerNewlines = strings.NewReplacer("\r", " ", "\n", " ")

// writeFile writes a file page's answer as given: allow-listed headers (others dropped, with a
// warn line), Content-Type (application/octet-stream when empty), the status, the body (none for
// 204, 304 or a HEAD). No doctype, head tags, panel, Alder-Status or morph. A status outside
// 200..599 is the app's 500 page.
func (rt *router) writeFile(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	f := resp.Raw
	if resp.Status < 200 || resp.Status > 599 {
		rt.fail(w, r, st, html.None(), fmt.Errorf("a file page answered status %d; a file's status is 200 to 599", resp.Status), nil)
		return
	}
	if err := rt.setSession(w, r, rq, resp.Session); err != nil {
		rt.fail(w, r, st, html.None(), err, nil)
		return
	}
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer(fmt.Sprintf("file %d %s %dB", resp.Status, ct, len(f.Body)))
	}
	hd := w.Header()
	for _, h := range f.Headers {
		name, ok := rtpkg.FileHeaderAllowed(h[0])
		if !ok {
			rt.log("warn", fmt.Sprintf("%s: header %q cannot be set from a page; dropped", r.URL.Path, h[0]))
			continue
		}
		if strings.HasPrefix(name, "X-") { // an X- header may repeat; the others are single-valued: the last wins
			hd.Add(name, headerNewlines.Replace(h[1]))
		} else {
			hd.Set(name, headerNewlines.Replace(h[1]))
		}
	}
	hd.Set("Content-Type", headerNewlines.Replace(ct))
	w.WriteHeader(resp.Status)
	if resp.Status != http.StatusNoContent && resp.Status != http.StatusNotModified && r.Method != http.MethodHead {
		_, _ = w.Write(f.Body)
	}
}

// redirectLimit is how many redirects in a row follow serves before it gives up: a GET of the
// chain answering a redirect when redirectLimit GETs are already behind it is a 500, as a test's
// browser (rt/testing) counts them.
const redirectLimit = 10

// follow answers a Datastar request whose handler redirected to resp.Location, a page of this
// app, with that page itself: it serves a Datastar GET of the Location in-process, with the
// session the redirect left (stored by this answer's cookie), and writes its answer — the morph
// of its body, its title and a history push of the Location (Replace when the request was a
// back/forward's) — or follows its own redirect in turn. The browser makes one request instead
// of two, and the page changes without a load (no flash, no stylesheet reloaded). A chain of
// more than redirectLimit redirects is a 500 logged for the request the client sent.
func (rt *router) follow(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	if st.gets > redirectLimit {
		rt.fail(w, st.origin, hop{}, html.None(), fmt.Errorf("more than %d redirects in a row (the last to %s)", redirectLimit, resp.Location), nil)
		return
	}
	u, err := r.URL.Parse(resp.Location)
	if err != nil {
		rt.fail(w, r, st, html.None(), err, nil)
		return
	}
	next := hop{origin: st.origin, url: resp.Location, mode: st.mode, gets: st.gets + 1, has: true, session: rq.Session, enc: rq.sessionEnc}
	if st.url == "" {
		next.mode = datastar.Push
	}
	if resp.Session != nil {
		next.session, next.enc = resp.Session, nil
		if rt.opts.Session != nil {
			next.enc, _ = rt.opts.Session.Encode(resp.Session)
		}
	}
	r2 := r.Clone(r.Context())
	r2.Method, r2.URL, r2.RequestURI = http.MethodGet, u, u.RequestURI()
	r2.Body, r2.ContentLength, r2.Form, r2.PostForm = http.NoBody, 0, nil, nil
	r2.Header.Del("Content-Type")
	r2.Header.Del("Content-Length")
	rt.serve(w, r2, next)
}

// followable reports whether a redirect to loc is to a page of this app the router can serve
// itself (follow): an absolute path ("/…", not "//…"), not the Datastar script, a file under
// /static/, nor a file page's (Route.File) — a file is a document of its own, never morphed in.
// Anything else — another site, "#" (an unsafe URL Url.external neutralised) — is navigated to
// by the browser.
func (rt *router) followable(loc string) bool {
	if !strings.HasPrefix(loc, "/") || strings.HasPrefix(loc, "//") || strings.HasPrefix(loc, "/\\") {
		return false
	}
	p, _, _ := strings.Cut(loc, "?")
	p, _, _ = strings.Cut(p, "#")
	if p == datastar.ScriptPath || strings.HasPrefix(p, staticPrefix) {
		return false
	}
	return !rt.isFileRoute(p)
}

// isFileRoute reports whether the route serve picks for a GET of escaped path p is a file page's.
func (rt *router) isFileRoute(p string) bool {
	segs, ok := splitPath(p)
	if !ok {
		return false
	}
	hit, _ := rt.find(http.MethodGet, segs)
	return hit != nil && hit.file
}

// dropSignals removes the `datastar` query parameter Datastar adds to every GET — its signals,
// as JSON, which for the boost's GETs is the empty payload {} — from u, leaving the other
// parameters as they were, in their order. Only that one is dropped: a `datastar` parameter of
// the page's own URL (/search?datastar=x) is kept.
func dropSignals(u *url.URL) {
	if u.RawQuery == "" {
		return
	}
	var keep []string
	dropped := false
	for _, kv := range strings.Split(u.RawQuery, "&") {
		k, v, _ := strings.Cut(kv, "=")
		k, kerr := url.QueryUnescape(k)
		v, verr := url.QueryUnescape(v)
		if !dropped && kerr == nil && verr == nil && k == "datastar" && v == "{}" {
			dropped = true
			continue
		}
		keep = append(keep, kv)
	}
	u.RawQuery = strings.Join(keep, "&")
}

// morph answers a boosted navigation (st.url set) with page: Alder-Status when status is not
// 200, then the events — the patch of the page's <body> (the morph), of its <title> (empty when
// it has none, so the old page's title does not stay), and the history step to st.url
// (datastar.Navigate in st.mode). Without st.url (a POST) it is the body patch alone, as a
// 404/500 answered to a message always was.
func (rt *router) morph(w http.ResponseWriter, r *http.Request, st hop, status int, page string) {
	if status != http.StatusOK {
		w.Header().Set(StatusHeader, strconv.Itoa(status))
	}
	err := datastar.PatchBody(w, rt.shellBody(w, rt.dev(bodyOf(page))))
	if tw, ok := w.(*tracker); ok {
		tw.patches = append(tw.patches, "body")
	}
	if err == nil && st.url != "" {
		t, _ := titleOf(page) // a page without a <title> empties the old one
		err = datastar.PatchTitle(w, t)
		if err == nil {
			err = datastar.Navigate(w, st.url, st.mode)
		}
		if tw, ok := w.(*tracker); ok {
			tw.patches = append(tw.patches, "title", "history "+st.url)
		}
	}
	if err != nil {
		rt.log("error", fmt.Sprintf("%s %s: writing the patch: %v", r.Method, r.URL.Path, err))
	}
	rt.notify(w)
	rt.patchPanel(w, status)
}

// titleOf is the content of the page's first <title> element before its <body>, as rendered
// (escaped); ok is false for a page without one.
func titleOf(page string) (string, bool) {
	head := page
	if i := startTag(page, "body"); i >= 0 {
		head = page[:i]
	}
	i := startTag(head, "title")
	if i < 0 {
		return "", false
	}
	j := strings.IndexByte(head[i:], '>')
	if j < 0 {
		return "", false
	}
	rest := head[i+j+1:]
	k := strings.Index(rest, "</title>")
	if k < 0 {
		return "", false
	}
	return rest[:k], true
}

// document writes "<!doctype html>\n" + page, with the Datastar script tag injected, as HTML.
func (rt *router) document(w http.ResponseWriter, status int, page string) {
	if tw, ok := w.(*tracker); ok && tw.alert != "" {
		page = startOfBody(page, `<al-alert variant="info" role="status">`+esc(tw.alert)+`</al-alert>`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("<!doctype html>\n" + withScript(rt.inlinePanel(w, rt.dev(page), status))))
}

// badQuery is why raw, a request's query string, does not parse, or "" when it does: a pair
// holding a raw `;`, which url.ParseQuery has refused since Go 1.17, or a bad `%` escape. Each
// failing pair is named by its key, so the answer says which parameter to encode (`;` as %3B).
// r.URL.Query() discards the error and the failing pairs with it, so a list-valued parameter
// written with `;` reached load at its default, with nothing logged (#12).
func badQuery(raw string) string {
	if _, err := url.ParseQuery(raw); err == nil {
		return ""
	}
	var why []string
	for _, kv := range strings.Split(raw, "&") {
		k, v, _ := strings.Cut(kv, "=")
		name, kerr := url.QueryUnescape(k)
		if kerr != nil {
			name = k // named as written
		}
		_, verr := url.QueryUnescape(v)
		switch {
		case strings.Contains(kv, ";"):
			why = append(why, fmt.Sprintf("query parameter %q holds a raw \";\": encode it as %%3B", name))
		case kerr != nil:
			why = append(why, fmt.Sprintf("query parameter %q: %v", name, kerr))
		case verr != nil:
			why = append(why, fmt.Sprintf("query parameter %q: %v", name, verr))
		}
	}
	if len(why) == 0 { // ParseQuery refused it for a reason not seen above: say so, at least
		why = append(why, "query does not parse: "+raw)
	}
	return strings.Join(why, "; ")
}

// badRequest answers a request whose query does not parse (why, from badQuery) with status 400
// and a page saying so, or as a body patch for a Datastar request; logged at warn.
func (rt *router) badRequest(w http.ResponseWriter, r *http.Request, st hop, why string) {
	rt.log("warn", fmt.Sprintf("%s %s: %s", r.Method, r.URL.Path, why))
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer("bad request")
	}
	rt.failure(w, r, st, http.StatusBadRequest, "<p>400 bad request: "+esc(why)+"</p>")
}

// notFound writes doc (or, when it renders empty, Options.NotFoundDoc) with status 404, or as
// a body patch for a Datastar request.
func (rt *router) notFound(w http.ResponseWriter, r *http.Request, st hop, doc html.Node) {
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer("not found")
	}
	page := html.Render(doc)
	if page == "" && rt.opts.NotFoundDoc != nil {
		page = html.Render(rt.opts.NotFoundDoc())
	}
	if page == "" {
		page = "<p>404 page not found</p>"
	}
	rt.failure(w, r, st, http.StatusNotFound, page)
}

// maintenance writes Options.MaintenanceDoc (or a default page) with status 503 and the header
// Alder-Status: 503 (on every answer, not only a Datastar one), or as a body patch for a
// Datastar request: a request on a silo that is not served (err says which and why; logged).
func (rt *router) maintenance(w http.ResponseWriter, r *http.Request, st hop, err error) {
	rt.log("warn", fmt.Sprintf("%s %s: maintenance: %v", r.Method, r.URL.Path, err))
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer("maintenance")
	}
	page := ""
	if rt.opts.MaintenanceDoc != nil {
		page = html.Render(rt.opts.MaintenanceDoc())
	}
	if page == "" {
		page = DefaultMaintenance
	}
	w.Header().Set(StatusHeader, "503")
	rt.failure(w, r, st, http.StatusServiceUnavailable, page)
}

// DefaultMaintenance is the maintenance page of an app without App.maintenance.
const DefaultMaintenance = "<html><head><title>Down for maintenance</title></head><body><h1>Down for maintenance</h1><p>This part of the app is being updated. Please try again in a few minutes.</p></body></html>"

// fail logs err (with stack, when given) and writes doc (or, when it renders empty,
// Options.ErrorDoc(err)) with status 500, or as a body patch for a Datastar request. In dev
// the error text is added as an HTML comment at the end of the <body>.
func (rt *router) fail(w http.ResponseWriter, r *http.Request, st hop, doc html.Node, err error, stack []byte) {
	msg := fmt.Sprintf("%s %s: %v", r.Method, r.URL.Path, err)
	if stack != nil {
		msg += "\n" + string(stack)
	}
	rt.log("error", msg)
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer("error")
		tw.detail = fmt.Sprintf("%v\n%s", err, stack)
	}
	page := html.Render(doc)
	if page == "" && rt.opts.ErrorDoc != nil {
		page = html.Render(rt.opts.ErrorDoc(err))
	}
	if page == "" {
		page = "<p>Internal Server Error</p>"
	}
	if rt.opts.Dev {
		page = endOfBody(page, "<!-- error: "+commentSafe(fmt.Sprint(err))+" -->")
	}
	rt.failure(w, r, st, http.StatusInternalServerError, page)
}

// failure writes a 400/404/500/503 page: a full document with that status, or, for a Datastar
// request, a 200 SSE patch of its body (the Datastar client ignores non-2xx answers) whose
// StatusHeader names the status, so a client that is not a browser (an Alder test's) can tell
// it from a re-rendered page; for a boosted navigation (st.url) also its title and the history
// step (morph): the address bar shows the missing page's URL, as after a page load.
func (rt *router) failure(w http.ResponseWriter, r *http.Request, st hop, status int, page string) {
	if datastar.IsDatastar(r) {
		rt.morph(w, r, st, status, page)
		return
	}
	rt.document(w, status, page)
}

// StatusHeader is the response header of a Datastar answer to a request that failed: "400",
// "404", "500" or "503" (the answer itself is a 200 body patch).
const StatusHeader = "Alder-Status"

// endOfBody inserts s just before the last </body> of page, or appends it when there is none.
func endOfBody(page, s string) string {
	if i := strings.LastIndex(page, "</body>"); i >= 0 {
		return page[:i] + s + page[i:]
	}
	return page + s
}

// startOfBody inserts s just after the <body> start tag of page, or prepends it when there is
// none.
func startOfBody(page, s string) string {
	if i := startTag(page, "body"); i >= 0 {
		if j := strings.IndexByte(page[i:], '>'); j >= 0 {
			return page[:i+j+1] + s + page[i+j+1:]
		}
	}
	return s + page
}

// NoticeCookie carries a Noticed answer's text across the redirect of a request without
// Datastar (a form posted with the script off), signed with the session key, for one page.
const NoticeCookie = "alder_notice"

// storeNotice sets NoticeCookie to text for the page the redirect goes to.
func (rt *router) storeNotice(w http.ResponseWriter, r *http.Request, text string) {
	http.SetCookie(w, &http.Cookie{
		Name: NoticeCookie, Value: Sign(rt.key, []byte("notice:"+text)), Path: "/", MaxAge: 60,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}

// takeNotice is the text NoticeCookie carries ("" for none, or one not signed by this app),
// clearing the cookie: the notice is shown once.
func (rt *router) takeNotice(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(NoticeCookie)
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: NoticeCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	p, ok := Verify(rt.key, c.Value)
	text, isNotice := strings.CutPrefix(string(p), "notice:")
	if !ok || !isNotice {
		return ""
	}
	return text
}

// commentSafe makes s safe inside an HTML comment: no "--" (which could end it).
func commentSafe(s string) string {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "- -")
	}
	return s
}

// HeadTags is what the router inserts into every page it serves whole (withScript), in two
// parts. The first goes right after the <head> start tag: <meta name="color-scheme"
// content="light dark">, so the browser paints the page's first frame (and the canvas behind a
// navigation) in the OS's scheme rather than white before the stylesheet arrives, then the
// Datastar script tag. The second, the boost (datastar.BoostTag: links of this site become
// Datastar GETs answered with a morph), goes just before </head>: it is about a kilobyte, and
// placed first it would push a layout's <meta charset> past the first 1024 bytes, where a
// browser looks for it.
func HeadTags() (first, boost string) {
	return colorScheme + BuildMeta() + pendingStyle + html.Render(datastar.ScriptTag()), datastar.BoostTag()
}

// pendingStyle is the in-flight look the runtime injects (not the library's): a 2px bar along the
// top of the page while a request is pending (html[data-alder-pending], set by the boost's
// window._alderBusy) and a dimmed form that is busy (aria-busy, inert).
const pendingStyle = `<style>html[data-alder-pending]::before{content:"";position:fixed;top:0;left:0;width:100%;height:2px;z-index:2147483647;background:var(--primary,currentColor);transform-origin:left;animation:alder-pending 1s ease-in-out infinite}@keyframes alder-pending{from{transform:scaleX(0)}to{transform:scaleX(1)}}form[aria-busy=true]{opacity:.6}</style>`

// BuildMeta is the <meta name="alder-build" content="…"> HeadTags carries: the build of the
// binary serving the page (BuildID), which the boost sends back with each boosted GET
// (datastar.BuildHeader).
func BuildMeta() string {
	return html.Render(html.El("meta", []html.Attr{html.A("name", "alder-build"), html.A("content", BuildID())}))
}

// BuildID is this binary's build: the first 16 hex digits of the SHA-256 of its executable
// (the compiled app and the runtime in it), computed once. Two processes of the same binary —
// a restart, a second replica — share it; a deploy of anything else changes it. When the
// executable cannot be read it is a value made from the process's start time, which differs
// on every start (a tab then reloads once more than it needs to, after a restart).
var BuildID = sync.OnceValue(func() string {
	if exe, err := os.Executable(); err == nil {
		if f, err := os.Open(exe); err == nil {
			defer f.Close()
			h := sha256.New()
			if _, err := io.Copy(h, f); err == nil {
				return hex.EncodeToString(h.Sum(nil))[:16]
			}
		}
	}
	return "t" + strconv.FormatInt(time.Now().UnixNano(), 36)
})

// StripHeadTags is page without what withScript inserted: an Alder test's browser holds the page
// as the view renders it.
func StripHeadTags(page string) string {
	first, boost := HeadTags()
	return strings.Replace(strings.Replace(page, first, "", 1), boost, "", 1)
}

// colorScheme is the color-scheme meta HeadTags begins with.
var colorScheme = html.Render(html.El("meta", []html.Attr{html.A("name", "color-scheme"), html.A("content", "light dark")}))

// withScript inserts HeadTags: the first part right after the first <head> start tag and the
// boost just before the </head> that follows it (right after the first part when there is
// none). A page without <head> gets both together where the browser's parser still puts them
// in the (implied) head, so the boost is never inside the <body> a patch replaces: right after
// the <html> start tag; without that, just before the <body> start tag; else at the start of
// the page.
func withScript(page string) string {
	first, boost := HeadTags()
	if i := startTag(page, "head"); i >= 0 {
		if j := strings.IndexByte(page[i:], '>'); j >= 0 {
			k := i + j + 1
			if e := strings.Index(page[k:], "</head>"); e >= 0 {
				return page[:k] + first + page[k:k+e] + boost + page[k+e:]
			}
			return page[:k] + first + boost + page[k:]
		}
	}
	if i := startTag(page, "html"); i >= 0 {
		if j := strings.IndexByte(page[i:], '>'); j >= 0 {
			k := i + j + 1
			return page[:k] + first + boost + page[k:]
		}
	}
	if i := startTag(page, "body"); i >= 0 {
		return page[:i] + first + boost + page[i:]
	}
	return first + boost + page
}

// reloadTags is what withReload inserts for the build generation gen: the element `alder run`
// patches its error overlay into, and one that subscribes to the dev server's reload events
// when Datastar initialises it (data-init: Datastar v1.0.0 has no data-on-load plugin, and a div
// fires no load event).
//
// The overlay box carries data-ignore-morph, so a body patch from the app (a Rerender) keeps the
// error in it: Datastar leaves an element alone when the old and the new one both carry the
// attribute, while the dev server's own `inner` patches of the box still apply. The subscriber
// names gen (?b=), so the dev server can tell a page of an older build, and keeps its stream
// open while the tab is hidden (openWhenHidden: @get closes it by default), so a reload sent
// meanwhile is not lost; a page that reconnects anyway (after a network blip, or a restarted
// `alder run`) is reloaded at once when its generation is not the current one.
func reloadTags(gen string) string {
	return `<div id="_alder_errors" data-ignore-morph></div><div data-init="@get('/_dev/reload?b=` +
		url.QueryEscape(gen) + `', {openWhenHidden: true})"></div>`
}

// withReload inserts reloadTags(gen) right after the first <body> start tag, or at the start of
// a page without one.
func withReload(page, gen string) string {
	tags := reloadTags(gen)
	if i := startTag(page, "body"); i >= 0 {
		if j := strings.IndexByte(page[i:], '>'); j >= 0 {
			k := i + j + 1
			return page[:k] + tags + page[k:]
		}
	}
	return tags + page
}

// dev is page with reloadTags (withReload) when Options.Reload is set, else page. It applies to
// full documents and body patches alike, so a patched <body> keeps the overlay and subscriber.
func (rt *router) dev(page string) string {
	if rt.opts.Reload != "" {
		return withReload(page, rt.opts.Reload)
	}
	return page
}

// startTag is the index of the first <name> or <name …> start tag in page, or -1.
func startTag(page, name string) int {
	open := "<" + name
	for i := 0; ; {
		j := strings.Index(page[i:], open)
		if j < 0 {
			return -1
		}
		k := i + j + len(open)
		if k < len(page) && (page[k] == '>' || page[k] == ' ') {
			return i + j
		}
		i = k
	}
}

// bodyOf is the <body>…</body> of a rendered page, inclusive, or the page wrapped in <body>
// when it has none.
func bodyOf(page string) string {
	i := startTag(page, "body")
	j := strings.LastIndex(page, "</body>")
	if i < 0 || j < i {
		return "<body>" + page + "</body>"
	}
	return page[i : j+len("</body>")]
}

func (rt *router) serveStatic(w http.ResponseWriter, r *http.Request) {
	// http.FileServer cleans the path already; refusing ".." outright makes that explicit.
	for _, s := range strings.Split(r.URL.Path, "/") {
		if s == ".." {
			http.NotFound(w, r)
			return
		}
	}
	if data, etag, ok := al.File(strings.TrimPrefix(r.URL.Path, designSystemPrefix)); ok && strings.HasPrefix(r.URL.Path, designSystemPrefix) {
		h := w.Header()
		h.Set("Cache-Control", DesignSystemCache)
		h.Set("ETag", etag)
		if strings.HasSuffix(r.URL.Path, ".woff2") {
			h.Set("Content-Type", "font/woff2")
		}
		http.ServeContent(w, r, r.URL.Path, time.Time{}, bytes.NewReader(data)) // 304 on If-None-Match
		return
	}
	rt.static.ServeHTTP(w, r)
}

// designSystemPrefix is where every build writes the `alder-ui` assets (buildkit
// WriteDesignSystem). Its files (al.Names) are served from the runtime's embedded copy,
// the same bytes, with DesignSystemCache and a strong ETag hashed from them once. Any other
// file there is the app's and is served like the rest of static/.
const designSystemPrefix = staticPrefix + "al/"

// DesignSystemCache is the Cache-Control of /static/al/'s files. The URL carries no version —
// it stays /static/al/al.css across compiler upgrades — so the stylesheet cannot be immutable;
// but revalidating it on every page load (no-cache) makes a load wait for a round trip before
// it can paint, the white flash between pages. So: fresh for five minutes (a load paints from
// the cache at once), then served stale for up to a day while the browser revalidates in the
// background (a 304 by the ETag when nothing changed). The trade: for up to five minutes after
// an upgrade a browser may show the previous release's stylesheet.
const DesignSystemCache = "public, max-age=300, stale-while-revalidate=86400"

// filesOnly is a FileSystem in which directories do not exist: no listings, no index pages.
type filesOnly struct{ fs http.FileSystem }

func (f filesOnly) Open(name string) (http.File, error) {
	file, err := f.fs.Open(name)
	if err != nil {
		return nil, err
	}
	st, err := file.Stat()
	if err != nil || st.IsDir() {
		file.Close()
		return nil, &fs.PathError{Op: "open", Path: name, Err: os.ErrNotExist}
	}
	return file, nil
}
