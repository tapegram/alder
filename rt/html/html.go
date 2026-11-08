// Package html builds HTML trees for generated Alder views. Text and
// attribute values are kept raw in the tree and escaped exactly once, by
// Render or RenderWith. Form controls (Input) are deferred: they are built at
// render time from the Control and the request's FormState, if any.
package html

import (
	"html"
	"maps"
	"net/url"
	"slices"
	"strings"

	"alder/rt"
)

type kind uint8

const (
	kNone kind = iota
	kElem
	kText
	kRaw
	kRawText
	kFrag
	kCtrl
)

// Node is an HTML node. Its zero value renders nothing.
type Node struct {
	kind     kind
	tag      string   // kElem
	text     string   // kText, kRaw, kRawText
	attrs    []Attr   // kElem
	children []Node   // kElem, kFrag
	control  *Control // kCtrl
}

// Attr is one attribute. Bool attributes render bare (`disabled`); Omit
// attributes do not render at all. An attribute made by RouteAttr has its value
// computed at render time from its route.
type Attr struct {
	Name, Value string
	Bool        bool
	Omit        bool
	route       *rt.Route
}

// RouteAttr is an attribute requesting route r (#160). Its value is, by name: "data-on:click"
// → RouteClick(r), "data-on:submit" → RouteSubmit(r), anything else (such as "href" or
// "action") → rt.RouteURL(r). A route's URL carries no route parameters: its request names the
// page in a header (PageHeader). A <button> with a "data-on:click" RouteAttr renders as a click
// button (RenderWith).
func RouteAttr(name string, r rt.Route) Attr { return Attr{Name: name, route: &r} }

// value is a's value as rendered.
func (a Attr) value() string {
	if r := a.route; r != nil {
		switch a.Name {
		case "data-on:click":
			return RouteClick(*r)
		case "data-on:submit":
			return RouteSubmit(*r)
		}
		return rt.RouteURL(*r)
	}
	return a.Value
}

// El is an element.
func El(tag string, attrs []Attr, children ...Node) Node {
	return Node{kind: kElem, tag: tag, attrs: attrs, children: children}
}

// Text is text content, escaped at render time.
func Text(s string) Node { return Node{kind: kText, text: s} }

// Raw is unescaped markup. Only Html.unsafe produces it.
func Raw(s string) Node { return Node{kind: kRaw, text: s} }

// RawText is the body of a <script> or <style> as the source wrote it. The generator writes it
// only with a Go string literal, from the parser's raw-text body, so it never holds an Alder
// value. It renders unescaped only as a direct child of <script> or <style>, and only when it
// holds neither `</script` nor `</style` in any case (the parser refuses those); anywhere else it
// is ordinary, escaped text.
func RawText(s string) Node { return Node{kind: kRawText, text: s} }

// rawTextElems are HTML's raw-text elements. Their only content is RawText: no Alder value
// (Text, Html.unsafe's Raw, an element or a fragment) is ever spliced into script or style text.
var rawTextElems = map[string]bool{"script": true, "style": true}

// rawTextOK: s may be written unescaped inside a raw-text element without ending it early.
func rawTextOK(s string) bool {
	low := strings.ToLower(s)
	return !strings.Contains(low, "</script") && !strings.Contains(low, "</style")
}

// Nodes is a fragment: its children render in place, with no wrapper.
func Nodes(ns ...Node) Node { return Node{kind: kFrag, children: ns} }

// None renders nothing.
func None() Node { return Node{} }

// RootID is the id an element delivery patches n by (rt/web: a GET route's view answered to an
// element fetch, #160): the id of n's first root element, the top-level elements once fragments
// are flattened and whitespace-only text is ignored. Datastar v1.0.0 morphs every top-level
// element of one patch onto its selector (cmd/alder: TestBrowser_DatastarMultiRoot), so several
// roots are allowed and the first one's id is the target ("load more": the next rows and a new
// sentinel replace the old sentinel). ok is false when there is no root element, when the first
// one has no id (or an empty one), or when a root is other text, raw markup or a form control.
func RootID(n Node) (id string, ok bool) {
	first, seen := true, false
	var walk func(n Node) bool
	walk = func(n Node) bool {
		switch n.kind {
		case kNone:
			return true
		case kFrag:
			for _, c := range n.children {
				if !walk(c) {
					return false
				}
			}
			return true
		case kText, kRawText:
			return strings.TrimLeft(n.text, " \t\n\r\f") == ""
		case kElem:
			if first {
				first = false
				id, seen = idOf(n.attrs)
				if wrapped(n) {
					return false
				}
			}
			return true
		default: // raw markup or a control: its root and id are not known before rendering
			return false
		}
	}
	if !walk(n) || !seen || id == "" {
		return "", false
	}
	return id, true
}

