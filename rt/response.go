package rt

import (
	"fmt"
	"slices"
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

// EventKind is what a ResponseEvent is (#329). Every consumer switches on it, so a kind added
// later is never read as another; the zero kind is no builder's.
type EventKind uint8

const (
	EventUnknown EventKind = iota // made by no builder
	EventToast                    // a toast (Toast)
	EventRemove                   // the removal of the element with the id ID (#309)
)

// ResponseEvent is one Datastar event a response sends besides its body, of kind Kind. Only the
// fields its Kind names are meaningful.
type ResponseEvent struct {
	Kind  EventKind
	Toast Toast  // EventToast
	ID    string // EventRemove: the id of the element removed
}

// Response is the Prelude `Response`, the answer of a write route (#160). Only the fields its
// Kind names are meaningful.
type Response struct {
	Kind    ResponseKind
	Status  int         // 0: the kind's code (Code)
	Headers [][2]string // reserved for Response.header; nothing sets it yet
	Target  Route       // RespRedirect, RespRender
	Text    string      // RespInvalid
	Roots   []any       // RespReplace: each an html.Node (rt cannot import rt/html), shown by ShowRoots
	Events  []ResponseEvent
}

// ShowRoots is the markup of each root of a replace as it is sent: the roots flattened (rt/web's
// ReplaceRoots: fragments opened, whitespace-only text dropped), each rendered, and a root that
// is not an html.Node in fmt's form. Set by rt/html's init, since rt cannot import rt/html.
// Response's equality and printing use it, so two replaces that send the same roots are equal
// and print alike (#329).
var ShowRoots func([]any) []string

// showRoots is roots' markup by ShowRoots, or each in fmt's form when rt/html is not linked.
func showRoots(roots []any) []string {
	if ShowRoots != nil {
		return ShowRoots(roots)
	}
	out := make([]string, len(roots))
	for i, root := range roots {
		out[i] = fmt.Sprint(root)
	}
	return out
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
	return withEvent(r, ResponseEvent{Kind: EventToast, Toast: Toast{Kind: k, Text: text}})
}

// WithRemove is `Response.remove id r`: r with the removal of the element with id id appended
// to its events. r's own events are not changed. On a redirect, refresh or invalid, which bring
// a whole page, the removal is carried but not sent (the web runtime logs it).
func WithRemove(id string, r Response) Response {
	return withEvent(r, ResponseEvent{Kind: EventRemove, ID: id})
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
		if e.Kind == EventToast {
			out = append(out, e.Toast)
		}
	}
	return out
}

// AlderEqual is Response's equality (Equaler): the same kind, code, headers, target (as Route's
// equality), text, roots (as sent, by their markup, in order: showRoots) and events (toasts and
// removals, in order).
func (r Response) AlderEqual(other any) bool {
	o := other.(Response)
	if r.Kind != o.Kind || r.Code() != o.Code() || r.Text != o.Text ||
		!r.Target.AlderEqual(o.Target) || len(r.Headers) != len(o.Headers) || len(r.Events) != len(o.Events) ||
		!slices.Equal(showRoots(r.Roots), showRoots(o.Roots)) {
		return false
	}
	for i := range r.Headers {
		if r.Headers[i] != o.Headers[i] {
			return false
		}
	}
	for i, e := range r.Events {
		if e != o.Events[i] {
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
		b.WriteString("Response.replace [" + strings.Join(showRoots(r.Roots), ", ") + "]")
	}
	for _, e := range r.Events {
		switch e.Kind {
		case EventRemove:
			b.WriteString(" |> Response.remove " + quote(e.ID))
		case EventToast:
			fn := string(e.Toast.Kind)
			if e.Toast.Kind == NoticeInfo || e.Toast.Kind == "" {
				fn = "toast"
			}
			b.WriteString(" |> Response." + fn + " " + quote(e.Toast.Text))
		default:
			fmt.Fprintf(&b, " |> <unknown event %d>", e.Kind)
		}
		app = true
	}
	return b.String(), app
}
