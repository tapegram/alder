--- file: app/Admin.alder ---
module Admin

import Post
import User
import Layout

page "/admin" { } requires user

type Model = {
  author: User,
  posts: List Post,
}

-- load lets only an Author in: a Reader who is logged in gets the same 404 as a page that does
-- not exist. Every message of the page runs after this `load`, so `update` needs no check of
-- its own that the user is an Author.
load : Params -> Session -> Query Model
load params session = {
  let me <- User.author session.user
  match me {
    Just user -> {
      let posts <-
        Db.from Post
          |> Db.where (Post.author.eq user.id)
          |> Db.orderBy Post.created.desc
          |> Db.all
      Query.succeed { author: user, posts: posts }
    },
    Nothing -> Query.notFound,
  }
}

form NewPost = {
  slug: Post.Slug,
  title: Text.NonEmpty,
  body: Text,
}

msg Create form NewPost
msg Publish { id: Post.PostId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  -- A new post is a draft: `published` is left at its default, False.
  Create p -> {
    let taken <- Post.bySlug p.form.slug
    match taken {
      Just _ -> Task.succeed (Reject "Another post has that slug."),
      Nothing -> {
        let id <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, author: model.author.id }
        Task.succeed (Redirect (Admin.Edit { id: id }))
      },
    }
  },
  Publish p -> {
    let post <- owned p.id model
    let _ <- Db.update Post post.id (\x -> { x | published: True })
    Task.succeed (Redirect (Admin {}))
  },
}

-- owned is the post a message names, when the page's author wrote it; otherwise the message is
-- a 404. The id comes from the request, so it is checked here, not trusted.
owned : Post.PostId -> Model -> Task Post
owned id model = {
  let found <- Db.get Post id
  match found {
    Just post -> if post.author == model.author.id then Task.succeed post else Task.notFound,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Your posts">
    <h1>Your posts</h1>

    <h2>Drafts</h2>
    <ul id="drafts">
      {for post in List.filter (\p -> not p.published) model.posts ->
        <li>
          <a class="title" href={Admin.Edit { id: post.id }}>{post.title}</a>
          <button click={Publish { id: post.id }}>Publish</button>
        </li>}
    </ul>

    <h2>Published</h2>
    <ul class="posts">
      {for post in List.filter (\p -> p.published) model.posts ->
        <li>
          <a href={Admin.Edit { id: post.id }}>{post.title}</a>
        </li>}
    </ul>

    <h2>New post</h2>
    <form submit={Create}>
      <Errors form />
      <Input field="slug" label="Slug (lowercase letters, digits and hyphens)" />
      <Errors field="slug" />
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Textarea field="body" label="Body (Markdown)" />
      <Errors field="body" />
      <button>Create post</button>
    </form>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a Reader cannot open the admin page" = {
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let asReader <- Test.run {
    let m <- load {} { user: reader }
    Task.succeed m.author.id
  }
  let asAuthor <- Test.run {
    let m <- load {} { user: author }
    Task.succeed m.author.id
  }
  Task.succeed ({ reader: asReader.value, author: asAuthor.value } == { reader: Err "not found", author: Ok author })
}

test "creating a post redirects to its edit page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let model <- load {} { user: author }
  let r <- Test.run (update (Create { form: { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi." } }) model { user: author })
  let created <- Post.bySlug (Post.Slug "hello")
  match created {
    Just post -> Task.succeed (r.value == Ok (Redirect (Admin.Edit { id: post.id }))),
    Nothing -> Task.fail "no post has the slug hello",
  }
}

test "a new post is a draft" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let model <- load {} { user: author }
  let _ <- Test.run (update (Create { form: { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi." } }) model { user: author })
  let created <- Post.bySlug (Post.Slug "hello")
  Task.succeed (Maybe.map (\p -> p.published) created == Just False)
}

test "a taken slug is refused" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let model <- load {} { user: author }
  let r <- Test.run (update (Create { form: { slug: Post.Slug "hello", title: Text.NonEmpty "Again", body: "" } }) model { user: author })
  let posts <- Db.from Post |> Db.count
  Task.succeed ({ value: r.value, posts: posts } == { value: Ok (Reject "Another post has that slug."), posts: 1 })
}

-- The admin page through the browser: a visitor is sent to log in; a Reader who is logged in
-- gets the 404; an Author gets the page, and can log out from it.
test "the admin page needs an Author" = {
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let visitor <- Test.open (Admin {})
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Admin {})
  Task.succeed ({ visitor: visitor.answer, reader: asReader.answer } == { visitor: Redirected (Auth.Login {}), reader: PageMissing })
}

test "an Author logs out from the admin page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Test.login { user: Just author }
  let page <- Test.get (Admin {})
  let out <- Test.click App.Logout
  Task.succeed ({ heading: Test.text "h1" page.doc, out: out.answer, user: out.session.user } == { heading: Just "Your posts", out: Redirected (Auth.Login {}), user: Nothing })
}

-- The dashboard lists the author's drafts only: not the published posts, not another author's.
test "the dashboard lists the author's drafts" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "mine", title: Text.NonEmpty "Mine", body: "", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "out", title: Text.NonEmpty "Out", body: "", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "theirs", title: Text.NonEmpty "Theirs", body: "", author: other }
  let _ <- Test.login { user: Just author }
  let page <- Test.get (Admin {})
  Task.succeed ({ drafts: Test.texts "#drafts .title" page.doc, published: Test.texts "ul.posts a" page.doc } == { drafts: ["Mine"], published: ["Out"] })
}

test "the newest draft is listed first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let page <- Test.get (Admin {})
  Task.succeed (Test.texts "#drafts .title" page.doc == ["Second", "First"])
}

test "publishing a draft puts it on the front page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let before <- Test.get (Posts.Index {})
  let admin <- Test.get (Admin {})
  let r <- Test.click (Publish { id: id })
  let front <- Test.get (Posts.Index {})
  Task.succeed ({ before: Test.texts "ul.posts a" before.doc, drafts: Test.texts "#drafts .title" admin.doc, answer: r.answer, remaining: Test.texts "#drafts .title" r.doc, front: Test.texts "ul.posts a" front.doc } == { before: [], drafts: ["Hello"], answer: Redirected (Admin {}), remaining: [], front: ["Hello"] })
}

