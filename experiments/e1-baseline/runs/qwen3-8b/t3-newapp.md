<!-- model=qwen3:8b task=t3-newapp.md seconds=182 prompt_tokens=11241 output_tokens=5302 -->
--- file: app/App.alder ---
module App

import User
import Layout
import Auth.Login
import Post.Index
import Post.Show
import Post.Admin

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

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  slug: Slug @id,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  owner: User.UserId @ref User,
  created: Time @created,
}

--- file: app/Post/Index.alder ---
module Post.Index

import Post
import User
import Layout
import Post.Show

page "/" { } requires user

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
    <ul id="posts">
      {for post in model.posts -> <PostItem post={post} />}
    </ul>
  </Layout.Page>

PostItem : { post: Post } -> Html Msg
PostItem props =
  <li>
    <a href={Post.Show { slug: props.post.slug }}>{props.post.title}</a>
  </li>

--- file: app/Post/Show.alder ---
module Post.Show

import Post
import Layout
import Comment
import Post.Admin

page "/posts/{slug}" { slug: Slug } requires user

type Model = {
  post: Post,
  comments: List Comment,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Post params.slug
  match found {
    Just post -> if post.published || post.owner == session.user then
      let comments <- Db.from Comment
        |> Db.where (Comment.post.eq params.slug)
        |> Db.all
      Query.succeed { post: post, comments: comments }
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  author: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg Submit = "/posts/{slug}/comment" form NewComment
msg Delete = "/posts/{slug}/delete" { }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Comment { post: params.slug, author: p.form.author, body: p.form.body }
    Task.succeed (Redirect (Post.Show { slug: params.slug }))
  },
  Delete p -> {
    let _ <- Db.delete Comment params.slug
    Task.succeed (Redirect (Post.Show { slug: params.slug }))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.post.title}>
    <h1>{model.post.title}</h1>
    <div>{model.post.body}</div>
    <h2>Comments</h2>
    <form submit={Submit}>
      <Input field="author" label="Author" />
      <Errors field="author" />
      <Input field="body" label="Body" />
      <Errors field="body" />
      <button>Add comment</button>
    </form>
    <ul>
      {for comment in model.comments -> <CommentItem comment={comment} />}
    </ul>
  </Layout.Page>

CommentItem : { comment: Comment } -> Html Msg
CommentItem props =
  <li>
    <strong>{props.comment.author}</strong>: {props.comment.body}
  </li>

--- file: app/Post/Admin.alder ---
module Post.Admin

import Post
import User
import Layout
import Auth.Login

page "/admin" { } requires user

type Model = {
  posts: List Post,
}

load : Params -> Session -> Query Model
load params session = {
  let posts <-
    Db.from Post
      |> Db.all
  Query.succeed { posts: posts }
}

form NewPost = {
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
}

msg Create = "/posts" form NewPost
msg Toggle = "/posts/{slug}/toggle" { slug: Slug }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Post { title: p.form.title, body: p.form.body, published: p.form.published, owner: session.user }
    Task.succeed (Redirect (Post.Admin {}))
  },
  Toggle p -> {
    let _ <- Db.update Post p.slug (\t -> { t | published: not t.published })
    Task.succeed (Redirect (Post.Admin {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Admin">
    <h1>Admin</h1>
    <form submit={Create}>
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="body" label="Body" />
      <Errors field="" />
      <button>Create post</button>
    </form>
    <ul>
      {for post in model.posts -> <PostRow post={post} />}
    </ul>
  </Layout</Layout.Page>

PostRow : { post: Post } -> Html Msg
PostRow props =
  <li>
    <a href={Post.Show { slug: props.post.slug }}>{props.post.title}</a>
    <button click={Toggle { slug: props.post.slug }}>Toggle published</button>
  </li>

--- file: app/Comment.alder ---
module Comment

table Comment {
  id: CommentId @id,
  post: Slug @ref Post,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout
import Post.Index

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
        Task.succeed (Redirect (Post.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect.")
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