# 14. A second app

The to-do app of chapter 2 has one table that matters and one kind of
user. `examples/blog` adds what a to-do list never needed: two tables
joined by a reference, two kinds of user, a capability that `load`
calls, pages of results, and a search. This chapter covers only what is
new; the rules are in chapters 6 to 10.

The blog has eleven modules of its own in `examples/blog/app/`, and
the seventeen of a site admin that `alder new admin` generated in
`app/Admin/` (chapter 15 tours them):

| Module | What it is |
|---|---|
| `App`, `Layout`, `Auth.Login` | As in the to-do app, but the session also holds the user's role, which the login copies from the user's row; the login leads to `/` |
| `User` | The user table, with a `role` column |
| `Post`, `Comment` | The two tables, and the functions about posts |
| `Markdown` | A capability that turns a post's body into HTML |
| `Posts.Index` | `/`: the published posts, a page at a time, and a search |
| `Posts.Show` | `/posts/{slug}`: one post, its comments, a comment form |
| `Write` | `/write`: an author's own posts, and a form for a new one |
| `Write.Edit` | `/write/{id}`: edit, publish, unpublish or delete a post |
| `Admin.…` | `/admin`: every table's rows, to list, search, create, edit and delete, for an Admin; generated (chapter 15) |

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
  Task.succeed (
    List.map (fits 80) ["Ann", "", "   ", Text.slice 0 80 long, long]
      == [True, False, False, True, False]
  )
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

-- `author` is the table's `@owner`: on the Write pages, `Db.mine Post`, `Db.getMine Post` and
-- `Db.insertMine Post` read and write only the signed-in author's posts (book 06, "Rules").
table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User @owner,
  created: Time @created,
}
```

A post's address is `/posts/hello-world`. The id is still generated, and
the slug is a second, `@unique` column. `Slug` is a refined brand too: a
form field of type `Post.Slug` reports an error before `update` runs,
and a route parameter that breaks the rule, `/posts/Bad%20Slug`, is a 404.
`author` is the post's owner, and `@owner` says so to the compiler: the
pages where an author manages their posts read and write them through
`Db`'s scoped operations (chapter 6, "Rules"). The public pages, which
show anyone's published posts, use `Db.from` as before.

```alder part=blog/Post
bySlug : Slug -> Query (Maybe Post)
bySlug slug =
  Db.from Post
    |> Db.where (Post.slug.eq slug)
    |> Db.first

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
  List.map isSlug ["hello-world", "post-2", "Hello", "a b", "", "x/y"]
    == [True, True, False, False, False, False]

test "a draft is not visible" = {
  let draft : Post = {
    id: PostId "p",
    slug: Slug "draft",
    title: Text.NonEmpty "Draft",
    body: "",
    published: False,
    author: User.UserId "author",
    created: Time.epoch,
  }
  let seen = [
    visible Nothing draft,
    visible (Just (User.UserId "reader")) draft,
    visible (Just (User.UserId "author")) draft,
    visible Nothing { draft | published: True },
  ]
  Task.succeed (seen == [False, False, True, True])
}
```

The second test builds a `Post` by hand, `created` included, with
`Time.epoch` as a fixed time (chapter 10).

The last two were added when `alder test --mutate` (chapter 10,
"Mutation testing") showed that no test noticed `bySlug` losing its
`where`, or `author` becoming `@ref User @cascade`. The first inserts two
posts and asks for the second, which a `bySlug` without the `where` would
not find first. The second deletes an author who has a post: under the
default rule, `@restrict`, the delete fails and the post stays, so
`Result.toMaybe` of the result is `Nothing`. The error's text names a
generated id, which differs from run to run, so the test does not
compare it.

```alder part=blog/Post
test "bySlug finds the post with that slug" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <- Db.insert Post {
    slug: Slug "first",
    title: Text.NonEmpty "First",
    body: "",
    author: author,
  }
  let _ <- Db.insert Post {
    slug: Slug "second",
    title: Text.NonEmpty "Second",
    body: "",
    author: author,
  }
  let found <- bySlug (Slug "second")
  Task.succeed (Maybe.map (\p -> p.title) found == Just (Text.NonEmpty "Second"))
}

-- `author` is `@ref User` with the default rule, `@restrict`: an author who still has posts
-- cannot be deleted, and the posts stay.
test "an author with posts cannot be deleted" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <- Db.insert Post {
    slug: Slug "hello",
    title: Text.NonEmpty "Hello",
    body: "",
    author: author,
  }
  let r <- Test.run (Db.delete User author)
  let posts <- Db.from Post |> Db.count
  Task.succeed ({ deleted: Result.toMaybe r.value, posts: posts } == { deleted: Nothing, posts: 1 })
}
```

## A role in the session: `Write` and `Write.Edit`

`User` declares `type Role = Reader | Author | Admin` and a column
`role: Role = Reader`. (`Admin` is the generated admin's role, chapter
15; the author's pages are `Write`, at `/write`, so that the admin can
have `/admin`.) The role a page asks for is the one in the
session: `App` declares the session with a `role` field beside `user`,

```alder fragment
session { user: Maybe User.UserId, role: User.Role = Reader }
```

and the login page copies the user's role from their row when it stores
them in the session:

```alder fragment
      Just id -> {
        let found <- Db.get User id
        let role = Maybe.withDefault Reader (Maybe.map (\u -> u.role) found)
        let _ <- Session.put (\s -> { s | user: Just id, role: role })
        Task.succeed (Redirect (Posts.Index {}))
      },
