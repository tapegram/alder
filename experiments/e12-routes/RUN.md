# E12 run record

E12_COMMIT: c9478ef711364347a9cbf173e3877ea8e2abd348
ALDER_SHA256: 74c6974f40dee436bb1f090b25f21476155522faa920f86c163e92df3b0c9336

- **Pinned:** 2026-11-11 (evening, America/New_York). `E12_COMMIT` is `c9478ef7` ("e12: prompt and
  sizes"), the commit that adds `prompt-reference.md` and `sizes.md` to the tree the gate ran on
  (`9145183b` plus those two files). From this record's commit on, no input path (`INPUTS` in
  `run-matrix.sh`) changes until the model run ends.
- **Binary:** `experiments/e12-routes/pin.sh binary c9478ef7` built `experiments/e12-routes/alder`
  from `git archive c9478ef7`.
  - `alder version`: `alder (devel) (go 1.27.4)`. A build from a `git archive` export has no VCS
    stamp, so the version line does not name the commit; the binary is identified by `E12_COMMIT`
    and its sha256 (`ALDER_SHA256`, above), which the preflight checks before every invocation.
- **Harness:** `claude --version`: `2.1.309 (Claude Code)` (the design-time version).
- **Prompt:** `pin.sh prompt HEAD experiments/e12-routes` on the clean tree at `9145183b`, whose
  export builds the same prompt as `c9478ef7`'s (the two differ only by the generated files); the
  preflight rebuilds it from `E12_COMMIT` and compares it byte for byte.

## The reference prompt

