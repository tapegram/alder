# Roadmap #52 item 4: blocking bugs and the v0.2.0 release — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix #172 and #175, write the first CHANGELOG, and tag v0.2.0 so the Homebrew alder has `alder lsp` (#174).

**Architecture:** Three small fixes land on `main` first (migration listing without globbing, a clearer E0017, a hint on the `Json.Value`/`Text` mismatch). Then `CHANGELOG.md` becomes the source of the GitHub release notes. The release itself is one tag push: `.github/workflows/release.yml` runs GoReleaser, which writes the release, pushes the Homebrew formula and the container image.

**Tech Stack:** Go, GoReleaser v2, GitHub Actions, `gh`.

**Spec:** none. The brainstorm was skipped at the user's request; the decisions are in "Decisions" below and are the plan author's calls unless marked otherwise. Issues: tapegram/alder #172, #174, #175.

## Decisions

1. **No `Json.raw`.** The Json design (`docs/superpowers/specs/2026-10-29-json-encode-design.md`, Non-goals) already ruled it out, and a passthrough exists: `File.text { contentType: "application/json", body: t }`. #175's claim that there is none is wrong. The fix is to make the compiler say so.
2. **E0017 stays an error.** A user module does not shadow a stdlib module. The message says what to do.
3. **Version is v0.2.0**: there are breaking changes since v0.1.0 and the project is pre-1.0.
4. **Release notes come from `CHANGELOG.md`**, not from the 672 commit subjects since v0.1.0.
5. **#172 covers `internal/deploy` too**: it has the same two globs under an app's `migrations/`. The runtime's globs under a data directory (`rt/db/stores.go:1264`, `rt/backup/silos.go:84`) are not touched here; Task 2 files an issue for them.
6. **nightvote-replay is not migrated here.** It still has `app/Json.alder`; it is used read-only to check how the new messages read.

## Global Constraints

