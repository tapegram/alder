--- file: app/App.alder ---
module App

import Layout
import Table.User

session { user: Maybe User.UserId }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me") })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] }
  )
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

--- file: app/Table/User.alder ---
module Table.User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}

--- file: app/Table/Post.alder ---
module Table.Post

import Table.User

type PostId = brand Text

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

-- `author` is the table's `@owner`: the admin reads and writes only the signed-in author's posts.
table Post {
  id: PostId @id,
  title: Text.NonEmpty,
  slug: Slug @unique,
  body: Text.NonEmpty,
  published: Bool = False,
  author: User.UserId @ref User @owner,
  created: Time @created,
}

-- A published post is anyone's to read; a draft only its author's.
visibleTo : Maybe User.UserId -> Post -> Bool
visibleTo user post = post.published || user == Just post.author

test "a draft is visible only to its author" =
  {
    reader: visibleTo Nothing {
      id: PostId "p",
      title: Text.NonEmpty "Draft",
      slug: Slug "draft",
      body: Text.NonEmpty "Not yet.",
      published: False,
      author: User.UserId "u",
      created: Time.epoch,
    },
    author: visibleTo (Just (User.UserId "u")) {
      id: PostId "p",
      title: Text.NonEmpty "Draft",
      slug: Slug "draft",
      body: Text.NonEmpty "Not yet.",
      published: False,
      author: User.UserId "u",
      created: Time.epoch,
    },
  } == { reader: False, author: True }

--- file: app/Table/Comment.alder ---
module Table.Comment

import Table.Post

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
fake Password {
  verify args = {
    let found <-
      Db.from User
        |> Db.where (User.email.eq args.email)
        |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Desk.Desk {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
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
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Desk.Desk {}), user: Just me }
  )
}

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let visitor <- Test.open (Desk.Desk {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Desk.Desk {})
  let out <- Test.click App.Logout
  Task.succeed (
    {
      visitor: visitor.answer,
      out: out.answer,
      user: out.session.user,
    } == {
      visitor: Redirected (Login {}),
      out: Redirected (Login {}),
      user: Nothing,
    }
  )
}

--- file: app/Posts/Index.alder ---
module Posts.Index

import Layout
import Table.Post
import Table.User

route Index = GET "/" { }

type Model = { posts: List Post }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let posts <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Blog">
    <h1>Blog</h1>
    <ul id="posts">
      {for post in model.posts ->
        <li><a href={Posts.Show.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
    <p><a href={Desk.Desk {}}>Admin</a></p>
  </Layout.Page>

test "/ lists only published posts, newest first" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "First",
    slug: Post.Slug "first",
    body: Text.NonEmpty "One.",
    published: True,
    author: me,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Hidden",
    slug: Post.Slug "hidden",
    body: Text.NonEmpty "Draft.",
    published: False,
    author: me,
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Second",
    slug: Post.Slug "second",
    body: Text.NonEmpty "Two.",
    published: True,
    author: me,
  }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#posts li a" page.doc == ["Second", "First"])
}

--- file: app/Posts/Show.alder ---
module Posts.Show

import Layout
import Table.Comment
import Table.Post
import Table.User

route Show       = GET  "/posts/{slug}"          { slug: Post.Slug }
route AddComment = POST "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

type Model = { post: Post, comments: List Comment }

form NewComment = { author: Text.NonEmpty, body: Text.NonEmpty }

-- A draft is a 404 for everyone but its author, signed in.
Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <-
    Db.from Post
      |> Db.where (Post.slug.eq params.slug)
      |> Db.first
  match found {
    Just post -> if Post.visibleTo session.user post then withComments post else Query.notFound,
    Nothing -> Query.notFound,
  }
}

withComments : Post -> Query Model
withComments post = {
  let comments <-
    Db.from Comment
      |> Db.where (Comment.post.eq post.id)
      |> Db.orderBy Comment.created.asc
      |> Db.all
  Query.succeed { post: post, comments: comments }
}

-- No `load` runs before a write: the post is looked up again, and a draft is a 404 here too.
AddComment.update : AddComment.Params -> Session -> Task Response
AddComment.update p session = {
  let found <-
    Db.from Post
      |> Db.where (Post.slug.eq p.slug)
      |> Db.first
  match found {
    Just post -> if Post.visibleTo session.user post then addTo post p.form else Task.notFound,
    Nothing -> Task.notFound,
  }
}

addTo : Post -> NewComment -> Task Response
addTo post f = {
  let _ <- Db.insert Comment { post: post.id, author: f.author, body: f.body }
  Task.succeed (Response.redirect (Show { slug: post.slug }))
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.post.title}>
    <article>
      <h1>{model.post.title}</h1>
      {if model.post.published then Html.none else <p class="draft">Draft</p>}
      <p>{model.post.body}</p>
    </article>
    <section>
      <h2>Comments</h2>
      <ul id="comments">
        {for c in model.comments -> <li><strong>{c.author}</strong>: {c.body}</li>}
      </ul>
      <form submit={AddComment { slug: model.post.slug }}>
        <Errors form />
        <Input field="author" label="Name" />
        <Errors field="author" />
        <Textarea field="body" label="Comment" />
        <Errors field="body" />
        <button>Add comment</button>
      </form>
    </section>
    <p><a href={Posts.Index.Index {}}>All posts</a></p>
  </Layout.Page>

