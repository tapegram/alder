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
  events Alder answers with: `PatchBody`, which replaces the `<body>`, and
  `Navigate`, which sends the browser to another URL.

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
trace of the steps that ran, `calls("toggle", "Db.get Todo", "Db.update
Todo")` (the to-do app's `Toggle` first reads the row to check its owner,
chapter 6): exactly the kind of check chapter 10 promises `alder test`
will make.

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
		return web.PageResponse(Todos__Index__view(model, session), &html.FormState{PageParams: params__Todos__Index(params)}, rq.Session, false)
	}}
}
```

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
`All`. Back in the router, `write` signs the session and sets the cookie,
then calls `html.RenderWith(node, state)`. Every message attribute in the
tree is an `html.MsgAttr`, a value that is only turned into text now: its
`rt.MsgRef` (module, message, the message's own parameters) is given the
page's parameters with `_p.` in front (`MsgRef.URL` in `rt/core.go`). A
`<button>` with a `click` is rendered as a click button here too: the
renderer knows whether it is inside a `<form>`, and wraps the button in a
form of its own when it is not (chapter 6, "With and without JavaScript").
Last, `document` writes `<!doctype html>` and the page, with the Datastar
script tag put after `<head>`. The answer begins like this (without the
`Date` and `Content-Length` headers, and with the page cut after
`</title>`):

```
HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Set-Cookie: alder_session=eyJleHAiOjE3OTQxNjUwMzIsInNlc3Npb24iOnsiVXNlciI6eyJPayI6dHJ1ZSwiVmFsIjoib1Y1RXQ1TlpVUHN6T2VMbk9TU0IifX19.K74dmYmShNDDHvaHP5wlwizxAZgG8yLfVwLnYee1aCw; Path=/; Expires=Sun, 08 Nov 2026 19:10:32 GMT; Max-Age=2592000; HttpOnly; SameSite=Lax

