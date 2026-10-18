# M16: The Render target — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `Render region { service, database: Turso …, instances?, plan? }` deploys a stateless app as a one-layer OCI image pushed in pure Go, rolled by Render with zero downtime against `/_alder/health`, its builds, records and lock kept in the shared Turso database; status, rollback, logs, restore and `ci init` work for it; chapter 16 documents it.

**Architecture:** (1) A websocket spike (`render_ws.go`) decides `logs -f` early. (2) Language: IR fields and the E0030 sentences. (3) Runtime: `/_alder/health`, no start-time migration on a `libsql://` store (behind → exit, ahead → maintenance + 503 health), deploy tables, deploy builds tagged `alder_deploy` (tzdata). (4) `oci.go`: the deterministic image and the registry v2 push, against a fake registry. (5) `render_api.go`: the REST client, against a fake Render API. (6) `render.go`: the `Target` composing `*Turso`, state in the database, the scenarios. (7) `cmd/alder`: the `render` kind, `address`, `--host` refused, the image path, `ci init`. (8) Book, reference, the manual `-tags render` test.

**Tech Stack:** Go 1.27 stdlib only (`net/http`, `archive/tar`, `compress/gzip`, `crypto/sha256`, `crypto/tls`, `encoding/json`); modernc SQLite and the existing Turso client (`rt/db.Turso`); `httptest` fakes.

**Spec:** `docs/superpowers/specs/2026-10-17-m16-render-target-design.md` (binding: the declaration, every E0030 sentence, the step table, the image content, the commands, CI, the testing list, the non-goals). Where a ruling below differs from the spec, the ruling wins and the book follows the ruling.

## Rulings (spec corrections found while planning)

