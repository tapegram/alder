# 15. Generators and the admin

Every Alder app is made of the same few things. There are **users**: a
`User` table, and a session whose `user` field says who is logged in
(chapter 6). There are **tables**, each in its own module, with their
migrations (chapter 8). There are **pages**: a path, a `Model`, a
`load`, a `view`, and an `update` for the page's **forms** and
**messages** (chapters 5 and 6). There are **capabilities**, the Go
behind the app (chapter 7). And there are **rules**: `requires user`,
`role R`, `@owner` (chapter 6, "Rules"). Every page has its tests
(chapter 10).

This chapter is about writing those pieces with commands instead of by
hand. `alder new page`, `alder new table`, `alder new form`, `alder new
msg`, `alder new capability`, `alder new type`, `alder new decl` and
`alder new test` each write one piece, and `alder new auth` the login.
`alder edit table` changes a table and writes its migration, `alder edit
page` a page's parameters. `alder fill` fills a gap the others leave.
`alder new admin` writes all of the pieces for every table at once: a complete admin, with a list, a page, a form and a delete page
per table, and `alder regen admin` keeps that admin up to date as the
tables change.

There are two readers in mind. One is you, starting an app: a generated
page is a correct, tested starting point that you then edit. The other
is an agent (an AI model) building an app from the command line. For an
agent the commands are the interface: it asks for a page or a table by
name and type, and gets back code that already checks and already has
tests. The only free text it writes is one typed expression at a time,
a body, through `alder fill`. The last section, "AI only in typed,
checked bodies", explains why.

Everything a generator writes is ordinary Alder. There is no hidden
framework: you can open each file, read it, and change it, and the
checker, `alder test` and `alder test --mutate` treat it like code you
typed. Chapter 11 has the reference table of every command and flag;
this chapter walks through them on the guestbook of chapter 2 and on the
blog of chapter 14.

## How a generator works

Every command does the same five things, in this order:

1. **Read the app** the way `alder check` does: parse every module,
   resolve the names and type-check it (chapter 13). An app that already
   has errors is refused (except by `alder edit table`, below), because
   nothing generated into it could be checked.
2. **Compute the change**: the files to create, and the edits to make to
   files that exist. The generators are Go code in the compiler's
   repository (`internal/generate/`); the small generators fill
   templates that are built into the `alder` binary, and the admin
   builds its files in Go.
3. **Apply it to a copy.** The app is copied to a temporary directory,
   and the change is made there, never in your app.
4. **Check the copy**, with the same checker. If the copy has an error,
   the command stops: it prints the copy's errors and the sentence `the
   result does not check (the errors above): nothing was written`, and
   exits with 1.
5. **Write**: only now are the files written into the app. The command
   prints each one (`wrote` for a new file, `edited` for a changed one),
   the unified diff of each edited file, and the copy's diagnostics,
   among them its **holes** (warning W0002, below).

This is called **copy, check, write**. It means that a generator never
leaves your app in a state that does not check, with one exception by
design: `alder edit table` writes a table change that breaks code
elsewhere, and prints the list of places to fix ("Changing a table",
below; `--strict` refuses instead). A generator also never overwrites
anything: when the file or the name it would write exists, it refuses
with one sentence and writes nothing; and when a file it is about to
write changed since the command read the app, it writes nothing and says
so.

`--dry-run` does steps 1 to 4 and prints what it would write: `would
write` and `would edit` lines, the diffs (a new file as a diff from
`/dev/null`) and `dry run: nothing written`. `--dir` names the app's
directory (the current one by default).

## A page: `alder new page`

The guestbook of chapter 2, as `alder new guestbook` writes it, has one
page, `Home`, and one table, `Entry`. In its directory:

```text command=guestbook
$ alder new page About /about
wrote app/About.alder
```

The first argument is the module, the second its path. The file:

```alder written=guestbook/app/About.alder
module About

import Layout

page "/about" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="About">
    <h1>About</h1>
  </Layout.Page>

test "the page is shown" = {
  let r <- Test.open (About {})
  Task.succeed ({ answer: r.answer, heading: Test.text "h1" r.doc } == { answer: Shown, heading: Just "About" })
}
```

It is the smallest page there is (chapter 6): no parameters, an empty
`Model`, a `view` in the app's layout, and one test, written as chapter
10 writes them, which opens the page through the test browser and checks
the answer and the heading. The page is yours from here: the generator
never touches it again.

## A table: `alder new table`

A table is a name and its fields. A field is written
`name:Type[=default][@attr[=arg]]…`, with no spaces (quote a field
whose type has one, `"note:Maybe Text"`). Here is a table of replies to
the guestbook's entries:

```text command=guestbook
$ alder new table Reply entry:Entry.EntryId@ref=Entry@cascade message:Text.NonEmpty hidden:Bool=False
wrote app/Reply.alder
wrote migrations/0002_create_reply.sql
  create table Reply
  add index on Reply.entry
edited migrations/schema.json
```

`entry:Entry.EntryId@ref=Entry@cascade` is a reference to an entry,
deleted with it (`@ref Entry @cascade`, chapter 8). `hidden:Bool=False`
has a default. The generator adds the id (`id: ReplyId @id`, with its
brand), `created` and `updated`:

```alder written=guestbook/app/Reply.alder
module Reply

import Entry

type ReplyId = brand Text

table Reply {
  id: ReplyId @id,
  entry: Entry.EntryId @ref Entry @cascade,
  message: Text.NonEmpty,
  hidden: Bool = False,
  created: Time @created,
  updated: Time @updated,
}

test "deleting its entry deletes a Reply" = {
  let target <- Db.insert Entry { name: Text.NonEmpty "name-1", message: Text.NonEmpty "message-1" }
  let _ <- Db.insert Reply { entry: target, message: Text.NonEmpty "message-1" }
  let _ <- Db.delete Entry target
  let left <- Db.from Reply |> Db.count
  Task.succeed (left == 0)
}
```

The test is there because of the `@cascade`: a test that deletes an
entry and counts the replies left. If the generator left it out, `alder
test --mutate` (chapter 10) would turn `@cascade` into `@restrict` and
no test would notice. Every reference with a delete rule gets such a
test: "deleting its X deletes a Y" for `@cascade`, "a Y keeps its X from
being deleted" for `@restrict`.

The table's migration is written in the same step, by the same code as
`alder migrate` (chapter 8), and `migrations/schema.json` is updated:

```sql written=guestbook/migrations/0002_create_reply.sql
-- alder migration 0002_create_reply
-- from 2e82b15d5f85
-- to b2852c92312f
-- step: create table Reply
-- step: add index on Reply.entry

CREATE TABLE "Reply__Reply" ("id" TEXT NOT NULL PRIMARY KEY, "entry" TEXT NOT NULL REFERENCES "Entry__Entry"("id") ON DELETE CASCADE, "message" TEXT NOT NULL, "hidden" INTEGER NOT NULL CHECK ("hidden" IN (0, 1)), "created" TEXT NOT NULL, "updated" TEXT NOT NULL) STRICT;
CREATE INDEX IF NOT EXISTS "Reply__Reply__entry" ON "Reply__Reply"("entry");
```

A type the checker does not know (`x:Nope.Type`) is refused with the
checker's own message, and a table name that is taken, with a sentence.

## A list of rows: `--list`

`alder new page` with `--list T` writes a page that lists a table's
rows:

```text command=guestbook
$ alder new page Replies /replies --list Reply
wrote app/Replies.alder
```

The module is the name you give, `Replies`, at `app/Replies.alder`: a
page's module is its name, and nothing is added to it (`Replies.Index`
would be `app/Replies/Index.alder`, if you prefer that).

```alder written=guestbook/app/Replies.alder
module Replies

import Entry
import Layout
import Reply

page "/replies" { page: Int = 1 }

type Model = { rows: List Reply, page: Int, pages: Int }

perPage : Int
perPage = 20

-- lastPage bounds `page`, so `(page - 1) * perPage` cannot overflow.
lastPage : Int
lastPage = 1000000

-- load reads one page of the rows, newest first, and how many pages there are.
load : Params -> Session -> Query Model
load params session = {
  let page = Int.clamp { min: 1, max: lastPage } params.page
  let rows <-
    Db.from Reply
      |> Db.orderBy Reply.created.desc
      |> Db.limit perPage
      |> Db.offset ((page - 1) * perPage)
      |> Db.all
  let count <- Db.from Reply |> Db.count
  Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Replies">
    <h1>Replies</h1>

    <al-table>
      <table>
        <thead>
          <tr><th>Entry</th><th>Message</th><th>Hidden</th><th>Created</th><th>Updated</th></tr>
        </thead>
        <tbody>
          {for row in model.rows ->
            <tr>
              <td>{row.entry}</td>
              <td>{row.message}</td>
              <td>{if row.hidden then <al-badge>Yes</al-badge> else <al-badge variant="secondary">No</al-badge>}</td>
              <td>{Time.format "Jan 2, 2006" row.created}</td>
              <td>{Time.format "Jan 2, 2006" row.updated}</td>
            </tr>}
        </tbody>
      </table>
    </al-table>

    <al-pagination role="navigation" aria-label="Pages">
      {if model.page > 1 then <a slot="prev" href={Replies { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
      <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
      {if model.page < model.pages then <a slot="next" href={Replies { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
    </al-pagination>
  </Layout.Page>

test "the list shows the rows, newest first" = {
  let entry1 <- Db.insert Entry { name: Text.NonEmpty "name-1", message: Text.NonEmpty "message-1" }
  let reply1 <- Db.insert Reply { entry: entry1, message: Text.NonEmpty "message-1" }
  let reply2 <- Db.insert Reply { entry: entry1, message: Text.NonEmpty "message-2" }
  let page <- Test.get (Replies {})
  Task.succeed (List.map (\row -> row.id) page.model.rows == [reply2, reply1])
}

test "the list has 20 rows a page" = {
  let entry1 <- Db.insert Entry { name: Text.NonEmpty "name-1", message: Text.NonEmpty "message-1" }
  let _ <- Task.all (List.map (\n -> Db.insert Reply { entry: entry1, message: Text.NonEmpty ("message-" ++ Text.fromInt n) }) (List.range 1 21))
  let first <- Test.get (Replies {})
  let second <- Test.get (Replies { page: 2 })
  Task.succeed ({ first: List.length first.model.rows, second: List.length second.model.rows, pages: Test.texts "al-pagination span" first.doc } == { first: 20, second: 1, pages: ["Previous", "Page 1 of 2"] })
}
```

Read it as chapter 6 reads a page. The parameter `page` says which page
of rows to show, 20 to a page, newest first; `load` clamps it, reads one
page with `Db.limit` and `Db.offset`, and counts the rows for the
number of pages. The `view` is an `al-table` (Appendix C) with a column
per field except the id: a `Bool` or a union would be an `al-badge`, a
`Time` is formatted, and a reference shows the id it holds. The
`al-pagination` links to the pages before and after.

The two tests are there to kill the page's mutants (chapter 10). The
first inserts two rows and checks their order, so a reversed `orderBy`
fails it. The second inserts 21 rows and checks that the first page has
20 and the second 1, so a dropped `limit` or `offset` fails it. Every
generated test is written that way: for what the code next to it could
get wrong.

The other options of `alder new page`:

- `--show T` writes a page for one row, at `<path>/{id}` (below).
- `--from T U …` loads every row of each table and shows each in a card,
  a skeleton for a page that brings several tables together, which you
  then design.
- `--requires user` adds `requires user` to the page, and a test that a
  visitor is sent to log in. With `--list T` on a table with an `@owner`
  column, the page reads `Db.mine T`, the user's own rows (chapter 6),
  and its test inserts another user's row to check that it is not
  shown. With `--show T`, it reads `Db.getMine`.
- `--role R` adds `, role R`, and a test that a user of another role
  gets no page. It implies `--requires user`, and is refused when the
  session has no `role` with a constructor `R`.

## A form: `alder new form`

A form is added to a page that exists. Its fields are written like a
table's:

```text command=guestbook
$ alder new form Replies NewReply entry:Entry.EntryId message:Text.NonEmpty
edited app/Replies.alder
--- a/app/Replies.alder
+++ b/app/Replies.alder
@@ -29,6 +29,21 @@
   Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
 }
 
+form NewReply = {
+  entry: Entry.EntryId,
+  message: Text.NonEmpty,
+}
+
+msg SubmitNewReply form NewReply
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  SubmitNewReply p -> {
+    let _ <- Db.insert Reply { entry: p.form.entry, message: p.form.message }
+    Task.succeed (Redirect (Replies {}))
+  },
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Replies">
@@ -57,6 +72,16 @@
       <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
       {if model.page < model.pages then <a slot="next" href={Replies { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
     </al-pagination>
+
+    <al-card>
+      <h2 slot="header">New reply</h2>
+      <form submit={SubmitNewReply}>
+        <al-input field="entry" label="Entry" />
+        <al-textarea field="message" label="Message" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New reply</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
 
 test "the list shows the rows, newest first" = {
@@ -74,3 +99,18 @@
   let second <- Test.get (Replies { page: 2 })
   Task.succeed ({ first: List.length first.model.rows, second: List.length second.model.rows, pages: Test.texts "al-pagination span" first.doc } == { first: 20, second: 1, pages: ["Previous", "Page 1 of 2"] })
 }
+
+test "submitting NewReply adds a row" = {
+  let entry1 <- Db.insert Entry { name: Text.NonEmpty "name-1", message: Text.NonEmpty "message-1" }
+  let _ <- Test.get (Replies {})
+  let r <- Test.submit NewReply { entry: entry1, message: Text.NonEmpty "message-1" }
+  let rows <- Db.from Reply |> Db.all
+  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { entry: row.entry, message: row.message, hidden: row.hidden }) rows } == { answer: Redirected (Replies {}), rows: [{ entry: entry1, message: Text.NonEmpty "message-1", hidden: False }] })
+}
+
+test "NewReply refuses an empty message" = {
+  let _ <- Test.get (Replies {})
+  let r <- Test.submitRaw NewReply [{ name: "entry", value: "x" }, { name: "message", value: "" }]
+  let rows <- Db.from Reply |> Db.count
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "message", message: "must not be empty" }], rows: 0 })
+}
```

The form's name, `NewReply`, must not be the table's: a form declares a
type, and a form named `Reply` would hide the table's type `Reply` in
this module (the copy would not check, so nothing would be written).

The generator wrote four things into the page. The `form` declaration
and its message, `msg SubmitNewReply form NewReply`. An `update` with
the message's arm, since the page had none. The form itself, in `view`:
in a new `al-card` at the end of the page (in the page's last `al-card`
if it has one), with an input per field (`al-textarea` for a `Text`
field named `body`, `message` or `description`, `al-select` for a union,
`al-checkbox` for a `Bool`), the form's errors in an `al-alert`, and the
button. And two tests: one submits the form and compares the stored row,
the defaults of the fields the form leaves out included (`hidden:
False`); the other submits an empty `message` and expects the decoder's
`FormRejected`.

The arm inserts a `Reply`, because every field of the form is a field of
`Reply` with the same name and type, and the form names every field an
insert must have. That is the rule: when the fields fit one table, the
arm is that table's insert; `--into T` names the table when two would
fit. When they fit none, the arm is a **hole**.

## A page for a row, and a message: `--show` and `alder new msg`

A page for one reply:

```text command=guestbook
$ alder new page Replies.Show /replies --show Reply
wrote app/Replies/Show.alder
```

Its path is `/replies/{id}`, with `id: Reply.ReplyId`. Its `load` reads
the row with `Db.get` and answers `Query.notFound` when there is none,
and its tests check both: the page shows the row's fields, and a missing
id is `PageMissing`.

Now a button that deletes the reply. `alder new msg` adds a message to a
page, and a button that sends it:

```text command=guestbook
$ alder new msg Replies.Show Delete
edited app/Replies/Show.alder
--- a/app/Replies/Show.alder
+++ b/app/Replies/Show.alder
@@ -17,6 +17,13 @@
   }
 }
 
