# 13. How the compiler works

This chapter follows one small program through every stage of the
compiler, from the characters in a file to the Go that runs. You don't
need to have written a compiler before. Each section shows the real output
of one stage and names where it came from, so you can produce it yourself.

The program is `testdata/golden/gen_basic/app/M.alder`:

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

Most of the chapter follows `area`, because it is short and uses a union,
a `match` and arithmetic. The other values show up where they illustrate
something `area` doesn't.

## 1. What a compiler is, in Alder's case

A compiler reads source text and writes a program. For Alder the text is
your `.alder` files and the program is a Go package. Go's own compiler then
turns that package into a binary.

The work is split into stages, each a package under `internal/`:

```
lex → parse → resolve names → check types → lower to IR → emit Go → go build
syntax        names           types, check    ir           gen
```

Each stage takes one data structure and produces the next: text becomes
tokens, tokens become a tree, the tree gets its names and types, then it is
rewritten into a simpler tree, and that tree is printed as Go. Splitting
the work this way means each stage only has to think about one thing. The
parser never asks what type something has; the emitter never asks whether
a name exists.

The first four stages can reject your program. Together they are what
`alder check` runs, and this book calls them "the checker". The last three
never reject anything: they run only when the checker found no errors and
no holes (`cmd/alder/build.go` returns before calling `ir.Lower` otherwise).
If lowering or emitting fails, or the generated Go doesn't pass `go vet`,
that is a bug in the compiler, reported as `internal error` with exit code 2,
never a diagnostic about your code.

This is a design decision. An agent fixing a program needs every error to
come from one place, in Alder's terms, with a code and a fix. If Go's
compiler could reject an Alder program, the agent would have to read Go
errors about code it never wrote. Chapter 12 records the reasoning under
[Go as compiler language and target](12-rationale.md#go-as-compiler-language-and-target-untested).

## 2. Lexing

The lexer (`internal/syntax/lexer.go`) reads characters and groups them
into **tokens**: a name, a number, a `{`, a `->`. Each token remembers its
kind, its text, and where it started: line, column and byte offset
(`Token` in `internal/syntax/token.go`).

Here is what the lexer produces for a file holding a `module App` line, a
blank line, and two lines, `session { user: Maybe User.UserId }` and
`update msg session = 1`, which are shaped like lines in
`examples/todos/app/App.alder`. The listing was printed by
`alder dump --tokens` (section 3 shows how to run it):

```
1:1 Keyword "module"
1:8 UIdent "App"
1:11 NL "\n"
2:1 NL "\n"
3:1 Keyword "session"
3:9 LBrace "{"
3:11 LIdent "user"
3:15 Colon ":"
3:17 UIdent "Maybe"
3:23 UIdent "User"
3:27 Dot "."
3:28 UIdent "UserId"
3:35 RBrace "}"
3:36 NL "\n"
4:1 LIdent "update"
4:8 Keyword "msg"
4:12 Keyword "session"
4:20 Equal "="
4:22 Int "1"
4:23 NL "\n"
```

Notice that `session` and `msg` are `Keyword` tokens on lines 3 and 4. On
line 3 `session` starts a declaration. On line 4, `msg` and
`session` are just parameter names, which is exactly how
`examples/todos/app/App.alder` uses them in `update msg session = match msg { … }`.

The **column-1 rule** settles this. Words that start a declaration
(`module`, `import`, `type`, `page`, `msg`, `form`, `table`, `capability`,
`session`, `login`, `test`) are keywords only when they are the first thing
on a line. Anywhere else they are ordinary names. The rule is applied as
the parser takes each token (`advance` in `internal/syntax/parser.go`),
which turns a declaration keyword with `Col > 1` back into a name. Parsing
the same two lines, with `module App` above them, gives:

```
(session (field user (Maybe User.UserId)))
(val update msg session = (int 1))
```

The first is a session declaration, the second a value with two parameters.
This is why you can name a record field `form` or a parameter `session`
without a conflict.

HTML needs a second mode. Inside `<h1>Something went wrong</h1>`, the words
are text, not names. `ScanText` in `lexer.go` reads raw text up to a `{` or to a `<` that starts
a tag (a `<` followed by a letter, `/` or `!`). Any other `<` is ordinary
text. The parser calls it for the children of an element (`parser_html.go`).

Positions matter because every error the compiler reports points at a
**span**: a file plus a start and end position. When you see

```
  area s = match s {
           ^
```

the caret came from a span that starts at the `match` token, which came
from the line and column the lexer recorded. Without positions on tokens, no later
stage could tell you where a problem is.

## 3. Parsing

The parser (`internal/syntax/parser.go`, `parser_expr.go`,
`parser_html.go`) reads tokens and builds an **abstract syntax tree**, or
AST: a tree of Go structs, defined in `internal/syntax/ast.go`, where each
node is one construct of the language. "Abstract" means the punctuation is
gone: there is no node for a comma or a brace, only for what they meant.

`syntax.Dump` prints an AST as nested parentheses, and `alder dump`
prints it for a file. From the repository root:

```
$ alder dump testdata/golden/gen_basic/app/M.alder
$ alder dump --tokens testdata/golden/gen_basic/app/M.alder
$ alder dump --ir testdata/golden/gen_basic
```

The first prints the AST, one module per line. The second prints the
tokens of section 2, and the third prints the IR of section 6; `--ir`
takes the app directory, because lowering needs the whole checked
program. (If `alder` is not installed, `go run ./cmd/alder dump …` does
the same.) Chapter 11 lists the options under
[Commands](11-tooling.md#commands).

Under the hood the command is short, and reading it is a first taste of
the compiler's Go. The AST case of `runDumpTo` in `cmd/alder/dump.go` is, trimmed a
little:

```go
src, err := os.ReadFile(f)
if err != nil {
	fmt.Fprintf(stderr, "alder dump: %v\n", err)
	return 2
}
m, diags := syntax.ParseFile(f, src)
fmt.Fprintln(stdout, syntax.Dump(m))
```

If you have never read Go: `:=` declares a variable and gives it its first
value, a function can return two values (`m, diags`), and `if err != nil`
checks whether reading the file failed. `syntax.ParseFile` is the parser's
entry point; it returns the module's AST and any parse errors, which the
command then prints to stderr. `--tokens` calls `syntax.Tokens`, which
runs the same parser but records each token it takes from the lexer, so
HTML text shows up as the `Text` runs the parser asked for. `--ir` runs the
pipeline `alder check` runs (load, resolve, check) and then `ir.Lower` and
`ir.Dump`. The golden tests do the same thing in bulk: `TestParseGolden`
in `internal/syntax/parser_test.go` parses every
`testdata/golden/parse_*.alder` and compares its dump with the `.dump` file
beside it.

This is the dump of `area`, from `alder dump` on `gen_basic` (wrapped
here for width):

```
(val area : (fn Shape -> Float) s =
  (match (var s)
    (arm (pctor Circle r) (* (* (float 3.14) (var r)) (var r)))
    (arm (pctor Rect (prec (w w) (h h))) (* (var w) (var h)))))
```

Read it from the outside in:

- `(val area : (fn Shape -> Float) s = …)` is a value declaration: its
  name, its signature (a function type from `Shape` to `Float`), its
  parameter `s`, and its body.
- `(match (var s) …)` is the `match`; `(var s)` is a reference to the name
  `s`. The parser doesn't know yet what `s` refers to.
- `(arm (pctor Circle r) …)` is an arm whose pattern is a constructor
  pattern `Circle` with a variable `r` inside.
- `(prec (w w) (h h))` is a record pattern: field `w` bound to `w`, field
  `h` bound to `h`.
- `(* (* (float 3.14) (var r)) (var r))` is `3.14 * r * r`. The nesting
  shows that `*` groups to the left: `(3.14 * r) * r`.

When the parser meets a token that can't continue what it is reading, it
reports **E0001** and says what it was in the middle of. The parser keeps a
stack of contexts (`in` in `parser.go`) and prefixes the innermost one to
the message. From `testdata/corpus/E0001-form-attr/`:

```
app/Index.alder:11:24: error[E0001]: in a form field: attributes such as `@unique` are only allowed on table fields
    title: Text.NonEmpty @unique,
                         ^
```

"in a form field" tells you which rule you broke; without it, the same
token would get a vaguer complaint.

A parse error stops everything. If any file fails to parse, `alder check`
reports the parse errors and nothing else. In
`testdata/golden/check_parse_halts/`, `A.alder` contains a tuple and
`C.alder` contains `y = "no"` where `y : Int`. The only output is:

```
app/A.alder:4:5: error[E0602]: tuples are not Alder; use a record `{ a: 1, b: 2 }`
  x = (1, 2)
      ^
```

This one is E0602, not E0001. It is still a parse error, but the parser
recognised a construct from another language and names the Alder form
instead (the E06xx rows in chapter 11's
[diagnostic codes](11-tooling.md#diagnostic-codes)).

The type error in `C.alder` is real, but it isn't reported. A program that
doesn't parse has no reliable tree, and checking a guessed tree produces
errors that vanish once the parse error is fixed. Fix the parse error, run
the check again, and then you'll see `C.alder`'s problem. Chapter 11
states this rule under [Diagnostics](11-tooling.md#diagnostics).

## 4. Names

The name resolver (`internal/names`) answers, for every name in the
program: which declaration does this mean?

It starts with modules. Every file declares one, and its name must match
its path: `app/Todos/Index.alder` must say `module Todos.Index`, or you get
E0005. That rule lets the compiler find a module from its name alone.

Each `import` is an edge in the **module graph** (`internal/names/graph.go`).
The graph must have no cycles: if `A` imports `B` and `B` imports `A`,
neither can be understood first, and you get E0006. Two module names may
not differ only in case either (`Ab` and `AB`, E0014): each module becomes
a file named after it, and on a case-insensitive file system the two would
be one file. With no cycles, the
modules can be put in an order where every module comes after the modules
it imports (`Program.Order`). Every later stage walks modules in that order.

Then `resolve.go` visits every name. `s` in `area` is the parameter. `List.map`
is the stdlib function `map` in module `List`. `Circle` is the constructor
of `M.Shape`. A name that matches nothing is **unresolved**, and the
resolver reports it.

`testdata/golden/check_poison/app/Index.alder` forgets `import Todo` and
then uses `Todo` four times: as a type in `Model`, in `Db.from Todo`, in
`Todo.title.asc`, and in a form field `priority: Todo.Priority`. Running
`alder check testdata/golden/check_poison` prints exactly one diagnostic:

```
app/Index.alder:5:28: error[E0201]: module `Todo` is not imported
  type Model = { todos: List Todo, titles: List Text }
                             ^^^^
  fix: import Todo
```

Three things keep it to one:

- The resolver reports each missing module once per module. It keeps a
  `reported` set (`notImported` in `internal/names/resolve.go`) and skips
  any module already in it.
- Every unresolved reference gets the **poison type**, `types.Err` in
  `internal/types/types.go`. Poison unifies with every type without
  complaint (`unify` in `unify.go` returns early when either side is
  poison), so `List Todo` failing to resolve can't cause a second error
  where `model.todos` is used. Poison never appears in a message.
- A module whose names failed to resolve has its value bodies skipped
  (pass 4 in `internal/check/check.go`), because checking them would only
  repeat the same problem. Every other module is still checked in full, so
  a missing import never hides type errors elsewhere.

One cause, one diagnostic, one fix. That matters because an agent fixes
the first error it reads; if one missing import produced five errors, it
might try to fix five things.

## 5. Types and checking

The type checker (`internal/check`, using `internal/types`) decides what
type every expression has and reports when two things that must agree
don't.

To the checker, a **type** is a Go value from `internal/types/types.go`: a
`Con` for a built-in like `Int` or `Float`, a `Record` or `Union` for your
declarations, a `Fun` for a function, an `App` for things like `List Text`,
a `Brand` for a brand, a `Param` for a type parameter (the `a` in
`Card`'s `Html a`, which can be any message type), or a `Var` for a type
not yet known.

That last kind is what makes **unification** work. When the checker meets
something whose type it can't know yet, it invents a fresh variable, say
`t1`. Later, when `t1` must equal some other type, the unifier records that
`t1` is that type. "Unify `A` with `B`" means "make `A` and `B` the same
type, filling in variables as needed, or fail".

Follow `m ?? 0` in `label`, where `m : Maybe Int`. The checker handles `??`
in `inferBinOp` (`internal/check/expr.go`): it makes a fresh variable `t1`,
checks the left side against `Maybe t1`, and checks the right side against
`t1`. Unifying `Maybe Int` (the type of `m`) with `Maybe t1` succeeds and
records `t1 = Int`. Checking `0` against `t1`, which is now `Int`,
succeeds. The result of `??` is `t1`, which is `Int`, exactly what
`Text.fromInt` needs. If the right side were a `Text`, the last step would
fail. For `count m = m ?? "none"` with `count : Maybe Int -> Int`:

```
app/M.alder:4:16: error[E0003]: expected Int, got Text
  count m = m ?? "none"
                 ^^^^^^
```

**Brands** show what the checker knows that the program doesn't. Given
`type UserId = brand Text`, the checker refuses a `Text` where a `UserId`
is required, but lets a `UserId` widen to `Text` (`Check` in `unify.go`).
Once the checker has said yes, the distinction has done its job. The next
stage erases brands, and the generated Go uses a plain `string`. The
checker remembers; the runtime forgets.

The checker runs in two directions. **Checking** pushes an expected type
down into an expression: in `area`, the signature says the body must be a
`Float`, so each arm is checked against `Float`. **Inferring** works up from
the leaves: `3.14 * r` has a type because `3.14` and `r` do. The functions
`check` and `infer` in `expr.go` call each other. Since every top-level
value has a signature, there is always an expected type to start from.
That is also why a missing signature is an error. For E0007 the checker
infers the type anyway (`inferSig`) and offers it as the fix, from
`testdata/corpus/E0007-inferred/`:

```
app/Index.alder:15:1: error[E0007]: `rank` has no type signature; write `rank : Priority -> Int` on the line before it
  rank p = match p {
  ^
  fix: rank : Priority -> Int
```

**Exhaustiveness** is checked per `match` (`internal/check/pattern.go`).
The checker lists the constructors of the subject's type, marks each one
an arm covers, and reports any left over. Delete the `Rect` arm from
`gen_basic`'s `area` and `alder check` says:

```
app/M.alder:7:10: error[E0004]: missing cases: `Rect`
  area s = match s {
           ^
  fix: ,
  Rect x -> ?todo,
```

The fix replaces the last arm's trailing comma and line end with `,`, a
new line and the missing arm, so it works whether or not the last arm
ended in a comma. `Rect x` binds the whole payload to `x` because the
checker can't guess which fields you need, and `?todo` is a typed hole.
Applying the fix leaves you with a warning that tells you what to write:
`warning[W0002]: hole ?todo has type Float`.

**HTML** is checked element by element (`internal/check/html.go`). A
lower-case tag must be one of the HTML5 tags listed in `htmlTags`; an
attribute's value is checked against the type its name calls for (a
`Text`, a `Bool` for `disabled`, a page for `href`, a message for
`click`). A tag with a hyphen is a **custom element**, and there the
checker has no list of its own: it asks the manifests
(`internal/check/elements.go`). Before any body is checked,
`loadElements` builds one table of custom elements from two Custom
Elements Manifests. The first is `alder-ui`'s, which the compiler carries
inside itself: `rt/al/custom-elements.json` is embedded in the binary
with the runtime (`rt/embed.go`), and `internal/elements` parses it once
(`elements.Builtin`). The second is the app's
`elements/custom-elements.json`, which `names.Load` reads along with the
`.alder` files (`Program.ElementsManifest`), so the checker itself never
touches the disk. `elements.Merge` puts them in one map by tag, the app's
entry replacing the built-in one for a tag both declare. A manifest is a
large format (classes, members, events, CSS parts); `internal/elements`
decodes only the part the checker uses: each declaration's `tagName`,
its attributes' names and `type.text` (`boolean` marks a boolean
attribute), and its slots' names. `customElement` then looks the tag up
(E0028 when it is missing), checks each attribute against the entry or
the short escape list of global attributes, the popover and invoker ones
included (`elements.Escaped`; `submit`
is refused outright), a literal value against the entry's list of
quoted values when its type is one (`Attr.Values`), and each child's
`slot="…"` against the entry's slots. A plain element's children are
looked at too (`misplacedSlots` in `html.go`): a `slot` there names no
slot of anything, so it is E0028. The one exception is a `<dialog>` the
author writes directly inside an `<al-dialog>`: its children's slots are
the `<al-dialog>`'s, checked against its entry (`Checker.slotted` marks
it), and the renderer then adds nothing to that dialog.

Four of the built-in elements are more than markup. `al-input`,
`al-textarea`, `al-select` and `al-checkbox` inside a `<form submit>` are
Alder's bound inputs: `wrappedInput` hands their `field`, `label` and
`value` to `formControl`, the same function that checks `<Input>`,
`<Textarea>` and `<Select>`, so the rules and the error messages are
shared rather than copied. The checker records each such element in
`Result.Wrapped`, with the component it stands for. The IR stage reads
that record and lowers the element to the same `Input` node an `<Input>`
becomes, with `Wrap` set to the tag; the runtime's `rt/html` then draws
the element around the control and the field's error list, setting
`invalid` when there are errors (chapter 5, "The input elements"). The
decision is made once, by the checker, and every later stage follows it.
A `help` attribute becomes a `<small class="help" id="help-<field>">`
after the label. When the field has errors, `Control.build` also gives
the native control `aria-invalid="true"`, and the error list the `id`
`errors-<field>`. The control's `aria-describedby` names the help, then
the errors list, whichever are there. Those are for screen readers:
`aria-invalid` says the value was refused, and `aria-describedby` names
the elements whose text describes the control, so the hint and the
messages are read with it. The errors `id` appears only with errors,
because a page of per-row forms repeats field names and an `id` must be
unique in a page. The `formCtx` also records which fields a wrapped
input shows, so an `<Errors field>` for one of them is E0028.

Two more built-in elements get something from the renderer, recorded the
same way in `Result.Rendered` by `rendered` (also in
`internal/check/elements.go`). A custom element is not a button: the
browser gives it no keyboard focus, Enter does nothing on it, and a form
does not submit through it. So an `<al-button>` with `click`, `type` or
`href` is recorded as `"button"` or `"a"`, and the IR stage
(`completeBuiltin` in `internal/ir/lower_html.go`) rewrites it into two
nested elements: the `<al-button>` with its styling attributes, and
inside it a real `<button>` (with the `click`, a `type`, and a copy of
`disabled`) or `<a>` (with the `href`; a disabled one gets
`aria-disabled="true"` and no `href`, under an `HIf` when `disabled` is
computed) holding the children. Before recording it, `alButton` checks
that the children cannot put a control inside that control: it walks
them through `if`, `for` and `match` and into nested elements, and
reports E0028 for an interactive tag or an element with `click`, and for
an `Html`-typed expression or a component (their markup is not known
until run time; the expression's type is in `Result.Types`, since the
children are checked first). It also refuses `href` with `click`. Everything after the IR
sees two ordinary elements. The one place that knows about the pair is
the renderer's click button (`clickButton` in `rt/html/html.go`; chapter 6,
"With and without JavaScript"):
its no-JavaScript form must wrap the whole `<al-button>`, not the
`<button>` alone, or the stylesheet's `al-button > button` would no
longer match. An `<al-alert>` without `role` whose `variant` is written
as a literal is recorded as `"status"` or `"alert"`, and the IR stage
adds that `role`, so a screen reader announces the message when it
appears. Two more elements are completed the same way (`"dialog"` and
`"popover"` in `Result.Rendered`, `completeDialog` in the IR's
`lower_html.go`): an `<al-dialog>` gets the native `<dialog>` the
library's CSS and the invoker commands act on (its `id` moves to it, the
`heading` becomes an `<h2 slot="header">` the dialog's `aria-labelledby`
names, a `dismissible` one gets a close form (none for a literal
`dismissible={False}`, an `if` around it for a computed one), and
`data-preserve-attr="open"` tells Datastar's morph to keep the `open` the
browser wrote; one whose direct child is the author's own `<dialog>` gets
nothing), and an `<al-toasts>` without `popover` gets
`popover="manual"`. Each rule is checked only for the built-in entry: an app that
redefines `al-button` in its own manifest gets its element as written.

The checker's output is a `check.Result` (`internal/check/result.go`). It
records the type of every expression (`Types`, keyed by AST node), the type
of every variable at the place it is introduced (`Binds`), every
signature, every declared type, constructor, table, form and page. The
next stage needs all of it: to write Go, you must know that `r` is a
`float64` and `m` an `rt.Maybe[int64]`, and the only stage that worked
that out is the checker. `Result` hands it over so nothing has to be
worked out twice.

## 6. The IR

The emitter could print Go straight from the AST and `check.Result`. It
doesn't, because the AST is shaped for humans: it has pipes, string
interpolation, nested patterns, and blocks, each of which would need its
own case in Go generation. **Lowering** (`internal/ir/lower*.go`) rewrites
the checked AST into a second tree, the **intermediate representation** or
IR, defined in `internal/ir/ir.go`.

The IR is smaller than the AST. Its contract, at the top of `ir.go`, says
what a consumer can assume:

- **Brands are erased.** Every type is the base type.
- **Every name is resolved.** A `Var` says whether it is a local, a
  top-level value of the app, or a stdlib value.
- **There is no `|>`, no block, no pattern.** Pipes are calls, blocks are
  `Let`s, and matches are decision trees.

One more property holds by construction: every expression node embeds
`Typed` (`ir.go`), so every node has a type, copied from `check.Result`.
The dump prints types only where something is bound or where the type
decides the Go (`: Float` on a `bind` or `let`, `: Maybe Int` on `??`), to
keep it readable.

Here is `area` as AST and as IR. The IR is from
`testdata/golden/ir_basic.ir`, written by `ir.Dump`:

```
(val area : (fn Shape -> Float) s =
  (match (var s)
    (arm (pctor Circle r) (* (* (float 3.14) (var r)) (var r)))
    (arm (pctor Rect (prec (w w) (h h))) (* (var w) (var h)))))
```

```
(value M.area : Shape -> Float (s)
  (match union (var s)
    (case Circle (bind _m1 value : Float)
      (let r : Float (var _m1) (* (* 3.14 (var r)) (var r))))
    (case Rect (bind _m2 w : Float) (bind _m3 h : Float)
      (let w : Float (var _m2) (let h : Float (var _m3) (* (var w) (var h)))))))
```

The differences, one at a time:

- `area` became `M.area`: the name is now fully qualified.
- `match` became `match union`: the IR knows the subject is a union, so the
  emitter will switch on its tag.
- Patterns are gone. `(pctor Circle r)` became `(case Circle …)`, a test of
  one tag, plus `(bind _m1 value : Float)`, which binds the payload to a
  fresh temporary `_m1`, plus `(let r : Float (var _m1) …)`, which gives it
  your name. `Rect`'s record pattern became one bind per field.
- Every binding shows its type (`: Float`), from `check.Result.Binds`.
- `(float 3.14)` became `3.14`: a literal's kind is now in its type.

Why the temporaries, when `Circle r` could just bind `r`? Because an arm
with nested patterns can bind a name, then fail a later test and fall
through to the next arm, and that next arm must still see whatever the
name meant outside the `match`. The match compiler
(`internal/ir/lower_match.go`) always binds parts to fresh temps `_mN`
first, and introduces your names with `Let` only after every test of the
arm has passed. No later arm ever sees a name a failed arm bound.

In the AST, `names` still has its two pipes, `(|> (|> (var xs) …) …)`.
In the IR they are gone, from `ir_basic.ir`:

```
(value M.names : List Text -> Text (xs)
  (call (std Text.join) ", " (call (std List.map) (std Text.toUpper) (var xs))))
```

`xs |> f |> g` became `g (f xs)`, and `(std …)` marks each callee as a
stdlib function. Interpolation in `label` became `str`, a concatenation of
parts, and `??` records the `Maybe Int` it works on:

```
(value M.label : Maybe Int -> Text (m)
  (str "n = " (call (std Text.fromInt) (?? : Maybe Int (var m) 0))))
```

Brand erasure is easiest to see in a small module with
`type UserId = brand Text` and `greet : UserId -> Text`,
`greet id = "hello {id}"`. Its IR dump (from `alder dump --ir`) shows the signature with the brand
already gone:

```
(value F.greet : Text -> Text (id) (str "hello " (var id)))
```

## 7. Emitting Go

The emitter (`internal/gen`) walks the IR and prints Go, formatted with
`go/format`. Its first job is choosing a Go type for each Alder type
(`internal/gen/types.go`):

| Alder | Go |
|---|---|
| `Int` | `int64` |
| `Float` | `float64` |
| `Bool` | `bool` |
| `Text` | `string` |
| `Unit` | `struct{}` |
| `Date` | `rt.Date` |
| `Time` | `time.Time` |
| a brand | its base type |
| `List a` | `[]T` |
| `Maybe a` | `rt.Maybe[T]` |
| `Result e a` | `rt.Result[E, T]` |
| `Dict k v` | `rt.Dict[K, V]` |
| `Html a` | `html.Node` |
| `Page` | `rt.Page` |
| a record | a `struct` (named for a declared record, anonymous otherwise) |
| a union | a `struct` with a `Tag` and one pointer field per payload |

Every snippet below is from `testdata/golden/gen_basic/want/M_alder.go`, which is
byte-for-byte what `alder build --emit` writes for `gen_basic`.

**`Shape`.** Go has no sum types: there is no way to say "a value that is
either a `Circle` with a float or a `Rect` with two". So a union becomes a
struct holding a tag, which says which constructor built it, and one field
per constructor's payload. A Go `struct` is a record type with named
fields, `type … uint8` declares a small number type, and `const ( … )`
gives names to fixed values:

```go
type M__Shape struct {
	Tag    tag__M__Shape
	Circle *payload__M__Shape__Circle
	Rect   *payload__M__Shape__Rect
}

type tag__M__Shape uint8

const (
	is__M__Shape__Circle tag__M__Shape = 0
	is__M__Shape__Rect   tag__M__Shape = 1
)

type payload__M__Shape__Circle struct {
	Value float64
}

type payload__M__Shape__Rect struct {
	W float64
	H float64
}

func M__Shape__Circle__(v float64) M__Shape {
	return M__Shape{Tag: is__M__Shape__Circle, Circle: &payload__M__Shape__Circle{Value: v}}
}
```

The `*` in `*payload__M__Shape__Circle` makes the field a *pointer*: it
holds the address of a payload struct stored elsewhere, or `nil` (no
address) when another constructor built the value. Only the field matching
`Tag` is set. Pointers are what make a recursive union possible. In
`type Tree a = Leaf | Node { left: Tree a, value: a, right: Tree a }`, a
`Node` holds two more trees; if each tree held its `Node` payload directly,
a tree would have to contain itself and Go would reject the type as
infinitely large. A pointer has a fixed size, so every payload is held by
pointer, recursive or not, and the emitter never has to work out which
unions recurse.
`Circle`'s `Float` payload is wrapped in a struct with one field, `Value`,
as the IR said: `ir_basic.ir` declares the union as
`(union M.Shape (Circle { value: Float }) (Rect { w: Float, h: Float }))`.

The constructor `Circle` becomes a function, `func M__Shape__Circle__(v
float64) M__Shape { … }`: it takes a `float64` named `v` and returns a
`M__Shape`. Inside, `M__Shape{Tag: …, Circle: …}` builds a struct value,
naming the fields it sets, and `&payload__M__Shape__Circle{Value: v}`
builds a payload struct and takes its address with `&`. Note the
underscores. Every part of a generated name is joined by a double `__`:
the module, the union, the constructor. A trailing `__` marks the
constructor (`M__Shape__Circle__`), and names that are not one of your
declarations start with a lowercase word saying what they are: `tag__` for
the tag type, `is__` for a tag constant, `payload__` for a payload struct.
Section 9 lists the whole scheme and why no two names can clash.

**`area`.** The IR's `match union` becomes a `switch` on the tag:

```go
func M__area(s M__Shape) float64 {
	switch s.Tag {
	case is__M__Shape__Circle:
		r := s.Circle.Value
		return 3.14 * r * r
	case is__M__Shape__Rect:
		w := s.Rect.W
		h := s.Rect.H
		return w * h
	}
	panic("unreachable")
}
```

`switch s.Tag { case …: }` compares `s.Tag` with each `case` in turn and
runs the first that matches. `r := s.Circle.Value` declares `r` and sets
it to the payload's field; Go follows the pointer in `s.Circle` for you.

The temp `_m1` from the IR is gone: when a temp's only use is to be renamed
to your name, the emitter binds your name directly (the comment at the top
of `internal/gen/match.go`). `panic("unreachable")` stops the program with
that message if it ever runs. It never does, because the checker proved
the match exhaustive, but Go requires a function that returns a value to
end in a `return` or a `panic`.

**`shift`.** A record update builds a new struct, copying every field
except the one you replaced. `p.X` reads field `X` of `p`:

```go
func M__shift(p M__Point) M__Point {
	return M__Point{X: p.X + 1.0, Y: p.Y}
}
```

**`names`.** Stdlib calls become calls into `rt/std`, one Go function per
Alder function, chosen from the table in `internal/gen/stdcall.go`.
`[]string` is a Go slice, a list of strings, and `std.TextToUpper` is
passed as a value, without calling it, for `ListMap` to call on each
element:

```go
func M__names(xs []string) string {
	return std.TextJoin(", ", std.ListMap(std.TextToUpper, xs))
}
```

**`label`.** `Maybe Int` is `rt.Maybe[int64]`, `??` is `rt.OrElse`, and
interpolation is `rt.Concat`. `rt.Maybe` is defined in `rt/core.go`:

```go
// Maybe is Alder's `Maybe a`. The zero value is Nothing.
type Maybe[T any] struct {
	Ok  bool
	Val T
}
```

`Maybe[T any]` is a *generic* type: `T` stands for any type, and
`rt.Maybe[int64]` is the version where `T` is `int64`, just as Alder's
`Maybe a` becomes `Maybe Int`. `Ok` is `true` for `Just` and `false` for
`Nothing`, and `Val` holds the value when there is one. `rt.OrElse(m, 0)`
returns `m.Val` when `m.Ok` is true and `0` otherwise, which is exactly
`m ?? 0`:

```go
func M__label(m rt.Maybe[int64]) string {
	return rt.Concat("n = ", std.TextFromInt(rt.OrElse(m, 0)))
}
```

Because `Maybe` holds its value directly, a record that refers to itself
through `Maybe` would be a struct that contains itself, which Go rejects
("invalid recursive type"): a `Node` would hold a `Maybe Node`, which holds
a `Node`, and so on forever. So a record field whose type leads back to its
own record through `Maybe` (or `Result`, or an anonymous record, possibly by
way of other records) is stored behind a pointer, written `*` in Go. From
`testdata/golden/gen_vet/o2-maybe-recursion`:

```alder
type Node = { v: Int, next: Maybe Node }
```

```go
type M__Node struct {
	V    int64
	Next *rt.Maybe[M__Node]
}
```

Reading the field follows the pointer, `(*n.Next)`, and building a record
stores a fresh pointer, `Next: new(rt.Nothing[M__Node]())`. Alder values
never change, so the pointer is never nil and two records may share it. A
`List Node` field needs no pointer: a Go slice already holds its elements
elsewhere, and so does a union's payload.

**Partial application.** `gen_basic` has none, because the pipes in
`names` became full calls. Write `adder 1` where a function is expected,
as in this module:

```alder
adder : Int -> Int -> Int
adder a b = a + b

addAll : List Int -> List Int
addAll xs = xs |> List.map (adder 1)
```

and the emitter writes a closure that waits for the missing argument
(from `build/partial/P_alder.go`, after `alder build --emit partial`, where
`partial/app/P.alder` holds `module P` and the lines above):

```go
func P__addAll(xs []int64) []int64 {
	return std.ListMap(func(_a1 int64) int64 {
		return P__adder(1, _a1)
	}, xs)
}
```

`func(_a1 int64) int64 { … }` is a function literal: a function with no
name, written where it is used. It takes the one argument `adder 1` is
still missing, under a fresh name the emitter picks, `_a1`, that nothing
else in scope uses, and calls `P__adder` with the `1`
it already has. A function literal that uses values from around it is
called a closure.

**Equality and ordering.** Go's `==` doesn't work on every struct, and
wouldn't mean the right thing if it did (two `Dict`s with the same entries
inserted in different orders are equal in Alder). So `==` on records,
unions, `Maybe`, `Dict`, `Time` and `Html` calls `rt.Eq`, which compares
structurally: a `Dict` ignoring insertion order, a `Time` as an instant.
A runtime type can bring an equality of its own by implementing
`rt.Equaler` (`AlderEqual(other any) bool`), which `rt.Eq` asks first:
`html.Node` does, comparing the markup the two nodes render to, so
`<p>{Html.text "a"}{Html.text "b"}</p> == <p>ab</p>` is `True` although
one tree has two text nodes and the other one.
`<` on a `Date` or `Time` calls `rt.DateCmp` or `rt.TimeCmp`. From
`build/facts/F_alder.go`:

