# M14: Deploy — CI/CD in the language — Design

Date: 2026-10-16. Status: approved in conversation. Related: M10
(distribution: the static binary, `alder version`, the release workflow),
M5b (migrations and the schema snapshot), M13 (the build id in every page),
the monetization research of 2026-10-13 (a hosted target is the product),
M15 partitions and silos (Turso; follows this milestone), the user's
direction: "the CI/CD story is part of the language", "something super
easy and trivial", Unison as the inspiration.

## Decisions recorded (2026-10-16)

- **Targets are built into the compiler behind one interface, with a
  script escape hatch** (C): `Ssh` and `Fly` ship now; `Turso` with M15;
  a hosted Alder target later; `Script "./deploy.sh"` receives the
  finished artifact directory and does the rest.
- **State lives at the target; the unit is the build id** (A): each
  environment keeps on its box `builds/<id>/`, the database, backups, the
  environment file, a `current` pointer; no local or repository state; a
  lost box is a restore from the streamed backup.
- **A fixed gate plus declared checks** (C): every deploy runs check,
  build, tests and `--mutate` against the app's floor; the app may add
  commands that must exit zero; `--unsafe-skip-tests` exists and is named
  for what it is; a data-dropping migration needs `--allow-data-loss` on
  the command line, never in the file.

## The declaration

```alder
module App

deploy {
  production: Ssh "deploy@box.example.com" { domain: "blog.example.com", path: "/srv/blog" },
  staging:    Fly "ord" { app: "blog-staging" },
  preview:    Ssh "deploy@box.example.com" { domain: "*.preview.example.com", path: "/srv/blog-preview" },
  floor:      0.95,
  checks:     ["./scripts/lint-content"],
  requires:   ["SMTP_URL", "STRIPE_KEY"],
}
```

- `deploy` appears at most once, in `App`. Environments are named fields
  of target type; `floor` (Float, default: the score of the last
  successful deploy, 0 before any), `checks` (List Text of commands run
  from the app root), `requires` (List Text of environment-variable names
  the adapters need at the target; the checker ALSO derives the names the
  adapters' packages read via `os.Getenv` where it can — the build's
  adapter scan already lists them — and warns on a mismatch).
- Targets: `Ssh host { domain, path, user?, port? }` (a Linux box reachable
  by SSH with a public key; the compiler installs a systemd unit per
  environment and a Caddy reverse proxy with automatic TLS for `domain`
  if `proxy: Caddy` (default) — `proxy: None` leaves TLS to the operator),
  `Fly region { app }` (the Fly machines API; a volume for the database),
  `Script path` (see below), later `Turso` and `Alder`.
- Checked: an unknown target kind, a missing `domain`/`path`/`app`, an
  empty `requires` name, a `floor` outside 0..1, a duplicate environment
  → E0030 with a sentence; the file carries NO secret: a value that looks
  like one (a token, a URL with a password) is E0030 "a secret belongs in
  the environment, not in `App`: name it in `requires`".
- `deploy` changes nothing in the program: no runtime code, no migration,
  no page. It is data the `alder deploy` command reads.

## The command

`alder deploy <env> [--allow-data-loss] [--unsafe-skip-tests] [--dry-run] [--json]`

