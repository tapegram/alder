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

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip turns light into dark" = flip Light == Dark

test "flip alternates" = flip (flip Light) == Light

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body class={if props.theme == Dark then Just "dark" else Nothing}>
      <nav>
        <button id="theme" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
        <button id="logout" click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/App.alder ---
module App

import User
import Theme
import Layout

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg SwitchTheme
msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
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

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
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

--- file: app/Notes/Index.alder ---
module Notes.Index

import Note
import Layout
import User
import Theme

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

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Notes" theme={session.theme}>
    <h1>Notes</h1>

    <form submit={Add}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Add</button>
    </form>

    <ul id="notes">
      {for note in model.notes ->
        <li><a class="title" href={Notes.Show { id: note.id }}>{note.title}</a></li>}
    </ul>
  </Layout.Page>

test "adding a note through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me, theme: Light }
  let _ <- Test.get (Notes.Index {})
  let added <- Test.submit NewNote { title: Text.NonEmpty "First", body: "hello" }
  let page <- Test.get (Notes.Index {})
  Task.succeed ({ answer: added.answer, titles: Test.texts "#notes a.title" page.doc, items: Test.count "#notes > li" page.doc } == { answer: Redirected (Notes.Index {}), titles: ["First"], items: 1 })
}

test "the list shows only my notes" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "Mine", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "", owner: other }
  let _ <- Test.login { user: Just me, theme: Light }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "#notes a.title" page.doc == ["Mine"])
}

test "the theme flips" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me, theme: Light }
  let before <- Test.get (Notes.Index {})
  let _ <- Test.click App.SwitchTheme
  let dark <- Test.get (Notes.Index {})
  let _ <- Test.click App.SwitchTheme
  let light <- Test.get (Notes.Index {})
  Task.succeed ({
    before: Test.text "#theme" before.doc,
    beforeDark: Test.has "body.dark" before.doc,
    theme: dark.session.theme,
    dark: Test.has "body.dark" dark.doc,
    label: Test.text "#theme" dark.doc,
    user: dark.session.user,
    again: light.session.theme,
    lightDark: Test.has "body.dark" light.doc,
    lightLabel: Test.text "#theme" light.doc,
  } == {
    before: Just "Switch to dark",
    beforeDark: False,
    theme: Dark,
    dark: True,
    label: Just "Switch to light",
    user: Just me,
    again: Light,
    lightDark: False,
    lightLabel: Just "Switch to dark",
  })
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import Layout
import User
import Theme

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

form EditNote = {
  title: Text.NonEmpty,
  body: Text,
}

-- The messages carry no id: the page's parameter names the note, and `load` has checked that
-- the session's user owns it, so `update` acts on `model.note`.
msg Save form EditNote
msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Show { id: model.note.id }))
  },
  Delete p -> {
    let _ <- Db.delete Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>

    <form submit={Save}>
      <Errors form />
      <Input field="title" label="Title" value={model.note.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body" value={model.note.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>

    <button click={Delete}>Delete</button>
    <a href={Notes.Index {}}>Back</a>
  </Layout.Page>

test "another user's note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "mine", owner: me }
  let _ <- Test.login { user: Just other, theme: Light }
  let theirs <- Test.open (Notes.Show { id: id })
  let _ <- Test.login { user: Just me, theme: Light }
  let mine <- Test.get (Notes.Show { id: id })
  Task.succeed ({ theirs: theirs.answer, title: Test.text "#title" mine.doc, body: Test.text "#body" mine.doc } == { theirs: PageMissing, title: Just "Secret", body: Just "mine" })
}

test "a missing note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me, theme: Light }
  let r <- Test.open (Notes.Show { id: Note.NoteId "nope" })
  Task.succeed (r.answer == PageMissing)
}

test "deleting a note through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Old", body: "", owner: me }
  let _ <- Test.login { user: Just me, theme: Light }
  let _ <- Test.get (Notes.Show { id: id })
  let r <- Test.click Delete
  let rows <- Db.from Note |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: Redirected (Notes.Index {}), rows: 0 })
}