```

`App.Logout` sets it back to `Reader` with the user (`{ s | user:
Nothing, role: Reader }`). Both Write pages declare

```alder fragment
page "/write" { } requires user, role Author
```

`requires user` sends a visitor to log in, as before; `role Author` then
answers a 404 to a logged-in Reader, the same 404 as a page that does not
exist, before `load` runs. The check reads the cookie and nothing else,
so it costs no query. It is the role at login, and the cookie keeps it
until logout or the session's 30 days: a user demoted since still passes
it. So both pages also re-read the user's row in `load` (chapter 6,
"Rules"; a test below shows it). Every message of the page goes through the same check, so
`update` needs none of its own.

`Write`'s `load` lists the author's own posts with `Db.mine Post |>
Db.orderBy Post.created.desc |> Db.all`, and its message `Create` stores
a new one with `Db.insertMine Post { slug: …, title: …, body: … }`, which
fills `author` from the session. `Create` first looks the slug up with
`Post.bySlug` and answers `Reject` if it is taken, where the insert would
fail on `@unique` with a 500 page. The edit page, in full:

```alder part=blog/Write.Edit
module Write.Edit

import Auth.Login
import Comment
import Layout
import Post
import Posts.Index
import User

page "/write/{id}" { id: Post.PostId } requires user, role Author

type Model = { post: Post }

-- load finds the post when the session's user wrote it: `Db.getMine` answers anyone else's
-- post, like a missing one, with a 404. The page `requires user, role Author`, the same gate as
-- the Write page, and re-reads the user's row as it does (User.isAuthor), so an author made a
-- Reader is turned away at once. The messages carry no id: they act on `model.post`, which this
-- `load` has found (as the to-do example's Todos.Show does), and every message runs it first; the
-- writes are the scoped forms all the same, which cost nothing.
load : Params -> Session -> Query Model
load params session = {
  let author <- User.isAuthor session.user
  if author then {
    let post <- Db.getMine Post params.id
    Query.succeed { post: post }
  } else
    Query.notFound
}
```

`load` first re-reads the user's row with `User.isAuthor`, the re-read
idiom of chapter 6, whose body is

```alder fragment
isAuthor : UserId -> Query Bool
isAuthor id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Author)
}
```

The role check already passed, but on the cookie's role, which is the
one read at login and lasts until logout (or the session's 30 days). The
re-read makes a role taken away in the database apply at once, for one
small query. Then `Db.getMine Post params.id` is the post when its
`author` is the session's user, and not found otherwise, so another
author's post is a 404, like a missing one. (`Auth.Login` is imported
for a test below, which logs in through its form.)

```alder part=blog/Write.Edit
form EditPost = { title: Text.NonEmpty, body: Text }

msg Save form EditPost

msg Publish

msg Unpublish

msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.updateMine Post model.post.id (\post -> { post |
      title: p.form.title,
      body: p.form.body
    })
    Task.succeed (Notice.with "Saved" Rerender)
  },
  Publish p -> {
    let _ <- Db.updateMine Post model.post.id (\post -> { post | published: True })
    Task.succeed Rerender
  },
  Unpublish p -> {
    let _ <- Db.updateMine Post model.post.id (\post -> { post | published: False })
    Task.succeed Rerender
  },
  -- The post's comments go with it: Comment.post is `@ref Post @cascade`.
  Delete p -> {
    let _ <- Db.deleteMine Post model.post.id
    Task.succeed (Redirect (Write {}))
  },
}
```

No message carries an id: each acts on `model.post`, which `load` has
found with `Db.getMine`, so no request can name another post. The writes
are still the scoped `Db.updateMine` and `Db.deleteMine`: they cost
nothing, and `alder test --mutate` counts them as the same program as
the plain ones here, so they ask for no extra test (chapter 6, "Rules").
`Delete` makes one write, and `@cascade` deletes the comments.

```alder part=blog/Write.Edit
view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={"Edit: " ++ model.post.title}>
    <h1>{model.post.title}</h1>
    <p>
      {if model.post.published then <al-badge>Published</al-badge> else <al-badge variant="secondary">Draft</al-badge>} · <a href={Posts.Show { slug: model.post.slug }}>View</a>
    </p>

    <al-card>
      <form submit={Save}>
        <al-alert variant="destructive"><Errors form /></al-alert>
        <al-input field="title" label="Title" value={model.post.title} />
        <al-textarea field="body" label="Body (Markdown)" value={model.post.body} />
        <al-button type="submit">Save</al-button>
      </form>
      <div slot="actions">
        {if model.post.published then
          <al-button variant="secondary" click={Unpublish}>Unpublish</al-button>
        else
          <al-button variant="secondary" click={Publish}>Publish</al-button>}
        <al-button variant="destructive" click={Delete}>Delete</al-button>
      </div>
    </al-card>
    <a href={Write {}}>All your posts</a>
  </Layout.Page>
