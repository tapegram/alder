# Appendix D: Standard library reference

This file is generated from the doc comments in `stdlib/`. To regenerate it, run `go test ./book -run TestAppendixD_Current -update`.

## Bool

Functions for `Bool`, which is either `True` or `False`.

`not` (in the Prelude), `&&` and `||` cover the logic on a `Bool`, so this module only holds
the conversion to `Text`.

### `Bool.toText`

```alder fragment
toText : Bool -> Text
```

Writes a `Bool` as `Text`.

The text is the name of the value, with a capital letter: `"True"` or `"False"`.

```alder fragment
Bool.toText True == "True"
Bool.toText False == "False"
Bool.toText (1 < 2) == "True"
```

## Bytes

Functions for `Bytes`, a body that is not text, such as an image or a download.

Bytes are counted and cut one byte at a time, unlike `Text`, which works in characters. Use
`Text.toBytes` to turn a `Text` into `Bytes`, and `Bytes.toText` to go back. A function never
changes its input: it gives back new `Bytes`.

### `Bytes.length`

```alder fragment
length : Bytes -> Int
```

The number of bytes.

This counts bytes, not characters. A letter with an accent takes more than one byte in UTF-8
text.

```alder fragment
Bytes.length (Text.toBytes "hello") == 5
Bytes.length (Text.toBytes "é") == 2
Bytes.length Bytes.empty == 0
```

### `Bytes.toText`

```alder fragment
toText : Bytes -> Maybe Text
```

Reads `Bytes` as `Text`, or `Nothing` when they are not valid UTF-8.

Valid UTF-8 is the text that `Text.toBytes` writes. Bytes that cut a character in half, or
that are not text at all, give `Nothing`. The empty `Bytes` give `Just ""`.

```alder fragment
Bytes.toText (Text.toBytes "héllo") == Just "héllo"
Bytes.toText Bytes.empty == Just ""
Bytes.toText (Bytes.fromList [255]) == Nothing
```

### `Bytes.empty`

```alder fragment
empty : Bytes
```

`Bytes` with nothing in them.

```alder fragment
Bytes.length Bytes.empty == 0
Bytes.empty == Bytes.fromList []
```

### `Bytes.isEmpty`

```alder fragment
isEmpty : Bytes -> Bool
```

Tells whether `Bytes` has no bytes at all.

```alder fragment
Bytes.isEmpty Bytes.empty == True
Bytes.isEmpty (Bytes.fromList [0]) == False
```

### `Bytes.slice`

```alder fragment
slice : Int -> Int -> Bytes -> Bytes
```

Takes the bytes from one position up to, but not including, another.

It follows the rule of `Text.slice`, counting bytes. Positions count from 0. A negative
position counts from the end, so `-1` is the last byte; wrap a negative number in
parentheses. A position past either end is moved to that end, and a start at or after the
end gives empty `Bytes`.

```alder fragment
Bytes.toList (Bytes.slice 1 3 (Bytes.fromList [10, 20, 30, 40])) == [20, 30]
Bytes.toList (Bytes.slice (-2) 4 (Bytes.fromList [10, 20, 30, 40])) == [30, 40]
Bytes.toList (Bytes.slice 2 100 (Bytes.fromList [10, 20, 30, 40])) == [30, 40]
Bytes.toList (Bytes.slice 3 1 (Bytes.fromList [10, 20, 30, 40])) == []
```

### `Bytes.append`

```alder fragment
append : Bytes -> Bytes -> Bytes
```

Puts one `Bytes` after another.

```alder fragment
Bytes.toList (Bytes.append (Bytes.fromList [1, 2]) (Bytes.fromList [3])) == [1, 2, 3]
Bytes.append Bytes.empty (Bytes.fromList [1]) == Bytes.fromList [1]
```

### `Bytes.concat`

```alder fragment
concat : List Bytes -> Bytes
```

Joins a list of `Bytes` into one, in order.

An empty list gives empty `Bytes`.

```alder fragment
Bytes.toList (Bytes.concat [Bytes.fromList [1], Bytes.fromList [2, 3]]) == [1, 2, 3]
Bytes.concat [] == Bytes.empty
```

### `Bytes.toList`

```alder fragment
toList : Bytes -> List Int
```

Lists the bytes as `Int`s.

Each number is from 0 to 255.

```alder fragment
Bytes.toList (Bytes.fromList [7, 8, 9]) == [7, 8, 9]
Bytes.toList (Text.toBytes "A") == [65]
Bytes.toList Bytes.empty == []
```

### `Bytes.fromList`

```alder fragment
fromList : List Int -> Bytes
```

Builds `Bytes` from a list of `Int`s.

Only the low eight bits of each number are kept, so a number outside 0 to 255 wraps around:
`256` becomes `0` and `-1` becomes `255`.

```alder fragment
Bytes.toList (Bytes.fromList [1, 2, 3]) == [1, 2, 3]
Bytes.toList (Bytes.fromList [256, 257]) == [0, 1]
Bytes.toList (Bytes.fromList [-1]) == [255]
```

## Date

Functions for `Date`, a calendar date such as 2026-10-08.

A `Date` is a year, a month and a day with no time of day and no time zone. Dates compare
with `==`, and `Date.toText` writes one as `YYYY-MM-DD`. Use `Time` for an instant. Where a
function needs a moment, such as `Date.today`, it is in UTC.

### `Date.today`

```alder fragment
today : Query Date
```

The current date, in UTC.

It reads the same clock as `Time.now`, in UTC, with a separate reading each time. It is a `Query`:
bind it with `let today <-`, or map it.

```alder fragment
Date.today |> Query.map Date.toText
```

### `Date.fromText`

```alder fragment
fromText : Text -> Maybe Date
```

Reads a `Date` from a `Text` written `YYYY-MM-DD`, or `Nothing` when it is not one.

The year, month and day must all be written in full with two digits for the month and the day.
Any other shape, and a date that does not exist such as February 30, gives `Nothing`.

```alder fragment
(Date.fromText "2026-10-08" |> Maybe.map Date.toText) == Just "2026-10-08"
Date.fromText "2026-02-30" == Nothing
Date.fromText "2026-10-8" == Nothing
Date.fromText "10/08/2026" == Nothing
Date.fromText "" == Nothing
```

### `Date.toText`

```alder fragment
toText : Date -> Text
```

Writes a `Date` as `YYYY-MM-DD`.

The year has at least four digits, and the month and the day have two, so for the years 0 to
9999 the text sorts in date order. A year below 0 or above 9999 does not keep that: it gets a
minus sign or a fifth digit, and the text then sorts out of order.

```alder fragment
Date.toText Date.epoch == "1970-01-01"
Date.toText (Date.addDays 40 Date.epoch) == "1970-02-10"
```

### `Date.format`

```alder fragment
format : Text -> Date -> Text
```

Writes a `Date` in a layout of your choosing.

The layout is written the way Go does it: it is the date `Mon Jan 2 15:04:05 2006` itself,
cut and changed to show the shape you want. `"Jan 2, 2006"` gives `Oct 8, 2026`, `"2006-01-02"`
gives `2026-10-08` and `"Monday"` gives the name of the weekday. The parts of the layout that
are not a part of that date are copied as they are. A `Date` has no time of day, so the time
parts of a layout show midnight.

```alder fragment
Date.format "Jan 2, 2006" (Date.addDays 40 Date.epoch) == "Feb 10, 1970"
Date.format "Monday" Date.epoch == "Thursday"
Date.format "02/01/06" (Date.addDays 40 Date.epoch) == "10/02/70"
```

### `Date.addDays`

```alder fragment
addDays : Int -> Date -> Date
```

Moves a `Date` by a number of days.

A negative number moves back. The month and the year carry over as they should, and a leap
day counts.

```alder fragment
(Date.fromParts { year: 2024, month: 2, day: 28 } |> Maybe.map (Date.addDays 1) |> Maybe.map Date.toText) == Just "2024-02-29"
(Date.addDays 1 Date.epoch |> Date.toText) == "1970-01-02"
(Date.addDays (-1) Date.epoch |> Date.toText) == "1969-12-31"
(Date.addDays 365 Date.epoch |> Date.toText) == "1971-01-01"
Date.addDays 0 Date.epoch == Date.epoch
```

### `Date.diffDays`

```alder fragment
diffDays : Date -> Date -> Int
```

The number of days from the first `Date` to the second.

`Date.diffDays a b` is `b` minus `a`, so it is negative when `b` comes first and `0` for the
same day.

```alder fragment
Date.diffDays Date.epoch (Date.addDays 10 Date.epoch) == 10
Date.diffDays (Date.addDays 10 Date.epoch) Date.epoch == -10
Date.diffDays Date.epoch Date.epoch == 0
```

### `Date.year`

```alder fragment
year : Date -> Int
```

The year of a `Date`.

```alder fragment
Date.year (Date.addDays 365 Date.epoch) == 1971
```

### `Date.month`

```alder fragment
month : Date -> Int
```

The month of a `Date`, from 1 for January to 12 for December.

```alder fragment
Date.month (Date.addDays 40 Date.epoch) == 2
```

### `Date.day`

```alder fragment
day : Date -> Int
```

The day of the month of a `Date`, from 1.

```alder fragment
Date.day (Date.addDays 40 Date.epoch) == 10
```

### `Date.fromParts`

```alder fragment
fromParts : { year: Int, month: Int, day: Int } -> Maybe Date -- Nothing for an impossible date
```

Makes a `Date` from a year, a month and a day, or `Nothing` for a date that does not exist.

It never rolls over: month 13 is not January of the next year and February 30 is not March 2,
both are `Nothing`.

```alder fragment
(Date.fromParts { year: 2026, month: 10, day: 8 } |> Maybe.map Date.toText) == Just "2026-10-08"
(Date.fromParts { year: 2024, month: 2, day: 29 } |> Maybe.map Date.toText) == Just "2024-02-29"
Date.fromParts { year: 2026, month: 2, day: 29 } == Nothing
Date.fromParts { year: 2026, month: 13, day: 1 } == Nothing
Date.fromParts { year: 2026, month: 4, day: 31 } == Nothing
Date.fromParts { year: 2026, month: 1, day: 0 } == Nothing
```

### `Date.toParts`

```alder fragment
toParts : Date -> { year: Int, month: Int, day: Int }
```

The year, the month and the day of a `Date` as a record.

```alder fragment
Date.toParts (Date.addDays 40 Date.epoch) == { year: 1970, month: 2, day: 10 }
```

### `Date.epoch`

```alder fragment
epoch : Date -- 1970-01-01
```

The date 1970-01-01, the day that `Time.epoch` falls on.

```alder fragment
Date.toText Date.epoch == "1970-01-01"
```

### `Date.weekday`

```alder fragment
weekday : Date -> Int -- ISO: 1 = Monday ... 7 = Sunday
```

The day of the week of a `Date`, numbered the ISO way: 1 for Monday up to 7 for Sunday.

Sunday is 7, not 0, so `Date.weekday d >= 6` tells a weekend.

```alder fragment
Date.weekday Date.epoch == 4
Date.weekday (Date.addDays 3 Date.epoch) == 7
Date.weekday (Date.addDays 4 Date.epoch) == 1
```

### `Date.addMonths`

```alder fragment
addMonths : Int -> Date -> Date -- clamps the day to the target month's length
```

Moves a `Date` by a number of months.

The day of the month stays the same where the target month is long enough. When the target
month is shorter than the day, the day is clamped to the last day of that
month: one month after January 31 is February 28, or 29 in a leap year. A negative number
moves back. Moving by months is not the same as moving by 30 days.

```alder fragment
(Date.fromParts { year: 2026, month: 1, day: 31 } |> Maybe.map (Date.addMonths 1) |> Maybe.map Date.toText) == Just "2026-02-28"
(Date.fromParts { year: 2024, month: 1, day: 31 } |> Maybe.map (Date.addMonths 1) |> Maybe.map Date.toText) == Just "2024-02-29"
(Date.addMonths 13 Date.epoch |> Date.toText) == "1971-02-01"
(Date.addMonths (-1) Date.epoch |> Date.toText) == "1969-12-01"
```

### `Date.addYears`

```alder fragment
addYears : Int -> Date -> Date -- Feb 29 becomes Feb 28 in a non-leap year
```

Moves a `Date` by a number of years.

It is `Date.addMonths` of twelve times as many months, so February 29 becomes February 28 in
a year that is not a leap year. A negative number moves back.

```alder fragment
(Date.fromParts { year: 2024, month: 2, day: 29 } |> Maybe.map (Date.addYears 1) |> Maybe.map Date.toText) == Just "2025-02-28"
(Date.fromParts { year: 2024, month: 2, day: 29 } |> Maybe.map (Date.addYears 4) |> Maybe.map Date.toText) == Just "2028-02-29"
(Date.addYears (-1) Date.epoch |> Date.toText) == "1969-01-01"
```

## Db

Functions for reading and writing the rows of the app's tables.

A table's name is a value of type `Table t` (`Db.get Todo id`), and its columns are values
named `Table.field` with methods that build conditions and orders (`Todo.done.eq False`,
`Todo.created.desc`). Reads are `Query` values and writes are `Task` values. A read builds a
`Select` with `Db.from` and the steps after it, then runs it with `Db.all`, `Db.first` or
`Db.count`. A column must be written in place, not held in a variable, and these functions
need to know their table where they are used: a function of `Table t` that calls them is a
type error, so write one function per table.

### `Db.get`

```alder fragment
get : Table t -> Id t -> Query (Maybe t)
```

Reads the row with an id, giving `Just` the row or `Nothing` when there is none.

A missing row is not a failure. `Db.get` does not look at who is signed in: use
`Db.getMine` for a row that must be the user's.

```alder fragment
Db.get Todo (Todo.TodoId "a")
```

### `Db.from`

```alder fragment
from : Table t -> Select t
```

A `Select` of every row of a table, for the steps after it to narrow and order.

Nothing is read until `Db.all`, `Db.first` or `Db.count` runs the `Select`.

```alder fragment
Db.from Todo |> Db.all
```

### `Db.where`

```alder fragment
where : Condition t -> Select t -> Select t
```

Keeps only the rows for which a condition holds.

Several `Db.where` steps are combined with "and". Where the step sits in the pipeline does
not change what it means: `Db.limit 10 |> Db.where c` is still up to ten rows for which `c`
holds.

```alder fragment
Db.from Todo |> Db.where (Todo.done.eq False) |> Db.where (Todo.title.contains "milk") |> Db.all
```

### `Db.orderBy`

```alder fragment
orderBy : Order t -> Select t -> Select t
```

Sorts the rows by a column, with `.asc` or `.desc` on the column.

Orders add up in the order written: the first is the main sort, and a later one breaks its
ties. A column of a union type without payloads sorts in the order its constructors are
declared. A `Maybe` column puts `Nothing` first with `.asc` and last with `.desc`. Without
any `Db.orderBy`, the order of the rows is not defined.

```alder fragment
Db.from Todo |> Db.orderBy Todo.done.asc |> Db.orderBy Todo.created.desc |> Db.all
```

### `Db.limit`

```alder fragment
limit : Int -> Select t -> Select t
```

Keeps at most a number of rows.

The last `Db.limit` written wins, and it applies after every condition and order, wherever
it appears. A limit of 0 gives no rows. A negative limit is no limit at all: every row comes
back.

```alder fragment
Db.from Todo |> Db.orderBy Todo.created.desc |> Db.limit 10 |> Db.all
```

### `Db.offset`

```alder fragment
offset : Int -> Select t -> Select t
```

Skips a number of rows before the ones that come back, for paging.

The last `Db.offset` written wins, and it applies after every condition and order, wherever
it appears. A negative offset is the same as 0.

```alder fragment
Db.from Todo |> Db.orderBy Todo.created.desc |> Db.limit 10 |> Db.offset 20 |> Db.all
```

### `Db.all`

```alder fragment
all : Select t -> Query (List t)
```

Reads every row of a `Select`.

No matching row gives an empty list.

```alder fragment
Db.from Todo |> Db.where (Todo.done.eq True) |> Db.all
```

### `Db.first`

```alder fragment
first : Select t -> Query (Maybe t)
```

Reads the first row of a `Select`, or `Nothing` when there is none.

A `Select` with `Db.limit 0` gives `Nothing`.

```alder fragment
Db.from Todo |> Db.orderBy Todo.created.desc |> Db.first
```

### `Db.count`

```alder fragment
count : Select t -> Query Int
```

Counts the rows of a `Select`.

The count follows the `Select`'s limit and offset: `Db.limit 5` counts at most five rows.

```alder fragment
Db.from Todo |> Db.where (Todo.done.eq False) |> Db.count
```

### `Db.and`

```alder fragment
and : List (Condition t) -> Condition t
```

A condition that holds when every condition of the list holds.

An empty list holds for every row.

```alder fragment
Db.from Todo |> Db.where (Db.and [Todo.done.eq False, Todo.due.isJust]) |> Db.all
Db.from Todo |> Db.where (Db.and []) |> Db.all
```

### `Db.or`

```alder fragment
or : List (Condition t) -> Condition t
```

A condition that holds when at least one condition of the list holds.

An empty list holds for no row.

```alder fragment
Db.from Todo |> Db.where (Db.or [Todo.title.contains "milk", Todo.title.contains "eggs"]) |> Db.all
Db.from Todo |> Db.where (Db.or []) |> Db.all
```

### `Db.not`

```alder fragment
not : Condition t -> Condition t
```

A condition that holds exactly for the rows the given condition does not.

On a `Maybe` column a condition is never unknown, so `Db.not (Todo.due.isJust)` keeps the
rows with no due date, and the negation of an `.eq (Just d)` keeps them too.

```alder fragment
Db.from Todo |> Db.where (Db.not (Todo.title.startsWith "x")) |> Db.all
```

### `Db.insert`

```alder fragment
insert : Table t -> Insert t -> Task (Id t)
```

Inserts a row and gives back its new id.

The row is an `Insert t`: the table's record without its `@id`, `@created` and `@updated`
fields, which are filled in, and in which fields with a default may be left out. The id is a
new random 20-character text. A row that breaks a `@unique` column fails the `Task` with a
message such as "User.email must be unique", and a `@ref` field naming no row fails it with
one such as "Todo.owner: no User with that id"; either way the `Task`'s earlier writes are
rolled back.

```alder fragment
Db.insert Todo { title: Text.NonEmpty "Buy milk", due: Nothing, owner: User.UserId "u" }
```

### `Db.update`

```alder fragment
update : Table t -> Id t -> (t -> t) -> Task Unit
```

Changes the row with an id: reads it, applies the function, and writes the result back.

The `@updated` field is set to the current time. A missing row fails the `Task` (a failure,
not a 404) with "Db.update Todo: no row with id …", and a function that changes the `@id`
field fails it with "Db.update Todo: update may not change the id"; either way nothing is
written. The `@unique` and `@ref` checks of `Db.insert` apply too.

```alder fragment
Db.update Todo (Todo.TodoId "a") (\t -> { t | done: True })
```

### `Db.delete`

```alder fragment
delete : Table t -> Id t -> Task Unit
```

Deletes the row with an id.

Deleting a row that does not exist is not an error. When other rows refer to it with `@ref`,
their delete rule decides: `@restrict` (the default) fails the `Task`, `@cascade` deletes them
too, and `@setNull` sets their field to `Nothing`.

```alder fragment
Db.delete Todo (Todo.TodoId "a")
```

### `Db.mine`

```alder fragment
mine : Table t -> Select t
```

A `Select` of the signed-in user's rows of a table.

It is `Db.from t` keeping the rows whose owner is the session's user. The table needs an
`@owner` column, and the call must be in a route that `requires user`, or in a helper that
only such routes call. The owner condition (`owner = session.user`) is added when the
`Select` runs, for the user of the running request or test, not the `session` argument; with
no signed-in user it fails with "Db.mine Todo: no signed-in user".

### `Db.getMine`

```alder fragment
getMine : Table t -> Id t -> Query t
```

Reads the signed-in user's row with an id.

When the row does not exist or is not the session user's, it stops as `Query.notFound`:
someone else's row is answered as a missing one, with a 404. There is no `Maybe` to match: a
missing row ends the request. It reads in one `SELECT` with `id = ? AND owner = ?`. The rules
of `Db.mine` (an `@owner` column, a route that `requires user`, a signed-in user) apply.

### `Db.updateMine`

```alder fragment
updateMine : Table t -> Id t -> (t -> t) -> Task Unit
```

Changes the signed-in user's row with an id, as `Db.update` does.

It reads the row with `id = ? AND owner = ?`, applies the function, and writes it back with
one UPDATE with `id = ? AND owner = ?`; no row changed is `Task.notFound`, so someone else's
row is a 404 and nothing is written. The owner column is never written: a function that
changes it does not give the row away. A function that changes the `@id` field fails the
`Task`. The rules of `Db.mine` apply.

### `Db.deleteMine`

```alder fragment
deleteMine : Table t -> Id t -> Task Unit
```

Deletes the signed-in user's row with an id.

It is one DELETE with `id = ? AND owner = ?`; no row deleted is `Task.notFound`, so a missing
row or someone else's is a 404. The rules of `Db.mine` apply.

