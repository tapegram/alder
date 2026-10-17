### Round 2

```text
$ alder new page Admin /admin --list Post --requires user
wrote app/Admin.alder
[exit 0]
$ alder fill --decl Admin.Model - <<'EOF'
{ posts: List Post }
EOF
app/Admin.alder:29:17: error[E0003]: record `Admin.Model` is missing field(s): posts
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Admin.alder:29:19: error[E0002]: no field `rows` on `Admin.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                    ^^^^^^^^^^
app/Admin.alder:29:31: error[E0002]: no field `page` on `Admin.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                ^^^^^^^^^^
app/Admin.alder:29:43: error[E0002]: no field `pages` on `Admin.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                            ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Admin.alder:43:23: error[E0002]: no field `rows` on `Admin.Model`
            {for row in model.rows ->
                        ^^^^^^^^^^
app/Admin.alder:45:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.slug}</td>
                     ^^^^^^^^
app/Admin.alder:46:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.title}</td>
                     ^^^^^^^^^
app/Admin.alder:47:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.body}</td>
                     ^^^^^^^^
app/Admin.alder:48:23: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{if row.published then <al-badge>Yes</al-badge> else <al-badge variant="secondary">No</al-badge>}</td>
                        ^^^^^^^^^^^^^
app/Admin.alder:49:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.author}</td>
                     ^^^^^^^^^^
app/Admin.alder:50:46: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{Time.format "Jan 2, 2006" row.created}</td>
                                               ^^^^^^^^^^^
app/Admin.alder:51:46: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{Time.format "Jan 2, 2006" row.updated}</td>
                                               ^^^^^^^^^^^
app/Admin.alder:58:11: error[E0002]: no field `page` on `Admin.Model`
        {if model.page > 1 then <a slot="prev" href={Admin { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
            ^^^^^^^^^^
app/Admin.alder:58:66: error[E0002]: no field `page` on `Admin.Model`
        {if model.page > 1 then <a slot="prev" href={Admin { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
                                                                   ^^^^^^^^^^
app/Admin.alder:59:32: error[E0002]: no field `page` on `Admin.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                 ^^^^^^^^^^
app/Admin.alder:59:61: error[E0002]: no field `pages` on `Admin.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                                              ^^^^^^^^^^^
app/Admin.alder:60:11: error[E0002]: no field `page` on `Admin.Model`
        {if model.page < model.pages then <a slot="next" href={Admin { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
            ^^^^^^^^^^
app/Admin.alder:60:24: error[E0002]: no field `pages` on `Admin.Model`
        {if model.page < model.pages then <a slot="next" href={Admin { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                         ^^^^^
… (1558 more characters cut)
[exit 1]
```

The harness stopped at line 3 of your script: `alder fill --decl Admin.Model - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Home.alder:8:23: warning[W0002]: hole ?load has type Query Home.Model
  load params session = ?load
                        ^^^^^
app/Home.alder:11:22: warning[W0002]: hole ?view has type Html Home.Msg
  view model session = ?view
                       ^^^^^
app/Posts/Show.alder:10:23: warning[W0002]: hole ?load has type Query Posts.Show.Model
  load params session = ?load
                        ^^^^^
app/Posts/Show.alder:13:22: warning[W0002]: hole ?view has type Html Posts.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
