# Survivors: reference/t11-workspace

One equivalent mutant, the one `examples/workspace/README.md` names for its own `Orgs.Index`
`Open` (book 14, "Its tests and its score"); no input can tell it from the app.

- `Orgs.Index Open:9 requires dropped` (`requires user` → served without user, user =
  `Table.User.UserId "mutant"`): Open's one step is a `Session.put` of the org, and a session
  without a user has its `via` fields cleared, so the visitor's org is dropped and the redirect
  to `/projects` (`requires user, org`) sends them to log in, as the `requires` would. A probe
  test (a page held, the user logged out, Open pressed) sees the same answer, url and session
  either way: `Redirected (Auth.Login.Login {})`, `/login`, no user, no org.
