--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Teams.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: app/Invite.alder ---
module Invite

import Team

type InviteId = brand Text

type Status = Pending | Accepted | Declined

table Invite {
  id: InviteId @id,
  team: Team.TeamId @ref Team,
  email: Text.Email,
  status: Status = Pending,
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import User
import Team
import Member
import Invite
import Layout

page "/invites" { } requires user

type Waiting = { id: Invite.InviteId, team: Team }

type Model = { invites: List Waiting }

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.succeed { invites: [] },
    Just user -> {
      let rows <- Db.from Invite |> Db.where (Invite.email.eq user.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) rows)) |> Db.all
      Query.succeed { invites: List.filterMap (pairWith teams) rows }
    },
  }
}

pairWith : List Team -> Invite -> Maybe Waiting
pairWith teams invite = match (List.find (\t -> t.id == invite.team) teams) {
  Just t -> Just { id: invite.id, team: t },
  Nothing -> Nothing,
}

-- An invite is answerable only by the user whose email it names, while it is pending.
isWaitingFor : Invite -> Maybe User -> Bool
isWaitingFor invite me = match me {
  Nothing -> False,
  Just u -> u.email == invite.email && invite.status == Pending,
}

waitingFor : Invite.InviteId -> User.UserId -> Task Invite
waitingFor id user = {
  let found <- Db.get Invite id
  let me <- Db.get User user
  match found {
    Nothing -> Task.notFound,
    Just invite -> if isWaitingFor invite me then Task.succeed invite else Task.notFound,
  }
}

-- The owner is never added as a member, and a member is added once.
addMember : Team.TeamId -> User.UserId -> Task Unit
addMember team user = {
  let found <- Db.get Team team
  let existing <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.first
  match found {
    Nothing -> Task.succeed (),
    Just t -> if t.owner == user || Maybe.isJust existing then Task.succeed () else insertMember team user,
  }
}

insertMember : Team.TeamId -> User.UserId -> Task Unit
insertMember team user = {
  let _ <- Db.insert Member { team: team, user: user }
  Task.succeed ()
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let invite <- waitingFor p.id session.user
    let _ <- addMember invite.team session.user
    let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
    Task.succeed (Redirect (Teams.Show { id: invite.team }))
  },
  Decline p -> {
    let invite <- waitingFor p.id session.user
    let _ <- Db.update Invite invite.id (\i -> { i | status: Declined })
    Task.succeed (Redirect (Invites.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for w in model.invites ->
        <li>
          <span class="team">{w.team.name}</span>
          <button click={Accept { id: w.id }}>Accept</button>
          <button click={Decline { id: w.id }}>Decline</button>
        </li>}
    </ul>
    <a href={Teams.Index {}}>Teams</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

-- Accepting makes the team visible; declining leaves it closed to the user.
test "accepting an invite opens the team, declining does not" = {
  let guest <- Db.insert User { email: Text.Email "guest@example.com", name: Text.NonEmpty "Guest", passwordHash: "" }
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let teamA <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let teamB <- Db.insert Team { name: Text.NonEmpty "Chess", owner: owner }
  let inviteA <- Db.insert Invite { team: teamA, email: Text.Email "guest@example.com", status: Pending }
  let inviteB <- Db.insert Invite { team: teamB, email: Text.Email "guest@example.com", status: Pending }
  let _ <- Test.login { user: Just guest }
  let _ <- Test.get (Invites.Index {})
  let accepted <- Test.click (Accept { id: inviteA })
  let garden <- Test.open (Teams.Show { id: teamA })
  let _ <- Test.get (Invites.Index {})
  let declined <- Test.click (Decline { id: inviteB })
  let chess <- Test.open (Teams.Show { id: teamB })
  let left <- Test.get (Invites.Index {})
  Task.succeed ({ accepted: accepted.answer, garden: garden.answer, declined: declined.answer, chess: chess.answer, waiting: Test.count "#invites li" left.doc } == { accepted: Redirected (Teams.Show { id: teamA }), garden: Shown, declined: Redirected (Invites.Index {}), chess: PageMissing, waiting: 0 })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Member.alder ---
module Member

import Team
import User

type MemberId = brand Text

table Member {
  id: MemberId @id,
  team: Team.TeamId @ref Team,
  user: User.UserId @ref User,
}

--- file: app/Team.alder ---
module Team

import User

type TeamId = brand Text

table Team {
  id: TeamId @id,
  name: Text.NonEmpty,
  owner: User.UserId @ref User,
}

--- file: app/Teams/Index.alder ---
module Teams.Index

import User
import Team
import Member
import Layout

page "/teams" { } requires user

type Model = { teams: List Team }

load : Params -> Session -> Query Model
load params session = {
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let teams <-
    Db.from Team
      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)])
      |> Db.orderBy Team.name.asc
      |> Db.all
  Query.succeed { teams: teams }
}

form NewTeam = {
  name: Text.NonEmpty,
}

