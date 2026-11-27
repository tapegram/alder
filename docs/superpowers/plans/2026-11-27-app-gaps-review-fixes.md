# App Gaps Review Fixes Implementation Plan (#326–#331)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Fix the findings of the final review of the app-gaps plan (#308–#311): six issues, one task each, one PR.

**Spec:** `docs/superpowers/specs/2026-11-24-app-gaps-design.md` (rulings R1–R16 at its end bind) for the behaviour being fixed; each task's issue text is its requirement. Review report with file:line detail: `.superpowers/sdd/2026-11-25-app-gaps/final-plan-review.md` (git-ignored, present in this worktree).

## Global Constraints

- One branch `review-fixes-326-331` off `main` (73773f30), one PR at the end. Each task is one or more commits; the suite is green after each task.
- No new Go dependency; `go.mod`'s require list does not change.
- No new diagnostic codes. A changed message keeps the existing style (back-quoted names, no trailing full stop where neighbours have none). Any changed message has its corpus `expected.diag` and `internal/diag/explain.go` text updated to match, and the book where it quotes the message.
- Test first: each fix has a test that fails before the change (say in the report if a clean RED was not possible and why).
- Messages, book text and comments in plain English, no em-dash chains; the book must match what the code does.
- Test commands: corpus `go test ./internal/check -run TestCorpus -count=1` (`-update` rewrites `expected.diag`; a fixture that checks warnings has a directory name starting with `W`); book `go test ./book -count=1`; real browser `go test -tags browser -run Browser ./cmd/alder`; whole suite `scripts/test.sh` (about 6 minutes; run it once at the end of the task, in the foreground under a timeout). Known flakes under full-suite load (re-run alone, do not fix here): `rt/jobs` `TestActiveSet_QueuedAndHealth`, `TestWorker_ClaimErrorRateLimited`, `rt/db` `TestIdleClose_IgnoresWorkerPoll`.
- CHANGELOG.md: one entry for this PR under the unreleased heading; each task adds its line to it.
- Do not push, open a PR, or comment on or close issues; the controller does that.

### Task 1: Text.Email folds on the SQL write side (#326)

**Decisions:**
- Fold in `rt/db` (or the generated code that calls it, whichever is the single place every write and condition passes): a `Text.Email` column value on insert and update, and a `Text.Email` operand of a condition (`.eq`, `.neq`, `.in` and any other operator a Text column has), including `Maybe Text.Email`. A LIKE-style pattern operand, if one exists for emails, is folded too.
- One fold function in `rt/db` used by both `DecodeEmail` and the new encode side. `rt/db` cannot import `rt/std`; the pin test lives in a package that can import both and asserts `std.TextFoldEmail`, the `rt/db` fold and the compile-time literal fold agree on a table of inputs including `É@X.com` and ASCII.
- End-to-end test: a Go adapter (or a test double at the same seam) returns `Ada@X.com`; `.eq` finds the stored `ada@x.com` row and an insert of the unfolded value is refused by `@unique`.
- "Share the open-coded email check" only if it is a small change; say so in the report if skipped.
- The book line of this issue is Task 6, not this task.

**Issue #326 text (the requirement):**

#### Case

#311 folds a `Text.Email` wherever Alder makes one (brand application, `Text.toEmail`, form and route decoders, row reads). A Go capability adapter that returns a `Text.Email` is not one of those sites: the adapter's Go string is the Alder value (`internal/gen/caps.go`), and nothing folds when a value is written or compared in SQL (`rt/db/encode.go` has `DecodeEmail` only).

Scenario (read from the code, not run): an OAuth or inbound-mail adapter returns `Ada@X.com`. `Db.where (User.email.eq e)` finds no row (stored `ada@x.com`); `Db.insert User { email: e }` passes `@unique`, which compares bytes. On the next read both rows decode to `ada@x.com`: two users with equal emails, which is the failure #311 set out to prevent.

Latent today: no adapter in the examples returns a `Text.Email`. The plan accepted the adapter gap as "compares unequal"; the duplicate past `@unique` was not weighed.

#### Proposal

