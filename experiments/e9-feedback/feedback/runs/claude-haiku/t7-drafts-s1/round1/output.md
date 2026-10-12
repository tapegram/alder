--- file: app/Admin.alder ---
module Admin

import Post
import User
import Layout

page "/admin" { } requires user

type Model = {
  author: User,
  posts: List Post,
  drafts: List Post,
}

-- load lets only an Author in: a Reader who is logged in gets the same 404 as a page that does
-- not exist. Every message of the page runs after this `load`, so `update` needs no check of
-- its own.
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
      Query.succeed { author: user, posts: posts, drafts: List.filter (\p -> not p.published) posts }
    },
    Nothing -> Query.notFound,
  }
}

form NewPost = {
  slug: Post.Slug,
  title: Text.NonEmpty,
  body: Text,
}

-- A new post is a draft: `published` takes its default, False.
msg Create form NewPost
msg Publish { id: Post.PostId }

-- authored is the post with that id when the user wrote it. Any other id, a missing one too, is a
-- 404: a message's id comes from the request, so `update` checks it again.
authored : Post.PostId -> User.UserId -> Task Post
authored id user = {
  let found <- Db.get Post id
  match found {
    Just post -> if post.author == user then Task.succeed post else Task.notFound,
    Nothing -> Task.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
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
    let post <- authored p.id session.user
    let _ <- Db.update Post post.id (\x -> { x | published: True })
    Task.succeed (Redirect (Admin {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Your posts">
    <h1>Your posts</h1>

    <h2>Drafts</h2>
    <ul id="drafts">
      {for post in model.drafts ->
        <li>
          <span class="title">{post.title}</span>
          <button click={Publish { id: post.id }}>Publish</button>
        </li>}
    </ul>

    <ul class="posts">
      {for post in model.posts ->
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

test "a taken slug is refused" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let model <- load {} { user: author }
  let r <- Test.run (update (Create { form: { slug: Post.Slug "hello", title: Text.NonEmpty "Again", body: "" } }) model { user: author })
  let posts <- Db.from Post |> Db.count
  Task.succeed ({ value: r.value, posts: posts } == { value: Ok (Reject "Another post has that slug."), posts: 1 })
}

-- A new post is a draft: it is listed under Drafts and not on the front page until published.
test "publishing a draft from the dashboard puts it on the front page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "draft-one", title: Text.NonEmpty "Draft one", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let dashboard <- Test.get (Admin {})
  let before <- Test.open (Posts.Index {})
  let _ <- Test.get (Admin {})
  let published <- Test.click (Publish { id: id })
  let front <- Test.open (Posts.Index {})
  Task.succeed ({ drafts: Test.texts "#drafts .title" dashboard.doc, before: Test.count "ul.posts li" before.doc, answer: published.answer, after: Test.texts "ul.posts a" front.doc } == { drafts: ["Draft one"], before: 0, answer: Redirected (Admin {}), after: ["Draft one"] })
}

test "publishing another author's draft is not found" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "draft-one", title: Text.NonEmpty "Draft one", body: "", author: author }
  let model <- load {} { user: other }
  let r <- Test.run (update (Publish { id: id }) model { user: other })
  let after <- Db.get Post id
  Task.succeed ({ value: r.value, published: Maybe.map (\p -> p.published) after } == { value: Err "not found", published: Just False })
}

-- The dashboard lists the signed-in author's drafts and no one else's, and not a published post.
test "the dashboard lists only the author's drafts" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "mine", title: Text.NonEmpty "Mine", body: "", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "live", title: Text.NonEmpty "Live", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "theirs", title: Text.NonEmpty "Theirs", body: "", author: other }
  let _ <- Test.login { user: Just author }
  let page <- Test.get (Admin {})
  Task.succeed (Test.texts "#drafts .title" page.doc == ["Mine"])
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

-- The page is a 404 for a visitor's own request only as a login redirect: a visitor is sent to log in.
test "a visitor is sent to log in from the edit page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let visitor <- Test.open (Admin.Edit { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

-- Saving changes the title and the body of the post, and the edit page answers Rerender.
test "saving changes the title and the body" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let model <- load { id: id } { user: author }
  let r <- Test.run (update (Save { form: { title: Text.NonEmpty "New", body: "New body" } }) model { user: author })
  let after <- Db.get Post id
  Task.succeed ({ value: r.value, title: Maybe.map (\p -> p.title) after, body: Maybe.map (\p -> p.body) after } == { value: Ok Rerender, title: Just (Text.NonEmpty "New"), body: Just "New body" })
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

-- Mutation note: the session put and the redirect of `Submit` are reached only through
-- Password.verify, and no test here may call that capability, so their mutants survive. Nothing
-- else in the module can tell them apart.

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

table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  author: User.UserId @ref User,
  created: Time @created,
  published: Bool = False,
}

