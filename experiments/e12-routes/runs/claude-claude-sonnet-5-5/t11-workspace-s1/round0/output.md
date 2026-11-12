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
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
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
    {
      answer: r.answer,
      user: r.session.user,
      title: Test.text "h1" r.doc,
    } == {
      answer: Redirected (Orgs.Index.Index {}),
      user: Just me,
      title: Just "Your orgs",
    }
  )
}

--- file: app/Orgs/Index.alder ---
module Orgs.Index

import Layout
import Table.Membership
import Table.Org
import Table.User

route Index  = GET  "/orgs"                { }                  requires user
route Open   = POST "/orgs/{org}/open"     { org: Org.OrgId }   requires user
route Create = POST "/orgs"                { } form NewOrg      requires user

type Model = { orgs: List Org }

form NewOrg = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let memberships <-
    Db.from Membership
      |> Db.where (Membership.user.eq session.user)
      |> Db.all
  let ids = List.map (\m -> m.org) memberships
  let orgs <-
    Db.from Org
      |> Db.where (Org.id.in ids)
      |> Db.orderBy Org.name.asc
      |> Db.all
  Query.succeed { orgs: orgs }
}

-- The session refuses an org the user is no member of: `Session.put` fails the task.
Open.update : Open.Params -> Session -> Task Response
Open.update p session = {
  let _ <- Session.put (\s -> { s | org: Just p.org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert Org { name: p.form.name }
  let _ <- Db.insert Membership { user: session.user, org: id }
  let _ <- Session.put (\s -> { s | org: Just id })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Orgs">
    <h1>Your orgs</h1>
    <ul id="orgs">
      {for o in model.orgs ->
        <li>
          <span class="name">{o.name}</span>
          <button click={Open { org: o.id }}>Open</button>
        </li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <Errors form />
        <al-input field="name" label="New org" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "the orgs listed are the user's own, ordered by name" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let zeta <- Db.insert Org { name: Text.NonEmpty "Zeta" }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member ann zeta
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Nothing }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#orgs .name" page.doc == ["Acme", "Zeta"])
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
  let _ <- Test.login { user: Just ann, org: Nothing }
  let r <- Test.run (Open.update { org: globex } { user: ann, org: Nothing })
  Task.succeed (Result.toMaybe r.value == Nothing)
}

test "opening an org the user belongs to" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Nothing }
  let _ <- Test.get (Index {})
  let r <- Test.click (Open { org: acme })
  Task.succeed (
    { answer: r.answer, org: r.session.org, name: Test.text "#org" r.doc }
      == { answer: Redirected (Projects.Index.Index {}), org: Just acme, name: Just "Acme" }
  )
}

test "creating an org makes its creator a member and opens it" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just ann, org: Nothing }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Create {}) { name: Text.NonEmpty "Newco" }
  let n <- Db.from Membership |> Db.count
  Task.succeed (
    {
      answer: r.answer,
      opened: r.session.org != Nothing,
      members: n,
      org: Test.text "#org" r.doc,
    } == {
      answer: Redirected (Projects.Index.Index {}),
      opened: True,
      members: 1,
      org: Just "Newco",
    }
  )
}

--- file: app/Projects/Index.alder ---
module Projects.Index

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.User

route Index  = GET  "/projects" { }                   requires user, org
route Create = POST "/projects" { } form NewProject   requires user, org

type Model = { org: Org, projects: List Project }

