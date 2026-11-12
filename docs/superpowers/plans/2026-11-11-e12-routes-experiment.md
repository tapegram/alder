# E12: AI usability after routes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the experiment material (task texts, hidden tests, reference solutions, harness) to routes, pass the gate at one pinned commit, run Haiku, Sonnet and Opus on thirteen tasks in `files` mode, and report the results against E7–E11.

**Architecture:** Everything E12 owns lives in `experiments/e12-routes/`; the canonical texts in `experiments/tasks/` are rewritten in place. The harness (`run_loop.py`, `report_loop.py`, `build_context.py`) gets route-era fixes first. The gate (`verify.sh`) proves every solution checks, builds, passes its own and the hidden tests and scores ≥ 0.9; the commit where it passes is `E12_COMMIT`. The binary and prompt are built from `git archive E12_COMMIT`, and `run-matrix.sh` checks that pin before every invocation and stops itself before 08:45 on weekdays.

**Tech Stack:** Python 3 (`unittest`), bash, Go (to build `alder`), Alder v0.2.0, the `claude` CLI (the user's subscription).

**Spec:** `docs/superpowers/specs/2026-11-11-e12-routes-experiment-design.md` (issue tapegram/alder#208). Read it before any task; every task argues from it.

## Global Constraints

- Everything lands on `main`, one commit per task (or more), no long-lived branches. Do not push unless the session's handover asks for it.
- **Working hours:** no model run, and no agent or background work, 09:00–17:00 America/New_York Monday to Friday; weekends are free. Check with `TZ=America/New_York date '+%u %H:%M'` (`%u` 1–5 is a weekday) before starting any task.
- Prompt budget: `build_context.py --reference` (with the example) stays ≤ 73,500 tokens (len/4), the cap in `test_build_context.py`.
- Models are the exact ids `claude:claude-haiku-5-5`, `claude:claude-sonnet-5-5`, `claude:claude-opus-5-5`; run directories `runs/claude-claude-haiku-5-5/<task>-s1/` etc.
- Cells: 13 tasks × 3 models × 2 samples (`s1`, `s2`) = 78; `--retries 5`; `--condition files`; no `--feedback`, no `--adapters`, no `--cli`.
- Hidden test counts are fixed: T4 8, T5 8, T6 9, T7 7, T9 7, T10 8, T11 9 (56); test names stay as E8/E9 wrote them.
- A hidden test is never loosened to let a solution pass, and never changed after the first model run.
- The old directories `experiments/e8-hidden/`, `e9-feedback/`, `e11-cli/` are not modified.
- The book, the checker and the generators are not changed by this plan; fixes become issues (Task 21).
- Table modules: T4–T7 keep the module names their old texts and hidden tests use (`app/User.alder`, module `User`; `app/Note.alder`, …); T9–T11 and the new T12–T15 use `app/Table/<T>.alder`, module `Table.<T>` (the drills' hidden tests, copied as they are, import `Table.User`, `Table.Room`, …).
- Vocabulary of the port: page module → route module with its GET route (`route Index = GET "/" { } requires user`); `msg M` → write route `route M` (method and path the model's choice, never fixed in a Names block); `msg M form F` → `route M form F`; `Redirect (X {})` → `Response.redirect (X.R {})`; `Rerender` → `Response.refresh`; hidden-test answers use chapter 10's `Redirected`, `Refreshed`, `Rendered`, `FormRejected`, `PageMissing`.
- `Auth.Login` in every Names block: `route Login = GET "/login" { }` and `form Credentials`; its write route is not named (the example calls it `Submit`, `alder new auth` writes `SignIn`); no hidden test names it (they log in with `Test.login`).
- `App`'s write routes keep their old names (`App.Logout`, `App.SwitchTheme`).

## Review Focus

1. **A cell cut off mid-run** (a crash, Ctrl-C, a reboot) leaves `runs/<model>/<task>-<s>/` without `summary.json`; a re-run must delete that directory and run the cell again, never skip it or mix rounds. Test: Task 16, `test_partial_cell_is_rerun`.
2. **A run spanning the weekday boundary** (started Sunday 22:00, still going Monday 08:45; or Friday evening into Saturday) must stop before the first cell at or after `STOP_AT` on Monday and must not stop on Saturday. Test: Task 16, `test_stop_at_weekday_only`.
3. **Route declarations as people write them**: aligned spacing (`route Show   = GET    "/todos/{id}"`), `requires user, org`, a write route with params and a form, a commented-out `-- route X`, and a Names-block route with no method or path (`route Rename { id: Item.ItemId } form NewName`). Test: Task 1, `test_names_of_routes`.
4. **A route module with two GET routes** (T12's `Find` and `Results`, T15's `Index` and `Row`), the extra one written first by the model: the page check must still find the Names block's GET route by path. Test: Task 1, `test_names_check_second_get_route`.
5. **A stale pin**: a material file edited after the gate, or a prompt built from HEAD instead of `E12_COMMIT`. The preflight must abort with the path named. Test: Task 16, `test_preflight_refuses_changed_input`.

---

### Task 1: Names check for routes, in every mode (suggested model: Sonnet)

**Files:**
- Modify: `experiments/run_loop.py` (`_names_of`, `_empty_names`, `names_check`, and `run_one` near line 1760)
- Modify: `experiments/report_loop.py` (the `Names` column, line ~71)
- Modify: `experiments/lint.py` (one docstring line)
- Modify: `experiments/README.md` (the taxonomy's ROUTE row; the `--cli` paragraph's names-check sentence)
- Test: `experiments/test_run_loop.py`, `experiments/test_cli.py`

**Interfaces:**
- Produces: `_names_of(module: str, text: str, decls: dict, line_start: bool) -> None` filling `decls = {"tables": {T: [cols]}, "session": [cols] | None, "pages": {module: path}, "routes": {module: {name: {"method": str|None, "path": str|None, "get": bool}}}, "forms": {module: set}}`. `pages[module]` is the path of the module's first GET route; every other route (write routes and further GET routes) is in `routes[module]`. A Names-block route without `= METHOD "path"` has `method`/`path` `None` and `get: False`.
- Produces: `names_check(task_text, app_dir) -> {"ok", "matches", "mismatches"} | None`, strings: `page Notes.Index /`, `page Notes.Index /: missing` (or `: /other`), `route Notes.Index.Delete (DELETE /notes/{id})` (the app's method and path, reported), `route Notes.Index.Delete: missing`, `form Notes.Index.NewNote`, `form …: missing`, `table …`, `session` as today.
- Produces: `summary["names"]` on every run with a Names block (all modes), computed on the final round's app (`run_dir/round<N>/` in files mode, `round<N>/<app>/` in cli mode as today); `summary["cli"]["names"]` stays.

- [ ] **Step 1: Write the failing tests** in `test_run_loop.py` (class `NamesTest`):
  - `test_names_of_routes`: app-side text with `route Show   = GET    "/todos/{id}" { id: Todo.TodoId }               requires user`, `route Save   = PUT    "/todos/{id}" { id: Todo.TodoId } form EditTodo requires user`, `route Index = GET "/projects" { } requires user, org`, `-- route Ghost = POST "/x" { }`, `form EditTodo = { title: Text.NonEmpty }` gives `pages == {"M": "/todos/{id}"}`, `routes["M"]["Save"] == {"method": "PUT", "path": "/todos/{id}", "get": False}`, `routes["M"]["Index"]["get"] is True`, no `Ghost`, `forms["M"] == {"EditTodo"}`. Task-side bullet text ``` `route Rename { id: Item.ItemId } form NewName` ``` (line_start False) gives `routes["M"]["Rename"] == {"method": None, "path": None, "get": False}` and `forms["M"] == {"NewName"}`.
  - `test_names_check_second_get_route`: a Names block naming `route Find = GET "/contacts" { q: Text = "" } requires user` and `route Results = GET "/contacts/results" { q: Text = "" } requires user` in `Contacts.Index`; an app declaring `Results` before `Find`: `"page Contacts.Index /contacts"` in matches and `"route Contacts.Index.Results (GET /contacts/results)"` in matches.
  - `test_names_in_files_summary`: a fake-model `run_one` (the existing `FakeModel` pattern) on a task with a Names block writes `summary["names"]` with `ok` and `matches`; a task without one writes `summary["names"] is None`.
  - In `test_cli.py` `test_names_check`: the `App.alder` fixture becomes `route Logout = DELETE "/session" { }`; assert `"route App.Logout (DELETE /session)" in r["matches"]`, `"page Notes.Index /: missing"` and `"form Notes.Index.NewNote: missing"` in mismatches. Its task text is the ported `t5-ownership.md` (Task 7); until then use an inline Names block with the same bullets.
  - `report_loop` test (in `test_run_loop.py`'s report tests): a files summary with `names` shows the `Names` column (`ok` / `N differ`).

- [ ] **Step 2: Run them to see them fail**

Run: `cd experiments && python3 -m unittest test_run_loop test_cli -k names -v`
Expected: FAIL (`KeyError: 'routes'`, no `summary["names"]`).

- [ ] **Step 3: Implement.** Replace the `page "…"` and `msg` regexes with one route regex (port of `cmd/alder/drill_test.go`'s mapping):

```python
ROUTE_RE = r'route\s+([A-Z]\w*)(?:\s*=\s*(GET|POST|PUT|PATCH|DELETE)\s+"([^"]*)")?'
```
prefixed by the existing anchor `a`. The first GET route per module fills `pages`; the rest go to `routes`. In `names_check`, a task's page matches when any GET route of that module in the app (first or in `routes` with `get: True`) has the path; each named route matches by name, its app method/path appended in parentheses when it has one. `run_one`: compute `names` for every mode, store `summary["names"]`. `report_loop.py`: read `c.get("names")` from `summary["cli"]` or `summary`, and show the column when any summary has either. `lint.py`: add to the module docstring "Predates routes (#160): it scores the page/msg model and is not used by the loop." `README.md`: ROUTE row becomes `| ROUTE | route/form wiring | \`href\` with a string, a write route's params missing, \`update\` on a GET route, \`form\` field mismatch, \`Params\` written by hand |`; the `--cli` paragraph's `names` sentence says each named GET route's path, each named route and form present, method and path reported, and that `summary.names` is recorded in every mode since E12.

- [ ] **Step 4: Run the whole Python suite**

Run: `cd experiments && python3 -m unittest test_run_loop test_cli test_build_context`
Expected: OK.

- [ ] **Step 5: Commit** `experiments: names check reads routes, recorded in every mode (#208)`.

### Task 2: The reference's command table and three drops (suggested model: Sonnet)

**Files:**
- Modify: `experiments/build_context.py` (`COMMANDS`, `DROP_SUBSECTIONS["10"]["12. Properties"]`, its comment)
- Test: `experiments/test_build_context.py` (`test_real_reference_keeps_properties`, `test_real_reference_cut`, a new `test_real_reference_commands`)

**Interfaces:**
- Produces: `build_context.py --reference` output with the route-era table; consumed by Task 17's prompt.

- [ ] **Step 1: Write the failing tests.**
  - `test_real_reference_keeps_properties`: move `"### Reading a failure"`, ``"### `--fuzz`, `--seed` and saved failures"``, `"### Health checks"` from the kept list to the dropped list; `"### The function form"` and `"### Where the inputs come from"` stay kept.
  - `test_real_reference_commands` (full reference, all default chapters, with the example): contains `"alder new route "`, `"alder new action "`, `"--msg <Route>"`, `"alder edit route "`, `"add-param"`, `"alder remove route "`; contains none of `"alder new page "`, `"alder new msg "`, `"alder edit page "`, `"alder remove page "`.
  - `test_real_reference_cut`: the cap stays `73500`; add a comment line "E12 (#208): the COMMANDS table on routes, paid for by three drops in chapter 10's Properties".

- [ ] **Step 2: Run them to see them fail**

Run: `cd experiments && python3 -m unittest test_build_context -v`
Expected: FAIL on the new assertions.

- [ ] **Step 3: Rewrite `COMMANDS`** from `alder new route --help`, `alder new action --help`, `alder new form --help`, `alder edit --help`, `alder remove --help` (v0.2.0): `new route` (its `--method`, `--form`, `--list/--show/--from`, `--blank`, `--param`, `--requires`, `--role`), `new action` (`--in-for`, `--when`, `--method`, `--path`), `new form … --msg <Route>`, `edit route <Route> add-param`, `remove route`; reword the page-era `--in-for` and `--blank` lines for routes; keep every other line. Add the three headings to `DROP_SUBSECTIONS["10"]["12. Properties"]` and update the comment above it. If the cap still fails, add a `TRIMS` entry rather than raising the cap.

- [ ] **Step 4: Run the suite and record the size**

Run: `cd experiments && python3 -m unittest test_build_context && python3 build_context.py --reference --size`
Expected: OK; `estimate (len/4)` ≤ 73500.

- [ ] **Step 5: Commit** `experiments: route-era command table; drop three Properties subsections from the reference (#208)`.

### Task 3: E12 skeleton, pin helper, drill replay, verify.sh (suggested model: Sonnet)

**Files:**
- Create: `experiments/e12-routes/tasks` (symlink `../tasks`), `experiments/e12-routes/pin.sh`, `experiments/e12-routes/replay_drill.py`, `experiments/e12-routes/verify.sh`
- Create: `experiments/e12-routes/hidden/{t9-bookings,t10-teams,t11-workspace}/Hidden.alder` (copies of `testdata/drill/<task>/Hidden.alder`, byte for byte)
- Create: `experiments/e12-routes/reference/{t9-bookings,t10-teams,t11-workspace}/` (drill-built apps)

**Interfaces:**
- Produces: `pin.sh binary COMMIT` → builds `experiments/e12-routes/alder` from `git archive COMMIT` in a temp dir (`go build -o … ./cmd/alder`), prints `sha256 <hex>` and `alder version`'s line. `pin.sh prompt COMMIT OUT_DIR` → in a fresh export of COMMIT runs `python3 experiments/build_context.py --reference > OUT_DIR/prompt-reference.md` and `--reference --size > OUT_DIR/sizes.md`. Both remove their export.
- Produces: `replay_drill.py ALDER SCRIPT APP_DIR` → `alder new <name> --bare` in the parent, then each step via `run_loop.parse_script` + `run_loop.execute_script(alder, steps, app_dir, cap=10_000)`; exits 1 naming the first failed line.
- Produces: `verify.sh [task …]` with E9's columns plus `names` (`ok` or `N differ`, from `run_loop.names_check` on the solution); `ALDER` defaults to `experiments/e12-routes/alder`.

- [ ] **Step 1: Build the prep binary.** Run: `experiments/e12-routes/pin.sh binary HEAD`. Expected: `alder v0.2.0 …` and a sha256 line; `experiments/e12-routes/alder` is git-ignored (`git status --short` shows nothing for it).

- [ ] **Step 2: Replay the drills** into `reference/<task>/` with `replay_drill.py` (app names as `drill_test.go`'s `drills`: `bookings`, `teams`, `ws`; copy the resulting `app/` only, no `build/`). Expected: exit 0 each.

- [ ] **Step 3: Write `verify.sh`**, E9's script with: the task list `t2-newpage t3-newapp t4-appmsg t5-ownership t6-orders t7-drafts t9-bookings t10-teams t11-workspace t12-search t13-feed t14-board t15-pantry`; T2/T3 smoke rows (`reference/<task>` only; check, build, own; no hidden, no mutate; T2's `reference/t2-newpage/app/` holds only the files the task adds or changes, overlaid on `git archive HEAD examples/todos`); the `names` column (must be `ok` for every solution of a task with a Names block); an expected-count table (`t4-appmsg 8, t5-ownership 8, t6-orders 9, t7-drafts 7, t9-bookings 7, t10-teams 8, t11-workspace 9`) that fails a hidden module with another count; the text check reduced to: tests paragraph, output paragraph, a Names block (T4–T15), none of `page "`, `` `msg ``, `Redirect (`, `Rerender` in any text, T6 never names `Task.fail`/`Task.notFound`; E9's byte-equality with E8's `with` texts removed. Only the tasks given on the command line are text-checked.

- [ ] **Step 4: Run the gate on the drill solutions**

Run: `experiments/e12-routes/verify.sh t9-bookings t10-teams t11-workspace`
Expected: every `reference` row `ok ok n/n k/k ≥0.9`, names `differ` (texts still page-era: Task 4 fixes), each `reference-alt` row `missing`; exit 1.

- [ ] **Step 5: Commit** `e12: skeleton, pin helper, drill replays and the gate script (#208)`.

### Task 4: Port T9, T10, T11 texts (suggested model: Sonnet)

**Files:**
- Modify: `experiments/tasks/t9-bookings.md`, `t10-teams.md`, `t11-workspace.md`

**Interfaces:**
- Consumes: `reference/<task>` (Task 3) and its names; the drills' `drillNames` in `cmd/alder/drill_test.go` are the target names.

- [ ] **Step 1: Rewrite each Names block** under the Global Constraints' vocabulary: tables at `app/Table/<T>.alder` (module `Table.<T>`), each GET route in full, each write route by name, params and form, each answer as `Response.redirect (Rooms.Show.Show { id: … })`; behaviour, ids, classes, tests and output paragraphs unchanged in substance.

- [ ] **Step 2: Verify** Run: `experiments/e12-routes/verify.sh t9-bookings t10-teams t11-workspace`. Expected: text check passes; every `reference` row names `ok`; alt rows still missing.

- [ ] **Step 3: Commit** `tasks: T9–T11 on routes (#208)`.

### Task 5: T9, T10, T11 second solutions (suggested model: Opus)

**Files:**
- Create: `experiments/e12-routes/reference-alt/{t9-bookings,t10-teams}/` (port of `e9-feedback/reference-alt/<task>`), `reference-alt/t11-workspace/` (hand-written: no E8/E9 solution exists), `SURVIVORS.md` where needed

- [ ] **Step 1: Port / write** each solution with different markup and module structure from the drill's (the gate's purpose: a hidden test fixing a detail the text does not is caught). Own tests in the modules they test, scoring ≥ 0.9.
- [ ] **Step 2: Run** `experiments/e12-routes/verify.sh t9-bookings t10-teams t11-workspace`. Expected: exit 0, all six rows green, names `ok`. On a failure apply the spec's "When the gate fails" rule and note the fix for RUN.md (keep a running `experiments/e12-routes/gate-notes.md`, merged into RUN.md in Task 17).
- [ ] **Step 3: Commit** `e12: second solutions for T9–T11 (#208)`.

### Tasks 6–9: Port T4, T5, T6, T7 (suggested model: Opus, one task each)

Each task ports one old task end to end; they are independent of each other.

| Task | Text | Hidden from | Solutions from | Extra |
|---|---|---|---|---|
| 6 T4 | `tasks/t4-appmsg.md` | `e8-hidden/hidden/t4-appmsg` (8) | `e8-hidden/reference{,-alt}/t4-appmsg` | `SwitchTheme` answers `Response.refresh` |
| 7 T5 | `tasks/t5-ownership.md` | `e9-feedback/hidden/t5-ownership` (8) | `e9-feedback/reference{,-alt}/t5-ownership` | 404 stays `Task.notFound` → `PageMissing` |
| 8 T6 | `tasks/t6-orders.md` | `e9-feedback/hidden/t6-orders` (9) | `e9-feedback/reference{,-alt}/t6-orders` | `msg AddLine { product } form NewLine` becomes `route AddLine { product: Product.ProductId } form NewLine` (the corrected cut, named in findings) |
| 9 T7 | `tasks/t7-drafts.md` | `e9-feedback/hidden/t7-drafts` (7) | `e9-feedback/reference{,-alt}/t7-drafts` | port `e8-hidden/base/blog-nodrafts` (11 modules, `adapters/`, `migrations/`, `go.mod`, `static/`) to `e12-routes/base/blog-nodrafts`; alt keeps E8's lossy migration (`DROPS DATA` yes) |

**Files (each):** Modify `experiments/tasks/<task>.md`; Create `experiments/e12-routes/hidden/<task>/Hidden.alder`, `reference/<task>/`, `reference-alt/<task>/` (with `SURVIVORS.md` where needed); T7 also `base/blog-nodrafts/`.

- [ ] **Step 1: Port the hidden tests** in the drills' style (`testdata/drill/t9-bookings/Hidden.alder`): route values with their full path (`Notes.Index.Index {}`), forms posted to their route (`Test.submit (Notes.Index.Add {}) {…}`), clicks on write routes (`Test.click (Notes.Index.Delete { id: n })`), chapter 10's answers. Same test names, same count, same assertions on data, ids and classes.
- [ ] **Step 2: Port the text** (Global Constraints' vocabulary); every name a hidden test uses is in the Names block.
- [ ] **Step 3: Port both solutions**, keeping them as different from each other as E8/E9 made them. T7: port the base first, then check it: `cd $(mktemp -d) && cp -R …/base/blog-nodrafts . && alder check blog-nodrafts` exits 0, and commit the base before the solutions (the harness's `--base` exports committed state).
- [ ] **Step 4: Run** `experiments/e12-routes/verify.sh <task>`. Expected: exit 0, both rows green, hidden `n/n` with the fixed count, names `ok`; T7 `drops` `no` / `yes`.
- [ ] **Step 5: Commit** `tasks, e12: T<n> on routes with its hidden tests and solutions (#208)`.

### Task 10: T2 and T3 texts and smoke solutions (suggested model: Opus)

**Files:**
- Modify: `experiments/tasks/t2-newpage.md`, `t3-newapp.md`
- Create: `experiments/e12-routes/reference/t2-newpage/app/…` (only the added or changed files), `reference/t3-newapp/app/…`

- [ ] **Step 1: Port the texts.** T2: "a page" becomes "a route module" at `/profile`; the logout sentence names `App.Logout` (`route Logout = DELETE "/session" { }` in the example) instead of "the Login page already has a Logout msg"; the rename form's write route is the model's. T3: page/msg words become routes; no Names block (as before).
- [ ] **Step 2: Write one route-era solution each** that does what the text asks, with the tests the text asks for.
- [ ] **Step 3: Run** `experiments/e12-routes/verify.sh t2-newpage t3-newapp`. Expected: both smoke rows `ok ok n/n`; exit 0.
- [ ] **Step 4: Commit** `tasks, e12: T2 and T3 on routes, smoke solutions (#208)`.

### Tasks 11–14: The new tasks T12–T15 (suggested model: Opus, one task each)

Each is a complete new app: a task text in the style of `t5-ownership.md` (bullets, **Names.** block, tests paragraph, output paragraph "Output every file, in full, …"), hidden tests, two solutions. Login (`User`, `Auth.Login`, `App.Logout`, `session { user: Maybe User.UserId }`) only in T12; T13–T15 have no users (`App` with `session { }`, no `login`). The patterns follow chapter 6's `Search`, `Feed` and `Dashboard` (`book/06-routes.md`, "Patterns").

**Files (each):** Create `experiments/tasks/<task>.md`, `experiments/e12-routes/hidden/<task>/Hidden.alder`, `reference/<task>/`, `reference-alt/<task>/`.

**Names (fixed by this plan):**

- **Task 11, T12 `t12-search`:** `table Contact { id: ContactId @id, name: Text.NonEmpty, email: Text.Email, owner: User.UserId @ref User @owner }` in `Table.Contact`; module `Contacts.Index`: `route Find = GET "/contacts" { q: Text = "" } requires user`, `route Results = GET "/contacts/results" { q: Text = "" } requires user` (view `<ul id="results">`, one `<li>` per match, the name its whole text), a field `name="q"` with `input={Results { q: … }}`, `form NewContact = { name: Text.NonEmpty, email: Text.Email }` posted by `route Add form NewContact`, answering `Response.redirect (Contacts.Index.Find {})`. Hidden (4): `"typing asks for the matching contacts only"`, `"an empty query lists every contact"`, `"another user's contacts never appear"`, `"the form adds a contact"`.
- **Task 12, T13 `t13-feed`:** `table Entry { id: EntryId @id, text: Text.NonEmpty, created: Time @created }` in `Table.Entry`; module `Feed.Index`: `route Index = GET "/feed" { }` (ten newest entries, newest first, each an `<li>` with the text as its whole text, then the sentinel), `route More = GET "/feed/more" { after: Int }` (the next ten after the first `after`, then the next sentinel; none after the last), `form NewEntry = { text: Text.NonEmpty }` posted by `route Post form NewEntry`, answering `Response.redirect (Feed.Index.Index {})`. Hidden (3), entries inserted with `Test.setClock`/`Test.advance` so `created` differs: `"two scrolls show all 25 entries and no sentinel"` (`Test.intersect (Feed.Index.More { after: 10 })`, then `{ after: 20 }`; 25 `<li>`, `Test.has "[data-on-intersect__once]"` false), `"no entry repeats"`, `"posting an entry puts it first"`.
  **Needs the user's ruling (recorded in RUN.md):** the spec leaves the cursor's type to the model (`More { before: … }`), but `Test.intersect` must be given the exact route value the page's sentinel requests, which a hidden test cannot construct for an unknown type. The plan fixes `{ after: Int }` (entries already shown, as chapter 6's `Feed`) under the spec's "a behaviour … the test cannot set up" rule. If the user prefers the spec's wording, the alternative is to drop T13's scroll test from the hidden set and record the cursor choice as a signal only.
- **Task 13, T14 `t14-board`:** `type Status = Open | Closed` and `table Ticket { id: TicketId @id, title: Text.NonEmpty, status: Status = Open, created: Time @created }` in `Table.Ticket`; module `Board`: `route Board = GET "/board" { }` rendering `<section id="recent" load={Recent {}}>` and `<span id="open" load={Open {}} every="10s">`, `route Recent = GET "/board/recent" { }` (view `<section id="recent">`, the five newest titles as `<li>`, newest first), `route Open = GET "/board/open" { }` (view `<span id="open">` whose whole text is the count), `route Close { id: Ticket.TicketId }`, a button per open ticket, answering `Response.refresh`. Hidden (3): `"the badge counts open tickets, again after an insert and after a close"` (`Test.get (Board.Board {})`, `Test.load (Board.Open {})` three times), `"recent lists the five newest of six, newest first"`, `"closing a ticket refreshes the board"` (`answer == Refreshed`). If the constructor `Open` and the route `Open` clash under the checker, the gate rule applies: rename the constructor in the text (`Pending | Closed`), recorded in RUN.md.
- **Task 14, T15 `t15-pantry`:** `table Item { id: ItemId @id, name: Text.NonEmpty @unique, quantity: Int }` in `Table.Item` (items are added outside the app, as T9's rooms); module `Pantry.Index`: `route Index = GET "/pantry" { }` (`<ul id="items">`, each item's `<li id="item-{id}">` from `Row`'s view), `route Row = GET "/pantry/items/{id}" { id: Item.ItemId }`, `form NewName = { name: Text.NonEmpty }` posted by `route Rename { id: Item.ItemId } form NewName` (answers `Response.invalid "name already taken"` when another item has the name, nothing stored; else `Response.redirect (Pantry.Index.Index {})`), `route Bump { id: Item.ItemId }` (quantity + 1, answers `Response.render (Pantry.Index.Row { id: … })`), `route Remove { id: Item.ItemId }` (answers `Response.refresh |> Response.toast "Removed"`). Each item's name is the whole text of its `.name`, its quantity of `.quantity`. Hidden (4): `"renaming to a taken name is rejected and changes nothing"` (`FormRejected`, status 422, row unchanged), `"renaming to a free name redirects to the pantry"`, `"bump adds one and renders the row"` (`Rendered (Pantry.Index.Row { id: a })`, `.quantity` text), `"remove refreshes and drops the item"` (`Refreshed`, the item absent).

- [ ] **Step 1: Write the hidden tests** with the names and assertions above.
- [ ] **Step 2: Write the task text** whose Names block carries exactly the names above (write routes by name, params and form only).
- [ ] **Step 3: Write two solutions**, different in markup and structure, with their own tests (≥ 0.9).
- [ ] **Step 4: Run** `experiments/e12-routes/verify.sh <task>`. Expected: exit 0, hidden `n/n`, names `ok`.
- [ ] **Step 5: Commit** `tasks, e12: T1<n> <name> (#208)`.

### Task 15: Docs for the port (suggested model: Sonnet)

**Files:**
- Modify: `experiments/tasks/README.md` (a paragraph: the E12 port, what changed in each text, the T6 correction, the table-module rule, T12–T15; the old wording's history kept)
- Modify: `experiments/README.md` (Tasks list: T11 and T12–T15; an E12 entry in the experiment list, "designed" form, filled in Task 19)
- Create: `experiments/e12-routes/README.md` (summary, layout as the spec's "Where E12 lives", exact commands: `pin.sh`, `verify.sh`, `run-matrix.sh`, `report_loop.py`)

- [ ] **Step 1: Write the three docs.**
- [ ] **Step 2: Verify** Run: `experiments/e12-routes/verify.sh`. Expected: exit 0 (all thirteen tasks) — the material is complete.
- [ ] **Step 3: Commit** `experiments: E12 docs (#208)`.

### Task 16: run-matrix.sh with preflight and STOP_AT (suggested model: Sonnet)

**Files:**
- Create: `experiments/e12-routes/run-matrix.sh`
- Test: `experiments/e12-routes/test_run_matrix.py` (unittest, drives the script with env overrides; no model is called)

**Interfaces:**
- Consumes: `RUN.md` lines `E12_COMMIT: <sha>` and `ALDER_SHA256: <hex>`; `pin.sh prompt` (Task 3).
- Produces: `run-matrix.sh` — cells in order sample (`s1`, `s2`) → model (haiku, sonnet, opus) → task (t2 … t15); env `MODELS`, `SAMPLES`, `TASKS` narrow it; `STOP_AT` (default `0845`); `E12_CLOCK="<u> <HHMM>"` overrides the clock (tests only); `DRY_RUN=1` prints each cell's `run_loop.py` command instead of running it; `PREFLIGHT_ONLY=1` runs the preflight and exits. Logs: `logs/<model-label>.log` (run_loop output), `logs/matrix.progress` (one line per cell: time, cell, `done`/`skipped`/`rerun`), and on a stop `STOPPED before <sample> <model> <task>` on stdout and in progress; exit 0 on complete, 3 on a stop, 2 on a failed preflight.

Per cell: flags `--exp experiments/e12-routes --model M --task T --sample S --condition files --build --test --mutate --retries 5 --prompt <abs>/prompt-reference.md --alder experiments/e12-routes/alder`; `--hidden experiments/e12-routes/hidden` for T4–T15 (not T2/T3, as E11); T2 adds `--base examples/todos`; T7 adds `--base experiments/e12-routes/base/blog-nodrafts --migrate`. A cell whose `summary.json` exists is skipped; a cell directory without one is deleted and rerun. Before each cell: stop when `%u` ≤ 5 and `STOP_AT` ≤ HHMM < 1700 in `TZ=America/New_York`; the same check refuses to start.

Preflight, before the first cell of every invocation: binary sha256 equals `ALDER_SHA256`; `alder check` exits 0 on `git archive E12_COMMIT examples/todos` and `… experiments/e12-routes/base/blog-nodrafts` (exported to a temp dir); `pin.sh prompt E12_COMMIT <tmp>` output byte-equals `prompt-reference.md`; `git diff --quiet E12_COMMIT -- <inputs>` and `git status --porcelain -- <inputs>` empty, inputs exactly the spec's list (`experiments/tasks experiments/e12-routes/{hidden,reference,reference-alt,base,verify.sh,run-matrix.sh,prompt-reference.md} experiments/run_loop.py experiments/report_loop.py experiments/build_context.py examples/todos`). `runs/`, `logs/`, `scores.md` are not checked. Each failure prints which check and path and exits 2.

- [ ] **Step 1: Write the failing tests.** Each test clones the repo into a temp dir (`git clone --local`), copies the working `run-matrix.sh` and `pin.sh` into the clone's `experiments/e12-routes/` and commits them there, writes a fixture `RUN.md` pinning the clone's HEAD, and runs with `DRY_RUN=1` unless it tests the preflight:
  - `test_cell_order_and_flags`: dry run prints 78 lines; line 1 is `s1 claude:claude-haiku-5-5 t2-newpage` with `--base examples/todos` and no `--hidden`; the T7 line has `--migrate`.
  - `test_done_cell_skipped`: a `summary.json` in `runs/claude-claude-haiku-5-5/t2-newpage-s1/` → that cell `skipped`.
  - `test_partial_cell_is_rerun`: the directory exists without `summary.json` → deleted, progress says `rerun`.
  - `test_stop_at_weekday_only`: `E12_CLOCK="1 0850"` → exit 3, `STOPPED before s1 claude:claude-haiku-5-5 t2-newpage`; `"6 0850"` and `"1 1705"` and `"5 2300"` run; `"1 0844"` runs; `STOP_AT=0800 E12_CLOCK="1 0810"` stops.
  - `test_preflight_refuses_changed_input`: after editing `experiments/tasks/t5-ownership.md` in the clone, `PREFLIGHT_ONLY=1` exits 2 naming the path; with a wrong `ALDER_SHA256` exits 2 naming the binary.
- [ ] **Step 2: Run** `python3 -m unittest experiments/e12-routes/test_run_matrix.py -v`. Expected: FAIL (no script).
- [ ] **Step 3: Write `run-matrix.sh`.**
- [ ] **Step 4: Run** the same command. Expected: OK.
- [ ] **Step 5: Commit** `e12: the matrix script, its preflight and the weekday stop (#208)`.

### Task 17: The gate and the pin (suggested model: Opus)

**Files:**
- Create: `experiments/e12-routes/prompt-reference.md`, `sizes.md`, `RUN.md`
- Delete: `experiments/e12-routes/gate-notes.md` (merged into RUN.md)

- [ ] **Step 1: Check the clock** (Global Constraints). Generate the prompt from the committed tree: `experiments/e12-routes/pin.sh prompt HEAD experiments/e12-routes` (the tree must be clean); commit `e12: prompt and sizes`. This commit is the candidate `C`.
- [ ] **Step 2: Build the run binary** `experiments/e12-routes/pin.sh binary C`.
- [ ] **Step 3: Run the gate** `experiments/e12-routes/verify.sh > /tmp/e12-gate.txt 2>&1; echo $?`. Expected: `0`, all thirteen tasks' rows green. On a failure: fix the material per the spec's rule, commit, return to Step 1 (a new `C`). Never edit a hidden test to suit a solution.
- [ ] **Step 4: Write `RUN.md`**: `E12_COMMIT: <C>`, `ALDER_SHA256: <hex>`, `alder version`, `claude --version`, the date, the reference size from `sizes.md` and its three changes against E11's (route chapters, COMMANDS, the three drops), the gate output verbatim, every gate-time fix and why (from `gate-notes.md`, including the T13 cursor ruling and any T14 rename), and an empty "Run log" section with a "Resume point" line.
- [ ] **Step 5: Preflight** `PREFLIGHT_ONLY=1 experiments/e12-routes/run-matrix.sh`. Expected: exit 0.
- [ ] **Step 6: Commit** `e12: pinned at <C>, gate green (#208)`. From here no input path changes until the run ends.

### Task 18: The model matrix run (suggested model: Sonnet, or the main session)

**Files:**
- Create/append: `experiments/e12-routes/runs/`, `logs/`, `scores.md`, `RUN.md` (Run log)

This task runs only outside 09:00–17:00 America/New_York on weekdays; weekends are free. It spans several sessions; each session does Steps 1–5 once.

- [ ] **Step 1: Check the clock.** `TZ=America/New_York date '+%u %H:%M'`. If `%u` ≤ 5 and the time is between 08:45 and 17:00, stop: do nothing until 17:00.
- [ ] **Step 2: Set the stop.** `STOP_AT=0845` (default) on any night before a weekday; lower it (e.g. `0830`) once the first cells' durations in `logs/matrix.progress` show a cell can take near a quarter hour, so no cell runs past 09:00. Record the value used in RUN.md.
- [ ] **Step 3: Run** in the foreground under a timeout that ends before 09:00 on a weekday: `STOP_AT=0845 experiments/e12-routes/run-matrix.sh`. Expected: preflight passes, then exit 0 (complete) or 3 (stopped, the next cell printed). Exit 2: stop and report the failed check; do not edit inputs.
- [ ] **Step 4: Record and commit at every stop.** `python3 experiments/report_loop.py experiments/e12-routes > experiments/e12-routes/scores.md`; append to RUN.md's Run log the date, start and stop times, cells done this session, `STOP_AT`, and set "Resume point: <sample> <model> <task>" (or "complete"); `git add experiments/e12-routes/{runs,logs,scores.md,RUN.md}` and commit `e12: run, <n>/78 cells (#208)`. The tree's inputs are untouched, so the next preflight passes.
- [ ] **Step 5: Hand over** with the resume point. Done when RUN.md says `Resume point: complete` and `scores.md` has 78 rows.

### Task 19: findings.md (suggested model: Opus)

**Files:**
- Create: `experiments/e12-routes/findings.md`
- Modify: `experiments/README.md` (the E12 entry: as run, the result in one paragraph)

- [ ] **Step 1: Extract** per cell from `summary.json`: own-test pass, rounds used, round-0 clean check, `codes` per round, hidden `k/n`, mutation score, `names`; and the same for E8/E9/E11-`files` cells of T2–T11 (their `scores.md` and summaries).
- [ ] **Step 2: Write** the spec's "What `findings.md` reports": the headline table (E12 beside E8/E9/E11-`files`, per task), round-0 `codes` old against new, a section per new task, E0045–E0047 as met, the T6 correction, the reference's three changes, and both samples' agreement per cell. Leave the signals and issue sections as headings filled by Tasks 20–21.
- [ ] **Step 3: Commit** `e12: findings (#208)`.

### Task 20: signals.md (suggested model: Opus)

**Files:**
- Create: `experiments/e12-routes/signals.md`
- Modify: `experiments/e12-routes/findings.md` (its signals section)

- [ ] **Step 1: Read** every cell's round outputs for "reasonable but not Alder" choices (the spec's list: `DELETE` written as `POST`, `render` where the rule says `refresh`, a trigger attribute on `<al-*>`, a `Response` built by hand, a path parameter in an `input=` route, and the write routes' methods the names check reports).
- [ ] **Step 2: Write** one table row per signal: cell, round, what, the book sentence it fights (file and heading), the checker's answer. Never counted in pass rates, taxonomy or hidden scores; a signal that coincides with a check error is listed here and counted there.
- [ ] **Step 3: Summarise** them in findings.md's section; commit `e12: signals (#208)`.

### Task 21: Issues (suggested model: Sonnet)

**Files:**
- Modify: `experiments/e12-routes/findings.md` (the issue list)

- [ ] **Step 1: File one GitHub issue per fix** on `tapegram/alder` (book, diagnostics, generators), each citing its cells and linking findings.md; reference #208.
- [ ] **Step 2: List** them in findings.md; comment on #208 with the summary and links.
- [ ] **Step 3: Commit** `e12: issues filed (#208)`.
