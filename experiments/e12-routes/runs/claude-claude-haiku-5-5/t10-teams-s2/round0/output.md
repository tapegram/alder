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

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <script>try{document.documentElement.classList.toggle("dark",localStorage.theme==="dark"||(!("theme" in localStorage)&&matchMedia("(prefers-color-scheme: dark)").matches))}catch(e){}</script>
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
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

--- file: app/Table/Team.alder ---
module Table.Team

import Table.User

type TeamId = brand Text

table Team {
  id: TeamId @id,
  name: Text.NonEmpty,
  owner: User.UserId @ref User,
}

--- file: app/Table/Member.alder ---
module Table.Member

import Table.Team
import Table.User

type MemberId = brand Text

-- One row per member who is not the owner: the owner has no row.
table Member {
  id: MemberId @id,
  team: Team.TeamId @ref Team,
  user: User.UserId @ref User,
}

-- Whether the user has a membership row for the team.
isMember : User.UserId -> Team.TeamId -> Query Bool
isMember user team = {
  let n <-
    Db.from Member
      |> Db.where (Member.team.eq team)
      |> Db.where (Member.user.eq user)
      |> Db.count
  Query.succeed (n > 0)
}

-- The team, when the user owns it or is one of its members; otherwise nothing.
visibleTeam : User.UserId -> Team.TeamId -> Query (Maybe Team)
visibleTeam user team = {
  let found <- Db.get Team team
  match found {
    Nothing -> Query.succeed Nothing,
    Just t -> {
      let member <- isMember user team
      Query.succeed (if t.owner == user || member then Just t else Nothing)
    },
  }
}

-- Makes the user a member of the team, unless they own it or already are one.
join : User.UserId -> Team.TeamId -> Task Unit
join user team = {
  let t <- Db.get Team team
  let already <- isMember user team
  let owner = Maybe.map (\x -> x.owner == user) t ?? False
  if already || owner then Task.succeed () else {
    let _ <- Db.insert Member { team: team, user: user }
    Task.succeed ()
  }
}

--- file: app/Table/Invite.alder ---
module Table.Invite

import Table.Team
import Table.User

type InviteId = brand Text

type Status = Pending | Accepted | Declined

-- An invite is for an email, and waits until it is accepted or declined.
table Invite {
  id: InviteId @id,
  team: Team.TeamId @ref Team,
  email: Text.Email,
  status: Status = Pending,
}

-- The invite with that id, when it is still waiting and is for the user's email; otherwise nothing.
waitingFor : User.UserId -> InviteId -> Query (Maybe Invite)
waitingFor user id = {
  let found <- Db.get Invite id
  let me <- Db.get User user
  let email = Maybe.map (\u -> u.email) me
  match found {
    Nothing -> Query.succeed Nothing,
    Just inv -> Query.succeed (if inv.status == Pending && Just inv.email == email then Just inv else Nothing),
  }
}

--- file: app/Teams/Index.alder ---
module Teams.Index

import Layout
import Table.Member
import Table.Team
import Table.User

route Index  = GET  "/teams" { } requires user
route Create = POST "/teams" { } form NewTeam requires user

form NewTeam = { name: Text.NonEmpty }

type Model = { teams: List Team }

-- The teams the user owns or is a member of, by name.
Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let joined <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let every <- Db.from Team |> Db.all
  let ids = List.map (\m -> m.team) joined
  let mine = List.filter (\t -> t.owner == session.user || List.member t.id ids) every
  Query.succeed { teams: List.sortByText (\t -> t.name) mine }
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
        <al-input field="name" label="New team" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <ul id="teams">
      {for t in model.teams ->
        <li>
          <a href={Teams.Show.Show { id: t.id }} class="name">{t.name}</a>
        </li>}
    </ul>
    <p>
      <a href={Invites.Index.Index {}}>Invites</a>
      <al-button variant="outline" click={App.Logout}>Log out</al-button>
    </p>
  </Layout.Page>

test "a new team is listed under its owner" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.submit (Create {}) { name: Text.NonEmpty "Club" }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Club"])
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Layout
import Table.Invite
import Table.Member
import Table.Team
import Table.User

