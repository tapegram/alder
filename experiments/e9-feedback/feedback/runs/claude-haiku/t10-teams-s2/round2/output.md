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

-- In a test, `Test.fake Password` stands in for the adapter: it accepts any user who has a row.
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

-- Submitting this form checks the password through the Password adapter, so no test here posts
-- it. The tests below look at the page alone.
test "the login page shows the form" = {
  let page <- Test.get (Auth.Login {})
  Task.succeed ({ heading: Test.text "h1" page.doc, forms: Test.count "form" page.doc } == { heading: Just "Log in", forms: 1 })
}

--- file: app/Invite.alder ---
module Invite

import Team
import User

type InviteId = brand Text

type Status = Pending | Accepted | Declined

table Invite {
  id: InviteId @id,
  team: Team.TeamId @ref Team,
  email: Text.Email,
  status: Status = Pending,
}

-- An invite made without a status is pending: the default applies on insert.
test "a new invite is pending" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let id <- Db.insert Invite { team: crew, email: Text.Email "ann@example.com" }
  let after <- Db.get Invite id
  Task.succeed (Maybe.map (\i -> i.status) after == Just Pending)
}

-- An invite refers to its team with the default rule, @restrict: a team with an invite cannot be
-- deleted, and the invite stays.
test "a team with an invite cannot be deleted" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Invite { team: crew, email: Text.Email "ann@example.com" }
  let r <- Test.run (Db.delete Team crew)
  let left <- Db.from Invite |> Db.count
  Task.succeed ({ deleted: Result.toMaybe r.value, invites: left } == { deleted: Nothing, invites: 1 })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import Invite
import Team
import Member
import User
import Layout

page "/invites" { } requires user

type Waiting = { id: Invite.InviteId, team: Text }

type Model = { invites: List Waiting }

toWaiting : Invite -> Team -> Waiting
toWaiting invite team = { id: invite.id, team: team.name }

waitingOf : Invite -> Query (Maybe Waiting)
waitingOf invite = Query.map (\t -> Maybe.map (toWaiting invite) t) (Db.get Team invite.team)

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  match found {
    Nothing -> Query.notFound,
    Just me -> {
      let invites <- Db.from Invite |> Db.where (Invite.email.eq me.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let rows <- Query.all (List.map waitingOf invites)
      Query.succeed { invites: List.filterMap (\r -> r) rows }
    },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

-- The invite must be waiting and addressed to the session's user; otherwise it is a 404.
waiting : Invite.InviteId -> User.UserId -> Task Invite
waiting id user = {
  let found <- Db.get Invite id
  let me <- Db.get User user
  let mine = Maybe.map (\u -> u.email) me
  match found {
    Just inv -> if mine == Just (inv.email) && inv.status == Pending then Task.succeed inv else Task.notFound,
    Nothing -> Task.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let inv <- waiting p.id session.user
    let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
    let _ <- Member.add inv.team session.user
    Task.succeed (Redirect (Teams.Show { id: inv.team }))
  },
  Decline p -> {
    let inv <- waiting p.id session.user
    let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
    Task.succeed (Redirect (Invites.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for i in model.invites ->
        <li>
          <span class="team">{i.team}</span>
          <button click={Accept { id: i.id }}>Accept</button>
          <button click={Decline { id: i.id }}>Decline</button>
        </li>}
    </ul>
    <a href={Teams.Index {}}>Teams</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a visitor is sent to log in from the invites page" = {
  let r <- Test.open (Invites.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

-- Only the invites waiting for my email, and not the declined ones, are listed.
test "the waiting invites to my email are listed" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let club <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let grove <- Db.insert Team { name: Text.NonEmpty "Grove", owner: owner }
  let _ <- Db.insert Invite { team: crew, email: Text.Email "ann@example.com", status: Pending }
  let _ <- Db.insert Invite { team: club, email: Text.Email "ann@example.com", status: Declined }
  let _ <- Db.insert Invite { team: grove, email: Text.Email "bob@example.com", status: Pending }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Invites.Index {})
  Task.succeed ({ teams: Test.texts "#invites .team" page.doc, count: Test.count "#invites li" page.doc } == { teams: ["Crew"], count: 1 })
}

-- Accepting makes me a member: the team is then on my list and its page opens.
test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let yes <- Db.insert Invite { team: crew, email: Text.Email "ann@example.com" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: yes })
  let teams <- Test.get (Teams.Index {})
  let crewPage <- Test.open (Teams.Show { id: crew })
  Task.succeed ({ answer: r.answer, title: Test.text "#name" r.doc, listed: Test.texts "#teams .name" teams.doc, shown: crewPage.answer, members: Test.texts "#members .name" crewPage.doc } == { answer: Redirected (Teams.Show { id: crew }), title: Just "Crew", listed: ["Crew"], shown: Shown, members: ["Owner", "Ann"] })
}

-- Declining takes the invite off the list, and the team stays closed to me.
test "declining removes the invite and keeps the team closed" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let club <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let no <- Db.insert Invite { team: club, email: Text.Email "ann@example.com" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: no })
  let teams <- Test.get (Teams.Index {})
  let closed <- Test.open (Teams.Show { id: club })
  Task.succeed ({ answer: r.answer, waiting: Test.count "#invites li" r.doc, listed: Test.count "#teams li" teams.doc, shown: closed.answer } == { answer: Redirected (Invites.Index {}), waiting: 0, listed: 0, shown: PageMissing })
}

