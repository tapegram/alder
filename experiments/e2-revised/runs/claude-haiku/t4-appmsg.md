--- file: app/App.alder ---
module App

import User
import Layout
import Auth.Login

type Theme = Light | Dark

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg SwitchTheme = "/switch-theme" { }

update : Msg -> Session -> Task Nav
update msg session = match msg {
  SwitchTheme p -> {
    let newTheme = match session.theme {
      Light -> Dark,
      Dark -> Light,
    }
    let _ <- Session.put (\s -> { s | theme: newTheme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>

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
  body: Text = "",
  owner: User.UserId @ref User,
  created: Time @created,
}

--- file: app/Layout.alder ---
module Layout

import App

Page : { title: Text, theme: App.Theme, children: List (Html App.Msg) } -> Html App.Msg
Page props =
  let bodyClass = match props.theme {
    Light -> "",
    Dark -> "dark",
  }
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body class={bodyClass}>
      <header>
        <nav>
          <button click={App.SwitchTheme}>{match props.theme {
            Light -> "Switch to dark",
            Dark -> "Switch to light",
          }}</button>
          <button click={App.Logout}>Log out</button>
        </nav>
      </header>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout
import App
import Notes.Index

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit = "/login" form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Notes.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Log in" theme={Light}>
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: app/Notes/Index.alder ---
module Notes.Index

import Note
import User
import Layout
import Notes.Show
import App

page "/" { } requires user

type Model = {
  notes: List Note,
}

load : Params -> Session -> Query Model
load params session = {
  let notes <-
    Db.from Note
      |> Db.where (Note.owner.eq session.user)
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes }
}

form NewNote = {
  title: Text.NonEmpty,
  body: Maybe Text,
}

msg Submit = "/" form NewNote

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Note { title: p.form.title, body: p.form.body ?? "", owner: session.user }
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Notes" theme={Light}>
    <h1>Notes</h1>

    <form submit={Submit}>
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Create</button>
    </form>

    <ul id="notes">
      {for note in model.notes -> <Item note={note} />}
    </ul>
  </Layout.Page>

Item : { note: Note } -> Html Msg
Item props =
  <li>
    <a href={Notes.Show { id: props.note.id }}>{props.note.title}</a>
  </li>

--- file: app/Notes/Show.alder ---
module Notes.Show

import Note
import Layout
import Notes.Index
import App

page "/notes/{id}" { id: Note.NoteId } requires user

type Model = { note: Note }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Note params.id
  match found {
    Just note -> if note.owner == session.user then Query.succeed { note: note } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form EditNote = {
  title: Text.NonEmpty,
  body: Maybe Text,
}

msg Save = "/notes/{id}/save" { id: Note.NoteId } form EditNote
msg Delete = "/notes/{id}/delete" { id: Note.NoteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Note p.id (\n -> { n | title: p.form.title, body: p.form.body ?? "" })
    Task.succeed (Redirect (Notes.Index {}))
  },
  Delete p -> {
    let _ <- Db.delete Note p.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.note.title} theme={Light}>
    <h1>{model.note.title}</h1>
    <form submit={Save { id: model.note.id }}>
      <Input field="title" label="Title" value={model.note.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body" value={if model.note.body == "" then Nothing else Just model.note.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>
    <button click={Delete { id: model.note.id }}>Delete</button>
    <a href={Notes.Index {}}>Back</a>
  </Layout.Page>
