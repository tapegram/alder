--- file: app/Post.alder ---
module Post

import User

type PostId = brand Text
type Slug = brand Text where (\s -> isSlug s)

-- A post is a draft until its author publishes it from the dashboard: `published` is False for
-- a new post. Only published posts are listed or found by a search, and a draft's page is a 404
-- for anyone but its author.
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

--- file: app/Admin.alder ---
module Admin

import Post
import User
import Layout

route Admin   = GET  "/admin"                { }                 requires user
route Create  = POST "/admin"                { } form NewPost    requires user
-- Publishing puts a draft in the published posts: a sub-resource of the post, so a PUT.
route Publish = PUT  "/admin/{id}/published" { id: Post.PostId } requires user

type Model = {
  author: User,
  posts: List Post,
  drafts: List Post,
}

-- load lets only an Author in: a Reader who is logged in gets the same 404 as a page that does
-- not exist. No `load` runs before a write route, so Create and Publish ask `User.author` too.
Admin.load : Admin.Params -> Session -> Query Model
Admin.load params session = {
  let me <- User.author session.user
  match me {
    Just user -> {
      let all <-
        Db.from Post
          |> Db.where (Post.author.eq user.id)
          |> Db.orderBy Post.created.desc
          |> Db.all
      let split = List.partition (\p -> p.published) all
      Query.succeed { author: user, posts: split.yes, drafts: split.no }
    },
    Nothing -> Query.notFound,
  }
}

form NewPost = {
  slug: Post.Slug,
  title: Text.NonEmpty,
  body: Text,
}

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

-- Publish changes the post only when the session's user wrote it and is still an Author; any
-- other id, like a missing one, is a 404.
Publish.update : Publish.Params -> Session -> Task Response
Publish.update p session = {
  let me <- User.author session.user
  let found <- Db.get Post p.id
  let mine =
    Maybe.andThen (\post -> if me != Nothing && post.author == session.user then Just post else Nothing) found
  match mine {
    Just post -> {
      let _ <- Db.update Post post.id (\x -> { x | published: True })
      Task.succeed (Response.redirect (Admin {}))
    },
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

    <h2>Published</h2>
    <ul class="posts">
      {for post in model.posts ->
        <li>
          <a href={Admin.Edit.Edit { id: post.id }}>{post.title}</a>
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
    Just post -> Task.succeed (r.value == Ok (Response.redirect (Admin.Edit.Edit { id: post.id })) && post.published == False),
    Nothing -> Task.fail "no post has the slug hello",
  }
}

test "a taken slug is refused" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let r <- Test.run (Create.update { form: { slug: Post.Slug "hello", title: Text.NonEmpty "Again", body: "" } } { user: author })
  let posts <- Db.from Post |> Db.count
  Task.succeed ({ value: r.value, posts: posts } == { value: Ok (Response.invalid "Another post has that slug."), posts: 1 })
}

-- The admin page through the browser: a visitor is sent to log in; a Reader who is logged in
-- gets the 404; an Author gets the page, and can log out from it.
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

-- The dashboard lists the author's drafts, and only those, in #drafts.
test "the dashboard lists the author's drafts" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "mine", title: Text.NonEmpty "Mine", body: "", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "out", title: Text.NonEmpty "Out", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "theirs", title: Text.NonEmpty "Theirs", body: "", author: other }
  let _ <- Test.login { user: Just author }
  let page <- Test.get (Admin {})
  Task.succeed (Test.texts "#drafts .title" page.doc == ["Mine"])
}

test "publishing a draft from the dashboard puts it on the front page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let front <- Test.get (Posts.Index.Index {})
  let before <- Test.get (Admin {})
  let r <- Test.click (Publish { id: id })
  let after <- Test.get (Posts.Index.Index {})
  Task.succeed (
    {
      frontBefore: Test.texts "ul.posts a" front.doc,
      drafts: Test.texts "#drafts .title" before.doc,
      answer: r.answer,
      left: Test.count "#drafts .title" r.doc,
      frontAfter: Test.texts "ul.posts a" after.doc,
    } == {
      frontBefore: [],
      drafts: ["Hello"],
      answer: Redirected (Admin {}),
      left: 0,
      frontAfter: ["Hello"],
    }
  )
}

