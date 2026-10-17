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
