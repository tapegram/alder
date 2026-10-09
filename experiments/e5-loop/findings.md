# E5 findings: checker in the loop

*Run 2026-10-08, branch `m2-loop`.* Matrix: `ollama:qwen3-coder:30b`, `claude:haiku`, `claude:sonnet` × T2 (new page in the example app), T3 (blog), T4 (notes with a theme switch). One generation, `alder check --json`, and if it fails, one retry that carries the previous answer and the diagnostics. Prompt: `prompt.md` (reference plus the to-do example, the E4 shape). Raw data: `runs/<model>/<task>/`, table: `scores.md`.

T2 ran with `--base examples/todos`: T2 asks for changes to the example app, so the model emits only the files it adds or changes, and round 0 starts from the example app. T3 and T4 ask for a new app ("not the todo app") and start empty. Seeding them would mix the todo modules into the app being checked.

## Harness problem found by the first run

The first run of this matrix (data in commit 49e7792 only) had a blind retry. `retry_prompt` sent the task and the diagnostics, each with its one source line, but not the model's previous files. Every model call is a fresh process, so round 1 never saw the code it was asked to fix and regenerated it from scratch. Haiku's T3 went 1 → 3 because of this: to fix one test it changed seven files, including `Layout.Page`'s props, and introduced a parse error. qwen's T2 repeated its error unchanged. The retry prompt now includes every `.alder` file of the previous round, in full and sorted by path, under `YOUR PREVIOUS ANSWER`, before the compiler feedback (test: `test_retry_prompt_includes_previous_files`). The whole matrix was re-run from clean; every number below is from that re-run. The first run also found that T2 had no base app (fixed with `--base`, test `test_base_seeds_round0`).

## 1. Headline

Errors are `alder check` error diagnostics, counted by the harness. E4 errors are hand-scored distinct mistakes, so the columns are close but not identical (see "Counting").

| Model | Task | E4 (hand, one shot) | Round 0 | Round 1 | Result |
|---|---|---|---|---|---|
| qwen3-coder:30b | T2 | 2 (E2) | 1 | 0 | pass |
| qwen3-coder:30b | T3 | 3 | 5 | 5 | fail |
| qwen3-coder:30b | T4 | 0 | 0 | — | pass |
| Haiku | T2 | 0 (E2) | 0 | — | pass |
| Haiku | T3 | 0 | 0 | — | pass |
| Haiku | T4 | 0 | 0 | — | pass |
| Sonnet | T2 | — | 0 | — | pass |
| Sonnet | T3 | 0 | 0 | — | pass |
| Sonnet | T4 | — | 0 | — | pass |

Per model:

| Model | Round 0 errors (T2+T3+T4) | Final errors | Pass |
|---|---|---|---|
| qwen3-coder:30b | 6 | 5 | 2 / 3 |
| Haiku | 0 | 0 | 3 / 3 |
| Sonnet | 0 | 0 | 3 / 3 |

The one-shot picture matches E4. Sonnet and Haiku are clean on every task; qwen is clean on T4 and fails T3 with its known habits. Things to know when comparing:

- **qwen runs at temperature 0 and is deterministic.** Its round 0s are byte-identical to the first run's (same token counts), so its round-0 numbers are a property of the prompt. The Claude CLI exposes no temperature: Haiku's T3 had 1 error in the first run (a `let`-bound row literal in a test, problem 1 below) and 0 here. Treat a single Claude run as one sample.
- **qwen's T3, 5 errors, all `E0001`, which hide the rest.** A parse error stops the compiler (chapter 11), so both rounds report only two parse mistakes (as five diagnostics) and nothing about types. If the round-1 parse errors are fixed by hand, `alder check` reports 15 more errors (see Counting). qwen's real T3 distance from a passing app is larger than either E4's 3 or this run's 5.

## 2. What one retry buys

| Model | Round-0 errors in failing runs | Removed by round 1 | New in round 1 (reported) | Survived |
|---|---|---|---|---|
| qwen3-coder:30b | 6 (T2: 1, T3: 5) | 1 (17%) | 0 | E0001 inline `brand` (3), E0001 `@unique` on a form field (2) |
| Haiku | 0 | — | — | — |
| Sonnet | 0 | — | — | — |

Only qwen needed a retry. Each failing or retried run is classified below using the brief's categories: (a) fixed what was named but introduced a new error; (b) ignored a diagnostic; (c) the compiler's message or fix was misleading; (d) the compiler was wrong.

