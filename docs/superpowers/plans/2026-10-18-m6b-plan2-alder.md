# M6b plan 2a — alder: hook hover, `new msg` comma, `--example` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The compiler-side half of M6b plan 2: hover text for the page hooks and the `page` header, `new msg`/`new form` inserting the trailing comma a hand-written `update` lacks, and `--example` on `new page` with an editor-only default.

**Architecture:** Three independent changes in `~/projects/alder`, each behind existing seams: `internal/check` gains a `HookDoc` table that `checkPage` and the LSP hover both read; `internal/generate.addDecls` gains one more edit; `generate.Field` gains `Default`, applied only in `generate.Form` (the editor's form), and `new page` gains the `example` flag and a template branch. Nothing in the LSP protocol changes.

**Tech Stack:** Go 1.2x, `text/template` with `[[ ]]` delimiters, the repo's golden tests (`testdata/help.json`, `testdata/generate/want/<case>/`), `scripts/test.sh`.

**Spec:** `docs/superpowers/specs/2026-10-18-m6b-plan2-nui-modal-design.md` §4 (and §6 for gates). The alder.nvim half is `2026-10-18-m6b-plan2-nvim-modal.md`; it depends on this plan only through `Field.Default` (Task 4).

**Branch:** work on `m6b-plan2` in `~/projects/alder` (create it from `main` at the start; `superpowers:using-git-worktrees` if isolation is wanted). Commit after every task.

## Global Constraints

- Every CLI message and refusal keeps the repo's voice: one sentence, backticks around code, what to do instead (see `Refusef` calls in `internal/generate/page.go`).
- `go test ./cmd/alder` byte-compares `help --json` with `testdata/help.json` (version stripped) and each generator case with `testdata/generate/want/<case>/`; regenerate with `go run ./cmd/alder help --json > testdata/help.json` and `go test ./cmd/alder -run TestGenerate -update`, then read the diff before committing.
- `cmd/alder/generate.go:235 newPageUsage` must list every flag of the `new page` Spec (a test pins the usage text against the Spec).
- The CLI default of `--example` is off; only `generate.Form` (called by the LSP) applies `Field.Default`.
- Generated pages must check clean and pass `alder test` (the `test: true` golden cases run both).
- Quick loop: `scripts/test.sh -short`; full gate before merge: `scripts/test.sh` (~200 s with 4 shards).

## Review Focus

1. Hovering `load` in a module that is not a page (e.g. a stdlib or capability module defining a value called `load`) must show the plain signature, never "Page hook" — Task 2 tests it on `perPage` and on a non-page module.
2. `new msg` on an `update` whose last arm ends in a `-- comment` after the code must put the comma before the comment, not inside it — Task 3 tests it.
3. `new msg` on an `update` whose match body is empty (`match msg {\n}`) must not touch the `{` line — Task 3 tests it.
4. `--example` combined with `--param` must be refused (the example adds its own `count` parameter) — Task 5 tests it.
5. `new page About /about` without `--example` must produce byte-identical output to today (the `page` golden case) — Task 5 keeps that golden unchanged.

---

### Task 1: `check.HookDoc` — one table for the hook signatures and roles

**Files:**
- Create: `internal/check/hookdoc.go`
- Create: `internal/check/hookdoc_test.go`
- Modify: `internal/check/pages.go:12-24` (`checkPage`)

**Interfaces:**
- Produces: `func HookDoc(name string) (sig, role string, ok bool)` in package `check`. `sig` is the required type as Alder text (`Params -> Session -> Query Model`), `role` a markdown paragraph, `ok` false for any name but `load`, `view`, `update`.

- [ ] **Step 1: Write the failing test**

```go
// internal/check/hookdoc_test.go
package check

import (
	"strings"
	"testing"
)

func TestHookDoc(t *testing.T) {
	want := map[string]string{
		"load":   "Params -> Session -> Query Model",
		"view":   "Model -> Session -> Html Msg",
		"update": "Msg -> Model -> Session -> Task Nav",
	}
	for name, sig := range want {
		got, role, ok := HookDoc(name)
		if !ok || got != sig {
			t.Errorf("HookDoc(%s) = %q, %v; want %q", name, got, ok, sig)
		}
		if !strings.HasPrefix(role, "Page hook. ") || !strings.Contains(role, "`"+sig+"`") {
			t.Errorf("HookDoc(%s) role %q: must start with \"Page hook. \" and quote the signature", name, role)
		}
	}
	if _, _, ok := HookDoc("Model"); ok {
		t.Error("Model is not a hook")
	}
	if _, _, ok := HookDoc(""); ok {
		t.Error("the empty name is not a hook")
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd ~/projects/alder && go test ./internal/check -run TestHookDoc`
Expected: FAIL — `undefined: HookDoc`.

- [ ] **Step 3: Write `HookDoc` and make `checkPage` read it**

```go
// internal/check/hookdoc.go
package check

// HookDoc is what the editor says about a page hook (hover, chapter 11): its required
// signature as Alder text and its role, one paragraph of markdown. ok is false for a name that
// is not a hook. checkPage reads the same table, so the two cannot drift.
func HookDoc(name string) (sig, role string, ok bool) {
	switch name {
	case "load":
		return "Params -> Session -> Query Model",
			"Page hook. Runs on every request before `view`: reads the page parameters and the session " +
				"and builds the `Model`. Required: `Params -> Session -> Query Model`.", true
	case "view":
		return "Model -> Session -> Html Msg",
			"Page hook. Renders the `Model` to HTML; `click` and `submit` in it send `Msg` values to " +
				"`update`. Required: `Model -> Session -> Html Msg`.", true
	case "update":
		return "Msg -> Model -> Session -> Task Nav",
			"Page hook. Handles one `Msg` and answers with a `Task Nav`: `Rerender` runs `load` and " +
				"`view` again, `Redirect` goes to a page (the same page with new parameters is how a page " +
				"keeps state), `Reject` answers with an error. State lives in the page parameters, the " +
				"session or a table — `Model` is rebuilt by `load` on every request. " +
				"Required: `Msg -> Model -> Session -> Task Nav`.", true
	}
	return "", "", false
}

// hookMessage is checkPage's E-message for hook name: "`load` must have type `…`".
func hookMessage(name string) string {
	sig, _, _ := HookDoc(name)
	return "`" + name + "` must have type `" + sig + "`"
}
```

In `internal/check/pages.go` replace the three literal messages with `hookMessage("load")`, `hookMessage("view")`, `hookMessage("update")`:

```go
	mc.requireSig("load", &types.Fun{Params: []types.Type{mc.page.Params, mc.session}, Result: &types.App{Con: "Query", Args: []types.Type{mc.modelType()}}},
		hookMessage("load"))
	mc.requireSig("view", &types.Fun{Params: []types.Type{mc.modelType(), mc.session}, Result: html(mc.page.Msg)},
		hookMessage("view"))
	mc.requireSig("update", &types.Fun{Params: []types.Type{mc.page.Msg, mc.modelType(), mc.session}, Result: &types.App{Con: "Task", Args: []types.Type{c.navType()}}},
		hookMessage("update"))
```

- [ ] **Step 4: Run the check package tests**

Run: `go test ./internal/check`
Expected: PASS (the hook-signature diagnostics' text is unchanged, so every golden that quotes it still matches; if one fails, the message text differs from the literal it replaced — fix the literal in `HookDoc`, not the golden).

- [ ] **Step 5: Commit**

```bash
git add internal/check/hookdoc.go internal/check/hookdoc_test.go internal/check/pages.go
git commit -m "check: HookDoc, the page hooks' signatures and roles in one table"
```

---

### Task 2: Hover for `load`/`view`/`update` and the `page` header

**Files:**
- Modify: `internal/lsp/hover.go:39-85` (`hoverText`) + two new functions
- Test: `internal/lsp/hover_test.go`

**Interfaces:**
- Consumes: `check.HookDoc` (Task 1); `declAt(m, pos)` and `nameSpan(d, m)` from `internal/lsp/scope.go`; `a.prog.Pages[mod]` (non-nil for a page module, as `internal/lsp/actions.go:172` uses it); `a.res.Pages[mod]` (`*check.PageInfo`, its `Params *types.Record`); `types.StringIn`.
- Produces: hover markdown with a "Page hook." paragraph for the hooks; a `page "<path>"` block for the header.

- [ ] **Step 1: Write the failing tests**

Append to `internal/lsp/hover_test.go` (`hover`, `openClean`, `blogCopy`, `start`, `c.at` exist there; `examples/blog/app/Posts/Index.alder` has `page "/" { page: Int = 1, q: Text = "" }`, `load params session = {`, `update msg model session = match msg {`, `view model session =`):

```go
func TestHover_PageHooks(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	cases := []struct{ needle, sig, role string }{
		{"load params session", "load : Params -> Session -> Query Model", "Page hook. Runs on every request"},
		{"view model session", "view : Model -> Session -> Html Msg", "Page hook. Renders"},
		{"update msg model session", "update : Msg -> Model -> Session -> Task Nav", "`Rerender` runs `load`"},
	}
	for _, tc := range cases {
		got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", tc.needle, 0))
		if !strings.Contains(got, "```alder\n"+tc.sig+"\n```") || !strings.Contains(got, tc.role) {
			t.Errorf("hover on %q:\n%s", tc.needle, got)
		}
	}
	// a value that is not a hook says nothing about hooks
	if got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "lastPage then", 0)); strings.Contains(got, "Page hook") {
		t.Errorf("lastPage is not a hook:\n%s", got)
	}
}

func TestHover_PageHooks_NotInAPage(t *testing.T) {
	c := start(t, blogCopy(t))
	// Post is a table module, not a page: a value called `view` there is an ordinary value
	text := c.disk("app/Post.alder") + "\nview : Int\nview = 1\n"
	c.open("app/Post.alder", text)
	c.diags("app/Post.alder")
	got := hover(t, c, "app/Post.alder", c.at("app/Post.alder", "view = 1", 0))
	if strings.Contains(got, "Page hook") {
		t.Errorf("view outside a page:\n%s", got)
	}
}

func TestHover_PageHeader(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	got := hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", `page "/"`, 0))
	for _, want := range []string{"```alder\npage \"/\"", "params : { page : Int, q : Text }", "hooks"} {
		if !strings.Contains(got, want) {
			t.Errorf("page header hover lacks %q:\n%s", want, got)
		}
	}
	// a type inside the header still hovers as the type
	got = hover(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "q: Text", len("q: ")))
	if strings.Contains(got, "hooks") {
		t.Errorf("Text in the header hovered as the page:\n%s", got)
	}
}
```

If `TestHover_PageHooks_NotInAPage` cannot get a last-good build because `view` outside a page is an error (`internal/check/decls.go:1203-1224`), change the appended value to `viewCount : Int\nviewCount = 1` and hover `viewCount` instead — the point is only that a non-hook shows no role text; keep the test name.

Check the exact `params` rendering: run the test once and copy the `types.StringIn` form of the record into the assertion if it differs from `{ page : Int, q : Text }` (defaults are not part of the type).

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/lsp -run 'TestHover_Page'`
Expected: FAIL — no "Page hook" / no `page "/"` block.

