# 14. A second app

The to-do app of chapter 2 has one table that matters and one kind of
user. `examples/blog` adds what a to-do list never needed: two tables
joined by a reference, two kinds of user, a capability that `load`
calls, pages of results, and a search. This chapter covers only what is
new; the rules are in chapters 6 to 10.

The blog has eleven modules in `examples/blog/app/`:

| Module | What it is |
|---|---|
| `App`, `Layout`, `Auth.Login` | As in the to-do app; the login leads to `/` |
| `User` | The user table, with a `role` column |
| `Post`, `Comment` | The two tables, and the functions about posts |
| `Markdown` | A capability that turns a post's body into HTML |
| `Posts.Index` | `/`: the published posts, a page at a time, and a search |
| `Posts.Show` | `/posts/{slug}`: one post, its comments, a comment form |
| `Admin` | `/admin`: an author's own posts, and a form for a new one |
| `Admin.Edit` | `/admin/{id}`: edit, publish, unpublish or delete a post |

Four modules are shown in full, piece by piece: `go test ./book` checks
that the pieces of each equal its file in `examples/blog`, blank lines
aside (chapter 11). The others are described in the text.

## A relation: `Comment`

```alder part=blog/Comment
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
```

A comment belongs to a post: `post: Post.PostId @ref Post` names a `Post`
row, and inserting a comment for a missing post fails (chapter 8,
"Writing"). `@cascade` is the field's **delete rule**: deleting a post
deletes its comments. The default, `@restrict`, would refuse to delete a
post that has comments, and `@setNull` needs a `Maybe` field. The rule
sits on the referring field, so this file says what any page's deletion
of a post does to its comments.

Anyone may comment without logging in, so the name and the text are
bounded: without a bound, one visitor could store a megabyte under a
post for every reader to download. `Name` and `Body` are brands of `Text`
with a refinement (chapter 4); a brand's base is a built-in type, not
another brand, so `fits` checks for itself what `Text.NonEmpty` would.
The comment form uses these types, so a longer comment is refused with
an error beside the field before `update` runs.

## A slug: `Post`

```alder part=blog/Post
module Post

import User

type PostId = brand Text
type Slug = brand Text where (\s -> isSlug s)

table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User,
  created: Time @created,
}
```

A post's address is `/posts/hello-world`. The id is still generated, and
the slug is a second, `@unique` column. `Slug` is a refined brand too: a
form field of type `Post.Slug` reports an error before `update` runs,
and a route parameter that breaks the rule, `/posts/Bad%20Slug`, is a 404.

```alder part=blog/Post
bySlug : Slug -> Query (Maybe Post)
bySlug slug = Db.from Post |> Db.where (Post.slug.eq slug) |> Db.first

-- isSlug is the rule a Slug keeps: lowercase letters, digits and hyphens, at least one.
isSlug : Text -> Bool
isSlug s = Text.matches "^[a-z0-9-]+$" s

-- visible is whether the viewer (the session's user, if any) may see the post: everyone sees a
-- published post; only its author sees a draft.
visible : Maybe User.UserId -> Post -> Bool
visible viewer post = post.published || viewer == Just post.author
```

`bySlug` is the lookup by slug. `isSlug` is the rule, written once for
the brand and the first test below. `visible` is the rule for drafts;
`viewer` is a `Maybe` because a visitor need not log in.

```alder part=blog/Post
test "a slug is lowercase letters, digits and hyphens" =
  List.map isSlug ["hello-world", "post-2", "Hello", "a b", "", "x/y"] == [True, True, False, False, False, False]

test "a draft is not visible" = {
  let draft : Post = { id: PostId "p", slug: Slug "draft", title: Text.NonEmpty "Draft", body: "", published: False, author: User.UserId "author", created: Time.epoch }
  let seen = [visible Nothing draft, visible (Just (User.UserId "reader")) draft, visible (Just (User.UserId "author")) draft, visible Nothing { draft | published: True }]
  Task.succeed (seen == [False, False, True, True])
}
```

The second test builds a `Post` by hand, `created` included, with
`Time.epoch` as a fixed time (chapter 10).

## A role checked in `load`: `Admin` and `Admin.Edit`

`User` declares `type Role = Reader | Author` and a column
`role: Role = Reader`. A role is a column, not a language feature, and a
page that only authors may open checks it in `load`, with the one check
`User` has for every page to call:

```alder fragment
author : UserId -> Query (Maybe User)
author id = Db.from User |> Db.where (User.id.eq id) |> Db.where (User.role.eq Author) |> Db.first
```

`Admin` (`requires user`) answers `Query.notFound` when `User.author`
of the session's user is `Nothing`, so a Reader at `/admin` gets the
same 404 as a page that does not exist. The runtime runs `load` before
every message of the page (chapter 6), so `update` needs no check of its
own. Its message `Create` looks the slug up with `Post.bySlug` and
answers `Reject` if it is taken, where the insert would fail on
`@unique` with a 500 page. The edit page, in full:

```alder part=blog/Admin.Edit
module Admin.Edit

import Post
import Comment
import User
import Layout
import Posts.Index

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
```

