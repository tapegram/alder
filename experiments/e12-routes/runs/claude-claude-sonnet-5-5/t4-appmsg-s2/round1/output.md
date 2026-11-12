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
  let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
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
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>

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
      beforeDark: Test.has "body.dark" before.doc,
      beforeLabel: Test.text "#theme" before.doc,
      theme: r.session.theme,
      dark: Test.has "body.dark" r.doc,
      label: Test.text "#theme" r.doc,
      backTheme: back.session.theme,
      backDark: Test.has "body.dark" back.doc,
    } == {
      beforeDark: False,
      beforeLabel: Just "Switch to dark",
      theme: Dark,
      dark: True,
      label: Just "Switch to light",
      backTheme: Light,
      backDark: False,
    }
  )
}

test "logging out works from any page" = {
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
    </head>
    <body class={if props.theme == Dark then Just "dark" else Nothing}>
      <header>
        <button id="theme" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
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
    Db.mine Note
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes }
}

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
  Task.succeed (Response.redirect (Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
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
      {for n in model.notes ->
        <li><a class="title" href={Notes.Show.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  </Layout.Page>

test "the list is newest first and only mine" = {
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
  Task.succeed (Test.texts "#notes a.title" page.doc == ["Second", "First"])
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
    { answer: added.answer, titles: Test.texts "#notes a.title" page.doc }
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
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}

Save.update : Save.Params -> Session -> Task Response
Save.update p session = {
  let _ <- Db.updateMine Note p.id (\n -> { n | title: p.form.title, body: p.form.body })
  Task.succeed (Response.redirect (Show { id: p.id }))
}

Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let _ <- Db.deleteMine Note p.id
  Task.succeed (Response.redirect (Notes.Index.Index {}))
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>
    <form submit={Save { id: model.note.id }}>
      <Errors form />
      <Input field="title" label="Title" value={model.note.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body" value={model.note.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>
    <button click={Delete { id: model.note.id }}>Delete</button>
    <a href={Notes.Index.Index {}}>Back</a>
  </Layout.Page>

test "another user's note is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "hidden", owner: me }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "a stranger cannot save or delete my note" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "hidden", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let saved <- Test.submit (Save { id: id }) { title: Text.NonEmpty "Hacked", body: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let deleted <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed (
    {
      saved: saved.answer,
      deleted: deleted.answer,
      note: Maybe.map (\n -> { title: n.title, body: n.body }) after,
    } == {
      saved: PageMissing,
      deleted: PageMissing,
      note: Just { title: Text.NonEmpty "Secret", body: "hidden" },
    }
  )
}

test "the owner edits and deletes a note" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Note { title: Text.NonEmpty "Old", body: "old body", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let saved <- Test.submit (Save { id: id }) { title: Text.NonEmpty "New", body: "new body" }
  let page <- Test.get (Show { id: id })
  let deleted <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed (
    {
      saved: saved.answer,
      title: Test.text "#title" page.doc,
      body: Test.text "#body" page.doc,
      deleted: deleted.answer,
      gone: Maybe.map (\n -> n.title) after,
    } == {
      saved: Redirected (Show { id: id }),
      title: Just "New",
      body: Just "new body",
      deleted: Redirected (Notes.Index.Index {}),
      gone: Nothing,
    }
  )
}

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip t = match t {
  Light -> Dark,
  Dark -> Light,
}

test "flip swaps the theme" = flip Light == Dark && flip Dark == Light

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