<!doctype html>
<html><head><script type="module" src="/_alder/datastar.js"></script><meta charset="utf-8" /><title>Todos</title>
```

and has the Done button with its message URL:

```html
<form method="post" action="/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&amp;_p.filter=Active" class="_alder-click" data-on:submit="@post(&#39;/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&amp;_p.filter=Active&#39;, {contentType: &#39;form&#39;})"><button type="submit">Done</button></form>
```

The `&#39;` and `&amp;` are the escaping of section 8's first part: the
attribute holds
`@post('/_m/Todos.Index/Toggle?id=…&_p.filter=Active', {contentType: 'form'})`.

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
   `Toggle`" did, with one step more in front: `owned` reads the row
   (`Db.get Todo`, which begins the transaction) and checks that its
   `owner` is the session's user, then one `UPDATE`, a commit, and `Nav`
   `Rerender`. If the task failed, `route__fail` answers 500, or 404 for
   `Task.notFound` (an id that is missing or someone else's), and nothing
   is stored, since `Run` already rolled back the writes and the session.
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
It sets the cookie, renders the page, cuts out its `<body>…</body>`
(`bodyOf`), and calls `datastar.PatchBody`, which writes one server-sent
event: the event name, two lines that say what to replace and how, and one
`data: elements` line for each line of the body. The answer is

```
HTTP/1.1 200 OK
Cache-Control: no-cache
Content-Type: text/event-stream
Set-Cookie: alder_session=…; Path=/; Expires=…; Max-Age=2592000; HttpOnly; SameSite=Lax

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
names no row, or another user's, `owned` ends in `Task.notFound`, and under
Datastar the 404 page's body arrives the same way as a re-render, as a 200
event.

A `Redirect` under Datastar is not a 303 either: `write` sends a 200 event
from `datastar.Navigate` that appends a self-removing script calling
`window.location.assign` with the URL, because the client's `fetch` would
follow a 303 itself and leave the address bar behind. Clicking "Log out"
on `/?filter=Active` (an App message whose `update` answers
`Redirect (Auth.Login {})`) answers

```
HTTP/1.1 200 OK
Cache-Control: no-cache
Content-Type: text/event-stream
Set-Cookie: alder_session=…; Path=/; Expires=…; Max-Age=2592000; HttpOnly; SameSite=Lax

event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">window.location.assign("/login")</script>

```

`TestE2E_Todos` in `internal/gen/e2e_test.go` runs this whole scenario
against the generated app on every `go test ./...`. Its program,
`internal/gen/testdata/e2e_todos_test.go.txt`, checks that a Datastar
answer starts with exactly
`event: datastar-patch-elements\ndata: selector body\ndata: mode outer\ndata: elements <body>`
and ends with `</body>\n\n`, that a `Redirect` under Datastar is exactly
the navigation event above, that a `Toggle` sent without JavaScript from
`/?filter=Completed` is a 303 back to that filter, and that another user
posting the messages with a@b.c's id gets 404 every time.

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
Datastar script it loads, the dev server serves `/_alder/datastar.js`
itself in that state.

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
of module M into `test__M__N`. `Todo`'s one test,
`test "countOpen ignores done todos" = countOpen [ … ] == 1` (two todos,
one done), is emitted into `build/todos/scratch/test/Todo_alder.go` as

```go
// test__Todo__1 is test "countOpen ignores done todos" of module Todo.
func test__Todo__1() bool {
	return rtesting.Eq(Todo__countOpen([]Todo__Todo{Todo__Todo{Id: "a", Title: "x", Done: false, Due: rt.Nothing[rt.Date](), Owner: "u", Created: std.TimeEpoch()}, Todo__Todo{Id: "b", Title: "y", Done: true, Due: rt.Nothing[rt.Date](), Owner: "u", Created: std.TimeEpoch()}}), int64(1))
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
	{Module: "Todo", Name: "countOpen ignores done todos", Pure: test__Todo__1},
	{Module: "Todos.Index", Name: "Toggle flips done", Task: test__Todos__Index__1},
	{Module: "Todos.Index", Name: "Toggle by another user is not found", Task: test__Todos__Index__2},
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

`app.Migrations` are the app's migrations ("Migrations", below), and
`app.FakeTables` the tables of its fakes ("Fakes", below; the to-do app
has none, so its test build has `var FakeTables = []db.TableDesc{}`).
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
	return eff.Bind(eff.Call("Test.run", true, func(_e3 *eff.Env) (struct {
		Value rt.Result[string, rt.Nav]
		Trace []string
	}, error) {
		_v4, _msg5, _ok6, _tr7, _err8 := rtesting.Inner(_e3, Todos__Index__update(Todos__Index__Msg__Toggle__(Todos__Index__ToggleParams{Id: id}), Todos__Index__Model{Filter: Todo__Filter__All__, Todos: []Todo__Todo{}}, Todos__Index__Session{User: me}))
		if _err8 != nil {
			var _z10 struct {
				Value rt.Result[string, rt.Nav]
				Trace []string
			}
			return _z10, _err8
		}
		_r9 := struct {
			Value rt.Result[string, rt.Nav]
			Trace []string
		}{Trace: _tr7}
		if _ok6 {
			_r9.Value = rt.Ok[string, rt.Nav](_v4)
		} else {
			_r9.Value = rt.Err[string, rt.Nav](_msg5)
		}
		return _r9, nil
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

### What is still to come

M4a added `alder eval` and the book verifier, M4b `alder test`, M4c
the REPL, M5b migrations and M5c fakes (above). See the design spec,
[`docs/superpowers/specs/2026-10-09-m4-test-repl-design.md`](../docs/superpowers/specs/2026-10-09-m4-test-repl-design.md),
"Milestones".

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
`Todos.Index`), plus two app-wide files and a `go.mod` that points back at
this repository for `rt`. `app__alder.go` holds the lists of section 8
(`Tables`, `Capabilities`), the `Caps` struct the generated `caps` package fills, `Options`,
`Serve` (which checks the capabilities, opens the database, creates the
tables and returns `web.NewRouter(Routes, …)`), and `session__codec`, the
cookie's JSON. `routes__alder.go` holds `Routes` and every handler of
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
Every Go file starts with `// Code generated by alder; DO NOT EDIT.`, and
the build runs `go vet` on the package before saying `wrote`. Before
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

## 10. Exercises

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
   emitter never calls `html.Raw` itself, and what would break if literal
   text in a view were emitted as `html.Raw`.
