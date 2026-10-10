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
        Task.succeed (Redirect (Notes.Index {}))
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
  body: Text = "",
  owner: User.UserId @ref User,
  created: Time @created,
}

test "note can be created with empty body" = {
  let n = { id: NoteId "a", title: Text.NonEmpty "x", body: "", owner: User.UserId "u", created: Time.epoch }
  n.title == Text.NonEmpty "x" && n.body == ""
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import Note
import Layout
import Theme
import User

page "/" { } requires user

type Model = { notes: List Note }

load : Params -> Session -> Query Model
load params session = {
  let notes <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}

form NewNote = {
  title: Text.NonEmpty,
  body: Text = "",
}

msg Submit form NewNote
msg Show { id: NoteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
    Task.succeed Rerender
  },
  Show p -> {
    Task.succeed (Redirect (Notes.Show { id: p.id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Notes" theme={session.theme}>
    <h1>Notes</h1>

    <form submit={Submit}>
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Add note</button>
    </form>

    <ul>
      {for note in model.notes ->
        <li>
          <a href={Notes.Show { id: note.id }}>{note.title}</a>
          <span>Created: {Time.ago note.created Time.epoch}</span>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import Layout
import Theme
import User

page "/notes/{id}" { id: NoteId } requires user

type Model = { note: Note }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Note params.id
  match found {
    Just note -> if note.owner == session.user then Query.succeed { note: note } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form EditNote = {
  title: Text.NonEmpty,
  body: Text = "",
}

msg Save form EditNote
msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed Rerender
  },
  Delete p -> {
    let _ <- Db.delete Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1>{model.note.title}</h1>
    <p>{model.note.body}</p>
    <p>Created: {Time.ago model.note.created Time.epoch}</p>

    <form submit={Save}>
      <Input field="title" label="Title" value={model.note.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body" value={model.note.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>

    <button click={Delete}>Delete</button>
    <a href={Notes.Index {}}>Back to notes</a>
  </Layout.Page>

test "user cannot see another user's note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "My Note", body: "", owner: me }
  let stranger = User.UserId "someone-else"
  let r <- Test.run (Notes.Show.update (Notes.Show.Show { id: id }) { note: { id: id, title: Text.NonEmpty "My Note", body: "", owner: me, created: Time.epoch } } { user: stranger, theme: Light })
  Task.succeed (r.value == Err "not found")
}

test "theme flips" = {
  let r <- Test.run (App.update (App.SwitchTheme {}) { user: Nothing, theme: Light } )
  Task.succeed (r.value == Ok Rerender && r.trace == ["Session.put"])
}

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light

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