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

test "a wrong email is refused on the form" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in with a known email goes to my teams" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Teams.Index {}), user: Just me })
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

--- file: app/Invites/Index.alder ---
module Invites.Index

import Invite
import Team
import Member
import User
import Layout

page "/invites" { } requires user

type Waiting = { id: Invite.InviteId, teamName: Text.NonEmpty }

type Model = { invites: List Waiting }

-- The waiting invite, with its team's name; nothing when the team is gone.
waitingFor : List Team -> Invite -> Maybe Waiting
waitingFor teams invite = Maybe.map (\t -> { id: invite.id, teamName: t.name }) (List.find (\t -> t.id == invite.team) teams)

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let invites <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.all
      Query.succeed { invites: List.filterMap (waitingFor teams) invites }
    },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

-- The invite a message names, when it is waiting and was sent to the session's user's email;
-- otherwise the message is a 404.
answerable : Invite.InviteId -> Session -> Task Invite
answerable id session = {
  let found <- Db.get Invite id
  match found {
    Nothing -> Task.notFound,
    Just invite -> {
      let me <- Db.get User session.user
      match me {
        Nothing -> Task.notFound,
        Just u -> if invite.email == u.email && invite.status == Pending then Task.succeed invite else Task.notFound,
      }
    },
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let invite <- answerable p.id session
    let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
    let _ <- Db.insert Member { team: invite.team, user: session.user }
    Task.succeed (Redirect (Teams.Show { id: invite.team }))
  },
  Decline p -> {
    let invite <- answerable p.id session
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

-- A visitor is sent to log in: the page requires a user.
test "a visitor is sent to log in from the invites page" = {
  let visitor <- Test.open (Invites.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com", status: Pending }
  let _ <- Test.login { user: Just me }
  let before <- Test.open (Teams.Show { id: team })
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: invite })
  let shown <- Test.open (Teams.Show { id: team })
  let after <- Db.get Invite invite
  Task.succeed ({ before: before.answer, accepted: r.answer, shown: shown.answer, name: Test.text "#name" shown.doc, status: Maybe.map (\i -> i.status) after } == { before: PageMissing, accepted: Redirected (Teams.Show { id: team }), shown: Shown, name: Just "Garden", status: Just Accepted })
}

test "declining an invite keeps the team closed" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com", status: Pending }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: invite })
  let shown <- Test.open (Teams.Show { id: team })
  let after <- Db.get Invite invite
  Task.succeed ({ answer: r.answer, waiting: Test.count "#invites li" r.doc, shown: shown.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Invites.Index {}), waiting: 0, shown: PageMissing, status: Just Declined })
}

test "an invite to another email cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com", status: Pending }
  let model : Model = { invites: [] }
  let r <- Test.run (update (Accept { id: invite }) model { user: stranger })
  let after <- Db.get Invite invite
  Task.succeed ({ value: r.value, status: Maybe.map (\i -> i.status) after } == { value: Err "not found", status: Just Pending })
}

-- A stranger cannot decline someone else's invite either.
test "a stranger cannot decline an invite" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com", status: Pending }
  let _ <- me
  let model : Model = { invites: [] }
  let r <- Test.run (update (Decline { id: invite }) model { user: stranger })
  let after <- Db.get Invite invite
  Task.succeed ({ value: r.value, status: Maybe.map (\i -> i.status) after } == { value: Err "not found", status: Just Pending })
}

-- An invite that was answered cannot be answered again: the second Accept is a 404 and adds no
-- second membership.
test "an answered invite cannot be answered again" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com", status: Pending }
  let model : Model = { invites: [] }
  let first <- Test.run (update (Accept { id: invite }) model { user: me })
  let second <- Test.run (update (Accept { id: invite }) model { user: me })
  Task.succeed ({ first: first.value, second: second.value } == { first: Ok (Redirect (Teams.Show { id: team })), second: Err "not found" })
}