+msg Delete
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Delete p -> ?delete,
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Reply">
@@ -38,6 +45,8 @@
         <dd>{Time.format "Jan 2, 2006" model.row.updated}</dd>
       </dl>
     </al-card>
+
+    <al-button click={Delete}>Delete</al-button>
   </Layout.Page>
 
 test "the page shows the row" = {
@@ -51,3 +60,17 @@
   let r <- Test.open (Replies.Show { id: Reply.ReplyId "missing" })
   Task.succeed (r.answer == PageMissing)
 }
+
+-- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
+-- deleted the page's row, and `after.doc` its HTML), as
+-- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Delete" = {
+  let entry1 <- Db.insert Entry { name: Text.NonEmpty "name-1", message: Text.NonEmpty "message-1" }
+  let reply1 <- Db.insert Reply { entry: entry1, message: Text.NonEmpty "message-1" }
+  let _ <- Test.get (Replies.Show { id: reply1 })
+  let r <- Test.click Delete
+  let after <- Test.open (Replies.Show { id: reply1 })
+  Task.succeed ?expected
+}
app/Replies/Show.alder:24:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Replies/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
```

This time the generator does not know what the message should do, so
it does not guess. The arm is the hole `?delete`, of type `Task Nav`: an
`update` arm is a task that ends with what the browser does next
(chapter 7). The test is written in full, except for its verdict: the
hole `?expected`, of type `Bool`. Its comment says what to put there.
After the click the test opens the page again with `Test.open`, which,
unlike `Test.get`, does not fail when the page is gone: a message may
delete the very row its page shows, as this one will, and then
`after.answer` is `PageMissing`.

## Holes, and filling them: `alder fill`

A **hole** is a name with a `?` in front, written where an expression
goes (chapter 11, "Holes"). The checker infers the type the expression must have
and reports it as the warning W0002: `hole ?delete has type Task Nav`. A
hole is not an error, so the app checks, and the generator wrote it.
But it cannot run:

```text command=guestbook
$ alder test
app/Replies/Show.alder:24:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Replies/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder test: no tests ran (fill the hole)
```

A test program with a hole in it would have nothing to run where the
hole is, so `alder test` refuses to start. After `alder new msg`, and
after `alder new form` when the fields fit no table, the order is
therefore **new, fill, test**: generate, fill each hole, then run the
tests.

`alder fill --list` lists the holes of the app, each with its module,
its declaration (an arm is `update.<Message>`, a test is its name in
quotes) and the type it must have:

```text command=guestbook
$ alder fill --list
Replies.Show update.Delete ?delete : Task Nav
Replies.Show "pressing Delete" ?expected : Bool
```

`alder fill <address> -` reads one expression from standard input and
puts it where the hole is, then checks the copy, and writes only if it
checks. Here the arm only deletes the row and forgets to say what the
browser does next, so its type is `Task Unit`, not `Task Nav`:

```text command=guestbook
$ alder fill Replies.Show.update.Delete - <<'EOF'
Db.delete Reply model.row.id
EOF
app/Replies/Show.alder:24:15: error[E0003]: expected Task Nav, got Task Unit
    Delete p -> Db.delete Reply model.row.id,
                ^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Replies/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
```

Nothing was written. The checker's message is the whole explanation, and
the other hole is still listed among the warnings. The right
expression is a block, which is one expression too:

```text command=guestbook
$ alder fill Replies.Show.update.Delete - <<'EOF'
{
  let _ <- Db.delete Reply model.row.id
  Task.succeed (Redirect (Replies {}))
}
EOF
edited app/Replies/Show.alder
--- a/app/Replies/Show.alder
+++ b/app/Replies/Show.alder
@@ -21,7 +21,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Delete p -> ?delete,
+  Delete p -> {
+    let _ <- Db.delete Reply model.row.id
+    Task.succeed (Redirect (Replies {}))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Replies/Show.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
```

`fill` indents the expression to fit where it goes. When the expression
names a module of the app qualified, such as `Member.isAdmin`, and the
page does not import it, `fill` adds the import and says so: `added
import Member to app/Replies/Show.alder` (with `--json`, `"imports":
["Member"]`). A standard library module needs no import, and a name
that is not a module's is the checker's to report. An address names one declaration: an arm
(`Replies.Show.update.Delete`), a test (`Replies.Show."pressing
Delete"`) or any other top-level name (`Replies.perPage`). When the
declaration has more than one hole, `--hole ?name` picks one. The
expression may contain holes of its own, which `fill` lists, to be
filled next.

The test's verdict (the comment that said what to put there goes with
the hole):

```text command=guestbook
$ alder fill 'Replies.Show."pressing Delete"' - <<'EOF'
{ answer: r.answer, after: after.answer } == { answer: Redirected (Replies {}), after: PageMissing }
EOF
edited app/Replies/Show.alder
--- a/app/Replies/Show.alder
+++ b/app/Replies/Show.alder
@@ -64,16 +64,11 @@
   Task.succeed (r.answer == PageMissing)
 }
 
--- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Delete" = {
   let entry1 <- Db.insert Entry { name: Text.NonEmpty "name-1", message: Text.NonEmpty "message-1" }
   let reply1 <- Db.insert Reply { entry: entry1, message: Text.NonEmpty "message-1" }
   let _ <- Test.get (Replies.Show { id: reply1 })
   let r <- Test.click Delete
   let after <- Test.open (Replies.Show { id: reply1 })
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, after: after.answer } == { answer: Redirected (Replies {}), after: PageMissing })
 }
```

And the tests run:

```text command=guestbook
$ alder test
ok   About "the page is shown"
ok   Home "signing adds an entry"
ok   Home "entries are listed newest first"
ok   Reply "deleting its entry deletes a Reply"
ok   Replies "the list shows the rows, newest first"
ok   Replies "the list has 20 rows a page"
ok   Replies "submitting NewReply adds a row"
ok   Replies "NewReply refuses an empty message"
ok   Replies.Show "the page shows the row"
ok   Replies.Show "a missing row is not found"
ok   Replies.Show "pressing Delete"
11 passed, 0 failed
```

## Refusals

A generator refuses rather than overwrite, and rather than write
something that does not check. A refusal is one sentence on standard
error, after any diagnostics; the exit code is 1, and nothing is
written. A module that exists:

```text command=guestbook
$ alder new page About /about
alder new page: module `About` exists (`app/About.alder`): a generator never overwrites a file; choose another module name
```

A result that does not check. `--into Reply` asks for a form that
inserts a `Reply`, but the form has no `entry`, and an insert must have
one:

```text command=guestbook
$ alder new form Replies.Show Note message:Text.NonEmpty --into Reply
app/Replies/Show.alder:35:30: error[E0003]: record `Reply.Insert Reply` is missing field(s): entry
      let _ <- Db.insert Reply { message: p.form.message }
                               ^^^^^^^^^^^^^^^^^^^^^^^^^^^
alder new form: the result does not check (the errors above): nothing was written
```

Why refuse instead of overwrite, or instead of writing and letting you
fix it? A file you wrote may hold work, and a generator cannot know
which of its lines matter: overwriting is the one mistake it must never
make. And code that does not check is a problem handed to the next
person, or the next agent, without the context of the command that made
it. The refusal keeps the app where it was and says why, while the
command and its arguments are still in front of you.

## Output for programs: `--json`

With `--json`, a command prints one JSON object on one line instead of
its text:

```text command=guestbook
$ alder new page Contact /contact --json
{"files":[{"path":"app/Contact.alder","action":"created"}],"diff":"","diagnostics":[],"holes":[],"written":true}
```

`files` lists the files written, each `created` or `edited`; `diff` is
the unified diff of the edited ones; `diagnostics` are the copy's, in
the form `alder check --json` prints them (chapter 11); `holes` lists
each hole with its module, declaration, arm, name, type and position;
`written` says whether anything was written. A table's command adds
`migration`, the migration's steps, and a refusal adds `error`, its
sentence. This object is the contract for programs that drive the
generators, such as an agent's harness (E11, below).

## The admin: `alder new admin`

`alder new admin` writes an admin for the whole app: for every table, a
page that lists its rows, a page for each row, a form to create one, a
form to edit one, and a page to delete one; and a dashboard over all of
them. It is what Django's and Rails' admins are, but written out as
plain Alder files, with tests, which you can read and change.

### What it needs

Every page of the admin `requires user, role Admin` (chapter 6,
"Rules"), so the app needs a logged-in user and a role. The guestbook
has neither:

```text command=guestbook
$ alder new admin
alder new admin: `alder new admin` needs a session with a `role` whose type has an `Admin` constructor: this app's session has no `user`; add a user table, a login that sets `user`, and a `role` (chapter 6, "Rules" and "Roles")
```

The sentence says what to add, and the generator does not add it for
you: it never edits `App` or the login. Who is an admin is a decision
about your app's users, not a detail to generate. To meet it (on an app
without a login, `alder new type User "Role = Member | Admin"` and
`alder new auth --role Role` do all three: "Logging in", below):

1. Give the user table a role: `type Role = Member | Admin` (any
   constructors, one of them `Admin`) and a column `role: Role =
   Member`.
2. Put the role in the session: `session { user: Maybe User.UserId,
   role: User.Role = Member }`.
3. Set it at login from the user's row, and put the default back at
   logout (chapter 6, "Roles", shows both).

