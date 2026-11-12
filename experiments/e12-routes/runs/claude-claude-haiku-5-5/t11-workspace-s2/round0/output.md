--- file: app/App.alder ---
module App

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.Todo
import Table.User

-- The user, and the org they work in now. `via Membership`: the server refuses an org the user is
-- not a member of (book 06, "Partitions"). `choose`: a user without an org is sent to pick one.
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

-- One row per member of an org. The session's `via Membership` checks it when an org is opened.
table Membership {
  id: MembershipId @id,
  user: User.UserId @ref User,
  org: Org.OrgId @ref Org,
}

-- Whether the user is still a member of the org. The routes that depend on it read it again on
-- every request, so a member removed from the org loses access at once (book 06, "Partitions").
isMember : User.UserId -> Org.OrgId -> Query Bool
isMember user org = {
  let n <-
    Db.from Membership
      |> Db.where (Membership.user.eq user)
      |> Db.where (Membership.org.eq org)
      |> Db.count
  Query.succeed (n > 0)
}

-- A request by someone who is no longer a member is a 404, as a row that is not theirs.
memberOnly : User.UserId -> Org.OrgId -> Query Unit
memberOnly user org = {
  let member <- isMember user org
  if member then Query.succeed () else Query.notFound
}

--- file: app/Table/Project.alder ---
module Table.Project

import Table.Org

type ProjectId = brand Text

-- A project belongs to one org; its rows live in that org's silo (App's `silo org`).
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

-- A to-do belongs to a project, and goes with it.
table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  project: Project.ProjectId @ref Project @cascade,
  org: Org.OrgId @ref Org @partition org,
}

--- file: app/Orgs/Index.alder ---
module Orgs.Index

import Layout
import Table.Membership
import Table.Org
import Table.User

route Index  = GET  "/orgs"        { }                  requires user
route Create = POST "/orgs"        { } form NewOrg      requires user
route Open   = PUT  "/session/org" { org: Org.OrgId }   requires user

type Model = { orgs: List Org }

form NewOrg = { name: Text.NonEmpty }

-- The orgs the user is a member of, by name.
Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let memberships <-
    Db.from Membership
      |> Db.where (Membership.user.eq session.user)
      |> Db.all
  let found <- Query.all (List.map (\m -> Db.get Org m.org) memberships)
  let orgs =
    found
      |> List.filterMap (\o -> o)
      |> List.sortByText (\o -> o.name)
  Query.succeed { orgs: orgs }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Your orgs">
    <h1>Your orgs</h1>
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
        <al-button type="submit">Create org</al-button>
      </form>
    </al-card>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

-- The creator becomes the org's first member, and the org is opened.
Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let org <- Db.insert Org { name: p.form.name }
  let _ <- Db.insert Membership { user: session.user, org: org }
  let _ <- Session.put (\s -> { s | org: Just org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}

-- A crafted request for an org the user is not a member of is refused by `via Membership`.
Open.update : Open.Params -> Session -> Task Response
Open.update p session = {
  let _ <- Session.put (\s -> { s | org: Just p.org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}

test "Open makes the org the session's org" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Db.insert Membership { user: me, org: acme }
  let _ <- Test.login { user: Just me, org: Nothing }
  let _ <- Test.get (Index {})
  let r <- Test.click (Open { org: acme })
  Task.succeed (
    { answer: r.answer, org: r.session.org }
      == { answer: Redirected (Projects.Index.Index {}), org: Just acme }
  )
}

test "a user cannot open an org they are not a member of" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Db.insert Membership { user: me, org: acme }
  let _ <- Test.login { user: Just me, org: Nothing }
  let _ <- Test.get (Index {})
  let r <- Test.run (Open.update { org: globex } { user: me, org: Nothing })
  let page <- Test.get (Index {})
  Task.succeed (
    { refused: Result.toMaybe r.value == Nothing, org: page.session.org }
      == { refused: True, org: Nothing }
  )
}

test "creating an org makes its creator a member and opens it" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me, org: Nothing }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Create {}) { name: Text.NonEmpty "Acme" }
  let members <- Db.from Membership |> Db.where (Membership.user.eq me) |> Db.count
  Task.succeed (
    { answer: r.answer, title: Test.text "#org" r.doc, members: members }
      == { answer: Redirected (Projects.Index.Index {}), title: Just "Acme", members: 1 }
  )
}

--- file: app/Projects/Index.alder ---
module Projects.Index

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.User

route Index  = GET  "/projects" { }                requires user, org
route Create = POST "/projects" { } form NewProject requires user, org

type Model = { org: Maybe Org, projects: List Project }

form NewProject = { name: Text.NonEmpty }

-- The org's name and its projects. A user who is no longer a member gets a 404 (book 06).
Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- Membership.memberOnly session.user session.org
  let org <- Db.get Org session.org
  let projects <-
    Db.scoped Project
      |> Db.orderBy Project.name.asc
      |> Db.all
  Query.succeed { org: org, projects: projects }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Projects">
    {match model.org {
      Just o -> <h1 id="org">{o.name}</h1>,
      Nothing -> Html.none,
    }}
    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New project" />
        <al-button type="submit">Add project</al-button>
      </form>
    </al-card>
    <ul id="projects">
      {for p in model.projects ->
        <li><a class="name" href={Projects.Show.Show { id: p.id }}>{p.name}</a></li>}
    </ul>
    <a href={Orgs.Index.Index {}}>Switch org</a>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let id <- Db.insertScoped Project { name: p.form.name }
  Task.succeed (Response.redirect (Projects.Show.Show { id: id }))
}

