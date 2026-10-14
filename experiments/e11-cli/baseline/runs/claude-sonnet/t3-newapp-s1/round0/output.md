--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <header>
        <a href={Url.external "/"}>My blog</a>
      </header>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}

--- file: app/Post.alder ---
module Post

type PostId = brand Text

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  created: Time @created,
}

--- file: app/Comment.alder ---
module Comment

import Post

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

import User
import Layout

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Admin.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Input field="password" label="Password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Admin.Index {}), user: Just me })
}

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import Layout

page "/" { }

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Blog">
    <h1>Posts</h1>
    <ul id="posts">
      {for p in model.posts ->
        <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
    </ul>
  </Layout.Page>

test "the front page lists only published posts, newest first" = {
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "a", published: True }
  let _ <- Db.insert Post { slug: Post.Slug "hidden", title: Text.NonEmpty "Hidden", body: "b" }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "c", published: True }
  let page <- Test.get (Posts.Index {})
  Task.succeed (Test.texts "#posts a" page.doc == ["Second", "First"])
}

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Comment
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> if post.published || Maybe.isJust session.user then loadFor post else Query.notFound,
  }
}

loadFor : Post -> Query Model
loadFor post = {
  let comments <-
    Db.from Comment
      |> Db.where (Comment.post.eq post.id)
      |> Db.orderBy Comment.created.asc
      |> Db.all
  Query.succeed { post: post, comments: comments }
}

form NewComment = {
  author: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg AddComment form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title}>
    <article>
      <h1>{model.post.title}</h1>
      <p>{model.post.body}</p>
    </article>
    <h2>Comments</h2>
    <ul id="comments">
      {for c in model.comments ->
        <li><strong>{c.author}</strong>: {c.body}</li>}
    </ul>
    <form submit={AddComment}>
      <Errors form />
      <Input field="author" label="Your name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Comment</button>
    </form>
  </Layout.Page>

test "an unpublished post is not found for a reader" = {
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Secret" }
  let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}

test "an unpublished post is shown to the logged-in author" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Secret" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == Shown)
}

test "a comment with an empty author is refused" = {
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi", published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], n: 0 })
}

test "a valid comment is added" = {
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi", published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice" }
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Rerendered, n: 1 })
}

--- file: app/Admin/Index.alder ---
module Admin.Index

import Post
import Layout

page "/admin" { } requires user

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <-
    Db.from Post
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

form NewPost = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text,
  published: Bool,
}

msg Create form NewPost
msg Toggle { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let taken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.count
    if taken > 0 then Task.succeed (Reject "That slug is already used.") else {
      let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, published: p.form.published }
      Task.succeed Rerender
    }
  },
  Toggle p -> {
    let found <- Db.get Post p.id
    match found {
      Nothing -> Task.notFound,
      Just post -> {
        let _ <- Db.update Post post.id (\x -> { x | published: not x.published })
        Task.succeed Rerender
      },
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin">
    <h1>Admin</h1>
    <form submit={Create}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <Input field="published" label="Published" />
      <Errors field="published" />
      <button>Create</button>
    </form>
    <ul id="posts">
      {for p in model.posts ->
        <li>
          <a href={Posts.Show { slug: p.slug }}>{p.title}</a>
          <button click={Toggle { id: p.id }}>{if p.published then "Unpublish" else "Publish"}</button>
        </li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a visitor is sent to log in" = {
  let r <- Test.open (Admin.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "the admin lists unpublished posts too" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "a", title: Text.NonEmpty "A", body: "x", published: True }
  let _ <- Db.insert Post { slug: Post.Slug "b", title: Text.NonEmpty "B", body: "y" }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Admin.Index {})
  Task.succeed (Test.count "#posts li" page.doc == 2)
}

test "creating a post through the form" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Index {})
  let r <- Test.submit NewPost { title: Text.NonEmpty "New", slug: Post.Slug "new", body: "b", published: True }
  let n <- Db.from Post |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Rerendered, n: 1 })
}

test "Toggle publishes a post" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { slug: Post.Slug "a", title: Text.NonEmpty "A", body: "x" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Index {})
  let r <- Test.click (Toggle { id: id })
  let after <- Db.get Post id
  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Rerendered, published: Just True })
}