```

The page is built from the design system's elements (Appendix C): the
status is an `<al-badge>`, the form sits in an `<al-card>` whose
`actions` slot holds the buttons, and the form's own error (a `Reject`)
appears in an `<al-alert variant="destructive">`, which Alder gives
`role="alert"` so a screen reader announces it. The design system hides
that alert while its list is empty (`al-alert:has(> ul.errors:empty)
{ display: none }`, in `al.css` since alder-ui v0.1.2).

```alder part=blog/Write.Edit
test "deleting a post deletes its comments" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let id <- Db.insert Post {
    slug: Post.Slug "hello",
    title: Text.NonEmpty "Hello",
    body: "",
    published: True,
    author: author,
  }
  let _ <- Db.insert Comment {
    post: id,
    author: Comment.Name "Ann",
    body: Comment.Body "First!",
  }
  let _ <- Db.insert Comment {
    post: id,
    author: Comment.Name "Bob",
    body: Comment.Body "Second.",
  }
  let _ <- Test.login { user: Just author, role: Author }
  let model <- load { id: id } { user: author, role: Author }
  let r <- Test.run (update Delete model { user: author, role: Author })
  let posts <- Db.from Post |> Db.count
  let comments <- Db.from Comment |> Db.count
  Task.succeed (
    {
      value: r.value,
      trace: r.trace,
      posts: posts,
      comments: comments,
    } == {
        value: Ok (Redirect (Write {})),
        trace: ["Db.deleteMine Post"],
        posts: 0,
        comments: 0,
      }
  )
}

-- Another author is let through the role check, but `Db.getMine` finds no post of theirs with
-- this id.
test "another author's post is not found" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
    role: Author,
  }
  let id <- Db.insert Post {
    slug: Post.Slug "hello",
    title: Text.NonEmpty "Hello",
    body: "",
    author: author,
  }
  let _ <- Test.login { user: Just other, role: Author }
  let r <- Test.open (Write.Edit { id: id })
  Task.succeed (r.answer == PageMissing)
}