test "Publish of another author's draft is not found" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let model <- load {} { user: other }
  let r <- Test.run (update (Publish { id: id }) model { user: other })
  let after <- Db.get Post id
  Task.succeed ({ value: r.value, published: Maybe.map (\p -> p.published) after } == { value: Err "not found", published: Just False })
}

--- file: app/Admin/Edit.alder ---
module Admin.Edit

import Post
import Comment
import User
import Layout

page "/admin/{id}" { id: Post.PostId } requires user

type Model = { post: Post }

-- load finds the post when the session's user wrote it and is still an Author (the same gate as
-- the Admin page: an author made a Reader can no longer edit their old posts); anyone else's
-- post, like a missing one, is a 404. The messages carry no id: they act on `model.post`, which
-- this `load` has authorized (as the to-do example's Todos.Show does).
load : Params -> Session -> Query Model
load params session = {
  let me <- User.author session.user
  let found <- Db.get Post params.id
  match found {
    Just post -> if me != Nothing && post.author == session.user then Query.succeed { post: post } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form EditPost = {
  title: Text.NonEmpty,
  body: Text,
}

msg Save form EditPost
msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Post model.post.id (\post -> { post | title: p.form.title, body: p.form.body })
    Task.succeed Rerender
  },
  -- The post's comments go with it: Comment.post is `@ref Post @cascade`.
  Delete p -> {
    let _ <- Db.delete Post model.post.id
    Task.succeed (Redirect (Admin {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={"Edit: " ++ model.post.title}>
    <h1>{model.post.title}</h1>
    <p><a href={Posts.Show { slug: model.post.slug }}>View</a></p>

    <form submit={Save}>
      <Errors form />
      <Input field="title" label="Title" value={model.post.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body (Markdown)" value={model.post.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>

    <button click={Delete}>Delete</button>
    <a href={Admin {}}>All your posts</a>
  </Layout.Page>

test "deleting a post deletes its comments" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Db.insert Comment { post: id, author: Comment.Name "Ann", body: Comment.Body "First!" }
  let _ <- Db.insert Comment { post: id, author: Comment.Name "Bob", body: Comment.Body "Second." }
  let model <- load { id: id } { user: author }
  let r <- Test.run (update Delete model { user: author })
  let posts <- Db.from Post |> Db.count
  let comments <- Db.from Comment |> Db.count
  Task.succeed ({ value: r.value, trace: r.trace, posts: posts, comments: comments } == { value: Ok (Redirect (Admin {})), trace: ["Db.delete Post"], posts: 0, comments: 0 })
}

test "another author's post is not found" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let r <- Test.run {
    let m <- load { id: id } { user: other }
    Task.succeed m.post.id
  }
  Task.succeed (r.value == Err "not found")
}

test "a demoted author cannot edit" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let before <- Test.run {
    let m <- load { id: id } { user: author }
    Task.succeed m.post.id
  }
  let _ <- Db.update User author (\u -> { u | role: Reader })
  let after <- Test.run {
    let m <- load { id: id } { user: author }
    Task.succeed m.post.id
  }
  Task.succeed ({ before: before.value, after: after.value } == { before: Ok id, after: Err "not found" })
}

-- The page through the browser: a visitor is sent to log in; the author saves the form.
test "a visitor is sent to log in from the edit page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let visitor <- Test.open (Admin.Edit { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "saving changes the title and the body" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Old.", author: author }
  let _ <- Test.login { user: Just author }
  let _ <- Test.get (Admin.Edit { id: id })
  let r <- Test.submit EditPost { title: Text.NonEmpty "Bread", body: "New." }
  let after <- Db.get Post id
  Task.succeed ({ answer: r.answer, title: Maybe.map (\p -> p.title) after, body: Maybe.map (\p -> p.body) after } == { answer: Rerendered, title: Just (Text.NonEmpty "Bread"), body: Just "New." })
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
    Task.succeed (Redirect (Auth.Login {}))
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

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}

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

-- The mutants of this `update` (the Session.put, the Redirect, the Reject) survive: every path
-- through `Submit` begins with `Password.verify`, so only a test that calls the Password
-- capability could tell them apart, and this app's tests do not call it.
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

--- file: app/Comment.alder ---
module Comment

import Post

type CommentId = brand Text

-- Anyone may comment, so a comment's name and text are bounded: a Name is 1 to 80 characters, a
-- Body 1 to 5,000, and neither is only spaces. (A brand's base is a built-in type, so each
-- checks for itself what Text.NonEmpty would.) The form refuses a longer one with an error
-- beside the field.
type Name = brand Text where (\s -> fits 80 s)
type Body = brand Text where (\s -> fits 5000 s)

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  author: Name,
  body: Body,
  created: Time @created,
}

-- fits is whether s has text other than spaces and at most n characters.
fits : Int -> Text -> Bool
fits n s = not (Text.isEmpty (Text.trim s)) && Text.length s <= n

test "a name or comment has text and a bound" = {
  let long = Text.join "" (List.map (\n -> "a") (List.range 1 81))
  Task.succeed (List.map (fits 80) ["Ann", "", "   ", Text.slice 0 80 long, long] == [True, False, False, True, False])
}

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
      <header>
        <a href={Posts.Index {}}>Blog</a>
      </header>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Markdown.alder ---
module Markdown

-- Rendering a post's body is outside the language: a capability implemented in Go
-- (adapters/markdown/markdown.go), which builds the HTML node by node, so a post cannot inject markup.
capability Markdown {
  render : Text -> Query (Html Never),
}

-- In a test, `let _ <- Test.fake Markdown` replaces the adapter with this: the body as plain
-- text, so a test about a page asserts on the page, not on how Markdown is rendered.
fake Markdown {
  render text = Query.succeed (Html.text text)
}

--- file: app/Post.alder ---
module Post

import User

type PostId = brand Text
type Slug = brand Text where (\s -> isSlug s)

-- A post is a draft until it is published: `published` starts False, and the dashboard's
-- "Publish" button sets it.
table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User,
  created: Time @created,
}

