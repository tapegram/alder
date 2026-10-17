### Round 1

```text
$ alder regen admin --all
app/Admin/Todo/Bulk.alder:46:19: error[E0201]: module `Notice` is not imported
      Task.succeed (Notice.with ("Deleted " ++ Text.fromInt (List.length allowed) ++ " of " ++ Text.fromInt (List.length model.rows)) (Redirect (Admin.Todo.Index {})))
                    ^^^^^^^^^^^
  fix: import Notice

app/Admin/Todo/Edit.alder:78:11: error[E0002]: no field `ownersTotal` on `Admin.Todo.Edit.Model`
        {if model.ownersTotal > 200 then <form submit={SearchOwner}><al-input field="q" label="Find owner" /><al-button type="submit" variant="secondary">Find</al-button></form> else <span></span>}
            ^^^^^^^^^^^^^^^^^
app/Admin/Todo/New.alder:73:11: error[E0002]: no field `ownersTotal` on `Admin.Todo.New.Model`
        {if model.ownersTotal > 200 then <form submit={SearchOwner}><al-input field="q" label="Find owner" /><al-button type="submit" variant="secondary">Find</al-button></form> else <span></span>}
            ^^^^^^^^^^^^^^^^^
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
alder regen admin: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 2 of your script: `alder regen admin --all`: it exited 1. The commands after it were not run.

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
