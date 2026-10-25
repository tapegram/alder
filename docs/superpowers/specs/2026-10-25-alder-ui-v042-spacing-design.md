# alder-ui v0.4.2: vertical spacing the shadcn way

Date: 2026-10-25. Issues: alder#39 (all three items), unblocks alder#21's close.
Repos: alder-ui (release v0.4.2), alder (vendor + Layouts + docs), nightvote-replay
(Layouts, re-pin, after-shots).

## 1. Intent

The #21 Plan 2 after-shots of nightvote-replay on alder-ui v0.4.1 looked better
than before but "cards smash together": an `h1` sits flush on the `al-card`
below it, cards touch cards, a table touches a button. alder#39 records this
plus two more gaps seen in the examples: the submit button is centred in grid
forms, and generated admin detail pages render label/value as flat lines.

The user's ruling: **mimic shadcn, and consolidate around it.** shadcn
components carry no outer margin; vertical spacing is the layout's job, done
with `flex flex-col gap-*` (alder-ui: `al-stack`). The fix is therefore not a
new default margin on components (the Pico/GOV.UK model, rejected) but:

1. apps put their page content in an `al-stack`, by way of their Layout;
2. alder-ui fixes the two element-level bugs that stacks and grids expose
   (`al-button` stretching, unstyled `dl`) and adds one gap step;
3. the scaffold, generators, examples, book and nightvote-replay follow.

## 2. Goals, non-goals, anti-goals

Goals

- A page written as `h1, al-alert, al-card, al-card, al-table, al-button`
  inside `Layout.Page` gets shadcn's section spacing with no per-page work.
- An `al-button` inside a grid form or an `al-stack` column is start-aligned
  at its natural width; full width is an explicit opt-in.
- Generated admin Show/Delete pages and the delete dialog's `dl` read as a
  detail list in shadcn's tokens.
- The rule is written down once, in alder-ui's README and the book's
  Appendix C: *blocks have no outer margin; siblings go in `al-stack`; the
  Layout owns the page stack.*
- nightvote-replay's after-shots are retaken on the result and #21 closes on
  the user's verdict.

Non-goals

- No default margins on `al-card`, `al-table`, `al-alert`, `al-button` or
  any other block component. (Rejected: Camp 2.)
- No page-level owl selector (`al-page > * + *`). The existing typography
  flow rhythm (`h2/h3/h4/p + *` → 1.5rem) **stays as the prose exception**;
  it is inert inside an `al-stack` because `al-stack > * { margin-block: 0 }`.
- No new elements. No description-list element; `dl` is styled by typography.
- No change to the apps' `form:not(._alder-click) { display: grid; gap: 1rem }`
  rule; it is shadcn's `grid gap-4` form and stays where it is (app CSS).
- No parity-grid changes beyond what the touched elements need.

Anti-goals (would make this a failure even if it works)

- A page that looks spaced in the scaffold but smashed in a hand-written
  app because the rule lives in a template rather than the Layout.
- A second, hidden spacing mechanism that fires for some neighbours only.
- Full-width buttons appearing wherever a button sits in a stack.

## 3. Decisions (the user's, unless marked as my call)

D1. **Layout owns the page stack.** The app Layout's content frame becomes
    `<al-page><main><al-stack gap="lg">{children}</al-stack></main></al-page>`.
    `gap="lg"` is 1.5rem = shadcn's `gap-6`, the gap between cards/sections in
    its blocks. Pages needing tighter groups add an inner `<al-stack gap="md">`;
    pages needing none use `plain` or their own markup. Applies to: the
    scaffold Layout, the generated admin Layout, the three examples' Layouts,
    nightvote-replay's `Layout` and `Layout.Live`. No page template changes.
    (Alternative rejected: a stack in every page template.)

D2. **`al-button` never stretches.** In a grid or flex column the host keeps
    its natural width and start alignment. `full` attribute = shadcn's
    `w-full` (host and inner `button`/`a` at `inline-size: 100%`). Icon sizes
    keep their fixed `--_al-w`. (Alternative rejected: inner button fills the
    host, giving full-width buttons in every stack.)