### `Db.insertMine`

```alder fragment
insertMine : Table t -> Insert t -> Task (Id t)
```

Inserts a row owned by the signed-in user and gives back its new id.

The row is `Insert t` without the owner column, written as a record literal in the call; the
owner is the session's user, and giving the owner field is a type error. Otherwise it is
`Db.insert`. The rules of `Db.mine` apply.

### `Db.scoped`

```alder fragment
scoped : Table t -> Select t
```

A `Select` of the rows of a table in the session's partition.

It is `Db.from t` keeping the rows whose partition column is the session's key
(`col = session.k`). The table needs a `@partition k` column, and the call must be in a
route that `requires k`. The condition is added when the `Select` runs, for the running
request or test; without the key it fails with a message such as "Db.scoped Project: the
session has no org". The `mine` operations are these for the key `user`.

### `Db.getScoped`

```alder fragment
getScoped : Table t -> Id t -> Query t
```

Reads the row with an id in the session's partition.

When the row does not exist or is in another partition, it stops as `Query.notFound`: a row
in another partition is answered as a missing one, with a 404. The rules of `Db.scoped`
apply.

### `Db.updateScoped`

```alder fragment
updateScoped : Table t -> Id t -> (t -> t) -> Task Unit
```

Changes the row with an id in the session's partition, as `Db.update` does.

It writes with one UPDATE with `id = ? AND col = ?`, never writing the partition column; no
row changed is `Task.notFound`, so a row in another partition is a 404 and nothing is
written. The rules of `Db.scoped` apply.

### `Db.deleteScoped`

```alder fragment
deleteScoped : Table t -> Id t -> Task Unit
```

Deletes the row with an id in the session's partition.

It is one DELETE with `id = ? AND col = ?`; no row deleted is `Task.notFound`, so a missing
row or one in another partition is a 404. The rules of `Db.scoped` apply.

### `Db.insertScoped`

```alder fragment
insertScoped : Table t -> Insert t -> Task (Id t)
```

Inserts a row in the session's partition and gives back its new id.

The row is `Insert t` without the partition column, written as a record literal in the call;
the column is the session's key. Otherwise it is `Db.insert`. The rules of `Db.scoped` apply.

## Dict

Functions for `Dict`, a collection that looks a value up by its key.

A key may be any value that `==` works on, such as a `Text`, an `Int`, a `List` or a record,
but not a function. Two keys are the same key when they are `==`. A `Dict` remembers the order
in which keys were first added, and `toList`, `keys`, `values`, `map`, `filter` and `foldl` all
follow that order. `==` on two dicts ignores the order. A function never changes its input: it
gives back a new `Dict`.

### `Dict.empty`

```alder fragment
empty : Dict k v
```

A `Dict` with nothing in it.

```alder fragment
Dict.size Dict.empty == 0
Dict.get "a" Dict.empty == Nothing
```

### `Dict.fromList`

```alder fragment
fromList : List { key: k, value: v } -> Dict k v
```

Builds a `Dict` from a list of keys and values.

The entries go in list order. When a key appears more than once, it keeps its first position
and takes its last value.

```alder fragment
Dict.get "b" (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }]) == Just 2
Dict.keys (Dict.fromList [{ key: "x", value: 1 }, { key: "y", value: 2 }, { key: "x", value: 3 }]) == ["x", "y"]
Dict.get "x" (Dict.fromList [{ key: "x", value: 1 }, { key: "y", value: 2 }, { key: "x", value: 3 }]) == Just 3
Dict.fromList [] == Dict.empty
```

### `Dict.toList`

```alder fragment
toList : Dict k v -> List { key: k, value: v }
```

Lists the entries of a `Dict` as records with a `key` and a `value`.

The entries come in the order the keys were first added.

```alder fragment
Dict.toList (Dict.singleton "a" 1) == [{ key: "a", value: 1 }]
Dict.toList (Dict.insert "b" 2 (Dict.singleton "a" 1)) == [{ key: "a", value: 1 }, { key: "b", value: 2 }]
Dict.toList Dict.empty == []
```

### `Dict.get`

```alder fragment
get : k -> Dict k v -> Maybe v
```

Looks up a key, giving `Just` its value or `Nothing` when the key is not there.

```alder fragment
Dict.get "a" (Dict.singleton "a" 1) == Just 1
Dict.get "z" (Dict.singleton "a" 1) == Nothing
```

### `Dict.insert`

```alder fragment
insert : k -> v -> Dict k v -> Dict k v
```

Binds a key to a value.

A key that is already there keeps its position and gets the new value. A new key goes at the
end.

```alder fragment
Dict.get "b" (Dict.insert "b" 2 (Dict.singleton "a" 1)) == Just 2
Dict.keys (Dict.insert "b" 2 (Dict.singleton "a" 1)) == ["a", "b"]
Dict.toList (Dict.insert "a" 9 (Dict.singleton "a" 1)) == [{ key: "a", value: 9 }]
```

### `Dict.remove`

```alder fragment
remove : k -> Dict k v -> Dict k v
```

Takes a key out of a `Dict`.

A key that is not there leaves the `Dict` as it was. The other entries keep their order.

```alder fragment
Dict.keys (Dict.remove "a" (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }])) == ["b"]
Dict.remove "z" (Dict.singleton "a" 1) == Dict.singleton "a" 1
```

### `Dict.keys`

```alder fragment
keys : Dict k v -> List k
```

Lists the keys of a `Dict`, in the order they were first added.

```alder fragment
Dict.keys (Dict.fromList [{ key: "b", value: 1 }, { key: "a", value: 2 }]) == ["b", "a"]
Dict.keys Dict.empty == []
```

### `Dict.values`

```alder fragment
values : Dict k v -> List v
```

Lists the values of a `Dict`, in the order their keys were first added.

Two keys with the same value give that value twice.

```alder fragment
Dict.values (Dict.fromList [{ key: "b", value: 1 }, { key: "a", value: 2 }]) == [1, 2]
Dict.values (Dict.fromList [{ key: "a", value: 7 }, { key: "b", value: 7 }]) == [7, 7]
Dict.values Dict.empty == []
```

### `Dict.size`

```alder fragment
size : Dict k v -> Int
```

The number of entries in a `Dict`.

```alder fragment
Dict.size (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }]) == 2
Dict.size Dict.empty == 0
```

### `Dict.singleton`

```alder fragment
singleton : k -> v -> Dict k v
```

A `Dict` with one entry.

```alder fragment
Dict.toList (Dict.singleton "a" 1) == [{ key: "a", value: 1 }]
Dict.size (Dict.singleton "a" 1) == 1
```

### `Dict.isEmpty`

```alder fragment
isEmpty : Dict k v -> Bool
```

Tells whether a `Dict` has no entries.

```alder fragment
Dict.isEmpty Dict.empty == True
Dict.isEmpty (Dict.singleton "a" 1) == False
```

### `Dict.member`

```alder fragment
member : k -> Dict k v -> Bool
```

Tells whether a key is in a `Dict`.

```alder fragment
Dict.member "a" (Dict.singleton "a" 1) == True
Dict.member "z" (Dict.singleton "a" 1) == False
```

### `Dict.update`

```alder fragment
update : k -> (Maybe v -> Maybe v) -> Dict k v -> Dict k v
```

Changes the value of one key with a function.

The function gets `Just` the current value, or `Nothing` when the key is not there. If it
gives `Just` a value, the key is bound to it: an existing key keeps its position and a new key
goes at the end. If it gives `Nothing`, the key is removed, or stays absent.

```alder fragment
Dict.get "a" (Dict.update "a" (\m -> Maybe.map (\n -> n + 1) m) (Dict.singleton "a" 1)) == Just 2
Dict.get "b" (Dict.update "b" (\m -> Just 0) (Dict.singleton "a" 1)) == Just 0
Dict.keys (Dict.update "b" (\m -> Just 0) (Dict.singleton "a" 1)) == ["a", "b"]
Dict.member "a" (Dict.update "a" (\m -> Nothing) (Dict.singleton "a" 1)) == False
Dict.size (Dict.update "z" (\m -> Nothing) (Dict.singleton "a" 1)) == 1
```

### `Dict.map`

```alder fragment
map : (k -> v -> w) -> Dict k v -> Dict k w
```

Changes every value with a function that also sees the key.

The keys and their order stay the same.

```alder fragment
Dict.toList (Dict.map (\k v -> v * 2) (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }])) == [{ key: "a", value: 2 }, { key: "b", value: 4 }]
Dict.values (Dict.map (\k v -> k ++ v) (Dict.singleton "a" "1")) == ["a1"]
```

### `Dict.filter`

```alder fragment
filter : (k -> v -> Bool) -> Dict k v -> Dict k v
```

Keeps the entries for which a function gives `True`.

The function sees the key and the value. The entries that stay keep their order.

```alder fragment
Dict.keys (Dict.filter (\k v -> v > 1) (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }, { key: "c", value: 3 }])) == ["b", "c"]
Dict.size (Dict.filter (\k v -> False) (Dict.singleton "a" 1)) == 0
```

### `Dict.foldl`

```alder fragment
foldl : (k -> v -> b -> b) -> b -> Dict k v -> b
```

Folds the entries of a `Dict` into one value, starting from the first key added.

The function gets the key, the value and the result so far, and the second argument is the
starting result. For an empty `Dict` the starting result comes back.

```alder fragment
Dict.foldl (\k v acc -> acc + v) 0 (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }]) == 3
Dict.foldl (\k v acc -> acc ++ [k]) [] (Dict.fromList [{ key: "b", value: 1 }, { key: "a", value: 2 }]) == ["b", "a"]
Dict.foldl (\k v acc -> acc + v) 0 Dict.empty == 0
```

### `Dict.partition`

```alder fragment
partition : (k -> v -> Bool) -> Dict k v -> { yes: Dict k v, no: Dict k v }
```

Splits a `Dict` in two by a test on each entry.

The `yes` dict has the entries for which the function gives `True` and the `no` dict has the
rest. Both keep the original order.

```alder fragment
Dict.keys (Dict.partition (\k v -> v > 1) (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }])).yes == ["b"]
Dict.keys (Dict.partition (\k v -> v > 1) (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }])).no == ["a"]
```

### `Dict.union`

```alder fragment
union : Dict k v -> Dict k v -> Dict k v
```

Combines two dicts, preferring the first.

A key in both dicts keeps the first dict's value and position. The keys only the second dict
has are added at the end, in their order.

```alder fragment
Dict.toList (Dict.union (Dict.singleton "a" 1) (Dict.fromList [{ key: "b", value: 2 }, { key: "a", value: 9 }])) == [{ key: "a", value: 1 }, { key: "b", value: 2 }]
Dict.union Dict.empty (Dict.singleton "a" 1) == Dict.singleton "a" 1
```

### `Dict.intersect`

```alder fragment
intersect : Dict k v -> Dict k v -> Dict k v
```

Keeps the entries of the first dict whose key is also in the second.

The values and the order come from the first dict; the values of the second are not used.

```alder fragment
Dict.toList (Dict.intersect (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }]) (Dict.singleton "b" 9)) == [{ key: "b", value: 2 }]
Dict.intersect (Dict.singleton "a" 1) Dict.empty == Dict.empty
```

### `Dict.diff`

```alder fragment
diff : Dict k v -> Dict k v -> Dict k v
```

Keeps the entries of the first dict whose key is not in the second.

The values and the order come from the first dict; the values of the second are not used.

```alder fragment
Dict.toList (Dict.diff (Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: 2 }]) (Dict.singleton "b" 9)) == [{ key: "a", value: 1 }]
Dict.diff (Dict.singleton "a" 1) Dict.empty == Dict.singleton "a" 1
```

## File

Functions for `File`, the answer of a file page.

A `File` is a status, a content type, some headers and a body. A file page's `view` returns
a `File` instead of markup, to serve text, JSON, a download or a page written by hand. Build
one with `File.text`, `File.bytes`, `File.html` or `File.json`, which answer 200, and change it
with `File.withStatus` and `File.withHeader`. The router writes a `File` as it is given, with
no doctype, head tags or panel added. `File.status`, `File.contentType` and `File.body` read
one back.

### `File.text`

```alder fragment
text : { contentType: Text, body: Text } -> File
```

A 200 answer with a text body.

The text is sent as UTF-8, and `contentType` is the content type to send with it, such as
`"text/plain; charset=utf-8"` or `"text/csv"`.

```alder fragment
File.status (File.text { contentType: "text/plain; charset=utf-8", body: "hello" }) == 200
File.contentType (File.text { contentType: "text/csv", body: "a,b" }) == "text/csv"
Bytes.toText (File.body (File.text { contentType: "text/plain", body: "hello" })) == Just "hello"
```

### `File.bytes`

```alder fragment
bytes : { contentType: Text, body: Bytes } -> File
```

A 200 answer with a body of bytes.

Use it for what is not text, such as an image or an archive, from a capability that returns
`Bytes`.

```alder fragment
File.status (File.bytes { contentType: "application/octet-stream", body: Bytes.fromList [1, 2, 3] }) == 200
Bytes.toList (File.body (File.bytes { contentType: "image/png", body: Bytes.fromList [1, 2, 3] })) == [1, 2, 3]
```

### `File.html`

```alder fragment
html : Html -> File
```

A 200 answer holding a web page, sent as `text/html; charset=utf-8`.

The markup is written exactly as it renders, byte for byte. The renderer writes no doctype, so
a page that must start with `<!doctype html>` needs one put in front with `Html.unsafe` in an
`Html.fragment`. Nothing is added: no head tags, no page script.

```alder fragment
File.contentType (File.html (<p>hi</p>)) == "text/html; charset=utf-8"
Bytes.toText (File.body (File.html (<p>hi</p>))) == Just "<p>hi</p>"
Bytes.toText (File.body (File.html (Html.fragment [Html.unsafe "<!doctype html>\n", <p>hi</p>]))) == Just "<!doctype html>\n<p>hi</p>"
```

### `File.json`

```alder fragment
json : Json.Value -> File
```

A 200 answer holding a `Json.Value`, sent as `application/json`.

The body is the compact text `Json.encode` gives.

```alder fragment
File.contentType (File.json (Json.int 1)) == "application/json"
Bytes.toText (File.body (File.json (Json.object [{ key: "ok", value: Json.bool True }]))) == Just "\{\"ok\":true}"
```

### `File.withStatus`

```alder fragment
withStatus : Int -> File -> File
```

Changes the status of a `File`.

The status must be from 200 to 599. A number written in the code that is outside that range
is a compile error, and one that comes from a variable is answered with the app's 500 page
when the route runs. The body is left out of an answer with status 204 or 304.

```alder fragment
File.status (File.text { contentType: "text/plain", body: "gone" } |> File.withStatus 410) == 410
File.status (File.withStatus 204 (File.text { contentType: "text/plain", body: "" })) == 204
```

### `File.withHeader`

```alder fragment
withHeader : { name: Text, value: Text } -> File -> File
```

Adds a header to a `File`.

Only these headers may be set: `Content-Disposition`, `Cache-Control`, `ETag`, `Last-Modified`,
`Retry-After`, `Content-Language` and any name starting with `X-`, in any letter case. The
router decides the rest: `Content-Type` comes from the `File`, `Set-Cookie` from the session,
`Location` from a redirect and `Content-Length` from the server. A name written in the code
that is not allowed is a compile error, and one that comes from a variable is dropped when
the route answers, with a line in the log. A header may be added more than once: an `X-`
header is then sent each time, and for the others the last value wins. A line break in a
value is changed to a space.

```alder fragment
File.text { contentType: "text/csv", body: "a,b" } |> File.withHeader { name: "Content-Disposition", value: "attachment; filename=\"data.csv\"" }
File.json (Json.int 1) |> File.withHeader { name: "Cache-Control", value: "max-age=60" }
```

### `File.status`

```alder fragment
status : File -> Int
```

The status of a `File`, such as 200.

```alder fragment
File.status (File.json Json.null) == 200
File.status (File.withStatus 404 (File.json Json.null)) == 404
```

### `File.contentType`

```alder fragment
contentType : File -> Text
```

The content type of a `File`.

When the content type is empty, as in `File.text { contentType: "", body: "x" }`, it is
`"application/octet-stream"`, which is also what the router sends.

```alder fragment
File.contentType (File.json Json.null) == "application/json"
File.contentType (File.text { contentType: "text/plain", body: "" }) == "text/plain"
File.contentType (File.text { contentType: "", body: "x" }) == "application/octet-stream"
```

### `File.body`

```alder fragment
body : File -> Bytes
```

The body of a `File`, as `Bytes`.

Use `Bytes.toText` to read it as text.

```alder fragment
File.body (File.text { contentType: "text/plain", body: "hé" }) == Text.toBytes "hé"
Bytes.length (File.body (File.text { contentType: "text/plain", body: "hé" })) == 3
```

## Float

Functions for `Float`, a number with a decimal point, and the constants that go with it.

A `Float` is a 64-bit floating-point number, so many decimals such as `0.1` are stored as the
nearest value that fits, and a calculation can be off in the last digit. Compare results
after rounding, not with `==`, unless the values are exact like `0.5` or `2.25`. Division by
zero and other impossible calculations are not failures: they give infinity or `NaN` (not a
number), which `Float.isInfinite` and `Float.isNaN` detect.

### `Float.round`

```alder fragment
round : Float -> Int
```

Rounds a `Float` to the nearest `Int`.

A value exactly halfway rounds away from zero, so `2.5` gives `3` and `-2.5` gives `-3`.
`NaN`, infinity and numbers too big for an `Int` give a result that depends on the platform.

```alder fragment
Float.round 2.4 == 2
Float.round 2.5 == 3
Float.round (-2.5) == -3
Float.round (-2.4) == -2
```

### `Float.floor`

```alder fragment
floor : Float -> Int
```

Rounds a `Float` down to the whole number below it.

Down means toward negative infinity, so `-2.5` gives `-3`. `NaN`, infinity and numbers too
big for an `Int` give a result that depends on the platform.

```alder fragment
Float.floor 2.9 == 2
Float.floor (-2.1) == -3
Float.floor 3.0 == 3
```

### `Float.ceil`

```alder fragment
ceil : Float -> Int
```

Rounds a `Float` up to the whole number above it.

Up means toward positive infinity, so `-2.5` gives `-2`. `NaN`, infinity and numbers too big
for an `Int` give a result that depends on the platform.

```alder fragment
Float.ceil 2.1 == 3
Float.ceil (-2.9) == -2
Float.ceil 3.0 == 3
```

### `Float.min`

```alder fragment
min : Float -> Float -> Float
```

The smaller of two `Float`s.

If either one is `NaN`, the result is `NaN`.

```alder fragment
Float.min 0.5 2.25 == 0.5
Float.min (-1.0) 1.0 == -1.0
Float.isNaN (Float.min (Float.sqrt (0.0 - 1.0)) 1.0) == True
```

### `Float.max`

```alder fragment
max : Float -> Float -> Float
```

The larger of two `Float`s.

If either one is `NaN`, the result is `NaN`.

```alder fragment
Float.max 0.5 2.25 == 2.25
Float.max (-1.0) 1.0 == 1.0
Float.isNaN (Float.max (Float.sqrt (0.0 - 1.0)) 1.0) == True
```

### `Float.abs`

```alder fragment
abs : Float -> Float
```

A `Float` without its sign.

```alder fragment
Float.abs (-2.25) == 2.25
Float.abs 2.25 == 2.25
Float.abs 0.0 == 0.0
```

### `Float.format`

```alder fragment
format : { decimals: Int } -> Float -> Text
```

Writes a `Float` as `Text` with a fixed number of decimals.

The text is padded with zeros to exactly that many decimals. It rounds the number as it
prints, halfway cases away from zero, working from the shortest decimal form of the `Float`:
`1.005` gives `"1.01"` with two decimals, even though the stored value is a hair below it. A
negative `decimals` counts as 0. A number that rounds to zero never gets a minus sign. `NaN`
and infinity come out as `"NaN"`, `"+Inf"` and `"-Inf"`.

```alder fragment
Float.format { decimals: 2 } 3.14159 == "3.14"
Float.format { decimals: 2 } 1.5 == "1.50"
Float.format { decimals: 2 } 1.005 == "1.01"
Float.format { decimals: 0 } 2.5 == "3"
Float.format { decimals: 1 } (-0.001) == "0.0"
Float.format { decimals: -1 } 7.6 == "8"
```

### `Float.sqrt`

```alder fragment
sqrt : Float -> Float
```

The square root of a `Float`.

The root of a negative number is `NaN`, not a failure.

```alder fragment
Float.sqrt 9.0 == 3.0
Float.sqrt 0.0 == 0.0
Float.isNaN (Float.sqrt (0.0 - 1.0)) == True
```

### `Float.exp`

```alder fragment
exp : Float -> Float
```

Raises `Float.e` to a power.

The result grows very fast: a large enough number gives infinity.

```alder fragment
Float.exp 0.0 == 1.0
Float.round (Float.exp 1.0 * 1000.0) == 2718
Float.isInfinite (Float.exp 1000.0) == True
```

### `Float.log`

```alder fragment
log : Float -> Float
```

