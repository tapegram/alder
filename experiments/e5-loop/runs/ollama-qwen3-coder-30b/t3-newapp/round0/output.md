--- file: app/App.alder ---
module App

import Layout
import Post
import User

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

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

msg Submit form Credentials

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
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Post.alder ---
module Post

import User

type PostId = brand Text
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User,
  created: Time @created,
}

table Comment {
  id: brand Text @id,
  post: PostId @ref Post,
  authorName: Text.NonEmpty,
  body: Text,
  created: Time @created,
}

test "slug validation allows lowercase letters digits and hyphens" =
  Slug "hello-world-123" == Slug "hello-world-123"

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import Layout

page "/" { }

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Blog" theme={session.theme}>
    <h1>Blog</h1>
    {for post in model.posts ->
      <article>
        <h2><a href={Posts.Show { slug: post.slug }}>{post.title}</a></h2>
        <p class="meta">Published {Date.format "Jan 2, 2006" (Time.toDate post.created)}</p>
        <p>{Text.slice 0 150 post.body}...</p>
      </article>}
  </Layout.Page>

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Layout

page "/posts/{slug}" { slug: Post.Slug } requires user

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Post params.slug
  match found {
    Just post -> if post.published || post.author == session.user then {
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

msg Submit form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Comment { post: model.post.id, authorName: p.form.authorName, body: p.form.body }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} theme={session.theme}>
    <article>
      <h1>{model.post.title}</h1>
      <p class="meta">Published {Date.format "Jan 2, 2006" (Time.toDate model.post.created)}</p>
      <div>{model.post.body}</div>
    </article>

    <h2>Comments</h2>
    {for comment in model.comments ->
      <article>
        <p class="meta">By {comment.authorName} on {Date.format "Jan 2, 2006" (Time.toDate comment.created)}</p>
        <p>{comment.body}</p>
      </article>}

    <h2>Add a comment</h2>
    <form submit={Submit}>
      <Errors form />
      <Input field="authorName" label="Your name" />
      <Errors field="authorName" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>
  </Layout.Page>

--- file: app/Posts/Admin.alder ---
module Posts.Admin

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
  slug: Post.Slug @unique,
  body: Text,
  published: Bool = False,
}

msg Create form NewPost
msg TogglePublished { id: Post.PostId }

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
      <label>
        <Input field="published" type="checkbox" />
        Published
      </label>
      <Errors field="published" />
      <button>Create</button>
    </form>

    <h2>Posts</h2>
    {for post in model.posts ->
      <article>
        <h3>{post.title}</h3>
        <p>Slug: {post.slug}</p>
        <p>Published: {Bool.toText post.published}</p>
        <button click={TogglePublished { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</button>
      </article>}
  </Layout.Page>

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light