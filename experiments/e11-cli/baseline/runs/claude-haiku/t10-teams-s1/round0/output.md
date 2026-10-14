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

test "Logout signs the user out" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let out <- Test.click Logout
  Task.succeed ({ answer: out.answer, user: out.session.user } == { answer: Redirected (Auth.Login {}), user: Nothing })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>{props.children}</main>
      </al-page>
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

-- One row per member who is not the owner. The owner is the team's `owner` field.
table Member {
  id: MemberId @id,
  team: Team.TeamId @ref Team,
  user: User.UserId @ref User,
}

-- Whether the user may see the team: its owner, or one of its members.
canSee : Team.TeamId -> User.UserId -> Query Bool
canSee team who = {
  let found <- Db.get Team team
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq who) |> Db.all
  let owned = Maybe.withDefault False (Maybe.map (\t -> t.owner == who) found)
  Query.succeed (owned || not (List.isEmpty rows))
}

-- The teams the user owns or is a member of, by name.
teamsOf : User.UserId -> Query (List Team)
teamsOf who = {
  let owned <- Db.from Team |> Db.where (Team.owner.eq who) |> Db.all
  let rows <- Db.from Member |> Db.where (Member.user.eq who) |> Db.all
  let joined <- Query.all (List.map (\m -> Db.get Team m.team) rows)
  Query.succeed (List.sortByText (\t -> t.name) (owned ++ List.filterMap (\x -> x) joined))
}

-- Makes the user a member of the team, unless they own it or already are one.
addMember : Team.TeamId -> User.UserId -> Task Unit
addMember team who = {
  let found <- Db.get Team team
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq who) |> Db.all
  let isOwner = Maybe.withDefault False (Maybe.map (\t -> t.owner == who) found)
  if isOwner || not (List.isEmpty rows) then Task.succeed () else {
    let _ <- Db.insert Member { team: team, user: who }
    Task.succeed ()
  }
}

-- Removes the user's membership rows of the team.
removeMember : Team.TeamId -> User.UserId -> Task Unit
removeMember team who = {
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq who) |> Db.all
  let _ <- Task.all (List.map (\r -> Db.delete Member r.id) rows)
  Task.succeed ()
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

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
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
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Teams.Index {}), user: Just me })
}

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Teams.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

--- file: app/Teams/Index.alder ---
module Teams.Index

import Team
import Member
import User
import Layout

page "/teams" { } requires user

type Model = { teams: List Team }

load : Params -> Session -> Query Model
load params session = {
  let teams <- Member.teamsOf session.user
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
    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New team" />
        <al-button type="submit">Create team</al-button>
      </form>
    </al-card>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "creating a team lists it" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let _ <- Test.submit NewTeam { name: Text.NonEmpty "Crew" }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts ".name" page.doc == ["Crew"])
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Team
import Member
import Invite
import User
import Layout

page "/teams/{id}" { id: Team.TeamId } requires user

type Model = { team: Team, members: List User }

-- Only the owner and members see the team; anyone else gets a 404.
load : Params -> Session -> Query Model
load params session = {
  let allowed <- Member.canSee params.id session.user
  if allowed then loadTeam params.id else Query.notFound
}

loadTeam : Team.TeamId -> Query Model
loadTeam id = {
  let found <- Db.get Team id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq id) |> Db.all
      let others <- Query.all (List.map (\r -> Db.get User r.user) rows)
      let owner <- Db.get User team.owner
      let ownerList = Maybe.toList owner
      let memberList = List.sortByText (\u -> u.name) (List.filterMap (\u -> u) others)
      Query.succeed { team: team, members: ownerList ++ memberList },
    },
  }
}

form NewInvite = {
  email: Text.Email,
}

msg Remove { user: User.UserId }
msg SendInvite form NewInvite

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  -- Only the owner removes, and never the owner: anything else is a 404 and changes nothing.
  Remove p -> {
    if session.user != model.team.owner || p.user == model.team.owner then Task.notFound else {
      let _ <- Member.removeMember model.team.id p.user
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    }
  },
  SendInvite p -> {
    if session.user != model.team.owner then Task.notFound else {
      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
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
          {if m.id == model.team.owner then <span class="role">owner</span> else if session.user == model.team.owner then <button class="remove" click={Remove { user: m.id }}>Remove</button> else Html.none}
        </li>}
    </ul>
    {if session.user == model.team.owner then <al-card><form submit={SendInvite}><al-input field="email" label="Invite by email" /><al-button type="submit">Invite</al-button></form></al-card> else Html.none}
    <p><a href={Teams.Index {}}>All teams</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a team's page is not found for a user who is not a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Stranger", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

test "a member can open the team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just bob }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == Shown)
}

