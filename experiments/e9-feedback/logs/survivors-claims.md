
## 5. Equivalence claims *(hand-scored)*

Every comment in a feedback round's answer that says a survivor cannot be told apart (`logs/extract.py claims`;
a claim repeated in the next round is counted once). *Verdict*: **correct** when the two programs agree on every
input (the reasoning checked against the code), **plausible** when the reasoning holds for the code but was not
checked with a probe, **wrong** when a test can kill it (the evidence names how), **text-induced** when a test can
kill it but only one the task text, read literally, forbids.

| cell | survivor(s) claimed | verdict | evidence |
|---|---|---|---|
| haiku/t6-orders-s1 | `Orders.Index load` `where dropped` (`Line.order.in …`) | plausible | `toRow` keeps each order's own lines by id; reading every line gives the same rows |
| haiku/t6-orders-s2 (failed rounds) | `reject → rerender` in `Orders.Show update` | wrong | through the browser both answer `Rerendered` (the form is gone), but a test of `update` itself sees the `Nav` value (`Test.run`), as Opus's T6 comment says |
| haiku/t7-drafts-s1 r1 | the three `Auth.Login update` arms after `Password.verify` | text-induced | `Test.fake Password` reaches them (the references do; Haiku's own r2 killed all three) |
| haiku/t7-drafts-s1, s2 (s2 in the rejected round) | page clamp `<`/`<=`, `>`/`>=`; `limit dropped` | correct | the blog's three equivalents (book 10; `reference/t7-drafts/SURVIVORS.md`) |
| haiku/t9-bookings-s1 | `skipBooking` `b.to > day` → `>=` | correct | at `b.to == day` both branches give `day` |
| haiku/t9-bookings-s1 | `Rooms.Index rowFor` `where dropped` (`Booking.to.gt today`) | plausible | the next-free walk starts at today and skips only bookings holding a day ≥ today |
| haiku/t9-bookings-s2 | `skip` `day < b.to` → `<=` | correct | at `day == b.to` both branches give the same date |
| sonnet/t7-drafts-s1, s2 | page clamp ×2; `limit dropped` | correct | as above (two claims) |
| sonnet/t9-bookings-s1 | `nextFree` `b.to > d` → `>=` | correct | at `b.to == d` the step returns `d` either way |
| sonnet/t9-bookings-s1 | `Rooms.Index load` `where dropped` (`Booking.to.gt today`) | plausible | as Haiku's |
| sonnet/t10-teams-s1 r2 | the three `Auth.Login update` arms | text-induced | Sonnet's own r3 killed all three through the form with `Test.fake Password` |
| sonnet/t10-teams-s2 | `Invites.Index update` `bool flipped` (`owns` default `False`) | plausible | the default applies only when the invite's team row is missing, which `@restrict` forbids |
| opus/t6-orders-s1 | `Orders.Index load` `where dropped` (`Line.order.in …`) | plausible | as Haiku's T6 |
| opus/t7-drafts-s1 | page clamp ×2; `limit dropped` | correct | as above |
| opus/t7-drafts-s1 | the three `Auth.Login update` arms | text-induced | "only a test that calls the Password capability could tell them apart, and this app's tests do not call it": `Test.fake Password` does, and Opus used it in T6 and T9 |
| opus/t9-bookings-s1 | `advance` `day < booking.to` → `<=` | correct | at equality both branches give the same date |
| opus/t10-teams-s1 | the three `Auth.Login update` arms | text-induced | "this app's tests may not call it, so no permitted test can tell them apart": as T7 |
| opus/t10-teams-s1 | `Invites.Index load` `where dropped` (`Team.id.in …`) | plausible | `waiting` picks each invite's team by id |

**20 claims** (Haiku 8, one of them in failed rounds and one in the rejected round; Sonnet 6; Opus 6): 9 correct,
6 plausible, **4 text-induced**, **1 wrong**. The four text-induced ones are the login arms. Each cites the task text's
"Do not write tests that call the `Password` capability" (all five tasks say it). The only test that kills those arms
logs in through the form with `Test.fake Password`, and the book's own words call that test one that "calls the
`Password` capability", so the sentence, read literally, bans it. The references use the fake, so the authors meant
the real adapter: a material defect (E8's texts carry the same sentence), to be fixed in E10's material as "do not
call the real `Password` adapter; `Test.fake Password` is fine". Opus left those three survivors in T7 and T10 while
killing the same three in T6 and T9 with `Test.fake Password`.
