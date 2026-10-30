# Absolute URLs (#9, #32): implementation plan

Spec: `docs/superpowers/specs/2026-10-29-absolute-urls-design.md` (binding). Test first, one commit
per task.

## Task 1: runtime base and `ParsePublicURL` (`rt`)

- Test `rt/core_test.go` `TestParsePublicURL`: table of the spec's accepted values
  (`https://ex.com/` → `https://ex.com`, ` http://localhost:18080// ` → `http://localhost:18080`,
  `HTTPS://Ex.com` → `https://Ex.com`) and refused ones (`localhost:8080`, `ftp://x`,
  `https://ex.com/app`, `https://ex.com/?a=1`, `https://ex.com/#x`, `https://u:p@ex.com`,
  `https://`), the error text naming the value.
- Test `TestListenPublicURL`: `[::]:8080` and `0.0.0.0:8080` → `http://localhost:8080`;
  `127.0.0.1:18765` → `http://127.0.0.1:18765`.
- Code `rt/core.go`: `DefaultPublicURL`, `var PublicURL = DefaultPublicURL`, `ParsePublicURL`,
  `ListenPublicURL(net.Addr)` (displayAddr's rule; `cmd/alder` `displayAddr` reuses
  `rt.DisplayAddr`).

## Task 2: `Url.absolute` (stdlib, gen mapping, `rt/std`)

- Test `rt/std/std_test.go` `TestUrlAbsolute`: path parameter + query; defaulted parameter left
  out; external `https://` unchanged; `Url.external "/static/app.css"` → base + path; `#top` and a
  sanitised `#` unchanged; a changed `rt.PublicURL` is used.
- Code: `stdlib/Url.alder` `absolute : Page -> Text`; `internal/gen/stdcall.go`
  `"Url.absolute": "std.UrlAbsolute"`; `rt/std/text.go` `UrlAbsolute`.
- Corpus: `testdata/corpus/url-absolute-ok` (a page whose `view` interpolates
  `Url.absolute (Index {})`, with a test asserting on it) and
  `testdata/corpus/E0003-url-absolute-text` (`Url.absolute "/x"`); expected.diag via
  `TestCorpus`.

## Task 3: generated `main` reads `ALDER_PUBLIC_URL` (`internal/gen/caps.go`)

- Tests `cmd/alder/publicurl_test.go`, building `testdata/corpus/url-absolute-ok`:
  - `TestPublicURL_BadValueExits`: `ALDER_PUBLIC_URL=localhost:8080` exits 1 with the message
    and prints no `listening on`.
  - `TestPublicURL_DerivedFromListener`: unset, `ALDER_ADDR=127.0.0.1:0` → page shows
    `http://127.0.0.1:<port>/`; `ALDER_ADDR=:0` → `http://localhost:<port>/`.
  - `TestPublicURL_Set`: `https://ex.com/` → `https://ex.com/`.
- Code: Main template imports `alder/rt`, parses before `app.Serve`, derives after `Listen`;
  package comments list the variable.

## Task 4: `alder run` passes the dev server's address

- Test `cmd/alder/run_test.go` `TestChildEnv_PublicURL`: `childEnv` carries
  `ALDER_PUBLIC_URL=http://localhost:8080` (and an `--addr` one); with the variable in the
  environment it is passed through untouched (not appended).
- Code: `cmd/alder/run.go`: `childEnv(environ, …, devAddr)` used by `startChild`.

## Task 5: tests see the fixed default

- Test `cmd/alder/publicurl_test.go` `TestPublicURL_TestRunnerIgnoresEnv`: `alder test` on
  url-absolute-ok with `ALDER_PUBLIC_URL=https://elsewhere.example` set passes (its test expects
  `http://localhost:8080/`). Nothing to code if green (the runner never sets the base).

## Task 6: deploy targets

- `internal/deploy/ssh_test.go`: `TestRunScript` gets the default line;
  `TestRunScript_PublicURL`: `--host` host; wildcard without host → no line; running the
  wrapper with sh (a fake `app` printing the variable) shows the default, and an `<path>/env`
  value wins.
- `internal/deploy/fly_test.go` `TestFly_ConfigPublicURL`: `https://<app>.fly.dev`.
- `internal/deploy/render_test.go`: first deploy sets `ALDER_PUBLIC_URL` to the address;
  `TestRender_EnvMergeKeepsDashboardPublicURL` keeps a dashboard value.
- `internal/deploy/init_test.go`: render.yaml (`https://blog.onrender.com` + comment) and
  fly.toml `[env]` (`https://blog.fly.dev`).
- Code: `ssh.go` `runScript`, `fly.go` `config`, `render.go` `mergedEnv`, `init.go`.

## Task 7: book

- `09-stdlib.md` (Url lines), `10-testing.md` (paragraph after `Test.fetch`), `11-tooling.md`
  (six variables, row, derived default, `alder run` sentence), `13-compiler.md` (startChild env,
  wrapper's variables), `16-deploy.md` (state listing `run` line, Fly, Render, deploy init, knobs).
- `python3 book/site/build.py`; `cd experiments && python3 -m unittest`.

## Finish

`go vet ./...`, `go test ./...`, rebase on origin/main, push, PR "Closes #9. Closes #32."