// wrapped reports whether element n renders inside a <form> of the renderer's own when it is not
// in a form: a click button (a <button> with a data-on:click RouteAttr, or the
// <al-button> around one), whose root is then that form, without n's id.
func wrapped(n Node) bool {
	if n.tag == "button" {
		return clickOf(n.attrs) != nil
	}
	return n.tag == "al-button" && len(n.children) == 1 && n.children[0].kind == kElem && n.children[0].tag == "button" && clickOf(n.children[0].attrs) != nil
}

// A is a text attribute.
func A(name, value string) Attr { return Attr{Name: name, Value: value} }

// B is a boolean attribute: present when on, omitted otherwise.
func B(name string, on bool) Attr { return Attr{Name: name, Bool: true, Omit: !on} }

// M is an optional attribute: Nothing omits it.
func M(name string, v rt.Maybe[string]) Attr {
	return Attr{Name: name, Value: v.Val, Omit: !v.Ok}
}

var void = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

// Render serialises a node, escaping text and attribute values. Tag and
// attribute names are trusted literals supplied by the emitter, never escaped.
// It is RenderWith(n, nil): form controls render their initial values and empty
// error lists.
func Render(n Node) string { return RenderWith(n, nil) }

// String is Render(n): rt.Show prints Html values as their markup.
func (n Node) String() string { return Render(n) }

// AlderEqual is Html's equality (rt.Equaler): two nodes are equal when they render to the same
// markup (Render), so `<p>{Html.text "a"}{Html.text "b"}</p> == <p>ab</p>` is True, as are two
// views whose printed sides in a failing test would be identical.
func (n Node) AlderEqual(other any) bool { return Render(n) == Render(other.(Node)) }

// AlderKey is Html's Dict key (rt.Keyer): its rendered markup, so two nodes equal under
// AlderEqual are one key.
func (n Node) AlderKey() string { return Render(n) }

// RenderWith serialises a node like Render, filling deferred form controls from
// st (see FormState). st may be nil.
//
// A `click` button — a <button> with a "data-on:click" RouteAttr — renders so that it works
// without JavaScript too. Outside a <form> it is wrapped in a form of its own:
//
//	<form method="post" action="<url>" class="_alder-click" data-on:submit="…@patch('<url>', window._alderO?.('form'))"><input type="hidden" name="_method" value="PATCH" /><button type="submit" …>…</button></form>
//
// for a write route, the _method field only for PUT, PATCH and DELETE (the router honours it on
// a POST); for a GET route `<form method="get" …>` with the route's query in hidden fields (a GET
// form replaces its action's query with its fields) and an element fetch (FetchOptions), so
// without JavaScript the button navigates to the route (style form._alder-click as display:
// inline to keep the button in line). Inside a <form>, where a form cannot nest, a write route's
// button carries the route as formaction and formmethod, and its _method as its own name and
// value (an author's own name and value on it are dropped, R16: keeping them would lose the
// override without JavaScript, which is worse than losing the author's field); a GET route's is
// type="button" with a data-on:click and needs JavaScript (a formmethod="get" would put every
// field of the enclosing form, a Secret one too, in the URL). A DELETE route's own form posts
// with JavaScript too (routeRequest). A click button that is the one child of an `<al-button>`
// (as the compiler renders `<al-button click={…}>`) is written the same way with the form
// outside the `<al-button>`:
//
//	<form … class="_alder-click" …><al-button variant="…"><button type="submit" …>…</button></al-button></form>
//
// Any other element with a click renders its RouteClick as data-on:click and needs JavaScript.
func RenderWith(n Node, st *FormState) string {
	var b strings.Builder
	r := renderer{b: &b, st: st}
	r.render(n)
	return b.String()
}