-- An id that names no invite cannot be answered.
test "an unknown invite cannot be accepted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let model : Model = { invites: [] }
  let r <- Test.run (update (Accept { id: Invite.InviteId "nope" }) model { user: me })
  Task.succeed (r.value == Err "not found")
}

-- The list shows only my waiting invites, each with its own team's name.
test "only my waiting invites are listed, with their team's name" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let garden <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let other <- Db.insert Team { name: Text.NonEmpty "Other", owner: owner }
  let _ <- Db.insert Invite { team: garden, email: Text.Email "me@example.com", status: Pending }
  let _ <- Db.insert Invite { team: other, email: Text.Email "stranger@example.com", status: Pending }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Invites.Index {})
  Task.succeed ({ teams: Test.texts "#invites .team" page.doc, count: Test.count "#invites li" page.doc } == { teams: ["Garden"], count: 1 })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
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

import Team
import Member
import User
import Layout

page "/teams" { } requires user

type Model = { teams: List Team }

load : Params -> Session -> Query Model
load params session = {
  let teams <- Db.from Team |> Db.orderBy Team.name.asc |> Db.all
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let mine = List.filter (\t -> t.owner == session.user || List.any (\m -> m.team == t.id) memberships) teams
  Query.succeed { teams: mine }
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
    <form submit={Create}>
      <Input field="name" label="New team name" />
      <Errors field="name" />
      <button>Create team</button>
    </form>
    <ul id="teams">
      {for t in model.teams ->
        <li><a class="name" href={Teams.Show { id: t.id }}>{t.name}</a></li>}
    </ul>
  </Layout.Page>

test "the teams I belong to are listed, by name" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zeta", owner: me }
  let joined <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let _ <- Db.insert Team { name: Text.NonEmpty "Other", owner: owner }
  let _ <- Db.insert Member { team: joined, user: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Alpha", "Zeta"])
}

-- A team that another user joined, and that I neither own nor belong to, is not listed to me.
test "a team I am not part of is not listed" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.count "#teams li" page.doc == 0)
}

test "creating a team opens its page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Garden" }
  Task.succeed (Test.text "#name" r.doc == Just "Garden")
}

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Teams.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Team
import Member
import User
import Invite
import Layout

page "/teams/{id}" { id: Team.TeamId } requires user

type Model = { team: Team, members: List User }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> viewable team session,
  }
}

-- A team is shown to its owner and its members only; anyone else gets a 404.
viewable : Team -> Session -> Query Model
viewable team session = {
  let memberRows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
  let mine = team.owner == session.user || List.any (\m -> m.user == session.user) memberRows
  if mine then {
    let users <- Db.from User |> Db.where (User.id.in ([team.owner] ++ List.map (\m -> m.user) memberRows)) |> Db.orderBy User.name.asc |> Db.all
    let owner = List.filter (\u -> u.id == team.owner) users
    let others = List.filter (\u -> u.id != team.owner) users
    Query.succeed { team: team, members: owner ++ others }
  } else Query.notFound
}

form NewInvite = {
  email: Text.Email,
}

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

-- Only the team's owner may change the team: anyone else gets a 404.
ownerOnly : Team -> Session -> Task Unit
ownerOnly team session = if session.user == team.owner then Task.succeed () else Task.notFound

