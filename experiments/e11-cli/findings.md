# E11 findings: CLI-only development

*Run 2026-10-14, branch `e11-cli`.* The question (design: `docs/superpowers/specs/2026-10-14-e11-cli-only-design.md`):
when a model may not write a file and may only run `alder` commands (the generators, `edit`, `remove`, `fill` with
one checked expression per hole), how far does it get, which gaps are the model's and which the command set's, and
how does it compare with the file-emitting baseline on the same tasks?

- **H1, structure by construction:** no structural errors; every refusal is a logic or typing error inside one
  expression, or a command misused.
- **H2, fewer rounds, same behaviour:** command runs pass in no more rounds than file runs, with hidden pass rates and
  mutation scores at least as high.
- **H3, the command set is the ceiling:** every stall is classifiable, and "command missing" is rare.

**Matrix as run.** Haiku and Sonnet × the eight canonical tasks (`experiments/tasks/`: T2–T7, T9, T10) × one sample
(`s1`), in two conditions: `cli` (`runs/<model>/<task>-s1/`, `run_loop.py --cli`) and `files` (`baseline/runs/…`,
the file-emitting mode with `--build --test --mutate`). Both: the same system prompt (`prompt-reference.md`,
`build_context.py --reference`), the same budget (6 rounds, `--retries 5`), the same hidden tests (E9's for T5, T6,
T7, T9, T10; E8's for T4; none for T2, T3); T2 on `examples/todos`, T7 on `e8-hidden/base/blog-nodrafts` with
`--migrate`. 32 cells. Runner: `run-matrix.sh`; progress in `logs/matrix-*.progress`; one line per cell in
`logs/{cli,baseline}-{haiku,sonnet}.log`. Tables: `python3 experiments/report_loop.py experiments/e11-cli` and
`python3 experiments/report_loop.py experiments/e11-cli/baseline`.

**Two deviations from the design.**
1. **No Opus.** The design had Opus in both conditions; it was dropped for budget (8 tasks × up to 6 rounds of long
   scripts). Every claim below is about Haiku and Sonnet only.
2. **All 16 baseline cells were run fresh** (the design reused E9's `baseline` cells for T5–T10 and ran only T2–T4
   fresh). Both conditions therefore share the system prompt; E9's cells (full-book prompt, `prompt-full.md`) are
   compared separately in §3.

**Binary.** `experiments/e11-cli/alder` (git-ignored), built at `87c7b42` (`alder version`: `devel+87c7b427c0a6`),
sha256 `2e10458ac83092e4f89b3ba87231b68c3c84c82903a158e07ebc2a418f098a6d`. The `cli` matrix was paused 08:55–17:33
(`logs/matrix-cli.progress`, "STOPPED for 9-5"); the restart skipped the finished cells, none was re-run.

Every number below comes from a cell's `summary.json`, its `round<N>/commands.jsonl`, `script.txt`,
`transcript.md`, `output.md` or `check.json`; a cell and round are cited as *model task rN* (e.g. *Haiku t6 r3*).
The stall classes (§6) are *hand-scored* from the transcripts.

## 1. Headline

| | `files` | `cli` |
|---|---|---|
| Cells passing their own tests (the harness's `pass`) | **16 / 16** | **12 / 16** (Haiku 7 / 8, Sonnet 5 / 8) |
| Rounds used by the passing cells | 1–2 (Haiku 14 rounds for 8 cells, Sonnet 9 for 8) | 4–6 (mean 4.8: Haiku 34 for 7, Sonnet 24 for 5) |
| Rounds whose app type-checked | 16 / 23 (7 round 0s failed `alder check`) | **82 / 82** |
| Hidden tests passed (T4–T10: 6 tasks × 2 models, 94 tests) | **88 / 94** (T4: 5 / 8 in both cells) | **60 / 94** (17 failed to compile, 17 not run: 2 cells never built) |
| Cells passing every hidden test | 10 / 12 | 8 / 12 |
| Mutation score, the 12 cells passing in both conditions | mean 0.67 | mean **0.80** (higher in 10 / 12) |
| Commands written / run | — | 1,779 / 640 (36 %) |
| Refusals (each ends its round) | — | 64 (Haiku 30, Sonnet 34) |

- **H1 holds for the app, not for the model.** No round of 82 left an app that did not type-check (`errors: 0` in
  every round); in `files`, round 0 failed the check in 7 of 16 cells (14 errors: E0001 ×4, E0028 ×6, E0201 ×4).
  But structural mistakes still occur, as refusals: 13 of the 640 commands run were refused for an import (E0201 ×2),
  a name used before the command that makes it (×5) or a name's case or a name taken (×6). Each cost the rest of its
  round's script instead of a line in a broken file. (§4)
- **H2 is refuted on rounds and pass rate, mixed on behaviour.** Every passing `cli` cell needed 4–6 rounds against
  1–2; 4 cells failed (both T4s, Sonnet T5 and T6) where `files` passed all 16. Hidden: 60 / 94 against 88 / 94.
  The mutation score is higher in 10 of 12 comparable cells (mean 0.80 against 0.67), because the generators write
  tests: of the 150 tests the eight passing bare-start `cli` cells ran last, 38 came from the model's own
  `alder new test`, the other 112 from the generators (`new table`'s delete-rule tests, 19 of them; `new form`,
  `new msg`, `new auth`, `--requires`). (§2, §3)
- **H3: gaps in three task shapes; the reference is the larger gap.** Of 64 refusals, 6
  are "command missing" (an `App`-level message ×2, a prop on the layout ×2, a form posted by a message with
  parameters ×2), 49 "command misused", 8 a body that did not check or a test that failed, 1 a tool hang, 0 a misread
  task. The 6 missing-command refusals sit in the 4 cells that did not reach the task: both T4s and both T6s (Haiku
  T6 passed its own tests with a form that differs from the task's Names and failed all 9 hidden tests). Of the 49
  misuses, 13 are one pattern: a lambda over parameters the declaration already binds (`\params session -> …` sent
  to `fill --decl Page.load`), which neither the reference nor the refusal's sentence names. (§5–§7)
- **The models never used `--json` or `--dry-run`** (0 of 1,779 commands). `--help` was written twice (*Sonnet t6
  r5*) and never ran (the script stopped at its 2nd command). In a script round, output cannot steer the same round.
- **A tool bug:** `alder fill --decl Orders.Show.view -` ran 600 s and was killed (*Haiku t6 r3*). Reproduced on a
  copy of that round's app: the body hangs on its `×` and returns at once with `x`. The cause: `qualifiers`
  (`internal/generate/fill.go:179`) tests `unicode.IsUpper(rune(ch))` on one byte, so the first byte of any 2-byte
  UTF-8 character (U+0080 and up) after a non-name byte (0xC3 reads as `Ã`) starts a name, `isNameByte` is false so
  `j == i`, and `i = j - 1` never advances: an infinite loop. (§6)

## 2. Per task × model

### `cli` (`runs/`)

| Model | Task | Rounds | Pass | Own tests (last round) | Hidden | Mutants | Cmds run / written | Refused | Stdin lines | Max body | `--decl` | Names |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Haiku | T2 newpage | 6 | r5 | 96 / 96 | — | 157 / 160 | 16 / 41 | 5 | 118 | 27 | 7 | — |
| Haiku | T3 newapp | 4 | r3 | 16 / 16 | — | 21 / 26 | 41 / 70 | 3 | 219 | 21 | 20 | — |
| Haiku | T4 appmsg | 6 | **fail** | 9 / 9 (r5 stopped at command 4) | 0 / 8 (check: E0002 ×14) | — | 23 / 176 | 6 | 22 | 15 | 4 | — |
| Haiku | T5 ownership | 4 | r3 | 15 / 15 | 8 / 8 | 15 / 16 | 35 / 99 | 3 | 130 | 22 | 12 | ok |
| Haiku | T6 orders | 6 | r5 | 21 / 21 | **0 / 9** (check: E0002 ×2, E0003 ×2) | 40 / 50 | 71 / 182 | 5 | 255 | 20 | 13 | ok |
| Haiku | T7 drafts | 5 | r4 | 29 / 29 | 7 / 7 | 37 / 55 | 29 / 84 | 4 | 269 | 27 | 19 | ok |
| Haiku | T9 bookings | 5 | r4 | 18 / 18 | 7 / 7 | 26 / 38 | 62 / 102 | 2 | 231 | 19 | 12 | ok |
| Haiku | T10 teams | 4 | r3 | 27 / 27 | 8 / 8 | 53 / 68 | 65 / 154 | 2 | 344 | 32 | 24 | ok |
| Sonnet | T2 newpage | 5 | r4 | 97 / 97 | — | 158 / 160 | 18 / 40 | 4 | 92 | 28 | 3 | — |
| Sonnet | T3 newapp | 5 | r4 | 14 / 14 | — | 19 / 23 | 38 / 93 | 4 | 118 | 21 | 7 | — |
| Sonnet | T4 appmsg | 6 | **fail** | never built | not run | — | 26 / 163 | 6 | 15 | 4 | 1 | — |
| Sonnet | T5 ownership | 6 | **fail** | 11 / 12 | 8 / 8 | — | 32 / 87 | 5 | 103 | 14 | 9 | ok |
| Sonnet | T6 orders | 6 | **fail** | never built | not run | — | 51 / 189 | 6 | 102 | 12 | 5 | 7 missing |
| Sonnet | T7 drafts | 5 | r4 | 29 / 29 | 7 / 7 | 37 / 54 | 30 / 54 | 4 | 270 | 34 | 19 | ok |
| Sonnet | T9 bookings | 4 | r3 | 16 / 16 | 7 / 7 | 27 / 42 | 46 / 96 | 2 | 191 | 13 | 13 | ok |
| Sonnet | T10 teams | 5 | r4 | 23 / 23 | 8 / 8 | 51 / 68 | 57 / 149 | 3 | 217 | 18 | 16 | ok |

Names "—": the task has no Names block (T2–T4). Every refusal is `exit1` except one `timeout` (*Haiku t6 r3*); no
`parse`, `not-a-command`, `cap` or `exit2` refusal occurred, and every answer had its one `alder-cli` block. The
largest bodies: 34 lines (`fill --decl Admin.view`, *Sonnet t7 r1*), 32 (`fill --decl Invites.Index.update`, *Haiku
t10 r2*), 28 (`fill --decl Todos.Index.view`, *Sonnet t2 r2*), 27 (`Todos.Index.view`, *Haiku t2 r5*;
`Admin.view`, *Haiku t7 r2*). The T10 drill's largest was 18 (book 15).

### `files` (`baseline/runs/`)

| Model | Task | Rounds | Round 0 check errors | Own tests | Hidden | Mutants |
|---|---|---|---|---|---|---|
| Haiku | T2 | 2 | E0028 | 94 | — | 157 / 161 |
| Haiku | T3 | 2 | E0028 ×5 | 8 | — | 14 / 26 |
| Haiku | T4 | 2 | E0201 ×2 | 11 | 5 / 8 | 19 / 22 |
| Haiku | T5 | 2 | E0001 | 12 | 8 / 8 | 17 / 20 |
| Haiku | T6 | 1 | — | 8 | 9 / 9 | 18 / 48 |
| Haiku | T7 | 1 | — | 30 | 7 / 7 | 38 / 55 |
| Haiku | T9 | 2 | E0001 ×2 | 13 | 7 / 7 | 24 / 41 |
| Haiku | T10 | 2 | E0001 | 14 | 8 / 8 | 30 / 55 |
| Sonnet | T2 | 1 | — | 98 | — | 160 / 161 |
| Sonnet | T3 | 2 | E0201 ×2 | 11 | — | 16 / 25 |
| Sonnet | T4 | 1 | — | 7 | 5 / 8 | 14 / 22 |
| Sonnet | T5 | 1 | — | 7 | 8 / 8 | 9 / 18 |
| Sonnet | T6 | 1 | — | 4 | 9 / 9 | 12 / 50 |
| Sonnet | T7 | 1 | — | 30 | 7 / 7 | 36 / 53 |
| Sonnet | T9 | 1 | — | 10 | 7 / 7 | 23 / 40 |
| Sonnet | T10 | 1 | — | 8 | 8 / 8 | 32 / 58 |

All 16 pass; every retry was for a check error in round 0 (`round0/check.json`: E0001 "the last expression of a
block is its result and takes no `,`"; E0028 "`<al-input field=…>` already shows the errors of `…`; remove
`<Errors field=… />`"; E0201 "module `User` is not imported"), fixed in round 1.

**T4's hidden tests measure names the canonical text does not give.** The canonical T4 (`experiments/tasks/
t4-appmsg.md`) has no Names block; E8's T4, for which the hidden module was written, had one (`msg SwitchTheme` in
`App`, `form EditNote`, …). Both `files` cells fail the same 3 of 8 hidden tests (the switch's label, the note page's
title element, the redirect after adding a note), each a detail E8's block fixed. Their `App` does declare
`msg SwitchTheme` (the reference: "Actions that belong to no page (log out, switch theme) are declared as `msg`…");
the `cli` scripts called theirs `ToggleTheme`. T4's hidden results compare the conditions only as "did the module
compile".

## 3. The comparison, per task × model

Rounds = rounds used (the passing round + 1, or 6 on a fail); mutants killed / ran.

| Model | Task | `cli` rounds | `files` rounds | `cli` hidden | `files` hidden | `cli` mutants | `files` mutants | E9 `baseline` s1 (full-book prompt): rounds, hidden, mutants |
|---|---|---|---|---|---|---|---|---|
| Haiku | T2 | 6 | 2 | — | — | 157 / 160 (0.98) | 157 / 161 (0.98) | — |
| Haiku | T3 | 4 | 2 | — | — | 21 / 26 (0.81) | 14 / 26 (0.54) | — |
| Haiku | T4 | fail | 2 | 0 / 8 | 5 / 8 | — | 19 / 22 (0.86) | — |
| Haiku | T5 | 4 | 2 | 8 / 8 | 8 / 8 | 15 / 16 (0.94) | 17 / 20 (0.85) | 1, 8 / 8, 12 / 18 |
| Haiku | T6 | 6 | 1 | 0 / 9 | 9 / 9 | 40 / 50 (0.80) | 18 / 48 (0.38) | 1, 9 / 9, 15 / 54 |
| Haiku | T7 | 5 | 1 | 7 / 7 | 7 / 7 | 37 / 55 (0.67) | 38 / 55 (0.69) | 1, 7 / 7, 40 / 56 |
| Haiku | T9 | 5 | 2 | 7 / 7 | 7 / 7 | 26 / 38 (0.68) | 24 / 41 (0.59) | 1, 7 / 7, 20 / 39 |
| Haiku | T10 | 4 | 2 | 8 / 8 | 8 / 8 | 53 / 68 (0.78) | 30 / 55 (0.55) | 2, 7 / 8, 32 / 57 |
| Sonnet | T2 | 5 | 1 | — | — | 158 / 160 (0.99) | 160 / 161 (0.99) | — |
| Sonnet | T3 | 5 | 2 | — | — | 19 / 23 (0.83) | 16 / 25 (0.64) | — |
| Sonnet | T4 | fail | 1 | not run | 5 / 8 | — | 14 / 22 (0.64) | — |
| Sonnet | T5 | fail | 1 | 8 / 8 | 8 / 8 | — (own tests failed) | 9 / 18 (0.50) | 1, 8 / 8, 12 / 18 |
| Sonnet | T6 | fail | 1 | not run | 9 / 9 | — | 12 / 50 (0.24) | 1, 9 / 9, 10 / 50 |
| Sonnet | T7 | 5 | 1 | 7 / 7 | 7 / 7 | 37 / 54 (0.69) | 36 / 53 (0.68) | 1, 7 / 7, 36 / 53 |
| Sonnet | T9 | 4 | 1 | 7 / 7 | 7 / 7 | 27 / 42 (0.64) | 23 / 40 (0.57) | 1, 7 / 7, 25 / 40 |
| Sonnet | T10 | 5 | 1 | 8 / 8 | 8 / 8 | 51 / 68 (0.75) | 32 / 58 (0.55) | 1, 8 / 8, 24 / 53 |

E9 columns: `python3 experiments/report_loop.py experiments/e9-feedback/baseline`, the `s1` rows. With the reference
prompt (this run's `files`) and the full-book prompt (E9), the file cells look alike on T5–T10: one or two rounds,
the hidden tests passed (except E9's Haiku T10, 7 / 8: its `Remove`, a task-text defect since fixed), scores 0.24–0.85. The
`cli` condition is the outlier on all three columns.

**Why the `cli` scores are higher.** The suites are bigger and mostly generated. The eight passing `cli` cells that
start bare (T3, T5, T6, T9, T10) ran 150 tests in their last round; the model's own `alder new test` commands that
ran made 38 of them. The other 112 are the generators': `new table`'s delete-rule tests (19, e.g. "deleting its post
deletes a Comment"), `new auth`'s login tests and `App`'s logout test, a `--requires`
page's "a visitor is sent to log in", `new form`'s "submitting X adds a row" and "X refuses an empty …", `new msg`'s
"pressing Y" (their `?expected` holes filled by the model). The `files` cells on the same tasks ran 84 tests in all.
Where most of the app is the base's (T2 on `examples/todos`, T7 on the blog), the scores are equal in both
conditions (0.98 / 0.98, 0.99 / 0.99, 0.67 / 0.69, 0.69 / 0.68). And the score did not track behaviour: *Haiku t6*'s
40 / 50 (0.80, higher than every T6 cell here or in E9's `s1` rows, Opus's 37 / 51 included) belongs to an app that
fails all 9 hidden tests (§5.2).

## 4. H1: structure by construction

**The app.** Each command checks a copy and writes only if it checks, so the app between commands always checks:
`errors: 0` in all 82 `cli` rounds (the report's `r0…r5` columns). In `files`, round 0 failed the check in 7 of 16
cells.

**The refusals.** The `files` error classes recur in `cli`, inside one command, and are refused without writing:

| Class | `files` round 0 | `cli` refusals (cell, round) |
|---|---|---|
| E0001, a `,` after a block's last expression | 4 errors, 3 cells | 2: `fill --decl Admin.load`, `fill --decl Posts.Show.load` ("standard input does not parse as one: expected `}`, got `,`", *Haiku t7 r1, r2*) |
| E0028, an `<Errors field>` beside an `<al-input>` | 6 errors, 2 cells | 1: `fill --decl Profile.view` (*Haiku t2 r2*) |
| E0201, a module not imported | 4 errors, 2 cells | 2, both in a generator's type argument: `alder edit session add-field theme:Theme=Light` ("module `Theme` is not imported", *Haiku t4 r0*), `alder new table Post … slug:Slug@unique` ("module `Slug` is not imported", *Sonnet t3 r0*). None in a `fill`: `fill` imports an app module its expression names. |
| A name used before the command that makes it | — | 5: `unknown Teams.Index` (*Haiku t10 r0*), `no field teams on Teams.Index.Model` (*Haiku t10 r1*), `unknown Notes.Show` (*Haiku t5 r2*), `unknown Notes.Index` (*Sonnet t4 r0*), `unknown Rooms.Show` (*Sonnet t9 r0*) |
| A name's case, or a name taken | — | 6: `new decl Layout Shell` / `Themed` ("is not a value name", *Haiku t4 r3, Sonnet t4 r3, Sonnet t6 r0*); `new test Admin "a visitor is sent to log in"` (made by `--requires` already, *Haiku t3 r1*); `new form Orders.Show NewLine … --msg AddLine` ("already declares `AddLine`", *Haiku t6 r2, Sonnet t6 r2*) |

**Verdict.** As the design states H1 ("every refusal is a logic or typing error inside one expression, or a command
misused"), it holds: every refusal falls in those classes (§6), and the app never carried a structural error. As the
book states it (chapter 15, "a structural error … cannot be made"), it needs a qualifier: the model still makes
structural mistakes — 13 of the 640 commands run (import ×2, order ×5, naming ×6) — and the command turns each into a
refusal instead of an error in a file. The cost differs: a check error in `files` is fixed with the rest of the app
in the next round; a refusal in `cli` discards every later command of the script (§5.6).

## 5. Why the failing cells failed

### 5.1 T4 (both models): the task's shape is outside the commands

T4 asks for a session theme with a switch button on every page and `<body class="dark">`. Both models planned it as
the reference describes: an `App`-level message and a layout that takes the theme. Their script comments: "the theme
switch: an app-level message, flipping the session's theme" (*Haiku t4 r2*), "app-level messages" (*Sonnet t4 r2*).

- `alder new msg App ToggleTheme` → "`App` is not a page (it has no `page` declaration)" (*Haiku t4 r2, Sonnet t4
  r2*). **No command declares a message in `App`**; `new auth` writes `msg Logout` there and nothing else can.
- `alder new decl Layout Shell "{ dark: Bool, title: Text, children: List (Html a) } -> Html a"` → "`Shell` is not a
  value name" (*Haiku t4 r3*; *Sonnet t4 r3* with `Themed`). **No command adds a prop to `Layout.Page`** or declares
  a component; `fill --decl` replaces a body, not a signature.
- The workaround (a `ToggleTheme` message on every page, a lower-case layout function) was reachable, and both found
  it: `alder new decl Layout shell …` ran in *Haiku t4 r5*; `new msg Notes.Index ToggleTheme` and the like are in
  every script from *Haiku r3* and *Sonnet r3*. Neither got there: each later round stopped early on a misuse —
  `--param` with `--show` (*r1*, both), a test block without `Task.succeed` (*Haiku r4*), `fill --decl
  Notes.Index.Model` on a `--list` page whose generated `load` still builds `{ rows, page, pages }` (*Haiku r5*), the
  lambda pattern (*Sonnet r4*), `new form` on a one-line placeholder view (*Sonnet r5*). Haiku's last app has no
  `Notes.Show` and no theme message; `Layout.Page` has no theme (the unused `Layout.shell` takes `dark`); the hidden module's 14 E0002 errors are `App.SwitchTheme`
  ×5, `Notes.Show` ×7, `Notes.Index.NewNote`, `Notes.Show.EditNote` (`alder check` re-run on a copy of
  `round5/appmsg/hidden`). Sonnet's last round left 4 holes and never built.

Stall: **command missing** (two rounds each), then **command misused** until the budget ran out.

### 5.2 Haiku T6: own tests pass, the hidden module does not compile — forced off the Names

The Names block says `form NewLine = { quantity: Int.Positive }`, posted by `msg AddLine { product:
Product.ProductId } form NewLine`. Haiku built exactly that, in that order (*r2*): `alder new msg Orders.Show AddLine
product:Product.ProductId`, then `alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine` →
"`Orders.Show` already declares `AddLine`: choose another name". Its next answer (*r3* `output.md`): "no command can
add a form to an existing message. The form now carries the product as a select field, not as a message parameter,
so the spec's `msg AddLine { product } form NewLine` shape isn't reachable with these commands." It removed the page
and ran `alder new form Orders.Show NewLine product:Product.ProductId quantity:Int.Positive --msg AddLine`. The app
passes its 21 tests; the hidden module does not compile: "no field `product` on `Orders.Show.AddLineParams`" and
"record `Orders.Show.NewLine` is missing field(s): product" (×2 each, `summary.json` `hidden`). The names check
reported **ok**: it checks that a page's messages and forms exist, not their fields or parameters.

Stall: **command missing** — `new form … --msg M` cannot attach a form to an existing message with parameters, and
`new msg` has no `--form`; either would have kept the task's names. A names check comparing form fields and message
parameters would have flagged the deviation.

### 5.3 Sonnet T6: never builds

Sonnet hit the same `AddLine` refusal (*r2*, its 15th command of 42). Its *r3* comment: "free Orders.Show of
references so it can be recreated with a form on AddLine". Then three rounds each ended at their 2nd command, each on
a different misuse: `alder fill Orders.Index.update.Create -` on an arm *r2* had already filled ("has no typed hole:
… give `alder fill --decl Orders.Index.update -` the whole `match msg { … }`", *r3*); `alder fill Auth.Login -
--hole ?afterLogin`, a module where a declaration goes ("names no module of this app", *r4*); `alder fill
Orders.Index."pressing Create" - --hole ?expected` with the address unquoted, which word-splits, as in a shell, to
`Orders.Index.pressing Create` (*r5*). Its *r5* script ended with `alder new msg --help` and `alder new form --help`,
never reached. The 7 names mismatches are all "missing" (`Products.Index` and its form and messages, `Pay`,
`Cancel`, `NewLine`): the commands that make them were never run. 5 holes left.

Stall: **command missing** (the form on `AddLine`), then **command misused**, one per round.

### 5.4 Sonnet T5: one generated test, one round short

*Sonnet t5 r5* ran all 7 commands; the 7th was its own `alder test`, which failed one test, so the round counts as
stopped. The failing test is `new form`'s "NewNote refuses an empty title": left `FormRejected [{ field: "",
message: "title must not be empty" }]`, right `FormRejected [{ field: "title", message: "must not be empty" }]`. The
model's view (`fill --decl Notes.Index.view`, *r3*) puts `<Input field="title" …/>` under one `<Errors form />` with
no list for the field, so the field's error lands in the form's list (book 10, "`FormRejected errors`"). Hidden
tests: 8 / 8. The earlier rounds were lost to `--requires user` before `new auth` (*r0*), `fill Notes.Show.Model`
without `--decl` (*r1*), the lambda pattern (*r2*), an unquoted test address (*r3*) and a generator's hole left
(*r4*).

Stall: **body (logic)** in the last round, after four **misused** rounds.

### 5.5 The holes that stop `alder build`

52 of 82 `cli` rounds ended with `alder build` refusing ("the program has N typed hole(s); fill them before
building"); no round's build failed for any other reason. 47 of the 52 are rounds whose script stopped early, so the
fills after the refusal never ran. The other 5 ran every command and still left holes (*Haiku t10 r2*, *Haiku t9
r1*, *Sonnet t10 r3*, *Sonnet t9 r2*, *Sonnet t5 r4*), and every hole left there is a generator's hole in a test:
`?expected` in `new form`'s and `new msg`'s tests, `?afterLogin` in `new auth`'s "logging in through the form"
(the rounds' build output). Each cost a round.

Did the models know a hole stops `alder test` (the reference says so in one line)? Sonnet ran `alder test`
mid-script with a hole left twice ("alder test: no tests ran (fill the hole)", *Sonnet t2 r2, t7 r1*). Five scripts
that ran contain `alder fill --list` (*Haiku t10 r2; Sonnet t10 r3, t9 r2, t9 r3, t5 r4*), the last command only in
*Haiku t10 r2* and *Sonnet t9 r2*; in a script its answer can only steer the next round. `new auth` leaves `?afterLogin` twice (in `update` and in its test) unless a `/` page exists or `--to` names
one; `--to` needs the page to exist, and a `--requires user` page needs the session `new auth` writes, so with the
order the reference gives ("tables, `new auth`, pages, fills") both holes stay until the model fills them one by one.
*Sonnet t6 r4*'s `alder fill Auth.Login - --hole ?afterLogin` was an attempt to fill both at once.

### 5.6 Did "a script that stops early" feed back well?

For continuity, mostly; for throughput, no. One refusal came from re-running a command an earlier round had run:
#56 (*Sonnet t6 r3*) re-sent the fill of `Orders.Index.update.Create` that *r2* had run, and the arm had no hole left.
The three "already declares/has" refusals hit generator-made names or the same script's own, and every next script
started from the refused command. But 64 of 82 rounds stopped early, and the scripts did not shrink: Haiku sent
15–38-command scripts in *t4 r1–r5*, of which 1–6 ran. Of 1,779 commands written, 640 ran. `--dry-run` and
`--json`, which exist to find a refusal without losing a round, were never used.

## 6. The stall table (all 64 refusals, hand-classified)

Classes: **missing** (no command does it), **misused** (a wrong flag, argument, address or order; a fill on a
non-hole), **body** (the one expression does not check, or a test the round ran fails), **tool** (the command hung),
**misread** (none).

| # | Cell | Command (short) | Reason (the command's sentence, short) | Class |
|---|---|---|---|---|
| 1 | Haiku t10 r0 | `fill --decl Auth.Login.update` | unknown `Teams.Index` (page made later) | misused: order |
| 2 | Haiku t10 r1 | `fill --decl Teams.Index.load` | no field `teams` on `Teams.Index.Model` (Model not filled yet) | misused: order |
| 3 | Haiku t2 r0 | `fill --decl Profile.load` | expected `Query Profile.Model`, got `_ -> _ -> Query …` | misused: lambda |
| 4 | Haiku t2 r1 | `fill --decl Profile.view` | expected `Text.NonEmpty`, got `Text` (an input's value) | body |
| 5 | Haiku t2 r2 | `fill --decl Profile.view` | E0028, a duplicate `<Errors field="name">` | body |
| 6 | Haiku t2 r3 | `fill 'Profile."submitting Rename"'` (a whole block) | cannot compare `Answer` with `Task Bool` (the block went into `?expected`) | misused: hole |
| 7 | Haiku t2 r4 | `fill 'Profile."Rename refuses an empty name"'` | has no typed hole | misused: non-hole |
| 8 | Haiku t3 r0 | `fill --decl Layout.Page` (`\props -> …`) | expected `Html a`, got `_ -> Html _` | misused: lambda |
| 9 | Haiku t3 r1 | `new test Admin "a visitor is sent to log in"` | already has that test (`--requires` made it) | misused: other |
| 10 | Haiku t3 r2 | `fill 'Posts."submitting NewComment"'` (a block) | cannot compare `Answer` with `Task Bool` | misused: hole |
| 11 | Haiku t4 r0 | `edit session add-field theme:Theme=Light` | E0201 module `Theme` is not imported | misused: unqualified type |
| 12 | Haiku t4 r1 | `new page Notes.Show … --param id:… --show Note` | `--param` goes with a page of its own or a `--blank` one | misused: flag |
| 13 | Haiku t4 r2 | `new msg App ToggleTheme` | `App` is not a page | **missing** |
| 14 | Haiku t4 r3 | `new decl Layout Shell "{ dark: Bool, … } -> Html a"` | not a value name | **missing** |
| 15 | Haiku t4 r4 | `fill 'App."the theme flips"'` | expected `Task Bool`, got `Bool` | body |
| 16 | Haiku t4 r5 | `fill --decl Notes.Index.Model` (a `--list` page) | Model is missing `notes`; no field `rows`, `page`, `pages` | misused: `--list` Model |
| 17 | Haiku t5 r0 | `fill --decl Notes.Index.Model` (a `--list` page) | the same | misused: `--list` Model |
| 18 | Haiku t5 r1 | `fill --decl Notes.Index.load` | got `_ -> _ -> Query …` | misused: lambda |
| 19 | Haiku t5 r2 | `fill --decl Notes.Index.view` | unknown `Notes.Show` | misused: order |
| 20 | Haiku t6 r0 | `new page Orders.Index / --requires user --blank` | needs a session with a `user` field | misused: order |
| 21 | Haiku t6 r1 | `fill --decl Orders.Index.load` | got `_ -> _ -> Query …` | misused: lambda |
| 22 | Haiku t6 r2 | `new form Orders.Show NewLine quantity:… --msg AddLine` | already declares `AddLine` | **missing** |
| 23 | Haiku t6 r3 | `fill --decl Orders.Show.view` | stopped after 600 s | **tool** |
| 24 | Haiku t6 r4 | `fill Products.Index.update.Add --hole ?add` | this arm has none; give `fill --decl … update` the whole match | misused: non-hole |
| 25 | Haiku t7 r0 | `fill 'Posts.Index."the front page lists …"'` | has no typed hole | misused: non-hole |
| 26 | Haiku t7 r1 | `fill --decl Admin.load` | does not parse: a `,` after the last expression | body |
| 27 | Haiku t7 r2 | `fill --decl Posts.Show.load` | the same | body |
| 28 | Haiku t7 r3 | `fill 'Posts.Show."a post's body is rendered as Markdown"'` | has no typed hole | misused: non-hole |
| 29 | Haiku t9 r0 | `fill --decl Rooms.Index.load` | got `_ -> _ -> Query …` | misused: lambda |
| 30 | Haiku t9 r2 | `fill 'Rooms.Show."NewBooking refuses an empty from"'` | has no typed hole | misused: non-hole |
| 31 | Sonnet t10 r0 | `new page Teams.Index … --requires user` | needs a session with a `user` field | misused: order |
| 32 | Sonnet t10 r1 | `fill --decl Teams.Index.view` (`\model session -> …`) | got `_ -> _ -> Html _` | misused: lambda |
| 33 | Sonnet t10 r2 | `new form Teams.Index NewTeam …` | `view` is not one element over several lines | misused: precondition |
| 34 | Sonnet t2 r0 | `fill Profile.load` | got `_ -> _ -> Query …` | misused: lambda |
| 35 | Sonnet t2 r1 | `new form Profile Rename …` | `view` is not one element over several lines | misused: precondition |
| 36 | Sonnet t2 r2 | `alder test` | no tests ran (fill the hole) | misused: test with a hole |
| 37 | Sonnet t2 r3 | `fill 'Profile."submitting Rename"'` (a block) | cannot compare `Answer` with `Task Bool` | misused: hole |
| 38 | Sonnet t3 r0 | `new table Post … slug:Slug@unique …` | E0201 module `Slug` is not imported | misused: unqualified type |
| 39 | Sonnet t3 r1 | `fill Home.load` | got `_ -> _ -> Query …` | misused: lambda |
| 40 | Sonnet t3 r2 | `fill Desk.update.CreatePost` | this arm has none | misused: non-hole |
| 41 | Sonnet t3 r3 | `alder test` | 1 failed: "NewComment refuses an empty author" (GET /posts/slug-1: not found) | body |
| 42 | Sonnet t4 r0 | `fill Auth.Login.update --hole ?afterLogin` | unknown `Notes.Index` | misused: order |
| 43 | Sonnet t4 r1 | `new page Notes.Show … --show Note … --param …` | `--param` goes with a page of its own or a `--blank` one | misused: flag |
| 44 | Sonnet t4 r2 | `new msg App ToggleTheme` | `App` is not a page | **missing** |
| 45 | Sonnet t4 r3 | `new decl Layout Themed "{ title, dark, children } -> Html a"` | not a value name | **missing** |
| 46 | Sonnet t4 r4 | `fill Notes.Index.load` | got `_ -> _ -> Query …` | misused: lambda |
| 47 | Sonnet t4 r5 | `new form Notes.Index NewNote …` | `view` is not one element over several lines | misused: precondition |
| 48 | Sonnet t5 r0 | `new page Notes.Show … --requires user` | needs a session with a `user` field | misused: order |
| 49 | Sonnet t5 r1 | `fill Notes.Show.Model` | has no typed hole | misused: non-hole |
| 50 | Sonnet t5 r2 | `fill --decl Notes.Index.view` (`\model session ->`) | got `_ -> _ -> Html …` | misused: lambda |
| 51 | Sonnet t5 r3 | `fill Notes.Index."submitting NewNote" --hole ?expected` | `Notes.Index.submitting NewNote` has no typed hole | misused: quoting |
| 52 | Sonnet t5 r5 | `alder test` | 1 failed: "NewNote refuses an empty title" | body |
| 53 | Sonnet t6 r0 | `new decl Layout Shell "{ title, children } -> Html a"` | not a value name (`Layout.Page` has that type already) | misused: other |
| 54 | Sonnet t6 r1 | `fill Orders.Index.load` | got `_ -> _ -> Query …` | misused: lambda |
| 55 | Sonnet t6 r2 | `new form Orders.Show NewLine quantity:… --msg AddLine` | already declares `AddLine` | **missing** |
| 56 | Sonnet t6 r3 | `fill Orders.Index.update.Create` | this arm has none | misused: non-hole |
| 57 | Sonnet t6 r4 | `fill Auth.Login --hole ?afterLogin` | names no module of this app | misused: other |
| 58 | Sonnet t6 r5 | `fill Orders.Index."pressing Create" --hole ?expected` | `Orders.Index.pressing Create` has no typed hole | misused: quoting |
| 59 | Sonnet t7 r0 | `fill --decl Posts.Index.load` | got `_ -> _ -> Query …` | misused: lambda |
| 60 | Sonnet t7 r1 | `alder test` | no tests ran (fill the hole) | misused: test with a hole |
| 61 | Sonnet t7 r2 | `fill --decl Admin."pressing Publish"` | `Admin` has no declaration `pressing Publish` | misused: quoting |
| 62 | Sonnet t7 r3 | `alder test` | 1 failed: "pressing Publish" (the page has no button for it) | body |
| 63 | Sonnet t9 r0 | `fill Rooms.Index.view` | unknown `Rooms.Show` | misused: order |
| 64 | Sonnet t9 r1 | `fill --decl Rooms.Show.update` (`\msg model session ->`) | got `Msg -> _ -> _ -> Query _` | misused: lambda |

**Counts.**

| Class | Haiku | Sonnet | Total |
|---|---|---|---|
| missing | 3 | 3 | **6** |
| misused | 21 | 28 | **49** |
| — lambda over bound parameters | 5 | 8 | 13 |
| — fill on a non-hole (a test, an arm, `Model`) | 5 | 3 | 8 |
| — order (a page or field used before it exists; `--requires user` before `new auth`) | 4 | 4 | 8 |
| — a block into a generated test's `?expected` hole | 2 | 1 | 3 |
| — a test address unquoted | 0 | 3 | 3 |
| — `new form` on a one-line view | 0 | 3 | 3 |
| — other (a duplicate test name; upper-case `new decl` where `Layout.Page` existed; `Auth.Login` as an address) | 1 | 2 | 3 |
| — `--param` with `--show` | 1 | 1 | 2 |
| — `Model` of a `--list` page | 2 | 0 | 2 |
| — an unqualified type in a generator argument | 1 | 1 | 2 |
| — `alder test` with a hole left | 0 | 2 | 2 |
| body | 5 | 3 | **8** |
| tool | 1 | 0 | **1** |
| misread | 0 | 0 | **0** |

**The tool hang (#23).** The 20-line view of *Haiku t6 r3* kept `alder fill --decl` running past the harness's
600 s. The same body in *r4*, with `×` replaced by `x` and `<al-card>` by `<div>`, ran in 0.02 s. On a copy of
`round3/orders`: the original body hangs (killed after 10 s); with only `×` → `x` it returns at once; with only
`<al-card>` → `<div>` it still hangs. `alder dump --tokens` on a module with `{…} × {…}` in a `<p>` lexes
`Text " × "` and returns, so the hang is past the lexer, in the `fill` path. A compiler bug, with its cause:
`qualifiers` in `internal/generate/fill.go` (line 179) applies `unicode.IsUpper(rune(ch))` to a single byte; the lead
byte of a 2-byte UTF-8 character (U+0080 and up, here 0xC3 of `×`, read as `Ã`) after a non-name byte enters the
name branch, `isNameByte` is false so `j == i`, and `i = j - 1` never advances. An infinite loop (~140 % CPU, flat
memory) on any such character, not `×` alone: `é` or `·` after a space trigger it too.

## 7. H3: the command-set gaps

Every stall was classifiable (64 of 64, none a misread). "Command missing" is 6 of 64 refusals (9 %), in three task
shapes, and those 6 sit in the four cells that did not reach the task. The command set is not a hard ceiling there:
T4 had a reachable workaround (§5.1), and its stalls became misuses. The rest are misuses of commands that exist; in
15 of the 49 (the lambda ×13, a `--list` page's `Model` ×2) neither the reference's command table nor the refusal's
sentence says what to do instead. For the `new form` precondition the refusal does say it ("add the markup
yourself") and for `--param` with `--show` it names the alternative (`--blank`), but the reference's table states
neither rule up front.

### No command (the input to the next milestone)

| Gap | Task, refusals | What the run wanted | Proposed |
|---|---|---|---|
| A message declared in `App` (a button on every page) | T4: #13, #44 | `alder new msg App ToggleTheme` | `alder new msg App <Name> [fields]`: the constructor, an arm with a hole in `App.update` (as `new auth` does for `Logout`), a test with a hole; a line under `alder new msg` in the reference. |
| A prop on `Layout.Page` (the body's class from the session) | T4: #14, #45 | `alder new decl Layout Shell "{ dark: Bool, … } -> Html a"` | `alder edit decl Layout.Page add-prop dark:Bool=False` (every `<Layout.Page …>` keeps checking with the default), or `new decl` accepting an upper-case component with a props record. |
| A form posted by a message that has its own parameters | T6: #22, #55 (and *Haiku t6*'s 0 / 9 hidden) | `new msg Orders.Show AddLine product:…`, then `new form Orders.Show NewLine quantity:… --msg AddLine` | `new form … --msg M` attaching to an existing `msg M { … }` that has no form (writing `msg M { … } form F`), or `new msg … --form F`. |
| Both `?afterLogin` holes, once the landing page exists | 9 of the 12 bare-start cells filled `?afterLogin` by hand, up to twice (not the two T3s, which had a `/` page or used `--to`; Sonnet T4 only attempted it, refused for unknown `Notes.Index`, #42); *Sonnet t6 r4*'s `fill Auth.Login - --hole ?afterLogin` (#57) | one command for the login's redirect and its test | `alder edit auth --to <Page>`: fills `?afterLogin` in `Auth.Login.update` and in its test. |

### The command exists; the reference or the refusal did not make it findable

| Misuse | Count | Proposed reference line or message |
|---|---|---|
| A lambda over the parameters a page's `load`/`view`/`update` (or `Layout.Page`) binds | 13, in 13 distinct cells (Haiku T2, T3, T5, T6, T9; Sonnet T10, T2, T3, T4, T5, T6, T7, T9) | Reference, under `--blank` / `fill --decl`: "`load params session = ?load`: the parameters are bound; send the body, not `\params session -> …`" (the table's "fill a function with a lambda" is about `new decl`). Message: when the result is a lambda whose parameters repeat the declaration's, "the parameters `params session` are bound already: send the body without `\params session ->`". |
| `fill <address>` where there is no hole (a generated test, an arm, `Model`) | 8 | The test refusal should carry the arm refusal's hint: "to replace the test's block, `alder fill --decl 'M."name"' -`" (the arm message has it, #24, #40, #56; the test and `Model` ones do not, #7, #25, #28, #30, #49). |
| A page or field used before the command that makes it; `--requires user` before `new auth` | 8 | Reference: "make the pages first (`--blank`), then fill: a body may name only what exists"; "`new auth` before any `--requires user` page" (the table's "Typical: tables, `new auth`, pages, fills" did not prevent #20, #31, #48). |
| A whole test block into a generated test's `?expected` hole | 3 | Message: when a `Task Bool` block goes into a hole that is not a `Task`, "to replace the whole test, use `--decl`". |
| A test address unquoted (`Mod."test name"` word-splits to `Mod.test name`) | 3 | Message: when `Mod.<words>` names a test, "quote the address: `'Mod.\"test name\"'`" (or accept it). |
| `new form`/`new msg` on a view that is one line | 3 | Reference: the precondition ("the view must be one element over several lines, e.g. `<Layout.Page …>` … `</Layout.Page>`"), beside "form and msg refuse while view is ?view". |
| `--param` with `--show`/`--list` | 2 | Reference: "`--show`, `--list` and `--from` make their own parameters; `--param` only with `--blank` or a plain page". |
| `fill --decl Model` on a `--list` page | 2 | Reference: "a `--list` page's `Model` and `load` go together: replace both, or start `--blank`". |
| An app type unqualified in a generator argument (`theme:Theme=Light`, `slug:Slug`) | 2 | Resolve as `fill` does (import the module; `Theme` → `Theme.Theme` when unique), or say "write `Theme.Theme`". |
| `alder test` mid-script with a hole left | 2 | Format page: "`alder test` refuses while a hole is left; run it last, after the fills". |

### The names check

`names_check` reported *Haiku t6* ok while its form and message differ from the Names block (§5.2). It should compare
a form's fields and a message's parameters, not only their presence.

## 8. Threats to validity

- **One sample per cell, two models, no Opus** (budget). A difference of one round or one hidden test between two
  cells is within noise; the condition-level differences (12 / 16 against 16 / 16; 4–6 rounds against 1–2) are not.
- **The worked example is T10's** (the first ten commands of the T10 drill, in `prompt-cli.md`). T10 was not the
  easiest `cli` task (Haiku 4 rounds, Sonnet 5).
- **Haiku's long outputs.** Haiku wrote 289,445 characters of answer over its 40 `cli` rounds (Sonnet 219,639 over
  42), Haiku's scripts up to 60 commands (*Haiku t10 r0*; the longest script of any cell, 62 commands, is *Sonnet t6 r0*), up to 1,115 s of model time per cell (*Haiku t6*). The longer a
  script, the more a stop discards.
- **The stop rule is strict by design.** A refused command ends the round, and a round cut short never passes even
  when its app does (e.g. *Haiku t3 r0*, *Sonnet t10 r0*, *Sonnet t7 r0* built and passed their tests while stopped).
  That is the condition; it measures "a script with no refusal", which is harder than "an app that works".
- **T4's hidden tests** assume E8's Names block, which the canonical T4 lacks (§2). **T2 and T3** have no hidden
  tests, so their `cli` passes rest on the tests alone, most of them generated; they were not hand-checked against
  the task text.
- **The names check checks existence only.** It confirms that a page's messages and forms exist, not their fields
  or parameters, which is why *Haiku t6* shows "ok" with hidden 0 / 9 (§5.2).
- **One hand-scorer.** The stall classes of §6 were assigned by one reader, without a second scoring.
- **The 600 s hang** (#23) cost *Haiku t6* a round (*r3*) to a tool bug, not the model.
- **The baseline was re-run**, not reused (deviation 2): this run's `files` cells and E9's differ in system prompt;
  they agree closely (§3).

## 9. What to do next (the loop's next iteration)

1. **Fix the `fill` hang** (§6, #23: `qualifiers` in `internal/generate/fill.go` reads one byte of a multi-byte
   character as a rune and never advances), with *Haiku t6 r3*'s body and bodies with `é` and `·` as regression
   tests. Ledgered as a compiler bug for the next fix batch; not fixed here.
2. **The four commands** of §7 (an `App` message, a prop on `Layout.Page`, a form on a message with parameters,
   `edit auth --to`), each with a drill line; then **the reference lines and messages** of §7's second table, the
   lambda message first (13 of 64 refusals).
3. **The names check** compares form fields and message parameters.
4. **T4's text** gets E8's Names block, or its hidden tests leave the comparison.
5. **Re-run E11** on the same matrix after 1–4 (with Opus when the budget allows) to see whether the misuses and the
   rounds come down; then the design's option B, one command per turn reading `--json`, which is what book 15's
   "reading their `--json` output" describes and what no script round can do.