- Work on `main`; no side branches (the user's rule). Commit after each task.
- No agent or background work between 9am and 5pm America/New_York on weekdays.
- Every subagent gets an explicit `model`: Sonnet for Tasks 1–5, Opus for the final review.
- Nothing is tagged, and nothing is pushed to another repository, before the user says go (Task 6, step 4).
- Never comment on, merge or close an outside contributor's PR.
- Commit messages follow the repository's style (`area: what changed (#issue)`) and end with the attribution lines of the session.
- Quick loop: `scripts/test.sh -short`. Full suite: `scripts/test.sh` (about 6 minutes), run in the foreground under a timeout.

## Review Focus

1. A `migrations/` directory that holds a subdirectory named `dir.sql`, or a file named just `.sql`: neither is listed as a migration. Test in Task 2.
2. An app path with `[`, `]`, `*` and `?` at once, and an unclosed `[`: migrations are found. Test in Task 2.
3. A missing `migrations/` directory: no error, no files (as the glob behaved). Test in Task 2.
4. The `Json.Value` hint appears only when the given type is `Text`, not for `Int` or a record. Test in Task 4.
5. A tag pushed with no `## vX.Y.Z` section in `CHANGELOG.md`: the release job fails before GoReleaser publishes anything. Test in Task 5.

---

### Task 1: Make CI on `main` green again

The `lint` job has failed since 2026-11-10: `experiments/test_build_context.py::test_real_book_headings_exist` looks for `### All fourteen, and one killed by luck`, and `book/10-testing.md:2679` now says `### All twelve, and one killed by luck`. The book is right.

**Files:**
- Modify: `experiments/build_context.py:93`, `experiments/test_build_context.py:305`

- [ ] **Step 1:** Run `cd experiments && python3 -m unittest test_build_context 2>&1 | tail -5`. Expected: FAIL, `'\n### All fourteen, and one killed by luck\n' not found`.
- [ ] **Step 2:** Replace `All fourteen, and one killed by luck` with `All twelve, and one killed by luck` in both files.
- [ ] **Step 3:** Run `cd experiments && python3 -m unittest`. Expected: `OK`. If another heading fails, fix it the same way: the book's heading wins.
- [ ] **Step 4:** Run `python3 book/site/test_site.py`. Expected: pass.
- [ ] **Step 5:** Commit: `experiments: the context builder follows ch. 10's renamed mutation heading`.

### Task 2: #172 — list migrations without globbing

**Files:**
- Create: `internal/migrate/sqlfiles.go`, `internal/migrate/sqlfiles_test.go`
- Modify: `internal/buildkit/migrations.go:32,48`, `internal/deploy/render.go:526`, `internal/deploy/plan.go:108`
- Test: `internal/buildkit/migrations_test.go`

**Interfaces:**
- Produces: `migrate.SQLFiles(dir string) ([]string, error)`: the paths (`filepath.Join(dir, name)`) of the regular files directly in `dir` whose name ends in `.sql` and is longer than `.sql`, sorted. A `dir` that does not exist gives `nil, nil`. Any other read error is returned.

- [ ] **Step 1: Write the failing tests** in `internal/migrate/sqlfiles_test.go`:

```go
func TestSQLFiles(t *testing.T) {
	for _, parent := range []string{"plain", "e2e-[x]-abc", "a*b?c", "open[bracket"} {
		t.Run(parent, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), parent, "migrations")
			must(t, os.MkdirAll(filepath.Join(dir, "dir.sql"), 0o755)) // a directory: not a migration
			for _, n := range []string{"0002_b.sql", "0001_a.sql", "schema.json", ".sql", "notes.sql.bak"} {
				must(t, os.WriteFile(filepath.Join(dir, n), nil, 0o644))
			}
			got, err := SQLFiles(dir)
			want := []string{filepath.Join(dir, "0001_a.sql"), filepath.Join(dir, "0002_b.sql")}
			if err != nil || !slices.Equal(got, want) {
				t.Fatalf("SQLFiles = %v, %v; want %v", got, err, want)
			}
		})
	}
}

func TestSQLFiles_MissingDir(t *testing.T) {
	got, err := SQLFiles(filepath.Join(t.TempDir(), "none"))
	if got != nil || err != nil {
		t.Fatalf("SQLFiles = %v, %v; want nil, nil", got, err)
	}
}
```

  (`must` is a three-line helper in the test file if the package has none. Skip the `a*b?c` case with `t.Skip` on Windows: those characters are not allowed in its paths.)

- [ ] **Step 2:** Run `go test ./internal/migrate -run TestSQLFiles`. Expected: FAIL, `undefined: SQLFiles`.
- [ ] **Step 3:** Implement `SQLFiles` with `os.ReadDir` (it returns entries sorted by name). Comment it in the package's style, saying why it does not glob (#172).
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5: Add the failing caller test** to `internal/buildkit/migrations_test.go`, next to `TestCopyMigrations`: `TestCopyMigrations_GlobCharsInPath` builds `src := filepath.Join(t.TempDir(), "demo-[x]", "migrations")` with `schema.json` and `0001_init.sql`, calls `CopyMigrations(src, dst)` and asserts no error and that `dst/0001_init.sql` exists. Run it. Expected: FAIL with `has schema.json but no migration (*.sql)`.
- [ ] **Step 6:** Replace the four `filepath.Glob(… "*.sql")` calls with `migrate.SQLFiles`. Keep each caller's error handling as it is (three ignore the error, `plan.go` returns it). Remove sorts that are now redundant and imports that are now unused.
- [ ] **Step 7:** Run `go test ./internal/migrate ./internal/buildkit ./internal/deploy`. Expected: PASS.
- [ ] **Step 8: End to end.** In the scratchpad: `alder new demo` inside a directory named `e2e-[x]`, add a table with a generator, `alder migrate`, then `alder test` from inside the app, using `go run ./cmd/alder` from the checkout. Expected: the tests run; no "no such table".
- [ ] **Step 9:** File a GitHub issue (do not fix): `rt: a data directory with glob metacharacters finds no silo databases (rt/db/stores.go:1264, rt/backup/silos.go:84)`, saying a backup would silently skip every silo, and linking #172.
- [ ] **Step 10:** Commit: `migrate, buildkit, deploy: list migrations by reading the directory, not globbing (#172)`.

### Task 3: #175 — E0017 says what to do

**Files:**
- Modify: `internal/names/graph.go:178`, `internal/diag/explain.go:19`
- Modify: `testdata/corpus/E0017-std-module-name/expected.diag`, `testdata/corpus/app-module-named-fuzz/expected.diag`
- Modify: `book/03-syntax.md` (near line 23), `book/11-tooling.md:2525-2528`
- Leave alone: everything under `experiments/` (records of past runs).

- [ ] **Step 1:** Change the two `expected.diag` files to the new message (for `Test`, and for `Fuzz`):

```
app/Test.alder:1:1 E0017 module `Test` is named like the standard library's `Test`, which is always in scope and would hide it; rename the module, its file and its imports (for example to `AppTest`), or use the standard library's `Test` in its place
```

- [ ] **Step 2:** Find the corpus runner (`grep -rn '"corpus"' --include='*_test.go' internal cmd`) and run it for these two cases. Expected: FAIL on the message.
- [ ] **Step 3:** Change the format string at `internal/names/graph.go:178` to produce that message (`%[1]s` for the three uses of the name).
- [ ] **Step 4:** In `internal/diag/explain.go`, append to E0017's text: `An app that compiled before can get this after an upgrade, when the standard library gains a module of that name (as it gained \`Json\` in v0.2.0); CHANGELOG.md lists every such addition.` Add the same point as a third bullet under `#### E0017` in `book/11-tooling.md`, and make `book/03-syntax.md`'s sentence agree with the new message if it quotes the old one.
- [ ] **Step 5:** Run the corpus runner, `go test ./internal/names ./internal/diag` and the book's tests (`grep -rn "book/" --include='*_test.go' -l cmd internal | head` finds them). Expected: PASS.
- [ ] **Step 6: Read it in the real case.** `cd ~/projects/nightvote-replay && go run -C ~/projects/alder ./cmd/alder check "$PWD"` (or the equivalent invocation). Expected: one E0017 for `app/Json.alder` with the new message and no separate errors for its importers. Change nothing in that repository.
- [ ] **Step 7:** Commit: `names: E0017 says to rename the module or use the stdlib's (#175)`.

### Task 4: #175 — a `Text` where a `Json.Value` is expected gets a hint

**Files:**
- Modify: `internal/check/expr.go` (`mismatch`, about line 509)
- Create: `testdata/corpus/E0003-json-value-got-text/` (copy the file layout of `testdata/corpus/E0017-std-module-name/`)
- Modify: `book/09-stdlib.md` (the `## Json` section, line 495 on), `internal/diag/explain.go` only if E0003's text lists the hints

- [ ] **Step 1: Write the corpus case.** One module with two functions: `a = File.json "{\"ok\":true}"` and `b = File.json 3` (in whatever declaration the corpus needs to type-check them). `expected.diag` has two E0003 lines:

```
… E0003 expected Json.Value, got Text; `Json.string` makes a JSON string of a Text; text that is already JSON goes in `File.text { contentType: "application/json", body: … }`
… E0003 expected Json.Value, got Int
```

  Fill in the positions from the first run; the message text is fixed.

- [ ] **Step 2:** Run the corpus runner for this case. Expected: FAIL, the first line lacks the hint.
- [ ] **Step 3:** In `mismatch`, after the `Maybe` hint, append the suffix above when the expected type is the stdlib's `Json.Value` and the given type is `Text`. Compare types, not their printed names.
- [ ] **Step 4:** Run the corpus runner and `go test ./internal/check`. Expected: PASS.
- [ ] **Step 5:** In `book/09-stdlib.md`'s `## Json` section, add two sentences: text that is already JSON is served with `File.text { contentType: "application/json", body: t }`, and there is no `Json.raw` because `Json.encode` never writes text that is not JSON.
- [ ] **Step 6:** Run the book's tests. Expected: PASS.
- [ ] **Step 7:** Commit: `check: a Text where a Json.Value is expected names Json.string and File.text (#175)`.

### Task 5: `CHANGELOG.md`, and release notes taken from it

**Files:**
- Create: `CHANGELOG.md`, `scripts/release-notes.sh`
- Modify: `.github/workflows/release.yml`, `.github/workflows/ci.yml` (the `lint` job, after line 182), `.goreleaser.yaml` (comments only), `book/11-tooling.md` ("Cutting a release", line 2256 on), `README.md` (one line linking the changelog)

**Interfaces:**
- Produces: `scripts/release-notes.sh vX.Y.Z` prints the body of `CHANGELOG.md`'s section whose heading starts with `## vX.Y.Z` (up to the next `## ` heading, without the heading line). It exits 1 with `release-notes: CHANGELOG.md has no section for vX.Y.Z` on stderr when the section is missing or holds only blank lines.

- [ ] **Step 1: Write `CHANGELOG.md`.** Shape:

```markdown
# Changelog

Every release of alder, newest first. "Breaking" lists what stops an app that compiled
with the previous release, each with what to do.

## v0.2.0 — unreleased

### Breaking
### Added
### Fixed
```

  **Breaking** must hold at least these, each with its fix. Check each against the code and the closed issue before writing it:
  - The standard library gained `Json` (#74). An app module named `Json` is E0017: rename it, or use the stdlib's.
  - `File.json` takes a `Json.Value`, not `Text`. Already-encoded text: `File.text { contentType: "application/json", body: t }`.
  - Routes replaced pages (#160 and its follow-ups). The removed syntax is E0046, which names the replacement; there is no migration tool.
  - `click=` on a write route that has `form F` is E0045: use `<form submit={R}>`.
  - Metrics: `PUT`, `PATCH` and `DELETE` have their own method label (7 values, not 4) (#180).
  - An element fetch of a path declared only with other methods answers not-found, not 405.

  Then sweep `git log v0.1.0..HEAD --format='%s'` and the issues closed since 2026-10-17 for other user-visible breaks (removed or renamed stdlib functions, changed CLI flags or output contracts, new error codes that refuse code v0.1.0 accepted) and add them.

  **Added** is headlines only, at most 15 lines: `alder lsp` and the editor requests, `alder tui`, the stdlib modules added since v0.1.0, new commands, new deploy targets. **Fixed** names #172 and #175. End the section with a link: `https://github.com/tapegram/alder/compare/v0.1.0...v0.2.0`.

- [ ] **Step 2:** Run `scripts/release-notes.sh v0.2.0` before the script exists, then write it (POSIX `awk`, `set -eu`, executable). Check:
  - `scripts/release-notes.sh v0.2.0 | head -3` prints `### Breaking` within the first lines, exit 0;
  - `scripts/release-notes.sh v9.9.9` prints the error, exit 1;
  - `scripts/release-notes.sh v0.2` exits 1 (a prefix of a version is not that version: match `v0.2.0` followed by a space or the end of the line).
- [ ] **Step 3:** In `ci.yml`'s `lint` job add a step `changelog (the top section is the release notes)` that runs the script for the first `## v…` heading of `CHANGELOG.md` and discards the output.
- [ ] **Step 4:** In `release.yml`, before the GoReleaser step, add `run: scripts/release-notes.sh "$GITHUB_REF_NAME" > "$RUNNER_TEMP/release-notes.md"`, and change GoReleaser's `args` to `release --clean --release-notes=${{ runner.temp }}/release-notes.md`. Leave `.goreleaser.yaml`'s `changelog:` block as it is (disabling it would also skip loading the notes file); change only the comments that say the notes come from the commits. Update the header comment of `release.yml` the same way.
- [ ] **Step 5:** Run `goreleaser check`. Expected: no errors. Run `goreleaser release --snapshot --clean --skip=docker` and then `dist/alder_darwin_arm64*/alder help --json | grep -c '"lsp"'`. Expected: `1`. Remove `dist/`.
- [ ] **Step 6:** Update `book/11-tooling.md` "Cutting a release": the notes are the tag's section of `CHANGELOG.md`; before tagging, rename `## vX.Y.Z — unreleased` to the date; a tag without a section fails the release job before anything is published. Add the changelog link to `README.md` near the install lines.
- [ ] **Step 7:** Run the book's tests and `python3 book/site/test_site.py`. Expected: PASS.
- [ ] **Step 8:** Commit: `release: CHANGELOG.md, and the release notes are its section for the tag (#175, #174)`.

### Task 6: Release v0.2.0 (#174) — main session, not a subagent

- [ ] **Step 1:** Dispatch the final review (Opus) of `04595cb0..HEAD`. Fix what it finds.
- [ ] **Step 2:** In `CHANGELOG.md` set the heading to `## v0.2.0 — <today's date in New York>`. In `book/01-why.md` (lines 125, 130, 155–159) and `book/11-tooling.md:2237`, change the example version `0.1.0` to `0.2.0`. Commit: `release: v0.2.0`.
- [ ] **Step 3:** Run `scripts/test.sh`. Expected: every part passes. Push `main`. Start the full CI with the mutation gates: `gh workflow run CI --ref main`; watch it once with `gh run watch <id> --exit-status` under a timeout. Expected: every job green, including `mutation`.
- [ ] **Step 4: Stop and ask the user.** Show the CHANGELOG section and the green run, and ask for the go to push the tag. The tag publishes a GitHub release, a Homebrew formula and a container image; none can be cleanly taken back.
- [ ] **Step 5:** `git tag -a v0.2.0 -m "alder v0.2.0" && git push origin v0.2.0`. Watch the `release` run once.
- [ ] **Step 6: Verify.**
  - `gh release view v0.2.0 --json body -q .body | head -5` starts with `### Breaking`. If it holds commit subjects instead, run `gh release edit v0.2.0 --notes-file <(scripts/release-notes.sh v0.2.0)` and file an issue about the workflow.
  - `gh api repos/tapegram/homebrew-alder/contents/Formula/alder.rb -q .content | base64 -d | grep version` shows `0.2.0`.
  - `brew update && brew upgrade tapegram/alder/alder`, then `$(brew --prefix)/bin/alder version` prints `alder v0.2.0 (…)` and `$(brew --prefix)/bin/alder help --json | grep -c '"lsp"'` prints `1`.
- [ ] **Step 7: Follow through.**
  - In `~/projects/alder-vscode` (on `main`): the README's install section says `brew install tapegram/alder/alder` or `brew upgrade alder` (v0.2.0 or later) in place of building from a checkout. Run its tests, commit, push. Ask the user before publishing a new extension version.
  - Close #172, #175 and #174 with comments naming the commits and the release. On #175, say why there is no `Json.raw`.
  - Tick item 4 in #52.
  - `go install ./cmd/alder`, fast-forward `~/projects/alder.nvim`.
  - Update memory (`alder-next-36-34`), then recommend a fresh session: the next item is #52 item 5 (#208).