bySlug : Slug -> Query (Maybe Post)
bySlug slug = Db.from Post |> Db.where (Post.slug.eq slug) |> Db.first

-- isSlug is the rule a Slug keeps: lowercase letters, digits and hyphens, at least one.
isSlug : Text -> Bool
isSlug s = Text.matches "^[a-z0-9-]+$" s

test "a slug is lowercase letters, digits and hyphens" =
  List.map isSlug ["hello-world", "post-2", "Hello", "a b", "", "x/y"] == [True, True, False, False, False, False]

-- `author` is `@ref User` with the default rule, `@restrict`: a user who still has posts cannot
-- be deleted, and the posts stay.
test "an author with posts cannot be deleted" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "" }
  let id <- Db.insert Post { slug: Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let r <- Test.run (Db.delete User author)
  let after <- Db.get Post id
  Task.succeed ({ deleted: Result.toMaybe r.value, post: Maybe.map (\p -> p.title) after } == { deleted: Nothing, post: Just (Text.NonEmpty "Hello") })
}

--- file: app/Posts/Index.alder ---
module Posts.Index

import Post
import User
import Layout

page "/" { page: Int = 1, q: Text = "" }

type Model = {
  page: Int,
  q: Text,
  posts: List Post,
  hasMore: Bool,
  canWrite: Bool,
}

