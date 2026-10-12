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

-- One row per member who is not the owner: the owner has no Member row.
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
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let visitor <- Test.open (Teams.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ visitor: visitor.answer, heading: Test.text "h1" visitor.doc, out: out.answer, user: out.session.user } == { visitor: Redirected (Auth.Login {}), heading: Just "Log in", out: Redirected (Auth.Login {}), user: Nothing })
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
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined = List.map (\m -> m.team) memberships
  let teams <-
    Db.from Team
      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in joined])
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

    <nav>
      <a href={Invites.Index {}}>Invites</a>
    </nav>

    <ul id="teams">
      {for t in model.teams ->
        <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
    </ul>

    <form submit={Create}>
      <Errors form />
      <Input field="name" label="Team name" />
      <Errors field="name" />
      <button>Create</button>
    </form>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "creating a team makes its creator the owner" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Fresh" }
  let found <- Db.from Team |> Db.first
  let goesToTeam = Maybe.map (\t -> r.answer == Redirected (Teams.Show { id: t.id })) found
  let owner = Maybe.map (\t -> t.owner) found
  Task.succeed ({ goesToTeam: goesToTeam, owner: owner, name: Test.text "#name" r.doc, members: Test.texts "#members .name" r.doc } == { goesToTeam: Just True, owner: Just ann, name: Just "Fresh", members: ["Ann"] })
}

test "an empty team name is rejected" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submitRaw NewTeam [{ name: "name", value: "" }]
  let rows <- Db.from Team |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
}

test "the list has the teams I own or belong to, by name" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zebra", owner: ann }
  let apple <- Db.insert Team { name: Text.NonEmpty "Apple", owner: bob }
  let _ <- Db.insert Team { name: Text.NonEmpty "Other", owner: bob }
  let _ <- Db.insert Member { team: apple, user: ann }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Index {})
  Task.succeed ({ names: Test.texts "#teams .name" page.doc, items: Test.count "#teams li" page.doc } == { names: ["Apple", "Zebra"], items: 2 })
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Team
import Member
import Invite
import User
import Layout

page "/teams/{id}" { id: Team.TeamId } requires user

type Model = {
  team: Team,
  owner: User,
  others: List User,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  match found {
    Just team -> members team session,
    Nothing -> Query.notFound,
  }
}

-- The page is for the team's owner and members only: for anyone else it is a 404.
members : Team -> Session -> Query Model
members team session = {
  let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
  let ids = rows |> List.map (\m -> m.user) |> List.filter (\u -> u != team.owner)
  if team.owner == session.user || List.member session.user ids then roster team ids else Query.notFound
}

roster : Team -> List User.UserId -> Query Model
roster team ids = {
  let owner <- Db.get User team.owner
  let others <- Db.from User |> Db.where (User.id.in ids) |> Db.orderBy User.name.asc |> Db.all
  match owner {
    Just o -> Query.succeed { team: team, owner: o, others: others },
    Nothing -> Query.notFound,
  }
}

form NewInvite = {
  email: Text.Email,
}

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

