```alder-cli
alder fill --decl 'Admin."pressing Publish"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Test.login { user: Just me }
  let post1 <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: me }
  let before <- Test.get (Admin {})
  let r <- Test.click (Publish { id: post1 })
  let after <- Db.get Post post1
  let front <- Test.get (Posts.Index {})
  Task.succeed ({ drafts: Test.texts "#drafts .title" before.doc, answer: r.answer, left: Test.texts "#drafts .title" r.doc, published: Maybe.map (\p -> p.published) after, front: Test.texts "ul.posts a" front.doc } == { drafts: ["Draft"], answer: Redirected (Admin {}), left: [], published: Just True, front: ["Draft"] })
}
EOF
alder new test Posts.Index "a draft is not listed and not found by a search"
alder fill 'Posts.Index."a draft is not listed and not found by a search"' - <<'EOF'
{
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let _ <- Db.insert Post { slug: Post.Slug "soup", title: Text.NonEmpty "Soup", body: "Use ripe tomatoes.", author: author, published: True }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Tomato draft", body: "More tomatoes.", author: author }
  let front <- load { page: 1, q: "" } { user: Nothing }
  let search <- load { page: 1, q: "tomato" } { user: Nothing }
  Task.succeed ({ front: List.map (\p -> p.title) front.posts, search: List.map (\p -> p.title) search.posts } == { front: [Text.NonEmpty "Soup"], search: [Text.NonEmpty "Soup"] })
}
EOF
alder new test Posts.Show "a draft is not found but by its author"
alder fill 'Posts.Show."a draft is not found but by its author"' - <<'EOF'
{
  let _ <- Test.fake Markdown
  let author <- Db.insert User { email: Text.Email "author@example.com", name: Text.NonEmpty "Author", passwordHash: "", role: Author }
  let reader <- Db.insert User { email: Text.Email "reader@example.com", name: Text.NonEmpty "Reader", passwordHash: "" }
  let _ <- Db.insert Post { slug: Post.Slug "draft", title: Text.NonEmpty "Draft", body: "", author: author }
  let asVisitor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just reader }
  let asReader <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  let _ <- Test.login { user: Just author }
  let asAuthor <- Test.open (Posts.Show { slug: Post.Slug "draft" })
  Task.succeed ({ visitor: asVisitor.answer, reader: asReader.answer, author: asAuthor.answer } == { visitor: PageMissing, reader: PageMissing, author: Shown })
}
EOF
alder test
```