Both examples do this. The to-do app's roles are `Member | Admin`; the
blog's are `Reader | Author | Admin`, where an Author writes posts on
the blog's own `Write` pages and an Admin manages everything through the
generated admin. In the blog (this transcript is the blog's history, from
before `app/Admin/` existed, so the book's test does not run it; the
blog's README replays it, file for file):

```text
$ alder new admin
wrote app/Admin/Comment/Delete.alder
wrote app/Admin/Comment/Edit.alder
wrote app/Admin/Comment/Index.alder
wrote app/Admin/Comment/New.alder
wrote app/Admin/Comment/Show.alder
wrote app/Admin/Index.alder
wrote app/Admin/Layout.alder
wrote app/Admin/Post/Delete.alder
wrote app/Admin/Post/Edit.alder
wrote app/Admin/Post/Index.alder
wrote app/Admin/Post/New.alder
wrote app/Admin/Post/Show.alder
wrote app/Admin/User/Delete.alder
wrote app/Admin/User/Edit.alder
wrote app/Admin/User/Index.alder
wrote app/Admin/User/New.alder
wrote app/Admin/User/Show.alder
```

`examples/blog/app/Admin/` holds exactly these files, as the command
wrote them; nobody has edited them, and the blog's README says how to
check that by running it again. Its 97 tests pass, and they kill all of
the admin's 151 mutants (chapter 10). For each table `T`, with `t` its
name in lower case:

| Module | Path | What it is |
|---|---|---|
| `Admin.Index` | `/admin` | the dashboard: per table, how many rows and the five newest |
| `Admin.Layout` | (a component) | `Admin.Layout.Page`: the app's layout, with a nav of the tables |
| `Admin.T.Index` | `/admin/t` | the rows: search, sort by any column, 25 to a page |
| `Admin.T.Show` | `/admin/t/{id}` | one row, every column, with Edit and Delete |
| `Admin.T.New` | `/admin/t/new` | the form that creates a row |
| `Admin.T.Edit` | `/admin/t/{id}/edit` | the form that changes a row |
| `Admin.T.Delete` | `/admin/t/{id}/delete` | what goes with the row, and the button, or why there is none |

`--module` and `--path` put the admin elsewhere (`--module Back.Office
--path /back`). An admin that exists is not written again: `alder new
admin` refuses, and says that `alder regen admin` updates it.

### The dashboard and the layout

The layout is a component that wraps the app's own `Layout.Page`, so the
admin looks like the rest of the app, with a nav of the tables:

```alder part=blog/Admin.Layout
-- generated by `alder new admin`; regions between
-- `-- @admin begin <name>` / `-- @admin end` are rewritten by `alder regen admin`
module Admin.Layout

-- @admin begin imports
import Layout
import User
-- @admin end

-- The admin's frame: the app's layout around a nav of every table's list.
Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <Layout.Page title={props.title ++ " · Admin"}>
    <nav aria-label="Admin">
      <al-stack direction="row" gap="sm">
        <a href={Admin.Index {}}>Dashboard</a>
        -- @admin begin nav
        <a href={Admin.Comment.Index {}}>Comments</a>
        <a href={Admin.Post.Index {}}>Posts</a>
        <a href={Admin.User.Index {}}>Users</a>
        -- @admin end
      </al-stack>
    </nav>
    {props.children}
  </Layout.Page>

-- @admin begin access
-- isAdmin is whether the signed-in user may use the admin, which every page's load asks (it runs
-- before every message too). The session's role is the one read at login, and lasts until
-- logout or the cookie's 30 days (chapter 6, "Rules"): the user table holds the role, so it is
-- read again, and a user demoted or deleted since gets a 404 at once.
isAdmin : User.UserId -> Query Bool
isAdmin id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Admin)
}
-- @admin end
```

The first two lines say where the file came from, and every file of the
admin has them. They name no version, so the same app gives the same
files from any build of `alder`.

The region `access` is the admin's own check. The session's role is the
one read at login, and the cookie keeps it until logout or its 30 days
(chapter 6, "Rules"); an Admin demoted, or deleted, since would keep the
admin all that time. So every page's `load` begins by asking `isAdmin`,
which reads the user's row again, and answers `Query.notFound` when the
row is gone or no longer says `Admin`; `load` runs before every message
too, so a form posted by a demoted Admin is refused the same way. This
is chapter 6's re-read idiom, generated. When the user table has no
column holding the role, there is nothing to read again: `isAdmin` is
`Query.succeed True`, the `page` line's role is the whole check, and the
dashboard's test says that the cookie's role lasts until logout or its
30 days. The lines between `-- @admin begin <name>` and `-- @admin
end` are **owned regions**: the parts that follow the tables, which
`alder regen admin` rewrites when the tables change ("Owned regions",
below). Everything else is yours.

The dashboard reads, for each table, its count and its five newest rows
(by `@created`, else by id), each labelled by its first `Text` field and
linked to its page:

```alder part=blog/Admin.Index
-- generated by `alder new admin`; regions between
-- `-- @admin begin <name>` / `-- @admin end` are rewritten by `alder regen admin`
module Admin.Index

-- @admin begin imports
import Admin.Layout
import Comment
import Post
import User
-- @admin end

page "/admin" { } requires user, role Admin

-- A card of the dashboard: a table's count and its newest rows, each linking to its page.
type Card = { key: Text, title: Text, href: Page, count: Int, rows: List { label: Text, href: Page } }

type Model = { cards: List Card }

-- recent is how many of a table's newest rows its card lists.
recent : Int
recent = 5

load : Params -> Session -> Query Model
load params session = {
  let admin <- Admin.Layout.isAdmin session.user
  if admin then {
    -- @admin begin cards
    let comments <- Db.from Comment |> Db.orderBy Comment.created.desc |> Db.limit recent |> Db.all
    let commentCount <- Db.from Comment |> Db.count
    let posts <- Db.from Post |> Db.orderBy Post.created.desc |> Db.limit recent |> Db.all
    let postCount <- Db.from Post |> Db.count
    let users <- Db.from User |> Db.orderBy User.created.desc |> Db.limit recent |> Db.all
    let userCount <- Db.from User |> Db.count
    Query.succeed { cards: [
      {
        key: "comments",
        title: "Comments",
        href: Admin.Comment.Index {},
        count: commentCount,
        rows: List.map (\row -> {
          label: row.author,
          href: Admin.Comment.Show { id: row.id },
        }) comments,
      },
      {
        key: "posts",
        title: "Posts",
        href: Admin.Post.Index {},
        count: postCount,
        rows: List.map (\row -> { label: row.slug, href: Admin.Post.Show { id: row.id } }) posts,
      },
      {
        key: "users",
        title: "Users",
        href: Admin.User.Index {},
        count: userCount,
        rows: List.map (\row -> { label: row.email, href: Admin.User.Show { id: row.id } }) users,
      },
    ] }
    -- @admin end
  } else Query.notFound
}
```

The `load` is one owned region, `cards`, since a table added later needs
a card. The `view` is not: it draws whatever cards `load` gives it, and
its headings and layout are yours to change.

```alder part=blog/Admin.Index
view : Model -> Session -> Html Msg
view model session =
  <Admin.Layout.Page title="Dashboard">
    <h1>Admin</h1>

    <al-stack gap="md">
      {for card in model.cards ->
        <al-card id={card.key}>
          <h2 slot="header"><a href={card.href}>{card.title}</a> <al-badge variant="secondary">{Text.fromInt card.count}</al-badge></h2>
          <ul>
            {for row in card.rows -> <li><a href={row.href}>{row.label}</a></li>}
          </ul>
        </al-card>}
    </al-stack>
  </Admin.Layout.Page>
```

Then its tests, in the region `tests`. A test that kills a mutant of the
generated code is part of what the generator writes, so the tests follow
the tables like the code does. Each table's card is tested with more rows
than it shows, so that the count, the order and the limit of five are
each checked;
then come the rules: a visitor is sent to log in; a Reader gets
`PageMissing`, even one whose row says Admin since, because the cookie's
role, the one read at login, is what the `page` line checks (chapter 6,
"A role is read at login"); and a demoted Admin, whose cookie still says
Admin, is refused at once.

```alder part=blog/Admin.Index
-- @admin begin tests
test "the dashboard counts the comments and lists the newest five" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let post1 <- Db.insert Post {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-1",
    body: "body-1",
    author: me,
  }
  let _ <- Task.all (List.map (\n -> Db.insert Comment {
    post: post1,
    author: Comment.Name ("author-" ++ Text.fromInt n),
    body: Comment.Body ("body-" ++ Text.fromInt n),
  }) (List.range 1 6))
  let page <- Test.get (Admin.Index {})
  Task.succeed ({
    count: Test.text "#comments al-badge" page.doc,
    rows: Test.texts "#comments li" page.doc,
  } == {
    count: Just "6",
    rows: ["author-6", "author-5", "author-4", "author-3", "author-2"],
  })
}

test "the dashboard counts the posts and lists the newest five" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let _ <- Task.all (List.map (\n -> Db.insert Post {
    slug: Post.Slug ("slug-" ++ Text.fromInt n),
    title: Text.NonEmpty ("title-" ++ Text.fromInt n),
    body: "body-" ++ Text.fromInt n,
    author: me,
  }) (List.range 1 6))
  let page <- Test.get (Admin.Index {})
  Task.succeed ({
    count: Test.text "#posts al-badge" page.doc,
    rows: Test.texts "#posts li" page.doc,
  } == {
    count: Just "6",
    rows: ["slug-6", "slug-5", "slug-4", "slug-3", "slug-2"],
  })
}

test "the dashboard counts the users and lists the newest five" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let _ <- Task.all (List.map (\n -> Db.insert User {
    email: Text.Email ("email-" ++ Text.fromInt n ++ "@example.com"),
    name: Text.NonEmpty ("name-" ++ Text.fromInt n),
    passwordHash: "passwordhash-" ++ Text.fromInt n,
  }) (List.range 2 7))
  let page <- Test.get (Admin.Index {})
  Task.succeed ({
    count: Test.text "#users al-badge" page.doc,
    rows: Test.texts "#users li" page.doc,
  } == {
    count: Just "7",
    rows: [
      "email-7@example.com",
      "email-6@example.com",
      "email-5@example.com",
      "email-4@example.com",
      "email-3@example.com",
    ],
  })
}

test "a visitor is sent to log in" = {
  let r <- Test.open (Admin.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a Reader gets no page, even one made Admin since logging in" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Reader }
  let r <- Test.open (Admin.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a demoted Admin is refused at once" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Reader,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let r <- Test.open (Admin.Index {})
  Task.succeed (r.answer == PageMissing)
}
-- @admin end
```

### A list

`Admin.Post.Index` is the list of posts. Its parameters carry the search
text, the sort column and direction, and the page:

```alder part=blog/Admin.Post.Index
-- generated by `alder new admin`; regions between
-- `-- @admin begin <name>` / `-- @admin end` are rewritten by `alder regen admin`
module Admin.Post.Index

-- @admin begin imports
import Admin.Layout
import Post
import User
-- @admin end

page "/admin/post" { q: Text = "", sort: Text = "created", dir: Text = "desc", page: Int = 1 } requires user, role Admin

type Model = {
  rows: List Post,
  labels: Labels,
  q: Text,
  sort: Text,
  dir: Text,
  page: Int,
  pages: Int,
  count: Int,
}

perPage : Int
perPage = 25

-- @admin begin search
-- search is the rows whose text contains q (every row for "").
search : Text -> Condition Post
search q = Db.or [Post.slug.contains q, Post.title.contains q, Post.body.contains q]
-- @admin end

-- @admin begin sorts
-- sorts are the columns the list sorts by, by name.
sorts : List { name: Text, asc: Order Post, desc: Order Post }
sorts = [
  { name: "slug", asc: Post.slug.asc, desc: Post.slug.desc },
  { name: "title", asc: Post.title.asc, desc: Post.title.desc },
  { name: "body", asc: Post.body.asc, desc: Post.body.desc },
  { name: "published", asc: Post.published.asc, desc: Post.published.desc },
  { name: "author", asc: Post.author.asc, desc: Post.author.desc },
  { name: "created", asc: Post.created.asc, desc: Post.created.desc },
]
-- @admin end

-- order is the ordering sort names, ascending when dir is "asc"; a name that is not a column's
-- sorts newest first.
order : Text -> Text -> Order Post
order sort dir = match List.find (\s -> s.name == sort) sorts {
  Just s -> if dir == "asc" then s.asc else s.desc,
  Nothing -> Post.created.desc,
}

-- load reads how many rows match the search, and one page of them (the last, for a page past it),
-- in the order asked for. The search text is cut to 200 characters (Text.slice): a text over
-- 10,000 bytes would fail the query (chapter 8, "Search").
```

`search` is a `Db.or` of `.contains` over every `Text` column that is
not an id or a reference (chapter 8, "Search": case-insensitive, and
`%` and `_` match themselves); `sorts` names every
column with its two orders, and `order` picks one by the name in the
address, newest first for a name that is not a column's. Both follow the
table, so both are regions.

```alder part=blog/Admin.Post.Index
load : Params -> Session -> Query Model
load params session = {
  let admin <- Admin.Layout.isAdmin session.user
  if admin then {
    let q = Text.slice 0 200 params.q
    let count <- Db.from Post |> Db.where (search q) |> Db.count
    let pages = Int.max 1 ((count + perPage - 1) / perPage)
    let page = Int.clamp { min: 1, max: pages } params.page
    let rows <-
      Db.from Post
        |> Db.where (search q)
        |> Db.orderBy (order params.sort params.dir)
        |> Db.limit perPage
        |> Db.offset ((page - 1) * perPage)
        |> Db.all
    let labels <- labelsOf rows
    Query.succeed {
      rows: rows,
      labels: labels,
      q: q,
      sort: params.sort,
      dir: params.dir,
      page: page,
      pages: pages,
      count: count,
    }
  } else Query.notFound
}

-- @admin begin labels
-- Labels are the rows the references of the rows shown name: their links show their labels.
type Labels = { authors: List User }

-- labelsOf reads the rows that rows refer to.
labelsOf : List Post -> Query Labels
labelsOf rows = {
  let authors <- Db.from User |> Db.where (User.id.in (List.map (\row -> row.author) rows)) |> Db.all
  Query.succeed { authors: authors }
}

-- authorLabel is the label of the user id names (the id itself if it names none).
authorLabel : Labels -> User.UserId -> Text
authorLabel labels id = match List.find (\r -> r.id == id) labels.authors {
  Just r -> r.email,
  Nothing -> id,
}
-- @admin end
```

`load` cuts the search text to 200 characters before it reaches the
query: a search text of more than 10,000 bytes fails the query (chapter
8, "Search"), and the address is anyone's to type. It counts first, so that a page past
the last is clamped to the last. A reference (here a post's `author`) is
shown by its label, the user's email, rather than by its id: `labelsOf`
reads only the users the page's rows refer to, with one query
(`.in`), not one per row.

```alder part=blog/Admin.Post.Index
view : Model -> Session -> Html Msg
view model session =
  <Admin.Layout.Page title="Posts">
    <h1>Posts</h1>

    <al-stack direction="row" gap="sm">
      <form method="get" action="/admin/post">
        <input type="search" name="q" value={model.q} aria-label="Search" />
        <input type="hidden" name="sort" value={model.sort} />
        <input type="hidden" name="dir" value={model.dir} />
      </form>
      <al-button href={Admin.Post.New {}}>New post</al-button>
    </al-stack>

    <p id="count">{if model.count == 1 then "1 row" else Text.fromInt model.count ++ " rows"}</p>

    <al-table>
      <table>
        <thead>
          <tr>
            -- @admin begin headers
            <th>
              <a href={Admin.Post.Index {
                q: model.q,
                sort: "slug",
                dir: if model.sort == "slug" && model.dir == "asc" then "desc" else "asc",
              }}>Slug</a>
            </th>
            <th>
              <a href={Admin.Post.Index {
                q: model.q,
                sort: "title",
                dir: if model.sort == "title" && model.dir == "asc" then "desc" else "asc",
              }}>Title</a>
            </th>
            <th>
              <a href={Admin.Post.Index {
                q: model.q,
                sort: "body",
                dir: if model.sort == "body" && model.dir == "asc" then "desc" else "asc",
              }}>Body</a>
            </th>
            <th>
              <a href={Admin.Post.Index {
                q: model.q,
                sort: "published",
                dir: if model.sort == "published" && model.dir == "asc" then "desc" else "asc",
              }}>Published</a>
            </th>
            <th>
              <a href={Admin.Post.Index {
                q: model.q,
                sort: "author",
                dir: if model.sort == "author" && model.dir == "asc" then "desc" else "asc",
              }}>Author</a>
            </th>
            <th>
              <a href={Admin.Post.Index {
                q: model.q,
                sort: "created",
                dir: if model.sort == "created" && model.dir == "asc" then "desc" else "asc",
              }}>Created</a>
            </th>
            -- @admin end
          </tr>
        </thead>
        <tbody>
          {for row in model.rows ->
            <tr>
              -- @admin begin cells
              <td><a href={Admin.Post.Show { id: row.id }}>{row.slug}</a></td>
              <td>{row.title}</td>
              <td>{row.body}</td>
              <td>{if row.published then <al-badge>Yes</al-badge> else <al-badge variant="secondary">No</al-badge>}</td>
              <td><a href={Admin.User.Show { id: row.author }}>{authorLabel model.labels row.author}</a></td>
              <td>{Time.format "Jan 2, 2006" row.created}</td>
              -- @admin end
            </tr>}
        </tbody>
      </table>
    </al-table>

    <al-pagination role="navigation" aria-label="Pages">
      {if model.page > 1 then <a slot="prev" href={Admin.Post.Index { q: model.q, sort: model.sort, dir: model.dir, page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
      <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
      {if model.page < model.pages then <a slot="next" href={Admin.Post.Index { q: model.q, sort: model.sort, dir: model.dir, page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
    </al-pagination>
  </Admin.Layout.Page>
```

The headers link to the same list sorted by that column, the other way
round when it already is. The cells render each column by its type: a
`Bool` as a badge, a `Time` formatted, a reference as a link to its
row's admin page, and the first column as a link to the row's own page.

```alder part=blog/Admin.Post.Index
-- @admin begin tests
test "the list shows every row, newest first, and the labels of the rows they refer to" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let post1 <- Db.insert Post {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-1",
    body: "body-1",
    author: me,
  }
  let post2 <- Db.insert Post {
    slug: Post.Slug "slug-2",
    title: Text.NonEmpty "title-2",
    body: "body-2",
    author: me,
  }
  let user2 <- Db.insert User {
    email: Text.Email "email-2@example.com",
    name: Text.NonEmpty "name-2",
    passwordHash: "passwordhash-2",
  }
  let page <- Test.get (Admin.Post.Index {})
  Task.succeed ({
    rows: List.map (\row -> row.id) page.model.rows,
    authors: List.length page.model.labels.authors,
    authorLabel: authorLabel page.model.labels me,
  } == {
    rows: [post2, post1],
    authors: 1,
    authorLabel: "email-1@example.com",
  })
}

test "sorting by slug goes both ways" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let post1 <- Db.insert Post {
    slug: Post.Slug "slug-2",
    title: Text.NonEmpty "title-1",
    body: "body-1",
    author: me,
  }
  let post2 <- Db.insert Post {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-2",
    body: "body-2",
    author: me,
  }
  let up <- Test.get (Admin.Post.Index { sort: "slug", dir: "asc" })
  let down <- Test.get (Admin.Post.Index { sort: "slug" })
  Task.succeed ({
    up: List.map (\row -> row.id) up.model.rows,
    down: List.map (\row -> row.id) down.model.rows,
  } == {
    up: [post2, post1],
    down: [post1, post2],
  })
}

test "searching finds the rows that contain the text" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let post1 <- Db.insert Post {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-1",
    body: "body-1",
    author: me,
  }
  let post2 <- Db.insert Post {
    slug: Post.Slug "slug-2",
    title: Text.NonEmpty "title-2",
    body: "body-2",
    author: me,
  }
  let page <- Test.get (Admin.Post.Index { q: "slug-2" })
  Task.succeed ({
    rows: List.map (\row -> row.id) page.model.rows,
    count: Test.text "#count" page.doc,
  } == {
    rows: [post2],
    count: Just "1 row",
  })
}

test "a search text of 20,000 characters is cut to 200" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let long = Text.join "" (List.map (\n -> "z") (List.range 1 20000))
  let page <- Test.get (Admin.Post.Index { q: long })
  Task.succeed ({
    q: Text.length page.model.q,
    rows: List.length page.model.rows,
  } == {
    q: 200,
    rows: 0,
  })
}

test "the list has 25 rows a page, and a page past the last is the last" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let _ <- Task.all (List.map (\n -> Db.insert Post {
    slug: Post.Slug ("slug-" ++ Text.fromInt n),
    title: Text.NonEmpty ("title-" ++ Text.fromInt n),
    body: "body-" ++ Text.fromInt n,
    author: me,
  }) (List.range 1 26))
  let first <- Test.get (Admin.Post.Index {})
  let second <- Test.get (Admin.Post.Index { page: 2 })
  let past <- Test.get (Admin.Post.Index { page: 9 })
  Task.succeed ({
    first: List.length first.model.rows,
    second: List.length second.model.rows,
    past: { page: past.model.page, rows: List.length past.model.rows },
    count: Test.text "#count" first.doc,
    pages: Test.texts "al-pagination span" first.doc,
  } == {
    first: 25,
    second: 1,
    past: { page: 2, rows: 1 },
    count: Just "26 rows",
    pages: ["Previous", "Page 1 of 2"],
  })
}

test "a visitor is sent to log in" = {
  let r <- Test.open (Admin.Post.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a Reader gets no page, even one made Admin since logging in" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Reader }
  let r <- Test.open (Admin.Post.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a demoted Admin is refused at once" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Reader,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let r <- Test.open (Admin.Post.Index {})
  Task.succeed (r.answer == PageMissing)
}
-- @admin end
```

### A form

`Admin.Post.New` creates a post. Its form has a field per column but
the id, `@created` and `@updated`:

```alder part=blog/Admin.Post.New
-- generated by `alder new admin`; regions between
-- `-- @admin begin <name>` / `-- @admin end` are rewritten by `alder regen admin`
module Admin.Post.New

-- @admin begin imports
import Admin.Layout
import Post
import User
-- @admin end

page "/admin/post/new" { } requires user, role Admin

type Model = {
  -- @admin begin model
  authors: List User,
  -- @admin end
}

load : Params -> Session -> Query Model
load params session = {
  let admin <- Admin.Layout.isAdmin session.user
  if admin then {
    -- @admin begin load
    let authors <- Db.from User |> Db.orderBy User.email.asc |> Db.all
    Query.succeed { authors: authors }
    -- @admin end
  } else Query.notFound
}

-- @admin begin form
form PostForm = {
  slug: Post.Slug,
  title: Text.NonEmpty,
  body: Text,
  published: Bool,
  author: User.UserId,
}
-- @admin end

msg Create form PostForm

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    -- @admin begin insert
    let slugTaken <- Db.from Post |> Db.where (Post.slug.eq p.form.slug) |> Db.count
    if slugTaken > 0 then
      Task.succeed (Reject "A post with this slug exists.")
    else {
      let id <- Db.insert Post {
        slug: p.form.slug,
        title: p.form.title,
        body: p.form.body,
        published: p.form.published,
        author: p.form.author,
      }
      Task.succeed (Redirect (Admin.Post.Show { id: id }))
    }
    -- @admin end
  },
}
```

Two things in it were not in `alder new form`'s forms. `slug` is
`@unique`, and inserting a second post with a slug that exists would
fail in the database, which a user would see as an error page; so the
arm counts the posts with that slug first, and answers `Reject` with a
sentence when there is one. And `author` refers to a user, so `load`
reads the users to choose from, ordered by their label.

```alder part=blog/Admin.Post.New
view : Model -> Session -> Html Msg
view model session =
  <Admin.Layout.Page title="New post">
    <h1>New post</h1>

    <al-card>
      <form submit={Create}>
        -- @admin begin inputs
        <al-input field="slug" label="Slug" />
        <al-input field="title" label="Title" />
        <al-textarea field="body" label="Body" />
        <al-checkbox field="published" label="Published" value={False} />
        <al-select field="author" label="Author" options={List.map (\r -> { value: r.id, label: r.email }) model.authors} />
        -- @admin end
        <al-alert variant="destructive"><Errors form /></al-alert>
        <al-button type="submit">Create</al-button>
      </form>
    </al-card>
  </Admin.Layout.Page>
```

The author is an `al-select` with `options={…}` (chapter 5): a list of
`{ value, label }` records, one per user, whose values have the field's
type, `User.UserId`. The browser shows the emails and posts the id.

A request can be crafted by hand, so the list in the browser is no
guarantee: someone could post the id of a user who is not in it, or of a
row of another table, or a text that names no row. So **a submitted
value must be one of the options**. When the form is posted, `load` runs
first, as it does before every message (chapter 6), and the generated Go
computes the `options` expression again from that model; a value that
is not one of them is the field error "must be one of the options", and
`update` does not run. That is the **select membership rule**, and it
belongs to `<Select options>` everywhere, not only to the admin. It is
also why `options` may use only the model and the session, never a
variable of a `for` or a `let` of the view: the runtime must be able to
compute the options again from the model alone (chapter 5).

The tests of the form submit a valid post and compare what was stored,
submit a slug that exists, and submit an author that was not offered:

```alder part=blog/Admin.Post.New
-- @admin begin tests
test "creating a post adds it, with the values submitted" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let user2 <- Db.insert User {
    email: Text.Email "email-2@example.com",
    name: Text.NonEmpty "name-2",
    passwordHash: "passwordhash-2",
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-1",
    body: "body-1",
    author: me,
  }
  let page <- Test.get (Admin.Post.New {})
  let r <- Test.submit PostForm {
    slug: Post.Slug "slug-2",
    title: Text.NonEmpty "title-2",
    body: "body-2",
    published: True,
    author: user2,
  }
  let rows <- Db.from Post |> Db.where (Post.slug.eq (Post.Slug "slug-2")) |> Db.all
  Task.succeed ({
    answers: List.map (\row -> Redirected (Admin.Post.Show { id: row.id })) rows,
    rows: List.map (\row -> {
      slug: row.slug,
      title: row.title,
      body: row.body,
      published: row.published,
      author: row.author,
    }) rows,
    authors: Test.texts "select[name=author] option" page.doc,
  } == {
    answers: [r.answer],
    rows: [{ slug: Post.Slug "slug-2", title: Text.NonEmpty "title-2", body: "body-2", published: True, author: user2 }],
    authors: ["email-1@example.com", "email-2@example.com"],
  })
}

test "a post with a slug that exists is refused" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let user2 <- Db.insert User {
    email: Text.Email "email-2@example.com",
    name: Text.NonEmpty "name-2",
    passwordHash: "passwordhash-2",
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-1",
    body: "body-1",
    author: me,
  }
  let _ <- Test.get (Admin.Post.New {})
  let r <- Test.submit PostForm {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-2",
    body: "body-2",
    published: True,
    author: user2,
  }
  let rows <- Db.from Post |> Db.count
  Task.succeed ({
    answer: r.answer,
    rows: rows,
  } == {
    answer: FormRejected [{ field: "", message: "A post with this slug exists." }],
    rows: 1,
  })
}
```

```alder part=blog/Admin.Post.New
test "an author not offered is refused" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let user2 <- Db.insert User {
    email: Text.Email "email-2@example.com",
    name: Text.NonEmpty "name-2",
    passwordHash: "passwordhash-2",
  }
  let page <- Test.get (Admin.Post.New {})
  let r <- Test.submitRaw PostForm [
    { name: "slug", value: "x" },
    { name: "title", value: "x" },
    { name: "body", value: "x" },
    { name: "published", value: "True" },
    { name: "author", value: "missing" },
  ]
  let rows <- Db.from Post |> Db.count
  Task.succeed ({
    answer: r.answer,
    rows: rows,
  } == {
    answer: FormRejected [{ field: "author", message: "must be one of the options" }],
    rows: 0,
  })
}

test "a visitor is sent to log in" = {
  let r <- Test.open (Admin.Post.New {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a Reader gets no page, even one made Admin since logging in" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Reader }
  let r <- Test.open (Admin.Post.New {})
  Task.succeed (r.answer == PageMissing)
}

test "a demoted Admin is refused at once" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Reader,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let r <- Test.open (Admin.Post.New {})
  Task.succeed (r.answer == PageMissing)
}
-- @admin end
```

`Admin.Post.Edit` is the same form, filled with the row's values, its
arm an update instead of an insert, and its `@unique` check counting
the other rows only.

### The delete page

Deleting a row is where a database's rules show (chapter 8, "Writing").
A row that a `@restrict` reference points at cannot be deleted: the
database refuses, the task fails, and a request whose task fails is an
error page. A row with `@cascade` references takes the rows that refer
to it along with it, which the person deleting it should know. So the
delete page counts both, before anyone presses anything. A user is the
author of posts, and `Post.author` is `@restrict`:

```alder part=blog/Admin.User.Delete
-- generated by `alder new admin`; regions between
-- `-- @admin begin <name>` / `-- @admin end` are rewritten by `alder regen admin`
module Admin.User.Delete

-- @admin begin imports
import Admin.Layout
import Post
import User
-- @admin end

page "/admin/user/{id}/delete" { id: User.UserId } requires user, role Admin

-- The row, and how many rows of each table refer to it.
type Model = {
  row: User,
  -- @admin begin model
  posts: Int,
  -- @admin end
}

load : Params -> Session -> Query Model
load params session = {
  let admin <- Admin.Layout.isAdmin session.user
  if admin then {
    -- @admin begin load
    let found <- Db.get User params.id
    let posts <- Db.from Post |> Db.where (Post.author.eq params.id) |> Db.count
    match found {
      Just row -> Query.succeed { row: row, posts: posts },
      Nothing -> Query.notFound,
    }
    -- @admin end
  } else Query.notFound
}

-- @admin begin blockers
-- blockers counts the rows that keep this one from being deleted, which the database would
-- refuse the delete for: the `@restrict` references to it, or to a row deleted with it.
blockers : Model -> Int
blockers model = model.posts
-- @admin end

-- Delete takes no input: the form is the button, and the place a refusal is shown.
form Confirm = { }

msg Remove form Confirm

-- Remove deletes the row, unless rows refer to it that keep it (load counted them, as it does
-- before every message): the database would refuse the delete with an error, and the page says
-- why instead.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Remove p -> {
    -- @admin begin remove
    if blockers model > 0 then
      Task.succeed (Reject "This user cannot be deleted while other rows refer to it.")
    else {
      let _ <- Db.delete User model.row.id
      Task.succeed (Redirect (Admin.User.Index {}))
    }
    -- @admin end
  },
}
```

`load` counts the posts that refer to the user, and `blockers` adds up
every count that keeps the row. When it is more than zero, the `view`
shows no button and says why, and the `Remove` arm refuses too, with a
`Reject`. The arm's check matters even though the button is gone: a
`POST` can be sent without the page, and the arm is the last place that
can turn it away before the database does it with an error.

```alder part=blog/Admin.User.Delete
view : Model -> Session -> Html Msg
view model session =
  <Admin.Layout.Page title="Delete user">
    <h1>Delete this user?</h1>

    <al-card>
      -- @admin begin row
      <p>{model.row.email}</p>
      -- @admin end
      <dl>
        -- @admin begin counts
        <dt>Posts that keep it from being deleted</dt>
        <dd id="posts">{Text.fromInt model.posts}</dd>
        -- @admin end
      </dl>
      <form id="delete" submit={Remove}>
        <al-alert variant="destructive"><Errors form /></al-alert>
        {if blockers model > 0 then <p>It cannot be deleted while other rows refer to it.</p> else <al-button variant="destructive" type="submit">Delete</al-button>}
      </form>
    </al-card>
    <a href={Admin.User.Show { id: model.row.id }}>Back</a>
  </Admin.Layout.Page>
```

The second test is exactly that crafted request: the page shows no
button, the test posts the form anyway, and the answer is the
`FormRejected` with the arm's sentence, and the user is still there.

```alder part=blog/Admin.User.Delete
-- @admin begin tests
test "deleting a user nothing refers to" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let user2 <- Db.insert User {
    email: Text.Email "email-2@example.com",
    name: Text.NonEmpty "name-2",
    passwordHash: "passwordhash-2",
  }
  let user3 <- Db.insert User {
    email: Text.Email "email-3@example.com",
    name: Text.NonEmpty "name-3",
    passwordHash: "passwordhash-3",
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-1",
    body: "body-1",
    author: user3,
  }
  let _ <- Test.get (Admin.User.Delete { id: user2 })
  let r <- Test.submit Confirm { }
  let gone <- Db.get User user2
  let kept <- Db.get User user3
  Task.succeed ({
    answer: r.answer,
    gone: Maybe.map (\row -> row.id) gone,
    kept: Maybe.map (\row -> row.id) kept,
  } == {
    answer: Redirected (Admin.User.Index {}),
    gone: Nothing,
    kept: Just user3,
  })
}

test "a user that posts refer to is not deleted" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let user2 <- Db.insert User {
    email: Text.Email "email-2@example.com",
    name: Text.NonEmpty "name-2",
    passwordHash: "passwordhash-2",
  }
  let _ <- Db.insert Post {
    slug: Post.Slug "slug-1",
    title: Text.NonEmpty "title-1",
    body: "body-1",
    author: user2,
  }
  let page <- Test.get (Admin.User.Delete { id: user2 })
  let r <- Test.submit Confirm { }
  let kept <- Db.get User user2
  Task.succeed ({
    count: Test.text "#posts" page.doc,
    buttons: Test.count "#delete button" page.doc,
    answer: r.answer,
    kept: Maybe.map (\row -> row.id) kept,
  } == {
    count: Just "1",
    buttons: 0,
    answer: FormRejected [{ field: "", message: "This user cannot be deleted while other rows refer to it." }],
    kept: Just user2,
  })
}

test "a missing row is not found" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let r <- Test.open (Admin.User.Delete { id: User.UserId "missing" })
  Task.succeed (r.answer == PageMissing)
}

test "a visitor is sent to log in" = {
  let user1 <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
  }
  let r <- Test.open (Admin.User.Delete { id: user1 })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a Reader gets no page, even one made Admin since logging in" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Admin,
  }
  let _ <- Test.login { user: Just me, role: Reader }
  let user2 <- Db.insert User {
    email: Text.Email "email-2@example.com",
    name: Text.NonEmpty "name-2",
    passwordHash: "passwordhash-2",
  }
  let r <- Test.open (Admin.User.Delete { id: user2 })
  Task.succeed (r.answer == PageMissing)
}

test "a demoted Admin is refused at once" = {
  let me <- Db.insert User {
    email: Text.Email "email-1@example.com",
    name: Text.NonEmpty "name-1",
    passwordHash: "passwordhash-1",
    role: Reader,
  }
  let _ <- Test.login { user: Just me, role: Admin }
  let user2 <- Db.insert User {
    email: Text.Email "email-2@example.com",
    name: Text.NonEmpty "name-2",
    passwordHash: "passwordhash-2",
  }
  let r <- Test.open (Admin.User.Delete { id: user2 })
  Task.succeed (r.answer == PageMissing)
}
-- @admin end
```

The post's delete page counts the other way: `Comment.post` is
`@cascade`, so it lists "Comments deleted with it", and keeps its
button. A `@restrict` reference further down a cascade (a comment that a
third table's row would keep) is counted too, at every level, and
blocks the delete like a direct one.

**A tree.** A table that refers to itself with `@cascade` (a reply to a
reply, a folder in a folder) is a tree: deleting a row deletes its
children, theirs, and so on to any depth, which no query can follow, so
the page cannot count every row a `@restrict` reference might hold far
down. When such a delete can reach a `@restrict` reference (one to the
table, or to a table its cascades reach), the delete page counts only the
row's own children (the function `children`, in the region `blockers`),
and deletes only a row without them: with children, it shows "Delete its
children first." and no button, and `Remove` answers `Reject` with the
same sentence. A row without children takes the usual path, its own
`@restrict` references counted. So of a chain a → b → c whose last row a
`@restrict` reference holds, a and b are refused for their children, c
for the reference, and a childless row nothing refers to is deleted; the
generated test checks the first and the last. A tree below another table
(a post's nodes, which have nodes of their own) is counted the same way:
the post is refused while its nodes have children. Only a table can refer
to itself: a cycle through two tables would need each module to import
the other, an import cycle (E0006). Without a `@restrict` in reach the
database cascades the whole tree, and the page deletes as usual. Neither
example has a tree.

### What the admin does not do

It reads and writes every row with the unscoped operations (`Db.from`,
`Db.get`, `Db.insert`, …), because the role is the rule there: an Admin
acts on everyone's rows (chapter 6, "When to use the unscoped forms").
It writes no check of ownership: the page's `requires user, role Admin`
checks the role, in the route, before `load` runs, for the page and for
every message, and `isAdmin` reads it again in `load`.

It never shows or edits a secret. A column is a secret when its type is
`Text.Secret`, or when its name, in any case, begins with `password`,
ends in `hash`, or holds `token` or `secret` (`passwordHash`,
`apiToken`, `clientSecret`): it is left out of the list, its search and
its sorts, the row's page and both forms, and it never labels a row. An
insert that must name one writes it empty (`""`, or `Nothing` for a
`Maybe`), so a user the admin creates has no password until the app
sets one; a secret of another type that an insert must name is refused,
with a sentence asking for a default or a `Maybe`. The blog's
`User.passwordHash` is such a column. It loads every row of a
referenced table for a select, which is fine for the tables an admin
usually manages, and offers no bulk actions and no permissions finer
than one role.

## Logging in: `alder new auth`

The admin needs a user and a role, and so does any page that `requires
user`. `alder new auth` writes the login: a user table (unless the app
has one), the login page, the edits of `App` that make it the app's
login, and the Go adapter that checks passwords. On the guestbook:

```text command=guestbook
$ alder new auth
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
wrote app/User.alder
wrote migrations/0003_create_user.sql
  create table User
edited migrations/schema.json
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
```

`App` is edited in four fixed places, and nothing else in it moves: the
session gains `user` after the fields it has; `login = Auth.Login`
follows the session; `msg Logout` and its arm go into `App`'s `update`
(a new arm before its closing brace, or, as here, a new `update` after
`login`); and a test of Logout goes at the end. The user module is
imported in its sorted place. The command is refused when the session
already has a `user`, when a module `Auth` or `Auth.…` exists, and when
`App` has a `login`.

The table it created:

```alder written=guestbook/app/User.alder
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text.Secret,
  created: Time @created,
}
```

`passwordHash` is a `Text.Secret`: the admin never shows it, and a
form never echoes it (chapter 9). `--users T` names another table; when
`T` exists, it must have `email: Text.Email` and a `passwordHash` of
`Text` or `Text.Secret`, and it is used as it is. `--role R` also puts
a role in the session, copied from the user's row at login and set back
to the first constructor at logout. The role is a decision about the
app's users, so the command does not invent one: `R` must be a union of
the user module, and the command that makes one comes first.

```text
$ alder new type User "Role = Member | Admin"
$ alder new auth --role Role
```

(on an app without a user table: the table it creates then has
`role: Role = Member`). That is what `alder new admin` asks for.

The login page is the blog's (chapter 14), with one difference. Its
`Password` capability has two entries, and its adapter is real:

```alder fragment
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
  hash : Text -> Task Text,
}
```

`verify` answers the user whose email and password these are, or
`Nothing`. `hash` turns a password into the text to store in
`passwordHash`: a registration page, or a "change your password" page,
calls it. In tests, the fake stands in for both (any user with a row
logs in, whatever the password; `hash` returns its input), and the
generated tests log in through the form, reject an unknown email, and
log out from the login page, which shows a Log out button to a signed-in
user. Where to put the other Log out buttons is the app's own decision,
so the command adds none. In a page's `view`:

```alder fragment
<al-button variant="outline" click={App.Logout}>Log out</al-button>
```

A login goes to the page at `/`; `--to <Page>` names another.

### The password adapter

The adapter, `adapters/auth/login_password.go`, is plain Go, using only
the standard library. A password is never stored. What is stored is the
result of a **key derivation function**: a slow, one-way function of
the password and a random **salt**. PBKDF2 with SHA-256 (Go's
`crypto/pbkdf2`) runs HMAC-SHA-256 600,000 times; the count makes each
guess of an attacker who has stolen the table cost as much as a login,
and the salt, 16 random bytes per password, makes two users with the
same password have different hashes, so one guess cannot be tried
against every row at once. The stored text says how it was made:

```text
pbkdf2-sha256$600000$<salt>$<hash>
```

with the salt and the 32-byte hash in unpadded base64. To check a
password, the adapter reads the row by email, splits the stored text,
derives the key again with the stored salt and the **stored** count (so
hashes made before a change of the count keep working), and compares
the two keys with `crypto/subtle.ConstantTimeCompare`, which takes the
same time wherever they differ: an ordinary comparison stops at the
first different byte, and its timing would tell an attacker how much of
a guess was right. An unknown email is `Nothing` too, after a
derivation all the same, so that the answer's timing does not say which
emails have accounts. A stored text that is not of this form is an
error, never a login; an empty one is a user without a password, who
cannot log in. The core of it:

```go
// Check reports whether password is the one stored was derived from, comparing in constant
// time with the iteration count stored names. A stored text that is not
// `pbkdf2-sha256$<iterations>$<salt>$<hash>` is an error.
func Check(stored, pw string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != scheme {
		return false, errors.New("auth: the stored password hash is not pbkdf2-sha256$<iterations>$<salt>$<hash>")
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > maxIterations {
		return false, fmt.Errorf("auth: the stored password hash has a bad iteration count %q", parts[1])
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false, errors.New("auth: the stored password hash has a bad salt")
	}
	want, err := enc.DecodeString(parts[3])
	if err != nil || len(want) < 16 {
		return false, errors.New("auth: the stored password hash has a bad hash (under 16 bytes)")
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
```

A stored key shorter than 16 bytes is malformed too: a truncated hash
would make guessing cheap. `Verify` finds the row the way the blog's
development stub does (chapter 14): it reads the `User` table through
the store in the running task's context, in the task's transaction.
When a login verifies against a hash made with fewer iterations than
the adapter's count now, it stores a new hash, made with the current
count, in the same transaction: raising `iterations` upgrades every
user at their next login. Its tests, `login_password_test.go`, run with
`go test ./adapters/...` in the built app (chapter 7): a round trip, a
wrong password, an unknown email (which derives against the dummy hash
all the same), the upgrade of an old hash, another iteration count, a
changed key and ten malformed texts.

The slowness is the point, and it has a cost: each attempt takes about
60 ms of CPU, so a server that lets anyone post `/login` as fast as they
like lets them use its CPU too. Put a rate limit in front of `/login`
(per address and per email) in the proxy or load balancer the app runs
behind.

## Types, values and tests: `alder new type`, `decl`, `test`

Three commands add one declaration each to a module that exists. A type,
written as chapter 4 writes it after `type`: a union, a brand, or a
record:

```text command=guestbook
$ alder new type Entry "Mood = Calm | Glad"
edited app/Entry.alder
--- a/app/Entry.alder
+++ b/app/Entry.alder
@@ -2,6 +2,8 @@
 
 type EntryId = brand Text
 
+type Mood = Calm | Glad
+
 table Entry {
   id: EntryId @id,
   name: Text.NonEmpty,
```

It goes after the module's last type declaration (a module that does
not exist is created, as `User` was for `Role` above). A value, with its
signature and a hole for its body:

```text command=guestbook
$ alder new decl Home perPage Int
edited app/Home.alder
…
+perPage : Int
+perPage = ?perPage
…
app/Home.alder:51:11: warning[W0002]: hole ?perPage has type Int
  perPage = ?perPage
            ^^^^^^^^
```

A function is a value of a function type, and its body a lambda
(`alder new decl Member row "Team.TeamId -> User.UserId -> Query (Maybe
Member)"`, then `\team user -> …`). And a test, whose whole body is the
hole `?test : Task Bool`:

```text command=guestbook
$ alder new test Home "a page has twenty entries"
edited app/Home.alder
…
+test "a page has twenty entries" = {
+  ?test
+}
…
```

Filled with a block, the block's lines become the test's:

```text command=guestbook
$ alder fill Home.perPage - <<'EOF'
20
EOF
edited app/Home.alder
…
```

```text command=guestbook
$ alder fill 'Home."a page has twenty entries"' - <<'EOF'
{
  let n = perPage
  Task.succeed (n == 20)
}
EOF
edited app/Home.alder
…
+  let n = perPage
+  Task.succeed (n == 20)
…
```

Each refuses a name the module has.

## Blank pages and whole bodies: `--blank` and `fill --decl`

`alder new page … --blank` writes a page whose `Model` is `{ }` and
whose `load` and `view` are holes, for an author who knows what the page
shows but no template fits it (with `--show T`, the page keeps
`/{id}` and its `id` parameter, and nothing else of `--show`):

```text command=guestbook
$ alder new page Stats /stats --blank
wrote app/Stats.alder
app/Stats.alder:8:23: warning[W0002]: hole ?load has type Query Stats.Model
  load params session = ?load
                        ^^^^^
app/Stats.alder:11:22: warning[W0002]: hole ?view has type Html Stats.Msg
  view model session = ?view
                       ^^^^^
```

`Model` is a type, and a type cannot be a hole. So the one command that
replaces more than a hole is `alder fill --decl <Module>.<decl> -`: it
replaces the **whole body** of one declaration (a value's body, a test's
block, or a type's right-hand side) with what standard input holds, one
expression or one type, keeping the declaration's name and signature.
As with every fill, the copy must check:

```text command=guestbook
$ alder fill --decl Stats.Model - <<'EOF'
{ entries: Int }
EOF
edited app/Stats.alder
--- a/app/Stats.alder
+++ b/app/Stats.alder
@@ -2,7 +2,7 @@
 
 page "/stats" { }
 
-type Model = { }
+type Model = { entries: Int }
 
 load : Params -> Session -> Query Model
 load params session = ?load
…
```

That checked because `load` and `view` are holes, whose types follow
`Model`. On a page whose `load` builds the old `Model`, the same change
is refused, with the error in `load`: one declaration at a time, each
step checked, is the order **Model, then load, then view**. The holes:

```text command=guestbook
$ alder fill Stats.load - <<'EOF'
{
  let n <- Db.from Entry |> Db.count
  Query.succeed { entries: n }
}
EOF
edited app/Stats.alder
…
added import Entry to app/Stats.alder
```

```text command=guestbook
$ alder fill --decl Stats.view - <<'EOF'
<Layout.Page title="Stats">
  <h1>Stats</h1>
  <p id="entries">{Text.fromInt model.entries} entries</p>
</Layout.Page>
EOF
edited app/Stats.alder
…
+view model session =
+  <Layout.Page title="Stats">
+    <h1>Stats</h1>
+    <p id="entries">{Text.fromInt model.entries} entries</p>
+  </Layout.Page>
…
added import Layout to app/Stats.alder
```

`Entry` was imported because the expression names the table's record
type, and `Layout` because it names `Layout.Page`. Markup is put on the
line after `view model session =`, two spaces in, as views are written.

When to prefer a hole: whenever there is one. A hole's type is the
checker's statement of what is missing, and a fill cannot disturb the
code around it. `--decl` rewrites a whole body, so it can drop what a
hole fill cannot: an `update` that loses an arm, a view that loses the
form a test submits. The checker refuses the first (the view still
sends the message: a `match` must be exhaustive), and the tests catch
the second. Use it for what has no hole: a `Model`, a body a template
wrote that does not fit, a generated test whose setup does not fit the
page's rules. A test rewritten whole is checked for its types only, not
for what it asserts, and `fill --decl` says so, naming `alder test
--mutate` as the check that it still tests something.

## Buttons in a list, and page parameters

`alder new msg` puts its button at the end of the view. A button that
acts on one row of a list belongs in the list, inside the `for` that
shows the row: `--in-for <binder>` puts it at the end of the loop body's
element, and fills the message's fields from the binder where it can (a
field of the binder with the same name and type, `user: m.user`; for a
field of a table's id type, the binder's id when the binder is a row of
that table, `id: e.id`), with holes for the rest. `--when <expression>`
shows it only when the expression holds:

```text command=guestbook
$ alder new msg Home Hide id:Entry.EntryId --in-for e --when "List.length model.entries > 1"
edited app/Home.alder
…
-      {for e in model.entries -> <li><b>{e.name}</b>: {e.message}</li>}
+      {for e in model.entries -> <li><b>{e.name}</b>: {e.message}{if List.length model.entries > 1 then <al-button class="hide" click={Hide { id: e.id }}>Hide</al-button> else Html.none}</li>}
…
```

A button in a loop has its message's name as its class (`hide`), so a
test can count the rows' buttons (`Test.count "#entries .hide"
page.doc`). Its arm and test verdict are holes, as for any message, and
so is the row the test clicks (`Hide { id: ?hideId }`): which rows the
loop shows is the page's logic (a team's members, but not its owner;
the invites to the user's email that wait), which the generator cannot
know. A comment above the test names the loop's list (`model.entries`)
and says what to do: insert rows so that the signed-in user sees the
row it clicks, and fill the hole with that row's values. `fill` drops
the generator's comments once their holes are filled.

`alder edit page <Page> add-param <name>:<Type>[=default]` adds a
parameter to a page: a query parameter, such as a search box's `q`:

```text command=guestbook
$ alder edit page Stats add-param "q:Text=\"\""
edited app/Stats.alder
--- a/app/Stats.alder
+++ b/app/Stats.alder
@@ -3,7 +3,7 @@
 import Entry
 import Layout
 
-page "/stats" { }
+page "/stats" { q: Text = "" }
 
 type Model = { entries: Int }
 
```

Without a default, every `Stats {}` in the app would lack it, and the
command is refused by the check.

## An app from the command line

The commands above are enough to build an app without opening a file.
The proof is a drill: the teams app of the experiment E9 (task T10:
users own teams, invite people by email, and an invited user accepts or
declines; only the owner and the members see a team, and only the owner
changes it), built from `alder new teams` with `alder new`, `alder
edit` and `alder fill` alone. The script below is the whole of it, every
command and every expression filled. `cmd/alder/drill_test.go` replays
it from `testdata/drill/t10-teams/script.txt` (and checks that the
blocks of this section are that file): the app's own 27 tests pass;
the 8 hidden acceptance tests E9 kept from its models pass, added as the
E9 harness adds them; and `alder test --mutate` kills all 49 of its
mutants. The guestbook the app started as is still there, at `/`.

The users first. The table is created with `passwordHash: Text`
rather than letting `new auth` create it (as `Text.Secret`), because
E9's tests insert users with `passwordHash: ""`; `new auth` uses a
table that exists as it is:

```text
# the users, and logging in
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text
alder new auth --users User
```

The tables. `Invite.status` needs its union first, and the union needs
its module, which `new table` creates; so the table comes first and the
column is added after the type, with a default, as a migration of its
own:

```text
# the tables
alder new table Team name:Text.NonEmpty owner:User.UserId@ref=User
alder new table Member team:Team.TeamId@ref=Team user:User.UserId@ref=User
alder new table Invite team:Team.TeamId@ref=Team email:Text.Email
alder new type Invite "Status = Pending | Accepted | Declined"
alder edit table Invite add-field status:Status=Pending
```

One helper, `Member.row`, and its test. A function is declared with its
type and filled with a lambda; a test is filled with its whole block:

```text
# a user's Member row in a team
alder new decl Member row "Team.TeamId -> User.UserId -> Query (Maybe Member)"
alder fill Member.row - <<'EOF'
\team user -> Db.from Member |> Db.where (Member.team.eq team) |> Db.where (Member.user.eq user) |> Db.first
EOF
alder new test Member "row is the row of the user in that team"
alder fill 'Member."row is the row of the user in that team"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let _ <- Db.insert Member { team: blue, user: bob }
  let _ <- Db.insert Member { team: red, user: ann }
  let want <- Db.insert Member { team: red, user: bob }
  let found <- row red bob
  Task.succeed (Maybe.map (\m -> m.id) found == Just want)
}
EOF
```

The pages, blank, all three at once: `/teams` links to `/teams/{id}`
and `/invites`, and a view cannot name a page that does not exist yet:

```text
# the three pages, blank: their Model, load and view are filled below
alder new page Teams.Index /teams --blank --requires user
alder new page Teams.Show /teams --show Team --blank --requires user
alder new page Invites.Index /invites --blank --requires user
```

`/teams`. `Model` first, by `--decl` (a type has no hole), then `load`
by its hole, then the view by `--decl` (to put the markup on its own
lines). The form `new form` adds fits no table (a `Team` needs an
`owner` the form does not have), so its arm is a hole, filled with the
insert; its generated test cannot know the new team's id, so it is
rewritten with `--decl`:

```text
# /teams: the teams a user owns or is a member of, by name, and a form for a new one
alder fill --decl Teams.Index.Model - <<'EOF'
{ teams: List Team }
EOF
alder fill Teams.Index.load - <<'EOF'
{
  let memberships <- Db.from Member |> Db.where (Member.user.eq session.user) |> Db.all
  let teams <-
    Db.from Team
      |> Db.where (Db.or [Team.owner.eq session.user, Team.id.in (List.map (\m -> m.team) memberships)])
      |> Db.orderBy Team.name.asc
      |> Db.all
  Query.succeed { teams: teams }
}
EOF
alder fill --decl Teams.Index.view - <<'EOF'
<Layout.Page title="Teams">
  <h1>Your teams</h1>
  <ul id="teams">
    {for team in model.teams ->
      <li><a class="name" href={Teams.Show { id: team.id }}>{team.name}</a></li>}
  </ul>
  <a href={Invites.Index {}}>Invites</a>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new form Teams.Index NewTeam name:Text.NonEmpty
alder fill Teams.Index.update.SubmitNewTeam - <<'EOF'
{
  let id <- Db.insert Team { name: p.form.name, owner: session.user }
  Task.succeed (Redirect (Teams.Show { id: id }))
}
EOF
alder fill --decl 'Teams.Index."submitting NewTeam"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Index {})
  let r <- Test.submit NewTeam { name: Text.NonEmpty "Red" }
  let teams <- Db.from Team |> Db.where (Team.owner.eq ann) |> Db.all
  let ids = List.map (\t -> t.id) teams
  Task.succeed ({ answer: r.answer, teams: List.length ids } == { answer: Redirected (Teams.Show { id: Maybe.withDefault (Team.TeamId "") (List.head ids) }), teams: 1 })
}
EOF
alder new test Teams.Index "the list: teams owned and joined, by name"
alder fill 'Teams.Index."the list: teams owned and joined, by name"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cid <- Db.insert User { email: Text.Email "cid@example.com", name: Text.NonEmpty "Cid", passwordHash: "" }
  let _ <- Db.insert Team { name: Text.NonEmpty "Amber", owner: ann }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: bob }
  let green <- Db.insert Team { name: Text.NonEmpty "Green", owner: cid }
  let _ <- Db.insert Team { name: Text.NonEmpty "Zinc", owner: cid }
  let _ <- Db.insert Member { team: green, user: bob }
  let _ <- Db.insert Member { team: blue, user: ann }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Teams.Index {})
  Task.succeed (Test.texts "#teams .name" page.doc == ["Amber", "Blue"])
}
EOF
```

`/teams/{id}`. The Remove button goes into the members' loop with
`--in-for u`, its `user` filled from the binder (`u.id`) and shown only
to the owner (`--when model.isOwner`). Each arm checks `model.isOwner`
itself: a member may hold the owner's page, and post its Remove. The
generated test of Remove presses it for the signed-in owner, who has no
button, so it is rewritten as the reference app's test:

```text
# /teams/{id}: the owner and the members, for them only; the owner removes members and invites
alder fill --decl Teams.Show.Model - <<'EOF'
{ team: Team, owner: Maybe User, others: List User, isOwner: Bool }
EOF
alder fill Teams.Show.load - <<'EOF'
{
  let found <- Db.get Team params.id
  match found {
    Just team -> {
      let rows <- Db.from Member |> Db.where (Member.team.eq team.id) |> Db.all
      let ids = List.map (\m -> m.user) rows
      if team.owner == session.user || List.member session.user ids then {
        let owner <- Db.get User team.owner
        let others <- Db.from User |> Db.where (User.id.in ids) |> Db.orderBy User.name.asc |> Db.all
        Query.succeed { team: team, owner: owner, others: others, isOwner: team.owner == session.user }
      } else Query.notFound
    },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill --decl Teams.Show.view - <<'EOF'
<Layout.Page title={model.team.name}>
  <h1 id="name">{model.team.name}</h1>
  <ul id="members">
    {match model.owner {
      Just o -> <li><span class="name">{o.name}</span> (owner)</li>,
      Nothing -> Html.none,
    }}
    {for u in model.others ->
      <li>
        <span class="name">{u.name}</span>
      </li>}
  </ul>
  <a href={Teams.Index {}}>All teams</a>
</Layout.Page>
EOF
alder new msg Teams.Show Remove user:User.UserId --in-for u --when model.isOwner
alder new form Teams.Show NewInvite email:Text.Email
alder fill Teams.Show.update.Remove - <<'EOF'
if not model.isOwner then Task.notFound
else if p.user == model.team.owner then Task.fail "The owner cannot leave the team."
else {
  let found <- Member.row model.team.id p.user
  match found {
    Just m -> {
      let _ <- Db.delete Member m.id
      Task.succeed (Redirect (Teams.Show { id: model.team.id }))
    },
    Nothing -> Task.notFound,
  }
}
EOF
alder fill Teams.Show.update.SubmitNewInvite - <<'EOF'
if not model.isOwner then Task.notFound
else {
  let _ <- Db.insert Invite { team: model.team.id, email: p.form.email }
  Task.succeed (Redirect (Teams.Show { id: model.team.id }))
}
EOF
alder fill 'Teams.Show."submitting NewInvite"' - <<'EOF'
Redirected (Teams.Show { id: team1 })
EOF
alder fill --decl 'Teams.Show."pressing Remove"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cid <- Db.insert User { email: Text.Email "cid@example.com", name: Text.NonEmpty "Cid", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: red, user: cid }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Teams.Show { id: red })
  let r <- Test.click (Remove { user: bob })
  let invited <- Test.submit NewInvite { email: Text.Email "new@example.com" }
  let invites <- Db.from Invite |> Db.all
  Task.succeed ({ removed: r.answer, names: Test.texts "#members .name" r.doc, invited: invited.answer, invites: List.map (\i -> { team: i.team, email: i.email, status: i.status }) invites } == { removed: Redirected (Teams.Show { id: red }), names: ["Ann", "Cid"], invited: Redirected (Teams.Show { id: red }), invites: [{ team: red, email: Text.Email "new@example.com", status: Pending }] })
}
EOF
alder new test Teams.Show "the page: owner and members only, the owner first, the others by name"
alder fill 'Teams.Show."the page: owner and members only, the owner first, the others by name"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Zoe", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cid <- Db.insert User { email: Text.Email "cid@example.com", name: Text.NonEmpty "Cid", passwordHash: "" }
  let dan <- Db.insert User { email: Text.Email "dan@example.com", name: Text.NonEmpty "Dan", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let _ <- Db.insert Member { team: red, user: cid }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Db.insert Member { team: blue, user: dan }
  let _ <- Test.login { user: Just ann }
  let owner <- Test.get (Teams.Show { id: red })
  let _ <- Test.login { user: Just cid }
  let member <- Test.get (Teams.Show { id: red })
  let _ <- Test.login { user: Just dan }
  let stranger <- Test.open (Teams.Show { id: red })
  Task.succeed ({ names: Test.texts "#members .name" owner.doc, buttons: Test.count ".remove" owner.doc, memberButtons: Test.count ".remove" member.doc, stranger: stranger.answer } == { names: ["Zoe", "Bob", "Cid"], buttons: 2, memberButtons: 0, stranger: PageMissing })
}
EOF
alder new test Teams.Show "only the owner changes the team, and the owner cannot leave"
alder fill 'Teams.Show."only the owner changes the team, and the owner cannot leave"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let cid <- Db.insert User { email: Text.Email "cid@example.com", name: Text.NonEmpty "Cid", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let _ <- Db.insert Member { team: red, user: bob }
  let _ <- Db.insert Member { team: red, user: cid }
  let asBob <- load { id: red } { user: bob }
  let byMember <- Test.run (update (Remove { user: cid }) asBob { user: bob })
  let inviteByMember <- Test.run (update (SubmitNewInvite { form: { email: Text.Email "x@example.com" } }) asBob { user: bob })
  let asAnn <- load { id: red } { user: ann }
  let self <- Test.run (update (Remove { user: ann }) asAnn { user: ann })
  let members <- Db.from Member |> Db.count
  let invites <- Db.from Invite |> Db.count
  Task.succeed ({ byMember: byMember.value, inviteByMember: inviteByMember.value, self: self.value, members: members, invites: invites } == { byMember: Err "not found", inviteByMember: Err "not found", self: Err "The owner cannot leave the team.", members: 2, invites: 0 })
}
EOF
```

`/invites`. `Row` is a record type, so `new type` declares it. The
binder `r` of the invites' loop is a `Row`, which has no `id`, so
`--in-for r` leaves the hole `?acceptId`, filled with `r.invite.id`
(the second fill names no hole: one is left). In the generated test,
which row to click is a hole too (`?acceptId` again, in the test), since
which rows the loop shows is the page's logic: the test inserted an
invite to the signed-in user's email, `invite1`:

```text
# /invites: the invites to the user's email that wait for an answer, to accept or decline
alder new type Invites.Index "Row = { invite: Invite, team: Text }"
alder fill --decl Invites.Index.Model - <<'EOF'
{ rows: List Row }
EOF
alder new decl Invites.Index waiting "User.UserId -> Query (List Invite)"
alder fill Invites.Index.waiting - <<'EOF'
\user -> {
  let me <- Db.get User user
  match me {
    Just u -> Db.from Invite |> Db.where (Invite.email.eq u.email) |> Db.where (Invite.status.eq Pending) |> Db.all,
    Nothing -> Query.succeed [],
  }
}
EOF
alder new decl Invites.Index row "Invite -> Query Row"
alder fill Invites.Index.row - <<'EOF'
\invite -> {
  let team <- Db.get Team invite.team
  Query.succeed { invite: invite, team: Maybe.withDefault "" (Maybe.map (\t -> t.name) team) }
}
EOF
alder fill Invites.Index.load - <<'EOF'
{
  let invites <- waiting session.user
  let rows <- Query.all (List.map row invites)
  Query.succeed { rows: rows }
}
EOF
alder fill --decl Invites.Index.view - <<'EOF'
<Layout.Page title="Invites">
  <h1>Invites</h1>
  <ul id="invites">
    {for r in model.rows ->
      <li>
        <span class="team">{r.team}</span>
      </li>}
  </ul>
  <a href={Teams.Index {}}>Teams</a>
