### Round 5

```text
$ alder fill --help
usage: alder fill <Module>.<decl>[.<arm>] - [--hole ?name] [--dir app] [--json] [--dry-run]  |  alder fill --decl <Module>.<decl> - [--dir app] [--json] [--dry-run]  |  alder fill --list [--dir app] [--json]
[exit 0]
$ alder regen admin --help
usage: alder regen admin [--module Admin] [--path /admin] [--all] [--dir app] [--json] [--dry-run]
[exit 0]
$ alder new admin --help
usage: alder new admin [--module Admin] [--path /admin] [--dir app] [--json] [--dry-run]
[exit 0]
$ alder remove --help
usage: alder remove page <Page module> | alder remove table <T>  [--dir app] [--json] [--dry-run]
[exit 0]
$ alder edit --help
usage: alder edit table <T> add-field <field>:<Type>[=default][@attr…] | rename-field <old> <new> | drop-field <field>  [--strict] [--dir app] [--json] [--dry-run]
       alder edit page <Page module> add-param <name>:<Type>[=default] [--dir app] [--json] [--dry-run]
       alder edit decl <Module>.<Name> add-prop <name>:<Type>=<default> [--dir app] [--json] [--dry-run]
       alder edit auth --to <Page> [--dir app] [--json] [--dry-run]
       alder edit session add-field <name>:<Type>=<default> [--strict] [--dir app] [--json] [--dry-run]
[exit 0]
$ alder new page --help
usage: alder new page <Module> <path> [--list T | --show T [--by <unique column>] | --from T …] [--blank] [--param <name>:<Type> …] [--requires user] [--role R] [--dir app] [--json] [--dry-run]
[exit 0]
$ alder fill --list
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

The harness stopped at line 7 of your script: `alder fill --list`: it exited 1. The commands after it were not run.

Your script stopped at command 7; the app is as the commands before it left it; continue.

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
