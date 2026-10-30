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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"alder/rt"
	"alder/rt/eff"
	"alder/rt/html"
	"alder/rt/web"
)

// AnswerTag says what a browser step's request came to: the Prelude `Answer`'s constructor.
type AnswerTag int

const (
	Shown         AnswerTag = iota // a GET showed its page
	Rerendered                     // a message re-rendered the page (the body patch was applied)
	Redirected                     // the answer was a redirect; the browser fetched Answer.Page
	FormRejected                   // a form came back with errors (Answer.Errors)
	PageMissing                    // 404: the not-found document is held
	RequestFailed                  // 500 (or a request the router refused): Answer.Text says why
)

func (t AnswerTag) String() string {
	switch t {
	case Shown:
		return "Shown"
	case Rerendered:
		return "Rerendered"
	case Redirected:
		return "Redirected"
	case FormRejected:
		return "FormRejected"
	case PageMissing:
		return "PageMissing"
	case RequestFailed:
		return "RequestFailed"
	}
	return "AnswerTag(" + strconv.Itoa(int(t)) + ")"
}

// Answer is the Prelude `Answer`: Tag, and the page of Redirected, the errors of FormRejected (in
// document order: each `<Errors>` list's items, Field "" for the form-level list) or the text
// of RequestFailed.
type Answer struct {
	Tag    AnswerTag
	Page   rt.Page
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
	// Notices are the toasts the step's answer showed (a Noticed answer), in order.
	Notices []string
	// Model is the model the page held was rendered from when the last request the step made
	// was a page's GET (web.Request.Model; Test.get's typed model), nil otherwise.
	Model any
}

// TestSecret is the cookie key of a test's router when its options carry none (the router
// refuses a short one outside dev mode).
var TestSecret = []byte("alder-test-cookie-key: tests only")

// cookieNow is the clock of the browser's router (cookie expiry only): fixed, so the cookies of
// a test never expire and reading it does not advance the test's own clock.
var cookieNow = func() time.Time { return defaultNow }

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

	cookie  string    // the session cookie's value, "" for none
	url     string    // the URL of the page held, "" before the first
	doc     html.Node // the page held (without the router's doctype and script tag)
	hasDoc  bool
	notices []string // the toasts of the step's requests (Result.Notices)

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
	b := &Browser{env: env, codec: opts.Session, login: opts.Login, heads: map[*eff.Env]string{}}
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
		s := head + "redirected to " + rt.PageURL(e.Answer.Page)
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
// the message as the test wrote it — the browser fills it from the URL (MsgLabel); the
// generated code may replace it with the message's Alder form (`Todos.Index.Toggle { id: "…"
// }`) — and Form the form's name for a Submit ("" when not known). Page is the held page's URL;
// Offers, when the page renders the same message with other parameters, says which (Offered).
type NoControlError struct {
	Submit bool
	Form   string
	Msg    string
	URL    string
	Page   string
	Offers string
}

