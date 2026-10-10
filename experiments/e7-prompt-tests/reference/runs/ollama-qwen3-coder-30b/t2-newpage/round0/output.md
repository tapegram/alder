--- file: app/App.alder ---
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Todos.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" theme={session.theme}>
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Profile.alder ---
module Profile

import Todo
import Layout
import User

page "/profile" { } requires user

type Model = {
  name: Text.NonEmpty,
  email: Text.Email,
  todoCount: Int,
  doneCount: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let todos <- Db.from Todo |> Db.where (Todo.owner.eq session.user) |> Db.all
  let todoCount = List.length todos
  let doneCount = List.length (List.filter (\t -> t.done) todos)
  let user <- Db.get User session.user
  match user {
    Just u -> Query.succeed { name: u.name, email: u.email, todoCount: todoCount, doneCount: doneCount }
    Nothing -> Query.notFound,
  }
}

form UpdateName = {
  name: Text.NonEmpty,
}

msg Save form UpdateName

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Profile" theme={session.theme}>
    <h1>Profile</h1>
    <p>Name: {model.name}</p>
    <p>Email: {model.email}</p>
    <p>Todos: {Text.fromInt model.todoCount} ({Text.fromInt model.doneCount} done)</p>

    <form submit={Save}>
      <Input field="name" label="Name" value={model.name} />
      <Errors field="name" />
      <button>Update Name</button>
    </form>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "load counts todos correctly" = {
  let user <- Db.insert User { email: Text.Email "user@example.com", name: Text.NonEmpty "User", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Todo 1", due: Nothing, owner: user }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Todo 2", due: Nothing, owner: user }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Todo 3", due: Nothing, owner: user }
  let r <- Test.run (load {} { user: user, theme: Light })
  let result = r.value
  match result {
    Ok m -> Task.succeed (m.todoCount == 3 && m.doneCount == 0)
    Err _ -> Task.succeed False,
  }
}

test "Save with empty name is refused" = {
  let user <- Db.insert User { email: Text.Email "user@example.com", name: Text.NonEmpty "User", passwordHash: "" }
  let r <- Test.run (update (Save { form: { name: Text.NonEmpty "" } }) { name: Text.NonEmpty "User", email: Text.Email "user@example.com", todoCount: 0, doneCount: 0 } { user: user, theme: Light })
  Task.succeed (r.value == Err "not found")
}

--- file: app/Todo.alder ---
module Todo

import User

type TodoId = brand Text

type Filter = All | Active | Completed

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User,
  created: Time @created,
}

matches : Filter -> Todo -> Bool
matches filter todo = match filter {
  All -> True,
  Active -> not todo.done,
  Completed -> todo.done,
}

filterLabel : Filter -> Text
filterLabel filter = match filter {
  All -> "All",
  Active -> "Active",
  Completed -> "Completed",
}

countOpen : List Todo -> Int
countOpen todos = todos |> List.filter (\t -> not t.done) |> List.length

