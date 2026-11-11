package rtesting

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"alder/rt"
	"alder/rt/datastar"
	"alder/rt/eff"
	"alder/rt/html"
	"alder/rt/web"
)

// AnswerTag says what a browser step's request came to: the Prelude `Answer`'s constructor.
type AnswerTag int

const (
	Shown         AnswerTag = iota // a GET showed its page
	Redirected                     // the answer was a redirect; the browser fetched Answer.Page
	FormRejected                   // a form came back with errors (Answer.Errors)
	PageMissing                    // 404: the not-found document is held
	RequestFailed                  // 500 (or a request the router refused): Answer.Text says why
	Rendered                       // a write route answered Response.render; Answer.Page is the route
	Refreshed                      // a write route answered Response.refresh
)

func (t AnswerTag) String() string {
	switch t {
	case Shown:
		return "Shown"
	case Redirected:
		return "Redirected"
	case FormRejected:
		return "FormRejected"
	case PageMissing:
		return "PageMissing"
	case RequestFailed:
		return "RequestFailed"
	case Rendered:
		return "Rendered"
	case Refreshed:
		return "Refreshed"
	}
	return "AnswerTag(" + strconv.Itoa(int(t)) + ")"
}

// Answer is the Prelude `Answer`: Tag, and the page of Redirected, the errors of FormRejected (in
// document order: each `<Errors>` list's items, Field "" for the form-level list) or the text
// of RequestFailed.
type Answer struct {
	Tag    AnswerTag
	Page   rt.Route
	Errors []web.FieldError
	Text   string
}

// Result is what a step gives the test: the answer, the session after it (what the browser's
// cookie now carries, decoded; the codec's Default without one; nil without a codec) and the
// document the browser now holds.
type Result struct {
	Answer  Answer
	Session any
	Doc     html.Node
	// Notices are the toasts the step's answer showed, in order: their text.
	Notices []string
	// Toasts are the same toasts with their kind (rt.NoticeKind: info, success, warning, error).
	Toasts []Toast
	// Model is the model the page held was rendered from when the last request the step made
	// was a page's GET (web.Request.Model; Test.get's typed model), nil otherwise.
	Model any
	// URL is the URL of the page the browser holds after the step, Status the code of the step's
	// last response (a Datastar answer's Alder-Status when it carries one).
	URL    string
	Status int64
}

// TestSecret is the cookie key of a test's router when its options carry none (the router
// refuses a short one outside dev mode).
var TestSecret = []byte("alder-test-cookie-key: tests only")

// cookieNow is the clock of the browser's router (cookie expiry only): fixed, so the cookies of
// a test never expire and reading it does not advance the test's own clock.
var cookieNow = func() time.Time { return defaultNow }

// failHead is the start of the router's error line: the request's method and masked URL
// (web's line).
var failHead = regexp.MustCompile(`^[A-Z]+ [^ ]+: `)

// buildMeta matches the alder-build meta a page's head carries (web.BuildMeta); its group is the
// build named.
var buildMeta = regexp.MustCompile(`<meta name="alder-build" content="([^"]*)"`)

// redirectLimit is how many redirects in a row the browser follows before it gives up (RequestFailed).
const redirectLimit = 10

// Browser is a test's in-memory browser: the app's real router (web.NewRouter over the app's
// routes), a cookie jar holding the session cookie, the current page's URL and the document it
// holds. Its requests are served in-process; each runs within the test's transaction under a
// savepoint (the router's per-request Env comes from Options.EnvFor: the test's Store, Caps,
// Now and Log, eff.Within the test's transaction), so the test's rows are visible to the page
// and a failed request undoes only its own writes. Submits and clicks are Datastar requests
// (Datastar-Request: true), so their answers are the patch and navigate events a real client
// applies; Get and Open are plain GETs. The effects each request runs are appended to the
// test's trace.
//
// A Browser belongs to one test and is not safe for concurrent use.
type Browser struct {
	env     *eff.Env
	handler http.Handler
	codec   web.SessionCodec
	key     []byte
	login   string
	routes  []web.Route // the app's routes: what a NoControlError's hint matches controls by (offered)

	cookie string    // the session cookie's value, "" for none
	url    string    // the URL of the page held, "" before the first
	doc    html.Node // the page held (without the router's doctype and script tag)
	hasDoc bool
	// htmlNames is the names of the <html> attributes the server set last (on the page held whole,
	// or by the last datastar.PatchHtmlAttrs event): the ones the next such event may remove
	htmlNames []string
	toasts    []Toast // the toasts of the step's requests (Result.Toasts and Notices)
	status    int64   // the code of the last response (Result.Status)
	// build is the build the held page names (its <meta name="alder-build">, which the router's
	// head tags carry: web.BuildMeta), "" for none: the page's script sends it with every
	// request it makes for a route (datastar.BuildHeader)
	build string
	// answered is the answer of the write route the last request ran (web.Options.Answered),
	// nil for none: what a step on a route reads its kind and target from
	answered *rt.Response

	// dom is the held page as a browser's parser builds it and Datastar patches it (html.Repair):
	// the page the drift check compares (checkPatched). doc stays the markup as the router sent
	// it, which is what a test reads.
	dom html.Node
	// shown is the hash of the body the held page shows (its <al-shown>, which the held doc
	// leaves out), sent with every post as html.ShownHeader, as the page's script does
	shown string
	// rendered is the body a full render of the last request's page would have sent
	// (web.Options.Rendered), "" when it re-rendered nothing: after applying the answer, every
	// region the answer patched must equal it (checkPatched)
	rendered string
	// morphed is whether the last request's answer replaced the whole body, regions the ids of
	// the elements it patched (`#id`, mode outer) in order, and domBefore dom before the answer:
	// what checkPatched compares
	morphed   bool
	regions   []string
	domBefore html.Node

	model   any         // the model the last request's page GET rendered (web.Request.Model), nil for none
	reqEnvs []*eff.Env  // the Envs of the request being served
	cur     *eff.Within // the transaction it runs within
	reqHead string      // "METHOD /decoded/path: ", how the router's log lines for it begin
	lastErr string      // the router's last error log line for it

	heads    map[*eff.Env]string // each request Env's "METHOD /path" (Options.Served)
	step     *stepAcc            // the step running (track), nil between steps
	bench    bool                // measure each step (`alder bench`)
	warnings []string            // the N+1 warnings of the steps so far (CaseResult.Warnings)
	measures []Measure           // with bench, each step's Measure (CaseResult.Bench)
}

