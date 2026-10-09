--- file: app/App.alder ---
```alder
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
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
```

--- file: app/Auth/Login.alder ---
```alder
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
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

msg Submit form Credentials

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

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" theme={session.theme}>
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
```

--- file: app/Layout.alder ---
```alder
module Layout

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body class={if props.theme == Dark then "dark" else ""}>
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>
```

--- file: app/Theme.alder ---
```alder
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light
```

--- file: app/User.alder ---
```alder
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}
```

--- file: app/Note.alder ---
```alder
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
```

--- file: app/Notes/Index.alder ---
```alder
module Notes.Index

import Note
import Layout

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
  body: Text,
}

msg Submit form NewNote

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let id <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
    Task.succeed (Redirect (Notes.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Notes" theme={session.theme}>
    <h1>Notes</h1>

    <form submit={Submit}>
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Add note</button>
    </form>

    <ul id="notes">
      {for note in model.notes -> <li><a href={Notes.Show { id: note.id }}>{note.title}</a></li>}
    </ul>
  </Layout.Page>
```

--- file: app/Notes/Show.alder ---
```alder
module Notes.Show

import Note
import Layout

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
  body: Text,
}

msg Save { id: Note.NoteId } form EditNote
msg Delete { id: Note.NoteId }

-- Runs the task only when the note exists and belongs to the session user.
-- Otherwise the page is re-run through load, which answers 404.
ifOwned : Note.NoteId -> Session -> Task Nav -> Task Nav
ifOwned id session next = {
  let found <- Db.get Note id
  match found {
    Just note -> if note.owner == session.user then next else Task.succeed Rerender,
    Nothing -> Task.succeed Rerender,
  }
}

save : Note.NoteId -> EditNote -> Task Nav
save id form = {
  let _ <- Db.update Note id (\n -> { n | title: form.title, body: form.body })
  Task.succeed (Redirect (Notes.Index {}))
}

remove : Note.NoteId -> Task Nav
remove id = {
  let _ <- Db.delete Note id
  Task.succeed (Redirect (Notes.Index {}))
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> ifOwned p.id session (save p.id p.form),
  Delete p -> ifOwned p.id session (remove p.id),
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.note.title} theme={session.theme}>
    <h1>{model.note.title}</h1>
    <p class="body">{model.note.body}</p>

    <form submit={Save { id: model.note.id }}>
      <Input field="title" label="Title" value={model.note.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body" value={model.note.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>

    <button click={Delete { id: model.note.id }}>Delete</button>
    <a href={Notes.Index {}}>Back</a>
  </Layout.Page>
```