```go
func F__same(a F__Point, b F__Point) bool {
	return rt.Eq(a, b)
}

func F__before(a rt.Date, b rt.Date) bool {
	return rt.DateCmp(a, b) < 0
}

func F__greet(id string) string {
	return rt.Concat("hello ", id)
}
```

`F__greet` takes a `string`: that is the erased `UserId` brand from
section 6.

**`Card`.** A component becomes a function from its props to an
`html.Node`, and markup becomes calls to the `rt/html` builder. The props
type is an anonymous `struct { … }` written in place, because `Card`'s
props record has no name. `[]html.Attr{ … }` is a slice literal, `nil`
means "no attributes", and `props.Children...` passes each element of the
slice as a separate argument:

```go
func M__Card(props struct {
	Title    string
	Children []html.Node
}) html.Node {
	return html.El("div", []html.Attr{html.A("class", "card")},
		html.El("h2", nil, html.Text(props.Title)),
		html.Nodes(props.Children...),
	)
}
```

Look at `html.Text(props.Title)`. The title is passed as it is, not
escaped. Escaping happens once, later, when the tree is rendered. In
`testdata/golden/gen_html/`, literal text `Tom & Jerry é` and the
expression `{"a < b"}` are emitted unescaped too:

```go
			html.El("p", []html.Attr{html.A("class", "intro")}, html.Text("Tom & Jerry é")),
			html.El("p", nil, html.Text("a < b")),
```

If the emitter escaped and then the runtime escaped again, `&` would come
out as `&amp;amp;`. One place does it, so it happens exactly once.

The body of a `<script>` or `<style>` is the one text that is never
escaped: `&&` escaped to `&amp;&amp;` would break the JavaScript. The parser
reads it as raw text (`(rawtext "…")` in `alder dump`), byte for byte up to
the closing tag, and the emitter writes it as `html.RawText("…")`, always
with a Go string literal. The renderer writes a `RawText` unescaped only
as a direct child of `<script>` or `<style>`, and only if it holds no
`</script` or `</style`; it drops every other child of those two elements
(a `Text`, an `Html.unsafe`, an element). So no Alder value can be spliced
into script or style text, even if some future path put one there.
`testdata/golden/render_html/` pins the rendered bytes.

## 8. The runtime

The generated code is short because most of the work is in the runtime: Go
packages the generated code imports. There are seven.

- **`rt`** (`rt/core.go`) holds the types the mapping table names: `Maybe`,
  `Result`, `Dict`, `Date`, `Page`. It also holds the helpers the emitter
  calls: `Concat`, `OrElse`, `Append`, `Eq`, `DateCmp`, `TimeCmp`, and
  `Int` and `Float`, which return their argument unchanged: a literal goes
  through one when Go would otherwise compute an operation on constants
  while compiling, and reject `1 / 0` or an `Int` that overflows, which
  Alder defines to happen when the program runs (chapter 4). A `Dict` takes
  keys of any type: it files each under `Key(k)`, a text that is the same
  for two keys exactly when they are `==`. `rt/nav.go` holds `Nav`, the
  result of an `update`: `rt.Rerender()`, `rt.Redirect(page)` or
  `rt.Reject(text)`.
- **`rt/html`** (`rt/html/html.go`) builds HTML trees (`El`, `Text`,
  `Raw`, `A`, `Nodes`) and renders them. `Render` is where escaping
  happens: a text node goes through `html.EscapeString`, and so does every
  attribute value, and a carriage return becomes `&#13;`. A `Raw` node is
  written as is.
- **`rt/std`** (`rt/std/*.go`) has one Go function per stdlib function:
  `std.ListMap` is `List.map`, `std.TextToUpper` is `Text.toUpper`. A call
  lets Go infer a generic one's type arguments; a stdlib function used as
  a value instead (`let r = List.reverse`) is written with them,
  `std.ListReverse[int64]`, worked out from its type at that use
  (`stdInstance` in `internal/gen/stdcall.go`). The effectful ones, in `rt/std/effects.go`, don't do anything: `std.TimeNow`
  returns a one-step effect tree that reads the clock when it is run.
- **`rt/eff`** (`rt/eff/eff.go`, `run.go`) is the effect runtime: the tree
  that a `Query` or `Task` is, and `Run`, which performs it. Chapter 7's
  "How effects run" introduces it.
- **`rt/db`** (`rt/db/table.go`, `store.go`, `sqlite.go`, `encode.go`)
  is the database: a descriptor for each table, `Select` values, the
  `Store` interface that the effect steps call, and its SQLite
  implementation.
- **`rt/web`** (`rt/web/router.go`, `request.go`, `session.go`,
  `decode.go`) is the web server: the router, the `Request` a handler gets
  and the `Response` it returns, the signed session cookie, and the
  helpers the generated form decoders call (`web.Text`, `web.Date`,
  `web.NonEmpty`, …). Chapter 6, "How a request runs", describes it from
  the outside.
- **`rt/datastar`** (`rt/datastar/datastar.go`) serves the Datastar
  script, which is built into the binary, and writes the server-sent
  events Alder answers with: `PatchBody`, which replaces the `<body>`;
  `PatchTitle`, which replaces the `<title>`; and `Navigate`, which either
  sends the browser to another URL or records a URL in its history. It
  also holds the *boost* (`BoostTag`), the attributes that turn a click
  on a link into a Datastar request.

The generated program is small because the runtime is the library. All of
`examples/todos` emits about 1,440 lines of Go across ten files, plus the
81-line `cmd/main.go`, and those lines mostly call into `rt`. The largest
file, `routes__alder.go` (560 lines), is the request handlers of the last
part of this section.

The rest of this section follows one `load` from source to SQL, then one
`update` arm through a transaction. The program is
`testdata/golden/ir_effects/app/`, whose page module `Index` uses the `Todo`
table (declared in `app/Todo.alder`, as every table is: chapter 8) and has a
page `"/pair/{a}/{b}"`. (`testdata/golden/gen_effects/app/`
holds the same three files, and `gen_effects/want/` the Go they become.)

### The source

```alder fragment
load : Params -> Session -> Query Model
load params session = {
  let todos <-
    Db.from Todo
      |> Db.where (Todo.owner.eq session.user)
      |> Db.orderBy Todo.created.desc
      |> Db.limit 10
      |> Db.all
  let pair <- Query.all [Db.get Todo params.a, Db.get Todo params.b]
  let now <- Time.now
  let q = Db.from Todo |> Db.where (Db.or [Todo.title.contains "x", Todo.done.eq True]) |> Db.offset 5
  let count <- Query.map (\n -> n + 1) (Db.count q)
  let first <- Db.first Todo.pending
  Query.succeed { todos: todos, pair: pair, now: now, count: count, first: first }
}
```

Five `let x <-` lines run effects (six steps in all, since `Query.all`
runs two `Db.get`s), one `let q =` builds a `Select` without running it,
and the last line wraps the result.
`Todo.pending` is a `Select Todo` declared in `Todo.alder`.

### The IR

`alder dump --ir testdata/golden/ir_effects` prints this for `load`
(`testdata/golden/ir_effects.ir` is the same text):

```
(value Index.load : Params -> Session -> Query Model (params session)
  (bind query todos : List Todo
    (db-select Todo.Todo
      (where (cond eq (col Todo.Todo owner) (field user (var session))))
      (order (ord (col Todo.Todo created) desc))
      (limit 10)
      all)
    (bind query pair : List (Maybe Todo)
      (all query
        (list
          (db-get Todo.Todo (field a (var params)))
          (db-get Todo.Todo (field b (var params)))))
      (bind query now : Time
        (std Time.now)
        (let q : Select Todo
          (db-select Todo.Todo
            (where
              (cond or
                (cond contains (col Todo.Todo title) "x")
                (cond eq (col Todo.Todo done) True)))
            (offset 5)
            select)
          (bind query count : Int
            (map query (lambda (n : Int) (+ (var n) 1)) (db-run count (var q)))
            (bind query first : Maybe Todo
              (db-run first (global Todo.pending))
              (pure query
                (record Model
                  (todos (var todos))
                  (pair (var pair))
                  (now (var now))
                  (count (var count))
                  (first (var first)))))))))))
```

The block is gone. Each `let x <- e` became a `bind` whose body is the
rest of the block, so the five binds nest one inside the next, and the
plain `let q =` became an ordinary `let` between two of them. The nodes
are defined in `internal/ir/ir.go` and produced by
`internal/ir/lower_eff.go`:

- `bind query todos : List Todo` is `EffBind`: run the effect, call the
  result `todos`, continue. `query` is its kind; in an `update` it says
  `task`.
- `db-select Todo.Todo … all` is `DbSelect`. The whole pipeline,
  `Db.from` through `Db.all`, is folded into one node that knows its
  table, its conditions, its orders, its limit, and what to do with it:
  `all`, `first`, `count`, or `select` for a pipeline that is only a value,
  like `q`.
- `cond eq (col Todo.Todo owner) …` is a `Cond` on a `ColumnRef`: the
  column `owner` of table `Todo.Todo`, compared with `session.user`.
- `all query (list …)` is `EffAll` (`Query.all`), `map query` is `EffMap`,
  and `pure query` is `EffPure` (`Query.succeed`).
- `db-run count (var q)` is `DbRun`: running a `Select` that is not
  written in place but held in a variable or, for `Db.first
  Todo.pending`, declared elsewhere (`global Todo.pending`).

### The Go

`alder build --emit` writes this (`gen_effects/want/Index_alder.go`):

```go
func Index__load(params Index__Params, session Index__Session) eff.Query[Index__Model] {
	return eff.Q(eff.Bind(eff.Call("Db.all Todo", false, func(_e1 *eff.Env) ([]Todo__Todo, error) {
		return db.All(_e1.Ctx, _e1.Store.(db.Store), db.TxOf(_e1), db.Limit(10, db.OrderBy(db.Desc("created"), db.Where(db.CEq("owner", session.User), db.From(table__Todo__Todo)))))
	}), func(todos []Todo__Todo) eff.Task[Index__Model] {
		return eff.Bind(eff.All([]eff.Task[rt.Maybe[Todo__Todo]]{eff.Call("Db.get Todo", false, func(_e2 *eff.Env) (rt.Maybe[Todo__Todo], error) {
			return db.GetRow(_e2.Ctx, _e2.Store.(db.Store), db.TxOf(_e2), table__Todo__Todo, params.A)
		}), eff.Call("Db.get Todo", false, func(_e3 *eff.Env) (rt.Maybe[Todo__Todo], error) {
			return db.GetRow(_e3.Ctx, _e3.Store.(db.Store), db.TxOf(_e3), table__Todo__Todo, params.B)
		})}), func(pair []rt.Maybe[Todo__Todo]) eff.Task[Index__Model] {
			return eff.Bind(eff.T(std.TimeNow()), func(now time.Time) eff.Task[Index__Model] {
				q := db.Offset(5, db.Where(db.COr([]db.Cond{db.CContains("title", "x"), db.CEq("done", db.EncodeBool(true))}), db.From(table__Todo__Todo)))
				return eff.Bind(eff.Map(func(n int64) int64 {
					return n + 1
				}, eff.Call("Db.count Todo", false, func(_e4 *eff.Env) (int64, error) {
					return db.CountRows(_e4.Ctx, _e4.Store.(db.Store), db.TxOf(_e4), q)
				})), func(count int64) eff.Task[Index__Model] {
					return eff.Bind(eff.Call("Db.first Todo", false, func(_e5 *eff.Env) (rt.Maybe[Todo__Todo], error) {
						return db.First(_e5.Ctx, _e5.Store.(db.Store), db.TxOf(_e5), Todo__pending)
					}), func(first rt.Maybe[Todo__Todo]) eff.Task[Index__Model] {
						return eff.Pure(Index__Model{Todos: todos, Pair: pair, Now: now, Count: count, First: first})
					})
				})
			})
		})
	}))
}
```

It is long, but it is the IR printed with Go spelling, one node at a time
(the table at the top of `internal/gen/effects.go` lists every rule):

- Each `bind` is `eff.Bind(first, func(x T) eff.Task[…] { return rest })`.
  The rest of the block is a Go function literal that receives the bound
  value, so the nesting is the same as in the IR.
- Each database node is `eff.Call(name, write, fn)`: a step named
  `"Db.all Todo"`, marked `false` because it doesn't write, whose `fn`
  receives the `Env` and calls a typed helper in `rt/db` (`db.All`,
  `db.GetRow`, `db.CountRows`, `db.First`) with the context, the store
  and the transaction from it. `db.TxOf(_e1)` is the running `Task`'s
  transaction, which the first such step of a `Task` begins, or `nil` in
  a `Query`.
- A `Select` is data. `db.Limit(10, db.OrderBy(db.Desc("created"),
  db.Where(db.CEq("owner", session.User), db.From(table__Todo__Todo))))`
  reads inside out: start from every row of `Todo`, keep `owner = …`,
  order by `created` descending, keep 10. `q :=` is a plain Go variable
  for the same reason: building a `Select` runs nothing.
- Values compared with a column are encoded the way the column stores
  them: `Todo.done.eq True` is `db.CEq("done", db.EncodeBool(true))`,
  because a `Bool` is stored as `1` (chapter 8).
- `eff.Bind`, `eff.Map` and `eff.All` work on `Task`s, so the emitter
  wraps the whole tree once in `eff.Q(…)` to make it a `Query` again, and
  turns the `Query` from `std.TimeNow()` into a `Task` with `eff.T(…)`.
  Neither changes the tree: they only change its Go type.

`table__Todo__Todo` is the table's **descriptor**, a package variable
the emitter writes into `Todo_alder.go`. It is what turns an Alder record
into a row and back:

```go
var table__Todo__Todo = &db.Table[Todo__Todo]{
	Name: "Todo.Todo",
	Columns: []db.Column{
		{Name: "id", Type: db.CText},
		{Name: "title", Type: db.CText},
		{Name: "done", Type: db.CBool},
		{Name: "due", Type: db.CDate, Nullable: true},
		{Name: "owner", Type: db.CText},
		{Name: "created", Type: db.CTime},
	},
	ID:      "id",
	Created: "created",
```

It goes on with `Encode` and `Decode`, which convert each field with the
helpers in `rt/db/encode.go` (`db.EncodeBool`, `db.DecodeDate`, …), and
`ReadID`. When the app starts, the list of every descriptor is passed to
`Ensure`, which creates the SQL tables (chapter 8, "Without migrations"),
or, for an app with migrations, to `Migrate` (below, "Migrations"). The
list is `Tables`, in a file of its own, `app__alder.go`:

```go
// Tables describes every table of the app, in module order then declaration order, for db.Ensure.
var Tables = []db.TableDesc{table__Todo__Todo.Desc()}

// Capabilities are the keys of eff.Env.Caps the app's capability calls look up: Caps.Registry
// fills them.
var Capabilities = []string{"Todo.Stats", "Index.Mailer"}
```

`Capabilities` names every capability the app calls, so `Serve` (in the
same file) can check at startup that each has an implementation and
refuse to start otherwise (chapter 7, "Implementing a capability"). A call
to one that isn't wired, which only a test that skips `Serve` can reach,
stops with `alder: capability "Index.Mailer" is not wired`. The file's name can't clash with a module's: only a module named `app_` would
produce `app__alder.go`, and a module name segment may not end in `_`
(E0001, section 9).

### What `Run` does

Calling `Index__load(params, session)` performs nothing. It returns the
tree. The caller (a request handler, as the last part of this section
shows, or a test) hands it to `eff.RunQuery(env, …)`, which calls `eff.Run` with kind `KQuery`. Here is
what happens, following `interpret` in `rt/eff/run.go`. `interpret` is a
loop holding the current node and a stack of continuations: the "rest of
the block" functions it has yet to call.

1. A `Query` gets no transaction: `env.Tx` is `nil`, and `db.TxOf` gives
   `nil` too. `Run` also puts `env` itself in `env.Ctx` for as long as it
   runs, so a capability can find it (chapter 7).
2. The node is a `bind`. Its continuation (the function that takes
   `todos`) goes on the stack, and the loop moves to its first part.
3. The node is the `call` named `Db.all Todo`. It isn't a write, so it may
   run in a `Query`. `Run` checks that the request hasn't been cancelled,
   logs `effect: Db.all Todo`, appends the name to `env.Trace`, and calls
   `fn`.
4. `fn` calls `db.All`, which renders the `Select` (`Select.SQL` in
   `rt/db/table.go`) and asks the store for the rows. The SQLite store
   runs

   ```
   SELECT "id", "title", "done", "due", "owner", "created" FROM "Todo__Todo" WHERE "owner" = ? ORDER BY "created" DESC LIMIT ?
   ```

   with the arguments `session.User` and `10`. Every name comes from the
   descriptor and every value is a `?`, so nothing from the request is
   ever pasted into SQL. Each row is turned into a `Todo__Todo` by the
   descriptor's `Decode`.
5. The result becomes a `pure` node. The loop pops the continuation and
   calls it with the list. That Go function now builds the *next* part of
   the tree, the `bind` around `eff.All`, and the loop carries on with
   it.
6. The two `Db.get` steps run in order, then `Time.now` reads
   `env.Now()`, then `q` is built and `Db.count` runs it, then
   `Db.first` runs `Todo.pending` with a limit of 1.
7. The last continuation returns `pure` of the `Model`. The stack is
   empty, so that is the result.

Run against a database holding one todo, the trace is

```
["Db.all Todo" "Db.get Todo" "Db.get Todo" "Time.now" "Db.count Todo" "Db.first Todo"]
```

one name per step, in source order. Step 5 is why the interpreter is
called **eager**: a continuation is a Go function that is called only once
the value it needs exists, so the tree after a step isn't built until the
step has run. That is what lets a `match` on a result decide what happens
next, and it is why two effect values can't be compared with `==`
(chapter 12, "Effects are data, interpreted eagerly"). The stack is a Go
slice, not Go's call stack, so a block of any length runs in constant
stack space.

### A `Task`: `Toggle`

The `Toggle` arm of `Index.update` lowers to

```
    (case Toggle (bind _m3 : ToggleParams)
      (let p : ToggleParams
        (var _m3)
        (bind task _
          (db-update Todo.Todo
            (field id (var p))
            (lambda (t : Todo)
              (update (var t)
                (done (call (std Prelude.not) (field done (var t)))))))
          (pure task (ctor Prelude.Nav.Rerender)))))
```

and is emitted as

```go
	case is__Index__Msg__Toggle:
		p := *msg.Toggle
		return eff.Bind(eff.Call("Db.update Todo", true, func(_e2 *eff.Env) (struct{}, error) {
			return struct{}{}, db.UpdateRow(_e2.Ctx, _e2.Store.(db.Store), db.TxOf(_e2), table__Todo__Todo, p.Id, func(t Todo__Todo) Todo__Todo {
				return Todo__Todo{Id: t.Id, Title: t.Title, Done: !t.Done, Due: t.Due, Owner: t.Owner, Created: t.Created}
			}, _e2.Now())
		}), func(_ struct{}) eff.Task[rt.Nav] {
			return eff.Pure(rt.Rerender())
		})
```

Two different `bind`s appear here. The first, `(bind _m3 : ToggleParams)`
right after `case Toggle`, is section 6's pattern bind: `match msg` binds
the message's payload to a temporary, and `(let p … (var _m3) …)` gives it
your name `p`. The second, `(bind task _ …)`, is an `EffBind` from this
section: run the `db-update`, discard its result (`_`), continue with
`pure task`. The kind word `task` (not `query`) tells them apart.

The Go follows section 7's `match` rules. `case is__Index__Msg__Toggle:`
tests the tag, and `p := *msg.Toggle` reads the payload: `msg.Toggle` is
a pointer to the payload struct (every payload is held by pointer), and
`*` reads the struct it points to. Here the temporary `_m3` is gone, as
`_m1` was in `area`. The last argument to `db.UpdateRow`, `_e2.Now()`, is
the time from the `Env`'s clock: the store writes it into the table's
`@updated` column, if it has one. This `Todo` has none, so it goes
unused, but the call looks the same for every table.

This time the step is marked `true`, a write, and the caller runs it with
`eff.Run(env, eff.KTask, …)`:

1. `Run` remembers `env.Session` and begins no transaction yet.
2. The `call` named `Db.update Todo` is traced and run. Its `db.TxOf(_e2)`
   finds no transaction, so it begins one (`env.Store.Begin`, through
   `env.Transaction`) and records it in `env.Tx`. `db.UpdateRow`
   (`rt/db/store.go`) reads the row inside the transaction,
   `SELECT "id", "title", "done", "due", "owner", "created" FROM "Todo__Todo" WHERE "id" = ?`,
   decodes it, and applies the function, which flips `Done`. It checks
   that the id didn't change, encodes the record, and writes it back:

   ```
   UPDATE "Todo__Todo" SET "title" = ?, "done" = ?, "due" = ?, "owner" = ? WHERE "id" = ?
   ```

   `id` and `created` are not in the `SET` list: neither can change.
3. The continuation returns `pure` of `rt.Rerender()`, a `Nav`.
4. The task succeeded, so `Run` commits the transaction step 2 began and
   returns the `Nav`. The trace is `["Db.update Todo"]`.

If `p.Id` names no row, step 2 fails with
`Db.update Todo: no row with id "nope"`, an `*eff.ErrFail`. The loop
stops at once, the continuation is never called, and `Run` rolls the
transaction back instead of committing, and puts back the session it
remembered. The `Remove` arm shows the same
for `Task.fail`: when `Db.get` finds nothing, `match found` picks
`Task.fail "no such todo"`, a `fail` node, and the transaction is rolled
back with nothing written.

