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

-- Logging out from a page that `requires user` ends at the login page either way; only a test
-- of `update` itself tells Redirect from Rerender.
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
-- a row, whatever the password. The Go adapter itself is never called by a test.
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

test "an unknown email is rejected with the form's message" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in stores the user and goes to the teams" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Teams.Index {}), user: Just me, title: Just "Teams" })
}

--- file: app/Invite.alder ---
module Invite

import User
import Team

type InviteId = brand Text

type Status = Pending | Accepted | Declined

table Invite {
  id: InviteId @id,
  team: Team.TeamId @ref Team,
  email: Text.Email,
  status: Status = Pending,
}

-- `team` is `@ref Team` with the default rule, `@restrict`: a team with an invite cannot be
-- deleted, and the invite stays.
test "a team with an invite cannot be deleted" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let r <- Test.run (Db.delete Team team)
  let after <- Db.get Invite inv
  Task.succeed ({ deleted: Result.toMaybe r.value, invite: Maybe.map (\i -> i.email) after } == { deleted: Nothing, invite: Just (Text.Email "bob@example.com") })
}

--- file: app/Invites/Index.alder ---
module Invites.Index

import User
import Team
import Member
import Invite
import Layout

page "/invites" { } requires user

type Model = {
  invites: List Invite,
  teams: List Team,
}

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Just u -> {
      let invites <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
    Nothing -> Query.succeed { invites: [], teams: [] },
  }
}

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

-- mine is the invite a message names, when it is waiting and addressed to the session's
-- user's email; otherwise the message is a 404.
mine : Invite.InviteId -> Session -> Task Invite
mine id session = {
  let me <- Db.get User session.user
  let found <- Db.get Invite id
  match found {
    Just inv -> match me {
      Just u -> if inv.email == u.email && inv.status == Pending then Task.succeed inv else Task.notFound,
      Nothing -> Task.notFound,
    },
    Nothing -> Task.notFound,
  }
}

-- The `False` default of `owns` applies only when the invite's team row is missing, which the
-- @ref on Invite.team (restrict: a team with an invite cannot be deleted) makes impossible:
-- no input can tell it from `True`, so that mutant survives honestly.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let inv <- mine p.id session
    let team <- Db.get Team inv.team
    let owns = Maybe.withDefault False (Maybe.map (\t -> t.owner == session.user) team)
    let existing <- Db.from Member |> Db.where (Member.team.eq inv.team) |> Db.where (Member.user.eq session.user) |> Db.count
    let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
    let _ <- if existing == 0 && not owns then Task.map (\x -> ()) (Db.insert Member { team: inv.team, user: session.user }) else Task.succeed ()
    Task.succeed (Redirect (Teams.Show { id: inv.team }))
  },
  Decline p -> {
    let inv <- mine p.id session
    let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
    Task.succeed (Redirect (Invites.Index {}))
  },
}

teamOf : List Team -> Invite -> Maybe Team
teamOf teams inv = List.find (\t -> t.id == inv.team) teams

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for inv in model.invites -> <Row inv={inv} team={teamOf model.teams inv} />}
    </ul>
  </Layout.Page>

Row : { inv: Invite, team: Maybe Team } -> Html Msg
Row props =
  <li>
    {match props.team {
      Just t -> <span class="team">{t.name}</span>,
      Nothing -> Html.none,
    }}
    <button click={Accept { id: props.inv.id }}>Accept</button>
    <button click={Decline { id: props.inv.id }}>Decline</button>
  </li>

test "accepting an invite makes the team visible, declining does not" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cy <- Db.insert User { email: Text.Email "cy@example.com", name: Text.NonEmpty "Cy", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inviteB <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let inviteC <- Db.insert Invite { team: team, email: Text.Email "cy@example.com" }
  let _ <- Test.login { user: Just bob }
  let before <- Test.open (Teams.Show { id: team })
  let listed <- Test.get (Invites.Index {})
  let acc <- Test.click (Accept { id: inviteB })
  let bobTeams <- Test.get (Teams.Index {})
  let bobPage <- Test.open (Teams.Show { id: team })
  let _ <- Test.login { user: Just cy }
  let _ <- Test.get (Invites.Index {})
  let dec <- Test.click (Decline { id: inviteC })
  let cyInvites <- Test.get (Invites.Index {})
  let cyTeams <- Test.get (Teams.Index {})
  let cyPage <- Test.open (Teams.Show { id: team })
  Task.succeed ({ before: before.answer, listed: Test.texts "#invites .team" listed.doc, accepted: acc.answer, bobTeams: Test.texts "#teams .name" bobTeams.doc, bobPage: bobPage.answer, declined: dec.answer, cyInvites: Test.count "#invites li" cyInvites.doc, cyTeams: Test.count "#teams li" cyTeams.doc, cyPage: cyPage.answer } == { before: PageMissing, listed: ["Core"], accepted: Redirected (Teams.Show { id: team }), bobTeams: ["Core"], bobPage: Shown, declined: Redirected (Invites.Index {}), cyInvites: 0, cyTeams: 0, cyPage: PageMissing })
}

