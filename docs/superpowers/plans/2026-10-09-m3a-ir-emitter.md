# M3a IR and Emitter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lower a checked Alder program to a typed IR and emit Go for the pure language (types, values, expressions, `match`, HTML, stdlib calls) that `go vet`s against a new runtime core, plus the four compiler improvements E5 justified.

**Architecture:** The checker gains a `Result` that records the type of every expression and binding it checked. `ir.Lower` turns the AST plus `Result` into a resolved, typed, desugared tree with no Go in it. `gen.Emit` writes one Go file per Alder module into a single Go package `app`, naming things `<Module>__<name>`, mapping records to structs, unions to tagged structs, brands to their base, generics to `rt` containers, and HTML to `rt/html` builder calls. A small runtime core (`rt`, `rt/html`, `rt/std`) exists so the generated code compiles; effects, pages and routing are M3b/M3c.

**Tech Stack:** Go 1.27 stdlib only (compiler and `rt` in M3a; `modernc.org/sqlite` arrives in M3b), `go/format`, `go vet` in tests.

**Spec:** `docs/superpowers/specs/2026-10-09-m3-codegen-runtime-design.md` (sections IR, Emitter, Runtime `rt/html`, Compiler items, Testing, Milestone M3a). Language reference: `book/`. Prior plans for conventions: `docs/superpowers/plans/2026-10-08-m1-alder-check.md`, `2026-10-09-m2-checker-loop.md`.

## Global Constraints

- Go `1.27`, module `alder`, stdlib only; `gofmt -l .` empty; `go vet ./... && go test ./...` green before every commit; `alder check examples/todos` clean; `python3 -m unittest` in `experiments/` still green.
- The emitter never sees an ill-typed program: `ir.Lower` and `gen.Emit` are called only when the checker reported no errors and no holes (W0002); `alder build` refuses otherwise with the diagnostics.
- Generated Go is formatted with `go/format`; names are `<Module with '.' replaced by '_'>__<name>`; Go keywords and `rt`-reserved names get a trailing `_`; all generated files are `package app`.
- Type mapping (spec table): `Int`→`int64`, `Float`→`float64`, `Bool`→`bool`, `Text`→`string`, `Unit`→`struct{}`, `Date`→`rt.Date`, `Time`→`time.Time`, brands→their base, `List a`→`[]T`, `Maybe a`→`rt.Maybe[T]`, `Result e a`→`rt.Result[E, T]`, `Dict k v`→`rt.Dict[K, V]`, `Html a`→`html.Node`, `Page`→`rt.Page`, records→`struct` (nominal: named type; anonymous: anonymous struct literal type), unions→`struct{ Tag <U>Tag; <one field per payload> }`.
- Diagnostic codes stay stable; new parse messages keep code E0001; the poison type never appears in a message.
- Commit messages end with the two attribution lines given in the conversation.

## Review Focus

1. A union constructor whose payload is an anonymous record (`Rect { w: Float, h: Float }`) — must emit a named payload struct, not an unnamable anonymous type in the union (Task 5 `TestGen_UnionAnonymousPayload`).
2. An Alder identifier that is a Go keyword or collides with a generated name (`type`, `func`, `Tag`, `go`) — must be mangled consistently in declaration and use (Task 5 `TestGen_KeywordNames`).
3. Text interpolation containing a `{` escaped as `\{` and a `"` — the emitted Go string literal must round-trip (Task 5 `TestGen_StringEscapes`).
4. A module with no top-level values (only types) — must still emit a valid Go file (Task 5 `TestGen_TypesOnlyModule`).
5. HTML text containing `<`, `&`, and a non-ASCII character — escaped exactly once, by the builder, not by the emitter (Task 3 `TestHTML_EscapeOnce`, Task 6 `TestGen_HTMLTextNotPreEscaped`).

---

## File structure

```
internal/syntax/parser*.go        Task 1: parse-error context ("in a form field: …")
internal/types/types.go           Task 1: `Err` poison type; unify treats it as compatible
internal/check/{decls,expr,pattern}.go   Task 1: poison on unresolved; E0007 inferred type
internal/names/resolve.go         Task 1: E0201 dedupe per module
testdata/corpus/{E0001-form-attr,E0001-inline-brand,E0201-dedupe,E0007-inferred}/
internal/check/result.go          Task 2: Result type + recording
internal/check/check.go           Task 2: Program returns *Result
cmd/alder/main.go, internal/check/check_test.go   Task 2: callers
rt/core.go                        Task 3: Maybe, Result, Dict, Date, Concat, OrElse, Append, Compare helpers
rt/core_test.go
rt/html/html.go                   Task 3: Node, El, Text, Raw, Attr, Render (escaping)
rt/html/html_test.go
rt/std/{list,maybe,result,text,num,date,html}.go  Task 3: stdlib implementations used by the emitter
rt/std/std_test.go
internal/ir/ir.go                 Task 4: node types
internal/ir/lower.go              Task 4: Lower
internal/ir/dump.go               Task 4: Dump for goldens
internal/ir/ir_test.go, testdata/golden/ir_*/
internal/gen/names.go             Task 5: mangling
internal/gen/types.go             Task 5: type mapping, type declarations
internal/gen/expr.go              Task 5: expressions, match
internal/gen/gen.go               Task 5: Emit, file assembly, go/format
internal/gen/gen_test.go, testdata/golden/gen_*/
internal/gen/html.go              Task 6: HTML and components
internal/gen/stdcall.go           Task 6: stdlib call mapping table
cmd/alder/main.go                 Task 6: `alder build --emit` (emit + vet, no main.go yet)
book/11-tooling.md                Task 6: build status
```