You can watch this happen. `TestGen_RunTodos` in
`internal/gen/gen_test.go` emits the to-do example, opens an in-memory
SQLite store, calls `Ensure`, and runs its `load` and `update` with
`eff.Run`: log in, add, toggle, delete, an add that is rolled back by a
later `Task.fail`, a log out whose session change is undone by a later
`Task.fail`, and a log out. After each one it checks the database and the
trace of the steps that ran, `calls("toggle", "Db.updateMine Todo")`
(the to-do app's `Toggle` is one scoped write, chapter 6, "Rules"; "The
rules", below, shows what it does): exactly the kind of check chapter 10
promises `alder test` will make.

### A request: `GET` and `POST`

The last two parts ran `load` and `update` by hand. In the server, a
request handler runs them. This part follows two requests through the
to-do app (chapter 2): a `GET` of the list, then a `POST` of the Done
button, sent by Datastar. Build the app and run it with the commands in
chapter 11, log in as `a@b.c` with the password `password`, and add a
todo called Milk. The bytes below are what that server sent; the id
`Em2Gv4v9BaA0Wf9ujESN` is random and differs every run.

**`GET /?filter=Active`.** The router's `ServeHTTP` (`rt/web/router.go`)
sees that the path is neither the Datastar script nor under `/static/`. It
splits the path into segments (none, for `/`) and finds the first route
with method `GET` whose pattern fits: `/`, the list page. It builds a
`web.Request` with no path parameters, the query `filter=Active`,
`Datastar: false`, and the session from the cookie, and gives the request
a fresh `eff.Env` holding the store, the capabilities, the clock and the
session. Then it calls the handler (`build/todos/routes__alder.go`):

```go
func Route__Todos__Index__page() web.Route {
	return web.Route{Method: "GET", Pattern: "/", Handle: func(rq *web.Request) web.Response {
		params, errs := decode__Todos__Index__Params(rq.Params, rq.Query)
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
		model, session, stop := page__Todos__Index(rq, params, nil)
		if stop != nil {
			return *stop
		}
		rq.Model = model
		return web.PageResponse(Todos__Index__view(model, session), &html.FormState{PageParams: params__Todos__Index(params)}, rq.Session, false)
	}}
}
```

(`rq.Model = model` keeps the model on the request. The server never
reads it; a test's browser does, "The browser", below.)

`decode__Todos__Index__Params` turns the text `Active` into the
constructor `Active` of `Todo.Filter`, through `web.Enum`. Any other text
(`?filter=Nope`) is a decode error, and the answer is a 404. A missing
`filter` is `All`, its default.

`page__Todos__Index` is the part of the lifecycle that every handler of
the page shares:

```go
func page__Todos__Index(rq *web.Request, params Todos__Index__Params, keep any) (Todos__Index__Model, Todos__Index__Session, *web.Response) {
	var model Todos__Index__Model
	app := rq.Env.Session.(App__Session)
	if !app.User.Ok {
		r := web.RedirectResponse(rq.Login, keep)
		return model, Todos__Index__Session{}, &r
	}
	session := Todos__Index__Session{User: app.User.Val}
	model, err := eff.RunQuery(rq.Env, Todos__Index__load(params, session))
	if err != nil {
		r := route__fail(err)
		r.Session = keep
		return model, session, &r
	}
	return model, session, nil
}
```

The first four lines are `requires user`. `app.User.Ok` is false when
`user` is `Nothing`, and the answer is then a 303 to `rq.Login`, which is
`/login` because `App.login` is `Auth.Login`. Otherwise the session is
narrowed: `session.User` is `app.User.Val`, a plain `string`, as section
9 explains. Then `load` builds its tree and `eff.RunQuery` runs it, as
"What `Run` does" showed. When the query stops with `Query.notFound` or
fails, `route__fail` makes a 404 or a 500. `keep` is `nil` here; the
`POST` below shows why it exists.

`view` turns the model into an `html.Node`. The handler returns it with a
`FormState` that holds only the page's parameters as text:

```go
func params__Todos__Index(p Todos__Index__Params) []rt.KV {
	return []rt.KV{rt.QueryKV("filter", []string{"All", "Active", "Completed"}[p.Filter.Tag], "All")}
}
```

`rt.QueryKV` marks the pair to be left out when the value is the default,
`All`. Back in the router, `write` compares the answer's session with the
one the request carried (`setSession`): a page view leaves it as it was,
so no cookie is set, and the browser keeps the one the login stored, with
its issued time (chapter 6, "The session cookie"). Then it calls
`html.RenderWith(node, state)`. Every message attribute in the
tree is an `html.MsgAttr`, a value that is only turned into text now: its
`rt.MsgRef` (module, message, the message's own parameters) is given the
page's parameters with `_p.` in front (`MsgRef.URL` in `rt/core.go`). A
`<button>` with a `click` is rendered as a click button here too: the
renderer knows whether it is inside a `<form>`, and wraps the button in a
form of its own when it is not (chapter 6, "With and without JavaScript");
the Done button is the `<button>` the compiler put inside an
`<al-button>` (section 5), and the form wraps the `<al-button>` whole.
Last, `document` writes `<!doctype html>` and the page, with the Datastar
script tag put after `<head>`. The answer begins like this (without the
`Date` and `Content-Length` headers, and with the page cut after
`</title>`):

```
HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8

<!doctype html>
<html><head><script type="module" src="/_alder/datastar-v1.0.0.js"></script><meta charset="utf-8" /><title>Todos</title>
```

and has the Done button with its message URL:

```html
<form method="post" action="/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&amp;_p.filter=Active" class="_alder-click" data-on:submit="window._alderAC?.abort();(window._alderBusy||((e,p)=&gt;p))(el, @post(&#39;/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&amp;_p.filter=Active&#39;, {contentType: &#39;form&#39;, requestCancellation: window._alderAC = new AbortController()}))"><al-button variant="secondary" size="sm"><button type="submit">Done</button></al-button></form>
```

The `&#39;` and `&amp;` are the escaping of section 8's first part: the
attribute holds
`window._alderAC?.abort();(window._alderBusy||((e,p)=&gt;p))(el, @post('/_m/Todos.Index/Toggle?id=…&_p.filter=Active', {contentType: 'form', requestCancellation: window._alderAC = new AbortController()}))`:
the abort and the shared AbortController make the newest navigation the only
one whose answer is applied (chapter 6, "Links and navigation").

**`POST /_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&_p.filter=Active`.**
Click Done, and the button submits its little form; Datastar catches the
submission and runs that `@post`. It sends the header
`Datastar-Request: true` and the form's fields, url-encoded: there are
none. The router matches the `POST` route with pattern
`/_m/Todos.Index/Toggle`, so `rq.Form` is empty. `rq.Datastar` is true. The handler,
`Route__Todos__Index__Toggle`, is the one quoted in chapter 6. It calls
`rt.PageParams(rq.Query)`, which keeps only the `_p.` keys with the prefix
cut off, so `filter=Active`, and decodes them with the same
`decode__Todos__Index__Params` the `GET` used. It decodes `id` from the
rest of the query. Then it calls:

```go
func update__Todos__Index(rq *web.Request, params Todos__Index__Params, msg Todos__Index__Msg, st *html.FormState) web.Response {
	model, session, stop := page__Todos__Index(rq, params, nil)
	if stop != nil {
		return *stop
	}
	nav, err := eff.Run(rq.Env, eff.KTask, Todos__Index__update(msg, model, session))
	if err != nil {
		return route__fail(err)
	}
	switch {
	case nav.Tag == rt.NavRedirect:
		return web.RedirectResponse(rt.PageURL(nav.Page), rq.Env.Session)
	case nav.Tag == rt.NavReject && st != nil:
		st.FormError = nav.Text
	case !rq.Datastar:
		return web.RedirectResponse(rt.PageAt("/", params__Todos__Index(params)), rq.Env.Session)
	default:
		st = &html.FormState{}
	}
	st.PageParams = params__Todos__Index(params)
	model, session, stop = page__Todos__Index(rq, params, rq.Env.Session)
	if stop != nil {
		return *stop
	}
	return web.PageResponse(Todos__Index__view(model, session), st, rq.Env.Session, true)
}
```

This is chapter 6's lifecycle, line by line:

1. `page__Todos__Index` checks `requires` and runs `load`, so `update`
   gets the same `Model` the page showed.
2. `eff.Run` with `eff.KTask` runs the `Toggle` arm much as "A `Task`:
   `Toggle`" did, as one step, `Db.updateMine Todo`: it reads the row
   with the owner in the `WHERE` (which begins the transaction), writes
   it with the owner in the `WHERE` again, then a commit, and `Nav`
   `Rerender` ("The rules", below). If the task failed, `route__fail`
   answers 500, or 404 for `Task.notFound` (an id that is missing or
   someone else's), and nothing is stored, since `Run` already rolled
   back the writes and the session.
3. Under Datastar, `Rerender` takes the `default` branch: an empty
   `FormState`, then the page's parameters, so the re-rendered buttons
   carry `_p.filter=Active` again. (Without Datastar it stops at
   `!rq.Datastar` instead, below.)
4. `page__Todos__Index` runs a second time, now with `keep` set to the
   session the task left. `load` reads the database after the commit, so
   the new model has Milk done. If this second load fails, the 404 or 500
   still stores that session: what the task committed is not lost.
5. `PageResponse(…, true)` asks for a patch.

The router's `write` sees a 200 with `Patch` set and `rq.Datastar` true.
The task left the session as it was, so it sets no cookie. It renders
the page, cuts out its `<body>…</body>`
(`bodyOf`), and calls `datastar.PatchBody`, which writes one server-sent
event: the event name, two lines that say what to replace and how, and one
`data: elements` line for each line of the body. The answer is

```
HTTP/1.1 200 OK
Cache-Control: no-cache
Content-Type: text/event-stream

event: datastar-patch-elements
data: selector body
data: mode outer
data: elements <body><main><h1>Todos</h1>…<ul id="todos"></ul><p id="summary">0 open</p>…</main></body>

```

(without the `Date` and `Transfer-Encoding` headers, and shortened at each
`…`). The rendered page has no line breaks, so the body
is one `elements` line, and a blank line ends the event. (`PatchBody`
turns every `\r\n` and lone `\r` into `\n` before it splits the lines, and
the renderer writes a carriage return in text as `&#13;`: a line end
inside your data can never end a `data:` line and start a field.) Datastar finds
`body` in the page and morphs it into the new one: Milk is done, so it
leaves the Active list, and the summary says `0 open`. No `<head>` and no
doctype are sent.

The same `POST` without the `Datastar-Request` header goes through the
same handler, up to the `switch`: there `Rerender` meets `!rq.Datastar`
and answers a 303 to the page the message came from, rebuilt from its
parameters by `rt.PageAt("/", params__Todos__Index(params))`:

```
HTTP/1.1 303 See Other
Location: /?filter=Active
```

The browser then `GET`s `/?filter=Active`, so its address bar shows the
list, not the message URL, and a refresh does not post again. If the id
names no row, or another user's, `Db.updateMine` ends in `Task.notFound`, and under
Datastar the 404 page's body arrives the same way as a re-render, as a 200
event.

A `Redirect` under Datastar is not a 303 either, because the client's
`fetch` would follow a 303 itself and leave the address bar behind. The
next two parts, "A boosted link" and "A redirect, followed", show what
the router sends instead: for a page of this app, the page itself.

**A boosted link.** Every page the router writes whole carries three
tags (`HeadTags` and `withScript` in `rt/web/router.go`): right after
`<head>`, the `color-scheme` meta and the Datastar script; just before
`</head>`, `datastar.BoostTag()`, last because it is about a kilobyte and
would otherwise push a layout's `<meta charset>` out of the first 1024
bytes, where the browser looks for it:

```html
<meta name="alder-boost" content="links"
  data-init="window._alderAt = location.pathname + location.search"
  data-on:click__window="…"
  data-on:popstate__window="…"
  data-on:scroll__window__throttle.200ms.trailing="…" />
```

(shown on several lines; the quotes inside the attributes are escaped as
`&#39;` in the real page). Datastar reads `data-*` attributes on every
element of the document, the `<head>` included. `data-on:click__window`
is a click listener on `window`, the outermost target, so it sees every
click on the page after the page's own handlers have had theirs; and it
is on an element in the `<head>`, which a body patch never touches, so
the listener outlives every morph. Its expression, `datastar.BoostClick`,
is plain JavaScript that Datastar compiles into a function (it rewrites
`@get(` into a call of its `get` action, and `$name` into a signal, so the
expression uses no `$`):

```js
if (evt.defaultPrevented || evt.button !== 0 || evt.metaKey || evt.ctrlKey || evt.shiftKey || evt.altKey) return;
const a = evt.target.closest ? evt.target.closest('a[href]') : null;
if (!a || a.hasAttribute('target') || a.hasAttribute('download') || a.hasAttribute('data-alder-reload')) return;
const u = new URL(a.getAttribute('href'), document.baseURI);
if (u.origin !== location.origin || (u.protocol !== 'http:' && u.protocol !== 'https:')) return;
if (u.hash && u.pathname === location.pathname && u.search === location.search) return;
evt.preventDefault();
window._alderHash = u.hash;
history.scrollRestoration = 'manual';
history.replaceState({alderY: scrollY}, '');
@get(u.pathname + u.search, {payload: {}})
```

Each `return` is one of chapter 6's exceptions, left to the browser. A
click that gets past them is cancelled (`preventDefault`), the link's
`#fragment`, if any, is kept in `window._alderHash` (a browser never
sends the fragment to the server, so the push adds it back), the scroll
position is saved in the state of the history entry being left, and
Datastar sends `GET /posts/42?datastar=%7B%7D` with
`Datastar-Request: true`. (Datastar always adds the page's signals to a
`GET` as the parameter `datastar`; `payload: {}` makes them `{}`, and the
router drops the parameter before it matches the route, so a page's
parameters never see it.)

`ServeHTTP` sees a Datastar `GET` and marks it as a navigation: the URL to
record is the request's, and the mode is *push*. The route runs as for any
`GET`, and `write`, finding a navigation, calls `morph` instead of writing
a document. The blog's home page, boosted, answers

```
HTTP/1.1 200 OK
Cache-Control: no-cache
Content-Type: text/event-stream

event: datastar-patch-elements
data: selector body
data: mode outer
data: elements <body><al-page><header><a href="/">Blog</a></header><main><h1>Blog</h1>…</main></al-page></body>

event: datastar-patch-elements
data: selector head > title
data: mode outer
data: elements <title>Blog</title>

event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">(function(){var u="/";if(u.indexOf("#")<0&&window._alderHash)u+=window._alderHash;window._alderHash="";history.pushState(null,"",u);window._alderAt=location.pathname+location.search;document.body.tabIndex=-1;document.body.focus({preventScroll:true});var h=location.hash&&document.getElementById(decodeURIComponent(location.hash.slice(1)));h?h.scrollIntoView():scrollTo(0,0)})()</script>

```

Datastar applies the three events in order. The first morphs the body.
The second replaces the `<title>`; its selector is `head > title`, so an
SVG's `<title>` in the body is left alone (`titleOf` cuts the title out of
the rendered page; a page without one sends `<title></title>`, so the old
page's title does not stay). The third is `datastar.Navigate` in mode
`Push`: a script appended to the body, which Datastar runs and which
removes itself. It records the URL, with the clicked link's fragment
added back when the URL has none of its own (`history.pushState`: the
address bar changes, nothing loads), notes the
path the page now shows in `window._alderAt`, makes the `<body>`
focusable (`tabindex="-1"`) and focuses it, so keyboard and screen reader
users start at the top of the new page, then scrolls to the top (or to
the element a `#fragment` in the URL names). A 404 or 500 is answered with
the same three events and the status in `Alder-Status`, as for a message.

Back and Forward fire `popstate` on `window`. The boost's
`data-on:popstate__window`, `datastar.BoostPopstate`, asks for the page
again unless only the fragment changed:

```js
if (location.pathname + location.search !== window._alderAt) @get(location.pathname + location.search, {payload: {}, headers: {'Alder-History': 'replace'}})
```

The header `Alder-History: replace` makes the mode *replace*: the
history entry exists already, so the script calls `history.replaceState`
instead (keeping the entry's fragment), and scrolls to the `alderY` in
that entry's state (`history.scrollRestoration` is `manual`, so the
browser does not scroll by itself before the page has come back). The
fourth attribute, `datastar.BoostScroll`, keeps `alderY` up to date: on
every `scroll` of the window, at most every 200 ms and once more after
the last one (`__throttle.200ms.trailing`), it runs
`history.replaceState(Object.assign({}, history.state, {alderY: scrollY}), '')`,
writing the position into the current entry. So an entry has its
position whichever way you leave it, by a click or by Back, and Back and
Forward both return to it.

**A redirect, followed.** A `Redirect` under Datastar to a page of this
app is followed by the router itself. `write` sees a 303 with
`rq.Datastar` set and a Location that `followable` accepts (a path
starting with `/`, not `//`, not the Datastar script, not a file under
`/static/`), stores the session as for any 303, and calls `follow`.
`follow` clones the request into a `GET` of the Location (no body, no
form) and serves it through the same `serve` the router runs for a
request from outside, with two differences: the request's session is the
one the redirect just stored (the browser does not have the new cookie
yet; it arrives with this answer), and the navigation to record is the
Location, in mode push. The target's handler runs, `Options.Served` sees
it (so a test's browser gets the target's model), and `write` answers
with the target's morph. If the target redirects in turn, `follow`
follows again; a GET that redirects when ten GETs of the chain are
already behind it is a 500, logged for the request the browser sent
(`more than 10 redirects in a row (the last to …)`), as the test browser
counts them. Clicking "Log out" on `/?filter=Active` (an App message
whose `update` answers `Redirect (Auth.Login {})`) answers

```
HTTP/1.1 200 OK
Cache-Control: no-cache
Content-Type: text/event-stream
Set-Cookie: alder_session=…; Path=/; Expires=…; Max-Age=2592000; HttpOnly; SameSite=Lax

event: datastar-patch-elements
data: selector body
data: mode outer
data: elements <body>…the login page's body…</body>

event: datastar-patch-elements
data: selector head > title
data: mode outer
data: elements <title>Log in</title>

event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">(function(){var u="/login";…history.pushState(null,"",u);…})()</script>

```

A Location that is not followable, another site's or `#`, is answered as
before M13, with `datastar.Navigate` in mode `Assign`: one event whose
script calls `window.location.assign` with the URL, a real navigation.

`TestE2E_Todos` in `internal/gen/e2e_test.go` runs this whole scenario
against the generated app on every `go test ./...`. Its program,
`internal/gen/testdata/e2e_todos_test.go.txt`, checks that a Datastar
answer starts with exactly
`event: datastar-patch-elements\ndata: selector body\ndata: mode outer\ndata: elements <body>`
and ends with `</body>\n\n`, that a `Redirect` under Datastar is the
target's body followed by the push of its URL, that a `Toggle` sent without JavaScript from
`/?filter=Completed` is a 303 back to that filter, and that another user
posting the messages with a@b.c's id gets 404 every time.

### The rules: scoped operations and roles

Chapter 6, "Rules", declares two rules: `@owner` on a table, used by the
five scoped `Db` operations, and `requires user, role R` on a page. This
part follows them through the compiler and the runtime, with the to-do
app (`Todo.owner @owner`) and the blog (`role Author`).

**The checker.** `@owner` is checked once `App`'s session is known
(`checkOwners`, `internal/check/rules.go`): the session must have a
`user` field, and the marked column must be a `@ref` of that field's
type without the `Maybe`, one per table (E0024). The column's name
becomes the table's `Owner`. A reference to one of the five operations
is checked where it is written (`inferVar` sends `Db.mine` and the others
to `inferScoped`), against the module being checked: it is legal in a
page module whose page `requires user`, and in a `test`; anywhere else
it is E0026. A top-level value that uses one legally is marked, and the
mark follows references to it within its module (`propagateScoped`, a
fixpoint), so a use of the value from a module where the operation
itself would be refused is E0026 at that use. After a body is typed,
each operation's table argument must have an `Owner` (E0025,
`checkScopedTables`).
`Db.insertMine`'s row is typed as `InsertMine T`, a type that is never
written in source: `Insert T` without the owner column. Its literal may
not name the owner (E0003), and a row that is not a literal in the call
is refused, so the row can only be what is written there. The role
clause is checked by `checkRoles`, called from `definePage` (E0027): `user` first, a `role` field
of the session whose type is a union and not a `Maybe`, and each name
one of its constructors.

**The IR.** Five nodes of their own, not flags on `DbGet` or `DbUpdate`,
so that every later stage has to say what it does with them:
`DbScoped{Table}`, a `Select` value; `DbGetScoped{Table, ID}`;
`DbUpdateScoped{Table, ID, Fn}`; `DbDeleteScoped{Table, ID}`;
`DbInsertScoped{Table, Row}`. Each also carries `Op`, the operation as
written (`Db.mine`, `Db.scoped`), which traces and reports use, and
`Key`, the session field it scopes by: `user` for the `mine` family and
for any table whose `@partition` is `user` (M15 generalised M11's
`DbMine` nodes into these: "Partitions", below). A pipeline that starts
from `Db.mine` is an ordinary `DbSelect` whose `Base` is the
`DbScoped`. The to-do list's `load` (`alder dump --ir examples/todos`,
as in `testdata/golden/ir_todos.ir`):

```
(value Todos.Index.load : Params -> Session -> Query Model (params session)
  (bind query todos : List Todo
    (db-select Todo.Todo
      (base (db-scoped Db.mine user Todo.Todo))
      (order (ord (col Todo.Todo created) desc))
      all)
```

and its `Toggle` arm:

```
        (bind task _
          (db-update-scoped Db.updateMine user Todo.Todo
            (field id (var p))
            (lambda (t : Todo)
              (update (var t)
                (done (call (std Prelude.not) (field done (var t)))))))
          (pure task (ctor Prelude.Nav.Rerender)))))
```

The table records its partition, `(partition user owner)`, and a page
its roles, `(roles Author)`. Nothing else in the IR changes.

**The Go.** The table's descriptor (`Todo_alder.go`) gets two fields:

```go
	Owner: "owner",
	User: func(s any) (string, bool) {
		a, ok := s.(App__Session)
		return a.User.Val, ok && a.User.Ok
	},
```

`Owner` is the column; `User` takes a session and says who is signed in,
if anyone. The operations are emitted like the unscoped ones, a `Call`
over a typed helper of `rt/db`. `load`'s `Db.mine Todo` is the `Select`
`db.Mine(table__Todo__Todo)`, with the order on top:

```go
func Todos__Index__load(params Todos__Index__Params, session Todos__Index__Session) eff.Query[Todos__Index__Model] {
	return eff.Q(eff.Bind(eff.Call("Db.all Todo", false, func(_e1 *eff.Env) ([]Todo__Todo, error) {
		return db.All(_e1.Ctx, _e1.Store.(db.Store), db.TxOf(_e1), db.OrderBy(db.Desc("created"), db.Mine(table__Todo__Todo)))
```

`Toggle` is one step:

```go
	case is__Todos__Index__Msg__Toggle:
		p := *msg.Toggle
		return eff.Bind(eff.Call("Db.updateMine Todo", true, func(_e2 *eff.Env) (struct{}, error) {
			return struct{}{}, db.UpdateMineRow(_e2.Ctx, _e2.Store.(db.Store), db.TxOf(_e2), table__Todo__Todo, p.Id, func(t Todo__Todo) Todo__Todo {
				return Todo__Todo{Id: t.Id, Title: t.Title, Done: !t.Done, Due: t.Due, Owner: t.Owner, Created: t.Created}
			}, _e2.Now())
		}), func(_ struct{}) eff.Task[rt.Nav] {
			return eff.Pure(rt.Rerender())
		})
```

and `Submit`'s `Db.insertMine` builds the row without its owner, a Go
struct of its own (`insertmine__Todo__Todo`), which
`insertminevals__Todo__Todo` turns into the column values with a `nil`
where the owner goes:

```go
		return eff.Bind(eff.Call("Db.insertMine Todo", true, func(_e1 *eff.Env) (string, error) {
			return db.InsertMineRow(_e1.Ctx, _e1.Store.(db.Store), db.TxOf(_e1), table__Todo__Todo, insertminevals__Todo__Todo(insertmine__Todo__Todo{Title: p.Form.Title, Done: false, Due: p.Form.Due}), _e1.Now())
```

Notice what none of these is given: the user. `update`'s `session` is
not passed to `UpdateMineRow`, and `load`'s is not in the `Select`.

**Whose user: the ambient session.** Each helper asks `owner`
(`rt/db/store.go`) for the user. `owner` takes the `Env` that is running
the effect from the context (`eff.EnvFrom(ctx)`, the same way a
capability finds it: chapter 7), reads its `Session`, and hands it to the
descriptor's `User`. In a served request `Env.Session` is the session
from the cookie; in a test, the test's session, which `Test.login` and
`Session.put` set (chapter 10, "Tests that use the rules"). Since
`Session.put` writes `Env.Session`, a scoped step after it acts for the
new user. With no `Env`, or `User` answering false, the step fails with
an `*eff.ErrFail` whose text is the operation and table: `Db.mine Todo:
no signed-in user`. Reading the user at run time, not from an argument,
is what lets a helper value such as `recent = Db.mine Todo |> Db.limit
5` (a Go package variable, built once) mean "the current user's" at each
use.

`db.Mine(t)` is a `Select` with a flag, `Mine: true`, and no condition
yet. `All`, `First` and `CountRows` resolve it as they start (`scoped`):
they get the user from `owner`, put `CEq(owner, user)` in front of the
select's conditions, and call `Env.Note("Db.mine Todo")`, which logs the
name and puts it into the trace just before the running step's own
name. That is why a trace reads `["Db.mine Todo", "Db.all Todo"]`: the
note is not a step (no `Call` ran for it), but it says which select the
step ran. `Select.SQL` refuses a `Mine` select that was not resolved, so
an unscoped query can never be made from one by mistake.

**The SQL.** With a store that logged each statement, the to-do app's
tests sent these (every value is a `?`):

- `Db.mine Todo |> Db.orderBy Todo.created.desc |> Db.all`, the list's
  `load`:

  ```
  SELECT "id", "title", "done", "due", "owner", "created" FROM "Todo__Todo" WHERE "owner" = ? ORDER BY "created" DESC
  ```

- `Db.mine Todo |> Db.count`, the test "Db.mine is what Me sees":

  ```
  SELECT COUNT(*) FROM (SELECT 1 FROM "Todo__Todo" WHERE "owner" = ?)
  ```

- `Db.getMine Todo params.id`, `Todos.Show`'s `load` (`GetMineRow`, through
  the store's `GetWhere`):

  ```
  SELECT "id", "title", "done", "due", "owner", "created" FROM "Todo__Todo" WHERE "id" = ? AND "owner" = ?
  ```

  No row is `eff.ErrNotFound`, so the page is a 404.

- `Db.updateMine Todo p.id f`, `Toggle` (`UpdateMineRow`), two
  statements in the `Task`'s transaction:

  ```
  SELECT "id", "title", "done", "due", "owner", "created" FROM "Todo__Todo" WHERE "id" = ? AND "owner" = ?
  UPDATE "Todo__Todo" SET "title" = ?, "done" = ?, "due" = ? WHERE "id" = ? AND "owner" = ?
  ```

- `Db.deleteMine Todo p.id`, `Delete` (`DeleteMineRow`, through
  `DeleteWhere`), one statement:

  ```
  DELETE FROM "Todo__Todo" WHERE "id" = ? AND "owner" = ?
  ```

- `Db.insertMine Todo { … }`, `Submit` (`InsertMineRow`): the same
  `INSERT` as `Db.insert`, with the user put in the owner's place:

  ```
  INSERT INTO "Todo__Todo" ("id", "title", "done", "due", "owner", "created") VALUES (?, ?, ?, ?, ?, ?)
  ```

When a stranger pressed my todo's Toggle ("Toggle by another user is
not found"), only the `SELECT … AND "owner" = ?` ran: it found nothing,
and no `UPDATE` was sent.

**The rows-affected check.** `UpdateMineRow` reads the row first because
`f` is a function of the row: it needs the current values to compute
the new ones. It checks that `f` kept the id, then writes. The `UPDATE`
carries the owner predicate again, and the store's `UpdateWhere` returns
how many rows the statement changed (`RowsAffected` of the result). If
that is 0 (the row was deleted or changed hands between the two
statements), the step is `eff.ErrNotFound` like a missing row, and the
`Task` rolls back. So ownership is decided by the statement that writes,
on the database that writes, whatever the read saw (chapter 6, "The
rules and a replicated database"). The `SET` list leaves out the id,
`@created` and the owner: `UpdateMineRow` removes the owner's column and
value before it calls `UpdateWhere`, so an `f` that changes `owner`
writes everything else and the row stays the user's. `DeleteMineRow`
needs no read: its one `DELETE` carries the predicate, and 0 rows
deleted is `eff.ErrNotFound`. (`Db.delete` of a missing row is not an
error; `Db.deleteMine`'s is, since it cannot tell missing from someone
else's, and both must be a 404.)

**The role check.** A page with `role R` gets one more check in its
route function, after the `requires user` redirect and before `load`.
The blog's `page__Write` (`build/blog/routes__alder.go`):

```go
func page__Write(rq *web.Request, params Write__Params, keep any) (Write__Model, Write__Session, *web.Response) {
	var model Write__Model
	app := rq.Env.Session.(App__Session)
	if !app.User.Ok {
		r := web.RedirectResponse(rq.Login, keep)
		return model, Write__Session{}, &r
	}
	if app.Role.Tag != is__User__Role__Author {
		r := web.NotFoundResponse(html.None())
		r.Session = keep
		return model, Write__Session{}, &r
	}
	session := Write__Session{User: app.User.Val, Role: app.Role}
	model, err := eff.RunQuery(rq.Env, Write__load(params, session))
```

`app.Role` is the session's union value; its `Tag` is compared with the
constructor's tag (with `role Author | Admin`, the test is
`!(… == Author || … == Admin)`). A wrong role is the not-found page,
before any query runs: the check costs no database read. Every handler
of the page, its `GET` and each message's `POST`, goes through
`page__Write` first, so a message is checked the same way. The page's
`Session` carries `Role` beside the narrowed `User`.

### Partitions and silos

M15 generalised the rules of the last part (chapter 6, "Partitions";
chapter 8, "Silos"). This part follows `examples/workspace` through the
same stages: a key in the session, a table partitioned by it, a silo,
and the runtime that keeps a database per org.

**The checker.** `internal/check/partitions.go` reads `@partition k`
(and `@owner` as `@partition user`) into the table's `PartKey` and
`PartCol`, once the session is known (E0032); `via`, `choose` and `silo`
declarations into `Result.Via`, `Choose` and `Silos` (E0032, E0033). A
scoped operation is checked as in M11, with the key in place of `user`:
legal where the module's page `requires` the table's key (or is the
admin form `silo k`), in a test, and, through a helper, wherever the
helper's own use is legal (E0026). An unscoped operation on a siloed
table is held to the same rule. After every body is checked, the
**write-set analysis** (`internal/check/writeset.go`) collects, for each
top-level value, the tables its body writes: every mention of
`Db.insert`, `Db.update`, `Db.delete` or a scoped form, called there or
not (`let ins = Db.insert`, a lambda handed `Db.insert`), each with the
table its type has at that mention — the analysis reads types, not
spellings, so a write cannot hide behind a name. It closes that over
every value the body refers to (a fixed point, so a helper of a helper
counts), splits each `update` into its `match` arms, and gives each arm
the store its writes fall in: the shared one, or `silo org`. An arm with
two stores is E0031. It over-approximates (a helper counts if it is
referred to, called or not) and never under-approximates. What it
proves is about `update` arms (and fakes): a `test`'s own Tasks and an
adapter's Go code are the runtime's to check, below. The answer is
`Result.Arms`, which the IR keeps as `Page.Arms` (`(arms (Create silo
org) (Remove silo org))` in `alder dump --ir`).

**The Go.** The table's descriptor names its partition and its silo
(`build/workspace/Project_alder.go`):

```go
	Owner: "org",
	User: func(s any) (string, bool) {
		a, ok := s.(App__Session)
		return a.Org.Val, ok && a.Org.Ok
	},
	Key:  "org",
	Silo: "org",
```

`Owner` and `User` are M11's fields, now the partition column and the
key's value in a session; the scoped helpers (`db.Scoped`,
`InsertScopedRow`, `DeleteScopedRow`, …) are the M11 helpers under
general names, and emit the same SQL with `"org" = ?`. A page that
`requires user, org` gets two checks in its route function: no `org`
is a redirect to `choose org`'s path, and then `db.Narrow(rq.Env, "org",
app.Org.Val)` records the request's org for the runtime. A message's
handler binds its `Task` to the arm's store before it runs it:

```go
func store__Projects__Index(msg Projects__Index__Msg) string {
	switch msg.Tag {
	case is__Projects__Index__Msg__Create:
		return "org"
	case is__Projects__Index__Msg__Remove:
		return "org"
	}
	return ""
}
```

and `db.Enter(rq.Env, store__Projects__Index(msg))` opens that org's
database and makes it the store `eff.Run` begins the transaction on. The
`return` at the end is the store of the `_` arm (the shared one when
there is none), and every arm whose store differs from it has a `case`,
the shared store's included: an explicit arm that writes the shared
database beside a `_` arm on the org's silo answers `""`, not the
silo.
`via` is one function, the router's session check (`session__via` in
`app__alder.go`), called whenever a `Session.put` changes the session:
a user set to `Nothing` clears the `org`; an `org` that changed to `Just
v` needs a `Membership` row for the user and `v`, read with `db.Member`
inside the running `Task`'s transaction, or the step fails with
"Session.put org: no Membership for this user and org". That failure is
marked (`eff.ErrFail`'s `NotMember`), so a message's route answers it
as `Reject "You are not a member of that workspace."` on the form that
sent it, and with the not-found page (404) for a message without a
form, rather than the error page; `Test.run` sees the failure as it is.

**A store per silo.** `rt/db/stores.go`'s `Stores` holds the shared
database and the silos opened so far, by key and value. Every database
step of `rt/db` asks `route` which store it runs on: a table with no
`Silo` runs on the shared one; a siloed table on the silo of the row's
own `org` (an insert), else of the request's narrowed key (`Env.Keys`),
else of the session. When that is the store the `Task` was bound to,
the step runs in its transaction. A read of the other store runs on a
read-only connection outside the transaction (SQLite's `query_only`,
and a Go wrapper that refuses a write), which is how a `load` reads an
org's name and its projects in one go. A write to another store cannot
happen in an `update` arm that checked; one that reaches it anyway (a
test's own `Task`, an adapter, a bug in gen) is refused by `route` as an
internal error rather than written outside the `Task`.

**Opening a silo.** The first request for an org opens its file
(`openSilo`): the id must be an id, the org's row must exist in the
shared database (one read), and then the open-time check of chapter 8
runs in one `BEGIN IMMEDIATE` on the file: create it at the build's
schema; serve it when its `_alder_schema` row is the build's (`siloHash`:
the app's schema hash and the tables of each silo, so a table moved
into a silo leaves the silos made before it behind); apply the
pending migrations (each statement whose target is a silo table, with
references out of the silo removed) when it is behind, unless one drops
data; refuse it when it is ahead. The outcome is cached per process. A
silo not served is a `*MaintenanceError`, which the route answers with
the maintenance page (503), and is checked again after a minute; one
unused for ten minutes is closed. Only requests for the same org wait
while it opens: the registry's lock guards the map, not the check.

**The drain.** A deploy starts the new build while the old one drains
its requests (chapter 16). The new build may migrate a silo the old one
has open. So once the old process is told to stop, it opens no new silo
(a request for one answers the maintenance page) and, before each
write to a silo it has open, reads that file's `_alder_schema` row again
inside the write's transaction: if another build migrated it, the write
is refused and the silo answers the maintenance page for the rest of
the drain. A process that is not draining never re-checks, which is why
a deploy always signals the old process before it starts the new.

**Tests.** `db.NewTestStores` gives each test a `:memory:` database per
org it touches, made on first use, each with its transaction open for
the whole test, and the test's savepoints (`Test.run`, each browser
request) span all of them. A browser request's `Task` is bound by the
same `db.Enter` as production's. A `Test.run`'s `Task` is bound too
(`rtesting.Inner` sets `Env.Pending`): to the store its first write
lands in, so a second store fails it as a request's would, or, for
`Test.run (update msg model session)` of a page, to `store__M(msg)`'s
store before it runs (`rtesting.InnerOn`), so a wrong arm store fails
the test as it would fail the request. The test's own body is not
bound: it may set up rows in the shared database and in several silos.

### The dev server

`alder run` (chapter 11) is the last piece: the program that sits between
your editor, the compiler and the browser. All of it is in
`cmd/alder/run.go`. Here is what happens between saving a file and the
browser showing the change.

```
browser ──HTTP──▶ dev server (alder run, ALDER_ADDR) ──proxy──▶ app (child, 127.0.0.1:<free port>)
   ▲                 │  watcher: polls app/, adapters/, static/
   └──/_dev/reload───┘  worker: check → build → start child → swap → clear + reload
```

**The proxy.** The dev server's handler, `devServer.ServeHTTP`, answers
`/_dev/reload` itself and hands every other request to Go's
`httputil.ReverseProxy`, aimed at the current child's address. The
address is read once per request, under a lock, so a request that started
before a swap finishes on the child it started on. `FlushInterval: -1`
makes the proxy pass each piece of an answer on as soon as the app writes
it, which a Datastar answer (a stream of server-sent events, "A request"
above) needs. Cookies and `303`s are passed on untouched, and the
`Host` header is kept, so the app sees the address the browser used.

**The watcher.** `watch` polls rather than asking the operating system
for change notifications, so the compiler needs no dependency beyond Go's
standard library. Every 300 ms, `snapshot` records the modification time
and size of every `.alder` file under `app/`, every file under
`adapters/` (any directory, any extension: `alder build` copies the whole
tree, so any of it can change the build; but not an editor's files,
which `editorNoise` recognizes by name: a name starting with `.`, ending
in `~`, `.swp` or `.swx`, or of the form `#…#`, nor a symbolic link,
which the build does not copy), the migration files, and every file
under `static/`; `diff` compares two
snapshots and says which kinds changed, `sourceChange`, `staticChange` or
both (a `change` is a set of bits). The watcher does not report a change
at once. It waits for a poll that finds nothing new:

```go
		cur := snapshot(dir)
		if k := diff(prev, cur); k != 0 {
			pending |= k
			prev = cur
			continue
		}
		if pending != 0 {
			select {
			case changed <- pending:
			case <-stop:
				return
			}
			pending = 0
		}
```

So an editor that writes five files in a row produces one change, sent
300 ms after the last write. This is *coalescing*: merging a burst of
events into one.

**The worker.** One goroutine, `work`, receives the changes and is the
only caller of `rebuild`. Because there is one worker, two rebuilds can
never run at the same time (the rebuild is *single-flight*). While it
rebuilds, the watcher waits on its send, and when the worker comes back it
merges everything queued into one change (`k |= k2`) before building
again. A change that is only `staticChange` skips the rebuild: the child
serves `static/` straight from your directory (`ALDER_STATIC`), so the
browser just has to reload.

**`rebuild`.** It runs `alder check`, then the build, then starts the new
child. Each step that fails calls `fail` and returns, and returning early
is what keeps the old child serving:

```go
func (d *devServer) rebuild() {
	var out bytes.Buffer
	if code := runCheckTo([]string{d.dir}, &out, &out); code != 0 {
		d.fail(out.String())
		return
	}
	d.stderr.Write(out.Bytes()) // warnings
	out.Reset()
	binary, code := buildApp(d.dir, &out)
	if code != 0 {
		d.fail(out.String())
		return
	}
	dir := filepath.Dir(binary)
	static, err := filepath.Abs(filepath.Join(d.dir, "static"))
	if err != nil {
		d.fail("alder run: " + err.Error())
		return
	}
	d.started++
	gen := fmt.Sprintf("%s-%d", d.nonce, d.started)
	c, err := startChild(binary, filepath.Join(dir, "dev.db"), static, gen, d.stderr)
	if err != nil {
		d.fail("alder run: " + err.Error())
		return
	}
```

`buildApp` (`cmd/alder/build.go`) is `alder build` itself, the steps of
chapter 11, returning the executable's path. `gen` is the new child's
*build generation*: `d.nonce`, eight random hex digits picked when the dev
server starts, then a dash and a count of the children started so far
(`afcb2a7d-1`, `afcb2a7d-2`, …). "The subscriber" below is what it is for.
`startChild` runs the executable with
`ALDER_ADDR=127.0.0.1:0 ALDER_DEV=1 ALDER_RELOAD=<gen>`, the database
`build/<app>/dev.db` and your `static/`, and reads the child's stdout
until the line `listening on <addr>` that every generated `main` prints
once it has its port (chapter 11, "`alder build`"). That line is how the dev server learns
the free port the operating system picked. If the line doesn't come
within 10 seconds, or the child exits first, the error carries the last
40 lines of the child's stderr. Only after all that does the swap happen:

```go
	d.mu.Lock()
	old := d.child
	d.child = c
	d.gen = gen
	announce := !d.announced
	d.announced = true
	d.mu.Unlock()
```

