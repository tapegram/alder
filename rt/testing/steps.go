package rtesting

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"alder/rt"
	"alder/rt/eff"
	"alder/rt/html"
	"alder/rt/web"
)

// What the code generated for a test's browser steps (Test.login, get, open, submit,
// submitRaw, click) and for the Doc queries calls.

// Doc is the Prelude's `Doc`: a page's document, parsed (the node tree of rt/html). Two Docs
// are equal (`==`) when their trees are, without the page's parameters in their message URLs
// (html.WithoutPageParams): the page a step holds was rendered with its request's state, a view
// rendered by Test.doc without one, and they are the same document.
type Doc struct{ Node html.Node }

// DocOf is the document a step's result holds.
func DocOf(n html.Node) Doc { return Doc{Node: n} }

// ViewDoc is `Test.doc v`: the view v rendered (without a request's state) and parsed back.
func ViewDoc(v html.Node) Doc {
	n, err := html.Parse(html.Render(v))
	if err != nil { // the renderer's output always parses; keep the view itself if not
		return Doc{Node: v}
	}
	return Doc{Node: n}
}

// AlderEqual is Doc's `==` (rt.Equaler).
func (d Doc) AlderEqual(other any) bool {
	return html.WithoutPageParams(d.Node).Equal(html.WithoutPageParams(other.(Doc).Node))
}

// AlderKey is Doc's Dict key (rt.Keyer): its markup without the page's parameters.
func (d Doc) AlderKey() string { return html.Render(html.WithoutPageParams(d.Node)) }

// AlderShow prints a Doc as its markup (rt.Shower).
func (d Doc) AlderShow() (string, bool) { return html.Render(d.Node), false }

// String is the document's markup.
func (d Doc) String() string { return html.Render(d.Node) }

// The queries, by selector text (parsed when the query runs: one that does not parse matches
// nothing) and, with Sel, by a selector the generated code parsed once (a literal, which the
// checker has parsed too: E0023).

// DocText is `Test.text sel doc`: the first match's text, whitespace collapsed.
func DocText(sel string, d Doc) rt.Maybe[string] { return DocTextSel(parse(sel), d) }

// DocTexts is `Test.texts sel doc`: every match's text, in document order.
func DocTexts(sel string, d Doc) []string { return DocTextsSel(parse(sel), d) }

// DocHas is `Test.has sel doc`.
func DocHas(sel string, d Doc) bool { return DocHasSel(parse(sel), d) }

// DocCount is `Test.count sel doc`.
func DocCount(sel string, d Doc) int64 { return DocCountSel(parse(sel), d) }

// DocAttr is `Test.attr name sel doc`: attribute name of the first match (Nothing when it has
// none, or nothing matches).
func DocAttr(name, sel string, d Doc) rt.Maybe[string] { return DocAttrSel(name, parse(sel), d) }

// Selector is a query's selector, or none when its text did not parse (it then matches nothing).
type Selector struct {
	sel html.Selector
	ok  bool
}

func parse(s string) Selector {
	sel, err := html.ParseSelector(s)
	return Selector{sel, err == nil}
}

// Sel is a selector literal of a query, parsed once (html.MustParseSelector: the checker
// accepted it).
func Sel(s string) Selector { return Selector{html.MustParseSelector(s), true} }

func (s Selector) all(d Doc) []html.Node {
	if !s.ok {
		return nil
	}
	return html.Select(d.Node, s.sel)
}

// DocTextSel is DocText with a parsed selector.
func DocTextSel(s Selector, d Doc) rt.Maybe[string] {
	if ns := s.all(d); len(ns) > 0 {
		return rt.Just(html.TextContent(ns[0]))
	}
	return rt.Nothing[string]()
}

// DocTextsSel is DocTexts with a parsed selector.
func DocTextsSel(s Selector, d Doc) []string {
	out := []string{}
	for _, n := range s.all(d) {
		out = append(out, html.TextContent(n))
	}
	return out
}

// DocHasSel is DocHas with a parsed selector.
func DocHasSel(s Selector, d Doc) bool { return len(s.all(d)) > 0 }

