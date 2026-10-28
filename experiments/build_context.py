#!/usr/bin/env python3
"""Build the context prompt given to models: reference chapters + example app.

Usage: build_context.py [--chapters 03,04,...] [--no-example] [--reference] [--book DIR] [--example DIR]
                        [--size [--count ollama:MODEL [--num-ctx N]]] > prompt.md

--reference drops the sections written for humans (HUMAN_SECTIONS, by exact `## ` heading),
except the `### ` subsections of a dropped section listed in KEEP_SUBSECTIONS, which stay under
the section's heading; and from a section it keeps, the `### ` subsections listed in
DROP_SUBSECTIONS (the inverse); it shortens the spans TRIMS lists, and renders an `error=` block of
a whole module as its error text alone. A listed heading or span missing from its chapter exits 2
naming it, so a renamed section never silently builds the wrong prompt. --size prints the prompt's size instead of the prompt: characters and
len/4; with --count ollama:MODEL also Ollama's prompt_eval_count for the prompt alone (one
/api/chat call, the prompt as the system message, num_predict 1).
"""
import argparse
import json
import pathlib
import re
import sys
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
BOOK = ROOT / "book"
EXAMPLE = ROOT / "examples" / "todos" / "app"

DEFAULT_CHAPTERS = ["03", "04", "05", "06", "07", "08", "09", "10"]

# The `## ` sections --reference drops: how the compiler and runtime work inside, schema
# migration, running the test tool — written for a human reader, not needed to write an app.
# Kept, like chapter 8's "Migrations": chapter 10's "8. Faking a capability" (an agent writes
# fakes and installs them; its rules and diagnostics are in that section) and "9. Testing through
# the browser" (the step table, the rules and the failure texts an agent's tests meet); its
# "10. How the browser works" is the inside, for a human reader.
HUMAN_SECTIONS = {
    "06": ["How a request runs"],
    "07": ["How effects run", "Implementing a capability"],
    "08": ["How tables are stored"],
    "10": ["2. Running the tests", "6. What a trace says",
           "7. Choosing tests, and output for programs", "10. How the browser works",
           "13. What cannot be tested yet"],
}

# The `### ` subsections of a dropped section that --reference keeps, by chapter and `## `
# heading: chapter 7's shape of a capability in Go (the interface and its methods: ctx first, an
# Args__ record, a1…an or nothing, result and error; the Go types; `import "app"`, rt, db.TxFrom)
# and its adapter naming rule (capability X of module A.B -> adapters/a/, func BX; a keyword
# directory), which an agent writing a capability needs; the rest of those sections is how the
# effects run and how the build wires the adapters.
KEEP_SUBSECTIONS = {
    "07": {"How effects run": ["What a capability looks like in Go"], "Implementing a capability": ["The rule"]},
}

# The `### ` subsections of a kept section that --reference drops, by chapter and `## `
# heading: the inverse of KEEP_SUBSECTIONS. Chapter 10's "9. Testing through the browser" keeps
# the step table and the result fields, one complete example (adding a todo), `Answer`, the
# session and the login through its form, how a record is sent (with the encoding table),
# `submitRaw` and `FormRejected`, the toggle and the stranger's examples, the queries and selectors,
# the list of every failure text and the rules; it drops the walkthrough of the first example
# and its failing report, the prose of `Test.get`/`Test.open`, the further form examples (the
# due date, two messages posting one form), the bad-input example, the refused click, the
# refused selector, and the comparison with a view (with a Doc's failure report and "when to use
# which"), and the case for testing through the browser at all. Chapter 9's step and query signatures repeat chapter 10's.
# Chapter 10's "11. Mutation testing" keeps the command and the score and survivor lines of the
# to-do app's run before its new tests, the operators, the rule with its two worked tests (and the
# score line after them), the three sentences on survivors that are not gaps and `--min` in one
# sentence (at most 6.6k characters in all: test_build_context checks); it drops why a mutation
# score (the case against line coverage), the reports of the rules' mutants (M11), all fourteen
# survivors with the countOpen story and the
# run after, what is never a mutant (the suppression rules: an agent sees only their counts), the
# logout redirect and the equivalent mutants in detail, the exact score with its transcript,
# `-run`/`-timeout`/`--json` (as section 7 is dropped) and how ids and lines change, all for a
# human reader. Chapter 6's "Rules" (M11) stays whole for an agent (`@owner`, the five scoped
# operations, the not-found answer, where they may be used, whose session, roles, the unscoped
# forms) but for its last subsection, the case for the design on a replicated database.
# Chapter 8's "Migrations" drops "When the app starts" (M11 final review, the reference cut):
# how the runtime applies the files and what it refuses, the inside, like "How tables are
# stored"; the workflow an agent follows (E0019, `alder migrate`, the files) stays. It also drops
# "Statements you add" (M12 final review): hand-written migration SQL, which an agent never
# writes. Chapter 6's "Example: a theme switch" (a worked App message) goes too: the rules it
# shows are stated in the section around it. Chapter 10's "12. Properties" keeps the forms, the
# input rules, the database properties, the report, the health checks (the reasons a run reports
# failing) and the commands an agent writing a property needs; it drops the saved-case walkthrough,
# the shrinker's internals and the `--mutate` interplay, reading for a person.
DROP_SUBSECTIONS = {
    "06": {"Rules": ["The rules and a replicated database"],
           "App-level messages": ["Example: a theme switch"],
           "File pages": ["Status and headers", "Links and the boost", "What stays HTML"]},
    "07": {"Capabilities": ["A capability returning bytes"],
           "How effects run": ["Waiting: `Query.delay` and `Task.sleep`"]},
    "08": {"Migrations": ["When the app starts", "Statements you add"]},
    "09": {"Test": ["Browser steps and queries"]},
    "10": {"11. Mutation testing": ["Why a mutation score", "The rules' mutants in a report", "How the two tests kill them", "All fourteen, and one killed by luck", "What is never a mutant",
                                    "The logout redirect", "Mutants that are the same program", "The exact score",
                                    "`-run`, `-timeout` and `--json`", "Ids and lines change with the code"],
           "9. Testing through the browser": ["Why through the browser", "Step by step", "`Test.get` and `Test.open`: requesting a page",
                                              "More forms", "Bad input, an example", "Clicks that fail", "A selector the compiler refuses",
                                              "Comparing a page with its view", "`Test.fetch`: requesting a file"],
           "12. Properties": ["Keeping the case", "How the shrinking works, and why you do not write it",
                              "Properties and `--mutate`"]},
}

