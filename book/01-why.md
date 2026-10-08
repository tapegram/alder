# 1. Why Alder

## The bet

AI agents write more correct code when the language gives them fewer
choices and a compiler that checks the things they get wrong. Alder is a
test of that bet.

Agents writing web apps in general-purpose languages fail in predictable
ways: a link to a route that doesn't exist, a form field whose name doesn't
match the handler, a database query that doesn't match the schema, a missing
case, a side effect in the wrong place, a forgotten auth check. None of these
are hard problems. They are coordination problems between parts of a program
that the language doesn't know are related.

Alder makes those parts language constructs (`page`, `msg`, `form`, `table`,
`capability`) so the compiler can check that they agree.

## Principles

1. **One way to do each thing.** No aliases, no alternative syntaxes, no
   operator overloading, no typeclasses. If two programs mean the same thing
   they should look the same.
2. **Everything is annotated.** Every top-level definition has a type
   signature. Any file can be understood without reading any other file.
3. **Effects are data.** Code that reads the world returns a `Query`. Code
   that changes the world returns a `Task`. Both are descriptions that the
   runtime carries out. Application code never performs an effect.
4. **Invalid states are unrepresentable.** Brands and refinements mean a
   value is validated once, at the boundary, and trusted everywhere else.
5. **Familiar on purpose.** Alder borrows syntax from Elm, Rust, and JSX
   rather than inventing its own, so it is easy to read for anyone (or any
   model) that has seen those.
6. **The compiler is a partner.** Errors are structured, point at the fix,
   and arrive in milliseconds.

## The shape of an Alder app

An Alder app is a set of **pages**. Each page is a module that declares:

- a **route** (`page "/todos/{id}" { id: TodoId }`),
- a **Model** type, derived fresh for every request,
- `load`, which describes what to read to build the Model,
- `view`, which turns the Model into HTML,
- zero or more **messages** (`msg`), which are the POST routes the page's
  HTML can send, and
- `update`, which handles a message and describes what to write and where
  to go next.

This is Elm's Model–View–Update, with one change: the Model is not stored
between requests. It is rebuilt from the route, the session, and the
database each time. There is nothing to invalidate and nothing to get out
of sync.

Pages are served as plain HTML. Interactions are HTTP POSTs. With Datastar
present in the page, the server responds to a POST with a patch of the
changed HTML over SSE, so the page updates in place without a reload.
Without JavaScript the same app works as full page loads.

## What Alder is not

- Not a general-purpose language. There is no file I/O, no threads, no
  FFI except capabilities.
- Not a client-side framework. There is no client state in Alder. If you
  need a rich client, Alder is the wrong tool.
- Not an ORM. The database layer covers CRUD and filtering; anything else is
  a capability implemented in Go.
