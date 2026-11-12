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

toggle : Theme -> Theme
toggle t = match t {
  Light -> Dark,
  Dark -> Light,
}

test "toggle flips the theme" =
  { a: toggle Light, b: toggle Dark } == { a: Dark, b: Light }

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
          <al-stack gap="lg">
            <nav>
              <button id="theme" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
              <button click={App.Logout}>Log out</button>
            </nav>
            {props.children}
          </al-stack>
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

route SwitchTheme = POST "/theme" { }
route Logout = DELETE "/session" { }

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
  let r <- Test.click SwitchTheme
  let back <- Test.click SwitchTheme
  Task.succeed (
    {
      beforeText: Test.text "#theme" before.doc,
      beforeDark: Test.has "body.dark" before.doc,
      theme: r.session.theme,
      text: Test.text "#theme" r.doc,
      dark: Test.has "body.dark" r.doc,
      backTheme: back.session.theme,
      backText: Test.text "#theme" back.doc,
    } == {
      beforeText: Just "Switch to dark",
      beforeDark: False,
      theme: Dark,
      text: Just "Switch to light",
      dark: True,
      backTheme: Light,
      backText: Just "Switch to dark",
    }
  )
}

test "logging out works from a page" = {
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
        <Errors form />
        <Input field="title" label="Title" />
        <Textarea field="body" label="Body" />
        <button>Add</button>
      </form>
    </al-card>
    <ul id="notes">
      {for n in model.notes -> <li><a class="title" href={Notes.Show.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  </Layout.Page>

test "notes are listed newest first, only the user's own" = {
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
  let _ <- Db.insert Note { title: Text.NonEmpty "Second", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "", owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#notes li a.title" page.doc == ["Second", "First"])
}

test "adding a note through the page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let added <- Test.submit (Add {}) { title: Text.NonEmpty "Milk", body: "2 litres" }
  let page <- Test.get (Index {})
  Task.succeed (
    { answer: added.answer, titles: Test.texts "#notes li a.title" page.doc }
      == { answer: Redirected (Index {}), titles: ["Milk"] }
  )
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
  match found {
    Just n -> if n.owner == session.user then {
      let _ <- Db.update Note p.id (\x -> { x | title: p.form.title, body: p.form.body })
      Task.succeed (Response.redirect (Show { id: p.id }))
    } else Task.notFound,
    Nothing -> Task.notFound,
  }
}

Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let found <- Db.get Note p.id
  match found {
    Just n -> if n.owner == session.user then {
      let _ <- Db.delete Note p.id
      Task.succeed (Response.redirect (Notes.Index.Index {}))
    } else Task.notFound,
    Nothing -> Task.notFound,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <al-card>
      <form submit={Save { id: model.note.id }}>
        <Errors form />
        <Input field="title" label="Title" value={model.note.title} />
        <Textarea field="body" label="Body" value={model.note.body} />
        <button>Save</button>
      </form>
    </al-card>
    <nav>
      <a href={Notes.Index.Index {}}>Back</a>
      <button click={Delete { id: model.note.id }}>Delete</button>
    </nav>
  </Layout.Page>

test "another user's note is not found" = {
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
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "hidden", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "the owner sees their note" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Plan", body: "do it", owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.get (Show { id: id })
  Task.succeed (
    { title: Test.text "#title" r.doc, body: Test.text "#body" r.doc }
      == { title: Just "Plan", body: Just "do it" }
  )
}

test "a stranger cannot delete a note" = {
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
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "hidden", owner: me }
  let r <- Test.run (Delete.update { id: id } { user: other, theme: Light })
  let after <- Db.get Note id
  Task.succeed (
    { value: r.value, note: Maybe.map (\n -> n.title) after }
      == { value: Err "not found", note: Just (Text.NonEmpty "Secret") }
  )
}
