# Error corpus: origin of each case

Each directory under `testdata/corpus/` is a minimal app that reproduces one
error observed in the experiments (or required by the spec's diagnostic
table), with the `expected.diag` the checker must produce. Scores and notes
are in `experiments/e1-baseline/scores.md`, `e2-revised`, `e3-pitfalls`, and
`e4-msgpath`. "Task" is the experiment task (T1 modify, T2 new page, T3 blog
or new app, T4 notes + theme).

| Case | Origin (experiment, model, task) |
|---|---|
| `E0101-get-by-slug` | `Db.get Post params.slug`: E1 qwen3-coder T3; E2 qwen3-coder T3 (full and refonly); E3 qwen3-coder T3 (plain and pitfalls); E4 qwen3-coder T3 (sixth run) |
| `E0102-maybe-field` | `user.name` / `user: user` on a `Maybe User`: E1 qwen3-coder T2, qwen3:8b T2; E2 qwen3-coder T2; E3 qwen3-coder T2 (plain and pitfalls) |
| `E0201-missing-import` | `Db.from Todo` without `import Todo`: E1 qwen3:8b T2 |
| `E0202-unqualified-type` | `priority: Priority` for `Todo.Priority`: E1 claude-haiku T1; also unqualified `UserId`, `Comment`, `Theme` in E1 qwen3-coder T3, E2 claude-sonnet T4, E2 qwen3-coder T3/T4 |
| `E0203-qualified-ctor` | `Todo.Normal`: E1 qwen3:8b T1 |
| `E0301-bare-msg` | `click={Toggle}` for a message with `{ id }`: E2 claude-haiku T4 (refonly: `EditNote`/`DeleteNote {}` sent without params), E2 qwen3-coder T4 refonly |
| `E0302-missing-arm` | Spec rule (chapter 6); `update` that does not handle every declared `msg`. No single scored run; written from the book |
| `E0303-href-string` | Spec rule (chapter 5); E1 qwen3-coder T2 `href={App.login}` is the nearest observed error (a non-page value in `href`), and E1 claude-opus T2 / claude-sonnet T2 worked around links with raw strings |
| `E0304-form-field` | Spec rule (chapter 5); nearest observed: E1 qwen3:8b T3 `<Errors field="" />`, E1 qwen3:8b T1 `<Input>` for a union |
| `E0401-write-in-load` | Spec rule (chapters 7 and 8); nearest observed: E1 qwen3:8b T3 `Db.delete Comment params.slug` and `params` used in `update`; written from the book |
| `E0402-bind-outside` | `let <-` outside an effect block: E1 qwen3:8b T3 (`let <-` inside `if` without a block) |
| `E0501-id-on-insert` | `Db.insert Post { id: ... }` for an `@id` field: E2 claude-haiku T3 (full) |
| `E0502-ref-type` | `post: Post.Slug @ref Post`: E3 qwen3-coder T3 (plain and pitfalls); E1 qwen3-coder T3 (`AuthorId` `@ref`) |
| `E0601-msg-in-layout` | `Html Msg` in a non-page module: E2 claude-sonnet T4 (refonly), E2 claude-haiku T3 refonly, E2 claude-haiku T4 (refonly); chapter 5's own example was wrong at the time |
| `E0602-case-of` | `case x of`: E2 qwen3-coder T4 (refonly), the first seen in any run |
| `E0602-let-in` | `let ... in`; the same family as the `let` statements in a function body without a `{ }` block: E2 claude-haiku T4 (full and refonly), E1 qwen3:8b T3 |
| `E0602-tuple` | Tuple in `sortBy`: E1 qwen3:8b T1 |
| `E0003-int-interp` | `"n = {List.length xs}"`: several runs interpolating an `Int` into `Text` without `Text.fromInt` (class TYP across E1 and E2); the compiler fix is the canonical conversion |
| `E0003-plus-text` | `+` on `Text`: E2 qwen3-coder T1 (full) |
| `E0003-date-on-time` | `Date.format` on a `Time`: E1 qwen3-coder T3; E2 qwen3-coder T3 (full) |
| `E0004-missing-case` | Spec rule (chapter 4); nearest observed: E1 qwen3-coder T1 `Todo.filterLabel` on the wrong union. Written from the book |
| `E0007-no-signature` | Spec rule (chapter 3); no observed run omitted a signature, included so the signature fix is covered |
| `E0009-dup-ctor` | Spec rule (chapters 3 and 4): constructors of two imported modules share one namespace. Written from the book |
| `E0010-no-update` | `msg` declared with no `update`: spec rule (chapter 6); nearest observed: E1 claude-haiku T2/T3 and E2 claude-haiku T4 declaring and sending messages the page could not handle |
| `E0002-undeclared-app-msg` | `App.Logout` used without declaring it in `App`: E2 claude-haiku T4 (full) |
| `E0011-list-param` | `List` page parameter: spec rule (chapter 6); written from the book |
| `E0011-id-text` | `id: Text @id` (must be a `brand Text`): E1 qwen3-coder T3; also E3 qwen3-coder T3 (pitfalls) on `Comment` |
| `E0002-other-page-msg` | `click={Auth.Login.Logout}`, sending another page's message: E1 qwen3:8b T2 |
| `E0002-app-login-value` | `href={App.login}` (`login` is a declaration, not a value): E1 qwen3-coder T2 |
| `E0301-form-msg-without-params` | `<form submit={AddComment}>` for a message with `{ slug }` params: E2 claude-haiku T3 (refonly) |
| `E0003-layout-app-msg` | Layout typed `Html App.Msg` (valid: it names the app message union), so a page's `Html Msg` children do not fit: E2 claude-haiku T4 (full) |
| `E0003-slug-vs-nothing` | `Slug "Hello" == Nothing` in a `test` (a brand compared with a `Maybe`): E2 qwen3-coder T3 (full) |
| `E0602-alias-unit` | `type Model = Unit`, an alias in disguise: E2 claude-haiku T4 (refonly) |
| `E0602-alias-keyword` | `type alias Point = { … }` (Elm): spec rule (chapter 4, "No type aliases"); no scored run wrote it; written from the book |
| `E0301-positional-arg` | `Toggle todo.id`, a message applied positionally instead of to a record: observed in M1's final review; shape from E1 qwen3-coder T1 (`Toggle todo.id` style) |
| `E0001-form-attr` | `title: Text.NonEmpty @unique` in a `form`: E5 qwen3-coder T3 rounds 0 and 1 (Problem 4, `experiments/e5-loop/repros/problem-4/`); the reasonless parse error led qwen to change the type instead |
| `E0001-inline-brand` | `id: brand Text` inside a record or table: E5 qwen3-coder T3 rounds 0 and 1 (Problem 4, `experiments/e5-loop/repros/problem-4/`) |
| `E0201-dedupe` | One missing import reported once per use: E5 qwen3-coder T3 (Problem 3, `experiments/e5-loop/repros/problem-3/`: two E0201 for `Theme` in one file) |
| `E0007-inferred` | Spec rule (chapter 3) with the fix the M1 final review asked for: the inferred signature instead of `?type` (M3 spec, "Compiler items (from E5)"); `E0007-no-signature` now gets the inferred fix too |

The golden `testdata/golden/check_poison` covers the rest of E5 Problem 3: an unresolved type cascading into E0011 (``has type `_` ``) and further errors; it must produce the one E0201 only.

Cases marked "written from the book" or "nearest observed" have no one-to-one scored error; they cover a diagnostic the spec requires and the experiments only approached.