-- `load` lets members in, so `update` checks again that the sender is the owner: an invite or a
-- Remove from anyone else is a 404. The owner cannot remove themselves.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p -> if model.team.owner != session.user then Task.notFound else {
    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
  },
  Remove p -> if model.team.owner != session.user || p.user == model.team.owner then Task.notFound else {
    let found <-
      Db.from Member
        |> Db.where (Member.team.eq model.team.id)
        |> Db.where (Member.user.eq p.user)
        |> Db.first
    match found {
      Just m -> {
        let _ <- Db.delete Member m.id
        Task.succeed (Redirect (Teams.Show { id: model.team.id }))
      },
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>

    <nav>
      <a href={Teams.Index {}}>Teams</a>
      <a href={Invites.Index {}}>Invites</a>
    </nav>

    <ul id="members">
      <Item user={model.owner} canRemove={False} />
      {for u in model.others -> <Item user={u} canRemove={model.team.owner == session.user} />}
    </ul>

    {if model.team.owner == session.user then
      <form submit={SendInvite}>
        <Errors form />
        <Input field="email" label="Invite by email" />
        <Errors field="email" />
        <button>Invite</button>
      </form>
    else Html.none}

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

Item : { user: User, canRemove: Bool } -> Html Msg
Item props =
  <li>
    <span class="name">{props.user.name}</span>
    {if props.canRemove then <button class="remove" click={Remove { user: props.user.id }}>Remove</button> else Html.none}
  </li>

test "a team's page is not found for a user who is not a member" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Test.login { user: Just bob }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

test "a visitor is sent to log in from a team" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "the owner is listed first, then the members by name" = {
  let zed <- Db.insert User { email: Text.Email "zed@example.com", name: Text.NonEmpty "Zed", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let amy <- Db.insert User { email: Text.Email "amy@example.com", name: Text.NonEmpty "Amy", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: zed }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: amy }
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Teams.Show { id: team })
  Task.succeed ({ name: Test.text "#name" page.doc, names: Test.texts "#members .name" page.doc, items: Test.count "#members li" page.doc } == { name: Just "Core", names: ["Zed", "Amy", "Bob"], items: 3 })
}

test "a member cannot remove another member" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cat <- Db.insert User { email: Text.Email "cat@example.com", name: Text.NonEmpty "Cat", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: cat }
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: cat }) page.model { user: bob })
  let count <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ buttons: Test.count ".remove" page.doc, value: r.value, members: count } == { buttons: 0, value: Err "not found", members: 2 })
}

test "a member cannot invite" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (SendInvite { form: { email: Text.Email "cat@example.com" } }) page.model { user: bob })
  let count <- Db.from Invite |> Db.count
  Task.succeed ({ forms: Test.count "input[name=email]" page.doc, value: r.value, invites: count } == { forms: 0, value: Err "not found", invites: 0 })
}

test "the owner removes a member but cannot remove themselves" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: team })
  let own <- Test.run (update (Remove { user: ann }) page.model { user: ann })
  let removed <- Test.click (Remove { user: bob })
  let count <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  let still <- Db.get Team team
  Task.succeed ({ buttons: Test.count ".remove" page.doc, own: own.value, answer: removed.answer, names: Test.texts "#members .name" removed.doc, members: count, team: Maybe.isJust still } == { buttons: 1, own: Err "not found", answer: Redirected (Teams.Show { id: team }), names: ["Ann"], members: 0, team: True })
}

test "the owner invites by email, whether or not a user has it" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let invites <- Db.from Invite |> Db.where (Invite.team.eq team) |> Db.all
  Task.succeed ({ answer: r.answer, emails: List.map (\i -> i.email) invites, statuses: List.map (\i -> i.status) invites } == { answer: Redirected (Teams.Show { id: team }), emails: [Text.Email "new@example.com"], statuses: [Pending] })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Invite
import Team
import Member
import User
import Layout

page "/invites" { } requires user

type Waiting = { invite: Invite, team: Team }

type Model = { invites: List Waiting }

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Just user -> waitingFor user,
    Nothing -> Query.succeed { invites: [] },
  }
}

waitingFor : User -> Query Model
waitingFor user = {
  let invites <-
    Db.from Invite
      |> Db.where (Invite.email.eq user.email)
      |> Db.where (Invite.status.eq Pending)
      |> Db.all
  let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
  Query.succeed { invites: List.filterMap (withTeam teams) invites }
}

withTeam : List Team -> Invite -> Maybe Waiting
withTeam teams invite = match List.find (\t -> t.id == invite.team) teams {
  Just t -> Just { invite: invite, team: t },
  Nothing -> Nothing,
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let invite <- mine p.id session
    let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
    let already <- isMember invite.team session.user
    let _ <- if already then Task.succeed () else addMember invite.team session.user
    Task.succeed (Redirect (Teams.Show { id: invite.team }))
  },
  Decline p -> {
    let invite <- mine p.id session
    let _ <- Db.update Invite invite.id (\i -> { i | status: Declined })
    Task.succeed (Redirect (Invites.Index {}))
  },
}