# Spans of text --reference shortens in a chapter (before dropping sections), by chapter: (start, end,
# replacement), the text from start up to (not including) end replaced by replacement; each start
# must occur once in the chapter and end after it, or the tool exits 2 naming it. They shorten
# what an agent needs in fewer words (final review of M11, the reference cut): chapter 6 points
# at the example app instead of quoting its load and update; chapter 8's "Your own rows" points at
# chapter 6 (chapter 9 lists the signatures); chapter 10's "Tests that use the rules" keeps the
# facts about whose session a test runs with and the failure line, without its transcript and
# its worked test of `Db.mine`, and says where an effect test runs in one paragraph; "A
# stranger's todo" keeps one test, the stale page's; chapter 6's re-read idiom keeps its helper
# and says how `load` uses it. Chapter 5's "Custom elements" (M7a) keeps its rules in one
# paragraph, and the input elements in another, without the manifest's story and the examples;
# its click-button paragraph keeps the fact, not the markup. Chapter 6's paragraph on generated
# pages goes (COMMANDS lists the generators).
TRIMS = {
    "05": [("A `<button click={…}>` works with and without JavaScript.", "`data-*` and `aria-*` attributes take `Text`.",
            "A `<button click={…}>` works with and without JavaScript (chapter 6); a\n`click` on any other element needs JavaScript.\n\n"),
           ("## Custom elements\n", "## Partial updates",
            "## Custom elements\n\nA tag with a hyphen is a custom element, declared by the built-in\n"
            "`alder-ui` manifest or the app's `elements/custom-elements.json`\n"
            "(which wins over it). E0028: an undeclared tag, attribute or `slot=\"x\"`; a quoted\n"
            "value on a boolean attribute; a literal outside an attribute's quoted values (computed\n"
            "ones are unchecked); `submit` on any custom element (it belongs on `<form>`); `slot` on\n"
            "a child of a plain element (a component's children are not slot-checked). Always accepted:\n"
            "`id`, `class`, `style`, `slot`, `hidden`,\n`title`, `lang`, `dir`, "
            "`tabindex`, `role`, `data-*`, `aria-*`, `click`, `href`.\n\n"
            "In a `<form submit>`, `<al-input field=\"f\" />` is `<Input>` (E0304 rules),\n`al-textarea` `<Textarea>`, "
            "`al-select` `<Select>`, `al-checkbox` `<Input>`\non a `Bool`; each renders its `help` and its own error list\n"
            "(an `<Errors field>` for it is E0028).\n\n")],
    "06": [("and, in `Write.Edit`,\n\n```alder fragment\nload params session = {\n  let author <- User.isAuthor", "Every message of the page runs `load` first",
            "which each Write page's `load` calls first, answering `Query.notFound`\nwhen it is `False`. "),
           ("A row `load` already found with `Db.getMine` is not one of the cases:", "### The rules and a replicated database",
            "A row `load` already found with `Db.getMine` is not one of the cases:\nwrite the scoped form; it costs "
            "nothing (`Todos.Show` writes\n`Db.updateMine Todo model.todo.id …`, and the mutation tool counts it\nas "
            "the same program as the unscoped one there).\n\n"),
           ("**Generated pages say the same.**", "**A role is read at login.**",
            ""),
           ("### Partitions\n\n`@owner` says", "## The request lifecycle",
            "### Partitions\n\n`@partition k` generalises `@owner` (which is `@partition user`) to any session\n"
            "key, here `org` (`examples/workspace`):\n\n```alder fragment\n"
            "session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership, role: User.Role = Member }\n"
            "choose org = Orgs.Index\n"
            "table Project { id: ProjectId @id, name: Text.NonEmpty, org: Org.OrgId @ref Org @partition org }\n```\n\n"
            "- The column is one `@ref` of the key's type without `Maybe`, and the session has the key\n"
            "  (E0032).\n"
            "- The generic scoped operations `Db.scoped T`, `Db.getScoped T id`, `Db.updateScoped T id f`\n"
            "  (never changes `org`), `Db.deleteScoped T id` and `Db.insertScoped T { … }` (a literal\n"
            "  without `org`) keep to the session's org; another org's row is not found. `Db.mine` on an\n"
            "  org table is E0025 (\"… write `Db.scoped Project`\").\n"
            "- They need a page that `requires user, org` (E0026); a `via` key comes after `user`\n"
            "  (E0027). There `session.org : Org.OrgId`. A visitor is sent to log in, a user without an\n"
            "  org to `choose org`'s page (required for a `via` key, E0032; that page must not require\n"
            "  the key); a key with no `choose` answers 404.\n"
            "- `via Membership`: a `Session.put` of `org` to `Just v` fails the `Task` (\"Session.put org:\n"
            "  no Membership for this user and org\") unless a `Membership` row of the session's user\n"
            "  and `v` exists then, inside the `Task`'s transaction (a row inserted earlier in the `Task`\n"
            "  counts; one deleted counts as gone). `Nothing` is always allowed, and a session that loses\n"
            "  its user loses its org. The membership table has exactly one `@ref` to the user table and\n"
            "  one to the key's (E0032); one `via` field per session. Picking an org:\n\n"
            "```alder fragment\nOpen p -> {\n  let _ <- Session.put (\\s -> { s | org: Just p.org })\n"
            "  Task.succeed (Redirect (Projects.Index {}))\n},\n```\n\n"
            "  A crafted `Open` of an org the user is no member of is refused; a test calls `update`\n"
            "  directly to send it.\n"
            "- The cookie keeps `org`; it is not checked again per request. A page that must notice a\n"
            "  removed member at once re-reads the membership in `load` (the re-read idiom):\n\n"
            "```alder fragment\nisMember : User.UserId -> Org.OrgId -> Query Bool\nisMember user org = {\n"
            "  let n <- Db.from Membership |> Db.where (Membership.user.eq user) |> Db.where (Membership.org.eq org) |> Db.count\n"
            "  Query.succeed (n > 0)\n}\n```\n\n"
            "  `load` begins `let member <- Membership.isMember session.user session.org` and answers\n"
            "  `Query.notFound` when it is `False`.\n"
            "- The admin form `requires user, role Admin, silo org` takes `org` from the page's\n"
            "  parameter `{ org: Org.OrgId, … }`, guarded by the role, not `via` (E0033).\n"
            "- Not in a fake or `App` (E0026); an org's name stays unscoped (`Db.get Org session.org`).\n"
            "- With `silo org { Project, Todo }` (chapter 8, \"Silos\") each org's rows are in its own\n"
            "  database, and a `Task` writes to one store (E0031).\n\n"),
           ("The to-do list's page uses four of them\n", "Why `update` needs them as much as `load` does",
            "The to-do list's page uses four of them, and `Todos.Show` reads its todo with the fifth\n"
            "(`examples/todos/app/Todos/Index.alder` and `Todos/Show.alder`, in the example app below).\n\n"),
           ("Two small ideas first, because a file has to say both.", "The compiler decides a page is a file page",
            "A `File` carries a content type (`text/csv`), a status (`200`, `404`, `503`), a body and extra headers.\n\n"),
           ("`File.text` answers `200` with the text", "`File.html` is the way to answer a web page",
            "`File.withHeader` sets a header: `Content-Disposition: attachment; filename=todos.csv` saves the answer\n"
            "(`File.text` etc.: chapter 9).\n\n"
            "Path segments are whole: `\"/reports/{id}/report.csv\"` is valid, `\"/reports/{id}.csv\"` is not.\n\n"),
           ("`File.html` is the way to answer a web page, byte for byte.", "`File.status`, `File.contentType` and `File.body` read",
            "`File.html` answers a web page byte for byte. Markup in a `view` has one root element and\n"
            "no doctype, so to start with `<!doctype html>` use `Html.fragment [Html.unsafe \"<!doctype html>\\n\", <html>…</html>]`.\n\n")],
    "08": [("## Silos\n\nA partition keeps", "### One store per `Task`",
            "## Silos\n\n`silo org { Project, Todo }` in `App` keeps those tables in a database per org; the\n"
            "rest are in the shared database. Each listed table is `@partition org`; a table is in one silo;\n"
            "a shared table may not `@ref` a siloed one, a siloed one may `@ref` a shared one or one of its\n"
            "silo (E0033). Every `Db` operation on a siloed table needs the key narrowed (E0026); there is\n"
            "no query over every org. `Db.update` cannot move a row to another org.\n\n"),
           ("### One store per `Task`\n", "## Beyond CRUD",
            "### One store per `Task`\n\nA `Task` is one transaction, so it writes to one store, the shared one or\n"
            "one silo; an `update` arm whose writes (followed into helpers, any module, and `Task.all`)\n"
            "fall in two is E0031: \"`Create` writes `Project` (silo `org`) and `AuditLog` (shared): a\n"
            "Task writes to one store. Write `AuditLog` inside the silo, or send it from its own\n"
            "message.\" Reads may mix (one `load` reads an org's name and its projects); `Session.put`\n"
            "and capabilities do not count.\n\n"),
           ("A table with an `@owner` column (above) also has five **scoped**\n", "## How tables are stored",
            "A table with an `@owner` column (above) also has five **scoped**\noperations, `Db.mine`, "
            "`Db.getMine`, `Db.updateMine`, `Db.deleteMine` and\n`Db.insertMine`, for the pages of a signed-in "
            "user: chapter 6, \"Rules\",\nexplains them, and chapter 9 lists their types.\n\n")],
    "09": [("`Float.sqrt`, `Float.exp`, `Float.log` and `Float.pow` are", "\n## Bool",
            "IEEE rules: `Float.sqrt (-1.0)` is NaN, `Float.log 0.0` -infinity; no failures.\n"),
           ("\n`Time.toMillis` counts milliseconds", "\n## File, Bytes", "\n"),
           ("A file page's `view` returns a `File`", "```alder\nFile.text ",
            "`Bytes` is the type of a non-text body; `Text.toBytes` makes one.\n\n"),
           ("\n`Bytes.length` counts bytes, not letters", "\n## Query, Task, Db",
            "\n"),
           ("- `Test.member : u -> k -> Task Unit` (M15)", "\n### Browser steps and queries",
            "- `Test.member ann acme : Task Unit` inserts the `via` table's row making the user a member\n"
            "  of the key (other columns `Nothing` or their default; E0003 if one is required).\n"
            "- `Test.fetch : Page -> Task { status: Int, contentType: Text, body: Bytes, headers: List { name: Text, value: Text } }`\n"
            "  requests a file page: the raw answer, redirects not followed, headers sorted by name.\n"),
           ("\nRun without the key (a test whose session has no `org`)", "`Query.notFound` and `Task.notFound`",
            "\nWithout the key they fail (\"Db.insertScoped Project: the session has no org\"). On a siloed\n"
            "table (chapter 8, \"Silos\") every `Db` operation needs the key.\n\n")],
    "10": [("**The partition mutants.**", "### The rules' mutants in a report",
            "`membership dropped` dies only to a test that tries an org the user is no member of.\n\n"),
           ("Without a signed-in user a scoped step fails, and says why.", "**Where an effect test runs.**",
            "Without a signed-in user a scoped step fails, saying so (`Err\n\"Db.updateMine Todo: no signed-in user\"`); "
            "it never answers as if the user\nhad no rows. A test may use the scoped operations itself too, after\n"
            "`Test.login`.\n\nWith a key (chapter 6, \"Partitions\"), `Test.login { user: Just ann, org: Just acme }`\n"
            "needs the membership first, as `Session.put` does: `Test.member ann acme` inserts it. Each org a\n"
            "test touches has its own in-memory database (a silo); insert another org's siloed rows before\n"
            "`Test.login`, as production refuses them in a request of this org. A crafted switch is a direct\n"
            "call: `Test.run (update (Open { org: globex }) model session)` is `Err \"Session.put org: no\n"
            "Membership for this user and org\"`, which kills the mutant `membership dropped`.\n\n"),
           ("**Where an effect test runs.** Every effect test gets a world of its own:", "A pure test (section 1) touches none of this",
            "**Where an effect test runs.** Each effect test gets a fresh, empty\nin-memory SQLite database with the "
            "app's tables; its block runs as one\n`Task`, in one transaction, and sees its own writes. `Time.now` and "
            "the\n`@created` a `Db.insert` fills read a clock that starts at\n`2026-01-01T00:00:00Z` and advances one "
            "second per read, so the rows a\ntest creates are ordered. The app's capabilities are wired to their\n"
            "adapters as in `alder build`; one without an adapter is a warning, and\ncalling it unfaked fails the test "
            "(\"capability Auth.Login.Password is\nnot wired in tests\"); `Test.fake` replaces one (section 8). The "
            "test's\nsession starts as `App`'s, every field at its default, and its browser\n(section 9) holds no page "
            "and no cookie.\n\n"),
           ("**Who may toggle a todo.** Chapter 6's rule", "My list is open in the browser;",
            "**Who may toggle a todo.** Chapter 6's rule, \"Rules\", says that a user\nwho posts `Toggle` with "
            "someone else's todo id gets a 404. A stranger's\nown list offers no button for my todo, but a page can "
            "be out of date. "),
           ("\"Delete by another user is not\nfound\" does the same with the Delete button.", "### Reading the page: queries",
            "\"Delete by another user is not\nfound\" does the same with the Delete button.\n\n")],
}

