# 9. Standard library

Every module here is always in scope. This is the complete list; a function
not listed here does not exist.

## List

```alder
List.length   : List a -> Int
List.isEmpty  : List a -> Bool
List.head     : List a -> Maybe a
List.last     : List a -> Maybe a
List.take     : Int -> List a -> List a
List.drop     : Int -> List a -> List a
List.reverse  : List a -> List a
List.map      : (a -> b) -> List a -> List b
List.filter   : (a -> Bool) -> List a -> List a
List.filterMap : (a -> Maybe b) -> List a -> List b
List.find     : (a -> Bool) -> List a -> Maybe a
List.any      : (a -> Bool) -> List a -> Bool
List.all      : (a -> Bool) -> List a -> Bool
List.member   : a -> List a -> Bool
List.foldl    : (a -> b -> b) -> b -> List a -> b
List.sum      : List Int -> Int
List.sumFloat : List Float -> Float
List.sortBy   : (a -> Int) -> List a -> List a      -- stable
List.sortByText : (a -> Text) -> List a -> List a   -- stable
List.indexed  : List a -> List { index: Int, item: a }
List.range    : Int -> Int -> List Int        -- inclusive
List.concat   : List (List a) -> List a
List.append   : List a -> List a -> List a    -- same as ++
List.unique   : List a -> List a
List.groupBy  : (a -> Text) -> List a -> Dict Text (List a)
List.indexedMap : (Int -> a -> b) -> List a -> List b
List.map2     : (a -> b -> c) -> List a -> List b -> List c
List.map3     : (a -> b -> c -> d) -> List a -> List b -> List c -> List d
List.foldr    : (a -> b -> b) -> b -> List a -> b
List.concatMap : (a -> List b) -> List a -> List b
List.intersperse : a -> List a -> List a
List.partition : (a -> Bool) -> List a -> { yes: List a, no: List a }
List.tail     : List a -> Maybe (List a)
List.repeat   : Int -> a -> List a
List.maximum  : List Int -> Maybe Int
List.minimum  : List Int -> Maybe Int
List.maximumFloat : List Float -> Maybe Float
List.minimumFloat : List Float -> Maybe Float
List.sortByFloat : (a -> Float) -> List a -> List a
```

`List.partition` returns a record with a `yes` field (the elements that satisfy the test) and a `no` field, each in input order. `List.map2` and `List.map3` stop at the shortest list.

`List.take`, `List.drop` and `List.tail` cost nothing however long the list:
the result shares the original's elements, which is safe because no list is
ever changed. `++` copies both sides into a new list (unless one is empty), so
a loop that grows a list with `acc ++ [x]` copies the whole list at every
step; use `List.foldl`, `List.concatMap` or, for text, a `Text.Builder`
(below) instead.

```alder eval=[10, 30, 50]
List.range 1 5
  |> List.filter (\n -> n % 2 == 1)
  |> List.map (\n -> n * 10)
```

```alder eval=[{ name: "a", age: 1 }, { name: "c", age: 1 }, { name: "b", age: 2 }]
List.sortBy (\p -> p.age) [{ name: "b", age: 2 }, { name: "a", age: 1 }, { name: "c", age: 1 }]
```

## Maybe

```alder
Maybe.map      : (a -> b) -> Maybe a -> Maybe b
Maybe.andThen  : (a -> Maybe b) -> Maybe a -> Maybe b
Maybe.withDefault : a -> Maybe a -> a          -- same as ??
Maybe.isJust   : Maybe a -> Bool
Maybe.toList   : Maybe a -> List a
```

```alder eval=42
Maybe.withDefault 0 (Text.toInt "42")
```

## Result

```alder
Result.map     : (a -> b) -> Result e a -> Result e b
Result.mapErr  : (e -> f) -> Result e a -> Result f a
Result.andThen : (a -> Result e b) -> Result e a -> Result e b
Result.withDefault : a -> Result e a -> a
Result.toMaybe : Result e a -> Maybe a
```

## Text