D3. **`dl` is styled in typography.css**, scoped like the rest to
    `al-page:not([plain])`: `dt` in `--muted-foreground` at the small text
    size, `dd` in `--foreground`, `margin: 0` on `dd`, rows as a two-column
    grid (`max-content 1fr`, column gap 1.5rem, row gap 0.5rem) at
    `min-width: 40rem`, single column below. A "beyond shadcn" addition like
    the flow rhythm, in shadcn's tokens. Generators keep emitting `dl`.
    (Alternative rejected: generators emit `al-table`.)

D4. **`al-stack gap="xl"` = 2.5rem** (shadcn's `gap-10`, used between sections
    in the reference). Added for completeness; D1 uses `lg`.

D5. **Flow rhythm stays** (Task 5b from v0.4.0). Documented as the prose
    exception, not the way to space components.

D6. **Scope split.** alder-ui v0.4.2 ships first (one PR, tag, release).
    Then one alder PR (vendor + Layouts + docs + examples). Then the
    nightvote-replay branch `alder-ui-041` is re-pinned and shots retaken;
    pushing nvr main still needs the user's OK (it redeploys Render).

D7 (my call). **Example CSS loses its stray block margins** once the Layout
    stack spaces things: todos `nav { margin: 1.5rem 0 0.5rem }`, blog
    `.account { margin-top: 1.5rem }`. List-item margins (blog
    `ul.posts li`, scaffold `li`) are inside lists and stay; workspace has
    only `margin: 0` resets. The `form:not(._alder-click)` grid rule is in
    the scaffold and all three examples (workspace ~line 16) and stays.

D8 (my call). The generated admin Layout (`admin.go` ~561) has two branches.
    With an app Layout it emits `<Layout.Page>nav {children}</Layout.Page>`:
    the app Layout supplies the stack, so the nav becomes the first stack
    child and gets the `lg` gap before the body; that is the intended look.
    Without an app Layout it emits its own `<al-page><nav/><main>…</main>
    </al-page>`: the stack goes inside `<main>`, around `{children}`. The
    nav's own `al-stack direction=row gap=sm` is unchanged in both.

D9 (my call). The stack goes inside `<main>` in every Layout; any `<header>`
    outside `<main>` (the blog example) is left alone.

D10 (my call). Apps generated from an older scaffold own their Layout and do
    not get the stack. Appendix C's migration/upgrade notes get one line
    with the Layout snippet; nothing automatic.

D11 (my call). The `dl` grid breakpoint is the viewport (`@media (min-width:
    40rem)`), the same breakpoint as `al-page`'s padding, not a container
    query. Labels are `max-content`, so a narrow page's card (max 40rem)
    is not cramped.

## 4. alder-ui v0.4.2

Branch `spacing-042` from main (3c7bc47 or later). One PR. Tag `v0.4.2`
after merge; `gh release create v0.4.2`.

### 4.1 `components/al-button.css`

- First reproduce in a browser (a `form { display: grid }` with a bare
  `<al-button>` and with `<al-button><button>`, and an `al-stack` column
  with both). The recon traced the stretch by reading the CSS, not by
  rendering it; the README's line "Buttons and badges keep their own width
  inside a column `al-stack`" (README ~85) is either already true by some
  other route or false, and the repro settles which rules 4.1 must touch.
  Expect both forms to need it.
- `full` is new (nothing in al-button.css or the manifest today). It works
  for the bare form, the `<button>` form and the `<a>` form alike.
- The host rule (`al-button, al-button > :is(button, a)`, ~line 162) keeps
  `display: inline-flex`. Add to the host: `width: fit-content` for the
  non-icon case (icon sizes set `--_al-w` explicitly and win), plus
  `justify-self: start; align-self: start` so a blockified host in a grid or
  flex column does not stretch. The wrapper form (`al-button:has(> :is(button,
  a))`, ~line 220) already zeroes its own box; it needs the same no-stretch
  treatment so the inner button is at the start, not centred.
- `al-button[full]` and `al-button[full] > :is(button, a)`:
  `inline-size: 100%; justify-self: stretch; align-self: stretch`.
- The inner `button`/`a` must stay full width *of the host* in both forms
  (it is today when the host is natural width).
- Parity: the oracle's shadcn Button is `inline-flex` with no `w-*`; the host
  width change is structural and goes in the allow-list with the reason
  "no-stretch host; shadcn relies on w-fit/self-start". Verify the grid-form
  case with a Playwright unittest (button `x` equals the form's content-box
  `x`, and width < form width), and the `full` case (widths equal).
- `al-stack[direction="row"]` children: unchanged (row items are not
  stretched on the main axis).

