--- file: app/App.alder ---
module App

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.Todo
import Table.User

session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership }

login = Auth.Login.Login

choose org = Orgs.Index.Index

silo org { Project, Todo }

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing, org: Nothing })
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

--- file: app/Table/Org.alder ---
module Table.Org

type OrgId = brand Text

table Org {
  id: OrgId @id,
  name: Text.NonEmpty,
}

--- file: app/Table/Membership.alder ---
module Table.Membership

import Table.Org
import Table.User

type MembershipId = brand Text

table Membership {
  id: MembershipId @id,
  user: User.UserId @ref User,
  org: Org.OrgId @ref Org,
}

isMember : User.UserId -> Org.OrgId -> Query Bool
isMember user org = {
  let n <-
    Db.from Membership
      |> Db.where (Membership.user.eq user)
      |> Db.where (Membership.org.eq org)
      |> Db.count
  Query.succeed (n > 0)
}

memberOnly : User.UserId -> Org.OrgId -> Query Unit
memberOnly user org = {
  let member <- isMember user org
  if member then Query.succeed () else Query.notFound
}

--- file: app/Table/Project.alder ---
module Table.Project

import Table.Org

type ProjectId = brand Text

table Project {
  id: ProjectId @id,
  name: Text.NonEmpty,
  org: Org.OrgId @ref Org @partition org,
}

--- file: app/Table/Todo.alder ---
module Table.Todo

import Table.Org
import Table.Project

type TodoId = brand Text

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  project: Project.ProjectId @ref Project @cascade,
  org: Org.OrgId @ref Org @partition org,
}

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
      let _ <- Session.put (\s -> { s | user: Just id, org: Nothing })
      Task.succeed (Response.redirect (Orgs.Index.Index {}))
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

test "logging in goes to the orgs" = {
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
      == { answer: Redirected (Orgs.Index.Index {}), user: Just me }
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
    r.answer == FormRejected [{ field: "", message: "Email or password is incorrect." }]
  )
}

--- file: app/Orgs/Index.alder ---
module Orgs.Index

import Layout
import Table.Membership
import Table.Org
import Table.User

route Index  = GET  "/orgs"         { }                requires user
route Open   = PUT  "/session/org"  { org: Org.OrgId } requires user
route Create = POST "/orgs"         { } form NewOrg    requires user

type Model = { orgs: List Org }

form NewOrg = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let memberships <-
    Db.from Membership
      |> Db.where (Membership.user.eq session.user)
      |> Db.all
  let orgs <-
    Db.from Org
      |> Db.where (Org.id.in (List.map (\m -> m.org) memberships))
      |> Db.orderBy Org.name.asc
      |> Db.all
  Query.succeed { orgs: orgs }
}

-- Setting an org the user is not a member of fails the task: the session's `via Membership`
-- refuses it.
Open.update : Open.Params -> Session -> Task Response
Open.update p session = {
  let _ <- Session.put (\s -> { s | org: Just p.org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let org <- Db.insert Org { name: p.form.name }
  let _ <- Db.insert Membership { user: session.user, org: org }
  let _ <- Session.put (\s -> { s | org: Just org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Orgs">
    <h1>Orgs</h1>
    <ul id="orgs">
      {for o in model.orgs ->
        <li>
          <span class="name">{o.name}</span>
          <al-button size="sm" click={Open { org: o.id }}>Open</al-button>
        </li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New org" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "the orgs listed are the user's own, by name" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let zed <- Db.insert Org { name: Text.NonEmpty "Zed" }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member ann zed
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#orgs .name" page.doc == ["Acme", "Zed"])
}

test "a user cannot open an org they are not a member of" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann }
  let refused <- Test.run (Open.update { org: globex } { user: ann, org: Nothing })
  let allowed <- Test.run (Open.update { org: acme } { user: ann, org: Nothing })
  Task.succeed (
    { refused: Result.toMaybe refused.value, allowed: Result.toMaybe allowed.value }
      == { refused: Nothing, allowed: Just (Response.redirect (Projects.Index.Index {})) }
  )
}

test "creating an org makes its creator a member and opens it" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Create {}) { name: Text.NonEmpty "Acme" }
  let members <- Db.from Membership |> Db.count
  Task.succeed (
    { answer: r.answer, opened: Maybe.isJust r.session.org, members: members }
      == { answer: Redirected (Projects.Index.Index {}), opened: True, members: 1 }
  )
}

--- file: app/Projects/Index.alder ---
module Projects.Index

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.User

route Index  = GET  "/projects" { }                    requires user, org
route Create = POST "/projects" { } form NewProject    requires user, org

type Model = { org: Text, projects: List Project }

form NewProject = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- Membership.memberOnly session.user session.org
  let found <- Db.get Org session.org
  match found {
    Nothing -> Query.notFound,
    Just o -> {
      let projects <-
        Db.scoped Project
          |> Db.orderBy Project.name.asc
          |> Db.all
      Query.succeed { org: o.name, projects: projects }
    },
  }
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let id <- Db.insertScoped Project { name: p.form.name }
  Task.succeed (Response.redirect (Projects.Show.Show { id: id }))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Projects">
    <h1 id="org">{model.org}</h1>
    <ul id="projects">
      {for pr in model.projects ->
        <li><a class="name" href={Projects.Show.Show { id: pr.id }}>{pr.name}</a></li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New project" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <a href={Orgs.Index.Index {}}>Orgs</a>
  </Layout.Page>

test "a member sees only their org's projects" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Db.insert Project { name: Text.NonEmpty "Rocket", org: acme }
  let _ <- Db.insert Project { name: Text.NonEmpty "Secret", org: globex }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let page <- Test.get (Index {})
  Task.succeed (
    { org: Test.text "#org" page.doc, names: Test.texts "#projects .name" page.doc }
      == { org: Just "Acme", names: ["Rocket"] }
  )
}

test "a member removed from the org loses access at once" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Db.insert Project { name: Text.NonEmpty "Rocket", org: acme }
  let m <- Db.insert Membership { user: ann, org: acme }
  let _ <- Test.login { user: Just ann, org: Just acme }
  let before <- Test.open (Index {})
  let _ <- Db.delete Membership m
  let after <- Test.open (Index {})
  Task.succeed (
    { before: before.answer, after: after.answer } == { before: Shown, after: PageMissing }
  )
}