**qwen3-coder T2: fixed, pass (1 → 0).** Round 0:
```
app/Profile.alder:23:25: error[E0003]: expected User, got Maybe User; match it: match m { Just x -> …, Nothing -> … }
  Query.succeed { user: user, todoStats: { total: total, done: done } }
```
Round 1 changed exactly that: it wrapped the rest of `load` in `match user { Just u -> { … Query.succeed { user: u, … } }, Nothing -> Query.notFound, }`. Every other file is byte-identical. With the blind retry of the first run, qwen re-emitted the same error. This is qwen's "`.field` on a `Maybe`" habit from E1–E4. One message fixed it, once the model could see the code it was fixing.

**qwen3-coder T3 (final, failing, 5 → 5): (b) ignored one error; (c) misled by another, and the wrong fix added hidden errors.** The edit itself is minimal: two lines changed and `User.alder` added. Before the fix, the retry re-rolled.
- **(b) Ignored:** `table Comment { id: brand Text @id, ... }` (an inline brand) is unchanged. The checker reports three diagnostics at one span: ``expected a type, got `brand` ``, ``expected `,` or closing brace``, ``expected end of line after declaration, got `brand` ``. None of them says a brand must be a declared type (`type CommentId = brand Text`).
- **(c) Misled:** `slug: Post.Slug @unique,` inside `form NewPost` gets ``Posts/Admin.alder:18:19: expected `,` or closing brace`` and ``expected end of line after declaration, got `@` ``, both pointing at the `@`. Nothing says form fields take no attributes. qwen guessed that the qualification was the problem: it changed the line to `slug: Slug @unique,` and also changed the page parameter in `Posts/Show.alder` to `{ slug: Slug }`. The parse errors stayed (they moved to column 14). Behind them, each unqualified `Slug` now produces an `E0202` and an ``E0011 … has type `_` ``, so the change is also a hidden (a).
- `User.alder` was added unasked. It had been missing since round 0, but no diagnostic said so, because the parse error stopped the check.

**No (d): the checker made no false positives in either run.** Every reported error, including the ones found under the parse errors, is a real mistake by the book.

### Counting

Check counts are per diagnostic and include cascades, so they run higher than E4's hand counts:
- One parse mistake gives two or three `E0001`s at the same span (all of qwen's T3 count).
- A parse error hides everything after it. qwen T3 round 1 with only the reported parse errors fixed by hand (scratch copy, not committed) gives 15 more errors: a missing `import Theme` in `App.alder` (6 diagnostics, problem 3), unqualified `Slug` in a form and a page parameter (4: 2 × E0202 + 2 × E0011 `_`), and the `Comment` table declared inside `Post` (5, problem 2).

## 3. Compiler problems found by E5

None is a false positive. Each is a message that did not lead the model, or a person, to the fix. All repros were re-run against the binary built for the re-run, using scratch copies of `examples/todos`.

**Problem 1: E0003 on a `let`-bound record literal does not suggest the annotation.** (Haiku T3, first run, commit 49e7792.) Reproduce by adding this test to `examples/todos/app/Todo.alder`:
```alder
test "let-bound literal" = {
  let t = { id: TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: UserId "u", created: Time.epoch }
  not (matches Active t)
}
```
```
app/Todo.alder:43:23: error[E0003]: expected Todo, got { id: Todo.TodoId, title: Text.NonEmpty, done: Bool, due: Maybe _, owner: User.UserId, created: Time }
```
`let t : Todo = { ... }` passes. **M3:** when the actual type is an anonymous record that matches the expected record's fields, give a `fix` adding the annotation at the `let`. **Book now:** chapter 10's examples all pass literals inline. Add an example of `let t : Todo = { … }` in a test, since binding a fixture first is the natural form.

**Problem 2: a table declared in another table's module is accepted, and its uses get a wrong fix.** (qwen T3, both runs; also qwen's E4 T3 "Comment declared inside Post".) Chapter 8 says "A table is declared in its own module named after it", but the checker accepts this in `examples/todos/app/Todo.alder` with exit 0:
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
There is no module `Note`, so the fix is wrong. Qualifying it does not help either: `Db.from Todo.Note |> Db.where (Todo.Note.todo.eq p.id)` gives ``module `Todo.Note` is not imported; fix: import Todo.Note``. Such a table cannot be used from another module at all. **M3:** enforce the rule at the declaration: an error on `table Note` in module `Todo` saying "move it to `app/Note.alder`" (E0005 or E0011). At a use site, never offer `import X` when no module `X` exists.

