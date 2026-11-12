--- file: app/Admin.alder ---
module Admin

import Post
import User
import Layout

route Admin   = GET  "/admin" { }              requires user
route Create  = POST "/admin" { } form NewPost requires user
route Publish = PUT  "/admin/{id}/published" { id: Post.PostId } requires user

type Model = {
  author: User,
  drafts: List Post,
}

-- load lets only an Author in: a Reader who is logged in gets the same 404 as a page that does
-- not exist. No `load` runs before a write route, so Create and Publish ask `User.author` too.
Admin.load : Admin.Params -> Session -> Query Model
Admin.load params session = {
  let me <- User.author session.user
  match me {
    Just user -> {
      let drafts <-
        Db.from Post
          |> Db.where (Post.author.eq user.id)
          |> Db.where (Post.published.eq False)
          |> Db.orderBy Post.created.desc
          |> Db.all
      Query.succeed { author: user, drafts: drafts }
    },
    Nothing -> Query.notFound,
  }
}

form NewPost = {
  slug: Post.Slug,
  title: Text.NonEmpty,
  body: Text,
}

-- A new post starts as a draft: `published` takes its default, False.
Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let me <- User.author session.user
  match me {
    Just user -> {
      let taken <- Post.bySlug p.form.slug
      match taken {
        Just _ -> Task.succeed (Response.invalid "Another post has that slug."),
        Nothing -> {
          let id <- Db.insert Post { slug: p.form.slug, title: p.form.title, body: p.form.body, author: user.id }
          Task.succeed (Response.redirect (Admin.Edit.Edit { id: id }))
        },
      }
    },
    Nothing -> Task.notFound,
  }
}

publish : Post -> Task Response
publish post = {
  let _ <- Db.update Post post.id (\x -> { x | published: True })
  Task.succeed (Response.redirect (Admin {}))
}

-- Only the signed-in Author who wrote the post may publish it; any other id is not found.
Publish.update : Publish.Params -> Session -> Task Response
Publish.update p session = {
  let me <- User.author session.user
  let found <- Db.get Post p.id
  match found {
    Just post -> if Maybe.isJust me && post.author == session.user then publish post else Task.notFound,
    Nothing -> Task.notFound,
  }
}

Admin.view : Model -> Session -> Html
Admin.view model session =
  <Layout.Page title="Your posts">
    <h1>Your posts</h1>

    <h2>Drafts</h2>
    <ul id="drafts">
      {for post in model.drafts ->
        <li>
          <a class="title" href={Admin.Edit.Edit { id: post.id }}>{post.title}</a>
          <button click={Publish { id: post.id }}>Publish</button>
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
    let m <- Admin.load {} { user: reader }
    Task.succeed m.author.id
  }
  let asAuthor <- Test.run {
    let m <- Admin.load {} { user: author }
    Task.succeed m.author.id
  }
  Task.succeed ({ reader: asReader.value, author: asAuthor.value } == { reader: Err "not found", author: Ok author })
}

test "creating a post redirects to its edit page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let r <- Test.run (Create.update { form: { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi." } } { user: author })
  let created <- Post.bySlug (Post.Slug "hello")
  match created {
    Just post -> Task.succeed (r.value == Ok (Response.redirect (Admin.Edit.Edit { id: post.id }))),
    Nothing -> Task.fail "no post has the slug hello",
  }
}

-- A new post is a draft: it is on the author's dashboard, not on the front page.
test "a new post starts as a draft" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Test.run (Create.update { form: { slug: Post.Slug "fresh", title: Text.NonEmpty "Fresh", body: "" } } { user: author })
  let created <- Post.bySlug (Post.Slug "fresh")
  Task.succeed (Maybe.map (\p -> p.published) created == Just False)
}

test "a taken slug is refused" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let r <- Test.run (Create.update { form: { slug: Post.Slug "hello", title: Text.NonEmpty "Again", body: "" } } { user: author })
  let posts <- Db.from Post |> Db.count
  Task.succeed ({ value: r.value, posts: posts } == { value: Ok (Response.invalid "Another post has that slug."), posts: 1 })
}

