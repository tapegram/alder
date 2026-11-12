--- file: app/App.alder ---
module App

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

--- file: app/Table/Contact.alder ---
module Table.Contact

import Table.User

type ContactId = brand Text

table Contact {
  id: ContactId @id,
  name: Text.NonEmpty,
  email: Text.Email,
  owner: User.UserId @ref User @owner,
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

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Contacts.Index.Find {}))
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
    { answer: r.answer, user: r.session.user } == { answer: Redirected (Contacts.Index.Find {}), user: Just me }
  )
}

--- file: app/Contacts/Index.alder ---
module Contacts.Index

import Layout
import Table.Contact
import Table.User

route Find    = GET  "/contacts"         { q: Text = "" } requires user
route Results = GET  "/contacts/results" { q: Text = "" } requires user
route Add     = POST "/contacts"         { } form NewContact requires user

form NewContact = { name: Text.NonEmpty, email: Text.Email }

type Model = { q: Text, contacts: List Contact }

-- Contacts of the signed-in user whose name contains `q`; an empty `q` matches all.
matching : Text -> Query (List Contact)
matching q = {
  let found <- Db.mine Contact |> Db.orderBy Contact.name.asc |> Db.all
  Query.succeed (List.filter (\c -> Text.contains (Text.toLower q) (Text.toLower c.name)) found)
}

Find.load : Find.Params -> Session -> Query Model
Find.load params session = {
  let found <- matching params.q
  Query.succeed { q: params.q, contacts: found }
}

Results.load : Results.Params -> Session -> Query (List Contact)
Results.load params session = matching params.q

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insertMine Contact { name: p.form.name, email: p.form.email }
  Task.succeed (Response.redirect (Contacts.Index.Find {}))
}

Results.view : List Contact -> Session -> Html
Results.view contacts session =
  <ul id="results">{for c in contacts -> <li>{c.name}</li>}</ul>

Find.view : Model -> Session -> Html
Find.view model session =
  <Layout.Page title="Contacts">
    <h1>Contacts</h1>

    <al-card>
      <form submit={Add}>
        <al-input field="name" label="Name" />
        <al-input field="email" label="Email" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>

    <input name="q" value={model.q} input={Results { q: model.q }} />
    <ul id="results">{for c in model.contacts -> <li>{c.name}</li>}</ul>
  </Layout.Page>

test "typing a query lists only the matching contacts" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insertMine Contact { name: Text.NonEmpty "Alice", email: Text.Email "a@example.com" }
  let _ <- Db.insertMine Contact { name: Text.NonEmpty "Bob", email: Text.Email "b@example.com" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Find { q: "" })
  let r <- Test.input (Results { q: "ali" })
  Task.succeed (Test.texts "#results li" r.doc == ["Alice"])
}

test "an empty query lists every contact" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insertMine Contact { name: Text.NonEmpty "Alice", email: Text.Email "a@example.com" }
  let _ <- Db.insertMine Contact { name: Text.NonEmpty "Bob", email: Text.Email "b@example.com" }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Find { q: "" })
  Task.succeed (Test.texts "#results li" page.doc == ["Alice", "Bob"])
}

test "another user's contacts never appear" = {
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
  let _ <- Db.insert Contact { name: Text.NonEmpty "Carol", email: Text.Email "c@example.com", owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Find { q: "" })
  let r <- Test.input (Results { q: "carol" })
  Task.succeed (Test.count "#results li" page.doc == 0 && Test.count "#results li" r.doc == 0)
}

test "the form adds a contact" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Find { q: "" })
  let r <- Test.submit (Add {}) { name: Text.NonEmpty "Dana", email: Text.Email "d@example.com" }
  let page <- Test.get (Find { q: "" })
  Task.succeed (
    { answer: r.answer, names: Test.texts "#results li" page.doc }
      == { answer: Redirected (Contacts.Index.Find {}), names: ["Dana"] }
  )
}