- [ ] **Step 3: Implement**

In `internal/lsp/hover.go`, add `"strings"` and `"alder/internal/check"` to the imports, then in `hoverText` insert this block right after the `switch n := n.(type)` over holes/markup and before `if ref, local, ok := target(...)`:

```go
	// a page hook's name at its declaration, or the page header itself
	if d := declAt(m, pos); d != nil && a.prog.Pages[m.Name] != nil {
		switch d := d.(type) {
		case *syntax.PageDecl:
			if path[len(path)-1] == syntax.Spanned(d) { // the header, not a type inside its parameters
				return pageHover(a, m, d)
			}
		case *syntax.ValueDecl:
			if syntax.Contains(nameSpan(d, m), pos) {
				if h := hookHover(a, m.Name, d.Name); h != "" {
					return withDoc(h, syntax.DocOf(m.Lines, d.GetSpan().Start.Line))
				}
			}
		}
	}
```

In the `RefValue/RefStd/RefComponent` case, before the existing `t, ok := a.res.Values[...]`:

```go
			if h := hookHover(a, ref.Module, ref.Name); h != "" {
				return withDoc(h, declDoc(a, ref))
			}
```

Add the two functions at the end of the file:

```go
// hookHover is the hover of page module mod's hook name (load, view, update): its signature as
// checked (the required one when the module has none yet) and the hook's role. "" when mod is
// not a page or name is not a hook.
func hookHover(a *analysis, mod, name string) string {
	if a.prog.Pages[mod] == nil {
		return ""
	}
	sig, role, ok := check.HookDoc(name)
	if !ok {
		return ""
	}
	if a.res != nil {
		if t, has := a.res.Values[mod+"."+name]; has && t != nil {
			sig = types.StringIn(t, mod)
		}
	}
	return "```alder\n" + name + " : " + sig + "\n```\n\n" + role
}

// pageHover is the hover of a page header: its path, parameter record and `requires` clauses.
func pageHover(a *analysis, m *syntax.Module, d *syntax.PageDecl) string {
	lines := []string{"page \"" + d.Path + "\""}
	if a.res != nil {
		if pi := a.res.Pages[m.Name]; pi != nil && pi.Params != nil {
			lines = append(lines, "params : "+types.StringIn(pi.Params, m.Name))
		}
	}
	if len(d.Requires) > 0 {
		lines = append(lines, "requires "+strings.Join(d.Requires, ", "))
	}
	if len(d.Roles) > 0 {
		lines = append(lines, "roles "+strings.Join(d.Roles, " | "))
	}
	if d.Silo != "" {
		lines = append(lines, "silo "+d.Silo)
	}
	return "```alder\n" + strings.Join(lines, "\n") + "\n```\n\n" +
		"The page's route and what a request needs. `load`, `view` and `update` are its hooks; " +
		"`Params` is this record, `Msg` its messages."
}
```