-- Publishing a draft from the dashboard puts it on the front page. The dashboard is held when
-- the Publish button is clicked, so the test fetches it again after looking at the front page.
test "publishing a draft from the dashboard puts it on the front page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "A draft", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let dash <- Test.get (Admin {})
  let before <- Test.get (Posts.Index.Index {})
  let _ <- Test.get (Admin {})
  let r <- Test.click (Publish { id: id })
  let after <- Test.get (Posts.Index.Index {})
  Task.succeed (
    {
      drafts: Test.count "#drafts li" dash.doc,
      before: Test.count "ul.posts li" before.doc,
      answer: r.answer,
      after: Test.texts "ul.posts a" after.doc,
    } == {
      drafts: 1,
      before: 0,
      answer: Redirected (Admin {}),
      after: ["A draft"],
    }
  )
}

-- An Author cannot publish another Author's draft: the id is not theirs, so it is not found.
test "publishing another author's draft is not found" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "A draft", body: "", author: author }
  let r <- Test.run (Publish.update { id: id } { user: other })
  let after <- Db.get Post id
  Task.succeed ({ value: r.value, published: Maybe.map (\p -> p.published) after } == { value: Err "not found", published: Just False })
}

-- The admin page needs an Author.
test "the admin page needs an Author" = {
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let visitor <- Test.open (Admin {})
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Admin {})
  Task.succeed ({ visitor: visitor.answer, reader: asReader.answer } == { visitor: Redirected (Auth.Login.Login {}), reader: PageMissing })
}

test "an Author logs out from the admin page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Test.login { user: Just author }
  let page <- Test.get (Admin {})
  let out <- Test.click App.Logout
  Task.succeed ({ heading: Test.text "h1" page.doc, out: out.answer, user: out.session.user } == { heading: Just "Your posts", out: Redirected (Auth.Login.Login {}), user: Nothing })
}

--- file: app/Admin/Edit.alder ---
module Admin.Edit

import Post
import Comment
import User
import Layout

route Edit   = GET    "/admin/{id}" { id: Post.PostId }               requires user
route Save   = PUT    "/admin/{id}" { id: Post.PostId } form EditPost requires user
route Delete = DELETE "/admin/{id}" { id: Post.PostId }               requires user

type Model = { post: Post }

-- editable finds the post when the session's user wrote it and is still an Author (the same
-- gate as the Admin page: an author made a Reader can no longer edit their old posts); anyone
-- else's post, like a missing one, is Nothing. The page's `load` and both write routes ask it:
-- no `load` runs before a write route, and its id comes from the request.
editable : Post.PostId -> User.UserId -> Query (Maybe Post)
editable id user = {
  let me <- User.author user
  let found <- Db.get Post id
  match found {
    Just post -> Query.succeed (if me != Nothing && post.author == user then Just post else Nothing),
    Nothing -> Query.succeed Nothing,
  }
}

Edit.load : Edit.Params -> Session -> Query Model
Edit.load params session = {
  let found <- editable params.id session.user
  match found {
    Just post -> Query.succeed { post: post },
    Nothing -> Query.notFound,
  }
}

form EditPost = {
  title: Text.NonEmpty,
  body: Text,
}

Save.update : Save.Params -> Session -> Task Response
Save.update p session = {
  let found <- editable p.id session.user
  match found {
    Just post -> {
      let _ <- Db.update Post post.id (\x -> { x | title: p.form.title, body: p.form.body })
      Task.succeed Response.refresh
    },
    Nothing -> Task.notFound,
  }
}

-- The post's comments go with it: Comment.post is `@ref Post @cascade`.
Delete.update : Delete.Params -> Session -> Task Response
Delete.update p session = {
  let found <- editable p.id session.user
  match found {
    Just post -> {
      let _ <- Db.delete Post post.id
      Task.succeed (Response.redirect (Admin.Admin {}))
    },
    Nothing -> Task.notFound,
  }
}

