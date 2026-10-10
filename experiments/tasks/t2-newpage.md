# Task T2: a profile page

Add a page to the example app at `/profile` that:

- requires a logged-in user,
- shows the user's name and email,
- shows how many todos they have and how many are done,
- has a form to change the user's name (which must not be empty), and
- has a button to log out (the Login page already has a Logout msg; do not
  duplicate it — link or post to it as the language allows, or explain in a
  comment if you cannot).

Also add a link to the profile page from the Index page.

Also write tests (`test "…" = …`, chapter 10) in the modules they test:
the profile page's `load` counts only the user's todos; renaming to an
empty name is refused. `alder test` must pass. Do not write tests that
call the `Password` capability.

Output only the files you add or change: not the files you leave as they are,
not the standard library, nothing outside `app/`. Give each in full, preceded by
a line `--- file: app/<path> ---`. No explanation, only files.
