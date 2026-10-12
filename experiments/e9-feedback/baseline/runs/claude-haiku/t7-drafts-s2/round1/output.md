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
-- its own. `posts` holds the author's drafts only.
load : Params -> Session -> Query Model
load params session = {
  let me <- User.author session.user
  match me {
    Just user -> {
      let posts <-
        Db.from Post
          |> Db.where (Post.author.eq user.id)
          |> Db.where (Post.published.eq False)
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
    let post <- ownPost p.id session
    let _ <- Db.update Post post.id (\t -> { t | published: True })
    Task.succeed (Redirect (Admin {}))
  },
}

-- ownPost is the post a message names, when the session's user wrote it; otherwise the message
-- is a 404. The id of Publish comes from the request, so `update` checks it.
ownPost : Post.PostId -> Session -> Task Post
ownPost id session = {
  let found <- Db.get Post id
  match found {
    Just post -> if post.author == session.user then Task.succeed post else Task.notFound,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Your posts">
    <h1>Your posts</h1>

    <h2>Drafts</h2>
    <ul id="drafts">
      {for post in model.posts ->
        <li>
          <a class="title" href={Admin.Edit { id: post.id }}>{post.title}</a>
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

-- A new post starts as a draft, and the dashboard lists it with a Publish button. The test
-- opens the dashboard again just before clicking, since the held page must be the one that
-- shows the button.
test "publishing a draft puts it on the front page" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "Hi.", author: author }
  let _ <- Test.login { user: Just author }
  let dashboard <- Test.get (Admin {})
  let front <- Test.get (Posts.Index {})
  let _ <- Test.get (Admin {})
  let r <- Test.click (Publish { id: id })
  let front2 <- Test.get (Posts.Index {})
  Task.succeed ({ before: Test.texts "#drafts .title" dashboard.doc, frontBefore: Test.texts "ul.posts a" front.doc, answer: r.answer, drafts: Test.count "#drafts li" r.doc, frontAfter: Test.texts "ul.posts a" front2.doc } == { before: ["Hello"], frontBefore: [], answer: Redirected (Admin {}), drafts: 0, frontAfter: ["Hello"] })
}

-- Publish names a post by its id, which comes from the request: another author's draft is a 404.
test "another author's draft cannot be published" = {
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Author }
  let id <- Db.insert Post { slug: Post.Slug "hello", title: Text.NonEmpty "Hello", body: "", author: author }
  let model <- load {} { user: other }
  let r <- Test.run (update (Publish { id: id }) model { user: other })
  let after <- Db.get Post id
  Task.succeed ({ value: r.value, published: Maybe.map (\p -> p.published) after } == { value: Err "not found", published: Just False })
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
