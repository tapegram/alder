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

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
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

form Credentials = { email: Text.Email, password: Text.Secret }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

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
  let r <- Test.submit (Submit {}) { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in goes to the teams" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit (Submit {}) { email: Text.Email "me@example.com", password: Text.Secret "secret" }
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

--- file: app/Invites/Index.alder ---
module Invites.Index

import Layout
import Table.Invite
import Table.Member
import Table.Team
import Table.User

route Index   = GET    "/invites"      { } requires user
route Accept  = PUT    "/invites/{id}" { id: Invite.InviteId } requires user
route Decline = DELETE "/invites/{id}" { id: Invite.InviteId } requires user

type Waiting = { invite: Invite.InviteId, teamName: Text }

type Model = { invites: List Waiting }

-- An invite the signed-in user may answer: sent to their email and still waiting.
answerable : Invite -> Maybe User -> Bool
answerable inv me =
  Maybe.withDefault False (Maybe.map (\u -> u.email == inv.email && inv.status == Pending) me)

waitingOne : Invite -> List Team -> Maybe Waiting
waitingOne i teams = {
  let found = List.find (\t -> t.id == i.team) teams
  match found {
    Nothing -> Nothing,
    Just t -> Just { invite: i.id, teamName: t.name },
  }
}

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let invites <-
        Db.from Invite
          |> Db.where (Invite.email.eq u.email)
          |> Db.where (Invite.status.eq Pending)
          |> Db.all
      let teams <- Db.from Team |> Db.all
      Query.succeed { invites: List.filterMap (\i -> waitingOne i teams) invites }
    },
  }
}

Accept.update : Accept.Params -> Session -> Task Response
Accept.update p session = {
  let found <- Db.get Invite p.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      if answerable inv me then {
        let _ <- Db.update Invite p.id (\i -> { i | status: Accepted })
        let _ <- Db.insert Member { team: inv.team, user: session.user }
        Task.succeed (Response.redirect (Teams.Show.Show { id: inv.team }))
      } else Task.notFound
    },
  }
}

Decline.update : Decline.Params -> Session -> Task Response
Decline.update p session = {
  let found <- Db.get Invite p.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      if answerable inv me then {
        let _ <- Db.update Invite p.id (\i -> { i | status: Declined })
        Task.succeed (Response.redirect (Index {}))
      } else Task.notFound
    },
  }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>

    <ul id="invites">
      {for w in model.invites ->
        <li>
          <span class="team">{w.teamName}</span>
          <button class="accept" click={Accept { id: w.invite }}>Accept</button>
          <button class="decline" click={Decline { id: w.invite }}>Decline</button>
        </li>}
    </ul>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Board", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Index {})
  let accepted <- Test.click (Accept { id: invite })
  let teams <- Test.open (Teams.Index.Index {})
  let page <- Test.open (Teams.Show.Show { id: team })
  Task.succeed (
    {
      answer: accepted.answer,
      listed: Test.texts ".name" teams.doc,
      shown: page.answer,
    } == {
      answer: Redirected (Teams.Show.Show { id: team }),
      listed: ["Board"],
      shown: Shown,
    }
  )
}

test "declining an invite leaves the team closed" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Board", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Index {})
  let declined <- Test.click (Decline { id: invite })
  let page <- Test.open (Teams.Show.Show { id: team })
  let teams <- Test.open (Teams.Index.Index {})
  Task.succeed (
    {
      answer: declined.answer,
      waiting: Test.count "#invites li" declined.doc,
      shown: page.answer,
      listed: Test.count ".name" teams.doc,
    } == {
      answer: Redirected (Index {}),
      waiting: 0,
      shown: PageMissing,
      listed: 0,
    }
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
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

--- file: app/Table/Invite.alder ---
module Table.Invite

import Table.Team

type InviteId = brand Text

type Status = Pending | Accepted | Declined

table Invite {
  id: InviteId @id,
  team: Team.TeamId @ref Team,
  email: Text.Email,
  status: Status = Pending,
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

--- file: app/Table/Team.alder ---
module Table.Team

import Table.User

type TeamId = brand Text

table Team {
  id: TeamId @id,
  name: Text.NonEmpty,
  owner: User.UserId @ref User,
}

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

--- file: app/Teams/Index.alder ---
module Teams.Index

import Layout
import Table.Member
import Table.Team
import Table.User

route Index  = GET  "/teams" { } requires user
route Create = POST "/teams" { } form NewTeam requires user

type Model = { teams: List Team }

form NewTeam = { name: Text.NonEmpty }

-- The teams a user can open: the ones they own, and the ones they are a member of.
visibleTo : User.UserId -> List Member -> Team -> Bool
visibleTo who memberships team =
  team.owner == who || List.any (\m -> m.team == team.id) memberships

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let teams <-
    Db.from Team
      |> Db.orderBy Team.name.asc
      |> Db.all
  let memberships <-
    Db.from Member
      |> Db.where (Member.user.eq session.user)
      |> Db.all
  Query.succeed { teams: List.filter (visibleTo session.user memberships) teams }
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert Team { name: p.form.name, owner: session.user }
  Task.succeed (Response.redirect (Teams.Show.Show { id: id }))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Teams">
    <h1>Teams</h1>

    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New team name" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>

    <ul id="teams">
      {for t in model.teams ->
        <li>
          <a class="name" href={Teams.Show.Show { id: t.id }}>{t.name}</a>
        </li>}
    </ul>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a new team is listed" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Index {})
  let _ <- Test.submit (Create {}) { name: Text.NonEmpty "Garden" }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts ".name" page.doc == ["Garden"])
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Layout
import Table.Invite
import Table.Member
import Table.Team
import Table.User

route Show       = GET    "/teams/{id}"                { id: Team.TeamId } requires user
route SendInvite = POST   "/teams/{id}/invites"        { id: Team.TeamId } form NewInvite requires user
route Remove     = DELETE "/teams/{id}/members/{user}" { id: Team.TeamId, user: User.UserId } requires user

type Model = { team: Team, members: List User }

form NewInvite = { email: Text.Email }

-- The owner and the members see the team; anyone else is not found.
canSee : User.UserId -> Team -> List Member -> Bool
canSee who team memberships =
  team.owner == who || List.any (\m -> m.user == who) memberships

-- The owner first, then the other members ordered by name.
membersOf : Team -> List Member -> List User -> List User
membersOf team memberships users = {
  let ids = List.map (\m -> m.user) memberships
  let owners = List.filter (\u -> u.id == team.owner) users
  let others = List.filter (\u -> u.id != team.owner && List.member u.id ids) users
  owners ++ List.sortByText (\u -> u.name) others
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let memberships <-
        Db.from Member
          |> Db.where (Member.team.eq team.id)
          |> Db.all
      let users <- Db.from User |> Db.all
      if canSee session.user team memberships then
        Query.succeed { team: team, members: membersOf team memberships users }
      else Query.notFound
    },
  }
}

