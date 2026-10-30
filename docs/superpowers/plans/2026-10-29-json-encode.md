# Json encoder — implementation plan (#1)

Spec: `docs/superpowers/specs/2026-10-29-json-encode-design.md` (binding). Test-first, one commit
per task. Unit tests for `rt/std` go in a new `rt/std/json_test.go` (same package, the `eq`
helper of `std_test.go`) rather than `std_test.go`, to keep parallel branches conflict-free.

## Task 1 — runtime type and encoder (`rt/json.go`)

Tests first, `rt/json_test.go` (package `rt_test`):

- `TestJsonEncode_Scalars`: null, true/false, int 0 / -7 / max int64, float 1.0 → `1`,
  0.1 → `0.1`, -0.0 → `-0`, 1e21 → no exponent, NaN/+Inf/-Inf → `null`.
- `TestJsonEncode_Escapes`: one case per row of the spec's escaping table (`"`, `\`, `\b \t \n
  \f \r`, U+0000/U+001F → `\u0000`/`\u001f`, `<` `>` `&` → `<` `>` `&`, U+2028/9
  → ` `/` `), and the not-escaped ones (`/`, U+007F, `é`, an emoji), plus an invalid
  byte → U+FFFD.
- `TestJsonEncode_Containers`: empty `[]`/`{}`, nested compact, key order kept.
- `TestJsonEncodeIndented`: n=2 nested object/array/empty containers exactly as the JS form;
  n=0 and n=-1 equal compact.
- `TestJsonValue_EqKeyShow`: `rt.Eq` true for int 1 vs float 1.0 and NaN vs null, false for 1 vs 2;
  `rt.Key` equal exactly when `rt.Eq`; `rt.Show` prints compact text unquoted, and inside a
  `Just` as an argument without parentheses.

Code: `JsonKind` tag (`JsonNull` = 0 so the zero value is null), `JsonValue{Kind, Bool, Int,
Float, Text, Items []JsonValue, Members []JsonMember}`, `JsonMember{Key, Value}`;
`JsonEncode(v)`, `JsonEncodeIndented(n, v)`, a hand-written escape loop; `AlderEqual`,
`AlderKey`, `AlderShow` through the compact encoding.

## Task 2 — stdlib module (`stdlib/Json.alder`, `rt/std/json.go`, gen wiring)

Tests first, `rt/std/json_test.go`:

- `TestJson_Constructors`: each of `JsonNull`, `JsonBool`, `JsonInt`, `JsonFloat`, `JsonString`,
  `JsonList`, `JsonObject` encodes as expected through `JsonEncode`.
- `TestJson_ObjectDuplicateKeys`: `[a:1, b:2, a:3]` → `{"a":3,"b":2}`; `==` and Show see one key.
- `TestJson_ObjectDoesNotAliasInput`: mutating the input list after `object` does not change it.
- `TestJson_EncodeIndented`: `JsonEncodeIndented(2, …)` and `(0, …)`.
- `TestJson_DictKey`: a `rt.Dict[rt.JsonValue, int64]` treats `int 1` and `float 1.0` as one key.
- Then `TestStdCallsCoverStdlib` / `TestStdCallsExist` (internal/gen) fail until wired.

Code: `stdlib/Json.alder` with the spec's exact signature lines and `--` docs; `rt/std/json.go`;
`// Json` group in `internal/gen/stdcall.go`; `GoType` case `"Value"` with `Module == "Json"` →
`rt.JsonValue` in `internal/gen/types.go`.

## Task 3 — `File.json : Json.Value -> File`

Tests first: `rt/std/file_test.go` uses `FileJson(JsonInt(1))` etc. (body `1`, content type
`application/json`). Code: `stdlib/File.alder` line, `rt/std/file.go` `FileJson(v rt.JsonValue)`.
Corpus: `testdata/corpus/file-ok/app/Slow.alder` → `File.json (Json.int model.waited)`;
`testdata/corpus/E0034-file-page-msg/app/Feed.alder` → `File.json (Json.list [])`. Regenerate any
golden output the corpus checks; confirm the old `File.json "[]"` is now a type mismatch.

## Task 4 — end-to-end compile check

A page `testdata/corpus/file-ok/app/Api.alder` (file-ok is already run by `alder test` in
`TestTest_FilePages`, so no new runner) that uses every `Json` function, `encodeIndented`, `==`
on values, and a `Dict (Json.Value) Text`, so gen's mapping compiles and runs in Go; the test's
expected count goes from 9 to 13.

## Task 5 — book

- `book/09-stdlib.md`: `## Json` before `## File, Bytes`: signature block, one sentence each for
  null floats, key order/duplicates, HTML-safe escapes, `encodeIndented 0`, and `==`/Show; the
  `File.json` line in the File block.
- `book/06-pages.md` sentence on `File.json`; examples in `book/07-effects.md` and
  `book/10-testing.md`.
- `python3 book/site/build.py`; `cd experiments && python3 -m unittest` (raise the token cap with
  an `#1` comment if needed).

## Task 6 — scratch verification (not committed)

- nightvote-replay `faultJson` and `/start` body rebuilt with `Json.*`: same JSON modulo
  separators; `Json.float` of NaN/Inf is `null`.
- A page with `Html.unsafe ("<script>var s = " ++ Json.encode (Json.string s) ++ "</script>")`,
  `s` holding `</script>` and U+2028: output has one `</script>`.

## Finish

`go vet ./...`, `go test ./...`, rebase on origin/main, push, PR (with a "Breaking change"
heading for `File.json`).
