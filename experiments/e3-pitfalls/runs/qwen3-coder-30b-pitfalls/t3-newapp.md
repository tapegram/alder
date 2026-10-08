<!-- model=qwen3-coder:30b task=t3-newapp.md seconds=46 prompt_tokens=12723 output_tokens=2187 -->
--- file: app/App.alder ---
module App

import Layout
import User
import Post

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
        Task.succeed (Redirect (Blog.Index {}))
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

--- file: app/Blog/Index.alder ---
module Blog.Index

import Post
import User
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
        <h2><a href={Blog.Show { slug: post.slug }}>{post.title}</a></h2>
        <p class="meta">Published on {Date.format "Jan 2, 2006" (Time.toDate post.created)}</p>
        <p>{Text.slice 0 150 post.body}...</p>
      </article>}
  </Layout.Page>

--- file: app/Blog/Show.alder ---
module Blog.Show

import Post
import User
import Layout

page "/posts/{slug}" { slug: Post.Slug } requires user

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Post params.slug
  match found {
    Just post ->
      if post.owner == session.user || post.published then {
        let comments <- Db.from Comment |> Db.where (Comment.post.eq params.slug) |> Db.orderBy Comment.created.desc |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound,
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
    let _ <- Db.insert Comment { author: p.form.author, body: p.form.body, post: model.post.slug }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} theme={session.theme}>
    <h1>{model.post.title}</h1>
    <p class="meta">Published on {Date.format "Jan 2, 2006" (Time.toDate model.post.created)}</p>
    <div>{model.post.body}</div>

    <h2>Comments</h2>
    {for comment in model.comments ->
      <article>
        <p class="comment-author">{comment.author}</p>
        <p>{comment.body}</p>
      </article>}

    <h2>Add a comment</h2>
    <form submit={Submit { slug: model.post.slug }}>
      <Errors form />
      <Input field="author" label="Your name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>

    {if model.post.owner == session.user ->
      <a href={Blog.Admin {}}>Back to admin</a>
    }
  </Layout.Page>

--- file: app/Blog/Admin.alder ---
module Blog.Admin

import Post
import User
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
  published: Bool = False,
}

msg Create = "/admin/posts" form NewPost
msg TogglePublished = "/admin/posts/{slug}/toggle" { slug: Post.Slug }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, published: p.form.published, owner: session.user }
    Task.succeed Rerender
  },
  TogglePublished p -> {
    let post <- Db.get Post p.slug
    match post {
      Just p -> {
        let _ <- Db.update Post p.slug (\t -> { t | published: not t.published })
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

    <h2>Create a new post</h2>
    <form submit={Create}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <Input field="published" label="Published" type="checkbox" />
      <Errors field="published" />
      <button>Create</button>
    </form>

    <h2>Posts</h2>
    {for post in model.posts ->
      <article>
        <h3>{post.title}</h3>
        <p>Slug: {post.slug}</p>
        <p>Published: {Bool.toText post.published}</p>
        <button click={TogglePublished { slug: post.slug }}>
          {if post.published then "Unpublish" else "Publish"}
        </button>
        <a href={Blog.Show { slug: post.slug }}>View</a>
      </article>}

    <button click={App.Logout}>Log out</button>
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
  body: Text.NonEmpty,
  published: Bool = False,
  owner: User.UserId @ref User,
  created: Time @created,
}

table Comment {
  id: Text @id,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  post: Post.Slug @ref Post,
  created: Time @created,
}

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light