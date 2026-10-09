--- file: app/App.alder ---
```
module App

import Author
import Layout

session { author: Maybe Author.AuthorId }

login = Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | author: Nothing })
    Task.succeed (Redirect (Home {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" author={Nothing}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" author={Nothing}>
    <h1>Not found</h1>
  </Layout.Page>
```

--- file: app/Layout.alder ---
```
module Layout

import Author

Page : { title: Text, author: Maybe Author.AuthorId, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <header>
        <a href={Home {}}>Blog</a>
        {match props.author {
          Just _ -> <nav class="account"><a href={Admin {}}>Admin</a> <button click={App.Logout}>Log out</button></nav>,
          Nothing -> <nav class="account"><a href={Login {}}>Log in</a></nav>,
        }}
      </header>
      <main>{props.children}</main>
    </body>
  </html>
```

--- file: app/Author.alder ---
```
module Author

type AuthorId = brand Text

table Author {
  id: AuthorId @id,
  email: Text.Email @unique,
  password: Text.Secret,
  created: Time @created,
}
```

--- file: app/Post.alder ---
```
module Post

import Author

type PostId = brand Text

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text,
  published: Bool = False,
  author: Author.AuthorId @ref Author,
  created: Time @created,
}

test "a slug is lowercase words joined by hyphens" =
  Text.matches "^[a-z0-9-]+$" "my-first-post"
```

--- file: app/Comment.alder ---
```
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
```
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
  <Layout.Page title="Blog" author={session.author}>
    <h1>Blog</h1>
    {if List.isEmpty model.posts then <p>No posts yet.</p> else <ul id="posts">
      {for post in model.posts ->
        <li>
          <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
          <time>{Time.format "Jan 2, 2006" post.created}</time>
        </li>}
    </ul>}
  </Layout.Page>
```

--- file: app/Login.alder ---
```
module Login

import Author
import Layout

-- Authors are rows of the Author table, added outside the app.
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
    let found <- Db.first (Db.from Author |> Db.where (Author.email.eq p.form.email))
    match found {
      Just author -> if author.password == p.form.password then signIn author.id else Task.succeed (Reject "Email or password is incorrect."),
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

signIn : Author.AuthorId -> Task Nav
signIn id = {
  let _ <- Session.put (\s -> { s | author: Just id })
  Task.succeed (Redirect (Home {}))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" author={session.author}>
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

--- file: app/Admin.alder ---
```
module Admin

import Post
import Layout

page "/admin" { } requires author

type Model = { posts: List Post }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.all
  Query.succeed { posts: posts }
}

form NewPost = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text,
}

msg CreatePost form NewPost
msg TogglePublished { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  CreatePost p -> {
    let taken <- Db.first (Db.from Post |> Db.where (Post.slug.eq p.form.slug))
    match taken {
      Just _ -> Task.succeed (Reject "Another post already uses that slug."),
      Nothing -> {
        let _ <- Db.insert Post { title: p.form.title, slug: p.form.slug, body: p.form.body, author: session.author }
        Task.succeed (Redirect (Admin {}))
      },
    }
  },
  TogglePublished p -> {
    let post <- postById p.id
    let _ <- Db.update Post post.id (\t -> { t | published: not t.published })
    Task.succeed Rerender
  },
}

postById : Post.PostId -> Task Post
postById id = {
  let found <- Db.get Post id
  match found {
    Just post -> Task.succeed post,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Admin" author={Just session.author}>
    <h1>Admin</h1>
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
      {for post in model.posts ->
        <li>
          <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
          {if post.published then "(published)" else "(draft)"}
          <button click={TogglePublished { id: post.id }}>{if post.published then "Unpublish" else "Publish"}</button>
        </li>}
    </ul>
  </Layout.Page>
```

--- file: app/Posts/Show.alder ---
```
module Posts.Show

import Post
import Comment
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = { post: Post, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.first (Db.from Post |> Db.where (Post.slug.eq params.slug))
  match found {
    Just post -> if post.published || session.author != Nothing then commentsOf post else Query.notFound,
    Nothing -> Query.notFound,
  }
}

commentsOf : Post -> Query Model
commentsOf post = {
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
    Task.succeed (Redirect (Posts.Show { slug: model.post.slug }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.post.title} author={session.author}>
    <article>
      <h1>{model.post.title}</h1>
      <p class="meta">{Time.format "Jan 2, 2006" model.post.created}</p>
      {if model.post.published then Html.none else <p><em>Draft: only logged-in authors can see this.</em></p>}
      <div style="white-space: pre-wrap">{model.post.body}</div>
    </article>
    <section>
      <h2>Comments</h2>
      {if List.isEmpty model.comments then <p>No comments yet.</p> else <ul id="comments">
        {for c in model.comments ->
          <li><strong>{c.author}</strong> {c.body}</li>}
      </ul>}
      <h2>Add a comment</h2>
      <form submit={AddComment}>
        <Errors form />
        <Input field="author" label="Your name" />
        <Errors field="author" />
        <Textarea field="body" label="Comment" />
        <Errors field="body" />
        <button>Post comment</button>
      </form>
    </section>
    <a href={Home {}}>All posts</a>
  </Layout.Page>
```
