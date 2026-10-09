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

type PostId = brand Text

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text.NonEmpty,
  published: Bool = False,
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

import User

Page : { title: Text, user: Maybe User.UserId, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <nav>
        <a href={Blog.Index {}}>My Blog</a>
        {match props.user {
          Just u -> <span><a href={Admin.Index {}}>Admin</a> <button click={App.Logout}>Log out</button></span>,
          Nothing -> <a href={Auth.Login {}}>Log in</a>,
        }}
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Blog.Index {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" user={Nothing}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" user={Nothing}>
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

msg Submit form Credentials

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

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" user={session.user}>
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
  <Layout.Page title="My Blog" user={session.user}>
    <h1>My Blog</h1>
    {if List.isEmpty model.posts then <p>No posts yet.</p> else <ul id="posts">{for post in model.posts -> <Item post={post} />}</ul>}
  </Layout.Page>

Item : { post: Post } -> Html Msg
Item props =
  <li>
    <a href={Blog.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span class="date"> {Time.format "Jan 2, 2006" props.post.created}</span>
  </li>

--- file: app/Blog/Show.alder ---
module Blog.Show

import Post
import Comment
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post -> if post.published || Maybe.isJust session.user then withComments post else Query.notFound,
    Nothing -> Query.notFound,
  }
}

withComments : Post -> Query Model
withComments post = {
  let comments <-
    Db.from Comment
      |> Db.where (Comment.post.eq post.id)
      |> Db.orderBy Comment.created.asc
      |> Db.all
  Query.succeed { post: post, comments: comments }
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
    Task.succeed (Redirect (Blog.Show { slug: model.post.slug }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} user={session.user}>
    <article>
      <h1>{model.post.title}</h1>
      <p class="date">{Time.format "Jan 2, 2006" model.post.created}</p>
      {if model.post.published then Html.none else <p class="draft">Draft: only you can see this post.</p>}
      <div class="body">{model.post.body}</div>
    </article>

    <section id="comments">
      <h2>Comments</h2>
      {if List.isEmpty model.comments then <p>No comments yet.</p> else <ul>{for c in model.comments -> <CommentItem comment={c} />}</ul>}
    </section>

    <form submit={AddComment}>
      <h3>Add a comment</h3>
      <Errors form />
      <Input field="author" label="Your name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Add comment</button>
    </form>

    <a href={Blog.Index {}}>Back</a>
  </Layout.Page>

CommentItem : { comment: Comment } -> Html Msg
CommentItem props =
  <li>
    <strong>{props.comment.author}</strong>
    <span class="date"> {Time.format "Jan 2, 2006" props.comment.created}</span>
    <p>{props.comment.body}</p>
  </li>

--- file: app/Admin/Index.alder ---
module Admin.Index

import Post
import Layout

page "/admin" { } requires user

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <-
    Db.from Post
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

form NewPost = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text.NonEmpty,
  published: Bool,
}

msg Create form NewPost
msg TogglePublished { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let existing <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.first
    match existing {
      Just other -> Task.succeed (Reject "That slug is already used."),
      Nothing -> {
        let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
        Task.succeed (Redirect (Admin.Index {}))
      },
    }
  },
  TogglePublished p -> {
    let found <- Db.get Post p.id
    match found {
      Just post -> {
        let _ <- Db.update Post post.id (\x -> { x | published: not x.published })
        Task.succeed Rerender
      },
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin" user={session.user}>
    <h1>Admin</h1>

    <form submit={Create}>
      <h2>New post</h2>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <Input field="published" label="Published" />
      <Errors field="published" />
      <button>Create post</button>
    </form>

    <h2>All posts</h2>
    <ul id="posts">
      {for post in model.posts -> <Row post={post} />}
    </ul>
  </Layout.Page>

Row : { post: Post } -> Html Msg
Row props =
  <li>
    <a href={Blog.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span class="status"> {if props.post.published then "Published" else "Draft"} </span>
    <button click={TogglePublished { id: props.post.id }}>{if props.post.published then "Unpublish" else "Publish"}</button>
  </li>
