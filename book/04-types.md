# 4. Types

## Built-in types

| Type | Values |
|---|---|
| `Int` | 64-bit integers |
| `Float` | 64-bit floats |
| `Bool` | `True`, `False` |
| `Text` | UTF-8 strings |
| `Unit` | `()` |
| `List a` | `[1, 2, 3]` |
| `Maybe a` | `Just x`, `Nothing` |
| `Result e a` | `Ok x`, `Err e` |
| `Dict k v` | built with `Dict.fromList`; `k` may be any type that can be compared with `==` (no functions or effect values in it, including inside a union's payloads), and two keys are the same key when they are `==` (so `Html` keys are compared by their markup) |
| `Date`, `Time` | calendar date; instant (chapter 9) |

There is no `Char` and no `String`: text is `Text`. There is no tuple: use
a record.

## Int

`Int` is a 64-bit two's-complement integer. Arithmetic that goes past the
largest or smallest `Int` wraps around: `9223372036854775807 + 1` is
`-9223372036854775808`. `/` truncates toward zero (`-7 / 2` is `-3`), and
`%` has the sign of the dividend (`-7 % 2` is `-1`). Dividing an `Int` by
zero, with `/` or `%`, is a runtime failure: the program stops where it
happens (later milestones turn it into a failed task). This holds for
constants too; `1 / 0` compiles and fails when it runs.

`Float` is a 64-bit IEEE float: `1.0 / 0.0` is infinity, not a failure.

## Type signatures

Every top-level value and function has a signature on the line before it.
Function types use `->`. Type parameters are lowercase names.

```alder
first : List a -> Maybe a
first xs = List.head xs
```

A signature may use any type parameter; there are no constraints on type
parameters (no typeclasses). Functions that need equality or ordering take
only concrete types, or take a comparison function.

## Records

A record type lists fields and their types:

```alder
type Point = { x: Float, y: Float }
```

Record types are **nominal**: a `Point` is not interchangeable with another
record type that happens to have the same fields. An anonymous record
literal `{ x: 1.0, y: 2.0 }` is given the type the context expects. A
signature may spell out a record type inline when it is used only once:

```alder
area : { w: Float, h: Float } -> Float
area r = r.w * r.h
```

## Unions

```alder
type Filter = All | Active | Completed

type Shape
  = Circle Float
  | Rect { w: Float, h: Float }

type Tree a
  = Leaf
  | Node { left: Tree a, value: a, right: Tree a }
```

A constructor takes zero or one argument. For several values, pass a record.
Constructors are functions: `Circle : Float -> Shape`.

`Maybe` and `Result` are ordinary unions defined in the standard library.

A union whose constructors all take no argument is **ordered by declaration**:
for `type Priority = Low | Normal | High`, `Low < Normal < High`. The
comparison operators work on such unions, and so does `Db.orderBy`.

## Brands

A brand is a type that has the same runtime representation as its base type
but is distinct to the type checker:

```alder
type TodoId = brand Text
type Cents = brand Int
```

Rules:

1. **Widening is implicit.** A `TodoId` can be used anywhere a `Text` is
   expected, with no conversion.
2. **Narrowing is explicit and local.** The constructor `TodoId : Text ->
   TodoId` exists only inside the module that declares the brand. Other
   modules receive brand values from boundaries (route parameters, forms,
   database rows) or from functions the declaring module exports.
3. **Operations return the base type.** `Text.trim id` is a `Text`, not a
   `TodoId`. Exception: `+`, `-`, `*`, `/`, `%` on two values of the same
   numeric brand return that brand (`Cents + Cents : Cents`).
4. **Equality requires identical types.** `id == text` is an error even
   though `id` widens to `Text`.
5. **A brand's base is a built-in type**, never another brand.

Rules 1 and 3 together decide arithmetic on two *different* brands of one
base: both widen to the base, and the result is the base. An
`Int.NonNegative` times an `Int.Positive` is an `Int`, and so is `Cents +
Int.Positive`; only `Cents + Cents` stays `Cents`.

```alder eval=6
-- 6 : Int, neither brand
(Int.NonNegative 2) * (Int.Positive 3)
```

Rule 4 is a type error, not a silent widening:

```alder error=E0003 "`==` needs both sides to have the same type"
type Cents = brand Int

sameAmount : Cents -> Int -> Bool
sameAmount c n = c == n
```

## Refinements

A brand may carry a check. The check is a `Bool` expression over the base
value, written as a function applied at boundaries:

```alder
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"
type Age = brand Int where (\n -> n >= 0 && n < 150)
```

When a value of this type is decoded from a route, a form, or a database
row, the check runs. If it fails, the form shows an error or the route is a
404. Inside the program the value is trusted and never re-checked.

The standard library defines common refinements:

```alder
Text.NonEmpty     -- length > 0 after trimming
Text.Email        -- contains an @ with text on both sides
Text.Secret       -- non-empty; rendered as a password input, never logged or echoed
Int.Positive      -- > 0
Int.NonNegative   -- >= 0
```

These are ordinary brands of `Text` and `Int` and widen the same way.

## Type parameters on records and unions

```alder
type Page a = { items: List a, total: Int }
```

## What there is not

- No type aliases. A new name is always a new type.
- No typeclasses, interfaces, or traits.
- No tuples.
- No record that contains itself by value (`type R = { r: R }`, directly
  or through other records, is E0011): a record may refer to itself only
  through `Maybe`, `List`, `Dict` or another union.
- No subtyping other than brand-to-base, and a `requires` page's `Session`
  to the app's (chapter 6).