`load` asks two things: the user is still an Author, so one made a
Reader can no longer edit old posts, and the post is theirs. Another
author's post is a 404, like a missing one.

```alder part=blog/Admin.Edit
form EditPost = {
  title: Text.NonEmpty,
  body: Text,
}

msg Save form EditPost
msg Publish
msg Unpublish
msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Post model.post.id (\post -> { post | title: p.form.title, body: p.form.body })
    Task.succeed Rerender
  },
  Publish p -> {
    let _ <- Db.update Post model.post.id (\post -> { post | published: True })
    Task.succeed Rerender
  },
  Unpublish p -> {
    let _ <- Db.update Post model.post.id (\post -> { post | published: False })
    Task.succeed Rerender
  },
  -- The post's comments go with it: Comment.post is `@ref Post @cascade`.
  Delete p -> {
    let _ <- Db.delete Post model.post.id
    Task.succeed (Redirect (Admin {}))
  },
}
```

No message carries an id: each acts on `model.post`, which `load` has
checked, so no request can name another post. This is chapter 6's
advice ("Authorize in `update` too"), and the blog follows it on every
page. `Delete` makes one write, and `@cascade` deletes the comments.

```alder part=blog/Admin.Edit
view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={"Edit: " ++ model.post.title}>
    <h1>{model.post.title}</h1>
    <p>
      {if model.post.published then "Published" else "Draft"}
      · <a href={Posts.Show { slug: model.post.slug }}>View</a>
    </p>

    <form submit={Save}>
      <Errors form />
      <Input field="title" label="Title" value={model.post.title} />
      <Errors field="title" />
      <Textarea field="body" label="Body (Markdown)" value={model.post.body} />
      <Errors field="body" />
      <button>Save</button>
    </form>

    {if model.post.published then
      <button click={Unpublish}>Unpublish</button>
    else
      <button click={Publish}>Publish</button>}
    <button click={Delete}>Delete</button>
    <a href={Admin {}}>All your posts</a>
  </Layout.Page>
```

```alder part=blog/Admin.Edit
test "deleting a post deletes its comments" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", published: True, author: author }
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

test "Publish shows the post on the front page; Unpublish hides it" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let draft <- load { id: id } { user: author }
  let _ <- update Publish draft { user: author }
  let published <- Posts.Index.load { page: 1, q: "" } { user: Nothing }
  let shown <- load { id: id } { user: author }
  let _ <- update Unpublish shown { user: author }
  let unpublished <- Posts.Index.load { page: 1, q: "" } { user: Nothing }
  Task.succeed ({ published: List.length published.posts, unpublished: List.length unpublished.posts } == { published: 1, unpublished: 0 })
}
```

The first test deletes a post with two comments through `update`. Its
trace has one step, `Db.delete Post`, since SQLite deletes the comments
in that statement (chapter 8, "Writing"). The next two wrap `load` in a
block, because `Test.run` takes a `Task` and `load` is a `Query`. The
last reads the front page through `Posts.Index.load`, which a test may
call once its module imports `Posts.Index`.

## A capability in `load`: `Posts.Show`

`app/Markdown.alder` declares `capability Markdown` with one entry,
`render : Text -> Query (Html Never)` (chapter 7 shows the module).
`Posts.Show`'s `load` finds the post with `Post.bySlug`, answers
`Query.notFound` when `Post.visible` says the session may not see it, so
a reader cannot tell a draft from an unused slug, renders the body, and
reads the comments oldest first. `view` embeds the body, an
`Html Never`, as `{model.body}` (chapter 5). The Go side builds it with
`html.El` and `html.Text`, never `html.Raw`, so a post cannot inject
markup; a link is kept only for an `http:` or `https:` address. That Go
is the adapter `examples/blog/adapters/markdown/markdown.go`: module
`Markdown`, capability `Markdown`, so directory `adapters/markdown/` and
constructor `func Markdown()` (chapter 7 quotes its top, and the
generated `caps` package that wires it).
`app/Markdown.alder` also has a `fake Markdown`, which one `Posts.Show`
test uses in the adapter's place (chapter 10, "Faking a capability").

The comment form's message, `msg AddComment form NewComment`, carries no
post id either. A draft's page is a 404 for everyone but its author, so
no one else can post its form, and `update` refuses a comment on a draft
anyway, with `Task.notFound`.

## Pages and a search: `Posts.Index`

```alder part=blog/Posts.Index
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
```

Both query parameters have defaults, so `/`, `/?page=2` and
`/?q=soup&page=2` reach this page (chapter 6); `/?page=abc` is a 404.

```alder part=blog/Posts.Index
-- load reads one page of published posts, newest first, matching `q` when it is not blank; `q`
-- is cut to 100 characters. It asks for one post more than a page holds: if that one comes back
-- there is a next page (`hasMore`), and it is dropped from the list. `canWrite` shows the link
-- to the Admin page to an Author only.
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
```

`load` keeps the page between 1 and `lastPage`, so a huge number cannot
overflow the offset. It asks for `perPage + 1` rows: if the extra one
comes back, there is an older page. A blank `q` gives `Db.and []`, which
matches every row; otherwise `Db.or` matches title or body (chapter 8,
"Search"). `q` is cut to 100 characters, since a search costs every
post's length times the query's and SQLite refuses a pattern over about
50 KB. `canWrite` is `User.author`, so a Reader never sees a 404 link.

