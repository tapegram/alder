# E5 findings: checker in the loop

*Run 2026-10-08, branch `m2-loop`.* Matrix: `ollama:qwen3-coder:30b`, `claude:haiku`, `claude:sonnet` × T2 (new page in the example app), T3 (blog), T4 (notes with a theme switch). One generation, `alder check --json`, and if it fails, one retry with the diagnostics. Prompt: `prompt.md` (reference plus the to-do example, the E4 shape). Raw data: `runs/<model>/<task>/`, table: `scores.md`.

T2 runs used `--base examples/todos` (see Harness notes): T2 asks for changes to the example app, so the model emits only the files it adds or changes, and round 0 starts from the example app. T3 and T4 start empty.

## 1. Headline

Errors are `alder check` error diagnostics, counted by the harness. E4 errors are hand-scored distinct mistakes, so the columns are close but not identical (see "Counting" below).

| Model | Task | E4 (hand, one shot) | Round 0 | Round 1 | Result |
|---|---|---|---|---|---|
| qwen3-coder:30b | T2 | 2 (E2) | 1 | 1 | fail |
| qwen3-coder:30b | T3 | 3 | 5 | 6 | fail |
| qwen3-coder:30b | T4 | 0 | 0 | — | pass |
| Haiku | T2 | 0 (E2) | 0 | — | pass |
| Haiku | T3 | 0 | 1 | 3 | fail |
| Haiku | T4 | 0 | 0 | — | pass |
| Sonnet | T2 | — | 0 | — | pass |
| Sonnet | T3 | 0 | 0 | — | pass |
| Sonnet | T4 | — | 0 | — | pass |

Per model:

| Model | Round 0 errors (T2+T3+T4) | Final errors | Pass |
|---|---|---|---|
| qwen3-coder:30b | 6 | 7 | 1 / 3 |
| Haiku | 1 | 3 | 2 / 3 |
| Sonnet | 0 | 0 | 3 / 3 |

The one-shot picture matches E4: Sonnet clean; Haiku clean on T4 and one error on T3; qwen clean on T4 and failing T3. Where the checker and E4's hand scores differ:

- **Haiku T3, 1 error the hand-scorer would likely have missed.** A `test` in `Post.alder` binds a row literal with `let` and passes it where a `Post` is expected:
  ```alder
  test "unpublished posts are hidden from visitors" = {
    let post = { id: PostId "a", title: Text.NonEmpty "x", slug: Slug "x", body: Text.NonEmpty "x", published: False, author: User.UserId "u", created: Time.epoch }
    not (canView post Nothing)
  }
  ```
  `app/Post.alder:25:16: error[E0003]: expected Post, got { id: Post.PostId, title: Text.NonEmpty, ... }`. The checker is right by the book (record types are nominal; a literal takes the type its context expects, and an unannotated `let` has no context), but see problem 1 below.
- **qwen T3, 5 errors, all `E0001`, which hide the rest.** A parse error stops the compiler (chapter 11), so round 0 reported only two mistakes (as five diagnostics) and nothing about types. When the round-1 parse errors are fixed by hand, `alder check` reports 12 more errors (see Counting). qwen's real T3 distance from a passing app is larger than either E4's 3 or this run's 6.

## 2. What one retry buys

| Model | Round-0 errors in failing runs | Removed by round 1 | New in round 1 | Survived |
|---|---|---|---|---|
| qwen3-coder:30b | 6 (T2: 1, T3: 5) | 2 (33%) | 3 | E0003 `Maybe User`, E0001 inline `brand` |
| Haiku | 1 (T3) | 1 (100%) | 3 | none, but net 1 → 3 |
| Sonnet | 0 | — | — | — |

**No run was rescued by the retry.** Each failing run is classified below using the brief's categories: (a) fixed what was named but introduced a new error; (b) ignored a diagnostic; (c) the compiler's message or fix was misleading; (d) the compiler was wrong.

**qwen3-coder T2: (b) ignored.** Round 0 and round 1 have the same error at the same code:
```
app/Profile.alder:23:25: error[E0003]: expected User, got Maybe User; match it: match m { Just x -> …, Nothing -> … }
  Query.succeed { user: user, todoStats: { total: total, done: done } }
```
The cause is seven lines up, `let user <- Db.get User session.user`. Round 1 rewrote the file (renamed `SaveName` to `Save`, joined the query onto one line, and dropped the Log out button it had in round 0) but left the `Db.get` and the record unchanged. The message is correct, and its hint points the right way. It names a placeholder `m`, though, not `user`, and it points at the use, not at the `let`; there is no structured `fix`. This is qwen's "`.field` on a `Maybe`" habit from E1–E4, which a message does not change in one retry at temperature 0.