1. **Gate**: `alder check` (errors refuse; holes refuse), `alder build`,
   `alder test`, `alder test --mutate` ≥ `floor` (the score printed; a
   refusal names the survivors' count), then each of `checks` in order
   (exit non-zero refuses with the command's last lines). `--unsafe-skip-
   tests` skips tests, mutate and checks, and the deploy record says so.
2. **Artifact**: `build/<app>/deploy/<build-id>/` holding the binary, the
   migration plan against the target's live schema snapshot (fetched in
   step 3; `--dry-run` stops after printing it), the static directory,
   `alder-deploy.json` (build id, version, commit if any, the gate's
   results, the migration plan's summary, the `requires` list).
3. **Target**: connect; verify the `requires` names are set at the
   target (refuse naming the missing ones); fetch the live `schema.json`
   (none on first deploy); compute the plan; if it drops data and
   `--allow-data-loss` is absent, refuse with the DROPS DATA lines.
4. **Ship**: upload the artifact to `builds/<id>/`; back up the database
   file (`VACUUM INTO builds/<id>/backup.db` through the running binary's
   maintenance endpoint, or `sqlite3` at the target, before any
   migration); apply migrations with the schema check (the binary does
   this at start: run `builds/<id>/app --migrate-only` first so a
   refusal happens before the switch); start the new build on a free
   port; health-check `GET /` until the page's `alder-build` meta equals
   `<id>` (timeout → stop it, refuse, nothing switched); switch the
   proxy (Caddy's admin API / the systemd unit's port file); stop the old
   build after its in-flight requests (SIGTERM, 30 s); update `current`;
   prune builds beyond `keep` (default 5).
5. **Record**: `builds/<id>/deployed.json` at the target (when, by whom:
   the SSH user + the local `USER`, the gate results, the plan); `alder
   status <env>` prints the current build, the last five, the database
   size, the last backup time; `alder rollback <env> [<id>]` switches
   the pointer back (and refuses when the target schema is newer than the
   build's snapshot unless `--allow-schema-mismatch`); `alder logs <env>
   [-f]` tails the unit's journal.
- `Script path`: steps 1–2 run as above; the script receives the artifact
  directory and the environment name as arguments and the `requires`
  names as `ALDER_REQUIRES`; its exit code is the result; `status`,
  `rollback` and `logs` are "not available for a Script target".
- Streaming backup: the `Ssh` and `Fly` targets install the binary's
  built-in WAL shipper? — NO: the runtime gains `ALDER_BACKUP_URL` (an S3-
  compatible bucket) and a goroutine that ships WAL frames every N seconds
  (Litestream's design, a small subset: snapshot + WAL segments, restore
  with `alder restore <env> --from <url> --to <time>`); `deploy` sets the
  variable when the declaration has `backup: S3 "s3://bucket/blog"` (its
  credentials are `requires`). Without `backup`, `deploy` warns once per
  run that only pre-migration snapshots exist.
- Zero downtime on one box: SQLite is multi-process; the new build opens
  the file while the old one drains; a migration that rebuilds a table
  takes the write lock for its duration (documented; `--dry-run` prints
  the plan so an operator can choose a window).

## CI

- `alder ci init` writes `.github/workflows/alder.yml`: on `pull_request`:
  `alder check`, `alder test --mutate` (the floor), and if the declaration
  has `preview`, `alder deploy preview` with the PR's number in the host
  (`<pr>.preview.example.com`) and a comment with the URL; on push to
  `main`: `alder deploy production`. The workflow installs Alder from the
  release (the Homebrew tap or the binary) and needs the SSH key / Fly
  token / `requires` values as repository secrets, listed by name in the
  file's comments. The workflow is three jobs because the command is the
  pipeline; anyone can run the same commands from a laptop.
- `alder ci init --gitlab` and others later; the first is GitHub.

## Book

- A new chapter `book/16-deploy.md` "Deploying": the declaration, the
  command step by step with a transcript against a local `Ssh` target
  (the book test runs it against a container? — RULING: against a local
  sshd in a temp directory is too much for `go test ./book`; the chapter's
  transcripts are produced by a `Script` target plus a recorded `Ssh`
  session marked "recorded"), `status`/`rollback`/`logs`, backups and
  restore, `ci init`, the preview environment, the data-loss rule, what
  the gate refuses and why, the Fly target, the Script hatch, the
  SQLite-in-production facts (the table from the 2026-10-16 discussion:
  one writer, one box, the disk as the risk, rebuild locks), when to look
  at Turso (M15). Chapter 11 (the commands), 12 (why deploy is a
  declaration; why the gate is fixed; why state lives at the target),
  13 (the deploy pipeline's code; the health check by build id; the
  backup shipper), README (the one-command story), the reference (the
  declaration + `alder deploy`/`ci init` lines; cap raised to 47k).

## Testing

- Checker: corpus for E0030 cases; the `requires` derivation warning.
- `cmd/alder`: the gate's refusals (floor, checks, holes); the artifact's
  contents; `--dry-run` output; the `Script` target end to end (a script
  that records its arguments); the `Ssh` target against a LOCAL sshd in
  a container when Docker is present (`-tags deploy`; CI runs it) else
  against `localhost` with the current user's key if `ALDER_DEPLOY_LOCAL=1`;
  health check, switch, rollback, prune, the data-loss refusal, the
  schema-newer-than-build refusal; the Fly target behind `-tags fly` with
  a real token (manual; documented).
- `rt`: the backup shipper's snapshot + WAL segments round trip to a local
  directory "bucket"; restore to a point in time.
- `alder ci init`'s file is golden-tested and validated with `actionlint`
  if present.

## Non-goals

- Kubernetes, containers as the unit (the binary is the unit; a Dockerfile
  exists from M10 for those who want it); multi-box production (M15/Turso);
  secrets management beyond "named and present"; blue/green across hosts;
  the hosted Alder target (its own milestone, after users exist); GitLab/
  other CI templates (later); database migrations with custom SQL steps
  beyond what `alder migrate` already supports.