Edit.view : Model -> Session -> Html
Edit.view model session =
  <Layout.Page title={"Edit: " ++ model.post.title}>
    <h1>{model.post.title}</h1>
    <p><a href={Posts.Show.Show { slug: model.post.slug }}>View</a></p>

    <form submit={Save { id: model.post.id }}>
      <Errors form />
      <Input field="title" label="Title" value={model.post.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body (Markdown)" value={model.post.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>

    <button click={Delete { id: model.post.id }}>Delete</button>
    <a href={Admin.Admin {}}>All your posts</a>
  </Layout.Page>

test "deleting a post deletes its comments" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Db.insert Comment { post: id, author: Comment.Name "Ann", body: Comment.Body "First!" }
  let _ <- Db.insert Comment { post: id, author: Comment.Name "Bob", body: Comment.Body "Second." }
  let r <- Test.run (Delete.update { id: id } { user: author })
  let posts <- Db.from Post |> Db.count
  let comments <- Db.from Comment |> Db.count
  Task.succeed ({ value: r.value, trace: r.trace, posts: posts, comments: comments } == { value: Ok (Response.redirect (Admin.Admin {})), trace: ["Db.first User", "Db.get Post", "Db.delete Post"], posts: 0, comments: 0 })
}

test "another author's post is not found" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let r <- Test.run {
    let m <- Edit.load { id: id } { user: other }
    Task.succeed m.post.id
  }
  Task.succeed (r.value == Err "not found")
}

test "a demoted author cannot edit" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let before <- Test.run {
    let m <- Edit.load { id: id } { user: author }
    Task.succeed m.post.id
  }
  let _ <- Db.update User author (\u -> { u | role: Reader })
  let after <- Test.run {
    let m <- Edit.load { id: id } { user: author }
    Task.succeed m.post.id
  }
  Task.succeed ({ before: before.value, after: after.value } == { before: Ok id, after: Err "not found" })
}

--- file: app/App.alder ---
module App

import User
import Layout

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

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Posts.Index.Index {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
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

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <header>
        <a href={Posts.Index.Index {}}>Blog</a>
      </header>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Markdown.alder ---
module Markdown

-- Rendering a post's body is outside the language: a capability implemented in Go
-- (adapters/markdown/markdown.go), which builds the HTML node by node, so a post cannot inject markup.
capability Markdown {
  render : Text -> Query Html,
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

route Index  = GET  "/"       { page: Int = 1, q: Text = "" }
-- A search posts its form to a write route like any other: `update` redirects to the first page
-- of the results, whose address (/?q=…) can be bookmarked and shared.
route Search = POST "/search" { } form SearchForm

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
-- is cut to 100 characters. Drafts are never listed or found. It asks for one post more than a
-- page holds: if that one comes back there is a next page (`hasMore`), and it is dropped from
-- the list. `canWrite` shows the link to the Admin page to an Author only.
Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
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

Search.update : Search.Params -> Session -> Task Response
Search.update p session = Task.succeed (Response.redirect (Index { page: 1, q: p.form.q }))

Index.view : Model -> Session -> Html
Index.view model session =
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
            <a href={Posts.Show.Show { slug: post.slug }}>{post.title}</a>
            <span class="date">{Time.format "Jan 2, 2006" post.created}</span>
          </li>}
      </ul>}

    <nav class="pages">
      {if model.page > 1 then <a href={Index { page: model.page - 1, q: model.q }} rel="prev">Newer</a> else Html.none}
      {if model.hasMore then <a href={Index { page: model.page + 1, q: model.q }} rel="next">Older</a> else Html.none}
    </nav>

    {match session.user {
      Just _ -> <p>{if model.canWrite then <a href={Admin.Admin {}}>Write</a> else Html.none} <button click={App.Logout}>Log out</button></p>,
      Nothing -> <p><a href={Auth.Login.Login {}}>Log in</a></p>,
    }}
  </Layout.Page>

test "the front page lists the posts, newest first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author, published: True }
  let model <- Index.load { page: 1, q: "" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"])
}

-- A draft is not on the front page, and a search does not find it.
test "a draft is not listed and not found by a search" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Secret draft", body: "tomato", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "live", title: Text.NonEmpty "Live", body: "", author: author, published: True }
  let all <- Index.load { page: 1, q: "" } { user: Nothing }
  let found <- Index.load { page: 1, q: "tomato" } { user: Nothing }
  Task.succeed ({ all: List.map (\p -> p.title) all.posts, searched: List.length found.posts } == { all: [Text.NonEmpty "Live"], searched: 0 })
}