-- The role is the one the session got at login, and the cookie keeps it until logout (or the
-- session's 30 days). An author made a Reader since then still passes `requires user, role
-- Author`, but `load` re-reads their row (User.isAuthor): the page is a 404 at once, and so is
-- every message, since each runs `load` first. After a new login the cookie says Reader too.
test "a demoted author is turned away at once" = {
  let _ <- Test.fake Login.Password
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let id <- Db.insert Post {
    slug: Post.Slug "hello",
    title: Text.NonEmpty "Hello",
    body: "",
    author: author,
  }
  let _ <- Test.get (Auth.Login {})
  let _ <- Test.submit Auth.Login.Credentials {
    email: Text.Email "author@example.com",
    password: Text.Secret "secret",
  }
  let before <- Test.open (Write.Edit { id: id })
  let _ <- Db.update User author (\u -> { u | role: Reader })
  let stale <- Test.open (Write.Edit { id: id })
  let _ <- Test.get (Auth.Login {})
  let _ <- Test.submit Auth.Login.Credentials {
    email: Text.Email "author@example.com",
    password: Text.Secret "secret",
  }
  let fresh <- Test.open (Write.Edit { id: id })
  Task.succeed (
    {
      before: before.answer,
      stale: stale.answer,
      fresh: fresh.answer,
    } == {
        before: Shown,
        stale: PageMissing,
        fresh: PageMissing,
      }
  )
}

test "a promoted reader logs in again before editing" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
    role: Author,
  }
  let id <- Db.insert Post {
    slug: Post.Slug "hello",
    title: Text.NonEmpty "Hello",
    body: "",
    author: me,
  }
  let _ <- Test.login { user: Just me, role: Reader }
  let r <- Test.open (Write.Edit { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "Publish shows the post on the front page; Unpublish hides it" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let id <- Db.insert Post {
    slug: Post.Slug "hello",
    title: Text.NonEmpty "Hello",
    body: "",
    author: author,
  }
  let _ <- Test.login { user: Just author, role: Author }
  let draft <- load { id: id } { user: author, role: Author }
  let _ <- update Publish draft { user: author, role: Author }
  let published <- Index.load { page: 1, q: "" } { user: Nothing, role: Reader }
  let shown <- load { id: id } { user: author, role: Author }
  let _ <- update Unpublish shown { user: author, role: Author }
  let unpublished <- Index.load { page: 1, q: "" } { user: Nothing, role: Reader }
  Task.succeed (
    { published: List.length published.posts, unpublished: List.length unpublished.posts }
      == { published: 1, unpublished: 0 }
  )
}
```

The first test deletes a post with two comments through `update`. Its
trace has one step, `Db.deleteMine Post`, since SQLite deletes the comments
in that statement (chapter 8, "Writing"). It calls `load` and `update`
directly, with a session record, but `Db.getMine` in `load` acts for the
test's own session, the one `Test.login` set, not for that record
(chapter 6, "Rules"); so the test logs in first. The second opens
another author's post as that author: the role check passes, and
`Db.getMine` answers `PageMissing`.

The third is the stale role. The author logs in through the login form
(with `Auth.Login`'s `fake Password`, chapter 10), which stores their
role, `Author`, in the session, and the edit page opens. Then their row
is changed to `Reader`. The cookie still says Author, so the role check
passes, but `load` re-reads the row and the page is a 404 (`stale` is
`PageMissing`) at once. After a second login the session holds the row's
role too. The fourth is the other way round: a row made Author whose
cookie still says Reader is turned away by the role check, until they
log in again (without it, the re-read alone would let them in, and
`alder test --mutate` reports the `role dropped` mutant as surviving).
The last test reads the front page through `Index.load`, which a
test may call once its module imports `Posts.Index`.

Two more go through the page, and were added after `alder test --mutate`
showed what nothing checked: that a visitor who opens an edit page is
sent to log in (with `requires user` deleted, the page would be served to
them), and that Save stores both fields. The second posts the form and
reads the page it re-renders: the heading and the text area now hold
what was saved, and the post's status is still the `<al-badge>` reading
Draft (a re-render replaces the page's `<body>`, custom elements and
all).

```alder part=blog/Write.Edit
-- The page through the browser: a visitor is sent to log in; the author saves the form, and the
-- page rerenders with its design-system elements (the status is an <al-badge>).
test "a visitor is sent to log in" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let id <- Db.insert Post {
    slug: Post.Slug "hello",
    title: Text.NonEmpty "Hello",
    body: "",
    author: author,
  }
  let visitor <- Test.open (Write.Edit { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "saving changes the title and the body" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let id <- Db.insert Post {
    slug: Post.Slug "hello",
    title: Text.NonEmpty "Hello",
    body: "",
    author: author,
  }
  let _ <- Test.login { user: Just author, role: Author }
  let _ <- Test.get (Write.Edit { id: id })
  let r <- Test.submit EditPost { title: Text.NonEmpty "Hello again", body: "More." }
  Task.succeed (
    {
      answer: r.answer,
      heading: Test.text "h1" r.doc,
      body: Test.text "textarea[name=body]" r.doc,
      status: Test.text "al-badge" r.doc,
      notices: r.notices,
    } == {
        answer: Rerendered,
        heading: Just "Hello again",
        body: Just "More.",
        status: Just "Draft",
        notices: ["Saved"],
      }
  )
}
```

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

import Layout
import Post
import User

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
-- to the Write page to an Author only.
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
  let canWrite = session.role == Author
  Query.succeed {
    page: page,
    q: q,
    posts: List.take perPage found,
    hasMore: List.length found > perPage,
    canWrite: canWrite,
  }
}
```

`load` keeps the page between 1 and `lastPage`, so a huge number cannot
overflow the offset. It asks for `perPage + 1` rows: if the extra one
comes back, there is an older page. A blank `q` gives `Db.and []`, which
matches every row; otherwise `Db.or` matches title or body (chapter 8,
"Search"). `q` is cut to 100 characters, since a search costs every
post's length times the query's and SQLite refuses a pattern over about
50 KB. `canWrite` is the session's role, so only an Author is shown the
Write link, and a Reader never sees a link to a 404.

```alder part=blog/Posts.Index
form SearchForm = { q: Text }

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
      <al-input field="q" label="Search" value={model.q} />
      <al-alert variant="destructive"><Errors form /></al-alert>
      <al-button type="submit" variant="secondary">Search</al-button>
    </form>

    {if List.isEmpty model.posts then
      <p class="empty">
        No posts{if Text.isEmpty model.q then "" else " match “" ++ model.q ++ "”"}.
      </p>
    else
      <ul class="posts">
        {for post in model.posts ->
          <li>
            <a href={Posts.Show { slug: post.slug }}>{post.title}</a>
            <span class="date">{Time.format "Jan 2, 2006" post.created}</span>
          </li>}
      </ul>}

    <al-pagination role="navigation" aria-label="Pages">
      {if model.page > 1 then
        <a slot="prev" href={Posts.Index { page: model.page - 1, q: model.q }} rel="prev">Newer</a>
      else
        <span slot="prev" aria-disabled="true">Newer</span>}
      <span>Page {Text.fromInt model.page}</span>
      {if model.hasMore then
        <a slot="next" href={Posts.Index { page: model.page + 1, q: model.q }} rel="next">Older</a>
      else
        <span slot="next" aria-disabled="true">Older</span>}
    </al-pagination>

    {match session.user {
      Just _ ->
        <div class="account">
          {if model.canWrite then <a href={Write {}}>Write</a> else Html.none} <al-button variant="outline" size="sm" click={App.Logout}>Log out</al-button>
        </div>,
      Nothing -> <p><a href={Auth.Login {}}>Log in</a></p>,
    }}
  </Layout.Page>
```

The page's links to the next and previous pages go in `<al-pagination>`'s
`prev` and `next` slots, a dimmed `<span>` standing in when there is no
such page; the default slot holds the app's own "Page N" (the page does
not count the results, so it cannot say "of M", which the element would
otherwise draw from its `page` and `pages` attributes).

## Its tests

`alder test examples/blog` runs 145 tests: the 103 of the generated
admin (chapter 15), and forty-two in seven modules, which `-run` picks
out (it matches each test's `Module "name"`):

```text
$ alder test examples/blog -run '^(Post|Comment|Auth|Posts|Write)[. ]'
ok   Post "a slug is lowercase letters, digits and hyphens"
ok   Post "a draft is not visible"
ok   Post "bySlug finds the post with that slug"
ok   Post "an author with posts cannot be deleted"
ok   Comment "a name or comment has text and a bound"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
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
ok   Posts.Index "a page that is exactly full has no next page"
ok   Posts.Index "logging out from the front page goes to log in"
ok   Posts.Show "a draft is not found for a reader"
ok   Posts.Show "a draft is shown to its author"
ok   Posts.Show "a post's body is rendered through the Markdown capability"
ok   Posts.Show "a comment on a draft is refused"
ok   Posts.Show "a comment is listed oldest first"
ok   Posts.Show "an empty comment is rejected"
ok   Posts.Show "commenting through the form"
ok   Write "a Reader cannot open the write page"
ok   Write "creating a post redirects to its edit page"
ok   Write "a taken slug is refused"
ok   Write "the write page needs an Author"
ok   Write "a demoted author cannot open the write page"
ok   Write "a promoted reader logs in again first"
ok   Write "an Author logs out from the write page"
ok   Write "the write page lists the author's own posts, newest first"
ok   Write.Edit "deleting a post deletes its comments"
ok   Write.Edit "another author's post is not found"
ok   Write.Edit "a demoted author is turned away at once"
ok   Write.Edit "a promoted reader logs in again before editing"
ok   Write.Edit "Publish shows the post on the front page; Unpublish hides it"
ok   Write.Edit "a visitor is sent to log in"
ok   Write.Edit "saving changes the title and the body"
42 passed, 0 failed
```

The `Posts.Index` tests call `load` the way the runtime does, with
parameters and a session record (both its fields: a default applies to
the cookie, not to a record written in a test), against rows the test
inserted:

```alder part=blog/Posts.Index
test "the front page lists only published posts, newest first" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "first",
    title: Text.NonEmpty "First",
    body: "",
    published: True,
    author: author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "draft",
    title: Text.NonEmpty "Draft",
    body: "",
    author: author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "second",
    title: Text.NonEmpty "Second",
    body: "",
    published: True,
    author: author,
  }
  let model <- load { page: 1, q: "" } { user: Nothing, role: Reader }
  Task.succeed (
    List.map (\p -> p.title) model.posts == [Text.NonEmpty "Second", Text.NonEmpty "First"]
  )
}

