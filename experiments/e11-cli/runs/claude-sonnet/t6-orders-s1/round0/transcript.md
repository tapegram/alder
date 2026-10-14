### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new type User "Role = Staff | Manager"
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -2,6 +2,8 @@
 
 type UserId = brand Text
 
+type Role = Staff | Manager
+
 table User {
   id: UserId @id,
   email: Text.Email @unique,
[exit 0]
$ alder edit table User add-field role:Role=Staff
edited app/User.alder
wrote migrations/0002_add_role.sql
  rebuild table User: User.role: new column filled with 'Staff'
edited migrations/schema.json
--- a/app/User.alder
+++ b/app/User.alder
@@ -10,4 +10,5 @@
   name: Text.NonEmpty,
   passwordHash: Text,
   created: Time @created,
+  role: Role = Staff,
 }
[exit 0]
$ alder new table Product name:Text.NonEmpty price:Int.NonNegative active:Bool=True --no-timestamps
wrote app/Product.alder
wrote migrations/0003_create_product.sql
  create table Product
edited migrations/schema.json
[exit 0]
$ alder new table Order owner:User.UserId@ref=User created:Time@created --no-timestamps
wrote app/Order.alder
wrote migrations/0004_create_order.sql
  create table Order
  add index on Order.owner
edited migrations/schema.json
[exit 0]
$ alder new type Order "Status = Open | Paid | Cancelled"
edited app/Order.alder
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -4,6 +4,8 @@
 
 type OrderId = brand Text
 
+type Status = Open | Paid | Cancelled
+
 table Order {
   id: OrderId @id,
   owner: User.UserId @ref User,
[exit 0]
$ alder edit table Order add-field status:Status=Open
edited app/Order.alder
wrote migrations/0005_add_status.sql
  rebuild table Order: Order.status: new column filled with 'Open'
edited migrations/schema.json
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -10,6 +10,7 @@
   id: OrderId @id,
   owner: User.UserId @ref User,
   created: Time @created,
+  status: Status = Open,
 }
 
 test "a Order keeps its owner from being deleted" = {
[exit 0]
$ alder new table Line order:Order.OrderId@ref=Order@cascade product:Product.ProductId@ref=Product quantity:Int.Positive --no-timestamps
wrote app/Line.alder
wrote migrations/0006_create_line.sql
  create table Line
  add index on Line.order
  add index on Line.product
edited migrations/schema.json
[exit 0]
$ alder new auth --users User
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,9 +1,22 @@
 module App
 
 import Layout
+import User
 
-session { }
+session { user: Maybe User.UserId }
 
+login = Auth.Login
+
+msg Logout
+
+update : Msg -> Session -> Task Nav
+update msg session = match msg {
+  Logout p -> {
+    let _ <- Session.put (\s -> { s | user: Nothing })
+    Task.succeed (Redirect (Auth.Login {}))
+  },
+}
+
 error : Error -> Html Never
 error err =
   <Layout.Page title="Error">
@@ -16,3 +29,10 @@
   <Layout.Page title="Not found">
     <h1>Not found</h1>
   </Layout.Page>
+
+-- Every Log out button is on a page that requires a user, so a browser test cannot tell this
+-- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
+test "Logout clears the user and goes to log in" = {
+  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
+}
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder new decl Layout Shell "{ title: Text, children: List (Html a) } -> Html a"
alder new decl: `Shell` is not a value name: a value is named by a lower-case identifier, such as `waiting`
[exit 1]
```

The harness stopped at line 17 of your script: `alder new decl Layout Shell "{ title: Text, children: List (Html a) } -> Html a"`: it exited 1. The commands after it were not run.

Your script stopped at command 10; the app is as the commands before it left it; continue.

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
