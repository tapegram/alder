Continuing from where the script stopped: the notes table and pages come first, then the login hole can point at a page that exists, then the theme.

```alder-cli
# the notes table, owned by the user
alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner

# pages: list (with create) and show (with edit/delete)
alder new page Notes.Index / --list Note --requires user
alder new page Notes.Show /notes/{id} --show Note --requires user --param id:Note.NoteId

# login goes to the notes list
alder fill Auth.Login.update --hole ?afterLogin - <<'EOF'
Notes.Index {}
EOF

# theme
alder new type User "Theme = Light | Dark"
alder edit session add-field theme:User.Theme=Light
alder new msg App ToggleTheme
alder fill --list
```