The natural logarithm of a `Float`, which is the logarithm to base `Float.e`.

The log of zero is negative infinity, and the log of a negative number is `NaN`. Neither is a
failure.

```alder fragment
Float.log 1.0 == 0.0
Float.round (Float.log Float.e * 100.0) == 100
Float.isInfinite (Float.log 0.0) == True
Float.isNaN (Float.log (0.0 - 1.0)) == True
```

### `Float.pow`

```alder fragment
pow : Float -> Float -> Float -- Float.pow base exponent
```

Raises a `Float` to a power.

The first argument is the base and the second is the exponent. Impossible cases, such as a
negative base with a fractional exponent, give `NaN`.

```alder fragment
Float.pow 2.0 3.0 == 8.0
Float.pow 2.0 (-1.0) == 0.5
Float.pow 9.0 0.5 == 3.0
Float.pow 5.0 0.0 == 1.0
Float.isNaN (Float.pow (0.0 - 8.0) 0.5) == True
```

### `Float.truncate`

```alder fragment
truncate : Float -> Int -- rounds toward zero
```

Drops the decimals of a `Float`, rounding toward zero.

Toward zero means `2.9` gives `2` and `-2.9` gives `-2`. `NaN`, infinity and numbers too big
for an `Int` give a result that depends on the platform.

```alder fragment
Float.truncate 2.9 == 2
Float.truncate (-2.9) == -2
Float.truncate 5.0 == 5
```

### `Float.clamp`

```alder fragment
clamp : { min: Float, max: Float } -> Float -> Float -- like Int.clamp: below min gives min, else above max gives max
```

Keeps a `Float` between a lowest and a highest value.

It works like `Int.clamp`. A number below `min` gives `min`, otherwise a number above `max`
gives `max`, otherwise the number itself. The check for `min` comes first, so when `min` is
above `max` a number below `min` still gives `min`. `NaN` stays `NaN`.

```alder fragment
Float.clamp { min: 0.0, max: 1.0 } 2.25 == 1.0
Float.clamp { min: 0.0, max: 1.0 } (-0.5) == 0.0
Float.clamp { min: 0.0, max: 1.0 } 0.5 == 0.5
Float.clamp { min: 2.0, max: 1.0 } 0.5 == 2.0
Float.isNaN (Float.clamp { min: 0.0, max: 1.0 } (Float.sqrt (0.0 - 1.0))) == True
```

### `Float.pi`

```alder fragment
pi : Float
```

The number pi, the ratio of a circle's circumference to its diameter.

```alder fragment
Float.round (Float.pi * 100.0) == 314
Float.floor Float.pi == 3
```

### `Float.e`

```alder fragment
e : Float
```

The number e, the base of natural logarithms, about 2.718.

```alder fragment
Float.round (Float.e * 1000.0) == 2718
Float.floor Float.e == 2
```

### `Float.isNaN`

```alder fragment
isNaN : Float -> Bool
```

Tells whether a `Float` is `NaN`, short for "not a number".

`NaN` is what an impossible calculation gives, such as the square root of a negative number.
It is not equal to anything, not even to itself, so `==` cannot find it; use this instead.

```alder fragment
Float.isNaN (Float.sqrt (0.0 - 1.0)) == True
Float.isNaN 1.5 == False
Float.isNaN (1.0 / 0.0) == False
```

### `Float.isInfinite`

```alder fragment
isInfinite : Float -> Bool
```

Tells whether a `Float` is positive or negative infinity.

Dividing a nonzero `Float` by zero gives infinity. `NaN` is not infinite.

```alder fragment
Float.isInfinite (1.0 / 0.0) == True
Float.isInfinite (-1.0 / 0.0) == True
Float.isInfinite 1.5 == False
Float.isInfinite (Float.sqrt (0.0 - 1.0)) == False
```

### `Float.logBase`

```alder fragment
logBase : Float -> Float -> Float -- Float.logBase base x
```

The logarithm of a `Float` to a chosen base.

The first argument is the base and the second is the number, so `Float.logBase 2.0 8.0` asks
what power of 2 gives 8. It is computed as a division of natural logarithms, so the answer can
be off in the last digit; round it when you need a whole number. A zero or negative number,
and a base of 1, give infinity or `NaN`, not a failure.

```alder fragment
Float.round (Float.logBase 2.0 8.0) == 3
Float.round (Float.logBase 10.0 1000.0) == 3
Float.logBase 2.0 1.0 == 0.0
```

### `Float.sin`

```alder fragment
sin : Float -> Float
```

The sine of an angle given in radians.

```alder fragment
Float.sin 0.0 == 0.0
Float.round (Float.sin (Float.pi / 2.0)) == 1
```

### `Float.cos`

```alder fragment
cos : Float -> Float
```

The cosine of an angle given in radians.

```alder fragment
Float.cos 0.0 == 1.0
Float.round (Float.cos Float.pi) == -1
```

### `Float.tan`

```alder fragment
tan : Float -> Float
```

The tangent of an angle given in radians.

```alder fragment
Float.tan 0.0 == 0.0
Float.round (Float.tan (Float.pi / 4.0) * 100.0) == 100
```

### `Float.asin`

```alder fragment
asin : Float -> Float
```

The angle in radians whose sine is the given number.

The input must be from `-1.0` to `1.0`; anything else gives `NaN`. The result is from `-pi/2`
to `pi/2`.

```alder fragment
Float.asin 0.0 == 0.0
Float.round (Float.asin 1.0 * 100.0) == 157
Float.isNaN (Float.asin 2.0) == True
```

### `Float.acos`

```alder fragment
acos : Float -> Float
```

The angle in radians whose cosine is the given number.

The input must be from `-1.0` to `1.0`; anything else gives `NaN`. The result is from `0.0`
to `pi`.

```alder fragment
Float.acos 1.0 == 0.0
Float.round (Float.acos 0.0 * 100.0) == 157
Float.isNaN (Float.acos 2.0) == True
```

### `Float.atan`

```alder fragment
atan : Float -> Float
```

The angle in radians whose tangent is the given number.

The result is between `-pi/2` and `pi/2`. Use `Float.atan2` when you have both sides of the
triangle and need the right quarter of the circle.

```alder fragment
Float.atan 0.0 == 0.0
Float.round (Float.atan 1.0 * 100.0) == 79
```

### `Float.atan2`

```alder fragment
atan2 : Float -> Float -> Float -- Float.atan2 y x
```

The angle in radians of the point at `x` across and `y` up, measured from the positive x axis.

The first argument is `y` and the second is `x`. Unlike `Float.atan`, it uses both signs to
pick the right quarter of the circle, and the result is from `-pi` to `pi`.

```alder fragment
Float.atan2 0.0 1.0 == 0.0
Float.round (Float.atan2 1.0 1.0 * 100.0) == 79
Float.round (Float.atan2 1.0 (-1.0) * 100.0) == 236
Float.round (Float.atan2 (-1.0) (-1.0) * 100.0) == -236
```

## Fuzz

Generators of random values for property tests, and the properties that use them.

A property test runs its body on many values drawn at random (100 unless `alder test --fuzz`
says otherwise) and, when one fails, shrinks it to a simpler failing value. A `Fuzz.Fuzzer`
describes how to draw a value; it is an ordinary value, so a generator can be declared once
with a signature and shared by many tests. A `Fuzz.Property`, made by `Fuzz.forAll` or
`Fuzz.forAllTask`, can only be a test's body.

`Fuzz.any : Fuzzer a` is a special form, not declared in this file: the generator that
`alder test` derives from a type, the explicit form of a derived argument. The compiler gives
it the generator of the type the context asks for, so that type must be known where it is
written (E0038 otherwise; inside a test, a type left open is `Int`).

### `Fuzz.Fuzzer`

```alder fragment
type Fuzzer a
```

A description of how to draw a value of type `a` from random choices.

A `Fuzzer` draws nothing until a property test runs it. Each generator also has a simplest
value, such as 0 for `Fuzz.int`, and a failing value shrinks toward it. Build one with the
generators of this module and combine them with `Fuzz.map`, `Fuzz.map2` and `Fuzz.andThen`.

### `Fuzz.Property`

```alder fragment
type Property
```

A property: what `Fuzz.forAll` and `Fuzz.forAllTask` make, and the body of a property test.

Only a test's body can be one; anywhere else it is E0039.

### `Fuzz.forAll`

```alder fragment
forAll : Fuzzer a -> (a -> Bool) -> Property
```

A property that holds when a function gives `True` for every value a generator draws.

`Fuzz.forAll gen f` draws a value with `gen` and gives it to `f`, as many times as the test
runs; a value for which `f` gives `False`, or crashes, fails the test and is shrunk. It can
only be a test's body (E0039 anywhere else). For a property over several values, draw a
record with `Fuzz.pair` or `Fuzz.map2`.

### `Fuzz.forAllTask`

```alder fragment
forAllTask : Fuzzer a -> (a -> Task Bool) -> Property
```

A property whose check is a `Task`, for code that reads or writes the database.

It is `Fuzz.forAll` with a function that gives a `Task Bool`. Every value drawn runs against a
fresh database, clock, capabilities and browser of its own, so no run sees another's writes.
It can only be a test's body (E0039 anywhere else).

### `Fuzz.constant`

```alder fragment
constant : a -> Fuzzer a
```

A generator that always gives the same value.

It makes no random choice, so there is nothing to shrink.

```alder fragment
Fuzz.constant 5
Fuzz.map2 (\n s -> { n: n, s: s }) (Fuzz.constant 0) Fuzz.text
```

### `Fuzz.bool`

```alder fragment
bool : Fuzzer Bool
```

A generator of `True` and `False`, each as likely.

A failure shrinks toward `False`.

```alder fragment
Fuzz.bool
```

### `Fuzz.int`

```alder fragment
int : Fuzzer Int
```

A generator of whole numbers, mostly small and sometimes extreme.

About half the numbers are from 0 to 10, and the rest are larger, up to the whole range of
`Int`; a few are values at the edges where overflow and off-by-one bugs hide, such as the
largest and smallest `Int`, 2^31 and 2^32 and their neighbours, 255 and 256. Each is
negative half the time. A failure shrinks toward 0.

```alder fragment
Fuzz.int
```

### `Fuzz.intRange`

```alder fragment
intRange : { min: Int, max: Int } -> Fuzzer Int
```

A generator of whole numbers from `min` to `max`, both included.

It gives `min` every time when `max` is not above `min`. `min` and `max` themselves come up
often, and so do the numbers nearest 0. A failure shrinks toward the number in the range
nearest 0.

```alder fragment
Fuzz.intRange { min: 1, max: 6 }
Fuzz.intRange { min: -10, max: 10 }
```

### `Fuzz.float`

```alder fragment
float : Fuzzer Float
```

A generator of decimal numbers, always finite: never `NaN` and never infinite.

Most are whole numbers or short decimals, and some are the values that break float code:
`0`, `-0`, numbers as tiny as the smallest one `Float` holds, and numbers as huge as the
largest. A failure shrinks toward 0.

```alder fragment
Fuzz.float
```

### `Fuzz.floatRange`

```alder fragment
floatRange : { min: Float, max: Float } -> Fuzzer Float
```

A generator of decimal numbers from `min` to `max`, both included.

It gives `min` every time when `max` is not above `min`, or when either is `NaN`; an
infinite bound counts as the largest finite `Float`, with its sign. The number in the range
nearest 0 and the ends of the range come up often. A failure shrinks toward the number in
the range nearest 0.

```alder fragment
Fuzz.floatRange { min: 0.0, max: 1.0 }
```

### `Fuzz.text`

```alder fragment
text : Fuzzer Text
```

A generator of text: mostly short and ASCII, sometimes Unicode, sometimes empty.

About three texts in ten are empty, and most of the rest are a few characters long. About a
quarter of them may hold characters beyond ASCII. Every character is printable or one of
`\n`, `\t` and `\r`, so every value can be written as an Alder literal. A failure shrinks
toward shorter text and toward `a`.

```alder fragment
Fuzz.text
```

### `Fuzz.textOf`

```alder fragment
textOf : { min: Int, max: Int } -> Fuzzer Text
```

A generator of text whose length, counted in code points, is from `min` to `max`.

The characters are drawn as `Fuzz.text` draws them. A `min` below 0 counts as 0, and a `max`
below `min` counts as `min`.

```alder fragment
Fuzz.textOf { min: 1, max: 20 }
```

### `Fuzz.char`

```alder fragment
char : Fuzzer Text
```

A generator of a text that holds one code point.

It is printable or one of `\n`, `\t` and `\r`; about one in five is beyond ASCII.

```alder fragment
Fuzz.char
```

### `Fuzz.list`

```alder fragment
list : Fuzzer a -> Fuzzer (List a)
```

A generator of lists of values from another generator, biased to short lists.

About three lists in ten are empty and most of the rest have a few elements; now and then one
is long. It has no upper bound, so a list of lists can grow until a test case makes more
random choices than `alder test` allows (8192), which fails the test: use `Fuzz.listOf` to
bound the length. A failure shrinks toward a shorter list.

```alder fragment
Fuzz.list Fuzz.int
```

### `Fuzz.listOf`

```alder fragment
listOf : { min: Int, max: Int } -> Fuzzer a -> Fuzzer (List a)
```

A generator of lists whose length is from `min` to `max`.

A `min` below 0 counts as 0, and a `max` below `min` counts as `min`.

```alder fragment
Fuzz.listOf { min: 1, max: 5 } Fuzz.int
Fuzz.listOf { min: 0, max: 10 } (Fuzz.listOf { min: 0, max: 10 } Fuzz.bool)
```

### `Fuzz.maybe`

```alder fragment
maybe : Fuzzer a -> Fuzzer (Maybe a)
```

A generator of `Nothing`, or `Just` a value from another generator.

About three in four are `Just`. A failure shrinks toward `Nothing`.

```alder fragment
Fuzz.maybe Fuzz.int
```

### `Fuzz.result`

```alder fragment
result : Fuzzer e -> Fuzzer a -> Fuzzer (Result e a)
```

A generator of `Ok` with a value of the second generator, or `Err` with one of the first.

Each is as likely. A failure shrinks toward `Ok`.

```alder fragment
Fuzz.result Fuzz.text Fuzz.int
```

### `Fuzz.pair`

```alder fragment
pair : Fuzzer a -> Fuzzer b -> Fuzzer { first: a, second: b }
```

A generator of a record of two values, `first` from one generator and `second` from another.

It is the way to give `Fuzz.forAll` two values at once.

```alder fragment
Fuzz.pair Fuzz.int Fuzz.text
```

### `Fuzz.dict`

```alder fragment
dict : Fuzzer k -> Fuzzer v -> Fuzzer (Dict k v)
```

A generator of dictionaries, with keys from one generator and values from another.

The entries are drawn as a list of keys and values, as `Fuzz.list` draws, so most
dictionaries are small. A key drawn twice keeps the value drawn last, so a dictionary can
have fewer entries than were drawn. A failure shrinks toward fewer entries.

```alder fragment
Fuzz.dict Fuzz.text Fuzz.int
```

### `Fuzz.oneOf`

```alder fragment
oneOf : List (Fuzzer a) -> Fuzzer a
```

A generator that draws from one of several generators, each as likely.

A failure shrinks toward the first generator in the list. An empty list fails the test that
draws from it.

```alder fragment
Fuzz.oneOf [Fuzz.intRange { min: 0, max: 9 }, Fuzz.constant 100]
```

### `Fuzz.oneOfValues`

```alder fragment
oneOfValues : List a -> Fuzzer a
```

A generator that picks one of several values, each as likely.

A failure shrinks toward the first value in the list. An empty list fails the test that
draws from it.

```alder fragment
Fuzz.oneOfValues ["draft", "published", "archived"]
```

### `Fuzz.frequency`

```alder fragment
frequency : List { weight: Int, fuzzer: Fuzzer a } -> Fuzzer a
```

A generator that draws from one of several generators, more often the heavier its weight.

A generator of weight 3 is picked three times as often as one of weight 1. A weight of 0 or
less is never picked, and when no weight is above 0 the test that draws from it fails. A
failure shrinks toward the first generator whose weight is above 0.

```alder fragment
Fuzz.frequency [{ weight: 9, fuzzer: Fuzz.intRange { min: 0, max: 9 } }, { weight: 1, fuzzer: Fuzz.int }]
```

### `Fuzz.map`

```alder fragment
map : (a -> b) -> Fuzzer a -> Fuzzer b
```

Changes the values a generator draws with a function.

It shrinks as the generator it is given does.

```alder fragment
Fuzz.map (\n -> n * 2) Fuzz.int
```

### `Fuzz.map2`

```alder fragment
map2 : (a -> b -> c) -> Fuzzer a -> Fuzzer b -> Fuzzer c
```

Combines the values of two generators with a function.

It draws from the first, then from the second, and gives both values to the function.

```alder fragment
Fuzz.map2 (\name age -> { name: name, age: age }) Fuzz.text (Fuzz.intRange { min: 0, max: 120 })
```

### `Fuzz.andThen`

```alder fragment
andThen : (a -> Fuzzer b) -> Fuzzer a -> Fuzzer b
```

Chooses the next generator from the value an earlier one drew.

It draws from the generator it is given, gives the value to the function, and draws from the
generator the function returns. Use it when how to draw depends on what was drawn, such as a
length and then that many items.

```alder fragment
Fuzz.andThen (\n -> Fuzz.listOf { min: n, max: n } Fuzz.bool) (Fuzz.intRange { min: 0, max: 5 })
```

### `Fuzz.filter`

```alder fragment
filter : (a -> Bool) -> Fuzzer a -> Fuzzer a
```

Keeps only the values of a generator for which a predicate gives `True`.

A value the predicate rejects is not drawn again: that test case is thrown away and does not
count as a run. The health check fails the test when the predicate rejects more than 90% of
the first 50 cases, so a predicate that rejects most values is better written as a
generator that draws only the values it accepts.

```alder fragment
Fuzz.filter (\n -> n != 0) Fuzz.int
```

### `Fuzz.fromRandom`

```alder fragment
fromRandom : Random.Generator a -> Fuzzer a
```

Turns a seeded `Random.Generator` into a generator for property tests.

It draws a random `Random.Seed` and gives the `Random.Generator`'s value at that seed. A
failure shrinks the seed toward `Random.seed 0`, never the value inside, so it shrinks to a
nearby seed and not to the simplest value. A generator written with `Fuzz` shrinks better;
this one is for reusing a generator the app already has.

```alder fragment
Fuzz.fromRandom (Random.intRange { min: 1, max: 6 })
```

## Html

Builders for `Html` values that markup does not write, and the render of one to text.

Markup (`<p>…</p>`) is the usual way to make `Html`. These are for a value from `Text`, for
nothing, for nodes with no wrapper, and for the markup of a node. Text is escaped when it is
rendered, except through `Html.unsafe`.

### `Html.unsafe`

```alder fragment
unsafe : Text -> Html
```

Markup taken as it is, with nothing escaped.

Never use it for text a person typed: a `<script>` in it runs. Only use it for markup the app
itself wrote or has checked.

```alder fragment
Html.toText (Html.unsafe "<b>hi</b>") == "<b>hi</b>"
```

### `Html.none`

```alder fragment
none : Html
```

An `Html` that renders nothing.

Use it for the branch of a `match` that shows nothing.

```alder fragment
Html.toText Html.none == ""
```

### `Html.text`

```alder fragment
text : Text -> Html
```

Text as an `Html` node, escaped when it is rendered.

The characters `<`, `>`, `&`, `"` and `'` come out as their entities, so the text shows as
it was written and never as markup.

```alder fragment
Html.toText (Html.text "a < b") == "a &lt; b"
Html.toText (Html.text "<b>") == "&lt;b&gt;"
```

### `Html.fragment`

```alder fragment
fragment : List Html -> Html
```

The nodes side by side with no wrapper element (a doctype before `<html>`).

An empty list renders nothing.

```alder fragment
Html.toText (Html.fragment [Html.text "a", Html.text "b"]) == "ab"
Html.toText (Html.fragment []) == ""
```

### `Html.toText`

```alder fragment
toText : Html -> Text
```

The markup `File.html` serves for the node.

Text in the node is escaped. The links and buttons it holds are not followed, because no page
is running to receive them.

```alder fragment
Html.toText (Html.text "x") == "x"
```

## Http

`Task`s that make an HTTP request and give back the body.

A request answers with a `Result`: `Ok` with the body of a 2xx response, and `Err` with an
`Http.HttpError` for any other status or when there is no response. The `Task` itself does not
fail on a bad answer, so a failed request is a value to match on. A request gives up after 10
seconds. A step that is only an HTTP call opens no database transaction.

### `Http.HttpError`

```alder fragment
type HttpError
```

Why a request did not give a 2xx response: the status and the body (or the error text).

The status is 0 when there was no response at all, such as a refused connection or a timeout. It is
always written qualified, as `Http.HttpError`.

### `Http.get`

```alder fragment
get : { url: Text } -> Task (Result HttpError Text)
```

Sends a GET request and gives the body of the answer.

A 2xx status is `Ok` with the body as text. Any other status is `Err`, with the status and the
body (or the status text when the body is empty). When there is no response, such as a
refused connection or a timeout, it is `Err` with status 0.

