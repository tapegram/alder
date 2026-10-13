package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	rtpkg "alder/rt"
	"alder/rt/datastar"
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
	Login       string // App.login page path, copied to Request.Login
	Reload      string // under `alder run`, the build generation (ALDER_RELOAD); non-empty: every page carries the dev server's overlay and reload subscriber (withReload)
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
	handle func(*Request) Response
}

// segment is a literal path segment, or a parameter when param is set.
type segment struct {
	lit   string
	param string
}

// NewRouter returns the app's http.Handler. For each request it: recovers panics (→ 500 via
// ErrorDoc); serves the Datastar script at datastar.ScriptPath and Options.StaticDir under
// /static/; matches method and pattern, the most specific pattern first (a literal segment
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
// Datastar client ignores other answers; and a Redirect is a 200 SSE event that navigates the
// browser to its Location (datastar.Navigate), because the client's fetch would follow a 303
// itself and leave the address bar and history at the old URL.
//
// The router's only mutable state is a once-only warning: it is safe for concurrent requests.
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
		rt.routes = append(rt.routes, route{method: r.Method, segs: compile(r.Pattern), handle: r.Handle})
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
	p := strings.TrimSuffix(strings.TrimPrefix(r.URL.EscapedPath(), "/"), "/")
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
	w := &tracker{ResponseWriter: rw}
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
			rt.fail(w, r, html.None(), err, debug.Stack())
		}
	}()

	get := r.Method == http.MethodGet || r.Method == http.MethodHead
	if get && r.URL.Path == datastar.ScriptPath {
		rt.script.ServeHTTP(w, r)
		return
	}
	if get && rt.static != nil && strings.HasPrefix(r.URL.Path, staticPrefix) {
		rt.serveStatic(w, r)
		return
	}

	method := r.Method
	if method == http.MethodHead {
		method = http.MethodGet
	}
	var hit *route
	var params map[string]string
	if segs, ok := pathSegments(r); ok {
		for i := range rt.routes {
			if rt.routes[i].method != method {
				continue
			}
			if p, ok := rt.routes[i].match(segs); ok {
				hit, params = &rt.routes[i], p
				break
			}
		}
	}
	if hit == nil {
		rt.notFound(w, r, html.None())
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
	rq.Session, rq.sessionEnc = rt.decodeSession(r)
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
	}
	resp := hit.handle(rq)
	if rt.opts.Served != nil {
		rt.opts.Served(rq)
	}
	rt.write(w, r, rq, resp)
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

func (rt *router) write(w http.ResponseWriter, r *http.Request, rq *Request, resp Response) {
	switch resp.Status {
	case http.StatusOK, http.StatusSeeOther:
		if err := rt.setSession(w, r, rq, resp.Session); err != nil {
			rt.fail(w, r, html.None(), err, nil)
			return
		}
		if resp.Status == http.StatusSeeOther {
			if !rtpkg.SafeURL(resp.Location) {
				// defense in depth: Url.external already turns such a URL into "#"
				rt.fail(w, r, html.None(), fmt.Errorf("%w: refusing to navigate to %s", eff.ErrInternal, rtpkg.URLScheme(resp.Location)), nil)
				return
			}
			if rq.Datastar {
				// the client's fetch would follow a 303 itself and patch the new page into the
				// old URL: navigate the browser instead
				if err := datastar.Navigate(w, resp.Location); err != nil {
					rt.log("error", fmt.Sprintf("%s %s: writing the navigation: %v", r.Method, r.URL.Path, err))
				}
				return
			}
			w.Header().Set("Location", resp.Location)
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		page := html.RenderWith(resp.Node, resp.State)
		if resp.Patch && rq.Datastar {
			if err := datastar.PatchBody(w, rt.dev(bodyOf(page))); err != nil {
				rt.log("error", fmt.Sprintf("%s %s: writing the patch: %v", r.Method, r.URL.Path, err))
			}
			return
		}
		rt.document(w, http.StatusOK, page)
	case http.StatusNotFound, http.StatusInternalServerError:
		// a non-nil session here is one a successful Task left before a later step failed
		// (the re-load after update): it is stored all the same
		if err := rt.setSession(w, r, rq, resp.Session); err != nil {
			rt.log("error", fmt.Sprintf("%s %s: %v", r.Method, r.URL.Path, err))
		}
		if resp.Status == http.StatusNotFound {
			rt.notFound(w, r, resp.Node)
		} else {
			rt.fail(w, r, resp.Node, resp.Err, nil)
		}
	default:
		rt.fail(w, r, html.None(), fmt.Errorf("handler returned a response with status %d", resp.Status), nil)
	}
}

// document writes "<!doctype html>\n" + page, with the Datastar script tag injected, as HTML.
func (rt *router) document(w http.ResponseWriter, status int, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("<!doctype html>\n" + withScript(rt.dev(page))))
}