// NewBrowser is a browser for the test running env, serving routes with opts: opts is the
// app's options, of which the browser replaces Store, Caps, Now (the cookie clock), Dev (off),
// Reload (none) and EnvFor, and gives Secret TestSecret when it is shorter than 16 bytes. The
// Env each request gets shares env's Store, Caps (read when the request is made, so a
// Test.fake installed since applies), Now and Log, and runs Within env's transaction, begun
// when the first request needs it (env.Transaction).
func NewBrowser(env *eff.Env, routes []web.Route, opts web.Options) *Browser {
	b := &Browser{env: env, codec: opts.Session, login: opts.Login, routes: routes, heads: map[*eff.Env]string{}}
	if len(opts.Secret) < 16 {
		opts.Secret = TestSecret
	}
	b.key = opts.Secret
	log := opts.Log
	opts.Log = func(level, msg string) {
		if level == "error" {
			b.lastErr = msg
		}
		if log != nil {
			log(level, msg)
		}
	}
	opts.Store, opts.Caps, opts.Now, opts.Dev, opts.Reload = env.Store, env.Caps, cookieNow, false, ""
	opts.EnvFor = func(ctx context.Context, session any) *eff.Env {
		e := &eff.Env{Ctx: ctx, Store: b.env.Store, Caps: b.env.Caps, Now: b.env.Now, Sleep: b.env.Sleep, Log: b.env.Log, Session: session, Within: b.cur,
			Stores: b.env.Stores, CheckSession: b.env.CheckSession,
			Rec: &eff.Recorder{}} // its statements, for the N+1 warning (and `alder bench`)
		b.reqEnvs = append(b.reqEnvs, e)
		return e
	}
	served := opts.Served
	opts.Served = func(rq *web.Request) {
		b.model = rq.Model
		if rq.Env != nil && rq.HTTP != nil {
			b.heads[rq.Env] = rq.HTTP.Method + " " + rq.HTTP.URL.Path
		}
		if served != nil {
			served(rq)
		}
	}
	rendered := opts.Rendered
	opts.Rendered = func(rq *web.Request, body string) {
		b.rendered = body
		if rendered != nil {
			rendered(rq, body)
		}
	}
	answered := opts.Answered
	opts.Answered = func(rq *web.Request, a rt.Response) {
		b.answered = &a
		if answered != nil {
			answered(rq, a)
		}
	}
	b.handler = web.NewRouter(routes, opts)
	return b
}

// within is the test's transaction for a request to run inside: env's, begun if no step has
// yet; nil outside a running Task (the request then runs on its own, as in production). An
// error beginning it is the step's error: a request that began its own transaction instead
// would wait forever for the single connection of the test's in-memory database.
func (b *Browser) within() (*eff.Within, error) {
	tx, err := b.env.Transaction()
	if err != nil {
		return nil, fmt.Errorf("beginning the test's transaction: %w", err)
	}
	if tx == nil {
		return nil, nil
	}
	sp, _ := b.env.Store.(eff.Savepointer)
	return &eff.Within{Tx: tx, Savepoints: sp}, nil
}

// browserKey carries a test's browser slot in its Env's context.
type browserKey struct{}

type browserSlot struct {
	once   sync.Once
	env    *eff.Env
	routes []web.Route
	opts   web.Options
	b      *Browser
	bench  bool // the browser measures its steps (`alder bench`)
}

// withBrowser is env.Ctx carrying the slot BrowserOf makes env's browser in.
func withBrowser(env *eff.Env, routes []web.Route, opts web.Options, bench bool) context.Context {
	ctx := env.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, browserKey{}, &browserSlot{env: env, routes: routes, opts: opts, bench: bench})
}

// BrowserOf is the browser of the test running env, made on first use: the test runner puts
// the app's routes and options in each task test's context (Options.Routes), so every case has
// a browser of its own and nothing of one case reaches the next. It panics when the test has
// none (a build without the app's routes).
func BrowserOf(env *eff.Env) *Browser {
	var slot *browserSlot
	if env != nil && env.Ctx != nil {
		slot, _ = env.Ctx.Value(browserKey{}).(*browserSlot)
	}
	if slot == nil {
		panic("alder: this test has no browser (the test program was built without the app's routes)")
	}
	slot.once.Do(func() { slot.b = NewBrowser(slot.env, slot.routes, slot.opts); slot.b.bench = slot.bench })
	return slot.b
}

// Doc is the document the browser holds (None before the first page).
func (b *Browser) Doc() html.Node { return b.doc }

// URL is the URL of the page the browser holds ("" before the first).
func (b *Browser) URL() string { return b.url }

// Login stores session in the browser's cookie, signed with the router's key as the router
// would store it (Test.login). Without a session codec (an app without a session) it does
// nothing.
func (b *Browser) Login(session any) error {
	if b.codec == nil {
		return nil
	}
	v, err := web.SessionCookie(b.key, b.codec, session, cookieNow())
	if err != nil {
		return err
	}
	b.cookie = v
	return nil
}

// Session is the session the browser's cookie carries: the codec's Default without a valid
// cookie, nil without a codec.
func (b *Browser) Session() any {
	if b.codec == nil {
		return nil
	}
	if b.cookie != "" {
		if s, ok := web.SessionFromCookie(b.key, b.codec, b.cookie, cookieNow()); ok {
			return s
		}
	}
	return b.codec.Default()
}

// NotShownError is Get's error: the page was not shown. Requires, when set (the generated code
// knows the page's `requires` field), is added to a redirect to the login page.
type NotShownError struct {
	URL      string
	Answer   Answer
	Requires string
}

// Error is "GET /todos/x: not found", "GET /: redirected to /login (requires user)" or
// "GET /: failed: <message>".
func (e *NotShownError) Error() string {
	head := "GET " + e.URL + ": "
	switch e.Answer.Tag {
	case PageMissing:
		return head + "not found"
	case Redirected:
		s := head + "redirected to " + rt.RouteURL(e.Answer.Page)
		if e.Requires != "" {
			s += " (requires " + e.Requires + ")"
		}
		return s
	case RequestFailed:
		return head + "failed: " + e.Answer.Text
	}
	return head + e.Answer.Tag.String()
}

// NoControlError is the error of a Submit or Click the held page offers no control for. Msg is
// the route as the test wrote it (`Todos.Index.Toggle { id: "…" }`) and Form the form's name for
// a Submit ("" when not known). Page is the held page's URL; Offers, when the page requests the
// same route with other parameters, says which (offered). Trigger names the attribute of a
// trigger step's missing element (`load`, `intersect`, `input`), "" for a Submit or Click.
type NoControlError struct {
	Submit  bool
	Trigger string
	Form    string
	Msg     string
	URL     string
	Page    string
	Offers  string
}