test "countOpen ignores done todos" =
  countOpen [
    { id: TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: TodoId "b", title: Text.NonEmpty "y", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 1

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import Layout
import Theme
import User

page "/" { filter: Todo.Filter = All } requires user

type Model = {
  filter: Todo.Filter,
  todos: List Todo,
}

load : Params -> Session -> Query Model
load params session = {
  let todos <-
    Db.from Todo
      |> Db.where (Todo.owner.eq session.user)
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}

form NewTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
}

msg Submit form NewTodo
msg Toggle { id: Todo.TodoId }
msg Delete { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
    Task.succeed (Redirect (Todos.Index { filter: model.filter }))
  },
  Toggle p -> {
    let todo <- owned p.id session
    let _ <- Db.update Todo todo.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  Delete p -> {
    let todo <- owned p.id session
    let _ <- Db.delete Todo todo.id
    Task.succeed Rerender
  },
}

-- owned is the todo a message names, when the session's user owns it; otherwise the message is
-- a 404. `load` only lists the user's todos, but a message's id comes from the request: anyone
-- can post `Toggle` with another user's id, so `update` checks it again.
owned : Todo.TodoId -> Session -> Task Todo
owned id session = {
  let found <- Db.get Todo id
  match found {
    Just todo -> if todo.owner == session.user then Task.succeed todo else Task.notFound,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Todos" theme={session.theme}>
    <h1>Todos</h1>

    <form submit={Submit}>
      <Input field="title" label="What needs doing?" />
      <Errors field="title" />
      <Input field="due" label="Due" />
      <Errors field="due" />
      <button>Add</button>
    </form>

    <nav>
      {for f in [All, Active, Completed] ->
        <a href={Todos.Index { filter: f }} aria-current={if model.filter == f then Just "page" else Nothing}>
          {Todo.filterLabel f}
        </a>}
    </nav>

    <ul id="todos">
      {for todo in model.todos -> <Item todo={todo} />}
    </ul>

    <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>

    <button click={App.Logout}>Log out</button>
    <a href={Profile {}}>Profile</a>
  </Layout.Page>

Item : { todo: Todo } -> Html Msg
Item props =
  <li class={if props.todo.done then "done" else ""}>
    <button click={Toggle { id: props.todo.id }}>{if props.todo.done then "Undo" else "Done"}</button>
    <a href={Todos.Show { id: props.todo.id }}>{props.todo.title}</a>
    {match props.todo.due {
      Just d -> <span class="due">{Date.format "Jan 2" d}</span>,
      Nothing -> Html.none,
    }}
    <button click={Delete { id: props.todo.id }}>Delete</button>
  </li>

test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: me, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.get Todo", "Db.update Todo"], done: Just True })
}

test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.get Todo"], done: Just False })
}

--- file: app/Todos/Show.alder ---
module Todos.Show

import Todo
import Layout

page "/todos/{id}" { id: Todo.TodoId } requires user

type Model = { todo: Todo }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Todo params.id
  match found {
    Just todo -> if todo.owner == session.user then Query.succeed { todo: todo } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form EditTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
}

-- The messages carry no id: the page's parameter names the todo, and `load` has checked that
-- the session's user owns it, so `update` acts on `model.todo` (book 06, "Authorize in update").
msg Save form EditTodo
msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Todo model.todo.id (\t -> { t | title: p.form.title, due: p.form.due })
    Task.succeed (Redirect (Todos.Index {}))
  },
  Delete p -> {
    let _ <- Db.delete Todo model.todo.id
    Task.succeed (Redirect (Todos.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.todo.title} theme={session.theme}>
    <h1>{model.todo.title}</h1>
    <form submit={Save}>
      <Input field="title" label="Title" value={model.todo.title} />
      <Errors field="title" />
      <Input field="due" label="Due" value={model.todo.due} />
      <Errors field="due" />
      <button>Save</button>
    </form>
    <button click={Delete}>Delete</button>
    <a href={Todos.Index {}}>Back</a>
  </Layout.Page>

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}

--- file: stdlib/Test.alder ---
module Test

import Db
import Session
import Time

-- The test runtime.
type Trace = List Text

-- Run a task and return its result and the steps it performed.
run : Task a -> Task { value: Result Text a, trace: Trace }

-- Get the list of steps so far.
trace : Task Trace

-- A test is a Bool that is True when it passes.
test : Text -> Bool -> Unit
test name verdict = if not verdict then Test.fail ("FAIL " ++ name) else ()

-- The test runner fails the test with this message.
fail : Text -> Task a

--- file: stdlib/Time.alder ---
module Time

import Db

type Time = brand Text

epoch : Time

now : Query Time

toDate : Time -> Date

format : Text -> Time -> Text

ago : Time -> Time -> Text

--- file: stdlib/Random.alder ---
module Random

int : { min: Int, max: Int } -> Task Int

id : Task Text

--- file: stdlib/Http.alder ---
module Http

type HttpError = {
  status: Int,
  body: Text,
}

get : { url: Text } -> Task (Result HttpError Text)

post : { url: Text, body: Text } -> Task (Result HttpError Text)

--- file: stdlib/Log.alder ---
module Log

info : Text -> Task Unit

warn : Text -> Task Unit

--- file: stdlib/Session.alder ---
module Session

put : (Session -> Session) -> Task Unit

type Session = { user: Maybe User.UserId, theme: Theme }

--- file: stdlib/Db.alder ---
module Db

-- A table is a type that has an `id` field of type `Id t`.
type Table t = t

-- The id of a row in a table.
type Id t = brand Text

-- A condition on a table's column.
type Condition t = Unit

-- A sort order for a table.
type Order t = Unit

-- A query that can be run to get rows from the database.
type Select t = Unit

get : Table t -> Id t -> Query (Maybe t)

from : Table t -> Select t

where : Condition t -> Select t -> Select t

orderBy : Order t -> Select t -> Select t

limit : Int -> Select t -> Select t

offset : Int -> Select t -> Select t

all : Select t -> Query (List t)

first : Select t -> Query (Maybe t)

count : Select t -> Query Int

insert : Table t -> Insert t -> Task (Id t)

update : Table t -> Id t -> (t -> t) -> Task Unit

delete : Table t -> Id t -> Task Unit

-- A condition that is true if the column's value equals the given value.
eq : a -> Condition t

-- A condition that is true if the column's value does not equal the given value.
ne : a -> Condition t

-- A condition that is true if the column's value is less than the given value.
lt : a -> Condition t

-- A condition that is true if the column's value is less than or equal to the given value.
le : a -> Condition t

-- A condition that is true if the column's value is greater than the given value.
gt : a -> Condition t

-- A condition that is true if the column's value is greater than or equal to the given value.
ge : a -> Condition t

-- A condition that is true if the column's value contains the given substring (case-insensitive).
contains : Text -> Condition t

-- A condition that is true if the column's value is in the list of values.
in : List a -> Condition t

-- A condition that is true if the column's value is Nothing.
isNothing : Condition t

-- A condition that is true if the column's value is Just.
isJust : Condition t

-- A sort order that sorts by the column in ascending order.
asc : Order t

-- A sort order that sorts by the column in descending order.
desc : Order t

-- Combine multiple conditions with AND.
and : List (Condition t) -> Condition t

-- Combine multiple conditions with OR.
or : List (Condition t) -> Condition t

-- Negate a condition.
not : Condition t -> Condition t

-- A record type that represents the fields of a table that can be inserted.
type Insert t = Unit

--- file: stdlib/Html.alder ---
module Html

-- Unsafe HTML text.
unsafe : Text -> Html Never

-- No HTML.
none : Html a

-- HTML text.
text : Text -> Html a

-- A URL to an external page.
external : Text -> Page

--- file: stdlib/Date.alder ---
module Date

today : Query Date

fromText : Text -> Maybe Date

toText : Date -> Text

format : Text -> Date -> Text

addDays : Int -> Date -> Date

diffDays : Date -> Date -> Int

year : Date -> Int

month : Date -> Int

day : Date -> Int

--- file: stdlib/Dict.alder ---
module Dict

empty : Dict k v

fromList : List { key: k, value: v } -> Dict k v

toList : Dict k v -> List { key: k, value: v }

get : k -> Dict k v -> Maybe v

insert : k -> v -> Dict k v -> Dict k v

remove : k -> Dict k v -> Dict k v

keys : Dict k v -> List k

values : Dict k v -> List v

size : Dict k v -> Int

--- file: stdlib/Bool.alder ---
module Bool

not : Bool -> Bool

toText : Bool -> Text

--- file: stdlib/Float.alder ---
module Float

round : Float -> Int

floor : Float -> Int

ceil : Float -> Int

min : Float -> Float -> Float

max : Float -> Float -> Float

abs : Float -> Float

format : { decimals: Int } -> Float -> Text

--- file: stdlib/Int.alder ---
module Int

toFloat : Int -> Float

min : Int -> Int -> Int

max : Int -> Int -> Int

abs : Int -> Int

clamp : { min: Int, max: Int } -> Int -> Int

Positive : brand Int

NonNegative : brand Int

--- file: stdlib/Text.alder ---
module Text

length : Text -> Int

isEmpty : Text -> Bool

trim : Text -> Text

toUpper : Text -> Text

toLower : Text -> Text

contains : Text -> Text -> Bool

startsWith : Text -> Text -> Bool

endsWith : Text -> Text -> Bool

replace : Text -> Text -> Text -> Text

split : Text -> Text -> List Text

join : Text -> List Text -> Text

slice : Int -> Int -> Text -> Text

matches : Text -> Text -> Bool

fromInt : Int -> Text

fromFloat : Float -> Text

toInt : Text -> Maybe Int

toFloat : Text -> Maybe Float

NonEmpty : brand Text

Email : brand Text

Url : brand Text

Secret : brand Text

--- file: stdlib/List.alder ---
module List

length : List a -> Int

isEmpty : List a -> Bool

head : List a -> Maybe a

last : List a -> Maybe a

take : Int -> List a -> List a

drop : Int -> List a -> List a

reverse : List a -> List a

map : (a -> b) -> List a -> List b

filter : (a -> Bool) -> List a -> List a

filterMap : (a -> Maybe b) -> List a -> List b

find : (a -> Bool) -> List a -> Maybe a

any : (a -> Bool) -> List a -> Bool

all : (a -> Bool) -> List a -> Bool

member : a -> List a -> Bool

foldl : (a -> b -> b) -> b -> List a -> b

sum : List Int -> Int

sumFloat : List Float -> Float

sortBy : (a -> Int) -> List a -> List a

sortByText : (a -> Text) -> List a -> List a

indexed : List a -> List { index: Int, item: a }

range : Int -> Int -> List Int

concat : List (List a) -> List a

append : List a -> List a -> List a

unique : List a -> List a

groupBy : (a -> Text) -> List a -> Dict Text (List a)

--- file: stdlib/Maybe.alder ---
module Maybe

map : (a -> b) -> Maybe a -> Maybe b

andThen : (a -> Maybe b) -> Maybe a -> Maybe b

withDefault : a -> Maybe a -> a

isJust : Maybe a -> Bool

toList : Maybe a -> List a

--- file: stdlib/Result.alder ---
module Result

map : (a -> b) -> Result e a -> Result e b

mapErr : (e -> f) -> Result e a -> Result f a

andThen : (a -> Result e b) -> Result e a -> Result e b

withDefault : a -> Result e a -> a

toMaybe : Result e a -> Maybe a

--- file: stdlib/Url.alder ---
module Url

-- External URL.
external : Text -> Page

--- file: stdlib/Query.alder ---
module Query

succeed : a -> Query a

map : (a -> b) -> Query a -> Query b

all : List (Query a) -> Query (List a)

notFound : Query a

--- file: stdlib/Task.alder ---
module Task

succeed : a -> Task a

fail : Text -> Task a

notFound : Task a

map : (a -> b) -> Task a -> Task b

all : List (Task a) -> Task (List a)

--- file: stdlib/Brand.alder ---
module Brand

-- A brand is a type that has the same runtime representation as its base type
-- but is distinct to the type checker.
type Brand t = t

-- Widening is implicit. A brand can be used anywhere its base type is expected.
-- Narrowing is explicit and local. The constructor exists only inside the module that declares the brand.
-- Operations return the base type.
-- Equality requires identical types.

--- file: stdlib/Type.alder ---
module Type

-- This module is a placeholder for type-related functionality.
-- It's not actually used in the language but serves as a reference point.
type Type = Unit

--- file: stdlib/Unit.alder ---
module Unit

-- The unit type.
type Unit = ()

-- A value of the unit type.
unit : Unit

--- file: stdlib/Bool.alder ---
module Bool

not : Bool -> Bool

toText : Bool -> Text

--- file: stdlib/Int.alder ---
module Int

toFloat : Int -> Float

min : Int -> Int -> Int

max : Int -> Int -> Int

abs : Int -> Int

clamp : { min: Int, max: Int } -> Int -> Int

Positive : brand Int

NonNegative : brand Int

--- file: stdlib/Float.alder ---
module Float

round : Float -> Int

floor : Float -> Int

ceil : Float -> Int

min : Float -> Float -> Float

max : Float -> Float -> Float

abs : Float -> Float

format : { decimals: Int } -> Float -> Text

--- file: stdlib/Text.alder ---
module Text

length : Text -> Int

isEmpty : Text -> Bool

trim : Text -> Text

toUpper : Text -> Text

toLower : Text -> Text

contains : Text -> Text -> Bool

startsWith : Text -> Text -> Bool

endsWith : Text -> Text -> Bool

replace : Text -> Text -> Text -> Text

split : Text -> Text -> List Text

join : Text -> List Text -> Text

slice : Int -> Int -> Text -> Text

matches : Text -> Text -> Bool

fromInt : Int -> Text

fromFloat : Float -> Text

toInt : Text -> Maybe Int

toFloat : Text -> Maybe Float

NonEmpty : brand Text

Email : brand Text

Url : brand Text

Secret : brand Text

--- file: stdlib/Date.alder ---
module Date

today : Query Date

fromText : Text -> Maybe Date

toText : Date -> Text

format : Text -> Date -> Text

addDays : Int -> Date -> Date

diffDays : Date -> Date -> Int

year : Date -> Int

month : Date -> Int

day : Date -> Int

--- file: stdlib/Html.alder ---
module Html

-- Unsafe HTML text.
unsafe : Text -> Html Never

-- No HTML.
none : Html a

-- HTML text.
text : Text -> Html a

-- A URL to an external page.
external : Text -> Page

--- file: stdlib/Dict.alder ---
module Dict

empty : Dict k v

fromList : List { key: k, value: v } -> Dict k v

toList : Dict k v -> List { key: k, value: v }

get : k -> Dict k v -> Maybe v

insert : k -> v -> Dict k v -> Dict k v

remove : k -> Dict k v -> Dict k v

keys : Dict k v -> List k

values : Dict k v -> List v

size : Dict k v -> Int

--- file: stdlib/Result.alder ---
module Result

map : (a -> b) -> Result e a -> Result e b

mapErr : (e -> f) -> Result e a -> Result f a

andThen : (a -> Result e b) -> Result e a -> Result e b

withDefault : a -> Result e a -> a

toMaybe : Result e a -> Maybe a

--- file: stdlib/Maybe.alder ---
module Maybe

map : (a -> b) -> Maybe a -> Maybe b

andThen : (a -> Maybe b) -> Maybe a -> Maybe b

withDefault : a -> Maybe a -> a

isJust : Maybe a -> Bool

toList : Maybe a -> List a

--- file: stdlib/List.alder ---
module List

length : List a -> Int

isEmpty : List a -> Bool

head : List a -> Maybe a

last : List a -> Maybe a

take : Int -> List a -> List a

drop : Int -> List a -> List a

reverse : List a -> List a

map : (a -> b) -> List a -> List b

filter : (a -> Bool) -> List a -> List a

filterMap : (a -> Maybe b) -> List a -> List b

find : (a -> Bool) -> List a -> Maybe a

any : (a -> Bool) -> List a -> Bool

all : (a -> Bool) -> List a -> Bool

member : a -> List a -> Bool

foldl : (a -> b -> b) -> b -> List a -> b

sum : List Int -> Int

sumFloat : List Float -> Float

sortBy : (a -> Int) -> List a -> List a

sortByText : (a -> Text) -> List a -> List a

indexed : List a -> List { index: Int, item: a }

range : Int -> Int -> List Int

concat : List (List a) -> List a

append : List a -> List a -> List a

unique : List a -> List a

groupBy : (a -> Text) -> List a -> Dict Text (List a)

--- file: stdlib/Session.alder ---
module Session

put : (Session -> Session) -> Task Unit

type Session = { user: Maybe User.UserId, theme: Theme }

--- file: stdlib/Time.alder ---
module Time

import Db

type Time = brand Text

epoch : Time

now : Query Time

toDate : Time -> Date

format : Text -> Time -> Text

ago : Time -> Time -> Text

--- file: stdlib/Random.alder ---
module Random

int : { min: Int, max: Int } -> Task Int

id : Task Text

--- file: stdlib/Http.alder ---
module Http

type HttpError = {
  status: Int,
  body: Text,
}

get : { url: Text } -> Task (Result HttpError Text)

post : { url: Text, body: Text } -> Task (Result HttpError Text)

--- file: std::Log.alder ---
module Log

info : Text -> Task Unit

warn : Text -> Task Unit

--- file: stdlib/Db.alder ---
module Db

-- A table is a type that has an `id` field of type `Id t`.
type Table t = t

-- The id of a row in a table.
type Id t = brand Text

-- A condition on a table's column.
type Condition t = Unit

-- A sort order for a table.
type Order t = Unit

-- A query that can be run to get rows from the database.
type Select t = Unit

get : Table t -> Id t -> Query (Maybe t)

from : Table t -> Select t

where : Condition t -> Select t -> Select t

orderBy : Order t -> Select t -> Select t

limit : Int -> Select t -> Select t

offset : Int -> Select t -> Select t

all : Select t -> Query (List t)

first : Select t -> Query (Maybe t)

count : Select t -> Query Int

insert : Table t -> Insert t -> Task (Id t)

update : Table t -> Id t -> (t -> t) -> Task Unit

delete : Table t -> Id t -> Task Unit

-- A condition that is true if the column's value equals the given value.
eq : a -> Condition t

-- A condition that is true if the column's value does not equal the given value.
ne : a -> Condition t

-- A condition that is true if the column's value is less than the given value.
lt : a -> Condition t

-- A condition that is true if the column's value is less than or equal to the given value.
le : a -> Condition t

-- A condition that is true if the column's value is greater than the given value.
gt : a -> Condition t

-- A condition that is true if the column's value is greater than or equal to the given value.
ge : a -> Condition t

-- A condition that is true if the column's value contains the given substring (case-insensitive).
contains : Text -> Condition t

-- A condition that is true if the column's value is in the list of values.
in : List a -> Condition t

-- A condition that is true if the column's value is Nothing.
isNothing : Condition t

-- A condition that is true if the column's value is Just.
isJust : Condition t

-- A sort order that sorts by the column in ascending order.
asc : Order t

-- A sort order that sorts by the column in descending order.
desc : Order t

-- Combine multiple conditions with AND.
and : List (Condition t) -> Condition t

-- Combine multiple conditions with OR.
or : List (Condition t) -> Condition t

-- Negate a condition.
not : Condition t -> Condition t

-- A record type that represents the fields of a table that can be inserted.
type Insert t = Unit

--- file: stdlib/Html.alder ---
module Html

-- Unsafe HTML text.
unsafe : Text -> Html Never

-- No HTML.
none : Html a

-- HTML text.
text : Text -> Html a

-- A URL to an external page.
external : Text -> Page

--- file: stdlib/Date.alder ---
module Date

today : Query Date

fromText : Text -> Maybe Date

toText : Date -> Text

format : Text -> Date -> Text

addDays : Int -> Date -> Date

diffDays : Date -> Date -> Int

year : Date -> Int

month : Date -> Int

day : Date -> Int

--- file: stdlib/Dict.alder ---
module Dict

empty : Dict k v

fromList : List { key: k, value: v } -> Dict k v

toList : Dict k v -> List { key: k, value: v }

get : k -> Dict k v -> Maybe v

insert : k -> v -> Dict k v -> Dict k v

remove : k -> Dict k v -> Dict k v

keys : Dict k v -> List k

values : Dict k v -> List v

size : Dict k v -> Int

--- file: stdlib/Result.alder ---
module Result

map : (a -> b) -> Result e a -> Result e b

mapErr : (e -> f) -> Result e a -> Result f a

andThen : (a -> Result e b) -> Result e a -> Result e b

withDefault : a -> Result e a -> a

toMaybe : Result e a -> Maybe a

--- file: stdlib/Maybe.alder ---
module Maybe

map : (a -> b) -> Maybe a -> Maybe b

andThen : (a -> Maybe b) -> Maybe a -> Maybe b

withDefault : a -> Maybe a -> a

isJust : Maybe a -> Bool

toList : Maybe a -> List a

--- file: stdlib/List.alder ---
module List

length : List a -> Int

isEmpty : List a -> Bool

head : List a -> Maybe a

last : List a -> Maybe a

take : Int -> List a -> List a

drop : Int -> List a -> List a

reverse : List a -> List a

map : (a -> b) -> List a -> List b

filter : (a -> Bool) -> List a -> List a

filterMap : (a -> Maybe b) -> List a -> List b

find : (a -> Bool) -> List a -> Maybe a

any : (a -> Bool) -> List a -> Bool

all : (a -> Bool) -> List a -> Bool

member : a -> List a -> Bool

foldl : (a -> b -> b) -> b -> List a -> b

sum : List Int -> Int

sumFloat : List Float -> Float

sortBy : (a -> Int) -> List a -> List a

sortByText : (a -> Text) -> List a -> List a

indexed : List a -> List { index: Int, item: a }

range : Int -> Int -> List Int

concat : List (List a) -> List a

append : List a -> List a -> List a

unique : List a -> List a

groupBy : (a -> Text) -> List a -> Dict Text (List a)

--- file: stdlib/Session.alder ---
module Session

put : (Session -> Session) -> Task Unit

type Session = { user: Maybe User.UserId, theme: Theme }

--- file: stdlib/Time.alder ---
module Time

import Db

type Time = brand Text

epoch : Time

now : Query Time

toDate : Time -> Date

format : Text -> Time -> Text

ago : Time -> Time -> Text

--- file: stdlib