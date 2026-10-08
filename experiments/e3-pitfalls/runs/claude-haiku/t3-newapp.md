--- file: app/App.alder ---
module App

import User
import Layout

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
        Task.succeed (Redirect (Blog.Index {}))
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

--- file: app/Blog/Index.alder ---
module Blog.Index

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
    <h1>Blog</h1>
    {if List.isEmpty model.posts then
      <p>No posts published yet.</p>
    else
      <ul>
        {for post in model.posts ->
          <li>
            <a href={Blog.Show { slug: post.slug }}>{post.title}</a>
            <p>{Date.format "Jan 2, 2006" (Time.toDate post.created)}</p>
          </li>
        }
      </ul>
    }
    <nav>
      {match session.user {
        Just id -> <a href={Admin {}}>Admin</a>,
        Nothing -> <a href={Auth.Login {}}>Log in</a>,
      }}
    </nav>
  </Layout.Page>

--- file: app/Blog/Show.alder ---
module Blog.Show

import Post
import Comment
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = {
  post: Post,
  comments: List Comment,
}

load : Params -> Session -> Query Model
load params session = {
  let post <-
    Db.from Post
      |> Db.where (Post.slug.eq params.slug)
      |> Db.first
  match post {
    Just p -> {
      let isAuthor = match session.user {
        Just id -> id == p.author,
        Nothing -> False,
      }
      if p.published || isAuthor then {
        let comments <-
          Db.from Comment
            |> Db.where (Comment.post.eq p.id)
            |> Db.orderBy Comment.created.desc
            |> Db.all
        Query.succeed { post: p, comments: comments }
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

msg AddComment = "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

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
    <h1>{model.post.title}</h1>
    <p>{Date.format "Jan 2, 2006" (Time.toDate model.post.created)}</p>
    <article>{Html.unsafe model.post.body}</article>

    <h2>Comments</h2>
    {if List.isEmpty model.comments then
      <p>No comments yet.</p>
    else
      <ul>
        {for comment in model.comments ->
          <li>
            <strong>{comment.author}</strong>
            <p>{comment.body}</p>
            <small>{Date.format "Jan 2, 2006" (Time.toDate comment.created)}</small>
          </li>
        }
      </ul>
    }

    <h2>Add a comment</h2>
    <form submit={AddComment { slug: model.post.slug }}>
      <Errors form />
      <Input field="author" label="Name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post</button>
    </form>

    <a href={Blog.Index {}}>Back to blog</a>
  </Layout.Page>

--- file: app/Admin.alder ---
module Admin

import Post
import Layout

page "/admin" { } requires user

type Model = {
  posts: List Post,
}

load : Params -> Session -> Query Model
load params session = {
  let posts <-
    Db.from Post
      |> Db.where (Post.author.eq session.user)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

form NewPost = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text.NonEmpty,
}

msg CreatePost = "/admin/posts" form NewPost
msg TogglePublished = "/admin/posts/{id}/toggle" { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  CreatePost p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
    Task.succeed (Redirect (Admin {}))
  },
  TogglePublished p -> {
    let _ <- Db.update Post p.id (\post -> { post | published: not post.published })
    Task.succeed Rerender
  },
}

PostRow : { post: Post } -> Html Msg
PostRow props =
  <li>
    <strong>{props.post.title}</strong> ({props.post.slug})
    {if props.post.published then " [published]" else " [draft]"}
    <button click={TogglePublished { id: props.post.id }}>
      {if props.post.published then "Unpublish" else "Publish"}
    </button>
  </li>

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin">
    <h1>Admin</h1>

    <h2>Create Post</h2>
    <form submit={CreatePost}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Create</button>
    </form>

    <h2>Your Posts</h2>
    {if List.isEmpty model.posts then
      <p>No posts yet.</p>
    else
      <ul>
        {for post in model.posts -> <PostRow post={post} />}
      </ul>
    }

    <a href={Blog.Index {}}>Back to blog</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
