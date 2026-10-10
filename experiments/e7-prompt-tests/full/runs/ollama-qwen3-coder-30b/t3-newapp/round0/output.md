--- file: app/App.alder ---
module App

import Layout
import User

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
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

import Blog
import Layout
import User

page "/" { }

type Model = { posts: List Blog.Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Blog.Post |> Db.where (Blog.Post.published.eq True) |> Db.orderBy Blog.Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Blog" theme={session.theme}>
    <h1>Blog</h1>
    {for post in model.posts ->
      <div>
        <a href={Blog.Show { slug: post.slug }}>{post.title}</a>
        <span>{Blog.Post.created.post}</span>
      </div>}
  </Layout.Page>

--- file: app/Blog/Show.alder ---
module Blog.Show

import Blog
import Layout
import User

page "/posts/{slug}" { slug: Blog.Slug } requires user

type Model = { post: Blog.Post, comments: List Blog.Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Blog.Post params.slug
  match found {
    Just post -> if post.owner == session.user || post.published then {
      let comments <- Db.from Blog.Comment |> Db.where (Blog.Comment.post.eq params.slug) |> Db.orderBy Blog.Comment.created.desc |> Db.all
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
    let _ <- Db.insert Blog.Comment { author: p.form.author, body: p.form.body, post: model.post.slug }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} theme={session.theme}>
    <h1>{model.post.title}</h1>
    <p>{model.post.body}</p>
    <form submit={Submit}>
      <Input field="author" label="Author" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post</button>
    </form>
    {for comment in model.comments ->
      <div>
        <strong>{comment.author}</strong>
        <p>{comment.body}</p>
      </div>}
  </Layout.Page>

--- file: app/Blog/Admin.alder ---
module Blog.Admin

import Blog
import Layout
import User

page "/admin" { } requires user

type Model = { posts: List Blog.Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Blog.Post |> Db.orderBy Blog.Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

form NewPost = {
  title: Text.NonEmpty,
  slug: Blog.Slug,
  body: Text.NonEmpty,
  published: Bool = False,
}

msg Create form NewPost
msg TogglePublished { id: Blog.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Blog.Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published, owner: session.user }
    Task.succeed Rerender
  },
  TogglePublished p -> {
    let post <- Db.get Blog.Post p.id
    match post {
      Just pt -> {
        let _ <- Db.update Blog.Post p.id (\t -> { t | published: not t.published })
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
      <label>
        <Input field="published" label="Published" />
        Published
      </label>
      <button>Create</button>
    </form>

    {for post in model.posts ->
      <div>
        <h2>{post.title}</h2>
        <p>{post.body}</p>
        <p>Published: {Bool.toText post.published}</p>
        <button click={TogglePublished { id: post.id }}>Toggle Published</button>
      </div>}
  </Layout.Page>

--- file: app/Blog.alder ---
module Blog

import User

type PostId = brand Text
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

table Comment {
  id: Text @id,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  post: Slug @ref Post,
  created: Time @created,
}

test "unpublished post is not found for reader" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { title: Text.NonEmpty "My Post", slug: Slug "my-post", body: Text.NonEmpty "Body", published: False, owner: me }
  let r <- Test.run (Blog.Show.load { slug: Slug "my-post" } { user: User.UserId "someone-else", theme: Light })
  Task.succeed (r.value == Err "not found")
}

test "comment with empty author is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { title: Text.NonEmpty "My Post", slug: Slug "my-post", body: Text.NonEmpty "Body", published: True, owner: me }
  let r <- Test.run (Blog.Show.update (Submit { author: "", body: Text.NonEmpty "Good comment" }) { post: { id: PostId "a", title: Text.NonEmpty "My Post", slug: Slug "my-post", body: Text.NonEmpty "Body", published: True, owner: me, created: Time.epoch }, comments: [] } { user: me, theme: Light })
  Task.succeed (r.value == Err "not found")
}

test "index lists only published posts, newest first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id1 <- Db.insert Post { title: Text.NonEmpty "Old Post", slug: Slug "old-post", body: Text.NonEmpty "Body", published: True, owner: me }
  let id2 <- Db.insert Post { title: Text.NonEmpty "New Post", slug: Slug "new-post", body: Text.NonEmpty "Body", published: True, owner: me }
  let r <- Test.run (Blog.Index.load {} { user: Nothing, theme: Light })
  Task.succeed (r.value == Ok { posts: [{ id: PostId "b", title: Text.NonEmpty "New Post", slug: Slug "new-post", body: Text.NonEmpty "Body", published: True, owner: me, created: Time.epoch }, { id: PostId "a", title: Text.NonEmpty "Old Post", slug: Slug "old-post", body: Text.NonEmpty "Body", published: True, owner: me, created: Time.epoch }] })
}

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
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light

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