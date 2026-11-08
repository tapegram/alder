// Page-module rules (chapter 6), unreachable since the old syntax is E0046 (#160): Task C deletes them.
package check

import (
	"alder/internal/types"
)

// checkPage checks the required signatures of a page module. App rules run from Program once
// every module's bodies are checked (see checkApp).
func (mc *modCtx) checkPage() {
	c := mc.c
	if mc.isApp || mc.page == nil {
		return
	}
	html := func(arg types.Type) types.Type { return &types.App{Con: "Html", Args: []types.Type{arg}} }
	mc.requireSig("load", &types.Fun{Params: []types.Type{mc.page.Params, mc.session}, Result: &types.App{Con: "Query", Args: []types.Type{mc.modelType()}}},
		hookSig("load"), hookMessage("load"))
	viewMsg := hookMessage("view") + ", or `Model -> Session -> File` for a file page"
	if mc.page.File {
		mc.requireSig("view", &types.Fun{Params: []types.Type{mc.modelType(), mc.session}, Result: types.File}, "Model -> Session -> File", viewMsg)
		return
	}
	mc.requireSig("view", &types.Fun{Params: []types.Type{mc.modelType(), mc.session}, Result: html(mc.page.Msg)}, hookSig("view"), viewMsg)
	mc.requireSig("update", &types.Fun{Params: []types.Type{mc.page.Msg, mc.modelType(), mc.session}, Result: &types.App{Con: "Task", Args: []types.Type{c.navType()}}},
		hookSig("update"), hookMessage("update"))
}

func (mc *modCtx) modelType() types.Type {
	if t, ok := mc.c.types[mc.c.key(mc.m.Name, "Model")]; ok {
		return t
	}
	return types.Err // a page without `Model` is reported as E0010
}

func (c *Checker) navType() types.Type {
	if t, ok := c.types["Prelude.Nav"]; ok {
		return t
	}
	return c.u.Fresh()
}

func viewIsNever(t types.Type) bool {
	fn, ok := t.(*types.Fun)
	if !ok {
		return false
	}
	h, ok := fn.Result.(*types.App)
	return ok && h.Con == "Html" && len(h.Args) == 1 && isCon(h.Args[0], "Never")
}
