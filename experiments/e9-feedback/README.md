# E9: survivors as feedback

E8's hidden tests showed whether a program does what its task asked. They
did not show whether the model's own tests would have noticed if it did not.
E9 measures that with `alder test --mutate` (book 10, "Mutation testing"):
the share of small, deliberate bugs (mutants) that the model's tests kill.
It then feeds the surviving mutants back to the model and measures whether
its tests get better.

## Hypotheses

- **H1, the score.** The mutation score of the tests Claude Haiku, Sonnet
  and Opus write, by model and task, and the kinds of mutants that survive
  (authorization, data, flow, logic, schema).
- **H2, feedback.** After the first round whose program checks, builds and
  passes its own tests, the surviving mutants are fed back (up to two
  rounds, app code frozen). Does the score rise, and are the new tests real
  tests or tautologies?
- **H3, behaviour.** E8's hidden acceptance tests stay the behaviour check.
  A feedback round adds tests only, so it cannot change a hidden result: the
  hidden tests run once, after the final round.

## Design

- **Five tasks**, each with a **Names** block (E8, "Names block"): T5
  ownership, T6 orders, T7 drafts on the blog, and two new, harder ones,
  T9 bookings and T10 teams. Each task has one text, `tasks/<task>.md`, in
  E8's `with` form: the body, the Names block, the tests paragraph and the
  output paragraph. Both conditions get the same text.
- **Two conditions.**
  - `baseline`: the task text, `--build --test --mutate`. The score is the
    final round's.
  - `feedback`: the same, plus `--feedback 2`. After the first round that
    passes with survivors, up to two more rounds with the survivor prompt
    below, app code frozen.