msg Create form NewTeam

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert Team { name: p.form.name, owner: session.user }
    Task.succeed (Redirect (Teams.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Teams">
    <h1>Teams</h1>
    <ul id="teams">
      {for t in model.teams ->
        <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
    </ul>
    <form submit={Create}>
      <Errors form />
      <Input field="name" label="New team" />
      <Errors field="name" />
      <button>Create team</button>
    </form>
    <a href={Invites.Index {}}>Invites</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "my teams are listed by name" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zebra", owner: me }
  let _ <- Db.insert Team { name: Text.NonEmpty "Apple", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Apple", "Zebra"])
}

test "creating a team opens its page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Garden" }
  Task.succeed (Test.text "#name" r.doc == Just "Garden")
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import User
import Team
import Member
import Invite
import Layout

page "/teams/{id}" { id: Team.TeamId } requires user

type Model = {
  team: Team,
  members: List User,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let ids = List.map (\m -> m.user) rows
      if canSee team session.user ids then loadTeam team ids else Query.notFound
    },
  }
}

-- The owner, or a member, may see the team; anyone else gets a 404.
canSee : Team -> User.UserId -> List User.UserId -> Bool
canSee team user ids = team.owner == user || List.member user ids

-- The owner comes first, then the other members by name.
loadTeam : Team -> List User.UserId -> Query Model
loadTeam team ids = {
  let owners <- Db.from User |> Db.where (User.id.eq team.owner) |> Db.all
  let others <- Db.from User |> Db.where (User.id.in ids) |> Db.orderBy User.name.asc |> Db.all
  Query.succeed { team: team, members: List.concat [owners, List.filter (\u -> u.id != team.owner) others] }
}

form NewInvite = {
  email: Text.Email,
}

msg Remove { user: User.UserId }
msg SendInvite form NewInvite

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Remove p -> if model.team.owner != session.user then Task.notFound else if p.user == model.team.owner then Task.succeed (Reject "The owner cannot be removed from the team.") else removeMember model.team.id p.user,
  SendInvite p -> if model.team.owner != session.user then Task.notFound else sendInvite model.team.id p.form.email,
}

removeMember : Team.TeamId -> User.UserId -> Task Nav
removeMember team user = {
  let found <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.first
  match found {
    Just m -> {
      let _ <- Db.delete Member m.id
      Task.succeed Rerender
    },
    Nothing -> Task.succeed Rerender,
  }
}

sendInvite : Team.TeamId -> Text.Email -> Task Nav
sendInvite team email = {
  let _ <- Db.insert Invite { team: team, email: email, status: Pending }
  Task.succeed (Redirect (Teams.Show { id: team }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      {for u in model.members ->
        <li>
          <span class="name">{u.name}</span>
          {if session.user == model.team.owner && u.id != model.team.owner then <button class="remove" click={Remove { user: u.id }}>Remove</button> else Html.none}
        </li>}
    </ul>
    {if session.user == model.team.owner then
      <form submit={SendInvite}>
        <Errors form />
        <Input field="email" label="Invite by email" />
        <Errors field="email" />
        <button>Invite</button>
      </form>
    else Html.none}
    <a href={Teams.Index {}}>All teams</a>
  </Layout.Page>

-- A stranger gets a 404 for a team they are not in.
test "a stranger cannot open a team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

test "a member sees no Remove button" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Test.login { user: Just member }
  let page <- Test.get (Teams.Show { id: team })
  Task.succeed ({ names: Test.texts "#members .name" page.doc, buttons: Test.count "button.remove" page.doc } == { names: ["Owner", "Member"], buttons: 0 })
}

test "the owner sees a Remove button beside each member but themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  Task.succeed ({ names: Test.texts "#members .name" page.doc, buttons: Test.count "button.remove" page.doc } == { names: ["Owner", "Member"], buttons: 1 })
}

test "the owner removes a member through the page" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: member })
  Task.succeed ({ answer: r.answer, names: Test.texts "#members .name" r.doc } == { answer: Rerendered, names: ["Owner"] })
}

-- A member posting Remove for another member is a 404, and nothing is removed.
test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Db.insert Member { team: team, user: other }
  let model <- load { id: team } { user: member }
  let r <- Test.run (update (Remove { user: other }) model { user: member })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, rows: left } == { value: Err "not found", rows: 2 })
}

-- The owner removes a member, but a removal of the owner is refused and changes nothing.
test "the owner removes a member, but not themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let model <- load { id: team } { user: owner }
  let removed <- Test.run (update (Remove { user: member }) model { user: owner })
  let refused <- Test.run (update (Remove { user: owner }) model { user: owner })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ removed: removed.value, refused: refused.value, left: left } == { removed: Ok Rerender, refused: Ok (Reject "The owner cannot be removed from the team."), left: 0 })
}

-- Only the owner may invite: a member's SendInvite is a 404 and stores no invite.
test "only the owner can invite" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let model <- load { id: team } { user: member }
  let r <- Test.run (update (SendInvite { form: { email: Text.Email "new@example.com" } }) model { user: member })
  let invites <- Db.from Invite |> Db.count
  Task.succeed ({ value: r.value, invites: invites } == { value: Err "not found", invites: 0 })
}

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}