From this point new requests go to the new child, and `d.gen` is the
current generation; then `old.stop()` sends
the old one `SIGTERM` and waits for it to exit, and the last lines clear
the overlay and tell every browser to reload:

```go
	d.mu.Lock()
	d.lastErr = ""
	d.broadcast(clearEvent, reloadEvent) // the clear first
	d.mu.Unlock()
```

A separate goroutine per child, `monitor`, waits for it to exit. If it
exits without having been stopped, it was a crash: the monitor sets the
current child to none (so the proxy answers 502) and calls `fail` with
the exit status, `save a source file to rebuild it`, and the last 40
lines of the child's stderr. It leaves `d.gen` alone: the generation
stays that of the app that crashed until a new one comes up.

**The subscriber.** Every page the child renders carries two elements
right after `<body>`. They come from the router, not the dev server
(`rt/web/router.go`):

```go
func reloadTags(gen string) string {
	return `<div id="_alder_errors" data-ignore-morph></div><div data-init="@get('/_dev/reload?b=` +
		url.QueryEscape(gen) + `', {openWhenHidden: true})"></div>`
}
```

`withReload` inserts them, and the router's `dev` method applies it only when
`Options.Reload`, the generation, is not empty. The generated `cmd/main.go`
sets it from the environment:

```go
		Reload:    os.Getenv("ALDER_RELOAD"),
```

So the first page `alder run examples/todos` served began

```html
<body><div id="_alder_errors" data-ignore-morph></div><div data-init="@get('/_dev/reload?b=afcb2a7d-1', {openWhenHidden: true})"></div><main>
```

`data-init` is Datastar's "run this when the element is set up";
`@get('/_dev/reload?b=afcb2a7d-1', …)` opens the event stream and applies
each event that arrives on it. (A `<div>` never fires a `load` event, so an
attribute that waited for one would never run; `data-init` runs as soon
as Datastar sets the element up.) Three details of these two elements
each fix a way the browser could miss a message:

- `data-ignore-morph`. When a `Rerender` patches the whole `<body>`
  ("A request" above), Datastar *morphs* the old body into the new one:
  it walks both and changes only what differs. The new body's box is
  empty, so without this attribute every click would wipe an error off
  the page. Datastar 1.0.0 leaves an element alone when the old and the
  new element both carry `data-ignore-morph`, which the box always does.
  The dev server's own patches still reach it: they patch the box's
  *inside* (`mode inner`, below), and the check compares only the box
  and the element that replaces it, not what goes inside.
- `openWhenHidden: true`. By default Datastar's `@get` closes its stream
  while the tab is hidden and opens it again when you come back, so a
  reload sent meanwhile would be lost. With this option the stream stays
  open.
- `?b=afcb2a7d-1`, the generation of the app that rendered the page. A
  stream can still close: the network drops for a moment (Datastar
  then retries), or you restart `alder run`. When a page reconnects,
  `serveReload` compares its generation with `d.gen` and, when they
  differ, sends the reload straight away; the page belongs to an older
  build or an earlier dev server, so whatever it missed, reloading is
  the answer. The nonce is what tells two dev servers apart: a restarted
  one counts from 1 again.

Why does the router add these, and not the proxy, which sees every
answer? To add them, the proxy would have to hold back each HTML answer,
find `<body>` in it, and fix up `Content-Length`; and it would also have
to rewrite inside Datastar's event streams, because a `Rerender` replaces
the whole `<body>` ("A request" above) and would wipe out a box that
only the first page load had. The router already knows which answers are
pages and which are body patches, and it passes both through `dev`, so
a patched body keeps the box and the subscriber. The proxy stays a plain
pass-through. Run the executable without `alder run` and nothing sets
`ALDER_RELOAD`, so its pages are exactly as before.

**The events.** `/_dev/reload` (`serveReload`) is a server-sent event
stream that stays open. It starts with a comment line, `: ready` (a line
starting with `:` is a comment in the format, ignored by the browser),
then the reload if the page's generation is not the current one, or else
the overlay if there is an error right now, then whatever is
broadcast, and a `: ping` comment every 15 seconds so idle connections
are not dropped. Every event is made by one function in
`rt/datastar/datastar.go`, the same one the router's `PatchBody` uses:

```go
func PatchEvent(selector, mode, elements string) []byte {
	var b strings.Builder
	b.WriteString("event: datastar-patch-elements\ndata: selector " + selector + "\ndata: mode " + mode + "\n")
	for _, line := range lines(elements) {
		b.WriteString("data: elements ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return []byte(b.String())
}
```

and `run.go` defines the three:

```go
var (
	clearEvent  = datastar.PatchEvent("#_alder_errors", "inner", "")
	reloadEvent = datastar.PatchEvent("body", "append", `<script data-effect="el.remove()">location.reload()</script>`)
)

func overlayEvent(msg string) []byte {
	return datastar.PatchEvent("#_alder_errors", "inner", overlayHTML(msg))
}
```

Here are the real bytes, captured with `curl -N` on
`/_dev/reload?b=afcb2a7d-1` (the generation of the page above) while
`alder run examples/todos` ran. Introducing an error in
`app/Layout.alder` (`<title>{1}</title>`, an `Int` where text belongs)
sent:

```
: ready

event: datastar-patch-elements
data: selector #_alder_errors
data: mode inner
data: elements <pre class="_alder-errors">app/Layout.alder:10:15: error[E0003]: expected Text, Html or List Html in `{…}`, got Int
data: elements         &lt;title&gt;{1}&lt;/title&gt;
data: elements                 ^
data: elements   fix: Text.fromInt (1)</pre>

```

`mode inner` replaces what is inside `#_alder_errors`, the empty box,
with the `<pre>`. The diagnostic is the one `alder check` prints, run
through `html.EscapeString` (`overlayHTML`), which is why `<title>`
arrives as `&lt;title&gt;`: your source is shown as text, never run as
HTML. Each line of it is its own `data: elements` line, because a line
end inside a data line would end it. A second `curl` opened while the
error stood got the same event straight after `: ready`, and the page
itself still answered 200 from the old child. Fixing the file then sent
these two events, one right after the other:

```
event: datastar-patch-elements
data: selector #_alder_errors
data: mode inner
data: elements 

event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">location.reload()</script>

```

The first event is the clear: `data: elements ` followed by nothing (the
line ends in a space) replaces the box's content with nothing. The second
appends a `<script>` to `<body>`. Datastar runs a script it patches in, so
the browser executes `location.reload()` and asks the dev server for the
page again, which the new child now answers. (`data-effect="el.remove()"`
removes the element once Datastar has seen it.) Datastar 1.0's events
patch elements or signals; it has no "run this script" event, so a
patched-in script is how a server makes the browser do something. The
clear goes first, so even a page that misses the reload stops showing the
old error. An edit with no
error, and a change under `static/`, send the same reload event; the
first also sends the clear before it. The new app is generation
`afcb2a7d-2`, so a third `curl` on `/_dev/reload?b=afcb2a7d-1`, opened
after the fix, the way a page that missed all this would reconnect, got
the reload at once:

```
: ready

event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">location.reload()</script>

```

The 502 page the dev server serves while there is no child carries the
same two elements, with the error already in the box and `b` set to the
current generation (empty before the first build), so it too reloads
by itself once a build succeeds, whether it hears the broadcast or
reconnects after it. Since there is then no app to serve the
Datastar script it loads, the dev server serves
`/_alder/datastar-v1.0.0.js` (`datastar.ScriptPath`) itself in that state.

**Stopping.** On Ctrl-C, `runRunTo` closes `done`, which ends the
watcher, the worker's wait, the monitors and every open `/_dev/reload`
stream; shuts down the HTTP server (5 seconds for requests in flight);
waits for a rebuild in progress to finish, so the child it may have
started is not left behind; and then stops the current child.

**When the dev server dies.** None of that runs when the dev server is
killed with `SIGKILL` (which a process cannot catch), and a child left
running would keep its port and its database open with no one to stop it.
So `startChild` gives each child a pipe as its standard input: it makes
one with `os.Pipe()`, passes the read end as `cmd.Stdin` (closing its own
copy once the child has started), keeps the write end in `child.stdinW`,
and adds `ALDER_EXIT_ON_STDIN_EOF=1` to the child's environment. Nothing
is ever written to the pipe. `stop` closes the write end after its
`SIGTERM`, and when the dev server's process ends any other way, the
kernel closes every file it had open, the write end too. The generated
`main` (chapter 11, "`alder build`") waits for that, right after it sets
up its signal handling:

```go
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if os.Getenv("ALDER_EXIT_ON_STDIN_EOF") == "1" {
		os.Unsetenv("ALDER_EXIT_ON_STDIN_EOF") // not inherited by a process the app starts
		// alder run holds the other end of stdin: when it dies, this read ends and the app stops
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			stop()
		}()
	}
```

`io.Copy` reads until end of file, which a pipe reports once every write
end is closed. `stop` is the function `signal.NotifyContext` returned:
it cancels `ctx`, which is exactly what a `SIGTERM` does, so the app
shuts down the same graceful way (requests in flight get 5 seconds).
The variable is cleared once read: a process the app starts inherits
the app's environment, and its standard input is not the dev server's
pipe. Without the variable the app never reads its standard input, so a
server started with it closed (`< /dev/null`) keeps serving; a test in
`cmd/alder` starts the built `examples/todos` that way and checks it is
still listening a second later. `alder test` uses the same technique for
the test program (`ALDER_TEST_STDIN_EOF`, "Running tests" below).

### The scratch runner

`alder eval` (chapter 11) answers "what is `Todo.countOpen []`?" with
the compiler itself, not with an interpreter that could disagree with it.
It writes the expression as one more module of the app, compiles the app
with that module through every stage of this chapter (parse, names,
check, IR, Go, `go build`), runs the executable, and prints what it
prints. The engine is `internal/scratch`, and its contract is the doc
comment of `Compile`:

```go
// Compile builds the app at appDir with extra (nil for none) added as one more module, and the
// caller's Go command mainGo (package main, importing "app"), into the output directory out;
// out "" is build/<app>/scratch/<purpose> under the working directory (purpose "" is "default").
// It is Check followed by CompileChecked.
//
// The extra module implicitly imports every app module except App (which needs no import)
// and those its source already imports: Compile inserts `import M` lines after the source's
// `module` line, or, when it has none, prepends `module <Name>` and the imports; unused
// implicit imports are not reported. LineOffset says how many lines were inserted.
//
// What callers may assume:
//   - Diagnostics are returned, never printed. An app diagnostic's Span.File is the file under
//     the app ("app/Todo.alder"); the extra module's is "<Name>.alder". Diags may hold warnings
//     alone and still produce a binary.
//   - Binary is "" when Diags hold an error or a typed hole (W0002), or when Err is set. Err is
//     set only for I/O failures, an extra module named like an app or stdlib module
//     (*ModuleExistsError), adapters that are refused or do not compile with the app
//     (*AdapterError: the user's, as a check error is), generated or caller Go that does not
//     vet, and go build failures; its message carries Go's output.
//   - Compile writes only under out (the generated module "app" with its caps/ when it wires
//     the adapters (Checked.WiresAdapters), adapters/, static/ and
//     migrations/ copied from the app, cmd/main.go and the executable out/scratch) and the
//     lock file <out>.lock; it never touches build/<app> itself. Nothing is written when the
//     check fails, and the previous executable in out stays until a new one is built, so a
//     failed Compile leaves the last good one runnable.
//   - Compiles into one out serialize: Compile takes an exclusive lock on <out>.lock (flock;
//     the kernel drops it if the process dies) before writing, waiting while another holds it
//     (CompileCheckedWith's ctx ends the wait; its Options.Waiting is told), and a Runnable
//     Build keeps it until Close, so no other Compile replaces the executable while the caller
//     runs it. A Build that is not Runnable holds no lock. Callers must Close a Runnable Build.
//   - Compiles into the same out reuse Go's build cache (a second build of an unchanged app is
//     much faster).
func Compile(appDir string, extra *Module, mainGo string, purpose, out string) Build
```

Here is `alder eval examples/todos 'Todo.filterLabel Active'`, step by step
(`cmd/alder/eval.go`):

1. **The scratch module.** `parseItems` (`cmd/alder/evalinput.go`)
   splits the input into its items, the `let`s and the expression (a
   `let`'s name must start with a lowercase letter or `_`: `let Todo = 1` would
   otherwise be shadowed by the module `Todo`), and
   `assemble` joins them into the source of module `Scratch`, line for
   line (two steps, so that a REPL can keep the `let`s of earlier
   entries and assemble them again with each new one): the expression's
   first line
   becomes `it = Todo.filterLabel Active`, and each `let name = expr` before it a
   top-level `name = expr` (an annotated `let name : T = expr` becomes the
   two lines `name : T` and `name = expr`, which is why eval keeps a map
   from each line it wrote back to the input's). Where each item starts
   is `snippets.Starts` (`internal/snippets`): a line at column 1 that is
   not blank, a comment, or a closer such as `}` or `</ul>`; the book
   verifier uses the same rule. None of the values has a signature.
2. **Check.** `scratch.Check` adds `module Scratch` and an `import` of
   every app module but `App` (a name an app or stdlib module already
   has is an error, `ModuleExistsError`; `--module` picks another name), and checks the program with `check.Options{InferModule:
   "Scratch"}`: that module's values without a signature are inferred
   instead of being E0007, and it may use any module's brand
   constructors, as a `test` may. A function may call itself: while its
   body is inferred, its own name has a fresh type variable, unified
   with the body's type at the end (let-rec); a value defined after it is
   not known yet, and using one is E0007. Diagnostics are mapped back
   through the inserted lines (`MapDiag`, which also maps the line an
   E0009 message names) and the rewriting of step 1, and come out as
   `input:1:18`.

   `--in Todo` changes what step 2 builds. There is no `Scratch` module:
   the file `app/Todo.alder` is overlaid with its own text plus the
   session, appended (a newline is added first when the file lacks one),
   so the session is simply the end of `Todo`. `Build.BaseLines` records
   how many lines are the module's own, which is how a line of the
   overlay is told from a line of the session. `InSession` says whether a
   diagnostic is about the session, and `MapDiag` leaves one in the
   module's own lines alone, still `app/Todo.alder:L:C`, so that a
   mistake in the module is not shown as a mistake in your input.
   `Module.In` must name an app module exactly; anything else is a
   `NoModuleError`.
3. **Choose a main.** The inferred type of `Scratch.it` picks the Go
   `main`, which `scratchMain` (`cmd/alder/scratchmain.go`) writes in one
   of three kinds. `mainPure` prints `rt.Show(app.Scratch__it)`, the
   value written as Alder (a polymorphic `it` is a generic thunk,
   instantiated at `any`); `alder eval` uses it for every value but a
   `Query` or a `Task`. `mainQuery` runs `it` as a Query against the
   database named by `ALDER_EVAL_DB`; eval uses it only with `--run`
   (`build/<app>/dev.db`), and otherwise prints `<Query>` with its type.
   `mainTask` runs it as a Task, whose writes commit; eval refuses a
   `Task`, and the REPL's `:run!` is what uses it. The two effect mains
   are one template; `scratchMain(mainQuery, true, "app.Scratch__it")`
   (an app with capabilities and a `.go` file somewhere under `adapters/`,
   `Checked.WiresAdapters`) imports `"app/caps"`, opens the database,
   creating it with the app's tables when absent, and then runs (from
   `build/todos/scratch/eval/cmd/main.go`, after
   `alder eval examples/todos --run 'Db.count (Db.from Todo.Todo)'`)

   ```go
   env := &eff.Env{Ctx: ctx, Store: store, Session: app.TestSession(), Caps: caps.Caps().Registry(), Now: time.Now}
   v, err := eff.RunQuery(env, app.Scratch__it)
   ```

   with the app's default session (`TestSession`, the one `alder test`
   uses; only a test build has it, so eval and the REPL build with
   `scratch.Options{Tests: true}`, below) and the adapters' capabilities,
   wired by the generated `caps` package as in `alder build` (chapter 7);
   before compiling, the build checks the adapters with
   `buildkit.CheckTestAdapters`: `alder build`'s check, whose refusals are
   the build's error (a `scratch.AdapterError`, exit 1, with the same
   lines), except that a missing adapter is left unwired (out of the
   `caps` package) and listed once, as a warning (`Build.Warning`), so
   eval goes on. Without adapters the
   main does not import `app/caps`, `caps/` is left out of the scratch
   module, and `Caps` is `app.Caps{}.Registry()`, every capability
   unwired: a call to one fails with "capability … is not wired in eval"
   (as does a call to one whose constructor returned `nil`). A failed effect
   prints its message to stderr and exits 1.
4. **Compile.** `scratch.CompileCheckedWith` builds what step 2
   checked, without checking again: it takes the lock file
   `build/todos/scratch/eval.lock`, lowers and emits the whole app plus
   `Scratch` into `build/todos/scratch/eval/` as a test build (its
   `Options.Tests`: the app's tests, `TestSession` and fakes are
   compiled in, as for `alder test`; eval is a developer tool, never
   deployed), and builds it. The
   returned `Build` keeps the lock until `alder eval` closes it after
   step 5, so another evaluation cannot replace the executable while
   this one runs it. Taking the lock first *tries* it
   (`flock(LOCK_EX|LOCK_NB)`, which fails at once instead of waiting
   when another process holds it); only then does eval print `alder
   eval: waiting for another alder eval of this app…` (the
   `Options.Waiting` function it passes) and try again every 100 ms. A
   blocking `flock` could not be interrupted, and the retries can: each
   checks the context first, which Ctrl-C cancels. The same context goes
   to `go vet` and `go build` (`exec.CommandContext`), so Ctrl-C during
   a build kills it. The scratch module's Go is one line:

   ```go
   // Code generated by alder; DO NOT EDIT.

   package app

   var Scratch__it string = Todo__filterLabel(Todo__Filter__Active__)
   ```

   and `cmd/main.go` is

   ```go
   func main() { fmt.Print(rt.Show(app.Scratch__it)) }
   ```

   Every compile goes into the same directory, so Go's build cache has
   the app's runtime packages from the last one. Before `go build`,
   `CompileCheckedWith` runs `go vet` on the generated module, so Go
   that the emitter got wrong is reported as an internal error rather
   than as whatever `go build` makes of it. Measured on
   `examples/todos`, warm (the median of five or six runs, from typing
   the line to the printed value):

   | | time |
   |---|---|
   | `alder eval`, a new expression (vetted) | 0.63 s |
   | `alder eval`, the expression of the run before | 0.22 s |
   | REPL, its first evaluation (vetted) | 0.6 s |
   | REPL, a new expression (no vet) | 0.47 s |
   | REPL, the expression of the line before | 0.10 s |
   | REPL, `:type` or `let` (a check alone) | a few ms |

   Vet saves about 0.15 s of a new expression; most of the rest is Go
   compiling the generated package `app` again, which holds the scratch
   module and so changes with every new expression, and relinking the
   program. The same expression again finds everything in the cache.
   `scratch.CompileCheckedWith(ctx, …, scratch.Options{NoVet: true})`
   skips vet, for the REPL, whose app was vetted when it was built; if
   `go build` then fails, it vets after all, so the error says the same
   as with vet (the generated Go is an internal error, the main's or the
   adapters' is theirs). `alder eval` keeps vetting: it is what agents
   and the book verifier run, and there correctness is worth the 0.15 s.
5. **Run.** `scratch.RunTethered` runs the executable in its own process
   group, so the terminal's Ctrl-C reaches `alder eval` alone; eval
   catches it (and SIGTERM), cancels the context, and the whole group is
   killed, as it is after `scratch.Timeout`. (Its standard input is a
   pipe that ties the program to eval; "The REPL" below explains why.)
   `classifyRun` turns a failure into one message, shared with the
   REPL: a stack overflow is one line instead of Go's hundreds, a panic
   its first paragraph, a timeout `the evaluation timed out after 1m0s`.
   Then `alder eval` prints the output with the type: `"Active" : Text`.

A captured session, from a fresh working directory:

```
$ alder eval examples/todos 'Todo.countOpen []'
0 : Int
$ alder eval examples/todos 'Db.from Todo |> Db.count'
<Query> : Query Int
$ printf 'let xs = [3, 1, 2]\nList.sortBy (\\x -> x) xs\n' | alder eval examples/todos -
[1, 2, 3] : List Int
$ alder eval examples/todos 'Todo.filterLabel 1'
input:1:18: error[E0003]: expected Todo.Filter, got Int
  Todo.filterLabel 1
                   ^
$ alder eval examples/todos --json 'Todo.filterLabel Active'
{"value":"\"Active\"","type":"Text","diagnostics":[]}
```

**The book verifier** (`go test ./book`, chapter 11) runs every block of
this book through the same pipeline, from the outside: it builds the
`alder` binary once and drives it. `book/snippets.go` extracts the fenced
`alder` blocks and classifies each by its first line of code: a module
(`module …`), declarations (a declaration keyword, `name : T` or
`name args = …` at column 1), a table of signatures (`List.map : …`
lines), or expressions. Declarations are wrapped the way
`scratch.Compile` wraps a scratch module, as `module Snippet<n>` with an
import of every example module but `App`, and a signature `Name.x : T`
becomes `sig1 : T` and `sig1 = Name.x`, so the checker compares the
book's type with the real one. A module shown piece by piece (`part=`
blocks, chapter 11) is joined first. All of a chapter's modules,
declarations and signature tables go into one copy of `examples/todos/app` and one
`alder check --json`; only when that is not clean is each block checked
alone, to pin every diagnostic on its block and line. Expressions are
only parsed, with `alder dump` (each starting where `snippets.Starts`
says), unless the fence gives `eval=` or `error=`: then they are
evaluated with `alder eval --json -` in a working directory shared by
all chapters, where the lock above makes the chapters take turns. An
`error=` block passes only when the diagnostic is in the block itself
and, when the fence quotes a part of the message, its message has it. Chapters run in
parallel, each in its own temporary directory; the whole book takes
about 15 seconds, almost all of it the evaluations.

### Running tests

`alder test` (chapter 10) is the scratch runner with no scratch module:
`cmd/alder/test.go` calls `scratch.CompileCheckedWith` with purpose
`"test"` and `scratch.Options{Tests: true}`, so the app is compiled as
it is, into `build/<app>/scratch/test/`, under the lock
`build/<app>/scratch/test.lock`, as a test build. What makes it a test
program is what the compiler emits for `test` declarations, which only
a test build has (`gen.Options.Tests`; a production build, `alder
build`'s, emits none of what this section shows), and the main.

**Each test is a function.** Section 7's emitter turns the N-th `test`
of module M into `test__M__N`. `Todo`'s first test,
`test "countOpen ignores done todos" = countOpen [ … ] == 2` (three todos,
one done), is emitted into `build/todos/scratch/test/Todo_alder.go` as

```go
// test__Todo__1 is test "countOpen ignores done todos" of module Todo.
func test__Todo__1() bool {
	return rtesting.Eq(Todo__countOpen([]Todo__Todo{Todo__Todo{Id: "a", Title: "x", Done: false, Due: rt.Nothing[rt.Date](), Owner: "u", Created: std.TimeEpoch()}, Todo__Todo{Id: "b", Title: "y", Done: true, Due: rt.Nothing[rt.Date](), Owner: "u", Created: std.TimeEpoch()}, Todo__Todo{Id: "c", Title: "z", Done: false, Due: rt.Nothing[rt.Date](), Owner: "u", Created: std.TimeEpoch()}}), int64(2))
}
```

The `==` is not `rt.Eq`, as it would be anywhere else, but
`rtesting.Eq`: the same comparison, which also records both sides as
`rt.Show` text, so that the report can print `left:` and `right:`. Only
one `==` is recorded: the test's body when it is an `==`, or, in an
effect test, the value of the `Task.succeed` that ends its block
(`recordEq` in `internal/ir/lower.go`). An effect test's function
returns an `eff.Task[bool]` instead of a `bool`, built like any other
task (section 8, "A `Task`: `Toggle`").

**The table of tests.** `app__alder.go` lists them, with the session a
test starts from:

```go
// Tests are the app's `test` declarations in module order then declaration order, for alder test (rtesting.Main).
var Tests = []rtesting.Case{
	{Module: "App", Name: "Logout clears the user and goes to log in", Task: test__App__1},
	{Module: "Auth.Login", Name: "an unknown email is rejected", Task: test__Auth__Login__1},
	{Module: "Auth.Login", Name: "logging in through the form", Task: test__Auth__Login__2},
	{Module: "Auth.Login", Name: "a visitor is sent to log in, and can log out", Task: test__Auth__Login__3},
	{Module: "Todo", Name: "countOpen ignores done todos", Pure: test__Todo__1},
	{Module: "Todo", Name: "Active keeps only open todos", Pure: test__Todo__2},
	{Module: "Todo", Name: "each filter has its label", Pure: test__Todo__3},
	{Module: "Todo", Name: "a user with todos cannot be deleted", Task: test__Todo__4},
	{Module: "Todos.Index", Name: "Toggle flips done", Task: test__Todos__Index__1},
	{Module: "Todos.Index", Name: "Toggle by another user is not found", Task: test__Todos__Index__2},
	{Module: "Todos.Index", Name: "Delete by another user is not found", Task: test__Todos__Index__3},
	{Module: "Todos.Index", Name: "adding a todo through the page", Task: test__Todos__Index__4},
	{Module: "Todos.Index", Name: "toggling a todo through the page", Task: test__Todos__Index__5},
	{Module: "Todos.Index", Name: "a stranger cannot see my todo", Task: test__Todos__Index__6},
	{Module: "Todos.Index", Name: "Delete removes the todo", Task: test__Todos__Index__7},
	{Module: "Todos.Index", Name: "the newest todo is listed first", Task: test__Todos__Index__8},
	{Module: "Todos.Index", Name: "Db.mine is what Me sees", Task: test__Todos__Index__9},
	{Module: "Todos.Show", Name: "a visitor is sent to log in from a todo", Task: test__Todos__Show__1},
	{Module: "Todos.Show", Name: "saving changes the title and the due date", Task: test__Todos__Show__2},
	{Module: "Todos.Show", Name: "deleting from the todo's page", Task: test__Todos__Show__3},
}

// TestSession is the session a test runs with: App's session with every field at its default (nil without an App module).
func TestSession() any {
	return session__codec{}.Default()
}
```

A `Case` has `Pure` (a `func() bool`) or `Task` (a `func() eff.Task[bool]`),
never both. `alder build` emits neither the tests nor the table: the
production module has no `Tests`, no `TestSession` and no `test__`
function, and does not import `alder/rt/testing` (`TestGen_ProdHasNoTests`
in `internal/gen/gen_test.go` checks it).

**The main.** `testMain` in `cmd/alder/test.go` writes the scratch
program's `cmd/main.go`. For the to-do app, which has adapters
(`build/todos/scratch/test/cmd/main.go`):

```go
package main

import (
	"os"

	rtesting "alder/rt/testing"
	"app"
	"app/caps"
)

func main() {
	caps := caps.Caps().Registry()
	os.Exit(rtesting.Main(app.Tests, app.Tables, app.FakeTables, app.Migrations, caps, app.TestSession(), app.Routes, app.Web(), os.Args[1:], os.Stdout, os.Stderr))
}
```

`app.Migrations` are the app's migrations ("Migrations", below),
`app.FakeTables` the tables of its fakes ("Fakes", below; the to-do
app's one fake, `Password`'s, has none, so its test build has
`var FakeTables = []db.TableDesc{}`), and `app.Routes` and `app.Web()`
the app's routes and router options, for the browser ("The browser",
below).
(The local `caps` takes the package's name from that line on, which Go
allows: `caps.Caps()` on the right is still the package.) A capability
whose adapter is missing has no line in the test build's `caps` package
(`gen.Options.Unwired`), so its field is `nil`, and `alder test` prints
the warning `Build.Warning` holds before running the tests. Without
adapters the main does not import `app/caps` and the capabilities are
`app.Caps{}.Registry()`: every
capability's key is there with no implementation, and a call to one
fails with "capability … is not wired", which the report turns into
"… is not wired in tests". `alder test` runs the program with
`scratch.RunStream`, which, unlike eval's `RunTethered`, hands the
program's output over as it is written instead of collecting it, so each
test's line appears as the test finishes. The runner passes an
environment, not arguments, so `alder test` hands its settings over as
variables, which `rtesting.Main` reads:

- `ALDER_TEST_RUN` and `ALDER_TEST_JSON=1`: `-run` and `--json`, as its
  flags' defaults.
- `ALDER_TEST_DEADLINE` (an RFC 3339 time) and `ALDER_TEST_TIMEOUT`:
  `-timeout` (default `5m`) as a moment, and the duration for the
  message. `Main` runs each case in a goroutine of its own and waits for
  it, for the deadline, or for a ten-second tick (which prints
  `still running M "name" (10s)` on standard error). At the deadline it
  reports the running case as `timed out after 5m0s`, prints the summary
  and returns 1 without waiting: `os.Exit` in the main then stops the
  goroutine. A task case's `context` carries the deadline too, 100 ms later, so a
  database step waiting on something stops then, and `Main`'s timer,
  not the step's error, reports the timeout.
- `ALDER_TEST_STATUS`: a file. Before each case `Main` writes
  `Module<tab>Name` to it, and it empties it when it returns. A Go panic
  in a test is recovered and fails that test, but some failures cannot
  be recovered: a stack overflow is a fatal error that ends the process
  with exit code 2 and a long dump on standard error. When the program
  exits with anything but 0 or 1, `alder test` reads the file to learn
  which test was running, prints a `FAIL` for it (an object under
  `--json`) whose `error:` is the first lines of the dump (the
  `fatal error:` line and the top frame of the running goroutine), then
  the summary line the program never printed (it counts the results it
  passed through, and this failure), and exits 1. It passes the program's standard error through line by line
  until a line that starts a Go crash report (`fatal error:`, `panic:`,
  `runtime:`, `goroutine …`), and keeps the rest, so the dump is never
  printed.
- `ALDER_TEST_STDIN_EOF=1`: the program's standard input is the read end
  of a pipe whose write end only `alder test` holds. `Main` reads it in
  a goroutine and exits when it reaches end of file. The program runs
  in a process group of its own (so a terminal's Ctrl-C reaches
  `alder test` alone, which then kills the group), which means a
  `kill -9` of `alder test` would not reach it; but the kernel closes a
  dead process's files, the pipe's write end among them, so the program
  sees end of file and exits too.

`alder test` waits five seconds past the deadline (`testGrace`) before
it kills a program that has not stopped by itself, and then reports the
running test as timed out.