If `path[len(path)-1] == syntax.Spanned(d)` never holds because `NodeAt` stops above the `PageDecl` (check `syntax.Children` for `*syntax.Module`), use `len(path) == 1` (only the module) as the condition instead; the header test tells you which.

- [ ] **Step 4: Run the hover tests and the whole lsp package**

Run: `go test ./internal/lsp`
Expected: PASS, including the pre-existing `TestHover_*` (`TestHover_DeclWithDoc` still shows the doc comment).

- [ ] **Step 5: Commit**

```bash
git add internal/lsp/hover.go internal/lsp/hover_test.go
git commit -m "lsp: hover explains the page hooks and the page header"
```

---

### Task 3: `addDecls` inserts the missing trailing comma

**Files:**
- Modify: `internal/generate/form.go:383-400` (`addDecls`) + one helper
- Test: `cmd/alder/generate_test.go` (new test function)

**Interfaces:**
- Consumes: `updateArmSpot(e, m)` (`internal/generate/srcedit.go:250`), `e.line(n)`, `e.replaceLine(n, text)`, `valueDecl(m, "update")`.
- Produces: `new msg` and `new form` work on a page whose last `update` arm has no trailing comma.

- [ ] **Step 1: Write the failing test**

Append to `cmd/alder/generate_test.go` (`genApp`, `alderIn`, `readFile`-style helpers exist there; the scaffold's `app/Home.alder` has one arm, `Submit p -> { … },` closing on a `  },` line, then `}`):

```go
// TestGenerate_NewMsgAddsMissingComma: Alder allows the last match arm without a trailing comma,
// so a hand-written `update` often lacks one; `new msg` adds it rather than refusing (M6b plan 2).
func TestGenerate_NewMsgAddsMissingComma(t *testing.T) {
	for _, tc := range []struct{ name, from, to, wantLine string }{
		{"bare", "  },\n}\n\nview", "  }\n}\n\nview", "  },\n"},
		{"comment after the arm", "  },\n}\n\nview", "  } -- the only arm so far\n}\n\nview", "  }, -- the only arm so far\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := genApp(t, "scaffold")
			home := filepath.Join(dir, "app", "Home.alder")
			src, err := os.ReadFile(home)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(src), tc.from) {
				t.Fatalf("scaffold Home.alder no longer has %q:\n%s", tc.from, src)
			}
			if err := os.WriteFile(home, []byte(strings.Replace(string(src), tc.from, tc.to, 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			if code, out, errs := alderIn(t, dir, "", "new", "msg", "Home", "Clear"); code != 0 {
				t.Fatalf("new msg on a page without the trailing comma: exit %d\n%s\n%s", code, out, errs)
			}
			after, _ := os.ReadFile(home)
			if !strings.Contains(string(after), tc.wantLine) || !strings.Contains(string(after), "Clear p ->") {
				t.Fatalf("comma not added before the new arm:\n%s", after)
			}
			if code, out, _ := alderIn(t, dir, "", "check"); code != 0 {
				t.Fatalf("the edited page does not check:\n%s", out)
			}
		})
	}
}

// TestGenerate_NewMsgEmptyMatch: an `update` whose match has no arms yet gets the arm and
// nothing else changes.
func TestGenerate_NewMsgEmptyMatch(t *testing.T) {
	dir := genApp(t, "scaffold")
	home := filepath.Join(dir, "app", "Home.alder")
	src, _ := os.ReadFile(home)
	start := strings.Index(string(src), "update msg model session = match msg {\n")
	end := strings.Index(string(src), "\n}\n\nview")
	if start < 0 || end < 0 {
		t.Fatalf("scaffold changed:\n%s", src)
	}
	// keep `msg Submit form Sign` so the form still has a message; empty the match
	emptied := string(src[:start]) + "update msg model session = match msg {\n" + string(src[end:])
	if err := os.WriteFile(home, []byte(emptied), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := alderIn(t, dir, "", "new", "msg", "Home", "Clear")
	after, _ := os.ReadFile(home)
	if !strings.Contains(string(after), "match msg {\n  Clear p ->") && code == 0 {
		t.Fatalf("exit %d\n%s\n%s\n%s", code, out, errs, after)
	}
}
```

The emptied `update` no longer handles `Submit`, so the checker may refuse the dry run (a non-exhaustive match) and `code` may be 1: the second test only asserts that when the command succeeds the arm follows the `{` line directly and no stray comma was added to `match msg {`. If the scaffold's checker rejects the emptied match outright, assert instead that `after` equals `emptied` (nothing written) and `errs` names the checker — either way the `{` line must be untouched: add `if strings.Contains(string(after), "match msg {,") { t.Fatal("comma added to the match line") }`.

- [ ] **Step 2: Run to see it fail**

Run: `go test ./cmd/alder -run 'TestGenerate_NewMsg(AddsMissingComma|EmptyMatch)'`
Expected: `bare` and `comment after the arm` FAIL with exit 1 and "the declaration the command edits does not check".

- [ ] **Step 3: Implement**

In `internal/generate/form.go`, change the `has` branch of `addDecls` and add the helper:

```go
	if has {
		if arm := lastArmLine(e, m.Syntax, line); arm > 0 {
			if fixed, changed := withTrailingComma(e.line(arm)); changed {
				e.replaceLine(arm, fixed)
			}
		}
		e.insertBefore(declStart(e, valueDecl(m.Syntax, "update")), decls+"\n")
		e.insertBefore(line, indent(arm, ind))
		return nil
	}
```

```go
// lastArmLine is the line of the last code in `update`'s match before its closing `}` at close:
// not blank, not a comment; 0 when the match has no arms yet. Alder allows the last arm without
// a trailing comma, so a new arm after it needs one added there.
func lastArmLine(e *srcEdit, m *syntax.Module, close int) int {
	mt, _ := valueDecl(m, "update").Body.(*syntax.Match)
	for n := close - 1; n > mt.Span.Start.Line; n-- {
		t := strings.TrimSpace(e.line(n))
		if t != "" && !strings.HasPrefix(t, "--") {
			return n
		}
	}
	return 0
}

// withTrailingComma is line with a `,` after its code when it has none, before any trailing
// `-- comment`; changed is false when the comma is already there.
func withTrailingComma(line string) (string, bool) {
	code, comment := line, ""
	if i := strings.Index(line, " --"); i >= 0 {
		code, comment = line[:i], line[i:]
	}
	trimmed := strings.TrimRight(code, " \t")
	if strings.HasSuffix(trimmed, ",") {
		return line, false
	}
	return trimmed + "," + comment, true
}
```

