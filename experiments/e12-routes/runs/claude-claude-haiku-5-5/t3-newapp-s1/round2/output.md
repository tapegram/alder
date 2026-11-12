--- file: app/App.alder ---
module App

import Layout
import Table.User

session { user: Maybe User.UserId, role: User.Role = Reader }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing, role: Reader })
  Task.succeed (Response.redirect (Home.Index {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

test "Logout clears the user and goes home" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me"), role: Author })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Home.Index {})), trace: ["Session.put"] }
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

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let user <- Db.get User id
      let role = Maybe.withDefault Reader (Maybe.map (\u -> u.role) user)
      let _ <- Session.put (\s -> { s | user: Just id, role: role })
      Task.succeed (Response.redirect (Home.Index {}))
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
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Home.Index {}), user: Just me }
  )
}

--- file: app/Home.alder ---
module Home

import Layout
import Table.Post
import Table.User

route Index = GET "/" { }

type Model = { posts: List Post }

-- Published posts only, newest first.
Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let posts <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Blog">
    <h1>Blog</h1>
    <ul>
      {for post in model.posts -> <li><a href={Reading.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
    <p><a href={Auth.Login.Login {}}>Log in</a></p>
  </Layout.Page>

test "the front page lists only published posts, newest first" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
    role: Author,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Old",
    slug: Post.Slug "old",
    body: "a",
    published: True,
    author: me,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "b",
    published: False,
    author: me,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "New",
    slug: Post.Slug "new",
    body: "c",
    published: True,
    author: me,
  }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "li a" page.doc == ["New", "Old"])
}

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

--- file: app/Reading.alder ---
module Reading

import Layout
import Table.Comment
import Table.Post
import Table.User

route Show  = GET  "/posts/{slug}"          { slug: Post.Slug }
route Reply = POST "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

form NewComment = { author: Text.NonEmpty, body: Text.NonEmpty }

type Model = { post: Post, comments: List Comment }

-- A draft is a 404 for everyone but its author.
loadVisible : Maybe User.UserId -> Post -> Query Model
loadVisible me post =
  if Post.visibleTo me post then {
    let comments <-
      Db.from Comment
        |> Db.where (Comment.post.eq post.id)
        |> Db.orderBy Comment.created.asc
        |> Db.all
    Query.succeed { post: post, comments: comments }
  } else Query.notFound

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> loadVisible session.user post,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    {if model.post.published then Html.none else <p>Not published yet.</p>}
    <article>{model.post.body}</article>
    <h2>Comments</h2>
    <ul id="comments">
      {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
    </ul>
    <al-card>
      <form submit={Reply { slug: model.post.slug }}>
        <Errors form />
        <al-input field="author" label="Your name" />
        <al-textarea field="body" label="Comment" />
        <al-button type="submit">Add comment</al-button>
      </form>
    </al-card>
  </Layout.Page>

Reply.update : Reply.Params -> Session -> Task Response
Reply.update p session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq p.slug) |> Db.first
  match found {
    Nothing -> Task.notFound,
    Just post -> if Post.visibleTo session.user post then {
      let _ <- Db.insert Comment { post: post.id, author: p.form.author, body: p.form.body }
      Task.succeed (Response.redirect (Show { slug: p.slug }))
    } else Task.notFound,
  }
}

test "an unpublished post is not found for a reader" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "Soon.",
    author: me,
  }
  let r <- Test.open (Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}

test "the author sees their own draft" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "Soon.",
    author: me,
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == Shown)
}

test "a comment with an empty author is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: "Hi.",
    published: True,
    author: me,
  }
  let _ <- Test.get (Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw (Reply { slug: Post.Slug "hello" }) [
    { name: "author", value: "" },
    { name: "body", value: "Nice post." },
  ]
  let n <- Db.from Comment |> Db.count
  let refused = match r.answer {
    FormRejected _ -> True,
    _ -> False,
  }
  Task.succeed ({ refused: refused, comments: n, status: r.status } == { refused: True, comments: 0, status: 422 })
}

--- file: app/Table/Comment.alder ---
module Table.Comment

import Table.Post

type CommentId = brand Text

-- A comment goes with its post. Both the author's name and the body are required.
table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}

--- file: app/Table/Post.alder ---
module Table.Post

