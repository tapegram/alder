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
    let _ <- Session.put (\s -> { s | theme: Theme.toggle s.theme })
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

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Dark })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}

test "the theme switch flips the theme and the body class" = {
  let _ <- Test.get (Auth.Login {})
  let _ <- Test.click App.SwitchTheme
  let dark <- Test.get (Auth.Login {})
  let _ <- Test.click App.SwitchTheme
  let light <- Test.get (Auth.Login {})
  Task.succeed ({ theme: dark.session.theme, darkClass: Test.has "body.dark" dark.doc, lightClass: Test.has "body.dark" light.doc } == { theme: Dark, darkClass: True, lightClass: False })
}

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    </head>
    <body class={if props.theme == Dark then Just "dark" else Nothing}>
      <al-page width="narrow">
        <main>{props.children}</main>
        <nav>
          <al-button variant="outline" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</al-button>
          <al-button variant="ghost" click={App.Logout}>Log out</al-button>
        </nav>
      </al-page>
    </body>
  </html>

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

toggle : Theme -> Theme
toggle t = match t {
  Light -> Dark,
  Dark -> Light,
}

test "toggle flips the theme both ways" =
  (toggle Light == Dark) && (toggle Dark == Light)

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

-- `owner` is the table's `@owner`: the scoped operations read and write only the signed-in user's notes.
table Note {
  id: NoteId @id,
  title: Text.NonEmpty,
  body: Text,
  owner: User.UserId @ref User @owner,
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

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has a row.
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
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form opens the notes" = {
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

--- file: app/Notes/Index.alder ---
module Notes.Index

import Note
import Layout

page "/" { } requires user

type Model = {
  notes: List Note,
}

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

msg Create form NewNote

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Notes" theme={session.theme}>
    <h1>Notes</h1>

    <al-card>
      <form submit={Create}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-textarea field="body" label="Body" />
        <al-button type="submit">Add note</al-button>
      </form>
    </al-card>

    <ul id="notes">
      {for note in model.notes -> <li><a href={Notes.Show { id: note.id }}>{note.title}</a></li>}
    </ul>

    <p id="count">{Text.fromInt (List.length model.notes)} notes</p>
  </Layout.Page>

test "the list shows only my notes, newest first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "Old", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Newer", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Not mine", body: "", owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "li a" page.doc == ["Newer", "Old"])
}

test "a new note is added to the list" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.submit NewNote { title: Text.NonEmpty "Milk", body: "" }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "li a" page.doc == ["Milk"])
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import Layout

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
    <al-card>
      <form submit={Save}>
        <Errors form />
        <al-input field="title" label="Title" value={model.note.title} />
        <al-textarea field="body" label="Body" value={model.note.body} />
        <al-button type="submit">Save</al-button>
      </form>
      <div slot="actions">
        <al-button variant="ghost" href={Notes.Index {}}>Back</al-button>
        <al-button variant="destructive" click={Delete}>Delete</al-button>
      </div>
    </al-card>
  </Layout.Page>

test "another user's note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "saving changes the title and body" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Show { id: id })
  let r <- Test.submit EditNote { title: Text.NonEmpty "Bread", body: "2 loaves" }
  let after <- Db.get Note id
  Task.succeed ({ answer: r.answer, title: Maybe.map (\n -> n.title) after, body: Maybe.map (\n -> n.body) after } == { answer: Redirected (Notes.Show { id: id }), title: Just (Text.NonEmpty "Bread"), body: Just "2 loaves" })
}

test "deleting from the note's page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Show { id: id })
  let r <- Test.click Delete
  let after <- Db.get Note id
  Task.succeed ({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
}