```alder
Text.length    : Text -> Int
Text.isEmpty   : Text -> Bool
Text.trim      : Text -> Text
Text.toUpper   : Text -> Text
Text.toLower   : Text -> Text
Text.contains  : Text -> Text -> Bool        -- Text.contains needle haystack
Text.startsWith : Text -> Text -> Bool
Text.endsWith  : Text -> Text -> Bool
Text.replace   : Text -> Text -> Text -> Text -- Text.replace old new text
Text.split     : Text -> Text -> List Text   -- Text.split sep text
Text.join      : Text -> List Text -> Text
Text.slice     : Int -> Int -> Text -> Text
Text.matches   : Text -> Text -> Bool        -- Text.matches regex text
Text.fromInt   : Int -> Text
Text.fromFloat : Float -> Text
Text.toInt     : Text -> Maybe Int
Text.toFloat   : Text -> Maybe Float
Text.toBytes   : Text -> Bytes               -- the UTF-8 bytes (chapter 9, "File, Bytes")
Text.padLeft   : Int -> Text -> Text -> Text -- Text.padLeft 2 "0" "7" == "07"
Text.padRight  : Int -> Text -> Text -> Text
Text.left      : Int -> Text -> Text         -- first n code points
Text.right     : Int -> Text -> Text         -- last n code points
Text.dropLeft  : Int -> Text -> Text
Text.dropRight : Int -> Text -> Text
Text.repeat    : Int -> Text -> Text
Text.reverse   : Text -> Text
Text.trimLeft  : Text -> Text
Text.trimRight : Text -> Text
Text.concat    : List Text -> Text           -- Text.join ""
Text.lines     : Text -> List Text
Text.words     : Text -> List Text
Text.indexes   : Text -> Text -> List Int    -- Text.indexes needle haystack
Text.toCodePoints : Text -> List Int
Text.fromCodePoints : List Int -> Text
Text.NonEmpty, Text.Email, Text.Url, Text.Secret   -- refinement brands
```

```alder eval=["a", "b", "c"]
Text.split ", " "a, b, c"
```

```alder eval=True
Text.contains "ell" "hello"
```

`Text.length` counts code points (`Text.length "é"` is 1), and
`Text.slice start end` takes the code points from `start` up to, not
including, `end`. A negative index counts from the end (`Text.slice (-3) (-1)
"abcdef"` is `"de"`; a negative argument needs its parentheses), indices past either end are clamped, and a `start` at
or after `end` gives `""`. `Text.matches regex text` is true when the
regex (RE2 syntax) matches anywhere in the text: write `^…$` to match the
whole text. An invalid regex matches nothing. `Text.fromFloat` writes the
shortest decimal that reads back as the same number, never an exponent
(`2.0` is `"2"`, `3.5` is `"3.5"`); infinity is `"+Inf"` or `"-Inf"` and
not-a-number is `"NaN"`. `Text.toInt` and `Text.toFloat` give `Nothing`
for anything else than a plain decimal number (spaces, overflow, `"NaN"`).

`Text.padLeft n pad s` makes `s` at least `n` code points long: a pad of
several characters is repeated and cut at the outer edge, so
`Text.padLeft 5 "ab" "x"` is `"ababx"`, and `s` itself is never cut.
`Text.lines` splits on `\n`, removes a `\r` before it (so `\r\n` text works)
and drops the empty line after a final newline: `Text.lines "a\nb\n"` is
`["a", "b"]`. `Text.indexes needle haystack` lists every code-point offset
where `needle` starts, overlapping matches included (`Text.indexes "aa" "aaa"`
is `[0, 1]`). There is no `toChars`: `Text.split "" s` gives the characters.

```alder eval=["de", "2", "3.5"]
[Text.slice (-3) (-1) "abcdef", Text.fromFloat 2.0, Text.fromFloat 3.5]
```

### Building a large Text

```alder
Text.Builder                                 -- an accumulator for a large Text
Text.builder   : Text.Builder                -- empty
Text.add       : Text -> Text.Builder -> Text.Builder   -- Text.add piece b
Text.build     : Text.Builder -> Text
```

Interpolation and `Text.join` each make a new Text, copying every byte they
are given. That is cheap for a heading, but a page that renders a large body
by nesting them (rows joined into options, options into items, items into a
file) copies each byte once per level: a 100 MB JSON file allocates close to
a gigabyte. A `Text.Builder` avoids that. `Text.add` puts a piece after
everything added so far without copying anything, and `Text.build` copies each
byte once, into the finished Text. A builder is a value like any other:
adding to it gives a new builder and leaves the old one as it was, so two
builders can grow from the same start. Make each row with interpolation and
add whole rows; adding many tiny pieces (a comma, a field) costs more than
the copy it saves. There is no `addAll`: `List.foldl Text.add b pieces` adds a
list of pieces in order.

