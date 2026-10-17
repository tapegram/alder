# M15: Partitions and silos — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `@partition <key>` generalises `@owner` with generic scoped operations and `requires <key>`; `via` makes membership a declaration the runtime enforces; `silo <key> { … }` gives a database per key value with a one-store-per-Task rule, open-time schema checks and a Turso deploy target; the admin, generators, a `workspace` example, the book and an experiment task follow.

**Architecture:** (1) Language: `@partition`, `via`, `silo`, `choose`, `maintenance`, the generic scoped ops, E0031–E0033, the write-set analysis. (2) Runtime: a store per silo value, open-time schema checks, `via` enforcement, cross-store reads, tests' per-silo memory stores, `Test.member`; the mutation operators. (3) Deploy: the `Turso` target (schema databases, children, sample preflight, status, PITR restore), silos as files on `Ssh`, `alder migrate silos`. (4) Admin/generators/example/book/reference/experiment task.

**Tech Stack:** Go; modernc SQLite (many connections); Turso platform API over `net/http` (+ libSQL over HTTP for children? — RULING: the runtime talks to Turso through its HTTP/`hrana` protocol via `net/http` only; no new Go dependency; the implementer verifies the protocol surface needed: open, exec, batch, and documents it).

**Spec:** `docs/superpowers/specs/2026-10-17-m15-partitions-silos-design.md` (binding: the surface, every E003x text, the one-store rule, the open-time outcomes, the Turso behaviours).

## Global Constraints

