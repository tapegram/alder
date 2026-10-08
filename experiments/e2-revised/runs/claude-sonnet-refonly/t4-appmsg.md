--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  password: Text.Secret,
}

--- file: app/Note.alder ---
module Note

import User

type NoteId = brand Text

table Note {
  id: NoteId @id,
  title: Text.NonEmpty,
  body: Text,
  owner: UserId @ref User,
  created: Time @created,
}

form NoteForm = {
  title: Text.NonEmpty,
  body: Text = "",
}

--- file: app/App.alder ---
module App

import User
import Login
import Notes.Index

type Theme = Light | Dark

session { user: Maybe UserId, theme: Theme = Light }

login = Login

error : Error -> Html Never
error err = <h1>Something went wrong</h1>

notFound : Html Never
notFound = <h1>Not found</h1>

msg ToggleTheme = "/theme" { }
msg Logout = "/logout" { }

update : Msg -> Session -> Task Nav
update msg session = match msg {
  ToggleTheme p -> {
    let _ <- Session.put (\s -> { s | theme: if s.theme == Light then Dark else Light })
    Task.succeed (Redirect (Notes.Index {}))
  },
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Login {}))
  },
}

--- file: app/Layout.alder ---
module Layout

import App

Shell : { title: Text, theme: App.Theme, children: List (Html Msg) } -> Html Msg
Shell props =
  <html>
    <head><title>{props.title}</title></head>
    <body class={if props.theme == Dark then Just "dark" else Nothing}>
      <nav>
        <button click={App.ToggleTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
        <button click={App.Logout}>Log out</button>
      </nav>
      {props.children}
    </body>
  </html>

--- file: app/Login.alder ---
module Login

import App
import Layout
import User

page "/login" { }

type Model = { theme: App.Theme }

form LoginForm = {
  email: Text.Email,
  password: Text.Secret,
}

msg Attempt = "/login" form LoginForm

load : Params -> Session -> Query Model
load params session = Query.succeed { theme: session.theme }

view : Model -> Html Msg
view model =
  <Layout.Shell title="Log in" theme={model.theme}>
    <h1>Log in</h1>
    <form submit={Attempt}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Shell>

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Attempt p -> {
    let found <- Db.from User |> Db.where (User.email.eq p.form.email) |> Db.first
    match found {
      Just u -> if u.password == p.form.password then {
        let _ <- Session.put (\s -> { s | user: Just u.id })
        Task.succeed (Redirect (Notes.Index {}))
      } else Task.succeed (Reject "Wrong email or password"),
      Nothing -> Task.succeed (Reject "Wrong email or password"),
    }
  },
}

--- file: app/Notes/Index.alder ---
module Notes.Index

import App
import Layout
import Note
import Notes.Show

page "/" { } requires user

type Model = { theme: App.Theme, notes: List Note }

load : Params -> Session -> Query Model
load params session = {
  let notes <- Db.from Note
    |> Db.where (Note.owner.eq session.user)
    |> Db.orderBy Note.created.desc
    |> Db.all
  Query.succeed { theme: session.theme, notes: notes }
}

msg Create = "/notes" form NoteForm

view : Model -> Html Msg
view model =
  <Layout.Shell title="Notes" theme={model.theme}>
    <h1>Notes</h1>
    {if List.isEmpty model.notes then <p>No notes yet.</p> else
      <ul id="notes">
        {for n in model.notes -> <li><a href={Notes.Show { id: n.id }}>{n.title}</a></li>}
      </ul>}
    <form submit={Create}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body" />
      <button>Add note</button>
    </form>
  </Layout.Shell>

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
    Task.succeed (Redirect (Notes.Show { id: id }))
  },
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import App
import Layout
import Note
import Notes.Index

page "/notes/{id}" { id: NoteId } requires user

type Model = { theme: App.Theme, note: Note }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Note params.id
  match found {
    Just n -> if n.owner == session.user then Query.succeed { theme: session.theme, note: n } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

msg Save = "/notes/{id}" { id: NoteId } form NoteForm
msg Delete = "/notes/{id}/delete" { id: NoteId }

view : Model -> Html Msg
view model =
  <Layout.Shell title={model.note.title} theme={model.theme}>
    <p><a href={Notes.Index {}}>Back to notes</a></p>
    <h1>{model.note.title}</h1>
    <p>{model.note.body}</p>
    <p>{Time.format "Jan 2, 2006" model.note.created}</p>
    <form submit={Save { id: model.note.id }}>
      <Errors form />
      <Input field="title" label="Title" value={model.note.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body" value={model.note.body} />
      <button>Save</button>
    </form>
    <button click={Delete { id: model.note.id }}>Delete</button>
  </Layout.Shell>

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Note p.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed Rerender
  },
  Delete p -> {
    let _ <- Db.delete Note p.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}