</Layout.Page>
EOF
alder new msg Invites.Index Accept id:Invite.InviteId --in-for r
alder new msg Invites.Index Decline id:Invite.InviteId --in-for r
alder fill Invites.Index.view - --hole '?acceptId' <<'EOF'
r.invite.id
EOF
alder fill Invites.Index.view - <<'EOF'
r.invite.id
EOF
alder new decl Invites.Index mine "Invite.InviteId -> User.UserId -> Task Invite"
alder fill Invites.Index.mine - <<'EOF'
\id user -> {
  let invites <- waiting user
  match List.find (\i -> i.id == id) invites {
    Just invite -> Task.succeed invite,
    Nothing -> Task.notFound,
  }
}
EOF
alder fill Invites.Index.update.Accept - <<'EOF'
{
  let invite <- mine p.id session.user
  let _ <- Db.update Invite invite.id (\i -> { i | status: Accepted })
  let _ <- Db.insert Member { team: invite.team, user: session.user }
  Task.succeed (Redirect (Teams.Show { id: invite.team }))
}
EOF
alder fill Invites.Index.update.Decline - <<'EOF'
{
  let invite <- mine p.id session.user
  let _ <- Db.update Invite invite.id (\i -> { i | status: Declined })
  Task.succeed (Redirect (Invites.Index {}))
}
EOF
alder fill 'Invites.Index."pressing Accept"' - --hole '?acceptId' <<'EOF'
invite1
EOF
alder fill 'Invites.Index."pressing Accept"' - <<'EOF'
{ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Teams.Show { id: team1 }), status: Just Accepted }
EOF
alder fill 'Invites.Index."pressing Decline"' - --hole '?declineId' <<'EOF'
invite1
EOF
alder fill 'Invites.Index."pressing Decline"' - <<'EOF'
{ answer: r.answer, status: Maybe.map (\i -> i.status) after } == { answer: Redirected (Invites.Index {}), status: Just Declined }
EOF
alder new test Invites.Index "only the waiting invites to the user are listed"
alder fill 'Invites.Index."only the waiting invites to the user are listed"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let green <- Db.insert Team { name: Text.NonEmpty "Green", owner: ann }
  let _ <- Db.insert Invite { team: red, email: Text.Email "bob@example.com" }
  let _ <- Db.insert Invite { team: blue, email: Text.Email "bob@example.com", status: Declined }
  let _ <- Db.insert Invite { team: green, email: Text.Email "cid@example.com" }
  let _ <- Test.login { user: Just bob }
  let page <- Test.get (Invites.Index {})
  Task.succeed (Test.texts "#invites .team" page.doc == ["Red"])
}
EOF
alder new test Invites.Index "accepting makes a member, declining does not"
alder fill 'Invites.Index."accepting makes a member, declining does not"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let blue <- Db.insert Team { name: Text.NonEmpty "Blue", owner: ann }
  let toBlue <- Db.insert Invite { team: blue, email: Text.Email "bob@example.com" }
  let toRed <- Db.insert Invite { team: red, email: Text.Email "bob@example.com" }
  let _ <- Test.login { user: Just bob }
  let _ <- Test.get (Invites.Index {})
  let accepted <- Test.click (Accept { id: toRed })
  let _ <- Test.get (Invites.Index {})
  let declined <- Test.click (Decline { id: toBlue })
  let invites <- Db.from Invite |> Db.orderBy Invite.status.asc |> Db.all
  let members <- Db.from Member |> Db.all
  Task.succeed ({ accepted: accepted.answer, declined: declined.answer, statuses: List.map (\i -> i.status) invites, members: List.map (\m -> { team: m.team, user: m.user }) members } == { accepted: Redirected (Teams.Show { id: red }), declined: Redirected (Invites.Index {}), statuses: [Accepted, Declined], members: [{ team: red, user: bob }] })
}
EOF
alder new test Invites.Index "an invite not to the user, or not waiting, cannot be answered"
alder fill 'Invites.Index."an invite not to the user, or not waiting, cannot be answered"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let red <- Db.insert Team { name: Text.NonEmpty "Red", owner: ann }
  let theirs <- Db.insert Invite { team: red, email: Text.Email "cid@example.com" }
  let done <- Db.insert Invite { team: red, email: Text.Email "bob@example.com", status: Declined }
  let model <- load {} { user: bob }
  let a <- Test.run (update (Accept { id: theirs }) model { user: bob })
  let b <- Test.run (update (Accept { id: done }) model { user: bob })
  let members <- Db.from Member |> Db.count
  Task.succeed ({ a: a.value, b: b.value, members: members } == { a: Err "not found", b: Err "not found", members: 0 })
}
EOF
```

And the app's own tests:

```text
# the app's own tests
alder check
alder test
```

The drill needed three things beyond M12's commands, each added to them
as a finding: `new auth` (no app with users could be built), `--blank`
with `fill --decl` (no `Model` could change), and `new type`, `new
decl`, `new test` with `--in-for` (a helper, a test, a row's button had
no command). It needed `--when` too: the hidden tests count the Remove
buttons a member sees, which must be none.

## Changing a table: `alder edit table`

`alder edit table T` makes one change to a table and writes its
migration: `add-field <field>` (written like `new table`'s fields),
`rename-field <old> <new>` (which writes `@renamed "old"`, so the
migration renames the column instead of dropping it and adding another:
chapter 8) or `drop-field <field>`.

A change to a table breaks the code that uses it. A field added without
a default must be named by every insert, and a field dropped must not
be used anywhere. So `alder edit table` writes the change anyway, and
prints what is broken, as a list of errors with their files and lines:
the to-do list for whoever made the change. A note on each reply:

```text command=guestbook
$ alder edit table Reply add-field "note:Maybe Text"
edited app/Reply.alder
wrote migrations/0004_add_note.sql
  add column Reply.note