test "the owner invites a user by email" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let waiting <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "new@example.com")) |> Db.count
  Task.succeed ({ answer: r.answer, waiting: waiting } == { answer: Redirected (Teams.Show { id: team }), waiting: 1 })
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let alice <- Db.insert User { email: Text.Email "alice@example.com", name: Text.NonEmpty "Alice", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: alice }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just alice }
  let r <- Test.click (Remove { user: bob })
  let left <- Db.from Member |> Db.where (Member.user.eq bob) |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: PageMissing, left: 1 })
}

test "the owner removes a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: bob })
  let left <- Db.from Member |> Db.where (Member.user.eq bob) |> Db.count
  Task.succeed ({ before: Test.texts ".name" page.doc, answer: r.answer, left: left } == { before: ["Owner", "Bob"], answer: Redirected (Teams.Show { id: team }), left: 0 })
}

-- The owner's own row has no Remove button, and the server refuses the removal anyway.
test "the owner cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let buttons = Test.count ".remove" page.doc
  let model <- loadTeam team
  let r <- Test.run (update (Remove { user: owner }) model { user: owner })
  Task.succeed ({ buttons: buttons, refused: Result.toMaybe r.value == Nothing } == { buttons: 1, refused: True })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Invite
import Team
import Member
import User
import Layout

page "/invites" { } requires user

type Waiting = { id: Invite.InviteId, teamId: Team.TeamId, teamName: Text }

type Model = { invites: List Waiting }

load : Params -> Session -> Query Model
load params session = {
  let mine <- waitingFor session.user
  Query.succeed { invites: mine }
}

waitingFor : User.UserId -> Query (List Waiting)
waitingFor who = {
  let me <- Db.get User who
  match me {
    Nothing -> Query.succeed [],
    Just u -> {
      let invites <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let rows <- Query.all (List.map waitingRow invites)
      Query.succeed (List.filterMap (\r -> r) rows)
    },
  }
}

waitingRow : Invite -> Query (Maybe Waiting)
waitingRow i = {
  let found <- Db.get Team i.team
  match found {
    Nothing -> Query.succeed Nothing,
    Just t -> Query.succeed (Just { id: i.id, teamId: t.id, teamName: t.name }),
  }
}

-- The invite, if it is waiting for this user; otherwise not found.
ownWaiting : Invite.InviteId -> User.UserId -> Query Invite
ownWaiting id who = {
  let me <- Db.get User who
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let found <- Db.from Invite |> Db.where (Invite.id.eq id) |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.first
      match found {
        Nothing -> Query.notFound,
        Just inv -> Query.succeed inv,
      }
    },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let inv <- ownWaiting p.id session.user
    let _ <- Member.addMember inv.team session.user
    let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
    Task.succeed (Redirect (Teams.Show { id: inv.team }))
  },
  Decline p -> {
    let inv <- ownWaiting p.id session.user
    let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
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
          <al-button variant="secondary" size="sm" click={Accept { id: w.id }}>Accept</al-button>
          <al-button variant="ghost" size="sm" click={Decline { id: w.id }}>Decline</al-button>
        </li>}
    </ul>
    <p><a href={Teams.Index {}}>Teams</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: inv })
  let page <- Test.get (Teams.Index {})
  let open <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: r.answer, listed: Test.texts ".name" page.doc, open: open.answer } == { answer: Redirected (Teams.Show { id: team }), listed: ["Crew"], open: Shown })
}

test "declining an invite leaves the team closed" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: inv })
  let page <- Test.get (Teams.Index {})
  let open <- Test.open (Teams.Show { id: team })
  let answered <- Db.get Invite inv
  Task.succeed ({ answer: r.answer, listed: Test.texts ".name" page.doc, open: open.answer, declined: Maybe.map (\i -> i.status) answered == Just Declined } == { answer: Redirected (Invites.Index {}), listed: [], open: PageMissing, declined: True })
}

test "an invite that is not yours cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.login { user: Just other }
  let r <- Test.click (Accept { id: inv })
  let joined <- Db.from Member |> Db.where (Member.user.eq other) |> Db.count
  Task.succeed ({ answer: r.answer, joined: joined } == { answer: PageMissing, joined: 0 })
}
