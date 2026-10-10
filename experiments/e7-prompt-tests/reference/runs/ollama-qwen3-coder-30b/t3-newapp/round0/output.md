--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId, theme: Theme = Light }

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
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
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
        Task.succeed (Redirect (Blog.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" theme={session.theme}>
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: app/Blog/Index.alder ---
module Blog.Index

import Post
import Layout
import User

page "/" { }

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Blog" theme={session.theme}>
    <h1>Blog</h1>
    <nav>
      {if session.user == Nothing then Html.none else <a href={Blog.Admin {}}>Admin</a>}
    </nav>
    <ul>
      {for post in model.posts ->
        <li><a href={Blog.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
  </Layout.Page>

--- file: app/Blog/Show.alder ---
module Blog.Show

import Post
import Layout
import User

page "/posts/{slug}" { slug: Post.Slug } requires user

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Post params.slug
  match found {
    Just post -> if post.owner == session.user || post.published then {
      let comments <- Db.from Comment |> Db.where (Comment.post.eq params.slug) |> Db.orderBy Comment.created.desc |> Db.all
      Query.succeed { post: post, comments: comments }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  author: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg Submit form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Comment { author: p.form.author, body: p.form.body, post: model.post.slug }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} theme={session.theme}>
    <h1>{model.post.title}</h1>
    <p>Published: {model.post.published}</p>
    <p>{model.post.body}</p>
    <h2>Comments</h2>
    <ul>
      {for comment in model.comments ->
        <li><strong>{comment.author}</strong>: {comment.body}</li>}
    </ul>
    <form submit={Submit}>
      <Input field="author" label="Your name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>
  </Layout.Page>

--- file: app/Blog/Admin.alder ---
module Blog.Admin

import Post
import Layout
import User

page "/admin" { } requires user

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

form NewPost = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text.NonEmpty,
  published: Bool = False,
}

msg Create form NewPost
msg TogglePublished { slug: Post.Slug }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published, owner: session.user }
    Task.succeed Rerender
  },
  TogglePublished p -> {
    let post <- Db.get Post p.slug
    match post {
      Just pt -> {
        let _ <- Db.update Post p.slug (\t -> { t | published: not t.published })
        Task.succeed Rerender
      },
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin" theme={session.theme}>
    <h1>Admin</h1>
    <form submit={Create}>
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <Input field="published" label="Published" type="checkbox" />
      <Errors field="published" />
      <button>Create post</button>
    </form>
    <h2>Posts</h2>
    <ul>
      {for post in model.posts ->
        <li>
          <span>{post.title} ({if post.published then "Published" else "Draft"})</span>
          <button click={TogglePublished { slug: post.slug }}>Toggle Published</button>
        </li>}
    </ul>
    <a href={Blog.Index {}}>Back to blog</a>
  </Layout.Page>

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Post.alder ---
module Post

import User

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text.NonEmpty,
  published: Bool = False,
  owner: User.UserId @ref User,
  created: Time @created,
}

type PostId = brand Text

table Comment {
  id: CommentId @id,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  post: Post.Slug @ref Post,
  created: Time @created,
}

type CommentId = brand Text

test "unpublished post is not found for reader" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { title: Text.NonEmpty "Milk", slug: Slug "milk", body: Text.NonEmpty "Buy milk", published: False, owner: me }
  let r <- Test.run (Blog.Show.load { slug: Slug "milk" } { user: User.UserId "someone-else", theme: Light })
  Task.succeed (r.value == Err "not found")
}

test "comment with empty author is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { title: Text.NonEmpty "Milk", slug: Slug "milk", body: Text.NonEmpty "Buy milk", published: True, owner: me }
  let r <- Test.run (Blog.Show.update (Submit { author: "", body: Text.NonEmpty "Good" }) { post: { id: PostId "a", title: Text.NonEmpty "Milk", slug: Slug "milk", body: Text.NonEmpty "Buy milk", published: True, owner: me, created: Time.epoch }, comments: [] } { user: me, theme: Light })
  Task.succeed (r.value == Err "not found")
}

test "index lists only published posts newest first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id1 <- Db.insert Post { title: Text.NonEmpty "First", slug: Slug "first", body: Text.NonEmpty "First post", published: True, owner: me }
  let id2 <- Db.insert Post { title: Text.NonEmpty "Second", slug: Slug "second", body: Text.NonEmpty "Second post", published: True, owner: me }
  let r <- Test.run (Blog.Index.load {} { user: Nothing, theme: Light })
  Task.succeed (r.value == Ok { posts: [{ id: PostId "a", title: Text.NonEmpty "Second", slug: Slug "second", body: Text.NonEmpty "Second post", published: True, owner: me, created: Time.epoch }, { id: PostId "b", title: Text.NonEmpty "First", slug: Slug "first", body: Text.NonEmpty "First post", published: True, owner: me, created: Time.epoch }] })
}

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