**qwen3-coder T3: (b) ignored one error, (a) fixed another and added a new one.**
- Ignored: `table Comment { id: brand Text @id, ... }` (an inline brand) is unchanged in round 1. The checker reports three diagnostics at one span: ``expected a type, got `brand` ``, ``expected `,` or closing brace``, ``expected end of line after declaration, got `brand` ``. None of them says the brand must be a declared type (`type CommentId = brand Text`). This is partly (c): a parse error with no hint.
- Fixed: `slug: Post.Slug @unique` inside a `form` (two `E0001`s) became `slug: Post.Slug`.
- New: `{if model.post.author == session.user -> <a href={Posts.Admin {}}>Back to admin</a>}` in `Posts/Show.alder` (``expected `then`, got `->` ``, three diagnostics). That is an invented `if … ->` form, written while it also changed the comment form.
- Round 1 also added `User.alder` without being asked. It had been missing in round 0, but no round-0 diagnostic said so, because the parse error stopped the check.

**Haiku T3: (a) fixed the one error by deleting the test, then introduced a parse error.** The round-1 output is a re-roll, not an edit. It renamed `CreatePost` to `Create`, changed `Layout.Page`'s prop from `loggedIn: Bool` to `user: Maybe User.UserId` in seven files, inlined two components, and changed `canView` to a `match`. The new error is a trailing comma after an `if` expression inside an arm block:
```alder
      if Post.canView post session.user then Query.succeed { post: post, comments: comments } else Query.notFound,
    },
```
``app/Posts/Show.alder:25:114: error[E0001]: expected `}`, got `,` `` (three diagnostics). If that comma is removed by hand, the next layer appears: the new `user` prop is passed `session.user` on a `requires user` page:
`app/Admin.alder:51:36: error[E0003]: expected Maybe User.UserId, got User.UserId`. So the round-1 app was two errors from passing. The churn that caused both came from the retry prompt, not the model (see Harness notes).

**No (d): the checker made no false positives in nine runs.** Every reported error, including the ones found under the parse errors, is a real mistake by the book.

### Counting

Check counts are per diagnostic and include cascades, so they run higher than E4's hand counts:
- One parse mistake gives two or three `E0001`s at the same span (all of qwen's and Haiku's round-1 counts).
- A parse error hides everything after it. qwen T3 round 1 with only the reported parse errors fixed by hand (scratch copy, not committed) gives 12 more errors: a missing `import Theme` in `App.alder` (6 diagnostics, see problem 3), `<Input ... type="checkbox" />` (E0304), and the `Comment` table declared inside `Post` (5 diagnostics, see problem 2).

## 3. Compiler problems found by E5

None is a false positive. Each is a message that did not lead the model, or a person, to the fix. Reproductions were run against the `m2-loop` binary built by the harness.

**Problem 1: E0003 on a `let`-bound record literal does not suggest the annotation.** (Haiku T3.) Reproduce by adding this test to `examples/todos/app/Todo.alder`:
```alder
test "let-bound literal" = {
  let t = { id: TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: UserId "u", created: Time.epoch }
  not (matches Active t)
}
```
```
app/Todo.alder:43:23: error[E0003]: expected Todo, got { id: Todo.TodoId, title: Text.NonEmpty, done: Bool, due: Maybe _, owner: User.UserId, created: Time }
```
`let t : Todo = { ... }` passes. **M3:** when the actual type is an anonymous record that matches the expected record's fields, give a `fix` adding the annotation at the `let` (or say "annotate the `let`: `let t : Todo = …`"). **Book now:** chapter 10's examples all pass literals inline. Add one sentence and an example of `let t : Todo = { … }` in a test, since binding a fixture first is the natural form.

**Problem 2: a table declared in another table's module is accepted, and its uses get a wrong fix.** (qwen T3; also qwen's E4 T3 "Comment declared inside Post".) Chapter 8 says "A table is declared in its own module named after it", but the checker accepts this in `examples/todos/app/Todo.alder` with exit 0:
```alder
type NoteId = brand Text

table Note {
  id: NoteId @id,
  todo: TodoId @ref Todo,
  body: Text,
}
```
The error appears only at a use in `Todos/Index.alder`, `let notes <- Db.from Note |> Db.where (Note.todo.eq p.id) |> Db.all`:
```
app/Todos/Index.alder:44:26: error[E0002]: unknown constructor `Note`
app/Todos/Index.alder:44:44: error[E0201]: module `Note` is not imported
  fix: import Note
```
There is no module `Note`, so the fix is wrong. Qualifying it does not help either: `Db.from Todo.Note |> Db.where (Todo.Note.todo.eq p.id)` gives ``module `Todo.Note` is not imported; fix: import Todo.Note``. Such a table cannot be used from another module at all. **M3:** enforce the rule at the declaration: an error on `table Note` in module `Todo` saying "move it to `app/Note.alder`" (E0005 or E0011). Until then, at a use site, do not offer `import X` when no module `X` exists.

