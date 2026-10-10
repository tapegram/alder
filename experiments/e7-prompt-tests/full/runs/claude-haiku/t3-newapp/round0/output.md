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
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/User.alder ---
module User

type UserId = brand Text

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
  author: User.UserId,
  created: Time @created,
}

test "slugs have no capitals" = not (Text.matches "^[a-z0-9-]+$" "Hello")

--- file: app/Comment.alder ---
module Comment

import Post

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post,
  author: Text,
  body: Text,
  created: Time @created,
}

-- complete is true when the author and the comment both have text once trimmed.
complete : Text -> Text -> Bool
complete author body =
  not (Text.isEmpty (Text.trim author)) && not (Text.isEmpty (Text.trim body))

add : Post.PostId -> Text -> Text -> Task Unit
add post author body = {
  let _ <- Db.insert Comment { post: post, author: Text.trim author, body: Text.trim body }
  Task.succeed ()
}

test "a comment with an empty author is refused" = not (complete "   " "a comment")

test "a comment with an empty body is refused" = not (complete "Ann" "  ")

--- file: app/Home.alder ---
module Home

import Post
import Layout
import User

page "/" { }

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.where (Post.published.eq True) |> Db.all
  Query.succeed { posts: listing posts }
}

-- listing keeps the published posts, newest first.
listing : List Post -> List Post
listing posts =
  posts
    |> List.filter (\p -> p.published)
    |> List.sortByText (\p -> Time.format "2006-01-02T15:04:05.000000000" p.created)
    |> List.reverse

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Blog">
    <h1>Blog</h1>
    <ul id="posts">
      {for p in model.posts ->
        <li><a href={Posts.Show { slug: p.slug }}>{p.title}</a></li>}
    </ul>
  </Layout.Page>

test "listing shows only published posts, newest first" = {
  let now <- Time.now
  let old : Post = { id: Post.PostId "a", slug: Post.Slug "old", title: Text.NonEmpty "Old", body: "", published: True, author: User.UserId "admin", created: Time.epoch }
  let draft : Post = { id: Post.PostId "b", slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", published: False, author: User.UserId "admin", created: now }
  let recent : Post = { id: Post.PostId "c", slug: Post.Slug "recent", title: Text.NonEmpty "Recent", body: "", published: True, author: User.UserId "admin", created: now }
  Task.succeed (listing [old, draft, recent] == [recent, old])
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
  let found <- Db.first (Db.from Post |> Db.where (Post.slug.eq params.slug))
  match found {
    Nothing -> Query.notFound,
    Just post -> if canSee post session then showPost post else Query.notFound,
  }
}

-- canSee: published posts are for everyone; a draft only for its author.
canSee : Post -> Session -> Bool
canSee post session = post.published || session.user == Just post.author

showPost : Post -> Query Model
showPost post = {
  let comments <-
    Db.from Comment
      |> Db.where (Comment.post.eq post.id)
      |> Db.orderBy Comment.created.asc
      |> Db.all
  Query.succeed { post: post, comments: comments }
}

form NewComment = {
  author: Text,
  body: Text,
}

msg AddComment form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddComment p -> {
    let ok = Comment.complete p.form.author p.form.body
    if ok then {
      let _ <- Comment.add model.post.id p.form.author p.form.body
      Task.succeed Rerender
    } else Task.succeed (Reject "Please give your name and a comment."),
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title}>
    <article>
      <h1>{model.post.title}</h1>
      {if model.post.published then Html.none else <p class="draft">Draft: only you can see this.</p>}
      <p>{model.post.body}</p>
    </article>

    <h2>Comments</h2>
    <ul id="comments">
      {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
    </ul>

    <form submit={AddComment}>
      <Errors form />
      <Input field="author" label="Your name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>

    <a href={Home {}}>All posts</a>
  </Layout.Page>

test "an unpublished post is not found for a reader" = {
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", published: False, author: User.UserId "admin" }
  let r <- Test.run {
    let m <- load { slug: Post.Slug "draft" } { user: Nothing }
    Task.succeed m
  }
  Task.succeed (r.value == Err "not found")
}

test "an unpublished post is shown to its author" = {
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", published: False, author: User.UserId "admin" }
  let r <- Test.run {
    let m <- load { slug: Post.Slug "draft" } { user: Just (User.UserId "admin") }
    Task.succeed m
  }
  Task.succeed (Maybe.isJust (Result.toMaybe r.value))
}

test "a comment with a blank author is refused" = {
  let id <- Db.insert Post { slug: Post.Slug "hi", title: Text.NonEmpty "Hi", body: "b", published: True, author: User.UserId "admin" }
  let post : Post = { id: id, slug: Post.Slug "hi", title: Text.NonEmpty "Hi", body: "b", published: True, author: User.UserId "admin", created: Time.epoch }
  let r <- Test.run (update (AddComment { form: { author: "  ", body: "hi" } }) { post: post, comments: [] } { user: Nothing })
  let n <- Db.count (Db.from Comment)
  Task.succeed ({ value: r.value, comments: n } == { value: Ok (Reject "Please give your name and a comment."), comments: 0 })
}

--- file: app/Admin.alder ---
module Admin

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
  body: Text,
}

msg Create form NewPost
msg Toggle { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let taken <- Db.first (Db.from Post |> Db.where (Post.slug.eq p.form.slug))
    match taken {
      Just _ -> Task.succeed (Reject "That slug is already used."),
      Nothing -> {
        let _ <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, published: False, author: session.user }
        Task.succeed Rerender
      },
    }
  },
  Toggle p -> {
    let post <- owned p.id session
    let _ <- Db.update Post post.id (\t -> { t | published: not t.published })
    Task.succeed Rerender
  },
}

-- owned is the post a message names, when the session's user wrote it; otherwise a 404.
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
    <h1>Posts</h1>

    <form submit={Create}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Create post</button>
    </form>

    <ul id="posts">
      {for p in model.posts -> <PostRow post={p} />}
    </ul>
  </Layout.Page>

PostRow : { post: Post } -> Html Msg
PostRow props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    {if props.post.published then " (published)" else " (draft)"}
    <button click={Toggle { id: props.post.id }}>{if props.post.published then "Unpublish" else "Publish"}</button>
  </li>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { password: Text.Secret } -> Task (Maybe User.UserId),
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  password: Text.Secret,
}

msg Submit form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Admin {}))
      },
      Nothing -> Task.succeed (Reject "The password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: adapters/password.go ---
package adapters

import (
	"context"
	"os"

	"alder/rt"
	"app"
)

// Caps are the implementations of the app's capabilities.
func Caps() app.Caps {
	return app.Caps{Auth__Login__Password: password{}}
}

// authorID is the id of the blog's single author. Posts are written as this user.
const authorID = "author"

// password checks the one shared password, read from BLOG_PASSWORD, and logs in the author.
type password struct{}

// Verify returns the author's id when the password matches BLOG_PASSWORD, and Nothing otherwise.
func (password) Verify(ctx context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	want := os.Getenv("BLOG_PASSWORD")
	if want == "" || args.Password != want {
		return rt.Nothing[string](), nil
	}
	return rt.Just(authorID), nil
}
