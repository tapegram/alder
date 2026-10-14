--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip t = match t {
  Light -> Dark,
  Dark -> Light,
}

test "flip toggles the theme" =
  { a: flip Light, b: flip Dark } == { a: Dark, b: Light }

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
  owner: User.UserId @ref User @owner,
  created: Time @created,
}

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body class={if props.theme == Dark then Just "dark" else Nothing}>
      <al-page width="narrow">
        <nav>
          <button click={App.SwitchTheme}>
            {match props.theme {
              Light -> "Switch to dark",
              Dark -> "Switch to light",
            }}
          </button>
          <button click={App.Logout}>Log out</button>
        </nav>
        <main>{props.children}</main>
      </al-page>
    </body>
  </html>

--- file: app/App.alder ---
module App

import User
import Theme
import Layout

session { user: Maybe User.UserId, theme: Theme.Theme = Light }

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

test "the theme flips and the body class follows" = {
  let _ <- Test.get (Auth.Login {})
  let first <- Test.click SwitchTheme
  let dark <- Test.get (Auth.Login {})
  let second <- Test.click SwitchTheme
  let light <- Test.get (Auth.Login {})
  Task.succeed ({ a: first.session.theme, b: Test.has "body.dark" dark.doc, c: second.session.theme, d: Test.has "body.dark" light.doc } == { a: Dark, b: True, c: Light, d: False })
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

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
      <Input field="password" label="Password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Notes.Index {}), user: Just me })
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import Note
import Layout
import User

page "/" { } requires user

type Model = { notes: List Note }

load : Params -> Session -> Query Model
load params session = {
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}

form NewNote = {
  title: Text.NonEmpty,
  body: Text,
}

msg Submit form NewNote

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let id <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Notes" theme={session.theme}>
    <h1>Notes</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="title" label="Title" />
      <Textarea field="body" label="Body" />
      <button>Add note</button>
    </form>
    <ul id="notes">
      {for n in model.notes ->
        <li><a href={Notes.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  </Layout.Page>

test "the newest note is listed first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "First", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Second", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
}

test "logging out works from the list" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ answer: out.answer, user: out.session.user } == { answer: Redirected (Auth.Login {}), user: Nothing })
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
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}

form EditNote = {
  title: Text.NonEmpty,
  body: Text,
}

msg Save form EditNote
msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Show { id: model.note.id }))
  },
  Delete p -> {
    let _ <- Db.deleteMine Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1>{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <form submit={Save}>
      <Errors form />
      <Input field="title" label="Title" value={model.note.title} />
      <Textarea field="body" label="Body" value={model.note.body} />
      <button>Save</button>
    </form>
    <button click={Delete}>Delete</button>
    <a href={Notes.Index {}}>Back</a>
  </Layout.Page>

test "a user cannot see another user's note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "hidden", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "the owner can edit and delete a note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Old", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Show { id: id })
  let saved <- Test.submit EditNote { title: Text.NonEmpty "New", body: "text" }
  let after <- Db.get Note id
  let _ <- Test.get (Notes.Show { id: id })
  let gone <- Test.click Delete
  let last <- Db.get Note id
  Task.succeed ({ saved: saved.answer, title: Maybe.map (\n -> n.title) after, body: Maybe.map (\n -> n.body) after, gone: gone.answer, left: Maybe.map (\n -> n.title) last } == { saved: Redirected (Notes.Show { id: id }), title: Just (Text.NonEmpty "New"), body: Just "text", gone: Redirected (Notes.Index {}), left: Nothing })
}