route Show       = GET    "/teams/{id}"                       { id: Team.TeamId }                    requires user
route Remove     = DELETE "/teams/{id}/members/{user}"        { id: Team.TeamId, user: User.UserId } requires user
route SendInvite = POST   "/teams/{id}/invites"               { id: Team.TeamId } form NewInvite     requires user

type Line = { user: User.UserId, name: Text.NonEmpty, owner: Bool }

type Model = { team: Team, isOwner: Bool, members: List Line }

form NewInvite = { email: Text.Email }

-- Only the owner and members see the team; anyone else is not found.
Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let visible <- Member.visibleTeam session.user params.id
  match visible {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let users <- Db.from User |> Db.all
      let ids = List.map (\m -> m.user) rows
      let people = List.filter (\u -> u.id == team.owner || List.member u.id ids) users
      let lines : List Line = List.map (\u -> { user: u.id, name: u.name, owner: u.id == team.owner }) people
      let owners = List.filter (\l -> l.owner) lines
      let others = List.sortByText (\l -> l.name) (List.filter (\l -> not l.owner) lines)
      Query.succeed { team: team, isOwner: team.owner == session.user, members: owners ++ others }
    },
  }
}

-- Only the owner may remove a member, and never the owner.
Remove.update : Remove.Params -> Session -> Task Response
Remove.update p session = {
  let team <- Db.get Team p.id
  let link <-
    Db.from Member
      |> Db.where (Member.team.eq p.id)
      |> Db.where (Member.user.eq p.user)
      |> Db.first
  match team {
    Nothing -> Task.notFound,
    Just t -> {
      if t.owner != session.user || p.user == t.owner then Task.notFound
      else {
        match link {
          Just m -> {
            let _ <- Db.delete Member m.id
            Task.succeed (Response.redirect (Teams.Show.Show { id: p.id }))
          },
          Nothing -> Task.notFound,
        }
      }
    },
  }
}