// Error is "no page is open: `Test.get` one first" before the browser holds a page (Page ""),
// else "the page / has no form NewTodo (Todos.Index.Submit)" for a Submit ("the page / has no
// form for Todos.Index.Submit" without Form) and "the page / has no button or form for
// Todos.Index.Toggle { id: "…" }" for a Click, each followed by " (the page has Toggle for id:
// a, b)" when the page renders the message with other parameters.
func (e *NoControlError) Error() string {
	if e.Page == "" {
		return "no page is open: `Test.get` one first"
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

// MsgLabel is a message URL (/_m/<Module>/<Msg>?k=v) as the test would write the message:
// `Module.Msg`, then its own parameters (not the page's `_p.` ones) as a record of text values
// in URL order, `Todos.Index.Toggle { id: "a" }`.
func MsgLabel(msgURL string) string {
	u, err := url.Parse(msgURL)
	if err != nil {
		return msgURL
	}
	name := strings.TrimPrefix(u.Path, "/_m/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[:i] + "." + name[i+1:]
	}
	var fields []string
	for _, kv := range strings.Split(u.RawQuery, "&") {
		k, v, _ := strings.Cut(kv, "=")
		k, _ = url.QueryUnescape(k)
		v, _ = url.QueryUnescape(v)
		if k == "" || strings.HasPrefix(k, rt.PagePrefix) {
			continue
		}
		fields = append(fields, k+": "+strconv.Quote(v))
	}
	if len(fields) == 0 {
		return name
	}
	return name + " { " + strings.Join(fields, ", ") + " }"
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
	notices, model := b.notices, b.model
	defer func() { b.notices, b.model = notices, model }()
	w, err := b.serve(http.MethodGet, u, nil, false)
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

// Submit posts fields to the form on the held page whose action is msgURL — compared as the
// message and its own parameters, so the page's `_p.` parameters the rendered URL carries are
// sent as rendered — as Datastar does (url-encoded, Datastar-Request: true), and applies the
// answer: Rerendered (the body patch replaces the held body), FormRejected (the same, when the
// re-rendered page shows errors), Redirected (the target fetched and held), PageMissing, RequestFailed.
// With no such form the error is a *NoControlError, and with a field the form has no input for
// a *NoInputError (Fields.For); either way nothing is sent.
func (b *Browser) Submit(msgURL string, fields url.Values) (Result, error) {
	return b.submit(msgURL, "", Fields{Values: fields})
}

// submit is Submit of fields, form the form's name for the errors.
func (b *Browser) submit(msgURL, form string, fields Fields) (Result, error) {
	defer b.track()()
	target, f, ok := b.formFor(msgURL)
	if !ok {
		return Result{}, &NoControlError{Submit: true, Form: form, Msg: MsgLabel(msgURL), URL: msgURL, Page: b.url,
			Offers: Offered(msgURL, b.controlURLs(true))}
	}
	send, err := fields.For(f, form)
	if err != nil {
		return Result{}, err
	}
	ans, err := b.post(target, send)
	if err != nil {
		return Result{}, err
	}
	return b.result(ans), nil
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

// Offered is what the held page offers for the message of msgURL with other parameters, from
// the URLs its controls post (urls): the message's name, then each control's own parameters in
// page order, each set once — "Toggle for id: a, b" when every control has the same single
// parameter, else "Rename for id: a, x: 1; id: b, x: 2"; "" when no control posts the message
// with parameters.
func Offered(msgURL string, urls []string) string {
	want, err := url.Parse(msgURL)
	if err != nil {
		return ""
	}
	type set struct{ keys, text, value string }
	var sets []set
	for _, u := range urls {
		p, err := url.Parse(u)
		if err != nil || p.Path != want.Path {
			continue
		}
		var keys, own []string
		value := ""
		for _, kv := range strings.Split(p.RawQuery, "&") {
			k, v, _ := strings.Cut(kv, "=")
			k, _ = url.QueryUnescape(k)
			v, _ = url.QueryUnescape(v)
			if k == "" || strings.HasPrefix(k, rt.PagePrefix) {
				continue
			}
			keys, own, value = append(keys, k), append(own, k+": "+v), v
		}
		s := set{strings.Join(keys, ","), strings.Join(own, ", "), value}
		if len(own) > 0 && !slices.Contains(sets, s) {
			sets = append(sets, s)
		}
	}
	if len(sets) == 0 {
		return ""
	}
	name := want.Path[strings.LastIndexByte(want.Path, '/')+1:]
	var texts, values []string
	single := !strings.Contains(sets[0].keys, ",")
	for _, s := range sets {
		texts, values = append(texts, s.text), append(values, s.value)
		single = single && s.keys == sets[0].keys
	}
	if single {
		return name + " for " + sets[0].keys + ": " + strings.Join(values, ", ")
	}
	return name + " for " + strings.Join(texts, "; ")
}

// Click posts the message msgURL as the held page's control for it would: a `click` button (its
// formaction, or its own form's action), a form posting it, or any element whose data-on:click
// posts it — compared as the message and its own parameters; the control's URL, with the page's
// `_p.` parameters, is what is posted. The answer is applied as for Submit. With no such control
// the error is a *NoControlError and nothing is sent.
func (b *Browser) Click(msgURL string) (Result, error) {
	defer b.track()()
	target, ok := b.control(msgURL)
	if !ok {
		return Result{}, &NoControlError{Msg: MsgLabel(msgURL), URL: msgURL, Page: b.url, Offers: Offered(msgURL, b.controlURLs(false))}
	}
	ans, err := b.post(target, url.Values{})
	if err != nil {
		return Result{}, err
	}
	return b.result(ans), nil
}

var (
	selBody     = html.MustParseSelector("body")
	selForm     = html.MustParseSelector("form")
	selAction   = html.MustParseSelector("[formaction]")
	selClick    = html.MustParseSelector("[data-on:click]")
	selClickP   = html.MustParseSelector("[data-on:click__prevent]")
	selErrors   = html.MustParseSelector("ul[data-errors]")
	selLi       = html.MustParseSelector("li")
	selInput    = html.MustParseSelector("input")
	selSelect   = html.MustParseSelector("select")
	selTextarea = html.MustParseSelector("textarea")
)

// control is the URL the held page's control for msgURL posts: a <form>'s action or
// data-on:submit, a [formaction] button's or any data-on:click's.
func (b *Browser) control(msgURL string) (string, bool) {
	want, ok := msgKey(msgURL)
	if !ok {
		return "", false
	}
	for _, u := range b.controlURLs(false) {
		if k, ok := msgKey(u); ok && k == want {
			return u, true
		}
	}
	return "", false
}

// formFor is the URL the held page's form for msgURL posts, and the form (a form to submit:
// not a click button's own).
func (b *Browser) formFor(msgURL string) (string, html.Node, bool) {
	want, ok := msgKey(msgURL)
	if !b.hasDoc || !ok {
		return "", html.Node{}, false
	}
	for _, f := range html.Select(b.doc, selForm) {
		if clickForm(f) {
			continue
		}
		for _, u := range formURLs(f) {
			if k, ok := msgKey(u); ok && k == want {
				return u, f, true
			}
		}
	}
	return "", html.Node{}, false
}

// clickForm reports whether form f is a click button's own form, which is not a form to submit.
func clickForm(f html.Node) bool {
	class, _ := html.AttrOf(f, "class")
	return slices.Contains(strings.Fields(class), "_alder-click")
}

// controlURLs are the URLs the held page's controls post: its forms' (for a submit only those,
// and not a click button's own form), then a [formaction] button's and any data-on:click's.
func (b *Browser) controlURLs(submit bool) []string {
	if !b.hasDoc {
		return nil
	}
	var urls []string
	for _, f := range html.Select(b.doc, selForm) {
		if !submit || !clickForm(f) {
			urls = append(urls, formURLs(f)...)
		}
	}
	if !submit {
		for _, n := range html.Select(b.doc, selAction) {
			a, _ := html.AttrOf(n, "formaction")
			urls = append(urls, a)
		}
		for _, sel := range []html.Selector{selClick, selClickP} {
			for _, n := range html.Select(b.doc, sel) {
				for _, name := range []string{"data-on:click", "data-on:click__prevent"} {
					if a, ok := html.AttrOf(n, name); ok {
						urls = append(urls, postURL(a))
					}
				}
			}
		}
	}
	return urls
}

// msgKey is rt.MsgKey of a message URL: its path and own parameters.
func msgKey(u string) (string, bool) {
	p, err := url.Parse(u)
	if err != nil || p.Path == "" {
		return "", false
	}
	return rt.MsgKey(p.Path, p.Query()), true
}

// postURL is the URL of a Datastar action `@post('<url>'…)`, "" for another.
func postURL(action string) string {
	_, rest, ok := strings.Cut(action, "@post('")
	if !ok {
		return ""
	}
	u, _, _ := strings.Cut(rest, "'")
	return u
}

// result is ans with the session the cookie now carries and the document held.
func (b *Browser) result(ans Answer) Result {
	return Result{Answer: ans, Session: b.Session(), Doc: b.doc, Model: b.model, Notices: b.notices}
}

// serve sends one request through the router: with the cookie, the held page as Referer (an App
// message redirects back to it), and for a Datastar request its header; it then keeps the cookie
// the answer sets and appends the effects the request ran to the test's trace.
func (b *Browser) serve(method, target string, form url.Values, ds bool) (*httptest.ResponseRecorder, error) {
	b.notices = nil
	cur, err := b.within()
	if err != nil {
		return nil, err
	}
	var req *http.Request
	if method == http.MethodPost {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if b.env.Ctx != nil {
		req = req.WithContext(b.env.Ctx)
	}
	if ds {
		req.Header.Set("Datastar-Request", "true")
	}
	if b.url != "" {
		req.Header.Set("Referer", "http://example.com"+b.url)
	}
	if b.cookie != "" {
		req.AddCookie(&http.Cookie{Name: web.CookieName, Value: b.cookie})
	}
	b.reqEnvs, b.lastErr, b.cur, b.model = nil, "", cur, nil
	b.reqHead = req.Method + " " + req.URL.Path + ": "
	w := httptest.NewRecorder()
	b.handler.ServeHTTP(w, req)
	for _, e := range b.reqEnvs {
		b.env.Trace = append(b.env.Trace, e.Trace...)
	}
	b.account(req.Method+" "+req.URL.Path, w.Body.Len())
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
			return Answer{Tag: Redirected, Page: rt.Page{Path: u}}, nil
		}
		w, err := b.serve(http.MethodGet, u, nil, false)
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
func (b *Browser) hold(u, page string) error {
	page = strings.TrimPrefix(page, "<!doctype html>\n")
	page = web.StripHeadTags(page)
	doc, err := html.Parse(page)
	if err != nil {
		return fmt.Errorf("the page at %s does not parse: %w", u, err)
	}
	b.url, b.doc, b.hasDoc = u, doc, true
	return nil
}

// failText is the error of the request just served, as the router logged it ("POST /x: boom"
// and a stack for a panic): the message alone.
func (b *Browser) failText() string {
	msg := b.lastErr
	if i := strings.Index(msg, "\ngoroutine "); i >= 0 {
		msg = msg[:i]
	}
	msg = strings.TrimPrefix(msg, b.reqHead)
	if msg == "" {
		msg = "internal server error"
	}
	return msg
}

// post sends a Datastar POST of form to target and applies its answer.
func (b *Browser) post(target string, form url.Values) (Answer, error) {
	w, err := b.serve(http.MethodPost, target, form, true)
	if err != nil {
		return Answer{}, err
	}
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
	if errs := pageErrors(b.doc); len(errs) > 0 {
		return Answer{Tag: FormRejected, Errors: errs}, nil
	}
	return Answer{Tag: Rerendered}, nil
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
	if sel == "al-toasts" && mode == "append" {
		b.notices = append(b.notices, strings.TrimSpace(html.TextContent(els)))
		return nil
	}
	if sel != "body" {
		return fmt.Errorf("a patch of %q: the browser only patches the body", sel)
	}
	if sel == "al-toasts" && mode == "append" {
		b.notices = append(b.notices, strings.TrimSpace(html.TextContent(els)))
		return nil
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
	b.doc, b.hasDoc = doc, true
	return nil
}

// external reports whether u leads to another site: it has a scheme or a host.
func external(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme != "" || p.Host != "")
}

// pageOf is the Page a URL of this site links to: its escaped path and its query in order.
func pageOf(u string) rt.Page {
	p, err := url.Parse(u)
	if err != nil {
		return rt.Page{Path: u}
	}
	page := rt.Page{Path: p.EscapedPath()}
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
