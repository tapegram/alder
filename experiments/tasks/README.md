# Canonical task texts

E10 should run these texts. T2–T4 are E7's (copied in for E8); T5, T6, T7 drafts, T9 bookings and T10 teams are
E9's (`e9-feedback/tasks/`), with E9's two material fixes applied here (and the first in T2–T4 too), while
`e8-hidden/` and `e9-feedback/` keep the texts as they were run. (1) The Password sentence: "Do not write tests that
call the `Password` capability" literally banned the book's fake-based login test, and the models obeyed: 23 of
E9's 50 first passes declared the example's `fake Password` and 0 used it, no first pass tested login, and four of
the five equivalence claims that were wrong about the program cited the sentence (`e9-feedback/findings.md` §2, §7).
Every text now says "Do not call the real `Password` adapter from a test; installing the fake with `Test.fake
Password` is fine." (2) T10's redirect: "Both answer `Redirect (Teams.Show …)`", after the invite's form and
message, was read by Haiku in 4 of 4 cells as applying to the invite only, so its `Remove` answered `Rerender` and
failed the hidden test (§4, §7); the Names block now gives `msg Remove` and `msg SendInvite` a redirect line each.

Since M11 (book 06, "Rules") a task's Names block may name the declared rules where it names an owned table or a
role page (`@owner`, `Db.mine`, `Db.getMine`, `Db.updateMine`, `Db.deleteMine`, `Db.insertMine`, `requires user,
role R`); E10 decides whether it does, and the texts above, as E8/E9 ran them, name none.

Since M12 (book 15) E11, "CLI-only development", consumes the generators instead of free-form edits: `alder new
page/table/form/msg/capability`, `alder edit table`, `alder fill` (the one free-text channel: one checked expression
per typed hole) and `alder new admin`/`alder regen admin`, reading their `--json`; the reference's command table lists them.

E11 ran these eight texts unchanged (Haiku and Sonnet, one sample, `cli` and `files`; `e11-cli/findings.md`). Two
findings about the texts: T4 has no Names block, while E8's hidden T4 tests (which E11 ran) use E8's names
(`App.SwitchTheme`, `Notes.Show.EditNote`, …), so T4's hidden results measure names the text does not give (both
`files` cells 5 / 8); and T6's `msg AddLine { product } form NewLine` cannot be built with the E11 commands (no
command attaches a form to a message with parameters), so a `cli` run must leave the Names (Haiku T6: own tests
pass, hidden 0 / 9).

Since the E11 fix batch, T4 carries E8's Names block (`App.SwitchTheme`, `Notes.Show.EditNote`, …, copied from
`e8-hidden/tasks/with/t4-appmsg.md`; its Password sentence stays the canonical one), so its hidden tests measure names
the text gives; E11's T4 results used the text without it. The drills' names check (`cmd/alder/drill_test.go`
`checkNames`) now also compares each form's fields and each message's parameters.

E11 run 2 (`e11-cli/findings-run2.md`) ran these texts with T4's Names block (Haiku, Sonnet, Opus, `cli`; Opus `files`); its T2 cells are void, because `examples/todos` did not pass `alder check` under the run's binary.

Since M15 (book 06, "Partitions"; book 08, "Silos") there is an eleventh text, T11 (`t11-workspace.md`): orgs whose
members work in one at a time (`session { …, org: Maybe Org.OrgId via Membership }`, `choose org = Orgs.Index`), and
whose projects and to-dos live in a database per org (`silo org { Project, Todo }`). Its Names block names the
declarations (`via`, `choose`, `silo`, `@partition org`, `requires user, org`) but not the scoped operations or the
re-read idiom, which the text asks for in words. Its hidden tests are `e9-feedback/hidden/t11-workspace/Hidden.alder`
(9 tests: two orgs, a non-member's Open refused from a stale page, a member removed losing access at once, a project
of another org not found), which `cmd/alder/drill_test.go` adds to the drill's app as the E9 harness adds its own;
the E9 run had no T11, so `e9-feedback/verify.sh` does not list it. The recorded drill
`testdata/drill/t11-workspace/script.txt` builds it from `alder new ws --bare` with the generators only (`alder new
silo`, `@partition=org`, `--requires org`, `alder edit session add-field org:Org.OrgId via Membership --choose
Orgs.Index`); its own tests score 37 / 37 and the hidden tests pass.

Since E12 (`e12-routes/`, spec `docs/superpowers/specs/2026-11-11-e12-routes-experiment-design.md`) the texts are
ported to routes (#160: a page is a route module's GET route, a message a write route), and there are thirteen:
the old wording is in `e8-hidden/tasks/`, `e9-feedback/tasks/` and in this file's history (`git log -p`), and
those directories keep the texts as they were run. The port changes vocabulary only: a page becomes a module's
GET route (named after the module, `Notes.Index.Index`), a message a write route (`route Add form NewNote`),
`Redirect (X {})` becomes `Response.redirect (X.R {})`, `Rerender` `Response.refresh` (or `render`). Tables,
fields, session, element ids and classes, behaviours, the tests paragraph and the output paragraph are as they
were, so each task's hidden tests keep their count (T4 8, T5 8, T6 9, T7 7, T9 7, T10 8, T11 9) and hidden
pass rates compare one to one with E8-E11. A Names block now names each write route's name, parameters and
form; a write route's method and path stay the model's choice, because the hidden tests act through the page
(`Test.submit (Notes.Index.Add {})`, `Test.click (Notes.Index.Delete { id })`).

What changed beyond vocabulary, text by text (the gate decisions are in `e12-routes/gate-notes.md`):

- **Write routes check again (T4, T6, T7).** A write route runs no `load`, so the check E8/E9's `load` did
  before every message is now each write route's own. T4 and T6 say it in words: a write route on another
  user's record (T4's `Save`/`Delete`, T6's forms and buttons) answers 404, "on its page and for its forms and
  buttons alike" (E8/E9 had this implicitly, through `load`; neither text names `Task.notFound` or
  `Task.fail`). T6 also says a Staff user gets 404 from the page and from its form and buttons alike. T4's
  `Delete` takes `{ id: Note.NoteId }`.
- **T6's cut is corrected.** E8's `msg AddLine { product } form NewLine` (a form on a message with
  parameters) could not be built with the E11 commands; it is an ordinary write route now. Because a write
  route does not know the page it came from, the order rides in the route: `route AddLine { id:
  Order.OrderId, product: Product.ProductId } form NewLine`, `route Pay { id: Order.OrderId }` and `route
  Cancel { id: Order.OrderId }` (`id`, as `Show`'s). The hidden tests send `Orders.Show.AddLine { id, product
  }`, `Pay { id }`, `Cancel { id }`.
- **T7's write routes take the post.** `Admin.Edit.Save`/`Delete { id }` and `Posts.Show.AddComment { slug }`
  take the post from their path and re-check what the page's `load` checked (the post is editable by the
  user; `Admin`'s `Create` asks for an author). T7 runs on `base/blog-nodrafts`, ported by hand from
  `e8-hidden/base/blog-nodrafts`.
- **T9-T11 take their write routes' parameters from the drill apps** (`Rooms.Show.Book`, `Teams.Show.Remove`,
  `Invites.Index.Accept`, ...), which the hidden tests require; the texts are slightly more specific than E9's.
  T9 and T10's references are the drill replay plus four tests added at the gate.
- **T12-T15 are new** (no history; each a complete app with a Names block, hidden tests and two reference
  solutions). T12 search (`input=`: `Results { q }` asked as the user types, a `<ul id="results">`, matching
  ignores case); T13 feed (`intersect=`: `More { after: Int }`, the number of entries already shown, fixed in
  the Names block because the hidden test must construct the route's value; chapter 6's `Feed`); T14 board
  (`load=`/`every=`: `Recent` and `Open` loaded on the page, `status: Status = Pending`, `type Status =
  Pending | Closed`, not `Open | Closed`, because the route `Open` clashes with the constructor `Open`
  (E0044); `Open` repeats `every="10s"`); T15 pantry (`Response` and a form: `Rename form NewName`, `@unique`
  name, `Bump`/`Remove { id }` with `render` and a toast). T12-T14's texts say what their lists contain and
  that there is no other `<li>` (no empty-state one) where a hidden test counts them, and name their departures
  from chapter 6's `Search`, `Feed` and `Dashboard` (`<li>` for `<p>`, ten seconds for thirty).
- **The table-module rule.** Each table lives in its own `Table.<Name>` module and the route modules import it
  (the shape the reference's chapter 3 teaches); the Names blocks name each table's module and file (`app/Table/Contact.alder`),
  and the names check compares the tables and their columns.

The harness's names check (`run_loop.names_check`) reads the route-era Names blocks (a GET route per module,
write routes with their parameters and forms) and is recorded in every `summary.json`, not only with `--cli`.

