### Round 5

```text
$ alder check --help
usage: alder check [dir] [--json]
[exit 0]
$ alder fill --help
usage: alder fill <Module>.<decl>[.<arm>] - [--hole ?name] [--dir app] [--json] [--dry-run]  |  alder fill --decl <Module>.<decl> - [--dir app] [--json] [--dry-run]  |  alder fill --list [--dir app] [--json]
[exit 0]
$ alder remove --help
usage: alder remove page <Page module> | alder remove table <T>  [--dir app] [--json] [--dry-run]
[exit 0]
```

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