edited migrations/schema.json
--- a/app/Reply.alder
+++ b/app/Reply.alder
@@ -11,6 +11,7 @@
   hidden: Bool = False,
   created: Time @created,
   updated: Time @updated,
+  note: Maybe Text,
 }
 
 test "deleting its entry deletes a Reply" = {
app/Replies.alder:42:30: error[E0003]: record `Reply.Insert Reply` is missing field(s): note
      let _ <- Db.insert Reply { entry: p.form.entry, message: p.form.message }
                               ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Replies.alder:89:33: error[E0003]: record `Reply.Insert Reply` is missing field(s): note
    let reply1 <- Db.insert Reply { entry: entry1, message: Text.NonEmpty "message-1" }
                                  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Replies.alder:90:33: error[E0003]: record `Reply.Insert Reply` is missing field(s): note
    let reply2 <- Db.insert Reply { entry: entry1, message: Text.NonEmpty "message-2" }
                                  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Replies.alder:97:54: error[E0003]: record `Reply.Insert Reply` is missing field(s): note
    let _ <- Task.all (List.map (\n -> Db.insert Reply { entry: entry1, message: Text.NonEmpty ("message-" ++ Text.fromInt n) }) (List.range 1 21))
                                                       ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Replies/Show.alder:57:33: error[E0003]: record `Reply.Insert Reply` is missing field(s): note
    let reply1 <- Db.insert Reply { entry: entry1, message: Text.NonEmpty "message-1" }
                                  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Replies/Show.alder:69:33: error[E0003]: record `Reply.Insert Reply` is missing field(s): note
    let reply1 <- Db.insert Reply { entry: entry1, message: Text.NonEmpty "message-1" }
                                  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Reply.alder:19:28: error[E0003]: record `Reply.Insert Reply` is missing field(s): note
    let _ <- Db.insert Reply { entry: target, message: Text.NonEmpty "message-1" }
                             ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
```

The table and its migration are written, and the app now has seven
errors, one per insert of a `Reply`: the form's arm, and six inserts in
five tests. Each needs `note: Nothing`, or a value
(`note:Maybe Text=Nothing` would have needed no change at all). `alder
edit table` is the one command that runs on an app with errors, so that
a second change can follow the first before the list is done; every
other command waits until `alder check` is clean. With `--strict`, it
refuses instead, and writes nothing, when the change breaks anything.

A change no migration can make is refused with `alder migrate`'s
reason: a new field that is neither a `Maybe` nor given a default
cannot be filled in the rows already stored.

## Owned regions and `alder regen admin`

The ordinary generators write a file once, and then it is yours. The
admin cannot work that way. It follows the tables: add a column, and
every list, page and form of that table, and their tests, should show
it. If regenerating meant overwriting, every change you made to the
admin would be lost the next time a table changed. If it meant never
touching the file again, the admin would fall behind the tables.

**Owned regions** are the middle way. A region is the lines between a
begin marker and an end marker, two comments:

```alder fragment
-- @admin begin cells
<td>…</td>
-- @admin end
```

`alder regen admin` generates every file of the admin again, in memory,
from the app as it is now, and then, in each file on disk, replaces the
lines inside each region with the same region's lines from the new
version. Everything outside the regions is left byte for byte: a heading
you reworded, a column you added after the `cells` region, a test you
wrote after the `tests` region. The regions are named (`imports`,
`nav`, `cards`, `search`, `sorts`, `labels`, `headers`, `cells`,
`fields`, `model`, `load`, `form`, `inputs`, `insert`, `update`,
`blockers`, `remove`, `row`, `counts`, `tests`), so a region is matched
by its name, not by its place.

Here the blog's comments gain a flag:

```text command=blog
$ alder edit table Comment add-field approved:Bool=True
edited app/Comment.alder
wrote migrations/0002_add_approved.sql
  rebuild table Comment: Comment.approved: new column filled with 1
edited migrations/schema.json
--- a/app/Comment.alder
+++ b/app/Comment.alder
@@ -17,6 +17,7 @@
   author: Name,
   body: Body,
   created: Time @created,
+  approved: Bool = True,
 }
 
 -- fits is whether s has text other than spaces and at most n characters.