// Error is "no page is open: `Test.get` one first" before the browser holds a page (Page ""),
// else "the page / has no form NewTodo (Todos.Index.Submit)" for a Submit ("the page / has no
// form for Todos.Index.Submit" without Form) and "the page / has no button or form for
// Todos.Index.Toggle { id: "…" }" for a Click, each followed by " (the page has Toggle for id:
// a, b)" when the page requests the route with other parameters. A trigger step's is "the page
// / has no element with `load` requesting /stats": the URL's path, as its matching ignores the
// query.
func (e *NoControlError) Error() string {
	if e.Page == "" {
		return "no page is open: `Test.get` one first"
	}
	if e.Trigger != "" {
		return "the page " + e.Page + " has no element with `" + e.Trigger + "` requesting " + urlPath(e.URL)
	}
	s := "the page " + e.Page + " has no button or form for " + e.Msg
	if e.Submit {
		if e.Form != "" {
			s = "the page " + e.Page + " has no form " + e.Form + " (" + e.Msg + ")"
		} else {
			s = "the page " + e.Page + " has no form for " + e.Msg
		}
	}
	if e.Offers != "" {
		s += " (the page has " + e.Offers + ")"
	}
	return s
}

// SeveralControlsError is the error of a trigger step (Step "Test.load", "Test.intersect",
// "Test.input") whose route the held page requests from more than one element: which one
// appeared, scrolled into view or changed is not known. Path is the route's URL path; Elements
// name each element, in document order (`<div id="a">`, `<div>` without an id).
type SeveralControlsError struct {
	Step     string
	Path     string
	Elements []string
}

// Error is "Test.load found 2 elements requesting /stats: <div id="a">, <div id="b">".
func (e *SeveralControlsError) Error() string {
	return fmt.Sprintf("%s found %d elements requesting %s: %s", e.Step, len(e.Elements), e.Path, strings.Join(e.Elements, ", "))
}

// NoInputError is the error of a Submit whose fields name one the matched form renders no
// input for (an <input>, <select> or <textarea> with that name): a browser sends only the
// form's inputs, so the page would never receive it. Form is the form's name ("" when not
// known).
type NoInputError struct {
	Form string
	Name string
}

// Error is "the page's form NewTodo has no input `due`".
func (e *NoInputError) Error() string {
	form := "the page's form "
	if e.Form != "" {
		form += e.Form + " "
	}
	return form + "has no input `" + e.Name + "`"
}

// Open requests the page at url with a plain GET and the browser's cookie, follows a redirect
// (to at most redirectLimit in a row) and holds what it ends on: Shown, Redirected (the page
// reached), PageMissing (the not-found document held) or RequestFailed (the error document held, Text the
// error). Its error is for internal failures only (a page that does not parse).
func (b *Browser) Open(u string) (Result, error) {
	defer b.track()()
	ans, err := b.navigate(u, false)
	if err != nil {
		return Result{}, err
	}
	return b.result(ans), nil
}

// Fetched is Test.fetch's answer: the Go type of its Alder record, field for field.
type Fetched = struct {
	Status      int64
	ContentType string
	Body        []byte
	Headers     []struct {
		Name  string
		Value string
	}
}

// Fetch is Test.fetch: one GET of u through the router (the cookie, kept as the answer sets it,
// and the request's effects on the trace, as every step), its raw answer returned: the status,
// Content-Type, body and every header (sorted by name), a redirect not followed. The page the
// browser holds, and the notices and model of the last step, stay as they were.
func (b *Browser) Fetch(u string) (Fetched, error) {
	defer b.track()()
	toasts, model := b.toasts, b.model
	defer func() { b.toasts, b.model = toasts, model }()
	w, err := b.get(u)
	if err != nil {
		return Fetched{}, err
	}
	res := w.Result()
	out := Fetched{Status: int64(res.StatusCode), ContentType: res.Header.Get("Content-Type"), Body: w.Body.Bytes()}
	for _, n := range slices.Sorted(maps.Keys(res.Header)) {
		for _, v := range res.Header[n] {
			out.Headers = append(out.Headers, struct {
				Name  string
				Value string
			}{n, v})
		}
	}
	return out, nil
}

// Get is Open for a test that expects the page shown: any other answer is a *NotShownError.
func (b *Browser) Get(u string) (Result, error) {
	r, err := b.Open(u)
	if err != nil {
		return r, err
	}
	if r.Answer.Tag != Shown {
		return r, &NotShownError{URL: u, Answer: r.Answer}
	}
	return r, nil
}

// Fields is a form's record as Test.submit posts it (the generated encode__M__F): Values, the
// fields as the form's inputs send them, and Absent, the names whose value is what the form's
// decoder makes of a field that is not sent (a Maybe's Nothing without a default, a field
// equal to its default): the page decodes the record the test wrote whether its form has an
// input for one of those or not. A Bool's False is not in Values at all (an unticked checkbox
// sends nothing), so it needs no input either.
type Fields struct {
	Values url.Values
	Absent []string
}

// For is the body a browser sends for fs through form f: the fields f has an input for (an
// <input>, <select> or <textarea> with the field's name, which Datastar's form serialization
// sends). A field it has no input for is left out when its name is in Absent; otherwise the
// error is a *NoInputError naming form and the first such field in name order — a user could
// not send it.
func (fs Fields) For(f html.Node, form string) (url.Values, error) {
	inputs := map[string]bool{}
	for _, sel := range []html.Selector{selInput, selSelect, selTextarea} {
		for _, n := range html.Select(f, sel) {
			if name, ok := html.AttrOf(n, "name"); ok {
				inputs[name] = true
			}
		}
	}
	out := url.Values{}
	for _, name := range slices.Sorted(maps.Keys(fs.Values)) {
		switch {
		case inputs[name]:
			out[name] = fs.Values[name]
		case !slices.Contains(fs.Absent, name):
			return nil, &NoInputError{Form: form, Name: name}
		}
	}
	return out, nil
}

var (
	selBody     = html.MustParseSelector("body")
	selHTML     = html.MustParseSelector("html")
	selShown    = html.MustParseSelector("al-shown")
	selErrors   = html.MustParseSelector("ul[data-errors]")
	selLi       = html.MustParseSelector("li")
	selInput    = html.MustParseSelector("input")
	selSelect   = html.MustParseSelector("select")
	selTextarea = html.MustParseSelector("textarea")
)

// clickForm reports whether form f is a click button's own form, which is not a form to submit.
func clickForm(f html.Node) bool {
	class, _ := html.AttrOf(f, "class")
	return slices.Contains(strings.Fields(class), "_alder-click")
}

// result is ans with the session the cookie now carries and the document held.
func (b *Browser) result(ans Answer) Result {
	var notices []string
	for _, t := range b.toasts {
		notices = append(notices, t.Text)
	}
	return Result{Answer: ans, Session: b.Session(), Doc: b.doc, Model: b.model, Notices: notices, Toasts: b.toasts, URL: b.url, Status: b.status}
}

// get sends a plain GET of u through the router (send).
func (b *Browser) get(u string) (*httptest.ResponseRecorder, error) {
	return b.send(outgoing{method: http.MethodGet, target: u})
}

// outgoing is one request the browser sends: its method and target, its body of type ctype
// (none when ctype is ""), its own headers, and ds for a Datastar request (Datastar-Request:
// true).
type outgoing struct {
	method, target string
	body, ctype    string
	header         http.Header
	ds             bool
}

