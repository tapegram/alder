# M12: Generators and the built-in admin — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `alder new page/table/form/msg/capability`, `alder edit table`, `alder fill` and `alder new admin`/`alder regen admin` generate plain, checked, tested Alder; both examples carry a generated admin; a book chapter with verified transcripts teaches it.

**Architecture:** (1) `internal/generate`: program facts from the IR, embedded templates, the copy-check-write pipeline, field syntax, holes, regions; the ordinary commands. (2) The admin generator and `regen` with owned regions, dynamic `<Select options>`, the fixture and its mutation score. (3) Examples, book chapter 15, reference.

**Tech Stack:** Go (`text/template`, `embed`), the existing checker/IR/migration machinery, `alder-ui` elements, M11 rules.

**Spec:** `docs/superpowers/specs/2026-10-13-m12-generators-admin-design.md` (binding: the command surface and refusal texts, the admin's pages and tests, owned regions, the examples' changes).

## Global Constraints

- Every command: compute → apply to a temporary copy → check → write only if the copy checks; refuse (exit 1, one sentence, nothing written) on an existing target, a taken name, a failed check, a failed precondition; `--dry-run` writes nothing; `--json` has the spec's shape. `alder new <app>` keeps its behaviour.
- Generated source is plain Alder with the book's formatting; it uses `alder-ui` elements and M11's rules where the spec says; never a hand-written ownership/role check.
- `alder regen admin` touches only owned regions; idempotent; refuses on a broken marker pair; never recreates a deleted file without `--all`.
- The generated admin's tests kill its mutants: fixture `--mutate` ≥ 0.95 with every survivor listed and judged equivalent in the report; the examples' overall scores ≥ 0.95 (todos' non-admin mutants stay 32/32).
- `alder new admin` never edits `App.alder` or the login; it is refused without a role (the spec's text).
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...`; `go test ./... -count=1`; both examples check/build/test + `--mutate`; `alder new` out of tree; `(cd experiments && python3 -m unittest)`; `go test ./book -count=1`; reference ≤ 46k.
- Commit messages end with the session trailers; stage by explicit path.

## Review Focus

1. **A command whose result does not check** (e.g. `alder new form` into a table whose column types do not match the fields; `alder fill` with a wrong-typed expression): expected — nothing written, the checker's diagnostics printed, exit 1; a second run after fixing works. Task 1.
2. **`alder edit table add-field` without a default on a table that is inserted in three places**: expected — the migration and the field are written, the three inserts are printed as diagnostics with file:line, `--strict` refuses; `alder regen admin` afterwards adds the column to the admin's list/show/forms and keeps a test the author added. Task 1 + Task 2.
3. **The admin on a table with a `@restrict` ref pointing at the row**: the delete page shows the referencing count and no delete button; with `@cascade` it shows what goes with it; the message is refused server-side too (a crafted POST does not delete a restricted row — the DB's own constraint surfaces as a `Reject`, not a 500). Task 2.
4. **A Reader on an admin page and its messages; a visitor; an Admin's stale cookie**: 404 / redirect / per M11's semantics; tests generated for each. Task 2.
5. **A regenerated admin after the author customised a file outside the regions** (renamed a heading, added a column rule, added a test): `regen` keeps every customisation byte-for-byte and rewrites only the regions; a diff shows exactly the regions. Task 2.

---

### Task 1: `internal/generate` and the ordinary commands

**Files:** `internal/generate/` (new: `program.go` — facts from the IR via the same load path `alder check` uses: tables (columns: name, type text, attrs `@id/@created/@updated/@ref T/@cascade/@restrict/@setNull/@owner/@renamed`, defaults), pages (module, path, params, requires, roles), session fields and the role union, modules and their files; `fields.go` — the field syntax parser with tests; `apply.go` — `Apply(appDir, edits []Edit) (Result, error)`: copy the app to a temp dir, apply creates/edits, run the checker, return diagnostics; write on success; `--dry-run`; the JSON result; `holes.go` — the hole index from the checker's W0002 data; `templates/*.tmpl` embedded: `page.tmpl`, `page_list.tmpl`, `page_show.tmpl`, `page_from.tmpl`, `table.tmpl`, `form.tmpl`, `msg.tmpl`, `capability.tmpl` (+ fake + adapter stub + install test), `test_*.tmpl`; `format.go` — the book's formatting), `cmd/alder/new.go` (subcommands `page`, `table`, `form`, `msg`, `capability`; `alder new <app>` unchanged), `cmd/alder/edit.go` (`edit table T add-field|rename-field|drop-field`, migration via the migrate machinery, diagnostics printed, `--strict`), `cmd/alder/fill.go` (`fill <decl> -`, `--hole`, `--list`), `internal/diag` (no new codes; refusal texts live in `generate`), tests: `internal/generate/*_test.go` (fields, regions-free pieces, holes), `cmd/alder/generate_test.go` (each command on a scratch copy of the scaffold and on `testdata/generate/fixture` (a table set with refs/cascade/restrict/unions/owner/roles): goldens of the written files; the app checks and `alder test` passes after each command; refusal texts; `--json`; `--dry-run`; RF1; RF2's first half), book: chapter 11 rows for the commands (the full chapter is Task 3).
- [ ] RED → implement → GREEN → commits (generate core + page/table; form/msg/capability; edit + fill).

**Interfaces (binding for Task 2):** `generate.Program`, `generate.Apply`, `generate.Edit{Path, Create bool, Body string}`/`Edit{Path, Replace []Region}`, the templates' data shapes, the field syntax; the fixture app.

### Task 2: `alder new admin`, `alder regen admin`, owned regions, `<Select options>`

**Files:** `internal/generate/regions.go` (`-- @admin begin <name>` … `-- @admin end`; `Replace`, `Names`, `Check`; tests: replace, idempotent, missing pair, nested text untouched), `internal/generate/admin.go` + `templates/admin_*.tmpl` (dashboard, layout, per-table index/show/new/edit/delete + tests, per the spec), `cmd/alder/new.go` (`admin` with `--module/--path`), `cmd/alder/regen.go`, the form machinery (`<Select field options={list}>` with `(Id t, Text)` options rendered as `<option value>`; decoding an id from the submitted value with the brand check; `al-select` wrapped form carries it; checker + gen + rt/html + corpus + golden — only if dynamic options are absent today; the implementer states), tests: `testdata/generate/fixture` with roles → `alder new admin` goldens; the fixture under `alder test` and `--mutate` (≥ 0.95; survivors listed); `regen` after `edit table add-field` (RF2 second half) and after author customisations (RF5); RF3 (restrict/cascade; the crafted POST); RF4 (Reader/visitor/stale cookie tests generated); the precondition refusal text; `--all`.
- [ ] RED → implement → GREEN → commits (regions + select options; admin generator; regen).

### Task 3: Examples, book, reference

**Files:** `examples/blog` (`Admin`→`Write` rename with paths `/write`, `/write/:id`; `User.Role` gains `Admin`; a user fixture with the role; `alder new admin` run and its output committed verbatim incl. tests; README section with the replayable commands; score ≥ 0.95 overall), `examples/todos` (`role: User.Role = Member`, `Member | Admin`, set at login; `alder new admin`; scores reported both ways), `internal/scaffold` README text (next step: `alder new admin` and what it needs), `cmd/alder` `TestExample_*` counts/scores, `book/15-generators.md` (per the spec; transcripts verified by `book/book_test.go`: a new block kind `command=` that runs the command on a scratch copy of the scaffold or of the blog at the chapter's named state and compares stdout and the written files with the block; the admin tour by `part=` blocks of the blog's generated pages), chapters 2 (if the scaffold README is quoted), 6 (a pointer from Rules to the admin), 11 (`alder new/edit/fill/regen` reference), 12, 13, 14 (`Write`), README (book + root), `book/site/build.py` order, `experiments/build_context.py` (the command table; `--reference --size` ≤ 46k, report), `experiments/tasks/README.md` (a line: E11 consumes these commands).
- [ ] examples → book → full gate → commits (examples; book + reference).

---

## Self-review notes

- Spec coverage: command surface (T1), admin + regen + regions + select options (T2), examples/book/reference (T3); non-goals untouched.
- Interfaces: T2 builds on T1's `Program`/`Apply`/templates/fixture; T3 runs T1/T2's commands on the examples and quotes their output.
- Review Focus 1–5 each have a test.
- Open to the implementer, stated in reports: whether `<Select options>` exists already; where in `view` a generated form/button is inserted when the root element has no `al-card`.
