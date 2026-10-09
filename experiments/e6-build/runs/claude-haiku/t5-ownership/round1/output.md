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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go. It answers whether
-- the password is right for the email; the user row is found, or made on first login, here.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task Bool,
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
    let ok <- Password.verify { email: p.form.email, password: p.form.password }
    if ok then signIn p.form.email else Task.succeed (Reject "Email or password is incorrect.")
  },
}

-- signIn puts the user with this email into the session.
signIn : Text.Email -> Task Nav
signIn email = {
  let found <- Db.first (Db.from User |> Db.where (User.email.eq email))
  match found {
    Just u -> remember u.id,
    Nothing -> {
      let id <- Db.insert User { email: email, name: email, passwordHash: "" }
      remember id
    },
  }
}

remember : User.UserId -> Task Nav
remember id = {
  let _ <- Session.put (\s -> { s | user: Just id })
  Task.succeed (Redirect (Notes.Index {}))
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
      <nav>
        <a href={Notes.Index {}}>Notes</a>
        <button click={App.Logout}>Log out</button>
      </nav>
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

-- ownedBy is the note with this id when it belongs to the user; otherwise Task.notFound (a 404),
-- whether the row is missing or someone else's. A message's id comes from the request, so
-- `update` checks ownership again.
ownedBy : User.UserId -> NoteId -> Task Note
ownedBy user id = {
  let found <- Db.get Note id
  match found {
    Just note -> if note.owner == user then Task.succeed note else Task.notFound,
    Nothing -> Task.notFound,
  }
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import Note
import Layout

page "/" { } requires user

type Model = { notes: List Note }

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
    let note <- Note.ownedBy session.user p.id
    let _ <- Db.delete Note note.id
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Notes">
    <h1>Notes</h1>
    <form submit={Add}>
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Add note</button>
    </form>
    <ul id="notes">
      {for note in model.notes -> <Item note={note} />}
    </ul>
  </Layout.Page>

Item : { note: Note } -> Html Msg
Item props =
  <li>
    <a href={Notes.Show { id: props.note.id }}>{props.note.title}</a>
    <button click={Delete { id: props.note.id }}>Delete</button>
  </li>

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import Layout

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

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.note.title}>
    <h1>{model.note.title}</h1>
    <p>{model.note.body}</p>
    <a href={Notes.Index {}}>Back to notes</a>
  </Layout.Page>

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text,
  passwordHash: Text,
  created: Time @created,
}
