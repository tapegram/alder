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
    { answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc }
      == { answer: Redirected (Teams.Index.Index {}), user: Just me, title: Just "Teams" }
  )
}

--- file: app/Teams/Index.alder ---
module Teams.Index

import Layout
import Table.Member
import Table.Team
import Table.User

route Index  = GET  "/teams" { }                  requires user
route Create = POST "/teams" { } form NewTeam     requires user

type Model = { teams: List Team }

form NewTeam = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let owned <- Db.from Team |> Db.where (Team.owner.eq session.user) |> Db.all
  let rows <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let joined <- Db.from Team |> Db.where (Team.id.in (List.map (\m -> m.team) rows)) |> Db.all
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
    <ul id="teams">
      {for t in model.teams ->
        <li><a class="name" href={Teams.Show.Show { id: t.id }}>{t.name}</a></li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <Errors form />
        <al-input field="name" label="Team name" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <nav><a href={Invites.Index.Index {}}>Invites</a></nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a created team is listed and owned by its creator" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.submit (Create {}) { name: Text.NonEmpty "Alpha" }
  let page <- Test.get (Index {})
  let teams <- Db.from Team |> Db.all
  Task.succeed (
    { owners: List.map (\t -> t.owner) teams, names: Test.texts "#teams .name" page.doc }
      == { owners: [me], names: ["Alpha"] }
  )
}

test "the list has owned and joined teams only, by name" = {
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
  let _ <- Db.insert Team { name: Text.NonEmpty "Zed", owner: me }
  let joined <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: other }
  let _ <- Db.insert Team { name: Text.NonEmpty "Hidden", owner: other }
  let _ <- Db.insert Member { team: joined, user: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Alpha", "Zed"])
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Layout
import Table.Invite
import Table.Member
import Table.Team
import Table.User

route Show       = GET    "/teams/{id}"                  { id: Team.TeamId }                          requires user
route SendInvite = POST   "/teams/{id}/invites"          { id: Team.TeamId } form NewInvite           requires user
route Remove     = DELETE "/teams/{id}/members/{user}"   { id: Team.TeamId, user: User.UserId }       requires user

type Model = { team: Team, members: List User }

form NewInvite = { email: Text.Email }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let ids = List.map (\m -> m.user) rows
      if team.owner != session.user && not (List.member session.user ids) then
        Query.notFound
      else {
        let owners <- Db.from User |> Db.where (User.id.eq team.owner) |> Db.all
        let others <- Db.from User |> Db.where (User.id.in ids) |> Db.all
        Query.succeed { team: team, members: owners ++ List.sortByText (\u -> u.name) others }
      }
    },
  }
}

SendInvite.update : SendInvite.Params -> Session -> Task Response
SendInvite.update p session = {
  let found <- Db.get Team p.id
  match found {
    Nothing -> Task.notFound,
    Just team ->
      if team.owner != session.user then
        Task.notFound
      else {
        let _ <- Db.insert Invite { team: team.id, email: p.form.email }
        Task.succeed (Response.redirect (Show { id: team.id }))
      },
  }
}

