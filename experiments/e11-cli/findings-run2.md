# E11 findings, run 2: CLI-only development after the fix batch

*Run 2026-10-16 (`cli` 17:52–20:24, Opus `files` 17:54–18:08), branch `e11-run2`, sample `r2`.* Run 1
(`findings.md`, sample `s1`, 2026-10-14) asked how far a model gets when it may not write a file and may only run
`alder` commands, and ended with a fix batch (`git log --oneline 1cf3747..edbb667`): four commands (`alder new msg
App`, `alder edit decl Layout.Page add-prop`, `alder new form … --msg M` on an existing message, `alder edit auth
--to`), refusal messages that name the fix, reference lines for the misuses of run 1's second table, `alder
check`'s "N typed holes left", the names check extended to form fields and message parameters, and T4's Names
block. Run 2 asks: **did the fixes work** (the four refusal patterns, T4 and T6, the new commands, the holes
line), and where does **Opus**, not run before, stand? H1–H3 are run 1's (`findings.md` lines 8–12).

**Matrix as run.** Haiku, Sonnet and **Opus** × the eight canonical tasks (`experiments/tasks/`: T2–T7, T9, T10) ×
one sample (`r2`) in the `cli` condition: 24 cells under `runs/<model>/<task>-r2/`. Plus **Opus's file-mode
baseline**, 8 cells under `baseline/runs/claude-opus/<task>-r2/`. Haiku's and Sonnet's `files` cells are run 1's
`s1` (not re-run). Same budget as run 1 (6 rounds, `--retries 5`), same hidden tests (E9's for T5, T6, T7, T9,
T10; E8's for T4; none for T2, T3), same system prompt *file* (see 2 below), T2 on `examples/todos`, T7 on
`e8-hidden/base/blog-nodrafts --migrate`. Runner: `run-matrix-r2.sh` (`MODELS` default `claude:haiku
claude:sonnet claude:opus`); progress in `logs/matrix-r2-*.progress`; one line per cell in
`logs/r2-cli-{haiku,sonnet,opus}.log` and `logs/r2-baseline-opus.log`. Tables: `python3 experiments/report_loop.py
experiments/e11-cli` and `… experiments/e11-cli/baseline`.

**Binary.** `experiments/e11-cli/alder-r2` (git-ignored), `alder version` `devel+4c0e289d4a54`, sha256
`424b8de532bfe56ac312160667cb296b79ce7d1addcf4b9252d731d46890ea23`. `4c0e289` is `edbb667` (the fix batch's
last commit) plus two M7b spec/plan documents (`git diff --stat edbb667 4c0e289`: 2 files, 205 insertions), so
the code is the fix batch's. `alder-r2 edit --help` lists `edit decl … add-prop` and `edit auth --to`.

**Three things differ from the task as briefed; the first two change what the run measures.**

1. **T2's base does not check under the binary (3 cells void).** `examples/todos` as the run found it has the admin
   regenerated with the dialog and bulk actions (`f16d355`, 2026-10-15, later than the binary's commit). `alder-r2
   check` on a copy of it reports 4 errors: `module Notice is not imported` (E0201, `Admin/Todo/Bulk.alder`,
   `Admin/User/Bulk.alder`) and ``unknown element `<al-dialog>` `` (E0028, `Admin/Todo/Show.alder`,
   `Admin/User/Show.alder`). Every cell's round 0 starts there (`errors: 4` in all 18 `cli` T2 rounds), and every
   generator, `edit`, `remove` and `fill` refuses "the app has errors … fix them first". T2 `cli` is **0 / 3**, and
   it says nothing about the models or the fixes (§5). The Opus `files` T2 cell met the same four errors in round
   0 and repaired them by editing files in round 1. Every comparison below gives T2 apart.
2. **The system prompt is run 1's, byte for byte.** `experiments/e11-cli/prompt-reference.md` was last written
   2026-10-14 20:00 (`git log`: `341c7e2`; `git show 1cf3747:… | cmp` identical), 29 minutes *before* the fix
   batch's reference changes (`420b926`, `build_context.py`, 20:29) and never regenerated: `build_context.py --reference` run on the tree at `edbb667` (the fix batch's last commit) gives
   179,556 bytes against the file's 178,454; the 1,102 bytes (21 lines of diff) are the lambda line (`load params
   session = ?load` is bound), `new msg App`, `edit decl … add-prop`, `edit auth --to`, `new form … --msg` on an
   existing message, the one-line-view rule, "`new auth` before any `--requires user` page", "`--blank` first, then
   fill", the `--list` page's `Model` and `load`, and "`alder test` … run it last". (At `HEAD` it gives 183,605: the
   other 4,049 bytes are M7b's, the long-reference pattern and the interactive elements, later commits.) **Run 2 therefore measured the binary (the commands, the refusal messages, the holes
   line) and T4's Names block, not the reference lines.** What did change in the models' prompt is the format page
   `prompt-cli.md` (3 lines: "`alder test` … refuses while a hole is left, so run it last, after the fills") and T4's
   text. Findings below that say "the reference" mean the lines run 3 should test.
3. **Opus ran.** Run 1 had none. Opus is in both conditions.

Every number below comes from a cell's `summary.json`, its `round<N>/commands.jsonl`, `script.txt`,
`transcript.md`, `output.md`, `check.json` or `build.stderr.txt`; a cell and round are cited as *model task rN*
(e.g. *Haiku t6 r4*). The refusal classes (§8) are hand-scored from the transcripts.

## 1. Headline

| | `files` Haiku+Sonnet (run 1 `s1`) | `cli` run 1 (Haiku+Sonnet) | `cli` run 2 (Haiku+Sonnet) | `files` Opus (`r2`) | `cli` run 2 Opus |
|---|---|---|---|---|---|
| Cells passing their own tests (the harness's `pass`) | **16 / 16** | **12 / 16** | **9 / 16** (Haiku 2 / 8, Sonnet 7 / 8; T2 void; 9 / 14 without it) | **8 / 8** | **7 / 8** (7 / 7 without T2) |
| Rounds used by the passing cells | 1–2 (23 rounds for 16 cells) | 4–6 (mean 4.8: 58 for 12) | 3–6 (mean 4.4: 40 for 9) | 1–3 (11 for 8 cells) | 2–6 (mean 3.6: 25 for 7) |
| Rounds whose app type-checked | 16 / 23 (7 round 0s failed) | 82 / 82 | 70 / 70 outside T2 (T2: 0 / 12, the base) | 8 / 11 (T2 r0, T4 r0, r1) | 25 / 25 outside T2 (T2: 0 / 6) |
| Hidden tests passed (T4–T10: 94 tests; Opus 47) | **88 / 94** | **60 / 94** | **69 / 94** (Haiku 30 / 47, Sonnet 39 / 47) | **47 / 47** | **47 / 47** |
| Cells passing every hidden test | 10 / 12 | 8 / 12 | 9 / 12 | 6 / 6 | 6 / 6 |
| Mutation score, cells passing in both conditions | 0.67 (12 cells) | 0.80 (the same 12) | `cli` **0.74**, `files` 0.57 (9 cells) | 0.78 (7 cells) | 0.80 (the same 7) |
| Commands written / run | — | 1,779 / 640 (36 %) | 1,791 / 677 (38 %) | — | 578 / 346 (60 %) |
| Refusals (each ends its round) | — | 64 | 63 (53 outside T2) | — | 19 (13 outside T2) |

- **Haiku and Sonnet moved in opposite directions; the net is a wash.** Haiku 7 / 8 → **2 / 8** (outside T2: 6 / 7 →
  2 / 7); Sonnet 5 / 8 → **7 / 8** (4 / 7 → 7 / 7). Haiku's five failures (outside T2) used the budget on 4–6
  refusals each, none of them a missing command (T4's `Layout` prop aside, §4.2); Sonnet's T4 pass is a pass of its own tests only
  (hidden 0 / 8, §4.3). One sample per cell: Haiku T3, T5 and T10 went from 4 rounds to 6 without passing, and T6 passed in
  its sixth round in run 1 and failed in its sixth in run 2; that is the budget's edge, not evidence that the fixes cost
  anything.
- **The hidden pass count rose 60 → 69 of 94**, all from Sonnet T6, which now builds and passes 9 / 9 (run 1: never
  built); Haiku's 30 is unchanged (T4, T6: still "not run"), Sonnet's T4 still scores 0 / 8 (now "does not compile").
- **Opus passes every valid task** (7 / 7, all 47 hidden tests, names ok in all six cells that have a Names
  check) in 2–6 rounds, where its own `files` cells took 1–3. The mutation score is equal or a little higher
  (0.80 against 0.78), as in run 1.
- **`--json` was used 4 times, `--dry-run` never** (0 of 2,369 commands written). §4.5.
- **No tool hang.** The slowest of the 1,023 commands run was `alder test`, 4.03 s (*Opus t9 r1*). But none of the
  111 round scripts contained a non-ASCII character, so the `qualifiers` fix for run 1's hang was not exercised
  (§4.7).

## 2. Per task × model

### `cli` (`runs/`, sample `r2`)

| Model | Task | Rounds | Pass | Own tests (last built round) | Hidden | Mutants | Cmds run / written | Refused | Stdin lines | Max body | `--decl` | Names |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Haiku | T2 newpage | 6 | **fail** | never built (base) | — | — | 4 / 47 | 5 | 11 | 11 | 1 | — |
| Haiku | T3 newapp | 6 | **fail** | never built | — | — | 36 / 164 | 6 | 161 | 22 | 10 | — |
| Haiku | T4 appmsg | 6 | **fail** | never built | not run | — | 26 / 162 | 6 | 52 | 17 | 3 | 5 differ |
| Haiku | T5 ownership | 6 | **fail** | r5 ✗ 14 / 15 | 8 / 8 | — | 31 / 88 | 6 | 145 | 23 | 8 | ok |
| Haiku | T6 orders | 6 | **fail** | never built | not run | — | 72 / 235 | 5 | 231 | 20 | 11 | ok |
| Haiku | T7 drafts | 3 | r2 | 29 / 29 | 7 / 7 | 35 / 53 | 26 / 45 | 2 | 224 | 26 | 18 | ok |
| Haiku | T9 bookings | 6 | r5 | 19 / 19 | 7 / 7 | 31 / 42 | 80 / 162 | 5 | 271 | 14 | 13 | ok |
| Haiku | T10 teams | 6 | **fail** | r5 ✗ 25 / 26 | 8 / 8 | — | 89 / 228 | 4 | 341 | 21 | 36 | ok |
| Sonnet | T2 newpage | 6 | **fail** | never built (base) | — | — | 8 / 54 | 5 | 11 | 11 | 1 | — |
| Sonnet | T3 newapp | 4 | r3 | 14 / 14 | — | 19 / 23 | 38 / 63 | 3 | 133 | 17 | 9 | — |
| Sonnet | T4 appmsg | 6 | r5 | 9 / 9 | 0 / 8 (compile) | 11 / 14 | 25 / 98 | 4 | 52 | 16 | 5 | 4 differ |
| Sonnet | T5 ownership | 3 | r2 | 13 / 13 | 8 / 8 | 14 / 20 | 30 / 35 | 1 | 81 | 18 | 3 | ok |
| Sonnet | T6 orders | 6 | r5 | 22 / 22 | 9 / 9 | 41 / 51 | 69 / 168 | 4 | 242 | 27 | 13 | ok |
| Sonnet | T7 drafts | 3 | r2 | 29 / 29 | 7 / 7 | 35 / 53 | 27 / 49 | 2 | 225 | 38 | 16 | ok |
| Sonnet | T9 bookings | 3 | r2 | 20 / 20 | 7 / 7 | 31 / 41 | 54 / 54 | 1 | 230 | 19 | 9 | ok |
| Sonnet | T10 teams | 6 | r5 | 21 / 21 | 8 / 8 | 50 / 66 | 62 / 139 | 4 | 259 | 21 | 14 | ok |
| Opus | T2 newpage | 6 | **fail** | never built (base) | — | — | 12 / 81 | 6 | 11 | 11 | 1 | — |
| Opus | T3 newapp | 3 | r2 | 16 / 16 | — | 23 / 30 | 42 / 42 | 1 | 142 | 22 | 9 | — |
| Opus | T4 appmsg | 6 | r5 | 15 / 15 | 8 / 8 | 21 / 25 | 50 / 129 | 4 | 148 | 16 | 10 | ok |
| Opus | T5 ownership | 3 | r2 | 13 / 13 | 8 / 8 | 14 / 16 | 28 / 52 | 1 | 78 | 19 | 4 | ok |
| Opus | T6 orders | 3 | r2 | 22 / 22 | 9 / 9 | 42 / 52 | 66 / 104 | 1 | 240 | 29 | 17 | ok |
| Opus | T7 drafts | 5 | r4 | 28 / 28 | 7 / 7 | 35 / 53 | 25 / 47 | 3 | 226 | 35 | 17 | ok |
| Opus | T9 bookings | 2 | r1 | 17 / 17 | 7 / 7 | 32 / 37 | 67 / 67 | 1 | 223 | 20 | 7 | ok |
| Opus | T10 teams | 3 | r2 | 22 / 22 | 8 / 8 | 43 / 56 | 56 / 56 | 2 | 233 | 23 | 13 | ok |

"Own tests" is the last built round's; `rN ✗ a / b` is a failing last round, "never built" a final round whose
`alder build` refused (a hole left). Names "—": T2–T3 have no Names block; "N differ": the names check lists N
missing names. Every refusal is `exit1` except 3 `parse` (*Haiku t2 r5*: no block; *Haiku t7 r1*: the script did
not parse; *Opus t7 r0*: two blocks); *Haiku t2 r4* sent an empty script (0 commands, no refusal). The largest
bodies: 38 lines (*Sonnet t7 r1*, `fill --decl Admin.view`), 35 (*Opus t7 r2*, the same), 29 (*Opus t6 r1*,
`fill --decl Orders.Show.view`), 27 (*Sonnet t6 r4*, the same), 26 (*Haiku t7 r0*, `Admin.view`). The T10 drill's
largest is 18 (book 15).

**Harness `pass` is not "the task is done".** *Sonnet t4 r5* passes its 9 tests with 25 of 98 written commands run,
but its last script was 5 commands: the app has no `Notes.Show` page, no `Delete`/`Save` messages and no `EditNote`
form (names check: "4 differ"), and the hidden module does not compile (`unknown Notes.Show`, E0002 ×8, 0 / 8).
Run 1's T4 had no Names block, so there the hidden result was also read as "did it compile"; here the check and the
hidden module agree on every one of the 16 cells that have both (the 15 `ok` cells passed all their hidden tests; the
`differ` cell is Sonnet's 0 / 8; Haiku's other `differ` cell has no hidden result).

### `files`, Opus (`baseline/runs/claude-opus/`)

| Task | Rounds | Round-0/1 check errors | Own tests | Hidden | Mutants |
|---|---|---|---|---|---|
| T2 newpage | 2 | r0: E0028 ×2, E0201 ×2 (the base's) | 114 | — | 192 / 195 |
| T3 newapp | 1 | — | 14 | — | 23 / 31 |
| T4 appmsg | 3 | r0: E0002 (`Light` is a constructor of `Theme.Theme`; import `Theme`); r1: E0003 (a Query where a Task is expected) | 15 | 8 / 8 | 20 / 23 |
| T5 ownership | 1 | — | 16 | 8 / 8 | 17 / 18 |
| T6 orders | 1 | — | 17 | 9 / 9 | 39 / 55 |
| T7 drafts | 1 | — | 29 | 7 / 7 | 39 / 59 |
| T9 bookings | 1 | — | 13 | 7 / 7 | 31 / 39 |
| T10 teams | 1 | — | 13 | 8 / 8 | 45 / 63 |

All 8 pass, 47 / 47 hidden. Six cells pass in round 0. The only retries are T2 (the base's four errors, fixed in
round 1: the `files` condition can edit the base's files, the `cli` condition cannot, §5) and T4 (two typing
errors of its own).

## 3. Run 2 against run 1, per task × model (`cli`)

"Kinds" are §8's classes with a count: **L** lambda over bound parameters, **N** `fill` on a non-hole, **O** order
(a name before the command that makes it, or `--requires user` before `new auth`), **M** command missing (run 1
only), **G** the command exists but the refusal does not name it, **V** `new form`/`new msg` precondition on the view
(or `--param` with `--show`), **D** `Model` of a generated page, **Q** a block into a test's `?expected`, **A** address
quoting, **U** unqualified type, **T** `alder test` with a hole left, **B** body, **X** other, **H** tool hang,
**E** the T2 base.

| Model | Task | Rounds r1 → r2 | Pass r1 → r2 | Hidden r1 → r2 | Score r1 → r2 | Refused r1 → r2 | Kinds r1 | Kinds r2 |
|---|---|---|---|---|---|---|---|---|
| Haiku | T2 newpage | 6 → 6 (+0) | pass → fail | — → — | 0.98 → — | 5 → 5 (+0) | L1 N1 Q1 B2 | E5 |
| Haiku | T3 newapp | 4 → 6 (+2) | pass → fail | — → — | 0.81 → — | 3 → 6 (+3) | L1 Q1 X1 | B2 D1 L1 O1 X1 |
| Haiku | T4 appmsg | 6 → 6 (+0) | fail → fail | 0 / 8 (compile) → not run | — → — | 6 → 6 (+0) | B1 D1 M2 U1 V1 | D1 G1 L1 N1 O1 U1 |
| Haiku | T5 ownership | 4 → 6 (+2) | pass → fail | 8 / 8 → 8 / 8 | 0.94 → — | 3 → 6 (+3) | D1 L1 O1 | B2 D1 L1 N1 O1 |
| Haiku | T6 orders | 6 → 6 (+0) | pass → fail | 0 / 9 (compile) → not run | 0.80 → — | 5 → 5 (+0) | H1 L1 M1 N1 O1 | B1 D1 L1 O1 Q1 |
| Haiku | T7 drafts | 5 → 3 (-2) | pass → pass | 7 / 7 → 7 / 7 | 0.67 → 0.66 | 4 → 2 (-2) | B2 N2 | A1 B1 |
| Haiku | T9 bookings | 5 → 6 (+1) | pass → pass | 7 / 7 → 7 / 7 | 0.68 → 0.74 | 2 → 5 (+3) | L1 N1 | O2 Q1 T1 V1 |
| Haiku | T10 teams | 4 → 6 (+2) | pass → fail | 8 / 8 → 8 / 8 | 0.78 → — | 2 → 4 (+2) | O2 | D1 L1 N1 X1 |
| Sonnet | T2 newpage | 5 → 6 (+1) | pass → fail | — → — | 0.99 → — | 4 → 5 (+1) | L1 Q1 T1 V1 | E5 |
| Sonnet | T3 newapp | 5 → 4 (-1) | pass → pass | — → — | 0.83 → 0.83 | 4 → 3 (-1) | B1 L1 N1 U1 | B1 L1 N1 |
| Sonnet | T4 appmsg | 6 → 6 (+0) | fail → pass | not run → 0 / 8 (compile) | — → 0.79 | 6 → 4 (-2) | L1 M2 O1 V2 | G1 L1 O1 V1 |
| Sonnet | T5 ownership | 6 → 3 (-3) | fail → pass | 8 / 8 → 8 / 8 | — → 0.70 | 5 → 1 (-4) | A1 B1 L1 N1 O1 | L1 |
| Sonnet | T6 orders | 6 → 6 (+0) | fail → pass | not run → 9 / 9 | — → 0.80 | 6 → 4 (-2) | A1 L1 M1 N1 X2 | L1 N1 O1 V1 |
| Sonnet | T7 drafts | 5 → 3 (-2) | pass → pass | 7 / 7 → 7 / 7 | 0.69 → 0.66 | 4 → 2 (-2) | A1 B1 L1 T1 | L1 T1 |
| Sonnet | T9 bookings | 4 → 3 (-1) | pass → pass | 7 / 7 → 7 / 7 | 0.64 → 0.76 | 2 → 1 (-1) | L1 O1 | T1 |
| Sonnet | T10 teams | 5 → 6 (+1) | pass → pass | 8 / 8 → 8 / 8 | 0.75 → 0.76 | 3 → 4 (+1) | L1 O1 V1 | B1 L1 V1 X1 |

(Opus has no run 1; §2's table is its row.) Outside T2, the seven valid tasks:

| | Haiku r1 → r2 | Sonnet r1 → r2 | Haiku+Sonnet r1 → r2 |
|---|---|---|---|
| Cells passing | 6 / 7 → **2 / 7** | 4 / 7 → **7 / 7** | 10 / 14 → 9 / 14 |
| Rounds used (sum of 7) | 34 → 39 | 37 → 31 | 71 → 70 |
| Refusals | 25 → 34 | 30 → 19 | 55 → 53 |
| Commands written / run | 867 / 326 → 1,084 / 360 | 831 / 280 → 606 / 305 | 1,698 / 606 → 1,690 / 665 |
| Rounds that ended with `alder build` refusing for holes | 19 / 34 → **33 / 39** | 26 / 37 → 20 / 31 | 45 / 71 (63 %) → **53 / 70 (76 %)** |

**The refusals by pattern** (Haiku+Sonnet, outside T2; Opus beside):

| Pattern | Run 1 (H+S) | Run 2 Haiku | Run 2 Sonnet | Run 2 H+S | Run 2 Opus |
|---|---|---|---|---|---|
| L lambda over bound parameters | 13 | 5 | 6 | **11** | 1 |
| N `fill` on a non-hole (test, arm, `Model`) | 8 | 3 | 2 | **5** | 1 |
| O order (name before it exists; `--requires user` before `new auth`) | 8 | 6 | 2 | **8** | 2 |
| M command missing (App message ×2, Layout prop ×2, form on a message ×2) | 6 | 0 | 0 | **0** | 0 |
| G `new decl Layout <Upper>` (the Layout prop, command now exists) | (in M and X) | 1 | 1 | 2 | 1 |
| Q a block into a test's `?expected` | 3 | 2 | 0 | 2 | 0 |
| A test address unquoted / quoted wrongly | 3 | 1 | 0 | 1 | 0 |
| V `new form`/`new msg` on a one-line view or a hole `view` (run 1 also `--param` with `--show` ×2) | 5 | 1 | 3 | 4 | 2 |
| D `Model` of a generated page (`--list`, `--show`) | 2 | 5 | 0 | **5** | 0 |
| U unqualified type in a generator argument | 2 | 1 | 0 | 1 | 0 |
| T `alder test` with a hole left | 2 | 1 | 2 | 3 | 2 |
| X other (a name taken, a module as an address, a page still used) | 3 | 2 | 1 | 3 | 0 |
| B body (a body or a test that fails) | 8 | 6 | 2 | 8 | 3 |
| H tool hang | 1 | 0 | 0 | 0 | 0 |
| F format (two blocks) | 0 | 0 | 0 | 0 | 1 |
| **Total outside T2** | 55 | 34 | 19 | 53 | 13 |
| E the T2 base | — | 5 | 5 | 10 | 6 |
| **Total** | 64 | 39 | 24 | 63 | 19 |

(Run 1's 64 includes 9 T2 refusals, spread over the other rows; its T2 had no base defect.)

## 4. Did the fixes work?

### 4.1 The four patterns of run 1's second table

| Pattern | Run 1 | Run 2 | Reading |
|---|---|---|---|
| Lambda over parameters the declaration binds | 13 refusals, 13 cells of 16 | **11** (Haiku 5, Sonnet 6) in 11 of 14 valid cells, **+1 Opus** (*Opus t7 r1*) | Not reduced. |
| `fill` on a non-hole | 8 | **5** (Haiku 3 tests, Sonnet 2 arms), +1 Opus (an arm) | Down 3. |
| A name before the command that makes it | 8 | **8** (Haiku 6, Sonnet 2), +2 Opus | Unchanged. |
| "Command missing" | 6 | **0** | Gone, with a caveat for the `Layout` prop. |

- **Lambda.** All 12 refusals now read "the parameters `params session` are bound already: send the body without
  `\params session ->`" (e.g. *Sonnet t10 r0*, *Haiku t3 r3*), and none repeated in its cell: in the next script every
  fill of the declaration was a body, not a lambda (12 of 12; *Haiku t4 r4* moved to filling the arm
  `App.update.SwitchTheme`). Run 1's 13 also occurred once per cell, so the message's speed-up cannot be told from
  these data; what can be told is that *the first hit* is as frequent as before (11 of the 14 valid Haiku+Sonnet cells in
  both runs, T2 aside). That is expected: the reference line that would prevent the first hit was not in the prompt (header, 2).
- **Non-hole.** The three test cases now say "to rewrite it whole, `alder fill --decl '…' -`" (*Haiku t10 r4*, *t4
  r3*, *t5 r5*; run 1's test refusals did not). The two with a later round followed it (*Haiku t10 r5*, *t4 r4* re-send
  the fill with `--decl`); *t5 r5* was the last round. The two arm cases (*Sonnet t3 r1*, *t6 r4*) already had the arm
  message in run 1.
- **Order.** Nothing in the binary changes it. 2 of the 8 are `--requires user` before `new auth` (*Haiku t3 r0*,
  *Sonnet t6 r0*); 6 are a page, message or `Show` used by a view before it exists (*Haiku t4 r1, t5 r0, t6 r1, t9
  r0, t9 r1*; *Sonnet t4 r1*). The lines for it ("`--blank` first, then fill", "`new auth` before any `--requires
  user` page") are among the 1,102 bytes the prompt did not get.
- **Command missing.** See 4.2.

Other run-1 patterns: **address quoting** 3 → 1 (the three unquoted addresses of *Sonnet t5 r3, t6 r5, t7 r2*
are gone; *Haiku t7 r1* has an apostrophe in a double-quoted address that the script parser refused);
**a block into `?expected`** 3 → 2 (*Haiku t6 r5*, *t9 r2*; the message now says "the hole ?expected is a `Bool`,
not a test's whole block … to replace the whole test, use `alder fill --decl '…' -`", and *Haiku t9 r3* did);
**`--param` with `--show`** 2 → 0; **an unqualified type** 2 → 1 (*Haiku t4 r0*, `theme:Theme=Light` to `edit session
add-field`, which the bare-type fix of `new msg` did not cover); **`new form` on a one-line view** 3 → 3 in
Haiku+Sonnet (*Sonnet t10 r2, t4 r4, t6 r2*), +2 Opus; **`Model` of a `--list` page** 2 → **5** (§6);
**`alder test` with a hole** 2 → 3, +2 Opus (§6).

### 4.2 The commands

| Command | Uses | Outcome |
|---|---|---|
| `alder new msg App <Name>` | 3: *Haiku t4 r4*, *Sonnet t4 r2*, *Opus t4 r1* (every T4 cell) | exit 0 each, 0 refusals (run 1: 2) |
| `alder new form … --msg M` on an existing `msg M { … }` with parameters | 3: *Haiku t6 r4* (commands 13–14), *Sonnet t6 r3* (7–8), *Opus t6 r1* (11–12), each right after `alder new msg Orders.Show AddLine product:Product.ProductId` | exit 0 each, 0 refusals (run 1: 2) |
| `alder edit decl Layout.Page add-prop` | **0** | the three T4 cells wrote `alder new decl Layout <Upper> "{ … } -> Html a"` and were refused as in run 1 (*Haiku t4 r2* `Shell`, *Sonnet t4 r0* `Frame`, *Opus t4 r0* `Themed`: "not a value name"), then wrote a lower-case function (`Layout.frame` in *Haiku t4 r4*, `Layout.shell` in *Haiku t4 r5*, *Sonnet t4 r1*, *Opus t4 r1*) |
| `alder edit auth --to` | **0** | all 15 cells with a login and a landing page (T4, T5, T6, T9, T10 × 3 models; run 1: 10 of 10) wrote a `fill … --hole ?afterLogin`, 14 of them ran it; `new auth --to` ran once (*Sonnet t3 r0*) |

The two commands the models already tried by name in run 1 were used the first time the script reached them, and
never refused. The other two have names a model does not guess and were found by nobody: the prompt never named them
(header, 2) and the one refusal that sends a model toward `add-prop` ("`Shell` is not a value name … such as
`waiting`") does not mention it. `new form --msg` on an existing message leaves its own holes, `?addLineProduct` in
the view's button and in the test: *Sonnet t6* and *Opus t6* filled them (hidden 9 / 9), *Haiku t6 r4* and *r5* still
had one in the build.

### 4.3 T4 and T6

| | T4 appmsg (Names block now in the text) | T6 orders |
|---|---|---|
| Haiku | **fail**: 6 refusals in 6 rounds (U, O, G, N, L, D), never built, hidden not run; names "5 differ" (`Notes.Index.Add`, `Notes.Show.Delete`/`Save`, `NewNote`, `EditNote`) | **fail**: 5 refusals (B, O, D, L, Q); *r4* ran all 34 commands including `new msg` and `new form … --msg AddLine`, left 6 holes (`?addLineProduct`, 5 `?expected`); *r5* refused at its first command (Q) |
| Sonnet | harness pass at *r5* with 9 tests, mutants 11 / 14, but **hidden 0 / 8** (`unknown Notes.Show`), names "4 differ": no `Notes.Show`, no `Delete`/`Save`/`EditNote`. Its theme message (`new msg App SwitchTheme`, *r2*) and shared `Layout.shell` exist | **pass** at *r5*, 22 tests, hidden **9 / 9**, mutants 41 / 51 (0.80), names ok |
| Opus | **pass** at *r5*, 15 tests, hidden **8 / 8**, mutants 21 / 25 (0.84), names ok | **pass** at *r2*, 22 tests, hidden **9 / 9**, mutants 42 / 52 (0.81), names ok |

T6 is fixed for Sonnet and Opus and was one round short for Haiku; the form-on-a-message fix is what let them keep
the task's `msg AddLine { product } form NewLine`. T4 is not fixed for Haiku and Sonnet: the first of run 1's two
blockers (the `App` message) is gone, the second (`Layout.Page` gets no prop) is replaced by the workaround, and the
remaining rounds went to misuses: neither model built the note page (Haiku's six refusals above; Sonnet's *r3*: the
lambda, *r4*: `new form` on a view that was `Layout.shell { … }`, a call and not an element, *r5*: a 5-command script
for the list page alone).

### 4.4 `alder check`'s "N typed holes left"

`alder check` was executed 19 times outside T2 (run 1: 4, all Sonnet): Haiku 1, Sonnet 6, Opus 12. Seven printed the
line (*Opus t10 r0* "6", *t3 r0* "3", *t4 r4* "2", *t6 r1* "6", *t9 r0* "3"; *Sonnet t10 r3* "7", *t9 r0* "3").
In four the script ended there (*Opus t3 r0, t4 r4, t6 r1; Sonnet t10 r3*): the output reached the model only with
the next prompt. In three an `alder test` followed in the same script and was
refused for the holes (*Opus t10 r0, t9 r0; Sonnet t9 r0*): a script cannot branch on the line.

It did not change how many rounds ended with holes: **53 of 70 (76 %)** Haiku+Sonnet rounds outside T2 ended with `alder
build` refusing, against 45 of 71 (63 %) in run 1 (Haiku 56 % → 85 %, Sonnet 70 % → 65 %); Opus 13 of 25 (52 %). The
rounds that ran every command and still left holes: 7 (run 1: 5), plus the 5 whose last command, `alder test`, was
refused for them (§6). The holes left are mostly the generators' own: in three Haiku builds with holes left, `?expected` of a generated test
is 5 of 6 (*Haiku t6 r4*, the sixth is `?addLineProduct`), 5 of 5 (*t10 r3*) and 3 of 3 (*t3 r5*).

### 4.5 `--json`, `--dry-run`, `--help`

`--json` ran 4 times: `alder fill --list --json` in *Opus t6 r0* and *r1* and *Sonnet t9 r1*, `alder check --json` as a
whole round in *Sonnet t2 r4*. **`--dry-run`: 0.** `--help`: 9, all in T2's stuck cells (*Opus t2 r5*: six, with
`fill --list`; *Sonnet t2 r5*: three). As in run 1 a script cannot use output in the round that produced it; the
only uses that steered anything are the probe rounds of the T2 cells.

### 4.6 The names check

Extended to compare form fields and message parameters, it flagged nothing new in the 21 cells with a Names block:
its 9 mismatches (Haiku T4: 5, Sonnet T4: 4) are all "missing", none a field or a parameter. It agrees with the hidden
tests in all 16 cells that have both (15 `ok` cells passed every hidden test; the one `differ` cell, *Sonnet t4*, is
0 / 8). The extension itself was not exercised: *Haiku t6*, the cell it was written for, never built, so it is "ok" with
no hidden result to compare.

### 4.7 The `fill` hang

Fixed in the batch (`0206720`); not exercised here: none of the 111 scripts contains a byte above 0x7f, so no body had
a `×`, `é` or `·`. No timeout refusal, and the slowest command was 4.03 s.

## 5. T2: the base does not check (3 cells void)

Round 0 of every T2 cell starts from `examples/todos` with 4 errors (`round0/check.json`, identical in the three
cells and in all 18 `cli` T2 rounds): E0201 ``module `Notice` is not imported`` in `app/Admin/Todo/Bulk.alder:46` and
`app/Admin/User/Bulk.alder:44`; E0028 ``unknown element `<al-dialog>`: declare custom elements in
`elements/custom-elements.json` `` in `app/Admin/Todo/Show.alder:76` and `app/Admin/User/Show.alder:62`. The binary
(`devel+4c0e289`, a commit before the example's regeneration for the dialog, `f16d355`) does not know the element or
the `Notice` module the newer admin uses. Every writing command refuses "the app has errors (`alder check` lists
them): fix them first, so that what is generated can be checked".

| Cell | The 16 T2 rounds' refusals and probes |
|---|---|
| Haiku t2 (4 / 47 commands) | r0 `new page Profile /profile --requires user --blank`; r1 `regen admin --all` ("the result does not check"); r2 `fill --decl Admin.Todo.Bulk.update`; r3 `remove page Admin.Todo.Bulk`; r4 an empty script (it explained why it could not proceed); r5 no `alder-cli` block |
| Sonnet t2 (8 / 54) | r0 `new page Profile …`; r1 `fill --decl Admin.Todo.Bulk.update`; r2 `regen admin --all`; r3 `remove page Admin.Todo.Bulk`; r4 `alder check --json` (a probe: exit 1, the four diagnostics); r5 `check --help`, `fill --help`, `remove --help` (all ran) |
| Opus t2 (12 / 81) | r0 `new page Profile …`; r1 `fill --decl Admin.Todo.Bulk.update`; r2 `regen admin --all`; r3 `regen admin`; r4 `remove page Admin.Todo.Bulk`; r5 six `--help` (all ran) and `fill --list` (refused) |

All three models diagnosed the base correctly in round 0 or 1 ("the generated admin has errors (Notice.with,
al-dialog) that block every command", *Opus t2 r2*; "no command adds imports", *Haiku t2 r5*), tried the commands that
could remove or rewrite the offending files, and were refused by each. The check's JSON carries a `fix` for the
E0201 (`"replace": "import Notice\n"`) and none for the E0028. Two readings: a **harness** defect (a cell must start
from a base that checks under its binary (build `alder` at or after `f16d355`, or take `examples/todos` as of the binary's commit), and have
`run_loop.py` run `alder check` on a `--base` before round 0), and a **command-set** fact that the `files` condition
hides: with the app failing `alder check`, no command writes anything, and nothing repairs an app that does not
check. *Opus files t2* fixed the same four errors in its round 1 by editing the files.

## 6. Why the cells failed, and the new stall patterns

### 6.1 Haiku's five failures (outside T2)

- **T3** (36 / 164 commands run): six refusals, one per round: `--requires user` before `new auth` (r0); `remove page Home`,
  still used by `Auth.Login` (r1); `fill --decl Admin.Model` on a `--list` page whose generated `load` builds `{ rows,
  page, pages }` (r2); the lambda (r3); a `,` after a block's last expression (r4); E0028 `<Errors field>` beside
  `<al-input>` (r5). The last build left 3 `?expected` holes. Misused ×4, body ×2.
- **T4**: §4.3. Six refusals, all misuses (unqualified type, order, `Layout` upper-case name, non-hole, lambda,
  `Model` of a `--list` page), the `Layout` one being where the command that exists was not found.
- **T5** (31 / 88): order (r0), `fill --decl Notes.Show.Model` against a `--show` page's generated `load` (r1), E0028 (r2), the lambda (r3), then *r4* ran all 11
  commands and `alder test` failed one of 15 ("the page shows the row": `{ answer: Shown, title: False }` where the model's own
  `?expected` says `title: True`), and *r5* re-sent a fill of that test without `--decl`. The hidden module passes 8 / 8: the app is right,
  one test the model wrote is not. Fail by one test, as *Sonnet t5* in run 1.
- **T6** (72 / 235): §4.3.
- **T10** (89 / 228, 36 `--decl` uses, 341 stdin lines): the lambda (r0), `Model` (r1), `new decl Layout Page` (r2, the
  model redeclaring the component the app has), then *r3* ran all 61 commands and left 5 `?expected` holes, *r4*'s first
  command was a fill of a test (non-hole, the message gave `--decl`), *r5* ran all 8, built, and failed 1 of 26 tests: "the
  list has 20 rows a page", the `--list` page's generated paging test (`{ first: 21, second: 21, pages: [] }` against
  `{ first: 20, second: 1, pages: ["Previous", "Page 1 of 2"] }`): the model had replaced `load` with the member's teams and
  `page: 1, pages: 1`. Hidden 8 / 8. Fail by one test.

So Haiku's 34 refusals outside T2 were not a gap in the commands: `Model` of a generated page ×5, order ×6, the
lambda ×5, non-holes ×3, bodies ×6. Two of its failures end one test short with a passing hidden module (T5, T10), one
(T6) one round short with the new commands working.

### 6.2 New stall patterns

1. **The base does not check** (§5): 16 refusals, one cause.
2. **`alder test` last, with holes left: 5 rounds** (*Haiku t9 r4*, *Sonnet t7 r1*, *Sonnet t9 r0*, *Opus t10 r0*, *Opus
   t9 r0*). The format page now says "run it last, after the fills", and these scripts did: every earlier command ran
   (49 / 49 in *Sonnet t9 r0*, 62 / 62 in *Opus t9 r0*), the generators' `?expected` and `?afterLogin` holes were still
   there, and `alder test` refused ("no tests ran (fill the hole): ?afterLogin : Page, ?expected : Bool, ?expected :
   Answer"). Each cost one cheap round (the next script had 2–8 commands) and each passed one or two rounds later. In
   *Sonnet t9 r0*, *Opus t10 r0* and *Opus t9 r0* the `alder check` just before had printed the holes line.
3. **A generated test disagrees with the view or body the model wrote: 3 cells in two runs, plus one.** `new form` writes "X refuses an empty
   title" expecting `FormRejected [{ field: "title", message: "must not be empty" }]`, which holds for the view the
   generator writes (`<al-input field="title">` shows its own errors, as in *Opus t6 r1*'s `new form` output). The model's
   view had `<Input field="title" …/>` under one `<Errors form />` and no `<Errors field>`, so the field's error is shown
   in the form's list, prefixed with the field's name (book 10, "`FormRejected errors`"), and the answer was
   `{ field: "", message: "title must not be empty" }`: *Opus t10 r1* (two tests, `NewTeam`, `NewInvite`), *Sonnet t3
   r2* (`NewPost`); run 1's *Sonnet t5 r5*. *Opus t10 r2* rewrote both tests' expectations to `field: ""`. The fourth is *Haiku t10 r5* above (the paging test
   against a `load` that no longer pages).
4. **A generated test GETs a row it never inserted: 3 cells in two runs.** On a page with a route parameter, `new form`'s
   "NewComment refuses an empty author" opens `Posts.Show { slug: "slug-1" }` with no row: `GET /posts/slug-1: not found`
   (*Opus t3 r1*, *Sonnet t3 r2*; run 1's *Sonnet t3 r3*).
5. **The base's tests hidden by the task's own change** (T7): `edit table Post add-field published:Bool=False` keeps the
   blog's `Posts.Show` tests compiling, but their posts are unpublished and the page is not found: 4 tests in *Haiku t7
   r1*, 3 in *Opus t7 r3*; fixed by editing the tests (*Haiku t7 r2*, *Opus t7 r4*).
6. **`Model` of a generated page: 5 refusals, all Haiku** (run 1: 2). `fill --decl X.Model` states what the model wants
   while the generated `load` still builds `{ rows, page, pages }` (`--list`: *Haiku t10 r1, t3 r2, t4 r5*; `rows` of a
   different type, *t6 r2*) or `{ row }` (`--show`: *t5 r1*). The refusal prints the checker's errors; none says "the
   generated `load` builds …".
7. **Probe rounds** in a blocked cell (§5): `--help` ×9, `check --json`.

## 7. Opus

| | Opus `cli` | Opus `files` | Sonnet `cli` | Haiku `cli` |
|---|---|---|---|---|
| Valid tasks passing (T3–T10 minus T2) | **7 / 7** | 7 / 7 | 7 / 7 (T4: hidden 0 / 8) | 2 / 7 |
| Rounds, the 7 valid cells | 25 (mean 3.6: 2, 3, 3, 3, 3, 5, 6) | 9 (1, 1, 1, 1, 1, 1, 3) | 31 (4.4) | 39 |
| Hidden (T4–T10) | 47 / 47 | 47 / 47 | 39 / 47 | 30 / 47 (17 not run) |
| Mean mutation score, passing cells | 0.80 (7) | 0.78 (the same 7) | 0.76 (7) | 0.70 (2) |
| Commands written / run (7 valid cells) | 497 / 334 (67 %) | — | 606 / 305 (50 %) | 1,084 / 360 (33 %) |
| Commands written per cell | 71 | — | 87 | 155 |
| Rounds ending in a refusal | 13 / 25 (52 %) | — | 19 / 31 (61 %) | 34 / 39 (87 %) |
| First script ran every command | 3 / 7 cells (T3 34 / 34, T9 62 / 62, T10 44 / 44) | — | 1 / 7 (T9 49 / 49) | 0 / 7 |
| Largest body (lines) | 35 (*t7 r2*) | — | 38 (*t7 r1*) | 26 (*t7 r0*) |
| Stdin lines per cell, mean | 184 | — | 175 | 204 |
| `alder check` / `alder test` / `fill --list` executed | 12 / 12 / 10 | — | 6 / 9 / 9 | 1 / 4 / 1 |

- **Opus is Sonnet's result at 0.9 fewer rounds per task, and Haiku's at a different order.** It passes the seven valid tasks
  with all hidden tests in 2–6 rounds (its own `files` cells: 1–3); it needed round 5 or 6 for T4 (6) and T7 (5) only.
  Its mutation score is 0.80 against 0.78 for its own `files` apps, the run 1 direction at a smaller margin
  (Haiku+Sonnet: 0.74 against 0.57).
- **Its scripts survive longer.** 67 % of its commands written ran (Sonnet 50 %, Haiku 33 %), and in 3 of 7 cells its
  first script ran to the end (*Opus t3 r0*, *t9 r0*, *t10 r0*, each leaving only the generators' holes). It writes
  fewer commands per cell (71, against 87 and 155): fewer of them are thrown away by a stop.
- **Its refusals are different in kind.** 13: `alder test` with holes left 2, a body (a failing test) 3, view
  precondition 2, order 2, lambda 1, non-hole 1, `Layout` name 1, format 1 (two script blocks, *t7 r0*). No `Model` of a
  generated page (Haiku 5), no unqualified type, no quoting, no block into `?expected`, and its bodies fail on a test,
  not on syntax: *Opus t10 r1* the generated "refuses an empty" tests, *t3 r1* the generated `slug-1` test, *t7 r3* three of
  the base blog's tests, against Haiku's `,` after a block's last expression (×2), E0028 (×2) and an own-name
  qualification, which are slips inside one expression.
- **It hits the same walls once.** `new decl Layout <Upper>` in T4 (r0), `new form`/`new msg` on a one-line view (T4 r1, T6
  r0), the lambda (T7 r1), a page used before it exists (T4 r3, T5 r0). It wrote the `?afterLogin` fill by hand in all 5 of
  the cells that needed it (no `edit auth --to`), and the lower-case `Layout.shell` workaround for T4's theme.
- **It reads output more:** `alder check` 12, `alder test` 12, `alder fill --list` 10 times, `--json` twice
  (`fill --list --json`, *t6 r0*, *r1*), against 6 / 9 / 9 and 1 / 4 / 1. In a script that is a probe for the next
  round, not for the same one.
- **Where it is slowest:** T4 passes at the last allowed round (r0 `Layout` name, r1 view precondition, r2 non-hole, r3
  order, then 15 commands in r4 and 5 in r5), and T7 takes 5 (r0 two blocks, r1 lambda, r2 a 35-line `Admin.view`, r3 three
  base tests hidden by `published`).
- **T2:** 0 / 1, void (§5); it spent r1–r5 on the base: a rewrite of the Bulk updates, `regen admin` twice, `remove
  page`, then six `--help`.

## 8. The stall table (the 66 refusals outside T2, hand-classified; T2's 16 are §5)

Classes as run 1 (§6): **missing** (no command does it), **misused** (a wrong flag, argument, address or order; a fill
on a non-hole; a command run in a state it refuses), **body** (the one expression does not check, or a test the round
ran fails), **tool** (the command hung), **misread**; plus **format** (the script block) and **environment** (the T2
base). "Misused: command exists, not named" is a `new decl Layout <Upper>` refusal while `edit decl Layout.Page
add-prop` exists.

| # | Cell | Command (short) | Reason (short) | Class |
|---|---|---|---|---|
| 1 | Haiku t10 r0 | `fill --decl Teams.Index.view -` | `model session` are bound already | misused: lambda |
| 2 | Haiku t10 r1 | `fill --decl Teams.Index.Model -` | Model missing field `teams`; no field `rows`, `page`, `pages` (the --list page's load) | misused: `Model` of a generated page |
| 3 | Haiku t10 r2 | `new decl Layout Page "{ title: Text, children: List (Html a) } -> Htm...` | `Page` is not a value name (it redeclares `Layout.Page`) | misused: other |
| 4 | Haiku t10 r4 | `fill 'Teams.Index."the list shows the rows, newest first"' -` | test has no typed hole (message gives `--decl`) | misused: non-hole |
| 5 | Haiku t3 r0 | `new page Admin /admin --blank --requires user` | `--requires user` needs a session with a `user` field (`new auth` not yet run) | misused: order |
| 6 | Haiku t3 r1 | `remove page Home` | `remove page Home`: still used by Auth.Login | misused: other |
| 7 | Haiku t3 r2 | `fill --decl Admin.Model -` | Admin.Model missing `posts` (the --list page's load) | misused: `Model` of a generated page |
| 8 | Haiku t3 r3 | `fill --decl Auth.Login.update -` | `msg model session` are bound already | misused: lambda |
| 9 | Haiku t3 r4 | `fill Posts.Show.load - --hole ?load` | a `,` after a block's last expression | body |
| 10 | Haiku t3 r5 | `fill --decl Posts.Show.view -` | E0028: `<Errors field>` beside `<al-input>` | body |
| 11 | Haiku t4 r0 | `edit session add-field theme:Theme=Light` | E0201: `Theme` not imported (`theme:Theme=Light`) | misused: unqualified type |
| 12 | Haiku t4 r1 | `fill Auth.Login.update.Submit - --hole ?afterLogin` | unknown `Notes.Index` (the page comes later) | misused: order |
| 13 | Haiku t4 r2 | `new decl Layout Shell "{ title: Text, dark: Bool, children: List (Htm...` | `Shell` is not a value name (`edit decl Layout.Page add-prop` exists; not named) | misused: command exists, not named |
| 14 | Haiku t4 r3 | `fill 'App."Logout clears the user and goes to log in"' -` | test has no typed hole (message gives `--decl`) | misused: non-hole |
| 15 | Haiku t4 r4 | `fill --decl App.update -` | `msg session` are bound already | misused: lambda |
| 16 | Haiku t4 r5 | `fill --decl Notes.Index.Model -` | Notes.Index.Model missing `notes` (the --list page's load) | misused: `Model` of a generated page |
| 17 | Haiku t5 r0 | `fill --decl Notes.Index.view -` | unknown `Notes.Show` (the page comes later) | misused: order |
| 18 | Haiku t5 r1 | `fill --decl Notes.Show.Model -` | Notes.Show.Model: no field `row` (the --show page's load builds `{ row }`) | misused: `Model` of a generated page |
| 19 | Haiku t5 r2 | `fill --decl Notes.Index.view -` | E0028: `<Errors field>` beside `<al-input>` | body |
| 20 | Haiku t5 r3 | `fill --decl Notes.Show.load -` | `params session` are bound already | misused: lambda |
| 21 | Haiku t5 r4 | `alder test` | `alder test`: "the page shows the row" fails (title: False) | body |
| 22 | Haiku t5 r5 | `fill 'Notes.Show."the page shows the row"' -` | test has no typed hole (message gives `--decl`) | misused: non-hole |
| 23 | Haiku t6 r0 | `fill 'Line."an order total sums its lines"' -` | `Line.orderTotal` inside `Line` (E0002: unqualified) | body |
| 24 | Haiku t6 r1 | `fill Auth.Login.update.Submit - --hole ?afterLogin` | unknown `Orders.Index` (the page comes later) | misused: order |
| 25 | Haiku t6 r2 | `fill --decl Orders.Index.Model -` | Orders.Index.Model: expected `List Row`, got `List Order` (the --list page's load) | misused: `Model` of a generated page |
| 26 | Haiku t6 r3 | `fill --decl Orders.Index.view -` | `model session` are bound already | misused: lambda |
| 27 | Haiku t6 r5 | `fill 'Orders.Index."pressing Create"' -` | `?expected` is a Bool, not a test's whole block | misused: block in `?expected` |
| 28 | Haiku t7 r0 | `fill --decl Posts.Show.load -` | a `,` after a block's last expression | body |
| 29 | Haiku t7 r1 | `(no command; the script did not parse)` | an apostrophe in the double-quoted address: the script does not parse ("an unterminated '") | misused: quoting |
| 30 | Haiku t9 r0 | `fill Rooms.Index.view - --hole ?view` | unknown `Rooms.Show` (the page comes later) | misused: order |
| 31 | Haiku t9 r1 | `fill Bookings.Index.view - --hole ?view` | unknown `Bookings.Show` (the page comes later) | misused: order |
| 32 | Haiku t9 r2 | `fill 'Rooms.Show."submitting NewBooking"' -` | `?expected` is an Answer, not a test's whole block | misused: block in `?expected` |
| 33 | Haiku t9 r3 | `new msg Bookings.Show Cancel` | `new msg`: `view` is a hole | misused: view precondition |
| 34 | Haiku t9 r4 | `alder test` | `alder test`: no tests ran (a hole left: `?expected`) | misused: test with a hole |
| 35 | Sonnet t10 r0 | `fill Teams.Index.load -` | `params session` are bound already | misused: lambda |
| 36 | Sonnet t10 r1 | `fill Auth.Login - --hole ?afterLogin` | `fill Auth.Login`: names no module of this app | misused: other |
| 37 | Sonnet t10 r2 | `new form Teams.Index NewTeam name:Text.NonEmpty --msg Create` | `new form`: `view` is not one element over several lines | misused: view precondition |
| 38 | Sonnet t10 r4 | `fill 'Teams.Show."the owner cannot remove themselves"' -` | test body: record where `Team` expected (E0003) | body |
| 39 | Sonnet t3 r0 | `fill Posts.Index.load -` | `params session` are bound already | misused: lambda |
| 40 | Sonnet t3 r1 | `fill Admin.Index.update.Create -` | arm has no typed hole (message gives `--decl update`) | misused: non-hole |
| 41 | Sonnet t3 r2 | `alder test` | `alder test`: "NewPost refuses an empty title" fails (FormRejected field `""`) | body |
| 42 | Sonnet t4 r0 | `new decl Layout Frame "{ title: Text, theme: Theme.Theme, children: L...` | `Frame` is not a value name (`edit decl Layout.Page add-prop` exists; not named) | misused: command exists, not named |
| 43 | Sonnet t4 r1 | `fill Layout.shell -` | unknown `App.SwitchTheme` (the message comes later) | misused: order |
| 44 | Sonnet t4 r3 | `fill --decl Notes.Index.load -` | `params session` are bound already | misused: lambda |
| 45 | Sonnet t4 r4 | `new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add` | `new form`: `view` is not one element over several lines | misused: view precondition |
| 46 | Sonnet t5 r0 | `fill Notes.Index.load -` | `params session` are bound already | misused: lambda |
| 47 | Sonnet t6 r0 | `new page Orders.Index '/' --blank --requires user` | `--requires user` needs a session with a `user` field (`new auth` not yet run) | misused: order |
| 48 | Sonnet t6 r1 | `fill Orders.Index.load -` | `params session` are bound already | misused: lambda |
| 49 | Sonnet t6 r2 | `new msg Orders.Index Create` | `new msg`: `view` is not one element over several lines | misused: view precondition |
| 50 | Sonnet t6 r4 | `fill Products.Index.update.Add -` | arm has no typed hole (message gives `--decl update`) | misused: non-hole |
| 51 | Sonnet t7 r0 | `fill --decl Posts.Index.load -` | `params session` are bound already | misused: lambda |
| 52 | Sonnet t7 r1 | `alder test` | `alder test`: no tests ran (a hole left: `?expected`) | misused: test with a hole |
| 53 | Sonnet t9 r0 | `alder test` | `alder test`: no tests ran (holes left: `?afterLogin`, `?expected` x2) | misused: test with a hole |
| 54 | Opus t10 r0 | `alder test` | `alder test`: no tests ran (holes left, 6; `alder check` had printed "6 typed holes left") | misused: test with a hole |
| 55 | Opus t10 r1 | `alder test` | `alder test`: "NewTeam refuses an empty name", "NewInvite refuses an empty email" fail (FormRejected field `""`) | body |
| 56 | Opus t3 r1 | `alder test` | `alder test`: "NewComment refuses an empty author" fails (GET /posts/slug-1: not found) | body |
| 57 | Opus t4 r0 | `new decl Layout Themed "{ title: Text, theme: Theme.Theme, children: ...` | `Themed` is not a value name (`edit decl Layout.Page add-prop` exists; not named) | misused: command exists, not named |
| 58 | Opus t4 r1 | `new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add` | `new form`: `view` is not one element over several lines | misused: view precondition |
| 59 | Opus t4 r2 | `fill Notes.Index.update.Add -` | arm has no typed hole (message gives `--decl update`) | misused: non-hole |
| 60 | Opus t4 r3 | `fill --decl Notes.Index.view -` | unknown `Notes.Show` (the page comes later) | misused: order |
| 61 | Opus t5 r0 | `fill Notes.Show.view -` | unknown `Notes.Index` (the page comes later) | misused: order |
| 62 | Opus t6 r0 | `new msg Orders.Index Create` | `new msg`: `view` is not one element over several lines | misused: view precondition |
| 63 | Opus t7 r0 | `(no command; the script did not parse)` | "your answer has 2 script blocks; send exactly one" | format |
| 64 | Opus t7 r1 | `fill --decl Posts.Index.load -` | `params session` are bound already | misused: lambda |
| 65 | Opus t7 r3 | `alder test` | 3 of the base's `Posts.Show` tests fail (their posts are unpublished: `published: Bool = False`; "not found") | body |
| 66 | Opus t9 r0 | `alder test` | `alder test`: no tests ran (holes left, 3; `alder check` had printed "3 typed holes left") | misused: test with a hole |

**Counts.**

| Class | Haiku | Sonnet | Opus | Total |
|---|---|---|---|---|
| missing | 0 | 0 | 0 | **0** (run 1: 6) |
| misused | 28 | 17 | 9 | **54** (run 1: 49) |
| — lambda over bound parameters | 5 | 6 | 1 | 12 |
| — order (a name or `new auth` not yet made) | 6 | 2 | 2 | 10 |
| — fill on a non-hole (a test, an arm) | 3 | 2 | 1 | 6 |
| — `new form`/`new msg` precondition on the view (one-line view ×5, a hole `view` ×1) | 1 | 3 | 2 | 6 |
| — `Model` of a generated page | 5 | 0 | 0 | 5 |
| — `alder test` with a hole left | 1 | 2 | 2 | 5 |
| — `new decl Layout <Upper>` (command exists, not named) | 1 | 1 | 1 | 3 |
| — other (`new decl Layout Page`, `remove page Home` still used, `Auth.Login` as an address) | 2 | 1 | 0 | 3 |
| — a block into a test's `?expected` | 2 | 0 | 0 | 2 |
| — quoting (an apostrophe in a quoted address) | 1 | 0 | 0 | 1 |
| — an unqualified type in a generator argument | 1 | 0 | 0 | 1 |
| body | 6 | 2 | 3 | **11** (run 1: 8) |
| format | 0 | 0 | 1 | **1** |
| tool | 0 | 0 | 0 | **0** (run 1: 1) |
| misread | 0 | 0 | 0 | **0** |
| environment (T2 base, §5) | 5 | 5 | 6 | 16 |

## 9. H1–H3 with run 2's data

**H1, structure by construction: holds for the app, with run 1's qualifier.** 95 of 95 rounds outside T2 left an app
that type-checks (Haiku 39, Sonnet 31, Opus 25); T2's 18 rounds did not because the base did not (§5). In `files`,
Opus's round 0 failed `alder check` in 2 of 8 cells (T2's base, T4's missing import) and Haiku+Sonnet's in 7 of 16 (run
1). The model still makes structural slips, now as refusals: an import ×1 (*Haiku t4 r0*), a name before the command that
makes it ×6 (*Haiku t4 r1, t5 r0, t6 r1, t9 r0, t9 r1; Sonnet t4 r1*), a name's case or a name taken ×3 (`new decl
Layout <Upper>` ×2, `new decl Layout Page`) = **10 of the 665 commands run** (run 1: 13 of 606); Opus 3 of 334.

**H2, fewer rounds, same behaviour: refuted on rounds and pass rate, for all three models.** Not one of the 16 valid
`cli` passes took fewer rounds than the same model's `files` cell (the least gap: *Opus t9*, 2 against 1). Pass: Haiku+Sonnet
9 / 14 outside T2 against 14 / 14, hidden 69 / 94 against 88 / 94, mean 4.4 rounds against 1.4. Behaviour: for **Opus**
the apps are as good (47 / 47 hidden in both, mutation 0.80 against 0.78) at 2.8 times the rounds (25 against 9 over
the seven valid tasks); Sonnet is as good on every cell but one (T4: its harness pass has 0 / 8 hidden); Haiku is not
(2 of 7). Against run 1 the pass rate moved down for Haiku (6 / 7 → 2 / 7 outside T2) and up for Sonnet (4 / 7 → 7 / 7)
with the same kinds of refusals, which one sample cannot separate from the 6-round budget's edge.

**H3, the command set is the ceiling: stronger than in run 1, and still not the whole story.** Every one of the 82
refusals is classifiable (66 + 16 for T2's base); **"command missing" is 0 of 82** (run 1: 6 of 64): the `App`
message and the form on a message with parameters were used at first reach, 3 times each, and never refused; only
the `Layout` prop was met by the old refusal (3 cells), because its new command has a name the models never saw.
What remains is the first script's quality: 66 of 95 rounds outside T2 ended in a refusal (Haiku 87 %, Sonnet 61 %,
Opus 52 %), 54 of the 66 being a command misused. And one gap run 1 could not see: **no command repairs an app that
fails `alder check`** (T2). All three verdicts hold under the stale prompt of the header, 2: they say what the
binary does for a model that has not been told about it.

### The gap list for run 3

| # | Gap | Evidence | Proposed |
|---|---|---|---|
| 1 | The harness: the prompt and the base | header 1–2; §5 | `python3 experiments/build_context.py --reference > experiments/e11-cli/prompt-reference.md`, run on the tree the binary is built from (at `edbb667`: +1,102 bytes; at `HEAD` +5,151, the rest M7b's, which would change the prompt for a second reason): the lambda line, `new msg App`, `edit auth --to`, `edit decl … add-prop`, `new form … --msg`, the one-line view, `--blank` first, `new auth` before `--requires user`, the `--list` page's `Model` and `load`, `alder test` last; `alder-r3` built from that same commit (record `alder version`); `run_loop.py` runs `alder check` on a `--base` before round 0 and refuses a base with errors; re-run the 24 cells |
| 2 | `new decl Layout <Upper>`: the command exists, the refusal does not point to it | *Haiku t4 r2*, *Sonnet t4 r0*, *Opus t4 r0*; `add-prop` 0 uses | the message: "`Shell` is not a value name; to add a prop to the component `Layout.Page`: `alder edit decl Layout.Page add-prop <name>:<Type>=<default>`" |
| 3 | `fill --decl X.Model` on a page whose `load` was generated | 5 refusals (*Haiku t10 r1, t3 r2, t4 r5, t5 r1, t6 r2*) | the message when the errors name the generated `load`: "`load` was generated for the model `{ rows, page, pages }` (`{ row }` for `--show`): fill `load` in the same script, after `Model`, or start with `--blank`" |
| 4 | `new form`'s "refuses an empty X" test assumes the field's own error list | *Opus t10 r1* (×2), *Sonnet t3 r2*; run 1 *Sonnet t5 r5* | `alder test`'s failure for a generated test whose actual is `FormRejected [{ field: "", message: "x must not be empty" }]` and expected has `field: "x"` adds: "this page has no `<Errors field="x">`, so the error is shown in `<Errors form />` as `x must not be empty` (book 10): add the field's `<Errors>` or expect `field: ""`"; or the generator writes the expectation after the view exists |
| 5 | `new form` on a page with a route parameter writes a test that opens a row it never inserted | *Opus t3 r1*, *Sonnet t3 r2*; run 1 *Sonnet t3 r3* | the "refuses an empty …" test inserts the page's row, as `--show` pages' generated tests do |
| 6 | A generated `--list` paging test after a replaced `load` | *Haiku t10 r5* | the reference line (3 above covers the message); no command |
| 7 | `alder test` refuses for a hole in an unrelated test | 5 rounds end on it (*Haiku t9 r4*, *Sonnet t7 r1, t9 r0*, *Opus t10 r0, t9 r0*) | a design question, not a fix: should `alder test` run the hole-free tests and report "N tests not run (holes)", exiting 1 only on a failure? The book says a hole stops `alder test`. If not: the format page names the two causes (the generators' `?expected` and `?afterLogin`) beside "run it last" |
| 8 | Order: a view names a page not yet made; `--requires user` before `new auth` | 10 (8 Haiku+Sonnet, as in run 1) | the reference lines of 1; if the count is unchanged in run 3 the next step is a command (`new page --blank` for every page in one call, or a fill that stubs the missing page), not a line |
| 9 | One-line view refuses `new form`/`new msg` | 6 | the reference line of 1; the refusal already names the fix |
| 10 | Nothing repairs an app that fails `alder check` | §5 | not needed once 1 holds; if wanted, `alder check --fix` applying the JSON `fix` entries (E0201's `import Notice`; E0028 has none) |

Run 1's second-table items that the data no longer shows: `--param` with `--show` (0), an unquoted test address (0),
the `fill` hang (not exercised, §4.7).

## 10. Threats to validity

- **One sample per cell, three models.** A difference of one round or one hidden test between two cells is noise. Haiku's
  fall (7 / 8 → 2 / 8) and Sonnet's rise (5 / 8 → 7 / 8) are on a 6-round budget: three Haiku cells went from 4 rounds to
  never passing, a pass in the sixth round (T6) became a fail in the sixth. They are not evidence that the fixes help or hurt
  either model.
- **The system prompt is run 1's** (header, 2). Run 2 does not test the reference lines of the fix batch (the lambda
  line, the `Layout` prop, the order rules, `edit auth --to`); §4.1–4.2 are about the *refusal
  messages* and the *commands*, which the models met by trying them. A claim that "the reference did not help" would be
  wrong; the reference was not given.
- **The files condition was not re-run for Haiku and Sonnet.** Their `files` cells are run 1's `s1`, with the binary of
  run 1 (`87c7b42`, so the older `check` messages), T4's text *without* the Names block, and T2 on the older `examples/todos`
  (which checked). Opus's `files` cells ran on `alder-r2`, with the new T4 text and the base with 4 errors, so the
  Opus-vs-Opus comparison is like for like, the Haiku/Sonnet `files` columns differ from run 2's `cli` in the binary and
  in T4's text. The system prompt is the same file in every cell of both conditions, so the fix batch's reference
  changes are *not* a confound for `files` (they changed nothing a model saw); the batch's binary changes (messages, the
  new commands) reach a file-mode cell only through `alder check`'s text.
- **T4's text differs between the runs.** Run 2's T4 has the Names block; the comparison of T4 across runs is not like
  for like (and the `files` T4 hidden 5 / 8 of run 1 does not carry over: Opus `files` T4 is 8 / 8).
- **T2 is void** (§5): 3 of 24 cells, 18 of 113 rounds. The headline gives both counts.
- **The same T10 worked example** (the first ten commands of the T10 drill in `prompt-cli.md`) is in every `cli` prompt, as
  in run 1; T10 was not the easiest task (Haiku 6 rounds, never passing; Sonnet 6; Opus 3).
- **The harness's pass is the model's own tests.** *Sonnet t4* passes with a missing page (§2). The stop rule is strict
  by design: a round cut short never passes even when its app does, and a last-command `alder test` refused for a hole
  (5 rounds) ends the round with every other command run.
- **One hand-scorer.** §8's classes were assigned by one reader. Where two classes could fit (a refusal for a hole
  in the view, "misused: view precondition" or "order"), the sentence the command gave decided.
- **Model snapshots.** The `claude` CLI was 2.1.296 in both runs (`summary.json` `claude_version`), but `haiku`,
  `sonnet` and `opus` are aliases the CLI resolves; two days apart a snapshot may differ, and this is not checked.
- **The binary's label** (`devel+4c0e289`) is not the batch's last commit (`edbb667`): it adds two documents (header).

## 11. What to do next

1. **Run 3 = run 2 with the reference.** Gap 1: regenerate `prompt-reference.md`, build `alder` from the same commit, preflight the
   base, and re-run the 24 `cli` cells (and Haiku's and Sonnet's `files` cells with the same binary and T4 text, so the
   three conditions share everything but the mode). The prediction to test: the lambda, order and one-line-view
   refusals fall; `edit decl … add-prop` and `edit auth --to` are used; T4 passes for Sonnet with its hidden tests.
2. **Gaps 2–5** (two messages, two generator tests), each with a drill line, before or with run 3: they are cheap and
   each has a named cell.
3. **Gap 7** is a design decision (a hole stops `alder test`), taken before the book's chapter 10 sentence is relied on
   again.
4. **Then the design's option B**, one command per turn reading `--json`: 66 of 95 rounds ended in a refusal that threw
   away the rest of the script, 4 `--json` uses and nine `--help` probes show where the models would have read the
   output.
