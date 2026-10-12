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
      <nav>
        <a href={Teams.Index {}}>Teams</a>
        <a href={Invites.Index {}}>Invites</a>
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Team.alder ---
module Team

import User

type TeamId = brand Text

table Team {
  id: TeamId @id,
  name: Text.NonEmpty,
  owner: User.UserId @ref User,
}

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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place.
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
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

--- file: app/Teams/Index.alder ---
module Teams.Index

import Team
import Member
import User
import Layout

page "/teams" { } requires user

type Model = {
  teams: List Team,
}

load : Params -> Session -> Query Model
load params session = {
  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined <- Query.all (List.map (\m -> Db.get Team m.team) memberships)
  let teams = List.sortByText (\t -> t.name) (owned ++ List.filterMap (\t -> t) joined)
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
        <li><a href={Teams.Show { id: t.id }}><span class="name">{t.name}</span></a></li>}
    </ul>
    <form submit={Create}>
      <Input field="name" label="New team name" />
      <Errors field="name" />
      <button>Create team</button>
    </form>
  </Layout.Page>

test "creating a team opens its page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Club" }
  Task.succeed (Test.text "#name" r.doc == Just "Club")
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Team
import Member
import Invite
import User
import Layout

page "/teams/{id}" { id: Team.TeamId } requires user

type Person = {
  user: User.UserId,
  name: Text,
  isOwner: Bool,
}

type Model = {
  team: Team,
  isOwner: Bool,
  members: List Person,
}

form NewInvite = {
  email: Text.Email,
}

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

personOf : User.UserId -> User -> Person
personOf owner u = { user: u.id, name: u.name, isOwner: u.id == owner }

teamPage : Team -> Session -> Query Model
teamPage team session = {
  let ownerRow <- Db.get User team.owner
  let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
  let memberRows <- Query.all (List.map (\m -> Db.get User m.user) rows)
  let others = List.sortByText (\u -> u.name) (List.filterMap (\u -> u) memberRows)
  let owners = List.filterMap (\u -> u) [ownerRow]
  Query.succeed { team: team, isOwner: team.owner == session.user, members: List.map (personOf team.owner) (owners ++ others) }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let mine <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.where (Member.user.eq session.user) |> Db.first
      if team.owner == session.user || Maybe.isJust mine then teamPage team session else Query.notFound
    },
  }
}

-- ownTeam is the team when the session's user owns it; otherwise the request is a 404.
ownTeam : Team.TeamId -> Session -> Task Team
ownTeam id session = {
  let found <- Db.get Team id
  match found {
    Just team -> if team.owner == session.user then Task.succeed team else Task.notFound,
    Nothing -> Task.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p -> {
    let team <- ownTeam model.team.id session
    let _ <- Db.insert Invite { team: team.id, email: p.form.email }
    Task.succeed (Redirect (Teams.Show { id: team.id }))
  },
  Remove p -> {
    let team <- ownTeam model.team.id session
    if p.user == team.owner then Task.notFound else {
      let row <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.where (Member.user.eq p.user) |> Db.first
      match row {
        Just m -> {
          let _ <- Db.delete Member m.id
          Task.succeed Rerender
        },
        Nothing -> Task.notFound,
      }
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      {for m in model.members ->
        <li>
          <span class="name">{m.name}</span>
          {if model.isOwner && not m.isOwner then <button class="remove" click={Remove { user: m.user }}>Remove</button> else Html.none}
        </li>}
    </ul>
    {if model.isOwner then
      <form submit={SendInvite}>
        <Input field="email" label="Invite by email" />
        <Errors field="email" />
        <button>Invite</button>
      </form>
    else Html.none}
    <a href={Teams.Index {}}>All teams</a>
  </Layout.Page>

test "a team's page is not found for a non-member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

test "the owner invites a user by email" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "friend@example.com" }
  let n <- Db.from Invite |> Db.count
  Task.succeed ({ answer: r.answer, invites: n } == { answer: Redirected (Teams.Show { id: team }), invites: 1 })
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Db.insert Member { team: team, user: other }
  let _ <- Test.login { user: Just member }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: other }) page.model { user: member })
  let left <- Db.from Member |> Db.count
  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 2 })
}

