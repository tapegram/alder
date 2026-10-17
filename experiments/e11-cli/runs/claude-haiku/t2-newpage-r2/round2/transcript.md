### Round 2

```text
$ alder fill --decl Admin.Todo.Bulk.update - <<'EOF'
\msg model session -> match msg {
  BulkDelete p -> {
    let allowed = List.filter (\row -> refused model session row == False) model.rows
    let _ <- Task.all (List.map (\row -> Db.delete Todo row.id) allowed)
    Task.succeed (Redirect (Admin.Todo.Index {}))
  },
  BulkSetDone p -> {
    let _ <- Task.all (List.map (\row -> Db.update Todo row.id (\r -> { r | done: p.form.value })) model.rows)
    Task.succeed (Redirect (Admin.Todo.Index {}))
  },
}
EOF
app/Admin/Todo/Bulk.alder:46:19: error[E0201]: module `Notice` is not imported
      Task.succeed (Notice.with ("Deleted " ++ Text.fromInt (List.length allowed) ++ " of " ++ Text.fromInt (List.length model.rows)) (Redirect (Admin.Todo.Index {})))
                    ^^^^^^^^^^^
  fix: import Notice

app/Admin/Todo/Show.alder:76:5: error[E0028]: unknown element `<al-dialog>`: declare custom elements in `elements/custom-elements.json` (chapter 5)
      <al-dialog id="delete-dialog" heading="Delete this todo?" dismissible>
      ^
app/Admin/User/Bulk.alder:44:19: error[E0201]: module `Notice` is not imported
      Task.succeed (Notice.with ("Deleted " ++ Text.fromInt (List.length allowed) ++ " of " ++ Text.fromInt (List.length model.rows)) (Redirect (Admin.User.Index {})))
                    ^^^^^^^^^^^
  fix: import Notice

app/Admin/User/Show.alder:62:5: error[E0028]: unknown element `<al-dialog>`: declare custom elements in `elements/custom-elements.json` (chapter 5)
      <al-dialog id="delete-dialog" heading="Delete this user?" dismissible>
      ^
alder fill: the app has errors (`alder check` lists them): fix them first, so that what is generated can be checked
[exit 1]
```

The harness stopped at line 3 of your script: `alder fill --decl Admin.Todo.Bulk.update - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The harness ran `alder check` on the app:

```text
app/Admin/Todo/Bulk.alder:46:19: error[E0201]: module `Notice` is not imported
      Task.succeed (Notice.with ("Deleted " ++ Text.fromInt (List.length allowed) ++ " of " ++ Text.fromInt (List.length model.rows)) (Redirect (Admin.Todo.Index {})))
                    ^^^^^^^^^^^
  fix: import Notice⏎
app/Admin/Todo/Show.alder:76:5: error[E0028]: unknown element `<al-dialog>`: declare custom elements in `elements/custom-elements.json` (chapter 5)
      <al-dialog id="delete-dialog" heading="Delete this todo?" dismissible>
      ^
app/Admin/User/Bulk.alder:44:19: error[E0201]: module `Notice` is not imported
      Task.succeed (Notice.with ("Deleted " ++ Text.fromInt (List.length allowed) ++ " of " ++ Text.fromInt (List.length model.rows)) (Redirect (Admin.User.Index {})))
                    ^^^^^^^^^^^
  fix: import Notice⏎
app/Admin/User/Show.alder:62:5: error[E0028]: unknown element `<al-dialog>`: declare custom elements in `elements/custom-elements.json` (chapter 5)
      <al-dialog id="delete-dialog" heading="Delete this user?" dismissible>
      ^
```