perPage : Int
perPage = 10

-- lastPage bounds `page`, so `(page - 1) * perPage` cannot overflow.
lastPage : Int
lastPage = 1000000

-- load reads one page of published posts, newest first, matching `q` when it is not blank; `q`
-- is cut to 100 characters. A draft is never listed, whoever asks. It asks for one post more
-- than a page holds: if that one comes back there is a next page (`hasMore`), and it is dropped
-- from the list. `canWrite` shows the link to the Admin page to an Author only.
--
-- Three mutants of this function are the same program, so no test can kill them: `< 1` as
-- `<= 1` and `> lastPage` as `>= lastPage` differ only at 1 and at lastPage, where both branches
-- give the same page; and without `Db.limit` the query reads more rows but `List.take perPage`
-- and `hasMore` come out the same.
load : Params -> Session -> Query Model
load params session = {
  let page = if params.page < 1 then 1 else if params.page > lastPage then lastPage else params.page
  let q = Text.slice 0 100 (Text.trim params.q)
  let matching =
    if Text.isEmpty q then Db.and [] else Db.or [Post.title.contains q, Post.body.contains q]
  let found <-
    Db.from Post
      |> Db.where (Post.published.eq True)
      |> Db.where matching
      |> Db.orderBy Post.created.desc
      |> Db.limit (perPage + 1)
      |> Db.offset ((page - 1) * perPage)
      |> Db.all
  let me <- match session.user {
    Just id -> User.author id,
    Nothing -> Query.succeed Nothing,
  }
  let canWrite = me != Nothing
  Query.succeed { page: page, q: q, posts: List.take perPage found, hasMore: List.length found > perPage, canWrite: canWrite }
}

form SearchForm = {
  q: Text,
}

