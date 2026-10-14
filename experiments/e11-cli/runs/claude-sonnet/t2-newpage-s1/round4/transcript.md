### Round 4

```text
$ alder fill 'Profile."submitting Rename"' - --hole ?expected <<'EOF'
Rerendered
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -64,7 +64,7 @@
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Profile {})
   let r <- Test.submit Rename { name: Text.NonEmpty "name-1" }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Rerendered)
 }
 
 test "Rename refuses an empty name" = {
[exit 0]
$ alder test
ok   Todo "countOpen ignores done todos"
ok   Todo "Active keeps only open todos"
ok   Todo "each filter has its label"
ok   Todo "a user with todos cannot be deleted"
ok   Admin.Index "the dashboard counts the todos and lists the newest five"
ok   Admin.Index "the dashboard counts the users and lists the newest five"
ok   Admin.Index "a visitor is sent to log in"
ok   Admin.Index "a Member gets no page, even one made Admin since logging in"
ok   Admin.Index "a demoted Admin is refused at once"
ok   Admin.Todo.Delete "deleting a todo nothing refers to"
ok   Admin.Todo.Delete "a missing row is not found"
ok   Admin.Todo.Delete "a visitor is sent to log in"
ok   Admin.Todo.Delete "a Member gets no page, even one made Admin since logging in"
ok   Admin.Todo.Delete "a demoted Admin is refused at once"
ok   Admin.Todo.Edit "saving changes every field"
ok   Admin.Todo.Edit "an owner not offered is refused"
ok   Admin.Todo.Edit "a missing row is not found"
ok   Admin.Todo.Edit "a visitor is sent to log in"
ok   Admin.Todo.Edit "a Member gets no page, even one made Admin since logging in"
ok   Admin.Todo.Edit "a demoted Admin is refused at once"
ok   Admin.Todo.Index "the list shows every row, newest first, and the labels of the rows they refer to"
ok   Admin.Todo.Index "sorting by title goes both ways"
ok   Admin.Todo.Index "searching finds the rows that contain the text"
ok   Admin.Todo.Index "filtering by done lists the rows with that value"
ok   Admin.Todo.Index "a search text of 20,000 characters is cut to 200"
ok   Admin.Todo.Index "the list has 25 rows a page, and a page past the last is the last"
ok   Admin.Todo.Index "a visitor is sent to log in"
ok   Admin.Todo.Index "a Member gets no page, even one made Admin since logging in"
ok   Admin.Todo.Index "a demoted Admin is refused at once"
ok   Admin.Todo.New "creating a todo adds it, with the values submitted"
ok   Admin.Todo.New "an owner not offered is refused"
ok   Admin.Todo.New "a visitor is sent to log in"
ok   Admin.Todo.New "a Member gets no page, even one made Admin since logging in"
ok   Admin.Todo.New "a demoted Admin is refused at once"
ok   Admin.Todo.Show "the page shows the row"
ok   Admin.Todo.Show "a missing row is not found"
ok   Admin.Todo.Show "a visitor is sent to log in"
ok   Admin.Todo.Show "a Member gets no page, even one made Admin since logging in"
ok   Admin.Todo.Show "a demoted Admin is refused at once"
ok   Admin.User.Delete "deleting a user nothing refers to"
ok   Admin.User.Delete "a user that todos refer to is not deleted"
ok   Admin.User.Delete "an Admin cannot delete their own account"
ok   Admin.User.Delete "a missing row is not found"
ok   Admin.User.Delete "a visitor is sent to log in"
ok   Admin.User.Delete "a Member gets no page, even one made Admin since logging in"
ok   Admin.User.Delete "a demoted Admin is refused at once"
ok   Admin.User.Edit "saving changes every field"
ok   Admin.User.Edit "saving an email another user has is refused"
ok   Admin.User.Edit "an Admin cannot change their own role"
ok   Admin.User.Edit "an Admin saves their own account, the role unchanged"
ok   Admin.User.Edit "an Admin demotes another Admin"
ok   Admin.User.Edit "a missing row is not found"
ok   Admin.User.Edit "a visitor is sent to log in"
ok   Admin.User.Edit "a Member gets no page, even one made Admin since logging in"
ok   Admin.User.Edit "a demoted Admin is refused at once"
ok   Admin.User.Index "the list shows every row, newest first"
ok   Admin.User.Index "sorting by email goes both ways"
ok   Admin.User.Index "searching finds the rows that contain the text"
ok   Admin.User.Index "filtering by role lists the rows with that value"
ok   Admin.User.Index "a search text of 20,000 characters is cut to 200"
ok   Admin.User.Index "the list has 25 rows a page, and a page past the last is the last"
ok   Admin.User.Index "a visitor is sent to log in"
ok   Admin.User.Index "a Member gets no page, even one made Admin since logging in"
ok   Admin.User.Index "
… (1807 more characters cut)
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
