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
      <al-page width="narrow">
        <header><nav><a href={Posts.Index.Index {}}>Home</a></nav></header>
        <main><al-stack gap="lg">{props.children}</al-stack></main>
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

import Table.User

type PostId = brand Text

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User,
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
      Task.succeed (Response.redirect (Admin.Dashboard {}))
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
      answer: Redirected (Admin.Dashboard {}),
      user: Just me,
      title: Just "Admin",
    }
  )
}

--- file: app/Posts/Index.alder ---
module Posts.Index

import Layout
import Table.Post
import Table.User

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
    <h1>Blog</h1>
    <ul id="posts">
      {for post in model.posts ->
        <li>
          <a href={Posts.Show.Show { slug: post.slug }}>{post.title}</a>
          <span class="date">{Time.format "Jan 2, 2006" post.created}</span>
        </li>}
    </ul>
    {if List.isEmpty model.posts then <p>No posts yet.</p> else Html.none}
  </Layout.Page>

test "the front page lists only published posts, newest first" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Old",
    slug: Post.Slug "old",
    body: "The first post.",
    published: True,
    author: me,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "Not yet.",
    published: False,
    author: me,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "New",
    slug: Post.Slug "new",
    body: "The latest post.",
    published: True,
    author: me,
  }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#posts a" page.doc == ["New", "Old"])
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

-- A published post is anyone's to read; a draft only its author's.
visible : Maybe User.UserId -> Post -> Bool
visible user post = post.published || user == Just post.author

-- The post with that slug, when the user may see it; otherwise a 404.
findVisible : Maybe User.UserId -> Post.Slug -> Query Post
findVisible user slug = {
  let found <-
    Db.from Post
      |> Db.where (Post.slug.eq slug)
      |> Db.first
  match found {
    Just post -> if visible user post then Query.succeed post else Query.notFound,
    Nothing -> Query.notFound,
  }
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let post <- findVisible session.user params.slug
  let comments <-
    Db.from Comment
      |> Db.where (Comment.post.eq post.id)
      |> Db.orderBy Comment.created.asc
      |> Db.all
  Query.succeed { post: post, comments: comments }
}

AddComment.update : AddComment.Params -> Session -> Task Response
AddComment.update p session = {
  let post <- findVisible session.user p.slug
  let _ <- Db.insert Comment { post: post.id, author: p.form.author, body: p.form.body }
  Task.succeed (Response.redirect (Show { slug: p.slug }) |> Response.success "Comment added")
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.post.title}>
    <article>
      <h1>{model.post.title}</h1>
      {if model.post.published then Html.none else <al-badge variant="outline">Draft</al-badge>}
      <p class="date">{Time.format "Jan 2, 2006" model.post.created}</p>
      <p>{model.post.body}</p>
    </article>
    <section id="comments">
      <h2>Comments</h2>
      <ul>
        {for c in model.comments ->
          <li><strong class="author">{c.author}</strong> <span class="body">{c.body}</span></li>}
      </ul>
    </section>
    <al-card>
      <form submit={AddComment { slug: model.post.slug }}>
        <Errors form />
        <al-input field="author" label="Name" />
        <al-textarea field="body" label="Comment" />
        <al-button type="submit">Add comment</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unpublished post is not found for a reader" = {
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
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "Not yet.",
    published: False,
    author: me,
  }
  let visitor <- Test.open (Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just other }
  let stranger <- Test.open (Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just me }
  let author <- Test.get (Show { slug: Post.Slug "draft" })
  Task.succeed (
    {
      visitor: visitor.answer,
      stranger: stranger.answer,
      author: Test.text "h1" author.doc,
    } == {
      visitor: PageMissing,
      stranger: PageMissing,
      author: Just "Draft",
    }
  )
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
    body: "Hi there.",
    published: True,
    author: me,
  }
  let _ <- Test.get (Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw (AddComment { slug: Post.Slug "hello" }) [
    { name: "author", value: "" },
    { name: "body", value: "Nice post" },
  ]
  let n <- Db.from Comment |> Db.count
  let rejected = match r.answer {
    FormRejected _ -> True,
    _ -> False,
  }
  Task.succeed (
    { rejected: rejected, status: r.status, comments: n }
      == { rejected: True, status: 422, comments: 0 }
  )
}

