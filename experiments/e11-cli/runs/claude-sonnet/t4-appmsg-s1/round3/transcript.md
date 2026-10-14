### Round 3

```text
$ alder new decl Theme flip "Theme -> Theme"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -4,3 +4,6 @@
 
 isDark : Theme -> Bool
 isDark = \t -> t == Dark
+
+flip : Theme -> Theme
+flip = ?flip
app/Theme.alder:9:8: warning[W0002]: hole ?flip has type Theme -> Theme
  flip = ?flip
         ^^^^^
[exit 0]
$ alder fill Theme.flip - <<'EOF'
\t -> if t == Dark then Light else Dark
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -6,4 +6,4 @@
 isDark = \t -> t == Dark
 
 flip : Theme -> Theme
-flip = ?flip
+flip = \t -> if t == Dark then Light else Dark
[exit 0]
$ alder new decl Theme light "Theme"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -7,3 +7,6 @@
 
 flip : Theme -> Theme
 flip = \t -> if t == Dark then Light else Dark
+
+light : Theme
+light = ?light
app/Theme.alder:12:9: warning[W0002]: hole ?light has type Theme
  light = ?light
          ^^^^^^
[exit 0]
$ alder fill Theme.light - <<'EOF'
Light
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -9,4 +9,4 @@
 flip = \t -> if t == Dark then Light else Dark
 
 light : Theme
-light = ?light
+light = Light
[exit 0]
$ alder new decl Theme dark "Theme"
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -10,3 +10,6 @@
 
 light : Theme
 light = Light
+
+dark : Theme
+dark = ?dark
app/Theme.alder:15:8: warning[W0002]: hole ?dark has type Theme
  dark = ?dark
         ^^^^^
[exit 0]
$ alder fill Theme.dark - <<'EOF'
Dark
EOF
edited app/Theme.alder
--- a/app/Theme.alder
+++ b/app/Theme.alder
@@ -12,4 +12,4 @@
 light = Light
 
 dark : Theme
-dark = ?dark
+dark = Dark
[exit 0]
$ alder new decl Layout Themed "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"
alder new decl: `Themed` is not a value name: a value is named by a lower-case identifier, such as `waiting`
[exit 1]
```

The harness stopped at line 16 of your script: `alder new decl Layout Themed "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"`: it exited 1. The commands after it were not run.

Your script stopped at command 7; the app is as the commands before it left it; continue.

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
