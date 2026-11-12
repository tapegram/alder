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

nameOf : Maybe Org -> Text
nameOf org = match org {
  Just o -> o.name,
  Nothing -> "",
}

--- file: app/Table/Membership.alder ---
module Table.Membership

import Table.Org
import Table.User

type MembershipId = brand Text

-- One row per member of an org. `via Membership` in App's session reads these rows.
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

-- Stops the request with 404 when the user is not a member of the org. The pages call it on
-- every request: the session keeps the org, so membership is read again each time.
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

--- file: app/Orgs/Index.alder ---
module Orgs.Index

import Layout
import Table.Membership
import Table.Org
import Table.User

route Index  = GET  "/orgs"            { }                requires user
route Open   = POST "/orgs/{org}/open" { org: Org.OrgId } requires user
route Create = POST "/orgs"            { } form NewOrg    requires user

type Model = { orgs: List Org }

form NewOrg = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let memberships <-
    Db.from Membership
      |> Db.where (Membership.user.eq session.user)
      |> Db.all
  let found <-
    memberships
      |> List.map (\m -> Db.get Org m.org)
      |> Query.all
  let orgs =
    found
      |> List.filterMap (\o -> o)
      |> List.sortByText (\o -> o.name)
  Query.succeed { orgs: orgs }
}

-- Opening an org the user is no member of is refused before the session changes.
Open.update : Open.Params -> Session -> Task Response
Open.update p session = {
  let _ <- Membership.memberOnly session.user p.org
  let _ <- Session.put (\s -> { s | org: Just p.org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}

-- The creator becomes the org's first member, and the org is opened.
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
          <al-button size="sm" click={Open { org: o.id }}>Open</al-button>
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

test "a user cannot open an org they are not a member of" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let mine <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let other <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member me mine
  let refused <- Test.run (Open.update { org: other } { user: me, org: Nothing })
  let allowed <- Test.run (Open.update { org: mine } { user: me, org: Nothing })
  Task.succeed (
    { refused: refused.value, allowed: allowed.value }
      == {
        refused: Err "not found",
        allowed: Ok (Response.redirect (Projects.Index.Index {})),
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

route Index  = GET  "/projects" { }                    requires user, org
route Create = POST "/projects" { } form NewProject    requires user, org

type Model = { orgName: Text, projects: List Project }

form NewProject = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- Membership.memberOnly session.user session.org
  let org <- Db.get Org session.org
  let projects <-
    Db.scoped Project
      |> Db.orderBy Project.name.asc
      |> Db.all
  Query.succeed { orgName: Org.nameOf org, projects: projects }
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
    <h1 id="org">{model.orgName}</h1>

    <al-card>
      <form submit={Create}>
        <al-input field="name" label="New project" />
        <al-button type="submit">Add project</al-button>
      </form>
    </al-card>

    <ul id="projects">
      {for p in model.projects ->
        <li>
          <a class="name" href={Projects.Show.Show { id: p.id }}>{p.name}</a>
        </li>}
    </ul>
  </Layout.Page>

test "a member sees only their org's projects" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member me acme
  let _ <- Test.member me globex
  let _ <- Test.login { user: Just me, org: Just globex }
  let _ <- Db.insertScoped Project { name: Text.NonEmpty "Secret" }
  let _ <- Test.login { user: Just me, org: Just acme }
  let _ <- Db.insertScoped Project { name: Text.NonEmpty "Roadmap" }
  let page <- Test.get (Index {})
  Task.succeed (
    { names: Test.texts ".name" page.doc, org: Test.text "#org" page.doc }
      == { names: ["Roadmap"], org: Just "Acme" }
  )
}

test "a member removed from the org loses access at once" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let _ <- Test.member me acme
  let _ <- Test.login { user: Just me, org: Just acme }
  let shown <- Test.open (Index {})
  let row <- Db.from Membership |> Db.where (Membership.user.eq me) |> Db.first
  let _ <- match row {
    Just m -> Db.delete Membership m.id,
    Nothing -> Task.succeed (),
  }
  let after <- Test.open (Index {})
  Task.succeed (
    { before: shown.answer, after: after.answer }
      == { before: Shown, after: PageMissing }
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

route Show   = GET   "/projects/{id}"              { id: Project.ProjectId }                    requires user, org
route Add    = POST  "/projects/{id}/todos"        { id: Project.ProjectId } form NewTodo       requires user, org
route Toggle = PATCH "/projects/{id}/todos/{todo}" { id: Project.ProjectId, todo: Todo.TodoId } requires user, org

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

-- The project must be one of this org's: another org's project is a 404.
Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let _ <- Db.getScoped Project p.id
  let _ <- Db.insertScoped Todo { title: p.form.title, project: p.id }
  Task.succeed (Response.redirect (Projects.Show.Show { id: p.id }))
}

-- A to-do is toggled only through its own project's URL.
Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let todo <- Db.getScoped Todo p.todo
  if todo.project == p.id then {
    let _ <- Db.updateScoped Todo p.todo (\t -> { t | done: not t.done })
    Task.succeed (Response.redirect (Projects.Show.Show { id: p.id }))
  } else {
    Task.notFound
  }
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
            {if t.done then "Undo" else "Done"}
          </al-button>
        </li>}
    </ul>

    <al-button variant="ghost" href={Projects.Index.Index {}}>Back</al-button>
  </Layout.Page>

test "a project of another org is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let acme <- Db.insert Org { name: Text.NonEmpty "Acme" }
  let globex <- Db.insert Org { name: Text.NonEmpty "Globex" }
  let _ <- Test.member me acme
  let _ <- Test.member me globex
  let _ <- Test.login { user: Just me, org: Just globex }
  let theirs <- Db.insertScoped Project { name: Text.NonEmpty "Secret" }
  let _ <- Test.login { user: Just me, org: Just acme }
  let r <- Test.open (Show { id: theirs })
  Task.succeed (r.answer == PageMissing)
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

-- A new login starts without an org: the user chooses one at /orgs.
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

test "logging in goes to the org list" = {
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