test "search matches title or body" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "tomatoes", title: Text.NonEmpty "Tomato season", body: "It is here.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let model <- Index.load { page: 1, q: "TOMATO" } { user: Nothing }
  let titles = List.map (\p -> p.title) model.posts
  Task.succeed ({ count: List.length titles, title: List.member (Text.NonEmpty "Tomato season") titles, body: List.member (Text.NonEmpty "Soup") titles } == { count: 2, title: True, body: True })
}

test "a `%` in the query is literal" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "done", title: Text.NonEmpty "100% done", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "ways", title: Text.NonEmpty "1000 ways", body: "", author: author, published: True }
  let model <- Index.load { page: 1, q: "100%" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "100% done"])
}

test "page 99 is empty" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let pages <- Query.all (List.map (\n -> Index.load { page: n, q: "" } { user: Nothing }) [1, 2, 99, 0])
  let seen = List.map (\m -> { page: m.page, posts: List.length m.posts, hasMore: m.hasMore }) pages
  Task.succeed (seen == [{ page: 1, posts: 10, hasMore: True }, { page: 2, posts: 1, hasMore: False }, { page: 99, posts: 0, hasMore: False }, { page: 1, posts: 10, hasMore: True }])
}

test "a negative page is page 1" = {
  let model <- Index.load { page: -3, q: "" } { user: Nothing }
  Task.succeed (model.page == 1)
}

test "a `_` in the query is literal" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "snake", title: Text.NonEmpty "snake_case", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "camel", title: Text.NonEmpty "snakeYcase", body: "", author: author, published: True }
  let model <- Index.load { page: 1, q: "e_c" } { user: Nothing }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "snake_case"])
}

-- The query is cut to its first 100 characters: SQLite refuses a pattern over ~50 KB, and below
-- that a search costs the length of every post times the length of the query.
test "a long query is cut" = {
  let long = Text.join "" (List.map (\n -> "tomato ") (List.range 1 100))
  let model <- Index.load { page: 1, q: long } { user: Nothing }
  Task.succeed (model.q == Text.slice 0 100 long)
}

test "a search answers with the first page of its results" = {
  let r <- Search.update { form: { q: "soup" } } { user: Nothing }
  Task.succeed (r == Response.redirect (Index { page: 1, q: "soup" }))
}

test "only an Author sees the Write link" = {
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let pages <- Query.all (List.map (\user -> Index.load { page: 1, q: "" } { user: user }) [Nothing, Just reader, Just author])
  Task.succeed (List.map (\m -> m.canWrite) pages == [False, False, True])
}

-- The same page through the browser: the links a page renders, and the search form posted as a
-- user posts it (book 10, "Testing through the browser").
test "the second page links back to the first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Task.all (List.map (\n -> Db.insert Post { slug: Post.Slug ("post-" ++ Text.fromInt n), title: Text.NonEmpty "A post", body: "", author: author, published: True }) (List.range 1 11))
  let first <- Test.get (Index {})
  let second <- Test.get (Index { page: 2 })
  Task.succeed ({ firstPosts: Test.count "ul.posts li" first.doc, next: Test.attr "href" "a[rel=next]" first.doc, secondPosts: Test.count "ul.posts li" second.doc, prev: Test.attr "href" "a[rel=prev]" second.doc, more: Test.has "a[rel=next]" second.doc } == { firstPosts: 10, next: Just "/?page=2", secondPosts: 1, prev: Just "/", more: False })
}

test "a search through the form" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "trains", title: Text.NonEmpty "Trains", body: "Fast ones.", author: author, published: True }
  let _ <- Test.get (Index {})
  let r <- Test.submit Search { q: "tomato" }
  Task.succeed ({ answer: r.answer, titles: Test.texts "ul.posts a" r.doc, query: Test.attr "value" "input[name=q]" r.doc } == { answer: Redirected (Index { page: 1, q: "tomato" }), titles: ["Soup"], query: Just "tomato" })
}

--- file: app/Posts/Show.alder ---
module Posts.Show

import Post
import User
import Comment
import Markdown
import Layout

route Show       = GET  "/posts/{slug}"          { slug: Post.Slug }
-- AddComment takes the page's slug from its path. No `load` runs before a write route, so its
-- `update` finds the post again: a slug that was never used is a 404.
route AddComment = POST "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

