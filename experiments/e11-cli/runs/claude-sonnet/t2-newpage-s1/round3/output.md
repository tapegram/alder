```alder-cli
alder fill 'Profile."submitting Rename"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submit Rename { name: Text.NonEmpty "New name" }
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: Rerendered, name: Just (Text.NonEmpty "New name") })
}
EOF
alder test
```
