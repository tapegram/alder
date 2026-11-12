--- file: app/App.alder ---
module App

import Layout
import Table.User

session { user: Maybe User.UserId }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me") })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] }
  )
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

--- file: app/Table/User.alder ---
module Table.User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}

--- file: app/Table/Team.alder ---
module Table.Team

import Table.User

type TeamId = brand Text

table Team {
  id: TeamId @id,
  name: Text.NonEmpty,
  owner: User.UserId @ref User,
}

-- The team, when the user is its owner; anyone else (or a missing team) is not found.
ownerOnly : TeamId -> User.UserId -> Query Team
ownerOnly id user = {
  let found <- Db.get Team id
  match found {
    Just team -> if team.owner == user then Query.succeed team else Query.notFound,
    Nothing -> Query.notFound,
  }
}

--- file: app/Table/Member.alder ---
module Table.Member

import Table.Team
import Table.User

type MemberId = brand Text

table Member {
  id: MemberId @id,
  team: Team.TeamId @ref Team,
  user: User.UserId @ref User,
}

-- The owner and the members may see a team.
canSee : Team -> User.UserId -> Query Bool
canSee team user =
  if team.owner == user then Query.succeed True
  else {
    let n <-
      Db.from Member
        |> Db.where (Member.team.eq team.id)
        |> Db.where (Member.user.eq user)
        |> Db.count
    Query.succeed (n > 0)
  }

isIn : Team.TeamId -> User.UserId -> Query Bool
isIn id user = {
  let found <- Db.get Team id
  match found {
    Just team -> canSee team user,
    Nothing -> Query.succeed False,
  }
}

--- file: app/Table/Invite.alder ---
module Table.Invite

import Table.Team
import Table.User

type InviteId = brand Text

type Status = Pending | Accepted | Declined

table Invite {
  id: InviteId @id,
  team: Team.TeamId @ref Team,
  email: Text.Email,
  status: Status = Pending,
}

-- The invite, when it is still waiting and is to the user's email; otherwise not found.
waitingFor : InviteId -> User.UserId -> Query Invite
waitingFor id user = {
  let found <- Db.get Invite id
  let me <- Db.get User user
  match found {
    Just invite ->
      if invite.status == Pending && Maybe.map (\u -> u.email) me == Just invite.email then Query.succeed invite
      else Query.notFound,
    Nothing -> Query.notFound,
  }
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
fake Password {
  verify args = {
    let found <-
      Db.from User
        |> Db.where (User.email.eq args.email)
        |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Teams.Index.Index {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    {
      answer: r.answer,
      user: r.session.user,
      title: Test.text "h1" r.doc,
    } == {
      answer: Redirected (Teams.Index.Index {}),
      user: Just me,
      title: Just "Teams",
    }
  )
}

--- file: app/Teams/Index.alder ---
module Teams.Index

import Layout
import Table.Member
import Table.Team
import Table.User

route Index  = GET  "/teams" { }              requires user
route Create = POST "/teams" { } form NewTeam requires user

type Model = { teams: List Team }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let memberships <-
    Db.from Member
      |> Db.where (Member.user.eq session.user)
      |> Db.all
  let teams <-
    Db.from Team
      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)])
      |> Db.orderBy Team.name.asc
      |> Db.all
  Query.succeed { teams: teams }
}

form NewTeam = { name: Text.NonEmpty }

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert Team { name: p.form.name, owner: session.user }
  Task.succeed (Response.redirect (Teams.Show.Show { id: id }))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Teams">
    <h1>Teams</h1>
    <ul id="teams">
      {for t in model.teams ->
        <li><a class="name" href={Teams.Show.Show { id: t.id }}>{t.name}</a></li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New team" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <nav><a href={Invites.Index.Index {}}>Invites</a></nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "the list shows the teams I own or joined, by name" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zebra", owner: ann }
  let joined <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: bob }
  let _ <- Db.insert Team { name: Text.NonEmpty "Other", owner: bob }
  let _ <- Db.insert Member { team: joined, user: ann }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Index {})
  let created <- Test.submit (Create {}) { name: Text.NonEmpty "Middle" }
  let page <- Test.get (Index {})
  Task.succeed (
    { status: created.status, names: Test.texts "#teams .name" page.doc }
      == { status: 200, names: ["Alpha", "Middle", "Zebra"] }
  )
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Layout
import Table.Invite
import Table.Member
import Table.Team
import Table.User

route Show       = GET    "/teams/{id}"                { id: Team.TeamId }                    requires user
route SendInvite = POST   "/teams/{id}/invites"        { id: Team.TeamId } form NewInvite     requires user
route Remove     = DELETE "/teams/{id}/members/{user}" { id: Team.TeamId, user: User.UserId } requires user

