# 14. A second app

The to-do app of chapter 2 has one table that matters and one kind of
user. `examples/blog` is the second example, and it adds the things a
to-do list never needed. It has two tables joined by a reference, two
kinds of user, a capability that `load` calls, pages of results, and a
search. This chapter covers only what is new. The rules themselves are in
chapters 6 to 10, and the chapter points back to them.

The blog has eleven modules in `examples/blog/app/`:

| Module | What it is |
|---|---|
| `App`, `Layout`, `Auth.Login` | As in the to-do app. The login page redirects to the front page |
| `User` | The user table, with a `role` column |
| `Post`, `Comment` | The two tables, and the functions about posts |
| `Markdown` | A capability that turns a post's body into HTML |
| `Posts.Index` | `/`: the published posts, a page at a time, with a search |
| `Posts.Show` | `/posts/{slug}`: one post, its comments, and a comment form |
| `Admin` | `/admin`: an author's own posts, and a form for a new one |
| `Admin.Edit` | `/admin/{id}`: edit, publish, unpublish or delete one post |

Four modules are shown in full below, piece by piece. `go test ./book`
joins the pieces of each one and checks that they equal the file in
`examples/blog`, ignoring blank lines (chapter 11, "The book is
verified"). The other modules are described in the text. Their files
are short.

## A relation: `Comment`

```alder part=blog/Comment
module Comment

import Post

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}
```

A comment belongs to a post: `post: Post.PostId @ref Post` names a `Post`
row, and inserting a comment for a post that does not exist fails
(chapter 8, "Writing"). The new part is `@cascade`, the field's **delete
rule**. It says what happens to a comment when its post is deleted.
`@cascade` deletes the comments too. Without a rule, the default
`@restrict` would refuse to delete a post that has comments, and
`@setNull` would need the field to be a `Maybe`. The rule sits on the
referring field, so `Comment.alder` is the one place that says what a
deletion does, whichever page deletes the post. `Admin.Edit`, below,
deletes a post with a single `Db.delete`.

`Comment` has no functions of its own. The page that shows a post reads
that post's comments.

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

A post's address is `/posts/hello-world`, not `/posts/<id>`. The id is
still generated, like every `@id` (chapter 8), and the slug is a second,
`@unique` column. `Slug` is a brand of `Text` with a refinement
(chapter 4): a slug that breaks the rule cannot be made. A form field of
type `Post.Slug` reports an error before `update` runs, and so does a
route parameter: `/posts/Bad%20Slug` is a 404.

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

`bySlug` is the lookup by slug, and both `Posts.Show` and `Admin` use it.
`isSlug` is the rule, written once and used both by the brand and by the
first test below. `visible` is the rule for drafts. Everyone can see a
published post, but only its author can see a draft. `viewer` is a
`Maybe` because a visitor who has not logged in can still read the blog.

```alder part=blog/Post
test "a slug is lowercase letters, digits and hyphens" =
  List.map isSlug ["hello-world", "post-2", "Hello", "a b", "", "x/y"] == [True, True, False, False, False, False]

test "a draft is not visible" = {
  let draft : Post = { id: PostId "p", slug: Slug "draft", title: Text.NonEmpty "Draft", body: "", published: False, author: User.UserId "author", created: Time.epoch }
  let seen = [visible Nothing draft, visible (Just (User.UserId "reader")) draft, visible (Just (User.UserId "author")) draft, visible Nothing { draft | published: True }]
  Task.succeed (seen == [False, False, True, True])
}
```

The two tests are pure (chapter 10, section 1). The second one builds a
`Post` record by hand, so it writes every field, `created` included, with
`Time.epoch` as a fixed time, as chapter 10's tests do.

## A role checked in `load`: `Admin` and `Admin.Edit`

`User` declares `type Role = Reader | Author` and a column
`role: Role = Reader`, so a user is a Reader unless the row says
otherwise. Roles are not a language feature. A role is a column, and a
page that only authors may open checks that column in `load`. The page
`Admin` (`app/Admin.alder`) declares `requires user`, reads the
session's `User` row, and continues only when `User.isAuthor user` is
true. In every other case the answer is `Query.notFound`, so a Reader at
`/admin` gets the same 404 as a page that does not exist. The runtime
runs `load` before every message of the page (chapter 6, "The request
lifecycle"), so `Admin`'s `update` needs no check of its own. Its one
message, `Create`, looks the new slug up with `Post.bySlug` before it
inserts. If the slug is taken, `Create` answers with `Reject`, and the
form shows the message. Without that check, the insert would fail on
`@unique` and the user would get a 500 page. On success, `Create`
redirects to the new post's edit page.

That edit page is shown here in full:

```alder part=blog/Admin.Edit
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
  let me <- Db.get User session.user
  let found <- Db.get Post params.id
  let isAuthor = Maybe.withDefault False (Maybe.map User.isAuthor me)
  match found {
    Just post -> if isAuthor && post.author == session.user then Query.succeed { post: post } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
```

This `load` checks two things. The user must still be an Author, which
is the same check as on `Admin`, so an author who was made a Reader can
no longer edit their old posts. The post must also belong to that
author. Another author's post gets a 404, just like a missing post.

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

None of the four messages carries an id. Each one acts on `model.post`,
which `load` has already checked, so no request can name a different
post. This is chapter 6's advice ("Authorize in `update` too"): when the
page's own parameter names the row, let the messages carry nothing. The
blog follows it on every page, so it needs no `owned` helper like the
to-do list's. `Delete` makes one write, and `@cascade` deletes the
post's comments with it.

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

`view` shows Publish or Unpublish, depending on the post's state. The
module ends with its tests:

```alder part=blog/Admin.Edit
test "deleting a post deletes its comments" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", published: True, author: author }
  let _ <- Db.insert Comment { post: id, author: Text.NonEmpty "Ann", body: Text.NonEmpty "First!" }
  let _ <- Db.insert Comment { post: id, author: Text.NonEmpty "Bob", body: Text.NonEmpty "Second." }
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
```

The first of the three tests checks the cascade. It inserts a post and
two comments, deletes the post through `update`, and counts the rows that remain. The
trace has one step, `Db.delete Post`, because SQLite deletes the
comments inside that same statement (chapter 8, "How tables are
stored"). The other two check the rules in `load`: another author's post, and an
author made a Reader after writing the post. A failing
`load` cannot be passed to `Test.run` directly: `Test.run` takes a
`Task`, and `load` is a `Query`. Writing `Test.run (load …)` is E0003,
and the message itself says what to do: "a Query runs inside a test's
block: `let x <- q`; `Test.run` takes a Task". So these tests wrap the
query in a block. A block that binds a `Query` and ends in
`Task.succeed` is a `Task`. When `load` answers `Query.notFound`, the
block fails, and `Test.run` reports the failure as `Err "not found"`.
The third test runs `load` twice in this way, once before and once after
a `Db.update` of the author's role.

## A capability in `load`: `Posts.Show`

`app/Markdown.alder` declares the blog's second capability:

```alder
capability Markdown {
  render : Text -> Query (Html Never),
}
```

Chapter 7 shows the whole module. `render` returns a
`Query`, so it is a read, and `load` may call it. `Posts.Show`'s `load`
(`app/Posts/Show.alder`) finds the post with `Post.bySlug params.slug`.
If `Post.visible session.user post` is false, the answer is
`Query.notFound`, so a reader cannot tell a draft from a slug that was
never used. Otherwise `load` writes `let body <- Markdown.render
post.body` and reads the comments, oldest first. The capability's name
is the same as its module's name, and that works: from another module,
`Markdown.render` names the capability's entry, because the module
`Markdown` has no value called `render`.

The Model keeps the rendered body as `body: Html Never`. That is markup
that sends no message. The page's `view` returns `Html Msg` and embeds
the body as `{model.body}`, with no conversion, because `Html Never`
fits any `Html a` (chapter 5). The Go side (`adapters/markdown.go`)
builds the HTML element by element with `html.El` and `html.Text`, and
never uses `html.Raw`. Every piece of text is escaped when the page is
rendered, so a post cannot inject markup. A link is kept only for an
`http:` or `https:` address. Any other link, such as `javascript:`,
stays as the literal text the author typed.

The comment form's message, `msg AddComment form NewComment`, carries no
post id either: the page's slug names the post. A draft's page is a 404
for everyone but its author, so its form cannot be posted. `update` also
refuses a comment on a draft, whoever sends it, with `Task.notFound`.

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
`/?q=soup&page=2` all reach this page (chapter 6). `/?page=abc` is a 404,
because `abc` is not an `Int`. `canWrite` decides whether the page shows
the link to `Admin`.

```alder part=blog/Posts.Index
-- load reads one page of published posts, newest first, matching `q` when it is not blank. It
-- asks for one post more than a page holds: if that one comes back there is a next page
-- (`hasMore`), and it is dropped from the list. `canWrite` shows the link to the Admin page to
-- an Author only.
load : Params -> Session -> Query Model
load params session = {
  let page = if params.page < 1 then 1 else if params.page > lastPage then lastPage else params.page
  let q = Text.trim params.q
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
    Just id -> Db.get User id,
    Nothing -> Query.succeed Nothing,
  }
  let canWrite = Maybe.withDefault False (Maybe.map User.isAuthor me)
  Query.succeed { page: page, q: q, posts: List.take perPage found, hasMore: List.length found > perPage, canWrite: canWrite }
}
```

`load` keeps the page number between 1 and `lastPage`, so `/?page=0`
shows the first page and a huge number cannot overflow the offset. A pagination step is `Db.limit` and `Db.offset` (chapter 8).
`load` asks for `perPage + 1` rows: if the extra row comes back, there
is an older page, and the extra row is dropped from the list.
The search is a condition built before the query runs. `load` trims
`q`, and a blank `q` gives `Db.and []`, which matches every row. Otherwise `Db.or` matches
posts whose title or body contains `q`. `.contains` ignores the case of
A–Z and matches `%` and `_` literally (chapter 8, "Search"). It is SQL's
`LIKE`, not full-text search. Last, `load` reads the session's `User`
row, if there is a session user, and sets `canWrite` only for an Author.
A Reader therefore never sees a link to a page that would be a 404.

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

The search box is an ordinary form (chapter 6), and its message answers
with a `Redirect` to the first page of results. The results therefore
have an address, `/?q=soup`, that a reader can bookmark, and the Newer
and Older links keep `q`. The form is named `SearchForm`, not `Query`.
A `form` declares a type, and a type named `Query` in this module would
hide the effect type `Query`: `load : … -> Query Model` would then be
E0008, "`Query` takes 0 type arguments".

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

`alder test examples/blog` runs the fifteen tests of five modules:

```text
$ alder test examples/blog
ok   Post "a slug is lowercase letters, digits and hyphens"
ok   Post "a draft is not visible"
ok   Admin "a Reader cannot open the admin page"
ok   Admin "creating a post redirects to its edit page"
ok   Admin.Edit "deleting a post deletes its comments"
ok   Admin.Edit "another author's post is not found"
ok   Admin.Edit "a demoted author cannot edit"
ok   Posts.Index "the front page lists only published posts, newest first"
ok   Posts.Index "search matches title or body"
ok   Posts.Index "a `%` in the query is literal"
ok   Posts.Index "page 99 is empty"
ok   Posts.Show "a draft is not found for a reader"
ok   Posts.Show "a draft is shown to its author"
ok   Posts.Show "a comment on a draft is refused"
ok   Posts.Show "a comment is listed oldest first"
15 passed, 0 failed
```

The tests that end `Posts.Index` call `load` the way the runtime does,
with parameters and a session, against rows the test inserted:

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
```

Look at the first test. It inserts a user and three posts, and the
middle post is a draft (`published` defaults to `False`). Then it checks
the titles `load` returns: the draft is missing, and `Second` comes
before `First`. The order can only be checked because a test's clock
ticks (chapter 10, "Where an effect test runs"). Each `Db.insert` reads
the clock to fill `created`, and each read is one second later than the
one before. So `Second` is newer than `First` by a second, and
`orderBy Post.created.desc` has something to sort. With a clock that
never moved, every row would have the same `created`, and the test
could not tell `desc` from `asc`. Changing `desc` to `asc` in `load`
makes this test fail, and making the same change in `Posts.Show` makes
"a comment is listed oldest first" fail.

"page 99 is empty" makes eleven posts in one step (`Task.all`) and then
loads four pages in one step (`Query.all`). It checks pages 1, 2, 99 and
0 together, and page 0 comes back as page 1. "a draft is shown to its
author" in `Posts.Show` runs the real Markdown adapter. Its body
`Not *yet*.` comes back equal to the literal `<p>Not <em>yet</em>.</p>`,
because `Html` values can be compared with `==` (chapter 10, section 1). The Go
adapter has its own Go tests in `adapters/markdown_test.go`, including
hostile input: scripts, `javascript:` links, unclosed markers, and 50 KB
bodies.

## Run it

```text
$ alder run examples/blog
```

As in chapter 11 ("`alder run`"), the dev server prints its address.
Open `http://localhost:8080/`. The front page is empty until someone
writes a post. The development stub of `Password`
(`adapters/password.go`) accepts two accounts, and both use the password
`password`:

- `author@example.com` is an Author. It can open `/admin` ("Write" on
  the front page after logging in), create a draft, publish it, and
  delete it.
- `reader@example.com` is a Reader. It can comment on a published post.
  For this account, `/admin` and every `/admin/{id}` are a 404.

The stub creates each account's `User` row the first time that account
logs in, with its role. Try a draft: it shows to its author with a
"Draft" note, and logged out the same address is a 404. Replace the stub
before the blog faces the internet.