test "search matches title or body" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "tomatoes",
    title: Text.NonEmpty "Tomato season",
    body: "It is here.",
    published: True,
    author: author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "soup",
    title: Text.NonEmpty "Soup",
    body: "Use ripe tomatoes.",
    published: True,
    author: author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "trains",
    title: Text.NonEmpty "Trains",
    body: "Fast ones.",
    published: True,
    author: author,
  }
  let model <- load { page: 1, q: "TOMATO" } { user: Nothing, role: Reader }
  let titles = List.map (\p -> p.title) model.posts
  Task.succeed (
    {
      count: List.length titles,
      title: List.member (Text.NonEmpty "Tomato season") titles,
      body: List.member (Text.NonEmpty "Soup") titles,
    } == {
        count: 2,
        title: True,
        body: True,
      }
  )
}

test "a `%` in the query is literal" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "done",
    title: Text.NonEmpty "100% done",
    body: "",
    published: True,
    author: author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "ways",
    title: Text.NonEmpty "1000 ways",
    body: "",
    published: True,
    author: author,
  }
  let model <- load { page: 1, q: "100%" } { user: Nothing, role: Reader }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "100% done"])
}

test "page 99 is empty" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <-
    Task.all (
      List.map (\n -> Db.insert Post {
        slug: Post.Slug ("post-" ++ Text.fromInt n),
        title: Text.NonEmpty "A post",
        body: "",
        published: True,
        author: author,
      }) (List.range 1 11)
    )
  let pages <-
    Query.all (List.map (\n -> load { page: n, q: "" } { user: Nothing, role: Reader }) [
      1,
      2,
      99,
      0,
    ])
  let seen =
    List.map (\m -> {
      page: m.page,
      posts: List.length m.posts,
      hasMore: m.hasMore,
    }) pages
  Task.succeed (
    seen == [
        {
          page: 1,
          posts: 10,
          hasMore: True,
        },
        {
          page: 2,
          posts: 1,
          hasMore: False,
        },
        {
          page: 99,
          posts: 0,
          hasMore: False,
        },
        {
          page: 1,
          posts: 10,
          hasMore: True,
        },
      ]
  )
}

test "a negative page is page 1" = {
  let model <- load { page: -3, q: "" } { user: Nothing, role: Reader }
  Task.succeed (model.page == 1)
}

test "a `_` in the query is literal" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "snake",
    title: Text.NonEmpty "snake_case",
    body: "",
    published: True,
    author: author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "camel",
    title: Text.NonEmpty "snakeYcase",
    body: "",
    published: True,
    author: author,
  }
  let model <- load { page: 1, q: "e_c" } { user: Nothing, role: Reader }
  Task.succeed (List.map (\p -> p.title) model.posts == [Text.NonEmpty "snake_case"])
}

-- The query is cut to its first 100 characters: SQLite refuses a pattern over ~50 KB, and below
-- that a search costs the length of every post times the length of the query.
test "a long query is cut" = {
  let long = Text.join "" (List.map (\n -> "tomato ") (List.range 1 100))
  let model <- load { page: 1, q: long } { user: Nothing, role: Reader }
  Task.succeed (model.q == Text.slice 0 100 long)
}

test "a search answers with the first page of its results" = {
  let model <- load { page: 3, q: "" } { user: Nothing, role: Reader }
  let nav <- update (Search { form: { q: "soup" } }) model { user: Nothing, role: Reader }
  Task.succeed (nav == Redirect (Posts.Index { page: 1, q: "soup" }))
}

