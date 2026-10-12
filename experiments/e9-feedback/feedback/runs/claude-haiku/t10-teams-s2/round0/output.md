--- file: app/User.alder ---
```alder
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}
```

--- file: app/App.alder ---
```alder
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
```

--- file: app/Layout.alder ---
```alder
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
```

--- file: app/Auth/Login.alder ---
```alder
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
```

--- file: app/Team.alder ---
```alder
module Team

import User

type TeamId = brand Text

table Team {
  id: TeamId @id,
  name: Text.NonEmpty,
  owner: User.UserId @ref User,
}
```

--- file: app/Member.alder ---
```alder
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
```

--- file: app/Invite.alder ---
```alder
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
```

--- file: app/Teams/Index.alder ---
```alder
module Teams.Index

import Team
import Member
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
```

--- file: app/Teams/Show.alder ---
```alder
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
      if team.owner == session.user || mine > 0 then loadTeam team else Query.notFound,
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
    let _ <- Db.insert Invite { team: model.team.id, email: p.form.email, status: Pending }
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

test "a team is not found for a user who is not a member" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let outsider <- Db.insert User { email: Text.Email "outsider@example.com", name: Text.NonEmpty "Outsider", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Test.login { user: Just outsider }
  let r <- Test.open (Teams.Show { id: team })
  Task.succeed (r.answer == PageMissing)
}

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

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let team <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let model <- load { id: team } { user: owner }
  let ownResult <- Test.run (update (Remove { user: owner }) model { user: owner })
  let removed <- Test.run (update (Remove { user: ann }) model { user: owner })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed ({ own: ownResult.value, removed: removed.value, left: left } == { own: Err "not found", removed: Ok Rerender, left: 0 })
}
```

--- file: app/Invites/Index.alder ---
```alder
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
      Query.succeed { invites: List.filterMap (\r -> r) rows },
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

test "accepting an invite makes the team visible, declining does not" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let crew <- Db.insert Team { name: Text.NonEmpty "Crew", owner: owner }
  let club <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let yes <- Db.insert Invite { team: crew, email: Text.Email "ann@example.com", status: Pending }
  let no <- Db.insert Invite { team: club, email: Text.Email "ann@example.com", status: Pending }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Invites.Index {})
  let _ <- Test.click (Decline { id: no })
  let accepted <- Test.click (Accept { id: yes })
  let teams <- Test.get (Teams.Index {})
  let crewPage <- Test.open (Teams.Show { id: crew })
  let clubPage <- Test.open (Teams.Show { id: club })
  let invites <- Test.get (Invites.Index {})
  Task.succeed ({ accepted: accepted.answer, names: Test.texts "#teams .name" teams.doc, crew: crewPage.answer, club: clubPage.answer, waiting: Test.count "#invites li" invites.doc } == { accepted: Redirected (Teams.Show { id: crew }), names: ["Crew"], crew: Shown, club: PageMissing, waiting: 0 })
}
```
