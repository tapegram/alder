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
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

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

-- Stops with a 404 unless the user owns the team: a write route, which runs no `load`,
-- begins with it.
ownerOnly : User.UserId -> TeamId -> Query Unit
ownerOnly user id = {
  let found <- Db.get Team id
  if Maybe.map (\t -> t.owner) found == Just user then Query.succeed () else Query.notFound
}

--- file: app/Table/Member.alder ---
module Table.Member

import Table.Team
import Table.User

type MemberId = brand Text

-- One row per member who is not the owner: the owner has no `Member` row.
table Member {
  id: MemberId @id,
  team: Team.TeamId @ref Team,
  user: User.UserId @ref User,
}

-- Whether the user owns the team or is one of its members.
canSee : User.UserId -> Team.TeamId -> Query Bool
canSee user id = {
  let found <- Db.get Team id
  let n <-
    Db.from Member
      |> Db.where (Member.team.eq id)
      |> Db.where (Member.user.eq user)
      |> Db.count
  Query.succeed (Maybe.map (\t -> t.owner) found == Just user || n > 0)
}

join : Team.TeamId -> User.UserId -> Task Unit
join team user = {
  let _ <- Db.insert Member { team: team, user: user }
  Task.succeed ()
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

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let visitor <- Test.open (Teams.Index.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Teams.Index.Index {})
  let out <- Test.click App.Logout
  Task.succeed (
    {
      visitor: visitor.answer,
      out: out.answer,
      user: out.session.user,
    } == {
      visitor: Redirected (Login {}),
      out: Redirected (Login {}),
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

route Index  = GET  "/teams" { }              requires user
route Create = POST "/teams" { } form NewTeam requires user

type Model = { teams: List Team }

-- The teams the user owns, and those they joined by accepting an invite.
Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let memberships <-
    Db.from Member
      |> Db.where (Member.user.eq session.user)
      |> Db.all
  let teams <-
    Db.from Team
      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)])
      |> Db.orderBy Team.name.asc
      |> Db.all
  Query.succeed { teams: teams }
}

form NewTeam = { name: Text.NonEmpty }

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
      {for t in model.teams -> <li><a class="name" href={Teams.Show.Show { id: t.id }}>{t.name}</a></li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New team" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <nav>
      <a href={Invites.Index.Index {}}>Invites</a>
    </nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "creating a team lists it for its owner, by name" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Team { name: Text.NonEmpty "Ants", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let created <- Test.submit (Create {}) { name: Text.NonEmpty "Zebras" }
  let page <- Test.get (Index {})
  Task.succeed (
    {
      shown: Test.text "#name" created.doc,
      teams: Test.texts "#teams .name" page.doc,
    } == {
      shown: Just "Zebras",
      teams: ["Ants", "Zebras"],
    }
  )
}

test "a member sees the team, a stranger does not" = {
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
  let stranger <- Db.insert User {
    email: Text.Email "stranger@example.com",
    name: Text.NonEmpty "Stranger",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: ann }
  let _ <- Test.login { user: Just ann }
  let annPage <- Test.get (Index {})
  let _ <- Test.login { user: Just stranger }
  let strangerPage <- Test.get (Index {})
  Task.succeed (
    {
      ann: Test.texts "#teams .name" annPage.doc,
      stranger: Test.count "#teams li" strangerPage.doc,
    } == {
      ann: ["Red"],
      stranger: 0,
    }
  )
}

--- file: app/Teams/Show.alder ---
module Teams.Show

import Layout
import Table.Invite
import Table.Member
import Table.Team
import Table.User

route Show       = GET    "/teams/{id}"                { id: Team.TeamId }                                requires user
route SendInvite = POST   "/teams/{id}/invites"        { id: Team.TeamId }                 form NewInvite requires user
route Remove     = DELETE "/teams/{id}/members/{user}" { id: Team.TeamId, user: User.UserId }                requires user

type Model = {
  team: Team,
  isOwner: Bool,
  owner: Maybe User,
  others: List User,
}

-- The page is the owner's and the members' only: anyone else gets a 404.
Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let allowed <- Member.canSee session.user params.id
  let found <- Db.get Team params.id
  match found {
    Nothing -> Query.notFound,
    Just team -> if allowed then members session.user team else Query.notFound,
  }
}

