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

test "logging out from the contacts page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Contacts.Index.Find {})
  let out <- Test.click Logout
  Task.succeed (
    { out: out.answer, user: out.session.user }
      == { out: Redirected (Auth.Login.Login {}), user: Nothing }
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

--- file: app/Table/Contact.alder ---
module Table.Contact

import Table.User

type ContactId = brand Text

-- `owner` is the table's `@owner`: `Db.mine Contact` and `Db.insertMine` keep to the signed-in
-- user's contacts.
table Contact {
  id: ContactId @id,
  name: Text.NonEmpty,
  email: Text.Email,
  owner: User.UserId @ref User @owner,
}

--- file: app/Contacts/Index.alder ---
module Contacts.Index

import Layout
import Table.Contact
import Table.User

route Find    = GET  "/contacts"         { q: Text = "" }                 requires user
route Results = GET  "/contacts/results" { q: Text = "" }                 requires user
route Add     = POST "/contacts"         { }              form NewContact requires user

type Model = { q: Text, contacts: List Contact }

form NewContact = { name: Text.NonEmpty, email: Text.Email }

-- The signed-in user's contacts whose name contains `q`, ignoring case; "" matches them all.
matching : Text -> Select Contact
matching q =
  Db.mine Contact
    |> Db.where (Contact.name.contains q)
    |> Db.orderBy Contact.name.asc

resultList : List Contact -> Html
resultList contacts = <ul id="results">{for c in contacts -> <li>{c.name}</li>}</ul>

Find.load : Find.Params -> Session -> Query Model
Find.load params session = {
  let contacts <- Db.all (matching params.q)
  Query.succeed { q: params.q, contacts: contacts }
}

Find.view : Model -> Session -> Html
Find.view model session =
  <Layout.Page title="Contacts">
    <h1>Contacts</h1>
    <input
      type="search"
      name="q"
      placeholder="Search by name"
      value={model.q}
      input={Results { q: model.q }}
    />
    {resultList model.contacts}
    <al-card>
      <form submit={Add}>
        <Errors form />
        <al-input field="name" label="Name" />
        <al-input field="email" label="Email" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

Results.load : Results.Params -> Session -> Query (List Contact)
Results.load params session = Db.all (matching params.q)

Results.view : List Contact -> Session -> Html
Results.view contacts session = resultList contacts

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insertMine Contact { name: p.form.name, email: p.form.email }
  Task.succeed (Response.redirect (Find {}))
}

test "typing a query lists only the matching contacts" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Alice Smith",
    email: Text.Email "alice@example.com",
    owner: me,
  }
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Bob Jones",
    email: Text.Email "bob@example.com",
    owner: me,
  }
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Malia Brown",
    email: Text.Email "malia@example.com",
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Find {})
  let r <- Test.input (Results { q: "ALI" })
  Task.succeed (Test.texts "#results li" r.doc == ["Alice Smith", "Malia Brown"])
}

test "the page opened with a query lists only its matches" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Alice Smith",
    email: Text.Email "alice@example.com",
    owner: me,
  }
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Bob Jones",
    email: Text.Email "bob@example.com",
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Find { q: "bob" })
  Task.succeed (Test.texts "#results li" page.doc == ["Bob Jones"])
}

test "an empty query lists every contact" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Bob Jones",
    email: Text.Email "bob@example.com",
    owner: me,
  }
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Alice Smith",
    email: Text.Email "alice@example.com",
    owner: me,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Find {})
  let typed <- Test.input (Results { q: "bo" })
  let cleared <- Test.input (Results { q: "" })
  Task.succeed (
    {
      page: Test.texts "#results li" page.doc,
      typed: Test.texts "#results li" typed.doc,
      cleared: Test.texts "#results li" cleared.doc,
    } == {
      page: ["Alice Smith", "Bob Jones"],
      typed: ["Bob Jones"],
      cleared: ["Alice Smith", "Bob Jones"],
    }
  )
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
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Alice Smith",
    email: Text.Email "alice@example.com",
    owner: me,
  }
  let _ <- Db.insert Contact {
    name: Text.NonEmpty "Eve Adams",
    email: Text.Email "eve@example.com",
    owner: other,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Find {})
  let typed <- Test.input (Results { q: "e" })
  let searched <- Test.get (Find { q: "Eve" })
  Task.succeed (
    {
      page: Test.texts "#results li" page.doc,
      typed: Test.texts "#results li" typed.doc,
      searched: Test.texts "#results li" searched.doc,
    } == {
      page: ["Alice Smith"],
      typed: ["Alice Smith"],
      searched: [],
    }
  )
}

test "the form adds a contact" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Find {})
  let r <- Test.submit (Add {}) {
    name: Text.NonEmpty "Carol White",
    email: Text.Email "carol@example.com",
  }
  let rows <- Db.from Contact |> Db.all
  Task.succeed (
    {
      answer: r.answer,
      listed: Test.texts "#results li" r.doc,
      rows: List.map (\c -> { name: c.name, email: c.email, owner: c.owner }) rows,
    } == {
      answer: Redirected (Find {}),
      listed: ["Carol White"],
      rows: [
        {
          name: Text.NonEmpty "Carol White",
          email: Text.Email "carol@example.com",
          owner: me,
        },
      ],
    }
  )
}

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Find {})
  Task.succeed (visitor.answer == Redirected (Auth.Login.Login {}))
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

form Credentials = { email: Text.Email, password: Text.Secret }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

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
      answer: Redirected (Contacts.Index.Find {}),
      user: Just me,
      title: Just "Contacts",
    }
  )
}
