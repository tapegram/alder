# M17 — File answers, Bytes, delays, Time arithmetic Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An Alder page whose `view` returns `File` answers any body with its own content type, status and allow-listed headers, written verbatim; `Bytes` is a built-in type capabilities return; `Query.delay`/`Task.sleep` are built-in effects that tests run on virtual time; `Time` gains millisecond arithmetic and `Float` gains `sqrt/exp/log/pow`; `Test.fetch` reads a file page in a test.

**Architecture:** `File` and `Bytes` are opaque Prelude types (`rt.File`, `[]byte` in Go). The checker marks a page module a *file page* when `view`'s declared result is `File` (`PageInfo.File`, `ir.Page.File`); gen emits a GET route returning `web.FileResponse`, a `web.Route{File: true}`, and `data-alder-reload` on literal links to it. The router writes a raw response before its HTML path (no doctype, head tags, panel or morph), answers a Datastar GET of a file route with the navigate event, and never follows a redirect into one. `Query.delay`/`Task.sleep` are ordinary std calls (like `Time.now`) that call a new `eff.Env.Sleep`; the test environment's clock (`rtesting.Clock`) advances instead of waiting. `Test.fetch` is a `Test.get` variant that returns the raw response.

**Tech Stack:** Go (compiler `internal/…`, runtime `rt/…`), Alder stdlib signatures in `stdlib/*.alder`, Playwright smoke (`go test -tags browser`), Python reference-prompt builder (`experiments/build_context.py`).

**Spec:** `docs/superpowers/specs/2026-10-20-m17-file-answers-design.md` (read it before any task; this plan argues from it).

## Global Constraints