**Problem 3: one missing import produces six errors, two of them misleading.** (qwen T3.) Delete `import Theme` from `examples/todos/app/App.alder`:
```
app/App.alder:6:36: error[E0011]: session field `theme` has type `_`, which cannot be stored in a session
app/App.alder:6:43: error[E0201]: module `Theme` is not imported   fix: import Theme
app/App.alder:6:51: error[E0002]: unknown constructor `Light`
app/App.alder:20:46: error[E0201]: module `Theme` is not imported   fix: import Theme
app/App.alder:27:37: error[E0002]: unknown constructor `Light`
app/App.alder:34:41: error[E0002]: unknown constructor `Light`
```
Only `E0201` is the cause. The `E0011` message (``type `_` ``) is noise from an unresolved type, and ``unknown constructor `Light` `` does not say that `Light` belongs to `Theme`. Chapter 11 says a module whose names could not be resolved is not type-checked further; that is not what happens here. **M3:** suppress `E0011`/`E0003` when a type is `_` from a failed resolution, and when an unknown constructor is exported by a module that exists, say "`Light` is a constructor of `Theme.Theme`; import `Theme`" with the fix.

**Problem 4 (design, not a bug): a parse error hides the rest of the program.** It is documented in chapter 11, and this run measured what it costs in a loop: qwen's T3 shows 5–6 errors per round while 12 more wait behind them, so each retry can reveal only one layer. **M3:** parse every file and report all parse errors, then type-check the modules that parsed and whose imports parsed, the same way name-resolution failures are already gated.

**Smaller message gaps:** the inline-`brand` parse error (three diagnostics, no hint that a brand is a declared type), and the `Maybe` hint's placeholder `m` (it could name the binding and point at the `let` that bound it).

## 4. Harness notes

- **Timing per round** (`summary.json`): qwen3-coder 22–43 s (mean 35 s, 1.2k–2.3k tokens); Haiku 32–77 s (mean 57 s); Sonnet 18–43 s (mean 28 s). The full matrix took 9 min 8 s and the seeded T2 re-run 1 min 52 s, faster than the 15–30 minutes expected because six of nine runs passed in round 0.
- **Harness bug fixed: T2 had no base app.** In the first matrix run, every T2 round 0 was checked with only the model's 1–3 new files, so all errors were ``unknown module `User` ``-style noise: qwen 50, Haiku 42, Sonnet 39 (`E0002`, `E0011`, `E0201`). Fix: `run_loop.py --base DIR` copies `DIR/app` into round 0 before the model's files are written, through the existing overlay. Test: `test_base_seeds_round0`. T2 was re-run with `--base examples/todos`, which is identical to the example app in `prompt.md` and checks clean. The unseeded runs are not committed. One observation from them is worth keeping: in round 1 Haiku and Sonnet reproduced the whole example app from the prompt from nothing but "unknown module" diagnostics and passed (42 → 0, 39 → 0). qwen re-emitted the same three files and stayed at 50.
- **The retry prompt does not include the previous answer.** `retry_prompt` sends the task plus the diagnostics, each with its one source line; the model never sees its own files. Round 1 is therefore a fresh generation with hints, not an edit. It explains Haiku's T3 churn (seven files changed to fix one test) and qwen's near-identical re-rolls at temperature 0. This is the biggest limit on what "one retry" measured here.
- **Splitter:** Haiku wrapped each file in a code fence in T3 and T4 (18 and 16 fence lines), and the splitter removed them. No preambles, no rejected paths, no `exit 2`, and no model errors, so no re-runs were needed.
- Passes are real apps. Every T3/T4 pass has 8–9 files and 254–327 lines. Sonnet T3 and qwen T4 were read in full: all routes, forms, `msg`s and the theme switch are present. Two semantic gaps that the checker cannot see, recorded as design signals: Sonnet's T3 shows unpublished posts to any logged-in user (`Post` has no author); qwen's T4 puts `class="dark"` on `<html>`, not `<body>`, and labels the buttons "Dark mode"/"Light mode". T2 passes add `Profile.alder` (66–72 lines), link it from `Todos/Index`, and log out with `<button click={App.Logout}>` with a comment, as the task asked.

## 5. What this changes in the plan

1. **The retry must include the previous files** (or a diff-style "edit these files" contract). Without them, the model re-rolls instead of fixing, and a retry can make things worse (Haiku T3, 1 → 3). Do this before measuring retries again; E5's "what one retry buys" mostly measures re-roll variance.
2. **Report every parse error and check past them** (problem 4), so one round shows more of the distance. In qwen's T3 round 1, 12 errors sat behind 6 parse diagnostics; 6 of them were in `App.alder`, which parsed and was never checked.
3. **Fix the three messages** (problems 1–3). Problem 2 is qwen's oldest habit (E4's "`Comment` inside `Post`"), and today the checker accepts the declaration and then suggests a fix that cannot work.
4. **Count distinct mistakes, not diagnostics,** when comparing with hand-scored experiments, or dedupe same-span diagnostics in `report_loop.py`.
5. Haiku and Sonnet are at the ceiling on T2–T4 in one shot; qwen3-coder remains the model to iterate on. The next run should give it more than one retry once (1) and (2) are in place.
