package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// formWireNames warns (W0008, #63) about a GET `<form>` (method "get" or none, and no `submit`
// message) whose literal `action` is the path of a page, when a plain `<input>`, `<select>` or
// `<textarea>` inside it has a literal `name` that is one of the page's query parameters
// declared with another wire name (`electionId: Text = "" as "ElectionId"`): the browser sends
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
	page, pd := mc.c.pageAt(action)
	if pd == nil || pd.Params == nil {
		return
	}
	wires := map[string]string{} // a parameter's name → its wire name, where they differ
	for _, f := range pd.Params.Fields {
		if f.Wire != "" && f.Wire != f.Name {
			wires[f.Name] = f.Wire
		}
	}
	if len(wires) == 0 {
		return
	}
	for _, ch := range x.Children {
		mc.formInputWires(ch, page, wires)
	}
}

// formInputWires walks e, markup inside a GET form to page, for the inputs formWireNames reports.
func (mc *modCtx) formInputWires(e syntax.Expr, page string, wires map[string]string) {
	switch x := e.(type) {
	case *syntax.Element:
		if x.Ref.Kind == syntax.RefComponent || x.Tag == "form" {
			return
		}
		if x.Tag == "input" || x.Tag == "select" || x.Tag == "textarea" {
			for _, a := range x.Attrs {
				if a.Name != "name" || a.Value == nil {
					continue
				}
				name, ok := literalText(a.Value)
				if !ok {
					continue
				}
				if w, ok := wires[name]; ok {
					d := mc.c.bag.Warn(diag.FormWireName, a.Span, "page `%s` reads its parameter `%s` from the query key `%s`, so `name=\"%s\"` in this GET form sets nothing; use the wire name: `name=\"%s\"`", page, name, w, name, w)
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

// pageAt is the page module, and its declaration, whose path a form's literal `action` names
// (`/r/17/wood/Results`; a query or fragment and a trailing slash are ignored, and a `{param}`
// segment takes any one segment), or "" and nil for none. As the router does, a literal segment
// wins over a parameter, left to right (`/todos/new` before `/todos/{id}`).
func (c *Checker) pageAt(action string) (string, *syntax.PageDecl) {
	if i := strings.IndexAny(action, "?#"); i >= 0 {
		action = action[:i]
	}
	if !strings.HasPrefix(action, "/") {
		return "", nil
	}
	want := strings.Split(strings.TrimSuffix(action, "/"), "/")
	best, bestRank := "", ""
	for _, name := range c.prog.Order {
		pd := c.prog.Pages[name]
		if pd == nil || !c.prog.IsPageModule(name) {
			continue
		}
		segs := strings.Split(strings.TrimSuffix(pd.Path, "/"), "/")
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
			best, bestRank = name, string(rank)
		}
	}
	if best == "" {
		return "", nil
	}
	return best, c.prog.Pages[best]
}
