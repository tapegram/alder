# Task T12: contacts, searched as you type

Write a complete new Alder app (not the todo app) for keeping contacts,
shared by many users on one server:

- Users log in with an email and a password, checked by a `Password`
  capability as in the example.
- A contact has a name and an email, and belongs to the user who added
  it. A user only ever sees their own contacts.
- `/contacts` requires login and lists the logged-in user's contacts
  whose name contains the query `q` (ignoring case; an empty `q` matches
  every contact), with a search field and a form to add a contact.
- As the user types in the search field, the list is replaced by the
  contacts matching what the field holds, without reloading the page.
- `/login` and logout, an `App` module, a `Layout` module and a `User`
  module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/Table/User.alder`: module `Table.User`, `type UserId = brand Text`, and
  `table User { id: UserId @id, email: Text.Email @unique, name: Text.NonEmpty, passwordHash: Text, created: Time @created }`.
- `app/App.alder`: module `App`, with exactly
  `session { user: Maybe User.UserId }`, `login = Auth.Login.Login`, and
  `route Logout`, which answers `Response.redirect (Auth.Login.Login {})`.
- `app/Table/Contact.alder`: module `Table.Contact`, `type ContactId = brand Text`, and
  `table Contact { id: ContactId @id, name: Text.NonEmpty, email: Text.Email, owner: User.UserId @ref User @owner }`.
- `app/Contacts/Index.alder`: route module `Contacts.Index`, with
  `route Find = GET "/contacts" { q: Text = "" } requires user`, the page, and
  `route Results = GET "/contacts/results" { q: Text = "" } requires user`,
  the matching contacts alone. `Results`'s view is a `<ul id="results">`
  with one `<li>` per matching contact, the contact's name its whole
  text; the page lists its matches the same way, in a
  `<ul id="results">`. The search field has `name="q"` and
  `input={Results { q: … }}` (chapter 6, "Search as you type").
  `form NewContact = { name: Text.NonEmpty, email: Text.Email }`, posted by
  `route Add form NewContact`, adds a contact for the logged-in user and
  answers `Response.redirect (Contacts.Index.Find {})`.
- `app/Auth/Login.alder`: route module `Auth.Login`,
  `route Login = GET "/login" { }`, with the `Password` capability and
  form `Credentials` as in the reference; a successful login answers
  `Response.redirect (Contacts.Index.Find {})`.

Also write tests (`test "…" = …`, chapter 10) in the modules they test:
typing a query lists only the matching contacts; an empty query lists them
all; another user's contacts never appear; the form adds a contact.
`alder test` must pass. Do not call the real `Password` adapter from a test;
installing the fake with `Test.fake Password` is fine.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
