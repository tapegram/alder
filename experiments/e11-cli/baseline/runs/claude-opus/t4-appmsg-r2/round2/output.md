--- file: app/App.alder ---
module App

import User
import Theme
import Layout

session { user: Maybe User.UserId, theme: Theme.Theme = Light }

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

test "SwitchTheme flips the theme and rerenders" = {
  let r <- Test.run (update SwitchTheme { user: Nothing, theme: Light })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Session.put"] })
}

test "the theme flips through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let before <- Test.get (Notes.Index {})
  let dark <- Test.click SwitchTheme
  let darkPage <- Test.get (Notes.Index {})
  let _ <- Test.click SwitchTheme
  let lightPage <- Test.get (Notes.Index {})
  Task.succeed ({ before: Test.text "#theme" before.doc, beforeDark: Test.has "body.dark" before.doc, theme: dark.session.theme, darkLabel: Test.text "#theme" darkPage.doc, darkBody: Test.has "body.dark" darkPage.doc, back: lightPage.session.theme, lightLabel: Test.text "#theme" lightPage.doc, lightBody: Test.has "body.dark" lightPage.doc } == { before: Just "Switch to dark", beforeDark: False, theme: Dark, darkLabel: Just "Switch to light", darkBody: True, back: Light, lightLabel: Just "Switch to dark", lightBody: False })
}

test "Logout clears the user and goes to log in" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let out <- Test.click Logout
  Task.succeed ({ out: out.answer, user: out.session.user } == { out: Redirected (Auth.Login {}), user: Nothing })
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

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Notes.Index {}), user: Just me, title: Just "Notes" })
}

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Notes.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
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
    </head>
    <body class={if props.theme == Dark then Just "dark" else Nothing}>
      <header>
        <button id="theme" click={App.SwitchTheme}>{Theme.switchLabel props.theme}</button>
        <button id="logout" click={App.Logout}>Log out</button>
      </header>
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
  owner: User.UserId @ref User @owner,
  created: Time @created,
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
  let notes <-
    Db.mine Note
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
    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
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
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let added <- Test.submit NewNote { title: Text.NonEmpty "Groceries", body: "milk" }
  let page <- Test.get (Notes.Index {})
  Task.succeed ({ answer: added.answer, titles: Test.texts "#notes a.title" page.doc } == { answer: Redirected (Notes.Index {}), titles: ["Groceries"] })
}

test "the newest note is listed first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "First", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Second", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "#notes li a.title" page.doc == ["Second", "First"])
}

test "another user's notes are not listed" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: me }
  let _ <- Test.login { user: Just other }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.count "#notes li" page.doc == 0)
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import Layout
import User
import Theme

page "/notes/{id}" { id: Note.NoteId } requires user

type Model = { note: Note }

-- `Db.getMine` finds the note only when it is the signed-in user's: another user's note, like a
-- missing one, is `Query.notFound`, a 404.
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

test "a user cannot see another user's note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "mine", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: id })
  let loaded <- Test.run {
    let m <- load { id: id } { user: other, theme: Light }
    Task.succeed m.note.body
  }
  Task.succeed ({ answer: r.answer, loaded: loaded.value } == { answer: PageMissing, loaded: Err "not found" })
}

test "the owner sees the note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Mine", body: "hello", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Show { id: id })
  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Mine", body: Just "hello" })
}

test "saving changes the title and the body" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Old", body: "old", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Show { id: id })
  let r <- Test.submit EditNote { title: Text.NonEmpty "New", body: "new" }
  let after <- Db.get Note id
  Task.succeed ({ answer: r.answer, title: Maybe.map (\n -> n.title) after, body: Maybe.map (\n -> n.body) after } == { answer: Redirected (Notes.Show { id: id }), title: Just (Text.NonEmpty "New"), body: Just "new" })
}

test "deleting from the note's page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Gone", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Show { id: id })
  let r <- Test.click Delete
  let after <- Db.get Note id
  Task.succeed ({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
}

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

test "flip turns Light to Dark and back" =
  List.map flip [Light, Dark] == [Dark, Light]

test "the button names the other theme" =
  List.map switchLabel [Light, Dark] == ["Switch to dark", "Switch to light"]

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