```

The field has a default, so no insert breaks, and the app checks. The
admin catches up:

```text command=blog
$ alder regen admin
edited app/Admin/Comment/Edit.alder
edited app/Admin/Comment/Index.alder
edited app/Admin/Comment/New.alder
edited app/Admin/Comment/Show.alder
…
--- a/app/Admin/Comment/Index.alder
+++ b/app/Admin/Comment/Index.alder
@@ -39,6 +39,7 @@
   { name: "author", asc: Comment.author.asc, desc: Comment.author.desc },
   { name: "body", asc: Comment.body.asc, desc: Comment.body.desc },
   { name: "created", asc: Comment.created.asc, desc: Comment.created.desc },
+  { name: "approved", asc: Comment.approved.asc, desc: Comment.approved.desc },
 ]
 -- @admin end
 
@@ -150,6 +151,13 @@
                 dir: if model.sort == "created" && model.dir == "asc" then "desc" else "asc",
               }}>Created</a>
             </th>
+            <th>
+              <a href={Admin.Comment.Index {
+                q: model.q,
+                sort: "approved",
+                dir: if model.sort == "approved" && model.dir == "asc" then "desc" else "asc",
+              }}>Approved</a>
+            </th>
             -- @admin end
           </tr>
         </thead>
@@ -161,6 +169,7 @@
               <td><a href={Admin.Comment.Show { id: row.id }}>{row.author}</a></td>
               <td>{row.body}</td>
               <td>{Time.format "Jan 2, 2006" row.created}</td>