# A `alder error=CODE "text"` block of a whole module (written to show the compiler refusing it)
# is rendered by --reference as the error alone, `error[CODE]: text`, in a text block: the
# prose around it says what the module does wrong.
ERROR_MODULE = re.compile(r'^```alder error=(E\d+) "([^\n]*)"\nmodule [^\n]*\n.*?^```\n', re.S | re.M)


def error_text(m):
    return "```text\nerror[" + m.group(1) + "]: " + m.group(2) + "\n```\n"


def trim(path, text, spans):
    """text with each (start, end, replacement) of spans applied; exits 2 naming a span whose start
    is not found exactly once or whose end does not follow it."""
    for start, end, repl in spans:
        i = text.find(start)
        j = text.find(end, i + len(start)) if i >= 0 else -1
        if i < 0 or text.count(start) != 1 or j < 0:
            print(f'build_context.py: --reference: {path} has no single span from {start!r} to {end!r} '
                  f"(update TRIMS to match the book)", file=sys.stderr)
            raise SystemExit(2)
        text = text[:i] + repl + text[j:]
    return text


HEADER = """You are writing code in Alder, a small language for server-rendered web
apps. Alder is not Elm, not TypeScript, and not any language you have seen
before; the reference below is complete. Anything not in the reference does
not exist. Follow it exactly.

====================  ALDER REFERENCE  ====================
"""