### 4.2 `typography.css`: `dl`

Per D3. Selectors: `:where(al-page:not([plain])) dl`, `… dt`, `… dd`.
`dl` resets `margin: 0`. A `dl` inside `al-dialog` (the delete dialog) is
inside `al-page` in Alder's markup, so it is covered; confirm in the demo.
Add a `dl` to `demo/typography.html` (or the showcase section the builder
finds fitting) and a pin in the unittests (grid at ≥40rem, stacked below).

### 4.3 `components/al-stack.css`

`al-stack[gap="xl"] { gap: 2.5rem; }`, plus the row form if the file
distinguishes them (it does not today). Document in README's al-stack row.

### 4.4 Docs

README: a short "Spacing" section near the top of the elements list:

> Elements have no outer margin, as in shadcn. Space siblings with
> `al-stack` (`sm` 0.5rem, `md` 1rem, `lg` 1.5rem, `xl` 2.5rem). An app's
> Layout wraps page content in `<al-stack gap="lg">`; inside `al-page`,
> prose keeps shadcn's Typography rhythm.

Migration note in the README/CHANGELOG for 0.4.2: `al-button` no longer
stretches in grids; use `full` where a full-width button was relied on.

### 4.5 Gate

`.venv/bin/python -m unittest discover -s test -p 'test_*.py'`,
`scripts/check.py`, parity `--strict` failing none / pending none, CI green.
Showcase: `index.html` is generated by `scripts/build.py` via
`scripts/showcase.py` from the demo pages; `check.py`'s bundle check and
`test/test_showcase.py` keep it in sync, and a new demo page must be added
to `showcase.ORDER` and `demo/index.html`. The `dl` and `full` cases go in
the existing demos (typography, button), so no new page is expected.
Manifest and custom-elements.json record `full` on al-button and `xl` on
al-stack.

## 5. alder

Branch `alder-ui-042-21` from main (55afa0e2 or later). One PR.

1. Vendor v0.4.2 (same procedure as Plan 2 Task 2: bundle, fonts list, the
   vendored version marker; `rt/web` and `cmd/alder/devui.go` need nothing
   unless the injected UI uses `dl` or a stretched button; check).
2. Layouts per D1: `internal/scaffold/files/app/Layout.alder`, the generated
   admin Layout in `internal/generate/admin.go` (~561), and
   `examples/{blog,todos,workspace}/app/Layout.alder` (or wherever each
   keeps its Page component). Goldens updated.
3. Admin dashboard (`admin.go` ~639) and silo (`admin_silo.go` ~320) already
   wrap cards in `al-stack gap=md`; keep them (an inner `md` group inside
   the page `lg` stack is the intended pattern). The silo page's `h1` and
   trailing `<a>` now sit in the page stack; fine.
4. Example CSS per D7.
5. Book Appendix C (`book/C-design-system.md`): the spacing rule (same text as the README's), with the
   Layout snippet; mention `full` and the `dl` detail list.
6. Visual pass of the three examples, light and dark, as in Plan 2 Task 6,
   with the shots in the PR.
7. After merge: `go install ./cmd/alder`, record the merge hash for nvr.

## 6. nightvote-replay (branch `alder-ui-041`)

1. `app/Layout.alder` (Page) and `Layout.Live`: wrap children per D1.
2. Re-pin `ALDER_REF` in nvr’s `Dockerfile` (ARG, line 8) to the alder merge hash; rebuild.
3. Retake the after-shots (`shots.mjs`, `.shots/`); compare with the v0.4.1
   after-shots and the before-shots.
4. The user's verdict closes #21. Pushing nvr main needs the user's OK.

The Plan 2 SDD ledger (`alder/.superpowers/sdd/2026-10-25-alder-ui-shadcn-plan2/progress.md`)
stays until #21 closes.

## 7. Left to the builder

- Exact selectors and specificity for the no-stretch host so it does not
  fight `al-stack[direction="row"]`, `al-card [slot="actions"]`, the
  pagination/menu/dialog buttons, or the icon sizes. Run parity after each.
- Whether `width: fit-content` alone suffices or `justify-self`/`align-self`
  are needed too; keep whichever set the Playwright pin needs.
- Where the `dl` demo goes (typography demo vs showcase section).
- The order of the three repos' work is fixed (D6); task granularity is not.

## 8. Open questions

None at hand-over; see the plan for execution choices.
