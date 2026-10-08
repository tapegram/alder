--- file: app/App.alder ---
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout = "/logout" { }
msg SwitchTheme = "/theme" { }

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
        Task.succeed (Redirect (Blogs.Index {}))
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
        <a href={Blogs.Index {}}>Blog</a>
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
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  post: Post.PostId @ref Post,
  created: Time @created,
}

--- file: app/Blogs/Index.alder ---
module Blogs.Index

import Post
import Layout

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

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Blog" theme={session.theme}>
    <h1>Blog</h1>
    {if List.isEmpty model.posts then
      <p>No posts yet.</p>
    else
      <ul>
        {for post in model.posts -> <PostLink post={post} />}
      </ul>
    }
    {match session.user {
      Just _ -> <a href={Admin.Index {}}>Admin</a>,
      Nothing -> <a href={Auth.Login {}}>Log in</a>,
    }}
  </Layout.Page>

PostLink : { post: Post } -> Html Msg
PostLink props =
  <li>
    <a href={Blogs.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span>{Date.format "Jan 2, 2006" (Time.toDate props.post.created)}</span>
  </li>

--- file: app/Blogs/Show.alder ---
module Blogs.Show

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
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post -> {
      let comments <- Db.from Comment |> Db.where (Comment.post.eq post.id) |> Db.orderBy Comment.created.asc |> Db.all
      if post.published then
        Query.succeed { post: post, comments: comments }
      else
        match session.user {
          Just u -> if u == post.author then Query.succeed { post: post, comments: comments } else Query.notFound,
          Nothing -> Query.notFound,
        }
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
    let _ <- Db.insert Comment { author: p.form.author, body: p.form.body, post: model.post.id }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} theme={session.theme}>
    <h1>{model.post.title}</h1>
    <p class="meta">{Date.format "Jan 2, 2006" (Time.toDate model.post.created)}</p>
    <div class="body">{Html.unsafe model.post.body}</div>

    <h2>Comments</h2>
    {if List.isEmpty model.comments then
      <p>No comments yet.</p>
    else
      <ul>
        {for comment in model.comments -> <CommentItem comment={comment} />}
      </ul>
    }

    <h2>Add a comment</h2>
    <form submit={Submit { slug: model.post.slug }}>
      <Errors form />
      <Input field="author" label="Name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post</button>
    </form>

    <a href={Blogs.Index {}}>Back</a>
  </Layout.Page>

CommentItem : { comment: Comment } -> Html Msg
CommentItem props =
  <li>
    <p class="author">{props.comment.author}</p>
    <p>{props.comment.body}</p>
    <p class="meta">{Date.format "Jan 2, 2006" (Time.toDate props.comment.created)}</p>
  </li>

--- file: app/Admin/Index.alder ---
module Admin.Index

import Post
import User
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

msg Create = "/admin/posts" form NewPost
msg PublishToggle = "/admin/posts/{id}/toggle" { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
    Task.succeed Rerender
  },
  PublishToggle p -> {
    let _ <- Db.update Post p.id (\post -> { post | published: not post.published })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin" theme={session.theme}>
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
      <button>Create</button>
    </form>

    <h2>Posts</h2>
    {if List.isEmpty model.posts then
      <p>No posts yet.</p>
    else
      <ul>
        {for post in model.posts -> <PostRow post={post} />}
      </ul>
    }

    <a href={Blogs.Index {}}>View blog</a>
  </Layout.Page>

PostRow : { post: Post } -> Html Msg
PostRow props =
  <li>
    <span>{props.post.title}</span>
    <span class={if props.post.published then "published" else "draft"}>{if props.post.published then "Published" else "Draft"}</span>
    <button click={PublishToggle { id: props.post.id }}>{if props.post.published then "Unpublish" else "Publish"}</button>
    <a href={Blogs.Show { slug: props.post.slug }}>View</a>
  </li>
