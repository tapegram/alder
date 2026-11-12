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

session { user: Maybe User.UserId }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
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

import Layout
import User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
fake Password {
  verify args = {
    let found <-
      Db.from User
        |> Db.where (User.email.eq args.email)
        |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Notes.Index.Index {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
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
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user } == { answer: Redirected (Notes.Index.Index {}), user: Just me }
  )
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import Layout
import Note
import User

route Index  = GET  "/"           { } requires user
route Add    = POST "/notes"      { } form NewNote requires user
route Delete = DELETE "/notes/{id}" { id: Note.NoteId } requires user

type Model = { notes: List Note }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let notes <-
    Db.mine Note
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes }
}

form NewNote = { title: Text.NonEmpty, body: Text }

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
  Task.succeed (Response.redirect (Notes.Index.Index {}))
}

-- Deleting another user's note, or a missing one, is a 404: `Db.deleteMine` finds only the
-- session user's own notes.
Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let _ <- Db.deleteMine Note p.id
  Task.succeed (Response.redirect (Notes.Index.Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Notes">
    <h1>Notes</h1>

    <al-card>
      <form submit={Add}>
        <al-input field="title" label="Title" />
        <al-input field="body" label="Body" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>

    <ul id="notes">
      {for note in model.notes -> <Item note={note} />}
    </ul>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

Item : { note: Note } -> Html
Item props =
  <li>
    <a class="title" href={Notes.Show.Show { id: props.note.id }}>{props.note.title}</a>
    <al-button variant="ghost" size="sm" click={Delete { id: props.note.id }}>Delete</al-button>
  </li>

test "the owner can delete a note" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note {
    title: Text.NonEmpty "Milk",
    body: "",
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed (
    { answer: r.answer, note: Maybe.map (\n -> n.title) after }
      == { answer: Redirected (Index {}), note: Nothing }
  )
}

test "deleting another user's note is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note {
    title: Text.NonEmpty "Milk",
    body: "",
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed (
    { answer: r.answer, note: Maybe.map (\n -> n.title) after }
      == { answer: PageMissing, note: Just (Text.NonEmpty "Milk") }
  )
}

test "deleting a missing note is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let r <- Test.click (Delete { id: Note.NoteId "missing" })
  Task.succeed (r.answer == PageMissing)
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Layout
import Note

route Show = GET "/notes/{id}" { id: Note.NoteId } requires user

type Model = { note: Note }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.note.title}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <a href={Notes.Index.Index {}}>Back</a>
  </Layout.Page>

test "another user's note is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note {
    title: Text.NonEmpty "Milk",
    body: "",
    owner: me,
  }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
