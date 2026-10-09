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

## Maybe

```alder
Maybe.map      : (a -> b) -> Maybe a -> Maybe b
Maybe.andThen  : (a -> Maybe b) -> Maybe a -> Maybe b
Maybe.withDefault : a -> Maybe a -> a          -- same as ??
Maybe.isJust   : Maybe a -> Bool
Maybe.toList   : Maybe a -> List a
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

`Text.length` counts code points (`Text.length "é"` is 1), and
`Text.slice start end` takes the code points from `start` up to, not
including, `end`. A negative index counts from the end (`Text.slice -3 -1
"abcdef"` is `"de"`), indices past either end are clamped, and a `start` at
or after `end` gives `""`. `Text.matches regex text` is true when the
regex (RE2 syntax) matches anywhere in the text: write `^…$` to match the
whole text. An invalid regex matches nothing. `Text.fromFloat` writes the
shortest decimal that reads back as the same number, never an exponent
(`2.0` is `"2"`, `3.5` is `"3.5"`); infinity is `"+Inf"` or `"-Inf"` and
not-a-number is `"NaN"`. `Text.toInt` and `Text.toFloat` give `Nothing`
for anything else than a plain decimal number (spaces, overflow, `"NaN"`).

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
`Float.round -2.5` is `-3`. `Float.format { decimals: n }` rounds the same
way to `n` decimals and pads with zeros (`Float.format { decimals: 2 }
1.005` is `"1.01"`, and `1.5` gives `"1.50"`); it never writes `"-0"`
(`-0.001` with one decimal is `"0.0"`).

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

A key may be of any type: a `List`, a record, a `Time`. Two keys are the
same key when they are `==`, so a `Time` key is its instant (the same
moment written in two time zones is one key). A `Dict` keeps its keys in
the order they were first inserted; `==` on two dicts ignores that order.

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

## Html

```alder
Html.unsafe : Text -> Html Never                -- unescaped markup; never for user text
Html.none  : Html a                             -- renders nothing
Html.text  : Text -> Html a
Url.external : Text -> Page                     -- external link for href
```

## Query, Task, Db, Session, Log, Time, Random, Http

See chapters 7 and 8.

## Form

```alder
Form.values : Form f -> f            -- for use in tests only
```