# The command table (book 15, chapter 11's "The generators"): one line per command with its
# flags, so an agent driving the app from the command line (E11) has the whole surface; the
# chapters themselves (11, 15) are for a human reader and are not in the reference.
COMMANDS = """
====================  COMMANDS  ====================
alder new <app>   the guestbook: App, Layout, table Entry, page Home at /, its tests. --bare: App
(empty session) and Layout only; start a task there. Typical: tables, `new auth` (before any
`--requires user` page), pages (`--blank` first, then fill: a body may name only what exists), fills.
The generators check a copy, writing only if it checks (edit table/session: written with the
breakage listed; --strict refuses); none overwrites. All take --dir D, --dry-run, --json, --help.
A hole stops `alder test` (it refuses while one is left): run it last, after the fills. Pass stdin with <<'EOF' (echo eats the `\\f` of `\\from`).
alder new table <Name> <field>:<Type>[=default][@ref=T][@cascade|@restrict|@setNull][@owner][@partition=k][@unique] … [--silo]
  adds created/updated (Time @created/@updated) unless --no-timestamps (then `created:Time@created`);
  `org:Org.OrgId@ref=Org@partition=org`; --silo puts it in App's `silo org` (a table referring to a siloed one must)
alder new silo <key> <T> … [--empty]   App's `silo key { T, … }`; each T must be @partition key;
  a T migrations/ create needs --empty (no rows yet: it writes the move's migration)
alder new page <Module> <path> [--list T | --show T [--by <unique col>] | --from T …] [--requires user|org] [--role R]
  --requires org writes `requires user, org`; --list/--show read an org-partitioned T with Db.scoped/getScoped
  a {x} in the path needs --param x:Type (load is then ?load); --show, --list and --from make their own
  parameters: --param only with --blank or a plain page. A --list page's Model and load go together:
  replace both (fill --decl), or start --blank
alder new form <Page> <Name> <field>:<Type> … [--into T] [--msg M]   (msg M, default Submit<Name>)
alder new msg <Page> <Name> [<field>:<Type> …] [--id id]
  form and msg refuse while view is ?view: fill view first; the view must be one element over several
  lines (`<Layout.Page …>` … `</Layout.Page>`)
alder new msg App <Name> [<field>:<Type> …]   a message of App (a button on any page): an arm with a hole
  in App.update and a test with a hole
alder new form <Page> <Form> <field>:<Type> … --msg M   when `msg M { … }` exists without a form: it becomes
  `msg M { … } form F` (its arm reads p.form); refused if M has a form already
alder new capability <Module> <entry> "<Type>"
alder new auth [--users User] [--role R] [--to Page]   the login (Auth.Login, Password with a real
  PBKDF2 adapter, App's session user/role, login, Logout); creates User (passwordHash: Text.Secret)
  if absent, else uses yours; R must exist; no / page and no --to: ?afterLogin, fill it later
alder edit auth --to <Page>   fills ?afterLogin in Auth.Login.update and its test (refused if none)
alder edit decl Layout.Page add-prop <name>:<Type>=<default>   a prop on a component; every use keeps checking
alder new type <Module> "Status = A | B" | "Slug = brand Text where …" | "Row = { … }"
alder new decl <Module> <name> "<Type>"   (name = ?name; fill a function with a lambda)
alder new test <Module> "<name>"   (its block is ?test : Task Bool; fill it with a block)
alder new page … --blank   Model = { }, load and view holes; with --show T keeps /{id}
alder new msg … --in-for <binder> [--when <Bool expr>]   the button in that `for`'s element,
  class = the msg's name, fields from the binder (u.id) or holes; its test clicks a hole:
  insert rows the loop shows the signed-in user, fill the hole with one
alder edit page <Page> add-param <name>:<Type>[=default]
alder edit session add-field <name>:<Type>=<default>
alder edit session add-field org:Org.OrgId via Membership [--choose <Page>]   a key: `org: Maybe Org.OrgId via
  Membership` (+ `choose org = Page`); the Membership table and the page must exist first
alder remove page <Page> | remove table <T>   refused while used; a table's migration drops data
alder fill --decl <Module>.<decl> -   replaces a whole body (value, test, type's right side):
  for Model (no hole) and bodies that do not fit; prefer a hole when there is one. The parameters
  of `load params session = ?load` are bound: send the body, not `\\params session -> …`
alder edit table <T> add-field <f> | rename-field <old> <new> | drop-field <f> [--strict]
  add-field without a default (=Nothing for a Maybe) breaks every insert of T (tests too), which
  no command fixes: give a default, or drop-field it
alder fill <address> - [--hole ?name]   fills a hole only (an arm without one: --decl update);
  fill --list. Addresses: Module.decl, Module.update.Msg, Module."test name", Module.Fake.entry;
  an app module the expression names (Member.x) is imported
alder new admin [--module Admin] [--path /admin]   (needs a session role with Admin)
alder regen admin [--module Admin] [--path /admin] [--all]
App may declare where it ships (book 16), once: `deploy { production: Ssh "u@host" { domain: "d",
  path: "/srv/x" }, staging: Fly "ord" { app: "a" }, local: Script "./s.sh", floor: 0.9, checks: ["cmd"],
  requires: ["SMTP_URL"], backup: S3 "s3://b/p", keep: 5, proxy: None }`; names only, never a secret (E0030)
alder deploy <env> [--dry-run] [--host h] [--allow-data-loss] [--unsafe-skip-tests]   the gate (check, build,
  test, --mutate >= floor, checks) then plan and ship; alder status|logs <env>; alder rollback <env> [id]
  [--restore-backup --yes]; alder restore <env> [--silo org:<id>] [--to t] [--out f] [--yes]; alder ci init
  (Fly: experimental, refuses a deploy with a pending migration); with silos: alder migrate silos <env>
  [--allow-data-loss], alder silos prune <env> [--yes]

"""

