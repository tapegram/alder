# workspace

The third example, which chapter 14 tours after the blog: orgs, the users who are their members,
and each org's projects and to-dos, which live in a database of the org's own. It is the
example of partitions and silos (book 06, "Partitions"; book 08, "Silos"): the session's `org`
is a key `via Membership`, `choose org = Orgs.Index` is where a session without one is sent,
`silo org { Project, Todo }` puts those two tables in a database per org, and `Activity`, the
org's log, is partitioned by org in the shared database instead. `alder check`, `alder build`
and `alder test` take this directory; chapter 11 explains each command.

To try it: `alder run`, then log in at `/login` with the password `password` as
`ann@example.com` or `bob@example.com` (Members) or `admin@example.com` (an Admin: `/admin`).
The accounts are the development stub's (`adapters/auth/password.go`), which accepts them only
under `alder run`. Make an org on `/orgs` as Ann, add Bob to it by email on `/members`, and log
in as Bob: he sees Ann's org's projects, and none of an org he is not in. Under `alder run` the
shared database is `build/workspace/dev.db` and each org's a file of its own under
`build/workspace/silos/org/`, named after the org's id (book 08, "Where the files live").

## What it shows

- **A key with a membership.** `session { user: …, org: Maybe Org.OrgId via Membership, … }`:
  `Orgs.Index`'s `Open` puts an org in the session, and the runtime refuses one the user has no
  `Membership` row for (`Orgs.Index "a non-member's switch is refused"`, a request crafted by
  hand), also when the row was deleted earlier in the same Task (`Orgs.Index "a membership
  deleted earlier in the same Task counts as gone"`). Logging in puts the first org the user
  joined (`Auth.Login`); logging out clears it with the user.
- **Pages that require the key.** `Projects.Index`, `Projects.Show` and `Members.Index` are
  `requires user, org`: a visitor is sent to log in, a user without an org to `Orgs.Index`
  (`Projects.Index "without an org the page sends you to choose one"`).
- **The scoped operations.** `Db.scoped Project`, `Db.getScoped`, `Db.insertScoped`,
  `Db.updateScoped`, `Db.deleteScoped` read and write the session's org's rows only: a project
  of another org is not found, and a crafted `Remove` of one changes nothing (`Projects.Index
  "another org's project cannot be removed"`, `Projects.Show "another org's project is not
  found"`).
- **A silo.** Project and Todo are in `silo org`: each org's rows are in its own file, so another
  org's are not merely filtered out but absent (`Projects.Index "a project is created in the
  org's database, and another org does not see it"`). `Todo.project`'s `@cascade` works inside
  the org's database (`Projects.Show "deleting a project deletes its to-dos, in the org's
  database"`).
- **The logical form.** Activity is `@partition org` but in no silo: one shared table, read with
  `Db.scoped Activity`, which adds the `WHERE` (`Members.Index "the members, by name, and the
  activity of this org only"`).
- **A read of each store in one load.** `Projects.Index` reads the org's name (shared) and its
  projects (its silo) in one `load` (`Projects.Index "a member sees the org's name and only its
  projects, by name"`).
- **One store per Task.** Every Task writes to one database: a project's Tasks write the org's;
  `Orgs.Index`'s `Create` (an Org, a Membership, a line of Activity) and `Members.Index`'s `Add`
  and `Remove` (a Membership and a line of Activity) write the shared one. A Task that wrote a
  project and a line of Activity would be E0031 (book 11).
- **The re-read idiom.** The cookie keeps `org` as it was put; it is not checked again on every
  request. The three org pages call `Membership.isMember` in their `load`, so a member removed
  since is turned away at once (`Projects.Index "a member removed from the org loses access at
  once"`, `Members.Index "removing a member logs it, and a removed member loses access at
  once"`).
- **The admin of a silo.** `/admin/orgs` lists every org; each opens its dashboard
  (`/admin/orgs/{org}`) and the pages of its projects and to-dos (`/admin/org/{org}/project…`),
  declared `requires user, role Admin, silo org`: the org comes from the page's parameter and the
  role check guards it, so an Admin need not be a member (`Admin.Project.Index "an Admin who is
  no member of the org lists its projects, and only its"`, `Admin.Project.Bulk "a bulk action
  stays inside one org"`, and a Member's 404 on each page).

## Mutation score

`alder test --mutate examples/workspace` (book 10, "Mutation testing"): 522 mutants, 307 tests,
511 killed, 11 survived (score 0.979). `TestExample_Workspace` fails if the score drops below
0.97, the exact 0.979 rounded down, or the number of mutants is not 522.

- The app's own code (34 tests): every mutant killed, `membership dropped` (the `via` check
  removed) by `Orgs.Index "a non-member's switch is refused"`, and `scope dropped` of
  `Db.scoped Activity` by `Members.Index "the members, by name, and the activity of this org
  only"`. The `scope dropped` of the siloed tables' scoped operations is not made: the org's
  database holds that org's rows only, so the mutant is equivalent, and the run says so.
- The generated admin (273 tests, `app/Admin/`, kept as `alder new admin` wrote them): the 11
  survivors. Five are `> 200` flipped to `>= 200` in the New pages' searches (`orgsTotal`,
  `usersTotal`, `projectsTotal`): a search of "" matches every row, as in the blog. Six are in
  Membership's list, whose columns are all references (and its `created`): no column has a sample
  order to sort by, so the list has no sort test (`order` and `toggle` survive), and its search
  and filters are `Db.and []` (the two `where dropped` are equivalent).

## Replaying the admin

Every file under `app/Admin/` is exactly what `alder new admin` wrote; none was edited by hand.
To check that the committed admin is still what the generator writes, in a copy of the
workspace:

```
rm -r app/Admin
alder new admin
git diff --stat        # nothing
```

`alder new admin` prints the files it writes: the pages of each shared table (Activity,
Membership, Org, User) under `app/Admin/<Table>/`, the siloed tables' (Project, Todo) the same
way at `/admin/org/{org}/…`, the picker and the dashboard of one org (`app/Admin/Orgs/Index.alder`
and `Show.alder`), the dashboard (`app/Admin/Index.alder`) and the layout. After a change to a
table, `alder regen admin` rewrites only the admin's owned regions (book 15).

## Migrations

`migrations/0001_initial.sql` creates every table; `migrations/schema.json` records which ones
live in `silo org`. The shared database has Project and Todo too, empty: the rows are in the
orgs' files, each created and migrated when the org is first opened (book 08, "The open-time
check"). Moving a table into the silo later is refused by `alder migrate` (book 08, "Silos").
