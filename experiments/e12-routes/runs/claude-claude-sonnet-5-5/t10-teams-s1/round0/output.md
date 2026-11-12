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

table Member {
  id: MemberId @id,
  team: Team.TeamId @ref Team,
  user: User.UserId @ref User,
}

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
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

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

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

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

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

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

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Teams.Index.Index {}), user: Just me }
  )
}

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

--- file: app/Teams/Index.alder ---
module Teams.Index

import Layout
import Table.Member
import Table.Team
import Table.User

route Index  = GET  "/teams" { }                   requires user
route Create = POST "/teams" { } form NewTeam      requires user

type Model = { teams: List Team }

form NewTeam = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let owned <-
    Db.from Team
      |> Db.where (Team.owner.eq session.user)
      |> Db.all
  let rows <-
    Db.from Member
      |> Db.where (Member.user.eq session.user)
      |> Db.all
  let joined <-
    Db.from Team
      |> Db.where (Team.id.in (List.map (\m -> m.team) rows))
      |> Db.all
  Query.succeed { teams: List.sortByText (\t -> t.name) (List.unique (owned ++ joined)) }
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
    <nav><a href={Invites.Index.Index {}}>Invites</a></nav>
    <ul id="teams">
      {for t in model.teams ->
        <li><a class="name" href={Teams.Show.Show { id: t.id }}>{t.name}</a></li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <Errors form />
        <al-input field="name" label="New team" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "teams lists owned and joined teams, and a created team is owned" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zed", owner: other }
  let alpha <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: other }
  let _ <- Db.insert Member { team: alpha, user: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.submit (Create {}) { name: Text.NonEmpty "Beta" }
  let page <- Test.get (Index {})
  let mine <-
    Db.from Team
      |> Db.where (Team.name.eq (Text.NonEmpty "Beta"))
      |> Db.all
  Task.succeed (
    { owners: List.map (\t -> t.owner) mine, listed: Test.texts "#teams .name" page.doc }
      == { owners: [me], listed: ["Alpha", "Beta"] }
  )
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Layout
import Table.Invite
import Table.Member
import Table.Team
import Table.User

route Show       = GET    "/teams/{id}"                { id: Team.TeamId }                    requires user
route SendInvite = POST   "/teams/{id}/invites"        { id: Team.TeamId } form NewInvite     requires user
route Remove     = DELETE "/teams/{id}/members/{user}" { id: Team.TeamId, user: User.UserId } requires user

type Model = { team: Team, isOwner: Bool, members: List User }

form NewInvite = { email: Text.Email }

canAccess : Team -> User.UserId -> Query Bool
canAccess t uid = {
  let n <-
    Db.from Member
      |> Db.where (Member.team.eq t.id)
      |> Db.where (Member.user.eq uid)
      |> Db.count
  Query.succeed (t.owner == uid || n > 0)
}

membersOf : Team -> Query (List User)
membersOf t = {
  let owner <- Db.get User t.owner
  let rows <-
    Db.from Member
      |> Db.where (Member.team.eq t.id)
      |> Db.all
  let users <-
    Db.from User
      |> Db.where (User.id.in (List.map (\m -> m.user) rows))
      |> Db.all
  let others =
    users
      |> List.filter (\u -> u.id != t.owner)
      |> List.sortByText (\u -> u.name)
  Query.succeed (Maybe.toList owner ++ others)
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just t -> {
      let allowed <- canAccess t session.user
      let ms <- membersOf t
      if allowed then
        Query.succeed { team: t, isOwner: t.owner == session.user, members: ms }
      else
        Query.notFound
    },
  }
}

SendInvite.update : SendInvite.Params -> Session -> Task Response
SendInvite.update p session = {
  let found <- Db.get Team p.id
  match found {
    Nothing -> Task.notFound,
    Just t ->
      if t.owner != session.user then
        Task.notFound
      else {
        let _ <- Db.insert Invite { team: t.id, email: p.form.email }
        Task.succeed (Response.redirect (Show { id: p.id }))
      },
  }
}

