```alder-cli
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF
alder test
```