---

### Task 1: The four E5 compiler items

**Files:**
- Modify: `internal/syntax/parser.go` (`expect`, `errorAt`, `parseFieldList`, a context stack), `internal/syntax/parser_expr.go` (type positions), `internal/types/types.go`, `internal/types/unify.go`, `internal/check/decls.go` (`typeOf`, E0007), `internal/check/expr.go` (`mismatch`), `internal/check/pattern.go` (exhaustiveness on poison), `internal/names/resolve.go` (`notImported`)
- Create: `testdata/corpus/E0001-form-attr/`, `testdata/corpus/E0001-inline-brand/`, `testdata/golden/check_poison/`, `testdata/corpus/E0201-dedupe/` (replaces nothing; `E0201-missing-import` stays), `testdata/corpus/E0007-inferred/`
- Modify: `experiments/corpus_notes.md`, `internal/diag/explain.go` (E0007 text)

**Interfaces:**
- Produces: `types.Err` (a `*Con{Name: "<error>"}` singleton) — `Unify`/`Check` succeed without binding when either side is `Err`; `types.String(Err)` prints `_`; `IsErr(t) bool`.
- Parser: `p.in(what string) func()` pushes a context used by `errorAt`/`expect`: messages become `in <what>: <msg>`.

- [ ] **Step 1: Write the corpus and golden cases**

`testdata/corpus/E0001-form-attr/app/{App,Index}.alder` — App as in `E0304-form-field`; `Index.alder` has `form NewTodo = { title: Text.NonEmpty @unique, }`. `expected.diag`:
```
app/Index.alder:L:C E0001 in a form field: attributes such as `@unique` are only allowed on table fields
```
`testdata/corpus/E0001-inline-brand/app/Index.alder` has `type Model = { id: brand Text }`. `expected.diag`:
```
app/Index.alder:L:C E0001 in a type: `brand` is only allowed as the whole body of a `type` declaration (`type TodoId = brand Text`)
```
(Column numbers: run once, confirm the span is the offending token, then fix the golden.)

`testdata/golden/check_poison/app/{App,Todo,Index}.alder`: `Index` uses `Db.from Todo |> Db.all` and `Todo.title` without `import Todo`, in `load`, with a `Model` that mentions `List Todo`. `check_poison.diag` expects exactly one `E0201 module `Todo` is not imported` (with the `import Todo⏎` fix) and nothing else — no E0011, no E0003, no E0002 cascade.

`testdata/corpus/E0201-dedupe/`: same source shape with three uses of `Todo` → exactly one E0201 at the first use.

`testdata/corpus/E0007-inferred/app/Index.alder`: `rank p = match p { Low -> 0, Normal -> 1, High -> 2 }` with `type Priority = Low | Normal | High` in scope → `E0007 … write `rank : Priority -> Int`` with fix `rank : Priority -> Int⏎`.

Add origin lines to `experiments/corpus_notes.md` (E5 Problems 4, 3, 3, and the M1 final review).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/check/ -run 'TestCorpus|TestCheckGolden'`
Expected: the four new cases FAIL (messages differ / extra diagnostics).

- [ ] **Step 3: Parse-error reasons**

```go
// internal/syntax/parser.go
type Parser struct {
	// …existing fields…
	ctx []string // what is being parsed, innermost last; used by errorAt
}

func (p *Parser) in(what string) func() {
	p.ctx = append(p.ctx, what)
	return func() { p.ctx = p.ctx[:len(p.ctx)-1] }
}

