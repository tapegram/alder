--- file: app/App.alder ---
```alder
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
  <Layout.Page title="Error" loggedIn={False}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" loggedIn={False}>
    <h1>Not found</h1>
  </Layout.Page>
```

--- file: app/Layout.alder ---
```alder
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
        <a href={Home {}}>Blog</a>
        {if props.loggedIn then <a href={Admin {}}>Admin</a> else <a href={Auth.Login {}}>Log in</a>}
        {if props.loggedIn then <button click={App.Logout}>Log out</button> else Html.none}
      </nav>
      <main>{props.children}</main>
    </body>
  </html>
```

--- file: app/User.alder ---
```alder
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}
```

--- file: app/Post.alder ---
```alder
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

-- Unpublished posts are visible only to their author.
canView : Post -> Maybe User.UserId -> Bool
canView post viewer = post.published || viewer == Just post.author

test "unpublished posts are hidden from visitors" = {
  let post = { id: PostId "a", title: Text.NonEmpty "x", slug: Slug "x", body: Text.NonEmpty "x", published: False, author: User.UserId "u", created: Time.epoch }
  not (canView post Nothing)
}
```

--- file: app/Comment.alder ---
```alder
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
```

--- file: app/Home.alder ---
```alder
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
  <Layout.Page title="Blog" loggedIn={Maybe.isJust session.user}>
    <h1>Blog</h1>
    <ul id="posts">
      {for post in model.posts -> <PostItem post={post} />}
    </ul>
  </Layout.Page>

PostItem : { post: Post } -> Html Msg
PostItem props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
  </li>
```

--- file: app/Posts/Show.alder ---
```alder
module Posts.Show

import Post
import Comment
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = {
  post: Post,
  comments: List Comment,
}

load : Params -> Session -> Query Model
load params session = {
  let found <-
    Db.from Post
      |> Db.where (Post.slug.eq params.slug)
      |> Db.first
  match found {
    Nothing -> Query.notFound,
    Just post -> {
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      if Post.canView post session.user then Query.succeed { post: post, comments: comments } else Query.notFound
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
  <Layout.Page title={model.post.title} loggedIn={Maybe.isJust session.user}>
    <article>
      <h1>{model.post.title}</h1>
      <p>{model.post.body}</p>
    </article>

    <section id="comments">
      <h2>Comments</h2>
      <ul>
        {for c in model.comments -> <CommentItem comment={c} />}
      </ul>
    </section>

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

CommentItem : { comment: Comment } -> Html Msg
CommentItem props =
  <li>
    <strong>{props.comment.author}</strong>
    <p>{props.comment.body}</p>
  </li>
```

--- file: app/Admin.alder ---
```alder
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
  body: Text.NonEmpty,
}

msg CreatePost form NewPost
msg TogglePublished { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  CreatePost p -> {
    let existing <-
      Db.from Post
        |> Db.where (Post.slug.eq p.form.slug)
        |> Db.first
    match existing {
      Just _ -> Task.succeed (Reject "That slug is already in use."),
      Nothing -> {
        let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.user }
        Task.succeed Rerender
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
    <h1>Posts</h1>

    <form submit={CreatePost}>
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
      {for post in model.posts -> <AdminRow post={post} />}
    </ul>
  </Layout.Page>

AdminRow : { post: Post } -> Html Msg
AdminRow props =
  <li>
    <a href={Posts.Show { slug: props.post.slug }}>{props.post.title}</a>
    <span>{if props.post.published then "Published" else "Draft"}</span>
    <button click={TogglePublished { id: props.post.id }}>{if props.post.published then "Unpublish" else "Publish"}</button>
  </li>
```

--- file: app/Auth/Login.alder ---
```alder
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
  <Layout.Page title="Log in" loggedIn={False}>
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
```
