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
    <nav>
      <a href={Teams.Index {}}>Teams</a>
      <a href={Invites.Index {}}>Invites</a>
      <button click={App.Logout}>Log out</button>
    </nav>

    <h1>Teams</h1>

    <ul id="teams">
      {for team in model.teams ->
        <li><a class="name" href={Teams.Show { id: team.id }}>{team.name}</a></li>}
    </ul>

    <form submit={Create}>
      <Errors form />
      <Input field="name" label="New team" />
      <Errors field="name" />
      <button>Create</button>
    </form>
  </Layout.Page>

test "creating a team makes the user its owner" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Rockets" }
  let found <- Db.from Team |> Db.first
  let page <- Test.get (Teams.Index {})
  Task.succeed ({ answer: Just r.answer, name: Test.text "#name" r.doc, owner: Maybe.map (\t -> t.owner) found, teams: Test.texts "#teams .name" page.doc } == { answer: Maybe.map (\t -> Redirected (Teams.Show { id: t.id })) found, name: Just "Rockets", owner: Just me, teams: ["Rockets"] })
}

test "an empty team name is rejected" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submitRaw NewTeam [{ name: "name", value: "" }]
  let rows <- Db.from Team |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
}

test "the list has the teams the user owns or is a member of, by name" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zebra", owner: me }
  let _ <- Db.insert Team { name: Text.NonEmpty "Middle", owner: other }
  let alpha <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: other }
  let _ <- Db.insert Member { team: alpha, user: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Teams.Index {})
  Task.succeed ({ names: Test.texts "#teams .name" page.doc, items: Test.count "#teams li" page.doc } == { names: ["Alpha", "Zebra"], items: 2 })
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
  members: List User,
}

-- The page is for the team's owner and members only: for anyone else it is a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Team params.id
  let rows <- Db.from Member |> Db.where (Member.team.eq params.id) |> Db.all
  match found {
    Just team -> {
      let ids = rows |> List.map (\m -> m.user) |> List.filter (\u -> u != team.owner)
      let owner <- Db.get User team.owner
      let members <- Db.from User |> Db.where (User.id.in ids) |> Db.orderBy User.name.asc |> Db.all
      match owner {
        Just o ->
          if team.owner == session.user || List.member session.user ids
            then Query.succeed { team: team, owner: o, members: members }
            else Query.notFound,
        Nothing -> Query.notFound,
      }
    },
    Nothing -> Query.notFound,
  }
}

form NewInvite = {
  email: Text.Email,
}

msg SendInvite form NewInvite
msg Remove { user: User.UserId }

-- `load` has checked that the session's user is the owner or a member; inviting and removing
-- are the owner's alone, so `update` checks that again. The owner is never removed.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  SendInvite p ->
    if model.team.owner == session.user then {
      let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    } else Task.notFound,
  Remove p ->
    if model.team.owner != session.user || p.user == model.team.owner then Task.notFound else {
      let rows <-
        Db.from Member
          |> Db.where (Member.team.eq model.team.id)
          |> Db.where (Member.user.eq p.user)
          |> Db.all
      let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.team.name}>
    <nav>
      <a href={Teams.Index {}}>Teams</a>
      <a href={Invites.Index {}}>Invites</a>
      <button click={App.Logout}>Log out</button>
    </nav>

    <h1 id="name">{model.team.name}</h1>

    <ul id="members">
      <li><span class="name">{model.owner.name}</span></li>
      {for member in model.members ->
        <li>
          <span class="name">{member.name}</span>
          {if model.team.owner == session.user
            then <button class="remove" click={Remove { user: member.id }}>Remove</button>
            else Html.none}
        </li>}
    </ul>

    {if model.team.owner == session.user
      then
        <form submit={SendInvite}>
          <Errors form />
          <Input field="email" label="Invite by email" />
          <Errors field="email" />
          <button>Invite</button>
        </form>
      else Html.none}
  </Layout.Page>

