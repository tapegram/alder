package rt

import (
	"fmt"
	"strings"
)

// ResponseKind says what a Response answers with (#160).
type ResponseKind uint8

const (
	RespRedirect ResponseKind = iota // 303 to Target's URL
	RespRender                       // 200, Target's view
	RespRefresh                      // 200, the page the browser is on, again
	RespInvalid                      // 422, the page again, form kept, Text as its error
	RespReplace                      // 200, each of Roots replaces the page's element with its id (#309)
)

// NoticeKind is the variant of a toast: Response.toast is info.
type NoticeKind string

const (
	NoticeInfo    NoticeKind = "info"
	NoticeSuccess NoticeKind = "success"
	NoticeWarning NoticeKind = "warning"
	NoticeError   NoticeKind = "error"
)

// Toast is a toast shown with a response: its variant and its text.
type Toast struct {
	Kind NoticeKind
	Text string
}

// ResponseEvent is one Datastar event a response sends besides its body: a toast, or the
// removal of the element with the id Remove (#309). Exactly one of the two is set.
type ResponseEvent struct {
	Toast  *Toast
	Remove string
}

// Response is the Prelude `Response`, the answer of a write route (#160). Only the fields its
// Kind names are meaningful.
type Response struct {
	Kind    ResponseKind
	Status  int         // 0: the kind's code (Code)
	Headers [][2]string // reserved for Response.header; nothing sets it yet
	Target  Route       // RespRedirect, RespRender
	Text    string      // RespInvalid
	Roots   []any       // RespReplace: each an html.Node (rt cannot import rt/html), shown by ShowRoot
	Events  []ResponseEvent
}

// ShowRoot renders a root of a replace (an html.Node) as its markup: set by rt/html's init, since
// rt cannot import rt/html. Response's equality and printing use it.
var ShowRoot func(any) string

// showRoot is root's markup by ShowRoot, or fmt's form of it when rt/html is not linked.
func showRoot(root any) string {
	if ShowRoot == nil {
		return fmt.Sprint(root)
	}
	return ShowRoot(root)
}

// Redirect is `Response.redirect r`.
func Redirect(r Route) Response { return Response{Kind: RespRedirect, Target: r} }

// Render is `Response.render r`.
func Render(r Route) Response { return Response{Kind: RespRender, Target: r} }

// Refresh is `Response.refresh`.
func Refresh() Response { return Response{Kind: RespRefresh} }

// Replace is `Response.replace roots`: each root (an html.Node, its list already flattened)
// replaces the page's element with the same id. Whether the roots can be sent is the web
// runtime's check.
func Replace(roots []any) Response { return Response{Kind: RespReplace, Roots: roots} }

// Invalid is `Response.invalid text`.
func Invalid(text string) Response { return Response{Kind: RespInvalid, Text: text} }

// WithToast is `Response.toast text r` (and success, warning, error by k): r with a toast
// appended to its events. r's own events are not changed.
func WithToast(k NoticeKind, text string, r Response) Response {
	return withEvent(r, ResponseEvent{Toast: &Toast{Kind: k, Text: text}})
}

// WithRemove is `Response.remove id r`: r with the removal of the element with id id appended
// to its events. r's own events are not changed. On a redirect, refresh or invalid, which bring
// a whole page, the removal is carried but not sent (the web runtime logs it).
func WithRemove(id string, r Response) Response {
	return withEvent(r, ResponseEvent{Remove: id})
}

// withEvent is r with e appended to a copy of its events.
func withEvent(r Response, e ResponseEvent) Response {
	ev := make([]ResponseEvent, len(r.Events), len(r.Events)+1)
	copy(ev, r.Events)
	r.Events = append(ev, e)
	return r
}

// Code is r's HTTP status: Status when set, else its kind's (303, 200, 200, 422, 200).
func (r Response) Code() int {
	if r.Status != 0 {
		return r.Status
	}
	switch r.Kind {
	case RespRedirect:
		return 303
	case RespInvalid:
		return 422
	}
	return 200
}

// Toasts are r's toasts, in the order they were added.
func (r Response) Toasts() []Toast {
	var out []Toast
	for _, e := range r.Events {
		if e.Toast != nil {
			out = append(out, *e.Toast)
		}
	}
	return out
}

// AlderEqual is Response's equality (Equaler): the same kind, code, headers, target (as Route's
// equality), text, roots (by their markup, in order) and events (toasts and removals, in order).
func (r Response) AlderEqual(other any) bool {
	o := other.(Response)
	if r.Kind != o.Kind || r.Code() != o.Code() || r.Text != o.Text ||
		!r.Target.AlderEqual(o.Target) || len(r.Headers) != len(o.Headers) || len(r.Events) != len(o.Events) ||
		len(r.Roots) != len(o.Roots) {
		return false
	}
	for i := range r.Roots {
		if showRoot(r.Roots[i]) != showRoot(o.Roots[i]) {
			return false
		}
	}
	for i := range r.Headers {
		if r.Headers[i] != o.Headers[i] {
			return false
		}
	}
	for i, e := range r.Events {
		f := o.Events[i]
		if e.Remove != f.Remove || (e.Toast == nil) != (f.Toast == nil) || e.Toast != nil && *e.Toast != *f.Toast {
			return false
		}
	}
	return true
}

// AlderShow prints r as the builders that make it (Shower), its modifiers in the order written:
// `Response.redirect (Todos.Index /todos) |> Response.toast "Added"`,
// `Response.replace [<p id="n">1</p>] |> Response.remove "member-3"`.
func (r Response) AlderShow() (string, bool) {
	var b strings.Builder
	app := true
	switch r.Kind {
	case RespRedirect, RespRender:
		name := "Response.redirect "
		if r.Kind == RespRender {
			name = "Response.render "
		}
		s, sapp := showRoute(r.Target)
		if sapp {
			s = "(" + s + ")"
		}
		b.WriteString(name + s)
	case RespRefresh:
		b.WriteString("Response.refresh")
		app = false
	case RespInvalid:
		b.WriteString("Response.invalid " + quote(r.Text))
	case RespReplace:
		roots := make([]string, len(r.Roots))
		for i, root := range r.Roots {
			roots[i] = showRoot(root)
		}
		b.WriteString("Response.replace [" + strings.Join(roots, ", ") + "]")
	}
	for _, e := range r.Events {
		if e.Toast == nil {
			b.WriteString(" |> Response.remove " + quote(e.Remove))
		} else {
			fn := string(e.Toast.Kind)
			if e.Toast.Kind == NoticeInfo || e.Toast.Kind == "" {
				fn = "toast"
			}
			b.WriteString(" |> Response." + fn + " " + quote(e.Toast.Text))
		}
		app = true
	}
	return b.String(), app
}
