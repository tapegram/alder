### Round 1

```text
$ alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner
wrote app/Note.alder
wrote migrations/0002_create_note.sql
  create table Note
  add index on Note.owner
edited migrations/schema.json
[exit 0]
$ alder new page Notes.Index / --list Note --requires user
wrote app/Notes/Index.alder
[exit 0]
$ alder new page Notes.Show /notes/{id} --show Note --requires user --param id:Note.NoteId
alder new page: --param goes with a page of its own or a --blank one: a --list, --show or --from page has its parameters made for it
[exit 1]
```

The harness stopped at line 6 of your script: `alder new page Notes.Show /notes/{id} --show Note --requires user --param id:Note.NoteId`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