-- An invite addressed to someone else cannot be answered by me: the answer is a 404 and the
-- invite stays waiting.
test "an invite to someone else cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let club <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let theirs <- Db.insert Invite { team: club, email: Text.Email "bob@example.com" }
  let accept <- Test.run (update (Accept { id: theirs }) { invites: [] } { user: ann })
  let decline <- Test.run (update (Decline { id: theirs }) { invites: [] } { user: ann })
  let after <- Db.get Invite theirs
  Task.succeed ({ accept: accept.value, decline: decline.value, status: Maybe.map (\i -> i.status) after } == { accept: Err "not found", decline: Err "not found", status: Just Pending })
}

-- An invite that has been answered is no longer waiting, so it cannot be answered again.
test "an answered invite cannot be answered again" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let yes <- Db.insert Invite { team: crew, email: Text.Email "ann@example.com" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.click (Accept { id: yes })
  let again <- Test.run (update (Decline { id: yes }) { invites: [] } { user: ann })
  let once <- Test.run (update (Accept { id: yes }) { invites: [] } { user: ann })
  let after <- Db.get Invite yes
  Task.succeed ({ again: again.value, once: once.value, status: Maybe.map (\i -> i.status) after } == { again: Err "not found", once: Err "not found", status: Just Accepted })
}

-- The owner accepting an invite to their own team adds no membership: the owner has no Member row.
test "the owner accepting their own invite adds no membership" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let mine <- Db.insert Invite { team: crew, email: Text.Email "owner@example.com" }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: mine })
  let members <- Db.from Member |> Db.where (Member.team.eq crew) |> Db.count
  Task.succeed ({ answer: r.answer, members: members } == { answer: Redirected (Teams.Show { id: crew }), members: 0 })
}

-- Two invites to one team, both accepted by the same person: one membership.
test "accepting two invites to one team adds one membership" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let first <- Db.insert Invite { team: crew, email: Text.Email "ann@example.com" }
  let second <- Db.insert Invite { team: crew, email: Text.Email "ann@example.com" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.click (Accept { id: first })
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.click (Accept { id: second })
  let members <- Db.from Member |> Db.where (Member.team.eq crew) |> Db.where (Member.user.eq ann) |> Db.count
  Task.succeed (members == 1)
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

-- Makes `user` a member of `team`, unless they already are or already own it. An accepted
-- invite calls this, so the same person accepting twice, or the owner accepting, adds no row.
add : Team.TeamId -> User.UserId -> Task Unit
add team user = {
  let owner <- Db.get Team team
  let existing <- Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.count
  let isOwner = (Maybe.map (\t -> t.owner == user) owner) ?? False
  let _ <- if isOwner || existing > 0 then Task.succeed () else Task.map (\m -> ()) (Db.insert Member { team: team, user: user })
  Task.succeed ()
}

test "adding a member adds one row" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- add crew ann
  let rows <- Db.from Member |> Db.where (Member.team.eq crew) |> Db.where (Member.user.eq ann) |> Db.count
  Task.succeed (rows == 1)
}

test "the owner is not added as a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- add crew owner
  let rows <- Db.from Member |> Db.where (Member.team.eq crew) |> Db.count
  Task.succeed (rows == 0)
}

test "adding a member twice adds one row" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- add crew ann
  let _ <- add crew ann
  let rows <- Db.from Member |> Db.where (Member.team.eq crew) |> Db.count
  Task.succeed (rows == 1)
}

-- Another member of the same team does not stop a new one from joining.
test "another member does not block a new one" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- add crew ann
  let _ <- add crew bob
  let bobRows <- Db.from Member |> Db.where (Member.team.eq crew) |> Db.where (Member.user.eq bob) |> Db.count
  Task.succeed (bobRows == 1)
}

-- A membership of another team does not stop joining this one.
test "membership of another team does not block" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let club <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- add crew ann
  let _ <- add club ann
  let inClub <- Db.from Member |> Db.where (Member.team.eq club) |> Db.where (Member.user.eq ann) |> Db.count
  Task.succeed (inClub == 1)
}