- **Two solutions per task**, `reference/` and `reference-alt/`, with the
  same Names but different markup and structure (E8, "Two solutions per
  task"). Every hidden test passes on both, and each scores at least 0.9
  under `--mutate` with its own tests.

### Global constraints (from the plan, verbatim)

- Feedback rounds may change only `test` declarations: the harness compares the IR of the round's program with tests removed (`alder dump --ir`, `(test …)` nodes stripped, hashed) against the passing round's; a mismatch rejects the round ("app code changed in a feedback round: only tests may change") and the retry says so; at most two feedback rounds, so a cell has at most `retries + 2` model calls after the first pass.
- The survivor retry prompt is exactly (binding): "Your tests pass, but S of N mutants survived (score X.XX). Each line below is a change to your app that no test noticed. Add a test, in the named module, that fails on it. Do not edit app code to remove a mutant. Only if no input can tell a survivor apart, say why in one comment." followed by at most 10 survivor lines in `alder test --mutate`'s text format, grouped by module, then the previous answer as usual.
- Hidden tests never appear in any prompt. The material gate: every hidden test passes on the reference and the alt solution; each reference scores ≥ 0.9 under `--mutate` (T7 on the blog base likewise); `verify.sh` prints both.
- The binary is a clean export of the E9 commit (after the codegen fix in Task 1), recorded with its sha256; prompts regenerated and committed with `sizes.md`.

## Matrix and run protocol

Claude only (the plan's option A). No run uses `--adapters`. Every run
uses `--build --test --mutate --retries 2 --hidden experiments/e9-feedback/hidden`,
and the `feedback` runs add `--feedback 2`.

| Model | Tasks | Conditions | Samples | Runs |
|---|---|---|---|---|
| Haiku | T5, T6, T7, T9, T10 | baseline, feedback | 2 (`s1`, `s2`) | 20 |
| Sonnet | T5, T6, T7, T9, T10 | baseline, feedback | 2 | 20 |
| Opus | T5, T6, T7, T9, T10 | baseline, feedback | 1 | 10 |

That is 40 + 10 = 50 cells.

Additional flags by task:

| Task | Flags |
|---|---|
| T5, T6, T9, T10 | no base. The `Password` capability's Go is not the model's; test builds run unwired. |
| T7 | `--base experiments/e8-hidden/base/blog-nodrafts --migrate` |

The protocol is E8's:

1. Run the Claude cells one model at a time: Haiku, Sonnet `s1`/`s2`, then
   Opus.
2. Make no edits to the material, the harness or the compiler mid-run.
3. A `harness_error` or `compiler_bug` gets a repro under `repros/`, and the
   run continues.

## Scoring

- **Own.** `outcome` and `final_pass`, as in E7 (check, build, the model's
  own tests).
- **Score.** The `mutation` record of the final accepted round: killed / all
  and the exact score. For `feedback`, `score_before` (the first passing
  round) → `score_after` (the last accepted round), the rounds used, and any
  `rejected_reason`. A round that changes app code is rejected. The cell's
  score is then the last accepted round's.
- **Hidden.** `hidden_pass` is `k/n` over `hidden/<task>/Hidden.alder`, run
  once after the final round, with E8's rules (a module that does not
  compile fails every test; `—` when the final round did not build).
- **Fed-back tests**, hand-scored in `survivors.md` as *real*, *tautology*
  (it passes on the mutant too, e.g. `test "x" = True`) or *wrong-module*.

## The material gate

`verify.sh` (below) must be green before any run:

- every reference and alt solution passes `alder check`,
  `alder build --emit`, its own `alder test` and every hidden test;
- each scores at least 0.9 under `alder test --mutate` with its own tests
  (the exact score, not the two printed decimals);
- every survivor is explained in that solution's `SURVIVORS.md`, one line
  each, as an equivalent mutant: a change no input can tell from the app.

## What changed since E8

- **T5 and T7.** E8's `with` texts, byte for byte, and E8's hidden modules.
- **T6, the two defects of E8's findings (§7).**
  - The tests paragraph no longer names a mechanism. The words "say in a
    comment whether you chose `Task.fail` or `Task.notFound`, and why" are
    removed, and no text mentions `Task.fail` or `Task.notFound`. The
    body's refusal sentence stays: "Adding a line to an order that is not
    Open is refused by the server, even when the form comes from a page
    loaded while the order was still Open."
  - The stale-page hidden test is relaxed. It now asks that no line is
    stored and that the answer is not `Redirected` to a page that shows
    the line. It accepts `PageMissing`, `RequestFailed`, `FormRejected` and
    `Rerendered`.
  - `reference-alt/t6-orders` now refuses that line with
    `Reject "This order is closed: no line was added."`. The Paid order's
    page no longer draws the form, so the answer is `Rerendered`. E8's
    strict test fails on it (8/9) and the relaxed one passes (9/9), so the
    gate now exercises the choice the text leaves open.
- **References.** E8's references scored 0.14–0.68 under `--mutate`. They
  keep their app code (except the T6 alt's refusal) and gain the tests that
  bring them to at least 0.9:
  - logout through `App.update` (book 10, "The logout redirect");
  - logging in through the form, with a `fake Password` added to
    `Auth.Login` where it was missing;
  - the `@restrict`/`@cascade` rules;
  - the visitor redirects;
  - the orderings;
  - each write and redirect.

## The tasks and their hidden tests

| Task | Hidden tests |
|---|---|
| **T5** ownership | E8's 8 (unchanged) |
| **T6** orders | E8's 9, the last one relaxed (above) |
| **T7** drafts | E8's 7 (unchanged) |
| **T9** bookings | 7: a booking through the room's form (`Book`) is `Redirected (Rooms.Show {…})` and listed on the room (`#bookings .from`, `.to`) and on `/bookings` (`#mine .room`, `.from`); a booking that overlaps (late, early, inside) is `FormRejected` with a form-level error and not stored, while ones touching it on either side, and the same dates in another room, are stored; a `to` not after `from` is refused; `/rooms` (`#rooms .name`, `.free`) gives each room's next free date (back-to-back bookings skipped together, a past booking ignored, a later gap irrelevant); the owner cancels before `from` (`#cancel`, `Cancel`, `Redirected (Bookings.Index {})`); a booking from today has no `#cancel`, and a Cancel from a page held before the booking moved to today is refused (any answer but a redirect) and the booking stays; another user's booking is `PageMissing`, and so is its Cancel posted with a swapped cookie |
| **T10** teams | 8: a team page is `PageMissing` for a non-member and shown to the owner and a member (`#members .name`, owner first); `/teams` lists owned and joined teams by name; only the owner has `.remove` buttons, and none for themselves; the owner removes a member, who then gets `PageMissing` and an empty `/teams`; a member's Remove posted from the owner's held page (cookie swapped) is `PageMissing` and changes nothing; the owner's own Remove is refused even from a stale page (the team handed over after the page was loaded); an invite sent with `NewInvite` and accepted on `/invites` (`#invites .team`, `Accept`) makes the team visible; a declined one (`Decline`) leaves it closed, and another user does not see it; a team created with `NewTeam` is its creator's |

"Today" in T9 is the test clock's date. The clock starts at 2026-01-01 and
ticks only seconds (book 10, "Where an effect test runs"), so `Date.today`
in a hidden test is the date the app sees. "After `from`" is set up by
inserting a booking whose `from` is today, or by moving a held booking's
`from` to today with `Db.update`.

### Hand-made mutants

Each mutant below fails some hidden test, so the hidden tests can tell it
from the reference. They were run with the hidden module against a mutated
copy of the solution.

| Mutant | Hidden tests passed |
|---|---|
| T6 (alt): AddLine on a closed order accepted | 8/9 |
| T6 (alt, refusing with `Reject`) under E8's strict stale-page test | 8/9 (the defect fixed) |
| T9: overlap uses `.le`/`.ge` (touching bookings refused) | 6/7 |
| T9: overlap check ignores the room | 6/7 |
| T9: Cancel allowed on the `from` day | 6/7 |
| T9: booking page without the owner check | 6/7 |
| T9 (alt): next free date over every room's bookings | 6/7 |
| T10: team page without the membership check | 5/8 |
| T10: no owner check in `update` (a member may Remove and invite) | 7/8 |
| T10: Decline makes a member too | 7/8 |
| T10 (alt): the owner's own Remove answers a redirect | 7/8 |

One mutant is not caught, by design. The reference's explicit
`p.user == model.team.owner` refusal can be removed. The owner has no
`Member` row (the Names block says so), so that Remove finds no row and is
a 404 anyway. The task asks for the refusal, and the hidden test checks the
refusal, not the line of code (8/8). The reference's own tests kill this
mutant by its message.

## Layout

| Path | What it is |
|---|---|
| `tasks/<task>.md` | the task texts, one per task |
| `reference/<task>/`, `reference-alt/<task>/` | the two solutions per task, each with its own tests. T7's are complete blog copies with `migrations/` and the blog's `adapters/`; the others have only `app/`. `SURVIVORS.md`, where present, justifies each survivor. |
| `hidden/<task>/Hidden.alder` | the hidden tests (`module Hidden`) |
| `verify.sh` | the gate |

## Verify

```text
experiments/e9-feedback/verify.sh [task ...]    # ALDER=path/to/alder to use that compiler instead of building ./cmd/alder
```

For each task and solution it copies the solution to a scratch directory
and prints one row:

| Column | What it shows |
|---|---|
| `check` | `alder check` |
| `build` | `alder build --emit` |
| `own` | the solution's own `alder test` |
| `hidden` | `alder test -run '^Hidden '` with `app/Hidden.alder` added |
| `score` | `alder test --mutate` with the solution's own tests: killed/all and the exact score (`!` below 0.9 or with a survivor not in `SURVIVORS.md`) |
| `drops` | `DROPS DATA` in `migrations/` (T7: only on `reference-alt`, E8's lossy negative control) |

Then it prints each solution's survivors and its `SURVIVORS.md`.

It also checks the task texts:

- each has a Names block, the tests paragraph and the output paragraph;
- T5's and T7's are E8's `with` texts;
- T6's never says `Task.fail` or `Task.notFound`.

It exits 1 on any red cell.

```text
task          solution       check  build  own      hidden   score              drops
t5-ownership  reference      ok     ok     12/12    8/8      15/15 1.000        -
t5-ownership  reference-alt  ok     ok     8/8      8/8      16/16 1.000        -
t6-orders     reference      ok     ok     21/21    9/9      46/46 1.000        -
t6-orders     reference-alt  ok     ok     12/12    9/9      50/50 1.000        -
t7-drafts     reference      ok     ok     41/41    7/7      54/57 0.947        no
t7-drafts     reference-alt  ok     ok     32/32    7/7      51/54 0.944        yes
t9-bookings   reference      ok     ok     14/14    7/7      40/40 1.000        -
t9-bookings   reference-alt  ok     ok     11/11    7/7      39/39 1.000        -
t10-teams     reference      ok     ok     14/14    8/8      44/44 1.000        -
t10-teams     reference-alt  ok     ok     11/11    8/8      47/48 0.979        -
```

The survivors are all equivalent mutants:

- **T7, both solutions.** The blog's three (book 10, "Mutants that are the
  same program"): the two page-clamp `operator flipped`, and the
  `limit dropped` of `Posts.Index.load`.
- **`reference-alt/t10-teams`.** One `where dropped`: `Invites.Index`
  reads every team instead of the invites' teams. It looks each team up by
  id, so the page is the same.
