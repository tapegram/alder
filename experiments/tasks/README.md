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