```alder fragment
Http.get { url: "https://example.com/rates.json" }
```

### `Http.post`

```alder fragment
post : { url: Text, body: Text } -> Task (Result HttpError Text)
```

Sends a POST request with a text body and gives the body of the answer.

The body is sent as `text/plain; charset=utf-8`. The answer is read as `Http.get` reads it.

```alder fragment
Http.post { url: "https://example.com/hook", body: "todo created" }
```

## Int

Functions for `Int`, a whole number, and the refinement types that go with it.

An `Int` is a 64-bit number. Arithmetic that goes past the largest or smallest `Int` wraps
around instead of failing, and so do the functions here that multiply.

### `Int.Positive`

```alder fragment
type Positive = brand Int where True
```

An `Int` that is greater than zero.

A value is checked when it is decoded from a form or a route, and is trusted after
that. `Int.Positive 3` makes one from a literal, but applying the brand to an `Int`
is not checked: `Int.Positive 0` is accepted, so the caller must make sure the number is
above zero.

### `Int.NonNegative`

```alder fragment
type NonNegative = brand Int where True
```

An `Int` that is zero or greater.

A value is checked when it is decoded from a form or a route, and is trusted after
that. `Int.NonNegative 0` makes one from a literal, but applying the brand to an `Int`
is not checked: `Int.NonNegative (0 - 1)` is accepted, so the caller must make sure the number
is not negative.

### `Int.toFloat`

```alder fragment
toFloat : Int -> Float
```

Turns an `Int` into a `Float` with the same value.

A very large `Int` may lose its last digits, because a `Float` cannot hold every 64-bit
number exactly.

```alder fragment
Int.toFloat 3 == 3.0
Int.toFloat (-2) == -2.0
Int.toFloat 0 == 0.0
```

### `Int.min`

```alder fragment
min : Int -> Int -> Int
```

The smaller of two `Int`s.

```alder fragment
Int.min 3 5 == 3
Int.min (-1) 4 == -1
Int.min 2 2 == 2
```

### `Int.max`

```alder fragment
max : Int -> Int -> Int
```

The larger of two `Int`s.

```alder fragment
Int.max 3 5 == 5
Int.max (-1) (-4) == -1
Int.max 2 2 == 2
```

### `Int.abs`

```alder fragment
abs : Int -> Int
```

An `Int` without its sign.

The smallest possible `Int`, `-9223372036854775808`, has no positive twin, so it comes back
unchanged and still negative.

```alder fragment
Int.abs (-5) == 5
Int.abs 5 == 5
Int.abs 0 == 0
Int.abs (-9223372036854775808) == -9223372036854775808
```

### `Int.clamp`

```alder fragment
clamp : { min: Int, max: Int } -> Int -> Int
```

Keeps an `Int` between a lowest and a highest value.

A number below `min` gives `min`, otherwise a number above `max` gives `max`, otherwise the
number itself. The check for `min` comes first, so when `min` is above `max` a number below
`min` still gives `min`.

```alder fragment
Int.clamp { min: 0, max: 10 } 15 == 10
Int.clamp { min: 0, max: 10 } (-3) == 0
Int.clamp { min: 0, max: 10 } 4 == 4
Int.clamp { min: 5, max: 1 } 3 == 5
Int.clamp { min: 5, max: 1 } 9 == 1
```

### `Int.pow`

```alder fragment
pow : Int -> Int -> Int -- Int.pow base exponent; a negative exponent gives 0
```

Raises an `Int` to a whole-number power.

The first argument is the base and the second is the exponent. A negative exponent gives `0`
and is never a failure. Like `*`, the result wraps around when it does not fit in an `Int`.
Any base to the power `0` is `1`.

```alder fragment
Int.pow 2 10 == 1024
Int.pow 5 0 == 1
Int.pow (-2) 3 == -8
Int.pow 2 (-1) == 0
Int.pow 2 64 == 0
```

## Job

Background jobs: put a job on the queue from a `Task`, and ask how it is doing.

A module declares a job with `job Name = { … }` and its arm `Name.run`, and `Name { … }`
builds a `Job` value from the job's parameters. `Job.enqueue` and its variants store that
value in the running `Task`'s transaction, so the job is queued only if the `Task` succeeds,
and a worker runs it later, outside any request. Enqueueing does not count as a database
write for the rule that a `Task` writes one store (E0031).

### `Job.Id`

```alder fragment
type Id = brand Text where True
```

A job's id: 8 lowercase hex characters, a brand of `Text`.

`Job.enqueue` and its variants give it back, and `Job.status` takes it. Being a brand of
`Text`, a route, a session field, a table column or another job can carry it. It does not say
which store the job is in. In a `test`, `Job.Id "x"` is an id that names no job, and
`Job.status` of it is `Nothing`.

### `Job.State`

```alder fragment
type State = Queued | Running | Done | Dead | Cancelled
```

Where a job stands: waiting, running, finished, given up, or taken back.

`Queued` waits for its time and a free worker, `Running` is being run, `Done` finished, and
`Dead` failed on every attempt its `retries` allow, or had parameters this build cannot read.
`Cancelled` was taken back by `Job.cancel`, the Jobs page or `alder jobs cancel`.
The constructors are written bare, like `Just`. A constructor of the same name that the
module declares, or one of a module it imports, comes first; one of a module it does not
import does not.

### `Job.Status`

```alder fragment
type Status = { state: State, attempts: Int, error: Maybe Text, runAt: Maybe Time }
```

What `Job.status` gives: a job's state, attempts so far, last failure and due time.

`runAt` is the time the job is next due while it is `Queued`, and `Nothing` in any other
state. `error` is the text of the last failed attempt, and a job that is `Done` after a retry
keeps it.

### `Job.enqueue`

```alder fragment
enqueue : Job -> Task Id
```

Queues a job to run as soon as a worker is free, and gives its id.

The job is stored in the running `Task`'s transaction: it is kept when the `Task` succeeds
and dropped with its writes when it fails. A job's parameters take at most 64 KB as JSON; a
larger job fails the `Task`.

```alder fragment
Job.enqueue (Jobs.Nightly.Nightly {})
```

### `Job.enqueueAt`

```alder fragment
enqueueAt : Time -> Job -> Task Id
```

Queues a job that does not run before the given time.

It is `Job.enqueue` with a due time: no worker takes the job before it, and once it has come
the next free worker takes it when it next looks for work (an idle worker looks every second).

```alder fragment
Job.enqueueAt (Time.fromMillis 86400000) (Jobs.Nightly.Nightly {})
```

### `Job.enqueueIn`

```alder fragment
enqueueIn : Int -> Job -> Task Id
```

Queues a job that does not run before the given number of milliseconds from now.

It is `Job.enqueueAt` with the due time worked out from the clock the `Task` runs on, as
`Time.now` reads it: a number of 0 or less is now, the same as `Job.enqueue`.

```alder fragment
Job.enqueueIn 3600000 (Jobs.Nightly.Nightly {})
```

### `Job.enqueueOnce`

```alder fragment
enqueueOnce : Text -> Job -> Task Id
```

Queues a job unless one of its kind with the same key is queued or running.

When such a job exists, nothing is added and its id comes back. A key belongs to the job's
kind, so two kinds of job may use the same key, and a job that is `Done`, `Dead` or `Cancelled` frees it.

```alder fragment
Job.enqueueOnce "nightly" (Jobs.Nightly.Nightly {})
```

### `Job.cancel`

```alder fragment
cancel : Id -> Task Bool
```

Cancels a job, and says whether it did.

`True` when the job was queued or running and is now cancelled; `False` when there is no such
job, it is finished, or it is a schedule's queued tick. A running job is stopped at its next
step and its writes are undone. In a silo-bound `Task`, a job in the shared store is cancelled
at once, apart from the `Task`'s transaction.

```alder fragment
\(id : Job.Id) -> Job.cancel id
```

### `Job.status`

```alder fragment
status : Id -> Query (Maybe Status)
```

The status of the job with this id, `Nothing` when there is no such job.

A job is stored where the `Task` that queued it writes: in an app with silos, the silo a
route's `update` writes in, else the shared store. `Job.status` looks first in the silo its
page `requires` (or its `Task` writes in), then in the shared store, so a page that requires
what the queueing route requires finds the job. A job in another silo is `Nothing` here, and
so is a job removed after it finished (a week after `Done`, 30 days after `Dead`; in a silo,
once a job there is queued or runs after that, or at the latest when the app next starts).

```alder fragment
\(id : Job.Id) ->
  Job.status id |> Query.map (Maybe.map (\(s : Job.Status) -> s.state == Done))
```

## Json

Functions for `Json.Value`, a JSON document you build and write out.

A JSON answer is a `Json.Value`: build it with the functions of this module and write it with
`Json.encode`, or serve it with `File.json`. Do not build JSON as `Text` by hand: the encoder
escapes what a hand-built string forgets. A list of values is built with `List.map` and
`Json.list`, as in `xs |> List.map Json.int |> Json.list`, and a `Dict Text Json.Value`
becomes an object with `d |> Dict.toList |> Json.object`. Text that is already JSON can be
served with `File.text { contentType: "application/json", body: t }`; there is no way to put
it inside a `Json.Value`, because the encoder never writes text that is not JSON.

Two `Json.Value`s are equal when they encode to the same text, so `Json.int 1` equals
`Json.float 1.0`. A `Json.Value` can be a `Dict` key on the same rule, and it is shown, in
the REPL, in `Log` and in test failures, as its compact JSON.

### `Json.Value`

```alder fragment
type Value
```

A JSON value: `null`, a boolean, a number, a string, a list or an object.

Build one with the functions of this module and write it with `Json.encode` or `File.json`.

### `Json.null`

```alder fragment
null   : Value
```

The JSON value `null`.

```alder fragment
Json.encode Json.null == "null"
```

### `Json.bool`

```alder fragment
bool   : Bool -> Value
```

A JSON `true` or `false`.

```alder fragment
Json.encode (Json.bool True) == "true"
Json.encode (Json.bool False) == "false"
```

### `Json.int`

```alder fragment
int    : Int -> Value
```

A JSON number from an `Int`.

Every digit is written, however large the number is.

```alder fragment
Json.encode (Json.int 42) == "42"
Json.encode (Json.int (-7)) == "-7"
Json.encode (Json.int 9007199254740993) == "9007199254740993"
```

### `Json.float`

```alder fragment
float  : Float -> Value
```

A JSON number from a `Float`.

It writes the shortest decimal that reads back as the same number, and never uses an
exponent, so `Json.float 1.0` is `1` and a very large number is written with all its digits.
JSON has no spelling for `NaN` or the infinities, so they are written as `null`: test with
`Float.isNaN` and `Float.isInfinite` first when you want something else.

```alder fragment
Json.encode (Json.float 2.5) == "2.5"
Json.encode (Json.float 1.0) == "1"
Json.encode (Json.float 1e21) == "1000000000000000000000"
Json.encode (Json.float (0.0 / 0.0)) == "null"
Json.encode (Json.float (1.0 / 0.0)) == "null"
Json.encode (Json.float (-1.0 / 0.0)) == "null"
```

### `Json.string`

```alder fragment
string : Text -> Value
```

A JSON string from a `Text`.

Quotes, backslashes and control characters are escaped. The characters `<`, `>` and `&`, and
the line separators U+2028 and U+2029, are also written as `\u` escapes (`<` is written
`\u003c`). The text means the same to any JSON reader, and the encoded value is safe to put inside a
`<script>` element through `Html.unsafe`: it can neither end the element nor break a
JavaScript string.

```alder fragment
Json.encode (Json.string "Ada") == "\"Ada\""
Json.encode (Json.string "a\"b") == "\"a\\\"b\""
Json.encode (Json.string "a\\b") == "\"a\\\\b\""
Json.encode (Json.string "line\nbreak") == "\"line\\nbreak\""
Json.encode (Json.string "<b>&") == "\"\\u003cb\\u003e\\u0026\""
Json.encode (Json.string "é") == "\"é\""
```

### `Json.list`

```alder fragment
list   : List Value -> Value
```

A JSON array from a list of values.

```alder fragment
Json.encode (Json.list [Json.int 1, Json.string "two", Json.null]) == "[1,\"two\",null]"
Json.encode (Json.list []) == "[]"
Json.encode ([1, 2, 3] |> List.map Json.int |> Json.list) == "[1,2,3]"
```

### `Json.object`

```alder fragment
object : List { key: Text, value: Value } -> Value
```

A JSON object from a list of keys and values.

The members are written in the order of the list. When a key is given twice, it keeps the
position of its first appearance and takes the value of its last, as `Dict.insert` would, so
the output never repeats a key.

```alder fragment
Json.encode (Json.object [{ key: "name", value: Json.string "Ada" }, { key: "age", value: Json.int 36 }]) == "\{\"name\":\"Ada\",\"age\":36}"
Json.encode (Json.object [{ key: "a", value: Json.int 1 }, { key: "b", value: Json.int 2 }, { key: "a", value: Json.int 3 }]) == "\{\"a\":3,\"b\":2}"
Json.encode (Json.object []) == "\{}"
```

### `Json.encode`

```alder fragment
encode : Value -> Text
```

Writes a `Json.Value` as compact JSON text, with no whitespace.

```alder fragment
Json.encode (Json.object [{ key: "ok", value: Json.bool True }, { key: "items", value: Json.list [Json.int 1, Json.int 2] }]) == "\{\"ok\":true,\"items\":[1,2]}"
```

### `Json.encodeIndented`

```alder fragment
encodeIndented : Int -> Value -> Text
```

Writes a `Json.Value` as JSON text with one list item or object member per line.

The number is how many spaces to indent each level, and a key is followed by a colon and a
space. A list or an object with nothing in it stays `[]` or `{}`. A number that is `0` or less
is the same as `Json.encode`.

```alder fragment
Json.encodeIndented 2 (Json.object [{ key: "a", value: Json.int 1 }]) == "\{\n  \"a\": 1\n}"
Json.encodeIndented 2 (Json.list [Json.int 1, Json.list [Json.int 2]]) == "[\n  1,\n  [\n    2\n  ]\n]"
Json.encodeIndented 4 (Json.object [{ key: "a", value: Json.list [] }]) == "\{\n    \"a\": []\n}"
Json.encodeIndented 0 (Json.list [Json.int 1, Json.int 2]) == "[1,2]"
Json.encodeIndented (-1) (Json.list [Json.int 1, Json.int 2]) == "[1,2]"
```

## List

Functions for working with lists: building, slicing, searching, transforming and sorting them.

A list is never changed in place. Every function here gives back a new list and leaves the
one you passed in as it was.

### `List.length`

```alder fragment
length : List a -> Int
```

The number of elements in a list.

```alder fragment
List.length [10, 20, 30] == 3
List.length [] == 0
```

### `List.isEmpty`

```alder fragment
isEmpty : List a -> Bool
```

Tells whether a list has no elements.

```alder fragment
List.isEmpty [] == True
List.isEmpty [1] == False
```

### `List.head`

```alder fragment
head : List a -> Maybe a
```

The first element of a list, if there is one.

An empty list gives `Nothing`.

```alder fragment
List.head [1, 2, 3] == Just 1
List.head [] == Nothing
```

### `List.last`

```alder fragment
last : List a -> Maybe a
```

The last element of a list, if there is one.

An empty list gives `Nothing`.

```alder fragment
List.last [1, 2, 3] == Just 3
List.last [] == Nothing
```

### `List.take`

```alder fragment
take : Int -> List a -> List a
```

The first n elements of a list.

A list shorter than n comes back whole. An n of zero or less gives the empty list.

```alder fragment
List.take 2 [1, 2, 3] == [1, 2]
List.take 5 [1, 2] == [1, 2]
List.take 0 [1, 2] == []
```

### `List.drop`

```alder fragment
drop : Int -> List a -> List a
```

A list without its first n elements.

An n larger than the list gives the empty list. An n of zero or less gives the list whole.
Like `List.take`, it costs the same however long the list is.

```alder fragment
List.drop 1 [1, 2, 3] == [2, 3]
List.drop 5 [1, 2] == []
List.drop 0 [1, 2] == [1, 2]
```

### `List.reverse`

```alder fragment
reverse : List a -> List a
```

The same elements in the opposite order.

```alder fragment
List.reverse [1, 2, 3] == [3, 2, 1]
List.reverse [] == []
```

### `List.map`

```alder fragment
map : (a -> b) -> List a -> List b
```

Applies a function to every element and collects the results, in the same order.

The new list has as many elements as the old one.

```alder fragment
List.map (\n -> n * 2) [1, 2, 3] == [2, 4, 6]
List.map (\n -> n * 2) [] == []
```

### `List.filter`

```alder fragment
filter : (a -> Bool) -> List a -> List a
```

Keeps the elements for which the test gives `True`, in their original order.

If no element passes, the result is the empty list.

```alder fragment
List.filter (\n -> n > 1) [1, 2, 3] == [2, 3]
List.filter (\n -> n > 5) [1, 2, 3] == []
```

### `List.filterMap`

```alder fragment
filterMap : (a -> Maybe b) -> List a -> List b
```

Applies a function that may fail to every element and keeps only the values it found.

An element for which the function gives `Nothing` is left out. The results stay in input
order. This is `List.map` and `List.filter` in one pass.

```alder fragment
List.filterMap Text.toInt ["1", "x", "3"] == [1, 3]
List.filterMap Text.toInt ["x", "y"] == []
```

### `List.find`

```alder fragment
find : (a -> Bool) -> List a -> Maybe a
```

The first element for which the test gives `True`, if there is one.

It gives `Nothing` when no element passes, including for an empty list.

```alder fragment
List.find (\n -> n > 1) [1, 2, 3] == Just 2
List.find (\n -> n > 5) [1, 2, 3] == Nothing
```

### `List.any`

```alder fragment
any : (a -> Bool) -> List a -> Bool
```

Tells whether at least one element passes the test.

An empty list gives `False`, because no element passes.

```alder fragment
List.any (\n -> n > 2) [1, 2, 3] == True
List.any (\n -> n > 5) [1, 2, 3] == False
List.any (\n -> n > 5) [] == False
```

### `List.all`

```alder fragment
all : (a -> Bool) -> List a -> Bool
```

Tells whether every element passes the test.

An empty list gives `True`, because no element fails.

```alder fragment
List.all (\n -> n > 0) [1, 2, 3] == True
List.all (\n -> n > 1) [1, 2, 3] == False
List.all (\n -> n > 1) [] == True
```

### `List.member`

```alder fragment
member : a -> List a -> Bool
```

Tells whether a value is one of the elements of a list.

Elements are compared with `==`, so records and other lists match by their contents.

```alder fragment
List.member 2 [1, 2, 3] == True
List.member 5 [1, 2, 3] == False
List.member { id: 1 } [{ id: 1 }, { id: 2 }] == True
```

### `List.foldl`

```alder fragment
foldl : (a -> b -> b) -> b -> List a -> b
```

Combines all the elements into one value, working from the left.

The function receives an element and the value built so far, and starts with the second
argument. The order matters when the function is not symmetric. An empty list gives the start
value.

```alder fragment
List.foldl (\n acc -> acc + n) 0 [1, 2, 3] == 6
List.foldl (\n acc -> [n] ++ acc) [] [1, 2, 3] == [3, 2, 1]
List.foldl (\n acc -> acc + n) 7 [] == 7
```

### `List.sum`

```alder fragment
sum : List Int -> Int
```

The total of a list of whole numbers.

An empty list gives 0.

```alder fragment
List.sum [1, 2, 3] == 6
List.sum [] == 0
```

### `List.sumFloat`

```alder fragment
sumFloat : List Float -> Float
```

The total of a list of decimal numbers.

An empty list gives 0.0.

```alder fragment
List.sumFloat [1.5, 2.5] == 4.0
List.sumFloat [] == 0.0
```

### `List.sortBy`

```alder fragment
sortBy : (a -> Int) -> List a -> List a
```

Sorts a list from smallest to largest by a whole-number key taken from each element.

The sort is stable: elements with equal keys keep the order they had in the input.

```alder fragment
List.sortBy (\n -> n) [3, 1, 2] == [1, 2, 3]
List.sortBy (\p -> p.age) [{ name: "b", age: 2 }, { name: "a", age: 1 }, { name: "c", age: 1 }] == [{ name: "a", age: 1 }, { name: "c", age: 1 }, { name: "b", age: 2 }]
```

### `List.sortByText`

```alder fragment
sortByText : (a -> Text) -> List a -> List a
```

Sorts a list alphabetically by a text key taken from each element.

Keys are compared byte by byte, so every capital letter sorts before every lowercase letter.
The sort is stable: elements with equal keys keep the order they had in the input.

```alder fragment
List.sortByText (\w -> w) ["pear", "apple", "fig"] == ["apple", "fig", "pear"]
List.sortByText (\w -> w) ["b", "B", "a"] == ["B", "a", "b"]
```

### `List.indexed`

```alder fragment
indexed : List a -> List { index: Int, item: a }
```

Pairs every element with its position in the list, counting from zero.

```alder fragment
List.indexed ["a", "b"] == [{ index: 0, item: "a" }, { index: 1, item: "b" }]
List.indexed [] == []
```

