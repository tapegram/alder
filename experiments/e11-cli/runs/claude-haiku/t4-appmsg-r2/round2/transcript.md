### Round 2

```text
$ alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner created:Time@created --no-timestamps
wrote app/Note.alder
wrote migrations/0002_create_note.sql
  create table Note
  add index on Note.owner
edited migrations/schema.json
[exit 0]
$ alder new decl Theme flip "Theme -> Theme"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -1,3 +1,6 @@
 module Theme
 
 type Theme = Light | Dark
+
+flip : Theme -> Theme
+flip = ?flip
app/Theme.alder:6:8: warning[W0002]: hole ?flip has type Theme -> Theme
  flip = ?flip
         ^^^^^
[exit 0]
$ alder fill Theme.flip - <<'EOF'
\t -> match t {
  Light -> Dark,
  Dark -> Light,
}
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -3,4 +3,7 @@
 type Theme = Light | Dark
 
 flip : Theme -> Theme
-flip = ?flip
+flip = \t -> match t {
+  Light -> Dark,
+  Dark -> Light,
+}
[exit 0]
$ alder new decl Theme isDark "Theme -> Bool"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -7,3 +7,6 @@
   Light -> Dark,
   Dark -> Light,
 }
+
+isDark : Theme -> Bool
+isDark = ?isDark
app/Theme.alder:12:10: warning[W0002]: hole ?isDark has type Theme -> Bool
  isDark = ?isDark
           ^^^^^^^
[exit 0]
$ alder fill Theme.isDark - <<'EOF'
\t -> match t {
  Light -> False,
  Dark -> True,
}
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -9,4 +9,7 @@
 }
 
 isDark : Theme -> Bool
-isDark = ?isDark
+isDark = \t -> match t {
+  Light -> False,
+  Dark -> True,
+}
[exit 0]
$ alder new decl Layout Shell "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"
alder new decl: `Shell` is not a value name: a value is named by a lower-case identifier, such as `waiting`
[exit 1]
```

The harness stopped at line 22 of your script: `alder new decl Layout Shell "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

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
