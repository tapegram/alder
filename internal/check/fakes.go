// Stateful fakes: `fake Cap { tables; entries }` beside capability Cap, and `Test.fake Cap`.
package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// declareFake records fake d of module m in pass 1: a second fake of one capability is E0009
// (and is skipped by every later pass), and each of its tables gets a placeholder like an app
// table, with the fake it belongs to in fakeTables. dup is declareTypeNames' duplicate check.
func (c *Checker) declareFake(m *syntax.Module, d *syntax.FakeDecl, dup func(name string, sp diag.Span) bool, cur *syntax.Decl) {
	if dup("fake "+d.Name, d.Span) {
		return
	}
	for _, t := range d.Tables {
		*cur = t
		if dup(t.Name, t.Span) {
			continue
		}
		k := c.key(m.Name, t.Name)
		c.types[k] = &types.Record{Module: m.Name, Name: t.Name}
		c.fakeTables[k] = d.Name
	}
}

// defineFakes runs after every capability is defined: a fake declared outside its capability's
// module is E0020; each entry of the capability needs exactly one definition (a missing or an
// extra one is E0003, a second one E0009). A fake that passes is recorded in c.fakes, and only
// those have their entries checked.
func (mc *modCtx) defineFakes() {
	c := mc.c
	for _, d := range c.live(mc.m) {
		fd, ok := d.(*syntax.FakeDecl)
		if !ok || fd.Ref.Kind != syntax.RefCapability {
			continue // unresolved: names reported it
		}
		if fd.Ref.Module != mc.m.Name {
			c.errorf(FakePlacement, fd.Span, "`fake %s` must be declared in the module that declares `capability %s`", fd.Name, fd.Name)
			continue
		}
		entries, ok := c.caps[fd.Ref.Name]
		if !ok {
			continue
		}
		c.fakes[fd.Ref.Name] = fd
		defined := map[string]*syntax.ValueDecl{}
		for _, e := range fd.Entries {
			if prev, dup := defined[e.Name]; dup {
				c.errorf(diag.Duplicate, e.Span, "`%s` is already defined in `fake %s` at line %d", e.Name, fd.Name, prev.Span.Start.Line)
				c.losers[e] = true
				continue
			}
			defined[e.Name] = e
			if _, isEntry := entries[e.Name]; !isEntry {
				c.errorf(diag.TypeMismatch, e.Span, "`fake %s` defines `%s`, which `capability %s` has no entry for", fd.Name, e.Name, fd.Name)
			}
		}
		for _, name := range mc.capEntryOrder(fd.Name) {
			if defined[name] == nil {
				c.errorf(diag.TypeMismatch, fd.Span, "`fake %s` lacks `%s` (`%s`)", fd.Name, name, mc.ts(entries[name]))
			}
		}
	}
}

// capEntryOrder is the entry names of capability name of this module, in declaration order.
func (mc *modCtx) capEntryOrder(name string) []string {
	for _, d := range mc.c.live(mc.m) {
		if cd, ok := d.(*syntax.CapDecl); ok && cd.Name == name {
			var out []string
			for _, e := range cd.Entries {
				out = append(out, e.Name)
			}
			return out
		}
	}
	return nil
}

// checkFakeEntries checks each entry of fake d (one defineFakes accepted) against the
// capability's entry type as its signature. Inside them the fake's own tables may be used.
func (mc *modCtx) checkFakeEntries(d *syntax.FakeDecl) {
	c := mc.c
	if c.fakes[d.Ref.Name] != d {
		return
	}
	entries := c.caps[d.Ref.Name]
	mc.fake = d.Name
	defer func() { mc.fake = "" }()
	for _, e := range d.Entries {
		if sig, ok := entries[e.Name]; ok && !c.losers[e] && e.Body != nil {
			from := len(c.bag.Items)
			mc.checkValue(e, sig)
			for i := from; i < len(c.bag.Items); i++ { // say which entry every diagnostic is in
				it := &c.bag.Items[i]
				it.Message = "in `fake " + d.Name + "`'s `" + e.Name + "`: " + it.Message
			}
		}
	}
}