```alder eval="name,votes\nKent,3\nDane,5\n"
[{ name: "Kent", votes: 3 }, { name: "Dane", votes: 5 }]
  |> List.foldl (\r b -> b |> Text.add "{r.name},{Text.fromInt r.votes}\n") (Text.builder |> Text.add "name,votes\n")
  |> Text.build
```

Under the hood a builder is a chain of the pieces, newest first, each link
remembering the total length so far. `Text.build` allocates that many bytes
once and fills them from the end, walking the chain back. A File page passes
the result straight to `File.text`, which uses the Text's bytes as the body
without copying them again.

## Int, Float

```alder
Int.toFloat  : Int -> Float
Int.min      : Int -> Int -> Int
Int.max      : Int -> Int -> Int
Int.abs      : Int -> Int
Int.clamp    : { min: Int, max: Int } -> Int -> Int
Int.pow      : Int -> Int -> Int                -- Int.pow base exponent
Int.Positive, Int.NonNegative                -- refinement brands

Float.round  : Float -> Int
Float.floor  : Float -> Int
Float.ceil   : Float -> Int
Float.min, Float.max, Float.abs
Float.format : { decimals: Int } -> Float -> Text
Float.sqrt   : Float -> Float
Float.exp    : Float -> Float
Float.log    : Float -> Float                   -- natural logarithm
Float.pow    : Float -> Float -> Float          -- Float.pow base exponent
Float.truncate : Float -> Int                   -- rounds toward zero
Float.clamp  : { min: Float, max: Float } -> Float -> Float
Float.pi, Float.e                               -- Float constants
Float.isNaN, Float.isInfinite : Float -> Bool
Float.logBase : Float -> Float -> Float         -- Float.logBase base x
Float.sin, Float.cos, Float.tan, Float.asin, Float.acos, Float.atan : Float -> Float
Float.atan2  : Float -> Float -> Float          -- Float.atan2 y x
```

`Int` arithmetic wraps on overflow; `/` truncates toward zero and `%` has
the dividend's sign; an `Int` division or remainder by zero is a runtime
failure (chapter 4). `Float` division by zero gives infinity or NaN.

`Int.pow` wraps on overflow like `*`, and a negative exponent gives `0`
(`Int.pow 2 (-1)` is `0`), never a failure. `Float.clamp` takes the same
record as `Int.clamp` and checks in the same order, so `min` above `max`
behaves as it does there. `Float.truncate` rounds toward zero.

A `Float` literal may carry an exponent: `1.5e-3`, `1e6` and `2E+3`. Any
literal with an exponent is a `Float`, including `1e6`. `Text.toFloat` reads
the same form, and `Text.fromFloat` output is unchanged. An `Int` literal
that does not fit in 64 bits (above `9223372036854775807`) is an error, and a
`Float` literal that overflows to infinity is one too; the smallest `Int` is
written `-9223372036854775808` (the `-` directly on the literal, not `-(…)`).

`Float.round` rounds half away from zero: `Float.round 2.5` is `3` and
`Float.round (-2.5)` is `-3`. `Float.format { decimals: n }` rounds the same
way to `n` decimals and pads with zeros (`Float.format { decimals: 2 }
1.005` is `"1.01"`, and `1.5` gives `"1.50"`); it never writes `"-0"`
(`-0.001` with one decimal is `"0.0"`).

```alder eval=[3, -3, -3, -1]
[Float.round 2.5, Float.round (-2.5), -7 / 2, -7 % 2]
```

```alder eval=["1.01", "1.50", "0.0"]
[Float.format { decimals: 2 } 1.005, Float.format { decimals: 2 } 1.5, Float.format { decimals: 1 } (-0.001)]
```

`Float.sqrt`, `Float.exp`, `Float.log` and `Float.pow` are Go's `math`
functions: they follow IEEE rules, so `Float.sqrt (-1.0)` and `Float.log (-1.0)`
are NaN and `Float.log 0.0` is negative infinity, not failures.

