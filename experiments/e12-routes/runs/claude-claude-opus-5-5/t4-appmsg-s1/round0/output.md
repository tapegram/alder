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
      <al-page width="narrow">
        <main>
          <nav>
            <button id="theme" click={App.SwitchTheme}>
              {if props.theme == Dark then "Switch to light" else "Switch to dark"}
            </button>
            <button id="logout" click={App.Logout}>Log out</button>
          </nav>
          <al-stack gap="lg">{props.children}</al-stack>
        </main>
      </al-page>
    </body>
  </html>

--- file: app/App.alder ---
module App

import Layout
import Theme
import User

session { user: Maybe User.UserId, theme: Theme.Theme = Light }

login = Auth.Login.Login

route SwitchTheme = POST   "/theme"   { }
route Logout      = DELETE "/session" { }

SwitchTheme.update : SwitchTheme.Params -> Session -> Task Response
SwitchTheme.update p session = {
  let next = if session.theme == Dark then Light else Dark
  let _ <- Session.put (\s -> { s | theme: next })
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
  let first <- Test.get (Notes.Index.Index {})
  let dark <- Test.click SwitchTheme
  let light <- Test.click SwitchTheme
  Task.succeed (
    {
      before: Test.text "#theme" first.doc,
      beforeDark: Test.has "body.dark" first.doc,
      darkTheme: dark.session.theme,
      darkButton: Test.text "#theme" dark.doc,
      darkBody: Test.has "body.dark" dark.doc,
      lightTheme: light.session.theme,
      lightButton: Test.text "#theme" light.doc,
      lightBody: Test.has "body.dark" light.doc,
    } == {
      before: Just "Switch to dark",
      beforeDark: False,
      darkTheme: Dark,
      darkButton: Just "Switch to light",
      darkBody: True,
      lightTheme: Light,
      lightButton: Just "Switch to dark",
      lightBody: False,
    }
  )
}

test "SwitchTheme answers refresh" = {
  let r <- Test.run (SwitchTheme.update {} { user: Nothing, theme: Light })
  Task.succeed (
    { value: r.value, trace: r.trace } == { value: Ok Response.refresh, trace: ["Session.put"] }
  )
}

test "Logout clears the user and goes to log in" = {
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
    {
      answer: r.answer,
      user: r.session.user,
      title: Test.text "h1" r.doc,
    } == {
      answer: Redirected (Notes.Index.Index {}),
      user: Just me,
      title: Just "Notes",
    }
  )
}

test "the theme switches on the login page" = {
  let _ <- Test.get (Login {})
  let r <- Test.click App.SwitchTheme
  Task.succeed (
    { theme: r.session.theme, button: Test.text "#theme" r.doc }
      == { theme: Dark, button: Just "Switch to light" }
  )
}

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Notes.Index.Index {})
  Task.succeed (visitor.answer == Redirected (Login {}))
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
  Task.succeed (Response.redirect (Notes.Index.Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Notes" theme={session.theme}>
    <h1>Notes</h1>
    <al-card>
      <form submit={Add}>
        <al-input field="title" label="Title" />
        <Textarea field="body" label="Body" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>
    <ul id="notes">
      {for n in model.notes ->
        <li><a class="title" href={Notes.Show.Show { id: n.id }}>{n.title}</a></li>}
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
  let added <- Test.submit (Add {}) { title: Text.NonEmpty "Groceries", body: "milk" }
  let page <- Test.get (Index {})
  Task.succeed (
    { answer: added.answer, titles: Test.texts "#notes li a.title" page.doc }
      == { answer: Redirected (Index {}), titles: ["Groceries"] }
  )
}

test "notes are newest first and only mine" = {
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

--- file: app/Notes/Show.alder ---
module Notes.Show

import Layout
import Note
import User

route Show   = GET    "/notes/{id}" { id: Note.NoteId }               requires user
route Save   = PUT    "/notes/{id}" { id: Note.NoteId } form EditNote requires user
route Delete = DELETE "/notes/{id}" { id: Note.NoteId }               requires user

type Model = { note: Note }

form EditNote = { title: Text.NonEmpty, body: Text }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Note params.id
  match found {
    Just n -> if n.owner == session.user then Query.succeed { note: n } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

Save.update : Save.Params -> Session -> Task Response
Save.update p session = {
  let found <- Db.get Note p.id
  let mine = Maybe.map (\n -> n.owner == session.user) found == Just True
  if mine then {
    let _ <- Db.update Note p.id (\x -> { x | title: p.form.title, body: p.form.body })
    Task.succeed (Response.redirect (Notes.Show.Show { id: p.id }))
  } else {
    Task.notFound
  }
}

Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let found <- Db.get Note p.id
  let mine = Maybe.map (\n -> n.owner == session.user) found == Just True
  if mine then {
    let _ <- Db.delete Note p.id
    Task.succeed (Response.redirect (Notes.Index.Index {}))
  } else {
    Task.notFound
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <al-card>
      <form submit={Save { id: model.note.id }}>
        <al-input field="title" label="Title" value={model.note.title} />
        <Textarea field="body" label="Body" value={model.note.body} />
        <al-button type="submit">Save</al-button>
      </form>
      <div slot="actions">
        <al-button variant="ghost" href={Notes.Index.Index {}}>Back</al-button>
        <al-button variant="destructive" click={Delete { id: model.note.id }}>Delete</al-button>
      </div>
    </al-card>
  </Layout.Page>

test "the owner sees the note" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Plans", body: "Go north", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  Task.succeed (
    { title: Test.text "#title" page.doc, body: Test.text "#body" page.doc }
      == { title: Just "Plans", body: Just "Go north" }
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
  let show <- Test.open (Show { id: id })
  Task.succeed (show.answer == PageMissing)
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
  let saved <- Test.run (
    Save.update
      { id: id, form: { title: Text.NonEmpty "Mine now", body: "" } }
      { user: other, theme: Light }
  )
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

test "saving changes the title and body" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Plans", body: "Go north", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.submit (Save { id: id }) { title: Text.NonEmpty "New plans", body: "Go south" }
  Task.succeed (
    {
      answer: r.answer,
      title: Test.text "#title" r.doc,
      body: Test.text "#body" r.doc,
    } == {
      answer: Redirected (Show { id: id }),
      title: Just "New plans",
      body: Just "Go south",
    }
  )
}

test "deleting from the note's page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Plans", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed (
    { answer: r.answer, gone: after == Nothing }
      == { answer: Redirected (Notes.Index.Index {}), gone: True }
  )
}
