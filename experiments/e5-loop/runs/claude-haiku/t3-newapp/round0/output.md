--- file: app/App.alder ---
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Home {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" theme={Light} loggedIn={False}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light} loggedIn={False}>
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme, loggedIn: Bool, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <nav>
        <a href={Home {}}>Blog</a>
        {if props.loggedIn then <a href={Admin {}}>Admin</a> else Html.none}
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
        {if props.loggedIn then <button click={App.Logout}>Log out</button> else <a href={Auth.Login {}}>Log in</a>}
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
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text.NonEmpty,
  published: Bool = False,
  author: User.UserId @ref User,
  created: Time @created,
}

test "slug pattern rejects uppercase" = Text.matches "^[a-z0-9-]+$" "Hello" == False

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

--- file: app/Home.alder ---
module Home

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
  <Layout.Page title="Blog" theme={session.theme} loggedIn={Maybe.isJust session.user}>
    <h1>Blog</h1>
    {if List.isEmpty model.posts then <p>No posts yet.</p> else Html.none}
    <ul id="posts">
      {for p in model.posts -> <PostCard post={p} />}
    </ul>
  </Layout.Page>

PostCard : { post: Post } -> Html Msg
PostCard props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span class="meta">{Time.format "Jan 2, 2006" props.post.created}</span>
  </li>

--- file: app/Posts/Show.alder ---
module Posts.Show

import User
import Post
import Comment
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = {
  post: Post,
  comments: List Comment,
}

-- Drafts are visible only to their author.
visible : Post -> Maybe User.UserId -> Bool
visible post user = match user {
  Just u -> post.published || u == post.author,
  Nothing -> post.published,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> {
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      if visible post session.user then Query.succeed { post: post, comments: comments } else Query.notFound
    },
  }
}

form NewComment = {
  author: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg AddComment form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} theme={session.theme} loggedIn={Maybe.isJust session.user}>
    <article>
      <h1>{model.post.title}</h1>
      <p class="meta">{Time.format "Jan 2, 2006" model.post.created}</p>
      {if model.post.published then Html.none else <p class="draft">Draft: only the author can see this.</p>}
      <p>{model.post.body}</p>
    </article>

    <section id="comments">
      <h2>Comments</h2>
      {if List.isEmpty model.comments then <p>No comments yet.</p> else Html.none}
      <ul>
        {for c in model.comments -> <CommentItem comment={c} />}
      </ul>
    </section>

    <h2>Leave a comment</h2>
    <form submit={AddComment}>
      <Errors form />
      <Input field="author" label="Name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>

    <a href={Home {}}>Back to posts</a>
  </Layout.Page>

CommentItem : { comment: Comment } -> Html Msg
CommentItem props =
  <li>
    <strong>{props.comment.author}</strong>
    <p>{props.comment.body}</p>
    <time>{Time.format "Jan 2, 2006" props.comment.created}</time>
  </li>

--- file: app/Admin.alder ---
module Admin

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
  body: Text.NonEmpty,
}

msg Submit form NewPost
msg Toggle { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let existing <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.first
    match existing {
      Just _ -> Task.succeed (Reject "That slug is already in use."),
      Nothing -> {
        let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
        Task.succeed (Redirect (Admin {}))
      },
    }
  },
  Toggle p -> {
    let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin" theme={session.theme} loggedIn={True}>
    <h1>Admin</h1>

    <h2>New post</h2>
    <form submit={Submit}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Create post</button>
    </form>

    <h2>All posts</h2>
    <ul id="posts">
      {for post in model.posts -> <PostRow post={post} />}
    </ul>
  </Layout.Page>

PostRow : { post: Post } -> Html Msg
PostRow props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    {if props.post.published then <span>Published</span> else <span>Draft</span>}
    <button click={Toggle { id: props.post.id }}>{if props.post.published then "Unpublish" else "Publish"}</button>
  </li>

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
        Task.succeed (Redirect (Admin {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" theme={session.theme} loggedIn={Maybe.isJust session.user}>
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