func (p *Parser) errorAt(t Token, msg string) {
	if n := len(p.ctx); n > 0 {
		msg = "in " + p.ctx[n-1] + ": " + msg
	}
	p.diags = append(p.diags, diag.Diagnostic{Code: diag.ParseError, Severity: "error", Span: p.spanTok(t), Message: msg})
}
```

Wrap: `parseFieldList` callers push `"a form field"` / `"a table field"` / `"a record type"` / `"a capability entry"` / `"the session"` / `"page parameters"` / `"message parameters"`; `parseType` pushes `"a type"`; `parseValueDecl` pushes `"the definition of `name`"`; `parseHTML` pushes `"<tag>"`. Two specific messages:
- In `parseFieldList` when `attrs == false` and the token is `At`: `p.errorAt(p.tok, "attributes such as `@" + name + "` are only allowed on table fields")`, then skip the attribute (consume `@name` and an optional UIdent) and continue — do not derail the field list.
- In `parseAType` when the token is the keyword `brand`: `p.errorAt(p.tok, "`brand` is only allowed as the whole body of a `type` declaration (`type TodoId = brand Text`)")`, consume `brand` and the base, and return the base type so parsing continues.
Existing goldens whose messages gain an `in …:` prefix are regenerated (`-update`) after confirming only the prefix changed.

- [ ] **Step 4: Poison type**

```go
// internal/types/types.go
var Err = &Con{Name: "<error>"}
func IsErr(t Type) bool { c, ok := t.(*Con); return ok && c == Err }
// String(Err) returns "_"
```
In `unify`: after `Apply`, `if IsErr(a) || IsErr(b) { return nil }` before the variable cases (no binding). In `check/decls.go` `typeOf`: every `return c.u.Fresh()` on an unresolved or unknown reference becomes `return types.Err` (keep `Fresh()` only for genuinely inferable positions, i.e. none in `typeOf`). In `expr.go` `mismatch`: if either side `IsErr`, return without reporting. In `pattern.go` `newExhaustiveness`: if the subject `IsErr`, mark `closed = true`. In `decls.go` representability checks: skip fields whose type `IsErr`. In `inferVar`/`inferCtor` fallthroughs that currently return `Fresh()` for a missing entry after an error, return `types.Err`.

- [ ] **Step 5: E0201 dedupe**

In `resolver`: `reported map[string]bool` (module name → already reported). `notImported` returns early when `r.reported[x]` is set; otherwise sets it and reports with the fix.

- [ ] **Step 6: E0007 with the inferred type**

Move E0007 emission from `declareValues` to `checkBodies`: for a `ValueDecl` with `Sig == nil`, build `env` with each parameter bound to a fresh var, `t := mc.infer(d.Body, env, effect{})`, then `sig := types.Fun{Params: paramVars, Result: t}` (or `t` when no params), `s := types.String(c.u.Apply(sig))`; if `s` contains `_` or `?`, use `?type` for the whole signature; emit E0007 with message "`rank` has no type signature; write `rank : <s>` on the line before it" and fix `InsertAt(line, 1, name + " : " + s + "\n")`. `declareValues` records nothing for such values (callers see E0002 as before). Update `explain.go` E0007.

- [ ] **Step 7: Run everything**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: PASS; the regenerated goldens differ only by the `in …:` prefixes and the removed cascades.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "m3a: parse-error context, poison type, E0201 dedupe, inferred E0007 signature"
```

---

### Task 2: `check.Result` — types for every node

**Files:**
- Create: `internal/check/result.go`
- Modify: `internal/check/check.go` (`Program` returns `*Result`), `expr.go` (`check`/`infer` record), `pattern.go` (pattern vars), `html.go` (for vars), `decls.go` (value signatures), `cmd/alder/main.go`, `internal/check/check_test.go`
- Create: `internal/check/result_test.go`

**Interfaces:**
- Produces:
  ```go
  package check
  type Result struct {
      Types    map[syntax.Expr]types.Type   // every expression checked or inferred (after Apply)
      Binds    map[any]types.Type           // *syntax.Ident (fn/lambda params), *syntax.LetStmt, *syntax.PVar, *syntax.HtmlFor (its Var), *syntax.PList (Rest)
      Values   map[string]types.Type        // "Module.name" → signature
      Decls    map[string]types.Type        // "Module.Type" → declared type (records, unions, brands, tables, forms)
      Tables   map[string]*types.TableInfo
      Forms    map[string]*types.Record
      Pages    map[string]*PageInfo         // exported copy of pageInfo: Params, Msg, Requires, Msgs, Module
      Ctors    map[string]*CtorInfo         // "Module.Ctor" → {Union, Arg}
      Caps     map[string]map[string]types.Type
      Session  *types.Record
      AppMsg   *types.Union
      Holes    int                          // count of W0002 reported
      U        *types.Unifier               // for Apply on stored types
  }
  func Program(prog *names.Program, bag *diag.Bag) *Result
  ```

- [ ] **Step 1: Write the failing test**

