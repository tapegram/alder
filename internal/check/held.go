package check

import "strings"

// What this file decides is diag.Fix.Held (#327): which migration fixes `alder fmt` waits with
// while a module of the program has errors. Such a module is not checked, so a change that reaches
// it, or that depends on what it holds, cannot be judged safe.

// anyFailed reports whether an app module of the program (names.Program.Modules, which holds no
// standard-library module) failed (names.Program.Failed).
func (c *Checker) anyFailed() bool {
	for name, failed := range c.prog.Failed {
		if failed && c.prog.Modules[name] != nil {
			return true
		}
	}
	return false
}

// importedByFailed reports whether a module that failed imports mod: its references to mod's
// values were not checked, so a rename in mod could break them unseen.
func (c *Checker) importedByFailed(mod string) bool {
	for name, failed := range c.prog.Failed {
		m := c.prog.Modules[name]
		if !failed || m == nil {
			continue
		}
		for _, im := range m.Imports {
			if im.Module == mod {
				return true
			}
		}
	}
	return false
}

// reachesFailed reports whether mod imports, directly or through other modules, a module that
// failed: what mod's values need through it is not known.
func (c *Checker) reachesFailed(mod string) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(name string) bool {
		m := c.prog.Modules[name]
		if m == nil || seen[name] {
			return false
		}
		seen[name] = true
		for _, im := range m.Imports {
			if c.prog.Failed[im.Module] && c.prog.Modules[im.Module] != nil || walk(im.Module) {
				return true
			}
		}
		return false
	}
	return walk(mod)
}

// moduleOf is the module of value key ("Module.value", where Module may itself hold dots): the
// longest module name that prefixes it.
func (c *Checker) moduleOf(key string) string {
	best := ""
	for name := range c.prog.Modules {
		if len(name) > len(best) && strings.HasPrefix(key, name+".") {
			best = name
		}
	}
	return best
}

// clauseHeld reports whether the `requires` clause W0014 adds to a helper of mc's module, for the
// missing keys of its inferred need, waits while a module has errors: a key needed through a
// helper of another module, or a module with errors that mc's module reaches (what it needs
// through that module is not known, so the clause could come out short).
func (mc *modCtx) clauseHeld(inferred map[string]need, missing []string) bool {
	c := mc.c
	if !c.anyFailed() {
		return false
	}
	for _, key := range missing {
		if via := c.moduleOf(inferred[key].Via); via != "" && via != mc.m.Name {
			return true
		}
	}
	return c.reachesFailed(mc.m.Name)
}