-- Only the owner may invite; the invite is stored whether or not the email has a user yet.
SendInvite.update : SendInvite.Params -> Session -> Task Response
SendInvite.update p session = {
  let team <- Db.get Team p.id
  match team {
    Nothing -> Task.notFound,
    Just t -> {
      if t.owner != session.user then Task.notFound
      else {
        let _ <- Db.insert Invite { team: t.id, email: p.form.email }
        Task.succeed (Response.redirect (Teams.Show.Show { id: t.id }))
      }
    },
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      {for m in model.members ->
        <li>
          <span class="name">{m.name}</span>
          {if model.isOwner && not m.owner then
            <al-button variant="destructive" size="sm" class="remove" click={Remove { id: model.team.id, user: m.user }}>Remove</al-button>
          else Html.none}
        </li>}
    </ul>
    {if model.isOwner then
      <al-card>
        <form submit={SendInvite { id: model.team.id }}>
          <al-input field="email" label="Invite by email" />
          <al-button type="submit">Invite</al-button>
        </form>
      </al-card>
    else Html.none}
    <a href={Teams.Index.Index {}}>All teams</a>
  </Layout.Page>

test "a stranger is not found on a team's page" = {
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
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let page <- Test.open (Show { id: team })
  Task.succeed (page.answer == PageMissing)
}

test "a member can open the team's page" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let member <- Db.insert User {
    email: Text.Email "member@example.com",
    name: Text.NonEmpty "Member",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Db.insert Member { team: team, user: member }
  let _ <- Test.login { user: Just member }
  let page <- Test.get (Show { id: team })
  Task.succeed (Test.text "#name" page.doc == Just "Club")
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let memberA <- Db.insert User {
    email: Text.Email "a@example.com",
    name: Text.NonEmpty "A",
    passwordHash: "",
  }
  let memberB <- Db.insert User {
    email: Text.Email "b@example.com",
    name: Text.NonEmpty "B",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Db.insert Member { team: team, user: memberA }
  let _ <- Db.insert Member { team: team, user: memberB }
  let _ <- Test.login { user: Just memberA }
  let r <- Test.run (Remove.update { id: team, user: memberB } { user: memberA })
  let rows <- Db.from Member |> Db.where (Member.user.eq memberB) |> Db.count
  Task.succeed ({ value: r.value, rows: rows } == { value: Err "not found", rows: 1 })
}

test "the owner removes a member, but not themselves" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let memberB <- Db.insert User {
    email: Text.Email "b@example.com",
    name: Text.NonEmpty "B",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let _ <- Db.insert Member { team: team, user: memberB }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Show { id: team })
  let removeButtons = Test.count ".remove" page.doc
  let r <- Test.click (Remove { id: team, user: memberB })
  let members <- Db.from Member |> Db.where (Member.team.eq team) |> Db.count
  let selfResult <- Test.run (Remove.update { id: team, user: owner } { user: owner })
  Task.succeed (
    {
      buttons: removeButtons,
      removed: r.answer,
      members: members,
      selfResult: selfResult.value,
    } == {
      buttons: 1,
      removed: Redirected (Show { id: team }),
      members: 0,
      selfResult: Err "not found",
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

route Index   = GET "/invites" { } requires user
route Accept  = PUT "/invites/{id}/accepted" { id: Invite.InviteId } requires user
route Decline = PUT "/invites/{id}/declined" { id: Invite.InviteId } requires user

type Line = { invite: Invite.InviteId, name: Text.NonEmpty }

type Model = { lines: List Line }

-- An invite with its team's name.
line : List Team -> Invite -> Maybe Line
line teams invite = {
  let found = List.find (\t -> t.id == invite.team) teams
  match found {
    Just t -> Just { invite: invite.id, name: t.name },
    Nothing -> Nothing,
  }
}

-- The invites waiting for the signed-in user's email.
Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let me <- Db.get User session.user
  let email = Maybe.map (\u -> u.email) me
  let waiting <- Db.from Invite |> Db.where (Invite.status.eq Pending) |> Db.all
  let mine = List.filter (\i -> Just i.email == email) waiting
  let teams <- Db.from Team |> Db.all
  Query.succeed { lines: List.filterMap (line teams) mine }
}

-- Accepting makes the user a member, and the team is then on their list and page.
Accept.update : Accept.Params -> Session -> Task Response
Accept.update p session = {
  let found <- Invite.waitingFor session.user p.id
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
      let _ <- Member.join session.user inv.team
      Task.succeed (Response.redirect (Teams.Show.Show { id: inv.team }))
    },
  }
}

-- Declining leaves the team closed to the user.
Decline.update : Decline.Params -> Session -> Task Response
Decline.update p session = {
  let found <- Invite.waitingFor session.user p.id
  match found {
    Nothing -> Task.notFound,
    Just inv -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
      Task.succeed (Response.redirect (Invites.Index.Index {}))
    },
  }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for l in model.lines ->
        <li>
          <span class="team">{l.name}</span>
          <al-button variant="secondary" size="sm" click={Accept { id: l.invite }}>Accept</al-button>
          <al-button variant="ghost" size="sm" click={Decline { id: l.invite }}>Decline</al-button>
        </li>}
    </ul>
    <a href={Teams.Index.Index {}}>Teams</a>
  </Layout.Page>

test "accepting an invite makes the team visible" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let accepted <- Test.click (Accept { id: invite })
  let teams <- Test.get (Teams.Index.Index {})
  Task.succeed (
    {
      answer: accepted.answer,
      page: Test.text "#name" accepted.doc,
      listed: Test.texts "#teams .name" teams.doc,
    } == {
      answer: Redirected (Teams.Show.Show { id: team }),
      page: Just "Club",
      listed: ["Club"],
    }
  )
}

test "declining an invite leaves the team closed" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Club", owner: owner }
  let invite <- Db.insert Invite { team: team, email: Text.Email "me@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let declined <- Test.click (Decline { id: invite })
  let page <- Test.open (Teams.Show.Show { id: team })
  Task.succeed (
    {
      answer: declined.answer,
      waiting: Test.count "#invites li" declined.doc,
      team: page.answer,
    } == {
      answer: Redirected (Index {}),
      waiting: 0,
      team: PageMissing,
    }
  )
}
