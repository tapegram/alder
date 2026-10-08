<!-- model=qwen3-coder:30b task=t3-newapp.md seconds=28 prompt_tokens=9901 output_tokens=1679 -->
--- file: app/App.alder ---
module App

session { user: Maybe UserId }

login = Auth.Login

error : Error -> Html Never
error err = <h1>Something went wrong</h1>

notFound : Html Never
notFound = <h1>Not found</h1>

msg Logout = "/logout" { }

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

--- file: app/Layout.alder ---
module Layout

Layout : { title: Text, children: List (Html Msg) } -> Html Msg
Layout props =
  <html>
    <head><title>{props.title}</title></head>
    <body>
      <header>
        <h1>My Blog</h1>
        <nav>
          <a href={Todos.Index {}}>Home</a>
          {if props.title == "Login" then <span></span> else <a href={App.Logout {}}>Logout</a>}
        </nav>
      </header>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Auth/Login.alder ---
module Auth.Login

page "/" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Html Msg
view model =
  <Layout title="Login">
    <h1>Login</h1>
    <form submit={Submit}>
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" type="password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout>

msg Submit = "/login" form Login

type Login = { email: Text.Email, password: Text.Secret }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Log.info "Login attempt"
    Task.succeed (Redirect (Todos.Index {}))
  },
}

--- file: app/Todo.alder ---
module Todo

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  slug: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  created: Time @created,
  owner: UserId @ref User,
}

type TodoId = brand Text
type UserId = brand Text

--- file: app/Todos/Index.alder ---
module Todos.Index

page "/" { }

type Model = { posts: List Todo }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Todo |> Db.where (Todo.published.eq True) |> Db.orderBy Todo.created.desc |> Db.all
  Query.succeed { posts: posts }
}

view : Model -> Html Msg
view model =
  <Layout title="Home">
    <h1>Posts</h1>
    <ul>
      {for post in model.posts -> <li><a href={Todos.Show { slug: post.slug }}>{post.title}</a></li>}
    </ul>
  </Layout>

msg Toggle = "/posts/{slug}/toggle" { slug: Text }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Toggle p -> {
    let post <- Db.get Todo p.slug
    let _ <- Db.update Todo p.slug (\t -> { t | published: not t.published })
    Task.succeed Rerender
  },
}

--- file: app/Todos/Show.alder ---
module Todos.Show

page "/posts/{slug}" { slug: Text }

type Model = { post: Todo, comments: List Comment }

load : Params -> Session -> Query Model
load params session = {
  let post <- Db.get Todo params.slug
  let comments <- Db.from Comment |> Db.where (Comment.post.eq params.slug) |> Db.all
  Query.succeed { post: post, comments: comments }
}

view : Model -> Html Msg
view model =
  <Layout title={model.post.title}>
    <h1>{model.post.title}</h1>
    <p>Published: {Bool.toText model.post.published}</p>
    <div>{model.post.body}</div>
    <h2>Comments</h2>
    <ul>
      {for comment in model.comments -> <li><strong>{comment.author}</strong>: {comment.body}</li>}
    </ul>
    <form submit={AddComment}>
      <Input field="author" label="Author" />
      <Errors field="author" />
      <Textarea field="body" label="Comment" />
      <Errors field="body" />
      <button>Add comment</button>
    </form>
  </Layout>

msg AddComment = "/posts/{slug}/comments" { slug: Text } form NewComment

type NewComment = { author: Text.NonEmpty, body: Text.NonEmpty }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddComment p -> {
    let _ <- Db.insert Comment { author: p.form.author, body: p.form.body, post: p.slug }
    Task.succeed Rerender
  },
}

--- file: app/Comment.alder ---
module Comment

table Comment {
  id: CommentId @id,
  author: Text.NonEmpty,
  body: Text,
  post: TodoId @ref Todo,
}

type CommentId = brand Text

--- file: app/Admin/Index.alder ---
module Admin.Index

page "/admin" { } requires user

type Model = { posts: List Todo }

load : Params -> Session -> Query Model
load params session = {
  let posts <- Db.from Todo |> Db.orderBy Todo.created.desc |> Db.all
  Query.succeed { posts: posts }
}

view : Model -> Html Msg
view model =
  <Layout title="Admin">
    <h1>Admin</h1>
    <ul>
      {for post in model.posts -> <li>{post.title} - {Bool.toText post.published} <button click={Toggle { slug: post.slug }}>Toggle</button></li>}
    </ul>
    <form submit={CreatePost}>
      <Input field="title" label="Title" />
      <Errors field="title" />
      <Input field="slug" label="Slug" />
      <Errors field="slug" />
      <Textarea field="body" label="Body" />
      <Errors field="body" />
      <button>Create post</button>
    </form>
  </Layout>

msg CreatePost = "/admin/posts" form NewPost

type NewPost = { title: Text.NonEmpty, slug: Text.NonEmpty, body: Text }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  CreatePost p -> {
    let _ <- Db.insert Todo { title: p.form.title, slug: p.form.slug, body: p.form.body, published: False, owner: session.user }
    Task.succeed Rerender
  },
}

--- file: app/Db.alder ---
module Db

capability Db {
  get : Table t -> Id t -> Query (Maybe t),
  from : Table t -> Select t,
  where : Condition t -> Select t -> Select t,
  orderBy : Order t -> Select t -> Select t,
  limit : Int -> Select t -> Select t,
  offset : Int -> Select t -> Select t,
  all : Select t -> Query (List t),
  first : Select t -> Query (Maybe t),
  count : Select t -> Query Int,
  insert : Table t -> Insert t -> Task (Id t),
  update : Table t -> Id t -> (t -> t) -> Task Unit,
  delete : Table t -> Id t -> Task Unit,
}