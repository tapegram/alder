# Absolute URLs: a configured public base and `Url.absolute`

Design for [tapegram/alder#9](https://github.com/tapegram/alder/issues/9) and
[#32](https://github.com/tapegram/alder/issues/32). Both come from nightvote-replay, which prints
absolute URLs for another program to fetch (NightVote's `base_url` per county, `/start`'s
`status_url`) and today gets its origin from a hand-written Go capability reading `PUBLIC_BASE`
(`app/Origin.alder`, `adapters/origin`). One design closes both issues. Everything marked
**(call)** is the author's decision, not the user's.

## Intent

A page value already knows its own URL (`rt.PageURL`: `/r/t0=1,key=k?date=…`); what Alder lacks
is the scheme and host in front of it. After this change the program is told its public address
once, as configuration, and `Url.absolute (Replay { cfg: raw })` gives
`https://nightvote-replay-test.onrender.com/r/t0=1,key=k` in production, `http://localhost:8080/r/…`
under `alder run` and in tests, with no hand-written adapter and no hard-coded host. Links stay
typed: the only way to an absolute URL is through a `Page`.

## Goals

1. `Url.absolute : Page -> Text`, usable anywhere a `Page` is, including a `view`.
2. One environment variable, `ALDER_PUBLIC_URL`, read by the executable, with a correct default
   wherever alder itself starts the executable (`alder run`, the test runner) and set by `alder
   deploy` for every target whose address it already knows.
3. A deterministic value in tests, so a test can assert on a printed absolute URL.
4. nightvote-replay can delete `Origin.alder`, `adapters/origin` and `PUBLIC_BASE`.

## Non-goals

- Reading the request's origin (`Host`, `X-Forwarded-Proto`, `X-Forwarded-Host`). See "Why no
  request origin" below: #32's ask is satisfied by the configured base instead.
- A `Url.origin : Text` constant, or any other way to build an absolute URL from text. The site's
  every address is a page value (`Url.external "/static/x"` for a static file), so there is no
  real case, and an untyped base invites `"{origin}/r/{raw}/clarity"` string building, which #9
  exists to end.
- Mounting the app under a path prefix (`https://ex.com/app`). Routes are absolute today; the
  base is a scheme and host only.
- A per-test or per-request override of the base.
- Touching the `Secure` cookie flag, which keeps reading `X-Forwarded-Proto` (a lower-stakes use:
  it only decides whether the cookie is sent over plain HTTP).
- `Text.Url`, the brand type in `stdlib/Text.alder`, is unrelated and untouched.

## Anti-goals

- A function whose answer depends on which request is running. `view` is pure and `load` runs
  outside any request in a test's `Test.run`; an answer that changes with the request would make
  `Url.absolute` unusable in both.
- Trusting a header a client can write. A spoofed `Host` on a page that prints links to itself is
  the classic password-reset-link attack and a cache-poisoning vector; a trusted-proxy list would
  be more configuration than the base itself.

## Why no request origin (what #32 asked, and what each ask gets)

#32 asks for three things. (1) A way to read the origin inside `load`/`update`: `Url.absolute`
is that, and works in `view` too, where `Session.origin : Text` as a task could not. (2) Honouring
`X-Forwarded-Proto`/`Host` behind Render's proxy: not offered **(call)**; the configured base
replaces it, and `alder deploy` sets the base to the address it health-checks, so a Render or Fly
deploy needs nothing by hand. Rails (`default_url_options[:host]`), Django (`SITE_ID`/
`ALLOWED_HOSTS`) and Phoenix (`url: [host: …]`) all configure the host for the same reasons.
(3) An origin for `Test.get`/`Test.fetch`: tests see the fixed default `http://localhost:8080`,
below. A request-origin reader is not planned; if a tunnel or a second hostname needs a different
base, the operator sets `ALDER_PUBLIC_URL` for that process.

## The API

```alder
-- stdlib/Url.alder
absolute : Page -> Text   -- the page's URL with the configured public base in front
```

Pure **(call)**: the base is a process constant, set once before the first request and never
changed, so `Url.absolute p` is a function of `p` for the life of the process, like a route
pattern compiled into the program. `rt.PublicURL` holds it (`rt/core.go`, beside `Page`), default
`rt.DefaultPublicURL = "http://localhost:8080"`.

Semantics (`rt/std/text.go`, `UrlAbsolute`, mapped by `internal/gen/stdcall.go`):

- Let `u = rt.PageURL(p)` (the same text `href` renders, defaulted parameters left out, the query
  URL-encoded). If `u` starts with `/`, the answer is `rt.PublicURL ++ u`; otherwise `u` unchanged
  **(call)**. So an `Url.external "https://ex.com/a"` page stays `https://ex.com/a`, `#top` stays
  `#top`, `Url.external "javascript:…"` (already `#`) stays `#`, and `Url.external "/static/app.css"`
  becomes `http://localhost:8080/static/app.css`.
- `Url.absolute (Todos.Index {}) == Url.absolute (Todos.Index { filter: All })`, since the pages
  are equal.

## `ALDER_PUBLIC_URL`

The executable reads `ALDER_PUBLIC_URL` (generated `main`, `internal/gen/caps.go`) **(call)** —
the name #9 proposes; `ALDER_HOST` already means the `--host` of a preview deploy.

- Shape: `http://host[:port]` or `https://host[:port]`. The value is trimmed of spaces and of
  every trailing `/`; then it must parse (`net/url`) with scheme `http` or `https`, a non-empty
  host, and no path, query, fragment or user info. The scheme is lowercased (`url.Parse` does),
  the host kept as written, the port kept. `rt.ParsePublicURL(s string) (string, error)` is this
  rule; `main` calls it before `app.Serve` and on an error prints
  `alder: ALDER_PUBLIC_URL must be http(s)://host[:port], such as https://blog.example.com (got "…")`
  and exits 1, as a bad `ALDER_SECRET` does. Refused: `localhost:8080` (no scheme), `ftp://x`,
  `https://ex.com/app`, `https://ex.com/?a=1`, `https://ex.com/#x`, `https://u:p@ex.com`, `https://`.
  Accepted: `https://ex.com/` → `https://ex.com`, ` http://localhost:18080// ` →
  `http://localhost:18080`, `HTTPS://Ex.com` → `https://Ex.com`.
- Unset or blank: after `net.Listen`, `main` sets the base to `http://` + the listener's address
  as `alder run` displays it (`cmd/alder/run.go` `displayAddr`: an unspecified host, `:8080` or
  `0.0.0.0:8080`, is shown as `localhost`; `127.0.0.1:18765` is kept) **(call)**. So
  `ALDER_ADDR=127.0.0.1:0` gives the port the operating system picked, and the nightvote-replay
  dev-log complaint ("a second local copy needs both `ALDER_ADDR` and `PUBLIC_BASE`") goes away.
  The `listening on <addr>` line is unchanged.
- Nothing else reads the variable: `alder eval`, `alder scratch`, the REPL and the test runner
  leave `rt.PublicURL` at its default.

### `alder run`

`startChild` (`cmd/alder/run.go`) appends `ALDER_PUBLIC_URL=http://<d.addr>` (the dev server's
displayed address, `localhost:8080` by default) to the child's environment, because the child
listens on `127.0.0.1:0` behind the proxy and would otherwise derive the wrong port. When the
shell that runs `alder run` has `ALDER_PUBLIC_URL` set (a tunnel, `https://x.ngrok.app`), it is
passed through untouched **(call)**: the person who set it knows the address the browser uses.

### Tests

The test runner (`rt/testing`) never sets `rt.PublicURL`, so inside `alder test` every
`Url.absolute` starts with `http://localhost:8080`, whatever the environment holds **(call)**: a
test's expected text is the same on every machine and in CI. This is the origin #32's third ask
wanted for `Test.get`/`Test.fetch`; the browser's requests themselves are unchanged
(`httptest.NewRequest` with the page's URL, `Referer: http://example.com…`).

### `alder deploy`

Each target sets the variable to the address `deployAddress` (`cmd/alder/deploy.go`) already
reports and health-checks, as a default the operator can override **(call)**:

| target | where | override |
|---|---|---|
| `Ssh` | the `run` wrapper (`internal/deploy/ssh.go` `runScript`), after the environment file is sourced: `: "${ALDER_PUBLIC_URL:=https://<domain>}"; export ALDER_PUBLIC_URL`; with `--host`, that host; a wildcard domain without `--host` writes no line | a line in `<path>/env` wins |
| `Fly` | the machine config's `env` (`fly.go` `config`): `https://<app>.fly.dev` | a Fly secret of the same name wins (Fly's rule) |
| `Render` | `mergedEnv` (`render.go`): added to `set` only when the service does not already have it, value `r.Address()` | the dashboard value is kept |
| `Script` | nothing: the script knows its platform (`ALDER_HOST` is already there for `--host`) | — |

`proxy: None` on an `Ssh` target still gets `https://<domain>`: `domain` is "the name the proxy
serves", and the operator who put their own proxy in front overrides in `<path>/env`.
`alder deploy init` writes `ALDER_PUBLIC_URL` into `render.yaml` (`https://<app>.onrender.com`,
with the comment `# change if Render gave the service another name`) and into `fly.toml`'s
`[env]` (`https://<app>.fly.dev`) **(call)**: a Blueprint app (nightvote-replay's setup) otherwise
derives `http://localhost:10000`.

## Where things change

| layer | files |
|---|---|
| signature | `stdlib/Url.alder`: `absolute : Page -> Text` (the checker reads it; `Page` is a built-in type, nothing to add in `internal/check` or `internal/types`) |
| name → Go | `internal/gen/stdcall.go`: `"Url.absolute": "std.UrlAbsolute"` |
| runtime | `rt/core.go`: `DefaultPublicURL`, `PublicURL`, `ParsePublicURL`; `rt/std/text.go`: `UrlAbsolute` beside `UrlExternal` |
| generated main | `internal/gen/caps.go` (the `main` template around line 745, the `Listen` at ~786, and the package comments at ~678 and ~707 listing the variables): read, validate, derive after `Listen`, set |
| dev server | `cmd/alder/run.go` `startChild`: the child's environment |
| deploy | `internal/deploy/ssh.go` `runScript`; `fly.go` `config`; `render.go` `mergedEnv`; `init.go` `renderYAML` and the Fly TOML |
| parser, formatter, IR, LSP, tree-sitter | nothing: a stdlib call is an ordinary qualified name; completion and hover read the signature file |
| book | `09-stdlib.md` (the `Url` lines after `Url.external`, with the semantics above); `10-testing.md` (one paragraph after `Test.fetch`: what `Url.absolute` gives in a test); `11-tooling.md` ("five environment variables" → six, a row for `ALDER_PUBLIC_URL`, and its derived default under `alder build`; a sentence under `alder run`); `13-compiler.md` (the `startChild` environment, line ~2155; the wrapper's variables, line ~4443); `16-deploy.md` (the `run` line in the state listing, line ~313; one sentence each in the Fly and Render sections and under `alder deploy init`; a row in "The knobs") |
| site | `book/site/build.py` regenerates `site/` from the book; run it after the book edit |

Also: tick #9 and #32 in roadmap #52; the user reinstalls (`go install ./cmd/alder`) after merge.

## nightvote-replay after this (not part of the change; what the builder checks against)

`Origin.alder`, `adapters/origin` and the `Origin` wiring go; `Links.sources` loses its `origin`
parameter and builds each `base_url` with `Url.absolute` on the page it names (`Replay { cfg }`,
the per-vendor pages under `/r/{cfg}/…`); `Start`'s `status_url` is `Url.absolute (ReplayStatus
{ cfg: raw })`; `model.origin` goes from `Index` and `Replay`; `render.yaml` replaces
`PUBLIC_BASE` with `ALDER_PUBLIC_URL` at the same value; tests expect `http://localhost:8080/r/…`
instead of `http://test.local/r/…`. Local runs set nothing.

## Tests the builder adds (named by behaviour)

- `rt/std`: absolute of a page with a path parameter and a query is the base plus `PageURL`;
  absolute of a page at its defaults omits the defaulted parameter; absolute of an external
  `https://` page is that URL unchanged; absolute of `Url.external "/static/app.css"` is the base
  plus the path; absolute of `#top` and of a sanitised `#` is unchanged.
- `rt` (`ParsePublicURL`): each accepted and refused value listed above, as a table.
- Generated main (`cmd/alder`, beside the run/build tests, with a scratch app whose page shows
  `Url.absolute` of itself): a bad `ALDER_PUBLIC_URL` exits 1 with the message, before anything is
  served; unset with `ALDER_ADDR=127.0.0.1:0` gives `http://127.0.0.1:<the port in "listening
  on">`; unset with `ALDER_ADDR=:0` gives `http://localhost:<port>`; set to `https://ex.com/` gives
  `https://ex.com/…`.
- `alder run`: the child's environment carries `ALDER_PUBLIC_URL=http://localhost:8080` (the dev
  server's address, or the `--addr`/`ALDER_ADDR` one); a value in the shell passes through.
- `rt/testing`: `Url.absolute` inside a test is `http://localhost:8080/…` when the runner's
  environment sets `ALDER_PUBLIC_URL` to something else.
- deploy: the `Ssh` wrapper holds the default line with the domain, with the `--host` host, and
  no line for a wildcard domain without a host; an `<path>/env` value wins (the existing
  fake-ssh harness runs the wrapper); the Fly config's env carries `https://<app>.fly.dev`; the
  Render merge adds the variable when the service lacks it and keeps a dashboard value; `deploy
  init`'s `render.yaml` and `fly.toml` carry it.
- corpus: `url-absolute-ok`, a page whose `view` interpolates `Url.absolute (Index {})`, checks and
  builds; `E0003-url-absolute-text`: `Url.absolute "/x"` is a type mismatch, since the argument is
  a `Page`.
- book: the existing book tests pass with the new paragraphs; `internal/deploy/ssh_test.go`
  (line ~67) lists the wrapper's lines and gets the new one.

## Verification

- `go test ./...` green, the new cases included.
- Scratch check, not committed: a copy of nightvote-replay rewritten as the section above
  describes compiles, its tests pass with `http://localhost:8080` expectations, and a local run
  with `ALDER_ADDR=127.0.0.1:18099` and nothing else prints `base_url`s starting
  `http://127.0.0.1:18099/r/`.
