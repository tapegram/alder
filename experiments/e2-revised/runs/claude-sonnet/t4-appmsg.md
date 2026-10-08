--- file: app/App.alder ---
module App

import User
import Theme
import Auth.Login
import Notes.Index

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout = "/logout" { }
msg SwitchTheme = "/theme" { }

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed (Redirect (Notes.Index {}))
  },
}

error : Error -> Html Never
error err =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>Error</title>
    </head>
    <body>
      <main>
        <h1>Something went wrong</h1>
        <p>Please try again.</p>
      </main>
    </body>
  </html>

notFound : Html Never
notFound =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>Not found</title>
    </head>
    <body>
      <main>
        <h1>Not found</h1>
      </main>
    </body>
  </html>

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

switchLabel : Theme -> Text
switchLabel theme = match theme {
  Light -> "Switch to dark",
  Dark -> "Switch to light",
}

bodyClass : Theme -> Maybe Text
bodyClass theme = match theme {
  Light -> Nothing,
  Dark -> Just "dark",
}

test "flip toggles light to dark" =
  flip Light == Dark

test "flip toggles dark to light" =
  flip Dark == Light

test "switch label for light offers dark" =
  switchLabel Light == "Switch to dark"

test "switch label for dark offers light" =
  switchLabel Dark == "Switch to light"

--- file: app/Layout.alder ---
module Layout

import App
import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body class={Theme.bodyClass props.theme}>
      <main>
        <div id="toolbar">
          <button click={App.SwitchTheme}>{Theme.switchLabel props.theme}</button>
          <button click={App.Logout}>Log out</button>
        </div>
        {props.children}
      </main>
    </body>
  </html>

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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout
import Notes.Index

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

page "/login" { }

type Model = { theme: Theme }

load : Params -> Session -> Query Model
load params session = Query.succeed { theme: session.theme }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit = "/login" form Credentials

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

view : Model -> Html Msg
view model =
  <Layout.Page title="Log in" theme={model.theme}>
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
import Notes.Show

page "/" { } requires user

type Model = {
  theme: Theme,
  notes: List Note,
}

load : Params -> Session -> Query Model
load params session = {
  let notes <-
    Db.from Note
      |> Db.where (Note.owner.eq session.user)
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { theme: session.theme, notes: notes }
}

form NewNote = {
  title: Text.NonEmpty,
  body: Text,
}

msg Submit = "/notes" form NewNote

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Notes" theme={model.theme}>
    <h1>Notes</h1>

    <form submit={Submit}>
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
    <span class="created">{Time.format "Jan 2, 2006" props.note.created}</span>
  </li>

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import Layout
import Notes.Index

page "/notes/{id}" { id: Note.NoteId } requires user

type Model = {
  theme: Theme,
  note: Note,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Note params.id
  match found {
    Just note -> if note.owner == session.user then Query.succeed { theme: session.theme, note: note } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form EditNote = {
  title: Text.NonEmpty,
  body: Text,
}

msg Save = "/notes/{id}/save" { id: Note.NoteId } form EditNote
msg Delete = "/notes/{id}/delete" { id: Note.NoteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Note p.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Show { id: p.id }))
  },
  Delete p -> {
    let _ <- Db.delete Note p.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.note.title} theme={model.theme}>
    <h1>{model.note.title}</h1>
    <p class="created">{Time.format "Jan 2, 2006" model.note.created}</p>
    <p id="body">{model.note.body}</p>
    <form submit={Save { id: model.note.id }}>
      <Input field="title" label="Title" value={model.note.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body" value={model.note.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>
    <button click={Delete { id: model.note.id }}>Delete</button>
    <a href={Notes.Index {}}>Back</a>
  </Layout.Page>