Remove.update : Remove.Params -> Session -> Task Response
Remove.update p session = {
  let found <- Db.get Team p.id
  match found {
    Nothing -> Task.notFound,
    Just team ->
      if team.owner != session.user || p.user == team.owner then
        Task.notFound
      else {
        let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.where (Member.user.eq p.user) |> Db.all
        let _ <- Task.all (List.map (\m -> Db.delete Member m.id) rows)
        Task.succeed (Response.redirect (Show { id: team.id }))
      },
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      {for u in model.members ->
        <li>
          <span class="name">{u.name}</span>
          {if model.team.owner == session.user && u.id != model.team.owner then
            <button class="remove" click={Remove { id: model.team.id, user: u.id }}>Remove</button>
          else Html.none}
        </li>}
    </ul>
    {if model.team.owner == session.user then
      <al-card>
        <form submit={SendInvite { id: model.team.id }}>
          <Errors form />
          <al-input field="email" label="Invite by email" />
          <al-button type="submit">Invite</al-button>
        </form>
      </al-card>
    else Html.none}
    <nav><a href={Teams.Index.Index {}}>Back</a></nav>
  </Layout.Page>

test "a team's page is not found for a non-member" = {
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
  let id <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "the owner and a member see the members, owner first" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let id <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let _ <- Db.insert Member { team: id, user: ann }
  let _ <- Test.login { user: Just owner }
  let asOwner <- Test.get (Show { id: id })
  let _ <- Test.login { user: Just ann }
  let asMember <- Test.get (Show { id: id })
  Task.succeed (
    {
      owner: Test.texts "#members .name" asOwner.doc,
      member: Test.texts "#members .name" asMember.doc,
      name: Test.text "#name" asMember.doc,
      removeForMember: Test.count "button.remove" asMember.doc,
      removeForOwner: Test.count "button.remove" asOwner.doc,
    } == {
      owner: ["Owner", "Ann"],
      member: ["Owner", "Ann"],
      name: Just "Alpha",
      removeForMember: 0,
      removeForOwner: 1,
    }
  )
}

test "a member cannot remove another member" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
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
  let id <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let _ <- Db.insert Member { team: id, user: ann }
  let _ <- Db.insert Member { team: id, user: bob }
  let r <- Test.run (Remove.update { id: id, user: bob } { user: ann })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ value: r.value, members: n } == { value: Err "not found", members: 2 })
}

test "the owner removes a member but not themselves" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let id <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let _ <- Db.insert Member { team: id, user: ann }
  let _ <- Test.login { user: Just owner }
  let page <- Test.get (Show { id: id })
  let r <- Test.click (Remove { id: id, user: ann })
  let self <- Test.run (Remove.update { id: id, user: owner } { user: owner })
  let n <- Db.from Member |> Db.count
  let after <- Test.get (Show { id: id })
  Task.succeed (
    {
      buttons: Test.count "button.remove" page.doc,
      answer: r.answer,
      self: self.value,
      members: n,
      names: Test.texts "#members .name" after.doc,
    } == {
      buttons: 1,
      answer: Redirected (Show { id: id }),
      self: Err "not found",
      members: 0,
      names: ["Owner"],
    }
  )
}

test "only the owner invites" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let id <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let _ <- Db.insert Member { team: id, user: ann }
  let denied <- Test.run (
    SendInvite.update { id: id, form: { email: Text.Email "x@example.com" } } { user: ann }
  )
  let none <- Db.from Invite |> Db.count
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Show { id: id })
  let sent <- Test.submit (SendInvite { id: id }) { email: Text.Email "x@example.com" }
  let invites <- Db.from Invite |> Db.all
  Task.succeed (
    {
      denied: denied.value,
      none: none,
      answer: sent.answer,
      emails: List.map (\i -> i.email) invites,
      status: List.map (\i -> i.status) invites,
    } == {
      denied: Err "not found",
      none: 0,
      answer: Redirected (Show { id: id }),
      emails: [Text.Email "x@example.com"],
      status: [Pending],
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

route Index   = GET "/invites"                { }                      requires user
route Accept  = PUT "/invites/{id}/accepted"  { id: Invite.InviteId }  requires user
route Decline = PUT "/invites/{id}/declined"  { id: Invite.InviteId }  requires user

type Model = { invites: List Invite, teams: List Team }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> {
      let invites <- Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all
      let teams <- Db.from Team |> Db.where (Team.id.in (List.map (\i -> i.team) invites)) |> Db.all
      Query.succeed { invites: invites, teams: teams }
    },
  }
}

waiting : Invite -> User -> Bool
waiting inv u = inv.email == u.email && inv.status == Pending

