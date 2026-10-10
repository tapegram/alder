# Fixture

One block of each kind the verifier knows. `TestBook_FixtureFails` wants exactly one failure:
the wrong `eval=` below.

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

An error example:

```alder error=E0003
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
