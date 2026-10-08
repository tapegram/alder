<!-- model=qwen3-coder:30b task=t3-newapp.md seconds=39 prompt_tokens=11924 output_tokens=2126 -->
--- file: app/App.alder ---
module App

import Layout
import Auth.Login
import Posts.Index
import Posts.Show
import Posts.Admin

session { user: Maybe UserId }

login = Auth.Login

msg Logout = "/logout" { }

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Posts.Index {}))
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
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Posts.Index
import App

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe UserId),
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit = "/login" form Credentials

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

view : Model -> Html Msg
view model =
  <Layout.Page title="Log in">
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
  author: User.UserId @ref User,
  created: Time @created,
}

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: PostId @ref Post,
  authorName: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}

test "slug validation allows lowercase letters digits and hyphens" =
  Slug "hello-world-123" == Slug "hello-world-123"

test "slug validation rejects uppercase letters" =
  Slug "Hello" == Nothing

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import Layout
import Posts.Show
import App

page "/" { }

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Blog">
    <h1>Blog</h1>
    {for post in model.posts ->
      <article>
        <h2><a href={Posts.Show { slug: post.slug }}>{post.title}</a></h2>
        <p>{Date.format "Jan 2, 2006" (Time.toDate post.created)}</p>
      </article>}
  </Layout.Page>

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Layout
import Posts.Index
import App

page "/posts/{slug}" { slug: Post.Slug } requires user

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Post params.slug
  match found {
    Just post -> if post.published || post.author == session.user then {
      let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
      Query.succeed { post: post, comments: comments }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  authorName: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg Submit = "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Comment { post: model.post.id, authorName: p.form.authorName, body: p.form.body }
    Task.succeed Rerender
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    <p>Published: {Date.format "Jan 2, 2006" (Time.toDate model.post.created)}</p>
    <div>{model.post.body}</div>

    <h2>Comments</h2>
    {for comment in model.comments ->
      <article>
        <p><strong>{comment.authorName}</strong></p>
        <p>{comment.body}</p>
        <p>{Date.format "Jan 2, 2006 at 3:04 PM" comment.created}</p>
      </article>}

    <h2>Add a comment</h2>
    <form submit={Submit { slug: model.post.slug }}>
      <Errors form />
      <Input field="authorName" label="Your name" />
      <Errors field="authorName" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>

    <a href={Posts.Index {}}>Back to blog</a>
  </Layout.Page>

--- file: app/Posts/Admin.alder ---
module Posts.Admin

import Post
import Layout
import Posts.Index
import App

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

msg Create = "/admin/posts" form NewPost
msg TogglePublished = "/admin/posts/{id}/toggle" { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published, author: session.user }
    Task.succeed Rerender
  },
  TogglePublished p -> {
    let post <- Db.get Post p.id
    match post {
      Just p -> {
        let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
        Task.succeed Rerender
      },
      Nothing -> Task.fail "Post not found",
    }
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Admin">
    <h1>Admin</h1>

    <h2>Create a post</h2>
    <form submit={Create}>
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
        <p>Slug: {post.slug}</p>
        <p>Published: {Bool.toText post.published}</p>
        <button click={TogglePublished { id: post.id }}>
          {if post.published then "Unpublish" else "Publish"}
        </button>
      </article>}

    <a href={Posts.Index {}}>Back to blog</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

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