test "the owner removes a member, and cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let member <- Db.insert User { email: Text.Email "member@example.com", name: Text.NonEmpty "Member", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let selfRemove <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  let memberRemove <- Test.run (update (Remove { user: member }) page.model { user: owner })
  let left <- Db.from Member |> Db.count
  let buttons = Test.count "button.remove" page.doc
  Task.succeed ({ self: selfRemove.value, removed: memberRemove.value, left: left, buttons: buttons } == { self: Err "not found", removed: Ok Rerender, left: 0, buttons: 1 })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Invite
import Team
import Member
import User
import Layout

page "/invites" { } requires user

type Waiting = {
  id: Invite.InviteId,
  teamName: Text,
}

type Model = {
  invites: List Waiting,
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

toWaiting : Invite -> Team -> Waiting
toWaiting invite team = { id: invite.id, teamName: team.name }

waitingOf : Invite -> Query (Maybe Waiting)
waitingOf invite = {
  let team <- Db.get Team invite.team
  Query.succeed (Maybe.map (toWaiting invite) team)
}

addressedTo : Invite -> Maybe User -> Bool
addressedTo invite me = Maybe.withDefault False (Maybe.map (\u -> u.email == invite.email) me)

waitingFor : User.UserId -> Query (List Waiting)
waitingFor uid = {
  let me <- Db.get User uid
  match me {
    Nothing -> Query.succeed [],
    Just u -> {
      let rows <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let items <- Query.all (List.map waitingOf rows)
      Query.succeed (List.filterMap (\w -> w) items)
    },
  }
}

load : Params -> Session -> Query Model
load params session = {
  let waiting <- waitingFor session.user
  Query.succeed { invites: waiting }
}

-- waitingInvite is the invite a message names, when it is addressed to the session's user and
-- still waiting; otherwise the request is a 404.
waitingInvite : Invite.InviteId -> Session -> Task Invite
waitingInvite id session = {
  let found <- Db.get Invite id
  let me <- Db.get User session.user
  match found {
    Just invite -> if invite.status == Pending && addressedTo invite me then Task.succeed invite else Task.notFound,
    Nothing -> Task.notFound,
  }
}

-- joinTeam makes the user a member, unless they own the team or already are one.
joinTeam : Invite -> User.UserId -> Task Unit
joinTeam invite uid = {
  let team <- Db.get Team invite.team
  let row <- Db.from Member |> Db.where (Member.team.eq invite.team) |> Db.where (Member.user.eq uid) |> Db.first
  let owner = Maybe.withDefault False (Maybe.map (\t -> t.owner == uid) team)
  if owner || Maybe.isJust row then Task.succeed () else {
    let _ <- Db.insert Member { team: invite.team, user: uid }
    Task.succeed ()
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let invite <- waitingInvite p.id session
    let _ <- joinTeam invite session.user
    let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
    Task.succeed (Redirect (Teams.Show { id: invite.team }))
  },
  Decline p -> {
    let invite <- waitingInvite p.id session
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
          <span class="team">{w.teamName}</span>
          <button click={Accept { id: w.id }}>Accept</button>
          <button click={Decline { id: w.id }}>Decline</button>
        </li>}
    </ul>
  </Layout.Page>

test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: invite })
  let teams <- Test.get (Teams.Index {})
  let opened <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts "#teams .name" teams.doc, open: opened.answer } == { answer: Redirected (Teams.Show { id: team }), listed: ["Club"], open: Shown })
}

test "declining an invite does not open the team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: invite })
  let teams <- Test.get (Teams.Index {})
  let opened <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, waiting: Test.count "#invites li" r.doc, open: opened.answer, listed: Test.texts "#teams .name" teams.doc } == { answer: Redirected (Invites.Index {}), waiting: 0, open: PageMissing, listed: [] })
}