- Fold when a `Text.Email` column value or condition operand is encoded for SQL. This closes adapters and any later construction site at once.
- A test that pins the three fold sites equal (compile-time literal fold, `std.TextFoldEmail`, `rt/db`'s `DecodeEmail` copy of `strings.ToLower`).
- Share the open-coded email check (three copies) while there.
- Book, adapter chapter: say that the runtime folds, or that an adapter must lower-case an email it returns.

Found by the final review of the app-gaps plan (#308–#311, f0aa627c..73773f30).

### Task 2: alder fmt holds cross-file migration fixes while a module fails resolution (#327)

**Decisions:**
- When any module of the project failed resolution, `alder fmt` applies no W0013 or W0014 fix whose change spans files (a declaration with references elsewhere, or a reference to another module) and prints one line saying it held them back and why, naming the module. Same-file-only fixes may still apply. The LSP formatting path follows the same rule without the printed line.
- The earlier partial-set rule (ledger: skip a declaration fix when its references lie outside the formatted set) stays.
- A run that changes nothing because the check could not run says so (the "first run silent" case) instead of exiting quietly.
- Idempotence test: after the import is fixed, one `alder fmt` run renames everything and a second changes nothing.

**Issue #327 text (the requirement):**

#### Case

`Todos.alder` has an unknown name (say a missing import) and declares `Add.update`; `Other.alder` tests `Todos.Add.update`. `alder fmt` rewrites `Other` to `Todos.Add.post` (W0013 fix) and leaves the declaration, so `Other`, which was clean, now fails with E0002 "route `Add` of `Todos` has no `post`". Fixing the import and running `alder fmt` again heals it. Confirmed by running.

Related, from the #308 task reviews: when import errors block the check, the first `alder fmt` run changes nothing and says nothing; a second run is needed.

#### Proposal

`alder fmt` does not apply cross-file migration fixes (W0013, W0014) while any module failed resolution, and says that it skipped them and why.

Found by the final review of the app-gaps plan (#308–#311, f0aa627c..73773f30).

### Task 3: `requires` clause edge-case diagnostics (#328)

**Decisions:**
- "Clause on a qualified declaration that is neither a route member nor a job": refuse it with the existing member-clause refusal code and a message in the same shape; do not add a code.
- `requires org, org`: one W0015 for the duplicate (or the existing duplicate-key message the route guard gives, if the guard has one; match the guard).
- Unknown key in a member clause: one diagnostic, the E0011.
- `skipNL`: the clause may sit on the line directly after the signature only; two or more newlines end the signature.
- Each bullet gets a corpus fixture or a Go test.

**Issue #328 text (the requirement):**

None of these lets a query escape its guard (the final review tried and found no escape); all are message quality.

- [ ] E0027 / expect-failure path in `parseValueRequires` leaves `Needs` non-empty with a zero `NeedsSpan` (and maybe an empty key); no fixture for a non-`Maybe` key.
- [ ] A job `run` clause naming `user` gets advice that fits a route, not a job.
- [ ] A clause on a qualified declaration that is neither a route member nor a job is accepted silently.
- [ ] `requires org, org` reports W0015 twice.
- [ ] A member clause with an unknown key reports E0011 and E0043 for one mistake.
- [ ] `siloJobMessage` (E0031 for `silo k` jobs) lacks the "through" note the other messages carry.
- [ ] `internal/check/rules.go` ~L412: a same-module reference from a context with no value (a brand's `where`, a field default) records no edge, while the cross-module path reports "this has no session". Not exploitable (pure contexts); make the two agree.
- [ ] `parseValueRequires`: `skipNL` may cross several newlines (the design says one).

Found by the final review of the app-gaps plan (#308–#311, f0aa627c..73773f30).

### Task 4: Response runtime hardening (#329)

**Decisions:**
- No-JS request with an empty removal id: keep the 500 (spec R11: a bad target delivers nothing). Add a test pinning it; no behaviour change.
- Trailing-space id: check the vendored Datastar client. If it trims the data line, treat a removal id with leading or trailing whitespace as a bad target (like the empty id) and test it; if it does not trim, add a test with the real bytes and change nothing.
- Dev panel labels: show `#` + the raw id, as the test browser messages do (ruling from the #309 work); the escaped form stays only in the event bytes.
- Typed removal: `AlderShow` and any other consumer switch on an explicit event kind, with an unknown kind printed as unknown rather than as a removal.

**Issue #329 text (the requirement):**

Minors from the #309 reviews. The panics need Go-side misuse today; the event handling becomes wrong when #321 adds event kinds, so do this first.

- [ ] `AlderShow` treats any non-toast event as a removal: give removals a typed event.
- [ ] `ReplaceRoots` has an unchecked `root.(html.Node)` (`rt/web/answer.go` ~L96); `ShowRoot` panics on a non-Node root.
- [ ] Roots are validated twice per answer.
- [ ] `Response` equality compares roots unflattened, so two equal answers can compare unequal in a test.
- [ ] No-JS request with an empty removal id on a replace gets the 500 page, though the removal would never be sent (303). Stricter than needed; decide.
- [ ] `datastar.IDSelector`: an id ending in a space gives a selector line ending in an escaped space; check whether the client trims the SSE data line.
- [ ] Dev panel labels show CSS-escaped selectors; a stale devpanel test comment ("no Patches tab").

Found by the final review of the app-gaps plan (#308–#311, f0aa627c..73773f30).

### Task 5: Test browser fidelity for replace and remove (#330)

**Decisions:**
- Root by root: apply each root in order; a root whose id the page no longer has at its turn fails the step with the existing "which the page does not have" message.
- `removeByID` removes every element with the id.
- Lazy regions: resolve nested lazy regions to a fixed depth of 3 and match the patch by the placeholder id, not the first patch; test the "answered no element" branch.
- `hasID` and `html.Repair`: if Repair can drop an id of a valid root, fix the comparison; if it cannot for roots the runtime accepts, record that in a comment and test and change nothing.
- Runs after Task 4: use its typed removal event.

**Issue #330 text (the requirement):**

Places where `rt/testing`'s browser can differ from a real one (#309 reviews). Only the first can let a test pass wrongly.

- [ ] `rt/testing/browser.go` ~L1516: all roots are looked up before any is applied. If root A's old element contains `#b` and the new A does not, a following root `#b` passes the "page has it" check but has nowhere to go once A is replaced; the real client logs a no-target error. Apply root by root. (Read from the code, not run.)
- [ ] `removeByID` removes the first element with the id; the client removes every match. Differs only on a page with duplicate ids.
- [ ] Lazy regions: `loadLazy` resolves one level and takes the first matching patch, so a replaced element that contains a lazy region compares against the placeholder (a false failure, never a false pass). The "answered no element" branch is untested.
- [ ] `hasID` fails a replace if `html.Repair` drops an id.
- [ ] The remove-ignored "logged once" test passes only through an earlier request; the invalid case is untested.

Found by the final review of the app-gaps plan (#308–#311, f0aa627c..73773f30).

### Task 6: Book leftovers (#331)

**Decisions:**
- The adapter rule after Task 1: the runtime folds a `Text.Email` when it is written or compared in the database, so an adapter need not lower-case, but a value an adapter returns is not folded in memory until it is stored and read; say what `==` does with it.
- W0013 entry in the same form as the W0014–W0016 entries; put all W entries in numeric order.
- Also say in chapter 10 what Task 5 changed if the chapter describes it (root-by-root, lazy depth 3).
- `go test ./book -count=1` green; appendix D is generated, do not hand-edit it.

**Issue #331 text (the requirement):**

- [ ] `book/11-tooling.md`: the warning reference has W0014–W0016 and no entry for W0013 (no `W0013` anywhere in the book), though it is the first warning an app from before #308 meets. The entries are also out of numeric order.
- [ ] Stale `update` wording in `internal/generate/specs.go` and the `Response.refresh` / `invalid` docs.
- [ ] Chapter 11: "the first of the last three" is ambiguous.
- [ ] "A form control cannot be a replace root" is enforced at run time and documented only in chapter 11.
- [ ] Chapter 10, fresh-load check: say that a replaced id the fresh page lacks is not compared (spec ruling R12), and that lazy regions resolve one level.
- [ ] Adapter chapter: the rule for a `Text.Email` an adapter returns (see the write-side fold issue).

Found by the final review of the app-gaps plan (#308–#311, f0aa627c..73773f30).

