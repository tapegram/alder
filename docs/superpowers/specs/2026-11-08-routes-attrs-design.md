# Route attributes: `input=`, `load=`, `intersect=`, `every=` (#160 PR 10)

Addendum to `2026-11-03-routes-design.md`, section "Attributes that request a route". That spec
named the four attributes, their checker rule and their Datastar targets, and left four things
open that a plan cannot be written around. This document settles them. Every decision here is the
designer's call (the user said "you decide"); the original spec's decisions stand unless this
document says otherwise.

## Intent

Routes (PRs 1–9) made every page and every message a route, and `href`, `click` and `submit`
request them. Four interactions still have no Alder spelling and are done today with raw
Datastar or not at all: search as you type, infinite scroll, a slow widget filled after the page
shows, and a polling badge. Each is "GET a route and patch its view in by id" with a different
trigger. This slice gives each trigger an attribute, so the compiler can check the target, the
test browser can drive it, and the book can teach the four patterns from the routes spec's table.

## Goals, non-goals, anti-goals

Goals:

- The four patterns from the routes spec's table are writable in Alder, checked by `alder check`,
  testable in `alder test`, and taught in the book.
- The runtime protocol grows nothing: every trigger is an element fetch (`Alder-Fetch`,
  `window._alderF`) of a GET route, answered as `click=` to a GET route is today.
- A wrong target is a compile error, never a runtime surprise.

Non-goals:

