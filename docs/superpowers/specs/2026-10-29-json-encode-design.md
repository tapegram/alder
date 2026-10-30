# Json: an encoder for File.json

Design for [tapegram/alder#1](https://github.com/tapegram/alder/issues/1). The gap was found by
Claude while building nightvote-replay, whose `app/Json.alder` hand-builds JSON text (`str` with
four `Text.replace` escapes, `obj`, `arr`, and a `num` that maps NaN and infinities to `null`
itself). The user was not available for this design: everything marked **(call)** is the author's
decision, not the user's.

## Intent

A page that answers JSON should build a value and hand it to `File.json`, and the encoder should
make the three mistakes nightvote-replay had to guard against impossible: an unescaped quote or
control character, a `NaN`/`+Inf` from `Text.fromFloat` landing in the body, and `{`-interpolation
clashes in hand-written object literals. After this change the whole of nightvote-replay's `Json`
module is `Json.string`, `Json.int`, `Json.float`, `Json.object`, `Json.list` and `Json.encode`.

## Goals

1. A stdlib `Json` module with an opaque `Json.Value`, the Elm `Json.Encode` constructors that
   translate to Alder's types, and `encode`.
2. The encoder never emits text that is not JSON (RFC 8259), whatever the inputs.
3. `File.json` takes a `Json.Value`, so the type leads a model to the encoder.
4. `Json.Value` behaves like every other opaque stdlib type under `==`, `Show` (the REPL, `Log`,
   test failures) and as a `Dict` key, and the end-to-end recipe for adding such a type is written
   down here for the next one.

## Non-goals

- Decoding (`Json.decode…`, `Json.Decoder a`). The tree representation below is chosen so a decoder
  can share `Json.Value` later, but nothing of it is built now.
- A raw escape hatch (`Json.raw : Text -> Value`). Pre-encoded text keeps its existing path,
  `File.text { contentType: "application/json", body: t }`; a `raw` would reopen goal 2.
- `Json.dict`, `Json.set`, `Json.array`, `Json.maybe`: `Dict.toList` already yields `object`'s
  record shape, there is no `Set`, and `Maybe.map Json.int m ?? Json.null` is one line.
- A `Fuzz` generator for `Json.Value` (`Fuzz.any` on a record holding one reports the existing
  "cannot derive" error, naming `Json.Value`). A raw string literal form (dev-log theme 7) is a
  separate issue; the encoder removes its JSON case.

## Anti-goals

- A name that means something different from its Elm namesake. Shapes may differ (records for
  tuples, a `List Value` instead of an encoder argument), meanings may not.
- Two ways to answer JSON from a page. `File.json` takes a `Json.Value` and nothing else; there is
  no `File.jsonValue` beside a `File.json : Text` (see "File.json" below).

## The type

`stdlib/Json.alder` declares `type Value` (with a `--` line above it, which the LSP shows on
hover). The checker already turns a parameterless `type` in a stdlib module into
`types.Con{Name: "Value", Module: "Json"}` (`internal/check/decls.go`), and the resolver already
requires the qualified spelling `Json.Value` (a bare `Value` is E0202 with its fix), as for
`Http.HttpError`. No change in either.

**Go representation.** `rt.JsonValue` in a new `rt/json.go`, a tree **(call)**: a kind
(null, bool, int, float, string, array, object) and its payload, with objects as an ordered list
of key/value pairs. Not the pre-encoded text: a tree gives `encodeIndented` without reparsing,
keeps `==` structural, and is what a decoder would produce. `internal/gen/types.go` maps the
`Con` to it with the `Fuzz.Property` pattern: `case "Value": if t.Module == "Json" { return
"rt.JsonValue" }`. The encoder (compact and indented) lives in `rt/json.go` next to the type,
because `rt.Show`, `rt.Eq` and `rt.Key` need it; `rt/std/json.go` holds the thin constructors.

**`==`, Show, Dict keys** **(call)**: `rt.JsonValue` implements `Equaler`, `Keyer` and `Shower`,
all through its compact encoding, exactly as `html.Node` does through its markup:

- `a == b` is `encode a == encode b`. Two values are equal when they are the same JSON. A
  consequence to document: `Json.int 1 == Json.float 1.0` and `Json.float (0.0 / 0.0) ==
  Json.null` are `True`, because that is what they encode to.
- A `Dict (Json.Value) v` keys on the same text, so `==` and `Dict.member` agree (`rt.Key`'s
  contract).
- `Show` prints the compact JSON text as is, unquoted (`{"a":1}`), as Html prints its markup. A
  test failure then reads "got `{"a":1}`, want `{"a":2}`"; a `Text` holding the same JSON shows
  quoted, so the two are distinguishable.

**Recipe for the next opaque stdlib type**, so it is in one place: `type X` in the module's
`.alder` file; a Go type in `rt/` with `AlderEqual`, `AlderKey` and `AlderShow` when field-by-field
reflection is not the right equality or printing; a `GoType` case keyed on name and module; the
functions in `rt/std/` with `StdCalls` rows; a book section. `TestStdCallsCoverStdlib` and
`TestStdCallsExist` fail until the rows and functions exist; nothing else in gen, ir, names or
check needs to know the type.

## The API

Signatures are the exact lines of `stdlib/Json.alder`. Each function needs a `"Json.name":
"std.JsonName"` row in `internal/gen/stdcall.go` and a Go function in `rt/std/json.go`
(`null` is a Go function of no parameters, like `Html.none`).

```alder
module Json

-- a JSON value: build it with the functions below, write it with encode or File.json
type Value

null   : Value
bool   : Bool -> Value
int    : Int -> Value
-- NaN, +Inf and -Inf encode as null
float  : Float -> Value
string : Text -> Value
list   : List Value -> Value
-- keys in list order; a repeated key keeps its first position and its last value
object : List { key: Text, value: Value } -> Value
-- compact: no whitespace
encode : Value -> Text
-- n spaces per level; n <= 0 is encode
encodeIndented : Int -> Value -> Text
```

**Module name** `Json`, not `Json.Encode` **(call)**. Stdlib modules are one segment and always in
scope; `Json.Encode.string` would be the only three-segment stdlib name, and nightvote-replay
reached for `Json.str`/`Json.obj` unprompted. Elm splits `Json.Encode`/`Json.Decode` because each is
a module of its own with a shared `Value`; here a decoder adds `Json.Decoder a` and `Json.decode…`
to the same module. An app module named `Json` is refused by the existing E0017 ("is a
standard-library module; rename it"), so nightvote-replay deletes its own on upgrade.

**`list : List Value -> Value`** **(call)**, not Elm's `(a -> Value) -> List a -> Value`: the
pipeline `xs |> List.map Json.int |> Json.list` is the Alder spelling, and `object` takes a list of
values the same way; one shape for both.

**`object` takes `List { key: Text, value: Value }`** — `Dict.toList`'s record, so a
`Dict Text Value` is `d |> Dict.toList |> Json.object`, and a field is written as a record literal
`{ key: "n", value: Json.int n }`. No `Json.field` helper **(call)**: it would be a second way to
write the record.

### Numbers

- `int` writes the decimal digits of the `Int` **(call)**, however large: JSON has no integer
  range, and a consumer that parses into a double loses precision above 2^53 on its own side. Not
  quoted.
- `float` writes `strconv.FormatFloat(f, 'f', -1, 64)`: the shortest text that reads back as the
  same number, no exponent, so `Json.float 1.0` is `1` and `Json.float 0.1` is `0.1` **(call)**.
  This is `Text.fromFloat`'s format and Go's `encoding/json`'s; one float spelling in the
  language. Negative zero writes `-0`.
- **Non-finite floats encode as `null`** **(call)** — the one decision the issue asked for. This
  is Elm's and `JSON.stringify`'s behaviour and what nightvote-replay chose. The alternatives are
  worse for the language: a `Maybe Value` result makes every float call site a `??`, and a runtime
  failure breaks the stdlib's rule that nothing panics (slices clamp, `Bytes.fromList` masks).
  `Float.isNaN`/`Float.isInfinite` exist for the caller who wants something else.

### Objects

- **Key order is list order** **(call)**. Not sorted: the author controls the output, `Dict.toList`
  is insertion order already, and Elm and JS do the same. Deterministic either way.
- **A duplicate key keeps its first position and takes its last value** **(call)**, resolved when
  `object` builds the value (so `==` and `Show` see one key). This is `Dict.insert`'s rule and the
  JS object-literal rule Elm inherits; emitting both copies would be JSON whose meaning depends on
  the parser (RFC 8259 §4), and refusing would need a `Maybe` or a panic.

### Strings and escaping

`string` writes `"` + escaped text + `"`. Escaped **(call)**:

| input | output |
|---|---|
| `"` `\` | `\"` `\\` |
| U+0008, U+0009, U+000A, U+000C, U+000D | `\b` `\t` `\n` `\f` `\r` |
| other U+0000–U+001F | `\u00XX` (lower-case hex) |
| `<` `>` `&` | `<` `>` `&` |
| U+2028, U+2029 | ` ` ` ` |

Everything else, including every other non-ASCII character, is written as UTF-8 as is; `/` and
U+007F are not escaped. The `<`/`>`/`&`/U+2028/U+2029 rows are the HTML-and-JS safety rows: a
`Json.encode` interpolated into `Html.unsafe "<script>…</script>"` or a
`<script type="application/json">` data island can then never close the script element or break
a JS string literal, and a parser reads the same text either way. One encoder, always safe, rather
than an `encodeForHtml` the model has to know to pick **(call)**; it is exactly Go's
`encoding/json` default. Alder `Text` is always valid UTF-8 (`Bytes.toText` is `Maybe`,
`Text.fromCodePoints` substitutes U+FFFD), so the encoder needs no invalid-byte path; the builder
adds one `utf8.ValidString` guard that writes U+FFFD for a byte that could not be a character.

### Output formatting

- `encode` is compact: `{"a":1,"b":[true,null,"x"]}`, no spaces anywhere, no trailing newline
  **(call)**. This is the wire format `File.json` wants and what `JSON.stringify`, Elm's
  `encode 0` and Go write.
- `encodeIndented n` is the JS/Elm pretty form with `n` spaces per level: each array item and
  object member on its own line, `"key": value` with one space after the colon, closing bracket
  on its own line at the parent's indent, empty containers as `[]` and `{}`, no trailing newline.
  `n <= 0` gives `encode`'s output **(call)**, as `JSON.stringify(v, null, 0)` and Elm's `encode 0`
  do; it is the one stdlib count that does not just clamp to zero, and the signature line says so.

### File.json

```alder
json : Json.Value -> File   -- application/json
```

**The type changes; no `File.jsonValue`** **(call)**. The type is the teaching surface of this
language: with `File.json : Text` still there and shorter, a model keeps hand-building text, which
is the behaviour #1 was filed about. `File.json t` was only sugar for
`File.text { contentType: "application/json", body: t }`, and that spelling stays for text that is
already JSON (a capability returning JSON bytes, a fixture). The break is one mechanical rewrite
per call site, reported as the ordinary mismatch (`Text` where `Json.Value` is expected). In this
repo that is two corpus files and the two book examples, which the builder updates; nightvote-replay
pins its own Alder build and has to delete its `Json` module on upgrade anyway, so its migration is
one commit. `std.FileJson` becomes `func FileJson(v rt.JsonValue) rt.File`, body `[]byte(encode)`.

## Where things change

| layer | files |
|---|---|
| signatures | new `stdlib/Json.alder`; `stdlib/File.alder` (`json`'s line) |
| name → Go mapping | `internal/gen/stdcall.go` (a `// Json` group); `internal/gen/types.go` `GoType` case |
| runtime type + encoder | new `rt/json.go`: `JsonValue`, `AlderEqual`/`AlderKey`/`AlderShow`, compact and indented encoding, escaping |
| stdlib functions | new `rt/std/json.go`; `rt/std/file.go` (`FileJson`) |
| unit tests | `rt/std/std_test.go`: one `eq(...)` per row of the escaping table and per behaviour named here (non-finite floats, `-0`, `1.0`, large ints, key order, duplicate keys, empty containers, `encodeIndented 2` and `0`, `==`/Dict-key/Show agreement); `rt/show_test.go` for Show |
| corpus | `testdata/corpus/file-ok/app/Slow.alder`, `testdata/corpus/E0034-file-page-msg/app/Feed.alder` (`File.json (Json.int …)`) |
| book | `book/09-stdlib.md`: a `## Json` section before "File, Bytes" with the signature block and one sentence each for `null` floats, key order/duplicates, the HTML-safe escapes and `encodeIndented 0`; `File.json`'s line and the sentence in `book/06-pages.md`; the examples in `book/07-effects.md` and `book/10-testing.md` |
| site | `book/site/build.py` after the book edit |

Also: tick #1 in the roadmap issue #52, and the user reinstalls (`go install ./cmd/alder`) after
merge for LSP completion of the new module.

## Left to the builder

- The Go shape of the tree (a tagged struct, or an interface with one type per kind). Match
  `rt.Nav`'s tagged-struct style unless the interface form is clearly shorter.
- Whether `Json.string`'s escaping is a hand-written loop or `encoding/json`'s string encoder
  (`json.Marshal` of a string gives exactly the table above on this repo's Go, U+FFFD
  substitution included). Either is fine; the tests are the contract.
- Wording of the book sentences and of the `--` doc lines.

## Verification

- `go test ./...` green, including the new `rt` and `rt/std` cases, the corpus, and
  `TestStdCallsCoverStdlib`.
- A scratch check: nightvote-replay's `faultJson` and the `/start` body rewritten with `Json.*`
  encode to the same JSON (modulo the hand version's `", "`/`": "` separators and two-decimal
  `num`), and its `num refuses NaN and infinity` test holds for `Json.float`. A scratch file, not
  a change to nightvote-replay.
- A scratch page that puts `Json.encode (Json.string s)`, with `s` holding `</script>` and
  U+2028 (`Text.fromCodePoints [8232]`), inside a `<script>` through `Html.unsafe`; the browser
  must see one script and one string.
