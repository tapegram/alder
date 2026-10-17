# M15: Partitions and silos — row-level security on any session key, and a database per key — Design

Date: 2026-10-17. Status: approved in conversation. Related: M11 (declared
rules: `@owner` + the scoped `Db` operations, which this generalises), M14
(deploy; the Turso target lands here), the user's direction (2026-10-16):
"per x, something resembling RLS", "database per user" via Turso, "not all
data should be siloed; blog posts are shared".

## Decisions recorded (2026-10-16/17)

- **A partition is a declaration on a table**: `@partition <sessionField>`
  on a `@ref` column; `@owner` is `@partition user` and stays as sugar.
  Logical by default (a predicate in one database).
- **A silo is physical partitioning where declared**: `silo <key> { T, U }`
  in `App`; one database per key value; siloed tables must be partitioned
  by the silo's key.
- **Membership is declared** (B): `session { org: Maybe Org.OrgId via
  Membership }` — the runtime refuses a `Session.put` of `org` to a value
  with no `Membership` row for the session's user.
- **One store per Task** (A): a `Task` writes to the shared store or to
  one silo, never both; reads across are fine; the outbox arrives with
  jobs; an explicit `Task.separately` is held in reserve.
- **Turso: the schema database is the unit, with a preflight sample** (C):
  deploy migrates the shared primary and the schema database under M14's
  gate, verifies the plan on the largest N children, switches, and the
  children follow; `status` reports stragglers.
- **Schema drift is checked per database at open time**: behind → migrate
  on open (never a data-dropping one: maintenance page + `alder migrate
  silos --allow-data-loss`); ahead → refuse that silo with a maintenance
  page and name the restore; match → serve and cache.

## The surface

### `@partition`

```alder
table Project { id: ProjectId @id, org: Org.OrgId @ref Org @partition org, … }
table Todo    { id: TodoId @id, owner: User.UserId @ref User @owner, … }      -- = @partition user
```

