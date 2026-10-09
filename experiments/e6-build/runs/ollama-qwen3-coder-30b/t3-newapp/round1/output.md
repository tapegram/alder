--- file: app/App.alder ---
module App

import Layout
import Theme
import Post
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
        Task.succeed (Redirect (Posts.Index {}))
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

--- file: app/Post.alder ---
module Post

import User

type PostId = brand Text
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text,
  published: Bool = False,
  created: Time @created,
  author: User.UserId @ref User,
}

table Comment {
  id: Text @id,
  postId: PostId @ref Post,
  author: Text.NonEmpty,
  body: Text,
  created: Time @created,
}

test "post can be created" =
  Post { id: PostId "a", title: Text.NonEmpty "x", slug: Slug "x", body: "body", published: False, created: Time.epoch, author: User.UserId "u" } != ()

--- file: app/Posts/Admin.alder ---
module Posts.Admin

import Post
import Layout

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
  body: Text,
  published: Bool = False,
}

msg Submit form NewPost
msg TogglePublished { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published, author: session.user }
    Task.succeed Rerender
  },
  TogglePublished p -> {
    let post <- Db.get Post p.id
    match post {
      Just pt -> {
        let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
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

    <h2>Create a post</h2>
    <form submit={Submit}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <Input field="published" label="Published" type="checkbox" />
      <Errors field="published" />
      <button>Create</button>
    </form>

    <h2>Posts</h2>
    {for post in model.posts ->
      <article>
        <h3>{post.title}</h3>
        <p>{Time.format "Jan 2, 2006" post.created}</p>
        <p>{if post.published then "Published" else "Draft"}</p>
        <button click={TogglePublished { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</button>
      </article>}
  </Layout.Page>

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import Layout

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
    {for post in model.posts ->
      <article>
        <h2><a href={Posts.Show { slug: post.slug }}>{post.title}</a></h2>
        <p>{Time.format "Jan 2, 2006" post.created}</p>
      </article>}
  </Layout.Page>

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = { post: Post, comments: List Post.Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Post params.slug
  match found {
    Just post -> {
      let comments <- Db.from Post.Comment |> Db.where (Post.Comment.postId.eq post.id) |> Db.orderBy Post.Comment.created.asc |> Db.all
      Query.succeed { post: post, comments: comments }
    },
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  author: Text.NonEmpty,
  body: Text,
}

msg Submit form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Post.Comment { postId: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} theme={session.theme}>
    <h1>{model.post.title}</h1>
    <p>{Time.format "Jan 2, 2006" model.post.created}</p>
    <div>{model.post.body}</div>

    <h2>Comments</h2>
    {for comment in model.comments ->
      <article>
        <p><strong>{comment.author}</strong></p>
        <p>{comment.body}</p>
        <p>{Time.format "Jan 2, 2006 at 3:04 PM" comment.created}</p>
      </article>}

    <h2>Add a comment</h2>
    <form submit={Submit}>
      <Errors form />
      <Input field="author" label="Name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post</button>
    </form>
  </Layout.Page>

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