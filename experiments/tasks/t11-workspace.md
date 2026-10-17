# Task T11: orgs, members and a database per org

Write a complete new Alder app (not the todo app) where users work in orgs:

- Users log in with an email and a password, checked by a `Password`
  capability as in the example. A successful login goes to `/orgs`.
- An org has a name. A user works in the orgs they are a member of; the
  org they work in now is part of their session, and the session may only
  ever hold an org the user is a member of: setting any other is refused
  by the server, whatever the request says.
- `/orgs` requires login and lists the user's orgs, each with an Open
  button that makes it the session's org and goes to `/projects`, and a
  form to create an org: its creator becomes its first member, and it is
  opened.
- An org's projects and their to-dos are the org's alone, and each org's
  live in a database of the org's own (a silo).
- `/projects` requires login and an org: a user without one is sent to
  `/orgs`. It shows the org's name and lists the org's projects, each
  linking to its page, with a form to create a project in the org.
- `/projects/{id}` requires login and an org, and shows one project of
  the org with its to-dos and a form to add one; each to-do has a button
  that marks it done, or open again. A project of another org is a 404.
- Both project pages are a 404 for a user who is no longer a member of
  the session's org (a membership deleted since the org was opened): the
  session keeps the org, so the pages read the membership again.
- `/login` and logout, an `App` module, a `Layout` module and a `User`
  module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/User.alder`: module `User`, `type UserId = brand Text`, and
  `table User { id: UserId @id, email: Text.Email @unique, name: Text.NonEmpty, passwordHash: Text, created: Time @created }`.
- `app/App.alder`: module `App`, with exactly
  `session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership }`,
  `login = Auth.Login`, `choose org = Orgs.Index`,
  `silo org { Project, Todo }`, and `msg Logout`, which answers
  `Redirect (Auth.Login {})`.
- `app/Org.alder`: module `Org`, `type OrgId = brand Text`, and
  `table Org { id: OrgId @id, name: Text.NonEmpty }`.
- `app/Membership.alder`: module `Membership`,
  `type MembershipId = brand Text`, and
  `table Membership { id: MembershipId @id, user: User.UserId @ref User, org: Org.OrgId @ref Org }`:
  one row per member of an org.
- `app/Project.alder`: module `Project`, `type ProjectId = brand Text`, and
  `table Project { id: ProjectId @id, name: Text.NonEmpty, org: Org.OrgId @ref Org @partition org }`.
- `app/Todo.alder`: module `Todo`, `type TodoId = brand Text`, and
  `table Todo { id: TodoId @id, title: Text.NonEmpty, done: Bool = False, project: Project.ProjectId @ref Project @cascade, org: Org.OrgId @ref Org @partition org }`.
- `app/Orgs/Index.alder`: page module `Orgs.Index`,
  `page "/orgs" { } requires user`. The user's orgs are listed in an
  element with `id="orgs"`, one `<li>` per org, ordered by name; inside
  it, an element with `class="name"` has the org's name as its whole
  text, and its Open button sends `msg Open { org: Org.OrgId }`, which
  answers `Redirect (Projects.Index {})`.
  `form NewOrg = { name: Text.NonEmpty }`, posted by
  `msg Create form NewOrg`, answers `Redirect (Projects.Index {})`.
- `app/Projects/Index.alder`: page module `Projects.Index`,
  `page "/projects" { } requires user, org`. The org's name is the whole
  text of an element with `id="org"`. The projects are listed in an
  element with `id="projects"`, one `<li>` per project, ordered by name;
  inside it, an element with `class="name"` has the project's name as its
  whole text. `form NewProject = { name: Text.NonEmpty }`, posted by
  `msg Create form NewProject`, answers
  `Redirect (Projects.Show { id: … })` (the new project).
- `app/Projects/Show.alder`: page module `Projects.Show`,
  `page "/projects/{id}" { id: Project.ProjectId } requires user, org`.
  The project's name is the whole text of an element with `id="name"`.
  Its to-dos are listed in an element with `id="todos"`, one `<li>` per
  to-do, ordered by title; the `<li>` of a done to-do has the class
  `done`; inside it, an element with `class="title"` has the to-do's title
  as its whole text, and its button sends `msg Toggle { id: Todo.TodoId }`.
  `form NewTodo = { title: Text.NonEmpty }` is posted by
  `msg Add form NewTodo`. `msg Add` and `msg Toggle` each answer
  `Redirect (Projects.Show { id: … })` (the same project).
- `app/Auth/Login.alder`: page module `Auth.Login`, `page "/login" { }`, with
  the `Password` capability, form `Credentials` and `msg Submit` as in the
  reference; a successful login answers `Redirect (Orgs.Index {})`.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: a
member sees only their org's projects; a user cannot open an org they are
not a member of; a project of another org is not found; a member removed
from the org loses access at once. `alder test` must pass. Do not call the
real `Password` adapter from a test; installing the fake with `Test.fake
Password` is fine.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