// send sends request o through the router: with the cookie, the held page as Referer, and for
// a Datastar request its header; it then keeps the cookie the answer sets and appends the
// effects the request ran to the test's trace.
func (b *Browser) send(o outgoing) (*httptest.ResponseRecorder, error) {
	b.toasts = nil
	cur, err := b.within()
	if err != nil {
		return nil, err
	}
	var req *http.Request
	if o.ctype != "" {
		req = httptest.NewRequest(o.method, o.target, strings.NewReader(o.body))
		req.Header.Set("Content-Type", o.ctype)
	} else {
		req = httptest.NewRequest(o.method, o.target, nil)
	}
	if b.env.Ctx != nil {
		req = req.WithContext(b.env.Ctx)
	}
	if o.ds {
		req.Header.Set("Datastar-Request", "true")
	}
	for k, vs := range o.header {
		req.Header[k] = vs
	}
	if b.url != "" {
		req.Header.Set("Referer", "http://example.com"+b.url)
	}
	if b.cookie != "" {
		req.AddCookie(&http.Cookie{Name: web.CookieName, Value: b.cookie})
	}
	b.reqEnvs, b.lastErr, b.cur, b.model, b.rendered, b.answered = nil, "", cur, nil, "", nil
	b.morphed, b.regions, b.domBefore = false, nil, b.dom
	b.reqHead = req.Method + " " + req.URL.Path + ": "
	w := httptest.NewRecorder()
	b.serveHTTP(w, req)
	recs := map[*eff.Recorder]bool{}
	for _, e := range b.reqEnvs {
		if e.Rec != nil && recs[e.Rec] {
			continue // the shown page of a write (web's runShown): its steps are on the write's trace already
		}
		recs[e.Rec] = true
		b.env.Trace = append(b.env.Trace, e.Trace...)
	}
	b.account(req.Method+" "+req.URL.Path, w.Body.Len())
	b.status = int64(w.Code)
	if code, err := strconv.Atoi(w.Header().Get(web.StatusHeader)); err == nil {
		b.status = int64(code)
	}
	b.reqEnvs = nil
	clear(b.heads)
	for _, c := range w.Result().Cookies() {
		if c.Name == web.CookieName {
			if c.MaxAge < 0 || c.Value == "" {
				b.cookie = ""
			} else {
				b.cookie = c.Value
			}
		}
	}
	return w, nil
}

// navigate GETs u, following redirects, and holds the page it ends on; redirected says u is
// itself a redirect's target, so a page shown is Redirected.
func (b *Browser) navigate(u string, redirected bool) (Answer, error) {
	for hop := 0; ; hop++ {
		if redirected && external(u) {
			// another site (Url.external): the browser does not leave the app, so nothing is
			// served and the held page and URL stay as they were; the answer names the target
			// as Url.external builds it
			return Answer{Tag: Redirected, Page: rt.Route{Path: u}}, nil
		}
		if redirected && strings.HasPrefix(u, "#") {
			// "#" (an unsafe URL Url.external neutralised) or "#frag": a browser stays on the
			// page it has, so nothing is requested and the held page and URL stay
			return Answer{Tag: Redirected, Page: rt.Route{Path: u}}, nil
		}
		w, err := b.get(u)
		if err != nil {
			return Answer{}, err
		}
		if w.Code == http.StatusSeeOther {
			if hop == redirectLimit {
				return Answer{Tag: RequestFailed, Text: fmt.Sprintf("more than %d redirects in a row (the last to %s)", redirectLimit, w.Header().Get("Location"))}, nil
			}
			u, redirected = w.Header().Get("Location"), true
			continue
		}
		ans, err := b.document(u, w)
		if err != nil {
			return ans, err
		}
		if redirected && ans.Tag == Shown {
			ans = Answer{Tag: Redirected, Page: pageOf(u)}
		}
		return ans, nil
	}
}

// document holds the page of a plain (non-Datastar) answer w to a request for u: a GET answered
// with anything but text/html (a file page) RequestFailed "GET /path: a file page (<type>): use
// `Test.fetch`", 200 Shown, 404 PageMissing, 500 RequestFailed, anything else (a request the router refused: 400, 413, 415) RequestFailed
// with its status and the router's text (the page's, for the HTML 400 of a query that does not parse).
func (b *Browser) document(u string, w *httptest.ResponseRecorder) (Answer, error) {
	body := w.Body.String()
	ct := w.Header().Get("Content-Type")
	isHTML := strings.HasPrefix(ct, "text/html")
	if !isHTML && strings.HasPrefix(b.reqHead, http.MethodGet+" ") {
		// a file page the checker could not see (Test.get/open of one is refused statically);
		// a POST's plain-text refusal (400, 413, 415) is mapped below
		return Answer{Tag: RequestFailed, Text: b.reqHead + "a file page (" + ct + "): use `Test.fetch`"}, nil
	}
	if isHTML {
		if err := b.hold(u, body); err != nil {
			return Answer{}, err
		}
	}
	switch {
	case w.Code == http.StatusOK && isHTML:
		return Answer{Tag: Shown}, nil
	case w.Code == http.StatusNotFound:
		return Answer{Tag: PageMissing}, nil
	case w.Code == http.StatusInternalServerError:
		return Answer{Tag: RequestFailed, Text: b.failText()}, nil
	case w.Code == http.StatusServiceUnavailable:
		return Answer{Tag: RequestFailed, Text: maintenanceText}, nil
	}
	if isHTML { // the router's own page, which names the status: a 400 for a query that does not parse
		return Answer{Tag: RequestFailed, Text: b.refusedText()}, nil
	}
	return Answer{Tag: RequestFailed, Text: fmt.Sprintf("%d %s", w.Code, strings.TrimSpace(body))}, nil
}

// refusedText is RequestFailed's text for the router's own page of a request it refused (the 400
// of a query that does not parse): the held body's text, which names the status and the
// parameter. The body alone: a Datastar answer patched only it, and the old title stays.
func (b *Browser) refusedText() string {
	if body := html.Select(b.doc, selBody); len(body) > 0 {
		return strings.TrimSpace(html.TextContent(body[0]))
	}
	return strings.TrimSpace(html.TextContent(b.doc))
}

// maintenanceText is RequestFailed's text for a request on a silo that is not served (503: the
// app's maintenance page is held).
const maintenanceText = "503 maintenance"

// hold makes page (a document the router served for u) the held one: its doctype and the
// Datastar script tag the router injected are left out, so it is the page as the view renders it.
// The build its head names (its alder-build meta's content, web.BuildMeta) is kept
// (Browser.build), as the page's script reads it: another build's page keeps that build, a page
// without the meta "".
func (b *Browser) hold(u, page string) error {
	page = strings.TrimPrefix(page, "<!doctype html>\n")
	build := ""
	if m := buildMeta.FindStringSubmatch(page); m != nil {
		build = m[1]
	}
	page = web.StripHeadTags(page)
	doc, err := html.Parse(page)
	if err != nil {
		return fmt.Errorf("the page at %s does not parse: %w", u, err)
	}
	b.url, b.hasDoc, b.build = u, true, build
	b.doc, b.shown = takeShown(doc)
	b.htmlNames = heldHTMLNames(b.doc)
	b.dom = html.Repair(b.doc)
	return nil
}