test "a user without an org is sent to the orgs" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just ann }
  let r <- Test.open (Index {})
  Task.succeed (r.answer == Redirected (Orgs.Index.Index {}))
}

test "creating a project goes to its page" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Create {}) { name: Text.NonEmpty "Rocket" }
  Task.succeed (Test.text "#name" r.doc == Just "Rocket")
}

--- file: app/Projects/Show.alder ---
module Projects.Show

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.Todo
import Table.User

route Show   = GET   "/projects/{id}"                 { id: Project.ProjectId }                         requires user, org
route Add    = POST  "/projects/{id}/todos"           { id: Project.ProjectId } form NewTodo            requires user, org
route Toggle = PATCH "/projects/{id}/todos/{todo}"    { id: Project.ProjectId, todo: Todo.TodoId }      requires user, org

type Model = { project: Project, todos: List Todo }

form NewTodo = { title: Text.NonEmpty }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let _ <- Membership.memberOnly session.user session.org
  let project <- Db.getScoped Project params.id
  let todos <-
    Db.scoped Todo
      |> Db.where (Todo.project.eq params.id)
      |> Db.orderBy Todo.title.asc
      |> Db.all
  Query.succeed { project: project, todos: todos }
}

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let _ <- Db.getScoped Project p.id
  let _ <- Db.insertScoped Todo { title: p.form.title, project: p.id }
  Task.succeed (Response.redirect (Show { id: p.id }))
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let todo <- Db.getScoped Todo p.todo
  if todo.project == p.id then {
    let _ <- Db.updateScoped Todo p.todo (\t -> { t | done: not t.done })
    Task.succeed (Response.redirect (Show { id: p.id }))
  } else
    Task.notFound
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.project.name}>
    <h1 id="name">{model.project.name}</h1>
    <ul id="todos">
      {for t in model.todos ->
        <li class={if t.done then "done" else ""}>
          <span class="title">{t.title}</span>
          <al-button size="sm" click={Toggle { id: model.project.id, todo: t.id }}>
            {if t.done then "Open again" else "Done"}
          </al-button>
        </li>}
    </ul>
    <al-card>
      <form submit={Add { id: model.project.id }}>
        <al-input field="title" label="New to-do" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>
    <a href={Projects.Index.Index {}}>Projects</a>
  </Layout.Page>

test "a project of another org is not found" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let secret <- Db.insert Project { name: Text.NonEmpty "Secret", org: globex }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let r <- Test.open (Show { id: secret })
  Task.succeed (r.answer == PageMissing)
}

test "a member removed from the org loses the project page at once" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let pr <- Db.insert Project { name: Text.NonEmpty "Rocket", org: acme }
  let m <- Db.insert Membership { user: ann, org: acme }
  let _ <- Test.login { user: Just ann, org: Just acme }
  let before <- Test.open (Show { id: pr })
  let _ <- Db.delete Membership m
  let after <- Test.open (Show { id: pr })
  Task.succeed (
    { before: before.answer, after: after.answer } == { before: Shown, after: PageMissing }
  )
}

test "adding and toggling a to-do" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let pr <- Db.insert Project { name: Text.NonEmpty "Rocket", org: acme }
  let t <- Db.insert Todo { title: Text.NonEmpty "Write", project: pr, org: acme }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let _ <- Test.get (Show { id: pr })
  let added <- Test.submit (Add { id: pr }) { title: Text.NonEmpty "Ship" }
  let toggled <- Test.click (Toggle { id: pr, todo: t })
  Task.succeed (
    {
      added: added.answer,
      toggled: toggled.answer,
      titles: Test.texts "#todos .title" toggled.doc,
      done: Test.count "li.done" toggled.doc,
    } == {
      added: Redirected (Show { id: pr }),
      toggled: Redirected (Show { id: pr }),
      titles: ["Ship", "Write"],
      done: 1,
    }
  )
}