test "only an Author sees the Write link" = {
  let reader <- Db.insert User {
    email: Text.Email "reader@example.com",
    name: Text.NonEmpty "Reader",
    passwordHash: "",
  }
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let pages <- Query.all [
    load { page: 1, q: "" } { user: Nothing, role: Reader },
    load { page: 1, q: "" } { user: Just reader, role: Reader },
    load { page: 1, q: "" } { user: Just author, role: Author },
  ]
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
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <-
    Task.all (
      List.map (\n -> Db.insert Post {
        slug: Post.Slug ("post-" ++ Text.fromInt n),
        title: Text.NonEmpty "A post",
        body: "",
        published: True,
        author: author,
      }) (List.range 1 11)
    )
  let first <- Test.get (Posts.Index {})
  let second <- Test.get (Posts.Index { page: 2 })
  Task.succeed (
    {
      firstPosts: Test.count "ul.posts li" first.doc,
      next: Test.attr "href" "a[rel=next]" first.doc,
      secondPosts: Test.count "ul.posts li" second.doc,
      prev: Test.attr "href" "a[rel=prev]" second.doc,
      more: Test.has "a[rel=next]" second.doc,
    } == {
        firstPosts: 10,
        next: Just "/?page=2",
        secondPosts: 1,
        prev: Just "/",
        more: False,
      }
  )
}

test "a search through the form" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "soup",
    title: Text.NonEmpty "Soup",
    body: "Use ripe tomatoes.",
    published: True,
    author: author,
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "trains",
    title: Text.NonEmpty "Trains",
    body: "Fast ones.",
    published: True,
    author: author,
  }
  let _ <- Test.get (Posts.Index {})
  let r <- Test.submit SearchForm { q: "tomato" }
  Task.succeed (
    {
      answer: r.answer,
      titles: Test.texts "ul.posts a" r.doc,
      query: Test.attr "value" "input[name=q]" r.doc,
    } == {
        answer: Redirected (Posts.Index { page: 1, q: "tomato" }),
        titles: ["Soup"],
        query: Just "tomato",
      }
  )
}
```

The first test can check that `Second` comes before `First` because a
test's clock ticks (chapter 10): each `Db.insert` reads it for `created`,
one second later each time. Changing `desc` to `asc` in `load` fails
this test, and flipping it in `Posts.Show` fails "a comment is listed
oldest first".

Three more tests go through the browser. `Write`'s "the write page needs
an Author" opens `/write` as a visitor (`Redirected (Auth.Login {})`:
`requires user` sends them to log in) and as a logged-in Reader
(`PageMissing`: the role check's 404); "an Author logs out from the
write page" clicks the page's `App.Logout` button and finds the
session's `user` gone. "a Reader cannot open the write page" logs in as
each role in turn, with `Test.login { user: Just reader, role: Reader }`
and then as the Author: the Reader gets `PageMissing`, the Author the
page (`Shown`). `Posts.Show`'s "an empty comment is rejected" posts the comment
form with an empty body through `Test.submitRaw`, which sends text as
typed, and gets `FormRejected [{ field: "body", message: "is not valid" }]`:
`Comment.Body` is a brand with a `where` rule, and a value its rule
refuses is "is not valid" (a `Text.NonEmpty` field would say "must not
be empty").

Ten tests were written after `alder test --mutate` (chapter 10, "Mutation
testing") ran on the blog: before them, 17 of its 58 mutants survived;
after, 3 do, and those three cannot be killed (chapter 10 explains why).
The generated admin, which came later, adds 175 mutants, and its own
tests kill every one (chapter 15).
`Post`'s two and `Write.Edit`'s two are above. `Write`'s "the write page
lists the author's own posts, newest first" inserts another author's post
between two of the author's own. `Posts.Show`'s "commenting through the
form" checks that a comment redirects back to the post. `Auth.Login`
gained the to-do app's `fake Password` and its two login tests, which
check the session's `user` (and now its `role`) after a login and the
form's message after a wrong one. `Posts.Index` gained the last two:

With exactly ten posts,
one full page, there is no next page; `hasMore` written with `>=` would
say there is, and only a test with exactly `perPage` posts tells `>`
from `>=`. The front page is the one page with a Log out button that a
visitor may also open, so logging out from it shows whether `App.Logout`
redirects: a `Rerender` would show the front page again, where every
other Log out button sits on a page that sends a visitor to log in
anyway. It logs in as an Author and checks that the role is `Reader`
again afterwards.

```alder part=blog/Posts.Index
test "a page that is exactly full has no next page" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <-
    Task.all (
      List.map (\n -> Db.insert Post {
        slug: Post.Slug ("post-" ++ Text.fromInt n),
        title: Text.NonEmpty "A post",
        body: "",
        published: True,
        author: author,
      }) (List.range 1 10)
    )
  let model <- load { page: 1, q: "" } { user: Nothing, role: Reader }
  Task.succeed (
    { posts: List.length model.posts, hasMore: model.hasMore } == { posts: 10, hasMore: False }
  )
}

