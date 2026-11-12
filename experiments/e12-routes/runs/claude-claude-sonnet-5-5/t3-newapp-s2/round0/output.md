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
  Task.succeed (Response.redirect (Posts.Index.Index {}))
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
      <al-page width="narrow">
        <main>
          <al-stack gap="lg">
            <nav><a href={Posts.Index.Index {}}>My blog</a></nav>
            {props.children}
          </al-stack>
        </main>
      </al-page>
      <al-toasts></al-toasts>
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

--- file: app/Table/Post.alder ---
module Table.Post

type PostId = brand Text

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text,
  published: Bool = False,
  created: Time @created,
}

--- file: app/Table/Comment.alder ---
module Table.Comment

import Table.Post

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
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
      Task.succeed (Response.redirect (Admin.Index.Index {}))
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
      == { answer: Redirected (Admin.Index.Index {}), user: Just me }
  )
}

--- file: app/Posts/Index.alder ---
module Posts.Index

import Layout
import Table.Post

route Index = GET "/" { }

type Model = { posts: List Post }

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
    <h1>Posts</h1>
    <ul id="posts">
      {for post in model.posts ->
        <li>
          <a href={Posts.Show.Show { slug: post.slug }}>{post.title}</a>
        </li>}
    </ul>
  </Layout.Page>

test "the front page lists only published posts, newest first" = {
  let _ <- Db.insert Post {
    title: Text.NonEmpty "First",
    slug: Post.Slug "first",
    body: "one",
    published: True,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Hidden",
    slug: Post.Slug "hidden",
    body: "secret",
    published: False,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Second",
    slug: Post.Slug "second",
    body: "two",
    published: True,
  }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#posts a" page.doc == ["Second", "First"])
}

--- file: app/Posts/Show.alder ---
module Posts.Show

import Layout
import Table.Comment
import Table.Post
import Table.User

route Show       = GET  "/posts/{slug}"          { slug: Post.Slug }
route AddComment = POST "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

type Model = { post: Post, comments: List Comment }

form NewComment = { author: Text.NonEmpty, body: Text.NonEmpty }

-- A draft is visible only to a logged-in user (the author); for everyone else it is a 404.
Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <-
    Db.from Post
      |> Db.where (Post.slug.eq params.slug)
      |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post ->
      if post.published || Maybe.isJust session.user then {
        let comments <-
          Db.from Comment
            |> Db.where (Comment.post.eq post.id)
            |> Db.orderBy Comment.created.asc
            |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound,
  }
}

AddComment.update : AddComment.Params -> Session -> Task Response
AddComment.update p session = {
  let found <-
    Db.from Post
      |> Db.where (Post.slug.eq p.slug)
      |> Db.first
  match found {
    Nothing -> Task.notFound,
    Just post ->
      if post.published || Maybe.isJust session.user then {
        let _ <- Db.insert Comment { post: post.id, author: p.form.author, body: p.form.body }
        Task.succeed (Response.redirect (Show { slug: p.slug }))
      } else Task.notFound,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    <article>
      <p>{model.post.body}</p>
    </article>
    <h2>Comments</h2>
    <ul id="comments">
      {for c in model.comments ->
        <li class="comment">
          <strong>{c.author}</strong>
          <p>{c.body}</p>
        </li>}
    </ul>
    <al-card>
      <form submit={AddComment { slug: model.post.slug }}>
        <Errors form />
        <al-input field="author" label="Your name" />
        <al-textarea field="body" label="Comment" />
        <al-button type="submit">Add comment</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unpublished post is not found for a reader" = {
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "wip",
    published: False,
  }
  let r <- Test.open (Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}

test "an unpublished post is shown to the logged-in author" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "wip",
    published: False,
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.get (Show { slug: Post.Slug "draft" })
  Task.succeed (Test.text "h1" r.doc == Just "Draft")
}

test "a comment with an empty author is refused" = {
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: "world",
    published: True,
  }
  let _ <- Test.get (Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw (AddComment { slug: Post.Slug "hello" }) [
    { name: "author", value: "" },
    { name: "body", value: "Nice post" },
  ]
  let n <- Db.from Comment |> Db.count
  let rejected = match r.answer {
    FormRejected errs -> True,
    _ -> False,
  }
  Task.succeed ({ rejected: rejected, comments: n } == { rejected: True, comments: 0 })
}

test "a valid comment is added" = {
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: "world",
    published: True,
  }
  let _ <- Test.get (Show { slug: Post.Slug "hello" })
  let r <- Test.submit (AddComment { slug: Post.Slug "hello" }) {
    author: Text.NonEmpty "Ann",
    body: Text.NonEmpty "Nice post",
  }
  let page <- Test.get (Show { slug: Post.Slug "hello" })
  Task.succeed (
    { answer: r.answer, authors: Test.texts "#comments strong" page.doc }
      == { answer: Redirected (Show { slug: Post.Slug "hello" }), authors: ["Ann"] }
  )
}

--- file: app/Admin/Index.alder ---
module Admin.Index

import Layout
import Table.Post
import Table.User

route Index      = GET   "/admin"             { }                      requires user
route CreatePost = POST  "/admin/posts"       { } form NewPost         requires user
route Toggle     = PATCH "/admin/posts/{id}"  { id: Post.PostId }      requires user

type Model = { posts: List Post }

form NewPost = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text.NonEmpty,
  published: Bool,
}

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let posts <-
    Db.from Post
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

CreatePost.update : CreatePost.Params -> Session -> Task Response
CreatePost.update p session = {
  let same <-
    Db.from Post
      |> Db.where (Post.slug.eq p.form.slug)
      |> Db.count
  if same > 0 then
    Task.succeed (Response.invalid "A post with that slug already exists.")
  else {
    let _ <- Db.insert Post {
      title: p.form.title,
      slug: p.form.slug,
      body: p.form.body,
      published: p.form.published,
    }
    Task.succeed (Response.redirect (Index {}))
  }
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
  Task.succeed Response.refresh
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Admin">
    <h1>Admin</h1>
    <al-card>
      <form submit={CreatePost}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-input field="slug" label="Slug" />
        <al-textarea field="body" label="Body" />
        <al-checkbox field="published" label="Published" />
        <al-button type="submit">Create post</al-button>
      </form>
    </al-card>
    <ul id="posts">
      {for post in model.posts ->
        <li>
          <a href={Posts.Show.Show { slug: post.slug }}>{post.title}</a>
          <al-badge variant="outline">{if post.published then "Published" else "Draft"}</al-badge>
          <al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>
            {if post.published then "Unpublish" else "Publish"}
          </al-button>
        </li>}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a visitor is sent to log in" = {
  let r <- Test.open (Index {})
  Task.succeed (r.answer == Redirected (Auth.Login.Login {}))
}

test "creating a post through the form" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let r <- Test.submit (CreatePost {}) {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: Text.NonEmpty "World",
    published: False,
  }
  let page <- Test.get (Index {})
  Task.succeed (
    { answer: r.answer, titles: Test.texts "#posts a" page.doc }
      == { answer: Redirected (Index {}), titles: ["Hello"] }
  )
}

test "the button toggles published" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Post {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: "world",
    published: False,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let r <- Test.click (Toggle { id: id })
  let after <- Db.get Post id
  Task.succeed (
    { answer: r.answer, published: Maybe.map (\x -> x.published) after }
      == { answer: Refreshed, published: Just True }
  )
}
