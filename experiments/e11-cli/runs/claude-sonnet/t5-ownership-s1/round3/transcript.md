### Round 3

```text
$ alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
  <form submit={Add}>
    <Errors form />
    <Input field="title" label="Title" />
    <Textarea field="body" label="Body" />
    <button>Add</button>
  </form>
  <ul id="notes">
    {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a><button click={Delete { id: n.id }}>Delete</button></li>}
  </ul>
  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -42,21 +42,20 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Notes">
-  <h1>Notes</h1>
-
-  <al-card>
-    <h2 slot="header">New note</h2>
+view model session =
+  <Layout.Page title="Notes">
+    <h1>Notes</h1>
     <form submit={Add}>
-      <al-input field="title" label="Title" />
-      <al-textarea field="body" label="Body" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New note</al-button>
+      <Errors form />
+      <Input field="title" label="Title" />
+      <Textarea field="body" label="Body" />
+      <button>Add</button>
     </form>
-  </al-card>
-
-  <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
-</Layout.Page>
+    <ul id="notes">
+      {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a><button click={Delete { id: n.id }}>Delete</button></li>}
+    </ul>
+    <button click={App.Logout}>Log out</button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
app/Notes/Index.alder:70:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Notes.Index."submitting NewNote" - --hole ?expected <<'EOF'
Redirected (Notes.Index {})
EOF
alder fill: `Notes.Index.submitting NewNote` has no typed hole (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 16 of your script: `alder fill Notes.Index."submitting NewNote" - --hole ?expected <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:70:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