Remove.update : Remove.Params -> Session -> Task Response
Remove.update p session = {
  let found <- Db.get Team p.id
  match found {
    Nothing -> Task.notFound,
    Just t ->
      if t.owner != session.user || p.user == t.owner then
        Task.notFound
      else {
        let rows <-
          Db.from Member
            |> Db.where (Member.team.eq p.id)
            |> Db.where (Member.user.eq p.user)
            |> Db.all
        let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
        Task.succeed (Response.redirect (Show { id: p.id }))
      },
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.team.name}>
    <p><a href={Teams.Index.Index {}}>All teams</a></p>
    <h1 id="name">{model.team.name}</h1>
    <h2>Members</h2>
    <ul id="members">
      {for u in model.members ->
        <li>
          <span class="name">{u.name}</span>
          {if model.isOwner && u.id != model.team.owner then <button class="remove" click={Remove { id: model.team.id, user: u.id }}>Remove</button> else Html.none}
        </li>}
    </ul>
    {if model.isOwner then <form submit={SendInvite { id: model.team.id }}>
      <Errors form />
      <al-input field="email" label="Invite by email" />
      <al-button type="submit">Invite</al-button>
    </form> else Html.none}
  </Layout.Page>

test "a team's page is not found for a non-member" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let eve <- Db.insert User {
    email: Text.Email "eve@example.com",
    name: Text.NonEmpty "Eve",
    passwordHash: "",
  }
  let tid <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Test.login { user: Just eve }
  let stranger <- Test.open (Show { id: tid })
  let _ <- Test.login { user: Just ann }
  let owner <- Test.get (Show { id: tid })
  Task.succeed (
    { stranger: stranger.answer, name: Test.text "#name" owner.doc, members: Test.texts "#members .name" owner.doc }
      == { stranger: PageMissing, name: Just "Core", members: ["Ann"] }
  )
}

test "a member cannot remove another member" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let cat <- Db.insert User {
    email: Text.Email "cat@example.com",
    name: Text.NonEmpty "Cat",
    passwordHash: "",
  }
  let tid <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Db.insert Member { team: tid, user: bob }
  let _ <- Db.insert Member { team: tid, user: cat }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Show { id: tid })
  let _ <- Test.login { user: Just bob }
  let r <- Test.click (Remove { id: tid, user: cat })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, members: n } == { answer: PageMissing, members: 2 })
}

test "the owner removes a member but cannot remove themselves" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let tid <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Db.insert Member { team: tid, user: bob }
  let _ <- Test.login { user: Just ann }
  let before <- Test.get (Show { id: tid })
  let r <- Test.click (Remove { id: tid, user: bob })
  let after <- Test.get (Show { id: tid })
  let self <- Test.run (Remove.update { id: tid, user: ann } { user: ann })
  let again <- Test.get (Show { id: tid })
  Task.succeed (
    {
      buttons: Test.count ".remove" before.doc,
      answer: r.answer,
      members: Test.texts "#members .name" after.doc,
      self: self.value,
      still: Test.texts "#members .name" again.doc,
    } == {
      buttons: 1,
      answer: Redirected (Show { id: tid }),
      members: ["Ann"],
      self: Err "not found",
      still: ["Ann"],
    }
  )
}

test "only the owner invites" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let tid <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let _ <- Db.insert Member { team: tid, user: bob }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Show { id: tid })
  let _ <- Test.login { user: Just bob }
  let denied <- Test.submit (SendInvite { id: tid }) { email: Text.Email "new@example.com" }
  let none <- Db.from Invite |> Db.count
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Show { id: tid })
  let ok <- Test.submit (SendInvite { id: tid }) { email: Text.Email "new@example.com" }
  let invites <- Db.from Invite |> Db.all
  Task.succeed (
    {
      denied: denied.answer,
      none: none,
      ok: ok.answer,
      invites: List.map (\i -> { email: i.email, status: i.status }) invites,
    } == {
      denied: PageMissing,
      none: 0,
      ok: Redirected (Show { id: tid }),
      invites: [{ email: Text.Email "new@example.com", status: Pending }],
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

route Index   = GET "/invites"               { }                       requires user
route Accept  = PUT "/invites/{id}/accepted" { id: Invite.InviteId }   requires user
route Decline = PUT "/invites/{id}/declined" { id: Invite.InviteId }   requires user

type Row = { id: Invite.InviteId, team: Text }

type Model = { rows: List Row }

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
      let teams <-
        Db.from Team
          |> Db.where (Team.id.in (List.map (\i -> i.team) invites))
          |> Db.all
      let rows : List Row =
        List.filterMap (\i -> Maybe.map (\t -> { id: i.id, team: t.name }) (List.find (\t -> t.id == i.team) teams)) invites
      Query.succeed { rows: List.sortByText (\r -> r.team) rows }
    },
  }
}

