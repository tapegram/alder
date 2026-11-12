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

--- file: app/App.alder ---
module App

import Layout
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

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import User

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

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
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Notes.Index.Index {}), user: Just me }
  )
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import Layout
import Note
import User

route Index  = GET    "/"            { }                      requires user
route Add    = POST   "/notes"       { } form NewNote         requires user
route Delete = DELETE "/notes/{id}"  { id: Note.NoteId }      requires user

type Model = { notes: List Note }

form NewNote = { title: Text.NonEmpty, body: Text }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let notes <-
    Db.from Note
      |> Db.where (Note.owner.eq session.user)
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes }
}

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
  Task.succeed (Response.redirect (Index {}))
}

Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let found <-
    Db.from Note
      |> Db.where (Note.id.eq p.id)
      |> Db.where (Note.owner.eq session.user)
      |> Db.first
  match found {
    Just n -> {
      let _ <- Db.delete Note n.id
      Task.succeed (Response.redirect (Index {}))
    },
    Nothing -> Task.notFound,
  }
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
      {for n in model.notes ->
        <li>
          <a class="title" href={Notes.Show.Show { id: n.id }}>{n.title}</a>
          <button click={Delete { id: n.id }}>Delete</button>
        </li>}
    </ul>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "only my notes are listed" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let _ <- Db.insert Note { title: Text.NonEmpty "Mine", body: "a", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "b", owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#notes a.title" page.doc == ["Mine"])
}

test "adding a note through the page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let added <- Test.submit (Add {}) { title: Text.NonEmpty "Hello", body: "World" }
  let page <- Test.get (Index {})
  Task.succeed (
    { answer: added.answer, titles: Test.texts "#notes a.title" page.doc }
      == { answer: Redirected (Index {}), titles: ["Hello"] }
  )
}

test "the owner can delete a note" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Mine", body: "a", owner: me }
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
  let id <- Db.insert Note { title: Text.NonEmpty "Mine", body: "a", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed (
    { answer: r.answer, note: Maybe.map (\n -> n.title) after }
      == { answer: PageMissing, note: Just (Text.NonEmpty "Mine") }
  )
}

test "deleting a missing note is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (Delete.update { id: Note.NoteId "nope" } { user: me })
  Task.succeed (r.value == Err "not found")
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Layout
import Note
import User

route Show = GET "/notes/{id}" { id: Note.NoteId } requires user

type Model = { note: Note }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <-
    Db.from Note
      |> Db.where (Note.id.eq params.id)
      |> Db.where (Note.owner.eq session.user)
      |> Db.first
  match found {
    Just n -> Query.succeed { note: n },
    Nothing -> Query.notFound,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.note.title}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <al-button variant="ghost" href={Notes.Index.Index {}}>Back</al-button>
  </Layout.Page>

test "the owner sees the note" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Mine", body: "text", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  Task.succeed (
    { title: Test.text "#title" page.doc, body: Test.text "#body" page.doc }
      == { title: Just "Mine", body: Just "text" }
  )
}

test "another user's note is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Mine", body: "text", owner: me }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
