package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// formWireNames warns (W0008, #63) about a GET `<form>` (method "get" or none, and no `submit`
// message) whose literal `action` is the path of a page or of a GET route, when a plain `<input>`, `<select>` or
// `<textarea>` inside it (without a `form` attribute naming another form) has a literal `name`
// that is one of the page's query parameters declared with another wire name, and no key the
// page reads (`electionId: Text = "" as "ElectionId"`): the browser sends
// the name as the query key, and the page reads the wire name, so the input sets nothing.
func (mc *modCtx) formWireNames(x *syntax.Element) {
	action, method := "", "get"
	hasAction := false
	for _, a := range x.Attrs {
		if a.Value == nil {
			continue
		}
		switch a.Name {
		case "submit":
			return // a message form posts to the page's own message URL
		case "method":
			m, ok := literalText(a.Value)
			if !ok {
				return
			}
			method = strings.ToLower(m)
		case "action":
			act, ok := literalText(a.Value)
			if !ok {
				return
			}
			action, hasAction = act, true
		}
	}
	if !hasAction || method != "get" {
		return
	}
	page, params := mc.c.pageAt(action)
	if params == nil {
		return
	}
	wires := map[string]string{} // a parameter's name → its wire name, where they differ
	reads := map[string]bool{}   // every key the page reads: a wire name, or a name without one
	for _, f := range params.Fields {
		if f.Wire != "" && f.Wire != f.Name {
			wires[f.Name] = f.Wire
		}
		if f.Wire != "" {
			reads[f.Wire] = true
		} else {
			reads[f.Name] = true
		}
	}
	for k := range reads { // `a: T as "b", b: T as "c"`: `name="b"` sets `a` (review I1)
		delete(wires, k)
	}
	if len(wires) == 0 {
		return
	}
	for _, ch := range x.Children {
		mc.formInputWires(ch, page, wires)
	}
}

// formInputWires walks e, markup inside a GET form to page (`page \`Wood\“ or
// `route \`Results.Index\“, as messages name it), for the inputs formWireNames reports.
func (mc *modCtx) formInputWires(e syntax.Expr, page string, wires map[string]string) {
	switch x := e.(type) {
	case *syntax.Element:
		if x.Ref.Kind == syntax.RefComponent || x.Tag == "form" {
			return
		}
		if (x.Tag == "input" || x.Tag == "select" || x.Tag == "textarea") && !hasHTMLAttr(x, "form") {
			// an input with `form="id"` belongs to that form, wherever it sits
			for _, a := range x.Attrs {
				if a.Name != "name" || a.Value == nil {
					continue
				}
				name, ok := literalText(a.Value)
				if !ok {
					continue
				}
				if w, ok := wires[name]; ok {
					d := mc.c.bag.Warn(diag.FormWireName, a.Span, "%s reads its parameter `%s` from the query key `%s`, so `name=\"%s\"` in this GET form sets nothing; use the wire name: `name=\"%s\"`", page, name, w, name, w)
					d.Fix = diag.ReplaceSpan(a.Value.GetSpan(), `"`+w+`"`)
				}
			}
		}
		for _, ch := range x.Children {
			mc.formInputWires(ch, page, wires)
		}
	case *syntax.HtmlFor:
		mc.formInputWires(x.Body, page, wires)
	case *syntax.HtmlIf:
		mc.formInputWires(x.Then, page, wires)
		mc.formInputWires(x.Else, page, wires)
	case *syntax.HtmlMatch:
		for _, a := range x.Arms {
			mc.formInputWires(a.Body, page, wires)
		}
	}
}

// pageAt is the page module or GET route whose path a form's literal `action` names
// (`/r/17/wood/Results`; a query or fragment and a trailing slash are ignored, and a `{param}`
// segment takes any one segment), as messages name it (`page \`Wood\“, `route
// \`Results.Index\“), and its declared parameters; "" and nil for none. As the router does, a
// literal segment wins over a parameter, left to right (`/todos/new` before `/todos/{id}`).
func (c *Checker) pageAt(action string) (string, *syntax.RecordType) {
	if i := strings.IndexAny(action, "?#"); i >= 0 {
		action = action[:i]
	}
	if !strings.HasPrefix(action, "/") {
		return "", nil
	}
	want := strings.Split(strings.TrimSuffix(action, "/"), "/")
	best, bestRank := "", ""
	var bestParams *syntax.RecordType
	type target struct {
		name, path string
		params     *syntax.RecordType
	}
	var targets []target
	for _, name := range c.prog.Order {
		if pd := c.prog.Pages[name]; pd != nil && c.prog.IsPageModule(name) {
			targets = append(targets, target{"page `" + name + "`", pd.Path, pd.Params})
		}
		for _, r := range c.routes[name] {
			if !isWrite(r.Method) {
				targets = append(targets, target{"route `" + name + "." + r.Name + "`", r.Path, r.Decl.Params})
			}
		}
	}
	for _, t := range targets {
		segs := strings.Split(strings.TrimSuffix(t.path, "/"), "/")
		if len(segs) != len(want) {
			continue
		}
		rank := make([]byte, len(segs)) // '1' a literal segment, '0' a parameter
		match := true
		for i, s := range segs {
			if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
				rank[i], match = '0', want[i] != ""
			} else {
				rank[i], match = '1', s == want[i]
			}
			if !match {
				break
			}
		}
		if match && (best == "" || string(rank) > bestRank) {
			best, bestRank, bestParams = t.name, string(rank), t.params
		}
	}
	return best, bestParams
}