test "an unpublished post is not found for a reader" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: Text.NonEmpty "Not yet.",
    published: False,
    author: me,
  }
  let reader <- Test.open (Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just other }
  let stranger <- Test.open (Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just me }
  let author <- Test.open (Show { slug: Post.Slug "draft" })
  Task.succeed (
    {
      reader: reader.answer,
      stranger: stranger.answer,
      author: author.answer,
    } == {
      reader: PageMissing,
      stranger: PageMissing,
      author: Shown,
    }
  )
}

test "a comment with an empty author is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: Text.NonEmpty "Hi there.",
    published: True,
    author: me,
  }
  let _ <- Test.get (Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw (AddComment { slug: Post.Slug "hello" }) [
    { name: "author", value: "" },
    { name: "body", value: "Nice post" },
  ]
  let n <- Db.from Comment |> Db.count
  Task.succeed (
    {
      status: r.status,
      error: Test.text "[data-errors=author] li" r.doc,
      comments: n,
    } == {
      status: 422,
      error: Just "must not be empty",
      comments: 0,
    }
  )
}

test "adding a comment" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Post {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: Text.NonEmpty "Hi there.",
    published: True,
    author: me,
  }
  let _ <- Test.get (Show { slug: Post.Slug "hello" })
  let r <- Test.submit (AddComment { slug: Post.Slug "hello" }) {
    author: Text.NonEmpty "Ann",
    body: Text.NonEmpty "Nice post",
  }
  Task.succeed (
    { answer: r.answer, comments: Test.texts "#comments li" r.doc }
      == { answer: Redirected (Show { slug: Post.Slug "hello" }), comments: ["Ann: Nice post"] }
  )
}

--- file: app/Desk.alder ---
module Desk

import Layout
import Table.Post
import Table.User

route Desk   = GET   "/admin"             { }                    requires user
route Create = POST  "/admin/posts"       { } form NewPost       requires user
route Toggle = PATCH "/admin/posts/{id}"  { id: Post.PostId }    requires user

type Model = { posts: List Post }

form NewPost = {
  title: Text.NonEmpty,
  slug: Post.Slug,
  body: Text.NonEmpty,
  published: Bool = False,
}

Desk.load : Desk.Params -> Session -> Query Model
Desk.load params session = {
  let posts <-
    Db.mine Post
      |> Db.orderBy Post.created.desc
      |> Db.all
  Query.succeed { posts: posts }
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let taken <-
    Db.from Post
      |> Db.where (Post.slug.eq p.form.slug)
      |> Db.count
  if taken > 0 then
    Task.succeed (Response.invalid "A post with that slug already exists.")
  else {
    let _ <- Db.insertMine Post {
      title: p.form.title,
      slug: p.form.slug,
      body: p.form.body,
      published: p.form.published,
    }
    Task.succeed (Response.redirect (Desk {}))
  }
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- Db.updateMine Post p.id (\x -> { x | published: not x.published })
  Task.succeed Response.refresh
}

Desk.view : Model -> Session -> Html
Desk.view model session =
  <Layout.Page title="Admin">
    <h1>Posts</h1>
    <ul id="posts">
      {for post in model.posts ->
        <li id={"post-" ++ post.id}>
          <a href={Posts.Show.Show { slug: post.slug }}>{post.title}</a>
          <span class="state">{if post.published then "published" else "draft"}</span>
          <button click={Toggle { id: post.id }}>
            {if post.published then "Unpublish" else "Publish"}
          </button>
        </li>}
    </ul>
    <h2>New post</h2>
    <form submit={Create}>
      <Errors form />
      <Input field="title" label="Title" />
      <Input field="slug" label="Slug" />
      <Textarea field="body" label="Body" />
      <Input field="published" label="Published" />
      <button>Create</button>
    </form>
    <p><a href={Posts.Index.Index {}}>View blog</a></p>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "toggling publishes a draft" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let id <- Db.insert Post {
    title: Text.NonEmpty "Draft",
    slug: Post.Slug "draft",
    body: Text.NonEmpty "Not yet.",
    published: False,
    author: me,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Desk {})
  let r <- Test.click (Toggle { id: id })
  let after <- Db.get Post id
  Task.succeed (
    { answer: r.answer, published: Maybe.map (\x -> x.published) after }
      == { answer: Refreshed, published: Just True }
  )
}

test "creating a post through the admin" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Desk {})
  let r <- Test.submit (Create {}) {
    title: Text.NonEmpty "Hello",
    slug: Post.Slug "hello",
    body: Text.NonEmpty "Hi there.",
    published: True,
  }
  let page <- Test.get (Desk {})
  Task.succeed (
    { answer: r.answer, titles: Test.texts "#posts li a" page.doc }
      == { answer: Redirected (Desk {}), titles: ["Hello"] }
  )
}
