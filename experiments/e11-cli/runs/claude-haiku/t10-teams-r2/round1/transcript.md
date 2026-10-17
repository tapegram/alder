### Round 1

```text
$ alder remove page Teams.Index
removed app/Teams/Index.alder
[exit 0]
$ alder remove page Invites.Index
removed app/Invites/Index.alder
[exit 0]
$ alder new page Teams.Index /teams --list Team --requires user
wrote app/Teams/Index.alder
[exit 0]
$ alder new page Invites.Index /invites --list Team --requires user
wrote app/Invites/Index.alder
[exit 0]
$ alder fill --decl Teams.Index.Model - <<'EOF'
{ teams: List Team }
EOF
app/Teams/Index.alder:29:17: error[E0003]: record `Teams.Index.Model` is missing field(s): teams
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Teams/Index.alder:29:19: error[E0002]: no field `rows` on `Teams.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                    ^^^^^^^^^^
app/Teams/Index.alder:29:31: error[E0002]: no field `page` on `Teams.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                ^^^^^^^^^^
app/Teams/Index.alder:29:43: error[E0002]: no field `pages` on `Teams.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                            ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Teams/Index.alder:43:23: error[E0002]: no field `rows` on `Teams.Index.Model`
            {for row in model.rows ->
                        ^^^^^^^^^^
app/Teams/Index.alder:45:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.name}</td>
                     ^^^^^^^^
app/Teams/Index.alder:46:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.owner}</td>
                     ^^^^^^^^^
app/Teams/Index.alder:53:11: error[E0002]: no field `page` on `Teams.Index.Model`
        {if model.page > 1 then <a slot="prev" href={Teams.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
            ^^^^^^^^^^
app/Teams/Index.alder:53:72: error[E0002]: no field `page` on `Teams.Index.Model`
        {if model.page > 1 then <a slot="prev" href={Teams.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
                                                                         ^^^^^^^^^^
app/Teams/Index.alder:54:32: error[E0002]: no field `page` on `Teams.Index.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                 ^^^^^^^^^^
app/Teams/Index.alder:54:61: error[E0002]: no field `pages` on `Teams.Index.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                                              ^^^^^^^^^^^
app/Teams/Index.alder:55:11: error[E0002]: no field `page` on `Teams.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Teams.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
            ^^^^^^^^^^
app/Teams/Index.alder:55:24: error[E0002]: no field `pages` on `Teams.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Teams.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                         ^^^^^^^^^^^
app/Teams/Index.alder:55:82: error[E0002]: no field `page` on `Teams.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Teams.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                                                                                   ^^^^^^^^^^
app/Teams/Index.alder:65:29: error[E0002]: no field `rows` on `Teams.Index.Model`
    Task.succeed (List.length page.model.rows == 2)
                              ^^^^^^^^^^^^^^^
app/Teams/Index.alder:74:38: error[E0002]: no field `rows` on `Teams.Index.Model`
    Task.succeed ({ first: List.length first.model.rows, second: List.length second.model.rows, pages: Test.texts "al-pagination span" first.doc } == { first: 20, second: 1, pages: ["Previous", "Page 1 of 2"] })
                          
… (496 more characters cut)
[exit 1]
```

The harness stopped at line 8 of your script: `alder fill --decl Teams.Index.Model - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Teams/Show.alder:11:23: warning[W0002]: hole ?load has type Query Teams.Show.Model
  load params session = ?load
                        ^^^^^
app/Teams/Show.alder:14:22: warning[W0002]: hole ?view has type Html Teams.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