test "a team's page is not found for a user who is not a member" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let _ <- Test.login { user: Just sam }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

test "the owner and the members see the team, the owner first" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just owner }
  let mine <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just ann }
  let hers <- Test.get (Teams.Show { id: team })
  Task.succeed ({ name: Test.text "#name" mine.doc, names: Test.texts "#members .name" mine.doc, buttons: Test.count "#members li .remove" mine.doc, hers: Test.texts "#members .name" hers.doc, herButtons: Test.count ".remove" hers.doc, herForm: Test.has "input[name=email]" hers.doc } == { name: Just "Rockets", names: ["Olive", "Ann", "Bob"], buttons: 2, hers: ["Olive", "Ann", "Bob"], herButtons: 0, herForm: False })
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (Remove { user: bob }) page.model { user: ann })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ value: r.value, members: left, buttons: Test.count ".remove" page.doc } == { value: Err "not found", members: 2, buttons: 0 })
}

test "a member cannot invite" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Show { id: team })
  let r <- Test.run (update (SendInvite { form: { email: Text.Email "new@example.com" } }) page.model { user: ann })
  let invites <- Db.from Invite |> Db.count
  Task.succeed ({ value: r.value, invites: invites } == { value: Err "not found", invites: 0 })
}

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let removed <- Test.click (Remove { user: ann })
  let page <- Test.get (Teams.Show { id: team })
  let self <- Test.run (update (Remove { user: owner }) page.model { user: owner })
  let after <- Test.get (Teams.Show { id: team })
  let _ <- Test.login { user: Just ann }
  let hers <- Test.open (Teams.Show { id: team })
  Task.succeed ({ answer: removed.answer, names: Test.texts "#members .name" removed.doc, buttons: Test.count ".remove" removed.doc, self: self.value, after: Test.texts "#members .name" after.doc, hers: hers.answer } == { answer: Redirected (Teams.Show { id: team }), names: ["Olive", "Bob"], buttons: 1, self: Err "not found", after: ["Olive", "Bob"], hers: PageMissing })
}

test "the owner invites an email, whether or not a user has it" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Teams.Show { id: team })
  let r <- Test.submit NewInvite { email: Text.Email "nobody@example.com" }
  let found <- Db.from Invite |> Db.first
  Task.succeed ({ answer: r.answer, invite: Maybe.map (\i -> { team: i.team, email: i.email, status: i.status }) found } == { answer: Redirected (Teams.Show { id: team }), invite: Just { team: team, email: Text.Email "nobody@example.com", status: Pending } })
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

-- The invites waiting for the session's user: those to the user's email that are still Pending.
load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Just user -> {
      let invites <-
        Db.from Invite
          |> Db.where (Invite.email.eq user.email)
          |> Db.where (Invite.status.eq Pending)
          |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: List.filterMap (waiting teams) invites }
    },
    Nothing -> Query.succeed { invites: [] },
  }
}

waiting : List Team -> Invite -> Maybe Waiting
waiting teams invite = Maybe.map (pair invite) (List.find (\t -> t.id == invite.team) teams)

pair : Invite -> Team -> Waiting
pair invite team = { invite: invite, team: team }

msg Accept { id: Invite.InviteId }
msg Decline { id: Invite.InviteId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Accept p -> {
    let invite <- waitingFor p.id session
    let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
    let _ <- join invite.team session.user
    Task.succeed (Redirect (Teams.Show { id: invite.team }))
  },
  Decline p -> {
    let invite <- waitingFor p.id session
    let _ <- Db.update Invite invite.id (\i -> { i | status: Declined })
    Task.succeed (Redirect (Invites.Index {}))
  },
}

-- waitingFor is the invite a message names, when it is to the session's user's email and still
-- waiting; otherwise the message is a 404. The id comes from the request, so it is checked here.
waitingFor : Invite.InviteId -> Session -> Task Invite
waitingFor id session = {
  let found <- Db.get Invite id
  let me <- Db.get User session.user
  match found {
    Just invite -> match me {
      Just user ->
        if invite.email == user.email && invite.status == Pending
          then Task.succeed invite
          else Task.notFound,
      Nothing -> Task.notFound,
    },
    Nothing -> Task.notFound,
  }
}