### `List.range`

```alder fragment
range : Int -> Int -> List Int
```

The whole numbers from one number to another, counting up by one.

Both ends are included. When the first number is larger than the second, the result is the
empty list; `List.range` never counts down.

```alder fragment
List.range 1 4 == [1, 2, 3, 4]
List.range 3 3 == [3]
List.range 4 1 == []
```

### `List.concat`

```alder fragment
concat : List (List a) -> List a
```

Joins a list of lists into one list, in order.

```alder fragment
List.concat [[1, 2], [3], []] == [1, 2, 3]
List.concat [] == []
```

### `List.append`

```alder fragment
append : List a -> List a -> List a
```

Puts one list after another.

This is the same as the `++` operator.

```alder fragment
List.append [1, 2] [3] == [1, 2, 3]
List.append [] [3] == [3]
```

### `List.unique`

```alder fragment
unique : List a -> List a
```

Removes repeated elements, keeping the first of each.

Elements are compared with `==`, so records and lists match by their contents. It takes time
in proportion to the length of the list. The one exception is NaN, the Float that
`Float.sqrt (0.0 - 1.0)` gives: `==` says no NaN equals another, but here every NaN counts as
one element, so only the first is kept.

```alder fragment
List.unique [1, 2, 1, 3, 2] == [1, 2, 3]
List.unique [{ id: 1 }, { id: 1 }] == [{ id: 1 }]
List.length (List.unique [Float.sqrt (0.0 - 1.0), Float.sqrt (0.0 - 1.0), 1.0]) == 2
```

### `List.groupBy`

```alder fragment
groupBy : (a -> Text) -> List a -> Dict Text (List a)
```

Sorts the elements into groups, one group for each distinct text key.

Inside a group the elements keep their input order, and the keys come in the order they are
first seen. An empty list gives an empty dictionary.

```alder fragment
List.groupBy (\w -> Text.left 1 w) ["apple", "avocado", "banana"] == Dict.fromList [{ key: "a", value: ["apple", "avocado"] }, { key: "b", value: ["banana"] }]
Dict.keys (List.groupBy (\w -> Text.left 1 w) ["banana", "apple", "blueberry"]) == ["b", "a"]
List.groupBy (\w -> w) [] == Dict.empty
```

### `List.indexedMap`

```alder fragment
indexedMap : (Int -> a -> b) -> List a -> List b
```

Like `List.map`, but the function also receives each element's position, counting from zero.

```alder fragment
List.indexedMap (\i x -> i * x) [5, 5, 5] == [0, 5, 10]
List.indexedMap (\i x -> i) [] == []
```

### `List.map2`

```alder fragment
map2 : (a -> b -> c) -> List a -> List b -> List c
```

Combines two lists element by element with a function.

When the lists have different lengths, the result stops at the shorter one and the extra
elements are ignored.

```alder fragment
List.map2 (\a b -> a + b) [1, 2, 3] [10, 20, 30] == [11, 22, 33]
List.map2 (\a b -> a + b) [1, 2, 3] [10] == [11]
```

### `List.map3`

```alder fragment
map3 : (a -> b -> c -> d) -> List a -> List b -> List c -> List d
```

Combines three lists element by element with a function.

The result stops at the shortest of the three lists.

```alder fragment
List.map3 (\a b c -> a + b + c) [1, 2] [10, 20] [100, 200] == [111, 222]
List.map3 (\a b c -> a + b + c) [1, 2] [10, 20] [100] == [111]
```

### `List.foldr`

```alder fragment
foldr : (a -> b -> b) -> b -> List a -> b
```

Combines all the elements into one value, working from the right.

It is `List.foldl` in the opposite direction: the function receives an element and the value
built so far, but the last element is handled first. An empty list gives the start value.

```alder fragment
List.foldr (\n acc -> [n] ++ acc) [] [1, 2, 3] == [1, 2, 3]
List.foldr (\n acc -> acc * 10 + n) 0 [1, 2, 3] == 321
List.foldr (\n acc -> acc + n) 7 [] == 7
```

### `List.concatMap`

```alder fragment
concatMap : (a -> List b) -> List a -> List b
```

Applies a function that gives a list to every element and joins the results into one list.

The joined lists keep their order. Elements for which the function gives the empty list leave
nothing behind.

```alder fragment
List.concatMap (\n -> [n, n]) [1, 2] == [1, 1, 2, 2]
List.concatMap (\n -> []) [1, 2] == []
```

### `List.intersperse`

```alder fragment
intersperse : a -> List a -> List a
```

Puts a separator between every two neighbouring elements.

A list with fewer than two elements has no neighbours, so it comes back unchanged.

```alder fragment
List.intersperse 0 [1, 2, 3] == [1, 0, 2, 0, 3]
List.intersperse 0 [1] == [1]
List.intersperse 0 [] == []
```

### `List.partition`

```alder fragment
partition : (a -> Bool) -> List a -> { yes: List a, no: List a }
```

Splits a list in two: the elements that pass the test and the elements that do not.

The result is a record with a `yes` field and a `no` field. Both halves keep the input order.

```alder fragment
List.partition (\n -> n > 2) [1, 2, 3, 4] == { yes: [3, 4], no: [1, 2] }
List.partition (\n -> n > 2) [] == { yes: [], no: [] }
```

### `List.tail`

```alder fragment
tail : List a -> Maybe (List a)
```

A list without its first element, if the list has one.

An empty list gives `Nothing`, and a list of one element gives `Just []`. Like `List.drop`, it
costs the same however long the list is.

```alder fragment
List.tail [1, 2, 3] == Just [2, 3]
List.tail [1] == Just []
List.tail [] == Nothing
```

### `List.repeat`

```alder fragment
repeat : Int -> a -> List a
```

A list of n copies of one value.

An n of zero or less gives the empty list.

```alder fragment
List.repeat 3 "ha" == ["ha", "ha", "ha"]
List.repeat 0 "ha" == []
```

### `List.maximum`

```alder fragment
maximum : List Int -> Maybe Int
```

The largest number in a list, if the list has any.

An empty list gives `Nothing`.

```alder fragment
List.maximum [3, 9, 2] == Just 9
List.maximum [] == Nothing
```

### `List.minimum`

```alder fragment
minimum : List Int -> Maybe Int
```

The smallest number in a list, if the list has any.

An empty list gives `Nothing`.

```alder fragment
List.minimum [3, 9, 2] == Just 2
List.minimum [] == Nothing
```

### `List.maximumFloat`

```alder fragment
maximumFloat : List Float -> Maybe Float
```

The largest decimal number in a list, if the list has any.

An empty list gives `Nothing`. A NaN counts as smaller than every other number, so it is the
result only when every element is NaN.

```alder fragment
List.maximumFloat [1.5, 9.5, 2.5] == Just 9.5
List.maximumFloat [] == Nothing
List.maximumFloat [Float.sqrt (0.0 - 1.0), 2.5] == Just 2.5
```

### `List.minimumFloat`

```alder fragment
minimumFloat : List Float -> Maybe Float
```

The smallest decimal number in a list, if the list has any.

An empty list gives `Nothing`. A NaN counts as smaller than every other number, so a list that
contains one has a NaN as its minimum.

```alder fragment
List.minimumFloat [1.5, 9.5, 2.5] == Just 1.5
List.minimumFloat [] == Nothing
Maybe.map Float.isNaN (List.minimumFloat [2.5, Float.sqrt (0.0 - 1.0)]) == Just True
```

### `List.sortByFloat`

```alder fragment
sortByFloat : (a -> Float) -> List a -> List a
```

Sorts a list from smallest to largest by a decimal-number key taken from each element.

The sort is stable: elements with equal keys keep the order they had in the input. An element
whose key is NaN sorts before all the others.

```alder fragment
List.sortByFloat (\p -> p.price) [{ id: 1, price: 2.5 }, { id: 2, price: 1.5 }] == [{ id: 2, price: 1.5 }, { id: 1, price: 2.5 }]
List.map (\p -> p.id) (List.sortByFloat (\p -> p.price) [{ id: 1, price: 2.5 }, { id: 2, price: Float.sqrt (0.0 - 1.0) }]) == [2, 1]
```

## Log

`Task`s that write a line to the app's log.

A line goes to the app's logger at its level, and `ALDER_LOG` decides which levels are kept: the
default, `info`, drops `Log.debug`. A text longer than 8 KB is cut and ends with "…". A log
line is kept for a long time and read by people who are not the user, so log ids and
outcomes, never the values people typed.

### `Log.debug`

```alder fragment
debug : Text -> Task Unit
```

Writes a line at the `debug` level, the lowest.

It is written only when `ALDER_LOG` is `debug`.

```alder fragment
Log.debug "checking the due date"
```

### `Log.info`

```alder fragment
info : Text -> Task Unit
```

Writes a line at the `info` level, for what normally happens.

It is kept by the default `ALDER_LOG`.

```alder fragment
Log.info "todo created"
```

### `Log.warn`

```alder fragment
warn : Text -> Task Unit
```

Writes a line at the `warn` level, for something that is odd but handled.

It is kept unless `ALDER_LOG` is `error`.

```alder fragment
Log.warn "the payment service was slow"
```

### `Log.error`

```alder fragment
error : Text -> Task Unit
```

Writes a line at the `error` level, for something that went wrong.

It is kept at every `ALDER_LOG` level. Logging does not fail the `Task`.

```alder fragment
Log.error "the payment was refused"
```

## Maybe

Functions for a `Maybe`, a value that may be missing.

A `Maybe` is `Just x` when the value is there and `Nothing` when it is not.

### `Maybe.map`

```alder fragment
map : (a -> b) -> Maybe a -> Maybe b
```

Applies a function to the value inside a `Just`.

A `Nothing` stays `Nothing` and the function is not called.

```alder fragment
Maybe.map (\n -> n + 1) (Just 1) == Just 2
Maybe.map (\n -> n + 1) Nothing == Nothing
```

### `Maybe.andThen`

```alder fragment
andThen : (a -> Maybe b) -> Maybe a -> Maybe b
```

Applies a function that may itself have no answer to the value inside a `Just`.

The result is `Nothing` if the input is `Nothing` or if the function gives `Nothing`. Unlike
`Maybe.map`, it does not nest one `Maybe` inside another.

```alder fragment
Maybe.andThen Text.toInt (Just "42") == Just 42
Maybe.andThen Text.toInt (Just "x") == Nothing
Maybe.andThen Text.toInt Nothing == Nothing
```

### `Maybe.withDefault`

```alder fragment
withDefault : a -> Maybe a -> a
```

The value inside a `Just`, or a fallback when there is `Nothing`.

This is the same as the `??` operator.

```alder fragment
Maybe.withDefault 0 (Just 5) == 5
Maybe.withDefault 0 Nothing == 0
```

### `Maybe.isJust`

```alder fragment
isJust : Maybe a -> Bool
```

Tells whether a `Maybe` holds a value.

```alder fragment
Maybe.isJust (Just 1) == True
Maybe.isJust Nothing == False
```

### `Maybe.toList`

```alder fragment
toList : Maybe a -> List a
```

Turns a `Maybe` into a list: one element for `Just`, none for `Nothing`.

```alder fragment
Maybe.toList (Just 1) == [1]
Maybe.toList Nothing == []
```

## Notice

The old names of the `Response` toast builders, kept as aliases for one release.

Each function is exactly the `Response` function of the same kind (`with` is
`Response.toast`), and each use is the warning W0012. Write the `Response` names instead.

### `Notice.with`

```alder fragment
with : Text -> Response -> Response
```

Adds a toast to a response, the way a route shows a success message.

It is the old name of `Response.toast`: it adds an `info` toast (news that is neither good nor
bad), and a use of it is W0012. Use `Response.success` to say that something was done.

```alder fragment
Response.refresh |> Notice.with "Saved"
```

### `Notice.success`

```alder fragment
success : Text -> Response -> Response
```

Adds a `success` toast to a response.

It is the old name of `Response.success`, and a use of it is W0012.

```alder fragment
Response.refresh |> Notice.success "Saved"
```

### `Notice.warning`

```alder fragment
warning : Text -> Response -> Response
```

Adds a `warning` toast to a response.

It is the old name of `Response.warning`, and a use of it is W0012.

```alder fragment
Response.refresh |> Notice.warning "Saved, but the due date was left out"
```

### `Notice.error`

```alder fragment
error : Text -> Response -> Response
```

Adds an `error` toast to a response.

It is the old name of `Response.error`, and a use of it is W0012.

```alder fragment
Response.refresh |> Notice.error "Nothing was saved"
```

## Prelude

The types every module uses without importing, and `not`.

Most of them are built in and have no constructors you write: a `List`, a `Dict`, a `Query` or a
`Task` is made by functions of its module (`List.take`, `Task.succeed`). `Bool`, `Maybe` and
`Result` are unions with constructors you can use anywhere, unqualified. Their functions live in
the modules of the same name. A type, table, form or union of your own may not take the name of
any of these except `Order`, `Form`, `Table`, `Id`, `Insert`, `Select`, `Condition` and `Column`,
which apps do not write in their own signatures, so they stay free for names such as
`table Order`.

### `Int`

```alder fragment
type Int
```

A 64-bit whole number, positive, negative or zero.

Its functions are in the `Int` module.

### `Float`

```alder fragment
type Float
```

A 64-bit number with a decimal part.

Its functions are in the `Float` module.

### `Text`

```alder fragment
type Text
```

A piece of UTF-8 text, such as a name or a sentence.

There is no separate type for a single character. Its functions are in the `Text` module.

### `Unit`

```alder fragment
type Unit
```

The type of `()`, the value that carries nothing, for an effect that gives no answer.

`Task Unit` is a write that succeeds with nothing to report, such as `Db.delete`.

### `Date`

```alder fragment
type Date
```

A calendar day, with no time of day.

Its functions are in the `Date` module.

### `Time`

```alder fragment
type Time
```

An instant in time.

Its functions are in the `Time` module.

### `Never`

```alder fragment
type Never
```

A type with no value of its own, which the checker accepts where another type is expected.

The checker lets it stand in for any type, and markup built from it fits any `Html`. You
cannot write `Html Never` yourself, though: that is an error, so write `Html`.

### `Error`

```alder fragment
type Error
```

What went wrong in a request, as the app's `error` page receives it.

The app's `error` must have the type `Error -> Html`, and it renders the page shown when a
request fails.

### `Route`

```alder fragment
type Route
```

A request to one of the app's routes: an href, or the target of a redirect.

A route's name, given its parameters, is a `Route` (`Todos.Index.Index { filter: All }`).
Two routes are equal when they have the same method and the same address.

### `Job`

```alder fragment
type Job
```

A background job to run, with its parameters: what `Job.enqueue` queues.

A job's name, given its parameters, is a `Job` (`Signup.SendWelcome { user: id }`), as a
route's name makes a `Route`. Two jobs are equal when they are of the same job and have equal
parameters. The `Job` module queues them and reads their status.

### `Response`

```alder fragment
type Response
```

What a write route's update answers: where to go or what to show, and toasts.

It is built with the functions of the `Response` module (`Response.redirect`,
`Response.render`), and a toast is added to one with `|>`.

### `File`

```alder fragment
type File
```

A page's answer when its view returns File: a status, a content type, headers and a body, written as given.

A route whose `view` returns a `File` instead of `Html` is a file page, such as a download or a
JSON document.

### `Bytes`

```alder fragment
type Bytes
```

A sequence of bytes, such as the body of a File.

Its functions are in the `Bytes` module.

### `List`

```alder fragment
type List a
```

An ordered collection of values that all have the same type.

A list is written `[1, 2, 3]`, and its functions are in the `List` module.

### `Dict`

```alder fragment
type Dict k v
```

A collection of values looked up by a key.

A `Dict` is built with `Dict.fromList`. A key may be any type that can be compared with `==`.

### `Html`

```alder fragment
type Html
```

Markup: elements and text; its links and clicks name routes (it takes no type argument).

A route's `view` and a component return one, and two `Html` values are equal when their markup
is.

### `Query`

```alder fragment
type Query a
```

A read that gives an `a` when it runs.

A `Query` cannot write, and it can stop with `Query.notFound` or `Query.redirect`. A route's
`load` is one.

### `Task`

```alder fragment
type Task a
```

A write, or any step that may fail or wait, giving an `a`.

A route's `update` is one: it ends with a `Response`. It can fail with `Task.fail` and is
stepped through with `let _ <-`.

### `Form`

```alder fragment
type Form f
```

A name the Prelude declares for a form's values, which app code does not write, build or receive.

The name is declared but not reserved: an app may still declare its own `Form`. Nothing in the
checker, the generator or the runtime produces a `Form f`. A write route that
names a `form` gets the decoded record in `params.form`, whose type is the form's own record.

### `Table`

```alder fragment
type Table t
```

A table's name used as a value, such as `Todo` in `Db.get Todo id`.

Its functions are in the `Db` module.

### `Id`

```alder fragment
type Id t
```

The id of a row of the table `t`, the type of its `@id` column.

A table's brand (`Todo.TodoId`) is the id of its rows. `Db.insert` gives one back.

### `Insert`

```alder fragment
type Insert t
```

A new row of the table `t`: its record without `@id`, `@created` and `@updated`.

It is written as a record literal in the call to `Db.insert`.

### `Select`

```alder fragment
type Select t
```

A read of the rows of the table `t`, built up before it runs.

`Db.from` starts one, and `Db.where` and `Db.orderBy` narrow it.

### `Condition`

```alder fragment
type Condition t
```

A test on one row of the table `t`, built from a column.

It is made by a column's methods (`Todo.done.eq False`) and joined with `Db.and`, `Db.or` and
`Db.not`.

### `Order`

```alder fragment
type Order t
```

A sort order on the rows of the table `t`, built from a column.

It is made by `.asc` or `.desc` on a column (`Todo.created.desc`).

### `Column`

```alder fragment
type Column t a
```

A column of the table `t` that holds an `a`, such as `Todo.title`.

A column has methods that build a `Condition` or an `Order`.

### `Bool`

```alder fragment
type Bool = True | False
```

Either `True` or `False`.

`True` and `False` are literals. The logic on a `Bool` is `not`, `&&` and `||`, and `Bool.toText`
writes one as text.

### `Maybe`

```alder fragment
type Maybe a = Just a | Nothing
```

A value that may be missing: `Just x` when it is there, `Nothing` when it is not.

A `Maybe` is how a field or a lookup says it may have no value, and `??` gives a fallback for
`Nothing`. Its functions are in the `Maybe` module.

### `Result`

```alder fragment
type Result e a = Ok a | Err e
```

The outcome of something that may fail: `Ok a` when it worked, `Err e` when it did not.

The `e` is what went wrong, and the `a` is the answer. Its functions are in the `Result` module.

### `Doc`

```alder fragment
type Doc
```

A parsed page, the page the browser holds in a test.

It exists only inside a `test`: a type declaration that uses it is an error, and so is a route's
`view`. A `Doc` can be compared with `==`, and the functions that read one are in the `Test`
module.

### `Answer`

```alder fragment
type Answer = Shown
            | Rendered Route
            | Refreshed
            | Redirected Route
            | FormRejected (List { field: Text, message: Text })
            | PageMissing
            | RequestFailed Text
```

What a request came to, in a test: the page shown, a redirect, a refusal, a 404 or a failure.

It exists only inside a `test`. `Shown` is a page shown and `Rendered` a route's view patched
into the page. `Refreshed` is a write whose `update` answered `Response.refresh`, and
`Redirected` a redirect to a route. `FormRejected` carries each field's message, and
`PageMissing` is a 404. `RequestFailed` carries the failure's message.

### `not`

```alder fragment
not : Bool -> Bool
```

The opposite of a `Bool`: `True` becomes `False` and `False` becomes `True`.

```alder fragment
not True == False
not False == True
not (1 > 2) == True
```

## Query

Functions for `Query`, an effect that only reads.

