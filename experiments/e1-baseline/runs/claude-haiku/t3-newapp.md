--- file: app/App.alder ---
module App

import User
import Layout
import Auth.Login

session { user: Maybe User.UserId }

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

import User

type PostId = brand Text
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User,
  created: Time @created,
}

--- file: app/Comment.alder ---
module Comment

import Post

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout
import Admin.Index

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
        Task.succeed (Redirect (Admin.Index {}))
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

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import Layout
import Posts.Show

page "/" { }

type Model = {
  posts: List Post,
}

load : Params -> Session -> Query Model
load params session = {
  let posts <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Blog">
    <h1>Blog</h1>
    <ul>
      {for post in model.posts ->
        <li>
          <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
          <p>{Time.format "Jan 2, 2006" post.created}</p>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Comment
import Layout
import Posts.Index

page "/posts/{slug}" { slug: Post.Slug }

type Model = {
  post: Post,
  comments: List Comment,
}

load : Params -> Session -> Query Model
load params session = {
  let post <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match post {
    Just p -> {
      if p.published || (match session.user {
        Just user -> user == p.author,
        Nothing -> False,
      }) then {
        let comments <- Db.from Comment |> Db.where (Comment.post.eq p.id) |> Db.orderBy Comment.created.asc |> Db.all
        Query.succeed { post: p, comments: comments }
      } else Query.notFound
    },
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  author: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg Submit = "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    <p>{Time.format "Jan 2, 2006" model.post.created}</p>
    <div>{Html.raw model.post.body}</div>

    <h2>Comments</h2>
    <ul>
      {for comment in model.comments ->
        <li>
          <strong>{comment.author}</strong>
          <p>{comment.body}</p>
          <p>{Time.format "Jan 2" comment.created}</p>
        </li>}
    </ul>

    <h3>Add a comment</h3>
    <form submit={Submit { slug: model.post.slug }}>
      <Errors form />
      <Input field="author" label="Your name" />
      <Errors field="author" />
      <Input field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>

    <a href={Posts.Index {}}>Back to blog</a>
  </Layout.Page>

--- file: app/Admin/Index.alder ---
module Admin.Index

import Post
import Layout
import Auth.Login
import Posts.Index

page "/admin" { } requires user

type Model = {
  posts: List Post,
}

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

form NewPost = {
  slug: Post.Slug,
  title: Text.NonEmpty,
  body: Text,
}

msg Submit = "/admin/posts" form NewPost
msg TogglePublished = "/admin/posts/{id}/toggle" { id: Post.PostId }
msg Logout = "/logout" { }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, author: session.user }
    Task.succeed Rerender
  },
  TogglePublished p -> {
    let _ <- Db.update Post p.id (\post -> { post | published: not post.published })
    Task.succeed Rerender
  },
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Admin">
    <h1>Blog Admin</h1>

    <h2>Create a post</h2>
    <form submit={Submit}>
      <Errors form />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="body" label="Body" />
      <Errors field="body" />
      <button>Create</button>
    </form>

    <h2>Posts</h2>
    <table>
      <thead>
        <tr>
          <th>Title</th>
          <th>Published</th>
          <th>Action</th>
        </tr>
      </thead>
      <tbody>
        {for post in model.posts ->
          <tr>
            <td>{post.title}</td>
            <td>{Bool.toText post.published}</td>
            <td><button click={TogglePublished { id: post.id }}>Toggle</button></td>
          </tr>}
      </tbody>
    </table>

    <form submit={Logout}>
      <button>Logout</button>
    </form>
  </Layout.Page>