members : User.UserId -> Team -> Query Model
members me team = {
  let owner <- Db.get User team.owner
  let rows <-
    Db.from Member
      |> Db.where (Member.team.eq team.id)
      |> Db.all
  let others <-
    Db.from User
      |> Db.where (User.id.in (List.map (\m -> m.user) rows))
      |> Db.orderBy User.name.asc
      |> Db.all
  Query.succeed {
    team: team,
    isOwner: team.owner == me,
    owner: owner,
    others: others,
  }
}

form NewInvite = { email: Text.Email }

-- No `load` runs before a write route: each begins by checking that the sender owns the team.
SendInvite.update : SendInvite.Params -> Session -> Task Response
SendInvite.update p session = {
  let _ <- Team.ownerOnly session.user p.id
  let _ <- Db.insert Invite { team: p.id, email: p.form.email }
  Task.succeed (Response.redirect (Show { id: p.id }))
}

Remove.update : Remove.Params -> Session -> Task Response
Remove.update p session = {
  let _ <- Team.ownerOnly session.user p.id
  if p.user == session.user then Task.notFound
  else {
    let row <-
      Db.from Member
        |> Db.where (Member.team.eq p.id)
        |> Db.where (Member.user.eq p.user)
        |> Db.first
    match row {
      Just m -> {
        let _ <- Db.delete Member m.id
        Task.succeed (Response.redirect (Show { id: p.id }))
      },
      Nothing -> Task.notFound,
    }
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.team.name}>
    <h1 id="name">{model.team.name}</h1>
    <ul id="members">
      {for u in Maybe.toList model.owner -> <li><span class="name">{u.name}</span> (owner)</li>}
      {for u in model.others ->
        <li>
          <span class="name">{u.name}</span>
          {if model.isOwner then
            <button class="remove" click={Remove { id: model.team.id, user: u.id }}>Remove</button>
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
    <nav>
      <a href={Teams.Index.Index {}}>Teams</a>
    </nav>
  </Layout.Page>

test "a team's page is not found for a user who is not a member" = {
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
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Show { id: team })
  let _ <- Test.login { user: Just owner }
  let mine <- Test.open (Show { id: team })
  Task.succeed (
    { stranger: r.answer, owner: mine.answer } == { stranger: PageMissing, owner: Shown }
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
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: alice }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Test.login { user: Just alice }
  let page <- Test.get (Show { id: team })
  let r <- Test.run (Remove.update { id: team, user: bob } { user: alice })
  let left <-
    Db.from Member
      |> Db.where (Member.team.eq team)
      |> Db.count
  Task.succeed (
    {
      buttons: Test.count ".remove" page.doc,
      value: r.value,
      members: left,
    } == {
      buttons: 0,
      value: Err "not found",
      members: 2,
    }
  )
}

test "the owner removes a member but cannot remove themselves" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Zed",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let alice <- Db.insert User {
    email: Text.Email "alice@example.com",
    name: Text.NonEmpty "Alice",
    passwordHash: "",
  }
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: bob }
  let _ <- Db.insert Member { team: team, user: alice }
  let _ <- Test.login { user: Just owner }
  let before <- Test.get (Show { id: team })
  let r <- Test.click (Remove { id: team, user: bob })
  let self <- Test.run (Remove.update { id: team, user: owner } { user: owner })
  let after <- Test.get (Show { id: team })
  Task.succeed (
    {
      before: Test.texts "#members .name" before.doc,
      buttons: Test.count "#members .remove" before.doc,
      answer: r.answer,
      self: self.value,
      after: Test.texts "#members .name" after.doc,
    } == {
      before: ["Zed", "Alice", "Bob"],
      buttons: 2,
      answer: Redirected (Show { id: team }),
      self: Err "not found",
      after: ["Zed", "Alice"],
    }
  )
}

test "only the owner invites" = {
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
  let team <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let _ <- Db.insert Member { team: team, user: alice }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Show { id: team })
  let sent <- Test.submit (SendInvite { id: team }) { email: Text.Email "new@example.com" }
  let _ <- Test.login { user: Just alice }
  let page <- Test.get (Show { id: team })
  let r <-
    Test.run (SendInvite.update { id: team, form: { email: Text.Email "x@example.com" } } { user: alice })
  let waiting <-
    Db.from Invite
      |> Db.where (Invite.status.eq Pending)
      |> Db.count
  Task.succeed (
    {
      sent: sent.answer,
      form: Test.has "form" page.doc,
      value: r.value,
      invites: waiting,
    } == {
      sent: Redirected (Show { id: team }),
      form: False,
      value: Err "not found",
      invites: 1,
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

route Index   = GET "/invites"               { }                     requires user
route Accept  = PUT "/invites/{id}/accepted" { id: Invite.InviteId } requires user
route Decline = PUT "/invites/{id}/declined" { id: Invite.InviteId } requires user

type Waiting = { id: Invite.InviteId, team: Text }

type Model = { invites: List Waiting }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.succeed { invites: [] },
    Just u -> pending u.email,
  }
}