`sizes.md`: **289,942 characters, 72,485 tokens** (len/4), under `test_build_context.py`'s cap of
73,500 (the spec's design-time figure was 293,865 / 73,466). E11's committed
`e11-cli/prompt-reference.md`, the prompt both E11 runs sent (`e11-cli/findings-run2.md`, "Three
things differ"), is 195,967 characters, 48,991 tokens. It differs from E11's in three ways:

1. **The route-era chapters.** The book chapters `--reference` cuts (3-10) are the post-#160
   chapters: a route module's GET route and write routes, `Response`, `input=`, `load=`,
   `intersect=`, `every=`, the chapter-10 `Answer` names (`Redirected`, `Refreshed`, `Rendered`,
   `FormRejected`, `PageMissing`); pages and messages are gone.
2. **The COMMANDS table** is rewritten for the route-era CLI (`alder new route`, `remove route`,
   ...).
3. **Three more subsections of chapter 10's "12. Properties" are dropped** to pay for that table:
   "Reading a failure", "`--fuzz`, `--seed` and saved failures" and "Health checks"
   (`DROP_SUBSECTIONS` in `build_context.py`; the failure text and the flags are met by running a
   test, not needed to write one).

## The gate

`experiments/e12-routes/verify.sh` (all thirteen tasks, `ALDER` = the pinned binary) on the tree
at `c9478ef7`: **exit 0**. Output, verbatim:

```
task          solution       check  build  own      hidden   score              names     drops
t2-newpage    reference      ok     ok     152/152  -        -                  -         -
t3-newapp     reference      ok     ok     16/16    -        -                  -         -
t4-appmsg     reference      ok     ok     17/17    8/8      26/26 1.000        ok        -
t4-appmsg     reference-alt  ok     ok     11/11    8/8      32/32 1.000        ok        -
t5-ownership  reference      ok     ok     14/14    8/8      17/17 1.000        ok        -
t5-ownership  reference-alt  ok     ok     10/10    8/8      18/18 1.000        ok        -
t6-orders     reference      ok     ok     26/26    9/9      54/54 1.000        ok        -
t6-orders     reference-alt  ok     ok     16/16    9/9      54/54 1.000        ok        -
t7-drafts     reference      ok     ok     43/43    7/7      58/61 0.951        ok        no
t7-drafts     reference-alt  ok     ok     33/33    7/7      56/59 0.949        ok        yes
t9-bookings   reference      ok     ok     22/22    7/7      47/47 1.000        ok        -
t9-bookings   reference-alt  ok     ok     13/13    7/7      42/42 1.000        ok        -
t10-teams     reference      ok     ok     28/28    8/8      49/49 1.000        ok        -
t10-teams     reference-alt  ok     ok     16/16    8/8      51/51 1.000        ok        -
t11-workspace reference      ok     ok     28/28    9/9      44/45 0.978        ok        -
t11-workspace reference-alt  ok     ok     13/13    9/9      45/46 0.978        ok        -
t12-search    reference      ok     ok     11/11    4/4      14/14 1.000        ok        -
t12-search    reference-alt  ok     ok     12/12    4/4      12/12 1.000        ok        -
t13-feed      reference      ok     ok     7/7      3/3      5/5 1.000          ok        -
t13-feed      reference-alt  ok     ok     4/4      3/3      9/9 1.000          ok        -
t14-board     reference      ok     ok     7/7      3/3      7/7 1.000          ok        -
t14-board     reference-alt  ok     ok     7/7      3/3      7/7 1.000          ok        -
t15-pantry    reference      ok     ok     10/10    4/4      14/14 1.000        ok        -
t15-pantry    reference-alt  ok     ok     10/10    4/4      17/17 1.000        ok        -

--- reference/t7-drafts: survivors
  Posts.Index   Index.load:33:17   operator flipped   params.page < 1 → params.page <= 1
  Posts.Index   Index.load:33:48   operator flipped   params.page > lastPage → params.page >= lastPage
  Posts.Index   Index.load:42      limit dropped      Db.limit (perPage + 1) → no limit
--- reference/t7-drafts: SURVIVORS.md
# Survivors: reference/t7-drafts

The three equivalent mutants of the blog (book 10, "Mutants that are the same program"), as in
experiments/e9-feedback/reference/t7-drafts/SURVIVORS.md; no input can tell any of them from the app.

- `Posts.Index Index.load:33:17 operator flipped` (`params.page < 1` → `<= 1`): the two differ only at page 1, where both branches give 1.
- `Posts.Index Index.load:33:48 operator flipped` (`params.page > lastPage` → `>=`): the two differ only at `lastPage`, where both branches give `lastPage`.
- `Posts.Index Index.load:42 limit dropped` (`Db.limit (perPage + 1)` → no limit): `load` keeps `List.take perPage` and sets `hasMore` from the length, so reading every post gives the same page, only slower.

--- reference-alt/t7-drafts: survivors
  Posts.Index   Index.load:33:17   operator flipped   params.page < 1 → params.page <= 1
  Posts.Index   Index.load:33:48   operator flipped   params.page > lastPage → params.page >= lastPage
  Posts.Index   Index.load:41      limit dropped      Db.limit (perPage + 1) → no limit
--- reference-alt/t7-drafts: SURVIVORS.md
# Survivors: reference-alt/t7-drafts

The three equivalent mutants of the blog (book 10, "Mutants that are the same program"), as in
experiments/e9-feedback/reference-alt/t7-drafts/SURVIVORS.md; no input can tell any of them from the app.

- `Posts.Index Index.load:33:17 operator flipped` (`params.page < 1` → `<= 1`): the two differ only at page 1, where both branches give 1.
- `Posts.Index Index.load:33:48 operator flipped` (`params.page > lastPage` → `>=`): the two differ only at `lastPage`, where both branches give `lastPage`.
- `Posts.Index Index.load:41 limit dropped` (`Db.limit (perPage + 1)` → no limit): `load` keeps `List.take perPage` and sets `hasMore` from the length, so reading every post gives the same page, only slower.

--- reference/t11-workspace: survivors
  Orgs.Index   Open:9   requires dropped   requires user → served without user (user = Table.User.UserId "mutant")
--- reference/t11-workspace: SURVIVORS.md
# Survivors: reference/t11-workspace

One equivalent mutant, the one `examples/workspace/README.md` names for its own `Orgs.Index`
`Open` (book 14, "Its tests and its score"); no input can tell it from the app.

- `Orgs.Index Open:9 requires dropped` (`requires user` → served without user, user =
  `Table.User.UserId "mutant"`): Open's one step is a `Session.put` of the org, and a session
  without a user has its `via` fields cleared, so the visitor's org is dropped and the redirect
  to `/projects` (`requires user, org`) sends them to log in, as the `requires` would. A probe
  test (a page held, the user logged out, Open pressed) sees the same answer, url and session
  either way: `Redirected (Auth.Login.Login {})`, `/login`, no user, no org.

--- reference-alt/t11-workspace: survivors
  Orgs.Index   Open:9   requires dropped   requires user → served without user (user = Table.User.UserId "mutant")
--- reference-alt/t11-workspace: SURVIVORS.md
# Survivors: reference-alt/t11-workspace

One equivalent mutant, the one `examples/workspace/README.md` names for its own `Orgs.Index`
`Open` (book 14, "Its tests and its score"); no input can tell it from the app.

- `Orgs.Index Open:9 requires dropped` (`requires user` → served without user, user =
  `Table.User.UserId "mutant"`): Open's one step is a `Session.put` of the org, and a session
  without a user has its `via` fields cleared, so the visitor's org is dropped and the redirect
  to `/projects` (`requires user, org`) sends them to log in, as the `requires` would. A probe
  test (a page held, the user logged out, Open pressed) sees the same answer, url and session
  either way: `Redirected (Auth.Login.Login {})`, `/login`, no user, no org.
```

## Gate-time fixes and decisions

Every change made to the material while bringing it to a green gate, and why (merged from the
working notes, `gate-notes.md`, deleted with this record). No hidden test was loosened; the only
hidden-test edits were drafting fixes made before their first gate run. The notes below that
`verify.sh` has no expected count for T12-T15 "yet" are out of date: the counts were added before
the pin (T12 4, T13 3, T14 3, T15 4) and the gate above checked them.
`hidden/t13-feed/Hidden.alder`'s header comment cites "gate-notes.md, T13": that is "T13 (Task 12)"
below (the hidden test is an input and stays as pinned).

### T9–T11 (Task 5)

- **Drill references' write-route gaps (tests added, no app code changed).** The first gate run
  scored the drill references 0.915 (T9), 0.939 (T10) and 0.978 (T11) with survivors that are
  test gaps, not equivalent mutants: the `requires dropped` of T9's `Rooms.Show.Book` and
  `Bookings.Show.Cancel`, of T10's `Invites.Index.Accept`/`Decline` and `Teams.Show.Remove`
  (each killed by a write sent from a page held while the user logged out), T9's
  `Rooms.Show Book.update:61 guard dropped` (a room deleted since its page was shown) and
  `Bookings.Show Cancel.update:66 invalid → refresh` (a Cancel from a page held the day before
  the booking started). The drill's own "a visitor cannot submit NewBooking" posts `from = to`,
  so `Response.invalid` re-renders a page that `requires user` and the mutant's answer is the
  same login redirect. Four tests, each headed "Added at the E12 gate", now kill all seven:
  `reference/t9-bookings` (Rooms.Show, Bookings.Show) and `reference/t10-teams` (Invites.Index,
  Teams.Show). `reference/` is therefore the drill replay plus these tests; a re-replay must
  re-apply them. Both now score 1.000.
- **T11's one survivor is equivalent** (both solutions): `Orgs.Index Open:9 requires dropped`,
  as in `examples/workspace/README.md`; justified in `reference/t11-workspace/SURVIVORS.md` and
  `reference-alt/t11-workspace/SURVIVORS.md`. A probe test (page held, logged out, Open pressed)
  sees the same answer, url and session against the app and the mutant.
- **No hidden test changed.** All six solutions pass every hidden test (7/7, 8/8, 9/9) with
  names `ok`; the alts (different markup, a plain-HTML layout, helpers in their own modules,
  `Auth.Login`'s write route named `Submit`, a started booking's Cancel answered 404 instead of
  422) caught no detail the texts leave open.

### T6 (Task 8)

- **`AddLine` carries the order (ruling).** The plan's corrected cut names
  `route AddLine { product: Product.ProductId } form NewLine`, but a write route does not know the
  page it came from (book 6, "The page a write came from") and runs no `load`, so the order it
  adds to has to ride in the route too. The Names block says
  `route AddLine { id: Order.OrderId, product: Product.ProductId } form NewLine`, `route Pay { id:
  Order.OrderId }` and `route Cancel { id: Order.OrderId }` (`id`, as `Show`'s and T4's `Save`'s);
  the hidden tests send `Orders.Show.AddLine { id, product }`, `Orders.Show.Pay { id }`,
  `Orders.Show.Cancel { id }`. The cut itself (a form on a parameterised write) is what findings
  names.
- **Write routes check again.** No `load` runs before a write route, so the page-era authority
  (`load` ran before every message) is now each write's own: the text says another user's order
  is 404 "on its page and for its forms and buttons alike" and a Staff user gets 404 "from the
  page and from its form and buttons alike", without naming `Task.notFound`/`Task.fail`. No
  hidden test checks a write's 404 (E9's never did); both solutions test it themselves.
- **Gate green on the first run**, no hidden test changed: 9/9 on both, scores 54/54 (1.000) on
  both after the solutions gained tests for the new write-route checks (Staff posts, a page held
  across a logout). A probe (reference without the Open check) fails exactly the stale-page
  hidden test.

### T7 (Task 9)

- **The base, `base/blog-nodrafts`, ported by hand** from `e8-hidden/base/blog-nodrafts` (11
  modules, `adapters/`, `migrations/`, `go.mod`, `static/` unchanged; `alder migrate` reports no
  change since `0001_initial`; `alder build` compiles the Go adapters). Each page module is a
  route module whose GET route is named after it (`Admin.Admin`, `Admin.Edit.Edit`,
  `Posts.Index.Index`, `Posts.Show.Show`, `Auth.Login.Login`), each msg a write route of the same
  name. Ruling: the write routes that acted on `model.post` now take the post from their path
  (`Admin.Edit.Save`/`Delete { id }`, `Posts.Show.AddComment { slug }`) and re-check what the
  page's `load` checked (Admin.Edit's `editable`; Admin's `Create` asks `User.author`), since no
  `load` runs before a write route. Its 25 tests are E8's, ported (one trace now starts with the
  re-check's `Db.first User`, `Db.get Post`). Committed before the solutions (66985b39).
- **Write-route gaps (tests added, no app code changed).** Ported as they were, the solutions
  scored 0.89 (reference) and 0.85 (alt). The new survivors were route-era test gaps: the
  `requires dropped` of `Admin.Create`, `Admin.Publish`, `Admin.Edit.Save` and `.Delete` (killed
  by a write sent from a page held while the user logged out: reference, one test in `Admin` and
  one in `Admin.Edit`; alt, one test in `Admin` for all four), alt's `Publish.update where dropped`
  (`Post.id.eq`: the "did not list" test now gives the author a draft of their own, which must
  stay a draft) and alt's `Save.update refresh → invalid` (the edit test now asserts `Refreshed`).
  Both now score 0.95; the three survivors left in each are E8's equivalent mutants, justified in
  each `SURVIVORS.md`.
- **No hidden test changed** beyond the port (7/7 on both; names `ok`; `drops` no / yes: the alt
  keeps E8's lossy migration, `0002_drop_created_change_post.sql`, dropping `User.created`).

### T12 (Task 11)

- **New task, nothing refused.** The language took everything the text asks for (`input=` on a
  plain `<input name="q">`, two GET routes in `Contacts.Index`, `@owner` on `Contact.owner`), so
  the text is the plan's names as written. The text adds two things the names leave open, both
  to fix what the hidden tests read: the page lists its matches in its own `<ul id="results">`
  (the element `Test.input` patches), and matching ignores case (`.contains`, chapter 8). The
  hidden tests query in each name's own case and compare single matches or counts, so they fix
  neither a case rule nor an order.
- **Drafting fix before the first gate run (not a loosening).** The first draft of "an empty
  query lists every contact" read `answer` from `Test.get`, whose result has no `answer`; it
  now opens the page with `Test.open`. Same assertions.
- **Mutation gap closed by a test.** Both solutions first left `Contact:… cascade ↔ restrict`
  alive; each `Table.Contact` now has "a user with contacts cannot be deleted" (as T5's
  `Note`). Both score 1.000 with no survivors, so neither has a `SURVIVORS.md`.
- The hidden
  module has 4 tests.

### T13 (Task 12)

- **The cursor is `More { after: Int }`, the number of entries already shown (user's ruling).**
  The spec's T13 bullet left the cursor's type to the model (`More { before: … }`, an `Int`
  offset or a key). `Test.intersect r` matches the page's sentinel by `r`'s path only (book 10:
  the query is not compared), but the hidden test must still write a value of `More`'s params
  type, `Feed.Index.More { after: 10 }`, and that value has to type-check against the model's
  route; a test cannot be written for a parameter whose name and type it does not know. Under
  the spec's rule for "a behaviour …
  the test cannot set up", the Names block fixes `route More = GET "/feed/more" { after: Int }`
  and says the first sentinel asks for `More { after: 10 }`, as chapter 6's `Feed` does. The
  alternative (drop the scroll test, record the cursor choice as a signal only) was declined:
  the scroll steps are the point of the task. The cost: the cursor's design is no longer the
  model's choice, so a keyset cursor (robust to entries posted between scrolls) is not
  observable in E12; offset paging, which the fixed cursor implies, repeats an entry when one is
  posted between scrolls, so no hidden test posts between scrolls.
- **Known blind spot (tests not changed).** Since `Test.intersect` compares only the path, the
  hidden tests fetch `More { after: 10 }` and `{ after: 20 }` themselves: a sentinel that
  requests a wrong `after` (say `More { after: 0 }`) still passes, as long as `More` answers
  correctly for the values the test sends. The text fixes what the sentinel asks for; the hidden
  tests do not check it.
- **Brand constructors only inside a `test`.** The hidden module's helper `insertAll` takes the
  texts as `Text.NonEmpty` values made in each test (`Text.NonEmpty (…)` in a top-level helper is
  E0011); the clock is set, then advanced a second before each insert, so `created` differs.
- **An equivalent mutant designed out of the reference.** The first reference read one row past
  the page (`Db.limit (pageSize + 1)`) to decide on the sentinel; dropping that limit is
  invisible to any test (the page is still `List.take 10`), scoring 5/6. The reference was
  rewritten (not trimmed) to count the table instead; `Db.limit pageSize` is then killed. Both
  solutions now score 1.000 with no survivors. The alt decides with `Db.first` at the next
  offset, in `Table.Entry`, with `<ol>`, a `<p>` sentinel and the design system's layout.
- **No language refusal changed the text.** Hidden 3/3 on both solutions, names `ok`. Probes: a
  `More` whose offset is one short fails "two scrolls…" and "no entry repeats"; an
  oldest-first feed fails "no entry repeats" and "posting an entry puts it first".

### T14 (Task 13)

- **The constructor is renamed (the gate rule): `type Status = Pending | Closed`.** With the
  plan's names, `type Status = Open | Closed` in `Table.Ticket` and `route Open` in `Board`, the
  checker refuses the route: `app/Board.alder:10:7: error[E0044]: route `Open` is named like the
  constructor `Open` (of `Table.Ticket.Status`): `Open { … }` cannot mean both; rename the
  route`. The route's name is the plan's (and chapter 6's `Dashboard`), so the constructor
  changes, as ruled: the text's Names block has `Pending | Closed` and
  `status: Status = Pending`, and says in one sentence why it is not `Open`. The hidden tests
  read `Pending` and `Closed`.
- **What the text adds to fix what the hidden tests read** (and where it departs from chapter
  6's `Dashboard`, it says so): `Recent`'s view uses `<li>` (the chapter's uses `<p>`), the
  title as each `<li>`'s whole text, no other `<li>` (no empty-state one), newest first, closed
  tickets included; `Open`'s view repeats `load={Open {}} every="10s"` on its root (as the
  chapter does; ten seconds, not thirty) and its whole text is the count in digits; the
  close buttons are on the page as `Board` renders it (not inside `#recent`/`#open`), so
  `Test.click (Board.Close { id })` finds them after a plain `Test.get`. No users: `App` has
  `session { }`, no `login`, no routes; tickets are added outside the app (as T9's rooms).
- **The polling step bites.** With `Open.view` reduced to `<span id="open">`, the hidden "the
  badge counts …" fails (checked by hand on a copy of the reference), as the spec intends.
- **Mutation.** Reference 7/7 at once. The alt first left `Board.load:21 orderBy reversed`
  alive (its pending list's order, the alt's own choice); its test "only pending tickets have a
  close button, newest first" now fixes that order. Both score 1.000, no `SURVIVORS.md`.
- The hidden
  module has 3 tests.

### T15 (Task 14)

- **New task, nothing refused.** The language took everything the text asks for (`@unique` on
  `Item.name`, set up by the hidden tests with two `Db.insert`s of different names; two GET
  routes in `Pantry.Index`; a per-row form posted by `Rename { id }`), so the Names block is the
  plan's names as written. The text adds what the hidden tests read and the names leave open:
  the `<ul id="items">`'s children are exactly the items' `<li>`s, the `.name`/`.quantity`
  elements (quantity as digits only) are the only ones of their class in the list, and the
  `<li>` holds the rename form and the `click={Bump …}`/`click={Remove …}` buttons that
  `Test.submit`/`Test.click` need. Rows are found by their fixed id (`#item-…`), so no hidden
  test depends on the list's order, which stays the model's choice.
- **Drafting fix before the first gate run (not a loosening).** The first draft counted
  `"#items li"`, which also counted the `<li>` of `<Errors form />`'s message list after a
  refused rename (3 instead of 2); it now counts `"#items > li"`, the children the text fixes.
- **No message check in the hidden tests.** `FormRejected` carries the errors the page shows,
  so the message is there only when the row's form has `<Errors form />` (W0005 warns without
  it). The hidden test checks `FormRejected _`, status 422, the stored row and the shown name;
  both solutions check the exact `[{ field: "", message: "name already taken" }]` themselves.
- **Toasts need no `<al-toasts>` for the test.** `r.notices` is `["Removed"]` with the alt's
  layout, which has no `<al-toasts>` region (the reference's has one).
- **Mutation gap closed by a test.** The alt first left `Table.Item nameTakenBy:14 where
  dropped` alive (its `Db.first` then returned the first row, which happened to be the item
  itself or the holder); a fourth case (a free name asked for the second item) kills it. Both
  solutions score 1.000 (14/14, 15/15) with no survivors, so neither has a `SURVIVORS.md`.
- Hidden 4/4 on both, names `ok`. Probes on the reference: `Bump` answering `refresh` fails
  only "bump adds one…"; `Remove` without the toast fails only "remove refreshes…"; `Rename`
  without the check fails only "renaming to a taken name…". The hidden module has 4 tests.

## The drill references are not a fresh replay

`reference/t9-bookings` and `reference/t10-teams` are the apps `replay_drill.py` builds plus **four
tests added at the gate** (tests only, no app code), each headed "Added at the E12 gate":
`reference/t9-bookings` in `Rooms.Show` and `Bookings.Show`, `reference/t10-teams` in
`Invites.Index` and `Teams.Show` (see "T9–T11" above). A re-replay with the E12 binary must
re-apply them, or the two solutions fall back to 0.915 and 0.939 under `--mutate`.

## Rulings

The controller's rulings (from the E12 execution log) that change the material, the texts, the
hidden tests, the harness's behaviour or the run procedure. Process-only rulings (task order,
concurrency, review rounds) are left out.

- **T13's cursor is `More { after: Int }`** (the number of entries already shown): a hidden test
  must construct the route value, so the Names block fixes it. Cost: the scroll test measures a
  fixed cursor, not the model's design (above, "T13").
- **T14's constructor `Open` is renamed `Pending | Closed`** because it clashes with route `Open`
  under the checker (E0044), as the gate rule says (above, "T14").
- **T11's second solution is hand-written**: no E8/E9 alternative exists for it.
- **The names check path-checks every named GET route** (supersedes an earlier ruling that checked
  only the first named GET route's path; a wrong `Row` path in T14 had scored `ok`).
- **Survivors are justified in `SURVIVORS.md`** for the drill references in their own task; only
  equivalent mutants are justified, test gaps are closed by tests (T9/T10 above).
- **`alder version` from a `git archive` build prints `alder (devel)`**, accepted: the binary is
  pinned by `E12_COMMIT` and its sha256.
- **`verify.sh` checks the hidden count of every task**, the new ones included: T12 4, T13 3, T14
  3, T15 4 (alongside T4 8, T5 8, T6 9, T7 7, T9 7, T10 8, T11 9).
- **Write routes take the parameters the reference apps and hidden tests use** (e.g. T11's
  `Toggle { id, todo }`): the hidden tests send them, so the texts are more specific than E9's.
- **T4: `Save`/`Delete` of another user's note is a 404, stated in the text.** E8's `load` gave
  this for free; a write route runs no `load`, so the text says it. No hidden test depends on it;
  findings names it as a deliberate wording addition.
- **T4: `Delete` takes `{ id: Note.NoteId }`**, forced by routes having no model.
- **T5: E9's `ASKED.md` is not ported; the hidden tests keep `Shown`.**
- **T9/T10: four tests added to the drill references** (tests only), since a `SURVIVORS.md` would
  have been false; the references therefore differ from a fresh replay (above).
- **T6: `AddLine` is `{ id: Order.OrderId, product: Product.ProductId } form NewLine`** (the plan
  said `{ product }`); `Pay` and `Cancel` take `{ id }`. A write route runs no `load` and cannot
  know the order otherwise. The corrected cut (a form on a parameterised write route) is kept and
  named in findings.
- **T6: the text keeps "404 for its forms and buttons alike"**, as T4: E9 had it through `load`.
  Findings lists T4's and T6's as deliberate wording additions, with the `AddLine` cut.
- **T7: the base's and solutions' write routes take `{ id }`/`{ slug }` and repeat `load`'s
  check**, as T4 and T6.
- **T3's smoke solution's `/admin` lists the logged-in author's posts** (no hidden tests; the
  smoke row only checks the text is buildable).
- **T12: the text excludes an empty-state `<li>` and states its departure from chapter 6's empty
  `<ul id="results">`**: a hidden test may not check a detail the text leaves open. The same rule
  was applied to T13–T15's texts.
- **T13: the reference counts the table instead of reading `pageSize + 1` rows**, to kill an
  otherwise undetectable mutant (a reference's internals are free; hidden tests unchanged).
- **T13: the text says `More`'s answer replaces the sentinel**, rewords the cursor ruling's reason,
  and notes the wrong-`after` blind spot (above, "T13").
- **T14: the text says the badge is the only `id="open"`**; the reference's variable `open` is
  renamed `pending`.
- **`run-matrix.sh`: `DRY_RUN=1` has no side effects** (it prints `would rerun` and deletes
  nothing; a dry run must never delete a live cell).
- **`run-matrix.sh`: a cell ending in `harness_error` stops the matrix with exit 4**, so a usage
  limit does not burn the remaining cells.
- **`run-matrix.sh` hardening for the unattended run:** cells get no stdin (`</dev/null`), the
  deadline is validated, a wall-clock watchdog kills a cell at 09:00 on weekdays, git failures
  fail closed, the lock is taken atomically, and orphaned build children are noted.

## Run log

Each stop and the end of the matrix gets a line here: date and time, the last finished cell, the
exit code, and the commit of `runs/`, `logs/` and `scores.md`.

- Night 1, Wed 2026-11-11 22:04 EST to Thu 2026-11-12 ~00:43 EST: all 78/78 cells done in one run (STOP_AT=0845, not reached), exit 0 ("matrix complete"). Committed in `e12: run, 78/78 cells (#208)`.

Resume point: complete.
