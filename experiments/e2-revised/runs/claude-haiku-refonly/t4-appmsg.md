--- file: app/App.alder ---
module App

import User

type Theme = Light | Dark

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

error : Error -> Html Never
error err = <h1>Something went wrong</h1>

notFound : Html Never
notFound = <h1>Not found</h1>

msg Logout = "/logout" { }

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  name: Text.NonEmpty,
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
  updated: Time @updated,
}

--- file: app/Layout.alder ---
module Layout

import App

Layout : { title: Text, session: Session, children: List (Html Msg) } -> Html Msg
Layout props =
  let themeClass = if props.session.theme == Light then "" else "dark"
  <html>
    <head><title>{props.title}</title></head>
    <body class={themeClass}>
      {props.children}
    </body>
  </html>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User

page "/login" { }

type Model = Unit

load : Params -> Session -> Query Model
load params session = Query.succeed ()

view : Model -> Html Msg
view model =
  <html>
    <head><title>Log in</title></head>
    <body>
      <h1>Log in</h1>
      <form submit={DoLogin}>
        <Errors form />
        <Input field="username" label="Username" />
        <Errors field="username" />
        <button>Log in</button>
      </form>
    </body>
  </html>

form LoginForm = {
  username: Text.NonEmpty,
}

msg DoLogin = "/do-login" { } form LoginForm

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  DoLogin p -> {
    let user <- Db.from User |> Db.where (User.name.eq p.form.username) |> Db.first
    match user {
      Just u -> {
        let _ <- Session.put (\s -> { s | user: Just u.id })
        Task.succeed (Redirect (Index {}))
      },
      Nothing -> Task.succeed (Reject "User not found"),
    }
  },
}

--- file: app/Index.alder ---
module Index

import Layout
import App
import Note
import User

page "/" { } requires user

type Model = { notes: List Note, session: Session }

load : Params -> Session -> Query Model
load params session = {
  let notes <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes, session: session }
}

view : Model -> Html Msg
view model =
  <Layout title="Notes" session={model.session}>
    <header>
      <h1>My Notes</h1>
      <button click={App.Logout {}}>Log out</button>
      {if model.session.theme == Light then
        <button click={SwitchTheme { to: Dark }}>Switch to dark</button>
      else
        <button click={SwitchTheme { to: Light }}>Switch to light</button>
      }
    </header>
    <form submit={CreateNote}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <button>Create Note</button>
    </form>
    <ul>
      {for note in model.notes -> <li><a href={Notes.Show { id: note.id }}>{note.title}</a></li>}
    </ul>
  </Layout>

form NewNote = {
  title: Text.NonEmpty,
}

msg CreateNote = "/notes" { } form NewNote
msg SwitchTheme = "/switch-theme" { to: App.Theme }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  CreateNote p -> {
    let id <- Db.insert Note { title: p.form.title, body: "", owner: session.user }
    Task.succeed (Redirect (Notes.Show { id: id }))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: p.to })
    Task.succeed Rerender
  },
}

--- file: app/Notes/Show.alder ---
module Notes.Show

import Layout
import App
import Note
import User

page "/notes/{id}" { id: Note.NoteId } requires user

type Model = { note: Note, session: Session }

load : Params -> Session -> Query Model
load params session = {
  let note <- Db.get Note params.id
  match note {
    Just n -> {
      if n.owner == session.user then
        Query.succeed { note: n, session: session }
      else
        Query.notFound
    },
    Nothing -> Query.notFound,
  }
}

view : Model -> Html Msg
view model =
  <Layout title={model.note.title} session={model.session}>
    <header>
      <h1>{model.note.title}</h1>
      <button click={App.Logout {}}>Log out</button>
      {if model.session.theme == Light then
        <button click={SwitchTheme { to: Dark }}>Switch to dark</button>
      else
        <button click={SwitchTheme { to: Light }}>Switch to light</button>
      }
    </header>
    <form submit={EditNote}>
      <Errors form />
      <Input field="title" label="Title" value={model.note.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body" value={model.note.body} />
      <button>Save</button>
    </form>
    <button click={DeleteNote {}}>Delete</button>
  </Layout>

form EditNoteForm = {
  title: Text.NonEmpty,
  body: Text = "",
}

msg EditNote = "/notes/{id}" { id: Note.NoteId } form EditNoteForm
msg DeleteNote = "/notes/{id}/delete" { id: Note.NoteId }
msg SwitchTheme = "/notes/{id}/switch-theme" { id: Note.NoteId, to: App.Theme }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  EditNote p -> {
    let _ <- Db.update Note p.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed Rerender
  },
  DeleteNote p -> {
    let _ <- Db.delete Note p.id
    Task.succeed (Redirect (Index {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: p.to })
    Task.succeed Rerender
  },
}