import Table.User

type PostId = brand Text

-- Lowercase letters, digits and hyphens: checked whenever a slug comes from a route or a form.
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

-- The title is plain Text in the table: the brand constructor `Text.NonEmpty` is usable only in
-- tests, so the helper `ownedBy` below could not build a post with it. Forms still require a
-- non-empty title (`Writing.NewPost`).
table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User,
  created: Time @created,
}

-- A draft is shown only to its author; a published post to everyone.
visibleTo : Maybe User.UserId -> Post -> Bool
visibleTo me post = post.published || me == Just post.author

-- A post by `author`, published or not, for the tests of `visibleTo`.
ownedBy : Bool -> User.UserId -> Post
ownedBy published author = {
  id: PostId "p",
  slug: Slug "draft",
  title: "Draft",
  body: "",
  published: published,
  author: author,
  created: Time.epoch,
}

test "a draft is visible to its author only" =
  not (visibleTo Nothing (ownedBy False (User.UserId "u")))
    && not (visibleTo (Just (User.UserId "other")) (ownedBy False (User.UserId "u")))
    && visibleTo (Just (User.UserId "u")) (ownedBy False (User.UserId "u"))
    && visibleTo Nothing (ownedBy True (User.UserId "u"))

--- file: app/Table/User.alder ---
module Table.User

type UserId = brand Text

-- A Reader reads and comments; an Author writes posts at /admin. The session holds the role
-- copied from this row at login (Auth.Login).
type Role = Reader | Author

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
  role: Role = Reader,
}

--- file: app/Writing.alder ---
module Writing

import Layout
import Table.Post
import Table.User

route Desk    = GET   "/admin"            { } requires user, role Author
route Create  = POST  "/admin/posts"      { } form NewPost requires user, role Author
route Publish = PATCH "/admin/posts/{id}" { id: Post.PostId } requires user, role Author

form NewPost = { title: Text.NonEmpty, slug: Post.Slug, body: Text }

type Model = { posts: List Post }

-- Every post, published or not, newest first.
Desk.load : Desk.Params -> Session -> Query Model
Desk.load params session = {
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let taken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.count
  if taken > 0 then
    Task.succeed (Response.invalid "A post with that slug already exists.")
  else {
    let _ <- Db.insert Post {
      title: p.form.title,
      slug: p.form.slug,
      body: p.form.body,
      author: session.user,
    }
    Task.succeed (Response.redirect (Desk {}))
  }
}

Publish.update : Publish.Params -> Session -> Task Response
Publish.update p session = {
  let _ <- Db.update Post p.id (\post -> { post | published: not post.published })
  Task.succeed Response.refresh
}

Row : { post: Post } -> Html
Row props =
  <li>
    <al-button variant="secondary" size="sm" click={Publish { id: props.post.id }}>
      {if props.post.published then "Unpublish" else "Publish"}
    </al-button>
    <a href={Reading.Show { slug: props.post.slug }}>{props.post.title}</a>
    {if props.post.published then Html.none else <al-badge variant="outline">draft</al-badge>}
  </li>

Desk.view : Model -> Session -> Html
Desk.view model session =
  <Layout.Page title="Admin">
    <h1>Posts</h1>
    <al-card>
      <form submit={Create}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-input field="slug" label="Slug" />
        <al-input field="body" label="Body" />
        <al-button type="submit">New post</al-button>
      </form>
    </al-card>
    <ul id="posts">
      {for post in model.posts -> <Row post={post} />}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a visitor is sent to log in for the desk" = {
  let visitor <- Test.open (Desk {})
  Task.succeed (visitor.answer == Redirected (Auth.Login.Login {}))
}

test "a reader cannot open the desk" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Desk {})
  Task.succeed (r.answer == PageMissing)
}

test "publishing a draft shows it on the front page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
    role: Author,
  }
  let id <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "Soon.",
    author: me,
  }
  let _ <- Test.login { user: Just me, role: Author }
  let _ <- Test.get (Desk {})
  let r <- Test.click (Publish { id: id })
  let page <- Test.get (Home.Index {})
  Task.succeed (
    { answer: r.answer, front: Test.texts "li a" page.doc }
      == { answer: Refreshed, front: ["Draft"] }
  )
}
