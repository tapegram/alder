# M10: Distribution — Design

Date: 2026-10-12. Status: approved in conversation (license and hosting
decided by the user); the M10 plan argues from this.

## Goal

Someone who is not us installs Alder with one command and builds, tests
and runs an app without a checkout of this repository. Today the compiler
works only from a checkout: the standard library is embedded
(`stdlib/embed.go`) but every generated app's `go.mod` says
`replace alder => <absolute path of the checkout>`.

## Decisions recorded (2026-10-12)

- **License: Apache-2.0**, matching Olifant (`sorcererdev/olifant`).
- **Hosting: `github.com/tapegram/alder`** under the user's account, private
  at first (public is a flip of a switch at release time); it may move to
  the `sorcererdev` organisation later. Therefore **nothing generated depends
  on the GitHub path**: the embedded runtime keeps the module name `alder`
  and a local `replace`, so a move changes only the install instructions.
- **Go on PATH is the one prerequisite.** `alder build` and `alder test`
  run `go build`; adapters are Go. The book says so in chapter 1 with the
  version floor.
- **Embed, never fetch.** The runtime (`rt/**`, the Datastar script, the
  Go module files it needs) ships inside the compiler binary and is
  written next to the generated app; a build needs no network and the
  runtime always matches the compiler that generated the code.

## The pieces

### 1. The runtime travels with the binary

- `rt/embed.go` (or `internal/dist`) embeds every `.go` file under `rt/`
  (not `_test.go`), `rt/datastar/*.js`, and a `go.mod`/`go.sum` for the
  module `alder` listing only its real dependency (`modernc.org/sqlite`
  and what it pulls in, copied from the repository's `go.sum`).
- `alder build`, the test build and the scratch builds write the runtime
  into `build/<app>/rt/` (refreshed on every build: files the embedded
  copy no longer has are removed) and the generated `go.mod` says
  `replace alder => ./rt` (relative, so the build directory is relocatable).
  The `go.sum` is written too, so `go build` is offline given a warm module
  cache; the first build on a machine fetches `modernc.org/sqlite` once
  through the normal Go proxy (documented).
- Developing Alder itself: a checkout's binary still works; the embedded
  copy is what it embeds, so the examples and the book verifier exercise
  the same path as a user.
- `alder version` prints the version (from `debug.ReadBuildInfo`: the
  module version for a `go install`ed or released binary; `devel+<commit>`
  for a checkout build) and the Go version it was built with; the
  generated `app__alder.go` header records it.

### 2. `alder new <name>`

- The first generator: `alder new guestbook` creates `guestbook/` with
  `app/App.alder` (an empty session, error and not-found pages),
  `app/Layout.alder`, `app/Entry.alder` (one table), `app/Home.alder`
  (a page listing entries with a form, one browser test), `adapters/`
  (empty, with a README line), `migrations/` (written by running the
  migration generator), `static/app.css` (a few lines), a `README.md`
  (the five commands), and a `go.mod` only when the directory is inside
  another Go module (as the examples need). The result checks, tests
  (1 test passing, mutation score reported), builds and serves.
- Refuses an existing non-empty directory; the name must be a valid
  module name (letters, digits, `-`).
- The scaffold is the book's guestbook from the "tiny guide" (chapter 2
  reuses it), so the book's first app and the generator agree by
  construction (the verifier builds the scaffold and checks it against
  chapter 2's blocks).

### 3. Releases

- `.goreleaser.yaml`: builds for darwin/linux × amd64/arm64 (and windows
  amd64 if it builds without changes; otherwise documented as untested),
  `CGO_ENABLED=0` (modernc is pure Go), version ldflags not needed (build
  info suffices), archives named `alder_<version>_<os>_<arch>`, checksums,
  release notes from the tag's annotation, a Homebrew formula pushed to
  `tapegram/homebrew-alder`, and a container image (`ghcr.io/tapegram/alder`)
  with Go installed so `alder build` works inside it.
- GitHub Actions: `ci.yml` on push and PR (gofmt, vet, `go test ./...`,
  the examples' check/build/test, `go test ./book`, the experiments'
  unittest), `release.yml` on a `v*` tag running GoReleaser.
- `go install` is NOT supported: the root module stays `alder` (renaming it
  to the GitHub path would churn every import and the embedded runtime's
  module identity); distribution is release binaries, Homebrew and the
  container image (ruled 2026-10-12 after Task 1).
- The first tag is the user's call; the plan ends with a successful
  `goreleaser release --snapshot` locally and the workflows green on a
  push.

### 4. The repository

- Create `github.com/tapegram/alder` (private), push `main`, set the
  default branch, enable Actions. The LICENSE file (Apache-2.0, copyright
  the user) at the root; a `NOTICE` is not needed. Every source file keeps
  its current header style (no per-file license headers).
- `README.md` at the root: what Alder is in three sentences, install
  (release binary / Homebrew / `go install`), the five-command guestbook,
  links to the book and the experiments, the license.

### 5. The docs site

- `book/site/build.py` renders the book's chapters to a static site under
  `site/` (one page per chapter, a sidebar from `book/README.md`'s order,
  code blocks with the language class; no JavaScript needed), plus the
  agent reference as `site/reference.md` (the output of
  `build_context.py --reference`) and `site/reference-full.md`.
- `pages.yml` publishes `site/` to GitHub Pages on each release tag
  (and on demand); until the repository is public the site is reachable
  only to the user.
- The verifier does not check the site; the site is a rendering of
  verified sources.

## Book

- Chapter 1 (or 2): "Installing" (the three ways; Go on PATH; `alder
  version`; the container image for CI) and `alder new` as the first
  command; chapter 2's guestbook becomes the scaffold's output.
- Chapter 11: `alder new`, `alder version`, the `build/<app>/rt/` layout
  and the relative `replace`.
- Chapter 13: how the runtime is embedded and written out; why the module
  name stays `alder`.
- Chapter 12: why embed rather than fetch; why Go is a prerequisite.

## Testing

- `rt/embed` round-trip: the embedded file list equals `git ls-files rt`
  minus tests (a test fails when a file is added without being embedded).
- A build from a scratch directory outside the repository, with the
  repository renamed temporarily (or `PATH` holding only the built
  binary and `go`), succeeds for both examples and for `alder new`'s
  scaffold; the served binary answers `/`.
- `alder new`: the scaffold checks, tests, builds; refuses non-empty
  directories; invalid names.
- `alder version` on a checkout build and on a `go install` build.
- GoReleaser snapshot build succeeds for every target; the container
  image builds the scaffold inside.

## Non-goals

- Windows support beyond "builds if it builds"; signing/notarisation;
  an installer script (`curl | sh`); a package for every distribution;
  moving to the organisation (a later rename of remotes and docs).
