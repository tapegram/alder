# E12 gate notes (merged into RUN.md in Task 17)

## T9–T11 (Task 5)

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

## T6 (Task 8)

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

## T7 (Task 9)

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