pending : Text.Email -> Query Model
pending email = {
  let invites <-
    Db.from Invite
      |> Db.where (Invite.email.eq email)
      |> Db.where (Invite.status.eq Pending)
      |> Db.all
  let teams <-
    Db.from Team
      |> Db.where (Team.id.in (List.map (\i -> i.team) invites))
      |> Db.all
  Query.succeed { invites: List.sortByText (\w -> w.team) (List.filterMap (waiting teams) invites) }
}

waiting : List Team -> Invite -> Maybe Waiting
waiting teams invite =
  List.find (\t -> t.id == invite.team) teams |> Maybe.map (\t -> { id: invite.id, team: t.name })

-- The invite, when it is to the user's email and still waiting; otherwise a 404.
waitingFor : User.UserId -> Invite.InviteId -> Query Invite
waitingFor user id = {
  let me <- Db.get User user
  let found <- Db.get Invite id
  match found {
    Just invite ->
      if invite.status == Pending && Maybe.map (\u -> u.email) me == Just invite.email then
        Query.succeed invite
      else Query.notFound,
    Nothing -> Query.notFound,
  }
}

Accept.update : Accept.Params -> Session -> Task Response
Accept.update p session = {
  let invite <- waitingFor session.user p.id
  let already <- Member.canSee session.user invite.team
  let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
  let _ <- if already then Task.succeed () else Member.join invite.team session.user
  Task.succeed (Response.redirect (Teams.Show.Show { id: invite.team }))
}

Decline.update : Decline.Params -> Session -> Task Response
Decline.update p session = {
  let invite <- waitingFor session.user p.id
  let _ <- Db.update Invite invite.id (\i -> { i | status: Declined })
  Task.succeed (Response.redirect (Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Invites">
    <h1>Invites</h1>
    <ul id="invites">
      {for i in model.invites ->
        <li>
          <span class="team">{i.team}</span>
          <button class="accept" click={Accept { id: i.id }}>Accept</button>
          <button class="decline" click={Decline { id: i.id }}>Decline</button>
        </li>}
    </ul>
    <nav>
      <a href={Teams.Index.Index {}}>Teams</a>
    </nav>
  </Layout.Page>

test "accepting an invite makes the team visible, declining does not" = {
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
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: owner }
  let toRed <- Db.insert Invite { team: red, email: Text.Email "ann@example.com" }
  let toBlue <- Db.insert Invite { team: blue, email: Text.Email "ann@example.com" }
  let _ <- Test.login { user: Just ann }
  let before <- Test.get (Index {})
  let accepted <- Test.click (Accept { id: toRed })
  let _ <- Test.get (Index {})
  let declined <- Test.click (Decline { id: toBlue })
  let teams <- Test.get (Teams.Index.Index {})
  let redPage <- Test.open (Teams.Show.Show { id: red })
  let bluePage <- Test.open (Teams.Show.Show { id: blue })
  let again <- Test.run (Accept.update { id: toBlue } { user: ann })
  Task.succeed (
    {
      before: Test.texts "#invites .team" before.doc,
      accepted: accepted.answer,
      declined: declined.answer,
      left: Test.count "#invites li" declined.doc,
      teams: Test.texts "#teams .name" teams.doc,
      red: redPage.answer,
      blue: bluePage.answer,
      again: again.value,
    } == {
      before: ["Blue", "Red"],
      accepted: Redirected (Teams.Show.Show { id: red }),
      declined: Redirected (Index {}),
      left: 0,
      teams: ["Red"],
      red: Shown,
      blue: PageMissing,
      again: Err "not found",
    }
  )
}

test "an invite to another email cannot be answered" = {
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
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: owner }
  let invite <- Db.insert Invite { team: red, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Index {})
  let r <- Test.run (Accept.update { id: invite } { user: ann })
  let joined <-
    Db.from Member
      |> Db.where (Member.user.eq ann)
      |> Db.count
  Task.succeed (
    {
      items: Test.count "#invites li" page.doc,
      value: r.value,
      joined: joined,
    } == {
      items: 0,
      value: Err "not found",
      joined: 0,
    }
  )
}
