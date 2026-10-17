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

test "Logout clears the user and goes home" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Home {})), trace: ["Session.put"] })
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
        <nav><a href={Home {}}>Blog</a></nav>
        <main>{props.children}</main>
      </al-page>
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

-- A slug is the post's address: lowercase letters, digits and hyphens.
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

-- `author` is the table's `@owner`: the admin writes only the signed-in user's posts.
table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User @owner,
  created: Time @created,
}

--- file: app/Comment.alder ---
module Comment

import Post

type CommentId = brand Text

-- A post's comments go when the post goes.
table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
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
        Task.succeed (Redirect (Admin.Posts {}))
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

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Admin.Posts {}), user: Just me })
}

test "logging out from the admin" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Posts {})
  let out <- Test.click App.Logout
  Task.succeed ({ out: out.answer, user: out.session.user } == { out: Redirected (Home {}), user: Nothing })
}

--- file: app/Home.alder ---
module Home

import Post
import User
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
    <ul id="posts">
      {for post in model.posts ->
        <li><a href={Posts.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
  </Layout.Page>

test "the front page lists only published posts, newest first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "One", published: True, author: me }
  let _ <- Db.insert Post { slug: Post.Slug "hidden", title: Text.NonEmpty "Hidden", body: "Draft", published: False, author: me }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "Two", published: True, author: me }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#posts a" page.doc == ["Second", "First"])
}

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Comment
import User
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = { post: Post, comments: List Comment }

-- An unpublished post is shown only to its author; to anyone else it is a 404, like a slug
-- that names no post.
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
      if post.published || session.user == Just post.author
        then Query.succeed { post: post, comments: comments }
        else Query.notFound
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
      <p id="body">{model.post.body}</p>
    </article>

    <h2>Comments</h2>
    <ul id="comments">
      {for c in model.comments ->
        <li><strong class="author">{c.author}</strong> <span class="body">{c.body}</span></li>}
    </ul>

    <al-card>
      <form submit={AddComment}>
        <Errors form />
        <al-input field="author" label="Name" />
        <al-textarea field="body" label="Comment" />
        <al-button type="submit">Add comment</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unpublished post is not found for a reader" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Soon", published: False, author: me }
  let visitor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let other <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ visitor: visitor.answer, other: other.answer } == { visitor: PageMissing, other: PageMissing })
}

test "the author sees their unpublished post" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Soon", published: False, author: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ answer: page.answer, title: Test.text "h1" page.doc } == { answer: Shown, title: Just "Draft" })
}

test "a comment with an empty author is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi there", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "" }, { name: "body", value: "Nice post" }]
  let n <- Db.from Comment |> Db.count
  let refused = match r.answer {
    FormRejected errs -> List.any (\e -> e.field == "author" || Text.startsWith "author" e.message) errs,
    _ -> False,
  }
  Task.succeed ({ refused: refused, comments: n } == { refused: True, comments: 0 })
}

test "a comment is added through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi there", published: True, author: me }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice post" }
  Task.succeed ({ answer: r.answer, authors: Test.texts "#comments .author" r.doc } == { answer: Redirected (Posts.Show { slug: Post.Slug "hello" }), authors: ["Ann"] })
}

--- file: app/Admin/Posts.alder ---
module Admin.Posts

import Post
import User
import Layout

page "/admin" { } requires user

type Model = { posts: List Post }

-- On a personal blog every post is the signed-in author's: `Db.mine Post` lists them all,
-- published or not, and the writes below act only on the author's own rows.
load : Params -> Session -> Query Model
load params session = {
  let posts <-
    Db.mine Post
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
    let taken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.count
    if taken > 0
      then Task.succeed (Reject "That slug is already taken.")
      else {
        let _ <- Db.insertMine Post { slug: p.form.slug, title: p.form.title, body: p.form.body, published: p.form.published }
        Task.succeed (Redirect (Admin.Posts {}))
      }
  },
  TogglePublished p -> {
    let _ <- Db.updateMine Post p.id (\t -> { t | published: not t.published })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Posts">
    <h1>Posts</h1>

    <ul id="posts">
      {for post in model.posts ->
        <li class={if post.published then "published" else "draft"}>
          <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
          <al-badge variant="outline">{if post.published then "Published" else "Draft"}</al-badge>
          <al-button variant="secondary" size="sm" click={TogglePublished { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</al-button>
        </li>}
    </ul>

    <h2>New post</h2>
    <al-card>
      <form submit={Create}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-input field="slug" label="Slug" />
        <al-textarea field="body" label="Body" />
        <al-checkbox field="published" label="Published" />
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a visitor is sent to log in from the admin" = {
  let visitor <- Test.open (Admin.Posts {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "creating a post through the admin" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Posts {})
  let r <- Test.submit NewPost { title: Text.NonEmpty "Hello", slug: Post.Slug "hello", body: Text.NonEmpty "Hi there", published: False }
  let page <- Test.get (Admin.Posts {})
  Task.succeed ({ answer: r.answer, titles: Test.texts "#posts a" page.doc, drafts: Test.count "li.draft" page.doc } == { answer: Redirected (Admin.Posts {}), titles: ["Hello"], drafts: 1 })
}

test "a taken slug is rejected" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi", published: True, author: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Posts {})
  let r <- Test.submit NewPost { title: Text.NonEmpty "Again", slug: Post.Slug "hello", body: Text.NonEmpty "Hi", published: False }
  let n <- Db.from Post |> Db.count
  Task.succeed ({ answer: r.answer, posts: n } == { answer: FormRejected [{ field: "", message: "That slug is already taken." }], posts: 1 })
}

test "toggling published through the admin" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi", published: False, author: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Posts {})
  let r <- Test.click (TogglePublished { id: id })
  let after <- Db.get Post id
  Task.succeed ({ answer: r.answer, published: Maybe.map (\t -> t.published) after } == { answer: Rerendered, published: Just True })
}

test "toggling another author's post is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi", published: False, author: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Admin.Posts {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (TogglePublished { id: id })
  let after <- Db.get Post id
  Task.succeed ({ answer: r.answer, published: Maybe.map (\t -> t.published) after } == { answer: PageMissing, published: Just False })
}