bySlug : Slug -> Query (Maybe Post)
bySlug slug = Db.from Post |> Db.where (Post.slug.eq slug) |> Db.first

-- isSlug is the rule a Slug keeps: lowercase letters, digits and hyphens, at least one.
isSlug : Text -> Bool
isSlug s = Text.matches "^[a-z0-9-]+$" s

test "a slug is lowercase letters, digits and hyphens" =
  List.map isSlug ["hello-world", "post-2", "Hello", "a b", "", "x/y"] == [True, True, False, False, False, False]

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
-- is cut to 100 characters. Drafts are never listed nor found. It asks for one post more than a
-- page holds: if that one comes back there is a next page (`hasMore`), and it is dropped from the
-- list. `canWrite` shows the link to the Admin page to an Author only.
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
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author, published: True }
  let model <- load { page: 1, q: "" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"])
}

-- A draft is neither on the front page nor found by a search; a published post is both.
test "a draft is not listed nor found by a search" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "draft-tomato", title: Text.NonEmpty "Draft tomato", body: "", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "open-tomato", title: Text.NonEmpty "Open tomato", body: "", author: author, published: True }
  let front <- load { page: 1, q: "" } { user: Nothing }
  let found <- load { page: 1, q: "tomato" } { user: Nothing }
  Task.succeed ({ front: List.map (\p -> p.title) front.posts, found: List.map (\p -> p.title) found.posts } == { front: [Text.NonEmpty "Open tomato"], found: [Text.NonEmpty "Open tomato"] })
}

test "search matches title or body" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "tomatoes", title: Text.NonEmpty "Tomato season", body: "It is here.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let model <- load { page: 1, q: "TOMATO" } { user: Nothing }
  let titles = List.map (\p -> p.title) model.posts
  Task.succeed ({ count: List.length titles, title: List.member (Text.NonEmpty "Tomato season") titles, body: List.member (Text.NonEmpty "Soup") titles } == { count: 2, title: True, body: True })
}

test "a `%` in the query is literal" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "done", title: Text.NonEmpty "100% done", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "ways", title: Text.NonEmpty "1000 ways", body: "", author: author, published: True }
  let model <- load { page: 1, q: "100%" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "100% done"])
}

test "page 99 is empty" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let pages <- Query.all (List.map (\n -> load { page: n, q: "" } { user: Nothing }) [1, 2, 99, 0])
  let seen = List.map (\m -> { page: m.page, posts: List.length m.posts, hasMore: m.hasMore }) pages
  Task.succeed (seen == [{ page: 1, posts: 10, hasMore: True }, { page: 2, posts: 1, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 10, hasMore: True }])
}

-- Exactly one page of posts has no next page: the extra post that would show one is not there.
test "a full page with no more posts has no next page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 10))
  let model <- load { page: 1, q: "" } { user: Nothing }
  Task.succeed ({ posts: List.length model.posts, hasMore: model.hasMore } == { posts: 10, hasMore: False })
}

test "a negative page is page 1" = {
  let model <- load { page: -3, q: "" } { user: Nothing }
  Task.succeed (model.page == 1)
}

test "a `_` in the query is literal" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "snake", title: Text.NonEmpty "snake_case", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "camel", title: Text.NonEmpty "snakeYcase", body: "", author: author, published: True }
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
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let first <- Test.get (Posts.Index {})
  let second <- Test.get (Posts.Index { page: 2 })
  Task.succeed ({ firstPosts: Test.count "ul.posts li" first.doc, next: Test.attr "href" "a[rel=next]" first.doc, secondPosts: Test.count "ul.posts li" second.doc, prev: Test.attr "href" "a[rel=prev]" second.doc, more: Test.has "a[rel=next]" second.doc } == { firstPosts: 10, next: Just "/?page=2", secondPosts: 1, prev: Just "/", more: False })
}

