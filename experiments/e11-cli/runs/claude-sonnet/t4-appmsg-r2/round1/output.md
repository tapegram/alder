```alder-cli
alder fill --list
alder new decl Layout shell "{ title: Text, theme: Theme.Theme, children: List (Html a) } -> Html a"
alder fill Layout.shell - <<'EOF'
\props -> <html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
  </head>
  <body class={if props.theme == Dark then Just "dark" else Nothing}>
    <main>
      <div>
        <button id="theme" click={App.SwitchTheme}>{if props.theme == Dark then "Switch to light" else "Switch to dark"}</button>
        <button click={App.Logout}>Log out</button>
      </div>
      {props.children}
    </main>
  </body>
</html>
EOF
alder new msg App SwitchTheme
alder fill --decl App.update - <<'EOF'
\msg session -> match msg {
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: match s.theme { Light -> Dark, Dark -> Light } })
    Task.succeed Rerender
  },
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}
EOF
alder new page Notes.Index / --blank --requires user
alder new page Notes.Show '/notes/{id}' --blank --requires user --param id:Note.NoteId
alder fill Auth.Login --hole ?afterLogin - <<'EOF'
Notes.Index {}
EOF
alder fill --decl Auth.Login.view - <<'EOF'
\model session -> Layout.shell { title: "Log in", theme: session.theme, children: [
  <h1>Log in</h1>,
  <form submit={Submit}>
    <Errors form />
    <Input field="email" label="Email" />
    <Input field="password" label="Password" />
    <button>Log in</button>
  </form>
] }
EOF
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill Notes.Index.load - <<'EOF'
\params session -> {
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill Notes.Index.view - <<'EOF'
\model session -> Layout.shell { title: "Notes", theme: session.theme, children: [] }
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder fill --decl Notes.Index.update - <<'EOF'
\msg model session -> match msg {
  Add p -> {
    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Index {}))
  },
}
EOF
alder fill --decl Notes.Index.view - <<'EOF'
\model session -> Layout.shell { title: "Notes", theme: session.theme, children: [
  <h1>Notes</h1>,
  <form submit={Add}>
    <Errors form />
    <Input field="title" label="Title" />
    <Textarea field="body" label="Body" />
    <button>Add</button>
  </form>,
  <ul id="notes">
    {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
  </ul>
] }
EOF
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill Notes.Show.load - <<'EOF'
\params session -> {
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}
EOF
alder fill Notes.Show.view - <<'EOF'
\model session -> Layout.shell { title: "Note", theme: session.theme, children: [] }
EOF
alder new form Notes.Show EditNote title:Text.NonEmpty body:Text --msg Save
alder new msg Notes.Show Delete
alder fill --decl Notes.Show.update - <<'EOF'
\msg model session -> match msg {
  Save p -> {
    let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Show { id: model.note.id }))
  },
  Delete p -> {
    let _ <- Db.deleteMine Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
\model session -> Layout.shell { title: "Note", theme: session.theme, children: [
  <h1 id="title">{model.note.title}</h1>,
  <p id="body">{model.note.body}</p>,
  <form submit={Save}>
    <Errors form />
    <Input field="title" label="Title" value={model.note.title} />
    <Textarea field="body" label="Body" value={model.note.body} />
    <button>Save</button>
  </form>,
  <button click={Delete}>Delete</button>,
  <a href={Notes.Index {}}>Back</a>
] }
EOF
alder new test Notes.Show "another user's note is not found"
alder fill 'Notes.Show."another user'"'"'s note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test App "the theme flips"
alder fill 'App."the theme flips"' - <<'EOF'
{
  let u <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just u }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click SwitchTheme
  Task.succeed ({ theme: r.session.theme, btn: Test.text "#theme" r.doc, cls: Test.attr "class" "body" r.doc } == { theme: Dark, btn: Just "Switch to light", cls: Just "dark" })
}
EOF
alder check
```
