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
unions, `Maybe`, `Dict` and `Time` calls `rt.Eq`, which compares
structurally: a `Dict` ignoring insertion order, a `Time` as an instant.
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
`testdata/golden/ir_effects/app/`, whose page module `Index` has a `Todo`
table and a page `"/pair/{a}/{b}"`. (`testdata/golden/gen_effects/app/`
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
`Ensure`, which creates the SQL tables (chapter 8, "Schema changes"). The
list is `Tables`, in a file of its own, `app__alder.go`:

```go
// Tables describes every table of the app, in module order then declaration order, for db.Ensure.
var Tables = []db.TableDesc{table__Todo__Todo.Desc()}

// Capabilities are the keys of eff.Env.Caps the app's capability calls look up: Caps.registry
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
later `Task.fail`, log out, a theme switch whose session change is undone
by a later `Task.fail`. After each one it checks the database and the
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
	session := Todos__Index__Session{User: app.User.Val, Theme: app.Theme}
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
Set-Cookie: alder_session=eyJleHAiOjE3OTQxNjUwMzIsInNlc3Npb24iOnsiVGhlbWUiOiJMaWdodCIsIlVzZXIiOnsiT2siOnRydWUsIlZhbCI6Im9WNUV0NU5aVVBzek9lTG5PU1NCIn19fQ.kfTeo6dGqWQphtqM3E1aJqqK_hG4xPmba5-_b6mEFs8; Path=/; Expires=Sun, 08 Nov 2026 19:10:32 GMT; Max-Age=2592000; HttpOnly; SameSite=Lax

<!doctype html>
<html class=""><head><script type="module" src="/_alder/datastar.js"></script><meta charset="utf-8" /><title>Todos</title>
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
data: elements <body><nav><form method="post" action="/_m/App/SwitchTheme?_p.filter=Active" class="_alder-click" data-on:submit="@post(&#39;/_m/App/SwitchTheme?_p.filter=Active&#39;, {contentType: &#39;form&#39;})"><button type="submit">Dark mode</button></form></nav><main><h1>Todos</h1>…<ul id="todos"></ul><p id="summary">0 open</p>…</main></body>

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
follow a 303 itself and leave the address bar behind. Clicking "Dark mode"
on `/?filter=Active` (an App message whose `Rerender` goes back to the
`Referer`) answers

```
HTTP/1.1 200 OK
Cache-Control: no-cache
Content-Type: text/event-stream
Set-Cookie: alder_session=…; Path=/; Expires=…; Max-Age=2592000; HttpOnly; SameSite=Lax

event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">window.location.assign("/?filter=Active")</script>

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
and size of every `.alder` file under `app/`, every `.go` file at the top
level of `adapters/` and every file under `static/`; `diff` compares two
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
<body><div id="_alder_errors" data-ignore-morph></div><div data-init="@get('/_dev/reload?b=afcb2a7d-1', {openWhenHidden: true})"></div><nav>
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
//     set only for I/O failures, generated or caller Go that does not vet, and go build
//     failures; its message carries Go's output.
//   - Compile writes only under out (the generated module "app", adapters/ and static/ copied
//     from the app, cmd/main.go and the executable out/scratch); it never touches build/<app>
//     itself. Nothing is written when the check fails, and the previous executable in out stays
//     until a new one is built, so a failed Compile leaves the last good one runnable.
//   - Compiles into the same out reuse Go's build cache (a second build of an unchanged app is
//     much faster). Two Compiles must not run into the same out at once.
func Compile(appDir string, extra *Module, mainGo string, purpose, out string) (b Build)
```

Here is `alder eval examples/todos 'Theme.flip Light'`, step by step
(`cmd/alder/eval.go`):

1. **The scratch module.** `evalModule` turns the input into the source
   of module `Scratch`, one line per input line: the expression becomes
   `it = Theme.flip Light`, and each `let name = expr` line before it a
   top-level `name = expr`. None has a signature.
2. **Check.** `scratch.Check` adds `module Scratch` and an `import` of
   every app module but `App`, and checks the program with
   `check.Options{InferTopLevel}` for `Scratch` alone, so its values
   without a signature are inferred instead of being E0007. Diagnostics
   are mapped back through the inserted lines (`MapDiag`) and the
   rewriting of step 1, and come out as `input:1:12`.
3. **Choose a main.** The inferred type of `Scratch.it` picks the Go
   `main`: a `Task` is refused; a `Query` is printed as `<Query>` with its
   type unless `--run` is given (then the main runs it against
   `build/<app>/dev.db`); any other value gets a main that prints
   `rt.Show(app.Scratch__it)`, the value written as Alder (a polymorphic
   `it` is a generic thunk, instantiated at `any`).
4. **Compile.** `scratch.Compile` lowers and emits the whole app plus
   `Scratch` into `build/todos/scratch/eval/`, holding the lock file
   `eval.lock` there, and builds it. The scratch module's Go is one line:

   ```go
   // Code generated by alder; DO NOT EDIT.

   package app

   var Scratch__it Theme__Theme = Theme__flip(Theme__Theme__Light__)
   ```

   and `cmd/main.go` is

   ```go
   func main() { fmt.Print(rt.Show(app.Scratch__it)) }
   ```

   Every compile goes into the same directory, so Go's build cache has
   the app's packages from the last one: an evaluation takes about 0.6 s.
5. **Run.** `scratch.Run` runs the executable (in its own process group,
   killed after `scratch.Timeout`) and `alder eval` prints its output
   with the type: `Dark : Theme`.

A captured session, from a fresh working directory:

```
$ alder eval examples/todos 'Todo.countOpen []'
0 : Int
$ alder eval examples/todos 'Db.from Todo |> Db.count'
<Query> : Query Int
$ printf 'let xs = [3, 1, 2]\nList.sortBy (\\x -> x) xs\n' | alder eval examples/todos -
[1, 2, 3] : List Int
$ alder eval examples/todos 'Theme.flip 1'
input:1:12: error[E0003]: expected Theme, got Int
  Theme.flip 1
             ^
$ alder eval examples/todos --json 'Theme.flip Light'
{"value":"Dark","type":"Theme","diagnostics":[]}
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
book's type with the real one. All of a chapter's modules, declarations
and signature tables go into one copy of `examples/todos/app` and one
`alder check --json`; only when that is not clean is each block checked
alone, to pin every diagnostic on its block and line. Expressions are
parsed with `alder dump`, or, when the fence gives `eval=`, evaluated with
`alder eval --json -` in a working directory shared by all chapters,
where the lock above makes the chapters take turns. Chapters run in
parallel, each in its own temporary directory; the whole book takes
about 15 seconds, almost all of it the evaluations.

### What is still to come

M4a added `alder eval` and the book verifier (above). The rest of M4
adds `alder test` and a REPL (`alder repl`), on the same scratch runner. See the design spec,
[`docs/superpowers/specs/2026-10-09-m3-codegen-runtime-design.md`](../docs/superpowers/specs/2026-10-09-m3-codegen-runtime-design.md),
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
(`Tables`, `Capabilities`), the `Caps` struct the adapters fill, `Options`,
`Serve` (which checks the capabilities, opens the database, creates the
tables and returns `web.NewRouter(Routes, …)`), and `session__codec`, the
cookie's JSON. `routes__alder.go` holds `Routes` and every handler of
section 8's last part. A full `alder build` (without `--emit`) adds
`cmd/main.go`, which reads the environment and calls `Serve`;
`adapters/` and `static/`, copies of the app's own; and the executable.
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
| `title` → `Title` | a record field, with a capital first letter so Go exports it | |
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

So to find a page's view, search for it:

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
		Theme    Theme__Theme
		Children []html.Node
	}{Title: "Todos", Theme: session.Theme, Children: []html.Node{
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