- `@partition k` marks exactly one `@ref` column of a table; `k` names a
  session field whose type is `Maybe <the referenced table's id type>`
  (E0032: "`@partition org` needs a session field `org` of type `Maybe
  Org.OrgId`"; "a table has one partition column"; "`@partition` needs a
  session"). `@owner` = `@partition user` (the existing E0024 texts stay).
- The scoped operations become generic over the table's declared key:
  `Db.scoped T` (a Select: `Db.from T` with `WHERE <col> = session.<k>`),
  `Db.getScoped`, `Db.updateScoped` (never writes the partition column),
  `Db.deleteScoped`, `Db.insertScoped` (fills the column; naming it is
  E0003). The `mine` family stays, legal only on tables partitioned by
  `user`, and is literally the same code path. Traces name them as written.
- Legality: a scoped operation on a table partitioned by `k` is legal only
  where `session.k` is narrowed: inside a page module whose page `requires
  k` (and its helpers; the M11 cross-module and fake rules apply, E0026's
  texts generalised: "`Db.scoped Project` needs a page that `requires org`:
  `Projects.Index` does not"). `requires user, org` reads as today's
  `requires user`; `requires org` alone is E0027-style refused when `org`'s
  session field has `via` (membership needs the user): "`org` needs `user`
  first".
- `requires k` on a page: a request whose session lacks `k` is redirected
  to the app's `login` page when `k` is `user`, else to a page the app
  names: `App` gains `choose org = Orgs.Index` (one per key with `via`;
  E0032 if missing): "pick a workspace" is the natural landing.
- A scoped read of a row in another partition is not-found; a write
  changes nothing and is not-found; with silos the row is physically
  absent, so the same answer.
- Mutation operators: `scope dropped` covers every key (the five forms);
  `membership dropped` (below); `partition dropped`? no — the same as
  scope dropped.

### Membership: `via`

```alder
session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership }
table Membership { id: MembershipId @id, user: User.UserId @ref User, org: Org.OrgId @ref Org }
```

- `via M` names a table with exactly one `@ref` to the user table and one
  `@ref` to the key's table (E0032 otherwise). `Session.put` of that field
  to `Just v` fails the effect ("Session.put org: no Membership for this
  user and org") unless a row (user, v) exists at that moment; `Nothing`
  is always allowed; the check runs inside the Task's transaction (a
  membership deleted in the same Task counts as gone). A field with `via`
  requires `user` in the session (checked). Login may set it only through
  the same rule (so a login that picks the user's first org is one
  `Db.first` + `Session.put`). The cookie carries the value as today; it is
  NOT re-verified per request (as roles are not) — the chapter says so and
  shows the re-read idiom (a page's `load` re-checking membership) for
  pages that must notice a removal at once; the admin's silo pages use it.
- Mutation operator `membership dropped`: the `via` check removed → the
  generated and hand-written "a non-member cannot switch" tests must kill
  it.
- A key without `via` (a `region` from a fixed list) is set freely; the
  checker still requires `requires region` where scoped ops run.

### `silo`

```alder
module App
silo org { Project, Task }
```

- At most one `silo` per key; a table appears in at most one silo; every
  listed table must be `@partition`ed by that key (E0033: "`Task` is in
  `silo org` but is not partitioned by `org`"); a `@ref` from a shared
  table to a siloed one is E0033 ("`AuditLog.project` refers to `Project`,
  which lives in `silo org`: a shared table cannot point into a silo");
  a `@ref` from a siloed table to a shared one is fine; a `@ref` between
  two tables of the same silo is fine; between different silos E0033.
- The unscoped `Db.from Project` on a siloed table is legal only where the
  key is narrowed too (there is no "all orgs" query by construction; the
  admin iterates orgs explicitly: `Db.inSilo org (Db.from Project)`? — NO:
  the admin picks an org by setting the session's key for the admin page
  (the picker page `Session.put`s it, via the membership of the ADMIN
  user? an Admin may not be a member) — RULING: an admin page declares
  `requires user, role Admin, silo org` and gets `session.org` from a page
  parameter `{ org: Org.OrgId }` the picker links, bypassing `via` because
  the role check already gates it; the chapter explains).
- **One store per Task (E0031)**: the checker computes each `update` arm's
  (and `App.update` arm's) write set; writes to the shared store and to a
  silo, or to two silos, in one arm → "`Create` writes `Project` (silo
  `org`) and `AuditLog` (shared): a Task writes to one store. Log inside
  the silo, or send the audit entry from its own message." Reads may mix.
  `Session.put` is shared-store-free (the cookie) and does not count.
  Capability calls are outside either store. A fake's entries follow the
  same rule.
- **Runtime**: a `Store` per silo key value, opened lazily on first use
  (`data/silos/<key>/<value>.db` locally; the key's value is the id's
  text, filename-safe by the id brand's rule), with the schema check on
  open (below); the shared store is `data/app.db` as today; a Task runs
  its transaction on the store its write set names (the checker tells
  gen which); reads from the other store use a read-only connection
  outside the transaction. Tests: a fresh `:memory:` database per silo
  value touched, created on first use; `Test.login` with a session that
  sets `org` must satisfy `via` (the test inserts the Membership row, as
  it inserts users today) — a test helper `Test.member user org` inserts
  one.
- **Schema drift at open**: each database stores the schema hash of the
  binary that last migrated it; on open: equal → serve (cached per
  process); behind → apply the pending migrations on open unless one
  drops data → that silo answers the app's `maintenance` page (a new
  optional `App` declaration; a default page exists) and `alder status`
  lists it `pending (data loss)` until `alder migrate silos <env>
  --allow-data-loss`; ahead → the maintenance page, `status` lists it
  `ahead`, naming the restore. The shared store keeps M14's start-time
  check.
- **Deploy on Turso** (the `Turso` target, M14's interface): the shared
  primary database + one schema database per silo; a child database per
  key value created on first use through the platform API; migrations:
  the gate migrates the shared store (backup, DROPS DATA rule) and the
  schema database, runs the plan on the largest N children (`sample`,
  default 5) with the full checks, refuses the switch on any failure,
  then switches and lets propagation run; `status` shows per-silo counts
  `ok/migrating/pending/ahead/failed`; restore for silos = Turso's
  point-in-time restore invoked by `alder restore <env> --silo org:<id>
  --to T`; the Turso target implements Locker and BackupRestorer and
  never restarts an old binary on a migrated database (Fly's deferred
  list is closed here for Turso; Fly itself stays experimental). Locally
  and on `Ssh`, silos are files under `<path>/db/silos/`, backed up by
  the shipper per file (a bounded number; the chapter states the limit
  and points at Turso beyond it).
- **The admin**: for siloed tables, a picker page (the shared key table)
  then the normal generated pages under `requires user, role Admin, silo
  org` with the key from a page parameter; bulk actions stay within one
  silo; the dashboard counts per silo on demand (not all at once).
- **Generators**: `alder new table … org:Org.OrgId@ref=Org@partition=org`;
  `alder new silo org Project Task`; `alder new page … --requires org`
  (and `--list T` on a partitioned table uses `Db.scoped`); `new auth`
  unchanged; `alder edit session add-field org:Org.OrgId via Membership`.

## Book

- Chapter 6 "Rules" grows "Partitions" (the general form; `@owner` as the
  special case; `via`; `requires k`; `choose`), chapter 8 (`@partition`,
  `silo`, the one-store rule, where the files live, the open-time check),
  chapter 9 (the `Db.*Scoped` entries), chapter 10 (`Test.member`; the
  operators; a two-org test), chapter 11 (E0031–E0033, `alder migrate
  silos`, `status`'s silo table), chapter 12 (why a declaration; why one
  store per Task; why not two-phase; why open-time checks; why Turso's
  schema database), chapter 13 (the store per silo; the write-set
  analysis; the open-time check; the Turso target), chapter 14/15/16
  updates, the reference (≤ 48k), a new example? — RULING: `examples/
  workspace` (orgs, memberships, projects, tasks; shared audit via a
  partitioned table; the admin's picker) with ≥ 0.95, and the experiment
  task `t11-workspace` with hidden tests.

## Testing

- Corpus: E0031–E0033 and the generalised E0024–E0027 texts; `via` rules;
  `silo` rules; the write-set analysis incl. helpers and `Task.all`.
- rt: a store per silo value; the open-time check's three outcomes
  (behind-migrate, behind-data-loss-maintenance, ahead-maintenance);
  `via` enforcement inside the transaction; cross-store reads.
- Browser tests in the example: two orgs, a non-member switch refused, a
  member sees only their org's projects, the admin picker; mutation
  gates (`scope dropped` per key, `membership dropped`).
- Deploy: the Turso target behind `-tags turso` with a real token
  (manual; documented) and a fake platform API for the default suite
  (schema database, children, the sample preflight, `status`'s table,
  point-in-time restore); the Ssh target with silos as files.
- Generators' goldens; `t11-workspace` in the drill test (commands only).

## Non-goals

- Cross-store transactions (the outbox comes with jobs); `Task.separately`;
  moving a row between partitions (delete + insert); per-request
  membership re-verification (the re-read idiom covers it); composite
  keys; silos keyed by something not in the session; Fly's full fix (stays
  experimental; Turso is the production target); a silo picker UI beyond
  the admin's and `choose`'s pages.