type Model = { team: Team, isOwner: Bool, members: List User }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Team params.id
  match found {
    Just team -> {
      let allowed <- Member.canSee team session.user
      if allowed then loadMembers team session.user else Query.notFound
    },
    Nothing -> Query.notFound,
  }
}

-- The owner first, then the other members by name.
loadMembers : Team -> User.UserId -> Query Model
loadMembers team me = {
  let owner <- Db.get User team.owner
  let rows <-
    Db.from Member
      |> Db.where (Member.team.eq team.id)
      |> Db.all
  let others <-
    Db.from User
      |> Db.where (User.id.in (List.map (\m -> m.user) rows))
      |> Db.orderBy User.name.asc
      |> Db.all
  Query.succeed { team: team, isOwner: team.owner == me, members: Maybe.toList owner ++ others }
}

form NewInvite = { email: Text.Email }

SendInvite.update : SendInvite.Params -> Session -> Task Response
SendInvite.update p session = {
  let team <- Team.ownerOnly p.id session.user
  let _ <- Db.insert Invite { team: team.id, email: p.form.email }
  Task.succeed (Response.redirect (Show { id: team.id }))
}

Remove.update : Remove.Params -> Session -> Task Response
Remove.update p session = {
  let team <- Team.ownerOnly p.id session.user
  if p.user == team.owner then Task.notFound
  else {
    let row <-
      Db.from Member
        |> Db.where (Member.team.eq team.id)
        |> Db.where (Member.user.eq p.user)
        |> Db.first
    match row {
      Just m -> {
        let _ <- Db.delete Member m.id
        Task.succeed (Response.redirect (Show { id: team.id }))
      },
      Nothing -> Task.notFound,
    }
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      {for u in model.members ->
        <li>
          <span class="name">{u.name}</span>
          {if model.isOwner && u.id != model.team.owner then <button class="remove" click={Remove { id: model.team.id, user: u.id }}>Remove</button> else Html.none}
        </li>}
    </ul>
    {if model.isOwner then <al-card><form submit={SendInvite { id: model.team.id }}><al-input field="email" label="Invite by email" /><al-button type="submit">Invite</al-button></form></al-card> else Html.none}
    <nav>
      <a href={Teams.Index.Index {}}>Teams</a>
      <a href={Invites.Index.Index {}}>Invites</a>
    </nav>
  </Layout.Page>

test "a team's page is not found for a user who is not a member" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let cara <- Db.insert User {
    email: Text.Email "cara@example.com",
    name: Text.NonEmpty "Cara",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: cara }
  let _ <- Test.login { user: Just bob }
  let stranger <- Test.open (Show { id: team })
  let _ <- Test.login { user: Just cara }
  let member <- Test.get (Show { id: team })
  Task.succeed (
    { stranger: stranger.answer, members: Test.texts "#members .name" member.doc }
      == { stranger: PageMissing, members: ["Ann", "Cara"] }
  )
}

test "a member cannot remove another member" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let cara <- Db.insert User {
    email: Text.Email "cara@example.com",
    name: Text.NonEmpty "Cara",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: cara }
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Show { id: team })
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Show { id: team })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { id: team, user: cara })
  let n <-
    Db.from Member
      |> Db.where (Member.team.eq team)
      |> Db.count
  Task.succeed (
    { buttons: Test.count "#members .remove" page.doc, answer: r.answer, members: n }
      == { buttons: 0, answer: PageMissing, members: 2 }
  )
}

test "the owner removes a member but cannot remove themselves" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Show { id: team })
  let removed <- Test.click (Remove { id: team, user: bob })
  let self <- Test.run (Remove.update { id: team, user: ann } { user: ann })
  let after <- Test.get (Show { id: team })
  Task.succeed (
    {
      buttons: Test.count "#members .remove" page.doc,
      removed: removed.answer,
      self: self.value,
      members: Test.texts "#members .name" after.doc,
    } == {
      buttons: 1,
      removed: Redirected (Show { id: team }),
      self: Err "not found",
      members: ["Ann"],
    }
  )
}

test "only the owner invites" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Show { id: team })
  let sent <- Test.submit (SendInvite { id: team }) { email: Text.Email "new@example.com" }
  let _ <- Test.login { user: Just bob }
  let refused <- Test.run (SendInvite.update { id: team, form: { email: Text.Email "x@example.com" } } { user: bob })
  let n <- Db.from Invite |> Db.count
  Task.succeed (
    { sent: sent.answer, refused: refused.value, invites: n }
      == { sent: Redirected (Show { id: team }), refused: Err "not found", invites: 1 }
  )
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Layout
import Table.Invite
import Table.Member
import Table.Team
import Table.User

