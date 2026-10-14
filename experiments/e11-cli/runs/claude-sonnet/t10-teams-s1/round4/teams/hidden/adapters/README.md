# adapters

The Go that implements the app's capabilities goes here, one package per capability's module.
A capability `X` declared in module `A.B` is implemented by the package in `adapters/a/` (the
module's first segment, lower-cased), which exports `func BX() app.Cap__A__B__X`; a module of
one segment drops the `B`, so `Markdown`'s `Markdown` is `func Markdown()` in
`adapters/markdown/`. This app declares no capability yet, so nothing goes here; when one is
missing, `alder build` prints the exact directory and function to write (the Alder book,
chapter 7, "Implementing a capability").
