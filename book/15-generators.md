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
msg` and `alder new capability` each write one piece. `alder edit table`
changes a table and writes its migration. `alder fill` fills a gap the
others leave. `alder new admin` writes all of the pieces for every table
at once: a complete admin, with a list, a page, a form and a delete page
per table, and `alder regen admin` keeps that admin up to date as the
tables change.

There are two readers in mind. One is you, starting an app: a generated
page is a correct, tested starting point that you then edit. The other
is an agent (an AI model) building an app from the command line. For an
agent the commands are the interface: it asks for a page or a table by
name and type, and gets back code that already checks and already has
tests. The only free text it writes is one typed expression at a time,
through `alder fill`. The last section, "AI only in small, typed,
checked gaps", explains why.

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

This is called **copy, check, write**. It means that a generator can
never leave your app in a state that does not check. A generator also
never overwrites anything: when the file or the name it would write
exists, it refuses with one sentence and writes nothing.

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

The test's verdict:

```text command=guestbook
$ alder fill 'Replies.Show."pressing Delete"' - <<'EOF'
{ answer: r.answer, after: after.answer } == { answer: Redirected (Replies {}), after: PageMissing }
EOF
edited app/Replies/Show.alder
--- a/app/Replies/Show.alder
+++ b/app/Replies/Show.alder
@@ -75,5 +75,5 @@
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
about your app's users, not a detail to generate. To meet it:

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
generated admin. In the blog:

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
admin has them. The lines between `-- @admin begin <name>` and `-- @admin
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
then come the rules: a visitor is sent to log in, a Reader gets
`PageMissing`, and an Admin is let in by the role the cookie got at login
(chapter 6, "A role is read at login").

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

**The generated admin's one limit** is a table that refers to itself
with `@cascade` (a reply to a reply). The delete page follows such a
reference one level down, not to any depth, since a query cannot
recurse. When, below that level, a row is held by a `@restrict`
reference, the page does not count it, and the delete fails in the
database as any refused delete does. Neither example has such a table.

### What the admin does not do

It reads and writes every row with the unscoped operations (`Db.from`,
`Db.get`, `Db.insert`, …), because the role is the rule there: an Admin
acts on everyone's rows (chapter 6, "When to use the unscoped forms").
It writes no check of its own, of the role or of ownership: the page's
`requires user, role Admin` does that, in the route, before `load`
runs, for the page and for every message. It loads every row of a
referenced table for a select, which is fine for the tables an admin
usually manages, and offers no bulk actions and no permissions finer
than one role.

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
wrote migrations/0003_add_note.sql
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
app/Replies/Show.alder:74:33: error[E0003]: record `Reply.Insert Reply` is missing field(s): note
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
  (deleted; --all writes it again)`; `--all` writes it again.
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

## AI only in small, typed, checked gaps

Look again at what the generators leave to their user. A page, a
table, a form, a message, the admin: each is written whole, from a name
and a few types, by code that does the same thing every time. The only
places left open are holes, each with a type the checker computed:
`?hide : Task Nav`, `?expected : Bool`. And the only way to fill one
from the command line is `alder fill`, which takes one expression,
checks the whole app with it, and writes nothing if it does not check.

That is the design for an agent. A language model is good at writing a
small expression that does one stated thing, and bad at keeping a large
structure consistent over many edits. So the structure (the module, the
types, the routes, the forms, the rules, the tests) comes from the
generators, deterministic and tested, and the model fills the gaps,
each small, each with a type that says what is wanted, each checked
before it is kept. A wrong fill costs one refusal with the checker's
message, not a broken app. The experiment E11, "CLI-only development",
measures this: an agent builds apps with these commands and `alder
fill` alone, reading their `--json` output, and never edits a file
directly.

The same commands are what another tool would wrap. Olifant, a generator
engine, can drive Alder through a thin pack of its own: each Olifant
command calls the matching `alder new`, `alder edit table`, `alder fill`
or `alder regen admin`, and reads its `--json`. The pack needs no
knowledge of Alder's syntax, because the compiler's generators already
have it; that is the point of putting them in the compiler.