EXAMPLE_HEADER = """
====================  EXAMPLE APP: examples/todos  ====================
A complete, correct Alder application. Match its style exactly.
"""

FENCE = re.compile(r"^\s*(```|~~~)")


def sections(text):
    """Split a chapter into [(heading or None, text)]: the part before the first `## ` line, then
    one part per `## ` heading up to the next. A `## ` line inside a code fence is not a heading.
    Joining the texts gives the chapter back."""
    out, cur, head, fenced = [], [], None, False
    for line in text.splitlines(keepends=True):
        if FENCE.match(line):
            fenced = not fenced
        if not fenced and line.startswith("## "):
            out.append((head, "".join(cur)))
            cur, head = [], line[3:].rstrip()
        cur.append(line)
    out.append((head, "".join(cur)))
    return out


def subsections(text):
    """Split a `## ` section's text into [(heading or None, text)] at its `### ` lines, outside
    code fences, as sections does at `## ` lines."""
    out, cur, head, fenced = [], [], None, False
    for line in text.splitlines(keepends=True):
        if FENCE.match(line):
            fenced = not fenced
        if not fenced and line.startswith("### "):
            out.append((head, "".join(cur)))
            cur, head = [], line[4:].rstrip()
        cur.append(line)
    out.append((head, "".join(cur)))
    return out