-- The front page is the one page with a Log out button that a visitor may also see, so logging
-- out from it shows whether `App.Logout` redirects: a Rerender would show the front page again.
-- Logging out also leaves the session's role at Reader.
test "logging out from the front page goes to log in" = {
  let author <- Db.insert User {
    email: Text.Email "author@example.com",
    name: Text.NonEmpty "Author",
    passwordHash: "",
    role: Author,
  }
  let _ <- Test.login { user: Just author, role: Author }
  let _ <- Test.get (Posts.Index {})
  let out <- Test.click App.Logout
  Task.succeed (
    {
      out: out.answer,
      user: out.session.user,
      role: out.session.role,
    } == {
        out: Redirected (Auth.Login {}),
        user: Nothing,
        role: Reader,
      }
  )
}
```

## Run it

```text
$ alder run examples/blog
```

`alder run` serves the app in dev mode (`ALDER_DEV=1`, chapter 11). In
dev mode only, the stub of `Password` (`adapters/auth/password.go`,
`func LoginPassword()`, as for the to-do app) accepts
three accounts, all with the password `password`, and creates each one's
`User` row on its first login:

- `author@example.com` is an Author: "Write" on the front page opens
  `/write`, to create, publish and delete posts.
- `reader@example.com` is a Reader: it can comment, and every `/write`
  and `/admin` page is a 404 for it.
- `admin@example.com` is an Admin: `/admin` opens the generated admin
  (chapter 15), where every post, comment and user can be changed.

Outside dev mode the stub refuses every login and logs that it must be
replaced, as it must before the blog faces the internet.

The blog's tables come from `examples/blog/migrations/` (chapter 8,
"Migrations"): `0001_initial.sql` creates them, and `alder run` and
`alder test` apply it. After a change to a `table`, run `alder migrate`;
until then `alder check` reports E0019.

## A third app: orgs, and a database per org

`examples/workspace` is the example of chapter 6's "Partitions" and
chapter 8's "Silos": users work in **orgs**, each org's projects and
to-dos are its own, and each org's live in a database of the org's own.
Its modules, besides the forty of the admin `alder new admin`
wrote (chapter 15, "The admin of a silo"):

| Module | What it is |
|---|---|
| `App` | The session with a key, `org … via Membership`; `choose org`; `silo org { Project, Todo }`; `maintenance` |
| `User`, `Org`, `Membership` | Shared tables: the users, the orgs, and who is a member of which; `Membership.isMember`, the re-read idiom |
| `Project`, `Todo` | The siloed tables, partitioned by `org` |
| `Activity` | The org's log: partitioned by `org`, but shared |
| `Auth.Login`, `Layout` | As in the blog; the login also puts the user's first org in the session |
| `Orgs.Index` | `/orgs`: the user's orgs, to open one, and a form for a new one (`choose org`'s page) |
| `Projects.Index`, `Projects.Show` | `/projects`, `/projects/{id}`: the org's projects and their to-dos |
| `Members.Index` | `/members`: the org's members, adding and removing them, and its activity |

### The session and the silo: `App`

`App` declares the key, the page that picks it, and the silo, each with
a comment saying what it does:

```alder part=workspace/App
module App

import Layout
import Org
import User

-- The session (book 06, "Partitions"): the signed-in user, the org they work in, and their role
-- as their row had it at login. `org` is a key: `via Membership` means a `Session.put` of it to
-- an org the user has no Membership row for fails, inside the Task's transaction. A page that
-- `requires user, org` sees it narrowed; one without it is sent to `choose org`'s page.
session {
  user: Maybe User.UserId,
  org: Maybe Org.OrgId via Membership,
  role: User.Role = Member,
}
```

`org` is a session field like `role`, a `Maybe` of the `Org` table's
id. `via Membership` is its rule: a `Session.put` that sets it needs a
`Membership` row for the user and that org, checked when it runs
(chapter 6, "Partitions").

```alder part=workspace/App
login = Auth.Login

-- where a page that `requires org` sends a session without one: the user's orgs, to pick one
choose org = Orgs.Index

-- Projects and their to-dos live in a database per org (book 08, "Silos"): one org's rows are
-- not in another's file at all. User, Org, Membership and Activity are shared; Activity is
-- partitioned by org there, a predicate (`Db.scoped`), not a file of its own.
silo org { Project, Todo }
```

`choose org` is where a page that `requires user, org` sends a session
without one. `silo org` puts the two tables in a database per org; the
four others are shared.

```alder part=workspace/App
msg Logout

msg Leave

update : Msg -> Session -> Task Nav
update msg session = match msg {
  -- logging out clears the user; a `via` key goes with it (the runtime clears `org`)
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing, role: Member })
    Task.succeed (Redirect (Auth.Login {}))
  },
  -- back to the list of orgs, to pick another
  Leave p -> {
    let _ <- Session.put (\s -> { s | org: Nothing })
    Task.succeed (Redirect (Orgs.Index {}))
  },
}
```

`Logout` sets only the user and the role: a session without a user
cannot keep an org, and the runtime clears it. `Leave` clears the org
alone, back to the list.

```alder part=workspace/App
error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>
```

```alder part=workspace/App
-- What a request answers when its org's database cannot be served (behind by a migration that
-- drops data, or ahead of this build: book 08, "Silos"): a 503, for that org only.
maintenance : Html Never
maintenance =
  <Layout.Page title="Down for maintenance">
    <h1>Down for maintenance</h1>
    <p>This workspace is being upgraded. Please try again in a few minutes.</p>
  </Layout.Page>
