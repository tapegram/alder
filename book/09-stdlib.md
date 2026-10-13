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
```

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

```alder eval=["de", "2", "3.5"]
[Text.slice (-3) (-1) "abcdef", Text.fromFloat 2.0, Text.fromFloat 3.5]
```

## Int, Float

```alder
Int.toFloat  : Int -> Float
Int.min      : Int -> Int -> Int
Int.max      : Int -> Int -> Int
Int.abs      : Int -> Int
Int.clamp    : { min: Int, max: Int } -> Int -> Int
Int.Positive, Int.NonNegative                -- refinement brands

Float.round  : Float -> Int
Float.floor  : Float -> Int
Float.ceil   : Float -> Int
Float.min, Float.max, Float.abs
Float.format : { decimals: Int } -> Float -> Text
```

`Int` arithmetic wraps on overflow; `/` truncates toward zero and `%` has
the dividend's sign; an `Int` division or remainder by zero is a runtime
failure (chapter 4). `Float` division by zero gives infinity or NaN.

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
```

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

Time.now       : Query Time
Time.toDate    : Time -> Date
Time.format    : Text -> Time -> Text
Time.ago       : Time -> Time -> Text           -- Time.ago now then = "3 hours ago"
```

`Date.diffDays a b` is the number of days from `a` to `b`, that is `b − a`:
`Date.diffDays monday wednesday` is `2`, and it is negative when `b` comes
first.

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
Url.external : Text -> Page                     -- external link for href or Redirect
```

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

`Query.notFound` and `Task.notFound` (both
`: Query a` / `: Task a`, no arguments) stop the effect and answer 404 with
`App.notFound`; a `Task` that ends in `Task.notFound` is rolled back like
any failed `Task`.

## Test

Only a `test` may use these (chapter 10); anywhere else each is E0003
"… is only available inside a `test`".

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

### Browser steps and queries

The browser steps (chapter 10, section 9) are special forms too: each is
applied directly to all its arguments, never passed as a value or piped
into, and `get`, `open`, `submit`, `submitRaw` and `click` need an `App`
module. `Session` here is `App`'s session record.

```text
Test.login     : Session -> Task Unit
Test.get       : Page -> Task { model: Model, session: Session, doc: Doc }
Test.open      : Page -> Task { answer: Answer, session: Session, doc: Doc }
Test.submit    : Form -> record -> Task { answer: Answer, session: Session, doc: Doc }
Test.submitRaw : Form -> List { name: Text, value: Text } -> Task { answer: Answer, session: Session, doc: Doc }
Test.click     : Msg -> Task { answer: Answer, session: Session, doc: Doc }
```

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

Two types of the Prelude exist only in tests (anywhere else, E0003
"`Doc` is a test type"): `Doc`, a parsed page, which has `==`, and

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