def check_subsections(path, h, subs, listed, table):
    """Exits 2 naming the first of listed (`### ` headings) not among subs, the `## ` section
    h's subsections; table names the dict to update."""
    names = {sh for sh, _ in subs}
    for sh in listed:
        if sh not in names:
            print(f'build_context.py: --reference: {path} has no subsection "### {sh}" in "## {h}" '
                  f"(update {table} to match the book)", file=sys.stderr)
            raise SystemExit(2)


def drop_sections(path, text, headings, keep=None, drop=None):
    """text without the `## ` sections named in headings, but for the `### ` subsections that
    keep (a dict: `## ` heading -> [`### ` headings]) lists for a dropped section, which stay
    after that section's `## ` line; and without the `### ` subsections drop (the same shape)
    lists for a kept section; exits 2 naming a heading not found."""
    keep, drop = keep or {}, drop or {}
    parts = sections(text)
    found = {h for h, _ in parts}
    for h in list(headings) + list(drop):
        if h not in found:
            print(f'build_context.py: --reference: {path} has no section "## {h}" '
                  f"(update HUMAN_SECTIONS or DROP_SUBSECTIONS to match the book)", file=sys.stderr)
            raise SystemExit(2)
    out = []
    for h, t in parts:
        if h not in headings:
            if not drop.get(h):
                out.append(t)
                continue
            subs = subsections(t)
            check_subsections(path, h, subs, drop[h], "DROP_SUBSECTIONS")
            out.extend(st for sh, st in subs if sh not in drop[h])
            continue
        if not keep.get(h):
            continue
        subs = subsections(t)
        check_subsections(path, h, subs, keep[h], "KEEP_SUBSECTIONS")
        out.append(t.splitlines(keepends=True)[0] + "\n")
        out.extend(st for sh, st in subs if sh in keep[h])
    return "".join(out)


