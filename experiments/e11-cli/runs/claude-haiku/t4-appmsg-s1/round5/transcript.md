### Round 5

```text
$ alder new decl Layout shell "{ title: Text, dark: Bool, body: Html a } -> Html a"
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -15,3 +15,6 @@
       </al-page>
     </body>
   </html>
+
+shell : { title: Text, dark: Bool, body: Html a } -> Html a
+shell = ?shell
app/Layout.alder:20:9: warning[W0002]: hole ?shell has type { title: Text, dark: Bool, body: Html a } -> Html a
  shell = ?shell
          ^^^^^^
[exit 0]
$ alder fill --decl Layout.shell - <<'EOF'
\props -> {
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body class={if props.dark then "dark" else ""}>
      <al-page width="narrow">
        <main>{props.body}</main>
      </al-page>
    </body>
  </html>
}
EOF
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -17,4 +17,18 @@
   </html>
 
 shell : { title: Text, dark: Bool, body: Html a } -> Html a
-shell = ?shell
+shell = \props -> {
+  <html>
+    <head>
+      <meta charset="utf-8" />
+      <title>{props.title}</title>
+      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
+      <link rel="stylesheet" href={Url.external "/static/app.css"} />
+    </head>
+    <body class={if props.dark then "dark" else ""}>
+      <al-page width="narrow">
+        <main>{props.body}</main>
+      </al-page>
+    </body>
+  </html>
+}
[exit 0]
$ alder fill 'App."the theme flips"' - --hole ?test <<'EOF'
Task.succeed ({ first: toggled Light, second: toggled (toggled Light) } == { first: Dark, second: Light })
EOF
edited app/App.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -47,5 +47,5 @@
 }
 
 test "the theme flips" = {
-  ?test
+  Task.succeed ({ first: toggled Light, second: toggled (toggled Light) } == { first: Dark, second: Light })
 }
app/App.alder:4:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
app/Notes/Index.alder:29:17: error[E0003]: record `Notes.Index.Model` is missing field(s): notes
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Notes/Index.alder:29:19: error[E0002]: no field `rows` on `Notes.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                    ^^^^^^^^^^
app/Notes/Index.alder:29:31: error[E0002]: no field `page` on `Notes.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                ^^^^^^^^^^
app/Notes/Index.alder:29:43: error[E0002]: no field `pages` on `Notes.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                            ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Notes/Index.alder:43:23: error[E0002]: no field `rows` on `Notes.Index.Model`
            {for row in model.rows ->
                        ^^^^^^^^^^
app/Notes/Index.alder:45:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.title}</td>
                     ^^^^^^^^^
app/Notes/Index.alder:46:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.body}</td>
                     ^^^^^^^^
app/Notes/Index.alder:47:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.owner}</td>
                     ^^^^^^^^^
app/Notes/Index.alder:48:46: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{Time.format "Jan 2, 2006" row.created}</td>
                                               ^^^^^^^^^^^
app/Notes/Index.alder:55:11: error[E0002]: no field `page` on `Notes.Index.Model`
        {if model.page > 1 then <a slot="prev" href={Notes.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
            ^^^^^^^^^^
app/Notes/Index.alder:55:72: error[E0002]: no field `page` on `Notes.Index.Model`
        {if model.page > 1 then <a slot="prev" href={Notes.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
                                                                         ^^^^^^^^^^
app/Notes/Index.alder:56:32: error[E0002]: no field `page` on `Notes.Index.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                 ^^^^^^^^^^
app/Notes/Index.alder:56:61: error[E0002]: no field `pages` on `Notes.Index.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                                              ^^^^^^^^^^^
app/Notes/Index.alder:57:11: error[E0002]: no field `page` on `Notes.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Notes.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
            ^^^^^^^^^^
app/Notes/Index.alder:57:24: error[E0002]: no field `pages` on `Notes.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Notes.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                         ^^^^^^^^^^^
app/Notes/Index.alder:57:82: error[E0002]: no field `page` on `Notes.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Notes.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                                                                                   ^^^^^^^^^^
app/Notes/Index.alder:69:35: error[E0003]: cannot infer the record type of `row`; add a type annotat
… (1163 more characters cut)
[exit 1]
```

The harness stopped at line 27 of your script: `alder fill --decl Notes.Index.Model - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 4; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
