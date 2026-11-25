# Four language gaps: handler names, patch responses, key needs per value, email case

Design for #308, #309, #310 and #311. Written 2026-11-24 without the user in the loop: every
choice below is a recommendation, and the last section lists each decision the user must rule on.
Nothing here is implemented. Builds on `2026-11-03-routes-design.md` (routes, `Response`),
`2026-11-09-rest-write-routes-design.md` (paths) and the M15 partition and silo work.

## Intent

Building a multi-tenant app in Alder showed four gaps. They are separate changes, designed together
because two of them (#310, #309) change how an app is laid out and the other two touch the same
book chapters, generators and examples.

| Issue | Symptom | Change |
|---|---|---|
| #308 | `R.update` reads as Elm's `update` (a model transition) and is not one | a write route's handler is named after its method: `Add.post`, `Remove.delete` |
| #309 | "remove a row and change a count" has no answer short of `refresh`, which renders the page twice | `Response.patch : List Html -> Response` and `Response.remove` |
| #310 | a query over a siloed or scoped table compiles only in a route module, so access checks are copied into routes or split into "route reads, pure helper decides" | the key need belongs to the value, is stated in its signature and is checked where the value is used |
| #311 | two sign-ups that differ only in the case of the email are two users | `Text.Email` is lower-case by construction |

The motivating failure for #310 is worth stating once. Because a helper outside a route module
cannot read a siloed table, routes read the rows and hand them to a pure helper that decides
access. A review then found a helper that trusted its callers to have filtered the rows first. The
language pushed the query and the decision apart; this design lets them live in one function.

## Goals

- A reusable query over a siloed or partitioned table can live in the table's module or any helper
  module, and its signature says which session key it needs.
- A write handler can answer with views it built itself, several at once, and can remove an
  element, without loading the page.
- A reader who knows HTTP can read a route module top to bottom without a glossary.
- An email is one value however it was typed, in Alder code and in the database alike.
- Existing apps keep compiling for one release, with warnings that carry their own fix.

## Non-goals

- No effect system in the type language. Needs stay a checker analysis (see "Alternatives" under
  #310).
- No new way to choose the key: `requires`, the admin form's `silo k` and a job's `silo k` remain
  the only places a key becomes certain.
- No append, prepend, signals or streaming in `Response` yet. The design leaves room for them.
- No `@nocase` attribute and no general "normalising brand" feature.
- No change to GET routes: `load` and `view` keep their names.

## Constraints found in the code

- **The need is already inferred per value.** `internal/check/rules.go` records, for every
  top-level value, the scoped operations it uses directly (`markScoped`), follows references
  inside a module (`scopedEdges`, `propagateScoped`) and checks a reference to another module's
  value where it is written (`scopedRef`). What ties this to route modules is one function,
  `narrowed`, whose last case accepts a helper only when its module declares a route
  (`routeHelper`); otherwise the message is "…: `M` declares no route". #310 is a small checker
  change plus a surface for it, not a new analysis.
- **Tests are already exempt by location.** `narrowed` returns true inside any `test`, in any
  module; the runtime fails a scoped step that has no key ("Db.updateMine Todo: no signed-in
  user"). A test of a helper has to live in a route module today only because the helper does.
- **Jobs.** A job's `run` is narrowed for `k` only when the job has `silo k` (`armJob`,
  `jobRefusal`); `user` is never available in a job.
- **E0031 is a separate, program-wide analysis** (`writeset.go`): it already follows helpers in
  any module, by reference, and over-approximates.
- **`rt.Response` is shaped for events.** `ResponseEvent` holds only a toast, and its comment
  says "element patches, removals and a location come with their builders". The book's `Response`
  section names `remove` and `patch` as builders "not in Alder yet".
- **Datastar, confirmed against the vendored client** (`rt/datastar/datastar.js`, v1.0.0, the
  `datastar-patch-elements` handler):
  - modes are `remove`, `outer`, `inner`, `replace`, `prepend`, `append`, `before`, `after`;
    `outer` is the default and morphs;
  - with no selector and mode `outer` or `replace`, each top-level element of the event is matched
    to the page's element with the same `id` (`document.getElementById`), one by one; a root whose
    id the page lacks logs a console warning (`PatchElementsNoTargetsFound`) and the others still
    apply;
  - every other mode needs a selector (`PatchElementsExpectedSelector` otherwise), so a removal
    is `selector …` with `mode remove` and no elements; it removes every match.
- **`render` is not the same mechanism.** `rt/web/deliver.go` sends `selector #<first root's id>`,
  mode `outer`, with all the view's roots, so several roots replace one target (the "load more"
  pattern, `html.RootID`). `patch` needs the no-selector form, where each root finds its own
  target.
- **Diagnostics can carry a fix** (`diag.Fix`), the language server offers them as quick fixes,
  and `alder fmt` already applies two migration fixes on its own (`internal/format/migrate.go`,
  E0204 and E0205). `alder fix` no longer exists: it was removed with #184.
- **Warnings exist** (`W0001`–`W0012`), and W0012 is the precedent for "an old name kept for one
  release".
- **`Text.Email` is `brand Text where True`**; its check (`web.Email`: one `@`, text on both
  sides, no whitespace) runs when a form or a route is decoded, and nothing changes the text. A
  brand constructor is usable only in its own module and in tests (E0011), so app code cannot
  build a lower-cased one. `.contains` and `.startsWith` already compare with SQLite's `lower()`,
  which folds ASCII only; `.eq` and `@unique` compare bytes.
- Signatures are mandatory on top-level values (appendix A), and `requires` is already a keyword
  (the route guard).

---

## 1. #308: a write route's handler is named after its method

### Problem

`Toggle.update : Toggle.Params -> Session -> Task Response` borrows Elm's name for something
else: no model goes in and none comes out. The book has to say so ("There is no `Model` slot and
no `Msg`"). The route line already says what the request is; the handler's name should repeat it.

### Design

A write route has exactly one member, named after its method in lower case:

```alder
route Add    = POST   "/members"      { } form NewMember      requires user, org
route Rename = PATCH  "/members/{id}" { id: Member.MemberId } requires user, org
route Remove = DELETE "/members/{id}" { id: Member.MemberId } requires user, org

Add.post      : Add.Params -> Session -> Task Response
Rename.patch  : Rename.Params -> Session -> Task Response
Remove.delete : Remove.Params -> Session -> Task Response
```

The type does not change. `PUT` gives `R.put`. GET routes keep `load` and `view`; there is no
`R.get`. In prose (book, messages, comments) the member is **the handler**: "a write route's
handler", never "the update".

Checker rules, all in the existing route-shape family (E0043):

| Case | Message |
|---|---|
| no handler | ``route `Add` is a POST route: it needs `Add.post : Add.Params -> Session -> Task Response` `` |
| another method's name | ``route `Add` is a POST route, so its handler is `Add.post`, not `Add.put` `` (fix: rename) |
| a method name on a GET route | ``route `Index` is a GET route, which has no `Index.post`: a GET route has `Index.load` and `Index.view`; a handler named after its method belongs to a write route (POST, PUT, PATCH or DELETE)`` |
| `load` or `view` on a write route | today's text with `update` replaced: ``…a write route has only `Add.post : …`; `load` and `view` belong to a GET route`` |
| both names | ``route `Add` has two handlers, `Add.update` and `Add.post`: keep `Add.post` `` |

A reference to the old name once the declaration is renamed (`Test.run (Toggle.update …)`) is
E0002 with the hint "`Toggle.update` is now `Toggle.patch`" and the rename as its fix.

Everything that prints a handler's name takes it from the declaration, so E0031 ("`Create.post`
writes `Project` (silo `org`) and …"), traces and the dev panel follow without their own change.

### Migration

For one release `R.update` is still accepted on a write route, with a warning (W0013):

```text
`Add.update` is now `Add.post`: a write route's handler is named after its method. `alder fmt` renames it.
```

- The warning carries the fix: the declaration's two names (signature and definition) and every
  reference to it, in any module, are renamed. The method is read from the route line, so the fix
  needs no input.
- `alder fmt` applies it, as it applies the import-qualifier migrations today. The language
  server offers it as a quick fix, and "fix all" in a file does the file. No `alder fix` command
  comes back for this (ruling R14).
- The release after, `R.update` is the E0043 "another method's name" error, still with the fix.
- The unqualified `update` of the page era stays E0046 as it is.

### What it touches

- **Compiler:** `internal/check/routes.go` (`checkWriteRoute`, `checkGetRoute`, `needsShown`),
  `html.go` (`mayInvalid` looks the member up by name), `helpers.go`, `removed.go`,
  `writeset.go` (arms), `internal/ir/lower.go` (member dispatch), `internal/fill`,
  `internal/mutate` if it names the member. A single `handlerName(method)` helper replaces the
  literal `"update"`.
- **Formatter:** the W0013 migration in `internal/format/migrate.go`.
- **LSP:** completion of route members (`complete.go` offers `update`), the "add missing member"
  action (`actions.go`), hover and documentSymbol text.
- **tree-sitter:** the grammar does not name `update`; the highlight queries and the drift test
  corpus are checked for it.
- **Generators:** the templates that write the handler (14 lines name `update`), `crud.go`, `holes.go`, the admin
  and auth generators; goldens regenerate.
- **Book:** chapters 2, 5, 6, 7, 8, 10, 13, 14, 15, 17 and appendix B use `update` for the
  handler. Chapter 6's opening and "The state rule" are reworded ("A handler changes data, and a
  GET route renders data"). Book snippets are tested, so the sweep is mechanical and verified.
- **Examples and fixtures:** `examples/*`, `testdata/corpus`, `golden`, `generate`, `mutate`.
  `alder fmt` does the examples; fixtures that test the warning keep the old name.
- **Editors:** alder.nvim and alder-vscode snippets and e2e expectations (alder-vscode's
  `src/generate/flow.ts` mentions `.update`).

### Tests

- Corpus: each row of the table above; W0013 with its fix applied gives a module that checks
  clean; a cross-module reference is renamed.
- `alder fmt` on a fixture app renames declarations and references and is idempotent.
- LSP: completion offers the method's name for the route under the cursor.

### Alternatives rejected

- **One word for every write (`write`, `handle`).** Ruled out by the user: the method name is
  more explicit and matches the route line.
- **`R.get` for GET routes.** A GET route has two members with two jobs; one name cannot replace
  both, and `load`/`view` mislead nobody.
- **Keep `update` as a permanent alias.** Two names for one thing, and generators would have to
  pick one.

### Open question

- **`Rename.patch` beside `Response.patch`** (#309). A PATCH handler that answers with a patch
  reads `Rename.patch p session = … Response.patch [ … ]`. Both names are right in their own
  vocabulary, and the qualifier tells them apart. Recommendation: keep both (ruling R9).

---

## 2. #310: the key need belongs to the value

### Problem

A read or write of a siloed table, and every scoped operation, needs a session key (`org`,
`user`) to be certain. Today that is expressed by location: the code must sit in a module that
declares a route (E0026 otherwise). So:

- `Thing.current : ThingId -> Query (Maybe Thing)` cannot be defined in `Table/Thing.alder`.
- A shared access check cannot be a query; it becomes a pure function over rows each route
  must remember to read and filter.
- A test of such a helper has to live in a route module.

The requirement itself is sound. Only its expression is wrong.

### Design in one paragraph

Every top-level value has a **need**: the set of session keys that must be certain when it runs.
The checker infers it (it already does), any module may hold a value with a need, the value's
signature states the need with a `requires` clause, and a reference to the value is allowed
exactly where each key is certain: in a member of a route that requires it, in a job that takes
it, in another value that states it, and in a test.

### Syntax

A signature may end with `requires` and session keys, the word and the meaning of the route
line:

```alder
module Table.Member

table Member {
  id: MemberId @id,
  name: Text.NonEmpty,
  active: Bool = True,
  org: Org.OrgId @ref Org @partition org,
}

visible : Query (List Member) requires org
visible = Db.scoped Member |> Db.where (Member.active.eq True) |> Db.orderBy Member.name.asc |> Db.all

count : Query Int requires org
count = Db.scoped Member |> Db.where (Member.active.eq True) |> Db.count

mineOpen : Select Todo requires user
```

```ebnf
valueDecl = declName ":" type [needs] NL declName {lname} "=" expr ;
needs     = "requires" lname {"," lname} ;   (* session keys only: no role, no silo *)
```

The clause may start the next line, as `->` may. It is not part of the type: `visible` is a
`Query (List Member)` and is passed, stored and unified as one.

A route's members (`Index.load`, `Add.post`) never write the clause; theirs is the route line. A
job's `run` takes its key from `silo k`.

### What gives a value a need

Unchanged from today, now in any module:

- a scoped operation: `Db.mine` and its family need `user`; `Db.scoped T` and its family need
  `T`'s partition key;
- an unscoped operation (`Db.from`, `Db.get`, `Db.insert`, `Db.update`, `Db.delete`) on a table
  listed in `silo k` needs `k`;
- a reference to a top-level value that has a need, whether or not it is called (the same
  over-approximation E0031 makes, and for the same reason: a reference can be applied anywhere
  later).

The third rule is what makes the analysis sound without types: a value with a need can only be
reached through a reference, and every reference sits in a body that then has the need itself or
is a place where the key is certain. A function parameter carries no need; the reference that
supplied the argument was checked where it was written.

### Where a key is certain

| Place | `k` is certain when |
|---|---|
| a route's `load`, `view` or handler | the route `requires k`, or is the admin form `…, silo k` |
| a job's `run` | the job has `silo k`; `user` never |
| a top-level value that is no route's or job's member, in any module | its signature says `requires k` |
| a `test` and a test helper | always; the runtime fails the step when the session has no `k` |
| a `fake` | never |

The only row that changes is the third. Today it reads "a helper of a module that declares a
route". The first, fourth and fifth rows are today's rules.

### The clause is checked

For a top-level value `v` that is no member, with inferred need *I* and stated need *S*:

1. A key in *I* that *S* lacks is E0026 at the use that needs it. The fix inserts or extends the
   clause.
2. A key in *S* that *I* lacks is a warning (W0015): the clause is allowed to run ahead of the
   body, so a helper can be declared and its callers written before it is filled in.
3. What other values see is *S*. A caller is checked against the signature alone, so an error
   never depends on the order modules are checked in, and the language server can check one file
   against its imports' signatures.

Because of rule 3, the clause is **required** wherever *I* is not empty. It is introduced in two
steps so that existing apps keep compiling (ruling R2):

- In a module that declares no route and no job, rule 1 is an error from the start. Nothing in
  such a module compiles today, so nothing breaks.
- In a module that declares a route or a job, where helpers with a need exist today, rule 1 is a
  warning (W0014) for one release and an error after. `alder fmt` adds the clause.

### Messages

All E0026 unless marked. `Access.reviewers` calls `Reviewer.current`, which uses
`Db.scoped Reviewer`.

| Case | Text |
|---|---|
| used in a route without the key (one hop: today's text, unchanged) | `` `Reviewer.current` uses `Db.scoped Reviewer`, which needs a route that `requires org`: `Index` does not `` |
| the same, through helpers | `` `Access.reviewers` uses `Db.scoped Reviewer` (through `Reviewer.current`), which needs a route that `requires org`: `Index` does not `` |
| used in a job | today's `jobRefusal` texts with the same subject: `` `Access.reviewers` uses `Db.scoped Reviewer` (through `Reviewer.current`), which needs a job with `silo org`: a job without `silo org` runs on the shared store `` |
| used in a fake | today's text: ``…cannot be used in a fake: a fake has no session`` |
| a helper without the clause | `` `current` uses `Db.scoped Reviewer`, which needs `org`: say so in its signature, `current : RecordId -> Query (Maybe Reviewer) requires org` `` (fix: insert) |
| a helper whose clause lacks a key | `` `current` uses `Db.mine Draft`, which needs `user`: its signature says `requires org`; write `requires user, org` `` (fix: replace; keys in the session's field order) |
| a helper calling a helper | `` `reviewers` uses `Reviewer.current`, which `requires org`: say so in its signature, `reviewers : RecordId -> Query (List Reviewer) requires org` `` |
| W0014, route-module helper, one release | `` `recent` uses `Db.mine`, which needs `user`: say so in its signature (`recent : Select Todo requires user`). `alder fmt` adds it. `` |
| W0015, clause ahead of the body | `` `current` says `requires org` and uses nothing that needs `org` `` |
| clause on a route's member (E0043) | `` `Index.load` takes what it requires from its route: write `requires org` on `route Index` `` |
| clause on a job's `run` (E0043) | `` `Digest.run` takes its key from its job: write `silo org` on `job Digest` `` |
| not a session key (E0032) | `` `requires team`: the session has no field `team` `` and `` `requires theme`: session field `theme` is not a `Maybe`, so there is nothing to require `` |
| `role` or `silo` in the clause (E0027) | `` a value requires session keys only: a role check belongs on the route line `` |

Gone: "…needs a route that `requires org`: `M` declares no route". The chain names at most the
first helper and the operation; `alder explain E0026` is rewritten around the table above.

### Runtime

Nothing changes. A scoped operation reads the running request's session, not an argument, and a
siloed table is opened by the session's key; the clause is erased before lowering. The runtime's
own failure ("no signed-in user", "no org") stays as the backstop for tests and for adapters.

### E0031 (one store per `Task`)

The two analyses stay separate and both stay program-wide:

- the need answers "may this run here?" and is checked at every reference;
- the write set answers "does this `Task` write two stores?" and is checked once per arm (a
  handler, a job's `run`, a fake's entry).

Moving writes into table modules adds no case E0031 does not already follow, since it walks
helpers in any module. Its message gains the same "(through `Helper`)" note when a write is
reached through a helper, because that will now be the usual shape:

```text
`Create.post` writes `Project` (silo `org`) and `AuditLog` (shared, through `Audit.record`): a Task writes to one store. Write `AuditLog` inside the silo, or send it from its own route.
```

A need says nothing about stores: a value that `requires org` may read the silo and write the
shared store, and a handler may call two such values as long as its writes land in one store.

### Jobs

No change to what a job can do. A job with `silo org` may now call any helper that
`requires org`, from any module, which is the gain. `user` stays unavailable ("give the job the
user as a parameter and use the unscoped operations"), and a job without `silo k` still cannot
reach `k`. An app that partitions by `org` without a silo has no job form that takes the key;
that gap exists today and is left for a follow-up (ruling R5).

### The admin form

`requires user, role Admin, silo org` makes `org` certain from the route's parameter, so every
helper that `requires org` is usable there and acts on the parameter's org. That is today's rule
for the scoped operations, reaching further. The book's warning box ("the role is the only
guard") gains one sentence: a helper cannot tell a member's request from an admin's.

### Tests

A test in any module may call a helper with a need, as it may use the operations directly today.
`Test.login` supplies the key; without it the step fails with the runtime's message. The checker
does not try to prove that a test logged in (ruling R6). Test helpers neither state nor are
checked for needs, as now.

### What it touches

- **Compiler:** `rules.go`: `narrowed` (its last case becomes "a top-level value whose signature
  states the key", with the two-step rule), `scopedRefusal` and `scopedRef` (messages, the
  through-note, reading *S* instead of the propagated set), `propagateScoped` (kept for the
  inference inside a module). Parser and AST: the clause on `ValueDecl`. `partitions.go`: the
  clause's keys are validated as a guard's are. `writeset.go`: the through-note.
  `internal/diag/explain.go`: E0026 and E0031.
- **Formatter:** prints the clause on the signature line, or on its own line when the signature
  is broken; applies the W0014 fix.
- **LSP:** hover shows the signature with its clause, and for a helper still under W0014 the
  inferred need; completion offers `requires` after a signature's type and the session's keys
  after it; quick fixes for the three "say so in its signature" messages; semantic token for the
  keyword.
- **tree-sitter:** a `requires_clause` node on the signature, reusing the guard's keyword
  highlight; grammar drift test.
- **Docs tooling:** `alder doc` and the stdlib reference print signatures verbatim, so the clause
  shows without work. No stdlib function has a need to state: the operations are the source.
- **Generators:** no generator has to change. `new resource`, `new admin` and `new form` keep
  writing their queries in the route module. Moving generated queries into the table module is a
  follow-up, decided after hand-written apps show the shape.
- **Mutation testing:** the rule mutants (scoped to unscoped) must not assume a route module;
  checked, and one mutate fixture added in a table module.
- **Book:** chapter 6 "Where they may be used", "`requires user, org`" and "Where the rules are
  kept" are rewritten around the table above; chapter 8 "Silos: the rules" drops "this is not a
  member of one" for helpers; chapter 10 "Tests that use the rules" gains a helper test in a
  table module; chapter 3 and appendix A get the clause; appendix B gets a pitfall entry ("read,
  then pass rows to a helper": put the query in the helper). Chapter 14's walk-through follows
  the example.
- **Examples:** `examples/workspace` moves `Project` and `Todo` queries that two routes share
  into their table modules; `examples/todos` gets `requires user` on its `recent`-style helpers.
- **Editors:** grammar bump in alder.nvim and alder-vscode; nothing else.

### Tests of the feature

- Corpus, positive: a table module defining a query over its own siloed table; a helper module
  two hops from the operation; use from a member route, from the admin form, from a `silo org`
  job and from a test in the table module.
- Corpus, negative: every row of the message table; a reference passed as an argument
  (`List.map Reviewer.current ids`) in a route without the key; a record field holding a needy
  function.
- The W0014 fixture: a route-module helper without the clause warns, `alder fmt` adds it, and
  the result checks clean.
- Runtime: a workspace test that calls a table-module helper as two orgs and sees two answers,
  and one without a login that fails with the runtime's message.
- LSP: hover and the insert-clause quick fix.

### Alternatives rejected

- **The need in the type (`Query` indexed by a key set).** `let <-` would have to union the sets
  of both sides, so `Query`, `Task`, `Select`, `Condition` and every stdlib combinator over them
  (`Query.map`, `Task.all`, `List.map` over queries) would need set-polymorphic signatures. Alder
  has no row types and no typeclasses, and adding either for this is the largest change the type
  system has seen. It would also split `Query a` into types that do not unify, so a list of
  queries with different needs stops type-checking. The house way for "what may this code
  reach" is already a reference analysis with a named chain (E0031, `Session.put` in jobs); this
  stays with it. What the type would have bought, a need visible at the definition, the clause
  gives.
- **Pass the key as an argument (`Db.scopedTo org Project`).** The key would be an ordinary
  `Org.OrgId`, which any code can obtain from any `Org` row, so the guarantee that a member route
  reads only the session's org is lost. The admin form is the one place that takes the key from
  a parameter, and it is fenced by a role check for that reason.
- **Inference only, no clause.** It is the smallest change (one line in `narrowed`), and it is
  PR A below. On its own it leaves the need invisible in a language where every top-level value
  states its type, puts the error far from the helper that caused it, and makes a caller's
  errors depend on another module's bodies. See ruling R2 for it as an option.
- **A new word (`needs`).** `requires org` on a route and on a value mean the same fact. One
  word; and `requires` is already reserved, where `needs` in a type position would read as a
  type variable.
- **Let the clause be exact (no key the body lacks).** A helper written before its body, or a
  typed hole, would not compile with its callers. A warning is enough.

### Open questions

1. **Clause required or optional** (ruling R2). Recommended: required, in the two steps above.
2. **A job form for a partition without a silo** (ruling R5). Recommended: not in this work.
3. **Should W0015 be an error after a release?** Recommended: no; it stays a warning.

---

## 3. #309: `Response.patch` and `Response.remove`

### Problem

A handler has two ways to change the page in place. `render r` shows one GET route's view and
runs that route's `load` to do it, even when the handler holds the data. `refresh` handles
anything, at the price of rendering the page before and after the write. "Delete a row, and the
count above the list changes" is the commonest case that fits neither: two places, one of them a
removal.

### Design

```alder fragment
Response.patch  : List Html -> Response               -- 200: each root replaces the element with its id
Response.remove : Text -> Response -> Response         -- also removes the element with this id
```

```alder
Remove.delete : Remove.Params -> Session -> Task Response
Remove.delete p session = {
  let _ <- Db.deleteScoped Member p.id
  let n <- Member.count
  Task.succeed (Response.patch [ countView n ] |> Response.remove ("member-" ++ p.id))
}

countView : Int -> Html
countView n = <p id="member-count">{Text.fromInt n} members</p>
```

`Index.view` draws the count with the same `countView`, so the page and the patch cannot drift
apart in markup.

**`patch views`**

- The list is flattened: fragments are opened and whitespace-only text is dropped. What remains
  are the **roots**.
- Every root is an element with a non-empty `id`, and no two roots share one. Text, raw markup
  and `<html>`, `<head>` or `<body>` are not roots.
- Each root replaces the page's element with the same `id`, by morph. Nothing else on the page is
  loaded, rendered or sent, and no before-view is rendered.
- `Response.patch []` is a 200 that changes nothing: the answer of a write whose only news is a
  toast.

**`remove id`** is a modifier, like `toast`: it adds one removal to a response. Removals run
after the response's patches, in the order written.

- It applies to `patch` and to `render` ("show this row again, and drop that one").
- On `redirect`, `refresh` and `invalid` it has nothing to do, since those answers bring a whole
  page. Written in place it is E0045: "`Response.remove` after `Response.refresh`: a refresh
  sends the page as it now is, which already lacks the element". Reached through a variable it
  is ignored and logged once.
- The id is the element's `id` as text, not a selector.

The issue wrote `Response.remove : Text -> Response` and then piped a response into it; the
modifier is the form its example uses (ruling R8).

**One sentence for the book:** `render` is one route's view by its own `load`; `patch` is views
you built from data you hold; `refresh` is the page again when you would rather not say what
changed.

### Checker

- **Root ids (new code, E0051).** For each item of a list literal given to `Response.patch`, the
  checker looks for its root when it can see one: an element written in place; a call of a
  top-level function whose body ends in an element (through `if`, `case` and a block's last
  expression); a fragment of those. A visible root without an `id` attribute is an error:

  ```text
  `Response.patch`: the root of `countView` is a `<p>` without an `id`. A patch replaces the element with the same `id`: give the root one.
  ```

  A root the checker cannot see (a variable, a `for`, a list built elsewhere) is left to the run
  time. An `id` whose value is computed counts as present.
- **Two visible roots with the same literal id** in one `patch` is E0051 too: "`Response.patch`
  has two roots with the id `member-count`".
- **`needsShown`** (does the route render a before-view?) is unchanged: `patch` and `remove` do
  not need one. A handler that may answer `invalid` or `refresh` on another branch still renders
  it. The existing over-approximation, "calls an app function whose result contains `Response`",
  will now catch helpers that only build patches; narrowing it is left to the builder.
- **W0005** (`invalid` without `<Errors form />`) is untouched.

### Runtime

`rt.Response` gets a kind `RespPatch` with its nodes, and `ResponseEvent` gets a removal beside
the toast. Under Datastar the answer is status 200 and, in order:

1. **One `datastar-patch-elements` event with no selector**, mode `outer`, whose elements are all
   the roots. Datastar matches each to the page by `id` (confirmed above). No selector means no
   CSS escaping of ids, which `PatchRegion`'s `#id` form would need.
2. **One event per removal:** `selector` naming the id, `mode remove`, no elements. The selector
   is built by a new `datastar.IDSelector(id)` that escapes the id for CSS (an id may start with
   a digit or hold a `.`); whether `PatchRegion` should use it too is a note for the builder.
3. Toasts, then the dev panel's patch, as for every answer.

The `Alder-Shown` hash is not updated, as after `render`: the guarantee is the partial page's,
that every region the server sent is right.

**A patch the runtime cannot send** (a root without an id, text at the top level, two roots with
one id) sends nothing at all, logs the route and the reason, and answers as a `render` whose
target failed does: no patch, an error toast, `Alder-Status: 500`. The write has committed and
the session it left is stored. Half a patch would leave the page wrong with no sign of it
(ruling R11).

**A root whose id the page lacks** is skipped by the browser with a console warning; the server
cannot know. Tests catch it (below).

**Without JavaScript** a `patch` is a 303 to the referring page, exactly as `render` is. Its
views are built and discarded; that cost is accepted for the fallback path.

**Dev panel:** the Patches tab lists `patch #member-count` and `remove #member-3`, and the answer
line reads "patch (1 element, 1 removed)".

**Equality and printing:** two `patch` responses are equal when their roots render to the same
markup, in order, with the same removals and toasts. `Test.run` prints one as
`Response.patch [<p id="member-count">…</p>] |> Response.remove "member-3"`, the roots as a
failed `Doc` comparison prints an element.

### Tests through the browser

`Answer` gains a constructor, so that one comparison says everything a step did:

```text
Patched { replaced: List Text, removed: List Text }      -- the ids, in the order sent
```

```alder
let r <- Test.click (Remove { id: ada })
Task.succeed ({ answer: r.answer, count: Test.text "#member-count" r.doc }
  == { answer: Patched { replaced: ["member-count"], removed: ["member-" ++ ada] }, count: Just "1 members" })
```

`r.doc` is the page the browser holds after the patch, so every existing query (`Test.text`,
`Test.has`) reads the patched elements; no new accessor is needed. `Patched` joins the names
reserved for `Answer` (E0012 for an app union that uses it).

What the browser checks on such a step, beside what it checks today:

- **Nowhere to go.** A root or a removal whose id the held page lacks fails the step, as for
  `render`: "DELETE /members/3: the answer patched `#member-count`, which the page does not
  have" (and "…removed `#member-3`, which…"). A real browser would skip it silently.
- **The fresh-load check.** The browser loads the held page again in a savepoint, as the element
  check does for `render`, and compares: each replaced element must equal the element with that
  id in the fresh page, and no removed id may be in it. A difference fails the step: "DELETE
  /members/3: the answer patched `#member-count` unlike the page loaded again (GET /members) at
  p#member-count", with both markups. An id the fresh page does not have is not compared. This is
  the check that makes `patch` as trustworthy as `render`: the handler built the view from its
  own data, and the test proves the page's own `load` agrees (ruling R12).
- It does not see a place the handler forgot. `Test.refresh` after the step does, as the book
  already says for `render`.

### How #309 and #310 meet

`render r` re-runs `r`'s `load`, and with it whatever access check that `load` makes. `patch`
does not: the views show what the handler loaded, under the checks the handler ran. So the data
for a patch must come from the same checked query the page uses, and before #310 that query
could not be shared: it had to be written in each route module or reduced to a pure function
over rows.

With both changes the shape is:

```alder
-- Table/Member.alder: the one definition of "the members this request may see"
visible : Query (List Member) requires org
count   : Query Int requires org

-- Members/Index.alder
Index.load params session = {
  let members <- Member.visible
  let n <- Member.count
  Query.succeed { members: members, count: n }
}

Remove.delete p session = {
  let _ <- Db.deleteScoped Member p.id
  let n <- Member.count
  Task.succeed (Response.patch [ countView n ] |> Response.remove ("member-" ++ p.id))
}
```

- `Index.load` and `Remove.delete` are members of routes that `requires org`, so both may call
  `Member.count`. A handler on a route without `org` gets E0026 at that call, naming
  `Db.scoped Member`.
- `countView` is `Int -> Html`: views are pure, never run a query and so never have a need. Only
  the data path is checked, which is the part that matters.
- The view function is shared by the page and the patch; the query is shared by the `load` and
  the handler. The fresh-load check in tests ties the two together.
- A handler may pass its own `session` to a GET route's view (`Row.view member session`) when
  the two routes require the same keys, as one route's view calls another's today.

The book's pitfalls appendix gets the rule in one line: a patch shows what its handler loaded,
so load it with the query the page uses.

### What it touches

- **Stdlib:** `stdlib/Response.alder` (two functions with doc comments), the `Answer` type and
  its reserved names, appendix D (generated and tested).
- **Compiler:** the root-id check (E0051) beside `needsShown`; E0045 for `remove` in place on the
  wrong base; lowering of the two builders; `explain.go`.
- **Runtime:** `rt/response.go` (kind, events, `Code`, equality, printing), `rt/web/answer.go`
  (a `RespPatch` arm, the plain-HTTP table, the failure path), `rt/datastar` (the no-selector
  event, `IDSelector`, the remove event), `rt/web/devpanel.go`, `rt/testing/browser.go` (apply
  the events to the held document, `Patched`, the two checks).
- **LSP, formatter, tree-sitter:** nothing beyond the stdlib's completion and hover data, which
  are generated from the doc comments.
- **Generators:** none change by default. `new resource`'s destroy route keeps `refresh`; a
  flag for patch answers can follow once the pattern has been used by hand.
- **Mutation testing:** if `internal/mutate` swaps `Response` builders, `patch` joins the table.
- **Book:** chapter 6 "`Response`" (the builders, the one-sentence rule, the list of builders
  "not yet"), "A write's response, with JavaScript" and "without", "What the patterns cost" (a
  new row), the event-format section (the two events); chapter 5 "Partial updates"; chapter 9;
  chapter 10 (`Answer`, "What the browser checks on every step", section 10's client); appendix
  B.
- **Examples:** `examples/workspace`'s project removal and `examples/todos`' delete move from
  `refresh` to `patch` + `remove`, each with a browser test.
- **Editors:** nothing.

### Tests of the feature

- `rt/datastar`: the event bytes for several roots, for a removal, for an id that needs
  escaping.
- `cmd/alder` browser tests (real headless browser, as `TestBrowser_DatastarMultiRoot`): two
  roots land on two elements; a removal removes; a missing target leaves the others applied.
- `rt/web`: the 303 without JavaScript; the all-or-nothing failure; toasts after patches.
- `rt/testing`: `Patched`, nowhere-to-go, the fresh-load check passing and failing.
- Corpus: E0051 for an in-place root, for a called view and for duplicate ids; E0045 for
  `remove` after `refresh`.

### Alternatives rejected

- **A `Patch` item type (`Response.patch [ Patch.view v, Patch.remove id ]`).** Every view would
  need a wrapper to cover the rarer case. The modifier keeps the common call at
  `Response.patch [ view ]` and matches `toast`.
- **Retire `render r` in favour of `patch`.** `render` is one line for the commonest case (a
  toggled row), it is what a `click` on a GET route does, and its element check is the strongest
  test the browser has. It stays, described as the shorthand it is (ruling R10).
- **One event per root with `selector #id`,** as `refresh` sends its regions. It needs CSS
  escaping and buys nothing: Datastar already matches roots by id.
- **Skip a bad root and send the rest.** A page that is partly updated looks right and is not.
- **Selectors instead of ids.** Ids are what regions, `render` and the test browser already
  speak, and an id cannot match two things by accident.
- **Append and prepend now.** "Add a row" can patch the list's container today
  (`<ul id="members">`), at the cost of loading the list. Datastar has the modes, and
  `Response.append : Text -> List Html -> Response -> Response` fits the modifier style when it
  is wanted. It is left out to keep the first step small and because an appended element has no
  counterpart to check against by id alone (ruling R13).

### Open questions

1. **`patch` or `replace` as the name** (ruling R9). Recommended: `patch`.
2. **Is the fresh-load check too strict?** A handler that patches in something a fresh page
   would not show (a "saved" badge) fails it. The state rule already says such things belong in
   a toast or the URL. Recommended: keep it strict, with no opt-out, and revisit if a real case
   appears (ruling R12).
3. **Should `needsShown` learn that a helper returns only patches?** Recommended: leave to the
   builder; it is a cost, not a correctness, matter.

---

## 4. #311: `Text.Email` is lower-case by construction

### Problem

`Ada@Example.com` and `ada@example.com` are one mailbox to everyone who sends mail, and two
values to Alder: `==` differs, `.eq` differs, and `@unique` stores both. App code cannot fold the
case itself, because it cannot construct a `Text.Email`.

### Design

A `Text.Email` holds its address in lower case. Every place one comes into being folds it:

| Where | Today | After |
|---|---|---|
| decoded from a form or a route | checked | lower-cased, then checked |
| `Text.Email "Ada@Example.com"` (the `Text` module, tests) | taken as is | lower-cased; a literal at compile time |
| read from a database row | taken as is | lower-cased |
| `Text.toEmail : Text -> Maybe Text.Email` (new) | not possible | lower-cased, `Nothing` when it does not look like an address |
| drawn by a property test's generator | any case | lower case |

- **The whole address is folded,** local part and domain. The standard allows a case-sensitive
  local part; no mail provider of note has one, and an app that treats `Ada@` and `ada@` as two
  people has the bug this issue reports (ruling R16).
- **The fold is Go's `strings.ToLower`:** Unicode simple case mapping, no locale tailoring. So
  `É` becomes `é` everywhere in Alder, and it is the same function `Text.toLower` uses. No
  punycode, no Unicode normalisation: `é` as one code point and as `e` plus a combining accent
  stay different, as they are for `Text`.
- **The check is unchanged** (one `@`, text on both sides, no whitespace). Surrounding spaces are
  still refused, not trimmed.
- With every value folded, `==`, `Dict` keys, `.eq`, `.in` and `@unique` agree without any of
  them changing. An email in one table equals the same email in another, which a column
  attribute could not promise.
- **`Text.toEmail`** fills a gap the issue implies: an address that arrives as `Text` (a line of
  an import, a field of a JSON body) has no way to become a `Text.Email` today.
- `Test.submit`'s promise "decoding it gives back `f`" gains one exception beside `Just ""`: an
  email comes back lower-cased, which it already is if it was built with `Text.Email`.

What is lost: the capitals a person typed, for display. An app that wants them keeps a second
`Text` column.

### Existing rows

A database written before this may hold mixed-case emails, and a row read from it would compare
unequal to what is stored. So the stored values are folded once, by a migration:

- A table's snapshot (the `to` of a migration file) records, per `Text.Email` column, that it is
  folded. A column without the mark gets a data step from `alder migrate`:

  ```text
  -- step: fold Text.Email column User.email to lower case
  UPDATE "User__User" SET "email" = lower("email") WHERE "email" <> lower("email");
  ```

- The app refuses to start until the migration exists, as for any other difference between the
  program and the files; `alder check` says the same in its migration hint.
- On a `@unique` column, two rows that differ only in case collide. The migration then fails
  when it runs and changes nothing, as adding `@unique` over duplicates does. The step's comment
  gives the query that lists the pairs, and the fix is the developer's (merge or delete, with a
  statement added at the top of the file, the existing rule).
- **SQLite's `lower()` folds ASCII only,** and it is what the step uses, since a migration is
  SQL and must also run on Turso. A stored address with a non-ASCII capital (`É@…`) is not
  folded by it, would be read as `é@…`, and would then not be found by `.eq`. The step's comment
  says so and gives a query that finds such rows (the exact query is the builder's); in
  practice they are almost always none. The book states the limit in one sentence.
- A silo's databases get the same step, as they get every migration.

### What it touches

- **Runtime:** `rt/web/decode.go` (fold before `Email`), the row decoder for a `Text.Email`
  column, `rt/std` for `Text.toEmail` and the constructor, the fuzz generator (`ir/fuzz.go`
  names it).
- **Compiler:** the brand constructor of `Text.Email` lowers to the folding function, and folds a
  literal; `internal/migrate` learns the mark and the step; `stdlib/Text.alder` (the type's doc
  comment, `toEmail`).
- **Generators:** `new auth`'s login looks a user up by `.eq`; it needs no change, and its test
  gains a mixed-case login.
- **LSP, formatter, tree-sitter, editors:** nothing.
- **Book:** chapter 4 "Refinements" (the one brand that changes its value, and why), chapter 8
  (the migration step, the ASCII note), chapter 9 and appendix D (`toEmail`), chapter 10
  (`Test.submit`'s exception), appendix B.
- **Examples:** each example gains the migration file; `examples/blog`'s sign-up gets a test
  that a second sign-up differing in case is refused.

### Tests

- Decode, construct, read and `toEmail` each fold; non-ASCII folds in Alder.
- A `@unique` email column refuses a case-variant insert.
- Migration: a fixture database with mixed-case rows is folded; one with a collision fails
  cleanly and is unchanged; a second `alder migrate` writes nothing.
- A property: for any generated `Text.Email` `e`, `Text.toLower e == e`.

### Alternatives rejected

- **`@unique @nocase` (SQLite `COLLATE NOCASE`).** It fixes the index and `.eq` for the columns
  that remember to say it, and nothing else: `==` in Alder code stays case-sensitive, so an
  invitation's email compared with a user's in a handler has the original bug; each new table
  with an email must opt in; changing a column's collation is a table rebuild; and `NOCASE`
  folds ASCII only, for ever, where the type folds Unicode for every new value. It does keep the
  typed capitals, which is its one advantage. It remains a reasonable feature for columns that
  are not emails (a username, a tag), and is listed as a follow-up.
- **A general normalising brand (`brand Text where check normalise f`).** A language feature
  for one stdlib type. `Text.Secret` already shows that a stdlib brand can have behaviour of its
  own; `Email` joins it.
- **Fold only the domain.** It is the letter of the standard and leaves the reported bug in
  place.
- **Fold on comparison, store as typed.** Every comparison site (Alder `==`, SQL `=`, the index,
  `Dict`) would need to know the type; one of them would be missed.

### Open questions

1. **Normalise or `@nocase`** (ruling R15), and **whole address or domain** (ruling R16).
2. **Non-ASCII stored capitals** are reported, not repaired. Recommended: accept; a Go-side
   repair pass at start-up is possible later if anyone has such rows.
3. **Trim surrounding whitespace** before the check (a pasted address often has a space).
   Recommended: not here; it applies to every text input and is its own question.

---

## Order and PR split

Proposed order: #308, then #310, then #309, then #311. #308 goes first because it is mechanical
and every example written for the other three then uses the final names once. #310 precedes #309
because the patch examples and the book's guidance rely on shared queries. #311 depends on
nothing and can run beside any of them.

| PR | Contents |
|---|---|
| 1 | #308 compiler: method-named handlers, W0013 with its fix, `alder fmt` migration, LSP completion and action, corpus |
| 2 | #308 sweep: generators and goldens, examples, book, editor repos |
| 3 | #310 A: needs in any module (the `narrowed` change), the through-note in E0026 and E0031, corpus and mutate fixtures. No syntax yet; helpers outside route modules compile with inferred needs |
| 4 | #310 B: the `requires` clause (parser, formatter, tree-sitter, checker rules 1 to 3, W0014 and W0015, fixes), LSP hover, completion and quick fixes |
| 5 | #310 sweep: `examples/workspace` and `examples/todos` restructured, book chapters 3, 6, 8, 10, 14 and appendices, editor grammar bumps |
| 6 | #309 runtime: `Response.patch` and `Response.remove`, Datastar events, plain-HTTP fallback, E0051 and E0045, dev panel, real-browser tests |
| 7 | #309 tests and docs: `Patched`, the two browser checks, examples moved to `patch`, book chapters 5, 6, 9, 10 |
| 8 | #311: folding, `Text.toEmail`, the migration step, examples' migrations, book |

PR 3 is useful on its own and unblocks apps at once; if the user rules for "inference only" in
R2, PR 4 shrinks to hover. Each sweep PR ends with the book's snippet tests and every example's
`alder test`.

## Follow-ups to file

1. `Response.append` / `prepend` (and what the test browser checks for them).
2. A job form that takes a partition key without a silo.
3. Generators that write shared queries into the table module.
4. `@nocase` for text columns that are not emails.
5. Narrow `needsShown` so a helper that only builds patches costs no before-view.
6. Trim surrounding whitespace in decoded text inputs.

## Left to the builder

- The exact codes (W0013 to W0015 and E0051 are the next free ones today).
- How a migration snapshot records "this email column is folded".
- Whether `PatchRegion` moves to `IDSelector`.
- The wording of `alder explain` for E0026, E0031, E0043 and E0051.
- How `Response` prints long roots in a failed comparison.

## Rulings needed

Each with the options and the recommendation. Sixteen in all.

**#310**

- **R1. Where the need lives.** (a) A checker analysis with a signature clause; (b) in the type,
  `Query` indexed by keys; (c) the key passed as an argument. *Recommended: (a).*
- **R2. The clause.** (a) Required wherever a value has a need, at once in modules without
  routes and after one release of W0014 in route modules; (b) optional everywhere, checked when
  written, hover shows the inferred need; (c) no clause, inference and hover only (PR 3 alone).
  *Recommended: (a): every top-level value already states its type, callers are checked against
  signatures alone, and errors land on the helper. (b) is the issue's wording and the fallback.*
- **R3. The word.** (a) `requires`, as on the route line; (b) `needs`. *Recommended: (a).*
- **R4. A clause naming a key the body does not use.** (a) Warning (W0015); (b) error;
  (c) silent. *Recommended: (a).*
- **R5. Jobs.** (a) Unchanged: `silo k` is the only way a job has a key, `user` never; (b) add a
  job form for a partition without a silo now. *Recommended: (a), with (b) as a follow-up.*
- **R6. Tests.** (a) Stay checked at run time, in any module; (b) the checker tries to prove a
  `Test.login` precedes the use. *Recommended: (a).*
- **R7. E0031.** (a) Unchanged apart from the "(through `Helper`)" note; (b) fold it into the
  need analysis. *Recommended: (a).*

**#309**

- **R8. The shape.** (a) `patch : List Html -> Response` and `remove : Text -> Response ->
  Response` as a modifier valid on `patch` and `render`; (b) a `Patch` item type; (c) `remove :
  Text -> Response` as a base response, so it cannot be combined. *Recommended: (a).*
- **R9. The name, given `R.patch` handlers.** (a) `Response.patch`; (b) `Response.replace`.
  *Recommended: (a): it is the book's and Datastar's word, and the qualifier disambiguates.*
- **R10. `render r`.** (a) Keep, as the shorthand for one route's view; (b) deprecate in favour
  of `patch`. *Recommended: (a).*
- **R11. A patch the runtime cannot send (a root without an id, duplicate ids).** (a) Send
  nothing, log, error toast, status 500 in `Alder-Status`; (b) send the good roots and log.
  *Recommended: (a).*
- **R12. The test browser after a patch.** (a) Fail a step whose target is missing, and compare
  each replaced element with the page loaded again; (b) only the missing-target check, leaving
  agreement to `Test.refresh`; (c) no automatic check. *Recommended: (a).*
- **R13. Scope.** (a) Replace and remove only; (b) append and prepend too. *Recommended: (a).*

**#308**

- **R14. Migration tool.** (a) W0013 for one release, fixed by `alder fmt` and the editor's
  quick fix; (b) bring back an `alder fix` command that applies every fix-carrying diagnostic;
  (c) rename at once with no deprecation release. *Recommended: (a): `alder fmt` already
  migrates import qualifiers, and `alder fix` was removed on purpose. If (b) is preferred, the
  W0014 clause fix would use it too.*

**#311**

- **R15. The mechanism.** (a) `Text.Email` folds to lower case wherever one is made, plus
  `Text.toEmail`; (b) `@nocase` on the column; (c) both. *Recommended: (a), with `@nocase`
  filed as a follow-up for other columns.*
- **R16. How much is folded.** (a) The whole address; (b) the domain only. *Recommended: (a).*

## Rulings (user, 2026-11-24)

Ruled one by one. Where a ruling departs from the recommendation, the sections above are superseded by it.

- R1 (a) checker analysis with a signature clause.
- R2 (a) clause required: at once in modules without routes; W0014 for one release in route modules.
- R3 (a) `requires`.
- R4 (a) warning, W0015.
- R5 (a) jobs unchanged (`silo k` only); a partition form for jobs is a follow-up issue.
- R6 (a) tests anywhere, key checked at run time.
- R7 (a) E0031 unchanged apart from the "(through `Helper`)" note.
- R8 (a) `List Html -> Response` plus a `remove : Text -> Response -> Response` modifier.
- R9 **(b) `Response.replace`**, not `Response.patch` (departs from the recommendation): it says what happens to each element and does not read against `R.patch` handlers. Rename throughout #309's design, tests (`Answer` constructor names included) and book.
- R10 **(b) deprecate `render r`** (departs from the recommendation): one mechanism; handlers load what they need and answer `Response.replace [ Row.view row s ]`. `render` warns for one release (like W0013), with the rewrite in the message, then is removed.
- R11 (a) send nothing, log, error toast, 500 in `Alder-Status`.
- R12 (a) fail a step whose target is missing, and compare each replaced element with the page loaded again.
- R13 (a) replace and remove only.
- R14 (a) W0013 for one release, fixed by `alder fmt` and the editor's quick fix.
- R15 (a) `Text.Email` folds wherever one is made, plus `Text.toEmail`; `@nocase` filed as a follow-up for other columns.
- R16 (a) the whole address.