test "an invite that is not the user's or not waiting cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cy <- Db.insert User { email: Text.Email "cy@example.com", name: Text.NonEmpty "Cy", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let stranger <- Test.run (update (Accept { id: inv }) { invites: [], teams: [] } { user: cy })
  let first <- Test.run (update (Accept { id: inv }) { invites: [], teams: [] } { user: bob })
  let again <- Test.run (update (Accept { id: inv }) { invites: [], teams: [] } { user: bob })
  let members <- Db.from Member |> Db.count
  Task.succeed ({ stranger: stranger.value, first: first.value, again: again.value, members: members } == { stranger: Err "not found", first: Ok (Redirect (Teams.Show { id: team })), again: Err "not found", members: 1 })
}

test "a visitor is sent to log in, and the page lists only the user's waiting invites" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let core <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let side <- Db.insert Team { name: Text.NonEmpty "Side", owner: owner }
  let waiting <- Db.insert Invite { team: core, email: Text.Email "bob@example.com" }
  let _ <- Db.insert Invite { team: side, email: Text.Email "cy@example.com" }
  let declined <- Db.insert Invite { team: side, email: Text.Email "bob@example.com" }
  let _ <- Db.update Invite declined (\i -> { i | status: Declined })
  let visitor <- Test.open (Invites.Index {})
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Invites.Index {})
  Task.succeed ({ visitor: visitor.answer, invites: List.map (\i -> i.id) page.model.invites, teams: List.map (\t -> t.name) page.model.teams, shown: Test.texts "#invites .team" page.doc } == { visitor: Redirected (Auth.Login {}), invites: [waiting], teams: [Text.NonEmpty "Core"], shown: ["Core"] })
}

test "accepting adds the user once, and only to that team" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cy <- Db.insert User { email: Text.Email "cy@example.com", name: Text.NonEmpty "Cy", passwordHash: "" }
  let core <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let side <- Db.insert Team { name: Text.NonEmpty "Side", owner: owner }
  let _ <- Db.insert Member { team: core, user: bob }
  let _ <- Db.insert Member { team: side, user: cy }
  let toSide <- Db.insert Invite { team: side, email: Text.Email "bob@example.com" }
  let again <- Db.insert Invite { team: side, email: Text.Email "bob@example.com" }
  let toCore <- Db.insert Invite { team: core, email: Text.Email "owner@example.com" }
  let a <- Test.run (update (Accept { id: toSide }) { invites: [], teams: [] } { user: bob })
  let b <- Test.run (update (Accept { id: again }) { invites: [], teams: [] } { user: bob })
  let c <- Test.run (update (Accept { id: toCore }) { invites: [], teams: [] } { user: owner })
  let sideN <- Db.from Member |> Db.where (Member.team.eq side) |> Db.count
  let coreN <- Db.from Member |> Db.where (Member.team.eq core) |> Db.count
  Task.succeed ({ a: a.value, b: b.value, c: c.value, side: sideN, core: coreN } == { a: Ok (Redirect (Teams.Show { id: side })), b: Ok (Redirect (Teams.Show { id: side })), c: Ok (Redirect (Teams.Show { id: core })), side: 2, core: 1 })
}

test "a declined invite cannot be answered again" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cy <- Db.insert User { email: Text.Email "cy@example.com", name: Text.NonEmpty "Cy", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let other <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let stranger <- Test.run (update (Decline { id: other }) { invites: [], teams: [] } { user: cy })
  let dec <- Test.run (update (Decline { id: inv }) { invites: [], teams: [] } { user: bob })
  let acc <- Test.run (update (Accept { id: inv }) { invites: [], teams: [] } { user: bob })
  let decAgain <- Test.run (update (Decline { id: inv }) { invites: [], teams: [] } { user: bob })
  let members <- Db.from Member |> Db.count
  Task.succeed ({ stranger: stranger.value, dec: dec.value, acc: acc.value, decAgain: decAgain.value, members: members } == { stranger: Err "not found", dec: Ok (Redirect (Invites.Index {})), acc: Err "not found", decAgain: Err "not found", members: 0 })
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

import User
import Team

type MemberId = brand Text

table Member {
  id: MemberId @id,
  team: Team.TeamId @ref Team,
  user: User.UserId @ref User,
}

-- Both references use the default rule, `@restrict`: a team with a member row, or a user who
-- is a member, cannot be deleted, and the row stays.
test "a team with a member cannot be deleted" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let m <- Db.insert Member { team: team, user: bob }
  let r <- Test.run (Db.delete Team team)
  let after <- Db.get Member m
  let teamAfter <- Db.get Team team
  Task.succeed ({ deleted: Result.toMaybe r.value, member: Maybe.map (\x -> x.user) after, team: Maybe.map (\t -> t.name) teamAfter } == { deleted: Nothing, member: Just bob, team: Just (Text.NonEmpty "Core") })
}

