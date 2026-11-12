--- file: app/App.alder ---
module App

import Layout
import Theme
import User

session { user: Maybe User.UserId, theme: Theme.Theme = Light }

login = Auth.Login.Login

route SwitchTheme = PUT    "/theme"   { }
route Logout      = DELETE "/session" { }

SwitchTheme.update : SwitchTheme.Params -> Session -> Task Response
SwitchTheme.update p session = {
  let _ <- Session.put (\s -> { s | theme: Theme.toggle s.theme })
  Task.succeed Response.refresh
}

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found" theme={Light}><h1>Not found</h1></Layout.Page>

test "the theme flips" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let before <- Test.get (Notes.Index.Index {})
  let dark <- Test.click SwitchTheme
  let light <- Test.click SwitchTheme
  Task.succeed (
    {
      before: Test.text "#theme" before.doc,
      beforeDark: Test.has "body.dark" before.doc,
      answer: dark.answer,
      darkTheme: dark.session.theme,
      darkLabel: Test.text "#theme" dark.doc,
      darkBody: Test.has "body.dark" dark.doc,
      lightTheme: light.session.theme,
      lightLabel: Test.text "#theme" light.doc,
      lightBody: Test.has "body.dark" light.doc,
    } == {
      before: Just "Switch to dark",
      beforeDark: False,
      answer: Refreshed,
      darkTheme: Dark,
      darkLabel: Just "Switch to light",
      darkBody: True,
      lightTheme: Light,
      lightLabel: Just "Switch to dark",
      lightBody: False,
    }
  )
}

test "the theme flips on the login page too" = {
  let _ <- Test.get (Auth.Login.Login {})
  let r <- Test.click SwitchTheme
  Task.succeed (
    { theme: r.session.theme, label: Test.text "#theme" r.doc }
      == { theme: Dark, label: Just "Switch to light" }
  )
}

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me"), theme: Light })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] }
  )
}

test "logging out from a page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index.Index {})
  let out <- Test.click Logout
  Task.succeed (
    { answer: out.answer, user: out.session.user }
      == { answer: Redirected (Auth.Login.Login {}), user: Nothing }
  )
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
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

form Credentials = { email: Text.Email, password: Text.Secret }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

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
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Notes.Index.Index {}), user: Just me }
  )
}

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme.Theme, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
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
  owner: User.UserId @ref User,
  created: Time @created,
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import Layout
import Note
import User

route Index = GET  "/"      { }              requires user
route Add   = POST "/notes" { } form NewNote requires user

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

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Notes" theme={session.theme}>
    <h1>Notes</h1>
    <form submit={Add}>
      <Errors form />
      <Input field="title" label="Title" />
      <Textarea field="body" label="Body" />
      <button>Add</button>
    </form>
    <ul id="notes">
      {for note in model.notes ->
        <li><a class="title" href={Notes.Show.Show { id: note.id }}>{note.title}</a></li>}
    </ul>
  </Layout.Page>

test "adding a note through the page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let added <- Test.submit (Add {}) { title: Text.NonEmpty "Milk", body: "Buy two" }
  let page <- Test.get (Index {})
  Task.succeed (
    { answer: added.answer, titles: Test.texts "#notes a.title" page.doc }
      == { answer: Redirected (Index {}), titles: ["Milk"] }
  )
}

test "my notes are listed newest first, and no one else's" = {
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
  let _ <- Db.insert Note { title: Text.NonEmpty "First", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "", owner: other }
  let _ <- Db.insert Note { title: Text.NonEmpty "Second", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#notes li a.title" page.doc == ["Second", "First"])
}

test "a visitor is sent to log in" = {
  let r <- Test.open (Index {})
  Task.succeed (r.answer == Redirected (Auth.Login.Login {}))
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Layout
import Note
import Theme
import User

route Show   = GET    "/notes/{id}" { id: Note.NoteId }               requires user
route Save   = PUT    "/notes/{id}" { id: Note.NoteId } form EditNote requires user
route Delete = DELETE "/notes/{id}" { id: Note.NoteId }               requires user

type Model = { note: Note }

form EditNote = { title: Text.NonEmpty, body: Text }

-- The note with that id when it belongs to `user`; anyone else's, like a missing one, is Nothing.
owned : User.UserId -> Note.NoteId -> Query (Maybe Note)
owned user id = {
  let found <- Db.get Note id
  Query.succeed (Maybe.andThen (\n -> if n.owner == user then Just n else Nothing) found)
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- owned session.user params.id
  match found {
    Just note -> Query.succeed { note: note },
    Nothing -> Query.notFound,
  }
}

Save.update : Save.Params -> Session -> Task Response
Save.update p session = {
  let found <- owned session.user p.id
  match found {
    Just note -> {
      let _ <- Db.update Note note.id (\n -> { n | title: p.form.title, body: p.form.body })
      Task.succeed (Response.redirect (Show { id: note.id }))
    },
    Nothing -> Task.notFound,
  }
}

Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let found <- owned session.user p.id
  match found {
    Just note -> {
      let _ <- Db.delete Note note.id
      Task.succeed (Response.redirect (Notes.Index.Index {}))
    },
    Nothing -> Task.notFound,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <form submit={Save { id: model.note.id }}>
      <Errors form />
      <Input field="title" label="Title" value={model.note.title} />
      <Textarea field="body" label="Body" value={model.note.body} />
      <button>Save</button>
    </form>
    <button click={Delete { id: model.note.id }}>Delete</button>
    <a href={Notes.Index.Index {}}>All notes</a>
  </Layout.Page>

test "the owner sees the note" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "Buy two", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  Task.succeed (
    { title: Test.text "#title" page.doc, body: Test.text "#body" page.doc }
      == { title: Just "Milk", body: Just "Buy two" }
  )
}

test "a user cannot see another user's note" = {
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
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "shh", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "saving or deleting another user's note is not found" = {
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
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "shh", owner: me }
  let _ <- Test.login { user: Just other }
  let saved <- Test.run (Save.update { id: id, form: { title: Text.NonEmpty "Mine now", body: "" } } { user: other, theme: Light })
  let deleted <- Test.run (Delete.update { id: id } { user: other, theme: Light })
  let after <- Db.get Note id
  Task.succeed (
    {
      saved: saved.value,
      deleted: deleted.value,
      title: Maybe.map (\n -> n.title) after,
    } == {
      saved: Err "not found",
      deleted: Err "not found",
      title: Just (Text.NonEmpty "Secret"),
    }
  )
}

test "saving changes the title and the body" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.submit (Save { id: id }) { title: Text.NonEmpty "Bread", body: "Rye" }
  Task.succeed (
    {
      answer: r.answer,
      title: Test.text "#title" r.doc,
      body: Test.text "#body" r.doc,
    } == {
      answer: Redirected (Show { id: id }),
      title: Just "Bread",
      body: Just "Rye",
    }
  )
}

test "deleting from the note's page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed (
    { answer: r.answer, note: Maybe.map (\n -> n.title) after }
      == { answer: Redirected (Notes.Index.Index {}), note: Nothing }
  )
}

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

toggle : Theme -> Theme
toggle theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

switchLabel : Theme -> Text
switchLabel theme = match theme {
  Light -> "Switch to dark",
  Dark -> "Switch to light",
}

test "toggle flips the theme" = List.map toggle [Light, Dark] == [Dark, Light]

test "the button says what it switches to" =
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