**Problem 3: one missing import produces six errors, and an unresolved type produces E0011 noise.** (qwen T3, both runs.) Delete `import Theme` from `examples/todos/app/App.alder`:
```
app/App.alder:6:36: error[E0011]: session field `theme` has type `_`, which cannot be stored in a session
app/App.alder:6:43: error[E0201]: module `Theme` is not imported   fix: import Theme
app/App.alder:6:51: error[E0002]: unknown constructor `Light`
app/App.alder:20:46: error[E0201]: module `Theme` is not imported   fix: import Theme
app/App.alder:27:37: error[E0002]: unknown constructor `Light`
app/App.alder:34:41: error[E0002]: unknown constructor `Light`
```
Only `E0201` is the cause. The same ``has type `_` `` E0011 follows every unresolved type: in the re-run, after an unqualified `Slug`, it appears as ``form field `slug` has type `_`, which cannot come from a form`` and ``page parameter `slug` has type `_`, which cannot appear in a URL``. Chapter 11 says a module whose names could not be resolved is not checked further; that is not what happens here. **M3:** suppress E0011/E0003 on a `_` type from a failed resolution. When an unknown constructor is exported by a module that exists, say "`Light` is a constructor of `Theme.Theme`; import `Theme`" with the fix.

**Problem 4: parse errors with no reason.** (qwen T3, both runs.) The two parse mistakes qwen could not fix get only the parser's position, with no hint at the rule. `@unique` on a form field gives ``expected `,` or closing brace`` at the `@`, which misled qwen into changing the type (see §2). An inline `brand` in a table field gives three diagnostics. **M3:** dedicated messages for both. Attributes are allowed only on table fields, and a brand is declared with `type X = brand …`. Also collapse same-span cascades into one diagnostic.

**Problem 5 (design, not a bug): a parse error hides the rest of the program.** It is documented in chapter 11, and this run measured what it costs in a loop: qwen's T3 shows 5 errors per round while 15 wait behind them, 6 of them in `App.alder`, which parsed and was never checked. **M3:** parse every file and report all parse errors, then check the modules that parsed and whose imports parsed, the same way name-resolution failures are already gated.

## 4. Harness notes

- **Timing per round** (`summary.json`): qwen3-coder 31–56 s (mean 43 s, 1.3k–2.5k tokens out; the retries are the slow ones, since they now carry the previous files); Haiku 25–86 s (mean 56 s); Sonnet 14–34 s (mean 23 s). The matrix took 7 min 32 s (T2 2 min 15 s, T3+T4 5 min 18 s) because 7 of 9 runs passed in round 0.
- **Splitter:** Haiku wrapped its T2 files in code fences, and the splitter removed them. No preambles, no rejected paths, no `exit 2`, and no model errors, so no re-runs were needed.
- **Passes are real apps.** T3/T4 passes have 8–10 files and 249–361 lines; T2 passes have the example's 8 files plus `Profile.alder`, linked from `Todos/Index`, with logout as `<button click={App.Logout}>`. Haiku T3 and qwen T4 were read in full: all routes, forms, `msg`s and the theme switch are present, and Haiku's T3 shows drafts to their author only (`visible : Post -> Maybe User.UserId -> Bool`). Gaps the checker cannot see, recorded as design signals: qwen's T4 puts `class="dark"` on `<html>`, not `<body>`, and labels the buttons "Dark mode"/"Light mode"; qwen's T3 puts `requires user` on the public `/posts/{slug}` page.

## 5. What this changes in the plan

1. **A retry must carry the previous answer.** With it, qwen's T2 went from repeating its error to a one-hunk fix. Done in this milestone; every later loop experiment depends on it.
2. **Report every parse error and check past them** (problem 5). qwen's T3 failure is now all parse-layer: two mistakes whose messages give no reason (problem 4), with 15 errors queued behind them. Each retry can reveal only one layer, so one retry cannot get there.
3. **Fix the messages** (problems 1–4). Problem 2 is qwen's oldest habit (E4's "`Comment` inside `Post`"), and today the checker accepts the declaration and then suggests a fix that cannot work. Problem 4 is the one that actually stopped qwen in this run.
4. **Count distinct mistakes, not diagnostics,** when comparing with hand-scored experiments, or dedupe same-span diagnostics in `report_loop.py`.
5. Haiku and Sonnet are at the ceiling on T2–T4 in one shot. qwen3-coder remains the model to iterate on: rerun T3 with more retries once (2) and (4) are in, and run the Claude models several times per task, since a single run is one sample.