```alder eval=[3.0, 8.0]
[Float.sqrt 9.0, Float.pow 2.0 3.0]
```

## Bool

```alder
not : Bool -> Bool
Bool.toText : Bool -> Text
```

`Bool.toText` writes the constructor names, `"True"` and `"False"`.

## Dict

```alder
Dict.empty    : Dict k v
Dict.fromList : List { key: k, value: v } -> Dict k v
Dict.toList   : Dict k v -> List { key: k, value: v }
Dict.get      : k -> Dict k v -> Maybe v
Dict.insert   : k -> v -> Dict k v -> Dict k v
Dict.remove   : k -> Dict k v -> Dict k v
Dict.keys     : Dict k v -> List k
Dict.values   : Dict k v -> List v
Dict.size     : Dict k v -> Int
Dict.singleton : k -> v -> Dict k v
Dict.isEmpty  : Dict k v -> Bool
Dict.member   : k -> Dict k v -> Bool
Dict.update   : k -> (Maybe v -> Maybe v) -> Dict k v -> Dict k v
Dict.map      : (k -> v -> w) -> Dict k v -> Dict k w
Dict.filter   : (k -> v -> Bool) -> Dict k v -> Dict k v
Dict.foldl    : (k -> v -> b -> b) -> b -> Dict k v -> b
Dict.partition : (k -> v -> Bool) -> Dict k v -> { yes: Dict k v, no: Dict k v }
Dict.union    : Dict k v -> Dict k v -> Dict k v
Dict.intersect : Dict k v -> Dict k v -> Dict k v
Dict.diff     : Dict k v -> Dict k v -> Dict k v
```

`Dict.update` calls the function with `Just` the current value, or `Nothing`
when the key is absent: `Just` keeps the key's position (a new key is
appended) and `Nothing` removes it. `Dict.union a b` prefers the first dict's
value and order, then appends the keys only `b` has. `map`, `filter`,
`partition`, `intersect` and `diff` keep key order; `foldl` walks it.

A key may be of any type `==` works on: a `List`, a record, a `Time`, but
not a function or a type containing one. Two keys are the
same key when they are `==`, so a `Time` key is its instant (the same
moment written in two time zones is one key), and `Html` keys are
compared by their markup (`<p>ab</p>` and
`<p>{Html.text "a"}{Html.text "b"}</p>` are one key). A `Dict` keeps its keys in
the order they were first inserted; `==` on two dicts ignores that order.

```alder eval=Just 2
Dict.fromList [{ key: "a", value: 1 }]
  |> Dict.insert "b" 2
  |> Dict.get "b"
```

## Date, Time

`Date` is a calendar date. `Time` is an instant (UTC).

```alder
Date.today     : Query Date
Date.fromText  : Text -> Maybe Date             -- "2026-10-08"
Date.toText    : Date -> Text                   -- "2026-10-08"
Date.format    : Text -> Date -> Text           -- Date.format "Jan 2, 2006" d
Date.addDays   : Int -> Date -> Date
Date.diffDays  : Date -> Date -> Int
Date.year, Date.month, Date.day : Date -> Int
Date.fromParts : { year: Int, month: Int, day: Int } -> Maybe Date
Date.toParts   : Date -> { year: Int, month: Int, day: Int }
Date.epoch     : Date                           -- 1970-01-01
Date.weekday   : Date -> Int                    -- 1 = Monday ... 7 = Sunday
Date.addMonths : Int -> Date -> Date
Date.addYears  : Int -> Date -> Date

Time.now       : Query Time
Time.toDate    : Time -> Date
Time.fromDate  : Date -> Time                   -- midnight UTC
Time.format    : Text -> Time -> Text
Time.ago       : Time -> Time -> Text           -- Time.ago now then = "3 hours ago"
Time.toMillis  : Time -> Int                    -- milliseconds since 1970-01-01 00:00 UTC
Time.fromMillis : Int -> Time
Time.addMillis : Int -> Time -> Time
Time.diffMillis : Time -> Time -> Int           -- Time.diffMillis a b = b − a, in milliseconds
```

`Date.diffDays a b` is the number of days from `a` to `b`, that is `b − a`:
`Date.diffDays monday wednesday` is `2`, and it is negative when `b` comes
first.