A `Query` is what a GET route's `load` gives back. Like a `Task`, it is a description that the
runtime runs, and its steps are chained with `let x <-` in a block. A `Query` cannot write and
has no transaction. It cannot fail either (a missing row is `Nothing`, a database outage is
the app's `error` page), but it can stop with `Query.notFound` or `Query.redirect`. A `Query`
can be bound with `let x <-` in a `Task` block; a `Task` cannot be used in a `Query` block.
Two queries cannot be compared with `==`.

### `Query.succeed`

```alder fragment
succeed : a -> Query a
```

A `Query` that reads nothing and gives back the value.

It is how a `load` block ends, with the page's model.

```alder fragment
Query.succeed { open: 3 }
```

### `Query.map`

```alder fragment
map : (a -> b) -> Query a -> Query b
```

Runs a `Query` and changes its result with a function.

A `Query` that stops (not found, or a redirect) stays stopped, and the function is not
called.

```alder fragment
Query.map List.length (Db.from Todo |> Db.all)
```

### `Query.delay`

```alder fragment
delay : Int -> Query Unit
```

Waits a number of milliseconds, then goes on.

The number is clamped to 0 through 60000 (one minute): a negative number waits not at all, a
larger one a minute. The wait belongs to the request, and a request that is cancelled stops
waiting at once. In tests the clock moves instead: the test's clock jumps forward by the wait
and nothing sleeps. A `Query` holds no transaction, so waiting in one costs the database
nothing.

```alder fragment
Query.delay 2000
```

### `Query.all`

```alder fragment
all : List (Query a) -> Query (List a)
```

Runs the queries one after another and gives back their results in the same order.

One that stops (not found, or a redirect) stops it, and the ones after it do not run. An empty
list gives back an empty list.

```alder fragment
Query.all [Db.count (Db.from Todo), Db.count (Db.from User)]
Query.all []
```

### `Query.notFound`

```alder fragment
notFound : Query a
```

A `Query` that stops, which a route answers with a 404 and the app's `notFound` page.

Nothing after it runs, and the page's `view` is not called. Bound in a `Task` block, it fails
the `Task` the same way `Task.notFound` does, and the `Task`'s writes are rolled back.

```alder fragment
\(todo : Maybe Todo) ->
  match todo {
    Just t -> Query.succeed t.title,
    Nothing -> Query.notFound,
  }
```

### `Query.redirect`

```alder fragment
redirect : Route -> Query a
```

A `Query` that stops and sends the browser to a route with a 303.

The route value can be the same page with other parameters, another page, or `Url.external`.
Nothing after it runs, and the page's `view` is not called: a GET route's `load` uses it to
send the browser on without showing anything. Bound in a `Task` block, it is a failure: the
`Task`'s writes and `Session.put` changes are undone and the request is answered with the 303
(in `Test.run` it is `Err "redirected to …"` with the URL). To write and then go, end the
`Task` with `Response.redirect` instead. A redirect to a URL that is only a fragment, such as
the `"#"` that `Url.external` makes of an unsafe URL, is refused with a 500, since it would
load the same page again.

```alder fragment
Query.redirect (Auth.Login.Login {})
```

## Random

Random numbers and ids: fresh ones from an effect, and repeatable ones from a seed.

`Random.int` and `Random.id` are effects (a `Task`): they give a different value every time
they run. The rest of the module is pure: a `Random.Generator` describes how to draw a value,
and `Random.step` draws it from a `Random.Seed` and gives back the seed for the next draw. The
same seed gives the same values on every run, on every platform and in every version of Alder,
so seeded draws suit test data, simulations and stable buckets, and work in `load`, in `view`
and in any function. None of it is for secrets: use `Random.id` for those.

### `Random.int`

```alder fragment
int : { min: Int, max: Int } -> Task Int
```

A random whole number from `min` to `max`, both included.

Every number in the range is equally likely. It gives `min` when `max` is not above `min`. A
different number comes on every run, so it is a `Task`, and the same call in a test is not
repeatable: draw from a `Random.Seed` for a number that must be the same every time. It is
not for secrets.

```alder fragment
Random.int { min: 1, max: 6 }
```

### `Random.id`

```alder fragment
id : Task Text
```

A new random id, 20 characters long.

It is made of letters, digits, `-` and `_`, so it is safe in a URL, and it comes from the
operating system's secure random source, so it can serve as an unguessable token for a reset
link. A different one comes on every run, so it is a `Task`.

```alder fragment
Random.id
```

### `Random.Seed`

```alder fragment
type Seed
```

The state of the random number generator: one 64-bit number.

A seed is a value, so drawing from it does not change it; `Random.step` gives back the seed
to use for the next draw. Reusing a seed gives the same draw again. A seed is shown as the
expression that makes it, such as `Random.seed 42` or `Random.seed (-7)`. Make one with
`Random.seed`.

### `Random.Generator`

```alder fragment
type Generator a
```

A description of how to draw a value of type `a` from a seed.

A `Generator` does nothing until `Random.step` runs it. Build one with `Random.intRange` and
the other generators of this module, and combine them with `Random.map`, `Random.map2`,
`Random.list` and `Random.andThen`.

### `Random.seed`

```alder fragment
seed : Int -> Seed
```

Makes a `Seed` from a whole number.

Every `Int` is a seed, `0` and the negative ones included, and different numbers give
different seeds. To make one from data, hash it, as in
`Random.seed (Text.hash (Text.join "/" [county, key]))`: every county and key then gets its
own stream of numbers, the same on every run.

```alder fragment
Random.seed 42 == Random.seed 42
(Random.seed 0 == Random.seed 1) == False
Random.seed (-1) == Random.seed (-1)
```

### `Random.step`

```alder fragment
step : Generator a -> Seed -> { value: a, seed: Seed }
```

Draws a value with a `Generator`, from a `Seed`.

It gives a record with the `value` drawn and the `seed` to use for the next draw. Reuse the
same seed and the same value comes out; thread the new seed through to get a stream of
different values. Each of `intRange`, `floatRange`, `normal`, `uniform` and
`weighted` takes one step, and `constant` and `map` take none, so a draw's place in the
stream is fixed by the program, except after `andThen`, or `list` with a count that was itself
drawn, which take as many steps as the values drawn decide. Adding a draw at the end of a
program does not change the draws before it.

```alder fragment
Random.step (Random.intRange { min: 1, max: 6 }) (Random.seed 42) == { value: 2, seed: Random.seed (-7046029254386353089) }
(Random.step (Random.intRange { min: 1, max: 6 }) (Random.seed 42)).value == 2
```

### `Random.constant`

```alder fragment
constant : a -> Generator a
```

A `Generator` that always gives the same value.

It takes no step: the seed that comes out is the seed that went in.

```alder fragment
Random.step (Random.constant 5) (Random.seed 42) == { value: 5, seed: Random.seed 42 }
```

### `Random.intRange`

```alder fragment
intRange : { min: Int, max: Int } -> Generator Int
```

A `Generator` of whole numbers from `min` to `max`, both included.

It gives `min` when `max` is not above `min`. It always takes one step, even then.

```alder fragment
(Random.step (Random.intRange { min: 1, max: 6 }) (Random.seed 42)).value == 2
(Random.step (Random.intRange { min: -3, max: 3 }) (Random.seed 42)).value == 2
(Random.step (Random.intRange { min: 5, max: 5 }) (Random.seed 42)).value == 5
(Random.step (Random.intRange { min: 3, max: 1 }) (Random.seed 42)).value == 3
(Random.step (Random.intRange { min: 5, max: 5 }) (Random.seed 42)).seed == (Random.step (Random.intRange { min: 1, max: 6 }) (Random.seed 42)).seed
```

### `Random.floatRange`

```alder fragment
floatRange : { min: Float, max: Float } -> Generator Float
```

A `Generator` of decimal numbers from `min` up to, but not including, `max`.

It almost never gives `max` itself: the draw is a little below the top of the range, but the sum is
rounded, so a top draw can round up to exactly `max`, about once in 2^52 draws. It gives `min` when `max` is not above `min`. It always takes
one step.

```alder fragment
Float.format { decimals: 4 } (Random.step (Random.floatRange { min: 10.0, max: 20.0 }) (Random.seed 42)).value == "17.4156"
(Random.step (Random.floatRange { min: 5.0, max: 5.0 }) (Random.seed 42)).value == 5.0
(Random.step (Random.floatRange { min: 5.0, max: 1.0 }) (Random.seed 42)).value == 5.0
```

### `Random.normal`

```alder fragment
normal : { mean: Float, sd: Float } -> Generator Float
```

A `Generator` of decimal numbers that follow a normal distribution, the bell curve.

Most draws fall near the `mean`, and `sd`, the standard deviation, says how far they spread:
about two draws in three are within one `sd` of the mean. It turns one even draw into a
normal one with Acklam's approximation of the inverse of the normal distribution, accurate to
about 1e-9, and it always takes one step. An `sd` of `0.0` gives the `mean` every time.

```alder fragment
Float.format { decimals: 4 } (Random.step (Random.normal { mean: 100.0, sd: 15.0 }) (Random.seed 42)).value == "109.7227"
(Random.step (Random.normal { mean: 10.0, sd: 0.0 }) (Random.seed 42)).value == 10.0
```

### `Random.uniform`

```alder fragment
uniform : a -> List a -> Generator a
```

A `Generator` that picks one of the values, each equally likely.

It is written `Random.uniform first rest`, the first value and then the others, so that the
list of choices can never be empty. It always takes one step.

```alder fragment
(Random.step (Random.uniform "a" ["b", "c"]) (Random.seed 42)).value == "b"
(Random.step (Random.uniform "only" []) (Random.seed 42)).value == "only"
```

### `Random.weighted`

```alder fragment
weighted : { weight: Float, value: a } -> List { weight: Float, value: a } -> Generator a
```

A `Generator` that picks one of the values, more often the heavier its weight.

It is written `Random.weighted first rest`, like `Random.uniform`, with a record of a `weight`
and a `value` for each. A value is picked with a probability in proportion to its weight, so a
value of weight 3.0 comes up three times as often as one of weight 1.0. A negative weight
counts as its absolute value. When every weight is zero the first value is picked. It always
takes one step.

```alder fragment
(Random.step (Random.weighted { weight: 1.0, value: "x" } [{ weight: 3.0, value: "y" }]) (Random.seed 42)).value == "y"
(Random.step (Random.weighted { weight: 0.0, value: "x" } [{ weight: 2.0, value: "y" }]) (Random.seed 42)).value == "y"
(Random.step (Random.weighted { weight: -1.0, value: "x" } [{ weight: 0.0, value: "y" }]) (Random.seed 42)).value == "x"
(Random.step (Random.weighted { weight: 0.0, value: "x" } [{ weight: 0.0, value: "y" }]) (Random.seed 42)).value == "x"
```

### `Random.list`

```alder fragment
list : Int -> Generator a -> Generator (List a)
```

A `Generator` of a list of draws from another `Generator`.

`Random.list n g` draws `n` values from `g` in turn, each from the seed the one before left,
so it takes `n` steps. It gives `[]` and takes no step when `n` is not above 0.

```alder fragment
(Random.step (Random.list 5 (Random.intRange { min: 1, max: 6 })) (Random.seed 42)).value == [2, 2, 1, 1, 5]
(Random.step (Random.list 0 (Random.intRange { min: 1, max: 6 })) (Random.seed 42)) == { value: [], seed: Random.seed 42 }
(Random.step (Random.list (-2) (Random.intRange { min: 1, max: 6 })) (Random.seed 42)).value == []
```

### `Random.map`

```alder fragment
map : (a -> b) -> Generator a -> Generator b
```

Changes the values a `Generator` draws with a function.

It takes no step of its own: it takes the steps of the `Generator` it is given.

```alder fragment
(Random.step (Random.map (\n -> n * 2) (Random.intRange { min: 1, max: 6 })) (Random.seed 42)).value == 4
```

### `Random.map2`

```alder fragment
map2 : (a -> b -> c) -> Generator a -> Generator b -> Generator c
```

Combines two `Generator`s with a function.

It runs the first, then the second from the seed the first leaves, and gives both values to
the function, so it takes the steps of both.

```alder fragment
(Random.step (Random.map2 (\x y -> [x, y]) (Random.intRange { min: 1, max: 6 }) (Random.intRange { min: 1, max: 6 })) (Random.seed 42)).value == [2, 2]
```

### `Random.andThen`

```alder fragment
andThen : (a -> Generator b) -> Generator a -> Generator b
```

Chooses the next `Generator` from the value an earlier one drew.

It runs the first `Generator`, gives its value to the function, and runs the `Generator` the
function returns from the seed the first left, so it takes the steps of both. Use it when how
to draw depends on what was drawn, such as a length and then that many items.

```alder fragment
(Random.step (Random.andThen (\n -> Random.list n (Random.intRange { min: 1, max: 6 })) (Random.intRange { min: 1, max: 3 })) (Random.seed 42)).value == [2, 1]
```

## Response

The answers a write route's `update` can give: a redirect, a re-render, a refresh or a refusal, with toasts.

A `Response` is built with these functions, each named after the HTTP it produces, and a
toast is added to one with `|>`. A `Response` can be compared with `==`, and two routes are
equal when they lead to the same address.

### `Response.redirect`

```alder fragment
redirect : Route -> Response
```

Sends the browser to a GET route with a 303.

Use it after creating something, so that a browser refresh does not send the form twice. It
goes to the route even when that is the page the browser is on. The route must be a GET route.

```alder fragment
Response.redirect (Todos.Index.Index { filter: Active })
```

### `Response.render`

```alder fragment
render : Route -> Response
```

Shows a GET route again in place, as one element with a 200.

The runtime runs the route's `load` and renders its `view`; the element replaces the element
with the same `id` on the page, so the view needs a root element with an `id` or nothing is
patched. Use it for something already on the page, such as a toggled row. The route must be a
GET route.

```alder fragment
Response.render (Todos.Index.Index { filter: All })
```

### `Response.refresh`

```alder fragment
refresh : Response
```

Loads the page the browser is on again, and sends the regions of it that changed, with a 200.

Use it when things appear or disappear, or when more than one place changes. It costs what
loading the page costs, twice: the page is rendered before the `update` to know what changed.

```alder fragment
Response.refresh
```

### `Response.invalid`

```alder fragment
invalid : Text -> Response
```

Refuses a submission for a reason only `update` can know, with a 422.

It shows the page the browser is on again with the values that were posted, and the text in
the form's `<Errors form />`. It is a successful result: the `Task` that gave it commits, so
its writes and `Session.put` changes are kept.

```alder fragment
Response.invalid "That name is already taken"
```

### `Response.toast`

```alder fragment
toast : Text -> Response -> Response
```

Adds an `info` toast to a response, for news that is neither good nor bad.

The toast is one more event on the response, shown once after the response is applied, and
the response is otherwise unchanged. Toasts added one after another show in that order.

```alder fragment
Response.refresh |> Response.toast "Showing archived posts"
```

### `Response.success`

```alder fragment
success : Text -> Response -> Response
```

Adds a `success` toast to a response, to say that what the person asked for was done.

It is `Response.toast` with the kind `success`.

```alder fragment
Response.redirect (Todos.Index.Index { filter: All }) |> Response.success "Saved"
```

### `Response.warning`

```alder fragment
warning : Text -> Response -> Response
```

Adds a `warning` toast to a response, to say that it was done with something to look at.

It is `Response.toast` with the kind `warning`.

```alder fragment
Response.refresh |> Response.warning "Saved, but the image was too large and was left out"
```

### `Response.error`

```alder fragment
error : Text -> Response -> Response
```

Adds an `error` toast to a response, to say that it was not done.

It is `Response.toast` with the kind `error`. An error toast stays for ten seconds, the others
five. A form's own problems are better told with `Response.invalid`, next to the form.

```alder fragment
Response.refresh |> Response.error "Error: try again"
```

## Result

Functions for a `Result`, the outcome of something that can fail.

A `Result` is `Ok x` with the value on success and `Err e` with the reason on failure.

### `Result.map`

```alder fragment
map : (a -> b) -> Result e a -> Result e b
```

Applies a function to the value inside an `Ok`.

An `Err` stays as it is and the function is not called.

```alder fragment
Result.map (\n -> n + 1) (Ok 1) == Ok 2
Result.map (\n -> n + 1) (Err "bad") == Err "bad"
```

### `Result.mapErr`

```alder fragment
mapErr : (e -> f) -> Result e a -> Result f a
```

Applies a function to the reason inside an `Err`, for example to rewrite an error message.

An `Ok` stays as it is and the function is not called.

```alder fragment
Result.mapErr (\e -> "failed: " ++ e) (Err "bad") == Err "failed: bad"
Result.mapErr (\e -> "failed: " ++ e) (Ok 1) == Ok 1
```

### `Result.andThen`

```alder fragment
andThen : (a -> Result e b) -> Result e a -> Result e b
```

Applies a function that may itself fail to the value inside an `Ok`.

The result is the first `Err` met, either the input's or the function's. Unlike `Result.map`,
it does not nest one `Result` inside another, so steps that can fail chain one after another.

```alder fragment
Result.andThen (\n -> if n > 0 then Ok n else Err "not positive") (Ok 5) == Ok 5
Result.andThen (\n -> if n > 0 then Ok n else Err "not positive") (Ok 0) == Err "not positive"
Result.andThen (\n -> Ok n) (Err "first") == Err "first"
```

### `Result.withDefault`

```alder fragment
withDefault : a -> Result e a -> a
```

The value inside an `Ok`, or a fallback when the result is an `Err`.

The reason in the `Err` is thrown away.

```alder fragment
Result.withDefault 0 (Ok 5) == 5
Result.withDefault 0 (Err "bad") == 0
```

### `Result.toMaybe`

```alder fragment
toMaybe : Result e a -> Maybe a
```

Turns a `Result` into a `Maybe`, forgetting the reason for a failure.

An `Ok` becomes a `Just` and an `Err` becomes `Nothing`.

```alder fragment
Result.toMaybe (Ok 1) == Just 1
Result.toMaybe (Err "bad") == Nothing
```

## Session

Changing the session, the record an app keeps for each visitor in a signed cookie.

The app declares the session's fields with `session { … }` in its `App` module. A route's
`load` and `update` get the session as an argument; only `Session.put` changes it.

### `Session.put`

```alder fragment
put : (Session -> Session) -> Task Unit
```

Changes the session with a function that gets the current session and gives back the new one.

The change belongs to the running `Task`: when the `Task` succeeds it is kept and the cookie
is stored, and when the `Task` fails it is undone like a database write. The scoped `Db`
operations that run after it in the same `Task` act for the session as it now is. A session
field declared `via` a membership table can be set to `Just v` only when that table has a row
for the session's user and `v`; otherwise the `Task` fails with a message such as
"Session.put org: no Membership for this user and org". Setting such a field to `Nothing` is
always allowed, and a session whose `user` becomes `Nothing` has every `via` field cleared.

```alder fragment
Session.put (\s -> { s | user: Nothing, role: Member })
```

## Task

Functions for `Task`, an effect that may write to the database, the session or a capability.

A `Task` is a description of work, not its result: nothing runs while it is built, and the
runtime runs it when a route's `update` (or a test) hands it over. Steps are chained with
`let x <- e` in a block; there is no `andThen`. A `Query` can be bound with `let x <-` inside
a `Task` block. All the writes of one `Task` run in one transaction: when the `Task` fails,
they are rolled back and its `Session.put` changes are undone. Two tasks cannot be compared
with `==`.

### `Task.succeed`

```alder fragment
succeed : a -> Task a
```

A `Task` that does nothing and gives back the value.

It is how a block ends: the last line of an `update` is usually `Task.succeed` with the
`Response` to send.

```alder fragment
Task.succeed (Response.redirect (Todos.Index.Index { filter: All }))
Task.succeed 42
```

### `Task.fail`

```alder fragment
fail : Text -> Task a
```

A `Task` that fails with the message.

A failure stops the `Task`: nothing after it runs, every database write it made is rolled
back, its `Session.put` changes are undone, and a route answers with the app's `error` page
(a 500). In `Test.run` the failure is `Err` with the message. There is no way to catch a
failure; a step that may go wrong in an expected way gives a `Result` instead.

```alder fragment
Task.fail "the payment was refused"
```

### `Task.notFound`

```alder fragment
notFound : Task a
```

A `Task` that fails as "not found", which a route answers with a 404.

It fails the way `Task.fail` does, rolling back the `Task`'s writes and session changes, but
the route answers 404 with the app's `notFound` page instead of the `error` page. Use it when
a request names something that is not there. In `Test.run` it is `Err "not found"`.

```alder fragment
\(todo : Maybe Todo) ->
  match todo {
    Just t -> Db.update Todo t.id (\x -> { x | done: True }),
    Nothing -> Task.notFound,
  }
```

### `Task.map`

```alder fragment
map : (a -> b) -> Task a -> Task b
```

Runs a `Task` and changes its result with a function.

A `Task` that fails stays failed, and the function is not called.

```alder fragment
Task.map (\id -> [id]) (Db.insert Todo { title: Text.NonEmpty "Buy milk", due: Nothing, owner: User.UserId "u" })
```

### `Task.sleep`

```alder fragment
sleep : Int -> Task Unit
```

Waits a number of milliseconds, then goes on.

The number is clamped to 0 through 60000 (one minute): a negative number waits not at all, a
larger one a minute. The wait belongs to the request, and a request that is cancelled stops
waiting at once. In tests the clock moves instead: the test's clock jumps forward by the wait
and nothing sleeps. A `Task` keeps its transaction open from its first database step, so a
sleep after a write makes other writers wait too: sleep before writing.

```alder fragment
Task.sleep 500
```

### `Task.all`

```alder fragment
all : List (Task a) -> Task (List a)
```

Runs the tasks one after another and gives back their results in the same order.

The first one that fails stops it, and the ones after it do not run. An empty list gives
back an empty list. Every element must be a `Task`: a `Query` in the list is a type error, so
bind it with `let x <-` first.

```alder fragment
Task.all [Db.delete Todo (Todo.TodoId "a"), Db.delete Todo (Todo.TodoId "b")]
Task.all []
```

## Test

Steps and queries for tests: failing tasks, fakes, the clock, and an in-memory browser.

The steps run a task that may fail (`Test.run`), put a capability's fake in place
(`Test.fake`), set and move the test's clock, and use the app's pages through a browser.
Only a `test` or a test helper may use this module; anywhere else, such as a route's `view`,
a use is E0040. The steps are `Task` values, written with `let x <-` in a test's block. Each
test has its own fresh database, clock, capabilities and browser, and every effect it runs is
recorded on its trace. The queries at the end (`Test.text`, `Test.has` and the others) are
ordinary functions that read a `Doc`, the parsed page a browser step gives.

### `Test.run`

```alder fragment
run : Task a -> Task { value: Result Text a, trace: List Text }
```

Runs a task that may fail, and gives what it came to instead of failing the test.

`value` is `Ok` with the task's result, or `Err` with the failure's message when the task
fails: `Task.fail` gives its own text, `Task.notFound` gives "not found", a
`Query.redirect` gives "redirected to" and the URL, and an error from a capability's adapter
gives the capability's name and the error. A task that fails has its database writes undone
and the session put back as it was. `trace` is the steps the task took, without the
`Test.run` step itself. Any other error, such as a browser step that fails, still fails the
test.

```alder fragment
Test.run (Task.fail "out of stock")
Test.run (Task.fail "out of stock") |> Task.map (\r -> r.value == Err "out of stock")
```

### `Test.trace`

```alder fragment
trace : Task (List Text)
```

The steps the test has taken so far, in order.

Each step is named as the trace shows it, such as `"Db.get Todo"` or `"Session.put"`. The
`Test.trace` step itself is not in the list.

```alder fragment
Test.trace |> Task.map (\steps -> List.member "Session.put" steps)
```

### `Test.fake`

```alder fragment
fake : Capability -> Task Unit
```

Puts a capability's `fake` in place of its adapter for the rest of the test.

It is a special form, not a function: its argument is the name of a capability whose module
declares a `fake` (`Stripe`, or `Payments.Stripe` through an import), never an expression. So
`Test.fake` cannot be applied to a variable, piped into, or left without its argument. A
capability without a `fake` is E0003. The routes the test's browser requests use the fake
too.

```alder fragment
Test.fake Login.Password
```

### `Test.login`

```alder fragment
login : Session -> Task Unit
```

Signs a session into the test's browser and makes it the test's own session.

The argument is `App`'s session record, checked when the test compiles; it is the cookie's
value, so a field with a default may be left out. The browser then sends it with every
request, and the scoped `Db` operations the test itself runs act for that user. A `via` key
in it (`org: Just acme`) needs its membership row first (`Test.member`), as `Session.put`
would. Unlike the other browser steps, it works in an app without an `App` module.

```alder fragment
Test.login { user: Just (User.UserId "u1"), role: Member }
Test.login { user: Nothing }
```

### `Test.member`

```alder fragment
member : u -> k -> Task Unit
```

Inserts the membership row that makes a user a member of a session key's `via` table.

`Test.member me org` writes a row of the table the session field's `via` names, with its
user column `me` and its key column `org`; every other column is its default, or `Nothing`
for a `Maybe` column. After it, a `Test.login` or a `Session.put` with that key passes the
membership check. An app whose session has no `via` field is E0003, and so is a membership
table with another column that has no default: insert that row with `Db.insert`.

### `Test.setClock`

```alder fragment
setClock : Time -> Task Unit
```

Sets the test's clock to a time, and stops it there.

From here on `Time.now`, `Date.today` and `@created` read that time, in the test and in the
routes its browser requests, and the clock stands still: the one-second tick of a clock that
was never set stops for the rest of the test. `Test.advance`, `Query.delay` and `Task.sleep`
still move it. A second `Test.setClock` moves it again, backwards too. A failed `Test.run`
does not undo it, and the trace shows it as `Test.setClock` and the time in UTC. A test that
never sets its clock starts at 2026-01-01T00:00:00Z.

```alder fragment
Test.setClock (Time.fromMillis 1772582400000)
```

### `Test.advance`

```alder fragment
advance : Int -> Task Unit
```

Moves the test's clock a number of milliseconds forward.

A negative number counts as 0: to go back, set the clock again. An advance beyond about 292
years stops there instead of wrapping into the past. A clock that ticks keeps ticking after
it, and a set one stays still. A failed `Test.run` does not undo it, and the trace shows it
as `Test.advance` and the milliseconds (`Test.advance 86400000ms`).

```alder fragment
Test.advance (24 * 60 * 60 * 1000)
```

### `Test.jobs`

```alder fragment
jobs : Task (List { id: Job.Id, job: Job, status: Job.Status })
```

Every job in the test's database, oldest first, with its status.

A test runs no worker: `Job.enqueue` stores the job in the test's transaction, so it is
listed here and gone with the test, and a `Test.run` that fails drops the jobs it queued.
Jobs that have run stay, as `Done` or `Dead`, and a failed one is `Queued` again with its
`error` and the `runAt` of its retry. Only `Test.work` runs them.

```alder fragment
Test.jobs |> Task.map (\rows -> List.length rows)
```

### `Test.work`

```alder fragment
work : Task (List { id: Job.Id, job: Job, result: Result Text Unit })
```

Runs every job that is due at the test's clock, oldest first, and gives each run's result.

Each job runs as its own `Task` in the test's transaction. A job that fails has its writes
undone, and it is retried at the clock's time plus the same wait a worker uses (10 seconds
after the first attempt) or, past its `retries`, is `Dead`; `result` is `Err` with its
error. The jobs that the jobs queue run in the same call when they are due. A clock the test
has not set moves a second at every reading, and running a job reads it, so a job due a few
seconds ahead, or a failed job's retry, can come due within the same call: set the clock with
`Test.setClock` and a job not yet due waits for `Test.advance`. After 1000 runs in one call
the test fails with "Test.work: jobs keep enqueueing jobs".

