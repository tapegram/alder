--- file: app/App.alder ---
module App

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.Todo
import Table.User

-- The org a user works in is part of the session. `via Membership` makes the server refuse a
-- `Session.put` of any org the user is no member of, whatever the request says.
session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership }

login = Auth.Login.Login

choose org = Orgs.Index.Index

-- Each org's projects and to-dos live in a database of the org's own.
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

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me"), org: Nothing })
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

--- file: app/Table/Org.alder ---
module Table.Org

type OrgId = brand Text

table Org { id: OrgId @id, name: Text.NonEmpty }

--- file: app/Table/Membership.alder ---
module Table.Membership

import Table.Org
import Table.User

type MembershipId = brand Text

-- One row per member of an org.
table Membership {
  id: MembershipId @id,
  user: User.UserId @ref User,
  org: Org.OrgId @ref Org,
}

-- The session keeps the org it was given; a page that must notice a removed member at once
-- reads the membership again (the re-read idiom).
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

test "logging in through the form goes to the orgs" = {
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
      title: Just "Orgs",
    }
  )
}

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let visitor <- Test.open (Orgs.Index.Index {})
  let _ <- Test.login { user: Just me, org: Nothing }
  let _ <- Test.get (Orgs.Index.Index {})
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

--- file: app/Orgs/Index.alder ---
module Orgs.Index

import Layout
import Table.Membership
import Table.Org
import Table.User

route Index  = GET  "/orgs"              { }                          requires user
route Open   = PUT  "/orgs/{org}/opened" { org: Org.OrgId }           requires user
route Create = POST "/orgs"              { }              form NewOrg requires user

type Model = { orgs: List Org }

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

-- The session's `via Membership` refuses an org the user is no member of: the Task fails.
Open.update : Open.Params -> Session -> Task Response
Open.update p session = {
  let _ <- Session.put (\s -> { s | org: Just p.org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}

form NewOrg = { name: Text.NonEmpty }

-- The creator becomes the first member, then the org is opened.
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
          <al-button variant="secondary" size="sm" click={Open { org: o.id }}>Open</al-button>
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

test "the list shows only the user's orgs, by name" = {
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
  let zeta <- Db.insert Org { name: Text.NonEmpty "Zeta" }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let other <- Db.insert Org { name: Text.NonEmpty "Other" }
  let _ <- Test.member ann zeta
  let _ <- Test.member ann acme
  let _ <- Test.member bob other
  let _ <- Test.login { user: Just ann, org: Nothing }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#orgs .name" page.doc == ["Acme", "Zeta"])
}

test "opening an org goes to its projects" = {
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

-- A crafted Open of an org the user is no member of: the server refuses it, and the session
-- keeps the org it had.
test "a user cannot open an org they are not a member of" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let orgs <- Test.get (Index {})
  let r <- Test.run (Open.update { org: globex } { user: ann, org: Just acme })
  let page <- Test.get (Projects.Index.Index {})
  Task.succeed (
    {
      listed: Test.texts "#orgs .name" orgs.doc,
      value: r.value,
      org: Test.text "#org" page.doc,
    } == {
      listed: ["Acme"],
      value: Err "Session.put org: no Membership for this user and org",
      org: Just "Acme",
    }
  )
}

test "creating an org makes the creator a member and opens it" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just ann, org: Nothing }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Create {}) { name: Text.NonEmpty "Acme" }
  let members <-
    Db.from Membership
      |> Db.where (Membership.user.eq ann)
      |> Db.count
  Task.succeed (
    { answer: r.answer, org: Test.text "#org" r.doc, members: members }
      == { answer: Redirected (Projects.Index.Index {}), org: Just "Acme", members: 1 }
  )
}

--- file: app/Projects/Index.alder ---
module Projects.Index

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.User

route Index  = GET  "/projects" { }                  requires user, org
route Create = POST "/projects" { } form NewProject requires user, org

type Model = { org: Org, projects: List Project }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- Membership.memberOnly session.user session.org
  let found <- Db.get Org session.org
  let projects <-
    Db.scoped Project
      |> Db.orderBy Project.name.asc
      |> Db.all
  match found {
    Just org -> Query.succeed { org: org, projects: projects },
    Nothing -> Query.notFound,
  }
}

form NewProject = { name: Text.NonEmpty }

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let id <- Db.insertScoped Project { name: p.form.name }
  Task.succeed (Response.redirect (Projects.Show.Show { id: id }))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title={model.org.name}>
    <h1 id="org">{model.org.name}</h1>
    <ul id="projects">
      {for p in model.projects ->
        <li><a class="name" href={Projects.Show.Show { id: p.id }}>{p.name}</a></li>}
    </ul>
    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New project" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
    <a href={Orgs.Index.Index {}}>Orgs</a>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a member sees only their org's projects" = {
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
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member ann acme
  let _ <- Test.member bob globex
  let _ <- Test.login { user: Just bob, org: Just globex }
  let _ <- Db.insertScoped Project { name: Text.NonEmpty "Secret" }
  let _ <- Test.login { user: Just ann, org: Just acme }
  let _ <- Db.insertScoped Project { name: Text.NonEmpty "Beta" }
  let _ <- Db.insertScoped Project { name: Text.NonEmpty "Alpha" }
  let page <- Test.get (Index {})
  Task.succeed (
    { org: Test.text "#org" page.doc, projects: Test.texts "#projects .name" page.doc }
      == { org: Just "Acme", projects: ["Alpha", "Beta"] }
  )
}

