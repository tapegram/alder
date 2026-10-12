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

-- Logging out from a page that `requires user` ends at the login page either way: a Rerender of
-- that page redirects there too. Only a test of `update` itself tells Redirect from Rerender.
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

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password, so a test can log in through the form below.
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
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

-- A known email, through the form, stores the user in the session and goes to the front page.
test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Posts.Index {}), user: Just me })
}

-- An unknown email is refused with the form's message, and no user is stored.
test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
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

-- bySlug finds the post with the slug asked for, not the first post in the table.
test "bySlug finds the post with that slug" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Slug "one", title: Text.NonEmpty "One", body: "", author: author }
  let _ <- Db.insert Post { slug: Slug "two", title: Text.NonEmpty "Two", body: "", author: author }
  let found <- bySlug (Slug "two")
  Task.succeed (Maybe.map (\p -> p.title) found == Just (Text.NonEmpty "Two"))
}

-- A user who still has posts cannot be deleted: `author` is `@ref User` with the default rule.
test "a user with posts cannot be deleted" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Slug "kept", title: Text.NonEmpty "Kept", body: "", author: author }
  let r <- Test.run (Db.delete User author)
  let posts <- Db.from Post |> Db.count
  Task.succeed ({ deleted: Result.toMaybe r.value, posts: posts } == { deleted: Nothing, posts: 1 })
}

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

-- The dashboard lists every post of the author, the newest at the top.
test "the dashboard lists the author's posts newest first" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "first", title: Text.NonEmpty "First", body: "", author: author }
  let _ <- Db.insert Post { slug: Post.Slug "second", title: Text.NonEmpty "Second", body: "", author: author }
  let _ <- Test.login { user: Just author }
  let page <- Test.get (Admin {})
  Task.succeed (Test.texts "ul.posts a" page.doc == ["Second", "First"])
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
