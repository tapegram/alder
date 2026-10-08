--- file: app/App.alder ---
module App

import User

session { user: Maybe User.UserId }

login = Auth.Login

error : Error -> Html Never
error err = <h1>Something went wrong</h1>

notFound : Html Never
notFound = <h1>Not found</h1>

msg Logout = "/logout" { }

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  username: Text.NonEmpty @unique,
}

--- file: app/Post.alder ---
module Post

import User

type PostId = brand Text
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
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
  post: Post.PostId @ref Post,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}

--- file: app/Layout.alder ---
module Layout

import Index
import Admin.Index
import Auth.Login
import User
import App

Layout : { title: Text, user: Maybe User.UserId, children: List (Html Msg) } -> Html Msg
Layout props =
  <html>
    <head><title>{props.title}</title></head>
    <body>
      <nav>
        <a href={Index {}}>Blog</a>
        {match props.user {
          Just u -> <span><a href={Admin.Index {}}>Admin</a> <button click={App.Logout}>Logout</button></span>,
          Nothing -> <a href={Auth.Login {}}>Login</a>,
        }}
      </nav>
      {props.children}
    </body>
  </html>

--- file: app/Index.alder ---
module Index

import Layout
import Post
import Posts.Show
import User

page "/" { }

type Model = { posts: List Post, user: Maybe User.UserId }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts, user: session.user }
}

view : Model -> Html Msg
view model =
  <Layout title="Blog" user={model.user}>
    <h1>Blog</h1>
    {if List.isEmpty model.posts then
      <p>No posts yet.</p>
    else
      <ul>
        {for post in model.posts -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
      </ul>
    }
  </Layout>

--- file: app/Posts/Show.alder ---
module Posts.Show

import Layout
import Post
import Comment
import User

page "/posts/{slug}" { slug: Post.Slug }

type Model = {
  post: Post,
  comments: List Comment,
  user: Maybe User.UserId,
}

load : Params -> Session -> Query Model
load params session = {
  let maybePost <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  let post <- match maybePost {
    Just p -> {
      if p.published || session.user == Just p.author then
        Query.succeed p
      else
        Query.notFound
    },
    Nothing -> Query.notFound,
  }
  let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.desc |> Db.all
  Query.succeed { post: post, comments: comments, user: session.user }
}

view : Model -> Html Msg
view model =
  <Layout title={model.post.title} user={model.user}>
    <h1>{model.post.title}</h1>
    <p>{model.post.body}</p>
    <h2>Comments</h2>
    {if List.isEmpty model.comments then
      <p>No comments yet.</p>
    else
      <ul>
        {for comment in model.comments -> <li><strong>{comment.author}</strong>: {comment.body}</li>}
      </ul>
    }
    <h3>Add a comment</h3>
    <form submit={AddComment}>
      <Errors form />
      <Input field="author" label="Your name" />
      <Errors field="author" />
      <Input field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>
  </Layout>

form AddCommentForm = {
  author: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg AddComment = "/posts/{slug}/comments" { slug: Post.Slug } form AddCommentForm

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed Rerender
  },
}

--- file: app/Admin/Index.alder ---
module Admin.Index

import Layout
import Post
import User

page "/admin" { } requires user

type Model = { posts: List Post, user: User.UserId }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.where (Post.author.eq session.user) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts, user: session.user }
}

view : Model -> Html Msg
view model =
  <Layout title="Admin" user={Just model.user}>
    <h1>Admin Panel</h1>
    <h2>Your Posts</h2>
    {if List.isEmpty model.posts then
      <p>No posts yet.</p>
    else
      <ul>
        {for post in model.posts -> <li>{post.title} {if post.published then <span>(published)</span> else <span>(draft)</span>} <button click={TogglePublish { id: post.id }}>Toggle</button></li>}
      </ul>
    }
    <h2>Create a new post</h2>
    <form submit={Create}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Input field="body" label="Body" />
      <Errors field="body" />
      <button>Create</button>
    </form>
  </Layout>

form CreatePostForm = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text.NonEmpty,
}

msg TogglePublish = "/admin/posts/{id}/toggle" { id: Post.PostId }
msg Create = "/admin/posts" form CreatePostForm

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  TogglePublish p -> {
    let _ <- Db.update Post p.id (\post -> { post | published: not post.published })
    Task.succeed Rerender
  },
  Create p -> {
    let id <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
    Task.succeed (Redirect (Admin.Index {}))
  },
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Index
import User

page "/login" { }

type Model = {}

load : Params -> Session -> Query Model
load params session = Query.succeed {}

view : Model -> Html Msg
view model =
  <Layout title="Log in" user={Nothing}>
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="username" label="Username" />
      <Errors field="username" />
      <button>Log in</button>
    </form>
  </Layout>

form LoginForm = {
  username: Text.NonEmpty,
}

msg Submit = "/login" form LoginForm

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let user <- Db.from User |> Db.where (User.username.eq p.form.username) |> Db.first
    match user {
      Just u -> {
        let _ <- Session.put (\s -> { s | user: Just u.id })
        Task.succeed (Redirect (Index {}))
      },
      Nothing -> Task.succeed (Reject "User not found"),
    }
  },
}