-- A search is a message like any form: `update` answers with the first page of the results,
-- whose address (/?q=…) can be bookmarked and shared.
msg Search form SearchForm

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Search p -> Task.succeed (Redirect (Posts.Index { page: 1, q: p.form.q })),
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Blog">
    <h1>Blog</h1>

    <form submit={Search} class="search">
      <Input field="q" label="Search" value={model.q} />
      <Errors form />
      <button>Search</button>
    </form>

    {if List.isEmpty model.posts then
      <p class="empty">No posts{if Text.isEmpty model.q then "" else " match “" ++ model.q ++ "”"}.</p>
    else
      <ul class="posts">
        {for post in model.posts ->
          <li>
            <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
            <span class="date">{Time.format "Jan 2, 2006" post.created}</span>
          </li>}
      </ul>}

    <nav class="pages">
      {if model.page > 1 then <a href={Posts.Index { page: model.page - 1, q: model.q }} rel="prev">Newer</a> else Html.none}
      {if model.hasMore then <a href={Posts.Index { page: model.page + 1, q: model.q }} rel="next">Older</a> else Html.none}
    </nav>

    {match session.user {
      Just _ -> <p>{if model.canWrite then <a href={Admin {}}>Write</a> else Html.none} <button click={App.Logout}>Log out</button></p>,
      Nothing -> <p><a href={Auth.Login {}}>Log in</a></p>,
    }}
  </Layout.Page>

test "the front page lists the posts, newest first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", published: True, author: author }
  let model <- load { page: 1, q: "" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"])
}

test "search matches title or body" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "tomatoes", title: Text.NonEmpty "Tomato season", body: "It is here.", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", published: True, author: author }
  let model <- load { page: 1, q: "TOMATO" } { user: Nothing }
  let titles = List.map (\p -> p.title) model.posts
  Task.succeed ({ count: List.length titles, title: List.member (Text.NonEmpty "Tomato season") titles, body: List.member (Text.NonEmpty "Soup") titles } == { count: 2, title: True, body: True })
}

test "a `%` in the query is literal" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "done", title: Text.NonEmpty "100% done", body: "", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "ways", title: Text.NonEmpty "1000 ways", body: "", published: True, author: author }
  let model <- load { page: 1, q: "100%" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "100% done"])
}

test "page 99 is empty" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", published: True, author: author }) (List.range 1 11))
  let pages <- Query.all (List.map (\n -> load { page: n, q: "" } { user: Nothing }) [1, 2, 99, 0])
  let seen = List.map (\m -> { page: m.page, posts: List.length m.posts, hasMore: m.hasMore }) pages
  Task.succeed (seen == [{ page: 1, posts: 10, hasMore: True }, { page: 2, posts: 1, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 10, hasMore: True }])
}

-- Exactly one page of posts: all ten are listed, and there is no next page.
test "a full page has no next page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", published: True, author: author }) (List.range 1 10))
  let model <- load { page: 1, q: "" } { user: Nothing }
  Task.succeed ({ posts: List.length model.posts, hasMore: model.hasMore } == { posts: 10, hasMore: False })
}

test "a negative page is page 1" = {
  let model <- load { page: -3, q: "" } { user: Nothing }
  Task.succeed (model.page == 1)
}

test "a `_` in the query is literal" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "snake", title: Text.NonEmpty "snake_case", body: "", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "camel", title: Text.NonEmpty "snakeYcase", body: "", published: True, author: author }
  let model <- load { page: 1, q: "e_c" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "snake_case"])
}

-- The query is cut to its first 100 characters: SQLite refuses a pattern over ~50 KB, and below
-- that a search costs the length of every post times the length of the query.
test "a long query is cut" = {
  let long = Text.join "" (List.map (\n -> "tomato ") (List.range 1 100))
  let model <- load { page: 1, q: long } { user: Nothing }
  Task.succeed (model.q == Text.slice 0 100 long)
}

test "a search answers with the first page of its results" = {
  let model <- load { page: 3, q: "" } { user: Nothing }
  let nav <- update (Search { form: { q: "soup" } }) model { user: Nothing }
  Task.succeed (nav == Redirect (Posts.Index { page: 1, q: "soup" }))
}

test "only an Author sees the Write link" = {
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let pages <- Query.all (List.map (\user -> load { page: 1, q: "" } { user: user }) [Nothing, Just reader, Just author])
  Task.succeed (List.map (\m -> m.canWrite) pages == [False, False, True])
}