```go
// internal/check/result_test.go
func TestResult_RecordsTypes(t *testing.T) {
	res, bag := runDirResult(t, "../../examples/todos")   // runDir variant returning *Result
	if bag.HasErrors() {
		t.Fatal(render(bag))
	}
	if res.Values["Todo.countOpen"] == nil || res.Decls["Todo.Todo"] == nil || res.Tables["Todo.Todo"] == nil {
		t.Fatalf("declarations missing from Result")
	}
	// every expression in every body has a type, and none is an unbound var
	n := 0
	for e, ty := range res.Types {
		n++
		if _, isVar := res.U.Apply(ty).(*types.Var); isVar {
			t.Errorf("unresolved type for %T at %v", e, e.GetSpan().Start)
		}
	}
	if n < 100 {
		t.Fatalf("too few recorded expressions: %d", n)
	}
	if res.Pages["Todos.Index"] == nil || res.Session == nil || res.AppMsg == nil {
		t.Fatal("page/app info missing")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/check/ -run TestResult` → compile error.

- [ ] **Step 3: Implement**

`Checker` gets `res *Result` created in `Program`; `check(e, expected, …)` records `c.res.Types[e] = expected` at its end (after a successful `Check`; on mismatch record `expected` anyway so lowering never finds a hole); `infer` records the returned type before returning (wrap the switch in a helper so every return path records). Record binds where variables are introduced: `checkBodies` params, lambda params (both check and infer paths), `checkBlock` lets, `checkPattern` `PVar`/`PList.Rest`, `htmlNode` `HtmlFor`. At the end of `Program`, copy the private maps into the exported ones (`PageInfo`/`CtorInfo` are exported mirrors of `pageInfo`/`ctorInfo`; add them and keep the private ones as aliases to avoid churn). `Holes` counts W0002. Update `cmd/alder/main.go` and `runDir` for the new return value (ignore it in `check`).

- [ ] **Step 4: Run** — `go test ./internal/check/` → PASS; whole suite green.

- [ ] **Step 5: Commit** — `git add -A && git commit -m "m3a: check.Result exposes types for every expression and binding"`

---

### Task 3: Runtime core for the pure language

**Files:**
- Create: `rt/core.go`, `rt/core_test.go`, `rt/html/html.go`, `rt/html/html_test.go`, `rt/std/list.go`, `rt/std/maybe.go`, `rt/std/result.go`, `rt/std/text.go`, `rt/std/num.go`, `rt/std/date.go`, `rt/std/html.go`, `rt/std/std_test.go`

**Interfaces:**
- `rt`:
  ```go
  type Maybe[T any] struct{ Ok bool; Val T }
  func Just[T any](v T) Maybe[T]; func Nothing[T any]() Maybe[T]
  type Result[E, T any] struct{ IsOk bool; Val T; Err E }
  func Ok[E, T any](v T) Result[E, T]; func Err[E, T any](e E) Result[E, T]
  type Dict[K comparable, V any] struct{ m map[K]V; keys []K }   // insertion-ordered
  type Date struct{ Y, M, D int }
  type Page struct{ Path string }                                 // M3c fills it; M3a needs the type
  func Concat(parts ...string) string
  func OrElse[T any](m Maybe[T], d T) T
  func Append[T any](a, b []T) []T
  func Unit() struct{}
  ```
- `rt/html`:
  ```go
  type Node struct{ /* opaque */ }
  type Attr struct{ Name, Value string; Bool bool; Omit bool }
  func El(tag string, attrs []Attr, children ...Node) Node
  func Text(s string) Node            // escaped at render
  func Raw(s string) Node             // Html.unsafe only
  func Nodes(ns ...Node) Node         // fragment (List (Html a) spliced)
  func None() Node
  func A(name, value string) Attr; func B(name string, on bool) Attr; func M(name string, v rt.Maybe[string]) Attr
  func Render(n Node) string          // escapes text and attribute values; boolean attrs render bare; Omit skips
  ```
- `rt/std`: one Go function per stdlib signature in `book/09-stdlib.md` for `List`, `Maybe`, `Result`, `Text`, `Int`, `Float`, `Bool`, `Date` (pure parts), `Time` (pure parts: `Format`, `ToDate`, `Epoch`, `Ago`), `Html.none/text`, named `ListMap`, `TextFromInt`, … (module + PascalCase name). Signatures follow the type mapping (e.g. `func ListMap[A, B any](f func(A) B, xs []A) []B`, `func TextMatches(re, s string) bool` with a regexp cache, `func ListSortBy[A any](key func(A) int64, xs []A) []A` stable).

- [ ] **Step 1: Write the tests**

