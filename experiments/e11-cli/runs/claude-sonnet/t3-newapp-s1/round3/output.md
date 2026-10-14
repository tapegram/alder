```alder-cli
alder fill Desk.update.Toggle - <<'EOF'
{
  let _ <- Db.update Post p.id (\t -> { t | published: not t.published })
  Task.succeed Rerender
}
EOF
alder fill 'Desk."pressing Toggle"' - --hole ?expected <<'EOF'
{ answer: r.answer, published: Maybe.map (\x -> x.published) after } == { answer: Rerendered, published: Just True }
EOF
alder fill --decl 'Posts.Show."submitting NewComment"' - <<'EOF'
{
  let _ <- Db.insert Post { title: Text.NonEmpty "Hello", slug: Slug.Slug "slug-1", body: "b", published: True }
  let _ <- Test.get (Posts.Show { slug: Slug.Slug "slug-1" })
  let r <- Test.submit NewComment { author: Text.NonEmpty "Ann", body: Text.NonEmpty "Nice" }
  let n <- Db.from Comment |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Rerendered, n: 1 })
}
EOF
alder check
alder test
```