- Branch `m17-file-answers` in `~/projects/alder`. Commit per task, subject `area: lowercase summary` (e.g. `rt: File and Bytes runtime types`), ending with the attribution lines the session reminder gives.
- Gate at the end of every task: `gofmt -l $(git ls-files '*.go')` empty, `go vet ./...` clean, the task's scoped tests green. Gate at the end of the plan (Task 12): `scripts/test.sh` green, `go vet -tags browser ./...`, `go test ./book`, `cd experiments && python3 -m unittest -v`.
- Every new file under `rt/` is added to the explicit `//go:embed` list in `rt/embed.go` (`TestFiles_Complete` in `rt/embed_test.go` compares it with `git ls-files rt`; `git add` the file before running it).
- Go mapping: `Bytes` ↔ `[]byte`, `File` ↔ `rt.File`; `Int` ↔ `int64`, `Float` ↔ `float64`, `Text` ↔ `string`, `Unit` ↔ `struct{}`, `Time` ↔ `time.Time`, `Maybe a` ↔ `rt.Maybe[A]` (`rt.Just`, `rt.Nothing`).
- `rt.File` is exactly: `type File struct { Status int; ContentType string; Headers [][2]string; Body []byte }` — `Status` 0 means 200, `ContentType` "" means `application/octet-stream`.
- Header allow-list (case-insensitive, compared after `textproto.CanonicalMIMEHeaderKey`): `Content-Disposition`, `Cache-Control`, `Etag` (canonical form of `ETag`), `Last-Modified`, `Retry-After`, `Content-Language`, and any name starting `X-`. A literal name outside it is **E0035**; at run time it is dropped with a `warn` log line.
- **Deviation from spec §3.1, recorded for the user:** the status range is **200..599**, not 100..599. Go's `WriteHeader` treats 1xx as informational and then sends an implicit 200 with the body, so a 1xx file answer cannot be written. A literal outside 200..599 is E0003; at run time the router answers the app's 500 page.
- Trace lines: `Query.delay <n>ms` / `Task.sleep <n>ms`, `n` after clamping to 0..60000.
- Diagnostics: **E0034** "a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the message to an HTML page"; **E0035** "`<Name>` cannot be set from a page; the headers a file may set are `Content-Disposition`, `Cache-Control`, `ETag`, `Last-Modified`, `Retry-After`, `Content-Language` and `X-…` names". Both go in `internal/diag/diag.go`, `internal/diag/explain.go` (≥ 80 chars) and the `codes` list of `TestExplainCoversEveryCode` (`internal/diag/diag_test.go:81`), and in book ch. 11's table.
- `404`, `500`, `503` (maintenance) and the `requires` redirect stay the app's HTML answers on a file page.
- Reference prompt: `python3 experiments/build_context.py --reference --size` measures **47,984** today (the spec's "45.8k" is stale; no test enforces a cap). M17's book text that reaches the reference must stay under about 500 tokens: put the ch. 6 detail in `###` subsections listed in `DROP_SUBSECTIONS`. Report the final number in Task 11's hand-back; the user decides whether to trim or raise the cap.
- Non-goals (spec §7): no `@key`, no streaming/ranged/compressed responses, no byte operations, no uploads, no POST to a file page, no scheduler.
- No agent or background work 9am–5pm America/New_York on weekdays.

## Review Focus

1. **Header names in any case, and header values with CR/LF.** `File.withHeader { name: "content-disposition", … }` must pass the checker and be sent; a value holding `"\r\nSet-Cookie: x=1"` must not inject a header. Pinned in Task 4 (`TestFile_HeaderCaseAndNewlines`) and Task 5 (corpus `E0035-file-header` includes a lowercase allowed name that is *not* reported).
2. **`File.withStatus 204` or `304` with a body.** Go refuses to write a body for these (`http.ErrBodyNotAllowed`). Expected: the status is sent, the body dropped, no 500 and no error log. Pinned in Task 4 (`TestFile_NoBodyStatuses`).
3. **A Datastar GET of a file route** (a boosted click on a link built from a `Page` *variable*, which gets no `data-alder-reload`). Expected: the navigate event to the same URL, and `load` does not run (no wasted query, no double trace). Pinned in Task 4 (`TestFile_DatastarGetNavigates` counts handler calls).
4. **A redirect into a file page under Datastar** (an HTML page's `update` answers `Redirect (Report.Csv {…})`). Expected: the navigate event (`Assign`), never a morph of the file into the page. Pinned in Task 4 (`TestFile_RedirectIntoFileNavigates`).
5. **`Test.fetch` on a file page whose `load` fails or whose `requires` fails.** Expected: `status: 404` with the app's HTML not-found body; `status: 303` with a `Location` header (not followed). Pinned in Task 7 (corpus `file-ok` tests "fetch a missing report is 404" and "fetch while logged out is 303").

---

## File map

- Create `rt/file.go` (`rt.File`, `FileHeaderAllowed`), `rt/std/file.go` (File/Bytes std functions), `stdlib/File.alder`, `stdlib/Bytes.alder`, `testdata/corpus/E0034-file-page-msg/`, `testdata/corpus/E0035-file-header/`, `testdata/corpus/E0003-file-status/`, `testdata/corpus/file-ok/`, `internal/generate/templates/page_file.tmpl`, `rt/web/file_test.go`, `rt/std/file_test.go`.
- Modify `stdlib/Prelude.alder`, `stdlib/Text.alder`, `stdlib/Time.alder`, `stdlib/Float.alder`, `stdlib/Query.alder`, `stdlib/Task.alder`, `stdlib/Test.alder`; `internal/types/types.go`; `internal/check/{check.go,decls.go,pages.go,hookdoc.go,expr.go,browser.go,result.go}`; `internal/names/resolve.go`; `internal/diag/{diag.go,explain.go,diag_test.go}`; `internal/ir/{ir.go,lower.go,lower_eff.go}` (and wherever `ir.Page` is built); `internal/gen/{types.go,stdcall.go,routes.go,html.go,browser.go,gen_test.go}`; `rt/embed.go`, `rt/show.go`; `rt/std/{text.go,date.go,num.go,effects.go,std_test.go}`; `rt/eff/eff.go`; `rt/web/{request.go,router.go}`; `rt/testing/{testing.go,browser.go}`; `internal/generate/{specs.go,page.go,msg.go,form.go}`; `cmd/alder/{generate.go,generate_test.go,test_test.go,browser_test.go}`, `cmd/alder/testdata/browser/{Smoke.alder,smoke.cjs}`, `testdata/help.json`; book chapters 6, 7, 9, 10, 11, 12; `experiments/build_context.py`, `experiments/e11-cli/prompt-reference.md`.

---

### Task 1: `File` and `Bytes` — types, stdlib surface, runtime values

**Files:**
- Create: `rt/file.go`, `rt/std/file.go`, `rt/std/file_test.go`, `stdlib/File.alder`, `stdlib/Bytes.alder`
- Modify: `stdlib/Prelude.alder:3-23`, `stdlib/Text.alder`, `internal/types/types.go:77-86`, `internal/check/check.go:292` (`declareStd`), `internal/check/decls.go:~903` (`preludeReserved`), `internal/names/resolve.go:13` (`preludeTypes`), `internal/diag/explain.go` (E0012 text), `internal/gen/types.go:14-62` (`GoType` + doc comment), `internal/gen/stdcall.go:17` (`StdCalls`), `internal/gen/gen_test.go:517` (`TestGoType`), `rt/embed.go:17-27`, `rt/show.go:159`, `rt/std/text.go`

**Interfaces:**
- Produces (Alder): `type File`, `type Bytes` (Prelude); `File.text : { contentType: Text, body: Text } -> File`, `File.bytes : { contentType: Text, body: Bytes } -> File`, `File.html : Html Never -> File`, `File.json : Text -> File`, `File.withStatus : Int -> File -> File`, `File.withHeader : { name: Text, value: Text } -> File -> File`, `File.status : File -> Int`, `File.contentType : File -> Text`, `File.body : File -> Bytes`, `Bytes.length : Bytes -> Int`, `Bytes.toText : Bytes -> Maybe Text`, `Text.toBytes : Text -> Bytes`.
- Produces (Go): `rt.File` (Global Constraints), `rt.FileHeaderAllowed(name string) (canonical string, ok bool)`, `types.File`, `types.Bytes` (`*types.Con` singletons), `std.FileText(r struct{ ContentType string; Body string }) rt.File`, `std.FileBytes(r struct{ ContentType string; Body []byte }) rt.File`, `std.FileHtml(n html.Node) rt.File`, `std.FileJson(s string) rt.File`, `std.FileWithStatus(code int64, f rt.File) rt.File`, `std.FileWithHeader(h struct{ Name string; Value string }, f rt.File) rt.File`, `std.FileStatus(f rt.File) int64`, `std.FileContentType(f rt.File) string`, `std.FileBody(f rt.File) []byte`, `std.BytesLength(b []byte) int64`, `std.BytesToText(b []byte) rt.Maybe[string]`, `std.TextToBytes(s string) []byte`.

- [ ] **Step 1: Write the failing runtime tests**

`rt/std/file_test.go`:

```go
package std

import (
	"testing"

	"alder/rt"
	"alder/rt/html"
)

func TestFile_Constructors(t *testing.T) {
	f := FileText(struct{ ContentType string; Body string }{"text/csv", "a,b\n"})
	if FileStatus(f) != 200 || FileContentType(f) != "text/csv" || string(FileBody(f)) != "a,b\n" {
		t.Fatalf("text: %+v", f)
	}
	j := FileJson(`{"a":1}`)
	if FileContentType(j) != "application/json" || string(FileBody(j)) != `{"a":1}` {
		t.Fatalf("json: %+v", j)
	}
	b := FileBytes(struct{ ContentType string; Body []byte }{"", []byte{0, 1}})
	if FileContentType(b) != "application/octet-stream" || BytesLength(FileBody(b)) != 2 {
		t.Fatalf("bytes: %+v", b)
	}
	h := FileHtml(html.Raw("<!doctype html>\n<p>x</p>"))
	if FileContentType(h) != "text/html; charset=utf-8" || string(FileBody(h)) != "<!doctype html>\n<p>x</p>" {
		t.Fatalf("html: %+v", h)
	}
}

func TestFile_StatusAndHeaders(t *testing.T) {
	f := FileWithHeader(struct{ Name string; Value string }{"Retry-After", "20"},
		FileWithStatus(503, FileText(struct{ ContentType string; Body string }{"text/plain", "busy"})))
	if FileStatus(f) != 503 || len(f.Headers) != 1 || f.Headers[0] != [2]string{"Retry-After", "20"} {
		t.Fatalf("%+v", f)
	}
	// headers are kept as given; the router filters them (rt/web)
	g := FileWithHeader(struct{ Name string; Value string }{"Set-Cookie", "x=1"}, f)
	if len(g.Headers) != 2 || len(f.Headers) != 1 {
		t.Fatalf("withHeader must not alias: %+v %+v", f, g)
	}
}

func TestBytes(t *testing.T) {
	if BytesLength(TextToBytes("héllo")) != 6 {
		t.Fatal("length counts bytes")
	}
	if got := BytesToText(TextToBytes("héllo")); got != rt.Just("héllo") {
		t.Fatalf("%v", got)
	}
	if got := BytesToText([]byte{0xff, 0xfe}); got.Ok {
		t.Fatalf("invalid UTF-8 must be Nothing, got %v", got)
	}
	if !rt.Equal(TextToBytes("ab"), []byte("ab")) || rt.Equal(TextToBytes("ab"), []byte("ac")) {
		t.Fatal("Bytes compares by content")
	}
	if !rt.Equal(FileJson("1"), FileJson("1")) || rt.Equal(FileJson("1"), FileWithStatus(500, FileJson("1"))) {
		t.Fatal("File compares by status, type, headers, body")
	}
}

func TestFileHeaderAllowed(t *testing.T) {
	for _, n := range []string{"Content-Disposition", "content-disposition", "ETag", "Cache-Control", "Last-Modified", "Retry-After", "Content-Language", "X-Replay", "x-anything"} {
		if _, ok := rt.FileHeaderAllowed(n); !ok {
			t.Errorf("%s should be allowed", n)
		}
	}
	for _, n := range []string{"Set-Cookie", "Location", "Content-Length", "Transfer-Encoding", "Content-Type", "Connection", "X", "bad name", ""} {
		if _, ok := rt.FileHeaderAllowed(n); ok {
			t.Errorf("%s should be refused", n)
		}
	}
}
```

Before writing, confirm the helper names in `rt`: the equality entry point (`grep -n '^func Equal' rt/core.go`) and the raw-HTML node constructor (`grep -n '^func Raw\|^func Unsafe' rt/html/*.go`) — use the names found (the test above assumes `rt.Equal` and `html.Raw`).

- [ ] **Step 2: Run to see it fail**

Run: `go test ./rt/std -run 'TestFile|TestBytes' -count=1`
Expected: FAIL to compile (`undefined: FileText`, `rt.FileHeaderAllowed`).

- [ ] **Step 3: Write `rt/file.go`**

```go
package rt

import (
	"net/textproto"
	"strings"
)

// File is a page's answer when its view returns File (chapter 6, "File pages"): written as
// given, with no doctype, head tags or panel. Status 0 means 200; ContentType "" means
// application/octet-stream. Headers keep what the page set, in order; the router sends only
// the names FileHeaderAllowed accepts.
type File struct {
	Status      int
	ContentType string
	Headers     [][2]string
	Body        []byte
}

var fileHeaders = map[string]bool{
	"Content-Disposition": true, "Cache-Control": true, "Etag": true,
	"Last-Modified": true, "Retry-After": true, "Content-Language": true,
}

// FileHeaderAllowed reports whether a file page may set header name, and its canonical form.
// The checker (E0035) and the router share it.
func FileHeaderAllowed(name string) (string, bool) {
	if name == "" || strings.ContainsAny(name, " \t\r\n:") {
		return "", false
	}
	c := textproto.CanonicalMIMEHeaderKey(name)
	return c, fileHeaders[c] || (strings.HasPrefix(c, "X-") && len(c) > 2)
}
```

Add `file.go` to the `//go:embed` list in `rt/embed.go` beside `core.go`.

- [ ] **Step 4: Write `rt/std/file.go` and `Text.toBytes`**

```go
package std

import (
	"unicode/utf8"

	"alder/rt"
	"alder/rt/html"
)

// FileText is File.text: a 200 answer of contentType with body as UTF-8.
func FileText(r struct{ ContentType string; Body string }) rt.File {
	return rt.File{Status: 200, ContentType: r.ContentType, Body: []byte(r.Body)}
}

// FileBytes is File.bytes.
func FileBytes(r struct{ ContentType string; Body []byte }) rt.File {
	return rt.File{Status: 200, ContentType: r.ContentType, Body: r.Body}
}

// FileHtml is File.html: the node rendered as view would render it, written as-is.
func FileHtml(n html.Node) rt.File {
	return rt.File{Status: 200, ContentType: "text/html; charset=utf-8", Body: []byte(html.Render(n))}
}

// FileJson is File.json.
func FileJson(s string) rt.File {
	return rt.File{Status: 200, ContentType: "application/json", Body: []byte(s)}
}

// FileWithStatus is File.withStatus; a code outside 200..599 is answered as the 500 page by
// the router (a literal one is E0003).
func FileWithStatus(code int64, f rt.File) rt.File { f.Status = int(code); return f }

// FileWithHeader is File.withHeader: appended, never aliasing f's slice.
func FileWithHeader(h struct{ Name string; Value string }, f rt.File) rt.File {
	hs := make([][2]string, len(f.Headers), len(f.Headers)+1)
	copy(hs, f.Headers)
	f.Headers = append(hs, [2]string{h.Name, h.Value})
	return f
}

func FileStatus(f rt.File) int64 {
	if f.Status == 0 {
		return 200
	}
	return int64(f.Status)
}

func FileContentType(f rt.File) string {
	if f.ContentType == "" {
		return "application/octet-stream"
	}
	return f.ContentType
}

func FileBody(f rt.File) []byte { return f.Body }

// BytesLength counts bytes, not code points (Text.length counts those).
func BytesLength(b []byte) int64 { return int64(len(b)) }

// BytesToText is Just the text when b is valid UTF-8.
func BytesToText(b []byte) rt.Maybe[string] {
	if !utf8.Valid(b) {
		return rt.Nothing[string]()
	}
	return rt.Just(string(b))
}
```

In `rt/std/text.go` add `// TextToBytes is the text's UTF-8 bytes.` `func TextToBytes(s string) []byte { return []byte(s) }`.

Anonymous-record argument structs list fields in the order the `.alder` signature declares them (`GoType`, `internal/gen/types.go:91-102`): the signatures below declare `contentType` before `body` and `name` before `value`, matching the Go above. `FileBytes` with `ContentType: ""` keeps "" (the getter answers `application/octet-stream`), so the test's `FileContentType(b)` check holds.

- [ ] **Step 5: Run the runtime tests**

Run: `go test ./rt/std -run 'TestFile|TestBytes' -count=1`
Expected: PASS. If the `File` equality check fails because `nil` and empty `Headers` differ, add `AlderEqual` to `rt.File` comparing `Status`/`ContentType`/`Body` with `bytes.Equal` and headers element-wise (mirroring `rt.Page`'s method at `rt/core.go:404`).

- [ ] **Step 6: Declare the Alder surface**

`stdlib/Prelude.alder`, after `type Page`: `type File` and `type Bytes` (each with a one-line `--` doc comment: the LSP hover reads it).

`stdlib/File.alder`:

```alder
module File

-- a 200 answer with the text as its body
text : { contentType: Text, body: Text } -> File
-- a 200 answer with the bytes as its body
bytes : { contentType: Text, body: Bytes } -> File
-- text/html; charset=utf-8, rendered verbatim: no doctype unless the node has one
html : Html Never -> File
-- application/json
json : Text -> File
-- 200..599
withStatus : Int -> File -> File
-- Content-Disposition, Cache-Control, ETag, Last-Modified, Retry-After, Content-Language, X-…
withHeader : { name: Text, value: Text } -> File -> File
status : File -> Int
contentType : File -> Text
body : File -> Bytes
```

`stdlib/Bytes.alder`:

```alder
module Bytes

-- the number of bytes
length : Bytes -> Int
-- Just when the bytes are valid UTF-8
toText : Bytes -> Maybe Text
```

`stdlib/Text.alder`: `toBytes : Text -> Bytes` (doc: `-- the text's UTF-8 bytes`).

- [ ] **Step 7: Register the types in the compiler**

- `internal/types/types.go`: add `File = &Con{Name: "File"}` and `Bytes = &Con{Name: "Bytes"}` to the singleton `var` block.
- `internal/check/check.go` `declareStd`: add `types.File`, `types.Bytes` to the pre-registered list.
- `internal/check/decls.go` `preludeReserved`: add `"File": true, "Bytes": true`.
- `internal/names/resolve.go` `preludeTypes`: add `"File"`, `"Bytes"`.
- `internal/diag/explain.go` E0012: add `` `File`, `Bytes` `` to the list of reserved names.
- `internal/gen/types.go` `GoType`, `case *types.Con`: `case "Bytes": return "[]byte"` and `case "File": return "rt.File"`; add both to the mapping in the doc comment above `GoType`.
- `internal/gen/stdcall.go` `StdCalls`, a `// File, Bytes` group: `"File.text": "std.FileText"`, `"File.bytes": "std.FileBytes"`, `"File.html": "std.FileHtml"`, `"File.json": "std.FileJson"`, `"File.withStatus": "std.FileWithStatus"`, `"File.withHeader": "std.FileWithHeader"`, `"File.status": "std.FileStatus"`, `"File.contentType": "std.FileContentType"`, `"File.body": "std.FileBody"`, `"Bytes.length": "std.BytesLength"`, `"Bytes.toText": "std.BytesToText"`, and under Text `"Text.toBytes": "std.TextToBytes"`.
- `internal/gen/gen_test.go` `TestGoType`: add rows `{types.Bytes, "[]byte"}`, `{types.File, "rt.File"}` in the table's existing shape.

- [ ] **Step 8: Show `Bytes` readably in test failures**

`rt/show.go:159`, before the generic `reflect.Slice` case: when `v.Type() == reflect.TypeFor[[]byte]()`, show `Text.toBytes "<text>"` (quoted the way `show` quotes `Text`) when the bytes are valid UTF-8, else `<N bytes>`. Add a row to the show test table (`grep -n 'func TestShow' rt/*_test.go`): `[]byte("hi")` → `Text.toBytes "hi"`, `[]byte{0xff}` → `<1 bytes>`.

- [ ] **Step 9: Run the compiler and runtime tests**

Run: `go test ./rt/... ./internal/gen ./internal/check ./internal/names ./internal/diag -count=1`
Expected: PASS, including `TestStdCallsCoverStdlib`, `TestStdCallsExist`, `TestFiles_Complete` (`git add rt/file.go rt/std/file.go` first), `TestCorpus` (no corpus file changes).

- [ ] **Step 10: Commit**

```bash
git add rt/file.go rt/std/file.go rt/std/file_test.go rt/std/text.go rt/embed.go rt/show.go stdlib internal
git commit -m "stdlib: File and Bytes types, File/Bytes modules, Text.toBytes"
```

---

### Task 2: `Time` milliseconds and `Float` math

**Files:**
- Modify: `stdlib/Time.alder`, `stdlib/Float.alder`, `rt/std/date.go` (beside `TimeEpoch`, line 38), `rt/std/num.go`, `internal/gen/stdcall.go`, `rt/std/std_test.go`

**Interfaces:**
- Produces: `Time.toMillis : Time -> Int`, `Time.fromMillis : Int -> Time`, `Time.addMillis : Int -> Time -> Time`, `Time.diffMillis : Time -> Time -> Int` (`b − a`); `Float.sqrt`, `Float.exp`, `Float.log : Float -> Float`, `Float.pow : Float -> Float -> Float`. Go: `std.TimeToMillis(t time.Time) int64`, `std.TimeFromMillis(ms int64) time.Time`, `std.TimeAddMillis(ms int64, t time.Time) time.Time`, `std.TimeDiffMillis(a, b time.Time) int64`, `std.FloatSqrt/FloatExp/FloatLog(f float64) float64`, `std.FloatPow(base, exp float64) float64`.

- [ ] **Step 1: Write the failing test** (append to `rt/std/std_test.go`)

```go
func TestTimeMillis(t *testing.T) {
	if TimeToMillis(TimeEpoch()) != 0 {
		t.Fatal("epoch is 0")
	}
	x := time.Date(2026, 11, 3, 20, 0, 0, 123_456_789, time.UTC)
	ms := TimeToMillis(x)
	if !TimeFromMillis(ms).Equal(x.Truncate(time.Millisecond)) || TimeToMillis(TimeFromMillis(ms)) != ms {
		t.Fatal("round trip on what toMillis gives")
	}
	if TimeDiffMillis(x, TimeAddMillis(1500, x)) != 1500 || TimeDiffMillis(TimeAddMillis(1500, x), x) != -1500 {
		t.Fatal("diffMillis a b = b - a")
	}
	if TimeFromMillis(-1000).Location() != time.UTC || TimeToMillis(TimeFromMillis(-1000)) != -1000 {
		t.Fatal("before the epoch, UTC")
	}
}

func TestFloatMath(t *testing.T) {
	if FloatSqrt(9) != 3 || FloatPow(2, 10) != 1024 || math.Abs(FloatLog(FloatExp(1))-1) > 1e-12 {
		t.Fatal("sqrt/pow/log/exp")
	}
}
```

- [ ] **Step 2: Run to see it fail**

Run: `go test ./rt/std -run 'TestTimeMillis|TestFloatMath' -count=1`
Expected: FAIL to compile (`undefined: TimeToMillis`).

- [ ] **Step 3: Implement**

`rt/std/date.go`:

```go
// TimeToMillis is milliseconds since Time.epoch.
func TimeToMillis(t time.Time) int64 { return t.UnixMilli() }

// TimeFromMillis is the UTC instant ms milliseconds after Time.epoch.
func TimeFromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// TimeAddMillis is t moved by ms milliseconds.
func TimeAddMillis(ms int64, t time.Time) time.Time { return t.Add(time.Duration(ms) * time.Millisecond) }

// TimeDiffMillis is `Time.diffMillis a b`: b − a in whole milliseconds, like Date.diffDays.
func TimeDiffMillis(a, b time.Time) int64 { return b.Sub(a).Milliseconds() }
```

`rt/std/num.go`: `func FloatSqrt(f float64) float64 { return math.Sqrt(f) }`, `FloatExp` (`math.Exp`), `FloatLog` (`math.Log`, natural), `func FloatPow(base, exp float64) float64 { return math.Pow(base, exp) }`, each with a one-line doc comment (`FloatLog` says "natural logarithm; log of a negative is NaN").

`stdlib/Time.alder` appends the four signatures (spec §3.5) with `--` comments; `stdlib/Float.alder` the four (`pow : Float -> Float -> Float -- Float.pow base exponent`). `StdCalls` gains the eight entries (`"Time.toMillis": "std.TimeToMillis"`, …, `"Float.pow": "std.FloatPow"`).

- [ ] **Step 4: Run**

Run: `go test ./rt/std ./internal/gen -run 'TestTimeMillis|TestFloatMath|TestStdCalls' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add stdlib/Time.alder stdlib/Float.alder rt/std/date.go rt/std/num.go rt/std/std_test.go internal/gen/stdcall.go
git commit -m "stdlib: Time millisecond arithmetic and Float sqrt/exp/log/pow"
```

---

### Task 3: `Query.delay` / `Task.sleep` and the virtual test clock

**Files:**
- Modify: `rt/eff/eff.go:164-204` (`Env`), `rt/std/effects.go`, `stdlib/Query.alder`, `stdlib/Task.alder`, `internal/gen/stdcall.go` (`StdCalls`, `isEffectful`), `internal/ir/lower_eff.go:37` (`effHead`), `rt/testing/testing.go:266-280, 386`, `rt/testing/browser.go:144`, `rt/std/std_test.go`, `rt/testing/` test file beside `Ticking`'s tests (`grep -n Ticking rt/testing/*_test.go`)

**Interfaces:**
- Consumes: nothing from Tasks 1–2.
- Produces: `eff.Env.Sleep func(time.Duration)` (nil = real sleep bounded by `Ctx`); `std.QueryDelay(ms int64) eff.Query[struct{}]`, `std.TaskSleep(ms int64) eff.Task[struct{}]`; `rtesting.Clock` with `NewClock(start time.Time) *Clock`, `(*Clock).Now() time.Time`, `(*Clock).Advance(d time.Duration)`; `Ticking(start)` kept as `NewClock(start).Now`.

- [ ] **Step 1: Write the failing tests**

`rt/std/std_test.go`:

```go
func TestQueryDelay_StubbedSleep(t *testing.T) {
	var slept []time.Duration
	env := &eff.Env{Sleep: func(d time.Duration) { slept = append(slept, d) }}
	for _, ms := range []int64{20000, -5, 90000} {
		if _, err := eff.RunQuery(env, QueryDelay(ms)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := eff.Run(env, TaskSleep(250)); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{20 * time.Second, 0, time.Minute, 250 * time.Millisecond}
	if !reflect.DeepEqual(slept, want) {
		t.Fatalf("slept %v, want %v (clamped to 0..60000 ms)", slept, want)
	}
	wantTrace := []string{"Query.delay 20000ms", "Query.delay 0ms", "Query.delay 60000ms", "Task.sleep 250ms"}
	if !reflect.DeepEqual(env.Trace, wantTrace) {
		t.Fatalf("trace %q", env.Trace)
	}
}

func TestQueryDelay_RealSleepStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := eff.RunQuery(&eff.Env{Ctx: ctx}, QueryDelay(60000))
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("a cancelled request stops the wait: err %v after %v", err, time.Since(start))
	}
}
```

Confirm the runner names first (`grep -n '^func Run' rt/eff/*.go`): the test assumes `eff.RunQuery(env, q)` and `eff.Run(env, task)`; use what exists.

Clock test (in the `rt/testing` test file that covers `Ticking`):

```go
func TestClock_AdvanceMovesNow(t *testing.T) {
	start := time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC)
	c := NewClock(start)
	if !c.Now().Equal(start) {
		t.Fatal("first reading is start")
	}
	c.Advance(20 * time.Second)
	if got := c.Now(); !got.Equal(start.Add(21 * time.Second)) { // +1s tick, +20s sleep
		t.Fatalf("got %v", got)
	}
}
```

- [ ] **Step 2: Run to see them fail**

Run: `go test ./rt/std ./rt/testing -run 'TestQueryDelay|TestClock' -count=1`
Expected: FAIL to compile (`unknown field Sleep`, `undefined: QueryDelay`, `undefined: NewClock`).

- [ ] **Step 3: Add `Env.Sleep` and the effects**

`rt/eff/eff.go`, after `Now`:

```go
	// Sleep waits d for Query.delay and Task.sleep; nil means time.Sleep bounded by Ctx. The
	// test environment advances its clock instead, so a slow page is tested in milliseconds.
	Sleep func(time.Duration)
```

`rt/std/effects.go`, beside `TimeNow`:

```go
// QueryDelay is Query.delay: wait ms milliseconds (clamped to 0..60000) through Env.Sleep.
func QueryDelay(ms int64) eff.Query[struct{}] { return eff.Q(delay("Query.delay", ms)) }

// TaskSleep is Task.sleep: a request-scoped wait, not a scheduler.
func TaskSleep(ms int64) eff.Task[struct{}] { return delay("Task.sleep", ms) }

func delay(name string, ms int64) eff.Task[struct{}] {
	ms = min(max(ms, 0), 60000)
	d := time.Duration(ms) * time.Millisecond
	return eff.Call(fmt.Sprintf("%s %dms", name, ms), false, func(e *eff.Env) (struct{}, error) {
		if e.Sleep != nil {
			e.Sleep(d)
			return struct{}{}, nil
		}
		ctx := e.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
			return struct{}{}, nil
		case <-ctx.Done():
			return struct{}{}, ctx.Err()
		}
	})
}
```

- [ ] **Step 4: Declare and route them as std calls**

`stdlib/Query.alder`: `-- wait ms milliseconds (0..60000); in tests the clock moves instead` `delay : Int -> Query Unit`. `stdlib/Task.alder`: `sleep : Int -> Task Unit` with the same comment.

They are ordinary calls in the IR, like `Time.now`:
- `internal/gen/stdcall.go` `StdCalls`, effects group: `"Query.delay": "std.QueryDelay"`, `"Task.sleep": "std.TaskSleep"`.
- `isEffectful`: a key in `StdCalls` is never effectful:

```go
func isEffectful(module, name string) bool {
	if _, ok := StdCalls[module+"."+name]; ok {
		return false // Query.delay, Task.sleep: calls, not effect nodes
	}
	return Effectful[module] || Effectful[module+"."+name]
}
```

  and update `Effectful`'s doc comment ("… except the StdCalls entries `Query.delay` and `Task.sleep`").
- `internal/ir/lower_eff.go` `effHead`, the `case "Query", "Task", "Db":` arm: return nil for `Query.delay` and `Task.sleep`:

```go
			case "Query", "Task", "Db":
				if x.Ref.Name == "delay" && x.Ref.Module == "Query" || x.Ref.Name == "sleep" && x.Ref.Module == "Task" {
					return nil // a std call (std.QueryDelay), like Time.now
				}
```

- [ ] **Step 5: The virtual clock**

`rt/testing/testing.go`, replace `Ticking` (266-280):

```go
// Clock is a test's time: each reading advances it a second, and a delay (Env.Sleep)
// advances it by the delay at once.
type Clock struct {
	mu   sync.Mutex
	next time.Time
}

func NewClock(start time.Time) *Clock { return &Clock{next: start} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.next
	c.next = c.next.Add(time.Second)
	return t
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next = c.next.Add(d)
}

// Ticking is a clock's Now: each reading advances it a second.
func Ticking(start time.Time) func() time.Time { return NewClock(start).Now }
```

At line 386: `clock := NewClock(now)` and `env := &eff.Env{…, Now: clock.Now, Sleep: clock.Advance, …}`.
`rt/testing/browser.go:144` `EnvFor`: add `Sleep: b.env.Sleep` to the derived `eff.Env`.
Leave `cmd/alder/scratchmain.go:111` (eval/repl) and `rt/web/router.go:493-502` (production) without `Sleep`: they sleep for real.

- [ ] **Step 6: Run**

Run: `go test ./rt/std ./rt/testing ./rt/eff ./internal/gen ./internal/ir -count=1`
Expected: PASS (`TestStdCallsCoverStdlib` accepts the two names in `StdCalls` only).

- [ ] **Step 7: Commit**

```bash
git add rt/eff/eff.go rt/std/effects.go rt/std/std_test.go rt/testing stdlib/Query.alder stdlib/Task.alder internal/gen/stdcall.go internal/ir/lower_eff.go
git commit -m "effects: Query.delay and Task.sleep through Env.Sleep; tests advance the clock"
```

---

### Task 4: The router writes file answers

**Files:**
- Create: `rt/web/file_test.go`
- Modify: `rt/web/request.go:15-19, 45-88`, `rt/web/router.go:163-167 (route), 444-526 (serve), 659-742 (write), 685-709 + 790-797 (redirects, followable)`

**Interfaces:**
- Consumes: `rt.File`, `rt.FileHeaderAllowed` (Task 1).
- Produces: `web.Route.File bool`; `web.Response.Raw *rt.File`; `web.FileResponse(f rt.File, session any) Response`; tracker answer `file <status> <content-type> <n>B`.

- [ ] **Step 1: Write the failing tests** (`rt/web/file_test.go`, package `web`, reusing `one`, `serve`, `testOpts`, `testSession`, `sessionCookie` from the package's tests; `router_test.go:157` `TestRouter_StaticAndScript` shows the Datastar navigate assertion to copy)

```go
package web

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	rtpkg "alder/rt"
)

func fileRoute(f rtpkg.File, calls *int) []Route {
	return []Route{{Method: "GET", Pattern: "/r.csv", File: true, Handle: func(rq *Request) Response {
		*calls++
		return FileResponse(f, rq.Session)
	}}}
}

func TestFile_WrittenVerbatim(t *testing.T) {
	var n int
	f := rtpkg.File{Status: 200, ContentType: "text/csv", Body: []byte("a,b\n"),
		Headers: [][2]string{{"Content-Disposition", "attachment; filename=r.csv"}, {"Set-Cookie", "x=1"}}}
	h := NewRouter(fileRoute(f, &n), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/csv" || w.Body.String() != "a,b\n" {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if w.Header().Get("Content-Disposition") != "attachment; filename=r.csv" {
		t.Fatal("allowed header sent")
	}
	if w.Header().Get("Set-Cookie") != "" || w.Header().Get(StatusHeader) != "" {
		t.Fatal("refused header dropped; no Alder-Status; no session cookie")
	}
}

func TestFile_StatusAndDefaults(t *testing.T) {
	var n int
	h := NewRouter(fileRoute(rtpkg.File{Status: 503, Body: []byte("busy"), Headers: [][2]string{{"Retry-After", "20"}}}, &n), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
	if w.Code != 503 || w.Body.String() != "busy" || w.Header().Get("Retry-After") != "20" ||
		w.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("%d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if strings.Contains(w.Body.String(), "<!doctype") {
		t.Fatal("a file's 503 is its own body, never the maintenance page")
	}
}

func TestFile_StatusOutOfRangeIs500(t *testing.T) {
	var n int
	h := NewRouter(fileRoute(rtpkg.File{Status: 101, Body: []byte("x")}, &n), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
	if w.Code != 500 || !strings.HasPrefix(w.Body.String(), "<!doctype html>") {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
}

func TestFile_NoBodyStatuses(t *testing.T) {
	for _, code := range []int{204, 304} {
		var n int
		var logs bytes.Buffer
		h := NewRouter(fileRoute(rtpkg.File{Status: code, Body: []byte("ignored")}, &n), testOpts(func(o *Options) {
			o.Log = func(level, msg string) { logs.WriteString(level + " " + msg + "\n") }
		}))
		w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
		if w.Code != code || w.Body.Len() != 0 || strings.Contains(logs.String(), "error") {
			t.Fatalf("%d: got %d %q, logs %q", code, w.Code, w.Body.String(), logs.String())
		}
	}
}

func TestFile_HeaderCaseAndNewlines(t *testing.T) {
	var n int
	f := rtpkg.File{Body: []byte("x"), Headers: [][2]string{
		{"content-disposition", "inline"}, {"X-Note", "a\r\nSet-Cookie: x=1"}}}
	h := NewRouter(fileRoute(f, &n), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
	if w.Header().Get("Content-Disposition") != "inline" {
		t.Fatal("a lowercase allowed name is sent, canonicalised")
	}
	if w.Header().Get("Set-Cookie") != "" || strings.ContainsAny(w.Header().Get("X-Note"), "\r\n") {
		t.Fatalf("a newline in a value must not inject a header: %q", w.Header())
	}
}

func TestFile_DatastarGetNavigates(t *testing.T) {
	var n int
	h := NewRouter(fileRoute(rtpkg.File{Body: []byte("x")}, &n), testOpts(nil))
	req := httptest.NewRequest("GET", "/r.csv", nil)
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if n != 0 {
		t.Fatalf("load ran %d times for a Datastar GET of a file", n)
	}
	if !strings.Contains(w.Body.String(), "/r.csv") || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("want the navigate event: %q", w.Body.String())
	}
}

func TestFile_RedirectIntoFileNavigates(t *testing.T) {
	var n int
	routes := append(fileRoute(rtpkg.File{Body: []byte("x")}, &n), Route{Method: "POST", Pattern: "/go", Handle: func(rq *Request) Response {
		return RedirectResponse("/r.csv", rq.Session)
	}})
	h := NewRouter(routes, testOpts(nil))
	req := httptest.NewRequest("POST", "/go", nil)
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if n != 0 || !strings.Contains(w.Body.String(), "/r.csv") || strings.Contains(w.Body.String(), "datastar-patch-elements") {
		t.Fatalf("a redirect into a file is the navigate event, never a morph: n=%d %q", n, w.Body.String())
	}
}
```

The POST in the last test may need the CSRF/origin header the router's other POST tests set (`grep -n 'httptest.NewRequest("POST"' rt/web/*_test.go | head -3`); copy what they set. Compare the navigate body with the exact SSE text `TestRouter_StaticAndScript` expects for `/static/`, and tighten the `strings.Contains` checks to equality with that form for `/r.csv`.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./rt/web -run TestFile -count=1`
Expected: FAIL to compile (`unknown field File in struct literal`, `undefined: FileResponse`).

- [ ] **Step 3: Types**

`rt/web/request.go`: `Route` gains `File bool // a file page's GET: answered raw; never morphed or followed`. `Response` gains `Raw *rtpkg.File // a file page's answer, written as given` (import `rtpkg "alder/rt"` if the file does not already). Extend the kind comment ("…; Raw set: a file, with its own status"). Constructor:

```go
// FileResponse is a file page's answer: f written as given, with f's status.
func FileResponse(f rtpkg.File, session any) Response {
	st := f.Status
	if st == 0 {
		st = http.StatusOK
	}
	return Response{Status: st, Raw: &f, Session: session}
}
```

`router.go` `route` struct gains `file bool`, copied from `Route.File` where `NewRouter` builds `route`s.

- [ ] **Step 4: Datastar GET of a file route — before the handler runs**

In `serve`, right after `hit` is found (after line 465's not-found return):

```go
	if hit.file && datastar.IsDatastar(r) {
		rt.reload(w, r) // a file is a document of its own: a real navigation, load not run
		return
	}
```

- [ ] **Step 5: `write`'s raw branch**

At the top of `write` (line 659), before the tracker switch:

```go
	if resp.Raw != nil {
		rt.writeFile(w, r, st, rq, resp)
		return
	}
```

and the method:

```go
// writeFile writes a file page's answer as given: allow-listed headers, Content-Type, the
// status, the body. No doctype, head tags, panel, Alder-Status or morph.
func (rt *router) writeFile(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	f := resp.Raw
	if resp.Status < 200 || resp.Status > 599 {
		rt.fail(w, r, st, html.None(), fmt.Errorf("a file page answered status %d; a file's status is 200 to 599", resp.Status), nil)
		return
	}
	if err := rt.setSession(w, r, rq, resp.Session); err != nil {
		rt.fail(w, r, st, html.None(), err, nil)
		return
	}
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer(fmt.Sprintf("file %d %s %dB", resp.Status, ct, len(f.Body)))
	}
	hd := w.Header()
	for _, h := range f.Headers {
		name, ok := rtpkg.FileHeaderAllowed(h[0])
		if !ok {
			rt.log("warn", fmt.Sprintf("%s: header %q cannot be set from a page; dropped", r.URL.Path, h[0]))
			continue
		}
		hd.Add(name, strings.NewReplacer("\r", " ", "\n", " ").Replace(h[1]))
	}
	hd.Set("Content-Type", ct)
	w.WriteHeader(resp.Status)
	if resp.Status != http.StatusNoContent && resp.Status != http.StatusNotModified && r.Method != http.MethodHead {
		_, _ = w.Write(f.Body)
	}
}
```

(Match `rt.fail`'s and `rt.log`'s real signatures — `router.go:933` and the `log` method; the report quotes `rt.fail(w, r, st, html.None(), err, nil)`.)

- [ ] **Step 6: Redirects never follow into a file**

Make `followable` a router method that also refuses a path matching a `file` route:

```go
func (rt *router) followable(loc string) bool {
	if !strings.HasPrefix(loc, "/") || strings.HasPrefix(loc, "//") || strings.HasPrefix(loc, "/\\") {
		return false
	}
	p, _, _ := strings.Cut(loc, "?")
	p, _, _ = strings.Cut(p, "#")
	if p == datastar.ScriptPath || strings.HasPrefix(p, staticPrefix) {
		return false
	}
	return !rt.isFileRoute(p)
}

// isFileRoute reports whether GET path p is a file page's.
func (rt *router) isFileRoute(p string) bool {
	segs, ok := splitPath(p)
	if !ok {
		return false
	}
	for i := range rt.routes {
		if rt.routes[i].method == http.MethodGet && rt.routes[i].file {
			if _, ok := rt.routes[i].match(segs); ok {
				return true
			}
		}
	}
	return false
}
```

`pathSegments(r)` takes a request; factor its body into `splitPath(p string) ([]string, bool)` (or whatever its segment type is) and call that from both. Update the call site at line ~690 (`rt.followable(loc)`) and any test calling `followable` directly (`grep -n 'followable(' rt/web/*_test.go`). With `followable` false, the existing branch answers `datastar.Navigate(w, loc, datastar.Assign)`.

- [ ] **Step 7: Run**

Run: `go test ./rt/web -count=1`
Expected: PASS (all old tests too: HTML answers are unchanged).

- [ ] **Step 8: Commit**

```bash
git add rt/web
git commit -m "web: file answers written raw; Datastar GETs and redirects into files navigate"
```

---

### Task 5: The checker — file pages, E0034, E0035, status literals

**Files:**
- Create: `testdata/corpus/E0034-file-page-msg/{app/App.alder,app/Report.alder,expected.diag}`, `testdata/corpus/E0035-file-header/…`, `testdata/corpus/E0003-file-status/…`, `testdata/corpus/E0003-file-view/…`
- Modify: `internal/diag/diag.go:93-129`, `internal/diag/explain.go`, `internal/diag/diag_test.go:81`, `internal/check/result.go:62` (`PageInfo`), `internal/check/decls.go:801-865` (`definePage`), `internal/check/pages.go:12-70`, `internal/check/expr.go:1240` (`inferApp` hook), `internal/check/browser.go:207-234` (`testGet`), `testdata/golden/check_pages.diag`

**Interfaces:**
- Consumes: `types.File` (Task 1), `rt.FileHeaderAllowed` (Task 1).
- Produces: `check.PageInfo.File bool`; `diag.FilePageMsg = "E0034"`, `diag.FileHeader = "E0035"`; `mc.checkFileArg(name string, args []syntax.Expr)`.

- [ ] **Step 1: Write the failing corpus cases**

Each dir has `app/App.alder` copied from `testdata/corpus/E0003-startswith-not-text/app/App.alder` (the `error`/`notFound` pair).

`E0034-file-page-msg/app/Report.alder`:

```alder
module Report

page "/report.txt" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

msg Refresh { }

update : Msg -> Model -> Session -> Task Nav
update m model session = Task.succeed Stay

view : Model -> Session -> File
view model session = File.text { contentType: "text/plain", body: "hi" }
```

`expected.diag`: one E0034 line at `msg Refresh` and one at `update` (positions: the `msg` keyword's line:col and the `update` signature's). Generate with `go test ./internal/check -run TestCorpus -update` after implementing, then **read the file** and confirm exactly those two lines and the E0034 text from Global Constraints. (If the page has messages, `update`'s `Nav` and `Msg` checks may add E0003s: the rule is that E0034 is the only code reported; suppress `requireSig("update", …)` for file pages.) Add a `form` case in the same module: `form Filter { q: Text }` gets an E0034 line too.

`E0035-file-header/app/Report.alder` — same page header, `view`:

```alder
view : Model -> Session -> File
view model session =
  File.text { contentType: "text/plain", body: "hi" }
    |> File.withHeader { name: "Set-Cookie", value: "a=1" }
    |> File.withHeader { name: "content-disposition", value: "inline" }
    |> File.withHeader { name: "X-Replay", value: "1" }
```

`expected.diag`: exactly one line, E0035 on the `"Set-Cookie"` literal: `` `Set-Cookie` cannot be set from a page; the headers a file may set are … `` (lowercase `content-disposition` and `X-Replay` pass).

`E0003-file-status/app/Report.alder` — `view` uses `File.withStatus 42` and `File.withStatus 503` (two pages or one chain): one E0003 line on `42`: ``File.withStatus` takes a status from 200 to 599, got 42`.

`E0003-file-view/app/Report.alder` — `view : Model -> Session -> Text` → one E0003: ``view` must have type `Model -> Session -> Html Msg`, or `Model -> Session -> File` for a file page`.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/check -run TestCorpus -count=1`
Expected: FAIL — the new dirs have no `expected.diag` content matching (write placeholder empty `expected.diag` files first so the failure shows the actual diagnostics, which today are E0003s on `view`'s `File` result).

- [ ] **Step 3: Diagnostic codes**

`internal/diag/diag.go` after `SiloDecl`:

```go
	FilePageMsg = "E0034" // a `msg`, `form` or `update` in a file page (view returns File)
	FileHeader  = "E0035" // File.withHeader with a literal name outside the allow-list
```

`explain.go`, one-line entries in the house style:
- `"E0034": "A file page has no messages (chapter 6, \"File pages\"). A page whose `view` returns `File` answers a body of its own (a CSV, a zip, a JSON document) instead of an HTML document, so there is no document to post a message or a form from and no `update` to run. Move the `msg`, `form` or `update` to an HTML page, and link to the file page from there.",`
- `"E0035": "A header a file page cannot set (chapter 6, \"File pages\"). `File.withHeader` sets `Content-Disposition`, `Cache-Control`, `ETag`, `Last-Modified`, `Retry-After`, `Content-Language` and any `X-…` name, in any case. The router owns the rest: `Content-Type` comes from the file, `Set-Cookie` from the session, `Location` from a redirect, `Content-Length` and the hop-by-hop headers from the server. A name that is not a literal is checked when the page answers, and a refused one is dropped with a log line.",`

`diag_test.go:81`: add `"E0034", "E0035"` after `"E0033"`.

- [ ] **Step 4: Mark file pages in `definePage`**

`internal/check/result.go` `PageInfo`: `File bool // view returns File (chapter 6, "File pages")`.

`internal/check/decls.go`, in `definePage` where `info` is built (line ~804): `info.File = viewReturnsFile(c.live(mc.m))`, with

```go
// viewReturnsFile reports whether a page module's declared `view` returns File. It reads the
// signature's syntax, so every module knows it before bodies are checked; `File` is a
// reserved Prelude name (E0012), so the name decides.
func viewReturnsFile(decls []syntax.Decl) bool {
	for _, d := range decls {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok || vd.Name != "view" || vd.Sig == nil {
			continue
		}
		t := vd.Sig
		for {
			ft, ok := t.(*syntax.FunType)
			if !ok {
				break
			}
			t = ft.Result
		}
		tn, ok := t.(*syntax.TypeName)
		return ok && tn.Name == "File" && (tn.Module == "" || tn.Module == "Prelude") && len(tn.Args) == 0
	}
	return false
}
```

(Use the real element type `c.live` returns.)

- [ ] **Step 5: `checkPage` for file pages**

`internal/check/pages.go` `checkPage`: when `mc.page.File`:
- `requireSig("view", Fun{[Model, Session]} -> types.File, msg)` where `msg` is the view message below.
- skip the `update` requirement;
- for each live decl: `*syntax.MsgDecl`, `*syntax.FormDecl`, or a `*syntax.ValueDecl` named `update` → `c.errorf(diag.FilePageMsg, <decl's name span or Span>, "a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the %s to an HTML page", kind)` with kind `message` / `form` / `update`.

For an HTML page, the wrong-`view` message becomes "`view` must have type `Model -> Session -> Html Msg`, or `Model -> Session -> File` for a file page" (keep the `Html Never` suffix logic). Build it in `pages.go`; leave `HookDoc`'s `sig` (LSP hover) as is, and add to `HookDoc["view"]`'s role text one clause: "or `File` for a file page (chapter 6)". Regenerate `testdata/golden/check_pages.diag` with `go test ./internal/check -run TestCheckGolden -update` and read the diff: only the `view` messages change.

- [ ] **Step 6: Literal arguments to `File.withStatus` / `File.withHeader`**

`internal/check/expr.go` `inferApp`, beside the `Test` selector hook (line 1240):

```go
	if v, ok := x.Fn.(*syntax.Var); ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "File" {
		mc.checkFileArg(v.Ref.Name, x.Args)
	}
```

and in `pages.go`:

```go
// checkFileArg checks the literal first argument of File.withStatus (200..599) and
// File.withHeader (an allowed name). The first argument is x.Args[0] in both the direct and the
// piped form; a non-literal is checked when the page answers.
func (mc *modCtx) checkFileArg(name string, args []syntax.Expr) {
	if len(args) == 0 {
		return
	}
	switch name {
	case "withStatus":
		if lit, ok := args[0].(*syntax.IntLit); ok && (lit.Value < 200 || lit.Value > 599) {
			mc.c.errorf(diag.TypeMismatch, lit.Span, "`File.withStatus` takes a status from 200 to 599, got %d", lit.Value)
		}
	case "withHeader":
		rec, ok := args[0].(*syntax.RecordLit)
		if !ok {
			return
		}
		for _, f := range rec.Fields {
			if f.Name != "name" { // use FieldInit's real name field
				continue
			}
			lit, ok := f.Value.(*syntax.StrLit)
			if !ok {
				return
			}
			var text strings.Builder
			for _, p := range lit.Parts {
				if p.Expr != nil {
					return
				}
				text.WriteString(p.Text)
			}
			if _, ok := rtpkg.FileHeaderAllowed(text.String()); !ok {
				mc.c.errorf(diag.FileHeader, lit.Span, "`%s` cannot be set from a page; the headers a file may set are `Content-Disposition`, `Cache-Control`, `ETag`, `Last-Modified`, `Retry-After`, `Content-Language` and `X-…` names", text.String())
			}
		}
	}
}
```

`internal/check` importing `alder/rt` (`rtpkg "alder/rt"`) is new: `rt/file.go` imports only the standard library, so it is cheap. If `go list -deps ./internal/check` shows it pulling more of `rt` than `rt/file.go`'s package needs, move the allow-list into a leaf package `rt/fileheader` instead (embed it) and have both import that. (A negative status literal is not an `IntLit` and is left to the run-time 500.)

- [ ] **Step 7: `Test.get`/`Test.open` on a file page** (`internal/check/browser.go` `testGet`, after `c.pages[page] == nil`):

```go
	if c.pages[page].File && name != "fetch" {
		c.errorf(diag.TypeMismatch, arg.GetSpan(), "`%s` is a file page: use `Test.fetch`", page)
		return types.Err
	}
```

Add a corpus case to `E0003-file-view`'s dir (a second module or test in it): `test "x" = { let r <- Test.open (Report {}) Task.succeed True }` against a file page → that line. (`Test.fetch` itself is Task 7.)

- [ ] **Step 8: Fill the expected files and run**

Run: `go test ./internal/check -run 'TestCorpus|TestCheckGolden|TestFixesApply' -update -count=1`, then read every new `expected.diag` and confirm it holds exactly the lines Step 1 lists; then `go test ./internal/check ./internal/diag -count=1`.
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/check internal/diag testdata/corpus/E0034-file-page-msg testdata/corpus/E0035-file-header testdata/corpus/E0003-file-status testdata/corpus/E0003-file-view testdata/golden/check_pages.diag
git commit -m "check: file pages (view returns File); E0034 messages, E0035 headers, status range"
```

---

### Task 6: IR and gen — the file route and links to it

**Files:**
- Modify: `internal/ir/ir.go:299` (`ir.Page`), the lowering that builds `ir.Page` (`grep -n '&Page{' internal/ir/*.go`), `internal/gen/routes.go:228-251`, `internal/gen/html.go:134-135`, `internal/gen/gen_test.go`

**Interfaces:**
- Consumes: `check.PageInfo.File` (Task 5), `web.FileResponse`, `web.Route.File` (Task 4).
- Produces: `ir.Page.File bool`; generated `web.Route{Method: "GET", Pattern: …, File: true, Handle: …}` returning `web.FileResponse(M__view(model, session), rq.Session)`; `html.A("data-alder-reload", "true")` beside the `href` of a literal link to a file page.

- [ ] **Step 1: Write the failing gen test** (`internal/gen/gen_test.go`, using `emitSource` and `contains`)

```go
func TestGen_FilePage(t *testing.T) {
	files := emitSource(t, map[string]string{
		"app/App.alder": "module App\n\nerror : Error -> Html Never\nerror err = <h1>Error</h1>\n\nnotFound : Html Never\nnotFound = <h1>Not found</h1>\n",
		"app/Report.alder": `module Report

page "/report.txt" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> File
view model session = File.text { contentType: "text/plain", body: "hi" } |> File.withStatus 503
`,
		"app/Home.alder": `module Home

page "/" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session = <a href={Report {}}>report</a>
`,
	})
	contains(t, files["routes__alder.go"],
		`return web.Route{Method: "GET", Pattern: "/report.txt", File: true, Handle: func(rq *web.Request) web.Response {`,
		"return web.FileResponse(Report__view(model, session), rq.Session)")
	contains(t, files["Home_alder.go"], `html.A("data-alder-reload", "true")`)
	vet(t, files)
}
```

Check the generated file names in an existing `emitSource` test (`grep -n 'files\["' internal/gen/gen_test.go | head`) and use them; also check how a bare page with no params is written as a link value in other tests (`Report {}` vs `Report`).

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/gen -run TestGen_FilePage -count=1`
Expected: FAIL (the route returns `web.PageResponse`).

- [ ] **Step 3: Implement**

- `ir.Page`: `File bool // view returns File: a raw GET answer, no messages`; set it from `PageInfo.File` where the lowerer builds the page.
- `routes.go` GET emitter: when `pg.File`, add `File: true, ` after the `Pattern` in the `web.Route` literal and replace the last line with `return web.FileResponse(%s(model, session), rq.Session)` (`GoName(m, "view")`); no `html.FormState`. Update the step comment ("view (… for a file page, its File written raw)"). A file page has no POST routes (E0034 guarantees no messages).
- `html.go` `case ir.AttrHref`: after appending the `href`, `if pr, ok := a.Value.(*ir.PageRef); ok && pr.Page != nil && pr.Page.File { out = append(out, `html.A("data-alder-reload", "true")`) }`. A `Page` held in a variable gets no attribute; the router's navigate answer (Task 4) covers it.
- Mutation: `internal/mutate/mutate.go`'s header says values whose result is `Html` are never mutated; a file page's `view` returns `File`, so it is mutated like any value (spec §5: "the existing kinds apply inside a file page's `load` and `view`"). No change; add one sentence to that header comment saying so.

- [ ] **Step 4: Run**

Run: `go test ./internal/gen ./internal/ir -count=1`
Expected: PASS, goldens unchanged (no golden app has a file page).

- [ ] **Step 5: Commit**

```bash
git add internal/ir internal/gen internal/mutate/mutate.go
git commit -m "gen: file page GET route returns FileResponse; links to file pages reload"
```

---

### Task 7: `Test.fetch` and the end-to-end `file-ok` app

**Files:**
- Create: `testdata/corpus/file-ok/{app/App.alder,app/User.alder,app/Auth/Login.alder,app/Precincts.alder,app/Results.alder,app/Zip.alder,app/Detail.alder,app/Turnout.alder,app/Slow.alder,app/Busy.alder,app/Secret.alder,expected.diag}`
- Modify: `stdlib/Test.alder:14-26`, `internal/check/browser.go:26, 207-234`, `internal/ir/ir.go:1003` (`TestGet`), the lowering of `Test.get` (`grep -n 'TestGet{' internal/ir/*.go`), `internal/gen/browser.go:10-35, 108-136`, `internal/gen/stdcall.go:150` (`Effectful`), `rt/testing/browser.go:620-721`, `cmd/alder/test_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: `Test.fetch : Page -> Task { status: Int, contentType: Text, body: Bytes, headers: List { name: Text, value: Text } }`; `ir.TestGet.Fetch bool`; `rtesting.Fetched = struct{ Status int64; ContentType string; Body []byte; Headers []struct{ Name string; Value string } }`; `(*rtesting.Browser).Fetch(u string) (Fetched, error)`.

- [ ] **Step 1: Write the end-to-end app (the spec's success criteria)**

Base it on `testdata/corpus/browser-ok` (copy `App.alder`, `User.alder`, `Auth/Login.alder` so `requires user` and `App.login` work). The new modules:

- `Results` — `page "/results/allracesbyprecinct.txt" { }`, `view` returns `File.text { contentType: "text/plain", body: "MN;1;Senate\n" }`.
- `Zip` — `capability Zip { detail : { election: Text } -> Query Bytes }` with `fake Zip { detail = \r -> Query.succeed (Text.toBytes ("PK" ++ r.election)) }` (use the fake syntax from `testdata/corpus/fake-ok`).
- `Detail` — `page "/{id}/detailxml.zip" { id: Text }`, `load` calls `Zip.detail { election: params.id }` into the model, `view` is `File.bytes { contentType: "application/zip", body: model.zip } |> File.withHeader { name: "Content-Disposition", value: "attachment; filename=detailxml.zip" }`.
- `Turnout` — `page "/lvt" { }`, `view = File.html (Html.fragment [Html.unsafe "<!doctype html>\n", <html><body><p id="n">42</p></body></html>])` (use the real fragment/list form `stdlib/Html.alder` offers).
- `Busy` — `page "/busy" { }`, `view` = `File.text {…"busy"} |> File.withStatus 503 |> File.withHeader { name: "Retry-After", value: "20" }`.
- `Slow` — `page "/slow" { }`, `load` does `let before <- Time.now`, `let _ <- Query.delay 20000`, `let after <- Time.now`, model `{ waited: Time.diffMillis before after }`; `view` = `File.json (Int.toText model.waited)`.
- `Secret` — `page "/secret.txt" { } requires user`, `view` = `File.text {…}`.
- `Precincts` — an HTML page `page "/" { }` whose `view` links `<a href={Results {}}>results</a>`; also a page with param `{ id: Text }` whose `load` does `Query.notFound` for id `"missing"` (or put that in `Detail`).

Tests (in the modules, `test "…" = { … }`):

```alder
test "results is text/plain" = {
  let r <- Test.fetch (Results {})
  Task.succeed ({ status: r.status, type: r.contentType, body: Bytes.toText r.body }
    == { status: 200, type: "text/plain", body: Just "MN;1;Senate\n" })
}

test "detail zip comes from the capability" = {
  let r <- Test.fetch (Detail { id: "2024" })
  Task.succeed (r.contentType == "application/zip" && Bytes.toText r.body == Just "PK2024"
    && List.member { name: "Content-Disposition", value: "attachment; filename=detailxml.zip" } r.headers)
}

test "turnout is byte-exact html" = {
  let r <- Test.fetch (Turnout {})
  Task.succeed (Bytes.toText r.body == Just "<!doctype html>\n<html><body><p id=\"n\">42</p></body></html>")
}

test "busy is a 503 with its own body" = {
  let r <- Test.fetch (Busy {})
  Task.succeed (r.status == 503 && Bytes.toText r.body == Just "busy")
}

test "slow waits twenty virtual seconds" = {
  let r <- Test.fetch (Slow {})
  Task.succeed (Bytes.toText r.body == Just "21000")
}

test "fetch a missing report is 404" = {
  let r <- Test.fetch (Detail { id: "missing" })
  Task.succeed (r.status == 404)
}

test "fetch while logged out is 303" = {
  let r <- Test.fetch (Secret {})
  Task.succeed (r.status == 303 && List.any (\h -> h.name == "Location") r.headers)
}
```

(`21000`: the two `Time.now` readings tick one second apart plus the 20 s delay — `Clock` semantics from Task 3. Use the real `List.member`/`List.any` names from `stdlib/List.alder`.) Add a `Test.trace`-based test that the trace contains `"Query.delay 20000ms"`.

Add `cmd/alder/test_test.go`:

```go
func TestTest_FilePages(t *testing.T) {
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/file-ok")
	if err != nil {
		t.Fatal(err)
	}
	inTemp(t)
	copyDir(t, src, "file-ok")
	start := time.Now()
	code, stdout, stderr := runTest(t, "file-ok")
	if code != 0 || !strings.Contains(stdout, "8 passed, 0 failed\n") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if time.Since(start) > 2*time.Minute {
		t.Fatalf("the 20 s delay must be virtual; took %v", time.Since(start))
	}
}
```

(Adjust "8 passed" to the number of tests written; the build dominates the time bound, which only catches a real 20 s sleep per run in aggregate. The decisive check is the `21000` body.)

- [ ] **Step 2: Run to see it fail**

Run: `go test ./internal/check -run TestCorpus -count=1` (fails: `Test.fetch` unknown), then `go test ./cmd/alder -run TestTest_FilePages -count=1`.
Expected: FAIL.

- [ ] **Step 3: Declare and type `Test.fetch`**

`stdlib/Test.alder`, after `open`:

```
-- special form: the argument is a page value; the raw answer, redirects not followed
fetch : Page -> Task { status: Int, contentType: Text, body: Bytes, headers: List { name: Text, value: Text } }
```

`internal/gen/stdcall.go` `Effectful`: `"Test.fetch": true`. `internal/check/browser.go`: add `fetch` to the step map at line 26 (sample `Test.fetch (Report.Csv { id })`), route it to `testGet` like `open`, and in `testGet` after the file-page check:

```go
	if name == "fetch" {
		header := &types.Record{Fields: []types.Field{{Name: "name", Type: types.Text}, {Name: "value", Type: types.Text}}}
		return &types.Record{Fields: []types.Field{
			{Name: "status", Type: types.Int}, {Name: "contentType", Type: types.Text},
			{Name: "body", Type: types.Bytes}, {Name: "headers", Type: &types.App{Con: "List", Args: []types.Type{header}}},
		}}
	}
```

(Match how `stepResult` wraps its record — if it returns `Task {…}`, wrap the same way; if the stdlib record type in `Test.alder` is marked `Anonymous`, set that field too, so the type unifies with the declared signature.)

- [ ] **Step 4: IR and gen**

`ir.TestGet` gains `Fetch bool`; the lowering sets it for `Test.fetch`. `internal/gen/browser.go` `case *ir.TestGet`: name `"Test.fetch "`; after `_u`/`_b` are built:

```go
			if x.Fetch {
				fmt.Fprintf(&w, "%s, %s := %s.Fetch(%s)\n%sreturn %s, nil\n", res, err, b, u, fail(c, err), res)
				return w.String()
			}
```

The record's Go type is `GoType` of the declared record: `struct{ Status int64; ContentType string; Body []byte; Headers []struct{ Name string; Value string } }`, identical to `rtesting.Fetched`, so `res` returns as is. Add `Test.fetch` to the step list in the doc comment at `browser.go:10-35`.

- [ ] **Step 5: `Browser.Fetch` and the HTML-only guard**

`rt/testing/browser.go`:

```go
// Fetched is Test.fetch's answer; its fields are the Alder record's, in its order.
type Fetched = struct {
	Status      int64
	ContentType string
	Body        []byte
	Headers     []struct{ Name string; Value string }
}

// Fetch is Test.fetch: one GET through the browser (cookie, savepoint, trace), the raw answer
// returned and redirects not followed. The open document is unchanged.
func (b *Browser) Fetch(u string) (Fetched, error) {
	w, err := b.serve("GET", u, nil) // use serve's real signature
	if err != nil {
		return Fetched{}, err
	}
	res := w.Result()
	out := Fetched{Status: int64(res.StatusCode), ContentType: res.Header.Get("Content-Type"), Body: w.Body.Bytes()}
	names := slices.Sorted(maps.Keys(res.Header))
	for _, n := range names {
		for _, v := range res.Header[n] {
			out.Headers = append(out.Headers, struct{ Name string; Value string }{n, v})
		}
	}
	return out, nil
}
```

`serve` (lines 620-660) must not replace the open document: read whether it sets `b.url`/`b.doc`; if it does, save and restore them around the call in `Fetch`. Its cookie handling stays (a file page cannot change the session, but a `requires` redirect path might).

In `document` (703-721), before the status mapping: a response whose `Content-Type` does not start with `text/html` answers `RequestFailed (method + " " + path + ": a file page (" + contentType + "): use `Test.fetch`")` — the run-time guard when the checker could not see the page (Task 5 Step 7 is the static one).

- [ ] **Step 6: Run**

Run: `go test ./internal/check -run TestCorpus -count=1 && go test ./rt/testing ./internal/gen -count=1 && go test ./cmd/alder -run 'TestTest_FilePages|TestTest_Browser' -count=1`
Expected: PASS. `file-ok/expected.diag` is empty.

- [ ] **Step 7: Commit**

```bash
git add stdlib/Test.alder internal rt/testing testdata/corpus/file-ok cmd/alder/test_test.go
git commit -m "test: Test.fetch; file-ok corpus app with text, zip, byte-exact html, 503 and a virtual delay"
```

---

### Task 8: Generators — `new page --file`, refusals

**Files:**
- Create: `internal/generate/templates/page_file.tmpl`
- Modify: `internal/generate/specs.go:86-107`, `internal/generate/page.go:88, 124, 190-222, 318-330`, `internal/generate/msg.go:~52`, the `new form` entry (`grep -n 'func NewForm' internal/generate/*.go`), `cmd/alder/generate.go:235` (`newPageUsage`), `cmd/alder/generate_test.go:~360-420`, `testdata/help.json`

**Interfaces:**
- Consumes: `File`, `File.text` (Task 1), `Test.fetch` (Task 7).
- Produces: `PageOptions.File bool`, `PageOptions.ContentType string`; `generate.isFilePage(m *syntax.Module) bool`.

- [ ] **Step 1: Write the failing CLI tests** (rows in `cmd/alder/generate_test.go`'s refusal table, and a success test beside the existing `new page` success test — find it with `grep -n '"new", "page"' cmd/alder/generate_test.go | head`)

- `new page Reports.Csv /reports.csv --file --type text/csv` succeeds; the written `app/Reports/Csv.alder` contains `view : Model -> Session -> File` and `File.text { contentType: "text/csv", body: ?body }`; `alder check` on the result reports only the hole (whatever the hole diagnostic is today — copy from the plain-page hole test).
- `--file` without `--type` writes `contentType: "text/plain"`.
- `new page X /x --file --list Note` → stderr `alder new page: --file makes a file page; --list and --show make HTML pages` exit 1 (same for `--show`, `--example`).
- `new msg Reports.Csv Refresh` on that file page → stderr `alder new msg: a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the message to an HTML page`, exit 1. Same text for `new form`.

- [ ] **Step 2: Run to see them fail**

Run: `go test ./cmd/alder -run 'TestGenerate|TestNew' -count=1`
Expected: FAIL (`unknown flag --file`).

- [ ] **Step 3: Implement**

- `specs.go` `new page` fields: `{Name: "file", Kind: ArgBool}`, `{Name: "type", Kind: ArgText}`; add `"file"` to `example`'s `DefaultUnless`; `Run` passes `File`, `ContentType` into `PageOptions`.
- `page.go`: refusal beside the `--list/--show` ones: `if o.File && (o.List != "" || o.Show != "" || o.From != "" || o.Blank) { return nil, Refusef("--file makes a file page; --list and --show make HTML pages") }`; `--type` without `--file` → `Refusef("--type goes with --file")`. Template switch: `case o.File: tmpl = "page_file.tmpl"`; `ContentType` defaults to `text/plain`.
- `page_file.tmpl` (delimiters `[[ ]]`, the `header` template from `common.tmpl`):

```
[[template "header" .]]

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> File
view model session = File.text { contentType: "[[.ContentType]]", body: ?body }
```

  The generated test for a file page uses `Test.fetch` and checks `r.status == 200` (build it with the same test builder the HTML templates use, swapping the step and assertion; with `--param`, the builder's sample params apply unchanged).
- `isFilePage(m *syntax.Module) bool` in `page.go`, the same signature walk as Task 5's `viewReturnsFile` (call that if it can be exported from `check` without an import cycle; otherwise duplicate the ten lines). `NewMsg` (`msg.go`, right after `pageModule`) and `NewForm`: `if isFilePage(m.Syntax) { return nil, Refusef("a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the message to an HTML page") }`.
- `cmd/alder/generate.go` `newPageUsage`: add `[--file [--type T]]` and one line: `--file      a file page: view returns File (text, CSV, JSON, a zip) instead of HTML`.
- Regenerate `testdata/help.json` the way `helpjson_test.go` says (`grep -n update cmd/alder/helpjson_test.go`).

- [ ] **Step 4: Run**

Run: `go test ./internal/generate ./cmd/alder -run 'TestGenerate|TestNew|TestHelpJSON' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/generate cmd/alder/generate.go cmd/alder/generate_test.go testdata/help.json
git commit -m "generate: new page --file [--type]; new msg/form refuse a file page"
```

---

### Task 9: Browser smoke — a link to a file page loads a document

**Files:**
- Modify: `cmd/alder/testdata/browser/Smoke.alder`, `cmd/alder/testdata/browser/smoke.cjs:~148`

**Interfaces:**
- Consumes: Tasks 4 and 6 (generated `data-alder-reload`, router navigate).

- [ ] **Step 1: Add the file page and links**

`Smoke.alder` gains a file page module (it is copied into the blog as `app/Smoke.alder`; if the file holds one module, add a second file `SmokeFile.alder` and map it in `startRunAppWith`'s file map in `cmd/alder/browser_test.go:~50`): `page "/smoke.txt" { }`, `view` = `File.text { contentType: "text/plain", body: "smoke file" }`, and on the existing smoke HTML page a link `<a id="file" href={SmokeFile {}}>file</a>`.

`smoke.cjs`, beside the `/static/x.pdf` check:

```js
// a link to a file page: the compiler marks it data-alder-reload; a plain document load
await page.goto(BASE + '/smoke');            // the smoke HTML page's path
const marked = await page.getAttribute('#file', 'data-alder-reload');
check('a file page link is marked', marked === 'true', marked);
await page.click('#file');
await page.waitForURL('**/smoke.txt', { timeout: 5000 }).catch(() => {});
check('a link to a file page: a plain navigation', docs.some((d) => d === 'GET ' + BASE + '/smoke.txt'), docs);
check('the file is shown as text', (await page.textContent('body')).includes('smoke file'), await page.content());
// a boosted GET of the same URL without the mark (a Page held in a variable) still navigates
await page.goto(BASE + '/smoke');
await link('/smoke.txt');
await page.click('#go');
await page.waitForURL('**/smoke.txt', { timeout: 5000 }).catch(() => {});
check('an unmarked link to a file page navigates', page.url().endsWith('/smoke.txt'), page.url());
```

- [ ] **Step 2: Run**

Run: `go test -tags browser -run Browser ./cmd/alder -count=1`
Expected: PASS (or SKIP when Playwright is absent — then say so in the hand-back; do not claim it passed).

- [ ] **Step 3: Commit**

```bash
git add cmd/alder/testdata/browser cmd/alder/browser_test.go
git commit -m "browser: smoke checks a link to a file page loads a document"
```

---

### Task 10: The book

**Files:**
- Modify: `book/06-pages.md` (new `## File pages` after `## \`update\` and \`Nav\`` at :320; the boost bullet list at :1520-1546), `book/07-effects.md`, `book/09-stdlib.md` (:189 `## Date, Time`, :242 effects section, `## Int, Float`, a new `## File, Bytes`), `book/10-testing.md` (§9 browser testing), `book/11-tooling.md` (:132 `alder new`, :217 generators table, :1554 diagnostics table), `book/12-rationale.md`

**Interfaces:**
- Consumes: the whole surface above; every `alder` block compiles in a copy of `examples/todos` (`book/book_test.go`); signature tables are compared with the stdlib.

- [ ] **Step 1: Write the chapter 6 section** — `## File pages`, with `###` subsections `Declaring a file page`, `Status and headers`, `Links and the boost`, `What stays HTML`. Content (spec §3.1–3.2, plain prose, for a beginner who does not know HTTP well: say what a content type and a status are in one sentence each):
  - a full module block (` ```alder `) that compiles against todos — e.g. `module Todos.Export`, `page "/todos.csv" { }`, `load` reading the todos table, `view` building CSV with `Text.join` and `File.text { contentType: "text/csv", … } |> File.withHeader { name: "Content-Disposition", value: "attachment; filename=todos.csv" }` (read `examples/todos/app` for the table and field names);
  - `File.html` with `Html.unsafe "<!doctype html>"` for byte-exact copies;
  - `File.withStatus` (200 to 599; why not 1xx in one sentence), the header allow-list and why the router owns the others;
  - E0034 as an ` ```alder error=E0034 "a file page has no messages" ` block, E0035 likewise;
  - links get `data-alder-reload`; a redirect into a file page navigates; 404/500/login stay HTML.
  Also extend the boost bullet list at :1520 with "a link to a file page: the compiler marks it `data-alder-reload` itself" and trim the hand-written `data-alder-reload` export example at :1546 to point at file pages.
- [ ] **Step 2: Chapter 7** — `### A capability returning bytes` (a `capability Zip { detail : … -> Query Bytes }` block, the adapter returning `[]byte`, the page wrapping it with `File.bytes`) and `### Waiting: Query.delay and Task.sleep` (request-scoped, clamped to a minute, a cancelled request stops it, a delay before the first database step holds no transaction, a delay inside a `Task` after a write holds one — don't).
- [ ] **Step 3: Chapter 9** — a `## File, Bytes` section with the signature table (exactly the stdlib lines) and an `alder eval=` example: ` ```alder eval=6 ` `Bytes.length (Text.toBytes "héllo")`; `Time.toMillis`/`fromMillis`/`addMillis`/`diffMillis` rows in `## Date, Time` with ` ```alder eval=0 ` `Time.toMillis Time.epoch`; `Float.sqrt/exp/log/pow` rows in `## Int, Float`; `Query.delay`/`Task.sleep` in the effects section; `Test.fetch` in `## Test`'s `text` signature block and bullets.
- [ ] **Step 4: Chapter 10** — in §9, a `Test.fetch` subsection: the record, redirects not followed, `Bytes.toText` to read text, the virtual delay (the clock moves; `Time.now` after it shows the time passed), and that `Test.open` on a file page is an error pointing here.
- [ ] **Step 5: Chapter 11** — `--file [--type T]` in the `alder new` table; E0034, E0035 rows after E0033 (`| E0034 | a file page with a message: "a file page has no messages: …" | — |`). Chapter 12: one paragraph "Why bytes have no operations" (encoders are Go adapters; a language without byte slicing has no off-by-one buffer bugs to write; the capability boundary is where formats live).
- [ ] **Step 6: Run the book verifier**

Run: `go test ./book -count=1`
Expected: PASS. Fix any block that fails by correcting the block (never by marking a real example `fragment` unless it cannot compile in todos by nature, like a capability adapter in Go).

- [ ] **Step 7: Commit**

```bash
git add book
git commit -m "book: file pages (ch 6), bytes and delays (ch 7), stdlib, Test.fetch, diagnostics"
```

---

### Task 11: The reference prompt

**Files:**
- Modify: `experiments/build_context.py:82-93` (`DROP_SUBSECTIONS`), `experiments/e11-cli/prompt-reference.md`

- [ ] **Step 1: Measure**

Run: `python3 experiments/build_context.py --reference --size`
Record the number (baseline before M17: 47,984).

- [ ] **Step 2: Drop the human detail**

Add to `DROP_SUBSECTIONS["06"]`: `"File pages": ["Status and headers", "Links and the boost", "What stays HTML"]` (keep `Declaring a file page`: an agent writing a file page needs the declaration and `File.text`). Add ch. 7's two new `###` subsections to `DROP_SUBSECTIONS["07"]` under their parent `##` names, keeping only the signatures that ch. 9 already carries. Measure again; target ≤ 48,500.

- [ ] **Step 3: Regenerate and test**

Run: `python3 experiments/build_context.py --reference > experiments/e11-cli/prompt-reference.md && cd experiments && python3 -m unittest -v`
Expected: PASS. Put the final `--size` number in the hand-back.

- [ ] **Step 4: Commit**

```bash
git add experiments/build_context.py experiments/e11-cli/prompt-reference.md
git commit -m "experiments: reference prompt with file pages; human detail dropped"
```

---

### Task 12: Whole-branch gate

- [ ] **Step 1:** `gofmt -l $(git ls-files '*.go')` (empty), `go vet ./...`, `go vet -tags browser ./...`.
- [ ] **Step 2:** `scripts/test.sh` — all parts `ok`. On a failure, read the failing log's tail the script prints, fix in the owning task's area, commit `fix: …`, rerun.
- [ ] **Step 3:** `go run ./cmd/alder check examples/todos && go run ./cmd/alder check examples/blog` (examples untouched, still clean), `go run ./cmd/alder test examples/todos`.
- [ ] **Step 4:** Spec walk: for each bullet of spec §5 "Tests to add", name the test that covers it in the hand-back (text page → `file-ok` "results"; doctype → "turnout"; capability Bytes + fake → "detail"; 503 → "busy"; E0035, E0034 → corpus; `Test.fetch` status/body/headers → `file-ok`; redirecting `requires` → "fetch while logged out"; delay advancing the clock + trace → "slow" + the trace test; real delay cancelled → `TestQueryDelay_RealSleepStopsOnCancel`; boost attribute + browser smoke → `TestGen_FilePage` + Task 9; `Time` round trips and `diffMillis` sign → `TestTimeMillis`; `Bytes` equality → `TestBytes`).
- [ ] **Step 5:** Push the branch only when the user says to (the spec commit is not pushed yet).