// takeShown is doc without the router's <al-shown> element, and the hash it carries ("" for none).
// The router puts it at the end of the <body>: it is looked for there first, without a walk
// of the whole page.
func takeShown(doc html.Node) (html.Node, string) {
	var shown html.Node
	if body, ok := bodyNode(doc, 2); ok {
		cs := body.Children()
		for i := len(cs) - 1; i >= 0 && i >= len(cs)-4; i-- {
			if isShown(cs[i]) {
				shown = cs[i]
				break
			}
		}
	}
	if shown.Tag() == "" {
		els := html.Select(doc, selShown)
		if len(els) == 0 {
			return doc, ""
		}
		shown = els[len(els)-1]
	}
	h, _ := html.AttrOf(shown, "data-h")
	return html.Without(doc, isShown), h
}

// bodyNode is the <body> of n found within depth levels (n itself, its children, …).
func bodyNode(n html.Node, depth int) (html.Node, bool) {
	if n.Tag() == "body" {
		return n, true
	}
	if depth == 0 {
		return html.Node{}, false
	}
	for _, c := range n.Children() {
		if c.Tag() == "body" || c.Tag() == "html" || c.Tag() == "" {
			if b, ok := bodyNode(c, depth-1); ok {
				return b, true
			}
		}
	}
	return html.Node{}, false
}

func isShown(n html.Node) bool {
	id, _ := html.AttrOf(n, "id")
	return n.Tag() == "al-shown" && id == datastar.ShownID
}

// DriftError is a step whose answer, applied to the page the browser held, did not give the
// page a full render shows: the router's fragments (html.Fragments) or the before-view are
// wrong. Every click and submit of every test checks it.
type DriftError struct {
	Step string // "PATCH /todos/7"
	Diff html.Difference
}

func (e *DriftError) Error() string {
	return fmt.Sprintf("%s: the patched page differs from a full render at %s: %s\n  patched: %s\n  full:    %s",
		e.Step, e.Diff.At, e.Diff.What, e.Diff.Left, e.Diff.Right)
}

// checkPatched compares what the last request's answer patched into the held page, as the
// browser built and patched it (dom), with the full render, as the browser's parser builds it,
// of the page the request re-rendered (b.rendered): the whole body when the answer morphed it
// (the router's implicit <al-toasts> region left out), else each region it patched (`#id`),
// by id. A region the answer did not send is not compared: the spec guarantees every region the
// server sent is right, and leaves the rest as the browser has them (#160, the user's 2026-11-04
// decision: no shown-hash gate), so a page held stale keeps its stale regions. It runs after
// the request was served: `alder bench` measures the router alone (serveHTTP), not this.
func (b *Browser) checkPatched() error {
	if b.rendered == "" || (!b.morphed && len(b.regions) == 0) {
		return nil
	}
	full, err := html.Parse(b.rendered)
	if err != nil {
		return fmt.Errorf("the full render of the page does not parse: %w", err)
	}
	want := html.Repair(full)
	step := strings.TrimSuffix(b.reqHead, ": ")
	if !b.morphed {
		// the page as it should be: the one held before the answer, each region the answer
		// patched as the full render has it. Compared whole, so a patch whose markup the
		// parser moves outside its element (html.Repair) is caught as well.
		expect := b.domBefore
		for _, id := range b.regions {
			w, ok := elementByID(want, id)
			if ok {
				expect, ok = html.ReplaceByID(expect, id, w)
			}
			if !ok {
				return &DriftError{Step: step, Diff: html.Difference{At: "#" + id,
					What: "the answer patched a region that the held page or the full render lacks"}}
			}
		}
		want = expect
	}
	held := html.Select(b.dom, selBody)
	if len(held) == 0 {
		return errors.New("the held page has no <body>")
	}
	got := held[0]
	if !strings.Contains(b.rendered, "<al-toasts") {
		got = html.Without(got, func(n html.Node) bool { return n.Tag() == "al-toasts" })
	}
	if bs := html.Select(want, selBody); !b.morphed && len(bs) > 0 {
		want = bs[0]
		if !strings.Contains(b.rendered, "<al-toasts") {
			want = html.Without(want, func(n html.Node) bool { return n.Tag() == "al-toasts" })
		}
	}
	if d, differ := html.Diff(got, want); differ {
		return &DriftError{Step: step, Diff: d}
	}
	return nil
}

// elementByID is n's first element whose id is id, in document order.
func elementByID(n html.Node, id string) (html.Node, bool) {
	if v, ok := html.AttrOf(n, "id"); ok && v == id && n.Tag() != "" {
		return n, true
	}
	for _, c := range n.Children() {
		if e, ok := elementByID(c, id); ok {
			return e, true
		}
	}
	return html.Node{}, false
}

// failText is the error of the request just served, as the router logged it ("POST /x?a=1: boom
// rid=5f3a2c1e" and a stack for a panic): the message alone, without the request (its method and
// masked URL, which holds no space), the id or the stack.
func (b *Browser) failText() string {
	msg := b.lastErr
	if i := strings.Index(msg, "\ngoroutine "); i >= 0 {
		msg = msg[:i]
	}
	if i := strings.LastIndex(msg, " rid="); i >= 0 && !strings.ContainsAny(msg[i+1:], " \n") {
		msg = msg[:i]
	}
	msg = failHead.ReplaceAllString(msg, "")
	if msg == "" {
		msg = "internal server error"
	}
	return msg
}