// DocCountSel is DocCount with a parsed selector.
func DocCountSel(s Selector, d Doc) int64 { return int64(len(s.all(d))) }

// DocAttrSel is DocAttr with a parsed selector.
func DocAttrSel(name string, s Selector, d Doc) rt.Maybe[string] {
	if ns := s.all(d); len(ns) > 0 {
		if v, ok := html.AttrOf(ns[0], name); ok {
			return rt.Just(v)
		}
	}
	return rt.Nothing[string]()
}

// FieldError is the Alder record { field: Text, message: Text } of a FormRejected answer's list.
type FieldError = struct {
	Field   string
	Message string
}

// FieldErrors is the payload of FormRejected, as the Alder list of { field, message } records.
func (a Answer) FieldErrors() []FieldError {
	out := make([]FieldError, len(a.Errors))
	for i, e := range a.Errors {
		out[i] = FieldError{Field: e.Field, Message: e.Message}
	}
	return out
}

// FormRejectedAnswer is the Prelude's `FormRejected errs`.
func FormRejectedAnswer(errs []FieldError) Answer {
	a := Answer{Tag: FormRejected, Errors: []web.FieldError{}}
	for _, e := range errs {
		a.Errors = append(a.Errors, web.FieldError{Field: e.Field, Message: e.Message})
	}
	return a
}

// AlderEqual is Answer's `==` (rt.Equaler): the same constructor and the payload it names
// (a Redirected's page compared as its URL).
func (a Answer) AlderEqual(other any) bool {
	b := other.(Answer)
	if a.Tag != b.Tag {
		return false
	}
	switch a.Tag {
	case Redirected:
		return rt.PageURL(a.Page) == rt.PageURL(b.Page)
	case FormRejected:
		return rt.Eq(a.FieldErrors(), b.FieldErrors())
	case RequestFailed:
		return a.Text == b.Text
	}
	return true
}

// AlderKey is Answer's Dict key (rt.Keyer): its Show text.
func (a Answer) AlderKey() string { s, _ := a.AlderShow(); return s }

// AlderShow prints an Answer as Alder (rt.Shower): Shown, Redirected /login,
// FormRejected [{ field: "title", message: "must not be empty" }], RequestFailed "boom".
func (a Answer) AlderShow() (string, bool) {
	switch a.Tag {
	case Redirected:
		return "Redirected " + rt.PageURL(a.Page), true
	case FormRejected:
		return "FormRejected " + rt.Show(a.FieldErrors()), true
	case RequestFailed:
		return "RequestFailed " + rt.Show(a.Text), true
	}
	return a.Tag.String(), false
}

// SessionOf is a step result's session as the app's Session type S (its zero value when the
// browser has none: an app without a session).
func SessionOf[S any](v any) S {
	s, _ := v.(S)
	return s
}

// GetPage is Test.get's request: Get, with requires — the page's `requires` field, "" for
// none — named in a redirect to the login page ("GET /: redirected to /login (requires user)").
func (b *Browser) GetPage(u, requires string) (Result, error) {
	r, err := b.Get(u)
	var ns *NotShownError
	if errors.As(err, &ns) && requires != "" && ns.Answer.Tag == Redirected && b.login != "" &&
		strings.SplitN(rt.PageURL(ns.Answer.Page), "?", 2)[0] == b.login {
		ns.Requires = requires
	}
	return r, err
}

// ModelOf is Test.get's typed model: the model the page's GET rendered (Result.Model), or
// the error naming the request when there is none (a route that is not a page's).
func ModelOf[M any](u string, r Result) (M, error) {
	m, ok := r.Model.(M)
	if !ok {
		return m, fmt.Errorf("GET %s: the page gave no model", u)
	}
	return m, nil
}