// Rendered is a page rendered with its regions recorded (RenderTracked).
type Rendered struct {
	Page    string
	Regions []Region // every element with an id, in document order (by Start)
	// RawIDs: Html.unsafe markup in the page holds an `id=`, whose element the renderer does not
	// see: no id of the page can be proved unique, so it has no regions (Fragments).
	RawIDs bool
	// Toasts: the page has an <al-toasts> element of its own (outside <head>, <template> and
	// <noscript>).
	Toasts bool
}

// Region is an element with an id attribute: Page[Start:End] is its markup. Inert: it is never a
// region (but counted for uniqueness): it is inside <head>, <template> or <noscript>, or the
// browser's parser would not build it as written (repairOf: it holds, or is, markup the parser
// restructures, such as a <div> in a <p>).
type Region struct {
	ID         string
	Start, End int
	Inert      bool
}

// RenderTracked renders n as RenderWith(n, st) does and records each element with an id as it
// writes it: no parse, no second walk.
func RenderTracked(n Node, st *FormState) Rendered { return RenderTrackedSized(n, st, 0) }

// RenderTrackedSized is RenderTracked with room for size bytes made up front: a page rendered
// for comparison with another of about that size (the before-view of a write's answer) grows
// its buffer once instead of step by step.
func RenderTrackedSized(n Node, st *FormState, size int) Rendered {
	var b strings.Builder
	b.Grow(size)
	tr := &tracking{}
	r := renderer{b: &b, st: st, track: tr}
	r.render(n)
	return Rendered{Page: b.String(), Regions: tr.regions, RawIDs: tr.rawIDs, Toasts: tr.toasts}
}

// tracking is what RenderTracked records.
type tracking struct {
	regions []Region
	inert   int // depth inside <head>, <template> or <noscript>
	rawIDs  bool
	toasts  bool
	open    []string // the elements open where the renderer is, outermost first
	openReg []int    // each one's index in regions, -1 for none
}

// repaired makes every open region inert: the parser restructures markup inside them.
func (tr *tracking) repaired() {
	for _, i := range tr.openReg {
		if i >= 0 {
			tr.regions[i].Inert = true
		}
	}
}

// renderer is one rendering: the request's form state and where in the tree it is.
type renderer struct {
	b      *strings.Builder
	track  *tracking // RenderTracked's record, nil otherwise
	st     *FormState
	inForm bool     // inside a <form> element
	form   string   // the request key (rt.RouteKey) of the enclosing <form>, "" for none
	orphan []string // the posting form's field errors no <Errors field> of it shows ("title must …")
}

