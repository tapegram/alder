package rt

import "strings"

// ResponseKind says what a Response answers with (#160).
type ResponseKind uint8

const (
	RespRedirect ResponseKind = iota // 303 to Target's URL
	RespRender                       // 200, Target's view
	RespRefresh                      // 200, the page the browser is on, again
	RespInvalid                      // 422, the page again, form kept, Text as its error
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

// ResponseEvent is one Datastar event a response sends besides its body. Only a toast so far;
// element patches, removals and a location come with their builders.
type ResponseEvent struct {
	Toast *Toast
}

// Response is the Prelude `Response`, the answer of a write route (#160). Only the fields its
// Kind names are meaningful.
type Response struct {
	Kind    ResponseKind
	Status  int         // 0: the kind's code (Code)
	Headers [][2]string // reserved for Response.header; nothing sets it yet
	Target  Route       // RespRedirect, RespRender
	Text    string      // RespInvalid
	Events  []ResponseEvent
}

// Redirect is `Response.redirect r`.
func Redirect(r Route) Response { return Response{Kind: RespRedirect, Target: r} }

// Render is `Response.render r`.
func Render(r Route) Response { return Response{Kind: RespRender, Target: r} }

// Refresh is `Response.refresh`.
func Refresh() Response { return Response{Kind: RespRefresh} }

// Invalid is `Response.invalid text`.
func Invalid(text string) Response { return Response{Kind: RespInvalid, Text: text} }

// WithToast is `Response.toast text r` (and success, warning, error by k): r with a toast
// appended to its events. r's own events are not changed.
func WithToast(k NoticeKind, text string, r Response) Response {
	ev := make([]ResponseEvent, len(r.Events), len(r.Events)+1)
	copy(ev, r.Events)
	r.Events = append(ev, ResponseEvent{Toast: &Toast{Kind: k, Text: text}})
	return r
}

// Code is r's HTTP status: Status when set, else its kind's (303, 200, 200, 422).
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
// equality), text and events.
func (r Response) AlderEqual(other any) bool {
	o := other.(Response)
	if r.Kind != o.Kind || r.Code() != o.Code() || r.Text != o.Text ||
		!r.Target.AlderEqual(o.Target) || len(r.Headers) != len(o.Headers) || len(r.Events) != len(o.Events) {
		return false
	}
	for i := range r.Headers {
		if r.Headers[i] != o.Headers[i] {
			return false
		}
	}
	for i, e := range r.Events {
		f := o.Events[i]
		if (e.Toast == nil) != (f.Toast == nil) || e.Toast != nil && *e.Toast != *f.Toast {
			return false
		}
	}
	return true
}

// AlderShow prints r as the builders that make it (Shower):
// `Response.redirect (Todos.Index /todos) |> Response.toast "Added"`.
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
	}
	for _, t := range r.Toasts() {
		fn := string(t.Kind)
		if t.Kind == NoticeInfo || t.Kind == "" {
			fn = "toast"
		}
		b.WriteString(" |> Response." + fn + " " + quote(t.Text))
		app = true
	}
	return b.String(), app
}