// apply applies w, the answer to a Datastar request, to the held page: a redirect followed, a
// navigation made, the patches applied (and checked: checkPatched). Its Answer is RequestFailed
// or PageMissing for the error an Alder-Status names, Redirected for a redirect, else
// FormRejected when the page shows errors and Refreshed when it does not (sendRoute then reads
// what the route answered).
func (b *Browser) apply(w *httptest.ResponseRecorder) (Answer, error) {
	if w.Code == http.StatusSeeOther { // not under Datastar, but a client follows it all the same
		return b.navigate(w.Header().Get("Location"), true)
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
		return b.document(b.url, w)
	}
	events, err := sseEvents(w.Body.String())
	if err != nil {
		return Answer{}, err
	}
	pushed := ""
	for _, ev := range events {
		if loc, ok := navigation(ev); ok {
			return b.navigate(loc, true)
		}
		if loc, ok := pushOf(ev); ok {
			pushed = loc
			continue
		}
		if attrs, ok := htmlAttrsOf(ev); ok {
			b.setHtmlAttrs(attrs)
			continue
		}
		if err := b.patch(ev); err != nil {
			return Answer{}, err
		}
	}
	if pushed != "" {
		// a redirect the router followed itself: the target's body, title and URL, as if the
		// browser had loaded it (the error, if it failed, is logged for the GET of it)
		b.url = pushed
		if p, err := url.Parse(pushed); err == nil {
			b.reqHead = http.MethodGet + " " + p.Path + ": "
		}
		switch w.Header().Get(web.StatusHeader) {
		case "400":
			return Answer{Tag: RequestFailed, Text: b.refusedText()}, nil
		case "404":
			return Answer{Tag: PageMissing}, nil
		case "500":
			return Answer{Tag: RequestFailed, Text: b.failText()}, nil
		case "503":
			return Answer{Tag: RequestFailed, Text: maintenanceText}, nil
		}
		if err := b.checkPatched(); err != nil { // the page morphed in, against its full render
			return Answer{}, err
		}
		return Answer{Tag: Redirected, Page: pageOf(pushed)}, nil
	}
	switch w.Header().Get(web.StatusHeader) {
	case "400": // a query that does not parse: the router's page says which parameter
		return Answer{Tag: RequestFailed, Text: b.refusedText()}, nil
	case "404":
		return Answer{Tag: PageMissing}, nil
	case "500":
		return Answer{Tag: RequestFailed, Text: b.failText()}, nil
	case "503":
		return Answer{Tag: RequestFailed, Text: maintenanceText}, nil
	}
	if err := b.checkPatched(); err != nil {
		return Answer{}, err
	}
	if errs := pageErrors(b.doc); len(errs) > 0 {
		return Answer{Tag: FormRejected, Errors: errs}, nil
	}
	return Answer{Tag: Refreshed}, nil
}

// sendRoute sends c's request for route r (routeControl) as the page's script does, with
// fields — url-encoded when c sends a form — and applies its answer. Its headers are the
// script's: Datastar-Request, Alder-Page (the held URL: location.pathname + location.search),
// Alder-Build (the held page's build, "" for none) and, for an element fetch (window._alderF)
// Alder-Fetch, else (window._alderO) Alder-Shown. Without a form, Datastar puts its empty
// payload in the query of a GET or DELETE (`datastar={}`, which rt/web drops) and sends it as a
// JSON body otherwise. The answer (apply) is then what the route answered: an element fetch's
// and a write's `render r` Rendered (PageMissing or RequestFailed when the element was not
// patched: the route's 404 or failure, toasted), `refresh` Refreshed, `invalid` (and a form
// that did not decode) FormRejected with the errors the page shows.
func (b *Browser) sendRoute(c routeCtl, r rt.Route, fields url.Values) (Answer, error) {
	o := outgoing{method: c.method, target: c.url, ds: true, header: http.Header{}}
	o.header.Set(html.PageHeader, b.url)
	o.header.Set(datastar.BuildHeader, b.build)
	if c.fetch {
		o.header.Set(datastar.FetchHeader, "1")
	} else {
		o.header.Set(html.ShownHeader, b.shown)
	}
	switch {
	case c.hasForm:
		o.body, o.ctype = fields.Encode(), "application/x-www-form-urlencoded"
	case c.method == http.MethodGet || c.method == http.MethodDelete:
		sep := "?"
		if strings.Contains(o.target, "?") {
			sep = "&"
		}
		o.target += sep + "datastar=" + url.QueryEscape("{}")
	default:
		o.body, o.ctype = "{}", "application/json"
	}
	w, err := b.send(o)
	if err != nil {
		return Answer{}, err
	}
	ans, err := b.apply(w)
	if err != nil || (ans.Tag != Refreshed && ans.Tag != FormRejected) {
		return ans, err // a redirect followed, a navigation, a failure
	}
	if c.fetch {
		return b.checkedElement(r)
	}
	if a := b.answered; a != nil {
		switch a.Kind {
		case rt.RespRender:
			return b.checkedElement(a.Target)
		case rt.RespRefresh:
			return Answer{Tag: Refreshed}, nil
		case rt.RespInvalid:
			errs := pageErrors(b.doc)
			if errs == nil {
				errs = []web.FieldError{}
			}
			return Answer{Tag: FormRejected, Errors: errs}, nil
		}
	}
	return ans, nil
}

// elementAnswer is the answer of a request whose answer patches route r's view in by its id:
// Rendered r when it patched an element; else (rt/web's renderFailed: the route answered no
// view, and a toast says so) RequestFailed with the error logged, or PageMissing without one.
func (b *Browser) elementAnswer(r rt.Route) Answer {
	switch {
	case len(b.regions) > 0:
		return Answer{Tag: Rendered, Page: r}
	case b.lastErr != "":
		return Answer{Tag: RequestFailed, Text: b.failText()}
	}
	return Answer{Tag: PageMissing}
}

// checkedElement is elementAnswer(r), and when it is Rendered r, the element check
// (checkElement) has passed.
func (b *Browser) checkedElement(r rt.Route) (Answer, error) {
	ans := b.elementAnswer(r)
	if ans.Tag != Rendered {
		return ans, nil
	}
	if err := b.checkElement(r); err != nil {
		return Answer{}, err
	}
	return ans, nil
}

// ElementError is the element check's failure (#160): the element an answer patched in for
// route Route is not Route's view of its load rendered on its own (Fetch: Route's URL fetched
// again as an element), as the browser's parser builds both. Diff says what differs; without
// one, Why says why the fetch gave no view to compare.
type ElementError struct {
	Step  string // the step's request, "POST /r/rows/1"
	ID    string // the element patched, "row-1"
	Route string // the route's name, "R.Row"
	Fetch string // "GET /r/rows/1"
	Why   string
	Diff  *html.Difference
}

// Error is "POST /r/rows/1: the answer patched #row-1 unlike R.Row's view rendered on its own
// (GET /r/rows/1) at li#row-1: <what>", then the two elements' markup.
func (e *ElementError) Error() string {
	head := fmt.Sprintf("%s: the answer patched #%s unlike %s's view rendered on its own (%s)", e.Step, e.ID, e.Route, e.Fetch)
	if e.Diff == nil {
		return head + ": " + e.Why
	}
	return fmt.Sprintf("%s at %s: %s\n  patched: %s\n  alone:   %s", head, e.Diff.At, e.Diff.What, e.Diff.Left, e.Diff.Right)
}

