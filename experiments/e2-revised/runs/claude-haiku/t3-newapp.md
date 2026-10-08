--- file: app/App.alder ---
module App

import User
import Layout
import Auth.Login

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout = "/logout" { }

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

type PostSlug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostSlug @id,
  title: Text.NonEmpty,
  body: Text.NonEmpty,
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
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  post: Post.PostSlug @ref Post,
  created: Time @created,
}

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import User
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
          <a href={Posts.Show { slug: post.id }}>{post.title}</a>
          <span>{Date.format "Jan 2, 2006" (Time.toDate post.created)}</span>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Comment
import Layout
import Posts.Index
import App

page "/posts/{slug}" { slug: Post.PostSlug }

type Model = {
  post: Post,
  comments: List Comment,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Post params.slug
  match found {
    Just post -> {
      let canView = post.published || (match session.user {
        Just u -> post.author == u,
        Nothing -> False,
      })
      if canView then {
        let comments <- Db.from Comment |> Db.where (Comment.post.eq params.slug) |> Db.orderBy Comment.created.asc |> Db.all
        Query.succeed { post: post, comments: comments }
      } else {
        Query.notFound
      }
    },
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  author: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg AddComment = "/posts/{slug}/comment" { slug: Post.PostSlug } form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { author: p.form.author, body: p.form.body, post: p.slug }
    Task.succeed Rerender
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    <p>{Date.format "Jan 2, 2006" (Time.toDate model.post.created)}</p>
    <p>{model.post.body}</p>
    
    <h2>Comments</h2>
    <ul>
      {for comment in model.comments ->
        <li>
          <strong>{comment.author}</strong>
          <p>{comment.body}</p>
        </li>}
    </ul>
    
    <h2>Add Comment</h2>
    <form submit={AddComment { slug: model.post.id }}>
      <Input field="author" label="Name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post Comment</button>
    </form>
    
    <a href={Posts.Index {}}>Back to Blog</a>
  </Layout.Page>

--- file: app/Admin/Index.alder ---
module Admin.Index

import Post
import User
import Layout
import Posts.Index
import App

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
  id: Post.PostSlug,
  title: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg Create = "/admin/posts" form NewPost
msg TogglePublished = "/admin/posts/{id}/toggle" { id: Post.PostSlug }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Post { id: p.form.id, title: p.form.title, body: p.form.body, author: session.user }
    Task.succeed (Redirect (Admin.Index {}))
  },
  TogglePublished p -> {
    let _ <- Db.update Post p.id (\post -> { post | published: not post.published })
    Task.succeed Rerender
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Admin">
    <h1>Admin - Manage Posts</h1>
    
    <h2>Create Post</h2>
    <form submit={Create}>
      <Input field="id" label="Slug" />
      <Errors field="id" />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Create</button>
    </form>
    
    <h2>Posts</h2>
    <table>
      <thead>
        <tr>
          <th>Title</th>
          <th>Slug</th>
          <th>Published</th>
          <th>Created</th>
          <th>Action</th>
        </tr>
      </thead>
      <tbody>
        {for post in model.posts ->
          <tr>
            <td>{post.title}</td>
            <td>{post.id}</td>
            <td>{if post.published then "Yes" else "No"}</td>
            <td>{Date.format "Jan 2, 2006" (Time.toDate post.created)}</td>
            <td>
              <button click={TogglePublished { id: post.id }}>
                {if post.published then "Unpublish" else "Publish"}
              </button>
            </td>
          </tr>}
      </tbody>
    </table>
    
    <button click={App.Logout}>Log out</button>
    <a href={Posts.Index {}}>Back to Blog</a>
  </Layout.Page>
