# Task T10: teams and invites

Write a complete new Alder app (not the todo app) where users form teams:

- Users log in with an email and a password, checked by a `Password`
  capability as in the example.
- A team has a name and an owner, the user who created it. Its other
  members are users who accepted an invite to it.
- `/teams` requires login and lists the teams the logged-in user owns or
  is a member of, each linking to its page, with a form to create a team
  (its owner is the user who creates it).
- `/teams/{id}` requires login and is for the team's owner and members
  only: for anyone else it is a 404. It shows the team's members, the
  owner included.
- The owner, and only the owner, invites a user by email from the team's
  page: the invite is stored with the email, whether or not a user has
  that email yet, and waits for an answer.
- `/invites` requires login and lists the invites waiting for the
  logged-in user (those to the user's email), each with an Accept and a
  Decline button. Accepting makes the user a member: the team is then on
  the user's `/teams` and its page opens. Declining does not: the invite
  leaves the list, and the team stays closed to the user. An invite that
  is not the user's, or is no longer waiting, cannot be answered (404).
- On the team's page the owner, and only the owner, sees a Remove button
  beside each member but themselves; it removes that member from the team.
  The owner cannot remove themselves: there is no button for it, and such
  a removal is refused even if the server receives one. A Remove or an
  invite the server receives from anyone but the owner is a 404, and
  changes nothing.
- `/login` and logout, an `App` module, a `Layout` module and a `User`
  module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/User.alder`: module `User`, `type UserId = brand Text`, and
  `table User { id: UserId @id, email: Text.Email @unique, name: Text.NonEmpty, passwordHash: Text, created: Time @created }`.
- `app/App.alder`: module `App`, with exactly
  `session { user: Maybe User.UserId }`, `login = Auth.Login`, and
  `msg Logout`, which answers `Redirect (Auth.Login {})`.
- `app/Team.alder`: module `Team`, `type TeamId = brand Text`, and
  `table Team { id: TeamId @id, name: Text.NonEmpty, owner: User.UserId @ref User }`.
- `app/Member.alder`: module `Member`, `type MemberId = brand Text`, and
  `table Member { id: MemberId @id, team: Team.TeamId @ref Team, user: User.UserId @ref User }`:
  one row per member who is not the owner (the owner has no `Member` row).
- `app/Invite.alder`: module `Invite`, `type InviteId = brand Text`,
  `type Status = Pending | Accepted | Declined`, and
  `table Invite { id: InviteId @id, team: Team.TeamId @ref Team, email: Text.Email, status: Status = Pending }`.
- `app/Teams/Index.alder`: page module `Teams.Index`,
  `page "/teams" { } requires user`. The teams are listed in an element
  with `id="teams"`, one `<li>` per team, ordered by name; inside it, an
  element with `class="name"` has the team's name as its whole text.
  `form NewTeam = { name: Text.NonEmpty }`, posted by
  `msg Create form NewTeam`, answers `Redirect (Teams.Show { id: … })`
  (the new team).
- `app/Teams/Show.alder`: page module `Teams.Show`,
  `page "/teams/{id}" { id: Team.TeamId } requires user`. The team's name
  is the whole text of an element with `id="name"`. The members are listed
  in an element with `id="members"`, one `<li>` per member, the owner first
  and then the others ordered by name; inside it, an element with
  `class="name"` has the user's name as its whole text. The Remove button
  has `class="remove"` and sends `msg Remove { user: User.UserId }`.
  `form NewInvite = { email: Text.Email }` is posted by
  `msg SendInvite form NewInvite`. Both answer
  `Redirect (Teams.Show { id: … })` (the same team).
- `app/Invites/Index.alder`: page module `Invites.Index`,
  `page "/invites" { } requires user`. The waiting invites are listed in
  an element with `id="invites"`, one `<li>` per invite; inside it, an
  element with `class="team"` has the team's name as its whole text.
  `msg Accept { id: Invite.InviteId }` answers
  `Redirect (Teams.Show { id: … })` (the invite's team), and
  `msg Decline { id: Invite.InviteId }` answers
  `Redirect (Invites.Index {})`.
- `app/Auth/Login.alder`: page module `Auth.Login`, `page "/login" { }`, with
  the `Password` capability, form `Credentials` and `msg Submit` as in the
  reference; a successful login answers `Redirect (Teams.Index {})`.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: a
team's page is not found for a user who is not a member; a member cannot
remove another member; the owner removes a member but cannot remove
themselves; accepting an invite makes the team visible, declining does not.
`alder test` must pass. Do not write tests that call the `Password`
capability.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
