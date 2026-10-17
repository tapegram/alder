# M16: The Render target — a stateless app on Render, its data on Turso — Design

Date: 2026-10-17. Status: approved in conversation. Related: M14 (deploy:
the `Target` interface, the gate, the commands, `alder ci init`), M15 (the
Turso database: migrations over HTTP, the sample preflight, PITR restore,
`_alder_lock`), the user's roadmap direction after M15: "a first-class
Render deploy target (Ssh doesn't fit a PaaS), designed Turso-first:
stateless app, zero-downtime, multi-instance", and in this conversation:
"I don't want downtime" — which removed the planned disk mode.

## Decisions recorded (2026-10-17)

- **Turso-only.** A Render environment keeps nothing between deploys; its
  data is a Turso database (M15's `database:`). A persistent disk is a
  non-goal: Render stops a disk-backed instance before the new one starts,
  so no shape of it is zero-downtime.
- **Image per build, pushed in pure Go.** The artifact becomes a one-layer
  OCI image on `scratch`, built and pushed by `alder deploy` with the
  registry HTTP API — no Docker, no shell in the image. Render deploys it
  by exact tag (`imageUrl`).
- **Render does the roll.** Render starts the new instances beside the old,
  health-checks `/_alder/health`, switches, and SIGTERMs the old. The
  engine's `Switch` and `Stop` are no-ops; its `Health` checks every
  instance serves the build.
- **State lives in the database.** Builds, records, the current pointer and
  the lock are rows of the shared Turso database — the one durable thing
  in a stateless service.
- **Migrations run first, from the deploying machine** (M15's path). The old
  build serves on the new schema until the roll completes; the gate's
  DROPS DATA rule is the guard, and the book says so.
- **Preview is one declared environment** (a Render service PRs overwrite),
  as on Ssh minus the per-PR host. Per-PR services are a non-goal with a
  sketch.
- **GHCR is the default registry**, one token for the push and for Render's
  registry credential; any registry can be declared.
- `alder new` defaulting to Render + Turso waits for the real-account round.

## The declaration

```alder
module App

deploy {
  production: Render "oregon" {
    service:   "blog",                          -- the Render service's name
    database:  Turso "my-org" { app: "blog" },  -- required: the app's data
    instances: 2,                               -- optional, default 1
    plan:      "starter",                       -- optional, Render's default
  },
  preview: Render "oregon" {
    service:  "blog-preview",
    database: Turso "my-org" { app: "blog-preview" },
  },
  image:    "ghcr.io/tavish/blog",              -- optional; default from the git remote
  requires: ["SMTP_URL"],
}
```

- `Render region { service, database, instances?, plan? }`. `region` is one
  of Render's: `oregon`, `ohio`, `virginia`, `frankfurt`, `singapore`.
  `service` is Render's name for the service; the engine finds it by name
  in the API key's workspace and creates it on the first deploy.
- `database:` is M15's field, unchanged: `Turso "org" { group?, app,
  sample? }`. `ir.Target.Database` already exists; its comment stops saying
  "an Ssh environment's". Two environments that share an `app` share data;
  the example gives `preview` its own.
- `instances` is Render's `numInstances` (≥ 1). Every Render environment is
  multi-instance-capable because its store is remote; this is where the
  roadmap's "refuse several instances on one local SQLite file" lands: the
  only target with an `instances` field is the one whose store is not a
  file.
- `plan` is passed to Render as given (`starter`, `standard`, `pro`, …, or
  a compute spec such as `1c-2g`); unset leaves Render's default.
- `image` is a deploy-level field: one registry path for all Render
  environments, tagged with the build id. Default: `ghcr.io/<owner>/<repo>`
  from the `origin` remote when it is a GitHub remote; otherwise E0030
  asks for it.
- `backup: S3` does not apply to a Render environment (Turso's point-in-time
  recovery is the backup). A declaration with `backup` and only Render
  environments is E0030; with mixed targets, the dry-run says which
  environments it applies to (M15's rule).

### What the checker refuses (E0030)

- No `database`: "a Render service keeps nothing between deploys: give it
  `database: Turso "org" { app }`".
- An unknown region (the five are named); a missing or empty `service`;
  `instances` below 1; `plan` outside `[a-z0-9_-]+`.
- `image` carrying a credential (`user:token@`) or a tag: "the registry
  path, without a tag: the build id is the tag"; outside the registry
  charset (`[a-z0-9._/-]`, a host, 2–3 path parts); missing with no GitHub
  `origin`: "name the registry path: `image: "ghcr.io/<owner>/<app>"`".
- `backup` with only Render environments: "Turso's point-in-time recovery
  is a Render environment's backup: `backup` applies to Ssh and Fly".
- `--host` at the command for a Render environment: refused ("the address
  is the service's: https://<service>.onrender.com").
- The secret rule (M14) holds: tokens never appear in `App`.

### The environment the command reads

On the deploying machine (a laptop or the CI runner), never in `App`:

- `RENDER_API_KEY` — the Render REST API.
- `ALDER_REGISTRY_TOKEN`, `ALDER_REGISTRY_USER` (default: the first path
  part of `image`) — the push, and Render's registry credential. Unset
  token: the image is pushed anonymously (fails for a private registry) and
  no credential is made — a public image.
- `TURSO_API_TOKEN` — as M15 (the platform API, the migrations).
- The `requires` names: their values are read here and set as the service's
  environment variables, as Fly does. `ALDER_SECRET` is generated once and
  kept on the service.

## The image and the registry

- **Content.** One layer: `/app/app` (the static, CGO-off `linux/amd64`
  binary — Render runs only `linux/amd64`; `Connect` reports that platform
  and the artifact is cross-built as for Fly), `/app/static/`,
  `/app/migrations/`, `/app/alder-deploy.json`. Config: `Entrypoint
  ["/app/app"]`, `Env ALDER_BUILD=<id>`, `ALDER_STATIC=/app/static`,
  `ExposedPorts 10000/tcp`. Base `scratch`: no shell, no packages.
- **TLS roots and time zones** come from the binary: the generated `main`
  imports `golang.org/x/crypto/x509roots/fallback` and `time/tzdata` when
  the build is for a deploy target (the deploy's build, not `alder run`'s;
  x/crypto is already a dependency through PBKDF2). Fly's image benefits
  too; its Alpine base stays as it is.
- **Build** (`internal/deploy/oci.go`): the artifact is tarred
  deterministically (sorted entries, fixed mtimes, uid/gid 0, modes 0755
  for `app`, 0644 else), gzipped with a fixed header; the layer's digest,
  the config JSON (OCI image config: architecture, os, rootfs diff id,
  config), the OCI manifest. The image digest is a function of the
  artifact: the same build id pushed twice yields the same digest (a test
  pins it).
- **Push**: registry API v2 by hand (`net/http`): `GET /v2/` → the
  `WWW-Authenticate` challenge → the token exchange with
  `ALDER_REGISTRY_USER/TOKEN` (`scope=repository:<name>:pull,push`); `HEAD`
  each blob and upload the missing ones (`POST` the upload, `PUT` the
  monolithic body — the layer is a few MB); `PUT
  /v2/<name>/manifests/<id>`. Retries on 5xx and 429 with backoff; errors
  name the status and the registry's body, never the token. The tag is the
  build id; there is no moving tag — Render deploys the exact tag and a
  rollback names an older one.
- **Render's side**: the first deploy of an `image` with a token makes a
  registry credential (`POST /registrycredentials {name: "alder-<image
  without host>", registry, username, authToken}`), found by name on later
  runs; the service is created `image: {imagePath: "<image>:<id>",
  registryCredentialId}`. `registry` is derived from the host (`ghcr.io` →
  `GITHUB`, `docker.io` → `DOCKER`, `registry.gitlab.com` → `GITLAB`,
  `*.pkg.dev` → `GOOGLE_ARTIFACT`, `*.amazonaws.com` → `AWS_ECR`; another
  host is E0030 naming these).
- **Prune**: images stay in the registry (Fly's rule); `keep` governs the
  records. Deleting GHCR package versions is a later nicety.

## The deploy, step by step

`Render` implements M14's `Target` plus `Locker`, `Preflighter`,
`DataLossAllower` and `BackupRestorer`, composing the existing `*Turso` for
everything database-side (as `Ssh` does with `database:`). The engine's order
is unchanged; each step on Render:

| Step | Render + Turso |
|---|---|
| Connect | `GET /owners` (the key's workspace), `GET /services?name=<service>&type=web_service`. Platform `linux/amd64`. Remembers whether the service exists. Changes nothing. |
| Preflight | Turso's (token, org, the shared database reachable, the sample). Render: the key answers; `instances` is accepted for `plan`. |
| Requires | `GET /services/{id}/env-vars` → names only, never values read back for display. The missing names are the refusal. Service not yet created: "missing" = the `requires` names unset on this machine. |
| Lock | M15's `_alder_lock` row in the shared database (holder `user@host`, stale takeover reported). |
| Schema / Current / Builds | Rows of the shared database: `_alder_deploy_builds` (id, record JSON, schema hash, created, migrated-at, `backup.pitr`), `_alder_deploy_current` (one row). Created by `--migrate-only` as `_alder_lock` is. The app schema snapshot and applied migrations are read from the database as M15 does. No tables → `Have false`, current `""`. |
| Upload | Build + push the image. Create the service if absent: `POST /services {type: web_service, name, ownerId, image, serviceDetails: {plan, region, numInstances, healthCheckPath: "/_alder/health", maxShutdownDelaySeconds: 30, envSpecificDetails: {}}, envVars}`. Then one `PUT /services/{id}/env-vars` with `ALDER_DB=libsql://<host>`, `ALDER_TURSO_ORG/GROUP/APP`, `TURSO_API_TOKEN` (the app mints silo tokens), `ALDER_ADDR=:10000`, `ALDER_SECRET`, the `requires` values. Env changes make Render redeploy; the PUT happens before the image deploy so the roll we trigger next is the one that counts (the engine cancels a deploy Render started for the env change, `POST /deploys/{d}/cancel`, when it is not ours). `instances` changed: `POST /services/{id}/scale {numInstances}`. |
| Backup | PITR timestamp → the build's record (M15). |
| Migrate | M15's: `--verify-silos <sample>`, then `--migrate-only` with the build's local executable over HTTP; the DROPS DATA rule at the apply point (`AllowDataLoss` → `ALDER_ALLOW_DATA_LOSS=1` for that run only). The old build keeps serving on the new schema until the roll completes. |
| Start | `POST /services/{id}/deploys {imageUrl: "<image>:<id>"}` → poll `GET /services/{id}/deploys/{d}` (every 3 s, `Retry-After` honoured) until `live`; `update_failed`, `canceled` → an error carrying the deploy's log lines (`GET /logs`). Timeout: `ALDER_DEPLOY_HEALTH_TIMEOUT` as for the other targets (default 10 min). Returns port 443. |
| Health | `GET https://<service>.onrender.com/_alder/health` until the body's build is `<id>` — `instances` consecutive answers, so every instance has rolled; the error carries the last body. |
| Switch / Stop | No-ops; Render did both. `Switch` returns 443. |
| Record / Prune | The record row; rows beyond `keep` removed (newest kept, the current protected). |
| Info | Turso storage bytes (M15), PITR as "backup: Turso point-in-time, <retention>", the service's URL, instance count, live deploy id and age. |
| Logs | `GET /logs?ownerId&resource=<service>&limit` for the last lines; `-f` subscribes (below). Lines carry Render's instance id. |
| Restore | M15's Turso path: the copy at `T` becomes the app's database → `PUT env-vars` with the new `ALDER_DB` → Render rolls the current image; the replaced database is kept and named. `--silo` as today. |

- **`/_alder/health`** (`rt/web`, always on): `200 {"build":"<id>","db":"ok"}`
  after `SELECT 1` on the shared store; `503 {"build":"<id>","db":"<error>"}`
  while the store is unreachable, so Render never switches to a build that
  cannot reach its database. Not cached, not logged by the dev panel.
- **The binary does not migrate on start in Turso mode** (several instances
  would race). On start it compares the shared schema with its
  `migrations/schema.json`: behind → exit non-zero with "the database is
  behind this build: run `alder deploy`" (Render fails the deploy; the old
  build keeps serving); ahead → serve the maintenance page (M15's
  behaviour; M14's rollback rule names `--restore-backup`); equal → serve.
- **Failure handling**: a refusal before `Start` changes nothing at Render
  (the image is in the registry; harmless). `Start` failing leaves Render on
  the previous build by Render's own revert; the engine records nothing as
  current and, for a deploy that failed after migrating, offers M14's
  `restoreAfterDrop` through `BackupRestorer` (the PITR copy) exactly as the
  Ssh+Turso environment does today.

## Commands

- `alder deploy <env> [--allow-data-loss --unsafe-skip-tests --dry-run
  --json]`: as M14. Transcript lines: "image pushed ghcr.io/…:<id> (3.1
  MB)", "Render deploy dep-… live (2 instances)", "health: 2/2 instances
  serve <id>". `--dry-run` connects, lists env-var names, fetches the
  schema, prints the plan and the sample; no POST/PUT, no push. `--json`
  gains `address` for every target (Ssh `https://<domain>`, Fly
  `https://<app>.fly.dev`, Render `https://<service>.onrender.com`).
- `alder status <env>`: the current build, the last `keep` records, Render's
  instance count, URL, live deploy id and age, Turso's storage bytes and
  PITR window, the silo table.
- `alder rollback <env> [id] [--restore-backup --yes]`: `POST deploys
  {imageUrl: <image>:<id>}` and poll; M14's schema rule; `--restore-backup`
  = the PITR copy at that build's `backup.pitr` → env-var PUT. Both are
  rolls, no downtime.
- `alder logs <env> [-f]`: `GET /logs`; `-f` opens
  `wss://api.render.com/v1/logs/subscribe` with the same filters and prints
  frames as they come. The websocket client is minimal RFC 6455 (handshake,
  masked text frames, ping/pong, close) in `render_ws.go`, no dependency.
  The plan's first task spikes it against the fake; if it is not done in a
  task, `-f` polls `GET /logs` with a cursor every 2 s instead and the book
  says so.
- `alder restore <env> --to T --yes [--silo k:v]`: M15's, with the env-var
  PUT in place of `turso.env`.

## CI

`alder ci init` detects Render environments and writes the same three jobs
(PR: check, mutate, preview deploy + comment; main: production), with:

- no SSH step; repository secrets listed by name in the header comment:
  `RENDER_API_KEY`, `TURSO_API_TOKEN`, `ALDER_REGISTRY_TOKEN`, plus the
  `requires` names.
- the job's `permissions: packages: write` and `ALDER_REGISTRY_TOKEN: ${{
  secrets.ALDER_REGISTRY_TOKEN || github.token }}`: the per-run token can
  push a GHCR image under the repository's owner, but Render's registry
  credential needs a long-lived PAT with `read:packages` — the comment says
  a PAT set once as `ALDER_REGISTRY_TOKEN` is simplest; the run token alone
  works only for a public image.
- the preview comment posts the `address` from `alder deploy preview
  --json`; no `--host`.
- a note that the first deploy creates the Render service, so the dashboard
  is for looking.

## Code

- `internal/deploy/render.go`: the `Render` target and the step mapping;
  composes `*Turso`. `render_api.go`: the client (owners, services by name,
  create, env-vars PUT, scale, registry credentials, deploys create/get/
  list/cancel, logs), bearer auth, `Retry-After`, errors with status and
  Render's message, never the key. `render_ws.go`: the websocket client.
  `oci.go`: the image builder and the registry push.
- `rt/web`: `/_alder/health`. The generated main imports
  `x509roots/fallback` and `time/tzdata` for deploy builds.
- `internal/ir`: `Target` gains `Service, Plan string; Instances int`;
  `Deploy` gains `Image string`. The checker: the E0030 sentences above.
- `cmd/alder/deploy.go`: the `render` kind, `address` in `--json`, `--host`
  refused; `ci.go`: the Render branch.

## Testing

- Default suite: a fake Render API (`httptest`: owners; services by name;
  create; env-vars GET/PUT; scale; registry credentials; deploys with a
  scripted state machine `created → update_in_progress → live |
  update_failed | canceled`; cancel; logs; one 429 with `Retry-After`), a
  fake registry (the v2 handshake, token exchange, blob HEAD/POST/PUT,
  manifest PUT, one 503), the fake Turso platform API (exists). Scenarios:
  first deploy creates credential, service, env, pushes, rolls; second
  deploy pushes and rolls only; `instances: 2` health waits for both;
  a migration refusal before any Render POST/PUT; `update_failed` → error
  with log lines, nothing recorded, the previous build current; rollback;
  rollback `--restore-backup` = exactly one env-var PUT with the new host;
  restore; the lock held → refusal naming the holder; `--dry-run` makes no
  POST/PUT (asserted on the fakes); the manifest golden and digest
  determinism; the websocket client against a fake server (frames, ping,
  close); `ci init` goldens for a Render-only and a mixed declaration.
- Checker corpus: every E0030 sentence above.
- `-tags render`, manual and documented: a real workspace, a real Turso
  organization, GHCR — the real-account round; the transcript is recorded
  into the chapter as the Ssh one is.

## Book

Chapter 16 gains "The Render target" after Fly: the declaration, a recorded
transcript, zero downtime and what it asks of migrations (the old build
serves on the new schema; the gate's DROPS DATA rule is the guard),
instances, rollback and restore, the registry and the one token, CI, and
"why no disk mode" in two sentences. Chapter 12 gets the rationale line (a
PaaS deploys an image it is handed; a file database and a stateless service
do not mix). The reference stays ≤ 48k.

## Non-goals

- A Render persistent disk (SQLite on a file): Render stops the instance
  before the new one starts; there is no zero-downtime shape. A file on a
  PaaS has the Ssh target or the Script hatch.
- Per-PR preview services: later `preview: Render … { perPr: True }`
  creating `<service>-<pr>` services keyed by the PR number, torn down on
  close; nothing in v1 blocks it.
- Autoscaling (`instances` is fixed; Render's autoscaler is one field
  later); custom domains on Render (the API has them; the dashboard for
  now); registry tag pruning; `alder new` defaulting to Render + Turso
  (after the real-account round); Render's Git-backed builds; service types
  other than `web_service`; regions beyond Render's five.