test "a user with no org is sent to choose one" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me, org: Nothing }
  let r <- Test.open (Index {})
  Task.succeed (r.answer == Redirected (Orgs.Index.Index {}))
}

test "a member sees only their org's projects" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Db.insert Membership { user: me, org: acme }
  let _ <- Db.insert Membership { user: me, org: globex }
  let _ <- Test.login { user: Just me, org: Just globex }
  let _ <- Db.insertScoped Project { name: Text.NonEmpty "Website" }
  let _ <- Test.login { user: Just me, org: Just acme }
  let _ <- Db.insertScoped Project { name: Text.NonEmpty "Launch" }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#projects .name" page.doc == ["Launch"])
}

--- file: app/Projects/Show.alder ---
module Projects.Show

import Layout
import Table.Membership
import Table.Org
import Table.Project
import Table.Todo
import Table.User

route Show   = GET   "/projects/{id}"                               { id: Project.ProjectId }                        requires user, org
route Add    = POST  "/projects/{id}/todos"                        { id: Project.ProjectId } form NewTodo          requires user, org
route Toggle = PATCH "/projects/{id}/todos/{todo}"                 { id: Project.ProjectId, todo: Todo.TodoId }     requires user, org

type Model = { project: Project, todos: List Todo }

form NewTodo = { title: Text.NonEmpty }

-- One project of the org, with its to-dos by title. Another org's project is a 404: the silo
-- has no such row, and the membership is read again on every request.
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

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.project.name}>
    <h1 id="name">{model.project.name}</h1>
    <al-card>
      <form submit={Add { id: model.project.id }}>
        <al-input field="title" label="New to-do" />
        <al-button type="submit">Add to-do</al-button>
      </form>
    </al-card>
    <ul id="todos">
      {for t in model.todos ->
        <li class={if t.done then "done" else ""}>
          <span class="title">{t.title}</span>
          <al-button variant="secondary" size="sm" click={Toggle { id: model.project.id, todo: t.id }}>
            {if t.done then "Open again" else "Done"}
          </al-button>
        </li>}
    </ul>
    <a href={Projects.Index.Index {}}>All projects</a>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let _ <- Db.getScoped Project p.id
  let _ <- Db.insertScoped Todo { title: p.form.title, project: p.id }
  Task.succeed (Response.redirect (Projects.Show.Show { id: p.id }))
}

-- The to-do must be of the project in the URL, and of the org.
Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let todo <- Db.getScoped Todo p.todo
  if todo.project == p.id then {
    let _ <- Db.updateScoped Todo p.todo (\t -> { t | done: not t.done })
    Task.succeed (Response.redirect (Projects.Show.Show { id: p.id }))
  } else Task.notFound
}

test "a project of another org is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Db.insert Membership { user: me, org: acme }
  let _ <- Db.insert Membership { user: me, org: globex }
  let _ <- Test.login { user: Just me, org: Just globex }
  let theirs <- Db.insertScoped Project { name: Text.NonEmpty "Website" }
  let _ <- Test.login { user: Just me, org: Just acme }
  let r <- Test.open (Show { id: theirs })
  Task.succeed (r.answer == PageMissing)
}

test "a member removed from the org loses access at once" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let link <- Db.insert Membership { user: me, org: acme }
  let _ <- Test.login { user: Just me, org: Just acme }
  let project <- Db.insertScoped Project { name: Text.NonEmpty "Launch" }
  let before <- Test.open (Show { id: project })
  let _ <- Db.delete Membership link
  let after <- Test.open (Show { id: project })
  Task.succeed (
    { before: before.answer, after: after.answer }
      == { before: Shown, after: PageMissing }
  )
}

test "toggling a to-do" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Db.insert Membership { user: me, org: acme }
  let _ <- Test.login { user: Just me, org: Just acme }
  let project <- Db.insertScoped Project { name: Text.NonEmpty "Launch" }
  let todo <- Db.insertScoped Todo { title: Text.NonEmpty "Write", project: project }
  let _ <- Test.get (Show { id: project })
  let r <- Test.click (Toggle { id: project, todo: todo })
  let after <- Db.getScoped Todo todo
  Task.succeed (
    { answer: r.answer, done: after.done }
      == { answer: Redirected (Show { id: project }), done: True }
  )
}

test "adding a to-do lists it" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Db.insert Membership { user: me, org: acme }
  let _ <- Test.login { user: Just me, org: Just acme }
  let project <- Db.insertScoped Project { name: Text.NonEmpty "Launch" }
  let _ <- Test.get (Show { id: project })
  let r <- Test.submit (Add { id: project }) { title: Text.NonEmpty "Write" }
  Task.succeed (
    { answer: r.answer, titles: Test.texts "#todos .title" r.doc }
      == { answer: Redirected (Show { id: project }), titles: ["Write"] }
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

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

-- A successful login goes to the orgs; the org is chosen there.
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
  let r <- Test.submit (Submit {}) {
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
  let r <- Test.submit (Submit {}) { email: Text.Email "me@example.com", password: Text.Secret "secret" }
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
