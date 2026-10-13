# M11: Declared rules — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `@owner` on a table and five scoped `Db` operations make "this row is mine" a one-line, checker-enforced, not-found-answering rule; `requires user, role R` makes a role check a cookie-level declaration; two mutation operators target them; the examples and the book use them.

**Architecture:** (1) Language: parser attributes, checker rules E0024–E0027 and the narrowed `session.role`, IR fields/nodes, lowering. (2) Codegen and runtime: the owner predicate in SQL, rows-affected not-found for scoped writes, the route's role check, the two mutation operators. (3) Examples, book, reference.

**Tech Stack:** Go; the existing `Db` lowering/emission and the route emitter.

**Spec:** `docs/superpowers/specs/2026-10-12-m11-declared-rules-design.md` (binding: the surface, the texts, the not-found semantics, where scoped operations are legal).

## Global Constraints

- Exactly the spec's surface and texts; `@owner` on exactly one `@ref` column of the session's user type; scoped operations legal only where `session.user` is narrowed (a page that `requires user` and its module's values); scoped reads/writes of a row that is not yours answer not-found; `insertMine` forbids naming the owner column.
- `@owner` and roles change nothing in the stored schema or migrations (`schema.json` unchanged for both examples; `alder migrate` says no changes after the examples adopt `@owner`).
- Scoped writes carry the owner predicate and a rows-affected check: `deleteMine` is one statement (`DELETE … WHERE id = ? AND owner = ?`); `updateMine` is a predicated read then a predicated write (`UPDATE … WHERE id = ? AND owner = ?`), both in the Task's transaction (never an unpredicated read-then-write). (Wording corrected in the final fix wave, m1.)
- The trace names the operations as written (`Db.mine Todo`, …).
- Both examples adopt the rules; their behaviour and test outcomes are unchanged except where tests referred to removed helpers; mutation scores stay ≥ today's (todos 1.00; blog 0.95 or better).
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...`; `go test ./... -count=1`; both examples check/build/test + `--mutate`; `(cd experiments && python3 -m unittest)`; `go test ./book -count=1`.
- Commit messages end with the session trailers; stage by explicit path.

## Review Focus

1. **A scoped operation in `App.update` or in a helper of a module whose page has no `requires user`**: expected — E0026 naming the page; a helper in a `requires user` page module is fine. Test in Task 1 (corpus) and Task 2 (e2e: the helper form).
2. **`Db.updateMine` on a row that exists but belongs to someone else**, inside a Task that did other writes first: expected — `Task.notFound` and the WHOLE transaction rolls back as any not-found does today; the row is unchanged; the trace shows `Db.updateMine Todo`. Test in Task 2 (rt/db + e2e).
3. **A role check versus a stale cookie**: a user demoted after login still passes `requires user, role Author` until re-login; the book says so and shows the re-read idiom; a test demonstrates both (cookie passes; the idiom's `load` 404s). Test in Task 3 (the blog's tests) and Task 2 (e2e).
4. **`scope dropped` mutant on `Db.getMine`**: the replacement must be well-typed (a `Query t` from `Db.get` + not-found on `Nothing`), and the examples' ownership tests must kill it (the stranger sees the row). Test in Task 2 (mutate golden + examples' scores).
5. **Migrations untouched**: adopting `@owner` in the blog/todos produces no migration and no `schema.json` diff; a `@renamed` owner column still works. Test in Task 1/3 (`alder migrate` → no changes; `git diff -- examples/*/migrations` empty).

---

### Task 1: Language — parser, checker, IR

**Files:** `internal/syntax` (`@owner` attribute on a field; `requires user, role R [| S]` on a page; dump), `internal/names` (nothing new unless `role` constructors need resolving against the session's union), `internal/check` (`decls.go`: `@owner` validation E0024 ×3; `Page.Roles` vs the session's `role` field E0027 ×2; `expr.go`/a new `rules.go`: the scoped operations' typing — `Db.mine/getMine/updateMine/deleteMine/insertMine` with E0025 (no owner), E0026 (not under `requires user`), the `insertMine` owner-field E0003; the narrowed `session.role` type inside the page), `internal/diag` (E0024–E0027 + explain), `internal/ir` (`Table.Owner`, `Page.Roles`, nodes `DbMine{Table}` (a Select base), `DbGetMine`, `DbUpdateMine`, `DbDeleteMine`, `DbInsertMine` — or `Scoped bool` on the existing nodes: implementer's call, stated), `lower*.go`, `dump.go`, `stdlib/Db.alder` (the five signatures with comments), corpus (`rules-ok`; the error cases), IR golden.
- [ ] RED → implement → GREEN (`go test ./internal/... -count=1`; examples still check since they do not use the surface yet) → commit.

**Interfaces (binding for Task 2):** `ir.Table.Owner string` (column name, "" if none); `ir.Page.Roles []string`; the five IR nodes with `Table *Table`, `ID Expr`, `Fn Expr` (update), `Row Expr` (insert); `Program` unchanged otherwise.

### Task 2: Codegen, runtime, mutation operators

**Files:** `rt/db` (`UpdateWhere`/`DeleteWhere` returning rows affected with an owner predicate; `Select` with the owner predicate for `mine`), `rt/eff` (nothing, or a helper to raise `ErrNotFound` on 0 rows), `internal/gen` (`effects.go`: the five nodes; `routes.go`: the role check after the `requires user` redirect — 404 document; `caps.go`: the session's narrowed role; `mutation.go` + `internal/mutate`: `scope dropped` (five forms) and `role dropped`), goldens (`gen_rules`; mutate goldens), e2e fixture app (`testdata/corpus/rules-ok` extended with an App session + roles + browser tests: stranger `PageMissing`; Reader on an Author page `PageMissing`; visitor `Redirected`; stale-page click → not found, nothing changed; the helper form; the stale-cookie demonstration), `rt/db` tests (RF2), `cmd/alder/test_test.go` (the fixture under `alder test` and `--mutate`: every mutant killed).
- [ ] RED → implement → GREEN (full gate) → commits (rt+gen; mutate).

### Task 3: Examples, book, reference

**Files:** `examples/todos` (`@owner`; `Db.mine`/`getMine`/`updateMine`/`deleteMine`; `owned` removed; tests adjusted; score re-recorded), `examples/blog` (`author @owner` on Post; `role` in the session set at login; `requires user, role Author` on Admin pages; `author` lookup removed; `Admin.Edit.load` via `getMine`; a demotion test per RF3; score re-recorded), `migrations/` unchanged (RF5), `TestExample_*`, chapter 14 re-synced; `book/06-pages.md` ("Rules" section), 08 (`@owner`), 09 (`Db`), 10 (the two operators; the todos survivors list re-recorded), 11 (E0024–E0027), 12 (why declared; not-found; session roles; the replica argument), 13 (the SQL and rows-affected; the route's role check), README; `experiments/build_context.py` (the reference keeps the rules); `experiments/tasks/` Names blocks may mention the operations (E10's call; a one-line note in `tasks/README.md`).
- [ ] examples → book → full gate → commits (examples; book).

---

## Self-review notes

- Spec coverage: surface (T1/T2), texts (T1), not-found (T2), mutation (T2), examples/book (T3), Turso argument (T3 ch.12), non-goals untouched.
- Interfaces: T2 consumes T1's IR; T3 consumes T2's behaviour and texts.
- Review Focus 1–5 each have a test.
- Numbering: this is M11; the generators/admin milestone becomes M12.