// checkElement is the element check (#160): after an answer that patched route r's view in by
// its root id (an element fetch, a write's `render r`), r's URL is fetched again as the page's
// script fetches an element (Alder-Fetch, the held page's Alder-Page and Alder-Build, the
// cookie), and the element the held page now has must equal the view that fetch answers, as the
// browser's parser builds both. Equal by construction; the check proves it. The fetch runs in a
// savepoint of the test's transaction that is rolled back (quietly): it sees the step's writes
// and leaves none, and it is not part of the step. The savepoint undoes database writes only:
// the fetch runs the route's `load` a second time, so a Query capability it calls runs twice,
// a test's fake too. The fake's tables are rolled back with the savepoint; any Go-side state it
// keeps sees both calls. Replaying the first run's capability calls instead would need a hook
// per capability call in rt/eff and generated wrappers: not worth it for a check (#177).
func (b *Browser) checkElement(r rt.Route) error {
	id := b.regions[len(b.regions)-1]
	u := rt.RouteURL(r)
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	ctx := b.env.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req := httptest.NewRequest(http.MethodGet, u+sep+"datastar="+url.QueryEscape("{}"), nil).WithContext(ctx)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set(datastar.FetchHeader, "1")
	req.Header.Set(html.PageHeader, b.url)
	req.Header.Set(datastar.BuildHeader, b.build)
	if b.url != "" {
		req.Header.Set("Referer", "http://example.com"+b.url)
	}
	if b.cookie != "" {
		req.AddCookie(&http.Cookie{Name: web.CookieName, Value: b.cookie})
	}
	fail := &ElementError{Step: strings.TrimSuffix(b.reqHead, ": "), ID: id, Route: r.Name, Fetch: http.MethodGet + " " + u}
	if fail.Route == "" {
		fail.Route = u
	}
	w, err := b.quietly(ctx, req)
	if err != nil {
		return err
	}
	var alone html.Node
	found := false
	if strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
		events, err := sseEvents(w.Body.String())
		if err != nil {
			return err
		}
		for _, ev := range events {
			sel := ev.field("selector")
			if ev.typ != "datastar-patch-elements" || ev.field("mode") != "outer" || !strings.HasPrefix(sel, "#") || sel == "#_alder_panel" {
				continue
			}
			if sel != "#"+id {
				fail.Why = "on its own its root is " + sel
				return fail
			}
			n, err := html.Parse(ev.field("elements"))
			if err != nil {
				return fmt.Errorf("the element check: the view %s answers does not parse: %w", fail.Fetch, err)
			}
			alone, found = html.RepairFragment(n), true
		}
	}
	if !found {
		status := strconv.Itoa(w.Code)
		if s := w.Header().Get(web.StatusHeader); s != "" {
			status = s
		}
		fail.Why = "on its own it answered no view (status " + status + ")"
		return fail
	}
	want, ok := elementByID(alone, id)
	if !ok {
		fail.Why = "on its own the view has no element with id " + id
		return fail
	}
	got, ok := elementByID(b.dom, id)
	if !ok {
		fail.Why = "the page the browser built has no element with that id"
		return fail
	}
	if d, differ := html.Diff(got, want); differ {
		fail.Diff = &d
		return fail
	}
	return nil
}

// quietly has the router serve req, the element check's fetch, within a savepoint of the test's
// transaction that is rolled back afterwards. It is not part of the step: what the request's
// hooks record (its Envs, so its trace and statements; the model; the render; the answer; an
// error logged) is put back as the step left it, it is not measured, and a cookie it sets is not
// kept. A savepoint that cannot be rolled back or released is the check's error: the step
// fails rather than go on in a transaction holding the fetch's writes.
func (b *Browser) quietly(ctx context.Context, req *http.Request) (_ *httptest.ResponseRecorder, err error) {
	reqEnvs, model, rendered, answered, lastErr := b.reqEnvs, b.model, b.rendered, b.answered, b.lastErr
	defer func() {
		b.reqEnvs, b.model, b.rendered, b.answered, b.lastErr = reqEnvs, model, rendered, answered, lastErr
		clear(b.heads)
	}()
	if w := b.cur; w != nil && w.Savepoints != nil {
		sp := eff.SavepointName()
		if err := w.Savepoints.Savepoint(ctx, w.Tx, sp); err != nil {
			return nil, fmt.Errorf("the element check: %w", err)
		}
		defer func() {
			rerr := w.Savepoints.RollbackTo(ctx, w.Tx, sp)
			if rerr == nil {
				rerr = w.Savepoints.Release(ctx, w.Tx, sp)
			}
			if rerr != nil && err == nil {
				err = fmt.Errorf("the element check: undoing its fetch: %w", rerr)
			}
		}()
	}
	rec := httptest.NewRecorder()
	b.handler.ServeHTTP(rec, req)
	return rec, nil
}

// pageErrors is the errors a page shows: the items of each `<ul class="errors" data-errors>`
// list in document order, Field the list's data-errors ("" for the form-level list, "form").
func pageErrors(doc html.Node) []web.FieldError {
	var out []web.FieldError
	for _, ul := range html.Select(doc, selErrors) {
		field, _ := html.AttrOf(ul, "data-errors")
		if field == "form" {
			field = ""
		}
		for _, li := range html.Select(ul, selLi) {
			out = append(out, web.FieldError{Field: field, Message: html.TextContent(li)})
		}
	}
	return out
}

// sseEvent is one server-sent event: its type and data lines, each split at its first space
// into a key and a value ("selector body", "elements <p>…").
type sseEvent struct {
	typ  string
	data [][2]string
}

func sseEvents(body string) ([]sseEvent, error) {
	var out []sseEvent
	var cur sseEvent
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if cur.typ != "" || len(cur.data) > 0 {
				out = append(out, cur)
			}
			cur = sseEvent{}
		case strings.HasPrefix(line, "event: "):
			cur.typ = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			k, v, _ := strings.Cut(strings.TrimPrefix(line, "data: "), " ")
			cur.data = append(cur.data, [2]string{k, v})
		}
	}
	if cur.typ != "" || len(cur.data) > 0 {
		out = append(out, cur)
	}
	return out, sc.Err()
}

// field is the value of ev's data lines with key k, joined by newlines (elements span several).
func (ev sseEvent) field(k string) string {
	var vs []string
	for _, d := range ev.data {
		if d[0] == k {
			vs = append(vs, d[1])
		}
	}
	return strings.Join(vs, "\n")
}

// navigation is the URL a datastar.Navigate event sends the browser to.
func navigation(ev sseEvent) (string, bool) {
	if ev.typ != "datastar-patch-elements" || ev.field("mode") != "append" {
		return "", false
	}
	_, rest, ok := strings.Cut(ev.field("elements"), "window.location.assign(")
	if !ok {
		return "", false
	}
	end := strings.LastIndex(rest, ")</script>")
	if end < 0 {
		return "", false
	}
	var u string
	if json.Unmarshal([]byte(rest[:end]), &u) != nil {
		return "", false
	}
	return u, true
}

// pushOf is the URL of a history step (datastar.Navigate in mode Push or Replace) the router
// ends a followed redirect's answer with: the script begins `(function(){var u=<JSON URL>;`.
func pushOf(ev sseEvent) (string, bool) {
	if ev.typ != "datastar-patch-elements" || ev.field("mode") != "append" {
		return "", false
	}
	els := ev.field("elements")
	_, rest, ok := strings.Cut(els, "(function(){var u=")
	if !ok || !(strings.Contains(rest, "history.pushState(") || strings.Contains(rest, "history.replaceState(")) {
		return "", false
	}
	var u string
	if json.NewDecoder(strings.NewReader(rest)).Decode(&u) != nil {
		return "", false
	}
	return u, true
}