- `@owner` and the `mine` family keep working byte-for-byte: M11's corpus, goldens, examples and scores are unchanged (todos 1.00, blog ≥ 0.95 as today); migrations of the existing examples untouched.
- A scoped operation on a siloed table can never read or write another key value's database: proven by a runtime test and a browser test (two orgs).
- No secret or key value appears in a diagnostic beyond what the source already shows; silo filenames are the ids' text (filename-safe by construction).
- Imports stay stdlib + `alder/*` + modernc; Turso via `net/http`.
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...` (all tags); `go test ./... -count=1 -timeout 30m`; `-tags browser`; `-tags deploy` in CI; all three examples check/build/test + `--mutate`; `alder new` out of tree; experiments unittest (reference ≤ 48k); `go test ./book`; the drill with `t11-workspace`.
- Commit messages end with the session trailers; stage by explicit path.

## Review Focus

1. **A Task that reads from the shared store and writes to a silo, and one that writes to both**: the first is fine (the read runs on a read-only connection outside the silo transaction), the second is E0031 with the ruled text, also when the shared write hides in a helper or a `Task.all`. Task 1 (corpus) + Task 2 (runtime: the read-only connection cannot write).
2. **A crafted `Session.put org` to a non-member org, and a membership deleted in the same Task before the put**: refused (effect failure) both times; `membership dropped` killed by the example's tests. Task 2 + Task 4.
3. **A silo database that is behind by a data-dropping migration, and one that is ahead**: the maintenance page for that org only, other orgs served, `status` lists them, `alder migrate silos --allow-data-loss` fixes the first, the restore command named for the second. Task 2 + Task 3.
4. **Turso deploy where one of the sampled children fails the plan**: nothing switched, the shared store's backup intact, the failure named with the child; a straggler after the switch shows as `migrating` then `ok`. Task 3 (fake platform API).
5. **The admin on a siloed table as an Admin who is not a member of the org**: the picker works, the pages run in that org's database via the page parameter and the role check, a Reader gets 404, the bulk actions stay in one silo. Task 4.

---

### Task 1: Language — `@partition`, `via`, `silo`, `choose`, `maintenance`, generic scoped ops, E0031–E0033

**Files:** `internal/syntax` (attributes `@partition k`; session field `via T`; `silo k { … }` and `choose k = Page`, `maintenance = Page`? (a `maintenance : Html Never` value like `notFound`) in `App`), `internal/check` (`rules.go` generalised: partition key per table, `requires k`, legality by narrowing, `via` validation, `silo` rules, E0031 write-set analysis over arms incl. helpers/`Task.all`/fakes, E0032/E0033 texts, `choose`/`maintenance` checks), `internal/diag` (E0031–E0033 + explain; the generalised E0024–E0027 texts), `internal/ir` (`Table.Partition{Key, Column}`; `Session.Via map[field]table`; `Silos map[key][]table`; `Page.Requires []string` incl. keys; `Choose map[key]page`; the scoped nodes generalised: `DbScoped{Table}` etc. with the key resolved; `Arm.Store` = shared|silo(key) from the analysis), `stdlib/Db.alder` (the five `*Scoped` signatures), corpus (`partitions-ok` with org + user keys, `silo-ok`, every E003x and generalised text), goldens, `alder dump`, chapter 6/8/9/11 rows (full chapters in Task 4).
- [ ] RED → implement → GREEN → commits (syntax+IR; checker+diag+corpus; stdlib+goldens).

**Interfaces (binding):** `ir.Table.Partition *Partition{Key, Column string}`; `ir.Program.Silos map[string][]*Table`; `ir.Program.Via map[string]*Table`; `ir.Program.Choose map[string]*Page`; `ir.Arm.Store` (`StoreShared` | `StoreSilo(key)`); the scoped nodes carry `Key string`.

### Task 2: Runtime — stores per silo, open-time checks, `via`, cross-store reads, tests, operators

**Files:** `rt/db` (a `Stores` registry: shared + `silo/<key>/<value>` opened lazily; the schema-hash row and the open-time check with its three outcomes; migrate-on-open for non-destructive pending migrations; read-only connections for cross-store reads), `rt/eff` (a Task's transaction bound to its arm's store; the `via` check inside `Session.put`; the maintenance answer), `rt/web` (the maintenance page route per silo; `requires k` redirect to `choose k`'s page), `internal/gen` (the arm's store; the key resolution; `Session.put` with `via`), `rt/testing` (a `:memory:` store per silo value touched; `Test.member user org`; answers unchanged), `internal/mutate` (`scope dropped` over every key; `membership dropped`), `gen.Main` (`alder migrate silos` support flags), tests: rt/db (stores, open-time outcomes incl. the data-loss and ahead cases, filename safety), rt/eff (`via` incl. the same-Task deletion), an e2e fixture app (`testdata/corpus/silo-ok` runnable with browser tests: two orgs, non-member refused, cross-store read ok, write mix refused at check), mutate goldens.
- [ ] RED → implement → GREEN → commits (stores + checks; via + eff; testing + operators; fixture).

### Task 3: Deploy — the `Turso` target, silos on `Ssh`, `alder migrate silos`, status

**Files:** `internal/deploy/turso.go` (platform API: databases, schema databases, children; the Locker and BackupRestorer; the sample preflight; propagation status; PITR restore; health by build id; behind `-tags turso` for a real run, a fake platform API for the default suite), `internal/deploy/ssh.go` (silo files under `db/silos/`, backed up per file by the shipper with a documented bound), `cmd/alder/{deploy,status,restore,migrate}.go` (`status`'s silo table; `restore --silo k:v --to T`; `migrate silos <env> [--allow-data-loss]`), `rt/backup` (per-file shipping for silos), tests (fake Turso: create/migrate/children/sample failure RF4/straggler; Ssh silos via `ALDER_DEPLOY_LOCAL=1`), chapter 16's Turso section (Task 4 finalises).
- [ ] RED → implement → GREEN → commits (turso; ssh silos; commands).

### Task 4: Admin, generators, the `workspace` example, book, reference, experiment task

**Files:** `internal/generate/admin.go` (the picker for siloed tables; `requires user, role Admin, silo k` pages with the key parameter; bulk within a silo; dashboard counts on demand), `internal/generate` (`new table … @partition=k`; `new silo k T U`; `new page --requires k`; `edit session add-field k:T via M`), goldens + mutation gates, `examples/workspace` (orgs, memberships, projects, tasks, a partitioned audit table, `choose org`, the admin; browser tests; ≥ 0.95), `experiments/tasks/t11-workspace.md` + hidden tests + the drill script (commands only), `TestExample_Workspace`, book: chapters 6/8/9/10/11/12/13/14/15/16 per the spec + README; the reference (≤ 48k; the partition rules, the one-store rule, the commands).
- [ ] implement → verify → commits (admin + generators; example + task; book + reference).

---

## Self-review notes

- Spec coverage: surface (T1), runtime/tests/operators (T2), deploy (T3), admin/generators/example/book (T4); non-goals untouched.
- Interfaces: T2/T3/T4 consume T1's IR; T4 consumes T2's `Test.member` and T3's commands.
- Review Focus 1–5 each have a test.
- Open to the implementer: the hrana/HTTP surface the runtime needs for Turso children (document; refuse unsupported features with a sentence).