-- The same page through the browser: the links a page renders, and the search form posted as a
-- user posts it (book 10, "Testing through the browser").
test "the second page links back to the first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", published: True, author: author }) (List.range 1 11))
  let first <- Test.get (Posts.Index {})
  let second <- Test.get (Posts.Index { page: 2 })
  Task.succeed ({ firstPosts: Test.count "ul.posts li" first.doc, next: Test.attr "href" "a[rel=next]" first.doc, secondPosts: Test.count "ul.posts li" second.doc, prev: Test.attr "href" "a[rel=prev]" second.doc, more: Test.has "a[rel=next]" second.doc } == { firstPosts: 10, next: Just "/?page=2", secondPosts: 1, prev: Just "/", more: False })
}

test "a search through the form" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", published: True, author: author }
  let _ <- Test.get (Posts.Index {})
  let r <- Test.submit SearchForm { q: "tomato" }
  Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
}

-- The front page does not require a user, so a Rerender after Logout would stay on it: this
-- test tells the Redirect to the login page from a Rerender.
test "logging out from the front page goes to log in" = {
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Test.login { user: Just reader }
  let _ <- Test.get (Posts.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ answer: out.answer, user: out.session.user } == { answer: Redirected (Auth.Login {}), user: Nothing })
}

-- A draft is not listed and not found by a search, even for its own author.
test "a draft is not on the front page and not found by a search" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Tomato draft", body: "More tomatoes.", author: author }
  let front <- load { page: 1, q: "" } { user: Just author }
  let found <- load { page: 1, q: "tomato" } { user: Just author }
  Task.succeed ({ front: List.map (\p -> p.title) front.posts, found: List.map (\p -> p.title) found.posts } == { front: [Text.NonEmpty "Soup"], found: [Text.NonEmpty "Soup"] })
}

test "a draft is not on the page a visitor sees" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Tomato draft", body: "More tomatoes.", author: author }
  let front <- Test.get (Posts.Index {})
  let r <- Test.submit SearchForm { q: "draft" }
  Task.succeed ({ front: Test.texts "ul.posts a" front.doc, found: Test.texts "ul.posts a" r.doc, empty: Test.has "p.empty" r.doc } == { front: ["Soup"], found: [], empty: True })
}

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import User
import Comment
import Markdown
import Layout

page "/posts/{slug}" { slug: Post.Slug }

type Model = {
  post: Post,
  body: Html Never,
  comments: List Comment,
}

-- load finds the post by its slug; a slug that was never used is a 404, and so is a draft for
-- anyone but its author. Every message of the page runs after this `load`, so nobody else can
-- comment on a draft either.
load : Params -> Session -> Query Model
load params session = {
  let found <- Post.bySlug params.slug
  match found {
    Just post -> if canSee post session then shown post else Query.notFound,
    Nothing -> Query.notFound,
  }
}

-- canSee is whether the session may read the post: anyone a published one, only its author a draft.
canSee : Post -> Session -> Bool
canSee post session = post.published || session.user == Just post.author

shown : Post -> Query Model
shown post = {
  let body <- Markdown.render post.body
  let comments <-
    Db.from Comment
      |> Db.where (Comment.post.eq post.id)
      |> Db.orderBy Comment.created.asc
      |> Db.all
  Query.succeed { post: post, body: body, comments: comments }
}

form NewComment = {
  author: Comment.Name,
  body: Comment.Body,
}

-- The message carries no post id: the page's slug names the post, which `load` found.
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
      <div class="body">{model.body}</div>
    </article>

    <section class="comments">
      <h2>Comments</h2>
      <ul>
        {for c in model.comments ->
          <li>
            <p class="who">{c.author}</p>
            <p>{c.body}</p>
          </li>}
      </ul>
      <form submit={AddComment}>
        <Errors form />
        <Input field="author" label="Name" />
        <Errors field="author" />
        <Textarea field="body" label="Comment" />
        <Errors field="body" />
        <button>Comment</button>
      </form>
    </section>

    <a href={Posts.Index {}}>All posts</a>
  </Layout.Page>