// htmlAttrsOf is the attributes a datastar.PatchHtmlAttrs event gives the page's <html>.
func htmlAttrsOf(ev sseEvent) ([][2]string, bool) {
	if ev.typ != "datastar-patch-elements" || ev.field("selector") != "body" || ev.field("mode") != "append" {
		return nil, false
	}
	return datastar.HtmlAttrsOf(ev.field("elements"))
}

// setHtmlAttrs applies a datastar.PatchHtmlAttrs event to the held page's <html> element as its
// script does in a browser: each of attrs is set, and each name the server set before
// (htmlNames) that attrs no longer names is removed; any other attribute stays. A page without
// <html> is left as it is.
func (b *Browser) setHtmlAttrs(attrs [][2]string) {
	els := html.Select(b.doc, selHTML)
	if len(els) == 0 {
		return
	}
	named := map[string]bool{}
	for _, a := range attrs {
		named[strings.ToLower(a[0])] = true
	}
	var as []html.Attr
	set := map[string]bool{}
	for _, a := range html.AttrsOf(els[0]) {
		switch {
		case named[a[0]]:
			for _, n := range attrs {
				if strings.ToLower(n[0]) == a[0] {
					a[1] = n[1]
				}
			}
			set[a[0]] = true
		case slices.Contains(b.htmlNames, a[0]):
			continue // the server set it before and no longer does
		}
		as = append(as, html.A(a[0], a[1]))
	}
	for _, a := range attrs {
		if n := strings.ToLower(a[0]); !set[n] {
			as, set[n] = append(as, html.A(n, a[1])), true
		}
	}
	b.htmlNames = slices.Collect(maps.Keys(named))
	if doc, ok := html.ReplaceElement(b.doc, "html", html.El("html", as, els[0].Children()...)); ok {
		b.doc = doc
		b.dom = html.Repair(b.doc)
	}
}

// heldHTMLNames is the names on the <html> element of a page just held whole: the server set
// them all, so the first attrs event may remove them (datastar.HtmlSeed's list in a browser).
func heldHTMLNames(doc html.Node) []string {
	els := html.Select(doc, selHTML)
	if len(els) == 0 {
		return nil
	}
	var names []string
	for _, a := range html.AttrsOf(els[0]) {
		names = append(names, a[0])
	}
	return names
}

// Toast is one toast a step's answer showed: a test's `{ kind : Text, text : Text }` (an alias,
// so the generated code's struct of those fields is this type).
type Toast = struct {
	Kind string
	Text string
}

// toastOf is the toast the router's <al-toast> el shows: its kind read back from its variant
// (al's "destructive" is an error).
func toastOf(el html.Node) Toast {
	kind, _ := html.AttrOf(el, "variant")
	if kind == "destructive" {
		kind = string(rt.NoticeError)
	}
	return Toast{Kind: kind, Text: strings.TrimSpace(html.TextContent(el))}
}

// patch applies a datastar-patch-elements event to the held document: the router patches the
// <body> (outer: replace it; inner: replace its children; append: add to them) and, after a
// followed redirect, the <title> (selector `head > title`: replaced, when the page has one).
func (b *Browser) patch(ev sseEvent) error {
	if ev.typ != "datastar-patch-elements" {
		return nil
	}
	sel, mode := ev.field("selector"), ev.field("mode")
	if sel == "head > title" {
		t, err := html.Parse(ev.field("elements"))
		if err != nil {
			return fmt.Errorf("the title patch does not parse: %w", err)
		}
		if doc, ok := html.ReplaceElement(b.doc, "title", t); ok {
			b.doc = doc
		}
		return nil
	}
	els, err := html.Parse(ev.field("elements"))
	if err != nil {
		return fmt.Errorf("the patch does not parse: %w", err)
	}
	if sel == "#_alder_panel" {
		return nil // the dev panel is not part of the page a test sees
	}
	if sel == "#"+datastar.ShownID {
		_, b.shown = takeShown(els)
		return nil
	}
	if id, ok := strings.CutPrefix(sel, "#"); ok && mode == "outer" {
		doc, ok := html.ReplaceByID(b.doc, id, els)
		if !ok {
			// a real browser drops it silently (a `render` of a row the page filtered out): a
			// test fails the step, naming the id
			return fmt.Errorf("%sthe answer patched `%s`, which the page does not have", b.reqHead, sel)
		}
		b.doc = doc
		b.regions = append(b.regions, id)
		// Datastar morphs the element into the nodes its parser builds from the patch; an id
		// the DOM lacks patches nothing (the drift check then reports the difference)
		if dom, ok := html.ReplaceByID(b.dom, id, html.RepairFragment(els)); ok {
			b.dom = dom
		}
		return nil
	}
	if sel == "al-toasts" && mode == "append" {
		b.toasts = append(b.toasts, toastOf(els))
		return nil
	}
	if sel != "body" {
		return fmt.Errorf("a patch of %q: the browser only patches the body", sel)
	}
	doc := b.doc
	if !b.hasDoc {
		doc = html.El("html", nil, html.El("body", nil))
	}
	var ok bool
	switch mode {
	case "outer", "":
		body := els
		if els.Tag() != "body" {
			body = html.El("body", nil, els)
		}
		doc, ok = html.ReplaceElement(doc, "body", body)
	case "inner":
		doc, ok = html.ReplaceElement(doc, "body", html.El("body", nil, els))
	case "append":
		doc, ok = html.AppendTo(doc, "body", els)
	default:
		return fmt.Errorf("a patch in mode %q: the browser does not apply it", mode)
	}
	if !ok {
		return errors.New("the held page has no <body> to patch")
	}
	b.hasDoc = true
	if mode == "append" {
		b.doc = doc
	} else {
		b.doc, b.shown = takeShown(doc)
		b.morphed = true
	}
	b.dom = html.Repair(b.doc)
	return nil
}

// external reports whether u leads to another site: it has a scheme or a host.
func external(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme != "" || p.Host != "")
}

// pageOf is the Route a URL of this site links to: its escaped path and its query in order,
// with no name or method.
func pageOf(u string) rt.Route {
	p, err := url.Parse(u)
	if err != nil {
		return rt.Route{Path: u}
	}
	page := rt.Route{Path: p.EscapedPath()}
	if p.RawQuery == "" {
		return page
	}
	for _, kv := range strings.Split(p.RawQuery, "&") {
		k, v, _ := strings.Cut(kv, "=")
		k, _ = url.QueryUnescape(k)
		v, _ = url.QueryUnescape(v)
		page.Query = append(page.Query, rt.KV{Key: k, Value: v})
	}
	return page
}