```alder part=blog/Posts.Index
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
```

The search form's message redirects to the first page of results, so
`/?q=soup` can be bookmarked. The form is `SearchForm`, not `Query`: a
`form` declares a type, and one named like a Prelude type would hide it
in its module, so the compiler refuses the name:

```alder error=E0012 "`Query` is a Prelude type; name the form differently"
module Search

form Query = {
  q: Text,
}

count : Query Int
count = Query.succeed 0
```

`count`'s `Query Int` would be the next error, and it is not reported:
a module's other errors wait until the name is changed.

```alder part=blog/Posts.Index
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
```

## Its tests

`alder test examples/blog` runs twenty-nine tests in six modules:

```text
$ alder test examples/blog
ok   Post "a slug is lowercase letters, digits and hyphens"
ok   Post "a draft is not visible"
ok   Admin "a Reader cannot open the admin page"
ok   Admin "creating a post redirects to its edit page"
ok   Admin "a taken slug is refused"
ok   Admin "the admin page needs an Author"
ok   Admin "an Author logs out from the admin page"
ok   Comment "a name or comment has text and a bound"
ok   Posts.Index "the front page lists only published posts, newest first"
ok   Posts.Index "search matches title or body"
ok   Posts.Index "a `%` in the query is literal"
ok   Posts.Index "page 99 is empty"
ok   Posts.Index "a negative page is page 1"
ok   Posts.Index "a `_` in the query is literal"
ok   Posts.Index "a long query is cut"
ok   Posts.Index "a search answers with the first page of its results"
ok   Posts.Index "only an Author sees the Write link"
ok   Posts.Index "the second page links back to the first"
ok   Posts.Index "a search through the form"
ok   Admin.Edit "deleting a post deletes its comments"
ok   Admin.Edit "another author's post is not found"
ok   Admin.Edit "a demoted author cannot edit"
ok   Admin.Edit "Publish shows the post on the front page; Unpublish hides it"
ok   Posts.Show "a draft is not found for a reader"
ok   Posts.Show "a draft is shown to its author"
ok   Posts.Show "a post's body is rendered through the Markdown capability"
ok   Posts.Show "a comment on a draft is refused"
ok   Posts.Show "a comment is listed oldest first"
ok   Posts.Show "an empty comment is rejected"
29 passed, 0 failed
```

The `Posts.Index` tests call `load` the way the runtime does, with
parameters and a session, against rows the test inserted:

```alder part=blog/Posts.Index
test "the front page lists only published posts, newest first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", published: True, author: author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
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
```

The last two go through the page as a browser would (chapter 10,
"Testing through the browser"). The first opens the first and second
pages and reads the links the `nav` renders: `Test.attr "href"
"a[rel=next]"` is the `href` of the first `<a rel="next">`, and the
second page's "Newer" link is `/`, the first page with both parameters at
their defaults, which the URL leaves out. The second posts the search form
with `Test.submit`, follows the redirect to `/?q=tomato` and reads the
titles listed there and the query kept in the search box:

```alder part=blog/Posts.Index
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
```

The first test can check that `Second` comes before `First` because a
test's clock ticks (chapter 10): each `Db.insert` reads it for `created`,
one second later each time. Changing `desc` to `asc` in `load` fails
this test, and flipping it in `Posts.Show` fails "a comment is listed
oldest first".

Three more tests go through the browser. `Admin`'s "the admin page needs
an Author" opens `/admin` as a visitor (`Redirected (Auth.Login {})`:
`requires user` sends them to log in) and as a logged-in Reader
(`PageMissing`: `load`'s 404); "an Author logs out from the admin page"
clicks the page's `App.Logout` button and finds the session's `user`
gone. `Posts.Show`'s "an empty comment is rejected" posts the comment
form with an empty body through `Test.submitRaw`, which sends text as
typed, and gets `FormRejected [{ field: "body", message: "is not valid" }]`:
`Comment.Body` is a brand with a `where` rule, and a value its rule
refuses is "is not valid" (a `Text.NonEmpty` field would say "must not
be empty").

## Run it

```text
$ alder run examples/blog
```

`alder run` serves the app in dev mode (`ALDER_DEV=1`, chapter 11). In
dev mode only, the stub of `Password` (`adapters/auth/password.go`,
`func LoginPassword()`, as for the to-do app) accepts
two accounts, both with the password `password`, and creates each one's
`User` row on its first login:

- `author@example.com` is an Author: "Write" on the front page opens
  `/admin`, to create, publish and delete posts.
- `reader@example.com` is a Reader: it can comment, and every `/admin`
  page is a 404 for it.

Outside dev mode the stub refuses every login and logs that it must be
replaced, as it must before the blog faces the internet.

The blog's tables come from `examples/blog/migrations/` (chapter 8,
"Migrations"): `0001_initial.sql` creates them, and `alder run` and
`alder test` apply it. After a change to a `table`, run `alder migrate`;
until then `alder check` reports E0019.
