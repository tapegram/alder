### Round 2

```text
$ alder new type Line "LineRow = { name: Text, quantity: Int, cost: Int }"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -6,6 +6,8 @@
 
 type LineId = brand Text
 
+type LineRow = { name: Text, quantity: Int, cost: Int }
+
 table Line {
   id: LineId @id,
   order: Order.OrderId @ref Order @cascade,
[exit 0]
$ alder new type Line "Choice = { id: Product.ProductId, name: Text }"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -8,6 +8,8 @@
 
 type LineRow = { name: Text, quantity: Int, cost: Int }
 
+type Choice = { id: Product.ProductId, name: Text }
+
 table Line {
   id: LineId @id,
   order: Order.OrderId @ref Order @cascade,
[exit 0]
$ alder new decl Line lineName "Line -> Query Text"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -30,6 +30,9 @@
   Query.succeed (List.sum costs)
 }
 
+lineName : Line -> Query Text
+lineName = ?lineName
+
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: me }
app/Line.alder:34:12: warning[W0002]: hole ?lineName has type Line -> Query Text
  lineName = ?lineName
             ^^^^^^^^^
[exit 0]
$ alder fill Line.lineName - <<'EOF'
\l -> {
  let p <- Db.get Product l.product
  Query.succeed (Maybe.withDefault "?" (Maybe.map (\x -> "{x.name}") p))
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -31,7 +31,10 @@
 }
 
 lineName : Line -> Query Text
-lineName = ?lineName
+lineName = \l -> {
+  let p <- Db.get Product l.product
+  Query.succeed (Maybe.withDefault "?" (Maybe.map (\x -> "{x.name}") p))
+}
 
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line lineRow "Line -> Query LineRow"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -36,6 +36,9 @@
   Query.succeed (Maybe.withDefault "?" (Maybe.map (\x -> "{x.name}") p))
 }
 
+lineRow : Line -> Query LineRow
+lineRow = ?lineRow
+
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: me }
app/Line.alder:40:11: warning[W0002]: hole ?lineRow has type Line -> Query Line.LineRow
  lineRow = ?lineRow
            ^^^^^^^^
[exit 0]
$ alder fill Line.lineRow - <<'EOF'
\l -> {
  let name <- lineName l
  let cost <- lineCost l
  Query.succeed { name: name, quantity: l.quantity, cost: cost }
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -37,7 +37,11 @@
 }
 
 lineRow : Line -> Query LineRow
-lineRow = ?lineRow
+lineRow = \l -> {
+  let name <- lineName l
+  let cost <- lineCost l
+  Query.succeed { name: name, quantity: l.quantity, cost: cost }
+}
 
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line orderLines "Order.OrderId -> Query (List LineRow)"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -43,6 +43,9 @@
   Query.succeed { name: name, quantity: l.quantity, cost: cost }
 }
 
+orderLines : Order.OrderId -> Query (List LineRow)
+orderLines = ?orderLines
+
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: me }
app/Line.alder:47:14: warning[W0002]: hole ?orderLines has type Order.OrderId -> Query (List Line.LineRow)
  orderLines = ?orderLines
               ^^^^^^^^^^^
[exit 0]
$ alder fill Line.orderLines - <<'EOF'
\order -> {
  let lines <- Db.from Line |> Db.where (Line.order.eq order) |> Db.all
  Query.all (List.map lineRow lines)
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -44,7 +44,10 @@
 }
 
 orderLines : Order.OrderId -> Query (List LineRow)
-orderLines = ?orderLines
+orderLines = \order -> {
+  let lines <- Db.from Line |> Db.where (Line.order.eq order) |> Db.all
+  Query.all (List.map lineRow lines)
+}
 
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line choices "Query (List Choice)"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -49,6 +49,9 @@
   Query.all (List.map lineRow lines)
 }
 
+choices : Query (List Choice)
+choices = ?choices
+
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: me }
app/Line.alder:53:11: warning[W0002]: hole ?choices has type Query (List Line.Choice)
  choices = ?choices
            ^^^^^^^^
[exit 0]
$ alder fill Line.choices - <<'EOF'
{
  let ps <- Db.from Product |> Db.where (Product.active.eq True) |> Db.all
  Query.succeed (List.map (\p -> { id: p.id, name: "{p.name}" }) ps)
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -50,7 +50,10 @@
 }
 
 choices : Query (List Choice)
-choices = ?choices
+choices = {
+  let ps <- Db.from Product |> Db.where (Product.active.eq True) |> Db.all
+  Query.succeed (List.map (\p -> { id: p.id, name: "{p.name}" }) ps)
+}
 
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl User isManager "UserId -> Query Bool"
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -12,3 +12,6 @@
   role: Role = Staff,
   created: Time @created,
 }
+
+isManager : UserId -> Query Bool
+isManager = ?isManager
app/User.alder:17:13: warning[W0002]: hole ?isManager has type User.UserId -> Query Bool
  isManager = ?isManager
              ^^^^^^^^^^
[exit 0]
$ alder fill User.isManager - <<'EOF'
\id -> {
  let found <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) found == Just Manager)
}
EOF
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -14,4 +14,7 @@
 }
 
 isManager : UserId -> Query Bool
-isManager = ?isManager
+isManager = \id -> {
+  let found <- Db.get User id
+  Query.succeed (Maybe.map (\u -> u.role) found == Just Manager)
+}
[exit 0]
$ alder new page Orders.Index '/' --list Order --requires user
wrote app/Orders/Index.alder
[exit 0]
$ alder new page Orders.Show '/orders/{id}' --show Order --requires user
wrote app/Orders/Show.alder
[exit 0]
$ alder new type Orders.Index "Row = { order: Order, total: Int }"
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -8,6 +8,8 @@
 
 type Model = { rows: List Order, page: Int, pages: Int }
 
+type Row = { order: Order, total: Int }
+
 perPage : Int
 perPage = 20
 
[exit 0]
$ alder fill --decl Orders.Index.Model - <<'EOF'
{ rows: List Row }
EOF
app/Orders/Index.alder:31:25: error[E0003]: expected List Orders.Index.Row, got List Order
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                          ^^^^
app/Orders/Index.alder:31:31: error[E0002]: no field `page` on `Orders.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                ^^^^^^^^^^
app/Orders/Index.alder:31:43: error[E0002]: no field `pages` on `Orders.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                            ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Orders/Index.alder:47:20: error[E0002]: no field `owner` on `Orders.Index.Row`
                <td>{row.owner}</td>
                     ^^^^^^^^^
app/Orders/Index.alder:48:36: error[E0002]: no field `status` on `Orders.Index.Row`
                <td><al-badge>{match row.status { Open -> "Open", Paid -> "Paid", Cancelled -> "Cancelled" }}</al-badge></td>
                                     ^^^^^^^^^^
app/Orders/Index.alder:49:46: error[E0002]: no field `created` on `Orders.Index.Row`
                <td>{Time.format "Jan 2, 2006" row.created}</td>
                                               ^^^^^^^^^^^
app/Orders/Index.alder:56:11: error[E0002]: no field `page` on `Orders.Index.Model`
        {if model.page > 1 then <a slot="prev" href={Orders.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
            ^^^^^^^^^^
app/Orders/Index.alder:56:73: error[E0002]: no field `page` on `Orders.Index.Model`
        {if model.page > 1 then <a slot="prev" href={Orders.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
                                                                          ^^^^^^^^^^
app/Orders/Index.alder:57:32: error[E0002]: no field `page` on `Orders.Index.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                 ^^^^^^^^^^
app/Orders/Index.alder:57:61: error[E0002]: no field `pages` on `Orders.Index.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                                              ^^^^^^^^^^^
app/Orders/Index.alder:58:11: error[E0002]: no field `page` on `Orders.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Orders.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
            ^^^^^^^^^^
app/Orders/Index.alder:58:24: error[E0002]: no field `pages` on `Orders.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Orders.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                         ^^^^^^^^^^^
app/Orders/Index.alder:58:83: error[E0002]: no field `page` on `Orders.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Orders.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                                                                                    ^^^^^^^^^^
app/Orders/Index.alder:68:35: error[E0002]: no field `id` on `Orders.Index.Row`
    Task.succeed (List.map (\row -> row.id) page.model.rows == [order2, order1])
                                    ^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 49 of your script: `alder fill --decl Orders.Index.Model - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 16; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