test "a user without an org is sent to choose one" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just ann, org: Nothing }
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
  let r <- Test.submit (Create {}) { name: Text.NonEmpty "Alpha" }
  let page <- Test.get (Index {})
  Task.succeed (
    { name: Test.text "#name" r.doc, projects: Test.texts "#projects .name" page.doc }
      == { name: Just "Alpha", projects: ["Alpha"] }
  )
}

-- The session keeps the org after the membership is deleted; the pages read it again.
test "a member removed from the org loses access at once" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let membership <- Db.insert Membership { user: ann, org: acme }
  let _ <- Test.login { user: Just ann, org: Just acme }
  let pid <- Db.insertScoped Project { name: Text.NonEmpty "Alpha" }
  let before <- Test.get (Index {})
  let _ <- Db.delete Membership membership
  let list <- Test.open (Index {})
  let show <- Test.open (Projects.Show.Show { id: pid })
  Task.succeed (
    {
      before: Test.texts "#projects .name" before.doc,
      list: list.answer,
      show: show.answer,
    } == {
      before: ["Alpha"],
      list: PageMissing,
      show: PageMissing,
    }
  )
}

--- file: app/Projects/Show.alder ---
module Projects.Show

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.Todo
import Table.User

route Show   = GET   "/projects/{id}"              { id: Project.ProjectId }                              requires user, org
route Add    = POST  "/projects/{id}/todos"        { id: Project.ProjectId }                 form NewTodo requires user, org
route Toggle = PATCH "/projects/{id}/todos/{todo}" { id: Project.ProjectId, todo: Todo.TodoId }              requires user, org

type Model = { project: Project, todos: List Todo }

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

form NewTodo = { title: Text.NonEmpty }

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let project <- Db.getScoped Project p.id
  let _ <- Db.insertScoped Todo { title: p.form.title, project: project.id }
  Task.succeed (Response.redirect (Show { id: p.id }))
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let todo <- Db.getScoped Todo p.todo
  if todo.project != p.id then Task.notFound
  else {
    let _ <- Db.updateScoped Todo p.todo (\t -> { t | done: not t.done })
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.project.name}>
    <h1 id="name">{model.project.name}</h1>
    <ul id="todos">
      {for t in model.todos ->
        <li class={if t.done then "done" else ""}>
          <span class="title">{t.title}</span>
          <al-button variant="secondary" size="sm" click={Toggle { id: model.project.id, todo: t.id }}>
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
    <a href={Projects.Index.Index {}}>All projects</a>
  </Layout.Page>

test "a project of another org is not found" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member ann acme
  let _ <- Test.member ann globex
  let _ <- Test.login { user: Just ann, org: Just acme }
  let pid <- Db.insertScoped Project { name: Text.NonEmpty "Alpha" }
  let mine <- Test.get (Show { id: pid })
  let _ <- Test.login { user: Just ann, org: Just globex }
  let other <- Test.open (Show { id: pid })
  Task.succeed (
    { mine: Test.text "#name" mine.doc, other: other.answer }
      == { mine: Just "Alpha", other: PageMissing }
  )
}

test "adding and toggling a to-do" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Test.member ann acme
  let _ <- Test.login { user: Just ann, org: Just acme }
  let pid <- Db.insertScoped Project { name: Text.NonEmpty "Alpha" }
  let tid <- Db.insertScoped Todo { title: Text.NonEmpty "Write", project: pid }
  let _ <- Test.get (Show { id: pid })
  let added <- Test.submit (Add { id: pid }) { title: Text.NonEmpty "Read" }
  let toggled <- Test.click (Toggle { id: pid, todo: tid })
  let after <- Db.getScoped Todo tid
  Task.succeed (
    {
      added: added.answer,
      titles: Test.texts "#todos .title" added.doc,
      toggled: toggled.answer,
      done: Test.texts "li.done .title" toggled.doc,
      stored: after.done,
    } == {
      added: Redirected (Show { id: pid }),
      titles: ["Read", "Write"],
      toggled: Redirected (Show { id: pid }),
      done: ["Write"],
      stored: True,
    }
  )
}

test "a removed member cannot add a to-do" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let membership <- Db.insert Membership { user: ann, org: acme }
  let _ <- Test.login { user: Just ann, org: Just acme }
  let pid <- Db.insertScoped Project { name: Text.NonEmpty "Alpha" }
  let _ <- Test.get (Show { id: pid })
  let _ <- Db.delete Membership membership
  let r <- Test.submit (Add { id: pid }) { title: Text.NonEmpty "Read" }
  let todos <- Db.scoped Todo |> Db.count
  Task.succeed ({ answer: r.answer, todos: todos } == { answer: PageMissing, todos: 0 })
}