// notFound writes doc (or, when it renders empty, Options.NotFoundDoc) with status 404, or as
// a body patch for a Datastar request.
func (rt *router) notFound(w http.ResponseWriter, r *http.Request, doc html.Node) {
	page := html.Render(doc)
	if page == "" && rt.opts.NotFoundDoc != nil {
		page = html.Render(rt.opts.NotFoundDoc())
	}
	if page == "" {
		page = "<p>404 page not found</p>"
	}
	rt.failure(w, r, http.StatusNotFound, page)
}

// fail logs err (with stack, when given) and writes doc (or, when it renders empty,
// Options.ErrorDoc(err)) with status 500, or as a body patch for a Datastar request. In dev
// the error text is added as an HTML comment at the end of the <body>.
func (rt *router) fail(w http.ResponseWriter, r *http.Request, doc html.Node, err error, stack []byte) {
	msg := fmt.Sprintf("%s %s: %v", r.Method, r.URL.Path, err)
	if stack != nil {
		msg += "\n" + string(stack)
	}
	rt.log("error", msg)
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
	rt.failure(w, r, http.StatusInternalServerError, page)
}

// failure writes a 404/500 page: a full document with that status, or, for a Datastar
// request, a 200 SSE patch of its body (the Datastar client ignores non-2xx answers) whose
// StatusHeader names the status, so a client that is not a browser (an Alder test's) can tell
// it from a re-rendered page.
func (rt *router) failure(w http.ResponseWriter, r *http.Request, status int, page string) {
	if datastar.IsDatastar(r) {
		w.Header().Set(StatusHeader, strconv.Itoa(status))
		if err := datastar.PatchBody(w, rt.dev(bodyOf(page))); err != nil {
			rt.log("error", fmt.Sprintf("%s %s: writing the patch: %v", r.Method, r.URL.Path, err))
		}
		return
	}
	rt.document(w, status, page)
}

// StatusHeader is the response header of a Datastar answer to a request that failed: "404" or
// "500" (the answer itself is a 200 body patch).
const StatusHeader = "Alder-Status"

// endOfBody inserts s just before the last </body> of page, or appends it when there is none.
func endOfBody(page, s string) string {
	if i := strings.LastIndex(page, "</body>"); i >= 0 {
		return page[:i] + s + page[i:]
	}
	return page + s
}

// commentSafe makes s safe inside an HTML comment: no "--" (which could end it).
func commentSafe(s string) string {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "- -")
	}
	return s
}

// withScript inserts the Datastar script tag right after the first <head> start tag; without
// one, right after the <html> start tag; without that, just before the last </body>; else at
// the start of the page.
func withScript(page string) string {
	tag := html.Render(datastar.ScriptTag())
	for _, name := range []string{"head", "html"} {
		if i := startTag(page, name); i >= 0 {
			if j := strings.IndexByte(page[i:], '>'); j >= 0 {
				k := i + j + 1
				return page[:k] + tag + page[k:]
			}
		}
	}
	if i := strings.LastIndex(page, "</body>"); i >= 0 {
		return page[:i] + tag + page[i:]
	}
	return tag + page
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
	if strings.HasPrefix(r.URL.Path, designSystemPrefix) {
		rel := filepath.FromSlash(strings.TrimPrefix(r.URL.Path, staticPrefix))
		if st, err := os.Stat(filepath.Join(rt.opts.StaticDir, rel)); err == nil && st.Mode().IsRegular() {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
	}
	rt.static.ServeHTTP(w, r)
}

// designSystemPrefix is where every build writes the `alder-ui` assets (buildkit
// WriteDesignSystem). They change only with the compiler, as datastar.js does, and are served
// with the same immutable cache policy.
const designSystemPrefix = staticPrefix + "al/"

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