acceptInvite : Invite -> User -> Task Response
acceptInvite inv u = {
  let _ <- Db.update Invite inv.id (\i -> { i | status: Accepted })
  let found <- Db.get Team inv.team
  match found {
    Nothing -> Task.notFound,
    Just t -> {
      let n <- Db.from Member |> Db.where (Member.team.eq t.id) |> Db.where (Member.user.eq u.id) |> Db.count
      if t.owner == u.id || n > 0 then
        Task.succeed (Response.redirect (Teams.Show.Show { id: t.id }))
      else {
        let _ <- Db.insert Member { team: t.id, user: u.id }
        Task.succeed (Response.redirect (Teams.Show.Show { id: t.id }))
      }
    },
  }
}

Accept.update : Accept.Params -> Session -> Task Response
Accept.update p session = {
  let found <- Db.get Invite p.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> match me {
      Nothing -> Task.notFound,
      Just u -> if waiting inv u then acceptInvite inv u else Task.notFound,
    },
  }
}

Decline.update : Decline.Params -> Session -> Task Response
Decline.update p session = {
  let found <- Db.get Invite p.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just inv -> match me {
      Nothing -> Task.notFound,
      Just u ->
        if waiting inv u then {
          let _ <- Db.update Invite inv.id (\i -> { i | status: Declined })
          Task.succeed (Response.redirect (Index {}))
        } else Task.notFound,
    },
  }
}

row : List Team -> Invite -> Html
row teams inv = match List.find (\t -> t.id == inv.team) teams {
  Just t ->
    <li>
      <span class="team">{t.name}</span>
      <button click={Accept { id: inv.id }}>Accept</button>
      <button click={Decline { id: inv.id }}>Decline</button>
    </li>,
  Nothing -> Html.none,
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for inv in model.invites -> row model.teams inv}
    </ul>
    <nav><a href={Teams.Index.Index {}}>Teams</a></nav>
  </Layout.Page>

test "accepting an invite opens the team" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let guest <- Db.insert User {
    email: Text.Email "guest@example.com",
    name: Text.NonEmpty "Guest",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "guest@example.com" }
  let _ <- Test.login { user: Just guest }
  let page <- Test.get (Index {})
  let r <- Test.click (Accept { id: inv })
  let teams <- Test.get (Teams.Index.Index {})
  let show <- Test.open (Teams.Show.Show { id: team })
  let again <- Test.get (Index {})
  Task.succeed (
    {
      waiting: Test.texts "#invites .team" page.doc,
      answer: r.answer,
      teams: Test.texts "#teams .name" teams.doc,
      show: show.answer,
      left: Test.count "#invites li" again.doc,
    } == {
      waiting: ["Alpha"],
      answer: Redirected (Teams.Show.Show { id: team }),
      teams: ["Alpha"],
      show: Shown,
      left: 0,
    }
  )
}

test "declining an invite keeps the team closed" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let guest <- Db.insert User {
    email: Text.Email "guest@example.com",
    name: Text.NonEmpty "Guest",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "guest@example.com" }
  let _ <- Test.login { user: Just guest }
  let _ <- Test.get (Index {})
  let r <- Test.click (Decline { id: inv })
  let teams <- Test.get (Teams.Index.Index {})
  let show <- Test.open (Teams.Show.Show { id: team })
  let again <- Test.get (Index {})
  Task.succeed (
    {
      answer: r.answer,
      teams: Test.count "#teams li" teams.doc,
      show: show.answer,
      left: Test.count "#invites li" again.doc,
    } == {
      answer: Redirected (Index {}),
      teams: 0,
      show: PageMissing,
      left: 0,
    }
  )
}

test "an invite that is not yours cannot be answered" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Alpha", owner: owner }
  let inv <- Db.insert Invite { team: team, email: Text.Email "guest@example.com" }
  let r <- Test.run (Accept.update { id: inv } { user: other })
  let n <- Db.from Member |> Db.count
  Task.succeed ({ value: r.value, members: n } == { value: Err "not found", members: 0 })
}