# Appendix C's elements, compact (M7a): --reference appends one line per `al-` element (its
# attributes, then its slots) to chapter 5's "Custom elements", read from the appendix's
# attribute tables and "Slots:" lines (which the book's tests hold to the vendored manifest).
DESIGN_SYSTEM = "C-design-system.md"
ELEMENT_TABLE_BEFORE = "## Partial updates"


def element_table(book):
    """The compact element table from book/C-design-system.md: a short header, then per element
    `al-x attr, flag!, choice=a*|b|c / slot, default` (`!` boolean, `*` the default choice)."""
    path = book / DESIGN_SYSTEM
    if not path.exists():
        print(f"build_context.py: --reference: {path} is missing (the element table comes from it)", file=sys.stderr)
        raise SystemExit(2)
    rows, tag = [], None
    for line in path.read_text().splitlines():
        m = re.match(r"^### `(al-[a-z-]+)`$", line)
        if m:
            tag = m.group(1)
            rows.append([tag, [], ""])
            continue
        if line.startswith("## ") or line.startswith("### "):
            tag = None
        if tag is None:
            continue
        if line.startswith("| `"):
            cells = [c.strip() for c in re.split(r"(?<!\\)\|", line.strip().strip("|"))]
            name, typ, default = (c.strip("`").replace("\\|", "|") for c in cells[:3])
            if typ == "boolean":
                attr = name + "!"
            elif '"' in typ:
                choices = [c.strip().strip('"') for c in typ.split("|")]
                d = default.strip('"')
                attr = name + "=" + "|".join(c + ("*" if c == d else "") for c in choices)
            else:
                attr = name
            rows[-1][1].append(attr)
        elif line.startswith("Slots: "):
            slots = re.sub(r" \(.*?\)", "", line[len("Slots: "):].rstrip(".")).replace("`", "")
            rows[-1][2] = "" if slots == "none" else " / " + slots
    out = ["The built-in elements (Appendix C; `!` boolean, `*` default, slots after `/`). Link\n"
           "`/static/al/al.css` in the layout (every build serves it). An `<al-button>` with `click`,\n"
           "`type` or `href` renders a real `<button>`/`<a>` inside, so its children are text and inline\n"
           "markup only (no control, `Html` value or component) and it takes `href` or `click`, not both\n"
           "(E0028); `<al-alert>` gets `role`. Link `/static/al/al.js` (`<script type=\"module\" src=\"...\">`,\n"
           "a plain string) for the interactive elements. A button opens a dialog or popover with\n"
           "`commandfor=\"id\" command=\"show-modal|close|toggle-popover\"` (no `click`); without\n"
           "JavaScript dialog, popover, menu and tabs work natively, a tooltip is hidden (repeat its\n"
           "text in `title`), a toast is not seen (use `al-alert`). Toasts come from `Notice.with\n"
           "\"text\" nav`.\n\n"]
    for tag, attrs, slots in rows:
        out.append(f"- `{tag}` {', '.join(attrs) or '(none)'}{slots}\n")
    return "".join(out) + "\n"


