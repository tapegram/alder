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

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <nav>
        <a href={Home {}}>Blog</a>
        <a href={Admin {}}>Admin</a>
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

-- A post is visible to everyone once published, and always to its author.
visibleTo : Maybe User.UserId -> Post -> Bool
visibleTo viewer post = post.published || viewer == Just post.author

-- True when each post is at least as new as the one after it.
isNewestFirst : List Post -> Bool
isNewestFirst posts = match posts {
  [] -> True,
  [x] -> True,
  [x, ..rest] -> match List.head rest {
    Just y -> x.created >= y.created && isNewestFirst rest,
    Nothing -> True,
  },
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

-- The reason a comment is refused, if it is. The form decoder already refuses empty
-- fields; this keeps the rule in one tested place that `update` checks again.
problem : Text -> Text -> Maybe Text
problem author body =
  if Text.isEmpty (Text.trim author) then Just "Author is required."
  else if Text.isEmpty (Text.trim body) then Just "Comment is required."
  else Nothing

test "a comment with an empty author is refused" =
  problem "" "Nice post" == Just "Author is required."

test "a comment with a blank author is refused" =
  problem "   " "Nice post" == Just "Author is required."

test "a comment with an empty body is refused" =
  problem "Ann" "" == Just "Comment is required."

test "a complete comment is accepted" =
  problem "Ann" "Nice post" == Nothing

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
    {if model.posts == [] then <p>No posts yet.</p> else <ul id="posts">{for post in model.posts -> <Item post={post} />}</ul>}
    {match session.user {
      Just u -> <a href={Admin {}}>Admin</a>,
      Nothing -> <a href={Auth.Login {}}>Log in</a>,
    }}
  </Layout.Page>

Item : { post: Post } -> Html Msg
Item props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span class="date">{Time.format "Jan 2, 2006" props.post.created}</span>
  </li>

test "lists only published posts, newest first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "A", slug: Post.Slug "a", body: Text.NonEmpty "x", published: True, author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: Text.NonEmpty "x", author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "C", slug: Post.Slug "c", body: Text.NonEmpty "x", published: True, author: me }
  let m <- load { } { user: Nothing }
  let titles = m.posts |> List.map (\p -> Text.trim p.title) |> List.sortByText (\t -> t)
  Task.succeed ({ titles: titles, ordered: Post.isNewestFirst m.posts } == { titles: ["A", "C"], ordered: True })
}

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import Comment
import User
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post -> {
      if Post.visibleTo session.user post then {
        let comments <-
          Db.from Comment
            |> Db.where (Comment.post.eq post.id)
            |> Db.orderBy Comment.created.asc
            |> Db.all
        Query.succeed { post: post, comments: comments }
      } else Query.notFound
    },
    Nothing -> Query.notFound,
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
    match Comment.problem p.form.author p.form.body {
      Just reason -> Task.succeed (Reject reason),
      Nothing -> {
        let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
        Task.succeed Rerender
      },
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title}>
    <article>
      <h1>{model.post.title}</h1>
      {if model.post.published then Html.none else <p class="draft">Draft: only you can see this post.</p>}
      <p class="meta">{Time.format "Jan 2, 2006" model.post.created}</p>
      <div class="body">{model.post.body}</div>
    </article>
    <section id="comments">
      <h2>Comments</h2>
      <ul>
        {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
      </ul>
      <form submit={AddComment}>
        <Errors form />
        <Input field="author" label="Your name" />
        <Errors field="author" />
        <Textarea field="body" label="Comment" />
        <Errors field="body" />
        <button>Add comment</button>
      </form>
    </section>
    <a href={Home {}}>Back</a>
  </Layout.Page>

test "an unpublished post is not found for a reader" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Post { title: Text.NonEmpty "Draft", slug: Post.Slug "draft", body: Text.NonEmpty "x", author: me }
  let _ <- Db.insert Post { title: Text.NonEmpty "Live", slug: Post.Slug "live", body: Text.NonEmpty "x", published: True, author: me }
  let reader <- Test.run {
    let m <- load { slug: Post.Slug "draft" } { user: Nothing }
    Task.succeed m
  }
  let author <- Test.run {
    let m <- load { slug: Post.Slug "draft" } { user: Just me }
    Task.succeed m
  }
  let live <- Test.run {
    let m <- load { slug: Post.Slug "live" } { user: Nothing }
    Task.succeed m
  }
  Task.succeed ({
    draftForReader: Maybe.isJust (Result.toMaybe reader.value),
    draftForAuthor: Maybe.isJust (Result.toMaybe author.value),
    liveForReader: Maybe.isJust (Result.toMaybe live.value),
  } == { draftForReader: False, draftForAuthor: True, liveForReader: True })
}

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
  published: Bool,
}

msg Create form NewPost
msg Toggle { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let taken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.count
    if taken > 0 then Task.succeed (Reject "That slug is already used.") else {
      let _ <- Db.insert Post {
        title: p.form.title,
        slug: p.form.slug,
        body: p.form.body,
        published: p.form.published,
        author: session.user,
      }
      Task.succeed (Redirect (Admin {}))
    }
  },
  Toggle p -> {
    let post <- owned p.id session
    let _ <- Db.update Post post.id (\x -> { x | published: not x.published })
    Task.succeed Rerender
  },
}

-- The id in a message comes from the request, so check it again here.
owned : Post.PostId -> Session -> Task Post
owned id session = {
  let found <- Db.get Post id
  match found {
    Just post -> if post.author == session.user then Task.succeed post else Task.notFound,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin">
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
      <button>Create</button>
    </form>
    <ul id="posts">
      {for post in model.posts -> <Row post={post} />}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

Row : { post: Post } -> Html Msg
Row props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span class="status">{if props.post.published then "Published" else "Draft"}</span>
    <button click={Toggle { id: props.post.id }}>{if props.post.published then "Unpublish" else "Publish"}</button>
  </li>
