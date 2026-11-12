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

--- file: app/App.alder ---
module App

import Layout
import Theme
import User

session { user: Maybe User.UserId, theme: Theme.Theme = Light }

login = Auth.Login.Login

route SwitchTheme = PATCH "/theme" { } requires user

route Logout = DELETE "/session" { }

SwitchTheme.update : SwitchTheme.Params -> Session -> Task Response
SwitchTheme.update p session = {
  let _ <- Session.put (\s -> { s | theme: match s.theme {
    Light -> Dark,
    Dark -> Light,
  } })
  Task.succeed Response.refresh
}

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error" theme={Light} body={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound =
  <Layout.Page title="Not found" theme={Light} body={Light}>
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme.Theme, body: Theme.Theme, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
    </head>
    <body class={if props.body == Dark then "dark" else ""}>
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
  owner: User.UserId @ref User @owner,
  created: Time @created,
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import Layout
import Note
import Theme
import User

route Index = GET "/" { } requires user
route Add   = POST "/notes" { } form NewNote requires user

form NewNote = { title: Text.NonEmpty, body: Text }

type Model = { notes: List Note, theme: Theme.Theme }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let notes <-
    Db.mine Note
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes, theme: session.theme }
}

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
  Task.succeed (Response.redirect (Notes.Index.Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Notes" theme={model.theme} body={model.theme}>
    <h1>Notes</h1>

    <al-button id="theme" variant="outline" click={App.SwitchTheme}>
      {if model.theme == Light then "Switch to dark" else "Switch to light"}
    </al-button>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>

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
  </Layout.Page>

Item : { note: Note } -> Html
Item props =
  <li>
    <a class="title" href={Notes.Show.Show { id: props.note.id }}>{props.note.title}</a>
  </li>

test "a user sees only their own notes" = {
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
  let _ <- Test.login { user: Just me, theme: Light }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "a.title" page.doc == ["Mine"])
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Layout
import Note
import Theme
import User

route Show   = GET "/notes/{id}" { id: Note.NoteId } requires user
route Save   = PUT "/notes/{id}" { id: Note.NoteId } form EditNote requires user
route Delete = DELETE "/notes/{id}" { id: Note.NoteId } requires user

form EditNote = { title: Text.NonEmpty, body: Text }

type Model = { note: Note, theme: Theme.Theme }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let note <- Db.getMine Note params.id
  Query.succeed { note: note, theme: session.theme }
}

Save.update : Save.Params -> Session -> Task Response
Save.update p session = {
  let _ <- Db.updateMine Note p.id (\n -> { n | title: p.form.title, body: p.form.body })
  Task.succeed (Response.redirect (Notes.Show.Show { id: p.id }))
}

Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let _ <- Db.deleteMine Note p.id
  Task.succeed (Response.redirect (Notes.Index.Index {}))
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.note.title} theme={model.theme} body={model.theme}>
    <h1 id="title">{model.note.title}</h1>
    <p id="body">{model.note.body}</p>

    <al-button id="theme" variant="outline" click={App.SwitchTheme}>
      {if model.theme == Light then "Switch to dark" else "Switch to light"}
    </al-button>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>

    <al-card>
      <form submit={Save { id: model.note.id }}>
        <al-input field="title" label="Title" value={model.note.title} />
        <al-input field="body" label="Body" value={model.note.body} />
        <al-button type="submit">Save</al-button>
      </form>
      <al-button variant="destructive" click={Delete { id: model.note.id }}>Delete</al-button>
    </al-card>

    <a href={Notes.Index.Index {}}>Back</a>
  </Layout.Page>

test "a user cannot open another user's note" = {
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
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: other }
  let _ <- Test.login { user: Just me, theme: Light }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Theme
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

type Model = { theme: Theme.Theme }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed { theme: session.theme }

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
  <Layout.Page title="Log in" theme={model.theme} body={model.theme}>
    <h1>Log in</h1>

    <al-button id="theme" variant="outline" click={App.SwitchTheme}>
      {if model.theme == Light then "Switch to dark" else "Switch to light"}
    </al-button>

    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "the theme flips" = {
  let _ <- Test.login { user: Nothing, theme: Light }
  let _ <- Test.get (Login {})
  let r <- Test.click App.SwitchTheme
  let page <- Test.get (Login {})
  Task.succeed (
    { answer: r.answer, theme: r.session.theme, body: Test.attr "class" "body" page.doc }
      == { answer: Refreshed, theme: Dark, body: Just "dark" }
  )
}