func (r *renderer) render(n Node) {
	b := r.b
	switch n.kind {
	case kCtrl:
		r.render(n.control.build(r.st, r.active(n.control), r.orphan))
	case kText:
		if tr := r.track; tr != nil && len(tr.open) > 0 && textFosters(tr.open[len(tr.open)-1], n.text) {
			tr.repaired()
		}
		b.WriteString(escape(n.text))
	case kRaw:
		if tr := r.track; tr != nil {
			if hasIDAttr(n.text) {
				tr.rawIDs = true
			}
			if strings.IndexByte(n.text, '<') >= 0 { // markup the renderer does not see: any shape
				tr.repaired()
			}
		}
		b.WriteString(n.text)
	case kRawText:
		b.WriteString(escape(n.text)) // outside <script> and <style>: text
	case kFrag:
		for _, c := range n.children {
			r.render(c)
		}
	case kElem:
		if n.tag == "button" {
			if c := clickOf(n.attrs); c != nil {
				r.clickButton(n, *c.route, nil)
				return
			}
		}
		if n.tag == "al-button" && len(n.children) == 1 && n.children[0].kind == kElem && n.children[0].tag == "button" {
			if c := clickOf(n.children[0].attrs); c != nil { // the design system's button: the form goes around it
				r.clickButton(n.children[0], *c.route, &n)
				return
			}
		}
		saved := *r
		if n.tag == "form" {
			r.inForm, r.form, r.orphan = true, formKey(n.attrs), nil
			if st := r.st; st != nil && st.Form != "" && st.Msg != "" && st.Msg == r.form {
				r.orphan = orphanErrors(n, st)
			}
		}
		region := -1
		tr := r.track
		if tr != nil {
			fixed := false
			if act, _ := repairOf(tr.open, n.tag); act != keep {
				tr.repaired()
				fixed = true
			}
			if id, ok := idOf(n.attrs); ok {
				region = len(tr.regions)
				tr.regions = append(tr.regions, Region{ID: id, Start: b.Len(), Inert: tr.inert > 0 || fixed})
			}
			if n.tag == "al-toasts" && tr.inert == 0 {
				tr.toasts = true
			}
			if n.tag == "head" || n.tag == "template" || n.tag == "noscript" {
				tr.inert++
				defer func() { tr.inert-- }()
			}
		}
		r.open(n.tag, n.attrs)
		if void[n.tag] {
			if region >= 0 {
				r.track.regions[region].End = b.Len()
			}
			*r = saved
			return
		}
		if tr != nil {
			tr.open, tr.openReg = append(tr.open, n.tag), append(tr.openReg, region)
		}
		for _, c := range n.children {
			if rawTextElems[n.tag] {
				if c.kind == kRawText && rawTextOK(c.text) {
					b.WriteString(c.text)
				}
				continue
			}
			r.render(c)
		}
		b.WriteString("</")
		b.WriteString(n.tag)
		b.WriteByte('>')
		if tr != nil {
			tr.open, tr.openReg = tr.open[:len(tr.open)-1], tr.openReg[:len(tr.openReg)-1]
		}
		if region >= 0 {
			r.track.regions[region].End = b.Len()
		}
		*r = saved
	}
}

// idOf is the value of attrs' id attribute as rendered, if it renders one.
func idOf(attrs []Attr) (string, bool) {
	for _, a := range attrs {
		if a.Name == "id" && !a.Omit && !a.Bool {
			return a.value(), true
		}
	}
	return "", false
}

// hasIDAttr reports whether raw markup may hold an id attribute: `id` before an `=`, any case,
// with any whitespace between (`id = "x"`).
func hasIDAttr(s string) bool {
	for i := strings.IndexByte(s, '='); i >= 0; {
		k := i
		for k > 0 && isSpace(s[k-1]) {
			k--
		}
		if k >= 2 && (s[k-2]|0x20) == 'i' && (s[k-1]|0x20) == 'd' {
			return true
		}
		j := strings.IndexByte(s[i+1:], '=')
		if j < 0 {
			return false
		}
		i += 1 + j
	}
	return false
}