test "a user who is a member cannot be deleted" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let m <- Db.insert Member { team: team, user: bob }
  let r <- Test.run (Db.delete User bob)
  let after <- Db.get Member m
  let userAfter <- Db.get User bob
  Task.succeed ({ deleted: Result.toMaybe r.value, member: Maybe.map (\x -> x.team) after, user: Maybe.map (\u -> u.name) userAfter } == { deleted: Nothing, member: Just team, user: Just (Text.NonEmpty "Bob") })
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

-- `owner` is `@ref User` with the default rule, `@restrict`: a user who owns a team cannot be
-- deleted, and the team stays.
test "a user who owns a team cannot be deleted" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let r <- Test.run (Db.delete User owner)
  let after <- Db.get Team team
  Task.succeed ({ deleted: Result.toMaybe r.value, team: Maybe.map (\t -> t.name) after } == { deleted: Nothing, team: Just (Text.NonEmpty "Core") })
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
  let ids = List.map (\m -> m.team) memberships
  let teams <- Db.from Team |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in ids]) |> Db.orderBy Team.name.asc |> Db.all
  Query.succeed { teams: teams }
}

form NewTeam = { name: Text.NonEmpty }

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
      <Input field="name" label="Team name" />
      <Errors field="name" />
      <button>Create</button>
    </form>
  </Layout.Page>

test "the list has the teams the user owns or belongs to, ordered by name" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let third <- Db.insert User { email: Text.Email "third@example.com", name: Text.NonEmpty "Third", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zeta", owner: me }
  let beta <- Db.insert Team { name: Text.NonEmpty "Beta", owner: other }
  let _ <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: other }
  let gamma <- Db.insert Team { name: Text.NonEmpty "Gamma", owner: other }
  let _ <- Db.insert Member { team: beta, user: me }
  let _ <- Db.insert Member { team: gamma, user: third }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Beta", "Zeta"])
}

test "creating a team makes the user its owner and opens its page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Core" }
  let teams <- Db.from Team |> Db.all
  Task.succeed ({ teams: List.map (\t -> { name: t.name, owner: t.owner }) teams, same: [r.answer] == List.map (\t -> Redirected (Teams.Show { id: t.id })) teams } == { teams: [{ name: Text.NonEmpty "Core", owner: me }], same: True })
}

test "a visitor is sent to log in" = {
  let r <- Test.open (Teams.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
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
  owner: User,
  members: List User,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  match found {
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let ids = List.map (\m -> m.user) rows
      let allowed = team.owner == session.user || List.member session.user ids
      let owner <- Db.get User team.owner
      let users <- Db.from User |> Db.where (User.id.in ids) |> Db.orderBy User.name.asc |> Db.all
      match owner {
        Just o -> if allowed then Query.succeed { team: team, owner: o, members: List.filter (\u -> u.id != team.owner) users } else Query.notFound,
        Nothing -> Query.notFound,
      }
    },
    Nothing -> Query.notFound,
  }
}

form NewInvite = { email: Text.Email }

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

-- `load` has checked that the session's user belongs to the team, but only the owner may
-- invite and remove: `update` checks the owner again.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p -> if model.team.owner == session.user then sendInvite model p.form.email else Task.notFound,
  Remove p -> if model.team.owner == session.user && p.user != model.team.owner then removeMember model p.user else Task.notFound,
}

sendInvite : Model -> Text.Email -> Task Nav
sendInvite model email = {
  let _ <- Db.insert Invite { team: model.team.id, email: email }
  Task.succeed (Redirect (Teams.Show { id: model.team.id }))
}