-- The team reference is @restrict: a team with a member cannot be deleted.
test "a team with a member cannot be deleted" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- add crew ann
  let r <- Test.run (Db.delete Team crew)
  let left <- Db.from Member |> Db.count
  Task.succeed ({ deleted: Result.toMaybe r.value, members: left } == { deleted: Nothing, members: 1 })
}

-- The user reference is @restrict: a user who is a member cannot be deleted.
test "a user who is a member cannot be deleted" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- add crew ann
  let r <- Test.run (Db.delete User ann)
  let left <- Db.from Member |> Db.count
  Task.succeed ({ deleted: Result.toMaybe r.value, members: left } == { deleted: Nothing, members: 1 })
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

-- The owner reference is @restrict: a user who owns a team cannot be deleted while it exists.
test "a user who owns a team cannot be deleted" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let r <- Test.run (Db.delete User owner)
  let left <- Db.from Team |> Db.count
  Task.succeed ({ deleted: Result.toMaybe r.value, teams: left } == { deleted: Nothing, teams: 1 })
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
  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined <- Query.all (List.map (\m -> Db.get Team m.team) memberships)
  let teams = List.append owned (List.filterMap (\t -> t) joined)
  Query.succeed { teams: List.sortByText (\t -> t.name) teams }
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
      <Input field="name" label="New team" />
      <Errors field="name" />
      <button>Create team</button>
    </form>
    <a href={Invites.Index {}}>Invites</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a visitor is sent to log in from the teams page" = {
  let r <- Test.open (Teams.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

-- Teams I own or am a member of are listed by name; another owner's team I am not in is not.
test "the teams are listed by name, owned or joined" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zebra", owner: me }
  let apple <- Db.insert Team { name: Text.NonEmpty "Apple", owner: other }
  let mango <- Db.insert Team { name: Text.NonEmpty "Mango", owner: other }
  let _ <- Db.insert Member { team: apple, user: me }
  let _ <- Db.insert Member { team: mango, user: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Apple", "Zebra"])
}

test "creating a team opens its page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Crew" }
  let created <- Db.from Team |> Db.first
  Task.succeed ({ page: Test.text "#name" r.doc, owned: (Maybe.map (\t -> t.owner == me) created) ?? False, opens: (Maybe.map (\t -> r.answer == Redirected (Teams.Show { id: t.id })) created) ?? False } == { page: Just "Crew", owned: True, opens: True })
}

test "logging out from the teams page goes to log in" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ answer: out.answer, user: out.session.user } == { answer: Redirected (Auth.Login {}), user: Nothing })
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

-- The team is shown to its owner and its members only; anyone else gets a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let mine <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.where (Member.user.eq session.user) |> Db.count
      if team.owner == session.user || mine > 0 then loadTeam team else Query.notFound
    },
  }
}

-- The owner is listed first, then the members ordered by name.
loadTeam : Team -> Query Model
loadTeam team = {
  let owner <- Db.get User team.owner
  let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
  let users <- Query.all (List.map (\m -> Db.get User m.user) rows)
  let others = List.sortByText (\u -> u.name) (List.filterMap (\u -> u) users)
  match owner {
    Nothing -> Query.notFound,
    Just o -> Query.succeed { team: team, members: ([o] ++ others) },
  }
}

form NewInvite = {
  email: Text.Email,
}

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

-- Only the owner may invite or remove; anyone else is a 404 and nothing changes.
ownerOnly : Model -> Session -> Task Unit
ownerOnly model session = if session.user == model.team.owner then Task.succeed () else Task.notFound

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p -> {
    let _ <- ownerOnly model session
    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
    Task.succeed (Redirect (Teams.Show { id: model.team.id }))
  },
  Remove p -> {
    let _ <- ownerOnly model session
    let _ <- if p.user == model.team.owner then Task.notFound else Task.succeed ()
    let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq p.user) |> Db.all
    let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
    Task.succeed Rerender
  },
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
        <Input field="email" label="Invite by email" />
        <Errors field="email" />
        <button>Invite</button>
      </form>
    else Html.none}
    <a href={Teams.Index {}}>Teams</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a visitor is sent to log in from a team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let r <- Test.open (Teams.Show { id: crew })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "an unknown team is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let _ <- Test.login { user: Just owner }
  let r <- Test.open (Teams.Show { id: Team.TeamId "nope" })
  Task.succeed (r.answer == PageMissing)
}

