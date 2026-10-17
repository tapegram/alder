The session field failed because `App` does not import `Theme`, and the `Light` default needs that import. This round fills the login's redirect hole, lists the addresses so the test hole can be found, adds the `SwitchTheme` message, and retries the session field with the qualified type.

```alder-cli
# where a successful login goes: the notes list
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
# list the addresses, to find the login test's hole
alder fill --list
# the app-level theme message
alder new msg App SwitchTheme
# the session field, with the theme's type qualified (App must import Theme for the constructor)
alder edit session add-field theme:Theme.Theme=Light
```