Note `mt.Span.Start.Line` is the line of `match msg {` (the match expression starts there); when the match spans one line `updateArmSpot` already returned `ok == false`, so `lastArmLine` never sees that case.

- [ ] **Step 4: Run the tests**

Run: `go test ./cmd/alder -run 'TestGenerate_NewMsg|TestGenerate_DoesNotCheck' && go test ./internal/generate`
Expected: PASS. Then `go test ./cmd/alder -run TestGenerate` to confirm no golden changed (the goldens' pages all have the comma already, so `changed` is false for them).

- [ ] **Step 5: Commit**

```bash
git add internal/generate/form.go cmd/alder/generate_test.go
git commit -m "generate: new msg/new form add the trailing comma a hand-written update lacks"
```

---

### Task 4: `Field.Default`, applied by `generate.Form` only

**Files:**
- Modify: `internal/generate/spec.go:37-50` (`Field`), `internal/generate/here.go:28-40` (`FieldState` doc) and `:73-104` (`Form`)
- Test: `internal/generate/here_test.go`

**Interfaces:**
- Produces: `Field.Default string`; `Form` fills an unset field with it, `FieldState.Source == "default"`, and `SetsOf` therefore includes it (a bool `Default: "true"` becomes `sets[name] = []string{""}`, i.e. `Has(name)`).
- Consumed by: Task 5 (`new page`'s `example` flag) and the alder.nvim form (the `[x] include example` toggle starts from `value`).

- [ ] **Step 1: Write the failing test**

Append to `internal/generate/here_test.go` (the file already loads a scaffold `Program` as `p` in `TestForm…`; copy that setup — `dir := …; p, err := Load(dir)` — into the new test):

```go
func TestForm_Default(t *testing.T) {
	dir := scaffoldApp(t, nil) // as TestForm_Validation does
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &Spec{ID: "x y", Args: []Field{{Name: "name", Kind: ArgText, Required: true}},
		Flags: []Field{{Name: "example", Kind: ArgBool, Default: "true"}, {Name: "strict", Kind: ArgBool}}}
	fields, sets, _ := Form(s, nil, nil, p)
	byName := map[string]FieldState{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	if f := byName["example"]; f.Value != "true" || f.Source != "default" || !f.Valid {
		t.Fatalf("example: %+v", f)
	}
	if f := byName["strict"]; f.Value != "" || f.Source != "" {
		t.Fatalf("strict has no default: %+v", f)
	}
	if !sets.Has("example") || sets.Has("strict") {
		t.Fatalf("sets %v", sets)
	}
	// you win over the default
	_, sets, _ = Form(s, nil, map[string]string{"example": "false", "name": "A"}, p)
	if sets.Has("example") {
		t.Fatalf("you = false still set: %v", sets)
	}
	// here wins over the default too
	_, sets, _ = Form(s, &Match{Sets: map[string]string{"example": "false"}}, nil, p)
	if sets.Has("example") {
		t.Fatalf("here = false still set: %v", sets)
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/generate -run TestForm_Default`
Expected: FAIL — `unknown field Default in struct literal`.

- [ ] **Step 3: Implement**

`internal/generate/spec.go`, in `Field` after `Choices`:

```go
	// Default is the editor's starting value when neither the cursor (here) nor the user set
	// the field: "true" for a bool flag the form should start checked. Only Form applies it;
	// the CLI's default is always unset, so agents get the minimal result.
	Default string
```

`internal/generate/here.go`, `FieldState.Source` comment: `// "here", "you", "default" or ""`.

`Form`, after the `you` loop and before `ready := true`:

```go
	for _, fs := range [][]Field{s.Args, s.Flags} {
		for _, f := range fs {
			if _, set := values[f.Name]; !set && f.Default != "" {
				values[f.Name], source[f.Name] = f.Default, "default"
			}
		}
	}
```

- [ ] **Step 4: Run the generate and lsp packages**

Run: `go test ./internal/generate ./internal/lsp`
Expected: PASS (no Spec has a `Default` yet, so nothing else changes).

- [ ] **Step 5: Commit**

```bash
git add internal/generate/spec.go internal/generate/here.go internal/generate/here_test.go
git commit -m "generate: Field.Default, the editor form's starting value"
```

---

### Task 5: `new page --example`

**Files:**
- Modify: `internal/generate/specs.go:86-105` (the `new page` Spec), `internal/generate/page.go` (`PageData`, `PageOptions`, `NewPage`), `internal/generate/templates/page.tmpl`, `cmd/alder/generate.go:235` (`newPageUsage`), `cmd/alder/generate_test.go:173` (`genCases`) and `:389` (the refusal table)
- Regenerate: `testdata/help.json`, `testdata/generate/want/page-example/`

**Interfaces:**
- Consumes: `Field.Default` (Task 4).
- Produces: flag `--example` on `alder new page`; `PageOptions.Example bool`; `PageData.Example bool`; refusal text `--example goes with a plain page: a --list, --show or --from page already shows \`load\` reading a table, and --blank and --param leave \`load\` to you`.

- [ ] **Step 1: Add the golden case and the refusal case (failing)**

In `cmd/alder/generate_test.go` `genCases`, after the `page` case:

```go
	{name: "page-example", base: "scaffold", args: []string{"new", "page", "About", "/about", "--example"}, test: true},
```

In the refusal table at `:389` (rows are `{base, args, stdin, errs}`), add:

```go
		{"scaffold", []string{"new", "page", "About", "/about", "--example", "--blank"}, "", "alder new page: --example goes with a plain page: a --list, --show or --from page already shows `load` reading a table, and --blank and --param leave `load` to you"},
		{"scaffold", []string{"new", "page", "Tags.Show", "/tags/{name}", "--example", "--param", "name:Text"}, "", "alder new page: --example goes with a plain page: a --list, --show or --from page already shows `load` reading a table, and --blank and --param leave `load` to you"},
```

Run: `go test ./cmd/alder -run 'TestGenerate$|TestGenerate_Refusals'` (use the real name of the test that walks the refusal table)
Expected: FAIL — `--example` is an unknown flag (usage exit 2).

- [ ] **Step 2: The Spec, the options, the usage**

`internal/generate/specs.go`, `new page` `Flags`, after `blank`:

```go
				{Name: "example", Kind: ArgBool, Default: "true", Summary: "a working example — state in a page parameter, a message, an update arm and a view that uses them — to delete once read"},
```

and in its `Run`: `… Blank: s.Has("blank"), Example: s.Has("example"), Params: s.All("param"), By: s.One("by")}`.

`internal/generate/page.go`, `PageOptions` after `Blank`:

```go
	// Example (--example): the plain page carries a `count: Int = 0` parameter, `msg Bump { by: Int }`,
	// an update arm redirecting to itself with the new count and a view showing it, every part
	// marked `-- example:` so the reader deletes it once understood. Off on the CLI; the editor's
	// form starts with it on (Field.Default).
	Example bool
```

`PageData` after `By`:

```go
	// Example: page.tmpl renders the --example page instead of the empty one
	Example bool
```

In `NewPage`, right after the `kinds > 1` refusal:

```go
	if o.Example && (o.Blank || kinds > 0 || len(o.Params) > 0) {
		return nil, Refusef("--example goes with a plain page: a --list, --show or --from page already shows `load` reading a table, and --blank and --param leave `load` to you")
	}
```

Where `d` is built (`d := PageData{Module: o.Module, Path: o.Path, Params: "{ }", Title: Title(o.Module)}`), add right after it:

```go
	if o.Example {
		d.Example = true
		d.Params = "{ count: Int = 0 }"
	}
```

`cmd/alder/generate.go:235`: insert `[--example]` after `[--blank]` in `newPageUsage`.

- [ ] **Step 3: The template**

Replace `internal/generate/templates/page.tmpl` with:

```
[[- template "header" .]]
[[if .Example]]
-- example: a page keeps state in its parameters (or the session, or a table), never in Model
-- between requests: `load` rebuilds Model on every request. Delete every part marked
-- `example` when you have seen how they fit together (the `count` parameter above is one).

type Model = { count: Int }

load : Params -> Session -> Query Model
load params session = Query.succeed { count: params.count }

-- example: a message carries a record; the view sends it from `click`.
msg Bump { by: Int }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  -- example: `update` answers with a Nav: Redirect to the same page with the new count
  -- (Rerender would run `load` again with the old parameters), or Reject with an error.
  Bump p -> Task.succeed (Redirect ([[.Module]] { count: model.count + p.by })),
}

-- example: the view shows the state and sends a message from a button.
view : Model -> Session -> Html Msg
view model session =
  [[.Open]]
    <h1>[[.Title]]</h1>
    <p id="count">Count: {Text.fromInt model.count}</p>
    <al-button click={Bump { by: 1 }}>Bump</al-button>
  [[.Close]]
[[else]]
type Model = { }

load : Params -> Session -> Query Model
load params session = [[if .LoadHole]]?load[[else]]Query.succeed { }[[end]]

view : Model -> Session -> Html Msg
view model session =
  [[.Open]]
    <h1>[[.Title]]</h1>
  [[.Close]]
[[end]]
[[- template "tests" .]]
```

Whitespace matters: the existing `page` golden must stay byte-identical. After the next step, `git status` must show no change under `testdata/generate/want/page/`; if it does, adjust the `[[else]]`/`[[end]]` trimming (`[[- else]]`, `[[- end]]`) until the plain page matches the old template's output exactly (one blank line between the header and `type Model`, and the file ending as before).

If the checker complains that `Text` is unknown in the generated page, add `"Text"` to the imports where `NewPage` builds `imports` for the example (`imports.add("Text")` or the equivalent on `newImports`'s result — read `internal/generate/imports.go`); if `Layout.Page` is the root, `Layout` is already imported by `rootTags`.

- [ ] **Step 4: Regenerate the goldens, read them**

```bash
go test ./cmd/alder -run TestGenerate -update
go run ./cmd/alder help --json > testdata/help.json
git status --short testdata
git diff testdata/help.json | head -40
cat testdata/generate/want/page-example/files/app/About.alder
```

Expected: only `testdata/help.json` (one new flag node under `new page`) and the new `testdata/generate/want/page-example/` appear; `About.alder` reads as the spec's §4.3 example with `About { count: … }` in the redirect. The `page` golden is untouched.

- [ ] **Step 5: Run the cmd tests**

Run: `go test ./cmd/alder -run 'TestGenerate|TestHelpJSON|Usage'`
Expected: PASS — `page-example` checks, `alder test` passes on it (the generated heading test), the refusals match, the usage pin sees `--example`.

Then the whole package once: `go test ./cmd/alder -short` (a few minutes).

- [ ] **Step 6: Commit**

```bash
git add internal/generate cmd/alder/generate.go cmd/alder/generate_test.go testdata/help.json testdata/generate/want/page-example
git commit -m "new page --example: a working state → msg → update → view example, editor default on"
```

---

### Task 6: Docs and the full gate

**Files:**
- Modify: `book/15-generators.md` (section `## A page: \`alder new page\``, ~line 78), `book/11-tooling.md` (the editor section around line 1640 — hover)

- [ ] **Step 1: Book**

In `book/15-generators.md`, at the end of the `## A page: alder new page` section (before `## A table`), add:

```markdown
A newcomer can ask for more than the empty page: `alder new page About /about --example`
adds a small, working example of how a page keeps state — a `count` parameter, a
`msg Bump { by: Int }`, an `update` arm that redirects to the same page with the new count,
and a view with a button that sends it. Every part is marked `-- example:` and is meant to
be deleted once read. The editor's generator form starts with the example on; the command
line starts with it off, so a program driving the CLI gets the minimal page.
```

In `book/11-tooling.md`, where the editor's hover is described (search for `**Hover**` or `K` near line 1640), add one sentence:

```markdown
On a page's `load`, `view` or `update`, hover also says what the hook does and what it must
return; on the `page` line it shows the route, the parameter record and the `requires` clauses.
```

- [ ] **Step 2: Full gate**

Run: `cd ~/projects/alder && scripts/test.sh`
Expected: every line `ok`, exit 0. Paste the summary lines into the commit message body if anything was flaky and re-run.

- [ ] **Step 3: Commit**

```bash
git add book/15-generators.md book/11-tooling.md
git commit -m "book: --example and the hook hover"
```

Then hand over to the alder.nvim plan (`2026-10-18-m6b-plan2-nvim-modal.md`), which needs this branch's `alder` binary on PATH (`go install ./cmd/alder` puts it at `~/go/bin/alder`, which the user's Neovim uses).