`Date.fromParts` is `Nothing` for an impossible date (month 13, Feb 30); it
never rolls over into the next month. `Date.weekday` is ISO-numbered, so
Monday is `1` and Sunday is `7`, and `weekday d >= 6` means weekend.
`Date.addMonths` clamps the day to the target month's length: one month after
January 31 is February 28 (29 in a leap year). `Date.addYears 1` on February 29
gives February 28.

```alder eval=Just "2026-10-09"
Date.fromText "2026-10-08"
  |> Maybe.map (Date.addDays 1)
  |> Maybe.map Date.toText
```

## Html

```alder
Html.unsafe : Text -> Html Never                -- unescaped markup; never for user text
Html.none  : Html a                             -- renders nothing
Html.text  : Text -> Html a
Html.fragment : List (Html a) -> Html a         -- the nodes side by side, no wrapper element
Html.toText : Html Never -> Text                -- the markup File.html serves
Url.external : Text -> Page                     -- external URL for href, src or Redirect
```

`Html.toText` takes `Html Never`, a view that sends no messages, because the
markup is rendered without a running page to receive them.

`Url.external` keeps a URL only when it is an absolute `http:` or `https:`
URL (`https://example.com/a`), a path of this site (`/static/app.css`,
but not `//host/…` or `/\host`, which browsers read as another site), or a
fragment (`#top`). Anything else becomes `#`, a link and a redirect that go
nowhere: `javascript:` and `data:` URLs in any case (`JavaScript:`), other
schemes (`mailto:`), URLs with leading spaces, relative paths, and any URL
holding a control character (browsers drop tabs and newlines, so `/\t/evil`
would be `//evil`). So a link or a
`Redirect` built from user input can never run script. The runtime checks
again: an `href` that is not such a URL renders as `#`, and a redirect to
one is answered with a 500 (`refusing to navigate to javascript:`).

A `Page` compares by its address: `Todos.Index {} == Todos.Index
{ filter: All }`, since a defaulted parameter is left out of the URL.

`Time.toMillis` counts milliseconds from the epoch, the instant
`Time.epoch`, and `Time.fromMillis` goes back. `Time.diffMillis a b` is
`b − a`, like `Date.diffDays`: negative when `b` is earlier.

```alder eval=0
Time.toMillis Time.epoch
```

## File, Bytes