**One world per test.** `rtesting.Run` (`rt/testing/testing.go`) runs
the cases one after another (`Main` does the same, one goroutine per
case, as above). A pure case is called, with a `recover` so
a Go panic fails that test and the next one still runs. A task case
gets its own environment: `db.OpenSQLite(":memory:")`, a database that
exists only in this process; the app's tables, made by `store.Migrate`
with the app's migrations when it has them and by `store.Ensure` with
its `Tables` when it has none; then the fakes' tables
(`store.CreateTables`, "Fakes", below); and an `eff.Env` with a copy of
the capabilities (`maps.Clone`, so a `Test.fake` changes only this
test's), the session from
`TestSession()`, and a clock that starts at `2026-01-01T00:00:00Z` and
moves one second on each read (`rtesting.Ticking`), so rows a test
inserts get distinct, ordered `created` times.
Then `eff.Run(env, eff.KTask, c.Task())` runs it exactly as the server
runs an `update` (section 8, "What `Run` does"): one transaction, begun
by the first database step, committed when the task succeeds. The
report's trace is `env.Trace`. Then the database is closed, and with
`:memory:` that is the end of it.

**`Test.run` is a step.** Section 6 lowers `Test.run t` to a node of its
own, and the emitter makes it a `Call` named `"Test.run"`, marked as a
write, whose function hands `t` to `rtesting.Inner` and packs the five
results into the record Alder sees. From `test__Todos__Index__1`
(re-indented):

```go
	return eff.Bind(eff.Call("Test.run", true, func(_e5 *eff.Env) (struct {
		Value rt.Result[string, rt.Nav]
		Trace []string
	}, error) {
		_v6, _msg7, _ok8, _tr9, _err10 := rtesting.Inner(_e5, Todos__Index__update(Todos__Index__Msg__Toggle__(Todos__Index__ToggleParams{Id: id}), Todos__Index__Model{Filter: Todo__Filter__All__, Todos: []Todo__Todo{}}, Todos__Index__Session{User: me}))
		if _err10 != nil {
			var _z12 struct {
				Value rt.Result[string, rt.Nav]
				Trace []string
			}
			return _z12, _err10
		}
		_r11 := struct {
			Value rt.Result[string, rt.Nav]
			Trace []string
		}{Trace: _tr9}
		if _ok8 {
			_r11.Value = rt.Ok[string, rt.Nav](_v6)
		} else {
			_r11.Value = rt.Err[string, rt.Nav](_msg7)
		}
		return _r11, nil
	}), func(r struct {
```

`Inner` is where the savepoint lives. Its doc comment is its contract:

```go
// Inner runs t under a savepoint as Test.run does: ok is true with v on success; on *eff.ErrFail,
// eff.ErrNotFound or *eff.ErrCapability (a capability's adapter returned an error) the
// savepoint is rolled back, the session restored, and msg is the failure text ("not found" for
// ErrNotFound, "<Name>: <msg>" for a capability's error); trace is the steps t performed. Any
// other error is returned.
//
// Inner is called from a Call's function while a Task runs (env.Kind is KTask); it begins the
// Task's transaction if no step has yet, so the savepoint has a transaction to live in. When
// env.Store does not implement db.Savepointer (or there is no store) t runs without one and a
// failure undoes nothing in the store. On an error other than a failure the savepoint is
// still rolled back, so the store is consistent, and the error is returned for the enclosing
// Run to fail the test with. A panic in t is not recovered: it unwinds through the enclosing
// Run, which rolls back the whole transaction.
func Inner[T any](env *eff.Env, t eff.Task[T]) (v T, msg string, ok bool, trace []string, err error)
```

A capability's error reaches `Inner` as an `*eff.ErrCapability`
because the emitter wraps it: a capability `Call`'s function ends
`_v, _err := _c.Verify(_e.Ctx, …); return _v, eff.CapError("Password.verify", _err)`.
`CapError` leaves `nil`, `*eff.ErrFail` and `eff.ErrNotFound` as they
are and wraps any other error with the call's name; the router answers
it like any other error, with a 500.

`eff.RunInner` (`rt/eff/run.go`) runs `t` with the same `Env`, inside
the running task: same transaction, same trace (which is why the inner
steps also appear in the test's trace, after `"Test.run"`). `Inner`
copies the part of the trace added since it started; that is `r.trace`.
The savepoint itself is three SQL statements on the open transaction,
in `rt/db/sqlite.go`: `SAVEPOINT alder_t1` before `t`, then
`RELEASE alder_t1` when `t` succeeded, or `ROLLBACK TO alder_t1` and
`RELEASE alder_t1` when it failed (`ROLLBACK TO` undoes the writes but
keeps the mark open, so it is released too). Each `Inner` takes the next
number from a counter, so nested and successive savepoints never share a
name. `Test.trace` is a smaller `Call` whose function returns
`rtesting.Trace(env)`: the trace so far without its own step.

### The REPL

`alder repl` (chapter 11) is the scratch runner kept open. `alder eval`
builds one scratch module from its input and throws it away; the REPL
keeps a list of the `let`s you typed and builds a new scratch module
from that list every time you type a line. The code is
`cmd/alder/repl.go`, and nothing in it is new compiler machinery: it
calls the same `parseItems`, `assemble`, `scratch.Check`,
`scratch.CompileCheckedWith`, `scratch.RunTethered` and `classifyRun` as
`alder eval`.

**The session is a list of items.** An *item* (`cmd/alder/evalinput.go`)
is one `let` or the expression, already rewritten as the lines it
contributes to a module: `let xs = [3, 1, 2]` becomes the top-level
definition `xs = [3, 1, 2]`, and the expression `total * 2` becomes
`it = total * 2`. The REPL keeps the kept `let`s as a slice, each with
the REPL line it was typed on. To evaluate `total * 2` after the session
of chapter 11, it appends the expression's item and joins them:

```text
xs = [3, 1, 2]
total = List.sum xs
  + 10
it = total * 2
```

`scratch.Check` puts `module Scratch` and an `import` of every app module
above that, and checks it with the app, exactly as for `alder eval`. The
signatures are inferred (no `let` needs one), and the type of `it` is
the type the REPL prints. A `let` is checked the same way, as the kept
`let`s plus the new one, with no `it`. Only when that check is clean
does the new `let` join the list, so the list always checks. Defining a
name again replaces the old item where it stands, so the `let`s after
it that use the name now use the new definition: a `let` is a
top-level definition, not a snapshot of a value. That is also why `let
x = x + 1` does not mean "one more than the old `x`": it is a
definition of `x` in terms of itself, which the checker refuses (E0007),
and the old `x` stays. A redefinition that a later `let` no longer
checks with is refused the same way, and the message names that `let`:
`a not redefined: let b (line 2) uses it: …`.

One entry is always one item. `parseItems` would start a new item at a
continuation line that begins at column 1 (`+ 10` typed after `let total
= List.sum xs \`), so the REPL indents such a line by two spaces before
parsing, and records a shift of -2 for it so its columns still match what
you typed.

**Why every line re-checks everything.** The session could remember what
each `let` checked to and check only the new line. It does not, because
checking the whole app plus the session is cheap: on `examples/todos` a
check takes a millisecond or two, against half a second for a build. A
full check also means the REPL needs no state of its own about types.
The checker's answer is always about the app as it is on disk and the
session as it is now, so nothing can go stale.

**Mapping a diagnostic back.** A diagnostic of the scratch module names
a line of the joined source, after `scratch.Check` inserted its import
lines. `Checked.MapDiag` undoes the inserted lines, which leaves a line
of the joined items. The REPL walks the items' lengths to find which
item it falls in: an item's `InLine` numbers count from the start of the
entry it came from, so they cannot say which item a line is in. It then
subtracts where that item starts, and hands the diagnostic to
`evalDiags`, the printer `alder eval` uses, with the item's own text as
the source to show and underline. `evalDiags` labels the position
`input:L:C`, and the REPL rewrites that label: `expr:L:C` for the
expression, `let total (line 10)` for a `let` (its REPL line plus the
line within it). Errors are printed wherever they are, but warnings only
for the line just typed: an old `let`'s warning would otherwise repeat on
every line.

**Evaluating.** When the check is clean, the type of `it` decides. A
`Query`, `Task` or `Select` is shown with a hint unless the command
asked to run it (`:run` a `Query`, `:run!` a `Task`). Anything else is
built with the pure main, which prints `rt.Show(it)`.

The module that is built holds only the `let`s the expression uses.
Every `let` is a top-level value, which Go computes as a package
variable when the program starts, used or not; a session with a `let`
that fails (`let boom = loop 0`, with a `loop` that calls itself
forever) or takes a minute would make every
later line fail or take a minute. So `reachable` scans the expression's
text for identifiers that are names of `let`s, then the text of those
`let`s, and so on until nothing new turns up, and the REPL checks and
builds the session with just those (in their order). The scan is
textual, so it can take a `let` that is not really used (its name
inside a string, or a record field of the same name), which costs a
little time, but never misses one. When the run fails, the message ends
with the `let`s it computed (`(the lets it uses — loop, boom — are
computed with it)`), since the failure may be theirs.

The build goes into `build/<app>/scratch/repl-<pid>/`, a directory of
this REPL's own (`<pid>` is its process id), under its lock, held until
the program has finished. With one shared directory a second REPL on the
same app would wait on the lock for as long as the first one's
evaluation ran; with one each, nothing waits, and Go's build cache,
which is shared, keeps the first build in a new directory about as fast
as any other. The REPL removes its directory when it ends, on `:quit`,
end of input, SIGTERM or SIGHUP. A REPL killed with SIGKILL cannot, so
`alder repl` starts by removing every `repl-<pid>` directory whose
process no longer exists (`kill(pid, 0)`, signal 0, which only asks
whether the process is there). `:run` and `:run!` use the effect mains of "The scratch
runner": a `Task` main runs `eff.Run`, which commits the transaction when
the task succeeds and rolls it back when it fails, so a failed `:run!`
leaves the database as it was.

**The no-vet build.** `go vet` checks the generated Go before it is
built, so that an emitter bug shows up as an internal error and not as a
confusing build failure. The first build of a session is vetted, and so
is the first after a reload, so a bug in the generated Go is still
caught. Later builds pass `scratch.Options{NoVet: true}`. On
`examples/todos`, warm, the first evaluation takes about 0.6 s and later
ones about 0.47 s (the table in "The scratch runner"): each new
expression changes the generated package, so Go compiles and links it
again.

**`:reload`.** The app's files may have changed since the `let`s were
checked. `:reload` checks the app alone first; if that has errors, they
are printed and the session is left as it is. The REPL remembers the
app's errors it last printed and, while they stay the same, says only
that the app has errors on later lines, instead of the whole block each
time. Otherwise it rebuilds the
list: each `let` in order is checked with the ones kept so far and kept
if clean, else dropped with the first error (`dropped let greeting:
error[E0002]: …`). A `let` that used a dropped one is dropped too: it is
checked after it, without it, and fails with `unknown name`. It also
marks the next build to be vetted.

**The loop.** Reading a line from the terminal blocks, but the REPL also
has to notice an edited file or a Ctrl-C while it waits. So a goroutine
reads standard input with a `bufio.Reader` and sends each line on a
channel. A line longer than 1 MB is read to its end and dropped, and the
REPL says `line too long (limit 1 MB); ignored` and reads on; any other
read error is printed, and the REPL exits 1. A second goroutine runs `watch`, the polling
loop of `alder run` ("The dev server"), which sends on another channel
when `app/` or `adapters/` change. The main loop prints the prompt and
`select`s on the two channels and the interrupt channel:

- a line is handled;
- a change runs `:reload` (`app changed: session re-checked`);
- a Ctrl-C says how to leave.

A change that arrives during an evaluation waits in its channel and is
handled before the next prompt.

Ctrl-C arrives as a signal. `main.go` asks for it with
`signal.Notify(…, os.Interrupt)`, so it no longer kills `alder`; each
one becomes a value on the interrupt channel. During an evaluation a
goroutine waits on that channel and cancels the evaluation's context. As
for `alder eval`, the program runs in its own process group, out of the
terminal's reach, and `RunTethered` kills the group when the context is
cancelled. A Ctrl-C during the build cancels the same context, which
kills `go build`. The REPL prints `alder repl: interrupted` and keeps the
session. SIGTERM and SIGHUP close a second channel, `terminate`: the
evaluation is cancelled the same way, and the REPL exits 1.

A REPL killed with SIGKILL runs no code at all, so it cannot kill the
program's group, and the program, in a group of its own, gets no
signal either. So the program watches its parent another way. Its
standard input is a pipe (`scratch.RunTethered`) whose write end only
the REPL holds. When `ALDER_SCRATCH_STDIN_EOF=1`, the runtime package
`alder/rt` starts a goroutine in its `init` (`rt/tether.go`) that reads
standard input until end of file, then prints `alder: parent gone` and
exits 2. When the REPL dies, the kernel closes its end of the pipe, the
read returns, and the program exits. `init` clears the variable once it
has read it, so a process the program starts does not inherit it (that
process's standard input is not the tether). `alder eval` uses
`RunTethered` too.

Why in `rt`, and not in the scratch main? The generated package
computes a plain value, such as `it = List.sum …` or a `let`, as a Go
package variable, while the package initializes. Go initializes every
package a program imports before the main package's `init` runs. A
goroutine started in the main would therefore start only after a
never-ending evaluation had finished, which is never. `rt` is
initialized before the app's variables by a rule of the Go spec: a
package's imports are initialized before the package itself. So the
generated package imports `alder/rt` always. An app with a union already
uses it (`rt.RegisterUnion`); for one that does not, `app__alder.go` has
a blank import, which runs a package's `init` without naming anything in
it (`testdata/golden/gen_generic/want/app__alder.go`, a production
build, which imports nothing else; a test build adds
`rtesting "alder/rt/testing"`):

```go
import (
	_ "alder/rt" // initialized before this package (rt/tether.go)
	"alder/rt/db"
)
```

### Migrations

Chapter 8 shows migrations from the outside. Three pieces of code make
them: `internal/migrate` writes a migration, `alder build` puts the files
into the executable, and `db.Migrate` in `rt/db/migrate.go` applies them.

**Writing one.** `alder migrate` (`cmd/alder/migrate.go`) checks the app
and lowers it to the IR, as a build does. `migrate.FromProgram`
(`internal/migrate/schema.go`) turns the IR's tables into a `Schema`: one
`Table` per `table`, named `Post.Post` (module, then table), with its
columns in declaration order and each column's type, `Maybe`-ness,
constructors, `@unique`, `@ref`, delete rule and default as SQL text
(`False` is `0`). `schema.json` is that value written as JSON with the
tables sorted by name, and `Schema.Hash` is the same hash `Ensure`
records: SHA-256 over the sorted `CREATE TABLE` statements. A file's
`from` and `to` lines are the first 12 hex digits of the hashes on
either side of it.

`migrate.Diff(old, new)` (`internal/migrate/diff.go`) pairs the tables
(by name, else by `@renamed`), then the columns of each pair, and emits
steps in a fixed order, so the same change always gives the same file:

1. drop the tables that are gone;
2. rename the renamed ones;
3. create the new ones;
4. for each kept table, in name order: drop columns, rename columns, add
   columns;
5. rebuild the tables `ALTER TABLE` cannot change;
6. drop, then create, `@ref` indexes.

A table needs a rebuild when no `ALTER TABLE` gives it the new
`CREATE TABLE` text byte for byte: SQLite stores that text, and the
start-up check compares hashes of it. `ADD COLUMN` only appends a
nullable column without `UNIQUE`; anything else (a new column that needs
a default, a changed type, a reordered table) is a rebuild, which
carries the table's renames, drops and additions in one step.
`migrate.SQL` (`internal/migrate/sqlite.go`) writes each step's
statements. A rebuild is SQLite's documented procedure: create
`T__alder__new` with the new statement, `INSERT INTO … SELECT` one
expression per new column (the default for an added column, the old name
for a renamed one, a conversion such as `CAST("x" AS REAL)` for a changed
type, with a `WHERE` that keeps only the rows the new column can hold),
drop `T`, rename `T__alder__new` to `T`, and create its `@ref` indexes
again. A conversion that loses values sets the step's `DropsData`.
`migrate.Plan` is `Diff` plus `SQL` plus the hashes; `migrate.Render`
writes the file, and `migrate.NextName` numbers it.

**Building it in.** `alder build` copies `migrations/*.sql` (not
`schema.json`) into `build/<app>/migrations/`
(`buildkit.CopyMigrations`), and the generated `app__alder.go` embeds
them with Go's `//go:embed`, which makes files part of the compiled
program:

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrations are the app's migrations in order (db.ReadMigrations): Serve applies those the
// database has not had (db.Migrate).
var Migrations = db.ReadMigrations(migrationsFS)
```

(from `testdata/golden/gen_migrations/want/app__alder.go`). An app
without migrations gets `var Migrations []db.Migration`, and `Serve` calls
`Ensure` instead. `buildkit.SchemaDrift` is E0019: it compares
`FromProgram`'s hash with the snapshot's, so every command that checks
the app refuses one whose tables are ahead of its migrations.

**Applying them.** `db.ReadMigrations` parses each embedded file with
`db.ParseMigration` into a `Migration`: its name, its `from` and `to`
hashes, and its statements (the header stripped). `Serve` passes them to
`store.MigrateLog` (`rt/db/migrate.go`), which works on one connection
(`database/sql` otherwise hands each statement whichever pooled
connection is free, and a `PRAGMA` holds only for its own connection):

1. It sets `PRAGMA foreign_keys = OFF`, outside any transaction (SQLite
   ignores it inside one), and restores the old value when done. With
   it on, a rebuild's `DROP TABLE "Post__Post"` would delete every
   comment through `ON DELETE CASCADE`. `legacy_alter_table` stays off,
   so `RENAME TO` also rewrites the `REFERENCES` of other tables.
2. It opens one transaction for the read phase (`prepare`). On a file
   database every transaction starts with `BEGIN IMMEDIATE` (the DSN's
   `_txlock=immediate`), which takes the write lock at once, so two
   processes starting together take turns, and each sees the other's
   work whole: the recorded names, the tables and the recorded hash are
   read in one snapshot, never between another process's file and its
   record. (Read outside a transaction, a second process could see a
   file's tables before its record, call that drift, and in dev mode
   recreate a database full of rows.)
3. Inside it: `_alder_migrations` (name, time applied) is read. A
   database with app tables but no such table was made by `Ensure`: it
   is adopted at the file whose `to` is its hash (the files up to it
   recorded, in this same transaction), or refused (dev: recreated). A
   recorded name with no file is refused (the newest recorded name past
   every file means the executable is older than the database), and so
   is a pending file numbered before a recorded one; the live hash is
   compared with the one recorded after the last migration (drift); and
   the chain is checked: the first pending file's `from` is the live
   hash, each next `from` the previous `to`, the last `to` the app's
   hash. Any refusal rolls the transaction back, adoption included, so
   the database is as it was.
4. The read phase commits, and each pending file runs in its own
   transaction (`applyMigration`): it skips the file if another process
   recorded it meanwhile; checks that the live hash is the file's `from`;
   runs the statements; runs `PRAGMA foreign_key_check` (which lists
   rows whose reference points at nothing, since nothing was enforced
   during the statements); and checks that the live hash is now the
   file's `to`. Only then the row in `_alder_migrations` and the live
   schema and hash in `_alder_meta`, then `COMMIT`. A failure rolls that
   file back. The `to` check is what catches a file whose statements were
   edited after `alder migrate` wrote them (its header still chains, so
   `alder check` cannot see it): `DROP COLUMN "views"` under a header
   that says the column stays is rolled back with the rows intact, not
   committed and discovered at the next start.
5. Last (`finish`, in the transaction of the last applied file, or in
   the preparation's own when nothing was pending), the live hash must equal
   the app's, the app's schema is recorded, and `IndexStmts` creates
   every `@ref` index that is missing (`CREATE INDEX IF NOT EXISTS`, not
   part of the hash), so an adopted database gets its indexes too.

A start that finds the database locked for longer than `busy_timeout`
(five seconds: another process is in a long rebuild) is refused with a
text that names the cause (`lockedErr`). `OpenSQLite` itself retries its
first connection while another process holds a new database: turning a
file to WAL is refused at once, without waiting, while anyone holds it.

`rtesting` and the `eval`/REPL mains call the same function, so a test's
database and the dev database are made exactly as production's.

### Fakes

Chapter 10 shows a `fake` from the outside. Here is what each stage
does with chapter 10's `Payments` module, built by `alder test` as the
app `shop`.

**Parsing, names and checking.** `fake` is a declaration keyword, like
`capability` (column 1 only). The parser reads `fake Stripe { … }` into
a `FakeDecl` holding its `TableDecl`s and its entries, each a
`ValueDecl` with no signature (`internal/syntax`). Names
(`internal/names/resolve.go`) resolve `Stripe` to the capability, which
must be in scope, and register the fake's tables as tables of the
module, so `Payments.Charge` and, from another module,
`Payments.Charge.cents` resolve as an app table's would. The checker
(`internal/check/fakes.go`) reports E0020 when the capability is in
another module, then checks each entry as a value whose signature is
the entry's type in the capability (a missing or extra entry is E0003).
It records which tables belong to a fake, and E0021 is a check on every
use of such a table, its type or a column: allowed inside a `test`, or
inside the fake's own entries, and nowhere else. `Test.fake Stripe` is
typed by the checker itself, not by its signature in
`stdlib/Test.alder` (`fake : Capability -> Task Unit`, where
`Capability` is a name only that file may write): the argument must be
a capability with a fake, and the result is `Task Unit`.

**The IR.** `ir.Program.Fakes` holds one `ir.Fake` per fake: its
module, a pointer to the `ir.Capability` it implements, its tables, and
its entries as `ir.Value`s in the capability's entry order. The tables
are lowered by the same code as the app's, but kept out of
`Program.Tables`. Everything that reads `Program.Tables` therefore
never sees them: the schema, `migrate.FromProgram`, E0019 and the
production build. `Test.fake Stripe` lowers to an `ir.TestFake` node
naming the fake.

**A fake becomes a Go struct.** `gen.Emit` takes `Options.Tests`, which
`alder test` sets (`scratch.Options{Tests: true}`), and so do `alder
eval` and the REPL, whose mains need `TestSession`. With it, the
module's Go file gets the fake's table, as a record type and a
`db.Table` exactly like an app table's (`Payments__Charge`,
`table__Payments__Charge`), and then the fake itself. From
`build/shop/scratch/test/Payments_alder.go`:

```go
// fake__Payments__Stripe is `fake Stripe` of module Payments (test builds only): it implements
// Cap__Payments__Stripe by running the fake's entries inside the effect that calls them
// (eff.RunInner on the Env eff.EnvFrom finds in ctx), so their steps, writes and failures are the
// calling test's. Test.fake Stripe installs it.
type fake__Payments__Stripe struct{}

func (fake__Payments__Stripe) Charge(ctx context.Context, args Args__Payments__Stripe__Charge) (rt.Result[string, string], error) {
	env := eff.EnvFrom(ctx)
	if env == nil {
		var zero rt.Result[string, string]
		return zero, errors.New("fake Stripe called outside a running effect")
	}
	return eff.RunInner(env, fakeentry__Payments__Stripe__Charge(args))
}
```

`Refund` is the same with its own types. The struct has no fields: a
fake's state is in its tables, not in Go. It has one method per entry,
with the signature of the capability's interface,
`Cap__Payments__Stripe`, so to the rest of the program it is one more
implementation of that interface, like the adapter's. Each entry's body
is emitted as an ordinary function returning an effect (an entry with
no parameters, such as a `balance : Query Int`, is a package `var`):

```go
func fakeentry__Payments__Stripe__Charge(args struct{ Cents int64 }) eff.Task[rt.Result[string, string]] {
	return eff.Bind(eff.Call("Db.insert Charge", true, func(_e1 *eff.Env) (string, error) {
		return db.InsertRow(_e1.Ctx, _e1.Store.(db.Store), db.TxOf(_e1), table__Payments__Charge, insertvals__Payments__Charge(insert__Payments__Charge{Cents: args.Cents, Refunded: false}), _e1.Now())
	}), func(id string) eff.Task[rt.Result[string, string]] {
		if args.Cents > 100000 {
			return eff.Fail[rt.Result[string, string]]("card declined")
		}
		return eff.Pure(rt.Ok[string, string](id))
	})
}
```

The method is how the Go world (a capability call) gets back into the
Alder world (an effect to interpret). A capability call's `Call`
passes `_e.Ctx` to the method, and `eff.Run` put the running `Env` in
that context, so `eff.EnvFrom(ctx)` finds it. `eff.RunInner(env, t)`
(`rt/eff/run.go`) then interprets the entry's effect inside the effect
that is already running, under its kind: the same transaction, the same
trace, the same session. This is why the trace shows `Stripe.charge`
(the capability call's own step) followed by `Db.insert Charge` (the
entry's step), why the insert is part of the test's transaction, and
why a `Test.run` savepoint around the call undoes it. A failure, such
as `eff.Fail` with `"card declined"`, comes back out of `RunInner` as
the method's error. `CapError` leaves an `*eff.ErrFail` as it is, so the
test sees `card declined`, not `Stripe.charge: card declined`. Called
from a `Query`, the entry runs as a `Query`, and a write it reaches is
the usual internal error.

`app__alder.go` gets two variables:

```go
// FakeTables are the test-only tables of the app's fakes, which alder test's main creates in each
// test's database after Tables (rtesting.Options.TestTables). Test builds only.
var FakeTables = []db.TableDesc{table__Payments__Charge.Desc()}

// Fakes are the app's fakes by the Env.Caps key of their capability: Test.fake installs one in the
// running test's Env.Caps. Test builds only.
var Fakes = map[string]func() any{
	"Payments.Stripe": func() any { return fake__Payments__Stripe{} },
}
```

and `Test.fake Stripe` is a `Call` that puts a new fake under the
capability's key in the running test's `Env.Caps`, the map every
capability call looks its implementation up in (from
`test__Payments__1`):

```go
	return eff.Bind(eff.Call("Test.fake Stripe", false, func(_e1 *eff.Env) (struct{}, error) {
		if _e1.Caps == nil {
			_e1.Caps = map[string]any{}
		}
		_e1.Caps["Payments.Stripe"] = Fakes["Payments.Stripe"]()
		return struct{}{}, nil
	}), func(_ struct{}) eff.Task[bool] {
```

Each case's `Env.Caps` is its own copy of the app's capabilities
(`runCase` clones the map), so the next test starts with the adapter
again. A `Test.run` savepoint covers the database, not this map, so a
fake installed inside a failing run stays installed.

**The fakes' tables in a test's database.** `rtesting.Main` passes
`FakeTables` on as `Options.TestTables`. For each case, after the app's
tables exist (by `Migrate` or `Ensure`), `ensure` calls
`store.CreateTables(ctx, o.TestTables, o.Tables)` (`rt/db/sqlite.go`):
in one transaction it runs a plain `CREATE TABLE` for each fake table,
resolving its `@ref`s against both lists, and its indexes. It records
nothing in `_alder_meta`, where `Ensure` and `Migrate` keep the app's
schema hash. The order matters: `Migrate` ends by checking that the
live tables are exactly the app's, so it runs before the fakes' tables
exist, never after.

**Production.** Without `Options.Tests` (`alder build` and `alder
run`) none of this is emitted: no fake struct, no entry functions, no
fake table, no `FakeTables` or `Fakes`. Nor are the tests (section 8,
"Running tests"): no `test__` functions, no `Tests`, no `TestSession`,
so nothing in a production build uses a fake, a fake's table or
`Test.fake`, and nothing imports `alder/rt/testing`. The `Test` nodes
(`ir.TestRun`, `ir.TestTrace`, `ir.TestFake` and a test's recorded `==`,
`ir.TestEq`) occur only in `test` bodies, so the emitter treats one
reaching it in a production build as a bug in the compiler: `testNode`
in `internal/gen/effects.go` panics, and `Emit` returns that as its
internal error instead of writing Go.

### The browser

Chapter 10, section 9, shows the browser steps from the outside, and
section 10 there what a step does. Here is how each stage of the
compiler and the runtime makes that happen, from the to-do app's test
build (`build/todos/scratch/test/`, what `alder test examples/todos`
compiles).

**Checking.** The six steps are special forms, like `Test.fake`: the
checker types them itself (`internal/check/browser.go`), and their lines
in `stdlib/Test.alder` are documentation. `Test.get p` and `Test.open p`
need `p` written as a page value in place (a page module's name applied
to its parameters' record), so the checker knows which page it is and
can give `Test.get`'s `model` that page's `Model`. `Test.submit F r`
finds the form `F`, the page (or `App`) that declares it, and the one
`msg … form F` that posts it, and checks `r` against the form's record
the way a form value is checked, defaults included. `Test.click m`
needs a message of a page or of `App` that carries no form.
`Test.login s` checks `s` against `App`'s session. Each result is the
record type chapter 10 gives, with `Session` being `App`'s session type
and `Doc` and `Answer` types of the Prelude that the checker lets only a
test name (E0003 elsewhere). The queries are ordinary functions of
`stdlib/Test.alder`, with one extra check: a selector argument written
as a literal is parsed with the runtime's own selector parser
(`rt/selector`, shared by the checker and `rt/html`), and a text
that does not parse is E0023, at the literal.

**The IR.** Each step lowers to a node of its own (`internal/ir`):
`TestLogin{Session}`, `TestGet{Page, Params, Open}` (`Open` for
`Test.open`), `TestSubmit{Form, Msg, Page, Params, Record}`,
`TestSubmitRaw{…, Fields}` and `TestClick{Page, Msg, Params}` (`Page` is
nil for an `App` message). The queries stay ordinary calls.

**A step is a `Call`.** Section 8's `Call` is a named step with a
function to run; every browser step is one, named as the trace shows it,
marked as a write, whose function asks the test's browser to do the
step and packs its result into the record Alder sees. From
`test__Todos__Index__4`, "adding a todo through the page"
(`Todos__Index_alder.go`, re-indented), the `Test.submit` step:

```go
eff.Call("Test.submit NewTodo", true, func(_e13 *eff.Env) (struct {
	Answer  rtesting.Answer
	Session App__Session
	Doc     rtesting.Doc
}, error) {
	_r14, _err15 := rtesting.BrowserOf(_e13).SubmitForm("/_m/Todos.Index/Submit", "NewTodo", encode__Todos__Index__NewTodo(Todos__Index__NewTodo{Title: "Milk", Due: rt.Nothing[rt.Date]()}))
	if _err15 != nil {
		var _z16 struct {
			Answer  rtesting.Answer
			Session App__Session
			Doc     rtesting.Doc
		}
		return _z16, _err15
	}
	return struct {
		Answer  rtesting.Answer
		Session App__Session
		Doc     rtesting.Doc
	}{Answer: _r14.Answer, Session: rtesting.SessionOf[App__Session](_r14.Session), Doc: rtesting.DocOf(_r14.Doc)}, nil
})
```

The record literal is the form's Go struct, and `encode__Todos__Index__NewTodo`
turns it into the text the page's inputs would send. Every form of the
app gets one in a test build (`routes__alder.go`), the inverse of the
form's decoder (section 8, "A request"):

```go
// encode__Todos__Index__NewTodo is form NewTodo as the browser's inputs submit it, for Test.submit
// (test builds): the inverse of decode__Todos__Index__NewTodo.
func encode__Todos__Index__NewTodo(r Todos__Index__NewTodo) url.Values {
	v := url.Values{}
	v.Set("title", r.Title)
	if r.Due.Ok {
		v.Set("due", std.DateToText(r.Due.Val))
	} else {
		v.Set("due", "")
	}
	return v
}
```

`SubmitForm` takes the message's path, `/_m/Todos.Index/Submit`, and the
form's name for its error text. A `Test.submit (Rename { id: b }) …`
calls `SubmitMsg` instead, with the message's address including its
parameters. `Test.click (Toggle { id: id })` builds the message's
address the way a `click={…}` does, and a label for the error text:

```go
_m15 := Todos__Index__ToggleParams{Id: id}
_r16, _err17 := rtesting.BrowserOf(_e14).ClickMsg(rt.MsgRef{Module: "Todos.Index", Msg: "Toggle", Params: []rt.KV{{Key: "id", Value: _m15.Id}}}.URL(nil), rtesting.MsgName("Todos.Index.Toggle", _m15))
```

`Test.get (Todos.Index {})` builds the page's address as an `href` does
(the default filter left out, so `/`), asks the browser for it, passing
the page's `requires` field for the error text, and takes the model:

```go
var _p5 Todos__Index__Params = Todos__Index__Params{Filter: Todo__Filter__All__}
_u6 := rt.PageURL(rt.Page{Path: "/", Query: []rt.KV{rt.QueryKV("filter", []string{"All", "Active", "Completed"}[_p5.Filter.Tag], "All")}})
_b7 := rtesting.BrowserOf(_e4)
_r8, _err9 := _b7.GetPage(_u6, "user")
```

followed by `rtesting.ModelOf[Todos__Index__Model](_u6, _r8)`, which
gives the `Model` the page rendered (below), typed.

A query is a call into `rt/testing`. A selector literal is parsed once,
when the program starts, into a package variable of `app__alder.go`:

```go
var (
	selector__1 = rtesting.Sel("h1")
	selector__2 = rtesting.Sel("#summary")
	selector__3 = rtesting.Sel("li.done")
	selector__4 = rtesting.Sel("li")
)
```

and `Test.text "#summary" page.doc` is
`rtesting.DocTextSel(selector__2, page.Doc)`. A selector that is not a
literal is passed as text to `rtesting.DocText(sel, d)`, which parses
it when the query runs and matches nothing when it does not parse.
`Answer` is `rtesting.Answer`, a struct with a `Tag` (one of the six
constructors) and the payload's fields; a `match` on it switches on the
tag. `Doc` is `rtesting.Doc`, which holds the parsed tree.

**The test program gets the routes.** The test build's `main` (section
8, "Running tests") passes `app.Routes` and `app.Web()` to
`rtesting.Main`. `Web` is generated in every build, production's
included, with the router options that do not depend on how the app
runs:

```go
// Web is the app's options for web.NewRouter that do not depend on how it runs: the session
// codec, App.error, App.notFound and the page App.login names. Serve adds the store, the
// capabilities, the log, dev mode, the secret, the static directory and the reload generation;
// alder test's browser routes the app with these options too.
func Web() web.Options {
	return web.Options{
		Session:     session__codec{},
		ErrorDoc:    App__error,
		NotFoundDoc: func() html.Node { return App__notFound },
		Login:       "/login",
	}
}
```

`Serve` starts from it: `w := Web()`, then sets the store, the
capabilities and the rest, and returns `web.NewRouter(Routes, w)`. The
test program builds its router from the same two values, so the
routes, the cookie's codec, the error pages and the login page are
production's.

**One browser per test.** For each task test, `rtesting.Run` puts a slot
in the test's `context` holding the routes and options.
`rtesting.BrowserOf(env)` finds the slot and makes the browser the first
time a step asks (`rt/testing/browser.go`, `NewBrowser`). A context
value, not a global, so each test's browser is its own, and a step
inside `Test.run` (which runs on the same `Env`) finds the same one.
`NewBrowser` calls `web.NewRouter(routes, opts)`, replacing a few
options: the test's store and capabilities; a clock for the cookie that
stands still; dev mode off; a fixed key for signing cookies; a log that
keeps the router's last error line, which becomes `RequestFailed`'s text; and
two hooks the router has for this, `EnvFor` and `Served`.

**`EnvFor`: a request inside the test's transaction.** In production
the router gives each request a fresh `eff.Env` (section 8, "What `Run`
does"), and `eff.Run` begins a transaction on the store. A test's
store is an in-memory SQLite with one connection, on which the test
already holds a transaction: a request that began another would wait
for ever. `web.Options.EnvFor`, when set, makes each request's `Env`
instead. The browser's sets `Within`:

```go
// Within is an outer transaction an effect runs inside instead of beginning its own: an Alder
// test's browser hands the router one, so every request of the test runs in the test's
// transaction (the test's rows are visible to the page, no second connection is opened). Run
// brackets a Task with a savepoint on Tx (when Savepoints is set), released when the Task
// succeeds and rolled back to when it fails, so a failed request undoes only its own writes;
// nothing is committed. A Query reads through Tx.
type Within struct {
	Tx         Tx
	Savepoints Savepointer // nil: no savepoint, and a failure undoes nothing
}
```

`Tx` is the test's transaction, begun by the browser if no step has yet
(`env.Transaction()`). With `Within` set, `eff.Run` (`rt/eff/run.go`)
never begins or commits: a `Query` (a page's `load`) reads through
`Tx`; a `Task` (an `update`) takes `SAVEPOINT alder_tN` as it starts,
then `RELEASE` when it succeeds, or `ROLLBACK TO` and `RELEASE` when it
fails, the same three statements as `Test.run`'s. The savepoint is taken
at once rather than at the first step, so that writes made by effects
the task starts on other `Env`s are inside it too. The request's `Env`
shares the test's store, capabilities (read at each request, so a
`Test.fake` installed later applies), clock and log, and after each
request the browser appends that `Env`'s trace to the test's: the
step's name first (the `Call`), then the request's effects.

**`Served` and `Request.Model`: the model.** `Test.get` gives the model
the page rendered, so it must come from the request itself. Each page's
`GET` route sets it on the request before rendering, `rq.Model =
model` (section 8, "A request"), in every build; production ignores
it. `web.Options.Served`, when set, is called with each request after
its handler answers. The browser's keeps `rq.Model`, and `ModelOf`
returns it with its Go type, or fails "GET /: the page gave no model".
`load` runs once, so a page that reads the clock in `load` shows the
time its model holds.

**Requests in memory.** A step makes an `http.Request` with
`httptest.NewRequest`: a plain `GET` for `get` and `open`; for `submit`
and `click` a `POST` with the header `Datastar-Request: true` and the
form as its body. Each request carries the cookie and, as `Referer`,
the held page's address (an `App` message redirects back to the page
that sent it). `router.ServeHTTP` writes the answer into an
`httptest.ResponseRecorder`, and the browser reads it:

- `200` with HTML: the page is shown; the browser parses it and holds it.
- `303`: followed with a `GET` (at most ten in a row), and the answer is
  `Redirected` with the page it ends on. A target with a scheme or a
  host is another site: `Redirected` with that address, nothing
  fetched, the held page kept.
- `404` and `500`: `PageMissing` and `RequestFailed`, holding the page served.
- A Datastar answer (`text/event-stream`) is a list of events. A
  `datastar-patch-elements` event whose selector is `body` replaces the
  held `<body>` with the one it carries, and one whose selector is
  `head > title` the held `<title>`; an event that runs
  `window.location.assign(…)`, Datastar's way of navigating, is followed
  with a `GET`, as above. An event that runs `history.pushState` (the
  router followed a redirect itself, "A redirect, followed") makes its
  URL the held page's address, and the answer is what the `GET` of it
  would have given: `Redirected` with that page, or `PageMissing` or
  `RequestFailed` by `Alder-Status`. After a patch, the router's `Alder-Status`
  header (set on a Datastar answer that is a 404 or a 500, which is
  otherwise a `200` patch like any other) gives `PageMissing` or `RequestFailed`;
  then a `<ul class="errors" data-errors="…">` with items in the page
  gives `FormRejected`, its items read in page order, `data-errors="form"`
  as the field `""`; anything else is `Rerendered`.

**Controls.** Before a `submit` or `click` is sent, the browser looks
for the control on the held page: each `form`'s `action` and its
`data-on:submit="@post('…')"`, and for a click also each button's
`formaction` and `data-on:click`. (A `click={…}` button renders inside
a small `form class="_alder-click"`, which a `submit` does not count as
a form.) Two addresses name the same message when `rt.MsgKey` of them
is equal: the path and the message's own parameters, with the page's
`_p.` ones left out. The browser posts the control's own address, `_p.`
parameters included. With no match it sends nothing and returns a
`*NoControlError`, whose text the test fails with; it names the held
page's URL and, when a control posts the same message with other
parameters, lists them (`Offered`). A submit then sends only the fields
the matched form has an `input`, `select` or `textarea` for, by `name`,
as Datastar's form serialization does (`Fields.For`). The generated
`encode__M__F` returns the encoded fields together with `Absent`, the
names whose value is what the decoder makes of a missing field (a
`Maybe` without a default that is `Nothing`, a field equal to its
default); such a field without an input is left out, and any other one
is a `*NoInputError`, "the page's form F has no input `n`".

**Parsing the page.** `rt/html` already renders a view to text. For the
browser it gained `Parse` (`rt/html/parse.go`), which reads that text
back into the same `Node` tree: elements with their attributes in order,
text with its entities decoded, the contents of `<script>` and
`<style>` kept as they are. It is written for the renderer's own
output, which is always regular (lower-case names, quoted values, `<br
/>`), and tested on it: every page of both examples, rendered by the
built apps, parses and renders back byte for byte. The browser drops
the `<!doctype html>` line and the Datastar `<script>` tag the router
adds, so the held tree is what `view` made. `Select(node, selector)`
walks the tree in document order and keeps each element that matches
the selector's last compound, checking the ones before it against the
element's ancestors (any ancestor for a space, the parent for `>`).
`TextContent` joins the text inside an element and collapses its
whitespace. `Doc`'s `==` is `Node.Equal` of the two trees after
`html.WithoutPageParams` has taken the `_p.` parameters out of every
message address, so a page compares equal to `Test.doc (view …)`,
which renders without a request. When a test's `==` of two `Doc`s is
`False`, `rtesting.Eq` records `html.Diff` instead of the two markups:
the path of the first element whose content differs and that element on
each side, cut to 120 characters, which the report prints as `left:`,
`right:` and `note:`.

**Production.** None of this is in an app's binary. The step nodes, like
`Test.run`'s, reach the emitter only from `test` bodies, which a
production build does not emit (a step that reached it anyway would be
the emitter's internal error, `testNode`); the `encode__` functions and
the `selector__` variables are emitted only in test builds; and
`TestGen_ProdHasNoTests` (`internal/gen/gen_test.go`) checks that a
production build of a fixture app using every step has none of
`encode__`, `selector__` or `BrowserOf`. What production gains is
`Web()`, the routes' `rq.Model = model`, the router's two hooks, left
unset, and the `Alder-Status` header on a Datastar answer that is a 404
or a 500, which Datastar's client ignores. `rt/html` imports
`rt/selector` (for `Select`), so the package is part of every
app's build, but nothing in production calls `Parse` or `Select`, and
the linker leaves their code out: `go tool nm` on a built app lists no
`rt/selector` symbol. (The selector package keeps the HTML element
names in a string rather than a map for the same reason: a map would be
built when the binary starts.)

### Mutation testing

Chapter 10, section 11, shows `alder test --mutate` from the outside.
Inside, it is three pieces: `internal/mutate` decides the mutants, the
emitter compiles all of them into one test program, and `rt/testing`
runs the suite once per mutant.

**Which mutants.** `mutate.Collect(p *ir.Program)` walks the typed IR
(section 6) and returns the mutants and the refused ones. A mutant is a
`mutate.Mutant`. It has:

- its `ID`;
- where it is: `Module`, `Decl`, the source `File`, `Line` and `Col`;
- its `Kind`, the operator's name;
- `Before` and `After`, the code as one line each, printed back from
  the IR by `internal/mutate/show.go` and shortened at 80 columns (an
  `arm swapped` whose two matches would not fit says `the All and Active
  bodies swapped` instead);
- the change itself.

For most operators the change is a pair of IR nodes. `Node` is the node
to replace (an `If`, an `Op`, a `DbSelect`, the effect of a `let … <-`
step). `Replacement` is a new node of the same type, built from the
existing node kinds: a `DbSelect` without one `where`, an `If` whose
condition is wrapped in `not`, an `EffPure` of a placeholder for a
write. Since the replacement has the node's type, every mutant is
well-typed, and the checker's work is not undone. `requires dropped`
names the `Page` instead, and the schema operators carry `Table`, a
clone of the table with the change.

To say where a survivor is, the lowerer records each mutated node's
source position (`ir.Pos`, a line and a column). The IR dump leaves
positions out, so its goldens did not change. Collect visits the
modules in the program's order (imports first, otherwise alphabetical).
In each module it takes a page's `requires` first, then the tables
field by field, then each value's body in pre-order, numbering from 1.
It never enters a `test`, a `fake`, a `view`, a component, or a value
whose result has `Html` in it. A change it refuses is a
`mutate.Suppressed` with a `Category` (`equivalent` or `skipped`) and a
`Why`.

**Mutant schemata: every mutant in one build.** Building the app once
per mutant would cost a `go build` each, seconds times 32. Instead,
`gen.Options{Tests: true, Mutate: true}` (which only `alder test
--mutate` sets) makes the emitter compile each mutated node with its
replacements in front of it, behind a switch on one package variable.
This technique is called *mutant schemata*. `Todo.countOpen` in the
plain test build (`build/todos/scratch/test/Todo_alder.go`) is

```go
func Todo__countOpen(todos []Todo__Todo) int64 {
	return std.ListLength(std.ListFilter(func(t Todo__Todo) bool {
		return !t.Done
	}, todos))
}
```

In the mutate build (`build/todos/scratch/mutate/Todo_alder.go`), where
mutant 14 is `not dropped` on `not t.done`, it is

```go
func Todo__countOpen(todos []Todo__Todo) int64 {
	return std.ListLength(std.ListFilter(func(t Todo__Todo) bool {
		switch Mutant {
		case 14:
			return t.Done
		}
		return !t.Done
	}, todos))
}
```

`Mutant` is 0 in a normal run, so the code runs as written. Set to 14,
the filter keeps the done todos instead. This is the mutant that the
strengthened "countOpen ignores done todos" kills: its three todos then
give 1, not 2.

**In place, not per subtree.** A replacement can share the node's
subtrees: a negated `if` has the same branches, a flipped `&&` the same
operands. Emitting each replacement in full would emit those subtrees
once per mutant, and their own mutants' switches inside each copy: an
`else if` chain nine levels deep, with two mutants per level, became 3
MB of Go that took a minute and 1.7 GB to compile. So the mutants of an
`if` switch only its condition, and its branches are emitted once. The
blog's page clamp,

```alder fragment
  let page = if params.page < 1 then 1 else if params.page > lastPage then lastPage else params.page
```

is in the mutate build

```go
	page := func() int64 {
		if (Mutant == 18) != (func() bool {
			switch Mutant {
			case 19:
				return params.Page <= 1
			}
			return params.Page < 1
		}()) {
			return 1
		}
		if (Mutant == 20) != (func() bool {
			switch Mutant {
			case 21:
				return params.Page >= Posts__Index__lastPage()
			}
			return params.Page > Posts__Index__lastPage()
		}()) {
			return Posts__Index__lastPage()
		}
		return params.Page
	}()
```

where 18 and 20 are `condition negated` (`(Mutant == 18) != c` is `not
c` under 18 and `c` otherwise) and 19 and 21 the flipped comparisons. A
`guard dropped` on an `if` is `Mutant != k && c` (or `Mutant == k ||
c`). A `match` with mutants emits each arm's body once: it first finds
the arm, and an arm that a mutant moves gives its number instead of its
value; `guard dropped` and `arm swapped` then change the number (under
an arm swap, arm 1 becomes 2 and 2 becomes 1), and a switch on the number
returns the body. For the other operators, the children a replacement
shares (the operands of a flipped operator, the argument of a dropped
`not`, an update's fields, a select's steps) are computed once into Go
variables before the switch, and the original and each replacement use
those (the right operand of `&&` and `||` becomes a function, called
only when needed). The mutate build is then a few hundred bytes per
mutant larger than the test build, and `TestGen_MutateNested` checks
that on twelve-deep chains.

The emitter switches a node in three places, and all three do nothing
when `Mutate` is unset:

- where a node is emitted as Go statements that return its value (the
  form above);
- where it is a Go expression (the same switch inside `func() T { …
  }()`);
- where it is an effect in a task's step (the closure's type is
  `eff.Task[A]`).

While it emits the original after the switch, it marks the node so that
it is not wrapped again. A node that the IR shares between places is
switched in each. A table field's default, for example, is one node in
every record literal that leaves the field out, tests' literals
included, which is what makes a `default changed` mutant reach every
insert.

`requires dropped` is in the route, not in a function body. Before the
check that sends a visitor to log in, `Todos.Index`'s route
(`routes__alder.go`) has

```go
	app := rq.Env.Session.(App__Session)
	if !app.User.Ok && Mutant == 117 {
		app.User = rt.Just("mutant")
	}
	if !app.User.Ok {
		r := web.RedirectResponse(rq.Login, keep)
		return model, Todos__Index__Session{}, &r
	}
```

so under mutant 117 a session without a user gets the placeholder user
`"mutant"`, and the check that follows lets the request through. `role
dropped` is in the route too, in the role check that follows the
`requires` check (section 8, "The rules"). The blog's `Write` page has

```go
	if Mutant != 190 && app.Role.Tag != is__User__Role__Author {
		r := web.NotFoundResponse(html.None())
		r.Session = keep
		return model, Write__Session{}, &r
	}
```

so under mutant 190 every signed-in user passes. `scope dropped` replaces
a scoped node with the unscoped one, built from existing IR nodes so it
has the same type: `Db.mine T` becomes `Db.from T`, and
`Db.updateMine T id f` becomes a `Db.get`, then a `match` on its result
that runs `Db.update T id f` for `Just _` and `Task.notFound` for
`Nothing`, so the mutant is "not found" only for a missing row. The
to-do app's `Toggle`, mutant 21, in the mutate build:

```go
			switch Mutant {
			case 20:
				return eff.Pure(struct{}{})
			case 21:
				return eff.Bind(eff.Call("Db.get Todo", false, func(_e2 *eff.Env) (rt.Maybe[Todo__Todo], error) {
					return db.GetRow(_e2.Ctx, _e2.Store.(db.Store), db.TxOf(_e2), table__Todo__Todo, p.Id)
				}), func(found rt.Maybe[Todo__Todo]) eff.Task[struct{}] {
					if found.Ok {
						return eff.Call("Db.update Todo", true, func(_e3 *eff.Env) (struct{}, error) {
							return struct{}{}, db.UpdateRow(_e3.Ctx, _e3.Store.(db.Store), db.TxOf(_e3), table__Todo__Todo, p.Id, func(t Todo__Todo) Todo__Todo {
								…
							}, _e3.Now())
						})
					}
					return eff.NotFound[struct{}]()
				})
			}
			return eff.Call("Db.updateMine Todo", true, func(_e4 *eff.Env) (struct{}, error) {
```

(20 is `write replaced` on the same step; `…` is the record the function
builds, with mutant 22's `not dropped` inside it.) The names `found` and
`row` the replacements bind are made fresh when the wrapped code already
uses them. `Db.getMine` becomes a `Db.get` whose `Nothing` is
`Query.notFound`, and `Db.deleteMine` a `Db.get` then `Db.delete`;
`Db.insertMine` has none, since the owner comes from the session either
way.

One more change: in a mutate build every constant is a function. A plain
build computes a constant once, as a Go package variable. The blog has

```go
var Posts__Index__perPage int64 = 10
```

A package variable is computed when the program starts, before any
mutant is switched on, so a mutant inside it would never be seen. In
the mutate build it is

```go
func Posts__Index__perPage() int64 {
	return 10
}
```

and its uses call `Posts__Index__perPage()`. The constant is computed
again at each use, so a constant that is costly to compute makes the
mutate run slower (never the plain test run or the production build,
where it is still a package variable).

**The table of mutants.** `app__alder.go` also gets

```go
// Mutant is the mutant the running suite switches on (alder test --mutate): an id of Mutants, 0 for none.
var Mutant int
```

and `Mutants`, one `rtesting.Mutant` per mutant in id order, such as

```go
	{ID: 7, Module: "Todo", Decl: "Todo", Kind: "cascade ↔ restrict", File: "app/Todo.alder", Line: 16, Col: 3, Before: "owner @ref User @restrict", After: "owner @ref User @cascade", Tables: []db.TableDesc{table__User__User.Desc(), {Name: "Todo.Todo", Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "title", Type: db.CText}, {Name: "done", Type: db.CBool}, {Name: "due", Type: db.CDate, Nullable: true}, {Name: "owner", Type: db.CText, Ref: "User.User", OnDelete: db.Cascade}, {Name: "created", Type: db.CTime}}, ID: "id", Created: "created"}}},
	{ID: 16, Module: "Todos.Index", Decl: "load", Kind: "orderBy reversed", File: "app/Todos/Index.alder", Line: 18, Col: 10, Before: "Db.orderBy Todo.created.desc", After: "Db.orderBy Todo.created.asc"},
```

A schema mutant carries `Tables`: the app's table descriptors, with its
own table's descriptor rendered from the mutant's clone (here `owner`'s
`OnDelete: db.Cascade`). The last line has the refused mutants' counts:

```go
// MutantsSuppressed and MutantsSkipped count the mutants refused as equivalent and as not applicable.
var MutantsSuppressed, MutantsSkipped = 0, 5
```

The test program's `main` registers the table before running the tests:

```go
rtesting.SetMutants(&app.Mutant, app.Mutants, app.MutantsSuppressed, app.MutantsSkipped)
```

A production build has none of this. `TestGen_Mutate`
(`internal/gen/gen_test.go`) builds every mutation fixture, both
examples and both corpus apps with `Mutate`, and vets each. It checks
that every mutant has its switch and that no production build mentions
`Mutant`.

**How a kill is decided.** `rtesting.Main` with `--mutate` first runs
the suite as usual. If a test fails, it prints `tests fail: fix them
before measuring mutants` and stops. Otherwise `runMutant`
(`rt/testing/mutate.go`) does this for each mutant in turn:

1. It sets `Mutant` to the mutant's id (and back to 0 afterwards).
2. For a schema mutant, it replaces the tables each test's database is
   made from with the mutant's `Tables`, made by `db.Ensure`, and drops
   the migrations, so the changed rule is what the test meets (the JSON
   says `"tables":"ensure"`).
3. It starts one timer for the mutant: `-timeout` (10 s by default), or
   three times the suite's own run when that is longer, so a slow suite
   does not time out on a slow moment of the machine. (The suite's run
   is logged, so a later program that goes on from a mutant has it.)
4. It runs the selected tests in order, each in a goroutine with its
   own fresh database, as plain `alder test` does. The first test that
   fails kills the mutant, and is its `by`. A test that panics (a Go
   runtime error, say) kills it with the note `crashed`. When the timer
   fires first, the running test kills it with the note `timed out`; the
   timer is looked at first, so a test that fails just as the time is up
   is a timeout too. If every test passes, the mutant survived.

The score is the number of killed mutants divided by the number of
mutants that ran (1 when none ran), and `--min` compares that exact
number.

**When a mutant ends the program.** Some mutants cannot be caught in a
goroutine. A stack overflow is a Go fatal error, which no `recover`
sees, and a test that loops for ever cannot be stopped from outside its
goroutine. So `alder test` runs the program with two files in
`build/<app>/scratch/mutate/`:

- Before each test, the program writes `#id`, the module and the test's
  name to the status file.
- After each mutant, it appends one JSON line, the mutant's outcome, to
  `mutants.log` (`ALDER_TEST_MUTATE_LOG`).

A timed-out mutant is logged, and then the program exits with code 3
(`rtesting.ExitMutantTimedOut`). If instead the program dies while the
status file names a mutant, `alder test` logs that mutant as killed,
`crashed`, by the test the status names. Either way `alder test` runs
the program again with `ALDER_TEST_MUTATE_FROM` set to the next id,
which skips the suite's own run and starts there. The run that reaches
the last mutant reads the log and prints the whole report. A crash in
the suite's own run is reported as plain `alder test` reports one,
followed by `tests fail: fix them before measuring mutants`.

### The runtime travels with the compiler

The generated code imports the runtime, so a build needs the runtime's
source next to it: Go compiles a program from source, its libraries
included. Until M10 the generated `go.mod` said `replace alder =>
"/path/to/the/checkout"`, and an app built only where this repository
was checked out. Now the runtime is inside the `alder` binary, and every
build writes it out.

**Embedding.** Go can put files into a program while compiling it: a
`//go:embed` comment above a variable names files of the package's
directory, and the compiler stores their bytes in the binary, readable at
run time through the variable (an `embed.FS`, a small read-only file
system). The standard library's signatures already travel this way
(`stdlib/embed.go`: `//go:embed *.alder`). The runtime's embed is
`rt/embed.go`:

```go
//go:embed core.go nav.go show.go tether.go
//go:embed al/al.go al/al.css al/custom-elements.json al/VERSION
//go:embed datastar/datastar.go datastar/datastar.js datastar/VERSION
//go:embed db/encode.go db/migrate.go db/sqlite.go db/store.go db/table.go
…
//go:embed dist/go.mod.txt dist/go.sum.txt
var Files embed.FS
```

Three rules of `go:embed` shape it. A pattern can name only files in the
package's own directory and below it, never `..`: so the embed is in
package `rt` itself, the root of the runtime, and the selector parser that
`rt/html` shares with the checker had to move into the runtime
(`internal/selector` became `rt/selector`) to be inside. A pattern cannot
leave files out, and `*.go` would take the tests too: so the list names
every file. And a pattern cannot reach into another module, which a
directory holding a file named `go.mod` is: so the runtime module's
`go.mod` and `go.sum` are stored as `dist/go.mod.txt` and
`dist/go.sum.txt`. An explicit list can fall behind, so a test holds it
to the repository: `TestFiles_Complete` (`rt/embed_test.go`) compares the
embedded files with `git ls-files rt` without the tests, `testdata/` and
`embed.go` itself, and fails on a file added to `rt/` and not to the list.
`TestDistModule` checks that the two `dist/` files equal the repository's
`go.mod` and `go.sum`, and that every module `go.mod` requires is used by a
package of `rt/`: the repository's dependencies are the runtime's (the
compiler links SQLite too, but only because it uses the runtime's
`rt/db`: the migration generator, buildkit's migration check, eval's
dev-database hint),
so the copy lists nothing the runtime does not need. The embed costs the
binary about 0.4 MB.

The design system travels the same way. `rt/al/` holds `al.css`,
`custom-elements.json` and `VERSION`, a copy of one release of the
`alder-ui` repository (`scripts/vendor-al.sh <tag>` makes it), and the
second `//go:embed` line of `rt/embed.go` names them. The checker reads
the manifest from `rt.Files` (above, section 5); a build copies the two
assets into the app's static directory (below). `TestDesignSystemVersion`
(`rt/embed_test.go`) checks that `VERSION`, the manifest's `version` and
the stylesheet's first line name the same release.

**Writing it out.** `buildkit.WriteRuntime(dst)`
(`internal/buildkit/buildkit.go`) writes `Files` into `dst/rt/` as the
module `alder`: `dist/go.mod.txt` becomes `rt/go.mod`, `dist/go.sum.txt`
`rt/go.sum`, and every other file keeps its path under `rt/rt/`. The
doubled `rt` is how Go finds packages: an import path is the module's
name followed by the package's directory inside the module, so with the
module `alder` rooted at `build/todos/rt/`, the import `alder/rt/html` is
the directory `build/todos/rt/rt/html/`. It writes `dst/go.sum` too, a
copy of the runtime's, since the app module's only outside dependencies
are the runtime's: with it Go can check every download, and a build
needs no network once Go's module cache holds `modernc.org/sqlite`.
`alder build` calls it right after writing the generated files, then
`buildkit.DownloadDeps`, which makes sure Go's module cache holds the
runtime's requirements before `go vet` could fail on them in a way that
looks like the compiler's fault (chapter 11 shows its one line and its
error); the scratch runner (`scratch.CompileCheckedWith`) does the same in its own
directory, so `alder test`, `eval` and the REPL build the same runtime as
`alder build`.

The static files go next to it: `buildkit.CopyStatic(src, dst)` replaces
`build/<app>/static/` with a copy of the app's `static/`, then calls
`WriteDesignSystem`, which writes `al/al.css` and
`al/custom-elements.json` from `rt.Files` into it. Every build has them,
whether or not the app has a `static/` directory, and `alder run` serves
the build's copy (it copies again when a file under `static/` changes,
before reloading the browser). The router (`serveStatic` in
`rt/web/router.go`) answers `/static/al/al.css` and
`/static/al/custom-elements.json` from the runtime's own embedded copy
(`rt/al/al.go`, package `al`, which embeds the two files again so the
written runtime has them) with `web.DesignSystemCache`,
`Cache-Control: public, max-age=300, stale-while-revalidate=86400`, and a
strong `ETag`, a SHA-256 of the bytes computed once; `http.ServeContent`
answers a matching `If-None-Match` with `304 Not Modified`. An immutable
header would be wrong there: the address stays `/static/al/al.css` across
compiler upgrades, so a browser would keep an old stylesheet for a year.
`no-cache`, what Alder sent before M13, made every page load wait for a
round trip to revalidate the stylesheet before it could paint: in dark
mode, a white frame. The five minutes of `max-age` let a load paint from
the cache at once; after them, `stale-while-revalidate` lets the browser
use its copy for up to a day while it revalidates in the background.
The price is that for up to five minutes after an upgrade a browser may
show the old stylesheet, and one load more after them: that load paints
with the stale copy while the revalidation fetches the new one. The Datastar script can be immutable because its
address, `datastar.ScriptPath`, carries `datastar.Version`.

The `<head>` a boosted navigation never patches has the same problem at
a larger scale: a tab open across a deploy would keep the old build's
stylesheet link, Datastar script and boost for as long as it is open.
`web.HeadTags` therefore begins with `web.BuildMeta()`,
`<meta name="alder-build" content="…">`, whose content is `web.BuildID()`:
the first 16 hex digits of the SHA-256 of the running executable, read
once (`sync.OnceValue`; when the executable cannot be read, a value made
from the start time). The boost's `data-init` copies it to
`window._alderBuild`, and its click and popstate `@get`s send it as the
header `Alder-Build` (`datastar.BuildHeader`). `ServeHTTP` compares it
with its own before routing: a Datastar `GET` of another build is
answered by `reload`, `datastar.Navigate(w, url, datastar.Assign)`, so
the browser loads the URL as a document and gets the new `<head>`, once
(a load of the URL the tab already shows replaces its history entry).
Two processes of one binary (a restart, a second replica) agree; a
request without the header (a hand-made one) is not checked. The same
`reload` answers a Datastar `GET` under `/static/` or `/_alder/`, files
that are no page to morph in (the boost leaves such links alone; an
older boost did not).

It refreshes rather than adds. It first walks `dst/rt/` and deletes every
file the embedded copy does not have (and the directories that leaves
empty), then writes each file whose content differs, leaving the rest
alone. Without the deletion, a file of an older runtime would stay and
be compiled with the new ones: `TestBuild_Todos`
(`cmd/alder/build_test.go`) plants `build/todos/rt/rt/web/gone.go`, which
does not compile, and the build succeeds because it is gone.
`TestWriteRuntime` (`internal/buildkit/buildkit_test.go`) checks the
rest: stray files and directories removed, an edited file restored, an
unchanged file not rewritten.

The generated `go.mod` is a constant now (`goMod` in
`internal/gen/gen.go`):

```
module app

go 1.27

require alder v0.0.0

replace alder => ./rt
```

`require alder v0.0.0` says the app needs the module `alder`, at a
version that does not matter, and `replace` says where its source is,
relative to the `go.mod`, so the directory can move. A `replace` to a
directory is final: Go reads the module from there and never asks a
proxy for it.

**Why the module is still called `alder`.** A Go module's name is
usually where it can be downloaded, such as `github.com/someone/alder`.
The runtime is never downloaded, so its name needs to be that of no
place, and `alder` is that: a name without a dot, which Go would not even
try to fetch. Had the runtime taken the repository's hosting address as
its name, every generated file would import it by that address (`import
"github.com/someone/alder/rt/html"`), and moving the repository would
change every app's generated code. With `alder` and a `replace`, the
generated code names nothing outside the build directory.

`TestDist_OutOfTree` (`cmd/alder/dist_test.go`) tests the whole: it
builds the `alder` binary, copies both examples to a temporary directory
outside the checkout, and runs `alder check`, `alder build` and `alder
test` there with a `PATH` that holds only that binary and Go; it then
checks that no file under `build/` contains the checkout's path and that
the built app answers `GET /`.

### The generators: copy, check, write

Chapter 15's commands are the compiler's front end used the other way
round: instead of reading source to make Go, they make source. They live
in `internal/generate/`, and every one ends in the same function,
`generate.Apply(appDir, edits, options)`.

A generator first loads the app as `alder check` does (`generate.Load`:
the names of section 4 and the checker of section 5) and turns what it
needs into plain facts, a `generate.Program`: the tables with their
columns, types and attributes, the pages with their paths and
`requires`, the session's `user` and `role`. From those facts it
computes a list of **edits**: a file to create, with its text; a file to
change, with its whole new text; or, for the admin, new bodies for the
owned regions of a file. The small generators fill templates
(`internal/generate/templates/*.tmpl`, Go's `text/template`, embedded in
the binary with `embed` like the runtime; their delimiters are `[[ ]]`,
since Alder's own braces would read as template actions). The admin
builds its text in Go (`internal/generate/admin.go`): it needs
decisions per column (a select for a reference, a checkbox for a
`Bool`, a count for a `@unique`) that templates express badly. Code
that goes into an existing file is placed by looking at the parsed
module: a new declaration before `update`, an arm before the `}` of its
`match`, a form inside the view's last `al-card`.

`Apply` then does what chapter 15 calls copy, check, write. It copies
the app into a temporary directory (all but `build/`, `.git` and the
databases), applies the edits there, runs the migration generator on
the copy when the command changed a table, and checks the copy with the
same loader and checker. If the copy has an error, `Apply` returns a
refusal carrying the copy's diagnostics, and the app was never touched.
Otherwise it writes each file the copy changed back into the app, and
returns what it did: the files, a unified diff of each edited one
(`generate.UnifiedDiff`, a longest-common-subsequence diff with three
lines of context), the diagnostics in the files the command changed, and
the holes. With `--dry-run` it stops before writing.

**Holes.** A hole `?name` parses as an expression of its own (section
3), and the checker gives it the type its place demands and reports it
as W0002 with that type. `generate.HoleIndex` turns those warnings into
addresses: the module, the declaration that encloses the hole's
position, the arm when the declaration is `update`. `alder fill` parses
standard input with `syntax.ParseExpr`, refuses anything after one
expression, puts the expression where the hole's span is (wrapped in
parentheses where it would otherwise bind differently, indented to its
place), and hands that edit to `Apply`: the checker then decides.

**Regions.** A region is found by scanning a file's lines for its
markers (`generate.Regions`): a `-- @admin begin <name>` line, the lines
after it, and the next `-- @admin end`. A missing or doubled marker is
an error before anything else happens. `alder regen admin` generates the
whole admin again in memory from the app as it is now, then for each
file on disk builds an edit that replaces the body of each named region
with the same region's body from the fresh version; lines outside the
regions are never part of an edit. A file whose regions are already
equal produces no edit, which is why a second run says there is nothing
to do. Errors in the admin's own files do not stop it (a renamed field
breaks them, inside the regions it is about to rewrite), but the result
goes through `Apply` like every other command's, and must check.

**Options checked on submit.** For a page whose view has a `<Select
options={…}>`, section 7 emits two more functions. One per select,
`choices__<Page>__<Form>__<field>`, is the `options` expression compiled
as a function of the view's parameters, the model and the session. The
other, `options__<Page>`, takes the posted form's values and returns a
field error, `web.MsgOption` ("must be one of the options"), for each
value that `html.Offered` does not find among the choices (the empty
value only when the select offers an empty option). A message's
handler calls it after `load` has run for the message and before
`update`, so the options are computed from the same model the page
would draw; a field error is answered like a decoder's error. This is
why `options` may use only the view's parameters: a name bound by a
`for` or a `let` of the view would not exist outside the view.

### Deploying: the pipeline, the health check, the backup shipper

`alder deploy` (chapter 16) is not part of the compiler's pipeline from
source to Go; it is a program that drives the compiler's other commands
and then talks to a server. Its code is in `internal/deploy/`, with the
command itself in `cmd/alder/deploy.go`.

**The gate reuses the commands.** `deploy.Runner` is an interface with
four methods, `Check`, `Build`, `Test` and `Mutate`; the command
implements it (`deployRunner`) by calling the same functions `alder
check`, `alder build` and `alder test` run, capturing what each prints.
`alder test --mutate --json` prints the score as one JSON object, which
the runner reads. The check step also hands back the lowered program,
whose `ir.Deploy` is the declaration (section 6): the environments, the
floor, the checks, the `requires` names. `deploy.GateCheck` and
`deploy.GateRest` (`gate.go`) run the steps in order, print one line per
step, and return a `*deploy.Refusal` (its text, and the step's output as
detail) for the first that fails.

**The artifact and the build id.** `MakeArtifact` (`artifact.go`)
builds the executable into a temporary directory with `go build` and
`GOOS`/`GOARCH` set to the target's platform (with `CGO_ENABLED=0`: the
SQLite driver, modernc.org/sqlite, is Go, so any platform can build for
any other), hashes it with SHA-256, and renames the directory to the
first 16 hex digits. The runtime computes the same number for itself
(`web.BuildID`: it hashes `os.Executable()` once, the first time it is
asked) and writes it into every page's `<meta name="alder-build">`, as
"The runtime travels with the compiler" above describes. Nothing records the id anywhere in between: the
same bytes give the same id on both sides.

**The plan.** Each migration file starts with a header naming the
schema hash it starts from and the one it leads to (`-- from`, `-- to`;
section "Migrations"). `MakePlan` (`plan.go`) hashes the live database's
`schema.json`, which the target hands back, finds the build's file whose
`-- to` is that hash, and lists the files after it as pending, with
their `-- step:` lines and the `DROPS DATA` lines `alder migrate` wrote
into the header. A live hash no file leads to is a refusal: the server
runs a newer schema than this build knows.

**The engine and the targets.** `deploy.Deploy` (`deploy.go`) is the
sequence of chapter 16, written once: it calls a `deploy.Target`
(`target.go`) step by step, `Connect`, `Requires`, `Schema`, `Upload`,
`Backup`, `Migrate`, `Start`, `Health`, `Switch`, `Record`, `Stop`,
`Prune`, and decides by itself what to refuse and what to print. A
target knows how to do each step at its kind of place and nothing about
the order, so the rules (no upload before the plan is accepted, no switch
before the health check, the previous build left serving on a failure)
hold for every target because they live in one function. On the
experimental Fly target the last rule holds only because it refuses a
deploy with a pending migration (`MigrationRefuser`): its previous image,
put back on a migrated database, would refuse to start. The
engine's tests drive it with a fake target that records its calls.

What a target can do beyond the core is a small optional interface the
engine asks for with a type assertion: `Locker` (the environment's
lock, taken by deploy, rollback and restore), `Preflighter` (the unit
and Caddy prepared before anything is uploaded), `DataLossAllower` (the
run's `--allow-data-loss`), `BackupRestorer` (put a pre-migration copy
back: `rollback --restore-backup`, and the automatic restore after a
failed deploy whose migrations dropped data), `Rerouter` (check that
the traffic reaches the build already serving), `StreamChecker` (the
environment file streams the backup) and `MigrationRefuser` (the Fly
target, which refuses a migrating deploy for now). A target without one
gets the plain behaviour, and the refusals name only the commands that
target has.

The `Ssh` target (`ssh.go`) runs every step as a small POSIX shell
script sent over the system's `ssh` to `sh -s` on standard input, after
a prelude defining the root directory and a function `alive` that says
whether a build's process runs. There is no agent on the server and no
library: `ssh`, `sh`, `tar` and `curl` are the whole protocol. The
upload is a tar stream Go writes (`archive/tar`) into a `.part`
directory, renamed when complete. Each build gets a generated `run`
wrapper that sources the operator's environment file and sets
`ALDER_DB`, `ALDER_STATIC`, `ALDER_ADDR` and `ALDER_BACKUP_URL`; the
wrapper holds names and paths, never a value, and `Requires` checks the
environment file with `grep -Eq` so that only an exit status crosses the
connection. Caddy is driven through its admin API on `localhost:2019`
with `curl`: one route per environment, its `@id` the unit's name, its
upstream port changed by a `PATCH` on switch. The lock is one more
`ssh` session, kept open for the run: its script takes `flock` on
`<path>/deploy.lock` (or creates `deploy.lock.d`) and then reads its
standard input until alder closes it, so the lock lives exactly as long
as the run's connection. Replacing the database (`RestoreBackup`,
`Restore`) first stops every build that runs, cancelling the delayed
stop of one still draining: a process that kept the old file open would
go on serving it, renamed aside. Three Caddy caveats follow from
driving it only through the admin API (chapter 16, "Caddy"): the routes
live in Caddy's running configuration, so a Caddy alder did not install
(no `--resume` drop-in) forgets them when it restarts from its
Caddyfile; a server of the box's own already on `:443` conflicts with
the `alder` server at `Switch`; and the admin API on localhost answers
every local user of the box. The `Fly` target (`fly.go`)
does the same steps through Fly's Machines API with `net/http`, running
its shell scripts in the machine through the API's `exec`, and `flyctl`
for the image build, the public addresses and the secrets. The `Script`
target (`script.go`) is not a `Target` at all: after the artifact, the
engine runs it and is done.

**The health check by build id.** `Health` asks the new build for `/`,
following redirects (a page that `requires user` answers `/` with the
login page, which is a page of the same build), until the answer is a
200 whose HTML contains `name="alder-build" content="<id>"`, or the
build's process has exited, or the timeout passes. The 200 matters: a
build started on a database that is ahead of it serves the maintenance
page, which carries the same meta, with 503, and must not be switched to. On the `Ssh` target that is `curl` in a loop at
the box, against the new build's own port; on Fly it is Go's HTTP client
against the public address, which is the only way in. The id is the
point: a check that only asked "does something answer on port 443?"
would pass against the old build.

**The binary's maintenance arguments.** The generated `main` (section
7) takes two arguments besides serving: `--migrate-only` checks
`ALDER_SECRET`, opens the database, applies the pending migrations
(printing `migrated <name>` for each, which the engine reads into the
record; a pending migration marked DROPS DATA on a database that had
migrations is refused, `db.ErrDropsData`, unless
`ALDER_ALLOW_DATA_LOSS=1`, so the data-loss rule holds where the
migration runs, whatever drove it) and exits; `--backup-to FILE` writes a consistent copy with
`VACUUM INTO` while another process serves the same file. Both run
without constructing a single adapter, so a migration can run on a box
where the mail server is down. An app with a `silo` (chapter 8) has four
more, which the silo commands run at the target (chapter 16):
`--silo-status` prints one JSON line per silo file
(`{"key":"org","id":"…","state":"ok|migrating|pending-data-loss|ahead|failed|orphaned","detail":"…"}`)
and changes nothing; `--migrate-silos [--allow-data-loss]` opens and
checks every silo file now, printing `org/<id>: ok`, `org/<id>: migrated
0002_x` or the state and why, and exits 1 when any is not served;
`--verify-silos N` tries the pending migrations on the N largest silos in
transactions it rolls back (a deploy's preflight); `--prune-silos …`
removes the orphaned silos it is given, each checked again first.

**The backup shipper.** `rt/backup` is a small Litestream. SQLite in
WAL mode appends every committed change to `app.db-wal` as **frames**
(a page of the database and a header), and only a **checkpoint** copies
them back into `app.db`. With `ALDER_BACKUP_URL` set, the runtime opens
its connections with `wal_autocheckpoint(0)`, so nothing checkpoints
behind the shipper's back, and starts a goroutine (`Shipper`) that
wakes every ten seconds: it takes the write lock (`BEGIN IMMEDIATE`:
writers wait for a moment, readers do not), copies the frames committed
since its last offset (it reads the WAL's header and walks frames to
the last one whose commit field is set), runs a `PASSIVE` checkpoint,
releases the lock, and uploads the copied frames as one segment. A new
generation starts with a snapshot: the database file copied outside the
write lock but inside a read transaction (only the shipper checkpoints,
and it does not until the copy is done, so the file cannot change under
it). The pre-migration copy a deploy takes is the other kind
(`snapshot.go`: `VACUUM INTO`, while another process serves). One
shipper runs per database (`flock` on `<db>-backup.lock`: the new
build's waits for the old build's to exit). Uploads that fail stay
queued on disk and are retried in order with a doubling wait; the queue
is capped at 256 MB of WAL segments (the generation's snapshot is not
counted, or a database larger than the cap would never ship), and past
it the queue is dropped for a fresh generation. The
upload is a plain HTTP `PUT` signed with AWS's Signature Version 4,
written out in `s3.go` (about 250 lines) instead of pulling in an SDK.
Restoring (`restore.go`) downloads the newest snapshot before the
asked-for time, writes each run of segments next to it as an
`-wal` file and lets SQLite itself apply them (`PRAGMA
wal_checkpoint(TRUNCATE)`), then checks the page count against the last
commit frame: SQLite is the only program that ever interprets the WAL.

**The workflow.** `alder ci init` (`cmd/alder/ci.go`) is a string
builder over the declaration: which of `production` and `preview` exist,
their kinds (an `Ssh` environment needs an SSH key in CI, a `Fly` one a
token and its values), the floor. Its output is compared with golden
files in `cmd/alder/testdata/ci/`, and chapter 16 quotes one of them,
checked by `go test ./book`.

### What is still to come

M4a added `alder eval` and the book verifier, M4b `alder test`, M4c
the REPL, M5b migrations, M5c fakes, M5e the browser, M9 mutation
testing and M10 the embedded runtime (above). See the design spec,
[`docs/superpowers/specs/2026-10-09-m4-test-repl-design.md`](../docs/superpowers/specs/2026-10-09-m4-test-repl-design.md),
"Milestones".

### The dev trace, the panel and `/_dev`

In developer mode the router gives each request's `eff.Env` a `Recorder`
(`rt/eff`): `Run` adds a step (name, milliseconds, and "adapter" or "fake" for a
capability's call) around every `Call`, and `rt/db` wraps its querier so each
statement is added with its arguments, time and rows touched (a read's rows
are counted by its caller once they are read, `Recorder.SetRows`). A nil
recorder costs nothing. A redirect the router follows in the same answer
(`follow`, chapter 6) serves the target page with the first request's
recorder: the hop notes where its steps and statements begin
(`tracker.hops`), the panel prints "then GET <url>" there, and each request
keeps its own answer (`tracker.setAnswer`), so the panel stays the message's.
After the answer the router renders the panel from it
(`rt/web/devpanel.go`): inlined before `</body>` on a full document; on a
Datastar answer the body patch carries an empty `<al-devpanel id="_alder_panel"
data-ignore-morph>` (kept by the morph, with its open state) and the panel's
contents follow as one more `datastar-patch-elements` event, selector
`#_alder_panel`, mode inner. The panel's script (`panelScript`, a constant of
`rt/web/devpanel.go`) is written once, after the panel of a full document, and a
morph leaves it running: it keeps the tab last chosen on the shell
(`data-tab`) and, after each patch of the panel (whose sections but Request
arrive `hidden`), shows that tab's section again; a `MutationObserver` on the
panel files each request
(its `data-rid`, bar line and sections) in `sessionStorage`, the last 20 of the
tab, and lists them in the History tab; "Copy as curl" copies the line the
server built (`curlLine`, with a `<your session cookie>` placeholder). `/_dev`
is a route the router answers itself when `ALDER_DEV=1`. Its page is one
`html/template` over two things: `Options.DevMap`, a compact JSON the
generator wrote into `Web()` (`internal/gen/devmap.go`: the pages with their
path, `requires`, roles, parameters and messages, the tables with columns and
attributes, the capability names, the session's fields), and what only the
running server knows (the build id, the migrations the database has, how each
capability is wired, the routes). Neither is served without `ALDER_DEV=1`:
`/_dev` is a 404 and no page carries the panel or its script (a test builds
an example, runs it without `ALDER_DEV` and checks both). A `Noticed` answer is unwrapped by the generated route
(`Nav.Unnotice`), the text kept on the request, and the toast patched after the
body.


## 9. Reading generated code

You can read the Go for any app. From the repository root:

```
$ go run ./cmd/alder build --emit examples/todos
wrote build/todos
```

`alder build --emit` refuses a program with errors or holes: it prints the
diagnostics and exits 1 without writing anything. For a hole it adds a
line such as `alder build: the program has 1 typed hole(s); fill them
before building`. When the program is clean, it writes into
`build/<app>/`, where `<app>` is the last part of the directory you gave
it, relative to where you ran the command. `build/` is never edited by
hand, and git ignores it.

`build/todos/` has one Go file per module, named after the module with `.`
replaced by `__` and `_alder.go` added (`Todos__Index_alder.go` for
`Todos.Index`), plus two app-wide files and a `go.mod` that finds the
runtime in `rt/` beside it (section 8, "The runtime travels with the
compiler"). `app__alder.go` holds the lists of section 8
(`Tables`, `Capabilities`), the `Caps` struct the generated `caps` package fills, `Options`,
`Serve` (which checks the capabilities, opens the database, creates the
tables and returns `web.NewRouter(Routes, …)`), `Web` (the router's
options that do not depend on how the app runs, which `Serve` starts
from), and `session__codec`, the cookie's JSON. `routes__alder.go` holds `Routes` and every handler of
section 8's last part. An app with capabilities also gets
`caps/caps__alder.go`, the package `caps` that wires them (chapter 7
quotes the blog's). A full `alder build` (without `--emit`) adds
`cmd/main.go`, which reads the environment and calls
`Serve(caps.Caps(), …)` (or `Serve(app.Caps{}, …)` for an app without
capabilities); `adapters/` and `static/`, copies of the app's own; and
the executable.

The fixed ending is there because Go reads meaning into file names: a file
ending in `_test.go` is a test, and one whose name ends in `_linux` or
`_amd64` (before the first `.`) is compiled only on Linux or only on that
processor. A module named `Foo_test` or `Bar_linux` is a fine Alder module,
and `Foo_test_alder.go` and `Bar_linux_alder.go` are ordinary files to Go.
Every Go file starts with `// Code generated by alder; DO NOT EDIT.`
(`app__alder.go`'s names the compiler's version too: `// Code generated by
alder devel+0be85a2f5d31; DO NOT EDIT.`, chapter 11), and the build runs `go vet` on the package before saying `wrote`. Before
writing, the build deletes the `.go` files in `build/<app>/` that start
with that line (and `go.mod`), so a module you removed or renamed leaves
no file behind; files you put there yourself are kept.

Every generated name follows one scheme (`internal/gen/names.go`), and its
parts are always joined by a double underscore, `__`:

| Go name | what it is | example |
|---|---|---|
| `Module__name` | a top-level declaration: value, function, component, record, union | `Todos__Index__view` (the `view` of `Todos.Index`) |
| `M__Union__Ctor__` | a constructor: its function, or its value when it takes no payload | `M__Shape__Circle__` |
| `payload__M__Union__Ctor` | the struct of a constructor's payload | `payload__M__Shape__Rect` |
| `tag__M__Union` | a union's tag type | `tag__M__Shape` |
| `is__M__Union__Ctor` | a constructor's tag | `is__M__Shape__Circle` |
| `widen__Page__Session` | a `requires` page's session turned into the app's | `widen__Home__Session` |
| `Route__M__page`, `Route__M__Msg` | the route of a page's `GET`, of a message's `POST` | `Route__Todos__Index__Toggle` |
| `page__M`, `update__M`, `params__M` | a page's `requires` and `load`; its message lifecycle; its parameters as text | `page__Todos__Index` |
| `decode__M__X` | the decoder of a page's `Params`, a message's parameters or a form | `decode__Todos__Index__NewTodo` |
| `check__M__Brand` | the check of a brand with a `where` refinement, run on decoded input | `check__Todo__Slug` |
| `route__…`, `session__…` | helpers shared by every route, and the session codec | `route__fail`, `session__codec` |
| `Cap__M__Cap` | a capability's Go interface | `Cap__Auth__Login__Password` |
| `Args__M__Cap__Entry` | the argument record of a capability entry | `Args__Auth__Login__Password__Verify` |
| `M__Cap` | a capability's field of `Caps` | `Auth__Login__Password` |
| `a_<dir>` | the alias of `app/adapters/<dir>` in `caps/caps__alder.go` | `a_auth` |
| first segment, lower-cased; the rest + the capability | the adapter's directory and constructor (`gen.AdapterPkg`, `gen.AdapterFunc`) | `adapters/auth/`, `LoginPassword` |
| `title` → `Title`, `_at` → `U__at` | a record field, exported (a capital first letter, or `U__` for a leading `_`) so that adapters, and the runtime's `Show`, `Eq` and `Key`, see it | |
| `type` → `type__` | a local whose name Go reserves, or that names `rt`, `html`, `std` | |

Why `__`? Your names may not contain it: the compiler rejects `max__len`
(E0001), and a module name segment may not end with `_`. So `__` can only
be a joint, and a generated name can be read back into its parts:
`Todos__Index__view` is module `Todos.Index` and name `view`, never module
`Todos` and a name `Index__view`. Modules `A.B` and `A_B` become
`A__B_alder.go` and `A_B_alder.go`, so they cannot overwrite each other's
file. Names the compiler adds
start with a lowercase word (`tag__`, `is__`, `payload__`, `widen__`,
`page__`, `decode__`, …), which no module name does, so they never clash
with yours either. The names your adapters use must be exported, so they
start with a capital: `Cap__`, `Args__` and `Route__`, and the fixed
`Caps`, `Serve`, `Options`, `Routes`, `Tables` and `Capabilities`. A
type `Password` in a module `Cap.Auth.Login` would be
`Cap__Auth__Login__Password`, the interface of capability `Password` in
`Auth.Login`, so a module whose first segment is exactly `Route`, `Cap` or
`Args` is refused (E0015). The fixed names contain no `__`, so no
declaration of yours can produce them. One more rule keeps `Args__` names
apart: an entry whose name starts with `_` gets an `x` in front
(`_raw` → `Args__M__C__x_raw`), so the part after the last `__` never
starts with `_`. The one
place your names and generated ones share a module is a page module (and
`App`): the compiler generates `Params`, `Msg`, `Session` and `PickParams`
for `msg Pick`, so a type or component of yours may not use those names
(E0012). A generated name can never be a Go keyword or clash with `rt`,
`html` or `std`.

**The build, in order** (`runBuildTo` in `cmd/alder/build.go`):

1. Check the program (section 5), and lower it to the IR (section 6).
2. Check the adapters, before anything is written:
   `buildkit.CheckAdapters` (`internal/buildkit/adapters.go`). It turns
   each capability of the IR's `Program.Caps` into a `Need`: the module,
   the capability, the directory (`gen.AdapterPkg`: the module's first
   segment, lower-cased), the function (`gen.AdapterFunc`: the other
   segments and the capability's name, joined) and the interface. Two
   needs with the same directory and function are the "internal naming
   error" (`gen.AdapterCollision`), checked first; then a directory named
   `internal` or `testdata` is refused, and so is an `adapters/go.mod`.
   Then the scan. It lists `adapters/` and wants each needed directory
   by its exact name, as a real directory: a symbolic link is skipped
   (as the copy below skips it), and a directory that differs only in
   letter case is refused by name, since on a case-insensitive file
   system it would pass the scan and then confuse Go. In each directory
   it parses, with Go's own parser (`go/parser`, which reads a file's
   declarations without type-checking anything), every regular `.go`
   file that is not a test and that Go would build here
   (`build.Default.MatchFile` applies build tags and `_linux`-style file
   names), and collects the functions that are exported, have no
   receiver, no type parameters and no parameters. A need whose function
   is not among them is missing; when the package declares the name as
   something else (a function with parameters, a variable, a type), the
   `Need` keeps that near miss in `Found` for its line. A file that does
   not parse counts as having every function, so that Go, later, reports
   the syntax error itself. Every missing need is printed, and the build
   exits 1.
3. Emit the Go (section 7); `gen.Emit` adds the `caps` file
   (`gen.CapsPackage`) when the program has capabilities. Write it
   (`buildkit.WriteGenerated`, which also removes a stale `caps/`).
4. Copy `migrations/`, then the whole `adapters/` tree
   (`buildkit.CopyAdapters`, which first removes, by exact name, what
   the app no longer has, so a directory renamed only in its case is not
   lost on macOS, then copies, skipping a file whose content is
   unchanged, so Go's build cache stays warm) and `static/`, and write
   `cmd/main.go` (`gen.Main`).
5. `go vet ./...` on the module, then `go build ./cmd`. When vet fails,
   the build vets the generated package alone to decide whose fault it
   is: if that fails too, it is an internal error (exit 2); otherwise the
   adapters (or the wiring of their constructors) do not compile, which
   is your Go, and exit 1 with Go's output
   (`buildkit.AdaptersCompileMessage`).

The test builds of `alder test`, `alder eval` and the REPL (section
"Running tests", above) follow the same steps through `scratch.CompileCheckedWith`,
with two differences. Step 2 is `buildkit.CheckTestAdapters`: every
refusal stands, but a missing need is returned instead, its key goes into
`gen.Options.Unwired`, `gen.CapsPackage` leaves that field of `Caps` out
(with a comment) and does not import a package nothing else needs, and
the warning goes into `Build.Warning` for the command to print. In step 5,
when the generated package vets but the module does not, the scratch
runner vets `./caps` and `./adapters/...` alone: if they fail it is a
`scratch.AdapterError` with step 5's message (the commands exit 1),
otherwise the fault is the scratch `main`'s.

Because every name follows the scheme, finding a page's view is a search:

```
$ grep -n "func Todos__Index__view" build/todos/*.go
build/todos/Todos__Index_alder.go:133:func Todos__Index__view(model Todos__Index__Model, session Todos__Index__Session) html.Node {
```

Its first lines show the `<Layout.Page>` component call and the `<form>`
with its message, an `rt.MsgRef` that becomes a URL only when the page is
rendered (section 8). The props are an anonymous struct type followed
directly by its value, `struct { … }{Title: "Todos", …}`, and the children
are a slice:

```go
func Todos__Index__view(model Todos__Index__Model, session Todos__Index__Session) html.Node {
	return Layout__Page(struct {
		Title    string
		Children []html.Node
	}{Title: "Todos", Children: []html.Node{
		html.El("h1", nil, html.Text("Todos")),
		html.FormTo(rt.MsgRef{Module: "Todos.Index", Msg: "Submit"}, nil,
			html.Input(html.Control{Kind: "input", Form: "Todos.Index.NewTodo", Name: "title", Type: "text", Label: "What needs doing?", Required: true}),
```

Compare it with the `view` in chapter 2. `session` has type
`Todos__Index__Session`, not the app's session: the page `requires user`, so
it gets its own session record in which `user` is a `UserId`, not a
`Maybe UserId`. Where the page passes `session` to code that expects the
app's session, the call goes through `widen__Todos__Index__Session`, which
the emitter writes into `Todos__Index_alder.go`: it copies the fields and wraps
`user` back in `rt.Just`.

## 10. The language server

The stages so far run once: `alder check` reads the files, checks them and
exits. An editor wants the same answers all day, about files that are half
written, a keystroke after each change. `alder lsp` (`internal/lsp`, started
by `cmd/alder/lsp.go`) is the compiler kept running for that. It adds no
new rules about Alder. It runs the stages you have read about on the text in
the editor and translates their answers (a span, a type, a declaration) into
the editor's protocol. Chapter 11, "Editor support", is the user's view;
this section is the inside.

The transcripts below are real. They were recorded by running Neovim 0.12.5
with alder.nvim on a copy of `examples/blog`, with `ALDER_LSP_LOG` set to a
file: `alder lsp` copies every message it receives (`->`) and sends (`<-`)
to that file, one JSON object per line. Long values are trimmed, and
each cut is marked `…`; whole messages are left out only where the text
says so; and the directory the copy lived in is written `…`. Nothing else is changed:
the lines are the log's, byte for byte, apart from those cuts. (Two
sessions are quoted, each a fresh run on a fresh copy of the blog, so
the ids start again at 1 in each.)

### The wire

An editor and a server talk over two pipes, the server's standard input and
standard output. A pipe is a stream of bytes with no message boundaries, so
the protocol has to say where one message ends. It borrows HTTP's way: a
header, a blank line, then a body whose length the header gave.

```text
Content-Length: 445\r\n
\r\n
{"id":1,"jsonrpc":"2.0","result":{"capabilities":{"codeActionProvider":{"codeActionKinds":["quickfix","refactor"]},"completionProvider":{"triggerCharacters":[".","\u003c","?"]},"definitionProvider":true,"executeCommandProvider":{"commands":["alder.explain"]},"hoverProvider":true,"positionEncoding":"utf-8","referencesProvider":true,"textDocumentSync":{"change":1,"openClose":true}},"serverInfo":{"name":"alder","version":"devel+7747fd1d9993"}}}
```

That is the server's answer to `initialize`, the first request. The log
holds only the bodies; the header is what `conn.send` writes in front of
one, and 445 is the length of this body in bytes (bytes, not characters,
which matters as soon as a message has an `é` in it). The body is JSON. (The
`\u003c` is Go's JSON encoder writing `<`, which it escapes by default; it
means the same character.) `readMessage` in
`internal/lsp/jsonrpc.go` is the reading side:

```go
func readMessage(r *bufio.Reader) (*message, error) {
	h, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(h.Get("Content-Length"))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("bad Content-Length %q", h.Get("Content-Length"))
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return &message{Error: &rpcError{codeParse, err.Error()}}, nil
	}
	return &m, nil
}
```

It reads the header lines with Go's MIME header reader (the same format as
HTTP's), takes the one header it cares about, `Content-Length`, reads exactly
that many bytes, and decodes them. A body that is not JSON becomes a
message that carries a parse error, so the server can answer it and read on.
A broken header is an error that ends the loop: after it the server no
longer knows where messages start, and cannot recover.

The body is **JSON-RPC 2.0**, three kinds of message in one struct:

```go
type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}
```

A **request** has an `id` and a `method`; the receiver must answer it with a
**response** carrying the same `id` and either a `result` or an `error`. A
**notification** has a `method` and no `id`: "this file changed" needs no
answer. The `id` lets a client have several requests in flight and match
answers to them as they come in any order. The server keeps the id as the
JSON text it arrived in (`idKey`), so the number 1 and the string "1" stay
different ids.

The beginning of a session, from the log. The request, with the editor's
capabilities cut:

```text
-> {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":3560,"clientInfo":{"version":"0.12.5","name":"Neovim"},"rootPath":"…/blog","rootUri":"file://…/blog","workDoneToken":"1","trace":"off","workspaceFolders":[{"uri":"file://…/blog","name":"…/blog"}],"capabilities":…}}
<- {"id":1,"jsonrpc":"2.0","result":{"capabilities":{"codeActionProvider":{"codeActionKinds":["quickfix","refactor"]},"completionProvider":{"triggerCharacters":[".","\u003c","?"]},"definitionProvider":true,"executeCommandProvider":{"commands":["alder.explain"]},"hoverProvider":true,"positionEncoding":"utf-8","referencesProvider":true,"textDocumentSync":{"change":1,"openClose":true}},"serverInfo":{"name":"alder","version":"devel+7747fd1d9993"}}}
-> {"jsonrpc":"2.0","method":"initialized","params":{}}
-> {"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"languageId":"alder","text":"module Posts.Index\n\nimport Post\n…","uri":"file://…/blog/app/Posts/Index.alder","version":0}}}
<- {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"file://…/blog/app/Posts/Index.alder","version":0,"diagnostics":[]}}
```

`initialize` tells the server where the project is (`rootUri`), and the
server tells the client what it can do (`capabilities`): hover, definition,
references, completion after the three characters `.`, `<` and `?`, code
actions of two kinds, one command. `textDocumentSync: {change: 1}` asks the
client to send the **whole** text of a file at each change, not a patch:
Alder files are small, and a patch applied to a wrong base would corrupt the
server's copy for good. After `initialized` the editor opens the file with
`didOpen` and sends its entire text, and the server, which has now built the
app, publishes the file's diagnostics (none). Two messages in that exchange
are the reason for two of the server's rules, which the next paragraphs
explain: `"positionEncoding":"utf-8"` and the whole `text` in `didOpen`.

**Why utf-8 positions.** An LSP position is a line and a character offset in
that line, both counted from 0. The protocol's historical unit for the
offset is the UTF-16 code unit, a unit that nothing in Alder uses. The
compiler counts lines from 1 and columns from 1 in **bytes** of the UTF-8
file (`diag.Pos`), the unit Go strings index by. Since LSP 3.17 a server can
announce a different unit, so `alder lsp` announces UTF-8 bytes, and the
conversion is `line-1` and `col-1`, both ways:

```go
func lspRange(line, col, endLine, endCol int) Range {
	pos := func(l, c int) Position { return Position{Line: max(l-1, 0), Character: max(c-1, 0)} }
	return Range{Start: pos(line, col), End: pos(endLine, endCol)}
}
```

The alternative is converting to and from UTF-16 on every position, and a
line like `"hé {name}"` is where such a conversion goes wrong. With bytes
there is no conversion to get wrong. (A client that can only count UTF-16
units is not served; Neovim speaks UTF-8.)

### The overlay

A language server must check what is in the editor, not what is on disk. The
file you are typing in has not been saved, and the other files may have
changes of their own in other buffers. The server keeps every open document
in memory (`docs` in `internal/lsp/docs.go`): `didOpen` and `didChange` put
the whole text there, `didClose` takes it out, and the file on disk applies
again.

The compiler, though, reads files. `names.Load` (section 4) walks `app/`,
reads every `.alder` file and parses it. It does take a list of extra
modules, parsed by the caller from memory, and that is how `alder eval` and
the REPL add the scratch module that holds your expression. That hook does not work for an editor, because
extra modules are **added** to the directory's under the duplicate rule: an
extra `Posts.Index` next to the file `app/Posts/Index.alder` is two
definitions of `Posts.Index`, E0009. What an editor needs is to **replace**
the file. `names.LoadOverlay` does that:

```go
		r, _ := filepath.Rel(dir, path)
		rel := filepath.ToSlash(r)
		if src, ok := overlay[rel]; ok {
			parse(rel, src)
			return nil
		}
		src, err := os.ReadFile(path)
```

`overlay` maps a path relative to the project ("app/Posts/Index.alder") to
the text to use. The walk visits each file on disk; when the overlay has a
text for it, it parses that and never reads the disk. An overlay file that
the walk did not find (a new buffer, not saved yet) is added after the
others, in path order. `names.Load` is `LoadOverlay` with a nil overlay, so
the compiler and the server run the same code, and
`buildkit.LoadCheckedOverlay` is `LoadChecked` with an overlay, which
`alder check` calls with none.

A URI the editor sends (`file:///…/blog/app/Posts/Index.alder`) must become
that relative path, and it must not escape the project. `server.rel` in
`docs.go` resolves symbolic links on both sides first, because macOS's
`/tmp` is a link to `/private/tmp` and a root the editor spelled one way
would otherwise never match a file it spelled the other way, and it refuses
any path that does not stay inside the root.

### Snapshots and the last good build

An editor sends a change on nearly every keystroke. Checking the whole app
is fast (chapter 11 calls `alder check` a matter of milliseconds), but not
free, so checking on each keystroke would waste most of its work, and
checking on the goroutine that reads messages would stop the server from
reading while it checked. The server splits the work between goroutines
(Go's lightweight threads: functions that run at the same time as the code that started them):

- the **reader** reads messages, one at a time, and applies document
  notifications at once, in the order they arrive. A `didChange` replaces
  the text and asks for a rebuild, which costs one send on a channel;
- the **analysis goroutine** builds the program, 150 milliseconds after the
  last change;
- two **workers** answer requests: a fast one for hover, definition,
  references, completion, code actions and the generator list, and a slow one
  for the generator dry runs (below), so that a dry run never makes a hover
  wait.

Asking for a rebuild is deliberately weak:

```go
func (s *server) schedule() {
	select {
	case s.tokens <- struct{}{}:
	default:
	}
}
```

`tokens` is a channel with room for one value. If it is empty, the send puts
a token in. If a token is already waiting, the `default` branch does
nothing: a rebuild is already due, and it will see this change too. Ten
keystrokes before the analysis goroutine wakes are one rebuild. `analyse`
then sleeps `Debounce` (150 ms, set in `cmd/alder/lsp.go`), swallows the
tokens that arrived meanwhile, and calls `rebuild`: typing a word is one
build, not five.

The editor is not the only thing that changes files. A poller (`internal/watch`,
the same one `alder run` uses) looks at the project's files on disk every 300
ms and asks for a rebuild when one changes, so a `git checkout` or a generator
run in a terminal shows up without a keystroke.

`rebuild` takes a snapshot of the documents, builds the app with them
overlaid, and stores the result:

```go
	over, vers := s.docs.snapshot()
	prog, res, bag, ioErrs := buildkit.LoadCheckedOverlay(s.rootReal, over)
	…
	buildkit.AddSchemaDrift(s.rootReal, prog, res, bag)
	a := &analysis{prog: prog, res: res, diags: bag.Items, versions: vers, lines: map[string][]string{}}
	…
	s.cur.Store(a)
	if res != nil {
		s.good.Store(a)
	}
	s.publish(a)
```

(`AddSchemaDrift` adds E0019, the migrations check, so the diagnostics are
those of `alder check`.) An `analysis` holds the program, the type
checker's result, the diagnostics, the version of every document it was
built from, and the text of every module as the build read it (`lines`).
Nobody changes it after `Store`. It is a **snapshot**, and a snapshot can be
read by any number of goroutines without a lock; `cur` and `good` are
`atomic.Pointer`s, so a worker picking the current snapshot never sees half
of a new one. A request that arrives during a rebuild answers from the
previous snapshot, and the next request sees the new one.

There are two pointers because a snapshot can be useless. If the text does
not parse, `LoadCheckedOverlay` does not type-check (the checker needs a
tree), so `res` is nil: the snapshot has the parse error to show, but no
types, no resolved names, nothing to hover on. `cur` is always the newest
snapshot, and `good` is the newest one **with** a `res`, the **last good
build**. Diagnostics are published from the newest; hover, definition,
references and completion answer from the last good one. Without that, every
time you typed a half line, everything would go dark until you finished it.

But the last good build was made from the text as it was, not from the text
as it is, so it can answer wrongly. If you insert a line above `lastPage`,
the build still says `lastPage` is on line 22; and the cursor, which the
editor reports in today's coordinates, is on what used to be line 23. The
rule that prevents the wrong answer (**R11**) is called freshness:

```go
func freshLines(a *analysis, rel string, now []string, line int, inclusive bool) bool {
	…
	last := line - 1
	if inclusive {
		last = line
	}
	…
	for i := 0; i < last; i++ {
		if built[i] != now[i] {
			return false
		}
	}
	return true
}
```

A position is **fresh** if the lines above it are the same text in the
build and now. Then every line number and byte column above the cursor means
the same thing in both, and the build's answer about the cursor's line holds
too, if that line itself is unchanged (`inclusive`). Hover, definition and
references ask for the cursor's line to be unchanged as well. Completion
does not, because the line under the cursor is the one being typed and is
usually not parseable. It asks that only the lines above it are unchanged,
and reads the line itself from the text.

Here is the rule at work, from the log. In `load`, line 31,
`let q = Text.slice 0 100 (Text.trim params.q)`, is replaced by `let q = Text.`,
as if it were being typed. The editor sends `didChange` (version 5) with the whole
new text, then asks for a hover on `lastPage` (request 3) and for completion
after `Text.` (request 4). The server answers both from the build it made
before the edit, since the 150 ms have not passed; then the rebuild runs and
publishes the parse error, and a hover below the edit (request 6) gets
nothing. (Between these the log has a second, identical hover on `lastPage`, and,
at the end, the `didChange` that puts the original line back; both are left
out.)

```text
-> {"jsonrpc":"2.0","method":"textDocument/didChange","params":{"contentChanges":[{"text":"module Posts.Index\n…"}],"textDocument":{"uri":"file://…/blog/app/Posts/Index.alder","version":5}}}
-> {"jsonrpc":"2.0","id":3,"method":"textDocument/hover","params":{"position":{"line":21,"character":2},"textDocument":{"uri":"file://…/blog/app/Posts/Index.alder"}}}
<- {"id":3,"jsonrpc":"2.0","result":{"contents":{"kind":"markdown","value":"```alder\nlastPage : Int\n```\n\nlastPage bounds `page`, so `(page - 1) * perPage` cannot overflow."}}}
-> {"jsonrpc":"2.0","id":4,"method":"textDocument/completion","params":{"position":{"line":30,"character":15},"textDocument":{"uri":"file://…/blog/app/Posts/Index.alder"}}}
<- {"id":4,"jsonrpc":"2.0","result":{"isIncomplete":false,"items":[{"label":"NonEmpty","kind":7,"textEdit":{"range":{"start":{"line":30,"character":15},"end":{"line":30,"character":15}},"newText":"NonEmpty"}},…,{"label":"trim","kind":3,"detail":"trim : Text -\u003e Text","textEdit":{"range":{"start":{"line":30,"character":15},"end":{"line":30,"character":15}},"newText":"trim"}},…]}}
<- {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"file://…/blog/app/Posts/Index.alder","version":5,"diagnostics":[{"range":{"start":{"line":30,"character":15},"end":{"line":30,"character":16}},"severity":1,"code":"E0001","source":"alder","message":"in the definition of `load`: expected field name, got end of line"}]}}
-> {"jsonrpc":"2.0","id":6,"method":"textDocument/hover","params":{"position":{"line":37,"character":36},"textDocument":{"uri":"file://…/blog/app/Posts/Index.alder"}}}
<- {"id":6,"jsonrpc":"2.0","result":null}
<- {"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"file://…/blog/app/Posts/Index.alder","version":6,"diagnostics":[]}}
```

Positions are 0-based: line 21 is the file's line 22, `lastPage =
1000000`, and line 30 is line 31, the one being typed. The new text does not
parse (E0001), so the rebuild makes a snapshot with no `res`, and no new good
build exists: every answer comes from the one made before the edit, and the
freshness rule decides whether it still applies. It compares the build's lines
with the text as it is **now**, not as it was at the last rebuild, which is
why requests 3 and 4 are right even though the rebuild had not run yet.

- Hover on `lastPage` (request 3, line 22) is **above** the edit: lines 1 to
  22 are identical, so the position is fresh and the old build's answer is
  right, with its doc comment.
- Completion after `Text.` (request 4, line 31) is **on** the edit. Lines 1
  to 30 are unchanged, which is all completion asks. The context, "a member of
  module `Text`", came from the text before the cursor (`Text.`, matched by
  the regular expression `reMember`), and the members from the module's
  declarations in the build.
- Hover on line 38 (request 6) is **below** the edit. Line 31 differs from
  the build's, so nothing from the build holds at line 38, and the server
  answers `null`, "nothing here". It would be wrong to answer: had the edit
  added a line, the build would describe a different place.

When the original line is put back, the next rebuild type-checks, becomes the new last
good build, and the server publishes `"diagnostics":[]` with version 6.
Definition and references are held to the same rule for every place they
return: a target in a file whose lines above it have changed since the build
is left out (`server.location`), rather than sent to the wrong line.

### From a position to a node

To answer "what is at line 33, column 21?", the server needs the tree node
there. `syntax.NodeAt(m, pos)` (`internal/syntax/walk.go`) returns the path
from the module down to the innermost node whose span contains the position:

```go
func NodeAt(m *Module, pos diag.Pos) []Spanned {
	path := []Spanned{m}
	for {
		var next Spanned
		for _, c := range Children(path[len(path)-1]) {
			sp := c.GetSpan()
			if sp.Start == sp.End {
				continue
			}
			if Contains(sp, pos) {
				next = c
			}
		}
		if next == nil {
			return path
		}
		path = append(path, next)
	}
}
```

It starts at the module, finds the child that contains the position, and
descends into it until no child does. The path matters more than the last
node: the meaning of a name depends on what surrounds it, which parameters
are in scope, which arm of a `match` it is in.

`Children` needs a list of every node's children, and `Spanned` is the
interface of every node: anything that embeds `Node`, with its `Span`. There
are dozens of node types, and the parser adds one whenever the language gains a
construct. A hand-written `Children` would need a case for each, and the day
someone adds a node and forgets the case, the node's insides become invisible
to hover and references, silently. So `Children` finds the children by
**reflection**, Go's ability to look at a value's type and fields at run
time: it looks at every exported field of a node, takes a field holding a
node, or a slice of nodes, or a plain struct that holds nodes (a string
interpolation's part), skips nil, and sorts what it found by source
position. A new node type with ordinary fields is covered the moment it
exists. Reflection is slower than a hand-written walk, and a lookup walks a
path of a few dozen nodes, not the whole tree.

What the reflection cannot know is whether the spans are right: a child
must lie inside its parent, or `NodeAt` would descend into the wrong
place. `TestChildren_SpansNest` (`internal/syntax/walk_test.go`) parses
every module of every example app and checks it for each child of each node.
The guarantee is that every node reachable through exported fields has a
span inside its parent's, on every file in `examples/`; it does not
guarantee that a new field is exported, or that a parser fills a span.

### Scope, hover and definition

Take the cursor on the `q` of `Text.isEmpty q` in the blog's `load`, line 33:

```alder fragment
  let matching =
    if Text.isEmpty q then Db.and [] else Db.or [Post.title.contains q, Post.body.contains q]
```

The editor asks for hover, and asks for definition, at line 32, character 20
(0-based):

```text
-> {"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":"file://…/blog/app/Posts/Index.alder"},"position":{"character":20,"line":32}}}
<- {"id":2,"jsonrpc":"2.0","result":{"contents":{"kind":"markdown","value":"```alder\nq : Text\n```"}}}
-> {"jsonrpc":"2.0","id":3,"method":"textDocument/definition","params":{"textDocument":{"uri":"file://…/blog/app/Posts/Index.alder"},"position":{"character":20,"line":32}}}
<- {"id":3,"jsonrpc":"2.0","result":[{"uri":"file://…/blog/app/Posts/Index.alder","range":{"start":{"line":30,"character":6},"end":{"line":30,"character":7}}}]}
```

How the server gets from the first to the second:

1. `server.at` (`scope.go`) takes the last good build, checks freshness
   (inclusive: line 33 must be unchanged), finds the module for the file,
   adds one to line and column to get the compiler's position `33:21`, and
   calls `NodeAt`. The path is `Module`, `ValueDecl` (`load`), `Block`,
   `LetStmt` (`matching`), `If`, `App` (`Text.isEmpty q`), `Var` (`q`).
2. `target` (`scope.go`) says what the node names. The `Var` carries a
   `Ref`, the resolver's answer (section 4) for the name: its kind is
   `RefLocal`, "a local variable", and a local has no module or declaration
   to point at. Which local? The resolver says only that it is local; the
   server asks `scopeAt`.
3. `scopeAt(path, pos)` walks the path from the outside in and collects the
   names in scope. At the `ValueDecl` it adds `load`'s parameters, `params` and
   `session`. At the `Block` it adds each `let` whose statement **ends before**
   `pos`:

   ```go
   		case *syntax.Block:
   			for _, st := range n.Stmts {
   				if !posLess(pos, st.Span.End) {
   					out = append(out, binder{st.Name, st, wordSpan(m, st.Span, len("let"), st.Name)})
   				}
   			}
   ```

   That adds `page` and `q`, and not `matching`, which is the statement the
   cursor is in: a `let` cannot use itself. An `Arm` adds its pattern's
   variables for its body, a lambda its parameters, a `for` its variable.
   The list is ordered outermost first, and `innermost` takes the **last**
   binder named `q`, so a name that is bound twice means the closer one,
   as in the language.
4. The binder is `q`, a `LetStmt` on line 31 whose name is at columns 7 to 8.
   Its `Key` is the node itself, and the type checker recorded each local's
   type under that key (`check.Result.Binds`). Hover prints
   `types.StringIn(t, m.Name)`, the type with names relative to this module,
   and the answer is `q : Text`. Definition returns the binder's span, shifted by
   `lspRange` to line 30, characters 6 to 7.

The same `target` drives three features. Hover on `lastPage` finds a `Var`
with a `RefValue` instead, looks its type up in `Result.Values`, and adds
the doc comment (`syntax.DocOf`: the run of `--` lines directly above the
declaration). Definition on it finds the declaration (`findDecl`) and returns
the span of its name. A name that resolves to the standard library gets a type
and an answer for hover, and no definition: the stdlib is embedded in the
binary, so it has no path to open.

### References

`references` asks the reverse question: where is this declaration used? The
compiler has no index from declarations to their uses, so the server walks
every module's tree, node by node, and asks for each name node whether it
resolves to the same declaration (`uses` in `references.go`):

```go
	for _, name := range a.prog.Order {
		m := a.prog.Modules[name]
		if m == nil || local != nil && m.File != local.Span.File {
			continue
		}
		var walk func(path []syntax.Spanned)
		walk = func(path []syntax.Spanned) {
			n := path[len(path)-1]
			if sp, ok := useSpan(m, n, path, ref, local); ok {
				sp.File = m.File
				out = append(out, sp)
			}
			for _, c := range syntax.Children(n) {
				walk(append(path[:len(path):len(path)], c))
			}
		}
		walk([]syntax.Spanned{m})
	}
```

The test in `useSpan` is the resolver's own answer: the node's `Ref` has the
same module and name as the target's, in the same **namespace** (a table
`Post` used in a type position and used as a value are the same declaration
with different `RefKind`s). A local compares its binder by identity, not
its name: two `q`s in two functions are two names.

The cost is a walk of the whole program per request: every node of every
module, plus a reflective `Children` call per node. There is no index to build
and no index to get out of date, and for an app the size of the blog it is
fast enough to answer a keypress. An app ten times larger takes about ten
times as long, which is the point at which an index kept per snapshot would
start to pay for itself. The server does not have one yet.

### Completion

Completion is the one feature that cannot wait for a good build, because it is
used on the line that does not parse. So it reads the context from the text,
and the facts from the build.

`completionAt` takes the line up to the cursor (`prefix`) and tries the
contexts in order: `import `, a module member (`Text.`), a field (`params.`),
a closing tag (`</`), an attribute inside a tag, a tag (`<`), a hole (`?`), a
pattern, and last everything else (names in scope, modules and keywords). The
first whose regular expression matches the prefix wins. These are text matches
on purpose; the parser has failed on this line, and its error is no help
deciding what the user means to write. The expressions are anchored on the
end of the prefix:

```go
	reMember   = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])([A-Z][A-Za-z0-9]*(?:\.[A-Z][A-Za-z0-9]*)*)\.([a-zA-Z0-9_]*)$`)
	reField    = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])([a-z][A-Za-z0-9_]*)\.([a-zA-Z0-9_]*)$`)
```

`Text.` is a capitalised name, a module, so `reMember` matches and `members`
lists the declarations of that module. `params.` is a lowercase name, a
value, so `reField` matches, and `fields` needs the type of `params`. For
that, the scope: on the line being typed, the scope at the **end of the line
above**, from the last good build (`scopeNow`). That contains the names
bound before this line, which is all this line can use, and the type of
`params` is `Params`, a record with the fields `page` and `q`, which are the
completions.

Both are gated on freshness: if the lines above the cursor have changed since
the build, the build's scope would be about different code and the server
offers nothing from it.

**Fitting a hole.** Put the cursor on `?n` in `Db.limit ?n`. The hole has a
type the checker inferred, here `Int`, and the useful completions are the
names whose type fits. `fits` asks the unifier (section 5):

```go
func (cx *completion) fits(name string, want types.Type) bool {
	…
	u := cx.a.res.U.Clone()
	return u.Check(u.Instantiate(t), want) == nil
}
```

For each name in scope it takes the name's type, makes the type's type
parameters fresh variables (`Instantiate`: a function over any list fits
whether the hole wants a list of `Int` or of `Text`), and asks whether it can be made equal to the hole's type. Unifying
**changes** the unifier: a type variable that was open becomes `Int`. The
build's unifier belongs to a snapshot that other requests are reading, and a
candidate that did not fit would leave a half-finished substitution behind.
So each question gets `Clone`, a copy with its own substitution (it carries
on counting fresh variables from the original's counter, so none collide),
and the copy is thrown away. The names that fit are sorted first
(`sortText` starting with 0) and the others after them (1). The item's edit
replaces the whole hole, `?` included, so choosing a name fills it.

### Generators

The generators of chapter 15 started as commands. The editor shows them in
menus, with forms and previews, so the generator must be described as data,
not as a command line to parse. A `generate.Spec` (`internal/generate/spec.go`)
is that description:

```go
type Spec struct {
	ID, Summary string
	Args, Flags []Field
	Here        func(HereCtx) *Match // nil: never "here"
	Run         func(sets Sets, app App, dryRun bool) (*Result, error)
}
```

`ID` is `"new msg"`, `Args` and `Flags` are the typed fields (`page` is a
`page`, `name` is text), `Run` is the generator, and `Here` says whether the
generator applies at the cursor and prefills a form if it does. `HereCtx` is
where the cursor is (module, whether it is a page, the declaration around it,
a hole under it); `inPage`, for example, is the `Here` of `new msg`:

```go
func inPage(c HereCtx) *Match {
	if !c.IsPage {
		return nil
	}
	return &Match{Sets: map[string]string{"page": c.Module}, Why: "the page at the cursor"}
}
```

The command line builds a `Sets` from the arguments and flags it parsed
against the same `Spec`; `alder help --json` renders the same `Spec`s; the
server's `alder/generators` lists them with those whose `Here` matched first.
Three programs, one description.

The server has a fourth custom request that needs no `Spec`: `alder/routes` returns
the app's pages and messages (`buildkit.Routes`, which `alder routes` prints too)
with their places, for an editor's route picker (chapter 11).

**A dry run on a copy.** A generator edits files, checks the result, and
refuses if it does not check. `Run(…, dryRun=true)` does all of that and
writes nothing. The server calls it on a copy of the app:

```go
	tmp, cleanup, err := generate.Materialise(s.rootReal, s.docs.overlay())
	if err != nil {
		return spec, nil, nil, false, err
	}
	defer cleanup()
	p, _ := generate.Load(tmp) // nil when the app does not check: fields still validate syntactically
	fields, sets, ready := generate.Form(spec, here, you, p)
	if !ready {
		return spec, fields, nil, false, nil
	}
	res, err = spec.Run(sets, generate.App{Dir: tmp, Migrate: s.o.Migrate}, true)
```

`Materialise` copies the project into a fresh temporary directory (not
`build/`, `.git`, or databases), then writes the open documents over the
copy. That is the overlay again, but here by writing files: the generators
read a directory, not a map of texts, so the dry run gets a directory that
looks the way the editor sees it, and the real project, which the user may be
looking at, is never touched. The migration writer runs in the copy too. The
`Migrate` function is passed in (`generate.App.Migrate`) so the server and the
command line run the same one.

`generate.Form` turns what the user has typed (`you`) over what `Here`
prefilled into a list of fields, each valid or not and why, and `ready` is
true when all are valid. The editor calls `alder/preview` as the form is
filled, once per field. Here is the walk-through of the spec: `new msg` on
`Posts.Index`, with the cursor on line 26. The first request has no values; the
form comes back with `page` prefilled from the cursor and `name` required:

```text
-> {"jsonrpc":"2.0","id":9,"method":"alder/preview","params":{"sets":{},"textDocument":{"uri":"file://…/blog/app/Posts/Index.alder"},"id":"new msg","position":{"line":25,"character":0}}}
<- {"id":9,"jsonrpc":"2.0","result":{"fields":[{"name":"page","kind":"page","flag":false,"required":true,"value":"Posts.Index","source":"here","valid":true,"choices":["Admin.Comment.Bulk",…,"Posts.Index","Posts.Show","Write","Write.Edit"]},{"name":"name","kind":"text","flag":false,"required":true,"value":"","source":"","valid":false,"why":"required"},{"name":"fields","kind":"fieldList","flag":false,"required":false,"value":"","source":"","valid":true},…],"files":[],"diff":"","diagnostics":[],"holes":[],"ready":false,"error":""}}
```

`"source":"here"` says that `page` came from the cursor, not from the user.
The user types `Delete`:

```text
-> {"jsonrpc":"2.0","id":10,"method":"alder/preview","params":{"sets":{"name":"Delete"},"textDocument":{"uri":"file://…/blog/app/Posts/Index.alder"},"id":"new msg","position":{"line":25,"character":0}}}
<- {"id":10,"jsonrpc":"2.0","result":{"fields":[…,{"name":"name","kind":"text","flag":false,"required":true,"value":"Delete","source":"you","valid":true},…],"files":[{"path":"app/Posts/Index.alder","action":"edited","added":17,"removed":0}],"diff":"--- a/app/Posts/Index.alder\n+++ b/app/Posts/Index.alder\n@@ -51,9 +51,12 @@\n …","diagnostics":[…],"holes":[{"module":"Posts.Index","decl":"update","arm":"Delete","name":"delete","type":"Task Nav","file":"app/Posts/Index.alder","line":59,"col":15},{"module":"Posts.Index","decl":"\"pressing Delete\"","name":"expected","type":"Bool","file":"app/Posts/Index.alder","line":213,"col":16}],"ready":true,"error":""}}
```

Now `ready` is true, the copy checked, and `diff` is the unified diff, which
the editor shows (here with its `\n` turned into line breaks):

```text
--- a/app/Posts/Index.alder
+++ b/app/Posts/Index.alder
@@ -51,9 +51,12 @@
 -- whose address (/?q=…) can be bookmarked and shared.
 msg Search form SearchForm
 
+msg Delete
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Search p -> Task.succeed (Redirect (Posts.Index { page: 1, q: p.form.q })),
+  Delete p -> ?delete,
 }
 
 view : Model -> Session -> Html Msg
@@ -88,6 +91,8 @@
       Just _ -> <div class="account">{if model.canWrite then <a href={Write {}}>Write</a> else Html.none} <al-button variant="outline" size="sm" click={App.Logout}>Log out</al-button></div>,
       Nothing -> <p><a href={Auth.Login {}}>Log in</a></p>,
     }}
+
+    <al-button click={Delete}>Delete</al-button>
   </Layout.Page>
 
 test "the front page lists only published posts, newest first" = {
@@ -195,3 +200,15 @@
   let out <- Test.click App.Logout
   Task.succeed ({ out: out.answer, user: out.session.user, role: out.session.role } == { out: Redirected (Auth.Login {}), user: Nothing, role: Reader })
 }
+
+-- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
+-- deleted the page's row, and `after.doc` its HTML), as
+-- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Delete" = {
+  let _ <- Test.get (Posts.Index {})
+  let r <- Test.click Delete
+  let after <- Test.open (Posts.Index {})
+  Task.succeed ?expected
+}
```

That is a `msg`, its arm of `update` with a hole, a button, and a test
with a hole, which is chapter 15's `alder new msg`, and the two holes
(W0002) are listed with their types so that the editor can offer to fill them. The
`diagnostics` in the response (the two W0002 warnings for the holes, cut above)
are the copy's, and the server publishes them nowhere: they describe the
dry-run copy, not the buffers.

The slow worker runs `alder/preview`, so it cannot stall hover, and a
second guard keeps previews in order. While the user types the name, the
editor sends a preview for each keystroke; a dry run takes longer than a
keystroke; so the answers could return out of order and a stale diff would
land on top of a fresh one. `dispatch` remembers the id of the newest
`alder/preview` (`latestPreview`), and `handle` answers an older one with
`-32800`, "cancelled", before it runs, or after it has run if a newer one
arrived meanwhile.

**Why the plan carries the texts.** `alder/preview` answers with a diff,
which is for a person to read. Applying a change needs more. The editor
puts text into its buffers, and a diff tells it which lines changed in a
file as the copy had it, not whether the buffer still looks like that. So
`alder/plan` answers with, for every file, its text before and its text
after:

```go
type planFile struct {
	Path   string  `json:"path"`
	Action string  `json:"action"`
	Before *string `json:"before"`
	After  *string `json:"after"`
}
```

`generate.FileChange` (`internal/generate/apply.go`) records both texts as the
generator wrote the copy. The client does three things with them that a
diff would not allow. It compares `before` with the buffer, and if they
differ, someone edited the file since the plan, and it refuses:
`alder: app/Posts/Index.alder changed since planning; nothing applied`. It
replaces the buffer's text with `after`, a whole-file replacement with no
patch to misapply. And the two pointers tell it the action: `before` null
means "create this file" (an unsaved buffer), `after` null means "remove
it". The server writes nothing in your project at any point. The editor writes
when you save.

The whole chain, for the story of one keystroke: the editor sends the
buffer, the reader stores it, the analysis goroutine rebuilds and
publishes a diagnostic, a worker answers a hover from the last good build if
its lines are fresh, and a dry run on a temporary copy of the same text
answers a generator. At no point is a file of the project written or an
analysis changed in place; the next change makes another snapshot, and the
old one stays valid for whoever still holds it.

### Scratch files in the language server

A scratch file is not a module, so the analysis of this section (a snapshot of
every module) has nothing to say about it. The server runs it as `alder
scratch` does (chapter 11, "`alder scratch`"), with `scratch.RunFile`, from
`internal/lsp/scratch.go`.

- **A goroutine per run.** `didOpen` and `didSave` of a scratch document, and
  every change to the app, start a run on a goroutine of its own and return at
  once, so the reader loop never waits for a build. The document's text is read
  from the overlay when the run starts.
- **Superseded by `seq`.** Each run takes the next number of one counter, shared
  by all scratch documents, and cancels the document's previous run. A run that
  was cancelled may still be on its way to a send, so a send checks, under the
  mutex that guards the runs, that its run is still the document's current one
  (and that the document is still open and the server not stopping); if not, it
  sends nothing. An old result can therefore never overwrite a newer one on the
  editor's screen, and a closed document leaves no run behind.
- **Two notifications.** `RunFile` calls back as soon as the file has checked,
  and the server sends `alder/scratch` with `phase: "types"`: each watch's
  line range, expression and type. When the build and the run end it sends
  `phase: "values"`, with the values. Each is sent after publishing the file's
  diagnostics, under the same mutex, so the editor sees both in order.
- **The same build directory and lock as the command.** The run builds into
  `build/<app>/scratch/file-<hash>/` of the project root, the directory `alder
  scratch` uses for that file, under the same lock. Two runs of one file, one
  from the editor and one from a terminal, wait for each other instead of
  writing into one directory at once.
- **Values are not diagnostics.** A diagnostic is a problem at a place, and
  the editor lists it, counts it and marks the line. A value is neither a
  problem nor a place to fix; as diagnostics, a scratch file of ten watches
  would put ten entries in the problem list every save. They travel in a
  notification of their own, and the plugin draws them as virtual text.

## 11. Exercises

1. **Dump an AST.** Write `module X` and `x : Int` / `x = 1 + 2 * 3` in a
   file, and print its dump with `alder dump` (section 3). Before you run
   it, predict where the parentheses go.
2. **One error for three uses.** Write a module that uses another module
   three times without importing it, and confirm `alder check` prints one
   E0201. `testdata/corpus/E0201-dedupe/` is one answer; explain which of
   the mechanisms in section 4 kept the other two uses quiet.
3. **Predict the Go for a union.** Take `type Switch = On | Off`. Using
   section 7, write down the Go you expect, then run `alder build --emit`
   and compare. Hint: neither constructor has a payload, so look for
   something other than a constructor function (section 9's table).
4. **Find the escaping.** Find the line in `rt/html/html.go` that turns
   `<` in a text node into `&lt;`. Then find the test in
   `rt/html/html_test.go` that proves it happens exactly once.
5. **Why only `Html.unsafe`?** `html.Raw` writes its text unescaped. Find
   every place generated code can reach it (start from `Html.unsafe` in
   `internal/gen/stdcall.go`, then `rt/std/html.go`). Explain why the
   emitter never calls `html.Raw` itself, what would break if literal
   text in a view were emitted as `html.Raw`, and why a `<script>` body is
   `html.RawText` and not `html.Raw`.
6. **Read a wire log.** Start Neovim on a file of `examples/blog` (a copy)
   with `ALDER_LSP_LOG=/tmp/wire.log` set, hover on a name, and quit. Find the
   `textDocument/hover` request in the log and the answer with its `id`.
   Convert the request's `line` and `character` into the line and column
   `alder check` would print for that place (section 10), and say why no
   further conversion is needed when the line contains an `é`.
7. **Break the freshness rule.** Read `TestFresh_TypingBreaksTheLine` in
   `internal/lsp/analysis_test.go`. For each position it asks about, say which
   of the lines above it differ from the build, and what `freshLines` answers
   with `inclusive` true and false. Then say which request would give a wrong
   answer if `freshLines` compared only the cursor's own line.