```

`maintenance` is the page an org's request gets while that org's
database cannot be served: behind by a migration that drops data, which
waits for an operator, or written by a newer build (chapter 8, "The
open-time check"). Every other org is served meanwhile.

```alder part=workspace/App
-- Both answers end where a browser would be sent anyway (the login page, `choose org`'s page),
-- so only a test of `update` itself tells a Redirect from a Rerender (book 10).
test "Logout and Leave say where to go" = {
  let me : Session = {
    user: Just (User.UserId "me"),
    org: Nothing,
    role: Member,
  }
  let out <- Test.run (update Logout me)
  let leave <- Test.run (update Leave me)
  Task.succeed (
    { out: out.value, leave: leave.value }
      == { out: Ok (Redirect (Auth.Login {})), leave: Ok (Redirect (Orgs.Index {})) }
  )
}
```

### A table in the silo: `Project`

```alder part=workspace/Project
module Project

import Org

type ProjectId = brand Text

-- Partitioned by `org` and in App's `silo org`: a database per org. Pages read it with
-- `Db.scoped Project` (the session's org), and a row of another org is not there to find.
table Project {
  id: ProjectId @id,
  name: Text.NonEmpty,
  org: Org.OrgId @ref Org @partition org,
  created: Time @created,
}
```

`@partition org` says which column holds the org; `silo org` in `App`
puts the table in the org's database. `Todo` is the same, with
`project: Project.ProjectId @ref Project @cascade`, a reference inside
one database, so deleting a project deletes its to-dos there. A shared
table may not refer to either (E0033).

### Choosing an org: `Orgs.Index`

`/orgs` requires the user but not the org: it is where a user without
one is sent. Its two messages are the two ways an org gets into a
session:

```alder fragment
Open p -> {
  let _ <- Session.put (\s -> { s | org: Just p.org })
  Task.succeed (Redirect (Projects.Index {}))
},
Create p -> {
  let org <- Db.insert Org { name: p.form.name }
  let _ <- Db.insert Membership { user: session.user, org: org }
  let _ <- Db.insert Activity { text: "The org was created.", org: org }
  let _ <- Session.put (\s -> { s | org: Just org })
  Task.succeed (Redirect (Projects.Index {}))
},
```

`Open` trusts nothing about the org it is given: the `via` check does
the work, and a test sends the `Open` of an org the user is no member of
by calling `update` itself (the page offers no such button):

```alder fragment
test "a non-member's switch is refused" = {
  …
  let _ <- Test.login { user: Just s.ann, org: Just s.acme }
  let r <- Test.run (update (Open { org: s.globex }) { orgs: [] } { user: s.ann, org: Nothing, role: Member })
  let page <- Test.get (Orgs.Index {})
  Task.succeed ({ value: r.value, org: page.session.org } == { value: Err "Session.put org: no Membership for this user and org", org: Just s.acme })
}
```

This test is the one that kills the mutant `membership dropped`
(chapter 10). `Create` writes an org, its first membership and a line of
activity: three shared tables, one store, one `Task` (E0031 would
refuse a `Task` that also wrote a project). Its `Session.put` passes the
check because the membership was inserted earlier in the same
transaction.

### An org's page: `Projects.Index`

`/projects` `requires user, org`, so inside it `session.org` is an
`Org.OrgId`. Its `load` does three things:

```alder fragment
load params session = {
  let member <- Membership.isMember session.user session.org
  if member then {
    let org <- Db.get Org session.org
    let projects <- Db.scoped Project |> Db.orderBy Project.name.asc |> Db.all
    Query.succeed { org: Maybe.withDefault "" (Maybe.map (\o -> o.name) org), projects: projects }
  } else Query.notFound
}
```

- `Membership.isMember` reads the membership again (the re-read idiom):
  a member removed since the org was put in their cookie gets a 404 at
  once, here and on every message of the page.
- `Db.get Org session.org` reads the shared database: the org's name.
- `Db.scoped Project` reads the org's own database. Both in one `load`:
  a read of the other store goes through a read-only connection, so the
  rule "one store per `Task`" does not limit reads (chapter 8).

Its messages write the org's database only, with `Db.insertScoped`
(which fills `org` in) and `Db.deleteScoped` (which finds nothing of
another org). `Projects.Show` reads one project with `Db.getScoped`: a
project of another org is a 404, the test "another org's project is not
found".

### The shared log: `Members.Index`

The org's log is partitioned by `org` and shared: every org's lines are
in one table of the shared database, and `Db.scoped Activity` reads the
session's org's (`WHERE "org" = ?`). It is written beside the
membership it is about, in one `Task`:

```alder fragment
Remove p -> {
  let rows <- Db.from Membership |> Db.where (Membership.org.eq session.org) |> Db.where (Membership.user.eq p.user) |> Db.all
  let _ <- Task.all (List.map (\m -> Db.delete Membership m.id) rows)
  let _ <- Db.insertScoped Activity { text: "A member left." }
  Task.succeed Rerender
},
```

Had the log been in the silo, this `Task` would write two stores; had
the projects page logged its new projects here, the same. Where a
table lives follows from what writes it.

### Its tests and its score

The app's own 34 tests cover what the rules promise, with two orgs:
`Projects.Index "a member sees the org's name and only its projects, by
name"`, `"a project is created in the org's database, and another org
does not see it"`, `Orgs.Index "a non-member's switch is refused"`, `"a
membership deleted earlier in the same Task counts as gone"`, and the
re-read tests "a member removed from the org loses access at once". The
generated admin brings 273 more. `alder test --mutate examples/workspace`
scores 0.979 (511 of 522 mutants killed); the 11 survivors are in the
generated admin, and `examples/workspace/README.md` says why each is
not a gap.

### Run it

```text
$ alder run examples/workspace
```

In dev mode the stub of `Password` accepts `ann@example.com` and
`bob@example.com` (Members) and `admin@example.com` (an Admin), all with
the password `password`. Log in as Ann, make an org on `/orgs`, add Bob
on `/members` by his email, and log in as Bob: he sees Ann's org's
projects, and none of another org's. Each org's database is a file under
`build/workspace/silos/org/`; the shared one is `build/workspace/dev.db`.
As the Admin, `/admin/orgs` lists every org, and each opens that org's
pages, a member or not.