test "publishing another author's post is not found" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let _ <- Test.login { user: Just other }
  let r <- Test.run (Publish.update { id: id } { user: other })
  let after <- Db.get Post id
  Task.succeed ({ value: r.value, published: Maybe.map (\p -> p.published) after } == { value: Err "not found", published: Just False })
}

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
-- is cut to 100 characters. A draft is never listed nor found. It asks for one post more than a
-- page holds: if that one comes back there is a next page (`hasMore`), and it is dropped from the
-- list. `canWrite` shows the link to the Admin page to an Author only.
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

-- A draft is never listed, not on the front page and not in a search, not even for its author.
test "a draft is not on the front page and not found by a search" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Tomato soup", body: "", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "salad", title: Text.NonEmpty "Tomato salad", body: "", author: author }
  let front <- Test.get (Index {})
  let found <- Test.get (Index { q: "tomato" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.get (Index { q: "salad" })
  Task.succeed (
    {
      front: Test.texts "ul.posts a" front.doc,
      found: Test.texts "ul.posts a" found.doc,
      asAuthor: Test.count "ul.posts li" asAuthor.doc,
    } == {
      front: ["Tomato soup"],
      found: ["Tomato soup"],
      asAuthor: 0,
    }
  )
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
-- `update` finds the post again: a slug that was never used, or a draft of someone else's, is a
-- 404.
route AddComment = POST "/posts/{slug}/comments" { slug: Post.Slug } form NewComment

type Model = {
  post: Post,
  body: Html,
  comments: List Comment,
}

-- readable is the post when the user may see it: a published post is anyone's to read, a draft
-- only its author's.
readable : Maybe User.UserId -> Post -> Maybe Post
readable user post = if post.published || user == Just post.author then Just post else Nothing

-- load finds the post by its slug; a slug that was never used, or someone else's draft, is a 404.
Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Post.bySlug params.slug
  let visible = Maybe.andThen (readable session.user) found
  match visible {
    Just post -> {
      let body <- Markdown.render post.body
      let comments <-
        Db.from Comment
          |> Db.where (Comment.post.eq post.id)
          |> Db.orderBy Comment.created.asc
          |> Db.all
      Query.succeed { post: post, body: body, comments: comments }
    },
    Nothing -> Query.notFound,
  }
}

form NewComment = {
  author: Comment.Name,
  body: Comment.Body,
}

AddComment.update : AddComment.Params -> Session -> Task Response
AddComment.update p session = {
  let found <- Post.bySlug p.slug
  let visible = Maybe.andThen (readable session.user) found
  match visible {
    Just post -> {
      let _ <- Db.insert Comment { post: post.id, author: p.form.author, body: p.form.body }
      Task.succeed (Response.redirect (Show { slug: post.slug }))
    },
    Nothing -> Task.notFound,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.post.title}>
    <article>
      <h1>{model.post.title}</h1>
      {if model.post.published then Html.none else <p class="draft">Draft</p>}
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

-- A draft's page is a 404 for a visitor and for a reader who is logged in; its author opens it.
test "a draft's page is not found but for its author" = {
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "Soon.", author: author }
  let visitor <- Test.open (Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Show { slug: Post.Slug "draft" })
  Task.succeed (
    {
      visitor: visitor.answer,
      reader: asReader.answer,
      author: asAuthor.answer,
      title: Test.text "h1" asAuthor.doc,
    } == {
      visitor: PageMissing,
      reader: PageMissing,
      author: Shown,
      title: Just "Draft",
    }
  )
}

test "a comment on someone else's draft is not found" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
  let r <- Test.run (AddComment.update { slug: Post.Slug "draft", form: { author: Comment.Name "Ann", body: Comment.Body "Hi" } } { user: Nothing })
  let comments <- Db.from Comment |> Db.count
  Task.succeed ({ value: r.value, comments: comments } == { value: Err "not found", comments: 0 })
}