waiting : Invite.InviteId -> User.UserId -> Query Invite
waiting id uid = {
  let found <- Db.get Invite id
  let me <- Db.get User uid
  match found {
    Nothing -> Query.notFound,
    Just inv -> match me {
      Nothing -> Query.notFound,
      Just u ->
        if inv.status == Pending && inv.email == u.email then
          Query.succeed inv
        else
          Query.notFound,
    },
  }
}

addMember : Team.TeamId -> User.UserId -> Task Unit
addMember tid uid = {
  let _ <- Db.insert Member { team: tid, user: uid }
  Task.succeed ()
}

Accept.update : Accept.Params -> Session -> Task Response
Accept.update p session = {
  let inv <- waiting p.id session.user
  let team <- Db.get Team inv.team
  let n <-
    Db.from Member
      |> Db.where (Member.team.eq inv.team)
      |> Db.where (Member.user.eq session.user)
      |> Db.count
  match team {
    Nothing -> Task.notFound,
    Just t -> {
      let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
      let _ <- if n > 0 || t.owner == session.user then Task.succeed () else addMember inv.team session.user
      Task.succeed (Response.redirect (Teams.Show.Show { id: inv.team }))
    },
  }
}

Decline.update : Decline.Params -> Session -> Task Response
Decline.update p session = {
  let inv <- waiting p.id session.user
  let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
  Task.succeed (Response.redirect (Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <nav><a href={Teams.Index.Index {}}>Teams</a></nav>
    <ul id="invites">
      {for r in model.rows ->
        <li>
          <span class="team">{r.team}</span>
          <button click={Accept { id: r.id }}>Accept</button>
          <button click={Decline { id: r.id }}>Decline</button>
        </li>}
    </ul>
  </Layout.Page>

test "accepting an invite makes the team visible, declining does not" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let t1 <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let t2 <- Db.insert Team { name: Text.NonEmpty "Other", owner: ann }
  let i1 <- Db.insert Invite { team: t1, email: Text.Email "bob@example.com" }
  let i2 <- Db.insert Invite { team: t2, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let before <- Test.get (Index {})
  let accepted <- Test.click (Accept { id: i1 })
  let teams <- Test.get (Teams.Index.Index {})
  let opened <- Test.open (Teams.Show.Show { id: t1 })
  let _ <- Test.get (Index {})
  let declined <- Test.click (Decline { id: i2 })
  let teams2 <- Test.get (Teams.Index.Index {})
  let closed <- Test.open (Teams.Show.Show { id: t2 })
  let left <- Test.get (Index {})
  Task.succeed (
    {
      waiting: Test.texts "#invites .team" before.doc,
      accepted: accepted.answer,
      teams: Test.texts "#teams .name" teams.doc,
      opened: opened.answer,
      declined: declined.answer,
      teams2: Test.texts "#teams .name" teams2.doc,
      closed: closed.answer,
      left: Test.texts "#invites .team" left.doc,
    } == {
      waiting: ["Core", "Other"],
      accepted: Redirected (Teams.Show.Show { id: t1 }),
      teams: ["Core"],
      opened: Shown,
      declined: Redirected (Index {}),
      teams2: ["Core"],
      closed: PageMissing,
      left: [],
    }
  )
}

test "an invite that is not the user's cannot be answered" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let cat <- Db.insert User {
    email: Text.Email "cat@example.com",
    name: Text.NonEmpty "Cat",
    passwordHash: "",
  }
  let t1 <- Db.insert Team { name: Text.NonEmpty "Core", owner: ann }
  let i1 <- Db.insert Invite { team: t1, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Just cat }
  let r <- Test.click (Accept { id: i1 })
  let members <- Db.from Member |> Db.count
  Task.succeed ({ answer: r.answer, members: members } == { answer: PageMissing, members: 0 })
}
