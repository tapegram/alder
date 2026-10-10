# Fixture

One block of each kind the verifier knows. `TestBook_FixtureFails` wants exactly four failures:
the wrong `eval=`, signature, declaration and `error=` message below.

A whole module, replacing the example's `Theme`:

```alder
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

isDark : Theme -> Bool
isDark theme = theme == Dark
```

Declarations, which see every example module:

```alder
-- a comment first
double : Int -> Int
double n = n * 2

open : List Todo.Todo -> Int
open todos = Todo.countOpen todos
```

An expression, parse-only (`todos` exists only in prose):

```alder
todos
  |> List.filter (\t -> not t.done)
  |> List.length
```

An expression with its value:

```alder eval=[2, 3, 4]
List.map (\x -> x + 1) [1, 2, 3]
```

A wrong value (the one failure):

```alder eval=4
1 + 2
```

An error example, with a part of its message:

```alder error=E0003 "expected Int, got Text"
x : Int
x = "a"
```

A reference table of signatures, each checked against the name's real type:

```alder
List.length : List a -> Int
Text.fromInt : Int -> Text   -- a comment
Float.min, Float.max         -- names alone are not checked
```

An excerpt:

```alder fragment
update msg model session = match msg {
  Toggle p -> ...,
}
```

Other fences are not Alder:

```go
func main() {}
```

A module walked through in pieces, checked whole:

```alder part=Extra
module Extra

import Theme
```

Prose between the pieces.

```alder part=Extra
dark : Theme
dark = Theme.flip Light
```

A wrong signature (the second failure):

```alder
Text.fromInt : Int -> Int
```

A wrong declaration (the third failure):

```alder
double : Int -> Int
double n = n ++ 2
```

A wrong error message (the fourth failure):

```alder error=E0003 "expected Bool"
y : Int
y = "b"
```
