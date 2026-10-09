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

Here is what the lexer produces for a file holding just two lines,
`session { user: Maybe User.UserId }` and `update msg session = 1`, which
are shaped like lines in `examples/todos/app/App.alder`. The listing was
printed by the `TestLexFile` test shown in section 3:

```
1:1 Keyword "session"
1:9 LBrace "{"
1:11 LIdent "user"
1:15 Colon ":"
1:17 UIdent "Maybe"
1:23 UIdent "User"
1:27 Dot "."
1:28 UIdent "UserId"
1:35 RBrace "}"
1:36 NL "\n"
2:1 LIdent "update"
2:8 Keyword "msg"
2:12 Keyword "session"
2:20 Equal "="
2:22 Int "1"
2:23 NL "\n"
```

Notice that `session` and `msg` are `Keyword` tokens on both lines. On the
first line `session` starts a declaration. On the second, `msg` and
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

`syntax.Dump` prints an AST as nested parentheses. `alder` has no command
for this yet (an `alder dump` command is a planned convenience), so you
print it from a small Go test. Go only lets code inside the repository
call `internal/syntax`, but `go test -overlay` can add a test file to a
package without changing the repository: the overlay file maps a path
inside the package to a file kept somewhere else.

Make a directory outside the repository, say `~/alder-dump`, and save this
as `~/alder-dump/dump_test.go`:

```go
package syntax

import (
	"fmt"
	"os"
	"testing"
)

func TestDumpFile(t *testing.T) {
	src, err := os.ReadFile(os.Getenv("ALDER_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	m, diags := ParseFile("M.alder", src)
	for _, d := range diags {
		fmt.Println(d.Code, d.Message)
	}
	for _, d := range m.Decls {
		fmt.Println(Dump(d))
	}
}
```

It reads the file named by the `ALDER_FILE` environment variable, parses
it, prints any parse errors, and prints each declaration's dump. If you
have never read Go: `package syntax` says which package the file belongs
to, and `import` names the packages it uses. `func TestDumpFile(t
*testing.T)` declares a function; `go test` runs every function whose name
starts with `Test`. `:=` declares a variable and gives it its first value,
`if err != nil` checks whether reading the file failed, and
`for _, d := range m.Decls` loops over each declaration. From the
repository root, write the overlay file and run the test:

```
$ printf '{"Replace":{"%s/internal/syntax/dump_file_test.go":"%s/alder-dump/dump_test.go"}}\n' "$PWD" "$HOME" > ~/alder-dump/overlay.json
$ ALDER_FILE=$PWD/testdata/golden/gen_basic/app/M.alder go test -overlay ~/alder-dump/overlay.json ./internal/syntax -run TestDumpFile -v
```

The file name must be an absolute path, because Go runs a test inside the
package's own directory. Two variations of the same recipe print the other
dumps in this chapter. Each needs one more entry in `overlay.json`'s
`Replace` map. For tokens, add a test `TestLexFile` to `internal/syntax`
(as `lex_file_test.go`, same imports) whose body is:

```go
	src, err := os.ReadFile(os.Getenv("ALDER_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLexer("M.alder", src)
	for tk := l.Next(); tk.Kind != EOF; tk = l.Next() {
		fmt.Printf("%d:%d %s %q\n", tk.Line, tk.Col, tk.Kind, tk.Text)
	}
```

For the IR (section 6), add a test `TestDumpDir` to `internal/ir` (as
`dump_dir_test.go`, with `package ir` and the same imports) whose body is
`fmt.Print(Dump(lowerDir(t, os.Getenv("ALDER_DIR"))))`, and set
`ALDER_DIR` to an app directory such as `$PWD/testdata/golden/gen_basic`.
`lowerDir`, a helper in `internal/ir/ir_test.go`, parses, resolves, checks
and lowers the app.

