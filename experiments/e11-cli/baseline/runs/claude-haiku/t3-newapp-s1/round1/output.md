--- file: app/Admin.alder ---
module Admin

import Post
import Layout
import User

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
  body: Text,
}

msg Create form NewPost
msg Toggle { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insertMine Post { slug: p.form.slug, title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Admin {}))
  },
  Toggle p -> {
    let _ <- Db.updateMine Post p.id (\t -> { t | published: not t.published })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin">
    <h1>Posts</h1>
    <al-card>
      <form submit={Create}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-input field="slug" label="Slug" />
        <al-textarea field="body" label="Body" />
        <al-button type="submit">Create post</al-button>
      </form>
    </al-card>
    <ul id="posts">
      {for post in model.posts ->
        <li>
          <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
          {if post.published then "published" else "draft"}
          <al-button variant="secondary" size="sm" click={Toggle { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</al-button>
        </li>}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Admin {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "creating a post adds a draft" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.submit NewPost { title: Text.NonEmpty "Hi", slug: Post.Slug "hi", body: "Body" }
  let drafts <- Db.from Post |> Db.where (Post.published.eq False) |> Db.count
  Task.succeed ({ answer: r.answer, drafts: drafts } == { answer: Redirected (Admin {}), drafts: 1 })
}

test "toggling publishes a post" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "x", published: False, author: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin {})
  let r <- Test.click (Toggle { id: id })
  let after <- Db.get Post id
  Task.succeed ({ answer: r.answer, published: Maybe.map (\p -> p.published) after } == { answer: Rerendered, published: Just True })
}

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
    Task.succeed (Redirect (Home {}))
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

fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
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
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Admin {}), user: Just me })
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

--- file: app/Home.alder ---
module Home

import Post
import Layout
import User

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
    <ul id="posts">
      {for post in model.posts -> <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
  </Layout.Page>

test "/ lists only published posts, newest first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "One", published: True, author: me }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Two", published: False, author: me }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "Three", published: True, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" page.doc == ["Second", "First"])
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>{props.children}</main>
      </al-page>
    </body>
  </html>

--- file: app/Post.alder ---
module Post

import User

type PostId = brand Text

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User @owner,
  created: Time @created,
}

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Comment
import Layout
import User

page "/posts/{slug}" { slug: Post.Slug }

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  let shown = match found {
    Nothing -> Nothing,
    Just post -> if post.published || session.user == (Just post.author) then Just post else Nothing,
  }
  match shown {
    Nothing -> Query.notFound,
    Just post -> {
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      Query.succeed { post: post, comments: comments }
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
  <Layout.Page title={model.post.title}>
    <article>
      <h1>{model.post.title}</h1>
      <p>{model.post.body}</p>
    </article>
    <h2>Comments</h2>
    <ul id="comments">
      {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
    </ul>
    <al-card>
      <form submit={AddComment}>
        <Errors form />
        <al-input field="author" label="Your name" />
        <al-textarea field="body" label="Comment" />
        <al-button type="submit">Post comment</al-button>
      </form>
    </al-card>
    <a href={Home {}}>Back</a>
  </Layout.Page>

test "an unpublished post is not found for a reader" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Secret", published: False, author: me }
  let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == PageMissing)
}

test "the author sees an unpublished post" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Secret", published: False, author: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed (r.answer == Shown)
}

test "a comment with an empty author is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice" }]
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, comments: n } == { answer: FormRejected [{ field: "author", message: "must not be empty" }], comments: 0 })
}

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
