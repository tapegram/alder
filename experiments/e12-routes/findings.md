# E12 findings: AI usability after routes

*Run Wed 2026-11-11 22:04 EST to Thu 2026-11-12 ~00:43 EST, one night, pinned at `c9478ef7` (`RUN.md`).*
The question (design: `docs/superpowers/specs/2026-11-11-e12-routes-experiment-design.md`, issue #208): after #160
replaced pages and messages with routes, how well do Claude models write Alder from the route-era reference, compared
with E8, E9 and E11's file-mode runs of the same tasks?

## In plain words

- **Every program worked in the end.** All 78 runs (13 tasks; Haiku, Sonnet and Opus; two tries each) finished with
  their own tests passing, and every one of the 420 hidden acceptance tests the models never saw passed. No earlier
  experiment with mutation testing had a perfect hidden score (E9 193 / 195, E11 135 / 141).
- **The first attempt is rougher than before.** Only 40 of 78 first attempts were accepted by `alder check` (51 %),
  against 18 / 20 in E8, 23 / 25 in E9 and 15 / 24 in E11. On E11's own eight tasks: 23 / 48 now against 15 / 24
  then. The models needed more retries: 131 rounds for 78 runs, and three runs needed four rounds, more than E8 and
  E9 ever allowed.
- **One mistake explains much of that, and our own task texts invite it.** The task texts name routes by their full
  path (`Response.redirect (Notes.Index.Index {})`). The models copy that into the module that declares the route,
  where Alder requires the short name (`Index {}`). That one error is 48 of the 116 first-attempt errors, in 21 runs;
  in 13 runs it was the only error. The checker's message names the fix, and every run fixed it in one retry.
- **The four new route features were easy.** Live search (`input=`), infinite scroll (`intersect=`), elements that
  load and poll on their own (`load=`, `every=`) and the kinds of `Response` all worked: 24 / 24 runs, 84 / 84 hidden
  tests. The polling task passed on the first try in all six runs.
- **None of the new route errors (E0045, E0046, E0047) was ever hit.** One old word still trips models: a helper
  named `page` is parsed as the removed `page` declaration (3 runs, all on the feed task).
- **The tests the models write are about as strong as before** (mutation score mean 0.67; on E11's tasks 0.61
  against E11's 0.69). Opus writes the strongest tests (0.76), Haiku the weakest (0.59).
- **A second try often goes differently.** For the same model and task, the two tries took the same number of rounds
  in only 22 of 39 pairs. One run per cell, as E8–E11 mostly did, would have mis-read a model on many tasks.

## Matrix as run

Haiku, Sonnet and Opus (`claude:claude-haiku-5-5`, `claude:claude-sonnet-5-5`, `claude:claude-opus-5-5`, exact ids)
× 13 tasks (T2–T7 and T9–T11 ported to routes; T12–T15 new) × 2 samples (`s1`, `s2`) = **78 cells**, `files`
condition, `--build --test --mutate --hidden experiments/e12-routes/hidden --retries 5` (6 rounds); T2 on
`examples/todos`, T7 on `base/blog-nodrafts` with `--migrate`. Binary sha256 `74c6974f…` (`RUN.md`), `claude
--version` 2.1.309, prompt 289,942 characters / 72,485 tokens (len/4) (`sizes.md`). One invocation of
`run-matrix.sh`, exit 0, `STOP_AT` not reached. The gate (`verify.sh`) passed before the run; no hidden test changed
after it.

**What it is compared with.** Every old figure below is recomputed from those cells' `summary.json`.

| | Cells used here | Prompt | Budget | Models |
|---|---|---|---|---|
| E8 (`e8-hidden/with/runs/`) | T4–T7, condition `with` (the tests paragraph, as E12's texts have); Haiku and Sonnet × 2, Opus × 1: 20 cells. T8 (not ported) and `without` left out | `full` (chapters 3–10 + example), 223,634 characters | 3 rounds | aliases `haiku`, `sonnet`, `opus` |
| E9 (`e9-feedback/baseline/runs/`) | T5–T7, T9, T10, condition `baseline` (no `--feedback`); Haiku and Sonnet × 2, Opus × 1: 25 cells | `full`, 250,290 characters | 3 rounds | aliases |
| E11 `files` (`e11-cli/baseline/runs/`) | T2–T7, T9, T10; Haiku and Sonnet from run 1 (`s1`), Opus from run 2 (`r2`): 24 cells | `reference`, 195,967 characters | 6 rounds | aliases |

The E8–E11 summaries record the alias (`"model": "haiku"`), not the model version it resolved to, so whether those
runs used the same model versions as E12 is not recorded anywhere. T11 has no earlier file-mode run and T12–T15 are
new; they are reported on their own. E8 ran without `--mutate`, so it has no mutation score. No earlier experiment
recorded `names` in file mode.

Every number below comes from a cell's `summary.json` or `round<N>/check.json` (E12's `scores.md` is made from the
same summaries by `report_loop.py`), from `RUN.md` or `sizes.md`, or from an older experiment's `sizes.md` or
`findings.md`. A cell is cited as *model task sample* (e.g. *Haiku t6 s1*), a round as `rN`.

## 1. Headline

| | E8 `with` (T4–T7) | E9 `baseline` | E11 `files` | **E12, all 13 tasks** | E12 on E11's 8 tasks |
|---|---|---|---|---|---|
| Cells | 20 | 25 | 24 | **78** | 48 |
| Cells passing their own tests | 20 / 20 | 25 / 25 | 24 / 24 | **78 / 78** | 48 / 48 |
| Cells passing in round 0 | 16 / 20 | 21 / 25 | 15 / 24 | **38 / 78** | 21 / 48 |
| Rounds used by the passing cells (range) | 24 (1–2) | 29 (1–2) | 34 (1–3) | **131 (1–4)** | 83 (1–4) |
| Round-0 clean checks | 18 / 20 | 23 / 25 | 15 / 24 | **40 / 78** | 23 / 48 |
| Round-0 check errors | 3 | 3 | 19 | **116** | 84 |
| Hidden tests passed | 160 / 160 | 193 / 195 | 135 / 141 | **420 / 420** | 282 / 282 |
| Cells passing every hidden test | 20 / 20 | 23 / 25 | 16 / 18 | **66 / 66** | 36 / 36 |
| Mutation score, mean of cells (killed / ran) | — | 0.57 (624 / 1,099) | 0.69 (1,025 / 1,338) | **0.67 (2,784 / 3,859)** | 0.61 (2,437 / 3,325) |
| Names check | not recorded | not recorded | not recorded | **66 / 66 ok** (T2 and T3 have no Names block) | 36 / 36 ok |

E12's rounds per cell: 38 cells passed in 1 round, 30 in 2, 7 in 3, 3 in 4 (*Haiku t5 s1*, *Sonnet t9 s1*, *Sonnet
t13 s2*). Under E8 and E9's 3-round budget those three would have failed; 75 / 78 passed within 3 rounds.

**Per model:**

| Model | | E8 (T4–T7) | E9 | E11 `files` | E12 all | E12 on E11's tasks |
|---|---|---|---|---|---|---|
| Haiku | cells passing in round 0 | 6 / 8 | 8 / 10 | 2 / 8 | 7 / 26 | 3 / 16 |
| | round-0 clean checks | 7 / 8 | 9 / 10 | 2 / 8 | 8 / 26 | 4 / 16 |
| | rounds used | 10 | 12 | 14 | 52 | 33 |
| | round-0 errors | 1 | 2 | 12 | 65 | 51 |
| | hidden | 64 / 64 | 76 / 78 | 44 / 47 | 140 / 140 | 94 / 94 |
| | mutation mean | — | 0.53 | 0.68 | 0.59 | 0.53 |
| Sonnet | cells passing in round 0 | 7 / 8 | 8 / 10 | 7 / 8 | 13 / 26 | 9 / 16 |
| | round-0 clean checks | 8 / 8 | 9 / 10 | 7 / 8 | 13 / 26 | 9 / 16 |
| | rounds used | 9 | 12 | 9 | 43 | 25 |
| | round-0 errors | 0 | 1 | 2 | 31 | 19 |
| | hidden | 64 / 64 | 78 / 78 | 44 / 47 | 140 / 140 | 94 / 94 |
| | mutation mean | — | 0.52 | 0.60 | 0.64 | 0.58 |
| Opus | cells passing in round 0 | 3 / 4 | 5 / 5 | 6 / 8 | 18 / 26 | 9 / 16 |
| | round-0 clean checks | 3 / 4 | 5 / 5 | 6 / 8 | 19 / 26 | 10 / 16 |
| | rounds used | 5 | 5 | 11 | 36 | 25 |
| | round-0 errors | 2 | 0 | 5 | 20 | 14 |
| | hidden | 32 / 32 | 39 / 39 | 47 / 47 | 140 / 140 | 94 / 94 |
| | mutation mean | — | 0.74 | 0.80 | 0.76 | 0.73 |

Reading it:

- **End result: as good or better everywhere.** No hidden failure in any of the 66 cells with hidden tests. E9's two
  (Haiku T10: `Rerender` where an ambiguous Names sentence asked for `Redirect`, `experiments/README.md`) and E11's
  six (T4: details that E8's Names block fixed and the canonical T4 of the time did not, `e11-cli/findings.md` §2)
  were task-text gaps; the texts were rewritten in the port (`tasks/README.md`, `RUN.md` "Rulings") and neither
  recurred.
- **First attempt: worse, mostly from one pattern (§3).** On E11's tasks, round-0 clean checks fall from 15 / 24
  (63 %) to 23 / 48 (48 %), and round-0 errors per cell rise from 0.8 to 1.75. Haiku is most affected (65 of the 116
  errors), Opus least (19 / 26 clean).
- **Mutation scores move little and not in one direction.** Haiku 0.59 (E9 0.53, E11 0.68), Sonnet 0.64 (0.52,
  0.60), Opus 0.76 (0.74, 0.80). The per-task order holds: T2 highest and T6 lowest of the ported tasks in E11 and
  E12, T6 lowest in E9 (§2). The mutants are generated from each program by each run's binary, so a score compares
  the test suites of different programs; read it as a trend.

## 2. Per task

"Rounds" is the total over the task's cells (range in brackets); "r0 clean" the round-0 checks with no error; "Mut"
the mean of the cells' scores.

| Task | Exp | Cells | Pass | Rounds | r0 clean | r0 errors | Hidden | All hidden | Mut (killed / ran) |
|---|---|---|---|---|---|---|---|---|---|
| T2 newpage | E11 | 3 | 3 | 5 (1–2) | 1 | 5 | — | — | 0.98 (509 / 517) |
| | **E12** | 6 | 6 | 8 (1–2) | 4 | 4 | — | — | 0.97 (1,368 / 1,408) |
| T3 newapp | E11 | 3 | 3 | 5 (1–2) | 1 | 7 | — | — | 0.64 (53 / 82) |
| | **E12** | 6 | 6 | 10 (1–3) | 3 | 14 | — | — | 0.55 (122 / 223) |
| T4 appmsg | E8 | 5 | 5 | 6 (1–2) | 5 | 0 | 40 / 40 | 5 / 5 | — |
| | E11 | 3 | 3 | 6 (1–3) | 1 | 3 | 18 / 24 | 1 / 3 | 0.79 (53 / 67) |
| | **E12** | 6 | 6 | 14 (2–3) | **0** | 17 | 48 / 48 | 6 / 6 | 0.60 (104 / 171) |
| T5 ownership | E8 | 5 | 5 | 5 (1–1) | 5 | 0 | 40 / 40 | 5 / 5 | — |
| | E9 | 5 | 5 | 5 (1–1) | 5 | 0 | 40 / 40 | 5 / 5 | 0.59 (53 / 90) |
| | E11 | 3 | 3 | 4 (1–2) | 2 | 1 | 24 / 24 | 3 / 3 | 0.76 (43 / 56) |
| | **E12** | 6 | 6 | 12 (1–4) | 2 | 12 | 48 / 48 | 6 / 6 | 0.57 (70 / 120) |
| T6 orders | E8 | 5 | 5 | 5 (1–1) | 5 | 0 | 45 / 45 | 5 / 5 | — |
| | E9 | 5 | 5 | 6 (1–2) | 4 | 1 | 45 / 45 | 5 / 5 | 0.38 (96 / 254) |
| | E11 | 3 | 3 | 3 (1–1) | 3 | 0 | 27 / 27 | 3 / 3 | 0.44 (69 / 153) |
| | **E12** | 6 | 6 | 10 (1–2) | 2 | 24 | 54 / 54 | 6 / 6 | 0.35 (126 / 359) |
| T7 drafts | E8 | 5 | 5 | 8 (1–2) | 3 | 3 | 35 / 35 | 5 / 5 | — |
| | E9 | 5 | 5 | 6 (1–2) | 5 | 0 | 35 / 35 | 5 / 5 | 0.70 (190 / 273) |
| | E11 | 3 | 3 | 3 (1–1) | 3 | 0 | 21 / 21 | 3 / 3 | 0.68 (113 / 167) |
| | **E12** | 6 | 6 | 10 (1–2) | 4 | 4 | 42 / 42 | 6 / 6 | 0.64 (244 / 382) |
| T9 bookings | E9 | 5 | 5 | 5 (1–1) | 5 | 0 | 35 / 35 | 5 / 5 | 0.61 (121 / 198) |
| | E11 | 3 | 3 | 4 (1–2) | 2 | 2 | 21 / 21 | 3 / 3 | 0.65 (78 / 120) |
| | **E12** | 6 | 6 | 9 (1–4) | 5 | 2 | 42 / 42 | 6 / 6 | 0.61 (170 / 277) |
| T10 teams | E9 | 5 | 5 | 7 (1–2) | 4 | 2 | 38 / 40 | 3 / 5 | 0.57 (164 / 284) |
| | E11 | 3 | 3 | 4 (1–2) | 2 | 1 | 24 / 24 | 3 / 3 | 0.60 (107 / 176) |
| | **E12** | 6 | 6 | 10 (1–3) | 3 | 7 | 48 / 48 | 6 / 6 | 0.61 (233 / 385) |
| T11 workspace | **E12** (no history) | 6 | 6 | 10 (1–3) | 3 | 8 | 54 / 54 | 6 / 6 | 0.53 (152 / 285) |
| T12 search | **E12** (new) | 6 | 6 | 11 (1–3) | 2 | 8 | 24 / 24 | 6 / 6 | 0.62 (53 / 86) |
| T13 feed | **E12** (new) | 6 | 6 | 14 (1–4) | 1 | 14 | 18 / 18 | 6 / 6 | 0.85 (31 / 37) |
| T14 board | **E12** (new) | 6 | 6 | 6 (1–1) | 6 | 0 | 18 / 18 | 6 / 6 | 0.86 (36 / 42) |
| T15 pantry | **E12** (new) | 6 | 6 | 7 (1–2) | 5 | 2 | 24 / 24 | 6 / 6 | 0.89 (75 / 84) |

The per-cell table (errors per round, own tests per round, hidden, mutants, names, codes) is `scores.md`.

- **T4 is the hardest task to get right first.** No E12 T4 cell checked clean in round 0 (0 / 6; E8 5 / 5). Four of
  the six met the own-name pattern (§3). Both Opus cells hit the
  line-continuation rule (E0001) in r0 and an unknown name (E0002) in r1: a `Theme` constructor (`Light`, `Dark`)
  used without `import Theme`, and in *Opus t4 s1* also the own-name pattern. They took 3 rounds each.
- **T6 has the most round-0 errors** (24), from four unrelated causes (§3, §6). Its mutation score is the lowest of
  any task in every experiment that measured it (0.35–0.44).
- **T2 did better than in E11** (4 / 6 clean against 1 / 3; both E12 failures Haiku's). Its base `examples/todos`
  checks under the pinned binary (the preflight checks it).

**Own tests that failed after a clean build**, the only rounds that failed for anything but the check: 5 rounds.
*Haiku t7 s1 r0* and *Opus t7 s1 r0* (1 / 30 each, the same test: "publishing a draft from the dashboard puts it on
the front page"), *Haiku t12 s2 r1* (2 / 5), *Haiku t13 s2 r1* (1 / 3, "scrolling twice through 25 entries…"),
*Haiku t5 s1 r2* (1 / 6, "deleting a missing note is not found"). Each passed in the next round.

**T7's migration dropped no data** in any cell: all six final rounds wrote `0002_change_post.sql` with one step
("rebuild table Post: Post.published: new column filled with 0") and no `DROPS DATA` line (E8: 0 / 10 as well).

**Material notes from the gate and the run** (candidates for the issue list, §10; nothing filed here):

- **The drill tests miss a logged-out `requires` on write routes.** The T9 and T10 references, replayed from the
  drills, first scored 0.915 and 0.939 under `--mutate` because no drill test sends a write from a page held while
  the user logged out; four tests were added at the gate (`RUN.md`, "T9–T11", "The drill references are not a fresh
  replay"). The drills' own tests were not changed.
- **T7's text mentions `Test.fake Password`** ("installing the fake with `Test.fake Password` is fine",
  `tasks/t7-drafts.md`), but the blog base declares no fake `Password`. No T7 cell was affected (6 / 6, 7 / 7
  hidden).

## 3. Round-0 error codes, old against new

Errors per code over every cell's round-0 check (`summary.json`, `rounds[0].codes`).

| Code | What the checker says | E8 (20) | E9 (25) | E11 `files` (24) | E12 (78) | E12 on E11's tasks (48) | E12 new tasks (24) |
|---|---|---|---|---|---|---|---|
| E0001 | parse error | — | 3 | 4 | 24 | 14 | 10 |
| E0002 | unknown name | 1 | — | 1 | **56** | 43 | 9 |
| E0003 | type mismatch | 2 | — | — | 7 | 3 | — |
| E0011 | brand constructor outside its module or a `test` | — | — | — | 5 | 4 | 1 |
| E0025 | `Db.mine` and kin on a table without `@owner` | — | — | — | 3 | 3 | — |
| E0027 | `requires … role` without a session `role` | — | — | — | 4 | 4 | — |
| E0028 | custom element misuse (`<Errors field>` beside `<al-input field>`) | — | — | 8 | 8 | 7 | 1 |
| E0201 | module not imported | — | — | 6 | 9 | 6 | 3 |
| **Total** (cells with any) | | 3 (2) | 3 (2) | 19 (9) | **116 (38)** | 84 (25) | 24 (10) |

T11 accounts for E12's remaining 8 (E0002 ×4, E0003 ×4). Over all rounds E12 has 136 errors (every round's
`check.json`): the 20 after round 0 fall in 10 rounds.

**E0002: a module's own route written with its full path (48 of the 56).** Every one reads "inside `Notes.Index`,
its own names are written unqualified: `Index`" (module and name vary). The models wrote the route as the task text
writes it: every Names block that names a redirect or render target writes it with its full path
(`Response.redirect (Notes.Index.Index {})`, `Response.redirect (Orders.Show.Show { id: … })`,
`Response.render (Pantry.Index.Row …)`), and the models copy that into the module that declares the route,
where the checker requires `Index {}`. It occurs in 21 of the 78 cells (Haiku 12, Sonnet 7, Opus 2) across 9
tasks, in no T14 cell (T14's text has no redirect), and in 13 cells it was the only round-0 error (*Haiku t4 s1*,
*t10 s1*, *t10 s2*, *t11 s1*, *t11 s2*; *Sonnet t4 s2*, *t6 s1*, *t7 s1*, *t12 s1*, *t12 s2*, *t15 s2*; *Opus t5
s1*, *t5 s2*). Every cell fixed it in the next round. The page-era texts could not provoke it: they named a page
module (`Redirect (Notes.Index {})`), and a module never names itself. It is the main reason round-0 clean checks
fell (§1). Whether the fix is the texts (the short name in the Names block, or a sentence that both are meant) or
the checker (accept a module's own full path) is for the issue list (§10). The other 8 E0002 are a table's
constructor without its import (`unknown constructor User`; `` `Author` is a constructor of `Table.User.Role`;
import `Table.User` ``), `Light`/`Dark` without `import Theme`, and one `unknown Profile.Show.Show` (*Haiku t2
s1*).

**E0001: 24 errors in 8 cells, five causes**, read from the messages and the lines they point at:

- **`page` is still read as the removed declaration** (6 errors, 3 cells, all T13: *Opus t13 s1*, *Sonnet t13 s1*,
  *s2*). Each wrote a helper `page : Int -> Query …` and got "expected string literal, got `:`" and "in page
  parameters: expected `,` or closing brace": the parser takes `page` for the old `page "…"` syntax, so the model is
  told about page parameters, not that `page` is gone (no E0046). Each renamed it in r1.
- A parenthesised call continued over several lines (8 errors, 3 cells: *Opus t4 s1*, *t4 s2*, *t13 s1*): "expected
  `)`, got `{`" and "expected `}`, got `)`" on a `Test.run (` … `)` or a `List.map (\n -> …)` spread over lines.
- A `,` after a block's last expression (3 errors, *Haiku t7 s2*), the rule all of E9's and E11's E0001 were.
- A `match` with a trailing `,` inside a `let` (5 errors from one site, *Opus t6 s2*).
- A `{- … -}` comment inside a route declaration (2 errors, *Sonnet t6 s2*: "in route parameters: expected field
  name, got `-`").

**The rest.** E0025 and E0027 (T5, T6, Haiku only): `Db.mine`/`Db.insertMine` on a table with no `@owner` column,
and `requires user role …` with no `role` in the session (*Haiku t6 s1*: 12 round-0 errors, the most of any cell).
E0011: brand constructors (`Text.NonEmpty`, `Table.User.UserId`) outside a `test` (T3, T9, T13). E0028 is E11's
`<Errors field>` beside `<al-input field>` (T2, T3, T13). E0201 is mostly `Layout` not imported in `App` (5 cells).
E0003 includes four `Test.login` calls whose session record lacks T11's `org` (*Sonnet t11 s2*).

## 4. The four new tasks

No history; each ran 6 cells (3 models × 2 samples). All four: 6 / 6 passing, every hidden test passed, names `ok`.

### T12 search (`input=`)

11 rounds; round-0 clean 2 / 6 (both Opus). Every final app declares `route Results = GET "/contacts/results"
{ q: Text = "" }` beside `Find` and puts `input={Results { q: model.q }}` on the query field, chapter 6's pattern.
Round-0 errors: the own-name pattern on `Find` (Haiku ×2, Sonnet ×2) and `Layout` not imported (Haiku ×2). *Haiku
t12 s2* built in r1 with 2 / 5 own tests failing and passed in r2. Mutation 0.62, the lowest of the new tasks.

### T13 feed (`intersect=`)

14 rounds, the most of the new tasks; round-0 clean 1 / 6 (*Opus t13 s2*). Three of the five failing cells were the
`page` keyword (§3). Every final app's sentinel is `intersect={More { after: … }}` (`next` ×3, `shown`,
`model.after + 10`, `model.after + pageSize`). Whether each asks for the right offset is not checked here: the
hidden tests fetch `More { after: 10 }` and `{ after: 20 }` themselves, a blind spot recorded at the gate (`RUN.md`,
"T13"). The cursor is fixed by the Names block (`More { after: Int }`, a ruling), so E12 does not observe a model's
choice of cursor. Mutation 0.85 (31 / 37).

### T14 board (`load=`, `every=`)

6 rounds: all six cells passed in round 0, the only task with no round-0 error. Every app writes `load={Open {}}
every="10s"` twice (the badge and `Open`'s view) and `load={Recent {}}` once, as the text asks, so the hidden
polling step passes. The ticket status is `Pending | Closed`, not the planned `Open | Closed`: `type Status = Open |
Closed` beside `route Open` is refused by E0044 ("route `Open` is named like the constructor `Open` … rename the
route"), so the constructor was renamed before the run (`RUN.md`, "T14"); the clash between a route's and a
constructor's name is a candidate issue. Mutation 0.86 in all six cells (6 / 7 each).

### T15 pantry (`Response` and a form)

7 rounds; round-0 clean 5 / 6 (*Sonnet t15 s2*: the own-name pattern on `Index` and `Row`). Every final app uses
every answer the text names: `Response.invalid` for a taken name, `Response.redirect` after a rename,
`Response.render` of `Row` after `Bump`, `Response.refresh` with `Response.toast` after `Remove`. Mutation 0.89, the
highest of the new tasks (Opus 14 / 14 in both cells). The methods the models chose for the write routes are design
signals (§9).

## 5. E0045–E0047 as the models met them

**Not at all.** No `check.json` of any round of the 78 cells has E0045 (a route requested in a way its method
refuses), E0046 (page-era syntax) or E0047 (a route trigger attribute misused), nor E0044; and every hidden module
compiled against its program, so no hidden check met them either. The run's 136 errors are the eight codes of §3.
The nearest thing to E0046 is §3's `page` helper, which the parser reports as E0001 before the checker can say the
syntax was removed.

## 6. The T6 correction, and two deliberate wording additions

**T6's old text had a cut that routes fix.** E9's T6 named `msg AddLine { product } form NewLine`, a form posted by
a message with parameters; E11's `cli` condition met it as a missing command (`e11-cli/findings.md`, H3: "a form
posted by a message with parameters ×2", in the T6 cells). In routes it is an ordinary write route with a form. A
write route runs no `load` and does not know the page it came from (book 6), so the order rides in the route too:
the ported text names `route AddLine { id: Order.OrderId, product: Product.ProductId } form NewLine`, `route Pay
{ id }` and `route Cancel { id }` (a ruling in `RUN.md`; the spec's `{ product }` alone could not work). Result:
6 / 6, hidden 54 / 54, names `ok` in every cell, so every model wrote `AddLine` with its form and both parameters.
Round-0 clean 2 / 6, and none of the four failures was about `AddLine` (§3: the own-name pattern, `@owner` and
`role` errors, a `match` with a `,`, a comment in a route line).

**Deliberate wording additions** (rulings in `RUN.md`): T4 and T6 now say that another user's note or order is a
404 for its forms and buttons too. Page-era `load` gave that for free, since it ran before every message; a write
route runs no `load`, so the text says it. No hidden test depends on it (E9's never checked a write's 404), so it
changes no comparison above.

## 7. The reference changed in three ways

E11 sent `e11-cli/prompt-reference.md`, 195,967 characters / 48,991 tokens; E12 sent 289,942 / 72,485 (len/4),
under the 73,500 cap (`RUN.md`, "The reference prompt"). The differences:

1. **The route-era chapters** (#160): chapters 3–10, as `--reference` cuts them, now teach route modules, GET and
   write routes, `Response`, `input=`, `load=`, `intersect=`, `every=` and chapter 10's `Answer` names; pages and
   messages are gone.
2. **The COMMANDS table** is rewritten for the route-era CLI (`alder new route`, `new action`, `edit route`, `remove
   route`). File-mode cells run no command, so this part of the prompt was not exercised. Noted during the run, not
   measured: per-command `--help` prints only the top-level usage.
3. **Three subsections of chapter 10's "12. Properties" are dropped** to pay for the table: "Reading a failure",
   "`--fuzz`, `--seed` and saved failures" and "Health checks". No task asks for a property test.

The prompt grew by 93,975 characters over E11's. That growth and the language change cannot be told apart here: the
E12 comparison is new book and new language against old book and old language, per task.

## 8. The two samples, per cell

For each model and task, `s1` / `s2`: rounds used · round 0 clean (`c`) or not (`x`) · mutation score. Both samples
passed in every pair, and passed every hidden test.

| Task | Haiku | Sonnet | Opus |
|---|---|---|---|
| T2 | 2 / 2 · x / x · 0.97 / 0.97 | 1 / 1 · c / c · 0.97 / 0.97 | 1 / 1 · c / c · 0.97 / 0.97 |
| T3 | 3 / 2 · x / x · 0.44 / 0.39 | 2 / 1 · x / c · 0.59 / 0.62 | 1 / 1 · c / c · 0.65 / 0.62 |
| T4 | 2 / 2 · x / x · 0.19 / 0.68 | 2 / 2 · x / x · 0.43 / 0.67 | 3 / 3 · x / x · 0.83 / 0.81 |
| T5 | 4 / 2 · x / x · 0.39 / 0.39 | 1 / 1 · c / c · 0.45 / 0.45 | 2 / 2 · x / x · 0.81 / 0.91 |
| T6 | 2 / 1 · x / c · 0.34 / 0.29 | 2 / 2 · x / x · 0.26 / 0.17 | 1 / 2 · c / x · 0.40 / 0.66 |
| T7 | 2 / 2 · c / x · 0.63 / 0.65 | 2 / 1 · x / c · 0.62 / 0.61 | 2 / 1 · c / c · 0.66 / 0.66 |
| T9 | 1 / 1 · c / c · 0.53 / 0.56 | 4 / 1 · x / c · 0.64 / 0.60 | 1 / 1 · c / c · 0.69 / 0.66 |
| T10 | 2 / 3 · x / x · 0.52 / 0.57 | 1 / 1 · c / c · 0.62 / 0.61 | 1 / 2 · c / x · 0.61 / 0.71 |
| T11 | 2 / 3 · x / x · 0.21 / 0.48 | 1 / 2 · c / x · 0.56 / 0.56 | 1 / 1 · c / c · 0.65 / 0.73 |
| T12 | 2 / 3 · x / x · 0.47 / 0.46 | 2 / 2 · x / x · 0.64 / 0.57 | 1 / 1 · c / c · 0.79 / 0.79 |
| T13 | 2 / 3 · x / x · 0.86 / 1.00 | 2 / 4 · x / x · 0.83 / 0.83 | 2 / 1 · x / c · 0.83 / 0.71 |
| T14 | 1 / 1 · c / c · 0.86 / 0.86 | 1 / 1 · c / c · 0.86 / 0.86 | 1 / 1 · c / c · 0.86 / 0.86 |
| T15 | 1 / 1 · c / c · 0.86 / 0.80 | 1 / 2 · c / x · 0.85 / 0.86 | 1 / 1 · c / c · 1.00 / 1.00 |

- **Pass and hidden agree in all 39 pairs.**
- **Rounds agree in 22 of 39 pairs; round 0 clean or not agrees in 29.** The largest gap is *Sonnet t9*: 4 rounds
  against 1 (s1 met E0011 twice, with an E0003 in between).
- **Mutation scores are within 0.05 in 25 of 39 pairs** (mean gap 0.06) and more than 0.2 apart in 4: *Haiku t4*
  0.19 / 0.68, *Sonnet t4* 0.43 / 0.67, *Opus t6* 0.40 / 0.66, *Haiku t11* 0.21 / 0.48. With one sample per cell
  (E11, and Opus in E8 and E9), Haiku's T4 tests would have scored 0.19 or 0.68 depending on the draw.

## 9. "Reasonable but not Alder" signals

**In plain words.** Besides errors, we looked for habits from ordinary web development that Alder does not
share. Examples are sending a delete as a `POST`, or putting a live-search trigger on a design-system element.
Such a choice can pass the checker and still be the wrong one for Alder, so these are recorded on their own
and never counted as errors. We read every round of every run (131 rounds, 934 files). Only one habit
appeared: a change written as `POST` to a path ending in a verb (`/orders/{id}/pay`, `/orgs/{org}/open`). The
book asks for the changed part to be named instead (`PUT /orders/{id}/paid`). That happened in 5 routes in 3
of 78 runs. The checker accepted it, as the book says it will, and it stayed in each final program. None of
the other habits the design listed occurred, not once. Every route that deletes a row is a `DELETE`, and
`render` is used only where the task asks for it. No `<al-*>` element carries a trigger attribute, every
`Response` comes from the book's builders, and the one live search reads a query key, not a path segment.
The details, with the search behind each zero, are in `signals.md`.

| Signal | Occurrences (cells · rounds) | Checker's answer |
|---|---|---|
| A change as `POST` to a verb path (`Pay`, `Cancel`, `Toggle` in *Haiku t6 s1*; `Open` in *Haiku t11 s1*, *Sonnet t11 s1*) | 3 cells · 9 route-rounds | accepted, no diagnostic; names `ok` |
| A `DELETE` written as `POST` | 0 (all 55 deleting route declarations are `DELETE`) | — |
| `render` where the rule says `refresh` | 0 (all 7 `render`s are T15's `Bump`, as asked) | — |
| A trigger attribute on `<al-*>` | 0 (of 1,407 `<al-*>` tags) | — |
| A `Response` built by hand | 0 | — |
| A path parameter in an `input=` route | 0 (of 11 `input=`) | — |

No signal coincided with a check error. The only near case is *Haiku t6 s1* r0. There the `POST
"/products/{id}/toggle"` line also carried E0027, which is about its `requires … role`, not its method; that
error is counted in §3. One method is not counted as a signal. 4 of 6 T4 cells wrote `SwitchTheme` as
`POST /theme`, and the prompt tells them to: chapter 6's "What the patterns cost" gives "login, logout,
theme" as "a `POST`". But the same chapter's `Logout` is `DELETE "/session"`, and its theme example, which is
not in the reference, is `PUT "/theme"`. That contradiction in the book is a candidate for §10.

## 10. Issues

Filed on `tapegram/alder` on 2026-11-12, each referencing #208:

- [#221](https://github.com/tapegram/alder/issues/221) Book: ch. 6 contradicts itself on the theme method. The table says `POST`, the example `PUT /theme`; 4 of 6 T4 cells wrote `POST /theme` (§9).
- [#215](https://github.com/tapegram/alder/issues/215) (commented, not new): a helper named `page` is parsed as the removed declaration and gives E0001. 3 T13 cells (§3). The open parser-cleanup issue already covers the fix.
- [#222](https://github.com/tapegram/alder/issues/222) Diagnostics: a route named like a constructor is E0044; T14's `Open | Closed` had to be renamed (§4). Whether to improve the message or allow the clash is open.
- [#223](https://github.com/tapegram/alder/issues/223) Book: "a route value keeps its full path" never says a module writes its own routes unqualified. 48 of the 56 round-0 E0002 (§3).
- [#224](https://github.com/tapegram/alder/issues/224) E12's own inputs (a ruling, not an Alder fix): full-path redirects in the Names blocks, T7's `Test.fake Password` with no fake in the base, drill tests missing a logged-out `requires` on write routes (§2).

**Not filed:** per-command `--help` printing only the top-level usage. It does not reproduce on current main (39f84c38): `alder new --help`, `alder migrate --help`, `alder edit --help` and `alder check --help` each print their own usage line(s), different from `alder --help`. The remark in §7 was about the pinned run binary or about the help being one line without examples; if wanted, a richer per-command help is a new request, not a bug.