This is the dump of `area`, from `TestDumpFile` on `gen_basic` (wrapped
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
neither can be understood first, and you get E0006. With no cycles, the
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
`greet id = "hello {id}"`. Its IR dump (from `TestDumpDir`) shows the signature with the brand
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

Every snippet below is from `testdata/golden/gen_basic/want/M.go`, which is
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
(from `build/partial/P.go`, after `alder build --emit partial`, where
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
`build/facts/F.go`:

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
packages the generated code imports. There are three.

- **`rt`** (`rt/core.go`) holds the types the mapping table names: `Maybe`,
  `Result`, `Dict`, `Date`, `Page`. It also holds the helpers the emitter
  calls: `Concat`, `OrElse`, `Append`, `Eq`, `DateCmp`, `TimeCmp`, and
  `Int` and `Float`, which return their argument unchanged: a literal goes
  through one when Go would otherwise compute an operation on constants
  while compiling, and reject `1 / 0` or an `Int` that overflows, which
  Alder defines to happen when the program runs (chapter 4). A `Dict` takes
  keys of any type: it files each under `Key(k)`, a text that is the same
  for two keys exactly when they are `==`.
- **`rt/html`** (`rt/html/html.go`) builds HTML trees (`El`, `Text`,
  `Raw`, `A`, `Nodes`) and renders them. `Render` is where escaping
  happens: a text node goes through `html.EscapeString`, and so does every
  attribute value. A `Raw` node is written as is.
- **`rt/std`** (`rt/std/*.go`) has one Go function per pure stdlib
  function: `std.ListMap` is `List.map`, `std.TextToUpper` is
  `Text.toUpper`.

The generated program is tiny because the runtime is the library. All of
`examples/todos` emits about 450 lines of Go across eight files, and those
lines mostly call into `rt`.

This is where M3a ends. What follows is planned, not built, and described
in the design spec,
[`docs/superpowers/specs/2026-10-09-m3-codegen-runtime-design.md`](../docs/superpowers/specs/2026-10-09-m3-codegen-runtime-design.md):

- **M3b (future):** an interpreter for `Query` and `Task` effects, and a
  SQLite database store. Until then, values whose types involve effects or
  the database (`load`, `update`) are not lowered; `ir.Module.Skipped`
  lists them.
- **M3c (future):** the router, sessions, form decoding, and Datastar
  patching, plus `main.go`, so `alder build` makes a server you can run.
- **M3d (future):** `alder run`, the dev loop that rebuilds and reloads on
  every save.

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
replaced by `__` (`Todos__Index.go` for `Todos.Index`), and a `go.mod` that
points back at this repository for `rt`. Every Go file starts with
`// Code generated by alder; DO NOT EDIT.`, and the build runs `go vet` on
the package before saying `wrote`.

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
| `title` → `Title` | a record field, with a capital first letter so Go exports it | |
| `type` → `type__` | a local whose name Go reserves, or that names `rt`, `html`, `std` | |

Why `__`? Your names may not contain it: the compiler rejects `max__len`
(E0001), and a module name segment may not end with `_`. So `__` can only
be a joint, and a generated name can be read back into its parts:
`Todos__Index__view` is module `Todos.Index` and name `view`, never module
`Todos` and a name `Index__view`. Modules `A.B` and `A_B` become `A__B` and
`A_B`, so they cannot overwrite each other's file. Names the compiler adds
start with a lowercase word (`tag__`, `is__`, `payload__`, `widen__`),
which no module name does, so they never clash with yours either. The one
place your names and generated ones share a module is a page module (and
`App`): the compiler generates `Params`, `Msg`, `Session` and `PickParams`
for `msg Pick`, so a type or component of yours may not use those names
(E0012). A generated name can never be a Go keyword or clash with `rt`,
`html` or `std`.

So to find a page's view, search for it:

```
$ grep -n "func Todos__Index__view" build/todos/*.go
build/todos/Todos__Index.go:73:func Todos__Index__view(model Todos__Index__Model, session Todos__Index__Session) html.Node {
```

Its first lines show the `<Layout.Page>` component call and the `<form>`
with its message URL. The props are an anonymous struct type followed
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
		html.Form(rt.MsgURL("Todos.Index", "Submit"), nil,
			html.Input(html.Control{Kind: "input", Name: "title", Type: "text", Label: "What needs doing?", Required: true}),
```

Compare it with the `view` in chapter 2. `session` has type
`Todos__Index__Session`, not the app's session: the page `requires user`, so
it gets its own session record in which `user` is a `UserId`, not a
`Maybe UserId`. Where the page passes `session` to code that expects the
app's session, the call goes through `widen__Todos__Index__Session`, which
the emitter writes into `Todos__Index.go`: it copies the fields and wraps
`user` back in `rt.Just`.

## 10. Exercises

1. **Dump an AST.** Write `module X` and `x : Int` / `x = 1 + 2 * 3` in a
   file, and print its dump with the `TestDumpFile` recipe from section 3
   (set `ALDER_FILE` to your file's absolute path). Before you run it,
   predict where the parentheses go.
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