test "a comment is added and shown" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: "Hi there.",
    published: True,
    author: me,
  }
  let _ <- Test.get (Show { slug: Post.Slug "hello" })
  let r <- Test.submit (AddComment { slug: Post.Slug "hello" }) {
    author: Text.NonEmpty "Ann",
    body: Text.NonEmpty "Nice post",
  }
  Task.succeed (
    { answer: r.answer, authors: Test.texts "#comments .author" r.doc }
      == { answer: Redirected (Show { slug: Post.Slug "hello" }), authors: ["Ann"] }
  )
}

--- file: app/Admin.alder ---
module Admin

import Layout
import Table.Post
import Table.User

route Dashboard = GET   "/admin"            { }                              requires user
route Create    = POST  "/admin/posts"      { }                form NewPost requires user
route Toggle    = PATCH "/admin/posts/{id}" { id: Post.PostId }              requires user

type Model = { posts: List Post }

form NewPost = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text.NonEmpty,
  published: Bool = False,
}

Dashboard.load : Dashboard.Params -> Session -> Query Model
Dashboard.load params session = {
  let posts <-
    Db.from Post
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let taken <-
    Db.from Post
      |> Db.where (Post.slug.eq p.form.slug)
      |> Db.count
  if taken > 0 then
    Task.succeed (Response.invalid "A post with that slug already exists.")
  else {
    let _ <- Db.insert Post {
      title: p.form.title,
      slug: p.form.slug,
      body: p.form.body,
      published: p.form.published,
      author: session.user,
    }
    Task.succeed (Response.redirect (Dashboard {}) |> Response.success "Post created")
  }
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- Db.update Post p.id (\x -> { x | published: not x.published })
  Task.succeed Response.refresh
}

Dashboard.view : Model -> Session -> Html
Dashboard.view model session =
  <Layout.Page title="Admin">
    <h1>Admin</h1>
    <al-card>
      <form submit={Create}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-input field="slug" label="Slug" help="lowercase letters, digits and hyphens" />
        <al-textarea field="body" label="Body" />
        <al-checkbox field="published" label="Published" />
        <al-button type="submit">Create post</al-button>
      </form>
    </al-card>
    <ul id="admin-posts">
      {for post in model.posts ->
        <li id={"post-" ++ post.id}>
          <a href={Posts.Show.Show { slug: post.slug }}>{post.title}</a>
          {if post.published then <al-badge>Published</al-badge> else <al-badge variant="outline">Draft</al-badge>}
          <al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>
            {if post.published then "Unpublish" else "Publish"}
          </al-button>
        </li>}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a visitor is sent to log in from the admin" = {
  let visitor <- Test.open (Dashboard {})
  Task.succeed (visitor.answer == Redirected (Auth.Login.Login {}))
}

test "creating a post through the form" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Dashboard {})
  let r <- Test.submit (Create {}) {
    title: Text.NonEmpty "First",
    slug: Post.Slug "first",
    body: Text.NonEmpty "Hello world.",
    published: True,
  }
  let posts <- Db.from Post |> Db.all
  Task.succeed (
    {
      answer: r.answer,
      posts: List.map (\x -> { title: x.title, published: x.published }) posts,
    } == {
      answer: Redirected (Dashboard {}),
      posts: [{ title: Text.NonEmpty "First", published: True }],
    }
  )
}

test "toggling publishes a draft" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: "Not yet.",
    published: False,
    author: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Dashboard {})
  let r <- Test.click (Toggle { id: id })
  let after <- Db.get Post id
  Task.succeed (
    { answer: r.answer, published: Maybe.map (\x -> x.published) after }
      == { answer: Refreshed, published: Just True }
  )
}