SendInvite.update : SendInvite.Params -> Session -> Task Response
SendInvite.update p session = {
  let found <- Db.get Team p.id
  match found {
    Nothing -> Task.notFound,
    Just team -> {
      if team.owner != session.user then Task.notFound else {
        let _ <- Db.insert Invite { team: p.id, email: p.form.email }
        Task.succeed (Response.redirect (Show { id: p.id }))
      }
    },
  }
}

-- The owner removes a member. Anyone else, and the owner removing themselves, is not found and
-- nothing changes.
Remove.update : Remove.Params -> Session -> Task Response
Remove.update p session = {
  let found <- Db.get Team p.id
  match found {
    Nothing -> Task.notFound,
    Just team -> {
      if team.owner != session.user || p.user == team.owner then Task.notFound else {
        let rows <-
          Db.from Member
            |> Db.where (Member.team.eq p.id)
            |> Db.where (Member.user.eq p.user)
            |> Db.all
        let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
        Task.succeed (Response.redirect (Show { id: p.id }))
      }
    },
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>

    <al-card>
      <ul id="members">
        {for m in model.members ->
          <li>
            <span class="name">{m.name}</span>
            {if model.team.owner == session.user && m.id != model.team.owner then
              <button class="remove" click={Remove { id: model.team.id, user: m.id }}>Remove</button>
            else Html.none}
          </li>}
      </ul>
    </al-card>

    {if model.team.owner == session.user then
      <al-card>
        <form submit={SendInvite { id: model.team.id }}>
          <al-input field="email" label="Invite by email" />
          <al-button type="submit">Invite</al-button>
        </form>
      </al-card>
    else Html.none}

    <al-button variant="outline" href={Teams.Index.Index {}}>Back to teams</al-button>
  </Layout.Page>

test "a stranger's team page is not found" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let stranger <- Db.insert User {
    email: Text.Email "stranger@example.com",
    name: Text.NonEmpty "Stranger",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Board", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let strangerPage <- Test.open (Show { id: team })
  let _ <- Test.login { user: Just owner }
  let ownerPage <- Test.open (Show { id: team })
  Task.succeed (
    { stranger: strangerPage.answer, owner: ownerPage.answer }
      == { stranger: PageMissing, owner: Shown }
  )
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let alice <- Db.insert User {
    email: Text.Email "alice@example.com",
    name: Text.NonEmpty "Alice",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Board", owner: owner }
  let _ <- Db.insert Member { team: team, user: alice }
  let _ <- Db.insert Member { team: team, user: bob }
  let r <- Test.run (Remove.update { id: team, user: alice } { user: bob })
  let left <- Db.from Member |> Db.where (Member.user.eq alice) |> Db.count
  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })
}

test "the owner removes a member but not themselves" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let alice <- Db.insert User {
    email: Text.Email "alice@example.com",
    name: Text.NonEmpty "Alice",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Board", owner: owner }
  let _ <- Db.insert Member { team: team, user: alice }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Show { id: team })
  let removed <- Test.click (Remove { id: team, user: alice })
  let self <- Test.run (Remove.update { id: team, user: owner } { user: owner })
  let left <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  Task.succeed (
    { removed: removed.answer, self: self.value, left: left }
      == { removed: Redirected (Show { id: team }), self: Err "not found", left: 0 }
  )
}