+              <td>{if row.approved then <al-badge>Yes</al-badge> else <al-badge variant="secondary">No</al-badge>}</td>
               -- @admin end
             </tr>}
         </tbody>
…
--- a/app/Admin/Comment/Show.alder
+++ b/app/Admin/Comment/Show.alder
@@ -65,6 +65,8 @@
         <dd>{model.row.body}</dd>
         <dt>Created</dt>
         <dd>{Time.format "Jan 2, 2006" model.row.created}</dd>
+        <dt>Approved</dt>
+        <dd>{if model.row.approved then <al-badge>Yes</al-badge> else <al-badge variant="secondary">No</al-badge>}</dd>
         -- @admin end
       </dl>
       <div slot="actions">
```

Only lines inside regions changed: the sort and the column of the list,
the field of the row's page, the form's field, input and insert, and the
tests, which now set and compare `approved` too. (The text above leaves
out the diffs of the two forms, where `…` stands; `Edit.alder` changes as
`New.alder` does.) A second run has nothing to do:

```text command=blog
$ alder regen admin
nothing to do: the admin is up to date
```

The rules of `regen`:

- **It is idempotent.** Run twice, the second run changes nothing and
  writes nothing.
- **It does not bring back what you deleted.** A page you removed (say,
  a table's delete page) is listed as `not recreated app/Admin/Comment/Delete.alder
  (deleted; --all writes it again)`; `--all` writes it again. Remove the
  links to it as well (the Delete button of the table's `Show` page),
  or the app no longer checks and `regen` refuses.
- **It adds what is new.** A table added since the admin was generated
  (one the nav's region does not link to yet) gets its five pages, and
  its card and its link appear in the dashboard's and the nav's regions.
- **It refuses a broken marker.** A begin marker without its end, an end
  without its begin, a region that begins twice: the command names the
  file and the region (`` `app/Admin/Post/Index.alder`: region `cells`
  has no end marker ``) and writes nothing.
- **It checks what it writes.** A field renamed or dropped breaks the
  admin's own files, inside the regions it is about to rewrite, so
  errors in the admin's files do not stop it. Errors elsewhere in the
  app do: fix the to-do list of `alder edit table` first. And the result
  must check, as for every generator.

With `--json`, `regen` adds `"skipped"` (the pages not written again)
and `"upToDate"` (whether there was nothing to do).

Why regions only for the admin? Because only the admin is regenerated.
A page from `alder new page` is a starting point: from its first edit,
it says what you want, and no command could know which of its lines
still follow the table. The admin is the opposite case: a projection of
the tables, which you mostly leave as it is, and touch here and there.
Regions make the boundary between the generator's lines and yours
visible in the file itself, and `regen` respects it.

## AI only in typed, checked bodies

Look again at what the commands leave to their user. Every declaration
is a command's: a module, a table and its migration, a type, a page and
its path and rules, a form, a message, a value's name and signature, a
test's name. So is every structure that ties them together: the
`update`'s arms, the forms in the view, the session, the login, the
imports. What is left open is bodies. A hole is the smallest:
`?hide : Task Nav`, `?expected : Bool`, a type the checker computed.
`--blank` and `fill --decl` hand over larger ones, a whole `load`, a
whole view, a whole test. Each is still **one expression** (or one
type), read from standard input, checked with the whole app, and written
only if it checks.

The drill above shows the proportions. Of the 733 lines of the teams
app's `app/` (643 without the guestbook it started as), 246 came from
standard input, 128 of them in tests; the largest body is 18 lines (a
test's block), and no body holds a declaration: none can, since a body
is an expression. The other lines, every declaration and every
connection between them, the commands wrote.

That is the design for an agent. A language model is good at writing an
expression that does one stated thing, and bad at keeping a large
structure consistent over many edits. So the structure comes from the
generators, deterministic and tested, and the model writes bodies, each
with a type that says what is wanted. The consequence: a **structural**
error (a page without its route, an arm without its message, a form the
view does not post, a table without its migration, an import missing)
cannot be made, because no command writes one and the agent writes
nothing else; a **logic** error (a wrong condition, a wrong row) is
the checker's, the tests' and `alder test --mutate`'s to catch. A wrong
body costs one refusal with the checker's message, not a broken app.
The experiment E11, "CLI-only development", measures this: an agent
builds apps with these commands alone, reading their `--json` output,
and never edits a file directly.

The same commands are what another tool would wrap. Olifant, a generator
engine, can drive Alder through a thin pack of its own: each Olifant
command calls the matching `alder new`, `alder edit table`, `alder fill`
or `alder regen admin`, and reads its `--json`. The pack needs no
knowledge of Alder's syntax, because the compiler's generators already
have it; that is the point of putting them in the compiler.
