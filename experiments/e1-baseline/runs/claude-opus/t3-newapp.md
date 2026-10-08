--- file: app/App.alder ---
module App

import User
import Layout
import Auth.Login

session { user: Maybe User.UserId }

login = Auth.Login

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
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
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

-- A post is visible when it is published, or when the viewer is its author.
visibleTo : Maybe User.UserId -> Post -> Bool
visibleTo viewer post = post.published || viewer == Just (post.author)

-- Split a body into paragraphs on blank lines, dropping empty ones.
paragraphs : Text -> List Text
paragraphs body =
  Text.split "\n\n" body
    |> List.map Text.trim
    |> List.filter (\p -> not (Text.isEmpty p))

statusLabel : Post -> Text
statusLabel post = if post.published then "Published" else "Draft"

test "paragraphs splits on blank lines and drops empties" =
  paragraphs "First.\n\nSecond.\n\n\n\n" == ["First.", "Second."]

test "unpublished post is hidden from anonymous viewers" =
  not (visibleTo Nothing { id: PostId "p", title: "x", slug: Slug "x", body: "b", published: False, author: User.UserId "u", created: Time.epoch })

test "unpublished post is visible to its author" =
  visibleTo (Just (User.UserId "u")) { id: PostId "p", title: "x", slug: Slug "x", body: "b", published: False, author: User.UserId "u", created: Time.epoch }

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
import Admin.Index

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

msg Submit = "/login" form Credentials
msg Logout = "/logout" { }

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
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

view : Model -> Html Msg
view model =
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

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import Layout
import Posts.Show

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

view : Model -> Html Never
view model =
  <Layout.Page title="Blog">
    <h1>Blog</h1>
    {if model.posts == []
      then <p>No posts yet.</p>
      else
        <ul id="posts">
          {for post in model.posts -> <Item post={post} />}
        </ul>}
  </Layout.Page>

Item : { post: Post } -> Html Never
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
import Posts.Index

page "/posts/{slug}" { slug: Post.Slug }

type Model = {
  post: Post,
  comments: List Comment,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first
  match found {
    Just post ->
      if Post.visibleTo session.user post then {
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

msg AddComment = "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { post: model.post.id, author: p.form.author, body: p.form.body }
    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.post.title}>
    <article>
      <h1>{model.post.title}</h1>
      <p class="date">
        {Time.format "Jan 2, 2006" model.post.created}
        {if model.post.published then Html.none else <span class="draft"> (draft)</span>}
      </p>
      {for para in Post.paragraphs model.post.body -> <p>{para}</p>}
    </article>

    <section id="comments">
      <h2>Comments ({Text.fromInt (List.length model.comments)})</h2>
      {if model.comments == []
        then <p>No comments yet.</p>
        else
          <ul>
            {for comment in model.comments -> <CommentItem comment={comment} />}
          </ul>}
    </section>

    <h2>Add a comment</h2>
    <form submit={AddComment { slug: model.post.slug }}>
      <Errors form />
      <Input field="author" label="Name" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Post comment</button>
    </form>

    <a href={Posts.Index {}}>Back to all posts</a>
  </Layout.Page>

CommentItem : { comment: Comment } -> Html Msg
CommentItem props =
  <li>
    <p class="author">{props.comment.author}</p>
    <p class="date">{Time.format "Jan 2, 2006" props.comment.created}</p>
    <p>{props.comment.body}</p>
  </li>

--- file: app/Admin/Index.alder ---
module Admin.Index

import Post
import User
import Layout
import Posts.Show

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

msg Create = "/admin/posts" form NewPost
msg TogglePublished = "/admin/posts/{id}/toggle" { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let existing <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.first
    match existing {
      Just _ -> Task.succeed (Reject "A post with that slug already exists."),
      Nothing -> {
        let _ <- Db.insert Post {
          title: p.form.title,
          slug: p.form.slug,
          body: p.form.body,
          published: p.form.published,
          author: session.user,
        }
        Task.succeed (Redirect (Admin.Index {}))
      },
    }
  },
  TogglePublished p -> {
    let _ <- Db.update Post p.id (\post -> { post | published: not post.published })
    Task.succeed Rerender
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Admin">
    <h1>Admin</h1>

    <h2>New post</h2>
    <form submit={Create}>
      <Errors form />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug (lowercase letters, digits, hyphens)" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <Input field="published" label="Publish now" />
      <Errors field="published" />
      <button>Create post</button>
    </form>

    <h2>All posts</h2>
    <table id="posts">
      <thead>
        <tr>
          <th>Title</th>
          <th>Created</th>
          <th>Status</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {for post in model.posts -> <Row post={post} />}
      </tbody>
    </table>
  </Layout.Page>

Row : { post: Post } -> Html Msg
Row props =
  <tr class={if props.post.published then "published" else "draft"}>
    <td><a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a></td>
    <td>{Time.format "Jan 2, 2006" props.post.created}</td>
    <td>{Post.statusLabel props.post}</td>
    <td>
      <button click={TogglePublished { id: props.post.id }}>
        {if props.post.published then "Unpublish" else "Publish"}
      </button>
    </td>
  </tr>