`rt/html/html_test.go`:
```go
func TestHTML_EscapeOnce(t *testing.T) {
	n := El("p", []Attr{A("title", `a"b<c`)}, Text("x < y & z é"), Raw("<b>raw</b>"))
	got := Render(n)
	want := `<p title="a&#34;b&lt;c">x &lt; y &amp; z é<b>raw</b></p>`
	if got != want { t.Fatalf("got %s", got) }
}
func TestHTML_BoolAndMaybeAttrs(t *testing.T) {
	got := Render(El("input", []Attr{B("disabled", true), B("checked", false), M("aria-current", rt.Nothing[string]()), M("class", rt.Just("x"))}))
	want := `<input disabled class="x" />`
	if got != want { t.Fatalf("got %s", got) }
}
func TestHTML_Void(t *testing.T) {
	if got := Render(El("br", nil)); got != "<br />" { t.Fatal(got) }
	if got := Render(El("div", nil)); got != "<div></div>" { t.Fatal(got) }
}
```
`rt/core_test.go`: `Maybe`/`OrElse`, `Dict` insertion order and `Get`, `Concat`, `Date` formatting (`Format("Jan 2, 2006", Date{2026,10,9}) == "Oct 9, 2026"`).
`rt/std/std_test.go`: table tests for `ListFilter/Map/FoldL/SortBy(stable)/Indexed/Range/Unique/GroupBy`, `TextSplit/Join/Replace/Matches/Slice/ToInt`, `IntClamp`, `FloatFormat`, `DateAddDays/DiffDays`, `TimeAgo`.

- [ ] **Step 2: Run to verify failure** — packages do not exist.

- [ ] **Step 3: Implement** the three packages per the interfaces; `Render` uses a `strings.Builder`, `html.EscapeString` for text, and attribute escaping of `"`, `&`, `<`, `>`; void elements per the HTML5 list (`area base br col embed hr img input link meta source track wbr`) render as `<tag … />`.

- [ ] **Step 4: Run** — `go test ./rt/...` PASS; `go vet ./...`.

- [ ] **Step 5: Commit** — `git add -A && git commit -m "m3a: runtime core: containers, html builder, stdlib implementations"`

---

### Task 4: IR and `Lower`

**Files:**
- Create: `internal/ir/ir.go`, `internal/ir/lower.go`, `internal/ir/dump.go`, `internal/ir/ir_test.go`, `testdata/golden/ir_todos.ir` (dump of `examples/todos`), `testdata/golden/ir_basic/app/*.alder` + `.ir`

**Interfaces:**
- Produces (exact shape; later tasks consume it):
  ```go
  package ir
  type Program struct {
      Modules  []*Module              // in names.Program.Order
      Tables   []*Table; Forms []*Form; Pages []*Page; AppMsgs []*Msg; AppUpdate *Value
      Session  *RecordType; Caps []*Capability
  }
  type Module struct{ Name string; Types []*TypeDecl; Values []*Value; Components []*Value }
  type TypeDecl struct{ Module, Name string; Record *RecordType; Union *UnionType }   // brands are erased: no TypeDecl
  type RecordType struct{ Module, Name string; Fields []FieldT }                        // Name=="" ⇒ anonymous
  type FieldT struct{ Name string; Type types.Type }
  type UnionType struct{ Module, Name string; Ctors []CtorT }
  type CtorT struct{ Name string; Payload *RecordType }   // nil = no payload; non-record payload wrapped as {value: T}
  type Value struct{ Module, Name string; Type types.Type; Params []*Param; Body Expr; IsComponent bool }
  type Param struct{ Name string; Type types.Type }
  type Table struct{ Module, Name string; Record *RecordType; IDField string; InsertFields []string }
  type Form struct{ Module, Name string; Record *RecordType }
  type Page struct{ Module, Path string; Params *RecordType; Requires string; Model *RecordType; Msgs []*Msg; View *Value; Load, Update *Value /* nil in M3a */ }
  type Msg struct{ Name string; Params *RecordType; Form *Form }
  type Capability struct{ Module, Name string; Entries []FieldT }

  type Expr interface{ Type() types.Type }
  // Lit{Kind: Int|Float|Bool|Text|Unit; Int int64; Float float64; Bool bool; Text string}
  // Var{Kind: Local|Global|Std; Module, Name string}      Global = "<Module>.<name>", Std = stdlib "List.map"
  // Call{Fn Expr; Args []Expr}  Partial{Fn Expr; Given []Expr; Missing int}  Lambda{Params []*Param; Body Expr}
  // Record{T *RecordType; Fields []FieldInit}  Update{Base Expr; Fields []FieldInit}  Field{Expr Expr; Name string}
  // Ctor{U *UnionType; Name string; Payload Expr /* Record or nil */}
  // Match{Subject Expr; Cases []Case{Tag string; Binds []Bind{Name, Field string; Type}; Body Expr}; Default Expr /* nil when exhaustive by cases */; Kind: Union|Bool|List|Lit}
  //   List matches lower to Kind:List with cases "nil" and "cons" (Binds head/rest); literal matches to Kind:Lit with a Default.
  // If{Cond, Then, Else}  Let{Name string; T types.Type; Value, Body Expr}  Op{Kind string /* "+","++","==","<","&&","??","|>"-free */; L, R Expr; OperandT types.Type}
  // Str{Parts []Expr /* all Text */}
  // HTML: Elem{Tag string; Attrs []HAttr{Name string; Kind: Text|Bool|Maybe|Href|Click|Submit; Value Expr}; Children []Expr}
  //       Comp{Fn *Value; Props *Record; Children []Expr}  HText{Text string}  HExpr{Expr Expr; Kind: Text|Html|List}
  //       HFor{Var string; T types.Type; List Expr; Body Expr}  HIf{Cond, Then, Else Expr}  HMatch{…same as Match with HTML bodies}
  //       Input{Kind: Input|Textarea|Select|Errors; Form *Form; Field, Label string; Value Expr /* may be nil */; BareForm bool}
  //       FormEl{Msg *Msg; Page *Page; Attrs []HAttr; Children []Expr}
  //       Effects are NOT lowered in M3a: a Page's Load/Update stay nil, and a Value whose type is Query/Task is skipped with a note in Dump.
  func Lower(prog *names.Program, res *check.Result) *Program
  func Dump(p *Program) string
  ```