-- A user who is not a member is not found, even when the team has members and the user is a
-- member of some other team.
test "a team is not found for a user who is not a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let outsider <- Db.insert User { email: Text.Email "outsider@example.com", name: Text.NonEmpty "Outsider", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let other <- Db.insert Team { name: Text.NonEmpty "Other", owner: owner }
  let _ <- Db.insert Member { team: crew, user: ann }
  let _ <- Db.insert Member { team: other, user: outsider }
  let _ <- Test.login { user: Just outsider }
  let r <- Test.open (Teams.Show { id: crew })
  Task.succeed (r.answer == PageMissing)
}

-- A member sees the team and only its own members, not those of another team.
test "a member sees the team and only its own members" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let carol <- Db.insert User { email: Text.Email "carol@example.com", name: Text.NonEmpty "Carol", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let other <- Db.insert Team { name: Text.NonEmpty "Other", owner: owner }
  let _ <- Db.insert Member { team: crew, user: ann }
  let _ <- Db.insert Member { team: other, user: carol }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: crew })
  Task.succeed ({ names: Test.texts "#members .name" page.doc, title: Test.text "#name" page.doc } == { names: ["Owner", "Ann"], title: Just "Crew" })
}

test "the owner is listed first, then the members by name" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: crew, user: bob }
  let _ <- Db.insert Member { team: crew, user: ann }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: crew })
  Task.succeed (Test.texts "#members .name" page.doc == ["Owner", "Ann", "Bob"])
}

-- The owner sees a Remove button beside each member but not beside themselves, and the invite form.
test "the owner sees Remove beside each member and the invite form" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: crew, user: ann }
  let _ <- Db.insert Member { team: crew, user: bob }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Teams.Show { id: crew })
  Task.succeed ({ remove: Test.count ".remove" page.doc, inputs: Test.count "input[name=email]" page.doc } == { remove: 2, inputs: 1 })
}

-- A member sees no Remove button and no invite form.
test "a member sees no Remove button and no invite form" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: crew, user: ann }
  let _ <- Db.insert Member { team: crew, user: bob }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: crew })
  Task.succeed ({ remove: Test.count ".remove" page.doc, inputs: Test.count "input[name=email]" page.doc } == { remove: 0, inputs: 0 })
}

-- The owner removes one member through the page: only that member's membership of this team goes.
test "the owner removes a member and nothing else" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let club <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Db.insert Member { team: crew, user: ann }
  let _ <- Db.insert Member { team: crew, user: bob }
  let _ <- Db.insert Member { team: club, user: ann }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: crew })
  let r <- Test.click (Remove { user: ann })
  let annCrew <- Db.from Member |> Db.where (Member.team.eq crew) |> Db.where (Member.user.eq ann) |> Db.count
  let annClub <- Db.from Member |> Db.where (Member.team.eq club) |> Db.where (Member.user.eq ann) |> Db.count
  let bobCrew <- Db.from Member |> Db.where (Member.team.eq crew) |> Db.where (Member.user.eq bob) |> Db.count
  Task.succeed ({ answer: r.answer, names: Test.texts "#members .name" r.doc, annCrew: annCrew, annClub: annClub, bobCrew: bobCrew } == { answer: Rerendered, names: ["Owner", "Bob"], annCrew: 0, annClub: 1, bobCrew: 1 })
}

-- A member cannot remove another member: the server answers 404 and nothing changes.
test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let model <- load { id: team } { user: ann }
  let r <- Test.run (update (Remove { user: bob }) model { user: ann })
  let left <- Db.from Member |> Db.where (Member.user.eq bob) |> Db.count
  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })
}

-- The owner cannot remove themselves, even when the server receives the message.
test "the owner cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let model <- load { id: team } { user: owner }
  let r <- Test.run (update (Remove { user: owner }) model { user: owner })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })
}

-- A member cannot invite: the server answers 404 and no invite is stored.
test "a member cannot invite" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let model <- load { id: team } { user: ann }
  let r <- Test.run (update (SendInvite { form: { email: Text.Email "carl@example.com" } }) model { user: ann })
  let invites <- Db.from Invite |> Db.count
  Task.succeed ({ value: r.value, invites: invites } == { value: Err "not found", invites: 0 })
}

-- The owner invites by email through the form: the invite is stored waiting, and the page reopens.
test "the owner invites by email" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: crew })
  let r <- Test.submit NewInvite { email: Text.Email "carl@example.com" }
  let waiting <- Db.from Invite |> Db.where (Invite.team.eq crew) |> Db.where (Invite.status.eq Pending) |> Db.count
  let mail <- Db.from Invite |> Db.where (Invite.email.eq (Text.Email "carl@example.com")) |> Db.count
  Task.succeed ({ answer: r.answer, waiting: waiting, mail: mail } == { answer: Redirected (Teams.Show { id: crew }), waiting: 1, mail: 1 })
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