-- The owner is never removed: the request is refused and nothing changes.
notSelf : Team -> User.UserId -> Task Unit
notSelf team user = if user == team.owner then Task.notFound else Task.succeed ()

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p -> {
    let _ <- ownerOnly model.team session
    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email, status: Pending }
    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
  },
  Remove p -> {
    let _ <- ownerOnly model.team session
    let _ <- notSelf model.team p.user
    let found <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.first
    match found {
      Just m -> {
        let _ <- Db.delete Member m.id
        Task.succeed Rerender
      },
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <h2>Members</h2>
    <ul id="members">
      {for m in model.members ->
        <li>
          <span class="name">{m.name}</span>
          {if m.id == model.team.owner then <span class="role">owner</span> else Html.none}
          {if session.user == model.team.owner && m.id != model.team.owner then <button class="remove" click={Remove { user: m.id }}>Remove</button> else Html.none}
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
    <a href={Teams.Index {}}>Back to teams</a>
  </Layout.Page>

test "a visitor is sent to log in from a team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let visitor <- Test.open (Teams.Show { id: team })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "a stranger's team page is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

-- A member of some other team has no access to this one.
test "a member of another team cannot see this team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let other <- Db.insert Team { name: Text.NonEmpty "Other", owner: owner }
  let _ <- Db.insert Member { team: other, user: ann }
  let _ <- Test.login { user: Just ann }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

-- A stranger is not let in because some other user is a member of this team.
test "a stranger is not let in by another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just sam }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

test "a member can open the team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just ann }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == Shown)
}

-- The owner comes first, then the members by name; a user who is not in the team is not listed.
test "the members are listed, the owner first, then by name" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Db.insert User { email: Text.Email "zed@example.com", name: Text.NonEmpty "Zed", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  Task.succeed (Test.texts "#members .name" page.doc == ["Owen", "Ann", "Bob"])
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Garden", owner: owner }, members: [] }
  let r <- Test.run (update (Remove { user: bob }) model { user: ann })
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, rows: rows } == { value: Err "not found", rows: 2 })
}

-- A member cannot invite either: the server answers 404 and writes no invite.
test "a member cannot invite" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Garden", owner: owner }, members: [] }
  let r <- Test.run (update (SendInvite { form: { email: Text.Email "bob@example.com" } }) model { user: ann })
  let waiting <- Db.from Invite |> Db.count
  Task.succeed ({ value: r.value, waiting: waiting } == { value: Err "not found", waiting: 0 })
}

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Garden", owner: owner }, members: [] }
  let removed <- Test.run (update (Remove { user: ann }) model { user: owner })
  let selfRemoval <- Test.run (update (Remove { user: owner }) model { user: owner })
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ removed: removed.value, self: selfRemoval.value, rows: rows } == { removed: Ok Rerender, self: Err "not found", rows: 0 })
}

-- Even if a member row for the owner exists, the owner's own removal is refused and the row stays.
test "the owner's own removal is refused even with a member row" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Db.insert Member { team: team, user: owner }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Garden", owner: owner }, members: [] }
  let r <- Test.run (update (Remove { user: owner }) model { user: owner })
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, rows: rows } == { value: Err "not found", rows: 2 })
}

-- Removing someone who is not a member of this team changes nothing, even if they are a member
-- of this team's other members' list.
test "a removal of someone not in this team changes nothing" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Garden", owner: owner }, members: [] }
  let r <- Test.run (update (Remove { user: bob }) model { user: owner })
  let rows <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, rows: rows } == { value: Err "not found", rows: 1 })
}

-- Removing a user who is a member of another team does not remove their row there.
test "a removal of someone in another team changes nothing" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let other <- Db.insert Team { name: Text.NonEmpty "Other", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Db.insert Member { team: other, user: bob }
  let model : Model = { team: { id: team, name: Text.NonEmpty "Garden", owner: owner }, members: [] }
  let r <- Test.run (update (Remove { user: bob }) model { user: owner })
  let otherRows <- Db.from Member |> Db.where (Member.team.eq other) |> Db.count
  Task.succeed ({ value: r.value, otherRows: otherRows } == { value: Err "not found", otherRows: 1 })
}

test "the owner removes a member through the page" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.click (Remove { user: ann })
  Task.succeed ({ buttons: Test.count "button.remove" page.doc, answer: r.answer, names: Test.texts "#members .name" r.doc } == { buttons: 1, answer: Rerendered, names: ["Owen"] })
}

test "the owner invites a user by email" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "ann@example.com" }
  let waiting <- Db.from Invite |> Db.where (Invite.status.eq Pending) |> Db.count
  Task.succeed ({ answer: r.answer, waiting: waiting } == { answer: Redirected (Teams.Show { id: team }), waiting: 1 })
}

test "a member sees no invite form" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Garden", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: team })
  Task.succeed (Test.has "input[name=email]" page.doc == False)
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
