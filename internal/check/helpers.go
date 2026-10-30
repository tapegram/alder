// Test helpers (#4): a top-level value that uses something only a test may use (`Test.*`, the
// Prelude's `Doc` and `Answer`, a fake's table, another test helper) is a test helper. Tests and
// other test helpers, in any module, may use it; production code may not (E0040), and some
// values can never be one (a page's wired values, App's, a fake's entries, a brand check, a
// field default). Mirrors the scoped-operation propagation of rules.go.
package check

import (
	"sort"

	"alder/internal/diag"
)

// testEdge is a reference, at span, from the code being checked to the top-level value to
// ("Module.name"): from is the referring value's key, or "" when the code can never be a
// helper, and never says why (non-empty exactly then); name is to as written there.
type testEdge struct {
	from, never, to, name string
	span                  diag.Span
}

// helperBan is why the code being checked can never be a test helper, "" when it can (a plain
// top-level value) or when it is a test.
func (mc *modCtx) helperBan() string {
	switch {
	case mc.inTest:
		return ""
	case mc.fake != "":
		return "a fake's entry is not a helper"
	case mc.never != "":
		return mc.never
	case mc.value == "":
		return "it is not a helper"
	case mc.isApp:
		return "`App." + mc.value + "` is not a helper"
	case mc.page != nil && (mc.value == "load" || mc.value == "view" || mc.value == "update"):
		return "`" + mc.value + "` is the page's " + mc.value + ", not a helper"
	}
	return ""
}

// testUse handles what (“`Test.fetch`”, “`Doc`”), something only a test may use, at span: in a
// test it is fine; in a plain top-level value it makes the value a test helper (the first such
// thing is its reason); anywhere else it is E0040, once per site. It reports whether the use may
// go on as in a test.
func (mc *modCtx) testUse(what string, span diag.Span) bool {
	c := mc.c
	if mc.inTest || c.prog.Std[mc.m.Name] != nil {
		return true
	}
	if ban := mc.helperBan(); ban != "" {
		if c.testReported == nil {
			c.testReported = map[diag.Span]bool{}
		}
		if !c.testReported[span] {
			c.testReported[span] = true
			c.errorf(diag.TestOnly, span, "%s is only available in a `test` or a test helper: %s", what, ban)
		}
		return false
	}
	k := c.key(mc.m.Name, mc.value)
	if c.testDirect == nil {
		c.testDirect = map[string]string{}
	}
	if c.testDirect[k] == "" {
		c.testDirect[k] = what
	}
	return true
}

// helperRef records a reference, at span, to the top-level value module.name, written as name
// (`fetchText`, `Helpers.fetchText`): markTestHelpers decides, once every body is checked,
// whether it reaches a test helper. A test's references are not recorded.
func (mc *modCtx) helperRef(module, name, written string, span diag.Span) {
	c := mc.c
	if mc.inTest || c.prog.Std[mc.m.Name] != nil || c.prog.Std[module] != nil {
		return
	}
	e := testEdge{never: mc.helperBan(), to: c.key(module, name), name: written, span: span}
	if e.never == "" {
		e.from = c.key(mc.m.Name, mc.value)
	}
	c.testEdges = append(c.testEdges, e)
}

// markTestHelpers computes Result.TestOnly, every value that uses something only tests may use
// directly or through the values it references (with the first reason), and reports E0040 at
// each reference to one from code that can never be a helper.
func (c *Checker) markTestHelpers() {
	only := map[string]string{}
	for k, why := range c.testDirect {
		only[k] = why
	}
	// a stable order, so a value reaching two helpers names the same one every run
	edges := append([]testEdge(nil), c.testEdges...)
	sort.SliceStable(edges, func(i, j int) bool { return lessSpan(edges[i].span, edges[j].span) })
	for changed := true; changed; {
		changed = false
		for _, e := range edges {
			if e.from == "" || only[e.from] != "" || only[e.to] == "" {
				continue
			}
			only[e.from] = "`" + e.name + "`, a test helper"
			changed = true
		}
	}
	for _, e := range edges {
		if e.from == "" && only[e.to] != "" {
			c.errorf(diag.TestOnly, e.span, "`%s` is a test helper (it uses %s): only a `test` or another test helper may use it", e.name, only[e.to])
		}
	}
	c.res.TestOnly = only
}

// written is a name as a reference spells it: `fetchText`, or `Helpers.fetchText` through its
// module's qualifier.
func written(qualifier, name string) string {
	if qualifier == "" {
		return name
	}
	return qualifier + "." + name
}

func lessSpan(a, b diag.Span) bool {
	if a.File != b.File {
		return a.File < b.File
	}
	if a.Start.Line != b.Start.Line {
		return a.Start.Line < b.Start.Line
	}
	return a.Start.Col < b.Start.Col
}
