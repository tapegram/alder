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
    Task.succeed (Redirect (Auth.Login {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" loggedIn={False}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" loggedIn={False}>
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, loggedIn: Bool, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <nav>
        <a href={Posts.Index {}}>Blog</a>
        {if props.loggedIn then <a href={Admin.Index {}}>Admin</a> else Html.none}
        {if props.loggedIn then <button click={App.Logout}>Log out</button> else <a href={Auth.Login {}}>Log in</a>}
      </nav>
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

canView : Bool -> Post -> Bool
canView loggedIn post = post.published || loggedIn

statusLabel : Post -> Text
statusLabel post = if post.published then "Published" else "Draft"

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
  <Layout.Page title="Log in" loggedIn={Maybe.isJust session.user}>
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
  <Layout.Page title="Blog" loggedIn={Maybe.isJust session.user}>
    <h1>Blog</h1>
    {if model.posts == [] then
      <p>No posts yet.</p>
    else
      <ul id="posts">
        {for post in model.posts -> <Item post={post} />}
      </ul>}
  </Layout.Page>

Item : { post: Post } -> Html Msg
Item props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span class="date">{Time.format "Jan 2, 2006" props.post.created}</span>
  </li>

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Comment
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post -> if Post.canView (Maybe.isJust session.user) post then {
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      Query.succeed { post: post, comments: comments }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  author: Text.NonEmpty,
  body: Text.NonEmpty,
}

msg AddComment { post: Post.PostId } form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: p.post, author: p.form.author, body: p.form.body }
    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} loggedIn={Maybe.isJust session.user}>
    <article>
      <h1>{model.post.title}</h1>
      <p class="date">{Time.format "Jan 2, 2006" model.post.created}</p>
      <div>{model.post.body}</div>
    </article>

    <section id="comments">
      <h2>Comments</h2>
      {if model.comments == [] then
        <p>No comments yet.</p>
      else
        <ul>
          {for c in model.comments ->
            <li>
              <strong>{c.author}</strong>
              <p>{c.body}</p>
            </li>}
        </ul>}
    </section>

    <form submit={AddComment { post: model.post.id }}>
      <Errors form />
      <Input field="author" label="Your name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Add comment</button>
    </form>
  </Layout.Page>

--- file: app/Admin/Index.alder ---
module Admin.Index

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
  published: Bool,
}

msg Create form NewPost
msg TogglePublished { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let existing <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.first
    match existing {
      Just other -> Task.succeed (Reject "That slug is already taken."),
      Nothing -> {
        let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published }
        Task.succeed (Redirect (Admin.Index {}))
      },
    }
  },
  TogglePublished p -> {
    let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin" loggedIn={True}>
    <h1>Admin</h1>

    <form submit={Create}>
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

    <ul id="posts">
      {for post in model.posts -> <Row post={post} />}
    </ul>
  </Layout.Page>

Row : { post: Post } -> Html Msg
Row props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span class="status">{Post.statusLabel props.post}</span>
    <button click={TogglePublished { id: props.post.id }}>{if props.post.published then "Unpublish" else "Publish"}</button>
  </li>