removeMember : Model -> User.UserId -> Task Nav
removeMember model uid = {
  let rows <- Db.from Member |> Db.where (Member.team.eq model.team.id) |> Db.where (Member.user.eq uid) |> Db.all
  let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
  Task.succeed (Redirect (Teams.Show { id: model.team.id }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      <li><span class="name">{model.owner.name}</span></li>
      {for u in model.members ->
        <Row user={u} canRemove={model.team.owner == session.user} />}
    </ul>
    {if model.team.owner == session.user then <InviteForm /> else Html.none}
    <a href={Teams.Index {}}>Back</a>
  </Layout.Page>

Row : { user: User, canRemove: Bool } -> Html Msg
Row props =
  <li>
    <span class="name">{props.user.name}</span>
    {if props.canRemove then <button class="remove" click={Remove { user: props.user.id }}>Remove</button> else Html.none}
  </li>

InviteForm : { } -> Html Msg
InviteForm props =
  <form submit={SendInvite}>
    <Input field="email" label="Invite by email" />
    <Errors field="email" />
    <button>Invite</button>
  </form>

memberCount : Team.TeamId -> Query Int
memberCount team = Db.count (Db.from Member |> Db.where (Member.team.eq team))

test "a team's page is not found for a user who is not a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just eve }
  let stranger <- Test.open (Teams.Show { id: team })
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Teams.Show { id: team })
  Task.succeed ({ stranger: stranger.answer, name: Test.text "#name" page.doc, members: Test.texts "#members .name" page.doc, removes: Test.count ".remove" page.doc } == { stranger: PageMissing, name: Just "Core", members: ["Owner", "Bob"], removes: 0 })
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cy <- Db.insert User { email: Text.Email "cy@example.com", name: Text.NonEmpty "Cy", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: cy }
  let model <- load { id: team } { user: bob }
  let r <- Test.run (update (Remove { user: cy }) model { user: bob })
  let n <- memberCount team
  Task.succeed ({ value: r.value, members: n } == { value: Err "not found", members: 2 })
}

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let model <- load { id: team } { user: owner }
  let self <- Test.run (update (Remove { user: owner }) model { user: owner })
  let afterSelf <- memberCount team
  let r <- Test.run (update (Remove { user: bob }) model { user: owner })
  let afterRemove <- memberCount team
  Task.succeed ({ self: self.value, afterSelf: afterSelf, value: r.value, afterRemove: afterRemove } == { self: Err "not found", afterSelf: 1, value: Ok (Redirect (Teams.Show { id: team })), afterRemove: 0 })
}

test "the owner invites by email and a member cannot" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let model <- load { id: team } { user: bob }
  let refused <- Test.run (update (SendInvite { form: { email: Text.Email "other@example.com" } }) model { user: bob })
  let invites <- Db.from Invite |> Db.where (Invite.team.eq team) |> Db.all
  Task.succeed ({ answer: r.answer, refused: refused.value, invites: List.map (\i -> { email: i.email, status: i.status }) invites } == { answer: Redirected (Teams.Show { id: team }), refused: Err "not found", invites: [{ email: Text.Email "new@example.com", status: Pending }] })
}

test "the owner sees and uses the controls, others do not" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let zed <- Db.insert User { email: Text.Email "zed@example.com", name: Text.NonEmpty "Zed", passwordHash: "" }
  let amy <- Db.insert User { email: Text.Email "amy@example.com", name: Text.NonEmpty "Amy", passwordHash: "" }
  let eve <- Db.insert User { email: Text.Email "eve@example.com", name: Text.NonEmpty "Eve", passwordHash: "" }
  let core <- Db.insert Team { name: Text.NonEmpty "Core", owner: owner }
  let side <- Db.insert Team { name: Text.NonEmpty "Side", owner: owner }
  let _ <- Db.insert Member { team: core, user: zed }
  let _ <- Db.insert Member { team: core, user: amy }
  let _ <- Db.insert Member { team: side, user: zed }
  let _ <- Db.insert Member { team: side, user: eve }
  let visitor <- Test.open (Teams.Show { id: core })
  let _ <- Test.login { user: Just eve }
  let outsider <- Test.open (Teams.Show { id: core })
  let _ <- Test.login { user: Just amy }
  let asAmy <- Test.get (Teams.Show { id: core })
  let _ <- Test.login { user: Just owner }
  let asOwner <- Test.get (Teams.Show { id: core })
  let removed <- Test.click (Remove { user: zed })
  let sideCount <- memberCount side
  let coreCount <- memberCount core
  Task.succeed ({ visitor: visitor.answer, outsider: outsider.answer, amyMembers: Test.texts "#members .name" asAmy.doc, amyRemoves: Test.count ".remove" asAmy.doc, amyInvite: Test.count "input[name=email]" asAmy.doc, ownerMembers: Test.texts "#members .name" asOwner.doc, ownerRemoves: Test.count ".remove" asOwner.doc, ownerInvite: Test.count "input[name=email]" asOwner.doc, removed: removed.answer, after: Test.texts "#members .name" removed.doc, side: sideCount, core: coreCount } == { visitor: Redirected (Auth.Login {}), outsider: PageMissing, amyMembers: ["Owner", "Amy", "Zed"], amyRemoves: 0, amyInvite: 0, ownerMembers: ["Owner", "Amy", "Zed"], ownerRemoves: 2, ownerInvite: 1, removed: Redirected (Teams.Show { id: core }), after: ["Owner", "Amy"], side: 2, core: 1 })
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
