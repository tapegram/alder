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

test "logging out through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let out <- Test.click Logout
  Task.succeed ({ out: out.answer, user: out.session.user } == { out: Redirected (Auth.Login {}), user: Nothing })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>{props.children}</main>
      </al-page>
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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
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

--- file: app/Notes/Index.alder ---
module Notes.Index

import Note
import User
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
  -- The id comes from the request: a note that is missing or someone else's is a 404.
  Delete p -> {
    let found <- Db.get Note p.id
    match found {
      Just n ->
        if n.owner == session.user then {
          let _ <- Db.delete Note n.id
          Task.succeed (Redirect (Notes.Index {}))
        } else Task.notFound,
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Notes">
    <h1>Notes</h1>

    <al-card>
      <form submit={Add}>
        <al-input field="title" label="Title" />
        <al-textarea field="body" label="Body" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>

    <ul id="notes">
      {for note in model.notes ->
        <li>
          <a class="title" href={Notes.Show { id: note.id }}>{note.title}</a>
          <al-button variant="ghost" size="sm" click={Delete { id: note.id }}>Delete</al-button>
        </li>}
    </ul>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "deleting another user's note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "secret", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: me })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Err "not found", note: Just (Text.NonEmpty "Theirs") })
}

test "deleting a missing note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
  Task.succeed (r.value == Err "not found")
}

test "the owner can delete a note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Mine", body: "hello", owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: me })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Ok (Redirect (Notes.Index {})), note: Nothing })
}

test "deleting through the page by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Mine", body: "hello", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let _ <- Test.login { user: Just other }
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed ({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: PageMissing, note: Just (Text.NonEmpty "Mine") })
}

test "the owner deletes through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Mine", body: "hello", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed ({ answer: r.answer, note: Maybe.map (\n -> n.title) after, items: Test.count "#notes li" r.doc } == { answer: Redirected (Notes.Index {}), note: Nothing, items: 0 })
}

test "adding a note through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let added <- Test.submit NewNote { title: Text.NonEmpty "Milk", body: "Buy milk" }
  let page <- Test.get (Notes.Index {})
  Task.succeed ({ answer: added.answer, titles: Test.texts "#notes a.title" page.doc } == { answer: Redirected (Notes.Index {}), titles: ["Milk"] })
}

test "only my notes are listed" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "First", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Second", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "", owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "#notes a.title" page.doc == ["Second", "First"])
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import User
import Layout

page "/notes/{id}" { id: Note.NoteId } requires user

type Model = { note: Note }

-- A note that is missing or someone else's is a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Note params.id
  match found {
    Just n -> if n.owner == session.user then Query.succeed { note: n } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.note.title}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <a href={Notes.Index {}}>Back</a>
  </Layout.Page>

test "the owner sees the note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "Buy milk", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Show { id: id })
  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "Buy milk" })
}

test "another user's note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "Buy milk", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "a missing note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Notes.Show { id: Note.NoteId "missing" })
  Task.succeed (r.answer == PageMissing)
}

test "a visitor is sent to log in from a note" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "Buy milk", owner: me }
  let visitor <- Test.open (Notes.Show { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}