- **R1 TLS roots.** `golang.org/x/crypto` is not a dependency of this repo (PBKDF2 is the stdlib's `crypto/pbkdf2`), and `x509roots/fallback` is a module of its own. The import rule stands. The image instead carries a CA bundle at `/etc/ssl/certs/ca-certificates.crt`, where Go's `crypto/x509` looks on Linux. The bundle is vendored as `internal/deploy/cacerts/cacert.pem` (Mozilla's, from `https://curl.se/ca/cacert.pem`) with its source URL, date and SHA-256 in `internal/deploy/cacerts/README.md`, embedded with `//go:embed`. Time zones: a generated `cmd/tzdata_deploy.go` under `//go:build alder_deploy` imports `time/tzdata`; `buildFor` passes `-tags alder_deploy`. The spec's sentence about x/crypto is corrected in the book.
- **R2 Service creation happens in `Start`, not `Upload`.** A service created during `Upload` would start the new image before the migrations ran. `Upload` pushes the image and, for an existing service, sets env vars and scale. When the service is absent, `Start` creates it with `imagePath: <image>:<id>`, the env vars and `numInstances`; the creation's own deploy is the one polled.
- **R3 The env-var PUT merges.** Render's `PUT /services/{id}/env-vars` replaces the whole set. The target GETs the current set (values held in memory, never printed or logged), overlays alder's names and the `requires` values set on this machine, and PUTs the union. A `requires` name unset here but present at the service is kept. `Requires` reports missing = unset both here and at the service.
- **R4 Deploy tables are made by the target.** `rt/db.EnsureDeployTables` (beside `createLockTable`) creates `_alder_deploy_builds` and `_alder_deploy_current`; the Render target calls it under the lock. Both names join `_alder_lock` in the two "not an app table" lists (`rt/db/sqlite.go:767`, `rt/db/migrate.go:744`). A builds row stores the build's `schema.json` and migration names, so `BuildSchema` and rollback work from a CI runner that never had the old artifact.
- **R5 The default `image` is resolved by the command, not the checker.** The checker stays free of git. `alder deploy`, `status`, `rollback` and `ci init` read `origin` from `<app>/.git/config` (a file read, no git binary). With no `image` and no GitHub origin, they refuse with the spec's E0030 sentence, prefixed `E0030`.
- **R6 The address is the service's URL.** Render suffixes `onrender.com` names that are taken, so the address is `serviceDetails.url` from the API. Before the service exists (dry run), it is `https://<service>.onrender.com`, marked "(expected)".
- **R7 The first deploy is locked too.** `Render.Lock` runs `Turso.Preflight` first (it creates the shared database when absent), then `Turso.Lock`, then `EnsureDeployTables`. Two first deploys from two CI runs cannot both proceed.
- **R8 The start-time check applies to every `libsql://` shared store**, the Ssh+Turso environment included: its deploys already migrate from the deploying machine before `Start`.

## Global Constraints

- Imports stay stdlib + `alder/*` + modernc. No new module in `go.mod`. The websocket, OCI and registry code is written by hand.
- No secret in `App.alder` (M14's rule), in the image, in a builds row, in a transcript, in `--json`, or in an error. `RENDER_API_KEY`, `ALDER_REGISTRY_TOKEN`, `TURSO_API_TOKEN`, `ALDER_SECRET` and `requires` values are tested absent from every output of every scenario.
- `--dry-run` makes no POST, PUT, PATCH or DELETE at the fake Render API, no blob or manifest upload at the fake registry, and no write at the fake Turso (asserted on the fakes' request logs).
- The image digest is a function of the artifact: the same artifact built twice gives the same manifest digest.
- Nothing is recorded as current until Render reports `live` and the health check sees the build on `instances` consecutive answers.
- The real Render API, a real registry and a real Turso organization are never contacted by the default suite or CI. `-tags render` is manual only.
- Base URLs come from `ALDER_RENDER_API` (default `https://api.render.com/v1`) and the `image` host; tests set them to the fakes.
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...` and `go vet -tags 'deploy render browser' ./...`; `go test ./... -count=1 -timeout 30m`; `go test -tags browser ./...`; all three examples check/build/test + `--mutate` with their scores unchanged; `alder new` out of tree; experiments unittest (reference ≤ 48k); `go test ./book`.
- Branch `m16-render`. Commit messages end with the session trailers. Stage by explicit path.

## Review Focus

1. **A service whose env vars were set in Render's dashboard** (e.g. `SMTP_URL` set there, not on the CI runner): a deploy keeps it, `Requires` does not report it missing, and no value appears in the transcript or `--json`. Task 6 `TestRender_EnvMergeKeepsDashboardVars`.
2. **The deploy tables seen as app tables**: after a Render deploy, `alder migrate` drift, `HasAppTables`, the silo `appTables` and `--migrate-only` on the next build must ignore `_alder_deploy_builds` and `_alder_deploy_current`. Task 3 `TestDeployTables_NotAppTables`.
3. **A deploy that fails at Render after migrating** (`update_failed`): the previous build still serves (Render's revert), nothing is recorded current, the error carries Render's log lines, and with a data-dropping migration the engine's `restoreAfterDrop` makes exactly one PITR copy and one env-var PUT. Task 6 `TestRender_UpdateFailedAfterDropRestores`.
4. **`alder rollback` from a fresh CI checkout** with no `build/<app>/deploy/<old id>/`: the schema rule reads the old build's snapshot from its builds row; the rollback POSTs `imageUrl: <image>:<old id>` and polls. Task 6 `TestRender_RollbackWithoutLocalArtifact`.
5. **A binary started on a database migrated by a newer build** (rollback with `--allow-schema-mismatch`, or a stray manual deploy): every route answers the maintenance page, `/_alder/health` answers 503 with `"db":"ahead: …"`, so Render keeps the current build. On a database that is behind, the binary exits 1 with the spec's sentence. Task 3 `TestStartCheck_Ahead` and `TestStartCheck_Behind`.

---

### Task 1: Websocket client spike (`render_ws.go`)

The spec makes this the first task: `logs -f` uses it if it lands, else polls.

**Files:**
- Create: `internal/deploy/render_ws.go`
- Test: `internal/deploy/render_ws_test.go`

**Interfaces:**
- Produces: `func DialWS(ctx context.Context, url string, header http.Header) (*WSConn, error)`; `func (c *WSConn) Read() (text string, err error)` returns the next text message (continuation frames joined, pings answered with pongs, `io.EOF` on a close frame after echoing it); `func (c *WSConn) Close() error` sends close 1000 and closes. `url` is `ws://` or `wss://` (TLS via `crypto/tls`, ServerName from the host).

- [ ] **Step 1: Write the failing tests.** A test server built on `httptest.NewServer` hijacks the connection and speaks raw frames (written in the test file by hand, so the client is not tested against itself):
  - `TestWS_Handshake`: the request carries `Upgrade: websocket`, `Connection: Upgrade`, `Sec-WebSocket-Version: 13`, a 16-byte base64 key, the caller's `Authorization` header; the client checks `Sec-WebSocket-Accept` = base64(SHA-1(key + `258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)) and refuses a wrong one.
  - `TestWS_TextFrames`: the server sends a 5-byte text frame, a 300-byte frame (16-bit length), a 70 000-byte frame (64-bit length) and a message split over a text frame plus a continuation frame; `Read` returns four strings equal to what was sent.
  - `TestWS_PingPong`: the server sends ping `"x"`; it then reads a masked pong with payload `"x"`, then sends a text frame that `Read` returns.
  - `TestWS_Close`: the server sends close 1000; `Read` returns `io.EOF` and the server reads a masked close frame back.
  - `TestWS_ClientFramesMasked`: frames the client writes have the mask bit set and a non-zero mask key.
- [ ] **Step 2: Run them to see them fail.** `go test ./internal/deploy -run TestWS -count=1` → FAIL (undefined: DialWS).
- [ ] **Step 3: Implement.** Handshake over `net.Dialer` / `tls.Dialer`, `bufio.Reader` for the response; a frame reader (FIN, opcode, mask, 7/16/64-bit length; refuse a masked server frame and a payload over 16 MiB); a frame writer that masks with `crypto/rand`. `ctx` cancellation closes the connection.
- [ ] **Step 4: Run them to see them pass.** Same command → PASS. Also `go vet ./internal/deploy`.
- [ ] **Step 5: Commit.** `git add internal/deploy/render_ws.go internal/deploy/render_ws_test.go && git commit -m "deploy: a minimal RFC 6455 client for Render's log stream"`.
- [ ] **If the spike does not converge in this task:** delete both files, commit nothing, and record in the task report that Task 6's `Logs(follow)` polls `GET /logs` every 2 s with a cursor; Task 8's book says so.

### Task 2: Language — the `Render` target and `image`

**Files:**
- Modify: `internal/ir/ir.go:91-121` (`Deploy`, `Target`, `Database`'s comment)
- Modify: `internal/check/deploy.go` (`deployTarget`, a new `renderTarget`, the `backup` rule, the `image` field in `deployField`)
- Modify: `internal/syntax` only if `instances: 2` (an integer field value inside a target record) does not parse today; check with the corpus first.
- Modify: `internal/diag` E0030's explain text (adds Render)
- Test: corpus cases under the existing deploy corpus directory (`deploy-ok` gains a Render environment; one case per sentence below), `alder dump` golden for the IR.

**Interfaces:**
- Produces: `ir.Target{Kind: "render"; Region, Service, Plan string; Instances int (0 when unset: means 1); Database *Database}` (`Region` reused); `ir.Deploy.Image string` ("" when unset).
- `ir.Target`'s comment: Kind is "ssh", "fly", "render" or "script"; `Database` is an Ssh or Render environment's.

- [ ] **Step 1: Write the failing corpus cases.** One file each, with the expected diagnostic text copied from the spec:
  - `render-no-database`: "a Render service keeps nothing between deploys: give it `database: Turso "org" { app }`".
  - `render-region`: `Render "mars"` → names the five: `oregon`, `ohio`, `virginia`, `frankfurt`, `singapore`.
  - `render-service-empty`, `render-service-missing`, `render-instances-zero`, `render-plan-charset` (`plan: "Big Plan"`, outside `[a-z0-9_-]+`).
  - `render-image-credential` (`image: "ghcr.io/u:tok@x/y"`) and `render-image-tag` (`image: "ghcr.io/u/app:v1"`): "the registry path, without a tag: the build id is the tag".
  - `render-image-charset`: uppercase, one path part (`ghcr.io`), four path parts.
  - `render-image-registry`: `image: "quay.io/u/app"` → names the five supported hosts (`ghcr.io`, `docker.io`, `registry.gitlab.com`, `*.pkg.dev`, `*.amazonaws.com`).
  - `render-backup-only`: `backup: S3 "s3://b/p"` with only Render environments → "Turso's point-in-time recovery is a Render environment's backup: `backup` applies to Ssh and Fly".
  - `render-secret-literal`: a token-looking literal in `service` → M14's secret sentence (no echo).
  - `deploy-ok` gains the spec's example declaration verbatim; `render-mixed-backup-ok`: Ssh + Render with `backup` is accepted.
- [ ] **Step 2: Run them to see them fail.** `go test ./internal/check -run Corpus -count=1` → the new cases FAIL ("unknown deploy target `Render`").
- [ ] **Step 3: Implement.** `deployTarget` gains `case "Render"` → `renderTarget`; the unknown-target sentence lists `Ssh`, `Fly`, `Render` or `Script`. `deployDatabase` is reused unchanged. The registry-host table (`RegistryKind(host) (string, bool)`: `GITHUB`, `DOCKER`, `GITLAB`, `GOOGLE_ARTIFACT`, `AWS_ECR`) lives in `internal/ir` so the checker and `internal/deploy` share it.
- [ ] **Step 4: Run to pass.** `go test ./internal/check ./internal/ir ./cmd/alder -run 'Corpus|Dump' -count=1` → PASS. Regenerate the dump golden with the suite's update flag only for the deploy-ok case and read the diff.
- [ ] **Step 5: Commit.** `feat(check): the Render deploy target and image (E0030)`.

### Task 3: Runtime — health, the start-time check, deploy tables, the `alder_deploy` build

**Files:**
- Modify: `rt/web/router.go` (the `/_alder/health` route under `alderPrefix`; a `SharedMaintenance error` option that makes every route answer the maintenance page)
- Modify: `internal/gen/caps.go:419-470` (`Serve`: for a `libsql://` path, check instead of migrate)
- Modify: `rt/db/migrate.go` (a read-only `CheckShared`), `rt/db/stores_turso.go` (`EnsureDeployTables`, row helpers), `rt/db/sqlite.go:767`, `rt/db/migrate.go:744`
- Modify: `cmd/alder/build.go:177` (write `cmd/tzdata_deploy.go`), `cmd/alder/deploy.go:411-427` (`buildFor` passes `-tags alder_deploy`)
- Test: `rt/web/health_test.go`, `rt/db/deploytables_test.go`, `rt/db/startcheck_test.go`, `cmd/alder/build_test.go`

**Interfaces:**
- Produces: `GET /_alder/health` → `200 {"build":"<ALDER_BUILD>","db":"ok"}` after `SELECT 1`; `503 {"build":…,"db":"<error>"}` when the query fails or `SharedMaintenance` is set (`"ahead: <detail>"`). `Cache-Control: no-store`; not logged; not in the dev panel; not behind the session or CSRF middleware.
- Produces: `type SharedState string` (`SharedOK`, `SharedBehind`, `SharedAhead`, `SharedFresh`); `func CheckShared(ctx context.Context, s *SQLite, tables []TableDesc, ms []Migration) (SharedState, string, error)` — reads `_alder_migrations`/`_alder_schema` like `inspect` does for silos, writes nothing.
- Produces: `func EnsureDeployTables(ctx context.Context, st *SQLite) error`; `type DeployRow struct{ ID, Record, Schema, SchemaHash, Migrations, Pitr string; Created time.Time }` (Migrations as a JSON list); `func PutDeployRow(ctx, st, DeployRow) error` (upsert); `func DeployRows(ctx, st) ([]DeployRow, error)` (newest first); `func DeleteDeployRows(ctx, st, ids []string) error`; `func SetCurrent(ctx, st, id string) error`; `func CurrentBuild(ctx, st) (string, error)` ("" when absent). Schema: `_alder_deploy_builds("id" TEXT PRIMARY KEY, "record" TEXT NOT NULL DEFAULT '', "schema" TEXT NOT NULL DEFAULT '', "schema_hash" TEXT NOT NULL DEFAULT '', "migrations" TEXT NOT NULL DEFAULT '[]', "pitr" TEXT NOT NULL DEFAULT '', "created" TEXT NOT NULL)`, `_alder_deploy_current("key" TEXT PRIMARY KEY CHECK ("key" = 'current'), "id" TEXT NOT NULL)`.

- [ ] **Step 1: Failing tests.**
  - `TestHealth_OK` / `TestHealth_DBDown` (a store whose connection is closed → 503 with the error) / `TestHealth_Ahead` (`SharedMaintenance` set → 503 `"db":"ahead: …"`, and `GET /` answers the maintenance page with 503) / `TestHealth_NoSession` (no `Set-Cookie`, `Cache-Control: no-store`).
  - `TestStartCheck_Behind`: a generated test app (the e2e fixture used by M15's silo tests) served with `ALDER_DB=libsql://<host>` against the existing `internal/tursofake` server (`ALDER_TURSO_API` pointed at it, as M15's runtime tests do); one migration pending → `Serve` returns an error whose text is "the database is behind this build: run `alder deploy`", and nothing was migrated.
  - `TestStartCheck_Ahead`: the store has a migration the build lacks → `Serve` succeeds and every route answers 503 maintenance.
  - `TestStartCheck_FileStillMigrates`: a file path still migrates on start (M14 unchanged).
  - `TestDeployTables_RoundTrip`: ensure twice (idempotent), put two rows, `DeployRows` newest first, `SetCurrent`, delete one.
  - `TestDeployTables_NotAppTables` (Review Focus 2): after `EnsureDeployTables` on a migrated store, `HasAppTables` on an otherwise empty store is false, drift reports nothing, and `MigrateLog` with no pending migrations is a no-op.
  - `TestBuild_TzdataDeployFile`: `alder build` writes `cmd/tzdata_deploy.go` whose first line is `//go:build alder_deploy` and which imports `time/tzdata`.
- [ ] **Step 2: Run to fail.** `go test ./rt/web ./rt/db ./cmd/alder -run 'Health|StartCheck|DeployTables|TzdataDeploy' -count=1`.
- [ ] **Step 3: Implement.** In `Serve`, when `path` starts with `libsql://`: `CheckShared`; `SharedBehind` → return the error above; `SharedAhead` → set `w.SharedMaintenance`; `SharedFresh` (no app tables) → also behind (`--migrate-only` makes them); else serve. Dev mode keeps migrating. Health uses `ALDER_BUILD` (`os.Getenv` once in `Serve`; "" in tests is fine). The generated main's package comment gains one sentence each for the health route and the libsql rule.
- [ ] **Step 4: Run to pass**, then `go test ./rt/... ./internal/gen ./cmd/alder -count=1 -timeout 30m` and the three examples' `alder test` (unchanged).
- [ ] **Step 5: Commit** in two commits: `rt: /_alder/health and the start-time check on a remote store`; `rt/db: deploy tables in the shared database; deploy builds embed tzdata`.

### Task 4: The image and the registry push (`oci.go`)

**Files:**
- Create: `internal/deploy/oci.go`, `internal/deploy/cacerts/cacert.pem`, `internal/deploy/cacerts/README.md`, `internal/deploy/cacerts/cacerts.go` (`//go:embed cacert.pem` → `var PEM []byte`)
- Create: `internal/registryfake/registryfake.go` (the fake, used by Tasks 4, 6, 7)
- Test: `internal/deploy/oci_test.go`, `internal/deploy/testdata/oci/manifest.golden.json`, `internal/deploy/testdata/oci/config.golden.json`

**Interfaces:**
- Produces: `type Image struct{ Layer []byte; LayerDigest, DiffID string; Config []byte; ConfigDigest string; Manifest []byte; Digest string }`; `func BuildImage(artifact, id string) (*Image, error)` — the layer holds `app/app` (0755), `app/static/…`, `app/migrations/…`, `app/alder-deploy.json` (0644), `etc/ssl/certs/ca-certificates.crt` (0644, `cacerts.PEM`); directories 0755; entries sorted; mtime 2000-01-01T00:00:00Z; uid/gid 0; uname/gname ""; gzip header with zero ModTime, no name, OS 255. Config: `{"architecture":"amd64","os":"linux","config":{"Entrypoint":["/app/app"],"Env":["ALDER_BUILD=<id>","ALDER_STATIC=/app/static","ALDER_ADDR=:10000"],"ExposedPorts":{"10000/tcp":{}},"WorkingDir":"/app"},"rootfs":{"type":"layers","diff_ids":["sha256:…"]}}` with no `created` field. Manifest: OCI `application/vnd.oci.image.manifest.v1+json`, schemaVersion 2.
- Produces: `type Registry struct{ Host, Repo, User, Token string; Client *http.Client; Base string /* tests: the fake's URL; "" → https://<Host> */ }`; `func ParseImage(path string) (host, repo string, err error)`; `func (r *Registry) Push(ctx context.Context, img *Image, tag string) error`.
- Produces (fake): `registryfake.New(t, registryfake.Options{User, Token string; Fail503Once bool; RequireAuth bool}) *Server` with `.URL`, `.Requests() []string` (method + path), `.Manifest(repo, tag) []byte`, `.Blob(digest) []byte`.

- [ ] **Step 1: Vendor the bundle.** `curl -fsSL https://curl.se/ca/cacert.pem -o internal/deploy/cacerts/cacert.pem && shasum -a 256 internal/deploy/cacerts/cacert.pem`; write the README with the URL, today's date and the hash, and how to refresh it.
- [ ] **Step 2: Failing tests.**
  - `TestImage_Deterministic`: build the same artifact directory twice (touching file mtimes between) → equal `Digest`.
  - `TestImage_Golden`: config and manifest equal the goldens (digests in the goldens are those of a fixed fake artifact in `testdata/oci/artifact/`).
  - `TestImage_Layer`: untar the layer → the five paths with the modes above, `app/app` executable, nothing else.
  - `TestImage_DigestsMatch`: `LayerDigest` = sha256 of `Layer`; `DiffID` = sha256 of the gunzipped layer.
  - `TestParseImage`: `ghcr.io/tavish/blog` → `ghcr.io`, `tavish/blog`; `docker.io/u/app` → `registry-1.docker.io` as the API host, repo `u/app`.
  - `TestPush_FirstAndSecond`: first push → token exchange with Basic `user:token` and `scope=repository:tavish/blog:pull,push`, two blob uploads (`POST /v2/<repo>/blobs/uploads/` then `PUT <location>?digest=…`), manifest `PUT /v2/<repo>/manifests/<id>`; second push of another tag with the same layer → HEAD 200 for both blobs, no blob upload, one manifest PUT.
  - `TestPush_Retry503`: `Fail503Once` → the push succeeds after one retry.
  - `TestPush_ErrorHidesToken`: the fake answers 401 on the manifest with a body; the error names `401` and the body, and does not contain the token.
  - `TestPush_Anonymous`: `Token == ""` → the token exchange is made without Basic auth.
- [ ] **Step 3: Run to fail.** `go test ./internal/deploy -run 'TestImage|TestParseImage|TestPush' -count=1`.
- [ ] **Step 4: Implement** `oci.go` and the fake. Retries: 5xx and 429, three attempts, backoff 1 s, 2 s (a package `var retryWait = time.Second` the tests set to 1 ms), honouring `Retry-After` seconds.
- [ ] **Step 5: Run to pass**, `go vet`, and commit: `deploy: a one-layer OCI image and a registry v2 push, by hand`.

### Task 5: The Render API client (`render_api.go`) and its fake

**Files:**
- Create: `internal/deploy/render_api.go`, `internal/renderfake/renderfake.go`
- Test: `internal/deploy/render_api_test.go`

**Interfaces:**
- Produces: `type RenderAPI struct{ Base, Key string; Client *http.Client }`; `func NewRenderAPI() *RenderAPI` (`ALDER_RENDER_API`, `RENDER_API_KEY`). Methods, each `(ctx context.Context, …)`:
  - `Owners() ([]RenderOwner, error)`; `RenderOwner{ID, Name, Type string}`.
  - `ServiceByName(name string) (RenderService, bool, error)` (`GET /services?name=<n>&type=web_service&limit=20`, exact-name match); `RenderService{ID, Name, OwnerID, URL, Region, Plan string; NumInstances int; ImagePath, RegistryCredentialID string}`.
  - `CreateService(RenderCreate) (RenderService, string /* the creation deploy id */, error)`; `RenderCreate{OwnerID, Name, Region, Plan, ImagePath, CredentialID string; Instances int; Env []RenderEnv}` — body per the spec's step table (`healthCheckPath: "/_alder/health"`, `maxShutdownDelaySeconds: 30`).
  - `EnvVars(serviceID string) ([]RenderEnv, error)`; `PutEnvVars(serviceID string, env []RenderEnv) error`; `RenderEnv{Key, Value string}` with a `String()` that prints the key only.
  - `Scale(serviceID string, n int) error`.
  - `Credential(name string) (string, bool, error)`; `CreateCredential(ownerID, name, registry, user, token string) (string, error)`.
  - `CreateDeploy(serviceID, imageURL string) (RenderDeploy, error)`; `Deploy(serviceID, id string) (RenderDeploy, error)`; `Deploys(serviceID string, limit int) ([]RenderDeploy, error)`; `CancelDeploy(serviceID, id string) error`; `RenderDeploy{ID, Status, ImageURL string; CreatedAt, FinishedAt time.Time}`.
  - `Logs(ownerID, resource string, limit int, start time.Time) ([]RenderLog, time.Time /* next start */, error)`; `RenderLog{Time time.Time; Instance, Message string}`.
  - `LogsURL(ownerID, resource string) string` → `wss://…/logs/subscribe?…` (from `Base`, `http`→`ws`).
- Produces: `type RenderAPIError struct{ Status int; Message string }` — `Error()` is "Render API <status>: <message>", never the key.
- Failure statuses: `build_failed`, `update_failed`, `pre_deploy_failed`, `canceled`, `deactivated`. Success: `live`.
- Produces (fake): `renderfake.New(t) *Server` with `.URL`; `.Requests() []string`; `.Service(name) *Service` (env, instances, current image, deploy history); `.Script(statuses ...string)` sets the status sequence the next deploy walks through, one step per `GET`; `.Fail429Once()`; `.InstancesServe(image)` hook exposing a health handler per instance; `.Logs` canned lines; a `/logs/subscribe` websocket endpoint (raw frames, as in Task 1's test, or absent when Task 1 was dropped). Creation of a service starts a deploy of its image, like Render; a `PUT env-vars` does not.

- [ ] **Step 1: Failing tests.** One per method against the fake: request method, path, query and JSON body asserted from `.Requests()` and the fake's state; `TestRenderAPI_RetryAfter` (429 with `Retry-After: 1` → one retry, waited via a package `var renderWait` hook); `TestRenderAPI_ErrorHidesKey`; `TestRenderAPI_EnvString` (`fmt.Sprint(env)` contains no value).
- [ ] **Step 2: Run to fail.** `go test ./internal/deploy -run TestRenderAPI -count=1`.
- [ ] **Step 3: Implement** the client (list responses are `[{cursor, service|owner|envVar|deploy|registryCredential: …}]`; logs are `{hasMore, logs, nextStartTime}`; labels carry the instance id under `"instance"`). The implementer checks every path and body against Render's public API reference (`https://api-docs.render.com`) and records any difference from this plan in the task report; the fake follows the reference.
- [ ] **Step 4: Run to pass**, commit: `deploy: the Render REST client and a fake Render API`.

### Task 6: The `Render` target (`render.go`)

**Files:**
- Create: `internal/deploy/render.go`
- Modify: `internal/deploy/turso.go` (export what Render needs: `SetShared`, `Host(ctx) (string, error)`; `Preflight` unchanged), `internal/deploy/deploy.go` (only if the engine needs a hook; see Step 3)
- Test: `internal/deploy/render_test.go`

**Interfaces:**
- Consumes: Task 3's deploy-row helpers and health JSON; Task 4's `BuildImage`, `Registry.Push`; Task 5's `RenderAPI`; Task 1's `DialWS` (or polling); the existing `*Turso`.
- Produces: `type Render struct{ Env, AppName, Region, Service, Plan, Image string; Instances int; DB *Turso; API *RenderAPI; Reg *Registry; Artifact func(id string) string /* the local artifact dir */; Getenv func(string) string; Requires []string; Out io.Writer; HTTP *http.Client /* the health check */ }`; `func NewRender(app, env string, t ir.Target, d *ir.Deploy, image string) *Render`; it implements `Target`, `Locker`, `Preflighter`, `DataLossAllower`, `BackupRestorer`, `identity` (`"render:"+Service`) and has `Address() string` (R6).
- Step mapping (the spec's table with R2–R7): `Connect` → owners (first owner unless `RENDER_OWNER_ID` is set), service by name, `linux/amd64`, the shared DB name from the service's `ALDER_TURSO_DB` env var (`DB.SetShared`). `Requires` → R3. `Lock` → R7. `Schema` → Turso's (as Ssh+Turso reads it). `BuildSchema(id)` → the local artifact when present, else the builds row (R4). `Current`/`Builds` → rows (`Build.BackupOf` from the record's `previous` when `pitr` is set, `Running` = the id of the live deploy's image tag). `Upload` → build + push the image ("image pushed <image>:<id> (<size>)"); existing service: env merge + PUT, scale if `instances` changed, cancel any deploy in progress whose image is not ours. `Backup` → PITR time into the row (`PutDeployRow`, pitr only). `Migrate` → `DB.Migrate(ctx, id, allowLoss)`. `Start` → R2 or `CreateDeploy`; poll every 3 s (`var renderPoll` hook) until `live` or a failure status or `ALDER_DEPLOY_HEALTH_TIMEOUT`; a failure returns an error with the last 50 log lines; returns 443. `Health` → `GET <address>/_alder/health` until `instances` consecutive bodies carry `id` ("health: 2/2 instances serve <id>"). `Switch` → `SetCurrent`, return 443. `Stop` → nil. `Record` → `PutDeployRow` with record, schema, hash, migrations. `Prune` → `DeleteDeployRows` beyond keep, protect respected. `Info` → Turso storage bytes, "backup: Turso point-in-time", address, instances, live deploy id and age. `Logs` → last 100 lines (`[instance] message`); follow → `DialWS(LogsURL)` printing frames, or polling. `Restore(db string)` → refused as Ssh+Turso does ("restored to a point in time"). `RestoreBackup(to, src)` → `DB.CopyAt(shared, pitr of src)` → env PUT with the copy's `ALDER_DB`/`ALDER_TURSO_DB` → `Restored{From: "Turso point in time <t> of <db>", Kept: <old db name>, Undo: PUT the old env back}`.

- [ ] **Step 1: Failing scenario tests** (fake Render + fake registry + `tursofake`, the engine's `Deploy`/`Rollback`/`Status` driven as `deploy_test.go` drives Ssh):
  - `TestRender_FirstDeploy`: credential created (`alder-tavish/blog`, `GITHUB`), image pushed, service created with the spec's body (health path, `numInstances`, region, plan), env vars include `ALDER_DB=libsql://<host>`, `ALDER_TURSO_*`, `TURSO_API_TOKEN`, `ALDER_ADDR=:10000`, a 48-hex `ALDER_SECRET`, the requires values; Render live; health 1/1; current = id; one builds row.
  - `TestRender_SecondDeploy`: no credential or service creation; one manifest PUT, blob HEADs only; `ALDER_SECRET` unchanged; one `POST deploys` with `imageUrl: <image>:<id2>`.
  - `TestRender_EnvMergeKeepsDashboardVars` (Review Focus 1).
  - `TestRender_TwoInstances`: `instances: 2`; the fake's two instances answer alternately, the second one late → health waits for 2/2; `instances` changed 1 → 2 → one `POST scale`.
  - `TestRender_MigrationRefusedBeforeRender`: a DROPS DATA plan without `--allow-data-loss` → no POST/PUT at Render, the image may be pushed (Upload precedes Migrate: assert only Render).
  - `TestRender_UpdateFailed`: script `update_in_progress, update_failed` → error contains the canned log lines; current unchanged; no row for the new id marked current.
  - `TestRender_UpdateFailedAfterDropRestores` (Review Focus 3).
  - `TestRender_Rollback` and `TestRender_RollbackWithoutLocalArtifact` (Review Focus 4); `TestRender_RollbackRestoreBackup`: exactly one PITR copy and exactly one env-var PUT naming the copy's host.
  - `TestRender_LockHeld`: a row in `_alder_lock` held by `ci@runner` → refusal naming it, no Render request beyond Connect's GETs.
  - `TestRender_DryRun`: zero non-GET requests at all three fakes.
  - `TestRender_NoSecretsInOutput`: run every scenario's transcript and `--json` through a check that no env value set in the test appears.
  - `TestRender_Logs` and `TestRender_LogsFollow` (frames printed until ctx ends; or the polling variant).
- [ ] **Step 2: Run to fail.** `go test ./internal/deploy -run TestRender_ -count=1`.
- [ ] **Step 3: Implement.** If the engine (`deploy.go:78-355`) needs a change for a stateless target (for instance, `stopPrevious` or `Rerouter` assumptions), make the smallest one and cover it with an Ssh test that still passes unchanged; name it in the task report.
- [ ] **Step 4: Run to pass**, then `go test ./internal/deploy -count=1` (Ssh, Fly, Turso tests unchanged).
- [ ] **Step 5: Commit** in two commits: `deploy: the Render target (image, roll, health, state in Turso)`; `deploy: Render rollback, restore and logs`.

### Task 7: Commands and CI

**Files:**
- Modify: `cmd/alder/deploy.go` (`describeTarget`, `targetFor` `case "render"`, `withHost` refusal, `--json` `address`, `envTarget` for status/rollback/logs/restore, `restoreTurso` with the env PUT for Render), `cmd/alder/ci.go` (the Render branch), `cmd/alder/help.go` if it lists targets
- Create: `cmd/alder/image.go` (`imagePath(dir string, d *ir.Deploy) (string, error)`, R5), `cmd/alder/deploy_render_test.go`
- Test goldens: `cmd/alder/testdata/ci/render-only.yml`, `cmd/alder/testdata/ci/mixed.yml`

**Interfaces:**
- Consumes: `deploy.NewRender`, `(*Render).Address()`, `ir.RegistryKind`.
- Produces: `--json` gains `"address"` for every target: Ssh `https://<domain>`, Fly `https://<app>.fly.dev`, Render `Address()`.

- [ ] **Step 1: Failing tests.**
  - `TestImagePath`: `image` set → it; unset with `origin = git@github.com:Tavish/Blog.git` → `ghcr.io/tavish/blog`; with `https://github.com/t/b` → `ghcr.io/t/b`; with a GitLab origin or none → the E0030 sentence.
  - `TestDeployRender_HostRefused`: `alder deploy production --host x` → exit 2, "the address is the service's: https://<service>.onrender.com".
  - `TestDeployRender_JSONAddress` and the same field for Ssh and Fly (existing tests updated).
  - `TestDeployRender_EndToEnd`: `alder deploy production` on a tiny app against the three fakes (env `ALDER_RENDER_API`, `ALDER_TURSO_API`, the registry host mapped to the fake via `ALDER_REGISTRY_BASE` for tests only) → transcript has "image pushed", "Render deploy dep-… live (1 instance)", "health: 1/1 instances serve <id>"; then `status`, `rollback`, `logs` run.
  - `TestCiInit_RenderOnly` / `TestCiInit_Mixed`: goldens. The Render job has no SSH step; `permissions: packages: write`; `ALDER_REGISTRY_TOKEN: ${{ secrets.ALDER_REGISTRY_TOKEN || github.token }}`; header comment lists `RENDER_API_KEY`, `TURSO_API_TOKEN`, `ALDER_REGISTRY_TOKEN` and the requires names, the PAT sentence, and the first-deploy note; the preview comment step reads `address` from `alder deploy preview --json`.
- [ ] **Step 2: Run to fail.** `go test ./cmd/alder -run 'ImagePath|DeployRender|CiInit' -count=1`.
- [ ] **Step 3: Implement**, then regenerate the two new goldens and read them line by line against the spec's CI section.
- [ ] **Step 4: Run to pass**: `go test ./cmd/alder -count=1 -timeout 30m`; `actionlint` on the goldens when installed.
- [ ] **Step 5: Commit**: `alder: deploy, status, rollback, logs and restore for Render; address in --json`; `ci init: the Render branch`.

### Task 8: Book, reference, the manual real-account test

**Files:**
- Modify: `book/16-deploy.md` ("The Render target" after Fly), `book/12-rationale.md` (one rationale line), the experiments reference (≤ 48k), `book/README.md` if it lists targets
- Create: `cmd/alder/deploy_render_real_test.go` (`//go:build render`)
- Test: `go test ./book`, the experiments unittest

- [ ] **Step 1: The chapter section**, in this order: the declaration (the spec's example, checked by the book verifier as the other deploy blocks are), what the command reads (the four variables), a transcript taken from `TestDeployRender_EndToEnd`'s output and marked "against the test fakes; the real-account transcript replaces it after the real round", zero downtime and what it asks of migrations (the old build serves on the new schema; the DROPS DATA rule is the guard), instances, rollback and restore, the registry and the one token (the PAT sentence), CI, "why no disk mode" in two sentences, and R1's CA bundle in one sentence. `logs -f` described as built (websocket) or as polling, per Task 1.
- [ ] **Step 2: Chapter 12 line**: a PaaS deploys an image it is handed; a file database and a stateless service do not mix.
- [ ] **Step 3: The reference**: the `Render` target's fields and the environment variables; check the size with the experiments unittest.
- [ ] **Step 4: The manual test.** `TestRenderReal` skips unless `ALDER_RENDER_REAL=1`; it runs `alder deploy`, `status`, a second deploy, `rollback`, `logs` against a throwaway service and Turso database named `alder-m16-<random>`, and prints the cleanup commands rather than deleting anything. The file's comment says it is run by hand with the user, never in CI.
- [ ] **Step 5: Gate and commit.** `go test ./book -count=1`; the experiments unittest; commit `book: the Render target (chapter 16), rationale, reference`.

### Task 9: Friction fixes found by hand (added 2026-10-17 at the user's request)

The user, building an app by hand, hit these; they asked for them to be folded into this branch. Each is small and has its own test. The final whole-branch review covers them.

**Files:** `internal/check` (module paths, HTML attributes, `<script>`/`<style>` bodies), `internal/syntax` (raw-text elements), `internal/diag/explain.go`, `internal/gen` / `rt/html` (rendering raw text), corpus cases under `testdata/corpus/`, `book/03-syntax.md`, `book/05-html.md`, `book/B-pitfalls.md`.

- [ ] **Fix 1: E0005 suggests an impossible module name.** Today `app/pages/About.alder` with `module About` reports "expected `module pages.About`", which is itself a parse error. When a directory segment is not a valid module segment (lowercase first letter, or other characters), the message names the directory to rename instead: "module `About` is in file app/pages/About.alder: the folder `pages` must be a module name, starting with an uppercase letter: rename it `Pages` and write `module Pages.About`". Corpus: `E0005-lowercase-folder`; the explain text gains the sentence.
- [ ] **Fix 2: boolean attributes on media and other elements.** `<video controls>` is "attribute `controls` needs a value" and `autoplay={False}` is "expected Text, got Bool". Extend the checker's boolean-attribute set to the HTML standard's full list (at least `controls`, `autoplay`, `muted`, `loop`, `playsinline`, `default` (track), `allowfullscreen` (iframe), `async`, `defer`, `nomodule` (script), `reversed` (ol), `open` (details, dialog), `inert`, `itemscope`, `novalidate`, `formnovalidate`, `ismap`); a bare attribute and `={Bool}` both work, rendered as HTML boolean attributes. Corpus: `html-boolean-media-ok`; a render golden showing `<video controls muted>`.
- [ ] **Fix 3: inline `<script>` and `<style>`.** Today the body of `<script>` is read as Alder (`{ … }` becomes an expression), so inline JavaScript cannot be written. RULING: `<script>` and `<style>` are raw-text elements, as in HTML. Their body is literal text up to the closing tag, with no `{ … }` expressions, rendered unescaped. The checker refuses a body containing `</script` or `</style` (case-insensitive), and refuses `{ … }` interpolation inside them. Dynamic data reaches scripts through `data-*` attributes, which are escaped. Corpus: `html-inline-script-ok` (a body with `{`, `<`, `&&`, quotes), `E0001-script-close-in-body`; a render golden proving the body is byte-for-byte unchanged; book chapter 5 gets a short "Scripts and styles" paragraph (files in `static/` preferred; inline allowed as raw text; data through `data-*`), and Appendix B a pitfall.
- [ ] **Fix 4: earlier ledgered compiler bugs.** Reproduce each in a corpus or scratch case first; fix the ones that still fail, each with a regression test; record "already fixed" with the reproduction for the others: (a) a `Refresh _ ->` arm crashes `alder check`; (b) `msg Poke { }` used as a `click` crashes; (c) an unused binding in a match arm fails `go vet` on the generated code; (d) `src={Url.external "…"}` on `<script>` is refused or mis-rendered.
- [ ] **Gate and commits:** one commit per fix (`check: …`), then the gate from Global Constraints (all tests, the three examples and their scores, `go test ./book`).

---

## Self-review notes

- **Spec coverage.** Declaration and E0030: Task 2 (+ R5 in Task 7). Environment variables: Tasks 6–7. Image and registry: Task 4 (+ R1). Step table: Task 6 (+ R2–R4, R6, R7). `/_alder/health` and no start migration: Task 3 (+ R8). Commands: Tasks 6–7. CI: Task 7. Testing list: Tasks 1, 4–7. Book: Task 8. Non-goals: untouched.
- **Interfaces.** Task 6 consumes Tasks 1, 3, 4, 5 by the names above; Task 7 consumes Task 6's `NewRender` and `Address`; Task 2's `ir.RegistryKind` is used by Tasks 6 and 7.
- **Review Focus.** Each line names its test and owning task.
- **Open to the implementer, stated in reports:** exact Render response shapes against the API reference (Task 5); whether `instances: 2` needs a parser change (Task 2); any engine change for a stateless target (Task 6).