// SubmitForm is Test.submit of a form by its name: the fields posted to the form on the held
// page whose message is msgPath (/_m/<Module>/<Msg>) — the message's own parameters, which the
// test did not write, are the rendered form's — as Submit posts them (only the fields the form
// has inputs for: Fields.For). Without such a form the error is a *NoControlError naming form
// ("the page / has no form NewTodo (Todos.Index.Submit)"); with several whose URLs differ (one
// per row) it is "the page has 2 forms for Todos.Index.Rename: write the message with its
// parameters".
func (b *Browser) SubmitForm(msgPath, form string, fields Fields) (Result, error) {
	var targets []string
	if b.hasDoc {
		for _, f := range html.Select(b.doc, selForm) {
			if clickForm(f) {
				continue
			}
			for _, u := range formURLs(f) {
				if p, err := url.Parse(u); err == nil && p.Path == msgPath {
					if !slices.Contains(targets, u) {
						targets = append(targets, u)
					}
					break
				}
			}
		}
	}
	switch len(targets) {
	case 0:
		return Result{}, &NoControlError{Submit: true, Form: form, Msg: MsgLabel(msgPath), URL: msgPath, Page: b.url}
	case 1:
		return b.submit(targets[0], form, fields)
	}
	return Result{}, fmt.Errorf("the page has %d forms for %s: write the message with its parameters", len(targets), MsgLabel(msgPath))
}

// SubmitMsg is Test.submit of a message with its parameters (`Test.submit (Save { id: id }) r`):
// Submit to the held page's form for msgURL (matched as the message and its own parameters),
// with form and label — the message as Alder writes it (MsgName) — in its error.
func (b *Browser) SubmitMsg(msgURL, form, label string, fields Fields) (Result, error) {
	r, err := b.submit(msgURL, form, fields)
	var nc *NoControlError
	if errors.As(err, &nc) {
		nc.Msg = label
	}
	return r, err
}

// formURLs are the URLs form f posts: its action and its data-on:submit's.
func formURLs(f html.Node) []string {
	var out []string
	if a, ok := html.AttrOf(f, "action"); ok {
		out = append(out, a)
	}
	if a, ok := html.AttrOf(f, "data-on:submit"); ok {
		out = append(out, postURL(a))
	}
	return out
}

// ClickMsg is Test.click's request: Click, with label — the message as the test wrote it,
// `Todos.Index.Toggle { id: "a" }` (MsgName) — in its error.
func (b *Browser) ClickMsg(msgURL, label string) (Result, error) {
	r, err := b.Click(msgURL)
	var nc *NoControlError
	if errors.As(err, &nc) {
		nc.Msg = label
	}
	return r, err
}

// MsgName is a message as Alder writes it: its module and name, then its parameters (a record
// value, printed by rt.Show) when it has any: `Todos.Index.Toggle { id: "a" }`, `App.Logout`.
func MsgName(name string, params any) string {
	if params == nil {
		return name
	}
	return name + " " + rt.Show(params)
}

// RawFields is Test.submitRaw's fields as a request body: each { name, value } in order, none
// Absent (every name the test wrote must be one the form has an input for). A name form does
// not declare (declared: its fields) is an error, "form NewTodo has no field `colour`".
func RawFields(form string, declared []string, fields []struct {
	Name  string
	Value string
}) (Fields, error) {
	out := url.Values{}
	for _, f := range fields {
		known := false
		for _, d := range declared {
			known = known || d == f.Name
		}
		if !known {
			return Fields{}, fmt.Errorf("form %s has no field `%s`", form, f.Name)
		}
		out[f.Name] = append(out[f.Name], f.Value)
	}
	return Fields{Values: out}, nil
}

// FormatFloat is a Float field as an input submits it: digits that parse back to the same
// value.
func FormatFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// Member is Test.member: a step ("Test.member") inserting, with insert (generated: the row of
// the `via` field's membership table), the membership of user in key.
func Member(insert func(e *eff.Env, user, key string) error, user, key string) eff.Task[struct{}] {
	return eff.Call("Test.member", true, func(e *eff.Env) (struct{}, error) {
		return struct{}{}, insert(e, user, key)
	})
}

// CheckSession is Test.login's check of the session it signs in with, next (an app whose
// session has a `via` field: the membership, as Session.put checks it): Env.CheckSession of the
// test's session and next, nil when the app has none.
func CheckSession(e *eff.Env, next any) (any, error) {
	if e.CheckSession == nil {
		return next, nil
	}
	return e.CheckSession(e, e.Session, next)
}
