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

test "a user's name is kept" =
  Text.NonEmpty "Me" == Text.NonEmpty "Me"

--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Note.alder ---
module Note

import User

type NoteId = brand Text

table Note {
  id: NoteId @id,
  title: Text.NonEmpty,
  body: Text,
  owner: User.UserId @ref User,
  created: Time @created,
}

test "a note's title is kept" =
  Text.NonEmpty "Milk" == Text.NonEmpty "Milk"

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place.
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
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
        Task.succeed (Redirect (Notes.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in">
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

--- file: app/Notes/Index.alder ---
module Notes.Index

import Note
import Layout
import User

page "/" { } requires user

type Model = {
  notes: List Note,
}

load : Params -> Session -> Query Model
load params session = {
  let notes <-
    Db.from Note
      |> Db.where (Note.owner.eq session.user)
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes }
}

form NewNote = {
  title: Text.NonEmpty,
  body: Text,
}

msg Add form NewNote
msg Delete { id: Note.NoteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
    Task.succeed (Redirect (Notes.Index {}))
  },
  Delete p -> {
    let note <- owned p.id session
    let _ <- Db.delete Note note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}

-- owned is the note a message names, when the session's user owns it; otherwise a 404.
owned : Note.NoteId -> Session -> Task Note
owned id session = {
  let found <- Db.get Note id
  match found {
    Just note -> if note.owner == session.user then Task.succeed note else Task.notFound,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Notes">
    <h1>Notes</h1>

    <form submit={Add}>
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="body" label="Body" />
      <Errors field="body" />
      <button>Add</button>
    </form>

    <ul id="notes">
      {for note in model.notes -> <Item note={note} />}
    </ul>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

Item : { note: Note } -> Html Msg
Item props =
  <li>
    <a class="title" href={Notes.Show { id: props.note.id }}>{props.note.title}</a>
    <button click={Delete { id: props.note.id }}>Delete</button>
  </li>

test "deleting another user's note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: other }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: Just me })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Err "not found", note: Just (Text.NonEmpty "Secret") })
}

test "deleting a missing note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let r <- Test.run (update (Delete { id: Note.NoteId "nope" }) { notes: [] } { user: Just me })
  Task.succeed (r.value == Err "not found")
}

test "the owner can delete a note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: Just me })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Ok (Redirect (Notes.Index {})), note: Nothing })
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import Layout
import User

page "/notes/{id}" { id: Note.NoteId } requires user

type Model = { note: Note }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Note params.id
  match found {
    Just note -> if note.owner == session.user then Query.succeed { note: note } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NoForm = { }

msg Remove

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Remove p -> {
    let _ <- Db.delete Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.note.title}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <button click={Remove}>Delete</button>
    <a href={Notes.Index {}}>Back</a>
  </Layout.Page>

test "a visitor is sent to log in from a note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let visitor <- Test.open (Notes.Show { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}
