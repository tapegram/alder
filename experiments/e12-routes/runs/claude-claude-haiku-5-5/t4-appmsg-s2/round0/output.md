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
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>

test "the theme flips between light and dark" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let first <- Test.get (Notes.Index.Index {})
  let dark <- Test.click App.SwitchTheme
  let again <- Test.get (Notes.Index.Index {})
  let light <- Test.click App.SwitchTheme
  Task.succeed (
    {
      start: Test.attr "class" "body" first.doc,
      startButton: Test.text "#theme" first.doc,
      answer: dark.answer,
      dark: Test.attr "class" "body" dark.doc,
      darkButton: Test.text "#theme" dark.doc,
      kept: Test.attr "class" "body" again.doc,
      light: Test.attr "class" "body" light.doc,
      lightButton: Test.text "#theme" light.doc,
    } == {
      start: Nothing,
      startButton: Just "Switch to dark",
      answer: Refreshed,
      dark: Just "dark",
      darkButton: Just "Switch to light",
      kept: Just "dark",
      light: Nothing,
      lightButton: Just "Switch to dark",
    }
  )
}

test "log out from any page goes to log in" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index.Index {})
  let out <- Test.click App.Logout
  Task.succeed (
    { answer: out.answer, user: out.session.user }
      == { answer: Redirected (Auth.Login.Login {}), user: Nothing }
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
      <al-page width="narrow">
        <main>
          <al-stack gap="lg">
            <nav>
              <al-button id="theme" variant="outline" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</al-button>
              <al-button variant="outline" click={App.Logout}>Log out</al-button>
            </nav>
            {props.children}
          </al-stack>
        </main>
      </al-page>
    </body>
  </html>

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
toggle theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "toggle flips the theme both ways" =
  toggle (toggle Light) == Light && toggle Light == Dark

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

-- The note with this id when `user` owns it, otherwise nothing: another user's note is as
-- missing as one that does not exist.
ownedNote : User.UserId -> NoteId -> Query (Maybe Note)
ownedNote user id = {
  let found <- Db.get Note id
  Query.succeed (Maybe.andThen (\n -> if n.owner == user then Just n else Nothing) found)
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import Layout
import Note

route Index = GET "/" { } requires user
route Add = POST "/notes" { } form NewNote requires user

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

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Notes" theme={session.theme}>
    <h1>Notes</h1>
    <al-card>
      <form submit={Add}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-textarea field="body" label="Body" />
        <al-button type="submit">Add note</al-button>
      </form>
    </al-card>
    <ul id="notes">
      {for n in model.notes -> <li><a class="title" href={Notes.Show.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  </Layout.Page>

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
  Task.succeed (Response.redirect (Notes.Index.Index {}))
}

test "a user's list holds only their own notes" = {
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
  let _ <- Db.insert Note { title: Text.NonEmpty "Mine", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "", owner: other }
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
  let added <- Test.submit (Add {}) { title: Text.NonEmpty "Milk", body: "" }
  let page <- Test.get (Index {})
  Task.succeed (
    {
      answer: added.answer,
      titles: Test.texts "#notes a.title" page.doc,
    } == {
      answer: Redirected (Index {}),
      titles: ["Milk"],
    }
  )
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Layout
import Note
import Theme

route Show = GET "/notes/{id}" { id: Note.NoteId } requires user
route Save = PUT "/notes/{id}" { id: Note.NoteId } form EditNote requires user
route Delete = DELETE "/notes/{id}" { id: Note.NoteId } requires user

type Model = { note: Note }

form EditNote = { title: Text.NonEmpty, body: Text }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Note.ownedNote session.user params.id
  match found {
    Just note -> Query.succeed { note: note },
    Nothing -> Query.notFound,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1 id="title">{model.note.title}</h1>
    <al-card>
      <form submit={Save { id: model.note.id }}>
        <Errors form />
        <al-input field="title" label="Title" value={model.note.title} />
        <al-textarea field="body" label="Body" value={model.note.body} />
        <al-button type="submit">Save</al-button>
      </form>
      <div slot="actions">
        <al-button variant="destructive" click={Delete { id: model.note.id }}>Delete</al-button>
      </div>
    </al-card>
    <p id="body">{model.note.body}</p>
  </Layout.Page>

Save.update : Save.Params -> Session -> Task Response
Save.update p session = {
  let found <- Note.ownedNote session.user p.id
  match found {
    Just n -> {
      let _ <- Db.update Note n.id (\x -> { x | title: p.form.title, body: p.form.body })
      Task.succeed (Response.redirect (Notes.Show.Show { id: n.id }))
    },
    Nothing -> Task.notFound,
  }
}

Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let found <- Note.ownedNote session.user p.id
  match found {
    Just n -> {
      let _ <- Db.delete Note n.id
      Task.succeed (Response.redirect (Notes.Index.Index {}))
    },
    Nothing -> Task.notFound,
  }
}

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
  let id <- Db.insert Note {
    title: Text.NonEmpty "Secret",
    body: "",
    owner: other,
  }
  let _ <- Test.login { user: Just me }
  let shown <- Test.open (Show { id: id })
  let saved <- Test.run (Save.update { id: id, form: { title: Text.NonEmpty "Hacked", body: "" } } { user: me, theme: Light })
  let deleted <- Test.run (Delete.update { id: id } { user: me, theme: Light })
  let after <- Db.get Note id
  Task.succeed (
    {
      shown: shown.answer,
      saved: saved.value,
      deleted: deleted.value,
      title: Maybe.map (\n -> n.title) after,
    } == {
      shown: PageMissing,
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
  let id <- Db.insert Note {
    title: Text.NonEmpty "Milk",
    body: "",
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.submit (Save { id: id }) { title: Text.NonEmpty "Bread", body: "2 loaves" }
  let after <- Db.get Note id
  Task.succeed (
    {
      answer: r.answer,
      title: Maybe.map (\n -> n.title) after,
      body: Maybe.map (\n -> n.body) after,
    } == {
      answer: Redirected (Show { id: id }),
      title: Just (Text.NonEmpty "Bread"),
      body: Just "2 loaves",
    }
  )
}

test "deleting from the note's page" = {
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
  let _ <- Test.get (Show { id: id })
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Note id
  Task.succeed (
    {
      answer: r.answer,
      note: Maybe.map (\n -> n.title) after,
    } == {
      answer: Redirected (Notes.Index.Index {}),
      note: Nothing,
    }
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

route Login = GET "/login" { }
route Submit = POST "/session" { } form Credentials

type Model = { }

form Credentials = { email: Text.Email, password: Text.Secret }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

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

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit (Submit {}) { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Notes.Index.Index {}), user: Just me }
  )
}

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Login {})
  let r <- Test.submit (Submit {}) { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}