def build(chapters, book, example, reference):
    out = [HEADER, COMMANDS]
    for ch in chapters:
        for f in sorted(book.glob(f"{ch}-*.md")):
            text = f.read_text()
            if reference:
                text = trim(f, text, TRIMS.get(ch, []))
            if reference and (ch in HUMAN_SECTIONS or ch in DROP_SUBSECTIONS):
                text = drop_sections(f, text, HUMAN_SECTIONS.get(ch, []), KEEP_SUBSECTIONS.get(ch), DROP_SUBSECTIONS.get(ch))
            if reference:
                text = ERROR_MODULE.sub(error_text, text)
            if reference and ch == "05":
                if ELEMENT_TABLE_BEFORE not in text:
                    print(f'build_context.py: --reference: {f} has no "{ELEMENT_TABLE_BEFORE}" '
                          f"(update ELEMENT_TABLE_BEFORE to match the book)", file=sys.stderr)
                    raise SystemExit(2)
                text = text.replace(ELEMENT_TABLE_BEFORE, element_table(book) + ELEMENT_TABLE_BEFORE, 1)
            out.append(text)
            out.append("\n")
    if example is not None:
        out.append(EXAMPLE_HEADER)
        for f in sorted(example.rglob("*.alder")):
            # the admin `alder new admin` generated (app/Admin/, book 15) is left out: it is the
            # generator's output, not an example to write by hand, and it would double the prompt
            if f.relative_to(example).parts[0] == "Admin":
                continue
            rel = f.relative_to(example.parent)
            out.append(f"\n--- file: {rel} ---\n")
            out.append(f.read_text())
    return "".join(out)


def ollama_count(model, prompt, num_ctx, host="http://localhost:11434"):
    """Ollama's prompt_eval_count for prompt as the system message alone (num_predict 1)."""
    body = {"model": model, "stream": False, "options": {"num_ctx": num_ctx, "temperature": 0, "num_predict": 1},
            "messages": [{"role": "system", "content": prompt}]}
    req = urllib.request.Request(host + "/api/chat", data=json.dumps(body).encode(),
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=3600) as r:
        return json.load(r).get("prompt_eval_count")


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--chapters", default=",".join(DEFAULT_CHAPTERS))
    ap.add_argument("--no-example", action="store_true")
    ap.add_argument("--reference", action="store_true", help="drop the sections written for humans (HUMAN_SECTIONS)")
    ap.add_argument("--size", action="store_true", help="print the prompt's size (characters, len/4) instead of the prompt")
    ap.add_argument("--count", metavar="ollama:MODEL", help="with the size, Ollama's prompt_eval_count for the prompt (implies --size)")
    ap.add_argument("--num-ctx", type=int, default=65536, help="num_ctx for --count (default 65536: large enough not to truncate)")
    ap.add_argument("--book", default=str(BOOK), help="the book directory (default: book/)")
    ap.add_argument("--example", default=str(EXAMPLE), help="the example's app directory (default: examples/todos/app)")
    args = ap.parse_args(argv)
    if args.count and not args.count.startswith("ollama:"):
        ap.error("--count takes ollama:MODEL")

    prompt = build(args.chapters.split(","), pathlib.Path(args.book),
                   None if args.no_example else pathlib.Path(args.example), args.reference)
    if not (args.size or args.count):
        print(prompt)
        return
    print(f"characters: {len(prompt)}")
    print(f"estimate (len/4): {len(prompt) // 4}")
    if args.count:
        model = args.count.partition(":")[2]
        try:
            n = ollama_count(model, prompt, args.num_ctx)
        except Exception as e:   # connection refused, timeout, bad JSON
            print(f"build_context.py: --count {args.count}: {e}", file=sys.stderr)
            raise SystemExit(1)
        print(f"prompt_eval_count ({args.count}, num_ctx {args.num_ctx}): {n}")
        if isinstance(n, int) and n >= args.num_ctx:
            print(f"build_context.py: warning: the count reached num_ctx {args.num_ctx}; raise --num-ctx", file=sys.stderr)


if __name__ == "__main__":
    main()