// open writes the start tag of tag with attrs (" />" for a void element).
func (r *renderer) open(tag string, attrs []Attr) {
	b := r.b
	b.WriteByte('<')
	b.WriteString(tag)
	for _, a := range attrs {
		if a.Omit {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(a.Name)
		if !a.Bool {
			v := a.value()
			if a.Name == "href" && !rt.SafeURL(v) {
				v = "#" // no javascript:, data: or protocol-relative link (rt.SafeURL)
			}
			b.WriteString(`="`)
			b.WriteString(escape(v))
			b.WriteByte('"')
		}
	}
	if void[tag] {
		b.WriteString(" />")
		return
	}
	b.WriteByte('>')
}

// clickButton writes button n, whose click requests route (see RenderWith). outer, when not
// nil, is the `<al-button>` whose one child n is: it is written around the button, inside the
// form, so the stylesheet's `al-button > button` still finds the button as its child.
func (r *renderer) clickButton(n Node, route rt.Route, outer *Node) {
	url := rt.RouteURL(route)
	get := route.Method == "GET"
	formMethod := "post"
	if get {
		formMethod = "get"
	}
	attrs := []Attr{A("type", "submit")}
	override := false
	switch {
	case r.inForm && get:
		attrs = []Attr{A("type", "button")}
	case r.inForm:
		attrs = append(attrs, A("formaction", url), A("formmethod", formMethod), B("formnovalidate", true))
		if m := overrideMethod(route.Method); m != "" {
			attrs = append(attrs, A("name", methodField), A("value", m))
			override = true
		}
		attrs = append(attrs, A("data-on:click__prevent", RouteClick(route)))
	}
	for _, a := range n.attrs {
		if a.Name != "type" && a.Name != "data-on:click" && !(override && (a.Name == "name" || a.Name == "value")) {
			attrs = append(attrs, a)
		}
	}
	if r.inForm && get {
		attrs = append(attrs, A("data-on:click", RouteClick(route)))
	}
	button := Node{kind: kElem, tag: "button", attrs: attrs, children: n.children}
	if outer != nil {
		o := *outer
		o.children = []Node{button}
		button = o
	}
	if r.inForm {
		saved := *r
		r.render(button)
		*r = saved
		return
	}
	var fields []Node
	if get {
		for _, kv := range route.Query {
			if !kv.Omit {
				fields = append(fields, hidden(kv.Key, kv.Value))
			}
		}
	} else if m := overrideMethod(route.Method); m != "" {
		fields = append(fields, hidden(methodField, m))
	}
	var submit string
	if get {
		submit = RouteClick(route)
	} else {
		submit = busy("el", routeRequest(route, PostOptions+"?.('form')", true))
	}
	r.render(El("form", []Attr{A("method", formMethod), RouteAttr("action", route), A("class", "_alder-click"), A("data-on:submit", submit)},
		append(fields, button)...))
}

// methodField is the url-encoded field a POST names its route's method by (rt/web honours it).
const methodField = "_method"

// overrideMethod is the _method value a no-JavaScript POST carries for a route of method m: m
// for PUT, PATCH and DELETE, "" for none.
func overrideMethod(m string) string {
	switch m {
	case "PUT", "PATCH", "DELETE":
		return m
	}
	return ""
}

// hidden is `<input type="hidden" name="…" value="…" />`.
func hidden(name, value string) Node {
	return El("input", []Attr{A("type", "hidden"), A("name", name), A("value", value)})
}

// FormToRoute is a `<form submit={R …}>` sending route r (#160): posted to r's URL, so it works
// without JavaScript; with it, Datastar sends the form's fields url-encoded with r's own method
// (RouteSubmit). attrs are the element's other attributes. FormState.Msg is rt.RouteKey(r) for
// its state to show.
//
// For PUT, PATCH and DELETE the form's own submit buttons (ownSubmits) carry the method as
// name="_method", so a click button inside the form sends only its own override, or none. A form
// without such a button (submitted by Enter in a field), or with one that has a name of its own
// (HTML keeps the first name attribute, so the button cannot carry both), falls back to a hidden
// _method field; a click button inside that form sends it too without JavaScript (a second
// _method for a PUT/PATCH/DELETE route, 400; a POST route then routed as the form's method). With JavaScript a DELETE form posts too, the _method in its body (routeRequest).
func FormToRoute(r rt.Route, attrs []Attr, children ...Node) Node {
	all := append([]Attr{A("method", "post"), RouteAttr("action", r), RouteAttr("data-on:submit", r)}, attrs...)
	if m := overrideMethod(r.Method); m != "" {
		var found bool
		if children, found = ownSubmits(children, m); !found {
			children = append([]Node{hidden(methodField, m)}, children...)
		}
	}
	return El("form", all, children...)
}

// ownSubmits is ns with name="_method" value=m on each of the form's own submit buttons — a
// <button> with no type or type="submit" and no click (clickOf), at any depth of elements and
// fragments — and whether there was one. When one of them has a name of its own (two submit
// buttons, action=save and action=close), none is changed and the result is false: a second
// name attribute would be ignored, and the hidden field alone must carry the method (a button's
// _method beside it would be a second one). The nodes are copied, never changed in place.
func ownSubmits(ns []Node, m string) ([]Node, bool) {
	if namedSubmit(ns) {
		return ns, false
	}
	found := false
	out := make([]Node, len(ns))
	for i, n := range ns {
		switch {
		case n.kind == kElem && n.tag == "button":
			if isOwnSubmit(n) {
				n.attrs = append(slices.Clone(n.attrs), A("name", methodField), A("value", m))
				found = true
			}
		case n.kind == kElem || n.kind == kFrag:
			var f bool
			if n.children, f = ownSubmits(n.children, m); f {
				found = true
			}
		}
		out[i] = n
	}
	return out, found
}

// isOwnSubmit reports whether button n submits its form itself: no type or type="submit", and no
// click (clickOf).
func isOwnSubmit(n Node) bool {
	typ, ok := attrValue(n.attrs, "type")
	return (!ok || typ == "submit") && clickOf(n.attrs) == nil
}

// namedSubmit reports whether one of ns's own submit buttons (isOwnSubmit, at any depth of
// elements and fragments) has a name attribute of its own.
func namedSubmit(ns []Node) bool {
	for _, n := range ns {
		switch {
		case n.kind == kElem && n.tag == "button":
			if _, named := attrValue(n.attrs, "name"); named && isOwnSubmit(n) {
				return true
			}
		case n.kind == kElem || n.kind == kFrag:
			if namedSubmit(n.children) {
				return true
			}
		}
	}
	return false
}

// attrValue is the value of attrs' plain attribute name, if it renders one.
func attrValue(attrs []Attr, name string) (string, bool) {
	for _, a := range attrs {
		if a.Name == name && !a.Omit && !a.Bool && a.route == nil {
			return a.Value, true
		}
	}
	return "", false
}

// RouteSubmit is the data-on:submit of a form sending route r: its fields url-encoded with r's
// method (a DELETE's posted, routeRequest) and the write options (PostOptions, contentType
// 'form'; before the helper exists, the content type alone).
func RouteSubmit(r rt.Route) string {
	return busy("el", routeRequest(r, PostOptions+"?.('form')||{contentType: 'form'}", true))
}

// RouteClick is the Datastar action of a click requesting route r: a write route's request
// with the write options (PostOptions), a GET route's an element fetch (FetchOptions).
func RouteClick(r rt.Route) string {
	if r.Method == "GET" {
		return busy("el", routeRequest(r, FetchOptions+"?.()", false))
	}
	return busy("el", routeRequest(r, PostOptions+"?.()", false))
}

// routeRequest is Datastar's action for r's method ("@patch('<url>', opts)"). The URL is
// rt.RouteURL(r) with each "'" as %27, which would otherwise end the string: rt.PathSegment
// escapes it already, so this is defence in depth for a literal Route.Path.
//
// form says opts send a form (contentType 'form'). Datastar v1.0.0 treats a DELETE as a GET
// (`!["GET","DELETE"].includes(method)` decides whether a request has a body): a form's fields,
// _method=DELETE among them, would go in the URL — into logs, and never decoded, since a write
// decodes its body — and no body would be sent. So a DELETE that sends a form is "@post", its
// _method=DELETE in the url-encoded body (the form's hidden field, or its submit button's name and
// value, which Datastar appends to `new FormData(form)` for the submit event's submitter;
// TestBrowser_RouteDeleteBody pins both). The router never honours a _method in the query (R7).
// A DELETE without a form (a click, JSON content) stays "@delete": its only query is
// `datastar={}`, which rt/web drops. PUT and PATCH keep their own verbs; future helpers that
// send a form (input=, load=, …) must not use "@delete" either.
func routeRequest(r rt.Route, opts string, form bool) string {
	u := strings.ReplaceAll(rt.RouteURL(r), "'", "%27")
	verb := strings.ToLower(r.Method)
	if form && r.Method == "DELETE" {
		verb = "post"
	}
	return "@" + verb + "('" + u + "', " + opts + ")"
}

// orphanErrors are the field errors of st that no <Errors field="…"> of its form inside form
// shows (nor a wrapped control, such as <al-input field="…">, which lists its own), each
// prefixed with its field's name, fields in name order: `<Errors form />` lists them after the
// form's own error, so no decoding error is dropped silently.
func orphanErrors(form Node, st *FormState) []string {
	shown := map[string]bool{}
	var walk func(Node)
	walk = func(n Node) {
		switch n.kind {
		case kCtrl:
			if c := n.control; (c.Kind == "errors" || c.Wrap != "") && c.Name != "" && c.Form == st.Form {
				shown[c.Name] = true // a wrapped control shows its field's errors itself
			}
		case kElem, kFrag:
			for _, c := range n.children {
				walk(c)
			}
		}
	}
	walk(form)
	var out []string
	for _, name := range slices.Sorted(maps.Keys(st.Errors)) {
		if !shown[name] {
			for _, m := range st.Errors[name] {
				out = append(out, name+" "+m)
			}
		}
	}
	return out
}

// clickOf is attrs' "data-on:click" RouteAttr, or nil.
func clickOf(attrs []Attr) *Attr {
	for i, a := range attrs {
		if a.Name == "data-on:click" && a.route != nil && !a.Omit {
			return &attrs[i]
		}
	}
	return nil
}

// formKey is the request key of a <form> with attrs: its "action" RouteAttr's (FormToRoute:
// rt.RouteKey), else its literal action URL's (rt.URLKey), "" for none.
func formKey(attrs []Attr) string {
	for _, a := range attrs {
		if a.Name != "action" || a.Omit {
			continue
		}
		if a.route != nil {
			return rt.RouteKey(*a.route)
		}
		if u, err := url.Parse(a.Value); err == nil {
			return rt.URLKey(u.Path, u.Query())
		}
	}
	return ""
}

// active reports whether control c shows the request's form state: the state is a form's
// (Form set) and c is a control of that Alder form inside the very <form> that sent the
// request (FormState.Msg), so another form of the same type — a per-row form of another row —
// renders its initial values.
func (r *renderer) active(c *Control) bool {
	st := r.st
	return st != nil && st.Form != "" && st.Form == c.Form && st.Msg != "" && st.Msg == r.form
}

// escape escapes text or an attribute value: html.EscapeString's five characters, and "\r" as
// "&#13;", so no rendered page carries a raw carriage return (a Datastar patch splits its lines
// on it; rt/datastar normalises line ends too).
func escape(s string) string {
	e := html.EscapeString(s)
	if strings.IndexByte(e, '\r') >= 0 {
		e = strings.ReplaceAll(e, "\r", "&#13;")
	}
	return e
}

// PostOptions is the helper a write route's request calls for its request options,
// window._alderO(contentType?) (datastar.BoostInit defines it, once, in the <head>, so each
// button of a long list carries a call of about 20 bytes): it aborts the navigation in flight
// and starts the page's next one (Navigation), and sends ShownHeader, the hash the page's
// <al-shown id="_alder_shown"> holds. It is called as `?.()`: a post made before the boost's
// data-init ran sends no header, and its answer morphs the whole body.
const PostOptions = "window._alderO"

// FetchOptions is the helper an element fetch (a click requesting a GET route, #160) calls for
// its request options, window._alderF() (datastar.BoostInit): FetchHeader and the build, without
// the navigation's abort controller. It is called as `?.()`, like PostOptions.
const FetchOptions = "window._alderF"

// Navigation is the request option every navigating Datastar request of a page shares — a
// write route's request (RouteSubmit, RouteClick, through PostOptions) and a boosted link's or a back/forward's GET
// (datastar.BoostClick, BoostPopstate): one AbortController, window._alderAC, each request
// aborting the one before it (window._alderAC?.abort()) and starting a new one. Only the latest
// navigation's answer is applied, so a slow post's answer cannot land after (and over) the
// page a later click asked for. The dev server's reload stream does not use it.
const Navigation = "requestCancellation: window._alderAC = new AbortController()"

// BusyNull opens the wrapper of a request that only marks the page pending (a boosted GET):
// BusyNull + request + ")". The wrapper is window._alderBusy (datastar.BoostInit) marking the
// element's form inert and aria-busy while the request is in flight and the page pending (a 2px
// bar); without the helper the request runs unwrapped.
const BusyNull = "(window._alderBusy||((e,p)=>p))(null, "

// busy wraps request, a route's request, in the wrapper of el (the element the handler runs on).
func busy(el, req string) string {
	return "(window._alderBusy||((e,p)=>p))(" + el + ", " + req + ")"
}