// fakeTableUse reports E0021 when the table tableKey ("Module.Table") belongs to a fake and is
// used at span outside a `test` body and outside that fake's own entries. Each place is
// reported once (a type may be converted more than once).
func (mc *modCtx) fakeTableUse(tableKey, table string, span diag.Span) {
	c := mc.c
	owner, ok := c.fakeTables[tableKey]
	if !ok || mc.inTest || (mc.fake == owner && tableKey == c.key(mc.m.Name, table)) {
		return
	}
	if c.fakeUses[span] {
		return
	}
	if mc.value != "" && mc.fake == "" && mc.helperBan() == "" { // a plain value: it becomes a helper
		mc.testUse("`"+table+"`", span)
		return
	}
	c.fakeUses[span] = true
	c.errorf(FakeTable, span, "`%s` is a fake's table: only tests and `fake %s` may use it", table, owner)
}

// inferTestFake types the special form `Test.fake Cap`: only inside a `test`, its one argument
// a capability name whose module declares a `fake` for it; a `Task Unit`.
func (mc *modCtx) inferTestFake(x *syntax.App, v *syntax.Var, pipe *pipeArg) types.Type {
	c := mc.c
	if !mc.testUse("`Test.fake`", v.Span) {
		return types.Err
	}
	if len(x.Args) > 1 || (pipe != nil && len(x.Args) > 0) {
		c.errorf(diag.Arity, x.Span, "too many arguments: `Test.fake` takes 1, got %d", len(x.Args)+boolInt(pipe != nil))
		return types.Err
	}
	var arg syntax.Expr
	if len(x.Args) == 1 {
		arg = x.Args[0]
	}
	ct, isName := arg.(*syntax.CtorRef)
	if !isName || ct.Ref.Kind != syntax.RefCapability {
		if isName && ct.Ref.Kind == syntax.RefNone {
			return types.Err // unresolved: names reported it
		}
		span := x.Span
		if pipe != nil {
			span = pipe.span
		} else if arg != nil {
			span = arg.GetSpan()
		}
		c.errorf(diag.TypeMismatch, span, "`Test.fake` wants a capability name, got an expression")
		return types.Err
	}
	if c.fakes[ct.Ref.Name] == nil {
		c.errorf(diag.TypeMismatch, ct.Span, "`Test.fake` wants a capability with a `fake`: `%s` has none", ct.Name)
		return types.Err
	}
	res := &types.App{Con: "Task", Args: []types.Type{types.Unit}}
	// the capability's name is not a value: recorded as Unit, as a step's form name is
	c.record(ct, types.Unit)
	c.record(v, &types.Fun{Params: []types.Type{types.Unit}, Result: res})
	return res
}

// columnTable is the table a column reference names: `Charge` in `Charge.cents`, and in the
// form `Payments.Charge.cents` a fake's table takes from another module. It reads the written
// module's last segment, which is always the table's name: an app table's module is named after
// it (a single segment, whose qualifier is its name), and a fake's table is written after its
// module's qualifier.
func columnTable(x *syntax.Var) string {
	if i := strings.LastIndexByte(x.Module, '.'); i >= 0 {
		return x.Module[i+1:]
	}
	return x.Module
}

// entryCap is the capability a capability entry's reference names, from its Ref ("Clock.now"):
// the written module may be a qualifier that is not the capability's name (`LibMarkdown.render`
// for `capability Markdown` of `import Lib.Markdown as LibMarkdown`).
func entryCap(x *syntax.Var) string {
	if i := strings.LastIndexByte(x.Ref.Name, '.'); i >= 0 {
		return x.Ref.Name[:i]
	}
	return x.Ref.Name
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// FakePlacement is E0020 and FakeTable E0021 (see internal/diag).
const (
	FakePlacement = diag.FakePlacement
	FakeTable     = diag.FakeTable
)