A file page's `view` returns a `File` (chapter 6, "File pages"). `Bytes` is
the type of a body that is not text (chapter 7, "A capability returning
bytes"); it has the few operations below, and a
`Text` becomes `Bytes` with `Text.toBytes`.

```alder
File.text       : { contentType: Text, body: Text } -> File      -- a 200 answer
File.bytes      : { contentType: Text, body: Bytes } -> File     -- a 200 answer
File.html       : Html Never -> File                             -- text/html; charset=utf-8, rendered verbatim: no doctype unless the node has one
File.json       : Text -> File                                   -- application/json
File.withStatus : Int -> File -> File                            -- 200..599
File.withHeader : { name: Text, value: Text } -> File -> File    -- Content-Disposition, Cache-Control, ETag, Last-Modified, Retry-After, Content-Language, X-…
File.status      : File -> Int
File.contentType : File -> Text
File.body        : File -> Bytes

Bytes.length    : Bytes -> Int                                   -- the number of bytes
Bytes.toText    : Bytes -> Maybe Text                            -- Just when the bytes are valid UTF-8
Bytes.empty     : Bytes
Bytes.isEmpty   : Bytes -> Bool
Bytes.slice     : Int -> Int -> Bytes -> Bytes                   -- same index rules as Text.slice, in bytes
Bytes.append    : Bytes -> Bytes -> Bytes
Bytes.concat    : List Bytes -> Bytes
Bytes.toList    : Bytes -> List Int                              -- each 0..255
Bytes.fromList  : List Int -> Bytes                              -- the low eight bits of each Int
```

`Bytes.length` counts bytes, not letters: `é` is two bytes in UTF-8.
`Bytes.slice` follows `Text.slice`'s rule, counting bytes: negative indices
count from the end, indices clamp, and `start >= end` gives `Bytes.empty`.
`Bytes.fromList` keeps the low eight bits of each `Int`, so
`Bytes.fromList [256]` is `Bytes.fromList [0]`.

```alder eval=6
Bytes.length (Text.toBytes "héllo")
```

## Query, Task, Db, Session, Log, Time, Random, Http

See chapters 7 and 8. The column methods, written on a column in place
(`Todo.title.contains q`), are not functions of a module; the compiler
knows them:

| Method | Columns | Type (for a column of type `a` in table `t`) |
|---|---|---|
| `.eq`, `.ne` | any | `a -> Condition t` |
| `.lt`, `.le`, `.gt`, `.ge` | ordered: `Int`, `Float`, `Text`, `Date`, `Time`, brands, unions without payloads | `a -> Condition t` |
| `.contains` | `Text` and its brands | `Text -> Condition t` (substring, ignores A–Z case) |
| `.startsWith` | `Text` and its brands | `Text -> Condition t` (prefix, ignores A–Z case) |
| `.in` | any | `List a -> Condition t` |
| `.isNothing`, `.isJust` | `Maybe` | `Condition t` |
| `.asc`, `.desc` | any | `Order t` |

The scoped operations of `Db`, for a table with an `@owner` column, used
only in a page module whose page `requires user` (chapter 6, "Rules";
chapter 8, "Your own rows"). Each acts for the signed-in user of the
running request or test, and without one fails with "`Db.mine Todo: no
signed-in user`" (the operation and table as written):

| Function | Type | |
|---|---|---|
| `Db.mine` | `Table t -> Select t` | the rows whose owner is the user; a `Select` like `Db.from t` |
| `Db.getMine` | `Table t -> Id t -> Query t` | the user's row; not found when it is missing or someone else's |
| `Db.updateMine` | `Table t -> Id t -> (t -> t) -> Task Unit` | changes the user's row; not found otherwise; never changes the owner |
| `Db.deleteMine` | `Table t -> Id t -> Task Unit` | deletes the user's row; not found otherwise |
| `Db.insertMine` | `Table t -> Insert t -> Task (Id t)` | inserts a row owned by the user; the row is a record literal in the call, without the owner column |

The same operations for any partition key (M15): a table with a
`@partition k` column (`@owner` is `@partition user`), used only where
the session's `k` is narrowed, in a page module whose page `requires k`
(chapter 6, "Partitions"). The `mine` family above is these with the key
`user`, written for tables partitioned by `user`:

| Function | Type | |
|---|---|---|
| `Db.scoped` | `Table t -> Select t` | the rows of the session's partition; a `Select` like `Db.from t` |
| `Db.getScoped` | `Table t -> Id t -> Query t` | the row in the session's partition; not found when it is missing or in another |
| `Db.updateScoped` | `Table t -> Id t -> (t -> t) -> Task Unit` | changes the row in the partition; not found otherwise; never changes the partition column |
| `Db.deleteScoped` | `Table t -> Id t -> Task Unit` | deletes the row in the partition; not found otherwise |
| `Db.insertScoped` | `Table t -> Insert t -> Task (Id t)` | inserts a row in the session's partition; the row is a record literal in the call, without the partition column |

Run without the key (a test whose session has no `org`), each fails with
the operation and table as written: "Db.insertScoped Project: the session
has no org". On a table in a `silo` (chapter 8, "Silos") they read and
write the org's own database, and so do `Db.from`, `Db.get`,
`Db.update`, `Db.delete` and `Db.insert` on it, which need the key too.

Waiting (chapter 7, "Waiting: `Query.delay` and `Task.sleep`"):

```alder
Query.delay : Int -> Query Unit                 -- milliseconds, clamped to 0..60000
Task.sleep  : Int -> Task Unit                  -- the same, in a Task
```

`Query.notFound` and `Task.notFound` (both
`: Query a` / `: Task a`, no arguments) stop the effect and answer 404 with
`App.notFound`; a `Task` that ends in `Task.notFound` is rolled back like
any failed `Task`.

## Test

Only a `test` or a test helper may use these (chapter 10, "Test
helpers": a value that uses one becomes a helper); where no test can
reach, such as a page's `view`, each is E0040 "… is only available in a
`test` or a test helper".

- `Test.run : Task a -> Task { value: Result Text a, trace: List Text }`
  runs a task that may fail: `value` is `Ok` with its result or `Err`
  with the failure's message (its writes undone), and `trace` is the
  steps it took (chapter 10, section 4).
- `Test.trace : Task (List Text)` is the steps the test has taken so far
  (chapter 10, section 5).
- `Test.fake Stripe : Task Unit` installs the `fake` of capability
  `Stripe` for the rest of the test (chapter 10, section 8). It is a
  special form, not a function: its argument is a capability's name
  (`Stripe` or `Payments.Stripe`), whose module declares a `fake`, and
  never an expression. It has no type of its own to pass around, so
  `Test.fake` cannot be applied to a variable, piped into, or left
  without its argument.
- `Test.member : u -> k -> Task Unit` (M15) inserts a row of the session
  key's `via` table that makes user `u` a member of `k`
  (`Test.member ann acme`): the user and key columns set, a `Maybe`
  column `Nothing`, a column with a default its default. A `via` table
  with another required column is E0003 ("`Test.member` fills only the
  user and the org of a `Membership`: `Membership.since` has no default;
  insert the row with `Db.insert Membership { … }`"), and so is an app
  without a `via` field (chapter 10, "Two orgs").
- `Test.setClock : Time -> Task Unit` sets the test's clock to a time:
  `Time.now`, `Date.today` and `@created` read it, and it stands still
  (no one-second tick) until something moves it (chapter 10, "Setting the clock").
- `Test.advance : Int -> Task Unit` moves the test's clock that many
  milliseconds forward (a negative number is 0), set or ticking.

### Browser steps and queries

The browser steps (chapter 10, section 9) are special forms too: each is
applied directly to all its arguments, never passed as a value or piped
into, and `get`, `open`, `fetch`, `submit`, `submitRaw` and `click` need an `App`
module. `Session` here is `App`'s session record.

```text
Test.login     : Session -> Task Unit
Test.get       : Page -> Task { model: Model, session: Session, doc: Doc }
Test.open      : Page -> Task { answer: Answer, session: Session, doc: Doc }
Test.submit    : Form -> record -> Task { answer: Answer, session: Session, doc: Doc }
Test.submitRaw : Form -> List { name: Text, value: Text } -> Task { answer: Answer, session: Session, doc: Doc }
Test.click     : Msg -> Task { answer: Answer, session: Session, doc: Doc }
Test.fetch     : Page -> Task { status: Int, contentType: Text, body: Bytes, headers: List { name: Text, value: Text } }
```

- `Test.login s` sets the browser's session cookie, through the rules a
  served request follows: a `via` key (`org: Just acme`) needs its
  membership row first (`Test.member`), as `Session.put` would.
- `Test.get p` and `Test.open p` take a page value written in place
  (`Todos.Index {}`, `Todos.Show { id: id }`); `model` is the `Model`
  of `p`'s page, which must be a record. `Test.get` fails the test
  unless the page is shown.
- `Test.submit F r` takes a form of a page or of `App` (`NewTodo` in its
  module, `Todos.Index.NewTodo` elsewhere) and that form's record; a
  form each row renders, or one two messages post, is written as its
  message with its parameters, `Test.submit (Rename { id: b }) { … }`.
  `Test.submitRaw` takes the same first argument and text fields.
- `Test.click m` takes a message of a page or of `App` that carries no
  form, with its parameters: `Toggle { id: id }`, `App.Logout`.
- `Test.fetch p` takes a page value written in place, like `Test.get`,
  and gives the raw answer: its `status`, `contentType`, `body` as
  `Bytes` and `headers` (sorted by name). It does not follow a redirect,
  and it is the step for a file page; `Test.get` and `Test.open` on a
  file page are checker errors (chapter 10, "`Test.fetch`: requesting a
  file").

Two types of the Prelude exist only in tests and test helpers (in a
type declaration, E0003 "`Doc` is a test type"; in a page's `view` and
the other values that can never be helpers, E0040): `Doc`, a parsed page, which has `==`, and

```text
type Answer = Shown | Rerendered | Redirected Page | FormRejected (List { field: Text, message: Text }) | PageMissing | RequestFailed Text
```

Their names, and `Answer`'s six constructors, are taken in every module
(E0012). The queries are ordinary functions on a `Doc`, available only
in tests like the rest of `Test`:

```text
Test.text  : Text -> Doc -> Maybe Text           -- the first match's text
Test.texts : Text -> Doc -> List Text            -- every match's text, in page order
Test.has   : Text -> Doc -> Bool
Test.count : Text -> Doc -> Int
Test.attr  : Text -> Text -> Doc -> Maybe Text   -- attribute name, then selector: the first match's
Test.doc   : Html a -> Doc                       -- a view rendered and parsed
```

A selector is a tag, `#id`, `.class`, `[attr]`, `[attr=value]`, or
several of those together (`li.done`), combined with spaces (inside) or
`>` (directly inside). Written as a literal it is checked when the test
compiles (E0023); any other selector that does not parse matches
nothing.

## Fuzz

The generators of property tests (chapter 10, section 12). Only a `test` body
may be a `Fuzz.Property`; a `Fuzz.Fuzzer a`, a description of how to draw
an `a` from random choices, may be a value anywhere, so a generator is
written once, with a signature, and shared.

```text
Fuzz.forAll     : Fuzz.Fuzzer a -> (a -> Bool) -> Fuzz.Property
Fuzz.forAllTask : Fuzz.Fuzzer a -> (a -> Task Bool) -> Fuzz.Property
Fuzz.any        : Fuzz.Fuzzer a
Fuzz.constant   : a -> Fuzz.Fuzzer a
Fuzz.bool       : Fuzz.Fuzzer Bool
Fuzz.int        : Fuzz.Fuzzer Int
Fuzz.intRange   : { min: Int, max: Int } -> Fuzz.Fuzzer Int
Fuzz.float      : Fuzz.Fuzzer Float
Fuzz.floatRange : { min: Float, max: Float } -> Fuzz.Fuzzer Float
Fuzz.text       : Fuzz.Fuzzer Text
Fuzz.textOf     : { min: Int, max: Int } -> Fuzz.Fuzzer Text
Fuzz.char       : Fuzz.Fuzzer Text
Fuzz.list       : Fuzz.Fuzzer a -> Fuzz.Fuzzer (List a)
Fuzz.listOf     : { min: Int, max: Int } -> Fuzz.Fuzzer a -> Fuzz.Fuzzer (List a)
Fuzz.maybe      : Fuzz.Fuzzer a -> Fuzz.Fuzzer (Maybe a)
Fuzz.result     : Fuzz.Fuzzer e -> Fuzz.Fuzzer a -> Fuzz.Fuzzer (Result e a)
Fuzz.pair       : Fuzz.Fuzzer a -> Fuzz.Fuzzer b -> Fuzz.Fuzzer { first: a, second: b }
Fuzz.dict       : Fuzz.Fuzzer k -> Fuzz.Fuzzer v -> Fuzz.Fuzzer (Dict k v)
Fuzz.oneOf      : List (Fuzz.Fuzzer a) -> Fuzz.Fuzzer a
Fuzz.oneOfValues : List a -> Fuzz.Fuzzer a
Fuzz.frequency  : List { weight: Int, fuzzer: Fuzz.Fuzzer a } -> Fuzz.Fuzzer a
Fuzz.map        : (a -> b) -> Fuzz.Fuzzer a -> Fuzz.Fuzzer b
Fuzz.map2       : (a -> b -> c) -> Fuzz.Fuzzer a -> Fuzz.Fuzzer b -> Fuzz.Fuzzer c
Fuzz.andThen    : (a -> Fuzz.Fuzzer b) -> Fuzz.Fuzzer a -> Fuzz.Fuzzer b
Fuzz.filter     : (a -> Bool) -> Fuzz.Fuzzer a -> Fuzz.Fuzzer a
```

`Fuzz.any` is a special form, like `Test.fake`: the compiler gives it the
generator it derives from the type the context asks for (chapter 13), so
that type must be concrete where it is written (E0038). `Fuzz.forAll` and
`Fuzz.forAllTask` are legal only as the body of a `test` (E0039). `int` is
mostly small and sometimes extreme; `float` is finite (no `NaN`, no
infinity); `text` is mostly short ASCII, sometimes Unicode, sometimes
empty, and `textOf` and `char` count code points; `list` is biased to
short lists, and `listOf` bounds the length. `filter` is subject to the
health check: a predicate that rejects over 90% of the first 50 draws
fails the test (chapter 10).