form NewProject = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- Membership.memberOnly session.user session.org
  let org <- Db.get Org session.org
  let projects <-
    Db.scoped Project
      |> Db.orderBy Project.name.asc
      |> Db.all
  match org {
    Just o -> Query.succeed { org: o, projects: projects },
    Nothing -> Query.notFound,
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
    <h1 id="org">{model.org.name}</h1>
    <ul id="projects">
      {for p in model.projects ->
        <li><a class="name" href={Projects.Show.Show { id: p.id }}>{p.name}</a></li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <Errors form />
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
  let _ <- Db.insert Project { name: Text.NonEmpty "Anvil", org: acme }
  let _ <- Db.insert Project { name: Text.NonEmpty "Secret", org: globex }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let page <- Test.get (Index {})
  Task.succeed (
    { org: Test.text "#org" page.doc, names: Test.texts "#projects .name" page.doc }
      == { org: Just "Acme", names: ["Anvil", "Rocket"] }
  )
}

test "a member removed from the org loses access at once" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let mid <- Db.insert Membership { user: ann, org: acme }
  let _ <- Test.login { user: Just ann, org: Just acme }
  let before <- Test.open (Index {})
  let _ <- Db.delete Membership mid
  let after <- Test.open (Index {})
  Task.succeed (
    { before: before.answer, after: after.answer } == { before: Shown, after: PageMissing }
  )
}

test "creating a project opens its page" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Create {}) { name: Text.NonEmpty "Plan" }
  Task.succeed (Test.text "#name" r.doc == Just "Plan")
}

--- file: app/Projects/Show.alder ---
module Projects.Show

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.Todo
import Table.User

route Show   = GET   "/projects/{id}"              { id: Project.ProjectId }                         requires user, org
route Add    = POST  "/projects/{id}/todos"        { id: Project.ProjectId } form NewTodo            requires user, org
route Toggle = PATCH "/projects/{id}/todos/{todo}" { id: Project.ProjectId, todo: Todo.TodoId }      requires user, org

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
  let _ <- Db.getScoped Project p.id
  let t <- Db.getScoped Todo p.todo
  if t.project == p.id then {
    let _ <- Db.updateScoped Todo p.todo (\x -> { x | done: not x.done })
    Task.succeed (Response.redirect (Show { id: p.id }))
  } else {
    Task.notFound
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.project.name}>
    <h1 id="name">{model.project.name}</h1>
    <ul id="todos">
      {for t in model.todos -> <Item project={model.project} todo={t} />}
    </ul>
    <al-card>
      <form submit={Add { id: model.project.id }}>
        <Errors form />
        <al-input field="title" label="New to-do" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>
    <a href={Projects.Index.Index {}}>Projects</a>
  </Layout.Page>

Item : { project: Project, todo: Todo } -> Html
Item props =
  <li class={if props.todo.done then "done" else ""}>
    <span class="title">{props.todo.title}</span>
    <button click={Toggle { id: props.project.id, todo: props.todo.id }}>
      {if props.todo.done then "Reopen" else "Done"}
    </button>
  </li>

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

test "adding a to-do through the page" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let pid <- Db.insert Project { name: Text.NonEmpty "Rocket", org: acme }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let _ <- Test.get (Show { id: pid })
  let r <- Test.submit (Add { id: pid }) { title: Text.NonEmpty "Milk" }
  Task.succeed (
    { answer: r.answer, titles: Test.texts "#todos .title" r.doc }
      == { answer: Redirected (Show { id: pid }), titles: ["Milk"] }
  )
}

test "a to-do is marked done and open again" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let pid <- Db.insert Project { name: Text.NonEmpty "Rocket", org: acme }
  let tid <- Db.insert Todo { title: Text.NonEmpty "Milk", project: pid, org: acme }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let _ <- Test.get (Show { id: pid })
  let first <- Test.click (Toggle { id: pid, todo: tid })
  let second <- Test.click (Toggle { id: pid, todo: tid })
  Task.succeed (
    { first: Test.has "li.done" first.doc, second: Test.has "li.done" second.doc }
      == { first: True, second: False }
  )
}

test "a removed member can neither read nor add" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let pid <- Db.insert Project { name: Text.NonEmpty "Rocket", org: acme }
  let mid <- Db.insert Membership { user: ann, org: acme }
  let _ <- Test.login { user: Just ann, org: Just acme }
  let _ <- Test.get (Show { id: pid })
  let _ <- Db.delete Membership mid
  let added <- Test.submit (Add { id: pid }) { title: Text.NonEmpty "Milk" }
  let opened <- Test.open (Show { id: pid })
  Task.succeed (
    { added: added.answer, opened: opened.answer }
      == { added: PageMissing, opened: PageMissing }
  )
}