- A no-JS fallback. These triggers need JavaScript by nature (the spec's call); the element renders
  nothing extra without it.
- A tunable debounce, a sub-second poll, polling only while visible, or `every` on `intersect`.
  Each is a follow-up once the fixed shape has been used in anger.
- New Datastar behaviour (several roots on one selector, new patch modes). The infinite-scroll
  shape below needs none.
- Changing `click=`, `submit=` or `href=`.

Anti-goals:

- Logic in two languages: the browser substitutes one query value; it never computes a URL,
  decides a route, or reads a signal. The server rendered a complete, correct URL.
- Hidden test behaviour: the test browser fires nothing on its own. A test says what the user did.

## The attributes

All four take a GET route written in place, as `href` does: `load={Stats {}}`,
`intersect={More { after: cursor }}`, `input={Results { q: model.q }}`. A write route is E0045
with the spec's wording ("a timer must not write" for a timed `load`). The answer is the route's
view, patched in where its root `id` stands, with the existing element-fetch delivery: a view
without a root id, a 404 or a failure toast as `click=` to a GET route does today.

| attribute | on | renders | fires |
|---|---|---|---|
| `load={R}` | any element | `data-init="<req>"` | once, when the element appears in the DOM (page load or a patch that brings it in) |
| `load={R} every="5s"` | any element | `data-init="<req>" data-on-interval__duration.5s="<req>"` | on appear, then every 5 s while the element is in the DOM |
| `intersect={R}` | any element | `data-on-intersect__once="<req>"` | once, when the element first scrolls into view |
| `input={R}` | `<input>`, `<textarea>`, `<select>`, or `<Input>`/`<Textarea>`/`<Select>` in a form | `data-on:input__debounce.300ms="<req with the field's value>"` | 300 ms after the user stops typing |

`<req>` is `@get('<url>', window._alderF?.())`, the request `RouteClick` renders for a GET
route **without its `busy` wrapper**. `_alderBusy` sets `inert` and `aria-busy` on the closest
form or the element for the request's duration, which is right for a button (no double send) and
wrong here: an `input=` field would freeze under the user's fingers after every keystroke, and a
polled badge inside a form would freeze the form every tick. None of the four triggers is a user
action that needs double-send protection (`__once` and `__debounce` already bound them), so none
uses `busy`. The `data-alder-pending` document marker that `busy` also sets is not set either;
these fetches are background work, not something the page should look busy for. `Alder-Page`,
`Alder-Build` and `Alder-Fetch` are sent by `_alderF` as for a GET `click`. (Vendored Datastar
v1.0.0 handles `data-on-intersect` and `data-on-interval` in its `on` plugin and has `__once`,
`__duration` and `__debounce`; the builder confirms the exact modifier spellings against
`rt/datastar/datastar.js` and pins them in `rt/html/html_test.go`. `RouteAttr.value` in
`rt/html/html.go` switches on the attribute name — `data-on:click`, `data-on:submit`, else the
URL — and grows a case per new name.)

**Overlapping requests.** Datastar's default `requestCancellation: "auto"` aborts the previous
request from the same element when a new one starts, so a slow answer to `q=mi` cannot land after
the answer to `q=mil`. `_alderF` must keep not overriding it (it sets no `requestCancellation`);
a Playwright test with a slow first response and a fast second pins the order.

**Element fetches and the shown hash.** As the routes spec says for `click=` to a GET route,
none of these touch `al-shown` or the region bookkeeping; the hash is stale after them by design.

**After a patch.** The patched view replaces the element whose id it carries. Idiomorph keeps an
element with the same id and updates its attributes; Datastar re-applies a `data-*` plugin only
when the attribute's value changed. So a polled view that renders itself with the same
`load={Badge {}} every="5s"` (same URL) keeps its interval running and does not re-run
`data-init`: **one request per tick**, which the polling Playwright test asserts via `loads`. A
view that renders the attribute with a different URL (a cursor, as the infinite-scroll sentinel
does) is a new trigger and fires once more, which is the point. If the builder finds Datastar
re-applies an unchanged `data-init` after a morph, `every` renders the interval alone (no
`data-init`) and the book says the first fetch is the timer's; the one-request-per-tick test
stays.

An element carries at most one of `load`, `intersect`, `input` (error otherwise); `click` and
`href` are independent of them and may coexist. `every` is only valid beside `load` (error
otherwise) — a timer is a repeating load, not a fifth trigger.

### `every`: the spelling

`every` is a **literal Text** attribute: `every="5s"`, `every="500ms"`, `every="1m"`. The grammar
is `^[1-9][0-9]*(ms|s|m)$`; anything else (a computed value, `"0s"`, a missing unit) is a compile
error naming the three units. The literal is passed through to Datastar's `__duration.<literal>`,
so the user reads and writes the same thing the browser runs. Alder has no duration type and this
slice does not add one; if one arrives later, `every={…}` can accept it beside the literal.

Why beside `load` and not a trigger of its own: a polled element is loaded now and again later;
reading `load={Badge {}} every="5s"` as "load Badge, every 5 s" says exactly that, and the
checker's write rule for `load` already covers the timer. The on-appear fetch is one request the
server-rendered badge may not strictly need; uniformity wins over saving it, and a recipe that
wants no first fetch renders an empty placeholder.

### `input`: how the field's value reaches the route

The route value is computed on the server at render time and gives a **complete URL** with
every query key set (`Results { q: model.q }` → `/results?q=mil`). The browser's only job is to
set **one query key** to the field's current value before sending. That key is the field's name:
`name="q"` on a plain control, or `field="q"` on `<Input>`/`<Textarea>`/`<Select>` inside a form.

Rendering: `data-on:input__debounce.300ms="<busy>(@get(window._alderQ('<url>', 'q', el.value),
window._alderF?.()))"`. `window._alderQ(url, key, value)` is a one-line helper added to
`datastar.BoostInit` beside `_alderF`: parse `url` against `location`, `searchParams.set(key,
value)`, return the string. It never adds or removes other keys.

Checker rules for `input`, in order:

1. The element is one of the five controls above; otherwise an error ("`input` fires as a field
   changes; put it on `<input>`, `<textarea>`, `<select>` or `<Input field=…>`").
2. The control has a literal `name` (or `field`); otherwise an error.
3. The route is GET (else E0045) and has a **query** parameter whose key (after `as "Key"`) is
   that name; otherwise an error naming the route and the key ("`input` sends field `q`;
   `Results` has no query parameter `q`"). A path parameter does not count: the browser cannot
   rewrite a path.
4. That parameter's type is `Text` (a default is fine); otherwise an error. The browser sends a
   string and the route's decoder must accept any string the user types.

The value in the route record (`q: model.q`) is the server's; the browser replaces it. The spec's
anti-goal holds: the page can never request something `view` did not compute, except for the one
string the user typed into the one field the compiler checked.

### `intersect`: the infinite-scroll shape

The routes spec left "Datastar's behaviour for several roots on one selector, and the shape of
`More` accordingly" to the builder. The answer is to need no such behaviour: **one root, nested**.

```
route More = GET "/todos/more" { after: Int }

view (rows, next) =
  <div id={"more-" ++ Int.toText after}>
    <ul> … one <li> per row … </ul>
    <div id={"more-" ++ Int.toText next} intersect={More { after: next }}></div>
  </div>
```

The page renders its first rows and a sentinel `<div id="more-10" intersect={More { after: 10 }}>`.
When it scrolls into view, `More { after: 10 }` answers a root with the same id containing the
next rows and the next sentinel, so the sentinel is replaced by rows-plus-sentinel. Ids are unique
because each carries its cursor; the DOM nests one level per page loaded, which is harmless. The
last page renders no sentinel. `__once` keeps a sentinel that stays visible from firing twice.
This is the shape Task 10.3's corpus app and the book use.

## The test browser

`alder test` drives the triggers explicitly; nothing fires on its own (a `Test.get` holds the page
as the server sent it). Three steps join `Test.get/open/fetch/submit/submitRaw/click`, each
taking a GET route written in place, as `Test.click` does:

| step | requires on the held page | sends | answer |
|---|---|---|---|
| `Test.load (Stats {})` | an element whose `load=` requests that route (matched by trigger kind and the route's **path**, query ignored, so a test may pass a value the page did not render; several matching elements is a failure naming them) | the element fetch | `Rendered`, `PageMissing`, `RequestFailed` as `Test.click` to a GET route |
| `Test.intersect (More { after: 10 })` | an element whose `intersect=` requests that route | the element fetch | same |
| `Test.input (Results { q: "mil" })` | a control whose `input=` requests that route | the element fetch of `RouteURL` of the **test's** value — exactly what the browser sends after `_alderQ` sets the key from the typed value | same |

A timer is a repeated load: a test polls by calling `Test.load` again. No step is added for
`every`. The checker treats the three like `Test.click` of a GET route (E0045 for a write route,
"wants a route written in place" otherwise); the element check (`checkElement`: the patched
element equals the route's view fetched on its own) runs for all three. A step whose element is
not on the held page fails as `Test.click` of a missing button does. `Test.input` does not change
the held document's field value: the field is not what the answer patches, and the equality
check compares the patched region.

`stepParams` records each step's one argument as the route value it was checked as.

## Checker, codes and explain

- Write-route targets: E0045, with the spec's wording and the explain entry extended to name the
  four attributes and three steps.
- Shape errors (`input` on a non-control or without a name, no query key, a non-Text key,
  `every` without `load` or malformed, two triggers on one element): the builder picks one
  existing HTML-attribute code or a new one, as the html checker's conventions dictate, and
  writes its explain entry; the `every` entry records the grammar and units.
- `alder explain` and the book show the spellings above.

## Where things change

| layer | files |
|---|---|
| checker | `internal/check/html.go` (the four attributes), `internal/check/browser.go` (three steps), `internal/diag/{diag,explain}.go` |
| IR / gen | `internal/ir/lower_html.go` (`AttrLoad`, `AttrIntersect`, `AttrInput`, the `every` literal), `internal/gen/html.go` |
| renderer | `rt/html/html.go` (`RouteLoad`, `RouteIntersect`, `RouteInput`; `RouteAttr` names `data-init`, `data-on-intersect__once`, `data-on-interval__duration.*`, `data-on:input__debounce.300ms`), `rt/html/html_test.go` |
| Datastar | `rt/datastar/datastar.go` `BoostInit` gains `window._alderQ`; `datastar_test.go` pins it |
| test browser | `rt/testing/browser.go` (`Load`, `Intersect`, `Input`; element matching), `internal/gen` test-step emission |
| corpus | `testdata/corpus/E0045-{load-write,input-write,intersect-write,every-write}/`, shape-error cases, `routes-attrs-ok/`, `routes-patterns-ok/` (the four patterns, each with an `alder test`) |
| browser tests | `cmd/alder/browser_test.go` (`-tags browser`): one Playwright test per pattern, asserting the request count (`loads`) and the patched region |
| book | ch. 6 gains a "Patterns" section walking the four, with the `More` shape; `book/site/build.py` regenerates |
| editors | no grammar change (plain attribute names); the builder greps `tree-sitter-alder/queries`, alder.nvim and alder-vscode for a `click`/`submit` attribute list and adds the four where one exists |

Generators (`new form`, `new msg`, …) emit none of these.

## Left to the builder

- Exact Datastar v1.0.0 modifier spellings, checked against the vendored script.
- The `_alderQ` implementation and whether `el.value` needs `?.` guards for `<select multiple>`
  (treat as: not supported in this slice; a multi-select `input=` is a Text key like any other).
- How the test browser reads the path out of the rendered attribute (it contains a JS
  expression, not a bare URL) and the wording of its "no such element" / "several elements"
  failures.
- Codes for the shape errors (above).
- The corpus apps' data: small, in-memory, no auth.
- Whether the book section goes before or after "Links and the boost".

## Tests the builder adds (by behaviour)

- Checker: each attribute to a write route (E0045), to a GET route (ok); `every` without `load`,
  malformed `every` (no unit, `0s`, computed), valid `ms`/`s`/`m`; `input` on a `<div>`, without
  `name`, with a route lacking the query key, with a path-param key, with an `Int` key, on
  `<Input field=…>` in a form (ok), with `as "Key"` renaming (matches the key); two triggers on
  one element.
- Rendering: each attribute's exact `data-*` output including the `busy` wrapper and
  `_alderF`; `input`'s `_alderQ` call with the key and `el.value`; `load every` renders both
  attributes; nothing extra without JavaScript.
- BoostInit: `_alderQ` sets one key and leaves the rest.
- Test browser: `Test.load`/`intersect`/`input` patch by id, run the element check, fail on a
  missing element, answer `PageMissing`/`RequestFailed` as `Test.click` does; `Test.input`'s URL
  carries the test's value.
- Rendering: no `_alderBusy` wrapper on any of the four.
- Patterns end to end (`routes-patterns-ok`): search as you type, infinite scroll (two pages then
  the end), slow widget, polling badge, each with an `alder test` and a Playwright test asserting
  one request per trigger and the patched region; the search test also proves a slow older
  answer never overwrites a newer one; the polling test asserts one request per tick across two
  ticks (no re-run of `data-init` after the patch).
- `go test ./book` on the new section; `scripts/test.sh` green.