type Model = {
  post: Post,
  body: Html,
  comments: List Comment,
}

-- canSee is whether this user may see a post: anyone sees a published post, and a draft only
-- its author (a visitor, or another user, gets the same 404 as a missing post).
canSee : Post -> Maybe User.UserId -> Bool
canSee post user = post.published || user == Just post.author

-- showPost reads the page of a post the user may see: its body rendered, its comments.
showPost : Post -> Query Model
showPost post = {
  let body <- Markdown.render post.body
  let comments <-
    Db.from Comment
      |> Db.where (Comment.post.eq post.id)
      |> Db.orderBy Comment.created.asc
      |> Db.all
  Query.succeed { post: post, body: body, comments: comments }
}

-- load finds the post by its slug; a slug that was never used, or a draft the user may not see,
-- is a 404.
Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Post.bySlug params.slug
  match found {
    Just post -> if canSee post session.user then showPost post else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  author: Comment.Name,
  body: Comment.Body,
}

addComment : Post -> Comment.Name -> Comment.Body -> Task Response
addComment post author body = {
  let _ <- Db.insert Comment { post: post.id, author: author, body: body }
  Task.succeed (Response.redirect (Show { slug: post.slug }))
}

-- A comment on a draft is refused the same way the draft's page is: not found.
AddComment.update : AddComment.Params -> Session -> Task Response
AddComment.update p session = {
  let found <- Post.bySlug p.slug
  match found {
    Just post -> if canSee post session.user then addComment post p.form.author p.form.body else Task.notFound,
    Nothing -> Task.notFound,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
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
      <form submit={AddComment { slug: model.post.slug }}>
        <Errors form />
        <Input field="author" label="Name" />
        <Errors field="author" />
        <Textarea field="body" label="Comment" />
        <Errors field="body" />
        <button>Comment</button>
      </form>
    </section>

    <a href={Posts.Index.Index {}}>All posts</a>
  </Layout.Page>

-- The body goes through the real Markdown adapter (adapters/markdown/markdown.go).
test "a post's body is rendered as Markdown" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- Show.load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed ({ title: m.post.title, body: m.body } == { title: Text.NonEmpty "Hello", body: <p>Not <em>yet</em>.</p> })
}

-- The same page with the Markdown capability faked (`fake Markdown` in app/Markdown.alder):
-- the body arrives as plain text, so the test is about where `load` puts it, not about Markdown.
test "a post's body is rendered through the Markdown capability" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Not *yet*.", author: author, published: True }
  let m <- Show.load { slug: Post.Slug "hello" } { user: Nothing }
  let steps <- Test.trace
  Task.succeed ({ body: m.body, rendered: List.member "Markdown.render" steps } == { body: Html.text "Not *yet*.", rendered: True })
}

-- A draft's page is not found for a reader, and is for its author.
test "a draft's page is not found for a reader, and is for its author" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
  let reader <- Test.run {
    let m <- Show.load { slug: Post.Slug "draft" } { user: Nothing }
    Task.succeed m.post.title
  }
  let own <- Test.run {
    let m <- Show.load { slug: Post.Slug "draft" } { user: Just author }
    Task.succeed m.post.title
  }
  Task.succeed (reader.value == Err "not found" && own.value == Ok (Text.NonEmpty "Draft"))
}

test "a comment is listed oldest first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let _ <- AddComment.update { slug: Post.Slug "hello", form: { author: Comment.Name "Ann", body: Comment.Body "First!" } } { user: Nothing }
  let _ <- AddComment.update { slug: Post.Slug "hello", form: { author: Comment.Name "Bob", body: Comment.Body "Second." } } { user: Nothing }
  let after <- Show.load { slug: Post.Slug "hello" } { user: Nothing }
  Task.succeed (List.map (\c -> c.body) after.comments == [Comment.Body "First!", Comment.Body "Second."])
}

-- An empty comment, posted as the form's inputs would send it: the form is re-rendered with the
-- error beside the field, and no comment is stored.
test "an empty comment is rejected" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author, published: True }
  let _ <- Test.get (Show { slug: Post.Slug "hello" })
  let r <- Test.submitRaw (AddComment { slug: Post.Slug "hello" }) [{ name: "author", value: "Ann" }, { name: "body", value: "" }]
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