-- The body goes through the real Markdown adapter (adapters/markdown/markdown.go).
test "a post's body is rendered as Markdown" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", published: True, author: author }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
}

-- The same page with the Markdown capability faked (`fake Markdown` in app/Markdown.alder):
-- the body arrives as plain text, so the test is about where `load` puts it, not about Markdown.
test "a post's body is rendered through the Markdown capability" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", published: True, author: author }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  let steps <- Test.trace
  Task.succeed ({ body: m.body, rendered: List.member "Markdown.render" steps } == { body: Html.text "Not *yet*.", rendered: True })
}

test "a comment is listed oldest first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", published: True, author: author }
  let before <- load { slug: Post.Slug "hello" } { user: Nothing }
  let _ <- update (AddComment { form: { author: Comment.Name "Ann", body: Comment.Body "First!" } }) before { user: Nothing }
  let _ <- update (AddComment { form: { author: Comment.Name "Bob", body: Comment.Body "Second." } }) before { user: Nothing }
  let after <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed (List.map (\c -> c.body) after.comments == [Comment.Body "First!", Comment.Body "Second."])
}

test "a post shows only its own comments" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let hello <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", published: True, author: author }
  let other <- Db.insert Post { slug: Post.Slug "other", title: Text.NonEmpty "Other", body: "", published: True, author: author }
  let _ <- Db.insert Comment { post: hello, author: Comment.Name "Ann", body: Comment.Body "On hello." }
  let _ <- Db.insert Comment { post: other, author: Comment.Name "Bob", body: Comment.Body "On other." }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed (List.map (\c -> c.body) m.comments == [Comment.Body "On hello."])
}

-- A comment through the form: the answer is a redirect to the post's own page, which lists it.
test "commenting through the form" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", published: True, author: author }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submit NewComment { author: Comment.Name "Ann", body: Comment.Body "First!" }
  Task.succeed ({ answer: r.answer, who: Test.texts ".comments .who" r.doc } == { answer: Redirected (Posts.Show { slug: Post.Slug "hello" }), who: ["Ann"] })
}

-- An empty comment, posted as the form's inputs would send it: the form is re-rendered with the
-- error beside the field, and no comment is stored.
test "an empty comment is rejected" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", published: True, author: author }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "Ann" }, { name: "body", value: "" }]
  let comments <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, author: Test.attr "value" "input[name=author]" r.doc, comments: comments } == { answer: FormRejected [{ field: "body", message: "is not valid" }], author: Just "Ann", comments: 0 })
}

-- A draft's page through the browser: a visitor and a logged-in reader get the 404; its author
-- gets the page.
test "a draft is not found by anyone but its author" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Soon.", author: author }
  let visitor <- Test.open (Posts.Show { slug: Post.Slug "hello" })
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Posts.Show { slug: Post.Slug "hello" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "hello" })
  Task.succeed ({ visitor: visitor.answer, reader: asReader.answer, author: asAuthor.answer, title: Test.text "h1" asAuthor.doc } == { visitor: PageMissing, reader: PageMissing, author: Shown, title: Just "Hello" })
}

test "a published post is shown to a reader" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Now.", published: True, author: author }
  let visitor <- Test.open (Posts.Show { slug: Post.Slug "hello" })
  Task.succeed ({ answer: visitor.answer, title: Test.text "h1" visitor.doc } == { answer: Shown, title: Just "Hello" })
}

--- file: app/User.alder ---
module User

type UserId = brand Text

-- Anyone, logged in or not, can read and comment. An Author can also write posts: the Admin
-- pages are a 404 for a Reader.
type Role = Reader | Author

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  role: Role = Reader,
  created: Time @created,
}

-- author is the user with that id when they are an Author, and Nothing for a Reader (or no
-- user): the one role check of the Admin pages and of the front page's Write link.
author : UserId -> Query (Maybe User)
author id = Db.from User |> Db.where (User.id.eq id) |> Db.where (User.role.eq Author) |> Db.first
