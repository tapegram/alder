### Round 2

```text
$ alder fill Notes.Index.update.Add - --hole ?add <<'EOF'
{
  let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
  Task.succeed (Redirect (Notes.Index {}))
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -29,7 +29,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Add p -> ?add,
+  Add p -> {
+    let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
+    Task.succeed (Redirect (Notes.Index {}))
+  },
   Delete p -> ?delete,
 }
 
app/Notes/Index.alder:36:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:53:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:66:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:87:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Notes.Index.update.Delete - --hole ?delete <<'EOF'
{
  let found <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.where (Note.id.eq p.id) |> Db.first
  match found {
    Just n -> {
      let _ <- Db.delete Note n.id
      Task.succeed (Redirect (Notes.Index {}))
    },
    Nothing -> Task.notFound,
  }
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -33,7 +33,16 @@
     let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
     Task.succeed (Redirect (Notes.Index {}))
   },
-  Delete p -> ?delete,
+  Delete p -> {
+    let found <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.where (Note.id.eq p.id) |> Db.first
+    match found {
+      Just n -> {
+        let _ <- Db.delete Note n.id
+        Task.succeed (Redirect (Notes.Index {}))
+      },
+      Nothing -> Task.notFound,
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Notes/Index.alder:62:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>

  <al-card>
    <form submit={Add}>
      <al-input field="title" label="Title" />
      <Errors field="title" />
      <al-textarea field="body" label="Body" />
      <Errors form />
      <al-button type="submit">Add</al-button>
    </form>
  </al-card>

  <ul id="notes">
    {for note in model.notes ->
      <li>
        <a class="title" href={Notes.Show { id: note.id }}>{note.title}</a>
        <al-button variant="ghost" size="sm" click={Delete { id: note.id }}>Delete</al-button>
      </li>}
  </ul>

  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
app/Notes/Index.alder:56:9: error[E0028]: `<al-input field="title">` already shows the errors of `title`; remove `<Errors field="title" />`
          <Errors field="title" />
          ^^^^^^^^^^^^^^^^^^^^^^^
app/Notes/Index.alder:84:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:105:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 20 of your script: `alder fill --decl Notes.Index.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:62:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