-- The first page has no link back, and with posts beyond it, a link forward.
test "the first page has no link back" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "only", title: Text.NonEmpty "Only", body: "", author: author, published: True }
  let first <- Test.get (Posts.Index {})
  Task.succeed ({ prev: Test.has "a[rel=prev]" first.doc, next: Test.has "a[rel=next]" first.doc } == { prev: False, next: False })
}

test "a search through the form" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let _ <- Test.get (Posts.Index {})
  let r <- Test.submit SearchForm { q: "tomato" }
  Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Posts.Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
}

-- Mutation note: at page 1 both `params.page < 1` and `<= 1` give 1; at `lastPage` both `>` and
-- `>=` give `lastPage`; and without `Db.limit` the first ten rows, and whether there are more than
-- ten, are the same. Those mutants are equivalent: no input can tell them apart.

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
-- anyone but its author.
load : Params -> Session -> Query Model
load params session = {
  let found <- Post.bySlug params.slug
  match found {
    Just post -> if visible post session then shown post else Query.notFound,
    Nothing -> Query.notFound,
  }
}

-- visible is whether the session may see the post: anyone may see a published post, and only
-- its author may see a draft.
visible : Post -> Session -> Bool
visible post session = post.published || session.user == Just post.author

-- shown is the page's Model for a post: its body rendered, and its comments.
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
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
}

-- The same page with the Markdown capability faked (`fake Markdown` in app/Markdown.alder):
-- the body arrives as plain text, so the test is about where `load` puts it, not about Markdown.
test "a post's body is rendered through the Markdown capability" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- load { slug: Post.Slug "hello" } { user: Nothing }
  let steps <- Test.trace
  Task.succeed ({ body: m.body, rendered: List.member "Markdown.render" steps } == { body: Html.text "Not *yet*.", rendered: True })
}

-- A draft is a 404 for a reader and for a visitor; its author can open it.
test "a draft is not found by a reader, and its author can open it" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
  let visitor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ visitor: visitor.answer, reader: asReader.answer, author: asAuthor.answer } == { visitor: PageMissing, reader: PageMissing, author: Shown })
}

test "a comment is listed oldest first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let before <- load { slug: Post.Slug "hello" } { user: Nothing }
  let _ <- update (AddComment { form: { author: Comment.Name "Ann", body: Comment.Body "First!" } }) before { user: Nothing }
  let _ <- update (AddComment { form: { author: Comment.Name "Bob", body: Comment.Body "Second." } }) before { user: Nothing }
  let after <- load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed (List.map (\c -> c.body) after.comments == [Comment.Body "First!", Comment.Body "Second."])
}

-- Commenting answers with a redirect back to the post.
test "a comment redirects back to its post" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let before <- load { slug: Post.Slug "hello" } { user: Nothing }
  let r <- Test.run (update (AddComment { form: { author: Comment.Name "Ann", body: Comment.Body "Hi." } }) before { user: Nothing })
  Task.succeed (r.value == Ok (Redirect (Posts.Show { slug: Post.Slug "hello" })))
}

-- A post's page lists its own comments only, not those of another post.
test "a post's page lists only its own comments" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "a", title: Text.NonEmpty "A", body: "", author: author, published: True }
  let b <- Db.insert Post { slug: Post.Slug "b", title: Text.NonEmpty "B", body: "", author: author, published: True }
  let _ <- Db.insert Comment { post: b, author: Comment.Name "Ann", body: Comment.Body "On b." }
  let m <- load { slug: Post.Slug "a" } { user: Nothing }
  Task.succeed (List.length m.comments == 0)
}

-- An empty comment, posted as the form's inputs would send it: the form is re-rendered with the
-- error beside the field, and no comment is stored.
test "an empty comment is rejected" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let _ <- Test.get (Posts.Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw NewComment [{ name: "author", value: "Ann" }, { name: "body", value: "" }]
  let comments <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, author: Test.attr "value" "input[name=author]" r.doc, comments: comments } == { answer: FormRejected [{ field: "body", message: "is not valid" }], author: Just "Ann", comments: 0 })
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

-- Only an Author is found, and only the one asked for: a Reader gets Nothing, and another
-- Author is not the user who was asked for.
test "author finds only an Author, and only the one asked for" = {
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "", role: Author }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "", role: Author }
  let ofReader <- author reader
  let ofB <- author b
  Task.succeed ({ reader: Maybe.map (\u -> u.id) ofReader, b: Maybe.map (\u -> u.id) ofB } == { reader: Nothing, b: Just b })
}