A job with `every` or `cron` runs on its schedule, as under a worker: the first `Test.work`
of a test queues its first run at the schedule's next time after the clock, and each run
queues the next, so moving the clock past several of its times runs it once.

```alder fragment
Test.work |> Task.map (\runs -> List.all (\r -> r.result == Ok ()) runs)
```

### `Test.get`

```alder fragment
get : Route -> Task { model: model, session: Session, doc: Doc, url: Text, status: Int }
```

Requests a GET route's page and fails the test unless the page is shown.

It is a special form: the argument is a GET route written in place with its parameters
(`Todos.Index.Index {}`, `Todos.Show.Show { id: id }`). A redirect, a missing page or an
error fails the test, and a redirect to the login page names the route's `requires`. `model`
is what the route's `load` gave, typed, which must not be a union (use `Test.open` for that
route). A write route is E0045, and a file route is E0003: use `Test.fetch`. `url` is the URL
of the page the browser holds after the step and `status` the code of its last response.
The result also has `notices` and `toasts`, the toasts the answer showed.

```alder fragment
Test.get (Todos.Index.Index {})
Test.get (Todos.Index.Index {}) |> Task.map (\page -> List.length page.model.todos)
```

### `Test.open`

```alder fragment
open : Route -> Task { answer: Answer, session: Session, doc: Doc, url: Text, status: Int }
```

Requests a GET route's page and gives what the request came to, whatever it was.

It is a special form: the argument is a GET route written in place, as for `Test.get`. It
follows redirects, up to 10 in a row, and `answer` is `Shown`, `Redirected` with the page it
reached, `PageMissing` or `RequestFailed`; the browser holds the page it ends on. A write
route is E0045, and a file route is E0003. `url` is the URL of the page held after the step
and `status` the code of its last response. The result also has `notices` and `toasts`.

```alder fragment
Test.open (Admin.Index.Index {})
Test.open (Admin.Index.Index {}) |> Task.map (\r -> r.answer == Redirected (Auth.Login.Login {}))
```

### `Test.fetch`

```alder fragment
fetch : Route -> Task { status: Int, contentType: Text, body: Bytes, headers: List { name: Text, value: Text } }
```

Requests a GET route once and gives the raw answer.

It is a special form: the argument is a GET route written in place, and it is the step for
a file page. It does not follow a redirect. The result is the response's `status`, its
`contentType`, its `body` as `Bytes`, and every header, sorted by name. The page the browser
holds stays as it was, so the result has no `url` and no `doc`. A write route is E0045.

```alder fragment
Test.fetch (Todos.Index.Index {}) |> Task.map (\r -> r.status)
```

### `Test.submit`

```alder fragment
submit : target -> f -> Task { answer: Answer, session: Session, doc: Doc, url: Text, status: Int }
```

Posts a record through the held page's form for a write route.

It is a special form: the first argument is a write route with `form F`, written in place
with its parameters (`Create`, or `Edit { id: id }`), and the second is `F`'s record, whose
defaults apply. The browser posts it through the page's form for that route, as a user
would. The test fails, with nothing sent, when the page holds no such form, or when a field
the form has no input for has a value that sending nothing would not give. `answer` is what
the request came to (`FormRejected` with `status` 422 after `Response.invalid`), and `url`
the page held after it. The result also has `notices` and `toasts`.

```alder fragment
Test.submit (Todos.Index.Submit {}) { title: Text.NonEmpty "Milk", due: Nothing }
```

### `Test.submitRaw`

```alder fragment
submitRaw : target -> List { name: Text, value: Text } -> Task { answer: Answer, session: Session, doc: Doc, url: Text, status: Int }
```

Posts text fields through the held page's form for a write route, for input the types forbid.

It is a special form: the first argument is as `Test.submit`'s, and the second the fields to
post, each a `name` and a text `value`. A name that is not a field of the route's form, or
that the held form has no input for, fails the test. Otherwise it posts as `Test.submit`
does, with the same result.

```alder fragment
Test.submitRaw (Todos.Index.Submit {}) [{ name: "title", value: "" }]
```

### `Test.click`

```alder fragment
click : target -> Task { answer: Answer, session: Session, doc: Doc, url: Text, status: Int }
```

Presses the held page's button for a route without a form.

It is a special form: the argument is a route written in place with its parameters. A write
route (`Toggle { id: id }`, `App.Logout`) is sent the way the page's control sends it; a GET
route (`Row { id: id }`) is fetched and patched into the held page. The test fails, with
nothing sent, when the held page has no control for that route. A route with a form is
E0045: use `Test.submit`. `answer` is `Rendered`, `Refreshed`, `FormRejected` (with `status`
422), `Redirected`, `PageMissing` or `RequestFailed`. The result also has `notices` and
`toasts`.

```alder fragment
Test.click (Todos.Index.Toggle { id: Todo.TodoId "a" })
Test.click App.Logout
```

### `Test.load`

```alder fragment
load : target -> Task { answer: Answer, session: Session, doc: Doc, url: Text, status: Int }
```

Fetches a GET route into the held page, as its element with `load` or `every` does.

It is a special form: the argument is a GET route written in place (`Stats {}`). The held
page must have exactly one element that requests the route's path with `load` or `every`;
none, or more than one, fails the test with nothing sent. The browser fires nothing on its
own, so this is how a test says the element appeared, or that its timer ticked. The answer
is patched into the page as the element's fetch is. A write route is E0045.

```alder fragment
Test.load (Todos.Show.Show { id: Todo.TodoId "a" })
```

### `Test.intersect`

```alder fragment
intersect : target -> Task { answer: Answer, session: Session, doc: Doc, url: Text, status: Int }
```

Fetches a GET route into the held page, as its element with `intersect` does.

It works as `Test.load` does, for the one element that requests the route with `intersect`:
it is how a test says the element scrolled into view.

```alder fragment
Test.intersect (Todos.Show.Show { id: Todo.TodoId "a" })
```

### `Test.input`

```alder fragment
input : target -> Task { answer: Answer, session: Session, doc: Doc, url: Text, status: Int }
```

Fetches a GET route into the held page, as its field with `input` does after typing.

It works as `Test.load` does, for the one field that requests the route with `input`. The
route's parameters are what the test wrote (`Results { q: "mil" }`), as if the user had
typed them.

```alder fragment
Test.input (Admin.Todos.Index.Index { q: "mil" })
```

### `Test.refresh`

```alder fragment
refresh : Task { answer: Answer, session: Session, doc: Doc, url: Text, status: Int }
```

Loads the page the browser holds again, and fails the test unless it is the page held.

It sends a plain `GET` of the held URL and compares the body it gets with the body the
browser held, leaving out the toasts (`<al-toasts>`). It is the check that what the steps
patched in is what the page renders. The test fails when the bodies differ, when the page is
not shown again (a redirect, a missing page or an error), or when the browser holds no page
yet. The `answer` is `Refreshed`.

```alder fragment
Test.refresh
Test.refresh |> Task.map (\r -> r.answer == Refreshed)
```

### `Test.text`

```alder fragment
text : Text -> Doc -> Maybe Text
```

The text of the first element a selector matches, or `Nothing` when none does.

The text is every piece of text inside the element, with runs of whitespace collapsed to one
space and the ends trimmed. A selector written as a literal is checked when the test
compiles (E0023); any other selector that does not parse matches nothing.

```alder fragment
Test.text "h1" (Test.doc (<h1>  Hello,   world </h1>)) == Just "Hello, world"
Test.text "p" (Test.doc (<p>a <b>bold</b> word</p>)) == Just "a bold word"
Test.text "#total" (Test.doc (<p id="total">3 left</p>)) == Just "3 left"
Test.text "h2" (Test.doc (<h1>Hello</h1>)) == Nothing
```

### `Test.texts`

```alder fragment
texts : Text -> Doc -> List Text
```

The text of every element a selector matches, in page order.

Each text is as `Test.text` gives it. No match gives the empty list. A selector written as a
literal is checked when the test compiles (E0023); any other selector that does not parse
matches nothing.

```alder fragment
Test.texts "li" (Test.doc (<ul><li>Milk</li><li>Eggs</li></ul>)) == ["Milk", "Eggs"]
Test.texts "li" (Test.doc (<p>none</p>)) == []
```

### `Test.has`

```alder fragment
has : Text -> Doc -> Bool
```

Whether a selector matches any element of a document.

A selector written as a literal is checked when the test compiles (E0023); any other
selector that does not parse matches nothing, so it gives `False`.

```alder fragment
Test.has "li.done" (Test.doc (<ul><li class="done">Milk</li></ul>)) == True
Test.has "ul > li" (Test.doc (<div><ul><li>Milk</li></ul></div>)) == True
Test.has "[data-id=7]" (Test.doc (<li data-id="7">x</li>)) == True
Test.has (Text.trim "[") (Test.doc (<p>x</p>)) == False
```

### `Test.count`

```alder fragment
count : Text -> Doc -> Int
```

The number of elements a selector matches.

The element at the top of the document counts too. A selector written as a literal is
checked when the test compiles (E0023); any other selector that does not parse matches
nothing, so it gives 0.

```alder fragment
Test.count "li" (Test.doc (<ul><li>Milk</li><li>Eggs</li></ul>)) == 2
Test.count "div" (Test.doc (<div><div>x</div></div>)) == 2
```

### `Test.attr`

```alder fragment
attr : Text -> Text -> Doc -> Maybe Text
```

The value of an attribute on the first element a selector matches.

The attribute's name comes first, then the selector. It gives `Nothing` when nothing
matches or the first match has no such attribute, and `Just ""` for an attribute written
without a value, such as `disabled`. A selector written as a literal is checked when the
test compiles (E0023); any other selector that does not parse matches nothing.

```alder fragment
Test.attr "title" "p" (Test.doc (<p title="Note">x</p>)) == Just "Note"
Test.attr "lang" "p" (Test.doc (<p title="Note">x</p>)) == Nothing
Test.attr "disabled" "button" (Test.doc (<button disabled>Go</button>)) == Just ""
```

### `Test.doc`

```alder fragment
doc : Html -> Doc
```

A view rendered to HTML and parsed back, as a document the queries can read.

It is how a test reads a view without a request: the result is compared with `==` and read
with the queries, as a step's `doc` is.

```alder fragment
Test.doc (<p>Hi</p>) == Test.doc (<p>Hi</p>)
Test.has "p" (Test.doc (<p>Hi</p>)) == True
```

## Text

Functions for `Text`, a piece of text, and the refinement types and builder that go with it.

A `Text` is a sequence of Unicode code points. Where a function counts or cuts text, it works
in code points, so `"é"` has length 1 whatever the bytes are. A function never changes its
input: it gives back a new `Text`.

### `Text.NonEmpty`

```alder fragment
type NonEmpty = brand Text where True
```

A `Text` that is not empty once its whitespace is trimmed.

A value is checked when it is decoded from a form or a route, and is trusted after that.
`Text.NonEmpty "Ada"` makes one from a literal, but applying the brand to a `Text` is not
checked: `Text.NonEmpty ""` is accepted, so the caller must make sure the text is not empty.

### `Text.Email`

```alder fragment
type Email = brand Text where True
```

A `Text` that looks like an email address.

It has exactly one `@`, some text on each side of it and no whitespace. It is a check on the
shape only: it does not say that the address exists.

The check runs when a value is decoded from a form or a route, not when the brand is applied
to a `Text`, so the caller must make sure of a `Text.Email` made by hand.

### `Text.Url`

```alder fragment
type Url = brand Text where True
```

A `Text` that is an absolute URL with a scheme and a host.

`https://example.com/a` passes. `mailto:a@b.com` and `//example.com` do not.

The check runs when a value is decoded from a form or a route, not when the brand is applied
to a `Text`, so the caller must make sure of a `Text.Url` made by hand.

### `Text.Secret`

```alder fragment
type Secret = brand Text where True
```

A `Text` that holds a secret, such as a password.

It may not be empty. A form shows a field of this type as a password input and never fills
it back in, not even when the form comes back with errors. The dev panel and its "Copy as
curl" line show `••••` for a route's form fields and path parameters of this type, and a log
line masks such a path parameter. A session field may not be a `Text.Secret` (E0048): the
session cookie is signed, not encrypted, so the user can read it.

That is all the masking there is. A `Text.Secret` widens to `Text`, so anything that takes a
`Text` shows it in clear: `Log.info` and the other `Log` functions, `Json.string`, `++`, and
text in a view. Do not pass one to them. A query parameter of this type is masked only when
its name says it holds a secret, and the dev panel's SQL tab shows a statement's arguments as
they are.

The check runs when a value is decoded from a form or a route, not when the brand is applied
to a `Text`, so the caller must make sure of a `Text.Secret` made by hand.

### `Text.Builder`

```alder fragment
type Builder
```

An accumulator for building a large `Text` without copying it at every step.

Adding a piece to a builder costs the same however much has been added, and `Text.build`
copies each byte once. Use it instead of nested `++` or `Text.join` when a page or a file is
large. A builder is a value: adding to it gives a new builder and leaves the old one alone.

### `Text.length`

```alder fragment
length : Text -> Int
```

The number of code points in a `Text`.

This counts characters, not bytes, so a letter like `é` counts once.

```alder fragment
Text.length "hello" == 5
Text.length "é" == 1
Text.length "" == 0
```

### `Text.isEmpty`

```alder fragment
isEmpty : Text -> Bool
```

Tells whether a `Text` has no characters at all.

A `Text` of only spaces is not empty. Use `Text.trim` first to treat it as empty.

```alder fragment
Text.isEmpty "" == True
Text.isEmpty " " == False
Text.isEmpty (Text.trim " ") == True
```

### `Text.trim`

```alder fragment
trim : Text -> Text
```

Removes the whitespace at both ends of a `Text`.

Whitespace is spaces, tabs, newlines and the other Unicode space characters. Whitespace in the
middle stays.

```alder fragment
Text.trim "  hi  " == "hi"
Text.trim "a b" == "a b"
Text.trim "\n\thi\n" == "hi"
```

### `Text.toUpper`

```alder fragment
toUpper : Text -> Text
```

Turns every letter of a `Text` into its upper case form.

Characters that have no upper case stay as they are.

```alder fragment
Text.toUpper "hello, 1" == "HELLO, 1"
Text.toUpper "é" == "É"
```

### `Text.toLower`

```alder fragment
toLower : Text -> Text
```

Turns every letter of a `Text` into its lower case form.

Characters that have no lower case stay as they are.

```alder fragment
Text.toLower "HELLO, 1" == "hello, 1"
Text.toLower "É" == "é"
```

### `Text.contains`

```alder fragment
contains : Text -> Text -> Bool
```

Tells whether a `Text` has another `Text` somewhere inside it.

The needle comes first and the text to search second. The match is exact, so case matters.
The empty needle is in every `Text`.

```alder fragment
Text.contains "ell" "hello" == True
Text.contains "ELL" "hello" == False
Text.contains "" "hello" == True
```

### `Text.startsWith`

```alder fragment
startsWith : Text -> Text -> Bool
```

Tells whether a `Text` begins with another `Text`.

The prefix comes first and the text to check second. Every `Text` starts with the empty one.

```alder fragment
Text.startsWith "he" "hello" == True
Text.startsWith "lo" "hello" == False
Text.startsWith "" "hello" == True
```

### `Text.endsWith`

```alder fragment
endsWith : Text -> Text -> Bool
```

Tells whether a `Text` ends with another `Text`.

The suffix comes first and the text to check second. Every `Text` ends with the empty one.

```alder fragment
Text.endsWith "lo" "hello" == True
Text.endsWith "he" "hello" == False
Text.endsWith "" "hello" == True
```

