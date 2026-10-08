<!-- model=qwen3-coder:30b task=t4-appmsg.md seconds=21 prompt_tokens=9953 output_tokens=1231 -->
--- file: app/App.alder ---
module App

session { user: Maybe UserId, theme: Theme = Light }

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

--- file: app/Auth/Login.alder ---
module Auth.Login

page "/" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Html Msg
view model = <html>
  <head><title>Login</title></head>
  <body>
    <h1>Login</h1>
    <p>You must be logged in to use the notes app.</p>
  </body>
</html>

msg Login = "/login" form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Login p -> {
    let user <- Db.get User (UserId "test-user-id")
    let _ <- Session.put (\s -> { s | user: Just (user.id) })
    Task.succeed (Redirect (Notes.Index {}))
  },
}

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

--- file: app/Layout.alder ---
module Layout

Layout : { title: Text, children: List (Html Msg), theme: Theme } -> Html Msg
Layout props =
  <html>
    <head><title>{props.title}</title></head>
    <body class={if props.theme == Dark then "dark" else ""}>
      {props.children}
    </body>
  </html>

--- file: app/Notes/Index.alder ---
module Notes.Index

page "/" { }

type Model = { notes: List Note }

load : Params -> Session -> Query Model
load params session = {
  let notes <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}

view : Model -> Html Msg
view model = <Layout title="Notes" theme={session.theme}>
  <h1>Notes</h1>
  <form submit={Create}>
    <Input field="title" label="Title" />
    <Errors field="title" />
    <Textarea field="body" label="Body" />
    <button type="submit">Create Note</button>
  </form>
  <ul>
    {for note in model.notes -> <li><a href={Notes.Show { id: note.id }}>{note.title}</a></li>}
  </ul>
</Layout>

msg Create = "/notes" form NewNote

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
    Task.succeed (Redirect (Notes.Show { id: id }))
  },
}

form NewNote = {
  title: Text.NonEmpty,
  body: Text = "",
}

--- file: app/Notes/Show.alder ---
module Notes.Show

page "/notes/{id}" { id: NoteId }

type Model = { note: Note }

load : Params -> Session -> Query Model
load params session = {
  let note <- Db.get Note params.id
  let note <- case note of {
    Just n -> Query.succeed n,
    Nothing -> Query.notFound,
  }
  let _ <- case note.owner == session.user of {
    True -> Query.succeed (),
    False -> Query.notFound,
  }
  Query.succeed { note: note }
}

view : Model -> Html Msg
view model = <Layout title={model.note.title} theme={session.theme}>
  <h1>{model.note.title}</h1>
  <p>{model.note.body}</p>
  <form submit={Update}>
    <Input field="title" label="Title" value={model.note.title} />
    <Errors field="title" />
    <Textarea field="body" label="Body" value={model.note.body} />
    <button type="submit">Update</button>
  </form>
  <form submit={Delete}>
    <button type="submit">Delete</button>
  </form>
  <a href={Notes.Index {}}>Back to notes</a>
</Layout>

msg Update = "/notes/{id}/update" { id: NoteId } form NoteUpdate
msg Delete = "/notes/{id}/delete" { id: NoteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Update p -> {
    let _ <- Db.update Note p.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed Rerender
  },
  Delete p -> {
    let _ <- Db.delete Note p.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}

form NoteUpdate = {
  title: Text.NonEmpty,
  body: Text = "",
}

--- file: app/Note.alder ---
module Note

table Note {
  id: NoteId @id,
  title: Text.NonEmpty,
  body: Text = "",
  owner: UserId @ref User,
  created: Time @created,
}

type NoteId = brand Text

--- file: app/User.alder ---
module User

table User {
  id: UserId @id,
  email: Text.Email @unique,
  password: Text.Secret,
} 

type UserId = brand Text
