### Round 4

```text
$ alder check --json
{"code":"E0201","severity":"error","file":"app/Admin/Todo/Bulk.alder","line":46,"col":19,"endLine":46,"endCol":30,"message":"module `Notice` is not imported","fix":{"line":9,"col":1,"endLine":9,"endCol":1,"replace":"import Notice\n"}}
{"code":"E0028","severity":"error","file":"app/Admin/Todo/Show.alder","line":76,"col":5,"endLine":81,"endCol":16,"message":"unknown element `<al-dialog>`: declare custom elements in `elements/custom-elements.json` (chapter 5)"}
{"code":"E0201","severity":"error","file":"app/Admin/User/Bulk.alder","line":44,"col":19,"endLine":44,"endCol":30,"message":"module `Notice` is not imported","fix":{"line":9,"col":1,"endLine":9,"endCol":1,"replace":"import Notice\n"}}
{"code":"E0028","severity":"error","file":"app/Admin/User/Show.alder","line":62,"col":5,"endLine":67,"endCol":16,"message":"unknown element `<al-dialog>`: declare custom elements in `elements/custom-elements.json` (chapter 5)"}
[exit 1]
```

The harness stopped at line 1 of your script: `alder check --json`: it exited 1. The commands after it were not run.

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