### `Text.replace`

```alder fragment
replace : Text -> Text -> Text -> Text
```

Replaces every occurrence of one `Text` with another.

The order is the text to find, the text to put in its place, then the text to change. A text
with no match comes back unchanged. Replacing the empty `Text` puts the new text between
every character and at both ends.

```alder fragment
Text.replace "l" "L" "hello" == "heLLo"
Text.replace "x" "y" "hello" == "hello"
Text.replace "l" "" "hello" == "heo"
Text.replace "" "-" "ab" == "-a-b-"
```

### `Text.split`

```alder fragment
split : Text -> Text -> List Text
```

Cuts a `Text` into a list of pieces at every occurrence of a separator.

The separator comes first. The separator itself is not in any piece, and two separators in a
row give an empty piece between them. A text with no separator comes back as a list of one.
An empty separator cuts the text into single characters.

```alder fragment
Text.split ", " "a, b, c" == ["a", "b", "c"]
Text.split "," "a,,b" == ["a", "", "b"]
Text.split "," "abc" == ["abc"]
Text.split "," "" == [""]
Text.split "" "abc" == ["a", "b", "c"]
```

### `Text.join`

```alder fragment
join : Text -> List Text -> Text
```

Joins a list of `Text` into one, with a separator between each pair.

The separator comes first. An empty list gives the empty `Text`, and a list of one gives that
one piece with no separator. It is the opposite of `Text.split`.

```alder fragment
Text.join ", " ["a", "b", "c"] == "a, b, c"
Text.join ", " ["a"] == "a"
Text.join ", " [] == ""
```

### `Text.slice`

```alder fragment
slice : Int -> Int -> Text -> Text
```

Takes the characters from one position up to, but not including, another.

Positions count code points from 0. A negative position counts from the end, so `-1` is the
last character; wrap a negative number in parentheses. A position past either end is moved
to that end, and a start at or after the end gives the empty `Text`.

```alder fragment
Text.slice 1 3 "abcdef" == "bc"
Text.slice (-3) (-1) "abcdef" == "de"
Text.slice 2 100 "abcdef" == "cdef"
Text.slice 4 2 "abcdef" == ""
```

### `Text.matches`

```alder fragment
matches : Text -> Text -> Bool
```

Tells whether a regular expression matches anywhere in a `Text`.

The regular expression comes first and is written in RE2 syntax. It matches a part of the
text unless you anchor it: write `^` and `$` to match the whole text. A regular expression
that is not valid matches nothing, and is not an error.

```alder fragment
Text.matches "[0-9]+" "room 42" == True
Text.matches "^[0-9]+$" "room 42" == False
Text.matches "^[0-9]+$" "42" == True
Text.matches "(" "anything" == False
```

### `Text.fromInt`

```alder fragment
fromInt : Int -> Text
```

Writes an `Int` as a `Text`, in decimal.

A negative number starts with `-`. There are no separators and no padding.

```alder fragment
Text.fromInt 42 == "42"
Text.fromInt (-7) == "-7"
Text.fromInt 0 == "0"
```

### `Text.fromFloat`

```alder fragment
fromFloat : Float -> Text
```

Writes a `Float` as a `Text`, in decimal.

It gives the shortest decimal that reads back as the same number, and never uses an exponent,
so a whole number has no `.0`. Infinity is `+Inf` or `-Inf` and not a number is `NaN`.

```alder fragment
Text.fromFloat 3.5 == "3.5"
Text.fromFloat 2.0 == "2"
Text.fromFloat (-0.25) == "-0.25"
Text.fromFloat (1.0 / 0.0) == "+Inf"
```

### `Text.toInt`

```alder fragment
toInt : Text -> Maybe Int
```

Reads an `Int` from a `Text`, or `Nothing` when it is not one.

The text must be a whole decimal number with an optional `+` or `-` at the start. Spaces, a
decimal point and a number too big for an `Int` all give `Nothing`.

```alder fragment
Text.toInt "42" == Just 42
Text.toInt "-7" == Just (-7)
Text.toInt " 42" == Nothing
Text.toInt "4.5" == Nothing
Text.toInt "" == Nothing
Text.toInt "99999999999999999999" == Nothing
```

### `Text.toFloat`

```alder fragment
toFloat : Text -> Maybe Float
```

Reads a `Float` from a `Text`, or `Nothing` when it is not one.

A whole number is fine, and so are a decimal point (`"2.5"`, `".5"`, `"5."`), a leading `+` or
`-`, and an exponent (`"1e3"` is 1000.0). It accepts whatever Go's `strconv.ParseFloat`
does, which includes hexadecimal forms such as `"0x1p-2"` and underscores between digits.
Spaces, text that is not a number, `NaN`, infinity and a number too big for a `Float` all
give `Nothing`.

```alder fragment
Text.toFloat "2.5" == Just 2.5
Text.toFloat "1e3" == Just 1000.0
Text.toFloat "-2.5" == Just (-2.5)
Text.toFloat ".5" == Just 0.5
Text.toFloat "1e400" == Nothing
Text.toFloat "3" == Just 3.0
Text.toFloat "abc" == Nothing
Text.toFloat " 2.5" == Nothing
Text.toFloat "NaN" == Nothing
```

### `Text.toBytes`

```alder fragment
toBytes : Text -> Bytes
```

The UTF-8 bytes of a `Text`.

A character outside plain ASCII takes more than one byte, so the `Bytes` may be longer than
the `Text` is. Use it to send text as a file answer with `File.bytes`, or to pass text to
something that takes `Bytes`.

```alder fragment
Bytes.length (Text.toBytes "abc") == 3
Bytes.length (Text.toBytes "é") == 2
```

### `Text.padLeft`

```alder fragment
padLeft : Int -> Text -> Text -> Text
```

Adds a pad to the left of a `Text` until it is a number of characters long.

The arguments are the length, the pad and the text. A pad of several characters is repeated
and cut at the outer edge. A text that is already that long, or longer, comes back whole, and
so does any text when the pad is empty.

```alder fragment
Text.padLeft 2 "0" "7" == "07"
Text.padLeft 5 "ab" "x" == "ababx"
Text.padLeft 2 "0" "123" == "123"
Text.padLeft 4 "" "x" == "x"
```

### `Text.padRight`

```alder fragment
padRight : Int -> Text -> Text -> Text
```

Adds a pad to the right of a `Text` until it is a number of characters long.

The arguments are the length, the pad and the text. A pad of several characters is repeated
and cut at the outer edge. A text that is already that long, or longer, comes back whole, and
so does any text when the pad is empty.

```alder fragment
Text.padRight 5 "." "ab" == "ab..."
Text.padRight 5 "ab" "x" == "xabab"
Text.padRight 2 "." "abc" == "abc"
Text.padRight 4 "" "x" == "x"
```

### `Text.left`

```alder fragment
left : Int -> Text -> Text
```

The first characters of a `Text`.

It takes that many code points. A count above the length gives the whole text, and a count of
zero or less gives the empty `Text`.

```alder fragment
Text.left 2 "hello" == "he"
Text.left 10 "hello" == "hello"
Text.left 0 "hello" == ""
Text.left (-1) "hello" == ""
```

### `Text.right`

```alder fragment
right : Int -> Text -> Text
```

The last characters of a `Text`.

It takes that many code points. A count above the length gives the whole text, and a count of
zero or less gives the empty `Text`.

```alder fragment
Text.right 2 "hello" == "lo"
Text.right 10 "hello" == "hello"
Text.right 0 "hello" == ""
Text.right (-1) "hello" == ""
```

### `Text.dropLeft`

```alder fragment
dropLeft : Int -> Text -> Text
```

Removes the first characters of a `Text` and keeps the rest.

A count above the length gives the empty `Text`, and a count of zero or less changes nothing.

```alder fragment
Text.dropLeft 2 "hello" == "llo"
Text.dropLeft 10 "hello" == ""
Text.dropLeft 0 "hello" == "hello"
Text.dropLeft (-1) "hello" == "hello"
```

### `Text.dropRight`

```alder fragment
dropRight : Int -> Text -> Text
```

Removes the last characters of a `Text` and keeps the rest.

A count above the length gives the empty `Text`, and a count of zero or less changes nothing.

```alder fragment
Text.dropRight 2 "hello" == "hel"
Text.dropRight 10 "hello" == ""
Text.dropRight 0 "hello" == "hello"
Text.dropRight (-1) "hello" == "hello"
```

### `Text.repeat`

```alder fragment
repeat : Int -> Text -> Text
```

Writes a `Text` over and over, a number of times.

A count of zero or less gives the empty `Text`. A very large count uses a very large amount
of memory, so do not pass a number you did not choose.

```alder fragment
Text.repeat 3 "ab" == "ababab"
Text.repeat 1 "ab" == "ab"
Text.repeat 0 "ab" == ""
Text.repeat (-2) "ab" == ""
```

### `Text.reverse`

```alder fragment
reverse : Text -> Text
```

Puts the characters of a `Text` in the opposite order.

It reverses code points, not bytes, so a letter like `é` stays whole.

```alder fragment
Text.reverse "abc" == "cba"
Text.reverse "é1" == "1é"
Text.reverse "" == ""
```

### `Text.trimLeft`

```alder fragment
trimLeft : Text -> Text
```

Removes the whitespace at the start of a `Text`.

Whitespace is spaces, tabs, newlines and the other Unicode space characters. The end is not
touched. `Text.trim` does both ends.

```alder fragment
Text.trimLeft "  hi  " == "hi  "
Text.trimLeft "hi" == "hi"
```

### `Text.trimRight`

```alder fragment
trimRight : Text -> Text
```

Removes the whitespace at the end of a `Text`.

Whitespace is spaces, tabs, newlines and the other Unicode space characters. The start is not
touched. `Text.trim` does both ends.

```alder fragment
Text.trimRight "  hi  " == "  hi"
Text.trimRight "hi" == "hi"
```

### `Text.concat`

```alder fragment
concat : List Text -> Text
```

Joins a list of `Text` into one with nothing between the pieces.

This is `Text.join ""`. An empty list gives the empty `Text`.

```alder fragment
Text.concat ["a", "b", "c"] == "abc"
Text.concat [] == ""
```

### `Text.lines`

```alder fragment
lines : Text -> List Text
```

Cuts a `Text` into its lines.

A line ends at `\n`, and a `\r` just before it is removed, so text from Windows works too. The
empty line after a final newline is dropped, and the empty `Text` has no lines. An empty line
in the middle is kept.

```alder fragment
Text.lines "a\nb" == ["a", "b"]
Text.lines "a\nb\n" == ["a", "b"]
Text.lines "a\r\nb" == ["a", "b"]
Text.lines "a\n\nb" == ["a", "", "b"]
Text.lines "" == []
```

### `Text.words`

```alder fragment
words : Text -> List Text
```

Cuts a `Text` into its words.

Words are separated by any run of whitespace, so the list never holds an empty word, and
spaces at the ends are ignored. A text with no words gives the empty list.

```alder fragment
Text.words "hello  big world" == ["hello", "big", "world"]
Text.words "  a\tb\n" == ["a", "b"]
Text.words "   " == []
```

### `Text.indexes`

```alder fragment
indexes : Text -> Text -> List Int
```

Every position in a `Text` where another `Text` starts.

The needle comes first and the text to search second. Positions count code points from 0, in
order. Matches may overlap. An empty needle, or one that is not found, gives the empty list.

```alder fragment
Text.indexes "aa" "aaa" == [0, 1]
Text.indexes "b" "abcb" == [1, 3]
Text.indexes "x" "abc" == []
Text.indexes "" "abc" == []
```

### `Text.toCodePoints`

```alder fragment
toCodePoints : Text -> List Int
```

The Unicode code point of each character in a `Text`.

Each is a number, such as 97 for `a`. `Text.fromCodePoints` turns the list back into a `Text`.

```alder fragment
Text.toCodePoints "aé" == [97, 233]
Text.toCodePoints "" == []
```

### `Text.fromCodePoints`

```alder fragment
fromCodePoints : List Int -> Text
```

Builds a `Text` from a list of Unicode code points.

A number that is not a character, one that is negative, above 1114111 or in the surrogate
range 55296 to 57343, becomes the replacement character U+FFFD.

```alder fragment
Text.fromCodePoints [104, 105] == "hi"
Text.fromCodePoints [] == ""
Text.toCodePoints (Text.fromCodePoints [-1]) == [65533]
```

### `Text.hash`

```alder fragment
hash : Text -> Int
```

A number made from a `Text`, the same on every run, version and platform.

It is FNV-1a (64 bit) of the UTF-8 bytes with the top bit cleared, so it is never negative
and `Text.hash s % n` is a bucket from 0 to n-1. The values are fixed for good, so it is safe
for a seed (see `Random.seed`), a bucket or a cache key. It is not cryptographic: do not use
it for passwords or for anything an attacker may choose.

```alder fragment
Text.hash "" == 5472609002491880229
Text.hash "hello" == 2607821981565500683
Text.hash "hello" % 10 == 3
```

### `Text.builder`

```alder fragment
builder : Builder
```

A builder with nothing added yet.

Start every `Text.Builder` from this, add pieces with `Text.add` and finish with `Text.build`.

```alder fragment
Text.build Text.builder == ""
```

### `Text.add`

```alder fragment
add : Text -> Builder -> Builder
```

Puts a piece of `Text` at the end of a builder.

The piece goes after everything added so far, and nothing is copied. The piece comes first and
the builder last, so it works well with `|>`.

```alder fragment
Text.build (Text.builder |> Text.add "a" |> Text.add "b") == "ab"
Text.build (Text.add "" Text.builder) == ""
```

### `Text.build`

```alder fragment
build : Builder -> Text
```

The `Text` of everything added to a builder, in the order it was added.

It copies each byte once. A builder can be built more than once, and building does not change
it.

```alder fragment
Text.build (Text.add "b" (Text.add "a" Text.builder)) == "ab"
```

## Time

Functions for `Time`, an instant on the clock.

A `Time` is one moment, counted in milliseconds from `Time.epoch`, and it has no time zone:
every function that writes it as text or takes a date out of it works in UTC. Two `Time`s
compare with `==` (they are equal when they are the same instant) and with `<` and `>`. Use
`Date` for a calendar day with no time of day.

### `Time.now`

```alder fragment
now : Query Time
```

The current time.

It is a `Query`: bind it with `let now <-`, or map it. In a test it is the test's clock.
Until the test sets it, the clock starts at 2026-01-01T00:00:00Z and moves one second on
each reading. `Test.setClock` sets it and stops that tick, and `Test.advance`, `Task.sleep`
and `Query.delay` move it forward.

```alder fragment
Time.now |> Query.map Time.toMillis
```

### `Time.epoch`

```alder fragment
epoch : Time
```

The instant 1970-01-01 at 00:00 UTC.

```alder fragment
Time.toMillis Time.epoch == 0
```

### `Time.toDate`

```alder fragment
toDate : Time -> Date
```

The calendar date of a `Time`, in UTC.

The time of day is dropped. A time just before midnight UTC is still the day before, whatever
the local time of the reader is.

```alder fragment
Date.toText (Time.toDate Time.epoch) == "1970-01-01"
Date.toText (Time.toDate (Time.fromMillis 86399999)) == "1970-01-01"
Date.toText (Time.toDate (Time.fromMillis 86400000)) == "1970-01-02"
```

### `Time.format`

```alder fragment
format : Text -> Time -> Text
```

Writes a `Time` in a layout of your choosing, in UTC.

The layout is written the way Go does it: it is the moment `Mon Jan 2 15:04:05 2006` itself,
cut and changed to show the shape you want. `"2006-01-02 15:04:05"` gives
`1970-01-01 00:00:00` for `Time.epoch`, and `"3:04 PM"` gives a 12-hour clock. The parts of
the layout that are not a part of that moment are copied as they are.

```alder fragment
Time.format "2006-01-02 15:04:05" Time.epoch == "1970-01-01 00:00:00"
Time.format "2006-01-02 15:04:05" (Time.fromMillis 90061000) == "1970-01-02 01:01:01"
Time.format "3:04 PM" (Time.fromMillis 46800000) == "1:00 PM"
```

### `Time.ago`

```alder fragment
ago : Time -> Time -> Text
```

Says how long ago a `Time` was, in words such as "3 hours ago".

`Time.ago now then` takes the current time first, which you get with `Time.now`, so the
function stays pure. It gives "just now" for less than a minute, and otherwise the largest
whole unit that fits: minutes, hours, days, months (counted as 30 days) or years (counted as
365 days), with an "s" when the number is more than 1. The fractions are dropped, so 90
minutes is "1 hour ago". When `then` is after `now` it says "in 3 hours" instead.

```alder fragment
Time.ago (Time.fromMillis 30000) Time.epoch == "just now"
Time.ago (Time.fromMillis 60000) Time.epoch == "1 minute ago"
Time.ago (Time.fromMillis 5400000) Time.epoch == "1 hour ago"
Time.ago (Time.fromMillis 10800000) Time.epoch == "3 hours ago"
Time.ago (Time.fromMillis 172800000) Time.epoch == "2 days ago"
Time.ago (Time.fromMillis 2592000000) Time.epoch == "1 month ago"
Time.ago (Time.fromMillis 31536000000) Time.epoch == "1 year ago"
Time.ago Time.epoch (Time.fromMillis 10800000) == "in 3 hours"
Time.ago Time.epoch (Time.fromMillis 30000) == "just now"
```

### `Time.toMillis`

```alder fragment
toMillis : Time -> Int
```

The number of milliseconds from `Time.epoch` to a `Time`.

A time before the epoch gives a negative number.

```alder fragment
Time.toMillis Time.epoch == 0
Time.toMillis (Time.fromMillis 1500) == 1500
Time.toMillis (Time.fromMillis (-1500)) == -1500
```

### `Time.fromMillis`

```alder fragment
fromMillis : Int -> Time
```

The `Time` a number of milliseconds after `Time.epoch`.

A negative number gives a time before the epoch.

```alder fragment
Time.toMillis (Time.fromMillis 1500) == 1500
Time.fromMillis 0 == Time.epoch
(Time.epoch < Time.fromMillis 1) == True
```

### `Time.addMillis`

```alder fragment
addMillis : Int -> Time -> Time
```

Moves a `Time` by a number of milliseconds.

A negative number moves back.

```alder fragment
Time.toMillis (Time.addMillis 250 (Time.fromMillis 1000)) == 1250
Time.toMillis (Time.addMillis (-250) (Time.fromMillis 1000)) == 750
Time.addMillis 0 Time.epoch == Time.epoch
```

### `Time.diffMillis`

```alder fragment
diffMillis : Time -> Time -> Int -- Time.diffMillis a b returns b − a
```

The number of milliseconds from the first `Time` to the second.

`Time.diffMillis a b` is `b` minus `a`, so it is negative when `b` comes first. It counts
whole milliseconds. To time a step, read `Time.now` before and after it and give the two
times to `Time.diffMillis`.

```alder fragment
Time.diffMillis Time.epoch (Time.fromMillis 1500) == 1500
Time.diffMillis (Time.fromMillis 1500) Time.epoch == -1500
Time.diffMillis Time.epoch Time.epoch == 0
```

### `Time.fromDate`

```alder fragment
fromDate : Date -> Time -- midnight UTC
```

The `Time` at the start of a `Date`: midnight UTC.

```alder fragment
Time.toMillis (Time.fromDate Date.epoch) == 0
Time.toMillis (Time.fromDate (Date.addDays 1 Date.epoch)) == 86400000
```

## Url

Links to places outside the app, and a route's full address.

A `Route` is a route name applied to its parameters; these two make one from a `Text` and
turn one into a `Text`.

### `Url.external`

```alder fragment
external : Text -> Route
```

A link to a URL outside the app, for `href`, `src` or `Response.redirect`.

The URL is kept only when it is an absolute `http:` or `https:` URL, a path of this site
(`/static/app.css`, but not `//host` or `/\host`) or a fragment (`#top`). Anything else
becomes `#`, a link that goes nowhere (a redirect to it is refused with a 500):
`javascript:` and `data:` URLs, other schemes such as `mailto:`, a URL with a leading space,
a relative path, and a URL holding a control character. So a link built from user input can never run script.

```alder fragment
Url.external "https://example.com/a" == Url.external "https://example.com/a"
Url.external "javascript:alert(1)" == Url.external "#"
Url.external "//evil.example" == Url.external "#"
Url.external "mailto:a@example.com" == Url.external "#"
```

### `Url.absolute`

```alder fragment
absolute : Route -> Text
```

The route's full URL, with the site's address in front, for a program that is not a browser.

It is the text `href={r}` renders (a defaulted parameter left out, the query encoded) with
the site's public address in front, taken from `ALDER_PUBLIC_URL`. Without it the running
server uses its own listening address, and a test uses `http://localhost:8080`. A route that
is not a path of this site, such as an external URL, a fragment or a `#`, comes back as its
URL, unchanged.

```alder fragment
Url.absolute (Todos.Index.Index { filter: Active }) == "http://localhost:8080/?filter=Active"
Url.absolute (Url.external "https://example.com/a") == "https://example.com/a"
```