- Lowering rules: brands erased (`types.Brand` → `Base`) everywhere types are copied; `|>` rewritten to `Call`; partial applications become `Partial`; constructor application `Toggle { id: x }` → `Ctor` with a `Record` payload; message constructors are lowered like any union ctor of the page's `Msg` union; `match` arms with record patterns bind each named field; nested one-level patterns are flattened into the case's `Binds` plus, when the inner pattern is refutable (literal/ctor), an inner `Match` in the body; string interpolation → `Str`; `Html.none` → `HExpr{Kind: Html}` of `Var Std "Html.none"`.

- [ ] **Step 1: Write the IR golden harness and the basic golden**

`testdata/golden/ir_basic/app/M.alder`:
```alder
module M

type Shape = Circle Float | Rect { w: Float, h: Float }
type Point = { x: Float, y: Float }

area : Shape -> Float
area s = match s {
  Circle r -> 3.14 * r * r,
  Rect { w: w, h: h } -> w * h,
}

shift : Point -> Point
shift p = { p | x: p.x + 1.0 }

names : List Text -> Text
names xs = xs |> List.map Text.toUpper |> Text.join ", "

label : Maybe Int -> Text
label m = "n = {Text.fromInt (m ?? 0)}"

Card : { title: Text, children: List (Html a) } -> Html a
Card props = <div class="card"><h2>{props.title}</h2>{props.children}</div>
```
`ir_basic.ir` is the `Dump` output; write it by hand from the rules above in the same S-expression style as `syntax.Dump` (e.g. `(value M.area : Shape -> Float (s) (match (var s) (case Circle (bind r value) (* (* 3.14 (var r)) (var r))) (case Rect (bind w w) (bind h h) (* (var w) (var h)))))`), then make `Lower` produce it. Harness `TestIRGolden` mirrors `TestParseGolden` (dirs `testdata/golden/ir_*`, `-update`). `TestIR_Todos` lowers `examples/todos` and compares with `ir_todos.ir` (generated once with `-update`, then reviewed by hand: every component, value and `view` present; `load`/`update` absent).

- [ ] **Step 2: Run to verify failure** — package missing.

- [ ] **Step 3: Implement `ir.go`, `lower.go`, `dump.go`** per the interface block; `Lower` walks `names.Program.Order`, uses `res.Types`/`res.Binds` for every node type (panic with the node position if a type is missing — a checker bug, caught by `TestIR_Todos`).

- [ ] **Step 4: Run** — goldens PASS; `go vet`.

- [ ] **Step 5: Commit** — `git add -A && git commit -m "m3a: typed IR and lowering for the pure language"`

---

### Task 5: Emitter — types, values, expressions, match

**Files:**
- Create: `internal/gen/names.go`, `internal/gen/types.go`, `internal/gen/expr.go`, `internal/gen/gen.go`, `internal/gen/gen_test.go`, `testdata/golden/gen_basic/` (`app/*.alder` + `want/*.go`), unit tests named in Review Focus

**Interfaces:**
- Produces:
  ```go
  package gen
  type Options struct{ RepoPath string /* for the replace directive */; Dev bool }
  func Emit(p *ir.Program, opts Options) (map[string][]byte, error)   // "M.go", "Todos_Index.go", "go.mod"; error only for go/format failures (a bug)
  func GoName(module, name string) string          // "Todos.Index", "view" → "Todos_Index__view"
  func GoType(t types.Type) string                 // mapping table
  ```