-- mine is the invite a message names, when it is to the session user's email and still waiting;
-- otherwise the message is a 404.
mine : Invite.InviteId -> Session -> Task Invite
mine id session = {
  let me <- Db.get User session.user
  let email = Maybe.map (\u -> u.email) me
  let found <- Db.get Invite id
  match found {
    Just invite -> if Just invite.email == email && invite.status == Pending then Task.succeed invite else Task.notFound,
    Nothing -> Task.notFound,
  }
}

-- The owner has no Member row, and nobody gets two.
isMember : Team.TeamId -> User.UserId -> Query Bool
isMember teamId userId = {
  let found <- Db.get Team teamId
  let rows <- Db.from Member |> Db.where (Member.team.eq teamId) |> Db.where (Member.user.eq userId) |> Db.count
  Query.succeed (rows > 0 || Maybe.map (\t -> t.owner) found == Just userId)
}

addMember : Team.TeamId -> User.UserId -> Task Unit
addMember teamId userId = {
  let _ <- Db.insert Member { team: teamId, user: userId }
  Task.succeed ()
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>

    <nav>
      <a href={Teams.Index {}}>Teams</a>
    </nav>

    <ul id="invites">
      {for w in model.invites ->
        <li>
          <span class="team">{w.team.name}</span>
          <button click={Accept { id: w.invite.id }}>Accept</button>
          <button click={Decline { id: w.invite.id }}>Decline</button>
        </li>}
    </ul>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "accepting an invite makes the team visible" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let invite <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: invite })
  let teams <- Test.get (Teams.Index {})
  let after <- Test.get (Invites.Index {})
  Task.succeed ({ waiting: Test.texts "#invites .team" page.doc, answer: r.answer, name: Test.text "#name" r.doc, members: Test.texts "#members .name" r.doc, teams: Test.texts "#teams .name" teams.doc, remaining: Test.count "#invites li" after.doc } == { waiting: ["Core"], answer: Redirected (Teams.Show { id: team }), name: Just "Core", members: ["Ann", "Bob"], teams: ["Core"], remaining: 0 })
}

test "declining an invite leaves the team closed" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let invite <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: invite })
  let teams <- Test.get (Teams.Index {})
  let show <- Test.open (Teams.Show { id: team })
  let count <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, remaining: Test.count "#invites li" r.doc, teams: Test.texts "#teams .name" teams.doc, show: show.answer, members: count } == { answer: Redirected (Invites.Index {}), remaining: 0, teams: [], show: PageMissing, members: 0 })
}

test "an invite to someone else is not listed and cannot be answered" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let invite <- Db.insert Invite { team: team, email: Text.Email "cat@example.com" }
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Invites.Index {})
  let accept <- Test.run (update (Accept { id: invite }) { invites: [] } { user: bob })
  let decline <- Test.run (update (Decline { id: invite }) { invites: [] } { user: bob })
  let found <- Db.get Invite invite
  let count <- Db.from Member |> Db.count
  Task.succeed ({ listed: Test.count "#invites li" page.doc, accept: accept.value, decline: decline.value, status: Maybe.map (\i -> i.status) found, members: count } == { listed: 0, accept: Err "not found", decline: Err "not found", status: Just Pending, members: 0 })
}

test "an invite that is no longer waiting cannot be answered" = {
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let invite <- Db.insert Invite { team: team, email: Text.Email "bob@example.com", status: Declined }
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Invites.Index {})
  let accept <- Test.run (update (Accept { id: invite }) { invites: [] } { user: bob })
  let found <- Db.get Invite invite
  let count <- Db.from Member |> Db.count
  Task.succeed ({ listed: Test.count "#invites li" page.doc, accept: accept.value, status: Maybe.map (\i -> i.status) found, members: count } == { listed: 0, accept: Err "not found", status: Just Declined, members: 0 })
}
