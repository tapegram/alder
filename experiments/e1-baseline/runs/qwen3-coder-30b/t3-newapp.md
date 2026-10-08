<!-- model=qwen3-coder:30b task=t3-newapp.md seconds=37 prompt_tokens=11239 output_tokens=2131 -->
--- file: app/App.alder ---
module App

import Layout
import Auth.Login
import Post

session { user: Maybe Post.AuthorId }

login = Auth.Login

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

--- file: app/Post.alder ---
module Post

import User

type PostId = brand Text
type AuthorId = brand Text
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text.NonEmpty,
  published: Bool = False,
  author: AuthorId @ref User,
  created: Time @created,
}

table Comment {
  id: Text @id,
  post: PostId @ref Post,
  authorName: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}

test "post with no comments has empty list" =
  List.length (Post.comments { id: PostId "a", title: "x", slug: Slug "x", body: "x", published: True, author: AuthorId "u", created: Time.epoch }) == 0

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import Layout
import Posts.Show
import Auth.Login

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

page "/posts/{slug}" { slug: Post.Slug } requires user

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Post params.slug
  match found {
    Just post ->
      if post.published || post.author == session.user then {
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
      <Input field="authorName" label="Your name" />
      <Errors field="authorName" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post</button>
    </form>

    <a href={Posts.Index {}}>Back to all posts</a>
  </Layout.Page>

--- file: app/Posts/Admin.alder ---
module Posts.Admin

import Post
import Layout
import Posts.Index

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
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <label>
        <Input field="published" type="checkbox" />
        Published
      </label>
      <button>Create</button>
    </form>

    <h2>Posts</h2>
    <table>
      <thead>
        <tr>
          <th>Title</th>
          <th>Slug</th>
          <th>Published</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {for post in model.posts ->
          <tr>
            <td>{post.title}</td>
            <td>{post.slug}</td>
            <td>{Bool.toText post.published}</td>
            <td><button click={TogglePublished { id: post.id }}>Toggle</button></td>
          </tr>}
      </tbody>
    </table>

    <a href={Posts.Index {}}>Back to blog</a>
  </Layout.Page>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout
import Posts.Index

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

msg Submit = "/login" form Credentials
msg Logout = "/logout" { }

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
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
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