- Emission rules (from the spec's table): nominal record → `type Todo__Todo struct { Id string; Title string; … }` (field names PascalCase via `Field(name)`, Go keywords suffixed `_`); union → `type M__Shape struct { Tag M__ShapeTag; Circle M__Shape_Circle; Rect M__Shape_Rect }` with `type M__ShapeTag uint8`, constants `M__Shape_Tag_Circle = 0`, payload structs per ctor (a non-record payload becomes `struct{ Value T }`), constructor funcs `func M__Shape_Circle_(v float64) M__Shape`; value → `func M__area(s M__Shape) float64 { … }`; zero-param value → `var M__greeting = …` unless it references a later var (then a func); `match` → `switch s.Tag { case M__Shape_Tag_Circle: r := s.Circle.Value; return … }`; `Let` → `x := …` inside a func body (expressions that need statements are emitted via an immediately-invoked closure `func() T { … }()`); `Op` per the table (`++` on text → `+`, on lists → `rt.Append`; `??` → `rt.OrElse`; payload-less union comparisons → tag compare); `Str` → `rt.Concat(...)`; brands erased; `Partial` → closure; stdlib `Var Std` → `std.<Module><Name>` (Task 6 holds the table; Task 5 emits the name mechanically as `std.` + module + PascalCase(name)); generics instantiated implicitly by Go.
- Every emitted file begins with `// Code generated by alder; DO NOT EDIT.` and `package app`, and imports only what it uses (`rt`, `rt/html`, `rt/std`, `time`).

- [ ] **Step 1: Write the golden and unit tests**

`testdata/golden/gen_basic/app/M.alder` = the `ir_basic` source; `want/M.go` written by hand following the rules (this is the specification of the output; e.g.
```go
func M__area(s M__Shape) float64 {
	switch s.Tag {
	case M__Shape_Tag_Circle:
		r := s.Circle.Value
		return 3.14 * r * r
	case M__Shape_Tag_Rect:
		w := s.Rect.W
		h := s.Rect.H
		return w * h
	}
	panic("unreachable")
}
```
). `TestGenGolden` lowers, emits, compares byte-for-byte with `want/`, then writes the files plus a `go.mod` into a temp dir and runs `go vet ./...` there with `-mod=mod` and the `replace` pointing at the repo; vet must pass. Unit tests: `TestGen_UnionAnonymousPayload`, `TestGen_KeywordNames` (`type`, `func`, `Tag` as Alder names), `TestGen_StringEscapes` (`"a \{b\} \"c\""`), `TestGen_TypesOnlyModule`, `TestGoType` (table of mappings incl. `List (Maybe Int)` → `[]rt.Maybe[int64]`, anonymous record → `struct{ W float64; H float64 }`).

- [ ] **Step 2: Run to verify failure** — package missing.

- [ ] **Step 3: Implement** `names.go` (mangling, keyword table, PascalCase fields), `types.go` (`GoType`, type declarations), `expr.go` (expression emission with a `needsStmt` analysis: `Let`, `Match` and `If` with non-trivial branches emit as statements inside closures when in expression position), `gen.go` (per-module file assembly, imports, `go/format`, `go.mod` with `module app`, `go 1.27`, `require alder v0.0.0`, `replace alder => <RepoPath>`).

- [ ] **Step 4: Run** — `go test ./internal/gen/` PASS (including the vet step); whole suite green.

- [ ] **Step 5: Commit** — `git add -A && git commit -m "m3a: Go emitter for types, values, expressions and match"`

---

### Task 6: Emitter — HTML, stdlib call table, `alder build --emit`

**Files:**
- Create: `internal/gen/html.go`, `internal/gen/stdcall.go`, `testdata/golden/gen_html/` (`app/*.alder` + `want/*.go`), `cmd/alder/build.go`
- Modify: `cmd/alder/main.go` (subcommand), `internal/gen/gen_test.go`, `book/11-tooling.md`

**Interfaces:**
- `stdcall.go`: `var StdCalls = map[string]string{"List.map": "std.ListMap", …}` — one entry per stdlib function implemented in Task 3; emitting a `Var Std` not in the table is a `gen` internal error (test `TestStdCallsCoverStdlib` parses `stdlib/*.alder` and asserts every pure signature has an entry; effectful ones — `Query`, `Task`, `Db`, `Session`, `Log`, `Random`, `Http`, `Time.now`, `Date.today` — are listed in an `Effectful` set and skipped until M3b).
- HTML emission: `Elem` → `html.El("tag", []html.Attr{…}, children...)` with `Text` attrs → `html.A`, `Bool` → `html.B`, `Maybe` → `html.M`, `Href` → `html.A("href", rt.PageURL(expr))` (`rt.PageURL(Page) string` added to `rt/core.go` returning `p.Path` for M3a), `Click`/`Submit` → `html.A("data-on:click", "@post('"+url+"')")` where `url` is `rt.MsgURL("<Module>", "<Msg>", params)` — add `MsgURL` to `rt` returning `/_m/Module/Msg?k=v…` with URL-encoded params (M3c wires it to the router); `HText` → `html.Text(s)` (NOT pre-escaped); `HExpr` Text → `html.Text(expr)`, Html → `expr`, List → `html.Nodes(expr...)`; `HFor` → `html.Nodes(std.ListMap(func(x T) html.Node { return … }, list)...)`; `HIf`/`HMatch` → Go `if`/`switch` inside a closure returning `html.Node`; `Comp` → `M__Card(struct{…}{Title: …, Children: []html.Node{…}})`; `Input`/`FormEl` → `html.Input(...)`/`html.Form(...)` stubs in `rt/html` that render the plain HTML form controls (name, type from the field type, label) without request state (M3c adds state).
- `alder build --emit [dir]`: check → (no errors, no holes) → lower → emit into `build/<app>/` (deleting stale generated files) → `go vet ./...` in that directory; exit 0/1/2 like `check`; without `--emit` it prints "`alder build` without --emit arrives in M3c" and exits 2.

- [ ] **Step 1: Write the golden and tests**

`testdata/golden/gen_html/app/{Layout,M}.alder`: the `Card` component from `ir_basic`, a `Layout.Page` with `children`, an element with `class`, `disabled={bool}`, `aria-current={maybe}`, an `href={Other {}}` (declare a trivial page `Other`), a `{for}` over a list, an `{if}`, a `{match}`, text containing `<`, `&`, `é`. `want/*.go` by hand. Tests: `TestGen_HTMLTextNotPreEscaped` (the emitted Go contains `html.Text("a < b")`, not `&lt;`), `TestStdCallsCoverStdlib`, `TestBuildEmit_Todos` (runs `alder build --emit examples/todos` in a temp copy and asserts exit 0 and that `build/todos/Todos_Index.go` contains `func Todos_Index__view(`; `load`/`update` absent in M3a).

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement** `html.go`, `stdcall.go`, `rt/html` `Input`/`Form`, `rt` `PageURL`/`MsgURL`, `cmd/alder/build.go`.

- [ ] **Step 4: Run** — `gofmt -l . && go vet ./... && go test ./...`; then `go run ./cmd/alder build --emit examples/todos` → `build/todos/` vets. Add `build/` to `.gitignore`.

- [ ] **Step 5: Book** — `book/11-tooling.md`: "`alder build --emit` generates and vets Go for the pure parts of an app (M3a); effects, pages and `alder run` arrive in M3b–M3d."

- [ ] **Step 6: Commit** — `git add -A && git commit -m "m3a: HTML emission, stdlib call table, alder build --emit"`

---

## Self-review notes

**Spec coverage.** IR node families and `Lower` → Task 4 (effects deferred to M3b as the spec's M3a says); emitter mapping table → Tasks 5–6; `rt/html` builder and escaping → Task 3 (moved up from the spec's M3b because generated HTML must vet in M3a — a ruling recorded in the plan header's architecture note); compiler items (parse-error reasons, poison type, E0201 dedupe, E0007 inferred) → Task 1; `check.Result` interface the spec left to the plan → Task 2; testing (IR dump goldens, emitter goldens vetted, `rt` unit tests, todos emits) → Tasks 3–6; book status line → Task 6.

**Placeholder scan.** Task 5/6 `want/*.go` goldens are "written by hand following the rules" — the rules are fully stated (mapping table, naming, match shape, file header); the implementer writes the files, as with M1's `.diag` goldens.

**Type consistency.** `check.Result` field names (Task 2) are the ones `ir.Lower` reads (Task 4: `res.Types`, `res.Binds`, `res.Values`, `res.Decls`, `res.Tables`, `res.Forms`, `res.Pages`, `res.Ctors`, `res.Session`, `res.AppMsg`, `res.U`). `ir` node names (Task 4) are the ones `gen` switches on (Tasks 5–6). `rt`/`rt/html`/`rt/std` function names (Task 3) are the ones `gen` emits (`rt.Maybe`, `rt.Concat`, `rt.OrElse`, `rt.Append`, `html.El/A/B/M/Text/Raw/Nodes/None`, `std.<Module><Name>`, `rt.PageURL`, `rt.MsgURL`).

**Review Focus coverage.** (1) Task 5 `TestGen_UnionAnonymousPayload`; (2) Task 5 `TestGen_KeywordNames`; (3) Task 5 `TestGen_StringEscapes`; (4) Task 5 `TestGen_TypesOnlyModule`; (5) Task 3 `TestHTML_EscapeOnce` + Task 6 `TestGen_HTMLTextNotPreEscaped`.