-- join makes the user a member of the team, unless the user owns it or is a member already.
join : Team.TeamId -> User.UserId -> Task Unit
join teamId userId = {
  let found <- Db.get Team teamId
  let n <-
    Db.from Member
      |> Db.where (Member.team.eq teamId)
      |> Db.where (Member.user.eq userId)
      |> Db.count
  match found {
    Just t -> if t.owner == userId || n > 0 then Task.succeed () else {
      let _ <- Db.insert Member { team: teamId, user: userId }
      Task.succeed ()
    },
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Invites">
    <nav>
      <a href={Teams.Index {}}>Teams</a>
      <a href={Invites.Index {}}>Invites</a>
      <button click={App.Logout}>Log out</button>
    </nav>

    <h1>Invites</h1>

    <ul id="invites">
      {for w in model.invites ->
        <li>
          <span class="team">{w.team.name}</span>
          <button class="accept" click={Accept { id: w.invite.id }}>Accept</button>
          <button class="decline" click={Decline { id: w.invite.id }}>Decline</button>
        </li>}
    </ul>
  </Layout.Page>

test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "ann@example.com" }
  let _ <- Test.login { user: Just ann }
  let before <- Test.open (Teams.Show { id: team })
  let listed <- Test.get (Invites.Index {})
  let r <- Test.click (Accept { id: invite })
  let teams <- Test.get (Teams.Index {})
  let rest <- Test.get (Invites.Index {})
  Task.succeed ({ before: before.answer, waiting: Test.texts "#invites .team" listed.doc, answer: r.answer, name: Test.text "#name" r.doc, members: Test.texts "#members .name" r.doc, teams: Test.texts "#teams .name" teams.doc, rest: Test.count "#invites li" rest.doc } == { before: PageMissing, waiting: ["Rockets"], answer: Redirected (Teams.Show { id: team }), name: Just "Rockets", members: ["Olive", "Ann"], teams: ["Rockets"], rest: 0 })
}

test "declining an invite leaves the team closed" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "ann@example.com" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let r <- Test.click (Decline { id: invite })
  let show <- Test.open (Teams.Show { id: team })
  let teams <- Test.get (Teams.Index {})
  let members <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, rest: Test.count "#invites li" r.doc, show: show.answer, teams: Test.texts "#teams .name" teams.doc, members: members } == { answer: Redirected (Invites.Index {}), rest: 0, show: PageMissing, teams: [], members: 0 })
}

test "an invite that is no longer waiting cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "ann@example.com" }
  let first <- Test.run (update (Decline { id: invite }) { invites: [] } { user: ann })
  let again <- Test.run (update (Accept { id: invite }) { invites: [] } { user: ann })
  let members <- Db.from Member |> Db.count
  Task.succeed ({ first: first.value, again: again.value, members: members } == { first: Ok (Redirect (Invites.Index {})), again: Err "not found", members: 0 })
}

test "an invite to someone else cannot be answered" = {
  let owner <- Db.insert User { email: Text.Email "olive@example.com", name: Text.NonEmpty "Olive", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Rockets", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "ann@example.com" }
  let _ <- Test.login { user: Just sam }
  let page <- Test.get (Invites.Index {})
  let accept <- Test.run (update (Accept { id: invite }) { invites: [] } { user: sam })
  let decline <- Test.run (update (Decline { id: invite }) { invites: [] } { user: sam })
  let after <- Db.get Invite invite
  let members <- Db.from Member |> Db.count
  Task.succeed ({ listed: Test.count "#invites li" page.doc, accept: accept.value, decline: decline.value, status: Maybe.map (\i -> i.status) after, members: members } == { listed: 0, accept: Err "not found", decline: Err "not found", status: Just Pending, members: 0 })
}