route Index   = GET "/invites"               { }                     requires user
route Accept  = PUT "/invites/{id}/accepted" { id: Invite.InviteId } requires user
route Decline = PUT "/invites/{id}/declined" { id: Invite.InviteId } requires user

type Row = { id: Invite.InviteId, team: Text }

type Model = { invites: List Row }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let me <- Db.get User session.user
  match me {
    Just u -> {
      let invites <-
        Db.from Invite
          |> Db.where (Invite.email.eq u.email)
          |> Db.where (Invite.status.eq Pending)
          |> Db.all
      let teams <-
        Db.from Team
          |> Db.where (Team.id.in (List.map (\i -> i.team) invites))
          |> Db.all
      Query.succeed { invites: List.map (\i -> { id: i.id, team: teamName teams i.team }) invites }
    },
    Nothing -> Query.succeed { invites: [] },
  }
}

teamName : List Team -> Team.TeamId -> Text
teamName teams id = {
  let found = List.find (\t -> t.id == id) teams
  match found {
    Just t -> t.name,
    Nothing -> "",
  }
}

joinTeam : Team.TeamId -> User.UserId -> Task Unit
joinTeam team user = {
  let _ <- Db.insert Member { team: team, user: user }
  Task.succeed ()
}

Accept.update : Accept.Params -> Session -> Task Response
Accept.update p session = {
  let invite <- Invite.waitingFor p.id session.user
  let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
  let already <- Member.isIn invite.team session.user
  let _ <- if already then Task.succeed () else joinTeam invite.team session.user
  Task.succeed (Response.redirect (Teams.Show.Show { id: invite.team }))
}

Decline.update : Decline.Params -> Session -> Task Response
Decline.update p session = {
  let invite <- Invite.waitingFor p.id session.user
  let _ <- Db.update Invite invite.id (\i -> { i | status: Declined })
  Task.succeed (Response.redirect (Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for r in model.invites ->
        <li>
          <span class="team">{r.team}</span>
          <button class="accept" click={Accept { id: r.id }}>Accept</button>
          <button class="decline" click={Decline { id: r.id }}>Decline</button>
        </li>}
    </ul>
    <nav><a href={Teams.Index.Index {}}>Teams</a></nav>
  </Layout.Page>

test "accepting an invite makes the team visible" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let invite <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let before <- Test.open (Teams.Show.Show { id: team })
  let list <- Test.get (Index {})
  let accepted <- Test.click (Accept { id: invite })
  let teams <- Test.get (Teams.Index.Index {})
  let invites <- Test.get (Index {})
  Task.succeed (
    {
      before: before.answer,
      listed: Test.texts "#invites .team" list.doc,
      accepted: accepted.answer,
      members: Test.texts "#members .name" accepted.doc,
      teams: Test.texts "#teams .name" teams.doc,
      left: Test.count "#invites li" invites.doc,
    } == {
      before: PageMissing,
      listed: ["Red"],
      accepted: Redirected (Teams.Show.Show { id: team }),
      members: ["Ann", "Bob"],
      teams: ["Red"],
      left: 0,
    }
  )
}

test "declining an invite does not make the team visible" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let invite <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Index {})
  let declined <- Test.click (Decline { id: invite })
  let teams <- Test.get (Teams.Index.Index {})
  let show <- Test.open (Teams.Show.Show { id: team })
  let again <- Test.run (Accept.update { id: invite } { user: bob })
  Task.succeed (
    {
      declined: declined.answer,
      left: Test.count "#invites li" declined.doc,
      teams: Test.count "#teams li" teams.doc,
      show: show.answer,
      again: again.value,
    } == {
      declined: Redirected (Index {}),
      left: 0,
      teams: 0,
      show: PageMissing,
      again: Err "not found",
    }
  )
}

test "someone else's invite cannot be answered" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let cara <- Db.insert User {
    email: Text.Email "cara@example.com",
    name: Text.NonEmpty "Cara",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let invite <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just cara }
  let accepted <- Test.run (Accept.update { id: invite } { user: cara })
  let declined <- Test.run (Decline.update { id: invite } { user: cara })
  let n <- Db.from Member |> Db.count
  let after <- Db.get Invite invite
  Task.succeed (
    {
      accepted: accepted.value,
      declined: declined.value,
      members: n,
      status: Maybe.map (\i -> i.status) after,
    } == {
      accepted: Err "not found",
      declined: Err "not found",
      members: 0,
      status: Just Pending,
    }
  )
}
